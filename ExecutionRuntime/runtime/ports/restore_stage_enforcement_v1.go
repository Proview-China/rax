package ports

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const RestoreStageEnforcementContractVersionV1 = "1.0.0"

// RestoreStagePreparedAttemptRefV1 is the Sandbox Owner's exact prepared
// workspace materialization attempt. It is not a Provider result or authority.
type RestoreStagePreparedAttemptRefV1 struct {
	SandboxAttempt   OperationDispatchSandboxFactRefV4 `json:"sandbox_attempt"`
	OperationDigest  core.Digest                       `json:"operation_digest"`
	EffectID         core.EffectIntentID               `json:"effect_id"`
	IntentRevision   core.Revision                     `json:"intent_revision"`
	IntentDigest     core.Digest                       `json:"intent_digest"`
	DispatchAttempt  OperationDispatchAttemptRefV3     `json:"dispatch_attempt"`
	Provider         ProviderBindingRefV2              `json:"provider"`
	BundleDigest     core.Digest                       `json:"bundle_digest"`
	PreparedUnixNano int64                             `json:"prepared_unix_nano"`
	ExpiresUnixNano  int64                             `json:"expires_unix_nano"`
	Digest           core.Digest                       `json:"digest"`
}

func (r RestoreStagePreparedAttemptRefV1) DigestV1() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.restore-stage-enforcement", RestoreStageEnforcementContractVersionV1, "RestoreStagePreparedAttemptRefV1", copy)
}

func SealRestoreStagePreparedAttemptRefV1(r RestoreStagePreparedAttemptRefV1) (RestoreStagePreparedAttemptRefV1, error) {
	r.Digest = ""
	digest, err := r.DigestV1()
	if err != nil {
		return RestoreStagePreparedAttemptRefV1{}, err
	}
	r.Digest = digest
	return r, r.Validate()
}

func (r RestoreStagePreparedAttemptRefV1) Validate() error {
	if r.SandboxAttempt.Validate() != nil || r.OperationDigest.Validate() != nil || r.EffectID == "" || r.IntentRevision == 0 || r.IntentDigest.Validate() != nil || r.DispatchAttempt.Validate() != nil || r.Provider.Validate() != nil || r.BundleDigest.Validate() != nil || r.PreparedUnixNano <= 0 || r.ExpiresUnixNano <= r.PreparedUnixNano || r.Digest.Validate() != nil || r.DispatchAttempt.OperationDigest != r.OperationDigest || r.DispatchAttempt.EffectID != r.EffectID || r.DispatchAttempt.IntentRevision != r.IntentRevision || r.DispatchAttempt.IntentDigest != r.IntentDigest || r.DispatchAttempt.AttemptID != r.SandboxAttempt.ID || r.ExpiresUnixNano > r.SandboxAttempt.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "Restore Stage prepared attempt is incomplete or crosses Runtime dispatch")
	}
	digest, err := r.DigestV1()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Stage prepared attempt digest drifted")
	}
	return nil
}

type RestoreStageSandboxCurrentProjectionV1 struct {
	ContractVersion        string                            `json:"contract_version"`
	Operation              OperationSubjectV3                `json:"operation"`
	OperationDigest        core.Digest                       `json:"operation_digest"`
	EffectID               core.EffectIntentID               `json:"effect_id"`
	IntentRevision         core.Revision                     `json:"intent_revision"`
	IntentDigest           core.Digest                       `json:"intent_digest"`
	DispatchAttempt        OperationDispatchAttemptRefV3     `json:"dispatch_attempt"`
	SandboxAttempt         OperationDispatchSandboxFactRefV4 `json:"sandbox_attempt"`
	RestoreAttempt         RestoreAttemptRefV2               `json:"restore_attempt"`
	Eligibility            RestoreEligibilityRefV2           `json:"eligibility"`
	Identity               RestoreIdentityReservationV2      `json:"identity"`
	SnapshotArtifact       CheckpointExternalExactFactRefV2  `json:"snapshot_artifact"`
	BundleProjectionDigest core.Digest                       `json:"bundle_projection_digest"`
	BundleDigest           core.Digest                       `json:"bundle_digest"`
	Provider               ProviderBindingRefV2              `json:"provider"`
	Prepared               RestoreStagePreparedAttemptRefV1  `json:"prepared"`
	Current                bool                              `json:"current"`
	CheckedUnixNano        int64                             `json:"checked_unix_nano"`
	ExpiresUnixNano        int64                             `json:"expires_unix_nano"`
	ProjectionDigest       core.Digest                       `json:"projection_digest"`
}

func (p RestoreStageSandboxCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.restore-stage-enforcement", RestoreStageEnforcementContractVersionV1, "RestoreStageSandboxCurrentProjectionV1", copy)
}

func SealRestoreStageSandboxCurrentProjectionV1(p RestoreStageSandboxCurrentProjectionV1, now time.Time) (RestoreStageSandboxCurrentProjectionV1, error) {
	p.ContractVersion = RestoreStageEnforcementContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return RestoreStageSandboxCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.ValidateCurrent(now)
}

// Validate checks the immutable sealed projection without claiming that its
// natural TTL is still current at a caller's wall clock.
func (p RestoreStageSandboxCurrentProjectionV1) Validate() error {
	if p.ContractVersion != RestoreStageEnforcementContractVersionV1 || p.Operation.Validate() != nil || p.OperationDigest.Validate() != nil || p.EffectID == "" || p.IntentRevision == 0 || p.IntentDigest.Validate() != nil || p.DispatchAttempt.Validate() != nil || p.SandboxAttempt.Validate() != nil || p.RestoreAttempt.Validate() != nil || p.Eligibility.Validate() != nil || p.Identity.Validate() != nil || p.SnapshotArtifact.Validate() != nil || p.BundleProjectionDigest.Validate() != nil || p.BundleDigest.Validate() != nil || p.Provider.Validate() != nil || p.Prepared.Validate() != nil || !p.Current || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "Restore Stage Sandbox current projection is incomplete or stale")
	}
	digest, err := p.Operation.DigestV3()
	if err != nil || digest != p.OperationDigest || p.Operation.Kind != RestoreStageOperationKindV1 || p.Operation.CustomOperationID != p.RestoreAttempt.ID || p.Operation.ExecutionScope.Identity.TenantID != p.RestoreAttempt.TenantID || p.RestoreAttempt.TenantID != p.Eligibility.TenantID || p.DispatchAttempt.OperationDigest != p.OperationDigest || p.DispatchAttempt.EffectID != p.EffectID || p.DispatchAttempt.IntentRevision != p.IntentRevision || p.DispatchAttempt.IntentDigest != p.IntentDigest || p.DispatchAttempt.AttemptID != p.SandboxAttempt.ID || p.Prepared.SandboxAttempt != p.SandboxAttempt || p.Prepared.DispatchAttempt != p.DispatchAttempt || p.Prepared.Provider != p.Provider || p.Prepared.BundleDigest != p.BundleDigest || p.Operation.ExecutionScope.Instance != p.Identity.TargetInstance || p.Operation.ExecutionScope.SandboxLease == nil || *p.Operation.ExecutionScope.SandboxLease != p.Identity.TargetLease || p.Operation.ExecutionScope.AuthorityEpoch != p.Identity.TargetFenceEpoch {
		return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage Sandbox current projection exact closure drifted")
	}
	if p.ExpiresUnixNano > p.SandboxAttempt.ExpiresUnixNano || p.ExpiresUnixNano > p.Prepared.ExpiresUnixNano || p.ExpiresUnixNano > p.Eligibility.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonBindingExpired, "Restore Stage Sandbox projection extends an upstream TTL")
	}
	projectionDigest, err := p.DigestV1()
	if err != nil || projectionDigest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Stage Sandbox projection digest drifted")
	}
	return nil
}

func (p RestoreStageSandboxCurrentProjectionV1) ValidateCurrent(now time.Time) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if now.IsZero() || now.UnixNano() < p.CheckedUnixNano || now.UnixNano() >= p.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "Restore Stage Sandbox current projection is outside its natural current window")
	}
	return nil
}

type InspectRestoreStageSandboxCurrentRequestV1 struct {
	Operation        OperationSubjectV3                `json:"operation"`
	EffectID         core.EffectIntentID               `json:"effect_id"`
	IntentRevision   core.Revision                     `json:"intent_revision"`
	IntentDigest     core.Digest                       `json:"intent_digest"`
	DispatchAttempt  OperationDispatchAttemptRefV3     `json:"dispatch_attempt"`
	SandboxAttempt   OperationDispatchSandboxFactRefV4 `json:"sandbox_attempt"`
	RestoreAttempt   RestoreAttemptRefV2               `json:"restore_attempt"`
	Eligibility      RestoreEligibilityRefV2           `json:"eligibility"`
	Identity         RestoreIdentityReservationV2      `json:"identity"`
	SnapshotArtifact CheckpointExternalExactFactRefV2  `json:"snapshot_artifact"`
	Provider         ProviderBindingRefV2              `json:"provider"`
}

func (r InspectRestoreStageSandboxCurrentRequestV1) Validate() error {
	if r.Operation.Validate() != nil || r.EffectID == "" || r.IntentRevision == 0 || r.IntentDigest.Validate() != nil || r.DispatchAttempt.Validate() != nil || r.SandboxAttempt.Validate() != nil || r.RestoreAttempt.Validate() != nil || r.Eligibility.Validate() != nil || r.Identity.Validate() != nil || r.SnapshotArtifact.Validate() != nil || r.Provider.Validate() != nil || r.DispatchAttempt.EffectID != r.EffectID || r.DispatchAttempt.IntentRevision != r.IntentRevision || r.DispatchAttempt.IntentDigest != r.IntentDigest || r.DispatchAttempt.AttemptID != r.SandboxAttempt.ID || r.Operation.ExecutionScope.Instance != r.Identity.TargetInstance || r.Operation.ExecutionScope.SandboxLease == nil || *r.Operation.ExecutionScope.SandboxLease != r.Identity.TargetLease || r.Operation.ExecutionScope.AuthorityEpoch != r.Identity.TargetFenceEpoch {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRestoreIncompatible, "Restore Stage Sandbox current request is incomplete")
	}
	return nil
}

type RestoreStageSandboxCurrentReaderV1 interface {
	InspectRestoreStageSandboxCurrentV1(context.Context, InspectRestoreStageSandboxCurrentRequestV1) (RestoreStageSandboxCurrentProjectionV1, error)
}

type EnforceRestoreStageDispatchRequestV1 struct {
	Operation                  OperationSubjectV3                      `json:"operation"`
	EffectID                   core.EffectIntentID                     `json:"effect_id"`
	PermitID                   string                                  `json:"permit_id"`
	ExpectedPermitFactRevision core.Revision                           `json:"expected_permit_fact_revision"`
	PermitDigest               core.Digest                             `json:"permit_digest"`
	AdmissionDigest            core.Digest                             `json:"admission_digest"`
	ReviewAuthorization        OperationReviewAuthorizationRefV4       `json:"review_authorization"`
	DispatchAttempt            OperationDispatchAttemptRefV3           `json:"dispatch_attempt"`
	SandboxAttempt             OperationDispatchSandboxFactRefV4       `json:"sandbox_attempt"`
	SandboxProjectionDigest    core.Digest                             `json:"sandbox_projection_digest"`
	RestoreAttempt             RestoreAttemptRefV2                     `json:"restore_attempt"`
	Eligibility                RestoreEligibilityRefV2                 `json:"eligibility"`
	Identity                   RestoreIdentityReservationV2            `json:"identity"`
	SnapshotArtifact           CheckpointExternalExactFactRefV2        `json:"snapshot_artifact"`
	Verifier                   ProviderBindingRefV2                    `json:"verifier"`
	Phase                      OperationDispatchEnforcementPhaseV4     `json:"phase"`
	ExpectedJournalRevision    core.Revision                           `json:"expected_journal_revision"`
	Prepare                    *OperationDispatchEnforcementPhaseRefV4 `json:"prepare,omitempty"`
	Prepared                   *RestoreStagePreparedAttemptRefV1       `json:"prepared,omitempty"`
}

func (r EnforceRestoreStageDispatchRequestV1) Validate() error {
	if r.Operation.Validate() != nil || r.EffectID == "" || r.PermitID == "" || r.ExpectedPermitFactRevision == 0 || r.PermitDigest.Validate() != nil || r.AdmissionDigest.Validate() != nil || r.ReviewAuthorization.Validate() != nil || r.DispatchAttempt.Validate() != nil || r.SandboxAttempt.Validate() != nil || r.SandboxProjectionDigest.Validate() != nil || r.RestoreAttempt.Validate() != nil || r.Eligibility.Validate() != nil || r.Identity.Validate() != nil || r.SnapshotArtifact.Validate() != nil || r.Verifier.Validate() != nil || r.DispatchAttempt.EffectID != r.EffectID || r.DispatchAttempt.PermitID != r.PermitID || r.DispatchAttempt.AttemptID != r.SandboxAttempt.ID || r.Operation.ExecutionScope.Instance != r.Identity.TargetInstance || r.Operation.ExecutionScope.SandboxLease == nil || *r.Operation.ExecutionScope.SandboxLease != r.Identity.TargetLease || r.Operation.ExecutionScope.AuthorityEpoch != r.Identity.TargetFenceEpoch {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "Restore Stage enforcement request is incomplete")
	}
	switch r.Phase {
	case OperationDispatchEnforcementPrepareV4:
		if r.ExpectedJournalRevision != 0 || r.Prepare != nil || r.Prepared != nil {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Restore Stage prepare enforcement carries later refs")
		}
	case OperationDispatchEnforcementExecuteV4:
		if r.ExpectedJournalRevision != 1 || r.Prepare == nil || r.Prepared == nil || r.Prepare.Validate() != nil || r.Prepared.Validate() != nil {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Restore Stage execute enforcement lacks exact prepare refs")
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "Restore Stage enforcement phase is invalid")
	}
	return nil
}

type RestoreStageEnforcementGovernancePortV1 interface {
	EnforceRestoreStageDispatchV1(context.Context, EnforceRestoreStageDispatchRequestV1) (OperationDispatchEnforcementPhaseRefV4, error)
	// InspectRestoreStageDispatchEnforcementByRequestV1 recovers only the
	// original exact enforcement attempt after an unknown/lost reply. It never
	// creates a journal revision and must reject changed request content.
	InspectRestoreStageDispatchEnforcementByRequestV1(context.Context, EnforceRestoreStageDispatchRequestV1) (OperationDispatchEnforcementPhaseRefV4, error)
	InspectCurrentRestoreStageDispatchEnforcementV1(context.Context, OperationSubjectV3, OperationDispatchEnforcementPhaseRefV4) (OperationDispatchEnforcementPhaseRefV4, error)
}

type RestoreStageEnforcementJournalV1 struct {
	ContractVersion         string                                  `json:"contract_version"`
	Operation               OperationSubjectV3                      `json:"operation"`
	OperationDigest         core.Digest                             `json:"operation_digest"`
	EffectID                core.EffectIntentID                     `json:"effect_id"`
	PermitID                string                                  `json:"permit_id"`
	SandboxAttempt          OperationDispatchSandboxFactRefV4       `json:"sandbox_attempt"`
	SandboxProjectionDigest core.Digest                             `json:"sandbox_projection_digest"`
	Sandbox                 RestoreStageSandboxCurrentProjectionV1  `json:"sandbox"`
	Prepare                 *OperationDispatchEnforcementPhaseRefV4 `json:"prepare,omitempty"`
	Execute                 *OperationDispatchEnforcementPhaseRefV4 `json:"execute,omitempty"`
	Revision                core.Revision                           `json:"revision"`
	Digest                  core.Digest                             `json:"digest"`
}

func (j RestoreStageEnforcementJournalV1) DigestV1() (core.Digest, error) {
	copy := j
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.restore-stage-enforcement", RestoreStageEnforcementContractVersionV1, "RestoreStageEnforcementJournalV1", copy)
}

func SealRestoreStageEnforcementJournalV1(j RestoreStageEnforcementJournalV1) (RestoreStageEnforcementJournalV1, error) {
	j.ContractVersion = RestoreStageEnforcementContractVersionV1
	j.Digest = ""
	digest, err := j.DigestV1()
	if err != nil {
		return RestoreStageEnforcementJournalV1{}, err
	}
	j.Digest = digest
	return j, j.Validate()
}

func (j RestoreStageEnforcementJournalV1) Validate() error {
	if j.ContractVersion != RestoreStageEnforcementContractVersionV1 || j.Operation.Validate() != nil || j.OperationDigest.Validate() != nil || j.EffectID == "" || j.PermitID == "" || j.SandboxAttempt.Validate() != nil || j.SandboxProjectionDigest.Validate() != nil || j.Sandbox.Validate() != nil || j.Sandbox.ProjectionDigest != j.SandboxProjectionDigest || j.Sandbox.SandboxAttempt != j.SandboxAttempt || !SameOperationSubjectV3(j.Sandbox.Operation, j.Operation) || j.Sandbox.EffectID != j.EffectID || j.Revision == 0 || j.Digest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "Restore Stage enforcement journal is incomplete")
	}
	opDigest, err := j.Operation.DigestV3()
	if err != nil || opDigest != j.OperationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Restore Stage enforcement journal operation drifted")
	}
	same := func(ref *OperationDispatchEnforcementPhaseRefV4, phase OperationDispatchEnforcementPhaseV4) bool {
		return ref != nil && ref.Validate() == nil && ref.OperationDigest == j.OperationDigest && ref.EffectID == j.EffectID && ref.PermitID == j.PermitID && ref.SandboxAttempt == j.SandboxAttempt && ref.Phase == phase
	}
	switch j.Revision {
	case 1:
		if !same(j.Prepare, OperationDispatchEnforcementPrepareV4) || j.Execute != nil {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Restore Stage enforcement revision one is not prepare-only")
		}
	case 2:
		if !same(j.Prepare, OperationDispatchEnforcementPrepareV4) || !same(j.Execute, OperationDispatchEnforcementExecuteV4) || j.Execute.PrepareReceiptDigest != j.Prepare.ReceiptDigest {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Restore Stage enforcement revision two lacks exact execute closure")
		}
	default:
		return core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "Restore Stage enforcement journal revision is unsupported")
	}
	digest, err := j.DigestV1()
	if err != nil || digest != j.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Restore Stage enforcement journal digest drifted")
	}
	return nil
}
