package conformance_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func TestN14ToolOwnerVisibilityAndExecutabilityRemainIndependent(t *testing.T) {
	manifest := union.ContextManifestSummary{
		ID: "manifest-N14", Version: "v1", Mode: "semantic_stable",
		Tools: union.ToolSurfaceManifest{Entries: []union.ToolSurfaceEntry{
			{
				ID: "visible-but-gated", NativeName: "inspect", Discovered: true, Registered: true,
				ModelVisible: true, Executable: false, PermissionMode: "approval_required", Owner: union.ExecutionOwnerHarness,
				Probe: union.ToolSurfaceProbe{Status: union.ToolProbeReported, EvidenceDigest: "sha256:visible-gated"},
			},
			{
				ID: "runtime-only-executable", NativeName: "verify", Discovered: true, Registered: true,
				ModelVisible: false, Executable: true, PermissionMode: "runtime_only", Owner: union.ExecutionOwnerPraxis,
				Probe: union.ToolSurfaceProbe{Status: union.ToolProbeObserved, EvidenceDigest: "sha256:runtime-executable"},
			},
		}},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("independent tool surface rejected: %v", err)
	}
	clone, err := manifest.Clone()
	if err != nil {
		t.Fatal(err)
	}
	visible := clone.Tools.Entries[0]
	runtimeOnly := clone.Tools.Entries[1]
	if visible.Owner != union.ExecutionOwnerHarness || !visible.ModelVisible || visible.Executable {
		t.Fatalf("visible gated tool axes collapsed: %#v", visible)
	}
	if runtimeOnly.Owner != union.ExecutionOwnerPraxis || runtimeOnly.ModelVisible || !runtimeOnly.Executable {
		t.Fatalf("runtime-only tool axes collapsed: %#v", runtimeOnly)
	}
	firstDigest, err := manifest.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	clone.Tools.Entries[0].Executable = true
	secondDigest, err := clone.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest == secondDigest {
		t.Fatal("executability change was lost from the manifest identity")
	}
}
