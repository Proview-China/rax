package modelinvoker

import (
	"errors"
	"fmt"
	"time"
)

type ErrorKind string

const (
	ErrorAuthentication        ErrorKind = "authentication"
	ErrorBilling               ErrorKind = "billing"
	ErrorPermission            ErrorKind = "permission_or_model_unavailable"
	ErrorInvalidRequest        ErrorKind = "invalid_request"
	ErrorUnsupportedCapability ErrorKind = "unsupported_capability"
	ErrorRateLimit             ErrorKind = "rate_limit"
	ErrorTimeout               ErrorKind = "timeout"
	ErrorCancelled             ErrorKind = "cancelled"
	ErrorProviderUnavailable   ErrorKind = "provider_unavailable"
	ErrorPolicyRejected        ErrorKind = "policy_rejected"
	ErrorStreamInterrupted     ErrorKind = "stream_interrupted"
	ErrorMapping               ErrorKind = "mapping_failed"
	ErrorUnknownProvider       ErrorKind = "unknown_provider"
	ErrorDuplicateProvider     ErrorKind = "duplicate_provider"
	ErrorProvider              ErrorKind = "provider_error"
)

// Error is the provider-neutral error envelope. Err remains available through
// errors.Unwrap/errors.Is/errors.As without being interpolated into Error().
type Error struct {
	Kind          ErrorKind
	Provider      ProviderID
	Operation     string
	Code          string
	Message       string
	HTTPStatus    int
	RequestID     string
	Retryable     bool
	RetryAfter    time.Duration
	MappingReport MappingReport
	Err           error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	message := e.Message
	if message == "" {
		message = string(e.Kind)
	}
	if e.Provider != "" && e.Operation != "" {
		return fmt.Sprintf("%s %s: %s", e.Provider, e.Operation, message)
	}
	if e.Provider != "" {
		return fmt.Sprintf("%s: %s", e.Provider, message)
	}
	return message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	if !ok || e == nil || other == nil {
		return false
	}
	return (other.Kind == "" || e.Kind == other.Kind) &&
		(other.Provider == "" || e.Provider == other.Provider) &&
		(other.Code == "" || e.Code == other.Code)
}

func NewError(kind ErrorKind, message string) *Error {
	return &Error{Kind: kind, Message: message}
}

func WrapError(kind ErrorKind, message string, err error) *Error {
	return &Error{Kind: kind, Message: message, Err: err}
}

func ErrorKindOf(err error) ErrorKind {
	var invocationError *Error
	if errors.As(err, &invocationError) && invocationError != nil {
		return invocationError.Kind
	}
	return ""
}

func newValidationError(message string) *Error {
	return &Error{Kind: ErrorInvalidRequest, Operation: "validate", Message: message}
}
