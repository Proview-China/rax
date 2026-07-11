package protocol

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

type FailureSource string

const (
	FailureSourceTransport FailureSource = "transport"
	FailureSourceContext   FailureSource = "context"
	FailureSourceHTTP      FailureSource = "http"
	FailureSourceSDK       FailureSource = "sdk"
	FailureSourceStream    FailureSource = "stream"
	FailureSourceProtocol  FailureSource = "protocol"
)

type FailureContext string

const (
	FailureContextNone             FailureContext = ""
	FailureContextCancelled        FailureContext = "cancelled"
	FailureContextDeadlineExceeded FailureContext = "deadline_exceeded"
)

// Signal is one whitelisted, data-only provider failure hint, for example a
// quota reason extracted from a Gemini details record. It cannot retain an SDK
// object or unstructured map.
type Signal struct {
	Key   string
	Value string
}

// Failure is the safe protocol-to-dialect error handoff. It intentionally has
// no error, http.Request, headers, credential, any, raw byte, or SDK field.
type Failure struct {
	Source     FailureSource
	Context    FailureContext
	HTTPStatus int
	Type       string
	Code       string
	Message    string
	RequestID  string
	RetryAfter time.Duration
	Signals    []Signal
	Raw        modelinvoker.RawPayload
}

type ErrorClassification struct {
	Kind       modelinvoker.ErrorKind
	Code       string
	Message    string
	Retryable  bool
	RetryAfter time.Duration
}

var signalKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,127}$`)

func (f Failure) Clone() Failure {
	clone := f
	clone.Signals = append([]Signal(nil), f.Signals...)
	clone.Raw = modelinvoker.NewRawPayload(f.Raw.Bytes())
	return clone
}

func (f Failure) Validate() error {
	if !f.Source.valid() {
		return fmt.Errorf("failure source is invalid")
	}
	if !f.Context.valid() {
		return fmt.Errorf("failure context is invalid")
	}
	if f.Source == FailureSourceContext && f.Context == FailureContextNone {
		return fmt.Errorf("context failure requires a cancellation or deadline context")
	}
	if f.HTTPStatus < 0 || f.HTTPStatus > 599 {
		return fmt.Errorf("failure HTTP status is invalid")
	}
	if f.RetryAfter < 0 {
		return fmt.Errorf("failure retry delay is invalid")
	}
	for name, value := range map[string]string{"type": f.Type, "code": f.Code, "message": f.Message, "request_id": f.RequestID} {
		if len(value) > 4096 || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("failure %s must be bounded and single-line", name)
		}
	}
	seen := make(map[string]struct{}, len(f.Signals))
	for index, signal := range f.Signals {
		if !signalKeyPattern.MatchString(signal.Key) || signal.Value == "" || len(signal.Value) > 4096 || strings.ContainsAny(signal.Value, "\r\n") {
			return fmt.Errorf("failure signal %d is invalid", index)
		}
		key := signal.Key + "\x00" + signal.Value
		if _, exists := seen[key]; exists {
			return fmt.Errorf("failure signal %d is duplicated", index)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// ContextFailureOf reduces arbitrary cancellation causes to the only two
// sentinels permitted in the public unwrap chain.
func ContextFailureOf(ctx context.Context, err error) FailureContext {
	if ctx != nil {
		switch ctx.Err() {
		case context.DeadlineExceeded:
			return FailureContextDeadlineExceeded
		case context.Canceled:
			return FailureContextCancelled
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return FailureContextDeadlineExceeded
	}
	if errors.Is(err, context.Canceled) {
		return FailureContextCancelled
	}
	return FailureContextNone
}

func (b *Base) NormalizeFailure(ctx context.Context, request modelinvoker.Request, operation string, failure Failure) *modelinvoker.Error {
	if b == nil || IsNil(b.dialect) {
		return bindingError("", modelinvoker.ErrorProviderUnavailable, operation, "protocol base is not initialized")
	}
	if fromContext := ContextFailureOf(ctx, nil); fromContext != FailureContextNone {
		failure.Context = fromContext
	}
	if err := failure.Validate(); err != nil {
		return b.binding.stampError(ctx, request, bindingError(b.binding.Provider, modelinvoker.ErrorProvider, operation, "protocol driver produced an invalid failure"), operation)
	}
	if failure.Context == FailureContextCancelled {
		return b.binding.stampError(ctx, request, &modelinvoker.Error{
			Kind: modelinvoker.ErrorCancelled, Provider: b.binding.Provider, Operation: operation,
			Message: "operation was cancelled", Err: context.Canceled,
		}, operation)
	}
	if failure.Context == FailureContextDeadlineExceeded {
		return b.binding.stampError(ctx, request, &modelinvoker.Error{
			Kind: modelinvoker.ErrorTimeout, Provider: b.binding.Provider, Operation: operation,
			Message: "operation timed out", Err: context.DeadlineExceeded,
		}, operation)
	}
	classification := b.dialect.ClassifyFailure(failure.Clone())
	if !validErrorKind(classification.Kind) {
		return b.binding.stampError(ctx, request, &modelinvoker.Error{
			Kind: modelinvoker.ErrorProvider, Provider: b.binding.Provider, Operation: operation,
			Code: "invalid_error_classification", Message: "provider failure classification was invalid",
			HTTPStatus: failure.HTTPStatus, RequestID: failure.RequestID,
		}, operation)
	}
	code := classification.Code
	if !safeClassificationText(code) {
		code = ""
	}
	if code == "" {
		code = failure.Code
	}
	message := classification.Message
	if message == "" || !safeClassificationText(message) {
		message = "provider operation failed"
	}
	retryAfter := classification.RetryAfter
	if failure.RetryAfter > retryAfter {
		retryAfter = failure.RetryAfter
	}
	if retryAfter < 0 {
		retryAfter = 0
	}
	if classification.Retryable && !retryableErrorKind(classification.Kind) {
		classification.Retryable = false
		retryAfter = 0
	}
	return b.binding.stampError(ctx, request, &modelinvoker.Error{
		Kind: classification.Kind, Provider: b.binding.Provider, Operation: operation,
		Code: code, Message: message, HTTPStatus: failure.HTTPStatus,
		RequestID: failure.RequestID, Retryable: classification.Retryable, RetryAfter: retryAfter,
	}, operation)
}

func retryableErrorKind(kind modelinvoker.ErrorKind) bool {
	return kind == modelinvoker.ErrorRateLimit || kind == modelinvoker.ErrorTimeout ||
		kind == modelinvoker.ErrorProviderUnavailable || kind == modelinvoker.ErrorProvider
}

func safeClassificationText(value string) bool {
	return len(value) <= 4096 && !strings.ContainsAny(value, "\r\n")
}

func (source FailureSource) valid() bool {
	switch source {
	case FailureSourceTransport, FailureSourceContext, FailureSourceHTTP, FailureSourceSDK, FailureSourceStream, FailureSourceProtocol:
		return true
	default:
		return false
	}
}

func (failureContext FailureContext) valid() bool {
	return failureContext == FailureContextNone || failureContext == FailureContextCancelled || failureContext == FailureContextDeadlineExceeded
}

func validErrorKind(kind modelinvoker.ErrorKind) bool {
	switch kind {
	case modelinvoker.ErrorAuthentication,
		modelinvoker.ErrorBilling,
		modelinvoker.ErrorPermission,
		modelinvoker.ErrorInvalidRequest,
		modelinvoker.ErrorUnsupportedCapability,
		modelinvoker.ErrorRateLimit,
		modelinvoker.ErrorTimeout,
		modelinvoker.ErrorCancelled,
		modelinvoker.ErrorProviderUnavailable,
		modelinvoker.ErrorPolicyRejected,
		modelinvoker.ErrorStreamInterrupted,
		modelinvoker.ErrorMapping,
		modelinvoker.ErrorUnknownProvider,
		modelinvoker.ErrorDuplicateProvider,
		modelinvoker.ErrorProvider:
		return true
	default:
		return false
	}
}
