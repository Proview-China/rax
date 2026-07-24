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

// CheckpointDomainResultCurrentAdapterV2 exposes only the Sandbox-owned
// immutable DomainResult as a Runtime-neutral current projection. Runtime
// retains Settlement authority; this reader cannot create a result or a phase
// Fact.
type CheckpointDomainResultCurrentAdapterV2 struct {
	phases       ports.CheckpointPhaseStore
	results      ports.CheckpointPhaseResultStoreV2
	reservations runtimeports.CheckpointParticipantPhaseReservationCurrentReaderV2
	clock        func() time.Time
}

func NewCheckpointDomainResultCurrentAdapterV2(phases ports.CheckpointPhaseStore, results ports.CheckpointPhaseResultStoreV2, reservations runtimeports.CheckpointParticipantPhaseReservationCurrentReaderV2, clock func() time.Time) (*CheckpointDomainResultCurrentAdapterV2, error) {
	if checkpointAdapterNilV1(phases) || checkpointAdapterNilV1(results) || checkpointAdapterNilV1(reservations) || clock == nil {
		return nil, errors.New("checkpoint DomainResult current adapter dependencies are required")
	}
	return &CheckpointDomainResultCurrentAdapterV2{phases: phases, results: results, reservations: reservations, clock: clock}, nil
}

func (a *CheckpointDomainResultCurrentAdapterV2) ReadCheckpointDomainResultCurrentV2(ctx context.Context, expected runtimeports.CheckpointParticipantDomainResultRefV2) (runtimeports.CheckpointParticipantDomainResultCurrentProjectionV2, error) {
	if a == nil || checkpointAdapterNilV1(ctx) || expected.Validate() != nil {
		return runtimeports.CheckpointParticipantDomainResultCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint DomainResult current request is invalid", ports.ErrConflict)
	}
	localExpected := contract.Ref{ID: expected.ID, Revision: uint64(expected.Revision), Digest: trimRuntimeDigestV1(expected.Digest)}
	domain, err := a.results.InspectCheckpointPhaseDomainResultByRefV2(ctx, localExpected)
	if err != nil {
		return runtimeports.CheckpointParticipantDomainResultCurrentProjectionV2{}, err
	}
	now := a.clock()
	if domain.ValidateCurrent(now) != nil {
		return runtimeports.CheckpointParticipantDomainResultCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint DomainResult is stale", ports.ErrStale)
	}
	reservation, err := a.phases.InspectCheckpointPhaseReservation(ctx, domain.ReservationRef)
	if err != nil || reservation.ValidateCurrent(now) != nil {
		return runtimeports.CheckpointParticipantDomainResultCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint DomainResult Reservation is stale", ports.ErrStale)
	}
	phase, err := checkpointRuntimePhaseV1(reservation.Phase)
	if err != nil {
		return runtimeports.CheckpointParticipantDomainResultCurrentProjectionV2{}, err
	}
	runtimeReservationRef := runtimeports.CheckpointParticipantPhaseReservationRefV2{ID: reservation.Meta.ID, Revision: runtimecore.Revision(reservation.Meta.Revision), Digest: runtimeDigest(reservation.Meta.Digest), ExpiresUnixNano: reservation.Meta.ExpiresUnixNano}
	runtimeReservation, err := a.reservations.InspectCheckpointParticipantPhaseReservationCurrentV2(ctx, runtimeReservationRef, phase)
	if err != nil || runtimeReservation.Validate(now) != nil || validateCheckpointDomainRuntimeMappingV2(domain, reservation, runtimeReservation) != nil {
		return runtimeports.CheckpointParticipantDomainResultCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint DomainResult Runtime mapping drifted", ports.ErrConflict)
	}
	mapped := checkpointRuntimeDomainResultRefV2(domain, runtimeReservation)
	if !sameRuntimeCheckpointDomainResultV2(mapped, expected) {
		return runtimeports.CheckpointParticipantDomainResultCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint DomainResult expected ref differs", ports.ErrConflict)
	}
	fresh := a.clock()
	domainS2, err := a.results.InspectCheckpointPhaseDomainResultByRefV2(ctx, localExpected)
	if err != nil {
		return runtimeports.CheckpointParticipantDomainResultCurrentProjectionV2{}, err
	}
	reservationS2, err := a.phases.InspectCheckpointPhaseReservation(ctx, domainS2.ReservationRef)
	if err != nil {
		return runtimeports.CheckpointParticipantDomainResultCurrentProjectionV2{}, err
	}
	runtimeReservationS2, err := a.reservations.InspectCheckpointParticipantPhaseReservationCurrentV2(ctx, runtimeReservationRef, phase)
	if err != nil || domainS2.ValidateCurrent(fresh) != nil || reservationS2.ValidateCurrent(fresh) != nil || runtimeReservationS2.Validate(fresh) != nil || validateCheckpointDomainRuntimeMappingV2(domainS2, reservationS2, runtimeReservationS2) != nil || !reflect.DeepEqual(domain, domainS2) || !reflect.DeepEqual(reservation, reservationS2) || !reflect.DeepEqual(runtimeReservation, runtimeReservationS2) {
		return runtimeports.CheckpointParticipantDomainResultCurrentProjectionV2{}, fmt.Errorf("%w: checkpoint DomainResult current changed between S1 and S2", ports.ErrConflict)
	}
	expires := min(domainS2.Meta.ExpiresUnixNano, reservationS2.Meta.ExpiresUnixNano, runtimeReservationS2.ExpiresUnixNano)
	projection := runtimeports.CheckpointParticipantDomainResultCurrentProjectionV2{Ref: mapped, Current: true, CheckedUnixNano: fresh.UnixNano(), ExpiresUnixNano: expires}
	digest, err := checkpointDomainResultProjectionDigestV2(projection)
	if err != nil {
		return runtimeports.CheckpointParticipantDomainResultCurrentProjectionV2{}, err
	}
	projection.ProjectionDigest = digest
	return projection, projection.Validate(now)
}

func checkpointDomainResultProjectionDigestV2(value runtimeports.CheckpointParticipantDomainResultCurrentProjectionV2) (runtimecore.Digest, error) {
	value.ProjectionDigest = ""
	return runtimecore.CanonicalJSONDigest("praxis.runtime.checkpoint-governance", runtimeports.CheckpointGovernanceContractVersionV2, "CheckpointParticipantDomainResultCurrentProjectionV2", value)
}

var _ runtimeports.CheckpointParticipantDomainResultCurrentReaderV2 = (*CheckpointDomainResultCurrentAdapterV2)(nil)
