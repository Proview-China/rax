package openaichat

import (
	"encoding/json"
	"fmt"
	"net/http"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	openaisdk "github.com/openai/openai-go/v3"
)

func normalizeResponse(base *protocol.Base, request modelinvoker.Request, native *openaisdk.ChatCompletion, headers http.Header, finishMapper FinishReasonMapper) (modelinvoker.Response, error) {
	if native == nil {
		return modelinvoker.Response{}, driverError(modelinvoker.ErrorProvider, "chat_completions.normalize", "provider returned a nil chat completion")
	}
	rawResponse, rawErr := adaptercore.RawPayload(native.RawJSON(), native)
	result := modelinvoker.Response{
		ID: native.ID, Protocol: modelinvoker.ProtocolChatCompletions, Model: native.Model,
		Status: modelinvoker.ResponseStatusCompleted, Usage: chatUsage(native.Usage),
		RequestID: base.RequestID(headers), ProviderMetadata: base.ProviderMetadata(headers),
		MappingReport: modelinvoker.MappingReport{Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: request.Endpoint},
		RawResponse:   rawResponse,
	}
	if rawErr != nil {
		return result, mappingErrorWithRequestID("chat_completions.audit_response", fmt.Sprintf("failed to preserve raw response: %v", rawErr), result.RequestID)
	}
	if len(native.Choices) == 0 {
		return result, driverError(modelinvoker.ErrorProvider, "chat_completions.normalize", "chat completion contains no choices")
	}
	if len(native.Choices) > 1 {
		if !request.AllowDegradation {
			return result, mappingErrorWithRequestID("chat_completions.normalize", "multiple choices require explicit degradation permission", result.RequestID)
		}
		result.MappingReport.Decisions = append(result.MappingReport.Decisions,
			degradation(modelinvoker.CapabilityTextGeneration, "only the first Chat Completions choice was normalized"))
	}
	choice := native.Choices[0]
	stopReason, knownFinishReason := chatStopReason(choice.FinishReason)
	result.StopReason = stopReason
	if choice.Message.Refusal != "" || choice.FinishReason == "content_filter" {
		result.StopReason = modelinvoker.StopReasonContentFilter
		message := choice.Message.Refusal
		if message == "" {
			message = "content was rejected by provider policy"
		}
		return result, &modelinvoker.Error{
			Kind: modelinvoker.ErrorPolicyRejected, Operation: "chat_completions.normalize",
			Code: choice.FinishReason, Message: message, RequestID: result.RequestID,
		}
	}
	if choice.Message.Content != "" {
		result.Output = append(result.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemText, Text: choice.Message.Content})
	}
	for _, tool := range choice.Message.ToolCalls {
		if tool.Type != "function" {
			if err := handleUnknownOutput(request, &result, "Chat Completions tool call "+tool.Type); err != nil {
				return result, err
			}
			continue
		}
		call := tool.AsFunction()
		semantic := modelinvoker.FunctionCall{ID: call.ID, Name: call.Function.Name, Arguments: json.RawMessage(call.Function.Arguments)}
		if !validFunctionCall(semantic) {
			return result, mappingErrorWithRequestID("chat_completions.normalize", "function call requires an ID, valid name, and JSON object arguments", result.RequestID)
		}
		result.Output = append(result.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: cloneCall(semantic)})
	}
	if finishMapper != nil {
		if mapping, handled := finishMapper.MapChatFinishReason(request, choice.FinishReason); handled {
			if mapping.Status != "" {
				result.Status = mapping.Status
			}
			if mapping.StopReason != "" {
				result.StopReason = mapping.StopReason
			}
			return result, mapping.Error
		}
	}
	switch choice.FinishReason {
	case "stop", "tool_calls":
	case "length":
		result.Status = modelinvoker.ResponseStatusIncomplete
	default:
		if !knownFinishReason && !request.AllowDegradation {
			return result, mappingErrorWithRequestID("chat_completions.normalize", fmt.Sprintf("unsupported finish reason %q", choice.FinishReason), result.RequestID)
		}
		if !knownFinishReason {
			result.StopReason = modelinvoker.StopReasonOther
			result.MappingReport.Decisions = append(result.MappingReport.Decisions,
				degradation(modelinvoker.CapabilityTextGeneration, fmt.Sprintf("finish reason %q retained only in RawResponse", choice.FinishReason)))
		}
	}
	return result, nil
}

func handleUnknownOutput(request modelinvoker.Request, result *modelinvoker.Response, description string) error {
	if !request.AllowDegradation {
		return mappingErrorWithRequestID("normalize_output", description+" is outside the implemented semantic slice", result.RequestID)
	}
	result.MappingReport.Decisions = append(result.MappingReport.Decisions,
		degradation(modelinvoker.CapabilityTextGeneration, description+" retained only in RawResponse"))
	return nil
}

func chatUsage(usage openaisdk.CompletionUsage) modelinvoker.Usage {
	return modelinvoker.Usage{
		InputTokens: usage.PromptTokens, OutputTokens: usage.CompletionTokens,
		ReasoningTokens: usage.CompletionTokensDetails.ReasoningTokens,
		CacheReadTokens: usage.PromptTokensDetails.CachedTokens, TotalTokens: usage.TotalTokens,
	}
}

func chatStopReason(finishReason string) (modelinvoker.StopReason, bool) {
	switch finishReason {
	case "stop":
		return modelinvoker.StopReasonEndTurn, true
	case "tool_calls":
		return modelinvoker.StopReasonToolCall, true
	case "length":
		return modelinvoker.StopReasonMaxOutputTokens, true
	case "content_filter":
		return modelinvoker.StopReasonContentFilter, true
	default:
		return modelinvoker.StopReasonOther, false
	}
}
