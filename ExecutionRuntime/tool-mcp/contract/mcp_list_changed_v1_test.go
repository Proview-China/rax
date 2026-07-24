package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestMCPListChangedObservationV1CanonicalAndCurrent(t *testing.T) {
	connection := testkit.MCPConnection()
	snapshot := testkit.MCPSnapshot()
	observation, err := contract.SealMCPListChangedObservationV1(contract.MCPListChangedObservationV1{
		Connection:       connection,
		Snapshot:         contract.ObjectRef{ID: snapshot.ID, Revision: snapshot.Revision, Digest: snapshot.Digest},
		Namespace:        contract.MCPListChangedToolsV1,
		SourceSequence:   1,
		ObservedUnixNano: testkit.FixedTime.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := observation.ValidateCurrent(testkit.FixedTime.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	changed := observation
	changed.Namespace = contract.MCPListChangedPromptsV1
	if err := changed.Validate(); err == nil {
		t.Fatal("same Ref accepted another list-changed namespace")
	}
	if err := observation.ValidateCurrent(testkit.FixedTime.Add(-time.Nanosecond)); err == nil {
		t.Fatal("list-changed current validation accepted clock rollback")
	}
}
