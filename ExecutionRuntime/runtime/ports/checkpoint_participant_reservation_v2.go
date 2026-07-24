package ports

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const CheckpointParticipantReservationContractVersionV2 = "2.0.0"

type CheckpointParticipantPhaseV2 string

const (
	CheckpointPhasePrepareV2 CheckpointParticipantPhaseV2 = "checkpoint_prepare"
	CheckpointPhaseCommitV2  CheckpointParticipantPhaseV2 = "checkpoint_commit"
	CheckpointPhaseAbortV2   CheckpointParticipantPhaseV2 = "checkpoint_abort"
)

type CheckpointParticipantPhaseStateV2 string

const (
	CheckpointParticipantReservedV2   CheckpointParticipantPhaseStateV2 = "reserved"
	CheckpointParticipantExecutingV2  CheckpointParticipantPhaseStateV2 = "executing"
	CheckpointParticipantPreparedV2   CheckpointParticipantPhaseStateV2 = "prepared"
	CheckpointParticipantCommittedV2  CheckpointParticipantPhaseStateV2 = "committed"
	CheckpointParticipantAbortedV2    CheckpointParticipantPhaseStateV2 = "aborted"
	CheckpointParticipantFailedV2     CheckpointParticipantPhaseStateV2 = "failed"
	CheckpointParticipantNotAppliedV2 CheckpointParticipantPhaseStateV2 = "not_applied"
	CheckpointParticipantUnknownV2    CheckpointParticipantPhaseStateV2 = "unknown"
)

type CheckpointParticipantRefV2 struct {
	ID     string               `json:"participant_id"`
	Owner  ProviderBindingRefV2 `json:"owner"`
	Digest core.Digest          `json:"digest"`
}

func (r CheckpointParticipantRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Owner.Validate() != nil {
		return checkpointInvalidV2("checkpoint Participant ref is incomplete")
	}
	return r.Digest.Validate()
}

type CheckpointParticipantPhaseReservationRefV2 struct {
	ID              string        `json:"reservation_id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func (r CheckpointParticipantPhaseReservationRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision == 0 || r.ExpiresUnixNano <= 0 {
		return checkpointInvalidV2("checkpoint Participant reservation ref is incomplete")
	}
	return r.Digest.Validate()
}

type CheckpointParticipantPhaseRefV2 struct {
	ID       string                            `json:"phase_fact_id"`
	Revision core.Revision                     `json:"revision"`
	Phase    CheckpointParticipantPhaseV2      `json:"phase"`
	State    CheckpointParticipantPhaseStateV2 `json:"state"`
	Digest   core.Digest                       `json:"digest"`
}

func (r CheckpointParticipantPhaseRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision == 0 || !validCheckpointPhaseV2(r.Phase) || !validCheckpointPhaseStateV2(r.State) {
		return checkpointInvalidV2("checkpoint Participant phase ref is incomplete")
	}
	return r.Digest.Validate()
}

type CheckpointParticipantDomainReservationRefV2 struct {
	ID       string        `json:"domain_reservation_id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

func (r CheckpointParticipantDomainReservationRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision == 0 {
		return checkpointInvalidV2("checkpoint domain reservation ref is incomplete")
	}
	return r.Digest.Validate()
}

type CheckpointParticipantDomainResultRefV2 struct {
	ID              string                       `json:"domain_result_id"`
	Revision        core.Revision                `json:"revision"`
	Kind            NamespacedNameV2             `json:"kind"`
	Attempt         CheckpointAttemptRefV2       `json:"attempt"`
	Participant     CheckpointParticipantRefV2   `json:"participant"`
	Phase           CheckpointParticipantPhaseV2 `json:"phase"`
	Operation       OperationSubjectV3           `json:"operation"`
	OperationDigest core.Digest                  `json:"operation_digest"`
	Digest          core.Digest                  `json:"digest"`
}

func (r CheckpointParticipantDomainResultRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision == 0 || ValidateNamespacedNameV2(r.Kind) != nil || r.Attempt.Validate() != nil || r.Participant.Validate() != nil || !validCheckpointPhaseV2(r.Phase) || r.Operation.Validate() != nil {
		return checkpointInvalidV2("checkpoint DomainResult ref is incomplete")
	}
	operationDigest, err := r.Operation.DigestV3()
	if err != nil || operationDigest != r.OperationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint DomainResult operation drifted")
	}
	return r.Digest.Validate()
}

type CheckpointParticipantApplySettlementRefV2 struct {
	ID           string                       `json:"apply_settlement_id"`
	Revision     core.Revision                `json:"revision"`
	Participant  CheckpointParticipantRefV2   `json:"participant"`
	Phase        CheckpointParticipantPhaseV2 `json:"phase"`
	SettlementID string                       `json:"settlement_id"`
	Digest       core.Digest                  `json:"digest"`
}

func (r CheckpointParticipantApplySettlementRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision == 0 || r.Participant.Validate() != nil || !validCheckpointPhaseV2(r.Phase) || !validCheckpointIDV2(r.SettlementID) {
		return checkpointInvalidV2("checkpoint ApplySettlement ref is incomplete")
	}
	return r.Digest.Validate()
}

type CheckpointParticipantPhaseClosureRefV2 struct {
	ID              string                                     `json:"closure_id"`
	Phase           CheckpointParticipantPhaseV2               `json:"phase"`
	Reservation     CheckpointParticipantPhaseReservationRefV2 `json:"reservation"`
	PhaseFact       CheckpointParticipantPhaseRefV2            `json:"phase_fact"`
	DomainResult    CheckpointParticipantDomainResultRefV2     `json:"domain_result"`
	Evidence        CheckpointRestoreEvidenceConsumptionRefV1  `json:"evidence"`
	Settlement      OperationCheckpointRestoreSettlementRefV5  `json:"settlement"`
	ApplySettlement CheckpointParticipantApplySettlementRefV2  `json:"apply_settlement"`
	Digest          core.Digest                                `json:"digest"`
}

func (r CheckpointParticipantPhaseClosureRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || !validCheckpointPhaseV2(r.Phase) || r.Reservation.Validate() != nil || r.PhaseFact.Validate() != nil || r.DomainResult.Validate() != nil || r.Evidence.Validate() != nil || r.Settlement.Validate() != nil || r.ApplySettlement.Validate() != nil || r.Digest.Validate() != nil {
		return checkpointInvalidV2("checkpoint Participant closure is incomplete")
	}
	if r.PhaseFact.Phase != r.Phase || r.DomainResult.Phase != r.Phase || r.ApplySettlement.Phase != r.Phase || r.Settlement.Phase != r.Phase || r.Evidence.Phase != r.Phase || r.ApplySettlement.SettlementID != r.Settlement.ID || r.DomainResult.Participant != r.ApplySettlement.Participant || r.DomainResult.Attempt != r.Evidence.Attempt || r.DomainResult.Attempt != r.Settlement.Attempt || r.DomainResult.OperationDigest != r.Settlement.OperationDigest || r.Evidence.Handoff.Attempt.EffectID != r.Settlement.EffectID {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Participant closure mixes phases or settlement")
	}
	digest, err := r.DigestV2()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Participant closure digest drifted")
	}
	return nil
}

func (r CheckpointParticipantPhaseClosureRefV2) DigestV2() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return checkpointDigestV2("CheckpointParticipantPhaseClosureRefV2", copy)
}

type CheckpointParticipantClosureRefV2 struct {
	ID          string                                  `json:"closure_id"`
	Participant CheckpointParticipantRefV2              `json:"participant"`
	Prepare     CheckpointParticipantPhaseClosureRefV2  `json:"prepare"`
	Terminal    *CheckpointParticipantPhaseClosureRefV2 `json:"terminal,omitempty"`
	Digest      core.Digest                             `json:"digest"`
}

func (r CheckpointParticipantClosureRefV2) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Participant.Validate() != nil || r.Prepare.Validate() != nil || r.Prepare.Phase != CheckpointPhasePrepareV2 || r.Digest.Validate() != nil {
		return checkpointInvalidV2("checkpoint Participant aggregate closure is incomplete")
	}
	if r.Terminal != nil {
		if err := r.Terminal.Validate(); err != nil {
			return err
		}
		if r.Terminal.Phase != CheckpointPhaseCommitV2 && r.Terminal.Phase != CheckpointPhaseAbortV2 {
			return checkpointInvalidV2("checkpoint terminal phase must be commit or abort")
		}
	}
	if r.Prepare.DomainResult.Participant != r.Participant || r.Prepare.ApplySettlement.Participant != r.Participant || (r.Terminal != nil && (r.Terminal.DomainResult.Participant != r.Participant || r.Terminal.ApplySettlement.Participant != r.Participant || r.Terminal.DomainResult.Attempt != r.Prepare.DomainResult.Attempt)) {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Participant aggregate closure mixes participants or attempts")
	}
	digest, err := r.DigestV2()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Participant aggregate closure digest drifted")
	}
	return nil
}

func (r CheckpointParticipantClosureRefV2) DigestV2() (core.Digest, error) {
	copy := r
	copy.Digest = ""
	return checkpointDigestV2("CheckpointParticipantClosureRefV2", copy)
}

type CheckpointParticipantBranchGuardRefV2 struct {
	TenantID      core.TenantID                `json:"tenant_id"`
	AttemptID     string                       `json:"attempt_id"`
	ParticipantID string                       `json:"participant_id"`
	SelectedPhase CheckpointParticipantPhaseV2 `json:"selected_phase"`
	Revision      core.Revision                `json:"revision"`
	Digest        core.Digest                  `json:"digest"`
}

func (r CheckpointParticipantBranchGuardRefV2) Validate() error {
	if strings.TrimSpace(string(r.TenantID)) == "" || !validCheckpointIDV2(r.AttemptID) || !validCheckpointIDV2(r.ParticipantID) || (r.SelectedPhase != CheckpointPhaseCommitV2 && r.SelectedPhase != CheckpointPhaseAbortV2) || r.Revision == 0 {
		return checkpointInvalidV2("checkpoint Participant branch guard is incomplete")
	}
	return r.Digest.Validate()
}

type CheckpointParticipantBranchGuardFactV2 struct {
	ContractVersion string                                 `json:"contract_version"`
	Ref             CheckpointParticipantBranchGuardRefV2  `json:"ref"`
	Attempt         CheckpointAttemptRefV2                 `json:"attempt"`
	Participant     CheckpointParticipantRefV2             `json:"participant"`
	Terminal        CheckpointParticipantPhaseClosureRefV2 `json:"terminal"`
	CreatedUnixNano int64                                  `json:"created_unix_nano"`
}

func (f CheckpointParticipantBranchGuardFactV2) Validate() error {
	if f.ContractVersion != CheckpointParticipantReservationContractVersionV2 || f.Ref.Validate() != nil || f.Attempt.Validate() != nil || f.Participant.Validate() != nil || f.Terminal.Validate() != nil || f.CreatedUnixNano <= 0 {
		return checkpointInvalidV2("checkpoint Participant branch guard fact is incomplete")
	}
	if f.Ref.TenantID != f.Attempt.TenantID || f.Ref.AttemptID != f.Attempt.ID || f.Ref.ParticipantID != f.Participant.ID || f.Ref.SelectedPhase != f.Terminal.Phase || (f.Terminal.Phase != CheckpointPhaseCommitV2 && f.Terminal.Phase != CheckpointPhaseAbortV2) || f.Terminal.DomainResult.Attempt != f.Attempt || f.Terminal.DomainResult.Participant != f.Participant || f.Terminal.ApplySettlement.Participant != f.Participant {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Participant branch guard binds another terminal branch")
	}
	digest, err := f.DigestV2()
	if err != nil || digest != f.Ref.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Participant branch guard digest drifted")
	}
	return nil
}

func (f CheckpointParticipantBranchGuardFactV2) DigestV2() (core.Digest, error) {
	copy := f
	copy.Ref.Digest = ""
	return checkpointDigestV2("CheckpointParticipantBranchGuardFactV2", copy)
}

func SealCheckpointParticipantBranchGuardFactV2(f CheckpointParticipantBranchGuardFactV2) (CheckpointParticipantBranchGuardFactV2, error) {
	f.ContractVersion = CheckpointParticipantReservationContractVersionV2
	f.Ref.Revision = 1
	f.Ref.Digest = ""
	digest, err := f.DigestV2()
	if err != nil {
		return CheckpointParticipantBranchGuardFactV2{}, err
	}
	f.Ref.Digest = digest
	return f, f.Validate()
}

type SelectCheckpointParticipantBranchRequestV2 struct {
	Attempt     CheckpointAttemptRefV2                 `json:"attempt"`
	Participant CheckpointParticipantRefV2             `json:"participant"`
	Terminal    CheckpointParticipantPhaseClosureRefV2 `json:"terminal"`
	SelectedAt  int64                                  `json:"selected_at_unix_nano"`
}

func (r SelectCheckpointParticipantBranchRequestV2) Validate() error {
	if r.Attempt.Validate() != nil || r.Participant.Validate() != nil || r.Terminal.Validate() != nil || r.SelectedAt <= 0 || (r.Terminal.Phase != CheckpointPhaseCommitV2 && r.Terminal.Phase != CheckpointPhaseAbortV2) {
		return checkpointInvalidV2("select checkpoint Participant branch request is incomplete")
	}
	if r.Terminal.DomainResult.Attempt != r.Attempt || r.Terminal.DomainResult.Participant != r.Participant || r.Terminal.ApplySettlement.Participant != r.Participant {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Participant branch selection mixes Attempt or Participant")
	}
	return nil
}

type CheckpointParticipantBranchGuardReaderV2 interface {
	InspectCheckpointParticipantBranchV2(context.Context, CheckpointParticipantBranchGuardRefV2) (CheckpointParticipantBranchGuardFactV2, error)
}

type CheckpointParticipantBranchGuardFactPortV2 interface {
	CheckpointParticipantBranchGuardReaderV2
	SelectCheckpointParticipantBranchV2(context.Context, SelectCheckpointParticipantBranchRequestV2) (CheckpointParticipantBranchGuardFactV2, error)
}

type CheckpointParticipantPhaseReservationCurrentProjectionV2 struct {
	ContractVersion  string                                      `json:"contract_version"`
	Ref              CheckpointParticipantPhaseReservationRefV2  `json:"ref"`
	Participant      CheckpointParticipantRefV2                  `json:"participant"`
	OwnerBinding     ProviderBindingRefV2                        `json:"owner_binding"`
	Phase            CheckpointParticipantPhaseV2                `json:"phase"`
	Attempt          CheckpointAttemptRefV2                      `json:"attempt"`
	Barrier          CheckpointBarrierLeaseRefV2                 `json:"barrier"`
	EffectCut        EffectCutRefV2                              `json:"effect_cut"`
	Operation        OperationSubjectV3                          `json:"operation"`
	OperationDigest  core.Digest                                 `json:"operation_digest"`
	EffectID         core.EffectIntentID                         `json:"effect_id"`
	EffectKind       EffectKindV2                                `json:"effect_kind"`
	IntentDigest     core.Digest                                 `json:"intent_digest"`
	PreviousPhase    *CheckpointParticipantPhaseClosureRefV2     `json:"previous_phase,omitempty"`
	Domain           CheckpointParticipantDomainReservationRefV2 `json:"domain"`
	Generation       GenerationBindingAssociationRefV1           `json:"generation"`
	CheckedUnixNano  int64                                       `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                       `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                                 `json:"projection_digest"`
}

func (p CheckpointParticipantPhaseReservationCurrentProjectionV2) Validate(now time.Time) error {
	if p.ContractVersion != CheckpointParticipantReservationContractVersionV2 || p.Ref.Validate() != nil || p.Participant.Validate() != nil || p.OwnerBinding.Validate() != nil || !validCheckpointPhaseV2(p.Phase) || p.Attempt.Validate() != nil || p.Barrier.Validate() != nil || p.EffectCut.Validate() != nil || p.Operation.Validate() != nil || p.EffectID == "" || ValidateNamespacedNameV2(NamespacedNameV2(p.EffectKind)) != nil || p.IntentDigest.Validate() != nil || p.Domain.Validate() != nil || p.Generation.Validate() != nil || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.IsZero() {
		return checkpointInvalidV2("checkpoint Participant current reservation is incomplete")
	}
	if now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) || p.Attempt.TenantID != p.Barrier.TenantID || p.Attempt.ID != p.Barrier.AttemptID {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "checkpoint Participant reservation is stale")
	}
	operationDigest, err := p.Operation.DigestV3()
	if err != nil || operationDigest != p.OperationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Participant operation drifted")
	}
	if p.Phase == CheckpointPhasePrepareV2 {
		if p.PreviousPhase != nil {
			return checkpointInvalidV2("checkpoint prepare cannot have a previous phase")
		}
	} else {
		if p.PreviousPhase == nil || p.PreviousPhase.Validate() != nil || p.PreviousPhase.Phase != CheckpointPhasePrepareV2 || p.PreviousPhase.PhaseFact.State != CheckpointParticipantPreparedV2 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "checkpoint commit or abort requires an exact prepared closure")
		}
	}
	digest, err := p.DigestV2()
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Participant current projection drifted")
	}
	return nil
}

func (p CheckpointParticipantPhaseReservationCurrentProjectionV2) DigestV2() (core.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return core.CanonicalJSONDigest("praxis.runtime.checkpoint-participant-reservation", CheckpointParticipantReservationContractVersionV2, "CheckpointParticipantPhaseReservationCurrentProjectionV2", copy)
}

type ReserveCheckpointParticipantPhaseRequestV2 struct {
	StableID        string                                  `json:"stable_id"`
	Participant     CheckpointParticipantRefV2              `json:"participant"`
	Phase           CheckpointParticipantPhaseV2            `json:"phase"`
	Attempt         CheckpointAttemptRefV2                  `json:"attempt"`
	Barrier         CheckpointBarrierLeaseRefV2             `json:"barrier"`
	EffectCut       EffectCutRefV2                          `json:"effect_cut"`
	PreviousPhase   *CheckpointParticipantPhaseClosureRefV2 `json:"previous_phase,omitempty"`
	ExpiresUnixNano int64                                   `json:"expires_unix_nano"`
}

func (r ReserveCheckpointParticipantPhaseRequestV2) Validate() error {
	if !validCheckpointIDV2(r.StableID) || r.Participant.Validate() != nil || !validCheckpointPhaseV2(r.Phase) || r.Attempt.Validate() != nil || r.Barrier.Validate() != nil || r.EffectCut.Validate() != nil || r.ExpiresUnixNano <= 0 {
		return checkpointInvalidV2("reserve checkpoint Participant phase request is incomplete")
	}
	if r.Phase == CheckpointPhasePrepareV2 {
		if r.PreviousPhase != nil {
			return checkpointInvalidV2("prepare reservation cannot carry PreviousPhase")
		}
		return nil
	}
	if r.PreviousPhase == nil || r.PreviousPhase.Validate() != nil || r.PreviousPhase.Phase != CheckpointPhasePrepareV2 || r.PreviousPhase.PhaseFact.State != CheckpointParticipantPreparedV2 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "successor reservation requires an exact prepared closure")
	}
	return nil
}

type CheckpointParticipantPhaseReservationFactV2 struct {
	Ref             CheckpointParticipantPhaseReservationRefV2 `json:"ref"`
	Request         ReserveCheckpointParticipantPhaseRequestV2 `json:"request"`
	CreatedUnixNano int64                                      `json:"created_unix_nano"`
}

func (f CheckpointParticipantPhaseReservationFactV2) Validate() error {
	if f.Ref.Validate() != nil || f.Request.Validate() != nil || f.CreatedUnixNano <= 0 || f.Ref.ExpiresUnixNano <= f.CreatedUnixNano {
		return checkpointInvalidV2("checkpoint Participant reservation fact is incomplete")
	}
	return nil
}

type CheckpointParticipantPhaseFactV2 struct {
	Ref             CheckpointParticipantPhaseRefV2            `json:"ref"`
	Reservation     CheckpointParticipantPhaseReservationRefV2 `json:"reservation"`
	PreviousPhase   *CheckpointParticipantPhaseClosureRefV2    `json:"previous_phase,omitempty"`
	UpdatedUnixNano int64                                      `json:"updated_unix_nano"`
}

type CheckpointParticipantPhaseCurrentProjectionV2 struct {
	Ref              CheckpointParticipantPhaseRefV2            `json:"ref"`
	Reservation      CheckpointParticipantPhaseReservationRefV2 `json:"reservation"`
	PreviousPhase    *CheckpointParticipantPhaseClosureRefV2    `json:"previous_phase,omitempty"`
	Current          bool                                       `json:"current"`
	CheckedUnixNano  int64                                      `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                      `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                                `json:"projection_digest"`
}

func (p CheckpointParticipantPhaseCurrentProjectionV2) Validate(now time.Time) error {
	if p.Ref.Validate() != nil || p.Reservation.Validate() != nil || !p.Current || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "checkpoint Participant phase is not current")
	}
	copy := p
	copy.ProjectionDigest = ""
	digest, err := checkpointDigestV2("CheckpointParticipantPhaseCurrentProjectionV2", copy)
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint Participant phase projection drifted")
	}
	return nil
}

func (f CheckpointParticipantPhaseFactV2) Validate() error {
	if f.Ref.Validate() != nil || f.Reservation.Validate() != nil || f.UpdatedUnixNano <= 0 {
		return checkpointInvalidV2("checkpoint Participant phase fact is incomplete")
	}
	return nil
}

type CheckpointParticipantDomainResultCurrentProjectionV2 struct {
	Ref              CheckpointParticipantDomainResultRefV2 `json:"ref"`
	Current          bool                                   `json:"current"`
	CheckedUnixNano  int64                                  `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                                  `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                            `json:"projection_digest"`
}

func (p CheckpointParticipantDomainResultCurrentProjectionV2) Validate(now time.Time) error {
	if p.Ref.Validate() != nil || !p.Current || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || p.ProjectionDigest.Validate() != nil || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "checkpoint DomainResult is not current")
	}
	copy := p
	copy.ProjectionDigest = ""
	digest, err := checkpointDigestV2("CheckpointParticipantDomainResultCurrentProjectionV2", copy)
	if err != nil || digest != p.ProjectionDigest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "checkpoint DomainResult projection drifted")
	}
	return nil
}

type ApplyCheckpointPhaseSettlementRequestV2 struct {
	Reservation      CheckpointParticipantPhaseReservationRefV2 `json:"reservation"`
	Settlement       OperationCheckpointRestoreSettlementRefV5  `json:"settlement"`
	ExpectedRevision core.Revision                              `json:"expected_revision"`
}

type CheckpointRestoreParticipantGovernancePortV2 interface {
	ReserveCheckpointPhaseV2(context.Context, ReserveCheckpointParticipantPhaseRequestV2) (CheckpointParticipantPhaseReservationRefV2, error)
	InspectCheckpointPhaseReservationHistoricalV2(context.Context, CheckpointParticipantPhaseReservationRefV2) (CheckpointParticipantPhaseReservationFactV2, error)
	InspectCheckpointPhaseV2(context.Context, CheckpointParticipantPhaseRefV2) (CheckpointParticipantPhaseFactV2, error)
	ReadCheckpointDomainResultCurrentV2(context.Context, CheckpointParticipantDomainResultRefV2) (CheckpointParticipantDomainResultCurrentProjectionV2, error)
	ApplyCheckpointPhaseSettlementV2(context.Context, ApplyCheckpointPhaseSettlementRequestV2) (CheckpointParticipantPhaseFactV2, error)
}

type CheckpointParticipantPhaseReservationCurrentReaderV2 interface {
	InspectCheckpointParticipantPhaseReservationCurrentV2(context.Context, CheckpointParticipantPhaseReservationRefV2, CheckpointParticipantPhaseV2) (CheckpointParticipantPhaseReservationCurrentProjectionV2, error)
}

type CheckpointParticipantPhaseCurrentReaderV2 interface {
	InspectCheckpointParticipantPhaseCurrentV2(context.Context, CheckpointParticipantPhaseRefV2) (CheckpointParticipantPhaseCurrentProjectionV2, error)
}

type CheckpointParticipantDomainResultCurrentReaderV2 interface {
	ReadCheckpointDomainResultCurrentV2(context.Context, CheckpointParticipantDomainResultRefV2) (CheckpointParticipantDomainResultCurrentProjectionV2, error)
}

func validCheckpointPhaseV2(phase CheckpointParticipantPhaseV2) bool {
	return phase == CheckpointPhasePrepareV2 || phase == CheckpointPhaseCommitV2 || phase == CheckpointPhaseAbortV2
}

func validCheckpointPhaseStateV2(state CheckpointParticipantPhaseStateV2) bool {
	switch state {
	case CheckpointParticipantReservedV2, CheckpointParticipantExecutingV2, CheckpointParticipantPreparedV2, CheckpointParticipantCommittedV2, CheckpointParticipantAbortedV2, CheckpointParticipantFailedV2, CheckpointParticipantNotAppliedV2, CheckpointParticipantUnknownV2:
		return true
	default:
		return false
	}
}
