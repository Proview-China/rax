package gemini_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/testkit/providercontract"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/gemini"
)

func TestGeminiProviderContract(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("X-Goog-Request-Id", "req_contract_gemini")
		writer.Header().Set("X-Request-Id", "req_contract_gemini")
		if strings.Contains(request.URL.Path, "streamGenerateContent") {
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = writer.Write(mustFixture(t, "testdata/stream-tool-thinking.sse"))
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(mustFixture(t, "testdata/response-text.json"))
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "contract-test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := server.URL + "/v1beta"
	request := modelinvoker.Request{
		Provider: provider.ProviderID,
		Protocol: modelinvoker.ProtocolGenerateContent,
		Endpoint: endpoint,
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
		ExpectedEndpoint: endpoint, NativeCalls: calls.Load,
	})
}
