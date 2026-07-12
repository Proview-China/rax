package minimax_test

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
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/minimax"
)

func request(endpoint, model string, protocol modelinvoker.Protocol) modelinvoker.Request {
	return modelinvoker.Request{
		Provider: provider.ProviderID, Protocol: protocol, Endpoint: endpoint, Model: model,
		Input:  []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
		Budget: modelinvoker.Budget{MaxOutputTokens: 256},
	}
}

func TestMessagesThinkingToolContinuationUsesDocumentedWireShape(t *testing.T) {
	type capture struct {
		path, apiKey, auth string
		body               map[string]any
	}
	captured := make(chan capture, 2)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured <- capture{path: r.URL.Path, apiKey: r.Header.Get("x-api-key"), auth: r.Header.Get("Authorization"), body: body}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("request-id", "minimax-message-request")
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(w, `{"id":"msg-1","type":"message","role":"assistant","model":"MiniMax-M3","content":[{"type":"thinking","thinking":"need weather","signature":"signed-minimax"},{"type":"tool_use","id":"call-weather","name":"weather","input":{"city":"Beijing"}}],"stop_reason":"tool_use","usage":{"input_tokens":5,"output_tokens":7}}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"msg-2","type":"message","role":"assistant","model":"MiniMax-M3","content":[{"type":"text","text":"sunny"}],"stop_reason":"end_turn","usage":{"input_tokens":9,"output_tokens":2}}`)
	}))
	defer server.Close()

	adapter, err := provider.New(provider.Config{APIKey: "payg-secret", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := server.URL + "/anthropic"
	r := request(endpoint, "MiniMax-M3", modelinvoker.ProtocolMessages)
	r.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
	r.Tools = []modelinvoker.Tool{{Name: "weather", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)}}
	first, err := adapter.Invoke(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	firstNative := <-captured
	if firstNative.path != "/anthropic/v1/messages" || firstNative.apiKey != "payg-secret" || firstNative.auth != "" {
		t.Fatalf("first wire=%#v", firstNative)
	}
	thinking, _ := firstNative.body["thinking"].(map[string]any)
	if thinking["type"] != "adaptive" {
		t.Fatalf("thinking=%#v body=%#v", thinking, firstNative.body)
	}
	if _, exists := firstNative.body["output_config"]; exists {
		t.Fatalf("unsupported output_config was sent: %#v", firstNative.body)
	}
	if first.State == nil || first.State.Kind != modelinvoker.StateProviderContinuation || first.Text() != "" {
		t.Fatalf("first response=%#v", first)
	}
	if len(first.Output) != 2 || first.Output[0].ReasoningSummary != "need weather" || first.Output[1].FunctionCall == nil {
		t.Fatalf("first output=%#v", first.Output)
	}

	secondRequest := request(endpoint, "MiniMax-M3", modelinvoker.ProtocolMessages)
	secondRequest.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
	secondRequest.State = first.State
	secondRequest.Input = []modelinvoker.InputItem{modelinvoker.FunctionResultInput("call-weather", `{"temperature":22}`, false)}
	second, err := adapter.Invoke(context.Background(), secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	secondNative := <-captured
	if second.Text() != "sunny" || secondNative.path != "/anthropic/v1/messages" {
		t.Fatalf("second=%#v wire=%#v", second, secondNative)
	}
	messages, _ := secondNative.body["messages"].([]any)
	encoded, _ := json.Marshal(messages)
	if strings.Contains(string(encoded), `"caller"`) || !strings.Contains(string(encoded), `"signature":"signed-minimax"`) {
		t.Fatalf("continuation wire=%s", encoded)
	}
}

func TestChatReasoningSplitAndCumulativeStreamAreNormalized(t *testing.T) {
	type capture struct {
		path, auth string
		body       map[string]any
	}
	captured := make(chan capture, 2)
	var streamMode atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured <- capture{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: body}
		if !streamMode.Load() {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"chat-1","model":"MiniMax-M3","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","reasoning_content":"private thought","reasoning_details":[{"type":"reasoning.text","text":"private thought"}],"content":"answer"}}],"usage":{"prompt_tokens":2,"completion_tokens":3,"completion_tokens_details":{"reasoning_tokens":1},"total_tokens":5},"base_resp":{"status_code":0,"status_msg":"success"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, event := range []string{
			`{"id":"chat-stream","model":"MiniMax-M3","choices":[{"index":0,"delta":{"reasoning_details":[{"type":"reasoning.text","text":"think"}]},"finish_reason":null}]}`,
			`{"id":"chat-stream","model":"MiniMax-M3","choices":[{"index":0,"delta":{"reasoning_details":[{"type":"reasoning.text","text":"thinking"}],"content":"Hel"},"finish_reason":null}]}`,
			`{"id":"chat-stream","model":"MiniMax-M3","choices":[{"index":0,"delta":{"reasoning_details":[{"type":"reasoning.text","text":"thinking"}],"content":"Hello"},"finish_reason":"stop"}]}`,
			`{"id":"chat-stream","model":"MiniMax-M3","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	adapter, err := provider.New(provider.Config{APIKey: "chat-secret", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := server.URL + "/v1"
	r := request(endpoint, "MiniMax-M3", modelinvoker.ProtocolChatCompletions)
	r.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortMedium}
	response, err := adapter.Invoke(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	native := <-captured
	thinking, _ := native.body["thinking"].(map[string]any)
	if native.path != "/v1/chat/completions" || native.auth != "Bearer chat-secret" || native.body["reasoning_split"] != true || thinking["type"] != "adaptive" {
		t.Fatalf("native=%#v", native)
	}
	if response.Text() != "answer" || response.Usage.ReasoningTokens != 1 || reasoningOutput(response) != "private thought" {
		t.Fatalf("response=%#v", response)
	}

	streamMode.Store(true)
	stream, err := adapter.Stream(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var textDeltas, reasoningDeltas strings.Builder
	var terminal *modelinvoker.Response
	for stream.Next() {
		event := stream.Event()
		textDeltas.WriteString(event.TextDelta)
		reasoningDeltas.WriteString(event.ReasoningDelta)
		if event.Type == modelinvoker.StreamEventResponseCompleted {
			terminal = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	_ = <-captured
	if textDeltas.String() != "Hello" || reasoningDeltas.String() != "thinking" || terminal == nil || terminal.Text() != "Hello" || reasoningOutput(*terminal) != "thinking" {
		t.Fatalf("text=%q reasoning=%q terminal=%#v", textDeltas.String(), reasoningDeltas.String(), terminal)
	}
}

func TestResponsesIsTypedAndStateless(t *testing.T) {
	captured := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer responses-secret" {
			t.Errorf("path/auth = %s/%s", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp-1","object":"response","created_at":1770000000,"model":"MiniMax-M3","status":"completed","output":[{"id":"resp-1-rs","type":"reasoning","status":"completed","summary":[{"type":"summary_text","text":"response thought"}],"content":[{"type":"reasoning_text","text":"response thought"}]},{"id":"resp-1-msg","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"response answer","annotations":[]}]}],"output_text":"response answer","usage":{"input_tokens":3,"input_tokens_details":{"cached_tokens":0},"output_tokens":4,"output_tokens_details":{"reasoning_tokens":2},"total_tokens":7},"parallel_tool_calls":true,"store":false,"truncation":"disabled"}`)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "responses-secret", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := server.URL + "/v1"
	r := request(endpoint, "MiniMax-M3", modelinvoker.ProtocolResponses)
	r.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortMinimal}
	response, err := adapter.Invoke(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	body := <-captured
	reasoning, _ := body["reasoning"].(map[string]any)
	if reasoning["effort"] != "minimal" || response.State != nil || response.Text() != "response answer" || reasoningOutput(response) != "response thought" || response.Usage.ReasoningTokens != 2 {
		t.Fatalf("body=%#v response=%#v", body, response)
	}
}

func TestSelectionOfferingAndErrorBoundaries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":"1008","message":"balance"}}`)
	}))
	defer server.Close()
	if _, err := provider.New(provider.Config{APIKey: "sk-cp-token-plan", BaseURL: server.URL, HTTPClient: server.Client()}); err == nil {
		t.Fatal("Token Plan key accepted by pay-as-you-go adapter")
	}
	adapter, err := provider.New(provider.Config{APIKey: "payg", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"MiniMax-M3-latest", "abab6.5-chat", "speech-2.8-hd", "unknown"} {
		if _, err := adapter.Invoke(context.Background(), request(server.URL+"/v1", model, modelinvoker.ProtocolChatCompletions)); err == nil {
			t.Errorf("unsupported model %q accepted", model)
		}
	}
	r := request(server.URL+"/v1", "MiniMax-M2.7", modelinvoker.ProtocolChatCompletions)
	r.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortNone}
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("M2.x disabled thinking accepted")
	}
	r = request(server.URL+"/v1", "MiniMax-M2.7", modelinvoker.ProtocolResponses)
	r.Input = append(r.Input, modelinvoker.FunctionResultInput("call", "result", false))
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("M2.x reasoning continuation without preserved history accepted")
	}
	r = request(server.URL+"/anthropic", "MiniMax-M3", modelinvoker.ProtocolMessages)
	r.Endpoint = server.URL + "/v1"
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("cross-protocol endpoint accepted")
	}
	if calls.Load() != 0 {
		t.Fatalf("rejected selections made %d HTTP calls", calls.Load())
	}

	_, err = adapter.Invoke(context.Background(), request(server.URL+"/v1", "MiniMax-M3", modelinvoker.ProtocolChatCompletions))
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorBilling {
		t.Fatalf("kind=%q err=%v", modelinvoker.ErrorKindOf(err), err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls=%d, want 1", calls.Load())
	}
}

func TestConfigurationFormattingAndModelIdentity(t *testing.T) {
	if _, err := provider.New(provider.Config{}); err == nil {
		t.Fatal("missing key accepted")
	}
	if _, err := provider.New(provider.Config{APIKey: "secret", BaseURL: "http://example.com"}); err == nil {
		t.Fatal("remote plain HTTP accepted")
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chat","model":"MiniMax-M2.7","choices":[{"index":0,"finish_reason":"stop","message":{"content":"mapped"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2},"base_resp":{"status_code":0}}`)
	}))
	defer server.Close()
	adapter, err = provider.New(provider.Config{APIKey: "secret", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Invoke(context.Background(), request(server.URL+"/v1", "MiniMax-M3", modelinvoker.ProtocolChatCompletions))
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping {
		t.Fatalf("silent model mapping kind=%q err=%v", modelinvoker.ErrorKindOf(err), err)
	}
}

func reasoningOutput(response modelinvoker.Response) string {
	var builder strings.Builder
	for _, item := range response.Output {
		if item.Type == modelinvoker.OutputItemReasoningSummary {
			builder.WriteString(item.ReasoningSummary)
		}
	}
	return builder.String()
}
