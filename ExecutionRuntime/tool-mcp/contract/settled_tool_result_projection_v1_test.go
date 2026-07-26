package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestSettledToolResultProjectionV1ArtifactExactAndCurrent(t *testing.T) {
	now := time.Unix(500, 0)
	fixture := testkit.ApplicationG6AFixture(now)
	artifact := toolcontract.ObjectRef{ID: "artifact-v1", Revision: 1, Digest: fixture.ToolResult.PayloadDigest}
	result := fixture.ToolResult
	result.Artifacts = []toolcontract.ObjectRef{artifact}
	result, err := toolcontract.SealToolResultV2(result)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := toolcontract.SealSettledToolResultProjectionV1(toolcontract.SettledToolResultProjectionV1{
		Result:     toolcontract.ObjectRef{ID: result.ID, Revision: result.Revision, Digest: result.Digest},
		Tool:       toolcontract.ObjectRef{ID: "praxis.core-tool/workspace-read-local-v1", Revision: 1, Digest: core.DigestBytes([]byte("tool"))},
		Inspection: result.Inspection, Schema: result.Schema, PayloadDigest: result.PayloadDigest,
		PayloadRevision: result.PayloadRevision, Artifact: &artifact, Complete: true,
		Classification:  core.DigestBytes([]byte("classification")),
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ValidateCurrent(result, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	clone := projection.Clone()
	clone.Artifact.ID = "different"
	if projection.Artifact.ID == clone.Artifact.ID {
		t.Fatal("projection clone aliases artifact")
	}
}

func TestSettledToolResultProjectionV1RejectsDriftAndExpiry(t *testing.T) {
	now := time.Unix(500, 0)
	fixture := testkit.ApplicationG6AFixture(now)
	artifact := toolcontract.ObjectRef{ID: "artifact-v1", Revision: 1, Digest: fixture.ToolResult.PayloadDigest}
	result := fixture.ToolResult
	result.Artifacts = []toolcontract.ObjectRef{artifact}
	result, err := toolcontract.SealToolResultV2(result)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := toolcontract.SealSettledToolResultProjectionV1(toolcontract.SettledToolResultProjectionV1{
		Result:     toolcontract.ObjectRef{ID: result.ID, Revision: result.Revision, Digest: result.Digest},
		Tool:       toolcontract.ObjectRef{ID: "praxis.core-tool/workspace-read-local-v1", Revision: 1, Digest: core.DigestBytes([]byte("tool"))},
		Inspection: result.Inspection, Schema: result.Schema, PayloadDigest: result.PayloadDigest,
		PayloadRevision: result.PayloadRevision, Artifact: &artifact, Complete: true,
		Classification:  core.DigestBytes([]byte("classification")),
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(5 * time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	drift := projection
	drift.PayloadRevision++
	if err := drift.ValidateCurrent(result, now.Add(time.Second)); err == nil {
		t.Fatal("payload drift must fail closed")
	}
	if err := projection.ValidateCurrent(result, now.Add(6*time.Second)); err == nil {
		t.Fatal("expired projection must fail closed")
	}
}
