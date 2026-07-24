package contract

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const RestoreExecutionContractVersionV1 = "praxis.application/restore-execution/v1"

type RestoreExecutionRequestV1 struct {
	ContractVersion          string                                        `json:"contract_version"`
	ID                       string                                        `json:"id"`
	IdempotencyKey           string                                        `json:"idempotency_key"`
	RestorePlan              runtimeports.CheckpointExternalExactFactRefV2 `json:"restore_plan"`
	RestoreAttemptID         string                                        `json:"restore_attempt_id"`
	RestoreEligibilityID     string                                        `json:"restore_eligibility_id"`
	StageActionID            string                                        `json:"stage_action_id"`
	StageIdempotencyKey      string                                        `json:"stage_idempotency_key"`
	ContextID                string                                        `json:"context_materialization_id"`
	ContextIdempotencyKey    string                                        `json:"context_idempotency_key"`
	ActivationIdempotencyKey string                                        `json:"activation_idempotency_key"`
	Requirements             []RestoreContextRequirementCoordinateV1       `json:"context_requirements"`
	EligibilityTTL           time.Duration                                 `json:"eligibility_ttl"`
	RequestedUnixNano        int64                                         `json:"requested_unix_nano"`
	NotAfterUnixNano         int64                                         `json:"not_after_unix_nano"`
	Digest                   core.Digest                                   `json:"digest"`
}

// RestoreExecutionIntentFactV1 is the Application Owner's immutable
// create-once boundary. It must be durable before Runtime may reserve a fresh
// RestoreAttempt/Instance/Lease. It grants no Admission, Permit, Fence or
// Provider authority.
type RestoreExecutionIntentFactV1 struct {
	ContractVersion string                    `json:"contract_version"`
	TenantID        core.TenantID             `json:"tenant_id"`
	ID              string                    `json:"id"`
	Revision        core.Revision             `json:"revision"`
	Request         RestoreExecutionRequestV1 `json:"request"`
	RequestDigest   core.Digest               `json:"request_digest"`
	CreatedUnixNano int64                     `json:"created_unix_nano"`
	Digest          core.Digest               `json:"digest"`
}

func (f RestoreExecutionIntentFactV1) Clone() RestoreExecutionIntentFactV1 {
	f.Request = f.Request.Clone()
	return f
}

func (f RestoreExecutionIntentFactV1) DigestV1() (core.Digest, error) {
	copy := f.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.restore-execution", RestoreExecutionContractVersionV1, "RestoreExecutionIntentFactV1", copy)
}

func SealRestoreExecutionIntentFactV1(request RestoreExecutionRequestV1, now time.Time) (RestoreExecutionIntentFactV1, error) {
	fact := RestoreExecutionIntentFactV1{
		ContractVersion: RestoreExecutionContractVersionV1,
		TenantID:        core.TenantID(request.RestorePlan.TenantID),
		ID:              request.ID,
		Revision:        1,
		Request:         request.Clone(),
		RequestDigest:   request.Digest,
		CreatedUnixNano: now.UnixNano(),
	}
	digest, err := fact.DigestV1()
	if err != nil {
		return RestoreExecutionIntentFactV1{}, err
	}
	fact.Digest = digest
	return fact, fact.ValidateCurrent(now)
}

func (f RestoreExecutionIntentFactV1) ValidateCurrent(now time.Time) error {
	if f.ContractVersion != RestoreExecutionContractVersionV1 || f.TenantID == "" || !validSingleCallIDV1(f.ID) || f.Revision != 1 || f.CreatedUnixNano <= 0 || f.RequestDigest.Validate() != nil || f.Digest.Validate() != nil || f.Request.ID != f.ID || core.TenantID(f.Request.RestorePlan.TenantID) != f.TenantID || f.Request.Digest != f.RequestDigest {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore execution Intent is incomplete or crosses its request")
	}
	if err := f.Request.ValidateCurrent(now); err != nil {
		return err
	}
	digest, err := f.DigestV1()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore execution Intent digest drifted")
	}
	return nil
}

func (r RestoreExecutionRequestV1) Clone() RestoreExecutionRequestV1 {
	r.Requirements = append([]RestoreContextRequirementCoordinateV1{}, r.Requirements...)
	return r
}

func (r RestoreExecutionRequestV1) DigestV1() (core.Digest, error) {
	copy := r.Clone()
	copy.Digest = ""
	sortRestoreContextRequirementRefsV1(copy.Requirements)
	return core.CanonicalJSONDigest("praxis.application.restore-execution", RestoreExecutionContractVersionV1, "RestoreExecutionRequestV1", copy)
}

func SealRestoreExecutionRequestV1(r RestoreExecutionRequestV1) (RestoreExecutionRequestV1, error) {
	r = r.Clone()
	r.ContractVersion = RestoreExecutionContractVersionV1
	sortRestoreContextRequirementRefsV1(r.Requirements)
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return RestoreExecutionRequestV1{}, err
	}
	r.Digest = digest
	return r, r.ValidateCurrent(time.Unix(0, r.RequestedUnixNano))
}

func (r RestoreExecutionRequestV1) ValidateCurrent(now time.Time) error {
	ids := []string{r.ID, r.IdempotencyKey, r.RestoreAttemptID, r.RestoreEligibilityID, r.StageActionID, r.StageIdempotencyKey, r.ContextID, r.ContextIdempotencyKey, r.ActivationIdempotencyKey}
	if r.ContractVersion != RestoreExecutionContractVersionV1 || r.RestorePlan.Validate() != nil || r.EligibilityTTL <= 0 || r.EligibilityTTL > time.Duration(runtimeports.MaxRestoreEligibilityTTLUnixNanoV2) || r.RequestedUnixNano <= 0 || r.NotAfterUnixNano <= r.RequestedUnixNano || now.IsZero() || now.UnixNano() < r.RequestedUnixNano || now.UnixNano() >= r.NotAfterUnixNano || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRestoreIncompatible, "Restore execution request is incomplete or stale")
	}
	for _, id := range ids {
		if !validSingleCallIDV1(id) {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonRestoreIncompatible, "Restore execution identities are invalid")
		}
	}
	if len(r.Requirements) < 7 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRestoreIncompatible, "Restore execution requires all Context current routes")
	}
	kinds := make(map[RestoreContextRequirementKindV1]struct{}, 7)
	for index, requirement := range r.Requirements {
		if requirement.Validate() != nil || requirement.Ref.TenantID != r.RestorePlan.TenantID || index > 0 && compareRestoreContextRequirementRefV1(r.Requirements[index-1], requirement) >= 0 {
			return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore execution requirement set is invalid")
		}
		kinds[requirement.Kind] = struct{}{}
	}
	if len(kinds) != 7 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRestoreIncompatible, "Restore execution Context route set is incomplete")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore execution request digest drifted")
	}
	return nil
}

type RestoreStageActionRequestV1 struct {
	ContractVersion  string                                                 `json:"contract_version"`
	ID               string                                                 `json:"id"`
	IdempotencyKey   string                                                 `json:"idempotency_key"`
	Attempt          runtimeports.RestoreAttemptRefV2                       `json:"restore_attempt"`
	Eligibility      runtimeports.RestoreEligibilityRefV2                   `json:"restore_eligibility"`
	Materialization  runtimeports.RestoreMaterializationCurrentProjectionV1 `json:"materialization_current"`
	NotAfterUnixNano int64                                                  `json:"not_after_unix_nano"`
	Digest           core.Digest                                            `json:"digest"`
}

func (r RestoreStageActionRequestV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Materialization = r.Materialization.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.restore-stage-action", RestoreExecutionContractVersionV1, "RestoreStageActionRequestV1", copy)
}

func SealRestoreStageActionRequestV1(r RestoreStageActionRequestV1) (RestoreStageActionRequestV1, error) {
	r.ContractVersion = RestoreExecutionContractVersionV1
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return RestoreStageActionRequestV1{}, err
	}
	r.Digest = digest
	return r, nil
}

func (r RestoreStageActionRequestV1) ValidateCurrent(now time.Time) error {
	if r.ContractVersion != RestoreExecutionContractVersionV1 || !validSingleCallIDV1(r.ID) || !validSingleCallIDV1(r.IdempotencyKey) || r.Attempt.Validate() != nil || r.Eligibility.Validate() != nil || r.Materialization.ValidateCurrent(now) != nil || r.NotAfterUnixNano <= 0 || now.IsZero() || now.UnixNano() >= r.NotAfterUnixNano || r.Digest.Validate() != nil || r.Attempt != r.Materialization.Attempt || r.Eligibility != r.Materialization.Eligibility {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRestoreIncompatible, "Restore Stage action request is incomplete or stale")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Stage action request digest drifted")
	}
	return nil
}

type RestoreStageActionInspectKeyV1 struct {
	ID             string                               `json:"id"`
	IdempotencyKey string                               `json:"idempotency_key"`
	Attempt        runtimeports.RestoreAttemptRefV2     `json:"restore_attempt"`
	Eligibility    runtimeports.RestoreEligibilityRefV2 `json:"restore_eligibility"`
	RequestDigest  core.Digest                          `json:"request_digest"`
}

func (k RestoreStageActionInspectKeyV1) Validate() error {
	if !validSingleCallIDV1(k.ID) || !validSingleCallIDV1(k.IdempotencyKey) || k.Attempt.Validate() != nil || k.Eligibility.Validate() != nil || k.RequestDigest.Validate() != nil || k.Attempt.TenantID != k.Eligibility.TenantID {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRestoreIncompatible, "Restore Stage inspect key is invalid")
	}
	return nil
}

type RestoreStageActionResultV1 struct {
	ContractVersion   string                                                      `json:"contract_version"`
	RequestDigest     core.Digest                                                 `json:"request_digest"`
	Stage             runtimeports.RestoreStageDomainResultCurrentProjectionV1    `json:"stage_current"`
	RuntimeSettlement runtimeports.RestoreStageSettlementRefV1                    `json:"runtime_settlement"`
	SandboxSettlement runtimeports.RestoreStageApplySettlementCurrentProjectionV1 `json:"sandbox_settlement_current"`
	CheckedUnixNano   int64                                                       `json:"checked_unix_nano"`
	ExpiresUnixNano   int64                                                       `json:"expires_unix_nano"`
	Digest            core.Digest                                                 `json:"digest"`
}

func (r RestoreStageActionResultV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.restore-stage-action", RestoreExecutionContractVersionV1, "RestoreStageActionResultV1", copy)
}

func SealRestoreStageActionResultV1(r RestoreStageActionResultV1) (RestoreStageActionResultV1, error) {
	r.ContractVersion = RestoreExecutionContractVersionV1
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return RestoreStageActionResultV1{}, err
	}
	r.Digest = digest
	return r, nil
}

func (r RestoreStageActionResultV1) ValidateFor(request RestoreStageActionRequestV1, now time.Time) error {
	if request.ValidateCurrent(now) != nil || r.ContractVersion != RestoreExecutionContractVersionV1 || r.RequestDigest != request.Digest || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano || now.UnixNano() < r.CheckedUnixNano || now.UnixNano() >= r.ExpiresUnixNano || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage action result is stale or not exact")
	}
	if err := r.Stage.Validate(now); err != nil {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage action DomainResult current is invalid")
	}
	if err := r.RuntimeSettlement.Validate(); err != nil {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage action Runtime Settlement ref is invalid")
	}
	if err := r.SandboxSettlement.ValidateCurrent(now); err != nil {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage action Sandbox Settlement current is invalid")
	}
	if r.Stage.Fact.RestoreAttempt != request.Attempt || r.Stage.Fact.Eligibility != request.Eligibility || !runtimeports.SameRestoreStageDomainResultFactRefV1(r.Stage.Fact, r.RuntimeSettlement.DomainResult) || !runtimeports.SameRestoreStageDomainResultFactRefV1(r.Stage.Fact, r.SandboxSettlement.Fact.DomainResult) || !runtimeports.SameRestoreStageSettlementRefV1(r.SandboxSettlement.Fact.RuntimeSettlement, r.RuntimeSettlement) {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage action exact Settlement closure drifted")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Stage action result digest drifted")
	}
	return nil
}

type RestoreExecutionResultV1 struct {
	ContractVersion string                                                        `json:"contract_version"`
	RequestDigest   core.Digest                                                   `json:"request_digest"`
	Attempt         runtimeports.RestoreAttemptRefV2                              `json:"restore_attempt"`
	Eligibility     runtimeports.RestoreEligibilityRefV2                          `json:"restore_eligibility"`
	Stage           RestoreStageActionResultV1                                    `json:"stage"`
	Context         runtimeports.RestoreContextMaterializationCurrentProjectionV1 `json:"context"`
	Activation      runtimeports.RestoreActivationRefV1                           `json:"activation"`
	Digest          core.Digest                                                   `json:"digest"`
}

func (r RestoreExecutionResultV1) Clone() RestoreExecutionResultV1 {
	r.Context = r.Context.Clone()
	return r
}

type RestoreExecutionResultFactV1 struct {
	ContractVersion string                    `json:"contract_version"`
	TenantID        core.TenantID             `json:"tenant_id"`
	ID              string                    `json:"id"`
	Revision        core.Revision             `json:"revision"`
	Request         RestoreExecutionRequestV1 `json:"request"`
	Result          RestoreExecutionResultV1  `json:"result"`
	CreatedUnixNano int64                     `json:"created_unix_nano"`
	Digest          core.Digest               `json:"digest"`
}

func (f RestoreExecutionResultFactV1) Clone() RestoreExecutionResultFactV1 {
	f.Request = f.Request.Clone()
	f.Result = f.Result.Clone()
	return f
}

func (f RestoreExecutionResultFactV1) DigestV1() (core.Digest, error) {
	copy := f.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.restore-execution", RestoreExecutionContractVersionV1, "RestoreExecutionResultFactV1", copy)
}

func SealRestoreExecutionResultFactV1(f RestoreExecutionResultFactV1, now time.Time) (RestoreExecutionResultFactV1, error) {
	f.ContractVersion = RestoreExecutionContractVersionV1
	f.TenantID = core.TenantID(f.Request.RestorePlan.TenantID)
	f.ID = f.Request.ID
	f.Revision = 1
	f.CreatedUnixNano = now.UnixNano()
	f.Digest = ""
	digest, err := f.DigestV1()
	if err != nil {
		return RestoreExecutionResultFactV1{}, err
	}
	f.Digest = digest
	return f, f.ValidateCurrent(now)
}

func (f RestoreExecutionResultFactV1) ValidateCurrent(now time.Time) error {
	if f.ContractVersion != RestoreExecutionContractVersionV1 || f.TenantID == "" || !validSingleCallIDV1(f.ID) || f.Revision != 1 || f.CreatedUnixNano <= 0 || f.Request.ID != f.ID || core.TenantID(f.Request.RestorePlan.TenantID) != f.TenantID || f.Result.RequestDigest != f.Request.Digest || f.Request.ValidateCurrent(now) != nil || f.Result.ValidateFor(f.Request, now) != nil || f.Digest.Validate() != nil {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore execution result Fact is incomplete or stale")
	}
	digest, err := f.DigestV1()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore execution result Fact digest drifted")
	}
	return nil
}

func (r RestoreExecutionResultV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.restore-execution", RestoreExecutionContractVersionV1, "RestoreExecutionResultV1", copy)
}

func SealRestoreExecutionResultV1(r RestoreExecutionResultV1) (RestoreExecutionResultV1, error) {
	r.ContractVersion = RestoreExecutionContractVersionV1
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return RestoreExecutionResultV1{}, err
	}
	r.Digest = digest
	return r, nil
}

func (r RestoreExecutionResultV1) ValidateFor(request RestoreExecutionRequestV1, now time.Time) error {
	if request.ValidateCurrent(now) != nil || r.ContractVersion != RestoreExecutionContractVersionV1 || r.RequestDigest != request.Digest || r.Attempt.Validate() != nil || r.Eligibility.Validate() != nil || r.Stage.ContractVersion != RestoreExecutionContractVersionV1 || r.Stage.RequestDigest.Validate() != nil || r.Stage.Stage.Validate(now) != nil || r.Stage.RuntimeSettlement.Validate() != nil || r.Stage.SandboxSettlement.ValidateCurrent(now) != nil || r.Context.ValidateCurrent(now) != nil || len(r.Context.Residuals) != 0 || r.Activation.Validate() != nil || r.Attempt.TenantID != runtimecoreTenantV1(request.RestorePlan.TenantID) || r.Eligibility.TenantID != r.Attempt.TenantID || r.Stage.Stage.Fact.RestoreAttempt != r.Attempt || r.Stage.Stage.Fact.Eligibility != r.Eligibility || !runtimeports.SameRestoreStageDomainResultFactRefV1(r.Stage.Stage.Fact, r.Stage.RuntimeSettlement.DomainResult) || !runtimeports.SameRestoreStageSettlementRefV1(r.Stage.SandboxSettlement.Fact.RuntimeSettlement, r.Stage.RuntimeSettlement) || r.Context.Fact.Attempt != r.Attempt || r.Context.Fact.Eligibility != r.Eligibility || r.Activation.Attempt.TenantID != r.Attempt.TenantID || r.Activation.Attempt.ID != r.Attempt.ID || r.Activation.Attempt.Revision != r.Attempt.Revision+1 || r.Digest.Validate() != nil {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore execution result is incomplete or drifted")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore execution result digest drifted")
	}
	return nil
}

func runtimecoreTenantV1(value string) core.TenantID { return core.TenantID(value) }
