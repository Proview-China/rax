package effectobserver_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/effect"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestProcessEvidenceSeparatesObservedExitFromVerification(t *testing.T) {
	now := time.Unix(1_720_000_000, 0).UTC()
	exitCode := 0
	validated, err := effect.ValidateCodeExecution("effect-process", "verify-process", "intent-process", "attempt-process", effect.ProcessEvidence{
		Argv: []string{"go", "test", "./internal/config"}, RuntimeIdentity: "go1.25", ExitCode: &exitCode,
		Stdout: []byte("ok\n"), CompletedAt: now,
	}, effect.ProcessExpectation{Argv: []string{"go", "test", "./internal/config"}, RuntimeIdentity: "go1.25", AllowedExitCodes: []int{0}})
	if err != nil {
		t.Fatal(err)
	}
	if validated.Verification.Status != union.VerificationVerified || validated.Effect.Payload.CodeExecution == nil || validated.Effect.Payload.CodeExecution.ExitCode == nil {
		t.Fatalf("validated = %#v", validated)
	}
	missingExit, err := effect.ValidateCodeExecution("effect-process-2", "verify-process-2", "intent-process", "attempt-process", effect.ProcessEvidence{
		Argv: []string{"go", "test", "./internal/config"}, RuntimeIdentity: "go1.25", CompletedAt: now,
	}, effect.ProcessExpectation{AllowedExitCodes: []int{0}})
	if err != nil {
		t.Fatal(err)
	}
	if missingExit.Verification.Status != union.VerificationUnverified || missingExit.Verification.FailureCode != "exit_code_unavailable" {
		t.Fatalf("missing exit = %#v", missingExit)
	}
}

func TestIntentSatisfactionIgnoresSupersededEffects(t *testing.T) {
	intent := union.IntentNode{ID: "intent", Kind: union.IntentModifyFile, Target: "/workspace/file.go", Required: true, Postconditions: []union.Condition{{Kind: "file_hash"}}}
	stale := union.EffectRecord{ID: "stale", IntentIDs: []union.IntentID{"intent"}, Kind: "file_changed", Target: "/workspace/file.go", Payload: union.EffectPayload{WorkspaceChange: &union.WorkspaceChange{Path: "/workspace/file.go"}}, VerificationStatus: union.VerificationContradicted}
	current := union.EffectRecord{ID: "current", IntentIDs: []union.IntentID{"intent"}, Kind: "file_changed", Target: "/workspace/file.go", Payload: union.EffectPayload{WorkspaceChange: &union.WorkspaceChange{Path: "/workspace/file.go"}}, VerificationStatus: union.VerificationVerified, SupersedesEffectIDs: []union.EffectID{"stale"}}
	satisfaction := effect.EvaluateIntent(intent, []union.EffectRecord{stale, current}, nil)
	if satisfaction.Status != union.IntentSatisfied || len(satisfaction.EffectIDs) != 1 || satisfaction.EffectIDs[0] != "current" {
		t.Fatalf("satisfaction = %#v", satisfaction)
	}
}

func TestComputerUseNeverVerifiesIrreversibleActionWithoutApprovalOrEvidence(t *testing.T) {
	now := time.Unix(1_720_000_000, 0).UTC()
	denied, err := effect.ValidateComputerUse("effect-computer", "verify-computer", "intent-computer", "attempt-computer", effect.ComputerEvidence{
		Action: "send", Target: "example", CompletedAt: now,
	}, effect.ComputerExpectation{Irreversible: true, Approved: false, RequireBeforeAfter: true})
	if err != nil {
		t.Fatal(err)
	}
	if denied.Verification.Status != union.VerificationContradicted || denied.Verification.FailureCode != "irreversible_action_not_approved" {
		t.Fatalf("denied = %#v", denied)
	}
	missing, err := effect.ValidateComputerUse("effect-computer-2", "verify-computer-2", "intent-computer", "attempt-computer", effect.ComputerEvidence{
		Action: "click", Target: "example", CompletedAt: now,
	}, effect.ComputerExpectation{RequireBeforeAfter: true})
	if err != nil {
		t.Fatal(err)
	}
	if missing.Verification.Status != union.VerificationUnverified || missing.Verification.FailureCode != "state_evidence_unavailable" {
		t.Fatalf("missing = %#v", missing)
	}
}
