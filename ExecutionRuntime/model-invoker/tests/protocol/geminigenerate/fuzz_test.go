package geminigenerate_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"google.golang.org/genai"
)

func FuzzDriverInvokeNeverPanicsOrLosesThoughtSignatureState(f *testing.F) {
	f.Add("thought", "Rome")
	f.Add("another thought", "")
	f.Fuzz(func(t *testing.T, thought, city string) {
		raw, err := json.Marshal(map[string]any{
			"candidates": []any{map[string]any{
				"content": map[string]any{"role": "model", "parts": []any{
					map[string]any{"text": "prefix " + thought, "thought": true, "thoughtSignature": "c2lnX2Z1eno="},
					map[string]any{"functionCall": map[string]any{"id": "call_fuzz", "name": "lookup", "args": map[string]any{"city": city}}, "thoughtSignature": "c2lnX2NhbGw="},
				}},
				"finishReason": "STOP", "index": 0,
			}},
			"responseId": "resp_fuzz", "modelVersion": "served-version",
			"usageMetadata": map[string]any{"promptTokenCount": 1, "candidatesTokenCount": 1, "totalTokenCount": 2},
		})
		if err != nil {
			t.Fatal(err)
		}
		response := decodeResponse(t, string(raw))
		request := validRequest()
		request.Tools = lookupTools()
		result, invokeErr := mustDriver(t, &fakeDialect{}, &fakeClient{responses: []*genai.GenerateContentResponse{response}}).Invoke(context.Background(), request)
		if result.Provider != testProvider || result.Protocol != modelinvoker.ProtocolGenerateContent ||
			result.MappingReport.Provider != testProvider || result.MappingReport.Endpoint != testEndpoint {
			t.Fatalf("identity = %#v", result)
		}
		if result.State != nil && (result.State.Provider != testProvider || result.State.Protocol != modelinvoker.ProtocolGenerateContent) {
			t.Fatalf("state identity = %#v", result.State)
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
	f.Add("text", "hello", int16(0))
	f.Add("", "", int16(1))
	f.Fuzz(func(t *testing.T, field, value string, index int16) {
		part := map[string]any{field: value}
		if field == "" {
			part = map[string]any{"text": value}
		}
		raw, err := json.Marshal(map[string]any{
			"candidates": []any{map[string]any{
				"content": map[string]any{"role": "model", "parts": []any{part}}, "index": index,
			}},
			"responseId": "resp_fuzz_stream",
		})
		if err != nil {
			t.Fatal(err)
		}
		response := decodeResponse(t, string(raw))
		client := &fakeClient{stream: &fakeStream{responses: []*genai.GenerateContentResponse{response}}}
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
