package runtimeadapter

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

const checkpointRuntimePhaseResultIDSuffixV2 = "-owner-phase-result"

// CheckpointPhaseResultCurrentAdapterV2 maps a Sandbox-owned immutable
// DomainResult into Runtime's read-only Participant phase-current shape. It
// does not create the final Sandbox PhaseFact; that remains gated by Runtime
// Settlement and Sandbox ApplySettlement CAS.
type CheckpointPhaseResultCurrentAdapterV2 struct {
	phases       ports.CheckpointPhaseStore
	results      ports.CheckpointPhaseResultStoreV2
	reservations runtimeports.CheckpointParticipantPhaseReservationCurrentReaderV2
	clock        func() time.Time
}

func NewCheckpointPhaseResultCurrentAdapterV2(phases ports.CheckpointPhaseStore, results ports.CheckpointPhaseResultStoreV2, reservations runtimeports.CheckpointParticipantPhaseReservationCurrentReaderV2, clock func() time.Time) (*CheckpointPhaseResultCurrentAdapterV2, error) {
	if checkpointAdapterNilV1(phases) || checkpointAdapterNilV1(results) || checkpointAdapterNilV1(reservations) || clock == nil {
		return nil, errors.New("checkpoint phase result current adapter dependencies are required")
	}
	return &CheckpointPhaseResultCurrentAdapterV2{phases: phases, results: results, reservations: reservations, clock: clock}, nil
}

func (a *CheckpointPhaseResultCurrentAdapterV2) InspectCheckpointParticipantPhaseCurrentV2(ctx context.Context, expected runtimeports.CheckpointParticipantPhaseRefV2) (runtimeports.CheckpointParticipantPhaseCurrentProjectionV2, error) {
	if a == nil || checkpointAdapterNilV1(ctx) || expected.Validate() != nil || !strings.HasSuffix(expected.ID, checkpointRuntimePhaseResultIDSuffixV2) {
		return runtimeports.CheckpointParticipantPhaseCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint phase result current request is invalid", ports.ErrConflict)
	}
	domainID := strings.TrimSuffix(expected.ID, checkpointRuntimePhaseResultIDSuffixV2)
	domain, err := a.results.InspectCheckpointPhaseDomainResultByIDV2(ctx, domainID)
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseCurrentProjectionV2{}, err
	}
	now := a.clock()
	reservation, runtimeReservation, phaseRef, err := a.readCheckpointPhaseResultCurrentV2(ctx, domain, now)
	if err != nil || phaseRef != expected {
		if err != nil {
			return runtimeports.CheckpointParticipantPhaseCurrentProjectionV2{}, err
		}
		return runtimeports.CheckpointParticipantPhaseCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint phase result expected ref differs", ports.ErrConflict)
	}
	fresh := a.clock()
	domainS2, err := a.results.InspectCheckpointPhaseDomainResultByIDV2(ctx, domainID)
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseCurrentProjectionV2{}, err
	}
	reservationS2, runtimeReservationS2, phaseRefS2, err := a.readCheckpointPhaseResultCurrentV2(ctx, domainS2, fresh)
	if err != nil || phaseRefS2 != phaseRef || !reflect.DeepEqual(domain, domainS2) || !reflect.DeepEqual(reservation, reservationS2) || !reflect.DeepEqual(runtimeReservation, runtimeReservationS2) {
		return runtimeports.CheckpointParticipantPhaseCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint phase result current changed between S1 and S2", ports.ErrConflict)
	}
	expires := min(domainS2.Meta.ExpiresUnixNano, reservationS2.Meta.ExpiresUnixNano, runtimeReservationS2.ExpiresUnixNano)
	projection := runtimeports.CheckpointParticipantPhaseCurrentProjectionV2{Ref: phaseRefS2, Reservation: runtimeReservationS2.Ref, PreviousPhase: runtimeReservationS2.PreviousPhase, Current: true, CheckedUnixNano: fresh.UnixNano(), ExpiresUnixNano: expires}
	projection.ProjectionDigest, err = checkpointPhaseResultProjectionDigestV2(projection)
	if err != nil {
		return runtimeports.CheckpointParticipantPhaseCurrentProjectionV2{}, err
	}
	return projection, projection.Validate(fresh)
}

func (a *CheckpointPhaseResultCurrentAdapterV2) readCheckpointPhaseResultCurrentV2(ctx context.Context, domain contract.CheckpointPhaseDomainResultV2, now time.Time) (contract.CheckpointPhaseReservation, runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2, runtimeports.CheckpointParticipantPhaseRefV2, error) {
	if domain.ValidateCurrent(now) != nil {
		return contract.CheckpointPhaseReservation{}, runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2{}, runtimeports.CheckpointParticipantPhaseRefV2{}, fmt.Errorf("%w: checkpoint phase DomainResult is stale", ports.ErrStale)
	}
	reservation, err := a.phases.InspectCheckpointPhaseReservation(ctx, domain.ReservationRef)
	if err != nil || reservation.ValidateCurrent(now) != nil {
		return contract.CheckpointPhaseReservation{}, runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2{}, runtimeports.CheckpointParticipantPhaseRefV2{}, fmt.Errorf("%w: checkpoint phase Reservation is stale", ports.ErrStale)
	}
	phase, state, err := checkpointRuntimePhaseStateV2(domain.Phase, domain.State)
	if err != nil {
		return contract.CheckpointPhaseReservation{}, runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2{}, runtimeports.CheckpointParticipantPhaseRefV2{}, err
	}
	runtimeRef := runtimeports.CheckpointParticipantPhaseReservationRefV2{ID: reservation.Meta.ID, Revision: runtimecore.Revision(reservation.Meta.Revision), Digest: runtimeDigest(reservation.Meta.Digest), ExpiresUnixNano: reservation.Meta.ExpiresUnixNano}
	runtimeReservation, err := a.reservations.InspectCheckpointParticipantPhaseReservationCurrentV2(ctx, runtimeRef, phase)
	if err != nil || runtimeReservation.Validate(now) != nil || validateCheckpointDomainRuntimeMappingV2(domain, reservation, runtimeReservation) != nil {
		return contract.CheckpointPhaseReservation{}, runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2{}, runtimeports.CheckpointParticipantPhaseRefV2{}, fmt.Errorf("%w: checkpoint phase Runtime mapping drifted", ports.ErrConflict)
	}
	digest, err := runtimecore.CanonicalJSONDigest("praxis.sandbox.checkpoint-phase-result", "2.0.0", "CheckpointParticipantPhaseResultRefV2", struct {
		Domain      contract.SnapshotArtifactExactRefV2
		Reservation runtimeports.CheckpointParticipantPhaseReservationRefV2
		Phase       runtimeports.CheckpointParticipantPhaseV2
		State       runtimeports.CheckpointParticipantPhaseStateV2
	}{domain.ExactRef(), runtimeReservation.Ref, phase, state})
	if err != nil {
		return contract.CheckpointPhaseReservation{}, runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2{}, runtimeports.CheckpointParticipantPhaseRefV2{}, err
	}
	ref := runtimeports.CheckpointParticipantPhaseRefV2{ID: domain.Meta.ID + checkpointRuntimePhaseResultIDSuffixV2, Revision: runtimecore.Revision(domain.Meta.Revision), Phase: phase, State: state, Digest: digest}
	return reservation, runtimeReservation, ref, ref.Validate()
}

func checkpointRuntimePhaseStateV2(phase contract.CheckpointPhase, state contract.CheckpointPhaseState) (runtimeports.CheckpointParticipantPhaseV2, runtimeports.CheckpointParticipantPhaseStateV2, error) {
	runtimePhase, err := checkpointRuntimePhaseV1(phase)
	if err != nil {
		return "", "", err
	}
	var runtimeState runtimeports.CheckpointParticipantPhaseStateV2
	switch state {
	case contract.CheckpointPhasePrepared:
		runtimeState = runtimeports.CheckpointParticipantPreparedV2
	case contract.CheckpointPhaseCommitted:
		runtimeState = runtimeports.CheckpointParticipantCommittedV2
	case contract.CheckpointPhaseAborted:
		runtimeState = runtimeports.CheckpointParticipantAbortedV2
	case contract.CheckpointPhaseFailed:
		runtimeState = runtimeports.CheckpointParticipantFailedV2
	case contract.CheckpointPhaseNotApplied:
		runtimeState = runtimeports.CheckpointParticipantNotAppliedV2
	case contract.CheckpointPhaseUnknown, contract.CheckpointPhaseIndeterminate:
		runtimeState = runtimeports.CheckpointParticipantUnknownV2
	default:
		return "", "", fmt.Errorf("%w: checkpoint phase result state is invalid", ports.ErrConflict)
	}
	return runtimePhase, runtimeState, nil
}

func checkpointPhaseResultProjectionDigestV2(value runtimeports.CheckpointParticipantPhaseCurrentProjectionV2) (runtimecore.Digest, error) {
	value.ProjectionDigest = ""
	return runtimecore.CanonicalJSONDigest("praxis.runtime.checkpoint-governance", runtimeports.CheckpointGovernanceContractVersionV2, "CheckpointParticipantPhaseCurrentProjectionV2", value)
}

var _ runtimeports.CheckpointParticipantPhaseCurrentReaderV2 = (*CheckpointPhaseResultCurrentAdapterV2)(nil)
