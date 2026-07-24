package toolcallobservation_test

import (
	"encoding/json"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestBlackboxPublicSyncAndStreamProduceSameObservation(t *testing.T) {
	digest := core.DigestBytes([]byte("blackbox-invocation"))
	call := modelinvoker.FunctionCall{ID: "call", Name: "lookup", Arguments: json.RawMessage(`{"b":2,"a":1}`)}
	response := modelinvoker.Response{ID: "response", Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall, Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}}}
	syncResult, err := modelinvoker.FinalizeToolCallCandidateObservationV1(digest, response)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := modelinvoker.NewToolCallCandidateStreamFinalizerV1(digest)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []modelinvoker.StreamEvent{
		{Type: modelinvoker.StreamEventFunctionCallStarted, Sequence: 1, ResponseID: response.ID, FunctionCall: &modelinvoker.FunctionCall{ID: "call", Name: "lookup"}},
		{Type: modelinvoker.StreamEventFunctionArgumentsDelta, Sequence: 2, ResponseID: response.ID, ArgumentsDelta: `{"b":2,"a":1}`, FunctionCall: &modelinvoker.FunctionCall{ID: "call", Name: "lookup", Arguments: json.RawMessage(`{"b":2,"a":1}`)}},
		{Type: modelinvoker.StreamEventFunctionCallCompleted, Sequence: 3, ResponseID: response.ID, FunctionCall: &call},
		{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 4, ResponseID: response.ID, Response: &response},
	} {
		if _, err = stream.Observe(event); err != nil {
			t.Fatal(err)
		}
	}
	streamResult, err := stream.Result()
	if err != nil || streamResult == nil || streamResult.Digest != syncResult.Digest || string(streamResult.Calls[0].CanonicalArguments) != `{"a":1,"b":2}` {
		t.Fatalf("sync/stream mismatch = %#v/%#v/%v", syncResult, streamResult, err)
	}
}
