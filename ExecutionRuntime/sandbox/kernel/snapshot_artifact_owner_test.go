package kernel

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestSnapshotArtifactOwnerReserveInspectAndExactReplay(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewSnapshotArtifactMemoryStore()
	owner := snapshotArtifactTestOwner(t, store, func() time.Time { return testkit.FixedNow })
	request := testkit.SnapshotArtifactRequest("reserve-inspect")

	created, err := owner.ReserveArtifact(ctx, &request)
	if err != nil || !created.Created {
		t.Fatalf("reserve = %#v, %v", created, err)
	}
	if created.Reservation.Meta.ExpiresUnixNano != testkit.FixedNow.Add(90*time.Minute).UnixNano() {
		t.Fatalf("reservation TTL was not capped by Owner: %d", created.Reservation.Meta.ExpiresUnixNano)
	}
	if created.CurrentIndex.ValidateCurrent(testkit.FixedNow) != nil || created.CurrentIndex.AggregateState != contract.SnapshotArtifactAggregateReserved {
		t.Fatalf("reserve did not return the exact current index: %#v", created.CurrentIndex)
	}
	replayed, err := owner.ReserveArtifact(ctx, &request)
	if err != nil || replayed.Created || !contract.SameSnapshotArtifactExactRef(replayed.Reservation.ExactRef(), created.Reservation.ExactRef()) || !contract.SameSnapshotArtifactExactRef(replayed.CurrentIndex.CurrentIndexRef, created.CurrentIndex.CurrentIndexRef) {
		t.Fatalf("exact replay = %#v, %v", replayed, err)
	}

	reservation, err := owner.InspectReservation(ctx, &contract.InspectSnapshotArtifactReservationRequestV2{ExpectedRef: created.Reservation.ExactRef()})
	if err != nil || !contract.SameSnapshotArtifactExactRef(reservation.ExactRef(), created.Reservation.ExactRef()) {
		t.Fatalf("inspect reservation = %#v, %v", reservation, err)
	}
	stable, err := owner.InspectReservationByStableKey(ctx, &contract.InspectSnapshotArtifactReservationByStableKeyRequestV2{StableKey: request.StableSourceKey()})
	if err != nil || !contract.SameSnapshotArtifactExactRef(stable.ExactRef(), created.Reservation.ExactRef()) {
		t.Fatalf("inspect stable = %#v, %v", stable, err)
	}

	index, err := store.InspectSnapshotArtifactCurrentIndex(ctx, created.Reservation.SubjectRef.ArtifactAggregateID)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := owner.InspectAggregateCurrent(ctx, &contract.InspectSnapshotArtifactAggregateCurrentRequestV2{
		ArtifactAggregateID:  created.Reservation.SubjectRef.ArtifactAggregateID,
		ExpectedAggregateRef: contract.SnapshotArtifactOptionalAggregateRefV2{Presence: contract.SnapshotArtifactPresent, Ref: &index.HeadAggregateEnvelopeRef},
		RequestedNotAfter:    testkit.FixedNow.Add(time.Hour).UnixNano(),
	})
	if err != nil || projection.AggregateState != contract.SnapshotArtifactAggregateReserved || projection.AggregateCurrentIndexRef.Revision != 1 {
		t.Fatalf("current projection = %#v, %v", projection, err)
	}
	if projection.ExpiresUnixNano != testkit.FixedNow.Add(time.Hour).UnixNano() {
		t.Fatalf("projection TTL was not capped by Owner: %d", projection.ExpiresUnixNano)
	}
	envelope, err := owner.InspectAggregateHistorical(ctx, &contract.InspectSnapshotArtifactAggregateHistoricalRequestV2{ExpectedRef: index.HeadAggregateEnvelopeRef})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.InspectEntryHistorical(ctx, &contract.InspectSnapshotArtifactEntryHistoricalRequestV2{ExpectedRef: envelope.AppliedEntryRef}); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotArtifactOwnerStableKeyConflictAndCloneIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewSnapshotArtifactMemoryStore()
	owner := snapshotArtifactTestOwner(t, store, func() time.Time { return testkit.FixedNow })
	request := testkit.SnapshotArtifactRequest("stable-conflict")
	result, err := owner.ReserveArtifact(ctx, &request)
	if err != nil {
		t.Fatal(err)
	}
	request.SourceOperationID = "mutated-operation"
	request.ExpectedContentDigest = testkit.Ref("mutated-content").Digest
	if _, err := owner.ReserveArtifact(ctx, &request); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("stable key content drift error = %v", err)
	}

	result.Reservation.SubjectRef.SchemaRef.ID = "caller-mutated"
	inspected, err := owner.InspectReservation(ctx, &contract.InspectSnapshotArtifactReservationRequestV2{ExpectedRef: result.Reservation.ExactRef()})
	if err != nil {
		t.Fatal(err)
	}
	if inspected.SubjectRef.SchemaRef.ID == "caller-mutated" {
		t.Fatal("caller mutation aliased stored reservation")
	}
	inspected.SubjectIdentity.ReservationID = "output-mutated"
	again, err := owner.InspectReservation(ctx, &contract.InspectSnapshotArtifactReservationRequestV2{ExpectedRef: result.Reservation.ExactRef()})
	if err != nil || again.SubjectIdentity.ReservationID == "output-mutated" {
		t.Fatalf("output mutation aliased store: %#v, %v", again, err)
	}
}

func TestSnapshotArtifactOwnerRejectsUnimplementedSuccessorStates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewSnapshotArtifactMemoryStore()
	owner := snapshotArtifactTestOwner(t, store, func() time.Time { return testkit.FixedNow })
	request := testkit.SnapshotArtifactRequest("unsupported-state")
	result, err := owner.ReserveArtifact(ctx, &request)
	if err != nil {
		t.Fatal(err)
	}
	index, _ := store.InspectSnapshotArtifactCurrentIndex(ctx, result.Reservation.SubjectRef.ArtifactAggregateID)
	envelope, _ := store.InspectSnapshotArtifactEnvelope(ctx, index.HeadAggregateEnvelopeRef)
	index.AggregateState = contract.SnapshotArtifactAggregateAvailable
	if _, err := contract.SealSnapshotArtifactAggregateCurrentIndexV2(index); err == nil {
		t.Fatal("unimplemented available current state passed sealing")
	}
	envelope.AggregateState = contract.SnapshotArtifactAggregateAvailable
	if _, err := contract.SealSnapshotArtifactAggregateEnvelopeV2(envelope); err == nil {
		t.Fatal("unimplemented available aggregate state passed sealing")
	}
}

func TestSnapshotArtifactOwnerConcurrentDifferentContentSingleWinner(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewSnapshotArtifactMemoryStore()
	owner := snapshotArtifactTestOwner(t, store, func() time.Time { return testkit.FixedNow })
	base := testkit.SnapshotArtifactRequest("concurrent-content")
	const workers = 64
	var created atomic.Int64
	var conflicts atomic.Int64
	var unexpected atomic.Value
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			request := base
			request.SourceOperationID = fmt.Sprintf("operation-%02d", index)
			request.ExpectedContentDigest = testkit.Ref(fmt.Sprintf("content-%02d", index)).Digest
			result, err := owner.ReserveArtifact(ctx, &request)
			switch {
			case err == nil && result.Created:
				created.Add(1)
			case errors.Is(err, ports.ErrConflict):
				conflicts.Add(1)
			default:
				unexpected.Store(fmt.Sprintf("worker %d result=%#v err=%v", index, result, err))
			}
		}(i)
	}
	wg.Wait()
	if value := unexpected.Load(); value != nil {
		t.Fatal(value)
	}
	if created.Load() != 1 || conflicts.Load() != workers-1 {
		t.Fatalf("created=%d conflicts=%d", created.Load(), conflicts.Load())
	}
}

func TestSnapshotArtifactOwnerConcurrentExactReplayOneCreator(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewSnapshotArtifactMemoryStore()
	owner := snapshotArtifactTestOwner(t, store, func() time.Time { return testkit.FixedNow })
	request := testkit.SnapshotArtifactRequest("concurrent-replay")
	const workers = 32
	var created atomic.Int64
	var replayed atomic.Int64
	var failed atomic.Value
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := owner.ReserveArtifact(ctx, &request)
			if err != nil {
				failed.Store(err.Error())
				return
			}
			if result.Created {
				created.Add(1)
			} else {
				replayed.Add(1)
			}
		}()
	}
	wg.Wait()
	if value := failed.Load(); value != nil {
		t.Fatal(value)
	}
	if created.Load() != 1 || replayed.Load() != workers-1 {
		t.Fatalf("created=%d replayed=%d", created.Load(), replayed.Load())
	}
}

func TestSnapshotArtifactOwnerLostReplyInspectsOriginalWinner(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	base := testkit.NewSnapshotArtifactMemoryStore()
	store := testkit.NewSnapshotArtifactLostReplyStore(base)
	owner := snapshotArtifactTestOwner(t, store, func() time.Time { return testkit.FixedNow })
	request := testkit.SnapshotArtifactRequest("lost-reply")
	result, err := owner.ReserveArtifact(ctx, &request)
	if err != nil || !result.Created {
		t.Fatalf("lost reply recovery = %#v, %v", result, err)
	}
	stored, err := base.InspectSnapshotArtifactReservationByStableKey(ctx, request.StableSourceKey())
	if err != nil || !contract.SameSnapshotArtifactExactRef(stored.ExactRef(), result.Reservation.ExactRef()) {
		t.Fatalf("stored lost reply winner = %#v, %v", stored, err)
	}
}

func TestSnapshotArtifactOwnerHistoricalInspectAfterCurrentExpiry(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewSnapshotArtifactMemoryStore()
	current := testkit.FixedNow
	owner := snapshotArtifactTestOwner(t, store, func() time.Time { return current })
	request := testkit.SnapshotArtifactRequest("historical-expiry")
	request.RequestedNotAfter = current.Add(time.Minute).UnixNano()
	result, err := owner.ReserveArtifact(ctx, &request)
	if err != nil {
		t.Fatal(err)
	}
	index, err := store.InspectSnapshotArtifactCurrentIndex(ctx, result.Reservation.SubjectRef.ArtifactAggregateID)
	if err != nil {
		t.Fatal(err)
	}
	current = time.Unix(0, index.CurrentIndexRef.ExpiresUnixNano)
	_, err = owner.InspectAggregateCurrent(ctx, &contract.InspectSnapshotArtifactAggregateCurrentRequestV2{
		ArtifactAggregateID:  result.Reservation.SubjectRef.ArtifactAggregateID,
		ExpectedAggregateRef: contract.SnapshotArtifactOptionalAggregateRefV2{Presence: contract.SnapshotArtifactPresent, Ref: &index.HeadAggregateEnvelopeRef},
		RequestedNotAfter:    current.Add(time.Hour).UnixNano(),
	})
	if err == nil {
		t.Fatal("expired current index authorized a projection")
	}
	if _, err := owner.InspectReservation(ctx, &contract.InspectSnapshotArtifactReservationRequestV2{ExpectedRef: result.Reservation.ExactRef()}); err != nil {
		t.Fatalf("historical reservation was not inspectable: %v", err)
	}
	envelope, err := owner.InspectAggregateHistorical(ctx, &contract.InspectSnapshotArtifactAggregateHistoricalRequestV2{ExpectedRef: index.HeadAggregateEnvelopeRef})
	if err != nil {
		t.Fatalf("historical envelope was not inspectable: %v", err)
	}
	if _, err := owner.InspectEntryHistorical(ctx, &contract.InspectSnapshotArtifactEntryHistoricalRequestV2{ExpectedRef: envelope.AppliedEntryRef}); err != nil {
		t.Fatalf("historical entry was not inspectable: %v", err)
	}
}

func TestSnapshotArtifactOwnerRejectsClockRollbackWithZeroWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewSnapshotArtifactMemoryStore()
	var reads atomic.Int64
	owner := snapshotArtifactTestOwner(t, store, func() time.Time {
		if reads.Add(1) == 1 {
			return testkit.FixedNow
		}
		return testkit.FixedNow.Add(-time.Nanosecond)
	})
	request := testkit.SnapshotArtifactRequest("clock-rollback")
	if _, err := owner.ReserveArtifact(ctx, &request); !errors.Is(err, ports.ErrStale) {
		t.Fatalf("clock rollback error = %v", err)
	}
	if _, err := store.InspectSnapshotArtifactReservationByStableKey(ctx, request.StableSourceKey()); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("clock rollback wrote a reservation: %v", err)
	}
}

func TestSnapshotArtifactOwnerRejectsClockBelowCommittedWatermark(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewSnapshotArtifactMemoryStore()
	current := testkit.FixedNow
	owner := snapshotArtifactTestOwner(t, store, func() time.Time { return current })
	first := testkit.SnapshotArtifactRequest("watermark-first")
	if _, err := owner.ReserveArtifact(ctx, &first); err != nil {
		t.Fatal(err)
	}
	current = testkit.FixedNow.Add(-time.Second)
	second := testkit.SnapshotArtifactRequest("watermark-second")
	if _, err := owner.ReserveArtifact(ctx, &second); !errors.Is(err, ports.ErrStale) {
		t.Fatalf("clock below committed watermark error = %v", err)
	}
	if _, err := store.InspectSnapshotArtifactReservationByStableKey(ctx, second.StableSourceKey()); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("clock below committed watermark wrote second reservation: %v", err)
	}
}

func TestSnapshotArtifactOwnerRefreshesAndRechecksCurrentPointer(t *testing.T) {
	ctx := context.Background()
	base := testkit.NewSnapshotArtifactMemoryStore()
	owner := snapshotArtifactTestOwner(t, base, func() time.Time { return testkit.FixedNow })
	request := testkit.SnapshotArtifactRequest("current-refresh")
	result, err := owner.ReserveArtifact(ctx, &request)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := base.InspectSnapshotArtifactCurrentIndex(ctx, result.Reservation.SubjectRef.ArtifactAggregateID)
	next := nextSnapshotArtifactCurrentIndex(t, first)
	if err := base.ReplaceSnapshotArtifactCurrent(next); err != nil {
		t.Fatal(err)
	}
	projection, err := owner.InspectAggregateCurrent(ctx, snapshotArtifactCurrentRequest(result, next))
	if err != nil || projection.AggregateCurrentIndexRef.Revision != next.CurrentIndexRef.Revision {
		t.Fatalf("reader did not refresh current pointer: %#v, %v", projection, err)
	}

	base2 := testkit.NewSnapshotArtifactMemoryStore()
	seedOwner := snapshotArtifactTestOwner(t, base2, func() time.Time { return testkit.FixedNow })
	result2, err := seedOwner.ReserveArtifact(ctx, &request)
	if err != nil {
		t.Fatal(err)
	}
	first2, _ := base2.InspectSnapshotArtifactCurrentIndex(ctx, result2.Reservation.SubjectRef.ArtifactAggregateID)
	switching := &snapshotArtifactSwitchingStore{SnapshotArtifactMemoryStore: base2, next: nextSnapshotArtifactCurrentIndex(t, first2)}
	reader := snapshotArtifactTestOwner(t, switching, func() time.Time { return testkit.FixedNow })
	if _, err := reader.InspectAggregateCurrent(ctx, snapshotArtifactCurrentRequest(result2, first2)); !errors.Is(err, ports.ErrStale) {
		t.Fatalf("mid-read current pointer drift error = %v", err)
	}
}

func TestSnapshotArtifactOwnerCurrentRejectsFutureOwnerClock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewSnapshotArtifactMemoryStore()
	owner := snapshotArtifactTestOwner(t, store, func() time.Time { return testkit.FixedNow })
	request := testkit.SnapshotArtifactRequest("future-owner-clock")
	result, err := owner.ReserveArtifact(ctx, &request)
	if err != nil {
		t.Fatal(err)
	}
	index, err := store.InspectSnapshotArtifactCurrentIndex(ctx, result.Reservation.SubjectRef.ArtifactAggregateID)
	if err != nil {
		t.Fatal(err)
	}
	index.CurrentIndexRef.Revision++
	index.OwnerClockWatermark = testkit.FixedNow.Add(time.Nanosecond).UnixNano()
	index.CheckedUnixNano = index.OwnerClockWatermark
	index, err = contract.SealSnapshotArtifactAggregateCurrentIndexV2(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceSnapshotArtifactCurrent(index); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.InspectAggregateCurrent(ctx, snapshotArtifactCurrentRequest(result, index)); !errors.Is(err, ports.ErrStale) {
		t.Fatalf("future Owner clock error = %v", err)
	}
}

func TestSnapshotArtifactCurrentIndexTTLIsCanonicalAndBoundaryIsExpired(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewSnapshotArtifactMemoryStore()
	owner := snapshotArtifactTestOwner(t, store, func() time.Time { return testkit.FixedNow })
	request := testkit.SnapshotArtifactRequest("current-ttl-canonical")
	result, err := owner.ReserveArtifact(ctx, &request)
	if err != nil {
		t.Fatal(err)
	}
	index, err := store.InspectSnapshotArtifactCurrentIndex(ctx, result.Reservation.SubjectRef.ArtifactAggregateID)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.ValidateCurrent(time.Unix(0, index.CurrentIndexRef.ExpiresUnixNano)); err == nil {
		t.Fatal("now == current index expiry authorized current")
	}

	tampered := index
	tampered.CurrentIndexRef.ExpiresUnixNano--
	if err := tampered.ValidateShape(); err == nil {
		t.Fatal("current index TTL tamper reused old digest")
	}
	resealed, err := contract.SealSnapshotArtifactAggregateCurrentIndexV2(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if resealed.CurrentIndexRef.Digest == index.CurrentIndexRef.Digest {
		t.Fatal("different current index TTL produced the same digest")
	}
}

func TestSnapshotArtifactCurrentIndexHistoryRejectsABA(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewSnapshotArtifactMemoryStore()
	owner := snapshotArtifactTestOwner(t, store, func() time.Time { return testkit.FixedNow })
	request := testkit.SnapshotArtifactRequest("current-no-aba")
	result, err := owner.ReserveArtifact(ctx, &request)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.InspectSnapshotArtifactCurrentIndex(ctx, result.Reservation.SubjectRef.ArtifactAggregateID)
	if err != nil {
		t.Fatal(err)
	}
	second := nextSnapshotArtifactCurrentIndex(t, first)
	if err := store.ReplaceSnapshotArtifactCurrent(second); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceSnapshotArtifactCurrent(first); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("revision rollback error = %v", err)
	}

	sameRevisionDrift := second
	sameRevisionDrift.RequestedNotAfter--
	sameRevisionDrift, err = contract.SealSnapshotArtifactAggregateCurrentIndexV2(sameRevisionDrift)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceSnapshotArtifactCurrent(sameRevisionDrift); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("same revision content drift error = %v", err)
	}
	current, err := store.InspectSnapshotArtifactCurrentIndex(ctx, result.Reservation.SubjectRef.ArtifactAggregateID)
	if err != nil || !contract.SameSnapshotArtifactExactRef(current.CurrentIndexRef, second.CurrentIndexRef) {
		t.Fatalf("current pointer moved after ABA attempts: %#v, %v", current.CurrentIndexRef, err)
	}
}

type snapshotArtifactSwitchingStore struct {
	*testkit.SnapshotArtifactMemoryStore
	next  contract.SnapshotArtifactAggregateCurrentIndexV2
	reads atomic.Int64
}

func (s *snapshotArtifactSwitchingStore) InspectSnapshotArtifactCurrentIndex(ctx context.Context, aggregateID string) (contract.SnapshotArtifactAggregateCurrentIndexV2, error) {
	if s.reads.Add(1) == 2 {
		if err := s.ReplaceSnapshotArtifactCurrent(s.next); err != nil {
			return contract.SnapshotArtifactAggregateCurrentIndexV2{}, err
		}
	}
	return s.SnapshotArtifactMemoryStore.InspectSnapshotArtifactCurrentIndex(ctx, aggregateID)
}

func nextSnapshotArtifactCurrentIndex(t *testing.T, current contract.SnapshotArtifactAggregateCurrentIndexV2) contract.SnapshotArtifactAggregateCurrentIndexV2 {
	t.Helper()
	next := current
	next.CurrentIndexRef.Revision++
	sealed, err := contract.SealSnapshotArtifactAggregateCurrentIndexV2(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func snapshotArtifactCurrentRequest(result contract.ReserveArtifactResultV2, index contract.SnapshotArtifactAggregateCurrentIndexV2) *contract.InspectSnapshotArtifactAggregateCurrentRequestV2 {
	return &contract.InspectSnapshotArtifactAggregateCurrentRequestV2{
		ArtifactAggregateID:  result.Reservation.SubjectRef.ArtifactAggregateID,
		ExpectedAggregateRef: contract.SnapshotArtifactOptionalAggregateRefV2{Presence: contract.SnapshotArtifactPresent, Ref: &index.HeadAggregateEnvelopeRef},
		RequestedNotAfter:    testkit.FixedNow.Add(time.Hour).UnixNano(),
	}
}

func snapshotArtifactTestOwner(t *testing.T, store snapshotArtifactStore, now func() time.Time) *SnapshotArtifactOwner {
	t.Helper()
	owner, err := NewSnapshotArtifactOwner(store, now, SnapshotArtifactOwnerLimits{
		MaxReservationTTL: 90 * time.Minute,
		MaxHistoryTTL:     3 * time.Hour,
		MaxProjectionTTL:  time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return owner
}
