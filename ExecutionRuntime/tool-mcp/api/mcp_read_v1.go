package api

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPServerReadPortV1 interface {
	InspectMCPServerV1(context.Context, toolcontract.ObjectRef) (toolcontract.MCPServerDescriptor, error)
	InspectCurrentMCPServerV1(context.Context, string) (toolcontract.MCPServerDescriptor, error)
}

type MCPProcessObservationReadPortV1 interface {
	toolcontract.MCPProcessObservationReadPortV1
}

type MCPReadV1 struct {
	servers MCPServerReadPortV1
	process MCPProcessObservationReadPortV1
}

func NewMCPReadV1(servers MCPServerReadPortV1, process MCPProcessObservationReadPortV1) (*MCPReadV1, error) {
	if nilLikeMCPReadV1(servers) || nilLikeMCPReadV1(process) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP read API dependencies are required")
	}
	return &MCPReadV1{servers: servers, process: process}, nil
}

func (a *MCPReadV1) InspectMCPServerV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPServerDescriptor, error) {
	if err := a.readyMCPReadV1(ctx); err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	value, err := a.servers.InspectMCPServerV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	if value.Validate() != nil || value.ID != exact.ID || value.Revision != exact.Revision || value.Digest != exact.Digest {
		return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP read API Server differs from exact Ref")
	}
	return cloneMCPServerReadV1(value), nil
}

func (a *MCPReadV1) InspectCurrentMCPServerV1(ctx context.Context, id string) (toolcontract.MCPServerDescriptor, error) {
	if err := a.readyMCPReadV1(ctx); err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	value, err := a.servers.InspectCurrentMCPServerV1(ctx, id)
	if err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	if value.Validate() != nil || value.ID != id {
		return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP read API current Server differs from ID")
	}
	return cloneMCPServerReadV1(value), nil
}

func (a *MCPReadV1) InspectMCPProcessObservationV1(ctx context.Context, exact toolcontract.MCPProcessObservationRefV1) (toolcontract.MCPProcessObservationV1, error) {
	if err := a.readyMCPReadV1(ctx); err != nil {
		return toolcontract.MCPProcessObservationV1{}, err
	}
	value, err := a.process.InspectMCPProcessObservationV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPProcessObservationV1{}, err
	}
	if value.Validate() != nil || value.Ref != exact {
		return toolcontract.MCPProcessObservationV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP read API process Observation differs from exact Ref")
	}
	return value, nil
}

func (a *MCPReadV1) ReadMCPProcessObservationPageV1(ctx context.Context, request toolcontract.MCPProcessObservationPageRequestV1) (toolcontract.MCPProcessObservationPageV1, error) {
	if err := a.readyMCPReadV1(ctx); err != nil {
		return toolcontract.MCPProcessObservationPageV1{}, err
	}
	if request.Validate() != nil {
		return toolcontract.MCPProcessObservationPageV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP read API process page request is invalid")
	}
	page, err := a.process.ReadMCPProcessObservationPageV1(ctx, request)
	if err != nil {
		return toolcontract.MCPProcessObservationPageV1{}, err
	}
	if page.Validate() != nil || page.Request != request {
		return toolcontract.MCPProcessObservationPageV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP read API process page differs from exact request")
	}
	return toolcontract.CloneMCPProcessObservationPageV1(page), nil
}

func (a *MCPReadV1) readyMCPReadV1(ctx context.Context) error {
	if a == nil || nilLikeMCPReadV1(a.servers) || nilLikeMCPReadV1(a.process) {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP read API is unavailable")
	}
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP read API context is required")
	}
	return ctx.Err()
}

func cloneMCPServerReadV1(value toolcontract.MCPServerDescriptor) toolcontract.MCPServerDescriptor {
	value.Transports = append([]runtimeports.NamespacedNameV2(nil), value.Transports...)
	return value
}

func nilLikeMCPReadV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
