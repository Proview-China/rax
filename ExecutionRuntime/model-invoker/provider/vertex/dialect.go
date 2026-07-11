package vertex

import (
	"encoding/json"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"net/http"
	"strings"
)

type dialect struct{ messages bool }

func (d dialect) ValidateRequest(r modelinvoker.Request) error {
	if d.messages && r.Budget.MaxOutputTokens <= 0 {
		return mappingError("validate", "Vertex Claude requires max output tokens greater than zero")
	}
	for namespace, raw := range r.ProviderOptions {
		var object map[string]json.RawMessage
		if namespace != ProviderID || json.Unmarshal(raw, &object) != nil || len(object) != 0 {
			return mappingError("validate", "Vertex provider options must use its namespace and be empty")
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
	case 429:
		kind, retry = modelinvoker.ErrorRateLimit, true
	case 500, 502, 503, 504:
		kind, retry = modelinvoker.ErrorProviderUnavailable, true
	}
	return protocol.ErrorClassification{Kind: kind, Code: f.Code, Message: "Vertex operation failed", Retryable: retry, RetryAfter: f.RetryAfter}
}
func (dialect) ProviderMetadata(h http.Header) modelinvoker.ProviderMetadata {
	m := make(modelinvoker.ProviderMetadata)
	for k, v := range h {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "x-goog-") || lower == "x-request-id" {
			m[lower] = strings.Join(v, ",")
		}
	}
	return m
}
func mappingError(op, msg string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: modelinvoker.ErrorMapping, Provider: ProviderID, Operation: op, Message: msg}
}

var _ protocol.Dialect = dialect{}
