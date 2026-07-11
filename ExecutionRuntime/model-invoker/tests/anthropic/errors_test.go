package anthropic_test

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
	provider "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/anthropic"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

func TestAPIErrorClassificationNoSDKLeakAndNoSDKRetry(t *testing.T) {
	fixture := mustRead(t, "testdata/error-overloaded.json")
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("content-type", "application/json")
		w.Header().Set("request-id", "req_error_header_01")
		w.Header().Set("retry-after", "1.5")
		w.WriteHeader(529)
		_, _ = w.Write(fixture)
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "secret-test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	response, err := adapter.Invoke(context.Background(), baseRequest())
	if err == nil {
		t.Fatal("Invoke() error = nil")
	}
	if calls.Load() != 1 {
		t.Fatalf("HTTP requests = %d, want exactly 1", calls.Load())
	}
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) {
		t.Fatalf("error type = %T", err)
	}
	if invocationError.Kind != modelinvoker.ErrorProviderUnavailable || !invocationError.Retryable ||
		invocationError.HTTPStatus != 529 || invocationError.RequestID != "req_error_header_01" ||
		invocationError.RetryAfter != 1500*time.Millisecond {
		t.Fatalf("normalized error = %#v", invocationError)
	}
	var sdkError *anthropicsdk.Error
	if errors.As(err, &sdkError) {
		t.Fatal("public error unwrap chain exposes *anthropic.Error")
	}
	if response.RawResponse.Empty() || response.RequestID != "req_error_header_01" {
		t.Fatalf("failed response audit = %#v", response)
	}
}

func TestBillingErrorClassification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"billing_error","message":"payment required"},"request_id":"req_bill"}`))
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "test", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Invoke(context.Background(), baseRequest())
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorBilling {
		t.Fatalf("kind = %q, error = %v", modelinvoker.ErrorKindOf(err), err)
	}
}

func TestCancelledContextDoesNotExposeSDKError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	adapter, err := provider.New(provider.Config{APIKey: "secret-test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = adapter.Invoke(ctx, baseRequest())
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorCancelled {
		t.Fatalf("kind = %q, error = %v", modelinvoker.ErrorKindOf(err), err)
	}
	var sdkError *anthropicsdk.Error
	if errors.As(err, &sdkError) {
		t.Fatal("cancelled error unwrap chain exposes *anthropic.Error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(context.Canceled) = false: %v", err)
	}
}

func TestContextCancellationWinsWithoutRetainingSimultaneousSDKError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		cancel()
		response := &http.Response{
			StatusCode: 529, Status: "529 overloaded", Header: make(http.Header),
			Request: request, Body: http.NoBody,
		}
		return nil, &anthropicsdk.Error{
			StatusCode: 529, Request: request, Response: response, RequestID: "req_sdk_should_not_escape",
		}
	})}
	adapter, err := provider.New(provider.Config{APIKey: "secret-test-key", BaseURL: "https://api.anthropic.test", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Invoke(ctx, baseRequest())
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorCancelled || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	var sdkError *anthropicsdk.Error
	if errors.As(err, &sdkError) {
		t.Fatal("simultaneous context error retained *anthropic.Error")
	}
}

func TestHTTPErrorMatrixRetryAfterRequestIDAndBoundary(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		errorType  string
		wantKind   modelinvoker.ErrorKind
		retryable  bool
		retryAfter time.Duration
		headerID   string
		bodyID     string
	}{
		{name: "400 invalid request", status: http.StatusBadRequest, errorType: "invalid_request_error", wantKind: modelinvoker.ErrorInvalidRequest, headerID: "req_400", bodyID: "body_400"},
		{name: "401 authentication", status: http.StatusUnauthorized, errorType: "authentication_error", wantKind: modelinvoker.ErrorAuthentication, headerID: "req_401", bodyID: "body_401"},
		{name: "403 permission", status: http.StatusForbidden, errorType: "permission_error", wantKind: modelinvoker.ErrorPermission, headerID: "req_403", bodyID: "body_403"},
		{name: "404 model unavailable", status: http.StatusNotFound, errorType: "not_found_error", wantKind: modelinvoker.ErrorPermission, headerID: "req_404", bodyID: "body_404"},
		{name: "408 timeout", status: http.StatusRequestTimeout, errorType: "timeout_error", wantKind: modelinvoker.ErrorTimeout, retryable: true, headerID: "req_408", bodyID: "body_408"},
		{name: "409 conflict", status: http.StatusConflict, errorType: "conflict_error", wantKind: modelinvoker.ErrorProviderUnavailable, retryable: true, headerID: "req_409", bodyID: "body_409"},
		{name: "429 rate limit milliseconds", status: http.StatusTooManyRequests, errorType: "rate_limit_error", wantKind: modelinvoker.ErrorRateLimit, retryable: true, retryAfter: 250 * time.Millisecond, headerID: "req_429", bodyID: "body_429"},
		{name: "500 api error", status: http.StatusInternalServerError, errorType: "api_error", wantKind: modelinvoker.ErrorProviderUnavailable, retryable: true, headerID: "req_500", bodyID: "body_500"},
		{name: "529 overloaded", status: 529, errorType: "overloaded_error", wantKind: modelinvoker.ErrorProviderUnavailable, retryable: true, headerID: "req_529", bodyID: "body_529"},
		{name: "unknown error with body request id", status: http.StatusTeapot, errorType: "future_error", wantKind: modelinvoker.ErrorProvider, bodyID: "body_unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const apiKey = "anthropic-error-secret-key"
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if got := r.Header.Get("x-api-key"); got != apiKey {
					t.Errorf("x-api-key = %q", got)
				}
				w.Header().Set("content-type", "application/json")
				if test.headerID != "" {
					w.Header().Set("request-id", test.headerID)
				}
				if test.retryAfter > 0 {
					w.Header().Set("retry-after-ms", "250")
				}
				w.WriteHeader(test.status)
				_, _ = fmt.Fprintf(w, `{"type":"error","error":{"type":%q,"message":"matrix failure"},"request_id":%q}`, test.errorType, test.bodyID)
			}))
			defer server.Close()

			adapter, err := provider.New(provider.Config{APIKey: apiKey, BaseURL: server.URL, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			response, err := adapter.Invoke(context.Background(), baseRequest())
			if err == nil {
				t.Fatal("Invoke() error = nil")
			}
			var got *modelinvoker.Error
			if !errors.As(err, &got) || got == nil {
				t.Fatalf("error type = %T", err)
			}
			wantRequestID := test.headerID
			if wantRequestID == "" {
				wantRequestID = test.bodyID
			}
			if got.Kind != test.wantKind || got.Code != test.errorType || got.HTTPStatus != test.status ||
				got.RequestID != wantRequestID || got.Retryable != test.retryable || got.RetryAfter != test.retryAfter {
				t.Fatalf("normalized error = %#v", got)
			}
			if calls.Load() != 1 {
				t.Fatalf("HTTP attempts = %d, want 1", calls.Load())
			}
			var sdkError *anthropicsdk.Error
			if errors.As(err, &sdkError) {
				t.Fatalf("SDK error crossed public boundary: %T", sdkError)
			}
			if strings.Contains(fmt.Sprintf("%+v", err), apiKey) || strings.Contains(string(response.RawRequest.Bytes()), apiKey) ||
				strings.Contains(string(response.RawResponse.Bytes()), apiKey) {
				t.Fatal("error audit data leaked API key")
			}
			if response.RequestID != wantRequestID || response.RawResponse.Empty() {
				t.Fatalf("failed response audit = %#v", response)
			}
		})
	}
}

func TestMalformed2xxPayloadIsNotRetriedAndDoesNotLeakSDKOrKey(t *testing.T) {
	const apiKey = "anthropic-malformed-secret-key"
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if got := r.Header.Get("x-api-key"); got != apiKey {
			t.Errorf("x-api-key = %q", got)
		}
		w.Header().Set("content-type", "application/json")
		w.Header().Set("request-id", "req_malformed_2xx")
		_, _ = w.Write([]byte(`{"id":`))
	}))
	defer server.Close()

	adapter, err := provider.New(provider.Config{APIKey: apiKey, BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := modelinvoker.NewRegistry(adapter)
	if err != nil {
		t.Fatal(err)
	}
	invoker, err := modelinvoker.NewInvoker(registry,
		modelinvoker.WithRetryPolicy(modelinvoker.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Millisecond, Multiplier: 2}),
		modelinvoker.WithSleeper(func(context.Context, time.Duration) error { return nil }),
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := invoker.Invoke(context.Background(), baseRequest())
	if err == nil {
		t.Fatal("Invoke() error = nil")
	}
	var got *modelinvoker.Error
	if !errors.As(err, &got) || got == nil {
		t.Fatalf("error type = %T", err)
	}
	if got.Kind != modelinvoker.ErrorProvider || got.Retryable || got.RequestID != "req_malformed_2xx" || calls.Load() != 1 {
		t.Fatalf("malformed response error/calls = %#v / %d", got, calls.Load())
	}
	var sdkError *anthropicsdk.Error
	if errors.As(err, &sdkError) {
		t.Fatalf("SDK error crossed public boundary: %T", sdkError)
	}
	if response.RawResponse.Empty() || strings.Contains(fmt.Sprintf("%+v", err), apiKey) ||
		strings.Contains(string(response.RawRequest.Bytes()), apiKey) || strings.Contains(string(response.RawResponse.Bytes()), apiKey) {
		t.Fatal("malformed response path leaked API key")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
