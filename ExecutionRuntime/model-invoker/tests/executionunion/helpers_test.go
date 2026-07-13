package executionunion_test

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

var fixtureTime = time.Date(2026, 7, 13, 1, 0, 0, 0, time.UTC)

func validInvocation(executionID union.ExecutionID) execution.Invocation {
	profile := union.VersionedIdentity{ID: "profile-test", Version: "v1"}
	route := union.VersionedIdentity{ID: "route-test", Version: "v1"}
	graph := union.IntentGraph{Nodes: []union.IntentNode{{
		ID: "intent-1", Kind: union.IntentModifyFile, Target: "/workspace/file.txt", Required: true,
	}}}
	request := union.UnifiedExecutionRequest{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: executionID,
		ProfileSelector: union.ProfileSelector{Exact: &profile}, ExecutionKind: union.ExecutionKindModel,
		SessionIntent:     union.SessionIntent{Mode: "new", SessionID: "session-1", TurnID: "turn-1"},
		DegradationPolicy: union.DegradationPolicy{Default: union.DegradationDefaultReject}, IntentGraph: graph,
	}
	plan := union.PreparedExecutionPlan{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: executionID, Profile: profile, Route: route,
		ProfileKeyDigest: "sha256:profile", ExecutionKind: union.ExecutionKindModel, IntentGraph: graph,
		Mechanisms: []union.MechanismPlan{{
			ID: "plan-1", IntentID: "intent-1", Kind: "caller_tool", Origin: union.CapabilityOriginCallerHosted,
			Owner: union.ExecutionOwnerPraxis, SelectionAuthority: union.SelectionAuthorityRuntime,
			SemanticFidelity: union.SemanticFidelityExact,
		}},
		ExpectedManifest: union.ContextManifestSummary{ID: "expected-manifest", Version: "v1", Mode: "test"},
		RouteFingerprint: "sha256:route",
	}
	invocation, err := execution.NewInvocation(request, plan)
	if err != nil {
		panic(err)
	}
	return invocation
}

func actualManifest() union.ContextManifestSummary {
	return union.ContextManifestSummary{ID: "actual-manifest", Version: "v1", Mode: "test"}
}

func attemptCandidate(sideEffects union.SideEffectState, sourceSequence uint64) union.UnifiedExecutionEvent {
	header := execution.CandidateHeader(union.EventOriginExternal, union.EventFamilyMechanism)
	header.Sequence = sourceSequence
	header.Timestamp = fixtureTime.Add(time.Duration(sourceSequence) * time.Second)
	header.MechanismPlanID = "plan-1"
	header.MechanismAttemptID = "attempt-1"
	header.IntentID = "intent-1"
	return union.UnifiedExecutionEvent{
		Header: header,
		Mechanism: &union.MechanismEvent{Kind: "attempt_started", Attempt: &union.MechanismAttempt{
			ID: "attempt-1", MechanismPlanID: "plan-1", Authoritative: true, ActualKind: "caller_tool",
			ActualOrigin: union.CapabilityOriginCallerHosted, ActualOwner: union.ExecutionOwnerPraxis,
			StartedAt: fixtureTime, Status: union.AttemptStatusRunning, SideEffectState: sideEffects,
		}},
	}
}

func terminalCandidate(status union.ExecutionStatus, sourceSequence uint64) union.UnifiedExecutionEvent {
	header := execution.CandidateHeader(union.EventOriginExternal, union.EventFamilyLifecycle)
	header.Sequence = sourceSequence
	header.Timestamp = fixtureTime.Add(time.Duration(sourceSequence) * time.Second)
	return union.UnifiedExecutionEvent{
		Header:    header,
		Lifecycle: &union.LifecycleEvent{Kind: "route_terminal", Status: status, StopReason: "route_done"},
	}
}

func syntheticCandidate() union.UnifiedExecutionEvent {
	executed := false
	header := execution.CandidateHeader(union.EventOriginExternal, union.EventFamilyModel)
	header.Sequence = 2
	header.Timestamp = fixtureTime.Add(2 * time.Second)
	header.IntentID = "intent-1"
	header.MechanismAttemptID = "attempt-1"
	header.ActionID = "action-1"
	header.ItemID = "item-1"
	return union.UnifiedExecutionEvent{
		Header: header,
		Model: &union.ModelEvent{
			Kind: "tool_result", ActionID: "action-1", Executed: &executed,
			ExecutionItemID: "item-1", SyntheticReason: "protocol_pairing_only", Payload: json.RawMessage(`{"status":"skipped"}`),
		},
	}
}

func observedEffect(now time.Time) union.EffectRecord {
	return union.EffectRecord{
		ID: "effect-1", IntentIDs: []union.IntentID{"intent-1"}, MechanismAttemptID: "attempt-1",
		Kind: "file_changed", Target: "/workspace/file.txt",
		Payload: union.EffectPayload{WorkspaceChange: &union.WorkspaceChange{
			Kind: "file_changed", Path: "/workspace/file.txt",
		}},
		ObservationSource: "fake_observer", VerificationStatus: union.VerificationUnverified, OccurredAt: now,
	}
}

func verifiedRecord(now time.Time) union.VerificationRecord {
	return union.VerificationRecord{
		ID: "verification-1", EffectIDs: []union.EffectID{"effect-1"}, IntentIDs: []union.IntentID{"intent-1"},
		Kind: "fake_postcondition", Status: union.VerificationVerified,
		Verifier: union.VersionedIdentity{ID: "fake-verifier", Version: "v1"}, CompletedAt: now,
	}
}

type fakeAdapter struct {
	id      string
	session execution.Session
}

func (adapter *fakeAdapter) Describe(context.Context) (execution.AdapterDescriptor, error) {
	id := adapter.id
	if id == "" {
		id = "fake"
	}
	return execution.AdapterDescriptor{
		Identity: union.VersionedIdentity{ID: id, Version: "v1"}, Origin: union.EventOriginExternal,
		ExecutionKinds: []union.ExecutionKind{union.ExecutionKindModel},
	}, nil
}

func (adapter *fakeAdapter) Preflight(context.Context, execution.Invocation) (execution.PreflightReport, error) {
	return execution.PreflightReport{Accepted: true, ActualManifest: actualManifest()}, nil
}

func (adapter *fakeAdapter) Open(context.Context, execution.Invocation) (execution.Session, error) {
	return adapter.session, nil
}

type fakeSession struct {
	events     chan union.UnifiedExecutionEvent
	commands   chan union.ExecutionCommand
	closeOnce  sync.Once
	closeErr   error
	closeCalls atomic.Int64
}

func newFakeSession(buffer int) *fakeSession {
	return &fakeSession{events: make(chan union.UnifiedExecutionEvent, buffer), commands: make(chan union.ExecutionCommand, buffer)}
}

func (session *fakeSession) Receive(ctx context.Context) (union.UnifiedExecutionEvent, error) {
	select {
	case event, open := <-session.events:
		if !open {
			return union.UnifiedExecutionEvent{}, io.EOF
		}
		return event, nil
	case <-ctx.Done():
		return union.UnifiedExecutionEvent{}, ctx.Err()
	}
}

func (session *fakeSession) Command(ctx context.Context, command union.ExecutionCommand) error {
	select {
	case session.commands <- command:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (session *fakeSession) Close() error {
	session.closeOnce.Do(func() { session.closeCalls.Add(1) })
	return session.closeErr
}

type reconcilerFunc func(context.Context, execution.ReconcileInput) (execution.ReconcileReport, error)

func (function reconcilerFunc) Reconcile(ctx context.Context, input execution.ReconcileInput) (execution.ReconcileReport, error) {
	return function(ctx, input)
}

type verifierFunc func(context.Context, execution.VerifyInput) (execution.VerificationReport, error)

func (function verifierFunc) Verify(ctx context.Context, input execution.VerifyInput) (execution.VerificationReport, error) {
	return function(ctx, input)
}

func newTestRuntime(t *testing.T, session *fakeSession, reconciler execution.Reconciler, verifier execution.Verifier) *execution.Runtime {
	t.Helper()
	registry := execution.NewRegistry()
	if err := registry.Register(context.Background(), &fakeAdapter{session: session}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	var tick atomic.Int64
	runtime, err := execution.NewRuntime(execution.RuntimeConfig{
		Registry: registry, Reconciler: reconciler, Verifier: verifier,
		Clock: func() time.Time {
			return fixtureTime.Add(time.Duration(tick.Add(1)) * time.Millisecond)
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	return runtime
}
