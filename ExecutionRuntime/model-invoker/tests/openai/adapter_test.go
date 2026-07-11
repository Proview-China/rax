package openai_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	openaiadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/openai"
)

func TestPublicInvokeSerializesAndNormalizesBothProtocols(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		protocol   modelinvoker.Protocol
		path       string
		fixture    string
		assertBody func(*testing.T, map[string]any)
	}{
		{
			name: "responses", protocol: modelinvoker.ProtocolResponses, path: "/v1/responses",
			fixture: "testdata/responses-success.json",
			assertBody: func(t *testing.T, body map[string]any) {
				publicWant(t, body, "previous_response_id", "resp_previous")
				publicWant(t, body, "max_output_tokens", float64(128))
				publicWant(t, body, "parallel_tool_calls", true)
				publicWant(t, publicMap(t, body["reasoning"]), "effort", "high")
				publicWant(t, publicMap(t, body["reasoning"]), "summary", "detailed")
				format := publicMap(t, publicMap(t, body["text"])["format"])
				publicWant(t, format, "type", "json_schema")
				publicWant(t, format, "strict", false)
				input := publicSlice(t, body["input"])
				if len(input) != 5 {
					t.Fatalf("input count = %d, want 5", len(input))
				}
				publicWant(t, publicMap(t, input[0]), "role", "system")
				publicWant(t, publicMap(t, input[1]), "role", "developer")
				publicWant(t, publicMap(t, input[3]), "type", "function_call")
				publicWant(t, publicMap(t, input[4]), "type", "function_call_output")
			},
		},
		{
			name: "chat completions", protocol: modelinvoker.ProtocolChatCompletions, path: "/v1/chat/completions",
			fixture: "testdata/chat-success.json",
			assertBody: func(t *testing.T, body map[string]any) {
				publicWant(t, body, "max_completion_tokens", float64(128))
				publicWant(t, body, "parallel_tool_calls", true)
				publicWant(t, body, "reasoning_effort", "high")
				format := publicMap(t, body["response_format"])
				publicWant(t, format, "type", "json_schema")
				publicWant(t, publicMap(t, format["json_schema"]), "strict", false)
				messages := publicSlice(t, body["messages"])
				if len(messages) != 5 {
					t.Fatalf("message count = %d, want 5", len(messages))
				}
				publicWant(t, publicMap(t, messages[0]), "role", "system")
				publicWant(t, publicMap(t, messages[1]), "role", "developer")
				assistant := publicMap(t, messages[3])
				publicWant(t, assistant, "role", "assistant")
				publicWant(t, publicMap(t, publicSlice(t, assistant["tool_calls"])[0]), "id", "call_in")
				publicWant(t, publicMap(t, messages[4]), "tool_call_id", "call_in")
				if _, exists := body["previous_response_id"]; exists {
					t.Fatal("Chat request contains Responses-only previous_response_id")
				}
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			responseBody := publicFixture(t, test.fixture)
			captured := make(chan publicRequestCapture, 1)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				var body map[string]any
				decodeErr := json.NewDecoder(request.Body).Decode(&body)
				captured <- publicRequestCapture{
					method: request.Method, path: request.URL.Path,
					authorization: request.Header.Get("Authorization"), body: body, decodeErr: decodeErr,
				}
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("X-Request-Id", "req_public")
				writer.Header().Set("OpenAI-Processing-Ms", "42")
				writer.Header().Set("X-Ratelimit-Limit-Tokens", "1000")
				_, _ = writer.Write(responseBody)
			}))
			t.Cleanup(server.Close)

			adapter := newPublicAdapter(t, server.URL)
			request := fullPublicRequest(test.protocol)
			request.Endpoint = server.URL + "/v1"
			response, err := adapter.Invoke(context.Background(), request)
			if err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}
			capture := <-captured
			if capture.decodeErr != nil {
				t.Fatalf("decode request: %v", capture.decodeErr)
			}
			if capture.method != http.MethodPost || capture.path != test.path {
				t.Fatalf("native request = %s %s, want POST %s", capture.method, capture.path, test.path)
			}
			if capture.authorization != "Bearer test-only-key" {
				t.Fatalf("Authorization = %q", capture.authorization)
			}
			publicWant(t, capture.body, "model", "test-model")
			test.assertBody(t, capture.body)
			if strings.Contains(string(response.RawRequest.Bytes()), "test-only-key") {
				t.Fatal("RawRequest leaked API key")
			}
			if response.MappingReport.Endpoint != request.Endpoint {
				t.Fatalf("mapping endpoint = %q, want %q", response.MappingReport.Endpoint, request.Endpoint)
			}

			wantText := "hello responses"
			if test.protocol == modelinvoker.ProtocolChatCompletions {
				wantText = "hello chat"
			}
			if response.Text() != wantText || response.RequestID != "req_public" {
				t.Fatalf("normalized text/requestID = %q/%q", response.Text(), response.RequestID)
			}
			if test.protocol == modelinvoker.ProtocolResponses {
				if response.State == nil || response.State.Kind != modelinvoker.StateServerContinuation ||
					response.State.Provider != openaiadapter.ProviderID || response.State.Protocol != modelinvoker.ProtocolResponses ||
					response.State.ID != response.ID || !response.State.Payload.Empty() {
					t.Fatalf("Responses continuation state = %#v", response.State)
				}
			} else if response.State != nil {
				t.Fatalf("Chat response state = %#v, want nil", response.State)
			}
			wantStopReason := modelinvoker.StopReasonToolCall
			if test.protocol == modelinvoker.ProtocolChatCompletions {
				wantStopReason = modelinvoker.StopReasonEndTurn
			}
			if response.StopReason != wantStopReason || response.StopSequence != "" {
				t.Fatalf("stop reason/sequence = %q/%q, want %q/empty", response.StopReason, response.StopSequence, wantStopReason)
			}
			calls := response.FunctionCalls()
			if len(calls) != 1 || calls[0].ID != "call_out" || calls[0].Name != "lookup" || string(calls[0].Arguments) != `{"city":"Rome"}` {
				t.Fatalf("function calls = %#v", calls)
			}
			wantUsage := modelinvoker.Usage{InputTokens: 10, OutputTokens: 7, ReasoningTokens: 2, CacheReadTokens: 3, TotalTokens: 17}
			if response.Usage != wantUsage {
				t.Fatalf("usage = %#v, want %#v", response.Usage, wantUsage)
			}
			if response.ProviderMetadata["openai-processing-ms"] != "42" || response.ProviderMetadata["x-ratelimit-limit-tokens"] != "1000" {
				t.Fatalf("provider metadata = %#v", response.ProviderMetadata)
			}
			if response.RawRequest.String() != "[REDACTED]" || response.RawResponse.String() != "[REDACTED]" {
				t.Fatal("raw audit payloads are not redacted by default")
			}
		})
	}
}

func publicFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}

func TestPublicUnspecifiedStrictnessPreservesProtocolDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		protocol     modelinvoker.Protocol
		responseBody string
		tool         func(*testing.T, map[string]any) map[string]any
		format       func(*testing.T, map[string]any) map[string]any
	}{
		{
			name: "responses", protocol: modelinvoker.ProtocolResponses,
			responseBody: `{"id":"resp","model":"test-model","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`,
			tool: func(t *testing.T, body map[string]any) map[string]any {
				return publicMap(t, publicSlice(t, body["tools"])[0])
			},
			format: func(t *testing.T, body map[string]any) map[string]any {
				return publicMap(t, publicMap(t, body["text"])["format"])
			},
		},
		{
			name: "chat completions", protocol: modelinvoker.ProtocolChatCompletions,
			responseBody: `{"id":"chat","model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
			tool: func(t *testing.T, body map[string]any) map[string]any {
				return publicMap(t, publicMap(t, publicSlice(t, body["tools"])[0])["function"])
			},
			format: func(t *testing.T, body map[string]any) map[string]any {
				return publicMap(t, publicMap(t, body["response_format"])["json_schema"])
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			captured := make(chan map[string]any, 1)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				captured <- body
				writer.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(writer, test.responseBody)
			}))
			t.Cleanup(server.Close)

			request := basePublicRequest(test.protocol)
			request.Tools = []modelinvoker.Tool{{
				Name:       "lookup",
				Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
			}}
			request.Output = modelinvoker.OutputConstraint{
				Type:   modelinvoker.OutputJSONSchema,
				Name:   "answer",
				Schema: json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string"}}}`),
			}

			if _, err := newPublicAdapter(t, server.URL).Invoke(context.Background(), request); err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}
			body := <-captured
			if _, exists := test.tool(t, body)["strict"]; exists {
				t.Fatalf("unspecified tool strictness was serialized: %#v", body)
			}
			if _, exists := test.format(t, body)["strict"]; exists {
				t.Fatalf("unspecified output strictness was serialized: %#v", body)
			}
		})
	}
}

func TestPublicCapabilitiesAndExplicitChatReasoningDegradation(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	captured := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		captured <- body
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"id":"chat","model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(server.Close)

	adapter := newPublicAdapter(t, server.URL)
	contract, err := adapter.Capabilities(context.Background(), modelinvoker.CapabilityQuery{Protocol: modelinvoker.ProtocolChatCompletions, Model: "test-model"})
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	if contract[modelinvoker.CapabilityReasoning].Level != modelinvoker.SupportNative ||
		contract[modelinvoker.CapabilityReasoningSummary].Level != modelinvoker.SupportPartial ||
		contract[modelinvoker.CapabilityServerState].Level != modelinvoker.SupportUnsupported {
		t.Fatalf("Chat capability contract = %#v", contract)
	}
	for _, capability := range []modelinvoker.Capability{modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityReasoning} {
		support := contract[capability]
		if len(support.Models) != 1 || support.Models[0] != "test-model" || len(support.Limitations) == 0 {
			t.Fatalf("model-scoped support for %s = %#v", capability, support)
		}
	}

	invoker := newPublicInvoker(t, adapter)
	effortOnly := basePublicRequest(modelinvoker.ProtocolChatCompletions)
	effortOnly.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortMedium}
	effortResponse, err := invoker.Invoke(context.Background(), effortOnly)
	if err != nil {
		t.Fatalf("Invoke(effort only) error = %v", err)
	}
	effortBody := <-captured
	publicWant(t, effortBody, "reasoning_effort", "medium")
	if effortResponse.MappingReport.HasDegradation() {
		t.Fatalf("effort-only request was marked degraded: %#v", effortResponse.MappingReport)
	}

	request := basePublicRequest(modelinvoker.ProtocolChatCompletions)
	request.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortMedium, Summary: modelinvoker.ReasoningSummaryAuto}
	_, err = invoker.Invoke(context.Background(), request)
	assertPublicErrorKind(t, err, modelinvoker.ErrorUnsupportedCapability)
	if calls.Load() != 1 {
		t.Fatalf("native calls before summary degradation permission = %d, want 1 effort-only call", calls.Load())
	}

	request.AllowDegradation = true
	response, err := invoker.Invoke(context.Background(), request)
	if err != nil {
		t.Fatalf("Invoke(allow degradation) error = %v", err)
	}
	body := <-captured
	publicWant(t, body, "reasoning_effort", "medium")
	if strings.Contains(string(response.RawRequest.Bytes()), "summary") {
		t.Fatalf("unsupported Chat reasoning summary was serialized: %s", response.RawRequest.Bytes())
	}
	if !response.MappingReport.HasDegradation() {
		t.Fatalf("mapping report lacks explicit degradation: %#v", response.MappingReport)
	}
	for _, decision := range response.MappingReport.Decisions {
		if decision.Capability == modelinvoker.CapabilityReasoning && decision.Action == modelinvoker.MappingDegraded {
			t.Fatalf("reasoning effort was incorrectly marked degraded: %#v", response.MappingReport)
		}
	}
}

func TestPublicFunctionErrorResultRequiresExplicitDegradation(t *testing.T) {
	t.Parallel()

	for _, protocol := range []modelinvoker.Protocol{modelinvoker.ProtocolResponses, modelinvoker.ProtocolChatCompletions} {
		protocol := protocol
		t.Run(string(protocol), func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writer.Header().Set("Content-Type", "application/json")
				if protocol == modelinvoker.ProtocolResponses {
					_, _ = fmt.Fprint(writer, `{"id":"resp","model":"test-model","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`)
					return
				}
				_, _ = fmt.Fprint(writer, `{"id":"chat","model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
			}))
			t.Cleanup(server.Close)

			adapter := newPublicAdapter(t, server.URL)
			request := basePublicRequest(protocol)
			request.Input = []modelinvoker.InputItem{
				modelinvoker.FunctionCallInput("call_1", "lookup", json.RawMessage(`{"city":"Rome"}`)),
				modelinvoker.FunctionResultInput("call_1", "tool failed", true),
			}
			_, err := newPublicInvoker(t, adapter).Invoke(context.Background(), request)
			assertPublicErrorKind(t, err, modelinvoker.ErrorUnsupportedCapability)
			if calls.Load() != 0 {
				t.Fatalf("native calls before degradation permission = %d", calls.Load())
			}

			request.AllowDegradation = true
			response, err := newPublicInvoker(t, adapter).Invoke(context.Background(), request)
			if err != nil {
				t.Fatalf("Invoke(allow degradation) error = %v", err)
			}
			if calls.Load() != 1 || !response.MappingReport.HasDegradation() {
				t.Fatalf("calls/report = %d/%#v", calls.Load(), response.MappingReport)
			}
			decisionFound := false
			for _, decision := range response.MappingReport.Decisions {
				decisionFound = decisionFound || decision.Capability == modelinvoker.CapabilityFunctionErrorResult && decision.Action == modelinvoker.MappingDegraded
			}
			if !decisionFound {
				t.Fatalf("function error degradation decision missing: %#v", response.MappingReport)
			}
		})
	}
}

func TestPublicUnknownOutputRequiresExplicitDegradation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Request-Id", "req_unknown")
		_, _ = fmt.Fprint(writer, `{"id":"resp","model":"test-model","status":"completed","output":[{"id":"img","type":"image_generation_call","result":"opaque"}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(server.Close)
	adapter := newPublicAdapter(t, server.URL)

	request := basePublicRequest(modelinvoker.ProtocolResponses)
	_, err := adapter.Invoke(context.Background(), request)
	if got := assertPublicErrorKind(t, err, modelinvoker.ErrorMapping); got.RequestID != "req_unknown" {
		t.Fatalf("mapping error request ID = %q", got.RequestID)
	}

	request.AllowDegradation = true
	response, err := adapter.Invoke(context.Background(), request)
	if err != nil {
		t.Fatalf("Invoke(allow degradation) error = %v", err)
	}
	if !response.MappingReport.HasDegradation() || response.RawResponse.Empty() {
		t.Fatalf("degraded response = %#v", response)
	}
}

func TestPublicUnknownResponsesStatusRequiresExplicitDegradation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Request-Id", "req_status")
		_, _ = fmt.Fprint(writer, `{"id":"resp","model":"test-model","status":"queued","output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`)
	}))
	t.Cleanup(server.Close)
	adapter := newPublicAdapter(t, server.URL)
	request := basePublicRequest(modelinvoker.ProtocolResponses)
	_, err := adapter.Invoke(context.Background(), request)
	if got := assertPublicErrorKind(t, err, modelinvoker.ErrorMapping); got.RequestID != "req_status" {
		t.Fatalf("mapping error request ID = %q", got.RequestID)
	}
	request.AllowDegradation = true
	response, err := adapter.Invoke(context.Background(), request)
	if err != nil || response.Status != modelinvoker.ResponseStatusInProgress || !response.MappingReport.HasDegradation() {
		t.Fatalf("degraded queued response/error = %#v / %v", response, err)
	}
}

func TestPublicAdapterRejectsChatStateWithoutNativeCall(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	t.Cleanup(server.Close)
	request := basePublicRequest(modelinvoker.ProtocolChatCompletions)
	request.State = &modelinvoker.State{
		Kind: modelinvoker.StateServerContinuation, Provider: openaiadapter.ProviderID,
		Protocol: modelinvoker.ProtocolChatCompletions, ID: "resp_previous",
	}
	_, err := newPublicAdapter(t, server.URL).Invoke(context.Background(), request)
	assertPublicErrorKind(t, err, modelinvoker.ErrorMapping)
	if calls.Load() != 0 {
		t.Fatalf("native calls = %d, want 0", calls.Load())
	}
}

func TestPublicAdapterEnforcesOpenAISpecificValidationBeforeNativeCall(t *testing.T) {
	t.Parallel()

	tooManyMetadata := modelinvoker.Metadata{}
	for index := 0; index < 17; index++ {
		tooManyMetadata[fmt.Sprintf("key_%d", index)] = "value"
	}
	tests := []struct {
		name   string
		mutate func(*modelinvoker.Request)
	}{
		{name: "metadata count", mutate: func(request *modelinvoker.Request) { request.Metadata = tooManyMetadata }},
		{name: "metadata key length", mutate: func(request *modelinvoker.Request) {
			request.Metadata = modelinvoker.Metadata{strings.Repeat("k", 65): "value"}
		}},
		{name: "metadata value length", mutate: func(request *modelinvoker.Request) {
			request.Metadata = modelinvoker.Metadata{"key": strings.Repeat("v", 513)}
		}},
		{name: "tool name", mutate: func(request *modelinvoker.Request) {
			request.Tools = []modelinvoker.Tool{{Name: "lookup.tool", Parameters: json.RawMessage(`{"type":"object"}`)}}
		}},
		{name: "function call ID", mutate: func(request *modelinvoker.Request) {
			request.Input = []modelinvoker.InputItem{modelinvoker.FunctionCallInput("", "lookup", json.RawMessage(`{}`))}
		}},
		{name: "function call name", mutate: func(request *modelinvoker.Request) {
			request.Input = []modelinvoker.InputItem{modelinvoker.FunctionCallInput("call", "lookup.tool", json.RawMessage(`{}`))}
		}},
		{name: "function result call ID", mutate: func(request *modelinvoker.Request) {
			request.Input = []modelinvoker.InputItem{modelinvoker.NamedFunctionResultInput("", "lookup", "done", false)}
		}},
		{name: "output name", mutate: func(request *modelinvoker.Request) {
			request.Output = modelinvoker.OutputConstraint{
				Type: modelinvoker.OutputJSONSchema, Name: "answer.schema", Schema: json.RawMessage(`{"type":"object"}`),
			}
		}},
		{name: "provider continuation", mutate: func(request *modelinvoker.Request) {
			request.State = &modelinvoker.State{
				Kind: modelinvoker.StateProviderContinuation, Provider: openaiadapter.ProviderID,
				Protocol: modelinvoker.ProtocolResponses, Payload: modelinvoker.NewRawPayload([]byte(`{"opaque":true}`)),
			}
		}},
		{name: "reasoning budget", mutate: func(request *modelinvoker.Request) {
			budget := int64(1024)
			request.Reasoning = &modelinvoker.Reasoning{BudgetTokens: &budget}
		}},
		{name: "max reasoning effort", mutate: func(request *modelinvoker.Request) {
			request.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortMax}
		}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
			t.Cleanup(server.Close)
			request := basePublicRequest(modelinvoker.ProtocolResponses)
			test.mutate(&request)
			_, err := newPublicAdapter(t, server.URL).Invoke(context.Background(), request)
			assertPublicErrorKind(t, err, modelinvoker.ErrorMapping)
			if calls.Load() != 0 {
				t.Fatalf("native calls = %d, want 0", calls.Load())
			}
		})
	}
}

func TestPublicIncompleteResponsesExposeStopReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		protocol modelinvoker.Protocol
		body     string
	}{
		{
			protocol: modelinvoker.ProtocolResponses,
			body:     `{"id":"resp","model":"test-model","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`,
		},
		{
			protocol: modelinvoker.ProtocolChatCompletions,
			body:     `{"id":"chat","model":"test-model","choices":[{"index":0,"finish_reason":"length","message":{"content":"partial"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.protocol), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(writer, test.body)
			}))
			t.Cleanup(server.Close)
			response, err := newPublicAdapter(t, server.URL).Invoke(context.Background(), basePublicRequest(test.protocol))
			if err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}
			if response.Status != modelinvoker.ResponseStatusIncomplete || response.StopReason != modelinvoker.StopReasonMaxOutputTokens {
				t.Fatalf("status/stop reason = %q/%q", response.Status, response.StopReason)
			}
		})
	}
}

func TestPublicInvalidNativeFunctionCallIsRejectedWithRequestID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		protocol modelinvoker.Protocol
		body     string
	}{
		{modelinvoker.ProtocolResponses, `{"id":"resp","model":"test-model","status":"completed","output":[{"id":"item","type":"function_call","call_id":"call","name":"lookup","arguments":"[]"}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`},
		{modelinvoker.ProtocolChatCompletions, `{"id":"chat","model":"test-model","choices":[{"index":0,"finish_reason":"tool_calls","message":{"tool_calls":[{"id":"call","type":"function","function":{"name":"lookup","arguments":"[]"}}]}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.protocol), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("X-Request-Id", "req_bad_call")
				_, _ = fmt.Fprint(writer, test.body)
			}))
			t.Cleanup(server.Close)
			_, err := newPublicAdapter(t, server.URL).Invoke(context.Background(), basePublicRequest(test.protocol))
			if got := assertPublicErrorKind(t, err, modelinvoker.ErrorMapping); got.RequestID != "req_bad_call" {
				t.Fatalf("mapping error request ID = %q", got.RequestID)
			}
		})
	}
}

func TestPublicResponseLevelFailuresAndRefusals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		protocol modelinvoker.Protocol
		body     string
		wantKind modelinvoker.ErrorKind
	}{
		{
			name: "Responses refusal", protocol: modelinvoker.ProtocolResponses, wantKind: modelinvoker.ErrorPolicyRejected,
			body: `{"id":"resp","model":"test-model","status":"completed","output":[{"id":"msg","type":"message","content":[{"type":"refusal","refusal":"blocked"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`,
		},
		{
			name: "Responses failed status", protocol: modelinvoker.ProtocolResponses, wantKind: modelinvoker.ErrorRateLimit,
			body: `{"id":"resp","model":"test-model","status":"failed","error":{"code":"rate_limit_exceeded","message":"limited"},"output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`,
		},
		{
			name: "Chat refusal", protocol: modelinvoker.ProtocolChatCompletions, wantKind: modelinvoker.ErrorPolicyRejected,
			body: `{"id":"chat","model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"refusal":"blocked"}}],"usage":{"prompt_tokens":1,"completion_tokens":0,"total_tokens":1}}`,
		},
		{
			name: "Chat content filter", protocol: modelinvoker.ProtocolChatCompletions, wantKind: modelinvoker.ErrorPolicyRejected,
			body: `{"id":"chat","model":"test-model","choices":[{"index":0,"finish_reason":"content_filter","message":{}}],"usage":{"prompt_tokens":1,"completion_tokens":0,"total_tokens":1}}`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("X-Request-Id", "req_terminal")
				_, _ = fmt.Fprint(writer, test.body)
			}))
			t.Cleanup(server.Close)
			adapter := newPublicAdapter(t, server.URL)
			response, err := adapter.Invoke(context.Background(), basePublicRequest(test.protocol))
			got := assertPublicErrorKind(t, err, test.wantKind)
			if got.RequestID != "req_terminal" {
				t.Fatalf("request ID = %q", got.RequestID)
			}
			wantStopReason := modelinvoker.StopReasonContentFilter
			if test.name == "Responses failed status" {
				wantStopReason = modelinvoker.StopReasonOther
			}
			if response.StopReason != wantStopReason {
				t.Fatalf("stop reason = %q, want %q", response.StopReason, wantStopReason)
			}
		})
	}
}

func fullPublicRequest(protocol modelinvoker.Protocol) modelinvoker.Request {
	strict := false
	parallel := true
	request := basePublicRequest(protocol)
	request.Instructions = []modelinvoker.Instruction{
		{Role: modelinvoker.RoleSystem, Text: "system rules"},
		{Role: modelinvoker.RoleDeveloper, Text: "developer rules"},
	}
	request.Input = []modelinvoker.InputItem{
		modelinvoker.MessageInput(modelinvoker.RoleUser, "hello"),
		modelinvoker.FunctionCallInput("call_in", "lookup", json.RawMessage(`{"city":"Paris"}`)),
		modelinvoker.FunctionResultInput("call_in", `{"temperature":21}`, false),
	}
	request.Tools = []modelinvoker.Tool{{
		Name: "lookup", Description: "look up a city", Strict: &strict,
		Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"],"additionalProperties":false}`),
	}}
	request.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceFunction, Name: "lookup"}
	request.ParallelToolCalls = &parallel
	request.Output = modelinvoker.OutputConstraint{
		Type: modelinvoker.OutputJSONSchema, Name: "answer", Description: "answer schema", Strict: &strict,
		Schema: json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}`),
	}
	request.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
	request.Budget.MaxOutputTokens = 128
	request.Metadata = modelinvoker.Metadata{"trace": "client"}
	if protocol == modelinvoker.ProtocolResponses {
		request.Reasoning.Summary = modelinvoker.ReasoningSummaryDetailed
		request.State = &modelinvoker.State{
			Kind: modelinvoker.StateServerContinuation, Provider: openaiadapter.ProviderID,
			Protocol: modelinvoker.ProtocolResponses, ID: "resp_previous",
		}
	}
	return request
}

func basePublicRequest(protocol modelinvoker.Protocol) modelinvoker.Request {
	return modelinvoker.Request{
		Provider: openaiadapter.ProviderID, Protocol: protocol, Model: "test-model",
		Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
	}
}

type publicTestHelper interface {
	Helper()
	Fatalf(string, ...any)
}

func newPublicAdapter(t publicTestHelper, serverURL string) *openaiadapter.Adapter {
	t.Helper()
	adapter, err := openaiadapter.New(openaiadapter.Config{APIKey: "test-only-key", BaseURL: serverURL + "/v1"})
	if err != nil {
		t.Fatalf("openai.New() error = %v", err)
	}
	return adapter
}

func newPublicInvoker(t *testing.T, adapter *openaiadapter.Adapter) *modelinvoker.Invoker {
	t.Helper()
	registry, err := modelinvoker.NewRegistry(adapter)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	invoker, err := modelinvoker.NewInvoker(registry)
	if err != nil {
		t.Fatalf("NewInvoker() error = %v", err)
	}
	return invoker
}

func assertPublicErrorKind(t *testing.T, err error, want modelinvoker.ErrorKind) *modelinvoker.Error {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want kind %q", want)
	}
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) {
		t.Fatalf("error = %T %v, want *modelinvoker.Error", err, err)
	}
	if invocationError.Kind != want {
		t.Fatalf("error kind = %q, want %q (error: %v)", invocationError.Kind, want, err)
	}
	return invocationError
}

type publicRequestCapture struct {
	method        string
	path          string
	authorization string
	body          map[string]any
	decodeErr     error
}

func publicMap(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value %#v has type %T, want JSON object", value, value)
	}
	return object
}

func publicSlice(t *testing.T, value any) []any {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("value %#v has type %T, want JSON array", value, value)
	}
	return items
}

func publicWant(t *testing.T, object map[string]any, key string, want any) {
	t.Helper()
	if got := object[key]; got != want {
		t.Fatalf("JSON field %q = %#v, want %#v", key, got, want)
	}
}
