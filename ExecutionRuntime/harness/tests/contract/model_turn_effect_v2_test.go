package contract_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestModelTurnEffectPayloadV2RoundTripsExactCustomCandidate(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	_, candidate := testkit.GovernedFactsV2(now)
	candidate.Provider.ComponentID = "custom.ninth/model-provider"
	candidate.Provider.Capability = "custom.ninth/model-turn"
	payload, err := contract.NewModelTurnEffectPayloadV2(candidate)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := contract.DecodeModelTurnEffectPayloadV2(payload, now)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := candidate.DigestV2()
	if decoded.CandidateDigest != digest || decoded.Candidate.Provider != candidate.Provider {
		t.Fatalf("custom candidate drifted across Effect envelope: %#v", decoded)
	}
	payload2, err := contract.NewModelTurnEffectPayloadV2(candidate)
	if err != nil || !bytes.Equal(payload.Inline, payload2.Inline) || payload.ContentDigest != payload2.ContentDigest {
		t.Fatalf("model turn payload encoding is not deterministic: err=%v", err)
	}
}

func TestModelTurnEffectPayloadV2RejectsSchemaBodyAndCandidateDrift(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	_, candidate := testkit.GovernedFactsV2(now)
	payload, err := contract.NewModelTurnEffectPayloadV2(candidate)
	if err != nil {
		t.Fatal(err)
	}

	wrongSchema := payload
	wrongSchema.Schema.Name = "other"
	if _, err := contract.DecodeModelTurnEffectPayloadV2(wrongSchema, now); !core.HasReason(err, core.ReasonUnknownSchema) {
		t.Fatalf("wrong schema was accepted: %v", err)
	}

	tampered := payload
	tampered.Inline = append([]byte(nil), payload.Inline...)
	tampered.Inline[len(tampered.Inline)-2] ^= 1
	if _, err := contract.DecodeModelTurnEffectPayloadV2(tampered, now); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("tampered body was accepted: %v", err)
	}

	expired := payload
	if _, err := contract.DecodeModelTurnEffectPayloadV2(expired, now.Add(2*time.Hour)); !core.HasReason(err, core.ReasonCapabilityExpired) {
		t.Fatalf("expired candidate was accepted: %v", err)
	}
}

func TestModelTurnCandidateV2EncodedEnvelopeBudgetIsClosedAtValidation(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	_, template := testkit.GovernedFactsV2(now)

	withInput := func(size int) contract.ModelTurnCandidateV2 {
		candidate := template
		body := bytes.Repeat([]byte{'x'}, size)
		candidate.Input.Inline = body
		candidate.Input.Ref = ""
		candidate.Input.Length = uint64(len(body))
		candidate.Input.ContentDigest = core.DigestBytes(body)
		return candidate
	}

	low, high := 1, runtimeports.MaxOpaqueInlineBytes
	for low < high {
		mid := low + (high-low+1)/2
		if withInput(mid).Validate(now) == nil {
			low = mid
		} else {
			high = mid - 1
		}
	}
	maxInput := low
	if maxInput >= runtimeports.MaxOpaqueInlineBytes {
		t.Fatal("candidate validation did not reserve JSON/base64 envelope overhead")
	}

	for _, size := range []int{1, 1024, maxInput} {
		candidate := withInput(size)
		if err := candidate.Validate(now); err != nil {
			t.Fatalf("input size %d should fit the complete envelope: %v", size, err)
		}
		payload, err := contract.NewModelTurnEffectPayloadV2(candidate)
		if err != nil {
			t.Fatalf("Validate accepted size %d but Effect encoding failed: %v", size, err)
		}
		if len(payload.Inline) > contract.MaxModelTurnEffectEnvelopeBytesV2 {
			t.Fatalf("encoded payload exceeded its budget: %d", len(payload.Inline))
		}
	}

	tooLarge := withInput(maxInput + 1)
	if err := tooLarge.Validate(now); !core.HasReason(err, core.ReasonCanonicalLimitExceeded) {
		t.Fatalf("first over-budget candidate did not fail at Validate: %v", err)
	}
}

func TestModelTurnCandidateV2ReferencedAndEmptyInputsFailClosed(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	_, candidate := testkit.GovernedFactsV2(now)

	referenced := candidate
	referenced.Input.Inline = nil
	referenced.Input.Ref = "object://tenant/model-input"
	if err := referenced.Input.Validate(); err != nil {
		t.Fatalf("fixture must isolate the model-turn reference policy: %v", err)
	}
	if err := referenced.Validate(now); !core.HasReason(err, core.ReasonUnknownSchema) {
		t.Fatalf("referenced input was not rejected without a governed resolver: %v", err)
	}

	empty := candidate
	empty.Input.Inline = []byte{}
	empty.Input.Ref = ""
	empty.Input.Length = 0
	empty.Input.ContentDigest = core.DigestBytes(nil)
	if err := empty.Validate(now); !core.HasReason(err, core.ReasonCanonicalLimitExceeded) {
		t.Fatalf("empty inline input was not rejected canonically: %v", err)
	}
}
