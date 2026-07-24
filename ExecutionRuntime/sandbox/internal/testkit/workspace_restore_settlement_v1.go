package testkit

import (
	"context"
	"errors"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type WorkspaceRestoreSettlementMemoryStoreV1 struct {
	mu         sync.Mutex
	byID       map[workspaceRestoreSettlementIDKeyV1]contract.WorkspaceRestoreApplySettlementFactV1
	byStage    map[workspaceRestoreSettlementStageKeyV1]workspaceRestoreSettlementIDKeyV1
	loseCreate bool
}

type workspaceRestoreSettlementIDKeyV1 struct{ TenantID, ID string }
type workspaceRestoreSettlementStageKeyV1 struct {
	TenantID string
	Stage    contract.SnapshotArtifactExactRefV2
}

func NewWorkspaceRestoreSettlementMemoryStoreV1() *WorkspaceRestoreSettlementMemoryStoreV1 {
	return &WorkspaceRestoreSettlementMemoryStoreV1{byID: make(map[workspaceRestoreSettlementIDKeyV1]contract.WorkspaceRestoreApplySettlementFactV1), byStage: make(map[workspaceRestoreSettlementStageKeyV1]workspaceRestoreSettlementIDKeyV1)}
}

func (s *WorkspaceRestoreSettlementMemoryStoreV1) LoseNextCreateReplyV1() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseCreate = true
}

func (s *WorkspaceRestoreSettlementMemoryStoreV1) CreateWorkspaceRestoreApplySettlementV1(_ context.Context, fact contract.WorkspaceRestoreApplySettlementFactV1) (contract.WorkspaceRestoreApplySettlementFactV1, error) {
	if fact.ValidateShape() != nil {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, errors.New("workspace restore ApplySettlement Fact is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idKey := workspaceRestoreSettlementIDKeyV1{TenantID: fact.TenantID, ID: fact.Meta.ID}
	stageKey := workspaceRestoreSettlementStageKeyV1{TenantID: fact.TenantID, Stage: fact.StageFactRef}
	if current, ok := s.byID[idKey]; ok {
		if current == fact {
			return current, nil
		}
		return contract.WorkspaceRestoreApplySettlementFactV1{}, ports.ErrConflict
	}
	if _, ok := s.byStage[stageKey]; ok {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, ports.ErrConflict
	}
	s.byID[idKey] = fact
	s.byStage[stageKey] = idKey
	if s.loseCreate {
		s.loseCreate = false
		return contract.WorkspaceRestoreApplySettlementFactV1{}, errors.New("injected lost workspace restore ApplySettlement create reply")
	}
	return fact, nil
}

func (s *WorkspaceRestoreSettlementMemoryStoreV1) InspectWorkspaceRestoreApplySettlementV1(_ context.Context, tenantID, id string) (contract.WorkspaceRestoreApplySettlementFactV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.byID[workspaceRestoreSettlementIDKeyV1{TenantID: tenantID, ID: id}]
	if !ok {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, ports.ErrNotFound
	}
	return fact, nil
}

func (s *WorkspaceRestoreSettlementMemoryStoreV1) InspectWorkspaceRestoreApplySettlementByStageV1(_ context.Context, tenantID string, stage contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreApplySettlementFactV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byStage[workspaceRestoreSettlementStageKeyV1{TenantID: tenantID, Stage: stage}]
	if !ok {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, ports.ErrNotFound
	}
	return s.byID[id], nil
}

var _ ports.WorkspaceRestoreSettlementStoreV1 = (*WorkspaceRestoreSettlementMemoryStoreV1)(nil)
