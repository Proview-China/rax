package operation

import (
	"context"
	"errors"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

type Invoker struct{ registry *Registry }

func NewInvoker(registry *Registry) (*Invoker, error) {
	if registry == nil {
		return nil, operationError("", modelinvoker.ErrorInvalidRequest, "new_invoker", "", "operation registry is required")
	}
	return &Invoker{registry: registry}, nil
}

func (i *Invoker) Invoke(ctx context.Context, request Request) (Result, error) {
	provider, report, call, cancel, err := i.prepare(ctx, request)
	if err != nil {
		return Result{}, err
	}
	defer cancel()
	result, err := provider.Invoke(call, request)
	result = complete(result, request, report)
	if err != nil {
		return result, normalizeError(call, request.Provider, "invoke", err)
	}
	return result, nil
}

func (i *Invoker) Stream(ctx context.Context, request Request) (Stream, error) {
	provider, _, call, cancel, err := i.prepare(ctx, request)
	if err != nil {
		return nil, err
	}
	stream, err := provider.Stream(call, request)
	if err != nil {
		cancel()
		return nil, normalizeError(call, request.Provider, "stream", err)
	}
	if stream == nil {
		cancel()
		return nil, operationError(request.Provider, modelinvoker.ErrorStreamInterrupted, "stream", "", "operation provider returned a nil stream")
	}
	return &contextStream{inner: stream, cancel: cancel}, nil
}

func (i *Invoker) prepare(ctx context.Context, request Request) (Provider, MappingReport, context.Context, context.CancelFunc, error) {
	if i == nil || i.registry == nil {
		return nil, MappingReport{}, nil, nil, operationError(request.Provider, modelinvoker.ErrorInvalidRequest, "prepare", "", "operation invoker is not initialized")
	}
	if ctx == nil {
		return nil, MappingReport{}, nil, nil, operationError(request.Provider, modelinvoker.ErrorInvalidRequest, "prepare", "", "context is nil")
	}
	if err := request.Validate(); err != nil {
		return nil, MappingReport{}, nil, nil, operationError(request.Provider, modelinvoker.ErrorInvalidRequest, "validate", "", err.Error())
	}
	provider, err := i.registry.Get(request.Provider)
	if err != nil {
		return nil, MappingReport{}, nil, nil, err
	}
	contract, err := provider.Capabilities(ctx, Query{Kind: request.Kind, Model: request.Model})
	if err != nil {
		return nil, MappingReport{}, nil, nil, err
	}
	report, err := Evaluate(request, contract)
	if err != nil {
		return nil, report, nil, nil, err
	}
	call, cancel := context.WithCancel(ctx)
	if request.Budget.Timeout > 0 {
		call, cancel = context.WithTimeout(ctx, request.Budget.Timeout)
	}
	return provider, report, call, cancel, nil
}

func complete(result Result, request Request, report MappingReport) Result {
	if result.Provider == "" {
		result.Provider = request.Provider
	}
	if result.Kind == "" {
		result.Kind = request.Kind
	}
	if result.Model == "" {
		result.Model = request.Model
	}
	result.MappingReport = report
	return result
}

func normalizeError(ctx context.Context, provider modelinvoker.ProviderID, operation string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return operationError(provider, modelinvoker.ErrorTimeout, operation, "", "operation timed out")
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return operationError(provider, modelinvoker.ErrorCancelled, operation, "", "operation was cancelled")
	}
	var typed *modelinvoker.Error
	if errors.As(err, &typed) {
		return typed
	}
	return &modelinvoker.Error{Kind: modelinvoker.ErrorProvider, Provider: provider, Operation: operation, Message: "operation provider failed", Err: err}
}

type contextStream struct {
	inner  Stream
	cancel context.CancelFunc
}

func (s *contextStream) Next() bool { return s != nil && s.inner != nil && s.inner.Next() }
func (s *contextStream) Event() StreamEvent {
	if s == nil || s.inner == nil {
		return StreamEvent{}
	}
	return s.inner.Event()
}
func (s *contextStream) Err() error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.Err()
}
func (s *contextStream) Close() error {
	if s == nil {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.inner != nil {
		return s.inner.Close()
	}
	return nil
}
