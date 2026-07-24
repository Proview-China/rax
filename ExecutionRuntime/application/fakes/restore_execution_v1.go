package fakes

import (
	"context"
	"sync"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type RestoreExecutionResultStoreV1 struct {
	mu                             sync.Mutex
	intents                        map[restoreExecutionResultKeyV1]applicationcontract.RestoreExecutionIntentFactV1
	values                         map[restoreExecutionResultKeyV1]applicationcontract.RestoreExecutionResultFactV1
	loseIntentReply, loseNextReply bool
}

type restoreExecutionResultKeyV1 struct {
	tenantID core.TenantID
	id       string
}

func NewRestoreExecutionResultStoreV1() *RestoreExecutionResultStoreV1 {
	return &RestoreExecutionResultStoreV1{intents: make(map[restoreExecutionResultKeyV1]applicationcontract.RestoreExecutionIntentFactV1), values: make(map[restoreExecutionResultKeyV1]applicationcontract.RestoreExecutionResultFactV1)}
}

func (s *RestoreExecutionResultStoreV1) LoseNextIntentReplyForTestV1() {
	s.mu.Lock()
	s.loseIntentReply = true
	s.mu.Unlock()
}

func (s *RestoreExecutionResultStoreV1) CreateRestoreExecutionIntentV1(ctx context.Context, candidate applicationcontract.RestoreExecutionIntentFactV1) (applicationcontract.RestoreExecutionIntentFactV1, error) {
	if ctx == nil || ctx.Err() != nil {
		return applicationcontract.RestoreExecutionIntentFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Restore execution Intent context is nil or canceled")
	}
	if err := candidate.ValidateCurrent(time.Unix(0, candidate.CreatedUnixNano)); err != nil {
		return applicationcontract.RestoreExecutionIntentFactV1{}, err
	}
	key := restoreExecutionResultKeyV1{candidate.TenantID, candidate.ID}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.intents[key]; ok {
		if existing.Digest != candidate.Digest || existing.RequestDigest != candidate.RequestDigest {
			return applicationcontract.RestoreExecutionIntentFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Restore execution Intent ID binds another request")
		}
		return existing.Clone(), nil
	}
	s.intents[key] = candidate.Clone()
	if s.loseIntentReply {
		s.loseIntentReply = false
		return applicationcontract.RestoreExecutionIntentFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Restore execution Intent reply loss")
	}
	return candidate.Clone(), nil
}

func (s *RestoreExecutionResultStoreV1) InspectRestoreExecutionIntentV1(ctx context.Context, tenantID core.TenantID, id string) (applicationcontract.RestoreExecutionIntentFactV1, error) {
	if ctx == nil || ctx.Err() != nil {
		return applicationcontract.RestoreExecutionIntentFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Restore execution Intent context is nil or canceled")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.intents[restoreExecutionResultKeyV1{tenantID, id}]
	if !ok {
		return applicationcontract.RestoreExecutionIntentFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Restore execution Intent not found")
	}
	return value.Clone(), nil
}

func (s *RestoreExecutionResultStoreV1) LoseNextReplyForTestV1() {
	s.mu.Lock()
	s.loseNextReply = true
	s.mu.Unlock()
}

func (s *RestoreExecutionResultStoreV1) CreateRestoreExecutionResultV1(ctx context.Context, candidate applicationcontract.RestoreExecutionResultFactV1) (applicationcontract.RestoreExecutionResultFactV1, error) {
	if ctx == nil {
		return applicationcontract.RestoreExecutionResultFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Restore execution result context is nil")
	}
	if err := ctx.Err(); err != nil {
		return applicationcontract.RestoreExecutionResultFactV1{}, err
	}
	if err := candidate.ValidateCurrent(time.Unix(0, candidate.CreatedUnixNano)); err != nil {
		return applicationcontract.RestoreExecutionResultFactV1{}, err
	}
	key := restoreExecutionResultKeyV1{candidate.TenantID, candidate.ID}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.values[key]; ok {
		if existing.Digest != candidate.Digest || existing.Request.Digest != candidate.Request.Digest || existing.Result.Digest != candidate.Result.Digest {
			return applicationcontract.RestoreExecutionResultFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Restore execution result ID binds another fact")
		}
		return existing.Clone(), nil
	}
	s.values[key] = candidate.Clone()
	if s.loseNextReply {
		s.loseNextReply = false
		return applicationcontract.RestoreExecutionResultFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Restore execution result reply loss")
	}
	return candidate.Clone(), nil
}

func (s *RestoreExecutionResultStoreV1) InspectRestoreExecutionResultV1(ctx context.Context, tenantID core.TenantID, id string) (applicationcontract.RestoreExecutionResultFactV1, error) {
	if ctx == nil {
		return applicationcontract.RestoreExecutionResultFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Restore execution result context is nil")
	}
	if err := ctx.Err(); err != nil {
		return applicationcontract.RestoreExecutionResultFactV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[restoreExecutionResultKeyV1{tenantID, id}]
	if !ok {
		return applicationcontract.RestoreExecutionResultFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Restore execution result not found")
	}
	return value.Clone(), nil
}

var _ applicationports.RestoreExecutionResultFactPortV1 = (*RestoreExecutionResultStoreV1)(nil)
var _ applicationports.RestoreExecutionIntentFactPortV1 = (*RestoreExecutionResultStoreV1)(nil)
