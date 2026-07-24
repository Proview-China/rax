package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const CheckpointRestoreEvidenceContractVersionV1 = "1.0.0"

const MaxCheckpointRestoreEvidenceQualificationTTLV1 = 30 * time.Second

type CheckpointRestoreEvidenceScopeV1 struct {
	Operation                 OperationSubjectV3                     `json:"operation"`
	OperationDigest           core.Digest                            `json:"operation_digest"`
	EffectID                  core.EffectIntentID                    `json:"effect_id"`
	EffectRevision            core.Revision                          `json:"effect_revision"`
	EffectKind                EffectKindV2                           `json:"effect_kind"`
	IntentDigest              core.Digest                            `json:"intent_digest"`
	Admission                 OperationEffectAdmissionReceiptV3      `json:"admission"`
	Authorization             OperationReviewAuthorizationRefV4      `json:"authorization"`
	DispatchAttempt           OperationDispatchAttemptRefV3          `json:"dispatch_attempt"`
	PermitID                  string                                 `json:"permit_id"`
	PermitFactRevision        core.Revision                          `json:"permit_fact_revision"`
	PermitDigest              core.Digest                            `json:"permit_digest"`
	AuthorizedAdmissionDigest core.Digest                            `json:"authorized_admission_digest"`
	PrepareEnforcement        OperationDispatchEnforcementPhaseRefV4 `json:"prepare_enforcement"`
	ExecuteEnforcement        OperationDispatchEnforcementPhaseRefV4 `json:"execute_enforcement"`
	Generation                GenerationArtifactRefV1                `json:"generation"`
	Assembly                  GenerationBindingAssociationRefV1      `json:"assembly"`
	SandboxAttempt            OperationDispatchSandboxFactRefV4      `json:"sandbox_attempt"`
	SandboxProjectionDigest   core.Digest                            `json:"sandbox_projection_digest"`
	SandboxLease              core.SandboxLeaseRef                   `json:"sandbox_lease"`
	FenceEpoch                core.Epoch                             `json:"fence_epoch"`
	Authority                 AuthorityBindingRefV2                  `json:"authority"`
	EvidencePolicy            OperationScopeEvidencePolicyRefV3      `json:"evidence_policy"`
	PayloadSchema             SchemaRefV2                            `json:"payload_schema"`
	PayloadDigest             core.Digest                            `json:"payload_digest"`
	PayloadRevision           core.Revision                          `json:"payload_revision"`
	PayloadLength             uint64                                 `json:"payload_length"`
	Source                    EvidenceSourceKeyV2                    `json:"source"`
}

func (s CheckpointRestoreEvidenceScopeV1) Validate() error {
	if s.Operation.Validate() != nil || s.OperationDigest.Validate() != nil || s.EffectID == "" || s.EffectRevision == 0 || ValidateNamespacedNameV2(NamespacedNameV2(s.EffectKind)) != nil || s.IntentDigest.Validate() != nil || s.Admission.Validate() != nil || s.Authorization.Validate() != nil || s.DispatchAttempt.Validate() != nil || !validCheckpointIDV2(s.PermitID) || s.PermitFactRevision == 0 || s.PermitDigest.Validate() != nil || s.AuthorizedAdmissionDigest.Validate() != nil || s.PrepareEnforcement.Validate() != nil || s.ExecuteEnforcement.Validate() != nil || s.Generation.Validate() != nil || s.Assembly.Validate() != nil || s.SandboxAttempt.Validate() != nil || s.SandboxProjectionDigest.Validate() != nil || s.SandboxLease.Validate() != nil || s.FenceEpoch == 0 || s.Authority.Validate() != nil || s.EvidencePolicy.Validate() != nil || s.PayloadSchema.Validate() != nil || s.PayloadDigest.Validate() != nil || s.PayloadRevision == 0 || s.PayloadLength == 0 || s.PayloadLength > MaxOperationScopeEvidencePayloadBytesV3 || s.Source.Validate() != nil {
		return checkpointInvalidV2("checkpoint Evidence scope is incomplete")
	}
	operationDigest, err := s.Operation.DigestV3()
	lease := s.Operation.ExecutionScope.SandboxLease
	if err != nil || operationDigest != s.OperationDigest || s.Operation.ExecutionScope.Identity.TenantID == "" || lease == nil || *lease != s.SandboxLease || s.FenceEpoch != s.SandboxLease.Epoch || s.Admission.OperationDigest != s.OperationDigest || s.Admission.EffectID != s.EffectID || s.Admission.IntentDigest != s.IntentDigest || s.DispatchAttempt.OperationDigest != s.OperationDigest || s.DispatchAttempt.EffectID != s.EffectID || s.DispatchAttempt.PermitID != s.PermitID || s.PrepareEnforcement.PermitID != s.PermitID || s.ExecuteEnforcement.PermitID != s.PermitID || s.PrepareEnforcement.PermitFactRevision != s.PermitFactRevision || s.ExecuteEnforcement.PermitFactRevision != s.PermitFactRevision || s.PrepareEnforcement.PermitDigest != s.PermitDigest || s.ExecuteEnforcement.PermitDigest != s.PermitDigest || s.PrepareEnforcement.AdmissionDigest != s.AuthorizedAdmissionDigest || s.ExecuteEnforcement.AdmissionDigest != s.AuthorizedAdmissionDigest || s.PrepareEnforcement.OperationDigest != s.OperationDigest || s.ExecuteEnforcement.OperationDigest != s.OperationDigest || s.PrepareEnforcement.EffectID != s.EffectID || s.ExecuteEnforcement.EffectID != s.EffectID || s.PrepareEnforcement.Phase != OperationDispatchEnforcementPrepareV4 || s.ExecuteEnforcement.Phase != OperationDispatchEnforcementExecuteV4 || s.PrepareEnforcement.ReceiptDigest != s.ExecuteEnforcement.PrepareReceiptDigest || s.PrepareEnforcement.AttemptID != s.DispatchAttempt.AttemptID || s.ExecuteEnforcement.AttemptID != s.DispatchAttempt.AttemptID || s.PrepareEnforcement.ReviewAuthorization != s.Authorization || s.ExecuteEnforcement.ReviewAuthorization != s.Authorization || s.PrepareEnforcement.SandboxAttempt != s.SandboxAttempt || s.ExecuteEnforcement.SandboxAttempt != s.SandboxAttempt || s.Admission.IntentRevision != s.DispatchAttempt.IntentRevision || s.Assembly.ID == "" {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence scope mixes governance, execution or assembly coordinates")
	}
	return nil
}

func (s CheckpointRestoreEvidenceScopeV1) DigestV1() (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", CheckpointRestoreEvidenceContractVersionV1, "CheckpointRestoreEvidenceScopeV1", s)
}

type CheckpointRestoreEvidenceConsumptionStateV1 string

const (
	CheckpointEvidenceConsumedCurrentV1     CheckpointRestoreEvidenceConsumptionStateV1 = "consumed_current"
	CheckpointEvidenceConsumedObservationV1 CheckpointRestoreEvidenceConsumptionStateV1 = "consumed_observation"
)

type CheckpointRestoreEvidenceQualificationRefV1 struct {
	ID              string                                     `json:"qualification_id"`
	Revision        core.Revision                              `json:"revision"`
	Attempt         CheckpointAttemptRefV2                     `json:"attempt"`
	Barrier         CheckpointBarrierLeaseRefV2                `json:"barrier"`
	EffectCut       EffectCutRefV2                             `json:"effect_cut"`
	Reservation     CheckpointParticipantPhaseReservationRefV2 `json:"reservation"`
	Phase           CheckpointParticipantPhaseV2               `json:"phase"`
	ScopeDigest     core.Digest                                `json:"scope_digest"`
	Digest          core.Digest                                `json:"digest"`
	ExpiresUnixNano int64                                      `json:"expires_unix_nano"`
}

func (r CheckpointRestoreEvidenceQualificationRefV1) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision == 0 || r.Attempt.Validate() != nil || r.Barrier.Validate() != nil || r.EffectCut.Validate() != nil || r.Reservation.Validate() != nil || !validCheckpointPhaseV2(r.Phase) || r.ScopeDigest.Validate() != nil || r.ExpiresUnixNano <= 0 {
		return checkpointInvalidV2("checkpoint Evidence qualification ref is incomplete")
	}
	if r.Attempt.TenantID != r.Barrier.TenantID || r.Attempt.ID != r.Barrier.AttemptID || r.EffectCut.Attempt.TenantID != r.Attempt.TenantID || r.EffectCut.Attempt.ID != r.Attempt.ID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence qualification ref mixes Attempt, Barrier or Effect Cut")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Evidence qualification ref digest drifted")
	}
	return nil
}

func (r CheckpointRestoreEvidenceQualificationRefV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", CheckpointRestoreEvidenceContractVersionV1, "CheckpointRestoreEvidenceQualificationRefV1", copy)
}

type CheckpointRestoreEvidenceProviderHandoffRefV1 struct {
	ID            string                                      `json:"handoff_id"`
	Revision      core.Revision                               `json:"revision"`
	Qualification CheckpointRestoreEvidenceQualificationRefV1 `json:"qualification"`
	Attempt       OperationDispatchAttemptRefV3               `json:"operation_attempt"`
	Phase         CheckpointParticipantPhaseV2                `json:"phase"`
	ScopeDigest   core.Digest                                 `json:"scope_digest"`
	Digest        core.Digest                                 `json:"digest"`
}

func (r CheckpointRestoreEvidenceProviderHandoffRefV1) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision != 1 || r.Qualification.Validate() != nil || r.Attempt.Validate() != nil || !validCheckpointPhaseV2(r.Phase) || r.ScopeDigest.Validate() != nil || r.Phase != r.Qualification.Phase || r.ScopeDigest != r.Qualification.ScopeDigest {
		return checkpointInvalidV2("checkpoint Evidence handoff ref is incomplete")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Evidence handoff ref digest drifted")
	}
	return nil
}

func (r CheckpointRestoreEvidenceProviderHandoffRefV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", CheckpointRestoreEvidenceContractVersionV1, "CheckpointRestoreEvidenceProviderHandoffRefV1", copy)
}

type CheckpointRestoreEvidenceConsumptionRefV1 struct {
	ID            string                                        `json:"consumption_id"`
	Revision      core.Revision                                 `json:"revision"`
	Qualification CheckpointRestoreEvidenceQualificationRefV1   `json:"qualification"`
	Handoff       CheckpointRestoreEvidenceProviderHandoffRefV1 `json:"handoff"`
	Record        EvidenceRecordRefV2                           `json:"record"`
	Attempt       CheckpointAttemptRefV2                        `json:"attempt"`
	Phase         CheckpointParticipantPhaseV2                  `json:"phase"`
	State         CheckpointRestoreEvidenceConsumptionStateV1   `json:"state"`
	ScopeDigest   core.Digest                                   `json:"scope_digest"`
	Source        EvidenceSourceKeyV2                           `json:"source"`
	Digest        core.Digest                                   `json:"digest"`
}

func (r CheckpointRestoreEvidenceConsumptionRefV1) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision != 1 || r.Qualification.Validate() != nil || r.Handoff.Validate() != nil || r.Record.Validate() != nil || r.Attempt.Validate() != nil || !validCheckpointPhaseV2(r.Phase) || r.ScopeDigest.Validate() != nil || r.Source.Validate() != nil || (r.State != CheckpointEvidenceConsumedCurrentV1 && r.State != CheckpointEvidenceConsumedObservationV1) {
		return checkpointInvalidV2("checkpoint Evidence consumption ref is incomplete")
	}
	if r.Attempt.ID != r.Qualification.Attempt.ID || r.Phase != r.Qualification.Phase || r.Handoff.Qualification != r.Qualification || r.ScopeDigest != r.Qualification.ScopeDigest || r.ScopeDigest != r.Handoff.ScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence closure mixes attempts or phases")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Evidence consumption ref digest drifted")
	}
	return nil
}

func (r CheckpointRestoreEvidenceConsumptionRefV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", CheckpointRestoreEvidenceContractVersionV1, "CheckpointRestoreEvidenceConsumptionRefV1", copy)
}

type CheckpointEvidenceSourceCurrentProjectionV1 struct {
	Source           EvidenceSourceKeyV2               `json:"source"`
	Policy           OperationScopeEvidencePolicyRefV3 `json:"policy"`
	Schema           SchemaRefV2                       `json:"schema"`
	Current          bool                              `json:"current"`
	CheckedUnixNano  int64                             `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                             `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                       `json:"projection_digest"`
}

func (p CheckpointEvidenceSourceCurrentProjectionV1) Validate(now time.Time) error {
	if p.Source.Validate() != nil || p.Policy.Validate() != nil || p.Schema.Validate() != nil || !p.Current || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceSourceStale, "checkpoint Evidence source is not current")
	}
	copy := p
	copy.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", CheckpointRestoreEvidenceContractVersionV1, "CheckpointEvidenceSourceCurrentProjectionV1", copy)
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Evidence source current projection drifted")
	}
	return nil
}

func SealCheckpointEvidenceSourceCurrentProjectionV1(p CheckpointEvidenceSourceCurrentProjectionV1, now time.Time) (CheckpointEvidenceSourceCurrentProjectionV1, error) {
	p.ProjectionDigest = ""
	copy := p
	digest, err := core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", CheckpointRestoreEvidenceContractVersionV1, "CheckpointEvidenceSourceCurrentProjectionV1", copy)
	if err != nil {
		return CheckpointEvidenceSourceCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type CheckpointEvidenceSourceCurrentReaderV1 interface {
	InspectCheckpointEvidenceSourceCurrentV1(context.Context, EvidenceSourceKeyV2) (CheckpointEvidenceSourceCurrentProjectionV1, error)
}

// CheckpointEvidenceExecutionCurrentProjectionV1 is the narrow read-only
// projection supplied by the Runtime dispatch/enforcement Owners before a
// checkpoint Evidence qualification is issued. It does not grant execution.
type CheckpointEvidenceExecutionCurrentProjectionV1 struct {
	Operation                 OperationSubjectV3                     `json:"operation"`
	OperationDigest           core.Digest                            `json:"operation_digest"`
	EffectID                  core.EffectIntentID                    `json:"effect_id"`
	EffectRevision            core.Revision                          `json:"effect_revision"`
	IntentRevision            core.Revision                          `json:"intent_revision"`
	IntentDigest              core.Digest                            `json:"intent_digest"`
	DispatchAttempt           OperationDispatchAttemptRefV3          `json:"dispatch_attempt"`
	PermitID                  string                                 `json:"permit_id"`
	PermitFactRevision        core.Revision                          `json:"permit_fact_revision"`
	PermitDigest              core.Digest                            `json:"permit_digest"`
	AuthorizedAdmissionDigest core.Digest                            `json:"authorized_admission_digest"`
	Authorization             OperationReviewAuthorizationRefV4      `json:"authorization"`
	PrepareEnforcement        OperationDispatchEnforcementPhaseRefV4 `json:"prepare_enforcement"`
	ExecuteEnforcement        OperationDispatchEnforcementPhaseRefV4 `json:"execute_enforcement"`
	SandboxAttempt            OperationDispatchSandboxFactRefV4      `json:"sandbox_attempt"`
	SandboxProjectionDigest   core.Digest                            `json:"sandbox_projection_digest"`
	SandboxLease              core.SandboxLeaseRef                   `json:"sandbox_lease"`
	FenceEpoch                core.Epoch                             `json:"fence_epoch"`
	PayloadSchema             SchemaRefV2                            `json:"payload_schema"`
	PayloadDigest             core.Digest                            `json:"payload_digest"`
	PayloadRevision           core.Revision                          `json:"payload_revision"`
	Current                   bool                                   `json:"current"`
	CheckedUnixNano           int64                                  `json:"checked_unix_nano"`
	ExpiresUnixNano           int64                                  `json:"expires_unix_nano"`
	ProjectionDigest          core.Digest                            `json:"projection_digest"`
}

func (p CheckpointEvidenceExecutionCurrentProjectionV1) Validate(now time.Time) error {
	if p.Operation.Validate() != nil || p.OperationDigest.Validate() != nil || p.EffectID == "" || p.EffectRevision == 0 || p.IntentRevision == 0 || p.IntentDigest.Validate() != nil || p.DispatchAttempt.Validate() != nil || !validCheckpointIDV2(p.PermitID) || p.PermitFactRevision == 0 || p.PermitDigest.Validate() != nil || p.AuthorizedAdmissionDigest.Validate() != nil || p.Authorization.Validate() != nil || p.PrepareEnforcement.Validate() != nil || p.ExecuteEnforcement.Validate() != nil || p.SandboxAttempt.Validate() != nil || p.SandboxProjectionDigest.Validate() != nil || p.SandboxLease.Validate() != nil || p.FenceEpoch == 0 || p.PayloadSchema.Validate() != nil || p.PayloadDigest.Validate() != nil || p.PayloadRevision == 0 || !p.Current || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "checkpoint Evidence execution closure is not current")
	}
	operationDigest, err := p.Operation.DigestV3()
	if err != nil || operationDigest != p.OperationDigest || p.DispatchAttempt.OperationDigest != p.OperationDigest || p.DispatchAttempt.EffectID != p.EffectID || p.DispatchAttempt.IntentRevision != p.IntentRevision || p.DispatchAttempt.IntentDigest != p.IntentDigest || p.DispatchAttempt.PermitID != p.PermitID || p.PrepareEnforcement.PermitID != p.PermitID || p.ExecuteEnforcement.PermitID != p.PermitID || p.PrepareEnforcement.PermitFactRevision != p.PermitFactRevision || p.ExecuteEnforcement.PermitFactRevision != p.PermitFactRevision || p.PrepareEnforcement.PermitDigest != p.PermitDigest || p.ExecuteEnforcement.PermitDigest != p.PermitDigest || p.PrepareEnforcement.AdmissionDigest != p.AuthorizedAdmissionDigest || p.ExecuteEnforcement.AdmissionDigest != p.AuthorizedAdmissionDigest || p.PrepareEnforcement.ReviewAuthorization != p.Authorization || p.ExecuteEnforcement.ReviewAuthorization != p.Authorization || p.PrepareEnforcement.SandboxAttempt != p.SandboxAttempt || p.ExecuteEnforcement.SandboxAttempt != p.SandboxAttempt || p.SandboxLease.Epoch != p.FenceEpoch {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence execution closure mixes Permit, Enforcement or Sandbox coordinates")
	}
	copy := p
	copy.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", CheckpointRestoreEvidenceContractVersionV1, "CheckpointEvidenceExecutionCurrentProjectionV1", copy)
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Evidence execution closure digest drifted")
	}
	return nil
}

func SealCheckpointEvidenceExecutionCurrentProjectionV1(p CheckpointEvidenceExecutionCurrentProjectionV1, now time.Time) (CheckpointEvidenceExecutionCurrentProjectionV1, error) {
	p.ProjectionDigest = ""
	copy := p
	digest, err := core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", CheckpointRestoreEvidenceContractVersionV1, "CheckpointEvidenceExecutionCurrentProjectionV1", copy)
	if err != nil {
		return CheckpointEvidenceExecutionCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type CheckpointEvidenceExecutionCurrentReaderV1 interface {
	InspectCheckpointEvidenceExecutionCurrentV1(context.Context, OperationSubjectV3, core.EffectIntentID, OperationDispatchAttemptRefV3) (CheckpointEvidenceExecutionCurrentProjectionV1, error)
}

type IssueCheckpointPhaseQualificationRequestV1 struct {
	ID              string                                     `json:"qualification_id"`
	Attempt         CheckpointAttemptRefV2                     `json:"attempt"`
	Barrier         CheckpointBarrierLeaseRefV2                `json:"barrier"`
	EffectCut       EffectCutRefV2                             `json:"effect_cut"`
	Reservation     CheckpointParticipantPhaseReservationRefV2 `json:"reservation"`
	Phase           CheckpointParticipantPhaseV2               `json:"phase"`
	Scope           CheckpointRestoreEvidenceScopeV1           `json:"scope"`
	ExpiresUnixNano int64                                      `json:"expires_unix_nano"`
}

func (r IssueCheckpointPhaseQualificationRequestV1) Validate(now time.Time) error {
	if !validCheckpointIDV2(r.ID) || r.Attempt.Validate() != nil || r.Barrier.Validate() != nil || r.EffectCut.Validate() != nil || r.Reservation.Validate() != nil || !validCheckpointPhaseV2(r.Phase) || r.Scope.Validate() != nil || r.ExpiresUnixNano < 0 || now.IsZero() || (r.ExpiresUnixNano > 0 && !now.Before(time.Unix(0, r.ExpiresUnixNano))) {
		return checkpointInvalidV2("checkpoint Evidence qualification request is incomplete or expired")
	}
	if r.Attempt.TenantID != r.Barrier.TenantID || r.Attempt.ID != r.Barrier.AttemptID || r.EffectCut.Attempt.TenantID != r.Attempt.TenantID || r.EffectCut.Attempt.ID != r.Attempt.ID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence request mixes Attempt, Barrier or Effect Cut")
	}
	return nil
}

// CreateCheckpointPhaseQualificationOwnerRequestV1 is the Evidence Owner
// write primitive. DerivedExpiresUnixNano is computed by the Runtime gateway
// from current Owner projections; a caller supplied expiry is only an exact
// expectation and cannot extend it.
type CreateCheckpointPhaseQualificationOwnerRequestV1 struct {
	Request                IssueCheckpointPhaseQualificationRequestV1 `json:"request"`
	DerivedExpiresUnixNano int64                                      `json:"derived_expires_unix_nano"`
}

func (r CreateCheckpointPhaseQualificationOwnerRequestV1) Validate(now time.Time) error {
	request := r.Request
	request.ExpiresUnixNano = r.DerivedExpiresUnixNano
	if r.DerivedExpiresUnixNano <= 0 || request.Validate(now) != nil {
		return checkpointInvalidV2("checkpoint Evidence Owner request is incomplete or expired")
	}
	if r.Request.ExpiresUnixNano != 0 && r.Request.ExpiresUnixNano != r.DerivedExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence expected expiry differs from the Owner-derived bound")
	}
	return nil
}

type CheckpointRestoreEvidenceQualificationFactV1 struct {
	ContractVersion string                                      `json:"contract_version"`
	Ref             CheckpointRestoreEvidenceQualificationRefV1 `json:"ref"`
	Request         IssueCheckpointPhaseQualificationRequestV1  `json:"request"`
	CreatedUnixNano int64                                       `json:"created_unix_nano"`
}

func (f CheckpointRestoreEvidenceQualificationFactV1) Validate() error {
	scopeDigest, err := f.Request.Scope.DigestV1()
	created := time.Unix(0, f.CreatedUnixNano)
	if f.ContractVersion != CheckpointRestoreEvidenceContractVersionV1 || f.Ref.Validate() != nil || f.CreatedUnixNano <= 0 || f.Request.Validate(created) != nil || time.Duration(f.Ref.ExpiresUnixNano-f.CreatedUnixNano) > MaxCheckpointRestoreEvidenceQualificationTTLV1 || f.Ref.ID != f.Request.ID || f.Ref.Attempt != f.Request.Attempt || f.Ref.Barrier != f.Request.Barrier || f.Ref.EffectCut != f.Request.EffectCut || f.Ref.Reservation != f.Request.Reservation || f.Ref.Phase != f.Request.Phase || f.Ref.ScopeDigest != scopeDigest || f.Ref.ExpiresUnixNano != f.Request.ExpiresUnixNano || err != nil {
		return checkpointInvalidV2("checkpoint Evidence qualification fact is incomplete")
	}
	return nil
}

type CheckpointRestoreEvidenceQualificationCurrentProjectionV1 struct {
	Ref              CheckpointRestoreEvidenceQualificationRefV1 `json:"ref"`
	Current          bool                                        `json:"current"`
	CheckedUnixNano  int64                                       `json:"checked_unix_nano"`
	ProjectionDigest core.Digest                                 `json:"projection_digest"`
	Scope            CheckpointRestoreEvidenceScopeV1            `json:"scope"`
}

func (p CheckpointRestoreEvidenceQualificationCurrentProjectionV1) Validate(now time.Time) error {
	scopeDigest, scopeErr := p.Scope.DigestV1()
	if p.Ref.Validate() != nil || p.Scope.Validate() != nil || scopeErr != nil || scopeDigest != p.Ref.ScopeDigest || !p.Current || p.CheckedUnixNano <= 0 || p.ProjectionDigest.Validate() != nil || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.Ref.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "checkpoint Evidence qualification is not current")
	}
	copy := p
	copy.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", CheckpointRestoreEvidenceContractVersionV1, "CheckpointRestoreEvidenceQualificationCurrentProjectionV1", copy)
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Evidence current projection drifted")
	}
	return nil
}

type CreateCheckpointPhaseProviderHandoffRequestV1 struct {
	ID            string                                      `json:"handoff_id"`
	Qualification CheckpointRestoreEvidenceQualificationRefV1 `json:"qualification"`
	Attempt       OperationDispatchAttemptRefV3               `json:"operation_attempt"`
	Phase         CheckpointParticipantPhaseV2                `json:"phase"`
	ScopeDigest   core.Digest                                 `json:"scope_digest"`
}

func (r CreateCheckpointPhaseProviderHandoffRequestV1) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Qualification.Validate() != nil || r.Attempt.Validate() != nil || !validCheckpointPhaseV2(r.Phase) || r.ScopeDigest.Validate() != nil || r.Phase != r.Qualification.Phase || r.ScopeDigest != r.Qualification.ScopeDigest {
		return checkpointInvalidV2("create checkpoint Evidence handoff request is incomplete")
	}
	return nil
}

type ConsumeCheckpointPhaseEvidenceRequestV1 struct {
	ID            string                                        `json:"consumption_id"`
	Qualification CheckpointRestoreEvidenceQualificationRefV1   `json:"qualification"`
	Handoff       CheckpointRestoreEvidenceProviderHandoffRefV1 `json:"handoff"`
	Record        EvidenceRecordRefV2                           `json:"record"`
	Source        EvidenceSourceKeyV2                           `json:"source"`
}

func (r ConsumeCheckpointPhaseEvidenceRequestV1) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Qualification.Validate() != nil || r.Handoff.Validate() != nil || r.Record.Validate() != nil || r.Source.Validate() != nil || r.Handoff.Qualification != r.Qualification {
		return checkpointInvalidV2("consume checkpoint Evidence request is incomplete")
	}
	return nil
}

type CheckpointRestoreEvidenceHandoffCurrentProjectionV1 struct {
	Ref              CheckpointRestoreEvidenceProviderHandoffRefV1 `json:"ref"`
	Current          bool                                          `json:"current"`
	CheckedUnixNano  int64                                         `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                         `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                                   `json:"projection_digest"`
}

func (p CheckpointRestoreEvidenceHandoffCurrentProjectionV1) Validate(now time.Time) error {
	if p.Ref.Validate() != nil || !p.Current || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "checkpoint Evidence handoff is not current")
	}
	copy := p
	copy.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", CheckpointRestoreEvidenceContractVersionV1, "CheckpointRestoreEvidenceHandoffCurrentProjectionV1", copy)
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence handoff projection drifted")
	}
	return nil
}

type CheckpointRestoreEvidenceConsumptionCurrentProjectionV1 struct {
	Ref              CheckpointRestoreEvidenceConsumptionRefV1 `json:"ref"`
	Current          bool                                      `json:"current"`
	CheckedUnixNano  int64                                     `json:"checked_unix_nano"`
	ProjectionDigest core.Digest                               `json:"projection_digest"`
}

func (p CheckpointRestoreEvidenceConsumptionCurrentProjectionV1) Validate() error {
	if p.Ref.Validate() != nil || p.Ref.State != CheckpointEvidenceConsumedCurrentV1 || !p.Current || p.CheckedUnixNano <= 0 || p.ProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "checkpoint Evidence consumption is not current")
	}
	copy := p
	copy.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", CheckpointRestoreEvidenceContractVersionV1, "CheckpointRestoreEvidenceConsumptionCurrentProjectionV1", copy)
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence consumption projection drifted")
	}
	return nil
}

type CheckpointRestoreEvidenceGovernancePortV1 interface {
	IssueCheckpointPhaseQualificationV1(context.Context, IssueCheckpointPhaseQualificationRequestV1) (CheckpointRestoreEvidenceQualificationRefV1, error)
	InspectCheckpointPhaseQualificationHistoricalV1(context.Context, CheckpointRestoreEvidenceQualificationRefV1) (CheckpointRestoreEvidenceQualificationFactV1, error)
	InspectCheckpointPhaseQualificationCurrentV1(context.Context, CheckpointRestoreEvidenceQualificationRefV1) (CheckpointRestoreEvidenceQualificationCurrentProjectionV1, error)
	CreateCheckpointPhaseProviderHandoffV1(context.Context, CreateCheckpointPhaseProviderHandoffRequestV1) (CheckpointRestoreEvidenceProviderHandoffRefV1, error)
	InspectCheckpointPhaseProviderHandoffHistoricalV1(context.Context, CheckpointRestoreEvidenceProviderHandoffRefV1) (CheckpointRestoreEvidenceProviderHandoffRefV1, error)
	InspectCheckpointPhaseProviderHandoffCurrentV1(context.Context, CheckpointRestoreEvidenceProviderHandoffRefV1) (CheckpointRestoreEvidenceHandoffCurrentProjectionV1, error)
	ConsumeCheckpointPhaseEvidenceCurrentV1(context.Context, ConsumeCheckpointPhaseEvidenceRequestV1) (CheckpointRestoreEvidenceConsumptionRefV1, error)
	ConsumeCheckpointPhaseEvidenceObservationV1(context.Context, ConsumeCheckpointPhaseEvidenceRequestV1) (CheckpointRestoreEvidenceConsumptionRefV1, error)
	InspectCheckpointPhaseEvidenceConsumptionHistoricalV1(context.Context, CheckpointRestoreEvidenceConsumptionRefV1) (CheckpointRestoreEvidenceConsumptionRefV1, error)
	InspectCheckpointPhaseEvidenceConsumptionCurrentV1(context.Context, CheckpointRestoreEvidenceConsumptionRefV1) (CheckpointRestoreEvidenceConsumptionCurrentProjectionV1, error)
}

// CheckpointRestoreEvidenceFactPortV1 is an Owner-only write surface. It is
// intentionally separate from the application-facing governance port.
type CheckpointRestoreEvidenceFactPortV1 interface {
	CreateCheckpointPhaseQualificationFactV1(context.Context, CreateCheckpointPhaseQualificationOwnerRequestV1) (CheckpointRestoreEvidenceQualificationRefV1, error)
	InspectCheckpointPhaseQualificationHistoricalV1(context.Context, CheckpointRestoreEvidenceQualificationRefV1) (CheckpointRestoreEvidenceQualificationFactV1, error)
	InspectCheckpointPhaseQualificationCurrentV1(context.Context, CheckpointRestoreEvidenceQualificationRefV1) (CheckpointRestoreEvidenceQualificationCurrentProjectionV1, error)
	CreateCheckpointPhaseProviderHandoffV1(context.Context, CreateCheckpointPhaseProviderHandoffRequestV1) (CheckpointRestoreEvidenceProviderHandoffRefV1, error)
	InspectCheckpointPhaseProviderHandoffHistoricalV1(context.Context, CheckpointRestoreEvidenceProviderHandoffRefV1) (CheckpointRestoreEvidenceProviderHandoffRefV1, error)
	InspectCheckpointPhaseProviderHandoffCurrentV1(context.Context, CheckpointRestoreEvidenceProviderHandoffRefV1) (CheckpointRestoreEvidenceHandoffCurrentProjectionV1, error)
	ConsumeCheckpointPhaseEvidenceCurrentV1(context.Context, ConsumeCheckpointPhaseEvidenceRequestV1) (CheckpointRestoreEvidenceConsumptionRefV1, error)
	ConsumeCheckpointPhaseEvidenceObservationV1(context.Context, ConsumeCheckpointPhaseEvidenceRequestV1) (CheckpointRestoreEvidenceConsumptionRefV1, error)
	InspectCheckpointPhaseEvidenceConsumptionHistoricalV1(context.Context, CheckpointRestoreEvidenceConsumptionRefV1) (CheckpointRestoreEvidenceConsumptionRefV1, error)
	InspectCheckpointPhaseEvidenceConsumptionCurrentV1(context.Context, CheckpointRestoreEvidenceConsumptionRefV1) (CheckpointRestoreEvidenceConsumptionCurrentProjectionV1, error)
}

type CheckpointEvidenceAttemptCurrentProjectionV1 struct {
	Attempt          CheckpointAttemptRefV2      `json:"attempt"`
	Barrier          CheckpointBarrierLeaseRefV2 `json:"barrier"`
	EffectCut        EffectCutRefV2              `json:"effect_cut"`
	Current          bool                        `json:"current"`
	CheckedUnixNano  int64                       `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                       `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                 `json:"projection_digest"`
}

func (p CheckpointEvidenceAttemptCurrentProjectionV1) Validate(now time.Time) error {
	if p.Attempt.Validate() != nil || p.Barrier.Validate() != nil || p.EffectCut.Validate() != nil || !p.Current || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() || p.Attempt.TenantID != p.Barrier.TenantID || p.Attempt.ID != p.Barrier.AttemptID || p.EffectCut.Attempt.TenantID != p.Attempt.TenantID || p.EffectCut.Attempt.ID != p.Attempt.ID || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "checkpoint Evidence Attempt closure is not current")
	}
	copy := p
	copy.ProjectionDigest = ""
	digest, err := core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", CheckpointRestoreEvidenceContractVersionV1, "CheckpointEvidenceAttemptCurrentProjectionV1", copy)
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Evidence Attempt closure drifted")
	}
	return nil
}

func SealCheckpointEvidenceAttemptCurrentProjectionV1(p CheckpointEvidenceAttemptCurrentProjectionV1, now time.Time) (CheckpointEvidenceAttemptCurrentProjectionV1, error) {
	p.ProjectionDigest = ""
	copy := p
	digest, err := core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-evidence", CheckpointRestoreEvidenceContractVersionV1, "CheckpointEvidenceAttemptCurrentProjectionV1", copy)
	if err != nil {
		return CheckpointEvidenceAttemptCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

// CheckpointEvidenceAttemptCurrentReaderV1 is the minimal currentness surface
// needed by the Evidence gateway. The Checkpoint governance gateway satisfies
// it without exposing its Fact Owner mutation port.
type CheckpointEvidenceAttemptCurrentReaderV1 interface {
	InspectCheckpointEvidenceAttemptCurrentV1(context.Context, CheckpointAttemptRefV2, CheckpointBarrierLeaseRefV2, EffectCutRefV2) (CheckpointEvidenceAttemptCurrentProjectionV1, error)
}
