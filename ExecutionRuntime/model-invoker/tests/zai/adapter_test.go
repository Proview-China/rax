package zai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/zai"
)

func request(endpoint, model string) modelinvoker.Request {
	return modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: endpoint, Model: model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}, Budget: modelinvoker.Budget{MaxOutputTokens: 64}}
}

func TestGLM52MapsThinkingEffortRequestIDAndReasoning(t *testing.T) {
	captured := make(chan struct {
		path, auth string
		body       map[string]any
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured <- struct {
			path, auth string
			body       map[string]any
		}{r.URL.Path, r.Header.Get("Authorization"), body}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"zai-1","request_id":"zai-request-1","model":"glm-5.2","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","reasoning_content":"glm thought","content":"glm answer"}}],"usage":{"prompt_tokens":2,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":1},"total_tokens":5}}`)
	}))
	defer server.Close()
	endpoint := server.URL + "/api/paas/v4"
	adapter, err := provider.New(provider.Config{APIKey: "zai-secret", BaseURL: endpoint, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	r := request(endpoint, "glm-5.2")
	r.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortMedium}
	response, err := adapter.Invoke(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	native := <-captured
	thinking, _ := native.body["thinking"].(map[string]any)
	if native.path != "/api/paas/v4/chat/completions" || native.auth != "Bearer zai-secret" || thinking["type"] != "enabled" || thinking["clear_thinking"] != true || native.body["reasoning_effort"] != "high" {
		t.Fatalf("native=%#v", native)
	}
	if response.Provider != provider.ProviderID || response.RequestID != "zai-request-1" || response.Text() != "glm answer" || response.Usage.CacheReadTokens != 1 {
		t.Fatalf("response=%#v", response)
	}
	found := false
	for _, item := range response.Output {
		if item.Type == modelinvoker.OutputItemReasoningSummary && item.ReasoningSummary == "glm thought" {
			found = true
		}
	}
	if !found {
		t.Fatalf("reasoning missing: %#v", response.Output)
	}
}

func TestStreamEnvelopeAndProviderFinishReasons(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, event := range []string{
			`{"id":"zai-stream","request_id":"zai-stream-request","model":"glm-5.2","choices":[{"index":0,"delta":{"reasoning_content":"think"},"finish_reason":null}]}`,
			`{"id":"zai-stream","request_id":"zai-stream-request","model":"glm-5.2","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":"model_context_window_exceeded"}]}`,
			`{"id":"zai-stream","request_id":"zai-stream-request","model":"glm-5.2","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()
	endpoint := server.URL + "/api/paas/v4"
	adapter, err := provider.New(provider.Config{APIKey: "key", BaseURL: endpoint, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := adapter.Stream(context.Background(), request(endpoint, "glm-5.2"))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var terminal *modelinvoker.Response
	for stream.Next() {
		if event := stream.Event(); event.Type == modelinvoker.StreamEventResponseCompleted {
			terminal = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if terminal == nil || terminal.Status != modelinvoker.ResponseStatusIncomplete || terminal.StopReason != modelinvoker.StopReasonOther || terminal.RequestID != "zai-stream-request" || terminal.Text() != "partial" {
		t.Fatalf("terminal=%#v", terminal)
	}
}

func TestPolicyNetworkAndBusinessErrorClassification(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   modelinvoker.ErrorKind
	}{
		{"sensitive finish", 200, `{"id":"x","request_id":"request-sensitive","model":"glm-5.2","choices":[{"index":0,"finish_reason":"sensitive","message":{"content":""}}],"usage":{"prompt_tokens":1,"completion_tokens":0,"total_tokens":1}}`, modelinvoker.ErrorPolicyRejected},
		{"network finish", 200, `{"id":"x","request_id":"request-network","model":"glm-5.2","choices":[{"index":0,"finish_reason":"network_error","message":{"content":""}}],"usage":{"prompt_tokens":1,"completion_tokens":0,"total_tokens":1}}`, modelinvoker.ErrorProviderUnavailable},
		{"balance", 429, `{"error":{"code":"1113","message":"Insufficient balance"}}`, modelinvoker.ErrorBilling},
		{"policy", 400, `{"error":{"code":"1301","message":"unsafe"}}`, modelinvoker.ErrorPolicyRejected},
		{"overloaded", 429, `{"error":{"code":"1305","message":"overloaded"}}`, modelinvoker.ErrorProviderUnavailable},
		{"coding key", 429, `{"error":{"code":"1315","message":"coding package key"}}`, modelinvoker.ErrorBilling},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			endpoint := server.URL + "/api/paas/v4"
			adapter, err := provider.New(provider.Config{APIKey: "key", BaseURL: endpoint, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			_, err = adapter.Invoke(context.Background(), request(endpoint, "glm-5.2"))
			if modelinvoker.ErrorKindOf(err) != test.want {
				t.Fatalf("kind=%q err=%v", modelinvoker.ErrorKindOf(err), err)
			}
		})
	}
}

func TestModelToolContinuationAndOfferingBoundaries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"ok","model":"glm-5.2","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()
	endpoint := server.URL + "/api/paas/v4"
	adapter, err := provider.New(provider.Config{APIKey: "key", BaseURL: endpoint, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"glm-latest", "glm-5v-turbo", "glm-coding", "unknown"} {
		if _, err := adapter.Invoke(context.Background(), request(endpoint, model)); err == nil {
			t.Errorf("unsupported model %q accepted", model)
		}
	}
	r := request(endpoint, "glm-4-32b-0414-128k")
	r.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("reasoning accepted on non-thinking model")
	}
	r = request(endpoint, "glm-5.2")
	r.Input = append(r.Input, modelinvoker.FunctionResultInput("call-1", "result", false))
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("thinking continuation without reasoning history accepted")
	}
	r = request(endpoint, "glm-5.2")
	r.Tools = []modelinvoker.Tool{{Name: "tool", Parameters: json.RawMessage(`{"type":"object"}`)}}
	r.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceRequired}
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("unsupported tool_choice accepted")
	}
	r = request(server.URL+"/api/coding/paas/v4", "glm-5.2")
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("Coding Plan endpoint accepted")
	}
	if calls.Load() != 0 {
		t.Fatalf("rejected selections made %d HTTP calls", calls.Load())
	}
}

func TestConfigAndFormattingNeverLeakKey(t *testing.T) {
	if _, err := provider.New(provider.Config{}); err == nil {
		t.Fatal("missing key accepted")
	}
	if _, err := provider.New(provider.Config{APIKey: "secret", BaseURL: "http://example.com/api/paas/v4"}); err == nil {
		t.Fatal("remote plain HTTP accepted")
	}
	adapter, err := provider.New(provider.Config{APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{fmt.Sprintf("%v", adapter), fmt.Sprintf("%#v", adapter), fmt.Sprintf("%v", provider.Config{APIKey: "secret"})} {
		if strings.Contains(value, "secret") {
			t.Fatalf("format leaked key: %q", value)
		}
	}
}
