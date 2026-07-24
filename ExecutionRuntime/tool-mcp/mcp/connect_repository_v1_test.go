package mcp_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	toolmcp "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
)

func TestMCPTransportConfigRepositoryCASLostReplyAndDeepCloneV1(t *testing.T) {
	ctx := context.Background()
	repository := toolmcp.NewInMemoryMCPTransportConfigRepositoryV1()
	fixture := testkit.MCPConnectV1(testkit.FixedTime, toolcontract.MCPTransportStdioV1)
	winner, err := repository.EnsureMCPTransportConfigV1(ctx, toolmcp.EnsureMCPTransportConfigRequestV1{Config: fixture.Config})
	if err != nil {
		t.Fatal(err)
	}
	winner.Stdio.Arguments[0] = "mutated"
	inspected, err := repository.InspectMCPTransportConfigV1(ctx, fixture.Config.Ref)
	if err != nil || inspected.Stdio.Arguments[0] != "--stdio" {
		t.Fatal("MCP Transport Config deep clone or lost-reply Inspect failed")
	}
	successor := fixture.Config
	successor.Ref.Revision = 2
	successor.Ref.Digest = ""
	successor.CreatedUnixNano++
	successor.Stdio.Arguments = []string{"--stdio", "--v2"}
	successor, err = toolcontract.SealMCPTransportConfigV1(successor)
	if err != nil {
		t.Fatal(err)
	}
	winner2, err := repository.EnsureMCPTransportConfigV1(ctx, toolmcp.EnsureMCPTransportConfigRequestV1{Config: successor, ExpectedCurrent: &fixture.Config.Ref})
	if err != nil || winner2.Ref != successor.Ref {
		t.Fatal("MCP Transport Config successor CAS failed")
	}
	if _, err := repository.EnsureMCPTransportConfigV1(ctx, toolmcp.EnsureMCPTransportConfigRequestV1{Config: fixture.Config}); err == nil {
		t.Fatal("revision 1 replay rolled back revision 2")
	}
}

func TestMCPConnectIntentRepositoryConcurrentSingleWinnerV1(t *testing.T) {
	repository := toolmcp.NewInMemoryMCPConnectIntentRepositoryV1()
	fixture := testkit.MCPConnectV1(testkit.FixedTime, toolcontract.MCPTransportStdioV1)
	var successes atomic.Int64
	var failures atomic.Int64
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			winner, err := repository.EnsureMCPConnectIntentV1(context.Background(), toolmcp.EnsureMCPConnectIntentRequestV1{Intent: fixture.Intent})
			if err != nil || winner.Ref != fixture.Intent.Ref {
				failures.Add(1)
				return
			}
			successes.Add(1)
		}()
	}
	wg.Wait()
	if failures.Load() != 0 || successes.Load() != 64 {
		t.Fatalf("same canonical concurrent MCP Connect Intent diverged: success=%d failure=%d", successes.Load(), failures.Load())
	}
	inspected, err := repository.InspectMCPConnectIntentV1(context.Background(), fixture.Intent.Ref)
	if err != nil || inspected.Ref != fixture.Intent.Ref {
		t.Fatal("MCP Connect Intent lost-reply Inspect failed")
	}
}

func TestMCPConnectRepositoriesFailClosedV1(t *testing.T) {
	fixture := testkit.MCPConnectV1(testkit.FixedTime, toolcontract.MCPTransportStdioV1)
	var nilConfigs *toolmcp.InMemoryMCPTransportConfigRepositoryV1
	if _, err := nilConfigs.EnsureMCPTransportConfigV1(context.Background(), toolmcp.EnsureMCPTransportConfigRequestV1{Config: fixture.Config}); err == nil {
		t.Fatal("typed-nil MCP Transport Config repository was admitted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := toolmcp.NewInMemoryMCPConnectIntentRepositoryV1().EnsureMCPConnectIntentV1(ctx, toolmcp.EnsureMCPConnectIntentRequestV1{Intent: fixture.Intent}); err != context.Canceled {
		t.Fatalf("canceled MCP Connect Intent request did not preserve sentinel: %v", err)
	}
	bad := fixture.Intent
	bad.Ref.Digest = core.DigestBytes([]byte("drift"))
	if _, err := toolmcp.NewInMemoryMCPConnectIntentRepositoryV1().EnsureMCPConnectIntentV1(context.Background(), toolmcp.EnsureMCPConnectIntentRequestV1{Intent: bad}); err == nil {
		t.Fatal("same-ID digest drift was admitted")
	}
}
