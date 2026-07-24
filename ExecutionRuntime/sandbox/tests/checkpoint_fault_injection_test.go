package sandbox_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
)

func TestCheckpointFaultMissingRequiredGateMakesZeroProviderCalls(t *testing.T) {
	t.Parallel()
	store := testkit.NewCheckpointMemoryStore()
	initial := testkit.CheckpointParticipant("missing-gate")
	if err := store.SeedCheckpointParticipant(initial); err != nil {
		t.Fatal(err)
	}
	controller, _ := kernel.NewCheckpointController(store, func() time.Time { return testkit.FixedNow })
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "missing-gate", initial, nil)
	if _, err := controller.ReservePhase(context.Background(), &reservation); err != nil {
		t.Fatal(err)
	}
	participant, _ := store.InspectCheckpointParticipantCurrent(context.Background(), initial.Meta.ID)
	coordinates, request := testkit.CheckpointCurrentFixture(reservation, participant, contract.CheckpointReadPreExecute)
	if err := store.SeedCheckpointCurrent(coordinates[:len(coordinates)-1]...); err != nil {
		t.Fatal(err)
	}
	reader, _ := kernel.NewCheckpointCurrentReader(store, store, func() time.Time { return testkit.FixedNow })
	var providerCalls atomic.Uint64
	if _, err := reader.ReadCheckpointParticipantCurrent(context.Background(), &request); err == nil {
		providerCalls.Add(1)
	}
	if providerCalls.Load() != 0 {
		t.Fatalf("provider calls=%d", providerCalls.Load())
	}
}

func TestCheckpointFaultExpiredHistoricalFactInspectsButCannotUnlock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewCheckpointMemoryStore()
	initial := testkit.CheckpointParticipant("historical")
	if err := store.SeedCheckpointParticipant(initial); err != nil {
		t.Fatal(err)
	}
	controller, _ := kernel.NewCheckpointController(store, func() time.Time { return testkit.FixedNow })
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "historical", initial, nil)
	if _, err := controller.ReservePhase(ctx, &reservation); err != nil {
		t.Fatal(err)
	}
	participant, _ := store.InspectCheckpointParticipantCurrent(ctx, initial.Meta.ID)
	fact, nextParticipant := testkit.CheckpointAppliedPhase(reservation, participant, contract.CheckpointPhasePrepared, "historical", testkit.FixedNow.Add(time.Hour))
	if err := store.AppendAppliedCheckpointPhase(nil, participant.Meta.Ref(), fact, nextParticipant, fact.Meta.ExpiresUnixNano); err != nil {
		t.Fatal(err)
	}
	historical, err := store.InspectCheckpointPhaseFact(ctx, fact.Meta.Ref())
	if err != nil || historical.State != contract.CheckpointPhasePrepared {
		t.Fatalf("historical state=%q err=%v", historical.State, err)
	}
	expiredController, _ := kernel.NewCheckpointController(store, func() time.Time { return time.Unix(0, fact.Meta.ExpiresUnixNano) })
	closure := fact.ClosureRef()
	commit := testkit.CheckpointReservation(contract.CheckpointPhaseCommit, "historical-commit", nextParticipant, &closure)
	if _, err := expiredController.ReservePhase(ctx, &commit); err == nil {
		t.Fatal("expired closure unlocked a successor")
	}
}
