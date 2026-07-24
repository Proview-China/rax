package decisioncurrent

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// BypassReviewFactReaderV5 exposes only the Review-owned exact/current facts
// needed to prove an operation_not_required decision. It has no mutation Port.
type BypassReviewFactReaderV5 interface {
	InspectTargetExactV1(context.Context, core.TenantID, reviewport.ExactFactRefV1) (contract.TargetSnapshotV1, error)
	InspectCaseV1(context.Context, core.TenantID, string) (contract.ReviewCaseV1, error)
	InspectCurrentBypassDecisionByCaseV1(context.Context, contract.BypassCaseExactRefV1) (contract.BypassDecisionV1, error)
}

type BypassCurrentSourceDependenciesV5 struct {
	Facts     BypassReviewFactReaderV5
	Policy    runtimeports.ReviewDecisionPolicyCurrentReaderV2
	Authority runtimeports.ReviewActorAuthorityCurrentReaderV2
	Scope     runtimeports.ReviewDecisionScopeCurrentReaderV1
	Binding   runtimeports.ProviderBindingCurrentnessPortV2
	Clock     func() time.Time
}

// BypassCurrentFactSourceV5 assembles one read-only S1/S2 cut. It does not
// create a Verdict, Runtime Authorization, Permit, Begin or provider effect.
type BypassCurrentFactSourceV5 struct {
	facts     BypassReviewFactReaderV5
	policy    runtimeports.ReviewDecisionPolicyCurrentReaderV2
	authority runtimeports.ReviewActorAuthorityCurrentReaderV2
	scope     runtimeports.ReviewDecisionScopeCurrentReaderV1
	binding   runtimeports.ProviderBindingCurrentnessPortV2
	clock     func() time.Time
}

func NewBypassCurrentFactSourceV5(d BypassCurrentSourceDependenciesV5) (*BypassCurrentFactSourceV5, error) {
	if nilcheck.IsNil(d.Facts) || nilcheck.IsNil(d.Policy) || nilcheck.IsNil(d.Authority) || nilcheck.IsNil(d.Scope) || nilcheck.IsNil(d.Binding) || d.Clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review V5 Bypass current source requires all read-only Owner dependencies")
	}
	return &BypassCurrentFactSourceV5{facts: d.Facts, policy: d.Policy, authority: d.Authority, scope: d.Scope, binding: d.Binding, clock: d.Clock}, nil
}

func (s *BypassCurrentFactSourceV5) InspectBypassCurrentFactsV5(ctx context.Context, request runtimeadapter.ExactCurrentRequestV5) (runtimeadapter.CurrentFactSnapshotV5, error) {
	if err := request.Validate(); err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	if request.Basis != runtimeports.OperationReviewBasisPolicyNotRequiredV5 {
		return runtimeadapter.CurrentFactSnapshotV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Review V5 Bypass source requires policy_not_required basis")
	}
	baseline := s.clock()
	if baseline.IsZero() {
		return runtimeadapter.CurrentFactSnapshotV5{}, clockRegressionV1("Review V5 Bypass source baseline clock is unavailable")
	}
	recoveryCtx, recoveryCancel, recoveryReady := boundedDetachedRecoveryV1(ctx, baseline, request.Intent.ExpiresUnixNano)
	if recoveryReady {
		defer recoveryCancel()
	}
	first, detached, err := s.readBypassCutV5(ctx, recoveryCtx, request)
	if err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	afterS1 := s.clock()
	if afterS1.IsZero() || afterS1.Before(baseline) {
		return runtimeadapter.CurrentFactSnapshotV5{}, clockRegressionV1("Review V5 Bypass source clock regressed across S1")
	}
	snapshotExpiry := minimumBypassCutExpiryV5(
		request.Intent.ExpiresUnixNano,
		first.target.ExpiresUnixNano,
		first.caseFact.ExpiresUnixNano,
		first.decision.ExpiresUnixNano,
		first.policy.ExpiresUnixNano,
		first.policy.Fact.ExpiresUnixNano,
		first.authority.ExpiresUnixNano,
		first.scope.ExpiresUnixNano,
		first.binding.ExpiresUnixNano,
	)
	s2Ctx, s2Cancel, s2Ready := tightenDetachedRecoveryV1(recoveryCtx, afterS1, snapshotExpiry)
	if !s2Ready {
		return runtimeadapter.CurrentFactSnapshotV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review V5 Bypass S1 snapshot expired before S2")
	}
	defer s2Cancel()
	readCtx := ctx
	if detached {
		readCtx = s2Ctx
	}
	second, _, err := s.readBypassCutV5(readCtx, s2Ctx, request)
	if err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	now := s.clock()
	if now.IsZero() || now.Before(afterS1) {
		return runtimeadapter.CurrentFactSnapshotV5{}, clockRegressionV1("Review V5 Bypass source clock regressed across S2")
	}
	if !reflect.DeepEqual(first, second) {
		return runtimeadapter.CurrentFactSnapshotV5{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Review V5 Bypass exact/current cut drifted between S1 and S2")
	}
	return projectBypassCutV5(request, second, now)
}

type bypassCompleteCutV5 struct {
	target    contract.TargetSnapshotV1
	caseFact  contract.ReviewCaseV1
	decision  contract.BypassDecisionV1
	policy    runtimeports.ReviewDecisionPolicyCurrentProjectionV2
	authority runtimeports.ReviewActorAuthorityCurrentProjectionV2
	scope     runtimeports.ReviewDecisionScopeCurrentProjectionV1
	binding   runtimeports.ProviderBindingCurrentProjectionV2
}

func (s *BypassCurrentFactSourceV5) readBypassCutV5(ctx, recoveryCtx context.Context, request runtimeadapter.ExactCurrentRequestV5) (bypassCompleteCutV5, bool, error) {
	detached := ctx == recoveryCtx && recoveryCtx != nil
	readCtx := ctx
	intent := request.Intent
	tenant := intent.Operation.ExecutionScope.Identity.TenantID
	target, recovered, err := retryValueV5(readCtx, recoveryCtx, s.clock, func(call context.Context) (contract.TargetSnapshotV1, error) {
		return s.facts.InspectTargetExactV1(call, tenant, reviewport.ExactV1(intent.Target, intent.Review.CandidateRevision, intent.Review.CandidateDigest))
	})
	if err != nil {
		return bypassCompleteCutV5{}, detached, err
	}
	if recovered && !detached {
		detached, readCtx = true, recoveryCtx
	}
	caseFact, recovered, err := retryValueV5(readCtx, recoveryCtx, s.clock, func(call context.Context) (contract.ReviewCaseV1, error) {
		return s.facts.InspectCaseV1(call, tenant, intent.Review.CaseRef)
	})
	if err != nil {
		return bypassCompleteCutV5{}, detached, err
	}
	if recovered && !detached {
		detached, readCtx = true, recoveryCtx
	}
	decision, recovered, err := retryValueV5(readCtx, recoveryCtx, s.clock, func(call context.Context) (contract.BypassDecisionV1, error) {
		return s.facts.InspectCurrentBypassDecisionByCaseV1(call, caseFact.BypassExactRefV1())
	})
	if err != nil {
		return bypassCompleteCutV5{}, detached, err
	}
	if recovered && !detached {
		detached, readCtx = true, recoveryCtx
	}
	targetRef := runtimeports.ReviewDecisionTargetRefV1{TenantID: target.TenantID, ID: target.ID, Revision: target.Revision, Digest: target.Digest, RunID: target.RunID}
	policySubject := runtimeports.ReviewDecisionPolicyApplicabilitySubjectV2{
		TenantID: tenant, TargetID: target.ID, TargetRevision: target.Revision,
		IntentID: decision.IntentID, IntentRevision: decision.IntentRevision, IntentSubjectDigest: decision.SubjectDigest,
		PayloadRevision: decision.PayloadRevision, PayloadDigest: decision.PayloadDigest,
		RunID: decision.RunID, Scope: decision.Scope, CurrentScope: decision.CurrentScope,
		ActionScopeDigest: decision.ActionScopeDigest, ActorAuthority: decision.ActorAuthority, Policy: decision.Policy,
	}
	policyRef, recovered, err := resolveBypassCurrentV5(readCtx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ReviewDecisionPolicyCurrentProjectionRefV2, error) {
		return s.policy.ResolveCurrentReviewDecisionPolicyV2(call, runtimeports.ReviewDecisionPolicyCurrentResolveRequestV2{Subject: policySubject})
	})
	if err != nil {
		return bypassCompleteCutV5{}, detached, err
	}
	if recovered && !detached {
		detached, readCtx = true, recoveryCtx
	}
	policy, recovered, err := retryValueV5(readCtx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ReviewDecisionPolicyCurrentProjectionV2, error) {
		return s.policy.InspectCurrentReviewDecisionPolicyV2(call, policySubject, policyRef)
	})
	if err != nil {
		return bypassCompleteCutV5{}, detached, err
	}
	if recovered && !detached {
		detached, readCtx = true, recoveryCtx
	}
	authoritySubject := runtimeports.ReviewActorAuthorityCurrentSubjectV2{Target: targetRef, ActorAuthority: decision.ActorAuthority, ActionScopeDigest: decision.ActionScopeDigest}
	authorityRef, recovered, err := resolveBypassCurrentV5(readCtx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ReviewActorAuthorityCurrentProjectionRefV2, error) {
		return s.authority.ResolveCurrentReviewActorAuthorityV2(call, runtimeports.ReviewActorAuthorityCurrentResolveRequestV2{Subject: authoritySubject})
	})
	if err != nil {
		return bypassCompleteCutV5{}, detached, err
	}
	if recovered && !detached {
		detached, readCtx = true, recoveryCtx
	}
	authority, recovered, err := retryValueV5(readCtx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ReviewActorAuthorityCurrentProjectionV2, error) {
		return s.authority.InspectCurrentReviewActorAuthorityV2(call, authoritySubject, authorityRef)
	})
	if err != nil {
		return bypassCompleteCutV5{}, detached, err
	}
	if recovered && !detached {
		detached, readCtx = true, recoveryCtx
	}
	scopeSubject := runtimeports.ReviewDecisionScopeCurrentSubjectV1{TenantID: tenant, Target: targetRef, RunID: target.RunID, Scope: decision.Scope, CurrentScope: decision.CurrentScope, ActionScopeDigest: decision.ActionScopeDigest}
	scopeRef, recovered, err := resolveBypassCurrentV5(readCtx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ReviewDecisionScopeCurrentProjectionRefV1, error) {
		return s.scope.ResolveCurrentReviewDecisionScopeV1(call, runtimeports.ReviewDecisionScopeCurrentResolveRequestV1{Subject: scopeSubject})
	})
	if err != nil {
		return bypassCompleteCutV5{}, detached, err
	}
	if recovered && !detached {
		detached, readCtx = true, recoveryCtx
	}
	scope, recovered, err := retryValueV5(readCtx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ReviewDecisionScopeCurrentProjectionV1, error) {
		return s.scope.InspectCurrentReviewDecisionScopeV1(call, scopeSubject, scopeRef)
	})
	if err != nil {
		return bypassCompleteCutV5{}, detached, err
	}
	if recovered && !detached {
		detached, readCtx = true, recoveryCtx
	}
	binding, recovered, err := retryValueV5(readCtx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ProviderBindingCurrentProjectionV2, error) {
		return s.binding.InspectProviderBindingCurrentV2(call, intent.Provider)
	})
	if err != nil {
		return bypassCompleteCutV5{}, detached, err
	}
	if recovered && !detached {
		detached = true
	}
	return bypassCompleteCutV5{target: target, caseFact: caseFact, decision: decision, policy: policy, authority: authority, scope: scope, binding: binding}, detached, nil
}

func resolveBypassCurrentV5[T any](ctx, recoveryCtx context.Context, clock func() time.Time, read func(context.Context) (T, error)) (T, bool, error) {
	value, err := singleReadV1(ctx, clock, read)
	if err != nil && unknownReadV1(err) {
		// Resolve has no expected full Ref. A detached retry is a new S1 lookup,
		// never a claim that the unknown prior result was recovered.
		originalUnknown := err
		if recoveryCtx == nil {
			var zero T
			return zero, false, originalUnknown
		}
		value, err = singleReadV1(recoveryCtx, clock, read)
		if err != nil || recoveryCtx.Err() != nil {
			if core.HasReason(err, core.ReasonClockRegression) {
				var zero T
				return zero, true, err
			}
			var zero T
			return zero, true, originalUnknown
		}
		return value, true, nil
	}
	return value, false, err
}

func validateBypassCompleteCutV5(request runtimeadapter.ExactCurrentRequestV5, cut bypassCompleteCutV5, now time.Time) error {
	intent, target, decision := request.Intent, cut.target, cut.decision
	if err := target.ValidateCurrent(contract.TargetCurrentnessV1{TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, PayloadRevision: target.PayloadRevision, PayloadDigest: target.PayloadDigest, Scope: target.Scope, ActionScopeDigest: target.ActionScopeDigest, Policy: target.Policy, ActorAuthority: target.ActorAuthority, CurrentScope: target.CurrentScope, EvidenceSetDigest: target.EvidenceSetDigest, ContextFrameDigest: target.ContextFrameDigest, Now: now}); err != nil {
		return err
	}
	if err := cut.caseFact.Validate(); err != nil {
		return err
	}
	if cut.caseFact.State != contract.CaseRoutedV1 || cut.caseFact.ID != intent.Review.CaseRef || cut.caseFact.TargetID != target.ID || cut.caseFact.TargetRevision != target.Revision || cut.caseFact.TargetDigest != target.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Bypass current Case drifted from exact Target or Intent")
	}
	if err := decision.ValidateCurrent(target.BypassExactRefV1(), cut.caseFact.BypassExactRefV1(), decision.PolicyCurrentProjection, now); err != nil {
		return err
	}
	targetRef := runtimeports.ReviewDecisionTargetRefV1{TenantID: target.TenantID, ID: target.ID, Revision: target.Revision, Digest: target.Digest, RunID: target.RunID}
	policySubject := runtimeports.ReviewDecisionPolicyApplicabilitySubjectV2{TenantID: target.TenantID, TargetID: target.ID, TargetRevision: target.Revision, IntentID: decision.IntentID, IntentRevision: decision.IntentRevision, IntentSubjectDigest: decision.SubjectDigest, PayloadRevision: decision.PayloadRevision, PayloadDigest: decision.PayloadDigest, RunID: decision.RunID, Scope: decision.Scope, CurrentScope: decision.CurrentScope, ActionScopeDigest: decision.ActionScopeDigest, ActorAuthority: decision.ActorAuthority, Policy: decision.Policy}
	if err := cut.policy.ValidateCurrent(decision.PolicyCurrentProjection, policySubject, now); err != nil {
		return err
	}
	if !cut.policy.Fact.OperationNotRequired || cut.policy.Fact.PolicyDecisionRef != decision.PolicyDecisionRef {
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "Policy does not currently authorize operation_not_required")
	}
	authoritySubject := runtimeports.ReviewActorAuthorityCurrentSubjectV2{Target: targetRef, ActorAuthority: decision.ActorAuthority, ActionScopeDigest: decision.ActionScopeDigest}
	if err := cut.authority.ValidateCurrent(cut.authority.Ref, authoritySubject, now); err != nil {
		return err
	}
	scopeSubject := runtimeports.ReviewDecisionScopeCurrentSubjectV1{TenantID: target.TenantID, Target: targetRef, RunID: target.RunID, Scope: decision.Scope, CurrentScope: decision.CurrentScope, ActionScopeDigest: decision.ActionScopeDigest}
	if err := cut.scope.ValidateCurrent(cut.scope.Ref, scopeSubject, now); err != nil {
		return err
	}
	if err := cut.binding.ValidateCurrent(intent.Provider, now); err != nil {
		return err
	}
	return nil
}

func projectBypassCutV5(request runtimeadapter.ExactCurrentRequestV5, cut bypassCompleteCutV5, now time.Time) (runtimeadapter.CurrentFactSnapshotV5, error) {
	if err := validateBypassCompleteCutV5(request, cut, now); err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	expires := minimumBypassCutExpiryV5(request.Intent.ExpiresUnixNano, cut.target.ExpiresUnixNano, cut.caseFact.ExpiresUnixNano, cut.decision.ExpiresUnixNano, cut.policy.ExpiresUnixNano, cut.policy.Fact.ExpiresUnixNano, cut.authority.ExpiresUnixNano, cut.scope.ExpiresUnixNano, cut.binding.ExpiresUnixNano)
	if expires <= now.UnixNano() {
		return runtimeadapter.CurrentFactSnapshotV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review V5 Bypass completed cut expired")
	}
	targetRef := contract.HumanTargetExactRefV2{TenantID: cut.target.TenantID, ID: cut.target.ID, Revision: cut.target.Revision, Digest: cut.target.Digest}
	policy, err := sealOwnerReceiptFromProjectionV5("policy", targetRef, nil, cut.decision.PolicyCurrentProjection.ID, cut.decision.PolicyCurrentProjection.Revision, cut.decision.PolicyCurrentProjection.Digest, cut.policy.Ref.ID, cut.policy.Ref.Revision, cut.policy.Ref.Digest, cut.decision.PolicyDecisionRef, true, cut.policy.CheckedUnixNano, cut.policy.ExpiresUnixNano, expires)
	if err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	policyDecision, err := sealOwnerReceiptFromProjectionV5("policy_decision", targetRef, nil, cut.policy.Fact.PolicyDecisionRef, cut.policy.Fact.Revision, cut.policy.Fact.Digest, cut.policy.Fact.PolicyDecisionRef, cut.policy.Fact.Revision, cut.policy.Fact.Digest, "", false, cut.policy.CheckedUnixNano, cut.policy.Fact.ExpiresUnixNano, expires)
	if err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	authority, err := sealOwnerReceiptFromProjectionV5("actor_authority", targetRef, nil, cut.decision.ActorAuthority.Ref, cut.decision.ActorAuthority.Revision, cut.decision.ActorAuthority.Digest, cut.authority.Ref.ID, cut.authority.Ref.Revision, cut.authority.Ref.Digest, "", false, cut.authority.CheckedUnixNano, cut.authority.ExpiresUnixNano, expires)
	if err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	scope, err := sealOwnerReceiptFromProjectionV5("scope", targetRef, nil, cut.decision.CurrentScope.Ref, cut.decision.CurrentScope.Revision, cut.decision.CurrentScope.Digest, cut.scope.Ref.ID, cut.scope.Ref.Revision, cut.scope.Ref.Digest, "", false, cut.scope.CheckedUnixNano, cut.scope.ExpiresUnixNano, expires)
	if err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	bindingSourceDigest, err := core.CanonicalJSONDigest("praxis.review.runtime-current", "praxis.review.runtime-current/v5", "ProviderBindingRefV2", request.Intent.Provider)
	if err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	binding, err := sealOwnerReceiptFromProjectionV5("binding", targetRef, nil, request.Intent.Provider.BindingSetID, request.Intent.Provider.BindingSetRevision, bindingSourceDigest, request.Intent.Provider.BindingSetID, request.Intent.Provider.BindingSetRevision, cut.binding.ProjectionDigest, "", false, cut.binding.IssuedUnixNano, cut.binding.ExpiresUnixNano, expires)
	if err != nil {
		return runtimeadapter.CurrentFactSnapshotV5{}, err
	}
	bypass := runtimeadapter.BypassCurrentSnapshotV5{CurrentCase: cut.caseFact, Decision: cut.decision, Policy: policy, PolicyDecision: policyDecision, Authority: authority, Scope: scope, Binding: binding}
	snapshot := runtimeadapter.CurrentFactSnapshotV5{Revision: cut.caseFact.Revision, Basis: request.Basis, Target: cut.target, PolicyNotRequired: &bypass, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires}
	return runtimeadapter.SealCurrentFactSnapshotV5(snapshot)
}

func minimumBypassCutExpiryV5(values ...int64) int64 {
	minimum := int64(0)
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}
