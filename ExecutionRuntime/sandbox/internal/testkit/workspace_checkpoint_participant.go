package testkit

import (
	"context"
	"errors"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type workspaceCheckpointKeyV2 struct {
	TenantID            string
	ScopeDigest         string
	CheckpointAttemptID string
	ParticipantID       string
}

type workspaceCheckpointFactKeyV2 struct {
	TenantID    string
	ScopeDigest string
	ID          string
}

type WorkspaceCheckpointParticipantMemoryStoreV2 struct {
	mu           sync.RWMutex
	prepared     map[workspaceCheckpointKeyV2]contract.WorkspaceCheckpointPreparedBundleV2
	participants map[workspaceCheckpointFactKeyV2]contract.WorkspaceCheckpointParticipantFactV2
	coverages    map[workspaceCheckpointFactKeyV2]contract.WorkspaceCheckpointCoverageFactV2
}

func NewWorkspaceCheckpointParticipantMemoryStoreV2() *WorkspaceCheckpointParticipantMemoryStoreV2 {
	return &WorkspaceCheckpointParticipantMemoryStoreV2{prepared: make(map[workspaceCheckpointKeyV2]contract.WorkspaceCheckpointPreparedBundleV2), participants: make(map[workspaceCheckpointFactKeyV2]contract.WorkspaceCheckpointParticipantFactV2), coverages: make(map[workspaceCheckpointFactKeyV2]contract.WorkspaceCheckpointCoverageFactV2)}
}

func (s *WorkspaceCheckpointParticipantMemoryStoreV2) CommitWorkspaceCheckpointPreparedV2(_ context.Context, bundle contract.WorkspaceCheckpointPreparedBundleV2) (bool, error) {
	if err := bundle.ValidateShape(); err != nil {
		return false, err
	}
	key := workspaceCheckpointKeyV2{TenantID: bundle.Participant.TenantID, ScopeDigest: bundle.Participant.ScopeDigest, CheckpointAttemptID: bundle.Participant.CheckpointAttemptRef.ID, ParticipantID: bundle.Participant.ParticipantID}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.prepared[key]; ok {
		if !contract.SameSnapshotArtifactExactRef(existing.Participant.ExactRef(), bundle.Participant.ExactRef()) || !contract.SameSnapshotArtifactExactRef(existing.Coverage.ExactRef(), bundle.Coverage.ExactRef()) {
			return false, ports.ErrConflict
		}
		return false, nil
	}
	participantKey := workspaceCheckpointFactKeyV2{TenantID: bundle.Participant.TenantID, ScopeDigest: bundle.Participant.ScopeDigest, ID: bundle.Participant.ExactRef().ID}
	coverageKey := workspaceCheckpointFactKeyV2{TenantID: bundle.Coverage.TenantID, ScopeDigest: bundle.Coverage.ScopeDigest, ID: bundle.Coverage.ExactRef().ID}
	if _, exists := s.participants[participantKey]; exists {
		return false, ports.ErrConflict
	}
	if _, exists := s.coverages[coverageKey]; exists {
		return false, ports.ErrConflict
	}
	copy := bundle.Clone()
	s.prepared[key] = copy
	s.participants[participantKey] = copy.Participant
	s.coverages[coverageKey] = copy.Coverage
	return true, nil
}

func (s *WorkspaceCheckpointParticipantMemoryStoreV2) InspectWorkspaceCheckpointPreparedV2(_ context.Context, request contract.InspectWorkspaceCheckpointPreparedRequestV2) (contract.WorkspaceCheckpointPreparedBundleV2, error) {
	if err := request.Validate(); err != nil {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, err
	}
	key := workspaceCheckpointKeyV2{TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, CheckpointAttemptID: request.CheckpointAttemptID, ParticipantID: request.ParticipantID}
	s.mu.RLock()
	defer s.mu.RUnlock()
	bundle, ok := s.prepared[key]
	if !ok {
		return contract.WorkspaceCheckpointPreparedBundleV2{}, ports.ErrNotFound
	}
	return bundle.Clone(), nil
}

func (s *WorkspaceCheckpointParticipantMemoryStoreV2) InspectWorkspaceCheckpointParticipantV2(_ context.Context, request contract.InspectWorkspaceCheckpointFactRequestV2) (contract.WorkspaceCheckpointParticipantFactV2, error) {
	if err := request.Validate(contract.WorkspaceCheckpointParticipantTypeURL, contract.WorkspaceCheckpointParticipantDigestDomain, "workspace checkpoint participant"); err != nil {
		return contract.WorkspaceCheckpointParticipantFactV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.participants[workspaceCheckpointFactKeyV2{TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, ID: request.ExpectedRef.ID}]
	if !ok {
		return contract.WorkspaceCheckpointParticipantFactV2{}, ports.ErrNotFound
	}
	if !contract.SameSnapshotArtifactExactRef(value.ExactRef(), request.ExpectedRef) || value.TenantID != request.TenantID || value.ScopeDigest != request.ScopeDigest {
		return contract.WorkspaceCheckpointParticipantFactV2{}, ports.ErrConflict
	}
	return value, nil
}

func (s *WorkspaceCheckpointParticipantMemoryStoreV2) InspectWorkspaceCheckpointCoverageV2(_ context.Context, request contract.InspectWorkspaceCheckpointFactRequestV2) (contract.WorkspaceCheckpointCoverageFactV2, error) {
	if err := request.Validate(contract.WorkspaceCheckpointCoverageTypeURL, contract.WorkspaceCheckpointCoverageDigestDomain, "workspace checkpoint coverage"); err != nil {
		return contract.WorkspaceCheckpointCoverageFactV2{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.coverages[workspaceCheckpointFactKeyV2{TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, ID: request.ExpectedRef.ID}]
	if !ok {
		return contract.WorkspaceCheckpointCoverageFactV2{}, ports.ErrNotFound
	}
	if !contract.SameSnapshotArtifactExactRef(value.ExactRef(), request.ExpectedRef) || value.TenantID != request.TenantID || value.ScopeDigest != request.ScopeDigest {
		return contract.WorkspaceCheckpointCoverageFactV2{}, ports.ErrConflict
	}
	return value.Clone(), nil
}

type WorkspaceCheckpointLostReplyStoreV2 struct {
	Base *WorkspaceCheckpointParticipantMemoryStoreV2
	mu   sync.Mutex
	lost bool
}

func (s *WorkspaceCheckpointLostReplyStoreV2) LoseNextSuccessfulReply() {
	s.mu.Lock()
	s.lost = true
	s.mu.Unlock()
}

func (s *WorkspaceCheckpointLostReplyStoreV2) CommitWorkspaceCheckpointPreparedV2(ctx context.Context, bundle contract.WorkspaceCheckpointPreparedBundleV2) (bool, error) {
	created, err := s.Base.CommitWorkspaceCheckpointPreparedV2(ctx, bundle)
	s.mu.Lock()
	lost := s.lost && err == nil && created
	if lost {
		s.lost = false
	}
	s.mu.Unlock()
	if lost {
		return false, errors.New("injected workspace checkpoint reply loss")
	}
	return created, err
}

func (s *WorkspaceCheckpointLostReplyStoreV2) InspectWorkspaceCheckpointPreparedV2(ctx context.Context, request contract.InspectWorkspaceCheckpointPreparedRequestV2) (contract.WorkspaceCheckpointPreparedBundleV2, error) {
	return s.Base.InspectWorkspaceCheckpointPreparedV2(ctx, request)
}
func (s *WorkspaceCheckpointLostReplyStoreV2) InspectWorkspaceCheckpointParticipantV2(ctx context.Context, request contract.InspectWorkspaceCheckpointFactRequestV2) (contract.WorkspaceCheckpointParticipantFactV2, error) {
	return s.Base.InspectWorkspaceCheckpointParticipantV2(ctx, request)
}
func (s *WorkspaceCheckpointLostReplyStoreV2) InspectWorkspaceCheckpointCoverageV2(ctx context.Context, request contract.InspectWorkspaceCheckpointFactRequestV2) (contract.WorkspaceCheckpointCoverageFactV2, error) {
	return s.Base.InspectWorkspaceCheckpointCoverageV2(ctx, request)
}

var _ ports.WorkspaceCheckpointParticipantStoreV2 = (*WorkspaceCheckpointParticipantMemoryStoreV2)(nil)
var _ ports.WorkspaceCheckpointParticipantStoreV2 = (*WorkspaceCheckpointLostReplyStoreV2)(nil)
