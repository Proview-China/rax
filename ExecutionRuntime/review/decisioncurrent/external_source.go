package decisioncurrent

import (
	"context"
	"reflect"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ExternalSourceV1 is the production-shaped, read-only REV-D11 aggregator.
// It owns no Policy, Authority, Scope, Binding or Evidence state and receives
// no mutation capability from those Owners.
type ExternalSourceV1 struct {
	binding   runtimeports.ReviewBindingAuthoritativeCurrentReaderV1
	evidence  runtimeports.ReviewEvidenceApplicabilityCurrentReaderV1
	policy    runtimeports.ReviewDecisionPolicyCurrentReaderV1
	authority runtimeports.ReviewDecisionAuthorityCurrentReaderV1
	scope     runtimeports.ReviewDecisionScopeCurrentReaderV1
	clock     func() time.Time
}

func NewExternalSourceV1(
	binding runtimeports.ReviewBindingAuthoritativeCurrentReaderV1,
	evidence runtimeports.ReviewEvidenceApplicabilityCurrentReaderV1,
	policy runtimeports.ReviewDecisionPolicyCurrentReaderV1,
	authority runtimeports.ReviewDecisionAuthorityCurrentReaderV1,
	scope runtimeports.ReviewDecisionScopeCurrentReaderV1,
	clock func() time.Time,
) (*ExternalSourceV1, error) {
	if nilcheck.IsNil(binding) || nilcheck.IsNil(evidence) || nilcheck.IsNil(policy) || nilcheck.IsNil(authority) || nilcheck.IsNil(scope) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "REV-D11 external current source requires five Owner readers and a clock")
	}
	return &ExternalSourceV1{binding: binding, evidence: evidence, policy: policy, authority: authority, scope: scope, clock: clock}, nil
}

type externalSubjectsV1 struct {
	policy            runtimeports.ReviewDecisionPolicyCurrentSubjectV1
	actorAuthority    runtimeports.ReviewDecisionAuthorityCurrentSubjectV1
	reviewerAuthority runtimeports.ReviewDecisionAuthorityCurrentSubjectV1
	scope             runtimeports.ReviewDecisionScopeCurrentSubjectV1
	binding           runtimeports.ReviewBindingSubjectV1
	evidence          []runtimeports.ReviewEvidenceApplicabilitySubjectV1
}

type externalRefsV1 struct {
	policy            runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1
	actorAuthority    runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1
	reviewerAuthority runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1
	scope             runtimeports.ReviewDecisionScopeCurrentProjectionRefV1
	binding           runtimeports.ReviewBindingProjectionRefV1
	evidence          []runtimeports.ReviewEvidenceApplicabilityRefV1
}

type externalCutV1 struct {
	policy            runtimeports.ReviewDecisionPolicyCurrentProjectionV1
	actorAuthority    runtimeports.ReviewDecisionAuthorityCurrentProjectionV1
	reviewerAuthority runtimeports.ReviewDecisionAuthorityCurrentProjectionV1
	scope             runtimeports.ReviewDecisionScopeCurrentProjectionV1
	binding           runtimeports.ReviewBindingCurrentProjectionV1
	evidence          []runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1
}

func (s *ExternalSourceV1) InspectDecisionExternalCurrentV1(ctx context.Context, request reviewport.DecisionExternalCurrentRequestV1) (reviewport.DecisionExternalCurrentProjectionV1, error) {
	baseline := s.clock()
	if baseline.IsZero() {
		return reviewport.DecisionExternalCurrentProjectionV1{}, clockRegressionV1("REV-D11 baseline clock is unavailable")
	}
	subjects, err := buildExternalSubjectsV1(request)
	if err != nil {
		return reviewport.DecisionExternalCurrentProjectionV1{}, err
	}
	recoveryCtx, recoveryCancel, recoveryReady := boundedDetachedRecoveryV1(ctx, baseline, request.Target.ExpiresUnixNano, request.Assignment.ExpiresUnixNano, request.Assignment.LeaseExpiresUnixNano)
	if recoveryReady {
		defer recoveryCancel()
	}
	cutCtx := ctx
	refs, err := s.resolveAllV1(cutCtx, request, subjects)
	if err != nil && unknownReadV1(err) {
		// Resolve has no expected Ref. A retry is a new S1 for the whole cut and
		// never claims to recover the unknown prior result. It shares the single
		// bounded detached recovery window with every later exact Inspect.
		originalUnknown := err
		if !recoveryReady {
			return reviewport.DecisionExternalCurrentProjectionV1{}, originalUnknown
		}
		cutCtx = recoveryCtx
		refs, err = s.resolveAllV1(cutCtx, request, subjects)
		if err != nil {
			if core.HasReason(err, core.ReasonClockRegression) {
				return reviewport.DecisionExternalCurrentProjectionV1{}, err
			}
			return reviewport.DecisionExternalCurrentProjectionV1{}, originalUnknown
		}
	}
	if err != nil {
		return reviewport.DecisionExternalCurrentProjectionV1{}, err
	}
	inspectCtx := cutCtx
	s1, detached, err := s.inspectAllV1(inspectCtx, recoveryCtx, request, subjects, refs)
	if err != nil {
		return reviewport.DecisionExternalCurrentProjectionV1{}, err
	}
	if detached {
		// Once an exact read has an unknown reply, completing this immutable
		// read cut must not fall back to an already-canceled caller context.
		// This grants no mutation retry and all remaining reads keep the same
		// exact refs resolved by S1.
		inspectCtx = recoveryCtx
	}
	afterS1 := s.clock()
	if afterS1.IsZero() || afterS1.Before(baseline) {
		return reviewport.DecisionExternalCurrentProjectionV1{}, clockRegressionV1("REV-D11 clock regressed across S1")
	}
	if err := validateExternalCutV1(s1, subjects, refs, afterS1); err != nil {
		return reviewport.DecisionExternalCurrentProjectionV1{}, err
	}
	s2Recovery, s2Cancel, s2Ready := tightenDetachedRecoveryV1(recoveryCtx, afterS1, externalCutExpiryV1(s1))
	if !s2Ready {
		return reviewport.DecisionExternalCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "REV-D11 S1 Owner snapshot expired before S2")
	}
	defer s2Cancel()
	if detached {
		inspectCtx = s2Recovery
	}
	s2, _, err := s.inspectAllV1(inspectCtx, s2Recovery, request, subjects, refs)
	if err != nil {
		return reviewport.DecisionExternalCurrentProjectionV1{}, err
	}
	now := s.clock()
	if now.IsZero() || now.Before(afterS1) {
		return reviewport.DecisionExternalCurrentProjectionV1{}, clockRegressionV1("REV-D11 clock regressed across S2")
	}
	if err := validateExternalCutV1(s2, subjects, refs, now); err != nil {
		return reviewport.DecisionExternalCurrentProjectionV1{}, err
	}
	if !sameExternalCutV1(s1, s2) {
		return reviewport.DecisionExternalCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "REV-D11 Owner projections drifted between S1 and S2")
	}
	return projectExternalCutV1(request, s2, refs)
}

func buildExternalSubjectsV1(request reviewport.DecisionExternalCurrentRequestV1) (externalSubjectsV1, error) {
	target := runtimeports.ReviewDecisionTargetRefV1{TenantID: request.Target.TenantID, ID: request.Target.ID, Revision: request.Target.Revision, Digest: request.Target.Digest, RunID: request.Target.RunID}
	assignment := runtimeports.ReviewDecisionAssignmentRefV1{TenantID: request.Target.TenantID, ID: request.Assignment.ID, Revision: request.Assignment.Revision, Digest: request.Assignment.Digest, ReviewerID: request.Assignment.ReviewerID}
	subjects := externalSubjectsV1{
		policy:            runtimeports.ReviewDecisionPolicyCurrentSubjectV1{Target: target, Policy: request.Target.Policy},
		actorAuthority:    runtimeports.ReviewDecisionAuthorityCurrentSubjectV1{Role: runtimeports.ReviewDecisionAuthorityActorV1, Target: target, Assignment: assignment, Authority: request.Target.ActorAuthority, ActionScopeDigest: request.Target.ActionScopeDigest},
		reviewerAuthority: runtimeports.ReviewDecisionAuthorityCurrentSubjectV1{Role: runtimeports.ReviewDecisionAuthorityReviewerV1, Target: target, Assignment: assignment, Authority: request.Assignment.ReviewerAuthority, ActionScopeDigest: request.Target.ActionScopeDigest},
		scope:             runtimeports.ReviewDecisionScopeCurrentSubjectV1{TenantID: request.Target.TenantID, Target: target, RunID: request.Target.RunID, Scope: request.Target.Scope, CurrentScope: request.Target.CurrentScope, ActionScopeDigest: request.Target.ActionScopeDigest},
		binding:           runtimeports.ReviewBindingSubjectV1{TenantID: request.Target.TenantID, AssignmentID: request.Assignment.ID, AssignmentRevision: request.Assignment.Revision, AssignmentDigest: request.Assignment.Digest, ReviewerID: request.Assignment.ReviewerID, TargetID: request.Target.ID, TargetRevision: request.Target.Revision, TargetDigest: request.Target.Digest},
	}
	for _, evidence := range request.Evidence {
		subjects.evidence = append(subjects.evidence, runtimeports.ReviewEvidenceApplicabilitySubjectV1{TenantID: request.Target.TenantID, Target: runtimeports.ReviewEvidenceTargetRefV1{ID: request.Target.ID, Revision: request.Target.Revision, Digest: request.Target.Digest}, RunID: request.Target.RunID, Scope: request.Target.Scope, ActionScopeDigest: request.Target.ActionScopeDigest, ReviewEvidence: evidence})
	}
	for _, validate := range []func() error{subjects.policy.Validate, subjects.actorAuthority.Validate, subjects.reviewerAuthority.Validate, subjects.scope.Validate, subjects.binding.Validate} {
		if err := validate(); err != nil {
			return externalSubjectsV1{}, err
		}
	}
	for _, evidence := range subjects.evidence {
		if err := evidence.Validate(); err != nil {
			return externalSubjectsV1{}, err
		}
	}
	return subjects, nil
}

func (s *ExternalSourceV1) resolveAllV1(ctx context.Context, request reviewport.DecisionExternalCurrentRequestV1, subjects externalSubjectsV1) (externalRefsV1, error) {
	var refs externalRefsV1
	var err error
	if refs.policy, err = singleReadV1(ctx, s.clock, func(readCtx context.Context) (runtimeports.ReviewDecisionPolicyCurrentProjectionRefV1, error) {
		return s.policy.ResolveCurrentReviewDecisionPolicyV1(readCtx, runtimeports.ReviewDecisionPolicyCurrentResolveRequestV1{Subject: subjects.policy})
	}); err != nil {
		return externalRefsV1{}, err
	}
	if refs.actorAuthority, err = singleReadV1(ctx, s.clock, func(readCtx context.Context) (runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1, error) {
		return s.authority.ResolveCurrentReviewDecisionAuthorityV1(readCtx, runtimeports.ReviewDecisionAuthorityCurrentResolveRequestV1{Subject: subjects.actorAuthority})
	}); err != nil {
		return externalRefsV1{}, err
	}
	if refs.reviewerAuthority, err = singleReadV1(ctx, s.clock, func(readCtx context.Context) (runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1, error) {
		return s.authority.ResolveCurrentReviewDecisionAuthorityV1(readCtx, runtimeports.ReviewDecisionAuthorityCurrentResolveRequestV1{Subject: subjects.reviewerAuthority})
	}); err != nil {
		return externalRefsV1{}, err
	}
	if refs.scope, err = singleReadV1(ctx, s.clock, func(readCtx context.Context) (runtimeports.ReviewDecisionScopeCurrentProjectionRefV1, error) {
		return s.scope.ResolveCurrentReviewDecisionScopeV1(readCtx, runtimeports.ReviewDecisionScopeCurrentResolveRequestV1{Subject: subjects.scope})
	}); err != nil {
		return externalRefsV1{}, err
	}
	if refs.binding, err = singleReadV1(ctx, s.clock, func(readCtx context.Context) (runtimeports.ReviewBindingProjectionRefV1, error) {
		return s.binding.ResolveCurrentReviewBindingV1(readCtx, runtimeports.ResolveReviewBindingCurrentRequestV1{Source: request.Assignment.ReviewerBinding, Subject: subjects.binding})
	}); err != nil {
		return externalRefsV1{}, err
	}
	for _, subject := range subjects.evidence {
		snapshot, resolveErr := singleReadV1(ctx, s.clock, func(readCtx context.Context) (runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
			return s.evidence.ResolveReviewEvidenceApplicabilityCurrentV1(readCtx, runtimeports.ResolveReviewEvidenceApplicabilityCurrentRequestV1{ContractVersion: runtimeports.ReviewEvidenceCurrentContractVersionV1, Subject: subject})
		})
		if resolveErr != nil {
			return externalRefsV1{}, resolveErr
		}
		refs.evidence = append(refs.evidence, snapshot.Projection.Ref)
	}
	return refs, nil
}

func (s *ExternalSourceV1) inspectAllV1(ctx, recoveryCtx context.Context, request reviewport.DecisionExternalCurrentRequestV1, subjects externalSubjectsV1, refs externalRefsV1) (externalCutV1, bool, error) {
	var cut externalCutV1
	var err error
	detached := false
	readCtx := ctx
	if cut.policy, detached, err = retryExactReadBoundedV1(readCtx, recoveryCtx, s.clock, func(readCtx context.Context) (runtimeports.ReviewDecisionPolicyCurrentProjectionV1, error) {
		return s.policy.InspectCurrentReviewDecisionPolicyV1(readCtx, subjects.policy, refs.policy)
	}); err != nil {
		return externalCutV1{}, false, err
	}
	if detached {
		readCtx = recoveryCtx
	}
	var recovered bool
	if cut.actorAuthority, recovered, err = retryExactReadBoundedV1(readCtx, recoveryCtx, s.clock, func(readCtx context.Context) (runtimeports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
		return s.authority.InspectCurrentReviewDecisionAuthorityV1(readCtx, subjects.actorAuthority, refs.actorAuthority)
	}); err != nil {
		return externalCutV1{}, false, err
	}
	if recovered && !detached {
		detached, readCtx = true, recoveryCtx
	}
	if cut.reviewerAuthority, recovered, err = retryExactReadBoundedV1(readCtx, recoveryCtx, s.clock, func(readCtx context.Context) (runtimeports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
		return s.authority.InspectCurrentReviewDecisionAuthorityV1(readCtx, subjects.reviewerAuthority, refs.reviewerAuthority)
	}); err != nil {
		return externalCutV1{}, false, err
	}
	if recovered && !detached {
		detached, readCtx = true, recoveryCtx
	}
	if cut.scope, recovered, err = retryExactReadBoundedV1(readCtx, recoveryCtx, s.clock, func(readCtx context.Context) (runtimeports.ReviewDecisionScopeCurrentProjectionV1, error) {
		return s.scope.InspectCurrentReviewDecisionScopeV1(readCtx, subjects.scope, refs.scope)
	}); err != nil {
		return externalCutV1{}, false, err
	}
	if recovered && !detached {
		detached, readCtx = true, recoveryCtx
	}
	if cut.binding, recovered, err = retryExactReadBoundedV1(readCtx, recoveryCtx, s.clock, func(readCtx context.Context) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
		return s.binding.InspectCurrentReviewBindingV1(readCtx, runtimeports.InspectCurrentReviewBindingRequestV1{ExpectedRef: refs.binding, ExpectedSource: request.Assignment.ReviewerBinding, ExpectedSubject: subjects.binding})
	}); err != nil {
		return externalCutV1{}, false, err
	}
	if recovered && !detached {
		detached, readCtx = true, recoveryCtx
	}
	for _, ref := range refs.evidence {
		value, evidenceRecovered, inspectErr := retryExactReadBoundedV1(readCtx, recoveryCtx, s.clock, func(readCtx context.Context) (runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
			return s.evidence.InspectCurrentReviewEvidenceApplicabilityV1(readCtx, ref)
		})
		if inspectErr != nil {
			return externalCutV1{}, false, inspectErr
		}
		if evidenceRecovered && !detached {
			detached, readCtx = true, recoveryCtx
		}
		cut.evidence = append(cut.evidence, value)
	}
	return cut, detached, nil
}

func singleReadV1[T any](ctx context.Context, clock func() time.Time, read func(context.Context) (T, error)) (T, error) {
	baseline := clock()
	if baseline.IsZero() {
		var zero T
		return zero, clockRegressionV1("REV-D11 Owner read baseline clock is unavailable")
	}
	value, err := read(ctx)
	now := clock()
	if now.IsZero() || now.Before(baseline) {
		var zero T
		return zero, clockRegressionV1("REV-D11 clock regressed across Owner read")
	}
	return value, err
}

func retryExactReadBoundedV1[T any](ctx, recoveryCtx context.Context, clock func() time.Time, read func(context.Context) (T, error)) (T, bool, error) {
	value, err := singleReadV1(ctx, clock, read)
	if err == nil || !unknownReadV1(err) {
		return value, false, err
	}
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

// retryExactReadV1 performs at most one exact retry inside the caller-owned
// recovery window. All reads belonging to one cut therefore share one bounded
// detached deadline instead of accumulating an independent timeout per field.
func retryExactReadV1[T any](ctx, recoveryCtx context.Context, clock func() time.Time, read func(context.Context) (T, error)) (T, bool, error) {
	value, err := singleReadV1(ctx, clock, read)
	if err == nil || !unknownReadV1(err) {
		return value, false, err
	}
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

func boundedDetachedRecoveryV1(parent context.Context, now time.Time, expiries ...int64) (context.Context, context.CancelFunc, bool) {
	if now.IsZero() {
		return nil, nil, false
	}
	limit := 5 * time.Second
	for _, expiry := range expiries {
		if expiry <= 0 {
			continue
		}
		remaining := time.Unix(0, expiry).Sub(now)
		if remaining <= 0 {
			return nil, nil, false
		}
		if remaining < limit {
			limit = remaining
		}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), limit)
	return ctx, cancel, true
}

func tightenDetachedRecoveryV1(parent context.Context, now time.Time, expiries ...int64) (context.Context, context.CancelFunc, bool) {
	if parent == nil || now.IsZero() {
		return nil, nil, false
	}
	var limit time.Duration
	for _, expiry := range expiries {
		if expiry <= 0 {
			continue
		}
		remaining := time.Unix(0, expiry).Sub(now)
		if remaining <= 0 {
			return nil, nil, false
		}
		if limit == 0 || remaining < limit {
			limit = remaining
		}
	}
	if limit == 0 {
		limit = 5 * time.Second
	}
	tightened, cancel := context.WithTimeout(parent, limit)
	return tightened, cancel, true
}

func externalCutExpiryV1(cut externalCutV1) int64 {
	values := []int64{
		cut.policy.ExpiresUnixNano,
		cut.actorAuthority.ExpiresUnixNano,
		cut.reviewerAuthority.ExpiresUnixNano,
		cut.scope.ExpiresUnixNano,
		cut.binding.ExpiresUnixNano,
	}
	for _, evidence := range cut.evidence {
		values = append(values, evidence.Projection.ExpiresUnixNano)
	}
	return minimumPositive(values...)
}

func validateExternalCutV1(cut externalCutV1, subjects externalSubjectsV1, refs externalRefsV1, now time.Time) error {
	for _, validate := range []func() error{
		func() error { return cut.policy.ValidateCurrent(refs.policy, subjects.policy, now) },
		func() error {
			return cut.actorAuthority.ValidateCurrent(refs.actorAuthority, subjects.actorAuthority, now)
		},
		func() error {
			return cut.reviewerAuthority.ValidateCurrent(refs.reviewerAuthority, subjects.reviewerAuthority, now)
		},
		func() error { return cut.scope.ValidateCurrent(refs.scope, subjects.scope, now) },
		func() error {
			return cut.binding.ValidateCurrent(refs.binding, cut.binding.Source, subjects.binding, now)
		},
	} {
		if err := validate(); err != nil {
			return err
		}
	}
	if len(cut.evidence) != len(refs.evidence) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "REV-D11 Evidence current set is incomplete")
	}
	for index := range cut.evidence {
		if err := cut.evidence[index].ValidateCurrent(refs.evidence[index], now); err != nil {
			return err
		}
		if cut.evidence[index].Projection.Subject != subjects.evidence[index] {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "REV-D11 Evidence current subject drifted")
		}
	}
	return nil
}

func sameExternalCutV1(left, right externalCutV1) bool {
	return reflect.DeepEqual(left, right)
}

func projectExternalCutV1(request reviewport.DecisionExternalCurrentRequestV1, cut externalCutV1, refs externalRefsV1) (reviewport.DecisionExternalCurrentProjectionV1, error) {
	proof, err := contract.SealDecisionExternalCurrentProofV1(contract.DecisionExternalCurrentProofV1{Policy: refs.policy, ActorAuthority: refs.actorAuthority, ReviewerAuthority: refs.reviewerAuthority, Scope: refs.scope, Binding: refs.binding})
	if err != nil {
		return reviewport.DecisionExternalCurrentProjectionV1{}, err
	}
	expires := minimumPositive(cut.policy.ExpiresUnixNano, cut.actorAuthority.ExpiresUnixNano, cut.reviewerAuthority.ExpiresUnixNano, cut.scope.ExpiresUnixNano, cut.binding.ExpiresUnixNano)
	result := reviewport.DecisionExternalCurrentProjectionV1{
		Policy:            cut.policy.Fact,
		ActorAuthority:    runtimeports.OperationGovernanceFactRefV3{Ref: cut.actorAuthority.Fact.Ref, Revision: cut.actorAuthority.Fact.Revision, Digest: cut.actorAuthority.Fact.Digest, ExpiresUnixNano: cut.actorAuthority.ExpiresUnixNano},
		ReviewerAuthority: runtimeports.OperationGovernanceFactRefV3{Ref: cut.reviewerAuthority.Fact.Ref, Revision: cut.reviewerAuthority.Fact.Revision, Digest: cut.reviewerAuthority.Fact.Digest, ExpiresUnixNano: cut.reviewerAuthority.ExpiresUnixNano},
		Scope:             runtimeports.OperationGovernanceFactRefV3{Ref: cut.scope.Fact.Ref, Revision: cut.scope.Fact.Revision, Digest: cut.scope.Fact.Digest, ExpiresUnixNano: cut.scope.ExpiresUnixNano},
		Binding:           contract.ReviewerBindingCurrentV1{Binding: cut.binding.Source, ProjectionRef: cut.binding.Ref, CheckedUnixNano: cut.binding.CheckedUnixNano, ProjectionDigest: cut.binding.ProjectionDigest, Current: true, ExpiresUnixNano: cut.binding.ExpiresUnixNano},
		ExternalProof:     &proof,
		Current:           true,
	}
	for index, item := range cut.evidence {
		projection := item.Projection
		current := contract.DecisionEvidenceCurrentV1{Review: request.Evidence[index], ApplicabilityRef: projection.Ref, Record: projection.Record, CheckedUnixNano: projection.CheckedUnixNano, ProjectionDigest: projection.ProjectionDigest, Current: true, ExpiresUnixNano: projection.ExpiresUnixNano}
		if projection.OwnerFact != nil {
			current.OwnerFact = *projection.OwnerFact
		}
		result.Evidence = append(result.Evidence, current)
		expires = minimumPositive(expires, projection.ExpiresUnixNano)
	}
	sort.Slice(result.Evidence, func(i, j int) bool { return result.Evidence[i].Review.Ref < result.Evidence[j].Review.Ref })
	result.ExpiresUnixNano = expires
	return result, nil
}

func unknownReadV1(err error) bool {
	return core.HasCategory(err, core.ErrorIndeterminate) || core.HasCategory(err, core.ErrorUnavailable)
}

func clockRegressionV1(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, message)
}

var _ reviewport.DecisionExternalCurrentReaderV1 = (*ExternalSourceV1)(nil)
