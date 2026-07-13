package conformance_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/effect"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestN03BeforeHashMismatchContradictsFileIntent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	observer, err := effect.NewFileObserver(effect.FilePolicy{AllowedRoots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := observer.Capture(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := observer.Capture(path)
	if err != nil {
		t.Fatal(err)
	}
	intent := union.IntentNode{ID: "intent-N03", Kind: union.IntentModifyFile, Target: path, Required: true}
	observed, err := observer.Observe("effect-N03", intent, "attempt-N03", before, after, negativeTestTime)
	if err != nil {
		t.Fatal(err)
	}
	validation, err := effect.VerifyFileEffect(observed, "verification-N03", effect.FileExpectation{
		BeforeHash: "sha256:not-the-captured-before-hash", AfterHash: after.State.Hash,
	}, negativeTestTime.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if validation.Verification.Status != union.VerificationContradicted || validation.Verification.FailureCode != "before_hash_mismatch" {
		t.Fatalf("file verification = %#v", validation.Verification)
	}
	satisfaction := effect.EvaluateIntent(intent, []union.EffectRecord{validation.Effect}, []union.VerificationRecord{validation.Verification})
	if satisfaction.Status != union.IntentContradicted {
		t.Fatalf("intent status = %q, want contradicted", satisfaction.Status)
	}
}

func TestN06InvalidStructuredOutputAfterRepairExhaustionHasNoEffect(t *testing.T) {
	intent := union.IntentNode{
		ID: "intent-N06", Kind: union.IntentProduceStructured, Target: "result", Required: true,
		Postconditions: []union.Condition{{Kind: "json_schema_valid"}},
	}
	schema := []byte(`{"type":"object","required":["ok"],"properties":{"ok":{"const":true}},"additionalProperties":false}`)
	candidates := [][]byte{
		[]byte(`{"ok":false}`),
		[]byte(`{"ok":"true"}`),
		[]byte(`{"still_wrong":true}`),
	}
	for repairAttempt, candidate := range candidates {
		_, err := effect.ValidateStructuredOutput(
			union.EffectID("effect-N06"), union.VerificationID("verification-N06"), intent.ID,
			"attempt-N06", candidate, schema, repairAttempt, negativeTestTime.Add(time.Duration(repairAttempt)*time.Second),
		)
		if !errors.Is(err, effect.ErrSchemaViolation) {
			t.Fatalf("repair attempt %d error = %v, want schema violation", repairAttempt, err)
		}
	}

	satisfaction := effect.EvaluateIntent(intent, nil, nil)
	if satisfaction.Status != union.IntentUnsatisfied || len(satisfaction.EffectIDs) != 0 {
		t.Fatalf("repair exhaustion invented success: %#v", satisfaction)
	}
}
