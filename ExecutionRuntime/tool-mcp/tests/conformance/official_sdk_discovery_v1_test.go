package conformance_test

import (
	"context"
	"testing"
	"time"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	toolmcp "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestConformanceOfficialGoSDKDiscoveryOverInMemoryTransport(t *testing.T) {
	ctx := context.Background()
	server := officialmcp.NewServer(&officialmcp.Implementation{Name: "praxis-conformance-server", Version: "1.0.0"}, nil)
	server.AddTool(&officialmcp.Tool{Name: "weather.lookup", Description: "Lookup weather", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}}}, nil)
	server.AddResource(&officialmcp.Resource{URI: "file:///praxis/conformance", Name: "conformance", MIMEType: "text/plain"}, nil)
	server.AddPrompt(&officialmcp.Prompt{Name: "summarize", Description: "Summarize a resource"}, nil)

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

	discovery, err := toolmcp.NewOfficialSDKDiscoveryV1(clientSession, func() time.Time { return testkit.FixedTime }, toolmcp.DefaultOfficialSDKDiscoveryLimitsV1())
	if err != nil {
		t.Fatal(err)
	}
	serverDescriptor, connection := testkit.MCPServer(), testkit.MCPConnection()
	snapshot, err := discovery.DiscoverV1(ctx, toolmcp.OfficialSDKDiscoveryRequestV1{
		Server:                   toolcontract.ObjectRef{ID: serverDescriptor.ID, Revision: serverDescriptor.Revision, Digest: serverDescriptor.Digest},
		Connection:               connection,
		SnapshotRevision:         1,
		Conformance:              "mcp/official-go-sdk-v1",
		RequestedExpiresUnixNano: testkit.FixedTime.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Tools) != 1 || snapshot.Tools[0].Name != "weather.lookup" || len(snapshot.Resources) != 1 || snapshot.Resources[0].URI != "file:///praxis/conformance" || len(snapshot.Prompts) != 1 || snapshot.Prompts[0].Name != "summarize" {
		t.Fatalf("official SDK discovery lost protocol objects: %+v", snapshot)
	}
	if snapshot.ProtocolVersion != toolcontract.MCPStableProtocolVersion {
		t.Fatalf("negotiated protocol drifted: %q", snapshot.ProtocolVersion)
	}
}
