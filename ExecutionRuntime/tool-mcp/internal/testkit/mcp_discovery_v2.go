package testkit

import (
	"time"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

func MCPCapabilitySnapshotV2(now time.Time) toolcontract.MCPCapabilitySnapshotV2 {
	server, connection := MCPServer(), MCPConnection()
	snapshot, err := toolcontract.SealMCPCapabilitySnapshotV2(toolcontract.MCPCapabilitySnapshotV2{
		Revision:                 1,
		Server:                   toolcontract.ObjectRef{ID: server.ID, Revision: server.Revision, Digest: server.Digest},
		Connection:               toolcontract.ObjectRef{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest},
		ConnectionEpoch:          connection.Epoch,
		ProtocolVersion:          toolcontract.MCPStableProtocolVersion,
		ServerInfoDigest:         Digest("snapshot-server-info"),
		ServerCapabilitiesDigest: Digest("snapshot-server-capabilities"),
		InstructionsDigest:       Digest("snapshot-instructions"),
		Tools: []toolcontract.MCPToolObservationV2{{
			Name: "echo", ObjectDigest: Digest("snapshot-tool-echo"), DescriptionDigest: Digest("snapshot-tool-description"), InputSchemaDigest: Digest("snapshot-tool-input"), OutputSchemaDigest: Digest("snapshot-tool-output"), AnnotationsDigest: Digest("snapshot-tool-annotations"), MetaDigest: Digest("snapshot-tool-meta"),
		}},
		SourceDigest:    Digest("snapshot-source"),
		Conformance:     "mcp/official-go-sdk-v1",
		CreatedUnixNano: now.UnixNano(),
		ExpiresUnixNano: now.Add(30 * time.Second).UnixNano(),
	})
	if err != nil {
		panic(err)
	}
	return snapshot
}
