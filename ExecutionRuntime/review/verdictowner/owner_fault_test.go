package verdictowner

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/caseengine"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type blockingLostDecideStoreV1 struct {
	reviewport.StoreV1
}

type flipDecisionCurrentV1 struct {
	inner reviewport.DecisionCurrentReaderV1
	flip  func()
}

func (r *flipDecisionCurrentV1) InspectDecisionCurrentV1(ctx context.Context, request reviewport.DecisionCurrentRequestV1) (contract.DecisionCurrentSnapshotV1, error) {
	value, err := r.inner.InspectDecisionCurrentV1(ctx, request)
	if err == nil {
		r.flip()
	}
	return value, err
}

func (s *blockingLostDecideStoreV1) DecideV1(ctx context.Context, mutation reviewport.DecideMutationV1) (contract.ReviewCaseV1, contract.VerdictV1, error) {
	if _, _, err := s.StoreV1.DecideV1(ctx, mutation); err != nil {
		return contract.ReviewCaseV1{}, contract.VerdictV1{}, err
	}
	return contract.ReviewCaseV1{}, contract.VerdictV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "committed Decide reply was lost")
}

func (s *blockingLostDecideStoreV1) InspectVerdictExactV1(ctx context.Context, _ core.TenantID, _ reviewport.ExactFactRefV1) (contract.VerdictV1, error) {
	<-ctx.Done()
	return contract.VerdictV1{}, ctx.Err()
}

func TestDecideLostReplyRecoveryReaderNeverReturnsIsBoundedV1(t *testing.T) {
	ctx := context.Background()
	clock := testkit.NewClock(time.Unix(1_970_000_000, 0))
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	testkit.PublishRubric(ctx, store, clock.Now(), "tenant-a")
	engine, err := caseengine.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(clock.Now())
	request := testkit.Request(clock.Now(), target, "case-bounded-recovery")
	clock.Advance(time.Second)
	caseFact, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-bounded-recovery", Request: &request, Target: target, ExpiresUnixNano: request.ExpiresUnixNano, Trace: testkit.TraceForTarget(clock.Now(), "case-bounded-recovery", target, contract.TraceRequestedV1, 1, request.ID)})
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
	clock.Advance(time.Second)
	round := testkit.Round(clock.Now(), caseFact, contract.RouteHumanV1)
	assignment := testkit.Assignment(clock.Now(), caseFact, round, contract.RouteHumanV1)
	caseFact, round, assignment, err = engine.StartRoundV1(ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(clock.Now(), caseFact, contract.TraceAssignedV1, 2, round.ID, assignment.ID)})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	caseFact, assignment, err = engine.ClaimAssignmentV1(ctx, reviewport.ClaimAssignmentMutationV1{TenantID: caseFact.TenantID, CaseID: caseFact.ID, AssignmentID: assignment.ID, ExpectedCase: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), LeaseHolder: "reviewer-a", LeaseExpiresUnixNano: clock.Now().Add(15 * time.Minute).UnixNano(), UpdatedUnixNano: clock.Now().UnixNano(), Traces: []contract.TraceFactV1{testkit.StartedTrace(clock.Now(), caseFact, assignment.ID)}})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	attestation := testkit.HumanAttestation(clock.Now(), caseFact, round, assignment, contract.ResolutionAcceptV1, "idem-bounded-recovery")
	caseFact, _, err = engine.RecordAttestationV1(ctx, reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), attestation, testkit.Trace(clock.Now(), caseFact, contract.TraceAttestedV1, 3, attestation.ID))
	if err != nil {
		t.Fatal(err)
	}
	current, err := memory.NewDecisionCurrentSourceV1(store, &testkit.ExternalCurrentReader{}, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := New(&blockingLostDecideStoreV1{StoreV1: store}, current, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	owner.recoveryTimeout = 10 * time.Millisecond
	clock.Advance(time.Second)
	started := time.Now()
	_, _, err = owner.DecideV1(ctx, DecideCommandV1{TenantID: caseFact.TenantID, CaseID: caseFact.ID, Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), AttestationID: attestation.ID, VerdictID: "verdict-bounded-recovery", Trace: testkit.Trace(clock.Now(), caseFact, contract.TraceVerdictV1, 4, "verdict-bounded-recovery")})
	if !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("bounded recovery lost original unknown outcome: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("non-returning exact Inspect exceeded bounded recovery: %s", elapsed)
	}
}

func TestDecideRubricStoreActualPointFailsClosedV1(t *testing.T) {
	for _, test := range []struct {
		name       string
		flip       func(*testkit.ManualClock, contract.RubricDefinitionV1)
		wantReason core.ReasonCode
	}{
		{name: "ttl_crossing", flip: func(clock *testkit.ManualClock, rubric contract.RubricDefinitionV1) {
			clock.Set(time.Unix(0, rubric.ExpiresUnixNano).Add(time.Nanosecond))
		}, wantReason: core.ReasonReviewVerdictStale},
		{name: "clock_rollback", flip: func(clock *testkit.ManualClock, _ contract.RubricDefinitionV1) {
			clock.Set(time.Unix(1_971_000_000, 0))
		}, wantReason: core.ReasonClockRegression},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			base := time.Unix(1_971_000_000, 0)
			ownerClock := testkit.NewClock(base)
			storeClock := testkit.NewClock(base)
			store := storetestkit.NewMemoryStoreV1(storeClock.Now)
			rubric := testkit.PublishRubric(ctx, store, base, "tenant-a")
			engine, _ := caseengine.New(store, ownerClock.Now)
			target := testkit.Target(base)
			request := testkit.Request(base, target, "case-decision-actual-"+test.name)
			ownerClock.Advance(time.Second)
			storeClock.Set(ownerClock.Now())
			caseFact, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: request.CaseID, Request: &request, Target: target, ExpiresUnixNano: request.ExpiresUnixNano, Trace: testkit.TraceForTarget(ownerClock.Now(), request.CaseID, target, contract.TraceRequestedV1, 1, request.ID)})
			if err != nil {
				t.Fatal(err)
			}
			for _, state := range []contract.CaseStateV1{contract.CaseAdmittedV1, contract.CaseRoutedV1} {
				ownerClock.Advance(time.Second)
				caseFact, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: caseFact.TenantID, CaseID: caseFact.ID, Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), Next: state}, Trace: testkit.TransitionTrace(ownerClock.Now(), caseFact, state)})
				if err != nil {
					t.Fatal(err)
				}
			}
			ownerClock.Advance(time.Second)
			storeClock.Set(ownerClock.Now())
			round := testkit.Round(ownerClock.Now(), caseFact, contract.RouteHumanV1)
			assignment := testkit.Assignment(ownerClock.Now(), caseFact, round, contract.RouteHumanV1)
			caseFact, round, assignment, err = engine.StartRoundV1(ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(ownerClock.Now(), caseFact, contract.TraceAssignedV1, 2, round.ID, assignment.ID)})
			if err != nil {
				t.Fatal(err)
			}
			ownerClock.Advance(time.Second)
			caseFact, assignment, err = engine.ClaimAssignmentV1(ctx, reviewport.ClaimAssignmentMutationV1{TenantID: caseFact.TenantID, CaseID: caseFact.ID, AssignmentID: assignment.ID, ExpectedCase: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), LeaseHolder: "reviewer-a", LeaseExpiresUnixNano: ownerClock.Now().Add(15 * time.Minute).UnixNano(), UpdatedUnixNano: ownerClock.Now().UnixNano(), Traces: []contract.TraceFactV1{testkit.StartedTrace(ownerClock.Now(), caseFact, assignment.ID)}})
			if err != nil {
				t.Fatal(err)
			}
			ownerClock.Advance(time.Second)
			attestation := testkit.HumanAttestation(ownerClock.Now(), caseFact, round, assignment, contract.ResolutionAcceptV1, "idem-decision-actual-"+test.name)
			caseFact, _, err = engine.RecordAttestationV1(ctx, reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), attestation, testkit.Trace(ownerClock.Now(), caseFact, contract.TraceAttestedV1, 3, attestation.ID))
			if err != nil {
				t.Fatal(err)
			}
			storeClock.Set(ownerClock.Now())
			current, err := memory.NewDecisionCurrentSourceV1(store, &testkit.ExternalCurrentReader{}, ownerClock.Now)
			if err != nil {
				t.Fatal(err)
			}
			reader := &flipDecisionCurrentV1{inner: current, flip: func() { test.flip(storeClock, rubric) }}
			owner, _ := New(store, reader, ownerClock.Now)
			ownerClock.Advance(time.Second)
			verdictID := "verdict-decision-actual-" + test.name
			_, _, err = owner.DecideV1(ctx, DecideCommandV1{TenantID: caseFact.TenantID, CaseID: caseFact.ID, Expected: reviewport.ExpectedV1(caseFact.Revision, caseFact.Digest), AttestationID: attestation.ID, VerdictID: verdictID, Trace: testkit.Trace(ownerClock.Now(), caseFact, contract.TraceVerdictV1, 4, verdictID)})
			if !core.HasReason(err, test.wantReason) {
				t.Fatalf("Decide Store actual-point %s was admitted: %v", test.name, err)
			}
			if _, err := store.InspectVerdictV1(ctx, caseFact.TenantID, verdictID); !core.HasCategory(err, core.ErrorNotFound) {
				t.Fatalf("failed Decide leaked Verdict: %v", err)
			}
		})
	}
}
