package current

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ReaderV1 struct {
	Store ports.StoreV1
	Clock func() time.Time
}

var _ ports.ReviewEligibilityCurrentReaderV1 = (*ReaderV1)(nil)

func NewReaderV1(store ports.StoreV1, clock func() time.Time) (*ReaderV1, error) {
	if nilish(store) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonOwnerMissing, "organization store is required")
	}
	if clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonOwnerMissing, "organization clock is required")
	}
	return &ReaderV1{store, clock}, nil
}

func (r *ReaderV1) ResolveCurrentReviewEligibilityV1(ctx context.Context, source contract.ReviewEligibilitySourceV1) (contract.ReviewEligibilityCurrentProjectionV1, error) {
	if r == nil || nilish(r.Store) || r.Clock == nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonOwnerMissing, "organization reader is incomplete")
	}
	if err := source.Validate(); err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	baseline, err := fresh(r.Clock)
	if err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	s1, err := r.Store.ReadReviewEligibilityClosureV1(ctx, source)
	if err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	s2, err := r.Store.ReadReviewEligibilityClosureV1(ctx, source)
	if err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	if !sameClosure(s1, s2) {
		return contract.ReviewEligibilityCurrentProjectionV1{}, ports.ConflictV1("organization S1/S2 closure drifted")
	}
	checked, err := fresh(r.Clock)
	if err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	if checked.Before(baseline) {
		return contract.ReviewEligibilityCurrentProjectionV1{}, clockRegression()
	}
	if checked.Before(baseline) {
		return contract.ReviewEligibilityCurrentProjectionV1{}, clockRegression()
	}
	candidate, err := build(source, s2, checked)
	if err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	stored, err := r.Store.CreateOrInspectReviewEligibilityProjectionV1(ctx, candidate)
	readCtx := ctx
	if err != nil {
		if core.HasCategory(err, core.ErrorIndeterminate) {
			readCtx = context.WithoutCancel(ctx)
			recovered, recoveryErr := r.Store.InspectReviewEligibilityProjectionV1(readCtx, candidate.Ref)
			if recoveryErr == nil {
				stored = recovered
				err = nil
			}
		}
		if err != nil {
			return contract.ReviewEligibilityCurrentProjectionV1{}, err
		}
	}
	if err = validateClosureCurrent(source, s2, checked); err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	if err = stored.ValidateCurrent(stored.Ref, checked); err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	if !projectionMatchesClosure(stored, s2) {
		return contract.ReviewEligibilityCurrentProjectionV1{}, ports.ConflictV1("sealed projection does not match current closure")
	}
	return stored.Clone(), nil
}

func (r *ReaderV1) InspectCurrentReviewEligibilityV1(ctx context.Context, ref contract.ReviewEligibilityProjectionRefV1) (contract.ReviewEligibilityCurrentProjectionV1, error) {
	if r == nil || nilish(r.Store) || r.Clock == nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonOwnerMissing, "organization reader is incomplete")
	}
	if err := ref.Validate(); err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	baseline, err := fresh(r.Clock)
	if err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	p, err := r.Store.InspectReviewEligibilityProjectionV1(ctx, ref)
	readCtx := ctx
	if err != nil && core.HasCategory(err, core.ErrorIndeterminate) {
		readCtx = context.WithoutCancel(ctx)
		p, err = r.Store.InspectReviewEligibilityProjectionV1(readCtx, ref)
	}
	if err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	s1, err := r.Store.ReadReviewEligibilityClosureV1(readCtx, ref.Source)
	if err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	s2, err := r.Store.ReadReviewEligibilityClosureV1(readCtx, ref.Source)
	if err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	if !sameClosure(s1, s2) || !projectionMatchesClosure(p, s2) {
		return contract.ReviewEligibilityCurrentProjectionV1{}, ports.ConflictV1("organization exact projection current closure drifted")
	}
	now, err := fresh(r.Clock)
	if err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	if now.Before(baseline) {
		return contract.ReviewEligibilityCurrentProjectionV1{}, clockRegression()
	}
	if err = validateClosureCurrent(ref.Source, s2, now); err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	if err = p.ValidateCurrent(ref, now); err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	return p.Clone(), nil
}

func build(source contract.ReviewEligibilitySourceV1, c ports.ReviewEligibilityClosureV1, checked time.Time) (contract.ReviewEligibilityCurrentProjectionV1, error) {
	if err := validateClosureCurrent(source, c, checked); err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	expires := c.Identity.ExpiresUnixNano
	all := []int64{c.Responsibility.ExpiresUnixNano, c.ResponsibilityIdentity.ExpiresUnixNano}
	for _, x := range c.Roles {
		all = append(all, x.ExpiresUnixNano)
	}
	if c.Delegation != nil {
		all = append(all, c.Delegation.ExpiresUnixNano, c.DelegatorIdentity.ExpiresUnixNano)
	}
	for _, x := range all {
		if x < expires {
			expires = x
		}
	}
	return contract.SealReviewEligibilityCurrentProjectionV1(contract.ReviewEligibilityCurrentProjectionV1{Source: source.Clone(), Identity: c.Identity, DelegatorIdentity: c.DelegatorIdentity, ResponsibilityIdentity: c.ResponsibilityIdentity, Roles: append([]contract.RoleGrantFactV1(nil), c.Roles...), Delegation: c.Delegation, Responsibility: c.Responsibility, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires})
}

func validateClosureCurrent(source contract.ReviewEligibilitySourceV1, c ports.ReviewEligibilityClosureV1, now time.Time) error {
	if err := c.Identity.ValidateCurrent(c.Identity.ExactRef(), now); err != nil {
		return err
	}
	if c.Identity.SubjectKind != contract.SubjectHumanV1 || c.Identity.SubjectID != source.ReviewerSubjectID {
		return ports.ConflictV1("reviewer identity source drifted")
	}
	if len(c.Roles) != len(source.RequiredRoles) {
		return ports.ConflictV1("required role closure is incomplete")
	}
	sort.Slice(c.Roles, func(i, j int) bool { return c.Roles[i].Role < c.Roles[j].Role })
	for i, role := range c.Roles {
		if err := role.ValidateCurrent(role.ExactRef(), now); err != nil {
			return err
		}
		if role.Identity != c.Identity.ExactRef() || role.Role != source.RequiredRoles[i] || role.ScopeDigest != source.ScopeDigest {
			return ports.ConflictV1("role current closure drifted")
		}
	}
	if err := c.Responsibility.ValidateCurrent(c.Responsibility.ExactRef(), now); err != nil {
		return err
	}
	if err := c.ResponsibilityIdentity.ValidateCurrent(c.ResponsibilityIdentity.ExactRef(), now); err != nil {
		return err
	}
	if c.Responsibility.Identity != c.ResponsibilityIdentity.ExactRef() || c.Responsibility.SubjectKind != source.ResponsibilitySubjectKind || c.Responsibility.SubjectID != source.ResponsibilitySubjectID || c.Responsibility.SubjectDigest != source.ResponsibilitySubjectDigest {
		return ports.ConflictV1("responsibility current closure drifted")
	}
	if source.Production && c.ResponsibilityIdentity.SubjectID == c.Identity.SubjectID {
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "production self-review is forbidden")
	}
	if source.RequireDelegation {
		if c.Delegation == nil || c.DelegatorIdentity == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "delegation current proof is missing")
		}
		if err := c.Delegation.ValidateCurrent(c.Delegation.ExactRef(), now); err != nil {
			return err
		}
		if err := c.DelegatorIdentity.ValidateCurrent(c.DelegatorIdentity.ExactRef(), now); err != nil {
			return err
		}
		if c.Delegation.Delegator != c.DelegatorIdentity.ExactRef() || c.Delegation.Delegate != c.Identity.ExactRef() || c.Delegation.DelegatorSubjectID != source.DelegatorSubjectID || c.Delegation.DelegateSubjectID != source.ReviewerSubjectID || c.Delegation.Role != source.DelegatedRole || c.Delegation.ScopeDigest != source.ScopeDigest {
			return core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "delegation current proof drifted")
		}
	} else if c.Delegation != nil || c.DelegatorIdentity != nil {
		return ports.ConflictV1("direct reviewer cannot carry delegation")
	}
	return nil
}

func sameClosure(a, b ports.ReviewEligibilityClosureV1) bool { return closureRefs(a) == closureRefs(b) }
func closureRefs(c ports.ReviewEligibilityClosureV1) string {
	v := fmt.Sprintf("%s/%d/%s|%s/%d/%s|%s/%d/%s", c.Identity.ID, c.Identity.Revision, c.Identity.Digest, c.Responsibility.ID, c.Responsibility.Revision, c.Responsibility.Digest, c.ResponsibilityIdentity.ID, c.ResponsibilityIdentity.Revision, c.ResponsibilityIdentity.Digest)
	for _, r := range c.Roles {
		v += fmt.Sprintf("|%s/%s/%d/%s", r.Role, r.ID, r.Revision, r.Digest)
	}
	if c.Delegation != nil {
		v += fmt.Sprintf("|%s/%d/%s/%s/%d/%s", c.Delegation.ID, c.Delegation.Revision, c.Delegation.Digest, c.DelegatorIdentity.ID, c.DelegatorIdentity.Revision, c.DelegatorIdentity.Digest)
	}
	return v
}
func projectionMatchesClosure(p contract.ReviewEligibilityCurrentProjectionV1, c ports.ReviewEligibilityClosureV1) bool {
	candidate, err := build(p.Source, c, time.Unix(0, p.CheckedUnixNano))
	return err == nil && candidate.Ref.ID == p.Ref.ID && closureRefs(c) == closureRefs(ports.ReviewEligibilityClosureV1{Identity: p.Identity, DelegatorIdentity: p.DelegatorIdentity, ResponsibilityIdentity: p.ResponsibilityIdentity, Roles: p.Roles, Delegation: p.Delegation, Responsibility: p.Responsibility})
}
func fresh(clock func() time.Time) (time.Time, error) {
	v := clock()
	if v.IsZero() || v.UnixNano() <= 0 {
		return time.Time{}, clockRegression()
	}
	return v, nil
}
func clockRegression() error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "organization current clock regressed")
}
func nilish(v any) bool {
	if v == nil {
		return true
	}
	x := reflect.ValueOf(v)
	switch x.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return x.IsNil()
	}
	return false
}
