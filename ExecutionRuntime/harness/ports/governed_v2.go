package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// CandidateFactPortV2 owns immutable model-turn candidates. A successful
// CreateCandidate is not a dispatch permit and not evidence of execution.
type CandidateFactPortV2 interface {
	CreateCandidateV2(context.Context, contract.ModelTurnCandidateV2) (contract.ModelTurnCandidateV2, error)
	InspectCandidateV2(context.Context, contract.RunRef, string) (contract.ModelTurnCandidateV2, error)
}

type SessionCASRequestV2 struct {
	ExpectedRevision core.Revision              `json:"expected_revision"`
	Next             contract.GovernedSessionV2 `json:"next"`
}

func (r SessionCASRequestV2) Validate() error {
	if r.ExpectedRevision == 0 || r.Next.Revision != r.ExpectedRevision+1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "session CAS expected and next revisions must be consecutive")
	}
	return r.Next.Validate()
}

// SessionFactPortV2 is the only Harness-owned authoritative Session store.
// Implementations must use contract.ValidateSessionTransitionV2 for every CAS.
type SessionFactPortV2 interface {
	CreateSessionV2(context.Context, contract.GovernedSessionV2) (contract.GovernedSessionV2, error)
	InspectSessionV2(context.Context, contract.RunRef, string) (contract.GovernedSessionV2, error)
	CompareAndSwapSessionV2(context.Context, SessionCASRequestV2) (contract.GovernedSessionV2, error)
}

type AttachPreparedAttemptRequestV2 struct {
	Run                     contract.RunRef                             `json:"run"`
	SessionID               string                                      `json:"session_id"`
	ExpectedSessionRevision core.Revision                               `json:"expected_session_revision"`
	Candidate               contract.CandidateRefV2                     `json:"candidate"`
	Reservation             contract.ModelDispatchReservationRefV2      `json:"reservation"`
	Attempt                 runtimeports.GovernedExecutionAttemptRefsV2 `json:"attempt"`
	UpdatedUnixNano         int64                                       `json:"updated_unix_nano"`
}

type AttachObservedAttemptRequestV2 struct {
	Run                     contract.RunRef                             `json:"run"`
	SessionID               string                                      `json:"session_id"`
	ExpectedSessionRevision core.Revision                               `json:"expected_session_revision"`
	Attempt                 runtimeports.GovernedExecutionAttemptRefsV2 `json:"attempt"`
	UpdatedUnixNano         int64                                       `json:"updated_unix_nano"`
}

type ApplySettledTurnRequestV2 struct {
	Run                     contract.RunRef                             `json:"run"`
	SessionID               string                                      `json:"session_id"`
	ExpectedSessionRevision core.Revision                               `json:"expected_session_revision"`
	Attempt                 runtimeports.GovernedExecutionAttemptRefsV2 `json:"attempt"`
	DomainResult            runtimeports.OpaquePayloadV2                `json:"domain_result"`
	UpdatedUnixNano         int64                                       `json:"updated_unix_nano"`
}

type ApplyUndispatchedSettlementRequestV2 struct {
	Run                     contract.RunRef                       `json:"run"`
	SessionID               string                                `json:"session_id"`
	ExpectedSessionRevision core.Revision                         `json:"expected_session_revision"`
	Candidate               contract.CandidateRefV2               `json:"candidate"`
	Settlement              runtimeports.OperationSettlementRefV3 `json:"settlement"`
	DomainResult            runtimeports.OpaquePayloadV2          `json:"domain_result"`
	UpdatedUnixNano         int64                                 `json:"updated_unix_nano"`
}

type MarkAttemptReconcilingRequestV2 struct {
	Run                     contract.RunRef `json:"run"`
	SessionID               string          `json:"session_id"`
	ExpectedSessionRevision core.Revision   `json:"expected_session_revision"`
	UpdatedUnixNano         int64           `json:"updated_unix_nano"`
}

// GovernedTurnStatePortV2 is the Harness-owned state attachment seam used by
// Application after Runtime governance commits. It does not execute a
// provider or grant Runtime/Settlement authority.
type GovernedTurnStatePortV2 interface {
	AttachPreparedAttemptV2(context.Context, AttachPreparedAttemptRequestV2) (contract.GovernedSessionV2, error)
	MarkAttemptReconcilingV2(context.Context, MarkAttemptReconcilingRequestV2) (contract.GovernedSessionV2, error)
	AttachObservedAttemptV2(context.Context, AttachObservedAttemptRequestV2) (contract.GovernedSessionV2, error)
	ApplySettledTurnV2(context.Context, ApplySettledTurnRequestV2) (contract.GovernedSessionV2, error)
	ApplyUndispatchedSettlementV2(context.Context, ApplyUndispatchedSettlementRequestV2) (contract.GovernedSessionV2, error)
}
