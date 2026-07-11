package openairesponses_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openairesponses"
	"github.com/openai/openai-go/v3/responses"
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
	return protocol.ErrorClassification{Kind: modelinvoker.ErrorProvider, Message: "safe classified failure"}
}

func (d *fakeDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	return modelinvoker.ProviderMetadata{"test-meta": headers.Get("X-Test-Meta")}
}

type fakeClient struct {
	response    *responses.Response
	createErr   error
	stream      openairesponses.EventStream
	createCalls int
	streamCalls int
	params      responses.ResponseNewParams
}

func (c *fakeClient) Create(_ context.Context, params responses.ResponseNewParams) (*responses.Response, http.Header, error) {
	c.createCalls++
	c.params = params
	return c.response, http.Header{"X-Request-Id": []string{"req_fake"}, "X-Test-Meta": []string{"meta"}}, c.createErr
}

func (c *fakeClient) Stream(_ context.Context, params responses.ResponseNewParams) (openairesponses.EventStream, http.Header) {
	c.streamCalls++
	c.params = params
	return c.stream, http.Header{"X-Request-Id": []string{"req_stream"}, "X-Test-Meta": []string{"meta"}}
}

type fakeStream struct {
	events   []responses.ResponseStreamEventUnion
	index    int
	closed   int
	terminal error
}

func (s *fakeStream) Next() bool {
	if s.index >= len(s.events) {
		return false
	}
	s.index++
	return true
}

func (s *fakeStream) Current() responses.ResponseStreamEventUnion {
	if s.index == 0 || s.index > len(s.events) {
		return responses.ResponseStreamEventUnion{}
	}
	return s.events[s.index-1]
}

func (s *fakeStream) Err() error { return s.terminal }

func (s *fakeStream) Close() error {
	s.closed++
	return nil
}

func TestDriverPreservesTypedItemsAndServerContinuation(t *testing.T) {
	native := decodeResponse(t, `{
		"id":"resp_1","model":"served-model","status":"completed",
		"output":[
			{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello","annotations":[]}]},
			{"id":"item_call","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"city\":\"Rome\"}","status":"completed"},
			{"id":"reason_1","type":"reasoning","summary":[{"type":"summary_text","text":"checked"}]}
		],
		"usage":{"input_tokens":4,"output_tokens":3,"output_tokens_details":{"reasoning_tokens":1},"total_tokens":7}
	}`)
	client := &fakeClient{response: &native}
	dialect := &fakeDialect{}
	driver := mustDriver(t, dialect, client)
	request := validRequest()
	request.State = &modelinvoker.State{
		Kind: modelinvoker.StateServerContinuation, Provider: testProvider,
		Protocol: modelinvoker.ProtocolResponses, ID: "resp_previous",
	}
	response, err := driver.Invoke(context.Background(), request)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if client.createCalls != 1 || dialect.validateCalls != 1 {
		t.Fatalf("client/dialect calls = %d/%d", client.createCalls, dialect.validateCalls)
	}
	encoded, err := json.Marshal(client.params)
	if err != nil {
		t.Fatal(err)
	}
	var params map[string]any
	if err := json.Unmarshal(encoded, &params); err != nil {
		t.Fatal(err)
	}
	if params["previous_response_id"] != "resp_previous" {
		t.Fatalf("previous_response_id = %#v", params["previous_response_id"])
	}
	if response.Provider != testProvider || response.Protocol != modelinvoker.ProtocolResponses ||
		response.MappingReport.Provider != testProvider || response.MappingReport.Endpoint != testEndpoint ||
		response.State == nil || response.State.Provider != testProvider || response.State.Protocol != modelinvoker.ProtocolResponses || response.State.ID != "resp_1" {
		t.Fatalf("response identity/state = %#v", response)
	}
	if response.Text() != "hello" || response.RequestID != "req_fake" || response.ProviderMetadata["test-meta"] != "meta" ||
		response.Usage.TotalTokens != 7 || response.Usage.ReasoningTokens != 1 {
		t.Fatalf("normalized response = %#v", response)
	}
	calls := response.FunctionCalls()
	if len(calls) != 1 || calls[0].ID != "call_1" || calls[0].Name != "lookup" || string(calls[0].Arguments) != `{"city":"Rome"}` {
		t.Fatalf("typed function calls = %#v", calls)
	}
	if len(response.Output) != 3 || response.Output[2].Type != modelinvoker.OutputItemReasoningSummary || response.Output[2].ReasoningSummary != "checked" {
		t.Fatalf("typed output items = %#v", response.Output)
	}
}

func TestDriverRejectsWrongStateKindBeforeClient(t *testing.T) {
	client := &fakeClient{}
	driver := mustDriver(t, &fakeDialect{}, client)
	request := validRequest()
	request.State = &modelinvoker.State{
		Kind: modelinvoker.StateProviderContinuation, Provider: testProvider,
		Protocol: modelinvoker.ProtocolResponses, ID: "opaque",
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

func TestDriverStreamPreservesNativeSequenceAndTerminalState(t *testing.T) {
	native := &fakeStream{events: []responses.ResponseStreamEventUnion{
		decodeEvent(t, `{"type":"response.created","sequence_number":4,"response":{"id":"resp_stream","model":"served-model","status":"in_progress","output":[]}}`),
		decodeEvent(t, `{"type":"response.output_text.delta","sequence_number":5,"item_id":"msg","output_index":0,"content_index":0,"delta":"hello"}`),
		decodeEvent(t, `{"type":"response.completed","sequence_number":9,"response":{"id":"resp_stream","model":"served-model","status":"completed","output":[{"id":"msg","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello","annotations":[]}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`),
	}}
	client := &fakeClient{stream: native}
	stream, err := mustDriver(t, &fakeDialect{}, client).Stream(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	var sequences []int64
	var types []modelinvoker.StreamEventType
	var terminal *modelinvoker.Response
	for stream.Next() {
		event := stream.Event()
		sequences = append(sequences, event.Sequence)
		types = append(types, event.Type)
		if event.Response != nil {
			terminal = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if len(sequences) != 4 || sequences[0] != 4 || sequences[len(sequences)-1] != 10 ||
		types[len(types)-2] != modelinvoker.StreamEventUsage || types[len(types)-1] != modelinvoker.StreamEventResponseCompleted {
		t.Fatalf("sequences/types = %v / %v", sequences, types)
	}
	if terminal == nil || terminal.Provider != testProvider || terminal.State == nil || terminal.State.Provider != testProvider ||
		terminal.State.ID != "resp_stream" || terminal.Text() != "hello" || terminal.Usage.TotalTokens != 3 {
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

func TestNewRejectsWrongBindingAndNilClients(t *testing.T) {
	wrong, err := protocol.NewBinding(testProvider, modelinvoker.ProtocolChatCompletions, testEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openairesponses.New(wrong, &fakeDialect{}, &fakeClient{}); err == nil {
		t.Fatal("New(wrong protocol) error = nil")
	}
	if _, err := openairesponses.New(mustBinding(t), &fakeDialect{}, nil); err == nil {
		t.Fatal("New(nil client) error = nil")
	}
	var typedNil *fakeClient
	if _, err := openairesponses.New(mustBinding(t), &fakeDialect{}, typedNil); err == nil {
		t.Fatal("New(typed-nil client) error = nil")
	}
}

func mustDriver(t *testing.T, dialect protocol.Dialect, client openairesponses.Client) *openairesponses.Driver {
	t.Helper()
	driver, err := openairesponses.New(mustBinding(t), dialect, client)
	if err != nil {
		t.Fatal(err)
	}
	return driver
}

func mustBinding(t *testing.T) protocol.Binding {
	t.Helper()
	binding, err := protocol.NewBinding(testProvider, modelinvoker.ProtocolResponses, testEndpoint, "x-request-id")
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func validRequest() modelinvoker.Request {
	return modelinvoker.Request{
		Provider: testProvider, Protocol: modelinvoker.ProtocolResponses,
		Endpoint: testEndpoint, Model: "test-model",
		Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
	}
}

func decodeResponse(t *testing.T, raw string) responses.Response {
	t.Helper()
	var value responses.Response
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func decodeEvent(t *testing.T, raw string) responses.ResponseStreamEventUnion {
	t.Helper()
	var value responses.ResponseStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

var (
	_ protocol.Dialect            = (*fakeDialect)(nil)
	_ openairesponses.Client      = (*fakeClient)(nil)
	_ openairesponses.EventStream = (*fakeStream)(nil)
)
