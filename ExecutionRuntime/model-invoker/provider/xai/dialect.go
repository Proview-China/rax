package xai

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openairesponses"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

type responsesDialect struct{}

func (responsesDialect) ValidateRequest(request modelinvoker.Request) error {
	if request.Model != "grok-4.5" {
		return providerError(modelinvoker.ErrorMapping, "validate", "model must be the approved grok-4.5 ID")
	}
	if request.Protocol != modelinvoker.ProtocolResponses {
		return providerError(modelinvoker.ErrorInvalidRequest, "validate", "only the xAI Responses protocol is approved")
	}
	if len(request.Metadata) != 0 {
		return providerError(modelinvoker.ErrorMapping, "validate", "xAI metadata is compatibility-only in the approved slice")
	}
	if request.Output.Type != modelinvoker.OutputText {
		return providerError(modelinvoker.ErrorUnsupportedCapability, "validate", "structured output is not approved in the xAI slice")
	}
	if len(request.Tools) > 128 {
		return providerError(modelinvoker.ErrorMapping, "validate", "xAI Responses accepts at most 128 tools")
	}
	for _, tool := range request.Tools {
		if tool.Strict != nil {
			return providerError(modelinvoker.ErrorMapping, "validate", "xAI does not document portable strict function schemas in this slice")
		}
	}
	if request.Reasoning != nil {
		if request.Reasoning.Summary != "" || request.Reasoning.BudgetTokens != nil {
			return providerError(modelinvoker.ErrorMapping, "validate", "xAI does not expose portable reasoning summary or budget controls")
		}
		switch request.Reasoning.Effort {
		case modelinvoker.ReasoningEffortLow, modelinvoker.ReasoningEffortMedium, modelinvoker.ReasoningEffortHigh:
		default:
			return providerError(modelinvoker.ErrorMapping, "validate", "grok-4.5 reasoning must be low, medium, or high and cannot be disabled")
		}
	}
	for namespace, raw := range request.ProviderOptions {
		if namespace != ProviderID {
			return providerError(modelinvoker.ErrorMapping, "validate", "xAI provider options must use the xai namespace")
		}
		var options struct {
			PromptCacheKey string `json:"prompt_cache_key"`
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &options) != nil || json.Unmarshal(raw, &fields) != nil || len(fields) != 1 {
			return providerError(modelinvoker.ErrorMapping, "validate", "xAI provider options must contain only prompt_cache_key")
		}
		if _, ok := fields["prompt_cache_key"]; !ok || !validPromptCacheKey(options.PromptCacheKey) {
			return providerError(modelinvoker.ErrorMapping, "validate", "xAI prompt_cache_key must be 1-256 printable ASCII bytes")
		}
	}
	return nil
}

func validPromptCacheKey(value string) bool {
	if len(value) == 0 || len(value) > 256 {
		return false
	}
	for _, character := range value {
		if character > unicode.MaxASCII || unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func (responsesDialect) ClassifyFailure(failure protocol.Failure) protocol.ErrorClassification {
	kind, retryable := classifyHTTP(failure.HTTPStatus)
	code := failure.Code
	if code == "" {
		code = failure.Type
	}
	if codeKind, codeRetryable, ok := classifyCode(code); ok {
		kind, retryable = codeKind, codeRetryable
	}
	if failure.Source == protocol.FailureSourceTransport {
		kind, retryable = modelinvoker.ErrorProviderUnavailable, true
	}
	if failure.Source == protocol.FailureSourceProtocol {
		kind, retryable = modelinvoker.ErrorProvider, false
	}
	return protocol.ErrorClassification{Kind: kind, Code: code, Message: "xAI operation failed", Retryable: retryable, RetryAfter: failure.RetryAfter}
}

func classifyHTTP(status int) (modelinvoker.ErrorKind, bool) {
	switch status {
	case 400, 405, 415, 422:
		return modelinvoker.ErrorInvalidRequest, false
	case 401:
		return modelinvoker.ErrorAuthentication, false
	case 403:
		return modelinvoker.ErrorPermission, false
	case 404:
		return modelinvoker.ErrorMapping, false
	case 429:
		return modelinvoker.ErrorRateLimit, true
	case 500, 502, 503, 504:
		return modelinvoker.ErrorProviderUnavailable, true
	default:
		return modelinvoker.ErrorProvider, false
	}
}

func classifyCode(code string) (modelinvoker.ErrorKind, bool, bool) {
	lower := strings.ToLower(strings.TrimSpace(code))
	switch {
	case lower == "invalid_api_key" || lower == "authentication_error" || lower == "unauthorized":
		return modelinvoker.ErrorAuthentication, false, true
	case lower == "permission_denied" || lower == "forbidden":
		return modelinvoker.ErrorPermission, false, true
	case lower == "model_not_found" || lower == "not_found" || lower == "previous_response_not_found":
		return modelinvoker.ErrorMapping, false, true
	case lower == "insufficient_quota" || lower == "billing_error":
		return modelinvoker.ErrorBilling, false, true
	case lower == "rate_limit_exceeded" || lower == "too_many_requests":
		return modelinvoker.ErrorRateLimit, true, true
	case lower == "server_error" || lower == "service_unavailable":
		return modelinvoker.ErrorProviderUnavailable, true, true
	default:
		return "", false, false
	}
}

func (responsesDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	result := make(modelinvoker.ProviderMetadata)
	for key, values := range headers {
		lower := strings.ToLower(key)
		if lower == "x-request-id" || lower == "request-id" || lower == "retry-after" || strings.HasPrefix(lower, "x-ratelimit-") {
			result[lower] = strings.Join(values, ",")
		}
	}
	return result
}

func (responsesDialect) MapResponsesRequest(request modelinvoker.Request, params responses.ResponseNewParams) (responses.ResponseNewParams, []modelinvoker.MappingDecision, error) {
	if raw, ok := request.ProviderOptions[ProviderID]; ok {
		var options struct {
			PromptCacheKey string `json:"prompt_cache_key"`
		}
		if err := json.Unmarshal(raw, &options); err != nil {
			return params, nil, providerError(modelinvoker.ErrorMapping, "map_responses", "failed to decode xAI provider options")
		}
		params.PromptCacheKey = openaisdk.String(options.PromptCacheKey)
		return params, []modelinvoker.MappingDecision{{Capability: modelinvoker.CapabilityPromptCaching, Action: modelinvoker.MappingExact, Detail: "xAI prompt_cache_key preserved"}}, nil
	}
	return params, nil, nil
}

var (
	_ protocol.Dialect              = responsesDialect{}
	_ openairesponses.RequestMapper = responsesDialect{}
)
