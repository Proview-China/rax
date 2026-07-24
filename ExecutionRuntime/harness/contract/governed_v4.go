package contract

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	GovernedContractVersionV4           = "praxis.harness.governed/v4"
	GovernedSessionV4CanonicalDomain    = "praxis.harness.governed-session-v4"
	GovernedSessionV4CanonicalSubject   = "GovernedSessionV4"
	SessionCASContractVersionV4         = "praxis.harness.session-cas/v4"
	SessionCASRequestV4CanonicalDomain  = "praxis.harness.session-cas-request-v4"
	SessionCASRequestV4CanonicalSubject = "SessionCASRequestV4"
)

type GovernedSessionV4 struct {
	ContractVersion        string                                       `json:"contract_version"`
	ID                     string                                       `json:"session_id"`
	Revision               core.Revision                                `json:"revision"`
	Run                    RunRef                                       `json:"run"`
	Endpoint               EndpointRefV2                                `json:"endpoint"`
	Phase                  SessionPhaseV2                               `json:"phase"`
	Turn                   uint32                                       `json:"turn"`
	Candidate              *CandidateRefV2                              `json:"candidate,omitempty"`
	DomainReservation      *ModelDispatchReservationRefV2               `json:"domain_reservation,omitempty"`
	Execution              *runtimeports.GovernedExecutionAttemptRefsV2 `json:"execution,omitempty"`
	PendingAction          *PendingActionV2                             `json:"pending_action,omitempty"`
	ApplicationBinding     *PendingActionApplicationBindingV2           `json:"application_binding,omitempty"`
	PendingInput           *PendingInputV2                              `json:"pending_input,omitempty"`
	UndispatchedSettlement *UndispatchedSettlementBindingV2             `json:"undispatched_settlement,omitempty"`
	CompletionClaim        CompletionClaim                              `json:"completion_claim,omitempty"`
	CreatedUnixNano        int64                                        `json:"created_unix_nano"`
	UpdatedUnixNano        int64                                        `json:"updated_unix_nano"`
	Digest                 core.Digest                                  `json:"digest"`
}

func (s GovernedSessionV4) Clone() GovernedSessionV4 {
	clone := s
	clone.Run.Scope = cloneExecutionScopeV3(s.Run.Scope)
	clone.Endpoint.Scope = cloneExecutionScopeV3(s.Endpoint.Scope)
	if s.Candidate != nil {
		v := *s.Candidate
		clone.Candidate = &v
	}
	if s.DomainReservation != nil {
		v := *s.DomainReservation
		clone.DomainReservation = &v
	}
	if s.Execution != nil {
		v := cloneGovernedExecutionRefsV3(*s.Execution)
		clone.Execution = &v
	}
	if s.PendingAction != nil {
		v := *s.PendingAction
		v.Payload.Inline = append([]byte(nil), s.PendingAction.Payload.Inline...)
		clone.PendingAction = &v
	}
	if s.ApplicationBinding != nil {
		v := s.ApplicationBinding.Clone()
		clone.ApplicationBinding = &v
	}
	if s.PendingInput != nil {
		v := *s.PendingInput
		clone.PendingInput = &v
	}
	if s.UndispatchedSettlement != nil {
		v := *s.UndispatchedSettlement
		v.Settlement = cloneOperationSettlementRefV3(v.Settlement)
		clone.UndispatchedSettlement = &v
	}
	return clone
}

func (s GovernedSessionV4) DigestV4() (core.Digest, error) {
	copy := s.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest(GovernedSessionV4CanonicalDomain, GovernedContractVersionV4, GovernedSessionV4CanonicalSubject, copy)
}

func (s GovernedSessionV4) Validate() error {
	if err := s.validateSubjectV4(); err != nil {
		return err
	}
	digest, err := s.DigestV4()
	if err != nil || digest != s.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "governed V4 Session digest drifted")
	}
	return nil
}

func (s GovernedSessionV4) validateSubjectV4() error {
	if s.ContractVersion != GovernedContractVersionV4 || strings.TrimSpace(s.ID) == "" || len(s.ID) > MaxReferenceBytes || s.Revision == 0 || s.CreatedUnixNano <= 0 || s.UpdatedUnixNano < s.CreatedUnixNano || s.Run.Validate() != nil || s.Endpoint.Validate() != nil || !runtimeports.SameExecutionScopeV2(s.Run.Scope, s.Endpoint.Scope) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "governed V4 Session identity is invalid")
	}
	if s.Candidate != nil && s.Candidate.Validate() != nil {
		return invalidSessionFieldsV4()
	}
	if s.DomainReservation != nil && (s.DomainReservation.Validate() != nil || s.Candidate == nil || s.DomainReservation.CandidateDigest != s.Candidate.Digest) {
		return invalidSessionFieldsV4()
	}
	if s.Execution != nil && s.Execution.ValidatePrepared() != nil {
		return invalidSessionFieldsV4()
	}
	if s.PendingAction != nil && s.PendingAction.Validate() != nil {
		return invalidSessionFieldsV4()
	}
	if s.ApplicationBinding != nil && s.ApplicationBinding.Validate() != nil {
		return invalidSessionFieldsV4()
	}
	if s.PendingInput != nil && s.PendingInput.Validate() != nil {
		return invalidSessionFieldsV4()
	}
	if s.UndispatchedSettlement != nil && s.UndispatchedSettlement.Validate() != nil {
		return invalidSessionFieldsV4()
	}
	if s.Phase != SessionWaitingActionV2 && s.ApplicationBinding != nil {
		return invalidSessionFieldsV4()
	}
	switch s.Phase {
	case SessionCreatingV2:
		if s.Turn != 0 || s.Candidate != nil || s.DomainReservation != nil || s.Execution != nil || s.PendingAction != nil || s.ApplicationBinding != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV4()
		}
	case SessionWaitingModelDispatchV2:
		if s.Turn == 0 || s.Candidate == nil || s.DomainReservation != nil || s.Execution != nil || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV4()
		}
	case SessionModelDispatchReservedV2:
		if s.Turn == 0 || s.Candidate == nil || s.DomainReservation == nil || s.Execution != nil || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV4()
		}
	case SessionModelInFlightV2:
		if s.Turn == 0 || s.Candidate == nil || s.DomainReservation == nil || s.Execution == nil || s.Execution.Observation != nil || s.Execution.Settlement != nil || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV4()
		}
	case SessionReconcilingV2:
		if s.Turn == 0 || s.Candidate == nil || s.DomainReservation == nil || s.Execution == nil || s.Execution.Settlement != nil || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV4()
		}
	case SessionWaitingSettlementV2:
		if s.Turn == 0 || s.Candidate == nil || s.DomainReservation == nil || !hasObservedUnsettledExecutionV2(s.Execution) || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV4()
		}
	case SessionWaitingActionV2:
		if s.Turn == 0 || s.Candidate != nil || s.DomainReservation != nil || s.Execution == nil || s.Execution.Settlement == nil || s.PendingAction == nil || s.ApplicationBinding == nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" || !samePendingActionV3(*s.PendingAction, s.ApplicationBinding.Base.PendingAction) || !sameOperationSettlementRefV3(*s.Execution.Settlement, s.ApplicationBinding.Base.ModelTurnSettlementRef) || !sameWaitingActionOperationScopeV4(s) {
			return invalidSessionFieldsV4()
		}
	case SessionWaitingInputV2:
		if s.Turn == 0 || s.Candidate != nil || s.DomainReservation != nil || !hasSettledExecutionV2(s.Execution) || s.PendingAction != nil || s.PendingInput == nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV4()
		}
	case SessionTerminalV2:
		if s.Candidate != nil || s.DomainReservation != nil || s.PendingAction != nil || s.ApplicationBinding != nil || s.PendingInput != nil || !validClaim(s.CompletionClaim) || !validTerminalExecutionOrUndispatchedV4(s) {
			return invalidSessionFieldsV4()
		}
	default:
		return invalidSessionFieldsV4()
	}
	return nil
}

func sameWaitingActionOperationScopeV4(s GovernedSessionV4) bool {
	if s.ApplicationBinding == nil {
		return false
	}
	operation := s.ApplicationBinding.OwnerCurrentInputs.ModelTurnOperation
	if operation.Kind != runtimeports.OperationScopeRunV3 || operation.RunID != s.Run.RunID || !runtimeports.SameExecutionScopeV2(operation.ExecutionScope, s.Run.Scope) {
		return false
	}
	digest, err := runtimeports.ExecutionScopeDigestV2(s.Run.Scope)
	return err == nil && operation.ExecutionScopeDigest == digest
}

func SealGovernedSessionV4(s GovernedSessionV4) (GovernedSessionV4, error) {
	s = s.Clone()
	s.ContractVersion = GovernedContractVersionV4
	s.Digest = ""
	if err := s.validateSubjectV4(); err != nil {
		return GovernedSessionV4{}, err
	}
	digest, err := s.DigestV4()
	if err != nil {
		return GovernedSessionV4{}, err
	}
	s.Digest = digest
	return s.Clone(), s.Validate()
}

func ValidateSessionTransitionV4(current, next GovernedSessionV4) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.ID != next.ID || current.Run.RunID != next.Run.RunID || !runtimeports.SameExecutionScopeV2(current.Run.Scope, next.Run.Scope) || current.Endpoint.ID != next.Endpoint.ID || current.Endpoint.IdentityDigest != next.Endpoint.IdentityDigest || current.CreatedUnixNano != next.CreatedUnixNano || next.Revision != current.Revision+1 || next.UpdatedUnixNano < current.UpdatedUnixNano || next.Turn < current.Turn || next.Turn > current.Turn+1 {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "governed V4 Session identity or monotonic lineage changed")
	}
	allowed := false
	switch current.Phase {
	case SessionCreatingV2:
		allowed = next.Phase == SessionWaitingModelDispatchV2 && next.Turn == 1 || isCancellationTerminalV4(next) && next.Turn == current.Turn
	case SessionWaitingModelDispatchV2:
		allowed = next.Turn == current.Turn && (next.Phase == SessionModelDispatchReservedV2 || isCancellationTerminalV4(next))
	case SessionModelDispatchReservedV2:
		allowed = next.Turn == current.Turn && (next.Phase == SessionModelInFlightV2 || isCancellationTerminalV4(next) || isUndispatchedFailureTerminalV4(next))
	case SessionModelInFlightV2:
		allowed = next.Turn == current.Turn && (next.Phase == SessionWaitingSettlementV2 || next.Phase == SessionReconcilingV2 || isCancellationTerminalV4(next))
	case SessionWaitingSettlementV2:
		allowed = next.Turn == current.Turn && (next.Phase == SessionWaitingActionV2 || next.Phase == SessionWaitingInputV2 || next.Phase == SessionTerminalV2)
	case SessionWaitingActionV2, SessionWaitingInputV2:
		allowed = next.Phase == SessionWaitingModelDispatchV2 && next.Turn == current.Turn+1 || isCancellationTerminalV4(next) && next.Turn == current.Turn
	case SessionReconcilingV2:
		allowed = next.Turn == current.Turn && (next.Phase == SessionWaitingSettlementV2 || next.Phase == SessionWaitingActionV2 || next.Phase == SessionWaitingInputV2 || next.Phase == SessionTerminalV2)
	}
	if !allowed {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "governed V4 Session transition is not allowed")
	}
	if (current.Phase == SessionWaitingSettlementV2 || current.Phase == SessionReconcilingV2) && next.Phase == SessionWaitingActionV2 {
		if current.Candidate == nil || current.Execution == nil || current.Execution.Observation == nil || next.Execution == nil || next.Execution.Settlement == nil || next.PendingAction == nil || next.ApplicationBinding == nil || next.PendingAction.SourceCandidate != *current.Candidate || next.ApplicationBinding.Base.PendingAction.SourceCandidate != *current.Candidate {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "governed V4 waiting_action Candidate lineage changed")
		}
		before := cloneGovernedExecutionRefsV3(*current.Execution)
		after := cloneGovernedExecutionRefsV3(*next.Execution)
		before.Settlement = nil
		after.Settlement = nil
		if !sameCanonicalV3("GovernedExecutionAttemptRefsWithoutSettlementV4", before, after) || next.Execution.Settlement.Observation == nil || !sameCanonicalV3("ProviderAttemptObservationRefV2", *current.Execution.Observation, *next.Execution.Settlement.Observation) {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "governed V4 observed attempt lineage changed")
		}
	}
	return nil
}

type SessionCASRequestV4 struct {
	ContractVersion  string            `json:"contract_version"`
	Run              RunRef            `json:"run"`
	SessionID        string            `json:"session_id"`
	ExpectedRevision core.Revision     `json:"expected_revision"`
	ExpectedDigest   core.Digest       `json:"expected_digest"`
	Next             GovernedSessionV4 `json:"next"`
	Digest           core.Digest       `json:"digest"`
}

func (r SessionCASRequestV4) Clone() SessionCASRequestV4 {
	r.Run.Scope = cloneExecutionScopeV3(r.Run.Scope)
	r.Next = r.Next.Clone()
	return r
}
func (r SessionCASRequestV4) DigestV4() (core.Digest, error) {
	c := r.Clone()
	c.Digest = ""
	return core.CanonicalJSONDigest(SessionCASRequestV4CanonicalDomain, SessionCASContractVersionV4, SessionCASRequestV4CanonicalSubject, c)
}
func (r SessionCASRequestV4) Validate() error {
	if r.ContractVersion != SessionCASContractVersionV4 || r.Run.Validate() != nil || strings.TrimSpace(r.SessionID) == "" || r.ExpectedRevision == 0 || r.ExpectedDigest.Validate() != nil || r.Next.Validate() != nil || r.Next.ID != r.SessionID || r.Next.Revision != r.ExpectedRevision+1 || r.Next.Run.RunID != r.Run.RunID || !runtimeports.SameExecutionScopeV2(r.Next.Run.Scope, r.Run.Scope) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "governed V4 CAS request is invalid")
	}
	d, e := r.DigestV4()
	if e != nil || d != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "governed V4 CAS request digest drifted")
	}
	return nil
}
func SealSessionCASRequestV4(r SessionCASRequestV4) (SessionCASRequestV4, error) {
	r = r.Clone()
	r.ContractVersion = SessionCASContractVersionV4
	r.Digest = ""
	d, e := r.DigestV4()
	if e != nil {
		return SessionCASRequestV4{}, e
	}
	r.Digest = d
	return r.Clone(), r.Validate()
}

func invalidSessionFieldsV4() error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "governed V4 Session fields do not match phase")
}
func validTerminalExecutionOrUndispatchedV4(s GovernedSessionV4) bool {
	if s.CompletionClaim == ClaimCancelled {
		return s.UndispatchedSettlement == nil
	}
	if s.UndispatchedSettlement != nil {
		return s.CompletionClaim == ClaimFailed && s.Execution == nil && s.UndispatchedSettlement.Validate() == nil
	}
	return hasTerminalExecutionV2(s.Execution, s.CompletionClaim)
}
func isCancellationTerminalV4(s GovernedSessionV4) bool {
	return s.Phase == SessionTerminalV2 && s.CompletionClaim == ClaimCancelled
}
func isUndispatchedFailureTerminalV4(s GovernedSessionV4) bool {
	return s.Phase == SessionTerminalV2 && s.CompletionClaim == ClaimFailed && s.Execution == nil && s.UndispatchedSettlement != nil
}
