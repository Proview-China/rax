package anthropicmessages

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

func normalizeMessage(base *protocol.Base, request modelinvoker.Request, native *anthropicsdk.Message, headers http.Header, extension ContinuationMapper, stopMapper StopReasonMapper) (modelinvoker.Response, error) {
	if native == nil {
		return modelinvoker.Response{}, driverError(modelinvoker.ErrorProvider, "messages.normalize", "Messages returned a nil message")
	}

	status, reason, knownReason := normalizeStopReason(string(native.StopReason))
	var mappedStopError error
	if !knownReason && stopMapper != nil {
		if mapped, handled := stopMapper.MapMessagesStopReason(request, string(native.StopReason)); handled {
			status, reason, knownReason = mapped.Status, mapped.StopReason, true
			mappedStopError = mapped.Error
		}
	}
	rawResponse, rawErr := adaptercore.RawPayload(native.RawJSON(), native)
	result := modelinvoker.Response{
		ID:               native.ID,
		Provider:         base.Binding().Provider,
		Protocol:         base.Binding().Protocol,
		Model:            string(native.Model),
		Status:           status,
		StopReason:       reason,
		StopSequence:     native.StopSequence,
		Usage:            normalizeUsage(native.Usage),
		RequestID:        base.RequestID(headers),
		ProviderMetadata: messageMetadata(base, native, headers),
		MappingReport: modelinvoker.MappingReport{
			Provider: base.Binding().Provider, Protocol: base.Binding().Protocol, Endpoint: request.Endpoint,
		},
		RawResponse: rawResponse,
	}
	if rawErr != nil {
		detail := "could not serialize Anthropic response audit payload: " + rawErr.Error()
		result.MappingReport.Decisions = append(result.MappingReport.Decisions, modelinvoker.MappingDecision{
			Capability: modelinvoker.CapabilityTextGeneration,
			Action:     modelinvoker.MappingRejected,
			Detail:     detail,
		})
		return result, mappingErrorWithRequestID("messages.audit_response", detail, result.RequestID)
	}
	if !knownReason {
		if !request.AllowDegradation {
			return result, mappingErrorWithRequestID("messages.normalize", fmt.Sprintf("unsupported Anthropic stop reason %q", native.StopReason), result.RequestID)
		}
		result.MappingReport.Decisions = append(result.MappingReport.Decisions,
			degradation(modelinvoker.CapabilityTextGeneration,
				fmt.Sprintf("unknown Anthropic stop reason %q mapped to incomplete", native.StopReason)))
	}

	state, err := continuationState(base.Binding(), request, native, extension)
	if err != nil {
		return result, mappingErrorWithRequestID("messages.continuation", err.Error(), result.RequestID)
	}
	result.State = state

	if err := normalizeContentBlocks(request, &result, native.Content); err != nil {
		return result, err
	}

	if native.StopReason == anthropicsdk.StopReasonRefusal {
		message := strings.TrimSpace(result.Text())
		if message == "" {
			message = "response was rejected by Anthropic policy"
		}
		return result, &modelinvoker.Error{
			Kind: modelinvoker.ErrorPolicyRejected, Provider: base.Binding().Provider, Operation: "messages.normalize",
			Code: "refusal", Message: message, RequestID: result.RequestID,
		}
	}
	return result, mappedStopError
}

func normalizeContentBlocks(request modelinvoker.Request, result *modelinvoker.Response, blocks []anthropicsdk.ContentBlockUnion) error {
	for _, block := range blocks {
		switch block.Type {
		case "text":
			result.Output = append(result.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemText, Text: block.Text})
		case "thinking":
			if block.Thinking != "" {
				result.Output = append(result.Output, modelinvoker.OutputItem{
					Type: modelinvoker.OutputItemReasoningSummary, ReasoningSummary: block.Thinking,
				})
			}
		case "redacted_thinking":
			// The encrypted block is intentionally exposed only through State/RawResponse.
		case "tool_use":
			tool := block.AsToolUse()
			if tool.Caller.Type != "" && tool.Caller.Type != "direct" {
				if err := handleUnknownContent(request, result, "non-direct Anthropic tool use caller "+tool.Caller.Type); err != nil {
					return err
				}
				continue
			}
			call := modelinvoker.FunctionCall{ID: tool.ID, Name: tool.Name, Arguments: cloneJSON(tool.Input)}
			if !validFunctionCall(call) {
				return mappingErrorWithRequestID("messages.normalize", "Anthropic tool use requires an ID, valid name, and JSON object input", result.RequestID)
			}
			result.Output = append(result.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call})
		default:
			if err := handleUnknownContent(request, result, "Anthropic content block "+block.Type); err != nil {
				return err
			}
		}
	}

	return nil
}

func normalizeStopReason(reason string) (modelinvoker.ResponseStatus, modelinvoker.StopReason, bool) {
	switch reason {
	case "end_turn":
		return modelinvoker.ResponseStatusCompleted, modelinvoker.StopReasonEndTurn, true
	case "max_tokens":
		return modelinvoker.ResponseStatusIncomplete, modelinvoker.StopReasonMaxOutputTokens, true
	case "stop_sequence":
		return modelinvoker.ResponseStatusCompleted, modelinvoker.StopReasonStopSequence, true
	case "tool_use":
		return modelinvoker.ResponseStatusCompleted, modelinvoker.StopReasonToolCall, true
	case "pause_turn":
		return modelinvoker.ResponseStatusIncomplete, modelinvoker.StopReasonPaused, true
	case "refusal":
		return modelinvoker.ResponseStatusFailed, modelinvoker.StopReasonContentFilter, true
	default:
		return modelinvoker.ResponseStatusIncomplete, modelinvoker.StopReasonOther, false
	}
}

func normalizeUsage(usage anthropicsdk.Usage) modelinvoker.Usage {
	input := usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens
	return modelinvoker.Usage{
		InputTokens:      input,
		OutputTokens:     usage.OutputTokens,
		ReasoningTokens:  usage.OutputTokensDetails.ThinkingTokens,
		CacheReadTokens:  usage.CacheReadInputTokens,
		CacheWriteTokens: usage.CacheCreationInputTokens,
		TotalTokens:      input + usage.OutputTokens,
	}
}

func continuationState(binding protocol.Binding, request modelinvoker.Request, native *anthropicsdk.Message, extension ContinuationMapper) (*modelinvoker.State, error) {
	if native == nil || len(native.Content) == 0 {
		return nil, nil
	}
	keep := native.StopReason == anthropicsdk.StopReasonToolUse
	for _, block := range native.Content {
		if block.Type == "thinking" || block.Type == "redacted_thinking" || block.Type == "tool_use" {
			keep = true
			break
		}
	}
	if !keep {
		return nil, nil
	}

	var source struct {
		Content []json.RawMessage `json:"content"`
	}
	raw := native.RawJSON()
	if raw == "" {
		data, err := json.Marshal(native)
		if err != nil {
			return nil, fmt.Errorf("serialize Anthropic message: %w", err)
		}
		raw = string(data)
	}
	if err := json.Unmarshal([]byte(raw), &source); err != nil {
		return nil, fmt.Errorf("extract Anthropic continuation content: %w", err)
	}
	if len(source.Content) != len(native.Content) {
		data, err := json.Marshal(native)
		if err != nil {
			return nil, fmt.Errorf("serialize accumulated Anthropic continuation: %w", err)
		}
		if err := json.Unmarshal(data, &source); err != nil || len(source.Content) != len(native.Content) {
			return nil, fmt.Errorf("Anthropic continuation content is incomplete")
		}
	}
	if extension != nil {
		mapped, err := extension.MapMessagesContinuation(request, source.Content)
		if err != nil {
			return nil, fmt.Errorf("map provider continuation content: %w", err)
		}
		source.Content = mapped
	}
	validation, err := validateContinuationContent(source.Content)
	if err != nil {
		return nil, fmt.Errorf("validate Anthropic continuation content: %w", err)
	}
	if !validation.hasResumable {
		return nil, fmt.Errorf("Anthropic continuation contains no thinking or tool-use state")
	}
	payload, err := json.Marshal(continuationMessage{
		Version: continuationVersion, Role: "assistant", Content: source.Content,
	})
	if err != nil {
		return nil, fmt.Errorf("serialize Anthropic continuation: %w", err)
	}
	return &modelinvoker.State{
		Kind:     modelinvoker.StateProviderContinuation,
		Provider: binding.Provider,
		Protocol: binding.Protocol,
		ID:       native.ID,
		Payload:  modelinvoker.NewRawPayload(payload),
	}, nil
}

func handleUnknownContent(request modelinvoker.Request, response *modelinvoker.Response, description string) error {
	if !request.AllowDegradation {
		return mappingErrorWithRequestID("messages.normalize", description+" is outside the unified Anthropic slice", response.RequestID)
	}
	response.MappingReport.Decisions = append(response.MappingReport.Decisions,
		degradation(modelinvoker.CapabilityTextGeneration, description+" retained only in RawResponse and continuation state"))
	return nil
}

func validFunctionCall(call modelinvoker.FunctionCall) bool {
	if call.ID == "" || !nativeToolNamePattern.MatchString(call.Name) {
		return false
	}
	_, err := jsonObject(call.Arguments)
	return err == nil
}

func cloneJSON(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}

func messageMetadata(base *protocol.Base, message *anthropicsdk.Message, headers http.Header) modelinvoker.ProviderMetadata {
	metadata := base.ProviderMetadata(headers)
	if message == nil {
		return metadata
	}
	metadata["stop_reason"] = string(message.StopReason)
	if message.StopSequence != "" {
		metadata["stop_sequence"] = message.StopSequence
	}
	metadata["input_tokens_uncached"] = strconv.FormatInt(message.Usage.InputTokens, 10)
	metadata["cache_creation_input_tokens"] = strconv.FormatInt(message.Usage.CacheCreationInputTokens, 10)
	metadata["cache_read_input_tokens"] = strconv.FormatInt(message.Usage.CacheReadInputTokens, 10)
	metadata["cache_creation_ephemeral_5m_input_tokens"] = strconv.FormatInt(message.Usage.CacheCreation.Ephemeral5mInputTokens, 10)
	metadata["cache_creation_ephemeral_1h_input_tokens"] = strconv.FormatInt(message.Usage.CacheCreation.Ephemeral1hInputTokens, 10)
	if message.Usage.ServiceTier != "" {
		metadata["service_tier"] = string(message.Usage.ServiceTier)
	}
	if message.Usage.InferenceGeo != "" {
		metadata["inference_geo"] = message.Usage.InferenceGeo
	}
	return metadata
}
