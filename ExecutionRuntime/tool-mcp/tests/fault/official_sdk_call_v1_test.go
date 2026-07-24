package fault_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	toolmcp "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFaultLostMCPProviderReplyNeverRedispatchesV1(t *testing.T) {
	fixture := testkit.MCPExecutionV1(testkit.FixedTime.Add(2 * time.Second))
	ctx := context.Background()
	session := &faultCallSessionV1{initialize: &officialmcp.InitializeResult{ProtocolVersion: toolcontract.MCPStableProtocolVersion, ServerInfo: &officialmcp.Implementation{Name: "fault", Version: "1.0.0"}, Capabilities: &officialmcp.ServerCapabilities{Tools: &officialmcp.ToolCapabilities{}}}, id: fixture.Command.Connection.SessionID, err: errors.New("lost reply")}
	commands, _ := toolmcp.NewInMemoryMCPExecutionCommandRepositoryV1(func() time.Time { return fixture.Now })
	_, _ = commands.CreateMCPExecutionCommandV1(ctx, fixture.Command)
	sessions, _ := toolmcp.NewInMemoryOfficialSDKCallSessionRepositoryV1(func() time.Time { return fixture.Now })
	_, _ = sessions.BindInitializedOfficialSDKSessionV1(ctx, toolmcp.OfficialSDKCallSessionBindingV1{Connection: fixture.Command.Connection, Snapshot: fixture.Command.Snapshot, ProviderTransport: fixture.ProviderTransport, Provider: fixture.Provider, CheckedUnixNano: fixture.Now.UnixNano(), ExpiresUnixNano: fixture.Command.NotAfterUnixNano, Session: session})
	executor, err := toolmcp.NewOfficialSDKPhysicalExecutorV1(commands, faultAssociationReaderV1{fixture.Association}, sessions, toolmcp.NewInMemoryMCPPhysicalExecutionStoreV1(), func() time.Time { return fixture.Now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = executor.ExecuteControlledOperationPhysicalV3(ctx, fixture.Authorization); err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost reply error=%v", err)
	}
	session.err = nil
	if _, err = executor.ExecuteControlledOperationPhysicalV3(ctx, fixture.Authorization); err != nil {
		t.Fatal(err)
	}
	entry, err := executor.InspectMCPPhysicalExecutionV1(ctx, fixture.Authorization.StableKeyDigest)
	if err != nil || session.calls.Load() != 1 || entry.State != toolmcp.MCPPhysicalExecutionUnknownV1 || entry.ProtocolReceipt != nil {
		t.Fatalf("calls=%d entry=%+v err=%v", session.calls.Load(), entry, err)
	}
}

type faultCallSessionV1 struct {
	initialize *officialmcp.InitializeResult
	id         string
	err        error
	calls      atomic.Uint64
}

func (s *faultCallSessionV1) InitializeResult() *officialmcp.InitializeResult { return s.initialize }
func (s *faultCallSessionV1) ID() string                                      { return s.id }
func (s *faultCallSessionV1) CallTool(context.Context, *officialmcp.CallToolParams) (*officialmcp.CallToolResult, error) {
	s.calls.Add(1)
	return nil, s.err
}

type faultAssociationReaderV1 struct {
	projection runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1
}

func (r faultAssociationReaderV1) InspectCurrentPreparedDomainCommandAssociationV1(context.Context, runtimeports.PreparedDomainCommandAssociationRefV1) (runtimeports.PreparedDomainCommandAssociationCurrentProjectionV1, error) {
	return r.projection, nil
}
