package applicationadapter

import (
	"context"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// InMemoryToolInputContractLeaseStoreV1 is a production-neutral Tool Owner
// create-once store. It is intentionally not a production backend.
type InMemoryToolInputContractLeaseStoreV1 struct {
	mu      sync.RWMutex
	records map[string]toolcontract.ToolInputContractCurrentProjectionV1
}

func NewInMemoryToolInputContractLeaseStoreV1() *InMemoryToolInputContractLeaseStoreV1 {
	return &InMemoryToolInputContractLeaseStoreV1{records: make(map[string]toolcontract.ToolInputContractCurrentProjectionV1)}
}

func (s *InMemoryToolInputContractLeaseStoreV1) CreateToolInputContractCurrentOnceV1(ctx context.Context, projection toolcontract.ToolInputContractCurrentProjectionV1) (toolcontract.ToolInputContractCurrentProjectionV1, error) {
	if ctx == nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, bindingInvalidV1("Tool Input Contract store context is required")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	if s == nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, bindingUnavailableV1("Tool Input Contract store is unavailable")
	}
	if err := projection.Validate(); err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	projection = toolcontract.CloneToolInputContractCurrentProjectionV1(projection)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	if s.records == nil {
		s.records = make(map[string]toolcontract.ToolInputContractCurrentProjectionV1)
	}
	if existing, ok := s.records[projection.Ref.ID]; ok {
		if err := existing.Validate(); err != nil || existing.Ref.ID != projection.Ref.ID {
			return toolcontract.ToolInputContractCurrentProjectionV1{}, bindingConflictV1("stored Tool Input Contract is non-canonical")
		}
		// Checked/Expires and the projection digest are winner-owned fresh
		// values. A same-issuance concurrent loser returns that winner after
		// comparing only the stable issuance and binding subjects.
		if !reflect.DeepEqual(existing.IssuanceSubject, projection.IssuanceSubject) {
			return toolcontract.ToolInputContractCurrentProjectionV1{}, bindingConflictV1("Tool Input Contract ID already names another issuance")
		}
		return toolcontract.CloneToolInputContractCurrentProjectionV1(existing), nil
	}
	s.records[projection.Ref.ID] = projection
	return toolcontract.CloneToolInputContractCurrentProjectionV1(projection), nil
}

func (s *InMemoryToolInputContractLeaseStoreV1) InspectToolInputContractCurrentByIssuanceIDV1(ctx context.Context, id string) (toolcontract.ToolInputContractCurrentProjectionV1, error) {
	if ctx == nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, bindingInvalidV1("Tool Input Contract store context is required")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	if s == nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, bindingUnavailableV1("Tool Input Contract store is unavailable")
	}
	if toolcontract.ValidateStableID(id) != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, bindingInvalidV1("Tool Input Contract issuance ID is invalid")
	}
	s.mu.RLock()
	projection, ok := s.records[id]
	s.mu.RUnlock()
	if !ok {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, bindingNotFoundV1("Tool Input Contract is absent")
	}
	if err := projection.Validate(); err != nil || projection.Ref.ID != id {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, bindingConflictV1("stored Tool Input Contract is non-canonical")
	}
	return toolcontract.CloneToolInputContractCurrentProjectionV1(projection), nil
}

func (s *InMemoryToolInputContractLeaseStoreV1) InspectExactToolInputContractCurrentV1(ctx context.Context, exact toolcontract.ToolInputContractCurrentRefV1) (toolcontract.ToolInputContractCurrentProjectionV1, error) {
	if err := exact.Validate(); err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	projection, err := s.InspectToolInputContractCurrentByIssuanceIDV1(ctx, exact.ID)
	if err != nil {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, err
	}
	if projection.Ref != exact {
		return toolcontract.ToolInputContractCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Tool Input Contract exact Ref drifted")
	}
	return projection, nil
}

var _ toolcontract.ToolInputContractLeaseStoreV1 = (*InMemoryToolInputContractLeaseStoreV1)(nil)
