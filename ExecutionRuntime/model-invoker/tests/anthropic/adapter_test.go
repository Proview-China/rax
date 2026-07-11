package anthropic_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/anthropic"
)

func TestMessagesRequestAndResponse(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "environment-key-must-not-be-used")
	t.Setenv("ANTHROPIC_BASE_URL", "http://127.0.0.1:1")
	fixture := mustRead(t, "testdata/message-text.json")
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "explicit-test-key" {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		assertJSONPath(t, body, "model", "claude-sonnet-4-6")
		assertJSONPath(t, body, "max_tokens", float64(256))
		thinking := body["thinking"].(map[string]any)
		assertJSONPath(t, thinking, "type", "adaptive")
		assertJSONPath(t, thinking, "display", "summarized")
		output := body["output_config"].(map[string]any)
		assertJSONPath(t, output, "effort", "high")
		format := output["format"].(map[string]any)
		assertJSONPath(t, format, "type", "json_schema")
		if _, exists := body["cache_control"]; exists {
			t.Errorf("unsupported cache_control was sent: %#v", body["cache_control"])
		}
		tool := body["tools"].([]any)[0].(map[string]any)
		assertJSONPath(t, tool, "strict", true)
		choice := body["tool_choice"].(map[string]any)
		assertJSONPath(t, choice, "disable_parallel_tool_use", true)
		metadata := body["metadata"].(map[string]any)
		assertJSONPath(t, metadata, "user_id", "opaque-user")

		w.Header().Set("content-type", "application/json")
		w.Header().Set("request-id", "req_text_01")
		w.Header().Set("anthropic-ratelimit-requests-remaining", "99")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	adapter, err := provider.New(provider.Config{APIKey: "explicit-test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	strict := true
	parallel := false
	request := baseRequest()
	request.Instructions = []modelinvoker.Instruction{{Role: modelinvoker.RoleSystem, Text: "Be concise."}}
	request.Tools = []modelinvoker.Tool{{
		Name: "get_weather", Description: "Get weather",
		Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"],"additionalProperties":false}`),
		Strict:     &strict,
	}}
	request.ParallelToolCalls = &parallel
	request.Output = modelinvoker.OutputConstraint{
		Type: modelinvoker.OutputJSONSchema, Name: "answer", Description: "Structured answer",
		Schema: json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}`),
		Strict: &strict,
	}
	request.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh, Summary: modelinvoker.ReasoningSummaryAuto}
	request.Metadata = modelinvoker.Metadata{"user_id": "opaque-user"}
	request.ProviderOptions = modelinvoker.ProviderOptions{provider.ProviderID: json.RawMessage(`{}`)}

	response, err := adapter.Invoke(context.Background(), request)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
	if response.ID != "msg_text_01" || response.RequestID != "req_text_01" || response.Text() != "Hello from Claude." {
		t.Fatalf("response identity/text = %#v / %q", response, response.Text())
	}
	if response.StopReason != modelinvoker.StopReasonEndTurn || response.Status != modelinvoker.ResponseStatusCompleted {
		t.Fatalf("status/stop = %q/%q", response.Status, response.StopReason)
	}
	if response.Usage.InputTokens != 17 || response.Usage.OutputTokens != 6 || response.Usage.ReasoningTokens != 2 ||
		response.Usage.CacheReadTokens != 4 || response.Usage.CacheWriteTokens != 3 || response.Usage.TotalTokens != 23 {
		t.Fatalf("usage = %#v", response.Usage)
	}
	if response.State == nil || response.State.Kind != modelinvoker.StateProviderContinuation ||
		!strings.Contains(string(response.State.Payload.Bytes()), "sig_text_01") {
		t.Fatalf("continuation state = %#v", response.State)
	}
	if response.ProviderMetadata["anthropic-ratelimit-requests-remaining"] != "99" {
		t.Fatalf("provider metadata = %#v", response.ProviderMetadata)
	}
	if got := fmt.Sprintf("%v", response.RawRequest); strings.Contains(got, "explicit-test-key") || got != "[REDACTED]" {
		t.Fatalf("RawRequest formatting = %q", got)
	}
	if strings.Contains(string(response.RawRequest.Bytes()), "explicit-test-key") ||
		strings.Contains(string(response.RawResponse.Bytes()), "explicit-test-key") {
		t.Fatal("successful response audit leaked API key")
	}
}

func TestThinkingToolContinuationRoundTrip(t *testing.T) {
	toolFixture := mustRead(t, "testdata/message-tool-thinking.json")
	textFixture := mustRead(t, "testdata/message-text.json")
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		w.Header().Set("content-type", "application/json")
		w.Header().Set("request-id", fmt.Sprintf("req_%d", call))
		if call == 1 {
			_, _ = w.Write(toolFixture)
			return
		}
		messages := body["messages"].([]any)
		if len(messages) != 2 {
			t.Errorf("messages count = %d, want 2", len(messages))
		} else {
			assistant := messages[0].(map[string]any)
			blocks := assistant["content"].([]any)
			if blocks[0].(map[string]any)["signature"] != "sig_tool_01" || blocks[1].(map[string]any)["id"] != "toolu_weather_01" {
				t.Errorf("assistant continuation = %#v", blocks)
			}
			result := messages[1].(map[string]any)["content"].([]any)[0].(map[string]any)
			if result["tool_use_id"] != "toolu_weather_01" || result["is_error"] != true {
				t.Errorf("tool result = %#v", result)
			}
		}
		_, _ = w.Write(textFixture)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}

	first := baseRequest()
	first.Tools = weatherTools()
	first.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh, Summary: modelinvoker.ReasoningSummaryAuto}
	response, err := adapter.Invoke(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if response.State == nil || len(response.FunctionCalls()) != 1 {
		t.Fatalf("first response = %#v", response)
	}

	second := baseRequest()
	second.Input = []modelinvoker.InputItem{modelinvoker.NamedFunctionResultInput("", "get_weather", "upstream failed", true)}
	second.Tools = weatherTools()
	second.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh, Summary: modelinvoker.ReasoningSummaryAuto}
	second.State = response.State
	if _, err := adapter.Invoke(context.Background(), second); err != nil {
		t.Fatalf("continuation Invoke() error = %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("requests = %d, want 2", calls.Load())
	}
}

func TestManualThinkingBudgetAndEffortAreCombined(t *testing.T) {
	fixture := mustRead(t, "testdata/message-text.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		thinking := body["thinking"].(map[string]any)
		assertJSONPath(t, thinking, "type", "enabled")
		assertJSONPath(t, thinking, "budget_tokens", float64(2048))
		assertJSONPath(t, thinking, "display", "summarized")
		output := body["output_config"].(map[string]any)
		assertJSONPath(t, output, "effort", "high")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	budget := int64(2048)
	request := baseRequest()
	request.Budget.MaxOutputTokens = 4096
	request.Reasoning = &modelinvoker.Reasoning{
		Effort: modelinvoker.ReasoningEffortHigh, Summary: modelinvoker.ReasoningSummaryAuto, BudgetTokens: &budget,
	}
	if _, err := adapter.Invoke(context.Background(), request); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
}

func TestExplicitFieldDegradationsAreAudited(t *testing.T) {
	fixture := mustRead(t, "testdata/message-text.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		system := body["system"].([]any)
		if system[0].(map[string]any)["text"] != "Developer-only instruction" {
			t.Errorf("system = %#v", system)
		}
		thinking := body["thinking"].(map[string]any)
		assertJSONPath(t, thinking, "display", "summarized")
		assertJSONPath(t, body["output_config"].(map[string]any), "effort", "low")
		if _, exists := body["metadata"]; exists {
			t.Errorf("unsupported metadata was sent: %#v", body["metadata"])
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := baseRequest()
	request.AllowDegradation = true
	request.Instructions = []modelinvoker.Instruction{{Role: modelinvoker.RoleDeveloper, Text: "Developer-only instruction"}}
	request.Metadata = modelinvoker.Metadata{"trace": "local-only"}
	request.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortMinimal, Summary: modelinvoker.ReasoningSummaryConcise}
	response, err := adapter.Invoke(context.Background(), request)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !response.MappingReport.HasDegradation() || len(response.MappingReport.Decisions) < 4 {
		t.Fatalf("mapping report = %#v", response.MappingReport)
	}
}

func TestMappingRejectionsDoNotReachHTTP(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	strictFalse := false
	tests := []struct {
		name   string
		mutate func(*modelinvoker.Request)
		want   string
	}{
		{name: "zero max tokens", mutate: func(r *modelinvoker.Request) { r.Budget.MaxOutputTokens = 0 }, want: "max output tokens"},
		{name: "developer instruction", mutate: func(r *modelinvoker.Request) {
			r.Instructions = []modelinvoker.Instruction{{Role: modelinvoker.RoleDeveloper, Text: "hidden"}}
		}, want: "developer instructions"},
		{name: "json object", mutate: func(r *modelinvoker.Request) { r.Output.Type = modelinvoker.OutputJSONObject }, want: "json_object"},
		{name: "non-strict schema", mutate: func(r *modelinvoker.Request) {
			r.Output = modelinvoker.OutputConstraint{Type: modelinvoker.OutputJSONSchema, Name: "answer", Schema: json.RawMessage(`{"type":"object"}`), Strict: &strictFalse}
		}, want: "cannot explicitly disable"},
		{name: "forced tool with thinking", mutate: func(r *modelinvoker.Request) {
			r.Tools = weatherTools()
			r.ToolChoice.Mode = modelinvoker.ToolChoiceRequired
			r.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
		}, want: "forced tool choice"},
		{name: "unknown metadata", mutate: func(r *modelinvoker.Request) { r.Metadata = modelinvoker.Metadata{"trace": "x"} }, want: "metadata key"},
		{name: "unknown function result id", mutate: func(r *modelinvoker.Request) {
			r.Input = []modelinvoker.InputItem{modelinvoker.FunctionResultInput("toolu_unknown", "orphan", false)}
		}, want: "unknown call ID"},
		{name: "unknown options", mutate: func(r *modelinvoker.Request) {
			r.ProviderOptions = modelinvoker.ProviderOptions{provider.ProviderID: json.RawMessage(`{"beta":true}`)}
		}, want: "provider options are not defined"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := baseRequest()
			test.mutate(&request)
			_, err := adapter.Invoke(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("HTTP calls = %d, want 0", calls.Load())
	}
}

func baseRequest() modelinvoker.Request {
	return modelinvoker.Request{
		Provider: provider.ProviderID, Protocol: modelinvoker.ProtocolMessages,
		Model: "claude-sonnet-4-6", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Hello")},
		Budget: modelinvoker.Budget{MaxOutputTokens: 256},
	}
}

func weatherTools() []modelinvoker.Tool {
	return []modelinvoker.Tool{{
		Name: "get_weather", Description: "Get weather",
		Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
	}}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertJSONPath(t *testing.T, object map[string]any, key string, want any) {
	t.Helper()
	if got := object[key]; got != want {
		t.Errorf("%s = %#v, want %#v", key, got, want)
	}
}
