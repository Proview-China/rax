package sandbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestSnapshotArtifactBlackBoxReserveInspectOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewSnapshotArtifactMemoryStore()
	owner, err := kernel.NewSnapshotArtifactOwner(store, func() time.Time { return testkit.FixedNow }, snapshotArtifactBlackBoxLimits())
	if err != nil {
		t.Fatal(err)
	}
	var public ports.SnapshotArtifactOwnerPortV2 = owner
	request := testkit.SnapshotArtifactRequest("blackbox")
	result, err := public.ReserveArtifact(ctx, &request)
	if err != nil || !result.Created {
		t.Fatalf("reserve = %#v, %v", result, err)
	}
	reservation, err := public.InspectReservation(ctx, &contract.InspectSnapshotArtifactReservationRequestV2{ExpectedRef: result.Reservation.ExactRef()})
	if err != nil || reservation.Meta.ID != result.Reservation.Meta.ID {
		t.Fatalf("inspect reservation = %#v, %v", reservation, err)
	}
	index, err := store.InspectSnapshotArtifactCurrentIndex(ctx, reservation.SubjectRef.ArtifactAggregateID)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := public.InspectAggregateCurrent(ctx, &contract.InspectSnapshotArtifactAggregateCurrentRequestV2{
		ArtifactAggregateID:  reservation.SubjectRef.ArtifactAggregateID,
		ExpectedAggregateRef: contract.SnapshotArtifactOptionalAggregateRefV2{Presence: contract.SnapshotArtifactPresent, Ref: &index.HeadAggregateEnvelopeRef},
		RequestedNotAfter:    testkit.FixedNow.Add(time.Hour).UnixNano(),
	})
	if err != nil || !projection.OwnerComputedCurrent || projection.AggregateState != contract.SnapshotArtifactAggregateReserved {
		t.Fatalf("inspect current = %#v, %v", projection, err)
	}
	if ports.Supported(ports.FeatureSnapshotArtifactOwner) {
		t.Fatal("owner-local candidate was advertised as production-supported")
	}
	if !ports.Supported(ports.FeatureSnapshotArtifactCapture) {
		t.Fatal("implemented reserved-to-available Snapshot Artifact capture was reported unsupported")
	}
}

func TestSnapshotArtifactBlackBoxTypedNilIsZeroWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewSnapshotArtifactMemoryStore()
	owner, err := kernel.NewSnapshotArtifactOwner(store, func() time.Time { return testkit.FixedNow }, snapshotArtifactBlackBoxLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.ReserveArtifact(ctx, nil); err == nil {
		t.Fatal("typed-nil reserve request passed")
	}
	if _, err := owner.InspectAggregateCurrent(ctx, nil); err == nil {
		t.Fatal("typed-nil current request passed")
	}
	request := testkit.SnapshotArtifactRequest("typed-nil")
	if _, err := store.InspectSnapshotArtifactReservationByStableKey(ctx, request.StableSourceKey()); err != ports.ErrNotFound {
		t.Fatalf("typed-nil path wrote Owner state: %v", err)
	}
}

func snapshotArtifactBlackBoxLimits() kernel.SnapshotArtifactOwnerLimits {
	return kernel.SnapshotArtifactOwnerLimits{
		MaxReservationTTL: 90 * time.Minute,
		MaxHistoryTTL:     3 * time.Hour,
		MaxProjectionTTL:  time.Hour,
	}
}
