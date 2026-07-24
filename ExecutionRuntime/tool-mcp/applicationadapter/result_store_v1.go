package applicationadapter

import (
	"context"
	"sync"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// InMemoryApplicationResultStoreV1 is a deterministic local reference store.
// It is not a production backend, durability claim, composition root or SLA.
type InMemoryApplicationResultStoreV1 struct {
	mu     sync.RWMutex
	values map[string]applicationcontract.SingleCallToolActionResultV1
}

func NewInMemoryApplicationResultStoreV1() *InMemoryApplicationResultStoreV1 {
	return &InMemoryApplicationResultStoreV1{values: make(map[string]applicationcontract.SingleCallToolActionResultV1)}
}
func (s *InMemoryApplicationResultStoreV1) CreateSingleCallApplicationResultV1(_ context.Context, request applicationcontract.SingleCallToolActionRequestV1, result applicationcontract.SingleCallToolActionResultV1) (applicationcontract.SingleCallToolActionResultV1, error) {
	if request.Validate() != nil || result.RequestID != request.ID || result.RequestDigest != request.Digest {
		return applicationcontract.SingleCallToolActionResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Application result create input is invalid")
	}
	key := resultKey(request.ID, request.Digest, request.ExecutionScopeDigest)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.values[key]; ok {
		if existing.Digest == result.Digest {
			return existing, nil
		}
		return applicationcontract.SingleCallToolActionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Application result key binds different content")
	}
	s.values[key] = result
	return result, nil
}
func (s *InMemoryApplicationResultStoreV1) InspectSingleCallApplicationResultV1(_ context.Context, key applicationports.InspectSingleCallToolActionRequestV1) (applicationcontract.SingleCallToolActionResultV1, error) {
	if err := key.Validate(); err != nil {
		return applicationcontract.SingleCallToolActionResultV1{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.values[resultKey(key.RequestID, key.RequestDigest, key.ScopeDigest)]
	if !ok {
		return applicationcontract.SingleCallToolActionResultV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Application result not found")
	}
	return value, nil
}
func resultKey(id string, digest, scope core.Digest) string {
	return id + "\x00" + string(digest) + "\x00" + string(scope)
}

var _ ApplicationResultStoreV1 = (*InMemoryApplicationResultStoreV1)(nil)
