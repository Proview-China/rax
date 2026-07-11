package protocol

import (
	"context"
	"errors"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

// StampMappingReport makes the Binding the authority for every runtime
// identity field while preserving protocol mapping decisions.
func (b Binding) StampMappingReport(request modelinvoker.Request, report modelinvoker.MappingReport) modelinvoker.MappingReport {
	result := report
	result.Provider = b.Provider
	result.Protocol = b.Protocol
	result.Endpoint = b.EffectiveEndpoint(request.Endpoint)
	result.Decisions = append([]modelinvoker.MappingDecision(nil), report.Decisions...)
	return result
}

// StampState returns an independent state value bound to this runtime
// Provider and protocol. The current public State cannot yet carry Route or
// Endpoint identity; later route-resolution work must add that versioned
// boundary before one Adapter serves multiple deployments.
func (b Binding) StampState(state *modelinvoker.State) *modelinvoker.State {
	if state == nil {
		return nil
	}
	result := *state
	result.Provider = b.Provider
	result.Protocol = b.Protocol
	result.Payload = modelinvoker.NewRawPayload(state.Payload.Bytes())
	return &result
}

// StampResponse overwrites all provider/protocol identity surfaces without
// mutating the input response.
func (b Binding) StampResponse(request modelinvoker.Request, response modelinvoker.Response) modelinvoker.Response {
	result := response
	result.Provider = b.Provider
	result.Protocol = b.Protocol
	if result.Model == "" {
		result.Model = request.Model
	}
	result.State = b.StampState(response.State)
	result.MappingReport = b.StampMappingReport(request, response.MappingReport)
	return result
}

// StampError overwrites identity, strips unsafe native causes, and retains
// only context.Canceled or context.DeadlineExceeded in the public unwrap chain.
// It returns the error interface deliberately so a nil input cannot become a
// non-nil interface containing a typed-nil *modelinvoker.Error.
func (b Binding) StampError(ctx context.Context, request modelinvoker.Request, err error, operation string) error {
	stamped := b.stampError(ctx, request, err, operation)
	if stamped == nil {
		return nil
	}
	return stamped
}

func (b Binding) stampError(ctx context.Context, request modelinvoker.Request, err error, operation string) *modelinvoker.Error {
	if err == nil {
		return nil
	}
	contextFailure := ContextFailureOf(ctx, err)
	var invocationError *modelinvoker.Error
	if errors.As(err, &invocationError) && invocationError != nil {
		result := *invocationError
		result.Provider = b.Provider
		if result.Operation == "" {
			result.Operation = operation
		}
		if result.Kind == "" {
			result.Kind = modelinvoker.ErrorProvider
		}
		result.MappingReport = b.StampMappingReport(request, invocationError.MappingReport)
		result.Err = nil
		if contextFailure == FailureContextNone {
			contextFailure = ContextFailureOf(nil, invocationError.Err)
		}
		applySafeContextFailure(&result, contextFailure)
		return &result
	}
	result := &modelinvoker.Error{
		Kind:      modelinvoker.ErrorProvider,
		Provider:  b.Provider,
		Operation: operation,
		Message:   "protocol driver failed",
	}
	result.MappingReport = b.StampMappingReport(request, modelinvoker.MappingReport{})
	applySafeContextFailure(result, contextFailure)
	return result
}

func (b Binding) StampEvent(ctx context.Context, request modelinvoker.Request, event modelinvoker.StreamEvent) modelinvoker.StreamEvent {
	result := event
	if event.Response != nil {
		response := b.StampResponse(request, *event.Response)
		result.Response = &response
	}
	if event.Error != nil {
		result.Error = b.stampError(ctx, request, event.Error, "stream")
	}
	return result
}

func applySafeContextFailure(result *modelinvoker.Error, failure FailureContext) {
	if result == nil {
		return
	}
	switch failure {
	case FailureContextCancelled:
		result.Kind = modelinvoker.ErrorCancelled
		result.Message = "operation was cancelled"
		result.Retryable = false
		result.RetryAfter = 0
		result.Err = context.Canceled
	case FailureContextDeadlineExceeded:
		result.Kind = modelinvoker.ErrorTimeout
		result.Message = "operation timed out"
		result.Retryable = false
		result.RetryAfter = 0
		result.Err = context.DeadlineExceeded
	}
}
