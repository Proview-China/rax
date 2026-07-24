package fakes

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// BindingAdmissionAttemptStoreV1 is a thread-safe conformance fixture. It is
// not a production persistence backend and never upgrades the legacy fake.
type BindingAdmissionAttemptStoreV1 struct {
	mu            sync.RWMutex
	items         map[string]control.BindingAdmissionAttemptFactV1
	loseCreate    bool
	loseCAS       bool
	createCommits uint64
	casCommits    uint64
}

func NewBindingAdmissionAttemptStoreV1() *BindingAdmissionAttemptStoreV1 {
	return &BindingAdmissionAttemptStoreV1{items: map[string]control.BindingAdmissionAttemptFactV1{}}
}

func (s *BindingAdmissionAttemptStoreV1) LoseNextCreateReplyV1() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.loseCreate = true
	s.mu.Unlock()
}

func (s *BindingAdmissionAttemptStoreV1) LoseNextCASReplyV1() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.loseCAS = true
	s.mu.Unlock()
}

func (s *BindingAdmissionAttemptStoreV1) CommitCountsV1() (uint64, uint64) {
	if s == nil {
		return 0, 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.createCommits, s.casCommits
}

func (s *BindingAdmissionAttemptStoreV1) CreateBindingAdmissionAttemptV1(ctx context.Context, value control.BindingAdmissionAttemptFactV1) (control.BindingAdmissionAttemptFactV1, error) {
	if err := contextError(ctx); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if s == nil {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Binding admission attempt store is unavailable")
	}
	if err := value.Validate(); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if value.Revision != 1 || value.State != control.BindingAdmissionIntentRecordedV1 {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Binding admission attempt must begin at intent_recorded revision one")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[value.AttemptID]; exists {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "Binding admission attempt already exists")
	}
	s.items[value.AttemptID] = value.CloneV1()
	s.createCommits++
	if s.loseCreate {
		s.loseCreate = false
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "injected lost Binding admission attempt create reply")
	}
	return value.CloneV1(), nil
}

func (s *BindingAdmissionAttemptStoreV1) CompareAndSwapBindingAdmissionAttemptV1(ctx context.Context, request control.BindingAdmissionAttemptCASRequestV1) (control.BindingAdmissionAttemptFactV1, error) {
	if err := contextError(ctx); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if s == nil {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Binding admission attempt store is unavailable")
	}
	if request.ExpectedRevision == 0 || request.ExpectedDigest.Validate() != nil || request.Next.Revision != request.ExpectedRevision+1 {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "Binding admission attempt CAS precondition is incomplete")
	}
	if err := request.Next.Validate(); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.items[request.Next.AttemptID]
	if !exists {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Binding admission attempt is absent")
	}
	if current.Revision != request.ExpectedRevision || current.Digest != request.ExpectedDigest {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Binding admission attempt CAS expected revision or digest drifted")
	}
	if err := control.ValidateBindingAdmissionAttemptSuccessorV1(current, request.Next); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	s.items[current.AttemptID] = request.Next.CloneV1()
	s.casCommits++
	if s.loseCAS {
		s.loseCAS = false
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "injected lost Binding admission attempt CAS reply")
	}
	return request.Next.CloneV1(), nil
}

func (s *BindingAdmissionAttemptStoreV1) InspectBindingAdmissionAttemptV1(ctx context.Context, attemptID string) (control.BindingAdmissionAttemptFactV1, error) {
	if err := contextError(ctx); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if s == nil || attemptID == "" {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission attempt lookup is invalid")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, exists := s.items[attemptID]
	if !exists {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Binding admission attempt is absent")
	}
	return value.CloneV1(), nil
}

var _ control.BindingAdmissionAttemptFactPortV1 = (*BindingAdmissionAttemptStoreV1)(nil)
