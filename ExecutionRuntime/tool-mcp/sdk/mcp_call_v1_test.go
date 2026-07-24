package sdk_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestMCPCallSDKV1ExactInspect(t *testing.T) {
	fixture := testkit.MCPExecutionV1(testkit.FixedTime)
	reader := mcpCallSDKReaderV1{command: fixture.Command, receipt: testkit.MCPProtocolReceiptV1(fixture, fixture.Now.Add(time.Second))}
	client, err := sdk.NewMCPCallV1(reader, reader)
	if err != nil {
		t.Fatal(err)
	}
	command, err := client.InspectMCPExecutionCommandV1(context.Background(), fixture.Command.Ref)
	if err != nil || command.Ref != fixture.Command.Ref {
		t.Fatalf("command=%#v err=%v", command, err)
	}
	receipt, err := client.InspectMCPProtocolReceiptV1(context.Background(), reader.receipt.Ref)
	if err != nil || receipt.Ref != reader.receipt.Ref {
		t.Fatalf("receipt=%#v err=%v", receipt, err)
	}
}

func TestMCPCallSDKV1FailsClosed(t *testing.T) {
	fixture := testkit.MCPExecutionV1(testkit.FixedTime)
	reader := mcpCallSDKReaderV1{command: fixture.Command, receipt: testkit.MCPProtocolReceiptV1(fixture, fixture.Now.Add(time.Second))}
	var typedNil *mcpCallSDKReaderV1
	if _, err := sdk.NewMCPCallV1(typedNil, reader); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil=%v", err)
	}
	client, _ := sdk.NewMCPCallV1(reader, reader)
	if _, err := client.InspectMCPExecutionCommandV1(nil, fixture.Command.Ref); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.InspectMCPProtocolReceiptV1(ctx, reader.receipt.Ref); err != context.Canceled {
		t.Fatalf("canceled context=%v", err)
	}
}

type mcpCallSDKReaderV1 struct {
	command toolcontract.MCPExecutionCommandFactV1
	receipt toolcontract.MCPProtocolReceiptV1
}

func (r mcpCallSDKReaderV1) InspectMCPExecutionCommandV1(_ context.Context, exact toolcontract.MCPExecutionCommandRefV1) (toolcontract.MCPExecutionCommandFactV1, error) {
	if exact != r.command.Ref {
		return toolcontract.MCPExecutionCommandFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "command not found")
	}
	return toolcontract.CloneMCPExecutionCommandFactV1(r.command), nil
}

func (r mcpCallSDKReaderV1) InspectMCPProtocolReceiptV1(_ context.Context, exact toolcontract.MCPProtocolReceiptRefV1) (toolcontract.MCPProtocolReceiptV1, error) {
	if exact != r.receipt.Ref {
		return toolcontract.MCPProtocolReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "receipt not found")
	}
	return toolcontract.CloneMCPProtocolReceiptV1(r.receipt), nil
}
