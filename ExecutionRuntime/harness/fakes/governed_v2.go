package fakes

import (
	"context"
	"sync"
	"time"

	bridgecontract "github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// GovernedStoreV2 is a deterministic test Fact Store. It deliberately makes
// no production durability or SLA claim.
type GovernedStoreV2 struct {
	mu                             sync.Mutex
	sessions                       map[string]contract.GovernedSessionV2
	sessionsV3                     map[string]contract.GovernedSessionV3
	sessionsV4                     map[string]contract.GovernedSessionV4
	candidates                     map[string]contract.ModelTurnCandidateV2
	reservations                   map[string]bridgecontract.ModelTurnOperationReservationFactV3
	reservationSubjects            map[string]string
	Clock                          func() time.Time
	LoseNextSessionCreateReply     bool
	LoseNextSessionCASReply        bool
	LoseNextSessionV3CreateReply   bool
	LoseNextSessionV3CASReply      bool
	LoseNextSessionV4CreateReply   bool
	LoseNextSessionV4CASReply      bool
	LoseNextCandidateCreateReply   bool
	LoseNextReservationCommitReply bool
}

func NewGovernedStoreV2() *GovernedStoreV2 {
	return &GovernedStoreV2{sessions: make(map[string]contract.GovernedSessionV2), sessionsV3: make(map[string]contract.GovernedSessionV3), sessionsV4: make(map[string]contract.GovernedSessionV4), candidates: make(map[string]contract.ModelTurnCandidateV2), reservations: make(map[string]bridgecontract.ModelTurnOperationReservationFactV3), reservationSubjects: make(map[string]string), Clock: time.Now}
}

var _ harnessports.SessionFactPortV2 = (*GovernedStoreV2)(nil)
var _ harnessports.CandidateFactPortV2 = (*GovernedStoreV2)(nil)
var _ harnessports.ModelTurnOperationReservationFactPortV3 = (*GovernedStoreV2)(nil)

func (s *GovernedStoreV2) CommitModelTurnOperationReservationV3(_ context.Context, request harnessports.CommitModelTurnOperationReservationRequestV3) (harnessports.CommittedModelTurnOperationReservationV3, error) {
	if err := request.Validate(); err != nil {
		return harnessports.CommittedModelTurnOperationReservationV3{}, err
	}
	sessionKey := governedRunKeyV2(request.NextSession.Run) + "\x00" + request.NextSession.ID
	reservationKey, err := modelTurnReservationKeyV3(request.Reservation.Scope, request.Reservation.StepKind, request.Reservation.Application.ID)
	if err != nil {
		return harnessports.CommittedModelTurnOperationReservationV3{}, err
	}
	subjectKey := string(request.Reservation.Reservation.DomainSubjectDigest)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[sessionKey]
	if !ok {
		return harnessports.CommittedModelTurnOperationReservationV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "reservation Session not found")
	}
	if existing, ok := s.reservations[reservationKey]; ok {
		if sameModelTurnReservationV3(existing, request.Reservation) && digestSessionV2(current) == digestSessionV2(request.NextSession) {
			return harnessports.CommittedModelTurnOperationReservationV3{Session: cloneGovernedSessionV2(current), Reservation: cloneModelTurnReservationV3(existing)}, nil
		}
		return harnessports.CommittedModelTurnOperationReservationV3{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "model-turn reservation replay changed content")
	}
	if current.Revision != request.ExpectedSessionRevision {
		return harnessports.CommittedModelTurnOperationReservationV3{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "reservation Session revision changed")
	}
	if err := contract.ValidateSessionTransitionV2(current, request.NextSession); err != nil {
		return harnessports.CommittedModelTurnOperationReservationV3{}, err
	}
	if owner, ok := s.reservationSubjects[subjectKey]; ok && owner != reservationKey {
		return harnessports.CommittedModelTurnOperationReservationV3{}, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "model-turn Session and Candidate are reserved by another attempt")
	}
	s.sessions[sessionKey] = cloneGovernedSessionV2(request.NextSession)
	s.reservations[reservationKey] = cloneModelTurnReservationV3(request.Reservation)
	s.reservationSubjects[subjectKey] = reservationKey
	if s.LoseNextReservationCommitReply {
		s.LoseNextReservationCommitReply = false
		return harnessports.CommittedModelTurnOperationReservationV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected atomic reservation commit reply loss")
	}
	return harnessports.CommittedModelTurnOperationReservationV3{Session: cloneGovernedSessionV2(request.NextSession), Reservation: cloneModelTurnReservationV3(request.Reservation)}, nil
}

func (s *GovernedStoreV2) InspectModelTurnOperationReservationV3(_ context.Context, scope core.ExecutionScope, step runtimeports.NamespacedNameV2, attemptID string) (bridgecontract.ModelTurnOperationReservationFactV3, error) {
	key, err := modelTurnReservationKeyV3(scope, step, attemptID)
	if err != nil {
		return bridgecontract.ModelTurnOperationReservationFactV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.reservations[key]
	if !ok {
		return bridgecontract.ModelTurnOperationReservationFactV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "model-turn reservation not found")
	}
	return cloneModelTurnReservationV3(fact), nil
}

func (s *GovernedStoreV2) CreateSessionV2(_ context.Context, session contract.GovernedSessionV2) (contract.GovernedSessionV2, error) {
	if err := session.Validate(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	if session.Revision != 1 || session.Phase != contract.SessionCreatingV2 {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "new governed session must start at creating revision one")
	}
	key := governedRunKeyV2(session.Run) + "\x00" + session.ID
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.sessions[key]; ok {
		if digestSessionV2(current) == digestSessionV2(session) {
			return cloneGovernedSessionV2(current), nil
		}
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "governed session already binds different content")
	}
	if _, ok := s.sessionsV3[key]; ok {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "governed V2/V3 session key is already occupied")
	}
	if _, ok := s.sessionsV4[key]; ok {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "governed V2/V3/V4 session key is already occupied")
	}
	for _, current := range s.sessions {
		if current.Phase != contract.SessionTerminalV2 && governedScopeKeyV2(current.Run.Scope) == governedScopeKeyV2(session.Run.Scope) {
			return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "run already has an active governed session")
		}
	}
	s.sessions[key] = cloneGovernedSessionV2(session)
	if s.LoseNextSessionCreateReply {
		s.LoseNextSessionCreateReply = false
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected session create reply loss")
	}
	return cloneGovernedSessionV2(session), nil
}

func (s *GovernedStoreV2) InspectSessionV2(_ context.Context, run contract.RunRef, id string) (contract.GovernedSessionV2, error) {
	if err := run.Validate(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[governedRunKeyV2(run)+"\x00"+id]
	if !ok {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "governed session not found")
	}
	return cloneGovernedSessionV2(current), nil
}

func (s *GovernedStoreV2) CompareAndSwapSessionV2(_ context.Context, request harnessports.SessionCASRequestV2) (contract.GovernedSessionV2, error) {
	if err := request.Validate(); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	key := governedRunKeyV2(request.Next.Run) + "\x00" + request.Next.ID
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessions[key]
	if !ok {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "governed session not found")
	}
	if current.Revision != request.ExpectedRevision {
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "governed session revision changed")
	}
	if err := contract.ValidateSessionTransitionV2(current, request.Next); err != nil {
		return contract.GovernedSessionV2{}, err
	}
	s.sessions[key] = cloneGovernedSessionV2(request.Next)
	if s.LoseNextSessionCASReply {
		s.LoseNextSessionCASReply = false
		return contract.GovernedSessionV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected session CAS reply loss")
	}
	return cloneGovernedSessionV2(request.Next), nil
}

func (s *GovernedStoreV2) CreateCandidateV2(_ context.Context, candidate contract.ModelTurnCandidateV2) (contract.ModelTurnCandidateV2, error) {
	if s.Clock == nil {
		return contract.ModelTurnCandidateV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "governed fake store clock is required")
	}
	if err := candidate.Validate(s.Clock()); err != nil {
		return contract.ModelTurnCandidateV2{}, err
	}
	key := governedRunKeyV2(candidate.Run) + "\x00" + candidate.ID
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.candidates[key]; ok {
		if digestCandidateV2(current) == digestCandidateV2(candidate) {
			return cloneModelCandidateV2(current), nil
		}
		return contract.ModelTurnCandidateV2{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "model candidate already binds different content")
	}
	s.candidates[key] = cloneModelCandidateV2(candidate)
	if s.LoseNextCandidateCreateReply {
		s.LoseNextCandidateCreateReply = false
		return contract.ModelTurnCandidateV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected candidate create reply loss")
	}
	return cloneModelCandidateV2(candidate), nil
}

func (s *GovernedStoreV2) InspectCandidateV2(_ context.Context, run contract.RunRef, id string) (contract.ModelTurnCandidateV2, error) {
	if err := run.Validate(); err != nil {
		return contract.ModelTurnCandidateV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.candidates[governedRunKeyV2(run)+"\x00"+id]
	if !ok {
		return contract.ModelTurnCandidateV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "model candidate not found")
	}
	return cloneModelCandidateV2(current), nil
}

func governedRunKeyV2(run contract.RunRef) string {
	digest, _ := core.CanonicalJSONDigest("praxis.harness.governed", contract.GovernedContractVersionV2, "RunKeyV2", run)
	return string(digest)
}

func governedScopeKeyV2(scope core.ExecutionScope) string {
	digest, _ := core.CanonicalJSONDigest("praxis.harness.governed", contract.GovernedContractVersionV2, "ExecutionScopeKeyV2", scope)
	return string(digest)
}

func digestSessionV2(value contract.GovernedSessionV2) core.Digest {
	digest, _ := core.CanonicalJSONDigest("praxis.harness.governed", contract.GovernedContractVersionV2, "GovernedSessionV2", value)
	return digest
}

func digestCandidateV2(value contract.ModelTurnCandidateV2) core.Digest {
	digest, _ := value.DigestV2()
	return digest
}

func cloneModelCandidateV2(value contract.ModelTurnCandidateV2) contract.ModelTurnCandidateV2 {
	clone := value
	clone.Run.Scope = cloneGovernedScopeV2(value.Run.Scope)
	clone.Endpoint.Scope = cloneGovernedScopeV2(value.Endpoint.Scope)
	clone.Input.Inline = append([]byte(nil), value.Input.Inline...)
	if value.Continuation != nil {
		continuation := *value.Continuation
		clone.Continuation = &continuation
	}
	return clone
}

func cloneGovernedSessionV2(value contract.GovernedSessionV2) contract.GovernedSessionV2 {
	clone := value
	clone.Run.Scope = cloneGovernedScopeV2(value.Run.Scope)
	clone.Endpoint.Scope = cloneGovernedScopeV2(value.Endpoint.Scope)
	if value.Candidate != nil {
		candidate := *value.Candidate
		clone.Candidate = &candidate
	}
	if value.DomainReservation != nil {
		reservation := *value.DomainReservation
		clone.DomainReservation = &reservation
	}
	if value.Execution != nil {
		execution := *value.Execution
		if value.Execution.Observation != nil {
			observation := *value.Execution.Observation
			execution.Observation = &observation
		}
		if value.Execution.Settlement != nil {
			settlement := *value.Execution.Settlement
			settlement.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), value.Execution.Settlement.Evidence...)
			if value.Execution.Settlement.Attempt.Delegation != nil {
				delegation := *value.Execution.Settlement.Attempt.Delegation
				settlement.Attempt.Delegation = &delegation
			}
			if value.Execution.Settlement.Observation != nil {
				observation := *value.Execution.Settlement.Observation
				settlement.Observation = &observation
			}
			if value.Execution.Settlement.DomainResultSchema != nil {
				schema := *value.Execution.Settlement.DomainResultSchema
				settlement.DomainResultSchema = &schema
			}
			execution.Settlement = &settlement
		}
		clone.Execution = &execution
	}
	if value.PendingAction != nil {
		action := *value.PendingAction
		action.Payload.Inline = append([]byte(nil), value.PendingAction.Payload.Inline...)
		clone.PendingAction = &action
	}
	if value.PendingInput != nil {
		input := *value.PendingInput
		clone.PendingInput = &input
	}
	if value.UndispatchedSettlement != nil {
		undispatched := *value.UndispatchedSettlement
		undispatched.Settlement = cloneOperationSettlementRefV3(value.UndispatchedSettlement.Settlement)
		clone.UndispatchedSettlement = &undispatched
	}
	return clone
}

func cloneGovernedScopeV2(value core.ExecutionScope) core.ExecutionScope {
	clone := value
	if value.SandboxLease != nil {
		lease := *value.SandboxLease
		clone.SandboxLease = &lease
	}
	return clone
}

func modelTurnReservationKeyV3(scope core.ExecutionScope, step runtimeports.NamespacedNameV2, attemptID string) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	if runtimeports.ValidateNamespacedNameV2(step) != nil || attemptID == "" {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "model-turn reservation key is incomplete")
	}
	digest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		return "", err
	}
	return string(digest) + "\x00" + string(step) + "\x00" + attemptID, nil
}

func sameModelTurnReservationV3(left, right bridgecontract.ModelTurnOperationReservationFactV3) bool {
	ld, le := left.DigestV3()
	rd, re := right.DigestV3()
	return le == nil && re == nil && ld == rd
}

func cloneModelTurnReservationV3(value bridgecontract.ModelTurnOperationReservationFactV3) bridgecontract.ModelTurnOperationReservationFactV3 {
	clone := value
	clone.Scope = cloneGovernedScopeV2(value.Scope)
	clone.Run.Scope = cloneGovernedScopeV2(value.Run.Scope)
	clone.Application = value.Application
	return clone
}
