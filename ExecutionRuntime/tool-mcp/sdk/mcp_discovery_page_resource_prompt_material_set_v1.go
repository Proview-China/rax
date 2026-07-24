package sdk

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPDiscoveryPageResourceMaterialSetV1 struct {
	reader toolcontract.MCPDiscoveryPageResourceMaterialSetExactReaderV1
}

func NewMCPDiscoveryPageResourceMaterialSetV1(reader toolcontract.MCPDiscoveryPageResourceMaterialSetExactReaderV1) (*MCPDiscoveryPageResourceMaterialSetV1, error) {
	if nilLikeV1(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Discovery Page Resource Material Set SDK dependency is required")
	}
	return &MCPDiscoveryPageResourceMaterialSetV1{reader: reader}, nil
}

func (s *MCPDiscoveryPageResourceMaterialSetV1) InspectMCPDiscoveryPageResourceMaterialSetV1(ctx context.Context, exactReceipt toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageResourceMaterialSetV1, error) {
	if s == nil || nilLikeV1(s.reader) {
		return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Discovery Page Resource Material Set SDK is unavailable")
	}
	if ctx == nil || exactReceipt.Validate() != nil {
		return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Discovery Page Resource Material Set context or receipt is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, err
	}
	first, err := s.reader.InspectMCPDiscoveryPageResourceMaterialSetV1(ctx, exactReceipt)
	if err != nil {
		return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, err
	}
	second, err := s.reader.InspectMCPDiscoveryPageResourceMaterialSetV1(ctx, exactReceipt)
	if err != nil {
		return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Receipt != exactReceipt || second.Receipt != exactReceipt || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page Resource Material Set exact read drifted")
	}
	return toolcontract.CloneMCPDiscoveryPageResourceMaterialSetV1(second), nil
}

type MCPDiscoveryPagePromptMaterialSetV1 struct {
	reader toolcontract.MCPDiscoveryPagePromptMaterialSetExactReaderV1
}

func NewMCPDiscoveryPagePromptMaterialSetV1(reader toolcontract.MCPDiscoveryPagePromptMaterialSetExactReaderV1) (*MCPDiscoveryPagePromptMaterialSetV1, error) {
	if nilLikeV1(reader) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "MCP Discovery Page Prompt Material Set SDK dependency is required")
	}
	return &MCPDiscoveryPagePromptMaterialSetV1{reader: reader}, nil
}

func (s *MCPDiscoveryPagePromptMaterialSetV1) InspectMCPDiscoveryPagePromptMaterialSetV1(ctx context.Context, exactReceipt toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPagePromptMaterialSetV1, error) {
	if s == nil || nilLikeV1(s.reader) {
		return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "MCP Discovery Page Prompt Material Set SDK is unavailable")
	}
	if ctx == nil || exactReceipt.Validate() != nil {
		return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "MCP Discovery Page Prompt Material Set context or receipt is invalid")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, err
	}
	first, err := s.reader.InspectMCPDiscoveryPagePromptMaterialSetV1(ctx, exactReceipt)
	if err != nil {
		return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, err
	}
	second, err := s.reader.InspectMCPDiscoveryPagePromptMaterialSetV1(ctx, exactReceipt)
	if err != nil {
		return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, err
	}
	if first.Validate() != nil || second.Validate() != nil || first.Receipt != exactReceipt || second.Receipt != exactReceipt || !reflect.DeepEqual(first, second) {
		return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "MCP Discovery Page Prompt Material Set exact read drifted")
	}
	return toolcontract.CloneMCPDiscoveryPagePromptMaterialSetV1(second), nil
}

var _ toolcontract.MCPDiscoveryPageResourceMaterialSetExactReaderV1 = (*MCPDiscoveryPageResourceMaterialSetV1)(nil)
var _ toolcontract.MCPDiscoveryPagePromptMaterialSetExactReaderV1 = (*MCPDiscoveryPagePromptMaterialSetV1)(nil)
