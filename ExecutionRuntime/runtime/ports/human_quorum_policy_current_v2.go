package ports

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	HumanQuorumPolicyCurrentContractVersionV2 = "praxis.runtime.human-quorum-policy-current/v2"
	MaxHumanQuorumPolicyItemsV2               = 128
)

const humanQuorumPolicyCurrentCanonicalDomainV2 = "praxis.runtime.human-quorum-policy-current"

// HumanQuorumPolicyCurrentSubjectV2 is the stable Policy-Owner lookup
// coordinate. Domain is tenant-defined and does not grant authority.
type HumanQuorumPolicyCurrentSubjectV2 struct {
	TenantID core.TenantID `json:"tenant_id"`
	Domain   string        `json:"domain"`
}

func (s HumanQuorumPolicyCurrentSubjectV2) Validate() error {
	if strings.TrimSpace(string(s.TenantID)) == "" || !canonicalHumanQuorumPolicyTextV2(s.Domain, 512) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human quorum policy subject is incomplete or non-canonical")
	}
	return nil
}

// HumanQuorumRoleRequirementV2 is Runtime-neutral. Review adapters map it
// field-by-field and must not type-pun a Review-owned role requirement.
type HumanQuorumRoleRequirementV2 struct {
	Role    string `json:"role"`
	Minimum uint32 `json:"minimum"`
}

type HumanQuorumPolicyCurrentProjectionRefV2 struct {
	ID       string        `json:"id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r HumanQuorumPolicyCurrentProjectionRefV2) Validate() error {
	if !canonicalHumanQuorumPolicyTextV2(r.ID, 512) || r.Revision == 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human quorum policy projection ref is incomplete")
	}
	return r.Digest.Validate()
}

type HumanQuorumPolicyProjectionStateV2 string

const (
	HumanQuorumPolicyProjectionActiveV2     HumanQuorumPolicyProjectionStateV2 = "active"
	HumanQuorumPolicyProjectionRevokedV2    HumanQuorumPolicyProjectionStateV2 = "revoked"
	HumanQuorumPolicyProjectionExpiredV2    HumanQuorumPolicyProjectionStateV2 = "expired"
	HumanQuorumPolicyProjectionSupersededV2 HumanQuorumPolicyProjectionStateV2 = "superseded"
)

func (s HumanQuorumPolicyProjectionStateV2) Validate() error {
	switch s {
	case HumanQuorumPolicyProjectionActiveV2, HumanQuorumPolicyProjectionRevokedV2, HumanQuorumPolicyProjectionExpiredV2, HumanQuorumPolicyProjectionSupersededV2:
		return nil
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "human quorum policy projection state is invalid")
	}
}

// HumanQuorumPolicyCurrentProjectionV2 is the immutable, sealed Policy Owner
// fact consumed by Human Multi-Sign V2. Checked, Expires and Digest are fixed
// at publication; passage of time only makes ValidateCurrent fail.
type HumanQuorumPolicyCurrentProjectionV2 struct {
	ContractVersion             string                                  `json:"contract_version"`
	Ref                         HumanQuorumPolicyCurrentProjectionRefV2 `json:"ref"`
	Subject                     HumanQuorumPolicyCurrentSubjectV2       `json:"subject"`
	State                       HumanQuorumPolicyProjectionStateV2      `json:"state"`
	Current                     bool                                    `json:"current"`
	AcceptThreshold             uint32                                  `json:"accept_threshold"`
	MaximumPanelSize            uint32                                  `json:"maximum_panel_size"`
	RoleRequirements            []HumanQuorumRoleRequirementV2          `json:"role_requirements"`
	RejectVetoRoles             []string                                `json:"reject_veto_roles"`
	DelegationRequired          bool                                    `json:"delegation_required"`
	ProductionSelfReviewAllowed bool                                    `json:"production_self_review_allowed"`
	MaxPanelDurationNanos       int64                                   `json:"max_panel_duration_nanos"`
	MaxVoteTTLNanos             int64                                   `json:"max_vote_ttl_nanos"`
	CheckedUnixNano             int64                                   `json:"checked_unix_nano"`
	ExpiresUnixNano             int64                                   `json:"expires_unix_nano"`
	ProjectionDigest            core.Digest                             `json:"projection_digest"`
}

func (p HumanQuorumPolicyCurrentProjectionV2) Clone() HumanQuorumPolicyCurrentProjectionV2 {
	p.RoleRequirements = append([]HumanQuorumRoleRequirementV2(nil), p.RoleRequirements...)
	p.RejectVetoRoles = append([]string(nil), p.RejectVetoRoles...)
	return p
}

func (p HumanQuorumPolicyCurrentProjectionV2) Validate() error {
	if p.ContractVersion != HumanQuorumPolicyCurrentContractVersionV2 || p.Ref.Validate() != nil || p.Subject.Validate() != nil || p.State.Validate() != nil || p.CheckedUnixNano <= 0 || p.CheckedUnixNano >= p.ExpiresUnixNano || p.Ref.Digest != p.ProjectionDigest {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "human quorum policy projection identity or time is incomplete")
	}
	if p.AcceptThreshold == 0 || p.MaximumPanelSize < p.AcceptThreshold || p.MaximumPanelSize > MaxHumanQuorumPolicyItemsV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "human quorum policy K-of-N bounds are invalid")
	}
	if err := validateHumanQuorumRoleRequirementsV2(p.RoleRequirements, p.MaximumPanelSize); err != nil {
		return err
	}
	if err := validateHumanQuorumVetoRolesV2(p.RejectVetoRoles); err != nil {
		return err
	}
	if !p.DelegationRequired || p.ProductionSelfReviewAllowed {
		return core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "human quorum production policy cannot disable delegation proof or allow self review")
	}
	if p.MaxPanelDurationNanos <= 0 || p.MaxVoteTTLNanos <= 0 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "human quorum policy duration bounds are incomplete")
	}
	if (p.State == HumanQuorumPolicyProjectionActiveV2) != p.Current {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human quorum policy state/current truth table drifted")
	}
	wantID, err := DeriveHumanQuorumPolicyCurrentProjectionIDV2(p.Subject)
	if err != nil || p.Ref.ID != wantID {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "human quorum policy stable projection ID drifted")
	}
	wantDigest, err := DigestHumanQuorumPolicyCurrentProjectionV2(p)
	if err != nil || wantDigest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "human quorum policy projection digest drifted")
	}
	return nil
}

func (p HumanQuorumPolicyCurrentProjectionV2) ValidateCurrent(expected HumanQuorumPolicyCurrentProjectionRefV2, subject HumanQuorumPolicyCurrentSubjectV2, now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if expected != p.Ref || subject != p.Subject {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "human quorum policy current ref or subject drifted")
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "human quorum policy currentness clock regressed")
	}
	if p.State != HumanQuorumPolicyProjectionActiveV2 || !p.Current || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human quorum policy is terminal or expired")
	}
	return nil
}

func DeriveHumanQuorumPolicyCurrentProjectionIDV2(subject HumanQuorumPolicyCurrentSubjectV2) (string, error) {
	if err := subject.Validate(); err != nil {
		return "", err
	}
	digest, err := core.CanonicalJSONDigest(humanQuorumPolicyCurrentCanonicalDomainV2, HumanQuorumPolicyCurrentContractVersionV2, "HumanQuorumPolicyCurrentProjectionIdentityV2", subject)
	if err != nil {
		return "", err
	}
	return "human-quorum-policy-current-" + strings.TrimPrefix(string(digest), "sha256:"), nil
}

func DigestHumanQuorumPolicyCurrentProjectionV2(p HumanQuorumPolicyCurrentProjectionV2) (core.Digest, error) {
	p = p.Clone()
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	return core.CanonicalJSONDigest(humanQuorumPolicyCurrentCanonicalDomainV2, HumanQuorumPolicyCurrentContractVersionV2, "HumanQuorumPolicyCurrentProjectionV2", p)
}

func SealHumanQuorumPolicyCurrentProjectionV2(p HumanQuorumPolicyCurrentProjectionV2) (HumanQuorumPolicyCurrentProjectionV2, error) {
	p = p.Clone()
	p.ContractVersion = HumanQuorumPolicyCurrentContractVersionV2
	sort.Slice(p.RoleRequirements, func(i, j int) bool { return p.RoleRequirements[i].Role < p.RoleRequirements[j].Role })
	sort.Strings(p.RejectVetoRoles)
	wantID, err := DeriveHumanQuorumPolicyCurrentProjectionIDV2(p.Subject)
	if err != nil {
		return HumanQuorumPolicyCurrentProjectionV2{}, err
	}
	if p.Ref.ID == "" {
		p.Ref.ID = wantID
	} else if p.Ref.ID != wantID {
		return HumanQuorumPolicyCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "human quorum policy stable projection ID drifted")
	}
	p.Ref.Digest = ""
	p.ProjectionDigest = ""
	digest, err := DigestHumanQuorumPolicyCurrentProjectionV2(p)
	if err != nil {
		return HumanQuorumPolicyCurrentProjectionV2{}, err
	}
	p.Ref.Digest = digest
	p.ProjectionDigest = digest
	return p, p.Validate()
}

type HumanQuorumPolicyCurrentResolveRequestV2 struct {
	Subject HumanQuorumPolicyCurrentSubjectV2 `json:"subject"`
}

func (r HumanQuorumPolicyCurrentResolveRequestV2) Validate() error { return r.Subject.Validate() }

type HumanQuorumPolicyCurrentPublishRequestV2 struct {
	Previous *HumanQuorumPolicyCurrentProjectionRefV2 `json:"previous,omitempty"`
	Value    HumanQuorumPolicyCurrentProjectionV2     `json:"value"`
}

func (r HumanQuorumPolicyCurrentPublishRequestV2) Validate() error {
	if err := r.Value.Validate(); err != nil {
		return err
	}
	if r.Previous == nil {
		if r.Value.Ref.Revision != 1 {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "initial human quorum policy revision must be one")
		}
		return nil
	}
	if err := r.Previous.Validate(); err != nil {
		return err
	}
	if r.Value.Ref.ID != r.Previous.ID || r.Value.Ref.Revision != r.Previous.Revision+1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "human quorum policy stable ID or revision drifted")
	}
	return nil
}

type HumanQuorumPolicyCurrentPublishReceiptV2 struct {
	Ref     HumanQuorumPolicyCurrentProjectionRefV2 `json:"ref"`
	Created bool                                    `json:"created"`
}

// HumanQuorumPolicyCurrentReaderV2 is the only public read surface for this
// Policy Owner. Resolve and current Inspect atomically verify the current full
// Ref; historical Inspect depends only on an exact Ref. Successes are deep
// clones. Closed errors are InvalidArgument, NotFound, Conflict,
// PreconditionFailed, Forbidden, Indeterminate and Unavailable.
type HumanQuorumPolicyCurrentReaderV2 interface {
	ResolveCurrentHumanQuorumPolicyV2(context.Context, HumanQuorumPolicyCurrentResolveRequestV2) (HumanQuorumPolicyCurrentProjectionRefV2, error)
	InspectCurrentHumanQuorumPolicyV2(context.Context, HumanQuorumPolicyCurrentSubjectV2, HumanQuorumPolicyCurrentProjectionRefV2) (HumanQuorumPolicyCurrentProjectionV2, error)
	InspectHistoricalHumanQuorumPolicyV2(context.Context, HumanQuorumPolicyCurrentProjectionRefV2) (HumanQuorumPolicyCurrentProjectionV2, error)
}

// HumanQuorumPolicyCurrentPublisherV2 is Policy-Owner-only. Publication is
// append-only create-once with a full-ref current CAS and revision+1. A lost
// reply is recovered only by exact historical Inspect of the same Ref.
type HumanQuorumPolicyCurrentPublisherV2 interface {
	PublishHumanQuorumPolicyCurrentV2(context.Context, HumanQuorumPolicyCurrentPublishRequestV2) (HumanQuorumPolicyCurrentPublishReceiptV2, error)
}

func validateHumanQuorumRoleRequirementsV2(values []HumanQuorumRoleRequirementV2, maximum uint32) error {
	if len(values) == 0 || len(values) > MaxHumanQuorumPolicyItemsV2 || !sort.SliceIsSorted(values, func(i, j int) bool { return values[i].Role < values[j].Role }) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "human quorum role requirements must be bounded and sorted")
	}
	var total uint64
	for i, value := range values {
		if !canonicalHumanQuorumPolicyTextV2(value.Role, 512) || value.Minimum == 0 {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "human quorum role requirement is incomplete")
		}
		if i > 0 && values[i-1].Role == value.Role {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "human quorum role requirement is duplicated")
		}
		total += uint64(value.Minimum)
	}
	if total > uint64(maximum) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "human quorum role minima exceed maximum panel size")
	}
	return nil
}

func validateHumanQuorumVetoRolesV2(values []string) error {
	if len(values) > MaxHumanQuorumPolicyItemsV2 || !sort.StringsAreSorted(values) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "human quorum veto roles must be bounded and sorted")
	}
	for i, value := range values {
		if !canonicalHumanQuorumPolicyTextV2(value, 512) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "human quorum veto role is incomplete")
		}
		if i > 0 && values[i-1] == value {
			return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "human quorum veto role is duplicated")
		}
	}
	return nil
}

func canonicalHumanQuorumPolicyTextV2(value string, maximum int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= maximum
}
