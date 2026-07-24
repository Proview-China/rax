package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

// CheckpointPhaseStore is the Sandbox Owner's atomic persistence boundary for
// participant Reservations and phase Facts. It owns neither Runtime checkpoint
// governance nor Continuity manifests.
type CheckpointPhaseStore interface {
	ReserveCheckpointPhase(context.Context, contract.Ref, contract.CheckpointPhaseReservation, contract.CheckpointParticipantFact) (created bool, err error)
	InspectCheckpointPhaseReservation(context.Context, contract.Ref) (contract.CheckpointPhaseReservation, error)
	InspectCheckpointParticipant(context.Context, contract.Ref) (contract.CheckpointParticipantFact, error)
	InspectCheckpointParticipantCurrent(context.Context, string) (contract.CheckpointParticipantFact, error)
	InspectCheckpointPhaseFact(context.Context, contract.Ref) (contract.CheckpointPhaseFact, error)
	InspectCheckpointPhaseFactCurrent(context.Context, string) (contract.CheckpointPhaseFact, error)
	InspectCheckpointPhaseFactByReservation(context.Context, contract.Ref) (contract.CheckpointPhaseFact, error)
}

// CheckpointCurrentSource independently reads current projections from their
// actual Owners. It exposes no write or Provider method.
type CheckpointCurrentSource interface {
	InspectCheckpointCurrent(context.Context, contract.CheckpointCurrentQuery) (contract.CheckpointCurrentCoordinate, error)
}

type CheckpointParticipantCurrentReader interface {
	ReadCheckpointParticipantCurrent(context.Context, *contract.CheckpointCurrentReadRequest) (contract.CheckpointParticipantCurrentProjection, error)
}

type CheckpointConformanceRequest struct {
	ReservationRef contract.Ref `json:"reservation_ref"`
}

func (r CheckpointConformanceRequest) Validate() error {
	return r.ReservationRef.ValidateShape("checkpoint conformance request reservation ref")
}

// CheckpointParticipantConformancePort is pure local contract conformance. It
// cannot call a Provider or claim production isolation.
type CheckpointParticipantConformancePort interface {
	AssessCheckpointParticipant(context.Context, CheckpointConformanceRequest) (contract.CheckpointConformanceReport, error)
}
