package contract

import "time"

type ForkPlan struct {
	PlanID                 string        `json:"plan_id"`
	SourceNodeRef          string        `json:"source_node_ref"`
	SourceCheckpointRef    string        `json:"source_checkpoint_ref,omitempty"`
	ParentLineageID        string        `json:"parent_lineage_id"`
	NewLineageID           string        `json:"new_lineage_id"`
	NewSessionIntent       string        `json:"new_session_intent"`
	ContextGeneration      string        `json:"context_generation"`
	AuthorityCeilingDigest string        `json:"authority_ceiling_digest"`
	RequiredRevalidations  []string      `json:"required_revalidations"`
	InheritedEffectRefs    []string      `json:"inherited_effect_refs"`
	ResidualRefs           []ResidualRef `json:"residual_refs"`
	ExpiresUnixNano        int64         `json:"expires_unix_nano"`
	Digest                 string        `json:"digest"`
}

func (p ForkPlan) CanonicalDigest() (string, error) {
	copy := p
	copy.Digest = ""
	var err error
	copy.RequiredRevalidations, err = NormalizeSet(p.RequiredRevalidations)
	if err != nil {
		return "", err
	}
	copy.InheritedEffectRefs, err = NormalizeSet(p.InheritedEffectRefs)
	if err != nil {
		return "", err
	}
	copy.ResidualRefs, err = NormalizeResiduals(p.ResidualRefs)
	if err != nil {
		return "", err
	}
	return CanonicalDigest(copy)
}

func (p ForkPlan) Validate(now time.Time) error {
	for field, value := range map[string]string{
		"plan_id": p.PlanID, "source_node_ref": p.SourceNodeRef,
		"parent_lineage_id": p.ParentLineageID, "new_lineage_id": p.NewLineageID,
		"new_session_intent": p.NewSessionIntent, "context_generation": p.ContextGeneration,
		"authority_ceiling_digest": p.AuthorityCeilingDigest,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if p.ParentLineageID == p.NewLineageID {
		return NewError(ErrInvalidArgument, "new_lineage_id", "fork must create a new lineage")
	}
	if len(p.RequiredRevalidations) == 0 {
		return NewError(ErrInvalidArgument, "required_revalidations", "profile, binding, tool, sandbox, or review revalidation is required")
	}
	if p.SourceCheckpointRef != "" {
		if err := ValidateToken("source_checkpoint_ref", p.SourceCheckpointRef); err != nil {
			return err
		}
	}
	for _, values := range [][]string{p.RequiredRevalidations, p.InheritedEffectRefs} {
		if _, err := NormalizeSet(values); err != nil {
			return err
		}
	}
	for _, residual := range p.ResidualRefs {
		if err := residual.Validate(); err != nil {
			return err
		}
	}
	if p.ExpiresUnixNano <= now.UnixNano() {
		return NewError(ErrInvalidArgument, "expires_unix_nano", "plan is expired")
	}
	expected, err := p.CanonicalDigest()
	return validatePlanDigest(p.Digest, expected, err)
}

type RewindPlan struct {
	PlanID                 string        `json:"plan_id"`
	TargetCheckpointRef    string        `json:"target_checkpoint_ref"`
	KeepChangeSetRefs      []string      `json:"keep_change_set_refs"`
	DropChangeSetRefs      []string      `json:"drop_change_set_refs"`
	DependencyConflictRefs []string      `json:"dependency_conflict_refs"`
	IrreversibleEffectRefs []string      `json:"irreversible_effect_refs"`
	RequiredReviewRefs     []string      `json:"required_review_refs"`
	ResidualRefs           []ResidualRef `json:"residual_refs"`
	Approved               bool          `json:"approved"`
	ExpiresUnixNano        int64         `json:"expires_unix_nano"`
	Digest                 string        `json:"digest"`
}

func (p RewindPlan) CanonicalDigest() (string, error) {
	copy := p
	copy.Digest = ""
	var err error
	sets := []*[]string{
		&copy.KeepChangeSetRefs, &copy.DropChangeSetRefs, &copy.DependencyConflictRefs,
		&copy.IrreversibleEffectRefs, &copy.RequiredReviewRefs,
	}
	originals := [][]string{
		p.KeepChangeSetRefs, p.DropChangeSetRefs, p.DependencyConflictRefs,
		p.IrreversibleEffectRefs, p.RequiredReviewRefs,
	}
	for i := range sets {
		*sets[i], err = NormalizeSet(originals[i])
		if err != nil {
			return "", err
		}
	}
	copy.ResidualRefs, err = NormalizeResiduals(p.ResidualRefs)
	if err != nil {
		return "", err
	}
	return CanonicalDigest(copy)
}

func (p RewindPlan) Validate(now time.Time) error {
	if err := ValidateToken("plan_id", p.PlanID); err != nil {
		return err
	}
	if err := ValidateToken("target_checkpoint_ref", p.TargetCheckpointRef); err != nil {
		return err
	}
	keep, err := NormalizeSet(p.KeepChangeSetRefs)
	if err != nil {
		return err
	}
	drop, err := NormalizeSet(p.DropChangeSetRefs)
	if err != nil {
		return err
	}
	for _, values := range [][]string{p.DependencyConflictRefs, p.IrreversibleEffectRefs, p.RequiredReviewRefs} {
		if _, err := NormalizeSet(values); err != nil {
			return err
		}
	}
	for _, residual := range p.ResidualRefs {
		if err := residual.Validate(); err != nil {
			return err
		}
	}
	for _, value := range keep {
		if Contains(drop, value) {
			return NewError(ErrInvalidArgument, "change_sets", "same change set cannot be kept and dropped")
		}
	}
	if p.Approved && len(p.DependencyConflictRefs) > 0 {
		return NewError(ErrInvalidArgument, "dependency_conflicts", "conflicted rewind cannot be approved")
	}
	if p.ExpiresUnixNano <= now.UnixNano() {
		return NewError(ErrInvalidArgument, "expires_unix_nano", "plan is expired")
	}
	expected, err := p.CanonicalDigest()
	return validatePlanDigest(p.Digest, expected, err)
}

type RestorePlan struct {
	PlanID                   string        `json:"plan_id"`
	RuntimeCheckpointFactRef string        `json:"runtime_checkpoint_fact_ref"`
	ContinuityManifestRef    string        `json:"continuity_manifest_ref"`
	SourceInstanceID         string        `json:"source_instance_id"`
	SourceInstanceEpoch      uint64        `json:"source_instance_epoch"`
	NewInstanceID            string        `json:"new_instance_id"`
	NewInstanceEpoch         uint64        `json:"new_instance_epoch"`
	NewSandboxLeaseRef       string        `json:"new_sandbox_lease_ref"`
	RequiredParticipantIDs   []string      `json:"required_participant_ids"`
	CompatibilityFactRefs    []string      `json:"compatibility_fact_refs"`
	ContextMaterialized      bool          `json:"context_materialized"`
	ResidualRefs             []ResidualRef `json:"residual_refs"`
	RecoveryCredentialRef    string        `json:"recovery_credential_ref"`
	ExpiresUnixNano          int64         `json:"expires_unix_nano"`
	Digest                   string        `json:"digest"`
}

func (p RestorePlan) CanonicalDigest() (string, error) {
	copy := p
	copy.Digest = ""
	var err error
	copy.RequiredParticipantIDs, err = NormalizeSet(p.RequiredParticipantIDs)
	if err != nil {
		return "", err
	}
	copy.CompatibilityFactRefs, err = NormalizeSet(p.CompatibilityFactRefs)
	if err != nil {
		return "", err
	}
	copy.ResidualRefs, err = NormalizeResiduals(p.ResidualRefs)
	if err != nil {
		return "", err
	}
	return CanonicalDigest(copy)
}

func (p RestorePlan) Validate(now time.Time) error {
	for field, value := range map[string]string{
		"plan_id": p.PlanID, "runtime_checkpoint_fact_ref": p.RuntimeCheckpointFactRef,
		"continuity_manifest_ref": p.ContinuityManifestRef, "source_instance_id": p.SourceInstanceID,
		"new_instance_id": p.NewInstanceID, "new_sandbox_lease_ref": p.NewSandboxLeaseRef,
		"recovery_credential_ref": p.RecoveryCredentialRef,
	} {
		if err := ValidateToken(field, value); err != nil {
			return err
		}
	}
	if p.NewInstanceID == p.SourceInstanceID || p.SourceInstanceEpoch == 0 || p.NewInstanceEpoch <= p.SourceInstanceEpoch {
		return NewError(ErrRestoreIncompatible, "new_instance", "restore requires a new instance and higher epoch")
	}
	if len(p.RequiredParticipantIDs) == 0 || len(p.CompatibilityFactRefs) == 0 {
		return NewError(ErrRestoreIncompatible, "restore_inputs", "participants and compatibility facts are required")
	}
	for _, values := range [][]string{p.RequiredParticipantIDs, p.CompatibilityFactRefs} {
		if _, err := NormalizeSet(values); err != nil {
			return err
		}
	}
	for _, residual := range p.ResidualRefs {
		if err := residual.Validate(); err != nil {
			return err
		}
	}
	if !p.ContextMaterialized && len(p.ResidualRefs) == 0 {
		return NewError(ErrRestoreIncompatible, "context", "unmaterialized context must fail closed or carry a residual")
	}
	if p.ExpiresUnixNano <= now.UnixNano() {
		return NewError(ErrRestoreIncompatible, "expires_unix_nano", "plan is expired")
	}
	expected, err := p.CanonicalDigest()
	return validatePlanDigest(p.Digest, expected, err)
}

func validatePlanDigest(provided, expected string, err error) error {
	if err != nil {
		return err
	}
	if provided == "" || provided != expected {
		return NewError(ErrInvalidArgument, "plan_digest", "canonical digest mismatch")
	}
	return nil
}
