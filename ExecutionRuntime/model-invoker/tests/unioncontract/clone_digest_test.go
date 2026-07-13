package unioncontract_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func requireDigest(t *testing.T, name string, digest func() (string, error)) string {
	t.Helper()
	first, err := digest()
	if err != nil {
		t.Fatalf("%s digest: %v", name, err)
	}
	second, err := digest()
	if err != nil {
		t.Fatalf("%s second digest: %v", name, err)
	}
	if first != second {
		t.Fatalf("%s digest is unstable: %q != %q", name, first, second)
	}
	if !strings.HasPrefix(first, "sha256:") || len(first) != len("sha256:")+64 {
		t.Fatalf("%s digest has unexpected form: %q", name, first)
	}
	return first
}

func TestRequestCloneIsDeepAndDigestIsStable(t *testing.T) {
	original := validRequest()
	digest := requireDigest(t, "request", original.Digest)

	clone, err := original.Clone()
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if !reflect.DeepEqual(original, clone) {
		t.Fatal("request clone differs before mutation")
	}
	clone.Metadata["trace"] = "changed"
	clone.Input[0].Content[0].Text = "changed"
	clone.Extensions["fixture"] = json.RawMessage(`{"enabled":false}`)
	if original.Metadata["trace"] != "contract" || original.Input[0].Content[0].Text != "move the file" || string(original.Extensions["fixture"]) != `{"enabled":true}` {
		t.Fatal("request clone mutation leaked into original")
	}
	changed, err := clone.Digest()
	if err != nil {
		t.Fatalf("changed request digest: %v", err)
	}
	if changed == digest {
		t.Fatal("request digest did not change after semantic mutation")
	}
}

func TestPlanCloneIsDeepAndComputedDigestExcludesDigestField(t *testing.T) {
	original := validPlan()
	digest := requireDigest(t, "plan", original.ComputeDigest)

	withStoredDigest := original
	withStoredDigest.Digest = "sha256:stale"
	storedDigest, err := withStoredDigest.ComputeDigest()
	if err != nil {
		t.Fatalf("ComputeDigest with stored digest: %v", err)
	}
	if storedDigest != digest {
		t.Fatalf("plan digest included its Digest field: %q != %q", storedDigest, digest)
	}

	clone, err := original.Clone()
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	clone.IntentGraph.Nodes[0].Metadata["fixture"] = "changed"
	clone.Mechanisms[0].ExpectedEffects[0] = "different_effect"
	clone.ExpectedManifest.Components[0].State = "changed"
	if original.IntentGraph.Nodes[0].Metadata["fixture"] != "union-contract" || original.Mechanisms[0].ExpectedEffects[0] != "file_moved" || original.ExpectedManifest.Components[0].State != "ready" {
		t.Fatal("plan clone mutation leaked into original")
	}
	changed, err := clone.ComputeDigest()
	if err != nil {
		t.Fatalf("changed plan digest: %v", err)
	}
	if changed == digest {
		t.Fatal("plan digest did not change after semantic mutation")
	}
}

func TestEventCloneIsDeepAndDigestIsStable(t *testing.T) {
	original := validEffectEvent()
	digest := requireDigest(t, "event", original.Digest)

	clone, err := original.Clone()
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	clone.Effect.Effect.Payload.WorkspaceChange.DestinationAfter.Hash = "sha256:changed"
	if original.Effect.Effect.Payload.WorkspaceChange.DestinationAfter.Hash != "sha256:before" {
		t.Fatal("event clone mutation leaked into original")
	}
	changed, err := clone.Digest()
	if err != nil {
		t.Fatalf("changed event digest: %v", err)
	}
	if changed == digest {
		t.Fatal("event digest did not change after semantic mutation")
	}
}

func TestCommandCloneIsDeepAndDigestIsStable(t *testing.T) {
	original := validApprovalCommand()
	digest := requireDigest(t, "command", original.Digest)

	clone, err := original.Clone()
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	clone.Payload = json.RawMessage(`{"decision":"deny"}`)
	clone.ActionRevision = 2
	if string(original.Payload) != `{"decision":"approve"}` || original.ActionRevision != 1 {
		t.Fatal("command clone mutation leaked into original")
	}
	changed, err := clone.Digest()
	if err != nil {
		t.Fatalf("changed command digest: %v", err)
	}
	if changed == digest {
		t.Fatal("command digest did not change after semantic mutation")
	}
}

func TestResultCloneIsDeepAndComputedDigestExcludesDigestField(t *testing.T) {
	original := validResult()
	digest := requireDigest(t, "result", original.ComputeDigest)

	withStoredDigest := original
	withStoredDigest.Digest = "sha256:stale"
	storedDigest, err := withStoredDigest.ComputeDigest()
	if err != nil {
		t.Fatalf("ComputeDigest with stored digest: %v", err)
	}
	if storedDigest != digest {
		t.Fatalf("result digest included its Digest field: %q != %q", storedDigest, digest)
	}

	clone, err := original.Clone()
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	clone.Effects[0].Payload.WorkspaceChange.UnifiedDiff = "changed"
	clone.WorkspaceChanges[0].DestinationAfter.Hash = "sha256:changed"
	clone.ContextManifest.Components[0].State = "changed"
	if original.Effects[0].Payload.WorkspaceChange.UnifiedDiff == "changed" || original.WorkspaceChanges[0].DestinationAfter.Hash != "sha256:before" || original.ContextManifest.Components[0].State != "ready" {
		t.Fatal("result clone mutation leaked into original")
	}
	changed, err := clone.ComputeDigest()
	if err != nil {
		t.Fatalf("changed result digest: %v", err)
	}
	if changed == digest {
		t.Fatal("result digest did not change after semantic mutation")
	}
}

func TestStableDigestCanonicalizesMapInsertionOrder(t *testing.T) {
	left := map[string]any{"z": 1, "a": map[string]string{"y": "2", "x": "1"}}
	right := make(map[string]any)
	right["a"] = map[string]string{"x": "1", "y": "2"}
	right["z"] = 1

	leftDigest, err := union.StableDigest(left)
	if err != nil {
		t.Fatalf("left digest: %v", err)
	}
	rightDigest, err := union.StableDigest(right)
	if err != nil {
		t.Fatalf("right digest: %v", err)
	}
	if leftDigest != rightDigest {
		t.Fatalf("equivalent maps have different digests: %q != %q", leftDigest, rightDigest)
	}
}

func TestTypedDigestsRefuseInvalidValues(t *testing.T) {
	request := validRequest()
	request.IntentGraph.Nodes = nil
	if _, err := request.Digest(); err == nil {
		t.Fatal("invalid request produced a digest")
	}

	event := validEffectEvent()
	event.Header.EffectID = "wrong"
	if _, err := event.Digest(); err == nil {
		t.Fatal("identity-incoherent event produced a digest")
	}
}
