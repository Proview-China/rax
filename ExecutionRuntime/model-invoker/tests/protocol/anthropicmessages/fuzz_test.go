package anthropicmessages_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

func FuzzDriverInvokeNeverPanicsOrLosesProviderContinuation(f *testing.F) {
	f.Add("reason", "Rome")
	f.Add("another reason", "")
	f.Fuzz(func(t *testing.T, thinking, city string) {
		raw, err := json.Marshal(map[string]any{
			"id": "msg_fuzz", "type": "message", "role": "assistant", "model": "served-model",
			"content": []any{
				map[string]any{"type": "thinking", "thinking": "prefix " + thinking, "signature": "sig_fuzz"},
				map[string]any{"type": "tool_use", "id": "toolu_fuzz", "name": "lookup", "input": map[string]any{"city": city}, "caller": map[string]any{"type": "direct"}},
			},
			"stop_reason": "tool_use", "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
		if err != nil {
			t.Fatal(err)
		}
		var native anthropicsdk.Message
		if err := json.Unmarshal(raw, &native); err != nil {
			t.Fatal(err)
		}
		request := validRequest()
		request.Tools = lookupTools()
		request.Reasoning = &modelinvoker.Reasoning{Effort: modelinvoker.ReasoningEffortHigh}
		response, invokeErr := mustDriver(t, &fakeDialect{}, &fakeClient{responses: []*anthropicsdk.Message{&native}}).Invoke(context.Background(), request)
		if response.Provider != testProvider || response.Protocol != modelinvoker.ProtocolMessages ||
			response.MappingReport.Provider != testProvider || response.MappingReport.Endpoint != testEndpoint {
			t.Fatalf("identity = %#v", response)
		}
		if response.State != nil && (response.State.Provider != testProvider || response.State.Protocol != modelinvoker.ProtocolMessages) {
			t.Fatalf("state identity = %#v", response.State)
		}
		if invokeErr != nil {
			var invocationError *modelinvoker.Error
			if !errors.As(invokeErr, &invocationError) || invocationError.Provider != testProvider || invocationError.Err != nil {
				t.Fatalf("unstable error = %#v", invokeErr)
			}
		}
	})
}

func FuzzDriverStreamNeverPanicsOrEmitsNonMonotonicSequence(f *testing.F) {
	f.Add("future_event", "value", int16(0))
	f.Add("content_block_delta", "hello", int16(1))
	f.Fuzz(func(t *testing.T, eventType, value string, index int16) {
		raw, err := json.Marshal(map[string]any{
			"type": eventType, "index": index,
			"delta": map[string]any{"type": "text_delta", "text": value},
		})
		if err != nil {
			t.Fatal(err)
		}
		var event anthropicsdk.MessageStreamEventUnion
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		client := &fakeClient{stream: &fakeStream{events: []anthropicsdk.MessageStreamEventUnion{event}}}
		stream, streamErr := mustDriver(t, &fakeDialect{}, client).Stream(context.Background(), validRequest())
		if streamErr != nil {
			var invocationError *modelinvoker.Error
			if !errors.As(streamErr, &invocationError) || invocationError.Provider != testProvider {
				t.Fatalf("unstable setup error = %#v", streamErr)
			}
			return
		}
		var previous int64
		for stream.Next() {
			event := stream.Event()
			if event.Sequence <= previous {
				t.Fatalf("sequence %d after %d", event.Sequence, previous)
			}
			previous = event.Sequence
			if event.Response != nil && event.Response.Provider != testProvider {
				t.Fatalf("response identity = %#v", event.Response)
			}
		}
		if err := stream.Err(); err != nil {
			var invocationError *modelinvoker.Error
			if !errors.As(err, &invocationError) || invocationError.Provider != testProvider || invocationError.Err != nil {
				t.Fatalf("unstable terminal error = %#v", err)
			}
		}
		_ = stream.Close()
	})
}
