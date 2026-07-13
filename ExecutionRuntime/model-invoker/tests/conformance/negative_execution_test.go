package conformance_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func negativeInvocation(executionID union.ExecutionID) execution.Invocation {
	profileID := union.VersionedIdentity{ID: "profile-negative", Version: "v1"}
	routeID := union.VersionedIdentity{ID: "route-negative", Version: "v1"}
	graph := union.IntentGraph{Nodes: []union.IntentNode{{
		ID: "intent-negative", Kind: union.IntentModifyFile, Target: "/workspace/file.txt", Required: true,
	}}}
	request := union.UnifiedExecutionRequest{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: executionID,
		ProfileSelector: union.ProfileSelector{Exact: &profileID}, ExecutionKind: union.ExecutionKindModel,
		SessionIntent:     union.SessionIntent{Mode: "new", SessionID: "session-negative", TurnID: "turn-negative"},
		DegradationPolicy: union.DegradationPolicy{Default: union.DegradationDefaultReject}, IntentGraph: graph,
	}
	plan := union.PreparedExecutionPlan{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: executionID, Profile: profileID, Route: routeID,
		ProfileKeyDigest: "sha256:profile-negative", ExecutionKind: union.ExecutionKindModel, IntentGraph: graph,
		Mechanisms: []union.MechanismPlan{{
			ID: "plan-negative", IntentID: "intent-negative", Kind: "harness_tool",
			Origin: union.CapabilityOriginHarnessHosted, Owner: union.ExecutionOwnerHarness,
			SelectionAuthority: union.SelectionAuthorityRuntime, SemanticFidelity: union.SemanticFidelityExact,
		}},
		ExpectedManifest: union.ContextManifestSummary{ID: "manifest-negative", Version: "v1", Mode: "semantic_stable"},
		RouteFingerprint: "sha256:route-negative",
	}
	invocation, err := execution.NewInvocation(request, plan)
	if err != nil {
		panic(err)
	}
	return invocation
}

func negativeLedgerHeader(sequence uint64, family union.EventFamily, origin union.EventOrigin) union.EventHeader {
	return union.EventHeader{
		EventID: union.EventID("negative-event-" + time.Duration(sequence).String()), SemanticVersion: union.SemanticVersionV1,
		ExecutionID: "exec-negative-ledger", Sequence: sequence, Timestamp: negativeTestTime.Add(time.Duration(sequence) * time.Second),
		Origin: origin, Family: family, Visibility: union.VisibilityAuditOnly, SecurityClassification: union.SecurityInternal,
		ExecutionKind: union.ExecutionKindModel, Profile: union.VersionedIdentity{ID: "profile-negative", Version: "v1"},
		Route: union.VersionedIdentity{ID: "route-negative", Version: "v1"},
	}
}

func appendNegativePreamble(t *testing.T, ledger *execution.EventLedger, sideEffects union.SideEffectState) {
	t.Helper()
	plan := negativeInvocation("exec-negative-ledger").Plan.Mechanisms[0]
	intentHeader := negativeLedgerHeader(1, union.EventFamilyIntent, union.EventOriginPraxis)
	intentHeader.IntentID = "intent-negative"
	plannedHeader := negativeLedgerHeader(2, union.EventFamilyMechanism, union.EventOriginPraxis)
	plannedHeader.IntentID, plannedHeader.MechanismPlanID = "intent-negative", "plan-negative"
	selectedHeader := negativeLedgerHeader(3, union.EventFamilyMechanism, union.EventOriginPraxis)
	selectedHeader.IntentID, selectedHeader.MechanismPlanID = "intent-negative", "plan-negative"
	attemptHeader := negativeLedgerHeader(4, union.EventFamilyMechanism, union.EventOriginHarness)
	attemptHeader.IntentID, attemptHeader.MechanismPlanID, attemptHeader.MechanismAttemptID = "intent-negative", "plan-negative", "attempt-negative"
	events := []union.UnifiedExecutionEvent{
		{Header: intentHeader, Intent: &union.IntentEvent{Kind: "accepted"}},
		{Header: plannedHeader, Mechanism: &union.MechanismEvent{Kind: "planned", Plan: &plan}},
		{Header: selectedHeader, Mechanism: &union.MechanismEvent{Kind: "selected", Plan: &plan}},
		{Header: attemptHeader, Mechanism: &union.MechanismEvent{Kind: "attempt_started", Attempt: &union.MechanismAttempt{
			ID: "attempt-negative", MechanismPlanID: "plan-negative", Authoritative: true, ActualKind: "harness_tool",
			ActualOrigin: union.CapabilityOriginHarnessHosted, ActualOwner: union.ExecutionOwnerHarness,
			Status: union.AttemptStatusRunning, SideEffectState: sideEffects,
		}}},
	}
	for _, event := range events {
		if err := ledger.Append(event); err != nil {
			t.Fatalf("append negative preamble event %d: %v", event.Header.Sequence, err)
		}
	}
}

func negativeApprovalEvent(sequence uint64, digest string, revision uint64) union.UnifiedExecutionEvent {
	header := negativeLedgerHeader(sequence, union.EventFamilyControl, union.EventOriginHarness)
	header.ApprovalID, header.ActionID, header.MechanismAttemptID = "approval-negative", "action-negative", "attempt-negative"
	return union.UnifiedExecutionEvent{Header: header, Control: &union.ControlEvent{
		Kind: execution.ControlApprovalRequested, ApprovalID: "approval-negative", ActionID: "action-negative",
		MechanismAttemptID: "attempt-negative", InputDigest: digest, ActionRevision: revision,
		ExpiresAt: negativeTestTime.Add(time.Hour),
	}}
}

func TestN04ApprovalRevisionChangeInvalidatesOldCommand(t *testing.T) {
	ledger, err := execution.NewEventLedger("exec-negative-ledger")
	if err != nil {
		t.Fatal(err)
	}
	appendNegativePreamble(t, ledger, union.SideEffectNone)
	if err := ledger.Append(negativeApprovalEvent(ledger.NextSequence(), "sha256:input-revision-1", 1)); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Append(negativeApprovalEvent(ledger.NextSequence(), "sha256:input-revision-2", 2)); err != nil {
		t.Fatal(err)
	}
	oldCommand := union.ExecutionCommand{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: "exec-negative-ledger", Kind: union.CommandApproveAction,
		ExpectedExecutionStatus: "running", IdempotencyKey: "approve-negative-old", ApprovalID: "approval-negative",
		ActionID: "action-negative", MechanismAttemptID: "attempt-negative", InputDigest: "sha256:input-revision-1", ActionRevision: 1,
	}
	if _, err := ledger.CheckCommand(oldCommand, negativeTestTime.Add(time.Minute)); !errors.Is(err, execution.ErrApprovalRevision) {
		t.Fatalf("old approval command error = %v, want revision mismatch", err)
	}
	currentCommand := oldCommand
	currentCommand.IdempotencyKey = "approve-negative-current"
	currentCommand.InputDigest = "sha256:input-revision-2"
	currentCommand.ActionRevision = 2
	if _, err := ledger.CheckCommand(currentCommand, negativeTestTime.Add(time.Minute)); err != nil {
		t.Fatalf("current approval command rejected: %v", err)
	}
}

func TestN05ActionThenDisconnectReconcilesToIndeterminate(t *testing.T) {
	session := &negativeSession{
		events: []union.UnifiedExecutionEvent{
			negativeAttemptCandidate(union.SideEffectPossible),
			negativeActionCompletedCandidate(),
		},
		terminalErr: io.ErrUnexpectedEOF,
	}
	var reconciled atomic.Bool
	runtime := negativeRuntime(t, session, negativeReconciler(func(_ context.Context, input execution.ReconcileInput) (execution.ReconcileReport, error) {
		reconciled.Store(true)
		if !input.State.HasUncertainSideEffects() {
			t.Fatal("disconnect reconciler did not receive the possible side effect")
		}
		return execution.ReconcileReport{SideEffectState: union.SideEffectUnknown, Quiesced: false}, nil
	}), nil)

	result, err := runtime.Execute(context.Background(), "negative", negativeInvocation("exec-N05"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !reconciled.Load() || result.Status != union.ExecutionStatusIndeterminate || len(result.Effects) != 0 {
		t.Fatalf("disconnect result reconciled=%v status=%q effects=%d", reconciled.Load(), result.Status, len(result.Effects))
	}
}

func TestN07CancelAcknowledgedWithoutQuiescenceCannotBeCancelled(t *testing.T) {
	ledger, err := execution.NewEventLedger("exec-negative-ledger")
	if err != nil {
		t.Fatal(err)
	}
	appendNegativePreamble(t, ledger, union.SideEffectNone)
	receipt, _ := json.Marshal(map[string]string{"command_digest": "sha256:cancel-command"})
	requested := union.UnifiedExecutionEvent{
		Header:  negativeLedgerHeader(ledger.NextSequence(), union.EventFamilyControl, union.EventOriginPraxis),
		Control: &union.ControlEvent{Kind: execution.ControlCancelRequested, IdempotencyKey: "cancel-negative", Payload: receipt},
	}
	if err := ledger.Append(requested); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{execution.ControlCancelDispatched, execution.ControlCancelAcknowledged} {
		origin := union.EventOriginPraxis
		if kind == execution.ControlCancelAcknowledged {
			origin = union.EventOriginHarness
		}
		event := union.UnifiedExecutionEvent{
			Header:  negativeLedgerHeader(ledger.NextSequence(), union.EventFamilyControl, origin),
			Control: &union.ControlEvent{Kind: kind},
		}
		if err := ledger.Append(event); err != nil {
			t.Fatalf("append %s: %v", kind, err)
		}
	}
	payload, _ := json.Marshal(execution.RouteTerminalCandidate{
		Status: union.ExecutionStatusCancelled, StopReason: "cancel_ack_only", SideEffectState: union.SideEffectNone,
	})
	if err := ledger.Append(union.UnifiedExecutionEvent{
		Header:     negativeLedgerHeader(ledger.NextSequence(), union.EventFamilyDiagnostic, union.EventOriginPraxis),
		Diagnostic: &union.DiagnosticEvent{Kind: execution.EventKindRouteTerminalCandidate, Payload: payload},
	}); err != nil {
		t.Fatal(err)
	}
	cancelled := union.UnifiedExecutionEvent{
		Header:    negativeLedgerHeader(ledger.NextSequence(), union.EventFamilyLifecycle, union.EventOriginPraxis),
		Lifecycle: &union.LifecycleEvent{Kind: "execution_cancelled", Status: union.ExecutionStatusCancelled},
	}
	if err := ledger.Append(cancelled); !errors.Is(err, execution.ErrLedgerInvariant) {
		t.Fatalf("cancelled terminal error = %v, want missing quiescence/reconciliation rejection", err)
	}
	if ledger.State().Terminal {
		t.Fatal("cancel acknowledgement alone committed a unified terminal")
	}
}

func TestN10SyntheticToolResultCannotProduceEffect(t *testing.T) {
	ledger, err := execution.NewEventLedger("exec-negative-ledger")
	if err != nil {
		t.Fatal(err)
	}
	appendNegativePreamble(t, ledger, union.SideEffectNone)
	executed := false
	syntheticHeader := negativeLedgerHeader(ledger.NextSequence(), union.EventFamilyModel, union.EventOriginHarness)
	syntheticHeader.IntentID, syntheticHeader.MechanismAttemptID, syntheticHeader.ActionID = "intent-negative", "attempt-negative", "action-negative"
	syntheticHeader.ItemID = "item-negative"
	if err := ledger.Append(union.UnifiedExecutionEvent{
		Header: syntheticHeader,
		Model: &union.ModelEvent{
			Kind: "tool_result", ActionID: "action-negative", ExecutionItemID: "item-negative", Executed: &executed,
			SyntheticReason: "protocol_pairing_only", Payload: json.RawMessage(`{"status":"skipped"}`),
		},
	}); err != nil {
		t.Fatal(err)
	}
	observed := negativeObservedEffect("effect-N10")
	effectHeader := negativeLedgerHeader(ledger.NextSequence(), union.EventFamilyEffect, union.EventOriginPraxis)
	effectHeader.IntentID, effectHeader.MechanismAttemptID, effectHeader.ActionID, effectHeader.EffectID = "intent-negative", "attempt-negative", "action-negative", observed.ID
	if err := ledger.Append(union.UnifiedExecutionEvent{
		Header: effectHeader, Effect: &union.EffectEvent{Kind: "observed", Effect: &observed},
	}); !errors.Is(err, execution.ErrLedgerInvariant) {
		t.Fatalf("synthetic Effect error = %v", err)
	}
	if len(ledger.State().EffectIDs) != 0 {
		t.Fatalf("synthetic result committed Effect IDs: %#v", ledger.State().EffectIDs)
	}
}

func TestN12EndTurnWithContradictedVerifierCannotSucceed(t *testing.T) {
	session := &negativeSession{events: []union.UnifiedExecutionEvent{
		negativeAttemptCandidate(union.SideEffectPossible),
		negativeTerminalCandidate(union.ExecutionStatusSucceeded, "end_turn"),
	}}
	observed := negativeObservedEffect("effect-N12")
	reconciler := negativeReconciler(func(context.Context, execution.ReconcileInput) (execution.ReconcileReport, error) {
		return execution.ReconcileReport{Effects: []union.EffectRecord{observed}, SideEffectState: union.SideEffectObserved, Quiesced: true}, nil
	})
	verifier := negativeVerifier(func(context.Context, execution.VerifyInput) (execution.VerificationReport, error) {
		return execution.VerificationReport{Verifications: []union.VerificationRecord{{
			ID: "verification-N12", EffectIDs: []union.EffectID{observed.ID}, IntentIDs: []union.IntentID{"intent-negative"},
			Kind: "required_workspace_postcondition", Status: union.VerificationContradicted,
			Verifier: union.VersionedIdentity{ID: "verifier-negative", Version: "v1"}, FailureCode: "postcondition_failed",
			CompletedAt: negativeTestTime.Add(time.Minute),
		}}}, nil
	})
	runtime := negativeRuntime(t, session, reconciler, verifier)

	result, err := runtime.Execute(context.Background(), "negative", negativeInvocation("exec-N12"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != union.ExecutionStatusFailed || result.VerificationStatus != union.VerificationContradicted || result.StopReason != "end_turn" {
		t.Fatalf("end_turn result = status=%q verification=%q reason=%q", result.Status, result.VerificationStatus, result.StopReason)
	}
}

func negativeAttemptCandidate(sideEffects union.SideEffectState) union.UnifiedExecutionEvent {
	header := execution.CandidateHeader(union.EventOriginHarness, union.EventFamilyMechanism)
	header.IntentID, header.MechanismPlanID, header.MechanismAttemptID = "intent-negative", "plan-negative", "attempt-negative"
	return union.UnifiedExecutionEvent{Header: header, Mechanism: &union.MechanismEvent{Kind: "attempt_started", Attempt: &union.MechanismAttempt{
		ID: "attempt-negative", MechanismPlanID: "plan-negative", Authoritative: true, ActualKind: "harness_tool",
		ActualOrigin: union.CapabilityOriginHarnessHosted, ActualOwner: union.ExecutionOwnerHarness,
		Status: union.AttemptStatusRunning, SideEffectState: sideEffects,
	}}}
}

func negativeTerminalCandidate(status union.ExecutionStatus, reason string) union.UnifiedExecutionEvent {
	return union.UnifiedExecutionEvent{
		Header:    execution.CandidateHeader(union.EventOriginHarness, union.EventFamilyLifecycle),
		Lifecycle: &union.LifecycleEvent{Kind: "route_terminal", Status: status, StopReason: reason},
	}
}

func negativeActionCompletedCandidate() union.UnifiedExecutionEvent {
	header := execution.CandidateHeader(union.EventOriginHarness, union.EventFamilyItem)
	header.IntentID, header.MechanismPlanID, header.MechanismAttemptID = "intent-negative", "plan-negative", "attempt-negative"
	header.ItemID, header.ActionID = "item-negative", "action-negative"
	return union.UnifiedExecutionEvent{Header: header, Item: &union.ItemEvent{
		Kind: "action_completed",
		Item: union.ExecutionItem{
			ID: "item-negative", Kind: "file_change", Status: union.ItemStatusCompleted,
			ActionID: "action-negative", AttemptID: "attempt-negative", SideEffectState: union.SideEffectPossible,
		},
	}}
}

func negativeObservedEffect(id union.EffectID) union.EffectRecord {
	path := "/workspace/file.txt"
	return union.EffectRecord{
		ID: id, IntentIDs: []union.IntentID{"intent-negative"}, MechanismAttemptID: "attempt-negative",
		Kind: "file_changed", Target: path, Payload: union.EffectPayload{WorkspaceChange: &union.WorkspaceChange{
			Kind: "file_changed", Path: path,
			Before: &union.FileStateSnapshot{Path: path, Exists: true, Type: union.FileStateRegular, Hash: "sha256:before", Size: 6},
			After:  &union.FileStateSnapshot{Path: path, Exists: true, Type: union.FileStateRegular, Hash: "sha256:after", Size: 5},
		}},
		ObservationSource: "negative_fixture_observer", VerificationStatus: union.VerificationUnverified,
		OccurredAt: negativeTestTime.Add(30 * time.Second),
	}
}

type negativeSession struct {
	mu          sync.Mutex
	events      []union.UnifiedExecutionEvent
	index       int
	terminalErr error
}

func (session *negativeSession) Receive(context.Context) (union.UnifiedExecutionEvent, error) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.index < len(session.events) {
		event := session.events[session.index]
		session.index++
		return event, nil
	}
	if session.terminalErr != nil {
		return union.UnifiedExecutionEvent{}, session.terminalErr
	}
	return union.UnifiedExecutionEvent{}, io.EOF
}

func (*negativeSession) Command(context.Context, union.ExecutionCommand) error { return nil }
func (*negativeSession) Close() error                                          { return nil }

type negativeAdapter struct{ session execution.Session }

func (*negativeAdapter) Describe(context.Context) (execution.AdapterDescriptor, error) {
	return execution.AdapterDescriptor{
		Identity: union.VersionedIdentity{ID: "negative", Version: "v1"}, Origin: union.EventOriginHarness,
		ExecutionKinds: []union.ExecutionKind{union.ExecutionKindModel},
	}, nil
}

func (*negativeAdapter) Preflight(context.Context, execution.Invocation) (execution.PreflightReport, error) {
	return execution.PreflightReport{Accepted: true, ActualManifest: union.ContextManifestSummary{
		ID: "actual-negative", Version: "v1", Mode: "semantic_stable",
	}}, nil
}

func (adapter *negativeAdapter) Open(context.Context, execution.Invocation) (execution.Session, error) {
	return adapter.session, nil
}

type negativeReconciler func(context.Context, execution.ReconcileInput) (execution.ReconcileReport, error)

func (function negativeReconciler) Reconcile(ctx context.Context, input execution.ReconcileInput) (execution.ReconcileReport, error) {
	return function(ctx, input)
}

type negativeVerifier func(context.Context, execution.VerifyInput) (execution.VerificationReport, error)

func (function negativeVerifier) Verify(ctx context.Context, input execution.VerifyInput) (execution.VerificationReport, error) {
	return function(ctx, input)
}

func negativeRuntime(t *testing.T, session execution.Session, reconciler execution.Reconciler, verifier execution.Verifier) *execution.Runtime {
	t.Helper()
	registry := execution.NewRegistry()
	if err := registry.Register(context.Background(), &negativeAdapter{session: session}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	var tick atomic.Int64
	runtime, err := execution.NewRuntime(execution.RuntimeConfig{
		Registry: registry, Reconciler: reconciler, Verifier: verifier,
		Clock: func() time.Time { return negativeTestTime.Add(time.Duration(tick.Add(1)) * time.Millisecond) },
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	return runtime
}
