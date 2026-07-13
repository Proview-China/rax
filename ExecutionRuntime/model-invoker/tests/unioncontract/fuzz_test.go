package unioncontract_test

import (
	"encoding/json"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func FuzzUnionEventValidation(f *testing.F) {
	validEffect, err := json.Marshal(validEffectEvent())
	if err != nil {
		f.Fatalf("marshal effect seed: %v", err)
	}
	modelHeader := validHeader(union.EventFamilyModel)
	validModel, err := json.Marshal(union.UnifiedExecutionEvent{
		Header: modelHeader,
		Model:  &union.ModelEvent{Kind: "content_completed", Content: []union.ContentPart{{Kind: "text", Text: "done"}}},
	})
	if err != nil {
		f.Fatalf("marshal model seed: %v", err)
	}
	f.Add(validEffect)
	f.Add(validModel)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"header":{"family":"effect"},"effect":{},"model":{}}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		var event union.UnifiedExecutionEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return
		}
		if err := event.Validate(); err != nil {
			return
		}

		clone, err := event.Clone()
		if err != nil {
			t.Fatalf("valid event failed to clone: %v", err)
		}
		if err := clone.Validate(); err != nil {
			t.Fatalf("clone of valid event became invalid: %v", err)
		}
		first, err := event.Digest()
		if err != nil {
			t.Fatalf("valid event failed to digest: %v", err)
		}
		second, err := clone.Digest()
		if err != nil {
			t.Fatalf("valid event clone failed to digest: %v", err)
		}
		if first != second {
			t.Fatalf("clone changed digest: %q != %q", first, second)
		}

		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("valid event failed to marshal: %v", err)
		}
		var roundTrip union.UnifiedExecutionEvent
		if err := json.Unmarshal(encoded, &roundTrip); err != nil {
			t.Fatalf("valid event failed to round trip: %v", err)
		}
		if err := roundTrip.Validate(); err != nil {
			t.Fatalf("round-tripped event became invalid: %v", err)
		}
		roundTripDigest, err := roundTrip.Digest()
		if err != nil {
			t.Fatalf("round-tripped event failed to digest: %v", err)
		}
		if roundTripDigest != first {
			t.Fatalf("round trip changed digest: %q != %q", roundTripDigest, first)
		}
	})
}

func FuzzRequestControlEnvelopeValidationIsDeterministic(f *testing.F) {
	f.Add([]byte("trace"))
	f.Add([]byte("Bearer abcdefghijklmnopqrstuvwxyz012345"))
	f.Add([]byte("sk-ant-abcdefghijklmnopqrstuvwxyz012345"))
	f.Add([]byte("eyJabcdefghijk.abcdefghijkl.abcdefghijkl"))
	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) > 2048 {
			payload = payload[:2048]
		}
		request := validRequest()
		request.Metadata["trace"] = string(payload)
		extension, err := json.Marshal(map[string]any{"nested": []any{string(payload)}})
		if err != nil {
			t.Fatal(err)
		}
		request.Extensions["routing"] = extension
		first := request.Validate()
		second := request.Validate()
		if errorString(first) != errorString(second) {
			t.Fatalf("validation drifted: %v != %v", first, second)
		}
		if first != nil {
			return
		}
		firstDigest, err := request.Digest()
		if err != nil {
			t.Fatal(err)
		}
		clone, err := request.Clone()
		if err != nil {
			t.Fatal(err)
		}
		secondDigest, err := clone.Digest()
		if err != nil || firstDigest != secondDigest {
			t.Fatalf("accepted control envelope digest drifted: %q != %q, %v", firstDigest, secondDigest, err)
		}
	})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
