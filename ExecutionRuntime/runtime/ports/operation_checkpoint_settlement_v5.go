package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const OperationCheckpointRestoreSettlementContractVersionV5 = "5.0.0"

type OperationCheckpointRestoreSettlementRefV5 struct {
	ID              string                       `json:"settlement_id"`
	Revision        core.Revision                `json:"revision"`
	TenantID        core.TenantID                `json:"tenant_id"`
	EffectID        core.EffectIntentID          `json:"effect_id"`
	Attempt         CheckpointAttemptRefV2       `json:"checkpoint_attempt"`
	Phase           CheckpointParticipantPhaseV2 `json:"phase"`
	OperationDigest core.Digest                  `json:"operation_digest"`
	Digest          core.Digest                  `json:"digest"`
}

func (r OperationCheckpointRestoreSettlementRefV5) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision != 1 || r.TenantID == "" || r.EffectID == "" || r.Attempt.Validate() != nil || !validCheckpointPhaseV2(r.Phase) || r.OperationDigest.Validate() != nil {
		return checkpointInvalidV2("checkpoint Operation Settlement V5 ref is incomplete")
	}
	return r.Digest.Validate()
}

type OperationCheckpointRestoreSettlementSubmissionV5 struct {
	ID                     string                                        `json:"settlement_id"`
	Operation              OperationSubjectV3                            `json:"operation"`
	OperationDigest        core.Digest                                   `json:"operation_digest"`
	EffectID               core.EffectIntentID                           `json:"effect_id"`
	ExpectedEffectRevision core.Revision                                 `json:"expected_effect_revision"`
	CheckpointAttempt      CheckpointAttemptRefV2                        `json:"checkpoint_attempt"`
	Phase                  CheckpointParticipantPhaseV2                  `json:"phase"`
	ParticipantFact        CheckpointParticipantPhaseRefV2               `json:"participant_fact"`
	Reservation            CheckpointParticipantPhaseReservationRefV2    `json:"reservation"`
	DomainResult           CheckpointParticipantDomainResultRefV2        `json:"domain_result"`
	Evidence               CheckpointRestoreEvidenceConsumptionRefV1     `json:"evidence"`
	Handoff                CheckpointRestoreEvidenceProviderHandoffRefV1 `json:"handoff"`
	DispatchAttempt        OperationDispatchAttemptRefV3                 `json:"dispatch_attempt"`
	Enforcement            OperationDispatchEnforcementPhaseRefV4        `json:"enforcement"`
	Owner                  ProviderBindingRefV2                          `json:"owner"`
	SettledUnixNano        int64                                         `json:"settled_unix_nano"`
}

func (s OperationCheckpointRestoreSettlementSubmissionV5) Validate() error {
	if !validCheckpointIDV2(s.ID) || s.Operation.Validate() != nil || s.EffectID == "" || s.ExpectedEffectRevision == 0 || s.CheckpointAttempt.Validate() != nil || !validCheckpointPhaseV2(s.Phase) || s.ParticipantFact.Validate() != nil || s.Reservation.Validate() != nil || s.DomainResult.Validate() != nil || s.Evidence.Validate() != nil || s.Handoff.Validate() != nil || s.DispatchAttempt.Validate() != nil || s.Enforcement.Validate() != nil || s.Owner.Validate() != nil || s.SettledUnixNano <= 0 {
		return checkpointInvalidV2("checkpoint Operation Settlement V5 submission is incomplete")
	}
	operationDigest, err := s.Operation.DigestV3()
	sameHandoffAttempt := sameCheckpointCanonicalV2("OperationDispatchAttemptRefV3", s.Handoff.Attempt, s.DispatchAttempt)
	if err != nil || operationDigest != s.OperationDigest || s.Operation.ExecutionScope.Identity.TenantID != s.CheckpointAttempt.TenantID || s.Evidence.State != CheckpointEvidenceConsumedCurrentV1 || s.Phase != s.ParticipantFact.Phase || s.Phase != s.DomainResult.Phase || s.Phase != s.Evidence.Phase || s.DomainResult.Attempt != s.CheckpointAttempt || s.Evidence.Attempt != s.CheckpointAttempt || s.Handoff != s.Evidence.Handoff || !sameHandoffAttempt {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Operation Settlement V5 closure drifted")
	}
	// DispatchAttempt is the immutable legacy V3 Permit coordinate while the
	// 4.1 Enforcement ref carries the V4 Permit Fact watermark. They must bind
	// one logical Permit/Attempt, but their revision/digest watermarks are
	// intentionally version-distinct.
	if !SameOperationSubjectV3(s.DomainResult.Operation, s.Operation) || s.DomainResult.OperationDigest != s.OperationDigest || s.DispatchAttempt.OperationDigest != s.OperationDigest || s.DispatchAttempt.EffectID != s.EffectID || s.Enforcement.Phase != OperationDispatchEnforcementExecuteV4 || s.Enforcement.OperationDigest != s.OperationDigest || s.Enforcement.EffectID != s.EffectID || s.Enforcement.PermitID != s.DispatchAttempt.PermitID || s.Enforcement.AttemptID != s.DispatchAttempt.AttemptID {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "checkpoint Operation Settlement V5 dispatch closure drifted")
	}
	return nil
}

type OperationCheckpointRestoreSettlementAssociationRefV5 struct {
	ID         string                                    `json:"association_id"`
	Revision   core.Revision                             `json:"revision"`
	Settlement OperationCheckpointRestoreSettlementRefV5 `json:"settlement"`
	Digest     core.Digest                               `json:"digest"`
}

func (r OperationCheckpointRestoreSettlementAssociationRefV5) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision != 1 || r.Settlement.Validate() != nil {
		return checkpointInvalidV2("checkpoint Settlement V5 association ref is incomplete")
	}
	return r.Digest.Validate()
}

type OperationCheckpointRestoreTerminalGuardRefV5 struct {
	TenantID   core.TenantID                             `json:"tenant_id"`
	EffectID   core.EffectIntentID                       `json:"effect_id"`
	Revision   core.Revision                             `json:"revision"`
	Settlement OperationCheckpointRestoreSettlementRefV5 `json:"settlement"`
	Digest     core.Digest                               `json:"digest"`
}

func (r OperationCheckpointRestoreTerminalGuardRefV5) Validate() error {
	if r.TenantID == "" || r.EffectID == "" || r.Revision != 1 || r.Settlement.Validate() != nil {
		return checkpointInvalidV2("checkpoint Settlement V5 guard ref is incomplete")
	}
	return r.Digest.Validate()
}

type OperationCheckpointRestoreTerminalProjectionRefV5 struct {
	ID         string                                    `json:"projection_id"`
	Revision   core.Revision                             `json:"revision"`
	Settlement OperationCheckpointRestoreSettlementRefV5 `json:"settlement"`
	Digest     core.Digest                               `json:"digest"`
}

type OperationCheckpointRestoreEffectTerminalRefV5 struct {
	TenantID         core.TenantID                             `json:"tenant_id"`
	EffectID         core.EffectIntentID                       `json:"effect_id"`
	PreviousRevision core.Revision                             `json:"previous_revision"`
	Revision         core.Revision                             `json:"revision"`
	OperationDigest  core.Digest                               `json:"operation_digest"`
	Settlement       OperationCheckpointRestoreSettlementRefV5 `json:"settlement"`
	Digest           core.Digest                               `json:"digest"`
}

func (r OperationCheckpointRestoreEffectTerminalRefV5) Validate() error {
	if r.TenantID == "" || r.EffectID == "" || r.PreviousRevision == 0 || r.Revision != r.PreviousRevision+1 || r.OperationDigest.Validate() != nil || r.Settlement.Validate() != nil || r.Digest.Validate() != nil || r.Settlement.TenantID != r.TenantID || r.Settlement.EffectID != r.EffectID || r.Settlement.OperationDigest != r.OperationDigest {
		return checkpointInvalidV2("checkpoint Settlement V5 terminal Effect ref is incomplete")
	}
	return nil
}

type OperationCheckpointRestoreEffectTerminalV5 struct {
	Ref               OperationCheckpointRestoreEffectTerminalRefV5 `json:"ref"`
	State             string                                        `json:"state"`
	PublishedUnixNano int64                                         `json:"published_unix_nano"`
}

func (v OperationCheckpointRestoreEffectTerminalV5) Validate() error {
	if v.Ref.Validate() != nil || v.State != "settled" || v.PublishedUnixNano <= 0 {
		return checkpointInvalidV2("checkpoint Settlement V5 terminal Effect is incomplete")
	}
	return nil
}

func (r OperationCheckpointRestoreTerminalProjectionRefV5) Validate() error {
	if !validCheckpointIDV2(r.ID) || r.Revision != 1 || r.Settlement.Validate() != nil {
		return checkpointInvalidV2("checkpoint Settlement V5 projection ref is incomplete")
	}
	return r.Digest.Validate()
}

type OperationCheckpointRestoreSettlementAssociationV5 struct {
	Ref              OperationCheckpointRestoreSettlementAssociationRefV5 `json:"ref"`
	SubmissionDigest core.Digest                                          `json:"submission_digest"`
}

func (v OperationCheckpointRestoreSettlementAssociationV5) Validate() error {
	if v.Ref.Validate() != nil {
		return checkpointInvalidV2("checkpoint Settlement V5 association is invalid")
	}
	return v.SubmissionDigest.Validate()
}

type OperationCheckpointRestoreTerminalGuardV5 struct {
	Ref             OperationCheckpointRestoreTerminalGuardRefV5 `json:"ref"`
	OperationDigest core.Digest                                  `json:"operation_digest"`
}

func (v OperationCheckpointRestoreTerminalGuardV5) Validate() error {
	if v.Ref.Validate() != nil {
		return checkpointInvalidV2("checkpoint Settlement V5 guard is invalid")
	}
	return v.OperationDigest.Validate()
}

type OperationCheckpointRestoreTerminalProjectionV5 struct {
	Ref          OperationCheckpointRestoreTerminalProjectionRefV5    `json:"ref"`
	Association  OperationCheckpointRestoreSettlementAssociationRefV5 `json:"association"`
	Guard        OperationCheckpointRestoreTerminalGuardRefV5         `json:"guard"`
	DomainResult CheckpointParticipantDomainResultRefV2               `json:"domain_result"`
}

func (v OperationCheckpointRestoreTerminalProjectionV5) Validate() error {
	if v.Ref.Validate() != nil || v.Association.Validate() != nil || v.Guard.Validate() != nil || v.DomainResult.Validate() != nil || v.Ref.Settlement != v.Association.Settlement || v.Ref.Settlement != v.Guard.Settlement {
		return checkpointInvalidV2("checkpoint Settlement V5 terminal projection is not exact")
	}
	return nil
}

type OperationCheckpointRestoreSettlementCommitBundleV5 struct {
	Submission     OperationCheckpointRestoreSettlementSubmissionV5  `json:"submission"`
	Settlement     OperationCheckpointRestoreSettlementRefV5         `json:"settlement"`
	Association    OperationCheckpointRestoreSettlementAssociationV5 `json:"association"`
	Guard          OperationCheckpointRestoreTerminalGuardV5         `json:"guard"`
	Projection     OperationCheckpointRestoreTerminalProjectionV5    `json:"projection"`
	EffectTerminal OperationCheckpointRestoreEffectTerminalV5        `json:"effect_terminal"`
}

func (b OperationCheckpointRestoreSettlementCommitBundleV5) Validate() error {
	if err := b.Submission.Validate(); err != nil {
		return err
	}
	if err := b.Settlement.Validate(); err != nil {
		return err
	}
	if err := b.Association.Validate(); err != nil {
		return err
	}
	if err := b.Guard.Validate(); err != nil {
		return err
	}
	if err := b.Projection.Validate(); err != nil {
		return err
	}
	if err := b.EffectTerminal.Validate(); err != nil {
		return err
	}
	if b.Settlement.ID != b.Submission.ID || b.Settlement.OperationDigest != b.Submission.OperationDigest || b.Settlement.EffectID != b.Submission.EffectID || b.Settlement.Attempt != b.Submission.CheckpointAttempt || b.Settlement.Phase != b.Submission.Phase {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Settlement V5 submission identity drifted")
	}
	if b.Association.Ref.Settlement != b.Settlement || b.Guard.Ref.Settlement != b.Settlement || b.Projection.Ref.Settlement != b.Settlement {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Settlement V5 terminal refs drifted")
	}
	if b.Projection.Association != b.Association.Ref || b.Projection.Guard != b.Guard.Ref || !sameCheckpointCanonicalV2("CheckpointParticipantDomainResultRefV2", b.Projection.DomainResult, b.Submission.DomainResult) {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Settlement V5 four-object closure drifted")
	}
	if b.EffectTerminal.Ref.Settlement != b.Settlement || b.EffectTerminal.Ref.PreviousRevision != b.Submission.ExpectedEffectRevision || b.EffectTerminal.Ref.OperationDigest != b.Submission.OperationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "checkpoint Settlement V5 terminal Effect closure drifted")
	}
	return nil
}

func sameCheckpointCanonicalV2(discriminator string, left, right any) bool {
	leftDigest, leftErr := checkpointDigestV2(discriminator, left)
	rightDigest, rightErr := checkpointDigestV2(discriminator, right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

type OperationCheckpointRestoreSettlementInspectionV5 struct {
	Bundle          OperationCheckpointRestoreSettlementCommitBundleV5 `json:"bundle"`
	Current         bool                                               `json:"current"`
	CheckedUnixNano int64                                              `json:"checked_unix_nano"`
}

func (i OperationCheckpointRestoreSettlementInspectionV5) Validate() error {
	if i.Bundle.Validate() != nil || !i.Current || i.CheckedUnixNano <= 0 {
		return checkpointInvalidV2("checkpoint Settlement V5 current inspection is incomplete")
	}
	return nil
}

type InspectOperationCheckpointRestoreSettlementRequestV5 struct {
	Operation    OperationSubjectV3 `json:"operation"`
	SettlementID string             `json:"settlement_id"`
}

func (r InspectOperationCheckpointRestoreSettlementRequestV5) Validate() error {
	if r.Operation.Validate() != nil || !validCheckpointIDV2(r.SettlementID) {
		return checkpointInvalidV2("checkpoint historical Settlement V5 request is incomplete")
	}
	return nil
}

type InspectCurrentOperationCheckpointRestoreSettlementRequestV5 struct {
	Operation OperationSubjectV3  `json:"operation"`
	EffectID  core.EffectIntentID `json:"effect_id"`
}

func (r InspectCurrentOperationCheckpointRestoreSettlementRequestV5) Validate() error {
	if r.Operation.Validate() != nil || !validCheckpointIDV2(string(r.EffectID)) {
		return checkpointInvalidV2("checkpoint current Settlement V5 request is incomplete")
	}
	return nil
}

// OperationSettlementCurrentReaderV5 is the capability-narrowed public read
// surface for the current Runtime-owned checkpoint Settlement closure. It does
// not grant Settle or Fact Owner authority.
type OperationSettlementCurrentReaderV5 interface {
	InspectCheckpointPhaseSettlementCurrentV5(context.Context, InspectCurrentOperationCheckpointRestoreSettlementRequestV5) (OperationCheckpointRestoreSettlementInspectionV5, error)
}

// OperationSettlementCurrentReaderProviderV5 is the composition capability
// supplied by the Runtime Kernel. The marker prevents a raw Fact Port from
// accidentally satisfying consumer wiring merely because it has the same
// Inspect method. It is not a language-level defense against a deliberately
// forged implementation; production composition binds the Kernel facade.
type OperationSettlementCurrentReaderProviderV5 interface {
	OperationSettlementCurrentReaderV5
	GatewayBackedOperationSettlementCurrentReaderV5()
}

type OperationCheckpointRestoreSettlementGovernancePortV5 interface {
	OperationSettlementCurrentReaderV5
	SettleCheckpointPhaseV5(context.Context, OperationCheckpointRestoreSettlementSubmissionV5) (OperationCheckpointRestoreSettlementRefV5, error)
	InspectCheckpointPhaseSettlementHistoricalV5(context.Context, InspectOperationCheckpointRestoreSettlementRequestV5) (OperationCheckpointRestoreSettlementCommitBundleV5, error)
	InspectCheckpointPhaseSettlementAssociationV5(context.Context, OperationSubjectV3, OperationCheckpointRestoreSettlementAssociationRefV5) (OperationCheckpointRestoreSettlementAssociationV5, error)
	InspectCheckpointPhaseTerminalGuardV5(context.Context, OperationSubjectV3, OperationCheckpointRestoreTerminalGuardRefV5) (OperationCheckpointRestoreTerminalGuardV5, error)
	InspectCheckpointPhaseTerminalProjectionV5(context.Context, OperationSubjectV3, OperationCheckpointRestoreTerminalProjectionRefV5) (OperationCheckpointRestoreTerminalProjectionV5, error)
}

type OperationCheckpointRestoreSettlementFactPortV5 interface {
	CommitCheckpointPhaseSettlementV5(context.Context, OperationCheckpointRestoreSettlementCommitBundleV5) (OperationCheckpointRestoreSettlementCommitBundleV5, error)
	InspectCheckpointPhaseSettlementHistoricalV5(context.Context, InspectOperationCheckpointRestoreSettlementRequestV5) (OperationCheckpointRestoreSettlementCommitBundleV5, error)
	InspectCheckpointPhaseSettlementCurrentV5(context.Context, InspectCurrentOperationCheckpointRestoreSettlementRequestV5) (OperationCheckpointRestoreSettlementInspectionV5, error)
	InspectCheckpointPhaseSettlementAssociationV5(context.Context, OperationSubjectV3, OperationCheckpointRestoreSettlementAssociationRefV5) (OperationCheckpointRestoreSettlementAssociationV5, error)
	InspectCheckpointPhaseTerminalGuardV5(context.Context, OperationSubjectV3, OperationCheckpointRestoreTerminalGuardRefV5) (OperationCheckpointRestoreTerminalGuardV5, error)
	InspectCheckpointPhaseTerminalProjectionV5(context.Context, OperationSubjectV3, OperationCheckpointRestoreTerminalProjectionRefV5) (OperationCheckpointRestoreTerminalProjectionV5, error)
}
