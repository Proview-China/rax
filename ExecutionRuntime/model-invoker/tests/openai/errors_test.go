package openai_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	openaiadapter "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/openai"
	openaisdk "github.com/openai/openai-go/v3"
)

func TestPublicHTTPErrorClassificationAndSingleSDKAttempt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		code       string
		errorType  string
		wantKind   modelinvoker.ErrorKind
		retryable  bool
		retryAfter time.Duration
	}{
		{name: "400", status: http.StatusBadRequest, code: "bad_request", errorType: "api_error", wantKind: modelinvoker.ErrorInvalidRequest},
		{name: "401", status: http.StatusUnauthorized, code: "invalid_api_key", errorType: "invalid_request_error", wantKind: modelinvoker.ErrorAuthentication},
		{name: "403", status: http.StatusForbidden, code: "forbidden", errorType: "api_error", wantKind: modelinvoker.ErrorPermission},
		{name: "404", status: http.StatusNotFound, code: "model_missing", errorType: "api_error", wantKind: modelinvoker.ErrorPermission},
		{name: "408", status: http.StatusRequestTimeout, code: "request_timeout", errorType: "server_error", wantKind: modelinvoker.ErrorTimeout, retryable: true},
		{name: "409", status: http.StatusConflict, code: "conflict", errorType: "api_error", wantKind: modelinvoker.ErrorProviderUnavailable, retryable: true},
		{name: "429", status: http.StatusTooManyRequests, code: "quota_exceeded", errorType: "api_error", wantKind: modelinvoker.ErrorRateLimit, retryable: true, retryAfter: 250 * time.Millisecond},
		{name: "500", status: http.StatusInternalServerError, code: "upstream_failure", errorType: "api_error", wantKind: modelinvoker.ErrorProviderUnavailable, retryable: true},
		{name: "503", status: http.StatusServiceUnavailable, code: "unavailable", errorType: "api_error", wantKind: modelinvoker.ErrorProviderUnavailable, retryable: true},
		{name: "policy marker", status: http.StatusBadRequest, code: "content_policy_violation", errorType: "api_error", wantKind: modelinvoker.ErrorPolicyRejected},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if request.URL.Path != "/v1/responses" {
					t.Errorf("path = %q", request.URL.Path)
				}
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("X-Request-Id", "req_error")
				if test.retryAfter > 0 {
					writer.Header().Set("Retry-After", "0.25")
				}
				writer.WriteHeader(test.status)
				_, _ = fmt.Fprintf(writer, `{"error":{"message":"provider message","type":%q,"code":%q,"param":"input"}}`, test.errorType, test.code)
			}))
			t.Cleanup(server.Close)

			adapter := newPublicAdapter(t, server.URL)
			response, err := adapter.Invoke(context.Background(), basePublicRequest(modelinvoker.ProtocolResponses))
			got := assertPublicErrorKind(t, err, test.wantKind)
			var sdkError *openaisdk.Error
			if errors.As(err, &sdkError) {
				t.Fatalf("provider SDK error crossed the public boundary: %T", sdkError)
			}
			if strings.Contains(fmt.Sprintf("%+v", err), "test-only-key") {
				t.Fatal("public error leaked API key")
			}
			if got.HTTPStatus != test.status || got.Code != test.code || got.Message != "provider message" {
				t.Fatalf("HTTP error fields = %#v", got)
			}
			if got.RequestID != "req_error" || got.Retryable != test.retryable || got.RetryAfter != test.retryAfter {
				t.Fatalf("request/retry fields = requestID:%q retryable:%v retryAfter:%v", got.RequestID, got.Retryable, got.RetryAfter)
			}
			if response.Provider != openaiadapter.ProviderID || response.Protocol != modelinvoker.ProtocolResponses ||
				response.Model != "test-model" || response.Status != modelinvoker.ResponseStatusFailed || response.RequestID != "req_error" {
				t.Fatalf("failure response identity = %#v", response)
			}
			if response.RawRequest.Empty() || response.RawResponse.Empty() || response.MappingReport.Provider != openaiadapter.ProviderID ||
				response.MappingReport.Protocol != modelinvoker.ProtocolResponses {
				t.Fatalf("failure response audit = %#v", response)
			}
			if strings.Contains(string(response.RawRequest.Bytes()), "test-only-key") || strings.Contains(string(response.RawResponse.Bytes()), "test-only-key") {
				t.Fatal("failure response audit leaked API key")
			}
			if calls.Load() != 1 {
				t.Fatalf("SDK HTTP attempts = %d, want exactly 1", calls.Load())
			}
		})
	}
}

func TestPublicChatHTTPErrorClassificationAndSingleSDKAttempt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		status    int
		code      string
		errorType string
		wantKind  modelinvoker.ErrorKind
		retryable bool
	}{
		{name: "400", status: http.StatusBadRequest, code: "bad_request", errorType: "api_error", wantKind: modelinvoker.ErrorInvalidRequest},
		{name: "401", status: http.StatusUnauthorized, code: "invalid_api_key", errorType: "invalid_request_error", wantKind: modelinvoker.ErrorAuthentication},
		{name: "403", status: http.StatusForbidden, code: "forbidden", errorType: "api_error", wantKind: modelinvoker.ErrorPermission},
		{name: "404", status: http.StatusNotFound, code: "model_missing", errorType: "api_error", wantKind: modelinvoker.ErrorPermission},
		{name: "408", status: http.StatusRequestTimeout, code: "request_timeout", errorType: "server_error", wantKind: modelinvoker.ErrorTimeout, retryable: true},
		{name: "409", status: http.StatusConflict, code: "conflict", errorType: "api_error", wantKind: modelinvoker.ErrorProviderUnavailable, retryable: true},
		{name: "429", status: http.StatusTooManyRequests, code: "quota_exceeded", errorType: "api_error", wantKind: modelinvoker.ErrorRateLimit, retryable: true},
		{name: "500", status: http.StatusInternalServerError, code: "upstream_failure", errorType: "api_error", wantKind: modelinvoker.ErrorProviderUnavailable, retryable: true},
		{name: "policy", status: http.StatusBadRequest, code: "content_policy_violation", errorType: "api_error", wantKind: modelinvoker.ErrorPolicyRejected},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if request.URL.Path != "/v1/chat/completions" {
					t.Errorf("path = %q", request.URL.Path)
				}
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("X-Request-Id", "req_chat_error")
				writer.WriteHeader(test.status)
				_, _ = fmt.Fprintf(writer, `{"error":{"message":"provider message","type":%q,"code":%q}}`, test.errorType, test.code)
			}))
			t.Cleanup(server.Close)

			response, err := newPublicAdapter(t, server.URL).Invoke(context.Background(), basePublicRequest(modelinvoker.ProtocolChatCompletions))
			got := assertPublicErrorKind(t, err, test.wantKind)
			var sdkError *openaisdk.Error
			if errors.As(err, &sdkError) {
				t.Fatalf("provider SDK error crossed public boundary: %T", sdkError)
			}
			if got.HTTPStatus != test.status || got.Code != test.code || got.Message != "provider message" ||
				got.RequestID != "req_chat_error" || got.Retryable != test.retryable || got.Err != nil {
				t.Fatalf("Chat HTTP error = %#v", got)
			}
			if response.Provider != openaiadapter.ProviderID || response.Protocol != modelinvoker.ProtocolChatCompletions ||
				response.Status != modelinvoker.ResponseStatusFailed || response.RawRequest.Empty() || response.RawResponse.Empty() {
				t.Fatalf("Chat failure response = %#v", response)
			}
			if calls.Load() != 1 {
				t.Fatalf("SDK HTTP attempts = %d, want 1", calls.Load())
			}
		})
	}
}

func TestPublicHTTPErrorRetainsProviderMappingDecisions(t *testing.T) {
	t.Parallel()

	for _, protocol := range []modelinvoker.Protocol{modelinvoker.ProtocolResponses, modelinvoker.ProtocolChatCompletions} {
		protocol := protocol
		t.Run(string(protocol), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("X-Request-Id", "req_audit")
				writer.WriteHeader(http.StatusInternalServerError)
				_, _ = fmt.Fprint(writer, `{"error":{"message":"provider failed","type":"server_error","code":"upstream_failure"}}`)
			}))
			t.Cleanup(server.Close)

			request := basePublicRequest(protocol)
			request.Input = []modelinvoker.InputItem{modelinvoker.FunctionResultInput("call_1", "tool failed", true)}
			request.AllowDegradation = true
			response, err := newPublicAdapter(t, server.URL).Invoke(context.Background(), request)
			assertPublicErrorKind(t, err, modelinvoker.ErrorProviderUnavailable)

			if response.RawRequest.Empty() || response.RawResponse.Empty() || response.RequestID != "req_audit" || response.Status != modelinvoker.ResponseStatusFailed {
				t.Fatalf("failure response audit = %#v", response)
			}
			found := false
			for _, decision := range response.MappingReport.Decisions {
				if decision.Capability == modelinvoker.CapabilityFunctionErrorResult && decision.Action == modelinvoker.MappingDegraded {
					found = true
				}
			}
			if !found {
				t.Fatalf("function error degradation decision missing from failure response: %#v", response.MappingReport)
			}
		})
	}
}

func TestPublicMalformedSuccessPayloadIsNotRetried(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Request-Id", "req_malformed")
		_, _ = fmt.Fprint(writer, `{"id":`)
	}))
	t.Cleanup(server.Close)
	adapter := newPublicAdapter(t, server.URL)
	registry, err := modelinvoker.NewRegistry(adapter)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	invoker, err := modelinvoker.NewInvoker(registry,
		modelinvoker.WithRetryPolicy(modelinvoker.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Millisecond, Multiplier: 2}),
		modelinvoker.WithSleeper(func(context.Context, time.Duration) error { return nil }),
	)
	if err != nil {
		t.Fatalf("NewInvoker() error = %v", err)
	}
	_, err = invoker.Invoke(context.Background(), basePublicRequest(modelinvoker.ProtocolResponses))
	got := assertPublicErrorKind(t, err, modelinvoker.ErrorProvider)
	if got.Retryable || got.RequestID != "req_malformed" || calls.Load() != 1 {
		t.Fatalf("malformed response error/calls = %#v / %d", got, calls.Load())
	}
	var sdkError *openaisdk.Error
	if errors.As(err, &sdkError) {
		t.Fatalf("provider SDK error crossed the public boundary: %T", sdkError)
	}
}

func TestPublicContextAndTransportErrors(t *testing.T) {
	t.Parallel()

	transportFailure := errors.New("dial failed")
	transportAdapter, err := openaiadapter.New(openaiadapter.Config{
		APIKey: "test-key", BaseURL: "https://api.example.test/v1",
		HTTPClient: &http.Client{Transport: publicRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, transportFailure
		})},
	})
	if err != nil {
		t.Fatalf("New(transport adapter) error = %v", err)
	}
	_, err = transportAdapter.Invoke(context.Background(), basePublicRequest(modelinvoker.ProtocolResponses))
	got := assertPublicErrorKind(t, err, modelinvoker.ErrorProviderUnavailable)
	if !got.Retryable || got.Err != nil || errors.Is(got, transportFailure) {
		t.Fatalf("transport error = %#v", got)
	}

	contextAdapter, err := openaiadapter.New(openaiadapter.Config{
		APIKey: "test-key", BaseURL: "https://api.example.test/v1",
		HTTPClient: &http.Client{Transport: publicRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		})},
	})
	if err != nil {
		t.Fatalf("New(context adapter) error = %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = contextAdapter.Invoke(cancelled, basePublicRequest(modelinvoker.ProtocolResponses))
	assertPublicErrorKind(t, err, modelinvoker.ErrorCancelled)

	deadline, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	_, err = contextAdapter.Invoke(deadline, basePublicRequest(modelinvoker.ProtocolResponses))
	assertPublicErrorKind(t, err, modelinvoker.ErrorTimeout)
}

type publicRoundTripFunc func(*http.Request) (*http.Response, error)

func (f publicRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
