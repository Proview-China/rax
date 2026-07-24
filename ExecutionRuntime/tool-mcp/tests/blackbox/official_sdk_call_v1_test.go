package blackbox_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	toolmcp "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBlackboxGovernedOfficialSDKToolsCallV1(t *testing.T) {
	fixture := testkit.MCPExecutionV1(testkit.FixedTime.Add(2 * time.Second))
	ctx := context.Background()
	server := officialmcp.NewServer(&officialmcp.Implementation{Name: "praxis-blackbox", Version: "1.0.0"}, nil)
	var calls atomic.Uint64
	server.AddTool(&officialmcp.Tool{Name: fixture.Command.SnapshotTool.Name, InputSchema: map[string]any{"type": "object"}}, func(context.Context, *officialmcp.CallToolRequest) (*officialmcp.CallToolResult, error) {
		calls.Add(1)
		return &officialmcp.CallToolResult{Content: []officialmcp.Content{&officialmcp.TextContent{Text: "blackbox-ok"}}}, nil
	})
	serverTransport, clientTransport := officialmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := officialmcp.NewClient(&officialmcp.Implementation{Name: "praxis-tool-mcp", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	commands, _ := toolmcp.NewInMemoryMCPExecutionCommandRepositoryV1(func() time.Time { return fixture.Now })
	if _, err = commands.CreateMCPExecutionCommandV1(ctx, fixture.Command); err != nil {
		t.Fatal(err)
	}
	sessions, _ := toolmcp.NewInMemoryOfficialSDKCallSessionRepositoryV1(func() time.Time { return fixture.Now })
	if _, err = sessions.BindInitializedOfficialSDKSessionV1(ctx, toolmcp.OfficialSDKCallSessionBindingV1{Connection: fixture.Command.Connection, Snapshot: fixture.Command.Snapshot, ProviderTransport: fixture.ProviderTransport, Provider: fixture.Provider, CheckedUnixNano: fixture.Now.UnixNano(), ExpiresUnixNano: fixture.Command.NotAfterUnixNano, Session: clientSession}); err != nil {
		t.Fatal(err)
	}
	entries := toolmcp.NewInMemoryMCPPhysicalExecutionStoreV1()
	executor, err := toolmcp.NewOfficialSDKPhysicalExecutorV1(commands, blackboxAssociationReaderV1{fixture.Association}, sessions, entries, func() time.Time { return fixture.Now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = executor.ExecuteControlledOperationPhysicalV3(ctx, fixture.Authorization); err != nil {
		t.Fatal(err)
	}
	entry, err := executor.InspectMCPPhysicalExecutionV1(ctx, fixture.Authorization.StableKeyDigest)
	if err != nil || calls.Load() != 1 || entry.State != toolmcp.MCPPhysicalExecutionObservedV1 || entry.ProtocolReceipt == nil {
		t.Fatalf("calls=%d entry=%+v err=%v", calls.Load(), entry, err)
	}
}

type blackboxAssociationReaderV1 struct {
	projection runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1
}

func (r blackboxAssociationReaderV1) InspectCurrentPreparedDomainCommandAssociationV1(context.Context, runtimeports.PreparedDomainCommandAssociationRefV1) (runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	return r.projection, nil
}
