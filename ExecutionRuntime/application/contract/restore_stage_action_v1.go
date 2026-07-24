package contract

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type RestoreStageAuthorizationInputCurrentProjectionV1 struct {
	ContractVersion   string                                        `json:"contract_version"`
	RequestDigest     core.Digest                                   `json:"request_digest"`
	Intent            runtimeports.OperationEffectIntentV3          `json:"intent"`
	SnapshotArtifact  runtimeports.CheckpointExternalExactFactRefV2 `json:"snapshot_artifact"`
	EvidenceSource    runtimeports.EvidenceSourceRegistrationRefV1  `json:"evidence_source"`
	AuthorizationID   string                                        `json:"authorization_id"`
	PermitID          string                                        `json:"permit_id"`
	DispatchAttemptID string                                        `json:"dispatch_attempt_id"`
	PermitTTL         time.Duration                                 `json:"permit_ttl"`
	CheckedUnixNano   int64                                         `json:"checked_unix_nano"`
	ExpiresUnixNano   int64                                         `json:"expires_unix_nano"`
	ProjectionDigest  core.Digest                                   `json:"projection_digest"`
}

func (p RestoreStageAuthorizationInputCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.Intent.Payload.Inline = append([]byte(nil), p.Intent.Payload.Inline...)
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.application.restore-stage-authorization-input", RestoreExecutionContractVersionV1, "RestoreStageAuthorizationInputCurrentProjectionV1", copy)
}

func SealRestoreStageAuthorizationInputCurrentProjectionV1(p RestoreStageAuthorizationInputCurrentProjectionV1, now time.Time) (RestoreStageAuthorizationInputCurrentProjectionV1, error) {
	p.Intent.Payload.Inline = append([]byte(nil), p.Intent.Payload.Inline...)
	p.ContractVersion = RestoreExecutionContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return RestoreStageAuthorizationInputCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.ValidateCurrent(now)
}

func (p RestoreStageAuthorizationInputCurrentProjectionV1) ValidateCurrent(now time.Time) error {
	if p.ContractVersion != RestoreExecutionContractVersionV1 || p.RequestDigest.Validate() != nil || p.Intent.Validate() != nil || p.SnapshotArtifact.Validate() != nil || p.EvidenceSource.Validate() != nil || p.EvidenceSource.Revision != 1 || p.EvidenceSource.SourceID != runtimeports.RestoreStageEvidenceSourceIDV1 || !validSingleCallIDV1(p.AuthorizationID) || !validSingleCallIDV1(p.PermitID) || !validSingleCallIDV1(p.DispatchAttemptID) || p.PermitTTL <= 0 || p.PermitTTL > runtimeports.MaxDispatchPermitTTL || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || now.UnixNano() >= p.ExpiresUnixNano || p.ExpiresUnixNano > p.Intent.ExpiresUnixNano || p.ProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectAuthorizationMissing, "Restore Stage authorization input is incomplete or stale")
	}
	if p.Intent.Operation.Kind != runtimeports.RestoreStageOperationKindV1 || p.Intent.Operation.CustomOperationID == "" {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage authorization input crosses operation kind")
	}
	if _, err := restoreStageAuthorizationPayloadV1(p.Intent); err != nil {
		return err
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Stage authorization input drifted")
	}
	return nil
}

func (p RestoreStageAuthorizationInputCurrentProjectionV1) ValidateFor(request RestoreStageActionRequestV1, now time.Time) error {
	payload, payloadErr := restoreStageAuthorizationPayloadV1(p.Intent)
	if p.ValidateCurrent(now) != nil || request.ValidateCurrent(now) != nil || payloadErr != nil || p.RequestDigest != request.Digest || p.Intent.Operation.CustomOperationID != request.Attempt.ID || p.Intent.Operation.ExecutionScope.Identity.TenantID != request.Attempt.TenantID || p.Intent.Operation.ExecutionScope.Instance != request.Materialization.Identity.TargetInstance || p.Intent.Operation.ExecutionScope.SandboxLease == nil || *p.Intent.Operation.ExecutionScope.SandboxLease != request.Materialization.Identity.TargetLease || p.Intent.Operation.ExecutionScope.AuthorityEpoch != request.Materialization.Identity.TargetFenceEpoch || p.SnapshotArtifact.TenantID != string(request.Attempt.TenantID) || !restoreMaterializationHasSnapshotV1(request.Materialization.Snapshots, p.SnapshotArtifact) || p.ExpiresUnixNano > request.NotAfterUnixNano || payload.RestoreAttempt != request.Attempt || payload.Eligibility != request.Eligibility || payload.Identity != request.Materialization.Identity || payload.SnapshotArtifact != p.SnapshotArtifact {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage authorization input does not bind the exact request")
	}
	return nil
}

func (p RestoreStageAuthorizationInputCurrentProjectionV1) ValidateForInspect(key RestoreStageActionInspectKeyV1, now time.Time) error {
	payload, payloadErr := restoreStageAuthorizationPayloadV1(p.Intent)
	if p.ValidateCurrent(now) != nil || key.Validate() != nil || payloadErr != nil || p.RequestDigest != key.RequestDigest || p.Intent.Operation.CustomOperationID != key.Attempt.ID || p.Intent.Operation.ExecutionScope.Identity.TenantID != key.Attempt.TenantID || p.SnapshotArtifact.TenantID != string(key.Attempt.TenantID) || payload.RestoreAttempt != key.Attempt || payload.Eligibility != key.Eligibility || payload.SnapshotArtifact != p.SnapshotArtifact {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage authorization input does not bind the inspect key")
	}
	return nil
}

func restoreStageAuthorizationPayloadV1(intent runtimeports.OperationEffectIntentV3) (runtimeports.RestoreStageOperationPayloadV1, error) {
	if intent.Kind != runtimeports.RestoreStageEffectKindV1 || intent.Payload.Ref != "" || len(intent.Payload.Inline) == 0 || intent.Payload.Schema.Namespace != runtimeports.RestoreStagePayloadSchemaNamespaceV1 || intent.Payload.Schema.Name != runtimeports.RestoreStagePayloadSchemaNameV1 || intent.Payload.Schema.Version != runtimeports.RestoreStagePayloadSchemaVersionV1 {
		return runtimeports.RestoreStageOperationPayloadV1{}, core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage authorization input lacks the typed inline payload")
	}
	return runtimeports.DecodeRestoreStageOperationPayloadV1(intent.Payload.Inline)
}

type RestoreStageAuthorizedDispatchV1 struct {
	RequestDigest    core.Digest                                          `json:"request_digest"`
	Dispatch         runtimeports.CurrentOperationDispatchAuthorizationV4 `json:"dispatch_current"`
	SnapshotArtifact runtimeports.CheckpointExternalExactFactRefV2        `json:"snapshot_artifact"`
	EvidenceSource   runtimeports.EvidenceSourceRegistrationRefV1         `json:"evidence_source"`
	CheckedUnixNano  int64                                                `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                                `json:"expires_unix_nano"`
	Digest           core.Digest                                          `json:"digest"`
}

func (a RestoreStageAuthorizedDispatchV1) DigestV1() (core.Digest, error) {
	copy := a
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.restore-stage-action", RestoreExecutionContractVersionV1, "RestoreStageAuthorizedDispatchV1", copy)
}

func SealRestoreStageAuthorizedDispatchV1(a RestoreStageAuthorizedDispatchV1) (RestoreStageAuthorizedDispatchV1, error) {
	a.Digest = ""
	digest, err := a.DigestV1()
	if err != nil {
		return RestoreStageAuthorizedDispatchV1{}, err
	}
	a.Digest = digest
	return a, nil
}

func (a RestoreStageAuthorizedDispatchV1) ValidateFor(request RestoreStageActionRequestV1, now time.Time) error {
	if request.ValidateCurrent(now) != nil || a.RequestDigest != request.Digest || a.Dispatch.Validate() != nil || a.SnapshotArtifact.Validate() != nil || a.EvidenceSource.Validate() != nil || a.EvidenceSource.Revision != 1 || a.EvidenceSource.SourceID != runtimeports.RestoreStageEvidenceSourceIDV1 || a.CheckedUnixNano <= 0 || a.ExpiresUnixNano <= a.CheckedUnixNano || now.IsZero() || now.UnixNano() < a.CheckedUnixNano || now.UnixNano() >= a.ExpiresUnixNano || a.Digest.Validate() != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "Restore Stage authorization is incomplete or stale")
	}
	record := a.Dispatch.Record
	legacy := record.Permit.LegacyPermit
	if record.State != runtimeports.OperationPermitBegunV4 || legacy.Operation.Kind != runtimeports.RestoreStageOperationKindV1 || legacy.Operation.CustomOperationID != request.Attempt.ID || legacy.Operation.ExecutionScope.Identity.TenantID != request.Attempt.TenantID || legacy.Operation.ExecutionScope.Instance != request.Materialization.Identity.TargetInstance || legacy.Operation.ExecutionScope.SandboxLease == nil || *legacy.Operation.ExecutionScope.SandboxLease != request.Materialization.Identity.TargetLease || legacy.Operation.ExecutionScope.AuthorityEpoch != request.Materialization.Identity.TargetFenceEpoch || record.Permit.Admission.Admission.EffectID != legacy.IntentID || record.Permit.Admission.Authorization != a.Dispatch.ReviewAuthorization || legacy.EnforcementPoint.Validate() != nil || a.SnapshotArtifact.TenantID != string(request.Attempt.TenantID) || !restoreMaterializationHasSnapshotV1(request.Materialization.Snapshots, a.SnapshotArtifact) || a.ExpiresUnixNano > legacy.ExpiresUnixNano || a.ExpiresUnixNano > request.NotAfterUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage authorization exact closure drifted")
	}
	digest, err := a.DigestV1()
	if err != nil || digest != a.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Stage authorization digest drifted")
	}
	return nil
}

func restoreMaterializationHasSnapshotV1(values []runtimeports.CheckpointExternalExactFactRefV2, expected runtimeports.CheckpointExternalExactFactRefV2) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

type RestoreStageEvidenceRequestV1 struct {
	RequestDigest      core.Digest                                            `json:"request_digest"`
	Governance         runtimeports.RestoreStageGovernanceCurrentProjectionV1 `json:"governance_current"`
	DomainResult       runtimeports.RestoreStageDomainResultFactRefV1         `json:"domain_result"`
	SourceRegistration runtimeports.EvidenceSourceRegistrationRefV1           `json:"source_registration"`
}

func (r RestoreStageEvidenceRequestV1) Validate(now time.Time) error {
	if r.RequestDigest.Validate() != nil || r.Governance.Validate(now) != nil || r.DomainResult.Validate() != nil || r.SourceRegistration.Validate() != nil || r.SourceRegistration.Revision != 1 || r.SourceRegistration.SourceID != runtimeports.RestoreStageEvidenceSourceIDV1 || !runtimeports.SameOperationSubjectV3(r.Governance.Operation, r.DomainResult.Operation) || r.Governance.RestoreAttempt != r.DomainResult.RestoreAttempt || r.Governance.Eligibility != r.DomainResult.Eligibility || r.Governance.DispatchAttempt != r.DomainResult.Attempt {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Restore Stage Evidence request does not bind the exact Owner Fact")
	}
	return nil
}

type RestoreStageActionResultFactV1 struct {
	ContractVersion string                      `json:"contract_version"`
	TenantID        core.TenantID               `json:"tenant_id"`
	ID              string                      `json:"id"`
	Revision        core.Revision               `json:"revision"`
	Request         RestoreStageActionRequestV1 `json:"request"`
	RequestDigest   core.Digest                 `json:"request_digest"`
	Result          RestoreStageActionResultV1  `json:"result"`
	CreatedUnixNano int64                       `json:"created_unix_nano"`
	Digest          core.Digest                 `json:"digest"`
}

func (f RestoreStageActionResultFactV1) Clone() RestoreStageActionResultFactV1 {
	f.Request.Materialization = f.Request.Materialization.Clone()
	return f
}

func (f RestoreStageActionResultFactV1) DigestV1() (core.Digest, error) {
	copy := f.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.restore-stage-action", RestoreExecutionContractVersionV1, "RestoreStageActionResultFactV1", copy)
}

func SealRestoreStageActionResultFactV1(f RestoreStageActionResultFactV1, request RestoreStageActionRequestV1, now time.Time) (RestoreStageActionResultFactV1, error) {
	f.ContractVersion = RestoreExecutionContractVersionV1
	f.TenantID = request.Attempt.TenantID
	f.ID = request.ID
	f.Revision = 1
	f.Request = request
	f.Request.Materialization = request.Materialization.Clone()
	f.RequestDigest = request.Digest
	f.CreatedUnixNano = now.UnixNano()
	f.Digest = ""
	digest, err := f.DigestV1()
	if err != nil {
		return RestoreStageActionResultFactV1{}, err
	}
	f.Digest = digest
	return f, f.ValidateFor(request, now)
}

func (f RestoreStageActionResultFactV1) ValidateFor(request RestoreStageActionRequestV1, now time.Time) error {
	if f.ContractVersion != RestoreExecutionContractVersionV1 || f.TenantID != request.Attempt.TenantID || f.ID != request.ID || f.Revision != 1 || f.RequestDigest != request.Digest || f.Request.Digest != request.Digest || f.CreatedUnixNano <= 0 || f.Digest.Validate() != nil {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage result Fact is incomplete or stale")
	}
	if err := f.Request.ValidateCurrent(now); err != nil {
		return err
	}
	if err := f.Result.ValidateFor(f.Request, now); err != nil {
		return err
	}
	digest, err := f.DigestV1()
	if err != nil || digest != f.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Stage result Fact digest drifted")
	}
	return nil
}
