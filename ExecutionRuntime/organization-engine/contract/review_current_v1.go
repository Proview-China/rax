package contract

import (
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ReviewEligibilitySourceV1 struct {
	TenantID                    core.TenantID `json:"tenant_id"`
	ReviewerSubjectID           string        `json:"reviewer_subject_id"`
	RequiredRoles               []string      `json:"required_roles"`
	ScopeDigest                 core.Digest   `json:"scope_digest"`
	ResponsibilitySubjectKind   string        `json:"responsibility_subject_kind"`
	ResponsibilitySubjectID     string        `json:"responsibility_subject_id"`
	ResponsibilitySubjectDigest core.Digest   `json:"responsibility_subject_digest"`
	DelegatorSubjectID          string        `json:"delegator_subject_id,omitempty"`
	DelegatedRole               string        `json:"delegated_role,omitempty"`
	RequireDelegation           bool          `json:"require_delegation"`
	Production                  bool          `json:"production"`
}

func (s ReviewEligibilitySourceV1) Clone() ReviewEligibilitySourceV1 {
	s.RequiredRoles = append([]string(nil), s.RequiredRoles...)
	return s
}
func (s ReviewEligibilitySourceV1) Validate() error {
	if strings.TrimSpace(string(s.TenantID)) == "" || invalidText(s.ReviewerSubjectID) || invalidText(s.ResponsibilitySubjectKind) || invalidText(s.ResponsibilitySubjectID) {
		return invalid("review eligibility source is incomplete")
	}
	if err := s.ScopeDigest.Validate(); err != nil {
		return err
	}
	if err := s.ResponsibilitySubjectDigest.Validate(); err != nil {
		return err
	}
	roles, err := CanonicalRolesV1(s.RequiredRoles)
	if err != nil {
		return err
	}
	if len(roles) != len(s.RequiredRoles) {
		return invalid("roles are not canonical")
	}
	for i := range roles {
		if roles[i] != s.RequiredRoles[i] {
			return invalid("roles are not sorted")
		}
	}
	if s.RequireDelegation != (s.DelegatorSubjectID != "" && s.DelegatedRole != "") || (!s.RequireDelegation && s.DelegatedRole != "") {
		return core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "delegation source is inconsistent")
	}
	if s.RequireDelegation && !contains(s.RequiredRoles, s.DelegatedRole) {
		return core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "delegated role is not required")
	}
	return nil
}

type ReviewEligibilityProjectionRefV1 struct {
	TenantID       core.TenantID             `json:"tenant_id"`
	ID             string                    `json:"id"`
	Source         ReviewEligibilitySourceV1 `json:"source"`
	Identity       IdentityRefV1             `json:"identity"`
	Roles          []RoleGrantRefV1          `json:"roles"`
	Delegation     *DelegationRefV1          `json:"delegation,omitempty"`
	Responsibility ResponsibilityRefV1       `json:"responsibility"`
	Digest         core.Digest               `json:"digest"`
}

func (r ReviewEligibilityProjectionRefV1) Clone() ReviewEligibilityProjectionRefV1 {
	r.Source = r.Source.Clone()
	r.Roles = append([]RoleGrantRefV1(nil), r.Roles...)
	if r.Delegation != nil {
		d := *r.Delegation
		r.Delegation = &d
	}
	return r
}
func (r ReviewEligibilityProjectionRefV1) Validate() error {
	if strings.TrimSpace(string(r.TenantID)) == "" || invalidText(r.ID) || len(r.Roles) == 0 || len(r.Roles) > MaxRolesV1 {
		return invalid("eligibility exact ref is incomplete")
	}
	if err := r.Source.Validate(); err != nil {
		return err
	}
	if r.Source.TenantID != r.TenantID {
		return conflict("eligibility source crosses tenant")
	}
	if err := r.Identity.Validate(); err != nil {
		return err
	}
	if err := r.Responsibility.Validate(); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, role := range r.Roles {
		if err := role.Validate(); err != nil {
			return err
		}
		key := role.ID + "\x00" + string(role.Digest)
		if role.TenantID != r.TenantID {
			return conflict("eligibility role refs are not unique canonical refs")
		}
		if _, ok := seen[key]; ok {
			return conflict("eligibility role refs are duplicated")
		}
		seen[key] = struct{}{}
	}
	if r.Identity.TenantID != r.TenantID || r.Responsibility.TenantID != r.TenantID {
		return conflict("eligibility refs cross tenant")
	}
	if r.Delegation != nil {
		if err := r.Delegation.Validate(); err != nil {
			return err
		}
		if r.Delegation.TenantID != r.TenantID {
			return conflict("eligibility delegation crosses tenant")
		}
	}
	return r.Digest.Validate()
}

type ReviewEligibilityCurrentProjectionV1 struct {
	ContractVersion        string                           `json:"contract_version"`
	Ref                    ReviewEligibilityProjectionRefV1 `json:"ref"`
	Source                 ReviewEligibilitySourceV1        `json:"source"`
	Identity               IdentityFactV1                   `json:"identity"`
	DelegatorIdentity      *IdentityFactV1                  `json:"delegator_identity,omitempty"`
	ResponsibilityIdentity IdentityFactV1                   `json:"responsibility_identity"`
	Roles                  []RoleGrantFactV1                `json:"roles"`
	Delegation             *DelegationFactV1                `json:"delegation,omitempty"`
	Responsibility         ResponsibilityFactV1             `json:"responsibility"`
	Current                bool                             `json:"current"`
	CheckedUnixNano        int64                            `json:"checked_unix_nano"`
	ExpiresUnixNano        int64                            `json:"expires_unix_nano"`
	ProjectionDigest       core.Digest                      `json:"projection_digest"`
}

func (p ReviewEligibilityCurrentProjectionV1) Clone() ReviewEligibilityCurrentProjectionV1 {
	p.Ref = p.Ref.Clone()
	p.Source = p.Source.Clone()
	p.Roles = append([]RoleGrantFactV1(nil), p.Roles...)
	if p.Delegation != nil {
		d := *p.Delegation
		p.Delegation = &d
	}
	if p.DelegatorIdentity != nil {
		d := *p.DelegatorIdentity
		p.DelegatorIdentity = &d
	}
	return p
}
func (p ReviewEligibilityCurrentProjectionV1) digestValue() ReviewEligibilityCurrentProjectionV1 {
	p = p.Clone()
	p.ProjectionDigest = ""
	p.Ref.Digest = ""
	return p
}

func SealReviewEligibilityCurrentProjectionV1(p ReviewEligibilityCurrentProjectionV1) (ReviewEligibilityCurrentProjectionV1, error) {
	p = p.Clone()
	p.ContractVersion = ContractVersionV1
	p.Current = true
	p.ProjectionDigest = ""
	p.Ref.Digest = ""
	sort.Slice(p.Roles, func(i, j int) bool { return p.Roles[i].Role < p.Roles[j].Role })
	p.Ref.Roles = make([]RoleGrantRefV1, len(p.Roles))
	for i := range p.Roles {
		p.Ref.Roles[i] = p.Roles[i].ExactRef()
	}
	p.Ref.Identity, p.Ref.Responsibility = p.Identity.ExactRef(), p.Responsibility.ExactRef()
	p.Ref.TenantID = p.Source.TenantID
	p.Ref.Source = p.Source.Clone()
	if p.Delegation != nil {
		d := p.Delegation.ExactRef()
		p.Ref.Delegation = &d
	}
	id, err := DeriveReviewEligibilityProjectionIDV1(p.Source, p.Ref.Identity, p.Ref.Roles, p.Ref.Delegation, p.Ref.Responsibility)
	if err != nil {
		return ReviewEligibilityCurrentProjectionV1{}, err
	}
	if p.Ref.ID == "" {
		p.Ref.ID = id
	} else if p.Ref.ID != id {
		return ReviewEligibilityCurrentProjectionV1{}, conflict("eligibility projection stable id drifted")
	}
	d, err := digest("ReviewEligibilityCurrentProjectionV1", p.digestValue())
	if err != nil {
		return ReviewEligibilityCurrentProjectionV1{}, err
	}
	p.ProjectionDigest, p.Ref.Digest = d, d
	return p, p.Validate()
}

func (p ReviewEligibilityCurrentProjectionV1) Validate() error {
	if p.ContractVersion != ContractVersionV1 || !p.Current || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano {
		return invalid("eligibility projection window is invalid")
	}
	if err := p.Source.Validate(); err != nil {
		return err
	}
	if err := p.Ref.Validate(); err != nil {
		return err
	}
	if err := p.Identity.Validate(); err != nil {
		return err
	}
	if err := p.Responsibility.Validate(); err != nil {
		return err
	}
	if err := p.ResponsibilityIdentity.Validate(); err != nil {
		return err
	}
	if !equalSource(p.Ref.Source, p.Source) || p.Ref.Identity != p.Identity.ExactRef() || p.Ref.Responsibility != p.Responsibility.ExactRef() || len(p.Roles) != len(p.Ref.Roles) {
		return conflict("projection exact refs drifted")
	}
	minExpiry, maxUpdated := p.Identity.ExpiresUnixNano, p.Identity.UpdatedUnixNano
	for i, role := range p.Roles {
		if err := role.Validate(); err != nil {
			return err
		}
		if role.ExactRef() != p.Ref.Roles[i] || role.Identity != p.Identity.ExactRef() || role.ScopeDigest != p.Source.ScopeDigest || role.Role != p.Source.RequiredRoles[i] {
			return conflict("projection role closure drifted")
		}
		if role.ExpiresUnixNano < minExpiry {
			minExpiry = role.ExpiresUnixNano
		}
		if role.UpdatedUnixNano > maxUpdated {
			maxUpdated = role.UpdatedUnixNano
		}
	}
	if p.Delegation != nil {
		if p.Ref.Delegation == nil || *p.Ref.Delegation != p.Delegation.ExactRef() {
			return conflict("projection delegation ref drifted")
		}
		if p.Delegation.Delegate != p.Identity.ExactRef() || p.Delegation.DelegateSubjectID != p.Source.ReviewerSubjectID || p.Delegation.DelegatorSubjectID != p.Source.DelegatorSubjectID || p.Delegation.ScopeDigest != p.Source.ScopeDigest || p.Delegation.Role != p.Source.DelegatedRole {
			return conflict("projection delegation closure drifted")
		}
		if p.Delegation.ExpiresUnixNano < minExpiry {
			minExpiry = p.Delegation.ExpiresUnixNano
		}
		if p.Delegation.UpdatedUnixNano > maxUpdated {
			maxUpdated = p.Delegation.UpdatedUnixNano
		}
		if p.DelegatorIdentity == nil || p.Delegation.Delegator != p.DelegatorIdentity.ExactRef() {
			return conflict("projection delegator identity drifted")
		}
		if p.DelegatorIdentity.ExpiresUnixNano < minExpiry {
			minExpiry = p.DelegatorIdentity.ExpiresUnixNano
		}
		if p.DelegatorIdentity.UpdatedUnixNano > maxUpdated {
			maxUpdated = p.DelegatorIdentity.UpdatedUnixNano
		}
	} else if p.Source.RequireDelegation || p.Ref.Delegation != nil || p.DelegatorIdentity != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "required delegation is absent")
	}
	if p.Responsibility.SubjectKind != p.Source.ResponsibilitySubjectKind || p.Responsibility.SubjectID != p.Source.ResponsibilitySubjectID || p.Responsibility.SubjectDigest != p.Source.ResponsibilitySubjectDigest {
		return conflict("projection responsibility subject drifted")
	}
	if p.Source.Production && p.Responsibility.Identity.ID == p.Identity.ID {
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "production self-review is forbidden")
	}
	if p.Responsibility.Identity != p.ResponsibilityIdentity.ExactRef() {
		return conflict("projection responsibility identity drifted")
	}
	if p.ResponsibilityIdentity.ExpiresUnixNano < minExpiry {
		minExpiry = p.ResponsibilityIdentity.ExpiresUnixNano
	}
	if p.ResponsibilityIdentity.UpdatedUnixNano > maxUpdated {
		maxUpdated = p.ResponsibilityIdentity.UpdatedUnixNano
	}
	if p.Responsibility.ExpiresUnixNano < minExpiry {
		minExpiry = p.Responsibility.ExpiresUnixNano
	}
	if p.Responsibility.UpdatedUnixNano > maxUpdated {
		maxUpdated = p.Responsibility.UpdatedUnixNano
	}
	if p.CheckedUnixNano < maxUpdated || p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= minExpiry || p.ExpiresUnixNano != minExpiry {
		return conflict("projection current window is not the exact closure min/max")
	}
	expectedID, err := DeriveReviewEligibilityProjectionIDV1(p.Source, p.Ref.Identity, p.Ref.Roles, p.Ref.Delegation, p.Ref.Responsibility)
	if err != nil || expectedID != p.Ref.ID {
		return conflict("projection stable id drifted")
	}
	expected, err := digest("ReviewEligibilityCurrentProjectionV1", p.digestValue())
	if err != nil {
		return err
	}
	if expected != p.ProjectionDigest || p.Ref.Digest != p.ProjectionDigest {
		return conflict("projection digest drifted")
	}
	return nil
}

func DeriveReviewEligibilityProjectionIDV1(source ReviewEligibilitySourceV1, identity IdentityRefV1, roles []RoleGrantRefV1, delegation *DelegationRefV1, responsibility ResponsibilityRefV1) (string, error) {
	if err := source.Validate(); err != nil {
		return "", err
	}
	body := struct {
		Source         ReviewEligibilitySourceV1 `json:"source"`
		Identity       IdentityRefV1             `json:"identity"`
		Roles          []RoleGrantRefV1          `json:"roles"`
		Delegation     *DelegationRefV1          `json:"delegation,omitempty"`
		Responsibility ResponsibilityRefV1       `json:"responsibility"`
	}{source.Clone(), identity, append([]RoleGrantRefV1(nil), roles...), delegation, responsibility}
	d, err := digest("ReviewEligibilityProjectionIdentityV1", body)
	if err != nil {
		return "", err
	}
	return "org-review-current:" + string(d), nil
}

func (p ReviewEligibilityCurrentProjectionV1) ValidateCurrent(expected ReviewEligibilityProjectionRefV1, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if !equalProjectionRef(p.Ref, expected) {
		return conflict("eligibility exact current ref drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "eligibility current clock regressed")
	}
	if now.UnixNano() >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "eligibility projection expired")
	}
	return nil
}

func equalProjectionRef(a, b ReviewEligibilityProjectionRefV1) bool {
	if a.TenantID != b.TenantID || a.ID != b.ID || !equalSource(a.Source, b.Source) || a.Identity != b.Identity || a.Responsibility != b.Responsibility || a.Digest != b.Digest || len(a.Roles) != len(b.Roles) {
		return false
	}
	for i := range a.Roles {
		if a.Roles[i] != b.Roles[i] {
			return false
		}
	}
	if (a.Delegation == nil) != (b.Delegation == nil) {
		return false
	}
	return a.Delegation == nil || *a.Delegation == *b.Delegation
}

func equalSource(a, b ReviewEligibilitySourceV1) bool {
	if a.TenantID != b.TenantID || a.ReviewerSubjectID != b.ReviewerSubjectID || a.ScopeDigest != b.ScopeDigest || a.ResponsibilitySubjectKind != b.ResponsibilitySubjectKind || a.ResponsibilitySubjectID != b.ResponsibilitySubjectID || a.ResponsibilitySubjectDigest != b.ResponsibilitySubjectDigest || a.DelegatorSubjectID != b.DelegatorSubjectID || a.DelegatedRole != b.DelegatedRole || a.RequireDelegation != b.RequireDelegation || a.Production != b.Production || len(a.RequiredRoles) != len(b.RequiredRoles) {
		return false
	}
	for i := range a.RequiredRoles {
		if a.RequiredRoles[i] != b.RequiredRoles[i] {
			return false
		}
	}
	return true
}
func refLess(a, b factRefV1) bool {
	if a.ID != b.ID {
		return a.ID < b.ID
	}
	if a.Revision != b.Revision {
		return a.Revision < b.Revision
	}
	return a.Digest < b.Digest
}
func contains(v []string, wanted string) bool {
	i := sort.SearchStrings(v, wanted)
	return i < len(v) && v[i] == wanted
}
