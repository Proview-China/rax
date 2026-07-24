package conformance_test

import (
	"encoding/json"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	toolmcp "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpProtocolVersion20250618 = "2025-06-18"

func TestConformanceMCP20250618OfficialTypesInitialize(t *testing.T) {
	officialParams := officialmcp.InitializeParams{
		ProtocolVersion: mcpProtocolVersion20250618,
		Capabilities:    &officialmcp.ClientCapabilities{},
		ClientInfo:      &officialmcp.Implementation{Name: "praxis-conformance-client", Version: "1.0.0"},
	}
	params, err := json.Marshal(officialParams)
	if err != nil {
		t.Fatal(err)
	}
	requestPayload, err := toolmcp.EncodeMessage(toolmcp.Message{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"initialize-2025-06-18"`),
		Method:  "initialize",
		Params:  params,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := toolmcp.DecodeMessage(requestPayload)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := toolmcp.DecodeInitializeRequest(request)
	if err != nil || decoded.ProtocolVersion != mcpProtocolVersion20250618 {
		t.Fatalf("official initialize request is incompatible: decoded=%#v err=%v", decoded, err)
	}
	negotiated, err := toolmcp.NegotiateProtocol(decoded.ProtocolVersion, []string{contract.MCPStableProtocolVersion, mcpProtocolVersion20250618})
	if err != nil || negotiated != mcpProtocolVersion20250618 {
		t.Fatalf("2025-06-18 downgrade negotiation failed: version=%q err=%v", negotiated, err)
	}
	response, err := toolmcp.EncodeInitializeResult(request.ID, toolmcp.InitializeResult{
		ProtocolVersion: negotiated,
		Capabilities:    toolmcp.ServerCapabilities{Tools: map[string]json.RawMessage{}},
		ServerInfo:      toolmcp.Implementation{Name: "praxis-conformance-server", Version: "1.0.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	responsePayload, err := toolmcp.EncodeMessage(response)
	if err != nil {
		t.Fatal(err)
	}
	decodedResponse, err := toolmcp.DecodeMessage(responsePayload)
	if err != nil {
		t.Fatal(err)
	}
	var officialResult officialmcp.InitializeResult
	if err = json.Unmarshal(decodedResponse.Result, &officialResult); err != nil {
		t.Fatal(err)
	}
	if officialResult.ProtocolVersion != mcpProtocolVersion20250618 || officialResult.ServerInfo == nil || officialResult.Capabilities == nil {
		t.Fatalf("Praxis initialize result is not consumable by official types: %#v", officialResult)
	}
}

func TestConformanceMCP20250618OfficialTypesListAndCall(t *testing.T) {
	listParams := officialmcp.ListToolsParams{Cursor: "cursor-1"}
	encodedListParams, err := json.Marshal(listParams)
	if err != nil {
		t.Fatal(err)
	}
	listRequest := roundTripMCPMessage20250618(t, toolmcp.Message{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/list",
		Params:  encodedListParams,
	})
	var decodedListParams officialmcp.ListToolsParams
	if err = json.Unmarshal(listRequest.Params, &decodedListParams); err != nil || decodedListParams.Cursor != listParams.Cursor {
		t.Fatalf("official tools/list params drifted: %#v err=%v", decodedListParams, err)
	}

	officialList := officialmcp.ListToolsResult{
		Tools: []*officialmcp.Tool{{
			Name:        "weather.lookup",
			Description: "Lookup weather",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{"type": "string"},
				},
				"required": []any{"city"},
			},
		}},
	}
	listResult, err := json.Marshal(officialList)
	if err != nil {
		t.Fatal(err)
	}
	decodedListResponse := roundTripMCPMessage20250618(t, toolmcp.Message{JSONRPC: "2.0", ID: listRequest.ID, Result: listResult})
	var decodedList officialmcp.ListToolsResult
	if err = json.Unmarshal(decodedListResponse.Result, &decodedList); err != nil || len(decodedList.Tools) != 1 || decodedList.Tools[0] == nil {
		t.Fatalf("official tools/list result drifted: %#v err=%v", decodedList, err)
	}
	schema, err := json.Marshal(decodedList.Tools[0].InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = toolmcp.ValidateToolSchema(schema); err != nil {
		t.Fatalf("2025-06-18 tool schema was rejected: %v", err)
	}

	callParams, err := json.Marshal(officialmcp.CallToolParams{Name: "weather.lookup", Arguments: map[string]any{"city": "Shanghai"}})
	if err != nil {
		t.Fatal(err)
	}
	callRequest := roundTripMCPMessage20250618(t, toolmcp.Message{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"call-1"`),
		Method:  "tools/call",
		Params:  callParams,
	})
	var decodedCall officialmcp.CallToolParamsRaw
	if err = json.Unmarshal(callRequest.Params, &decodedCall); err != nil || decodedCall.Name != "weather.lookup" || string(decodedCall.Arguments) != `{"city":"Shanghai"}` {
		t.Fatalf("official tools/call params drifted: %#v err=%v", decodedCall, err)
	}

	callResult, err := json.Marshal(officialmcp.CallToolResult{Content: []officialmcp.Content{&officialmcp.TextContent{Text: "sunny"}}})
	if err != nil {
		t.Fatal(err)
	}
	decodedCallResponse := roundTripMCPMessage20250618(t, toolmcp.Message{JSONRPC: "2.0", ID: callRequest.ID, Result: callResult})
	var resultShape struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err = json.Unmarshal(decodedCallResponse.Result, &resultShape); err != nil || len(resultShape.Content) != 1 || resultShape.Content[0].Type != "text" || resultShape.Content[0].Text != "sunny" {
		t.Fatalf("official tools/call result drifted: %#v err=%v", resultShape, err)
	}
}

func TestConformanceMCP20250618DoesNotWidenStableChain(t *testing.T) {
	if contract.MCPStableProtocolVersion != "2025-11-25" {
		t.Fatalf("formal stable protocol drifted: %q", contract.MCPStableProtocolVersion)
	}
	if _, err := toolmcp.NegotiateProtocol(mcpProtocolVersion20250618, []string{contract.MCPStableProtocolVersion}); err == nil {
		t.Fatal("newer-only server was silently treated as a 2025-06-18 overlap")
	}
}

func roundTripMCPMessage20250618(t *testing.T, message toolmcp.Message) toolmcp.Message {
	t.Helper()
	payload, err := toolmcp.EncodeMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := toolmcp.DecodeMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}
