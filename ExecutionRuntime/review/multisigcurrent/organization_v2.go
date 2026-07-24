// Package multisigcurrent composes read-only external Owner current cuts for
// Review Human Multi-Sign V2. It owns no Organization facts or mutations.
package multisigcurrent

import (
	"context"
	"reflect"
	"sort"
	"time"

	organizationcontract "github.com/Proview-China/rax/ExecutionRuntime/organization-engine/contract"
	organizationports "github.com/Proview-China/rax/ExecutionRuntime/organization-engine/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// OrganizationSourceV2 is the Review-owned consumer adapter over the public
// Organization current Reader. It never receives an Organization write port.
type OrganizationSourceV2 struct {
	reader organizationports.ReviewEligibilityCurrentReaderV1
	clock  func() time.Time
}

func NewOrganizationSourceV2(reader organizationports.ReviewEligibilityCurrentReaderV1, clock func() time.Time) (*OrganizationSourceV2, error) {
	if nilcheck.IsNil(reader) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "human Organization current source requires the Owner reader and a clock")
	}
	return &OrganizationSourceV2{reader: reader, clock: clock}, nil
}

func (s *OrganizationSourceV2) InspectHumanOrganizationCurrentV2(ctx context.Context, requests []reviewport.HumanOrganizationCurrentRequestV2) (reviewport.HumanOrganizationCurrentCutV2, error) {
	if s == nil || nilcheck.IsNil(s.reader) || s.clock == nil {
		return reviewport.HumanOrganizationCurrentCutV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "human Organization current source is incomplete")
	}
	if len(requests) == 0 || len(requests) > 64 {
		return reviewport.HumanOrganizationCurrentCutV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "human Organization current request set is empty or unbounded")
	}
	baseline, err := s.fresh(time.Time{}, "human Organization cut baseline clock is unavailable")
	if err != nil {
		return reviewport.HumanOrganizationCurrentCutV2{}, err
	}
	requests = append([]reviewport.HumanOrganizationCurrentRequestV2(nil), requests...)
	for i := range requests {
		requests[i] = requests[i].Clone()
		if err := requests[i].Validate(); err != nil {
			return reviewport.HumanOrganizationCurrentCutV2{}, err
		}
	}
	sort.Slice(requests, func(i, j int) bool {
		if requests[i].Assignment.ID != requests[j].Assignment.ID {
			return requests[i].Assignment.ID < requests[j].Assignment.ID
		}
		if requests[i].Assignment.Revision != requests[j].Assignment.Revision {
			return requests[i].Assignment.Revision < requests[j].Assignment.Revision
		}
		return requests[i].Assignment.Digest < requests[j].Assignment.Digest
	})
	tenant := requests[0].Panel.TenantID
	items := make([]reviewport.HumanOrganizationAssignmentCurrentV2, 0, len(requests))
	last := baseline
	for index, request := range requests {
		if request.Panel.TenantID != tenant || (index > 0 && requests[index-1].Assignment.ID == request.Assignment.ID) {
			return reviewport.HumanOrganizationCurrentCutV2{}, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "human Organization request set crosses tenant or duplicates an Assignment")
		}
		item, _, checked, readErr := s.inspectOne(ctx, request, last)
		if readErr != nil {
			return reviewport.HumanOrganizationCurrentCutV2{}, readErr
		}
		last = checked
		items = append(items, item)
	}
	now, err := s.fresh(last, "human Organization clock regressed after the current set")
	if err != nil {
		return reviewport.HumanOrganizationCurrentCutV2{}, err
	}
	expires := int64(0)
	for _, item := range items {
		if expires == 0 || item.ExpiresUnixNano < expires {
			expires = item.ExpiresUnixNano
		}
	}
	cut, err := reviewport.SealHumanOrganizationCurrentCutV2(reviewport.HumanOrganizationCurrentCutV2{TenantID: tenant, Items: items, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		return reviewport.HumanOrganizationCurrentCutV2{}, err
	}
	return cut.Clone(), cut.Validate(now)
}

func (s *OrganizationSourceV2) inspectOne(ctx context.Context, request reviewport.HumanOrganizationCurrentRequestV2, previous time.Time) (reviewport.HumanOrganizationAssignmentCurrentV2, bool, time.Time, error) {
	source, err := request.OrganizationSourceV1()
	if err != nil {
		return reviewport.HumanOrganizationAssignmentCurrentV2{}, false, time.Time{}, err
	}
	resolved, readChecked, err := s.read(previous, ctx, func(readCtx context.Context) (organizationcontract.ReviewEligibilityCurrentProjectionV1, error) {
		return s.reader.ResolveCurrentReviewEligibilityV1(readCtx, source)
	})
	detached := false
	if err != nil && unknownCurrentReadV2(err) {
		originalUnknown := normalizeCurrentReadErrorV2(err)
		// Resolve has no expected ref. This is a new S1, not recovery of the
		// unknown response, and it uses one bounded context detached from the caller.
		recoveryCtx, cancel, ok := boundedCurrentRecoveryContextV2(ctx, readChecked, request.Panel.ExpiresUnixNano, request.Assignment.ExpiresUnixNano, request.Assignment.LeaseExpiresUnixNano)
		if !ok {
			return reviewport.HumanOrganizationAssignmentCurrentV2{}, false, time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human Organization detached Resolve recovery crossed current TTL")
		}
		detached = true
		resolved, readChecked, err = s.read(readChecked, recoveryCtx, func(retryCtx context.Context) (organizationcontract.ReviewEligibilityCurrentProjectionV1, error) {
			return s.reader.ResolveCurrentReviewEligibilityV1(retryCtx, source)
		})
		cancel()
		if err != nil && !core.HasReason(err, core.ReasonClockRegression) {
			return reviewport.HumanOrganizationAssignmentCurrentV2{}, true, time.Time{}, originalUnknown
		}
	}
	if err != nil {
		return reviewport.HumanOrganizationAssignmentCurrentV2{}, detached, time.Time{}, err
	}
	resolvedAt, err := s.fresh(readChecked, "human Organization clock regressed after Resolve")
	if err != nil {
		return reviewport.HumanOrganizationAssignmentCurrentV2{}, detached, time.Time{}, err
	}
	if err := validateOrganizationProjectionV2(request, source, resolved, resolvedAt); err != nil {
		return reviewport.HumanOrganizationAssignmentCurrentV2{}, detached, time.Time{}, err
	}
	ref := resolved.Ref.Clone()
	s1, recovered, s1Checked, err := s.readExact(ctx, ref, resolvedAt, request.Panel.ExpiresUnixNano, request.Assignment.ExpiresUnixNano, request.Assignment.LeaseExpiresUnixNano, resolved.ExpiresUnixNano)
	detached = detached || recovered
	if err != nil {
		return reviewport.HumanOrganizationAssignmentCurrentV2{}, detached, time.Time{}, err
	}
	afterS1, err := s.fresh(s1Checked, "human Organization clock regressed across S1")
	if err != nil {
		return reviewport.HumanOrganizationAssignmentCurrentV2{}, detached, time.Time{}, err
	}
	if err := validateOrganizationProjectionV2(request, source, s1, afterS1); err != nil {
		return reviewport.HumanOrganizationAssignmentCurrentV2{}, detached, time.Time{}, err
	}
	s2, recovered, s2Checked, err := s.readExact(ctx, ref, afterS1, request.Panel.ExpiresUnixNano, request.Assignment.ExpiresUnixNano, request.Assignment.LeaseExpiresUnixNano, s1.ExpiresUnixNano)
	detached = detached || recovered
	if err != nil {
		return reviewport.HumanOrganizationAssignmentCurrentV2{}, detached, time.Time{}, err
	}
	now, err := s.fresh(s2Checked, "human Organization clock regressed across S2")
	if err != nil {
		return reviewport.HumanOrganizationAssignmentCurrentV2{}, detached, time.Time{}, err
	}
	if err := validateOrganizationProjectionV2(request, source, s2, now); err != nil {
		return reviewport.HumanOrganizationAssignmentCurrentV2{}, detached, time.Time{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return reviewport.HumanOrganizationAssignmentCurrentV2{}, detached, time.Time{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human Organization projection drifted between exact S1 and S2")
	}
	requestDigest, err := request.Digest()
	if err != nil {
		return reviewport.HumanOrganizationAssignmentCurrentV2{}, detached, time.Time{}, err
	}
	item := reviewport.HumanOrganizationAssignmentCurrentV2{
		RequestDigest:      requestDigest,
		Assignment:         request.Assignment.ExactRef(),
		ReviewerIdentity:   request.Assignment.ReviewerIdentity,
		OwnerProjectionRef: ref.Clone(),
		CheckedUnixNano:    s2.CheckedUnixNano,
		ExpiresUnixNano:    s2.ExpiresUnixNano,
		ProjectionDigest:   s2.ProjectionDigest,
	}
	if err := item.Validate(request, now); err != nil {
		return reviewport.HumanOrganizationAssignmentCurrentV2{}, detached, time.Time{}, err
	}
	return item.Clone(), detached, now, nil
}

func (s *OrganizationSourceV2) readExact(ctx context.Context, ref organizationcontract.ReviewEligibilityProjectionRefV1, previous time.Time, expiries ...int64) (organizationcontract.ReviewEligibilityCurrentProjectionV1, bool, time.Time, error) {
	value, readChecked, err := s.read(previous, ctx, func(readCtx context.Context) (organizationcontract.ReviewEligibilityCurrentProjectionV1, error) {
		return s.reader.InspectCurrentReviewEligibilityV1(readCtx, ref)
	})
	if err == nil || !unknownCurrentReadV2(err) {
		return value.Clone(), false, readChecked, err
	}
	originalUnknown := normalizeCurrentReadErrorV2(err)
	recoveryCtx, cancel, ok := boundedCurrentRecoveryContextV2(ctx, readChecked, expiries...)
	if !ok {
		return organizationcontract.ReviewEligibilityCurrentProjectionV1{}, true, time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human Organization detached exact recovery crossed current TTL")
	}
	defer cancel()
	value, readChecked, err = s.read(readChecked, recoveryCtx, func(readCtx context.Context) (organizationcontract.ReviewEligibilityCurrentProjectionV1, error) {
		return s.reader.InspectCurrentReviewEligibilityV1(readCtx, ref)
	})
	if err != nil && !core.HasReason(err, core.ReasonClockRegression) {
		return organizationcontract.ReviewEligibilityCurrentProjectionV1{}, true, time.Time{}, originalUnknown
	}
	return value.Clone(), true, readChecked, err
}

func (s *OrganizationSourceV2) read(previous time.Time, ctx context.Context, fn func(context.Context) (organizationcontract.ReviewEligibilityCurrentProjectionV1, error)) (organizationcontract.ReviewEligibilityCurrentProjectionV1, time.Time, error) {
	before, err := s.fresh(previous, "human Organization clock regressed before Owner read")
	if err != nil {
		return organizationcontract.ReviewEligibilityCurrentProjectionV1{}, time.Time{}, err
	}
	value, readErr := fn(ctx)
	after, err := s.fresh(before, "human Organization clock regressed across Owner read")
	if err != nil {
		return organizationcontract.ReviewEligibilityCurrentProjectionV1{}, time.Time{}, err
	}
	return value.Clone(), after, normalizeCurrentReadErrorV2(readErr)
}

func (s *OrganizationSourceV2) fresh(previous time.Time, message string) (time.Time, error) {
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= 0 || (!previous.IsZero() && now.Before(previous)) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, message)
	}
	return now, nil
}

func validateOrganizationProjectionV2(request reviewport.HumanOrganizationCurrentRequestV2, source organizationcontract.ReviewEligibilitySourceV1, projection organizationcontract.ReviewEligibilityCurrentProjectionV1, now time.Time) error {
	if err := projection.ValidateCurrent(projection.Ref, now); err != nil {
		return err
	}
	if !reflect.DeepEqual(projection.Source, source) || !reflect.DeepEqual(projection.Ref.Source, source) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Organization projection source drifted from the Review request")
	}
	assignment := request.Assignment
	panel := request.Panel
	identity := projection.Identity.ExactRef()
	if identity.TenantID != assignment.ReviewerIdentity.TenantID || identity.ID != assignment.ReviewerIdentity.Ref || identity.Revision != assignment.ReviewerIdentity.Revision || identity.Digest != assignment.ReviewerIdentity.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Organization reviewer identity drifted from the Assignment")
	}
	if len(projection.Roles) != len(assignment.Roles) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Organization role closure is incomplete")
	}
	canVeto := false
	for index, role := range projection.Roles {
		if role.Role != assignment.Roles[index] || role.ScopeDigest != request.ActionScopeDigest || role.Identity != projection.Identity.ExactRef() {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Organization role closure drifted from the Assignment")
		}
		canVeto = canVeto || role.CanVeto
	}
	if canVeto != assignment.CanVeto {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Organization veto eligibility drifted from the Assignment")
	}
	responsibility := projection.Responsibility.ExactRef()
	if responsibility.TenantID != panel.ResponsibilitySubject.TenantID || responsibility.ID != panel.ResponsibilitySubject.Ref || responsibility.Revision != panel.ResponsibilitySubject.Revision || responsibility.Digest != panel.ResponsibilitySubject.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Organization responsibility fact drifted from the Panel")
	}
	responsibleIdentity := projection.ResponsibilityIdentity.ExactRef()
	if responsibleIdentity.TenantID != panel.ResponsibilitySubject.IdentityProof.TenantID || responsibleIdentity.ID != panel.ResponsibilitySubject.IdentityProof.Ref || responsibleIdentity.Revision != panel.ResponsibilitySubject.IdentityProof.Revision || responsibleIdentity.Digest != panel.ResponsibilitySubject.IdentityProof.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Organization responsibility identity drifted from the Panel")
	}
	if assignment.Delegated {
		if projection.Delegation == nil || projection.DelegatorIdentity == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "Organization delegation current proof is missing")
		}
		delegation := projection.Delegation.ExactRef()
		delegator := projection.DelegatorIdentity.ExactRef()
		if delegation.TenantID != assignment.DelegationFact.TenantID || delegation.ID != assignment.DelegationFact.Ref || delegation.Revision != assignment.DelegationFact.Revision || delegation.Digest != assignment.DelegationFact.Digest || delegator.TenantID != assignment.DelegatorIdentity.TenantID || delegator.ID != assignment.DelegatorIdentity.Ref || delegator.Revision != assignment.DelegatorIdentity.Revision || delegator.Digest != assignment.DelegatorIdentity.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "Organization delegation closure drifted from the Assignment")
		}
	} else if projection.Delegation != nil || projection.DelegatorIdentity != nil {
		return core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "direct Assignment received an Organization delegation")
	}
	return nil
}

func unknownCurrentReadV2(err error) bool {
	return unknownReadV2(err)
}

var _ reviewport.HumanOrganizationCurrentReaderV2 = (*OrganizationSourceV2)(nil)
