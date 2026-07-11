package bedrock_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	bedrockprotocol "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/bedrock"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsruntime "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	awstypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

const testProvider modelinvoker.ProviderID = "test-bedrock"
const testEndpoint = "https://bedrock-runtime.us-east-1.amazonaws.com"

type fakeDialect struct{}

func (fakeDialect) ValidateRequest(modelinvoker.Request) error { return nil }
func (fakeDialect) ClassifyFailure(protocol.Failure) protocol.ErrorClassification {
	return protocol.ErrorClassification{Kind: modelinvoker.ErrorProvider, Message: "safe Bedrock failure"}
}
func (fakeDialect) ProviderMetadata(http.Header) modelinvoker.ProviderMetadata { return nil }

type fakeClient struct {
	converseInput       *awsruntime.ConverseInput
	converseOutput      *awsruntime.ConverseOutput
	converseErr         error
	converseStreamInput *awsruntime.ConverseStreamInput
	converseStream      bedrockprotocol.ConverseEventStream
	invokeInput         *awsruntime.InvokeModelInput
	invokeOutput        *awsruntime.InvokeModelOutput
	invokeStreamInput   *awsruntime.InvokeModelWithResponseStreamInput
	invokeStream        bedrockprotocol.InvokeEventStream
}

func (c *fakeClient) Converse(_ context.Context, in *awsruntime.ConverseInput) (*awsruntime.ConverseOutput, error) {
	c.converseInput = in
	return c.converseOutput, c.converseErr
}
func (c *fakeClient) ConverseStream(_ context.Context, in *awsruntime.ConverseStreamInput) (bedrockprotocol.ConverseEventStream, error) {
	c.converseStreamInput = in
	return c.converseStream, c.converseErr
}
func (c *fakeClient) InvokeModel(_ context.Context, in *awsruntime.InvokeModelInput) (*awsruntime.InvokeModelOutput, error) {
	c.invokeInput = in
	return c.invokeOutput, c.converseErr
}
func (c *fakeClient) InvokeModelWithResponseStream(_ context.Context, in *awsruntime.InvokeModelWithResponseStreamInput) (bedrockprotocol.InvokeEventStream, error) {
	c.invokeStreamInput = in
	return c.invokeStream, c.converseErr
}

type converseEvents struct {
	events chan awstypes.ConverseStreamOutput
	err    error
	closed bool
}

func (s *converseEvents) Events() <-chan awstypes.ConverseStreamOutput { return s.events }
func (s *converseEvents) Err() error                                   { return s.err }
func (s *converseEvents) Close() error                                 { s.closed = true; return nil }

type invokeEvents struct {
	events chan awstypes.ResponseStream
	err    error
	closed bool
}

func (s *invokeEvents) Events() <-chan awstypes.ResponseStream { return s.events }
func (s *invokeEvents) Err() error                             { return s.err }
func (s *invokeEvents) Close() error                           { s.closed = true; return nil }

func TestConverseMapsAgentSemanticsAndNormalizesOutput(t *testing.T) {
	client := &fakeClient{converseOutput: &awsruntime.ConverseOutput{Output: &awstypes.ConverseOutputMemberMessage{Value: awstypes.Message{Role: awstypes.ConversationRoleAssistant, Content: []awstypes.ContentBlock{&awstypes.ContentBlockMemberText{Value: "hello"}, &awstypes.ContentBlockMemberToolUse{Value: awstypes.ToolUseBlock{ToolUseId: aws.String("call-1"), Name: aws.String("lookup"), Input: document.NewLazyDocument(map[string]any{"city": "Rome"})}}}}}, StopReason: awstypes.StopReasonToolUse, Usage: &awstypes.TokenUsage{InputTokens: aws.Int32(3), OutputTokens: aws.Int32(4), TotalTokens: aws.Int32(7)}}}
	driver := mustDriver(t, modelinvoker.ProtocolBedrockConverse, client)
	request := converseRequest()
	response, err := driver.Invoke(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if client.converseInput == nil || aws.ToString(client.converseInput.ModelId) != "anthropic.claude-test" || len(client.converseInput.Messages) != 3 || len(client.converseInput.System) != 1 || client.converseInput.ToolConfig == nil {
		t.Fatalf("mapped input = %#v", client.converseInput)
	}
	if response.Text() != "hello" || response.StopReason != modelinvoker.StopReasonToolCall || response.Usage.TotalTokens != 7 || response.Provider != testProvider {
		t.Fatalf("response = %#v", response)
	}
	calls := response.FunctionCalls()
	if len(calls) != 1 || calls[0].ID != "call-1" || calls[0].Name != "lookup" || string(calls[0].Arguments) != "{\"city\":\"Rome\"}" {
		t.Fatalf("calls = %#v", calls)
	}
	if response.RawRequest.Empty() || response.RawResponse.Empty() {
		t.Fatal("raw audit boundaries are empty")
	}
}

func TestConverseStreamPreservesOrderToolAndTerminalUsage(t *testing.T) {
	channel := make(chan awstypes.ConverseStreamOutput, 8)
	channel <- &awstypes.ConverseStreamOutputMemberMessageStart{Value: awstypes.MessageStartEvent{Role: awstypes.ConversationRoleAssistant}}
	channel <- &awstypes.ConverseStreamOutputMemberContentBlockDelta{Value: awstypes.ContentBlockDeltaEvent{ContentBlockIndex: aws.Int32(0), Delta: &awstypes.ContentBlockDeltaMemberText{Value: "hi"}}}
	channel <- &awstypes.ConverseStreamOutputMemberContentBlockStart{Value: awstypes.ContentBlockStartEvent{ContentBlockIndex: aws.Int32(1), Start: &awstypes.ContentBlockStartMemberToolUse{Value: awstypes.ToolUseBlockStart{ToolUseId: aws.String("call-1"), Name: aws.String("lookup")}}}}
	channel <- &awstypes.ConverseStreamOutputMemberContentBlockDelta{Value: awstypes.ContentBlockDeltaEvent{ContentBlockIndex: aws.Int32(1), Delta: &awstypes.ContentBlockDeltaMemberToolUse{Value: awstypes.ToolUseBlockDelta{Input: aws.String(`{"city":"Rome"}`)}}}}
	channel <- &awstypes.ConverseStreamOutputMemberContentBlockStop{Value: awstypes.ContentBlockStopEvent{ContentBlockIndex: aws.Int32(1)}}
	channel <- &awstypes.ConverseStreamOutputMemberMetadata{Value: awstypes.ConverseStreamMetadataEvent{Usage: &awstypes.TokenUsage{InputTokens: aws.Int32(2), OutputTokens: aws.Int32(2), TotalTokens: aws.Int32(4)}}}
	channel <- &awstypes.ConverseStreamOutputMemberMessageStop{Value: awstypes.MessageStopEvent{StopReason: awstypes.StopReasonToolUse}}
	close(channel)
	native := &converseEvents{events: channel}
	client := &fakeClient{converseStream: native}
	stream, err := mustDriver(t, modelinvoker.ProtocolBedrockConverse, client).Stream(context.Background(), converseRequest())
	if err != nil {
		t.Fatal(err)
	}
	var sequence int64
	var terminal *modelinvoker.Response
	var types []modelinvoker.StreamEventType
	for stream.Next() {
		event := stream.Event()
		if event.Sequence <= sequence {
			t.Fatalf("sequence = %d after %d", event.Sequence, sequence)
		}
		sequence = event.Sequence
		types = append(types, event.Type)
		if event.Type == modelinvoker.StreamEventResponseCompleted {
			terminal = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if terminal == nil || terminal.Text() != "hi" || terminal.Usage.TotalTokens != 4 || terminal.StopReason != modelinvoker.StopReasonToolCall {
		t.Fatalf("terminal/types = %#v / %#v", terminal, types)
	}
	if err := stream.Close(); err != nil || !native.closed {
		t.Fatalf("close = %v/%v", err, native.closed)
	}
}

func TestInvokeModelRetainsProviderNativeJSONAndStreamChunks(t *testing.T) {
	client := &fakeClient{invokeOutput: &awsruntime.InvokeModelOutput{Body: []byte(`{"outputText":"ok"}`), ContentType: aws.String("application/json")}}
	driver := mustDriver(t, modelinvoker.ProtocolBedrockInvoke, client)
	request := invokeRequest()
	response, err := driver.Invoke(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(response.RawResponse.Bytes()); got != `{"outputText":"ok"}` {
		t.Fatalf("raw response = %s", got)
	}
	if client.invokeInput == nil || string(client.invokeInput.Body) != `{"prompt":"hello"}` {
		t.Fatalf("invoke input = %#v", client.invokeInput)
	}
	channel := make(chan awstypes.ResponseStream, 2)
	channel <- &awstypes.ResponseStreamMemberChunk{Value: awstypes.PayloadPart{Bytes: []byte(`{"output`)}}
	channel <- &awstypes.ResponseStreamMemberChunk{Value: awstypes.PayloadPart{Bytes: []byte(`Text":"ok"}`)}}
	close(channel)
	native := &invokeEvents{events: channel}
	client.invokeStream = native
	stream, err := driver.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var terminal *modelinvoker.Response
	for stream.Next() {
		if stream.Event().Type == modelinvoker.StreamEventResponseCompleted {
			terminal = stream.Event().Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if terminal == nil || string(terminal.RawResponse.Bytes()) != `{"outputText":"ok"}` {
		t.Fatalf("terminal = %#v", terminal)
	}
}

func TestDriverDropsNativeFailureAndRejectsUnsafeModes(t *testing.T) {
	client := &fakeClient{converseErr: &awstypes.AccessDeniedException{Message: aws.String("native-secret")}}
	driver := mustDriver(t, modelinvoker.ProtocolBedrockConverse, client)
	response, err := driver.Invoke(context.Background(), converseRequest())
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError.Err != nil || invocationError.Kind != modelinvoker.ErrorPermission ||
		invocationError.Code != "AccessDeniedException" || response.Status != modelinvoker.ResponseStatusFailed {
		t.Fatalf("failure = %#v / %v", response, err)
	}
	if strings.Contains(err.Error(), "native-secret") {
		t.Fatal("native SDK message leaked")
	}
	request := converseRequest()
	request.State = &modelinvoker.State{Kind: modelinvoker.StateServerContinuation, Provider: testProvider, Protocol: modelinvoker.ProtocolBedrockConverse, ID: "state"}
	if _, err := driver.Invoke(context.Background(), request); err == nil {
		t.Fatal("continuation state unexpectedly accepted")
	}
}

func mustDriver(t *testing.T, id modelinvoker.Protocol, client bedrockprotocol.Client) *bedrockprotocol.Driver {
	t.Helper()
	binding, err := protocol.NewBinding(testProvider, id, testEndpoint, "x-amzn-requestid")
	if err != nil {
		t.Fatal(err)
	}
	driver, err := bedrockprotocol.New(binding, fakeDialect{}, client)
	if err != nil {
		t.Fatal(err)
	}
	return driver
}
func converseRequest() modelinvoker.Request {
	strict := true
	return modelinvoker.Request{Provider: testProvider, Protocol: modelinvoker.ProtocolBedrockConverse, Endpoint: testEndpoint, Model: "anthropic.claude-test", Instructions: []modelinvoker.Instruction{{Role: modelinvoker.RoleSystem, Text: "be concise"}}, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello"), modelinvoker.FunctionCallInput("prior", "lookup", json.RawMessage(`{"city":"Paris"}`)), modelinvoker.FunctionResultInput("prior", "sunny", false)}, Tools: []modelinvoker.Tool{{Name: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`), Strict: &strict}}, ToolChoice: modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceAuto}, Budget: modelinvoker.Budget{MaxOutputTokens: 64}}
}
func invokeRequest() modelinvoker.Request {
	return modelinvoker.Request{Provider: testProvider, Protocol: modelinvoker.ProtocolBedrockInvoke, Endpoint: testEndpoint, Model: "amazon.test", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}, ProviderOptions: modelinvoker.ProviderOptions{testProvider: json.RawMessage(`{"body":{"prompt":"hello"}}`)}, AllowDegradation: true}
}
