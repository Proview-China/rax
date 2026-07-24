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

// BypassPolicySourceV1 reads only the Policy Owner's immutable current
// projection. Authority, Scope, Binding, Budget and Evidence remain Runtime
// Gateway conditions and are deliberately absent from this Review reader.
type BypassPolicySourceV1 struct {
	policy runtimeports.ReviewDecisionPolicyCurrentReaderV1
	clock  func() time.Time
}

func NewBypassPolicySourceV1(policy runtimeports.ReviewDecisionPolicyCurrentReaderV1, clock func() time.Time) (*BypassPolicySourceV1, error) {
	if nilcheck.IsNil(policy) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Bypass Policy source requires the public Policy current Reader and clock")
	}
	return &BypassPolicySourceV1{policy: policy, clock: clock}, nil
}

func (s *BypassPolicySourceV1) ReadBypassCurrentV1(ctx context.Context, decision contract.BypassDecisionV1, ownerBaseline time.Time) (contract.BypassExternalCurrentProofV1, error) {
	if err := decision.Validate(); err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	baseline := s.clock()
	if baseline.IsZero() || (!ownerBaseline.IsZero() && baseline.Before(ownerBaseline)) {
		return contract.BypassExternalCurrentProofV1{}, clockRegressionV1("Bypass Policy S1 clock is unavailable or regressed")
	}
	target := runtimeports.ReviewDecisionTargetRefV1{TenantID: decision.TenantID, ID: decision.Target.ID, Revision: decision.Target.Revision, Digest: decision.Target.Digest, RunID: decision.RunID}
	subject := runtimeports.ReviewDecisionPolicyCurrentSubjectV1{Target: target, Policy: decision.Policy}
	if err := subject.Validate(); err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	recoveryCtx, recoveryCancel, recoveryReady := boundedDetachedRecoveryV1(ctx, baseline, decision.ExpiresUnixNano)
	if recoveryReady {
		defer recoveryCancel()
	}
	readCtx := ctx
	ref, err := singleReadV1(readCtx, s.clock, func(callCtx context.Context) (runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1, error) {
		return s.policy.ResolveCurrentReviewDecisionPolicyV1(callCtx, runtimeports.ReviewDecisionPolicyCurrentResolveRequestV1{Subject: subject})
	})
	if err != nil && unknownReadV1(err) {
		// Resolve has no expected Ref. This is a new S1, not recovery of the
		// unknown result.
		originalUnknown := err
		if !recoveryReady {
			return contract.BypassExternalCurrentProofV1{}, originalUnknown
		}
		readCtx = recoveryCtx
		ref, err = singleReadV1(readCtx, s.clock, func(callCtx context.Context) (runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1, error) {
			return s.policy.ResolveCurrentReviewDecisionPolicyV1(callCtx, runtimeports.ReviewDecisionPolicyCurrentResolveRequestV1{Subject: subject})
		})
		if err != nil {
			if core.HasReason(err, core.ReasonClockRegression) {
				return contract.BypassExternalCurrentProofV1{}, err
			}
			return contract.BypassExternalCurrentProofV1{}, originalUnknown
		}
	}
	if err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	s1, detached, err := retryExactReadV1(readCtx, recoveryCtx, s.clock, func(callCtx context.Context) (runtimeports.ReviewDecisionPolicyCurrentProjectionV1, error) {
		return s.policy.InspectCurrentReviewDecisionPolicyV1(callCtx, subject, ref)
	})
	if err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	if detached {
		readCtx = recoveryCtx
	}
	afterS1 := s.clock()
	if afterS1.IsZero() || afterS1.Before(baseline) {
		return contract.BypassExternalCurrentProofV1{}, clockRegressionV1("Bypass Policy clock regressed across S1")
	}
	if err := validateBypassPolicyProjectionV1(s1, ref, subject, decision, afterS1); err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	s2Ctx, s2Cancel, s2Ready := tightenDetachedRecoveryV1(recoveryCtx, afterS1, s1.ExpiresUnixNano)
	if !s2Ready {
		return contract.BypassExternalCurrentProofV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Bypass Policy S1 snapshot expired before S2")
	}
	defer s2Cancel()
	if detached {
		readCtx = s2Ctx
	}
	s2, _, err := retryExactReadV1(readCtx, s2Ctx, s.clock, func(callCtx context.Context) (runtimeports.ReviewDecisionPolicyCurrentProjectionV1, error) {
		return s.policy.InspectCurrentReviewDecisionPolicyV1(callCtx, subject, ref)
	})
	if err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	now := s.clock()
	if now.IsZero() || now.Before(afterS1) {
		return contract.BypassExternalCurrentProofV1{}, clockRegressionV1("Bypass Policy clock regressed across S2")
	}
	if err := validateBypassPolicyProjectionV1(s2, ref, subject, decision, now); err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return contract.BypassExternalCurrentProofV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Bypass Policy projection drifted between S1 and S2")
	}
	proof, err := contract.SealBypassExternalCurrentProofV1(contract.BypassExternalCurrentProofV1{Policy: ref, CheckedUnixNano: s2.CheckedUnixNano, ExpiresUnixNano: s2.ExpiresUnixNano})
	if err != nil {
		return contract.BypassExternalCurrentProofV1{}, err
	}
	if proof.Digest != decision.ExternalProof.Digest {
		return contract.BypassExternalCurrentProofV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Bypass Decision did not bind the exact Policy read cut")
	}
	return proof, nil
}

func validateBypassPolicyProjectionV1(p runtimeports.ReviewDecisionPolicyCurrentProjectionV1, ref runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1, subject runtimeports.ReviewDecisionPolicyCurrentSubjectV1, decision contract.BypassDecisionV1, now time.Time) error {
	if err := p.ValidateCurrent(ref, subject, now); err != nil {
		return err
	}
	if !p.Fact.OperationNotRequired || p.Fact.PolicyDecisionRef != decision.PolicyDecisionRef || p.Fact.Ref != decision.Policy.Ref || p.Fact.Revision != decision.Policy.Revision || p.Fact.Digest != decision.Policy.Digest || p.Fact.Scope != decision.Scope || p.Fact.RunID != decision.RunID || p.Fact.CurrentScope != decision.CurrentScope || p.Fact.ActorAuthorityRef != decision.ActorAuthority.Ref {
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "Policy does not currently authorize this exact not-required decision")
	}
	return nil
}

var _ bypassowner.ExternalCurrentCutV1 = (*BypassPolicySourceV1)(nil)
