package anthropic

import (
	"net/http"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func providerError(kind modelinvoker.ErrorKind, operation, message string, cause error) *modelinvoker.Error {
	return &modelinvoker.Error{
		Kind: kind, Provider: ProviderID, Operation: operation, Message: message, Err: cause,
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

func classifyAPIError(status int, errorType string) (modelinvoker.ErrorKind, bool) {
	switch strings.ToLower(errorType) {
	case "authentication_error":
		return modelinvoker.ErrorAuthentication, false
	case "permission_error", "not_found_error":
		return modelinvoker.ErrorPermission, false
	case "billing_error":
		return modelinvoker.ErrorBilling, false
	case "invalid_request_error", "request_too_large":
		return modelinvoker.ErrorInvalidRequest, false
	case "rate_limit_error":
		return modelinvoker.ErrorRateLimit, true
	case "timeout_error":
		return modelinvoker.ErrorTimeout, true
	case "overloaded_error", "api_error", "conflict_error":
		return modelinvoker.ErrorProviderUnavailable, true
	}

	switch status {
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
		return modelinvoker.ErrorInvalidRequest, false
	case http.StatusUnauthorized:
		return modelinvoker.ErrorAuthentication, false
	case http.StatusPaymentRequired:
		return modelinvoker.ErrorBilling, false
	case http.StatusForbidden, http.StatusNotFound:
		return modelinvoker.ErrorPermission, false
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return modelinvoker.ErrorTimeout, true
	case http.StatusConflict:
		return modelinvoker.ErrorProviderUnavailable, true
	case http.StatusTooManyRequests:
		return modelinvoker.ErrorRateLimit, true
	default:
		if status >= 500 {
			return modelinvoker.ErrorProviderUnavailable, true
		}
	}
	return modelinvoker.ErrorProvider, false
}
