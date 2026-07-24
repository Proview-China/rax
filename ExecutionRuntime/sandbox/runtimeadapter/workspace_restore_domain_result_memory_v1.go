package runtimeadapter

import (
	"context"
	"errors"
	"sync"
)

// MemoryWorkspaceRestoreStageRuntimeBindingStoreV1 is a reference backend for
// tests. Production composition must inject a durable State Plane Owner.
type MemoryWorkspaceRestoreStageRuntimeBindingStoreV1 struct {
	mu         sync.RWMutex
	items      map[workspaceRestoreStageRuntimeBindingKeyV1]WorkspaceRestoreStageRuntimeBindingV1
	loseCreate bool
}

type workspaceRestoreStageRuntimeBindingKeyV1 struct {
	TenantID string
	FactID   string
}

func NewMemoryWorkspaceRestoreStageRuntimeBindingStoreV1() *MemoryWorkspaceRestoreStageRuntimeBindingStoreV1 {
	return &MemoryWorkspaceRestoreStageRuntimeBindingStoreV1{items: make(map[workspaceRestoreStageRuntimeBindingKeyV1]WorkspaceRestoreStageRuntimeBindingV1)}
}

func (s *MemoryWorkspaceRestoreStageRuntimeBindingStoreV1) LoseNextCreateReplyV1() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseCreate = true
}

func (s *MemoryWorkspaceRestoreStageRuntimeBindingStoreV1) CreateWorkspaceRestoreStageRuntimeBindingV1(_ context.Context, value WorkspaceRestoreStageRuntimeBindingV1) (WorkspaceRestoreStageRuntimeBindingV1, error) {
	if s == nil || value.Validate() != nil {
		return WorkspaceRestoreStageRuntimeBindingV1{}, errors.New("workspace restore Stage Runtime binding is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := workspaceRestoreStageRuntimeBindingKeyV1{TenantID: value.TenantID, FactID: value.FactRef.ID}
	if current, ok := s.items[key]; ok {
		if current == value {
			return current, nil
		}
		return WorkspaceRestoreStageRuntimeBindingV1{}, errors.New("workspace restore Stage Runtime binding conflicts with existing content")
	}
	s.items[key] = value
	if s.loseCreate {
		s.loseCreate = false
		return WorkspaceRestoreStageRuntimeBindingV1{}, errors.New("injected lost workspace restore Stage binding create reply")
	}
	return value, nil
}

func (s *MemoryWorkspaceRestoreStageRuntimeBindingStoreV1) InspectWorkspaceRestoreStageRuntimeBindingV1(_ context.Context, tenantID, factID string) (WorkspaceRestoreStageRuntimeBindingV1, error) {
	if s == nil || tenantID == "" || factID == "" {
		return WorkspaceRestoreStageRuntimeBindingV1{}, errors.New("workspace restore Stage Runtime binding key is invalid")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.items[workspaceRestoreStageRuntimeBindingKeyV1{TenantID: tenantID, FactID: factID}]
	if !ok {
		return WorkspaceRestoreStageRuntimeBindingV1{}, errors.New("workspace restore Stage Runtime binding is absent")
	}
	return value, nil
}

var _ WorkspaceRestoreStageRuntimeBindingStoreV1 = (*MemoryWorkspaceRestoreStageRuntimeBindingStoreV1)(nil)
