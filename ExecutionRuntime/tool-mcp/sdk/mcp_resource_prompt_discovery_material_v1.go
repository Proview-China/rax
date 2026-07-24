package sdk

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPResourceDiscoveryMaterialV1 struct {
	reader toolcontract.MCPResourceDiscoveryMaterialExactReaderV1
}

func NewMCPResourceDiscoveryMaterialV1(reader toolcontract.MCPResourceDiscoveryMaterialExactReaderV1) (*MCPResourceDiscoveryMaterialV1, error) {
	if nilLikeV1(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Resource Discovery Material SDK dependency is required")
	}
	return &MCPResourceDiscoveryMaterialV1{reader: reader}, nil
}

func (s *MCPResourceDiscoveryMaterialV1) InspectExactMCPResourceDiscoveryMaterialV1(ctx context.Context, exact toolcontract.MCPResourceDiscoveryMaterialRefV1) (toolcontract.MCPResourceDiscoveryMaterialV1, error) {
	if s == nil || nilLikeV1(s.reader) {
		return toolcontract.MCPResourceDiscoveryMaterialV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Resource Discovery Material SDK is unavailable")
	}
	if ctx == nil || exact.Validate() != nil {
		return toolcontract.MCPResourceDiscoveryMaterialV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Resource Discovery Material SDK context or exact Ref is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPResourceDiscoveryMaterialV1{}, err
	}
	first, err := s.reader.InspectExactMCPResourceDiscoveryMaterialV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPResourceDiscoveryMaterialV1{}, err
	}
	second, err := s.reader.InspectExactMCPResourceDiscoveryMaterialV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPResourceDiscoveryMaterialV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Ref != exact || second.Ref != exact || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPResourceDiscoveryMaterialV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Resource Discovery Material SDK exact read drifted")
	}
	return second.Clone(), nil
}

type MCPPromptDiscoveryMaterialV1 struct {
	reader toolcontract.MCPPromptDiscoveryMaterialExactReaderV1
}

func NewMCPPromptDiscoveryMaterialV1(reader toolcontract.MCPPromptDiscoveryMaterialExactReaderV1) (*MCPPromptDiscoveryMaterialV1, error) {
	if nilLikeV1(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Prompt Discovery Material SDK dependency is required")
	}
	return &MCPPromptDiscoveryMaterialV1{reader: reader}, nil
}

func (s *MCPPromptDiscoveryMaterialV1) InspectExactMCPPromptDiscoveryMaterialV1(ctx context.Context, exact toolcontract.MCPPromptDiscoveryMaterialRefV1) (toolcontract.MCPPromptDiscoveryMaterialV1, error) {
	if s == nil || nilLikeV1(s.reader) {
		return toolcontract.MCPPromptDiscoveryMaterialV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Prompt Discovery Material SDK is unavailable")
	}
	if ctx == nil || exact.Validate() != nil {
		return toolcontract.MCPPromptDiscoveryMaterialV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Prompt Discovery Material SDK context or exact Ref is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPPromptDiscoveryMaterialV1{}, err
	}
	first, err := s.reader.InspectExactMCPPromptDiscoveryMaterialV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPPromptDiscoveryMaterialV1{}, err
	}
	second, err := s.reader.InspectExactMCPPromptDiscoveryMaterialV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPPromptDiscoveryMaterialV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Ref != exact || second.Ref != exact || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPPromptDiscoveryMaterialV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Prompt Discovery Material SDK exact read drifted")
	}
	return second.Clone(), nil
}

var _ toolcontract.MCPResourceDiscoveryMaterialExactReaderV1 = (*MCPResourceDiscoveryMaterialV1)(nil)
var _ toolcontract.MCPPromptDiscoveryMaterialExactReaderV1 = (*MCPPromptDiscoveryMaterialV1)(nil)
