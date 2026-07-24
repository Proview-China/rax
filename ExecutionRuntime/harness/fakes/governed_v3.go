package fakes

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var _ harnessports.SessionFactPortV3 = (*GovernedStoreV2)(nil)

func (s *GovernedStoreV2) CreateSessionV3(_ context.Context, session contract.GovernedSessionV3) (contract.GovernedSessionV3, error) {
	if s == nil {
		return contract.GovernedSessionV3{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "governed V3 store is unavailable")
	}
	if err := session.Validate(); err != nil {
		return contract.GovernedSessionV3{}, err
	}
	if session.Revision != 1 || session.Phase != contract.SessionCreatingV2 || session.ApplicationBinding != nil {
		return contract.GovernedSessionV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "new governed V3 session must start at creating revision one without an Application binding")
	}
	key := governedRunKeyV2(session.Run) + "\x00" + session.ID
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[key]; ok {
		return contract.GovernedSessionV3{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "governed V2/V3 session key is already occupied")
	}
	if _, ok := s.sessionsV4[key]; ok {
		return contract.GovernedSessionV3{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "governed V2/V3/V4 session key is already occupied")
	}
	if current, ok := s.sessionsV3[key]; ok {
		if reflect.DeepEqual(current, session) {
			return current.Clone(), nil
		}
		return contract.GovernedSessionV3{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "governed V3 session already binds different content")
	}
	for _, current := range s.sessionsV3 {
		if current.Phase != contract.SessionTerminalV2 && governedScopeKeyV2(current.Run.Scope) == governedScopeKeyV2(session.Run.Scope) {
			return contract.GovernedSessionV3{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "run already has an active governed V3 session")
		}
	}
	s.sessionsV3[key] = session.Clone()
	if s.LoseNextSessionV3CreateReply {
		s.LoseNextSessionV3CreateReply = false
		return contract.GovernedSessionV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected governed V3 session create reply loss")
	}
	return session.Clone(), nil
}

func (s *GovernedStoreV2) InspectSessionV3(_ context.Context, run contract.RunRef, id string) (contract.GovernedSessionV3, error) {
	if s == nil {
		return contract.GovernedSessionV3{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "governed V3 store is unavailable")
	}
	if err := run.Validate(); err != nil {
		return contract.GovernedSessionV3{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessionsV3[governedRunKeyV2(run)+"\x00"+id]
	if !ok {
		return contract.GovernedSessionV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "governed V3 session not found")
	}
	return current.Clone(), nil
}

func (s *GovernedStoreV2) CompareAndSwapSessionV3(_ context.Context, request contract.SessionCASRequestV3) (contract.GovernedSessionV3, error) {
	if s == nil {
		return contract.GovernedSessionV3{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "governed V3 store is unavailable")
	}
	if err := request.Validate(); err != nil {
		return contract.GovernedSessionV3{}, err
	}
	key := governedRunKeyV2(request.Run) + "\x00" + request.SessionID
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.sessionsV3[key]
	if !ok {
		return contract.GovernedSessionV3{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "governed V3 session not found")
	}
	// Replaying the exact successor after an indeterminate reply is idempotent.
	if reflect.DeepEqual(current, request.Next) {
		return current.Clone(), nil
	}
	if current.Revision != request.ExpectedRevision || current.Digest != request.ExpectedDigest {
		return contract.GovernedSessionV3{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "governed V3 session revision or digest changed")
	}
	if err := contract.ValidateSessionTransitionV3(current, request.Next); err != nil {
		return contract.GovernedSessionV3{}, err
	}
	s.sessionsV3[key] = request.Next.Clone()
	if s.LoseNextSessionV3CASReply {
		s.LoseNextSessionV3CASReply = false
		return contract.GovernedSessionV3{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected governed V3 session CAS reply loss")
	}
	return request.Next.Clone(), nil
}
