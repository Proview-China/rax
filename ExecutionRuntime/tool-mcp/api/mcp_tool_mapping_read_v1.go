package api

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPToolMappingReadV1 struct {
	reader toolcontract.MCPToolMappingManifestExactReaderV1
}

func NewMCPToolMappingReadV1(reader toolcontract.MCPToolMappingManifestExactReaderV1) (*MCPToolMappingReadV1, error) {
	if nilLikeMCPDiscoveryReadV2(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Tool Mapping read dependency is required")
	}
	return &MCPToolMappingReadV1{reader: reader}, nil
}

func (a *MCPToolMappingReadV1) InspectMCPToolMappingManifestV1(ctx context.Context, exact toolcontract.MCPToolMappingManifestRefV1) (toolcontract.MCPToolMappingManifestV1, error) {
	if ctx == nil {
		return toolcontract.MCPToolMappingManifestV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Tool Mapping read context is required")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPToolMappingManifestV1{}, err
	}
	if a == nil || nilLikeMCPDiscoveryReadV2(a.reader) || exact.Validate() != nil {
		return toolcontract.MCPToolMappingManifestV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Tool Mapping read API is unavailable or invalid")
	}
	first, err := a.reader.InspectMCPToolMappingManifestV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPToolMappingManifestV1{}, err
	}
	second, err := a.reader.InspectMCPToolMappingManifestV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPToolMappingManifestV1{}, err
	}
	if first.Ref != exact || second.Ref != exact || first.Validate() != nil || second.Validate() != nil || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPToolMappingManifestV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Tool Mapping changed during exact read")
	}
	return second, nil
}
