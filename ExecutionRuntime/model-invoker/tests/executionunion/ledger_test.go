package executionunion_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func ledgerHeader(sequence uint64, family union.EventFamily) union.EventHeader {
	return union.EventHeader{
		EventID: union.EventID(fmt.Sprintf("event-%d", sequence)), SemanticVersion: union.SemanticVersionV1,
		ExecutionID: "exec-ledger", Sequence: sequence, Timestamp: fixtureTime.Add(time.Duration(sequence) * time.Second),
		Origin: union.EventOriginPraxis, Family: family, Visibility: union.VisibilityAuditOnly,
		SecurityClassification: union.SecurityInternal, ExecutionKind: union.ExecutionKindModel,
		Profile: union.VersionedIdentity{ID: "profile-test", Version: "v1"},
		Route:   union.VersionedIdentity{ID: "route-test", Version: "v1"},
	}
}

func appendCoreLedger(t *testing.T, ledger *execution.EventLedger, sideEffects union.SideEffectState) {
	t.Helper()
	invocation := validInvocation("exec-ledger")
	plan := invocation.Plan.Mechanisms[0]
	events := []union.UnifiedExecutionEvent{
		{Header: ledgerHeader(1, union.EventFamilyLifecycle), Lifecycle: &union.LifecycleEvent{Kind: "execution_started"}},
		{Header: func() union.EventHeader {
			header := ledgerHeader(2, union.EventFamilyIntent)
			header.IntentID = "intent-1"
			return header
		}(), Intent: &union.IntentEvent{Kind: "accepted"}},
		{Header: func() union.EventHeader {
			header := ledgerHeader(3, union.EventFamilyMechanism)
			header.IntentID = "intent-1"
			header.MechanismPlanID = "plan-1"
			return header
		}(), Mechanism: &union.MechanismEvent{Kind: "planned", Plan: &plan}},
		{Header: func() union.EventHeader {
			header := ledgerHeader(4, union.EventFamilyMechanism)
			header.IntentID = "intent-1"
			header.MechanismPlanID = "plan-1"
			return header
		}(), Mechanism: &union.MechanismEvent{Kind: "selected", Plan: &plan}},
		{Header: func() union.EventHeader {
			header := ledgerHeader(5, union.EventFamilyMechanism)
			header.IntentID = "intent-1"
			header.MechanismPlanID = "plan-1"
			header.MechanismAttemptID = "attempt-1"
			return header
		}(), Mechanism: &union.MechanismEvent{Kind: "attempt_started", Attempt: &union.MechanismAttempt{
			ID: "attempt-1", MechanismPlanID: "plan-1", Authoritative: true, ActualKind: "caller_tool",
			ActualOrigin: union.CapabilityOriginCallerHosted, ActualOwner: union.ExecutionOwnerPraxis,
			Status: union.AttemptStatusRunning, SideEffectState: sideEffects,
		}}},
	}
	for _, event := range events {
		if err := ledger.Append(event); err != nil {
			t.Fatalf("Append core event %d: %v", event.Header.Sequence, err)
		}
	}
}

func TestLedgerRejectsSequenceDriftAndEventsAfterTerminal(t *testing.T) {
	ledger, err := execution.NewEventLedger("exec-ledger")
	if err != nil {
		t.Fatal(err)
	}
	bad := union.UnifiedExecutionEvent{Header: ledgerHeader(2, union.EventFamilyLifecycle), Lifecycle: &union.LifecycleEvent{Kind: "execution_started"}}
	if err := ledger.Append(bad); !errors.Is(err, execution.ErrSequence) {
		t.Fatalf("sequence drift error = %v", err)
	}
	appendCoreLedger(t, ledger, union.SideEffectNone)
	appendReconciliation(t, ledger, union.SideEffectNone)
	appendRouteCandidate(t, ledger, union.ExecutionStatusFailed, union.SideEffectNone)
	if ledger.State().Terminal {
		t.Fatal("route terminal candidate became a unified terminal")
	}
	terminal := union.UnifiedExecutionEvent{
		Header:    ledgerHeader(ledger.NextSequence(), union.EventFamilyLifecycle),
		Lifecycle: &union.LifecycleEvent{Kind: "execution_failed", Status: union.ExecutionStatusFailed},
	}
	if err := ledger.Append(terminal); err != nil {
		t.Fatalf("Append terminal: %v", err)
	}
	after := union.UnifiedExecutionEvent{
		Header: ledgerHeader(ledger.NextSequence(), union.EventFamilyLifecycle), Lifecycle: &union.LifecycleEvent{Kind: "late_progress"},
	}
	if err := ledger.Append(after); !errors.Is(err, execution.ErrTerminal) {
		t.Fatalf("post-terminal error = %v", err)
	}
}

func TestLedgerRequiresMechanismSelectionBeforeAttempt(t *testing.T) {
	ledger, _ := execution.NewEventLedger("exec-ledger")
	invocation := validInvocation("exec-ledger")
	plan := invocation.Plan.Mechanisms[0]
	started := union.UnifiedExecutionEvent{Header: ledgerHeader(1, union.EventFamilyLifecycle), Lifecycle: &union.LifecycleEvent{Kind: "execution_started"}}
	acceptedHeader := ledgerHeader(2, union.EventFamilyIntent)
	acceptedHeader.IntentID = "intent-1"
	plannedHeader := ledgerHeader(3, union.EventFamilyMechanism)
	plannedHeader.IntentID, plannedHeader.MechanismPlanID = "intent-1", "plan-1"
	for _, event := range []union.UnifiedExecutionEvent{
		started,
		{Header: acceptedHeader, Intent: &union.IntentEvent{Kind: "accepted"}},
		{Header: plannedHeader, Mechanism: &union.MechanismEvent{Kind: "planned", Plan: &plan}},
	} {
		if err := ledger.Append(event); err != nil {
			t.Fatal(err)
		}
	}
	attempt := attemptCandidate(union.SideEffectNone, 1)
	attempt.Header = ledgerHeader(4, union.EventFamilyMechanism)
	attempt.Header.IntentID, attempt.Header.MechanismPlanID, attempt.Header.MechanismAttemptID = "intent-1", "plan-1", "attempt-1"
	if err := ledger.Append(attempt); !errors.Is(err, execution.ErrLedgerInvariant) {
		t.Fatalf("attempt before selection error = %v", err)
	}
}

func TestPlanBoundLedgerRejectsAdapterIntentAndMechanismInjection(t *testing.T) {
	invocation := validInvocation("exec-ledger-bound")
	ledger, err := execution.NewEventLedgerForPlan(invocation.Plan)
	if err != nil {
		t.Fatal(err)
	}
	headerFor := func(sequence uint64, family union.EventFamily, origin union.EventOrigin) union.EventHeader {
		header := ledgerHeader(sequence, family)
		header.ExecutionID = invocation.Request.ExecutionID
		header.Origin = origin
		return header
	}
	if err := ledger.Append(union.UnifiedExecutionEvent{
		Header: headerFor(1, union.EventFamilyLifecycle, union.EventOriginPraxis), Lifecycle: &union.LifecycleEvent{Kind: "execution_started"},
	}); err != nil {
		t.Fatal(err)
	}

	unknownIntentHeader := headerFor(2, union.EventFamilyIntent, union.EventOriginExternal)
	unknownIntentHeader.IntentID = "intent-injected"
	if err := ledger.Append(union.UnifiedExecutionEvent{Header: unknownIntentHeader, Intent: &union.IntentEvent{Kind: "accepted"}}); !errors.Is(err, execution.ErrLedgerInvariant) {
		t.Fatalf("injected intent error = %v", err)
	}

	knownIntentHeader := headerFor(2, union.EventFamilyIntent, union.EventOriginPraxis)
	knownIntentHeader.IntentID = "intent-1"
	if err := ledger.Append(union.UnifiedExecutionEvent{Header: knownIntentHeader, Intent: &union.IntentEvent{Kind: "accepted"}}); err != nil {
		t.Fatal(err)
	}
	adapterAcceptHeader := headerFor(3, union.EventFamilyIntent, union.EventOriginExternal)
	adapterAcceptHeader.IntentID = "intent-1"
	if err := ledger.Append(union.UnifiedExecutionEvent{Header: adapterAcceptHeader, Intent: &union.IntentEvent{Kind: "accepted"}}); !errors.Is(err, execution.ErrAdapterAuthority) {
		t.Fatalf("adapter intent authority error = %v", err)
	}

	injectedPlan := invocation.Plan.Mechanisms[0]
	injectedPlan.ID = "plan-injected"
	injectedHeader := headerFor(3, union.EventFamilyMechanism, union.EventOriginExternal)
	injectedHeader.IntentID, injectedHeader.MechanismPlanID = injectedPlan.IntentID, injectedPlan.ID
	if err := ledger.Append(union.UnifiedExecutionEvent{
		Header: injectedHeader, Mechanism: &union.MechanismEvent{Kind: "selected", Plan: &injectedPlan},
	}); !errors.Is(err, execution.ErrLedgerInvariant) {
		t.Fatalf("injected mechanism error = %v", err)
	}

	mutatedPlan := invocation.Plan.Mechanisms[0]
	mutatedPlan.Kind = "policy_bypass"
	mutatedHeader := headerFor(3, union.EventFamilyMechanism, union.EventOriginExternal)
	mutatedHeader.IntentID, mutatedHeader.MechanismPlanID = mutatedPlan.IntentID, mutatedPlan.ID
	if err := ledger.Append(union.UnifiedExecutionEvent{
		Header: mutatedHeader, Mechanism: &union.MechanismEvent{Kind: "selected", Plan: &mutatedPlan},
	}); !errors.Is(err, execution.ErrLedgerInvariant) {
		t.Fatalf("mutated mechanism error = %v", err)
	}

	planned := invocation.Plan.Mechanisms[0]
	plannedHeader := headerFor(3, union.EventFamilyMechanism, union.EventOriginPraxis)
	plannedHeader.IntentID, plannedHeader.MechanismPlanID = planned.IntentID, planned.ID
	if err := ledger.Append(union.UnifiedExecutionEvent{Header: plannedHeader, Mechanism: &union.MechanismEvent{Kind: "planned", Plan: &planned}}); err != nil {
		t.Fatal(err)
	}
	selectedHeader := headerFor(4, union.EventFamilyMechanism, union.EventOriginExternal)
	selectedHeader.IntentID, selectedHeader.MechanismPlanID = planned.IntentID, planned.ID
	if err := ledger.Append(union.UnifiedExecutionEvent{Header: selectedHeader, Mechanism: &union.MechanismEvent{Kind: "selected", Plan: &planned}}); err != nil {
		t.Fatalf("exact Adapter selection from sealed plan: %v", err)
	}
}

func TestPlanBoundLedgerRejectsCrossIntentPlanAttemptAndUnknownCausation(t *testing.T) {
	base := validInvocation("exec-ledger-bound-cross")
	request := base.Request
	secondIntent := request.IntentGraph.Nodes[0]
	secondIntent.ID = "intent-2"
	secondIntent.Target = "/workspace/other.txt"
	request.IntentGraph.Nodes = append(request.IntentGraph.Nodes, secondIntent)
	plan := base.Plan
	plan.IntentGraph = request.IntentGraph
	secondPlan := plan.Mechanisms[0]
	secondPlan.ID = "plan-2"
	secondPlan.IntentID = "intent-2"
	plan.Mechanisms = append(plan.Mechanisms, secondPlan)
	plan.Digest = ""
	delete(plan.Metadata, "request_digest")
	invocation, err := execution.NewInvocation(request, plan)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := execution.NewEventLedgerForPlan(invocation.Plan)
	if err != nil {
		t.Fatal(err)
	}
	headerFor := func(family union.EventFamily, origin union.EventOrigin) union.EventHeader {
		header := ledgerHeader(ledger.NextSequence(), family)
		header.ExecutionID = invocation.Request.ExecutionID
		header.Origin = origin
		return header
	}
	appendEvent := func(event union.UnifiedExecutionEvent) {
		t.Helper()
		if err := ledger.Append(event); err != nil {
			t.Fatalf("Append sequence %d: %v", event.Header.Sequence, err)
		}
	}
	appendEvent(union.UnifiedExecutionEvent{Header: headerFor(union.EventFamilyLifecycle, union.EventOriginPraxis), Lifecycle: &union.LifecycleEvent{Kind: "execution_started"}})
	for _, intentID := range []union.IntentID{"intent-1", "intent-2"} {
		header := headerFor(union.EventFamilyIntent, union.EventOriginPraxis)
		header.IntentID = intentID
		appendEvent(union.UnifiedExecutionEvent{Header: header, Intent: &union.IntentEvent{Kind: "accepted"}})
	}
	for index := range invocation.Plan.Mechanisms {
		mechanism := invocation.Plan.Mechanisms[index]
		header := headerFor(union.EventFamilyMechanism, union.EventOriginPraxis)
		header.IntentID, header.MechanismPlanID = mechanism.IntentID, mechanism.ID
		appendEvent(union.UnifiedExecutionEvent{Header: header, Mechanism: &union.MechanismEvent{Kind: "planned", Plan: &mechanism}})
		header = headerFor(union.EventFamilyMechanism, union.EventOriginPraxis)
		header.IntentID, header.MechanismPlanID = mechanism.IntentID, mechanism.ID
		appendEvent(union.UnifiedExecutionEvent{Header: header, Mechanism: &union.MechanismEvent{Kind: "selected", Plan: &mechanism}})
	}
	attempt := attemptCandidate(union.SideEffectNone, 1)
	attempt.Header = headerFor(union.EventFamilyMechanism, union.EventOriginExternal)
	attempt.Header.IntentID, attempt.Header.MechanismPlanID, attempt.Header.MechanismAttemptID = "intent-1", "plan-1", "attempt-1"
	appendEvent(attempt)

	tests := []struct {
		name   string
		header union.EventHeader
	}{
		{
			name: "known plan bound to wrong intent",
			header: func() union.EventHeader {
				header := headerFor(union.EventFamilyDiagnostic, union.EventOriginExternal)
				header.IntentID, header.MechanismPlanID = "intent-2", "plan-1"
				return header
			}(),
		},
		{
			name: "known attempt bound to wrong intent",
			header: func() union.EventHeader {
				header := headerFor(union.EventFamilyDiagnostic, union.EventOriginExternal)
				header.IntentID, header.MechanismAttemptID = "intent-2", "attempt-1"
				return header
			}(),
		},
		{
			name: "known attempt bound to wrong plan",
			header: func() union.EventHeader {
				header := headerFor(union.EventFamilyDiagnostic, union.EventOriginExternal)
				header.IntentID, header.MechanismPlanID, header.MechanismAttemptID = "intent-2", "plan-2", "attempt-1"
				return header
			}(),
		},
		{
			name: "unobserved attempt",
			header: func() union.EventHeader {
				header := headerFor(union.EventFamilyDiagnostic, union.EventOriginExternal)
				header.IntentID, header.MechanismPlanID, header.MechanismAttemptID = "intent-1", "plan-1", "attempt-unknown"
				return header
			}(),
		},
		{
			name: "unknown unified causation",
			header: func() union.EventHeader {
				header := headerFor(union.EventFamilyDiagnostic, union.EventOriginExternal)
				header.CausationID = "event-never-committed"
				return header
			}(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := union.UnifiedExecutionEvent{Header: test.header, Diagnostic: &union.DiagnosticEvent{Kind: "binding_probe"}}
			if err := ledger.Append(event); !errors.Is(err, execution.ErrLedgerInvariant) {
				t.Fatalf("Append() error = %v", err)
			}
		})
	}
	validCausation := headerFor(union.EventFamilyDiagnostic, union.EventOriginExternal)
	validCausation.CausationID = "event-1"
	appendEvent(union.UnifiedExecutionEvent{Header: validCausation, Diagnostic: &union.DiagnosticEvent{Kind: "binding_probe"}})
}

func TestLedgerApprovalRevisionExpiryAndIdempotency(t *testing.T) {
	ledger, _ := execution.NewEventLedger("exec-ledger")
	appendCoreLedger(t, ledger, union.SideEffectNone)
	expires := fixtureTime.Add(time.Hour)
	zeroRevision := approvalEvent(ledger.NextSequence(), "approval-zero", "action-zero", "digest-zero", 0, expires)
	if err := ledger.Append(zeroRevision); !errors.Is(err, execution.ErrLedgerInvariant) {
		t.Fatalf("zero approval revision error = %v", err)
	}
	request := approvalEvent(ledger.NextSequence(), "approval-1", "action-1", "digest-1", 1, expires)
	if err := ledger.Append(request); err != nil {
		t.Fatalf("approval request: %v", err)
	}
	staleRevision := approvalEvent(ledger.NextSequence(), "approval-1", "action-1", "digest-1", 2, expires)
	if err := ledger.Append(staleRevision); !errors.Is(err, execution.ErrLedgerInvariant) {
		t.Fatalf("unchanged input revision error = %v", err)
	}
	newRevision := approvalEvent(ledger.NextSequence(), "approval-1", "action-1", "digest-2", 2, expires)
	if err := ledger.Append(newRevision); err != nil {
		t.Fatalf("new approval revision: %v", err)
	}
	staleCommand := approvalCommand("approval-1", "action-1", "digest-1", 1, "approve-1")
	if _, err := ledger.CheckCommand(staleCommand, fixtureTime.Add(time.Minute)); !errors.Is(err, execution.ErrApprovalRevision) {
		t.Fatalf("stale command error = %v", err)
	}
	command := approvalCommand("approval-1", "action-1", "digest-2", 2, "approve-1")
	disposition, err := ledger.CheckCommand(command, fixtureTime.Add(time.Minute))
	if err != nil {
		t.Fatalf("CheckCommand: %v", err)
	}
	receipt, _ := json.Marshal(map[string]string{"command_digest": disposition.Digest})
	header := ledgerHeader(ledger.NextSequence(), union.EventFamilyControl)
	header.ApprovalID, header.ActionID, header.MechanismAttemptID = "approval-1", "action-1", "attempt-1"
	resolved := union.UnifiedExecutionEvent{Header: header, Control: &union.ControlEvent{
		Kind: execution.ControlApprovalResolved, ApprovalID: "approval-1", ActionID: "action-1",
		MechanismAttemptID: "attempt-1", InputDigest: "digest-2", ActionRevision: 2,
		IdempotencyKey: "approve-1", Decision: "approve", Payload: receipt,
	}}
	if err := ledger.Append(resolved); err != nil {
		t.Fatalf("approval resolution: %v", err)
	}
	duplicate, err := ledger.CheckCommand(command, fixtureTime.Add(2*time.Minute))
	if err != nil || !duplicate.Duplicate {
		t.Fatalf("idempotent replay = %#v, %v", duplicate, err)
	}
	conflict := command
	conflict.Payload = json.RawMessage(`{"changed":true}`)
	if _, err := ledger.CheckCommand(conflict, fixtureTime.Add(2*time.Minute)); !errors.Is(err, execution.ErrIdempotencyConflict) {
		t.Fatalf("idempotency conflict error = %v", err)
	}

	expiring := approvalEvent(ledger.NextSequence(), "approval-2", "action-2", "digest-x", 1, fixtureTime.Add(10*time.Minute))
	if err := ledger.Append(expiring); err != nil {
		t.Fatalf("expiring approval: %v", err)
	}
	expiredCommand := approvalCommand("approval-2", "action-2", "digest-x", 1, "approve-2")
	disposition, err = ledger.CheckCommand(expiredCommand, fixtureTime.Add(10*time.Minute))
	if !errors.Is(err, execution.ErrApprovalExpired) || !disposition.ApprovalExpired {
		t.Fatalf("expired approval disposition = %#v, %v", disposition, err)
	}
}

func TestLedgerCancelChainAndUnknownSideEffect(t *testing.T) {
	ledger, _ := execution.NewEventLedger("exec-ledger")
	appendCoreLedger(t, ledger, union.SideEffectUnknown)
	if err := ledger.Append(approvalEvent(ledger.NextSequence(), "approval-cancel", "action-cancel", "digest-cancel", 1, fixtureTime.Add(time.Hour))); err != nil {
		t.Fatalf("pending approval: %v", err)
	}
	appendControlWithReceipt(t, ledger, execution.ControlCancelRequested, "cancel-1")
	if got := ledger.State().Approvals["approval-cancel"].Status; got != execution.ApprovalInvalidated {
		t.Fatalf("approval status after cancel = %q", got)
	}
	appendControl(t, ledger, execution.ControlCancelDispatched, nil)
	if err := ledger.Append(controlLedgerEvent(ledger.NextSequence(), execution.ControlCancellationQuiesced, nil)); !errors.Is(err, execution.ErrCancelState) {
		t.Fatalf("quiesced before ack error = %v", err)
	}
	appendControl(t, ledger, execution.ControlCancelAcknowledged, nil)
	appendControl(t, ledger, execution.ControlCancellationQuiesced, nil)
	appendControl(t, ledger, execution.ControlReconciliationStarted, nil)
	summary, _ := json.Marshal(execution.ReconciliationSummary{SideEffectState: union.SideEffectUnknown, Quiesced: true})
	appendControl(t, ledger, execution.ControlReconciliationComplete, summary)
	appendRouteCandidate(t, ledger, union.ExecutionStatusCancelled, union.SideEffectUnknown)
	cancelled := union.UnifiedExecutionEvent{
		Header:    ledgerHeader(ledger.NextSequence(), union.EventFamilyLifecycle),
		Lifecycle: &union.LifecycleEvent{Kind: "execution_cancelled", Status: union.ExecutionStatusCancelled},
	}
	if err := ledger.Append(cancelled); !errors.Is(err, execution.ErrLedgerInvariant) {
		t.Fatalf("cancelled with unknown side effect error = %v", err)
	}
	indeterminate := cancelled
	indeterminate.Header = ledgerHeader(ledger.NextSequence(), union.EventFamilyLifecycle)
	indeterminate.Lifecycle = &union.LifecycleEvent{Kind: "execution_indeterminate", Status: union.ExecutionStatusIndeterminate}
	if err := ledger.Append(indeterminate); err != nil {
		t.Fatalf("indeterminate terminal: %v", err)
	}
}

func TestLedgerRejectsEffectForSyntheticAttempt(t *testing.T) {
	ledger, _ := execution.NewEventLedger("exec-ledger")
	appendCoreLedger(t, ledger, union.SideEffectNone)
	synthetic := syntheticCandidate()
	synthetic.Header = ledgerHeader(ledger.NextSequence(), union.EventFamilyModel)
	synthetic.Header.IntentID, synthetic.Header.MechanismAttemptID, synthetic.Header.ActionID = "intent-1", "attempt-1", "action-1"
	synthetic.Header.ItemID = "item-1"
	if err := ledger.Append(synthetic); err != nil {
		t.Fatalf("synthetic result: %v", err)
	}
	observed := observedEffect(fixtureTime.Add(time.Minute))
	header := ledgerHeader(ledger.NextSequence(), union.EventFamilyEffect)
	header.IntentID, header.MechanismAttemptID, header.ActionID, header.EffectID = "intent-1", "attempt-1", "action-1", "effect-1"
	event := union.UnifiedExecutionEvent{Header: header, Effect: &union.EffectEvent{Kind: "observed", Effect: &observed}}
	if err := ledger.Append(event); !errors.Is(err, execution.ErrLedgerInvariant) {
		t.Fatalf("synthetic Effect error = %v", err)
	}
}

func TestLedgerRequiresCausalAndAcceptedEffectVerificationAssociations(t *testing.T) {
	ledger, _ := execution.NewEventLedger("exec-ledger")
	appendCoreLedger(t, ledger, union.SideEffectNone)

	unaccepted := observedEffect(fixtureTime.Add(time.Minute))
	unaccepted.IntentIDs = append(unaccepted.IntentIDs, "intent-other")
	header := ledgerHeader(ledger.NextSequence(), union.EventFamilyEffect)
	header.IntentID, header.MechanismAttemptID, header.EffectID = "intent-1", "attempt-1", unaccepted.ID
	if err := ledger.Append(union.UnifiedExecutionEvent{Header: header, Effect: &union.EffectEvent{Kind: "observed", Effect: &unaccepted}}); !errors.Is(err, execution.ErrLedgerInvariant) {
		t.Fatalf("unaccepted Effect intent error = %v", err)
	}

	forward := observedEffect(fixtureTime.Add(time.Minute))
	forward.SupersedesEffectIDs = []union.EffectID{"effect-later"}
	header = ledgerHeader(ledger.NextSequence(), union.EventFamilyEffect)
	header.IntentID, header.MechanismAttemptID, header.EffectID = "intent-1", "attempt-1", forward.ID
	if err := ledger.Append(union.UnifiedExecutionEvent{Header: header, Effect: &union.EffectEvent{Kind: "observed", Effect: &forward}}); !errors.Is(err, execution.ErrLedgerInvariant) {
		t.Fatalf("forward Effect supersession error = %v", err)
	}

	first := observedEffect(fixtureTime.Add(time.Minute))
	header = ledgerHeader(ledger.NextSequence(), union.EventFamilyEffect)
	header.IntentID, header.MechanismAttemptID, header.EffectID = "intent-1", "attempt-1", first.ID
	if err := ledger.Append(union.UnifiedExecutionEvent{Header: header, Effect: &union.EffectEvent{Kind: "observed", Effect: &first}}); err != nil {
		t.Fatalf("first Effect: %v", err)
	}
	second := observedEffect(fixtureTime.Add(2 * time.Minute))
	second.ID = "effect-2"
	second.SupersedesEffectIDs = []union.EffectID{first.ID}
	header = ledgerHeader(ledger.NextSequence(), union.EventFamilyEffect)
	header.IntentID, header.MechanismAttemptID, header.EffectID = "intent-1", "attempt-1", second.ID
	if err := ledger.Append(union.UnifiedExecutionEvent{Header: header, Effect: &union.EffectEvent{Kind: "observed", Effect: &second}}); err != nil {
		t.Fatalf("causal Effect supersession: %v", err)
	}

	verification := verifiedRecord(fixtureTime.Add(3 * time.Minute))
	verification.EffectIDs = []union.EffectID{second.ID}
	verification.IntentIDs = []union.IntentID{"intent-other"}
	header = ledgerHeader(ledger.NextSequence(), union.EventFamilyEffect)
	header.VerificationID, header.EffectID, header.IntentID = verification.ID, second.ID, "intent-other"
	if err := ledger.Append(union.UnifiedExecutionEvent{Header: header, Effect: &union.EffectEvent{Kind: "verified", Verification: &verification}}); !errors.Is(err, execution.ErrLedgerInvariant) {
		t.Fatalf("unaccepted verification intent error = %v", err)
	}
}

func TestLedgerRejectsAttemptAndItemStateRegression(t *testing.T) {
	ledger, _ := execution.NewEventLedger("exec-ledger")
	appendCoreLedger(t, ledger, union.SideEffectPossible)
	completed := attemptStateEvent(ledger.NextSequence(), union.AttemptStatusCompleted, union.SideEffectObserved)
	if err := ledger.Append(completed); err != nil {
		t.Fatalf("complete attempt: %v", err)
	}
	regressed := attemptStateEvent(ledger.NextSequence(), union.AttemptStatusRunning, union.SideEffectNone)
	if err := ledger.Append(regressed); !errors.Is(err, execution.ErrLedgerInvariant) {
		t.Fatalf("attempt regression error = %v", err)
	}
	pending := itemStateEvent(ledger.NextSequence(), union.ItemStatusPending, union.SideEffectPossible)
	if err := ledger.Append(pending); err != nil {
		t.Fatalf("pending item: %v", err)
	}
	completedItem := itemStateEvent(ledger.NextSequence(), union.ItemStatusCompleted, union.SideEffectObserved)
	if err := ledger.Append(completedItem); err != nil {
		t.Fatalf("complete item: %v", err)
	}
	late := itemStateEvent(ledger.NextSequence(), union.ItemStatusInProgress, union.SideEffectNone)
	if err := ledger.Append(late); !errors.Is(err, execution.ErrLedgerInvariant) {
		t.Fatalf("item regression error = %v", err)
	}
}

func attemptStateEvent(sequence uint64, status union.AttemptStatus, sideEffects union.SideEffectState) union.UnifiedExecutionEvent {
	header := ledgerHeader(sequence, union.EventFamilyMechanism)
	header.IntentID, header.MechanismPlanID, header.MechanismAttemptID = "intent-1", "plan-1", "attempt-1"
	return union.UnifiedExecutionEvent{Header: header, Mechanism: &union.MechanismEvent{Kind: "attempt_" + string(status), Attempt: &union.MechanismAttempt{
		ID: "attempt-1", MechanismPlanID: "plan-1", Authoritative: true, ActualKind: "caller_tool",
		ActualOrigin: union.CapabilityOriginCallerHosted, ActualOwner: union.ExecutionOwnerPraxis,
		Status: status, SideEffectState: sideEffects,
	}}}
}

func itemStateEvent(sequence uint64, status union.ItemStatus, sideEffects union.SideEffectState) union.UnifiedExecutionEvent {
	header := ledgerHeader(sequence, union.EventFamilyItem)
	header.IntentID, header.MechanismPlanID, header.MechanismAttemptID = "intent-1", "plan-1", "attempt-1"
	header.ItemID, header.ActionID = "item-1", "action-1"
	return union.UnifiedExecutionEvent{Header: header, Item: &union.ItemEvent{Kind: "tool_execution", Item: union.ExecutionItem{
		ID: "item-1", Kind: "tool", Status: status, ActionID: "action-1", AttemptID: "attempt-1", SideEffectState: sideEffects,
	}}}
}

func approvalEvent(sequence uint64, approvalID union.ApprovalID, actionID union.ActionID, digest string, revision uint64, expires time.Time) union.UnifiedExecutionEvent {
	header := ledgerHeader(sequence, union.EventFamilyControl)
	header.ApprovalID, header.ActionID, header.MechanismAttemptID = approvalID, actionID, "attempt-1"
	return union.UnifiedExecutionEvent{Header: header, Control: &union.ControlEvent{
		Kind: execution.ControlApprovalRequested, ApprovalID: approvalID, ActionID: actionID,
		MechanismAttemptID: "attempt-1", InputDigest: digest, ActionRevision: revision, ExpiresAt: expires,
	}}
}

func approvalCommand(approvalID union.ApprovalID, actionID union.ActionID, digest string, revision uint64, key string) union.ExecutionCommand {
	return union.ExecutionCommand{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: "exec-ledger", Kind: union.CommandApproveAction,
		ExpectedExecutionStatus: "running", IdempotencyKey: key, ApprovalID: approvalID, ActionID: actionID,
		MechanismAttemptID: "attempt-1", InputDigest: digest, ActionRevision: revision,
	}
}

func appendReconciliation(t *testing.T, ledger *execution.EventLedger, sideEffects union.SideEffectState) {
	t.Helper()
	appendControl(t, ledger, execution.ControlReconciliationStarted, nil)
	payload, _ := json.Marshal(execution.ReconciliationSummary{SideEffectState: sideEffects, Quiesced: true})
	appendControl(t, ledger, execution.ControlReconciliationComplete, payload)
}

func appendRouteCandidate(t *testing.T, ledger *execution.EventLedger, status union.ExecutionStatus, sideEffects union.SideEffectState) {
	t.Helper()
	payload, _ := json.Marshal(execution.RouteTerminalCandidate{Status: status, SideEffectState: sideEffects})
	event := union.UnifiedExecutionEvent{
		Header:     ledgerHeader(ledger.NextSequence(), union.EventFamilyDiagnostic),
		Diagnostic: &union.DiagnosticEvent{Kind: execution.EventKindRouteTerminalCandidate, Payload: payload},
	}
	if err := ledger.Append(event); err != nil {
		t.Fatalf("route terminal candidate: %v", err)
	}
}

func appendControlWithReceipt(t *testing.T, ledger *execution.EventLedger, kind, idempotency string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"command_digest": "sha256:command"})
	event := controlLedgerEvent(ledger.NextSequence(), kind, payload)
	event.Control.IdempotencyKey = idempotency
	if err := ledger.Append(event); err != nil {
		t.Fatalf("control %s: %v", kind, err)
	}
}

func appendControl(t *testing.T, ledger *execution.EventLedger, kind string, payload json.RawMessage) {
	t.Helper()
	if err := ledger.Append(controlLedgerEvent(ledger.NextSequence(), kind, payload)); err != nil {
		t.Fatalf("control %s: %v", kind, err)
	}
}

func controlLedgerEvent(sequence uint64, kind string, payload json.RawMessage) union.UnifiedExecutionEvent {
	return union.UnifiedExecutionEvent{
		Header: ledgerHeader(sequence, union.EventFamilyControl), Control: &union.ControlEvent{Kind: kind, Payload: payload},
	}
}
