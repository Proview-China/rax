package fakes

import (
	"context"
	"sync"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type RestoreStageActionResultStoreV1 struct {
	mu            sync.Mutex
	values        map[restoreExecutionResultKeyV1]applicationcontract.RestoreStageActionResultFactV1
	loseNextReply bool
}

func NewRestoreStageActionResultStoreV1() *RestoreStageActionResultStoreV1 {
	return &RestoreStageActionResultStoreV1{values: make(map[restoreExecutionResultKeyV1]applicationcontract.RestoreStageActionResultFactV1)}
}

func (s *RestoreStageActionResultStoreV1) LoseNextReplyForTestV1() {
	s.mu.Lock()
	s.loseNextReply = true
	s.mu.Unlock()
}

func (s *RestoreStageActionResultStoreV1) CreateRestoreStageActionResultV1(ctx context.Context, candidate applicationcontract.RestoreStageActionResultFactV1) (applicationcontract.RestoreStageActionResultFactV1, error) {
	if ctx == nil {
		return applicationcontract.RestoreStageActionResultFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Restore Stage result context is nil")
	}
	if err := ctx.Err(); err != nil {
		return applicationcontract.RestoreStageActionResultFactV1{}, err
	}
	if err := candidate.ValidateFor(candidate.Request, time.Unix(0, candidate.CreatedUnixNano)); err != nil {
		return applicationcontract.RestoreStageActionResultFactV1{}, err
	}
	key := restoreExecutionResultKeyV1{candidate.TenantID, candidate.ID}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.values[key]; ok {
		if existing.Digest != candidate.Digest || existing.RequestDigest != candidate.RequestDigest || existing.Result.Digest != candidate.Result.Digest {
			return applicationcontract.RestoreStageActionResultFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Restore Stage result ID binds another fact")
		}
		return existing.Clone(), nil
	}
	s.values[key] = candidate.Clone()
	if s.loseNextReply {
		s.loseNextReply = false
		return applicationcontract.RestoreStageActionResultFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Restore Stage result reply loss")
	}
	return candidate.Clone(), nil
}

func (s *RestoreStageActionResultStoreV1) InspectRestoreStageActionResultV1(ctx context.Context, tenantID core.TenantID, id string) (applicationcontract.RestoreStageActionResultFactV1, error) {
	if ctx == nil {
		return applicationcontract.RestoreStageActionResultFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Restore Stage result context is nil")
	}
	if err := ctx.Err(); err != nil {
		return applicationcontract.RestoreStageActionResultFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[restoreExecutionResultKeyV1{tenantID, id}]
	if !ok {
		return applicationcontract.RestoreStageActionResultFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Restore Stage result not found")
	}
	return value.Clone(), nil
}

var _ applicationports.RestoreStageActionResultFactPortV1 = (*RestoreStageActionResultStoreV1)(nil)
