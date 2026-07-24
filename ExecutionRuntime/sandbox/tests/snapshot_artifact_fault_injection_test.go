package sandbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestSnapshotArtifactFaultLostReplyDoesNotCreateSecondReservation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	base := testkit.NewSnapshotArtifactMemoryStore()
	owner, err := kernel.NewSnapshotArtifactOwner(testkit.NewSnapshotArtifactLostReplyStore(base), func() time.Time { return testkit.FixedNow }, snapshotArtifactBlackBoxLimits())
	if err != nil {
		t.Fatal(err)
	}
	request := testkit.SnapshotArtifactRequest("fault-lost-reply")
	first, err := owner.ReserveArtifact(ctx, &request)
	if err != nil || !first.Created {
		t.Fatalf("lost reply = %#v, %v", first, err)
	}
	second, err := owner.ReserveArtifact(ctx, &request)
	if err != nil || second.Created || second.Reservation.Meta.ID != first.Reservation.Meta.ID {
		t.Fatalf("replay after lost reply = %#v, %v", second, err)
	}
	drift := request
	drift.RequestedNotAfter++
	if _, err := owner.ReserveArtifact(ctx, &drift); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("lost-reply stable key accepted TTL drift: %v", err)
	}
}

func TestSnapshotArtifactFaultExpiredReservationCannotProduceCurrent(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewSnapshotArtifactMemoryStore()
	now := testkit.FixedNow
	owner, err := kernel.NewSnapshotArtifactOwner(store, func() time.Time { return now }, snapshotArtifactBlackBoxLimits())
	if err != nil {
		t.Fatal(err)
	}
	request := testkit.SnapshotArtifactRequest("fault-expiry")
	request.RequestedNotAfter = now.Add(time.Second).UnixNano()
	result, err := owner.ReserveArtifact(ctx, &request)
	if err != nil {
		t.Fatal(err)
	}
	now = time.Unix(0, result.Reservation.Meta.ExpiresUnixNano)
	index, err := store.InspectSnapshotArtifactCurrentIndex(ctx, result.Reservation.SubjectRef.ArtifactAggregateID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = owner.InspectAggregateCurrent(ctx, snapshotArtifactCurrentBlackBoxRequest(result.Reservation.SubjectRef.ArtifactAggregateID, index.HeadAggregateEnvelopeRef, now.Add(time.Hour)))
	if err == nil {
		t.Fatal("expired reservation produced current projection")
	}
}
