package api

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// MCPToolDiscoveryMaterialReadV1 is a transport-neutral exact read API. It
// cannot discover, map, admit, or execute a Tool.
type MCPToolDiscoveryMaterialReadV1 struct {
	reader toolcontract.MCPToolDiscoveryMaterialExactReaderV1
}

func NewMCPToolDiscoveryMaterialReadV1(reader toolcontract.MCPToolDiscoveryMaterialExactReaderV1) (*MCPToolDiscoveryMaterialReadV1, error) {
	if nilLikeMCPToolDiscoveryMaterialReadV1(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Tool Discovery Material read dependency is required")
	}
	return &MCPToolDiscoveryMaterialReadV1{reader: reader}, nil
}

func (a *MCPToolDiscoveryMaterialReadV1) InspectExactMCPToolDiscoveryMaterialV1(ctx context.Context, exact toolcontract.MCPToolDiscoveryMaterialRefV1) (toolcontract.MCPToolDiscoveryMaterialV1, error) {
	if err := a.readyV1(ctx); err != nil {
		return toolcontract.MCPToolDiscoveryMaterialV1{}, err
	}
	if exact.Validate() != nil {
		return toolcontract.MCPToolDiscoveryMaterialV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Tool Discovery Material read exact Ref is invalid")
	}
	first, err := a.reader.InspectExactMCPToolDiscoveryMaterialV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPToolDiscoveryMaterialV1{}, err
	}
	second, err := a.reader.InspectExactMCPToolDiscoveryMaterialV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPToolDiscoveryMaterialV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Ref != exact || second.Ref != exact || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPToolDiscoveryMaterialV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Tool Discovery Material read differs from exact immutable Ref")
	}
	return second.Clone(), nil
}

func (a *MCPToolDiscoveryMaterialReadV1) readyV1(ctx context.Context) error {
	if a == nil || nilLikeMCPToolDiscoveryMaterialReadV1(a.reader) {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Tool Discovery Material read API is unavailable")
	}
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Tool Discovery Material read context is required")
	}
	return ctx.Err()
}

func nilLikeMCPToolDiscoveryMaterialReadV1(value any) bool {
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

var _ toolcontract.MCPToolDiscoveryMaterialExactReaderV1 = (*MCPToolDiscoveryMaterialReadV1)(nil)
