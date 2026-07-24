package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/storage/sqlite"
)

func TestCheckpointSQLiteReservePersistsExactHistoryAndCurrent(t *testing.T) {
	store := openCheckpointStore(t)
	participant := testkit.CheckpointParticipant("sqlite-reserve")
	if err := store.CreateCheckpointParticipant(context.Background(), participant); err != nil {
		t.Fatal(err)
	}
	controller, err := kernel.NewCheckpointController(store, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "sqlite-reserve", participant, nil)
	created, err := controller.ReservePhase(context.Background(), &reservation)
	if err != nil {
		t.Fatal(err)
	}
	if !contract.SameRef(created.Meta.Ref(), reservation.Meta.Ref()) {
		t.Fatalf("reservation drifted: %+v", created.Meta.Ref())
	}
	historical, err := store.InspectCheckpointPhaseReservation(context.Background(), reservation.Meta.Ref())
	if err != nil || !contract.SameRef(historical.Meta.Ref(), reservation.Meta.Ref()) {
		t.Fatalf("historical reservation: %+v %v", historical.Meta.Ref(), err)
	}
	current, err := store.InspectCheckpointParticipantCurrent(context.Background(), participant.Meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Meta.Revision != participant.Meta.Revision+1 || current.ActiveReservation.Ref == nil || !contract.SameRef(*current.ActiveReservation.Ref, reservation.Meta.Ref()) {
		t.Fatalf("participant current was not advanced exactly: %+v", current)
	}
	if _, err := controller.ReservePhase(context.Background(), &reservation); err != nil {
		t.Fatalf("exact lost-reply replay did not recover: %v", err)
	}
	old, err := store.InspectCheckpointParticipant(context.Background(), participant.Meta.Ref())
	if err != nil || !contract.SameRef(old.Meta.Ref(), participant.Meta.Ref()) {
		t.Fatalf("old participant history changed: %+v %v", old.Meta.Ref(), err)
	}
}

func TestCheckpointSQLiteConcurrentCreateOnceHasOneWinner(t *testing.T) {
	store := openCheckpointStore(t)
	participant := testkit.CheckpointParticipant("sqlite-cas64")
	if err := store.CreateCheckpointParticipant(context.Background(), participant); err != nil {
		t.Fatal(err)
	}
	controller, err := kernel.NewCheckpointController(store, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	var winners atomic.Int64
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for index := 0; index < 64; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "sqlite-cas64-"+string(rune('a'+index)), participant, nil)
			// All candidates bind the same authoritative global attempt and Participant,
			// while carrying different content/IDs. The stable phase key must admit one.
			reservation.AttemptID = participant.CheckpointAttemptRef.ID
			reservation.Base.CheckpointAttempt = participant.CheckpointAttemptRef
			sealed, sealErr := contract.SealCheckpointPhaseReservation(reservation)
			if sealErr != nil {
				errs <- sealErr
				return
			}
			if _, reserveErr := controller.ReservePhase(context.Background(), &sealed); reserveErr == nil {
				winners.Add(1)
			} else if !errors.Is(reserveErr, ports.ErrConflict) {
				errs <- reserveErr
			}
		}(index)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if winners.Load() != 1 {
		t.Fatalf("checkpoint phase create-once winners=%d", winners.Load())
	}
	current, err := store.InspectCheckpointParticipantCurrent(context.Background(), participant.Meta.ID)
	if err != nil || current.Meta.Revision != participant.Meta.Revision+1 {
		t.Fatalf("participant current CAS did not advance once: %+v %v", current.Meta, err)
	}
}

func TestCheckpointSQLiteRejectsAttemptIdentityBypassBeforeWrite(t *testing.T) {
	store := openCheckpointStore(t)
	participant := testkit.CheckpointParticipant("sqlite-attempt")
	if err := store.CreateCheckpointParticipant(context.Background(), participant); err != nil {
		t.Fatal(err)
	}
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "sqlite-attempt", participant, nil)
	reservation.AttemptID = "caller-minted-attempt"
	if _, err := contract.SealCheckpointPhaseReservation(reservation); err == nil {
		t.Fatal("caller-minted AttemptID was accepted")
	}
	if _, err := store.InspectCheckpointPhaseReservation(context.Background(), reservation.Meta.Ref()); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("invalid reservation reached durable state: %v", err)
	}
}

func openCheckpointStore(t *testing.T) *sqlite.Store {
	t.Helper()
	store, err := sqlite.OpenWithClock(context.Background(), filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
