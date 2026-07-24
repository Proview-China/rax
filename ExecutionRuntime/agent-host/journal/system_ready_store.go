package journal

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// MemorySystemReadyStoreV2 is a deterministic reference-test Fact Store. It is
// intentionally not a durable production backend.
type MemorySystemReadyStoreV2 struct {
	mu       sync.RWMutex
	owner    core.OwnerRef
	facts    map[string]contract.SystemReadyFactV2
	currents map[string]contract.SystemReadyCurrentV2
}

func NewMemorySystemReadyStoreV2(owner core.OwnerRef) (*MemorySystemReadyStoreV2, error) {
	if err := owner.Validate(); err != nil {
		return nil, err
	}
	return &MemorySystemReadyStoreV2{owner: owner, facts: map[string]contract.SystemReadyFactV2{}, currents: map[string]contract.SystemReadyCurrentV2{}}, nil
}

func (s *MemorySystemReadyStoreV2) InspectSystemReadyCurrentForAvailabilityV2(ctx context.Context, expected runtimeports.AgentExecutionAvailabilityRefV1) (contract.SystemReadyCurrentV2, error) {
	if s == nil {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorUnavailable, "system_ready_store_missing", "SystemReady store is nil")
	}
	if ctx == nil {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := expected.Validate(); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	if expected.Owner != s.owner {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "availability_owner_drift", "availability Ref belongs to another Owner")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	current, ok := s.currents[expected.ID]
	if !ok {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorNotFound, "system_ready_current_missing", "SystemReady Current does not exist")
	}
	projection, err := current.ToAgentExecutionAvailabilityV1(s.owner)
	if err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	if projection.Ref != expected {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "availability_projection_drift", "availability projection does not match the exact requested Ref")
	}
	return current, nil
}

func (s *MemorySystemReadyStoreV2) CreateSystemReadyFactV2(ctx context.Context, desired contract.SystemReadyFactV2) (contract.SystemReadyFactV2, error) {
	if s == nil {
		return contract.SystemReadyFactV2{}, contract.NewError(contract.ErrorUnavailable, "system_ready_store_missing", "SystemReady store is nil")
	}
	if ctx == nil {
		return contract.SystemReadyFactV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := desired.Validate(); err != nil {
		return contract.SystemReadyFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.facts[desired.Ref.ID]; ok {
		if current.Ref != desired.Ref || current.Digest != desired.Digest {
			return contract.SystemReadyFactV2{}, contract.NewError(contract.ErrorConflict, "system_ready_fact_conflict", "SystemReady Fact ID already binds another immutable fact")
		}
		return cloneSystemReadyFactV2(current), nil
	}
	s.facts[desired.Ref.ID] = cloneSystemReadyFactV2(desired)
	return cloneSystemReadyFactV2(desired), nil
}

func (s *MemorySystemReadyStoreV2) InspectSystemReadyFactV2(ctx context.Context, ref contract.SystemReadyFactRefV2) (contract.SystemReadyFactV2, error) {
	if s == nil {
		return contract.SystemReadyFactV2{}, contract.NewError(contract.ErrorUnavailable, "system_ready_store_missing", "SystemReady store is nil")
	}
	if ctx == nil {
		return contract.SystemReadyFactV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := ref.Validate(); err != nil {
		return contract.SystemReadyFactV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.facts[ref.ID]
	if !ok {
		return contract.SystemReadyFactV2{}, contract.NewError(contract.ErrorNotFound, "system_ready_fact_missing", "SystemReady Fact does not exist")
	}
	if value.Ref != ref {
		return contract.SystemReadyFactV2{}, contract.NewError(contract.ErrorConflict, "system_ready_fact_ref_drift", "SystemReady Fact exact Ref drifted")
	}
	return cloneSystemReadyFactV2(value), nil
}

func (s *MemorySystemReadyStoreV2) CreateSystemReadyCurrentV2(ctx context.Context, desired contract.SystemReadyCurrentV2) (contract.SystemReadyCurrentV2, error) {
	if s == nil {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorUnavailable, "system_ready_store_missing", "SystemReady store is nil")
	}
	if ctx == nil {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := desired.Validate(); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.facts[desired.FactRef.ID]
	if !ok {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorPrecondition, "system_ready_fact_missing", "SystemReady Current requires its immutable Fact")
	}
	if fact.Ref != desired.FactRef {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "system_ready_fact_ref_drift", "SystemReady Current names a non-exact Fact Ref")
	}
	if fact.HostID != desired.HostID || fact.StartID != desired.StartID {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "system_ready_subject_drift", "SystemReady Current subject does not match its immutable Fact")
	}
	if current, ok := s.currents[desired.Ref.ID]; ok {
		if current.Ref != desired.Ref || current.ProjectionDigest != desired.ProjectionDigest {
			return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "system_ready_current_conflict", "SystemReady Current ID already exists")
		}
		return current, nil
	}
	s.currents[desired.Ref.ID] = desired
	return desired, nil
}

func (s *MemorySystemReadyStoreV2) CompareAndSwapSystemReadyCurrentV2(ctx context.Context, expected contract.SystemReadyCurrentRefV2, next contract.SystemReadyCurrentV2) (contract.SystemReadyCurrentV2, error) {
	if s == nil {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorUnavailable, "system_ready_store_missing", "SystemReady store is nil")
	}
	if ctx == nil {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := expected.Validate(); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	if err := next.Validate(); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.currents[expected.ID]
	if !ok {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorNotFound, "system_ready_current_missing", "SystemReady Current does not exist")
	}
	if current.Ref != expected {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "system_ready_current_cas_conflict", "SystemReady Current expected Ref drifted")
	}
	fact, ok := s.facts[next.FactRef.ID]
	if !ok {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorPrecondition, "system_ready_fact_missing", "SystemReady Current successor requires its immutable Fact")
	}
	if fact.Ref != next.FactRef {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "system_ready_fact_ref_drift", "SystemReady Current successor names a non-exact Fact Ref")
	}
	if fact.HostID != next.HostID || fact.StartID != next.StartID {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "system_ready_subject_drift", "SystemReady Current successor subject does not match its immutable Fact")
	}
	if err := contract.ValidateSystemReadyCurrentSuccessorV2(current, next); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	s.currents[expected.ID] = next
	return next, nil
}

func (s *MemorySystemReadyStoreV2) InspectSystemReadyCurrentV2(ctx context.Context, ref contract.SystemReadyCurrentRefV2) (contract.SystemReadyCurrentV2, error) {
	if s == nil {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorUnavailable, "system_ready_store_missing", "SystemReady store is nil")
	}
	if ctx == nil {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if err := ref.Validate(); err != nil {
		return contract.SystemReadyCurrentV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	current, ok := s.currents[ref.ID]
	if !ok {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorNotFound, "system_ready_current_missing", "SystemReady Current does not exist")
	}
	if current.Ref != ref {
		return contract.SystemReadyCurrentV2{}, contract.NewError(contract.ErrorConflict, "system_ready_current_ref_drift", "SystemReady Current exact Ref drifted")
	}
	return current, nil
}

func cloneSystemReadyFactV2(value contract.SystemReadyFactV2) contract.SystemReadyFactV2 {
	value.Components = append([]contract.ComponentProductionCurrentV2(nil), value.Components...)
	return value
}
