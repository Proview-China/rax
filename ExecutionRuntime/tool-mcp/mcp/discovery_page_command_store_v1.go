package mcp

import (
	"context"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPDiscoveryPageCommandRepositoryV1 interface {
	toolcontract.MCPDiscoveryPageCommandExactReaderV1
	EnsureMCPDiscoveryPageCommandV1(context.Context, toolcontract.MCPDiscoveryPageCommandV1) (toolcontract.MCPDiscoveryPageCommandV1, error)
}

type InMemoryMCPDiscoveryPageCommandRepositoryV1 struct {
	mu     sync.RWMutex
	values map[string]toolcontract.MCPDiscoveryPageCommandV1
}

func NewInMemoryMCPDiscoveryPageCommandRepositoryV1() *InMemoryMCPDiscoveryPageCommandRepositoryV1 {
	return &InMemoryMCPDiscoveryPageCommandRepositoryV1{values: make(map[string]toolcontract.MCPDiscoveryPageCommandV1)}
}

func (r *InMemoryMCPDiscoveryPageCommandRepositoryV1) EnsureMCPDiscoveryPageCommandV1(ctx context.Context, command toolcontract.MCPDiscoveryPageCommandV1) (toolcontract.MCPDiscoveryPageCommandV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPDiscoveryPageCommandV1{}, err
	}
	if r == nil || command.Validate() != nil {
		return toolcontract.MCPDiscoveryPageCommandV1{}, invalid("MCP Discovery Page Command Ensure is invalid")
	}
	command = cloneMCPDiscoveryPageCommandV1(command)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPDiscoveryPageCommandV1{}, err
	}
	if winner, ok := r.values[command.Ref.ID]; ok {
		if !reflect.DeepEqual(winner, command) {
			return toolcontract.MCPDiscoveryPageCommandV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Discovery Page Command ID binds another canonical command")
		}
		return cloneMCPDiscoveryPageCommandV1(winner), nil
	}
	r.values[command.Ref.ID] = command
	return cloneMCPDiscoveryPageCommandV1(command), nil
}

func (r *InMemoryMCPDiscoveryPageCommandRepositoryV1) InspectMCPDiscoveryPageCommandV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageCommandV1, error) {
	if err := requireMCPExecutionContextV1(ctx); err != nil {
		return toolcontract.MCPDiscoveryPageCommandV1{}, err
	}
	if r == nil || exact.Validate() != nil {
		return toolcontract.MCPDiscoveryPageCommandV1{}, invalid("MCP Discovery Page Command exact Inspect is invalid")
	}
	r.mu.RLock()
	value, ok := r.values[exact.ID]
	r.mu.RUnlock()
	if !ok {
		return toolcontract.MCPDiscoveryPageCommandV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Discovery Page Command not found")
	}
	if value.Ref != exact {
		return toolcontract.MCPDiscoveryPageCommandV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page Command exact Ref drifted")
	}
	return cloneMCPDiscoveryPageCommandV1(value), nil
}

func cloneMCPDiscoveryPageCommandV1(value toolcontract.MCPDiscoveryPageCommandV1) toolcontract.MCPDiscoveryPageCommandV1 {
	value.Cursor = append([]byte(nil), value.Cursor...)
	return value
}

var _ MCPDiscoveryPageCommandRepositoryV1 = (*InMemoryMCPDiscoveryPageCommandRepositoryV1)(nil)
