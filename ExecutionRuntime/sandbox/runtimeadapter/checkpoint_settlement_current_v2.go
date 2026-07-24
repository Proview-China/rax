package runtimeadapter

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// CheckpointSettlementCurrentAdapterV2 converts an exact Runtime-owned V5
// current Settlement inspection into the narrow Sandbox projection consumed
// by ApplySettlement. It cannot Settle and never copies Runtime disposition.
type CheckpointSettlementCurrentAdapterV2 struct {
	phases       ports.CheckpointPhaseStore
	results      ports.CheckpointPhaseResultStoreV2
	reservations runtimeports.CheckpointParticipantPhaseReservationCurrentReaderV2
	settlements  runtimeports.OperationSettlementCurrentReaderV5
	clock        func() time.Time
}

func NewCheckpointSettlementCurrentAdapterV2(phases ports.CheckpointPhaseStore, results ports.CheckpointPhaseResultStoreV2, reservations runtimeports.CheckpointParticipantPhaseReservationCurrentReaderV2, settlements runtimeports.OperationSettlementCurrentReaderV5, clock func() time.Time) (*CheckpointSettlementCurrentAdapterV2, error) {
	if checkpointAdapterNilV1(phases) || checkpointAdapterNilV1(results) || checkpointAdapterNilV1(reservations) || checkpointAdapterNilV1(settlements) || clock == nil {
		return nil, errors.New("checkpoint Settlement current adapter dependencies are required")
	}
	return &CheckpointSettlementCurrentAdapterV2{phases: phases, results: results, reservations: reservations, settlements: settlements, clock: clock}, nil
}

func (a *CheckpointSettlementCurrentAdapterV2) InspectCheckpointPhaseSettlementCurrentV2(ctx context.Context, expectedDomain contract.SnapshotArtifactExactRefV2, expectedSettlement contract.Ref) (contract.CheckpointPhaseSettlementCurrentProjectionV2, error) {
	if a == nil || checkpointAdapterNilV1(ctx) || expectedDomain.ValidateShape("checkpoint DomainResult") != nil || expectedDomain.TypeURL != contract.CheckpointPhaseDomainResultTypeURLV2 || expectedSettlement.ValidateShape("Runtime checkpoint Settlement") != nil {
		return contract.CheckpointPhaseSettlementCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint Settlement current request is invalid", ports.ErrConflict)
	}
	domain, err := a.results.InspectCheckpointPhaseDomainResultV2(ctx, expectedDomain)
	if err != nil {
		return contract.CheckpointPhaseSettlementCurrentProjectionV2{}, err
	}
	now := a.clock()
	if domain.ValidateCurrent(now) != nil || domain.ExactRef() != expectedDomain {
		return contract.CheckpointPhaseSettlementCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint DomainResult is not exact current", ports.ErrStale)
	}
	reservation, err := a.phases.InspectCheckpointPhaseReservation(ctx, domain.ReservationRef)
	if err != nil || reservation.ValidateCurrent(now) != nil {
		return contract.CheckpointPhaseSettlementCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint Reservation is not current", ports.ErrStale)
	}
	runtimePhase, err := checkpointRuntimePhaseV1(reservation.Phase)
	if err != nil {
		return contract.CheckpointPhaseSettlementCurrentProjectionV2{}, err
	}
	runtimeReservationRef := runtimeports.CheckpointParticipantPhaseReservationRefV2{ID: reservation.Meta.ID, Revision: runtimecore.Revision(reservation.Meta.Revision), Digest: runtimeDigest(reservation.Meta.Digest), ExpiresUnixNano: reservation.Meta.ExpiresUnixNano}
	runtimeReservation, err := a.reservations.InspectCheckpointParticipantPhaseReservationCurrentV2(ctx, runtimeReservationRef, runtimePhase)
	if err != nil || runtimeReservation.Validate(now) != nil || validateCheckpointDomainRuntimeMappingV2(domain, reservation, runtimeReservation) != nil {
		return contract.CheckpointPhaseSettlementCurrentProjectionV2{}, fmt.Errorf("%w: Runtime checkpoint Reservation mapping is not exact current", ports.ErrConflict)
	}
	inspection, err := a.settlements.InspectCheckpointPhaseSettlementCurrentV5(ctx, runtimeports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5{Operation: runtimeReservation.Operation, EffectID: runtimeReservation.EffectID})
	if err != nil {
		return contract.CheckpointPhaseSettlementCurrentProjectionV2{}, err
	}
	if inspection.Validate() != nil || inspection.CheckedUnixNano <= 0 || inspection.CheckedUnixNano > now.UnixNano() {
		return contract.CheckpointPhaseSettlementCurrentProjectionV2{}, fmt.Errorf("%w: Runtime checkpoint Settlement is not exact current", ports.ErrConflict)
	}
	mappedDomain := checkpointRuntimeDomainResultRefV2(domain, runtimeReservation)
	bundle := inspection.Bundle
	settlement := bundle.Settlement
	if !runtimeports.SameOperationSubjectV3(bundle.Submission.Operation, runtimeReservation.Operation) || bundle.Submission.EffectID != runtimeReservation.EffectID || !sameRuntimeCheckpointDomainResultV2(bundle.Submission.DomainResult, mappedDomain) || !sameRuntimeCheckpointDomainResultV2(bundle.Projection.DomainResult, mappedDomain) || settlement.ID != expectedSettlement.ID || uint64(settlement.Revision) != expectedSettlement.Revision || trimRuntimeDigestV1(settlement.Digest) != expectedSettlement.Digest {
		return contract.CheckpointPhaseSettlementCurrentProjectionV2{}, fmt.Errorf("%w: Runtime checkpoint Settlement crosses Sandbox DomainResult", ports.ErrConflict)
	}
	fresh := a.clock()
	domainS2, err := a.results.InspectCheckpointPhaseDomainResultV2(ctx, expectedDomain)
	if err != nil {
		return contract.CheckpointPhaseSettlementCurrentProjectionV2{}, err
	}
	reservationS2, err := a.phases.InspectCheckpointPhaseReservation(ctx, domainS2.ReservationRef)
	if err != nil {
		return contract.CheckpointPhaseSettlementCurrentProjectionV2{}, err
	}
	runtimeReservationS2, err := a.reservations.InspectCheckpointParticipantPhaseReservationCurrentV2(ctx, runtimeReservationRef, runtimePhase)
	if err != nil {
		return contract.CheckpointPhaseSettlementCurrentProjectionV2{}, err
	}
	inspectionS2, err := a.settlements.InspectCheckpointPhaseSettlementCurrentV5(ctx, runtimeports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5{Operation: runtimeReservationS2.Operation, EffectID: runtimeReservationS2.EffectID})
	if err != nil || domainS2.ValidateCurrent(fresh) != nil || reservationS2.ValidateCurrent(fresh) != nil || runtimeReservationS2.Validate(fresh) != nil || inspectionS2.Validate() != nil || inspectionS2.CheckedUnixNano > fresh.UnixNano() || validateCheckpointDomainRuntimeMappingV2(domainS2, reservationS2, runtimeReservationS2) != nil || !reflect.DeepEqual(domain, domainS2) || !reflect.DeepEqual(reservation, reservationS2) || !reflect.DeepEqual(runtimeReservation, runtimeReservationS2) || !reflect.DeepEqual(inspection, inspectionS2) {
		return contract.CheckpointPhaseSettlementCurrentProjectionV2{}, fmt.Errorf("%w: Runtime checkpoint Settlement changed between S1 and S2", ports.ErrConflict)
	}
	expires := min(domainS2.Meta.ExpiresUnixNano, reservationS2.Meta.ExpiresUnixNano, runtimeReservationS2.ExpiresUnixNano)
	return contract.SealCheckpointPhaseSettlementCurrentProjectionV2(contract.CheckpointPhaseSettlementCurrentProjectionV2{DomainResultRef: expectedDomain, RuntimeSettlementRef: expectedSettlement, CheckedUnixNano: inspectionS2.CheckedUnixNano, ExpiresUnixNano: expires})
}

func validateCheckpointDomainRuntimeMappingV2(domain contract.CheckpointPhaseDomainResultV2, reservation contract.CheckpointPhaseReservation, current runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2) error {
	if current.Ref.ID != reservation.Meta.ID || uint64(current.Ref.Revision) != reservation.Meta.Revision || trimRuntimeDigestV1(current.Ref.Digest) != reservation.Meta.Digest || current.Ref.ExpiresUnixNano != reservation.Meta.ExpiresUnixNano || current.Participant.ID != domain.ParticipantRef.ID || trimRuntimeDigestV1(current.Participant.Digest) != domain.ParticipantRef.Digest || current.Attempt.ID != domain.CheckpointAttemptRef.ID || uint64(current.Attempt.Revision) != domain.CheckpointAttemptRef.Revision || trimRuntimeDigestV1(current.Attempt.Digest) != domain.CheckpointAttemptRef.Digest || current.Phase != checkpointRuntimePhaseMustV2(domain.Phase) || checkpointRuntimeOperationIDV2(current.Operation) != domain.OperationID || string(current.EffectID) != domain.EffectID {
		return errors.New("checkpoint DomainResult differs from Runtime Reservation")
	}
	return nil
}

func checkpointRuntimeOperationIDV2(operation runtimeports.OperationSubjectV3) string {
	switch operation.Kind {
	case runtimeports.OperationScopeActivationV3:
		return operation.ActivationAttemptID
	case runtimeports.OperationScopeRunV3:
		return string(operation.RunID)
	case runtimeports.OperationScopeTerminationV3:
		return operation.TerminationAttemptID
	case runtimeports.OperationScopeAdminV3:
		return operation.AdminOperationID
	default:
		return operation.CustomOperationID
	}
}

func checkpointRuntimePhaseMustV2(phase contract.CheckpointPhase) runtimeports.CheckpointParticipantPhaseV2 {
	value, _ := checkpointRuntimePhaseV1(phase)
	return value
}

func checkpointRuntimeDomainResultRefV2(domain contract.CheckpointPhaseDomainResultV2, reservation runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2) runtimeports.CheckpointParticipantDomainResultRefV2 {
	return runtimeports.CheckpointParticipantDomainResultRefV2{ID: domain.Meta.ID, Revision: runtimecore.Revision(domain.Meta.Revision), Kind: runtimeports.NamespacedNameV2("praxis.sandbox/checkpoint-phase-domain-result"), Attempt: reservation.Attempt, Participant: reservation.Participant, Phase: reservation.Phase, Operation: reservation.Operation, OperationDigest: reservation.OperationDigest, Digest: runtimeDigest(domain.Meta.Digest)}
}

func sameRuntimeCheckpointDomainResultV2(left, right runtimeports.CheckpointParticipantDomainResultRefV2) bool {
	return left.ID == right.ID && left.Revision == right.Revision && left.Kind == right.Kind && left.Attempt == right.Attempt && left.Participant == right.Participant && left.Phase == right.Phase && runtimeports.SameOperationSubjectV3(left.Operation, right.Operation) && left.OperationDigest == right.OperationDigest && left.Digest == right.Digest
}

var _ ports.CheckpointPhaseSettlementCurrentReaderV2 = (*CheckpointSettlementCurrentAdapterV2)(nil)
