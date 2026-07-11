package openai_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/testkit/providercontract"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/openai"
)

func TestOpenAIProviderContract(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		writer.Header().Set("X-Request-Id", "req_contract_openai")
		if body["stream"] == true {
			writer.Header().Set("Content-Type", "text/event-stream")
			events := []string{
				`{"type":"response.created","sequence_number":1,"response":{"id":"resp_contract_stream","model":"contract-model","status":"in_progress","output":[]}}`,
				`{"type":"response.output_text.delta","sequence_number":2,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"hello contract"}`,
				`{"type":"response.completed","sequence_number":3,"response":{"id":"resp_contract_stream","model":"contract-model","status":"completed","output":[{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello contract","annotations":[]}]}],"usage":{"input_tokens":2,"output_tokens":2,"total_tokens":4}}}`,
			}
			for _, event := range events {
				_, _ = fmt.Fprintf(writer, "data: %s\n\n", event)
			}
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(publicFixture(t, "testdata/responses-success.json"))
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "contract-test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := modelinvoker.Request{
		Provider: provider.ProviderID,
		Protocol: modelinvoker.ProtocolResponses,
		Endpoint: server.URL,
		Model:    "contract-model",
		Input:    []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
		Budget:   modelinvoker.Budget{MaxOutputTokens: 64},
	}
	providercontract.Run(t, providercontract.Case{
		Provider:            adapter,
		UnsupportedProtocol: modelinvoker.ProtocolMessages,
		Request:             request,
	})
	if calls.Load() != 0 {
		t.Fatalf("provider contract made %d HTTP calls", calls.Load())
	}
	providercontract.RunBehavior(t, providercontract.BehaviorCase{
		Provider: adapter, InvokeRequest: request, StreamRequest: request,
		ExpectedEndpoint: server.URL, NativeCalls: calls.Load,
	})
}
