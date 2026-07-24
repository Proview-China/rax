package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
)

func TestCodecStrictRoundTrip(t *testing.T) {
	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	message, err := mcp.DecodeMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := mcp.EncodeMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mcp.DecodeMessage(encoded); err != nil {
		t.Fatal(err)
	}
}

func TestCodecRejectsAmbiguousOrExtendedEnvelope(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"jsonrpc":"2.0","jsonrpc":"2.0","id":1,"method":"x"}`),
		[]byte(`{"jsonrpc":"2.0","id":1.5,"method":"x"}`),
		[]byte(`{"jsonrpc":"2.0","id":null,"method":"x"}`),
		[]byte(`{"jsonrpc":"2.0","id":1,"result":{},"error":{"code":-1,"message":"x"}}`),
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"x","unexpected":true}`),
	}
	for _, payload := range cases {
		if _, err := mcp.DecodeMessage(payload); err == nil {
			t.Fatalf("invalid JSON-RPC envelope accepted: %s", payload)
		}
	}
}

func TestInitializeAndStableProtocolNegotiation(t *testing.T) {
	params := mcp.InitializeParams{ProtocolVersion: contract.MCPStableProtocolVersion, ClientInfo: mcp.Implementation{Name: "client", Version: "1.0.0"}}
	raw, _ := json.Marshal(params)
	request := mcp.Message{JSONRPC: "2.0", ID: json.RawMessage(`"init-1"`), Method: "initialize", Params: raw}
	if _, err := mcp.DecodeInitializeRequest(request); err != nil {
		t.Fatal(err)
	}
	version, err := mcp.NegotiateProtocol("2026-07-28", []string{"2025-06-18", contract.MCPStableProtocolVersion})
	if err != nil || version != contract.MCPStableProtocolVersion {
		t.Fatalf("draft request did not cap to stable baseline: %q %v", version, err)
	}
}

func TestLocalTransportAndUnsupportedExternal(t *testing.T) {
	transport, err := mcp.NewLocalTransport(func(_ context.Context, request mcp.Message) (mcp.Message, error) {
		return mcp.Message{JSONRPC: "2.0", ID: request.ID, Result: json.RawMessage(`{"tools":[]}`)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	request := mcp.Message{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/list", Params: json.RawMessage(`{}`)}
	if _, err := transport.RoundTrip(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	transport.Close()
	if _, err := transport.RoundTrip(context.Background(), request); err == nil {
		t.Fatal("closed local transport accepted a request")
	}
	if err := mcp.ConnectExternal(context.Background(), testkit.MCPServer()); !errors.Is(err, contract.ErrExternalEffectUnsupported) && err != contract.ErrExternalEffectUnsupported {
		t.Fatalf("external connection did not return unsupported: %v", err)
	}
}

func FuzzDecodeMessage(f *testing.F) {
	f.Add([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	f.Fuzz(func(t *testing.T, payload []byte) {
		message, err := mcp.DecodeMessage(payload)
		if err == nil {
			if encoded, encodeErr := mcp.EncodeMessage(message); encodeErr != nil || len(encoded) == 0 {
				t.Fatalf("decoded message failed to encode: %v", encodeErr)
			}
		}
	})
}

func BenchmarkDecodeMessage(b *testing.B) {
	payload := []byte(`{"jsonrpc":"2.0","id":"bench-1","method":"tools/call","params":{"name":"example","arguments":{"value":1}}}`)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := mcp.DecodeMessage(payload); err != nil {
			b.Fatal(err)
		}
	}
}
