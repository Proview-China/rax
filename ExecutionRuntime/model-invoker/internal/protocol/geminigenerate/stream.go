package geminigenerate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"google.golang.org/genai"
)

type generateContentStream struct {
	ctx             context.Context
	cancel          context.CancelFunc
	request         modelinvoker.Request
	base            *protocol.Base
	native          EventStream
	headers         http.Header
	rawRequest      modelinvoker.RawPayload
	decisions       []modelinvoker.MappingDecision
	envelope        continuationEnvelope
	queue           []modelinvoker.StreamEvent
	current         modelinvoker.StreamEvent
	err             error
	closed          bool
	terminal        bool
	started         bool
	sequence        int64
	responseID      string
	modelVersion    string
	finishReason    genai.FinishReason
	finishMessage   string
	promptFeedback  *genai.GenerateContentResponsePromptFeedback
	usage           modelinvoker.Usage
	usageMetadata   *genai.GenerateContentResponseUsageMetadata
	usageSeen       bool
	usageFallback   bool
	hasFunctionCall bool
	chunkOrdinal    int64
	modelParts      []*genai.Part
	output          []modelinvoker.OutputItem
	nativeEvents    []modelinvoker.RawPayload
	rawStream       bytes.Buffer
	seenNativeCalls map[string]nativeCallSnapshot
	seenIDLessCalls map[idLessCallPosition]idLessCallSnapshot
}

type nativeCallSnapshot struct {
	arguments      string
	modelPartIndex int
}

type idLessCallPosition struct {
	candidate int
	ordinal   int
}

type idLessCallSnapshot struct {
	chunk          int64
	fingerprint    string
	modelPartIndex int
}

func newGenerateContentStream(
	ctx context.Context,
	cancel context.CancelFunc,
	request modelinvoker.Request,
	base *protocol.Base,
	native EventStream,
	headers http.Header,
	rawRequest modelinvoker.RawPayload,
	decisions []modelinvoker.MappingDecision,
	envelope continuationEnvelope,
) modelinvoker.Stream {
	return &generateContentStream{
		ctx:             ctx,
		cancel:          cancel,
		request:         request,
		base:            base,
		native:          native,
		headers:         headers.Clone(),
		rawRequest:      rawRequest,
		decisions:       append([]modelinvoker.MappingDecision(nil), decisions...),
		envelope:        envelope,
		seenNativeCalls: make(map[string]nativeCallSnapshot),
		seenIDLessCalls: make(map[idLessCallPosition]idLessCallSnapshot),
	}
}

func (s *generateContentStream) Next() bool {
	if s == nil || s.closed || (s.terminal && len(s.queue) == 0) {
		return false
	}
	for len(s.queue) == 0 && !s.terminal {
		if contextErr := s.ctx.Err(); contextErr != nil {
			s.failWithStatus(
				normalizeFailure(s.ctx, s.base, s.request, "generate_content.stream", s.headers, contextErr),
				modelinvoker.RawPayload{},
				modelinvoker.ResponseStatusCancelled,
				s.currentStopReason(),
			)
			break
		}
		if !s.native.Next() {
			if contextErr := s.ctx.Err(); contextErr != nil {
				s.failWithStatus(
					normalizeFailure(s.ctx, s.base, s.request, "generate_content.stream", s.headers, contextErr),
					modelinvoker.RawPayload{},
					modelinvoker.ResponseStatusCancelled,
					s.currentStopReason(),
				)
			} else if nativeErr := s.native.Err(); nativeErr != nil {
				raw, rawErr := rawErrorPayload(nativeErr)
				if rawErr != nil {
					s.fail(protocolError(
						"generate_content.stream_error_audit",
						"failed to construct controlled Gemini stream error payload",
						s.requestID(),
					), modelinvoker.RawPayload{})
					break
				}
				s.captureRaw(raw)
				s.fail(normalizeFailure(s.ctx, s.base, s.request, "generate_content.stream", s.headers, nativeErr), raw)
			} else {
				s.fail(driverError(
					modelinvoker.ErrorStreamInterrupted,
					"generate_content.stream",
					"Gemini stream ended without a terminal finish reason",
				), modelinvoker.RawPayload{})
			}
			break
		}
		s.mapChunk(s.native.Current())
	}
	if len(s.queue) == 0 {
		return false
	}
	s.current = s.queue[0]
	s.queue = s.queue[1:]
	return true
}

func (s *generateContentStream) mapChunk(native *genai.GenerateContentResponse) {
	if native == nil {
		s.ensureStarted(modelinvoker.RawPayload{})
		s.fail(protocolError("generate_content.stream", "Gemini stream returned a nil response chunk", s.requestID()), modelinvoker.RawPayload{})
		return
	}
	raw, rawErr := rawGenerateContentPayload(native)
	if rawErr != nil {
		s.ensureStarted(modelinvoker.RawPayload{})
		s.fail(protocolError(
			"generate_content.stream_raw",
			"failed to construct controlled Gemini stream payload",
			s.requestID(),
		), modelinvoker.RawPayload{})
		return
	}
	s.captureRaw(raw)
	s.chunkOrdinal++
	if err := s.captureIdentity(native); err != nil {
		s.ensureStarted(raw)
		s.fail(err, raw)
		return
	}
	s.ensureStarted(raw)

	if native.PromptFeedback != nil {
		s.promptFeedback = native.PromptFeedback
	}
	if reason := blockedReason(native.PromptFeedback); reason != "" {
		message := strings.TrimSpace(native.PromptFeedback.BlockReasonMessage)
		if message == "" {
			message = "Gemini rejected the prompt under provider policy"
		}
		s.finishReason = genai.FinishReasonSafety
		s.fail(&modelinvoker.Error{
			Kind: modelinvoker.ErrorPolicyRejected, Provider: s.base.Binding().Provider, Operation: "generate_content.stream",
			Code: reason, Message: message, RequestID: s.requestID(),
		}, raw)
		return
	}
	if len(native.Candidates) > 1 {
		s.fail(mappingErrorWithRequestID(
			"generate_content.stream",
			"Gemini multiple candidates are outside the unified GenerateContent slice",
			s.requestID(),
		), raw)
		return
	}

	var candidate *genai.Candidate
	if len(native.Candidates) == 1 {
		candidate = native.Candidates[0]
		if candidate == nil {
			s.fail(protocolError("generate_content.stream", "Gemini stream candidate is nil", s.requestID()), raw)
			return
		}
		if candidate.Index != 0 {
			s.fail(mappingErrorWithRequestID(
				"generate_content.stream",
				fmt.Sprintf("Gemini candidate index %d is outside the single-candidate slice", candidate.Index),
				s.requestID(),
			), raw)
			return
		}
		if err := s.mapCandidateContent(candidate, raw); err != nil {
			s.fail(err, raw)
			return
		}
	}

	if native.UsageMetadata != nil {
		usage, calculated := normalizeUsage(native.UsageMetadata)
		changed := !s.usageSeen || usage != s.usage
		s.usage = usage
		s.usageMetadata = native.UsageMetadata
		s.usageSeen = true
		if calculated && !s.usageFallback {
			s.usageFallback = true
			s.decisions = append(s.decisions, transformation(
				modelinvoker.CapabilityUsageReporting,
				"Gemini omitted totalTokenCount; total calculated from prompt, tool-use prompt, candidate, and thought counts",
			))
		}
		if changed {
			usageCopy := usage
			s.enqueue(modelinvoker.StreamEvent{
				Type: modelinvoker.StreamEventUsage, ResponseID: s.responseID, Usage: &usageCopy, Raw: raw,
			})
		}
	}

	if candidate == nil || candidate.FinishReason == "" || candidate.FinishReason == genai.FinishReasonUnspecified {
		return
	}
	s.finishReason = candidate.FinishReason
	s.finishMessage = candidate.FinishMessage
	if !knownFinishReason(candidate.FinishReason) && s.request.AllowDegradation {
		s.decisions = append(s.decisions, degradation(
			modelinvoker.CapabilityTextGeneration,
			fmt.Sprintf("unknown Gemini finish reason %q mapped to incomplete", candidate.FinishReason),
		))
		s.complete(modelinvoker.ResponseStatusIncomplete, modelinvoker.StopReasonOther, raw)
		return
	}
	status, stopReason, finishErr := normalizeFinishReason(
		candidate.FinishReason,
		candidate.FinishMessage,
		s.hasFunctionCall,
		s.requestID(),
	)
	if finishErr != nil {
		var invocationError *modelinvoker.Error
		if candidateError, ok := finishErr.(*modelinvoker.Error); ok && candidateError != nil {
			copy := *candidateError
			copy.Operation = "generate_content.stream"
			invocationError = &copy
		}
		if invocationError == nil {
			invocationError = driverError(modelinvoker.ErrorProvider, "generate_content.stream", "Gemini stream finish reason failed")
		}
		s.failWithStatus(invocationError, raw, status, stopReason)
		return
	}
	s.complete(status, stopReason, raw)
}

func (s *generateContentStream) captureIdentity(native *genai.GenerateContentResponse) error {
	if native.ResponseID != "" {
		if s.responseID != "" && s.responseID != native.ResponseID {
			return protocolError(
				"generate_content.stream",
				fmt.Sprintf("Gemini stream response ID changed from %q to %q", s.responseID, native.ResponseID),
				s.requestID(),
			)
		}
		s.responseID = native.ResponseID
	}
	if native.ModelVersion != "" {
		if s.modelVersion != "" && s.modelVersion != native.ModelVersion {
			return protocolError(
				"generate_content.stream",
				fmt.Sprintf("Gemini stream model version changed from %q to %q", s.modelVersion, native.ModelVersion),
				s.requestID(),
			)
		}
		s.modelVersion = native.ModelVersion
	}
	return nil
}

func (s *generateContentStream) mapCandidateContent(candidate *genai.Candidate, raw modelinvoker.RawPayload) error {
	if candidate.Content == nil {
		return nil
	}
	if candidate.Content.Role != genai.RoleModel {
		return protocolError(
			"generate_content.stream",
			fmt.Sprintf("Gemini stream candidate has invalid role %q", candidate.Content.Role),
			s.requestID(),
		)
	}
	idLessOrdinal := 0
	terminalSnapshot := candidate.FinishReason != "" && candidate.FinishReason != genai.FinishReasonUnspecified
	for _, part := range candidate.Content.Parts {
		if part == nil {
			return protocolError("generate_content.stream", "Gemini stream candidate contains a nil part", s.requestID())
		}
		unsupported := unsupportedPartDescription(part)
		if unsupported != "" {
			if !s.request.AllowDegradation {
				return mappingErrorWithRequestID("generate_content.stream", unsupported+" is outside the unified Gemini slice", s.requestID())
			}
			s.decisions = append(s.decisions, degradation(
				modelinvoker.CapabilityTextGeneration,
				unsupported+" retained only in NativeEvents",
			))
			continue
		}
		if err := validateContinuationPart(genai.RoleModel, part); err != nil {
			return protocolError("generate_content.stream", "Gemini stream part is invalid: "+err.Error(), s.requestID())
		}
		partIndex := len(s.modelParts)
		if part.FunctionCall != nil {
			var idLessPosition idLessCallPosition
			var idLessFingerprint string
			if part.FunctionCall.ID == "" {
				idLessPosition = idLessCallPosition{candidate: int(candidate.Index), ordinal: idLessOrdinal}
				idLessOrdinal++
				var err error
				idLessFingerprint, err = functionCallFingerprint(part.FunctionCall)
				if err != nil {
					return protocolError("generate_content.stream", "Gemini function call arguments cannot be fingerprinted", s.requestID())
				}
				if previous, exists := s.seenIDLessCalls[idLessPosition]; terminalSnapshot && exists &&
					previous.chunk < s.chunkOrdinal && previous.fingerprint == idLessFingerprint {
					if err := s.mergeRepeatedCallSignature(previous.modelPartIndex, part.ThoughtSignature); err != nil {
						return err
					}
					previous.chunk = s.chunkOrdinal
					s.seenIDLessCalls[idLessPosition] = previous
					continue
				}
			}
			call, err := normalizeFunctionCall(&s.envelope, s.responseID, int(candidate.Index), partIndex, part.FunctionCall)
			if err != nil {
				return mappingErrorWithRequestID("generate_content.stream", err.Error(), s.requestID())
			}
			arguments := string(call.Arguments)
			if part.FunctionCall.ID != "" {
				if previous, exists := s.seenNativeCalls[part.FunctionCall.ID]; exists {
					if previous.arguments == arguments {
						if err := s.mergeRepeatedCallSignature(previous.modelPartIndex, part.ThoughtSignature); err != nil {
							return err
						}
						// Some transports repeat a complete function-call snapshot.
						// It is preserved in NativeEvents but not emitted twice.
						continue
					}
					return protocolError("generate_content.stream", "Gemini reused a function call ID with different arguments", s.requestID())
				}
			}
			if err := s.appendModelPart(part); err != nil {
				return err
			}
			if part.FunctionCall.ID != "" {
				s.seenNativeCalls[part.FunctionCall.ID] = nativeCallSnapshot{
					arguments: arguments, modelPartIndex: len(s.modelParts) - 1,
				}
			} else {
				s.seenIDLessCalls[idLessPosition] = idLessCallSnapshot{
					chunk: s.chunkOrdinal, fingerprint: idLessFingerprint, modelPartIndex: len(s.modelParts) - 1,
				}
			}
			s.hasFunctionCall = true
			s.output = append(s.output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: cloneSemanticCall(call)})
			s.enqueue(modelinvoker.StreamEvent{
				Type: modelinvoker.StreamEventFunctionCallStarted, ResponseID: s.responseID,
				FunctionCall: cloneSemanticCall(&modelinvoker.FunctionCall{ID: call.ID, Name: call.Name, Arguments: json.RawMessage(`{}`)}), Raw: raw,
			})
			s.enqueue(modelinvoker.StreamEvent{
				Type: modelinvoker.StreamEventFunctionArgumentsDelta, ResponseID: s.responseID,
				ArgumentsDelta: arguments, FunctionCall: cloneSemanticCall(call), Raw: raw,
			})
			s.enqueue(modelinvoker.StreamEvent{
				Type: modelinvoker.StreamEventFunctionCallCompleted, ResponseID: s.responseID,
				FunctionCall: cloneSemanticCall(call), Raw: raw,
			})
			continue
		}
		if part.Thought {
			if err := s.appendModelPart(part); err != nil {
				return err
			}
			if part.Text != "" {
				s.enqueue(modelinvoker.StreamEvent{
					Type: modelinvoker.StreamEventReasoningDelta, ResponseID: s.responseID,
					ReasoningDelta: part.Text, Raw: raw,
				})
			}
			continue
		}
		if part.Text != "" {
			if err := s.appendModelPart(part); err != nil {
				return err
			}
			s.output = append(s.output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemText, Text: part.Text})
			s.enqueue(modelinvoker.StreamEvent{
				Type: modelinvoker.StreamEventTextDelta, ResponseID: s.responseID,
				TextDelta: part.Text, Raw: raw,
			})
			continue
		}
		if len(part.ThoughtSignature) > 0 {
			if err := s.appendModelPart(part); err != nil {
				return err
			}
		}
	}
	return nil
}

func functionCallFingerprint(call *genai.FunctionCall) (string, error) {
	if call == nil {
		return "", fmt.Errorf("function call is nil")
	}
	arguments := call.Args
	if arguments == nil {
		arguments = map[string]any{}
	}
	raw, err := json.Marshal(arguments)
	if err != nil {
		return "", err
	}
	return call.Name + "\x00" + string(raw), nil
}

func (s *generateContentStream) mergeRepeatedCallSignature(modelPartIndex int, signature []byte) error {
	if modelPartIndex < 0 || modelPartIndex >= len(s.modelParts) || s.modelParts[modelPartIndex] == nil {
		return protocolError("generate_content.stream", "repeated Gemini function call references an invalid prior part", s.requestID())
	}
	if len(signature) == 0 {
		return nil
	}
	existing := s.modelParts[modelPartIndex].ThoughtSignature
	if len(existing) > 0 && !bytes.Equal(existing, signature) {
		return protocolError("generate_content.stream", "repeated Gemini function call changed its thought signature", s.requestID())
	}
	if len(existing) == 0 {
		s.modelParts[modelPartIndex].ThoughtSignature = append([]byte(nil), signature...)
	}
	return nil
}

func (s *generateContentStream) appendModelPart(part *genai.Part) error {
	cloned, err := cloneContent(&genai.Content{Role: genai.RoleModel, Parts: []*genai.Part{part}})
	if err != nil || cloned == nil || len(cloned.Parts) != 1 {
		return protocolError("generate_content.stream", "failed to clone Gemini stream part", s.requestID())
	}
	s.modelParts = append(s.modelParts, cloned.Parts[0])
	return nil
}

func (s *generateContentStream) ensureStarted(raw modelinvoker.RawPayload) {
	if s.started {
		return
	}
	s.started = true
	response, _ := s.buildResponse(modelinvoker.ResponseStatusInProgress, "")
	s.enqueue(modelinvoker.StreamEvent{
		Type: modelinvoker.StreamEventResponseStarted, ResponseID: s.responseID, Response: &response, Raw: raw,
	})
}

func (s *generateContentStream) complete(status modelinvoker.ResponseStatus, stopReason modelinvoker.StopReason, raw modelinvoker.RawPayload) {
	response, err := s.buildResponse(status, stopReason)
	if err != nil {
		s.fail(mappingErrorWithRequestID("generate_content.continuation", err.Error(), s.requestID()), raw)
		return
	}
	s.terminal = true
	s.enqueue(modelinvoker.StreamEvent{
		Type: modelinvoker.StreamEventResponseCompleted, ResponseID: s.responseID, Response: &response, Raw: raw,
	})
}

func (s *generateContentStream) fail(err error, raw modelinvoker.RawPayload) {
	s.failWithStatus(err, raw, modelinvoker.ResponseStatusFailed, s.currentStopReason())
}

func (s *generateContentStream) failWithStatus(
	err error,
	raw modelinvoker.RawPayload,
	status modelinvoker.ResponseStatus,
	stopReason modelinvoker.StopReason,
) {
	if s.terminal {
		return
	}
	var invocationError *modelinvoker.Error
	if existing, ok := err.(*modelinvoker.Error); ok && existing != nil {
		copy := *existing
		invocationError = &copy
	} else {
		invocationError = driverError(modelinvoker.ErrorStreamInterrupted, "generate_content.stream", "Gemini stream failed")
	}
	if invocationError.Provider == "" {
		invocationError.Provider = s.base.Binding().Provider
	}
	if invocationError.Operation == "" {
		invocationError.Operation = "generate_content.stream"
	}
	if invocationError.RequestID == "" {
		invocationError.RequestID = s.requestID()
	}
	response, responseErr := s.buildResponse(status, stopReason)
	if responseErr != nil {
		response = s.minimalResponse(status, stopReason)
	}
	s.err = invocationError
	s.terminal = true
	s.enqueue(modelinvoker.StreamEvent{
		Type: modelinvoker.StreamEventError, ResponseID: s.responseID,
		Response: &response, Error: invocationError, Raw: raw,
	})
}

func (s *generateContentStream) buildResponse(status modelinvoker.ResponseStatus, stopReason modelinvoker.StopReason) (modelinvoker.Response, error) {
	response := s.minimalResponse(status, stopReason)
	if status == modelinvoker.ResponseStatusInProgress && len(s.modelParts) == 0 {
		return response, nil
	}
	var content *genai.Content
	if len(s.modelParts) > 0 {
		content = &genai.Content{Role: genai.RoleModel, Parts: s.modelParts}
	}
	if !continuationEnvelopeRequiresState(s.envelope) && !contentRequiresContinuation(content) {
		return response, nil
	}
	envelope := s.envelope
	var err error
	envelope.Contents, err = cloneContents(s.envelope.Contents)
	if err != nil {
		return response, fmt.Errorf("clone Gemini continuation history: %w", err)
	}
	if content != nil {
		cloned, cloneErr := cloneContent(content)
		if cloneErr != nil {
			return response, fmt.Errorf("clone Gemini stream continuation content: %w", cloneErr)
		}
		envelope.Contents = append(envelope.Contents, cloned)
	}
	state, err := encodeContinuation(s.base.Binding(), envelope, s.responseID)
	if err != nil {
		return response, err
	}
	response.State = state
	return response, nil
}

func (s *generateContentStream) minimalResponse(status modelinvoker.ResponseStatus, stopReason modelinvoker.StopReason) modelinvoker.Response {
	metadata := s.base.ProviderMetadata(s.headers)
	if s.modelVersion != "" {
		metadata["model_version"] = s.modelVersion
	}
	if s.finishReason != "" {
		metadata["finish_reason"] = string(s.finishReason)
	}
	if s.finishMessage != "" {
		metadata["finish_message"] = s.finishMessage
	}
	if reason := blockedReason(s.promptFeedback); reason != "" {
		metadata["prompt_block_reason"] = reason
	}
	if s.usageMetadata != nil {
		metadata["prompt_token_count"] = fmt.Sprintf("%d", s.usageMetadata.PromptTokenCount)
		metadata["tool_use_prompt_token_count"] = fmt.Sprintf("%d", s.usageMetadata.ToolUsePromptTokenCount)
		metadata["candidate_token_count"] = fmt.Sprintf("%d", s.usageMetadata.CandidatesTokenCount)
		metadata["thought_token_count"] = fmt.Sprintf("%d", s.usageMetadata.ThoughtsTokenCount)
		metadata["cached_content_token_count"] = fmt.Sprintf("%d", s.usageMetadata.CachedContentTokenCount)
	}
	return modelinvoker.Response{
		ID:               s.responseID,
		Provider:         s.base.Binding().Provider,
		Protocol:         s.base.Binding().Protocol,
		Model:            s.request.Model,
		Status:           status,
		StopReason:       stopReason,
		Output:           cloneOutput(s.output),
		Usage:            s.usage,
		RequestID:        s.requestID(),
		ProviderMetadata: metadata,
		RawRequest:       s.rawRequest,
		RawResponse:      modelinvoker.NewRawPayload(s.rawStream.Bytes()),
		NativeEvents:     append([]modelinvoker.RawPayload(nil), s.nativeEvents...),
		MappingReport: modelinvoker.MappingReport{
			Provider:  s.base.Binding().Provider,
			Protocol:  s.base.Binding().Protocol,
			Endpoint:  s.base.Binding().EffectiveEndpoint(s.request.Endpoint),
			Decisions: append([]modelinvoker.MappingDecision(nil), s.decisions...),
		},
	}
}

func (s *generateContentStream) currentStopReason() modelinvoker.StopReason {
	if s.finishReason == "" || s.finishReason == genai.FinishReasonUnspecified {
		return modelinvoker.StopReasonOther
	}
	_, stop, _ := normalizeFinishReason(s.finishReason, s.finishMessage, s.hasFunctionCall, s.requestID())
	return stop
}

func (s *generateContentStream) requestID() string {
	if s == nil {
		return ""
	}
	return s.base.RequestID(s.headers)
}

func (s *generateContentStream) captureRaw(raw modelinvoker.RawPayload) {
	if raw.Empty() {
		return
	}
	s.nativeEvents = append(s.nativeEvents, raw)
	s.rawStream.Write(raw.Bytes())
	s.rawStream.WriteByte('\n')
}

func (s *generateContentStream) enqueue(event modelinvoker.StreamEvent) {
	s.sequence++
	event.Sequence = s.sequence
	if event.ResponseID == "" {
		event.ResponseID = s.responseID
	}
	s.queue = append(s.queue, event)
}

func (s *generateContentStream) Event() modelinvoker.StreamEvent {
	if s == nil {
		return modelinvoker.StreamEvent{}
	}
	return s.current
}

func (s *generateContentStream) Err() error {
	if s == nil {
		return nil
	}
	return s.err
}

func (s *generateContentStream) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	if s.native == nil {
		return nil
	}
	if err := s.native.Close(); err != nil {
		return normalizeFailure(s.ctx, s.base, s.request, "generate_content.stream_close", s.headers, err)
	}
	return nil
}

func cloneSemanticCall(call *modelinvoker.FunctionCall) *modelinvoker.FunctionCall {
	if call == nil {
		return nil
	}
	copy := *call
	copy.Arguments = append(json.RawMessage(nil), call.Arguments...)
	return &copy
}

func cloneOutput(output []modelinvoker.OutputItem) []modelinvoker.OutputItem {
	result := append([]modelinvoker.OutputItem(nil), output...)
	for index := range result {
		result[index].FunctionCall = cloneSemanticCall(result[index].FunctionCall)
	}
	return result
}

func knownFinishReason(reason genai.FinishReason) bool {
	switch reason {
	case genai.FinishReasonStop,
		genai.FinishReasonMaxTokens,
		genai.FinishReasonSafety,
		genai.FinishReasonRecitation,
		genai.FinishReasonLanguage,
		genai.FinishReasonOther,
		genai.FinishReasonBlocklist,
		genai.FinishReasonProhibitedContent,
		genai.FinishReasonSPII,
		genai.FinishReasonMalformedFunctionCall,
		genai.FinishReasonImageSafety,
		genai.FinishReasonUnexpectedToolCall,
		genai.FinishReasonImageProhibitedContent,
		genai.FinishReasonNoImage,
		genai.FinishReasonImageRecitation,
		genai.FinishReasonImageOther:
		return true
	default:
		return false
	}
}

var _ modelinvoker.Stream = (*generateContentStream)(nil)
