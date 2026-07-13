package executionunion_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestRuntimeOwnsEffectsVerificationAndTerminal(t *testing.T) {
	session := newFakeSession(4)
	session.events <- attemptCandidate(union.SideEffectPossible, 41)
	session.events <- terminalCandidate(union.ExecutionStatusSucceeded, 42)
	reconciler := reconcilerFunc(func(_ context.Context, input execution.ReconcileInput) (execution.ReconcileReport, error) {
		if !input.State.HasUncertainSideEffects() {
			t.Fatal("reconciler did not receive the possible side effect")
		}
		return execution.ReconcileReport{
			Effects:         []union.EffectRecord{observedEffect(fixtureTime.Add(time.Minute))},
			SideEffectState: union.SideEffectObserved, Quiesced: true,
		}, nil
	})
	verifier := verifierFunc(func(_ context.Context, input execution.VerifyInput) (execution.VerificationReport, error) {
		if len(input.Effects) != 1 {
			t.Fatalf("verifier effects = %d, want 1", len(input.Effects))
		}
		return execution.VerificationReport{Verifications: []union.VerificationRecord{verifiedRecord(fixtureTime.Add(2 * time.Minute))}}, nil
	})
	runtime := newTestRuntime(t, session, reconciler, verifier)
	invocation := validInvocation("exec-success")
	run, err := runtime.Start(context.Background(), "fake", invocation)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	result, err := run.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if result.Status != union.ExecutionStatusSucceeded || result.VerificationStatus != union.VerificationVerified {
		t.Fatalf("result status = %q/%q", result.Status, result.VerificationStatus)
	}
	if len(result.Effects) != 1 || len(result.Verifications) != 1 || len(result.IntentSatisfaction) != 1 {
		t.Fatalf("projected result counts = effects %d verifications %d satisfaction %d", len(result.Effects), len(result.Verifications), len(result.IntentSatisfaction))
	}
	if result.Effects[0].VerificationStatus != union.VerificationVerified || len(result.Effects[0].VerificationRefs) != 1 || result.Effects[0].VerificationRefs[0] != result.Verifications[0].ID {
		t.Fatalf("Effect/verification association = %#v / %#v", result.Effects[0], result.Verifications[0])
	}
	if result.IntentSatisfaction[0].Status != union.IntentSatisfied {
		t.Fatalf("intent satisfaction = %q", result.IntentSatisfaction[0].Status)
	}
	if result.SessionID != "session-1" || result.TurnID != "turn-1" || result.Digest == "" {
		t.Fatalf("result identity/digest was not projected: %#v", result)
	}
	replayed, err := (execution.Projector{}).Project(execution.ProjectionInput{
		Invocation: invocation, Events: run.Events(), ContextManifest: actualManifest(),
	})
	if err != nil {
		t.Fatalf("Project(replay): %v", err)
	}
	if replayed.Digest != result.Digest || replayed.TerminalEventID != result.TerminalEventID {
		initialJSON, _ := json.Marshal(result)
		replayJSON, _ := json.Marshal(replayed)
		t.Fatalf("replay drifted: initial %s replay %s", initialJSON, replayJSON)
	}
}

func TestRuntimeKeepsVerifiedTerminalDespiteCloseErrorAndClosesOnce(t *testing.T) {
	session := newFakeSession(4)
	session.closeErr = errors.New("release transport failed after terminal")
	session.events <- attemptCandidate(union.SideEffectPossible, 1)
	session.events <- terminalCandidate(union.ExecutionStatusSucceeded, 2)
	reconciler := reconcilerFunc(func(context.Context, execution.ReconcileInput) (execution.ReconcileReport, error) {
		return execution.ReconcileReport{Effects: []union.EffectRecord{observedEffect(fixtureTime.Add(time.Minute))}, SideEffectState: union.SideEffectObserved, Quiesced: true}, nil
	})
	verifier := verifierFunc(func(context.Context, execution.VerifyInput) (execution.VerificationReport, error) {
		return execution.VerificationReport{Verifications: []union.VerificationRecord{verifiedRecord(fixtureTime.Add(2 * time.Minute))}}, nil
	})
	result, err := newTestRuntime(t, session, reconciler, verifier).Execute(context.Background(), "fake", validInvocation("exec-close-error"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != union.ExecutionStatusSucceeded || session.closeCalls.Load() != 1 {
		t.Fatalf("status/close calls = %q/%d", result.Status, session.closeCalls.Load())
	}
	foundTransportResidual := false
	for _, residual := range result.Residuals {
		foundTransportResidual = foundTransportResidual || residual.Kind == "transport_error"
	}
	if !foundTransportResidual {
		t.Fatalf("close error did not remain auditable: %#v", result.Residuals)
	}
}

func TestRuntimeConcurrentWaitersReceiveIndependentResultClones(t *testing.T) {
	session := newFakeSession(4)
	session.events <- attemptCandidate(union.SideEffectNone, 1)
	session.events <- terminalCandidate(union.ExecutionStatusFailed, 2)
	run, err := newTestRuntime(t, session, nil, nil).Start(context.Background(), "fake", validInvocation("exec-waiters"))
	if err != nil {
		t.Fatal(err)
	}
	const waiters = 16
	results := make(chan union.UnifiedExecutionResult, waiters)
	errorsByWaiter := make(chan error, waiters)
	for range waiters {
		go func() {
			result, waitErr := run.Wait(context.Background())
			results <- result
			errorsByWaiter <- waitErr
		}()
	}
	var digest string
	for range waiters {
		result := <-results
		if waitErr := <-errorsByWaiter; waitErr != nil {
			t.Fatal(waitErr)
		}
		if result.Status != union.ExecutionStatusFailed || result.Digest == "" {
			t.Fatalf("wait result = %#v", result)
		}
		if digest == "" {
			digest = result.Digest
		} else if result.Digest != digest {
			t.Fatalf("waiter digest drift = %q vs %q", result.Digest, digest)
		}
		result.Residuals = append(result.Residuals, union.Residual{Path: "caller", Kind: "clone_mutation", Severity: "P3", Impact: "must remain local"})
	}
	final, err := run.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, residual := range final.Residuals {
		if residual.Kind == "clone_mutation" {
			t.Fatal("caller mutated the stored Runtime result")
		}
	}
}

func TestRuntimePreservesSourceOrderButOwnsGlobalOrder(t *testing.T) {
	session := newFakeSession(4)
	session.events <- attemptCandidate(union.SideEffectNone, 900)
	session.events <- terminalCandidate(union.ExecutionStatusFailed, 901)
	runtime := newTestRuntime(t, session, nil, nil)
	executionRun, err := runtime.Start(context.Background(), "fake", validInvocation("exec-sequence"))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := executionRun.Wait(context.Background()); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	events := executionRun.Events()
	terminalCount := 0
	foundSource := false
	for index, event := range events {
		if event.Header.Sequence != uint64(index+1) {
			t.Fatalf("global sequence[%d] = %d", index, event.Header.Sequence)
		}
		if event.Header.SourceSequence == 900 {
			foundSource = true
			if event.Header.EventID == "" || event.Header.Profile.ID != "profile-test" || event.Header.Route.ID != "route-test" {
				t.Fatalf("runtime header not authoritative: %#v", event.Header)
			}
		}
		if event.Lifecycle != nil && event.Lifecycle.Status != "" {
			terminalCount++
			if index != len(events)-1 || event.Header.Origin != union.EventOriginPraxis {
				t.Fatalf("terminal was not the final Praxis event: %#v", event)
			}
		}
	}
	if !foundSource || terminalCount != 1 {
		t.Fatalf("source preserved=%v terminal count=%d", foundSource, terminalCount)
	}
}

func TestProjectorAggregatesOutputDeltasAndUsageButExcludesReasoning(t *testing.T) {
	session := newFakeSession(8)
	session.events <- attemptCandidate(union.SideEffectPossible, 1)
	session.events <- modelCandidate("reasoning_delta", "secret analysis", "provider_exposed_reasoning", 2, nil)
	session.events <- modelCandidate("content_delta", "hel", "provider_exposed_output", 3, nil)
	session.events <- modelCandidate("content_delta", "lo", "provider_exposed_output", 4, nil)
	session.events <- modelCandidate("model_usage", "", "", 5, []union.UsageMetric{{
		Kind: "output_tokens", Value: 2, Unit: "tokens", Scope: "model_step", Source: "provider", Quality: "reported",
	}})
	session.events <- terminalCandidate(union.ExecutionStatusSucceeded, 6)
	reconciler := reconcilerFunc(func(context.Context, execution.ReconcileInput) (execution.ReconcileReport, error) {
		return execution.ReconcileReport{Effects: []union.EffectRecord{observedEffect(fixtureTime.Add(time.Minute))}, SideEffectState: union.SideEffectObserved, Quiesced: true}, nil
	})
	verifier := verifierFunc(func(context.Context, execution.VerifyInput) (execution.VerificationReport, error) {
		return execution.VerificationReport{Verifications: []union.VerificationRecord{verifiedRecord(fixtureTime.Add(2 * time.Minute))}}, nil
	})
	result, err := newTestRuntime(t, session, reconciler, verifier).Execute(context.Background(), "fake", validInvocation("exec-content-usage"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.FinalContent) != 1 || result.FinalContent[0].Text != "hello" {
		t.Fatalf("final content = %#v", result.FinalContent)
	}
	if len(result.UsageMetrics) != 1 || result.UsageMetrics[0].Kind != "output_tokens" {
		t.Fatalf("usage = %#v", result.UsageMetrics)
	}
}

func TestRuntimeRedactsCandidateCredentialFieldsAndKnownLiterals(t *testing.T) {
	session := newFakeSession(4)
	session.events <- attemptCandidate(union.SideEffectNone, 1)
	header := execution.CandidateHeader(union.EventOriginExternal, union.EventFamilyModel)
	header.Sequence, header.Timestamp = 2, fixtureTime.Add(2*time.Second)
	session.events <- union.UnifiedExecutionEvent{Header: header, Model: &union.ModelEvent{
		Kind: "content_completed", Content: []union.ContentPart{{Kind: "text", Text: "value=known-secret"}},
		Payload: json.RawMessage(`{"password":"plain-password","nested":{"apiKey":"known-secret","deeper":{"accessToken":"plain-access","client-secret":"plain-client"}},"note":"known-secret"}`),
	}}
	session.events <- terminalCandidate(union.ExecutionStatusFailed, 3)
	registry := execution.NewRegistry()
	if err := registry.Register(context.Background(), &fakeAdapter{session: session}); err != nil {
		t.Fatal(err)
	}
	redactor, err := execution.NewEventRedactor([]byte("known-secret"))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := execution.NewRuntime(execution.RuntimeConfig{Registry: registry, Sanitizer: redactor})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runtime.Start(context.Background(), "fake", validInvocation("exec-redacted"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	encodedEvents, _ := json.Marshal(run.Events())
	if strings.Contains(string(encodedEvents), "known-secret") || strings.Contains(string(encodedEvents), "plain-password") ||
		strings.Contains(string(encodedEvents), "plain-access") || strings.Contains(string(encodedEvents), "plain-client") ||
		!strings.Contains(string(encodedEvents), "[REDACTED]") {
		t.Fatalf("candidate event was not redacted: %s", encodedEvents)
	}
	if len(result.FinalContent) != 1 || result.FinalContent[0].Text != "value=[REDACTED]" {
		t.Fatalf("redacted final content = %#v", result.FinalContent)
	}
}

func modelCandidate(kind, text, disclosure string, sourceSequence uint64, usage []union.UsageMetric) union.UnifiedExecutionEvent {
	header := execution.CandidateHeader(union.EventOriginExternal, union.EventFamilyModel)
	header.Sequence = sourceSequence
	header.Timestamp = fixtureTime.Add(time.Duration(sourceSequence) * time.Second)
	content := []union.ContentPart(nil)
	if text != "" {
		content = []union.ContentPart{{Kind: "text", Text: text}}
	}
	return union.UnifiedExecutionEvent{Header: header, Model: &union.ModelEvent{Kind: kind, Content: content, DisclosureClass: disclosure, Usage: usage}}
}

func TestSyntheticResultCannotProduceEffect(t *testing.T) {
	session := newFakeSession(5)
	session.events <- attemptCandidate(union.SideEffectNone, 1)
	session.events <- syntheticCandidate()
	session.events <- terminalCandidate(union.ExecutionStatusSucceeded, 3)
	reconciler := reconcilerFunc(func(context.Context, execution.ReconcileInput) (execution.ReconcileReport, error) {
		return execution.ReconcileReport{
			Effects:         []union.EffectRecord{observedEffect(fixtureTime.Add(time.Minute))},
			SideEffectState: union.SideEffectObserved, Quiesced: true,
		}, nil
	})
	runtime := newTestRuntime(t, session, reconciler, nil)
	result, err := runtime.Execute(context.Background(), "fake", validInvocation("exec-synthetic"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Effects) != 0 {
		t.Fatalf("synthetic result produced %d Effects", len(result.Effects))
	}
	if result.Status != union.ExecutionStatusIndeterminate {
		t.Fatalf("synthetic attempted side effect status = %q, want indeterminate", result.Status)
	}
	if len(result.Residuals) == 0 {
		t.Fatal("rejected synthetic Effect did not leave a residual")
	}
}

func TestUnknownSideEffectIsIndeterminate(t *testing.T) {
	session := newFakeSession(4)
	session.events <- attemptCandidate(union.SideEffectUnknown, 1)
	session.events <- terminalCandidate(union.ExecutionStatusSucceeded, 2)
	runtime := newTestRuntime(t, session, nil, nil)
	result, err := runtime.Execute(context.Background(), "fake", validInvocation("exec-unknown"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != union.ExecutionStatusIndeterminate || result.VerificationStatus == union.VerificationVerified {
		t.Fatalf("unknown side effect projected as %q/%q", result.Status, result.VerificationStatus)
	}
}

func TestAdapterEffectCandidateIsRejectedAndNeverCommitted(t *testing.T) {
	session := newFakeSession(3)
	session.events <- attemptCandidate(union.SideEffectPossible, 1)
	header := execution.CandidateHeader(union.EventOriginExternal, union.EventFamilyEffect)
	session.events <- union.UnifiedExecutionEvent{
		Header: header,
		Effect: &union.EffectEvent{Kind: "observed", Effect: ptrEffect(observedEffect(fixtureTime.Add(time.Minute)))},
	}
	runtime := newTestRuntime(t, session, nil, nil)
	result, err := runtime.Execute(context.Background(), "fake", validInvocation("exec-adapter-effect"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != union.ExecutionStatusIndeterminate || len(result.Effects) != 0 {
		t.Fatalf("adapter Effect candidate projected as status=%q effects=%d", result.Status, len(result.Effects))
	}
}

func TestRuntimeRejectsAdapterIntentAndMechanismInjection(t *testing.T) {
	tests := []struct {
		name      string
		candidate union.UnifiedExecutionEvent
	}{
		{
			name: "accepted intent",
			candidate: func() union.UnifiedExecutionEvent {
				header := execution.CandidateHeader(union.EventOriginExternal, union.EventFamilyIntent)
				header.IntentID = "intent-injected"
				return union.UnifiedExecutionEvent{Header: header, Intent: &union.IntentEvent{Kind: "accepted"}}
			}(),
		},
		{
			name: "mechanism plan",
			candidate: func() union.UnifiedExecutionEvent {
				plan := validInvocation("unused").Plan.Mechanisms[0]
				plan.ID = "plan-injected"
				header := execution.CandidateHeader(union.EventOriginExternal, union.EventFamilyMechanism)
				header.IntentID, header.MechanismPlanID = plan.IntentID, plan.ID
				return union.UnifiedExecutionEvent{Header: header, Mechanism: &union.MechanismEvent{Kind: "selected", Plan: &plan}}
			}(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := newFakeSession(1)
			session.events <- test.candidate
			run, err := newTestRuntime(t, session, nil, nil).Start(context.Background(), "fake", validInvocation("exec-adapter-injection"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := run.Wait(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != union.ExecutionStatusIndeterminate {
				t.Fatalf("injected candidate status = %q", result.Status)
			}
			for _, event := range run.Events() {
				if event.Header.IntentID == "intent-injected" || event.Header.MechanismPlanID == "plan-injected" {
					t.Fatalf("injected candidate was committed: %#v", event)
				}
			}
		})
	}
}

func TestRuntimeRejectsCandidateSessionAndTurnIdentitySpoofing(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*union.EventHeader)
	}{
		{name: "session", mutate: func(header *union.EventHeader) { header.SessionID = "session-spoofed" }},
		{name: "turn", mutate: func(header *union.EventHeader) { header.TurnID = "turn-spoofed" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := newFakeSession(1)
			candidate := modelCandidate("content_completed", "must-not-commit", "provider_exposed_output", 1, nil)
			test.mutate(&candidate.Header)
			session.events <- candidate
			run, err := newTestRuntime(t, session, nil, nil).Start(context.Background(), "fake", validInvocation("exec-identity-spoof"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := run.Wait(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != union.ExecutionStatusIndeterminate {
				t.Fatalf("status = %q", result.Status)
			}
			for _, event := range run.Events() {
				if event.Model != nil && len(event.Model.Content) != 0 {
					t.Fatalf("spoofed candidate committed: %#v", event)
				}
			}
		})
	}
}

func TestPreflightManifestDriftFailsBeforeOpenAndCleansPreparedProcess(t *testing.T) {
	invocation := validInvocation("exec-manifest-drift")
	invocation.Plan.ExpectedManifest.Components = []union.ManifestComponent{{
		Kind: "identity", Name: "model", Version: "model-v1", State: "present", Digest: "sha256:expected",
	}}
	invocation.Plan.Digest = ""
	delete(invocation.Plan.Metadata, "request_digest")
	invocation, err := execution.NewInvocation(invocation.Request, invocation.Plan)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &manifestDriftAdapter{}
	registry := execution.NewRegistry()
	if err := registry.Register(context.Background(), adapter); err != nil {
		t.Fatal(err)
	}
	runtime, err := execution.NewRuntime(execution.RuntimeConfig{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Start(context.Background(), "manifest-drift", invocation)
	if !errors.Is(err, execution.ErrPreflightManifestDrift) {
		t.Fatalf("Start() error = %v", err)
	}
	if adapter.openCalls != 0 || adapter.cleanupCalls != 1 {
		t.Fatalf("open/cleanup calls = %d/%d", adapter.openCalls, adapter.cleanupCalls)
	}
}

type manifestDriftAdapter struct {
	openCalls    int
	cleanupCalls int
}

func (*manifestDriftAdapter) Describe(context.Context) (execution.AdapterDescriptor, error) {
	return execution.AdapterDescriptor{
		Identity: union.VersionedIdentity{ID: "manifest-drift", Version: "v1"}, Origin: union.EventOriginHarness,
		ExecutionKinds: []union.ExecutionKind{union.ExecutionKindModel},
	}, nil
}

func (*manifestDriftAdapter) Preflight(_ context.Context, invocation execution.Invocation) (execution.PreflightReport, error) {
	actual, _ := invocation.Plan.ExpectedManifest.Clone()
	actual.ID = "actual-manifest-drift"
	actual.Components[0].Digest = "sha256:changed"
	actual.Digest = ""
	actual.Digest, _ = actual.ComputeDigest()
	return execution.PreflightReport{Accepted: true, ActualManifest: actual}, nil
}

func (adapter *manifestDriftAdapter) Open(context.Context, execution.Invocation) (execution.Session, error) {
	adapter.openCalls++
	return nil, errors.New("Open must not be called")
}

func (adapter *manifestDriftAdapter) ClosePrepared(union.ExecutionID) error {
	adapter.cleanupCalls++
	return nil
}

func ptrEffect(value union.EffectRecord) *union.EffectRecord { return &value }

func TestRuntimeCancellationRequiresAckQuiescenceAndReconcile(t *testing.T) {
	session := newFakeSession(8)
	runtime := newTestRuntime(t, session, reconcilerFunc(func(_ context.Context, input execution.ReconcileInput) (execution.ReconcileReport, error) {
		if input.State.Cancellation != execution.CancellationReconciling {
			t.Fatalf("reconciler cancellation phase = %q", input.State.Cancellation)
		}
		return execution.ReconcileReport{SideEffectState: union.SideEffectNone, Quiesced: true}, nil
	}), nil)
	run, err := runtime.Start(context.Background(), "fake", validInvocation("exec-cancel"))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	command := union.ExecutionCommand{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: "exec-cancel", Kind: union.CommandCancelExecution,
		ExpectedExecutionStatus: "running", IdempotencyKey: "cancel-1",
	}
	if err := run.Command(context.Background(), command); err != nil {
		t.Fatalf("Command(cancel): %v", err)
	}
	select {
	case got := <-session.commands:
		if got.Kind != union.CommandCancelExecution {
			t.Fatalf("adapter command kind = %q", got.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("adapter did not receive cancellation")
	}
	session.events <- controlCandidate(execution.ControlCancelAcknowledged)
	session.events <- controlCandidate(execution.ControlCancellationQuiesced)
	session.events <- terminalCandidate(union.ExecutionStatusCancelled, 3)
	result, err := run.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if result.Status != union.ExecutionStatusCancelled {
		t.Fatalf("cancel result = %q", result.Status)
	}
	state := run.State()
	if state.Cancellation != execution.CancellationReconciled || state.Reconciliation != execution.ReconciliationCompleted {
		t.Fatalf("cancel/reconcile state = %q/%q", state.Cancellation, state.Reconciliation)
	}
	if err := run.Command(context.Background(), command); !errors.Is(err, execution.ErrTerminal) && err != nil {
		t.Fatalf("duplicate command after terminal = %v", err)
	}
}

func TestRuntimeConcurrentIdenticalCancelDispatchesOnce(t *testing.T) {
	session := newFakeSession(8)
	run, err := newTestRuntime(t, session, nil, nil).Start(context.Background(), "fake", validInvocation("exec-cancel-idempotent"))
	if err != nil {
		t.Fatal(err)
	}
	command := union.ExecutionCommand{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: "exec-cancel-idempotent", Kind: union.CommandCancelExecution,
		ExpectedExecutionStatus: "running", IdempotencyKey: "cancel-same",
	}
	start := make(chan struct{})
	errorsByCommand := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			errorsByCommand <- run.Command(context.Background(), command)
		}()
	}
	close(start)
	for range 2 {
		if commandErr := <-errorsByCommand; commandErr != nil {
			t.Fatalf("concurrent cancel = %v", commandErr)
		}
	}
	select {
	case delivered := <-session.commands:
		if delivered.Kind != union.CommandCancelExecution {
			t.Fatalf("delivered command = %q", delivered.Kind)
		}
	default:
		t.Fatal("cancel was not dispatched")
	}
	select {
	case duplicate := <-session.commands:
		t.Fatalf("duplicate command reached Adapter: %#v", duplicate)
	default:
	}
	session.events <- controlCandidate(execution.ControlCancelAcknowledged)
	session.events <- controlCandidate(execution.ControlCancellationQuiesced)
	session.events <- terminalCandidate(union.ExecutionStatusCancelled, 3)
	result, err := run.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != union.ExecutionStatusCancelled {
		t.Fatalf("status = %q", result.Status)
	}
}

func TestCommandReceiptOrdersBeforeEagerAdapterAcknowledgement(t *testing.T) {
	session := newEagerCancelSession()
	registry := execution.NewRegistry()
	if err := registry.Register(context.Background(), &fakeAdapter{id: "eager", session: session}); err != nil {
		t.Fatal(err)
	}
	runtime, err := execution.NewRuntime(execution.RuntimeConfig{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runtime.Start(context.Background(), "eager", validInvocation("exec-eager-cancel"))
	if err != nil {
		t.Fatal(err)
	}
	command := union.ExecutionCommand{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: "exec-eager-cancel", Kind: union.CommandCancelExecution,
		ExpectedExecutionStatus: "running", IdempotencyKey: "cancel-eager-1",
	}
	if err := run.Command(context.Background(), command); err != nil {
		t.Fatalf("Command: %v", err)
	}
	result, err := run.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if result.Status != union.ExecutionStatusCancelled {
		t.Fatalf("status = %q, want cancelled", result.Status)
	}
	var requested, dispatched, acknowledged, quiesced uint64
	for _, event := range run.Events() {
		if event.Control == nil {
			continue
		}
		switch event.Control.Kind {
		case execution.ControlCancelRequested:
			requested = event.Header.Sequence
		case execution.ControlCancelDispatched:
			dispatched = event.Header.Sequence
		case execution.ControlCancelAcknowledged:
			acknowledged = event.Header.Sequence
		case execution.ControlCancellationQuiesced:
			quiesced = event.Header.Sequence
		}
	}
	if requested == 0 || !(requested < dispatched && dispatched < acknowledged && acknowledged < quiesced) {
		t.Fatalf("cancel order = requested:%d dispatched:%d ack:%d quiesced:%d", requested, dispatched, acknowledged, quiesced)
	}
}

func TestUnconfirmedCancelDispatchTerminatesIndeterminate(t *testing.T) {
	session := &failingCommandSession{}
	registry := execution.NewRegistry()
	if err := registry.Register(context.Background(), &fakeAdapter{id: "dispatch-failure", session: session}); err != nil {
		t.Fatal(err)
	}
	runtime, err := execution.NewRuntime(execution.RuntimeConfig{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runtime.Start(context.Background(), "dispatch-failure", validInvocation("exec-dispatch-failure"))
	if err != nil {
		t.Fatal(err)
	}
	command := union.ExecutionCommand{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: "exec-dispatch-failure", Kind: union.CommandCancelExecution,
		ExpectedExecutionStatus: "running", IdempotencyKey: "cancel-dispatch-failure",
	}
	if err := run.Command(context.Background(), command); err == nil {
		t.Fatal("cancel dispatch unexpectedly succeeded")
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := run.Wait(waitCtx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if result.Status != union.ExecutionStatusIndeterminate || !run.State().Terminal {
		t.Fatalf("dispatch failure result = %q terminal=%v", result.Status, run.State().Terminal)
	}
}

type eagerCancelSession struct {
	events   chan union.UnifiedExecutionEvent
	consumed chan struct{}
}

func newEagerCancelSession() *eagerCancelSession {
	return &eagerCancelSession{events: make(chan union.UnifiedExecutionEvent, 3), consumed: make(chan struct{}, 1)}
}

func (session *eagerCancelSession) Receive(ctx context.Context) (union.UnifiedExecutionEvent, error) {
	select {
	case event := <-session.events:
		if event.Control != nil && event.Control.Kind == execution.ControlCancelAcknowledged {
			session.consumed <- struct{}{}
		}
		return event, nil
	case <-ctx.Done():
		return union.UnifiedExecutionEvent{}, ctx.Err()
	}
}

func (session *eagerCancelSession) Command(ctx context.Context, command union.ExecutionCommand) error {
	if command.Kind != union.CommandCancelExecution {
		return errors.New("unsupported command")
	}
	session.events <- controlCandidate(execution.ControlCancelAcknowledged)
	session.events <- controlCandidate(execution.ControlCancellationQuiesced)
	session.events <- terminalCandidate(union.ExecutionStatusCancelled, 3)
	select {
	case <-session.consumed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (*eagerCancelSession) Close() error { return nil }

type failingCommandSession struct{}

func (*failingCommandSession) Receive(ctx context.Context) (union.UnifiedExecutionEvent, error) {
	<-ctx.Done()
	return union.UnifiedExecutionEvent{}, ctx.Err()
}

func (*failingCommandSession) Command(context.Context, union.ExecutionCommand) error {
	return errors.New("dispatch outcome is unknown")
}

func (*failingCommandSession) Close() error { return nil }

func controlCandidate(kind string) union.UnifiedExecutionEvent {
	header := execution.CandidateHeader(union.EventOriginExternal, union.EventFamilyControl)
	return union.UnifiedExecutionEvent{Header: header, Control: &union.ControlEvent{Kind: kind}}
}
