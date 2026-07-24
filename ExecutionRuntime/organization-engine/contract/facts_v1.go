package contract

import (
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const ContractVersionV1 = "praxis.organization.review-current/v1"

const MaxRolesV1 = 64

type FactStateV1 string

const (
	FactActiveV1     FactStateV1 = "active"
	FactRevokedV1    FactStateV1 = "revoked"
	FactSupersededV1 FactStateV1 = "superseded"
)

type SubjectKindV1 string

const (
	SubjectHumanV1   SubjectKindV1 = "human"
	SubjectAgentV1   SubjectKindV1 = "agent"
	SubjectServiceV1 SubjectKindV1 = "service"
)

type FactMetaV1 struct {
	ContractVersion string        `json:"contract_version"`
	TenantID        core.TenantID `json:"tenant_id"`
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	CreatedUnixNano int64         `json:"created_unix_nano"`
	UpdatedUnixNano int64         `json:"updated_unix_nano"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
	State           FactStateV1   `json:"state"`
}

type factRefV1 struct {
	TenantID core.TenantID `json:"tenant_id"`
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

type IdentityRefV1 factRefV1
type RoleGrantRefV1 factRefV1
type DelegationRefV1 factRefV1
type ResponsibilityRefV1 factRefV1

func (r IdentityRefV1) Validate() error       { return validateRef(factRefV1(r)) }
func (r RoleGrantRefV1) Validate() error      { return validateRef(factRefV1(r)) }
func (r DelegationRefV1) Validate() error     { return validateRef(factRefV1(r)) }
func (r ResponsibilityRefV1) Validate() error { return validateRef(factRefV1(r)) }

type IdentityFactV1 struct {
	FactMetaV1
	SubjectKind   SubjectKindV1 `json:"subject_kind"`
	SubjectID     string        `json:"subject_id"`
	DisplayHandle string        `json:"display_handle"`
}

type RoleGrantFactV1 struct {
	FactMetaV1
	Identity    IdentityRefV1 `json:"identity"`
	Role        string        `json:"role"`
	ScopeDigest core.Digest   `json:"scope_digest"`
	CanVeto     bool          `json:"can_veto"`
}

type DelegationFactV1 struct {
	FactMetaV1
	Delegator          IdentityRefV1 `json:"delegator"`
	Delegate           IdentityRefV1 `json:"delegate"`
	DelegatorSubjectID string        `json:"delegator_subject_id"`
	DelegateSubjectID  string        `json:"delegate_subject_id"`
	Role               string        `json:"role"`
	ScopeDigest        core.Digest   `json:"scope_digest"`
}

type ResponsibilityFactV1 struct {
	FactMetaV1
	SubjectKind   string        `json:"subject_kind"`
	SubjectID     string        `json:"subject_id"`
	SubjectDigest core.Digest   `json:"subject_digest"`
	Identity      IdentityRefV1 `json:"identity"`
}

func DeriveIdentityIDV1(tenant core.TenantID, subjectID string) (string, error) {
	return deriveID("identity", tenant, struct {
		TenantID  core.TenantID `json:"tenant_id"`
		SubjectID string        `json:"subject_id"`
	}{tenant, subjectID})
}

func DeriveRoleGrantIDV1(tenant core.TenantID, identityID, role string, scope core.Digest) (string, error) {
	return deriveID("role", tenant, struct {
		TenantID    core.TenantID `json:"tenant_id"`
		IdentityID  string        `json:"identity_id"`
		Role        string        `json:"role"`
		ScopeDigest core.Digest   `json:"scope_digest"`
	}{tenant, identityID, role, scope})
}

func DeriveDelegationIDV1(tenant core.TenantID, delegator, delegate, role string, scope core.Digest) (string, error) {
	return deriveID("delegation", tenant, struct {
		TenantID    core.TenantID `json:"tenant_id"`
		Delegator   string        `json:"delegator_subject_id"`
		Delegate    string        `json:"delegate_subject_id"`
		Role        string        `json:"role"`
		ScopeDigest core.Digest   `json:"scope_digest"`
	}{tenant, delegator, delegate, role, scope})
}

func DeriveResponsibilityIDV1(tenant core.TenantID, kind, subjectID string) (string, error) {
	return deriveID("responsibility", tenant, struct {
		TenantID    core.TenantID `json:"tenant_id"`
		SubjectKind string        `json:"subject_kind"`
		SubjectID   string        `json:"subject_id"`
	}{tenant, kind, subjectID})
}

func SealIdentityV1(v IdentityFactV1) (IdentityFactV1, error) {
	v.ContractVersion, v.Digest = ContractVersionV1, ""
	id, err := DeriveIdentityIDV1(v.TenantID, v.SubjectID)
	if err != nil {
		return IdentityFactV1{}, err
	}
	if v.ID == "" {
		v.ID = id
	} else if v.ID != id {
		return IdentityFactV1{}, conflict("identity stable id drifted")
	}
	if err := v.validateShape(); err != nil {
		return IdentityFactV1{}, err
	}
	v.Digest, err = digest("IdentityFactV1", v)
	if err != nil {
		return IdentityFactV1{}, err
	}
	return v, v.Validate()
}

func SealRoleGrantV1(v RoleGrantFactV1) (RoleGrantFactV1, error) {
	v.ContractVersion, v.Digest = ContractVersionV1, ""
	id, err := DeriveRoleGrantIDV1(v.TenantID, v.Identity.ID, v.Role, v.ScopeDigest)
	if err != nil {
		return RoleGrantFactV1{}, err
	}
	if v.ID == "" {
		v.ID = id
	} else if v.ID != id {
		return RoleGrantFactV1{}, conflict("role stable id drifted")
	}
	if err := v.validateShape(); err != nil {
		return RoleGrantFactV1{}, err
	}
	v.Digest, err = digest("RoleGrantFactV1", v)
	if err != nil {
		return RoleGrantFactV1{}, err
	}
	return v, v.Validate()
}

func SealDelegationV1(v DelegationFactV1) (DelegationFactV1, error) {
	v.ContractVersion, v.Digest = ContractVersionV1, ""
	id, err := DeriveDelegationIDV1(v.TenantID, v.DelegatorSubjectID, v.DelegateSubjectID, v.Role, v.ScopeDigest)
	if err != nil {
		return DelegationFactV1{}, err
	}
	if v.ID == "" {
		v.ID = id
	} else if v.ID != id {
		return DelegationFactV1{}, conflict("delegation stable id drifted")
	}
	if err := v.validateShape(); err != nil {
		return DelegationFactV1{}, err
	}
	v.Digest, err = digest("DelegationFactV1", v)
	if err != nil {
		return DelegationFactV1{}, err
	}
	return v, v.Validate()
}

func SealResponsibilityV1(v ResponsibilityFactV1) (ResponsibilityFactV1, error) {
	v.ContractVersion, v.Digest = ContractVersionV1, ""
	id, err := DeriveResponsibilityIDV1(v.TenantID, v.SubjectKind, v.SubjectID)
	if err != nil {
		return ResponsibilityFactV1{}, err
	}
	if v.ID == "" {
		v.ID = id
	} else if v.ID != id {
		return ResponsibilityFactV1{}, conflict("responsibility stable id drifted")
	}
	if err := v.validateShape(); err != nil {
		return ResponsibilityFactV1{}, err
	}
	v.Digest, err = digest("ResponsibilityFactV1", v)
	if err != nil {
		return ResponsibilityFactV1{}, err
	}
	return v, v.Validate()
}

func (v IdentityFactV1) ExactRef() IdentityRefV1 {
	return IdentityRefV1{v.TenantID, v.ID, v.Revision, v.Digest}
}
func (v RoleGrantFactV1) ExactRef() RoleGrantRefV1 {
	return RoleGrantRefV1{v.TenantID, v.ID, v.Revision, v.Digest}
}
func (v DelegationFactV1) ExactRef() DelegationRefV1 {
	return DelegationRefV1{v.TenantID, v.ID, v.Revision, v.Digest}
}
func (v ResponsibilityFactV1) ExactRef() ResponsibilityRefV1 {
	return ResponsibilityRefV1{v.TenantID, v.ID, v.Revision, v.Digest}
}

func (v IdentityFactV1) Validate() error {
	if err := v.validateShape(); err != nil {
		return err
	}
	actual := v.Digest
	v.Digest = ""
	return validateDigest("IdentityFactV1", actual, v)
}
func (v RoleGrantFactV1) Validate() error {
	if err := v.validateShape(); err != nil {
		return err
	}
	actual := v.Digest
	v.Digest = ""
	return validateDigest("RoleGrantFactV1", actual, v)
}
func (v DelegationFactV1) Validate() error {
	if err := v.validateShape(); err != nil {
		return err
	}
	actual := v.Digest
	v.Digest = ""
	return validateDigest("DelegationFactV1", actual, v)
}
func (v ResponsibilityFactV1) Validate() error {
	if err := v.validateShape(); err != nil {
		return err
	}
	actual := v.Digest
	v.Digest = ""
	return validateDigest("ResponsibilityFactV1", actual, v)
}

func (v IdentityFactV1) ValidateCurrent(expected IdentityRefV1, now time.Time) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if v.ExactRef() != expected {
		return conflict("identity current ref drifted")
	}
	return validateCurrent(v.FactMetaV1, now)
}
func (v RoleGrantFactV1) ValidateCurrent(expected RoleGrantRefV1, now time.Time) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if v.ExactRef() != expected {
		return conflict("role current ref drifted")
	}
	return validateCurrent(v.FactMetaV1, now)
}
func (v DelegationFactV1) ValidateCurrent(expected DelegationRefV1, now time.Time) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if v.ExactRef() != expected {
		return conflict("delegation current ref drifted")
	}
	return validateCurrent(v.FactMetaV1, now)
}
func (v ResponsibilityFactV1) ValidateCurrent(expected ResponsibilityRefV1, now time.Time) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if v.ExactRef() != expected {
		return conflict("responsibility current ref drifted")
	}
	return validateCurrent(v.FactMetaV1, now)
}

func (v IdentityFactV1) validateShape() error {
	if err := validateMeta(v.FactMetaV1); err != nil {
		return err
	}
	if v.SubjectKind != SubjectHumanV1 && v.SubjectKind != SubjectAgentV1 && v.SubjectKind != SubjectServiceV1 {
		return invalid("identity subject kind is unsupported")
	}
	if invalidText(v.SubjectID) || invalidText(v.DisplayHandle) {
		return invalid("identity subject is incomplete")
	}
	id, err := DeriveIdentityIDV1(v.TenantID, v.SubjectID)
	if err != nil || id != v.ID {
		return conflict("identity stable id mismatch")
	}
	return nil
}
func (v RoleGrantFactV1) validateShape() error {
	if err := validateMeta(v.FactMetaV1); err != nil {
		return err
	}
	if err := v.Identity.Validate(); err != nil {
		return err
	}
	if v.Identity.TenantID != v.TenantID || invalidText(v.Role) {
		return conflict("role identity or role drifted")
	}
	if err := v.ScopeDigest.Validate(); err != nil {
		return err
	}
	id, err := DeriveRoleGrantIDV1(v.TenantID, v.Identity.ID, v.Role, v.ScopeDigest)
	if err != nil || id != v.ID {
		return conflict("role stable id mismatch")
	}
	return nil
}
func (v DelegationFactV1) validateShape() error {
	if err := validateMeta(v.FactMetaV1); err != nil {
		return err
	}
	if err := v.Delegator.Validate(); err != nil {
		return err
	}
	if err := v.Delegate.Validate(); err != nil {
		return err
	}
	if v.Delegator.TenantID != v.TenantID || v.Delegate.TenantID != v.TenantID || v.Delegator == v.Delegate || invalidText(v.DelegatorSubjectID) || invalidText(v.DelegateSubjectID) || invalidText(v.Role) {
		return conflict("delegation participants are invalid")
	}
	if err := v.ScopeDigest.Validate(); err != nil {
		return err
	}
	id, err := DeriveDelegationIDV1(v.TenantID, v.DelegatorSubjectID, v.DelegateSubjectID, v.Role, v.ScopeDigest)
	if err != nil || id != v.ID {
		return conflict("delegation stable id mismatch")
	}
	return nil
}
func (v ResponsibilityFactV1) validateShape() error {
	if err := validateMeta(v.FactMetaV1); err != nil {
		return err
	}
	if invalidText(v.SubjectKind) || invalidText(v.SubjectID) {
		return invalid("responsibility subject is incomplete")
	}
	if err := v.SubjectDigest.Validate(); err != nil {
		return err
	}
	if err := v.Identity.Validate(); err != nil {
		return err
	}
	if v.Identity.TenantID != v.TenantID {
		return conflict("responsibility identity crosses tenant")
	}
	id, err := DeriveResponsibilityIDV1(v.TenantID, v.SubjectKind, v.SubjectID)
	if err != nil || id != v.ID {
		return conflict("responsibility stable id mismatch")
	}
	return nil
}

func validateMeta(v FactMetaV1) error {
	if v.ContractVersion != ContractVersionV1 || strings.TrimSpace(string(v.TenantID)) == "" || invalidText(v.ID) || v.Revision == 0 || v.CreatedUnixNano <= 0 || v.UpdatedUnixNano < v.CreatedUnixNano || v.ExpiresUnixNano <= v.UpdatedUnixNano {
		return invalid("fact metadata is incomplete")
	}
	if v.State != FactActiveV1 && v.State != FactRevokedV1 && v.State != FactSupersededV1 {
		return invalid("fact state is unsupported")
	}
	return nil
}
func validateCurrent(v FactMetaV1, now time.Time) error {
	if now.IsZero() || now.UnixNano() <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "current clock is invalid")
	}
	if v.State != FactActiveV1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "fact is terminal")
	}
	if now.UnixNano() < v.UpdatedUnixNano || now.UnixNano() >= v.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "fact is not current")
	}
	return nil
}
func validateRef(v factRefV1) error {
	if strings.TrimSpace(string(v.TenantID)) == "" || invalidText(v.ID) || v.Revision == 0 {
		return invalid("exact ref is incomplete")
	}
	return v.Digest.Validate()
}
func deriveID(kind string, tenant core.TenantID, body any) (string, error) {
	if strings.TrimSpace(string(tenant)) == "" {
		return "", invalid("tenant is required")
	}
	d, err := core.CanonicalJSONDigest("praxis.organization.review-current", ContractVersionV1, kind+"IdentityV1", body)
	if err != nil {
		return "", err
	}
	return "org-" + kind + ":" + string(d), nil
}
func digest(kind string, body any) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.organization.review-current", ContractVersionV1, kind, body)
}
func validateDigest(kind string, actual core.Digest, body any) error {
	if err := actual.Validate(); err != nil {
		return err
	}
	expected, err := digest(kind, body)
	if err != nil {
		return err
	}
	if expected != actual {
		return conflict("fact digest drifted")
	}
	return nil
}
func invalid(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, message)
}
func conflict(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, message)
}
func invalidText(v string) bool { return strings.TrimSpace(v) == "" || len(v) > 512 }

func CanonicalRolesV1(v []string) ([]string, error) {
	out := append([]string(nil), v...)
	sort.Strings(out)
	if len(out) == 0 || len(out) > MaxRolesV1 {
		return nil, invalid("role set is empty or unbounded")
	}
	for i, role := range out {
		if invalidText(role) || (i > 0 && out[i-1] == role) {
			return nil, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "role set contains duplicate")
		}
	}
	return out, nil
}
