package xai_test

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
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/xai"
)

func request(endpoint string) modelinvoker.Request {
	return modelinvoker.Request{
		Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolResponses, Endpoint: endpoint, Model: "grok-4.5",
		Input:  []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
		Budget: modelinvoker.Budget{MaxOutputTokens: 256},
	}
}

func TestConfigurationAndEndpointBoundaries(t *testing.T) {
	adapter, err := provider.New(provider.Config{APIKey: "xai-test-key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Capabilities(context.Background(), modelinvoker.CapabilityQuery{Protocol: modelinvoker.ProtocolResponses, Endpoint: "https://api.x.ai/v1", Model: "grok-4.5"}); err != nil {
		t.Fatalf("official endpoint rejected: %v", err)
	}
	if _, err := adapter.Capabilities(context.Background(), modelinvoker.CapabilityQuery{Protocol: modelinvoker.ProtocolResponses, Endpoint: "https://grok.com/v1", Model: "grok-4.5"}); err == nil {
		t.Fatal("Grok consumer endpoint accepted")
	}

	invalid := []provider.Config{
		{},
		{APIKey: "bad\nkey"},
		{APIKey: "xai-test", BaseURL: "https://grok.com/v1"},
		{APIKey: "xai-test", BaseURL: "https://api.x.ai/v1"},
		{APIKey: "xai-test", BaseURL: "ftp://127.0.0.1/v1"},
		{APIKey: "xai-test", BaseURL: "http://127.0.0.1/v1?key=secret"},
	}
	for index, config := range invalid {
		if _, err := provider.New(config); err == nil {
			t.Errorf("invalid config %d accepted", index)
		}
	}
	config := provider.Config{APIKey: "xai-format-secret"}
	if strings.Contains(fmt.Sprintf("%v %#v", config, config), "format-secret") || strings.Contains(fmt.Sprintf("%v %#v", adapter, adapter), "test-key") {
		t.Fatal("configuration formatting leaked a key")
	}
}

func TestResponsesStateReasoningToolsCacheAndStreaming(t *testing.T) {
	type capture struct {
		path string
		auth string
		body map[string]any
	}
	captured := make(chan capture, 3)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured <- capture{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: body}
		w.Header().Set("x-request-id", "xai-test-request")
		if body["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher := w.(http.Flusher)
			for _, event := range []string{
				`{"type":"response.created","sequence_number":1,"response":{"id":"resp-xai-stream","model":"grok-4.5","status":"in_progress","output":[]}}`,
				`{"type":"response.output_text.delta","sequence_number":2,"item_id":"msg","output_index":0,"content_index":0,"delta":"streamed"}`,
				`{"type":"response.completed","sequence_number":3,"response":{"id":"resp-xai-stream","model":"grok-4.5","status":"completed","output":[{"id":"msg","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"streamed","annotations":[]}]}],"usage":{"input_tokens":5,"input_tokens_details":{"cached_tokens":3},"output_tokens":2,"output_tokens_details":{"reasoning_tokens":1},"total_tokens":7}}}`,
			} {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
				flusher.Flush()
			}
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(w, `{"id":"resp-xai-1","object":"response","created_at":1770000000,"model":"grok-4.5","status":"completed","output":[{"id":"reason","type":"reasoning","summary":[]},{"id":"call","type":"function_call","status":"completed","call_id":"weather-1","name":"weather","arguments":"{\"city\":\"Memphis\"}"}],"usage":{"input_tokens":3,"input_tokens_details":{"cached_tokens":2},"output_tokens":4,"output_tokens_details":{"reasoning_tokens":3},"total_tokens":7,"cost_in_usd_ticks":1234}}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"resp-xai-2","object":"response","created_at":1770000001,"model":"grok-4.5","status":"completed","output":[{"id":"msg","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"sunny","annotations":[]}]}],"usage":{"input_tokens":5,"output_tokens":1,"total_tokens":6}}`)
	}))
	defer server.Close()

	adapter, err := provider.New(provider.Config{APIKey: "xai-responses-secret", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	r := request(server.URL)
	r.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortMedium}
	parallel := true
	r.ParallelToolCalls = &parallel
	r.Tools = []modelinvoker.Tool{{Name: "weather", Description: "Look up weather", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)}}
	r.ProviderOptions = modelinvoker.ProviderOptions{provider.ProviderID: json.RawMessage(`{"prompt_cache_key":"conversation-123"}`)}
	first, err := adapter.Invoke(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	native := <-captured
	reasoning, _ := native.body["reasoning"].(map[string]any)
	if native.path != "/responses" || native.auth != "Bearer xai-responses-secret" || reasoning["effort"] != "medium" || native.body["prompt_cache_key"] != "conversation-123" || native.body["parallel_tool_calls"] != true {
		t.Fatalf("native request=%#v", native)
	}
	if first.State == nil || first.State.Provider != provider.ProviderID || first.State.ID != "resp-xai-1" || first.Usage.ReasoningTokens != 3 || first.Usage.CacheReadTokens != 2 || len(first.FunctionCalls()) != 1 {
		t.Fatalf("first response=%#v", first)
	}

	continued := request(server.URL)
	continued.State = first.State
	continued.Input = []modelinvoker.InputItem{modelinvoker.FunctionResultInput("weather-1", `{"temperature":23}`, false)}
	second, err := adapter.Invoke(context.Background(), continued)
	if err != nil {
		t.Fatal(err)
	}
	secondNative := <-captured
	if secondNative.body["previous_response_id"] != "resp-xai-1" || second.Text() != "sunny" {
		t.Fatalf("continuation body=%#v response=%#v", secondNative.body, second)
	}

	stream, err := adapter.Stream(context.Background(), request(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var text strings.Builder
	var terminal *modelinvoker.Response
	for stream.Next() {
		event := stream.Event()
		text.WriteString(event.TextDelta)
		if event.Type == modelinvoker.StreamEventResponseCompleted {
			terminal = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	_ = <-captured
	if text.String() != "streamed" || terminal == nil || terminal.Usage.CacheReadTokens != 3 || terminal.Usage.ReasoningTokens != 1 {
		t.Fatalf("stream text=%q terminal=%#v", text.String(), terminal)
	}
}

func TestPreflightFailureAndSecurityBoundaries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":"rate_limit_exceeded","message":"secret provider detail"}}`)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "xai-boundary-secret", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}

	bad := []modelinvoker.Request{}
	wrongModel := request(server.URL)
	wrongModel.Model = "grok-4.3"
	bad = append(bad, wrongModel)
	wrongEndpoint := request(server.URL + "/proxy")
	bad = append(bad, wrongEndpoint)
	wrongProtocol := request(server.URL)
	wrongProtocol.Protocol = modelinvoker.ProtocolChatCompletions
	bad = append(bad, wrongProtocol)
	metadata := request(server.URL)
	metadata.Metadata = modelinvoker.Metadata{"ignored": "value"}
	bad = append(bad, metadata)
	structured := request(server.URL)
	structured.Output.Type = modelinvoker.OutputJSONObject
	bad = append(bad, structured)
	none := request(server.URL)
	none.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortNone}
	bad = append(bad, none)
	unknownOption := request(server.URL)
	unknownOption.ProviderOptions = modelinvoker.ProviderOptions{provider.ProviderID: json.RawMessage(`{"other":true}`)}
	bad = append(bad, unknownOption)
	strict := true
	strictTool := request(server.URL)
	strictTool.Tools = []modelinvoker.Tool{{Name: "tool", Parameters: json.RawMessage(`{"type":"object"}`), Strict: &strict}}
	bad = append(bad, strictTool)
	for index, item := range bad {
		if _, err := adapter.Invoke(context.Background(), item); err == nil {
			t.Errorf("invalid request %d accepted", index)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("preflight failures made %d HTTP calls", calls.Load())
	}

	_, err = adapter.Invoke(context.Background(), request(server.URL))
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError.Kind != modelinvoker.ErrorRateLimit || !invocationError.Retryable || invocationError.Code != "rate_limit_exceeded" {
		t.Fatalf("error=%#v", invocationError)
	}
	if strings.Contains(fmt.Sprint(err), "secret provider detail") || strings.Contains(fmt.Sprint(err), "xai-boundary-secret") {
		t.Fatalf("public error leaked provider data: %v", err)
	}
}

func TestFailureMatrixModelMismatchRedirectCancellationAndBodyLimit(t *testing.T) {
	tests := []struct {
		status int
		code   string
		kind   modelinvoker.ErrorKind
		retry  bool
	}{
		{401, "invalid_api_key", modelinvoker.ErrorAuthentication, false},
		{403, "permission_denied", modelinvoker.ErrorPermission, false},
		{404, "model_not_found", modelinvoker.ErrorMapping, false},
		{400, "insufficient_quota", modelinvoker.ErrorBilling, false},
		{429, "rate_limit_exceeded", modelinvoker.ErrorRateLimit, true},
		{503, "service_unavailable", modelinvoker.ErrorProviderUnavailable, true},
	}
	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprintf(w, `{"error":{"code":%q,"message":"detail"}}`, tc.code)
			}))
			defer server.Close()
			adapter, err := provider.New(provider.Config{APIKey: "xai-error-secret", BaseURL: server.URL, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			_, err = adapter.Invoke(context.Background(), request(server.URL))
			var invocationError *modelinvoker.Error
			if !errors.As(err, &invocationError) || invocationError.Kind != tc.kind || invocationError.Retryable != tc.retry || invocationError.Code != tc.code {
				t.Fatalf("error=%#v", invocationError)
			}
		})
	}

	mismatch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp","object":"response","model":"grok-4.3","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`)
	}))
	defer mismatch.Close()
	adapter, err := provider.New(provider.Config{APIKey: "xai-model-secret", BaseURL: mismatch.URL, HTTPClient: mismatch.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Invoke(context.Background(), request(mismatch.URL)); modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping {
		t.Fatalf("model mismatch err=%v", err)
	}

	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	adapter, err = provider.New(provider.Config{APIKey: "xai-redirect-secret", BaseURL: source.URL, HTTPClient: source.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Invoke(context.Background(), request(source.URL)); err == nil || redirected.Load() != 0 {
		t.Fatalf("redirect followed or accepted: calls=%d err=%v", redirected.Load(), err)
	}

	var cancelledCalls atomic.Int32
	cancelServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { cancelledCalls.Add(1) }))
	defer cancelServer.Close()
	adapter, err = provider.New(provider.Config{APIKey: "xai-cancel-secret", BaseURL: cancelServer.URL, HTTPClient: cancelServer.Client()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = adapter.Invoke(ctx, request(cancelServer.URL))
	if !errors.Is(err, context.Canceled) || cancelledCalls.Load() != 0 {
		t.Fatalf("cancel err=%v calls=%d", err, cancelledCalls.Load())
	}

	largeClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(io.LimitReader(repeatingReader('x'), 9<<20)), ContentLength: 9 << 20, Request: r}, nil
	})}
	adapter, err = provider.New(provider.Config{APIKey: "xai-limit-secret", BaseURL: "http://127.0.0.1:1", HTTPClient: largeClient})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Invoke(context.Background(), request("http://127.0.0.1:1"))
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError.Code != adaptercore.ResponseBodyLimitErrorCode || invocationError.Retryable {
		t.Fatalf("body-limit error=%#v", invocationError)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type repeatingReader byte

func (reader repeatingReader) Read(destination []byte) (int, error) {
	for index := range destination {
		destination[index] = byte(reader)
	}
	return len(destination), nil
}
