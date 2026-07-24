package contract

import (
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const HumanMultiSignContractV2 = "praxis.review.human-multisig/v2"

// The following named refs are intentionally not interchangeable. They bind
// exact Review-owned facts; none grants currentness or authority by itself.
type humanFactExactRefV2 struct {
	TenantID core.TenantID `json:"tenant_id"`
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

type HumanCaseExactRefV2 humanFactExactRefV2
type HumanTargetExactRefV2 humanFactExactRefV2
type HumanRoundExactRefV2 humanFactExactRefV2
type HumanPanelExactRefV2 humanFactExactRefV2
type HumanPanelAssignmentExactRefV2 humanFactExactRefV2
type HumanAttestationExactRefV2 humanFactExactRefV2
type HumanQuorumDecisionExactRefV2 humanFactExactRefV2
type HumanFindingExactRefV2 humanFactExactRefV2
type HumanVerdictExactRefV2 humanFactExactRefV2

func validateHumanFactRefV2(ref humanFactExactRefV2) error {
	if blank(string(ref.TenantID)) || invalidID(ref.ID) || ref.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human multisign exact fact ref is incomplete")
	}
	return ref.Digest.Validate()
}

func (r HumanCaseExactRefV2) Validate() error { return validateHumanFactRefV2(humanFactExactRefV2(r)) }
func (r HumanTargetExactRefV2) Validate() error {
	return validateHumanFactRefV2(humanFactExactRefV2(r))
}
func (r HumanRoundExactRefV2) Validate() error { return validateHumanFactRefV2(humanFactExactRefV2(r)) }
func (r HumanPanelExactRefV2) Validate() error { return validateHumanFactRefV2(humanFactExactRefV2(r)) }
func (r HumanPanelAssignmentExactRefV2) Validate() error {
	return validateHumanFactRefV2(humanFactExactRefV2(r))
}
func (r HumanAttestationExactRefV2) Validate() error {
	return validateHumanFactRefV2(humanFactExactRefV2(r))
}
func (r HumanQuorumDecisionExactRefV2) Validate() error {
	return validateHumanFactRefV2(humanFactExactRefV2(r))
}
func (r HumanFindingExactRefV2) Validate() error {
	return validateHumanFactRefV2(humanFactExactRefV2(r))
}
func (r HumanVerdictExactRefV2) Validate() error {
	return validateHumanFactRefV2(humanFactExactRefV2(r))
}

func validateHumanFactIdentityV2(f FactIdentityV1) error {
	if f.ContractVersion != HumanMultiSignContractV2 || blank(string(f.TenantID)) || invalidID(f.ID) || f.Revision == 0 || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human multisign fact identity is incomplete")
	}
	return nil
}

// HumanQuorumPolicyBindingV2 is a nominal carrier for an external Policy
// Owner exact fact. Validate does not assert currentness or authority.
type HumanQuorumPolicyBindingV2 struct {
	TenantID        core.TenantID `json:"tenant_id"`
	Ref             string        `json:"ref"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	Domain          string        `json:"domain"`
	CheckedUnixNano int64         `json:"checked_unix_nano"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r HumanQuorumPolicyBindingV2) Validate() error {
	if blank(string(r.TenantID)) || invalidID(r.Ref) || r.Revision == 0 || invalidText(r.Domain) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human quorum policy exact ref is incomplete")
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	if r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictStale, "human quorum policy projection window is invalid")
	}
	return nil
}

// HumanIdentityProofRefV2, HumanDelegationFactRefV2 and
// HumanResponsibilitySubjectRefV2 are nominal exact-ref carriers for their
// external Owners. They are neither current projections nor Authority facts.
type HumanIdentityProofRefV2 struct {
	TenantID core.TenantID `json:"tenant_id"`
	Ref      string        `json:"ref"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r HumanIdentityProofRefV2) Validate() error {
	if blank(string(r.TenantID)) || invalidID(r.Ref) || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human identity proof exact ref is incomplete")
	}
	return r.Digest.Validate()
}

type HumanDelegationFactRefV2 struct {
	TenantID core.TenantID `json:"tenant_id"`
	Ref      string        `json:"ref"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r HumanDelegationFactRefV2) Validate() error {
	if blank(string(r.TenantID)) || invalidID(r.Ref) || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human delegation exact ref is incomplete")
	}
	return r.Digest.Validate()
}

type HumanResponsibilitySubjectRefV2 struct {
	TenantID      core.TenantID           `json:"tenant_id"`
	Ref           string                  `json:"ref"`
	Revision      core.Revision           `json:"revision"`
	Digest        core.Digest             `json:"digest"`
	IdentityProof HumanIdentityProofRefV2 `json:"identity_proof"`
}

func (r HumanResponsibilitySubjectRefV2) Validate() error {
	if blank(string(r.TenantID)) || invalidID(r.Ref) || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human responsibility subject exact ref is incomplete")
	}
	if err := r.Digest.Validate(); err != nil {
		return err
	}
	if r.IdentityProof.TenantID != r.TenantID {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "responsibility identity tenant drifted")
	}
	return r.IdentityProof.Validate()
}

type HumanRoleRequirementV2 struct {
	Role    string `json:"role"`
	Minimum uint32 `json:"minimum"`
}

func validateRoleRequirementsV2(v []HumanRoleRequirementV2, max uint32) error {
	if len(v) == 0 || len(v) > MaxListItemsV1 || !sort.SliceIsSorted(v, func(i, j int) bool { return v[i].Role < v[j].Role }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "human role requirements must be bounded, non-empty and sorted")
	}
	var total uint64
	for i, x := range v {
		if invalidText(x.Role) || x.Minimum == 0 || (i > 0 && v[i-1].Role == x.Role) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "human role requirement is invalid or duplicated")
		}
		total += uint64(x.Minimum)
	}
	if total > uint64(max) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "required role minima exceed maximum panel size")
	}
	return nil
}

type HumanPanelStateV2 string

const (
	HumanPanelProposedV2               HumanPanelStateV2 = "proposed"
	HumanPanelOpenV2                   HumanPanelStateV2 = "open"
	HumanPanelQuorumSatisfiedV2        HumanPanelStateV2 = "quorum_satisfied"
	HumanPanelDecidingV2               HumanPanelStateV2 = "deciding"
	HumanPanelDecidedV2                HumanPanelStateV2 = "decided"
	HumanPanelVetoedV2                 HumanPanelStateV2 = "vetoed"
	HumanPanelWaitingRevisionV2        HumanPanelStateV2 = "waiting_revision"
	HumanPanelWaitingEvidenceV2        HumanPanelStateV2 = "waiting_evidence"
	HumanPanelWaitingHigherAuthorityV2 HumanPanelStateV2 = "waiting_higher_authority"
	HumanPanelRevokedV2                HumanPanelStateV2 = "revoked"
	HumanPanelExpiredV2                HumanPanelStateV2 = "expired"
	HumanPanelSupersededV2             HumanPanelStateV2 = "superseded"
	HumanPanelIndeterminateV2          HumanPanelStateV2 = "indeterminate"
)

func validateHumanPanelStateV2(s HumanPanelStateV2) error {
	switch s {
	case HumanPanelProposedV2, HumanPanelOpenV2, HumanPanelQuorumSatisfiedV2, HumanPanelDecidingV2, HumanPanelDecidedV2, HumanPanelVetoedV2, HumanPanelWaitingRevisionV2, HumanPanelWaitingEvidenceV2, HumanPanelWaitingHigherAuthorityV2, HumanPanelRevokedV2, HumanPanelExpiredV2, HumanPanelSupersededV2, HumanPanelIndeterminateV2:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "human panel state is unsupported")
	}
}

func activeHumanPanelStateV2(s HumanPanelStateV2) bool {
	switch s {
	case HumanPanelProposedV2, HumanPanelOpenV2, HumanPanelQuorumSatisfiedV2, HumanPanelDecidingV2, HumanPanelWaitingRevisionV2, HumanPanelWaitingEvidenceV2, HumanPanelWaitingHigherAuthorityV2, HumanPanelIndeterminateV2:
		return true
	default:
		return false
	}
}

func CanTransitionHumanPanelV2(from, to HumanPanelStateV2) bool {
	if from == to || !activeHumanPanelStateV2(from) {
		return false
	}
	if to == HumanPanelRevokedV2 || to == HumanPanelExpiredV2 || to == HumanPanelSupersededV2 || to == HumanPanelIndeterminateV2 {
		return true
	}
	allowed := map[HumanPanelStateV2]map[HumanPanelStateV2]bool{
		HumanPanelProposedV2:               {HumanPanelOpenV2: true},
		HumanPanelOpenV2:                   {HumanPanelQuorumSatisfiedV2: true, HumanPanelVetoedV2: true, HumanPanelWaitingRevisionV2: true, HumanPanelWaitingEvidenceV2: true, HumanPanelWaitingHigherAuthorityV2: true},
		HumanPanelQuorumSatisfiedV2:        {HumanPanelDecidingV2: true},
		HumanPanelDecidingV2:               {HumanPanelDecidedV2: true, HumanPanelVetoedV2: true, HumanPanelWaitingRevisionV2: true, HumanPanelWaitingEvidenceV2: true, HumanPanelWaitingHigherAuthorityV2: true},
		HumanPanelWaitingRevisionV2:        {HumanPanelProposedV2: true},
		HumanPanelWaitingEvidenceV2:        {HumanPanelOpenV2: true},
		HumanPanelWaitingHigherAuthorityV2: {HumanPanelProposedV2: true},
		HumanPanelIndeterminateV2:          {HumanPanelOpenV2: true, HumanPanelDecidingV2: true},
	}
	return allowed[from][to]
}

type HumanReviewPanelV2 struct {
	FactIdentityV1
	Case                        HumanCaseExactRefV2              `json:"case"`
	Target                      HumanTargetExactRefV2            `json:"target"`
	Round                       HumanRoundExactRefV2             `json:"round"`
	QuorumPolicy                HumanQuorumPolicyBindingV2       `json:"quorum_policy"`
	ResponsibilitySubject       HumanResponsibilitySubjectRefV2  `json:"responsibility_subject"`
	State                       HumanPanelStateV2                `json:"state"`
	AssignmentRefs              []HumanPanelAssignmentExactRefV2 `json:"assignment_refs"`
	AcceptThreshold             uint32                           `json:"accept_threshold"`
	MaximumPanelSize            uint32                           `json:"maximum_panel_size"`
	RoleRequirements            []HumanRoleRequirementV2         `json:"role_requirements"`
	RejectVetoRoles             []string                         `json:"reject_veto_roles"`
	DelegationRequired          bool                             `json:"delegation_required"`
	ProductionSelfReviewAllowed bool                             `json:"production_self_review_allowed"`
	MaxPanelDurationNanos       int64                            `json:"max_panel_duration_nanos"`
	MaxVoteTTLNanos             int64                            `json:"max_vote_ttl_nanos"`
	ExpiresUnixNano             int64                            `json:"expires_unix_nano"`
}

func (p HumanReviewPanelV2) Clone() HumanReviewPanelV2 {
	p.AssignmentRefs = append([]HumanPanelAssignmentExactRefV2(nil), p.AssignmentRefs...)
	p.RoleRequirements = append([]HumanRoleRequirementV2(nil), p.RoleRequirements...)
	p.RejectVetoRoles = append([]string(nil), p.RejectVetoRoles...)
	return p
}

func (p HumanReviewPanelV2) digestValue() HumanReviewPanelV2 { p.Digest = ""; return p.Clone() }

func (p HumanReviewPanelV2) validateShape() error {
	if err := validateHumanFactIdentityV2(p.FactIdentityV1); err != nil {
		return err
	}
	for _, err := range []error{p.Case.Validate(), p.Target.Validate(), p.Round.Validate(), p.QuorumPolicy.Validate(), p.ResponsibilitySubject.Validate(), validateHumanPanelStateV2(p.State)} {
		if err != nil {
			return err
		}
	}
	if p.Case.TenantID != p.TenantID || p.Target.TenantID != p.TenantID || p.Round.TenantID != p.TenantID || p.QuorumPolicy.TenantID != p.TenantID || p.ResponsibilitySubject.TenantID != p.TenantID {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human panel cross-tenant exact ref")
	}
	expectedID, err := DeriveHumanPanelIDV2(p.TenantID, p.Case, p.Round, p.QuorumPolicy)
	if err != nil {
		return err
	}
	if p.ID != expectedID {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human panel id does not bind its exact identity")
	}
	if p.AcceptThreshold == 0 || p.MaximumPanelSize < p.AcceptThreshold || p.MaximumPanelSize > MaxListItemsV1 || len(p.AssignmentRefs) > int(p.MaximumPanelSize) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "human panel K-of-N bounds are invalid")
	}
	if p.State == HumanPanelProposedV2 {
		if len(p.AssignmentRefs) != 0 {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "proposed panel cannot claim unpublished assignments")
		}
	} else if len(p.AssignmentRefs) != int(p.MaximumPanelSize) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "non-proposed panel requires its complete N-assignment exact set")
	}
	if len(p.AssignmentRefs) > 0 {
		if !sort.SliceIsSorted(p.AssignmentRefs, func(i, j int) bool {
			return humanFactRefLessV2(humanFactExactRefV2(p.AssignmentRefs[i]), humanFactExactRefV2(p.AssignmentRefs[j]))
		}) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "panel assignment refs must be sorted")
		}
		for i, ref := range p.AssignmentRefs {
			if err := ref.Validate(); err != nil {
				return err
			}
			if ref.TenantID != p.TenantID || (i > 0 && humanFactExactRefV2(p.AssignmentRefs[i-1]) == humanFactExactRefV2(ref)) {
				return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "panel assignment ref is duplicated or cross-tenant")
			}
		}
	}
	if err := validateRoleRequirementsV2(p.RoleRequirements, p.MaximumPanelSize); err != nil {
		return err
	}
	if err := validateSortedUniqueStringsV2(p.RejectVetoRoles, false); err != nil {
		return err
	}
	if !p.DelegationRequired || p.ProductionSelfReviewAllowed || p.MaxPanelDurationNanos <= 0 || p.MaxVoteTTLNanos <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human production panel policy snapshot is unsafe")
	}
	if err := ValidateExpires(p.CreatedUnixNano, p.ExpiresUnixNano); err != nil {
		return err
	}
	if p.CreatedUnixNano < p.QuorumPolicy.CheckedUnixNano || p.ExpiresUnixNano > p.QuorumPolicy.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "panel exceeds exact quorum policy current window")
	}
	if p.ExpiresUnixNano-p.CreatedUnixNano > p.MaxPanelDurationNanos {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "panel expiry exceeds policy duration")
	}
	return nil
}

func SealHumanReviewPanelV2(p HumanReviewPanelV2) (HumanReviewPanelV2, error) {
	p = p.Clone()
	p.ContractVersion = HumanMultiSignContractV2
	p.Digest = ""
	sort.Slice(p.AssignmentRefs, func(i, j int) bool {
		return humanFactRefLessV2(humanFactExactRefV2(p.AssignmentRefs[i]), humanFactExactRefV2(p.AssignmentRefs[j]))
	})
	sort.Slice(p.RoleRequirements, func(i, j int) bool { return p.RoleRequirements[i].Role < p.RoleRequirements[j].Role })
	sort.Strings(p.RejectVetoRoles)
	expectedID, err := DeriveHumanPanelIDV2(p.TenantID, p.Case, p.Round, p.QuorumPolicy)
	if err != nil {
		return HumanReviewPanelV2{}, err
	}
	if p.ID == "" {
		p.ID = expectedID
	} else if p.ID != expectedID {
		return HumanReviewPanelV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human panel id does not bind its exact identity")
	}
	if err := p.validateShape(); err != nil {
		return HumanReviewPanelV2{}, err
	}
	d, err := sealHumanV2("HumanReviewPanelV2", p.digestValue())
	if err != nil {
		return HumanReviewPanelV2{}, err
	}
	p.Digest = d
	return p, p.Validate()
}

func DeriveHumanPanelIDV2(tenant core.TenantID, caseRef HumanCaseExactRefV2, roundRef HumanRoundExactRefV2, policy HumanQuorumPolicyBindingV2) (string, error) {
	if blank(string(tenant)) || caseRef.TenantID != tenant || roundRef.TenantID != tenant || policy.TenantID != tenant {
		return "", core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human panel identity inputs cross tenant")
	}
	for _, err := range []error{caseRef.Validate(), roundRef.Validate(), policy.Validate()} {
		if err != nil {
			return "", err
		}
	}
	d, err := sealHumanV2("HumanReviewPanelIdentityV2", struct {
		Tenant core.TenantID              `json:"tenant_id"`
		Case   HumanCaseExactRefV2        `json:"case"`
		Round  HumanRoundExactRefV2       `json:"round"`
		Policy HumanQuorumPolicyBindingV2 `json:"policy"`
	}{tenant, caseRef, roundRef, policy})
	if err != nil {
		return "", err
	}
	return "human-panel-v2:" + string(d), nil
}

func (p HumanReviewPanelV2) Validate() error {
	if err := p.validateShape(); err != nil {
		return err
	}
	return validateSealedHumanV2("HumanReviewPanelV2", p.digestValue(), p.Digest)
}

func (p HumanReviewPanelV2) ExactRef() HumanPanelExactRefV2 {
	return HumanPanelExactRefV2{TenantID: p.TenantID, ID: p.ID, Revision: p.Revision, Digest: p.Digest}
}
func (p HumanReviewPanelV2) ValidateCurrent(expected HumanPanelExactRefV2, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.ExactRef() != expected {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human panel exact current ref drifted")
	}
	if !activeHumanPanelStateV2(p.State) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human panel is terminal")
	}
	return ValidateNow(now, p.CreatedUnixNano, p.ExpiresUnixNano)
}

type HumanAssignmentStateV2 string

const (
	HumanAssignmentOfferedV2    HumanAssignmentStateV2 = "offered"
	HumanAssignmentClaimedV2    HumanAssignmentStateV2 = "claimed"
	HumanAssignmentReleasedV2   HumanAssignmentStateV2 = "released"
	HumanAssignmentRevokedV2    HumanAssignmentStateV2 = "revoked"
	HumanAssignmentExpiredV2    HumanAssignmentStateV2 = "expired"
	HumanAssignmentSupersededV2 HumanAssignmentStateV2 = "superseded"
)

type HumanPanelAssignmentV2 struct {
	FactIdentityV1
	Panel                 HumanPanelExactRefV2                     `json:"panel"`
	Case                  HumanCaseExactRefV2                      `json:"case"`
	Round                 HumanRoundExactRefV2                     `json:"round"`
	Target                HumanTargetExactRefV2                    `json:"target"`
	ReviewerIdentity      HumanIdentityProofRefV2                  `json:"reviewer_identity"`
	ReviewerAuthority     runtimeports.AuthorityBindingRefV2       `json:"reviewer_authority"`
	ReviewerBinding       runtimeports.ReviewComponentBindingRefV2 `json:"reviewer_binding"`
	Roles                 []string                                 `json:"roles"`
	CanVeto               bool                                     `json:"can_veto"`
	Delegated             bool                                     `json:"delegated"`
	DelegatorIdentity     HumanIdentityProofRefV2                  `json:"delegator_identity,omitempty"`
	DelegateIdentity      HumanIdentityProofRefV2                  `json:"delegate_identity,omitempty"`
	DelegationFact        HumanDelegationFactRefV2                 `json:"delegation_fact,omitempty"`
	DelegatedRole         string                                   `json:"delegated_role,omitempty"`
	DelegationScopeDigest core.Digest                              `json:"delegation_scope_digest,omitempty"`
	State                 HumanAssignmentStateV2                   `json:"state"`
	LeaseHolder           string                                   `json:"lease_holder,omitempty"`
	LeaseExpiresUnixNano  int64                                    `json:"lease_expires_unix_nano,omitempty"`
	ExpiresUnixNano       int64                                    `json:"expires_unix_nano"`
}

func (a HumanPanelAssignmentV2) Clone() HumanPanelAssignmentV2 {
	a.Roles = append([]string(nil), a.Roles...)
	return a
}
func (a HumanPanelAssignmentV2) digestValue() HumanPanelAssignmentV2 { a.Digest = ""; return a.Clone() }
func (a HumanPanelAssignmentV2) validateShape() error {
	if err := validateHumanFactIdentityV2(a.FactIdentityV1); err != nil {
		return err
	}
	for _, err := range []error{a.Panel.Validate(), a.Case.Validate(), a.Round.Validate(), a.Target.Validate(), a.ReviewerIdentity.Validate(), a.ReviewerAuthority.Validate(), a.ReviewerBinding.Validate()} {
		if err != nil {
			return err
		}
	}
	if a.Panel.TenantID != a.TenantID || a.Case.TenantID != a.TenantID || a.Round.TenantID != a.TenantID || a.Target.TenantID != a.TenantID || a.ReviewerIdentity.TenantID != a.TenantID {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "assignment exact refs cross tenant")
	}
	if err := validateSortedUniqueStringsV2(a.Roles, false); err != nil {
		return err
	}
	switch a.State {
	case HumanAssignmentOfferedV2, HumanAssignmentClaimedV2, HumanAssignmentReleasedV2, HumanAssignmentRevokedV2, HumanAssignmentExpiredV2, HumanAssignmentSupersededV2:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "human assignment state is unsupported")
	}
	if a.Delegated {
		for _, err := range []error{a.DelegatorIdentity.Validate(), a.DelegateIdentity.Validate(), a.DelegationFact.Validate(), a.DelegationScopeDigest.Validate()} {
			if err != nil {
				return err
			}
		}
		if a.DelegatorIdentity.TenantID != a.TenantID || a.DelegateIdentity.TenantID != a.TenantID || a.DelegationFact.TenantID != a.TenantID || a.DelegateIdentity != a.ReviewerIdentity || invalidText(a.DelegatedRole) {
			return core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "delegation exact binding drifted")
		}
		if !containsStringV2(a.Roles, a.DelegatedRole) {
			return core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "delegated role is not assigned")
		}
	} else if a.DelegatorIdentity != (HumanIdentityProofRefV2{}) || a.DelegateIdentity != (HumanIdentityProofRefV2{}) || a.DelegationFact != (HumanDelegationFactRefV2{}) || a.DelegatedRole != "" || a.DelegationScopeDigest != "" {
		return core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "direct assignment cannot claim delegation")
	}
	if a.State == HumanAssignmentClaimedV2 {
		if invalidID(a.LeaseHolder) || a.LeaseExpiresUnixNano <= a.UpdatedUnixNano || a.LeaseExpiresUnixNano > a.ExpiresUnixNano {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonStaleLeaseRevision, "claimed assignment lease is invalid")
		}
	} else if a.LeaseHolder != "" || a.LeaseExpiresUnixNano != 0 {
		return core.NewError(core.ErrorConflict, core.ReasonStaleLeaseRevision, "unclaimed assignment cannot retain lease")
	}
	return ValidateExpires(a.CreatedUnixNano, a.ExpiresUnixNano)
}
func SealHumanPanelAssignmentV2(a HumanPanelAssignmentV2) (HumanPanelAssignmentV2, error) {
	a = a.Clone()
	a.ContractVersion = HumanMultiSignContractV2
	a.Digest = ""
	sort.Strings(a.Roles)
	if err := a.validateShape(); err != nil {
		return HumanPanelAssignmentV2{}, err
	}
	d, err := sealHumanV2("HumanPanelAssignmentV2", a.digestValue())
	if err != nil {
		return HumanPanelAssignmentV2{}, err
	}
	a.Digest = d
	return a, a.Validate()
}
func (a HumanPanelAssignmentV2) Validate() error {
	if err := a.validateShape(); err != nil {
		return err
	}
	return validateSealedHumanV2("HumanPanelAssignmentV2", a.digestValue(), a.Digest)
}
func (a HumanPanelAssignmentV2) ExactRef() HumanPanelAssignmentExactRefV2 {
	return HumanPanelAssignmentExactRefV2{TenantID: a.TenantID, ID: a.ID, Revision: a.Revision, Digest: a.Digest}
}
func (a HumanPanelAssignmentV2) ValidateCurrent(expected HumanPanelAssignmentExactRefV2, now time.Time) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if a.ExactRef() != expected {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "assignment exact current ref drifted")
	}
	if a.State != HumanAssignmentOfferedV2 && a.State != HumanAssignmentClaimedV2 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "assignment is terminal")
	}
	if err := ValidateNow(now, a.CreatedUnixNano, a.ExpiresUnixNano); err != nil {
		return err
	}
	if a.State == HumanAssignmentClaimedV2 && now.UnixNano() >= a.LeaseExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleLeaseRevision, "assignment lease expired")
	}
	return nil
}

type HumanAttestationV2 struct {
	FactIdentityV1
	IdempotencyKey        string                                   `json:"idempotency_key"`
	Panel                 HumanPanelExactRefV2                     `json:"panel"`
	Assignment            HumanPanelAssignmentExactRefV2           `json:"assignment"`
	Case                  HumanCaseExactRefV2                      `json:"case"`
	Round                 HumanRoundExactRefV2                     `json:"round"`
	Target                HumanTargetExactRefV2                    `json:"target"`
	Policy                HumanQuorumPolicyBindingV2               `json:"policy"`
	ResponsibilitySubject HumanResponsibilitySubjectRefV2          `json:"responsibility_subject"`
	ReviewerIdentity      HumanIdentityProofRefV2                  `json:"reviewer_identity"`
	ReviewerAuthority     runtimeports.AuthorityBindingRefV2       `json:"reviewer_authority"`
	Delegation            *HumanDelegationFactRefV2                `json:"delegation,omitempty"`
	ReviewerBinding       runtimeports.ReviewComponentBindingRefV2 `json:"reviewer_binding"`
	Resolution            ResolutionV1                             `json:"resolution"`
	ReasonCodes           []string                                 `json:"reason_codes"`
	FindingRefs           []HumanFindingExactRefV2                 `json:"finding_refs"`
	Evidence              []runtimeports.ReviewEvidenceRefV2       `json:"evidence"`
	EvidenceDigest        core.Digest                              `json:"evidence_digest"`
	Conditions            []runtimeports.ReviewConditionV2         `json:"conditions"`
	ConditionsDigest      core.Digest                              `json:"conditions_digest,omitempty"`
	ObservedUnixNano      int64                                    `json:"observed_unix_nano"`
	ExpiresUnixNano       int64                                    `json:"expires_unix_nano"`
}

func (a HumanAttestationV2) Clone() HumanAttestationV2 {
	a.ReasonCodes = append([]string(nil), a.ReasonCodes...)
	a.FindingRefs = append([]HumanFindingExactRefV2(nil), a.FindingRefs...)
	a.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), a.Evidence...)
	a.Conditions = append([]runtimeports.ReviewConditionV2(nil), a.Conditions...)
	if a.Delegation != nil {
		x := *a.Delegation
		a.Delegation = &x
	}
	return a
}
func (a HumanAttestationV2) digestValue() HumanAttestationV2 { a.Digest = ""; return a.Clone() }
func (a HumanAttestationV2) validateShape() error {
	if err := validateHumanFactIdentityV2(a.FactIdentityV1); err != nil {
		return err
	}
	if a.Revision != 1 || invalidID(a.IdempotencyKey) || a.ObservedUnixNano < a.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human attestation identity/time is invalid")
	}
	for _, err := range []error{a.Panel.Validate(), a.Assignment.Validate(), a.Case.Validate(), a.Round.Validate(), a.Target.Validate(), a.Policy.Validate(), a.ResponsibilitySubject.Validate(), a.ReviewerIdentity.Validate(), a.ReviewerAuthority.Validate(), a.ReviewerBinding.Validate()} {
		if err != nil {
			return err
		}
	}
	if a.Panel.TenantID != a.TenantID || a.Assignment.TenantID != a.TenantID || a.Case.TenantID != a.TenantID || a.Round.TenantID != a.TenantID || a.Target.TenantID != a.TenantID || a.Policy.TenantID != a.TenantID || a.ResponsibilitySubject.TenantID != a.TenantID || a.ReviewerIdentity.TenantID != a.TenantID {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "attestation exact refs cross tenant")
	}
	if a.ResponsibilitySubject.IdentityProof.TenantID == a.ReviewerIdentity.TenantID && a.ResponsibilitySubject.IdentityProof.Ref == a.ReviewerIdentity.Ref {
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "production human self-review is forbidden")
	}
	if a.Delegation != nil {
		if err := a.Delegation.Validate(); err != nil {
			return err
		}
		if a.Delegation.TenantID != a.TenantID {
			return core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "attestation delegation crosses tenant")
		}
	}
	switch a.Resolution {
	case ResolutionAcceptV1, ResolutionConditionalV1, ResolutionRequestChangesV1, ResolutionEscalateHumanV1, ResolutionRejectV1, ResolutionInsufficientEvidenceV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "human attestation resolution is unsupported")
	}
	if err := validateSortedUniqueStringsV2(a.ReasonCodes, false); err != nil {
		return err
	}
	if len(a.FindingRefs) > MaxListItemsV1 || !sort.SliceIsSorted(a.FindingRefs, func(i, j int) bool {
		return humanFactRefLessV2(humanFactExactRefV2(a.FindingRefs[i]), humanFactExactRefV2(a.FindingRefs[j]))
	}) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "attestation finding refs must be bounded and sorted")
	}
	for i, r := range a.FindingRefs {
		if err := r.Validate(); err != nil {
			return err
		}
		if r.TenantID != a.TenantID || (i > 0 && humanFactExactRefV2(a.FindingRefs[i-1]) == humanFactExactRefV2(r)) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "attestation finding ref duplicated or cross-tenant")
		}
	}
	if err := validateEvidenceSetV2(a.Evidence, a.EvidenceDigest); err != nil {
		return err
	}
	if err := validateConditionsSetV2(a.Conditions, a.ConditionsDigest, a.Resolution == ResolutionConditionalV1); err != nil {
		return err
	}
	for _, condition := range a.Conditions {
		if condition.ExpiresUnixNano <= a.ObservedUnixNano || a.ExpiresUnixNano > condition.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "human attestation exceeds an exact condition TTL")
		}
	}
	return ValidateExpires(a.CreatedUnixNano, a.ExpiresUnixNano)
}
func SealHumanAttestationV2(a HumanAttestationV2) (HumanAttestationV2, error) {
	a = a.Clone()
	a.ContractVersion = HumanMultiSignContractV2
	a.Digest = ""
	sort.Strings(a.ReasonCodes)
	sort.Slice(a.FindingRefs, func(i, j int) bool {
		return humanFactRefLessV2(humanFactExactRefV2(a.FindingRefs[i]), humanFactExactRefV2(a.FindingRefs[j]))
	})
	sortEvidenceV2(a.Evidence)
	sortConditionsV2(a.Conditions)
	if err := a.validateShape(); err != nil {
		return HumanAttestationV2{}, err
	}
	d, err := sealHumanV2("HumanAttestationV2", a.digestValue())
	if err != nil {
		return HumanAttestationV2{}, err
	}
	a.Digest = d
	return a, a.Validate()
}
func (a HumanAttestationV2) Validate() error {
	if err := a.validateShape(); err != nil {
		return err
	}
	return validateSealedHumanV2("HumanAttestationV2", a.digestValue(), a.Digest)
}
func (a HumanAttestationV2) ExactRef() HumanAttestationExactRefV2 {
	return HumanAttestationExactRefV2{TenantID: a.TenantID, ID: a.ID, Revision: a.Revision, Digest: a.Digest}
}
func (a HumanAttestationV2) ValidateCurrent(expected HumanAttestationExactRefV2, now time.Time) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if a.ExactRef() != expected {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "attestation exact current ref drifted")
	}
	return ValidateNow(now, a.CreatedUnixNano, a.ExpiresUnixNano)
}

type HumanSatisfiedRoleCountV2 struct {
	Role                 string `json:"role"`
	DistinctCurrentCount uint32 `json:"distinct_current_count"`
}

type HumanQuorumDecisionV2 struct {
	FactIdentityV1
	Panel                        HumanPanelExactRefV2             `json:"panel"`
	Policy                       HumanQuorumPolicyBindingV2       `json:"policy"`
	AcceptedAttestationRefs      []HumanAttestationExactRefV2     `json:"accepted_attestation_refs"`
	OtherAttestationRefs         []HumanAttestationExactRefV2     `json:"other_attestation_refs"`
	DistinctReviewerIdentityRefs []HumanIdentityProofRefV2        `json:"distinct_reviewer_identity_refs"`
	SatisfiedRoleCounts          []HumanSatisfiedRoleCountV2      `json:"satisfied_role_counts"`
	AcceptCount                  uint32                           `json:"accept_count"`
	Threshold                    uint32                           `json:"threshold"`
	Resolution                   ResolutionV1                     `json:"resolution"`
	Vetoed                       bool                             `json:"vetoed"`
	VetoAttestationRef           *HumanAttestationExactRefV2      `json:"veto_attestation_ref,omitempty"`
	Conditions                   []runtimeports.ReviewConditionV2 `json:"conditions"`
	ConditionsDigest             core.Digest                      `json:"conditions_digest,omitempty"`
	EvidenceSetDigest            core.Digest                      `json:"evidence_set_digest"`
	ReviewerSetDigest            core.Digest                      `json:"reviewer_set_digest"`
	CheckedUnixNano              int64                            `json:"checked_unix_nano"`
	ExpiresUnixNano              int64                            `json:"expires_unix_nano"`
}

func (q HumanQuorumDecisionV2) Clone() HumanQuorumDecisionV2 {
	q.AcceptedAttestationRefs = append([]HumanAttestationExactRefV2(nil), q.AcceptedAttestationRefs...)
	q.OtherAttestationRefs = append([]HumanAttestationExactRefV2(nil), q.OtherAttestationRefs...)
	q.DistinctReviewerIdentityRefs = append([]HumanIdentityProofRefV2(nil), q.DistinctReviewerIdentityRefs...)
	q.SatisfiedRoleCounts = append([]HumanSatisfiedRoleCountV2(nil), q.SatisfiedRoleCounts...)
	q.Conditions = append([]runtimeports.ReviewConditionV2(nil), q.Conditions...)
	if q.VetoAttestationRef != nil {
		ref := *q.VetoAttestationRef
		q.VetoAttestationRef = &ref
	}
	return q
}
func (q HumanQuorumDecisionV2) digestValue() HumanQuorumDecisionV2 { q.Digest = ""; return q.Clone() }
func (q HumanQuorumDecisionV2) validateShape() error {
	if err := validateHumanFactIdentityV2(q.FactIdentityV1); err != nil {
		return err
	}
	if q.Revision != 1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "quorum decision must be create-once revision one")
	}
	for _, err := range []error{q.Panel.Validate(), q.Policy.Validate(), q.EvidenceSetDigest.Validate(), q.ReviewerSetDigest.Validate()} {
		if err != nil {
			return err
		}
	}
	if q.Panel.TenantID != q.TenantID || q.Policy.TenantID != q.TenantID {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "quorum exact refs cross tenant")
	}
	acceptsMayBeEmpty := q.Resolution != ResolutionAcceptV1 && q.Resolution != ResolutionConditionalV1
	if err := validateAttestationRefsV2(q.AcceptedAttestationRefs, q.TenantID, acceptsMayBeEmpty); err != nil {
		return err
	}
	if err := validateAttestationRefsV2(q.OtherAttestationRefs, q.TenantID, true); err != nil {
		return err
	}
	for _, accepted := range q.AcceptedAttestationRefs {
		if containsAttestationRefV2(q.OtherAttestationRefs, accepted) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "attestation cannot be both accepted and non-accepted")
		}
	}
	if err := validateIdentityRefsV2(q.DistinctReviewerIdentityRefs, q.TenantID); err != nil {
		return err
	}
	if len(q.DistinctReviewerIdentityRefs) != len(q.AcceptedAttestationRefs)+len(q.OtherAttestationRefs) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "one distinct reviewer identity proof is required per counted attestation")
	}
	expectedReviewerSet, err := ComputeHumanReviewerSetDigestV2(q.DistinctReviewerIdentityRefs)
	if err != nil {
		return err
	}
	if expectedReviewerSet != q.ReviewerSetDigest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "quorum reviewer set digest drifted")
	}
	if int(q.AcceptCount) != len(q.AcceptedAttestationRefs) || q.Threshold == 0 {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "quorum accept count/threshold drifted")
	}
	if q.AcceptCount < q.Threshold && (q.Resolution == ResolutionAcceptV1 || q.Resolution == ResolutionConditionalV1) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "accepting quorum is below threshold")
	}
	if len(q.SatisfiedRoleCounts) == 0 || !sort.SliceIsSorted(q.SatisfiedRoleCounts, func(i, j int) bool { return q.SatisfiedRoleCounts[i].Role < q.SatisfiedRoleCounts[j].Role }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "quorum role counts must be sorted")
	}
	for i, x := range q.SatisfiedRoleCounts {
		if invalidText(x.Role) || x.DistinctCurrentCount == 0 || int(x.DistinctCurrentCount) > len(q.DistinctReviewerIdentityRefs) || (i > 0 && q.SatisfiedRoleCounts[i-1].Role == x.Role) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "quorum role count invalid or duplicated")
		}
	}
	switch q.Resolution {
	case ResolutionAcceptV1, ResolutionConditionalV1, ResolutionRequestChangesV1, ResolutionEscalateHumanV1, ResolutionRejectV1, ResolutionInsufficientEvidenceV1:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "quorum resolution unsupported")
	}
	if q.Resolution == ResolutionRejectV1 {
		if !q.Vetoed || q.VetoAttestationRef == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "reject quorum requires an exact veto attestation")
		}
		if err := q.VetoAttestationRef.Validate(); err != nil {
			return err
		}
		if q.VetoAttestationRef.TenantID != q.TenantID || !containsAttestationRefV2(q.OtherAttestationRefs, *q.VetoAttestationRef) {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "veto attestation is not in the audited non-accept set")
		}
	} else if q.Vetoed || q.VetoAttestationRef != nil {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "non-reject quorum cannot claim veto")
	}
	if err := validateConditionsSetV2(q.Conditions, q.ConditionsDigest, q.Resolution == ResolutionConditionalV1); err != nil {
		return err
	}
	for _, condition := range q.Conditions {
		if condition.ExpiresUnixNano <= q.CheckedUnixNano || q.ExpiresUnixNano > condition.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "human quorum exceeds an exact condition TTL")
		}
	}
	if q.CheckedUnixNano < q.CreatedUnixNano || q.CheckedUnixNano >= q.ExpiresUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "quorum checked time is outside TTL")
	}
	return ValidateExpires(q.CreatedUnixNano, q.ExpiresUnixNano)
}
func SealHumanQuorumDecisionV2(q HumanQuorumDecisionV2) (HumanQuorumDecisionV2, error) {
	q = q.Clone()
	q.ContractVersion = HumanMultiSignContractV2
	q.Digest = ""
	sort.Slice(q.AcceptedAttestationRefs, func(i, j int) bool {
		return humanFactRefLessV2(humanFactExactRefV2(q.AcceptedAttestationRefs[i]), humanFactExactRefV2(q.AcceptedAttestationRefs[j]))
	})
	sort.Slice(q.OtherAttestationRefs, func(i, j int) bool {
		return humanFactRefLessV2(humanFactExactRefV2(q.OtherAttestationRefs[i]), humanFactExactRefV2(q.OtherAttestationRefs[j]))
	})
	sort.Slice(q.DistinctReviewerIdentityRefs, func(i, j int) bool {
		return identityRefLessV2(q.DistinctReviewerIdentityRefs[i], q.DistinctReviewerIdentityRefs[j])
	})
	sort.Slice(q.SatisfiedRoleCounts, func(i, j int) bool { return q.SatisfiedRoleCounts[i].Role < q.SatisfiedRoleCounts[j].Role })
	sortConditionsV2(q.Conditions)
	if err := q.validateShape(); err != nil {
		return HumanQuorumDecisionV2{}, err
	}
	d, err := sealHumanV2("HumanQuorumDecisionV2", q.digestValue())
	if err != nil {
		return HumanQuorumDecisionV2{}, err
	}
	q.Digest = d
	return q, q.Validate()
}
func (q HumanQuorumDecisionV2) Validate() error {
	if err := q.validateShape(); err != nil {
		return err
	}
	return validateSealedHumanV2("HumanQuorumDecisionV2", q.digestValue(), q.Digest)
}
func (q HumanQuorumDecisionV2) ExactRef() HumanQuorumDecisionExactRefV2 {
	return HumanQuorumDecisionExactRefV2{TenantID: q.TenantID, ID: q.ID, Revision: q.Revision, Digest: q.Digest}
}
func (q HumanQuorumDecisionV2) ValidateCurrent(expected HumanQuorumDecisionExactRefV2, now time.Time) error {
	if err := q.Validate(); err != nil {
		return err
	}
	if q.ExactRef() != expected {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "quorum exact current ref drifted")
	}
	return ValidateNow(now, q.CheckedUnixNano, q.ExpiresUnixNano)
}

type HumanVerdictStateV2 string

const (
	HumanVerdictAcceptedV2    HumanVerdictStateV2 = "accepted"
	HumanVerdictRejectedV2    HumanVerdictStateV2 = "rejected"
	HumanVerdictConditionalV2 HumanVerdictStateV2 = "conditional"
	HumanVerdictExpiredV2     HumanVerdictStateV2 = "expired"
	HumanVerdictRevokedV2     HumanVerdictStateV2 = "revoked"
	HumanVerdictSupersededV2  HumanVerdictStateV2 = "superseded"
)

type HumanVerdictV2 struct {
	FactIdentityV1
	Case                  HumanCaseExactRefV2                        `json:"case"`
	Target                HumanTargetExactRefV2                      `json:"target"`
	Round                 HumanRoundExactRefV2                       `json:"round"`
	Panel                 HumanPanelExactRefV2                       `json:"panel"`
	QuorumDecision        HumanQuorumDecisionExactRefV2              `json:"quorum_decision"`
	Policy                HumanQuorumPolicyBindingV2                 `json:"policy"`
	Scope                 core.ExecutionScope                        `json:"scope"`
	CurrentScope          runtimeports.ExecutionScopeBindingRefV2    `json:"current_scope"`
	ReviewerSetDigest     core.Digest                                `json:"reviewer_set_digest"`
	ReviewerAuthorityRefs []runtimeports.AuthorityBindingRefV2       `json:"reviewer_authority_refs"`
	BindingClosures       []runtimeports.ReviewComponentBindingRefV2 `json:"binding_closures"`
	AttestationRefs       []HumanAttestationExactRefV2               `json:"attestation_refs"`
	Evidence              []runtimeports.ReviewEvidenceRefV2         `json:"evidence"`
	EvidenceSetDigest     core.Digest                                `json:"evidence_set_digest"`
	Conditions            []runtimeports.ReviewConditionV2           `json:"conditions"`
	ConditionsDigest      core.Digest                                `json:"conditions_digest,omitempty"`
	ReasonCodes           []string                                   `json:"reason_codes"`
	State                 HumanVerdictStateV2                        `json:"state"`
	ExpiresUnixNano       int64                                      `json:"expires_unix_nano"`
	InvalidationReason    core.ReasonCode                            `json:"invalidation_reason,omitempty"`
}

func (v HumanVerdictV2) Clone() HumanVerdictV2 {
	v.ReviewerAuthorityRefs = append([]runtimeports.AuthorityBindingRefV2(nil), v.ReviewerAuthorityRefs...)
	v.BindingClosures = append([]runtimeports.ReviewComponentBindingRefV2(nil), v.BindingClosures...)
	v.AttestationRefs = append([]HumanAttestationExactRefV2(nil), v.AttestationRefs...)
	v.Evidence = append([]runtimeports.ReviewEvidenceRefV2(nil), v.Evidence...)
	v.Conditions = append([]runtimeports.ReviewConditionV2(nil), v.Conditions...)
	v.ReasonCodes = append([]string(nil), v.ReasonCodes...)
	return v
}
func (v HumanVerdictV2) digestValue() HumanVerdictV2 { v.Digest = ""; return v.Clone() }
func (v HumanVerdictV2) validateShape() error {
	if err := validateHumanFactIdentityV2(v.FactIdentityV1); err != nil {
		return err
	}
	if v.Revision != 1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human verdict must be create-once revision one")
	}
	for _, err := range []error{v.Case.Validate(), v.Target.Validate(), v.Round.Validate(), v.Panel.Validate(), v.QuorumDecision.Validate(), v.Policy.Validate(), v.Scope.Validate(), v.CurrentScope.Validate(), v.ReviewerSetDigest.Validate()} {
		if err != nil {
			return err
		}
	}
	if v.Case.TenantID != v.TenantID || v.Target.TenantID != v.TenantID || v.Round.TenantID != v.TenantID || v.Panel.TenantID != v.TenantID || v.QuorumDecision.TenantID != v.TenantID || v.Policy.TenantID != v.TenantID {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "human verdict exact refs cross tenant")
	}
	if len(v.ReviewerAuthorityRefs) == 0 || len(v.ReviewerAuthorityRefs) > MaxListItemsV1 || !sort.SliceIsSorted(v.ReviewerAuthorityRefs, func(i, j int) bool {
		return authorityRefKeyV2(v.ReviewerAuthorityRefs[i]) < authorityRefKeyV2(v.ReviewerAuthorityRefs[j])
	}) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "reviewer authority refs must be bounded and sorted")
	}
	for i, r := range v.ReviewerAuthorityRefs {
		if err := r.Validate(); err != nil {
			return err
		}
		if i > 0 && r == v.ReviewerAuthorityRefs[i-1] {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "reviewer authority ref duplicated")
		}
	}
	if len(v.BindingClosures) == 0 || len(v.BindingClosures) > MaxListItemsV1 || !sort.SliceIsSorted(v.BindingClosures, func(i, j int) bool {
		return componentBindingKeyV2(v.BindingClosures[i]) < componentBindingKeyV2(v.BindingClosures[j])
	}) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "binding closures must be bounded and sorted")
	}
	for i, r := range v.BindingClosures {
		if err := r.Validate(); err != nil {
			return err
		}
		if i > 0 && r == v.BindingClosures[i-1] {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "binding closure duplicated")
		}
	}
	if err := validateAttestationRefsV2(v.AttestationRefs, v.TenantID, false); err != nil {
		return err
	}
	if err := validateEvidenceSetV2(v.Evidence, v.EvidenceSetDigest); err != nil {
		return err
	}
	switch v.State {
	case HumanVerdictAcceptedV2, HumanVerdictRejectedV2, HumanVerdictConditionalV2, HumanVerdictExpiredV2, HumanVerdictRevokedV2, HumanVerdictSupersededV2:
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "human verdict state unsupported")
	}
	if err := validateConditionsSetV2(v.Conditions, v.ConditionsDigest, v.State == HumanVerdictConditionalV2); err != nil {
		return err
	}
	for _, condition := range v.Conditions {
		if condition.ExpiresUnixNano <= v.CreatedUnixNano || v.ExpiresUnixNano > condition.ExpiresUnixNano {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "human verdict exceeds an exact condition TTL")
		}
	}
	if err := validateSortedUniqueStringsV2(v.ReasonCodes, false); err != nil {
		return err
	}
	if v.State == HumanVerdictAcceptedV2 || v.State == HumanVerdictRejectedV2 || v.State == HumanVerdictConditionalV2 {
		if v.InvalidationReason != "" {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "active human verdict cannot carry invalidation")
		}
	} else if v.InvalidationReason == "" {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "terminal human verdict requires invalidation reason")
	}
	return ValidateExpires(v.CreatedUnixNano, v.ExpiresUnixNano)
}
func SealHumanVerdictV2(v HumanVerdictV2) (HumanVerdictV2, error) {
	v = v.Clone()
	v.ContractVersion = HumanMultiSignContractV2
	v.Digest = ""
	sort.Slice(v.ReviewerAuthorityRefs, func(i, j int) bool {
		return authorityRefKeyV2(v.ReviewerAuthorityRefs[i]) < authorityRefKeyV2(v.ReviewerAuthorityRefs[j])
	})
	sort.Slice(v.BindingClosures, func(i, j int) bool {
		return componentBindingKeyV2(v.BindingClosures[i]) < componentBindingKeyV2(v.BindingClosures[j])
	})
	sort.Slice(v.AttestationRefs, func(i, j int) bool {
		return humanFactRefLessV2(humanFactExactRefV2(v.AttestationRefs[i]), humanFactExactRefV2(v.AttestationRefs[j]))
	})
	sortEvidenceV2(v.Evidence)
	sortConditionsV2(v.Conditions)
	sort.Strings(v.ReasonCodes)
	if err := v.validateShape(); err != nil {
		return HumanVerdictV2{}, err
	}
	d, err := sealHumanV2("HumanVerdictV2", v.digestValue())
	if err != nil {
		return HumanVerdictV2{}, err
	}
	v.Digest = d
	return v, v.Validate()
}
func (v HumanVerdictV2) Validate() error {
	if err := v.validateShape(); err != nil {
		return err
	}
	return validateSealedHumanV2("HumanVerdictV2", v.digestValue(), v.Digest)
}
func (v HumanVerdictV2) ExactRef() HumanVerdictExactRefV2 {
	return HumanVerdictExactRefV2{TenantID: v.TenantID, ID: v.ID, Revision: v.Revision, Digest: v.Digest}
}
func (v HumanVerdictV2) ValidateCurrent(expected HumanVerdictExactRefV2, now time.Time) error {
	if err := v.Validate(); err != nil {
		return err
	}
	if v.ExactRef() != expected {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "human verdict exact current ref drifted")
	}
	if v.State != HumanVerdictAcceptedV2 && v.State != HumanVerdictRejectedV2 && v.State != HumanVerdictConditionalV2 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human verdict is terminal")
	}
	return ValidateNow(now, v.CreatedUnixNano, v.ExpiresUnixNano)
}

func ComputeHumanReviewerSetDigestV2(v []HumanIdentityProofRefV2) (core.Digest, error) {
	copyValue := append([]HumanIdentityProofRefV2(nil), v...)
	sort.Slice(copyValue, func(i, j int) bool { return identityRefLessV2(copyValue[i], copyValue[j]) })
	if len(copyValue) == 0 || len(copyValue) > MaxListItemsV1 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "reviewer identity set is empty or unbounded")
	}
	for i, ref := range copyValue {
		if err := ref.Validate(); err != nil {
			return "", err
		}
		if i > 0 && copyValue[i-1].TenantID == ref.TenantID && copyValue[i-1].Ref == ref.Ref {
			return "", core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "reviewer identity set contains a duplicate")
		}
	}
	return sealHumanV2("HumanReviewerIdentitySetV2", copyValue)
}

func sealHumanV2(kind string, value any) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.review.human-multisig", HumanMultiSignContractV2, kind, value)
}
func validateSealedHumanV2(kind string, value any, actual core.Digest) error {
	if err := actual.Validate(); err != nil {
		return err
	}
	expected, err := sealHumanV2(kind, value)
	if err != nil {
		return err
	}
	if expected != actual {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "human multisign digest does not bind exact content")
	}
	return nil
}
func humanFactRefLessV2(a, b humanFactExactRefV2) bool {
	if a.ID != b.ID {
		return a.ID < b.ID
	}
	if a.Revision != b.Revision {
		return a.Revision < b.Revision
	}
	return a.Digest < b.Digest
}
func identityRefLessV2(a, b HumanIdentityProofRefV2) bool {
	if a.Ref != b.Ref {
		return a.Ref < b.Ref
	}
	if a.Revision != b.Revision {
		return a.Revision < b.Revision
	}
	return a.Digest < b.Digest
}
func validateSortedUniqueStringsV2(v []string, allowEmpty bool) error {
	if (!allowEmpty && len(v) == 0) || len(v) > MaxListItemsV1 || !sort.StringsAreSorted(v) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "string set must be bounded and sorted")
	}
	for i, x := range v {
		if invalidText(x) || (i > 0 && v[i-1] == x) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "string set contains invalid or duplicated value")
		}
	}
	return nil
}
func containsStringV2(v []string, w string) bool {
	i := sort.SearchStrings(v, w)
	return i < len(v) && v[i] == w
}
func sortEvidenceV2(v []runtimeports.ReviewEvidenceRefV2) {
	sort.Slice(v, func(i, j int) bool { return v[i].Ref < v[j].Ref })
}
func validateEvidenceSetV2(v []runtimeports.ReviewEvidenceRefV2, d core.Digest) error {
	if len(v) == 0 || len(v) > MaxListItemsV1 || !sort.SliceIsSorted(v, func(i, j int) bool { return v[i].Ref < v[j].Ref }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "evidence set must be bounded, non-empty and sorted")
	}
	for i, x := range v {
		if err := x.Validate(); err != nil {
			return err
		}
		if i > 0 && v[i-1].Ref == x.Ref {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "evidence ref duplicated")
		}
	}
	expected, err := ComputeReviewEvidenceDigestV1(v)
	if err != nil {
		return err
	}
	if expected != d {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "evidence set digest drifted")
	}
	return nil
}
func sortConditionsV2(v []runtimeports.ReviewConditionV2) {
	sort.Slice(v, func(i, j int) bool {
		if v[i].ID != v[j].ID {
			return v[i].ID < v[j].ID
		}
		return v[i].Revision < v[j].Revision
	})
}
func validateConditionsSetV2(v []runtimeports.ReviewConditionV2, d core.Digest, required bool) error {
	if len(v) > runtimeports.MaxReviewConditionsV2 || !sort.SliceIsSorted(v, func(i, j int) bool {
		if v[i].ID != v[j].ID {
			return v[i].ID < v[j].ID
		}
		return v[i].Revision < v[j].Revision
	}) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "condition set is unbounded or unsorted")
	}
	for i, x := range v {
		if err := x.Validate(); err != nil {
			return err
		}
		if i > 0 && v[i-1].ID == x.ID {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "condition id duplicated")
		}
	}
	if required && len(v) == 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewConditionUnsatisfied, "conditional result requires conditions")
	}
	if !required && len(v) > 0 {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "non-conditional result cannot carry conditions")
	}
	if len(v) == 0 {
		if d != "" {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "empty conditions cannot carry digest")
		}
		return nil
	}
	expected, err := runtimeports.DigestReviewConditionsV2(v)
	if err != nil {
		return err
	}
	if expected != d {
		return core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "condition digest drifted")
	}
	return nil
}

func validateConditionsSetV2Compat(v []runtimeports.ReviewConditionV2, d core.Digest, required bool) error {
	if required && len(v) == 0 {
		if d.Validate() != nil {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonReviewConditionUnsatisfied, "legacy conditional result requires a conditions digest")
		}
		return nil
	}
	return validateConditionsSetV2(v, d, required)
}

// CanonicalAcceptedConditionsV2 derives the exact condition union contributed
// by the accepted Attestation set. Accept contributes the empty set;
// conditional_acceptance contributes its complete sealed set. The same ID may
// occur in more than one vote only when every field is identical.
func CanonicalAcceptedConditionsV2(attestations []HumanAttestationV2, accepted []HumanAttestationExactRefV2) ([]runtimeports.ReviewConditionV2, core.Digest, error) {
	byID := make(map[string]HumanAttestationV2, len(attestations))
	for _, attestation := range attestations {
		if err := attestation.Validate(); err != nil {
			return nil, "", err
		}
		if old, ok := byID[attestation.ID]; ok && old.ExactRef() != attestation.ExactRef() {
			return nil, "", core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "human attestation ID drifted")
		}
		byID[attestation.ID] = attestation
	}
	conditions := make(map[runtimeports.NamespacedNameV2]runtimeports.ReviewConditionV2)
	for _, ref := range accepted {
		attestation, ok := byID[ref.ID]
		if !ok || attestation.ExactRef() != ref {
			return nil, "", core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "accepted attestation exact ref drifted")
		}
		if attestation.Resolution != ResolutionAcceptV1 && attestation.Resolution != ResolutionConditionalV1 {
			return nil, "", core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "accepted set contains a non-accepting attestation")
		}
		for _, condition := range attestation.Conditions {
			if old, exists := conditions[condition.ID]; exists && old != condition {
				return nil, "", core.NewError(core.ErrorConflict, core.ReasonReviewConditionUnsatisfied, "same condition ID changed exact content across accepted votes")
			}
			conditions[condition.ID] = condition
		}
	}
	out := make([]runtimeports.ReviewConditionV2, 0, len(conditions))
	for _, condition := range conditions {
		out = append(out, condition)
	}
	sortConditionsV2(out)
	if len(out) == 0 {
		return nil, "", nil
	}
	digest, err := runtimeports.DigestReviewConditionsV2(out)
	return out, digest, err
}
func validateAttestationRefsV2(v []HumanAttestationExactRefV2, tenant core.TenantID, allowEmpty bool) error {
	if (!allowEmpty && len(v) == 0) || len(v) > MaxListItemsV1 || !sort.SliceIsSorted(v, func(i, j int) bool { return humanFactRefLessV2(humanFactExactRefV2(v[i]), humanFactExactRefV2(v[j])) }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "attestation refs must be bounded and sorted")
	}
	for i, r := range v {
		if err := r.Validate(); err != nil {
			return err
		}
		if r.TenantID != tenant || (i > 0 && humanFactExactRefV2(v[i-1]) == humanFactExactRefV2(r)) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "attestation ref duplicated or cross-tenant")
		}
	}
	return nil
}
func validateIdentityRefsV2(v []HumanIdentityProofRefV2, tenant core.TenantID) error {
	if len(v) == 0 || len(v) > MaxListItemsV1 || !sort.SliceIsSorted(v, func(i, j int) bool { return identityRefLessV2(v[i], v[j]) }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "identity refs must be bounded and sorted")
	}
	for i, r := range v {
		if err := r.Validate(); err != nil {
			return err
		}
		if r.TenantID != tenant || (i > 0 && v[i-1].TenantID == r.TenantID && v[i-1].Ref == r.Ref) {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "identity ref duplicated or cross-tenant")
		}
	}
	return nil
}
func containsAttestationRefV2(v []HumanAttestationExactRefV2, wanted HumanAttestationExactRefV2) bool {
	for _, ref := range v {
		if ref == wanted {
			return true
		}
	}
	return false
}
func authorityRefKeyV2(r runtimeports.AuthorityBindingRefV2) string {
	return r.Ref + "\x00" + string(r.Digest)
}
func componentBindingKeyV2(r runtimeports.ReviewComponentBindingRefV2) string {
	return r.BindingSetID + "\x00" + string(r.ComponentID) + "\x00" + string(r.Capability)
}
