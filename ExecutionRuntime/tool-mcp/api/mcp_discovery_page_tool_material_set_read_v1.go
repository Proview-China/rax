package api

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPDiscoveryPageToolMaterialSetReadV1 struct {
	reader toolcontract.MCPDiscoveryPageToolMaterialSetExactReaderV1
}

func NewMCPDiscoveryPageToolMaterialSetReadV1(reader toolcontract.MCPDiscoveryPageToolMaterialSetExactReaderV1) (*MCPDiscoveryPageToolMaterialSetReadV1, error) {
	if nilLikeMCPDiscoveryPageToolMaterialSetReadV1(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Discovery Page Tool Material Set read dependency is required")
	}
	return &MCPDiscoveryPageToolMaterialSetReadV1{reader: reader}, nil
}

func (a *MCPDiscoveryPageToolMaterialSetReadV1) InspectMCPDiscoveryPageToolMaterialSetV1(ctx context.Context, exactReceipt toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageToolMaterialSetV1, error) {
	if err := a.readyV1(ctx); err != nil {
		return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, err
	}
	if exactReceipt.Validate() != nil {
		return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Discovery Page Tool Material Set read receipt Ref is invalid")
	}
	first, err := a.reader.InspectMCPDiscoveryPageToolMaterialSetV1(ctx, exactReceipt)
	if err != nil {
		return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, err
	}
	second, err := a.reader.InspectMCPDiscoveryPageToolMaterialSetV1(ctx, exactReceipt)
	if err != nil {
		return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Receipt != exactReceipt || second.Receipt != exactReceipt || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page Tool Material Set read differs from exact receipt")
	}
	return toolcontract.CloneMCPDiscoveryPageToolMaterialSetV1(second), nil
}

func (a *MCPDiscoveryPageToolMaterialSetReadV1) readyV1(ctx context.Context) error {
	if a == nil || nilLikeMCPDiscoveryPageToolMaterialSetReadV1(a.reader) {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Discovery Page Tool Material Set read API is unavailable")
	}
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Discovery Page Tool Material Set read context is required")
	}
	return ctx.Err()
}

func nilLikeMCPDiscoveryPageToolMaterialSetReadV1(value any) bool {
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

var _ toolcontract.MCPDiscoveryPageToolMaterialSetExactReaderV1 = (*MCPDiscoveryPageToolMaterialSetReadV1)(nil)
