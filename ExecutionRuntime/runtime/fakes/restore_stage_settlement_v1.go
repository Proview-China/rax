package fakes

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RestoreStageSettlementStoreV1 is deterministic reference infrastructure.
// It makes no production durability, backend, topology, or SLA claim.
type RestoreStageSettlementStoreV1 struct {
	mu            sync.Mutex
	byID          map[string]ports.RestoreStageSettlementFactV1
	byEffect      map[restoreStageSettlementEffectKeyV1]string
	loseCreate    bool
	createCommits uint64
}

type restoreStageSettlementEffectKeyV1 struct {
	OperationDigest core.Digest
	EffectID        core.EffectIntentID
}

func NewRestoreStageSettlementStoreV1() *RestoreStageSettlementStoreV1 {
	return &RestoreStageSettlementStoreV1{
		byID:     make(map[string]ports.RestoreStageSettlementFactV1),
		byEffect: make(map[restoreStageSettlementEffectKeyV1]string),
	}
}

func (s *RestoreStageSettlementStoreV1) LoseNextCreateReplyV1() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseCreate = true
}

func (s *RestoreStageSettlementStoreV1) CreateCommitCountV1() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createCommits
}

func (s *RestoreStageSettlementStoreV1) CreateRestoreStageSettlementV1(_ context.Context, fact ports.RestoreStageSettlementFactV1) (ports.RestoreStageSettlementFactV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fact.Validate(); err != nil {
		return ports.RestoreStageSettlementFactV1{}, err
	}
	if current, ok := s.byID[fact.Submission.ID]; ok {
		if current == fact {
			return current, nil
		}
		return ports.RestoreStageSettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Restore Stage Settlement ID already contains different content")
	}
	key := restoreStageSettlementEffectKeyV1{OperationDigest: fact.Submission.OperationDigest, EffectID: fact.Submission.EffectID}
	if currentID, ok := s.byEffect[key]; ok {
		current := s.byID[currentID]
		if current == fact {
			return current, nil
		}
		return ports.RestoreStageSettlementFactV1{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Restore Stage effect already has a different Settlement")
	}
	s.byID[fact.Submission.ID] = fact
	s.byEffect[key] = fact.Submission.ID
	s.createCommits++
	if s.loseCreate {
		s.loseCreate = false
		return ports.RestoreStageSettlementFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected lost Restore Stage Settlement create reply")
	}
	return fact, nil
}

func (s *RestoreStageSettlementStoreV1) InspectRestoreStageSettlementV1(_ context.Context, id string) (ports.RestoreStageSettlementFactV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fact, ok := s.byID[id]
	if !ok {
		return ports.RestoreStageSettlementFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "Restore Stage Settlement is absent")
	}
	return fact, nil
}

func (s *RestoreStageSettlementStoreV1) InspectRestoreStageSettlementByEffectV1(_ context.Context, operation ports.OperationSubjectV3, effectID core.EffectIntentID) (ports.RestoreStageSettlementFactV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	digest, err := operation.DigestV3()
	if err != nil {
		return ports.RestoreStageSettlementFactV1{}, err
	}
	id, ok := s.byEffect[restoreStageSettlementEffectKeyV1{OperationDigest: digest, EffectID: effectID}]
	if !ok {
		return ports.RestoreStageSettlementFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "Restore Stage Settlement current index is absent")
	}
	return s.byID[id], nil
}

var _ ports.RestoreStageSettlementFactPortV1 = (*RestoreStageSettlementStoreV1)(nil)
