package runtimeadapter

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type checkpointResultStoreStubV2 struct {
	value contract.CheckpointPhaseDomainResultV2
}

func (*checkpointResultStoreStubV2) CreateCheckpointPhaseDomainResultV2(context.Context, contract.CheckpointPhaseDomainResultV2) (bool, error) {
	return false, ports.ErrUnsupported
}

func (s *checkpointResultStoreStubV2) InspectCheckpointPhaseDomainResultV2(_ context.Context, expected contract.SnapshotArtifactExactRefV2) (contract.CheckpointPhaseDomainResultV2, error) {
	if s.value.ExactRef() != expected {
		return contract.CheckpointPhaseDomainResultV2{}, ports.ErrNotFound
	}
	return s.value, nil
}

func (s *checkpointResultStoreStubV2) InspectCheckpointPhaseDomainResultByRefV2(_ context.Context, expected contract.Ref) (contract.CheckpointPhaseDomainResultV2, error) {
	if !contract.SameRef(s.value.Meta.Ref(), expected) {
		return contract.CheckpointPhaseDomainResultV2{}, ports.ErrNotFound
	}
	return s.value, nil
}

func (s *checkpointResultStoreStubV2) InspectCheckpointPhaseDomainResultByIDV2(_ context.Context, id string) (contract.CheckpointPhaseDomainResultV2, error) {
	if s.value.Meta.ID != id {
		return contract.CheckpointPhaseDomainResultV2{}, ports.ErrNotFound
	}
	return s.value, nil
}

func (s *checkpointResultStoreStubV2) InspectCheckpointPhaseDomainResultByReservationV2(_ context.Context, expected contract.Ref) (contract.CheckpointPhaseDomainResultV2, error) {
	if !contract.SameRef(s.value.ReservationRef, expected) {
		return contract.CheckpointPhaseDomainResultV2{}, ports.ErrNotFound
	}
	return s.value, nil
}

func (*checkpointResultStoreStubV2) CommitCheckpointPhaseApplySettlementV2(context.Context, contract.Ref, contract.CheckpointPhaseFact, contract.CheckpointParticipantFact) (bool, error) {
	return false, ports.ErrUnsupported
}

type checkpointReservationDriftReaderV2 struct {
	value runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2
	calls atomic.Uint64
}

func (r *checkpointReservationDriftReaderV2) InspectCheckpointParticipantPhaseReservationCurrentV2(context.Context, runtimeports.CheckpointParticipantPhaseReservationRefV2, runtimeports.CheckpointParticipantPhaseV2) (runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2, error) {
	value := r.value
	if r.calls.Add(1) > 1 {
		value.ProjectionDigest = runtimeDigest(testkit.Ref("drift").Digest)
	}
	return value, nil
}

func TestCheckpointDomainResultCurrentMapsExactSandboxOwnerFactV2(t *testing.T) {
	fixture := checkpointDispatchReaderFixtureV1(t, "domain-current", nil)
	domain := checkpointDomainResultFixtureV2(t, fixture.reservation)
	results := &checkpointResultStoreStubV2{value: domain}
	reservations := &checkpointRuntimeReservationReaderV1{value: fixture.runtimeReservation}
	adapter, err := NewCheckpointDomainResultCurrentAdapterV2(fixture.store, results, reservations, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	expected := checkpointRuntimeDomainResultRefV2(domain, fixture.runtimeReservation)
	projection, err := adapter.ReadCheckpointDomainResultCurrentV2(context.Background(), expected)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Ref != expected || projection.Validate(testkit.FixedNow) != nil || projection.ExpiresUnixNano != domain.Meta.ExpiresUnixNano {
		t.Fatalf("unexpected checkpoint DomainResult current projection: %+v", projection)
	}
}

func TestCheckpointDomainResultCurrentRejectsS1S2RuntimeReservationDriftV2(t *testing.T) {
	fixture := checkpointDispatchReaderFixtureV1(t, "domain-drift", nil)
	domain := checkpointDomainResultFixtureV2(t, fixture.reservation)
	drift := &checkpointReservationDriftReaderV2{value: fixture.runtimeReservation}
	adapter, err := NewCheckpointDomainResultCurrentAdapterV2(fixture.store, &checkpointResultStoreStubV2{value: domain}, drift, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ReadCheckpointDomainResultCurrentV2(context.Background(), checkpointRuntimeDomainResultRefV2(domain, fixture.runtimeReservation)); err == nil {
		t.Fatal("S1/S2 Runtime Reservation drift was accepted")
	}
}

func TestCheckpointDomainResultCurrentRejectsTypedNilAndExactExpiryV2(t *testing.T) {
	var results *checkpointResultStoreStubV2
	if _, err := NewCheckpointDomainResultCurrentAdapterV2(testkit.NewCheckpointMemoryStore(), results, &checkpointRuntimeReservationReaderV1{}, time.Now); err == nil {
		t.Fatal("typed-nil checkpoint DomainResult store was accepted")
	}
	fixture := checkpointDispatchReaderFixtureV1(t, "domain-expired", nil)
	domain := checkpointDomainResultFixtureV2(t, fixture.reservation)
	adapter, err := NewCheckpointDomainResultCurrentAdapterV2(fixture.store, &checkpointResultStoreStubV2{value: domain}, &checkpointRuntimeReservationReaderV1{value: fixture.runtimeReservation}, func() time.Time { return time.Unix(0, domain.Meta.ExpiresUnixNano) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ReadCheckpointDomainResultCurrentV2(context.Background(), checkpointRuntimeDomainResultRefV2(domain, fixture.runtimeReservation)); err == nil {
		t.Fatal("checkpoint DomainResult was current at now == expires")
	}
}

func TestCheckpointPhaseResultCurrentIsOwnerDerivedWithoutFinalPhaseFactV2(t *testing.T) {
	fixture := checkpointDispatchReaderFixtureV1(t, "phase-result", nil)
	domain := checkpointDomainResultFixtureV2(t, fixture.reservation)
	results := &checkpointResultStoreStubV2{value: domain}
	reservations := &checkpointRuntimeReservationReaderV1{value: fixture.runtimeReservation}
	adapter, err := NewCheckpointPhaseResultCurrentAdapterV2(fixture.store, results, reservations, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	_, _, expected, err := adapter.readCheckpointPhaseResultCurrentV2(context.Background(), domain, testkit.FixedNow)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := adapter.InspectCheckpointParticipantPhaseCurrentV2(context.Background(), expected)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Ref != expected || projection.Ref.State != runtimeports.CheckpointParticipantPreparedV2 || projection.Validate(testkit.FixedNow) != nil {
		t.Fatalf("unexpected checkpoint phase result current: %+v", projection)
	}
	if _, err := fixture.store.InspectCheckpointPhaseFactByReservation(context.Background(), fixture.reservation.Meta.Ref()); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("pre-Settlement current projection minted a final Sandbox PhaseFact: %v", err)
	}
}

func TestCheckpointPhaseResultCurrentRejectsS1S2DriftAndCallerMintedRefV2(t *testing.T) {
	fixture := checkpointDispatchReaderFixtureV1(t, "phase-result-drift", nil)
	domain := checkpointDomainResultFixtureV2(t, fixture.reservation)
	results := &checkpointResultStoreStubV2{value: domain}
	drift := &checkpointReservationDriftReaderV2{value: fixture.runtimeReservation}
	adapter, err := NewCheckpointPhaseResultCurrentAdapterV2(fixture.store, results, drift, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	_, _, expected, err := adapter.readCheckpointPhaseResultCurrentV2(context.Background(), domain, testkit.FixedNow)
	if err != nil {
		t.Fatal(err)
	}
	// readCheckpointPhaseResultCurrentV2 consumed the first read; reset so the
	// public call proves its own S1/S2 drift rejection.
	drift.calls.Store(0)
	if _, err := adapter.InspectCheckpointParticipantPhaseCurrentV2(context.Background(), expected); err == nil {
		t.Fatal("checkpoint phase result S1/S2 drift was accepted")
	}
	stable := &checkpointRuntimeReservationReaderV1{value: fixture.runtimeReservation}
	adapter, err = NewCheckpointPhaseResultCurrentAdapterV2(fixture.store, results, stable, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	minted := expected
	minted.Digest = runtimeDigest(testkit.Ref("caller-minted-phase-result").Digest)
	if _, err := adapter.InspectCheckpointParticipantPhaseCurrentV2(context.Background(), minted); err == nil {
		t.Fatal("caller-minted checkpoint phase result ref was accepted")
	}
}

func checkpointDomainResultFixtureV2(t *testing.T, reservation contract.CheckpointPhaseReservation) contract.CheckpointPhaseDomainResultV2 {
	t.Helper()
	meta := testkit.Meta(reservation.Meta.ID+"-domain-result", 1)
	meta.ExpiresUnixNano = reservation.Meta.ExpiresUnixNano
	value, err := contract.SealCheckpointPhaseDomainResultV2(contract.CheckpointPhaseDomainResultV2{
		Meta: meta, ReservationRef: reservation.Meta.Ref(), TenantID: reservation.TenantID,
		ParticipantRef: reservation.ParticipantRef, CheckpointAttemptRef: reservation.Base.CheckpointAttempt,
		Phase: reservation.Phase, PreviousPresence: reservation.PreviousPresence, PreviousPhase: reservation.PreviousPhase,
		OperationID: reservation.OperationID, EffectID: reservation.EffectID, AttemptID: reservation.AttemptID,
		State: contract.CheckpointPhasePrepared, ProviderAttemptRef: testkit.Ref("checkpoint-provider-attempt"),
		ProviderObservation: testkit.Ref("checkpoint-provider-observation"), ProviderReceipt: testkit.Ref("checkpoint-provider-receipt"),
		EvidenceConsumption: testkit.Ref("checkpoint-evidence-consumption"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
