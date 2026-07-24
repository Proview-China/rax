package ports

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	RestoreStageGovernanceContractVersionV1              = "1.0.0"
	RestoreStageEffectKindV1                EffectKindV2 = "praxis.runtime/restore-workspace-stage"
	RestoreStageOperationKindV1                          = OperationScopeKindV3("praxis.runtime/restore-attempt")
	RestoreStagePayloadSchemaNamespaceV1                 = "praxis.runtime"
	RestoreStagePayloadSchemaNameV1                      = "restore-stage-payload"
	RestoreStagePayloadSchemaVersionV1                   = "1.0.0"
)

// RestoreStageOperationPayloadV1 is the only typed binding carried through
// the existing Operation V3 governance chain. It grants no capability by
// itself and contains no Provider/root coordinate.
type RestoreStageOperationPayloadV1 struct {
	ContractVersion  string                           `json:"contract_version"`
	RestoreAttempt   RestoreAttemptRefV2              `json:"restore_attempt"`
	Eligibility      RestoreEligibilityRefV2          `json:"restore_eligibility"`
	Identity         RestoreIdentityReservationV2     `json:"identity_reservation"`
	SnapshotArtifact CheckpointExternalExactFactRefV2 `json:"snapshot_artifact"`
}

func (p RestoreStageOperationPayloadV1) Validate() error {
	if p.ContractVersion != RestoreStageGovernanceContractVersionV1 || p.RestoreAttempt.Validate() != nil || p.Eligibility.Validate() != nil || p.Identity.Validate() != nil || p.SnapshotArtifact.Validate() != nil || p.RestoreAttempt.TenantID != p.Eligibility.TenantID || string(p.RestoreAttempt.TenantID) != p.SnapshotArtifact.TenantID {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRestoreIncompatible, "Restore Stage payload is incomplete or cross-tenant")
	}
	return nil
}

func (p RestoreStageOperationPayloadV1) CanonicalBytesV1() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(p)
}

func DecodeRestoreStageOperationPayloadV1(data []byte) (RestoreStageOperationPayloadV1, error) {
	if len(data) == 0 {
		return RestoreStageOperationPayloadV1{}, restoreInvalidV2("Restore Stage payload is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value RestoreStageOperationPayloadV1
	if err := decoder.Decode(&value); err != nil {
		return RestoreStageOperationPayloadV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return RestoreStageOperationPayloadV1{}, restoreInvalidV2("Restore Stage payload contains trailing JSON")
	}
	if err := value.Validate(); err != nil {
		return RestoreStageOperationPayloadV1{}, err
	}
	canonical, err := value.CanonicalBytesV1()
	if err != nil || !bytes.Equal(data, canonical) {
		return RestoreStageOperationPayloadV1{}, restoreInvalidV2("Restore Stage payload is not strict canonical JSON")
	}
	return value, nil
}

type InspectRestoreStageGovernanceCurrentRequestV1 struct {
	RestoreAttempt     RestoreAttemptRefV2                    `json:"restore_attempt"`
	Eligibility        RestoreEligibilityRefV2                `json:"restore_eligibility"`
	Operation          OperationSubjectV3                     `json:"operation"`
	EffectID           core.EffectIntentID                    `json:"effect_id"`
	Admission          OperationEffectAdmissionReceiptV3      `json:"admission"`
	Authorization      OperationReviewAuthorizationRefV4      `json:"authorization"`
	PermitID           string                                 `json:"permit_id"`
	DispatchAttempt    OperationDispatchAttemptRefV3          `json:"dispatch_attempt"`
	ExecuteEnforcement OperationDispatchEnforcementPhaseRefV4 `json:"execute_enforcement"`
	SnapshotArtifact   CheckpointExternalExactFactRefV2       `json:"snapshot_artifact"`
}

func (r InspectRestoreStageGovernanceCurrentRequestV1) Validate() error {
	if r.RestoreAttempt.Validate() != nil || r.Eligibility.Validate() != nil || r.Operation.Validate() != nil || strings.TrimSpace(string(r.EffectID)) == "" || r.Admission.Validate() != nil || r.Authorization.Validate() != nil || strings.TrimSpace(r.PermitID) == "" || r.DispatchAttempt.Validate() != nil || r.ExecuteEnforcement.Validate() != nil || r.SnapshotArtifact.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRestoreIncompatible, "Restore Stage governance current request is incomplete")
	}
	if r.RestoreAttempt.TenantID != r.Eligibility.TenantID || string(r.RestoreAttempt.TenantID) != r.SnapshotArtifact.TenantID || r.Operation.ExecutionScope.Identity.TenantID != r.RestoreAttempt.TenantID || r.Admission.EffectID != r.EffectID || r.DispatchAttempt.EffectID != r.EffectID || r.ExecuteEnforcement.EffectID != r.EffectID || r.DispatchAttempt.PermitID != r.PermitID || r.ExecuteEnforcement.PermitID != r.PermitID || r.ExecuteEnforcement.ReviewAuthorization != r.Authorization {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage governance request exact coordinates drifted")
	}
	return nil
}

type RestoreStageGovernanceCurrentProjectionV1 struct {
	ContractVersion         string                                 `json:"contract_version"`
	RestoreAttempt          RestoreAttemptRefV2                    `json:"restore_attempt"`
	Eligibility             RestoreEligibilityRefV2                `json:"restore_eligibility"`
	Identity                RestoreIdentityReservationV2           `json:"identity_reservation"`
	Operation               OperationSubjectV3                     `json:"operation"`
	EffectID                core.EffectIntentID                    `json:"effect_id"`
	EffectRevision          core.Revision                          `json:"effect_revision"`
	IntentDigest            core.Digest                            `json:"intent_digest"`
	Admission               OperationEffectAdmissionReceiptV3      `json:"admission"`
	DispatchAdmissionDigest core.Digest                            `json:"dispatch_admission_digest"`
	Authorization           OperationReviewAuthorizationRefV4      `json:"authorization"`
	PermitID                string                                 `json:"permit_id"`
	PermitFactRevision      core.Revision                          `json:"permit_fact_revision"`
	PermitDigest            core.Digest                            `json:"permit_digest"`
	BeginRecordRevision     core.Revision                          `json:"begin_record_revision"`
	BeginRecordDigest       core.Digest                            `json:"begin_record_digest"`
	DispatchAttempt         OperationDispatchAttemptRefV3          `json:"dispatch_attempt"`
	ExecuteEnforcement      OperationDispatchEnforcementPhaseRefV4 `json:"execute_enforcement"`
	MaterializationDigest   core.Digest                            `json:"materialization_digest"`
	SnapshotArtifact        CheckpointExternalExactFactRefV2       `json:"snapshot_artifact"`
	CheckedUnixNano         int64                                  `json:"checked_unix_nano"`
	ExpiresUnixNano         int64                                  `json:"expires_unix_nano"`
	ProjectionDigest        core.Digest                            `json:"projection_digest"`
}

func (p RestoreStageGovernanceCurrentProjectionV1) Validate(now time.Time) error {
	if p.ContractVersion != RestoreStageGovernanceContractVersionV1 || p.RestoreAttempt.Validate() != nil || p.Eligibility.Validate() != nil || p.Identity.Validate() != nil || p.Operation.Validate() != nil || strings.TrimSpace(string(p.EffectID)) == "" || p.EffectRevision == 0 || p.IntentDigest.Validate() != nil || p.Admission.Validate() != nil || p.DispatchAdmissionDigest.Validate() != nil || p.Authorization.Validate() != nil || strings.TrimSpace(p.PermitID) == "" || p.PermitFactRevision == 0 || p.PermitDigest.Validate() != nil || p.BeginRecordRevision == 0 || p.BeginRecordDigest.Validate() != nil || p.DispatchAttempt.Validate() != nil || p.ExecuteEnforcement.Validate() != nil || p.MaterializationDigest.Validate() != nil || p.SnapshotArtifact.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || now.UnixNano() >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRestoreIncompatible, "Restore Stage governance projection is incomplete or stale")
	}
	if p.RestoreAttempt.TenantID != p.Eligibility.TenantID || p.Operation.ExecutionScope.Identity.TenantID != p.RestoreAttempt.TenantID || p.Operation.ExecutionScope.Instance != p.Identity.TargetInstance || p.Operation.ExecutionScope.SandboxLease == nil || *p.Operation.ExecutionScope.SandboxLease != p.Identity.TargetLease || p.Operation.ExecutionScope.AuthorityEpoch != p.Identity.TargetFenceEpoch || p.Admission.EffectID != p.EffectID || p.Admission.IntentRevision != p.EffectRevision || p.Admission.IntentDigest != p.IntentDigest || p.DispatchAttempt.EffectID != p.EffectID || p.DispatchAttempt.IntentRevision != p.EffectRevision || p.DispatchAttempt.IntentDigest != p.IntentDigest || p.DispatchAttempt.PermitID != p.PermitID || p.DispatchAttempt.PermitRevision != p.PermitFactRevision || p.DispatchAttempt.PermitDigest != p.PermitDigest || p.ExecuteEnforcement.OperationDigest != p.DispatchAttempt.OperationDigest || p.ExecuteEnforcement.EffectID != p.EffectID || p.ExecuteEnforcement.PermitID != p.PermitID || p.ExecuteEnforcement.PermitFactRevision != p.PermitFactRevision || p.ExecuteEnforcement.PermitDigest != p.PermitDigest || p.ExecuteEnforcement.AdmissionDigest != p.DispatchAdmissionDigest || p.ExecuteEnforcement.ReviewAuthorization != p.Authorization || p.ExecuteEnforcement.AttemptID != p.DispatchAttempt.AttemptID || p.ExecuteEnforcement.Phase != OperationDispatchEnforcementExecuteV4 {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage governance exact closure drifted")
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Stage governance projection digest drifted")
	}
	return nil
}

func (p RestoreStageGovernanceCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.restore-stage-governance", RestoreStageGovernanceContractVersionV1, "RestoreStageGovernanceCurrentProjectionV1", copy)
}

func SealRestoreStageGovernanceCurrentProjectionV1(p RestoreStageGovernanceCurrentProjectionV1, now time.Time) (RestoreStageGovernanceCurrentProjectionV1, error) {
	p.ContractVersion = RestoreStageGovernanceContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return RestoreStageGovernanceCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type RestoreStageGovernanceCurrentPortV1 interface {
	InspectRestoreStageGovernanceCurrentV1(context.Context, InspectRestoreStageGovernanceCurrentRequestV1) (RestoreStageGovernanceCurrentProjectionV1, error)
}
