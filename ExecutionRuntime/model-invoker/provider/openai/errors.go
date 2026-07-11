package openai

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

func classifyAPIError(status int, errorType, code string) (modelinvoker.ErrorKind, bool) {
	marker := strings.ToLower(errorType + " " + code)
	switch status {
	case http.StatusUnauthorized:
		return modelinvoker.ErrorAuthentication, false
	case http.StatusForbidden, http.StatusNotFound:
		return modelinvoker.ErrorPermission, false
	case http.StatusRequestTimeout:
		return modelinvoker.ErrorTimeout, true
	case http.StatusConflict, http.StatusTooEarly:
		return modelinvoker.ErrorProviderUnavailable, true
	case http.StatusTooManyRequests:
		return modelinvoker.ErrorRateLimit, true
	default:
		if status >= 500 {
			return modelinvoker.ErrorProviderUnavailable, true
		}
	}
	if strings.Contains(marker, "content_policy") || strings.Contains(marker, "safety") || strings.Contains(marker, "moderation") {
		return modelinvoker.ErrorPolicyRejected, false
	}
	if status == http.StatusBadRequest || status == http.StatusUnprocessableEntity || strings.Contains(marker, "invalid") {
		return modelinvoker.ErrorInvalidRequest, false
	}
	if strings.Contains(marker, "rate_limit") {
		return modelinvoker.ErrorRateLimit, true
	}
	if strings.Contains(marker, "timeout") {
		return modelinvoker.ErrorTimeout, true
	}
	if strings.Contains(marker, "server_error") {
		return modelinvoker.ErrorProviderUnavailable, true
	}
	return modelinvoker.ErrorProvider, false
}
