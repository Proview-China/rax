package contract

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	PurgeIntentObjectKindV1 = "memory_knowledge_purge_intent"
	PurgeEffectKindV1       = "memory_knowledge_physical_purge"
	PurgeExecutionBlockedV1 = "unsupported_until_runtime_permit_evidence_settlement"
)

// PurgeIntentV1 is an Owner fact describing what a future governed physical
// purge must delete. It is not a Permit, an execution receipt, or proof that
// any bytes were deleted.
type PurgeIntentV1 struct {
	ContractVersion        string       `json:"contract_version"`
	ObjectKind             string       `json:"object_kind"`
	Ref                    Ref          `json:"ref"`
	Owner                  OwnerDomain  `json:"owner"`
	TenantID               string       `json:"tenant_id"`
	EffectKind             string       `json:"effect_kind"`
	ExecutionSupport       string       `json:"execution_support"`
	TargetKind             string       `json:"target_kind"`
	TargetRef              Ref          `json:"target_ref"`
	TombstoneRef           Ref          `json:"tombstone_ref"`
	ContentRefs            []ContentRef `json:"content_refs"`
	ProjectionRefs         []Ref        `json:"projection_refs"`
	AuthorityRef           Ref          `json:"authority_ref"`
	PolicyRef              Ref          `json:"policy_ref"`
	ScopeRef               Ref          `json:"scope_ref"`
	OperationRef           Ref          `json:"operation_ref"`
	RequestedByRef         Ref          `json:"requested_by_ref"`
	RetentionDecisionRef   Ref          `json:"retention_decision_ref"`
	LegalHoldInspectionRef Ref          `json:"legal_hold_inspection_ref"`
	NotBefore              time.Time    `json:"not_before"`
	CreatedAt              time.Time    `json:"created_at"`
	ExpiresAt              time.Time    `json:"expires_at"`
	Digest                 string       `json:"digest"`
}

func SealPurgeIntentV1(in PurgeIntentV1) (PurgeIntentV1, error) {
	in.ContractVersion, in.ObjectKind = FrameworkContractVersionV1, PurgeIntentObjectKindV1
	in.EffectKind, in.ExecutionSupport = PurgeEffectKindV1, PurgeExecutionBlockedV1
	contents, err := normalizeContentRefs(in.ContentRefs)
	if err != nil {
		return PurgeIntentV1{}, err
	}
	in.ContentRefs = contents
	projections, err := normalizeSemanticRefsAllowEmpty(in.ProjectionRefs)
	if err != nil {
		return PurgeIntentV1{}, err
	}
	in.ProjectionRefs = projections
	in.NotBefore, in.CreatedAt, in.ExpiresAt = in.NotBefore.UTC(), in.CreatedAt.UTC(), in.ExpiresAt.UTC()
	in.Ref.Digest, in.Digest = "", ""
	digest, err := Digest(in)
	if err != nil {
		return PurgeIntentV1{}, err
	}
	in.Ref.Digest, in.Digest = digest, digest
	if err := in.Validate(in.CreatedAt); err != nil {
		return PurgeIntentV1{}, err
	}
	return in, nil
}

func (in PurgeIntentV1) Validate(now time.Time) error {
	if in.ContractVersion != FrameworkContractVersionV1 || in.ObjectKind != PurgeIntentObjectKindV1 || (in.Owner != OwnerMemory && in.Owner != OwnerKnowledge) || strings.TrimSpace(in.TenantID) == "" || (in.TargetKind != "record" && (in.Owner != OwnerKnowledge || in.TargetKind != "source")) || in.EffectKind != PurgeEffectKindV1 || in.ExecutionSupport != PurgeExecutionBlockedV1 || in.CreatedAt.IsZero() || in.NotBefore.Before(in.CreatedAt) || !in.ExpiresAt.After(in.NotBefore) || !in.ExpiresAt.After(now) {
		return fmt.Errorf("%w: purge intent", ErrInvalidArgument)
	}
	for _, ref := range []Ref{in.Ref, in.TargetRef, in.TombstoneRef, in.AuthorityRef, in.PolicyRef, in.ScopeRef, in.OperationRef, in.RequestedByRef, in.RetentionDecisionRef, in.LegalHoldInspectionRef} {
		if ref.Validate() != nil {
			return fmt.Errorf("%w: purge exact ref", ErrInvalidArgument)
		}
	}
	contents, err := normalizeContentRefs(in.ContentRefs)
	if err != nil || !slices.Equal(contents, in.ContentRefs) {
		return fmt.Errorf("%w: purge content refs", ErrInvalidArgument)
	}
	projections, err := normalizeSemanticRefsAllowEmpty(in.ProjectionRefs)
	if err != nil || !slices.Equal(projections, in.ProjectionRefs) {
		return fmt.Errorf("%w: purge projection refs", ErrInvalidArgument)
	}
	copy := in
	copy.Ref.Digest, copy.Digest = "", ""
	digest, err := Digest(copy)
	if err != nil {
		return err
	}
	if in.Ref.Digest != digest || in.Digest != digest {
		return fmt.Errorf("%w: purge intent digest", ErrEvidenceConflict)
	}
	return nil
}

func normalizeContentRefs(in []ContentRef) ([]ContentRef, error) {
	out := slices.Clone(in)
	seen := make(map[string]struct{}, len(out))
	for _, ref := range out {
		if ref.Validate() != nil {
			return nil, ErrInvalidArgument
		}
		if _, exists := seen[ref.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate purge content", ErrEvidenceConflict)
		}
		seen[ref.ID] = struct{}{}
	}
	slices.SortFunc(out, func(a, b ContentRef) int { return strings.Compare(a.ID, b.ID) })
	if out == nil {
		out = []ContentRef{}
	}
	return out, nil
}
