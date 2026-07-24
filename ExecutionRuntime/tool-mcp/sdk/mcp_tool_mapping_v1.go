package sdk

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

type MCPToolMappingAdmissionPortV1 interface {
	AdmitMCPToolMappingV1(context.Context, toolcontract.MCPToolMappingAdmissionRequestV1) (registry.MCPToolMappingAdmissionResultV1, error)
}

type MCPToolMappingV1 struct {
	admission MCPToolMappingAdmissionPortV1
}

func NewMCPToolMappingV1(admission MCPToolMappingAdmissionPortV1) (*MCPToolMappingV1, error) {
	if nilLikeV1(admission) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Tool Mapping SDK Admission Port is required")
	}
	return &MCPToolMappingV1{admission: admission}, nil
}

func (s *MCPToolMappingV1) AdmitMCPToolMappingV1(ctx context.Context, request toolcontract.MCPToolMappingAdmissionRequestV1) (registry.MCPToolMappingAdmissionResultV1, error) {
	if ctx == nil {
		return registry.MCPToolMappingAdmissionResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Tool Mapping SDK context is required")
	}
	if err := ctx.Err(); err != nil {
		return registry.MCPToolMappingAdmissionResultV1{}, err
	}
	if s == nil || nilLikeV1(s.admission) {
		return registry.MCPToolMappingAdmissionResultV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Tool Mapping SDK is unavailable")
	}
	return s.admission.AdmitMCPToolMappingV1(ctx, request)
}
