package kernel_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestCheckpointPublicControllerHasNoCallerFactWrite(t *testing.T) {
	var store *testkit.CheckpointMemoryStore
	if _, err := kernel.NewCheckpointController(store, func() time.Time { return testkit.FixedNow }); err == nil {
		t.Fatal("controller accepted typed-nil store")
	}
	valid := testkit.NewCheckpointMemoryStore()
	controller := mustCheckpointController(t, valid)
	typeOfController := reflect.TypeOf(controller)
	for _, forbidden := range []string{"RecordPhaseFact", "ReconcileUnknown"} {
		if _, exists := typeOfController.MethodByName(forbidden); exists {
			t.Fatalf("public caller fact write %s still exists", forbidden)
		}
	}
	if _, err := controller.ReservePhase(context.Background(), nil); err == nil {
		t.Fatal("controller accepted nil reservation")
	}
}

func TestCheckpointReserveCAS64DifferentContentsSingleWinner(t *testing.T) {
	ctx := context.Background()
	store := testkit.NewCheckpointMemoryStore()
	participant := testkit.CheckpointParticipant("cas64")
	if err := store.SeedCheckpointParticipant(participant); err != nil {
		t.Fatal(err)
	}
	controller := mustCheckpointController(t, store)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var winners atomic.Int64
	var unexpected atomic.Int64
	for index := 0; index < 64; index++ {
		reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "cas64-"+string(rune('a'+index)), participant, nil)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := controller.ReservePhase(ctx, &reservation); err == nil {
				winners.Add(1)
			} else if !errors.Is(err, ports.ErrConflict) {
				unexpected.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if winners.Load() != 1 || unexpected.Load() != 0 {
		t.Fatalf("winners=%d unexpected=%d", winners.Load(), unexpected.Load())
	}
}

func TestCheckpointReserveLostReplyRecoversExactReservationAndParticipant(t *testing.T) {
	base := testkit.NewCheckpointMemoryStore()
	participant := testkit.CheckpointParticipant("lost-reserve")
	if err := base.SeedCheckpointParticipant(participant); err != nil {
		t.Fatal(err)
	}
	store := testkit.NewCheckpointLostReplyStore(base, testkit.CheckpointLoseReserveReply)
	controller := mustCheckpointController(t, store)
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "lost-reserve", participant, nil)
	got, err := controller.ReservePhase(context.Background(), &reservation)
	if err != nil {
		t.Fatalf("lost reply exact recovery: %v", err)
	}
	if !contract.SameRef(got.Meta.Ref(), reservation.Meta.Ref()) {
		t.Fatal("lost reply returned another reservation")
	}
	exactReplay := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "lost-reserve", participant, nil)
	reservation.Watermarks[0].Sequence++
	stored, err := base.InspectCheckpointPhaseReservation(context.Background(), got.Meta.Ref())
	if err != nil || stored.Watermarks[0].Sequence != 1 {
		t.Fatalf("stored reservation alias sequence=%d err=%v", stored.Watermarks[0].Sequence, err)
	}
	if _, err := controller.ReservePhase(context.Background(), &reservation); err == nil {
		t.Fatal("mutated caller content replay was accepted")
	}
	if _, err := controller.ReservePhase(context.Background(), &exactReplay); err != nil {
		t.Fatalf("exact replay after lost reply: %v", err)
	}
}

func TestCheckpointCurrentReaderEnforcesEarlyAbsentEffectAndMinTTL(t *testing.T) {
	store, reservation, participant := reservedCheckpoint(t, "reader")
	coordinates, request := testkit.CheckpointCurrentFixture(reservation, participant, contract.CheckpointReadPreAdmission)
	coordinates[0].Meta.ExpiresUnixNano = testkit.FixedNow.Add(30 * time.Minute).UnixNano()
	if err := store.SeedCheckpointCurrent(coordinates...); err != nil {
		t.Fatal(err)
	}
	reader := mustCheckpointReader(t, store, store)
	projection, err := reader.ReadCheckpointParticipantCurrent(context.Background(), &request)
	if err != nil {
		t.Fatal(err)
	}
	if projection.ExpiresUnixNano != testkit.FixedNow.Add(30*time.Minute).UnixNano() {
		t.Fatalf("projection TTL=%d", projection.ExpiresUnixNano)
	}

	prePrepare, _ := testkit.CheckpointCurrentFixture(reservation, participant, contract.CheckpointReadPrePrepare)
	for _, coordinate := range prePrepare {
		if coordinate.Kind == contract.CheckpointCurrentAdmission {
			if err := store.SeedCheckpointCurrent(coordinate); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if _, err := reader.ReadCheckpointParticipantCurrent(context.Background(), &request); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("early admission gate = %v", err)
	}

	drift := &driftCheckpointSource{CheckpointCurrentSource: store}
	driftReader := mustCheckpointReader(t, store, drift)
	if _, err := driftReader.ReadCheckpointParticipantCurrent(context.Background(), &request); err == nil {
		t.Fatal("cross-effect coordinate was accepted")
	}
}

func TestCheckpointCurrentReaderRefreshesOwnerCurrentAndRejectsCoordinateDrift(t *testing.T) {
	store, reservation, participant := reservedCheckpoint(t, "current-refresh")
	coordinates, request := testkit.CheckpointCurrentFixture(reservation, participant, contract.CheckpointReadPreExecute)
	if err := store.SeedCheckpointCurrent(coordinates...); err != nil {
		t.Fatal(err)
	}
	reader := mustCheckpointReader(t, store, store)
	before, err := reader.ReadCheckpointParticipantCurrent(context.Background(), &request)
	if err != nil {
		t.Fatal(err)
	}

	original := coordinates[0]
	refreshed := original
	refreshed.Meta.ExpiresUnixNano = testkit.FixedNow.Add(time.Minute).UnixNano()
	store.ReplaceCheckpointCurrent(refreshed)
	after, err := reader.ReadCheckpointParticipantCurrent(context.Background(), &request)
	if err != nil {
		t.Fatalf("exact refreshed current was rejected: %v", err)
	}
	if after.ExpiresUnixNano != refreshed.Meta.ExpiresUnixNano || after.ProjectionDigest == before.ProjectionDigest {
		t.Fatalf("reader reused cached projection: before=%d/%s after=%d/%s", before.ExpiresUnixNano, before.ProjectionDigest, after.ExpiresUnixNano, after.ProjectionDigest)
	}

	tests := []struct {
		name   string
		mutate func(*contract.CheckpointCurrentCoordinate)
	}{
		{name: "lease", mutate: func(value *contract.CheckpointCurrentCoordinate) { value.Runtime.LeaseEpoch++ }},
		{name: "fence", mutate: func(value *contract.CheckpointCurrentCoordinate) { value.Runtime.FenceEpoch++ }},
		{name: "change-set", mutate: func(value *contract.CheckpointCurrentCoordinate) {
			ref := *value.ChangeSet.Ref
			ref.ID = "drifted-change-set"
			value.ChangeSet.Ref = &ref
		}},
		{name: "watermark", mutate: func(value *contract.CheckpointCurrentCoordinate) {
			value.Watermarks = append([]contract.CheckpointWatermark(nil), value.Watermarks...)
			value.Watermarks[0].Sequence++
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store.ReplaceCheckpointCurrent(original)
			drifted := original
			test.mutate(&drifted)
			store.ReplaceCheckpointCurrent(drifted)
			if _, readErr := reader.ReadCheckpointParticipantCurrent(context.Background(), &request); !errors.Is(readErr, ports.ErrStale) {
				t.Fatalf("drift was not fail closed: %v", readErr)
			}
		})
	}
}

func TestCheckpointDifferentPreparedClosureCannotDoubleWin(t *testing.T) {
	ctx := context.Background()
	store, prepare, participant := reservedCheckpoint(t, "double-closure")
	coordinates, request := testkit.CheckpointCurrentFixture(prepare, participant, contract.CheckpointReadPreExecute)
	if err := store.SeedCheckpointCurrent(coordinates...); err != nil {
		t.Fatal(err)
	}
	projection, err := mustCheckpointReader(t, store, store).ReadCheckpointParticipantCurrent(ctx, &request)
	if err != nil {
		t.Fatal(err)
	}
	firstFact, firstParticipant := testkit.CheckpointAppliedPhase(prepare, participant, contract.CheckpointPhasePrepared, "double-closure", time.Unix(0, projection.ExpiresUnixNano))
	if err := store.AppendAppliedCheckpointPhase(nil, participant.Meta.Ref(), firstFact, firstParticipant, projection.ExpiresUnixNano); err != nil {
		t.Fatal(err)
	}

	secondFact := firstFact
	secondFact.Meta.Revision++
	secondFact.Meta.UpdatedUnixNano = testkit.FixedNow.UnixNano()
	secondFact.EvidenceRefs = []contract.Ref{testkit.Ref("second-closure-evidence")}
	secondFact.DomainResultRef = testkit.Ref("second-closure-domain-result")
	secondFact.RuntimeSettlementRef = testkit.Ref("second-closure-settlement-v5")
	secondFact.ApplySettlementRef = testkit.Ref("second-closure-apply")
	secondFact, err = contract.SealCheckpointPhaseFact(secondFact)
	if err != nil {
		t.Fatal(err)
	}
	secondParticipant := firstParticipant
	secondParticipant.Meta.Revision++
	secondParticipant.Meta.UpdatedUnixNano = testkit.FixedNow.UnixNano()
	closureB := secondFact.ClosureRef()
	secondParticipant.Closure = &closureB
	secondParticipant, err = contract.SealCheckpointParticipantFact(secondParticipant)
	if err != nil {
		t.Fatal(err)
	}
	firstRef := firstFact.Meta.Ref()
	if err := store.AppendAppliedCheckpointPhase(&firstRef, firstParticipant.Meta.Ref(), secondFact, secondParticipant, projection.ExpiresUnixNano); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("second prepared closure append = %v", err)
	}

	closureA := firstFact.ClosureRef()
	attemptBypass := testkit.CheckpointReservation(contract.CheckpointPhaseAbort, "double-closure-attempt-bypass", firstParticipant, &closureA)
	attemptBypass.AttemptID = "caller-minted-attempt"
	attemptBypass.Meta.Digest = ""
	attemptBypass.Meta.Digest, err = contract.Digest("checkpoint-phase-reservation-v2", attemptBypass)
	if err != nil {
		t.Fatal(err)
	}
	if _, reserveErr := mustCheckpointController(t, store).ReservePhase(ctx, &attemptBypass); reserveErr == nil {
		t.Fatal("same global checkpoint attempt accepted a caller-minted attempt ID")
	}
	currentParticipant, inspectErr := store.InspectCheckpointParticipantCurrent(ctx, firstParticipant.Meta.ID)
	if inspectErr != nil || !contract.SameRef(currentParticipant.Meta.Ref(), firstParticipant.Meta.Ref()) {
		t.Fatalf("rejected attempt bypass moved participant current pointer: current=%v err=%v", currentParticipant.Meta.Ref(), inspectErr)
	}

	commit := testkit.CheckpointReservation(contract.CheckpointPhaseCommit, "double-closure-commit", firstParticipant, &closureA)
	controller := mustCheckpointController(t, store)
	if _, reserveErr := controller.ReservePhase(ctx, &commit); reserveErr != nil {
		t.Fatalf("commit with current prepared participant: %v", reserveErr)
	}
	latestParticipant, inspectErr := store.InspectCheckpointParticipantCurrent(ctx, firstParticipant.Meta.ID)
	if inspectErr != nil || contract.SameRef(latestParticipant.Meta.Ref(), firstParticipant.Meta.Ref()) {
		t.Fatalf("commit did not advance participant current: current=%v err=%v", latestParticipant.Meta.Ref(), inspectErr)
	}

	abortAfterCommit := testkit.CheckpointReservation(contract.CheckpointPhaseAbort, "double-closure-abort-latest", latestParticipant, &closureA)
	if _, reserveErr := controller.ReservePhase(ctx, &abortAfterCommit); !errors.Is(reserveErr, ports.ErrConflict) {
		t.Fatalf("same prepared closure reopened sibling after commit with latest participant: %v", reserveErr)
	}
	afterRejectedAbort, inspectErr := store.InspectCheckpointParticipantCurrent(ctx, latestParticipant.Meta.ID)
	if inspectErr != nil || !contract.SameRef(afterRejectedAbort.Meta.Ref(), latestParticipant.Meta.Ref()) {
		t.Fatalf("branch conflict moved participant current: current=%v err=%v", afterRejectedAbort.Meta.Ref(), inspectErr)
	}

	const siblingCount = 32
	var conflicts, unexpected atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for index := 0; index < siblingCount; index++ {
		candidate := testkit.CheckpointReservation(contract.CheckpointPhaseAbort, "branch-sibling-"+string(rune('a'+index)), latestParticipant, &closureA)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, reserveErr := controller.ReservePhase(ctx, &candidate)
			if errors.Is(reserveErr, ports.ErrConflict) {
				conflicts.Add(1)
			} else {
				unexpected.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if conflicts.Load() != siblingCount || unexpected.Load() != 0 {
		t.Fatalf("latest-revision sibling branch conflicts=%d unexpected=%d", conflicts.Load(), unexpected.Load())
	}
	afterConcurrentSiblings, inspectErr := store.InspectCheckpointParticipantCurrent(ctx, latestParticipant.Meta.ID)
	if inspectErr != nil || !contract.SameRef(afterConcurrentSiblings.Meta.Ref(), latestParticipant.Meta.Ref()) {
		t.Fatalf("concurrent sibling guard moved participant current: current=%v err=%v", afterConcurrentSiblings.Meta.Ref(), inspectErr)
	}
}

func TestCheckpointHistoryExactInspectCloneAndNoABA(t *testing.T) {
	store, reservation, participant := reservedCheckpoint(t, "history")
	coordinates, request := testkit.CheckpointCurrentFixture(reservation, participant, contract.CheckpointReadPreExecute)
	if err := store.SeedCheckpointCurrent(coordinates...); err != nil {
		t.Fatal(err)
	}
	reader := mustCheckpointReader(t, store, store)
	if _, err := reader.ReadCheckpointParticipantCurrent(context.Background(), &request); err != nil {
		t.Fatal(err)
	}
	readerExpiry := testkit.FixedNow.Add(time.Hour)
	unknown, afterUnknown := testkit.CheckpointAppliedPhase(reservation, participant, contract.CheckpointPhaseUnknown, "history", readerExpiry)
	if err := store.AppendAppliedCheckpointPhase(nil, participant.Meta.Ref(), unknown, afterUnknown, readerExpiry.UnixNano()); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadCheckpointParticipantCurrent(context.Background(), &request); !errors.Is(err, ports.ErrStale) {
		t.Fatalf("historical participant revision remained current: %v", err)
	}
	next, afterNext := testkit.ReconciledCheckpointAppliedPhase(unknown, afterUnknown, "history", readerExpiry)
	unknownRef := unknown.Meta.Ref()
	if err := store.AppendAppliedCheckpointPhase(&unknownRef, afterUnknown.Meta.Ref(), next, afterNext, readerExpiry.UnixNano()); err != nil {
		t.Fatal(err)
	}
	historical, err := store.InspectCheckpointPhaseFact(context.Background(), unknownRef)
	if err != nil || historical.State != contract.CheckpointPhaseUnknown {
		t.Fatalf("historical state=%q err=%v", historical.State, err)
	}
	historical.EvidenceRefs[0].ID = "mutated"
	again, _ := store.InspectCheckpointPhaseFact(context.Background(), unknownRef)
	if again.EvidenceRefs[0].ID == "mutated" {
		t.Fatal("historical inspect returned a store alias")
	}
	wrongRef := unknownRef
	wrongRef.Digest = testkit.Ref("wrong-history-digest").Digest
	if _, err := store.InspectCheckpointPhaseFact(context.Background(), wrongRef); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("historical inspect ignored full ref: %v", err)
	}
	current, err := store.InspectCheckpointPhaseFactCurrent(context.Background(), unknown.Meta.ID)
	if err != nil || !contract.SameRef(current.Meta.Ref(), next.Meta.Ref()) {
		t.Fatalf("current ref=%v err=%v", current.Meta.Ref(), err)
	}
	if err := store.AppendAppliedCheckpointPhase(&unknownRef, afterUnknown.Meta.Ref(), next, afterNext, readerExpiry.UnixNano()); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("old expected ref reopened history: %v", err)
	}
}

func TestCheckpointAppliedFactTTLDoesNotExceedReader(t *testing.T) {
	store, reservation, participant := reservedCheckpoint(t, "fact-ttl")
	readerExpiry := testkit.FixedNow.Add(time.Hour)
	fact, next := testkit.CheckpointAppliedPhase(reservation, participant, contract.CheckpointPhasePrepared, "fact-ttl", readerExpiry)
	fact.Meta.ExpiresUnixNano = readerExpiry.Add(time.Nanosecond).UnixNano()
	fact, _ = contract.SealCheckpointPhaseFact(fact)
	next.Meta.ExpiresUnixNano = fact.Meta.ExpiresUnixNano
	closure := fact.ClosureRef()
	next.Closure = &closure
	next, _ = contract.SealCheckpointParticipantFact(next)
	if err := store.AppendAppliedCheckpointPhase(nil, participant.Meta.Ref(), fact, next, readerExpiry.UnixNano()); !errors.Is(err, ports.ErrStale) {
		t.Fatalf("fact extended Reader TTL: %v", err)
	}
}

type driftCheckpointSource struct {
	ports.CheckpointCurrentSource
}

func (s *driftCheckpointSource) InspectCheckpointCurrent(ctx context.Context, query contract.CheckpointCurrentQuery) (contract.CheckpointCurrentCoordinate, error) {
	coordinate, err := s.CheckpointCurrentSource.InspectCheckpointCurrent(ctx, query)
	if err == nil {
		coordinate.EffectID = "cross-effect"
	}
	return coordinate, err
}

func reservedCheckpoint(t *testing.T, suffix string) (*testkit.CheckpointMemoryStore, contract.CheckpointPhaseReservation, contract.CheckpointParticipantFact) {
	t.Helper()
	store := testkit.NewCheckpointMemoryStore()
	initial := testkit.CheckpointParticipant(suffix)
	if err := store.SeedCheckpointParticipant(initial); err != nil {
		t.Fatal(err)
	}
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, suffix, initial, nil)
	if _, err := mustCheckpointController(t, store).ReservePhase(context.Background(), &reservation); err != nil {
		t.Fatal(err)
	}
	participant, err := store.InspectCheckpointParticipantCurrent(context.Background(), initial.Meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	return store, reservation, participant
}

func mustCheckpointController(t *testing.T, store ports.CheckpointPhaseStore) *kernel.CheckpointController {
	t.Helper()
	controller, err := kernel.NewCheckpointController(store, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	return controller
}

func mustCheckpointReader(t *testing.T, store ports.CheckpointPhaseStore, source ports.CheckpointCurrentSource) *kernel.CheckpointCurrentReader {
	t.Helper()
	reader, err := kernel.NewCheckpointCurrentReader(store, source, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	return reader
}
