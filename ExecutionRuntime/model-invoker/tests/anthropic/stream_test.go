package anthropic_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/anthropic"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

func TestStreamingThinkingTextToolAndUsage(t *testing.T) {
	fixture := mustRead(t, "testdata/stream-tool-thinking.sse")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			return
		}
		if body["stream"] != true {
			t.Errorf("stream = %#v", body["stream"])
		}
		w.Header().Set("content-type", "text/event-stream")
		w.Header().Set("request-id", "req_stream_01")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := baseRequest()
	request.Tools = weatherTools()
	request.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh, Summary: modelinvoker.ReasoningSummaryAuto}
	stream, err := adapter.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	var eventTypes []modelinvoker.StreamEventType
	var reasoning, text, arguments strings.Builder
	var completed *modelinvoker.FunctionCall
	var final *modelinvoker.Response
	var sequence int64
	for stream.Next() {
		event := stream.Event()
		sequence++
		if event.Sequence != sequence {
			t.Fatalf("sequence = %d, want %d", event.Sequence, sequence)
		}
		eventTypes = append(eventTypes, event.Type)
		switch event.Type {
		case modelinvoker.StreamEventReasoningDelta:
			reasoning.WriteString(event.ReasoningDelta)
		case modelinvoker.StreamEventTextDelta:
			text.WriteString(event.TextDelta)
		case modelinvoker.StreamEventFunctionArgumentsDelta:
			arguments.WriteString(event.ArgumentsDelta)
		case modelinvoker.StreamEventFunctionCallCompleted:
			completed = event.FunctionCall
		case modelinvoker.StreamEventResponseCompleted:
			final = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream.Err() = %v", err)
	}
	if reasoning.String() != "I should use a tool." || text.String() != "Checking weather." || arguments.String() != `{"city":"Paris"}` {
		t.Fatalf("deltas reasoning=%q text=%q args=%q", reasoning.String(), text.String(), arguments.String())
	}
	if completed == nil || completed.ID != "toolu_stream_01" || string(completed.Arguments) != `{"city":"Paris"}` {
		t.Fatalf("completed call = %#v", completed)
	}
	if final == nil || final.Status != modelinvoker.ResponseStatusCompleted || final.StopReason != modelinvoker.StopReasonToolCall ||
		final.State == nil || final.RequestID != "req_stream_01" {
		t.Fatalf("final response = %#v", final)
	}
	if final.Usage.InputTokens != 15 || final.Usage.OutputTokens != 12 || final.Usage.ReasoningTokens != 5 || final.Usage.TotalTokens != 27 {
		t.Fatalf("final usage = %#v", final.Usage)
	}
	if !containsEvent(eventTypes, modelinvoker.StreamEventResponseStarted) ||
		!containsEvent(eventTypes, modelinvoker.StreamEventUsage) ||
		!containsEvent(eventTypes, modelinvoker.StreamEventFunctionCallStarted) {
		t.Fatalf("event types = %#v", eventTypes)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
}

func TestStreamErrorPreservesPartialTextAndCompletedTool(t *testing.T) {
	sse := strings.Join([]string{
		"event: message_start\n",
		`data: {"type":"message_start","message":{"id":"msg_partial","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":5,"output_tokens":1}}}` + "\n\n",
		"event: content_block_start\n",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n",
		"event: content_block_delta\n",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial text"}}` + "\n\n",
		"event: content_block_stop\n",
		`data: {"type":"content_block_stop","index":0}` + "\n\n",
		"event: content_block_start\n",
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_partial","name":"get_weather","input":{},"caller":{"type":"direct"}}}` + "\n\n",
		"event: content_block_delta\n",
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":\"Paris\"}"}}` + "\n\n",
		"event: content_block_stop\n",
		`data: {"type":"content_block_stop","index":1}` + "\n\n",
		"event: error\n",
		`data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"},"request_id":"req_stream_body"}` + "\n\n",
	}, "")
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("content-type", "text/event-stream")
		w.Header().Set("request-id", "req_stream_error")
		_, _ = w.Write([]byte(sse))
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "secret-test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := baseRequest()
	request.Tools = weatherTools()
	stream, err := adapter.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var errorEvent *modelinvoker.StreamEvent
	for stream.Next() {
		event := stream.Event()
		if event.Type == modelinvoker.StreamEventError {
			copy := event
			errorEvent = &copy
		}
	}
	if calls != 1 {
		t.Fatalf("HTTP calls = %d, want 1", calls)
	}
	if errorEvent == nil || errorEvent.Response == nil || errorEvent.Response.Status != modelinvoker.ResponseStatusFailed {
		t.Fatalf("error event = %#v", errorEvent)
	}
	if errorEvent.Response.Text() != "partial text" || len(errorEvent.Response.FunctionCalls()) != 1 ||
		errorEvent.Response.FunctionCalls()[0].ID != "toolu_partial" {
		t.Fatalf("partial response = %#v", errorEvent.Response)
	}
	if errorEvent.Response.State == nil || errorEvent.Response.State.Kind != modelinvoker.StateProviderContinuation ||
		!strings.Contains(string(errorEvent.Response.State.Payload.Bytes()), "toolu_partial") {
		t.Fatalf("partial continuation = %#v", errorEvent.Response.State)
	}
	if modelinvoker.ErrorKindOf(stream.Err()) != modelinvoker.ErrorProviderUnavailable {
		t.Fatalf("stream error = %v", stream.Err())
	}
	var sdkError *anthropicsdk.Error
	if errors.As(stream.Err(), &sdkError) {
		t.Fatal("stream error unwrap chain exposes *anthropic.Error")
	}
	if strings.Contains(string(errorEvent.Response.RawRequest.Bytes()), "secret-test-key") ||
		strings.Contains(string(errorEvent.Response.RawResponse.Bytes()), "secret-test-key") ||
		strings.Contains(stream.Err().Error(), "secret-test-key") {
		t.Fatal("stream failure audit leaked API key")
	}
}

func TestStreamErrorPreservesThinkingSignatureContinuation(t *testing.T) {
	sse := strings.Join([]string{
		"event: message_start\n",
		`data: {"type":"message_start","message":{"id":"msg_thinking_partial","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":3,"output_tokens":1}}}` + "\n\n",
		"event: content_block_start\n",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}` + "\n\n",
		"event: content_block_delta\n",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"partial thought"}}` + "\n\n",
		"event: content_block_delta\n",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig_partial_01"}}` + "\n\n",
		"event: content_block_stop\n",
		`data: {"type":"content_block_stop","index":0}` + "\n\n",
		"event: content_block_start\n",
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}` + "\n\n",
		"event: content_block_delta\n",
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"unfinished text"}}` + "\n\n",
		"event: error\n",
		`data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"},"request_id":"req_thinking_partial"}` + "\n\n",
	}, "")
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("content-type", "text/event-stream")
		w.Header().Set("request-id", "req_thinking_header")
		_, _ = w.Write([]byte(sse))
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := baseRequest()
	request.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
	stream, err := adapter.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var partial *modelinvoker.Response
	for stream.Next() {
		if event := stream.Event(); event.Type == modelinvoker.StreamEventError {
			partial = event.Response
		}
	}
	if modelinvoker.ErrorKindOf(stream.Err()) != modelinvoker.ErrorProviderUnavailable || calls.Load() != 1 {
		t.Fatalf("stream error/calls = %v/%d", stream.Err(), calls.Load())
	}
	if partial == nil || partial.State == nil ||
		!strings.Contains(string(partial.State.Payload.Bytes()), "sig_partial_01") ||
		!strings.Contains(string(partial.State.Payload.Bytes()), "partial thought") ||
		strings.Contains(string(partial.State.Payload.Bytes()), "unfinished text") {
		t.Fatalf("thinking partial continuation = %#v", partial)
	}
	if partial.Text() != "unfinished text" {
		t.Fatalf("visible partial text = %q", partial.Text())
	}
}

func TestUnexpectedEOFProducesFailedPartialResponse(t *testing.T) {
	sse := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_eof","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"before eof"}}` + "\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := adapter.Stream(context.Background(), baseRequest())
	if err != nil {
		t.Fatal(err)
	}
	var partial *modelinvoker.Response
	for stream.Next() {
		if event := stream.Event(); event.Type == modelinvoker.StreamEventError {
			partial = event.Response
		}
	}
	if modelinvoker.ErrorKindOf(stream.Err()) != modelinvoker.ErrorStreamInterrupted {
		t.Fatalf("stream error = %v", stream.Err())
	}
	if partial == nil || partial.Status != modelinvoker.ResponseStatusFailed || partial.Text() != "before eof" || partial.State != nil {
		t.Fatalf("partial response = %#v", partial)
	}
	if !strings.Contains(stream.Err().Error(), "unfinished content blocks") {
		t.Fatalf("stream error = %v", stream.Err())
	}
}

func TestStreamRejectsInvalidContentBlockLifecycle(t *testing.T) {
	tests := []struct {
		name   string
		events string
		want   string
	}{
		{
			name: "duplicate start",
			events: sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
				sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
			want: "started more than once",
		},
		{
			name:   "unknown delta index",
			events: sseEvent("content_block_delta", `{"type":"content_block_delta","index":7,"delta":{"type":"text_delta","text":"orphan"}}`),
			want:   "no matching content block start",
		},
		{
			name:   "unknown stop index",
			events: sseEvent("content_block_stop", `{"type":"content_block_stop","index":7}`),
			want:   "no matching content block start",
		},
		{
			name: "delta type mismatch",
			events: sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
				sseEvent("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"wrong"}}`),
			want: "does not match text content block",
		},
		{
			name: "delta after stop",
			events: sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
				sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`) +
				sseEvent("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"late"}}`),
			want: "targets a closed text block",
		},
		{
			name: "duplicate stop",
			events: sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
				sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`) +
				sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`),
			want: "stopped more than once",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events, err := runSSE(t, streamMessageStart("msg_invalid_lifecycle")+test.events, baseRequest())
			if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("stream error = %v, want mapping containing %q", err, test.want)
			}
			assertFailedWithoutCompletion(t, events)
		})
	}
}

func TestStreamRejectsInvalidMessageLifecycle(t *testing.T) {
	tests := []struct {
		name   string
		events string
		want   string
	}{
		{
			name: "missing message start",
			events: streamMessageDelta("end_turn") +
				sseEvent("message_stop", `{"type":"message_stop"}`),
			want: "message_delta arrived before message_start",
		},
		{
			name:   "duplicate message start",
			events: streamMessageStart("msg_first") + streamMessageStart("msg_second"),
			want:   "message_start arrived more than once",
		},
		{
			name:   "message stop before delta",
			events: streamMessageStart("msg_no_delta") + sseEvent("message_stop", `{"type":"message_stop"}`),
			want:   "message_stop arrived before message_delta",
		},
		{
			name: "duplicate message delta",
			events: streamMessageStart("msg_duplicate_delta") +
				streamMessageDelta("end_turn") + streamMessageDelta("end_turn"),
			want: "message_delta arrived more than once",
		},
		{
			name: "message delta with open block",
			events: streamMessageStart("msg_open_at_delta") +
				sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`) +
				streamMessageDelta("end_turn"),
			want: "message_delta arrived with unfinished content blocks",
		},
		{
			name: "content after message delta",
			events: streamMessageStart("msg_content_after_delta") +
				streamMessageDelta("end_turn") +
				sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`),
			want: "content block start arrived after message_delta",
		},
		{
			name:   "pre-populated message start content",
			events: sseEvent("message_start", `{"type":"message_start","message":{"id":"msg_prepopulated","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"not streamed"}],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}}`),
			want:   "message_start must not contain pre-populated content blocks",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events, err := runSSE(t, test.events, baseRequest())
			if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("stream error = %v, want mapping containing %q", err, test.want)
			}
			assertFailedWithoutCompletion(t, events)
		})
	}
}

func TestStreamNonDirectToolCallerNeverEmitsUnifiedFunctionCall(t *testing.T) {
	start := streamMessageStart("msg_non_direct_tool")
	toolStart := sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_non_direct","name":"get_weather","input":{},"caller":{"type":"code_execution_20260120","tool_id":"srvtool_01"}}}`)

	t.Run("strict rejection", func(t *testing.T) {
		events, err := runSSE(t, start+toolStart, baseRequest())
		if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping || !strings.Contains(err.Error(), "non-direct Anthropic tool use caller") {
			t.Fatalf("stream error = %v", err)
		}
		assertNoUnifiedFunctionEvents(t, events)
		assertFailedWithoutCompletion(t, events)
	})

	t.Run("explicit degradation cannot reinterpret caller", func(t *testing.T) {
		request := baseRequest()
		request.AllowDegradation = true
		events, err := runSSE(t, start+toolStart, request)
		if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping || !strings.Contains(err.Error(), "non-direct Anthropic tool use caller") {
			t.Fatalf("stream error = %v", err)
		}
		assertNoUnifiedFunctionEvents(t, events)
		assertFailedWithoutCompletion(t, events)
	})
}

func TestMessageStopRejectsEveryUnclosedContentBlockType(t *testing.T) {
	starts := map[string]string{
		"text":              `{"type":"text","text":""}`,
		"thinking":          `{"type":"thinking","thinking":"","signature":""}`,
		"redacted_thinking": `{"type":"redacted_thinking","data":"opaque"}`,
		"tool_use":          `{"type":"tool_use","id":"toolu_open","name":"get_weather","input":{},"caller":{"type":"direct"}}`,
	}
	for name, block := range starts {
		t.Run(name, func(t *testing.T) {
			sse := streamMessageStart("msg_unclosed_"+name) +
				sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":`+block+`}`) +
				sseEvent("message_stop", `{"type":"message_stop"}`)
			events, err := runSSE(t, sse, baseRequest())
			if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping || !strings.Contains(err.Error(), "message_stop arrived with unfinished content blocks") {
				t.Fatalf("stream error = %v", err)
			}
			errorEvent := assertFailedWithoutCompletion(t, events)
			if errorEvent.Response.State != nil {
				t.Fatalf("unfinished %s block produced continuation %#v", name, errorEvent.Response.State)
			}
		})
	}
}

func TestThinkingStopWithoutSignatureIsRejected(t *testing.T) {
	sse := streamMessageStart("msg_missing_signature") +
		sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`) +
		sseEvent("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"unsigned thought"}}`) +
		sseEvent("content_block_stop", `{"type":"content_block_stop","index":0}`)
	events, err := runSSE(t, sse, baseRequest())
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorMapping || !strings.Contains(err.Error(), "without a complete signature") {
		t.Fatalf("stream error = %v", err)
	}
	errorEvent := assertFailedWithoutCompletion(t, events)
	if errorEvent.Response.State != nil {
		t.Fatalf("incomplete signature produced continuation %#v", errorEvent.Response.State)
	}
}

func TestUnclosedThinkingSignatureIsNotUsedAsContinuation(t *testing.T) {
	sse := streamMessageStart("msg_partial_signature") +
		sseEvent("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`) +
		sseEvent("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"partial signed thought"}}`) +
		sseEvent("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig_prefix_only"}}`)
	events, err := runSSE(t, sse, baseRequest())
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorStreamInterrupted || !strings.Contains(err.Error(), "unfinished content blocks") {
		t.Fatalf("stream error = %v", err)
	}
	errorEvent := assertFailedWithoutCompletion(t, events)
	if errorEvent.Response.State != nil || len(errorEvent.Response.Output) != 1 ||
		errorEvent.Response.Output[0].ReasoningSummary != "partial signed thought" {
		t.Fatalf("partial signature response = %#v", errorEvent.Response)
	}
}

func TestStreamCloseAndCancellationStopTransportWithoutReplay(t *testing.T) {
	t.Run("early close is idempotent and closes transport", func(t *testing.T) {
		var calls atomic.Int64
		server, transportClosed := newBlockingAnthropicSSEServer(t, &calls)
		defer server.Close()
		adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		stream, err := adapter.Stream(context.Background(), baseRequest())
		if err != nil {
			t.Fatal(err)
		}
		if err := stream.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		if err := stream.Close(); err != nil {
			t.Fatalf("second Close() error = %v", err)
		}
		select {
		case <-transportClosed:
		case <-time.After(2 * time.Second):
			t.Fatal("transport was not closed after early Close")
		}
		next := stream.Next()
		if next || calls.Load() != 1 {
			t.Fatalf("Next/calls after close = %v/%d", next, calls.Load())
		}
	})

	t.Run("context cancellation is classified and not replayed", func(t *testing.T) {
		var calls atomic.Int64
		server, transportClosed := newBlockingAnthropicSSEServer(t, &calls)
		defer server.Close()
		adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		stream, err := adapter.Stream(ctx, baseRequest())
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		done := make(chan struct{})
		go func() {
			for stream.Next() {
			}
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = stream.Close()
			t.Fatal("cancelled stream did not stop")
		}
		if modelinvoker.ErrorKindOf(stream.Err()) != modelinvoker.ErrorCancelled {
			t.Fatalf("cancelled stream error = %v", stream.Err())
		}
		select {
		case <-transportClosed:
		case <-time.After(2 * time.Second):
			t.Fatal("transport was not closed after cancellation")
		}
		if calls.Load() != 1 {
			t.Fatalf("HTTP calls = %d, want 1", calls.Load())
		}
		if err := stream.Close(); err != nil {
			t.Fatalf("Close() after cancellation = %v", err)
		}
	})
}

func newBlockingAnthropicSSEServer(t *testing.T, calls *atomic.Int64) (*httptest.Server, <-chan struct{}) {
	t.Helper()
	transportClosed := make(chan struct{})
	var closeOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("content-type", "text/event-stream")
		w.Header().Set("request-id", "req_blocking_stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("response writer does not support flushing")
			return
		}
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		<-r.Context().Done()
		closeOnce.Do(func() { close(transportClosed) })
	}))
	return server, transportClosed
}

func streamMessageStart(id string) string {
	return sseEvent("message_start", `{"type":"message_start","message":{"id":"`+id+`","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}}`)
}

func streamMessageDelta(reason string) string {
	return sseEvent("message_delta", `{"type":"message_delta","delta":{"stop_reason":"`+reason+`","stop_sequence":null},"usage":{"output_tokens":1}}`)
}

func sseEvent(name, data string) string {
	return "event: " + name + "\n" + "data: " + data + "\n\n"
}

func runSSE(t *testing.T, sse string, request modelinvoker.Request) ([]modelinvoker.StreamEvent, error) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		w.Header().Set("request-id", "req_stream_state_machine")
		_, _ = w.Write([]byte(sse))
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := adapter.Stream(context.Background(), request)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()
	var events []modelinvoker.StreamEvent
	for stream.Next() {
		events = append(events, stream.Event())
	}
	return events, stream.Err()
}

func assertFailedWithoutCompletion(t *testing.T, events []modelinvoker.StreamEvent) *modelinvoker.StreamEvent {
	t.Helper()
	var failure *modelinvoker.StreamEvent
	for index := range events {
		switch events[index].Type {
		case modelinvoker.StreamEventError:
			failure = &events[index]
		case modelinvoker.StreamEventResponseCompleted:
			t.Fatalf("invalid stream emitted ResponseCompleted: %#v", events[index])
		}
	}
	if failure == nil || failure.Response == nil || failure.Response.Status != modelinvoker.ResponseStatusFailed {
		t.Fatalf("stream events do not contain a failed response: %#v", events)
	}
	return failure
}

func assertNoUnifiedFunctionEvents(t *testing.T, events []modelinvoker.StreamEvent) {
	t.Helper()
	for _, event := range events {
		switch event.Type {
		case modelinvoker.StreamEventFunctionCallStarted,
			modelinvoker.StreamEventFunctionArgumentsDelta,
			modelinvoker.StreamEventFunctionCallCompleted:
			t.Fatalf("non-direct tool caller emitted unified function event: %#v", event)
		}
	}
}

func containsEvent(events []modelinvoker.StreamEventType, target modelinvoker.StreamEventType) bool {
	for _, event := range events {
		if event == target {
			return true
		}
	}
	return false
}
