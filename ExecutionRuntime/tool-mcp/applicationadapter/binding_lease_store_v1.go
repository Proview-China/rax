package applicationadapter

import (
	"context"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// SingleCallToolActionBindingLeaseStoreV1 is Tool Owner-internal storage.  It
// exposes neither Application nor Runtime mutation authority.
type SingleCallToolActionBindingLeaseStoreV1 interface {
	CreateSingleCallToolActionBindingLeaseOnceV1(context.Context, SingleCallToolActionBindingCurrentProjectionV1) (SingleCallToolActionBindingCurrentProjectionV1, error)
	InspectSingleCallToolActionBindingLeaseByIssuanceIDV1(context.Context, string) (SingleCallToolActionBindingCurrentProjectionV1, error)
	InspectExactSingleCallToolActionBindingLeaseV1(context.Context, SingleCallToolActionBindingCurrentRefV1) (SingleCallToolActionBindingCurrentProjectionV1, error)
}

type InMemorySingleCallToolActionBindingLeaseStoreV1 struct {
	mu      sync.RWMutex
	records map[string]SingleCallToolActionBindingCurrentProjectionV1
}

func NewInMemorySingleCallToolActionBindingLeaseStoreV1() *InMemorySingleCallToolActionBindingLeaseStoreV1 {
	return &InMemorySingleCallToolActionBindingLeaseStoreV1{records: make(map[string]SingleCallToolActionBindingCurrentProjectionV1)}
}

func (s *InMemorySingleCallToolActionBindingLeaseStoreV1) CreateSingleCallToolActionBindingLeaseOnceV1(ctx context.Context, projection SingleCallToolActionBindingCurrentProjectionV1) (SingleCallToolActionBindingCurrentProjectionV1, error) {
	if ctx == nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, bindingInvalidV1("context is required")
	}
	if s == nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, bindingUnavailableV1("binding lease store is unavailable")
	}
	if err := projection.Validate(); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	projection = cloneBindingProjectionV1(projection)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		s.records = make(map[string]SingleCallToolActionBindingCurrentProjectionV1)
	}
	if existing, ok := s.records[projection.Ref.ID]; ok {
		if err := existing.Validate(); err != nil {
			return SingleCallToolActionBindingCurrentProjectionV1{}, bindingConflictV1("stored binding lease is non-canonical")
		}
		// The issuance subject, not the contender's fresh clock sample, is the
		// create-once identity.  A concurrent loser must return the winner's
		// immutable Ref/Checked/Expires instead of comparing its independently
		// computed projection digest or lease window.
		if !reflect.DeepEqual(existing.IssuanceSubject, projection.IssuanceSubject) || !reflect.DeepEqual(existing.Subject, projection.Subject) {
			return SingleCallToolActionBindingCurrentProjectionV1{}, bindingConflictV1("binding lease ID already names another issuance")
		}
		return cloneBindingProjectionV1(existing), nil
	}
	s.records[projection.Ref.ID] = projection
	return cloneBindingProjectionV1(projection), nil
}

func (s *InMemorySingleCallToolActionBindingLeaseStoreV1) InspectSingleCallToolActionBindingLeaseByIssuanceIDV1(ctx context.Context, id string) (SingleCallToolActionBindingCurrentProjectionV1, error) {
	if ctx == nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, bindingInvalidV1("context is required")
	}
	if s == nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, bindingUnavailableV1("binding lease store is unavailable")
	}
	if !validBindingIDV1(id) {
		return SingleCallToolActionBindingCurrentProjectionV1{}, bindingInvalidV1("binding issuance ID is invalid")
	}
	s.mu.RLock()
	projection, ok := s.records[id]
	s.mu.RUnlock()
	if !ok {
		return SingleCallToolActionBindingCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "binding lease is absent")
	}
	if err := projection.Validate(); err != nil || projection.Ref.ID != id {
		return SingleCallToolActionBindingCurrentProjectionV1{}, bindingConflictV1("stored binding lease is non-canonical")
	}
	return cloneBindingProjectionV1(projection), nil
}

func (s *InMemorySingleCallToolActionBindingLeaseStoreV1) InspectExactSingleCallToolActionBindingLeaseV1(ctx context.Context, exact SingleCallToolActionBindingCurrentRefV1) (SingleCallToolActionBindingCurrentProjectionV1, error) {
	if err := exact.Validate(); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	projection, err := s.InspectSingleCallToolActionBindingLeaseByIssuanceIDV1(ctx, exact.ID)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV1{}, err
	}
	if projection.Ref != exact {
		return SingleCallToolActionBindingCurrentProjectionV1{}, bindingConflictV1("binding lease exact ref drifted")
	}
	return projection, nil
}
