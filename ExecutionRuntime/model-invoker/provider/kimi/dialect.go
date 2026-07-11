package kimi

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
)

type modelClass uint8

const (
	modelUnknown modelClass = iota
	modelK2
	modelMoonshotV1
)

var kimiToolName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)

func modelKind(model string) modelClass {
	switch model {
	case "kimi-k2.7-code", "kimi-k2.7-code-highspeed", "kimi-k2.6", "kimi-k2.5":
		return modelK2
	case "moonshot-v1-8k", "moonshot-v1-32k", "moonshot-v1-128k":
		return modelMoonshotV1
	default:
		return modelUnknown
	}
}
func isK27(model string) bool {
	return model == "kimi-k2.7-code" || model == "kimi-k2.7-code-highspeed"
}

type chatDialect struct{}

func (chatDialect) ValidateRequest(request modelinvoker.Request) error {
	kind := modelKind(request.Model)
	if kind == modelUnknown {
		return providerError(modelinvoker.ErrorMapping, "validate", "model must be a current approved Kimi Open Platform text model")
	}
	if request.State != nil {
		return providerError(modelinvoker.ErrorMapping, "validate", "Kimi Chat continuation state is not implemented")
	}
	if request.Output.Type == modelinvoker.OutputJSONSchema {
		return providerError(modelinvoker.ErrorMapping, "validate", "Kimi does not declare strict JSON Schema output in this binding")
	}
	if kind == modelMoonshotV1 && request.Reasoning != nil {
		return providerError(modelinvoker.ErrorUnsupportedCapability, "validate", "Moonshot V1 text models do not support the K2 reasoning contract")
	}
	if kind == modelMoonshotV1 && len(request.Tools) > 0 {
		return providerError(modelinvoker.ErrorUnsupportedCapability, "validate", "Moonshot V1 tool support is not declared by this binding")
	}
	if isK27(request.Model) && request.Reasoning != nil && request.Reasoning.Effort == modelinvoker.ReasoningEffortNone {
		return providerError(modelinvoker.ErrorMapping, "validate", "Kimi K2.7 Code thinking is always enabled and cannot be disabled")
	}
	thinkingEnabled := kind == modelK2 && (request.Reasoning == nil || request.Reasoning.Effort != modelinvoker.ReasoningEffortNone)
	if thinkingEnabled && hasFunctionResult(request) {
		return providerError(modelinvoker.ErrorMapping, "validate", "thinking tool continuation requires preserved reasoning_content, which is not yet representable")
	}
	for i, tool := range request.Tools {
		if !kimiToolName.MatchString(tool.Name) {
			return providerError(modelinvoker.ErrorMapping, "validate", fmt.Sprintf("tool %d name is invalid for Kimi", i))
		}
	}
	for namespace, raw := range request.ProviderOptions {
		var object map[string]json.RawMessage
		if namespace != ProviderID || json.Unmarshal(raw, &object) != nil || len(object) != 0 {
			return providerError(modelinvoker.ErrorMapping, "validate", "Kimi provider options must use its namespace and be empty")
		}
	}
	return nil
}

func (chatDialect) MapChatRequest(request modelinvoker.Request, params openaisdk.ChatCompletionNewParams) (openaisdk.ChatCompletionNewParams, []modelinvoker.MappingDecision, error) {
	if request.Reasoning == nil {
		return params, nil, nil
	}
	params.ReasoningEffort = ""
	decision := modelinvoker.MappingDecision{Capability: modelinvoker.CapabilityReasoning, Action: modelinvoker.MappingTransformed}
	if isK27(request.Model) {
		decision.Detail = "Kimi K2.7 Code always enables thinking and does not accept an effort control"
		return params, []modelinvoker.MappingDecision{decision}, nil
	}
	if modelKind(request.Model) != modelK2 {
		return params, nil, nil
	}
	typeValue := "enabled"
	if request.Reasoning.Effort == modelinvoker.ReasoningEffortNone {
		typeValue = "disabled"
	}
	decision.Detail = "portable reasoning intent mapped to Kimi thinking.type=" + typeValue
	raw, err := json.Marshal(params)
	if err != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_reasoning", "failed to encode Kimi Chat request")
	}
	var object map[string]any
	if json.Unmarshal(raw, &object) != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_reasoning", "failed to inspect Kimi Chat request")
	}
	object["thinking"] = map[string]string{"type": typeValue}
	raw, err = json.Marshal(object)
	if err != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_reasoning", "failed to encode Kimi thinking")
	}
	param.SetJSON(raw, &params)
	return params, []modelinvoker.MappingDecision{decision}, nil
}

func (chatDialect) MapChatResponse(_ modelinvoker.Request, native *openaisdk.ChatCompletion, result *modelinvoker.Response) error {
	if native == nil || result == nil || len(native.Choices) == 0 {
		return nil
	}
	var extension struct {
		Reasoning string `json:"reasoning_content"`
	}
	if raw := native.Choices[0].Message.RawJSON(); raw != "" && json.Unmarshal([]byte(raw), &extension) != nil {
		return providerError(modelinvoker.ErrorMapping, "normalize_reasoning", "malformed Kimi reasoning_content")
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
		return "", nil, providerError(modelinvoker.ErrorMapping, "stream_reasoning", "malformed Kimi reasoning_content delta")
	}
	return extension.Reasoning, nil, nil
}

func (chatDialect) ClassifyFailure(f protocol.Failure) protocol.ErrorClassification {
	kind, retry := modelinvoker.ErrorProvider, false
	switch f.HTTPStatus {
	case 400:
		kind = modelinvoker.ErrorInvalidRequest
	case 401:
		kind = modelinvoker.ErrorAuthentication
	case 403, 404:
		kind = modelinvoker.ErrorPermission
	case 429:
		kind, retry = modelinvoker.ErrorRateLimit, true
	case 499:
		kind = modelinvoker.ErrorCancelled
	case 500, 503:
		kind, retry = modelinvoker.ErrorProviderUnavailable, true
	}
	if f.Type == "content_filter" {
		kind, retry = modelinvoker.ErrorPolicyRejected, false
	}
	if f.Type == "exceeded_current_quota_error" {
		kind, retry = modelinvoker.ErrorBilling, false
	}
	if f.Type == "engine_overloaded_error" || f.Type == "rate_limit_reached_error" {
		kind, retry = modelinvoker.ErrorRateLimit, true
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
	return protocol.ErrorClassification{Kind: kind, Code: code, Message: "Kimi Open Platform operation failed", Retryable: retry, RetryAfter: f.RetryAfter}
}

func (chatDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	result := make(modelinvoker.ProviderMetadata)
	for key, values := range headers {
		lower := strings.ToLower(key)
		if lower == "x-request-id" || lower == "request-id" || lower == "retry-after" || strings.HasPrefix(lower, "x-ratelimit-") {
			result[lower] = strings.Join(values, ",")
		}
	}
	return result
}

func hasFunctionResult(request modelinvoker.Request) bool {
	for _, item := range request.Input {
		if item.Type == modelinvoker.InputTypeFunctionResult {
			return true
		}
	}
	return false
}

var (
	_ protocol.Dialect          = chatDialect{}
	_ openaichat.RequestMapper  = chatDialect{}
	_ openaichat.ResponseMapper = chatDialect{}
	_ openaichat.StreamMapper   = chatDialect{}
)
