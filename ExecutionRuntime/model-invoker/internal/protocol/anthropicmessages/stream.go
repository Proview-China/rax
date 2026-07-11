package anthropicmessages

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/adaptercore"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

type pendingToolCall struct {
	id        string
	name      string
	arguments []byte
}

type streamContentBlock struct {
	kind   string
	closed bool
	tool   *pendingToolCall
}

type messageStream struct {
	ctx                context.Context
	request            modelinvoker.Request
	base               *protocol.Base
	native             EventStream
	headers            http.Header
	rawRequest         modelinvoker.RawPayload
	decisions          []modelinvoker.MappingDecision
	continuationMapper ContinuationMapper
	stopReasonMapper   StopReasonMapper

	accumulator  anthropicsdk.Message
	blocks       map[int64]*streamContentBlock
	nativeEvents []modelinvoker.RawPayload
	queue        []modelinvoker.StreamEvent
	current      modelinvoker.StreamEvent
	err          error
	sequence     int64
	responseID   string
	messageStart bool
	messageDelta bool
	terminal     bool
	closed       bool
}

func newMessageStream(
	ctx context.Context,
	request modelinvoker.Request,
	base *protocol.Base,
	native EventStream,
	headers http.Header,
	rawRequest modelinvoker.RawPayload,
	decisions []modelinvoker.MappingDecision,
	continuationMapper ContinuationMapper,
	stopReasonMapper StopReasonMapper,
) modelinvoker.Stream {
	return &messageStream{
		ctx: ctx, request: request, base: base, native: native, headers: headers, rawRequest: rawRequest,
		decisions: append([]modelinvoker.MappingDecision(nil), decisions...), continuationMapper: continuationMapper, stopReasonMapper: stopReasonMapper,
		blocks: make(map[int64]*streamContentBlock),
	}
}

func (s *messageStream) Next() bool {
	if s.closed || (s.terminal && len(s.queue) == 0) {
		return false
	}
	for len(s.queue) == 0 && s.err == nil {
		if !s.native.Next() {
			if err := s.native.Err(); err != nil {
				raw := errorRawPayload(err)
				if !raw.Empty() {
					s.nativeEvents = append(s.nativeEvents, raw)
				}
				s.fail(normalizeFailure(s.ctx, s.base, s.request, "messages.stream", s.headers, err), raw)
			} else if !s.terminal {
				detail := "Anthropic stream ended without message_stop"
				if open := s.openBlockIndexes(); len(open) != 0 {
					detail = fmt.Sprintf("Anthropic stream ended with unfinished content blocks at indexes %v", open)
				}
				s.fail(driverError(modelinvoker.ErrorStreamInterrupted, "messages.stream", detail), modelinvoker.RawPayload{})
			}
			break
		}
		s.mapEvent(s.native.Current())
	}
	if len(s.queue) == 0 {
		return false
	}
	s.current = s.queue[0]
	s.queue = s.queue[1:]
	return true
}

func (s *messageStream) mapEvent(event anthropicsdk.MessageStreamEventUnion) {
	raw, err := adaptercore.RawPayload(event.RawJSON(), event)
	if err != nil {
		detail := "could not serialize Anthropic stream event audit payload: " + err.Error()
		s.decisions = append(s.decisions, modelinvoker.MappingDecision{
			Capability: modelinvoker.CapabilityStreaming,
			Action:     modelinvoker.MappingRejected,
			Detail:     detail,
		})
		s.fail(mappingErrorWithRequestID("messages.stream.audit_event", detail, s.base.RequestID(s.headers)), modelinvoker.RawPayload{})
		return
	}
	s.nativeEvents = append(s.nativeEvents, raw)
	typed := event.AsAny()
	if err := s.validateEvent(typed); err != nil {
		s.fail(err, raw)
		return
	}
	if err := s.accumulator.Accumulate(event); err != nil {
		s.fail(driverError(modelinvoker.ErrorProvider, "messages.stream", "could not accumulate Messages stream: "+err.Error()), raw)
		return
	}

	switch typed := typed.(type) {
	case anthropicsdk.MessageStartEvent:
		s.messageStart = true
		s.responseID = typed.Message.ID
		s.enqueue(modelinvoker.StreamEvent{Type: modelinvoker.StreamEventResponseStarted, ResponseID: s.responseID, Raw: raw})
	case anthropicsdk.ContentBlockStartEvent:
		s.mapContentStart(typed, raw)
	case anthropicsdk.ContentBlockDeltaEvent:
		s.mapContentDelta(typed, raw)
	case anthropicsdk.ContentBlockStopEvent:
		s.mapContentStop(typed, raw)
	case anthropicsdk.MessageDeltaEvent:
		s.messageDelta = true
		s.mergeDeltaUsage(typed.Usage)
		usage := normalizeUsage(s.accumulator.Usage)
		s.enqueue(modelinvoker.StreamEvent{Type: modelinvoker.StreamEventUsage, ResponseID: s.responseID, Usage: &usage, Raw: raw})
	case anthropicsdk.MessageStopEvent:
		s.finish(raw)
	default:
		s.enqueue(modelinvoker.StreamEvent{Type: modelinvoker.StreamEventNative, ResponseID: s.responseID, Raw: raw})
	}
}

func (s *messageStream) validateEvent(event any) error {
	requestID := s.base.RequestID(s.headers)
	switch typed := event.(type) {
	case anthropicsdk.MessageStartEvent:
		if s.messageStart {
			return mappingErrorWithRequestID("messages.stream", "message_start arrived more than once", requestID)
		}
		if typed.Message.ID == "" {
			return mappingErrorWithRequestID("messages.stream", "message_start requires a response ID", requestID)
		}
		if len(typed.Message.Content) != 0 {
			return mappingErrorWithRequestID("messages.stream", "message_start must not contain pre-populated content blocks", requestID)
		}
	case anthropicsdk.ContentBlockStartEvent:
		if !s.messageStart {
			return mappingErrorWithRequestID("messages.stream", "content block start arrived before message_start", requestID)
		}
		if s.messageDelta {
			return mappingErrorWithRequestID("messages.stream", "content block start arrived after message_delta", requestID)
		}
		if _, exists := s.blocks[typed.Index]; exists {
			return mappingErrorWithRequestID("messages.stream", fmt.Sprintf("content block index %d was started more than once", typed.Index), requestID)
		}
		if typed.Index != int64(len(s.accumulator.Content)) {
			return mappingErrorWithRequestID("messages.stream", fmt.Sprintf("content block start index %d is out of sequence; expected %d", typed.Index, len(s.accumulator.Content)), requestID)
		}
		if typed.ContentBlock.Type == "tool_use" {
			tool := typed.ContentBlock.AsToolUse()
			if tool.ID == "" || !nativeToolNamePattern.MatchString(tool.Name) {
				return mappingErrorWithRequestID("messages.stream", "tool-use start requires an ID and valid name", requestID)
			}
		}
	case anthropicsdk.ContentBlockDeltaEvent:
		if !s.messageStart {
			return mappingErrorWithRequestID("messages.stream", "content delta arrived before message_start", requestID)
		}
		if s.messageDelta {
			return mappingErrorWithRequestID("messages.stream", "content delta arrived after message_delta", requestID)
		}
		block, exists := s.blocks[typed.Index]
		if !exists {
			return mappingErrorWithRequestID("messages.stream", fmt.Sprintf("content delta index %d has no matching content block start", typed.Index), requestID)
		}
		if block.closed {
			return mappingErrorWithRequestID("messages.stream", fmt.Sprintf("content delta index %d targets a closed %s block", typed.Index, block.kind), requestID)
		}
		if !contentDeltaMatches(block.kind, typed.Delta.Type) {
			return mappingErrorWithRequestID("messages.stream", fmt.Sprintf("delta type %q does not match %s content block at index %d", typed.Delta.Type, block.kind, typed.Index), requestID)
		}
	case anthropicsdk.ContentBlockStopEvent:
		if !s.messageStart {
			return mappingErrorWithRequestID("messages.stream", "content block stop arrived before message_start", requestID)
		}
		if s.messageDelta {
			return mappingErrorWithRequestID("messages.stream", "content block stop arrived after message_delta", requestID)
		}
		block, exists := s.blocks[typed.Index]
		if !exists {
			return mappingErrorWithRequestID("messages.stream", fmt.Sprintf("content block stop index %d has no matching content block start", typed.Index), requestID)
		}
		if block.closed {
			return mappingErrorWithRequestID("messages.stream", fmt.Sprintf("content block index %d was stopped more than once", typed.Index), requestID)
		}
	case anthropicsdk.MessageDeltaEvent:
		if !s.messageStart {
			return mappingErrorWithRequestID("messages.stream", "message_delta arrived before message_start", requestID)
		}
		if s.messageDelta {
			return mappingErrorWithRequestID("messages.stream", "message_delta arrived more than once", requestID)
		}
		if open := s.openBlockIndexes(); len(open) != 0 {
			return mappingErrorWithRequestID("messages.stream", fmt.Sprintf("message_delta arrived with unfinished content blocks at indexes %v", open), requestID)
		}
	case anthropicsdk.MessageStopEvent:
		if !s.messageStart {
			return mappingErrorWithRequestID("messages.stream", "message_stop arrived before message_start", requestID)
		}
		if open := s.openBlockIndexes(); len(open) != 0 {
			return mappingErrorWithRequestID("messages.stream", fmt.Sprintf("message_stop arrived with unfinished content blocks at indexes %v", open), requestID)
		}
		if !s.messageDelta {
			return mappingErrorWithRequestID("messages.stream", "message_stop arrived before message_delta", requestID)
		}
	}
	return nil
}

func contentDeltaMatches(blockType, deltaType string) bool {
	switch blockType {
	case "text":
		return deltaType == "text_delta" || deltaType == "citations_delta"
	case "thinking":
		return deltaType == "thinking_delta" || deltaType == "signature_delta"
	case "redacted_thinking":
		return false
	case "tool_use":
		return deltaType == "input_json_delta"
	default:
		return true
	}
}

func (s *messageStream) openBlockIndexes() []int64 {
	result := make([]int64, 0)
	for index := int64(0); index < int64(len(s.accumulator.Content)); index++ {
		if block := s.blocks[index]; block != nil && !block.closed {
			result = append(result, index)
		}
	}
	return result
}

func (s *messageStream) mapContentStart(event anthropicsdk.ContentBlockStartEvent, raw modelinvoker.RawPayload) {
	block := event.ContentBlock
	tracked := &streamContentBlock{kind: block.Type}
	s.blocks[event.Index] = tracked
	switch block.Type {
	case "tool_use":
		tool := block.AsToolUse()
		if tool.Caller.Type != "" && tool.Caller.Type != "direct" {
			s.fail(mappingErrorWithRequestID(
				"messages.stream",
				"non-direct Anthropic tool use caller "+tool.Caller.Type+" is outside the unified Anthropic slice",
				s.base.RequestID(s.headers),
			), raw)
			return
		}
		arguments := cloneJSON(tool.Input)
		if string(arguments) == "{}" {
			arguments = nil
		}
		tracked.tool = &pendingToolCall{id: tool.ID, name: tool.Name, arguments: arguments}
		call := modelinvoker.FunctionCall{ID: tool.ID, Name: tool.Name, Arguments: cloneJSON(tool.Input)}
		s.enqueue(modelinvoker.StreamEvent{
			Type: modelinvoker.StreamEventFunctionCallStarted, ResponseID: s.responseID,
			FunctionCall: &call, Raw: raw,
		})
	case "text", "thinking", "redacted_thinking":
		s.enqueue(modelinvoker.StreamEvent{Type: modelinvoker.StreamEventNative, ResponseID: s.responseID, Raw: raw})
	default:
		s.handleUnsupportedStreamContent("Anthropic content block "+block.Type, raw)
	}
}

func (s *messageStream) mapContentDelta(event anthropicsdk.ContentBlockDeltaEvent, raw modelinvoker.RawPayload) {
	if block := s.blocks[event.Index]; block != nil {
		if !supportedStreamBlock(block.kind) {
			s.handleUnsupportedStreamContent("Anthropic "+block.kind+" content delta", raw)
			return
		}
	}
	switch delta := event.Delta.AsAny().(type) {
	case anthropicsdk.TextDelta:
		s.enqueue(modelinvoker.StreamEvent{
			Type: modelinvoker.StreamEventTextDelta, ResponseID: s.responseID, TextDelta: delta.Text, Raw: raw,
		})
	case anthropicsdk.InputJSONDelta:
		pending := s.blocks[event.Index].tool
		pending.arguments = append(pending.arguments, delta.PartialJSON...)
		call := modelinvoker.FunctionCall{ID: pending.id, Name: pending.name, Arguments: cloneJSON(pending.arguments)}
		s.enqueue(modelinvoker.StreamEvent{
			Type: modelinvoker.StreamEventFunctionArgumentsDelta, ResponseID: s.responseID,
			ArgumentsDelta: delta.PartialJSON, FunctionCall: &call, Raw: raw,
		})
	case anthropicsdk.ThinkingDelta:
		s.enqueue(modelinvoker.StreamEvent{
			Type: modelinvoker.StreamEventReasoningDelta, ResponseID: s.responseID,
			ReasoningDelta: delta.Thinking, Raw: raw,
		})
	case anthropicsdk.SignatureDelta:
		s.enqueue(modelinvoker.StreamEvent{Type: modelinvoker.StreamEventNative, ResponseID: s.responseID, Raw: raw})
	case anthropicsdk.CitationsDelta:
		s.handleUnsupportedStreamContent("Anthropic citation delta", raw)
	default:
		s.handleUnsupportedStreamContent("unknown Anthropic content delta", raw)
	}
}

func (s *messageStream) mapContentStop(event anthropicsdk.ContentBlockStopEvent, raw modelinvoker.RawPayload) {
	if event.Index < 0 || event.Index >= int64(len(s.accumulator.Content)) {
		s.fail(mappingErrorWithRequestID("messages.stream", fmt.Sprintf("content block stop index %d is out of range", event.Index), s.base.RequestID(s.headers)), raw)
		return
	}
	tracked := s.blocks[event.Index]
	switch tracked.kind {
	case "thinking":
		if s.accumulator.Content[event.Index].AsThinking().Signature == "" {
			s.fail(mappingErrorWithRequestID("messages.stream", fmt.Sprintf("thinking block at index %d stopped without a complete signature", event.Index), s.base.RequestID(s.headers)), raw)
			return
		}
	case "tool_use":
		tool := s.accumulator.Content[event.Index].AsToolUse()
		call := modelinvoker.FunctionCall{ID: tool.ID, Name: tool.Name, Arguments: cloneJSON(tool.Input)}
		if !validFunctionCall(call) {
			s.fail(mappingErrorWithRequestID("messages.stream", "completed Messages tool use has invalid JSON input", s.base.RequestID(s.headers)), raw)
			return
		}
		tracked.closed = true
		s.enqueue(modelinvoker.StreamEvent{
			Type: modelinvoker.StreamEventFunctionCallCompleted, ResponseID: s.responseID,
			FunctionCall: &call, Raw: raw,
		})
		return
	}
	tracked.closed = true
	s.enqueue(modelinvoker.StreamEvent{Type: modelinvoker.StreamEventNative, ResponseID: s.responseID, Raw: raw})
}

func supportedStreamBlock(blockType string) bool {
	switch blockType {
	case "text", "thinking", "redacted_thinking", "tool_use":
		return true
	default:
		return false
	}
}

func (s *messageStream) mergeDeltaUsage(delta anthropicsdk.MessageDeltaUsage) {
	if delta.JSON.InputTokens.Valid() {
		s.accumulator.Usage.InputTokens = delta.InputTokens
	}
	if delta.JSON.CacheCreationInputTokens.Valid() {
		s.accumulator.Usage.CacheCreationInputTokens = delta.CacheCreationInputTokens
	}
	if delta.JSON.CacheReadInputTokens.Valid() {
		s.accumulator.Usage.CacheReadInputTokens = delta.CacheReadInputTokens
	}
	if delta.JSON.OutputTokens.Valid() {
		s.accumulator.Usage.OutputTokens = delta.OutputTokens
	}
	if delta.JSON.OutputTokensDetails.Valid() {
		s.accumulator.Usage.OutputTokensDetails = delta.OutputTokensDetails
	}
	if delta.JSON.ServerToolUse.Valid() {
		s.accumulator.Usage.ServerToolUse = delta.ServerToolUse
	}
}

func (s *messageStream) finish(raw modelinvoker.RawPayload) {
	if open := s.openBlockIndexes(); len(open) != 0 {
		s.fail(mappingErrorWithRequestID("messages.stream", fmt.Sprintf("Messages stream stopped with unfinished content blocks at indexes %v", open), s.base.RequestID(s.headers)), raw)
		return
	}
	response, err := normalizeMessage(s.base, s.request, &s.accumulator, s.headers, s.continuationMapper, s.stopReasonMapper)
	response.RawRequest = s.rawRequest
	response.NativeEvents = append([]modelinvoker.RawPayload(nil), s.nativeEvents...)
	response.MappingReport.Decisions = append(response.MappingReport.Decisions, s.decisions...)
	if err != nil {
		s.failWithResponse(err, raw, &response)
		return
	}
	s.terminal = true
	s.enqueue(modelinvoker.StreamEvent{
		Type: modelinvoker.StreamEventResponseCompleted, ResponseID: s.responseID, Response: &response, Raw: raw,
	})
	_ = s.closeNative()
}

func (s *messageStream) handleUnsupportedStreamContent(description string, raw modelinvoker.RawPayload) {
	if !s.request.AllowDegradation {
		s.fail(mappingErrorWithRequestID("messages.stream", description+" is outside the unified Messages slice", s.base.RequestID(s.headers)), raw)
		return
	}
	s.decisions = append(s.decisions, degradation(modelinvoker.CapabilityTextGeneration, description+" retained only as a native event"))
	s.enqueue(modelinvoker.StreamEvent{Type: modelinvoker.StreamEventNative, ResponseID: s.responseID, Raw: raw})
}

func (s *messageStream) enqueue(event modelinvoker.StreamEvent) {
	s.sequence++
	event.Sequence = s.sequence
	if event.ResponseID == "" {
		event.ResponseID = s.responseID
	}
	s.queue = append(s.queue, event)
}

func (s *messageStream) fail(err error, raw modelinvoker.RawPayload) {
	response := s.partialResponse()
	s.failWithResponse(err, raw, &response)
}

func (s *messageStream) failWithResponse(err error, raw modelinvoker.RawPayload, response *modelinvoker.Response) {
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError == nil {
		invocationError = driverError(modelinvoker.ErrorStreamInterrupted, "messages.stream", "Messages stream failed")
	}
	s.err = invocationError
	s.terminal = true
	if response != nil {
		response.Status = modelinvoker.ResponseStatusFailed
	}
	s.enqueue(modelinvoker.StreamEvent{
		Type: modelinvoker.StreamEventError, ResponseID: s.responseID,
		Error: invocationError, Response: response, Raw: raw,
	})
	_ = s.closeNative()
}

func (s *messageStream) partialResponse() modelinvoker.Response {
	content := s.partialOutputContent()
	partial := s.accumulator
	partial.Content = content
	status, stopReason, knownStop := normalizeStopReason(string(partial.StopReason))
	if !knownStop && s.stopReasonMapper != nil {
		if mapped, handled := s.stopReasonMapper.MapMessagesStopReason(s.request, string(partial.StopReason)); handled {
			status, stopReason, knownStop = mapped.Status, mapped.StopReason, true
		}
	}
	if !knownStop {
		status = modelinvoker.ResponseStatusInProgress
		stopReason = ""
	}
	rawResponse, rawErr := adaptercore.RawPayload(s.accumulator.RawJSON(), &s.accumulator)
	response := modelinvoker.Response{
		ID: s.responseID, Provider: s.base.Binding().Provider, Protocol: s.base.Binding().Protocol,
		Model: s.request.Model, Status: status, StopReason: stopReason, StopSequence: partial.StopSequence,
		Usage: normalizeUsage(s.accumulator.Usage), RequestID: s.base.RequestID(s.headers),
		ProviderMetadata: messageMetadata(s.base, &s.accumulator, s.headers),
		MappingReport: modelinvoker.MappingReport{
			Provider: s.base.Binding().Provider, Protocol: s.base.Binding().Protocol, Endpoint: s.request.Endpoint,
			Decisions: append([]modelinvoker.MappingDecision(nil), s.decisions...),
		},
		RawRequest: s.rawRequest, RawResponse: rawResponse,
		NativeEvents: append([]modelinvoker.RawPayload(nil), s.nativeEvents...),
	}
	if rawErr != nil {
		response.MappingReport.Decisions = append(response.MappingReport.Decisions, modelinvoker.MappingDecision{
			Capability: modelinvoker.CapabilityStreaming,
			Action:     modelinvoker.MappingRejected,
			Detail:     "could not serialize Anthropic partial response audit payload: " + rawErr.Error(),
		})
	}
	_ = normalizeContentBlocks(s.request, &response, content)
	closed := s.closedContent()
	if len(closed) != 0 {
		continuation := s.accumulator
		continuation.Content = closed
		response.State, _ = continuationState(s.base.Binding(), s.request, &continuation, s.continuationMapper)
	}
	return response
}

func (s *messageStream) partialOutputContent() []anthropicsdk.ContentBlockUnion {
	content := make([]anthropicsdk.ContentBlockUnion, 0, len(s.accumulator.Content))
	for index, native := range s.accumulator.Content {
		block := s.blocks[int64(index)]
		if block == nil || block.closed || block.kind == "text" || block.kind == "thinking" {
			content = append(content, native)
		}
	}
	return content
}

func (s *messageStream) closedContent() []anthropicsdk.ContentBlockUnion {
	content := make([]anthropicsdk.ContentBlockUnion, 0, len(s.accumulator.Content))
	for index, native := range s.accumulator.Content {
		if block := s.blocks[int64(index)]; block != nil && block.closed {
			content = append(content, native)
		}
	}
	return content
}

func (s *messageStream) Event() modelinvoker.StreamEvent { return s.current }

func (s *messageStream) Err() error { return s.err }

func (s *messageStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.closeNative()
}

func (s *messageStream) closeNative() error {
	if s.native == nil {
		return nil
	}
	native := s.native
	s.native = nil
	return native.Close()
}

var _ modelinvoker.Stream = (*messageStream)(nil)
