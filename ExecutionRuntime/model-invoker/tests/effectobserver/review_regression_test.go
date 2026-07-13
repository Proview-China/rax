package effectobserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/effect"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestMoveObserverBindsEffectToRequestedDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	actualDestination := filepath.Join(root, "actual.txt")
	requestedDestination := filepath.Join(root, "requested.txt")
	if err := os.WriteFile(source, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	observer, err := effect.NewFileObserver(effect.FilePolicy{AllowedRoots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	sourceBefore, _ := observer.Capture(source)
	destinationBefore, _ := observer.Capture(actualDestination)
	if err := os.Rename(source, actualDestination); err != nil {
		t.Fatal(err)
	}
	sourceAfter, _ := observer.Capture(source)
	destinationAfter, _ := observer.Capture(actualDestination)
	intent := union.IntentNode{
		ID: "move", Kind: union.IntentMoveFile, Target: source,
		Specification: json.RawMessage(`{"destination":` + strconv.Quote(requestedDestination) + `}`),
	}
	_, err = observer.ObserveMove("effect", intent, "attempt", sourceBefore, sourceAfter, destinationBefore, destinationAfter, time.Now())
	if !errors.Is(err, effect.ErrIntentMismatch) {
		t.Fatalf("wrong move destination error = %v", err)
	}
}

func TestIntentSatisfactionRequiresTheCorrectPayloadArmAndMoveDestination(t *testing.T) {
	move := union.IntentNode{
		ID: "move", Kind: union.IntentMoveFile, Target: "/workspace/source.txt",
		Specification: json.RawMessage(`{"destination":"/workspace/wanted.txt"}`),
	}
	wrongDestination := union.EffectRecord{
		ID: "wrong", IntentIDs: []union.IntentID{"move"}, Kind: "file_moved", Target: move.Target,
		Payload:            union.EffectPayload{WorkspaceChange: &union.WorkspaceChange{Path: move.Target, Destination: "/workspace/other.txt"}},
		VerificationStatus: union.VerificationVerified,
	}
	if got := effect.EvaluateIntent(move, []union.EffectRecord{wrongDestination}, nil); got.Status != union.IntentUnsatisfied {
		t.Fatalf("wrong destination satisfied move: %#v", got)
	}

	file := union.IntentNode{ID: "file", Kind: union.IntentModifyFile, Target: "/workspace/file.go"}
	wrongArm := union.EffectRecord{
		ID: "wrong-arm", IntentIDs: []union.IntentID{"file"}, Kind: "file_changed", Target: file.Target,
		Payload: union.EffectPayload{Extension: json.RawMessage(`{"claimed":true}`)}, VerificationStatus: union.VerificationVerified,
	}
	if got := effect.EvaluateIntent(file, []union.EffectRecord{wrongArm}, nil); got.Status != union.IntentUnsatisfied {
		t.Fatalf("wrong payload arm satisfied file intent: %#v", got)
	}
}

func TestUnrelatedEffectCannotSuppressAnotherIntentDuringSatisfaction(t *testing.T) {
	intent := union.IntentNode{ID: "wanted", Kind: union.IntentModifyFile, Target: "/workspace/file.go"}
	valid := union.EffectRecord{
		ID: "valid", IntentIDs: []union.IntentID{"wanted"}, Kind: "file_changed", Target: intent.Target,
		Payload: union.EffectPayload{WorkspaceChange: &union.WorkspaceChange{Path: intent.Target}}, VerificationStatus: union.VerificationVerified,
	}
	unrelated := union.EffectRecord{
		ID: "unrelated", IntentIDs: []union.IntentID{"other"}, Kind: "file_changed", Target: "/workspace/other.go",
		Payload:            union.EffectPayload{WorkspaceChange: &union.WorkspaceChange{Path: "/workspace/other.go"}},
		VerificationStatus: union.VerificationVerified, SupersedesEffectIDs: []union.EffectID{"valid"},
	}
	if got := effect.EvaluateIntent(intent, []union.EffectRecord{valid, unrelated}, nil); got.Status != union.IntentSatisfied {
		t.Fatalf("unrelated supersession suppressed intent: %#v", got)
	}
}

func TestComputerUseRequiresObservedAfterStateOrExternalReadback(t *testing.T) {
	now := time.Unix(1_720_000_000, 0).UTC()
	missing, err := effect.ValidateComputerUse("effect", "verify", "intent", "attempt", effect.ComputerEvidence{
		Action: "click", Target: "button#save", CompletedAt: now,
	}, effect.ComputerExpectation{})
	if err != nil {
		t.Fatal(err)
	}
	if missing.Verification.Status != union.VerificationUnverified || missing.Verification.FailureCode != "effect_evidence_unavailable" {
		t.Fatalf("missing evidence = %#v", missing)
	}
	evidence := union.EvidenceRef{Kind: "screenshot", Source: "desktop_observer", Digest: "sha256:after", CapturedAt: now, Sensitivity: "internal"}
	verified, err := effect.ValidateComputerUse("effect-2", "verify-2", "intent", "attempt", effect.ComputerEvidence{
		Action: "click", Target: "button#save", AfterRefs: []union.EvidenceRef{evidence}, CompletedAt: now,
	}, effect.ComputerExpectation{})
	if err != nil || verified.Verification.Status != union.VerificationVerified {
		t.Fatalf("after-state evidence was not verified: %#v, %v", verified, err)
	}
}

func TestToolObserverRejectsModelClaimAsExecutionEvidence(t *testing.T) {
	_, err := effect.ValidateToolCall("effect", "verify", "intent", "attempt", effect.ToolCallEvidence{
		ToolID: "workspace.read", ActionID: "action", Mechanism: "model_claim", Origin: union.CapabilityOriginNative,
		Owner: union.ExecutionOwnerModel, Input: []byte(`{"path":"x"}`), ResultOrigin: union.EventOriginModel,
		SideEffectState: union.SideEffectNone, CompletedAt: time.Now(),
	}, effect.ToolCallExpectation{ToolID: "workspace.read"})
	if !errors.Is(err, effect.ErrInvalidPolicy) {
		t.Fatalf("model-origin tool evidence error = %v", err)
	}
}

func TestTypedNilRepairFunctionReturnsErrorInsteadOfPanicking(t *testing.T) {
	var repair effect.StructuredRepairFunc
	_, err := effect.ValidateStructuredOutputWithRepair(
		context.Background(), "effect", "verify", "intent", "attempt", []byte(`not-json`),
		json.RawMessage(`{"type":"object"}`),
		effect.StructuredMechanism{Kind: union.StructuredEmulatedSchema, Origin: union.CapabilityOriginEmulated, Fidelity: union.SemanticFidelityTransformed},
		1, repair, time.Now(),
	)
	if !errors.Is(err, effect.ErrRepairExhausted) || !errors.Is(err, effect.ErrInvalidPolicy) {
		t.Fatalf("typed nil repair error = %v", err)
	}
}

func TestLiteralRedactorOwnsInputsAndOutputs(t *testing.T) {
	secret := []byte("credential-material")
	redactor, err := effect.NewLiteralRedactor(secret)
	if err != nil {
		t.Fatal(err)
	}
	secret[0] = 'X'
	input := []byte("credential-material")
	redacted := redactor.Redact(input)
	input[0] = 'X'
	if string(redacted) != "[REDACTED]" {
		t.Fatalf("redactor alias or input-copy failure: %q", redacted)
	}
}
