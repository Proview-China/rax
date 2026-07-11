package anthropic_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/anthropic"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

func FuzzAnthropicSuccessPayload(f *testing.F) {
	f.Add([]byte(`{"id":"msg_fuzz","type":"message","role":"assistant","model":"claude-test-model","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	f.Add([]byte(`{"id":`))
	f.Add([]byte(`{"id":"msg_unknown","type":"message","role":"assistant","model":"claude-test-model","content":[{"type":"future_block","value":1}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	f.Add([]byte{0xff, 0x00, '\n'})

	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) > 1<<20 {
			t.Skip()
		}
		const apiKey = "anthropic-fuzz-secret-key"
		var calls atomic.Int64
		client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}, "Request-Id": []string{"req_fuzz_payload"}},
				Body:       io.NopCloser(bytes.NewReader(payload)),
				Request:    request,
			}, nil
		})}
		adapter, err := provider.New(provider.Config{APIKey: apiKey, BaseURL: "https://api.anthropic.test", HTTPClient: client})
		if err != nil {
			t.Fatal(err)
		}
		response, err := adapter.Invoke(context.Background(), baseRequest())
		if calls.Load() != 1 {
			t.Fatalf("HTTP calls = %d, want 1", calls.Load())
		}
		if strings.Contains(string(response.RawRequest.Bytes()), apiKey) {
			t.Fatal("fuzz request audit leaked API key")
		}
		if err != nil {
			var sdkError *anthropicsdk.Error
			if errors.As(err, &sdkError) {
				t.Fatalf("SDK error crossed public boundary: %T", sdkError)
			}
			var invocationError *modelinvoker.Error
			if errors.As(err, &invocationError) && invocationError.Retryable {
				t.Fatalf("malformed 2xx payload became retryable: %#v", invocationError)
			}
		}
	})
}

func FuzzAnthropicSSEPayload(f *testing.F) {
	f.Add([]byte("event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_fuzz_stream","type":"message","role":"assistant","model":"claude-test-model","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":0}}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}` + "\n\n" +
		"event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n"))
	f.Add([]byte("event: content_block_delta\ndata: {\"type\":\n\n"))
	f.Add([]byte("event: future_event\ndata: {\"type\":\"future_event\"}\n\n"))

	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) > 1<<20 {
			t.Skip()
		}
		var calls atomic.Int64
		client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "Request-Id": []string{"req_fuzz_stream"}},
				Body:       io.NopCloser(bytes.NewReader(payload)),
				Request:    request,
			}, nil
		})}
		adapter, err := provider.New(provider.Config{APIKey: "fuzz-key", BaseURL: "https://api.anthropic.test", HTTPClient: client})
		if err != nil {
			t.Fatal(err)
		}
		stream, err := adapter.Stream(context.Background(), baseRequest())
		if err != nil {
			var sdkError *anthropicsdk.Error
			if errors.As(err, &sdkError) {
				t.Fatalf("SDK error crossed public boundary: %T", sdkError)
			}
			return
		}
		defer stream.Close()
		events := 0
		for stream.Next() {
			events++
			if got := fmt.Sprintf("%v", stream.Event().Raw); got != "[REDACTED]" {
				t.Fatalf("stream raw formatting = %q", got)
			}
			if events > 4096 {
				t.Fatal("finite SSE payload produced too many events")
			}
		}
		var sdkError *anthropicsdk.Error
		if errors.As(stream.Err(), &sdkError) {
			t.Fatalf("SDK error crossed public boundary: %T", sdkError)
		}
		if calls.Load() != 1 {
			t.Fatalf("HTTP calls = %d, want 1", calls.Load())
		}
	})
}

func FuzzAnthropicFunctionArguments(f *testing.F) {
	f.Add([]byte(`{"city":"Paris"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`{"city":`))
	f.Add([]byte{0xff})

	responsePayload := []byte(`{"id":"msg_fuzz_args","type":"message","role":"assistant","model":"claude-test-model","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(responsePayload)),
			Request:    request,
		}, nil
	})}
	adapter, err := provider.New(provider.Config{APIKey: "fuzz-key", BaseURL: "https://api.anthropic.test", HTTPClient: client})
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, arguments []byte) {
		if len(arguments) > 1<<20 {
			t.Skip()
		}
		request := baseRequest()
		request.Input = []modelinvoker.InputItem{
			modelinvoker.FunctionCallInput("toolu_fuzz", "get_weather", json.RawMessage(arguments)),
		}
		_, err := adapter.Invoke(context.Background(), request)
		var object map[string]json.RawMessage
		validObject := json.Unmarshal(arguments, &object) == nil && object != nil
		if validObject && err != nil {
			t.Fatalf("valid JSON object arguments rejected: %v", err)
		}
		if !validObject && err == nil {
			t.Fatal("invalid function arguments were accepted")
		}
	})
}
