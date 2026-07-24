package decisioncurrent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestBypassCurrentFactSourceV5ProjectsOnlyExactPublicOwnerCut(t *testing.T) {
	fixture := newBypassCurrentFixtureV5(t)
	snapshot, err := fixture.source.InspectReviewCurrentFactsV5(context.Background(), runtimeadapter.ExactCurrentRequestV5{Intent: fixture.intent, Basis: runtimeports.OperationReviewBasisPolicyNotRequiredV5})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.PolicyNotRequired == nil || snapshot.Quorum != nil || snapshot.PolicyNotRequired.Decision.ID != fixture.decision.ID {
		t.Fatalf("Bypass source did not preserve the exact route: %+v", snapshot)
	}
	reader, err := runtimeadapter.NewReaderV5(fixture.source, fixture.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := reader.InspectOperationReviewCurrentV5(context.Background(), runtimeports.OperationReviewCurrentRequestV5{Intent: fixture.intent, Basis: runtimeports.OperationReviewBasisPolicyNotRequiredV5})
	if err != nil {
		t.Fatal(err)
	}
	if projection.PolicyNotRequired == nil || projection.PolicyNotRequired.BypassDecision.ID != fixture.decision.ID || projection.PolicyNotRequired.PolicyDecisionRef.Ref != fixture.decision.PolicyDecisionRef {
		t.Fatalf("Bypass Runtime V5 projection drifted: %+v", projection)
	}
	if fixture.policy.inspectCalls < 4 || fixture.authority.inspectCalls < 4 || fixture.scope.calls < 4 || fixture.binding.calls < 4 {
		t.Fatalf("Reader did not perform S1/S2 on every call: policy=%d authority=%d scope=%d binding=%d", fixture.policy.inspectCalls, fixture.authority.inspectCalls, fixture.scope.calls, fixture.binding.calls)
	}
}

func TestBypassCurrentFactSourceV5FailsClosedOnEveryOwnerS2Drift(t *testing.T) {
	for _, name := range []string{"policy", "authority", "scope", "binding"} {
		t.Run(name, func(t *testing.T) {
			fixture := newBypassCurrentFixtureV5(t)
			switch name {
			case "policy":
				fixture.policy.driftAt = 2
			case "authority":
				fixture.authority.driftAt = 2
			case "scope":
				fixture.scope.driftAt = 2
			case "binding":
				fixture.binding.driftAt = 2
			}
			snapshot, err := fixture.source.InspectReviewCurrentFactsV5(context.Background(), runtimeadapter.ExactCurrentRequestV5{Intent: fixture.intent, Basis: runtimeports.OperationReviewBasisPolicyNotRequiredV5})
			if err == nil || snapshot.Digest != "" {
				t.Fatalf("%s S2 drift reached a sealed snapshot: value=%+v err=%v", name, snapshot, err)
			}
		})
	}
}

func TestBypassCurrentFactSourceV5LostReplyTTLAndClockRollback(t *testing.T) {
	t.Run("lost exact inspect", func(t *testing.T) {
		fixture := newBypassCurrentFixtureV5(t)
		ctx, cancel := context.WithCancel(context.Background())
		fixture.policy.loseInspect, fixture.policy.cancel = true, cancel
		snapshot, err := fixture.source.InspectReviewCurrentFactsV5(ctx, runtimeadapter.ExactCurrentRequestV5{Intent: fixture.intent, Basis: runtimeports.OperationReviewBasisPolicyNotRequiredV5})
		if err != nil || snapshot.Digest == "" || ctx.Err() == nil || fixture.policy.inspectCalls != 3 {
			t.Fatalf("lost exact Policy reply was not recovered once without mutation: value=%+v err=%v calls=%d ctx=%v", snapshot, err, fixture.policy.inspectCalls, ctx.Err())
		}
	})
	t.Run("minimum TTL", func(t *testing.T) {
		fixture := newBypassCurrentFixtureV5(t)
		fixture.binding.value.ExpiresUnixNano = fixture.clock.Now().Add(30 * time.Second).UnixNano()
		fixture.binding.value, _ = runtimeports.SealProviderBindingCurrentProjectionV2(fixture.binding.value)
		snapshot, err := fixture.source.InspectReviewCurrentFactsV5(context.Background(), runtimeadapter.ExactCurrentRequestV5{Intent: fixture.intent, Basis: runtimeports.OperationReviewBasisPolicyNotRequiredV5})
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.ExpiresUnixNano != fixture.binding.value.ExpiresUnixNano {
			t.Fatalf("Bypass TTL did not use the shortest Owner input: got=%d want=%d", snapshot.ExpiresUnixNano, fixture.binding.value.ExpiresUnixNano)
		}
	})
	t.Run("clock rollback", func(t *testing.T) {
		fixture := newBypassCurrentFixtureV5(t)
		fixture.clock.rollbackAt = fixture.clock.calls + 2
		snapshot, err := fixture.source.InspectReviewCurrentFactsV5(context.Background(), runtimeadapter.ExactCurrentRequestV5{Intent: fixture.intent, Basis: runtimeports.OperationReviewBasisPolicyNotRequiredV5})
		if !core.HasReason(err, core.ReasonClockRegression) || snapshot.Digest != "" {
			t.Fatalf("clock rollback reached a sealed Bypass snapshot: value=%+v err=%v", snapshot, err)
		}
	})
}

type bypassCurrentFixtureV5 struct {
	clock     *humanExternalClockV2
	source    *CurrentFactSourceV5
	intent    runtimeports.OperationEffectIntentV3
	decision  contract.BypassDecisionV1
	policy    *bypassPolicyReaderV5
	authority *bypassAuthorityReaderV5
	scope     *humanScopeReaderV2
	binding   *bypassBindingReaderV5
}

func newBypassCurrentFixtureV5(t *testing.T) bypassCurrentFixtureV5 {
	t.Helper()
	human := newCurrentSourceFixtureV5(t)
	clock := human.clock
	now := clock.Now()
	baseFacts := human.source.facts.(*currentFactsReaderV5)
	target := baseFacts.target

	authorityFact, err := runtimeports.SealDispatchAuthorityFactV3(runtimeports.DispatchAuthorityFactV3{Ref: runtimeports.AuthorityBindingRefV2{Ref: "authority-bypass-v5", Revision: 1, Epoch: target.Scope.AuthorityEpoch}, Scope: target.Scope, RunID: target.RunID, ActionScopeDigest: target.ActionScopeDigest, State: runtimeports.AuthorityFactActive, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(8 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	target.ActorAuthority = authorityFact.Ref
	policyFact := runtimeports.ReviewPolicyFactV2{Ref: "policy-bypass-v5", Revision: 1, SubjectDigest: target.SubjectDigest, Scope: target.Scope, RunID: target.RunID, CurrentScope: target.CurrentScope, RiskClass: "review/low", ActorAuthorityRef: authorityFact.Ref.Ref, ReviewerAuthorityRef: "review/not-applicable", OperationNotRequired: true, PolicyDecisionRef: "policy-decision-bypass-v5", Active: true, ExpiresUnixNano: now.Add(7 * time.Minute).UnixNano()}
	policyFact.Digest, _ = policyFact.DigestV2()
	target.Policy = runtimeports.ReviewPolicyBindingRefV2{Ref: policyFact.Ref, Revision: policyFact.Revision, Digest: policyFact.Digest}
	target.Digest = ""
	target, err = contract.SealTargetSnapshotV1(target)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"operation":"human-review-v5"}`)
	intent := currentSourceIntentV5(t, target, payload, "case-bypass-current-v5")
	caseFact, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: intent.Review.CaseRef, Revision: 1, CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.UnixNano()}, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, State: contract.CaseRoutedV1, ExpiresUnixNano: now.Add(6 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	policySubject := runtimeports.ReviewDecisionPolicyApplicabilitySubjectV2{TenantID: target.TenantID, TargetID: target.ID, TargetRevision: target.Revision, IntentID: intent.ID, IntentRevision: intent.Revision, IntentSubjectDigest: target.SubjectDigest, PayloadRevision: target.PayloadRevision, PayloadDigest: target.PayloadDigest, RunID: target.RunID, Scope: target.Scope, CurrentScope: target.CurrentScope, ActionScopeDigest: target.ActionScopeDigest, ActorAuthority: target.ActorAuthority, Policy: target.Policy}
	policyProjection, err := runtimeports.SealReviewDecisionPolicyCurrentProjectionV2(runtimeports.ReviewDecisionPolicyCurrentProjectionV2{Ref: runtimeports.ReviewDecisionPolicyCurrentProjectionRefV2{Revision: 1}, Subject: policySubject, Fact: policyFact, State: runtimeports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := contract.SealBypassExternalCurrentProofV1(contract.BypassExternalCurrentProofV1{Policy: policyProjection.Ref, CheckedUnixNano: policyProjection.CheckedUnixNano, ExpiresUnixNano: policyProjection.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := contract.SealBypassDecisionV1(contract.BypassDecisionV1{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "bypass-current-v5", Revision: 1, CreatedUnixNano: now.Add(-time.Second).UnixNano(), UpdatedUnixNano: now.UnixNano()}, Target: target.BypassExactRefV1(), Case: caseFact.BypassExactRefV1(), IntentID: intent.ID, IntentRevision: intent.Revision, SubjectDigest: target.SubjectDigest, PayloadRevision: target.PayloadRevision, PayloadDigest: target.PayloadDigest, Scope: target.Scope, RunID: target.RunID, ActionScopeDigest: target.ActionScopeDigest, Policy: target.Policy, PolicyCurrentProjection: policyProjection.Ref, PolicyDecisionRef: policyFact.PolicyDecisionRef, ActorAuthority: target.ActorAuthority, CurrentScope: target.CurrentScope, TargetEvidenceSetDigest: target.EvidenceSetDigest, Profile: contract.ProfileYOLOV1, Risk: contract.RiskLowV1, EffectClass: contract.EffectObserveOnlyV1, Environment: contract.EnvironmentProductionV1, RouteDecisionDigest: externalDigestV1("route-bypass-current-v5"), ExternalProof: proof, State: contract.BypassDecisionActiveV1, ExpiresUnixNano: policyProjection.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	targetRef := runtimeports.ReviewDecisionTargetRefV1{TenantID: target.TenantID, ID: target.ID, Revision: target.Revision, Digest: target.Digest, RunID: target.RunID}
	authoritySubject := runtimeports.ReviewActorAuthorityCurrentSubjectV2{Target: targetRef, ActorAuthority: target.ActorAuthority, ActionScopeDigest: target.ActionScopeDigest}
	authorityProjection, err := runtimeports.SealReviewActorAuthorityCurrentProjectionV2(runtimeports.ReviewActorAuthorityCurrentProjectionV2{Ref: runtimeports.ReviewActorAuthorityCurrentProjectionRefV2{Revision: 1}, Subject: authoritySubject, Fact: authorityFact, State: runtimeports.ReviewDecisionGovernanceProjectionActiveV1, Current: true, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(4 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	oldScope := human.source.scope.(*humanScopeReaderV2).value.Fact
	scopeSubject := runtimeports.ReviewDecisionScopeCurrentSubjectV1{TenantID: target.TenantID, Target: targetRef, RunID: target.RunID, Scope: target.Scope, CurrentScope: target.CurrentScope, ActionScopeDigest: target.ActionScopeDigest}
	scopeProjection := sealExternalScopeV1(t, scopeSubject, oldScope, now.Add(-time.Second), now.Add(3*time.Minute))
	bindingProjection, err := runtimeports.SealProviderBindingCurrentProjectionV2(runtimeports.ProviderBindingCurrentProjectionV2{ContractVersion: runtimeports.ProviderBindingCurrentnessContractVersionV2, Ref: intent.Provider, State: runtimeports.ProviderBindingCurrentActiveV2, BindingSetDigest: externalDigestV1("binding-set-bypass-v5"), BindingSetSemanticDigest: externalDigestV1("binding-set-semantic-bypass-v5"), BindingID: "binding-bypass-v5", BindingRevision: 1, GrantDigest: externalDigestV1("binding-grant-bypass-v5"), IssuedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(2 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	facts := &bypassFactsReaderV5{target: target, caseFact: caseFact, decision: decision}
	policyReader := &bypassPolicyReaderV5{value: policyProjection}
	authorityReader := &bypassAuthorityReaderV5{value: authorityProjection}
	scopeReader := &humanScopeReaderV2{value: scopeProjection}
	bindingReader := &bypassBindingReaderV5{value: bindingProjection}
	bypass, err := NewBypassCurrentFactSourceV5(BypassCurrentSourceDependenciesV5{Facts: facts, Policy: policyReader, Authority: authorityReader, Scope: scopeReader, Binding: bindingReader, Clock: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	human.source.bypass = bypass
	return bypassCurrentFixtureV5{clock: clock, source: human.source, intent: intent, decision: decision, policy: policyReader, authority: authorityReader, scope: scopeReader, binding: bindingReader}
}

type bypassFactsReaderV5 struct {
	target   contract.TargetSnapshotV1
	caseFact contract.ReviewCaseV1
	decision contract.BypassDecisionV1
}

func (r *bypassFactsReaderV5) InspectTargetExactV1(_ context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.TargetSnapshotV1, error) {
	if tenant != r.target.TenantID || ref != reviewport.ExactV1(r.target.ID, r.target.Revision, r.target.Digest) {
		return contract.TargetSnapshotV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Bypass Target exact ref drifted")
	}
	value := r.target
	value.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), value.Evidence...)
	return value, nil
}
func (r *bypassFactsReaderV5) InspectCaseV1(_ context.Context, tenant core.TenantID, id string) (contract.ReviewCaseV1, error) {
	if tenant != r.caseFact.TenantID || id != r.caseFact.ID {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Bypass Case absent")
	}
	return r.caseFact, nil
}
func (r *bypassFactsReaderV5) InspectCurrentBypassDecisionByCaseV1(_ context.Context, ref contract.BypassCaseExactRefV1) (contract.BypassDecisionV1, error) {
	if ref != r.caseFact.BypassExactRefV1() {
		return contract.BypassDecisionV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Bypass Decision Case drifted")
	}
	return r.decision, nil
}

type bypassPolicyReaderV5 struct {
	mu                    sync.Mutex
	value                 runtimeports.ReviewDecisionPolicyCurrentProjectionV2
	inspectCalls, driftAt int
	loseInspect           bool
	cancel                context.CancelFunc
}

func (r *bypassPolicyReaderV5) ResolveCurrentReviewDecisionPolicyV2(_ context.Context, request runtimeports.ReviewDecisionPolicyCurrentResolveRequestV2) (runtimeports.ReviewDecisionPolicyCurrentProjectionRefV2, error) {
	if !runtimeports.SameReviewDecisionPolicyApplicabilitySubjectV2(request.Subject, r.value.Subject) {
		return runtimeports.ReviewDecisionPolicyCurrentProjectionRefV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Bypass Policy subject drifted")
	}
	return r.value.Ref, nil
}
func (r *bypassPolicyReaderV5) InspectCurrentReviewDecisionPolicyV2(ctx context.Context, subject runtimeports.ReviewDecisionPolicyApplicabilitySubjectV2, ref runtimeports.ReviewDecisionPolicyCurrentProjectionRefV2) (runtimeports.ReviewDecisionPolicyCurrentProjectionV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inspectCalls++
	if r.loseInspect && r.inspectCalls == 1 {
		if r.cancel != nil {
			r.cancel()
		}
		return runtimeports.ReviewDecisionPolicyCurrentProjectionV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "lost Bypass Policy reply")
	}
	if err := ctx.Err(); err != nil {
		return runtimeports.ReviewDecisionPolicyCurrentProjectionV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "Bypass Policy context ended")
	}
	if r.driftAt > 0 && r.inspectCalls >= r.driftAt {
		return runtimeports.ReviewDecisionPolicyCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Bypass Policy current drifted")
	}
	if ref != r.value.Ref || !runtimeports.SameReviewDecisionPolicyApplicabilitySubjectV2(subject, r.value.Subject) {
		return runtimeports.ReviewDecisionPolicyCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Bypass Policy exact current drifted")
	}
	return r.value.Clone(), nil
}
func (r *bypassPolicyReaderV5) InspectHistoricalReviewDecisionPolicyV2(context.Context, runtimeports.ReviewDecisionPolicyCurrentProjectionRefV2) (runtimeports.ReviewDecisionPolicyCurrentProjectionV2, error) {
	return r.value.Clone(), nil
}

type bypassAuthorityReaderV5 struct {
	mu                    sync.Mutex
	value                 runtimeports.ReviewActorAuthorityCurrentProjectionV2
	inspectCalls, driftAt int
}

func (r *bypassAuthorityReaderV5) ResolveCurrentReviewActorAuthorityV2(_ context.Context, request runtimeports.ReviewActorAuthorityCurrentResolveRequestV2) (runtimeports.ReviewActorAuthorityCurrentProjectionRefV2, error) {
	if request.Subject != r.value.Subject {
		return runtimeports.ReviewActorAuthorityCurrentProjectionRefV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Bypass Authority subject drifted")
	}
	return r.value.Ref, nil
}
func (r *bypassAuthorityReaderV5) InspectCurrentReviewActorAuthorityV2(_ context.Context, subject runtimeports.ReviewActorAuthorityCurrentSubjectV2, ref runtimeports.ReviewActorAuthorityCurrentProjectionRefV2) (runtimeports.ReviewActorAuthorityCurrentProjectionV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inspectCalls++
	if r.driftAt > 0 && r.inspectCalls >= r.driftAt {
		return runtimeports.ReviewActorAuthorityCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Bypass Authority current drifted")
	}
	if subject != r.value.Subject || ref != r.value.Ref {
		return runtimeports.ReviewActorAuthorityCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Bypass Authority exact current drifted")
	}
	return r.value.Clone(), nil
}
func (r *bypassAuthorityReaderV5) InspectHistoricalReviewActorAuthorityV2(context.Context, runtimeports.ReviewActorAuthorityCurrentProjectionRefV2) (runtimeports.ReviewActorAuthorityCurrentProjectionV2, error) {
	return r.value.Clone(), nil
}

type bypassBindingReaderV5 struct {
	mu             sync.Mutex
	value          runtimeports.ProviderBindingCurrentProjectionV2
	calls, driftAt int
}

func (r *bypassBindingReaderV5) InspectProviderBindingCurrentV2(_ context.Context, expected runtimeports.ProviderBindingRefV2) (runtimeports.ProviderBindingCurrentProjectionV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.driftAt > 0 && r.calls >= r.driftAt {
		return runtimeports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Bypass Provider Binding current drifted")
	}
	if expected != r.value.Ref {
		return runtimeports.ProviderBindingCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Bypass Provider Binding exact ref drifted")
	}
	return r.value, nil
}
