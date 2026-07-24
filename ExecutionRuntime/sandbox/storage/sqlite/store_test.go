package sqlite_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	sandboxsqlite "github.com/Proview-China/rax/ExecutionRuntime/sandbox/storage/sqlite"
)

func TestFactStoreLifecyclePersistsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sandbox.db")
	store := openStore(t, ctx, path)
	if err := store.InitializeEnvironmentProjection(ctx, testkit.Projection()); err != nil {
		t.Fatal(err)
	}
	controller, err := kernel.NewController(store, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	reservation := testkit.Reservation(contract.EffectAllocate, 1, "sqlite")
	if err := controller.Reserve(ctx, reservation); err != nil {
		t.Fatal(err)
	}
	observation := testkit.Observation(reservation, 1, "sqlite")
	if accepted, err := controller.RecordObservation(ctx, observation); err != nil || !accepted {
		t.Fatalf("observation accepted=%v err=%v", accepted, err)
	}
	inspection := testkit.Inspection(reservation, observation, contract.DispositionConfirmedApplied, "sqlite")
	if err := controller.RecordInspection(ctx, inspection); err != nil {
		t.Fatal(err)
	}
	domain := testkit.Result(reservation, inspection, contract.DomainResultPayload{AllocationConfirmed: true}, "sqlite")
	if err := controller.CommitDomainResult(ctx, domain); err != nil {
		t.Fatal(err)
	}
	settlement := testkit.Settlement(domain, "sqlite")
	projection, err := controller.ApplySettlement(ctx, domain.Meta.ID, settlement)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openStore(t, ctx, path)
	defer reopened.Close()
	if got, err := reopened.GetReservation(ctx, reservation.Meta.ID); err != nil || !contract.SameRef(got.Meta.Ref(), reservation.Meta.Ref()) {
		t.Fatalf("reservation after restart=%#v err=%v", got, err)
	}
	if got, err := reopened.GetObservation(ctx, observation.Meta.ID); err != nil || !contract.SameRef(got.Meta.Ref(), observation.Meta.Ref()) {
		t.Fatalf("observation after restart=%#v err=%v", got, err)
	}
	if got, err := reopened.GetDomainResult(ctx, domain.Meta.ID); err != nil || !contract.SameRef(got.Meta.Ref(), domain.Meta.Ref()) {
		t.Fatalf("domain after restart=%#v err=%v", got, err)
	}
	if got, err := reopened.GetSettlementBinding(ctx, settlement.OpaqueRef); err != nil || !contract.SameRef(got, domain.Meta.Ref()) {
		t.Fatalf("settlement binding after restart=%#v err=%v", got, err)
	}
	if got, err := reopened.GetProjection(ctx, projection.Lease.LeaseID); err != nil || !contract.SameRef(got.Meta.Ref(), projection.Meta.Ref()) {
		t.Fatalf("projection after restart=%#v err=%v", got, err)
	}
}

func TestFactStoreSourceOrderingAndSettlementNoABA(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, ctx, filepath.Join(t.TempDir(), "sandbox.db"))
	defer store.Close()
	if err := store.InitializeEnvironmentProjection(ctx, testkit.Projection()); err != nil {
		t.Fatal(err)
	}
	reservation := testkit.Reservation(contract.EffectAllocate, 1, "source")
	if err := store.CreateReservation(ctx, reservation); err != nil {
		t.Fatal(err)
	}
	first := testkit.Observation(reservation, 1, "source")
	if accepted, err := store.AppendObservation(ctx, reservation.Meta.ID, first); err != nil || !accepted {
		t.Fatalf("first observation accepted=%v err=%v", accepted, err)
	}
	replay := first
	replay.Meta = testkit.Meta("observation-replayed-coordinate", 1)
	if accepted, err := store.AppendObservation(ctx, reservation.Meta.ID, replay); err != nil || accepted {
		t.Fatalf("source replay accepted=%v err=%v", accepted, err)
	}
	conflict := replay
	conflict.Meta = testkit.Meta("observation-conflicting-coordinate", 1)
	conflict.PayloadDigest = testkit.Ref("different-source-payload").Digest
	if _, err := store.AppendObservation(ctx, reservation.Meta.ID, conflict); !errors.Is(err, ports.ErrSourceConflict) {
		t.Fatalf("same source coordinate different payload err=%v", err)
	}
}

func TestSnapshotArtifactOwnerPersistsReservedAndAvailableHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sandbox.db")
	store := openStore(t, ctx, path)
	owner := snapshotOwner(t, store)
	request := testkit.SnapshotArtifactRequest("sqlite-persist")
	reserved, err := owner.ReserveArtifact(ctx, &request)
	if err != nil || !reserved.Created {
		t.Fatalf("reserve=%#v err=%v", reserved, err)
	}
	reservedIndex, err := store.InspectSnapshotArtifactCurrentIndex(ctx, reserved.Reservation.SubjectRef.ArtifactAggregateID)
	if err != nil {
		t.Fatal(err)
	}
	commit := testkit.SnapshotArtifactCommitRequest(reserved.Reservation, reservedIndex, "sqlite-persist", testkit.FixedNow)
	available, err := owner.CommitArtifact(ctx, &commit)
	if err != nil || !available.Created || available.CurrentIndex.AggregateState != contract.SnapshotArtifactAggregateAvailable {
		t.Fatalf("commit=%#v err=%v", available, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openStore(t, ctx, path)
	defer reopened.Close()
	owner = snapshotOwner(t, reopened)
	stable, err := owner.InspectReservationByStableKey(ctx, &contract.InspectSnapshotArtifactReservationByStableKeyRequestV2{StableKey: request.StableSourceKey()})
	if err != nil || !contract.SameSnapshotArtifactExactRef(stable.ExactRef(), reserved.Reservation.ExactRef()) {
		t.Fatalf("stable reservation after restart=%#v err=%v", stable, err)
	}
	if _, err := owner.InspectAggregateHistorical(ctx, &contract.InspectSnapshotArtifactAggregateHistoricalRequestV2{ExpectedRef: reservedIndex.HeadAggregateEnvelopeRef}); err != nil {
		t.Fatalf("reserved history after restart: %v", err)
	}
	if _, err := owner.InspectArtifactFact(ctx, &contract.InspectSnapshotArtifactFactRequestV2{ExpectedRef: available.Fact.ExactRef()}); err != nil {
		t.Fatalf("available fact after restart: %v", err)
	}
	current, err := reopened.InspectSnapshotArtifactCurrentIndex(ctx, reserved.Reservation.SubjectRef.ArtifactAggregateID)
	if err != nil || !contract.SameSnapshotArtifactExactRef(current.CurrentIndexRef, available.CurrentIndex.CurrentIndexRef) {
		t.Fatalf("current after restart=%#v err=%v", current, err)
	}
	if _, err := owner.CommitArtifact(ctx, &commit); err != nil {
		t.Fatalf("exact lost-reply replay did not inspect winner: %v", err)
	}
}

func TestSnapshotArtifactSQLiteConcurrentDifferentContentSingleWinner(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, ctx, filepath.Join(t.TempDir(), "sandbox.db"))
	defer store.Close()
	owner := snapshotOwner(t, store)
	base := testkit.SnapshotArtifactRequest("sqlite-race")
	const workers = 64
	var winners atomic.Int64
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
				winners.Add(1)
			case errors.Is(err, ports.ErrConflict):
				conflicts.Add(1)
			default:
				unexpected.Store(fmt.Sprintf("worker=%d result=%#v err=%v", index, result, err))
			}
		}(i)
	}
	wg.Wait()
	if value := unexpected.Load(); value != nil {
		t.Fatal(value)
	}
	if winners.Load() != 1 || conflicts.Load() != workers-1 {
		t.Fatalf("winners=%d conflicts=%d", winners.Load(), conflicts.Load())
	}
}

func openStore(t *testing.T, ctx context.Context, path string) *sandboxsqlite.Store {
	t.Helper()
	store, err := sandboxsqlite.OpenWithClock(ctx, path, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func snapshotOwner(t *testing.T, store ports.SnapshotArtifactStoreV2) *kernel.SnapshotArtifactOwner {
	t.Helper()
	reader := &testkit.SnapshotArtifactCommitCurrentReader{ReadFunc: func(_ int, request contract.CommitSnapshotArtifactRequestV2) (contract.SnapshotArtifactCommitCurrentProjectionV2, error) {
		return testkit.SnapshotArtifactCommitProjection(request, "tenant-1", "workspace-checkpoint", testkit.FixedNow), nil
	}}
	owner, err := kernel.NewSnapshotArtifactOwnerWithCommitCurrent(store, reader, func() time.Time { return testkit.FixedNow }, kernel.SnapshotArtifactOwnerLimits{MaxReservationTTL: 90 * time.Minute, MaxHistoryTTL: 3 * time.Hour, MaxProjectionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return owner
}
