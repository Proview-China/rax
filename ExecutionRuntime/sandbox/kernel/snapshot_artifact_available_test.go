package kernel

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestSnapshotArtifactOwnerCommitAvailableExactReplayAndHistory(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedNow
	store := testkit.NewSnapshotArtifactMemoryStore()
	reader := stableSnapshotArtifactCommitReaderV2(now)
	owner := snapshotArtifactAvailableOwnerV2(t, store, reader, func() time.Time { return now })
	request := testkit.SnapshotArtifactRequest("available")
	reserved, err := owner.ReserveArtifact(ctx, &request)
	if err != nil {
		t.Fatal(err)
	}
	reservedIndex, err := store.InspectSnapshotArtifactCurrentIndex(ctx, reserved.Reservation.SubjectRef.ArtifactAggregateID)
	if err != nil {
		t.Fatal(err)
	}
	commit := testkit.SnapshotArtifactCommitRequest(reserved.Reservation, reservedIndex, "available", now)
	first, err := owner.CommitArtifact(ctx, &commit)
	if err != nil || !first.Created || first.CurrentIndex.AggregateState != contract.SnapshotArtifactAggregateAvailable {
		t.Fatalf("commit=%#v err=%v", first, err)
	}
	second, err := owner.CommitArtifact(ctx, &commit)
	if err != nil || second.Created || !contract.SameSnapshotArtifactExactRef(first.Fact.ExactRef(), second.Fact.ExactRef()) {
		t.Fatalf("replay=%#v err=%v", second, err)
	}
	if _, err := owner.InspectAggregateHistorical(ctx, &contract.InspectSnapshotArtifactAggregateHistoricalRequestV2{ExpectedRef: reservedIndex.HeadAggregateEnvelopeRef}); err != nil {
		t.Fatalf("reserved history disappeared: %v", err)
	}
	if _, err := owner.InspectAggregateHistorical(ctx, &contract.InspectSnapshotArtifactAggregateHistoricalRequestV2{ExpectedRef: first.CurrentIndex.HeadAggregateEnvelopeRef}); err != nil {
		t.Fatalf("available history missing: %v", err)
	}
	inspected, err := owner.InspectArtifactFact(ctx, &contract.InspectSnapshotArtifactFactRequestV2{ExpectedRef: first.Fact.ExactRef()})
	if err != nil || !contract.SameSnapshotArtifactExactRef(inspected.ExactRef(), first.Fact.ExactRef()) {
		t.Fatalf("fact inspect=%#v err=%v", inspected, err)
	}
	inspected.ProviderReceiptRef.ID = "caller-mutated"
	again, err := owner.InspectArtifactFact(ctx, &contract.InspectSnapshotArtifactFactRequestV2{ExpectedRef: first.Fact.ExactRef()})
	if err != nil || again.ProviderReceiptRef.ID == "caller-mutated" {
		t.Fatalf("artifact fact store leaked caller alias: %#v err=%v", again, err)
	}
	projection, err := owner.InspectAggregateCurrent(ctx, &contract.InspectSnapshotArtifactAggregateCurrentRequestV2{ArtifactAggregateID: reserved.Reservation.SubjectRef.ArtifactAggregateID, ExpectedAggregateRef: contract.SnapshotArtifactOptionalAggregateRefV2{Presence: contract.SnapshotArtifactPresent, Ref: &first.CurrentIndex.HeadAggregateEnvelopeRef}, RequestedNotAfter: now.Add(time.Hour).UnixNano()})
	if err != nil || projection.AggregateState != contract.SnapshotArtifactAggregateAvailable || projection.ArtifactFactRef.Ref == nil || !contract.SameSnapshotArtifactExactRef(*projection.ArtifactFactRef.Ref, first.Fact.ExactRef()) {
		t.Fatalf("available current=%#v err=%v", projection, err)
	}
}

func TestSnapshotArtifactOwnerAvailableLostReplyInspectsOriginalWinner(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedNow
	base := testkit.NewSnapshotArtifactMemoryStore()
	store := testkit.NewSnapshotArtifactAvailableLostReplyStore(base)
	owner := snapshotArtifactAvailableOwnerV2(t, store, stableSnapshotArtifactCommitReaderV2(now), func() time.Time { return now })
	reserveRequest := testkit.SnapshotArtifactRequest("lost-available")
	reserved, err := owner.ReserveArtifact(ctx, &reserveRequest)
	if err != nil {
		t.Fatal(err)
	}
	index, _ := store.InspectSnapshotArtifactCurrentIndex(ctx, reserved.Reservation.SubjectRef.ArtifactAggregateID)
	request := testkit.SnapshotArtifactCommitRequest(reserved.Reservation, index, "lost-available", now)
	result, err := owner.CommitArtifact(ctx, &request)
	if err != nil || !result.Created || result.CurrentIndex.AggregateState != contract.SnapshotArtifactAggregateAvailable {
		t.Fatalf("lost reply recovery=%#v err=%v", result, err)
	}
	replay, err := owner.CommitArtifact(ctx, &request)
	if err != nil || replay.Created || !contract.SameSnapshotArtifactExactRef(replay.Fact.ExactRef(), result.Fact.ExactRef()) {
		t.Fatalf("lost reply replay=%#v err=%v", replay, err)
	}
}

func TestSnapshotArtifactOwnerCommitS2DriftIsZeroWrite(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedNow
	store := testkit.NewSnapshotArtifactMemoryStore()
	var base contract.SnapshotArtifactCommitCurrentProjectionV2
	reader := &testkit.SnapshotArtifactCommitCurrentReader{ReadFunc: func(call int, request contract.CommitSnapshotArtifactRequestV2) (contract.SnapshotArtifactCommitCurrentProjectionV2, error) {
		if call == 1 {
			base = testkit.SnapshotArtifactCommitProjection(request, "tenant-1", "workspace-checkpoint", now)
			return base, nil
		}
		drifted := base
		drifted.ProviderReceiptRef = testkit.Ref("different-receipt")
		return contract.SealSnapshotArtifactCommitCurrentProjectionV2(drifted, now)
	}}
	owner := snapshotArtifactAvailableOwnerV2(t, store, reader, func() time.Time { return now })
	reserveRequest := testkit.SnapshotArtifactRequest("s2-drift")
	reserved, err := owner.ReserveArtifact(ctx, &reserveRequest)
	if err != nil {
		t.Fatal(err)
	}
	index, _ := store.InspectSnapshotArtifactCurrentIndex(ctx, reserved.Reservation.SubjectRef.ArtifactAggregateID)
	commit := testkit.SnapshotArtifactCommitRequest(reserved.Reservation, index, "s2-drift", now)
	if _, err := owner.CommitArtifact(ctx, &commit); err == nil {
		t.Fatal("S2 drift committed an artifact")
	}
	current, err := store.InspectSnapshotArtifactCurrentIndex(ctx, index.ArtifactAggregateID)
	if err != nil || !contract.SameSnapshotArtifactExactRef(current.CurrentIndexRef, index.CurrentIndexRef) {
		t.Fatalf("S2 drift changed current: %#v err=%v", current, err)
	}
}

func TestSnapshotArtifactOwnerConcurrentDifferentArtifactSingleWinner(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedNow
	store := testkit.NewSnapshotArtifactMemoryStore()
	reader := &testkit.SnapshotArtifactCommitCurrentReader{ReadFunc: func(_ int, request contract.CommitSnapshotArtifactRequestV2) (contract.SnapshotArtifactCommitCurrentProjectionV2, error) {
		return testkit.SnapshotArtifactCommitProjection(request, "tenant-1", "workspace-checkpoint", now), nil
	}}
	owner := snapshotArtifactAvailableOwnerV2(t, store, reader, func() time.Time { return now })
	reserveRequest := testkit.SnapshotArtifactRequest("race")
	reserved, err := owner.ReserveArtifact(ctx, &reserveRequest)
	if err != nil {
		t.Fatal(err)
	}
	index, _ := store.InspectSnapshotArtifactCurrentIndex(ctx, reserved.Reservation.SubjectRef.ArtifactAggregateID)
	var wg sync.WaitGroup
	results := make(chan error, 64)
	for worker := 0; worker < 64; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			request := testkit.SnapshotArtifactCommitRequest(reserved.Reservation, index, fmt.Sprintf("race-%02d", worker), now)
			_, err := owner.CommitArtifact(ctx, &request)
			results <- err
		}(worker)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
			continue
		}
		if !errorsIsAnyV2(err, ports.ErrConflict, ports.ErrStale) {
			t.Fatalf("unexpected concurrent error: %v", err)
		}
	}
	if success != 1 {
		t.Fatalf("concurrent winners=%d want=1", success)
	}
}

func stableSnapshotArtifactCommitReaderV2(now time.Time) *testkit.SnapshotArtifactCommitCurrentReader {
	return &testkit.SnapshotArtifactCommitCurrentReader{ReadFunc: func(_ int, request contract.CommitSnapshotArtifactRequestV2) (contract.SnapshotArtifactCommitCurrentProjectionV2, error) {
		return testkit.SnapshotArtifactCommitProjection(request, "tenant-1", "workspace-checkpoint", now), nil
	}}
}

func snapshotArtifactAvailableOwnerV2(t *testing.T, store snapshotArtifactStore, reader ports.SnapshotArtifactCommitCurrentReaderV2, now func() time.Time) *SnapshotArtifactOwner {
	t.Helper()
	owner, err := NewSnapshotArtifactOwnerWithCommitCurrent(store, reader, now, SnapshotArtifactOwnerLimits{MaxReservationTTL: 90 * time.Minute, MaxHistoryTTL: 3 * time.Hour, MaxProjectionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func errorsIsAnyV2(err error, targets ...error) bool {
	for _, target := range targets {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
