package journal

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

// MemorySystemReadyAttemptStoreV2 is a thread-safe reference Fact Store. It is
// intentionally not a durable production backend.
type MemorySystemReadyAttemptStoreV2 struct {
	mu    sync.RWMutex
	items map[string]contract.SystemReadyAttemptFactV2
}

func NewMemorySystemReadyAttemptStoreV2() *MemorySystemReadyAttemptStoreV2 {
	return &MemorySystemReadyAttemptStoreV2{items: map[string]contract.SystemReadyAttemptFactV2{}}
}

func (s *MemorySystemReadyAttemptStoreV2) CreateSystemReadyAttemptV2(ctx context.Context, value contract.SystemReadyAttemptFactV2) (contract.SystemReadyAttemptFactV2, error) {
	if s == nil {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorUnavailable, "system_ready_attempt_store_missing", "SystemReady attempt store is unavailable")
	}
	if ctx == nil {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := value.Validate(); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	if value.State != contract.SystemReadyAttemptIntentRecordedV2 || value.Revision != 1 {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorPrecondition, "system_ready_attempt_initial_state_invalid", "SystemReady attempt must be created at intent_recorded revision one")
	}
	id := contract.DeriveSystemReadyAttemptIDV2(value.StepKey)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[id]; exists {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorConflict, "system_ready_attempt_exists", "SystemReady attempt already exists")
	}
	s.items[id] = value.CloneV2()
	return value.CloneV2(), nil
}

func (s *MemorySystemReadyAttemptStoreV2) CompareAndSwapSystemReadyAttemptV2(ctx context.Context, expected contract.ExactRefV1, next contract.SystemReadyAttemptFactV2) (contract.SystemReadyAttemptFactV2, error) {
	if s == nil {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorUnavailable, "system_ready_attempt_store_missing", "SystemReady attempt store is unavailable")
	}
	if ctx == nil {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := expected.Validate(); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	if err := next.Validate(); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	id := contract.DeriveSystemReadyAttemptIDV2(next.StepKey)
	if expected.Kind != contract.SystemReadyAttemptRefKindV2 || expected.ID != id {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorConflict, "system_ready_attempt_expected_ref_drift", "SystemReady attempt expected Ref drifted")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.items[id]
	if !exists {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorNotFound, "system_ready_attempt_missing", "SystemReady attempt is missing")
	}
	if current.RefV2() != expected {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorConflict, "system_ready_attempt_cas_conflict", "SystemReady attempt expected revision or digest drifted")
	}
	if err := contract.ValidateSystemReadyAttemptSuccessorV2(current, next); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	s.items[id] = next.CloneV2()
	return next.CloneV2(), nil
}

func (s *MemorySystemReadyAttemptStoreV2) InspectSystemReadyAttemptV2(ctx context.Context, key contract.SystemReadyAttemptStepKeyV2) (contract.SystemReadyAttemptFactV2, error) {
	if s == nil {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorUnavailable, "system_ready_attempt_store_missing", "SystemReady attempt store is unavailable")
	}
	if ctx == nil {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := key.Validate(); err != nil {
		return contract.SystemReadyAttemptFactV2{}, err
	}
	id := contract.DeriveSystemReadyAttemptIDV2(key)
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, exists := s.items[id]
	if !exists {
		return contract.SystemReadyAttemptFactV2{}, contract.NewError(contract.ErrorNotFound, "system_ready_attempt_missing", "SystemReady attempt is missing")
	}
	return value.CloneV2(), nil
}

var _ ports.SystemReadyAttemptFactPortV2 = (*MemorySystemReadyAttemptStoreV2)(nil)
