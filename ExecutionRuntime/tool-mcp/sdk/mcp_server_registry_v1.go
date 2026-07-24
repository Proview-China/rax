package sdk

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
)

// MCPServerRegistryV1 manages immutable Tool-owned server descriptors only.
// Registration does not connect, initialize, discover, or authorize a server.
type MCPServerRegistryV1 struct {
	repository mcp.MCPServerDescriptorRepositoryV1
	clock      ClockV1
}

func NewMCPServerRegistryV1(repository mcp.MCPServerDescriptorRepositoryV1, clock ClockV1) (*MCPServerRegistryV1, error) {
	if nilLikeV1(repository) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Server Registry SDK dependencies are required")
	}
	return &MCPServerRegistryV1{repository: repository, clock: clock}, nil
}

func (s *MCPServerRegistryV1) RegisterMCPServerV1(ctx context.Context, descriptor toolcontract.MCPServerDescriptor, expectedCurrent *toolcontract.ObjectRef) (toolcontract.MCPServerDescriptor, error) {
	now, err := s.readyMCPServerRegistryV1(ctx)
	if err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	if descriptor.Validate() != nil {
		return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Server Descriptor is invalid")
	}
	if descriptor.CreatedUnixNano > now.UnixNano() {
		return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Server Descriptor cannot be registered before creation")
	}
	request := mcp.EnsureMCPServerDescriptorRequestV1{Descriptor: descriptor, ExpectedCurrent: cloneMCPServerObjectRefV1(expectedCurrent)}
	registered, err := s.repository.EnsureMCPServerDescriptorV1(ctx, request)
	if err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	if registered.Validate() != nil || registered.ID != descriptor.ID || registered.Revision != descriptor.Revision || registered.Digest != descriptor.Digest {
		return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "registered MCP Server Descriptor differs from request")
	}
	return cloneMCPServerDescriptorSDKV1(registered), nil
}

func (s *MCPServerRegistryV1) InspectMCPServerV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPServerDescriptor, error) {
	if _, err := s.readyMCPServerRegistryV1(ctx); err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	value, err := s.repository.InspectMCPServerDescriptorV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	if value.Validate() != nil || value.ID != exact.ID || value.Revision != exact.Revision || value.Digest != exact.Digest {
		return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Server Descriptor differs from exact Ref")
	}
	return cloneMCPServerDescriptorSDKV1(value), nil
}

func (s *MCPServerRegistryV1) InspectCurrentMCPServerV1(ctx context.Context, id string) (toolcontract.MCPServerDescriptor, error) {
	if _, err := s.readyMCPServerRegistryV1(ctx); err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	value, err := s.repository.InspectCurrentMCPServerDescriptorV1(ctx, id)
	if err != nil {
		return toolcontract.MCPServerDescriptor{}, err
	}
	if value.Validate() != nil || value.ID != id {
		return toolcontract.MCPServerDescriptor{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "current MCP Server Descriptor differs from ID")
	}
	return cloneMCPServerDescriptorSDKV1(value), nil
}

func (s *MCPServerRegistryV1) readyMCPServerRegistryV1(ctx context.Context) (time.Time, error) {
	if s == nil || nilLikeV1(s.repository) || s.clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Server Registry SDK is unavailable")
	}
	if ctx == nil {
		return time.Time{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Server Registry SDK context is required")
	}
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	now := s.clock()
	if now.IsZero() {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "MCP Server Registry SDK clock is unavailable")
	}
	return now.UTC(), nil
}

func cloneMCPServerObjectRefV1(value *toolcontract.ObjectRef) *toolcontract.ObjectRef {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneMCPServerDescriptorSDKV1(value toolcontract.MCPServerDescriptor) toolcontract.MCPServerDescriptor {
	value.Transports = append([]runtimeports.NamespacedNameV2(nil), value.Transports...)
	return value
}
