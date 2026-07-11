package gemini_test

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
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/gemini"
	"google.golang.org/genai"
)

func TestRedirectsNeverSendGeminiAPIKeyToSecondHop(t *testing.T) {
	secret := "gemini-redirect-secret"
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
				secondKey.Store(request.Header.Get("x-goog-api-key"))
			}))
			t.Cleanup(second.Close)

			var target string
			firstKey := make(chan string, 1)
			handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/second-hop" {
					secondCalls.Add(1)
					secondKey.Store(request.Header.Get("x-goog-api-key"))
					return
				}
				firstKey <- request.Header.Get("x-goog-api-key")
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
			invocationError := geminiErrorKind(t, err, modelinvoker.ErrorProvider)
			if invocationError.HTTPStatus != test.status || invocationError.Retryable {
				t.Fatalf("redirect error = %#v", invocationError)
			}
			if got := <-firstKey; got != secret {
				t.Fatalf("first-hop x-goog-api-key = %q", got)
			}
			if secondCalls.Load() != 0 || secondKey.Load().(string) != "" {
				t.Fatalf("second-hop calls/key = %d/%q", secondCalls.Load(), secondKey.Load())
			}
			assertGeminiResponseNoSecret(t, response, secret)
			assertGeminiErrorNoSecret(t, err, secret)
		})
	}
}

func TestMaliciousGeminiErrorAndMalformedResponsesAreRedacted(t *testing.T) {
	secret := "gemini-loopback-secret"
	tests := []struct {
		name     string
		status   int
		body     string
		wantKind modelinvoker.ErrorKind
	}{
		{
			name: "JSON error", status: http.StatusBadRequest,
			body:     `{"error":{"code":400,"message":"gemini-loopback-secret","status":"gemini-loopback-secret","details":[{"@type":"type.googleapis.com/google.rpc.RequestInfo","requestId":"gemini-loopback-secret"}]}}`,
			wantKind: modelinvoker.ErrorInvalidRequest,
		},
		{name: "malformed 2xx", status: http.StatusOK, body: `{"responseId":"gemini-loopback-secret"`, wantKind: modelinvoker.ErrorProvider},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("content-type", "application/json")
				writer.Header().Set("x-goog-request-id", secret)
				writer.Header().Set("x-goog-quota-project", secret)
				writer.WriteHeader(test.status)
				_, _ = fmt.Fprint(writer, test.body)
			}))
			t.Cleanup(server.Close)
			adapter, err := provider.New(provider.Config{APIKey: secret, BaseURL: server.URL, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			response, err := adapter.Invoke(context.Background(), baseRequest())
			geminiErrorKind(t, err, test.wantKind)
			if response.RawResponse.Empty() || !strings.Contains(string(response.RawResponse.Bytes()), "[REDACTED]") {
				t.Fatalf("captured RawResponse = %q", response.RawResponse.Bytes())
			}
			assertGeminiResponseNoSecret(t, response, secret)
			assertGeminiErrorNoSecret(t, err, secret)
		})
	}
}

func TestMaliciousGeminiSuccessfulResponseIsRedacted(t *testing.T) {
	secret := "gemini-success-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("content-type", "application/json")
		writer.Header().Set("x-goog-request-id", secret)
		writer.Header().Set("x-goog-quota-project", secret)
		_, _ = fmt.Fprint(writer, `{"candidates":[{"content":{"role":"model","parts":[{"text":"gemini-success-secret"}]},"finishReason":"STOP","index":0}],"modelVersion":"gemini-success-secret","responseId":"gemini-success-secret","usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`)
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
	assertGeminiResponseNoSecret(t, response, secret)
}

func TestMaliciousGeminiStreamEventsStateAndErrorsAreRedacted(t *testing.T) {
	secret := "Z2VtaW5pLXN0cmVhbS1zZWNyZXQ="
	split := len(secret) / 2
	sse := strings.Join([]string{
		fmt.Sprintf(`data: {"candidates":[{"content":{"role":"model","parts":[{"text":%q,"thought":true,"thoughtSignature":"Z2VtaW5pLXN0cmVhbS1zZWNyZXQ="}]},"index":0}],"modelVersion":"Z2VtaW5pLXN0cmVhbS1zZWNyZXQ=","responseId":"Z2VtaW5pLXN0cmVhbS1zZWNyZXQ="}`, secret[:split]) + "\n\n",
		fmt.Sprintf(`data: {"candidates":[{"content":{"role":"model","parts":[{"text":%q,"thought":true}]},"index":0}],"modelVersion":"Z2VtaW5pLXN0cmVhbS1zZWNyZXQ=","responseId":"Z2VtaW5pLXN0cmVhbS1zZWNyZXQ="}`, secret[split:]) + "\n\n",
		`data: {"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"SAFETY","finishMessage":"Z2VtaW5pLXN0cmVhbS1zZWNyZXQ=","index":0}],"modelVersion":"Z2VtaW5pLXN0cmVhbS1zZWNyZXQ=","responseId":"Z2VtaW5pLXN0cmVhbS1zZWNyZXQ="}` + "\n\n",
	}, "")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("content-type", "text/event-stream")
		writer.Header().Set("x-goog-request-id", secret)
		writer.Header().Set("x-goog-quota-project", secret)
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
	var semantic, raw, native string
	for stream.Next() {
		event := stream.Event()
		assertGeminiEventNoSecret(t, event, secret)
		semantic += event.TextDelta + event.ReasoningDelta
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
	assertGeminiStringsNoSecret(t, secret, semantic, raw, native)
	assertGeminiErrorNoSecret(t, stream.Err(), secret)
}

func TestGeminiResponseBodyLimitIsNonRetryableForInvokeAndStream(t *testing.T) {
	secret := "gemini-body-limit-secret"

	t.Run("oversized 2xx", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("content-type", "application/json")
			_, _ = io.CopyN(writer, geminiRepeatingReader('x'), adaptercore.DefaultMaxResponseBodyBytes+1)
		}))
		t.Cleanup(server.Close)
		adapter, err := provider.New(provider.Config{APIKey: secret, BaseURL: server.URL, HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		response, err := adapter.Invoke(context.Background(), baseRequest())
		assertGeminiResponseBodyLimit(t, err, secret, server.URL)
		if response.RawResponse.Len() > int(adaptercore.DefaultMaxResponseBodyBytes) {
			t.Fatalf("RawResponse length = %d", response.RawResponse.Len())
		}
	})

	t.Run("unbounded SSE", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("content-type", "text/event-stream")
			writer.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(writer, `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"index":0}]}`+"\n\n")
			writer.(http.Flusher).Flush()
			_, _ = io.CopyN(writer, geminiRepeatingReader('s'), adaptercore.DefaultMaxResponseBodyBytes+1)
		}))
		t.Cleanup(server.Close)
		adapter, err := provider.New(provider.Config{APIKey: secret, BaseURL: server.URL, HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		stream, err := adapter.Stream(context.Background(), baseRequest())
		if err != nil {
			assertGeminiResponseBodyLimit(t, err, secret, server.URL)
			return
		}
		var eventErr error
		for stream.Next() {
			event := stream.Event()
			assertGeminiEventNoSecret(t, event, secret)
			if event.Error != nil {
				eventErr = event.Error
			}
		}
		assertGeminiResponseBodyLimit(t, eventErr, secret, server.URL)
		assertGeminiResponseBodyLimit(t, stream.Err(), secret, server.URL)
		if closeErr := stream.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	})
}

func geminiErrorKind(t *testing.T, err error, want modelinvoker.ErrorKind) *modelinvoker.Error {
	t.Helper()
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError == nil || invocationError.Kind != want {
		t.Fatalf("error = %#v, want kind %q", err, want)
	}
	return invocationError
}

func assertGeminiEventNoSecret(t *testing.T, event modelinvoker.StreamEvent, secret string) {
	t.Helper()
	assertGeminiStringsNoSecret(t, secret,
		string(event.Type), event.ResponseID, event.TextDelta, event.ReasoningDelta, event.ArgumentsDelta, string(event.Raw.Bytes()),
	)
	if event.FunctionCall != nil {
		assertGeminiStringsNoSecret(t, secret, event.FunctionCall.ID, event.FunctionCall.Name, string(event.FunctionCall.Arguments))
	}
	if event.Response != nil {
		assertGeminiResponseNoSecret(t, *event.Response, secret)
	}
	if event.Error != nil {
		assertGeminiErrorNoSecret(t, event.Error, secret)
	}
}

func assertGeminiResponseNoSecret(t *testing.T, response modelinvoker.Response, secret string) {
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
	assertGeminiStringsNoSecret(t, secret, values...)
}

func assertGeminiErrorNoSecret(t *testing.T, err error, secret string) {
	t.Helper()
	if err == nil {
		return
	}
	assertGeminiStringsNoSecret(t, secret, err.Error(), fmt.Sprintf("%v", err), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err))
	var invocationError *modelinvoker.Error
	if errors.As(err, &invocationError) {
		assertGeminiStringsNoSecret(t, secret,
			string(invocationError.Kind), string(invocationError.Provider), invocationError.Operation,
			invocationError.Code, invocationError.Message, invocationError.RequestID, invocationError.MappingReport.Endpoint,
		)
		if invocationError.Err != nil {
			assertGeminiStringsNoSecret(t, secret, invocationError.Err.Error(), fmt.Sprintf("%#v", invocationError.Err))
		}
	}
	var sdkError genai.APIError
	if errors.As(err, &sdkError) {
		t.Fatal("Gemini SDK error crossed public boundary")
	}
}

func assertGeminiStringsNoSecret(t *testing.T, secret string, values ...string) {
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

type geminiRepeatingReader byte

func (r geminiRepeatingReader) Read(destination []byte) (int, error) {
	for index := range destination {
		destination[index] = byte(r)
	}
	return len(destination), nil
}

func assertGeminiResponseBodyLimit(t *testing.T, err error, secret, endpoint string) {
	t.Helper()
	invocationError := geminiErrorKind(t, err, modelinvoker.ErrorProvider)
	if invocationError.Code != adaptercore.ResponseBodyLimitErrorCode || invocationError.Retryable || invocationError.Provider != provider.ProviderID {
		t.Fatalf("response-body limit error = %#v", invocationError)
	}
	if invocationError.Err != nil {
		t.Fatalf("response-body limit retained cause: %#v", err)
	}
	if got := invocationError.MappingReport.Endpoint; got != "" && !strings.HasPrefix(got, endpoint) && !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("response-body limit endpoint = %q, want audited endpoint", got)
	}
	assertGeminiErrorNoSecret(t, err, secret)
}
