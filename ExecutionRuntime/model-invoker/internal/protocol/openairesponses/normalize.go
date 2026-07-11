package openairesponses

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/openai/openai-go/v3/responses"
)

func normalizeResponse(ctx context.Context, base *protocol.Base, request modelinvoker.Request, native *responses.Response, headers http.Header) (modelinvoker.Response, error) {
	if native == nil {
		return modelinvoker.Response{}, driverError(modelinvoker.ErrorProvider, "responses.normalize", "provider returned a nil response")
	}
	status, knownStatus := responseStatus(string(native.Status))
	rawResponse, rawErr := adaptercore.RawPayload(native.RawJSON(), native)
	result := modelinvoker.Response{
		ID:               native.ID,
		Protocol:         modelinvoker.ProtocolResponses,
		Model:            string(native.Model),
		Status:           status,
		Usage:            responseUsage(native.Usage),
		RequestID:        base.RequestID(headers),
		Metadata:         modelinvoker.Metadata(native.Metadata),
		State:            responseState(native.ID),
		ProviderMetadata: base.ProviderMetadata(headers),
		MappingReport:    modelinvoker.MappingReport{Protocol: modelinvoker.ProtocolResponses, Endpoint: request.Endpoint},
		RawResponse:      rawResponse,
	}
	if rawErr != nil {
		return result, mappingErrorWithRequestID("responses.audit_response", fmt.Sprintf("failed to preserve raw response: %v", rawErr), result.RequestID)
	}
	if !knownStatus {
		if !request.AllowDegradation {
			return result, mappingErrorWithRequestID("responses.normalize", fmt.Sprintf("unsupported response status %q", native.Status), result.RequestID)
		}
		result.MappingReport.Decisions = append(result.MappingReport.Decisions,
			degradation(modelinvoker.CapabilityTextGeneration, fmt.Sprintf("response status %q mapped to in_progress", native.Status)))
	}
	if native.IncompleteDetails.Reason != "" {
		result.ProviderMetadata["incomplete_reason"] = native.IncompleteDetails.Reason
		switch native.IncompleteDetails.Reason {
		case "max_output_tokens":
			result.StopReason = modelinvoker.StopReasonMaxOutputTokens
		case "content_filter":
			result.StopReason = modelinvoker.StopReasonContentFilter
			return result, &modelinvoker.Error{
				Kind: modelinvoker.ErrorPolicyRejected, Operation: "responses.normalize",
				Code: native.IncompleteDetails.Reason, Message: "response was truncated by provider content policy", RequestID: result.RequestID,
			}
		default:
			result.StopReason = modelinvoker.StopReasonOther
		}
	}

	hasFunctionCall := false
	for _, item := range native.Output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				switch content.Type {
				case "output_text":
					result.Output = append(result.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemText, Text: content.Text})
				case "refusal":
					result.StopReason = modelinvoker.StopReasonContentFilter
					return result, &modelinvoker.Error{
						Kind: modelinvoker.ErrorPolicyRejected, Operation: "responses.normalize",
						Code: "refusal", Message: content.Refusal, RequestID: result.RequestID,
					}
				default:
					if err := handleUnknownOutput(request, &result, "Responses message content "+content.Type); err != nil {
						return result, err
					}
				}
			}
		case "function_call":
			hasFunctionCall = true
			call := item.AsFunctionCall()
			semantic := modelinvoker.FunctionCall{ID: call.CallID, Name: call.Name, Arguments: json.RawMessage(call.Arguments)}
			if !validFunctionCall(semantic) {
				return result, mappingErrorWithRequestID("responses.normalize", "function call requires an ID, valid name, and JSON object arguments", result.RequestID)
			}
			result.Output = append(result.Output, modelinvoker.OutputItem{
				Type:         modelinvoker.OutputItemFunctionCall,
				FunctionCall: cloneCall(semantic),
			})
		case "reasoning":
			reasoning := item.AsReasoning()
			for _, summary := range reasoning.Summary {
				result.Output = append(result.Output, modelinvoker.OutputItem{
					Type: modelinvoker.OutputItemReasoningSummary, ReasoningSummary: summary.Text,
				})
			}
		default:
			if err := handleUnknownOutput(request, &result, "Responses output item "+item.Type); err != nil {
				return result, err
			}
		}
	}
	if result.StopReason == "" && result.Status == modelinvoker.ResponseStatusCompleted {
		result.StopReason = modelinvoker.StopReasonEndTurn
		if hasFunctionCall {
			result.StopReason = modelinvoker.StopReasonToolCall
		}
	} else if result.StopReason == "" && result.Status == modelinvoker.ResponseStatusIncomplete {
		result.StopReason = modelinvoker.StopReasonOther
	}

	if result.Status == modelinvoker.ResponseStatusFailed {
		result.StopReason = modelinvoker.StopReasonOther
		code := string(native.Error.Code)
		return result, base.NormalizeFailure(ctx, request, "responses.generate", protocol.Failure{
			Source: protocol.FailureSourceSDK, Code: code,
			Message: native.Error.Message, RequestID: result.RequestID,
		})
	}
	return result, nil
}

func handleUnknownOutput(request modelinvoker.Request, result *modelinvoker.Response, description string) error {
	if !request.AllowDegradation {
		return mappingErrorWithRequestID("normalize_output", description+" is outside the first unified semantic slice", result.RequestID)
	}
	result.MappingReport.Decisions = append(result.MappingReport.Decisions,
		degradation(modelinvoker.CapabilityTextGeneration, description+" retained only in RawResponse"))
	return nil
}

func responseStatus(status string) (modelinvoker.ResponseStatus, bool) {
	switch status {
	case "completed":
		return modelinvoker.ResponseStatusCompleted, true
	case "incomplete":
		return modelinvoker.ResponseStatusIncomplete, true
	case "failed":
		return modelinvoker.ResponseStatusFailed, true
	case "cancelled":
		return modelinvoker.ResponseStatusCancelled, true
	case "in_progress":
		return modelinvoker.ResponseStatusInProgress, true
	default:
		return modelinvoker.ResponseStatusInProgress, false
	}
}

func responseState(id string) *modelinvoker.State {
	if id == "" {
		return nil
	}
	return &modelinvoker.State{
		Kind: modelinvoker.StateServerContinuation, Protocol: modelinvoker.ProtocolResponses, ID: id,
	}
}

func responseUsage(usage responses.ResponseUsage) modelinvoker.Usage {
	return modelinvoker.Usage{
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		ReasoningTokens: usage.OutputTokensDetails.ReasoningTokens,
		CacheReadTokens: usage.InputTokensDetails.CachedTokens, TotalTokens: usage.TotalTokens,
	}
}
