package api_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestMCPCallReadAPIV1ExactImmutableAndClone(t *testing.T) {
	fixture := testkit.MCPExecutionV1(testkit.FixedTime)
	source := &mcpCallReadSourceV1{command: fixture.Command, receipt: testkit.MCPProtocolReceiptV1(fixture, fixture.Now.Add(time.Second))}
	read, err := api.NewMCPCallReadV1(source, source)
	if err != nil {
		t.Fatal(err)
	}
	command, err := read.InspectMCPExecutionCommandV1(context.Background(), fixture.Command.Ref)
	if err != nil || command.Ref != fixture.Command.Ref {
		t.Fatalf("command=%#v err=%v", command, err)
	}
	receipt, err := read.InspectMCPProtocolReceiptV1(context.Background(), source.receipt.Ref)
	if err != nil || receipt.Ref != source.receipt.Ref {
		t.Fatalf("receipt=%#v err=%v", receipt, err)
	}
	command.Params.Inline[0] ^= 0xff
	receipt.CanonicalResponse[0] ^= 0xff
	againCommand, _ := read.InspectMCPExecutionCommandV1(context.Background(), fixture.Command.Ref)
	againReceipt, _ := read.InspectMCPProtocolReceiptV1(context.Background(), source.receipt.Ref)
	if againCommand.Params.ContentDigest != fixture.Command.Params.ContentDigest || againReceipt.ResponseDigest != source.receipt.ResponseDigest {
		t.Fatal("MCP Call read API exposed aliased immutable facts")
	}
}

func TestMCPCallReadAPIV1FailsClosedAndConcurrent(t *testing.T) {
	fixture := testkit.MCPExecutionV1(testkit.FixedTime)
	source := &mcpCallReadSourceV1{command: fixture.Command, receipt: testkit.MCPProtocolReceiptV1(fixture, fixture.Now.Add(time.Second))}
	var typedNil *mcpCallReadSourceV1
	if _, err := api.NewMCPCallReadV1(typedNil, source); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil=%v", err)
	}
	read, _ := api.NewMCPCallReadV1(source, source)
	if _, err := read.InspectMCPExecutionCommandV1(nil, fixture.Command.Ref); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := read.InspectMCPProtocolReceiptV1(ctx, source.receipt.Ref); err != context.Canceled {
		t.Fatalf("canceled context=%v", err)
	}
	source.drift = true
	if _, err := read.InspectMCPExecutionCommandV1(context.Background(), fixture.Command.Ref); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("S1/S2 command drift=%v", err)
	}
	source.drift = false
	const workers = 64
	var group sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := read.InspectMCPProtocolReceiptV1(context.Background(), source.receipt.Ref)
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

type mcpCallReadSourceV1 struct {
	mu      sync.Mutex
	command toolcontract.MCPExecutionCommandFactV1
	receipt toolcontract.MCPProtocolReceiptV1
	drift   bool
	calls   int
}

func (s *mcpCallReadSourceV1) InspectMCPExecutionCommandV1(_ context.Context, exact toolcontract.MCPExecutionCommandRefV1) (toolcontract.MCPExecutionCommandFactV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if exact != s.command.Ref {
		return toolcontract.MCPExecutionCommandFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "command not found")
	}
	s.calls++
	value := toolcontract.CloneMCPExecutionCommandFactV1(s.command)
	if s.drift && s.calls%2 == 0 {
		value.JSONRPCRequestID = "drifted-request"
	}
	return value, nil
}

func (s *mcpCallReadSourceV1) InspectMCPProtocolReceiptV1(_ context.Context, exact toolcontract.MCPProtocolReceiptRefV1) (toolcontract.MCPProtocolReceiptV1, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if exact != s.receipt.Ref {
		return toolcontract.MCPProtocolReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "receipt not found")
	}
	return toolcontract.CloneMCPProtocolReceiptV1(s.receipt), nil
}
