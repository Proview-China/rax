package gemini_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"google.golang.org/genai"
)

func TestAPIErrorMatrixHeaderFallbackNoSDKLeakAndNoRetry(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		status     string
		wantKind   modelinvoker.ErrorKind
		wantRetry  bool
	}{
		{name: "bad request", statusCode: 400, status: "INVALID_ARGUMENT", wantKind: modelinvoker.ErrorInvalidRequest},
		{name: "authentication", statusCode: 401, status: "UNAUTHENTICATED", wantKind: modelinvoker.ErrorAuthentication},
		{name: "permission", statusCode: 403, status: "PERMISSION_DENIED", wantKind: modelinvoker.ErrorPermission},
		{name: "not found", statusCode: 404, status: "NOT_FOUND", wantKind: modelinvoker.ErrorPermission},
		{name: "billing", statusCode: 402, status: "BILLING_NOT_ENABLED", wantKind: modelinvoker.ErrorBilling},
		{name: "rate limit", statusCode: 429, status: "RESOURCE_EXHAUSTED", wantKind: modelinvoker.ErrorRateLimit, wantRetry: true},
		{name: "unavailable", statusCode: 503, status: "UNAVAILABLE", wantKind: modelinvoker.ErrorProviderUnavailable, wantRetry: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writer.Header().Set("x-goog-request-id", "req_error_header")
				writer.Header().Set("retry-after", "2")
				body := fmt.Sprintf(`{"error":{"code":%d,"message":"provider failure","status":%q,"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"750ms"}]}}`, test.statusCode, test.status)
				writeJSON(writer, test.statusCode, []byte(body))
			}))
			defer server.Close()

			response, err := newAdapter(t, server.URL, server.Client()).Invoke(context.Background(), baseRequest())
			if err == nil {
				t.Fatal("Invoke() error = nil")
			}
			var invocationError *modelinvoker.Error
			if !errors.As(err, &invocationError) {
				t.Fatalf("error type = %T", err)
			}
			if invocationError.Kind != test.wantKind || invocationError.HTTPStatus != test.statusCode ||
				invocationError.RequestID != "req_error_header" || invocationError.Retryable != test.wantRetry {
				t.Fatalf("normalized error = %#v", invocationError)
			}
			if test.wantRetry && invocationError.RetryAfter != 2*time.Second {
				t.Fatalf("retry after = %v, want header maximum 2s", invocationError.RetryAfter)
			}
			if calls.Load() != 1 {
				t.Fatalf("HTTP calls = %d, SDK retried request", calls.Load())
			}
			var sdkError genai.APIError
			if errors.As(err, &sdkError) {
				t.Fatal("public unwrap chain exposes genai.APIError")
			}
			if response.Status != modelinvoker.ResponseStatusFailed || response.RequestID != "req_error_header" || response.RawResponse.Empty() {
				t.Fatalf("failed response audit = %#v", response)
			}
			if strings.Contains(err.Error(), testAPIKey) || strings.Contains(string(response.RawRequest.Bytes()), testAPIKey) ||
				strings.Contains(string(response.RawResponse.Bytes()), testAPIKey) {
				t.Fatal("error or raw audit leaked API key")
			}
		})
	}
}

func TestTransportErrorDoesNotEnterPublicUnwrapChain(t *testing.T) {
	var calls atomic.Int64
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, &url.Error{Op: request.Method, URL: request.URL.String(), Err: errors.New("offline")}
	})}
	adapter := newAdapter(t, "", client)
	_, err := adapter.Invoke(context.Background(), baseRequest())
	if err == nil || modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorProviderUnavailable {
		t.Fatalf("Invoke() error = %v", err)
	}
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || !invocationError.Retryable {
		t.Fatalf("normalized error = %#v", invocationError)
	}
	var transportError *url.Error
	if errors.As(err, &transportError) {
		t.Fatalf("public unwrap chain exposes transport/SDK error: %#v", transportError)
	}
	var sdkError genai.APIError
	if errors.As(err, &sdkError) {
		t.Fatal("public unwrap chain exposes genai.APIError")
	}
	if calls.Load() != 1 {
		t.Fatalf("transport calls = %d, want one", calls.Load())
	}
}

func TestContextCancellationPreservesOnlyContextSentinel(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	adapter := newAdapter(t, "", client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := adapter.Invoke(ctx, baseRequest())
	if modelinvoker.ErrorKindOf(err) != modelinvoker.ErrorCancelled || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled error = %v", err)
	}
	var sdkError genai.APIError
	if errors.As(err, &sdkError) {
		t.Fatal("cancelled error exposes genai.APIError")
	}
}

func TestMalformedPolicyAndMultipleCandidateResponses(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantKind modelinvoker.ErrorKind
	}{
		{name: "malformed JSON", body: `{`, wantKind: modelinvoker.ErrorProvider},
		{name: "no candidate", body: `{"responseId":"empty","usageMetadata":{"promptTokenCount":1}}`, wantKind: modelinvoker.ErrorProvider},
		{name: "nil content", body: `{"candidates":[{"finishReason":"STOP","index":0}]}`, wantKind: modelinvoker.ErrorProvider},
		{name: "multiple candidates", body: `{"candidates":[{"content":{"role":"model","parts":[{"text":"a"}]},"finishReason":"STOP","index":0},{"content":{"role":"model","parts":[{"text":"b"}]},"finishReason":"STOP","index":1}]}`, wantKind: modelinvoker.ErrorMapping},
		{name: "malformed function call", body: `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"args":{}}}]},"finishReason":"MALFORMED_FUNCTION_CALL","index":0}]}`, wantKind: modelinvoker.ErrorProvider},
		{name: "prompt policy", body: `{"promptFeedback":{"blockReason":"PROHIBITED_CONTENT","blockReasonMessage":"blocked"}}`, wantKind: modelinvoker.ErrorPolicyRejected},
		{name: "candidate policy", body: `{"candidates":[{"content":{"role":"model","parts":[{"text":"blocked"}]},"finishReason":"SAFETY","finishMessage":"unsafe","index":0}]}`, wantKind: modelinvoker.ErrorPolicyRejected},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writeJSON(writer, http.StatusOK, []byte(test.body))
			}))
			defer server.Close()
			response, err := newAdapter(t, server.URL, server.Client()).Invoke(context.Background(), baseRequest())
			if err == nil || modelinvoker.ErrorKindOf(err) != test.wantKind {
				t.Fatalf("Invoke() response/error = %#v / %v, want kind %q", response, err, test.wantKind)
			}
			var invocationError *modelinvoker.Error
			if errors.As(err, &invocationError) && invocationError.Retryable {
				t.Fatalf("2xx malformed/policy response is retryable: %#v", invocationError)
			}
			if response.RawResponse.Empty() || strings.Contains(string(response.RawResponse.Bytes()), testAPIKey) {
				t.Fatalf("2xx failure RawResponse = %q", response.RawResponse.Bytes())
			}
		})
	}
}

func TestStreamInitialHTTPErrorIsNormalizedWithoutRetry(t *testing.T) {
	fixture := mustFixture(t, "testdata/error-resource-exhausted.json")
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.Header().Set("x-goog-request-id", "req_stream_error")
		writer.Header().Set("retry-after", "1")
		writeJSON(writer, http.StatusTooManyRequests, fixture)
	}))
	defer server.Close()
	_, err := newAdapter(t, server.URL, server.Client()).Stream(context.Background(), baseRequest())
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError.Kind != modelinvoker.ErrorRateLimit ||
		invocationError.RequestID != "req_stream_error" || invocationError.RetryAfter != time.Second {
		t.Fatalf("stream error = %#v", invocationError)
	}
	if calls.Load() != 1 {
		t.Fatalf("stream HTTP calls = %d, want one", calls.Load())
	}
}
