package geminigenerate

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"google.golang.org/genai"
)

func normalizeGenerateContent(
	base *protocol.Base,
	request modelinvoker.Request,
	native *genai.GenerateContentResponse,
	headers http.Header,
	envelope continuationEnvelope,
) (modelinvoker.Response, error) {
	if native == nil {
		return modelinvoker.Response{}, driverError(modelinvoker.ErrorProvider, "generate_content.normalize", "GenerateContent returned a nil response")
	}
	rawResponse, rawErr := rawGenerateContentPayload(native)
	usage, calculatedTotal := normalizeUsage(native.UsageMetadata)
	result := modelinvoker.Response{
		ID:               native.ResponseID,
		Provider:         base.Binding().Provider,
		Protocol:         base.Binding().Protocol,
		Model:            request.Model,
		Usage:            usage,
		RequestID:        base.RequestID(headers),
		ProviderMetadata: responseMetadata(base, native, headers),
		MappingReport: modelinvoker.MappingReport{
			Provider: base.Binding().Provider, Protocol: base.Binding().Protocol, Endpoint: request.Endpoint,
		},
		RawResponse: rawResponse,
	}
	if rawErr != nil {
		result.Status = modelinvoker.ResponseStatusFailed
		return result, protocolError(
			"generate_content.raw_response",
			"failed to construct controlled Gemini response payload",
			result.RequestID,
		)
	}
	if calculatedTotal {
		result.MappingReport.Decisions = append(result.MappingReport.Decisions, transformation(
			modelinvoker.CapabilityUsageReporting,
			"Gemini omitted totalTokenCount; total calculated from prompt, tool-use prompt, candidate, and thought counts",
		))
	}

	if blockedReason(native.PromptFeedback) != "" {
		result.Status = modelinvoker.ResponseStatusFailed
		result.StopReason = modelinvoker.StopReasonContentFilter
		message := strings.TrimSpace(native.PromptFeedback.BlockReasonMessage)
		if message == "" {
			message = "Gemini rejected the prompt under provider policy"
		}
		return result, &modelinvoker.Error{
			Kind: modelinvoker.ErrorPolicyRejected, Provider: base.Binding().Provider, Operation: "generate_content.normalize",
			Code: blockedReason(native.PromptFeedback), Message: message, RequestID: result.RequestID,
		}
	}
	if len(native.Candidates) == 0 {
		result.Status = modelinvoker.ResponseStatusFailed
		return result, protocolError("generate_content.normalize", "Gemini response contains no candidate", result.RequestID)
	}
	if len(native.Candidates) != 1 {
		result.Status = modelinvoker.ResponseStatusFailed
		return result, mappingErrorWithRequestID("generate_content.normalize", "Gemini multiple candidates are outside the unified GenerateContent slice", result.RequestID)
	}
	candidate := native.Candidates[0]
	if candidate == nil {
		result.Status = modelinvoker.ResponseStatusFailed
		return result, protocolError("generate_content.normalize", "Gemini response candidate is nil", result.RequestID)
	}
	result.ProviderMetadata["finish_reason"] = string(candidate.FinishReason)
	if candidate.FinishMessage != "" {
		result.ProviderMetadata["finish_message"] = candidate.FinishMessage
	}
	result.ProviderMetadata["candidate_token_count"] = strconv.FormatInt(int64(candidate.TokenCount), 10)

	status, stopReason, finishError := normalizeFinishReason(candidate.FinishReason, candidate.FinishMessage, false, result.RequestID)
	result.Status = status
	result.StopReason = stopReason
	if candidate.Content == nil {
		if finishError != nil {
			return result, finishError
		}
		return result, protocolError("generate_content.normalize", "Gemini response candidate contains no content", result.RequestID)
	}
	if candidate.Content.Role != genai.RoleModel {
		return result, protocolError("generate_content.normalize", fmt.Sprintf("Gemini candidate has invalid role %q", candidate.Content.Role), result.RequestID)
	}
	if len(candidate.Content.Parts) == 0 {
		if finishError != nil {
			return result, finishError
		}
		return result, protocolError("generate_content.normalize", "Gemini candidate content has no parts", result.RequestID)
	}

	hasFunctionCall := false
	continuationParts := make([]*genai.Part, 0, len(candidate.Content.Parts))
	for partIndex, part := range candidate.Content.Parts {
		if part == nil {
			return result, protocolError("generate_content.normalize", fmt.Sprintf("Gemini candidate part %d is nil", partIndex), result.RequestID)
		}
		unsupported := unsupportedPartDescription(part)
		if unsupported != "" {
			if !request.AllowDegradation {
				return result, mappingErrorWithRequestID("generate_content.normalize", unsupported+" is outside the unified Gemini slice", result.RequestID)
			}
			result.MappingReport.Decisions = append(result.MappingReport.Decisions, degradation(
				modelinvoker.CapabilityTextGeneration,
				unsupported+" retained only in RawResponse",
			))
			continue
		}
		if err := validateContinuationPart(genai.RoleModel, part); err != nil {
			return result, protocolError("generate_content.normalize", fmt.Sprintf("Gemini candidate part %d is invalid: %v", partIndex, err), result.RequestID)
		}
		cloned, err := cloneContent(&genai.Content{Role: genai.RoleModel, Parts: []*genai.Part{part}})
		if err != nil || cloned == nil || len(cloned.Parts) != 1 {
			return result, protocolError("generate_content.normalize", "failed to clone Gemini candidate part", result.RequestID)
		}
		continuationParts = append(continuationParts, cloned.Parts[0])
		if part.FunctionCall != nil {
			hasFunctionCall = true
			call, err := normalizeFunctionCall(&envelope, native.ResponseID, int(candidate.Index), partIndex, part.FunctionCall)
			if err != nil {
				return result, mappingErrorWithRequestID("generate_content.normalize", err.Error(), result.RequestID)
			}
			result.Output = append(result.Output, modelinvoker.OutputItem{
				Type: modelinvoker.OutputItemFunctionCall, FunctionCall: call,
			})
			continue
		}
		if part.Thought {
			// Thought text is provider-native reasoning. It is intentionally not
			// mislabeled as a portable reasoning summary.
			continue
		}
		if part.Text != "" {
			result.Output = append(result.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemText, Text: part.Text})
		}
	}

	if hasFunctionCall && candidate.FinishReason == genai.FinishReasonStop {
		result.StopReason = modelinvoker.StopReasonToolCall
	}
	content := &genai.Content{Role: genai.RoleModel, Parts: continuationParts}
	if continuationEnvelopeRequiresState(envelope) || contentRequiresContinuation(content) {
		if len(content.Parts) > 0 {
			envelope.Contents = append(envelope.Contents, content)
		}
		state, err := encodeContinuation(base.Binding(), envelope, native.ResponseID)
		if err != nil {
			return result, mappingErrorWithRequestID("generate_content.continuation", err.Error(), result.RequestID)
		}
		result.State = state
	}
	if finishError != nil {
		return result, finishError
	}
	return result, nil
}

func normalizeFunctionCall(
	envelope *continuationEnvelope,
	responseID string,
	candidateIndex int,
	partIndex int,
	native *genai.FunctionCall,
) (*modelinvoker.FunctionCall, error) {
	if native == nil || !nativeToolNamePattern.MatchString(native.Name) {
		return nil, fmt.Errorf("Gemini function call requires a valid name")
	}
	arguments := native.Args
	if arguments == nil {
		arguments = map[string]any{}
	}
	raw, err := json.Marshal(arguments)
	if err != nil {
		return nil, fmt.Errorf("Gemini function call %q has invalid arguments", native.Name)
	}
	id, err := addContinuationCall(envelope, responseID, candidateIndex, partIndex, native)
	if err != nil {
		return nil, err
	}
	return &modelinvoker.FunctionCall{ID: id, Name: native.Name, Arguments: raw}, nil
}

func normalizeFinishReason(
	reason genai.FinishReason,
	message string,
	hasFunctionCall bool,
	requestIDValue string,
) (modelinvoker.ResponseStatus, modelinvoker.StopReason, error) {
	switch reason {
	case genai.FinishReasonStop:
		stop := modelinvoker.StopReasonEndTurn
		if hasFunctionCall {
			stop = modelinvoker.StopReasonToolCall
		}
		return modelinvoker.ResponseStatusCompleted, stop, nil
	case genai.FinishReasonMaxTokens:
		return modelinvoker.ResponseStatusIncomplete, modelinvoker.StopReasonMaxOutputTokens, nil
	case genai.FinishReasonOther:
		return modelinvoker.ResponseStatusIncomplete, modelinvoker.StopReasonOther, nil
	case genai.FinishReasonSafety,
		genai.FinishReasonRecitation,
		genai.FinishReasonLanguage,
		genai.FinishReasonBlocklist,
		genai.FinishReasonProhibitedContent,
		genai.FinishReasonSPII,
		genai.FinishReasonImageSafety,
		genai.FinishReasonImageProhibitedContent,
		genai.FinishReasonImageRecitation:
		if strings.TrimSpace(message) == "" {
			message = "Gemini stopped generation under provider policy"
		}
		return modelinvoker.ResponseStatusFailed, modelinvoker.StopReasonContentFilter, &modelinvoker.Error{
			Kind: modelinvoker.ErrorPolicyRejected, Operation: "generate_content.normalize",
			Code: string(reason), Message: message, RequestID: requestIDValue,
		}
	case genai.FinishReasonMalformedFunctionCall, genai.FinishReasonUnexpectedToolCall:
		if strings.TrimSpace(message) == "" {
			message = "Gemini generated an invalid function call"
		}
		return modelinvoker.ResponseStatusFailed, modelinvoker.StopReasonOther, &modelinvoker.Error{
			Kind: modelinvoker.ErrorProvider, Operation: "generate_content.normalize",
			Code: string(reason), Message: message, RequestID: requestIDValue,
		}
	case genai.FinishReasonNoImage, genai.FinishReasonImageOther:
		if strings.TrimSpace(message) == "" {
			message = "Gemini did not produce the requested output modality"
		}
		return modelinvoker.ResponseStatusFailed, modelinvoker.StopReasonOther, &modelinvoker.Error{
			Kind: modelinvoker.ErrorProvider, Operation: "generate_content.normalize",
			Code: string(reason), Message: message, RequestID: requestIDValue,
		}
	case "", genai.FinishReasonUnspecified:
		return modelinvoker.ResponseStatusFailed, modelinvoker.StopReasonOther,
			protocolError("generate_content.normalize", "Gemini response has no terminal finish reason", requestIDValue)
	default:
		return modelinvoker.ResponseStatusIncomplete, modelinvoker.StopReasonOther,
			mappingErrorWithRequestID("generate_content.normalize", fmt.Sprintf("unsupported Gemini finish reason %q", reason), requestIDValue)
	}
}

func normalizeUsage(native *genai.GenerateContentResponseUsageMetadata) (modelinvoker.Usage, bool) {
	if native == nil {
		return modelinvoker.Usage{}, false
	}
	input := int64(native.PromptTokenCount) + int64(native.ToolUsePromptTokenCount)
	output := int64(native.CandidatesTokenCount) + int64(native.ThoughtsTokenCount)
	total := int64(native.TotalTokenCount)
	calculated := false
	if total == 0 && input+output > 0 {
		total = input + output
		calculated = true
	}
	return modelinvoker.Usage{
		InputTokens:      input,
		OutputTokens:     output,
		ReasoningTokens:  int64(native.ThoughtsTokenCount),
		CacheReadTokens:  int64(native.CachedContentTokenCount),
		CacheWriteTokens: 0,
		TotalTokens:      total,
	}, calculated
}

func contentRequiresContinuation(content *genai.Content) bool {
	if content == nil {
		return false
	}
	for _, part := range content.Parts {
		if part != nil && (part.FunctionCall != nil || part.Thought || len(part.ThoughtSignature) > 0) {
			return true
		}
	}
	return false
}

// continuationEnvelopeRequiresState keeps provider-required model/tool history
// alive across later plain-text turns without turning ordinary text into state.
func continuationEnvelopeRequiresState(envelope continuationEnvelope) bool {
	for _, content := range envelope.Contents {
		if contentRequiresContinuation(content) {
			return true
		}
	}
	return false
}

func unsupportedPartDescription(part *genai.Part) string {
	if part == nil {
		return "nil Gemini part"
	}
	unsupported := make([]string, 0, 8)
	if part.CodeExecutionResult != nil {
		unsupported = append(unsupported, "code execution result")
	}
	if part.ExecutableCode != nil {
		unsupported = append(unsupported, "executable code")
	}
	if part.FileData != nil {
		unsupported = append(unsupported, "file data")
	}
	if part.FunctionResponse != nil {
		unsupported = append(unsupported, "model function response")
	}
	if part.InlineData != nil {
		unsupported = append(unsupported, "inline media")
	}
	if part.MediaResolution != nil || part.VideoMetadata != nil {
		unsupported = append(unsupported, "media metadata")
	}
	if part.ToolCall != nil || part.ToolResponse != nil {
		unsupported = append(unsupported, "server-side tool content")
	}
	if len(part.PartMetadata) > 0 {
		unsupported = append(unsupported, "part metadata")
	}
	if len(unsupported) > 0 {
		return "Gemini " + strings.Join(unsupported, ", ")
	}
	if part.FunctionCall != nil && (part.Text != "" || part.Thought) {
		return "Gemini part with multiple semantic values"
	}
	return ""
}

func blockedReason(feedback *genai.GenerateContentResponsePromptFeedback) string {
	if feedback == nil || feedback.BlockReason == "" || feedback.BlockReason == genai.BlockedReasonUnspecified {
		return ""
	}
	return string(feedback.BlockReason)
}

func responseMetadata(base *protocol.Base, native *genai.GenerateContentResponse, headers http.Header) modelinvoker.ProviderMetadata {
	metadata := base.ProviderMetadata(headers)
	if native == nil {
		return metadata
	}
	if native.ModelVersion != "" {
		metadata["model_version"] = native.ModelVersion
	}
	if reason := blockedReason(native.PromptFeedback); reason != "" {
		metadata["prompt_block_reason"] = reason
	}
	if native.UsageMetadata != nil {
		metadata["prompt_token_count"] = strconv.FormatInt(int64(native.UsageMetadata.PromptTokenCount), 10)
		metadata["tool_use_prompt_token_count"] = strconv.FormatInt(int64(native.UsageMetadata.ToolUsePromptTokenCount), 10)
		metadata["candidate_token_count"] = strconv.FormatInt(int64(native.UsageMetadata.CandidatesTokenCount), 10)
		metadata["thought_token_count"] = strconv.FormatInt(int64(native.UsageMetadata.ThoughtsTokenCount), 10)
		metadata["cached_content_token_count"] = strconv.FormatInt(int64(native.UsageMetadata.CachedContentTokenCount), 10)
	}
	return metadata
}

func rawGenerateContentPayload(native *genai.GenerateContentResponse) (modelinvoker.RawPayload, error) {
	if native == nil {
		return modelinvoker.RawPayload{}, nil
	}
	raw := ""
	if native.SDKHTTPResponse != nil {
		raw = native.SDKHTTPResponse.Body
		if raw != "" && !json.Valid([]byte(raw)) {
			return modelinvoker.RawPayload{}, fmt.Errorf("Gemini SDK response body is not valid JSON")
		}
	}
	payload := make(map[string]any)
	if native.Candidates != nil {
		payload["candidates"] = native.Candidates
	}
	if !native.CreateTime.IsZero() {
		payload["createTime"] = native.CreateTime
	}
	if native.ModelVersion != "" {
		payload["modelVersion"] = native.ModelVersion
	}
	if native.PromptFeedback != nil {
		payload["promptFeedback"] = native.PromptFeedback
	}
	if native.ResponseID != "" {
		payload["responseId"] = native.ResponseID
	}
	if native.UsageMetadata != nil {
		payload["usageMetadata"] = native.UsageMetadata
	}
	if native.ModelStatus != nil {
		payload["modelStatus"] = native.ModelStatus
	}
	controlled, err := adaptercore.RawPayload(raw, payload)
	if err != nil {
		return modelinvoker.RawPayload{}, fmt.Errorf("serialize Gemini response audit payload: %w", err)
	}
	return controlled, nil
}

func protocolError(operation, message, id string) *modelinvoker.Error {
	return &modelinvoker.Error{
		Kind: modelinvoker.ErrorProvider, Operation: operation,
		Message: message, RequestID: id,
	}
}
