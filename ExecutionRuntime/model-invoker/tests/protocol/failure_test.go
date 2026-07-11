package protocol_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
)

func TestBaseNormalizeFailurePreservesOnlySafeClassification(t *testing.T) {
	dialect := &fakeDialect{
		classification: protocol.ErrorClassification{
			Kind: modelinvoker.ErrorRateLimit, Message: "safe classified failure", Retryable: true, RetryAfter: 100 * time.Millisecond,
		},
		mutateFailure: true,
	}
	base, err := protocol.NewBase(mustBinding(t), dialect)
	if err != nil {
		t.Fatal(err)
	}
	failure := protocol.Failure{
		Source: protocol.FailureSourceSDK, HTTPStatus: http.StatusTooManyRequests,
		Type: "api_error", Code: "quota_exhausted", Message: "untrusted native message", RequestID: "req_safe",
		RetryAfter: 250 * time.Millisecond,
		Signals:    []protocol.Signal{{Key: "quota.reason", Value: "rate_limit"}},
		Raw:        modelinvoker.NewRawPayload([]byte(`{"error":"safe-audit"}`)),
	}
	got := base.NormalizeFailure(context.Background(), validRequest(""), "fake.invoke", failure)
	if got.Kind != modelinvoker.ErrorRateLimit || got.Provider != testProvider || got.Operation != "fake.invoke" ||
		got.Code != failure.Code || got.Message != "safe classified failure" || got.HTTPStatus != http.StatusTooManyRequests ||
		got.RequestID != "req_safe" || !got.Retryable || got.RetryAfter != 250*time.Millisecond || got.Err != nil {
		t.Fatalf("NormalizeFailure() = %#v", got)
	}
	if got.MappingReport.Provider != testProvider || got.MappingReport.Protocol != modelinvoker.ProtocolChatCompletions || got.MappingReport.Endpoint != testEndpoint {
		t.Fatalf("normalized error MappingReport = %#v", got.MappingReport)
	}
	if dialect.classifyCalls != 1 || dialect.lastFailure.Signals[0].Value != "rate_limit" ||
		failure.Signals[0].Value != "rate_limit" || string(failure.Raw.Bytes()) != `{"error":"safe-audit"}` {
		t.Fatalf("dialect received shared Failure state: %#v / %#v", dialect.lastFailure, failure)
	}
}

func TestFailureAndStampErrorCannotRetainSDKRequestOrCredential(t *testing.T) {
	const secret = "provider-credential-secret"
	native := &fakeSDKError{
		Request: &http.Request{
			Method: http.MethodPost,
			Header: http.Header{"Authorization": []string{"Bearer " + secret}},
		},
		Credential: fakeCredential{APIKey: secret},
		message:    "SDK failed with " + secret,
	}
	binding := mustBinding(t)
	got := binding.StampError(context.Background(), validRequest(""), native, "fake.invoke")
	var invocationError *modelinvoker.Error
	if !errors.As(got, &invocationError) || invocationError.Provider != testProvider || invocationError.Kind != modelinvoker.ErrorProvider || invocationError.Err != nil || invocationError.Message != "protocol driver failed" {
		t.Fatalf("StampError(native) = %#v", got)
	}
	var exposed *fakeSDKError
	if errors.As(got, &exposed) {
		t.Fatal("public unwrap chain retained fake SDK error")
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	formatted := fmt.Sprintf("%v|%+v|%#v|%s", got, got, got, encoded)
	if strings.Contains(formatted, secret) || strings.Contains(formatted, "Authorization") {
		t.Fatalf("public error retained SDK request or credential: %s", formatted)
	}
}

func TestContextFailureAllowlistDropsCustomCancellationCauses(t *testing.T) {
	custom := errors.New("custom secret cancellation cause")
	cancelled, cancel := context.WithCancelCause(context.Background())
	cancel(custom)
	if context.Cause(cancelled) != custom {
		t.Fatal("test cancellation cause was not installed")
	}

	timedOut, stop := context.WithTimeoutCause(context.Background(), 0, custom)
	defer stop()
	<-timedOut.Done()
	if context.Cause(timedOut) != custom {
		t.Fatal("test timeout cause was not installed")
	}

	tests := []struct {
		name        string
		ctx         context.Context
		err         error
		wantContext protocol.FailureContext
	}{
		{name: "plain error", ctx: context.Background(), err: custom, wantContext: protocol.FailureContextNone},
		{name: "wrapped cancellation", ctx: context.Background(), err: fmt.Errorf("wrapped: %w", context.Canceled), wantContext: protocol.FailureContextCancelled},
		{name: "wrapped deadline", ctx: context.Background(), err: fmt.Errorf("wrapped: %w", context.DeadlineExceeded), wantContext: protocol.FailureContextDeadlineExceeded},
		{name: "custom cancellation cause", ctx: cancelled, err: custom, wantContext: protocol.FailureContextCancelled},
		{name: "custom timeout cause", ctx: timedOut, err: custom, wantContext: protocol.FailureContextDeadlineExceeded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := protocol.ContextFailureOf(test.ctx, test.err); got != test.wantContext {
				t.Fatalf("ContextFailureOf() = %q, want %q", got, test.wantContext)
			}
		})
	}

	base, err := protocol.NewBase(mustBinding(t), &fakeDialect{classification: protocol.ErrorClassification{
		Kind: modelinvoker.ErrorProvider, Retryable: true, RetryAfter: time.Hour,
	}})
	if err != nil {
		t.Fatal(err)
	}
	request := validRequest("")
	validFailure := protocol.Failure{Source: protocol.FailureSourceTransport, RetryAfter: 30 * time.Second}

	for _, test := range []struct {
		name     string
		ctx      context.Context
		failure  protocol.Failure
		wantKind modelinvoker.ErrorKind
		sentinel error
	}{
		{name: "context cancellation", ctx: cancelled, failure: validFailure, wantKind: modelinvoker.ErrorCancelled, sentinel: context.Canceled},
		{name: "context timeout", ctx: timedOut, failure: validFailure, wantKind: modelinvoker.ErrorTimeout, sentinel: context.DeadlineExceeded},
		{name: "explicit cancellation", ctx: context.Background(), failure: protocol.Failure{Source: protocol.FailureSourceTransport, Context: protocol.FailureContextCancelled}, wantKind: modelinvoker.ErrorCancelled, sentinel: context.Canceled},
		{name: "explicit timeout", ctx: context.Background(), failure: protocol.Failure{Source: protocol.FailureSourceTransport, Context: protocol.FailureContextDeadlineExceeded}, wantKind: modelinvoker.ErrorTimeout, sentinel: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := base.NormalizeFailure(test.ctx, request, "fake.invoke", test.failure)
			if got.Kind != test.wantKind || got.Err != test.sentinel || !errors.Is(got, test.sentinel) || errors.Is(got, custom) ||
				got.Retryable || got.RetryAfter != 0 {
				t.Fatalf("normalized context failure = %#v", got)
			}
		})
	}

	ordinary := base.NormalizeFailure(context.Background(), request, "fake.invoke", validFailure)
	if ordinary.Err != nil || errors.Is(ordinary, custom) || errors.Is(ordinary, context.Canceled) || errors.Is(ordinary, context.DeadlineExceeded) {
		t.Fatalf("ordinary Failure retained a cause: %#v", ordinary)
	}

	original := &modelinvoker.Error{
		Kind: modelinvoker.ErrorRateLimit, Retryable: true, RetryAfter: time.Hour, Err: custom,
		Message: "retryable before cancellation",
	}
	stamped := mustBinding(t).StampError(cancelled, request, original, "fake.invoke")
	var stampedInvocationError *modelinvoker.Error
	if !errors.As(stamped, &stampedInvocationError) || stampedInvocationError.Kind != modelinvoker.ErrorCancelled || stampedInvocationError.Err != context.Canceled || stampedInvocationError.Retryable || stampedInvocationError.RetryAfter != 0 || errors.Is(stamped, custom) {
		t.Fatalf("context did not clear retry policy from original Error: %#v", stamped)
	}
	if !original.Retryable || original.RetryAfter != time.Hour || original.Err != custom {
		t.Fatal("StampError mutated original retryable Error")
	}
}

func TestFailureValidationCloneAndRawAreDefensive(t *testing.T) {
	const secret = "raw-provider-secret"
	source := []byte(`{"token":"` + secret + `"}`)
	failure := protocol.Failure{
		Source:  protocol.FailureSourceProtocol,
		Signals: []protocol.Signal{{Key: "protocol.reason", Value: "malformed"}},
		Raw:     modelinvoker.NewRawPayload(source),
	}
	if err := failure.Validate(); err != nil {
		t.Fatalf("valid Failure rejected: %v", err)
	}
	source[2] = 'X'
	clone := failure.Clone()
	clone.Signals[0].Value = "mutated"
	bytes := clone.Raw.Bytes()
	bytes[2] = 'Y'
	if failure.Signals[0].Value != "malformed" || !strings.Contains(string(failure.Raw.Bytes()), secret) {
		t.Fatalf("Failure.Clone shared storage: %#v", failure)
	}
	encoded, err := json.Marshal(failure)
	if err != nil {
		t.Fatal(err)
	}
	formatted := fmt.Sprintf("%v|%#v|%s", failure, failure, encoded)
	if strings.Contains(formatted, secret) || !strings.Contains(formatted, "[REDACTED]") {
		t.Fatalf("default Failure formatting leaked Raw: %s", formatted)
	}

	invalid := []protocol.Failure{
		{},
		{Source: protocol.FailureSource("unknown")},
		{Source: protocol.FailureSourceHTTP, Context: protocol.FailureContext("unknown")},
		{Source: protocol.FailureSourceHTTP, HTTPStatus: 600},
		{Source: protocol.FailureSourceHTTP, RetryAfter: -time.Nanosecond},
		{Source: protocol.FailureSourceHTTP, Message: "bad\r\nmessage"},
		{Source: protocol.FailureSourceHTTP, Signals: []protocol.Signal{{Key: "UPPER", Value: "x"}}},
		{Source: protocol.FailureSourceHTTP, Signals: []protocol.Signal{{Key: "same", Value: "x"}, {Key: "same", Value: "x"}}},
	}
	for index, candidate := range invalid {
		if err := candidate.Validate(); err == nil {
			t.Fatalf("invalid Failure %d accepted: %#v", index, candidate)
		}
	}
}

func TestInvalidFailureDoesNotReachDialectOrExposeFields(t *testing.T) {
	dialect := &fakeDialect{classification: protocol.ErrorClassification{Kind: modelinvoker.ErrorRateLimit}}
	base, err := protocol.NewBase(mustBinding(t), dialect)
	if err != nil {
		t.Fatal(err)
	}
	got := base.NormalizeFailure(context.Background(), validRequest(""), "fake.invoke", protocol.Failure{
		Source: protocol.FailureSourceSDK, Message: "unsafe\r\nmessage",
	})
	if got.Kind != modelinvoker.ErrorProvider || got.Provider != testProvider || got.Err != nil || dialect.classifyCalls != 0 {
		t.Fatalf("invalid Failure result/calls = %#v / %d", got, dialect.classifyCalls)
	}
	if strings.Contains(fmt.Sprintf("%#v", got), "unsafe") {
		t.Fatal("invalid Failure fields crossed public boundary")
	}
}

func TestInvalidOrEmptyClassificationFailsClosed(t *testing.T) {
	for _, kind := range []modelinvoker.ErrorKind{"", "future_untrusted_kind"} {
		t.Run(string(kind), func(t *testing.T) {
			dialect := &fakeDialect{classification: protocol.ErrorClassification{
				Kind: kind, Code: "unsafe_code", Message: "unsafe message", Retryable: true, RetryAfter: time.Hour,
			}}
			base, err := protocol.NewBase(mustBinding(t), dialect)
			if err != nil {
				t.Fatal(err)
			}
			got := base.NormalizeFailure(context.Background(), validRequest(""), "fake.invoke", protocol.Failure{
				Source: protocol.FailureSourceSDK, HTTPStatus: http.StatusServiceUnavailable,
				Code: "native_code", RequestID: "req_invalid_classification", RetryAfter: 30 * time.Second,
			})
			if got.Kind != modelinvoker.ErrorProvider || got.Code != "invalid_error_classification" ||
				got.Retryable || got.RetryAfter != 0 || got.Err != nil {
				t.Fatalf("invalid classification did not fail closed: %#v", got)
			}
			if got.Provider != testProvider || got.MappingReport.Provider != testProvider || got.MappingReport.Endpoint != testEndpoint {
				t.Fatalf("fail-closed classification identity = %#v", got)
			}
		})
	}
}

func TestHTTPTimeoutClassificationMayRemainRetryable(t *testing.T) {
	dialect := &fakeDialect{classification: protocol.ErrorClassification{
		Kind: modelinvoker.ErrorTimeout, Message: "upstream request timed out", Retryable: true,
	}}
	base, err := protocol.NewBase(mustBinding(t), dialect)
	if err != nil {
		t.Fatal(err)
	}
	got := base.NormalizeFailure(context.Background(), validRequest(""), "fake.invoke", protocol.Failure{
		Source: protocol.FailureSourceHTTP, HTTPStatus: http.StatusRequestTimeout,
	})
	if got.Kind != modelinvoker.ErrorTimeout || !got.Retryable || got.Err != nil {
		t.Fatalf("HTTP timeout classification = %#v", got)
	}
}
