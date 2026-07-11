package zai

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

var zaiToolName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func supportedModel(model string) bool {
	switch model {
	case "glm-5.2", "glm-5.1", "glm-5-turbo", "glm-5", "glm-4.7", "glm-4.7-flash", "glm-4.7-flashx", "glm-4.6", "glm-4.5", "glm-4.5-air", "glm-4.5-x", "glm-4.5-airx", "glm-4.5-flash", "glm-4-32b-0414-128k":
		return true
	default:
		return false
	}
}
func thinkingModel(model string) bool { return supportedModel(model) && model != "glm-4-32b-0414-128k" }

type chatDialect struct{}

func (chatDialect) ValidateRequest(request modelinvoker.Request) error {
	if !supportedModel(request.Model) {
		return providerError(modelinvoker.ErrorMapping, "validate", "model must be a current approved Z.AI text model")
	}
	if request.State != nil {
		return providerError(modelinvoker.ErrorMapping, "validate", "Z.AI Chat continuation state is not implemented")
	}
	if request.Output.Type == modelinvoker.OutputJSONSchema {
		return providerError(modelinvoker.ErrorMapping, "validate", "Z.AI does not declare strict JSON Schema output in this binding")
	}
	if !thinkingModel(request.Model) && request.Reasoning != nil {
		return providerError(modelinvoker.ErrorUnsupportedCapability, "validate", "the selected GLM model does not support the current thinking contract")
	}
	if request.Reasoning != nil && request.Reasoning.Effort == modelinvoker.ReasoningEffortMinimal {
		return providerError(modelinvoker.ErrorMapping, "validate", "Z.AI minimal effort skips thinking and cannot represent portable minimal reasoning")
	}
	thinkingEnabled := thinkingModel(request.Model) && (request.Reasoning == nil || request.Reasoning.Effort != modelinvoker.ReasoningEffortNone)
	if thinkingEnabled && hasFunctionResult(request) {
		return providerError(modelinvoker.ErrorMapping, "validate", "thinking tool continuation requires preserved reasoning_content, which is not enabled in this binding")
	}
	if len(request.Tools) > 0 && request.ToolChoice.Mode != modelinvoker.ToolChoiceAuto {
		return providerError(modelinvoker.ErrorMapping, "validate", "Z.AI currently supports only tool_choice=auto")
	}
	for i, tool := range request.Tools {
		if !zaiToolName.MatchString(tool.Name) {
			return providerError(modelinvoker.ErrorMapping, "validate", fmt.Sprintf("tool %d name is invalid for Z.AI", i))
		}
	}
	for namespace, raw := range request.ProviderOptions {
		var object map[string]json.RawMessage
		if namespace != ProviderID || json.Unmarshal(raw, &object) != nil || len(object) != 0 {
			return providerError(modelinvoker.ErrorMapping, "validate", "Z.AI provider options must use its namespace and be empty")
		}
	}
	return nil
}

func (chatDialect) MapChatRequest(request modelinvoker.Request, params openaisdk.ChatCompletionNewParams) (openaisdk.ChatCompletionNewParams, []modelinvoker.MappingDecision, error) {
	if request.Reasoning == nil {
		return params, nil, nil
	}
	thinking := "enabled"
	decisions := []modelinvoker.MappingDecision{}
	if request.Reasoning.Effort == modelinvoker.ReasoningEffortNone {
		thinking = "disabled"
		params.ReasoningEffort = ""
	}
	if request.Model == "glm-5.2" && thinking == "enabled" {
		switch request.Reasoning.Effort {
		case modelinvoker.ReasoningEffortLow, modelinvoker.ReasoningEffortMedium:
			params.ReasoningEffort = shared.ReasoningEffortHigh
			decisions = append(decisions, transformed("Z.AI maps low/medium reasoning effort to high"))
		case modelinvoker.ReasoningEffortXHigh:
			params.ReasoningEffort = shared.ReasoningEffort("max")
			decisions = append(decisions, transformed("Z.AI maps xhigh reasoning effort to max"))
		case modelinvoker.ReasoningEffortHigh, modelinvoker.ReasoningEffortMax, "":
		default:
			return params, nil, providerError(modelinvoker.ErrorMapping, "map_reasoning", "unsupported GLM-5.2 reasoning effort")
		}
	} else if thinking == "enabled" {
		params.ReasoningEffort = ""
		decisions = append(decisions, transformed("portable reasoning intent mapped to Z.AI thinking.type=enabled without an effort tier"))
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_reasoning", "failed to encode Z.AI Chat request")
	}
	var object map[string]any
	if json.Unmarshal(raw, &object) != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_reasoning", "failed to inspect Z.AI Chat request")
	}
	object["thinking"] = map[string]any{"type": thinking, "clear_thinking": true}
	raw, err = json.Marshal(object)
	if err != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_reasoning", "failed to encode Z.AI thinking")
	}
	param.SetJSON(raw, &params)
	return params, decisions, nil
}

func (chatDialect) MapChatResponse(_ modelinvoker.Request, native *openaisdk.ChatCompletion, result *modelinvoker.Response) error {
	if native == nil || result == nil {
		return nil
	}
	var envelope struct {
		RequestID string `json:"request_id"`
	}
	if raw := native.RawJSON(); raw != "" && json.Unmarshal([]byte(raw), &envelope) != nil {
		return providerError(modelinvoker.ErrorMapping, "normalize_envelope", "malformed Z.AI response envelope")
	}
	if result.RequestID == "" {
		result.RequestID = envelope.RequestID
	}
	if len(native.Choices) == 0 {
		return nil
	}
	var extension struct {
		Reasoning string `json:"reasoning_content"`
	}
	if raw := native.Choices[0].Message.RawJSON(); raw != "" && json.Unmarshal([]byte(raw), &extension) != nil {
		return providerError(modelinvoker.ErrorMapping, "normalize_reasoning", "malformed Z.AI reasoning_content")
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
		return "", nil, providerError(modelinvoker.ErrorMapping, "stream_reasoning", "malformed Z.AI reasoning_content delta")
	}
	return extension.Reasoning, nil, nil
}

func (chatDialect) MapChatStreamMetadata(_ modelinvoker.Request, chunk openaisdk.ChatCompletionChunk) (openaichat.StreamMetadata, error) {
	var envelope struct {
		RequestID string `json:"request_id"`
	}
	if raw := chunk.RawJSON(); raw != "" && json.Unmarshal([]byte(raw), &envelope) != nil {
		return openaichat.StreamMetadata{}, providerError(modelinvoker.ErrorMapping, "stream_envelope", "malformed Z.AI stream envelope")
	}
	return openaichat.StreamMetadata{RequestID: envelope.RequestID}, nil
}

func (chatDialect) MapChatFinishReason(_ modelinvoker.Request, reason string) (openaichat.FinishReasonMapping, bool) {
	switch reason {
	case "sensitive":
		return openaichat.FinishReasonMapping{Error: providerError(modelinvoker.ErrorPolicyRejected, "finish", "Z.AI rejected sensitive content")}, true
	case "model_context_window_exceeded":
		return openaichat.FinishReasonMapping{Status: modelinvoker.ResponseStatusIncomplete, StopReason: modelinvoker.StopReasonOther}, true
	case "network_error":
		return openaichat.FinishReasonMapping{Error: providerError(modelinvoker.ErrorProviderUnavailable, "finish", "Z.AI inference network failed")}, true
	default:
		return openaichat.FinishReasonMapping{}, false
	}
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
	case 500, 502, 503, 504:
		kind, retry = modelinvoker.ErrorProviderUnavailable, true
	}
	switch f.Code {
	case "1000", "1001", "1003", "1005":
		kind, retry = modelinvoker.ErrorAuthentication, false
	case "1113":
		kind, retry = modelinvoker.ErrorBilling, false
	case "1210", "1213", "1214", "1215", "1261":
		kind, retry = modelinvoker.ErrorInvalidRequest, false
	case "1211", "1212", "1220", "1221", "1222":
		kind, retry = modelinvoker.ErrorPermission, false
	case "1301":
		kind, retry = modelinvoker.ErrorPolicyRejected, false
	case "1302":
		kind, retry = modelinvoker.ErrorRateLimit, true
	case "1305":
		kind, retry = modelinvoker.ErrorProviderUnavailable, true
	case "1308", "1309", "1310", "1311", "1313", "1314", "1315", "1316", "1317", "1318", "1319", "1320", "1321":
		kind, retry = modelinvoker.ErrorBilling, false
	}
	if f.Source == protocol.FailureSourceTransport {
		kind, retry = modelinvoker.ErrorProviderUnavailable, true
	}
	if f.Source == protocol.FailureSourceProtocol {
		kind, retry = modelinvoker.ErrorProvider, false
	}
	return protocol.ErrorClassification{Kind: kind, Code: f.Code, Message: "Z.AI operation failed", Retryable: retry, RetryAfter: f.RetryAfter}
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
func transformed(detail string) modelinvoker.MappingDecision {
	return modelinvoker.MappingDecision{Capability: modelinvoker.CapabilityReasoning, Action: modelinvoker.MappingTransformed, Detail: detail}
}

var (
	_ protocol.Dialect                = chatDialect{}
	_ openaichat.RequestMapper        = chatDialect{}
	_ openaichat.ResponseMapper       = chatDialect{}
	_ openaichat.StreamMapper         = chatDialect{}
	_ openaichat.StreamMetadataMapper = chatDialect{}
	_ openaichat.FinishReasonMapper   = chatDialect{}
)
