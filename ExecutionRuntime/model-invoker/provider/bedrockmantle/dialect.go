package bedrockmantle

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
)

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type cloudDialect struct {
	store    bool
	messages bool
}

func (d cloudDialect) ValidateRequest(request modelinvoker.Request) error {
	if request.Protocol == modelinvoker.ProtocolResponses && !d.store && request.State != nil {
		return mappingError("validate", "Bedrock Mantle store=false cannot use previous_response_id")
	}
	if d.messages && request.Budget.MaxOutputTokens <= 0 {
		return mappingError("validate", "Bedrock Mantle Messages requires max output tokens greater than zero")
	}
	for index, tool := range request.Tools {
		if !toolNamePattern.MatchString(tool.Name) {
			return mappingError("validate", fmt.Sprintf("tool %d name is invalid", index))
		}
	}
	for namespace, raw := range request.ProviderOptions {
		var object map[string]json.RawMessage
		if namespace != ProviderID || json.Unmarshal(raw, &object) != nil || len(object) != 0 {
			return mappingError("validate", "Bedrock Mantle provider options must use its namespace and be empty")
		}
	}
	return nil
}
func (cloudDialect) ClassifyFailure(failure protocol.Failure) protocol.ErrorClassification {
	kind, retry := modelinvoker.ErrorProvider, false
	switch failure.HTTPStatus {
	case 401:
		kind = modelinvoker.ErrorAuthentication
	case 403:
		kind = modelinvoker.ErrorPermission
	case 429:
		kind, retry = modelinvoker.ErrorRateLimit, true
	case 500, 502, 503, 504:
		kind, retry = modelinvoker.ErrorProviderUnavailable, true
	}
	return protocol.ErrorClassification{Kind: kind, Code: failure.Code, Message: "Bedrock Mantle operation failed", Retryable: retry, RetryAfter: failure.RetryAfter}
}
func (cloudDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	result := make(modelinvoker.ProviderMetadata)
	for key, values := range headers {
		lower := strings.ToLower(key)
		if lower == "x-amzn-requestid" || strings.HasPrefix(lower, "x-ratelimit-") || strings.HasPrefix(lower, "anthropic-ratelimit-") {
			result[lower] = strings.Join(values, ",")
		}
	}
	return result
}
func mappingError(operation, message string) *modelinvoker.Error {
	return &modelinvoker.Error{Kind: modelinvoker.ErrorMapping, Provider: ProviderID, Operation: operation, Message: message}
}

var _ protocol.Dialect = cloudDialect{}
