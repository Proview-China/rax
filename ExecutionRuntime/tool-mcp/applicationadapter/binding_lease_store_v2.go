package applicationadapter

import (
	"context"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type InMemorySingleCallToolActionBindingLeaseStoreV2 struct {
	mu      sync.RWMutex
	records map[string]SingleCallToolActionBindingCurrentProjectionV2
}

func NewInMemorySingleCallToolActionBindingLeaseStoreV2() *InMemorySingleCallToolActionBindingLeaseStoreV2 {
	return &InMemorySingleCallToolActionBindingLeaseStoreV2{records: make(map[string]SingleCallToolActionBindingCurrentProjectionV2)}
}

func (s *InMemorySingleCallToolActionBindingLeaseStoreV2) CreateSingleCallToolActionBindingCurrentOnceV2(ctx context.Context, projection SingleCallToolActionBindingCurrentProjectionV2) (SingleCallToolActionBindingCurrentProjectionV2, error) {
	if ctx == nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, bindingInvalidV1("BindingV2 store context is required")
	}
	if err := ctx.Err(); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	if s == nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, bindingUnavailableV1("BindingV2 store is unavailable")
	}
	if err := projection.Validate(); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	projection = CloneSingleCallToolActionBindingCurrentProjectionV2(projection)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	if s.records == nil {
		s.records = make(map[string]SingleCallToolActionBindingCurrentProjectionV2)
	}
	if winner, ok := s.records[projection.Ref.ID]; ok {
		if err := winner.Validate(); err != nil || winner.Ref.ID != projection.Ref.ID {
			return SingleCallToolActionBindingCurrentProjectionV2{}, bindingConflictV1("stored BindingV2 root is non-canonical")
		}
		if !reflect.DeepEqual(winner.IssuanceSubject, projection.IssuanceSubject) {
			return SingleCallToolActionBindingCurrentProjectionV2{}, bindingConflictV1("BindingV2 ID already names another stable issuance")
		}
		return CloneSingleCallToolActionBindingCurrentProjectionV2(winner), nil
	}
	s.records[projection.Ref.ID] = projection
	return CloneSingleCallToolActionBindingCurrentProjectionV2(projection), nil
}

func (s *InMemorySingleCallToolActionBindingLeaseStoreV2) InspectSingleCallToolActionBindingCurrentByIssuanceIDV2(ctx context.Context, id string) (SingleCallToolActionBindingCurrentProjectionV2, error) {
	if ctx == nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, bindingInvalidV1("BindingV2 store context is required")
	}
	if err := ctx.Err(); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	if s == nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, bindingUnavailableV1("BindingV2 store is unavailable")
	}
	if toolcontract.ValidateStableID(id) != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, bindingInvalidV1("BindingV2 issuance ID is invalid")
	}
	s.mu.RLock()
	winner, ok := s.records[id]
	s.mu.RUnlock()
	if !ok {
		return SingleCallToolActionBindingCurrentProjectionV2{}, bindingNotFoundV1("BindingV2 root is absent")
	}
	if err := winner.Validate(); err != nil || winner.Ref.ID != id {
		return SingleCallToolActionBindingCurrentProjectionV2{}, bindingConflictV1("stored BindingV2 root is non-canonical")
	}
	return CloneSingleCallToolActionBindingCurrentProjectionV2(winner), nil
}

func (s *InMemorySingleCallToolActionBindingLeaseStoreV2) InspectExactSingleCallToolActionBindingCurrentV2(ctx context.Context, exact toolcontract.SingleCallToolActionBindingCurrentRefV2) (SingleCallToolActionBindingCurrentProjectionV2, error) {
	if err := exact.Validate(); err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	winner, err := s.InspectSingleCallToolActionBindingCurrentByIssuanceIDV2(ctx, exact.ID)
	if err != nil {
		return SingleCallToolActionBindingCurrentProjectionV2{}, err
	}
	if winner.Ref != exact {
		return SingleCallToolActionBindingCurrentProjectionV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "BindingV2 exact Ref drifted")
	}
	return winner, nil
}

var _ SingleCallToolActionBindingLeaseStoreV2 = (*InMemorySingleCallToolActionBindingLeaseStoreV2)(nil)
