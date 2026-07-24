package ports

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const CheckpointRestoreDispatchSandboxCurrentContractVersionV1 = "1.0.0"

type CheckpointRestoreDispatchSandboxStageV1 string

const (
	CheckpointRestoreDispatchSandboxPrePrepareV1 CheckpointRestoreDispatchSandboxStageV1 = "pre_prepare"
	CheckpointRestoreDispatchSandboxPreExecuteV1 CheckpointRestoreDispatchSandboxStageV1 = "pre_execute"
)

type CheckpointRestoreDispatchWatermarkV1 struct {
	SourceID    string     `json:"source_id"`
	SourceEpoch core.Epoch `json:"source_epoch"`
	Sequence    uint64     `json:"sequence"`
}

func (w CheckpointRestoreDispatchWatermarkV1) Validate() error {
	if !validCheckpointIDV2(w.SourceID) || w.SourceEpoch == 0 || w.Sequence == 0 {
		return checkpointInvalidV2("checkpoint dispatch watermark is incomplete")
	}
	return nil
}

// CheckpointRestoreDispatchSandboxCurrentProjectionV1 is the Runtime-owned,
// Sandbox-supplied neutral projection for checkpoint Provider actual-point
// enforcement. It is not a Permit, an Enforcement receipt, Evidence, or a
// Sandbox domain Fact. The Runtime dispatch Attempt and Sandbox phase
// Reservation remain distinct exact coordinates.
type CheckpointRestoreDispatchSandboxCurrentProjectionV1 struct {
	ContractVersion    string                                                   `json:"contract_version"`
	Operation          OperationSubjectV3                                       `json:"operation"`
	OperationDigest    core.Digest                                              `json:"operation_digest"`
	EffectID           core.EffectIntentID                                      `json:"effect_id"`
	IntentRevision     core.Revision                                            `json:"intent_revision"`
	IntentDigest       core.Digest                                              `json:"intent_digest"`
	Reservation        CheckpointParticipantPhaseReservationCurrentProjectionV2 `json:"reservation_current"`
	SandboxReservation OperationDispatchSandboxFactRefV4                        `json:"sandbox_reservation"`
	Participant        OperationDispatchSandboxFactRefV4                        `json:"participant_current"`
	DispatchAttempt    OperationDispatchSandboxFactRefV4                        `json:"runtime_dispatch_attempt"`
	RuntimeLease       OperationDispatchRuntimeLeaseBindingV4                   `json:"runtime_lease_binding"`
	Requirement        OperationDispatchSandboxFactRefV4                        `json:"requirement_current"`
	Policy             OperationDispatchSandboxFactRefV4                        `json:"policy_current"`
	Workspace          OperationDispatchSandboxFactRefV4                        `json:"workspace_current"`
	ChangeSet          *OperationDispatchSandboxFactRefV4                       `json:"change_set_current,omitempty"`
	Placement          OperationDispatchSandboxFactRefV4                        `json:"placement_current"`
	Backend            OperationDispatchSandboxFactRefV4                        `json:"backend_current"`
	Slot               OperationDispatchSandboxFactRefV4                        `json:"slot_current"`
	Generation         GenerationBindingAssociationRefV1                        `json:"generation_binding"`
	Verifier           ProviderBindingRefV2                                     `json:"verifier"`
	Stage              CheckpointRestoreDispatchSandboxStageV1                  `json:"stage"`
	PrepareEnforcement *OperationDispatchEnforcementPhaseRefV4                  `json:"prepare_enforcement,omitempty"`
	PreparedAttempt    *PreparedProviderAttemptRefV2                            `json:"prepared_attempt,omitempty"`
	Watermarks         []CheckpointRestoreDispatchWatermarkV1                   `json:"watermarks"`
	Current            bool                                                     `json:"current"`
	ProjectionRevision core.Revision                                            `json:"projection_revision"`
	CheckedUnixNano    int64                                                    `json:"checked_unix_nano"`
	ExpiresUnixNano    int64                                                    `json:"expires_unix_nano"`
	ProjectionDigest   core.Digest                                              `json:"projection_digest"`
}

func (p CheckpointRestoreDispatchSandboxCurrentProjectionV1) Validate(now time.Time) error {
	if p.ContractVersion != CheckpointRestoreDispatchSandboxCurrentContractVersionV1 || p.Operation.Validate() != nil || p.EffectID == "" || p.IntentRevision == 0 || p.Reservation.Validate(now) != nil || p.SandboxReservation.Validate() != nil || p.Participant.Validate() != nil || p.DispatchAttempt.Validate() != nil || p.RuntimeLease.Validate() != nil || p.Requirement.Validate() != nil || p.Policy.Validate() != nil || p.Workspace.Validate() != nil || p.Placement.Validate() != nil || p.Backend.Validate() != nil || p.Slot.Validate() != nil || p.Generation.Validate() != nil || p.Verifier.Validate() != nil || !p.Current || p.ProjectionRevision == 0 || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonProviderBindingStale, "checkpoint Sandbox dispatch projection is incomplete or stale")
	}
	operationDigest, err := p.Operation.DigestV3()
	if err != nil || operationDigest != p.OperationDigest || !SameOperationSubjectV3(p.Reservation.Operation, p.Operation) || p.Reservation.OperationDigest != p.OperationDigest || p.Reservation.EffectID != p.EffectID || p.Reservation.Ref.ID != p.SandboxReservation.ID || p.Reservation.Ref.Revision != p.SandboxReservation.Revision || p.Reservation.Ref.Digest != p.SandboxReservation.Digest || p.Reservation.Ref.ExpiresUnixNano != p.SandboxReservation.ExpiresUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Sandbox dispatch operation or Reservation drifted")
	}
	if p.Reservation.Attempt.TenantID != p.Operation.ExecutionScope.Identity.TenantID || p.Reservation.Barrier.AttemptID != p.Reservation.Attempt.ID || p.Reservation.EffectCut.Attempt != p.Reservation.Attempt || p.Operation.ExecutionScope.SandboxLease == nil || *p.Operation.ExecutionScope.SandboxLease != p.RuntimeLease.Lease || p.RuntimeLease.Instance != p.Operation.ExecutionScope.Instance || p.RuntimeLease.FenceEpoch != p.Operation.ExecutionScope.AuthorityEpoch || p.RuntimeLease.ScopeDigest != p.Operation.ExecutionScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "checkpoint Sandbox dispatch Attempt, Lease, Fence or Scope drifted")
	}
	if err := p.IntentDigest.Validate(); err != nil {
		return err
	}
	if p.ChangeSet != nil && p.ChangeSet.Validate() != nil {
		return checkpointInvalidV2("checkpoint Sandbox dispatch ChangeSet ref is invalid")
	}
	if len(p.Watermarks) == 0 || !slices.IsSortedFunc(p.Watermarks, func(a, b CheckpointRestoreDispatchWatermarkV1) int { return strings.Compare(a.SourceID, b.SourceID) }) {
		return checkpointInvalidV2("checkpoint Sandbox dispatch watermarks are empty or unsorted")
	}
	for index, watermark := range p.Watermarks {
		if watermark.Validate() != nil || index > 0 && p.Watermarks[index-1].SourceID == watermark.SourceID {
			return checkpointInvalidV2("checkpoint Sandbox dispatch watermarks are invalid or duplicated")
		}
	}
	switch p.Stage {
	case CheckpointRestoreDispatchSandboxPrePrepareV1:
		if p.PrepareEnforcement != nil || p.PreparedAttempt != nil {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "checkpoint pre-prepare projection carries later execution facts")
		}
	case CheckpointRestoreDispatchSandboxPreExecuteV1:
		if p.PrepareEnforcement == nil || p.PreparedAttempt == nil || p.PrepareEnforcement.Validate() != nil || p.PreparedAttempt.Validate() != nil || p.PrepareEnforcement.Phase != OperationDispatchEnforcementPrepareV4 || p.PrepareEnforcement.OperationDigest != p.OperationDigest || p.PrepareEnforcement.EffectID != p.EffectID || p.PrepareEnforcement.SandboxAttempt != p.DispatchAttempt || p.PreparedAttempt.OperationDigest != p.OperationDigest || p.PreparedAttempt.IntentID != p.EffectID || p.PreparedAttempt.IntentRevision != p.IntentRevision || p.PreparedAttempt.IntentDigest != p.IntentDigest || p.PreparedAttempt.AttemptID != p.DispatchAttempt.ID || p.PreparedAttempt.Provider != p.Verifier {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "checkpoint pre-execute projection lacks the exact prepared closure")
		}
	default:
		return checkpointInvalidV2("checkpoint Sandbox dispatch stage is invalid")
	}
	expires := []int64{p.Reservation.ExpiresUnixNano, p.SandboxReservation.ExpiresUnixNano, p.Participant.ExpiresUnixNano, p.DispatchAttempt.ExpiresUnixNano, p.RuntimeLease.Ref.ExpiresUnixNano, p.Requirement.ExpiresUnixNano, p.Policy.ExpiresUnixNano, p.Workspace.ExpiresUnixNano, p.Placement.ExpiresUnixNano, p.Backend.ExpiresUnixNano, p.Slot.ExpiresUnixNano}
	if p.ChangeSet != nil {
		expires = append(expires, p.ChangeSet.ExpiresUnixNano)
	}
	if p.PrepareEnforcement != nil {
		expires = append(expires, p.PrepareEnforcement.ExpiresUnixNano)
	}
	if p.PreparedAttempt != nil {
		expires = append(expires, p.PreparedAttempt.ExpiresUnixNano)
	}
	for _, upper := range expires {
		if p.ExpiresUnixNano > upper {
			return core.NewError(core.ErrorConflict, core.ReasonBindingExpired, "checkpoint Sandbox dispatch projection extends an upstream TTL")
		}
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Sandbox dispatch projection digest drifted")
	}
	return nil
}

func (p CheckpointRestoreDispatchSandboxCurrentProjectionV1) DigestV1() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-dispatch-sandbox-current", CheckpointRestoreDispatchSandboxCurrentContractVersionV1, "CheckpointRestoreDispatchSandboxCurrentProjectionV1", copy)
}

func SealCheckpointRestoreDispatchSandboxCurrentProjectionV1(p CheckpointRestoreDispatchSandboxCurrentProjectionV1, now time.Time) (CheckpointRestoreDispatchSandboxCurrentProjectionV1, error) {
	p.ContractVersion = CheckpointRestoreDispatchSandboxCurrentContractVersionV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

func (p CheckpointRestoreDispatchSandboxCurrentProjectionV1) ValidateCurrent(operation OperationSubjectV3, effectID core.EffectIntentID, reservation CheckpointParticipantPhaseReservationRefV2, stage CheckpointRestoreDispatchSandboxStageV1, now time.Time) error {
	if err := p.Validate(now); err != nil {
		return err
	}
	if !SameOperationSubjectV3(p.Operation, operation) || p.EffectID != effectID || p.Reservation.Ref != reservation || p.Stage != stage {
		return core.NewError(core.ErrorConflict, core.ReasonProviderBindingStale, "checkpoint Sandbox dispatch projection binds another exact request")
	}
	return nil
}

type CheckpointRestoreDispatchSandboxCurrentReaderV1 interface {
	InspectCheckpointRestoreDispatchSandboxCurrentV1(context.Context, OperationSubjectV3, core.EffectIntentID, CheckpointParticipantPhaseReservationRefV2) (CheckpointRestoreDispatchSandboxCurrentProjectionV1, error)
}

type EnforceCurrentCheckpointRestoreDispatchRequestV1 struct {
	Operation                  OperationSubjectV3                         `json:"operation"`
	EffectID                   core.EffectIntentID                        `json:"effect_id"`
	PermitID                   string                                     `json:"permit_id"`
	ExpectedPermitFactRevision core.Revision                              `json:"expected_permit_fact_revision"`
	PermitDigest               core.Digest                                `json:"permit_digest"`
	AdmissionDigest            core.Digest                                `json:"admission_digest"`
	ReviewAuthorization        OperationReviewAuthorizationRefV4          `json:"review_authorization"`
	AttemptID                  string                                     `json:"attempt_id"`
	Phase                      OperationDispatchEnforcementPhaseV4        `json:"phase"`
	Reservation                CheckpointParticipantPhaseReservationRefV2 `json:"checkpoint_reservation"`
	SandboxProjectionDigest    core.Digest                                `json:"sandbox_projection_digest"`
	Verifier                   ProviderBindingRefV2                       `json:"verifier"`
	ExpectedJournalRevision    core.Revision                              `json:"expected_journal_revision"`
	Prepare                    *OperationDispatchEnforcementPhaseRefV4    `json:"prepare,omitempty"`
	PreparedAttempt            *PreparedProviderAttemptRefV2              `json:"prepared_attempt,omitempty"`
}

func (r EnforceCurrentCheckpointRestoreDispatchRequestV1) Validate() error {
	if r.Operation.Validate() != nil || r.EffectID == "" || !validCheckpointIDV2(r.PermitID) || r.ExpectedPermitFactRevision == 0 || !validCheckpointIDV2(r.AttemptID) || r.Reservation.Validate() != nil || r.Verifier.Validate() != nil || r.ReviewAuthorization.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "checkpoint enforcement request identity is incomplete")
	}
	for _, digest := range []core.Digest{r.PermitDigest, r.AdmissionDigest, r.SandboxProjectionDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	switch r.Phase {
	case OperationDispatchEnforcementPrepareV4:
		if r.ExpectedJournalRevision != 0 || r.Prepare != nil || r.PreparedAttempt != nil {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "checkpoint prepare enforcement carries later phase watermarks")
		}
	case OperationDispatchEnforcementExecuteV4:
		if r.ExpectedJournalRevision != 1 || r.Prepare == nil || r.PreparedAttempt == nil || r.Prepare.Validate() != nil || r.PreparedAttempt.Validate() != nil {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "checkpoint execute enforcement lacks exact prepare watermarks")
		}
	default:
		return checkpointInvalidV2("checkpoint enforcement phase is invalid")
	}
	return nil
}

type InspectCurrentCheckpointRestoreDispatchRequestV1 struct {
	Operation               OperationSubjectV3                         `json:"operation"`
	EffectID                core.EffectIntentID                        `json:"effect_id"`
	PermitID                string                                     `json:"permit_id"`
	Phase                   OperationDispatchEnforcementPhaseV4        `json:"phase"`
	PermitDigest            core.Digest                                `json:"permit_digest"`
	AdmissionDigest         core.Digest                                `json:"admission_digest"`
	ReviewAuthorization     OperationReviewAuthorizationRefV4          `json:"review_authorization"`
	Reservation             CheckpointParticipantPhaseReservationRefV2 `json:"checkpoint_reservation"`
	SandboxProjectionDigest core.Digest                                `json:"sandbox_projection_digest"`
}

func (r InspectCurrentCheckpointRestoreDispatchRequestV1) Validate() error {
	if r.Operation.Validate() != nil || r.EffectID == "" || !validCheckpointIDV2(r.PermitID) || (r.Phase != OperationDispatchEnforcementPrepareV4 && r.Phase != OperationDispatchEnforcementExecuteV4) || r.ReviewAuthorization.Validate() != nil || r.Reservation.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonDispatchPermitInvalid, "checkpoint current enforcement Inspect is incomplete")
	}
	for _, digest := range []core.Digest{r.PermitDigest, r.AdmissionDigest, r.SandboxProjectionDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type CurrentCheckpointRestoreDispatchEnforcementV1 struct {
	Dispatch        CurrentOperationDispatchAuthorizationV4             `json:"dispatch_current"`
	Sandbox         CheckpointRestoreDispatchSandboxCurrentProjectionV1 `json:"checkpoint_sandbox_current"`
	Journal         OperationDispatchEnforcementJournalV4               `json:"journal"`
	Phase           OperationDispatchEnforcementPhaseRefV4              `json:"phase"`
	CheckedUnixNano int64                                               `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                               `json:"expires_unix_nano"`
	Digest          core.Digest                                         `json:"digest"`
}

func (e CurrentCheckpointRestoreDispatchEnforcementV1) Validate(now time.Time) error {
	if e.Dispatch.Validate() != nil || e.Sandbox.Validate(now) != nil || e.Journal.Validate() != nil || e.Phase.Validate() != nil || e.CheckedUnixNano <= 0 || e.ExpiresUnixNano <= e.CheckedUnixNano || now.IsZero() || now.UnixNano() < e.CheckedUnixNano || !now.Before(time.Unix(0, e.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "checkpoint current enforcement envelope is incomplete or stale")
	}
	expected, err := e.Journal.PhaseRefV4(e.Phase.Phase)
	if err != nil || expected != e.Phase || e.Phase.SandboxAttempt != e.Sandbox.DispatchAttempt || e.ExpiresUnixNano > e.Dispatch.Record.Permit.LegacyPermit.ExpiresUnixNano || e.ExpiresUnixNano > e.Sandbox.ExpiresUnixNano || e.Phase.PermitDigest != e.Dispatch.Record.PermitDigest || e.Phase.AdmissionDigest != e.Dispatch.Record.Permit.Admission.Digest || e.Phase.ReviewAuthorization != e.Dispatch.ReviewAuthorization {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "checkpoint current enforcement envelope coordinates drifted")
	}
	digest, err := e.DigestV1()
	if err != nil || digest != e.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint current enforcement envelope digest drifted")
	}
	return nil
}

func (e CurrentCheckpointRestoreDispatchEnforcementV1) DigestV1() (core.Digest, error) {
	copy := e
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.runtime.checkpoint-restore-dispatch-sandbox-current", CheckpointRestoreDispatchSandboxCurrentContractVersionV1, "CurrentCheckpointRestoreDispatchEnforcementV1", copy)
}

func SealCurrentCheckpointRestoreDispatchEnforcementV1(e CurrentCheckpointRestoreDispatchEnforcementV1, now time.Time) (CurrentCheckpointRestoreDispatchEnforcementV1, error) {
	e.Digest = ""
	digest, err := e.DigestV1()
	if err != nil {
		return CurrentCheckpointRestoreDispatchEnforcementV1{}, err
	}
	e.Digest = digest
	return e, e.Validate(now)
}

// CheckpointRestoreDispatchEnforcementGovernancePortV1 is the additive
// checkpoint actual-point gate. It never calls a Provider or writes Sandbox,
// Checkpoint, Lease, Fence, Evidence, Settlement, or Outcome facts.
type CheckpointRestoreDispatchEnforcementGovernancePortV1 interface {
	EnforceCurrentCheckpointRestoreDispatchV1(context.Context, EnforceCurrentCheckpointRestoreDispatchRequestV1) (CurrentCheckpointRestoreDispatchEnforcementV1, error)
	InspectCurrentCheckpointRestoreDispatchV1(context.Context, InspectCurrentCheckpointRestoreDispatchRequestV1) (CurrentCheckpointRestoreDispatchEnforcementV1, error)
}
