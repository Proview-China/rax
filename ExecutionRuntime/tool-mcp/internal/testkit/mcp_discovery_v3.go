package testkit

import (
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func MCPCapabilitySnapshotV3(now time.Time) toolcontract.MCPCapabilitySnapshotV3 {
	material := MCPToolDiscoveryMaterialV1()
	set := MCPDiscoveryPageToolMaterialSetV1()
	snapshot, err := toolcontract.SealMCPCapabilitySnapshotV3(toolcontract.MCPCapabilitySnapshotV3{
		Revision:                 1,
		Server:                   toolcontract.ObjectRef{ID: "test-server", Revision: 1, Digest: Digest("test-server")},
		Connection:               toolcontract.ObjectRef{ID: material.Connection.ID, Revision: material.Connection.Revision, Digest: material.Connection.Digest},
		ConnectionEpoch:          1,
		ProtocolVersion:          toolcontract.MCPStableProtocolVersion,
		ServerInfoDigest:         Digest("snapshot-v3-server-info"),
		ServerCapabilitiesDigest: Digest("snapshot-v3-server-capabilities"),
		InstructionsDigest:       Digest("snapshot-v3-instructions"),
		Tools:                    []toolcontract.MCPToolObservationV2{material.Source},
		Pages: []toolcontract.MCPDiscoveryPageProvenanceV3{{
			Namespace: runtimeports.MCPDiscoveryPageToolsNamespaceV1, PageOrdinal: 0, Command: material.Command,
			ProtocolReceipt: set.Receipt, ApplySettlement: toolcontract.ObjectRef{ID: "test-page-apply", Revision: 1, Digest: Digest("test-page-apply")},
			ResponsePageDigest: set.ResponsePageDigest, MaterialSet: set.Ref,
		}},
		ToolMaterials: []toolcontract.MCPToolMaterialProvenanceV3{{Source: material.Source, PageReceipt: set.Receipt, Material: material.Ref}},
		SourceDigest:  Digest("snapshot-v3-source"), Conformance: "mcp/official-go-sdk-v1",
		CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return snapshot
}
