package protocol_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
)

func TestBeginFailureExtractionIsSafeAndDeterministic(t *testing.T) {
	failure, existing, ready := protocol.BeginFailureExtraction(context.Background(), "req-1", nil)
	if !ready || existing != nil || failure.Source != protocol.FailureSourceSDK || failure.RequestID != "req-1" {
		t.Fatalf("nil extraction = %#v/%#v/%v", failure, existing, ready)
	}

	native := errors.New("native sdk secret")
	public := &modelinvoker.Error{Kind: modelinvoker.ErrorMapping, Message: "safe", Err: native}
	_, existing, ready = protocol.BeginFailureExtraction(context.Background(), "req-2", public)
	if !ready || existing != public {
		t.Fatalf("existing public error = %#v/%v", existing, ready)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	failure, existing, ready = protocol.BeginFailureExtraction(cancelled, "req-3", native)
	if !ready || existing != nil || failure.Source != protocol.FailureSourceContext || failure.Context != protocol.FailureContextCancelled {
		t.Fatalf("cancelled extraction = %#v/%#v/%v", failure, existing, ready)
	}

	failure, existing, ready = protocol.BeginFailureExtraction(
		context.Background(), "unsafe\nrequest-id", &adaptercore.RedirectError{StatusCode: 307},
	)
	if !ready || existing != nil || failure.Source != protocol.FailureSourceHTTP || failure.HTTPStatus != 307 ||
		failure.Message != "redirect response was rejected" || failure.RequestID != "" {
		t.Fatalf("redirect extraction = %#v/%#v/%v", failure, existing, ready)
	}
}

func TestExtractCommonFailureStoresNoNativeDetails(t *testing.T) {
	secretURL := "https://credential.example.test/bearer-secret"
	transport := &url.Error{Op: "Post", URL: secretURL, Err: errors.New("dial secret-host")}
	failure := protocol.Failure{Source: protocol.FailureSourceSDK}
	if !protocol.ExtractCommonFailure(transport, &failure) || failure.Source != protocol.FailureSourceTransport ||
		failure.Message != "transport failed" || strings.Contains(failure.Message, "secret") {
		t.Fatalf("transport extraction = %#v", failure)
	}

	invalidPayloads := []error{&json.SyntaxError{Offset: 1}, io.ErrUnexpectedEOF}
	for _, err := range invalidPayloads {
		failure = protocol.Failure{Source: protocol.FailureSourceSDK}
		if !protocol.ExtractCommonFailure(err, &failure) || failure.Source != protocol.FailureSourceProtocol ||
			failure.Code != "invalid_response_payload" || failure.Message != "provider returned an invalid response payload" {
			t.Fatalf("payload extraction for %T = %#v", err, failure)
		}
	}

	failure = protocol.Failure{Source: protocol.FailureSourceSDK}
	if protocol.ExtractCommonFailure(errors.New("unknown native secret"), &failure) || failure.Source != protocol.FailureSourceSDK {
		t.Fatalf("unknown extraction = %#v", failure)
	}
}

func TestBoundedFailureTextFailsClosed(t *testing.T) {
	if got := protocol.BoundedFailureText(strings.Repeat("x", 4096)); len(got) != 4096 {
		t.Fatalf("exact bound length = %d", len(got))
	}
	for _, value := range []string{"safe\nsecret", "safe\rsecret", strings.Repeat("x", 4097)} {
		if got := protocol.BoundedFailureText(value); got != "" {
			t.Fatalf("BoundedFailureText(%d bytes) = %q", len(value), got)
		}
	}
}
