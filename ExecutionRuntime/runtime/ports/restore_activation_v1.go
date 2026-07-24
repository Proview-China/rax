package ports

import (
	"context"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const RestoreActivationContractVersionV1 = "1.0.0"

// RestoreStageApplySettlementRefV1 is Runtime's neutral exact view of the
// Sandbox-owned ApplySettlement fact. It carries no copied Sandbox state.
type RestoreStageApplySettlementRefV1 struct {
	Owner             ProviderBindingRefV2              `json:"owner"`
	ID                string                            `json:"id"`
	Revision          core.Revision                     `json:"revision"`
	Digest            core.Digest                       `json:"digest"`
	TenantID          core.TenantID                     `json:"tenant_id"`
	DomainResult      RestoreStageDomainResultFactRefV1 `json:"domain_result"`
	RuntimeSettlement RestoreStageSettlementRefV1       `json:"runtime_settlement"`
}

// SameRestoreStageApplySettlementRefV1 compares the public exact identity of
// Sandbox's ApplySettlement fact without relying on Go pointer identity inside
// the embedded Runtime settlement ExecutionScope.
func SameRestoreStageApplySettlementRefV1(left, right RestoreStageApplySettlementRefV1) bool {
	return left.Owner == right.Owner &&
		left.ID == right.ID &&
		left.Revision == right.Revision &&
		left.Digest == right.Digest &&
		left.TenantID == right.TenantID &&
		SameRestoreStageDomainResultFactRefV1(left.DomainResult, right.DomainResult) &&
		SameRestoreStageSettlementRefV1(left.RuntimeSettlement, right.RuntimeSettlement)
}

func (r RestoreStageApplySettlementRefV1) Validate() error {
	if r.Owner.Validate() != nil || validateEvidenceIDV2(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil || validateEvidenceIDV2(string(r.TenantID)) != nil || r.DomainResult.Validate() != nil || r.RuntimeSettlement.Validate() != nil || r.DomainResult.TenantID != r.TenantID || !SameRestoreStageDomainResultFactRefV1(r.DomainResult, r.RuntimeSettlement.DomainResult) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonSettlementOwnerMismatch, "Restore Stage ApplySettlement ref is incomplete")
	}
	return nil
}

type RestoreStageApplySettlementCurrentProjectionV1 struct {
	ContractVersion  string                           `json:"contract_version"`
	Fact             RestoreStageApplySettlementRefV1 `json:"fact"`
	CheckedUnixNano  int64                            `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                            `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                      `json:"projection_digest"`
}

func (p RestoreStageApplySettlementCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.restore-stage-apply-settlement-current", RestoreActivationContractVersionV1, "RestoreStageApplySettlementCurrentProjectionV1", copy)
}

func SealRestoreStageApplySettlementCurrentProjectionV1(p RestoreStageApplySettlementCurrentProjectionV1, now time.Time) (RestoreStageApplySettlementCurrentProjectionV1, error) {
	p.ContractVersion = RestoreActivationContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return RestoreStageApplySettlementCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.ValidateCurrent(now)
}

func (p RestoreStageApplySettlementCurrentProjectionV1) ValidateCurrent(now time.Time) error {
	if p.ContractVersion != RestoreActivationContractVersionV1 || p.Fact.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || now.UnixNano() >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonSettlementOwnerMismatch, "Restore Stage ApplySettlement projection is incomplete or stale")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Stage ApplySettlement projection drifted")
	}
	return nil
}

type RestoreStageApplySettlementCurrentReaderV1 interface {
	InspectRestoreStageApplySettlementCurrentV1(context.Context, RestoreStageApplySettlementRefV1) (RestoreStageApplySettlementCurrentProjectionV1, error)
}

// RestoreContextMaterializationRefV1 is a neutral exact ref to the Context
// Owner's authoritative new Generation/current publication. Runtime does not
// interpret Context payloads or create Context facts.
type RestoreContextMaterializationRefV1 struct {
	Owner             ProviderBindingRefV2               `json:"owner"`
	ID                string                             `json:"id"`
	Revision          core.Revision                      `json:"revision"`
	Digest            core.Digest                        `json:"digest"`
	TenantID          core.TenantID                      `json:"tenant_id"`
	Attempt           RestoreAttemptRefV2                `json:"restore_attempt"`
	Eligibility       RestoreEligibilityRefV2            `json:"restore_eligibility"`
	Identity          RestoreIdentityReservationV2       `json:"identity_reservation"`
	SourceScopeDigest core.Digest                        `json:"source_scope_digest"`
	TargetScopeDigest core.Digest                        `json:"target_scope_digest"`
	SourceGeneration  CheckpointExternalExactFactRefV2   `json:"source_generation"`
	TargetGeneration  CheckpointExternalExactFactRefV2   `json:"target_generation"`
	TargetFrames      []CheckpointExternalExactFactRefV2 `json:"target_frames"`
	CurrentDigest     core.Digest                        `json:"current_digest"`
}

func (r RestoreContextMaterializationRefV1) Clone() RestoreContextMaterializationRefV1 {
	r.TargetFrames = append([]CheckpointExternalExactFactRefV2{}, r.TargetFrames...)
	return r
}

func (r RestoreContextMaterializationRefV1) Validate() error {
	if r.Owner.Validate() != nil || validateEvidenceIDV2(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil || validateEvidenceIDV2(string(r.TenantID)) != nil || r.Attempt.Validate() != nil || r.Eligibility.Validate() != nil || r.Identity.Validate() != nil || r.SourceScopeDigest.Validate() != nil || r.TargetScopeDigest.Validate() != nil || r.SourceGeneration.Validate() != nil || r.TargetGeneration.Validate() != nil || r.CurrentDigest.Validate() != nil || len(r.TargetFrames) == 0 || len(r.TargetFrames) > MaxRestoreGovernanceExternalRefsV2 || r.Attempt.TenantID != r.TenantID || r.Eligibility.TenantID != r.TenantID || r.SourceGeneration.TenantID != string(r.TenantID) || r.SourceGeneration.ScopeDigest != string(r.SourceScopeDigest) || r.TargetGeneration.TenantID != string(r.TenantID) || r.TargetGeneration.ScopeDigest != string(r.TargetScopeDigest) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRestoreIncompatible, "Restore Context materialization ref is incomplete")
	}
	for index, ref := range r.TargetFrames {
		if ref.Validate() != nil || ref.TenantID != string(r.TenantID) || ref.ScopeDigest != string(r.TargetScopeDigest) || index > 0 && compareRestoreExternalRefV2(r.TargetFrames[index-1], ref) >= 0 {
			return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Context target Frame crosses tenant or target scope")
		}
	}
	return nil
}

type RestoreContextMaterializationCurrentProjectionV1 struct {
	ContractVersion  string                             `json:"contract_version"`
	Fact             RestoreContextMaterializationRefV1 `json:"fact"`
	Residuals        []CheckpointExternalExactFactRefV2 `json:"residuals"`
	CheckedUnixNano  int64                              `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                              `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                        `json:"projection_digest"`
}

func (p RestoreContextMaterializationCurrentProjectionV1) Clone() RestoreContextMaterializationCurrentProjectionV1 {
	p.Fact = p.Fact.Clone()
	p.Residuals = append([]CheckpointExternalExactFactRefV2{}, p.Residuals...)
	return p
}

func (p RestoreContextMaterializationCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p.Clone()
	copy.ProjectionDigest = ""
	sort.Slice(copy.Residuals, func(i, j int) bool { return compareRestoreExternalRefV2(copy.Residuals[i], copy.Residuals[j]) < 0 })
	return core.CanonicalJSONDigest("praxis.runtime.restore-context-materialization-current", RestoreActivationContractVersionV1, "RestoreContextMaterializationCurrentProjectionV1", copy)
}

func SealRestoreContextMaterializationCurrentProjectionV1(p RestoreContextMaterializationCurrentProjectionV1, now time.Time) (RestoreContextMaterializationCurrentProjectionV1, error) {
	p = p.Clone()
	sort.Slice(p.Residuals, func(i, j int) bool { return compareRestoreExternalRefV2(p.Residuals[i], p.Residuals[j]) < 0 })
	p.ContractVersion = RestoreActivationContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return RestoreContextMaterializationCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.ValidateCurrent(now)
}

func (p RestoreContextMaterializationCurrentProjectionV1) ValidateCurrent(now time.Time) error {
	if p.ContractVersion != RestoreActivationContractVersionV1 || p.Fact.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || now.UnixNano() >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRestoreIncompatible, "Restore Context materialization projection is incomplete or stale")
	}
	for index, residual := range p.Residuals {
		if residual.Validate() != nil || residual.TenantID != string(p.Fact.TenantID) || index > 0 && compareRestoreExternalRefV2(p.Residuals[index-1], residual) >= 0 {
			return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Context residual set is invalid")
		}
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Context materialization projection drifted")
	}
	return nil
}

type RestoreContextMaterializationCurrentReaderV1 interface {
	InspectRestoreContextMaterializationCurrentV1(context.Context, RestoreContextMaterializationRefV1) (RestoreContextMaterializationCurrentProjectionV1, error)
}

type RestoreActivationSubmissionV1 struct {
	Attempt           RestoreAttemptRefV2                `json:"restore_attempt"`
	Eligibility       RestoreEligibilityRefV2            `json:"restore_eligibility"`
	Stage             RestoreStageDomainResultFactRefV1  `json:"stage_domain_result"`
	RuntimeSettlement RestoreStageSettlementRefV1        `json:"runtime_settlement"`
	SandboxSettlement RestoreStageApplySettlementRefV1   `json:"sandbox_apply_settlement"`
	Context           RestoreContextMaterializationRefV1 `json:"context_materialization"`
	IdempotencyKey    string                             `json:"idempotency_key"`
}

func (s RestoreActivationSubmissionV1) Validate() error {
	if s.Attempt.Validate() != nil || s.Eligibility.Validate() != nil || s.Stage.Validate() != nil || s.RuntimeSettlement.Validate() != nil || s.SandboxSettlement.Validate() != nil || s.Context.Validate() != nil || validateEvidenceIDV2(s.IdempotencyKey) != nil || s.Attempt.TenantID != s.Eligibility.TenantID || s.Stage.RestoreAttempt != s.Attempt || s.Stage.Eligibility != s.Eligibility || !SameRestoreStageDomainResultFactRefV1(s.Stage, s.RuntimeSettlement.DomainResult) || !SameRestoreStageDomainResultFactRefV1(s.Stage, s.SandboxSettlement.DomainResult) || !SameRestoreStageSettlementRefV1(s.SandboxSettlement.RuntimeSettlement, s.RuntimeSettlement) || s.Context.Attempt != s.Attempt || s.Context.Eligibility != s.Eligibility || s.Context.Identity.TargetInstance != s.Stage.Operation.ExecutionScope.Instance || s.Stage.Operation.ExecutionScope.SandboxLease == nil || s.Context.Identity.TargetLease != *s.Stage.Operation.ExecutionScope.SandboxLease || s.Context.Identity.TargetFenceEpoch != s.Stage.Operation.ExecutionScope.AuthorityEpoch || s.Context.TargetScopeDigest != s.Stage.Operation.ExecutionScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Activation submission exact closure drifted")
	}
	return nil
}

type RestoreActivationRefV1 struct {
	ID       string              `json:"id"`
	Revision core.Revision       `json:"revision"`
	Digest   core.Digest         `json:"digest"`
	Attempt  RestoreAttemptRefV2 `json:"restore_attempt"`
}

func (r RestoreActivationRefV1) Validate() error {
	if validateEvidenceIDV2(r.ID) != nil || r.Revision != 1 || r.Digest.Validate() != nil || r.Attempt.Validate() != nil {
		return restoreInvalidV2("Restore Activation ref is incomplete")
	}
	return nil
}

type RestoreActivationFactV1 struct {
	ContractVersion   string                        `json:"contract_version"`
	Ref               RestoreActivationRefV1        `json:"ref"`
	Submission        RestoreActivationSubmissionV1 `json:"submission"`
	Identity          RestoreIdentityReservationV2  `json:"identity_reservation"`
	ActivatedUnixNano int64                         `json:"activated_unix_nano"`
}

func (f RestoreActivationFactV1) DigestV1() (core.Digest, error) {
	copy := f
	copy.Ref.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.restore-activation", RestoreActivationContractVersionV1, "RestoreActivationFactV1", copy)
}

func SealRestoreActivationFactV1(f RestoreActivationFactV1) (RestoreActivationFactV1, error) {
	f.ContractVersion = RestoreActivationContractVersionV1
	f.Ref.Revision = 1
	f.Ref.Digest = ""
	digest, err := f.DigestV1()
	if err != nil {
		return RestoreActivationFactV1{}, err
	}
	f.Ref.Digest = digest
	return f, f.Validate()
}

func (f RestoreActivationFactV1) Validate() error {
	if f.ContractVersion != RestoreActivationContractVersionV1 || f.Ref.Validate() != nil || f.Submission.Validate() != nil || f.Identity.Validate() != nil || f.ActivatedUnixNano <= 0 || f.Ref.Attempt.TenantID != f.Submission.Attempt.TenantID || f.Ref.Attempt.ID != f.Submission.Attempt.ID || f.Ref.Attempt.Revision != f.Submission.Attempt.Revision+1 || f.Identity != f.ContextIdentityV1() {
		return restoreInvalidV2("Restore Activation fact is incomplete")
	}
	digest, err := f.DigestV1()
	if err != nil || digest != f.Ref.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Activation fact drifted")
	}
	return nil
}

func (f RestoreActivationFactV1) ContextIdentityV1() RestoreIdentityReservationV2 {
	return f.Submission.Context.Identity
}

type RestoreActivationCommitRequestV1 struct {
	ExpectedAttempt RestoreAttemptRefV2     `json:"expected_attempt"`
	NextAttempt     RestoreAttemptFactV2    `json:"next_attempt"`
	Activation      RestoreActivationFactV1 `json:"activation"`
}

type RestoreActivationFactPortV1 interface {
	CommitRestoreActivationV1(context.Context, RestoreActivationCommitRequestV1) (RestoreActivationFactV1, error)
	InspectRestoreActivationV1(context.Context, RestoreActivationRefV1) (RestoreActivationFactV1, error)
	InspectRestoreActivationByAttemptV1(context.Context, RestoreAttemptRefV2) (RestoreActivationFactV1, error)
	InspectRestoreActivationByStableAttemptV1(context.Context, core.TenantID, string) (RestoreActivationFactV1, error)
}

type RestoreActivationGovernancePortV1 interface {
	ActivateRestoreV1(context.Context, RestoreActivationSubmissionV1) (RestoreActivationRefV1, error)
	InspectRestoreActivationV1(context.Context, RestoreActivationRefV1) (RestoreActivationFactV1, error)
	InspectRestoreActivationByAttemptV1(context.Context, RestoreAttemptRefV2) (RestoreActivationFactV1, error)
	// InspectRestoreActivationByStableAttemptV1 is the lost-reply recovery
	// reader used when the caller knows the original Attempt identity but cannot
	// derive the Runtime-owned successor digest. It never creates or advances
	// an Attempt.
	InspectRestoreActivationByStableAttemptV1(context.Context, core.TenantID, string) (RestoreActivationFactV1, error)
}
