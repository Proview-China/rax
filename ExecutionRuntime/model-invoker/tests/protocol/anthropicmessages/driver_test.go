package anthropicmessages_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/anthropicmessages"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

const (
	testProvider modelinvoker.ProviderID = "acme-messages"
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
	responses   []*anthropicsdk.Message
	createErr   error
	stream      anthropicmessages.EventStream
	createCalls int
	streamCalls int
	params      []anthropicsdk.MessageNewParams
}

func (c *fakeClient) Create(_ context.Context, params anthropicsdk.MessageNewParams) (*anthropicsdk.Message, http.Header, error) {
	c.createCalls++
	c.params = append(c.params, params)
	var response *anthropicsdk.Message
	if len(c.responses) != 0 {
		response = c.responses[0]
		c.responses = c.responses[1:]
	}
	return response, http.Header{"Request-Id": []string{"req_fake"}, "X-Test-Meta": []string{"meta"}}, c.createErr
}

func (c *fakeClient) Stream(_ context.Context, params anthropicsdk.MessageNewParams) (anthropicmessages.EventStream, http.Header) {
	c.streamCalls++
	c.params = append(c.params, params)
	return c.stream, http.Header{"Request-Id": []string{"req_stream"}, "X-Test-Meta": []string{"meta"}}
}

type fakeStream struct {
	events   []anthropicsdk.MessageStreamEventUnion
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

func (s *fakeStream) Current() anthropicsdk.MessageStreamEventUnion {
	if s.index == 0 || s.index > len(s.events) {
		return anthropicsdk.MessageStreamEventUnion{}
	}
	return s.events[s.index-1]
}

func (s *fakeStream) Err() error { return s.terminal }

func (s *fakeStream) Close() error {
	s.closed++
	return nil
}

func TestDriverPreservesSignedContinuationAndBindingIdentity(t *testing.T) {
	first := decodeMessage(t, `{
		"id":"msg_1","type":"message","role":"assistant","model":"served-model",
		"content":[
			{"type":"thinking","thinking":"use the tool","signature":"sig_1"},
			{"type":"text","text":"checking"},
			{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"city":"Rome"},"caller":{"type":"direct"}}
		],
		"stop_reason":"tool_use","stop_sequence":null,
		"usage":{"input_tokens":4,"output_tokens":3,"output_tokens_details":{"thinking_tokens":1}}
	}`)
	second := decodeMessage(t, `{
		"id":"msg_2","type":"message","role":"assistant","model":"served-model",
		"content":[{"type":"text","text":"done"}],
		"stop_reason":"end_turn","stop_sequence":null,
		"usage":{"input_tokens":2,"output_tokens":1}
	}`)
	client := &fakeClient{responses: []*anthropicsdk.Message{&first, &second}}
	dialect := &fakeDialect{}
	driver := mustDriver(t, dialect, client)

	request := validRequest()
	request.Tools = lookupTools()
	request.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh, Summary: modelinvoker.ReasoningSummaryAuto}
	response, err := driver.Invoke(context.Background(), request)
	if err != nil {
		t.Fatalf("first Invoke() error = %v", err)
	}
	if client.createCalls != 1 || dialect.validateCalls != 1 {
		t.Fatalf("client/dialect calls = %d/%d", client.createCalls, dialect.validateCalls)
	}
	if response.Provider != testProvider || response.Protocol != modelinvoker.ProtocolMessages ||
		response.MappingReport.Provider != testProvider || response.MappingReport.Endpoint != testEndpoint ||
		response.RequestID != "req_fake" || response.ProviderMetadata["test-meta"] != "meta" {
		t.Fatalf("response identity/metadata = %#v", response)
	}
	if response.State == nil || response.State.Kind != modelinvoker.StateProviderContinuation ||
		response.State.Provider != testProvider || response.State.Protocol != modelinvoker.ProtocolMessages || response.State.ID != "msg_1" {
		t.Fatalf("continuation identity = %#v", response.State)
	}
	payload := string(response.State.Payload.Bytes())
	if !strings.Contains(payload, "sig_1") || !strings.Contains(payload, "toolu_1") || strings.Contains(payload, "checking") {
		t.Fatalf("continuation payload = %s", payload)
	}
	if response.Text() != "checking" || len(response.FunctionCalls()) != 1 || response.Usage.ReasoningTokens != 1 {
		t.Fatalf("normalized first response = %#v", response)
	}

	continued := validRequest()
	continued.Input = []modelinvoker.InputItem{modelinvoker.FunctionResultInput("toolu_1", "sunny", false)}
	continued.Tools = lookupTools()
	continued.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh, Summary: modelinvoker.ReasoningSummaryAuto}
	continued.State = response.State
	final, err := driver.Invoke(context.Background(), continued)
	if err != nil {
		t.Fatalf("continued Invoke() error = %v", err)
	}
	if final.Text() != "done" || client.createCalls != 2 || dialect.validateCalls != 2 {
		t.Fatalf("continued response/calls = %#v / %d/%d", final, client.createCalls, dialect.validateCalls)
	}
	encoded, err := json.Marshal(client.params[1])
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	messages := wire["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("continued messages = %#v", messages)
	}
	blocks := messages[0].(map[string]any)["content"].([]any)
	if len(blocks) != 2 || blocks[0].(map[string]any)["signature"] != "sig_1" || blocks[1].(map[string]any)["id"] != "toolu_1" {
		t.Fatalf("continued assistant blocks = %#v", blocks)
	}
	result := messages[1].(map[string]any)["content"].([]any)[0].(map[string]any)
	if result["tool_use_id"] != "toolu_1" {
		t.Fatalf("continued tool result = %#v", result)
	}
}

func TestDriverRejectsWrongStateKindBeforeClient(t *testing.T) {
	client := &fakeClient{}
	driver := mustDriver(t, &fakeDialect{}, client)
	request := validRequest()
	request.State = &modelinvoker.State{
		Kind: modelinvoker.StateServerContinuation, Provider: testProvider,
		Protocol: modelinvoker.ProtocolMessages, ID: "opaque",
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

func TestDriverStreamPreservesSequenceTerminalIdentityAndClose(t *testing.T) {
	native := &fakeStream{events: []anthropicsdk.MessageStreamEventUnion{
		decodeEvent(t, `{"type":"message_start","message":{"id":"msg_stream","type":"message","role":"assistant","model":"served-model","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":2,"output_tokens":0}}}`),
		decodeEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
		decodeEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`),
		decodeEvent(t, `{"type":"content_block_stop","index":0}`),
		decodeEvent(t, `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`),
		decodeEvent(t, `{"type":"message_stop"}`),
	}}
	client := &fakeClient{stream: native}
	stream, err := mustDriver(t, &fakeDialect{}, client).Stream(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	var previous int64
	var terminal *modelinvoker.Response
	for stream.Next() {
		event := stream.Event()
		if event.Sequence <= previous {
			t.Fatalf("sequence %d after %d", event.Sequence, previous)
		}
		previous = event.Sequence
		if event.Response != nil {
			terminal = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if terminal == nil || terminal.Provider != testProvider || terminal.Protocol != modelinvoker.ProtocolMessages ||
		terminal.MappingReport.Endpoint != testEndpoint || terminal.Text() != "hello" || terminal.Usage.TotalTokens != 3 {
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
	wrong, err := protocol.NewBinding(testProvider, modelinvoker.ProtocolResponses, testEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := anthropicmessages.New(wrong, &fakeDialect{}, &fakeClient{}); err == nil {
		t.Fatal("New(wrong protocol) error = nil")
	}
	if _, err := anthropicmessages.New(mustBinding(t), &fakeDialect{}, nil); err == nil {
		t.Fatal("New(nil client) error = nil")
	}
	var typedNil *fakeClient
	if _, err := anthropicmessages.New(mustBinding(t), &fakeDialect{}, typedNil); err == nil {
		t.Fatal("New(typed-nil client) error = nil")
	}
}

func mustDriver(t *testing.T, dialect protocol.Dialect, client anthropicmessages.Client) *anthropicmessages.Driver {
	t.Helper()
	driver, err := anthropicmessages.New(mustBinding(t), dialect, client)
	if err != nil {
		t.Fatal(err)
	}
	return driver
}

func mustBinding(t *testing.T) protocol.Binding {
	t.Helper()
	binding, err := protocol.NewBinding(testProvider, modelinvoker.ProtocolMessages, testEndpoint, "request-id")
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func validRequest() modelinvoker.Request {
	return modelinvoker.Request{
		Provider: testProvider, Protocol: modelinvoker.ProtocolMessages,
		Endpoint: testEndpoint, Model: "test-model",
		Input:  []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
		Budget: modelinvoker.Budget{MaxOutputTokens: 4096},
	}
}

func lookupTools() []modelinvoker.Tool {
	return []modelinvoker.Tool{{
		Name: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
	}}
}

func decodeMessage(t *testing.T, raw string) anthropicsdk.Message {
	t.Helper()
	var value anthropicsdk.Message
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func decodeEvent(t *testing.T, raw string) anthropicsdk.MessageStreamEventUnion {
	t.Helper()
	var value anthropicsdk.MessageStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

var (
	_ protocol.Dialect              = (*fakeDialect)(nil)
	_ anthropicmessages.Client      = (*fakeClient)(nil)
	_ anthropicmessages.EventStream = (*fakeStream)(nil)
)
