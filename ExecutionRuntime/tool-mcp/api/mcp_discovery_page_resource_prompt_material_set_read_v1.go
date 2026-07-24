package api

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPDiscoveryPageResourceMaterialSetReadV1 struct {
	reader toolcontract.MCPDiscoveryPageResourceMaterialSetExactReaderV1
}

func NewMCPDiscoveryPageResourceMaterialSetReadV1(reader toolcontract.MCPDiscoveryPageResourceMaterialSetExactReaderV1) (*MCPDiscoveryPageResourceMaterialSetReadV1, error) {
	if nilLikeMCPDiscoveryPageToolMaterialSetReadV1(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Discovery Page Resource Material Set read dependency is required")
	}
	return &MCPDiscoveryPageResourceMaterialSetReadV1{reader: reader}, nil
}

func (a *MCPDiscoveryPageResourceMaterialSetReadV1) InspectMCPDiscoveryPageResourceMaterialSetV1(ctx context.Context, exactReceipt toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageResourceMaterialSetV1, error) {
	if a == nil || nilLikeMCPDiscoveryPageToolMaterialSetReadV1(a.reader) {
		return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Discovery Page Resource Material Set read API is unavailable")
	}
	if ctx == nil || exactReceipt.Validate() != nil {
		return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Discovery Page Resource Material Set context or receipt is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, err
	}
	first, err := a.reader.InspectMCPDiscoveryPageResourceMaterialSetV1(ctx, exactReceipt)
	if err != nil {
		return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, err
	}
	second, err := a.reader.InspectMCPDiscoveryPageResourceMaterialSetV1(ctx, exactReceipt)
	if err != nil {
		return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Receipt != exactReceipt || second.Receipt != exactReceipt || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page Resource Material Set read drifted")
	}
	return toolcontract.CloneMCPDiscoveryPageResourceMaterialSetV1(second), nil
}

type MCPDiscoveryPagePromptMaterialSetReadV1 struct {
	reader toolcontract.MCPDiscoveryPagePromptMaterialSetExactReaderV1
}

func NewMCPDiscoveryPagePromptMaterialSetReadV1(reader toolcontract.MCPDiscoveryPagePromptMaterialSetExactReaderV1) (*MCPDiscoveryPagePromptMaterialSetReadV1, error) {
	if nilLikeMCPDiscoveryPageToolMaterialSetReadV1(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Discovery Page Prompt Material Set read dependency is required")
	}
	return &MCPDiscoveryPagePromptMaterialSetReadV1{reader: reader}, nil
}

func (a *MCPDiscoveryPagePromptMaterialSetReadV1) InspectMCPDiscoveryPagePromptMaterialSetV1(ctx context.Context, exactReceipt toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPagePromptMaterialSetV1, error) {
	if a == nil || nilLikeMCPDiscoveryPageToolMaterialSetReadV1(a.reader) {
		return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Discovery Page Prompt Material Set read API is unavailable")
	}
	if ctx == nil || exactReceipt.Validate() != nil {
		return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Discovery Page Prompt Material Set context or receipt is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, err
	}
	first, err := a.reader.InspectMCPDiscoveryPagePromptMaterialSetV1(ctx, exactReceipt)
	if err != nil {
		return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, err
	}
	second, err := a.reader.InspectMCPDiscoveryPagePromptMaterialSetV1(ctx, exactReceipt)
	if err != nil {
		return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Receipt != exactReceipt || second.Receipt != exactReceipt || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page Prompt Material Set read drifted")
	}
	return toolcontract.CloneMCPDiscoveryPagePromptMaterialSetV1(second), nil
}

var _ toolcontract.MCPDiscoveryPageResourceMaterialSetExactReaderV1 = (*MCPDiscoveryPageResourceMaterialSetReadV1)(nil)
var _ toolcontract.MCPDiscoveryPagePromptMaterialSetExactReaderV1 = (*MCPDiscoveryPagePromptMaterialSetReadV1)(nil)
