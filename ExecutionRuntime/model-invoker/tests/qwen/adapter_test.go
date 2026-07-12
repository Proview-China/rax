package qwen_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/qwen"
)

func request(endpoint, model string, protocol modelinvoker.Protocol) modelinvoker.Request {
	return modelinvoker.Request{
		Provider: provider.ProviderID, Protocol: protocol, Endpoint: endpoint, Model: model,
		Input:  []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
		Budget: modelinvoker.Budget{MaxOutputTokens: 256},
	}
}

func TestRegionWorkspaceAndCredentialBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		region    provider.Region
		workspace string
		endpoint  string
	}{
		{"beijing", provider.RegionChinaBeijing, "llm-beijing-123", "https://llm-beijing-123.cn-beijing.maas.aliyuncs.com/compatible-mode/v1"},
		{"singapore", provider.RegionSingapore, "llm-singapore-456", "https://llm-singapore-456.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, err := provider.New(provider.Config{APIKey: "sk-ws-payg", Region: tc.region, WorkspaceID: tc.workspace})
			if err != nil {
				t.Fatal(err)
			}
			query := modelinvoker.CapabilityQuery{Protocol: modelinvoker.ProtocolResponses, Endpoint: tc.endpoint, Model: "qwen3.7-max"}
			if _, err := adapter.Capabilities(context.Background(), query); err != nil {
				t.Fatalf("derived endpoint rejected: %v", err)
			}
			query.Model = "qwen3.7-plus"
			if _, err := adapter.Capabilities(context.Background(), query); err != nil {
				t.Fatalf("current recommended qwen3.7-plus rejected: %v", err)
			}
			query.Endpoint = "https://dashscope.aliyuncs.com/compatible-mode/v1"
			if _, err := adapter.Capabilities(context.Background(), query); err == nil {
				t.Fatal("shared or cross-workspace endpoint accepted")
			}
		})
	}

	invalid := []provider.Config{
		{},
		{APIKey: "sk-sp-subscription", Region: provider.RegionChinaBeijing, WorkspaceID: "llm-valid"},
		{APIKey: "other-secret", Region: provider.RegionChinaBeijing, WorkspaceID: "llm-valid"},
		{APIKey: "sk-ws-payg", Region: "us-east-1", WorkspaceID: "llm-valid"},
		{APIKey: "sk-ws-payg", Region: provider.RegionChinaBeijing, WorkspaceID: "Bad/Workspace"},
		{APIKey: "sk-ws-payg", Region: provider.RegionChinaBeijing, WorkspaceID: "llm-valid", BaseURL: "https://coding.dashscope.aliyuncs.com/v1"},
		{APIKey: "sk-ws-payg", Region: provider.RegionChinaBeijing, WorkspaceID: "llm-valid", BaseURL: "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1"},
	}
	for index, config := range invalid {
		if _, err := provider.New(config); err == nil {
			t.Errorf("invalid config %d accepted", index)
		}
	}
}

func TestResponsesTypedStateReasoningAndTools(t *testing.T) {
	type capture struct {
		path string
		auth string
		body map[string]any
	}
	captured := make(chan capture, 2)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured <- capture{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: body}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "qwen-response-request")
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(w, `{"id":"resp-qwen-1","object":"response","created_at":1770000000,"model":"qwen3.7-max","status":"completed","output":[{"id":"reason-1","type":"reasoning","summary":[{"type":"summary_text","text":"qwen thought"}]},{"id":"call-1","type":"function_call","status":"completed","call_id":"weather-1","name":"weather","arguments":"{\"city\":\"Beijing\"}"}],"usage":{"input_tokens":3,"output_tokens":4,"output_tokens_details":{"reasoning_tokens":2},"total_tokens":7}}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"resp-qwen-2","object":"response","created_at":1770000001,"model":"qwen3.7-max","status":"completed","output":[{"id":"msg-2","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"sunny","annotations":[]}]}],"usage":{"input_tokens":5,"output_tokens":1,"total_tokens":6}}`)
	}))
	defer server.Close()

	adapter, err := provider.New(provider.Config{APIKey: "sk-ws-responses-secret", Region: provider.RegionChinaBeijing, WorkspaceID: "llm-test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := server.URL
	r := request(endpoint, "qwen3.7-max", modelinvoker.ProtocolResponses)
	r.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortMedium}
	r.Tools = []modelinvoker.Tool{{Name: "weather", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)}}
	first, err := adapter.Invoke(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	native := <-captured
	reasoning, _ := native.body["reasoning"].(map[string]any)
	if native.path != "/responses" || native.auth != "Bearer sk-ws-responses-secret" || reasoning["effort"] != "medium" {
		t.Fatalf("native=%#v", native)
	}
	if first.State == nil || first.State.Kind != modelinvoker.StateServerContinuation || first.State.ID != "resp-qwen-1" {
		t.Fatalf("state=%#v", first.State)
	}
	if len(first.Output) != 2 || first.Output[1].FunctionCall == nil || first.Output[1].FunctionCall.ID != "weather-1" {
		t.Fatalf("output=%#v", first.Output)
	}

	secondRequest := request(endpoint, "qwen3.7-max", modelinvoker.ProtocolResponses)
	secondRequest.State = first.State
	secondRequest.Input = []modelinvoker.InputItem{modelinvoker.FunctionResultInput("weather-1", `{"temperature":22}`, false)}
	second, err := adapter.Invoke(context.Background(), secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	continued := <-captured
	if continued.body["previous_response_id"] != "resp-qwen-1" || second.Text() != "sunny" {
		t.Fatalf("continued=%#v response=%#v", continued.body, second)
	}
}

func TestChatThinkingJSONObjectAndStreamingUsage(t *testing.T) {
	type capture struct {
		body map[string]any
	}
	captured := make(chan capture, 2)
	var streaming atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured <- capture{body: body}
		if !streaming.Load() {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"chat-qwen-1","object":"chat.completion","created":1770000000,"model":"qwen3.6-flash","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","reasoning_content":"qwen thinks","content":"{\"answer\":\"ok\"}"}}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, event := range []string{
			`{"id":"chat-qwen-stream","model":"qwen3.6-flash","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"think"},"finish_reason":null}]}`,
			`{"id":"chat-qwen-stream","model":"qwen3.6-flash","choices":[{"index":0,"delta":{"reasoning_content":" more","content":"hello"},"finish_reason":null}]}`,
			`{"id":"chat-qwen-stream","model":"qwen3.6-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`{"id":"chat-qwen-stream","model":"qwen3.6-flash","choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`,
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	adapter, err := provider.New(provider.Config{APIKey: "sk-old-payg", Region: provider.RegionSingapore, WorkspaceID: "llm-test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	r := request(server.URL, "qwen3.6-flash", modelinvoker.ProtocolChatCompletions)
	r.Input[0] = modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply as a JSON object")
	r.Output.Type = modelinvoker.OutputJSONObject
	budget := int64(512)
	r.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh, BudgetTokens: &budget}
	response, err := adapter.Invoke(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	native := <-captured
	if native.body["enable_thinking"] != true || native.body["thinking_budget"] != float64(512) {
		t.Fatalf("body=%#v", native.body)
	}
	if _, exists := native.body["reasoning_effort"]; exists {
		t.Fatalf("Qwen Chat received non-Qwen reasoning_effort: %#v", native.body)
	}
	if response.Text() != `{"answer":"ok"}` || reasoningText(response) != "qwen thinks" {
		t.Fatalf("response=%#v", response)
	}

	streaming.Store(true)
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
	if text.String() != "hello" || reasoning.String() != "think more" || terminal == nil || terminal.Usage.TotalTokens != 5 {
		t.Fatalf("text=%q reasoning=%q terminal=%#v", text.String(), reasoning.String(), terminal)
	}
}

func TestSelectionSilentOmissionAndErrorBoundaries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"code":"Arrearage","message":"secret billing detail"}}`)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "sk-ws-boundary-secret", Region: provider.RegionChinaBeijing, WorkspaceID: "llm-test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}

	bad := []modelinvoker.Request{
		request(server.URL, "qwen3.7-max-2026-06-08", modelinvoker.ProtocolResponses),
		request(server.URL, "qwen-vl-max", modelinvoker.ProtocolChatCompletions),
		request(server.URL+"/wrong", "qwen3.7-max", modelinvoker.ProtocolResponses),
	}
	metadata := request(server.URL, "qwen3.7-max", modelinvoker.ProtocolResponses)
	metadata.Metadata = modelinvoker.Metadata{"ignored": "value"}
	bad = append(bad, metadata)
	structured := request(server.URL, "qwen3.7-max", modelinvoker.ProtocolResponses)
	structured.Output.Type = modelinvoker.OutputJSONObject
	bad = append(bad, structured)
	low := request(server.URL, "qwen3.7-max", modelinvoker.ProtocolResponses)
	low.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortLow}
	bad = append(bad, low)
	jsonWithoutPrompt := request(server.URL, "qwen3.6-flash", modelinvoker.ProtocolChatCompletions)
	jsonWithoutPrompt.Output.Type = modelinvoker.OutputJSONObject
	bad = append(bad, jsonWithoutPrompt)
	continuation := request(server.URL, "qwen3.6-flash", modelinvoker.ProtocolChatCompletions)
	continuation.Input = []modelinvoker.InputItem{modelinvoker.FunctionResultInput("call", "result", false)}
	bad = append(bad, continuation)
	for index, r := range bad {
		if _, err := adapter.Invoke(context.Background(), r); err == nil {
			t.Errorf("invalid request %d accepted", index)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("preflight rejections made %d HTTP calls", calls.Load())
	}

	_, err = adapter.Invoke(context.Background(), request(server.URL, "qwen3.7-max", modelinvoker.ProtocolResponses))
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorBilling {
		t.Fatalf("kind=%q err=%v", modelinvoker.ErrorKindOf(err), err)
	}
	if strings.Contains(fmt.Sprint(err), "secret billing detail") || strings.Contains(fmt.Sprint(err), "sk-ws-boundary-secret") {
		t.Fatalf("error leaked sensitive provider data: %v", err)
	}
}

func TestConfigurationFormattingAndModelMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp-mismatch","object":"response","created_at":1770000000,"model":"qwen3-max","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()
	config := provider.Config{APIKey: "sk-ws-format-secret", Region: provider.RegionChinaBeijing, WorkspaceID: "llm-test", BaseURL: server.URL, HTTPClient: server.Client()}
	if strings.Contains(fmt.Sprintf("%v %#v", config, config), "format-secret") {
		t.Fatal("config formatting leaked API key")
	}
	adapter, err := provider.New(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprintf("%v %#v", adapter, adapter), "format-secret") {
		t.Fatal("adapter formatting leaked API key")
	}
	_, err = adapter.Invoke(context.Background(), request(server.URL, "qwen3.7-max", modelinvoker.ProtocolResponses))
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping {
		t.Fatalf("kind=%q err=%v", modelinvoker.ErrorKindOf(err), err)
	}
}

func TestFailureMatrixRedirectCancellationAndBodyLimit(t *testing.T) {
	tests := []struct {
		status int
		code   string
		kind   modelinvoker.ErrorKind
		retry  bool
	}{
		{http.StatusUnauthorized, "InvalidApiKey", modelinvoker.ErrorAuthentication, false},
		{http.StatusForbidden, "Model.AccessDenied", modelinvoker.ErrorPermission, false},
		{http.StatusNotFound, "ModelNotFound", modelinvoker.ErrorMapping, false},
		{http.StatusTooManyRequests, "Throttling.RateQuota", modelinvoker.ErrorRateLimit, true},
		{http.StatusTooManyRequests, "insufficient_quota", modelinvoker.ErrorRateLimit, true},
		{http.StatusBadRequest, "AllocationQuota.FreeTierOnly", modelinvoker.ErrorBilling, false},
		{http.StatusServiceUnavailable, "ServiceUnavailable", modelinvoker.ErrorProviderUnavailable, true},
	}
	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprintf(w, `{"error":{"code":%q,"message":"provider detail"}}`, tc.code)
			}))
			defer server.Close()
			adapter, err := provider.New(provider.Config{APIKey: "sk-ws-error-secret", Region: provider.RegionChinaBeijing, WorkspaceID: "llm-test", BaseURL: server.URL, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			_, err = adapter.Invoke(context.Background(), request(server.URL, "qwen3.7-max", modelinvoker.ProtocolResponses))
			var invocationError *modelinvoker.Error
			if !errors.As(err, &invocationError) || invocationError.Kind != tc.kind || invocationError.Retryable != tc.retry || invocationError.Code != tc.code {
				t.Fatalf("error=%#v", invocationError)
			}
		})
	}

	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	adapter, err := provider.New(provider.Config{APIKey: "sk-ws-redirect-secret", Region: provider.RegionChinaBeijing, WorkspaceID: "llm-test", BaseURL: source.URL, HTTPClient: source.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Invoke(context.Background(), request(source.URL, "qwen3.7-max", modelinvoker.ProtocolResponses)); err == nil || redirected.Load() != 0 {
		t.Fatalf("redirect followed or accepted: redirected=%d err=%v", redirected.Load(), err)
	}

	var called atomic.Int32
	cancelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called.Add(1) }))
	defer cancelServer.Close()
	adapter, err = provider.New(provider.Config{APIKey: "sk-ws-cancel-secret", Region: provider.RegionChinaBeijing, WorkspaceID: "llm-test", BaseURL: cancelServer.URL, HTTPClient: cancelServer.Client()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = adapter.Invoke(ctx, request(cancelServer.URL, "qwen3.7-max", modelinvoker.ProtocolResponses))
	if !errors.Is(err, context.Canceled) || called.Load() != 0 {
		t.Fatalf("cancel err=%v calls=%d", err, called.Load())
	}

	largeClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(io.LimitReader(repeatingReader('x'), 9<<20)), ContentLength: 9 << 20, Request: r,
		}, nil
	})}
	adapter, err = provider.New(provider.Config{APIKey: "sk-ws-limit-secret", Region: provider.RegionChinaBeijing, WorkspaceID: "llm-test", BaseURL: "http://127.0.0.1:1", HTTPClient: largeClient})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Invoke(context.Background(), request("http://127.0.0.1:1", "qwen3.7-max", modelinvoker.ProtocolResponses))
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError.Code != adaptercore.ResponseBodyLimitErrorCode || invocationError.Retryable {
		t.Fatalf("body-limit error=%#v", invocationError)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type repeatingReader byte

func (r repeatingReader) Read(destination []byte) (int, error) {
	for index := range destination {
		destination[index] = byte(r)
	}
	return len(destination), nil
}

func reasoningText(response modelinvoker.Response) string {
	var result strings.Builder
	for _, item := range response.Output {
		if item.Type == modelinvoker.OutputItemReasoningSummary {
			result.WriteString(item.ReasoningSummary)
		}
	}
	return result.String()
}
