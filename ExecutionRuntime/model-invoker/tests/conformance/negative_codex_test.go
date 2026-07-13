package conformance_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/effect"
	codex "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/codexappserver"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestN11CodexProvisionalDiffWithoutWorkspaceChangeProducesNoFileEffect(t *testing.T) {
	mapper, err := codex.NewMapper(codex.MappingContext{
		ExecutionID: "exec-N11", Profile: union.VersionedIdentity{ID: "profile-codex", Version: "v1"},
		Route: union.VersionedIdentity{ID: "route-codex", Version: "v1"}, IntentID: "intent-N11",
		MechanismPlanID: "plan-N11", MechanismAttemptID: "attempt-N11", ApprovalTTL: time.Minute,
		Clock: func() time.Time { return negativeTestTime },
	})
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := mapper.Map(codex.NativeEvent{
		Kind: codex.NativeProvisionalDiff, Method: "turn/diff/updated", ThreadID: "thread-N11", TurnID: "turn-N11",
		Delta:  "--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+claimed\n",
		Params: []byte(`{"threadId":"thread-N11","turnId":"turn-N11"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if mapped.Effect != nil || mapped.Item == nil || mapped.Item.Item.Kind != "provisional_diff" || mapped.Item.Item.SideEffectState != union.SideEffectPossible {
		t.Fatalf("provisional diff mapping = %#v", mapped)
	}

	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
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
	after, err := observer.Capture(path)
	if err != nil {
		t.Fatal(err)
	}
	intent := union.IntentNode{ID: "intent-N11", Kind: union.IntentModifyFile, Target: path, Required: true}
	if _, err := observer.Observe("effect-N11", intent, "attempt-N11", before, after, negativeTestTime); !errors.Is(err, effect.ErrNoObservableChange) {
		t.Fatalf("unchanged workspace observation error = %v", err)
	}
	if satisfaction := effect.EvaluateIntent(intent, nil, nil); satisfaction.Status != union.IntentUnsatisfied {
		t.Fatalf("provisional diff satisfied file intent: %#v", satisfaction)
	}
}
