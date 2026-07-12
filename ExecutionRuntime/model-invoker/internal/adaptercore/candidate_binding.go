package adaptercore

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

// CandidateBindingResult is the internal, post-construction trust result used
// by built-in factories before a Provider can be published to the Gateway.
type CandidateBindingResult struct {
	Provider modelinvoker.Provider
	Closer   io.Closer
	Endpoint string
}

// CandidateBindingError marks only errors produced by the internal candidate
// finalizer. Route Gateway may preserve this safe structure while treating
// arbitrary AdapterFactory build errors as untrusted.
type CandidateBindingError struct{ err error }

func (err *CandidateBindingError) Error() string {
	if err == nil || err.err == nil {
		return "candidate binding rejected"
	}
	return err.err.Error()
}
func (err *CandidateBindingError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.err
}

// FinalizeCandidateBinding validates the constructed Provider identity and
// adapter-owned endpoint receipt. Every rejected non-nil candidate is closed
// exactly once, and close causes remain unwrap-observable without entering the
// public error text.
func FinalizeCandidateBinding(ctx context.Context, expected modelinvoker.ProviderID, protocolID modelinvoker.Protocol, requestedEndpoint string, provider modelinvoker.Provider, buildErr error) (CandidateBindingResult, error) {
	closer := candidateCloser(provider)
	if buildErr != nil {
		return CandidateBindingResult{}, candidateBindingError(errors.Join(
			candidateError(modelinvoker.ErrorProviderUnavailable, "factory_build_failed", "adapter factory failed", nil),
			closeCandidate(closer),
		))
	}
	if contextErr := candidateContextError(ctx); contextErr != nil {
		return CandidateBindingResult{}, candidateBindingError(errors.Join(
			candidateError(candidateContextKind(contextErr), "factory_context_done", "adapter construction was cancelled before publication", contextErr),
			closeCandidate(closer),
		))
	}
	if nilCandidate(provider) {
		return CandidateBindingResult{}, candidateBindingError(candidateError(modelinvoker.ErrorProviderUnavailable, "factory_provider_nil", "adapter factory returned a nil provider", nil))
	}
	if provider.ID() != expected {
		return CandidateBindingResult{}, candidateBindingError(errors.Join(
			candidateError(modelinvoker.ErrorMapping, "factory_provider_mismatch", "adapter factory returned a provider with the wrong identity", nil),
			closeCandidate(closer),
		))
	}
	receipt, ok := provider.(CandidateBindingReceipt)
	if !ok {
		return CandidateBindingResult{}, candidateBindingError(errors.Join(
			candidateError(modelinvoker.ErrorMapping, "factory_endpoint_receipt_missing", "adapter does not expose the internal construction endpoint receipt", nil),
			closeCandidate(closer),
		))
	}
	endpoint, ok := receipt.CandidateBindingEndpoint(protocolID, requestedEndpoint)
	if !ok || strings.TrimSpace(endpoint) == "" {
		return CandidateBindingResult{}, candidateBindingError(errors.Join(
			candidateError(modelinvoker.ErrorMapping, "factory_endpoint_receipt_invalid", "adapter construction receipt does not match the selected protocol binding", nil),
			closeCandidate(closer),
		))
	}
	return CandidateBindingResult{Provider: provider, Closer: closer, Endpoint: endpoint}, nil
}

func candidateCloser(provider modelinvoker.Provider) io.Closer {
	if nilCandidate(provider) {
		return nil
	}
	if closer, ok := provider.(io.Closer); ok {
		return closer
	}
	return candidateNoopCloser{}
}

type candidateNoopCloser struct{}

func (candidateNoopCloser) Close() error { return nil }

func closeCandidate(closer io.Closer) error {
	if closer == nil {
		return nil
	}
	if err := closer.Close(); err != nil {
		return candidateError(modelinvoker.ErrorProviderUnavailable, "adapter_close_failed", "adapter close failed", &candidateLifecycleCause{raw: err})
	}
	return nil
}

func candidateBindingError(err error) *CandidateBindingError {
	return &CandidateBindingError{err: err}
}

type candidateLifecycleCause struct{ raw error }

func (*candidateLifecycleCause) Error() string { return "adapter lifecycle cause" }
func (cause *candidateLifecycleCause) Unwrap() error {
	return nil
}
func (cause *candidateLifecycleCause) Is(target error) bool {
	return cause != nil && errors.Is(cause.raw, target)
}

// IsCandidateLifecycleCause lets Route Gateway preserve only lifecycle causes
// created by this internal finalizer while sanitizing arbitrary provider errors.
func IsCandidateLifecycleCause(err error) bool {
	_, ok := err.(*candidateLifecycleCause)
	return ok
}

func candidateError(kind modelinvoker.ErrorKind, code, message string, err error) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: kind, Operation: "route_gateway", Code: code, Message: message, Err: err}
}

func candidateContextError(ctx context.Context) error {
	if ctx == nil {
		return context.Canceled
	}
	return ctx.Err()
}

func candidateContextKind(err error) modelinvoker.ErrorKind {
	if errors.Is(err, context.DeadlineExceeded) {
		return modelinvoker.ErrorTimeout
	}
	return modelinvoker.ErrorCancelled
}

func nilCandidate(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
