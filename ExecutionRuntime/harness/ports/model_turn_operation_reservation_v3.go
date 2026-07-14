package ports

import (
	"context"

	bridgecontract "github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ModelTurnOperationReservationFactPortV3 persists the recovery index for the
// Session-CAS reservation. Create is exact-idempotent; Inspect is mandatory
// after an uncertain create reply.
type CommitModelTurnOperationReservationRequestV3 struct {
	ExpectedSessionRevision core.Revision                                      `json:"expected_session_revision"`
	NextSession             contract.GovernedSessionV2                         `json:"next_session"`
	Reservation             bridgecontract.ModelTurnOperationReservationFactV3 `json:"reservation"`
}

func (r CommitModelTurnOperationReservationRequestV3) Validate() error {
	if r.ExpectedSessionRevision == 0 || r.NextSession.Revision != r.ExpectedSessionRevision+1 || r.NextSession.Phase != contract.SessionModelDispatchReservedV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "reservation commit Session revision and phase are incomplete")
	}
	if err := r.NextSession.Validate(); err != nil {
		return err
	}
	if err := r.Reservation.Validate(); err != nil {
		return err
	}
	local := r.NextSession.DomainReservation
	if !runtimeports.SameExecutionScopeV2(r.NextSession.Run.Scope, r.Reservation.Scope) || r.NextSession.Run.RunID != r.Reservation.Run.RunID || r.NextSession.ID != r.Reservation.SessionID || r.NextSession.Revision != r.Reservation.SessionRevision || r.NextSession.Candidate == nil || *r.NextSession.Candidate != r.Reservation.Candidate || local == nil || local.ID != r.Reservation.Reservation.ID || local.Digest != r.Reservation.Reservation.Digest || local.AttemptID != r.Reservation.Application.ID || local.IntentDigest != r.Reservation.Reservation.IntentDigest || local.CandidateDigest != r.Reservation.Candidate.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "reservation commit Session projection differs from bridge Fact")
	}
	return nil
}

type CommittedModelTurnOperationReservationV3 struct {
	Session     contract.GovernedSessionV2                         `json:"session"`
	Reservation bridgecontract.ModelTurnOperationReservationFactV3 `json:"reservation"`
}

func (r CommittedModelTurnOperationReservationV3) Validate() error {
	return (CommitModelTurnOperationReservationRequestV3{ExpectedSessionRevision: r.Session.Revision - 1, NextSession: r.Session, Reservation: r.Reservation}).Validate()
}

type ModelTurnOperationReservationFactPortV3 interface {
	CommitModelTurnOperationReservationV3(context.Context, CommitModelTurnOperationReservationRequestV3) (CommittedModelTurnOperationReservationV3, error)
	InspectModelTurnOperationReservationV3(context.Context, core.ExecutionScope, runtimeports.NamespacedNameV2, string) (bridgecontract.ModelTurnOperationReservationFactV3, error)
}
