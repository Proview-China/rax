package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
)

// BeginFailureExtraction performs the SDK-neutral first pass shared by every
// concrete protocol driver. A ready result can be normalized immediately;
// otherwise the driver may inspect its own SDK error type before calling
// ExtractCommonFailure.
func BeginFailureExtraction(ctx context.Context, requestID string, err error) (failure Failure, existing *modelinvoker.Error, ready bool) {
	failure = Failure{
		Source: FailureSourceSDK, Context: ContextFailureOf(ctx, err),
		RequestID: BoundedFailureText(requestID),
	}
	if err == nil {
		return failure, nil, true
	}
	if errors.As(err, &existing) && existing != nil {
		return failure, existing, true
	}
	if failure.Context != FailureContextNone {
		failure.Source = FailureSourceContext
		return failure, nil, true
	}
	var redirect *adaptercore.RedirectError
	if errors.As(err, &redirect) && redirect != nil {
		failure.Source = FailureSourceHTTP
		failure.HTTPStatus = redirect.StatusCode
		failure.Message = "redirect response was rejected"
		return failure, nil, true
	}
	return failure, nil, false
}

// ExtractCommonFailure recognizes SDK-independent transport and malformed-wire
// failures. It stores no native error, URL, request, header, or credential.
func ExtractCommonFailure(err error, failure *Failure) bool {
	if err == nil || failure == nil {
		return false
	}
	var urlError *url.Error
	if errors.As(err, &urlError) && urlError != nil {
		failure.Source = FailureSourceTransport
		failure.Message = "transport failed"
		return true
	}
	var syntaxError *json.SyntaxError
	var typeError *json.UnmarshalTypeError
	if errors.As(err, &syntaxError) || errors.As(err, &typeError) || errors.Is(err, io.ErrUnexpectedEOF) {
		failure.Source = FailureSourceProtocol
		failure.Code = "invalid_response_payload"
		failure.Message = "provider returned an invalid response payload"
		return true
	}
	return false
}

// BoundedFailureText is the only text gate used before SDK-derived values enter
// Failure. Multiline or oversized values fail closed to an empty field.
func BoundedFailureText(value string) string {
	if len(value) > 4096 || strings.ContainsAny(value, "\r\n") {
		return ""
	}
	return value
}
