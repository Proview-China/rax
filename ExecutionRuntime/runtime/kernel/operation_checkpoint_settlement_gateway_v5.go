package kernel

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type OperationCheckpointRestoreSettlementGatewayV5 struct {
	Facts         ports.OperationCheckpointRestoreSettlementFactPortV5
	Inputs        ports.CheckpointAttemptInputsCurrentReaderV2
	Reservations  ports.CheckpointParticipantPhaseReservationCurrentReaderV2
	Participants  ports.CheckpointParticipantPhaseCurrentReaderV2
	DomainResults ports.CheckpointParticipantDomainResultCurrentReaderV2
	Evidence      ports.CheckpointRestoreEvidenceGovernancePortV1
	Enforcement   ports.OperationDispatchEnforcementGovernancePortV4
	Clock         func() time.Time
}

// OperationSettlementCurrentReaderFacadeV5 is the only Runtime-provided
// composition adapter for the narrowed current Reader. It deliberately wraps
// the Kernel Gateway rather than the structurally compatible raw Fact Port.
type OperationSettlementCurrentReaderFacadeV5 struct {
	gateway *OperationCheckpointRestoreSettlementGatewayV5
}

func NewOperationSettlementCurrentReaderFacadeV5(gateway *OperationCheckpointRestoreSettlementGatewayV5) (*OperationSettlementCurrentReaderFacadeV5, error) {
	if gateway == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "checkpoint Settlement V5 Kernel Gateway is required")
	}
	if err := requireCheckpointDependencyV2(gateway.Facts, "checkpoint Settlement V5 Fact Owner"); err != nil {
		return nil, err
	}
	return &OperationSettlementCurrentReaderFacadeV5{gateway: gateway}, nil
}

func (*OperationSettlementCurrentReaderFacadeV5) GatewayBackedOperationSettlementCurrentReaderV5() {}

func (f *OperationSettlementCurrentReaderFacadeV5) InspectCheckpointPhaseSettlementCurrentV5(ctx context.Context, request ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5) (ports.OperationCheckpointRestoreSettlementInspectionV5, error) {
	if f == nil || f.gateway == nil {
		return ports.OperationCheckpointRestoreSettlementInspectionV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "checkpoint Settlement V5 current Reader facade is required")
	}
	return f.gateway.InspectCheckpointPhaseSettlementCurrentV5(ctx, request)
}

func (g OperationCheckpointRestoreSettlementGatewayV5) SettleCheckpointPhaseV5(ctx context.Context, submission ports.OperationCheckpointRestoreSettlementSubmissionV5) (ports.OperationCheckpointRestoreSettlementRefV5, error) {
	if err := submission.Validate(); err != nil {
		return ports.OperationCheckpointRestoreSettlementRefV5{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Settlement V5 Fact Owner"); err != nil {
		return ports.OperationCheckpointRestoreSettlementRefV5{}, err
	}
	if err := requireCheckpointDependencyV2(g.DomainResults, "checkpoint DomainResult current Reader"); err != nil {
		return ports.OperationCheckpointRestoreSettlementRefV5{}, err
	}
	for _, dependency := range []struct {
		value any
		name  string
	}{{g.Inputs, "checkpoint Attempt Inputs current Reader"}, {g.Reservations, "checkpoint Reservation current Reader"}, {g.Participants, "checkpoint Participant current Reader"}, {g.Evidence, "checkpoint Evidence current Reader"}, {g.Enforcement, "checkpoint Enforcement current Reader"}} {
		if err := requireCheckpointDependencyV2(dependency.value, dependency.name); err != nil {
			return ports.OperationCheckpointRestoreSettlementRefV5{}, err
		}
	}
	if err := requireCheckpointDependencyV2(g.Clock, "checkpoint Settlement Clock"); err != nil {
		return ports.OperationCheckpointRestoreSettlementRefV5{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.OperationCheckpointRestoreSettlementRefV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "checkpoint Settlement clock is zero")
	}
	snapshot, err := g.readCurrentCheckpointSettlementClosureV5(ctx, submission, now)
	if err != nil {
		return ports.OperationCheckpointRestoreSettlementRefV5{}, err
	}
	fresh := g.Clock()
	if fresh.IsZero() || fresh.Before(now) {
		return ports.OperationCheckpointRestoreSettlementRefV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "checkpoint Settlement clock regressed before commit")
	}
	snapshot2, err := g.readCurrentCheckpointSettlementClosureV5(ctx, submission, fresh)
	if err != nil {
		return ports.OperationCheckpointRestoreSettlementRefV5{}, err
	}
	if snapshot != snapshot2 {
		return ports.OperationCheckpointRestoreSettlementRefV5{}, checkpointGatewayConflictV2("checkpoint Settlement current closure changed before terminal CAS")
	}
	expected, err := control.BuildOperationCheckpointRestoreSettlementBundleV5(submission)
	if err != nil {
		return ports.OperationCheckpointRestoreSettlementRefV5{}, err
	}
	committed, err := g.Facts.CommitCheckpointPhaseSettlementV5(ctx, expected)
	if err != nil {
		if !checkpointRecoverableV2(err) {
			return ports.OperationCheckpointRestoreSettlementRefV5{}, err
		}
		committed, err = g.Facts.InspectCheckpointPhaseSettlementHistoricalV5(context.WithoutCancel(ctx), ports.InspectOperationCheckpointRestoreSettlementRequestV5{Operation: submission.Operation, SettlementID: submission.ID})
		if err != nil {
			return ports.OperationCheckpointRestoreSettlementRefV5{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectSettlementMissing, "V5 settlement outcome cannot be inspected")
		}
	}
	if err := committed.Validate(); err != nil {
		return ports.OperationCheckpointRestoreSettlementRefV5{}, err
	}
	if committed.Settlement != expected.Settlement {
		return ports.OperationCheckpointRestoreSettlementRefV5{}, checkpointGatewayConflictV2("V5 settlement recovery found different canonical content")
	}
	return committed.Settlement, nil
}

type checkpointSettlementCurrentSnapshotV5 struct {
	Inputs            core.Digest
	QualificationFact core.Digest
	Reservation       core.Digest
	Participant       core.Digest
	Domain            core.Digest
	Qualification     core.Digest
	Handoff           core.Digest
	Consumption       core.Digest
	Prepare           core.Digest
	Execute           core.Digest
}

func (g OperationCheckpointRestoreSettlementGatewayV5) readCurrentCheckpointSettlementClosureV5(ctx context.Context, submission ports.OperationCheckpointRestoreSettlementSubmissionV5, now time.Time) (checkpointSettlementCurrentSnapshotV5, error) {
	inputs, err := g.Inputs.InspectCheckpointAttemptInputsCurrentV2(ctx, submission.CheckpointAttempt)
	if err != nil {
		return checkpointSettlementCurrentSnapshotV5{}, err
	}
	if err := inputs.Validate(now); err != nil {
		return checkpointSettlementCurrentSnapshotV5{}, err
	}
	qualification, err := g.Evidence.InspectCheckpointPhaseQualificationCurrentV1(ctx, submission.Evidence.Qualification)
	if err != nil {
		return checkpointSettlementCurrentSnapshotV5{}, err
	}
	if err := qualification.Validate(now); err != nil {
		return checkpointSettlementCurrentSnapshotV5{}, err
	}
	qualificationFact, err := g.Evidence.InspectCheckpointPhaseQualificationHistoricalV1(ctx, submission.Evidence.Qualification)
	if err != nil {
		return checkpointSettlementCurrentSnapshotV5{}, err
	}
	if err := qualificationFact.Validate(); err != nil || qualificationFact.Ref != submission.Evidence.Qualification {
		if err != nil {
			return checkpointSettlementCurrentSnapshotV5{}, err
		}
		return checkpointSettlementCurrentSnapshotV5{}, checkpointGatewayConflictV2("checkpoint Qualification historical Fact drifted")
	}
	scope := qualification.Scope
	if qualificationFact.Request.Attempt != submission.CheckpointAttempt || qualificationFact.Request.Reservation != submission.Reservation || qualificationFact.Request.Phase != submission.Phase || !sameCheckpointScopeV5(qualificationFact.Request.Scope, scope) || scope.OperationDigest != submission.OperationDigest || !ports.SameOperationSubjectV3(scope.Operation, submission.Operation) || scope.EffectID != submission.EffectID || scope.EffectRevision != submission.ExpectedEffectRevision || scope.DispatchAttempt != submission.DispatchAttempt || scope.ExecuteEnforcement != submission.Enforcement || scope.Assembly != inputs.GenerationBinding || scope.Generation != inputs.GenerationArtifact || scope.Authority != inputs.AuthorityRef || scope.Source != submission.Evidence.Source {
		return checkpointSettlementCurrentSnapshotV5{}, checkpointGatewayConflictV2("checkpoint Settlement Evidence scope does not bind current attempt inputs")
	}
	reservation, err := g.Reservations.InspectCheckpointParticipantPhaseReservationCurrentV2(ctx, submission.Reservation, submission.Phase)
	if err != nil {
		return checkpointSettlementCurrentSnapshotV5{}, err
	}
	if err := reservation.Validate(now); err != nil || reservation.Ref != submission.Reservation || reservation.Attempt != submission.CheckpointAttempt || reservation.Barrier != qualificationFact.Request.Barrier || reservation.EffectCut != qualificationFact.Request.EffectCut || reservation.Participant != submission.DomainResult.Participant || reservation.OwnerBinding != submission.Owner || reservation.Phase != submission.Phase || !ports.SameOperationSubjectV3(reservation.Operation, submission.Operation) || reservation.OperationDigest != submission.OperationDigest || reservation.EffectID != submission.EffectID || reservation.EffectKind != scope.EffectKind || reservation.IntentDigest != scope.IntentDigest || reservation.Generation != inputs.GenerationBinding {
		if err != nil {
			return checkpointSettlementCurrentSnapshotV5{}, err
		}
		return checkpointSettlementCurrentSnapshotV5{}, checkpointGatewayConflictV2("checkpoint Reservation current ref drifted")
	}
	participant, err := g.Participants.InspectCheckpointParticipantPhaseCurrentV2(ctx, submission.ParticipantFact)
	if err != nil {
		return checkpointSettlementCurrentSnapshotV5{}, err
	}
	if err := participant.Validate(now); err != nil || participant.Ref != submission.ParticipantFact || participant.Reservation != reservation.Ref || participant.Ref.Phase != submission.Phase || !checkpointParticipantTerminalStateMatchesPhaseV5(participant.Ref) {
		if err != nil {
			return checkpointSettlementCurrentSnapshotV5{}, err
		}
		return checkpointSettlementCurrentSnapshotV5{}, checkpointGatewayConflictV2("checkpoint Participant current fact drifted")
	}
	domain, err := g.DomainResults.ReadCheckpointDomainResultCurrentV2(ctx, submission.DomainResult)
	if err != nil {
		return checkpointSettlementCurrentSnapshotV5{}, err
	}
	if err := domain.Validate(now); err != nil || domain.Ref != submission.DomainResult || domain.Ref.Participant != reservation.Participant || domain.Ref.Phase != submission.Phase || !ports.SameOperationSubjectV3(domain.Ref.Operation, submission.Operation) || domain.Ref.OperationDigest != submission.OperationDigest {
		if err != nil {
			return checkpointSettlementCurrentSnapshotV5{}, err
		}
		return checkpointSettlementCurrentSnapshotV5{}, checkpointGatewayConflictV2("checkpoint DomainResult current ref drifted")
	}
	handoff, err := g.Evidence.InspectCheckpointPhaseProviderHandoffCurrentV1(ctx, submission.Handoff)
	if err != nil {
		return checkpointSettlementCurrentSnapshotV5{}, err
	}
	if err := handoff.Validate(now); err != nil || handoff.Ref != submission.Handoff || handoff.Ref.Qualification != qualificationFact.Ref || handoff.Ref.Attempt != submission.DispatchAttempt || handoff.Ref.Phase != submission.Phase || handoff.Ref.ScopeDigest != qualificationFact.Ref.ScopeDigest {
		if err != nil {
			return checkpointSettlementCurrentSnapshotV5{}, err
		}
		return checkpointSettlementCurrentSnapshotV5{}, checkpointGatewayConflictV2("checkpoint Handoff current ref drifted")
	}
	consumption, err := g.Evidence.InspectCheckpointPhaseEvidenceConsumptionCurrentV1(ctx, submission.Evidence)
	if err != nil {
		return checkpointSettlementCurrentSnapshotV5{}, err
	}
	if err := consumption.Validate(); err != nil || consumption.Ref != submission.Evidence || consumption.Ref.Qualification != qualificationFact.Ref || consumption.Ref.Handoff != handoff.Ref || consumption.Ref.Attempt != submission.CheckpointAttempt || consumption.Ref.Phase != submission.Phase || consumption.Ref.ScopeDigest != qualificationFact.Ref.ScopeDigest || consumption.Ref.Source != scope.Source {
		if err != nil {
			return checkpointSettlementCurrentSnapshotV5{}, err
		}
		return checkpointSettlementCurrentSnapshotV5{}, checkpointGatewayConflictV2("checkpoint Evidence consumption current ref drifted")
	}
	prepare, err := g.inspectCheckpointEnforcementCurrentV5(ctx, scope, ports.OperationDispatchEnforcementPrepareV4)
	if err != nil {
		return checkpointSettlementCurrentSnapshotV5{}, err
	}
	execute, err := g.inspectCheckpointEnforcementCurrentV5(ctx, scope, ports.OperationDispatchEnforcementExecuteV4)
	if err != nil {
		return checkpointSettlementCurrentSnapshotV5{}, err
	}
	qualificationFactDigest, err := control.CheckpointCanonicalDigestV2("CheckpointRestoreEvidenceQualificationFactV1", qualificationFact)
	if err != nil {
		return checkpointSettlementCurrentSnapshotV5{}, err
	}
	return checkpointSettlementCurrentSnapshotV5{Inputs: inputs.ProjectionDigest, QualificationFact: qualificationFactDigest, Reservation: reservation.ProjectionDigest, Participant: participant.ProjectionDigest, Domain: domain.ProjectionDigest, Qualification: qualification.ProjectionDigest, Handoff: handoff.ProjectionDigest, Consumption: consumption.ProjectionDigest, Prepare: prepare.Digest, Execute: execute.Digest}, nil
}

func sameCheckpointScopeV5(left, right ports.CheckpointRestoreEvidenceScopeV1) bool {
	leftDigest, leftErr := left.DigestV1()
	rightDigest, rightErr := right.DigestV1()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func checkpointParticipantTerminalStateMatchesPhaseV5(ref ports.CheckpointParticipantPhaseRefV2) bool {
	switch ref.Phase {
	case ports.CheckpointPhasePrepareV2:
		return ref.State == ports.CheckpointParticipantPreparedV2
	case ports.CheckpointPhaseCommitV2:
		return ref.State == ports.CheckpointParticipantCommittedV2
	case ports.CheckpointPhaseAbortV2:
		return ref.State == ports.CheckpointParticipantAbortedV2
	default:
		return false
	}
}

func (g OperationCheckpointRestoreSettlementGatewayV5) inspectCheckpointEnforcementCurrentV5(ctx context.Context, scope ports.CheckpointRestoreEvidenceScopeV1, phase ports.OperationDispatchEnforcementPhaseV4) (ports.OperationDispatchEnforcementJournalV4, error) {
	ref := scope.PrepareEnforcement
	if phase == ports.OperationDispatchEnforcementExecuteV4 {
		ref = scope.ExecuteEnforcement
	}
	request := ports.InspectOperationDispatchEnforcementRequestV4{Operation: scope.Operation, EffectID: scope.EffectID, PermitID: ref.PermitID, Phase: phase}
	current, err := g.Enforcement.InspectOperationDispatchEnforcementV4(ctx, request)
	if err != nil {
		return ports.OperationDispatchEnforcementJournalV4{}, err
	}
	expected, phaseErr := current.PhaseRefV4(phase)
	if err := current.Validate(); err != nil || phaseErr != nil || expected != ref {
		if err != nil {
			return ports.OperationDispatchEnforcementJournalV4{}, err
		}
		return ports.OperationDispatchEnforcementJournalV4{}, checkpointGatewayConflictV2("checkpoint Enforcement historical ref drifted")
	}
	return current, nil
}

func (g OperationCheckpointRestoreSettlementGatewayV5) InspectCheckpointPhaseSettlementHistoricalV5(ctx context.Context, request ports.InspectOperationCheckpointRestoreSettlementRequestV5) (ports.OperationCheckpointRestoreSettlementCommitBundleV5, error) {
	if err := request.Validate(); err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Settlement V5 Fact Owner"); err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	bundle, err := g.Facts.InspectCheckpointPhaseSettlementHistoricalV5(ctx, request)
	if err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	return bundle, bundle.Validate()
}
func (g OperationCheckpointRestoreSettlementGatewayV5) InspectCheckpointPhaseSettlementCurrentV5(ctx context.Context, request ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5) (ports.OperationCheckpointRestoreSettlementInspectionV5, error) {
	if err := request.Validate(); err != nil {
		return ports.OperationCheckpointRestoreSettlementInspectionV5{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Settlement V5 Fact Owner"); err != nil {
		return ports.OperationCheckpointRestoreSettlementInspectionV5{}, err
	}
	inspection, err := g.Facts.InspectCheckpointPhaseSettlementCurrentV5(ctx, request)
	if err != nil {
		return ports.OperationCheckpointRestoreSettlementInspectionV5{}, err
	}
	if err := inspection.Validate(); err != nil {
		return ports.OperationCheckpointRestoreSettlementInspectionV5{}, err
	}
	bundle := inspection.Bundle
	if !ports.SameOperationSubjectV3(bundle.Submission.Operation, request.Operation) || bundle.Submission.EffectID != request.EffectID || bundle.Settlement.EffectID != request.EffectID {
		return ports.OperationCheckpointRestoreSettlementInspectionV5{}, checkpointGatewayConflictV2("checkpoint Settlement V5 current request drifted")
	}
	return inspection, nil
}
func (g OperationCheckpointRestoreSettlementGatewayV5) InspectCheckpointPhaseSettlementAssociationV5(ctx context.Context, operation ports.OperationSubjectV3, ref ports.OperationCheckpointRestoreSettlementAssociationRefV5) (ports.OperationCheckpointRestoreSettlementAssociationV5, error) {
	if operation.Validate() != nil || ref.Validate() != nil {
		return ports.OperationCheckpointRestoreSettlementAssociationV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "checkpoint Settlement V5 association inspect request is invalid")
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Settlement V5 Fact Owner"); err != nil {
		return ports.OperationCheckpointRestoreSettlementAssociationV5{}, err
	}
	value, err := g.Facts.InspectCheckpointPhaseSettlementAssociationV5(ctx, operation, ref)
	if err != nil {
		return ports.OperationCheckpointRestoreSettlementAssociationV5{}, err
	}
	if err := value.Validate(); err != nil || value.Ref != ref {
		return ports.OperationCheckpointRestoreSettlementAssociationV5{}, checkpointGatewayConflictV2("checkpoint Settlement V5 association inspect drifted")
	}
	return value, nil
}
func (g OperationCheckpointRestoreSettlementGatewayV5) InspectCheckpointPhaseTerminalGuardV5(ctx context.Context, operation ports.OperationSubjectV3, ref ports.OperationCheckpointRestoreTerminalGuardRefV5) (ports.OperationCheckpointRestoreTerminalGuardV5, error) {
	if operation.Validate() != nil || ref.Validate() != nil {
		return ports.OperationCheckpointRestoreTerminalGuardV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "checkpoint Settlement V5 guard inspect request is invalid")
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Settlement V5 Fact Owner"); err != nil {
		return ports.OperationCheckpointRestoreTerminalGuardV5{}, err
	}
	value, err := g.Facts.InspectCheckpointPhaseTerminalGuardV5(ctx, operation, ref)
	if err != nil {
		return ports.OperationCheckpointRestoreTerminalGuardV5{}, err
	}
	if err := value.Validate(); err != nil || value.Ref != ref {
		return ports.OperationCheckpointRestoreTerminalGuardV5{}, checkpointGatewayConflictV2("checkpoint Settlement V5 guard inspect drifted")
	}
	return value, nil
}
func (g OperationCheckpointRestoreSettlementGatewayV5) InspectCheckpointPhaseTerminalProjectionV5(ctx context.Context, operation ports.OperationSubjectV3, ref ports.OperationCheckpointRestoreTerminalProjectionRefV5) (ports.OperationCheckpointRestoreTerminalProjectionV5, error) {
	if operation.Validate() != nil || ref.Validate() != nil {
		return ports.OperationCheckpointRestoreTerminalProjectionV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "checkpoint Settlement V5 projection inspect request is invalid")
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Settlement V5 Fact Owner"); err != nil {
		return ports.OperationCheckpointRestoreTerminalProjectionV5{}, err
	}
	value, err := g.Facts.InspectCheckpointPhaseTerminalProjectionV5(ctx, operation, ref)
	if err != nil {
		return ports.OperationCheckpointRestoreTerminalProjectionV5{}, err
	}
	if err := value.Validate(); err != nil || value.Ref != ref {
		return ports.OperationCheckpointRestoreTerminalProjectionV5{}, checkpointGatewayConflictV2("checkpoint Settlement V5 projection inspect drifted")
	}
	return value, nil
}
