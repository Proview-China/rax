package sdk

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// MCPCallV1 is an exact read-only SDK for immutable governed Call facts. It
// cannot create a command, call a Provider, or infer no-effect from NotFound.
type MCPCallV1 struct {
	commands toolcontract.MCPExecutionCommandExactReaderV1
	receipts toolcontract.MCPProtocolReceiptExactReaderV1
}

func NewMCPCallV1(commands toolcontract.MCPExecutionCommandExactReaderV1, receipts toolcontract.MCPProtocolReceiptExactReaderV1) (*MCPCallV1, error) {
	if nilLikeV1(commands) || nilLikeV1(receipts) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Call SDK dependencies are required")
	}
	return &MCPCallV1{commands: commands, receipts: receipts}, nil
}

func (s *MCPCallV1) InspectMCPExecutionCommandV1(ctx context.Context, exact toolcontract.MCPExecutionCommandRefV1) (toolcontract.MCPExecutionCommandFactV1, error) {
	if err := s.readyV1(ctx); err != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, err
	}
	if exact.Validate() != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP execution command Ref is invalid")
	}
	first, err := s.commands.InspectMCPExecutionCommandV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, err
	}
	second, err := s.commands.InspectMCPExecutionCommandV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPExecutionCommandFactV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Ref != exact || second.Ref != exact || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPExecutionCommandFactV1{}, mcpCallSDKDriftV1("Command")
	}
	return toolcontract.CloneMCPExecutionCommandFactV1(second), nil
}

func (s *MCPCallV1) InspectMCPProtocolReceiptV1(ctx context.Context, exact toolcontract.MCPProtocolReceiptRefV1) (toolcontract.MCPProtocolReceiptV1, error) {
	if err := s.readyV1(ctx); err != nil {
		return toolcontract.MCPProtocolReceiptV1{}, err
	}
	if exact.Validate() != nil {
		return toolcontract.MCPProtocolReceiptV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Protocol Receipt Ref is invalid")
	}
	first, err := s.receipts.InspectMCPProtocolReceiptV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPProtocolReceiptV1{}, err
	}
	second, err := s.receipts.InspectMCPProtocolReceiptV1(ctx, exact)
	if err != nil {
		return toolcontract.MCPProtocolReceiptV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Ref != exact || second.Ref != exact || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPProtocolReceiptV1{}, mcpCallSDKDriftV1("Receipt")
	}
	return toolcontract.CloneMCPProtocolReceiptV1(second), nil
}

func (s *MCPCallV1) readyV1(ctx context.Context) error {
	if s == nil || nilLikeV1(s.commands) || nilLikeV1(s.receipts) {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Call SDK is unavailable")
	}
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Call SDK context is required")
	}
	return ctx.Err()
}

func mcpCallSDKDriftV1(kind string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Call SDK "+kind+" differs from exact immutable Ref")
}
