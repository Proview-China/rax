package openairesponses_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/openai/openai-go/v3/responses"
)

func FuzzDriverInvokeNeverPanicsOrLosesTypedState(f *testing.F) {
	f.Add("completed", "hello")
	f.Add("future_status", "")
	f.Fuzz(func(t *testing.T, status, text string) {
		raw, err := json.Marshal(map[string]any{
			"id": "resp_fuzz", "model": "served-model", "status": status,
			"output": []any{map[string]any{
				"id": "msg", "type": "message", "role": "assistant", "status": "completed",
				"content": []any{map[string]any{"type": "output_text", "text": text, "annotations": []any{}}},
			}},
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
		})
		if err != nil {
			t.Fatal(err)
		}
		var native responses.Response
		if err := json.Unmarshal(raw, &native); err != nil {
			t.Fatal(err)
		}
		request := validRequest()
		request.AllowDegradation = true
		response, invokeErr := mustDriver(t, &fakeDialect{}, &fakeClient{response: &native}).Invoke(context.Background(), request)
		if response.Provider != testProvider || response.Protocol != modelinvoker.ProtocolResponses ||
			response.MappingReport.Provider != testProvider || response.MappingReport.Endpoint != testEndpoint ||
			response.State == nil || response.State.Provider != testProvider || response.State.ID != "resp_fuzz" {
			t.Fatalf("identity/state = %#v", response)
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
	f.Add("response.output_text.delta", "hello", uint16(5))
	f.Add("future.event", "", uint16(0))
	f.Fuzz(func(t *testing.T, eventType, delta string, sequence uint16) {
		raw, err := json.Marshal(map[string]any{
			"type": eventType, "sequence_number": sequence, "delta": delta,
		})
		if err != nil {
			t.Fatal(err)
		}
		var event responses.ResponseStreamEventUnion
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		client := &fakeClient{stream: &fakeStream{events: []responses.ResponseStreamEventUnion{event}}}
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
