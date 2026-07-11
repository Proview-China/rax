package qwen

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openairesponses"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

var qwenToolName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func supportedModel(model string) bool {
	switch model {
	case "qwen3.7-max", "qwen3-max", "qwen3.6-plus", "qwen3.6-flash", "qwen-plus", "qwen-flash", "qwen3-coder-plus", "qwen3-coder-flash":
		return true
	default:
		return false
	}
}

type commonDialect struct{ protocol modelinvoker.Protocol }

func (d commonDialect) ValidateRequest(request modelinvoker.Request) error {
	if !supportedModel(request.Model) {
		return providerError(modelinvoker.ErrorMapping, "validate", "model must be in the approved Qwen text alias set")
	}
	if request.Protocol != d.protocol {
		return providerError(modelinvoker.ErrorInvalidRequest, "validate", "request protocol does not match the Qwen binding")
	}
	if len(request.Metadata) != 0 {
		return providerError(modelinvoker.ErrorMapping, "validate", "Qwen compatibility docs do not list portable metadata; refusing silent omission")
	}
	if request.ParallelToolCalls != nil {
		return providerError(modelinvoker.ErrorMapping, "validate", "Qwen parallel tool control is not approved in this slice")
	}
	for index, tool := range request.Tools {
		if !qwenToolName.MatchString(tool.Name) {
			return providerError(modelinvoker.ErrorMapping, "validate", fmt.Sprintf("tool %d name is invalid for Qwen", index))
		}
		if tool.Strict != nil {
			return providerError(modelinvoker.ErrorMapping, "validate", "Qwen compatibility docs do not list strict function schemas")
		}
	}
	if request.ToolChoice.Mode == modelinvoker.ToolChoiceRequired && len(request.Tools) != 1 {
		return providerError(modelinvoker.ErrorMapping, "validate", "Qwen required tool choice needs exactly one function")
	}
	if request.Reasoning != nil {
		if request.Reasoning.Summary != "" {
			return providerError(modelinvoker.ErrorMapping, "validate", "Qwen does not expose portable reasoning summary-style control")
		}
		if request.Protocol == modelinvoker.ProtocolResponses {
			if request.Reasoning.BudgetTokens != nil {
				return providerError(modelinvoker.ErrorMapping, "validate", "Qwen Responses does not expose portable thinking_budget")
			}
			switch request.Reasoning.Effort {
			case "", modelinvoker.ReasoningEffortNone, modelinvoker.ReasoningEffortMinimal, modelinvoker.ReasoningEffortMedium, modelinvoker.ReasoningEffortHigh:
			default:
				return providerError(modelinvoker.ErrorMapping, "validate", "Qwen Responses supports only none, minimal, medium, or high reasoning effort")
			}
		} else {
			switch request.Reasoning.Effort {
			case "", modelinvoker.ReasoningEffortNone, modelinvoker.ReasoningEffortMinimal, modelinvoker.ReasoningEffortLow, modelinvoker.ReasoningEffortMedium, modelinvoker.ReasoningEffortHigh:
			default:
				return providerError(modelinvoker.ErrorMapping, "validate", "Qwen Chat portable reasoning is an on/off intent, not an xhigh/max tier")
			}
			if request.Reasoning.BudgetTokens != nil && *request.Reasoning.BudgetTokens <= 0 {
				return providerError(modelinvoker.ErrorMapping, "validate", "Qwen thinking_budget must be positive")
			}
		}
	}
	if request.Protocol == modelinvoker.ProtocolResponses {
		if request.Output.Type != modelinvoker.OutputText {
			return providerError(modelinvoker.ErrorUnsupportedCapability, "validate", "Qwen Responses structured output is not approved in this slice")
		}
	} else {
		if request.State != nil {
			return providerError(modelinvoker.ErrorMapping, "validate", "Qwen Chat does not support server continuation state")
		}
		if request.Output.Type != modelinvoker.OutputText && request.Output.Type != modelinvoker.OutputJSONObject {
			return providerError(modelinvoker.ErrorUnsupportedCapability, "validate", "Qwen Chat supports text or JSON Object output in this slice")
		}
		if request.Output.Type == modelinvoker.OutputJSONObject && !requestMentionsJSON(request) {
			return providerError(modelinvoker.ErrorMapping, "validate", "Qwen JSON Object output requires an explicit JSON instruction")
		}
		if hasFunctionResult(request) && reasoningActive(request) {
			return providerError(modelinvoker.ErrorMapping, "validate", "Qwen thinking tool continuation requires complete reasoning_content history, which portable Chat input cannot preserve")
		}
	}
	for _, instruction := range request.Instructions {
		if instruction.Role == modelinvoker.RoleDeveloper {
			return providerError(modelinvoker.ErrorMapping, "validate", "Qwen compatibility docs do not list the developer role")
		}
	}
	for _, item := range request.Input {
		if item.Message != nil && item.Message.Role == modelinvoker.RoleDeveloper {
			return providerError(modelinvoker.ErrorMapping, "validate", "Qwen compatibility docs do not list the developer role")
		}
	}
	for namespace, raw := range request.ProviderOptions {
		var object map[string]json.RawMessage
		if namespace != ProviderID || json.Unmarshal(raw, &object) != nil || len(object) != 0 {
			return providerError(modelinvoker.ErrorMapping, "validate", "Qwen provider options must use its namespace and be empty")
		}
	}
	return nil
}

func requestMentionsJSON(request modelinvoker.Request) bool {
	for _, instruction := range request.Instructions {
		if strings.Contains(strings.ToLower(instruction.Text), "json") {
			return true
		}
	}
	for _, item := range request.Input {
		if item.Message != nil && strings.Contains(strings.ToLower(item.Message.Text), "json") {
			return true
		}
	}
	return false
}

func reasoningActive(request modelinvoker.Request) bool {
	return request.Reasoning == nil || request.Reasoning.Effort != modelinvoker.ReasoningEffortNone
}

func hasFunctionResult(request modelinvoker.Request) bool {
	for _, item := range request.Input {
		if item.Type == modelinvoker.InputTypeFunctionResult {
			return true
		}
	}
	return false
}

func (commonDialect) ClassifyFailure(failure protocol.Failure) protocol.ErrorClassification {
	kind, retryable := classifyHTTP(failure.HTTPStatus)
	code := failure.Code
	if code == "" {
		code = failure.Type
	}
	if codeKind, codeRetry, ok := classifyCode(code); ok {
		kind, retryable = codeKind, codeRetry
	}
	if failure.Source == protocol.FailureSourceTransport {
		kind, retryable = modelinvoker.ErrorProviderUnavailable, true
	}
	if failure.Source == protocol.FailureSourceProtocol {
		kind, retryable = modelinvoker.ErrorProvider, false
	}
	return protocol.ErrorClassification{Kind: kind, Code: code, Message: "Alibaba Model Studio operation failed", Retryable: retryable, RetryAfter: failure.RetryAfter}
}

func classifyHTTP(status int) (modelinvoker.ErrorKind, bool) {
	switch status {
	case 400:
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
	case lower == "arrearage" || lower == "allocationquota.freetieronly":
		return modelinvoker.ErrorBilling, false, true
	case lower == "invalidapikey" || lower == "invalid_api_key":
		return modelinvoker.ErrorAuthentication, false, true
	case lower == "accessdenied" || lower == "access_denied" || strings.Contains(lower, "model.accessdenied"):
		return modelinvoker.ErrorPermission, false, true
	case lower == "modelnotfound" || lower == "model_not_found" || lower == "model_not_supported" || lower == "workspacenotfound":
		return modelinvoker.ErrorMapping, false, true
	case strings.HasPrefix(lower, "throttling") || lower == "limit_requests" || lower == "limit_burst_rate" || lower == "insufficient_quota":
		return modelinvoker.ErrorRateLimit, true, true
	default:
		return "", false, false
	}
}

func (commonDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	result := make(modelinvoker.ProviderMetadata)
	for key, values := range headers {
		lower := strings.ToLower(key)
		if lower == "x-request-id" || lower == "request-id" || lower == "x-dashscope-request-id" || lower == "retry-after" || strings.HasPrefix(lower, "x-ratelimit-") {
			result[lower] = strings.Join(values, ",")
		}
	}
	return result
}

type responsesDialect struct{ commonDialect }

func (responsesDialect) ValidateRequest(request modelinvoker.Request) error {
	return commonDialect{protocol: modelinvoker.ProtocolResponses}.ValidateRequest(request)
}
func (responsesDialect) ClassifyFailure(f protocol.Failure) protocol.ErrorClassification {
	return commonDialect{}.ClassifyFailure(f)
}
func (responsesDialect) ProviderMetadata(h http.Header) modelinvoker.ProviderMetadata {
	return commonDialect{}.ProviderMetadata(h)
}
func (responsesDialect) MapResponsesRequest(request modelinvoker.Request, params responses.ResponseNewParams) (responses.ResponseNewParams, []modelinvoker.MappingDecision, error) {
	return params, nil, nil
}

type chatDialect struct{ commonDialect }

func (chatDialect) ValidateRequest(request modelinvoker.Request) error {
	return commonDialect{protocol: modelinvoker.ProtocolChatCompletions}.ValidateRequest(request)
}
func (chatDialect) ClassifyFailure(f protocol.Failure) protocol.ErrorClassification {
	return commonDialect{}.ClassifyFailure(f)
}
func (chatDialect) ProviderMetadata(h http.Header) modelinvoker.ProviderMetadata {
	return commonDialect{}.ProviderMetadata(h)
}
func (chatDialect) MapChatRequest(request modelinvoker.Request, params openaisdk.ChatCompletionNewParams) (openaisdk.ChatCompletionNewParams, []modelinvoker.MappingDecision, error) {
	if request.Reasoning == nil {
		return params, nil, nil
	}
	params.ReasoningEffort = ""
	raw, err := json.Marshal(params)
	if err != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_chat", "failed to encode Qwen Chat request")
	}
	var object map[string]any
	if json.Unmarshal(raw, &object) != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_chat", "failed to inspect Qwen Chat request")
	}
	delete(object, "reasoning_effort")
	enabled := request.Reasoning.Effort != modelinvoker.ReasoningEffortNone
	object["enable_thinking"] = enabled
	if request.Reasoning.BudgetTokens != nil {
		object["thinking_budget"] = *request.Reasoning.BudgetTokens
	}
	raw, err = json.Marshal(object)
	if err != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_chat", "failed to encode Qwen thinking controls")
	}
	param.SetJSON(raw, &params)
	detail := "portable reasoning intent mapped to Qwen enable_thinking=" + fmt.Sprint(enabled)
	return params, []modelinvoker.MappingDecision{{Capability: modelinvoker.CapabilityReasoning, Action: modelinvoker.MappingTransformed, Detail: detail}}, nil
}

func (chatDialect) MapChatResponse(_ modelinvoker.Request, native *openaisdk.ChatCompletion, result *modelinvoker.Response) error {
	if native == nil || result == nil || len(native.Choices) == 0 {
		return nil
	}
	reasoning, err := chatReasoning(native.Choices[0].Message.RawJSON())
	if err != nil {
		return providerError(modelinvoker.ErrorMapping, "normalize_reasoning", "malformed Qwen reasoning_content")
	}
	if reasoning != "" {
		result.Output = append(result.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemReasoningSummary, ReasoningSummary: reasoning})
	}
	return nil
}

func (chatDialect) MapChatChunk(_ modelinvoker.Request, choice openaisdk.ChatCompletionChunkChoice) (string, []modelinvoker.MappingDecision, error) {
	reasoning, err := chatReasoning(choice.Delta.RawJSON())
	if err != nil {
		return "", nil, providerError(modelinvoker.ErrorMapping, "stream_reasoning", "malformed Qwen reasoning_content delta")
	}
	return reasoning, nil, nil
}

func chatReasoning(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	var envelope struct {
		Reasoning string `json:"reasoning_content"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return "", err
	}
	return envelope.Reasoning, nil
}

var (
	_ protocol.Dialect              = responsesDialect{}
	_ openairesponses.RequestMapper = responsesDialect{}
	_ protocol.Dialect              = chatDialect{}
	_ openaichat.RequestMapper      = chatDialect{}
	_ openaichat.ResponseMapper     = chatDialect{}
	_ openaichat.StreamMapper       = chatDialect{}
)
