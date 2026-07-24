package contract

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const SandboxLifecycleContractVersionV4 = "praxis.application/sandbox-lifecycle/v4"

type SandboxLifecyclePlanRefV4 struct {
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r SandboxLifecyclePlanRefV4) ValidateCurrent(now time.Time) error {
	if r.ID == "" || r.Revision == 0 || r.Digest.Validate() != nil || r.ExpiresUnixNano <= 0 || now.IsZero() || !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "Sandbox lifecycle plan ref is incomplete or expired")
	}
	return nil
}

type SandboxLifecycleRequestV4 struct {
	ContractVersion   string                          `json:"contract_version"`
	ID                string                          `json:"id"`
	Plan              SandboxLifecyclePlanRefV4       `json:"plan"`
	Operation         runtimeports.OperationSubjectV3 `json:"operation"`
	EffectID          core.EffectIntentID             `json:"effect_id"`
	AttemptID         string                          `json:"attempt_id"`
	RequestedUnixNano int64                           `json:"requested_unix_nano"`
	Digest            core.Digest                     `json:"digest"`
}

func SealSandboxLifecycleRequestV4(value SandboxLifecycleRequestV4) (SandboxLifecycleRequestV4, error) {
	value.ContractVersion = SandboxLifecycleContractVersionV4
	value.Digest = ""
	digest, err := value.DigestV4()
	if err != nil {
		return SandboxLifecycleRequestV4{}, err
	}
	value.Digest = digest
	return value, value.ValidateCurrent(time.Unix(0, value.RequestedUnixNano))
}

func (r SandboxLifecycleRequestV4) DigestV4() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.sandbox-lifecycle", SandboxLifecycleContractVersionV4, "SandboxLifecycleRequestV4", copy)
}

func (r SandboxLifecycleRequestV4) ValidateCurrent(now time.Time) error {
	if r.ContractVersion != SandboxLifecycleContractVersionV4 || r.ID == "" || r.AttemptID == "" || r.EffectID == "" || r.RequestedUnixNano <= 0 || r.Operation.Validate() != nil || r.Plan.ValidateCurrent(now) != nil || now.IsZero() || now.UnixNano() < r.RequestedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "Sandbox lifecycle request is incomplete, future-dated, or expired")
	}
	digest, err := r.DigestV4()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Sandbox lifecycle request digest drifted")
	}
	return nil
}

type SandboxLifecycleResultV4 struct {
	ContractVersion string                                                `json:"contract_version"`
	ID              string                                                `json:"id"`
	Revision        core.Revision                                         `json:"revision"`
	RequestDigest   core.Digest                                           `json:"request_digest"`
	Plan            SandboxLifecyclePlanRefV4                             `json:"plan"`
	DomainResult    runtimeports.OperationSettlementDomainResultFactRefV4 `json:"domain_result"`
	Settlement      runtimeports.OperationInspectionSettlementRefV4       `json:"settlement"`
	CheckedUnixNano int64                                                 `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                                 `json:"expires_unix_nano"`
	Digest          core.Digest                                           `json:"digest"`
}

func SealSandboxLifecycleResultV4(value SandboxLifecycleResultV4, now time.Time) (SandboxLifecycleResultV4, error) {
	value.ContractVersion = SandboxLifecycleContractVersionV4
	value.Revision = 1
	value.Digest = ""
	digest, err := value.DigestV4()
	if err != nil {
		return SandboxLifecycleResultV4{}, err
	}
	value.Digest = digest
	return value, value.ValidateCurrent(now)
}

func (r SandboxLifecycleResultV4) DigestV4() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.sandbox-lifecycle", SandboxLifecycleContractVersionV4, "SandboxLifecycleResultV4", copy)
}

func (r SandboxLifecycleResultV4) ValidateCurrent(now time.Time) error {
	if r.ContractVersion != SandboxLifecycleContractVersionV4 || r.ID == "" || r.Revision != 1 || r.RequestDigest.Validate() != nil || r.Plan.ValidateCurrent(now) != nil || r.DomainResult.Validate() != nil || r.Settlement.Validate(now) != nil || r.CheckedUnixNano <= 0 || r.ExpiresUnixNano <= r.CheckedUnixNano || now.IsZero() || !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "Sandbox lifecycle result is incomplete or expired")
	}
	if !runtimeports.SameOperationSettlementDomainResultFactRefV4(r.DomainResult, r.Settlement.DomainResult) || r.Settlement.Settlement.DomainResult != r.DomainResult {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Sandbox lifecycle result combines different DomainResult and Settlement facts")
	}
	digest, err := r.DigestV4()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Sandbox lifecycle result digest drifted")
	}
	return nil
}
