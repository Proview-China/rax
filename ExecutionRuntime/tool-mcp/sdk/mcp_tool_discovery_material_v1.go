package sdk

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// MCPToolDiscoveryMaterialV1 is an exact read-only SDK over untrusted Tool
// definition material observed during a governed MCP Discovery Page.
type MCPToolDiscoveryMaterialV1 struct {
	reader toolcontract.MCPToolDiscoveryMaterialExactReaderV1
}

func NewMCPToolDiscoveryMaterialV1(reader toolcontract.MCPToolDiscoveryMaterialExactReaderV1) (*MCPToolDiscoveryMaterialV1, error) {
	if nilLikeV1(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Tool Discovery Material SDK dependency is required")
	}
	return &MCPToolDiscoveryMaterialV1{reader: reader}, nil
}

func (s *MCPToolDiscoveryMaterialV1) InspectExactMCPToolDiscoveryMaterialV1(ctx context.Context, exact toolcontract.MCPToolDiscoveryMaterialRefV1) (toolcontract.MCPToolDiscoveryMaterialV1, error) {
	if err := s.readyV1(ctx); err != nil {
		return toolcontract.MCPToolDiscoveryMaterialV1{}, err
	}
	if exact.Validate() != nil {
		return toolcontract.MCPToolDiscoveryMaterialV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Tool Discovery Material exact Ref is invalid")
	}
	first, err := s.reader.InspectExactMCPToolDiscoveryMaterialV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPToolDiscoveryMaterialV1{}, err
	}
	second, err := s.reader.InspectExactMCPToolDiscoveryMaterialV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPToolDiscoveryMaterialV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Ref != exact || second.Ref != exact || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPToolDiscoveryMaterialV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Tool Discovery Material SDK exact read drifted")
	}
	return second.Clone(), nil
}

func (s *MCPToolDiscoveryMaterialV1) readyV1(ctx context.Context) error {
	if s == nil || nilLikeV1(s.reader) {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Tool Discovery Material SDK is unavailable")
	}
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Tool Discovery Material SDK context is required")
	}
	return ctx.Err()
}

var _ toolcontract.MCPToolDiscoveryMaterialExactReaderV1 = (*MCPToolDiscoveryMaterialV1)(nil)
