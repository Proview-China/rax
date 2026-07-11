package adaptercore

import (
	"context"
	"errors"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

// ContextError converts a context terminal error into the only two causes that
// may cross the provider-neutral public unwrap boundary. Unknown errors fail
// closed without retaining their native value.
func ContextError(provider modelinvoker.ProviderID, operation string, err error) *modelinvoker.Error {
	result := &modelinvoker.Error{
		Kind: modelinvoker.ErrorProvider, Provider: provider, Operation: operation,
		Message: "operation failed",
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		result.Kind = modelinvoker.ErrorTimeout
		result.Message = "request timed out"
		result.Err = context.DeadlineExceeded
	case errors.Is(err, context.Canceled):
		result.Kind = modelinvoker.ErrorCancelled
		result.Message = "request was cancelled"
		result.Err = context.Canceled
	}
	return result
}
