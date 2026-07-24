package fakes

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var _ harnessports.SessionFactPortV4 = (*GovernedStoreV2)(nil)

func (s *GovernedStoreV2) CreateSessionV4(_ context.Context, session contract.GovernedSessionV4) (contract.GovernedSessionV4, error) {
	if s == nil {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "governed V4 store is unavailable")
	}
	if err := session.Validate(); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	if session.Revision != 1 || session.Phase != contract.SessionCreatingV2 || session.ApplicationBinding != nil {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "new governed V4 session must start at creating revision one without a binding")
	}
	key := governedRunKeyV2(session.Run) + "\x00" + session.ID
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[key]; ok {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "governed V2/V3/V4 key occupied")
	}
	if _, ok := s.sessionsV3[key]; ok {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "governed V2/V3/V4 key occupied")
	}
	if current, ok := s.sessionsV4[key]; ok {
		if reflect.DeepEqual(current, session) {
			return current.Clone(), nil
		}
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "governed V4 content differs")
	}
	for _, current := range s.sessionsV4 {
		if current.Phase != contract.SessionTerminalV2 && governedScopeKeyV2(current.Run.Scope) == governedScopeKeyV2(session.Run.Scope) {
			return contract.GovernedSessionV4{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "run already has active V4 session")
		}
	}
	s.sessionsV4[key] = session.Clone()
	if s.LoseNextSessionV4CreateReply {
		s.LoseNextSessionV4CreateReply = false
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected V4 create reply loss")
	}
	return session.Clone(), nil
}

func (s *GovernedStoreV2) InspectSessionV4(_ context.Context, run contract.RunRef, id string) (contract.GovernedSessionV4, error) {
	if s == nil {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "governed V4 store unavailable")
	}
	if err := run.Validate(); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessionsV4[governedRunKeyV2(run)+"\x00"+id]
	if !ok {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "governed V4 session not found")
	}
	return v.Clone(), nil
}

func (s *GovernedStoreV2) CompareAndSwapSessionV4(_ context.Context, request contract.SessionCASRequestV4) (contract.GovernedSessionV4, error) {
	if s == nil {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "governed V4 store unavailable")
	}
	if err := request.Validate(); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	key := governedRunKeyV2(request.Run) + "\x00" + request.SessionID
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessionsV4[key]
	if !ok {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "governed V4 session not found")
	}
	if reflect.DeepEqual(current, request.Next) {
		return current.Clone(), nil
	}
	if current.Revision != request.ExpectedRevision || current.Digest != request.ExpectedDigest {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "governed V4 revision or digest changed")
	}
	if err := contract.ValidateSessionTransitionV4(current, request.Next); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	s.sessionsV4[key] = request.Next.Clone()
	if s.LoseNextSessionV4CASReply {
		s.LoseNextSessionV4CASReply = false
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected V4 CAS reply loss")
	}
	return request.Next.Clone(), nil
}
