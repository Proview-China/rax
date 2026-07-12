package openaichat_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	openaisdk "github.com/openai/openai-go/v3"
)

const (
	testProvider modelinvoker.ProviderID = "acme-hosted"
	testEndpoint                         = "https://gateway.example.test/v1"
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
	return protocol.ErrorClassification{
		Kind: modelinvoker.ErrorProvider, Code: failure.Code, Message: "safe classified failure",
	}
}

func (d *fakeDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	return modelinvoker.ProviderMetadata{"test-meta": headers.Get("X-Test-Meta")}
}

type fakeClient struct {
	response    *openaisdk.ChatCompletion
	createErr   error
	stream      openaichat.EventStream
	createCalls int
	streamCalls int
	params      openaisdk.ChatCompletionNewParams
}

func (c *fakeClient) Create(_ context.Context, params openaisdk.ChatCompletionNewParams) (*openaisdk.ChatCompletion, http.Header, error) {
	c.createCalls++
	c.params = params
	return c.response, http.Header{"X-Request-Id": []string{"req_fake"}, "X-Test-Meta": []string{"meta"}}, c.createErr
}

func (c *fakeClient) Stream(_ context.Context, params openaisdk.ChatCompletionNewParams) (openaichat.EventStream, http.Header) {
	c.streamCalls++
	c.params = params
	return c.stream, http.Header{"X-Request-Id": []string{"req_stream"}, "X-Test-Meta": []string{"meta"}}
}

type fakeStream struct {
	events   []openaisdk.ChatCompletionChunk
	index    int
	closed   int
	terminal error
	closeErr error
}

func (s *fakeStream) Next() bool {
	if s.index >= len(s.events) {
		return false
	}
	s.index++
	return true
}

func (s *fakeStream) Current() openaisdk.ChatCompletionChunk {
	if s.index == 0 || s.index > len(s.events) {
		return openaisdk.ChatCompletionChunk{}
	}
	return s.events[s.index-1]
}

func (s *fakeStream) Err() error { return s.terminal }

func (s *fakeStream) Close() error {
	s.closed++
	return s.closeErr
}

func TestDriverUsesInjectedIdentityAndMapsInvoke(t *testing.T) {
	native := decodeCompletion(t, `{"id":"chat_1","model":"served-model","choices":[{"index":0,"finish_reason":"stop","message":{"content":"hello"}}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)
	client := &fakeClient{response: &native}
	dialect := &fakeDialect{}
	driver := mustDriver(t, dialect, client)
	request := validRequest()
	request.Instructions = []modelinvoker.Instruction{{Role: modelinvoker.RoleSystem, Text: "system"}}
	response, err := driver.Invoke(context.Background(), request)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if client.createCalls != 1 || dialect.validateCalls != 1 {
		t.Fatalf("client/dialect calls = %d/%d", client.createCalls, dialect.validateCalls)
	}
	if client.params.Model != openaisdk.ChatModel(request.Model) || len(client.params.Messages) != 2 {
		t.Fatalf("mapped params = %#v", client.params)
	}
	if response.Provider != testProvider || response.Protocol != modelinvoker.ProtocolChatCompletions ||
		response.MappingReport.Provider != testProvider || response.MappingReport.Endpoint != testEndpoint ||
		response.Text() != "hello" || response.RequestID != "req_fake" || response.ProviderMetadata["test-meta"] != "meta" {
		t.Fatalf("response = %#v", response)
	}
	if response.RawRequest.Empty() || response.RawResponse.Empty() {
		t.Fatalf("audit payloads = request:%v response:%v", response.RawRequest.Empty(), response.RawResponse.Empty())
	}
}

func TestDriverRejectsStateBeforeClient(t *testing.T) {
	client := &fakeClient{}
	driver := mustDriver(t, &fakeDialect{}, client)
	request := validRequest()
	request.State = &modelinvoker.State{
		Kind: modelinvoker.StateServerContinuation, Provider: testProvider,
		Protocol: modelinvoker.ProtocolChatCompletions, ID: "previous",
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
	driver := mustDriver(t, dialect, client)
	response, err := driver.Invoke(context.Background(), validRequest())
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError.Provider != testProvider || invocationError.Err != nil ||
		invocationError.Message != "safe classified failure" || response.Provider != testProvider || response.Status != modelinvoker.ResponseStatusFailed {
		t.Fatalf("failure response/error = %#v / %#v", response, err)
	}
	if strings.Contains(err.Error(), secret) || len(dialect.failures) != 1 || dialect.failures[0].Source != protocol.FailureSourceSDK {
		t.Fatalf("unsafe or missing failure handoff = %v / %#v", err, dialect.failures)
	}
}

func TestDriverInvokeRejectsMissingAndMismatchedAuthoritativeModel(t *testing.T) {
	for _, test := range []struct{ name, actual, code string }{
		{"missing", "", "response_model_missing"},
		{"mismatch", "other-model", "response_model_mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			native := decodeCompletion(t, `{"id":"untrusted","model":"`+test.actual+`","choices":[{"index":0,"finish_reason":"stop","message":{"content":"untrusted"}}]}`)
			response, err := mustDriver(t, &fakeDialect{}, &fakeClient{response: &native}).Invoke(context.Background(), validRequest())
			var invocationError *modelinvoker.Error
			if !errors.As(err, &invocationError) || invocationError.Code != test.code || response.Status != modelinvoker.ResponseStatusFailed || len(response.Output) != 0 || !response.RawResponse.Empty() {
				t.Fatalf("authoritative model rejection = %#v / %v", response, err)
			}
		})
	}
}

func TestDriverStreamPreservesOrderUsageTerminalAndIdentity(t *testing.T) {
	native := &fakeStream{events: []openaisdk.ChatCompletionChunk{
		decodeChunk(t, `{"id":"chat_stream","model":"served-model","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":"stop"}]}`),
		decodeChunk(t, `{"id":"chat_stream","model":"served-model","choices":[],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`),
	}}
	client := &fakeClient{stream: native}
	driver := mustDriver(t, &fakeDialect{}, client)
	stream, err := driver.Stream(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	var types []modelinvoker.StreamEventType
	var terminal *modelinvoker.Response
	var previous int64
	for stream.Next() {
		event := stream.Event()
		if event.Sequence <= previous {
			t.Fatalf("non-monotonic sequence %d after %d", event.Sequence, previous)
		}
		previous = event.Sequence
		types = append(types, event.Type)
		if event.Response != nil {
			terminal = event.Response
			if event.Response.Provider != testProvider || event.Response.MappingReport.Endpoint != testEndpoint {
				t.Fatalf("stream response identity = %#v", event.Response)
			}
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err() = %v", err)
	}
	if len(types) < 4 || types[len(types)-2] != modelinvoker.StreamEventUsage || types[len(types)-1] != modelinvoker.StreamEventResponseCompleted {
		t.Fatalf("event order = %v", types)
	}
	if terminal == nil || terminal.Text() != "hello" || terminal.Usage.TotalTokens != 3 || terminal.RequestID != "req_stream" {
		t.Fatalf("terminal response = %#v", terminal)
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

func TestDriverStreamModelIdentityFailureClosesOnceWithoutPayloadOrCloseLeak(t *testing.T) {
	for _, test := range []struct{ name, actual, code string }{
		{"missing", "", "response_model_missing"},
		{"mismatch", "other-model", "response_model_mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			const sentinel = "CHAT-CLOSE-SECRET-MUST-NOT-LEAK"
			closeFailure := errors.New(sentinel)
			native := &fakeStream{events: []openaisdk.ChatCompletionChunk{
				decodeChunk(t, `{"id":"untrusted","model":"`+test.actual+`","choices":[{"index":0,"delta":{"content":"untrusted"}}]}`),
			}, closeErr: closeFailure}
			stream, err := mustDriver(t, &fakeDialect{}, &fakeClient{stream: native}).Stream(context.Background(), validRequest())
			if err != nil {
				t.Fatal(err)
			}
			assertIdentityFailureStream(t, stream, native, test.code, closeFailure, sentinel)
		})
	}
}

func assertIdentityFailureStream(t *testing.T, stream modelinvoker.Stream, native *fakeStream, code string, closeFailure error, sentinel string) {
	t.Helper()
	if !stream.Next() {
		t.Fatal("identity failure Next() = false")
	}
	event := stream.Event()
	if event.Type != modelinvoker.StreamEventError || event.Error == nil || event.Error.Code != code || event.Response != nil || !event.Raw.Empty() {
		t.Fatalf("identity failure event = %#v", event)
	}
	if err := stream.Err(); !errors.Is(err, closeFailure) || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("identity failure Err() lost or leaked close cause: %v", err)
	}
	if err := stream.Close(); !errors.Is(err, closeFailure) || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("identity failure Close() lost or leaked close cause: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
	if native.closed != 1 {
		t.Fatalf("native Close calls = %d, want 1", native.closed)
	}
}

func TestNewRejectsWrongBindingAndNilClients(t *testing.T) {
	wrong, err := protocol.NewBinding(testProvider, modelinvoker.ProtocolResponses, testEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openaichat.New(wrong, &fakeDialect{}, &fakeClient{}); err == nil {
		t.Fatal("New(wrong protocol) error = nil")
	}
	binding := mustBinding(t)
	if _, err := openaichat.New(binding, &fakeDialect{}, nil); err == nil {
		t.Fatal("New(nil client) error = nil")
	}
	var typedNil *fakeClient
	if _, err := openaichat.New(binding, &fakeDialect{}, typedNil); err == nil {
		t.Fatal("New(typed-nil client) error = nil")
	}
}

func mustDriver(t *testing.T, dialect protocol.Dialect, client openaichat.Client) *openaichat.Driver {
	t.Helper()
	driver, err := openaichat.New(mustBinding(t), dialect, client)
	if err != nil {
		t.Fatal(err)
	}
	return driver
}

func mustBinding(t *testing.T) protocol.Binding {
	t.Helper()
	binding, err := protocol.NewBinding(testProvider, modelinvoker.ProtocolChatCompletions, testEndpoint, "x-request-id")
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func validRequest() modelinvoker.Request {
	return modelinvoker.Request{
		Provider: testProvider, Protocol: modelinvoker.ProtocolChatCompletions,
		Endpoint: testEndpoint, Model: "served-model",
		Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
	}
}

func decodeCompletion(t *testing.T, raw string) openaisdk.ChatCompletion {
	t.Helper()
	var value openaisdk.ChatCompletion
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func decodeChunk(t *testing.T, raw string) openaisdk.ChatCompletionChunk {
	t.Helper()
	var value openaisdk.ChatCompletionChunk
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

var (
	_ protocol.Dialect       = (*fakeDialect)(nil)
	_ openaichat.Client      = (*fakeClient)(nil)
	_ openaichat.EventStream = (*fakeStream)(nil)
)
