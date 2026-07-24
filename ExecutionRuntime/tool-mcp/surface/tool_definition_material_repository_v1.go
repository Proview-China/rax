package surface

import (
	"context"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type InMemoryToolDefinitionMaterialRepositoryV1 struct {
	mu     sync.RWMutex
	values map[string]toolcontract.ToolDefinitionMaterialV1
}

func NewInMemoryToolDefinitionMaterialRepositoryV1() *InMemoryToolDefinitionMaterialRepositoryV1 {
	return &InMemoryToolDefinitionMaterialRepositoryV1{values: make(map[string]toolcontract.ToolDefinitionMaterialV1)}
}

func (r *InMemoryToolDefinitionMaterialRepositoryV1) EnsureExactToolDefinitionMaterialV1(ctx context.Context, material toolcontract.ToolDefinitionMaterialV1) (toolcontract.ToolDefinitionMaterialV1, error) {
	if r == nil || r.values == nil {
		return toolcontract.ToolDefinitionMaterialV1{}, toolDefinitionMaterialUnavailableV1("Tool Definition Material Repository is unavailable")
	}
	if err := toolDefinitionMaterialContextErrorV1(ctx); err != nil {
		return toolcontract.ToolDefinitionMaterialV1{}, err
	}
	material = material.Clone()
	if err := material.Validate(); err != nil {
		return toolcontract.ToolDefinitionMaterialV1{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := toolDefinitionMaterialContextErrorV1(ctx); err != nil {
		return toolcontract.ToolDefinitionMaterialV1{}, err
	}
	if winner, ok := r.values[material.Ref.ID]; ok {
		if !reflect.DeepEqual(winner, material) {
			return toolcontract.ToolDefinitionMaterialV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Tool Definition Material ID already binds different content")
		}
		return winner.Clone(), nil
	}
	r.values[material.Ref.ID] = material.Clone()
	return material.Clone(), nil
}

func (r *InMemoryToolDefinitionMaterialRepositoryV1) InspectExactToolDefinitionMaterialV1(ctx context.Context, exact toolcontract.ToolDefinitionMaterialRefV1) (toolcontract.ToolDefinitionMaterialV1, error) {
	if r == nil || r.values == nil {
		return toolcontract.ToolDefinitionMaterialV1{}, toolDefinitionMaterialUnavailableV1("Tool Definition Material Repository is unavailable")
	}
	if err := toolDefinitionMaterialContextErrorV1(ctx); err != nil {
		return toolcontract.ToolDefinitionMaterialV1{}, err
	}
	if err := exact.Validate(); err != nil {
		return toolcontract.ToolDefinitionMaterialV1{}, err
	}
	r.mu.RLock()
	winner, ok := r.values[exact.ID]
	r.mu.RUnlock()
	if !ok {
		return toolcontract.ToolDefinitionMaterialV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Tool Definition Material exact Ref is absent")
	}
	if winner.Ref != exact {
		return toolcontract.ToolDefinitionMaterialV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Tool Definition Material exact Ref drifted")
	}
	if err := winner.Validate(); err != nil {
		return toolcontract.ToolDefinitionMaterialV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "stored Tool Definition Material is non-canonical")
	}
	if err := toolDefinitionMaterialContextErrorV1(ctx); err != nil {
		return toolcontract.ToolDefinitionMaterialV1{}, err
	}
	return winner.Clone(), nil
}

func toolDefinitionMaterialContextErrorV1(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Tool Definition Material context is required")
	}
	return ctx.Err()
}

func toolDefinitionMaterialUnavailableV1(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, message)
}

var _ toolcontract.ToolDefinitionMaterialRepositoryV1 = (*InMemoryToolDefinitionMaterialRepositoryV1)(nil)
