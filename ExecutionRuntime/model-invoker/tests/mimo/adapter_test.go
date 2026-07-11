package mimo_test

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
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/mimo"
)

func request(endpoint, model string, protocol modelinvoker.Protocol) modelinvoker.Request {
	return modelinvoker.Request{
		Provider: provider.ProviderID, Protocol: protocol, Endpoint: endpoint, Model: model,
		Input:  []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
		Budget: modelinvoker.Budget{MaxOutputTokens: 256},
	}
}

func TestMessagesUsesBearerThinkingAndLosslessToolContinuation(t *testing.T) {
	type capture struct {
		path, authorization, apiKey, xAPIKey string
		body                                 map[string]any
	}
	captured := make(chan capture, 2)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured <- capture{path: r.URL.Path, authorization: r.Header.Get("Authorization"), apiKey: r.Header.Get("api-key"), xAPIKey: r.Header.Get("x-api-key"), body: body}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("request-id", "mimo-message-request")
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(w, `{"id":"msg-1","type":"message","role":"assistant","model":"mimo-v2.5-pro","content":[{"type":"thinking","thinking":"need weather","signature":"signed-mimo"},{"type":"tool_use","id":"call-weather","name":"weather","input":{"city":"Beijing"}}],"stop_reason":"tool_use","usage":{"input_tokens":5,"output_tokens":7}}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"msg-2","type":"message","role":"assistant","model":"mimo-v2.5-pro","content":[{"type":"text","text":"sunny"}],"stop_reason":"end_turn","usage":{"input_tokens":9,"output_tokens":2}}`)
	}))
	defer server.Close()

	adapter, err := provider.New(provider.Config{APIKey: "sk-payg-secret", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := server.URL + "/anthropic"
	firstRequest := request(endpoint, "mimo-v2.5-pro", modelinvoker.ProtocolMessages)
	firstRequest.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
	firstRequest.ParallelToolCalls = boolPtr(false)
	firstRequest.Tools = []modelinvoker.Tool{{Name: "weather", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)}}
	first, err := adapter.Invoke(context.Background(), firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	native := <-captured
	if native.path != "/anthropic/v1/messages" || native.authorization != "Bearer sk-payg-secret" || native.apiKey != "" || native.xAPIKey != "" {
		t.Fatalf("wire headers/path=%#v", native)
	}
	thinking, _ := native.body["thinking"].(map[string]any)
	toolChoice, _ := native.body["tool_choice"].(map[string]any)
	if thinking["type"] != "enabled" || toolChoice["type"] != "auto" || toolChoice["disable_parallel_tool_use"] != true {
		t.Fatalf("thinking/tool_choice=%#v/%#v body=%#v", thinking, toolChoice, native.body)
	}
	if _, exists := native.body["output_config"]; exists {
		t.Fatalf("unsupported Anthropic output_config was sent: %#v", native.body)
	}
	if first.State == nil || first.State.Kind != modelinvoker.StateProviderContinuation || len(first.Output) != 2 || reasoningOutput(first) != "need weather" {
		t.Fatalf("first response=%#v", first)
	}

	secondRequest := request(endpoint, "mimo-v2.5-pro", modelinvoker.ProtocolMessages)
	secondRequest.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortLow}
	secondRequest.State = first.State
	secondRequest.Input = []modelinvoker.InputItem{modelinvoker.FunctionResultInput("call-weather", `{"temperature":22}`, false)}
	second, err := adapter.Invoke(context.Background(), secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	continuation := <-captured
	messages, _ := continuation.body["messages"].([]any)
	wire, _ := json.Marshal(messages)
	if second.Text() != "sunny" || strings.Contains(string(wire), `"caller"`) || !strings.Contains(string(wire), `"signature":"signed-mimo"`) {
		t.Fatalf("second=%#v wire=%s", second, wire)
	}
}

func TestChatReasoningJSONObjectAndStreamAreNormalized(t *testing.T) {
	type capture struct {
		path, authorization string
		body                map[string]any
	}
	captured := make(chan capture, 2)
	var streamMode atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured <- capture{path: r.URL.Path, authorization: r.Header.Get("Authorization"), body: body}
		if !streamMode.Load() {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"chat-1","model":"mimo-v2.5","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","reasoning_content":"private thought","content":"{\"answer\":\"ok\"}"}}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, event := range []string{
			`{"id":"chat-stream","model":"mimo-v2.5","choices":[{"index":0,"delta":{"reasoning_content":"think"},"finish_reason":null}]}`,
			`{"id":"chat-stream","model":"mimo-v2.5","choices":[{"index":0,"delta":{"reasoning_content":"ing","content":"Hel"},"finish_reason":null}]}`,
			`{"id":"chat-stream","model":"mimo-v2.5","choices":[{"index":0,"delta":{"content":"lo"},"finish_reason":"stop"}]}`,
			`{"id":"chat-stream","model":"mimo-v2.5","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	adapter, err := provider.New(provider.Config{APIKey: "sk-chat-secret", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := server.URL + "/v1"
	r := request(endpoint, "mimo-v2.5", modelinvoker.ProtocolChatCompletions)
	r.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortMedium}
	r.Output.Type = modelinvoker.OutputJSONObject
	response, err := adapter.Invoke(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	native := <-captured
	thinking, _ := native.body["thinking"].(map[string]any)
	format, _ := native.body["response_format"].(map[string]any)
	if native.path != "/v1/chat/completions" || native.authorization != "Bearer sk-chat-secret" || thinking["type"] != "enabled" || format["type"] != "json_object" {
		t.Fatalf("native=%#v", native)
	}
	if response.Text() != `{"answer":"ok"}` || reasoningOutput(response) != "private thought" {
		t.Fatalf("response=%#v", response)
	}

	streamMode.Store(true)
	stream, err := adapter.Stream(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var text, reasoning strings.Builder
	var terminal *modelinvoker.Response
	for stream.Next() {
		event := stream.Event()
		text.WriteString(event.TextDelta)
		reasoning.WriteString(event.ReasoningDelta)
		if event.Type == modelinvoker.StreamEventResponseCompleted {
			terminal = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	_ = <-captured
	if text.String() != "Hello" || reasoning.String() != "thinking" || terminal == nil || terminal.Text() != "Hello" || reasoningOutput(*terminal) != "thinking" {
		t.Fatalf("text=%q reasoning=%q terminal=%#v", text.String(), reasoning.String(), terminal)
	}
}

func TestSelectionOfferingAndEndpointBoundaries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chat","model":"mimo-v2.5-pro","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()

	for _, config := range []provider.Config{
		{APIKey: "tp-token-plan", BaseURL: server.URL, HTTPClient: server.Client()},
		{APIKey: "sk-payg", BaseURL: "https://token-plan-sgp.xiaomimimo.com"},
		{APIKey: "sk-payg", BaseURL: "https://example.com"},
		{APIKey: "sk-payg", BaseURL: "http://api.xiaomimimo.com"},
		{APIKey: "sk-payg", BaseURL: "https://user@api.xiaomimimo.com"},
		{APIKey: "sk-payg", BaseURL: "https://api.xiaomimimo.com?key=value"},
	} {
		if _, err := provider.New(config); err == nil {
			t.Errorf("unsafe or cross-offering config accepted: %#v", config)
		}
	}
	adapter, err := provider.New(provider.Config{APIKey: "sk-payg", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"mimo-v2-pro", "mimo-v2-omni", "mimo-v2-flash", "mimo-v2.5-tts", "unknown"} {
		if _, err := adapter.Invoke(context.Background(), request(server.URL+"/v1", model, modelinvoker.ProtocolChatCompletions)); err == nil {
			t.Errorf("unsupported model %q accepted", model)
		}
	}
	if _, err := adapter.Invoke(context.Background(), request(server.URL+"/v1", "mimo-v2.5-pro", modelinvoker.ProtocolResponses)); err == nil {
		t.Fatal("undocumented Responses binding accepted")
	}
	r := request(server.URL+"/v1", "mimo-v2.5-pro", modelinvoker.ProtocolMessages)
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("cross-protocol endpoint accepted")
	}
	r = request(server.URL+"/v1", "mimo-v2.5-pro", modelinvoker.ProtocolChatCompletions)
	r.Input = append(r.Input, modelinvoker.FunctionResultInput("call", "result", false))
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("thinking Chat tool continuation without reasoning history accepted")
	}
	r = request(server.URL+"/v1", "mimo-v2.5-pro", modelinvoker.ProtocolChatCompletions)
	r.ToolChoice.Mode = modelinvoker.ToolChoiceNone
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("non-auto tool choice accepted")
	}
	r = request(server.URL+"/v1", "mimo-v2.5-pro", modelinvoker.ProtocolChatCompletions)
	r.ParallelToolCalls = boolPtr(true)
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("Chat parallel-tool control accepted")
	}
	r = request(server.URL+"/anthropic", "mimo-v2.5-pro", modelinvoker.ProtocolMessages)
	r.Tools = []modelinvoker.Tool{{Name: "tool", Parameters: json.RawMessage(`{"type":"object"}`), Strict: boolPtr(true)}}
	if _, err := adapter.Invoke(context.Background(), r); err == nil {
		t.Fatal("Messages strict tool schema accepted")
	}
	if calls.Load() != 0 {
		t.Fatalf("rejected selections made %d HTTP calls", calls.Load())
	}
}

func TestDedicatedFinishReasonsAndHTTPFailures(t *testing.T) {
	var mode atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch mode.Load() {
		case 0:
			_, _ = io.WriteString(w, `{"id":"chat","model":"mimo-v2.5-pro","choices":[{"index":0,"finish_reason":"repetition_truncation","message":{"content":"partial"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
		case 1:
			_, _ = io.WriteString(w, `{"id":"msg","type":"message","role":"assistant","model":"mimo-v2.5-pro","content":[{"type":"text","text":"partial"}],"stop_reason":"repetition_truncation","usage":{"input_tokens":1,"output_tokens":1}}`)
		case 2:
			_, _ = io.WriteString(w, `{"id":"msg","type":"message","role":"assistant","model":"mimo-v2.5-pro","content":[],"stop_reason":"content_filter","usage":{"input_tokens":1,"output_tokens":0}}`)
		default:
			w.WriteHeader(int(mode.Load()))
			_, _ = io.WriteString(w, `{"error":{"type":"api_error","message":"private provider detail"}}`)
		}
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "sk-secret", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := adapter.Invoke(context.Background(), request(server.URL+"/v1", "mimo-v2.5-pro", modelinvoker.ProtocolChatCompletions))
	if err != nil || chat.Status != modelinvoker.ResponseStatusIncomplete || chat.StopReason != modelinvoker.StopReasonOther {
		t.Fatalf("chat repetition=%#v err=%v", chat, err)
	}
	mode.Store(1)
	messages, err := adapter.Invoke(context.Background(), request(server.URL+"/anthropic", "mimo-v2.5-pro", modelinvoker.ProtocolMessages))
	if err != nil || messages.Status != modelinvoker.ResponseStatusIncomplete || messages.StopReason != modelinvoker.StopReasonOther {
		t.Fatalf("messages repetition=%#v err=%v", messages, err)
	}
	mode.Store(2)
	filtered, err := adapter.Invoke(context.Background(), request(server.URL+"/anthropic", "mimo-v2.5-pro", modelinvoker.ProtocolMessages))
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorPolicyRejected || filtered.StopReason != modelinvoker.StopReasonContentFilter {
		t.Fatalf("messages filter=%#v kind=%q err=%v", filtered, modelinvoker.ErrorKindOf(err), err)
	}

	for _, test := range []struct {
		status int
		kind   modelinvoker.ErrorKind
	}{{400, modelinvoker.ErrorInvalidRequest}, {401, modelinvoker.ErrorAuthentication}, {402, modelinvoker.ErrorBilling}, {403, modelinvoker.ErrorPermission}, {404, modelinvoker.ErrorPermission}, {421, modelinvoker.ErrorPolicyRejected}, {429, modelinvoker.ErrorRateLimit}, {500, modelinvoker.ErrorProviderUnavailable}, {503, modelinvoker.ErrorProviderUnavailable}} {
		mode.Store(int32(test.status))
		_, err := adapter.Invoke(context.Background(), request(server.URL+"/v1", "mimo-v2.5-pro", modelinvoker.ProtocolChatCompletions))
		if got := modelinvoker.ErrorKindOf(err); got != test.kind {
			t.Errorf("HTTP %d kind=%q err=%v, want %q", test.status, got, err, test.kind)
		}
		if strings.Contains(fmt.Sprint(err), "private provider detail") {
			t.Errorf("HTTP %d leaked raw provider message: %v", test.status, err)
		}
	}
}

func TestConfigurationFormattingModelIdentityAndRedirect(t *testing.T) {
	if _, err := provider.New(provider.Config{}); err == nil {
		t.Fatal("missing key accepted")
	}
	adapter, err := provider.New(provider.Config{APIKey: "sk-secret"})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{fmt.Sprintf("%v", adapter), fmt.Sprintf("%#v", adapter), fmt.Sprintf("%v", provider.Config{APIKey: "sk-secret"})} {
		if strings.Contains(value, "sk-secret") {
			t.Fatalf("format leaked key: %q", value)
		}
	}

	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
	defer target.Close()
	var mismatch atomic.Bool
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mismatch.Load() {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"chat","model":"mimo-v2.5","choices":[{"index":0,"finish_reason":"stop","message":{"content":"mapped"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
			return
		}
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	adapter, err = provider.New(provider.Config{APIKey: "sk-secret", BaseURL: source.URL, HTTPClient: source.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Invoke(context.Background(), request(source.URL+"/v1", "mimo-v2.5-pro", modelinvoker.ProtocolChatCompletions))
	if err == nil || redirected.Load() != 0 {
		t.Fatalf("redirect followed or accepted: redirected=%d err=%v", redirected.Load(), err)
	}
	mismatch.Store(true)
	_, err = adapter.Invoke(context.Background(), request(source.URL+"/v1", "mimo-v2.5-pro", modelinvoker.ProtocolChatCompletions))
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping {
		t.Fatalf("silent model mapping kind=%q err=%v", modelinvoker.ErrorKindOf(err), err)
	}
}

func boolPtr(value bool) *bool { return &value }

func reasoningOutput(response modelinvoker.Response) string {
	var builder strings.Builder
	for _, item := range response.Output {
		if item.Type == modelinvoker.OutputItemReasoningSummary {
			builder.WriteString(item.ReasoningSummary)
		}
	}
	return builder.String()
}
