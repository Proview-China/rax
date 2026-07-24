package fakes

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// InMemoryPreparedDomainCommandAssociationStoreV1 is a reference-test store;
// it is not a production State Plane backend.
type InMemoryPreparedDomainCommandAssociationStoreV1 struct {
	mu    sync.RWMutex
	facts map[string]ports.PreparedDomainCommandAssociationCurrentProjectionV1
}

func NewInMemoryPreparedDomainCommandAssociationStoreV1() *InMemoryPreparedDomainCommandAssociationStoreV1 {
	return &InMemoryPreparedDomainCommandAssociationStoreV1{facts: make(map[string]ports.PreparedDomainCommandAssociationCurrentProjectionV1)}
}

func (s *InMemoryPreparedDomainCommandAssociationStoreV1) CreatePreparedDomainCommandAssociationV1(ctx context.Context, projection ports.PreparedDomainCommandAssociationCurrentProjectionV1) (ports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	if err := associationStoreContextV1(ctx); err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	if s == nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "prepared domain command association store is unavailable")
	}
	if err := projection.Validate(); err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.facts[projection.Ref.ID]; ok {
		if existing.Ref != projection.Ref {
			return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "prepared domain command association already exists")
		}
		return existing, nil
	}
	s.facts[projection.Ref.ID] = projection
	return projection, nil
}

func (s *InMemoryPreparedDomainCommandAssociationStoreV1) InspectPreparedDomainCommandAssociationV1(ctx context.Context, exact ports.PreparedDomainCommandAssociationRefV1) (ports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	if err := associationStoreContextV1(ctx); err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	if s == nil || exact.ID == "" || exact.Revision != 1 {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "prepared domain command association lookup is invalid")
	}
	projection, err := s.InspectPreparedDomainCommandAssociationByIDV1(ctx, exact.ID)
	if err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	if exact != projection.Ref {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "prepared domain command association exact Ref drifted")
	}
	return projection, nil
}

func (s *InMemoryPreparedDomainCommandAssociationStoreV1) InspectPreparedDomainCommandAssociationByIDV1(ctx context.Context, id string) (ports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	if err := associationStoreContextV1(ctx); err != nil {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, err
	}
	if s == nil || id == "" {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "prepared domain command association ID lookup is invalid")
	}
	s.mu.RLock()
	projection, ok := s.facts[id]
	s.mu.RUnlock()
	if !ok {
		return ports.PreparedDomainCommandAssociationCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "prepared domain command association not found")
	}
	return projection, nil
}

func associationStoreContextV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "prepared domain command association store context is nil")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

var _ ports.PreparedDomainCommandAssociationStoreV1 = (*InMemoryPreparedDomainCommandAssociationStoreV1)(nil)
