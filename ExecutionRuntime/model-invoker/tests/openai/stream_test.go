package openai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func TestPublicResponsesStreamNormalizesTextToolsNativeAndTerminal(t *testing.T) {
	t.Parallel()

	server := newSSEServer(t,
		`{"type":"response.created","sequence_number":1,"response":{"id":"resp_stream","model":"test-model","status":"in_progress","output":[]}}`,
		`{"type":"response.output_text.delta","sequence_number":2,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"hello "}`,
		`{"type":"response.output_item.added","sequence_number":3,"output_index":1,"item":{"id":"item_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"","status":"in_progress"}}`,
		`{"type":"response.function_call_arguments.delta","sequence_number":4,"item_id":"item_1","output_index":1,"delta":"{\"city\":"}`,
		`{"type":"response.function_call_arguments.delta","sequence_number":5,"item_id":"item_1","output_index":1,"delta":"\"Rome\"}"}`,
		`{"type":"response.function_call_arguments.done","sequence_number":6,"item_id":"item_1","output_index":1,"name":"lookup","arguments":"{\"city\":\"Rome\"}"}`,
		`{"type":"response.reasoning_summary_text.delta","sequence_number":7,"item_id":"reasoning_1","output_index":2,"summary_index":0,"delta":"because "}`,
		`{"type":"response.future.event","sequence_number":8,"opaque":"kept"}`,
		`{"type":"response.completed","sequence_number":9,"response":{"id":"resp_stream","model":"test-model","status":"completed","output":[{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello done","annotations":[]}]},{"id":"item_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"city\":\"Rome\"}","status":"completed"}],"usage":{"input_tokens":5,"input_tokens_details":{"cached_tokens":1},"output_tokens":4,"output_tokens_details":{"reasoning_tokens":2},"total_tokens":9}}}`,
	)
	adapter := newPublicAdapter(t, server.URL)
	invoker := newPublicInvoker(t, adapter)
	request := basePublicRequest(modelinvoker.ProtocolResponses)

	stream, err := invoker.Stream(context.Background(), request)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer func() { _ = stream.Close() }()

	var types []modelinvoker.StreamEventType
	var sequences []int64
	var terminal *modelinvoker.Response
	var reasoningDelta string
	var streamedUsage *modelinvoker.Usage
	for stream.Next() {
		event := stream.Event()
		types = append(types, event.Type)
		sequences = append(sequences, event.Sequence)
		if event.Type == modelinvoker.StreamEventFunctionArgumentsDelta && event.FunctionCall == nil {
			t.Fatal("function argument delta lost tool identity")
		}
		if event.Type == modelinvoker.StreamEventNative && event.Raw.Empty() {
			t.Fatal("native event lost raw payload")
		}
		if event.Type == modelinvoker.StreamEventReasoningDelta {
			reasoningDelta = event.ReasoningDelta
			if event.TextDelta != "" {
				t.Fatalf("reasoning event reused text delta: %#v", event)
			}
		}
		if event.Type == modelinvoker.StreamEventUsage {
			streamedUsage = event.Usage
		}
		if event.Response != nil {
			terminal = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err() = %v", err)
	}
	wantTypes := []modelinvoker.StreamEventType{
		modelinvoker.StreamEventResponseStarted,
		modelinvoker.StreamEventTextDelta,
		modelinvoker.StreamEventFunctionCallStarted,
		modelinvoker.StreamEventFunctionArgumentsDelta,
		modelinvoker.StreamEventFunctionArgumentsDelta,
		modelinvoker.StreamEventFunctionCallCompleted,
		modelinvoker.StreamEventReasoningDelta,
		modelinvoker.StreamEventNative,
		modelinvoker.StreamEventUsage,
		modelinvoker.StreamEventResponseCompleted,
	}
	if !reflect.DeepEqual(types, wantTypes) {
		t.Fatalf("event order = %v, want %v", types, wantTypes)
	}
	assertStrictlyIncreasingSequences(t, sequences)
	if streamedUsage == nil || streamedUsage.TotalTokens != 9 || streamedUsage.ReasoningTokens != 2 {
		t.Fatalf("streamed usage = %#v", streamedUsage)
	}
	if terminal == nil || terminal.Text() != "hello done" || terminal.Usage.TotalTokens != 9 || terminal.Usage.CacheReadTokens != 1 {
		t.Fatalf("terminal response = %#v", terminal)
	}
	if terminal.RequestID != "req_stream" || terminal.State == nil || terminal.State.Kind != modelinvoker.StateServerContinuation ||
		terminal.State.Provider != "openai" || terminal.State.Protocol != modelinvoker.ProtocolResponses || terminal.State.ID != "resp_stream" {
		t.Fatalf("terminal request/state = %q/%#v", terminal.RequestID, terminal.State)
	}
	if terminal.StopReason != modelinvoker.StopReasonToolCall || reasoningDelta != "because " {
		t.Fatalf("stop reason/reasoning delta = %q/%q", terminal.StopReason, reasoningDelta)
	}
	var auditedRequest map[string]any
	if err := json.Unmarshal(terminal.RawRequest.Bytes(), &auditedRequest); err != nil {
		t.Fatalf("decode RawRequest: %v", err)
	}
	publicWant(t, auditedRequest, "stream", true)
	calls := terminal.FunctionCalls()
	if len(calls) != 1 || calls[0].ID != "call_1" || string(calls[0].Arguments) != `{"city":"Rome"}` {
		t.Fatalf("terminal calls = %#v", calls)
	}
	if len(terminal.NativeEvents) != 9 || terminal.RawRequest.Empty() {
		t.Fatalf("audit data missing: native=%d rawRequest=%v", len(terminal.NativeEvents), terminal.RawRequest.Empty())
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestPublicResponsesStreamFailureModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		events   []string
		wantKind modelinvoker.ErrorKind
	}{
		{
			name:     "provider error event",
			events:   []string{`{"type":"error","sequence_number":1,"code":"server_error","message":"boom"}`},
			wantKind: modelinvoker.ErrorProviderUnavailable,
		},
		{
			name:     "response failed",
			events:   []string{`{"type":"response.failed","sequence_number":1,"response":{"id":"resp_failed","model":"test-model","status":"failed","error":{"code":"rate_limit_exceeded","message":"limited"},"output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}}`},
			wantKind: modelinvoker.ErrorRateLimit,
		},
		{
			name: "invalid completed arguments",
			events: []string{
				`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"item_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":""}}`,
				`{"type":"response.function_call_arguments.done","sequence_number":2,"item_id":"item_1","output_index":0,"name":"lookup","arguments":"not-json"}`,
			},
			wantKind: modelinvoker.ErrorMapping,
		},
		{
			name:     "missing terminal event",
			events:   []string{`{"type":"response.output_text.delta","sequence_number":1,"delta":"partial"}`},
			wantKind: modelinvoker.ErrorStreamInterrupted,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := newSSEServer(t, test.events...)
			adapter := newPublicAdapter(t, server.URL)
			stream, err := adapter.Stream(context.Background(), basePublicRequest(modelinvoker.ProtocolResponses))
			if err != nil {
				t.Fatalf("Stream() error = %v", err)
			}
			var sawErrorEvent bool
			var partial *modelinvoker.Response
			var sequences []int64
			for stream.Next() {
				sequences = append(sequences, stream.Event().Sequence)
				if stream.Event().Type == modelinvoker.StreamEventError {
					sawErrorEvent = true
					partial = stream.Event().Response
				}
			}
			invocationError := assertPublicErrorKind(t, stream.Err(), test.wantKind)
			if !sawErrorEvent {
				t.Fatal("stream did not emit an error event")
			}
			assertStrictlyIncreasingSequences(t, sequences)
			if invocationError.Provider != "openai" || invocationError.RequestID != "req_stream" {
				t.Fatalf("provider/request ID = %q/%q", invocationError.Provider, invocationError.RequestID)
			}
			if partial == nil || partial.RawRequest.Empty() || partial.RawResponse.Empty() || len(partial.NativeEvents) == 0 {
				t.Fatalf("partial failure audit = %#v", partial)
			}
			_ = stream.Close()
		})
	}
}

func TestPublicChatStreamAggregatesParallelToolsUsageBeforeTerminal(t *testing.T) {
	t.Parallel()

	server := newSSEServer(t,
		`{"id":"chat_stream","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"hello "},"finish_reason":null}]}`,
		`{"id":"chat_stream","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"second","arguments":"{\"b\":"}},{"index":0,"id":"call_a","type":"function","function":{"name":"first","arguments":"{\"a\":"}}]},"finish_reason":null}]}`,
		`{"id":"chat_stream","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"2}"}},{"index":0,"function":{"arguments":"1}"}}]},"finish_reason":null}]}`,
		`{"id":"chat_stream","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`{"id":"chat_stream","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[],"usage":{"prompt_tokens":5,"prompt_tokens_details":{"cached_tokens":1},"completion_tokens":4,"completion_tokens_details":{"reasoning_tokens":2},"total_tokens":9}}`,
	)
	adapter := newPublicAdapter(t, server.URL)
	invoker := newPublicInvoker(t, adapter)
	stream, err := invoker.Stream(context.Background(), basePublicRequest(modelinvoker.ProtocolChatCompletions))
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer func() { _ = stream.Close() }()

	var types []modelinvoker.StreamEventType
	var sequences []int64
	var terminal *modelinvoker.Response
	for stream.Next() {
		event := stream.Event()
		types = append(types, event.Type)
		sequences = append(sequences, event.Sequence)
		if event.Response != nil {
			terminal = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err() = %v", err)
	}
	if terminal == nil || terminal.Usage.TotalTokens != 9 || terminal.Usage.CacheReadTokens != 1 || terminal.StopReason != modelinvoker.StopReasonToolCall {
		t.Fatalf("terminal response = %#v", terminal)
	}
	if terminal.RequestID != "req_stream" {
		t.Fatalf("terminal request ID = %q", terminal.RequestID)
	}
	var auditedRequest map[string]any
	if err := json.Unmarshal(terminal.RawRequest.Bytes(), &auditedRequest); err != nil {
		t.Fatalf("decode RawRequest: %v", err)
	}
	publicWant(t, auditedRequest, "stream", true)
	if len(types) < 2 || types[len(types)-2] != modelinvoker.StreamEventUsage || types[len(types)-1] != modelinvoker.StreamEventResponseCompleted {
		t.Fatalf("usage/terminal order = %v", types)
	}
	assertStrictlyIncreasingSequences(t, sequences)
	calls := terminal.FunctionCalls()
	if len(calls) != 2 || calls[0].ID != "call_a" || calls[1].ID != "call_b" || string(calls[0].Arguments) != `{"a":1}` || string(calls[1].Arguments) != `{"b":2}` {
		t.Fatalf("parallel calls = %#v", calls)
	}
}

func TestPublicChatStreamFailureModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		events   []string
		wantKind modelinvoker.ErrorKind
	}{
		{
			name:     "unknown finish reason",
			events:   []string{`{"id":"chat","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"future_reason"}]}`},
			wantKind: modelinvoker.ErrorMapping,
		},
		{
			name:     "content filter",
			events:   []string{`{"id":"chat","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"content_filter"}]}`},
			wantKind: modelinvoker.ErrorPolicyRejected,
		},
		{
			name: "invalid tool arguments",
			events: []string{
				`{"id":"chat","model":"test-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call","type":"function","function":{"name":"lookup","arguments":"bad"}}]},"finish_reason":null}]}`,
				`{"id":"chat","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			},
			wantKind: modelinvoker.ErrorMapping,
		},
		{
			name:     "missing finish reason",
			events:   []string{`{"id":"chat","model":"test-model","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}`},
			wantKind: modelinvoker.ErrorStreamInterrupted,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := newSSEServer(t, test.events...)
			adapter := newPublicAdapter(t, server.URL)
			stream, err := adapter.Stream(context.Background(), basePublicRequest(modelinvoker.ProtocolChatCompletions))
			if err != nil {
				t.Fatalf("Stream() error = %v", err)
			}
			var sawError bool
			var partial *modelinvoker.Response
			var sequences []int64
			for stream.Next() {
				sequences = append(sequences, stream.Event().Sequence)
				if stream.Event().Type == modelinvoker.StreamEventError {
					sawError = true
					partial = stream.Event().Response
				}
			}
			got := assertPublicErrorKind(t, stream.Err(), test.wantKind)
			if !sawError || partial == nil || partial.RawRequest.Empty() || partial.RawResponse.Empty() || len(partial.NativeEvents) == 0 {
				t.Fatalf("partial Chat failure audit = %#v", partial)
			}
			assertStrictlyIncreasingSequences(t, sequences)
			if test.name == "content filter" && partial.StopReason != modelinvoker.StopReasonContentFilter {
				t.Fatalf("content filter stop reason = %q", partial.StopReason)
			}
			if got.RequestID != "req_stream" {
				t.Fatalf("request ID = %q", got.RequestID)
			}
			_ = stream.Close()
		})
	}
}

func TestPublicChatStreamUnknownFinishReasonCanBeExplicitlyDegraded(t *testing.T) {
	t.Parallel()

	server := newSSEServer(t,
		`{"id":"chat","model":"test-model","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":"future_reason"}]}`,
		`{"id":"chat","model":"test-model","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
	)
	request := basePublicRequest(modelinvoker.ProtocolChatCompletions)
	request.AllowDegradation = true
	stream, err := newPublicAdapter(t, server.URL).Stream(context.Background(), request)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer func() { _ = stream.Close() }()
	var terminal *modelinvoker.Response
	for stream.Next() {
		if stream.Event().Response != nil && stream.Event().Type == modelinvoker.StreamEventResponseCompleted {
			terminal = stream.Event().Response
		}
	}
	if stream.Err() != nil || terminal == nil || !terminal.MappingReport.HasDegradation() || terminal.StopReason != modelinvoker.StopReasonOther {
		t.Fatalf("terminal/error = %#v / %v", terminal, stream.Err())
	}
}

func TestPublicSDKStructuredStreamErrorsAreSanitizedAndAudited(t *testing.T) {
	t.Parallel()

	for _, protocol := range []modelinvoker.Protocol{modelinvoker.ProtocolResponses, modelinvoker.ProtocolChatCompletions} {
		protocol := protocol
		t.Run(string(protocol), func(t *testing.T) {
			t.Parallel()
			server := newSSEServer(t, `{"error":{"message":"limited","type":"rate_limit_error","code":"rate_limit_exceeded"}}`)
			stream, err := newPublicAdapter(t, server.URL).Stream(context.Background(), basePublicRequest(protocol))
			if err != nil {
				t.Fatalf("Stream() error = %v", err)
			}
			defer func() { _ = stream.Close() }()
			var event modelinvoker.StreamEvent
			if !stream.Next() {
				t.Fatalf("Next() = false, err=%v", stream.Err())
			}
			event = stream.Event()
			if event.Type != modelinvoker.StreamEventError || event.Response == nil || event.Raw.Empty() {
				t.Fatalf("structured error event = %#v", event)
			}
			got := assertPublicErrorKind(t, event.Error, modelinvoker.ErrorRateLimit)
			if got.Code != "rate_limit_exceeded" || !got.Retryable || got.RequestID != "req_stream" || got.Err != nil {
				t.Fatalf("structured error = %#v", got)
			}
			if stream.Next() {
				t.Fatal("Next() after structured error = true")
			}
			assertPublicErrorKind(t, stream.Err(), modelinvoker.ErrorRateLimit)
		})
	}
}

func TestPublicTerminalStreamsStopReadingAndCloseTransport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		protocol modelinvoker.Protocol
		events   []string
		wantStop modelinvoker.StopReason
	}{
		{modelinvoker.ProtocolResponses, []string{`{"type":"response.completed","sequence_number":1,"response":{"id":"resp","model":"test-model","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}}`}, modelinvoker.StopReasonEndTurn},
		{modelinvoker.ProtocolChatCompletions, []string{
			`{"id":"chat","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`{"id":"chat","model":"test-model","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":0,"total_tokens":1}}`,
		}, modelinvoker.StopReasonEndTurn},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.protocol), func(t *testing.T) {
			t.Parallel()
			transportClosed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "text/event-stream")
				writer.Header().Set("X-Request-Id", "req_terminal_close")
				flusher, ok := writer.(http.Flusher)
				if !ok {
					t.Error("response writer does not support flushing")
					return
				}
				for _, event := range test.events {
					_, _ = fmt.Fprintf(writer, "data: %s\n\n", event)
					flusher.Flush()
				}
				<-request.Context().Done()
				close(transportClosed)
			}))
			t.Cleanup(server.Close)

			stream, err := newPublicAdapter(t, server.URL).Stream(context.Background(), basePublicRequest(test.protocol))
			if err != nil {
				t.Fatalf("Stream() error = %v", err)
			}
			t.Cleanup(func() { _ = stream.Close() })
			var terminal bool
			var terminalResponse *modelinvoker.Response
			for stream.Next() {
				terminal = terminal || stream.Event().Type == modelinvoker.StreamEventResponseCompleted
				if stream.Event().Type == modelinvoker.StreamEventResponseCompleted {
					terminalResponse = stream.Event().Response
				}
				if terminal {
					break
				}
			}
			nextAfterTerminal := stream.Next()
			if !terminal || nextAfterTerminal {
				t.Fatalf("terminal/next = %v/%v", terminal, nextAfterTerminal)
			}
			if terminalResponse == nil || terminalResponse.StopReason != test.wantStop {
				t.Fatalf("terminal stop reason = %#v, want %q", terminalResponse, test.wantStop)
			}
			if err := stream.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if err := stream.Close(); err != nil {
				t.Fatalf("second Close() error = %v", err)
			}
			select {
			case <-transportClosed:
			case <-time.After(time.Second):
				t.Fatal("transport was not closed after terminal event")
			}
		})
	}
}

func assertStrictlyIncreasingSequences(t *testing.T, sequences []int64) {
	t.Helper()
	if len(sequences) == 0 {
		t.Fatal("stream emitted no sequences")
	}
	previous := int64(0)
	for index, sequence := range sequences {
		if sequence <= previous {
			t.Fatalf("sequence[%d] = %d, previous = %d; sequences must be strictly increasing", index, sequence, previous)
		}
		previous = sequence
	}
}

func newSSEServer(t *testing.T, events ...string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s", request.Method)
		}
		if request.URL.Path != "/v1/responses" && request.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-only-key" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode stream request: %v", err)
		} else if body["stream"] != true {
			t.Errorf("stream request body = %#v", body)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("X-Request-Id", "req_stream")
		for _, event := range events {
			_, _ = fmt.Fprintf(writer, "data: %s\n\n", event)
		}
		_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	return server
}
