package minimax

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/anthropicmessages"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openairesponses"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	openaisdk "github.com/openai/openai-go/v3"
	openaiparam "github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

var minimaxToolName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func supportedModel(model string) bool {
	switch model {
	case "MiniMax-M3", "MiniMax-M2.7", "MiniMax-M2.7-highspeed", "MiniMax-M2.5", "MiniMax-M2.5-highspeed", "MiniMax-M2.1", "MiniMax-M2.1-highspeed", "MiniMax-M2":
		return true
	default:
		return false
	}
}

func isM3(model string) bool { return model == "MiniMax-M3" }

type commonDialect struct{ protocol modelinvoker.Protocol }

func (d commonDialect) ValidateRequest(request modelinvoker.Request) error {
	if !supportedModel(request.Model) {
		return providerError(modelinvoker.ErrorMapping, "validate", "model must be a current approved MiniMax text model")
	}
	if request.Protocol != d.protocol {
		return providerError(modelinvoker.ErrorInvalidRequest, "validate", "request protocol does not match the MiniMax binding")
	}
	if request.Output.Type != modelinvoker.OutputText {
		return providerError(modelinvoker.ErrorUnsupportedCapability, "validate", "MiniMax compatibility bindings currently support text output only")
	}
	if request.ParallelToolCalls != nil {
		return providerError(modelinvoker.ErrorMapping, "validate", "MiniMax parallel tool control is not implemented")
	}
	if len(request.Tools) > 128 {
		return providerError(modelinvoker.ErrorMapping, "validate", "MiniMax accepts at most 128 function tools in this slice")
	}
	for index, tool := range request.Tools {
		if !minimaxToolName.MatchString(tool.Name) {
			return providerError(modelinvoker.ErrorMapping, "validate", fmt.Sprintf("tool %d name is invalid for MiniMax", index))
		}
		if tool.Strict != nil {
			return providerError(modelinvoker.ErrorMapping, "validate", "MiniMax strict tool schemas are not approved in this slice")
		}
	}
	if request.ToolChoice.Mode != modelinvoker.ToolChoiceAuto && request.ToolChoice.Mode != modelinvoker.ToolChoiceNone {
		return providerError(modelinvoker.ErrorMapping, "validate", "MiniMax currently supports only auto or none tool choice")
	}
	if request.Reasoning != nil {
		if request.Reasoning.BudgetTokens != nil {
			return providerError(modelinvoker.ErrorMapping, "validate", "MiniMax does not expose a portable thinking token budget")
		}
		if request.Reasoning.Summary != "" {
			return providerError(modelinvoker.ErrorMapping, "validate", "MiniMax does not expose portable reasoning summary style control")
		}
		switch request.Reasoning.Effort {
		case "", modelinvoker.ReasoningEffortNone, modelinvoker.ReasoningEffortMinimal, modelinvoker.ReasoningEffortLow, modelinvoker.ReasoningEffortMedium, modelinvoker.ReasoningEffortHigh:
		default:
			return providerError(modelinvoker.ErrorMapping, "validate", "MiniMax supports none, minimal, low, medium, or high reasoning compatibility values")
		}
		if !isM3(request.Model) && request.Reasoning.Effort == modelinvoker.ReasoningEffortNone {
			return providerError(modelinvoker.ErrorUnsupportedCapability, "validate", "MiniMax M2.x thinking cannot be disabled")
		}
	}
	if request.Protocol == modelinvoker.ProtocolResponses && request.State != nil {
		return providerError(modelinvoker.ErrorMapping, "validate", "MiniMax Responses does not support server continuation state")
	}
	if request.Protocol != modelinvoker.ProtocolMessages && hasFunctionResult(request) && reasoningActive(request) {
		return providerError(modelinvoker.ErrorMapping, "validate", "MiniMax thinking tool continuation requires full reasoning history, which this OpenAI-compatible portable input cannot preserve")
	}
	for namespace, raw := range request.ProviderOptions {
		var object map[string]json.RawMessage
		if namespace != ProviderID || json.Unmarshal(raw, &object) != nil || len(object) != 0 {
			return providerError(modelinvoker.ErrorMapping, "validate", "MiniMax provider options must use its namespace and be empty")
		}
	}
	return nil
}

func reasoningActive(request modelinvoker.Request) bool {
	if !isM3(request.Model) {
		return true
	}
	return request.Reasoning != nil && request.Reasoning.Effort != modelinvoker.ReasoningEffortNone
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
	return protocol.ErrorClassification{Kind: kind, Code: code, Message: "MiniMax operation failed", Retryable: retryable, RetryAfter: failure.RetryAfter}
}

func classifyHTTP(status int) (modelinvoker.ErrorKind, bool) {
	switch status {
	case 400:
		return modelinvoker.ErrorInvalidRequest, false
	case 401:
		return modelinvoker.ErrorAuthentication, false
	case 402:
		return modelinvoker.ErrorBilling, false
	case 403, 404:
		return modelinvoker.ErrorPermission, false
	case 429:
		return modelinvoker.ErrorRateLimit, true
	case 500, 502, 503, 504:
		return modelinvoker.ErrorProviderUnavailable, true
	default:
		return modelinvoker.ErrorProvider, false
	}
}

func classifyCode(code string) (modelinvoker.ErrorKind, bool, bool) {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "1004", "2049", "authentication_error", "invalid_api_key":
		return modelinvoker.ErrorAuthentication, false, true
	case "1008", "insufficient_balance", "billing_error":
		return modelinvoker.ErrorBilling, false, true
	case "1002", "1041", "rate_limit_error", "rate_limit":
		return modelinvoker.ErrorRateLimit, true, true
	case "1000", "1001", "1024", "1033", "api_error", "overloaded_error":
		return modelinvoker.ErrorProviderUnavailable, true, true
	case "1026", "1027", "content_filter", "policy_error":
		return modelinvoker.ErrorPolicyRejected, false, true
	case "1039", "1042", "2013", "invalid_request_error":
		return modelinvoker.ErrorInvalidRequest, false, true
	case "permission_error", "not_found_error":
		return modelinvoker.ErrorPermission, false, true
	default:
		return "", false, false
	}
}

func providerCodeError(code string) error {
	kind, retryable, ok := classifyCode(code)
	if !ok {
		kind = modelinvoker.ErrorProvider
	}
	return &modelinvoker.Error{Kind: kind, Provider: ProviderID, Operation: "provider_response", Code: code, Message: "MiniMax operation failed", Retryable: retryable}
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
	params.OutputConfig.Effort = ""
	if isM3(request.Model) {
		if request.Reasoning.Effort == modelinvoker.ReasoningEffortNone {
			disabled := anthropicsdk.NewThinkingConfigDisabledParam()
			params.Thinking = anthropicsdk.ThinkingConfigParamUnion{OfDisabled: &disabled}
			return params, nil, nil
		}
		adaptive := anthropicsdk.ThinkingConfigAdaptiveParam{}
		params.Thinking = anthropicsdk.ThinkingConfigParamUnion{OfAdaptive: &adaptive}
		return params, []modelinvoker.MappingDecision{transformed("MiniMax M3 maps portable reasoning effort to an adaptive thinking on/off switch")}, nil
	}
	params.Thinking = anthropicsdk.ThinkingConfigParamUnion{}
	return params, []modelinvoker.MappingDecision{transformed("MiniMax M2.x always-on thinking ignores portable depth tiers")}, nil
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
	params.ReasoningEffort = ""
	raw, err := json.Marshal(params)
	if err != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_chat", "failed to encode MiniMax Chat request")
	}
	var object map[string]any
	if json.Unmarshal(raw, &object) != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_chat", "failed to inspect MiniMax Chat request")
	}
	delete(object, "reasoning_effort")
	object["reasoning_split"] = true
	decisions := []modelinvoker.MappingDecision{}
	if isM3(request.Model) {
		thinking := "disabled"
		if request.Reasoning != nil && request.Reasoning.Effort != modelinvoker.ReasoningEffortNone {
			thinking = "adaptive"
			decisions = append(decisions, transformed("MiniMax M3 maps portable reasoning effort to an adaptive thinking on/off switch"))
		}
		object["thinking"] = map[string]any{"type": thinking}
	} else if request.Reasoning != nil {
		decisions = append(decisions, transformed("MiniMax M2.x always-on thinking ignores portable depth tiers"))
	}
	raw, err = json.Marshal(object)
	if err != nil {
		return params, nil, providerError(modelinvoker.ErrorMapping, "map_chat", "failed to encode MiniMax Chat dialect")
	}
	openaiparam.SetJSON(raw, &params)
	return params, decisions, nil
}

type chatReasoningEnvelope struct {
	ReasoningContent string `json:"reasoning_content"`
	ReasoningDetails []struct {
		Text string `json:"text"`
	} `json:"reasoning_details"`
}

func reasoningText(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	var envelope chatReasoningEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return "", err
	}
	if envelope.ReasoningContent != "" {
		return envelope.ReasoningContent, nil
	}
	var builder strings.Builder
	for _, detail := range envelope.ReasoningDetails {
		builder.WriteString(detail.Text)
	}
	return builder.String(), nil
}

func (chatDialect) MapChatResponse(_ modelinvoker.Request, native *openaisdk.ChatCompletion, result *modelinvoker.Response) error {
	if native == nil || result == nil {
		return nil
	}
	var envelope struct {
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
	}
	if raw := native.RawJSON(); raw != "" && json.Unmarshal([]byte(raw), &envelope) != nil {
		return providerError(modelinvoker.ErrorMapping, "normalize_chat", "malformed MiniMax response envelope")
	}
	if envelope.BaseResp.StatusCode != 0 {
		return providerCodeError(strconv.Itoa(envelope.BaseResp.StatusCode))
	}
	if len(native.Choices) == 0 {
		return nil
	}
	reasoning, err := reasoningText(native.Choices[0].Message.RawJSON())
	if err != nil {
		return providerError(modelinvoker.ErrorMapping, "normalize_reasoning", "malformed MiniMax reasoning payload")
	}
	if reasoning != "" {
		result.Output = append(result.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemReasoningSummary, ReasoningSummary: reasoning})
	}
	return nil
}

func cumulativeDelta(current, previous string) string {
	if current == "" {
		return ""
	}
	if strings.HasPrefix(current, previous) {
		return current[len(previous):]
	}
	return current
}

func (chatDialect) MapChatStreamDelta(_ modelinvoker.Request, choice openaisdk.ChatCompletionChunkChoice, previousText, previousReasoning string) (string, string, []modelinvoker.MappingDecision, error) {
	reasoning, err := reasoningText(choice.Delta.RawJSON())
	if err != nil {
		return "", "", nil, providerError(modelinvoker.ErrorMapping, "stream_reasoning", "malformed MiniMax reasoning stream payload")
	}
	return cumulativeDelta(choice.Delta.Content, previousText), cumulativeDelta(reasoning, previousReasoning), nil, nil
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
	if request.Reasoning == nil {
		return params, nil, nil
	}
	if isM3(request.Model) {
		if request.Reasoning.Effort == "" {
			params.Reasoning.Effort = shared.ReasoningEffortMinimal
		}
		if request.Reasoning.Effort != modelinvoker.ReasoningEffortNone {
			return params, []modelinvoker.MappingDecision{transformed("MiniMax M3 treats compatible reasoning effort values as an on/off switch, not depth control")}, nil
		}
		return params, nil, nil
	}
	return params, []modelinvoker.MappingDecision{transformed("MiniMax M2.x always-on thinking ignores portable depth tiers")}, nil
}
func (responsesDialect) MapResponsesResponse(_ modelinvoker.Request, native *responses.Response, result *modelinvoker.Response) error {
	if result == nil {
		return nil
	}
	result.State = nil
	if native == nil || native.RawJSON() == "" {
		return nil
	}
	var envelope struct {
		Store *bool `json:"store"`
	}
	if err := json.Unmarshal([]byte(native.RawJSON()), &envelope); err != nil {
		return providerError(modelinvoker.ErrorMapping, "normalize_responses", "malformed MiniMax Responses envelope")
	}
	if envelope.Store != nil && *envelope.Store {
		return providerError(modelinvoker.ErrorMapping, "normalize_responses", "MiniMax Responses unexpectedly reported stored server state")
	}
	return nil
}

func transformed(detail string) modelinvoker.MappingDecision {
	return modelinvoker.MappingDecision{Capability: modelinvoker.CapabilityReasoning, Action: modelinvoker.MappingTransformed, Detail: detail}
}

var (
	_ protocol.Dialect                     = messagesDialect{}
	_ anthropicmessages.RequestMapper      = messagesDialect{}
	_ anthropicmessages.ContinuationMapper = messagesDialect{}
	_ protocol.Dialect                     = chatDialect{}
	_ openaichat.RequestMapper             = chatDialect{}
	_ openaichat.ResponseMapper            = chatDialect{}
	_ openaichat.StreamDeltaMapper         = chatDialect{}
	_ protocol.Dialect                     = responsesDialect{}
	_ openairesponses.RequestMapper        = responsesDialect{}
	_ openairesponses.ResponseMapper       = responsesDialect{}
)
