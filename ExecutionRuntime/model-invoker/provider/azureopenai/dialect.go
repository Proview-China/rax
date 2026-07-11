package azureopenai

import (
	"encoding/json"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"net/http"
	"strings"
)

type dialect struct {
	deployment string
	legacy     bool
}

func (d dialect) ValidateRequest(r modelinvoker.Request) error {
	if r.Model != d.deployment {
		return mappingError("validate", "request model must equal the configured Azure deployment name")
	}
	if d.legacy && r.State != nil {
		return mappingError("validate", "legacy Azure Chat does not support Responses continuation state")
	}
	for namespace, raw := range r.ProviderOptions {
		var object map[string]json.RawMessage
		if namespace != ProviderID || json.Unmarshal(raw, &object) != nil || len(object) != 0 {
			return mappingError("validate", "Azure provider options must use its namespace and be empty")
		}
	}
	return nil
}
func (dialect) ClassifyFailure(f protocol.Failure) protocol.ErrorClassification {
	kind, retry := modelinvoker.ErrorProvider, false
	switch f.HTTPStatus {
	case 401:
		kind = modelinvoker.ErrorAuthentication
	case 403:
		kind = modelinvoker.ErrorPermission
	case 404:
		kind = modelinvoker.ErrorPermission
	case 429:
		kind, retry = modelinvoker.ErrorRateLimit, true
	case 500, 502, 503, 504:
		kind, retry = modelinvoker.ErrorProviderUnavailable, true
	}
	return protocol.ErrorClassification{Kind: kind, Code: f.Code, Message: "Azure OpenAI operation failed", Retryable: retry, RetryAfter: f.RetryAfter}
}
func (dialect) ProviderMetadata(h http.Header) modelinvoker.ProviderMetadata {
	m := make(modelinvoker.ProviderMetadata)
	for k, v := range h {
		lower := strings.ToLower(k)
		if lower == "x-request-id" || lower == "apim-request-id" || strings.HasPrefix(lower, "x-ratelimit-") {
			m[lower] = strings.Join(v, ",")
		}
	}
	return m
}
func mappingError(op, msg string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: modelinvoker.ErrorMapping, Provider: ProviderID, Operation: op, Message: msg}
}

var _ protocol.Dialect = dialect{}
