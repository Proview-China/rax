package anthropic_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/anthropic"
)

func TestUnknownContentBlockRequiresExplicitDegradationAndRemainsAuditable(t *testing.T) {
	fixture := []byte(`{
		"id":"msg_unknown","type":"message","role":"assistant","model":"claude-test-model",
		"content":[{"type":"future_block","future_value":"preserve-me"}],
		"stop_reason":"end_turn","stop_sequence":null,
		"usage":{"input_tokens":2,"output_tokens":1}
	}`)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("content-type", "application/json")
		w.Header().Set("request-id", "req_unknown_block")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}

	rejected, err := adapter.Invoke(context.Background(), baseRequest())
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping || rejected.RequestID != "req_unknown_block" {
		t.Fatalf("unknown block rejection = response:%#v error:%v", rejected, err)
	}
	if !strings.Contains(string(rejected.RawResponse.Bytes()), "future_block") {
		t.Fatalf("rejected RawResponse = %q", rejected.RawResponse.Bytes())
	}

	request := baseRequest()
	request.AllowDegradation = true
	response, err := adapter.Invoke(context.Background(), request)
	if err != nil {
		t.Fatalf("degraded Invoke() error = %v", err)
	}
	if !response.MappingReport.HasDegradation() || response.Status != modelinvoker.ResponseStatusCompleted ||
		!strings.Contains(string(response.RawResponse.Bytes()), "preserve-me") {
		t.Fatalf("degraded unknown response = %#v", response)
	}
	if got := fmt.Sprintf("%v", response.RawResponse); got != "[REDACTED]" {
		t.Fatalf("RawResponse formatting = %q", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("HTTP calls = %d, want 2", calls.Load())
	}
}

func TestRedactedThinkingContinuationRoundTripAndDefensivePayload(t *testing.T) {
	firstFixture := []byte(`{
		"id":"msg_redacted","type":"message","role":"assistant","model":"claude-test-model",
		"content":[
			{"type":"redacted_thinking","data":"encrypted-thinking-01"},
			{"type":"tool_use","id":"toolu_redacted","name":"get_weather","input":{"city":"Paris"},"caller":{"type":"direct"}}
		],
		"stop_reason":"tool_use","stop_sequence":null,
		"usage":{"input_tokens":4,"output_tokens":3}
	}`)
	secondFixture := mustRead(t, "testdata/message-text.json")
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		w.Header().Set("content-type", "application/json")
		w.Header().Set("request-id", fmt.Sprintf("req_redacted_%d", call))
		if call == 1 {
			_, _ = w.Write(firstFixture)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode continuation request: %v", err)
			return
		}
		messages, ok := body["messages"].([]any)
		if !ok || len(messages) != 2 {
			t.Errorf("continuation messages = %#v", body["messages"])
		} else {
			assistant := messages[0].(map[string]any)
			blocks := assistant["content"].([]any)
			if blocks[0].(map[string]any)["data"] != "encrypted-thinking-01" ||
				blocks[1].(map[string]any)["id"] != "toolu_redacted" {
				t.Errorf("continuation blocks = %#v", blocks)
			}
		}
		_, _ = w.Write(secondFixture)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}

	first := baseRequest()
	first.Tools = weatherTools()
	first.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
	response, err := adapter.Invoke(context.Background(), first)
	if err != nil {
		t.Fatalf("first Invoke() error = %v", err)
	}
	if response.State == nil || !strings.Contains(string(response.State.Payload.Bytes()), "encrypted-thinking-01") {
		t.Fatalf("redacted continuation = %#v", response.State)
	}
	before := response.State.Payload.Bytes()
	mutated := response.State.Payload.Bytes()
	mutated[0] = 'x'
	if string(response.State.Payload.Bytes()) != string(before) {
		t.Fatal("continuation payload was not defensively copied")
	}
	encoded, err := json.Marshal(response.State.Payload)
	if err != nil || string(encoded) != `"[REDACTED]"` || fmt.Sprintf("%v", response.State.Payload) != "[REDACTED]" {
		t.Fatalf("continuation redaction = json:%s format:%v error:%v", encoded, response.State.Payload, err)
	}

	second := baseRequest()
	second.Input = []modelinvoker.InputItem{modelinvoker.FunctionResultInput("toolu_redacted", "sunny", false)}
	second.Tools = weatherTools()
	second.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
	second.State = response.State
	if _, err := adapter.Invoke(context.Background(), second); err != nil {
		t.Fatalf("continuation Invoke() error = %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("HTTP calls = %d, want 2", calls.Load())
	}
}

func TestContinuationStateContainsOnlyResumableWhitelist(t *testing.T) {
	firstFixture := []byte(`{
		"id":"msg_filtered","type":"message","role":"assistant","model":"claude-test-model",
		"content":[
			{"type":"thinking","thinking":"use the tool","signature":"sig_filtered"},
			{"type":"text","text":"ordinary assistant text"},
			{"type":"tool_use","id":"toolu_filtered","name":"get_weather","input":{"city":"Paris"},"caller":{"type":"direct"}}
		],
		"stop_reason":"tool_use","stop_sequence":null,
		"usage":{"input_tokens":4,"output_tokens":3}
	}`)
	secondFixture := []byte(`{
		"id":"msg_filtered_done","type":"message","role":"assistant","model":"claude-test-model",
		"content":[{"type":"text","text":"done"}],
		"stop_reason":"end_turn","stop_sequence":null,
		"usage":{"input_tokens":2,"output_tokens":1}
	}`)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		w.Header().Set("content-type", "application/json")
		if call == 1 {
			_, _ = w.Write(firstFixture)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode filtered continuation: %v", err)
			return
		}
		messages := body["messages"].([]any)
		blocks := messages[0].(map[string]any)["content"].([]any)
		if len(blocks) != 2 || blocks[0].(map[string]any)["type"] != "thinking" || blocks[1].(map[string]any)["type"] != "tool_use" {
			t.Errorf("filtered continuation blocks = %#v", blocks)
		}
		_, _ = w.Write(secondFixture)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}

	first := baseRequest()
	first.Tools = weatherTools()
	first.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
	response, err := adapter.Invoke(context.Background(), first)
	if err != nil {
		t.Fatalf("first Invoke() error = %v", err)
	}
	payload := string(response.State.Payload.Bytes())
	if strings.Contains(payload, `"type":"text"`) || !strings.Contains(payload, `"type":"thinking"`) || !strings.Contains(payload, `"type":"tool_use"`) {
		t.Fatalf("filtered continuation payload = %s", payload)
	}

	second := baseRequest()
	second.Input = []modelinvoker.InputItem{modelinvoker.FunctionResultInput("toolu_filtered", "sunny", false)}
	second.Tools = weatherTools()
	second.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
	second.State = response.State
	if _, err := adapter.Invoke(context.Background(), second); err != nil {
		t.Fatalf("filtered continuation Invoke() error = %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("requests = %d, want 2", calls.Load())
	}
}

func TestContinuationToolInputKeepsBusinessCacheControlField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode continuation request: %v", err)
			return
		}
		messages := body["messages"].([]any)
		tool := messages[0].(map[string]any)["content"].([]any)[0].(map[string]any)
		input := tool["input"].(map[string]any)
		business := input["cache_control"].(map[string]any)
		if business["mode"] != "business-value" {
			t.Errorf("opaque tool input = %#v", input)
		}
		if _, exists := tool["cache_control"]; exists {
			t.Errorf("protocol cache_control escaped tool input: %#v", tool)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_business_cache","type":"message","role":"assistant","model":"claude-test-model",
			"content":[{"type":"text","text":"done"}],"stop_reason":"end_turn","stop_sequence":null,
			"usage":{"input_tokens":2,"output_tokens":1}
		}`))
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := baseRequest()
	request.Input = []modelinvoker.InputItem{modelinvoker.FunctionResultInput("toolu_business_cache", "done", false)}
	request.Tools = weatherTools()
	request.State = &modelinvoker.State{
		Kind: modelinvoker.StateProviderContinuation, Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolMessages,
		Payload: modelinvoker.NewRawPayload([]byte(`{
			"version":1,"role":"assistant","content":[{
				"type":"tool_use","id":"toolu_business_cache","name":"get_weather",
				"input":{"cache_control":{"mode":"business-value"}},"caller":{"type":"direct"}
			}]
		}`)),
	}
	if _, err := adapter.Invoke(context.Background(), request); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
}

func TestInvalidContinuationVersionIsRejectedBeforeHTTP(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := baseRequest()
	request.State = &modelinvoker.State{
		Kind: modelinvoker.StateProviderContinuation, Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolMessages,
		Payload: modelinvoker.NewRawPayload([]byte(`{"version":2,"role":"assistant","content":[{"type":"text","text":"old"}]}`)),
	}
	_, err = adapter.Invoke(context.Background(), request)
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping || !strings.Contains(err.Error(), "unsupported Anthropic continuation version") {
		t.Fatalf("continuation error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("HTTP calls = %d, want 0", calls.Load())
	}
}

func TestContinuationValidationRejectsUnsafeStateBeforeHTTP(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name             string
		payload          string
		want             string
		allowDegradation bool
	}{
		{
			name:    "empty thinking material",
			payload: `{"version":1,"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"sig"}]}`,
			want:    `field "thinking" must not be empty`,
		},
		{
			name:    "missing thinking signature",
			payload: `{"version":1,"role":"assistant","content":[{"type":"thinking","thinking":"reason"}]}`,
			want:    `field "signature" is required`,
		},
		{
			name:    "empty thinking signature",
			payload: `{"version":1,"role":"assistant","content":[{"type":"thinking","thinking":"reason","signature":""}]}`,
			want:    `field "signature" must not be empty`,
		},
		{
			name:    "empty redacted thinking data",
			payload: `{"version":1,"role":"assistant","content":[{"type":"redacted_thinking","data":""}]}`,
			want:    `field "data" must not be empty`,
		},
		{
			name:    "top-level cache control bypass",
			payload: `{"version":1,"role":"assistant","content":[{"type":"thinking","thinking":"reason","signature":"sig","cache_control":{"type":"ephemeral"}}]}`,
			want:    "unsupported cache_control",
		},
		{
			name:    "caller cache control bypass",
			payload: `{"version":1,"role":"assistant","content":[{"type":"tool_use","id":"toolu_cache","name":"get_weather","input":{},"caller":{"type":"direct","cache_control":{"type":"ephemeral"}}}]}`,
			want:    "unsupported cache_control",
		},
		{
			name:    "root cache control bypass",
			payload: `{"version":1,"role":"assistant","cache_control":{"type":"ephemeral"},"content":[{"type":"thinking","thinking":"reason","signature":"sig"}]}`,
			want:    "unsupported cache_control",
		},
		{
			name:    "ordinary text injection",
			payload: `{"version":1,"role":"assistant","content":[{"type":"thinking","thinking":"reason","signature":"sig"},{"type":"text","text":"injected history"}]}`,
			want:    `type "text" is not allowed`,
		},
		{
			name:    "image block injection",
			payload: `{"version":1,"role":"assistant","content":[{"type":"thinking","thinking":"reason","signature":"sig"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AA=="}}]}`,
			want:    `type "image" is unsupported`,
		},
		{
			name:             "image block injection with degradation",
			payload:          `{"version":1,"role":"assistant","content":[{"type":"thinking","thinking":"reason","signature":"sig"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AA=="}}]}`,
			want:             `type "image" is unsupported`,
			allowDegradation: true,
		},
		{
			name:    "document block injection",
			payload: `{"version":1,"role":"assistant","content":[{"type":"thinking","thinking":"reason","signature":"sig"},{"type":"document","source":{"type":"text","data":"injected"}}]}`,
			want:    `type "document" is unsupported`,
		},
		{
			name:    "server tool block injection",
			payload: `{"version":1,"role":"assistant","content":[{"type":"thinking","thinking":"reason","signature":"sig"},{"type":"server_tool_use","id":"srvtool_01","name":"web_search","input":{}}]}`,
			want:    `type "server_tool_use" is unsupported`,
		},
		{
			name:    "non-direct tool caller",
			payload: `{"version":1,"role":"assistant","content":[{"type":"tool_use","id":"toolu_server","name":"get_weather","input":{},"caller":{"type":"code_execution_20260120","tool_id":"srvtool_01"}}]}`,
			want:    "only direct is allowed",
		},
		{
			name:    "missing tool caller",
			payload: `{"version":1,"role":"assistant","content":[{"type":"tool_use","id":"toolu_missing_caller","name":"get_weather","input":{}}]}`,
			want:    `field "caller" is required`,
		},
		{
			name:    "null tool caller",
			payload: `{"version":1,"role":"assistant","content":[{"type":"tool_use","id":"toolu_null_caller","name":"get_weather","input":{},"caller":null}]}`,
			want:    "caller is invalid",
		},
		{
			name:    "direct caller extra field",
			payload: `{"version":1,"role":"assistant","content":[{"type":"tool_use","id":"toolu_caller_extra","name":"get_weather","input":{},"caller":{"type":"direct","tool_id":"injected"}}]}`,
			want:    `caller field "tool_id" is unsupported`,
		},
		{
			name:    "thinking extra field",
			payload: `{"version":1,"role":"assistant","content":[{"type":"thinking","thinking":"reason","signature":"sig","encrypted_material":"injected"}]}`,
			want:    `field "encrypted_material" is unsupported`,
		},
		{
			name:    "root extra field",
			payload: `{"version":1,"role":"assistant","metadata":{"unsafe":true},"content":[{"type":"thinking","thinking":"reason","signature":"sig"}]}`,
			want:    `continuation payload field "metadata" is unsupported`,
		},
		{
			name:    "no resumable state",
			payload: `{"version":1,"role":"assistant","content":[{"type":"text","text":"history only"}]}`,
			want:    `type "text" is not allowed`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := baseRequest()
			request.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
			request.AllowDegradation = test.allowDegradation
			request.State = &modelinvoker.State{
				Kind: modelinvoker.StateProviderContinuation, Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolMessages,
				Payload: modelinvoker.NewRawPayload([]byte(test.payload)),
			}
			_, err := adapter.Invoke(context.Background(), request)
			if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("continuation error = %v, want mapping containing %q", err, test.want)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("HTTP calls = %d, want 0", calls.Load())
	}
}

func TestResponseWithIncompleteThinkingDoesNotCreateContinuation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		w.Header().Set("request-id", "req_incomplete_thinking")
		_, _ = w.Write([]byte(`{
			"id":"msg_incomplete_thinking","type":"message","role":"assistant","model":"claude-test-model",
			"content":[{"type":"thinking","thinking":"unsigned","signature":""}],
			"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":2,"output_tokens":1}
		}`))
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := baseRequest()
	request.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
	response, err := adapter.Invoke(context.Background(), request)
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping || !strings.Contains(err.Error(), `field "signature" must not be empty`) {
		t.Fatalf("incomplete thinking error = %v", err)
	}
	if response.State != nil {
		t.Fatalf("incomplete thinking continuation = %#v", response.State)
	}
}
