package ports

import (
	"cmp"
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	RestoreGovernanceContractVersionV2       = "2.0.0"
	RestoreAttemptTransportKindV2            = "praxis.runtime/restore-attempt"
	RestorePlanSubmittedStateV2              = "submitted"
	MaxRestoreGovernanceExternalRefsV2       = 512
	MaxRestoreEligibilityTTLUnixNanoV2 int64 = int64(5 * time.Minute)
)

type RestoreAttemptStateV2 string

const (
	RestoreAttemptReservedV2         RestoreAttemptStateV2 = "reserved"
	RestoreAttemptEligibilityBoundV2 RestoreAttemptStateV2 = "eligibility_bound"
	RestoreAttemptBegunV2            RestoreAttemptStateV2 = "begun"
	RestoreAttemptStagingV2          RestoreAttemptStateV2 = "staging"
	RestoreAttemptStagedV2           RestoreAttemptStateV2 = "staged"
	RestoreAttemptActivatedV2        RestoreAttemptStateV2 = "activated"
	RestoreAttemptAbortedV2          RestoreAttemptStateV2 = "aborted"
	RestoreAttemptIndeterminateV2    RestoreAttemptStateV2 = "indeterminate"
)

type RestoreEligibilityStateV2 string

const (
	RestoreEligibilityActiveV2     RestoreEligibilityStateV2 = "active"
	RestoreEligibilityRevokedV2    RestoreEligibilityStateV2 = "revoked"
	RestoreEligibilitySupersededV2 RestoreEligibilityStateV2 = "superseded"
	RestoreEligibilityExpiredV2    RestoreEligibilityStateV2 = "expired"
)

type RestoreAttemptRefV2 struct {
	TenantID core.TenantID `json:"tenant_id"`
	ID       string        `json:"attempt_id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r RestoreAttemptRefV2) Validate() error {
	if strings.TrimSpace(string(r.TenantID)) == "" || !validCheckpointIDV2(r.ID) || r.Revision == 0 || r.Digest.Validate() != nil {
		return restoreInvalidV2("Restore Attempt ref is incomplete")
	}
	return nil
}

type RestoreEligibilityRefV2 struct {
	TenantID        core.TenantID `json:"tenant_id"`
	ID              string        `json:"eligibility_id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r RestoreEligibilityRefV2) Validate() error {
	if strings.TrimSpace(string(r.TenantID)) == "" || !validCheckpointIDV2(r.ID) || r.Revision == 0 || r.ExpiresUnixNano <= 0 || r.Digest.Validate() != nil {
		return restoreInvalidV2("Restore Eligibility ref is incomplete")
	}
	return nil
}

type RestoreIdentityReservationV2 struct {
	SourceInstance   core.InstanceRef     `json:"source_instance"`
	TargetInstance   core.InstanceRef     `json:"target_instance"`
	TargetLease      core.SandboxLeaseRef `json:"target_sandbox_lease"`
	TargetFenceEpoch core.Epoch           `json:"target_fence_epoch"`
}

func (r RestoreIdentityReservationV2) Validate() error {
	if r.SourceInstance.Validate() != nil || r.TargetInstance.Validate() != nil || r.TargetLease.Validate() != nil || r.TargetFenceEpoch == 0 || r.TargetInstance.ID == r.SourceInstance.ID || r.TargetInstance.Epoch <= r.SourceInstance.Epoch {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRestoreIncompatible, "Restore reservation requires a fresh Instance, higher Epoch, new Lease, and Fence")
	}
	return nil
}

// CheckpointRestoreOperationScopeV2 is a coordinate only. The transport kind,
// Plan, Attempt, Instance, Lease, or Fence reservation grants no Eligibility,
// Review Authorization, Permit, Begin, Stage, or Provider access.
type CheckpointRestoreOperationScopeV2 struct {
	ContractVersion   string                           `json:"contract_version"`
	TransportKind     string                           `json:"transport_kind"`
	TenantID          core.TenantID                    `json:"tenant_id"`
	SourceScopeDigest core.Digest                      `json:"source_scope_digest"`
	RestorePlan       CheckpointExternalExactFactRefV2 `json:"restore_plan"`
	Consistency       CheckpointConsistencyRefV2       `json:"checkpoint_consistency"`
	AttemptID         string                           `json:"restore_attempt_id"`
	Identity          RestoreIdentityReservationV2     `json:"identity_reservation"`
	ConflictDomain    string                           `json:"conflict_domain"`
	Digest            core.Digest                      `json:"digest"`
}

func (s CheckpointRestoreOperationScopeV2) Validate() error {
	if s.ContractVersion != RestoreGovernanceContractVersionV2 || s.TransportKind != RestoreAttemptTransportKindV2 || strings.TrimSpace(string(s.TenantID)) == "" || s.SourceScopeDigest.Validate() != nil || s.RestorePlan.Validate() != nil || s.Consistency.Validate() != nil || !validCheckpointIDV2(s.AttemptID) || s.Identity.Validate() != nil || !validRestoreConflictDomainV2(s.ConflictDomain, s.TenantID) || s.Digest.Validate() != nil {
		return restoreInvalidV2("Checkpoint Restore Operation Scope is incomplete")
	}
	if s.RestorePlan.TenantID != string(s.TenantID) || s.RestorePlan.ScopeDigest != string(s.SourceScopeDigest) || s.Consistency.Attempt.TenantID != s.TenantID {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Checkpoint Restore Operation Scope crosses tenant or source scope")
	}
	digest, err := s.DigestV2()
	if err != nil || digest != s.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Checkpoint Restore Operation Scope digest drifted")
	}
	return nil
}

func (s CheckpointRestoreOperationScopeV2) DigestV2() (core.Digest, error) {
	copy := s
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-operation-scope", RestoreGovernanceContractVersionV2, "CheckpointRestoreOperationScopeV2", copy)
}

func SealCheckpointRestoreOperationScopeV2(s CheckpointRestoreOperationScopeV2) (CheckpointRestoreOperationScopeV2, error) {
	s.ContractVersion = RestoreGovernanceContractVersionV2
	s.TransportKind = RestoreAttemptTransportKindV2
	s.Digest = ""
	digest, err := s.DigestV2()
	if err != nil {
		return CheckpointRestoreOperationScopeV2{}, err
	}
	s.Digest = digest
	return s, s.Validate()
}

type RestoreAttemptFactV2 struct {
	ContractVersion   string                            `json:"contract_version"`
	Ref               RestoreAttemptRefV2               `json:"ref"`
	State             RestoreAttemptStateV2             `json:"state"`
	OperationScope    CheckpointRestoreOperationScopeV2 `json:"operation_scope"`
	IdempotencyKey    string                            `json:"idempotency_key"`
	Eligibility       *RestoreEligibilityRefV2          `json:"eligibility,omitempty"`
	RequestedNotAfter int64                             `json:"requested_not_after_unix_nano"`
	CreatedUnixNano   int64                             `json:"created_unix_nano"`
	UpdatedUnixNano   int64                             `json:"updated_unix_nano"`
}

func (f RestoreAttemptFactV2) Clone() RestoreAttemptFactV2 {
	if f.Eligibility != nil {
		value := *f.Eligibility
		f.Eligibility = &value
	}
	return f
}

func (f RestoreAttemptFactV2) Validate() error {
	if f.ContractVersion != RestoreGovernanceContractVersionV2 || f.Ref.Validate() != nil || f.OperationScope.Validate() != nil || !validCheckpointIDV2(f.IdempotencyKey) || f.RequestedNotAfter <= 0 || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano || f.Ref.TenantID != f.OperationScope.TenantID || f.Ref.ID != f.OperationScope.AttemptID {
		return restoreInvalidV2("Restore Attempt fact is incomplete")
	}
	switch f.State {
	case RestoreAttemptReservedV2:
		if f.Ref.Revision != 1 || f.Eligibility != nil {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "reserved Restore Attempt cannot carry Eligibility")
		}
	case RestoreAttemptEligibilityBoundV2:
		if f.Ref.Revision < 2 || f.Eligibility == nil || f.Eligibility.Validate() != nil || f.Eligibility.TenantID != f.Ref.TenantID {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Eligibility-bound Restore Attempt is incomplete")
		}
	case RestoreAttemptBegunV2, RestoreAttemptStagingV2, RestoreAttemptStagedV2, RestoreAttemptActivatedV2, RestoreAttemptAbortedV2, RestoreAttemptIndeterminateV2:
		if f.Eligibility == nil || f.Eligibility.Validate() != nil {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "progressed Restore Attempt lacks Eligibility history")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "Restore Attempt state is unsupported")
	}
	digest, err := f.DigestV2()
	if err != nil || digest != f.Ref.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Attempt digest drifted")
	}
	return nil
}

func (f RestoreAttemptFactV2) DigestV2() (core.Digest, error) {
	copy := f.Clone()
	copy.Ref.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.restore-attempt", RestoreGovernanceContractVersionV2, "RestoreAttemptFactV2", copy)
}

func SealRestoreAttemptFactV2(f RestoreAttemptFactV2) (RestoreAttemptFactV2, error) {
	f.ContractVersion = RestoreGovernanceContractVersionV2
	f.Ref.Digest = ""
	digest, err := f.DigestV2()
	if err != nil {
		return RestoreAttemptFactV2{}, err
	}
	f.Ref.Digest = digest
	return f, f.Validate()
}

// RestorePlanCurrentProjectionV2 is the neutral Runtime consumer projection
// returned by a Continuity-backed Adapter. It contains only exact refs and the
// Runtime identity proposal; it does not grant execution eligibility.
type RestorePlanCurrentProjectionV2 struct {
	ContractVersion              string                           `json:"contract_version"`
	RestorePlan                  CheckpointExternalExactFactRefV2 `json:"restore_plan"`
	State                        string                           `json:"state"`
	CheckpointConsistency        CheckpointConsistencyFactV2      `json:"checkpoint_consistency"`
	ManifestSeal                 CheckpointManifestSealRefV2      `json:"manifest_seal"`
	SourceScopeDigest            core.Digest                      `json:"source_scope_digest"`
	IdentityProposal             RestoreIdentityReservationV2     `json:"identity_proposal"`
	ConflictDomain               string                           `json:"conflict_domain"`
	RequiredParticipantSetDigest core.Digest                      `json:"required_participant_set_digest"`
	CheckedUnixNano              int64                            `json:"checked_unix_nano"`
	ExpiresUnixNano              int64                            `json:"expires_unix_nano"`
	ProjectionDigest             core.Digest                      `json:"projection_digest"`
}

func (p RestorePlanCurrentProjectionV2) Validate(now time.Time) error {
	if p.ContractVersion != RestoreGovernanceContractVersionV2 || p.RestorePlan.Validate() != nil || p.State != RestorePlanSubmittedStateV2 || p.CheckpointConsistency.Validate() != nil || p.ManifestSeal.Validate() != nil || p.SourceScopeDigest.Validate() != nil || p.IdentityProposal.Validate() != nil || !validRestoreConflictDomainV2(p.ConflictDomain, core.TenantID(p.RestorePlan.TenantID)) || p.RequiredParticipantSetDigest.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRestoreIncompatible, "Restore Plan current projection is incomplete or stale")
	}
	if p.RestorePlan.ScopeDigest != string(p.SourceScopeDigest) || p.CheckpointConsistency.Ref.Attempt.TenantID != core.TenantID(p.RestorePlan.TenantID) || p.ManifestSeal.Attempt.TenantID != core.TenantID(p.RestorePlan.TenantID) || p.ManifestSeal != p.CheckpointConsistency.ManifestSeal || p.RequiredParticipantSetDigest != p.CheckpointConsistency.ParticipantSetDigest {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Plan current projection exact closure drifted")
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Plan current projection digest drifted")
	}
	return nil
}

func (p RestorePlanCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.restore-plan-current", RestoreGovernanceContractVersionV2, "RestorePlanCurrentProjectionV2", copy)
}

func SealRestorePlanCurrentProjectionV2(p RestorePlanCurrentProjectionV2, now time.Time) (RestorePlanCurrentProjectionV2, error) {
	p.ContractVersion = RestoreGovernanceContractVersionV2
	p.ProjectionDigest = ""
	digest, err := p.DigestV2()
	if err != nil {
		return RestorePlanCurrentProjectionV2{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type RestoreEligibilityInputsCurrentProjectionV2 struct {
	ContractVersion          string                             `json:"contract_version"`
	Attempt                  RestoreAttemptRefV2                `json:"attempt"`
	OperationScopeDigest     core.Digest                        `json:"operation_scope_digest"`
	SourceScopeDigest        core.Digest                        `json:"source_scope_digest"`
	ReviewTarget             OperationReviewTargetRefV4         `json:"review_target"`
	ReviewRequirementRefs    []CheckpointExternalExactFactRefV2 `json:"review_requirement_refs"`
	PolicyBasisRefs          []CheckpointExternalExactFactRefV2 `json:"policy_basis_refs"`
	AuthorityRequirementRefs []CheckpointExternalExactFactRefV2 `json:"authority_refs"`
	ScopeRequirementRefs     []CheckpointExternalExactFactRefV2 `json:"scope_refs"`
	BudgetRequirementRefs    []CheckpointExternalExactFactRefV2 `json:"budget_refs"`
	BindingRequirementRefs   []CheckpointExternalExactFactRefV2 `json:"binding_refs"`
	ContextRequirementRefs   []CheckpointExternalExactFactRefV2 `json:"context_refs"`
	CheckedUnixNano          int64                              `json:"checked_unix_nano"`
	ExpiresUnixNano          int64                              `json:"expires_unix_nano"`
	ProjectionDigest         core.Digest                        `json:"projection_digest"`
}

func (p RestoreEligibilityInputsCurrentProjectionV2) Clone() RestoreEligibilityInputsCurrentProjectionV2 {
	p.ReviewRequirementRefs = append([]CheckpointExternalExactFactRefV2{}, p.ReviewRequirementRefs...)
	p.PolicyBasisRefs = append([]CheckpointExternalExactFactRefV2{}, p.PolicyBasisRefs...)
	p.AuthorityRequirementRefs = append([]CheckpointExternalExactFactRefV2{}, p.AuthorityRequirementRefs...)
	p.ScopeRequirementRefs = append([]CheckpointExternalExactFactRefV2{}, p.ScopeRequirementRefs...)
	p.BudgetRequirementRefs = append([]CheckpointExternalExactFactRefV2{}, p.BudgetRequirementRefs...)
	p.BindingRequirementRefs = append([]CheckpointExternalExactFactRefV2{}, p.BindingRequirementRefs...)
	p.ContextRequirementRefs = append([]CheckpointExternalExactFactRefV2{}, p.ContextRequirementRefs...)
	return p
}

func (p RestoreEligibilityInputsCurrentProjectionV2) Validate(now time.Time) error {
	if p.ContractVersion != RestoreGovernanceContractVersionV2 || p.Attempt.Validate() != nil || p.OperationScopeDigest.Validate() != nil || p.SourceScopeDigest.Validate() != nil || p.ReviewTarget.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRestoreIncompatible, "Restore Eligibility inputs are incomplete or stale")
	}
	sets := [][]CheckpointExternalExactFactRefV2{p.ReviewRequirementRefs, p.PolicyBasisRefs, p.AuthorityRequirementRefs, p.ScopeRequirementRefs, p.BudgetRequirementRefs, p.BindingRequirementRefs, p.ContextRequirementRefs}
	for _, values := range sets {
		if len(values) == 0 || len(values) > MaxRestoreGovernanceExternalRefsV2 || !restoreExternalRefsCanonicalV2(values) {
			return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Eligibility input set is empty or non-canonical")
		}
		for _, ref := range values {
			if ref.Validate() != nil || ref.TenantID != string(p.Attempt.TenantID) || ref.ScopeDigest != string(p.SourceScopeDigest) {
				return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Eligibility input ref crosses tenant")
			}
		}
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Eligibility input projection drifted")
	}
	return nil
}

func (p RestoreEligibilityInputsCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p.Clone()
	copy.ProjectionDigest = ""
	normalizeRestoreEligibilityInputRefsV2(&copy)
	return core.CanonicalJSONDigest("praxis.runtime.restore-eligibility-inputs-current", RestoreGovernanceContractVersionV2, "RestoreEligibilityInputsCurrentProjectionV2", copy)
}

func SealRestoreEligibilityInputsCurrentProjectionV2(p RestoreEligibilityInputsCurrentProjectionV2, now time.Time) (RestoreEligibilityInputsCurrentProjectionV2, error) {
	p = p.Clone()
	normalizeRestoreEligibilityInputRefsV2(&p)
	p.ContractVersion = RestoreGovernanceContractVersionV2
	p.ProjectionDigest = ""
	digest, err := p.DigestV2()
	if err != nil {
		return RestoreEligibilityInputsCurrentProjectionV2{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type RestoreEligibilityFactV2 struct {
	ContractVersion          string                             `json:"contract_version"`
	Ref                      RestoreEligibilityRefV2            `json:"ref"`
	State                    RestoreEligibilityStateV2          `json:"state"`
	Attempt                  RestoreAttemptRefV2                `json:"attempt"`
	OperationScopeDigest     core.Digest                        `json:"operation_scope_digest"`
	RestorePlan              CheckpointExternalExactFactRefV2   `json:"restore_plan"`
	CheckpointConsistency    CheckpointConsistencyRefV2         `json:"checkpoint_consistency"`
	Identity                 RestoreIdentityReservationV2       `json:"identity_reservation"`
	ReviewTarget             OperationReviewTargetRefV4         `json:"review_target"`
	ReviewRequirementRefs    []CheckpointExternalExactFactRefV2 `json:"review_requirement_refs"`
	PolicyBasisRefs          []CheckpointExternalExactFactRefV2 `json:"policy_basis_refs"`
	AuthorityRequirementRefs []CheckpointExternalExactFactRefV2 `json:"authority_refs"`
	ScopeRequirementRefs     []CheckpointExternalExactFactRefV2 `json:"scope_refs"`
	BudgetRequirementRefs    []CheckpointExternalExactFactRefV2 `json:"budget_refs"`
	BindingRequirementRefs   []CheckpointExternalExactFactRefV2 `json:"binding_refs"`
	ContextRequirementRefs   []CheckpointExternalExactFactRefV2 `json:"context_refs"`
	InputsProjectionDigest   core.Digest                        `json:"inputs_projection_digest"`
	CreatedUnixNano          int64                              `json:"created_unix_nano"`
	UpdatedUnixNano          int64                              `json:"updated_unix_nano"`
	InvalidationReason       core.ReasonCode                    `json:"invalidation_reason,omitempty"`
}

func (f RestoreEligibilityFactV2) Clone() RestoreEligibilityFactV2 {
	f.ReviewRequirementRefs = append([]CheckpointExternalExactFactRefV2{}, f.ReviewRequirementRefs...)
	f.PolicyBasisRefs = append([]CheckpointExternalExactFactRefV2{}, f.PolicyBasisRefs...)
	f.AuthorityRequirementRefs = append([]CheckpointExternalExactFactRefV2{}, f.AuthorityRequirementRefs...)
	f.ScopeRequirementRefs = append([]CheckpointExternalExactFactRefV2{}, f.ScopeRequirementRefs...)
	f.BudgetRequirementRefs = append([]CheckpointExternalExactFactRefV2{}, f.BudgetRequirementRefs...)
	f.BindingRequirementRefs = append([]CheckpointExternalExactFactRefV2{}, f.BindingRequirementRefs...)
	f.ContextRequirementRefs = append([]CheckpointExternalExactFactRefV2{}, f.ContextRequirementRefs...)
	return f
}

func (f RestoreEligibilityFactV2) Validate() error {
	if f.ContractVersion != RestoreGovernanceContractVersionV2 || f.Ref.Validate() != nil || f.Attempt.Validate() != nil || f.OperationScopeDigest.Validate() != nil || f.RestorePlan.Validate() != nil || f.CheckpointConsistency.Validate() != nil || f.Identity.Validate() != nil || f.ReviewTarget.Validate() != nil || f.InputsProjectionDigest.Validate() != nil || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano || f.Ref.TenantID != f.Attempt.TenantID || f.RestorePlan.TenantID != string(f.Attempt.TenantID) {
		return restoreInvalidV2("Restore Eligibility fact is incomplete")
	}
	sets := [][]CheckpointExternalExactFactRefV2{f.ReviewRequirementRefs, f.PolicyBasisRefs, f.AuthorityRequirementRefs, f.ScopeRequirementRefs, f.BudgetRequirementRefs, f.BindingRequirementRefs, f.ContextRequirementRefs}
	for _, values := range sets {
		if len(values) == 0 || len(values) > MaxRestoreGovernanceExternalRefsV2 || !restoreExternalRefsCanonicalV2(values) {
			return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Eligibility exact ref set is empty or non-canonical")
		}
		for _, ref := range values {
			if ref.Validate() != nil || ref.TenantID != string(f.Ref.TenantID) || ref.ScopeDigest != f.RestorePlan.ScopeDigest {
				return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Eligibility exact ref crosses tenant or source scope")
			}
		}
	}
	switch f.State {
	case RestoreEligibilityActiveV2:
		if f.InvalidationReason != "" || f.Ref.Revision != 1 {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "active Restore Eligibility has invalid history")
		}
	case RestoreEligibilityRevokedV2, RestoreEligibilitySupersededV2, RestoreEligibilityExpiredV2:
		if f.InvalidationReason == "" || f.Ref.Revision < 2 {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "inactive Restore Eligibility lacks reason or revision")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "Restore Eligibility state is unsupported")
	}
	digest, err := f.DigestV2()
	if err != nil || digest != f.Ref.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Eligibility digest drifted")
	}
	return nil
}

func (f RestoreEligibilityFactV2) ValidateCurrent(now time.Time) error {
	if err := f.Validate(); err != nil {
		return err
	}
	if f.State != RestoreEligibilityActiveV2 || now.IsZero() || now.UnixNano() < f.CreatedUnixNano || !now.Before(time.Unix(0, f.Ref.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Restore Eligibility is not active current")
	}
	return nil
}

func (f RestoreEligibilityFactV2) DigestV2() (core.Digest, error) {
	copy := f.Clone()
	copy.Ref.Digest = ""
	normalizeRestoreEligibilityFactRefsV2(&copy)
	return core.CanonicalJSONDigest("praxis.runtime.restore-eligibility", RestoreGovernanceContractVersionV2, "RestoreEligibilityFactV2", copy)
}

func SealRestoreEligibilityFactV2(f RestoreEligibilityFactV2) (RestoreEligibilityFactV2, error) {
	f = f.Clone()
	normalizeRestoreEligibilityFactRefsV2(&f)
	f.ContractVersion = RestoreGovernanceContractVersionV2
	f.Ref.Digest = ""
	digest, err := f.DigestV2()
	if err != nil {
		return RestoreEligibilityFactV2{}, err
	}
	f.Ref.Digest = digest
	return f, f.Validate()
}

type CreateRestoreAttemptRequestV2 struct {
	AttemptID         string                           `json:"attempt_id"`
	IdempotencyKey    string                           `json:"idempotency_key"`
	RestorePlan       CheckpointExternalExactFactRefV2 `json:"restore_plan"`
	RequestedNotAfter int64                            `json:"requested_not_after_unix_nano"`
}

func (r CreateRestoreAttemptRequestV2) Validate() error {
	if !validCheckpointIDV2(r.AttemptID) || !validCheckpointIDV2(r.IdempotencyKey) || r.RestorePlan.Validate() != nil || r.RequestedNotAfter <= 0 {
		return restoreInvalidV2("Create Restore Attempt request is incomplete")
	}
	return nil
}

type InspectRestoreAttemptRequestV2 struct {
	TenantID  core.TenantID `json:"tenant_id"`
	AttemptID string        `json:"attempt_id"`
}

func (r InspectRestoreAttemptRequestV2) Validate() error {
	if strings.TrimSpace(string(r.TenantID)) == "" || !validCheckpointIDV2(r.AttemptID) {
		return restoreInvalidV2("Inspect Restore Attempt request is incomplete")
	}
	return nil
}

type IssueRestoreEligibilityRequestV2 struct {
	EligibilityID string              `json:"eligibility_id"`
	Attempt       RestoreAttemptRefV2 `json:"attempt"`
	RequestedTTL  time.Duration       `json:"requested_ttl"`
}

func (r IssueRestoreEligibilityRequestV2) Validate() error {
	if !validCheckpointIDV2(r.EligibilityID) || r.Attempt.Validate() != nil || r.RequestedTTL <= 0 || int64(r.RequestedTTL) > MaxRestoreEligibilityTTLUnixNanoV2 {
		return restoreInvalidV2("Issue Restore Eligibility request is incomplete or unbounded")
	}
	return nil
}

type InspectRestoreEligibilityCurrentRequestV2 struct {
	Attempt             RestoreAttemptRefV2     `json:"attempt"`
	ExpectedEligibility RestoreEligibilityRefV2 `json:"expected_eligibility"`
}

func (r InspectRestoreEligibilityCurrentRequestV2) Validate() error {
	if r.Attempt.Validate() != nil || r.ExpectedEligibility.Validate() != nil || r.Attempt.TenantID != r.ExpectedEligibility.TenantID {
		return restoreInvalidV2("Inspect current Restore Eligibility request is incomplete")
	}
	return nil
}

type RestoreEligibilityBindBundleV2 struct {
	Attempt     RestoreAttemptFactV2     `json:"attempt"`
	Eligibility RestoreEligibilityFactV2 `json:"eligibility"`
}

func (b RestoreEligibilityBindBundleV2) Validate() error {
	if b.Attempt.Validate() != nil || b.Eligibility.Validate() != nil || b.Attempt.State != RestoreAttemptEligibilityBoundV2 || b.Attempt.Eligibility == nil || *b.Attempt.Eligibility != b.Eligibility.Ref || b.Eligibility.Attempt.Revision+1 != b.Attempt.Ref.Revision || b.Eligibility.Attempt.ID != b.Attempt.Ref.ID || b.Eligibility.Attempt.TenantID != b.Attempt.Ref.TenantID || b.Eligibility.OperationScopeDigest != b.Attempt.OperationScope.Digest || b.Eligibility.RestorePlan != b.Attempt.OperationScope.RestorePlan || b.Eligibility.CheckpointConsistency != b.Attempt.OperationScope.Consistency || b.Eligibility.Identity != b.Attempt.OperationScope.Identity {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Eligibility bind bundle is not exact")
	}
	return nil
}

type RestoreAttemptCommitRequestV2 struct {
	Candidate RestoreAttemptFactV2 `json:"candidate"`
}

type RestoreEligibilityBindCommitRequestV2 struct {
	ExpectedAttempt RestoreAttemptRefV2            `json:"expected_attempt"`
	Bundle          RestoreEligibilityBindBundleV2 `json:"bundle"`
}

type RestoreEligibilityCASRequestV2 struct {
	Expected RestoreEligibilityRefV2  `json:"expected"`
	Next     RestoreEligibilityFactV2 `json:"next"`
}

type RestorePlanCurrentReaderV2 interface {
	InspectRestorePlanCurrentV2(context.Context, CheckpointExternalExactFactRefV2) (RestorePlanCurrentProjectionV2, error)
}

type RestoreEligibilityInputsCurrentReaderV2 interface {
	InspectRestoreEligibilityInputsCurrentV2(context.Context, RestoreAttemptFactV2) (RestoreEligibilityInputsCurrentProjectionV2, error)
}

type RestoreGovernanceFactPortV2 interface {
	CreateRestoreAttemptV2(context.Context, RestoreAttemptFactV2) (RestoreAttemptFactV2, error)
	InspectRestoreAttemptCurrentV2(context.Context, InspectRestoreAttemptRequestV2) (RestoreAttemptFactV2, error)
	InspectRestoreAttemptHistoricalV2(context.Context, RestoreAttemptRefV2) (RestoreAttemptFactV2, error)
	BindRestoreEligibilityV2(context.Context, RestoreEligibilityBindCommitRequestV2) (RestoreEligibilityBindBundleV2, error)
	InspectRestoreEligibilityHistoricalV2(context.Context, RestoreEligibilityRefV2) (RestoreEligibilityFactV2, error)
	InspectRestoreEligibilityCurrentV2(context.Context, InspectRestoreEligibilityCurrentRequestV2) (RestoreEligibilityFactV2, error)
	CompareAndSwapRestoreEligibilityV2(context.Context, RestoreEligibilityCASRequestV2) (RestoreEligibilityFactV2, error)
}

// RestoreGovernancePortV2 deliberately stops at Attempt reservation and
// short-TTL Eligibility. Action Admission, Review Authorization, Permit,
// Begin, Stage, Evidence/Settlement, Activate, Abort, and Provider execution
// remain separate public contracts and are not implied by this interface.
type RestoreGovernancePortV2 interface {
	CreateRestoreAttemptV2(context.Context, CreateRestoreAttemptRequestV2) (RestoreAttemptFactV2, error)
	InspectRestoreAttemptV2(context.Context, InspectRestoreAttemptRequestV2) (RestoreAttemptFactV2, error)
	InspectRestoreAttemptHistoricalV2(context.Context, RestoreAttemptRefV2) (RestoreAttemptFactV2, error)
	IssueRestoreEligibilityV2(context.Context, IssueRestoreEligibilityRequestV2) (RestoreEligibilityBindBundleV2, error)
	InspectRestoreEligibilityV2(context.Context, RestoreEligibilityRefV2) (RestoreEligibilityFactV2, error)
	InspectCurrentRestoreEligibilityV2(context.Context, InspectRestoreEligibilityCurrentRequestV2) (RestoreEligibilityFactV2, error)
	CompareAndSwapRestoreEligibilityV2(context.Context, RestoreEligibilityCASRequestV2) (RestoreEligibilityFactV2, error)
}

func restoreInvalidV2(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonRestoreIncompatible, message)
}

func validRestoreConflictDomainV2(value string, tenant core.TenantID) bool {
	return strings.HasPrefix(value, "tenant/"+string(tenant)+"/") && validCheckpointIDV2(value)
}

func normalizeRestoreEligibilityInputRefsV2(p *RestoreEligibilityInputsCurrentProjectionV2) {
	sets := []*[]CheckpointExternalExactFactRefV2{&p.ReviewRequirementRefs, &p.PolicyBasisRefs, &p.AuthorityRequirementRefs, &p.ScopeRequirementRefs, &p.BudgetRequirementRefs, &p.BindingRequirementRefs, &p.ContextRequirementRefs}
	for _, values := range sets {
		*values = append([]CheckpointExternalExactFactRefV2{}, (*values)...)
		sort.Slice(*values, func(i, j int) bool { return compareRestoreExternalRefV2((*values)[i], (*values)[j]) < 0 })
	}
}

func normalizeRestoreEligibilityFactRefsV2(f *RestoreEligibilityFactV2) {
	projection := RestoreEligibilityInputsCurrentProjectionV2{
		ReviewRequirementRefs: f.ReviewRequirementRefs, PolicyBasisRefs: f.PolicyBasisRefs,
		AuthorityRequirementRefs: f.AuthorityRequirementRefs, ScopeRequirementRefs: f.ScopeRequirementRefs, BudgetRequirementRefs: f.BudgetRequirementRefs,
		BindingRequirementRefs: f.BindingRequirementRefs, ContextRequirementRefs: f.ContextRequirementRefs,
	}
	normalizeRestoreEligibilityInputRefsV2(&projection)
	f.ReviewRequirementRefs = projection.ReviewRequirementRefs
	f.PolicyBasisRefs = projection.PolicyBasisRefs
	f.AuthorityRequirementRefs = projection.AuthorityRequirementRefs
	f.ScopeRequirementRefs = projection.ScopeRequirementRefs
	f.BudgetRequirementRefs = projection.BudgetRequirementRefs
	f.BindingRequirementRefs = projection.BindingRequirementRefs
	f.ContextRequirementRefs = projection.ContextRequirementRefs
}

func restoreExternalRefsCanonicalV2(values []CheckpointExternalExactFactRefV2) bool {
	for index := range values {
		if index > 0 && compareRestoreExternalRefV2(values[index-1], values[index]) >= 0 {
			return false
		}
	}
	return true
}

func compareRestoreExternalRefV2(a, b CheckpointExternalExactFactRefV2) int {
	for _, pair := range [][2]string{
		{a.TenantID, b.TenantID}, {a.ScopeDigest, b.ScopeDigest},
		{a.Owner.BindingSetID, b.Owner.BindingSetID}, {a.Owner.ComponentID, b.Owner.ComponentID},
		{a.Owner.ManifestDigest, b.Owner.ManifestDigest}, {a.Owner.ArtifactDigest, b.Owner.ArtifactDigest},
		{a.Owner.Capability, b.Owner.Capability}, {a.Owner.FactKind, b.Owner.FactKind},
		{a.ContractVersion, b.ContractVersion}, {a.SchemaRef, b.SchemaRef}, {a.ID, b.ID}, {a.Digest, b.Digest},
	} {
		if result := cmp.Compare(pair[0], pair[1]); result != 0 {
			return result
		}
	}
	if result := cmp.Compare(a.Owner.BindingRevision, b.Owner.BindingRevision); result != 0 {
		return result
	}
	return cmp.Compare(a.Revision, b.Revision)
}
