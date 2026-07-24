package contract_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestMCPToolMappingManifestV1ExactClosure(t *testing.T) {
	f := testkit.MCPToolMappingFixture(testkit.FixedTime)
	if err := f.Mapping.ValidateAgainst(f.Snapshot, f.Material, f.Capability, f.Tool); err != nil {
		t.Fatal(err)
	}

	t.Run("material_not_in_snapshot", func(t *testing.T) {
		snapshot := toolcontract.CloneMCPCapabilitySnapshotV3(f.Snapshot)
		snapshot.ToolMaterials[0].Material.Digest = testkit.Digest("other-material")
		snapshot.ValidationDigest, snapshot.Digest = "", ""
		snapshot, err := toolcontract.SealMCPCapabilitySnapshotV3(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		mapping := f.Mapping
		mapping.Snapshot = snapshot.ObjectRef()
		mapping.Ref = toolcontract.MCPToolMappingManifestRefV1{}
		mapping, err = toolcontract.SealMCPToolMappingManifestV1(mapping)
		if err != nil {
			t.Fatal(err)
		}
		if err := mapping.ValidateAgainst(snapshot, f.Material, f.Capability, f.Tool); err == nil {
			t.Fatal("Mapping accepted Material absent from Snapshot provenance")
		}
	})

	t.Run("tool_artifact_drift", func(t *testing.T) {
		tool := f.Tool
		tool.ArtifactDigest = testkit.Digest("other-artifact")
		tool.Digest = ""
		tool, err := toolcontract.SealTool(tool)
		if err != nil {
			t.Fatal(err)
		}
		mapping := f.Mapping
		mapping.Tool = toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}
		mapping.Ref = toolcontract.MCPToolMappingManifestRefV1{}
		mapping, err = toolcontract.SealMCPToolMappingManifestV1(mapping)
		if err != nil {
			t.Fatal(err)
		}
		if err := mapping.ValidateAgainst(f.Snapshot, f.Material, f.Capability, tool); err == nil {
			t.Fatal("Mapping accepted Tool Artifact drift")
		}
	})

	t.Run("non_mcp_mechanism", func(t *testing.T) {
		tool := f.Tool
		tool.Mechanism = toolcontract.MechanismLocal
		tool.Digest = ""
		tool, err := toolcontract.SealTool(tool)
		if err != nil {
			t.Fatal(err)
		}
		mapping := f.Mapping
		mapping.Tool = toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}
		mapping.Ref = toolcontract.MCPToolMappingManifestRefV1{}
		mapping, err = toolcontract.SealMCPToolMappingManifestV1(mapping)
		if err != nil {
			t.Fatal(err)
		}
		if err := mapping.ValidateAgainst(f.Snapshot, f.Material, f.Capability, tool); err == nil {
			t.Fatal("Mapping accepted non-MCP mechanism")
		}
	})

	t.Run("mapping_digest_drift", func(t *testing.T) {
		mapping := f.Mapping
		mapping.MappingPolicyDigest = testkit.Digest("changed-policy")
		if err := mapping.Validate(); !core.HasCategory(err, core.ErrorConflict) {
			t.Fatalf("mapping digest drift error=%v", err)
		}
	})
}
