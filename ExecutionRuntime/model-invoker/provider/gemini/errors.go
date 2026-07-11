package gemini

import (
	"net/http"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func providerError(kind modelinvoker.ErrorKind, operation, message string, cause error) *modelinvoker.Error {
	return &modelinvoker.Error{
		Kind:      kind,
		Provider:  ProviderID,
		Operation: operation,
		Message:   message,
		Err:       cause,
	}
}

func mappingError(operation, message string) *modelinvoker.Error {
	return providerError(modelinvoker.ErrorMapping, operation, message, nil)
}

func mappingErrorWithRequestID(operation, message, id string) *modelinvoker.Error {
	err := mappingError(operation, message)
	err.RequestID = id
	return err
}

func classifyAPIError(status int, providerStatus string) (modelinvoker.ErrorKind, bool) {
	switch strings.ToUpper(providerStatus) {
	case "UNAUTHENTICATED":
		return modelinvoker.ErrorAuthentication, false
	case "PERMISSION_DENIED", "FAILED_PRECONDITION", "NOT_FOUND":
		return modelinvoker.ErrorPermission, false
	case "INVALID_ARGUMENT", "OUT_OF_RANGE":
		return modelinvoker.ErrorInvalidRequest, false
	case "RESOURCE_EXHAUSTED":
		return modelinvoker.ErrorRateLimit, true
	case "CANCELLED":
		return modelinvoker.ErrorCancelled, false
	case "DEADLINE_EXCEEDED":
		return modelinvoker.ErrorTimeout, true
	case "INTERNAL", "UNAVAILABLE", "ABORTED":
		return modelinvoker.ErrorProviderUnavailable, true
	}
	switch status {
	case http.StatusPaymentRequired:
		return modelinvoker.ErrorBilling, false
	case http.StatusUnauthorized:
		return modelinvoker.ErrorAuthentication, false
	case http.StatusForbidden, http.StatusNotFound:
		return modelinvoker.ErrorPermission, false
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return modelinvoker.ErrorInvalidRequest, false
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return modelinvoker.ErrorTimeout, true
	case http.StatusTooManyRequests:
		return modelinvoker.ErrorRateLimit, true
	case 499:
		return modelinvoker.ErrorCancelled, false
	default:
		if status >= 500 {
			return modelinvoker.ErrorProviderUnavailable, true
		}
	}
	return modelinvoker.ErrorProvider, false
}
