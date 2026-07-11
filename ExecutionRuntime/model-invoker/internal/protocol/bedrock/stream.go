package bedrock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/aws/aws-sdk-go-v2/aws"
	awstypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

type toolState struct{ id, name string }

type converseStream struct {
	ctx        context.Context
	request    modelinvoker.Request
	native     ConverseEventStream
	rawRequest modelinvoker.RawPayload
	decisions  []modelinvoker.MappingDecision
	current    modelinvoker.StreamEvent
	sequence   int64
	response   modelinvoker.Response
	tools      map[int32]toolState
	err        error
	closed     bool
}

func newConverseStream(ctx context.Context, request modelinvoker.Request, native ConverseEventStream, raw modelinvoker.RawPayload, decisions []modelinvoker.MappingDecision) *converseStream {
	return &converseStream{ctx: ctx, request: request, native: native, rawRequest: raw, decisions: append([]modelinvoker.MappingDecision(nil), decisions...), tools: make(map[int32]toolState), response: modelinvoker.Response{Provider: request.Provider, Protocol: request.Protocol, Model: request.Model, Status: modelinvoker.ResponseStatusInProgress, RawRequest: raw, MappingReport: modelinvoker.MappingReport{Provider: request.Provider, Protocol: request.Protocol, Endpoint: request.Endpoint, Decisions: append([]modelinvoker.MappingDecision(nil), decisions...)}}}
}

func (s *converseStream) Next() bool {
	if s == nil || s.closed || s.err != nil {
		return false
	}
	select {
	case <-s.ctx.Done():
		s.err = s.ctx.Err()
		return false
	case native, ok := <-s.native.Events():
		if !ok {
			if nativeErr := s.native.Err(); nativeErr != nil {
				s.err = safeFailure(s.request.Provider, "bedrock_converse.stream", nativeErr)
			}
			return false
		}
		s.sequence++
		s.current = modelinvoker.StreamEvent{Sequence: s.sequence}
		s.mapEvent(native)
		return true
	}
}

func (s *converseStream) mapEvent(native awstypes.ConverseStreamOutput) {
	switch event := native.(type) {
	case *awstypes.ConverseStreamOutputMemberMessageStart:
		s.current.Type = modelinvoker.StreamEventResponseStarted
	case *awstypes.ConverseStreamOutputMemberContentBlockStart:
		if tool, ok := event.Value.Start.(*awstypes.ContentBlockStartMemberToolUse); ok {
			index := aws.ToInt32(event.Value.ContentBlockIndex)
			state := toolState{id: aws.ToString(tool.Value.ToolUseId), name: aws.ToString(tool.Value.Name)}
			s.tools[index] = state
			s.current.Type = modelinvoker.StreamEventFunctionCallStarted
			s.current.FunctionCall = &modelinvoker.FunctionCall{ID: state.id, Name: state.name}
		} else {
			s.current.Type = modelinvoker.StreamEventNative
		}
	case *awstypes.ConverseStreamOutputMemberContentBlockDelta:
		switch delta := event.Value.Delta.(type) {
		case *awstypes.ContentBlockDeltaMemberText:
			s.current.Type, s.current.TextDelta = modelinvoker.StreamEventTextDelta, delta.Value
			s.response.Output = appendText(s.response.Output, delta.Value)
		case *awstypes.ContentBlockDeltaMemberToolUse:
			s.current.Type, s.current.ArgumentsDelta = modelinvoker.StreamEventFunctionArgumentsDelta, aws.ToString(delta.Value.Input)
		default:
			s.current.Type = modelinvoker.StreamEventNative
		}
	case *awstypes.ConverseStreamOutputMemberContentBlockStop:
		index := aws.ToInt32(event.Value.ContentBlockIndex)
		if tool, ok := s.tools[index]; ok {
			s.current.Type = modelinvoker.StreamEventFunctionCallCompleted
			s.current.FunctionCall = &modelinvoker.FunctionCall{ID: tool.id, Name: tool.name}
			delete(s.tools, index)
		} else {
			s.current.Type = modelinvoker.StreamEventNative
		}
	case *awstypes.ConverseStreamOutputMemberMetadata:
		s.response.Usage = normalizeUsage(event.Value.Usage)
		usage := s.response.Usage
		s.current.Type, s.current.Usage = modelinvoker.StreamEventUsage, &usage
	case *awstypes.ConverseStreamOutputMemberMessageStop:
		s.response.Status = modelinvoker.ResponseStatusCompleted
		s.response.StopReason = normalizeStopReason(string(event.Value.StopReason))
		response := s.response
		s.current.Type, s.current.Response = modelinvoker.StreamEventResponseCompleted, &response
	default:
		s.current.Type = modelinvoker.StreamEventNative
	}
	raw, _ := json.Marshal(map[string]any{"aws_event": fmt.Sprintf("%T", native)})
	s.current.Raw = modelinvoker.NewRawPayload(raw)
}

func appendText(output []modelinvoker.OutputItem, delta string) []modelinvoker.OutputItem {
	if len(output) > 0 && output[len(output)-1].Type == modelinvoker.OutputItemText {
		output[len(output)-1].Text += delta
		return output
	}
	return append(output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemText, Text: delta})
}

func (s *converseStream) Event() modelinvoker.StreamEvent {
	if s == nil {
		return modelinvoker.StreamEvent{}
	}
	return s.current
}
func (s *converseStream) Err() error {
	if s == nil {
		return fmt.Errorf("Bedrock Converse stream is nil")
	}
	return s.err
}
func (s *converseStream) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	if err := s.native.Close(); err != nil {
		return safeFailure(s.request.Provider, "bedrock_converse.close", err)
	}
	return nil
}

type invokeStream struct {
	ctx               context.Context
	request           modelinvoker.Request
	native            InvokeEventStream
	rawRequest        modelinvoker.RawPayload
	current           modelinvoker.StreamEvent
	sequence          int64
	body              bytes.Buffer
	err               error
	closed, completed bool
}

func newInvokeStream(ctx context.Context, request modelinvoker.Request, native InvokeEventStream, raw modelinvoker.RawPayload) *invokeStream {
	return &invokeStream{ctx: ctx, request: request, native: native, rawRequest: raw}
}

func (s *invokeStream) Next() bool {
	if s == nil || s.closed || s.err != nil || s.completed {
		return false
	}
	select {
	case <-s.ctx.Done():
		s.err = s.ctx.Err()
		return false
	case native, ok := <-s.native.Events():
		if !ok {
			if err := s.native.Err(); err != nil {
				s.err = safeFailure(s.request.Provider, "bedrock_invoke_model.stream", err)
				return false
			}
			s.sequence++
			s.completed = true
			response := modelinvoker.Response{Provider: s.request.Provider, Protocol: s.request.Protocol, Model: s.request.Model, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonOther, RawRequest: s.rawRequest, RawResponse: modelinvoker.NewRawPayload(s.body.Bytes())}
			s.current = modelinvoker.StreamEvent{Type: modelinvoker.StreamEventResponseCompleted, Sequence: s.sequence, Response: &response}
			return true
		}
		s.sequence++
		s.current = modelinvoker.StreamEvent{Type: modelinvoker.StreamEventNative, Sequence: s.sequence}
		if chunk, ok := native.(*awstypes.ResponseStreamMemberChunk); ok {
			s.body.Write(chunk.Value.Bytes)
			s.current.Raw = modelinvoker.NewRawPayload(chunk.Value.Bytes)
		}
		return true
	}
}
func (s *invokeStream) Event() modelinvoker.StreamEvent {
	if s == nil {
		return modelinvoker.StreamEvent{}
	}
	return s.current
}
func (s *invokeStream) Err() error {
	if s == nil {
		return fmt.Errorf("Bedrock InvokeModel stream is nil")
	}
	return s.err
}
func (s *invokeStream) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	if err := s.native.Close(); err != nil {
		return safeFailure(s.request.Provider, "bedrock_invoke_model.close", err)
	}
	return nil
}

var _ modelinvoker.Stream = (*converseStream)(nil)
var _ modelinvoker.Stream = (*invokeStream)(nil)
