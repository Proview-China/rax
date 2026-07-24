package contract

import (
	"reflect"
	"time"
)

const (
	RewindPlanGovernanceContractV2 = "praxis.continuity/rewind-plan-governance/v2"
	RewindPlanFactSchemaV2         = "praxis.continuity/rewind-plan-fact/v2"
	RewindPlanCapabilityV2         = "rewind-plan-governance-v2"
)

type RewindPlanStateV2 string

const (
	RewindPlanDraftV2                 RewindPlanStateV2 = "draft"
	RewindPlanWorkspaceInspectedV2    RewindPlanStateV2 = "workspace_inspected"
	RewindPlanDependenciesInspectedV2 RewindPlanStateV2 = "dependencies_inspected"
	RewindPlanAdmittedV2              RewindPlanStateV2 = "admitted"
	RewindPlanRejectedV2              RewindPlanStateV2 = "rejected"
	RewindPlanExpiredV2               RewindPlanStateV2 = "expired"
	RewindPlanSubmittedV2             RewindPlanStateV2 = "submitted"
)

type RewindPlanRefV2 ExactFactRefV2

func (r RewindPlanRefV2) Validate() error {
	value := ExactFactRefV2(r)
	if err := value.Validate(); err != nil {
		return err
	}
	if value.ContractVersion != RewindPlanGovernanceContractV2 || value.SchemaRef != RewindPlanFactSchemaV2 {
		return NewError(ErrInvalidArgument, "rewind_plan_ref", "wrong contract or schema")
	}
	return validateRewindPlanOwnerV2(value.Owner)
}

func (r RewindPlanRefV2) Exact() ExactFactRefV2 { return ExactFactRefV2(r) }

// RewindPlanFactV2 is planning material only. It binds exact facts needed to
// request a new Sandbox workspace-commit effect, but grants no Review,
// Runtime, Sandbox Provider, or filesystem authority.
type RewindPlanFactV2 struct {
	ContractVersion string            `json:"contract_version"`
	SchemaRef       string            `json:"schema_ref"`
	PlanID          string            `json:"plan_id"`
	Revision        uint64            `json:"revision"`
	Digest          string            `json:"digest"`
	Owner           OwnerBinding      `json:"owner"`
	Scope           Scope             `json:"scope"`
	State           RewindPlanStateV2 `json:"state"`
	IdempotencyKey  string            `json:"idempotency_key"`

	CheckpointConsistencyRef  ExactFactRefV2              `json:"checkpoint_consistency_ref"`
	ManifestSealRef           CheckpointManifestSealRefV2 `json:"manifest_seal_ref"`
	SourceWorkspaceViewRef    ExactFactRefV2              `json:"source_workspace_view_ref"`
	ExpectedWorkspaceRevision string                      `json:"expected_workspace_revision"`
	FileScopeDigest           string                      `json:"file_scope_digest"`
	KeepChangeSetRefs         []ExactFactRefV2            `json:"keep_change_set_refs"`
	DropChangeSetRefs         []ExactFactRefV2            `json:"drop_change_set_refs"`
	PlannedChangeSetRef       ExactFactRefV2              `json:"planned_change_set_ref"`
	DependencyInspectionRefs  []ExactFactRefV2            `json:"dependency_inspection_refs"`
	ReviewRequirementRefs     []ExactFactRefV2            `json:"review_requirement_refs"`
	IrreversibleEffectRefs    []ExactFactRefV2            `json:"irreversible_effect_refs"`
	ResidualRefs              []ExactFactRefV2            `json:"residual_refs"`
	WorkspaceSelectionDigest  string                      `json:"workspace_selection_digest"`
	ConflictDomain            string                      `json:"conflict_domain"`
	CreatedUnixNano           int64                       `json:"created_unix_nano"`
	UpdatedUnixNano           int64                       `json:"updated_unix_nano"`
	ExpiresUnixNano           int64                       `json:"expires_unix_nano"`
}

func (p RewindPlanFactV2) Validate() error {
	if p.ContractVersion != RewindPlanGovernanceContractV2 || p.SchemaRef != RewindPlanFactSchemaV2 {
		return NewError(ErrInvalidArgument, "rewind_plan", "wrong contract or schema")
	}
	if err := validateRewindPlanOwnerV2(p.Owner); err != nil {
		return err
	}
	for field, value := range map[string]string{"plan_id": p.PlanID, "idempotency_key": p.IdempotencyKey, "conflict_domain": p.ConflictDomain} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if err := p.Scope.Validate(); err != nil {
		return err
	}
	if p.Revision == 0 || p.CreatedUnixNano <= 0 || p.UpdatedUnixNano < p.CreatedUnixNano || p.ExpiresUnixNano <= p.CreatedUnixNano {
		return NewError(ErrInvalidArgument, "rewind_plan", "invalid revision or timestamps")
	}
	if !validRewindPlanStateV2(p.State) {
		return NewError(ErrInvalidArgument, "rewind_plan_state", "unknown state")
	}
	if p.State == RewindPlanExpiredV2 && p.UpdatedUnixNano < p.ExpiresUnixNano {
		return NewError(ErrInvalidArgument, "rewind_plan_state", "expired state predates plan expiry")
	}
	if (p.State == RewindPlanAdmittedV2 || p.State == RewindPlanSubmittedV2) && len(p.ResidualRefs) != 0 {
		return NewError(ErrRewindConflict, "residual_refs", "admitted or submitted rewind cannot carry residuals")
	}
	if err := ValidateDigest("expected_workspace_revision", p.ExpectedWorkspaceRevision); err != nil {
		return err
	}
	if err := ValidateDigest("file_scope_digest", p.FileScopeDigest); err != nil {
		return err
	}
	if err := ValidateDigest("workspace_selection_digest", p.WorkspaceSelectionDigest); err != nil {
		return err
	}
	if p.ConflictDomain != "tenant/"+p.Scope.TenantID+"/sandbox/workspace" {
		return NewError(ErrRewindConflict, "conflict_domain", "rewind must use the stable tenant workspace conflict domain")
	}
	if err := validateRewindOwnerKindV2(p.CheckpointConsistencyRef, "praxis/runtime", "checkpoint_consistency_fact_v2", "checkpoint_consistency_ref"); err != nil {
		return err
	}
	if err := p.ManifestSealRef.Validate(); err != nil {
		return err
	}
	if err := validateRewindOwnerKindV2(p.SourceWorkspaceViewRef, "praxis/sandbox", "workspace_view_v1", "source_workspace_view_ref"); err != nil {
		return err
	}
	if err := validateRewindOwnerKindV2(p.PlannedChangeSetRef, "praxis/sandbox", "workspace_change_set_v1", "planned_change_set_ref"); err != nil {
		return err
	}
	sets := []struct {
		values []ExactFactRefV2
		field  string
	}{
		{p.KeepChangeSetRefs, "keep_change_set_refs"},
		{p.DropChangeSetRefs, "drop_change_set_refs"},
		{p.DependencyInspectionRefs, "dependency_inspection_refs"},
		{p.ReviewRequirementRefs, "review_requirement_refs"},
		{p.IrreversibleEffectRefs, "irreversible_effect_refs"},
		{p.ResidualRefs, "residual_refs"},
	}
	allRefs := []ExactFactRefV2{p.CheckpointConsistencyRef, p.ManifestSealRef.Exact(), p.SourceWorkspaceViewRef, p.PlannedChangeSetRef}
	for _, set := range sets {
		normalized, err := normalizeExactRefsV2(set.values, set.field)
		if err != nil {
			return err
		}
		allRefs = append(allRefs, normalized...)
	}
	if len(p.KeepChangeSetRefs)+len(p.DropChangeSetRefs) == 0 || len(p.DependencyInspectionRefs) == 0 || len(p.ReviewRequirementRefs) == 0 {
		return NewError(ErrRewindConflict, "rewind_requirements", "workspace selection, dependency inspection, and review requirement refs are required")
	}
	selection := make(map[ExactFactIdentityKeyV2]bool, len(p.KeepChangeSetRefs)+len(p.DropChangeSetRefs))
	for _, ref := range p.KeepChangeSetRefs {
		if err := validateRewindOwnerKindV2(ref, "praxis/sandbox", "workspace_change_set_v1", "keep_change_set_refs"); err != nil {
			return err
		}
		selection[ref.IdentityKey()] = true
	}
	for _, ref := range p.DropChangeSetRefs {
		if err := validateRewindOwnerKindV2(ref, "praxis/sandbox", "workspace_change_set_v1", "drop_change_set_refs"); err != nil {
			return err
		}
		if selection[ref.IdentityKey()] {
			return NewError(ErrRewindConflict, "workspace_selection", "same ChangeSet cannot be kept and dropped")
		}
	}
	for _, ref := range p.ReviewRequirementRefs {
		if err := validateRewindOwnerKindV2(ref, "praxis/review", "review_requirement_fact_v2", "review_requirement_refs"); err != nil {
			return err
		}
	}
	for _, ref := range p.DependencyInspectionRefs {
		if err := validateRewindOwnerKindV2(ref, "praxis/continuity", "rewind_dependency_inspection_v1", "dependency_inspection_refs"); err != nil {
			return err
		}
	}
	for _, ref := range p.IrreversibleEffectRefs {
		if err := validateRewindOwnerKindV2(ref, "praxis/runtime", "operation_settlement_fact_v4", "irreversible_effect_refs"); err != nil {
			return err
		}
	}
	for _, ref := range p.ResidualRefs {
		if err := validateRewindOwnerKindV2(ref, "praxis/continuity", "rewind_residual_fact_v1", "residual_refs"); err != nil {
			return err
		}
	}
	if err := validateRewindSelectionCoordinatesV2(p); err != nil {
		return err
	}
	for _, ref := range allRefs {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.TenantID != p.Scope.TenantID || ref.ScopeDigest != p.Scope.ExecutionScopeDigest {
			return NewError(ErrRewindConflict, "rewind_plan_refs", "cross-tenant or cross-scope reference splice")
		}
	}
	selectionDigest, err := p.CanonicalWorkspaceSelectionDigest()
	if err != nil || selectionDigest != p.WorkspaceSelectionDigest {
		return NewError(ErrRevisionConflict, "workspace_selection_digest", "canonical workspace selection digest mismatch")
	}
	expected, err := p.CanonicalDigest()
	if err != nil {
		return err
	}
	if p.Digest == "" || p.Digest != expected {
		return NewError(ErrRevisionConflict, "rewind_plan_digest", "canonical digest mismatch")
	}
	return nil
}

func (p RewindPlanFactV2) ValidateCurrent(now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now.IsZero() {
		return NewError(ErrInvalidArgument, "now", "injected current time is required")
	}
	if now.Before(time.Unix(0, p.CreatedUnixNano)) || p.State == RewindPlanExpiredV2 || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return NewError(ErrRewindConflict, "rewind_plan_ttl", "rewind plan is not current")
	}
	return nil
}

func (p RewindPlanFactV2) CanonicalWorkspaceSelectionDigest() (string, error) {
	keep, err := normalizeExactRefsV2(p.KeepChangeSetRefs, "keep_change_set_refs")
	if err != nil {
		return "", err
	}
	drop, err := normalizeExactRefsV2(p.DropChangeSetRefs, "drop_change_set_refs")
	if err != nil {
		return "", err
	}
	return CanonicalDigest(struct {
		SourceWorkspaceViewRef    ExactFactRefV2
		ExpectedWorkspaceRevision string
		FileScopeDigest           string
		Keep                      []ExactFactRefV2
		Drop                      []ExactFactRefV2
		PlannedChangeSetRef       ExactFactRefV2
	}{p.SourceWorkspaceViewRef, p.ExpectedWorkspaceRevision, p.FileScopeDigest, keep, drop, p.PlannedChangeSetRef})
}

func (p RewindPlanFactV2) CanonicalDigest() (string, error) {
	copy := p.Clone()
	copy.Digest = ""
	var err error
	sets := []struct {
		target *[]ExactFactRefV2
		field  string
	}{
		{&copy.KeepChangeSetRefs, "keep_change_set_refs"}, {&copy.DropChangeSetRefs, "drop_change_set_refs"},
		{&copy.DependencyInspectionRefs, "dependency_inspection_refs"}, {&copy.ReviewRequirementRefs, "review_requirement_refs"},
		{&copy.IrreversibleEffectRefs, "irreversible_effect_refs"}, {&copy.ResidualRefs, "residual_refs"},
	}
	for _, set := range sets {
		*set.target, err = normalizeExactRefsV2(*set.target, set.field)
		if err != nil {
			return "", err
		}
	}
	return CanonicalDigest(copy)
}

func (p RewindPlanFactV2) Clone() RewindPlanFactV2 {
	copy := p
	copy.KeepChangeSetRefs = append([]ExactFactRefV2{}, p.KeepChangeSetRefs...)
	copy.DropChangeSetRefs = append([]ExactFactRefV2{}, p.DropChangeSetRefs...)
	copy.DependencyInspectionRefs = append([]ExactFactRefV2{}, p.DependencyInspectionRefs...)
	copy.ReviewRequirementRefs = append([]ExactFactRefV2{}, p.ReviewRequirementRefs...)
	copy.IrreversibleEffectRefs = append([]ExactFactRefV2{}, p.IrreversibleEffectRefs...)
	copy.ResidualRefs = append([]ExactFactRefV2{}, p.ResidualRefs...)
	return copy
}

func (p RewindPlanFactV2) Ref() RewindPlanRefV2 {
	return RewindPlanRefV2(ExactFactRefV2{ContractVersion: p.ContractVersion, SchemaRef: p.SchemaRef, Owner: p.Owner, TenantID: p.Scope.TenantID, ID: p.PlanID, Revision: p.Revision, Digest: p.Digest, ScopeDigest: p.Scope.ExecutionScopeDigest})
}

func SameRewindPlanStableIdentityV2(current, next RewindPlanFactV2) bool {
	left, right := current.Clone(), next.Clone()
	left.Revision, right.Revision, left.Digest, right.Digest, left.State, right.State, left.UpdatedUnixNano, right.UpdatedUnixNano = 0, 0, "", "", "", "", 0, 0
	return reflect.DeepEqual(left, right)
}

func AdvanceRewindPlanStateV2(current RewindPlanFactV2, next RewindPlanStateV2, now time.Time) error {
	if now.IsZero() {
		return NewError(ErrInvalidArgument, "now", "injected current time is required")
	}
	if next == RewindPlanExpiredV2 {
		if now.Before(time.Unix(0, current.ExpiresUnixNano)) {
			return NewError(ErrRevisionConflict, "rewind_plan_state", "plan cannot expire before its TTL")
		}
		switch current.State {
		case RewindPlanDraftV2, RewindPlanWorkspaceInspectedV2, RewindPlanDependenciesInspectedV2, RewindPlanAdmittedV2:
			return nil
		default:
			return NewError(ErrRevisionConflict, "rewind_plan_state", "terminal plan cannot expire")
		}
	}
	if !now.Before(time.Unix(0, current.ExpiresUnixNano)) {
		return NewError(ErrRewindConflict, "rewind_plan_ttl", "expired plan may only transition to expired")
	}
	allowed := false
	switch current.State {
	case RewindPlanDraftV2:
		allowed = next == RewindPlanWorkspaceInspectedV2 || next == RewindPlanRejectedV2
	case RewindPlanWorkspaceInspectedV2:
		allowed = next == RewindPlanDependenciesInspectedV2 || next == RewindPlanRejectedV2
	case RewindPlanDependenciesInspectedV2:
		allowed = next == RewindPlanAdmittedV2 || next == RewindPlanRejectedV2
	case RewindPlanAdmittedV2:
		allowed = next == RewindPlanSubmittedV2
	}
	if !allowed {
		return NewError(ErrRevisionConflict, "rewind_plan_state", "invalid or terminal plan transition")
	}
	return nil
}

func validRewindPlanStateV2(state RewindPlanStateV2) bool {
	switch state {
	case RewindPlanDraftV2, RewindPlanWorkspaceInspectedV2, RewindPlanDependenciesInspectedV2, RewindPlanAdmittedV2, RewindPlanRejectedV2, RewindPlanExpiredV2, RewindPlanSubmittedV2:
		return true
	default:
		return false
	}
}

func validateRewindOwnerKindV2(ref ExactFactRefV2, component, kind, field string) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if ref.Owner.ComponentID != component || ref.Owner.FactKind != kind {
		return NewError(ErrRewindConflict, field, "wrong semantic Owner or fact kind")
	}
	return nil
}

func validateRewindPlanOwnerV2(owner OwnerBinding) error {
	if err := owner.Validate(); err != nil {
		return err
	}
	if owner.ComponentID != ContinuityComponentID || owner.Capability != RewindPlanCapabilityV2 || owner.FactKind != "rewind_plan_fact_v2" {
		return NewError(ErrInvalidArgument, "owner_binding", "wrong Continuity Rewind Plan owner, capability, or fact kind")
	}
	return nil
}

type rewindExactCoordinateV2 struct {
	ContractVersion string
	SchemaRef       string
	Owner           OwnerBinding
	TenantID        string
	ID              string
	Revision        uint64
	ScopeDigest     string
}

func rewindCoordinateV2(ref ExactFactRefV2) rewindExactCoordinateV2 {
	return rewindExactCoordinateV2{ref.ContractVersion, ref.SchemaRef, ref.Owner, ref.TenantID, ref.ID, ref.Revision, ref.ScopeDigest}
}

func validateRewindSelectionCoordinatesV2(plan RewindPlanFactV2) error {
	coordinates := make(map[rewindExactCoordinateV2]string, len(plan.KeepChangeSetRefs)+len(plan.DropChangeSetRefs)+1)
	for _, group := range []struct {
		name string
		refs []ExactFactRefV2
	}{{"keep", plan.KeepChangeSetRefs}, {"drop", plan.DropChangeSetRefs}, {"planned", []ExactFactRefV2{plan.PlannedChangeSetRef}}} {
		for _, ref := range group.refs {
			key := rewindCoordinateV2(ref)
			if previous, exists := coordinates[key]; exists {
				return NewError(ErrRewindConflict, "workspace_selection", "same ChangeSet coordinate appears in "+previous+" and "+group.name+" selections")
			}
			coordinates[key] = group.name
		}
	}
	return nil
}
