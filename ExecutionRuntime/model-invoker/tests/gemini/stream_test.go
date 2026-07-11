package gemini_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func TestGenerateContentStreamOrderingDedupUsageContinuationAndClose(t *testing.T) {
	fixture := mustFixture(t, "testdata/stream-tool-thinking.sse")
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
		writer.Header().Set("content-type", "text/event-stream")
		writer.Header().Set("x-goog-request-id", "req_gemini_stream_01")
		_, _ = writer.Write(fixture)
	}))
	defer server.Close()

	request := baseRequest()
	request.Tools = weatherTools()
	stream, err := newAdapter(t, server.URL, server.Client()).Stream(context.Background(), request)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var eventTypes []modelinvoker.StreamEventType
	var text, reasoning, arguments strings.Builder
	var final *modelinvoker.Response
	var previousSequence int64
	completedCalls := 0
	usageEvents := 0
	for stream.Next() {
		event := stream.Event()
		if event.Sequence <= previousSequence {
			t.Fatalf("sequence = %d after %d", event.Sequence, previousSequence)
		}
		previousSequence = event.Sequence
		eventTypes = append(eventTypes, event.Type)
		switch event.Type {
		case modelinvoker.StreamEventTextDelta:
			text.WriteString(event.TextDelta)
		case modelinvoker.StreamEventReasoningDelta:
			reasoning.WriteString(event.ReasoningDelta)
		case modelinvoker.StreamEventFunctionArgumentsDelta:
			arguments.WriteString(event.ArgumentsDelta)
		case modelinvoker.StreamEventFunctionCallCompleted:
			completedCalls++
		case modelinvoker.StreamEventUsage:
			usageEvents++
		case modelinvoker.StreamEventResponseCompleted:
			final = event.Response
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream Err() = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	capture := <-captured
	if capture.err != nil || capture.method != http.MethodPost ||
		capture.path != "/v1beta/models/gemini-test-model:streamGenerateContent" ||
		capture.query != "alt=sse" || capture.key != testAPIKey {
		t.Fatalf("stream request = %#v", capture)
	}
	if text.String() != "Hello " || reasoning.String() != "checking weather" || arguments.String() != `{"city":"Rome"}` {
		t.Fatalf("stream deltas text/reasoning/args = %q/%q/%q", text.String(), reasoning.String(), arguments.String())
	}
	if completedCalls != 1 {
		t.Fatalf("completed function calls = %d, repeated snapshot was not deduplicated", completedCalls)
	}
	if usageEvents != 2 {
		t.Fatalf("usage events = %d, want changed initial and final totals", usageEvents)
	}
	if final == nil {
		t.Fatal("stream has no terminal response")
	}
	wantUsage := modelinvoker.Usage{
		InputTokens: 11, OutputTokens: 7, ReasoningTokens: 4,
		CacheReadTokens: 5, TotalTokens: 18,
	}
	if final.Status != modelinvoker.ResponseStatusCompleted || final.StopReason != modelinvoker.StopReasonToolCall ||
		final.Text() != "Hello " || final.Usage != wantUsage || final.RequestID != "req_gemini_stream_01" {
		t.Fatalf("terminal response = %#v, text = %q", final, final.Text())
	}
	calls := final.FunctionCalls()
	if len(calls) != 1 || calls[0].ID != "call_stream_01" || calls[0].Name != "get_weather" ||
		string(calls[0].Arguments) != `{"city":"Rome"}` {
		t.Fatalf("terminal function calls = %#v", calls)
	}
	if final.State == nil || !strings.Contains(string(final.State.Payload.Bytes()), "thoughtSignature") {
		t.Fatalf("terminal continuation = %#v", final.State)
	}
	if len(final.NativeEvents) != 4 || final.RawRequest.Empty() || final.RawResponse.Empty() {
		t.Fatalf("terminal raw/native audit = raw request %d, raw response %d, events %d", final.RawRequest.Len(), final.RawResponse.Len(), len(final.NativeEvents))
	}
	var auditRequest map[string]any
	if err := json.Unmarshal(final.RawRequest.Bytes(), &auditRequest); err != nil || auditRequest["stream"] != true {
		t.Fatalf("stream audit request = %#v, error = %v", auditRequest, err)
	}
	for _, required := range []modelinvoker.StreamEventType{
		modelinvoker.StreamEventResponseStarted,
		modelinvoker.StreamEventTextDelta,
		modelinvoker.StreamEventReasoningDelta,
		modelinvoker.StreamEventFunctionCallStarted,
		modelinvoker.StreamEventFunctionArgumentsDelta,
		modelinvoker.StreamEventFunctionCallCompleted,
		modelinvoker.StreamEventUsage,
		modelinvoker.StreamEventResponseCompleted,
	} {
		if !containsEvent(eventTypes, required) {
			t.Fatalf("missing event %q in %#v", required, eventTypes)
		}
	}
}

func TestStreamUnexpectedEOFReturnsPartialAuditedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("content-type", "text/event-stream")
		writer.Header().Set("x-goog-request-id", "req_eof")
		_, _ = writer.Write([]byte("data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"before eof\"}]},\"index\":0}],\"responseId\":\"resp_eof\"}\n\n"))
	}))
	defer server.Close()

	stream, err := newAdapter(t, server.URL, server.Client()).Stream(context.Background(), baseRequest())
	if err != nil {
		t.Fatal(err)
	}
	var partial *modelinvoker.Response
	var errorEvent *modelinvoker.Error
	for stream.Next() {
		event := stream.Event()
		if event.Type == modelinvoker.StreamEventError {
			partial = event.Response
			errorEvent = event.Error
		}
	}
	if modelinvoker.ErrorKindOf(stream.Err()) != modelinvoker.ErrorStreamInterrupted || errorEvent == nil {
		t.Fatalf("stream error = %v / %#v", stream.Err(), errorEvent)
	}
	if partial == nil || partial.Status != modelinvoker.ResponseStatusFailed || partial.Text() != "before eof" ||
		partial.RequestID != "req_eof" || len(partial.NativeEvents) != 1 || partial.RawResponse.Empty() {
		t.Fatalf("partial response = %#v", partial)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestStreamCloseCancelsSDKIteratorAndIsIdempotent(t *testing.T) {
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("content-type", "text/event-stream")
		flusher, ok := writer.(http.Flusher)
		if !ok {
			return
		}
		_, _ = writer.Write([]byte("data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"open\"}]},\"index\":0}],\"responseId\":\"resp_open\"}\n\n"))
		flusher.Flush()
		<-request.Context().Done()
		close(cancelled)
	}))
	defer server.Close()

	stream, err := newAdapter(t, server.URL, server.Client()).Stream(context.Background(), baseRequest())
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
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not cancel the SDK iterator HTTP request")
	}
	if stream.Next() {
		t.Fatal("closed stream produced another event")
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
