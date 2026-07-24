package contract

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	GovernedContractVersionV3           = "praxis.harness.governed/v3"
	GovernedSessionV3CanonicalDomain    = "praxis.harness.governed-session"
	GovernedSessionV3CanonicalSubject   = "GovernedSessionV3"
	SessionCASContractVersionV3         = "praxis.harness.session-cas/v3"
	SessionCASRequestV3CanonicalDomain  = "praxis.harness.session-cas-request"
	SessionCASRequestV3CanonicalSubject = "SessionCASRequestV3"
)

type PendingActionApplicationBindingV1 struct {
	PendingAction          PendingActionV2                         `json:"pending_action"`
	IdentityRef            ModelToolCallPendingActionIdentityRefV1 `json:"identity_ref"`
	DomainResultFactRef    SettledTurnDomainResultFactRefV3        `json:"domain_result_fact_ref"`
	ModelTurnSettlementRef runtimeports.OperationSettlementRefV3   `json:"model_turn_settlement_ref"`
}

func (b PendingActionApplicationBindingV1) Clone() PendingActionApplicationBindingV1 {
	clone := b
	clone.PendingAction.Payload.Inline = append([]byte(nil), b.PendingAction.Payload.Inline...)
	clone.ModelTurnSettlementRef = cloneOperationSettlementRefV3(b.ModelTurnSettlementRef)
	return clone
}

func (b PendingActionApplicationBindingV1) Validate() error {
	if err := b.PendingAction.Validate(); err != nil {
		return err
	}
	if err := b.IdentityRef.Validate(); err != nil {
		return err
	}
	if err := b.DomainResultFactRef.Validate(); err != nil {
		return err
	}
	if err := b.ModelTurnSettlementRef.Validate(); err != nil {
		return err
	}
	if b.IdentityRef != b.DomainResultFactRef.IdentityRef || b.PendingAction.Ref != b.IdentityRef.PendingActionRef || b.PendingAction.RequestDigest != b.IdentityRef.PendingActionRequestDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "PendingAction Application binding identity differs")
	}
	settlement := b.ModelTurnSettlementRef
	if settlement.DomainResultSchema == nil || *settlement.DomainResultSchema != b.DomainResultFactRef.Schema || settlement.DomainResultDigest != b.DomainResultFactRef.ContentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "PendingAction Application binding Settlement differs from DomainResult")
	}
	return nil
}

type GovernedSessionV3 struct {
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
	ApplicationBinding     *PendingActionApplicationBindingV1           `json:"application_binding,omitempty"`
	PendingInput           *PendingInputV2                              `json:"pending_input,omitempty"`
	UndispatchedSettlement *UndispatchedSettlementBindingV2             `json:"undispatched_settlement,omitempty"`
	CompletionClaim        CompletionClaim                              `json:"completion_claim,omitempty"`
	CreatedUnixNano        int64                                        `json:"created_unix_nano"`
	UpdatedUnixNano        int64                                        `json:"updated_unix_nano"`
	Digest                 core.Digest                                  `json:"digest"`
}

func (s GovernedSessionV3) Clone() GovernedSessionV3 {
	clone := s
	clone.Run.Scope = cloneExecutionScopeV3(s.Run.Scope)
	clone.Endpoint.Scope = cloneExecutionScopeV3(s.Endpoint.Scope)
	if s.Candidate != nil {
		value := *s.Candidate
		clone.Candidate = &value
	}
	if s.DomainReservation != nil {
		value := *s.DomainReservation
		clone.DomainReservation = &value
	}
	if s.Execution != nil {
		value := cloneGovernedExecutionRefsV3(*s.Execution)
		clone.Execution = &value
	}
	if s.PendingAction != nil {
		value := *s.PendingAction
		value.Payload.Inline = append([]byte(nil), s.PendingAction.Payload.Inline...)
		clone.PendingAction = &value
	}
	if s.ApplicationBinding != nil {
		value := s.ApplicationBinding.Clone()
		clone.ApplicationBinding = &value
	}
	if s.PendingInput != nil {
		value := *s.PendingInput
		clone.PendingInput = &value
	}
	if s.UndispatchedSettlement != nil {
		value := *s.UndispatchedSettlement
		value.Settlement = cloneOperationSettlementRefV3(s.UndispatchedSettlement.Settlement)
		clone.UndispatchedSettlement = &value
	}
	return clone
}

func (s GovernedSessionV3) DigestV3() (core.Digest, error) {
	copy := s.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest(GovernedSessionV3CanonicalDomain, GovernedContractVersionV3, GovernedSessionV3CanonicalSubject, copy)
}

func (s GovernedSessionV3) Validate() error {
	if err := s.validateSubjectV3(); err != nil {
		return err
	}
	digest, err := s.DigestV3()
	if err != nil {
		return err
	}
	if digest != s.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "governed V3 Session digest drifted")
	}
	return nil
}

func (s GovernedSessionV3) validateSubjectV3() error {
	if s.ContractVersion != GovernedContractVersionV3 || strings.TrimSpace(s.ID) == "" || len(s.ID) > MaxReferenceBytes || s.Revision == 0 || s.CreatedUnixNano <= 0 || s.UpdatedUnixNano < s.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "governed V3 Session identity, revision and timestamps are required")
	}
	if err := s.Run.Validate(); err != nil {
		return err
	}
	if err := s.Endpoint.Validate(); err != nil {
		return err
	}
	if !runtimeports.SameExecutionScopeV2(s.Run.Scope, s.Endpoint.Scope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidReference, "governed V3 Session endpoint and Run scope differ")
	}
	if s.Candidate != nil {
		if err := s.Candidate.Validate(); err != nil {
			return err
		}
	}
	if s.DomainReservation != nil {
		if err := s.DomainReservation.Validate(); err != nil {
			return err
		}
		if s.Candidate == nil || s.DomainReservation.CandidateDigest != s.Candidate.Digest {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "governed V3 reservation differs from Candidate")
		}
	}
	if s.Execution != nil {
		if err := s.Execution.ValidatePrepared(); err != nil {
			return err
		}
	}
	if s.PendingAction != nil {
		if err := s.PendingAction.Validate(); err != nil {
			return err
		}
	}
	if s.ApplicationBinding != nil {
		if err := s.ApplicationBinding.Validate(); err != nil {
			return err
		}
	}
	if s.PendingInput != nil {
		if err := s.PendingInput.Validate(); err != nil {
			return err
		}
	}
	if s.UndispatchedSettlement != nil {
		if err := s.UndispatchedSettlement.Validate(); err != nil {
			return err
		}
	}
	if s.Phase != SessionWaitingActionV2 && s.ApplicationBinding != nil {
		return invalidSessionFieldsV3()
	}
	switch s.Phase {
	case SessionCreatingV2:
		if s.Turn != 0 || s.Candidate != nil || s.DomainReservation != nil || s.Execution != nil || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV3()
		}
	case SessionWaitingModelDispatchV2:
		if s.Turn == 0 || s.Candidate == nil || s.DomainReservation != nil || s.Execution != nil || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV3()
		}
	case SessionModelDispatchReservedV2:
		if s.Turn == 0 || s.Candidate == nil || s.DomainReservation == nil || s.Execution != nil || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV3()
		}
	case SessionModelInFlightV2:
		if s.Turn == 0 || s.Candidate == nil || s.DomainReservation == nil || s.Execution == nil || s.Execution.Observation != nil || s.Execution.Settlement != nil || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV3()
		}
	case SessionReconcilingV2:
		if s.Turn == 0 || s.Candidate == nil || s.DomainReservation == nil || s.Execution == nil || s.Execution.Settlement != nil || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV3()
		}
	case SessionWaitingSettlementV2:
		if s.Turn == 0 || s.Candidate == nil || s.DomainReservation == nil || !hasObservedUnsettledExecutionV2(s.Execution) || s.PendingAction != nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV3()
		}
	case SessionWaitingActionV2:
		if s.Turn == 0 || s.Candidate != nil || s.DomainReservation != nil || s.Execution == nil || s.Execution.Settlement == nil || s.PendingAction == nil || s.ApplicationBinding == nil || s.PendingInput != nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV3()
		}
		if !samePendingActionV3(*s.PendingAction, s.ApplicationBinding.PendingAction) || !sameOperationSettlementRefV3(*s.Execution.Settlement, s.ApplicationBinding.ModelTurnSettlementRef) {
			return invalidSessionFieldsV3()
		}
	case SessionWaitingInputV2:
		if s.Turn == 0 || s.Candidate != nil || s.DomainReservation != nil || !hasSettledExecutionV2(s.Execution) || s.PendingAction != nil || s.PendingInput == nil || s.UndispatchedSettlement != nil || s.CompletionClaim != "" {
			return invalidSessionFieldsV3()
		}
	case SessionTerminalV2:
		if s.Candidate != nil || s.DomainReservation != nil || s.PendingAction != nil || s.PendingInput != nil || !validClaim(s.CompletionClaim) || !validTerminalExecutionOrUndispatchedV3(s) {
			return invalidSessionFieldsV3()
		}
	default:
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "unknown governed V3 Session phase")
	}
	return nil
}

func SealGovernedSessionV3(session GovernedSessionV3) (GovernedSessionV3, error) {
	session.ContractVersion = GovernedContractVersionV3
	session.Digest = ""
	if err := session.validateSubjectV3(); err != nil {
		return GovernedSessionV3{}, err
	}
	digest, err := session.DigestV3()
	if err != nil {
		return GovernedSessionV3{}, err
	}
	session.Digest = digest
	return session.Clone(), session.Validate()
}

func ValidateSessionTransitionV3(current, next GovernedSessionV3) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if current.ID != next.ID || current.ContractVersion != next.ContractVersion || current.Run.RunID != next.Run.RunID || !runtimeports.SameExecutionScopeV2(current.Run.Scope, next.Run.Scope) || current.Endpoint.ID != next.Endpoint.ID || current.Endpoint.IdentityDigest != next.Endpoint.IdentityDigest || current.CreatedUnixNano != next.CreatedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "governed V3 Session immutable identity changed")
	}
	if next.Revision != current.Revision+1 || next.UpdatedUnixNano < current.UpdatedUnixNano || next.Turn < current.Turn || next.Turn > current.Turn+1 {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "governed V3 Session revision, time or turn is not monotonic")
	}
	allowed := false
	switch current.Phase {
	case SessionCreatingV2:
		allowed = next.Phase == SessionWaitingModelDispatchV2 && next.Turn == 1 || isCancellationTerminalV3(next) && next.Turn == current.Turn
	case SessionWaitingModelDispatchV2:
		allowed = next.Turn == current.Turn && (next.Phase == SessionModelDispatchReservedV2 || isCancellationTerminalV3(next))
	case SessionModelDispatchReservedV2:
		allowed = next.Turn == current.Turn && (next.Phase == SessionModelInFlightV2 || isCancellationTerminalV3(next) || isUndispatchedFailureTerminalV3(next))
	case SessionModelInFlightV2:
		allowed = next.Turn == current.Turn && (next.Phase == SessionWaitingSettlementV2 || next.Phase == SessionReconcilingV2 || isCancellationTerminalV3(next))
	case SessionWaitingSettlementV2:
		allowed = next.Turn == current.Turn && (next.Phase == SessionWaitingActionV2 || next.Phase == SessionWaitingInputV2 || next.Phase == SessionTerminalV2)
	case SessionWaitingActionV2, SessionWaitingInputV2:
		allowed = next.Phase == SessionWaitingModelDispatchV2 && next.Turn == current.Turn+1 || isCancellationTerminalV3(next) && next.Turn == current.Turn
	case SessionReconcilingV2:
		allowed = next.Turn == current.Turn && (next.Phase == SessionWaitingSettlementV2 || next.Phase == SessionWaitingActionV2 || next.Phase == SessionWaitingInputV2 || next.Phase == SessionTerminalV2)
	}
	if !allowed {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "governed V3 Session transition is not allowed")
	}
	if current.Phase == SessionWaitingSettlementV2 && next.Phase == SessionWaitingActionV2 {
		if err := validateWaitingActionLineageV3(current, next); err != nil {
			return err
		}
	}
	return nil
}

// validateWaitingActionLineageV3 proves every lineage coordinate available in
// the Harness Session itself. It deliberately does not claim to re-read the
// Identity or DomainResult owner facts; that exact owner read remains a
// coordinator/current-reader obligation before downstream dispatch.
func validateWaitingActionLineageV3(current, next GovernedSessionV3) error {
	if current.Candidate == nil || current.Execution == nil || current.Execution.Observation == nil || next.Execution == nil || next.Execution.Observation == nil || next.Execution.Settlement == nil || next.PendingAction == nil || next.ApplicationBinding == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "waiting_action successor lacks current-to-next lineage")
	}
	if next.PendingAction.SourceCandidate != *current.Candidate || next.ApplicationBinding.PendingAction.SourceCandidate != *current.Candidate {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "waiting_action successor changed the settled Candidate")
	}
	currentExecution := cloneGovernedExecutionRefsV3(*current.Execution)
	nextExecution := cloneGovernedExecutionRefsV3(*next.Execution)
	currentExecution.Settlement = nil
	nextExecution.Settlement = nil
	if !sameCanonicalV3("GovernedExecutionAttemptRefsWithoutSettlementV3", currentExecution, nextExecution) ||
		!sameCanonicalV3("ProviderAttemptObservationRefV2", *current.Execution.Observation, *next.Execution.Observation) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "waiting_action successor changed the observed execution attempt")
	}
	if next.Execution.Settlement.Observation == nil || !sameCanonicalV3("ProviderAttemptObservationRefV2", *current.Execution.Observation, *next.Execution.Settlement.Observation) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "waiting_action Settlement changed the observed execution result")
	}
	binding := next.ApplicationBinding
	if binding.IdentityRef.SourceKeyDigest != binding.DomainResultFactRef.SourceKeyDigest || binding.IdentityRef != binding.DomainResultFactRef.IdentityRef {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "waiting_action Identity and DomainResult source lineage differ")
	}
	return nil
}

func sameCanonicalV3(subject string, left, right any) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest(GovernedSessionV3CanonicalDomain, GovernedContractVersionV3, subject, left)
	rightDigest, rightErr := core.CanonicalJSONDigest(GovernedSessionV3CanonicalDomain, GovernedContractVersionV3, subject, right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

type SessionCASRequestV3 struct {
	ContractVersion  string            `json:"contract_version"`
	Run              RunRef            `json:"run"`
	SessionID        string            `json:"session_id"`
	ExpectedRevision core.Revision     `json:"expected_revision"`
	ExpectedDigest   core.Digest       `json:"expected_digest"`
	Next             GovernedSessionV3 `json:"next"`
	Digest           core.Digest       `json:"digest"`
}

func (r SessionCASRequestV3) Clone() SessionCASRequestV3 {
	clone := r
	clone.Run.Scope = cloneExecutionScopeV3(r.Run.Scope)
	clone.Next = r.Next.Clone()
	return clone
}
func (r SessionCASRequestV3) DigestV3() (core.Digest, error) {
	copy := r.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest(SessionCASRequestV3CanonicalDomain, SessionCASContractVersionV3, SessionCASRequestV3CanonicalSubject, copy)
}
func (r SessionCASRequestV3) Validate() error {
	if r.ContractVersion != SessionCASContractVersionV3 || strings.TrimSpace(r.SessionID) == "" || r.ExpectedRevision == 0 || r.ExpectedDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "governed V3 Session CAS exact expected fields are required")
	}
	if err := r.Run.Validate(); err != nil {
		return err
	}
	if err := r.Next.Validate(); err != nil {
		return err
	}
	if r.Next.Revision != r.ExpectedRevision+1 || r.Next.ID != r.SessionID || r.Next.Run.RunID != r.Run.RunID || !runtimeports.SameExecutionScopeV2(r.Next.Run.Scope, r.Run.Scope) {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "governed V3 Session CAS Next differs from expected subject")
	}
	digest, err := r.DigestV3()
	if err != nil {
		return err
	}
	if digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "governed V3 Session CAS digest drifted")
	}
	return nil
}

func SealSessionCASRequestV3(request SessionCASRequestV3) (SessionCASRequestV3, error) {
	request.ContractVersion = SessionCASContractVersionV3
	request.Digest = ""
	digest, err := request.DigestV3()
	if err != nil {
		return SessionCASRequestV3{}, err
	}
	request.Digest = digest
	return request.Clone(), request.Validate()
}

func invalidSessionFieldsV3() error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "governed V3 Session phase fields are inconsistent")
}
func isCancellationTerminalV3(s GovernedSessionV3) bool {
	return s.Phase == SessionTerminalV2 && s.CompletionClaim == ClaimCancelled
}
func isUndispatchedFailureTerminalV3(s GovernedSessionV3) bool {
	return s.Phase == SessionTerminalV2 && s.CompletionClaim == ClaimFailed && s.Execution == nil && s.UndispatchedSettlement != nil
}
func validTerminalExecutionOrUndispatchedV3(s GovernedSessionV3) bool {
	if s.CompletionClaim == ClaimCancelled {
		return s.UndispatchedSettlement == nil
	}
	if s.UndispatchedSettlement != nil {
		return s.CompletionClaim == ClaimFailed && s.Execution == nil
	}
	return hasTerminalExecutionV2(s.Execution, s.CompletionClaim)
}
func samePendingActionV3(a, b PendingActionV2) bool {
	ad, ae := core.CanonicalJSONDigest(GovernedSessionV3CanonicalDomain, GovernedContractVersionV3, "PendingActionV2", a)
	bd, be := core.CanonicalJSONDigest(GovernedSessionV3CanonicalDomain, GovernedContractVersionV3, "PendingActionV2", b)
	return ae == nil && be == nil && ad == bd
}
func sameOperationSettlementRefV3(a, b runtimeports.OperationSettlementRefV3) bool {
	ad, ae := core.CanonicalJSONDigest(GovernedSessionV3CanonicalDomain, GovernedContractVersionV3, "OperationSettlementRefV3", a)
	bd, be := core.CanonicalJSONDigest(GovernedSessionV3CanonicalDomain, GovernedContractVersionV3, "OperationSettlementRefV3", b)
	return ae == nil && be == nil && ad == bd
}

func cloneExecutionScopeV3(value core.ExecutionScope) core.ExecutionScope {
	clone := value
	if value.SandboxLease != nil {
		lease := *value.SandboxLease
		clone.SandboxLease = &lease
	}
	return clone
}
func cloneGovernedExecutionRefsV3(value runtimeports.GovernedExecutionAttemptRefsV2) runtimeports.GovernedExecutionAttemptRefsV2 {
	clone := value
	if value.Observation != nil {
		observation := *value.Observation
		clone.Observation = &observation
	}
	if value.Settlement != nil {
		settlement := cloneOperationSettlementRefV3(*value.Settlement)
		clone.Settlement = &settlement
	}
	return clone
}
func cloneOperationSettlementRefV3(value runtimeports.OperationSettlementRefV3) runtimeports.OperationSettlementRefV3 {
	clone := value
	clone.Attempt = cloneOperationDispatchAttemptRefV3(value.Attempt)
	if value.Observation != nil {
		observation := *value.Observation
		clone.Observation = &observation
	}
	if value.InspectionEffect != nil {
		inspection := cloneOperationDispatchAttemptRefV3(*value.InspectionEffect)
		clone.InspectionEffect = &inspection
	}
	if value.InspectionSettlement != nil {
		inspection := cloneOperationInspectionSettlementRefV3(*value.InspectionSettlement)
		clone.InspectionSettlement = &inspection
	}
	clone.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), value.Evidence...)
	if value.DomainResultSchema != nil {
		schema := *value.DomainResultSchema
		clone.DomainResultSchema = &schema
	}
	return clone
}
func cloneOperationInspectionSettlementRefV3(value runtimeports.OperationInspectionSettlementRefV3) runtimeports.OperationInspectionSettlementRefV3 {
	clone := value
	clone.Attempt = cloneOperationDispatchAttemptRefV3(value.Attempt)
	if value.Observation != nil {
		observation := *value.Observation
		clone.Observation = &observation
	}
	clone.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), value.Evidence...)
	if value.DomainResultSchema != nil {
		schema := *value.DomainResultSchema
		clone.DomainResultSchema = &schema
	}
	return clone
}
func cloneOperationDispatchAttemptRefV3(value runtimeports.OperationDispatchAttemptRefV3) runtimeports.OperationDispatchAttemptRefV3 {
	clone := value
	if value.Delegation != nil {
		delegation := *value.Delegation
		clone.Delegation = &delegation
	}
	return clone
}
