package openaichat_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/protocol/openaichat"
	openaisdk "github.com/openai/openai-go/v3"
)

func FuzzDriverInvokeNeverPanicsOrLosesIdentity(f *testing.F) {
	f.Add("hello", "stop")
	f.Add("", "future_reason")
	f.Fuzz(func(t *testing.T, content, finishReason string) {
		raw, err := json.Marshal(map[string]any{
			"id": "chat", "model": "served-model",
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": finishReason,
				"message": map[string]any{"content": content},
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
		if err != nil {
			t.Fatal(err)
		}
		var native openaisdk.ChatCompletion
		if err := json.Unmarshal(raw, &native); err != nil {
			t.Fatal(err)
		}
		driver := mustDriver(t, &fakeDialect{}, &fakeClient{response: &native})
		response, invokeErr := driver.Invoke(context.Background(), validRequest())
		if response.Provider != testProvider || response.Protocol != modelinvoker.ProtocolChatCompletions ||
			response.MappingReport.Provider != testProvider || response.MappingReport.Endpoint != testEndpoint {
			t.Fatalf("identity = %#v", response)
		}
		if invokeErr != nil {
			var invocationError *modelinvoker.Error
			if !errors.As(invokeErr, &invocationError) || invocationError.Provider != testProvider || invocationError.Err != nil {
				t.Fatalf("unstable public error = %#v", invokeErr)
			}
		}
	})
}

func FuzzDriverStreamNeverPanicsOrEmitsNonMonotonicSequence(f *testing.F) {
	f.Add("hello", "stop")
	f.Add("partial", "future_reason")
	f.Fuzz(func(t *testing.T, content, finishReason string) {
		raw, err := json.Marshal(map[string]any{
			"id": "chat", "model": "served-model",
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": finishReason,
				"delta": map[string]any{"content": content},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		var chunk openaisdk.ChatCompletionChunk
		if err := json.Unmarshal(raw, &chunk); err != nil {
			t.Fatal(err)
		}
		client := &fakeClient{stream: &fakeStream{events: []openaisdk.ChatCompletionChunk{chunk}}}
		driver := mustDriver(t, &fakeDialect{}, client)
		stream, streamErr := driver.Stream(context.Background(), validRequest())
		if streamErr != nil {
			var invocationError *modelinvoker.Error
			if !errors.As(streamErr, &invocationError) || invocationError.Provider != testProvider {
				t.Fatalf("unstable stream setup error = %#v", streamErr)
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

var _ openaichat.Client = (*fakeClient)(nil)
