package toolcallobservation_test

import (
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func TestToolCallObservationProjectionExactRefAndIdempotency(t *testing.T) {
	observation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(invocationDigest(), response(
		modelinvoker.FunctionCall{ID: "call", Name: "lookup", Arguments: json.RawMessage(`{"b":2,"a":1}`)},
	))
	if err != nil {
		t.Fatal(err)
	}
	first, err := modelinvoker.NewToolCallCandidateObservationProjectionV1("execution-1", 17, "response-1", observation)
	if err != nil {
		t.Fatal(err)
	}
	second, err := modelinvoker.NewToolCallCandidateObservationProjectionV1("execution-1", 17, "response-1", observation)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same exact source produced different projection refs: %#v / %#v", first.Ref, second.Ref)
	}
	if first.Ref.InvocationID != "execution-1" || first.Ref.InvocationDigest != observation.InvocationDigest || first.Ref.ObservationDigest != observation.Digest || first.Ref.Source.SourceSequence != 17 || first.Ref.Source.ResponseID != "response-1" || first.Ref.Revision != 1 {
		t.Fatalf("projection ref is incomplete: %#v", first.Ref)
	}
	if len(first.Observation.Calls) != 1 || first.Observation.Calls[0].Ordinal != 0 || first.Observation.Calls[0].CallID != "call" || first.Observation.Calls[0].Name != "lookup" || string(first.Observation.Calls[0].CanonicalArguments) != `{"a":1,"b":2}` {
		t.Fatalf("projection call is not exact: %#v", first.Observation.Calls)
	}
	payload, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := modelinvoker.DecodeToolCallCandidateObservationProjectionV1(payload)
	if err != nil || !reflect.DeepEqual(decoded, first) {
		t.Fatalf("Decode() = %#v, %v", decoded, err)
	}
	decoded.Observation.Calls[0].CanonicalArguments[2] = 'z'
	if string(first.Observation.Calls[0].CanonicalArguments) != `{"a":1,"b":2}` {
		t.Fatal("decoded projection aliased finalized observation bytes")
	}
}

func TestToolCallObservationProjectionRejectsLineageAndEncodingDrift(t *testing.T) {
	observation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(invocationDigest(), response(
		modelinvoker.FunctionCall{ID: "call", Name: "lookup", Arguments: json.RawMessage(`{}`)},
	))
	if err != nil {
		t.Fatal(err)
	}
	valid, err := modelinvoker.NewToolCallCandidateObservationProjectionV1("execution-1", 1, "response-1", observation)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*modelinvoker.ToolCallCandidateObservationProjectionV1)
	}{
		{"ref_digest", func(value *modelinvoker.ToolCallCandidateObservationProjectionV1) {
			value.Ref.Digest = value.Observation.Digest
		}},
		{"observation_digest", func(value *modelinvoker.ToolCallCandidateObservationProjectionV1) {
			value.Ref.ObservationDigest = invocationDigest()
		}},
		{"invocation_digest", func(value *modelinvoker.ToolCallCandidateObservationProjectionV1) {
			value.Ref.InvocationDigest = value.Observation.Digest
		}},
		{"source_sequence", func(value *modelinvoker.ToolCallCandidateObservationProjectionV1) { value.Ref.Source.SourceSequence++ }},
		{"call_ordinal", func(value *modelinvoker.ToolCallCandidateObservationProjectionV1) {
			value.Observation.Calls[0].Ordinal = 1
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := valid.Clone()
			test.mutate(&mutated)
			if err := mutated.Validate(); err == nil {
				t.Fatal("mutated projection validated")
			}
		})
	}
	if _, err := modelinvoker.NewToolCallCandidateObservationProjectionV1("", 1, "response", observation); err == nil {
		t.Fatal("empty invocation ID was accepted")
	}
	if _, err := modelinvoker.NewToolCallCandidateObservationProjectionV1("execution", 0, "response", observation); err == nil {
		t.Fatal("zero source sequence was accepted")
	}
	payload, _ := json.Marshal(valid)
	if _, err := modelinvoker.DecodeToolCallCandidateObservationProjectionV1(append(payload, []byte(` {}`)...)); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
	var object map[string]any
	_ = json.Unmarshal(payload, &object)
	object["unknown"] = true
	unknown, _ := json.Marshal(object)
	if _, err := modelinvoker.DecodeToolCallCandidateObservationProjectionV1(unknown); err == nil {
		t.Fatal("unknown projection field was accepted")
	}
	duplicateKeys := map[string]json.RawMessage{
		"top_level": json.RawMessage(`{"contract_version":"a","contract_version":"b","ref":{},"observation":{}}`),
		"nested":    json.RawMessage(`{"contract_version":"a","ref":{"id":"a","id":"b"},"observation":{}}`),
	}
	for name, duplicate := range duplicateKeys {
		t.Run(name+"_duplicate_key", func(t *testing.T) {
			if _, err := modelinvoker.DecodeToolCallCandidateObservationProjectionV1(duplicate); errorCode(err) != "tool_call_observation_projection_json_invalid" {
				t.Fatalf("duplicate key error = %v", err)
			}
		})
	}
}

func TestToolCallObservationProjectionConcurrentFinalizeIsDeterministic(t *testing.T) {
	observation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(invocationDigest(), response(
		modelinvoker.FunctionCall{ID: "call", Name: "lookup", Arguments: json.RawMessage(`{"value":1}`)},
	))
	if err != nil {
		t.Fatal(err)
	}
	const workers = 32
	results := make([]modelinvoker.ToolCallCandidateObservationProjectionV1, workers)
	errs := make([]error, workers)
	var wait sync.WaitGroup
	for index := range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results[index], errs[index] = modelinvoker.NewToolCallCandidateObservationProjectionV1("execution", 9, "response", observation)
		}()
	}
	wait.Wait()
	for index := range workers {
		if errs[index] != nil || !reflect.DeepEqual(results[0], results[index]) {
			t.Fatalf("worker %d projection/error = %#v/%v", index, results[index].Ref, errs[index])
		}
	}
}
