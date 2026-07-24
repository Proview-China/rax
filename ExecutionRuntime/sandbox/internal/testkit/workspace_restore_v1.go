package testkit

import (
	"context"
	"errors"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type WorkspaceRestoreMemoryStoreV1 struct {
	mu      sync.Mutex
	current map[string]contract.WorkspaceRestoreAttemptV1
	history map[contract.SnapshotArtifactExactRefV2]contract.WorkspaceRestoreAttemptV1
	facts   map[contract.SnapshotArtifactExactRefV2]contract.WorkspaceRestoreStageFactV1
}

func NewWorkspaceRestoreMemoryStoreV1() *WorkspaceRestoreMemoryStoreV1 {
	return &WorkspaceRestoreMemoryStoreV1{
		current: make(map[string]contract.WorkspaceRestoreAttemptV1),
		history: make(map[contract.SnapshotArtifactExactRefV2]contract.WorkspaceRestoreAttemptV1),
		facts:   make(map[contract.SnapshotArtifactExactRefV2]contract.WorkspaceRestoreStageFactV1),
	}
}

func (s *WorkspaceRestoreMemoryStoreV1) CreateWorkspaceRestoreAttemptV1(_ context.Context, value contract.WorkspaceRestoreAttemptV1) (bool, error) {
	if err := value.ValidateShape(); err != nil || value.Meta.Revision != 1 || value.State != contract.WorkspaceRestoreAttemptPreparedV1 {
		return false, errors.New("invalid initial workspace restore attempt")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.current[value.StableKeyDigest]; ok {
		if existing.ExactRef() == value.ExactRef() {
			return false, nil
		}
		return false, ports.ErrConflict
	}
	clone := value.Clone()
	s.current[value.StableKeyDigest] = clone
	s.history[value.ExactRef()] = clone
	return true, nil
}

func (s *WorkspaceRestoreMemoryStoreV1) CASWorkspaceRestoreAttemptV1(_ context.Context, expected contract.SnapshotArtifactExactRefV2, next contract.WorkspaceRestoreAttemptV1) (bool, error) {
	if err := next.ValidateShape(); err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.current[next.StableKeyDigest]
	if !ok {
		return false, ports.ErrNotFound
	}
	if current.ExactRef() != expected {
		if current.ExactRef() == next.ExactRef() {
			return false, nil
		}
		return false, ports.ErrConflict
	}
	if err := validateWorkspaceRestoreTransitionV1(current, next, false); err != nil {
		return false, err
	}
	clone := next.Clone()
	s.current[next.StableKeyDigest] = clone
	s.history[next.ExactRef()] = clone
	return true, nil
}

func (s *WorkspaceRestoreMemoryStoreV1) CommitWorkspaceRestoreStageV1(_ context.Context, expected contract.SnapshotArtifactExactRefV2, next contract.WorkspaceRestoreAttemptV1, fact contract.WorkspaceRestoreStageFactV1) (bool, error) {
	if err := next.ValidateShape(); err != nil || fact.ValidateShape() != nil || next.StageFactRef == nil || *next.StageFactRef != fact.ExactRef() || next.ProviderStageAttemptRef == nil || fact.AttemptRef != *next.ProviderStageAttemptRef || next.RootRef == nil || *next.RootRef != fact.RootRef {
		return false, errors.New("invalid workspace restore final commit closure")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.current[next.StableKeyDigest]
	if !ok {
		return false, ports.ErrNotFound
	}
	if current.ExactRef() != expected {
		if current.ExactRef() == next.ExactRef() {
			if stored, ok := s.facts[fact.ExactRef()]; ok && stored.ExactRef() == fact.ExactRef() {
				return false, nil
			}
		}
		return false, ports.ErrConflict
	}
	if err := validateWorkspaceRestoreTransitionV1(current, next, true); err != nil {
		return false, err
	}
	if _, exists := s.facts[fact.ExactRef()]; exists {
		return false, ports.ErrConflict
	}
	s.facts[fact.ExactRef()] = fact.Clone()
	clone := next.Clone()
	s.current[next.StableKeyDigest] = clone
	s.history[next.ExactRef()] = clone
	return true, nil
}

func (s *WorkspaceRestoreMemoryStoreV1) InspectWorkspaceRestoreAttemptByStableKeyV1(_ context.Context, stable string) (contract.WorkspaceRestoreAttemptV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.current[stable]
	if !ok {
		return contract.WorkspaceRestoreAttemptV1{}, ports.ErrNotFound
	}
	return value.Clone(), nil
}

func (s *WorkspaceRestoreMemoryStoreV1) InspectWorkspaceRestoreAttemptV1(_ context.Context, ref contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreAttemptV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.history[ref]
	if !ok {
		return contract.WorkspaceRestoreAttemptV1{}, ports.ErrNotFound
	}
	return value.Clone(), nil
}

func (s *WorkspaceRestoreMemoryStoreV1) InspectWorkspaceRestoreStageFactV1(_ context.Context, ref contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreStageFactV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.facts[ref]
	if !ok {
		return contract.WorkspaceRestoreStageFactV1{}, ports.ErrNotFound
	}
	return value.Clone(), nil
}

func validateWorkspaceRestoreTransitionV1(current, next contract.WorkspaceRestoreAttemptV1, final bool) error {
	if current.Meta.ID != next.Meta.ID || next.Meta.Revision != current.Meta.Revision+1 || current.Meta.CreatedUnixNano != next.Meta.CreatedUnixNano || current.Meta.ExpiresUnixNano != next.Meta.ExpiresUnixNano || next.Meta.UpdatedUnixNano < current.Meta.UpdatedUnixNano || current.StableKeyDigest != next.StableKeyDigest || current.Request != next.Request || current.BundleProjectionDigest != next.BundleProjectionDigest || current.BundleDigest != next.BundleDigest {
		return ports.ErrConflict
	}
	if final {
		if (next.State != contract.WorkspaceRestoreAttemptStagedV1 && next.State != contract.WorkspaceRestoreAttemptPartialV1) || !sameWorkspaceRestoreGovernanceV1(current, next) || next.ProviderStageAttemptRef == nil {
			return ports.ErrConflict
		}
		if current.State == contract.WorkspaceRestoreAttemptInvocationV1 && *next.ProviderStageAttemptRef != current.ExactRef() {
			return ports.ErrConflict
		}
		if current.State == contract.WorkspaceRestoreAttemptReconcileRequiredV1 && (current.ProviderStageAttemptRef == nil || *next.ProviderStageAttemptRef != *current.ProviderStageAttemptRef) {
			return ports.ErrConflict
		}
		if current.State != contract.WorkspaceRestoreAttemptInvocationV1 && current.State != contract.WorkspaceRestoreAttemptReconcileRequiredV1 {
			return ports.ErrConflict
		}
		return nil
	}
	switch current.State {
	case contract.WorkspaceRestoreAttemptPreparedV1:
		if next.State != contract.WorkspaceRestoreAttemptGovernedV1 || current.Governance != nil || current.GovernanceProjectionDigest != "" || next.Governance == nil || next.ProviderStageAttemptRef != nil {
			return ports.ErrConflict
		}
	case contract.WorkspaceRestoreAttemptGovernedV1:
		if next.State != contract.WorkspaceRestoreAttemptInvocationV1 || !sameWorkspaceRestoreGovernanceV1(current, next) || next.ProviderStageAttemptRef != nil {
			return ports.ErrConflict
		}
	case contract.WorkspaceRestoreAttemptInvocationV1:
		if next.State != contract.WorkspaceRestoreAttemptReconcileRequiredV1 || !sameWorkspaceRestoreGovernanceV1(current, next) || next.ProviderStageAttemptRef == nil || *next.ProviderStageAttemptRef != current.ExactRef() {
			return ports.ErrConflict
		}
	default:
		return ports.ErrConflict
	}
	return nil
}

func sameWorkspaceRestoreGovernanceV1(current, next contract.WorkspaceRestoreAttemptV1) bool {
	return current.GovernanceProjectionDigest == next.GovernanceProjectionDigest && current.Governance != nil && next.Governance != nil && *current.Governance == *next.Governance
}

type WorkspaceRestoreLostReplyStoreV1 struct {
	Base             ports.WorkspaceRestoreStoreV1
	LoseCreateOnce   bool
	LoseCASStateOnce contract.WorkspaceRestoreAttemptStateV1
	LoseCommitOnce   bool
	mu               sync.Mutex
}

func (s *WorkspaceRestoreLostReplyStoreV1) CreateWorkspaceRestoreAttemptV1(ctx context.Context, value contract.WorkspaceRestoreAttemptV1) (bool, error) {
	created, err := s.Base.CreateWorkspaceRestoreAttemptV1(ctx, value)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil && created && s.LoseCreateOnce {
		s.LoseCreateOnce = false
		return false, errors.New("injected lost create reply")
	}
	return created, err
}

func (s *WorkspaceRestoreLostReplyStoreV1) CASWorkspaceRestoreAttemptV1(ctx context.Context, expected contract.SnapshotArtifactExactRefV2, next contract.WorkspaceRestoreAttemptV1) (bool, error) {
	created, err := s.Base.CASWorkspaceRestoreAttemptV1(ctx, expected, next)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil && created && s.LoseCASStateOnce == next.State {
		s.LoseCASStateOnce = ""
		return false, errors.New("injected lost CAS reply")
	}
	return created, err
}

func (s *WorkspaceRestoreLostReplyStoreV1) CommitWorkspaceRestoreStageV1(ctx context.Context, expected contract.SnapshotArtifactExactRefV2, next contract.WorkspaceRestoreAttemptV1, fact contract.WorkspaceRestoreStageFactV1) (bool, error) {
	created, err := s.Base.CommitWorkspaceRestoreStageV1(ctx, expected, next, fact)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil && created && s.LoseCommitOnce {
		s.LoseCommitOnce = false
		return false, errors.New("injected lost commit reply")
	}
	return created, err
}

func (s *WorkspaceRestoreLostReplyStoreV1) InspectWorkspaceRestoreAttemptByStableKeyV1(ctx context.Context, stable string) (contract.WorkspaceRestoreAttemptV1, error) {
	return s.Base.InspectWorkspaceRestoreAttemptByStableKeyV1(ctx, stable)
}
func (s *WorkspaceRestoreLostReplyStoreV1) InspectWorkspaceRestoreAttemptV1(ctx context.Context, ref contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreAttemptV1, error) {
	return s.Base.InspectWorkspaceRestoreAttemptV1(ctx, ref)
}
func (s *WorkspaceRestoreLostReplyStoreV1) InspectWorkspaceRestoreStageFactV1(ctx context.Context, ref contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreStageFactV1, error) {
	return s.Base.InspectWorkspaceRestoreStageFactV1(ctx, ref)
}

var _ ports.WorkspaceRestoreStoreV1 = (*WorkspaceRestoreMemoryStoreV1)(nil)
var _ ports.WorkspaceRestoreStoreV1 = (*WorkspaceRestoreLostReplyStoreV1)(nil)
