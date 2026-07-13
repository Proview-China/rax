package effectobserver_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/effect"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestToolCallEffectPreservesOwnershipAndSatisfiesMatchingIntent(t *testing.T) {
	now := time.Date(2026, 7, 13, 3, 0, 0, 0, time.UTC)
	validated, err := effect.ValidateToolCall(
		"effect-tool", "verify-tool", "intent-tool", "attempt-tool",
		effect.ToolCallEvidence{
			ToolID: "workspace.read", ActionID: "action-tool", Mechanism: "caller_function",
			Origin: union.CapabilityOriginCallerHosted, Owner: union.ExecutionOwnerPraxis,
			Input: []byte(`{"path":"config.go"}`), Output: []byte(`{"ok":true}`),
			ResultOrigin: union.EventOriginExternal, SideEffectState: union.SideEffectNone, CompletedAt: now,
		},
		effect.ToolCallExpectation{ToolID: "workspace.read", ActionID: "action-tool", Owner: union.ExecutionOwnerPraxis, RequireOutput: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := validated.Effect.Validate(); err != nil {
		t.Fatalf("Effect.Validate: %v", err)
	}
	if validated.Effect.Payload.ToolCall == nil || !validated.Effect.Payload.ToolCall.Executed ||
		validated.Effect.Payload.ToolCall.InputDigest == "" || validated.Effect.Payload.ToolCall.OutputDigest == "" {
		t.Fatalf("tool Effect = %#v", validated.Effect)
	}
	intent := union.IntentNode{ID: "intent-tool", Kind: union.IntentCallTool, Target: "workspace.read", Required: true}
	satisfaction := effect.EvaluateIntent(intent, []union.EffectRecord{validated.Effect}, []union.VerificationRecord{validated.Verification})
	if satisfaction.Status != union.IntentSatisfied {
		t.Fatalf("satisfaction = %#v", satisfaction)
	}
}

func TestToolCallWrongOwnerIsContradictedAndWrongEffectKindCannotSatisfy(t *testing.T) {
	now := time.Date(2026, 7, 13, 3, 1, 0, 0, time.UTC)
	validated, err := effect.ValidateToolCall(
		"effect-tool", "verify-tool", "intent-tool", "attempt-tool",
		effect.ToolCallEvidence{
			ToolID: "workspace.write", ActionID: "action-tool", Mechanism: "harness_tool",
			Origin: union.CapabilityOriginHarnessHosted, Owner: union.ExecutionOwnerHarness,
			Input: []byte(`{"path":"config.go"}`), ResultOrigin: union.EventOriginHarness,
			SideEffectState: union.SideEffectPossible, CompletedAt: now,
		},
		effect.ToolCallExpectation{ToolID: "workspace.write", Owner: union.ExecutionOwnerPraxis, RequireOutput: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if validated.Verification.Status != union.VerificationContradicted || validated.Verification.FailureCode != "execution_owner_mismatch" {
		t.Fatalf("verification = %#v", validated.Verification)
	}
	fileIntent := union.IntentNode{ID: "intent-tool", Kind: union.IntentModifyFile, Target: "/workspace/config.go", Required: true}
	satisfaction := effect.EvaluateIntent(fileIntent, []union.EffectRecord{validated.Effect}, []union.VerificationRecord{validated.Verification})
	if satisfaction.Status != union.IntentUnsatisfied {
		t.Fatalf("wrong Effect kind satisfied file intent: %#v", satisfaction)
	}
}
