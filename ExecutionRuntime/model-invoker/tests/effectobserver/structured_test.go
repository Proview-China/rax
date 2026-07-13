package effectobserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/effect"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestStructuredOutputStrictSchemaAndCanonicalDigest(t *testing.T) {
	now := time.Unix(1_720_000_000, 0).UTC()
	schema := []byte(`{"type":"object","required":["status"],"properties":{"status":{"const":"succeeded"}},"additionalProperties":false}`)
	first, err := effect.ValidateStructuredOutput("effect-structured", "verify-structured", "intent-structured", "attempt-structured", []byte(`{"status":"succeeded"}`), schema, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Effect.VerificationStatus != union.VerificationVerified || first.Verification.Status != union.VerificationVerified {
		t.Fatalf("first = %#v", first)
	}
	if first.Effect.Payload.StructuredOutput == nil || first.Effect.Payload.StructuredOutput.RepairAttempts != 1 {
		t.Fatalf("payload = %#v", first.Effect.Payload)
	}
	reordered := []byte(`{"additionalProperties":false,"properties":{"status":{"const":"succeeded"}},"required":["status"],"type":"object"}`)
	second, err := effect.ValidateStructuredOutput("effect-structured-2", "verify-structured-2", "intent-structured", "attempt-structured", []byte(`{"status":"succeeded"}`), reordered, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Effect.Payload.StructuredOutput.SchemaDigest != second.Effect.Payload.StructuredOutput.SchemaDigest {
		t.Fatalf("schema digest is not canonical: %q != %q", first.Effect.Payload.StructuredOutput.SchemaDigest, second.Effect.Payload.StructuredOutput.SchemaDigest)
	}
}

func TestStructuredOutputRepairIsBoundedAndOnlyEmitsValidatedEffect(t *testing.T) {
	now := time.Unix(1_720_000_000, 0).UTC()
	schema := []byte(`{"type":"object","required":["status"],"properties":{"status":{"const":"succeeded"}},"additionalProperties":false}`)
	mechanism := effect.StructuredMechanism{Kind: union.StructuredEmulatedSchema, Origin: union.CapabilityOriginEmulated, Fidelity: union.SemanticFidelityTransformed}
	var attempts int
	validated, err := effect.ValidateStructuredOutputWithRepair(
		context.Background(), "effect-repaired", "verify-repaired", "intent-repaired", "attempt-repaired",
		[]byte(`{"status":"failed"}`), schema, mechanism, 2,
		effect.StructuredRepairFunc(func(_ context.Context, attempt int, invalid []byte, contract json.RawMessage) ([]byte, error) {
			attempts++
			if attempt != attempts || len(invalid) == 0 || len(contract) == 0 {
				t.Fatalf("repair input attempt=%d calls=%d", attempt, attempts)
			}
			return []byte(`{"status":"succeeded"}`), nil
		}), now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || validated.Effect.Payload.StructuredOutput == nil || validated.Effect.Payload.StructuredOutput.RepairAttempts != 1 {
		t.Fatalf("repair result attempts=%d effect=%#v", attempts, validated.Effect)
	}

	attempts = 0
	_, err = effect.ValidateStructuredOutputWithRepair(
		context.Background(), "effect-exhausted", "verify-exhausted", "intent-exhausted", "attempt-exhausted",
		[]byte(`not-json`), schema, mechanism, 2,
		effect.StructuredRepairFunc(func(context.Context, int, []byte, json.RawMessage) ([]byte, error) {
			attempts++
			return []byte(`{"status":"still-wrong"}`), nil
		}), now,
	)
	if !errors.Is(err, effect.ErrRepairExhausted) || attempts != 2 {
		t.Fatalf("repair exhaustion err=%v attempts=%d", err, attempts)
	}
}

func TestStructuredOutputRejectsDuplicateKeysViolationAndExternalRef(t *testing.T) {
	now := time.Unix(1_720_000_000, 0).UTC()
	schema := []byte(`{"type":"object","required":["status"],"properties":{"status":{"const":"succeeded"}},"additionalProperties":false}`)
	_, err := effect.ValidateStructuredOutput("e", "v", "i", "a", []byte(`{"status":"succeeded","status":"failed"}`), schema, 0, now)
	if !errors.Is(err, effect.ErrInvalidJSON) {
		t.Fatalf("duplicate key error = %v", err)
	}
	_, err = effect.ValidateStructuredOutput("e", "v", "i", "a", []byte(`{"status":"failed"}`), schema, 0, now)
	if !errors.Is(err, effect.ErrSchemaViolation) {
		t.Fatalf("schema violation error = %v", err)
	}
	_, err = effect.ValidateStructuredOutput("e", "v", "i", "a", []byte(`{"status":"succeeded"}`), []byte(`{"$ref":"https://example.invalid/schema.json"}`), 0, now)
	if !errors.Is(err, effect.ErrInvalidSchema) || !errors.Is(err, effect.ErrExternalSchemaRef) {
		t.Fatalf("external ref error = %v", err)
	}
}
