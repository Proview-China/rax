package runtimeadapter

import (
	"context"
	"errors"
	"sync"
)

type MemoryWorkspaceRestorePreparedRuntimeBindingStoreV1 struct {
	mu         sync.RWMutex
	items      map[workspaceRestorePreparedBindingKeyV1]WorkspaceRestorePreparedRuntimeBindingV1
	loseCreate bool
}

type workspaceRestorePreparedBindingKeyV1 struct{ TenantID, AttemptID string }

func NewMemoryWorkspaceRestorePreparedRuntimeBindingStoreV1() *MemoryWorkspaceRestorePreparedRuntimeBindingStoreV1 {
	return &MemoryWorkspaceRestorePreparedRuntimeBindingStoreV1{items: make(map[workspaceRestorePreparedBindingKeyV1]WorkspaceRestorePreparedRuntimeBindingV1)}
}

func (s *MemoryWorkspaceRestorePreparedRuntimeBindingStoreV1) LoseNextCreateReplyV1() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseCreate = true
}

func (s *MemoryWorkspaceRestorePreparedRuntimeBindingStoreV1) CreateWorkspaceRestorePreparedRuntimeBindingV1(_ context.Context, value WorkspaceRestorePreparedRuntimeBindingV1) (WorkspaceRestorePreparedRuntimeBindingV1, error) {
	if s == nil || value.Validate() != nil {
		return WorkspaceRestorePreparedRuntimeBindingV1{}, errors.New("workspace restore prepared Runtime binding is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := workspaceRestorePreparedBindingKeyV1{TenantID: value.TenantID, AttemptID: value.Attempt.ID}
	if current, ok := s.items[key]; ok {
		if current == value {
			return current, nil
		}
		return WorkspaceRestorePreparedRuntimeBindingV1{}, errors.New("workspace restore prepared Runtime binding conflicts")
	}
	s.items[key] = value
	if s.loseCreate {
		s.loseCreate = false
		return WorkspaceRestorePreparedRuntimeBindingV1{}, errors.New("injected lost workspace restore prepared binding reply")
	}
	return value, nil
}

func (s *MemoryWorkspaceRestorePreparedRuntimeBindingStoreV1) InspectWorkspaceRestorePreparedRuntimeBindingV1(_ context.Context, tenantID, attemptID string) (WorkspaceRestorePreparedRuntimeBindingV1, error) {
	if s == nil || tenantID == "" || attemptID == "" {
		return WorkspaceRestorePreparedRuntimeBindingV1{}, errors.New("workspace restore prepared Runtime binding key is invalid")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.items[workspaceRestorePreparedBindingKeyV1{TenantID: tenantID, AttemptID: attemptID}]
	if !ok {
		return WorkspaceRestorePreparedRuntimeBindingV1{}, errors.New("workspace restore prepared Runtime binding is absent")
	}
	return value, nil
}

var _ WorkspaceRestorePreparedRuntimeBindingStoreV1 = (*MemoryWorkspaceRestorePreparedRuntimeBindingStoreV1)(nil)
