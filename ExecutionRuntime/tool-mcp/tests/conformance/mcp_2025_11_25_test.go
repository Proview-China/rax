package conformance_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
)

func TestConformanceMCP20251125InitializeAndNotification(t *testing.T) {
	params, _ := json.Marshal(mcp.InitializeParams{ProtocolVersion: contract.MCPStableProtocolVersion, Capabilities: mcp.ClientCapabilities{}, ClientInfo: mcp.Implementation{Name: "conformance-client", Version: "1.0.0"}})
	request := mcp.Message{JSONRPC: "2.0", ID: json.RawMessage(`"initialize-1"`), Method: "initialize", Params: params}
	decoded, err := mcp.DecodeInitializeRequest(request)
	if err != nil || decoded.ProtocolVersion != contract.MCPStableProtocolVersion {
		t.Fatalf("initialize conformance failed: %v", err)
	}
	result, err := mcp.EncodeInitializeResult(request.ID, mcp.InitializeResult{ProtocolVersion: contract.MCPStableProtocolVersion, Capabilities: mcp.ServerCapabilities{Tools: map[string]json.RawMessage{}}, ServerInfo: mcp.Implementation{Name: "wave1-server", Version: "1.0.0"}})
	if err != nil || result.Error != nil {
		t.Fatalf("initialize result conformance failed: %v", err)
	}
	notification := mcp.Message{JSONRPC: "2.0", Method: "notifications/initialized"}
	if _, err := mcp.EncodeMessage(notification); err != nil {
		t.Fatalf("initialized notification conformance failed: %v", err)
	}
}

func TestConformanceToolSchemaAndNoExternalEffects(t *testing.T) {
	if _, err := mcp.ValidateToolSchema([]byte(`{"type":"object","properties":{"value":{"type":"integer"}},"required":["value"]}`)); err != nil {
		t.Fatal(err)
	}
	if err := mcp.ConnectExternal(context.Background(), testkit.MCPServer()); !errors.Is(err, contract.ErrExternalEffectUnsupported) && err != contract.ErrExternalEffectUnsupported {
		t.Fatalf("real external effect boundary drifted: %v", err)
	}
	if err := mcp.DiscoverExternal(context.Background(), testkit.MCPConnection()); err != contract.ErrExternalEffectUnsupported {
		t.Fatalf("real discovery boundary drifted: %v", err)
	}
	if err := mcp.InvokeExternal(context.Background(), testkit.MCPConnection(), mcp.Message{}); err != contract.ErrExternalEffectUnsupported {
		t.Fatalf("real invocation boundary drifted: %v", err)
	}
}
