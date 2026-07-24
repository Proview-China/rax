package contract

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	FrameworkContractVersionV1  = "praxis.memory-knowledge/framework/v1"
	ScopeCoordinateObjectKindV1 = "memory_knowledge_scope_coordinate"
	ViewPolicyObjectKindV1      = "memory_knowledge_view_policy"
)

type ScopeKind string

const (
	ScopeRunWorking         ScopeKind = "run_working"
	ScopeIdentityPrivate    ScopeKind = "identity_private"
	ScopeAgentLineage       ScopeKind = "agent_lineage"
	ScopeUser               ScopeKind = "user"
	ScopeTeam               ScopeKind = "team"
	ScopeDomain             ScopeKind = "domain"
	ScopeOrganization       ScopeKind = "organization_shared"
	ScopePersonalSource     ScopeKind = "personal_source"
	ScopeProjectSource      ScopeKind = "project_source"
	ScopeTeamSource         ScopeKind = "team_source"
	ScopeDomainSource       ScopeKind = "domain_source"
	ScopeOrganizationSource ScopeKind = "organization_source"
	ScopeExternalSource     ScopeKind = "external_source"
)

type DisclosureMode string

const (
	DisclosureFull          DisclosureMode = "full"
	DisclosureSummary       DisclosureMode = "summary"
	DisclosureCitationOnly  DisclosureMode = "citation_only"
	DisclosureExistenceOnly DisclosureMode = "existence_only"
	DisclosureDenied        DisclosureMode = "denied"
)

type ScopeCoordinateV1 struct {
	ContractVersion string      `json:"contract_version"`
	ObjectKind      string      `json:"object_kind"`
	Owner           OwnerDomain `json:"owner"`
	TenantID        string      `json:"tenant_id"`
	ScopeKind       ScopeKind   `json:"scope_kind"`
	ScopeID         string      `json:"scope_id"`
	IdentityRef     Ref         `json:"identity_ref"`
	IdentityEpoch   uint64      `json:"identity_epoch"`
	LineageRef      Ref         `json:"lineage_ref"`
	AuthorityRef    Ref         `json:"authority_ref"`
	AuthorityEpoch  uint64      `json:"authority_epoch"`
	PolicyRef       Ref         `json:"policy_ref"`
	Purpose         string      `json:"purpose"`
	Sensitivity     string      `json:"sensitivity"`
	Digest          string      `json:"digest"`
}

func SealScopeCoordinateV1(in ScopeCoordinateV1) (ScopeCoordinateV1, error) {
	in.ContractVersion = FrameworkContractVersionV1
	in.ObjectKind = ScopeCoordinateObjectKindV1
	in.Digest = ""
	digest, err := Digest(in)
	if err != nil {
		return ScopeCoordinateV1{}, err
	}
	in.Digest = digest
	if err := in.Validate(); err != nil {
		return ScopeCoordinateV1{}, err
	}
	return in, nil
}

func (in ScopeCoordinateV1) Validate() error {
	if in.ContractVersion != FrameworkContractVersionV1 || in.ObjectKind != ScopeCoordinateObjectKindV1 || (in.Owner != OwnerMemory && in.Owner != OwnerKnowledge) || strings.TrimSpace(in.TenantID) == "" || strings.TrimSpace(in.ScopeID) == "" || in.IdentityEpoch == 0 || in.AuthorityEpoch == 0 || strings.TrimSpace(in.Purpose) == "" || strings.TrimSpace(in.Sensitivity) == "" {
		return fmt.Errorf("%w: incomplete scope coordinate", ErrInvalidArgument)
	}
	if !scopeKindOwnedBy(in.ScopeKind, in.Owner) {
		return fmt.Errorf("%w: scope kind does not belong to owner", ErrInvalidArgument)
	}
	for _, ref := range []Ref{in.IdentityRef, in.LineageRef, in.AuthorityRef, in.PolicyRef} {
		if err := ref.Validate(); err != nil {
			return err
		}
	}
	copy := in
	copy.Digest = ""
	digest, err := Digest(copy)
	if err != nil {
		return err
	}
	if digest != in.Digest {
		return fmt.Errorf("%w: scope coordinate digest", ErrEvidenceConflict)
	}
	return nil
}

type ScopeGrantV1 struct {
	ScopeRef   Ref            `json:"scope_ref"`
	Disclosure DisclosureMode `json:"disclosure"`
}

type ViewPolicyV1 struct {
	ContractVersion string            `json:"contract_version"`
	ObjectKind      string            `json:"object_kind"`
	Ref             Ref               `json:"ref"`
	Owner           OwnerDomain       `json:"owner"`
	PrincipalScope  ScopeCoordinateV1 `json:"principal_scope"`
	Grants          []ScopeGrantV1    `json:"grants"`
	MaxItems        int               `json:"max_items"`
	MaxBytes        int64             `json:"max_bytes"`
	MaxTokens       int               `json:"max_tokens"`
	CreatedAt       time.Time         `json:"created_at"`
	ExpiresAt       time.Time         `json:"expires_at"`
	Digest          string            `json:"digest"`
}

func SealViewPolicyV1(in ViewPolicyV1) (ViewPolicyV1, error) {
	in.ContractVersion = FrameworkContractVersionV1
	in.ObjectKind = ViewPolicyObjectKindV1
	if err := in.PrincipalScope.Validate(); err != nil {
		return ViewPolicyV1{}, err
	}
	grants, err := normalizeScopeGrants(in.Grants)
	if err != nil {
		return ViewPolicyV1{}, err
	}
	in.Grants = grants
	in.CreatedAt = in.CreatedAt.UTC()
	in.ExpiresAt = in.ExpiresAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	digest, err := Digest(in)
	if err != nil {
		return ViewPolicyV1{}, err
	}
	in.Ref.Digest, in.Digest = digest, digest
	if err := in.Validate(in.CreatedAt); err != nil {
		return ViewPolicyV1{}, err
	}
	return in, nil
}

func (in ViewPolicyV1) Validate(now time.Time) error {
	if in.ContractVersion != FrameworkContractVersionV1 || in.ObjectKind != ViewPolicyObjectKindV1 || in.Owner != in.PrincipalScope.Owner || in.MaxItems <= 0 || in.MaxItems > 4096 || in.MaxBytes <= 0 || in.MaxBytes > 64<<20 || in.MaxTokens <= 0 || in.CreatedAt.IsZero() || !in.ExpiresAt.After(in.CreatedAt) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: incomplete or expired view policy", ErrInvalidArgument)
	}
	if err := in.Ref.Validate(); err != nil {
		return err
	}
	if err := in.PrincipalScope.Validate(); err != nil {
		return err
	}
	grants, err := normalizeScopeGrants(in.Grants)
	if err != nil || !slices.Equal(grants, in.Grants) {
		return fmt.Errorf("%w: non-canonical grants", ErrInvalidArgument)
	}
	copy := in
	copy.Ref.Digest, copy.Digest = "", ""
	digest, err := Digest(copy)
	if err != nil {
		return err
	}
	if in.Ref.Digest != digest || in.Digest != digest {
		return fmt.Errorf("%w: view policy digest", ErrEvidenceConflict)
	}
	return nil
}

func normalizeScopeGrants(in []ScopeGrantV1) ([]ScopeGrantV1, error) {
	out := slices.Clone(in)
	seen := make(map[string]struct{}, len(out))
	for _, grant := range out {
		if err := grant.ScopeRef.Validate(); err != nil {
			return nil, err
		}
		if !validDisclosure(grant.Disclosure) {
			return nil, fmt.Errorf("%w: disclosure", ErrInvalidArgument)
		}
		if _, exists := seen[grant.ScopeRef.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate semantic scope", ErrEvidenceConflict)
		}
		seen[grant.ScopeRef.ID] = struct{}{}
	}
	slices.SortFunc(out, func(a, b ScopeGrantV1) int {
		if c := strings.Compare(a.ScopeRef.ID, b.ScopeRef.ID); c != 0 {
			return c
		}
		if a.ScopeRef.Revision < b.ScopeRef.Revision {
			return -1
		}
		if a.ScopeRef.Revision > b.ScopeRef.Revision {
			return 1
		}
		return strings.Compare(a.ScopeRef.Digest, b.ScopeRef.Digest)
	})
	if out == nil {
		out = []ScopeGrantV1{}
	}
	return out, nil
}

func scopeKindOwnedBy(kind ScopeKind, owner OwnerDomain) bool {
	switch owner {
	case OwnerMemory:
		return kind == ScopeRunWorking || kind == ScopeIdentityPrivate || kind == ScopeAgentLineage || kind == ScopeUser || kind == ScopeTeam || kind == ScopeDomain || kind == ScopeOrganization
	case OwnerKnowledge:
		return kind == ScopePersonalSource || kind == ScopeProjectSource || kind == ScopeTeamSource || kind == ScopeDomainSource || kind == ScopeOrganizationSource || kind == ScopeExternalSource
	default:
		return false
	}
}

func validDisclosure(mode DisclosureMode) bool {
	return mode == DisclosureFull || mode == DisclosureSummary || mode == DisclosureCitationOnly || mode == DisclosureExistenceOnly || mode == DisclosureDenied
}
