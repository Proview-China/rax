package mimo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/anthropicmessages"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	anthropicparam "github.com/anthropics/anthropic-sdk-go/packages/param"
	openaisdk "github.com/openai/openai-go/v3"
	openaiparam "github.com/openai/openai-go/v3/packages/param"
)

func supportedModel(model string) bool {
	return model == "mimo-v2.5-pro" || model == "mimo-v2.5"
}

type commonDialect struct{ protocol modelinvoker.Protocol }

func (d commonDialect) ValidateRequest(request modelinvoker.Request) error {
	if !supportedModel(request.Model) {
		return providerError(modelinvoker.ErrorMapping, "validate", "model must be a current approved MiMo V2.5 text model")
	}
	if request.Protocol != d.protocol {
		return providerError(modelinvoker.ErrorInvalidRequest, "validate", "request protocol does not match the MiMo binding")
	}
	if request.ToolChoice.Mode != modelinvoker.ToolChoiceAuto {
		return providerError(modelinvoker.ErrorMapping, "validate", "MiMo only supports automatic tool selection")
	}
	if request.Protocol == modelinvoker.ProtocolMessages {
		if request.Output.Type != modelinvoker.OutputText {
			return providerError(modelinvoker.ErrorUnsupportedCapability, "validate", "MiMo Messages supports text output only")
		}
		for _, tool := range request.Tools {
			if tool.Strict != nil {
				return providerError(modelinvoker.ErrorMapping, "validate", "MiMo Messages does not document strict tool schemas")
			}
		}
	} else {
		if request.Output.Type != modelinvoker.OutputText && request.Output.Type != modelinvoker.OutputJSONObject {
			return providerError(modelinvoker.ErrorUnsupportedCapability, "validate", "MiMo Chat supports text or JSON Object output, not strict JSON Schema output")
		}
		if request.ParallelToolCalls != nil {
			return providerError(modelinvoker.ErrorMapping, "validate", "MiMo Chat does not expose portable parallel-tool control")
		}
		if request.State != nil {
			return providerError(modelinvoker.ErrorMapping, "validate", "MiMo Chat continuation state is not implemented")
		}
		if reasoningActive(request) && hasFunctionResult(request) {
			return providerError(modelinvoker.ErrorMapping, "validate", "MiMo thinking tool continuation requires complete reasoning_content history, which portable Chat input cannot preserve")
		}
	}
	if request.Reasoning != nil {
		if request.Reasoning.BudgetTokens != nil || request.Reasoning.Summary != "" {
			return providerError(modelinvoker.ErrorMapping, "validate", "MiMo exposes a thinking switch but no portable budget or summary-style control")
		}
		switch request.Reasoning.Effort {
		case "", modelinvoker.ReasoningEffortNone, modelinvoker.ReasoningEffortMinimal, modelinvoker.ReasoningEffortLow, modelinvoker.ReasoningEffortMedium, modelinvoker.ReasoningEffortHigh:
		default:
			return providerError(modelinvoker.ErrorMapping, "validate", "MiMo supports portable reasoning intent only as an enabled or disabled switch")
		}
	}
	for namespace, raw := range request.ProviderOptions {
		var object map[string]json.RawMessage
		if namespace != ProviderID || json.Unmarshal(raw, &object) != nil || len(object) != 0 {
			return providerError(modelinvoker.ErrorMapping, "validate", "MiMo provider options must use its namespace and be empty")
		}
	}
	return nil
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
	kind, retryable := modelinvoker.ErrorProvider, false
	switch failure.HTTPStatus {
	case 400:
		kind = modelinvoker.ErrorInvalidRequest
	case 401:
		kind = modelinvoker.ErrorAuthentication
	case 402:
		kind = modelinvoker.ErrorBilling
	case 403, 404:
		kind = modelinvoker.ErrorPermission
	case 421:
		kind = modelinvoker.ErrorPolicyRejected
	case 429:
		kind, retryable = modelinvoker.ErrorRateLimit, true
	case 500, 503:
		kind, retryable = modelinvoker.ErrorProviderUnavailable, true
	}
	if failure.Source == protocol.FailureSourceTransport {
		kind, retryable = modelinvoker.ErrorProviderUnavailable, true
	}
	if failure.Source == protocol.FailureSourceProtocol {
		kind, retryable = modelinvoker.ErrorProvider, false
	}
	code := failure.Code
	if code == "" {
		code = failure.Type
	}
	return protocol.ErrorClassification{Kind: kind, Code: code, Message: "MiMo operation failed", Retryable: retryable, RetryAfter: failure.RetryAfter}
}

func (commonDialect) ProviderMetadata(headers http.Header) modelinvoker.ProviderMetadata {
	result := make(modelinvoker.ProviderMetadata)
	for key, values := range headers {
		lower := strings.ToLower(key)
		if lower == "x-request-id" || lower == "request-id" || lower == "retry-after" || strings.HasPrefix(lower, "x-ratelimit-") {
			result[lower] = strings.Join(values, ",")
		}
	}
	return result
}

type messagesDialect struct{ commonDialect }

func (messagesDialect) ValidateRequest(request modelinvoker.Request) error {
	return commonDialect{protocol: modelinvoker.ProtocolMessages}.ValidateRequest(request)
}
func (messagesDialect) ClassifyFailure(f protocol.Failure) protocol.ErrorClassification {
	return commonDialect{}.ClassifyFailure(f)
}
func (messagesDialect) ProviderMetadata(h http.Header) modelinvoker.ProviderMetadata {
	return commonDialect{}.ProviderMetadata(h)
}
func (messagesDialect) MapMessagesRequest(request modelinvoker.Request, params anthropicsdk.MessageNewParams) (anthropicsdk.MessageNewParams, []modelinvoker.MappingDecision, error) {
	for messageIndex := range params.Messages {
		for blockIndex := range params.Messages[messageIndex].Content {
			if tool := params.Messages[messageIndex].Content[blockIndex].OfToolUse; tool != nil {
				tool.Caller = anthropicsdk.ToolUseBlockParamCallerUnion{}
			}
		}
	}
	if request.Reasoning == nil {
		return params, nil, nil
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_messages", "failed to encode MiMo Messages request")
	}
	var object map[string]any
	if json.Unmarshal(raw, &object) != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_messages", "failed to inspect MiMo Messages request")
	}
	delete(object, "output_config")
	typeValue := "enabled"
	if request.Reasoning.Effort == modelinvoker.ReasoningEffortNone {
		typeValue = "disabled"
	}
	object["thinking"] = map[string]string{"type": typeValue}
	raw, err = json.Marshal(object)
	if err != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_messages", "failed to encode MiMo thinking")
	}
	anthropicparam.SetJSON(raw, &params)
	return params, []modelinvoker.MappingDecision{transformed("portable reasoning intent mapped to MiMo thinking.type=" + typeValue)}, nil
}

func (messagesDialect) MapMessagesContinuation(_ modelinvoker.Request, content []json.RawMessage) ([]json.RawMessage, error) {
	result := make([]json.RawMessage, len(content))
	for index, raw := range content {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			return nil, fmt.Errorf("content %d is not a JSON object", index)
		}
		var kind string
		_ = json.Unmarshal(fields["type"], &kind)
		if kind == "tool_use" {
			if _, exists := fields["caller"]; !exists {
				fields["caller"] = json.RawMessage(`{"type":"direct"}`)
			}
			mapped, err := json.Marshal(fields)
			if err != nil {
				return nil, fmt.Errorf("content %d could not be canonicalized", index)
			}
			result[index] = mapped
			continue
		}
		result[index] = append(json.RawMessage(nil), raw...)
	}
	return result, nil
}

func (messagesDialect) MapMessagesStopReason(_ modelinvoker.Request, reason string) (anthropicmessages.StopReasonMapping, bool) {
	switch reason {
	case "repetition_truncation":
		return anthropicmessages.StopReasonMapping{Status: modelinvoker.ResponseStatusIncomplete, StopReason: modelinvoker.StopReasonOther}, true
	case "content_filter":
		return anthropicmessages.StopReasonMapping{
			Status: modelinvoker.ResponseStatusFailed, StopReason: modelinvoker.StopReasonContentFilter,
			Error: providerError(modelinvoker.ErrorPolicyRejected, "messages.normalize", "content was rejected by MiMo policy"),
		}, true
	default:
		return anthropicmessages.StopReasonMapping{}, false
	}
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
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_chat", "failed to encode MiMo Chat request")
	}
	var object map[string]any
	if json.Unmarshal(raw, &object) != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_chat", "failed to inspect MiMo Chat request")
	}
	delete(object, "reasoning_effort")
	typeValue := "enabled"
	if request.Reasoning.Effort == modelinvoker.ReasoningEffortNone {
		typeValue = "disabled"
	}
	object["thinking"] = map[string]string{"type": typeValue}
	raw, err = json.Marshal(object)
	if err != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_chat", "failed to encode MiMo thinking")
	}
	openaiparam.SetJSON(raw, &params)
	return params, []modelinvoker.MappingDecision{transformed("portable reasoning intent mapped to MiMo thinking.type=" + typeValue)}, nil
}

func (chatDialect) MapChatResponse(_ modelinvoker.Request, native *openaisdk.ChatCompletion, result *modelinvoker.Response) error {
	if native == nil || result == nil || len(native.Choices) == 0 {
		return nil
	}
	reasoning, err := chatReasoning(native.Choices[0].Message.RawJSON())
	if err != nil {
		return providerError(modelinvoker.ErrorMapping, "normalize_reasoning", "malformed MiMo reasoning_content")
	}
	if reasoning != "" {
		result.Output = append(result.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemReasoningSummary, ReasoningSummary: reasoning})
	}
	return nil
}

func (chatDialect) MapChatChunk(_ modelinvoker.Request, choice openaisdk.ChatCompletionChunkChoice) (string, []modelinvoker.MappingDecision, error) {
	reasoning, err := chatReasoning(choice.Delta.RawJSON())
	if err != nil {
		return "", nil, providerError(modelinvoker.ErrorMapping, "stream_reasoning", "malformed MiMo reasoning_content delta")
	}
	return reasoning, nil, nil
}

func (chatDialect) MapChatFinishReason(_ modelinvoker.Request, reason string) (openaichat.FinishReasonMapping, bool) {
	if reason != "repetition_truncation" {
		return openaichat.FinishReasonMapping{}, false
	}
	return openaichat.FinishReasonMapping{Status: modelinvoker.ResponseStatusIncomplete, StopReason: modelinvoker.StopReasonOther}, true
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

func transformed(detail string) modelinvoker.MappingDecision {
	return modelinvoker.MappingDecision{Capability: modelinvoker.CapabilityReasoning, Action: modelinvoker.MappingTransformed, Detail: detail}
}

var (
	_ protocol.Dialect                     = messagesDialect{}
	_ anthropicmessages.RequestMapper      = messagesDialect{}
	_ anthropicmessages.ContinuationMapper = messagesDialect{}
	_ anthropicmessages.StopReasonMapper   = messagesDialect{}
	_ protocol.Dialect                     = chatDialect{}
	_ openaichat.RequestMapper             = chatDialect{}
	_ openaichat.ResponseMapper            = chatDialect{}
	_ openaichat.StreamMapper              = chatDialect{}
	_ openaichat.FinishReasonMapper        = chatDialect{}
)
