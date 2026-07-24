package contract

import (
	"reflect"
	"strings"
	"time"
)

const (
	RestorePlanGovernanceContractV2 = "praxis.continuity/restore-plan-governance/v2"
	RestorePlanFactSchemaV2         = "praxis.continuity/restore-plan-fact/v2"
	RestorePlanCapabilityV2         = "restore-plan-governance-v2"
)

type RestorePlanStateV2 string

const (
	RestorePlanDraftV2                  RestorePlanStateV2 = "draft"
	RestorePlanCheckpointInspectedV2    RestorePlanStateV2 = "checkpoint_inspected"
	RestorePlanCompatibilityInspectedV2 RestorePlanStateV2 = "compatibility_inspected"
	RestorePlanAdmittedV2               RestorePlanStateV2 = "admitted"
	RestorePlanRejectedV2               RestorePlanStateV2 = "rejected"
	RestorePlanExpiredV2                RestorePlanStateV2 = "expired"
	RestorePlanSubmittedV2              RestorePlanStateV2 = "submitted"
)

type RestorePlanRefV2 ExactFactRefV2

func (r RestorePlanRefV2) Validate() error {
	value := ExactFactRefV2(r)
	if err := value.Validate(); err != nil {
		return err
	}
	if value.ContractVersion != RestorePlanGovernanceContractV2 || value.SchemaRef != RestorePlanFactSchemaV2 {
		return NewError(ErrInvalidArgument, "restore_plan_ref", "wrong contract or schema")
	}
	return validateRestorePlanOwnerV2(value.Owner)
}

func (r RestorePlanRefV2) Exact() ExactFactRefV2 { return ExactFactRefV2(r) }

// RestoreInstanceProposalV2 is a proposal only. Runtime remains the sole Owner
// of Instance, Epoch, Lease, Fence, RestoreAttempt, and Activation facts.
type RestoreInstanceProposalV2 struct {
	InstanceID string `json:"instance_id"`
	Epoch      uint64 `json:"epoch"`
	LeaseID    string `json:"lease_id"`
	LeaseEpoch uint64 `json:"lease_epoch"`
	FenceEpoch uint64 `json:"fence_epoch"`
}

func (p RestoreInstanceProposalV2) Validate(sourceInstanceID string, sourceEpoch uint64) error {
	for field, value := range map[string]string{"instance_id": p.InstanceID, "lease_id": p.LeaseID} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if sourceEpoch == 0 || p.InstanceID == sourceInstanceID || p.Epoch <= sourceEpoch || p.LeaseEpoch == 0 || p.FenceEpoch == 0 {
		return NewError(ErrRestoreIncompatible, "instance_proposal", "restore requires a fresh Instance, higher Epoch, new Lease, and Fence proposal")
	}
	return nil
}

// RestorePlanFactV2 is Continuity-owned historical planning material. Even an
// admitted or submitted plan does not grant Runtime eligibility, review,
// authority, permit, fence, stage, activation, or Provider access.
type RestorePlanFactV2 struct {
	ContractVersion string             `json:"contract_version"`
	SchemaRef       string             `json:"schema_ref"`
	PlanID          string             `json:"plan_id"`
	Revision        uint64             `json:"revision"`
	Digest          string             `json:"digest"`
	Owner           OwnerBinding       `json:"owner"`
	Scope           Scope              `json:"scope"`
	State           RestorePlanStateV2 `json:"state"`
	IdempotencyKey  string             `json:"idempotency_key"`

	CheckpointConsistencyRef     ExactFactRefV2              `json:"checkpoint_consistency_ref"`
	ManifestSealRef              CheckpointManifestSealRefV2 `json:"manifest_seal_ref"`
	FrozenRefSetDigest           string                      `json:"frozen_ref_set_digest"`
	SourceInstanceRef            ExactFactRefV2              `json:"source_instance_ref"`
	SourceInstanceEpoch          uint64                      `json:"source_instance_epoch"`
	ProposedInstance             RestoreInstanceProposalV2   `json:"proposed_instance"`
	RequiredParticipantSetDigest string                      `json:"required_participant_set_digest"`

	ContextGenerationRef     ExactFactRefV2   `json:"context_generation_ref"`
	ContextFrameRefs         []ExactFactRefV2 `json:"context_frame_refs"`
	CompatibilityRefs        []ExactFactRefV2 `json:"compatibility_refs"`
	CurrentnessRefs          []ExactFactRefV2 `json:"currentness_refs"`
	ReviewRequirementRefs    []ExactFactRefV2 `json:"review_requirement_refs"`
	AuthorityRequirementRefs []ExactFactRefV2 `json:"authority_requirement_refs"`
	BudgetRequirementRefs    []ExactFactRefV2 `json:"budget_requirement_refs"`
	BindingRequirementRefs   []ExactFactRefV2 `json:"binding_requirement_refs"`

	ConflictDomain        string           `json:"conflict_domain"`
	ResidualPolicyRef     ExactFactRefV2   `json:"residual_policy_ref"`
	ResidualRefs          []ExactFactRefV2 `json:"residual_refs"`
	RecoveryCredentialRef ExactFactRefV2   `json:"recovery_credential_ref"`
	CreatedUnixNano       int64            `json:"created_unix_nano"`
	UpdatedUnixNano       int64            `json:"updated_unix_nano"`
	ExpiresUnixNano       int64            `json:"expires_unix_nano"`
}

func (p RestorePlanFactV2) Validate() error {
	if p.ContractVersion != RestorePlanGovernanceContractV2 || p.SchemaRef != RestorePlanFactSchemaV2 {
		return NewError(ErrInvalidArgument, "restore_plan", "wrong contract or schema")
	}
	if err := validateRestorePlanOwnerV2(p.Owner); err != nil {
		return err
	}
	for field, value := range map[string]string{
		"plan_id": p.PlanID, "idempotency_key": p.IdempotencyKey,
		"conflict_domain": p.ConflictDomain,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := p.Scope.Validate(); err != nil {
		return err
	}
	if p.Revision == 0 || p.CreatedUnixNano <= 0 || p.UpdatedUnixNano < p.CreatedUnixNano || p.ExpiresUnixNano <= p.CreatedUnixNano {
		return NewError(ErrInvalidArgument, "restore_plan", "invalid revision or timestamps")
	}
	if !validRestorePlanStateV2(p.State) {
		return NewError(ErrInvalidArgument, "restore_plan_state", "unknown state")
	}
	if p.State == RestorePlanExpiredV2 && p.UpdatedUnixNano < p.ExpiresUnixNano {
		return NewError(ErrInvalidArgument, "restore_plan_state", "expired state predates plan expiry")
	}
	if err := ValidateDigest("frozen_ref_set_digest", p.FrozenRefSetDigest); err != nil {
		return err
	}
	if err := ValidateDigest("required_participant_set_digest", p.RequiredParticipantSetDigest); err != nil {
		return err
	}
	if err := p.ManifestSealRef.Validate(); err != nil {
		return err
	}
	if err := p.ProposedInstance.Validate(p.SourceInstanceRef.ID, p.SourceInstanceEpoch); err != nil {
		return err
	}
	if !strings.HasPrefix(p.ConflictDomain, "tenant/"+p.Scope.TenantID+"/") {
		return NewError(ErrRestoreIncompatible, "conflict_domain", "must remain in the stable tenant conflict domain")
	}
	if err := validateRestorePlanOwnerKindV2(p.CheckpointConsistencyRef, "praxis/runtime", "checkpoint_consistency_fact_v2", "checkpoint_consistency_ref"); err != nil {
		return err
	}
	if err := validateRestorePlanOwnerKindV2(p.SourceInstanceRef, "praxis/runtime", "instance_fact_v2", "source_instance_ref"); err != nil {
		return err
	}
	if err := validateRestorePlanOwnerKindV2(p.ContextGenerationRef, "praxis/context", "context_generation_fact_v2", "context_generation_ref"); err != nil {
		return err
	}
	if p.ManifestSealRef.Exact().TenantID != p.Scope.TenantID || p.ManifestSealRef.Exact().ScopeDigest != p.Scope.ExecutionScopeDigest {
		return NewError(ErrRestoreIncompatible, "manifest_seal_ref", "manifest Seal belongs to another tenant or execution scope")
	}
	allRefs := []ExactFactRefV2{
		p.CheckpointConsistencyRef, p.SourceInstanceRef, p.ContextGenerationRef,
		p.ResidualPolicyRef, p.RecoveryCredentialRef,
	}
	sets := [][]ExactFactRefV2{
		p.ContextFrameRefs, p.CompatibilityRefs, p.CurrentnessRefs,
		p.ReviewRequirementRefs, p.AuthorityRequirementRefs,
		p.BudgetRequirementRefs, p.BindingRequirementRefs, p.ResidualRefs,
	}
	for _, values := range sets {
		normalized, err := normalizeExactRefsV2(values, "restore_plan_refs")
		if err != nil {
			return err
		}
		allRefs = append(allRefs, normalized...)
	}
	if len(p.ContextFrameRefs) == 0 || len(p.CompatibilityRefs) == 0 || len(p.CurrentnessRefs) == 0 ||
		len(p.ReviewRequirementRefs) == 0 || len(p.AuthorityRequirementRefs) == 0 ||
		len(p.BudgetRequirementRefs) == 0 || len(p.BindingRequirementRefs) == 0 {
		return NewError(ErrRestoreIncompatible, "restore_requirements", "all Context, compatibility, currentness, review, authority, budget, and binding requirement sets are required")
	}
	for _, ref := range allRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.TenantID != p.Scope.TenantID || ref.ScopeDigest != p.Scope.ExecutionScopeDigest {
			return NewError(ErrRestoreIncompatible, "restore_plan_refs", "cross-tenant or cross-scope reference splice")
		}
	}
	expected, err := p.CanonicalDigest()
	if err != nil {
		return err
	}
	if p.Digest == "" || p.Digest != expected {
		return NewError(ErrRevisionConflict, "restore_plan_digest", "canonical digest mismatch")
	}
	return nil
}

func (p RestorePlanFactV2) ValidateCurrent(now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now.IsZero() {
		return NewError(ErrInvalidArgument, "now", "injected current time is required")
	}
	if now.Before(time.Unix(0, p.CreatedUnixNano)) {
		return NewError(ErrRestoreIncompatible, "restore_plan_ttl", "restore plan is not current before creation")
	}
	if p.State == RestorePlanExpiredV2 || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return NewError(ErrRestoreIncompatible, "restore_plan_ttl", "restore plan is expired")
	}
	return nil
}

func (p RestorePlanFactV2) CanonicalDigest() (string, error) {
	copy := p.Clone()
	copy.Digest = ""
	var err error
	sets := []struct {
		target *[]ExactFactRefV2
		field  string
	}{
		{&copy.ContextFrameRefs, "context_frame_refs"},
		{&copy.CompatibilityRefs, "compatibility_refs"},
		{&copy.CurrentnessRefs, "currentness_refs"},
		{&copy.ReviewRequirementRefs, "review_requirement_refs"},
		{&copy.AuthorityRequirementRefs, "authority_requirement_refs"},
		{&copy.BudgetRequirementRefs, "budget_requirement_refs"},
		{&copy.BindingRequirementRefs, "binding_requirement_refs"},
		{&copy.ResidualRefs, "residual_refs"},
	}
	for _, set := range sets {
		*set.target, err = normalizeExactRefsV2(*set.target, set.field)
		if err != nil {
			return "", err
		}
	}
	return CanonicalDigest(copy)
}

func (p RestorePlanFactV2) Clone() RestorePlanFactV2 {
	copy := p
	copy.ContextFrameRefs = append([]ExactFactRefV2{}, p.ContextFrameRefs...)
	copy.CompatibilityRefs = append([]ExactFactRefV2{}, p.CompatibilityRefs...)
	copy.CurrentnessRefs = append([]ExactFactRefV2{}, p.CurrentnessRefs...)
	copy.ReviewRequirementRefs = append([]ExactFactRefV2{}, p.ReviewRequirementRefs...)
	copy.AuthorityRequirementRefs = append([]ExactFactRefV2{}, p.AuthorityRequirementRefs...)
	copy.BudgetRequirementRefs = append([]ExactFactRefV2{}, p.BudgetRequirementRefs...)
	copy.BindingRequirementRefs = append([]ExactFactRefV2{}, p.BindingRequirementRefs...)
	copy.ResidualRefs = append([]ExactFactRefV2{}, p.ResidualRefs...)
	return copy
}

func (p RestorePlanFactV2) Ref() RestorePlanRefV2 {
	return RestorePlanRefV2(ExactFactRefV2{
		ContractVersion: p.ContractVersion, SchemaRef: p.SchemaRef, Owner: p.Owner,
		TenantID: p.Scope.TenantID, ID: p.PlanID, Revision: p.Revision,
		Digest: p.Digest, ScopeDigest: p.Scope.ExecutionScopeDigest,
	})
}

func SameRestorePlanStableIdentityV2(current, next RestorePlanFactV2) bool {
	left, right := current.Clone(), next.Clone()
	left.Revision, right.Revision = 0, 0
	left.Digest, right.Digest = "", ""
	left.State, right.State = "", ""
	left.UpdatedUnixNano, right.UpdatedUnixNano = 0, 0
	return reflect.DeepEqual(left, right)
}

func AdvanceRestorePlanStateV2(current RestorePlanFactV2, next RestorePlanStateV2, now time.Time) error {
	if now.IsZero() {
		return NewError(ErrInvalidArgument, "now", "injected current time is required")
	}
	if next == RestorePlanExpiredV2 {
		if now.Before(time.Unix(0, current.ExpiresUnixNano)) {
			return NewError(ErrRevisionConflict, "restore_plan_state", "plan cannot expire before its TTL")
		}
		switch current.State {
		case RestorePlanDraftV2, RestorePlanCheckpointInspectedV2, RestorePlanCompatibilityInspectedV2, RestorePlanAdmittedV2:
			return nil
		default:
			return NewError(ErrRevisionConflict, "restore_plan_state", "terminal plan cannot expire")
		}
	}
	if !now.Before(time.Unix(0, current.ExpiresUnixNano)) {
		return NewError(ErrRestoreIncompatible, "restore_plan_ttl", "expired plan may only transition to expired")
	}
	allowed := false
	switch current.State {
	case RestorePlanDraftV2:
		allowed = next == RestorePlanCheckpointInspectedV2 || next == RestorePlanRejectedV2
	case RestorePlanCheckpointInspectedV2:
		allowed = next == RestorePlanCompatibilityInspectedV2 || next == RestorePlanRejectedV2
	case RestorePlanCompatibilityInspectedV2:
		allowed = next == RestorePlanAdmittedV2 || next == RestorePlanRejectedV2
	case RestorePlanAdmittedV2:
		allowed = next == RestorePlanSubmittedV2
	}
	if !allowed {
		return NewError(ErrRevisionConflict, "restore_plan_state", "invalid or terminal plan transition")
	}
	return nil
}

func validRestorePlanStateV2(state RestorePlanStateV2) bool {
	switch state {
	case RestorePlanDraftV2, RestorePlanCheckpointInspectedV2, RestorePlanCompatibilityInspectedV2,
		RestorePlanAdmittedV2, RestorePlanRejectedV2, RestorePlanExpiredV2, RestorePlanSubmittedV2:
		return true
	default:
		return false
	}
}

func validateRestorePlanOwnerKindV2(ref ExactFactRefV2, component, kind, field string) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if ref.Owner.ComponentID != component || ref.Owner.FactKind != kind {
		return NewError(ErrRestoreIncompatible, field, "wrong semantic Owner or fact kind")
	}
	return nil
}

func validateRestorePlanOwnerV2(owner OwnerBinding) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	if owner.ComponentID != ContinuityComponentID || owner.Capability != RestorePlanCapabilityV2 || owner.FactKind != "restore_plan_fact_v2" {
		return NewError(ErrInvalidArgument, "owner_binding", "wrong Continuity Restore Plan owner, capability, or fact kind")
	}
	return nil
}
