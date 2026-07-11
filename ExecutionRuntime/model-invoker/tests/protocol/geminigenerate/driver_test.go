package geminigenerate_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/geminigenerate"
	"google.golang.org/genai"
)

const (
	testProvider modelinvoker.ProviderID = "acme-generate"
	testEndpoint                         = "https://gateway.example.test/v1beta"
)

type fakeDialect struct {
	validateCalls int
	failures      []protocol.Failure
}

func (d *fakeDialect) ValidateRequest(modelinvoker.Request) error {
	d.validateCalls++
	return nil
}

func (d *fakeDialect) ClassifyFailure(failure protocol.Failure) protocol.ErrorClassification {
	d.failures = append(d.failures, failure.Clone())
	return protocol.ErrorClassification{Kind: modelinvoker.ErrorProvider, Message: "safe classified failure"}
}

func (d *fakeDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	return modelinvoker.ProviderMetadata{"test-meta": headers.Get("X-Test-Meta")}
}

type fakeClient struct {
	responses   []*genai.GenerateContentResponse
	createErr   error
	stream      geminigenerate.EventStream
	streamErr   error
	createCalls int
	streamCalls int
	models      []string
	contents    [][]*genai.Content
	configs     []*genai.GenerateContentConfig
}

func (c *fakeClient) GenerateContent(_ context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, http.Header, error) {
	c.createCalls++
	c.models = append(c.models, model)
	c.contents = append(c.contents, contents)
	c.configs = append(c.configs, config)
	var response *genai.GenerateContentResponse
	if len(c.responses) != 0 {
		response = c.responses[0]
		c.responses = c.responses[1:]
	}
	return response, http.Header{"X-Request-Id": []string{"req_fake"}, "X-Test-Meta": []string{"meta"}}, c.createErr
}

func (c *fakeClient) GenerateContentStream(_ context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (geminigenerate.EventStream, http.Header, error) {
	c.streamCalls++
	c.models = append(c.models, model)
	c.contents = append(c.contents, contents)
	c.configs = append(c.configs, config)
	return c.stream, http.Header{"X-Request-Id": []string{"req_stream"}, "X-Test-Meta": []string{"meta"}}, c.streamErr
}

type fakeStream struct {
	responses []*genai.GenerateContentResponse
	index     int
	closed    int
	terminal  error
}

func (s *fakeStream) Next() bool {
	if s.index >= len(s.responses) {
		return false
	}
	s.index++
	return true
}

func (s *fakeStream) Current() *genai.GenerateContentResponse {
	if s.index == 0 || s.index > len(s.responses) {
		return nil
	}
	return s.responses[s.index-1]
}

func (s *fakeStream) Err() error { return s.terminal }

func (s *fakeStream) Close() error {
	s.closed++
	return nil
}

func TestDriverPreservesThoughtSignatureContinuationAndBindingIdentity(t *testing.T) {
	first := decodeResponse(t, `{
		"candidates":[{"content":{"role":"model","parts":[
			{"text":"use the tool","thought":true,"thoughtSignature":"c2lnX3Rob3VnaHQ="},
			{"functionCall":{"id":"call_1","name":"lookup","args":{"city":"Rome"}},"thoughtSignature":"c2lnX2NhbGw="}
		]},"finishReason":"STOP","index":0}],
		"modelVersion":"served-version","responseId":"resp_1",
		"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2,"thoughtsTokenCount":1,"totalTokenCount":7}
	}`)
	second := decodeResponse(t, `{
		"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},"finishReason":"STOP","index":0}],
		"modelVersion":"served-version","responseId":"resp_2",
		"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}
	}`)
	client := &fakeClient{responses: []*genai.GenerateContentResponse{first, second}}
	dialect := &fakeDialect{}
	driver := mustDriver(t, dialect, client)

	request := validRequest()
	request.Tools = lookupTools()
	response, err := driver.Invoke(context.Background(), request)
	if err != nil {
		t.Fatalf("first Invoke() error = %v", err)
	}
	if client.createCalls != 1 || dialect.validateCalls != 1 || client.models[0] != "test-model" {
		t.Fatalf("client/dialect calls/model = %d/%d/%q", client.createCalls, dialect.validateCalls, client.models[0])
	}
	if response.Provider != testProvider || response.Protocol != modelinvoker.ProtocolGenerateContent ||
		response.MappingReport.Provider != testProvider || response.MappingReport.Endpoint != testEndpoint ||
		response.RequestID != "req_fake" || response.ProviderMetadata["test-meta"] != "meta" {
		t.Fatalf("response identity/metadata = %#v", response)
	}
	if response.State == nil || response.State.Kind != modelinvoker.StateProviderContinuation ||
		response.State.Provider != testProvider || response.State.Protocol != modelinvoker.ProtocolGenerateContent || response.State.ID != "resp_1" {
		t.Fatalf("continuation identity = %#v", response.State)
	}
	payload := string(response.State.Payload.Bytes())
	if !strings.Contains(payload, "thoughtSignature") || !strings.Contains(payload, "call_1") {
		t.Fatalf("continuation payload = %s", payload)
	}
	calls := response.FunctionCalls()
	if len(calls) != 1 || calls[0].ID != "call_1" || calls[0].Name != "lookup" || string(calls[0].Arguments) != `{"city":"Rome"}` {
		t.Fatalf("normalized calls = %#v", calls)
	}
	if response.Usage.ReasoningTokens != 1 || response.StopReason != modelinvoker.StopReasonToolCall {
		t.Fatalf("usage/stop = %#v/%q", response.Usage, response.StopReason)
	}

	continued := validRequest()
	continued.Input = []modelinvoker.InputItem{modelinvoker.FunctionResultInput("call_1", "sunny", false)}
	continued.Tools = lookupTools()
	continued.State = response.State
	final, err := driver.Invoke(context.Background(), continued)
	if err != nil {
		t.Fatalf("continued Invoke() error = %v", err)
	}
	if final.Text() != "done" || final.State == nil || final.State.Provider != testProvider || client.createCalls != 2 {
		t.Fatalf("continued response/calls = %#v / %d", final, client.createCalls)
	}
	encoded, err := json.Marshal(client.contents[1])
	if err != nil {
		t.Fatal(err)
	}
	var contents []any
	if err := json.Unmarshal(encoded, &contents); err != nil {
		t.Fatal(err)
	}
	if len(contents) != 3 {
		t.Fatalf("continued contents = %#v", contents)
	}
	modelParts := contents[1].(map[string]any)["parts"].([]any)
	if len(modelParts) != 2 || modelParts[0].(map[string]any)["thoughtSignature"] == nil || modelParts[1].(map[string]any)["thoughtSignature"] == nil {
		t.Fatalf("replayed model parts = %#v", modelParts)
	}
	functionResponse := contents[2].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
	if functionResponse["id"] != "call_1" || functionResponse["name"] != "lookup" {
		t.Fatalf("function response = %#v", functionResponse)
	}
}

func TestDriverRejectsWrongStateKindBeforeClient(t *testing.T) {
	client := &fakeClient{}
	driver := mustDriver(t, &fakeDialect{}, client)
	request := validRequest()
	request.State = &modelinvoker.State{
		Kind: modelinvoker.StateServerContinuation, Provider: testProvider,
		Protocol: modelinvoker.ProtocolGenerateContent, ID: "opaque",
	}
	_, err := driver.Invoke(context.Background(), request)
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError.Kind != modelinvoker.ErrorMapping || invocationError.Provider != testProvider {
		t.Fatalf("state rejection = %#v", err)
	}
	if client.createCalls != 0 {
		t.Fatalf("native calls = %d, want 0", client.createCalls)
	}
}

func TestDriverFailureDropsNativeCause(t *testing.T) {
	const secret = "native-sdk-secret"
	client := &fakeClient{createErr: errors.New(secret)}
	dialect := &fakeDialect{}
	response, err := mustDriver(t, dialect, client).Invoke(context.Background(), validRequest())
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError.Provider != testProvider || invocationError.Err != nil ||
		invocationError.Message != "safe classified failure" || response.Provider != testProvider || response.Status != modelinvoker.ResponseStatusFailed {
		t.Fatalf("failure response/error = %#v / %#v", response, err)
	}
	if strings.Contains(err.Error(), secret) || len(dialect.failures) != 1 || dialect.failures[0].Source != protocol.FailureSourceSDK {
		t.Fatalf("unsafe failure handoff = %v / %#v", err, dialect.failures)
	}
}

func TestDriverStreamPreservesDedupedToolStateTerminalIdentityAndClose(t *testing.T) {
	responses := []*genai.GenerateContentResponse{
		decodeResponse(t, `{"candidates":[{"content":{"role":"model","parts":[{"text":"hello "}]},"index":0}],"modelVersion":"served-version","responseId":"resp_stream","usageMetadata":{"promptTokenCount":2,"totalTokenCount":2}}`),
		decodeResponse(t, `{"candidates":[{"content":{"role":"model","parts":[{"text":"thinking","thought":true,"thoughtSignature":"c2lnX3Rob3VnaHQ="}]},"index":0}],"modelVersion":"served-version","responseId":"resp_stream"}`),
		decodeResponse(t, `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call_stream","name":"lookup","args":{"city":"Rome"}},"thoughtSignature":"c2lnX2NhbGw="}]},"index":0}],"modelVersion":"served-version","responseId":"resp_stream"}`),
		decodeResponse(t, `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call_stream","name":"lookup","args":{"city":"Rome"}},"thoughtSignature":"c2lnX2NhbGw="}]},"finishReason":"STOP","index":0}],"modelVersion":"served-version","responseId":"resp_stream","usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":2,"thoughtsTokenCount":1,"totalTokenCount":5}}`),
	}
	native := &fakeStream{responses: responses}
	client := &fakeClient{stream: native}
	request := validRequest()
	request.Tools = lookupTools()
	stream, err := mustDriver(t, &fakeDialect{}, client).Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var previous int64
	var terminal *modelinvoker.Response
	completedCalls := 0
	for stream.Next() {
		event := stream.Event()
		if event.Sequence <= previous {
			t.Fatalf("sequence %d after %d", event.Sequence, previous)
		}
		previous = event.Sequence
		if event.Type == modelinvoker.StreamEventFunctionCallCompleted {
			completedCalls++
		}
		if event.Response != nil && event.Type == modelinvoker.StreamEventResponseCompleted {
			terminal = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if completedCalls != 1 || terminal == nil || terminal.Provider != testProvider || terminal.Protocol != modelinvoker.ProtocolGenerateContent ||
		terminal.MappingReport.Endpoint != testEndpoint || terminal.Text() != "hello " || terminal.State == nil || terminal.State.Provider != testProvider ||
		len(terminal.FunctionCalls()) != 1 || terminal.Usage.TotalTokens != 5 {
		t.Fatalf("terminal response/calls = %#v / %d", terminal, completedCalls)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if native.closed != 1 {
		t.Fatalf("native Close calls = %d", native.closed)
	}
}

func TestNewRejectsWrongBindingAndNilClients(t *testing.T) {
	wrong, err := protocol.NewBinding(testProvider, modelinvoker.ProtocolMessages, testEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := geminigenerate.New(wrong, &fakeDialect{}, &fakeClient{}); err == nil {
		t.Fatal("New(wrong protocol) error = nil")
	}
	if _, err := geminigenerate.New(mustBinding(t), &fakeDialect{}, nil); err == nil {
		t.Fatal("New(nil client) error = nil")
	}
	var typedNil *fakeClient
	if _, err := geminigenerate.New(mustBinding(t), &fakeDialect{}, typedNil); err == nil {
		t.Fatal("New(typed-nil client) error = nil")
	}
}

func mustDriver(t *testing.T, dialect protocol.Dialect, client geminigenerate.Client) *geminigenerate.Driver {
	t.Helper()
	driver, err := geminigenerate.New(mustBinding(t), dialect, client)
	if err != nil {
		t.Fatal(err)
	}
	return driver
}

func mustBinding(t *testing.T) protocol.Binding {
	t.Helper()
	binding, err := protocol.NewBinding(testProvider, modelinvoker.ProtocolGenerateContent, testEndpoint, "x-request-id")
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func validRequest() modelinvoker.Request {
	return modelinvoker.Request{
		Provider: testProvider, Protocol: modelinvoker.ProtocolGenerateContent,
		Endpoint: testEndpoint, Model: "test-model",
		Input:  []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
		Budget: modelinvoker.Budget{MaxOutputTokens: 128},
	}
}

func lookupTools() []modelinvoker.Tool {
	return []modelinvoker.Tool{{
		Name: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"],"additionalProperties":false}`),
	}}
}

func decodeResponse(t testing.TB, raw string) *genai.GenerateContentResponse {
	t.Helper()
	var value genai.GenerateContentResponse
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return &value
}

var (
	_ protocol.Dialect           = (*fakeDialect)(nil)
	_ geminigenerate.Client      = (*fakeClient)(nil)
	_ geminigenerate.EventStream = (*fakeStream)(nil)
)
