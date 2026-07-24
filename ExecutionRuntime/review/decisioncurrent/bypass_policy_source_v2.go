package decisioncurrent

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/bypassowner"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// BypassPolicySourceV2 is the production-shaped Policy cut for new Bypass
// Decisions. V2 removes the Target/Policy digest cycle while preserving the
// exact three-field projection Ref persisted by BypassDecisionV1.
type BypassPolicySourceV2 struct {
	policy runtimeports.ReviewDecisionPolicyCurrentReaderV2
	clock  func() time.Time
}

func NewBypassPolicySourceV2(policy runtimeports.ReviewDecisionPolicyCurrentReaderV2, clock func() time.Time) (*BypassPolicySourceV2, error) {
	if nilcheck.IsNil(policy) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Bypass Policy V2 source requires the public Policy current Reader and clock")
	}
	return &BypassPolicySourceV2{policy: policy, clock: clock}, nil
}

func (s *BypassPolicySourceV2) ReadBypassCurrentV1(ctx context.Context, decision contract.BypassDecisionV1, ownerBaseline time.Time) (contract.BypassExternalCurrentProofV1, error) {
	if err := decision.Validate(); err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	baseline := s.clock()
	if baseline.IsZero() || (!ownerBaseline.IsZero() && baseline.Before(ownerBaseline)) {
		return contract.BypassExternalCurrentProofV1{}, clockRegressionV1("Bypass Policy V2 S1 clock is unavailable or regressed")
	}
	subject := bypassPolicySubjectV2(decision)
	if err := subject.Validate(); err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	recoveryCtx, recoveryCancel, recoveryReady := boundedDetachedRecoveryV1(ctx, baseline, decision.ExpiresUnixNano)
	if recoveryReady {
		defer recoveryCancel()
	}
	readCtx := ctx
	ref, detached, err := resolveBypassCurrentV5(readCtx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ReviewDecisionPolicyCurrentProjectionRefV2, error) {
		return s.policy.ResolveCurrentReviewDecisionPolicyV2(call, runtimeports.ReviewDecisionPolicyCurrentResolveRequestV2{Subject: subject})
	})
	if err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	if detached {
		readCtx = recoveryCtx
	}
	s1, detached, err := retryExactReadV1(readCtx, recoveryCtx, s.clock, func(call context.Context) (runtimeports.ReviewDecisionPolicyCurrentProjectionV2, error) {
		return s.policy.InspectCurrentReviewDecisionPolicyV2(call, subject, ref)
	})
	if err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	if detached {
		readCtx = recoveryCtx
	}
	afterS1 := s.clock()
	if afterS1.IsZero() || afterS1.Before(baseline) {
		return contract.BypassExternalCurrentProofV1{}, clockRegressionV1("Bypass Policy V2 clock regressed across S1")
	}
	if err := validateBypassPolicyProjectionV2(s1, ref, subject, decision, afterS1); err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	s2Ctx, s2Cancel, s2Ready := tightenDetachedRecoveryV1(recoveryCtx, afterS1, s1.ExpiresUnixNano)
	if !s2Ready {
		return contract.BypassExternalCurrentProofV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Bypass Policy V2 S1 snapshot expired before S2")
	}
	defer s2Cancel()
	if detached {
		readCtx = s2Ctx
	}
	s2, _, err := retryExactReadV1(readCtx, s2Ctx, s.clock, func(call context.Context) (runtimeports.ReviewDecisionPolicyCurrentProjectionV2, error) {
		return s.policy.InspectCurrentReviewDecisionPolicyV2(call, subject, ref)
	})
	if err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	now := s.clock()
	if now.IsZero() || now.Before(afterS1) {
		return contract.BypassExternalCurrentProofV1{}, clockRegressionV1("Bypass Policy V2 clock regressed across S2")
	}
	if err := validateBypassPolicyProjectionV2(s2, ref, subject, decision, now); err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return contract.BypassExternalCurrentProofV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Bypass Policy V2 projection drifted between S1 and S2")
	}
	proof, err := contract.SealBypassExternalCurrentProofV1(contract.BypassExternalCurrentProofV1{Policy: ref, CheckedUnixNano: s2.CheckedUnixNano, ExpiresUnixNano: s2.ExpiresUnixNano})
	if err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	if proof.Digest != decision.ExternalProof.Digest || proof.Policy != decision.PolicyCurrentProjection {
		return contract.BypassExternalCurrentProofV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Bypass Decision did not bind the exact Policy V2 cut")
	}
	return proof, nil
}

func bypassPolicySubjectV2(decision contract.BypassDecisionV1) runtimeports.ReviewDecisionPolicyApplicabilitySubjectV2 {
	return runtimeports.ReviewDecisionPolicyApplicabilitySubjectV2{TenantID: decision.TenantID, TargetID: decision.Target.ID, TargetRevision: decision.Target.Revision, IntentID: decision.IntentID, IntentRevision: decision.IntentRevision, IntentSubjectDigest: decision.SubjectDigest, PayloadRevision: decision.PayloadRevision, PayloadDigest: decision.PayloadDigest, RunID: decision.RunID, Scope: decision.Scope, CurrentScope: decision.CurrentScope, ActionScopeDigest: decision.ActionScopeDigest, ActorAuthority: decision.ActorAuthority, Policy: decision.Policy}
}

func validateBypassPolicyProjectionV2(p runtimeports.ReviewDecisionPolicyCurrentProjectionV2, ref runtimeports.ReviewDecisionPolicyCurrentProjectionRefV2, subject runtimeports.ReviewDecisionPolicyApplicabilitySubjectV2, decision contract.BypassDecisionV1, now time.Time) error {
	if err := p.ValidateCurrent(ref, subject, now); err != nil {
		return err
	}
	if !p.Fact.OperationNotRequired || p.Fact.PolicyDecisionRef != decision.PolicyDecisionRef || p.Fact.Ref != decision.Policy.Ref || p.Fact.Revision != decision.Policy.Revision || p.Fact.Digest != decision.Policy.Digest || !runtimeports.SameExecutionScopeV2(p.Fact.Scope, decision.Scope) || p.Fact.RunID != decision.RunID || p.Fact.CurrentScope != decision.CurrentScope || p.Fact.ActorAuthorityRef != decision.ActorAuthority.Ref || p.Fact.SubjectDigest != decision.SubjectDigest {
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "Policy V2 does not currently authorize this exact operation_not_required decision")
	}
	return nil
}

var _ bypassowner.ExternalCurrentCutV1 = (*BypassPolicySourceV2)(nil)
