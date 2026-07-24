package runtimeadapter

import (
	"context"
	"errors"
	"sync"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// MemoryDomainResultBindingStoreV4 is a deterministic local/reference store.
// Production composition must inject a durable State Plane implementation.
type MemoryDomainResultBindingStoreV4 struct {
	mu    sync.RWMutex
	items map[string]runtimeports.OperationSettlementDomainResultFactRefV4
}

func NewMemoryDomainResultBindingStoreV4() *MemoryDomainResultBindingStoreV4 {
	return &MemoryDomainResultBindingStoreV4{items: make(map[string]runtimeports.OperationSettlementDomainResultFactRefV4)}
}

func (s *MemoryDomainResultBindingStoreV4) CreateDomainResultRuntimeBindingV4(_ context.Context, value runtimeports.OperationSettlementDomainResultFactRefV4) (runtimeports.OperationSettlementDomainResultFactRefV4, error) {
	if s == nil || value.Validate() != nil {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, errors.New("domain result binding is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.items[value.ID]; ok {
		if runtimeports.SameOperationSettlementDomainResultFactRefV4(existing, value) {
			return existing, nil
		}
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, errors.New("domain result binding conflicts with existing content")
	}
	s.items[value.ID] = value
	return value, nil
}

func (s *MemoryDomainResultBindingStoreV4) InspectDomainResultRuntimeBindingV4(_ context.Context, id string) (runtimeports.OperationSettlementDomainResultFactRefV4, error) {
	if s == nil || id == "" {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, errors.New("domain result binding key is invalid")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.items[id]
	if !ok {
		return runtimeports.OperationSettlementDomainResultFactRefV4{}, errors.New("domain result binding not found")
	}
	return value, nil
}

var _ DomainResultRuntimeBindingStoreV4 = (*MemoryDomainResultBindingStoreV4)(nil)
