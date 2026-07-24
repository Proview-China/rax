package sandbox_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/storage/sqlite"
)

func TestCheckpointDomainResultToApplySettlementBlackBoxV2(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedNow
	clock := func() time.Time { return now }
	store, err := sqlite.OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), clock)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	participant := testkit.CheckpointParticipant("domain-result")
	if err := store.CreateCheckpointParticipant(ctx, participant); err != nil {
		t.Fatal(err)
	}
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "domain-result", participant, nil)
	controller, err := kernel.NewCheckpointController(store, clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.ReservePhase(ctx, &reservation); err != nil {
		t.Fatal(err)
	}
	projection := checkpointResultProjectionV2(t, reservation, contract.CheckpointPhasePrepared, now)
	current := &checkpointResultCurrentReaderV2{value: projection}
	settlements := &checkpointSettlementCurrentReaderV2{}
	owner, err := kernel.NewCheckpointPhaseResultOwnerV2(store, store, current, settlements, clock, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	record := contract.RecordCheckpointPhaseDomainResultRequestV2{ReservationRef: reservation.Meta.Ref(), ExpectedProjectionDigest: projection.ProjectionDigest, RequestedNotAfter: now.Add(time.Hour).UnixNano()}
	domain, err := owner.RecordCheckpointPhaseDomainResultV2(ctx, &record)
	if err != nil || domain.State != contract.CheckpointPhasePrepared {
		t.Fatalf("domain=%#v err=%v", domain, err)
	}
	settlement := testkit.Ref("runtime-checkpoint-settlement-domain-result")
	settlements.value, err = contract.SealCheckpointPhaseSettlementCurrentProjectionV2(contract.CheckpointPhaseSettlementCurrentProjectionV2{DomainResultRef: domain.ExactRef(), RuntimeSettlementRef: settlement, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	apply := contract.CheckpointPhaseApplySettlementV2{DomainResultRef: domain.ExactRef(), RuntimeSettlementRef: settlement}
	fact, err := owner.ApplyCheckpointPhaseSettlementV2(ctx, &apply)
	if err != nil || fact.State != contract.CheckpointPhasePrepared || fact.DomainResultRef.ID != domain.Meta.ID {
		t.Fatalf("fact=%#v err=%v", fact, err)
	}
	replay, err := owner.ApplyCheckpointPhaseSettlementV2(ctx, &apply)
	if err != nil || !contract.SameRef(replay.Meta.Ref(), fact.Meta.Ref()) {
		t.Fatalf("replay=%#v err=%v", replay, err)
	}
	currentParticipant, err := store.InspectCheckpointParticipantCurrent(ctx, participant.Meta.ID)
	if err != nil || currentParticipant.State != contract.CheckpointParticipantPrepared || currentParticipant.Closure == nil || !contract.SameCheckpointPhaseClosure(*currentParticipant.Closure, fact.ClosureRef()) {
		t.Fatalf("participant=%#v err=%v", currentParticipant, err)
	}
	historical, err := store.InspectCheckpointPhaseDomainResultV2(ctx, domain.ExactRef())
	if err != nil || historical.ExactRef() != domain.ExactRef() {
		t.Fatalf("historical=%#v err=%v", historical, err)
	}
}

func TestCheckpointApplySettlementConcurrentReplaySingleFactV2(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedNow
	store, err := sqlite.OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	participant := testkit.CheckpointParticipant("apply-race")
	if err := store.CreateCheckpointParticipant(ctx, participant); err != nil {
		t.Fatal(err)
	}
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "apply-race", participant, nil)
	controller, _ := kernel.NewCheckpointController(store, func() time.Time { return now })
	if _, err := controller.ReservePhase(ctx, &reservation); err != nil {
		t.Fatal(err)
	}
	projection := checkpointResultProjectionV2(t, reservation, contract.CheckpointPhasePrepared, now)
	settlementReader := &checkpointSettlementCurrentReaderV2{}
	owner, _ := kernel.NewCheckpointPhaseResultOwnerV2(store, store, &checkpointResultCurrentReaderV2{value: projection}, settlementReader, func() time.Time { return now }, time.Hour)
	domain, err := owner.RecordCheckpointPhaseDomainResultV2(ctx, &contract.RecordCheckpointPhaseDomainResultRequestV2{ReservationRef: reservation.Meta.Ref(), ExpectedProjectionDigest: projection.ProjectionDigest, RequestedNotAfter: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	settlement := testkit.Ref("runtime-checkpoint-settlement-race")
	settlementReader.value, _ = contract.SealCheckpointPhaseSettlementCurrentProjectionV2(contract.CheckpointPhaseSettlementCurrentProjectionV2{DomainResultRef: domain.ExactRef(), RuntimeSettlementRef: settlement, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	request := contract.CheckpointPhaseApplySettlementV2{DomainResultRef: domain.ExactRef(), RuntimeSettlementRef: settlement}
	refs := make(chan contract.Ref, 64)
	errs := make(chan error, 64)
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fact, err := owner.ApplyCheckpointPhaseSettlementV2(ctx, &request)
			refs <- fact.Meta.Ref()
			errs <- err
		}()
	}
	wg.Wait()
	close(refs)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent ApplySettlement: %v", err)
		}
	}
	var winner contract.Ref
	for ref := range refs {
		if winner.ID == "" {
			winner = ref
		}
		if !contract.SameRef(ref, winner) {
			t.Fatalf("multiple phase Facts: %#v vs %#v", winner, ref)
		}
	}
}

func checkpointResultProjectionV2(t *testing.T, reservation contract.CheckpointPhaseReservation, state contract.CheckpointPhaseState, now time.Time) contract.CheckpointPhaseResultCurrentProjectionV2 {
	t.Helper()
	value, err := contract.SealCheckpointPhaseResultCurrentProjectionV2(contract.CheckpointPhaseResultCurrentProjectionV2{ReservationRef: reservation.Meta.Ref(), State: state, ProviderAttemptRef: testkit.Ref("provider-attempt"), ProviderObservation: testkit.Ref("provider-observation"), ProviderReceipt: testkit.Ref("provider-receipt"), EvidenceConsumption: testkit.Ref("evidence-consumption"), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

type checkpointResultCurrentReaderV2 struct {
	value contract.CheckpointPhaseResultCurrentProjectionV2
}

func (r *checkpointResultCurrentReaderV2) InspectCheckpointPhaseResultCurrentV2(context.Context, contract.Ref) (contract.CheckpointPhaseResultCurrentProjectionV2, error) {
	return r.value, nil
}

type checkpointSettlementCurrentReaderV2 struct {
	value contract.CheckpointPhaseSettlementCurrentProjectionV2
}

func (r *checkpointSettlementCurrentReaderV2) InspectCheckpointPhaseSettlementCurrentV2(context.Context, contract.SnapshotArtifactExactRefV2, contract.Ref) (contract.CheckpointPhaseSettlementCurrentProjectionV2, error) {
	return r.value, nil
}
