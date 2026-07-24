package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func discoverySnapshotV2(t *testing.T) contract.MCPCapabilitySnapshotV2 {
	t.Helper()
	server, connection := testkit.MCPServer(), testkit.MCPConnection()
	sealed, err := contract.SealMCPCapabilitySnapshotV2(contract.MCPCapabilitySnapshotV2{
		Revision:                 1,
		Server:                   contract.ObjectRef{ID: server.ID, Revision: server.Revision, Digest: server.Digest},
		Connection:               contract.ObjectRef{ID: connection.ID, Revision: connection.Revision, Digest: connection.Digest},
		ConnectionEpoch:          connection.Epoch,
		ProtocolVersion:          contract.MCPStableProtocolVersion,
		ServerInfoDigest:         testkit.Digest("server-info"),
		ServerCapabilitiesDigest: testkit.Digest("server-capabilities"),
		InstructionsDigest:       testkit.Digest("instructions"),
		Tools: []contract.MCPToolObservationV2{
			{Name: "zeta", ObjectDigest: testkit.Digest("tool-zeta"), DescriptionDigest: testkit.Digest("description-zeta"), InputSchemaDigest: testkit.Digest("input-zeta"), OutputSchemaDigest: testkit.Digest("output-zeta"), AnnotationsDigest: testkit.Digest("annotations-zeta"), MetaDigest: testkit.Digest("meta-zeta")},
			{Name: "alpha", ObjectDigest: testkit.Digest("tool-alpha"), DescriptionDigest: testkit.Digest("description-alpha"), InputSchemaDigest: testkit.Digest("input-alpha"), OutputSchemaDigest: testkit.Digest("output-alpha"), AnnotationsDigest: testkit.Digest("annotations-alpha"), MetaDigest: testkit.Digest("meta-alpha")},
		},
		Resources:       []contract.MCPResourceObservationV2{{URI: "file:///resource", Name: "resource", ObjectDigest: testkit.Digest("resource"), DescriptionDigest: testkit.Digest("resource-description"), AnnotationsDigest: testkit.Digest("resource-annotations"), MetaDigest: testkit.Digest("resource-meta")}},
		Prompts:         []contract.MCPPromptObservationV2{{Name: "prompt", ObjectDigest: testkit.Digest("prompt"), DescriptionDigest: testkit.Digest("prompt-description"), ArgumentsDigest: testkit.Digest("prompt-arguments"), MetaDigest: testkit.Digest("prompt-meta")}},
		SourceDigest:    testkit.Digest("source"),
		Conformance:     "mcp/official-go-sdk-v1",
		CreatedUnixNano: testkit.FixedTime.UnixNano(),
		ExpiresUnixNano: testkit.FixedTime.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func TestMCPCapabilitySnapshotV2SealsDeterministicallyAndClones(t *testing.T) {
	snapshot := discoverySnapshotV2(t)
	if snapshot.Tools[0].Name != "alpha" || snapshot.Tools[1].Name != "zeta" {
		t.Fatalf("tools were not normalized: %+v", snapshot.Tools)
	}
	if err := snapshot.ValidateCurrent(testkit.FixedTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	clone := contract.CloneMCPCapabilitySnapshotV2(snapshot)
	clone.Tools[0].Name = "mutated"
	if snapshot.Tools[0].Name != "alpha" {
		t.Fatal("snapshot clone aliased the source slices")
	}
	if _, err := contract.SealMCPCapabilitySnapshotV2(snapshot); err != nil {
		t.Fatalf("same canonical snapshot was not idempotent: %v", err)
	}
}

func TestMCPCapabilitySnapshotV2RejectsDuplicateAndDigestDrift(t *testing.T) {
	snapshot := discoverySnapshotV2(t)
	snapshot.Tools = append(snapshot.Tools, snapshot.Tools[0])
	snapshot.Digest, snapshot.ValidationDigest = "", ""
	if _, err := contract.SealMCPCapabilitySnapshotV2(snapshot); err == nil {
		t.Fatal("duplicate tool name was accepted")
	}

	snapshot = discoverySnapshotV2(t)
	snapshot.SourceDigest = core.DigestBytes([]byte("changed-source"))
	if err := snapshot.Validate(); err == nil {
		t.Fatal("changed source under the same sealed digest was accepted")
	}
	if err := snapshot.ValidateCurrent(time.Unix(0, snapshot.ExpiresUnixNano)); err == nil {
		t.Fatal("expired snapshot was accepted as current")
	}
}
