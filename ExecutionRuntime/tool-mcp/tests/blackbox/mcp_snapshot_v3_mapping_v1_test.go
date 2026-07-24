package blackbox_test

import (
	"encoding/json"
	"testing"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestBlackboxMCPSnapshotV3MappingJSONRoundTrip(t *testing.T) {
	f := testkit.MCPToolMappingFixture(testkit.FixedTime)
	payload, err := json.Marshal(struct {
		Snapshot toolcontract.MCPCapabilitySnapshotV3  `json:"snapshot"`
		Mapping  toolcontract.MCPToolMappingManifestV1 `json:"mapping"`
	}{f.Snapshot, f.Mapping})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Snapshot toolcontract.MCPCapabilitySnapshotV3  `json:"snapshot"`
		Mapping  toolcontract.MCPToolMappingManifestV1 `json:"mapping"`
	}
	if err = json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Snapshot.ObjectRef() != f.Snapshot.ObjectRef() || decoded.Mapping.Ref != f.Mapping.Ref || decoded.Snapshot.Validate() != nil || decoded.Mapping.ValidateAgainst(decoded.Snapshot, f.Material, f.Capability, f.Tool) != nil {
		t.Fatalf("round trip drifted: %#v", decoded)
	}
}
