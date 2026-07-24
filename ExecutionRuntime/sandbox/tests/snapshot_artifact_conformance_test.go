package sandbox_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestSnapshotArtifactConformancePublicSurfaceIsGovernedOwnerOnly(t *testing.T) {
	t.Parallel()
	typeOf := reflect.TypeOf((*ports.SnapshotArtifactOwnerPortV2)(nil)).Elem()
	want := map[string]bool{
		"ReserveArtifact":               true,
		"CommitArtifact":                true,
		"InspectReservation":            true,
		"InspectReservationByStableKey": true,
		"InspectAggregateHistorical":    true,
		"InspectAggregateCurrent":       true,
		"InspectEntryHistorical":        true,
		"InspectArtifactFact":           true,
	}
	if typeOf.NumMethod() != len(want) {
		t.Fatalf("public method count=%d want=%d", typeOf.NumMethod(), len(want))
	}
	for index := 0; index < typeOf.NumMethod(); index++ {
		method := typeOf.Method(index)
		if !want[method.Name] {
			t.Fatalf("unexpected public method %s", method.Name)
		}
		for _, forbidden := range []string{"Apply", "CAS", "Evidence", "Settlement", "Provider", "Retention", "Delete", "Purge"} {
			if strings.Contains(method.Name, forbidden) {
				t.Fatalf("public method %s exposes forbidden capability", method.Name)
			}
		}
	}
}

func TestSnapshotArtifactConformanceStableIdentityIsNotCurrent(t *testing.T) {
	t.Parallel()
	typeOf := reflect.TypeOf(contract.SnapshotArtifactSubjectIdentityV2{})
	for _, forbidden := range []string{"Revision", "ExpiresUnixNano", "RequestedNotAfter"} {
		if _, ok := typeOf.FieldByName(forbidden); ok {
			t.Fatalf("stable identity contains %s", forbidden)
		}
	}
	if ports.Supported(ports.FeatureSnapshotArtifactOwner) {
		t.Fatal("SnapshotArtifact owner-local candidate claims production support")
	}
	if !ports.Supported(ports.FeatureSnapshotArtifactCapture) {
		t.Fatal("implemented Snapshot Artifact capture slice was reported unsupported")
	}
}

func snapshotArtifactCurrentBlackBoxRequest(aggregateID string, expected contract.SnapshotArtifactAggregateRefV2, bound time.Time) *contract.InspectSnapshotArtifactAggregateCurrentRequestV2 {
	return &contract.InspectSnapshotArtifactAggregateCurrentRequestV2{
		ArtifactAggregateID:  aggregateID,
		ExpectedAggregateRef: contract.SnapshotArtifactOptionalAggregateRefV2{Presence: contract.SnapshotArtifactPresent, Ref: &expected},
		RequestedNotAfter:    bound.UnixNano(),
	}
}
