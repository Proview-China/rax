package review_test

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
	"github.com/Proview-China/rax/ExecutionRuntime/review/verdictowner"
)

type flow struct {
	ctx        context.Context
	clock      *testkit.ManualClock
	store      *memory.Store
	engine     *caseengine.Engine
	target     contract.TargetSnapshotV1
	caseValue  contract.ReviewCaseV1
	assignment contract.ReviewerAssignmentV1
	round      contract.ReviewRoundV1
	sequence   uint64
}

type resolvedFlow struct {
	*flow
	owner       *verdictowner.Owner
	resolved    contract.ReviewCaseV1
	verdict     contract.VerdictV1
	attestation contract.AttestationV1
}

func newVerdictOwner(t *testing.T, f *flow, mutate func(*reviewport.DecisionExternalCurrentProjectionV1)) *verdictowner.Owner {
	t.Helper()
	source, err := memory.NewDecisionCurrentSourceV1(f.store, &testkit.ExternalCurrentReader{Mutate: mutate}, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := verdictowner.New(f.store, source, f.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func newResolvedFlow(t *testing.T, verdictTTL time.Duration) *resolvedFlow {
	t.Helper()
	f := newReviewingFlow(t, contract.RouteHumanV1)
	f.clock.Advance(time.Second)
	att := testkit.HumanAttestation(f.clock.Now(), f.caseValue, f.round, f.assignment, contract.ResolutionAcceptV1, "idem-resolved")
	c, _, err := f.engine.RecordAttestationV1(f.ctx, reviewport.ExpectedV1(f.caseValue.Revision, f.caseValue.Digest), att, testkit.Trace(f.clock.Now(), f.caseValue, contract.TraceAttestedV1, 3, att.ID))
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	external := &testkit.ExternalCurrentReader{Mutate: func(value *reviewport.DecisionExternalCurrentProjectionV1) {
		expires := f.clock.Now().Add(verdictTTL).UnixNano()
		value.ExpiresUnixNano = expires
		value.ActorAuthority.ExpiresUnixNano = expires
		value.ReviewerAuthority.ExpiresUnixNano = expires
		value.Scope.ExpiresUnixNano = expires
		value.Binding.ExpiresUnixNano = expires
		for i := range value.Evidence {
			value.Evidence[i].ExpiresUnixNano = expires
		}
	}}
	source, _ := memory.NewDecisionCurrentSourceV1(f.store, external, f.clock.Now)
	owner, _ := verdictowner.New(f.store, source, f.clock.Now)
	resolved, v, err := owner.DecideV1(f.ctx, verdictowner.DecideCommandV1{TenantID: c.TenantID, CaseID: c.ID, Expected: reviewport.ExpectedV1(c.Revision, c.Digest), AttestationID: att.ID, VerdictID: "verdict-resolved", Trace: testkit.Trace(f.clock.Now(), c, contract.TraceVerdictV1, 4, "verdict-resolved")})
	if err != nil {
		t.Fatal(err)
	}
	return &resolvedFlow{flow: f, owner: owner, resolved: resolved, verdict: v, attestation: att}
}

func newReviewingFlow(t *testing.T, route contract.RouteV1) *flow {
	t.Helper()
	clock := testkit.NewClock(time.Unix(1_750_000_000, 0))
	target := testkit.Target(clock.Now())
	return newReviewingFlowFromTarget(t, route, clock, target)
}

func newReviewingFlowWithPolicyExpiry(t *testing.T, route contract.RouteV1, policyExpiresUnixNano int64) *flow {
	t.Helper()
	clock := testkit.NewClock(time.Unix(1_750_000_000, 0))
	target := testkit.TargetWithPolicyExpiry(clock.Now(), policyExpiresUnixNano)
	return newReviewingFlowFromTarget(t, route, clock, target)
}

func newReviewingFlowFromTarget(t *testing.T, route contract.RouteV1, clock *testkit.ManualClock, target contract.TargetSnapshotV1) *flow {
	t.Helper()
	ctx := context.Background()
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	testkit.PublishRubric(ctx, store, clock.Now(), target.TenantID)
	engine, _ := caseengine.New(store, clock.Now)
	request := testkit.Request(clock.Now(), target, "case-a")
	clock.Advance(time.Second)
	c, err := engine.CreateCaseV1(ctx, caseengine.CreateCaseCommandV1{CaseID: "case-a", Request: &request, Target: target, ExpiresUnixNano: request.ExpiresUnixNano, Trace: testkit.TraceForTarget(clock.Now(), "case-a", target, contract.TraceRequestedV1, 1, request.ID)})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []contract.CaseStateV1{contract.CaseAdmittedV1, contract.CaseRoutedV1} {
		clock.Advance(time.Second)
		c, err = engine.TransitionWithTraceV2(ctx, caseengine.TransitionWithTraceCommandV2{TransitionCommandV1: caseengine.TransitionCommandV1{TenantID: c.TenantID, CaseID: c.ID, Expected: reviewport.ExpectedV1(c.Revision, c.Digest), Next: state}, Trace: testkit.TransitionTrace(clock.Now(), c, state)})
		if err != nil {
			t.Fatal(err)
		}
	}
	clock.Advance(time.Second)
	round := testkit.Round(clock.Now(), c, route)
	assignment := testkit.Assignment(clock.Now(), c, round, route)
	c, _, assignment, err = engine.StartRoundV1(ctx, reviewport.StartRoundMutationV1{Expected: reviewport.ExpectedV1(c.Revision, c.Digest), Round: round, Assignment: assignment, Trace: testkit.Trace(clock.Now(), c, contract.TraceAssignedV1, 2, round.ID, assignment.ID)})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	c, assignment, err = engine.ClaimAssignmentV1(ctx, reviewport.ClaimAssignmentMutationV1{TenantID: c.TenantID, ExpectedCase: reviewport.ExpectedV1(c.Revision, c.Digest), ExpectedAssignment: reviewport.ExpectedV1(assignment.Revision, assignment.Digest), CaseID: c.ID, AssignmentID: assignment.ID, LeaseHolder: "worker-a", LeaseExpiresUnixNano: clock.Now().Add(15 * time.Minute).UnixNano(), UpdatedUnixNano: clock.Now().UnixNano(), Traces: []contract.TraceFactV1{testkit.StartedTrace(clock.Now(), c, assignment.ID)}})
	if err != nil {
		t.Fatal(err)
	}
	return &flow{ctx: ctx, clock: clock, store: store, engine: engine, target: target, caseValue: c, round: round, assignment: assignment, sequence: 3}
}
