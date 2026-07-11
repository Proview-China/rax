package anthropic_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/anthropic"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

func TestRedirectsNeverSendAnthropicAPIKeyToSecondHop(t *testing.T) {
	secret := "anthropic-redirect-secret"
	tests := []struct {
		name       string
		status     int
		tlsFirst   bool
		sameOrigin bool
	}{
		{name: "same origin", status: http.StatusFound, sameOrigin: true},
		{name: "cross origin", status: http.StatusTemporaryRedirect},
		{name: "HTTPS downgrade", status: http.StatusPermanentRedirect, tlsFirst: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var secondCalls atomic.Int64
			var secondKey atomic.Value
			secondKey.Store("")
			second := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				secondCalls.Add(1)
				secondKey.Store(request.Header.Get("x-api-key"))
			}))
			t.Cleanup(second.Close)

			var target string
			firstKey := make(chan string, 1)
			handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/second-hop" {
					secondCalls.Add(1)
					secondKey.Store(request.Header.Get("x-api-key"))
					return
				}
				firstKey <- request.Header.Get("x-api-key")
				http.Redirect(writer, request, target, test.status)
			})
			var first *httptest.Server
			if test.tlsFirst {
				first = httptest.NewTLSServer(handler)
			} else {
				first = httptest.NewServer(handler)
			}
			t.Cleanup(first.Close)
			if test.sameOrigin {
				target = first.URL + "/second-hop"
			} else {
				target = second.URL + "/second-hop"
			}

			adapter, err := provider.New(provider.Config{APIKey: secret, BaseURL: first.URL, HTTPClient: first.Client()})
			if err != nil {
				t.Fatal(err)
			}
			response, err := adapter.Invoke(context.Background(), baseRequest())
			invocationError := anthropicErrorKind(t, err, modelinvoker.ErrorProvider)
			if invocationError.HTTPStatus != test.status || invocationError.Retryable {
				t.Fatalf("redirect error = %#v", invocationError)
			}
			if got := <-firstKey; got != secret {
				t.Fatalf("first-hop x-api-key = %q", got)
			}
			if secondCalls.Load() != 0 || secondKey.Load().(string) != "" {
				t.Fatalf("second-hop calls/key = %d/%q", secondCalls.Load(), secondKey.Load())
			}
			assertAnthropicResponseNoSecret(t, response, secret)
			assertAnthropicErrorNoSecret(t, err, secret)
		})
	}
}

func TestMaliciousAnthropicErrorAndMalformedResponsesAreRedacted(t *testing.T) {
	secret := "anthropic-loopback-secret"
	tests := []struct {
		name     string
		status   int
		body     string
		wantKind modelinvoker.ErrorKind
	}{
		{
			name: "JSON error", status: http.StatusBadRequest,
			body:     `{"type":"error","error":{"type":"invalid_request_error","message":"anthropic-loopback-secret"},"request_id":"anthropic-loopback-secret"}`,
			wantKind: modelinvoker.ErrorInvalidRequest,
		},
		{name: "malformed 2xx", status: http.StatusOK, body: `{"id":"anthropic-loopback-secret"`, wantKind: modelinvoker.ErrorProvider},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("content-type", "application/json")
				writer.Header().Set("request-id", secret)
				writer.Header().Set("anthropic-ratelimit-requests-remaining", secret)
				writer.WriteHeader(test.status)
				_, _ = fmt.Fprint(writer, test.body)
			}))
			t.Cleanup(server.Close)
			adapter, err := provider.New(provider.Config{APIKey: secret, BaseURL: server.URL, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			response, err := adapter.Invoke(context.Background(), baseRequest())
			anthropicErrorKind(t, err, test.wantKind)
			if response.RawResponse.Empty() || !strings.Contains(string(response.RawResponse.Bytes()), "[REDACTED]") {
				t.Fatalf("captured RawResponse = %q", response.RawResponse.Bytes())
			}
			assertAnthropicResponseNoSecret(t, response, secret)
			assertAnthropicErrorNoSecret(t, err, secret)
		})
	}
}

func TestMaliciousAnthropicSuccessfulResponseIsRedacted(t *testing.T) {
	secret := "anthropic-success-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("content-type", "application/json")
		writer.Header().Set("request-id", secret)
		writer.Header().Set("anthropic-ratelimit-requests-remaining", secret)
		_, _ = fmt.Fprint(writer, `{"id":"anthropic-success-secret","type":"message","role":"assistant","model":"anthropic-success-secret","content":[{"type":"text","text":"anthropic-success-secret"}],"stop_reason":"end_turn","stop_sequence":"anthropic-success-secret","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	t.Cleanup(server.Close)
	adapter, err := provider.New(provider.Config{APIKey: secret, BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Invoke(context.Background(), baseRequest())
	if err != nil {
		t.Fatal(err)
	}
	assertAnthropicResponseNoSecret(t, response, secret)
}

func TestMaliciousAnthropicStreamEventsStateAndErrorsAreRedacted(t *testing.T) {
	secret := "anthropic-stream-secret"
	sse := strings.Join([]string{
		"event: message_start\n",
		`data: {"type":"message_start","message":{"id":"anthropic-stream-secret","type":"message","role":"assistant","model":"anthropic-stream-secret","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n",
		"event: content_block_start\n",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}` + "\n\n",
		"event: content_block_delta\n",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"anthropic-stream-"}}` + "\n\n",
		"event: content_block_delta\n",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"secret"}}` + "\n\n",
		"event: content_block_delta\n",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"anthropic-stream-secret"}}` + "\n\n",
		"event: content_block_stop\n",
		`data: {"type":"content_block_stop","index":0}` + "\n\n",
		"event: error\n",
		`data: {"type":"error","error":{"type":"overloaded_error","message":"anthropic-stream-secret"},"request_id":"anthropic-stream-secret"}` + "\n\n",
	}, "")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("content-type", "text/event-stream")
		writer.Header().Set("request-id", secret)
		writer.Header().Set("anthropic-ratelimit-requests-remaining", secret)
		_, _ = writer.Write([]byte(sse))
	}))
	t.Cleanup(server.Close)
	adapter, err := provider.New(provider.Config{APIKey: secret, BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := baseRequest()
	request.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
	stream, err := adapter.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var errorEvent *modelinvoker.StreamEvent
	var reasoning, raw, native string
	for stream.Next() {
		event := stream.Event()
		assertAnthropicEventNoSecret(t, event, secret)
		reasoning += event.ReasoningDelta
		raw += string(event.Raw.Bytes())
		if event.Type == modelinvoker.StreamEventError {
			copy := event
			errorEvent = &copy
			if event.Response != nil {
				for _, item := range event.Response.NativeEvents {
					native += string(item.Bytes())
				}
			}
		}
	}
	if errorEvent == nil || errorEvent.Response == nil || errorEvent.Response.State == nil ||
		errorEvent.Response.RawResponse.Empty() || len(errorEvent.Response.NativeEvents) == 0 {
		t.Fatalf("stream error event = %#v", errorEvent)
	}
	assertAnthropicStringsNoSecret(t, secret, reasoning, raw, native)
	assertAnthropicErrorNoSecret(t, stream.Err(), secret)
}

func TestAnthropicResponseBodyLimitIsNonRetryableForInvokeAndStream(t *testing.T) {
	secret := "anthropic-body-limit-secret"

	t.Run("oversized 2xx", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("content-type", "application/json")
			_, _ = io.CopyN(writer, anthropicRepeatingReader('x'), adaptercore.DefaultMaxResponseBodyBytes+1)
		}))
		t.Cleanup(server.Close)
		adapter, err := provider.New(provider.Config{APIKey: secret, BaseURL: server.URL, HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		response, err := adapter.Invoke(context.Background(), baseRequest())
		assertAnthropicResponseBodyLimit(t, err, secret, server.URL)
		if response.RawResponse.Len() > int(adaptercore.DefaultMaxResponseBodyBytes) {
			t.Fatalf("RawResponse length = %d", response.RawResponse.Len())
		}
	})

	t.Run("unbounded SSE", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("content-type", "text/event-stream")
			writer.WriteHeader(http.StatusOK)
			writer.(http.Flusher).Flush()
			_, _ = io.CopyN(writer, anthropicRepeatingReader('s'), adaptercore.DefaultMaxResponseBodyBytes+1)
		}))
		t.Cleanup(server.Close)
		adapter, err := provider.New(provider.Config{APIKey: secret, BaseURL: server.URL, HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		stream, err := adapter.Stream(context.Background(), baseRequest())
		if err != nil {
			assertAnthropicResponseBodyLimit(t, err, secret, server.URL)
			return
		}
		var eventErr error
		for stream.Next() {
			event := stream.Event()
			assertAnthropicEventNoSecret(t, event, secret)
			if event.Error != nil {
				eventErr = event.Error
			}
		}
		assertAnthropicResponseBodyLimit(t, eventErr, secret, server.URL)
		assertAnthropicResponseBodyLimit(t, stream.Err(), secret, server.URL)
		if closeErr := stream.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	})
}

func anthropicErrorKind(t *testing.T, err error, want modelinvoker.ErrorKind) *modelinvoker.Error {
	t.Helper()
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError == nil || invocationError.Kind != want {
		t.Fatalf("error = %#v, want kind %q", err, want)
	}
	return invocationError
}

func assertAnthropicEventNoSecret(t *testing.T, event modelinvoker.StreamEvent, secret string) {
	t.Helper()
	assertAnthropicStringsNoSecret(t, secret,
		string(event.Type), event.ResponseID, event.TextDelta, event.ReasoningDelta, event.ArgumentsDelta, string(event.Raw.Bytes()),
	)
	if event.FunctionCall != nil {
		assertAnthropicStringsNoSecret(t, secret, event.FunctionCall.ID, event.FunctionCall.Name, string(event.FunctionCall.Arguments))
	}
	if event.Response != nil {
		assertAnthropicResponseNoSecret(t, *event.Response, secret)
	}
	if event.Error != nil {
		assertAnthropicErrorNoSecret(t, event.Error, secret)
	}
}

func assertAnthropicResponseNoSecret(t *testing.T, response modelinvoker.Response, secret string) {
	t.Helper()
	values := []string{
		response.ID, string(response.Provider), string(response.Protocol), response.Model, response.StopSequence, response.RequestID,
		string(response.RawRequest.Bytes()), string(response.RawResponse.Bytes()), response.MappingReport.Endpoint,
	}
	for _, output := range response.Output {
		values = append(values, output.Text, output.ReasoningSummary)
		if output.FunctionCall != nil {
			values = append(values, output.FunctionCall.ID, output.FunctionCall.Name, string(output.FunctionCall.Arguments))
		}
	}
	for key, value := range response.Metadata {
		values = append(values, key, value)
	}
	for key, value := range response.ProviderMetadata {
		values = append(values, key, value)
	}
	if response.State != nil {
		values = append(values, response.State.ID, string(response.State.Payload.Bytes()))
	}
	for _, decision := range response.MappingReport.Decisions {
		values = append(values, decision.Detail)
	}
	for _, raw := range response.NativeEvents {
		values = append(values, string(raw.Bytes()))
	}
	assertAnthropicStringsNoSecret(t, secret, values...)
}

func assertAnthropicErrorNoSecret(t *testing.T, err error, secret string) {
	t.Helper()
	if err == nil {
		return
	}
	assertAnthropicStringsNoSecret(t, secret, err.Error(), fmt.Sprintf("%v", err), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err))
	var invocationError *modelinvoker.Error
	if errors.As(err, &invocationError) {
		assertAnthropicStringsNoSecret(t, secret,
			string(invocationError.Kind), string(invocationError.Provider), invocationError.Operation,
			invocationError.Code, invocationError.Message, invocationError.RequestID, invocationError.MappingReport.Endpoint,
		)
		if invocationError.Err != nil {
			assertAnthropicStringsNoSecret(t, secret, invocationError.Err.Error(), fmt.Sprintf("%#v", invocationError.Err))
		}
	}
	var sdkError *anthropicsdk.Error
	if errors.As(err, &sdkError) {
		t.Fatalf("Anthropic SDK error crossed public boundary: %T", sdkError)
	}
}

func assertAnthropicStringsNoSecret(t *testing.T, secret string, values ...string) {
	t.Helper()
	encoded, _ := json.Marshal(secret)
	forms := []string{secret, string(encoded[1 : len(encoded)-1]), url.QueryEscape(secret), url.PathEscape(secret)}
	for _, value := range values {
		for _, form := range forms {
			if form != "" && strings.Contains(value, form) {
				t.Fatalf("public value contains API key form %q: %s", form, value)
			}
		}
	}
}

type anthropicRepeatingReader byte

func (r anthropicRepeatingReader) Read(destination []byte) (int, error) {
	for index := range destination {
		destination[index] = byte(r)
	}
	return len(destination), nil
}

func assertAnthropicResponseBodyLimit(t *testing.T, err error, secret, endpoint string) {
	t.Helper()
	invocationError := anthropicErrorKind(t, err, modelinvoker.ErrorProvider)
	if invocationError.Code != adaptercore.ResponseBodyLimitErrorCode || invocationError.Retryable || invocationError.Provider != provider.ProviderID {
		t.Fatalf("response-body limit error = %#v", invocationError)
	}
	if invocationError.Err != nil {
		t.Fatalf("response-body limit retained cause: %#v", err)
	}
	if got := invocationError.MappingReport.Endpoint; got != "" && !strings.HasPrefix(got, endpoint) && !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("response-body limit endpoint = %q, want audited endpoint", got)
	}
	assertAnthropicErrorNoSecret(t, err, secret)
}
