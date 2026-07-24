package applicationadapter_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/applicationadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/review/caseengine"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	"github.com/Proview-China/rax/ExecutionRuntime/review/verdictowner"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type waitingFixtureV1 struct {
	ctx     context.Context
	clock   *testkit.ManualClock
	store   *memory.Store
	request contract.ReviewRequestV1
	target  contract.TargetSnapshotV1
	caseV   contract.ReviewCaseV1
}

func newWaitingFixtureV1(t *testing.T, id string) waitingFixtureV1 {
	t.Helper()
	ctx := context.Background()
	now := time.Unix(1_900_500_000, 0)
	clock := testkit.NewClock(now)
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	testkit.PublishRubric(ctx, store, now, "tenant-a")
	target := testkit.Target(now)
	request := testkit.Request(now, target, id)
	owner, _ := service.New(store, clock.Now)
	view, err := owner.SubmitV1(ctx, service.SubmitCommandV1{Request: request, Target: target, Trace: testkit.TraceForTarget(now, id, target, contract.TraceRequestedV1, 1, request.ID)})
	if err != nil {
		t.Fatal(err)
	}
	return waitingFixtureV1{ctx: ctx, clock: clock, store: store, request: request, target: target, caseV: view.Case}
}

func appRequestV1(t *testing.T, f waitingFixtureV1) applicationcontract.ReviewWaitingRequestV1 {
	t.Helper()
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(f.target.Scope)
	if err != nil {
		t.Fatal(err)
	}
	now := f.clock.Now()
	phase := applicationcontract.ReviewPhasePointCoordinateV1{Kind: applicationcontract.ReviewPhaseActionV1, ID: "phase-" + f.caseV.ID, Revision: 1, Digest: testkit.Digest("phase-" + f.caseV.ID), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: f.request.ExpiresUnixNano}
	target := applicationcontract.ReviewWaitingTargetCoordinateV1{TenantID: f.target.TenantID, ID: f.target.ID, Revision: f.target.Revision, Digest: f.target.Digest, RunID: f.target.RunID, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: f.target.ExpiresUnixNano}
	request, err := applicationcontract.SealReviewWaitingRequestV1(applicationcontract.ReviewWaitingRequestV1{Delivery: applicationcontract.ReviewWaitingDetachedV1, ExecutionScope: f.target.Scope, ExecutionScopeDigest: scopeDigest, Phase: phase, Target: target, ReviewRequest: applicationcontract.ReviewRequestCoordinateV1{TenantID: f.request.TenantID, ID: f.request.ID, Revision: f.request.Revision, Digest: f.request.Digest, CaseID: f.request.CaseID}, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: f.request.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func resolveWaitingFixtureV1(t *testing.T, f waitingFixtureV1, resolution contract.ResolutionV1, suffix string) (waitingFixtureV1, contract.VerdictV1) {
	t.Helper()
	engine, _ := caseengine.New(f.store, f.clock.Now)
	current := f.caseV
	var err error
	for _, state := range []contract.CaseStateV1{contract.CaseAdmittedV1, contract.CaseRoutedV1} {
		f.clock.Advance(time.Second)
		current, err = engine.TransitionWithTraceV2(f.ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: current.TenantID, CaseID: current.ID, Expected: reviewport.ExpectedV1(current.Revision, current.Digest), Next: state}, Trace: testkit.TransitionTrace(f.clock.Now(), current, state)})
		if err != nil {
			t.Fatal(err)
		}
	}
	f.clock.Advance(time.Second)
	round := testkit.Round(f.clock.Now(), current, contract.RouteHumanV1)
	assignment := testkit.Assignment(f.clock.Now(), current, round, contract.RouteHumanV1)
	current, _, assignment, err = engine.StartRoundV1(f.ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(current.Revision, current.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(f.clock.Now(), current, contract.TraceAssignedV1, 2, round.ID, assignment.ID)})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	current, assignment, err = engine.ClaimAssignmentV1(f.ctx, reviewport.ClaimAssignmentMutationV1{TenantID: current.TenantID, ExpectedCase: reviewport.ExpectedV1(current.Revision, current.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), CaseID: current.ID, AssignmentID: assignment.ID, LeaseHolder: assignment.ReviewerID, LeaseExpiresUnixNano: f.clock.Now().Add(15 * time.Minute).UnixNano(), UpdatedUnixNano: f.clock.Now().UnixNano(), Traces: []contract.TraceFactV1{testkit.StartedTrace(f.clock.Now(), current, assignment.ID)}})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(f.clock.Now(), current, round, assignment, resolution, suffix)
	current, _, err = engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(current.Revision, current.Digest), attestation, testkit.Trace(f.clock.Now(), current, contract.TraceAttestedV1, 3, attestation.ID))
	if err != nil {
		t.Fatal(err)
	}
	source, _ := memory.NewDecisionCurrentSourceV1(f.store, &testkit.ExternalCurrentReader{}, f.clock.Now)
	verdicts, _ := verdictowner.New(f.store, source, f.clock.Now)
	f.clock.Advance(time.Second)
	verdictID := "verdict-" + suffix
	resolved, verdict, err := verdicts.DecideV1(f.ctx, verdictowner.DecideCommandV1{TenantID: current.TenantID, CaseID: current.ID, Expected: reviewport.ExpectedV1(current.Revision, current.Digest), AttestationID: attestation.ID, VerdictID: verdictID, Trace: testkit.Trace(f.clock.Now(), current, contract.TraceVerdictV1, 4, verdictID)})
	if err != nil {
		t.Fatal(err)
	}
	f.caseV = resolved
	return f, verdict
}

func TestReviewWaitingAdapterPendingAndResolvedProjectionV1(t *testing.T) {
	t.Run("pending defers", func(t *testing.T) {
		f := newWaitingFixtureV1(t, "case-application-pending")
		request := appRequestV1(t, f)
		adapter, err := applicationadapter.NewReviewWaitingAdapterV1(f.store, f.clock.Now)
		if err != nil {
			t.Fatal(err)
		}
		projection, err := adapter.StartOrInspectReviewV1(f.ctx, request)
		if err != nil || projection.Decision != applicationcontract.ReviewPhaseDeferV1 || projection.Verdict != nil || projection.Case.Digest != f.caseV.Digest {
			t.Fatalf("pending projection=%+v err=%v", projection, err)
		}
		f.clock.Advance(time.Second)
		replayed, err := adapter.InspectReviewV1(f.ctx, applicationcontract.ReviewWaitingInspectRequestV1{Request: request.ReviewRequest, Target: request.Target})
		if err != nil || replayed != projection {
			t.Fatalf("time advance resealed the immutable current projection: first=%+v second=%+v err=%v", projection, replayed, err)
		}
	})

	t.Run("accepted resolves to allow", func(t *testing.T) {
		f := newWaitingFixtureV1(t, "case-application-allow")
		f, verdict := resolveWaitingFixtureV1(t, f, contract.ResolutionAcceptV1, "application-allow")
		request := appRequestV1(t, f)
		adapter, _ := applicationadapter.NewReviewWaitingAdapterV1(f.store, f.clock.Now)
		projection, err := adapter.StartOrInspectReviewV1(f.ctx, request)
		if err != nil || projection.Decision != applicationcontract.ReviewPhaseAllowV1 || projection.Verdict == nil || projection.Verdict.Digest != verdict.Digest || projection.Case.Revision != verdict.CaseRevision+1 {
			t.Fatalf("accepted projection=%+v err=%v", projection, err)
		}
	})
}

type waitingStoreWrapperV1 struct {
	*memory.Store
	requestFailures atomic.Int64
	requestCalls    atomic.Int64
	caseReads       atomic.Int64
	driftCase       *contract.ReviewCaseV1
	driftExactCase  *contract.ReviewCaseV1
	blockRecovery   bool
	notFoundRequest bool
}

func (s *waitingStoreWrapperV1) InspectRequestExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.ReviewRequestV1, error) {
	s.requestCalls.Add(1)
	if s.notFoundRequest {
		return contract.ReviewRequestV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "injected authoritative NotFound")
	}
	if s.requestFailures.Add(-1) >= 0 {
		return contract.ReviewRequestV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected read reply loss")
	}
	if s.blockRecovery {
		if _, ok := ctx.Deadline(); ok {
			<-ctx.Done()
			return contract.ReviewRequestV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected blocked recovery")
		}
	}
	return s.Store.InspectRequestExactV1(ctx, tenant, ref)
}

func (s *waitingStoreWrapperV1) InspectCaseV1(ctx context.Context, tenant core.TenantID, id string) (contract.ReviewCaseV1, error) {
	if s.caseReads.Add(1) == 2 && s.driftCase != nil {
		return *s.driftCase, nil
	}
	return s.Store.InspectCaseV1(ctx, tenant, id)
}

func (s *waitingStoreWrapperV1) InspectCaseExactV1(ctx context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.ReviewCaseV1, error) {
	if s.driftExactCase != nil {
		return *s.driftExactCase, nil
	}
	return s.Store.InspectCaseExactV1(ctx, tenant, ref)
}

func TestReviewWaitingAdapterReadRecoveryDriftRollbackAndConcurrencyV1(t *testing.T) {
	f := newWaitingFixtureV1(t, "case-application-fault")
	request := appRequestV1(t, f)

	t.Run("read reply loss retries exact only", func(t *testing.T) {
		store := &waitingStoreWrapperV1{Store: f.store}
		store.requestFailures.Store(1)
		adapter, _ := applicationadapter.NewReviewWaitingAdapterV1(store, f.clock.Now)
		projection, err := adapter.InspectReviewV1(context.Background(), applicationcontract.ReviewWaitingInspectRequestV1{Request: request.ReviewRequest, Target: request.Target})
		if err != nil || projection.RequestDigest != f.request.Digest {
			t.Fatalf("read-only recovery projection=%+v err=%v", projection, err)
		}
	})

	t.Run("blocked recovery is bounded by target TTL and preserves original unknown", func(t *testing.T) {
		store := &waitingStoreWrapperV1{Store: f.store, blockRecovery: true}
		store.requestFailures.Store(1)
		short := request
		short.Target.ExpiresUnixNano = f.clock.Now().Add(20 * time.Millisecond).UnixNano()
		adapter, _ := applicationadapter.NewReviewWaitingAdapterV1(store, f.clock.Now)
		started := time.Now()
		_, err := adapter.InspectReviewV1(context.Background(), applicationcontract.ReviewWaitingInspectRequestV1{Request: short.ReviewRequest, Target: short.Target})
		if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
			t.Fatalf("blocked detached recovery exceeded Target TTL: %s", elapsed)
		}
		if !core.HasCategory(err, core.ErrorIndeterminate) || !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
			t.Fatalf("blocked recovery overwrote original Unknown: %v", err)
		}
	})

	t.Run("authoritative NotFound is not retried", func(t *testing.T) {
		store := &waitingStoreWrapperV1{Store: f.store, notFoundRequest: true}
		adapter, _ := applicationadapter.NewReviewWaitingAdapterV1(store, f.clock.Now)
		if _, err := adapter.InspectReviewV1(context.Background(), applicationcontract.ReviewWaitingInspectRequestV1{Request: request.ReviewRequest, Target: request.Target}); !core.HasCategory(err, core.ErrorNotFound) {
			t.Fatalf("authoritative NotFound was not preserved: %v", err)
		}
		if store.requestCalls.Load() != 1 {
			t.Fatalf("authoritative NotFound triggered retry: calls=%d", store.requestCalls.Load())
		}
	})

	t.Run("S1 S2 drift fails closed", func(t *testing.T) {
		drift := f.caseV
		drift.Revision++
		drift.UpdatedUnixNano++
		drift.State = contract.CaseAdmittedV1
		drift.Digest = ""
		drift, _ = contract.SealReviewCaseV1(drift)
		store := &waitingStoreWrapperV1{Store: f.store, driftCase: &drift}
		adapter, _ := applicationadapter.NewReviewWaitingAdapterV1(store, f.clock.Now)
		if _, err := adapter.StartOrInspectReviewV1(f.ctx, request); !core.HasReason(err, core.ReasonReviewVerdictStale) {
			t.Fatalf("S1/S2 drift did not fail closed: %v", err)
		}
	})

	t.Run("clock rollback fails closed", func(t *testing.T) {
		calls := 0
		adapter, _ := applicationadapter.NewReviewWaitingAdapterV1(f.store, func() time.Time {
			calls++
			if calls == 1 {
				return f.clock.Now()
			}
			return f.clock.Now().Add(-time.Second)
		})
		if _, err := adapter.StartOrInspectReviewV1(f.ctx, request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("clock rollback did not fail closed: %v", err)
		}
	})

	t.Run("S2 actual point rollback fails closed", func(t *testing.T) {
		calls := 0
		adapter, _ := applicationadapter.NewReviewWaitingAdapterV1(f.store, func() time.Time {
			calls++
			if calls < 3 {
				return f.clock.Now()
			}
			return f.clock.Now().Add(-time.Second)
		})
		if _, err := adapter.StartOrInspectReviewV1(f.ctx, request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("S2 actual-point rollback did not fail closed: %v", err)
		}
	})

	t.Run("S2 actual point TTL crossing fails closed", func(t *testing.T) {
		calls := 0
		adapter, _ := applicationadapter.NewReviewWaitingAdapterV1(f.store, func() time.Time {
			calls++
			if calls < 3 {
				return f.clock.Now()
			}
			return time.Unix(0, f.request.ExpiresUnixNano)
		})
		if _, err := adapter.StartOrInspectReviewV1(f.ctx, request); err == nil {
			t.Fatal("S2 actual-point TTL crossing was accepted")
		}
	})

	t.Run("resolved Verdict historical Case drift fails closed", func(t *testing.T) {
		resolved, _ := resolveWaitingFixtureV1(t, newWaitingFixtureV1(t, "case-application-history-drift"), contract.ResolutionAcceptV1, "application-history-drift")
		resolvedRequest := appRequestV1(t, resolved)
		drift := resolved.caseV
		drift.Revision--
		drift.State = contract.CaseAttestedV1
		drift.VerdictID, drift.VerdictRevision, drift.VerdictDigest = "", 0, ""
		drift.TargetDigest = testkit.Digest("wrong-target")
		drift.Digest = ""
		drift, _ = contract.SealReviewCaseV1(drift)
		store := &waitingStoreWrapperV1{Store: resolved.store, driftExactCase: &drift}
		adapter, _ := applicationadapter.NewReviewWaitingAdapterV1(store, resolved.clock.Now)
		if _, err := adapter.StartOrInspectReviewV1(resolved.ctx, resolvedRequest); !core.HasReason(err, core.ReasonReviewVerdictStale) && !core.HasReason(err, core.ReasonReviewCandidateConflict) {
			t.Fatalf("historical predecessor Case drift was accepted: %v", err)
		}
	})

	t.Run("64 readers share one immutable cut", func(t *testing.T) {
		adapter, _ := applicationadapter.NewReviewWaitingAdapterV1(f.store, f.clock.Now)
		const workers = 64
		var group sync.WaitGroup
		errors := make(chan error, workers)
		digests := make(chan core.Digest, workers)
		for range workers {
			group.Add(1)
			go func() {
				defer group.Done()
				projection, err := adapter.StartOrInspectReviewV1(f.ctx, request)
				errors <- err
				digests <- projection.Digest
			}()
		}
		group.Wait()
		close(errors)
		close(digests)
		var first core.Digest
		for err := range errors {
			if err != nil {
				t.Fatal(err)
			}
		}
		for digest := range digests {
			if first == "" {
				first = digest
			} else if digest != first {
				t.Fatalf("concurrent projection digest drifted: %s != %s", digest, first)
			}
		}
	})
}

func TestReviewWaitingAdapterRejectsTypedNilDependenciesV1(t *testing.T) {
	var store *memory.Store
	if _, err := applicationadapter.NewReviewWaitingAdapterV1(store, time.Now); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil Store was accepted: %v", err)
	}
}
