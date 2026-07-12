package kimi_test

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
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/kimi"
)

func request(endpoint, model string) modelinvoker.Request {
	return modelinvoker.Request{Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: endpoint, Model: model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}, Budget: modelinvoker.Budget{MaxOutputTokens: 64}}
}

func TestK27PreservesReasoningWithoutSendingForbiddenThinkingControl(t *testing.T) {
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
		w.Header().Set("x-request-id", "kimi-request")
		_, _ = io.WriteString(w, `{"id":"kimi-1","model":"kimi-k2.7-code","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","reasoning_content":"kimi thought","content":"kimi answer"}}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "moonshot-secret", BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	r := request(server.URL+"/v1", "kimi-k2.7-code")
	r.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
	response, err := adapter.Invoke(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	native := <-captured
	if native.path != "/v1/chat/completions" || native.auth != "Bearer moonshot-secret" {
		t.Fatalf("native = %#v", native)
	}
	if _, exists := native.body["thinking"]; exists {
		t.Fatalf("K2.7 request sent forbidden thinking control: %#v", native.body)
	}
	if _, exists := native.body["reasoning_effort"]; exists {
		t.Fatalf("K2.7 request sent unsupported reasoning_effort: %#v", native.body)
	}
	if response.Provider != provider.ProviderID || response.Text() != "kimi answer" || response.RequestID != "kimi-request" {
		t.Fatalf("response = %#v", response)
	}
	found := false
	for _, item := range response.Output {
		if item.Type == modelinvoker.OutputItemReasoningSummary && item.ReasoningSummary == "kimi thought" {
			found = true
		}
	}
	if !found {
		t.Fatalf("reasoning missing: %#v", response.Output)
	}
}

func TestK26MapsReasoningSwitchAndRejectsUnrepresentableContinuation(t *testing.T) {
	bodies := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"kimi-2","model":"kimi-k2.6","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "key", BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	r := request(server.URL+"/v1", "kimi-k2.6")
	r.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortNone}
	if _, err := adapter.Invoke(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	thinking, _ := (<-bodies)["thinking"].(map[string]any)
	if thinking["type"] != "disabled" {
		t.Fatalf("thinking = %#v", thinking)
	}
	r = request(server.URL+"/v1", "kimi-k2.6")
	r.Input = append(r.Input, modelinvoker.FunctionResultInput("call-1", "tool", false))
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("thinking tool continuation without reasoning history was accepted")
	}
	if len(bodies) != 0 {
		t.Fatal("rejected continuation reached HTTP")
	}
}

func TestKimiStreamPreservesReasoningBeforeTextAndTerminal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, event := range []string{
			`{"id":"kimi-stream","model":"kimi-k2.7-code","choices":[{"index":0,"delta":{"reasoning_content":"think"},"finish_reason":null}]}`,
			`{"id":"kimi-stream","model":"kimi-k2.7-code","choices":[{"index":0,"delta":{"content":"answer"},"finish_reason":"stop"}]}`,
			`{"id":"kimi-stream","model":"kimi-k2.7-code","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "key", BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := adapter.Stream(context.Background(), request(server.URL+"/v1", "kimi-k2.7-code"))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var order []modelinvoker.StreamEventType
	var terminal *modelinvoker.Response
	var last int64
	for stream.Next() {
		event := stream.Event()
		if event.Sequence <= last {
			t.Fatal("non-monotonic stream")
		}
		last = event.Sequence
		order = append(order, event.Type)
		if event.Type == modelinvoker.StreamEventResponseCompleted {
			terminal = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	reasoningIndex, textIndex := -1, -1
	for i, kind := range order {
		if kind == modelinvoker.StreamEventReasoningDelta && reasoningIndex < 0 {
			reasoningIndex = i
		}
		if kind == modelinvoker.StreamEventTextDelta && textIndex < 0 {
			textIndex = i
		}
	}
	if reasoningIndex < 0 || textIndex <= reasoningIndex || terminal == nil || terminal.Text() != "answer" || terminal.Provider != provider.ProviderID {
		t.Fatalf("order=%v terminal=%#v", order, terminal)
	}
}

func TestModelsOfferingAndQuotaBoundaries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"type":"exceeded_current_quota_error","message":"quota exhausted"}}`)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "key", BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Invoke(context.Background(), request(server.URL+"/v1", "kimi-k2.6"))
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorBilling {
		t.Fatalf("quota error = %v", err)
	}
	for _, model := range []string{"kimi-k2", "kimi-latest", "kimi-for-coding", "kimi-k2-0905-preview", "moonshot-v1-8k-vision-preview"} {
		if _, err := adapter.Invoke(context.Background(), request(server.URL+"/v1", model)); err == nil {
			t.Errorf("retired or wrong-offering model %q accepted", model)
		}
	}
	r := request(server.URL+"/coding/v1", "kimi-k2.6")
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("Kimi Code endpoint accepted by pay-as-you-go adapter")
	}
	if calls.Load() != 1 {
		t.Fatalf("unexpected HTTP calls = %d", calls.Load())
	}
}

func TestConfigAndFormattingNeverLeakKey(t *testing.T) {
	if _, err := provider.New(provider.Config{}); err == nil {
		t.Fatal("missing key accepted")
	}
	if _, err := provider.New(provider.Config{APIKey: "secret", BaseURL: "http://example.com/v1"}); err == nil {
		t.Fatal("remote plain HTTP accepted")
	}
	if _, err := provider.New(provider.Config{APIKey: "secret", BaseURL: "https://example.com/v1"}); err == nil {
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
		_, _ = io.WriteString(writer, `{"id":"mapped","model":"kimi-k2.6","choices":[{"index":0,"finish_reason":"stop","message":{"content":"mapped"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "secret", BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Invoke(context.Background(), request(server.URL+"/v1", "kimi-k2.7-code")); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping {
		t.Fatalf("response model mismatch kind=%q err=%v", modelinvoker.ErrorKindOf(err), err)
	}
}
