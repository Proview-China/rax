package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	RestoreStageEvidenceContractVersionV1 = "1.0.0"
	RestoreStageEvidenceSourceIDV1        = NamespacedNameV2("praxis.sandbox/workspace-restore-stage-source")
	RestoreStageEvidenceEventKindV1       = NamespacedNameV2("praxis.sandbox/workspace-restore-stage-fact")
	RestoreStageEvidenceClassV1           = NamespacedNameV2("praxis.sandbox/authoritative-fact")
)

// RestoreStageDomainEvidenceCurrentProjectionV1 is the Sandbox Owner's
// current, immutable Evidence projection for one exact DomainResult fact.
// Runtime does not construct or reinterpret the Sandbox payload reference.
type RestoreStageDomainEvidenceCurrentProjectionV1 struct {
	ContractVersion  string                                      `json:"contract_version"`
	Domain           RestoreStageDomainResultCurrentProjectionV1 `json:"domain_current"`
	Payload          EvidencePayloadRefV2                        `json:"payload"`
	ProjectionDigest core.Digest                                 `json:"projection_digest"`
}

func (p RestoreStageDomainEvidenceCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.restore-stage-evidence", RestoreStageEvidenceContractVersionV1, "RestoreStageDomainEvidenceCurrentProjectionV1", copy)
}

func SealRestoreStageDomainEvidenceCurrentProjectionV1(p RestoreStageDomainEvidenceCurrentProjectionV1, now time.Time) (RestoreStageDomainEvidenceCurrentProjectionV1, error) {
	p.ContractVersion = RestoreStageEvidenceContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return RestoreStageDomainEvidenceCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.ValidateCurrent(now)
}

func (p RestoreStageDomainEvidenceCurrentProjectionV1) ValidateCurrent(now time.Time) error {
	if p.ContractVersion != RestoreStageEvidenceContractVersionV1 || p.Domain.Validate(now) != nil || p.Payload.Validate() != nil || p.ProjectionDigest.Validate() != nil || p.Payload.Schema.Key() != p.Domain.Fact.PayloadSchema.Key() || p.Payload.ContentDigest != p.Domain.Fact.PayloadDigest || p.Payload.Revision != p.Domain.Fact.PayloadRevision {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "Restore Stage DomainResult Evidence projection is incomplete or stale")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Stage DomainResult Evidence projection drifted")
	}
	return nil
}

type RestoreStageDomainEvidenceCurrentReaderV1 interface {
	InspectRestoreStageDomainEvidenceCurrentV1(context.Context, RestoreStageDomainResultFactRefV1) (RestoreStageDomainEvidenceCurrentProjectionV1, error)
}

// PublishRestoreStageEvidenceRequestV1 carries only exact coordinates. The
// source sequence is deliberately absent and is derived from the Evidence
// Owner's create-once dedicated source.
type PublishRestoreStageEvidenceRequestV1 struct {
	Governance         RestoreStageGovernanceCurrentProjectionV1 `json:"governance_current"`
	DomainResult       RestoreStageDomainResultFactRefV1         `json:"domain_result"`
	SourceRegistration EvidenceSourceRegistrationRefV1           `json:"source_registration"`
}

func (r PublishRestoreStageEvidenceRequestV1) Validate(now time.Time) error {
	if r.Governance.Validate(now) != nil || r.DomainResult.Validate() != nil || r.SourceRegistration.Validate() != nil || r.SourceRegistration.Revision != 1 || r.SourceRegistration.SourceID != RestoreStageEvidenceSourceIDV1 || !SameOperationSubjectV3(r.Governance.Operation, r.DomainResult.Operation) || r.Governance.RestoreAttempt != r.DomainResult.RestoreAttempt || r.Governance.Eligibility != r.DomainResult.Eligibility || r.Governance.DispatchAttempt != r.DomainResult.Attempt {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Restore Stage Evidence request does not bind one exact governed DomainResult")
	}
	return nil
}

type RestoreStageEvidenceGovernancePortV1 interface {
	PublishRestoreStageEvidenceV1(context.Context, PublishRestoreStageEvidenceRequestV1) (EvidenceRecordRefV2, error)
	InspectRestoreStageEvidenceV1(context.Context, PublishRestoreStageEvidenceRequestV1) (EvidenceLedgerRecordV2, error)
}
