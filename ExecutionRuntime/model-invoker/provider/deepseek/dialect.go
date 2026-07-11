package deepseek

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func supportedModel(model string) bool {
	return model == "deepseek-v4-flash" || model == "deepseek-v4-pro"
}

type chatDialect struct{}

func (chatDialect) ValidateRequest(request modelinvoker.Request) error {
	if !supportedModel(request.Model) {
		return providerError(modelinvoker.ErrorMapping, "validate", "model must be deepseek-v4-flash or deepseek-v4-pro")
	}
	if request.State != nil {
		return providerError(modelinvoker.ErrorMapping, "validate", "Chat Completions does not support continuation state")
	}
	if request.Output.Type == modelinvoker.OutputJSONSchema {
		return providerError(modelinvoker.ErrorMapping, "validate", "DeepSeek Chat does not declare strict JSON Schema support")
	}
	if request.Reasoning != nil && request.Reasoning.Effort == modelinvoker.ReasoningEffortMinimal {
		return providerError(modelinvoker.ErrorMapping, "validate", "DeepSeek does not document minimal reasoning effort")
	}
	return validateOptions(request)
}

func (chatDialect) MapChatRequest(request modelinvoker.Request, params openaisdk.ChatCompletionNewParams) (openaisdk.ChatCompletionNewParams, []modelinvoker.MappingDecision, error) {
	decisions := []modelinvoker.MappingDecision{}
	if request.Reasoning == nil {
		return params, decisions, nil
	}
	thinking := "enabled"
	switch request.Reasoning.Effort {
	case modelinvoker.ReasoningEffortNone:
		thinking = "disabled"
		params.ReasoningEffort = ""
	case modelinvoker.ReasoningEffortLow, modelinvoker.ReasoningEffortMedium:
		params.ReasoningEffort = shared.ReasoningEffortHigh
		decisions = append(decisions, degraded("DeepSeek maps low/medium reasoning effort to high"))
	case modelinvoker.ReasoningEffortXHigh:
		params.ReasoningEffort = shared.ReasoningEffort("max")
		decisions = append(decisions, degraded("DeepSeek maps xhigh reasoning effort to max"))
	case modelinvoker.ReasoningEffortHigh, modelinvoker.ReasoningEffortMax, "":
	default:
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_reasoning", "unsupported DeepSeek reasoning effort")
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_reasoning", "failed to encode Chat request")
	}
	var object map[string]any
	if json.Unmarshal(raw, &object) != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_reasoning", "failed to inspect Chat request")
	}
	object["thinking"] = map[string]string{"type": thinking}
	raw, err = json.Marshal(object)
	if err != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_reasoning", "failed to encode DeepSeek thinking")
	}
	param.SetJSON(raw, &params)
	return params, decisions, nil
}

func (chatDialect) MapChatResponse(_ modelinvoker.Request, native *openaisdk.ChatCompletion, result *modelinvoker.Response) error {
	if native == nil || result == nil || len(native.Choices) == 0 {
		return nil
	}
	var extension struct {
		Reasoning string `json:"reasoning_content"`
	}
	if raw := native.Choices[0].Message.RawJSON(); raw != "" && json.Unmarshal([]byte(raw), &extension) != nil {
		return providerError(modelinvoker.ErrorMapping, "normalize_reasoning", "malformed DeepSeek reasoning_content")
	}
	if extension.Reasoning != "" {
		result.Output = append(result.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemReasoningSummary, ReasoningSummary: extension.Reasoning})
	}
	return nil
}

func (chatDialect) MapChatChunk(_ modelinvoker.Request, choice openaisdk.ChatCompletionChunkChoice) (string, []modelinvoker.MappingDecision, error) {
	var extension struct {
		Reasoning string `json:"reasoning_content"`
	}
	if raw := choice.Delta.RawJSON(); raw != "" && json.Unmarshal([]byte(raw), &extension) != nil {
		return "", nil, providerError(modelinvoker.ErrorMapping, "stream_reasoning", "malformed DeepSeek reasoning_content delta")
	}
	return extension.Reasoning, nil, nil
}

func (chatDialect) ClassifyFailure(f protocol.Failure) protocol.ErrorClassification {
	return classifyFailure(f)
}
func (chatDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	return metadata(headers)
}

type messagesDialect struct{}

func (messagesDialect) ValidateRequest(request modelinvoker.Request) error {
	if !supportedModel(request.Model) {
		return providerError(modelinvoker.ErrorMapping, "validate", "Messages model must be an exact current DeepSeek model ID")
	}
	if request.State != nil && request.State.Kind != modelinvoker.StateProviderContinuation {
		return providerError(modelinvoker.ErrorMapping, "validate", "Messages accepts only provider continuation state")
	}
	if request.Budget.MaxOutputTokens <= 0 {
		return providerError(modelinvoker.ErrorMapping, "validate", "Messages requires max output tokens greater than zero")
	}
	if request.Reasoning != nil && request.Reasoning.BudgetTokens != nil {
		return providerError(modelinvoker.ErrorMapping, "validate", "DeepSeek ignores Messages budget_tokens; the current binding rejects it")
	}
	for i, tool := range request.Tools {
		if !toolNamePattern.MatchString(tool.Name) {
			return providerError(modelinvoker.ErrorMapping, "validate", fmt.Sprintf("tool %d name is invalid", i))
		}
	}
	for _, item := range request.Input {
		if item.FunctionResult != nil && item.FunctionResult.IsError {
			return providerError(modelinvoker.ErrorMapping, "validate", "DeepSeek ignores Messages tool_result is_error; the current binding rejects it")
		}
	}
	return validateOptions(request)
}
func (messagesDialect) ClassifyFailure(f protocol.Failure) protocol.ErrorClassification {
	return classifyFailure(f)
}
func (messagesDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	return metadata(headers)
}

func validateOptions(request modelinvoker.Request) error {
	for namespace, raw := range request.ProviderOptions {
		var object map[string]json.RawMessage
		if namespace != ProviderID || json.Unmarshal(raw, &object) != nil || len(object) != 0 {
			return providerError(modelinvoker.ErrorMapping, "validate", "DeepSeek provider options must use its namespace and be empty")
		}
	}
	return nil
}

func classifyFailure(f protocol.Failure) protocol.ErrorClassification {
	kind, retry := modelinvoker.ErrorProvider, false
	switch f.HTTPStatus {
	case 400, 422:
		kind = modelinvoker.ErrorInvalidRequest
	case 401:
		kind = modelinvoker.ErrorAuthentication
	case 402:
		kind = modelinvoker.ErrorBilling
	case 403:
		kind = modelinvoker.ErrorPermission
	case 429:
		kind, retry = modelinvoker.ErrorRateLimit, true
	case 500, 502, 503, 504:
		kind, retry = modelinvoker.ErrorProviderUnavailable, true
	}
	if f.Source == protocol.FailureSourceTransport {
		kind, retry = modelinvoker.ErrorProviderUnavailable, true
	}
	if f.Source == protocol.FailureSourceProtocol {
		kind, retry = modelinvoker.ErrorProvider, false
	}
	code := f.Code
	if code == "" {
		code = f.Type
	}
	return protocol.ErrorClassification{Kind: kind, Code: code, Message: "DeepSeek operation failed", Retryable: retry, RetryAfter: f.RetryAfter}
}

func metadata(headers http.Header) modelinvoker.ProviderMetadata {
	result := make(modelinvoker.ProviderMetadata)
	for key, values := range headers {
		lower := strings.ToLower(key)
		if lower == "x-request-id" || lower == "request-id" || lower == "retry-after" || strings.HasPrefix(lower, "x-ratelimit-") {
			result[lower] = strings.Join(values, ",")
		}
	}
	return result
}

func degraded(detail string) modelinvoker.MappingDecision {
	return modelinvoker.MappingDecision{Capability: modelinvoker.CapabilityReasoning, Action: modelinvoker.MappingDegraded, Detail: detail}
}

var (
	_ protocol.Dialect          = chatDialect{}
	_ protocol.Dialect          = messagesDialect{}
	_ openaichat.RequestMapper  = chatDialect{}
	_ openaichat.ResponseMapper = chatDialect{}
	_ openaichat.StreamMapper   = chatDialect{}
)
