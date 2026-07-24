package mcp

import (
	"context"
	"reflect"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type EnsureMCPServerDescriptorRequestV1 struct {
	Descriptor      toolcontract.MCPServerDescriptor `json:"descriptor"`
	ExpectedCurrent *toolcontract.ObjectRef          `json:"expected_current,omitempty"`
}

func (r EnsureMCPServerDescriptorRequestV1) Validate() error {
	if r.Descriptor.Validate() != nil {
		return invalid("MCP Server Descriptor Ensure request is invalid")
	}
	if r.ExpectedCurrent != nil {
		if r.ExpectedCurrent.Validate() != nil || r.ExpectedCurrent.ID != r.Descriptor.ID || r.Descriptor.Revision != r.ExpectedCurrent.Revision+1 {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Server Descriptor successor request drifted")
		}
	} else if r.Descriptor.Revision != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Server Descriptor create must start at revision 1")
	}
	return nil
}

type MCPServerDescriptorReaderV1 interface {
	InspectMCPServerDescriptorV1(context.Context, toolcontract.ObjectRef) (toolcontract.MCPServerDescriptor, error)
	InspectCurrentMCPServerDescriptorV1(context.Context, string) (toolcontract.MCPServerDescriptor, error)
}

type MCPServerDescriptorRepositoryV1 interface {
	MCPServerDescriptorReaderV1
	EnsureMCPServerDescriptorV1(context.Context, EnsureMCPServerDescriptorRequestV1) (toolcontract.MCPServerDescriptor, error)
}

type InMemoryMCPServerDescriptorRepositoryV1 struct {
	mu      sync.RWMutex
	history map[string]toolcontract.MCPServerDescriptor
	current map[string]toolcontract.ObjectRef
}

func NewInMemoryMCPServerDescriptorRepositoryV1() *InMemoryMCPServerDescriptorRepositoryV1 {
	return &InMemoryMCPServerDescriptorRepositoryV1{history: make(map[string]toolcontract.MCPServerDescriptor), current: make(map[string]toolcontract.ObjectRef)}
}

func (r *InMemoryMCPServerDescriptorRepositoryV1) EnsureMCPServerDescriptorV1(ctx context.Context, request EnsureMCPServerDescriptorRequestV1) (toolcontract.MCPServerDescriptor, error) {
	if err := serverDescriptorContextV1(ctx); err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	if r == nil {
		return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Server Descriptor repository is unavailable")
	}
	if err := request.Validate(); err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	descriptor := cloneMCPServerDescriptorV1(request.Descriptor)
	exact := serverDescriptorRefV1(descriptor)
	key := serverDescriptorHistoryKeyV1(exact)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	current, exists := r.current[descriptor.ID]
	if winner, ok := r.history[key]; ok {
		if current != exact || !reflect.DeepEqual(winner, descriptor) {
			return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "MCP Server Descriptor history cannot replace current")
		}
		return cloneMCPServerDescriptorV1(winner), nil
	}
	if !exists {
		if request.ExpectedCurrent != nil || descriptor.Revision != 1 {
			return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Server Descriptor current is absent")
		}
	} else {
		if request.ExpectedCurrent == nil || current != *request.ExpectedCurrent || descriptor.Revision != current.Revision+1 {
			return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "MCP Server Descriptor current changed")
		}
	}
	r.history[key] = descriptor
	r.current[descriptor.ID] = exact
	return cloneMCPServerDescriptorV1(descriptor), nil
}

func (r *InMemoryMCPServerDescriptorRepositoryV1) InspectMCPServerDescriptorV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPServerDescriptor, error) {
	if err := serverDescriptorContextV1(ctx); err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	if r == nil || exact.Validate() != nil {
		return toolcontract.MCPServerDescriptor{}, invalid("MCP Server Descriptor exact Inspect is invalid")
	}
	r.mu.RLock()
	descriptor, ok := r.history[serverDescriptorHistoryKeyV1(exact)]
	r.mu.RUnlock()
	if !ok {
		return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Server Descriptor not found")
	}
	if serverDescriptorRefV1(descriptor) != exact {
		return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Server Descriptor exact Ref drifted")
	}
	return cloneMCPServerDescriptorV1(descriptor), nil
}

func (r *InMemoryMCPServerDescriptorRepositoryV1) InspectCurrentMCPServerDescriptorV1(ctx context.Context, id string) (toolcontract.MCPServerDescriptor, error) {
	if err := serverDescriptorContextV1(ctx); err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	if r == nil || toolcontract.ValidateStableID(id) != nil {
		return toolcontract.MCPServerDescriptor{}, invalid("MCP Server Descriptor current Inspect is invalid")
	}
	r.mu.RLock()
	exact, ok := r.current[id]
	descriptor := r.history[serverDescriptorHistoryKeyV1(exact)]
	r.mu.RUnlock()
	if !ok {
		return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "current MCP Server Descriptor not found")
	}
	if serverDescriptorRefV1(descriptor) != exact {
		return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "current MCP Server Descriptor index drifted")
	}
	return cloneMCPServerDescriptorV1(descriptor), nil
}

func serverDescriptorRefV1(descriptor toolcontract.MCPServerDescriptor) toolcontract.ObjectRef {
	return toolcontract.ObjectRef{ID: descriptor.ID, Revision: descriptor.Revision, Digest: descriptor.Digest}
}

func serverDescriptorHistoryKeyV1(exact toolcontract.ObjectRef) string {
	return exact.ID + "\x00" + string(exact.Digest)
}

func cloneMCPServerDescriptorV1(descriptor toolcontract.MCPServerDescriptor) toolcontract.MCPServerDescriptor {
	descriptor.Transports = append([]runtimeports.NamespacedNameV2(nil), descriptor.Transports...)
	return descriptor
}

func serverDescriptorContextV1(ctx context.Context) error {
	if ctx == nil {
		return invalid("MCP Server Descriptor context is required")
	}
	return ctx.Err()
}
