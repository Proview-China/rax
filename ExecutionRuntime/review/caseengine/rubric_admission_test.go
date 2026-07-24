package caseengine_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/caseengine"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type roundRubricFixtureV1 struct {
	ctx        context.Context
	clock      *testkit.ManualClock
	storeClock *testkit.ManualClock
	store      reviewport.StoreV1
	engine     *caseengine.Engine
	caseFact   contract.ReviewCaseV1
}

func newRoundRubricFixtureV1(t *testing.T) roundRubricFixtureV1 {
	t.Helper()
	ctx := context.Background()
	base := time.Unix(1_901_300_000, 0)
	clock := testkit.NewClock(base)
	storeClock := testkit.NewClock(base)
	store := storetestkit.NewMemoryStoreV1(storeClock.Now)
	testkit.PublishRubric(ctx, store, base, "tenant-a")
	engine, err := caseengine.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(base)
	request := testkit.Request(base, target, "case-round-rubric")
	clock.Advance(time.Second)
	storeClock.Set(clock.Now())
	caseFact, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: request.CaseID, Request: &request, Target: target, ExpiresUnixNano: request.ExpiresUnixNano, Trace: testkit.TraceForTarget(clock.Now(), request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []contract.CaseStateV1{contract.CaseAdmittedV1, contract.CaseRoutedV1} {
		clock.Advance(time.Second)
		caseFact, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: caseFact.TenantID, CaseID: caseFact.ID, Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), Next: state}, Trace: testkit.TransitionTrace(clock.Now(), caseFact, state)})
		if err != nil {
			t.Fatal(err)
		}
	}
	return roundRubricFixtureV1{ctx: ctx, clock: clock, storeClock: storeClock, store: store, engine: engine, caseFact: caseFact}
}

func (f roundRubricFixtureV1) mutation(t *testing.T) reviewport.StartRoundMutationV1 {
	t.Helper()
	f.clock.Advance(time.Second)
	round := testkit.Round(f.clock.Now(), f.caseFact, contract.RouteHumanV1)
	assignment := testkit.Assignment(f.clock.Now(), f.caseFact, round, contract.RouteHumanV1)
	return reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(f.caseFact.Revision, f.caseFact.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(f.clock.Now(), f.caseFact, contract.TraceAssignedV1, 2, round.ID, assignment.ID)}
}

func TestStartRoundRubricStoreActualPointFailsClosedV1(t *testing.T) {
	for _, test := range []struct {
		name       string
		wantReason core.ReasonCode
	}{
		{name: "ttl_crossing", wantReason: core.ReasonReviewVerdictStale},
		{name: "clock_rollback", wantReason: core.ReasonClockRegression},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newRoundRubricFixtureV1(t)
			mutation := f.mutation(t)
			if test.name == "ttl_crossing" {
				rubric, err := f.store.InspectRubricExactV1(f.ctx, f.caseFact.TenantID, *f.caseFact.Rubric)
				if err != nil {
					t.Fatal(err)
				}
				f.storeClock.Set(time.Unix(0, rubric.ExpiresUnixNano).Add(time.Nanosecond))
			} else {
				f.storeClock.Set(time.Unix(1_901_300_000, 0))
			}
			if _, _, _, err := f.engine.StartRoundV1(f.ctx, mutation); !core.HasReason(err, test.wantReason) {
				t.Fatalf("StartRound actual-point %s was admitted: %v", test.name, err)
			}
			current, err := f.store.InspectCaseV1(f.ctx, f.caseFact.TenantID, f.caseFact.ID)
			if err != nil || current.Digest != f.caseFact.Digest {
				t.Fatalf("failed StartRound changed Case: %+v %v", current, err)
			}
			if _, err := f.store.InspectRoundExactV1(f.ctx, mutation.Round.TenantID, reviewport.ExactV1(mutation.Round.ID, mutation.Round.Revision, mutation.Round.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("failed StartRound leaked Round: %v", err)
			}
		})
	}
}

func TestStartRoundRequiresCaseAndRoundExactRubricV1(t *testing.T) {
	f := newRoundRubricFixtureV1(t)
	mutation := f.mutation(t)
	mutation.Round.Rubric = nil
	mutation.Round.Digest = ""
	mutation.Round, _ = contract.SealReviewRoundV1(mutation.Round)
	if _, _, _, err := f.engine.StartRoundV1(f.ctx, mutation); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("Round without exact Rubric was admitted: %v", err)
	}
}
