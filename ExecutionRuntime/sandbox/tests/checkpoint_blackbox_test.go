package sandbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestCheckpointBlackBoxOwnerAppliedPreparedClosureUnlocksOneSuccessor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := testkit.NewCheckpointMemoryStore()
	initial := testkit.CheckpointParticipant("blackbox")
	if err := store.SeedCheckpointParticipant(initial); err != nil {
		t.Fatal(err)
	}
	controller, _ := kernel.NewCheckpointController(store, func() time.Time { return testkit.FixedNow })
	prepare := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "blackbox-prepare", initial, nil)
	if _, err := controller.ReservePhase(ctx, &prepare); err != nil {
		t.Fatal(err)
	}
	reserved, _ := store.InspectCheckpointParticipantCurrent(ctx, initial.Meta.ID)
	coordinates, request := testkit.CheckpointCurrentFixture(prepare, reserved, contract.CheckpointReadPreExecute)
	if err := store.SeedCheckpointCurrent(coordinates...); err != nil {
		t.Fatal(err)
	}
	reader, _ := kernel.NewCheckpointCurrentReader(store, store, func() time.Time { return testkit.FixedNow })
	projection, err := reader.ReadCheckpointParticipantCurrent(ctx, &request)
	if err != nil {
		t.Fatal(err)
	}
	prepared, appliedParticipant := testkit.CheckpointAppliedPhase(prepare, reserved, contract.CheckpointPhasePrepared, "blackbox-prepare", time.Unix(0, projection.ExpiresUnixNano))
	if err := store.AppendAppliedCheckpointPhase(nil, reserved.Meta.Ref(), prepared, appliedParticipant, projection.ExpiresUnixNano); err != nil {
		t.Fatal(err)
	}

	closure := prepared.ClosureRef()
	commit := testkit.CheckpointReservation(contract.CheckpointPhaseCommit, "blackbox-commit", appliedParticipant, &closure)
	if _, err := controller.ReservePhase(ctx, &commit); err != nil {
		t.Fatal(err)
	}
	abort := testkit.CheckpointReservation(contract.CheckpointPhaseAbort, "blackbox-abort", appliedParticipant, &closure)
	if _, err := controller.ReservePhase(ctx, &abort); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("sibling abort = %v", err)
	}
}

func TestCheckpointBlackBoxFailedAndNotAppliedHaveNoSuccessor(t *testing.T) {
	t.Parallel()
	for _, state := range []contract.CheckpointPhaseState{contract.CheckpointPhaseFailed, contract.CheckpointPhaseNotApplied} {
		t.Run(string(state), func(t *testing.T) {
			participant := testkit.CheckpointParticipant("blackbox-" + string(state))
			reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "blackbox-"+string(state), participant, nil)
			fact, _ := testkit.CheckpointAppliedPhase(reservation, participant, state, "blackbox-"+string(state), testkit.FixedNow.Add(time.Hour))
			closure := fact.ClosureRef()
			candidate := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "candidate-"+string(state), participant, nil)
			candidate.Phase = contract.CheckpointPhaseCommit
			candidate.PreviousPresence = contract.CheckpointPresent
			candidate.PreviousPhase = &closure
			if _, err := contract.SealCheckpointPhaseReservation(candidate); err == nil {
				t.Fatalf("%q created a successor", state)
			}
		})
	}
}
