package application

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type turnContinuationCoordinatorOwnerStubV1 struct {
	mu              sync.Mutex
	now             time.Time
	current         contract.TurnContinuationCurrentV1
	present         bool
	loseBeginReply  bool
	loseCommitReply bool
	commitErr       error
	beginCalls      int
	commitCalls     int
	inspectCalls    int
}

func (s *turnContinuationCoordinatorOwnerStubV1) BeginTurnContinuationV1(_ context.Context, start contract.TurnContinuationStartRequestV1) (contract.TurnContinuationCurrentV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.beginCalls++
	if s.present {
		if s.current.Start.Digest != start.Digest {
			return contract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "another Start already owns the Attempt")
		}
		return s.current.Clone(), nil
	}
	pending, err := contract.SealTurnContinuationPendingV1(start, s.now.UnixNano(), start.RequestedNotAfterUnixNano)
	if err != nil {
		return contract.TurnContinuationCurrentV1{}, err
	}
	s.current, s.present = pending, true
	if s.loseBeginReply {
		s.loseBeginReply = false
		return contract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost Begin reply")
	}
	return pending.Clone(), nil
}

func (s *turnContinuationCoordinatorOwnerStubV1) CommitTurnContinuationV1(_ context.Context, request contract.TurnContinuationCommitRequestV1) (contract.TurnContinuationCurrentV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commitCalls++
	if s.commitErr != nil {
		return contract.TurnContinuationCurrentV1{}, s.commitErr
	}
	if !s.present || s.current.Start.AttemptRefV1() != request.Pending.Start.AttemptRefV1() {
		return contract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "pending Attempt differs")
	}
	if s.current.State == contract.TurnContinuationContextCurrentV1 {
		if s.current.CommitRequestDigest != request.Digest {
			return contract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "another Commit already won")
		}
		return s.current.Clone(), nil
	}
	next, err := contract.SealTurnContinuationContextCurrentV1(request, s.now.UnixNano(), request.RequestedNotAfterUnixNano)
	if err != nil {
		return contract.TurnContinuationCurrentV1{}, err
	}
	s.current = next
	if s.loseCommitReply {
		s.loseCommitReply = false
		return contract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost Commit reply")
	}
	return next.Clone(), nil
}

func (s *turnContinuationCoordinatorOwnerStubV1) InspectTurnContinuationV1(_ context.Context, request contract.TurnContinuationInspectRequestV1) (contract.TurnContinuationCurrentV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inspectCalls++
	if !s.present || request.Validate() != nil || request.AttemptRef != s.current.Start.AttemptRefV1() {
		return contract.TurnContinuationCurrentV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Attempt not found")
	}
	return s.current.Clone(), nil
}

type turnContinuationContextCoordinatorStubV1 struct {
	mu       sync.Mutex
	expected ContextTurnRefreshCoordinationRequestV1
	result   contract.ContextTurnRefreshResultV1
	err      error
	calls    int
}

func (s *turnContinuationContextCoordinatorStubV1) CoordinateContextTurnRefreshV1(_ context.Context, request ContextTurnRefreshCoordinationRequestV1) (contract.ContextTurnRefreshResultV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if request.ID != s.expected.ID || request.ExecutionScopeDigest != s.expected.ExecutionScopeDigest || request.RunID != s.expected.RunID || !bytes.Equal(request.OpaqueContextRequest, s.expected.OpaqueContextRequest) {
		return contract.ContextTurnRefreshResultV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Context request drifted")
	}
	if s.err != nil {
		return contract.ContextTurnRefreshResultV1{}, s.err
	}
	return s.result, nil
}

type turnContinuationCoordinatorFixtureV1 struct {
	now     time.Time
	request TurnContinuationCoordinationRequestV1
	refresh contract.ContextTurnRefreshResultV1
}

func TestTurnContinuationCoordinatorOrdinaryClosedLoopV1(t *testing.T) {
	fixture := newTurnContinuationCoordinatorFixtureV1(t)
	owner := &turnContinuationCoordinatorOwnerStubV1{now: fixture.now}
	contextCoordinator := &turnContinuationContextCoordinatorStubV1{expected: fixture.request.ContextRefresh, result: fixture.refresh}
	coordinator, err := NewTurnContinuationCoordinatorV1(TurnContinuationCoordinatorConfigV1{Continuation: owner, Context: contextCoordinator, Clock: func() time.Time { return fixture.now }})
	mustTurnContinuationCoordinatorV1(t, err)
	current, err := coordinator.CoordinateTurnContinuationV1(context.Background(), fixture.request)
	mustTurnContinuationCoordinatorV1(t, err)
	if err := current.ModelTurnAllowedV1(fixture.now); err != nil {
		t.Fatal(err)
	}
	if current.State != contract.TurnContinuationContextCurrentV1 || current.AppliedContextRefresh == nil || current.AppliedContextRefresh.Digest != fixture.refresh.Digest || owner.beginCalls != 1 || owner.commitCalls != 1 || owner.inspectCalls != 0 || contextCoordinator.calls != 1 {
		t.Fatalf("ordinary continuation did not close exactly once: current=%+v begin=%d commit=%d inspect=%d context=%d", current, owner.beginCalls, owner.commitCalls, owner.inspectCalls, contextCoordinator.calls)
	}
}

func TestTurnContinuationCoordinatorLostRepliesInspectOriginalOnlyV1(t *testing.T) {
	for _, loss := range []string{"begin", "commit"} {
		t.Run(loss, func(t *testing.T) {
			fixture := newTurnContinuationCoordinatorFixtureV1(t)
			owner := &turnContinuationCoordinatorOwnerStubV1{now: fixture.now, loseBeginReply: loss == "begin", loseCommitReply: loss == "commit"}
			contextCoordinator := &turnContinuationContextCoordinatorStubV1{expected: fixture.request.ContextRefresh, result: fixture.refresh}
			coordinator, err := NewTurnContinuationCoordinatorV1(TurnContinuationCoordinatorConfigV1{Continuation: owner, Context: contextCoordinator, Clock: func() time.Time { return fixture.now }})
			mustTurnContinuationCoordinatorV1(t, err)
			current, err := coordinator.CoordinateTurnContinuationV1(context.Background(), fixture.request)
			mustTurnContinuationCoordinatorV1(t, err)
			if err := current.ModelTurnAllowedV1(fixture.now); err != nil {
				t.Fatal(err)
			}
			if owner.beginCalls != 1 || owner.commitCalls != 1 || owner.inspectCalls != 1 || contextCoordinator.calls != 1 {
				t.Fatalf("%s loss redispatched mutation: begin=%d commit=%d inspect=%d context=%d", loss, owner.beginCalls, owner.commitCalls, owner.inspectCalls, contextCoordinator.calls)
			}
		})
	}
}

func TestTurnContinuationCoordinatorContextFailureLeavesPendingAndRejectsPayloadSpliceV1(t *testing.T) {
	fixture := newTurnContinuationCoordinatorFixtureV1(t)
	owner := &turnContinuationCoordinatorOwnerStubV1{now: fixture.now}
	contextCoordinator := &turnContinuationContextCoordinatorStubV1{expected: fixture.request.ContextRefresh, result: fixture.refresh, err: errors.New("context failed")}
	coordinator, err := NewTurnContinuationCoordinatorV1(TurnContinuationCoordinatorConfigV1{Continuation: owner, Context: contextCoordinator, Clock: func() time.Time { return fixture.now }})
	mustTurnContinuationCoordinatorV1(t, err)
	if _, err = coordinator.CoordinateTurnContinuationV1(context.Background(), fixture.request); err == nil {
		t.Fatal("Context failure was accepted")
	}
	if owner.current.State != contract.TurnContinuationPendingV1 || owner.commitCalls != 0 || owner.current.ModelTurnAllowedV1(fixture.now) == nil {
		t.Fatalf("Context failure did not fail closed at pending: %+v", owner.current)
	}

	spliced := fixture.request
	spliced.ContextRefresh.OpaqueContextRequest = []byte(`{"spliced":true}`)
	if _, err = coordinator.CoordinateTurnContinuationV1(context.Background(), spliced); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same Attempt with another Context payload was accepted: %v", err)
	}
	if contextCoordinator.calls != 1 || owner.commitCalls != 0 {
		t.Fatalf("payload splice crossed an Owner boundary: context=%d commit=%d", contextCoordinator.calls, owner.commitCalls)
	}
}

func TestTurnContinuationCoordinatorConcurrentSameRequestHasOneSuccessorV1(t *testing.T) {
	fixture := newTurnContinuationCoordinatorFixtureV1(t)
	owner := &turnContinuationCoordinatorOwnerStubV1{now: fixture.now}
	contextCoordinator := &turnContinuationContextCoordinatorStubV1{expected: fixture.request.ContextRefresh, result: fixture.refresh}
	coordinator, err := NewTurnContinuationCoordinatorV1(TurnContinuationCoordinatorConfigV1{Continuation: owner, Context: contextCoordinator, Clock: func() time.Time { return fixture.now }})
	mustTurnContinuationCoordinatorV1(t, err)
	const workers = 64
	results := make(chan contract.TurnContinuationCurrentV1, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			value, callErr := coordinator.CoordinateTurnContinuationV1(context.Background(), fixture.request)
			if callErr != nil {
				errs <- callErr
				return
			}
			results <- value
		}()
	}
	wait.Wait()
	close(results)
	close(errs)
	for callErr := range errs {
		t.Fatal(callErr)
	}
	var digest core.Digest
	for value := range results {
		if digest == "" {
			digest = value.Digest
		}
		if value.Digest != digest {
			t.Fatal("concurrent request returned more than one current")
		}
	}
	if contextCoordinator.calls != 1 || owner.commitCalls != 1 {
		t.Fatalf("concurrent request created duplicate Context/Commit effects: context=%d commit=%d", contextCoordinator.calls, owner.commitCalls)
	}
}

func TestTurnContinuationCoordinatorExecutionBoundaryHasNoModelToolOrProviderPortV1(t *testing.T) {
	configType := reflect.TypeOf(TurnContinuationCoordinatorConfigV1{})
	if configType.NumField() != 3 {
		t.Fatalf("execution boundary expanded unexpectedly: fields=%d", configType.NumField())
	}
	for index := 0; index < configType.NumField(); index++ {
		field := configType.Field(index)
		boundary := strings.ToLower(field.Name + " " + field.Type.String())
		for _, forbidden := range []string{"model", "tool", "provider"} {
			if strings.Contains(boundary, forbidden) {
				t.Fatalf("TurnContinuation execution boundary gained forbidden %s authority: %s", forbidden, boundary)
			}
		}
	}
	contextPort := reflect.TypeOf((*ContextTurnRefreshCoordinatorPortV1)(nil)).Elem()
	if contextPort.NumMethod() != 1 || contextPort.Method(0).Name != "CoordinateContextTurnRefreshV1" {
		t.Fatalf("Context seam expanded unexpectedly: %s", contextPort)
	}
}

func TestTurnContinuationCoordinatorFailsClosedOnExpiredAndRegressedClockV1(t *testing.T) {
	t.Run("expired before Begin", func(t *testing.T) {
		fixture := newTurnContinuationCoordinatorFixtureV1(t)
		owner := &turnContinuationCoordinatorOwnerStubV1{now: fixture.now}
		contextCoordinator := &turnContinuationContextCoordinatorStubV1{expected: fixture.request.ContextRefresh, result: fixture.refresh}
		coordinator, err := NewTurnContinuationCoordinatorV1(TurnContinuationCoordinatorConfigV1{Continuation: owner, Context: contextCoordinator, Clock: func() time.Time { return time.Unix(0, fixture.request.Start.RequestedNotAfterUnixNano) }})
		mustTurnContinuationCoordinatorV1(t, err)
		if _, err = coordinator.CoordinateTurnContinuationV1(context.Background(), fixture.request); err == nil {
			t.Fatal("expired continuation was accepted")
		}
		if owner.beginCalls != 0 || owner.commitCalls != 0 || contextCoordinator.calls != 0 {
			t.Fatalf("expired request crossed an execution boundary: begin=%d context=%d commit=%d", owner.beginCalls, contextCoordinator.calls, owner.commitCalls)
		}
	})

	t.Run("clock regresses after Begin", func(t *testing.T) {
		fixture := newTurnContinuationCoordinatorFixtureV1(t)
		owner := &turnContinuationCoordinatorOwnerStubV1{now: fixture.now}
		contextCoordinator := &turnContinuationContextCoordinatorStubV1{expected: fixture.request.ContextRefresh, result: fixture.refresh}
		var clockCalls atomic.Int64
		clock := func() time.Time {
			if clockCalls.Add(1) == 1 {
				return fixture.now
			}
			return fixture.now.Add(-time.Nanosecond)
		}
		coordinator, err := NewTurnContinuationCoordinatorV1(TurnContinuationCoordinatorConfigV1{Continuation: owner, Context: contextCoordinator, Clock: clock})
		mustTurnContinuationCoordinatorV1(t, err)
		if _, err = coordinator.CoordinateTurnContinuationV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock regression was not rejected: %v", err)
		}
		if owner.beginCalls != 1 || owner.commitCalls != 0 || contextCoordinator.calls != 0 || owner.current.ModelTurnAllowedV1(fixture.now) == nil {
			t.Fatalf("clock regression did not remain pending before Context: begin=%d context=%d commit=%d", owner.beginCalls, contextCoordinator.calls, owner.commitCalls)
		}
	})
}

func TestTurnContinuationCoordinatorStaleActiveContextCASFailsClosedV1(t *testing.T) {
	fixture := newTurnContinuationCoordinatorFixtureV1(t)
	owner := &turnContinuationCoordinatorOwnerStubV1{
		now:       fixture.now,
		commitErr: core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "ActiveContext CAS is stale"),
	}
	contextCoordinator := &turnContinuationContextCoordinatorStubV1{expected: fixture.request.ContextRefresh, result: fixture.refresh}
	coordinator, err := NewTurnContinuationCoordinatorV1(TurnContinuationCoordinatorConfigV1{Continuation: owner, Context: contextCoordinator, Clock: func() time.Time { return fixture.now }})
	mustTurnContinuationCoordinatorV1(t, err)
	if _, err = coordinator.CoordinateTurnContinuationV1(context.Background(), fixture.request); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("stale ActiveContext CAS was not preserved: %v", err)
	}
	if owner.beginCalls != 1 || contextCoordinator.calls != 1 || owner.commitCalls != 1 || owner.inspectCalls != 0 || owner.current.ModelTurnAllowedV1(fixture.now) == nil {
		t.Fatalf("stale CAS was retried or opened the Model gate: begin=%d context=%d commit=%d inspect=%d", owner.beginCalls, contextCoordinator.calls, owner.commitCalls, owner.inspectCalls)
	}
}

func TestTurnContinuationCoordinatorFinalReplayRejectsUnprovableOwnerBackedRefreshV1(t *testing.T) {
	fixture := newTurnContinuationCoordinatorFixtureV1(t)
	owner := &turnContinuationCoordinatorOwnerStubV1{now: fixture.now}
	contextCoordinator := &turnContinuationContextCoordinatorStubV1{expected: fixture.request.ContextRefresh, result: fixture.refresh}
	coordinator, err := NewTurnContinuationCoordinatorV1(TurnContinuationCoordinatorConfigV1{Continuation: owner, Context: contextCoordinator, Clock: func() time.Time { return fixture.now }})
	mustTurnContinuationCoordinatorV1(t, err)
	if _, err = coordinator.CoordinateTurnContinuationV1(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	ownerBacked := fixture.request
	ownerBacked.ContextRefresh.Memory = &contract.ContextOwnerSourceRequestV1{}
	if _, err = coordinator.CoordinateTurnContinuationV1(context.Background(), ownerBacked); !core.HasCategory(err, core.ErrorCapabilityUnavailable) {
		t.Fatalf("final replay accepted an owner-backed refresh it cannot prove exactly: %v", err)
	}
	if owner.beginCalls != 1 || owner.commitCalls != 1 || contextCoordinator.calls != 1 {
		t.Fatalf("unprovable final replay crossed an Owner boundary: begin=%d context=%d commit=%d", owner.beginCalls, contextCoordinator.calls, owner.commitCalls)
	}
}

func newTurnContinuationCoordinatorFixtureV1(t *testing.T) turnContinuationCoordinatorFixtureV1 {
	t.Helper()
	now := time.Unix(1_820_000_000, 0)
	digest := func(seed string) core.Digest { return core.DigestBytes([]byte(seed)) }
	exact := func(kind, id string) contract.ContextRefreshExactRefV1 {
		return contract.ContextRefreshExactRefV1{Kind: runtimeports.NamespacedNameV2(kind), ID: id, Revision: 1, Digest: digest(id)}
	}
	scope, runID := digest("coordinator-scope"), core.AgentRunID("coordinator-run")
	sessionDigest, turnDigest := digest("coordinator-session"), digest("coordinator-turn")
	source, err := contract.SealContextTurnSourceCurrentV1(contract.ContextTurnSourceCurrentV1{
		ExecutionScopeDigest: scope, RunID: runID,
		Session:              contract.SingleCallSessionCoordinateV1{ID: "coordinator-session", Revision: 2, Digest: sessionDigest, Phase: contract.SingleCallSessionWaitingActionV1, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()},
		SessionApplicability: contract.SingleCallSessionApplicabilitySourceCoordinateV1{Kind: contract.SingleCallSessionSourceKindV1, ID: "session:" + string(sessionDigest), Revision: 2, Digest: sessionDigest},
		Turn:                 contract.SingleCallTurnCoordinateV1{ID: "turn:" + string(turnDigest), Ordinal: 3, Revision: 3, Digest: turnDigest},
		TurnApplicability:    contract.SingleCallTurnApplicabilitySourceCoordinateV1{Kind: contract.SingleCallTurnSourceKindV1, ID: "turn:" + string(turnDigest), Revision: 3, Digest: turnDigest},
		CheckedUnixNano:      now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	mustTurnContinuationCoordinatorV1(t, err)
	active, err := contract.SealHarnessActiveContextRefV1(contract.HarnessActiveContextRefV1{
		Revision: 5, ExecutionScopeDigest: scope, RunID: runID, SessionID: source.Session.ID, TurnOrdinal: source.Turn.Ordinal,
		ManifestRef: exact("context/manifest", "coordinator-manifest-3"), FrameRef: exact("context/frame", "coordinator-frame-3"), GenerationRef: exact("context/generation", "coordinator-generation-3"), ContextCurrentPointerRef: exact("context/generation-current", "coordinator-current-3"), UpdatedUnixNano: now.Add(-2 * time.Second).UnixNano(),
	})
	mustTurnContinuationCoordinatorV1(t, err)
	startInput := contract.TurnContinuationStartRequestV1{
		ExecutionScopeDigest: scope, RunID: runID, Source: source,
		SettledToolResult:     contract.SingleCallToolActionResultRefV2{ID: "coordinator-result", Revision: 1, Digest: digest("coordinator-result"), RequestID: "coordinator-request", RequestRevision: 1, RequestDigest: digest("coordinator-request"), ActionCoordinateDigest: digest("coordinator-action"), ToolResultID: "coordinator-tool-result", ToolResultRevision: 1, ToolResultDigest: digest("coordinator-tool-result")},
		ExpectedActiveContext: active, TargetTurn: source.Turn.Ordinal + 1, RequestedNotAfterUnixNano: now.Add(15 * time.Second).UnixNano(),
	}
	attemptID, err := contract.DeriveTurnContinuationAttemptIDV1(startInput)
	mustTurnContinuationCoordinatorV1(t, err)
	contextRequest := ContextTurnRefreshCoordinationRequestV1{
		ID: attemptID, ExecutionScopeDigest: scope, RunID: runID, SourceSession: source.Session, SessionApplicability: source.SessionApplicability, SourceTurn: source.Turn, TurnApplicability: source.TurnApplicability,
		OpaqueContextRequest: []byte(`{"tool_result":"exact"}`), RequestedNotAfterNano: startInput.RequestedNotAfterUnixNano,
	}
	prepare, err := contract.SealContextTurnRefreshPrepareRequestV1(contract.ContextTurnRefreshPrepareRequestV1{
		ID: contextRequest.ID, ExecutionScopeDigest: scope, RunID: runID, SourceSession: source.Session, SessionApplicability: source.SessionApplicability, SourceTurn: source.Turn, TurnApplicability: source.TurnApplicability,
		ExpectedTargetTurn: source.Turn.Ordinal + 1, OpaqueContextRequest: contextRequest.OpaqueContextRequest, RequestedNotAfterNano: contextRequest.RequestedNotAfterNano,
	})
	mustTurnContinuationCoordinatorV1(t, err)
	startInput.ExpectedContextRefreshAttempt, err = prepare.AttemptRefV1()
	mustTurnContinuationCoordinatorV1(t, err)
	start, err := contract.SealTurnContinuationStartRequestV1(startInput)
	mustTurnContinuationCoordinatorV1(t, err)
	settlement, pointer := exact("context/apply-settlement", "coordinator-apply-4"), exact("context/generation-current", "coordinator-current-4")
	refresh, err := contract.SealContextTurnRefreshResultV1(contract.ContextTurnRefreshResultV1{
		AttemptRef: start.ExpectedContextRefreshAttempt, PendingDomainResultRef: exact("context/pending-result", "coordinator-pending-4"), TransitionProofRef: exact("context/transition-proof", "coordinator-proof-4"), ManifestRef: exact("context/manifest", "coordinator-manifest-4"), FrameRef: exact("context/frame", "coordinator-frame-4"), GenerationRef: exact("context/generation", "coordinator-generation-4"),
		StableSourceSetDigest: digest("coordinator-stable"), S1AssociationSetDigest: digest("coordinator-s1"), S2AssociationSetDigest: digest("coordinator-s2"), ApplySettlementRef: &settlement, CurrentPointerRef: &pointer,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(12 * time.Second).UnixNano(), State: contract.ContextTurnRefreshAppliedStateV1,
	})
	mustTurnContinuationCoordinatorV1(t, err)
	return turnContinuationCoordinatorFixtureV1{now: now, request: TurnContinuationCoordinationRequestV1{Start: start, ContextRefresh: contextRequest}, refresh: refresh}
}

func mustTurnContinuationCoordinatorV1(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
