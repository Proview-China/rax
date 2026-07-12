package openai_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/openai"
)

func TestOpenAINonStreamRequiresAuthoritativeExactModelBeforeSuccess(t *testing.T) {
	for _, protocolID := range []modelinvoker.Protocol{modelinvoker.ProtocolChatCompletions, modelinvoker.ProtocolResponses} {
		for _, actual := range []string{"test-model", "other-model", ""} {
			name := string(protocolID) + "/" + actual
			if actual == "" {
				name = string(protocolID) + "/missing"
			}
			t.Run(name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					writer.Header().Set("Content-Type", "application/json")
					if protocolID == modelinvoker.ProtocolResponses {
						_, _ = fmt.Fprintf(writer, `{"id":"resp","model":%q,"status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}`, actual)
						return
					}
					_, _ = fmt.Fprintf(writer, `{"id":"chat","model":%q,"choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, actual)
				}))
				defer server.Close()
				adapter, err := provider.New(provider.Config{APIKey: "offline", BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
				if err != nil {
					t.Fatal(err)
				}
				request := basePublicRequest(protocolID)
				request.Endpoint = server.URL + "/v1"
				response, err := adapter.Invoke(context.Background(), request)
				if actual == request.Model {
					if err != nil || response.Model != request.Model {
						t.Fatalf("correct actual model = %#v, %v", response, err)
					}
					return
				}
				assertModelIdentityError(t, err, actual == "")
				if response.Status != modelinvoker.ResponseStatusFailed || !response.RawResponse.Empty() {
					t.Fatalf("untrusted non-stream response leaked as success/audit: %#v", response)
				}
			})
		}
	}
}

func TestOpenAIChatStreamChecksFirstModelBeforeSemanticsAndRejectsCrossChunkDrift(t *testing.T) {
	tests := []struct {
		name   string
		events []string
		want   string
	}{
		{"correct", []string{
			`{"id":"chat","model":"test-model","choices":[{"index":0,"delta":{"content":"safe"},"finish_reason":null}]}`,
			`{"id":"chat","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		}, "safe"},
		{"missing first", []string{`{"id":"chat","choices":[{"index":0,"delta":{"content":"unsafe"},"finish_reason":null}]}`}, ""},
		{"mismatch first", []string{`{"id":"chat","model":"other-model","choices":[{"index":0,"delta":{"content":"unsafe"},"finish_reason":null}]}`}, ""},
		{"cross chunk", []string{
			`{"id":"chat","model":"test-model","choices":[{"index":0,"delta":{"content":"safe"},"finish_reason":null}]}`,
			`{"id":"chat","model":"other-model","choices":[{"index":0,"delta":{"content":"unsafe"},"finish_reason":"stop"}]}`,
		}, "safe"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stream := openAIIdentityStream(t, modelinvoker.ProtocolChatCompletions, test.events)
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
			if text != test.want || strings.Contains(text, "unsafe") {
				t.Fatalf("delivered text = %q, want %q without unsafe content", text, test.want)
			}
			if test.name == "correct" {
				if err := stream.Err(); err != nil {
					t.Fatal(err)
				}
			} else {
				assertModelIdentityError(t, stream.Err(), test.name == "missing first")
				if errorEvent == nil || errorEvent.Response != nil || !errorEvent.Raw.Empty() {
					t.Fatalf("identity error event leaked untrusted response/raw: %#v", errorEvent)
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

func TestOpenAIResponsesStreamRequiresCreatedModelBeforeSemanticEvents(t *testing.T) {
	tests := []struct {
		name   string
		events []string
		want   string
	}{
		{"correct", []string{
			`{"type":"response.created","sequence_number":1,"response":{"id":"resp","model":"test-model","status":"in_progress","output":[]}}`,
			`{"type":"response.output_text.delta","sequence_number":2,"delta":"safe"}`,
			`{"type":"response.completed","sequence_number":3,"response":{"id":"resp","model":"test-model","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
		}, "safe"},
		{"missing created", []string{`{"type":"response.created","sequence_number":1,"response":{"id":"resp","status":"in_progress","output":[]}}`}, ""},
		{"mismatch created", []string{`{"type":"response.created","sequence_number":1,"response":{"id":"resp","model":"other-model","status":"in_progress","output":[]}}`}, ""},
		{"semantic before created", []string{`{"type":"response.output_text.delta","sequence_number":1,"delta":"unsafe"}`}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stream := openAIIdentityStream(t, modelinvoker.ProtocolResponses, test.events)
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
			if text != test.want || strings.Contains(text, "unsafe") {
				t.Fatalf("delivered text = %q", text)
			}
			if test.name == "correct" {
				if err := stream.Err(); err != nil {
					t.Fatal(err)
				}
			} else {
				assertModelIdentityError(t, stream.Err(), test.name != "mismatch created")
				if errorEvent == nil || errorEvent.Response != nil || !errorEvent.Raw.Empty() {
					t.Fatalf("identity error event leaked untrusted response/raw: %#v", errorEvent)
				}
			}
			_ = stream.Close()
			_ = stream.Close()
		})
	}
}

func openAIIdentityStream(t *testing.T, protocolID modelinvoker.Protocol, events []string) modelinvoker.Stream {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		for _, event := range events {
			_, _ = fmt.Fprintf(writer, "data: %s\n\n", event)
		}
	}))
	t.Cleanup(server.Close)
	adapter, err := provider.New(provider.Config{APIKey: "offline", BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := basePublicRequest(protocolID)
	request.Endpoint = server.URL + "/v1"
	stream, err := adapter.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

func assertModelIdentityError(t *testing.T, err error, missing bool) {
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
