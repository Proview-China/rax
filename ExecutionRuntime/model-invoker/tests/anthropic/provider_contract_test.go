package anthropic_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/testkit/providercontract"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/anthropic"
)

func TestAnthropicProviderContract(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		writer.Header().Set("Request-Id", "req_contract_anthropic")
		if body["stream"] == true {
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = writer.Write(mustRead(t, "testdata/stream-tool-thinking.sse"))
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(mustRead(t, "testdata/message-text.json"))
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "contract-test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := modelinvoker.Request{
		Provider: provider.ProviderID,
		Protocol: modelinvoker.ProtocolMessages,
		Endpoint: server.URL,
		Model:    "contract-model",
		Input:    []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
		Budget:   modelinvoker.Budget{MaxOutputTokens: 64},
	}
	providercontract.Run(t, providercontract.Case{
		Provider:            adapter,
		UnsupportedProtocol: modelinvoker.ProtocolResponses,
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
