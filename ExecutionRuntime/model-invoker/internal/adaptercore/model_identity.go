package adaptercore

import (
	"errors"
)

import modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"

// ResponseModelError requires the authoritative protocol response identity and
// rejects both omission and drift from the exact pre-credential selection.
func ResponseModelError(provider modelinvoker.ProviderID, operation, requested, actual string) error {
	if actual == requested && actual != "" {
		return nil
	}
	message := "provider response model does not match the exact requested model"
	code := "response_model_mismatch"
	if actual == "" {
		message = "provider response omitted the required authoritative model identity"
		code = "response_model_missing"
	}
	return &modelinvoker.Error{
		Kind: modelinvoker.ErrorMapping, Provider: provider, Operation: operation, Code: code,
		Message: message,
	}
}

// SafeCloseError keeps a provider Close cause available to errors.Is without
// exposing its untrusted text or raw error tree through Error/Unwrap.
func SafeCloseError(provider modelinvoker.ProviderID, operation string, err error) error {
	if err == nil {
		return nil
	}
	return &modelinvoker.Error{
		Kind: modelinvoker.ErrorProviderUnavailable, Provider: provider, Operation: operation,
		Code: "stream_close_failed", Message: "provider stream close failed", Err: &safeCloseCause{raw: err},
	}
}

type safeCloseCause struct{ raw error }

func (*safeCloseCause) Error() string { return "provider stream close cause" }
func (*safeCloseCause) Unwrap() error { return nil }
func (cause *safeCloseCause) Is(target error) bool {
	return cause != nil && errors.Is(cause.raw, target)
}

// SafeCloseCauseOf returns only the opaque cause created by SafeCloseError.
// Protocol identity stamping can preserve this errors.Is bridge without
// retaining any arbitrary provider error tree.
func SafeCloseCauseOf(err error) error {
	var cause *safeCloseCause
	if errors.As(err, &cause) && cause != nil {
		return cause
	}
	return nil
}
