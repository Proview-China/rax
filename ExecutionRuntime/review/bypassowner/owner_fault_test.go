package bypassowner_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/bypassowner"
	"github.com/Proview-China/rax/ExecutionRuntime/review/caseengine"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/storetestkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type staticBypassCurrentV1 struct {
	proof contract.BypassExternalCurrentProofV1
}

func (s staticBypassCurrentV1) ReadBypassCurrentV1(context.Context, contract.BypassDecisionV1, time.Time) (contract.BypassExternalCurrentProofV1, error) {
	return s.proof, nil
}

type blockingLostBypassStoreV1 struct {
	reviewport.StoreV1
	reviewport.BypassStoreV1
	creates atomic.Int32
}

func (s *blockingLostBypassStoreV1) CreateBypassDecisionV1(ctx context.Context, mutation reviewport.CreateBypassDecisionMutationV1) (contract.BypassDecisionV1, error) {
	s.creates.Add(1)
	if _, err := s.BypassStoreV1.CreateBypassDecisionV1(ctx, mutation); err != nil {
		return contract.BypassDecisionV1{}, err
	}
	return contract.BypassDecisionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "lost Bypass create reply")
}

func (s *blockingLostBypassStoreV1) InspectBypassDecisionExactV1(ctx context.Context, _ contract.BypassDecisionExactRefV1) (contract.BypassDecisionV1, error) {
	<-ctx.Done()
	return contract.BypassDecisionV1{}, ctx.Err()
}

func TestBypassOwnerLostReplyRecoveryIsBoundedAndPreservesOriginalUnknownV1(t *testing.T) {
	ctx := context.Background()
	base := time.Unix(1_960_000_000, 0)
	clock := testkit.NewClock(base)
	store := storetestkit.NewMemoryStoreV1(clock.Now)
	rubric := testkit.PublishRubric(ctx, store, clock.Now(), "tenant-a")
	target := testkit.Target(clock.Now())
	request := testkit.Request(clock.Now(), target, "case-bypass-owner")
	request.Rubric = rubric.ExactRef()
	request.Digest = ""
	request, _ = contract.SealReviewRequestV1(request)
	engine, err := caseengine.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
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
	checked := clock.Now()
	policyProjection := runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1{ID: target.Policy.Ref, Revision: target.Policy.Revision, Digest: target.Policy.Digest}
	proof, err := contract.SealBypassExternalCurrentProofV1(contract.BypassExternalCurrentProofV1{Policy: policyProjection, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: checked.Add(10 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := contract.SealBypassDecisionV1(contract.BypassDecisionV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "bypass-owner-decision", Revision: 1, CreatedUnixNano: checked.UnixNano(), UpdatedUnixNano: checked.UnixNano()},
		Target:         target.BypassExactRefV1(), Case: caseFact.BypassExactRefV1(), IntentID: target.IntentID, IntentRevision: target.IntentRevision,
		SubjectDigest: target.SubjectDigest, PayloadRevision: target.PayloadRevision, PayloadDigest: target.PayloadDigest, Scope: target.Scope, RunID: target.RunID,
		ActionScopeDigest: target.ActionScopeDigest, Policy: target.Policy, PolicyCurrentProjection: policyProjection, PolicyDecisionRef: "policy-decision-bypass-owner",
		ActorAuthority: target.ActorAuthority, CurrentScope: target.CurrentScope, TargetEvidenceSetDigest: target.EvidenceSetDigest,
		Profile: contract.ProfileYOLOV1, Risk: contract.RiskLowV1, EffectClass: contract.EffectObserveOnlyV1, Environment: contract.EnvironmentProductionV1,
		RouteDecisionDigest: testkit.Digest("bypass-owner-route"), ExternalProof: proof, State: contract.BypassDecisionActiveV1, ExpiresUnixNano: proof.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	wrapper := &blockingLostBypassStoreV1{StoreV1: store, BypassStoreV1: store}
	owner, err := bypassowner.New(wrapper, staticBypassCurrentV1{proof: proof}, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	bypassowner.SetRecoveryTimeoutForTestV1(owner, 10*time.Millisecond)
	started := time.Now()
	_, err = owner.CreateV1(ctx, reviewport.CreateBypassDecisionMutationV1{Decision: decision})
	if !core.HasCategory(err, core.ErrorIndeterminate) || !core.HasReason(err, core.ReasonEffectUnknownOutcome) {
		t.Fatalf("blocked recovery replaced the original Bypass Unknown: %v", err)
	}
	if elapsed := time.Since(started); elapsed >= 500*time.Millisecond || wrapper.creates.Load() != 1 {
		t.Fatalf("Bypass recovery blocked or repeated create: elapsed=%v creates=%d", elapsed, wrapper.creates.Load())
	}
	if stored, inspectErr := store.InspectBypassDecisionExactV1(ctx, decision.ExactRef()); inspectErr != nil || stored.Digest != decision.Digest {
		t.Fatalf("lost reply did not leave the exact committed Decision: stored=%+v err=%v", stored, inspectErr)
	}
}
