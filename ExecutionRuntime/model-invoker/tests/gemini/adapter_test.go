package gemini_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/gemini"
)

type capturedRequest struct {
	method string
	path   string
	query  string
	key    string
	body   map[string]any
	err    error
}

func TestGenerateContentRequestResponseCapabilitiesAndRawSafety(t *testing.T) {
	t.Setenv("GOOGLE_GENAI_USE_VERTEXAI", "true")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GOOGLE_GEMINI_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("GOOGLE_VERTEX_BASE_URL", "http://127.0.0.1:2")
	fixture := mustFixture(t, "testdata/response-text.json")
	captured := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		data, readErr := io.ReadAll(request.Body)
		var body map[string]any
		decodeErr := json.Unmarshal(data, &body)
		if readErr != nil {
			decodeErr = readErr
		}
		captured <- capturedRequest{
			method: request.Method, path: request.URL.Path, query: request.URL.RawQuery,
			key: request.Header.Get("x-goog-api-key"), body: body, err: decodeErr,
		}
		writer.Header().Set("x-goog-request-id", "req_gemini_text_01")
		writer.Header().Set("x-goog-quota-project", "project-a")
		writeJSON(writer, http.StatusOK, fixture)
	}))
	defer server.Close()

	adapter := newAdapter(t, server.URL, server.Client())
	strict := true
	parallel := false
	request := baseRequest()
	request.Endpoint = server.URL + "/v1beta/"
	request.AllowDegradation = true
	request.Instructions = []modelinvoker.Instruction{
		{Role: modelinvoker.RoleSystem, Text: "System rule"},
		{Role: modelinvoker.RoleDeveloper, Text: "Developer rule"},
	}
	request.Input = []modelinvoker.InputItem{
		modelinvoker.MessageInput(modelinvoker.RoleUser, "first"),
		modelinvoker.MessageInput(modelinvoker.RoleUser, "second"),
	}
	request.Tools = weatherTools()
	request.Tools[0].Strict = &strict
	request.ToolChoice = modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceFunction, Name: "get_weather"}
	request.ParallelToolCalls = &parallel
	request.Output = modelinvoker.OutputConstraint{
		Type: modelinvoker.OutputJSONSchema, Name: "answer", Description: "Answer payload",
		Schema: json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}`),
		Strict: &strict,
	}
	request.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh, Summary: modelinvoker.ReasoningSummaryAuto}
	request.Metadata = modelinvoker.Metadata{"trace": "local-only"}

	response, err := adapter.Invoke(context.Background(), request)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	capture := <-captured
	if capture.err != nil {
		t.Fatalf("decode native request: %v", capture.err)
	}
	if capture.method != http.MethodPost || capture.path != "/v1beta/models/gemini-test-model:generateContent" || capture.query != "" {
		t.Fatalf("native request = %s %s?%s", capture.method, capture.path, capture.query)
	}
	if capture.key != testAPIKey {
		t.Fatalf("x-goog-api-key = %q", capture.key)
	}

	contents := array(t, capture.body["contents"])
	if len(contents) != 1 {
		t.Fatalf("contents count = %d, want adjacent user messages combined", len(contents))
	}
	user := object(t, contents[0])
	wantValue(t, user, "role", "user")
	parts := array(t, user["parts"])
	if len(parts) != 2 || object(t, parts[0])["text"] != "first" || object(t, parts[1])["text"] != "second" {
		t.Fatalf("combined user parts = %#v", parts)
	}
	system := object(t, capture.body["systemInstruction"])
	if len(array(t, system["parts"])) != 2 {
		t.Fatalf("systemInstruction = %#v", system)
	}
	declaration := object(t, array(t, object(t, array(t, capture.body["tools"])[0])["functionDeclarations"])[0])
	wantValue(t, declaration, "name", "get_weather")
	toolConfig := object(t, object(t, capture.body["toolConfig"])["functionCallingConfig"])
	wantValue(t, toolConfig, "mode", "ANY")
	generation := object(t, capture.body["generationConfig"])
	wantValue(t, generation, "maxOutputTokens", float64(128))
	wantValue(t, generation, "responseMimeType", "application/json")
	if object(t, generation["responseJsonSchema"])["title"] != "answer" {
		t.Fatalf("responseJsonSchema = %#v", generation["responseJsonSchema"])
	}
	thinking := object(t, generation["thinkingConfig"])
	wantValue(t, thinking, "thinkingLevel", "HIGH")
	wantValue(t, thinking, "includeThoughts", true)
	if _, exists := capture.body["labels"]; exists {
		t.Fatalf("Vertex-only labels leaked into Developer API request: %#v", capture.body)
	}

	wantUsage := modelinvoker.Usage{
		InputTokens: 12, OutputTokens: 8, ReasoningTokens: 3,
		CacheReadTokens: 4, TotalTokens: 20,
	}
	if response.ID != "resp_gemini_text_01" || response.RequestID != "req_gemini_text_01" ||
		response.Text() != "Hello from Gemini." || response.Usage != wantUsage {
		t.Fatalf("normalized response = %#v, text = %q", response, response.Text())
	}
	if response.Status != modelinvoker.ResponseStatusCompleted || response.StopReason != modelinvoker.StopReasonEndTurn || response.State != nil {
		t.Fatalf("status/stop/state = %q/%q/%#v", response.Status, response.StopReason, response.State)
	}
	if response.ProviderMetadata["model_version"] != "gemini-test-version" || response.ProviderMetadata["x-goog-quota-project"] != "project-a" {
		t.Fatalf("provider metadata = %#v", response.ProviderMetadata)
	}
	if !response.MappingReport.HasDegradation() || len(response.MappingReport.Decisions) < 4 {
		t.Fatalf("mapping report = %#v", response.MappingReport)
	}
	if response.MappingReport.Endpoint != server.URL+"/v1beta" {
		t.Fatalf("normalized mapping endpoint = %q", response.MappingReport.Endpoint)
	}
	if response.RawRequest.String() != "[REDACTED]" || response.RawResponse.String() != "[REDACTED]" {
		t.Fatal("raw payloads are not redacted by default")
	}
	if strings.Contains(string(response.RawRequest.Bytes()), testAPIKey) || strings.Contains(string(response.RawResponse.Bytes()), testAPIKey) {
		t.Fatal("raw payload leaked API key")
	}

	contract, err := adapter.Capabilities(context.Background(), modelinvoker.CapabilityQuery{
		Protocol: modelinvoker.ProtocolGenerateContent,
		Model:    request.Model,
		Endpoint: server.URL + "/v1beta/",
	})
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	if contract[modelinvoker.CapabilityTextGeneration].Level != modelinvoker.SupportNative ||
		contract[modelinvoker.CapabilityProviderContinuation].Level != modelinvoker.SupportNative ||
		contract[modelinvoker.CapabilityReasoningSummary].Level != modelinvoker.SupportPartial ||
		contract[modelinvoker.CapabilityServerState].Level != modelinvoker.SupportUnsupported {
		t.Fatalf("capability contract = %#v", contract)
	}
}

func TestExplicitFunctionCallAndResponseHistory(t *testing.T) {
	fixture := mustFixture(t, "testdata/response-text.json")
	captured := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		data, _ := io.ReadAll(request.Body)
		var body map[string]any
		_ = json.Unmarshal(data, &body)
		captured <- body
		writeJSON(writer, http.StatusOK, fixture)
	}))
	defer server.Close()

	request := baseRequest()
	request.Input = []modelinvoker.InputItem{
		modelinvoker.MessageInput(modelinvoker.RoleUser, "Use two tools"),
		modelinvoker.FunctionCallInput("call_lookup_01", "lookup", json.RawMessage(`{"query":"Praxis"}`)),
		modelinvoker.FunctionCallInput("", "get_weather", json.RawMessage(`{"city":"Rome"}`)),
		modelinvoker.FunctionResultInput("call_lookup_01", "found", false),
		modelinvoker.NamedFunctionResultInput("", "get_weather", "upstream failed", true),
	}
	response, err := newAdapter(t, server.URL, server.Client()).Invoke(context.Background(), request)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if response.Text() != "Hello from Gemini." {
		t.Fatalf("response text = %q", response.Text())
	}

	contents := array(t, (<-captured)["contents"])
	if len(contents) != 3 {
		t.Fatalf("contents = %#v, want user/model/user grouping", contents)
	}
	modelParts := array(t, object(t, contents[1])["parts"])
	if len(modelParts) != 2 {
		t.Fatalf("model function parts = %#v", modelParts)
	}
	firstCall := object(t, object(t, modelParts[0])["functionCall"])
	wantValue(t, firstCall, "id", "call_lookup_01")
	wantValue(t, firstCall, "name", "lookup")
	secondCall := object(t, object(t, modelParts[1])["functionCall"])
	wantValue(t, secondCall, "name", "get_weather")
	if _, exists := secondCall["id"]; exists {
		t.Fatalf("ID-less function call gained a native ID: %#v", secondCall)
	}
	resultParts := array(t, object(t, contents[2])["parts"])
	firstResult := object(t, object(t, resultParts[0])["functionResponse"])
	wantValue(t, firstResult, "id", "call_lookup_01")
	wantValue(t, firstResult, "name", "lookup")
	wantValue(t, object(t, firstResult["response"]), "output", "found")
	secondResult := object(t, object(t, resultParts[1])["functionResponse"])
	wantValue(t, secondResult, "name", "get_weather")
	if _, exists := secondResult["id"]; exists {
		t.Fatalf("ID-less function response gained a native ID: %#v", secondResult)
	}
	wantValue(t, object(t, secondResult["response"]), "error", "upstream failed")
}

func TestJSONObjectAndMappingRejections(t *testing.T) {
	fixture := mustFixture(t, "testdata/response-text.json")
	var calls atomic.Int64
	captured := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		data, _ := io.ReadAll(request.Body)
		var body map[string]any
		_ = json.Unmarshal(data, &body)
		captured <- body
		writeJSON(writer, http.StatusOK, fixture)
	}))
	defer server.Close()
	adapter := newAdapter(t, server.URL, server.Client())

	jsonObject := baseRequest()
	jsonObject.Output.Type = modelinvoker.OutputJSONObject
	if _, err := adapter.Invoke(context.Background(), jsonObject); err != nil {
		t.Fatalf("JSON object Invoke() error = %v", err)
	}
	generation := object(t, (<-captured)["generationConfig"])
	wantValue(t, generation, "responseMimeType", "application/json")
	if _, exists := generation["responseJsonSchema"]; exists {
		t.Fatalf("json_object unexpectedly sent schema: %#v", generation)
	}

	tests := []struct {
		name   string
		mutate func(*modelinvoker.Request)
	}{
		{name: "unsupported schema keyword", mutate: func(request *modelinvoker.Request) {
			request.Output = modelinvoker.OutputConstraint{Type: modelinvoker.OutputJSONSchema, Name: "bad", Schema: json.RawMessage(`{"type":"object","patternProperties":{}}`)}
		}},
		{name: "budget and level", mutate: func(request *modelinvoker.Request) {
			budget := int64(64)
			request.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh, BudgetTokens: &budget}
		}},
		{name: "metadata without degradation", mutate: func(request *modelinvoker.Request) {
			request.Metadata = modelinvoker.Metadata{"trace": "not-supported"}
		}},
		{name: "foreign provider options", mutate: func(request *modelinvoker.Request) {
			request.ProviderOptions = modelinvoker.ProviderOptions{provider.ProviderID: json.RawMessage(`{"candidateCount":2}`)}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := baseRequest()
			test.mutate(&request)
			if _, err := adapter.Invoke(context.Background(), request); err == nil {
				t.Fatal("Invoke() error = nil")
			}
		})
	}
	if calls.Load() != 1 {
		t.Fatalf("native calls = %d, want only JSON object request", calls.Load())
	}
}

func TestReasoningBudgetAndUsageTotalFallback(t *testing.T) {
	captured := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		data, _ := io.ReadAll(request.Body)
		var body map[string]any
		_ = json.Unmarshal(data, &body)
		captured <- body
		writeJSON(writer, http.StatusOK, []byte(`{
			"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP","index":0}],
			"responseId":"resp_usage_fallback",
			"usageMetadata":{"promptTokenCount":2,"toolUsePromptTokenCount":1,"candidatesTokenCount":2,"thoughtsTokenCount":1,"cachedContentTokenCount":1}
		}`))
	}))
	defer server.Close()

	budget := int64(256)
	request := baseRequest()
	request.Reasoning = &modelinvoker.Reasoning{BudgetTokens: &budget}
	response, err := newAdapter(t, server.URL, server.Client()).Invoke(context.Background(), request)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	thinking := object(t, object(t, (<-captured)["generationConfig"])["thinkingConfig"])
	wantValue(t, thinking, "thinkingBudget", float64(256))
	wantUsage := modelinvoker.Usage{
		InputTokens: 3, OutputTokens: 3, ReasoningTokens: 1,
		CacheReadTokens: 1, TotalTokens: 6,
	}
	if response.Usage != wantUsage {
		t.Fatalf("usage = %#v, want %#v", response.Usage, wantUsage)
	}
	if len(response.MappingReport.Decisions) != 1 || response.MappingReport.Decisions[0].Action != modelinvoker.MappingTransformed {
		t.Fatalf("usage fallback decision = %#v", response.MappingReport)
	}
}
