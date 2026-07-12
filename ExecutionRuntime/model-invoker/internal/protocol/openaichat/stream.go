package openaichat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	openaisdk "github.com/openai/openai-go/v3"
)

type toolState struct {
	call    modelinvoker.FunctionCall
	started bool
	done    bool
}

type stream struct {
	ctx            context.Context
	base           *protocol.Base
	request        modelinvoker.Request
	native         EventStream
	headers        http.Header
	rawRequest     modelinvoker.RawPayload
	decisions      []modelinvoker.MappingDecision
	queue          []modelinvoker.StreamEvent
	current        modelinvoker.StreamEvent
	err            error
	closed         bool
	terminal       bool
	pendingStatus  modelinvoker.ResponseStatus
	stopReason     modelinvoker.StopReason
	started        bool
	responseID     string
	model          string
	text           bytes.Buffer
	reasoning      bytes.Buffer
	usage          modelinvoker.Usage
	tools          map[int64]*toolState
	nativeEvents   []modelinvoker.RawPayload
	rawStream      bytes.Buffer
	sequence       int64
	extension      StreamMapper
	deltaMapper    StreamDeltaMapper
	finishMapper   FinishReasonMapper
	metadataMapper StreamMetadataMapper
	requestID      string
	metadata       modelinvoker.ProviderMetadata
	closeOnce      sync.Once
	closeErr       error
}

func newStream(
	ctx context.Context,
	base *protocol.Base,
	request modelinvoker.Request,
	native EventStream,
	headers http.Header,
	rawRequest modelinvoker.RawPayload,
	decisions []modelinvoker.MappingDecision,
	extension StreamMapper,
	deltaMapper StreamDeltaMapper,
	finishMapper FinishReasonMapper,
	metadataMapper StreamMetadataMapper,
) modelinvoker.Stream {
	return &stream{
		ctx: ctx, base: base, request: request, native: native, headers: headers, rawRequest: rawRequest,
		decisions: append([]modelinvoker.MappingDecision(nil), decisions...), tools: make(map[int64]*toolState), extension: extension, deltaMapper: deltaMapper, finishMapper: finishMapper, metadataMapper: metadataMapper,
	}
}

func (s *stream) Next() bool {
	if s == nil || s.closed || (s.terminal && len(s.queue) == 0) {
		return false
	}
	for len(s.queue) == 0 && s.err == nil {
		if !s.native.Next() {
			if err := s.native.Err(); err != nil {
				raw := s.captureStreamError(err)
				s.fail(normalizeFailure(s.ctx, s.base, s.request, "chat_completions.stream", s.headers, err), raw)
			} else if s.pendingStatus != "" && !s.terminal {
				s.complete(s.pendingStatus, modelinvoker.StreamEvent{ResponseID: s.responseID})
			} else if !s.terminal {
				s.fail(driverError(modelinvoker.ErrorStreamInterrupted, "chat_completions.stream", "stream ended without a terminal chunk"), modelinvoker.RawPayload{})
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

func (s *stream) captureStreamError(err error) modelinvoker.RawPayload {
	raw := streamErrorPayload(err)
	if raw.Empty() {
		return raw
	}
	s.nativeEvents = append(s.nativeEvents, raw)
	s.rawStream.Write(raw.Bytes())
	s.rawStream.WriteByte('\n')
	return raw
}

func (s *stream) mapChunk(chunk openaisdk.ChatCompletionChunk) {
	if err := s.base.VerifyResponseModel(s.request, chunk.Model, "chat_completions.stream_model"); err != nil {
		s.fail(err, modelinvoker.RawPayload{})
		return
	}
	raw := modelinvoker.NewRawPayload([]byte(chunk.RawJSON()))
	s.nativeEvents = append(s.nativeEvents, raw)
	if chunk.RawJSON() != "" {
		s.rawStream.WriteString(chunk.RawJSON())
		s.rawStream.WriteByte('\n')
	}
	if chunk.ID != "" {
		s.responseID = chunk.ID
	}
	s.model = chunk.Model
	if s.metadataMapper != nil {
		mapped, err := s.metadataMapper.MapChatStreamMetadata(s.request, chunk)
		if err != nil {
			s.fail(err, raw)
			return
		}
		if mapped.RequestID != "" {
			s.requestID = mapped.RequestID
		}
		if len(mapped.ProviderMetadata) > 0 {
			if s.metadata == nil {
				s.metadata = make(modelinvoker.ProviderMetadata)
			}
			for key, value := range mapped.ProviderMetadata {
				s.metadata[key] = value
			}
		}
	}
	baseEvent := modelinvoker.StreamEvent{ResponseID: s.responseID, Raw: raw}
	if !s.started && s.responseID != "" {
		s.started = true
		started := baseEvent
		started.Type = modelinvoker.StreamEventResponseStarted
		s.enqueue(started)
	}
	if chunk.JSON.Usage.Valid() {
		s.usage = chatUsage(chunk.Usage)
		usage := s.usage
		usageEvent := baseEvent
		usageEvent.Type = modelinvoker.StreamEventUsage
		usageEvent.Usage = &usage
		s.enqueue(usageEvent)
	}
	if len(chunk.Choices) == 0 && s.pendingStatus != "" && !s.terminal {
		s.complete(s.pendingStatus, baseEvent)
		return
	}
	for _, choice := range chunk.Choices {
		if choice.Index != 0 {
			if !s.request.AllowDegradation {
				s.fail(mappingError("chat_completions.stream", "multiple streamed choices require degradation permission"), raw)
				return
			}
			s.recordDegradation(modelinvoker.CapabilityTextGeneration, "streamed choices after index 0 retained only in NativeEvents")
			continue
		}
		if choice.Delta.Refusal != "" || choice.FinishReason == "content_filter" {
			s.stopReason = modelinvoker.StopReasonContentFilter
			message := choice.Delta.Refusal
			if message == "" {
				message = "content was rejected by provider policy"
			}
			s.fail(&modelinvoker.Error{
				Kind: modelinvoker.ErrorPolicyRejected, Operation: "chat_completions.stream",
				Code: choice.FinishReason, Message: message, RequestID: s.requestIDValue(),
			}, raw)
			return
		}
		textDelta := choice.Delta.Content
		if s.deltaMapper != nil {
			mappedText, mappedReasoning, extensionDecisions, err := s.deltaMapper.MapChatStreamDelta(s.request, choice, s.text.String(), s.reasoning.String())
			if err != nil {
				s.fail(err, raw)
				return
			}
			for _, decision := range extensionDecisions {
				s.recordDegradation(decision.Capability, decision.Detail)
			}
			textDelta = mappedText
			if mappedReasoning != "" {
				s.reasoning.WriteString(mappedReasoning)
				reasoningEvent := baseEvent
				reasoningEvent.Type = modelinvoker.StreamEventReasoningDelta
				reasoningEvent.ReasoningDelta = mappedReasoning
				s.enqueue(reasoningEvent)
			}
		} else if s.extension != nil {
			delta, extensionDecisions, err := s.extension.MapChatChunk(s.request, choice)
			if err != nil {
				s.fail(err, raw)
				return
			}
			for _, decision := range extensionDecisions {
				s.recordDegradation(decision.Capability, decision.Detail)
			}
			if delta != "" {
				s.reasoning.WriteString(delta)
				reasoningEvent := baseEvent
				reasoningEvent.Type = modelinvoker.StreamEventReasoningDelta
				reasoningEvent.ReasoningDelta = delta
				s.enqueue(reasoningEvent)
			}
		}
		if textDelta != "" {
			s.text.WriteString(textDelta)
			textEvent := baseEvent
			textEvent.Type = modelinvoker.StreamEventTextDelta
			textEvent.TextDelta = textDelta
			s.enqueue(textEvent)
		}
		for _, delta := range choice.Delta.ToolCalls {
			state := s.tools[delta.Index]
			if state == nil {
				state = &toolState{}
				s.tools[delta.Index] = state
			}
			if delta.ID != "" {
				state.call.ID = delta.ID
			}
			if delta.Function.Name != "" {
				state.call.Name += delta.Function.Name
			}
			if !state.started && (state.call.ID != "" || state.call.Name != "") {
				state.started = true
				started := baseEvent
				started.Type = modelinvoker.StreamEventFunctionCallStarted
				started.FunctionCall = cloneCall(state.call)
				s.enqueue(started)
			}
			if delta.Function.Arguments != "" {
				state.call.Arguments = append(state.call.Arguments, delta.Function.Arguments...)
				arguments := baseEvent
				arguments.Type = modelinvoker.StreamEventFunctionArgumentsDelta
				arguments.ArgumentsDelta = delta.Function.Arguments
				arguments.FunctionCall = cloneCall(state.call)
				s.enqueue(arguments)
			}
		}
		if s.finishMapper != nil && choice.FinishReason != "" {
			if mapping, handled := s.finishMapper.MapChatFinishReason(s.request, choice.FinishReason); handled {
				if mapping.Error != nil {
					s.fail(mapping.Error, raw)
					return
				}
				s.pendingStatus = mapping.Status
				s.stopReason = mapping.StopReason
				continue
			}
		}
		switch choice.FinishReason {
		case "":
		case "tool_calls":
			if !s.completeTools(baseEvent) {
				return
			}
			s.pendingStatus = modelinvoker.ResponseStatusCompleted
			s.stopReason = modelinvoker.StopReasonToolCall
		case "stop":
			if len(s.tools) > 0 {
				if !s.request.AllowDegradation {
					s.fail(mappingErrorWithRequestID("chat_completions.stream", "tool deltas ended with stop instead of tool_calls", s.requestIDValue()), raw)
					return
				}
				if !s.completeTools(baseEvent) {
					return
				}
				s.recordDegradation(modelinvoker.CapabilityToolCalling, "tool deltas ended with stop instead of tool_calls")
			}
			s.pendingStatus = modelinvoker.ResponseStatusCompleted
			s.stopReason = modelinvoker.StopReasonEndTurn
		case "length":
			if len(s.tools) > 0 {
				if !s.request.AllowDegradation {
					s.fail(mappingErrorWithRequestID("chat_completions.stream", "incomplete tool calls require degradation permission", s.requestIDValue()), raw)
					return
				}
				s.recordDegradation(modelinvoker.CapabilityToolCalling, "incomplete tool calls retained only in NativeEvents")
			}
			s.pendingStatus = modelinvoker.ResponseStatusIncomplete
			s.stopReason = modelinvoker.StopReasonMaxOutputTokens
		default:
			if !s.request.AllowDegradation {
				s.fail(mappingError("chat_completions.stream", fmt.Sprintf("unsupported finish reason %q", choice.FinishReason)), raw)
				return
			}
			s.recordDegradation(modelinvoker.CapabilityTextGeneration,
				fmt.Sprintf("finish reason %q retained only in NativeEvents", choice.FinishReason))
			s.pendingStatus = modelinvoker.ResponseStatusCompleted
			s.stopReason = modelinvoker.StopReasonOther
		}
	}
}

func (s *stream) enqueue(event modelinvoker.StreamEvent) {
	s.sequence++
	event.Sequence = s.sequence
	s.queue = append(s.queue, event)
}

func (s *stream) recordDegradation(capability modelinvoker.Capability, detail string) {
	for _, decision := range s.decisions {
		if decision.Capability == capability && decision.Action == modelinvoker.MappingDegraded && decision.Detail == detail {
			return
		}
	}
	s.decisions = append(s.decisions, degradation(capability, detail))
}

func (s *stream) completeTools(baseEvent modelinvoker.StreamEvent) bool {
	indices := make([]int, 0, len(s.tools))
	for index := range s.tools {
		indices = append(indices, int(index))
	}
	sort.Ints(indices)
	for _, rawIndex := range indices {
		state := s.tools[int64(rawIndex)]
		if state.done {
			continue
		}
		if !state.started || !validFunctionCall(state.call) {
			s.fail(mappingErrorWithRequestID("chat_completions.stream", "completed function call requires an ID, valid name, and JSON object arguments", s.requestIDValue()), baseEvent.Raw)
			return false
		}
		state.done = true
		event := baseEvent
		event.Type = modelinvoker.StreamEventFunctionCallCompleted
		event.FunctionCall = cloneCall(state.call)
		s.enqueue(event)
	}
	return true
}

func (s *stream) complete(status modelinvoker.ResponseStatus, baseEvent modelinvoker.StreamEvent) {
	if s.terminal {
		return
	}
	s.terminal = true
	response := modelinvoker.Response{
		ID: s.responseID, Protocol: modelinvoker.ProtocolChatCompletions, Model: s.model,
		Status: status, StopReason: s.stopReason, Usage: s.usage, RequestID: s.requestIDValue(),
		ProviderMetadata: s.providerMetadataValue(), RawRequest: s.rawRequest,
		RawResponse: modelinvoker.NewRawPayload(s.rawStream.Bytes()), NativeEvents: append([]modelinvoker.RawPayload(nil), s.nativeEvents...),
		MappingReport: modelinvoker.MappingReport{
			Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: s.request.Endpoint,
			Decisions: append([]modelinvoker.MappingDecision(nil), s.decisions...),
		},
	}
	if s.text.Len() > 0 {
		response.Output = append(response.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemText, Text: s.text.String()})
	}
	if s.reasoning.Len() > 0 {
		response.Output = append(response.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemReasoningSummary, ReasoningSummary: s.reasoning.String()})
	}
	indices := make([]int, 0, len(s.tools))
	for index := range s.tools {
		indices = append(indices, int(index))
	}
	sort.Ints(indices)
	for _, rawIndex := range indices {
		state := s.tools[int64(rawIndex)]
		if state.done {
			response.Output = append(response.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: cloneCall(state.call)})
		}
	}
	baseEvent.Type = modelinvoker.StreamEventResponseCompleted
	baseEvent.Response = &response
	s.enqueue(baseEvent)
}

func (s *stream) fail(err error, raw modelinvoker.RawPayload) {
	var invocationError *modelinvoker.Error
	if candidate, ok := err.(*modelinvoker.Error); ok && candidate != nil {
		copy := *candidate
		invocationError = &copy
	} else {
		invocationError = driverError(modelinvoker.ErrorStreamInterrupted, "chat_completions.stream", "stream failed")
	}
	if invocationError.RequestID == "" {
		invocationError.RequestID = s.requestIDValue()
	}
	s.terminal = true
	s.err = invocationError
	if invocationError.Code == "response_model_missing" || invocationError.Code == "response_model_mismatch" {
		s.enqueue(modelinvoker.StreamEvent{Type: modelinvoker.StreamEventError, ResponseID: s.responseID, Error: invocationError})
		s.closeNative()
		s.err = errors.Join(invocationError, adaptercore.SafeCloseError(s.base.Binding().Provider, "chat_completions.stream_close", s.closeErr))
		return
	}
	s.enqueue(modelinvoker.StreamEvent{
		Type: modelinvoker.StreamEventError, ResponseID: s.responseID,
		Response: s.partialResponse(), Error: invocationError, Raw: raw,
	})
}

func (s *stream) partialResponse() *modelinvoker.Response {
	response := &modelinvoker.Response{
		ID: s.responseID, Protocol: modelinvoker.ProtocolChatCompletions, Model: s.model,
		Status: modelinvoker.ResponseStatusFailed, StopReason: s.stopReason, Usage: s.usage,
		RequestID: s.requestIDValue(), ProviderMetadata: s.providerMetadataValue(), RawRequest: s.rawRequest,
		RawResponse: modelinvoker.NewRawPayload(s.rawStream.Bytes()), NativeEvents: append([]modelinvoker.RawPayload(nil), s.nativeEvents...),
		MappingReport: modelinvoker.MappingReport{
			Protocol: modelinvoker.ProtocolChatCompletions, Endpoint: s.request.Endpoint,
			Decisions: append([]modelinvoker.MappingDecision(nil), s.decisions...),
		},
	}
	if s.text.Len() > 0 {
		response.Output = append(response.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemText, Text: s.text.String()})
	}
	if s.reasoning.Len() > 0 {
		response.Output = append(response.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemReasoningSummary, ReasoningSummary: s.reasoning.String()})
	}
	return response
}

func (s *stream) requestIDValue() string {
	if s.requestID != "" {
		return s.requestID
	}
	return s.base.RequestID(s.headers)
}

func (s *stream) providerMetadataValue() modelinvoker.ProviderMetadata {
	result := s.base.ProviderMetadata(s.headers)
	if result == nil {
		result = make(modelinvoker.ProviderMetadata)
	}
	for key, value := range s.metadata {
		result[key] = value
	}
	return result
}

func (s *stream) Event() modelinvoker.StreamEvent {
	if s == nil {
		return modelinvoker.StreamEvent{}
	}
	return s.current
}

func (s *stream) Err() error {
	if s == nil {
		return nil
	}
	return s.err
}

func (s *stream) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	return adaptercore.SafeCloseError(s.base.Binding().Provider, "chat_completions.stream_close", s.closeNative())
}

func (s *stream) closeNative() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		if s.native != nil {
			s.closeErr = s.native.Close()
			s.native = nil
		}
	})
	return s.closeErr
}

var _ modelinvoker.Stream = (*stream)(nil)
