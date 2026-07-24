package decisionworker_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/caseengine"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/decisionworker"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/verdictowner"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type fixtureV1 struct {
	ctx   context.Context
	clock *testkit.ManualClock
	store *memory.Store
	caseV contract.ReviewCaseV1
}

func newFixtureV1(t *testing.T) fixtureV1 {
	t.Helper()
	ctx := context.Background()
	clock := testkit.NewClock(time.Unix(1_750_000_000, 0))
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	testkit.PublishRubric(ctx, store, clock.Now(), "tenant-a")
	engine, err := caseengine.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(clock.Now())
	request := testkit.Request(clock.Now(), target, "case-worker")
	clock.Advance(time.Second)
	caseV, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{
		CaseID: "case-worker", Request: &request, Target: target,
		ExpiresUnixNano: request.ExpiresUnixNano,
		Trace:           testkit.TraceForTarget(clock.Now(), "case-worker", target, contract.TraceRequestedV1, 1, request.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []contract.CaseStateV1{contract.CaseAdmittedV1, contract.CaseRoutedV1} {
		clock.Advance(time.Second)
		caseV, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: caseV.TenantID, CaseID: caseV.ID, Expected: reviewport.ExpectedV1(caseV.Revision, caseV.Digest), Next: state}, Trace: testkit.TransitionTrace(clock.Now(), caseV, state)})
		if err != nil {
			t.Fatal(err)
		}
	}
	clock.Advance(time.Second)
	round := testkit.Round(clock.Now(), caseV, contract.RouteHumanV1)
	assignment := testkit.Assignment(clock.Now(), caseV, round, contract.RouteHumanV1)
	caseV, _, assignment, err = engine.StartRoundV1(ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(caseV.Revision, caseV.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(clock.Now(), caseV, contract.TraceAssignedV1, 2, round.ID, assignment.ID)})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	caseV, assignment, err = engine.ClaimAssignmentV1(ctx, reviewport.ClaimAssignmentMutationV1{TenantID: caseV.TenantID, ExpectedCase: reviewport.ExpectedV1(caseV.Revision, caseV.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), CaseID: caseV.ID, AssignmentID: assignment.ID, LeaseHolder: "worker-a", LeaseExpiresUnixNano: clock.Now().Add(15 * time.Minute).UnixNano(), UpdatedUnixNano: clock.Now().UnixNano(), Traces: []contract.TraceFactV1{testkit.StartedTrace(clock.Now(), caseV, assignment.ID)}})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(clock.Now(), caseV, round, assignment, contract.ResolutionAcceptV1, "idem-worker")
	caseV, _, err = engine.RecordAttestationV1(ctx, reviewport.ExpectedV1(caseV.Revision, caseV.Digest), attestation, testkit.Trace(clock.Now(), caseV, contract.TraceAttestedV1, 3, attestation.ID))
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	return fixtureV1{ctx: ctx, clock: clock, store: store, caseV: caseV}
}

type workerStoreV1 interface {
	reviewport.StoreV1
	reviewport.CaseQueryStoreV1
}

func newWorkerV1(t *testing.T, store workerStoreV1, clock *testkit.ManualClock) *decisionworker.Worker {
	t.Helper()
	current, err := memory.NewDecisionCurrentSourceV1(store, &testkit.ExternalCurrentReader{}, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := verdictowner.New(store, current, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := decisionworker.New(store, owner, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

func assertOneResolvedVerdictV1(t *testing.T, f fixtureV1) {
	t.Helper()
	caseV, err := f.store.InspectCaseV1(f.ctx, f.caseV.TenantID, f.caseV.ID)
	if err != nil {
		t.Fatal(err)
	}
	if caseV.State != contract.CaseResolvedV1 || caseV.VerdictID == "" {
		t.Fatalf("case not resolved exactly once: %+v", caseV)
	}
	if _, err := f.store.InspectVerdictExactV1(f.ctx, caseV.TenantID, reviewport.ExactV1(caseV.VerdictID, caseV.VerdictRevision, caseV.VerdictDigest)); err != nil {
		t.Fatal(err)
	}
	trace, err := f.store.ListTraceV1(f.ctx, caseV.TenantID, caseV.ID)
	if err != nil {
		t.Fatal(err)
	}
	verdictEvents := 0
	for _, fact := range trace {
		if fact.Event == contract.TraceVerdictV1 {
			verdictEvents++
		}
	}
	if verdictEvents != 1 {
		t.Fatalf("got %d verdict trace facts, want exactly one", verdictEvents)
	}
}

func TestWorkerResolvesAttestedCaseAndReplayDoesNotDuplicateV1(t *testing.T) {
	f := newFixtureV1(t)
	worker := newWorkerV1(t, f.store, f.clock)
	result, _, err := worker.RunOnceV1(f.ctx, f.caseV.TenantID, "", 16)
	if err != nil {
		t.Fatal(err)
	}
	if result.Inspected != 1 || result.Resolved != 1 || len(result.Failures) != 0 {
		t.Fatalf("unexpected first run: %+v", result)
	}
	result, _, err = worker.RunOnceV1(f.ctx, f.caseV.TenantID, "", 16)
	if err != nil {
		t.Fatal(err)
	}
	if result.Inspected != 0 || result.Resolved != 0 || len(result.Failures) != 0 {
		t.Fatalf("resolved case re-entered work queue: %+v", result)
	}
	assertOneResolvedVerdictV1(t, f)
}

func TestWorkerSixtyFourConcurrentReconcilersConvergeV1(t *testing.T) {
	f := newFixtureV1(t)
	worker := newWorkerV1(t, f.store, f.clock)
	var start sync.WaitGroup
	start.Add(1)
	var workers sync.WaitGroup
	var hardFailures atomic.Int64
	for range 64 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			start.Wait()
			result, _, err := worker.RunOnceV1(f.ctx, f.caseV.TenantID, "", 16)
			if err != nil {
				hardFailures.Add(1)
				return
			}
			for _, failure := range result.Failures {
				if !core.HasCategory(failure.Err, core.ErrorConflict) {
					hardFailures.Add(1)
				}
			}
		}()
	}
	start.Done()
	workers.Wait()
	if hardFailures.Load() != 0 {
		t.Fatalf("concurrent reconcilers produced %d non-conflict failures", hardFailures.Load())
	}
	assertOneResolvedVerdictV1(t, f)
}

type decideReplyLostStoreV1 struct {
	workerStoreV1
	once atomic.Bool
}

func (s *decideReplyLostStoreV1) DecideV1(ctx context.Context, mutation reviewport.DecideMutationV1) (contract.ReviewCaseV1, contract.VerdictV1, error) {
	caseV, verdict, err := s.workerStoreV1.DecideV1(ctx, mutation)
	if err == nil && s.once.CompareAndSwap(false, true) {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected reply loss after Review Decide commit")
	}
	return caseV, verdict, err
}

func TestWorkerLostDecideReplyRecoversExactCaseAndVerdictV1(t *testing.T) {
	f := newFixtureV1(t)
	wrapped := &decideReplyLostStoreV1{workerStoreV1: f.store}
	worker := newWorkerV1(t, wrapped, f.clock)
	result, _, err := worker.RunOnceV1(f.ctx, f.caseV.TenantID, "", 16)
	if err != nil {
		t.Fatal(err)
	}
	if result.Resolved != 1 || len(result.Failures) != 0 || !wrapped.once.Load() {
		t.Fatalf("lost reply did not converge through exact Inspect: %+v", result)
	}
	assertOneResolvedVerdictV1(t, f)
}
