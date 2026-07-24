package surface_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

func TestWhiteboxSurfaceCompilerIsDeterministic(t *testing.T) {
	cap1, tool1 := testkit.Capability(), testkit.Tool()
	cap2 := cap1
	cap2.ID = "tool/another"
	cap2.Digest = ""
	var err error
	cap2, err = contract.SealCapability(cap2)
	if err != nil {
		t.Fatal(err)
	}
	tool2 := tool1
	tool2.ID = "tool/another-local"
	tool2.Capability = contract.ObjectRef{ID: string(cap2.ID), Revision: cap2.Revision, Digest: cap2.Digest}
	tool2.Digest = ""
	tool2, err = contract.SealTool(tool2)
	if err != nil {
		t.Fatal(err)
	}
	selections := []surface.Selection{
		{Capability: cap1, Tool: tool1, ModelName: "zeta", DescriptionDigest: testkit.Digest("zeta"), Visible: true, Allowed: true},
		{Capability: cap2, Tool: tool2, ModelName: "alpha", DescriptionDigest: testkit.Digest("alpha"), Visible: true, Allowed: true, PreApproved: true},
	}
	request := surface.CompileRequest{Owner: testkit.Owner(), ResolvedPlanDigest: testkit.Digest("plan"), ProfileDigest: testkit.Digest("profile"), CapabilityGrantDigest: testkit.Digest("grant"), RegistrySnapshotDigest: testkit.Digest("registry"), Dialect: "model/default", Selections: selections, Revision: 1, CreatedAt: testkit.FixedTime, ExpiresAt: testkit.FixedTime.Add(time.Hour)}
	first, err := surface.Compile(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Selections[0], request.Selections[1] = request.Selections[1], request.Selections[0]
	second, err := surface.Compile(request)
	if err != nil || first.Digest != second.Digest || first.Entries[0].ModelName != "alpha" {
		t.Fatalf("surface compile is not deterministic: %v", err)
	}
}

func TestSurfaceCompilerRejectsSetEscalation(t *testing.T) {
	request := surface.CompileRequest{Owner: testkit.Owner(), ResolvedPlanDigest: testkit.Digest("plan"), ProfileDigest: testkit.Digest("profile"), CapabilityGrantDigest: testkit.Digest("grant"), RegistrySnapshotDigest: testkit.Digest("registry"), Dialect: "model/default", Revision: 1, CreatedAt: testkit.FixedTime, ExpiresAt: testkit.FixedTime.Add(time.Hour), Selections: []surface.Selection{{Capability: testkit.Capability(), Tool: testkit.Tool(), ModelName: "bad", DescriptionDigest: testkit.Digest("bad"), Allowed: true}}}
	if _, err := surface.Compile(request); err == nil {
		t.Fatal("hidden-but-allowed entry was accepted")
	}
}

func TestSurfaceCompilerRegistryDriftCreatesNewIdentityWithoutMutatingPrior(t *testing.T) {
	request := surface.CompileRequest{
		Owner: testkit.Owner(), ResolvedPlanDigest: testkit.Digest("plan"), ProfileDigest: testkit.Digest("profile"),
		CapabilityGrantDigest: testkit.Digest("grant"), RegistrySnapshotDigest: testkit.Digest("registry-v1"), Dialect: "model/default",
		Selections: []surface.Selection{{Capability: testkit.Capability(), Tool: testkit.Tool(), ModelName: "example", DescriptionDigest: testkit.Digest("description"), Visible: true, Allowed: true}},
		Revision:   1, CreatedAt: testkit.FixedTime, ExpiresAt: testkit.FixedTime.Add(time.Hour),
	}
	first, err := surface.Compile(request)
	if err != nil {
		t.Fatal(err)
	}
	firstID, firstDigest, firstRegistry := first.ID, first.Digest, first.RegistrySnapshotDigest

	drifted := request
	drifted.RegistrySnapshotDigest = testkit.Digest("registry-v2")
	drifted.Revision = 2
	drifted.CreatedAt = request.CreatedAt.Add(time.Second)
	drifted.ExpiresAt = request.ExpiresAt.Add(time.Second)
	second, err := surface.Compile(drifted)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID || second.Digest == first.Digest || second.RegistrySnapshotDigest == first.RegistrySnapshotDigest {
		t.Fatalf("Registry drift reused the prior Surface identity: first=%+v second=%+v", first, second)
	}
	if first.ID != firstID || first.Digest != firstDigest || first.RegistrySnapshotDigest != firstRegistry {
		t.Fatal("compiling a drifted Registry Snapshot mutated the prior Surface")
	}

	again, err := surface.Compile(request)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != firstID || again.Digest != firstDigest || again.RegistrySnapshotDigest != firstRegistry {
		t.Fatalf("recompiling the original exact Snapshot did not recover the original Surface: %+v", again)
	}
}

func BenchmarkSurfaceCompile(b *testing.B) {
	request := surface.CompileRequest{
		Owner: testkit.Owner(), ResolvedPlanDigest: testkit.Digest("plan"), ProfileDigest: testkit.Digest("profile"),
		CapabilityGrantDigest: testkit.Digest("grant"), RegistrySnapshotDigest: testkit.Digest("registry"), Dialect: "model/default",
		Selections: []surface.Selection{{Capability: testkit.Capability(), Tool: testkit.Tool(), ModelName: "example", DescriptionDigest: testkit.Digest("description"), Visible: true, Allowed: true}},
		Revision:   1, CreatedAt: testkit.FixedTime, ExpiresAt: testkit.FixedTime.Add(time.Hour),
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := surface.Compile(request); err != nil {
			b.Fatal(err)
		}
	}
}
