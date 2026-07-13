package relaycompat_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/relaycompat"
)

const relayModel = "relay-model"

func TestInvokerNormalizesTextAndToolCallsAcrossEveryRelayProtocol(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		protocol   modelinvoker.Protocol
		path       string
		apiVersion string
	}{
		{name: "chat", protocol: modelinvoker.ProtocolChatCompletions, path: "/v1/chat/completions"},
		{name: "responses", protocol: modelinvoker.ProtocolResponses, path: "/v1/responses"},
		{name: "messages", protocol: modelinvoker.ProtocolMessages, path: "/v1/messages"},
		{name: "generate", protocol: modelinvoker.ProtocolGenerateContent, path: "/v1beta/models/relay-model:generateContent", apiVersion: "v1beta"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if request.Method != http.MethodPost || request.URL.Path != test.path || request.URL.RawQuery != "" {
					t.Errorf("request = %s %s?%s, want POST %s", request.Method, request.URL.Path, request.URL.RawQuery, test.path)
				}
				if test.protocol == modelinvoker.ProtocolMessages {
					if request.Header.Get("x-api-key") != "relay-test-key" {
						t.Errorf("Messages x-api-key was not set")
					}
				} else if test.protocol == modelinvoker.ProtocolGenerateContent {
					if request.Header.Get("x-goog-api-key") != "relay-test-key" {
						t.Errorf("GenerateContent x-goog-api-key was not set")
					}
				} else if request.Header.Get("Authorization") != "Bearer relay-test-key" {
					t.Errorf("OpenAI-compatible Authorization was not set")
				}
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("X-Request-Id", "relay-request")
				writer.Header().Set("X-Goog-Request-Id", "relay-request")
				_, _ = writer.Write([]byte(fixture(test.protocol, body["tools"] != nil)))
			}))
			t.Cleanup(server.Close)

			baseURL := server.URL
			if test.protocol == modelinvoker.ProtocolChatCompletions || test.protocol == modelinvoker.ProtocolResponses {
				baseURL += "/v1"
			}
			adapter, err := relaycompat.New(relaycompat.Config{
				APIKey: "relay-test-key", BaseURL: baseURL, Protocol: test.protocol,
				APIVersion: test.apiVersion, AllowedModels: []string{relayModel}, HTTPClient: server.Client(),
			})
			if err != nil {
				t.Fatal(err)
			}
			registry, err := modelinvoker.NewRegistry(adapter)
			if err != nil {
				t.Fatal(err)
			}
			invoker, err := modelinvoker.NewInvoker(registry)
			if err != nil {
				t.Fatal(err)
			}
			endpoint := baseURL
			if test.protocol == modelinvoker.ProtocolGenerateContent {
				endpoint += "/v1beta"
			}
			request := baseRequest(test.protocol, endpoint)
			text, err := invoker.Invoke(context.Background(), request)
			if err != nil {
				t.Fatalf("text Invoke() error = %v", err)
			}
			if text.Text() != "relay-ok" || text.Provider != relaycompat.ProviderID || text.Protocol != test.protocol || text.Model != relayModel || text.Usage.TotalTokens == 0 {
				t.Fatalf("normalized text response = %#v", text)
			}

			request.Tools = []modelinvoker.Tool{{
				Name: "lookup", Description: "Look up a city",
				Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"],"additionalProperties":false}`),
			}}
			request.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceRequired}
			tool, err := invoker.Invoke(context.Background(), request)
			if err != nil {
				t.Fatalf("tool Invoke() error = %v", err)
			}
			functionCalls := tool.FunctionCalls()
			if len(functionCalls) != 1 || functionCalls[0].Name != "lookup" || !json.Valid(functionCalls[0].Arguments) || !strings.Contains(string(functionCalls[0].Arguments), "Rome") {
				t.Fatalf("normalized function calls = %#v", functionCalls)
			}
			if calls.Load() != 2 {
				t.Fatalf("native request count = %d, want 2", calls.Load())
			}
		})
	}
}

func TestRelayConfigFailsClosedOnModelEndpointAndProtocolDrift(t *testing.T) {
	t.Parallel()
	configs := []relaycompat.Config{
		{APIKey: "key", BaseURL: "https://example.com/v1", Protocol: modelinvoker.ProtocolChatCompletions},
		{APIKey: "key", BaseURL: "http://example.com/v1", Protocol: modelinvoker.ProtocolChatCompletions, AllowedModels: []string{"m"}},
		{APIKey: "key", BaseURL: "https://user@example.com/v1", Protocol: modelinvoker.ProtocolChatCompletions, AllowedModels: []string{"m"}},
		{APIKey: "key", BaseURL: "https://example.com/v1", Protocol: modelinvoker.ProtocolGenerateContent, AllowedModels: []string{"m"}},
	}
	for index, config := range configs {
		if _, err := relaycompat.New(config); err == nil {
			t.Fatalf("invalid config %d was accepted", index)
		}
	}

	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	adapter, err := relaycompat.New(relaycompat.Config{
		APIKey: "key", BaseURL: server.URL + "/v1", Protocol: modelinvoker.ProtocolChatCompletions,
		AllowedModels: []string{"allowed"}, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := baseRequest(modelinvoker.ProtocolChatCompletions, server.URL+"/v1")
	request.Model = "forged"
	if _, err := adapter.Invoke(context.Background(), request); err == nil {
		t.Fatal("model outside exact allowlist was accepted")
	}
	request.Model = "allowed"
	request.Protocol = modelinvoker.ProtocolResponses
	if _, err := adapter.Invoke(context.Background(), request); err == nil {
		t.Fatal("protocol drift was accepted")
	}
}

func TestRelayFailureIsClassifiedAndCredentialIsRedacted(t *testing.T) {
	const secret = "relay-secret-that-must-not-leak"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Request-Id", "relay-rate-id")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":{"message":"upstream saturated","type":"relay_busy","code":"busy"}}`))
	}))
	t.Cleanup(server.Close)
	adapter, err := relaycompat.New(relaycompat.Config{
		APIKey: secret, BaseURL: server.URL + "/v1", Protocol: modelinvoker.ProtocolChatCompletions,
		AllowedModels: []string{relayModel}, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Invoke(context.Background(), baseRequest(modelinvoker.ProtocolChatCompletions, server.URL+"/v1"))
	if err == nil {
		t.Fatal("429 error = nil")
	}
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError.Kind != modelinvoker.ErrorRateLimit || invocationError.HTTPStatus != http.StatusTooManyRequests || !invocationError.Retryable || invocationError.RequestID != "relay-rate-id" {
		t.Fatalf("normalized error = %#v", invocationError)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(string(response.RawRequest.Bytes()), secret) || strings.Contains(string(response.RawResponse.Bytes()), secret) || strings.Contains(fmt.Sprintf("%#v", adapter), secret) {
		t.Fatal("relay credential leaked through error, Raw, or formatting")
	}
}

func baseRequest(protocol modelinvoker.Protocol, endpoint string) modelinvoker.Request {
	return modelinvoker.Request{
		Provider: relaycompat.ProviderID, Protocol: protocol, Endpoint: endpoint, Model: relayModel,
		Input:  []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Return relay-ok, or call lookup for Rome when a tool exists.")},
		Budget: modelinvoker.Budget{MaxOutputTokens: 32},
	}
}

func fixture(protocol modelinvoker.Protocol, tool bool) string {
	if tool {
		switch protocol {
		case modelinvoker.ProtocolChatCompletions:
			return `{"id":"chat-tool","object":"chat.completion","created":1,"model":"relay-model","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"city\":\"Rome\"}"}}]}}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`
		case modelinvoker.ProtocolResponses:
			return `{"id":"resp-tool","object":"response","created_at":1,"model":"relay-model","status":"completed","output":[{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"city\":\"Rome\"}","status":"completed"}],"usage":{"input_tokens":5,"output_tokens":3,"total_tokens":8}}`
		case modelinvoker.ProtocolMessages:
			return `{"id":"msg-tool","type":"message","role":"assistant","model":"relay-model","content":[{"type":"tool_use","id":"call_1","name":"lookup","input":{"city":"Rome"},"caller":{"type":"direct"}}],"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":5,"output_tokens":3}}`
		case modelinvoker.ProtocolGenerateContent:
			return `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call_1","name":"lookup","args":{"city":"Rome"}}}]},"finishReason":"STOP","index":0}],"modelVersion":"relay-served","responseId":"gemini-tool","usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3,"totalTokenCount":8}}`
		}
	}
	switch protocol {
	case modelinvoker.ProtocolChatCompletions:
		return `{"id":"chat-text","object":"chat.completion","created":1,"model":"relay-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"relay-ok"}}],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`
	case modelinvoker.ProtocolResponses:
		return `{"id":"resp-text","object":"response","created_at":1,"model":"relay-model","status":"completed","output":[{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"relay-ok","annotations":[]}]}],"usage":{"input_tokens":4,"output_tokens":2,"total_tokens":6}}`
	case modelinvoker.ProtocolMessages:
		return `{"id":"msg-text","type":"message","role":"assistant","model":"relay-model","content":[{"type":"text","text":"relay-ok","citations":null}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":4,"output_tokens":2}}`
	case modelinvoker.ProtocolGenerateContent:
		return `{"candidates":[{"content":{"role":"model","parts":[{"text":"relay-ok"}]},"finishReason":"STOP","index":0}],"modelVersion":"relay-served","responseId":"gemini-text","usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2,"totalTokenCount":6}}`
	default:
		panic(fmt.Sprintf("unsupported protocol %q", protocol))
	}
}
