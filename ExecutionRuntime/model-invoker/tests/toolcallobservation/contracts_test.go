package toolcallobservation_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func invocationDigest() core.Digest {
	return core.DigestBytes([]byte("tool-call-observation-invocation"))
}

func response(calls ...modelinvoker.FunctionCall) modelinvoker.Response {
	result := modelinvoker.Response{ID: "response", Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall}
	for index := range calls {
		call := calls[index]
		result.Output = append(result.Output, modelinvoker.OutputItem{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call})
	}
	return result
}

func errorCode(err error) string {
	var invocationError *modelinvoker.Error
	if errors.As(err, &invocationError) {
		return invocationError.Code
	}
	return ""
}

func TestUnitAtomicBatchOrderCanonicalizationAndValidation(t *testing.T) {
	result, err := modelinvoker.FinalizeToolCallCandidateObservationV1(invocationDigest(), response(
		modelinvoker.FunctionCall{ID: "call-b", Name: "second", Arguments: json.RawMessage(`{"z":1.0,"a":[true,10e-1]}`)},
		modelinvoker.FunctionCall{ID: "call-a", Name: "first", Arguments: json.RawMessage(` { "value" : -0 } `)},
	))
	if err != nil || result.Validate() != nil {
		t.Fatalf("finalize/validate = %#v/%v", result, err)
	}
	if len(result.Calls) != 2 || result.Calls[0].Ordinal != 0 || result.Calls[0].CallID != "call-b" || result.Calls[1].Ordinal != 1 || result.Calls[1].CallID != "call-a" {
		t.Fatalf("ordered calls = %#v", result.Calls)
	}
	if string(result.Calls[0].CanonicalArguments) != `{"a":[true,1],"z":1}` || string(result.Calls[1].CanonicalArguments) != `{"value":0}` {
		t.Fatalf("canonical arguments = %s / %s", result.Calls[0].CanonicalArguments, result.Calls[1].CanonicalArguments)
	}
	mutated := result.Clone()
	mutated.Calls[0].Ordinal = 2
	if mutated.Validate() == nil {
		t.Fatal("non-contiguous ordinal validated")
	}
	mutated = result.Clone()
	mutated.Digest = core.DigestBytes([]byte("drift"))
	if mutated.Validate() == nil {
		t.Fatal("digest drift validated")
	}
}

func TestWhiteboxStrictJSONFailureMatrixThroughPublicFinalizer(t *testing.T) {
	invalidUTF8 := json.RawMessage([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})
	oversized := append(json.RawMessage(`{"value":"`), bytes.Repeat([]byte{'x'}, core.MaxCanonicalDocumentBytes)...)
	oversized = append(oversized, []byte(`"}`)...)
	tests := []struct {
		name string
		raw  json.RawMessage
		code string
	}{
		{"partial", json.RawMessage(`{"a":`), "tool_call_arguments_json_invalid"},
		{"array", json.RawMessage(`[]`), "tool_call_arguments_not_object"},
		{"scalar", json.RawMessage(`1`), "tool_call_arguments_not_object"},
		{"null", json.RawMessage(`null`), "tool_call_arguments_not_object"},
		{"trailing", json.RawMessage(`{} {}`), "tool_call_arguments_trailing"},
		{"duplicate_key", json.RawMessage(`{"a":1,"a":2}`), "tool_call_arguments_duplicate_key"},
		{"invalid_utf8", invalidUTF8, "tool_call_arguments_utf8_invalid"},
		{"oversized", oversized, "tool_call_arguments_size_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := modelinvoker.FinalizeToolCallCandidateObservationV1(invocationDigest(), response(modelinvoker.FunctionCall{ID: "call", Name: "tool", Arguments: test.raw}))
			if result.Digest != "" || len(result.Calls) != 0 || errorCode(err) != test.code {
				t.Fatalf("result/error = %#v/%v, want atomic zero/%q", result, err, test.code)
			}
		})
	}
}

func TestUnitAtomicFailureMatrix(t *testing.T) {
	valid := modelinvoker.FunctionCall{ID: "call", Name: "tool", Arguments: json.RawMessage(`{}`)}
	tests := []struct {
		name     string
		response modelinvoker.Response
		code     string
	}{
		{"duplicate_same", response(valid, valid), "duplicate_tool_call_id"},
		{"duplicate_different", response(valid, modelinvoker.FunctionCall{ID: "call", Name: "other", Arguments: json.RawMessage(`{"x":1}`)}), "duplicate_tool_call_id"},
		{"one_invalid", response(valid, modelinvoker.FunctionCall{ID: "bad", Name: "other", Arguments: json.RawMessage(`[]`)}), "tool_call_arguments_not_object"},
		{"zero", response(), "tool_call_terminal_empty"},
		{"wrong_stop", modelinvoker.Response{Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn, Output: response(valid).Output}, "tool_call_terminal_mismatch"},
		{"incomplete", modelinvoker.Response{Status: modelinvoker.ResponseStatusIncomplete, StopReason: modelinvoker.StopReasonToolCall, Output: response(valid).Output}, "tool_call_terminal_mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := modelinvoker.FinalizeToolCallCandidateObservationV1(invocationDigest(), test.response)
			if result.Digest != "" || len(result.Calls) != 0 || errorCode(err) != test.code {
				t.Fatalf("result/error = %#v/%v, want %q", result, err, test.code)
			}
		})
	}
}

func TestCount100DigestDeterministic(t *testing.T) {
	input := response(modelinvoker.FunctionCall{ID: "call", Name: "tool", Arguments: json.RawMessage(`{"b":2,"a":1.00}`)})
	var want core.Digest
	for run := 0; run < 100; run++ {
		result, err := modelinvoker.FinalizeToolCallCandidateObservationV1(invocationDigest(), input)
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if run == 0 {
			want = result.Digest
		} else if result.Digest != want {
			t.Fatalf("run %d digest = %s, want %s", run, result.Digest, want)
		}
	}
}

func TestRace32ConcurrentFinalizeReturnsIndependentCopies(t *testing.T) {
	input := response(modelinvoker.FunctionCall{ID: "call", Name: "tool", Arguments: json.RawMessage(`{"city":"Oslo"}`)})
	results := make([]modelinvoker.ToolCallCandidateObservationV1, 32)
	errs := make([]error, 32)
	var group sync.WaitGroup
	for index := range results {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			results[index], errs[index] = modelinvoker.FinalizeToolCallCandidateObservationV1(invocationDigest(), input)
		}(index)
	}
	group.Wait()
	for index := range results {
		if errs[index] != nil || results[index].Digest != results[0].Digest {
			t.Fatalf("result %d = %#v/%v", index, results[index], errs[index])
		}
	}
	results[0].Calls[0].CanonicalArguments[0] = '['
	if string(results[1].Calls[0].CanonicalArguments) != `{"city":"Oslo"}` {
		t.Fatal("concurrent results share mutable argument storage")
	}
}

func TestBlackboxParallelInterleavingUsesTerminalOutputOrder(t *testing.T) {
	terminal := response(
		modelinvoker.FunctionCall{ID: "call-a", Name: "first", Arguments: json.RawMessage(`{"a":1}`)},
		modelinvoker.FunctionCall{ID: "call-b", Name: "second", Arguments: json.RawMessage(`{"b":2}`)},
	)
	finalizer, err := modelinvoker.NewToolCallCandidateStreamFinalizerV1(invocationDigest())
	if err != nil {
		t.Fatal(err)
	}
	events := []modelinvoker.StreamEvent{
		{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1, ResponseID: "response"},
		{Type: modelinvoker.StreamEventFunctionCallStarted, Sequence: 2, ResponseID: "response", FunctionCall: &modelinvoker.FunctionCall{ID: "call-b", Name: "second"}},
		{Type: modelinvoker.StreamEventFunctionCallStarted, Sequence: 3, ResponseID: "response", FunctionCall: &modelinvoker.FunctionCall{ID: "call-a", Name: "first"}},
		{Type: modelinvoker.StreamEventFunctionArgumentsDelta, Sequence: 4, ResponseID: "response", ArgumentsDelta: `{"b":`, FunctionCall: &modelinvoker.FunctionCall{ID: "call-b", Name: "second", Arguments: json.RawMessage(`{"b":`)}},
		{Type: modelinvoker.StreamEventFunctionArgumentsDelta, Sequence: 5, ResponseID: "response", ArgumentsDelta: `{"a":`, FunctionCall: &modelinvoker.FunctionCall{ID: "call-a", Name: "first", Arguments: json.RawMessage(`{"a":`)}},
		{Type: modelinvoker.StreamEventFunctionArgumentsDelta, Sequence: 6, ResponseID: "response", ArgumentsDelta: `2}`, FunctionCall: &modelinvoker.FunctionCall{ID: "call-b", Name: "second", Arguments: json.RawMessage(`{"b":2}`)}},
		{Type: modelinvoker.StreamEventFunctionArgumentsDelta, Sequence: 7, ResponseID: "response", ArgumentsDelta: `1}`, FunctionCall: &modelinvoker.FunctionCall{ID: "call-a", Name: "first", Arguments: json.RawMessage(`{"a":1}`)}},
		{Type: modelinvoker.StreamEventFunctionCallCompleted, Sequence: 8, ResponseID: "response", FunctionCall: &modelinvoker.FunctionCall{ID: "call-b", Name: "second", Arguments: json.RawMessage(`{"b":2}`)}},
		{Type: modelinvoker.StreamEventFunctionCallCompleted, Sequence: 9, ResponseID: "response", FunctionCall: &modelinvoker.FunctionCall{ID: "call-a", Name: "first", Arguments: json.RawMessage(`{"a":1}`)}},
		{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 10, ResponseID: "response", Response: &terminal},
	}
	var streamed *modelinvoker.ToolCallCandidateObservationV1
	for index, event := range events {
		result, observeErr := finalizer.Observe(event)
		if observeErr != nil {
			t.Fatalf("event %d: %v", index, observeErr)
		}
		if index < len(events)-1 && result != nil {
			t.Fatalf("partial event %d produced observation", index)
		}
		if result != nil {
			streamed = result
		}
	}
	synchronous, err := modelinvoker.FinalizeToolCallCandidateObservationV1(invocationDigest(), terminal)
	if err != nil || streamed == nil || streamed.Digest != synchronous.Digest || streamed.Calls[0].CallID != "call-a" || streamed.Calls[1].CallID != "call-b" {
		t.Fatalf("stream/sync = %#v/%#v/%v", streamed, synchronous, err)
	}
	streamed.Calls[0].CanonicalArguments[0] = '['
	stored, err := finalizer.Result()
	if err != nil || string(stored.Calls[0].CanonicalArguments) != `{"a":1}` {
		t.Fatalf("immutable stored result = %#v/%v", stored, err)
	}
}

func TestStreamTerminalResponseIDBindingIsExactAndUnambiguous(t *testing.T) {
	call := modelinvoker.FunctionCall{ID: "call", Name: "tool", Arguments: json.RawMessage(`{"a":1}`)}

	t.Run("empty_terminal_inherits_bound_id", func(t *testing.T) {
		terminal := response(call)
		terminal.ID = ""
		finalizer, err := modelinvoker.NewToolCallCandidateStreamFinalizerV1(invocationDigest())
		if err != nil {
			t.Fatal(err)
		}
		if result, observeErr := finalizer.Observe(modelinvoker.StreamEvent{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1, ResponseID: "response-bound"}); observeErr != nil || result != nil {
			t.Fatalf("start = %#v/%v", result, observeErr)
		}
		result, observeErr := finalizer.Observe(modelinvoker.StreamEvent{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 2, Response: &terminal})
		responseID, responseIDErr := finalizer.FinalizedResponseID()
		if observeErr != nil || result == nil || responseIDErr != nil || responseID != "response-bound" {
			t.Fatalf("inherited terminal = %#v/%v, response ID = %q/%v", result, observeErr, responseID, responseIDErr)
		}
	})

	tests := []struct {
		name       string
		startedID  string
		terminalID string
		code       string
	}{
		{name: "terminal_mismatch", startedID: "response-bound", terminalID: "response-other", code: "tool_call_stream_response_conflict"},
		{name: "terminal_empty_without_binding", code: "tool_call_stream_response_missing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			terminal := response(call)
			terminal.ID = test.terminalID
			finalizer, err := modelinvoker.NewToolCallCandidateStreamFinalizerV1(invocationDigest())
			if err != nil {
				t.Fatal(err)
			}
			sequence := int64(1)
			if test.startedID != "" {
				if _, err = finalizer.Observe(modelinvoker.StreamEvent{Type: modelinvoker.StreamEventResponseStarted, Sequence: sequence, ResponseID: test.startedID}); err != nil {
					t.Fatal(err)
				}
				sequence++
			}
			result, observeErr := finalizer.Observe(modelinvoker.StreamEvent{Type: modelinvoker.StreamEventResponseCompleted, Sequence: sequence, Response: &terminal})
			responseID, responseIDErr := finalizer.FinalizedResponseID()
			if result != nil || errorCode(observeErr) != test.code || responseID != "" || responseIDErr == nil {
				t.Fatalf("result/error/response ID = %#v/%v/%q/%v, want zero/%q", result, observeErr, responseID, responseIDErr, test.code)
			}
		})
	}
}

func TestFaultStreamFailClosedMatrix(t *testing.T) {
	terminal := response(modelinvoker.FunctionCall{ID: "call", Name: "tool", Arguments: json.RawMessage(`{"a":1}`)})
	tests := []struct {
		name   string
		events []modelinvoker.StreamEvent
		code   string
	}{
		{"bedrock_missing_correlation", []modelinvoker.StreamEvent{{Type: modelinvoker.StreamEventFunctionArgumentsDelta, Sequence: 1, ArgumentsDelta: `{}`}}, "tool_call_stream_correlation_unsupported"},
		{"delta_before_start", []modelinvoker.StreamEvent{{Type: modelinvoker.StreamEventFunctionArgumentsDelta, Sequence: 1, ArgumentsDelta: `{}`, FunctionCall: &modelinvoker.FunctionCall{ID: "call", Name: "tool", Arguments: json.RawMessage(`{}`)}}}, "tool_call_stream_delta_state_invalid"},
		{"duplicate_start", []modelinvoker.StreamEvent{{Type: modelinvoker.StreamEventFunctionCallStarted, Sequence: 1, FunctionCall: &modelinvoker.FunctionCall{ID: "call", Name: "tool"}}, {Type: modelinvoker.StreamEventFunctionCallStarted, Sequence: 2, FunctionCall: &modelinvoker.FunctionCall{ID: "call", Name: "tool"}}}, "duplicate_tool_call_id"},
		{"duplicate_complete", []modelinvoker.StreamEvent{{Type: modelinvoker.StreamEventFunctionCallStarted, Sequence: 1, FunctionCall: &modelinvoker.FunctionCall{ID: "call", Name: "tool"}}, {Type: modelinvoker.StreamEventFunctionCallCompleted, Sequence: 2, FunctionCall: &modelinvoker.FunctionCall{ID: "call", Name: "tool", Arguments: json.RawMessage(`{}`)}}, {Type: modelinvoker.StreamEventFunctionCallCompleted, Sequence: 3, FunctionCall: &modelinvoker.FunctionCall{ID: "call", Name: "tool", Arguments: json.RawMessage(`{}`)}}}, "tool_call_stream_complete_state_invalid"},
		{"partial_terminal", []modelinvoker.StreamEvent{{Type: modelinvoker.StreamEventFunctionCallStarted, Sequence: 1, FunctionCall: &modelinvoker.FunctionCall{ID: "call", Name: "tool"}}, {Type: modelinvoker.StreamEventFunctionArgumentsDelta, Sequence: 2, ArgumentsDelta: `{"a":`, FunctionCall: &modelinvoker.FunctionCall{ID: "call", Name: "tool", Arguments: json.RawMessage(`{"a":`)}}, {Type: modelinvoker.StreamEventResponseCompleted, Sequence: 3, Response: &terminal}}, "tool_call_stream_terminal_conflict"},
		{"stream_error", []modelinvoker.StreamEvent{{Type: modelinvoker.StreamEventError, Sequence: 1, Error: modelinvoker.NewError(modelinvoker.ErrorProvider, "boom")}}, "tool_call_stream_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			finalizer, err := modelinvoker.NewToolCallCandidateStreamFinalizerV1(invocationDigest())
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range test.events {
				_, err = finalizer.Observe(event)
				if err != nil {
					break
				}
			}
			result, resultErr := finalizer.Result()
			if result != nil || errorCode(err) != test.code || resultErr == nil {
				t.Fatalf("result/errors = %#v/%v/%v, want %q", result, err, resultErr, test.code)
			}
		})
	}
}

func TestUnitTerminalSnapshotRecovery(t *testing.T) {
	terminal := response(modelinvoker.FunctionCall{ID: "snapshot", Name: "tool", Arguments: json.RawMessage(`{"x":1}`)})
	finalizer, err := modelinvoker.NewToolCallCandidateStreamFinalizerV1(invocationDigest())
	if err != nil {
		t.Fatal(err)
	}
	result, err := finalizer.Observe(modelinvoker.StreamEvent{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 1, Response: &terminal})
	if err != nil || result == nil || result.Calls[0].CallID != "snapshot" {
		t.Fatalf("snapshot recovery = %#v/%v", result, err)
	}
}
