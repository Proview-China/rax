package anthropic_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/anthropic"
)

func TestAnthropicNonStreamRequiresAuthoritativeExactModelBeforeSuccess(t *testing.T) {
	for _, actual := range []string{"claude-sonnet-4-6", "other-model", ""} {
		name := actual
		if name == "" {
			name = "missing"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(writer, `{"id":"msg","type":"message","role":"assistant","model":%q,"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`, actual)
			}))
			defer server.Close()
			adapter, err := provider.New(provider.Config{APIKey: "offline", BaseURL: server.URL, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			response, err := adapter.Invoke(context.Background(), baseRequest())
			if actual == "claude-sonnet-4-6" {
				if err != nil || response.Model != actual {
					t.Fatalf("correct response = %#v, %v", response, err)
				}
				return
			}
			assertAnthropicModelIdentityError(t, err, actual == "")
			if response.Status != modelinvoker.ResponseStatusFailed || !response.RawResponse.Empty() {
				t.Fatalf("untrusted response leaked as success/audit: %#v", response)
			}
		})
	}
}

func TestAnthropicStreamChecksMessageStartModelBeforeAnySemanticEvent(t *testing.T) {
	tests := []struct {
		name, actual string
		beforeStart  bool
		wantText     string
	}{
		{"correct", "claude-sonnet-4-6", false, "safe"},
		{"missing", "", false, ""},
		{"mismatch", "other-model", false, ""},
		{"semantic before start", "", true, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "text/event-stream")
				if test.beforeStart {
					_, _ = fmt.Fprint(writer, "event: content_block_delta\n"+`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"unsafe"}}`+"\n\n")
					return
				}
				_, _ = fmt.Fprintf(writer, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"model\":%q,\"content\":[],\"stop_reason\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n", test.actual)
				if test.actual != "claude-sonnet-4-6" {
					return
				}
				_, _ = fmt.Fprint(writer,
					"event: content_block_start\n"+`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`+"\n\n"+
						"event: content_block_delta\n"+`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"safe"}}`+"\n\n"+
						"event: content_block_stop\n"+`data: {"type":"content_block_stop","index":0}`+"\n\n"+
						"event: message_delta\n"+`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}`+"\n\n"+
						"event: message_stop\n"+`data: {"type":"message_stop"}`+"\n\n")
			}))
			defer server.Close()
			adapter, err := provider.New(provider.Config{APIKey: "offline", BaseURL: server.URL, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			stream, err := adapter.Stream(context.Background(), baseRequest())
			if err != nil {
				t.Fatal(err)
			}
			var text string
			var errorEvent *modelinvoker.StreamEvent
			for stream.Next() {
				event := stream.Event()
				text += event.TextDelta
				if event.Type == modelinvoker.StreamEventError {
					copy := event
					errorEvent = &copy
				}
			}
			if text != test.wantText || strings.Contains(text, "unsafe") {
				t.Fatalf("delivered text = %q", text)
			}
			if test.name == "correct" {
				if err := stream.Err(); err != nil {
					t.Fatal(err)
				}
			} else {
				assertAnthropicModelIdentityError(t, stream.Err(), test.name != "mismatch")
				if errorEvent == nil || errorEvent.Response != nil || !errorEvent.Raw.Empty() {
					t.Fatalf("identity event leaked untrusted response/raw: %#v", errorEvent)
				}
			}
			if err := stream.Close(); err != nil {
				t.Fatal(err)
			}
			if err := stream.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func assertAnthropicModelIdentityError(t *testing.T, err error, missing bool) {
	t.Helper()
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError == nil || invocationError.Kind != modelinvoker.ErrorMapping {
		t.Fatalf("model identity error = %T %v", err, err)
	}
	want := "response_model_mismatch"
	if missing {
		want = "response_model_missing"
	}
	if invocationError.Code != want {
		t.Fatalf("model identity code = %q, want %q", invocationError.Code, want)
	}
}
