package multisigcurrent

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	"github.com/Proview-China/rax/ExecutionRuntime/review/multisigowner"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const maximumDetachedCurrentRecoveryV2 = 5 * time.Second

// ExternalSourceV2 is the production-shaped Human Multi-Sign external-current
// aggregator. It owns no external fact and receives only narrow read ports.
type ExternalSourceV2 struct {
	review       reviewport.HumanMultiSignExactReaderV2
	organization reviewport.HumanOrganizationCurrentReaderV2
	coordinates  reviewport.HumanOrganizationCurrentRequestResolverV2
	policy       runtimeports.HumanQuorumPolicyCurrentReaderV2
	authority    runtimeports.ReviewDecisionAuthorityCurrentReaderV1
	binding      runtimeports.ReviewBindingAuthoritativeCurrentReaderV1
	scope        runtimeports.ReviewDecisionScopeCurrentReaderV1
	evidence     runtimeports.ReviewEvidenceApplicabilityCurrentReaderV1
	clock        func() time.Time
}

func NewExternalSourceV2(
	review reviewport.HumanMultiSignExactReaderV2,
	organization reviewport.HumanOrganizationCurrentReaderV2,
	coordinates reviewport.HumanOrganizationCurrentRequestResolverV2,
	policy runtimeports.HumanQuorumPolicyCurrentReaderV2,
	authority runtimeports.ReviewDecisionAuthorityCurrentReaderV1,
	binding runtimeports.ReviewBindingAuthoritativeCurrentReaderV1,
	scope runtimeports.ReviewDecisionScopeCurrentReaderV1,
	evidence runtimeports.ReviewEvidenceApplicabilityCurrentReaderV1,
	clock func() time.Time,
) (*ExternalSourceV2, error) {
	if nilcheck.IsNil(review) || nilcheck.IsNil(organization) || nilcheck.IsNil(coordinates) || nilcheck.IsNil(policy) || nilcheck.IsNil(authority) || nilcheck.IsNil(binding) || nilcheck.IsNil(scope) || nilcheck.IsNil(evidence) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "human multisign external current requires all exact Owner readers and a clock")
	}
	return &ExternalSourceV2{review: review, organization: organization, coordinates: coordinates, policy: policy, authority: authority, binding: binding, scope: scope, evidence: evidence, clock: clock}, nil
}

type externalRequestV2 struct {
	panel       contract.HumanReviewPanelV2
	assignments []contract.HumanPanelAssignmentV2
	evidence    []runtimeports.ReviewEvidenceRefV2
	subject     core.Digest
	baseline    time.Time
}

type externalSubjectsV2 struct {
	policy         runtimeports.HumanQuorumPolicyCurrentSubjectV2
	authorities    []runtimeports.ReviewDecisionAuthorityCurrentSubjectV1
	bindings       []runtimeports.ReviewBindingSubjectV1
	scope          runtimeports.ReviewDecisionScopeCurrentSubjectV1
	evidence       []runtimeports.ReviewEvidenceApplicabilitySubjectV1
	organization   []reviewport.HumanOrganizationCurrentRequestV2
	bindingSources []runtimeports.ReviewComponentBindingRefV2
}

type externalRefsV2 struct {
	policy      runtimeports.HumanQuorumPolicyCurrentProjectionRefV2
	authorities []runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1
	bindings    []runtimeports.ReviewBindingProjectionRefV1
	scope       runtimeports.ReviewDecisionScopeCurrentProjectionRefV1
	evidence    []runtimeports.ReviewEvidenceApplicabilityRefV1
}

type externalCutV2 struct {
	policy       runtimeports.HumanQuorumPolicyCurrentProjectionV2
	authorities  []runtimeports.ReviewDecisionAuthorityCurrentProjectionV1
	bindings     []runtimeports.ReviewBindingCurrentProjectionV1
	scope        runtimeports.ReviewDecisionScopeCurrentProjectionV1
	evidence     []runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1
	organization reviewport.HumanOrganizationCurrentCutV2
}

func (s *ExternalSourceV2) ValidatePanelCurrentV2(ctx context.Context, proposed contract.HumanReviewPanelV2, assignments []contract.HumanPanelAssignmentV2, open contract.HumanReviewPanelV2, baseline time.Time) (multisigowner.ExternalCurrentProofV2, error) {
	guarded, err := s.withClockWatermarkV2(baseline)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	subject, err := multisigowner.PanelCurrentSubjectDigestV2(proposed, assignments, open)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	if proposed.Target != open.Target || proposed.QuorumPolicy != open.QuorumPolicy || proposed.ResponsibilitySubject != open.ResponsibilitySubject {
		return multisigowner.ExternalCurrentProofV2{}, currentConflictV2("open Panel drifted from its proposed external coordinates")
	}
	return guarded.validateV2(ctx, externalRequestV2{panel: proposed.Clone(), assignments: cloneAssignmentsV2(assignments), subject: subject, baseline: baseline})
}

func (s *ExternalSourceV2) ValidateAttestationCurrentV2(ctx context.Context, panel contract.HumanReviewPanelV2, assignment contract.HumanPanelAssignmentV2, attestation contract.HumanAttestationV2, baseline time.Time) (multisigowner.ExternalCurrentProofV2, error) {
	guarded, err := s.withClockWatermarkV2(baseline)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	subject, err := multisigowner.AttestationCurrentSubjectDigestV2(panel, assignment, attestation)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	if attestation.Panel != panel.ExactRef() || attestation.Assignment != assignment.ExactRef() || attestation.Target != panel.Target || attestation.Policy != panel.QuorumPolicy || attestation.ReviewerIdentity != assignment.ReviewerIdentity || attestation.ReviewerAuthority != assignment.ReviewerAuthority || attestation.ReviewerBinding != assignment.ReviewerBinding {
		return multisigowner.ExternalCurrentProofV2{}, currentConflictV2("Attestation drifted from its exact Panel or Assignment")
	}
	assignments, detached, err := guarded.readAssignmentsV2(ctx, panel, baseline)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	if detached {
		recoveryCtx, cancel, ok := boundedCurrentRecoveryContextV2(ctx, guarded.clock(), panel.ExpiresUnixNano, assignment.ExpiresUnixNano, assignment.LeaseExpiresUnixNano)
		if !ok {
			return multisigowner.ExternalCurrentProofV2{}, currentConflictV2("human multisign Assignment recovery crossed current TTL")
		}
		defer cancel()
		ctx = recoveryCtx
	}
	if !containsAssignmentV2(assignments, assignment.ExactRef()) {
		return multisigowner.ExternalCurrentProofV2{}, currentConflictV2("Attestation Assignment is absent from the exact Panel set")
	}
	return guarded.validateV2(ctx, externalRequestV2{panel: panel.Clone(), assignments: assignments, evidence: append([]runtimeports.ReviewEvidenceRefV2(nil), attestation.Evidence...), subject: subject, baseline: baseline})
}

func (s *ExternalSourceV2) ValidateDecisionCurrentV2(ctx context.Context, panel contract.HumanReviewPanelV2, quorum contract.HumanQuorumDecisionV2, verdict contract.HumanVerdictV2, baseline time.Time) (multisigowner.ExternalCurrentProofV2, error) {
	guarded, err := s.withClockWatermarkV2(baseline)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	subject, err := multisigowner.DecisionCurrentSubjectDigestV2(panel, quorum, verdict)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	if quorum.Panel.TenantID != panel.TenantID || quorum.Panel.ID != panel.ID || quorum.Panel.Revision+1 != panel.Revision || verdict.Panel != panel.ExactRef() || verdict.QuorumDecision != quorum.ExactRef() || verdict.Target != panel.Target || verdict.Policy != panel.QuorumPolicy || quorum.Policy != panel.QuorumPolicy {
		return multisigowner.ExternalCurrentProofV2{}, currentConflictV2("Decision drifted from its exact Panel, Quorum or Policy")
	}
	assignments, detached, err := guarded.readAssignmentsV2(ctx, panel, baseline)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	if detached {
		recoveryCtx, cancel, ok := boundedCurrentRecoveryContextV2(ctx, guarded.clock(), panel.ExpiresUnixNano)
		if !ok {
			return multisigowner.ExternalCurrentProofV2{}, currentConflictV2("human multisign Assignment recovery crossed current TTL")
		}
		defer cancel()
		ctx = recoveryCtx
	}
	if !sameVerdictAssignmentClosureV2(assignments, verdict) {
		return multisigowner.ExternalCurrentProofV2{}, currentConflictV2("Verdict Authority or Binding set drifted from the exact Panel Assignments")
	}
	return guarded.validateV2(ctx, externalRequestV2{panel: panel.Clone(), assignments: assignments, evidence: append([]runtimeports.ReviewEvidenceRefV2(nil), verdict.Evidence...), subject: subject, baseline: baseline})
}

func (s *ExternalSourceV2) withClockWatermarkV2(baseline time.Time) (*ExternalSourceV2, error) {
	if s == nil || s.clock == nil || baseline.IsZero() {
		return nil, clockRegressionV2("human multisign external current baseline is unavailable")
	}
	var mu sync.Mutex
	last := baseline
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		now := s.clock()
		if now.IsZero() || now.Before(last) {
			return time.Time{}
		}
		last = now
		return now
	}
	copy := *s
	copy.clock = clock
	return &copy, nil
}

func (s *ExternalSourceV2) readAssignmentsV2(ctx context.Context, panel contract.HumanReviewPanelV2, baseline time.Time) ([]contract.HumanPanelAssignmentV2, bool, error) {
	values, detached, err := exactReadV2(ctx, s.clock, func(readCtx context.Context) ([]contract.HumanPanelAssignmentV2, error) {
		return s.review.ListHumanPanelAssignmentsV2(readCtx, panel.ExactRef())
	}, panel.ExpiresUnixNano)
	if err != nil {
		return nil, false, err
	}
	now := s.clock()
	if now.IsZero() || now.Before(baseline) {
		return nil, false, clockRegressionV2("human multisign Assignment read clock regressed")
	}
	return cloneAssignmentsV2(values), detached, nil
}

func (s *ExternalSourceV2) validateV2(ctx context.Context, request externalRequestV2) (multisigowner.ExternalCurrentProofV2, error) {
	start := s.clock()
	if request.baseline.IsZero() || start.IsZero() || start.Before(request.baseline) {
		return multisigowner.ExternalCurrentProofV2{}, clockRegressionV2("human multisign external current baseline is unavailable or regressed")
	}
	if err := request.panel.ValidateCurrent(request.panel.ExactRef(), start); err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	if err := validateAssignmentSetV2(request.panel, request.assignments, start); err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	cutCtx := ctx
	initialExpiries := []int64{request.panel.ExpiresUnixNano}
	for _, assignment := range request.assignments {
		initialExpiries = append(initialExpiries, assignment.ExpiresUnixNano, assignment.LeaseExpiresUnixNano)
	}
	target, detached, err := exactReadV2(cutCtx, s.clock, func(readCtx context.Context) (contract.TargetSnapshotV1, error) {
		return s.review.InspectTargetExactV1(readCtx, request.panel.Target.TenantID, reviewport.ExactV1(request.panel.Target.ID, request.panel.Target.Revision, request.panel.Target.Digest))
	}, initialExpiries...)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	target.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), target.Evidence...)
	if target.TenantID != request.panel.Target.TenantID || target.ID != request.panel.Target.ID || target.Revision != request.panel.Target.Revision || target.Digest != request.panel.Target.Digest {
		return multisigowner.ExternalCurrentProofV2{}, currentConflictV2("exact Target read drifted from Panel")
	}
	afterTarget := s.clock()
	if afterTarget.IsZero() || afterTarget.Before(start) {
		return multisigowner.ExternalCurrentProofV2{}, clockRegressionV2("human multisign Target read clock regressed")
	}
	if err := target.ValidateCurrent(contract.TargetCurrentnessV1{TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, PayloadRevision: target.PayloadRevision, PayloadDigest: target.PayloadDigest, Scope: target.Scope, ActionScopeDigest: target.ActionScopeDigest, Policy: target.Policy, ActorAuthority: target.ActorAuthority, CurrentScope: target.CurrentScope, EvidenceSetDigest: target.EvidenceSetDigest, ContextFrameDigest: target.ContextFrameDigest, Now: afterTarget}); err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	var cutCancel context.CancelFunc
	if detached {
		var ok bool
		cutCtx, cutCancel, ok = boundedCurrentRecoveryContextV2(ctx, afterTarget, append(initialExpiries, target.ExpiresUnixNano)...)
		if !ok {
			return multisigowner.ExternalCurrentProofV2{}, currentConflictV2("human multisign Target recovery crossed current TTL")
		}
		defer cutCancel()
	}
	organizationRequests, recoveredCoordinates, err := exactReadV2(cutCtx, s.clock, func(readCtx context.Context) ([]reviewport.HumanOrganizationCurrentRequestV2, error) {
		return s.coordinates.InspectHumanOrganizationCurrentRequestsV2(readCtx, request.panel.Clone(), cloneAssignmentsV2(request.assignments))
	}, append(initialExpiries, target.ExpiresUnixNano)...)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	organizationRequests = cloneOrganizationRequestsV2(organizationRequests)
	if err := validateOrganizationRequestsV2(request.panel, request.assignments, organizationRequests); err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	if recoveredCoordinates && !detached {
		var ok bool
		cutCtx, cutCancel, ok = boundedCurrentRecoveryContextV2(ctx, s.clock(), append(initialExpiries, target.ExpiresUnixNano)...)
		if !ok {
			return multisigowner.ExternalCurrentProofV2{}, currentConflictV2("human multisign coordinate recovery crossed current TTL")
		}
		defer cutCancel()
		detached = true
	}
	subjects, err := buildExternalSubjectsV2(target, request.panel, request.assignments, request.evidence, organizationRequests)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	resolveCtx := cutCtx
	refs, err := s.resolveAllV2(resolveCtx, subjects)
	if err != nil && unknownReadV2(err) {
		originalUnknown := normalizeCurrentReadErrorV2(err)
		recoveryCtx, cancel, ok := boundedCurrentRecoveryContextV2(cutCtx, afterTarget, append(initialExpiries, target.ExpiresUnixNano)...)
		if !ok {
			return multisigowner.ExternalCurrentProofV2{}, currentConflictV2("human multisign detached Resolve recovery crossed current TTL")
		}
		refs, err = s.resolveAllV2(recoveryCtx, subjects)
		cancel()
		if err != nil && !core.HasReason(err, core.ReasonClockRegression) {
			return multisigowner.ExternalCurrentProofV2{}, originalUnknown
		}
		if err == nil && !detached {
			var keepOK bool
			resolveCtx, cutCancel, keepOK = boundedCurrentRecoveryContextV2(ctx, s.clock(), append(initialExpiries, target.ExpiresUnixNano)...)
			if !keepOK {
				return multisigowner.ExternalCurrentProofV2{}, currentConflictV2("human multisign Resolve recovery crossed current TTL")
			}
			defer cutCancel()
			detached = true
		}
	}
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	wantPolicy := runtimeports.HumanQuorumPolicyCurrentProjectionRefV2{ID: request.panel.QuorumPolicy.Ref, Revision: request.panel.QuorumPolicy.Revision, Digest: request.panel.QuorumPolicy.Digest}
	if refs.policy != wantPolicy {
		return multisigowner.ExternalCurrentProofV2{}, currentConflictV2("Panel quorum Policy is not the Policy Owner current exact ref")
	}
	baseExpiries := []int64{target.ExpiresUnixNano, request.panel.ExpiresUnixNano}
	for _, assignment := range request.assignments {
		baseExpiries = append(baseExpiries, assignment.ExpiresUnixNano, assignment.LeaseExpiresUnixNano)
	}
	s1, inspectDetached, err := s.inspectAllV2(resolveCtx, subjects, refs, baseExpiries...)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	afterS1 := s.clock()
	if afterS1.IsZero() || afterS1.Before(afterTarget) {
		return multisigowner.ExternalCurrentProofV2{}, clockRegressionV2("human multisign external current clock regressed across S1")
	}
	if err := validateExternalCutV2(request.panel, subjects, refs, s1, afterS1); err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	s2Expiries := append([]int64(nil), baseExpiries...)
	s2Expiries = append(s2Expiries, s1.policy.ExpiresUnixNano, s1.scope.ExpiresUnixNano, s1.organization.ExpiresUnixNano)
	for _, value := range s1.authorities {
		s2Expiries = append(s2Expiries, value.ExpiresUnixNano)
	}
	for _, value := range s1.bindings {
		s2Expiries = append(s2Expiries, value.ExpiresUnixNano)
	}
	for _, value := range s1.evidence {
		s2Expiries = append(s2Expiries, value.Projection.ExpiresUnixNano)
	}
	inspectCtx := resolveCtx
	var inspectCancel context.CancelFunc
	if inspectDetached {
		var ok bool
		inspectCtx, inspectCancel, ok = boundedCurrentRecoveryContextV2(ctx, afterS1, s2Expiries...)
		if !ok {
			return multisigowner.ExternalCurrentProofV2{}, currentConflictV2("human multisign S1 recovery crossed current TTL")
		}
		defer inspectCancel()
	}
	s2, _, err := s.inspectAllV2(inspectCtx, subjects, refs, s2Expiries...)
	if err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	now := s.clock()
	if now.IsZero() || now.Before(afterS1) {
		return multisigowner.ExternalCurrentProofV2{}, clockRegressionV2("human multisign external current clock regressed across S2")
	}
	if err := validateExternalCutV2(request.panel, subjects, refs, s2, now); err != nil {
		return multisigowner.ExternalCurrentProofV2{}, err
	}
	if !sameExternalCutV2(s1, s2) {
		return multisigowner.ExternalCurrentProofV2{}, currentConflictV2("human multisign Owner projections drifted between S1 and S2")
	}
	expires := minimumExpiryV2(target.ExpiresUnixNano, request.panel.ExpiresUnixNano, s2.policy.ExpiresUnixNano, s2.scope.ExpiresUnixNano, s2.organization.ExpiresUnixNano)
	for _, assignment := range request.assignments {
		expires = minimumExpiryV2(expires, assignment.ExpiresUnixNano, assignment.LeaseExpiresUnixNano)
	}
	for _, value := range s2.authorities {
		expires = minimumExpiryV2(expires, value.ExpiresUnixNano)
	}
	for _, value := range s2.bindings {
		expires = minimumExpiryV2(expires, value.ExpiresUnixNano)
	}
	for _, value := range s2.evidence {
		expires = minimumExpiryV2(expires, value.Projection.ExpiresUnixNano)
	}
	return multisigowner.SealExternalCurrentProofV2(multisigowner.ExternalCurrentProofV2{TenantID: request.panel.TenantID, SubjectDigest: request.subject, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
}

func buildExternalSubjectsV2(target contract.TargetSnapshotV1, panel contract.HumanReviewPanelV2, assignments []contract.HumanPanelAssignmentV2, evidence []runtimeports.ReviewEvidenceRefV2, organization []reviewport.HumanOrganizationCurrentRequestV2) (externalSubjectsV2, error) {
	targetRef := runtimeports.ReviewDecisionTargetRefV1{TenantID: target.TenantID, ID: target.ID, Revision: target.Revision, Digest: target.Digest, RunID: target.RunID}
	result := externalSubjectsV2{
		policy:       runtimeports.HumanQuorumPolicyCurrentSubjectV2{TenantID: panel.TenantID, Domain: panel.QuorumPolicy.Domain},
		scope:        runtimeports.ReviewDecisionScopeCurrentSubjectV1{TenantID: target.TenantID, Target: targetRef, RunID: target.RunID, Scope: target.Scope, CurrentScope: target.CurrentScope, ActionScopeDigest: target.ActionScopeDigest},
		organization: cloneOrganizationRequestsV2(organization),
	}
	for _, assignment := range assignments {
		assignmentRef := runtimeports.ReviewDecisionAssignmentRefV1{TenantID: assignment.TenantID, ID: assignment.ID, Revision: assignment.Revision, Digest: assignment.Digest, ReviewerID: assignment.ReviewerIdentity.Ref}
		result.authorities = append(result.authorities,
			runtimeports.ReviewDecisionAuthorityCurrentSubjectV1{Role: runtimeports.ReviewDecisionAuthorityActorV1, Target: targetRef, Assignment: assignmentRef, Authority: target.ActorAuthority, ActionScopeDigest: target.ActionScopeDigest},
			runtimeports.ReviewDecisionAuthorityCurrentSubjectV1{Role: runtimeports.ReviewDecisionAuthorityReviewerV1, Target: targetRef, Assignment: assignmentRef, Authority: assignment.ReviewerAuthority, ActionScopeDigest: target.ActionScopeDigest},
		)
		result.bindings = append(result.bindings, runtimeports.ReviewBindingSubjectV1{TenantID: target.TenantID, AssignmentID: assignment.ID, AssignmentRevision: assignment.Revision, AssignmentDigest: assignment.Digest, ReviewerID: assignment.ReviewerIdentity.Ref, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest})
		result.bindingSources = append(result.bindingSources, assignment.ReviewerBinding)
	}
	for _, ref := range evidence {
		result.evidence = append(result.evidence, runtimeports.ReviewEvidenceApplicabilitySubjectV1{TenantID: target.TenantID, Target: runtimeports.ReviewEvidenceTargetRefV1{ID: target.ID, Revision: target.Revision, Digest: target.Digest}, RunID: target.RunID, Scope: target.Scope, ActionScopeDigest: target.ActionScopeDigest, ReviewEvidence: ref})
	}
	for _, validate := range []func() error{result.policy.Validate, result.scope.Validate} {
		if err := validate(); err != nil {
			return externalSubjectsV2{}, err
		}
	}
	for _, value := range result.authorities {
		if err := value.Validate(); err != nil {
			return externalSubjectsV2{}, err
		}
	}
	for _, value := range result.bindings {
		if err := value.Validate(); err != nil {
			return externalSubjectsV2{}, err
		}
	}
	for _, value := range result.evidence {
		if err := value.Validate(); err != nil {
			return externalSubjectsV2{}, err
		}
	}
	return result, nil
}

func (s *ExternalSourceV2) resolveAllV2(ctx context.Context, subjects externalSubjectsV2) (externalRefsV2, error) {
	var refs externalRefsV2
	var err error
	if refs.policy, err = timedReadV2(ctx, s.clock, func(readCtx context.Context) (runtimeports.HumanQuorumPolicyCurrentProjectionRefV2, error) {
		return s.policy.ResolveCurrentHumanQuorumPolicyV2(readCtx, runtimeports.HumanQuorumPolicyCurrentResolveRequestV2{Subject: subjects.policy})
	}); err != nil {
		return externalRefsV2{}, err
	}
	for _, subject := range subjects.authorities {
		ref, readErr := timedReadV2(ctx, s.clock, func(readCtx context.Context) (runtimeports.ReviewDecisionAuthorityCurrentProjectionRefV1, error) {
			return s.authority.ResolveCurrentReviewDecisionAuthorityV1(readCtx, runtimeports.ReviewDecisionAuthorityCurrentResolveRequestV1{Subject: subject})
		})
		if readErr != nil {
			return externalRefsV2{}, readErr
		}
		refs.authorities = append(refs.authorities, ref)
	}
	for index, subject := range subjects.bindings {
		ref, readErr := timedReadV2(ctx, s.clock, func(readCtx context.Context) (runtimeports.ReviewBindingProjectionRefV1, error) {
			return s.binding.ResolveCurrentReviewBindingV1(readCtx, runtimeports.ResolveReviewBindingCurrentRequestV1{Source: subjects.bindingSources[index], Subject: subject})
		})
		if readErr != nil {
			return externalRefsV2{}, readErr
		}
		refs.bindings = append(refs.bindings, ref)
	}
	if refs.scope, err = timedReadV2(ctx, s.clock, func(readCtx context.Context) (runtimeports.ReviewDecisionScopeCurrentProjectionRefV1, error) {
		return s.scope.ResolveCurrentReviewDecisionScopeV1(readCtx, runtimeports.ReviewDecisionScopeCurrentResolveRequestV1{Subject: subjects.scope})
	}); err != nil {
		return externalRefsV2{}, err
	}
	for _, subject := range subjects.evidence {
		snapshot, readErr := timedReadV2(ctx, s.clock, func(readCtx context.Context) (runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
			return s.evidence.ResolveReviewEvidenceApplicabilityCurrentV1(readCtx, runtimeports.ResolveReviewEvidenceApplicabilityCurrentRequestV1{ContractVersion: runtimeports.ReviewEvidenceCurrentContractVersionV1, Subject: subject})
		})
		if readErr != nil {
			return externalRefsV2{}, readErr
		}
		refs.evidence = append(refs.evidence, snapshot.Projection.Ref)
	}
	return refs, nil
}

func (s *ExternalSourceV2) inspectAllV2(ctx context.Context, subjects externalSubjectsV2, refs externalRefsV2, baseExpiries ...int64) (externalCutV2, bool, error) {
	var cut externalCutV2
	var err error
	detached := false
	readCtx := ctx
	var recoveryCancel context.CancelFunc
	knownExpiries := append([]int64(nil), baseExpiries...)
	promote := func() error {
		if detached {
			return nil
		}
		recoveryCtx, cancel, ok := boundedCurrentRecoveryContextV2(ctx, s.clock(), knownExpiries...)
		if !ok {
			return clockRegressionV2("human multisign exact-cut recovery clock is unavailable")
		}
		readCtx = recoveryCtx
		recoveryCancel = cancel
		detached = true
		return nil
	}
	defer func() {
		if recoveryCancel != nil {
			recoveryCancel()
		}
	}()
	if cut.policy, detached, err = exactReadV2(readCtx, s.clock, func(readCtx context.Context) (runtimeports.HumanQuorumPolicyCurrentProjectionV2, error) {
		return s.policy.InspectCurrentHumanQuorumPolicyV2(readCtx, subjects.policy, refs.policy)
	}, baseExpiries...); err != nil {
		return externalCutV2{}, false, err
	}
	if detached {
		// exactReadV2 used a bounded recovery context for the lost call. Keep
		// the remaining cut detached, but start a fresh bounded window.
		detached = false
		knownExpiries = append(knownExpiries, cut.policy.ExpiresUnixNano)
		if err := promote(); err != nil {
			return externalCutV2{}, false, err
		}
	} else {
		knownExpiries = append(knownExpiries, cut.policy.ExpiresUnixNano)
	}
	for index, subject := range subjects.authorities {
		value, recovered, readErr := exactReadV2(readCtx, s.clock, func(readCtx context.Context) (runtimeports.ReviewDecisionAuthorityCurrentProjectionV1, error) {
			return s.authority.InspectCurrentReviewDecisionAuthorityV1(readCtx, subject, refs.authorities[index])
		}, knownExpiries...)
		if readErr != nil {
			return externalCutV2{}, false, readErr
		}
		if recovered {
			if err := promote(); err != nil {
				return externalCutV2{}, false, err
			}
		}
		cut.authorities = append(cut.authorities, value)
		knownExpiries = append(knownExpiries, value.ExpiresUnixNano)
	}
	for index, subject := range subjects.bindings {
		value, recovered, readErr := exactReadV2(readCtx, s.clock, func(readCtx context.Context) (runtimeports.ReviewBindingCurrentProjectionV1, error) {
			return s.binding.InspectCurrentReviewBindingV1(readCtx, runtimeports.InspectCurrentReviewBindingRequestV1{ExpectedRef: refs.bindings[index], ExpectedSource: subjects.bindingSources[index], ExpectedSubject: subject})
		}, knownExpiries...)
		if readErr != nil {
			return externalCutV2{}, false, readErr
		}
		if recovered {
			if err := promote(); err != nil {
				return externalCutV2{}, false, err
			}
		}
		cut.bindings = append(cut.bindings, value.CloneV1())
		knownExpiries = append(knownExpiries, value.ExpiresUnixNano)
	}
	if cut.scope, detached, err = exactReadMergeV2(readCtx, detached, s.clock, func(readCtx context.Context) (runtimeports.ReviewDecisionScopeCurrentProjectionV1, error) {
		return s.scope.InspectCurrentReviewDecisionScopeV1(readCtx, subjects.scope, refs.scope)
	}, knownExpiries...); err != nil {
		return externalCutV2{}, false, err
	}
	if detached && recoveryCancel == nil {
		detached = false
		knownExpiries = append(knownExpiries, cut.scope.ExpiresUnixNano)
		if err := promote(); err != nil {
			return externalCutV2{}, false, err
		}
	} else {
		knownExpiries = append(knownExpiries, cut.scope.ExpiresUnixNano)
	}
	for _, ref := range refs.evidence {
		value, recovered, readErr := exactReadV2(readCtx, s.clock, func(readCtx context.Context) (runtimeports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
			return s.evidence.InspectCurrentReviewEvidenceApplicabilityV1(readCtx, ref)
		}, knownExpiries...)
		if readErr != nil {
			return externalCutV2{}, false, readErr
		}
		if recovered {
			if err := promote(); err != nil {
				return externalCutV2{}, false, err
			}
		}
		cut.evidence = append(cut.evidence, runtimeports.CloneReviewEvidenceApplicabilityCurrentSnapshotV1(value))
		knownExpiries = append(knownExpiries, value.Projection.ExpiresUnixNano)
	}
	organization, readErr := timedReadV2(readCtx, s.clock, func(readCtx context.Context) (reviewport.HumanOrganizationCurrentCutV2, error) {
		return s.organization.InspectHumanOrganizationCurrentV2(readCtx, cloneOrganizationRequestsV2(subjects.organization))
	})
	if readErr != nil && unknownReadV2(readErr) {
		originalUnknown := normalizeCurrentReadErrorV2(readErr)
		now := s.clock()
		recoveryCtx, cancel, ok := boundedCurrentRecoveryContextV2(readCtx, now, knownExpiries...)
		if !ok {
			return externalCutV2{}, false, clockRegressionV2("human multisign Organization detached recovery clock is unavailable")
		}
		organization, readErr = timedReadAfterV2(recoveryCtx, s.clock, now, func(readCtx context.Context) (reviewport.HumanOrganizationCurrentCutV2, error) {
			return s.organization.InspectHumanOrganizationCurrentV2(readCtx, cloneOrganizationRequestsV2(subjects.organization))
		})
		cancel()
		detached = true
		if readErr != nil && !core.HasReason(readErr, core.ReasonClockRegression) {
			return externalCutV2{}, false, originalUnknown
		}
	}
	if readErr != nil {
		return externalCutV2{}, false, readErr
	}
	cut.organization = organization.Clone()
	cut.policy = cut.policy.Clone()
	return cut, detached, nil
}

func validateExternalCutV2(panel contract.HumanReviewPanelV2, subjects externalSubjectsV2, refs externalRefsV2, cut externalCutV2, now time.Time) error {
	if err := cut.policy.ValidateCurrent(refs.policy, subjects.policy, now); err != nil {
		return err
	}
	if cut.policy.AcceptThreshold != panel.AcceptThreshold || cut.policy.MaximumPanelSize != panel.MaximumPanelSize || cut.policy.DelegationRequired != panel.DelegationRequired || cut.policy.ProductionSelfReviewAllowed != panel.ProductionSelfReviewAllowed || cut.policy.MaxPanelDurationNanos != panel.MaxPanelDurationNanos || cut.policy.MaxVoteTTLNanos != panel.MaxVoteTTLNanos || !sameRoleRequirementsV2(cut.policy.RoleRequirements, panel.RoleRequirements) || !reflect.DeepEqual(cut.policy.RejectVetoRoles, panel.RejectVetoRoles) || cut.policy.CheckedUnixNano != panel.QuorumPolicy.CheckedUnixNano || cut.policy.ExpiresUnixNano != panel.QuorumPolicy.ExpiresUnixNano {
		return currentConflictV2("Panel quorum Policy snapshot drifted from the exact current projection")
	}
	if len(cut.authorities) != len(subjects.authorities) || len(cut.bindings) != len(subjects.bindings) || len(cut.evidence) != len(subjects.evidence) {
		return currentConflictV2("human multisign external current cut cardinality drifted")
	}
	for index := range cut.authorities {
		if err := cut.authorities[index].ValidateCurrent(refs.authorities[index], subjects.authorities[index], now); err != nil {
			return err
		}
	}
	for index := range cut.bindings {
		if err := cut.bindings[index].ValidateCurrent(refs.bindings[index], subjects.bindingSources[index], subjects.bindings[index], now); err != nil {
			return err
		}
	}
	if err := cut.scope.ValidateCurrent(refs.scope, subjects.scope, now); err != nil {
		return err
	}
	for index := range cut.evidence {
		if err := cut.evidence[index].ValidateCurrent(refs.evidence[index], now); err != nil {
			return err
		}
		if cut.evidence[index].Projection.Subject != subjects.evidence[index] {
			return currentConflictV2("human multisign Evidence current subject drifted")
		}
	}
	if err := cut.organization.Validate(now); err != nil {
		return err
	}
	if cut.organization.TenantID != panel.TenantID || len(cut.organization.Items) != len(subjects.organization) {
		return currentConflictV2("human multisign Organization current set drifted")
	}
	byAssignment := make(map[contract.HumanPanelAssignmentExactRefV2]reviewport.HumanOrganizationAssignmentCurrentV2, len(cut.organization.Items))
	for _, item := range cut.organization.Items {
		byAssignment[item.Assignment] = item
	}
	for _, request := range subjects.organization {
		item, ok := byAssignment[request.Assignment.ExactRef()]
		if !ok {
			return currentConflictV2("human multisign Organization current Assignment is missing")
		}
		if err := item.Validate(request, now); err != nil {
			return err
		}
	}
	return nil
}

func sameExternalCutV2(left, right externalCutV2) bool {
	if !reflect.DeepEqual(left.policy, right.policy) || !reflect.DeepEqual(left.authorities, right.authorities) || !reflect.DeepEqual(left.bindings, right.bindings) || !reflect.DeepEqual(left.scope, right.scope) || !reflect.DeepEqual(left.evidence, right.evidence) {
		return false
	}
	// The Organization Reader seals a fresh Review receipt per completed read
	// cut. Compare its immutable Owner items and true TTL, not its receipt time.
	return reflect.DeepEqual(left.organization.Items, right.organization.Items) && left.organization.TenantID == right.organization.TenantID && left.organization.ExpiresUnixNano == right.organization.ExpiresUnixNano
}

func validateAssignmentSetV2(panel contract.HumanReviewPanelV2, assignments []contract.HumanPanelAssignmentV2, now time.Time) error {
	if len(assignments) == 0 || len(assignments) != int(panel.MaximumPanelSize) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human multisign requires the complete Panel Assignment set")
	}
	values := cloneAssignmentsV2(assignments)
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	seenIdentity := map[string]struct{}{}
	for index, assignment := range values {
		if index > 0 && values[index-1].ID == assignment.ID {
			return currentConflictV2("human multisign Assignment set contains a duplicate")
		}
		if assignment.TenantID != panel.TenantID || assignment.Case != panel.Case || assignment.Round != panel.Round || assignment.Target != panel.Target || assignment.Panel.ID != panel.ID || assignment.Panel.TenantID != panel.TenantID {
			return currentConflictV2("human multisign Assignment crosses Panel exact coordinates")
		}
		if panel.State != contract.HumanPanelProposedV2 && !containsAssignmentRefV2(panel.AssignmentRefs, assignment.ExactRef()) {
			return currentConflictV2("human multisign Assignment exact revision is absent from the current Panel")
		}
		if err := assignment.ValidateCurrent(assignment.ExactRef(), now); err != nil {
			return err
		}
		identity := string(assignment.ReviewerIdentity.TenantID) + "\x00" + assignment.ReviewerIdentity.Ref
		if _, ok := seenIdentity[identity]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "human multisign Assignment set repeats one reviewer Identity")
		}
		seenIdentity[identity] = struct{}{}
	}
	return nil
}

func validateOrganizationRequestsV2(panel contract.HumanReviewPanelV2, assignments []contract.HumanPanelAssignmentV2, requests []reviewport.HumanOrganizationCurrentRequestV2) error {
	if len(requests) != len(assignments) {
		return currentConflictV2("Organization coordinate resolver returned an incomplete Assignment set")
	}
	want := make(map[contract.HumanPanelAssignmentExactRefV2]struct{}, len(assignments))
	for _, assignment := range assignments {
		want[assignment.ExactRef()] = struct{}{}
	}
	seen := map[contract.HumanPanelAssignmentExactRefV2]struct{}{}
	for _, request := range requests {
		if err := request.Validate(); err != nil {
			return err
		}
		if request.Panel.ExactRef() != panel.ExactRef() {
			return currentConflictV2("Organization coordinate request drifted from Panel")
		}
		ref := request.Assignment.ExactRef()
		if _, ok := want[ref]; !ok {
			return currentConflictV2("Organization coordinate request contains an unknown Assignment")
		}
		if _, ok := seen[ref]; ok {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "Organization coordinate request duplicates an Assignment")
		}
		seen[ref] = struct{}{}
	}
	return nil
}

func exactReadV2[T any](ctx context.Context, clock func() time.Time, read func(context.Context) (T, error), expiries ...int64) (T, bool, error) {
	baseline := clock()
	if baseline.IsZero() {
		var zero T
		return zero, false, clockRegressionV2("human multisign exact read baseline clock is unavailable")
	}
	value, err := read(ctx)
	after := clock()
	if after.IsZero() || after.Before(baseline) {
		var zero T
		return zero, false, clockRegressionV2("human multisign clock regressed across exact Owner read")
	}
	err = normalizeCurrentReadErrorV2(err)
	if err == nil || !unknownReadV2(err) {
		return value, false, err
	}
	originalUnknown := normalizeCurrentReadErrorV2(err)
	recoveryCtx, cancel, ok := boundedCurrentRecoveryContextV2(ctx, after, expiries...)
	if !ok {
		var zero T
		return zero, true, clockRegressionV2("human multisign detached exact recovery clock is unavailable")
	}
	defer cancel()
	value, err = timedReadAfterV2(recoveryCtx, clock, after, read)
	if err != nil && !core.HasReason(err, core.ReasonClockRegression) {
		var zero T
		return zero, true, originalUnknown
	}
	return value, true, err
}

func timedReadAfterV2[T any](ctx context.Context, clock func() time.Time, previous time.Time, read func(context.Context) (T, error)) (T, error) {
	baseline := clock()
	if baseline.IsZero() || baseline.Before(previous) {
		var zero T
		return zero, clockRegressionV2("human multisign clock regressed before detached Owner recovery")
	}
	value, err := read(ctx)
	now := clock()
	if now.IsZero() || now.Before(baseline) {
		var zero T
		return zero, clockRegressionV2("human multisign clock regressed across detached Owner recovery")
	}
	return value, normalizeCurrentReadErrorV2(err)
}

func exactReadMergeV2[T any](readCtx context.Context, detached bool, clock func() time.Time, read func(context.Context) (T, error), expiries ...int64) (T, bool, error) {
	value, recovered, err := exactReadV2(readCtx, clock, read, expiries...)
	if recovered && !detached {
		detached = true
	}
	return value, detached, err
}

func timedReadV2[T any](ctx context.Context, clock func() time.Time, read func(context.Context) (T, error)) (T, error) {
	baseline := clock()
	if baseline.IsZero() {
		var zero T
		return zero, clockRegressionV2("human multisign Owner read baseline clock is unavailable")
	}
	value, err := read(ctx)
	now := clock()
	if now.IsZero() || now.Before(baseline) {
		var zero T
		return zero, clockRegressionV2("human multisign clock regressed across Owner read")
	}
	return value, normalizeCurrentReadErrorV2(err)
}

func boundedCurrentRecoveryContextV2(parent context.Context, now time.Time, expiries ...int64) (context.Context, context.CancelFunc, bool) {
	if now.IsZero() || now.UnixNano() <= 0 {
		return nil, nil, false
	}
	limit := maximumDetachedCurrentRecoveryV2
	for _, expiry := range expiries {
		if expiry == 0 {
			continue
		}
		if expiry <= now.UnixNano() {
			return nil, nil, false
		}
		remaining := time.Duration(expiry - now.UnixNano())
		if remaining < limit {
			limit = remaining
		}
	}
	if limit <= 0 {
		return nil, nil, false
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), limit)
	return ctx, cancel, true
}

func unknownReadV2(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || core.HasCategory(err, core.ErrorIndeterminate) || core.HasCategory(err, core.ErrorUnavailable)
}

func normalizeCurrentReadErrorV2(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "human multisign Owner read completion is unknown")
	}
	return err
}

func clockRegressionV2(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, message)
}

func currentConflictV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, message)
}

func cloneAssignmentsV2(values []contract.HumanPanelAssignmentV2) []contract.HumanPanelAssignmentV2 {
	out := append([]contract.HumanPanelAssignmentV2(nil), values...)
	for index := range out {
		out[index] = out[index].Clone()
	}
	return out
}

func cloneOrganizationRequestsV2(values []reviewport.HumanOrganizationCurrentRequestV2) []reviewport.HumanOrganizationCurrentRequestV2 {
	out := append([]reviewport.HumanOrganizationCurrentRequestV2(nil), values...)
	for index := range out {
		out[index] = out[index].Clone()
	}
	return out
}

func containsAssignmentV2(values []contract.HumanPanelAssignmentV2, ref contract.HumanPanelAssignmentExactRefV2) bool {
	for _, value := range values {
		if value.ExactRef() == ref {
			return true
		}
	}
	return false
}

func containsAssignmentRefV2(values []contract.HumanPanelAssignmentExactRefV2, ref contract.HumanPanelAssignmentExactRefV2) bool {
	for _, value := range values {
		if value == ref {
			return true
		}
	}
	return false
}

func sameVerdictAssignmentClosureV2(assignments []contract.HumanPanelAssignmentV2, verdict contract.HumanVerdictV2) bool {
	if len(verdict.ReviewerAuthorityRefs) == 0 || len(verdict.ReviewerAuthorityRefs) != len(verdict.BindingClosures) {
		return false
	}
	authorities := make(map[runtimeports.AuthorityBindingRefV2]struct{}, len(verdict.ReviewerAuthorityRefs))
	bindings := make(map[runtimeports.ReviewComponentBindingRefV2]struct{}, len(verdict.BindingClosures))
	for _, value := range verdict.ReviewerAuthorityRefs {
		authorities[value] = struct{}{}
	}
	for _, value := range verdict.BindingClosures {
		bindings[value] = struct{}{}
	}
	matched := 0
	for _, assignment := range assignments {
		_, hasAuthority := authorities[assignment.ReviewerAuthority]
		_, hasBinding := bindings[assignment.ReviewerBinding]
		if hasAuthority != hasBinding {
			return false
		}
		if hasAuthority {
			matched++
		}
	}
	return matched == len(verdict.ReviewerAuthorityRefs) && len(authorities) == matched && len(bindings) == matched
}

func sameRoleRequirementsV2(left []runtimeports.HumanQuorumRoleRequirementV2, right []contract.HumanRoleRequirementV2) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Role != right[index].Role || left[index].Minimum != right[index].Minimum {
			return false
		}
	}
	return true
}

func minimumExpiryV2(values ...int64) int64 {
	var result int64
	for _, value := range values {
		if value > 0 && (result == 0 || value < result) {
			result = value
		}
	}
	return result
}

var _ multisigowner.ExternalCurrentCutV2 = (*ExternalSourceV2)(nil)
