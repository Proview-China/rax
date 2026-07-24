package testkit

import (
	"time"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type MCPToolMappingFixtureV1 struct {
	Snapshot   toolcontract.MCPCapabilitySnapshotV3
	Material   toolcontract.MCPToolDiscoveryMaterialV1
	Capability toolcontract.CapabilityDescriptor
	Tool       toolcontract.ToolDescriptor
	Mapping    toolcontract.MCPToolMappingManifestV1
}

func MCPToolMappingFixture(now time.Time) MCPToolMappingFixtureV1 {
	material := MCPToolDiscoveryMaterialV1()
	snapshot := MCPCapabilitySnapshotV3(now)
	capability := Capability()
	capability.InputSchema.ContentDigest = material.Source.InputSchemaDigest
	capability.OutputSchema.ContentDigest = material.Source.OutputSchemaDigest
	capability.CreatedUnixNano = now.UnixNano()
	capability.Digest = ""
	capability, err := toolcontract.SealCapability(capability)
	if err != nil {
		panic(err)
	}
	tool := Tool()
	tool.Owner = capability.Owner
	tool.Capability = toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
	tool.ArtifactDigest = material.Ref.Digest
	tool.Mechanism = toolcontract.MechanismMCP
	tool.InputSchema = capability.InputSchema
	tool.OutputSchema = capability.OutputSchema
	tool.CreatedUnixNano = now.UnixNano()
	tool.Digest = ""
	tool, err = toolcontract.SealTool(tool)
	if err != nil {
		panic(err)
	}
	mapping, err := toolcontract.SealMCPToolMappingManifestV1(toolcontract.MCPToolMappingManifestV1{
		Owner: tool.Owner, Snapshot: snapshot.ObjectRef(), Source: material.Source, SourceMaterial: material.Ref,
		Capability:    toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest},
		Tool:          toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest},
		MappingPolicy: "praxis.tool/mcp-explicit-mapping-v1", MappingPolicyDigest: Digest("mcp-explicit-mapping-policy"),
		CreatedUnixNano: now.UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return MCPToolMappingFixtureV1{Snapshot: snapshot, Material: material, Capability: capability, Tool: tool, Mapping: mapping}
}
