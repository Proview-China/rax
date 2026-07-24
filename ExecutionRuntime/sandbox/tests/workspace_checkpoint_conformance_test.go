package sandbox_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceCheckpointConformanceOwnerSurfaceHasNoRuntimeOrProviderPower(t *testing.T) {
	t.Parallel()
	typeOf := reflect.TypeOf((*ports.WorkspaceCheckpointParticipantOwnerPortV2)(nil)).Elem()
	want := map[string]bool{"PrepareWorkspaceCheckpointParticipantV2": true, "InspectWorkspaceCheckpointPreparedV2": true}
	if typeOf.NumMethod() != len(want) {
		t.Fatalf("Workspace Checkpoint Owner public method count=%d want=%d", typeOf.NumMethod(), len(want))
	}
	for index := 0; index < typeOf.NumMethod(); index++ {
		method := typeOf.Method(index)
		if !want[method.Name] {
			t.Fatalf("unexpected Workspace Checkpoint Owner method %s", method.Name)
		}
		for _, forbidden := range []string{"Runtime", "Provider", "Execute", "Activate", "Restore", "Settlement", "Evidence", "Permit", "Fence", "CAS"} {
			if strings.Contains(method.Name, forbidden) {
				t.Fatalf("Workspace Checkpoint Owner method %s exposes forbidden capability", method.Name)
			}
		}
	}
	if !ports.Supported(ports.FeatureCheckpointParticipant) || !ports.Supported(ports.FeatureCheckpointRestore) {
		t.Fatal("module-level production composition omitted implemented Checkpoint/Restore support")
	}
}

func TestWorkspaceCheckpointConformanceCallerCannotSupplyCurrentAggregate(t *testing.T) {
	t.Parallel()
	requestType := reflect.TypeOf(contract.PrepareWorkspaceCheckpointParticipantRequestV2{})
	if _, ok := requestType.FieldByName("SnapshotAggregateRef"); ok {
		t.Fatal("caller request can inject trusted Snapshot aggregate current")
	}
	projectionType := reflect.TypeOf(contract.WorkspaceCheckpointPreparationCurrentProjectionV2{})
	if _, ok := projectionType.FieldByName("SnapshotAggregateRef"); !ok {
		t.Fatal("Owner current projection does not carry exact Snapshot aggregate current")
	}
	for _, forbidden := range []string{"Trusted", "Current", "Sequence", "Outcome", "Verdict", "Authorization", "Permit", "Receipt"} {
		if _, ok := requestType.FieldByName(forbidden); ok {
			t.Fatalf("caller request carries forbidden trusted field %s", forbidden)
		}
	}
}
