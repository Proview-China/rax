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
)

func TestWhiteboxAttestationWaitingStates(t *testing.T) {
	for _, tc := range []struct {
		name       string
		resolution contract.ResolutionV1
		want       contract.CaseStateV1
	}{{"revision", contract.ResolutionRequestChangesV1, contract.CaseWaitingRevisionV1}, {"human", contract.ResolutionEscalateHumanV1, contract.CaseWaitingHumanV1}, {"evidence", contract.ResolutionInsufficientEvidenceV1, contract.CaseWaitingEvidenceV1}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			clock := testkit.NewClock(time.Unix(1_750_000_000, 0))
			store := storetestkit.NewMemoryStoreV1(clock.Now)
			testkit.PublishRubric(ctx, store, clock.Now(), "tenant-a")
			engine, _ := caseengine.New(store, clock.Now)
			target := testkit.Target(clock.Now())
			request := testkit.Request(clock.Now(), target, "case-a")
			clock.Advance(time.Second)
			created, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-a", Request: &request, Target: target, ExpiresUnixNano: request.ExpiresUnixNano, Trace: testkit.TraceForTarget(clock.Now(), "case-a", target, contract.TraceRequestedV1, 1, request.ID)})
			if err != nil {
				t.Fatal(err)
			}
			current, round, assignment := advanceToReviewing(t, ctx, engine, clock, created, contract.RouteHumanV1)
			clock.Advance(time.Second)
			att := testkit.HumanAttestation(clock.Now(), current, round, assignment, tc.resolution, "idem-"+tc.name)
			trace := testkit.Trace(clock.Now(), current, contract.TraceAttestedV1, 3, att.ID)
			var got contract.ReviewCaseV1
			if tc.resolution == contract.ResolutionEscalateHumanV1 {
				escalated := testkit.Trace(clock.Now(), current, contract.TraceEscalatedV1, 4, att.ID)
				escalated.CaseRevision++
				escalated.Digest = ""
				escalated, err = contract.SealTraceFactV1(escalated)
				if err == nil {
					got, _, err = engine.RecordAttestationWithTraceV2(ctx, reviewport.ExpectedV1(current.Revision, current.Digest), att, trace, []contract.TraceFactV1{escalated})
				}
			} else {
				got, _, err = engine.RecordAttestationV1(ctx, reviewport.ExpectedV1(current.Revision, current.Digest), att, trace)
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.State != tc.want {
				t.Fatalf("got %s want %s", got.State, tc.want)
			}
		})
	}
}

func advanceToReviewing(t *testing.T, ctx context.Context, engine *caseengine.Engine, clock *testkit.ManualClock, c contract.ReviewCaseV1, route contract.RouteV1) (contract.ReviewCaseV1, contract.ReviewRoundV1, contract.ReviewerAssignmentV1) {
	t.Helper()
	for _, state := range []contract.CaseStateV1{contract.CaseAdmittedV1, contract.CaseRoutedV1} {
		clock.Advance(time.Second)
		var err error
		c, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: c.TenantID, CaseID: c.ID, Expected: reviewport.ExpectedV1(c.Revision, c.Digest), Next: state}, Trace: testkit.TransitionTrace(clock.Now(), c, state)})
		if err != nil {
			t.Fatal(err)
		}
	}
	clock.Advance(time.Second)
	round := testkit.Round(clock.Now(), c, route)
	assignment := testkit.Assignment(clock.Now(), c, round, route)
	var err error
	c, _, assignment, err = engine.StartRoundV1(ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(c.Revision, c.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(clock.Now(), c, contract.TraceAssignedV1, 2, round.ID, assignment.ID)})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	c, assignment, err = engine.ClaimAssignmentV1(ctx, reviewport.ClaimAssignmentMutationV1{TenantID: c.TenantID, ExpectedCase: reviewport.ExpectedV1(c.Revision, c.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), CaseID: c.ID, AssignmentID: assignment.ID, LeaseHolder: "worker-a", LeaseExpiresUnixNano: clock.Now().Add(15 * time.Minute).UnixNano(), UpdatedUnixNano: clock.Now().UnixNano(), Traces: []contract.TraceFactV1{testkit.StartedTrace(clock.Now(), c, assignment.ID)}})
	if err != nil {
		t.Fatal(err)
	}
	return c, round, assignment
}
