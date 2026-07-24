package mcp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGovernedOfficialSDKCallV1OverStreamableHTTP(t *testing.T) {
	now := time.Now().UTC()
	base := testkit.MCPExecutionV1(now)
	var calls atomic.Uint64
	server := governedTransportCallServerV1(base.Command.SnapshotTool.Name, "streamable-http-ok", &calls)
	handler := officialmcp.NewStreamableHTTPHandler(func(*http.Request) *officialmcp.Server { return server }, nil)
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	config := governedTransportConfigV1(t, base, toolcontract.MCPTransportStreamableHTTPV1, httpServer.URL+"/mcp", nil)
	material := MCPConnectCredentialMaterialV1{
		CredentialFactsDigest: testDigestV1("http-no-credentials"),
		CheckedUnixNano:       now.UnixNano(),
		ExpiresUnixNano:       now.Add(20 * time.Second).UnixNano(),
		Environment:           map[string]string{},
		Headers:               map[string]string{},
	}
	session, _, err := (officialMCPConnectDriverV1{}).Connect(context.Background(), config, material)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	entry := executeGovernedTransportCallV1(t, now, session)
	if calls.Load() != 1 || entry.ProtocolReceipt == nil || !bytes.Contains(entry.ProtocolReceipt.CanonicalResponse, []byte("streamable-http-ok")) {
		t.Fatalf("calls=%d entry=%+v", calls.Load(), entry)
	}
}

func TestGovernedOfficialSDKCallV1OverStdio(t *testing.T) {
	if os.Getenv("PRAXIS_MCP_STDIO_CALL_HELPER") == "1" {
		base := testkit.MCPExecutionV1(time.Now().UTC())
		server := governedTransportCallServerV1(base.Command.SnapshotTool.Name, "stdio-ok", nil)
		if err := server.Run(context.Background(), &officialmcp.StdioTransport{}); err != nil {
			os.Exit(2)
		}
		return
	}

	now := time.Now().UTC()
	base := testkit.MCPExecutionV1(now)
	config := governedTransportConfigV1(t, base, toolcontract.MCPTransportStdioV1, "", []string{"PRAXIS_MCP_STDIO_CALL_HELPER"})
	config.Stdio.Executable = os.Args[0]
	config.Stdio.Arguments = []string{"-test.run=TestGovernedOfficialSDKCallV1OverStdio"}
	config, _ = toolcontract.SealMCPTransportConfigV1(config)
	material := MCPConnectCredentialMaterialV1{
		CredentialFactsDigest: testDigestV1("stdio-call-credentials"),
		CheckedUnixNano:       now.UnixNano(),
		ExpiresUnixNano:       now.Add(20 * time.Second).UnixNano(),
		Environment:           map[string]string{"PRAXIS_MCP_STDIO_CALL_HELPER": "1"},
		Headers:               map[string]string{},
	}
	session, _, err := (officialMCPConnectDriverV1{}).Connect(context.Background(), config, material)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	entry := executeGovernedTransportCallV1(t, now, session)
	if entry.ProtocolReceipt == nil || !bytes.Contains(entry.ProtocolReceipt.CanonicalResponse, []byte("stdio-ok")) {
		t.Fatalf("entry=%+v", entry)
	}
}

func governedTransportCallServerV1(name, response string, calls *atomic.Uint64) *officialmcp.Server {
	server := officialmcp.NewServer(&officialmcp.Implementation{Name: "praxis-transport-call-fixture", Version: "1.0.0"}, nil)
	server.AddTool(&officialmcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, func(context.Context, *officialmcp.CallToolRequest) (*officialmcp.CallToolResult, error) {
		if calls != nil {
			calls.Add(1)
		}
		return &officialmcp.CallToolResult{Content: []officialmcp.Content{&officialmcp.TextContent{Text: response}}}, nil
	})
	return server
}

func governedTransportConfigV1(t *testing.T, fixture testkit.MCPExecutionFixtureV1, kind runtimeports.NamespacedNameV2, endpoint string, placeholders []string) toolcontract.MCPTransportConfigV1 {
	t.Helper()
	config := toolcontract.MCPTransportConfigV1{
		Ref:                      toolcontract.MCPTransportConfigRefV1{Revision: 1},
		Owner:                    fixture.Command.Server.Owner,
		Server:                   fixture.Command.Connection.Server,
		Kind:                     kind,
		ProviderTransport:        fixture.ProviderTransport,
		ArtifactDigest:           testDigestV1("transport-artifact"),
		ConfigDigest:             testDigestV1("transport-config"),
		NetworkScopeDigest:       testDigestV1("transport-network-scope"),
		SandboxRequirementDigest: testDigestV1("transport-sandbox"),
		CreatedUnixNano:          fixture.Now.UnixNano(),
	}
	if kind == toolcontract.MCPTransportStdioV1 {
		config.Stdio = &toolcontract.MCPStdioTransportConfigV1{Executable: os.Args[0], CredentialPlaceholders: append([]string(nil), placeholders...)}
	} else {
		config.StreamableHTTP = &toolcontract.MCPStreamableHTTPTransportConfigV1{Endpoint: endpoint}
	}
	sealed, err := toolcontract.SealMCPTransportConfigV1(config)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func executeGovernedTransportCallV1(t *testing.T, now time.Time, connected OfficialSDKConnectSessionV1) MCPPhysicalExecutionEntryV1 {
	t.Helper()
	callSession, ok := connected.(OfficialSDKCallSessionV1)
	if !ok {
		t.Fatalf("official connected Session %T does not expose tools/call", connected)
	}
	providerSessionID := callSession.ID()
	if providerSessionID == "" {
		// stdio has no protocol session identifier. The Praxis Connection fact
		// still requires a local, bounded session coordinate; it is never sent
		// to or treated as an identity asserted by the Provider.
		providerSessionID = "mcp-stdio-local-session-v1"
	}
	fixture := testkit.MCPExecutionWithProviderSessionV1(now, providerSessionID)
	commands, err := NewInMemoryMCPExecutionCommandRepositoryV1(func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = commands.CreateMCPExecutionCommandV1(context.Background(), fixture.Command); err != nil {
		t.Fatal(err)
	}
	sessions, err := NewInMemoryOfficialSDKCallSessionRepositoryV1(func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sessions.BindInitializedOfficialSDKSessionV1(context.Background(), OfficialSDKCallSessionBindingV1{
		Connection:        fixture.Command.Connection,
		Snapshot:          fixture.Command.Snapshot,
		ProviderTransport: fixture.ProviderTransport,
		Provider:          fixture.Provider,
		CheckedUnixNano:   now.UnixNano(),
		ExpiresUnixNano:   fixture.Command.NotAfterUnixNano,
		Session:           callSession,
	}); err != nil {
		t.Fatal(err)
	}
	entries := NewInMemoryMCPPhysicalExecutionStoreV1()
	executor, err := NewOfficialSDKPhysicalExecutorV1(commands, &fixedAssociationReaderV1{projection: fixture.Association}, sessions, entries, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = executor.ExecuteControlledOperationPhysicalV3(context.Background(), fixture.Authorization); err != nil {
		t.Fatal(err)
	}
	entry, err := executor.InspectMCPPhysicalExecutionV1(context.Background(), fixture.Authorization.StableKeyDigest)
	if err != nil || entry.State != MCPPhysicalExecutionObservedV1 {
		t.Fatalf("entry=%+v err=%v", entry, err)
	}
	return entry
}
