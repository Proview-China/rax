package api

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPResourceDiscoveryMaterialReadV1 struct {
	reader toolcontract.MCPResourceDiscoveryMaterialExactReaderV1
}

func NewMCPResourceDiscoveryMaterialReadV1(reader toolcontract.MCPResourceDiscoveryMaterialExactReaderV1) (*MCPResourceDiscoveryMaterialReadV1, error) {
	if nilLikeMCPToolDiscoveryMaterialReadV1(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Resource Discovery Material read dependency is required")
	}
	return &MCPResourceDiscoveryMaterialReadV1{reader: reader}, nil
}

func (a *MCPResourceDiscoveryMaterialReadV1) InspectExactMCPResourceDiscoveryMaterialV1(ctx context.Context, exact toolcontract.MCPResourceDiscoveryMaterialRefV1) (toolcontract.MCPResourceDiscoveryMaterialV1, error) {
	if a == nil || nilLikeMCPToolDiscoveryMaterialReadV1(a.reader) {
		return toolcontract.MCPResourceDiscoveryMaterialV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Resource Discovery Material read API is unavailable")
	}
	if ctx == nil || exact.Validate() != nil {
		return toolcontract.MCPResourceDiscoveryMaterialV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Resource Discovery Material read context or exact Ref is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPResourceDiscoveryMaterialV1{}, err
	}
	first, err := a.reader.InspectExactMCPResourceDiscoveryMaterialV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPResourceDiscoveryMaterialV1{}, err
	}
	second, err := a.reader.InspectExactMCPResourceDiscoveryMaterialV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPResourceDiscoveryMaterialV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Ref != exact || second.Ref != exact || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPResourceDiscoveryMaterialV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Resource Discovery Material read differs from exact immutable Ref")
	}
	return second.Clone(), nil
}

type MCPPromptDiscoveryMaterialReadV1 struct {
	reader toolcontract.MCPPromptDiscoveryMaterialExactReaderV1
}

func NewMCPPromptDiscoveryMaterialReadV1(reader toolcontract.MCPPromptDiscoveryMaterialExactReaderV1) (*MCPPromptDiscoveryMaterialReadV1, error) {
	if nilLikeMCPToolDiscoveryMaterialReadV1(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Prompt Discovery Material read dependency is required")
	}
	return &MCPPromptDiscoveryMaterialReadV1{reader: reader}, nil
}

func (a *MCPPromptDiscoveryMaterialReadV1) InspectExactMCPPromptDiscoveryMaterialV1(ctx context.Context, exact toolcontract.MCPPromptDiscoveryMaterialRefV1) (toolcontract.MCPPromptDiscoveryMaterialV1, error) {
	if a == nil || nilLikeMCPToolDiscoveryMaterialReadV1(a.reader) {
		return toolcontract.MCPPromptDiscoveryMaterialV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Prompt Discovery Material read API is unavailable")
	}
	if ctx == nil || exact.Validate() != nil {
		return toolcontract.MCPPromptDiscoveryMaterialV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Prompt Discovery Material read context or exact Ref is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPPromptDiscoveryMaterialV1{}, err
	}
	first, err := a.reader.InspectExactMCPPromptDiscoveryMaterialV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPPromptDiscoveryMaterialV1{}, err
	}
	second, err := a.reader.InspectExactMCPPromptDiscoveryMaterialV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPPromptDiscoveryMaterialV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Ref != exact || second.Ref != exact || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPPromptDiscoveryMaterialV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Prompt Discovery Material read differs from exact immutable Ref")
	}
	return second.Clone(), nil
}

var _ toolcontract.MCPResourceDiscoveryMaterialExactReaderV1 = (*MCPResourceDiscoveryMaterialReadV1)(nil)
var _ toolcontract.MCPPromptDiscoveryMaterialExactReaderV1 = (*MCPPromptDiscoveryMaterialReadV1)(nil)
