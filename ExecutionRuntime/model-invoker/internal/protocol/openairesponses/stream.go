package openairesponses

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol"
	"github.com/openai/openai-go/v3/responses"
)

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
	responseID     string
	toolCalls      map[string]modelinvoker.FunctionCall
	nativeEvents   []modelinvoker.RawPayload
	rawStream      bytes.Buffer
	sequence       int64
	responseMapper ResponseMapper
}

func newStream(
	ctx context.Context,
	base *protocol.Base,
	request modelinvoker.Request,
	native EventStream,
	headers http.Header,
	rawRequest modelinvoker.RawPayload,
	decisions []modelinvoker.MappingDecision,
	responseMapper ResponseMapper,
) modelinvoker.Stream {
	return &stream{
		ctx: ctx, base: base, request: request, native: native, headers: headers, rawRequest: rawRequest,
		decisions: append([]modelinvoker.MappingDecision(nil), decisions...), responseMapper: responseMapper,
		toolCalls: make(map[string]modelinvoker.FunctionCall),
	}
}

func (s *stream) Next() bool {
	if s.closed || (s.terminal && len(s.queue) == 0) {
		return false
	}
	for len(s.queue) == 0 && s.err == nil {
		if !s.native.Next() {
			if err := s.native.Err(); err != nil {
				raw := s.captureStreamError(err)
				s.fail(normalizeFailure(s.ctx, s.base, s.request, "responses.stream", s.headers, err), raw, 0)
			} else if !s.terminal {
				s.fail(driverError(modelinvoker.ErrorStreamInterrupted, "responses.stream", "stream ended without a terminal event"), modelinvoker.RawPayload{}, 0)
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

func (s *stream) mapEvent(event responses.ResponseStreamEventUnion) {
	rawJSON := event.RawJSON()
	raw := modelinvoker.NewRawPayload([]byte(rawJSON))
	s.nativeEvents = append(s.nativeEvents, raw)
	if rawJSON != "" {
		s.rawStream.WriteString(rawJSON)
		s.rawStream.WriteByte('\n')
	}
	if event.Response.ID != "" {
		s.responseID = event.Response.ID
	}
	base := modelinvoker.StreamEvent{ResponseID: s.responseID, Raw: raw}

	switch event.Type {
	case "response.created":
		base.Type = modelinvoker.StreamEventResponseStarted
		base.ResponseID = event.Response.ID
		s.enqueue(base, event.SequenceNumber)
	case "response.output_text.delta":
		base.Type = modelinvoker.StreamEventTextDelta
		base.TextDelta = event.Delta
		s.enqueue(base, event.SequenceNumber)
	case "response.output_item.added":
		if event.Item.Type != "function_call" {
			base.Type = modelinvoker.StreamEventNative
			s.enqueue(base, event.SequenceNumber)
			return
		}
		call := event.Item.AsFunctionCall()
		semantic := modelinvoker.FunctionCall{ID: call.CallID, Name: call.Name, Arguments: json.RawMessage(call.Arguments)}
		if event.Item.ID == "" || semantic.ID == "" || !toolNamePattern.MatchString(semantic.Name) {
			s.fail(mappingErrorWithRequestID("responses.stream", "function call start requires item ID, call ID, and valid name", s.base.RequestID(s.headers)), raw, event.SequenceNumber)
			return
		}
		s.toolCalls[event.Item.ID] = semantic
		base.Type = modelinvoker.StreamEventFunctionCallStarted
		base.FunctionCall = cloneCall(semantic)
		s.enqueue(base, event.SequenceNumber)
	case "response.function_call_arguments.delta":
		call, ok := s.toolCalls[event.ItemID]
		if !ok {
			s.fail(mappingErrorWithRequestID("responses.stream", "function argument delta has no matching function call", s.base.RequestID(s.headers)), raw, event.SequenceNumber)
			return
		}
		call.Arguments = append(call.Arguments, event.Delta...)
		s.toolCalls[event.ItemID] = call
		base.Type = modelinvoker.StreamEventFunctionArgumentsDelta
		base.ArgumentsDelta = event.Delta
		base.FunctionCall = cloneCall(call)
		s.enqueue(base, event.SequenceNumber)
	case "response.function_call_arguments.done":
		call, ok := s.toolCalls[event.ItemID]
		if !ok {
			s.fail(mappingErrorWithRequestID("responses.stream", "completed function arguments have no matching function call", s.base.RequestID(s.headers)), raw, event.SequenceNumber)
			return
		}
		if event.Name != "" {
			call.Name = event.Name
		}
		call.Arguments = json.RawMessage(event.Arguments)
		if !validFunctionCall(call) {
			s.fail(mappingErrorWithRequestID("responses.stream", "completed function call requires an ID, valid name, and JSON object arguments", s.base.RequestID(s.headers)), raw, event.SequenceNumber)
			return
		}
		s.toolCalls[event.ItemID] = call
		base.Type = modelinvoker.StreamEventFunctionCallCompleted
		base.FunctionCall = cloneCall(call)
		s.enqueue(base, event.SequenceNumber)
	case "response.reasoning_summary_text.delta":
		base.Type = modelinvoker.StreamEventReasoningDelta
		base.ReasoningDelta = event.Delta
		s.enqueue(base, event.SequenceNumber)
	case "response.completed":
		response, err := normalizeResponse(s.ctx, s.base, s.request, &event.Response, s.headers)
		if err == nil && s.responseMapper != nil {
			err = s.responseMapper.MapResponsesResponse(s.request, &event.Response, &response)
		}
		response.RawRequest = s.rawRequest
		response.NativeEvents = append([]modelinvoker.RawPayload(nil), s.nativeEvents...)
		response.MappingReport.Decisions = append(response.MappingReport.Decisions, s.decisions...)
		s.enqueueResponsesUsage(event.Response, response.Usage, base, event.SequenceNumber)
		if err != nil {
			s.failWithResponse(err, raw, event.SequenceNumber, &response)
			return
		}
		s.terminal = true
		base.Type = modelinvoker.StreamEventResponseCompleted
		base.Response = &response
		s.enqueue(base, event.SequenceNumber)
	case "response.incomplete":
		response, err := normalizeResponse(s.ctx, s.base, s.request, &event.Response, s.headers)
		if err == nil && s.responseMapper != nil {
			err = s.responseMapper.MapResponsesResponse(s.request, &event.Response, &response)
		}
		response.RawRequest = s.rawRequest
		response.NativeEvents = append([]modelinvoker.RawPayload(nil), s.nativeEvents...)
		response.MappingReport.Decisions = append(response.MappingReport.Decisions, s.decisions...)
		s.enqueueResponsesUsage(event.Response, response.Usage, base, event.SequenceNumber)
		if err != nil {
			s.failWithResponse(err, raw, event.SequenceNumber, &response)
			return
		}
		s.terminal = true
		base.Type = modelinvoker.StreamEventResponseCompleted
		base.Response = &response
		s.enqueue(base, event.SequenceNumber)
	case "response.failed":
		response, err := normalizeResponse(s.ctx, s.base, s.request, &event.Response, s.headers)
		if err == nil && s.responseMapper != nil {
			err = s.responseMapper.MapResponsesResponse(s.request, &event.Response, &response)
		}
		if err == nil {
			err = driverError(modelinvoker.ErrorProvider, "responses.stream", "stream failed")
		}
		response.RawRequest = s.rawRequest
		response.NativeEvents = append([]modelinvoker.RawPayload(nil), s.nativeEvents...)
		response.MappingReport.Decisions = append(response.MappingReport.Decisions, s.decisions...)
		s.enqueueResponsesUsage(event.Response, response.Usage, base, event.SequenceNumber)
		s.failWithResponse(err, raw, event.SequenceNumber, &response)
	case "error":
		err := s.base.NormalizeFailure(s.ctx, s.request, "responses.stream", protocol.Failure{
			Source: protocol.FailureSourceStream, Code: event.Code, Message: event.Message,
			RequestID: s.base.RequestID(s.headers), Raw: raw,
		})
		s.fail(err, raw, event.SequenceNumber)
	default:
		base.Type = modelinvoker.StreamEventNative
		s.enqueue(base, event.SequenceNumber)
	}
}

func (s *stream) enqueue(event modelinvoker.StreamEvent, nativeSequence int64) {
	next := s.sequence + 1
	if nativeSequence > next {
		next = nativeSequence
	}
	s.sequence = next
	event.Sequence = next
	s.queue = append(s.queue, event)
}

func (s *stream) enqueueResponsesUsage(native responses.Response, usage modelinvoker.Usage, base modelinvoker.StreamEvent, nativeSequence int64) {
	if !native.JSON.Usage.Valid() {
		return
	}
	base.Type = modelinvoker.StreamEventUsage
	base.Usage = &usage
	s.enqueue(base, nativeSequence)
}

func (s *stream) fail(err error, raw modelinvoker.RawPayload, sequence int64) {
	s.failWithResponse(err, raw, sequence, s.partialResponse())
}

func (s *stream) failWithResponse(err error, raw modelinvoker.RawPayload, sequence int64, response *modelinvoker.Response) {
	var invocationError *modelinvoker.Error
	if candidate, ok := err.(*modelinvoker.Error); ok && candidate != nil {
		copy := *candidate
		invocationError = &copy
	} else {
		invocationError = driverError(modelinvoker.ErrorStreamInterrupted, "responses.stream", "stream failed")
	}
	if invocationError.RequestID == "" {
		invocationError.RequestID = s.base.RequestID(s.headers)
	}
	s.terminal = true
	s.err = invocationError
	s.enqueue(modelinvoker.StreamEvent{
		Type: modelinvoker.StreamEventError, ResponseID: s.responseID,
		Response: response, Error: invocationError, Raw: raw,
	}, sequence)
}

func (s *stream) partialResponse() *modelinvoker.Response {
	result := &modelinvoker.Response{
		ID: s.responseID, Protocol: modelinvoker.ProtocolResponses,
		Model: s.request.Model, Status: modelinvoker.ResponseStatusFailed, State: responseState(s.responseID),
		RequestID: s.base.RequestID(s.headers), ProviderMetadata: s.base.ProviderMetadata(s.headers),
		RawRequest: s.rawRequest, RawResponse: modelinvoker.NewRawPayload(s.rawStream.Bytes()),
		NativeEvents: append([]modelinvoker.RawPayload(nil), s.nativeEvents...),
		MappingReport: modelinvoker.MappingReport{
			Protocol: modelinvoker.ProtocolResponses, Endpoint: s.request.Endpoint,
			Decisions: append([]modelinvoker.MappingDecision(nil), s.decisions...),
		},
	}
	if s.responseMapper != nil {
		_ = s.responseMapper.MapResponsesResponse(s.request, nil, result)
	}
	return result
}

func (s *stream) Event() modelinvoker.StreamEvent { return s.current }

func (s *stream) Err() error { return s.err }

func (s *stream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.native.Close()
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

var (
	_ modelinvoker.Stream = (*stream)(nil)
)
