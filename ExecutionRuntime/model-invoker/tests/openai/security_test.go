package openai_test

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
	openaiadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/openai"
	openaisdk "github.com/openai/openai-go/v3"
)

func TestPublicChatBindingRestoresIdentityAfterRedaction(t *testing.T) {
	secret := string(openaiadapter.ProviderID)

	t.Run("validation error", func(t *testing.T) {
		adapter, err := openaiadapter.New(openaiadapter.Config{APIKey: secret, BaseURL: "https://api.example.test/v1"})
		if err != nil {
			t.Fatal(err)
		}
		request := basePublicRequest(modelinvoker.ProtocolChatCompletions)
		request.State = &modelinvoker.State{
			Kind: modelinvoker.StateServerContinuation, Provider: openaiadapter.ProviderID,
			Protocol: modelinvoker.ProtocolChatCompletions, ID: "previous",
		}
		_, err = adapter.Invoke(context.Background(), request)
		var invocationError *modelinvoker.Error
		if !errors.As(err, &invocationError) || invocationError.Provider != openaiadapter.ProviderID ||
			invocationError.MappingReport.Provider != openaiadapter.ProviderID {
			t.Fatalf("validation identity after redaction = %#v", err)
		}
	})

	t.Run("invoke", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Header.Get("Authorization") != "Bearer "+secret {
				t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"id":"chat","model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
		}))
		t.Cleanup(server.Close)
		adapter, err := openaiadapter.New(openaiadapter.Config{APIKey: secret, BaseURL: server.URL + "/v1"})
		if err != nil {
			t.Fatal(err)
		}
		response, err := adapter.Invoke(context.Background(), basePublicRequest(modelinvoker.ProtocolChatCompletions))
		if err != nil {
			t.Fatal(err)
		}
		if response.Provider != openaiadapter.ProviderID || response.MappingReport.Provider != openaiadapter.ProviderID ||
			response.Protocol != modelinvoker.ProtocolChatCompletions {
			t.Fatalf("identity after redaction = %#v", response)
		}
	})

	t.Run("stream", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Header.Get("Authorization") != "Bearer "+secret {
				t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
			}
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(writer, "data: {\"id\":\"chat\",\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
			_, _ = fmt.Fprint(writer, "data: {\"id\":\"chat\",\"model\":\"test-model\",\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n")
			_, _ = fmt.Fprint(writer, "data: [DONE]\n\n")
		}))
		t.Cleanup(server.Close)
		adapter, err := openaiadapter.New(openaiadapter.Config{APIKey: secret, BaseURL: server.URL + "/v1"})
		if err != nil {
			t.Fatal(err)
		}
		stream, err := adapter.Stream(context.Background(), basePublicRequest(modelinvoker.ProtocolChatCompletions))
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		var terminal *modelinvoker.Response
		for stream.Next() {
			if stream.Event().Response != nil {
				terminal = stream.Event().Response
			}
		}
		if err := stream.Err(); err != nil {
			t.Fatal(err)
		}
		if terminal == nil || terminal.Provider != openaiadapter.ProviderID ||
			terminal.MappingReport.Provider != openaiadapter.ProviderID || terminal.Protocol != modelinvoker.ProtocolChatCompletions {
			t.Fatalf("stream identity after redaction = %#v", terminal)
		}
	})
}

func TestPublicResponsesBindingRestoresIdentityAfterRedaction(t *testing.T) {
	secret := string(openaiadapter.ProviderID)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+secret {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"id":"resp","model":"test-model","status":"completed","output":[{"id":"msg","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(server.Close)
	adapter, err := openaiadapter.New(openaiadapter.Config{APIKey: secret, BaseURL: server.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Invoke(context.Background(), basePublicRequest(modelinvoker.ProtocolResponses))
	if err != nil {
		t.Fatal(err)
	}
	if response.Provider != openaiadapter.ProviderID || response.MappingReport.Provider != openaiadapter.ProviderID ||
		response.State == nil || response.State.Provider != openaiadapter.ProviderID || response.State.Protocol != modelinvoker.ProtocolResponses {
		t.Fatalf("Responses identity after redaction = %#v", response)
	}
}

func TestPublicChatBindingDoesNotRestoreSecretEndpointBytes(t *testing.T) {
	const secret = "sk-endpoint-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"id":"chat","model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(server.Close)
	endpoint := server.URL + "/v1/" + secret
	adapter, err := openaiadapter.New(openaiadapter.Config{APIKey: secret, BaseURL: endpoint})
	if err != nil {
		t.Fatal(err)
	}
	request := basePublicRequest(modelinvoker.ProtocolChatCompletions)
	request.Endpoint = endpoint
	response, err := adapter.Invoke(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Provider != openaiadapter.ProviderID || response.MappingReport.Provider != openaiadapter.ProviderID ||
		!strings.Contains(response.MappingReport.Endpoint, "[REDACTED]") {
		t.Fatalf("public binding identity/endpoint = %#v", response.MappingReport)
	}
	assertOpenAIResponseHasNoSecret(t, response, secret)
}

func TestPublicOpenAIRedirectsNeverSendAuthorizationToSecondHop(t *testing.T) {
	secret := "sk-redirect-secret"
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
			var secondHopCalls atomic.Int64
			var secondHopAuthorization atomic.Value
			secondHopAuthorization.Store("")
			secondHop := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				secondHopCalls.Add(1)
				secondHopAuthorization.Store(request.Header.Get("Authorization"))
			}))
			t.Cleanup(secondHop.Close)

			var target string
			firstAuthorization := make(chan string, 1)
			handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/second-hop" {
					secondHopCalls.Add(1)
					secondHopAuthorization.Store(request.Header.Get("Authorization"))
					return
				}
				firstAuthorization <- request.Header.Get("Authorization")
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
				target = secondHop.URL + "/second-hop"
			}

			adapter, err := openaiadapter.New(openaiadapter.Config{
				APIKey: secret, BaseURL: first.URL + "/v1", HTTPClient: first.Client(),
			})
			if err != nil {
				t.Fatal(err)
			}
			response, err := adapter.Invoke(context.Background(), basePublicRequest(modelinvoker.ProtocolResponses))
			invocationError := assertPublicErrorKind(t, err, modelinvoker.ErrorProvider)
			if invocationError.HTTPStatus != test.status || invocationError.Retryable {
				t.Fatalf("redirect error = %#v", invocationError)
			}
			if authorization := <-firstAuthorization; authorization != "Bearer "+secret {
				t.Fatalf("first-hop Authorization = %q", authorization)
			}
			if secondHopCalls.Load() != 0 || secondHopAuthorization.Load().(string) != "" {
				t.Fatalf("second-hop calls/Authorization = %d/%q", secondHopCalls.Load(), secondHopAuthorization.Load())
			}
			assertOpenAIResponseHasNoSecret(t, response, secret)
			assertOpenAIErrorHasNoSecret(t, err, secret)
		})
	}
}

func TestPublicOpenAIMaliciousErrorAndMalformedResponsesAreRedacted(t *testing.T) {
	secret := "sk-loopback-secret"
	tests := []struct {
		name         string
		protocol     modelinvoker.Protocol
		status       int
		responseBody string
		wantKind     modelinvoker.ErrorKind
	}{
		{
			name: "JSON error", protocol: modelinvoker.ProtocolResponses, status: http.StatusBadRequest,
			responseBody: `{"error":{"message":"sk-loopback-secret","type":"invalid_request_error","code":"sk-loopback-secret"}}`,
			wantKind:     modelinvoker.ErrorInvalidRequest,
		},
		{
			name: "malformed Responses 2xx", protocol: modelinvoker.ProtocolResponses, status: http.StatusOK,
			responseBody: `{"id":"sk-loopback-secret"`, wantKind: modelinvoker.ErrorProvider,
		},
		{
			name: "malformed Chat 2xx", protocol: modelinvoker.ProtocolChatCompletions, status: http.StatusOK,
			responseBody: `{"id":"sk-loopback-secret"`, wantKind: modelinvoker.ErrorProvider,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("X-Request-Id", secret)
				writer.Header().Set("OpenAI-Processing-Ms", secret)
				writer.Header().Set("X-Ratelimit-Limit-Tokens", secret)
				writer.WriteHeader(test.status)
				_, _ = fmt.Fprint(writer, test.responseBody)
			}))
			t.Cleanup(server.Close)

			adapter, err := openaiadapter.New(openaiadapter.Config{APIKey: secret, BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			response, err := adapter.Invoke(context.Background(), basePublicRequest(test.protocol))
			assertPublicErrorKind(t, err, test.wantKind)
			if response.RawResponse.Empty() || !strings.Contains(string(response.RawResponse.Bytes()), "[REDACTED]") {
				t.Fatalf("captured RawResponse = %q", response.RawResponse.Bytes())
			}
			assertOpenAIResponseHasNoSecret(t, response, secret)
			assertOpenAIErrorHasNoSecret(t, err, secret)
		})
	}
}

func TestPublicOpenAIMaliciousStreamEventsAreRedacted(t *testing.T) {
	secret := "sk-stream-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("X-Request-Id", secret)
		writer.Header().Set("OpenAI-Processing-Ms", secret)
		writer.Header().Set("X-Ratelimit-Limit-Tokens", secret)
		events := []string{
			`{"type":"response.created","sequence_number":1,"response":{"id":"sk-stream-secret","model":"sk-stream-secret","status":"in_progress","output":[]}}`,
			`{"type":"response.output_text.delta","sequence_number":2,"item_id":"item","output_index":0,"content_index":0,"delta":"sk-stream-"}`,
			`{"type":"response.output_text.delta","sequence_number":3,"item_id":"item","output_index":0,"content_index":0,"delta":"secret"}`,
			`{"type":"error","sequence_number":4,"code":"sk-stream-secret","message":"sk-stream-secret"}`,
		}
		for _, event := range events {
			_, _ = fmt.Fprintf(writer, "data: %s\n\n", event)
		}
	}))
	t.Cleanup(server.Close)
	adapter, err := openaiadapter.New(openaiadapter.Config{APIKey: secret, BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := adapter.Stream(context.Background(), basePublicRequest(modelinvoker.ProtocolResponses))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var sawError bool
	var text, raw, native string
	for stream.Next() {
		event := stream.Event()
		assertOpenAIEventHasNoSecret(t, event, secret)
		text += event.TextDelta
		raw += string(event.Raw.Bytes())
		if event.Type == modelinvoker.StreamEventError {
			sawError = true
			if event.Response == nil || event.Response.State == nil || event.Response.RawResponse.Empty() || len(event.Response.NativeEvents) == 0 {
				t.Fatalf("partial response audit = %#v", event.Response)
			}
			for _, item := range event.Response.NativeEvents {
				native += string(item.Bytes())
			}
		}
	}
	if !sawError {
		t.Fatal("stream emitted no error event")
	}
	assertOpenAIStringsHaveNoSecret(t, secret, text, raw, native)
	assertOpenAIErrorHasNoSecret(t, stream.Err(), secret)
}

func TestPublicOpenAIResponseBodyLimitIsNonRetryableForInvokeAndStream(t *testing.T) {
	secret := "sk-body-limit-secret"

	t.Run("oversized 2xx", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.CopyN(writer, openAIRepeatingReader('x'), adaptercore.DefaultMaxResponseBodyBytes+1)
		}))
		t.Cleanup(server.Close)
		adapter, err := openaiadapter.New(openaiadapter.Config{APIKey: secret, BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		response, err := adapter.Invoke(context.Background(), basePublicRequest(modelinvoker.ProtocolResponses))
		assertOpenAIResponseBodyLimit(t, err, secret, server.URL)
		if response.RawResponse.Len() > int(adaptercore.DefaultMaxResponseBodyBytes) {
			t.Fatalf("RawResponse length = %d", response.RawResponse.Len())
		}
	})

	t.Run("unbounded SSE", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.WriteHeader(http.StatusOK)
			writer.(http.Flusher).Flush()
			_, _ = io.CopyN(writer, openAIRepeatingReader('s'), adaptercore.DefaultMaxResponseBodyBytes+1)
		}))
		t.Cleanup(server.Close)
		adapter, err := openaiadapter.New(openaiadapter.Config{APIKey: secret, BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		stream, err := adapter.Stream(context.Background(), basePublicRequest(modelinvoker.ProtocolResponses))
		if err != nil {
			assertOpenAIResponseBodyLimit(t, err, secret, server.URL)
			return
		}
		var eventErr error
		for stream.Next() {
			event := stream.Event()
			assertOpenAIEventHasNoSecret(t, event, secret)
			if event.Error != nil {
				eventErr = event.Error
			}
		}
		assertOpenAIResponseBodyLimit(t, eventErr, secret, server.URL)
		assertOpenAIResponseBodyLimit(t, stream.Err(), secret, server.URL)
		if closeErr := stream.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	})

	t.Run("Chat oversized 2xx", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.CopyN(writer, openAIRepeatingReader('x'), adaptercore.DefaultMaxResponseBodyBytes+1)
		}))
		t.Cleanup(server.Close)
		adapter, err := openaiadapter.New(openaiadapter.Config{APIKey: secret, BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		response, err := adapter.Invoke(context.Background(), basePublicRequest(modelinvoker.ProtocolChatCompletions))
		assertOpenAIResponseBodyLimit(t, err, secret, server.URL)
		if response.Provider != openaiadapter.ProviderID || response.Protocol != modelinvoker.ProtocolChatCompletions ||
			response.RawResponse.Len() > int(adaptercore.DefaultMaxResponseBodyBytes) {
			t.Fatalf("Chat body-limit response = %#v", response)
		}
	})

	t.Run("Chat unbounded SSE", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.WriteHeader(http.StatusOK)
			writer.(http.Flusher).Flush()
			_, _ = io.CopyN(writer, openAIRepeatingReader('s'), adaptercore.DefaultMaxResponseBodyBytes+1)
		}))
		t.Cleanup(server.Close)
		adapter, err := openaiadapter.New(openaiadapter.Config{APIKey: secret, BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		stream, err := adapter.Stream(context.Background(), basePublicRequest(modelinvoker.ProtocolChatCompletions))
		if err != nil {
			assertOpenAIResponseBodyLimit(t, err, secret, server.URL)
			return
		}
		var eventErr error
		for stream.Next() {
			if stream.Event().Error != nil {
				eventErr = stream.Event().Error
			}
		}
		assertOpenAIResponseBodyLimit(t, eventErr, secret, server.URL)
		assertOpenAIResponseBodyLimit(t, stream.Err(), secret, server.URL)
		if closeErr := stream.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	})
}

func TestPublicOpenAIAdapterFormattingDoesNotExposeSecrets(t *testing.T) {
	secret := `sk-format/a b`
	adapter, err := openaiadapter.New(openaiadapter.Config{
		APIKey:  secret,
		BaseURL: "https://example.test/v1/" + url.PathEscape(secret),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{adapter, *adapter} {
		for _, format := range []string{"%v", "%+v", "%#v"} {
			formatted := fmt.Sprintf(format, value)
			if strings.Contains(formatted, secret) || strings.Contains(formatted, url.PathEscape(secret)) || strings.Contains(formatted, url.QueryEscape(secret)) {
				t.Fatalf("adapter format %s leaked secret: %s", format, formatted)
			}
		}
	}
}

func assertOpenAIEventHasNoSecret(t *testing.T, event modelinvoker.StreamEvent, secret string) {
	t.Helper()
	parts := []string{
		string(event.Type), event.ResponseID, event.TextDelta, event.ReasoningDelta, event.ArgumentsDelta, string(event.Raw.Bytes()),
	}
	if event.FunctionCall != nil {
		parts = append(parts, event.FunctionCall.ID, event.FunctionCall.Name, string(event.FunctionCall.Arguments))
	}
	if event.Response != nil {
		assertOpenAIResponseHasNoSecret(t, *event.Response, secret)
	}
	if event.Error != nil {
		assertOpenAIErrorHasNoSecret(t, event.Error, secret)
	}
	assertOpenAIStringsHaveNoSecret(t, secret, parts...)
}

func assertOpenAIResponseHasNoSecret(t *testing.T, response modelinvoker.Response, secret string) {
	t.Helper()
	parts := []string{
		response.ID, string(response.Provider), string(response.Protocol), response.Model, response.StopSequence, response.RequestID,
		string(response.RawRequest.Bytes()), string(response.RawResponse.Bytes()), response.MappingReport.Endpoint,
	}
	for _, item := range response.Output {
		parts = append(parts, item.Text, item.ReasoningSummary)
		if item.FunctionCall != nil {
			parts = append(parts, item.FunctionCall.ID, item.FunctionCall.Name, string(item.FunctionCall.Arguments))
		}
	}
	for key, value := range response.Metadata {
		parts = append(parts, key, value)
	}
	for key, value := range response.ProviderMetadata {
		parts = append(parts, key, value)
	}
	if response.State != nil {
		parts = append(parts, response.State.ID, string(response.State.Payload.Bytes()))
	}
	for _, decision := range response.MappingReport.Decisions {
		parts = append(parts, decision.Detail)
	}
	for _, raw := range response.NativeEvents {
		parts = append(parts, string(raw.Bytes()))
	}
	assertOpenAIStringsHaveNoSecret(t, secret, parts...)
}

func assertOpenAIErrorHasNoSecret(t *testing.T, err error, secret string) {
	t.Helper()
	if err == nil {
		return
	}
	assertOpenAIStringsHaveNoSecret(t, secret, err.Error(), fmt.Sprintf("%v", err), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err))
	var invocationError *modelinvoker.Error
	if errors.As(err, &invocationError) {
		assertOpenAIStringsHaveNoSecret(t, secret,
			string(invocationError.Kind), string(invocationError.Provider), invocationError.Operation, invocationError.Code,
			invocationError.Message, invocationError.RequestID, invocationError.MappingReport.Endpoint,
		)
		if invocationError.Err != nil {
			assertOpenAIStringsHaveNoSecret(t, secret, invocationError.Err.Error(), fmt.Sprintf("%#v", invocationError.Err))
		}
	}
	var sdkError *openaisdk.Error
	if errors.As(err, &sdkError) {
		t.Fatalf("SDK error crossed public boundary: %T", sdkError)
	}
}

func assertOpenAIStringsHaveNoSecret(t *testing.T, secret string, values ...string) {
	t.Helper()
	encoded, _ := json.Marshal(secret)
	forms := []string{secret, string(encoded[1 : len(encoded)-1]), url.QueryEscape(secret), url.PathEscape(secret)}
	for _, value := range values {
		for _, form := range forms {
			if form != "" && strings.Contains(value, form) {
				t.Fatalf("public value contains secret form %q: %s", form, value)
			}
		}
	}
}

type openAIRepeatingReader byte

func (r openAIRepeatingReader) Read(destination []byte) (int, error) {
	for index := range destination {
		destination[index] = byte(r)
	}
	return len(destination), nil
}

func assertOpenAIResponseBodyLimit(t *testing.T, err error, secret, endpoint string) {
	t.Helper()
	invocationError := assertPublicErrorKind(t, err, modelinvoker.ErrorProvider)
	if invocationError.Code != adaptercore.ResponseBodyLimitErrorCode || invocationError.Retryable || invocationError.Provider != openaiadapter.ProviderID {
		t.Fatalf("response-body limit error = %#v", invocationError)
	}
	if invocationError.Err != nil {
		t.Fatalf("response-body limit retained cause: %#v", err)
	}
	if got := invocationError.MappingReport.Endpoint; got != "" && !strings.HasPrefix(got, endpoint) && !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("response-body limit endpoint = %q, want audited endpoint", got)
	}
	assertOpenAIErrorHasNoSecret(t, err, secret)
}
