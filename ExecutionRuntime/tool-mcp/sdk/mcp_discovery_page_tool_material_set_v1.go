package sdk

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPDiscoveryPageToolMaterialSetV1 struct {
	reader toolcontract.MCPDiscoveryPageToolMaterialSetExactReaderV1
}

func NewMCPDiscoveryPageToolMaterialSetV1(reader toolcontract.MCPDiscoveryPageToolMaterialSetExactReaderV1) (*MCPDiscoveryPageToolMaterialSetV1, error) {
	if nilLikeV1(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Discovery Page Tool Material Set SDK dependency is required")
	}
	return &MCPDiscoveryPageToolMaterialSetV1{reader: reader}, nil
}

func (s *MCPDiscoveryPageToolMaterialSetV1) InspectMCPDiscoveryPageToolMaterialSetV1(ctx context.Context, exactReceipt toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageToolMaterialSetV1, error) {
	if err := s.readyV1(ctx); err != nil {
		return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, err
	}
	if exactReceipt.Validate() != nil {
		return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Discovery Page Tool Material Set receipt Ref is invalid")
	}
	first, err := s.reader.InspectMCPDiscoveryPageToolMaterialSetV1(ctx, exactReceipt)
	if err != nil {
		return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, err
	}
	second, err := s.reader.InspectMCPDiscoveryPageToolMaterialSetV1(ctx, exactReceipt)
	if err != nil {
		return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Receipt != exactReceipt || second.Receipt != exactReceipt || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page Tool Material Set SDK exact read drifted")
	}
	return toolcontract.CloneMCPDiscoveryPageToolMaterialSetV1(second), nil
}

func (s *MCPDiscoveryPageToolMaterialSetV1) readyV1(ctx context.Context) error {
	if s == nil || nilLikeV1(s.reader) {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Discovery Page Tool Material Set SDK is unavailable")
	}
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Discovery Page Tool Material Set SDK context is required")
	}
	return ctx.Err()
}

var _ toolcontract.MCPDiscoveryPageToolMaterialSetExactReaderV1 = (*MCPDiscoveryPageToolMaterialSetV1)(nil)
