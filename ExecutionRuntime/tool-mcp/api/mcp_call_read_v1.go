package api

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// MCPCallReadV1 exposes immutable Tool/MCP Owner command and protocol receipt
// facts. NotFound is never interpreted as proof that a Provider was not called.
type MCPCallReadV1 struct {
	commands toolcontract.MCPExecutionCommandExactReaderV1
	receipts toolcontract.MCPProtocolReceiptExactReaderV1
}

func NewMCPCallReadV1(commands toolcontract.MCPExecutionCommandExactReaderV1, receipts toolcontract.MCPProtocolReceiptExactReaderV1) (*MCPCallReadV1, error) {
	if nilLikeMCPCallReadV1(commands) || nilLikeMCPCallReadV1(receipts) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Call read API dependencies are required")
	}
	return &MCPCallReadV1{commands: commands, receipts: receipts}, nil
}

func (a *MCPCallReadV1) InspectMCPExecutionCommandV1(ctx context.Context, exact toolcontract.MCPExecutionCommandRefV1) (toolcontract.MCPExecutionCommandFactV1, error) {
	if err := a.readyV1(ctx); err != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, err
	}
	if exact.Validate() != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP execution command Ref is invalid")
	}
	first, err := a.commands.InspectMCPExecutionCommandV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, err
	}
	second, err := a.commands.InspectMCPExecutionCommandV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Ref != exact || second.Ref != exact || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPExecutionCommandFactV1{}, mcpCallReadDriftV1("Command")
	}
	return toolcontract.CloneMCPExecutionCommandFactV1(second), nil
}

func (a *MCPCallReadV1) InspectMCPProtocolReceiptV1(ctx context.Context, exact toolcontract.MCPProtocolReceiptRefV1) (toolcontract.MCPProtocolReceiptV1, error) {
	if err := a.readyV1(ctx); err != nil {
		return toolcontract.MCPProtocolReceiptV1{}, err
	}
	if exact.Validate() != nil {
		return toolcontract.MCPProtocolReceiptV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Protocol Receipt Ref is invalid")
	}
	first, err := a.receipts.InspectMCPProtocolReceiptV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPProtocolReceiptV1{}, err
	}
	second, err := a.receipts.InspectMCPProtocolReceiptV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPProtocolReceiptV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Ref != exact || second.Ref != exact || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPProtocolReceiptV1{}, mcpCallReadDriftV1("Receipt")
	}
	return toolcontract.CloneMCPProtocolReceiptV1(second), nil
}

func (a *MCPCallReadV1) readyV1(ctx context.Context) error {
	if a == nil || nilLikeMCPCallReadV1(a.commands) || nilLikeMCPCallReadV1(a.receipts) {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Call read API is unavailable")
	}
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Call read API context is required")
	}
	return ctx.Err()
}

func mcpCallReadDriftV1(kind string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Call read API "+kind+" differs from exact immutable Ref")
}

func nilLikeMCPCallReadV1(value any) bool {
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
