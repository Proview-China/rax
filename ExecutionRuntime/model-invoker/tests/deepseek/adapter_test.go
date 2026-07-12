package deepseek_test

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
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/deepseek"
)

func chatRequest(endpoint string) modelinvoker.Request {
	return modelinvoker.Request{
		Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolChatCompletions,
		Endpoint: endpoint, Model: "deepseek-v4-pro",
		Input:     []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
		Budget:    modelinvoker.Budget{MaxOutputTokens: 64},
		Reasoning: &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh},
	}
}

func TestChatPreservesDeepSeekThinkingAndReasoning(t *testing.T) {
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
		w.Header().Set("x-request-id", "deepseek-request")
		_, _ = io.WriteString(w, `{"id":"chat-1","model":"deepseek-v4-pro","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","reasoning_content":"reasoned","content":"answer"}}],"usage":{"prompt_tokens":2,"completion_tokens":3,"completion_tokens_details":{"reasoning_tokens":1},"total_tokens":5}}`)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "deepseek-secret", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Invoke(context.Background(), chatRequest(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	native := <-captured
	thinking, _ := native.body["thinking"].(map[string]any)
	if native.path != "/chat/completions" || native.auth != "Bearer deepseek-secret" || thinking["type"] != "enabled" || native.body["reasoning_effort"] != "high" {
		t.Fatalf("native = %#v", native)
	}
	if response.Provider != provider.ProviderID || response.Text() != "answer" || response.RequestID != "deepseek-request" || response.Usage.ReasoningTokens != 1 {
		t.Fatalf("response = %#v", response)
	}
	found := false
	for _, item := range response.Output {
		if item.Type == modelinvoker.OutputItemReasoningSummary && item.ReasoningSummary == "reasoned" {
			found = true
		}
	}
	if !found {
		t.Fatalf("reasoning output missing: %#v", response.Output)
	}
}

func TestChatStreamPreservesReasoningSequenceAndTerminal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, event := range []string{
			`{"id":"chat-stream","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"think "},"finish_reason":null}]}`,
			`{"id":"chat-stream","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"reasoning_content":"more","content":"ok"},"finish_reason":"stop"}]}`,
			`{"id":"chat-stream","model":"deepseek-v4-pro","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := chatRequest(server.URL)
	request.Stream = true
	stream, err := adapter.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var reasoning strings.Builder
	var terminal *modelinvoker.Response
	var last int64
	for stream.Next() {
		event := stream.Event()
		if event.Sequence <= last {
			t.Fatalf("non-monotonic sequence %d after %d", event.Sequence, last)
		}
		last = event.Sequence
		if event.Type == modelinvoker.StreamEventReasoningDelta {
			reasoning.WriteString(event.ReasoningDelta)
		}
		if event.Type == modelinvoker.StreamEventResponseCompleted {
			terminal = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if reasoning.String() != "think more" || terminal == nil || terminal.Text() != "ok" || terminal.Provider != provider.ProviderID {
		t.Fatalf("reasoning=%q terminal=%#v", reasoning.String(), terminal)
	}
}

func TestMessagesUsesSeparateEndpointAndRejectsSilentModelMapping(t *testing.T) {
	var calls atomic.Int32
	captured := make(chan struct{ path, key string }, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		captured <- struct{ path, key string }{r.URL.Path, r.Header.Get("x-api-key")}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg-1","type":"message","role":"assistant","model":"deepseek-v4-flash","content":[{"type":"text","text":"messages ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":2}}`)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "deepseek-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolMessages, Endpoint: server.URL + "/anthropic", Model: "deepseek-v4-flash", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}, Budget: modelinvoker.Budget{MaxOutputTokens: 32}}
	response, err := adapter.Invoke(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	native := <-captured
	if native.path != "/anthropic/v1/messages" || native.key != "deepseek-key" || response.Text() != "messages ok" {
		t.Fatalf("native=%#v response=%#v", native, response)
	}
	request.Model = "claude-sonnet-4-5"
	if _, err := adapter.Invoke(context.Background(), request); err == nil {
		t.Fatal("Claude alias reached DeepSeek's silent model mapper")
	}
	request.Model = "deepseek-chat"
	if _, err := adapter.Invoke(context.Background(), request); err == nil {
		t.Fatal("deprecated DeepSeek alias accepted")
	}
	if calls.Load() != 1 {
		t.Fatalf("unexpected HTTP calls = %d", calls.Load())
	}
}

func TestConfigErrorAndFormattingNeverLeakKey(t *testing.T) {
	if _, err := provider.New(provider.Config{}); err == nil {
		t.Fatal("missing key accepted")
	}
	if _, err := provider.New(provider.Config{APIKey: "secret", BaseURL: "http://example.com"}); err == nil {
		t.Fatal("insecure remote endpoint accepted")
	}
	if _, err := provider.New(provider.Config{APIKey: "secret", BaseURL: "https://example.com"}); err == nil {
		t.Fatal("arbitrary HTTPS endpoint accepted")
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

func TestResponseModelMismatchIsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"mapped","model":"deepseek-v4-flash","choices":[{"index":0,"finish_reason":"stop","message":{"content":"mapped"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "secret", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Invoke(context.Background(), chatRequest(server.URL)); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping {
		t.Fatalf("response model mismatch kind=%q err=%v", modelinvoker.ErrorKindOf(err), err)
	}
}
