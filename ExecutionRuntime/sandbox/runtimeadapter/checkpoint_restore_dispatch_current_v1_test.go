package runtimeadapter

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type checkpointRuntimeReservationReaderV1 struct {
	value runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2
}

func (r *checkpointRuntimeReservationReaderV1) InspectCheckpointParticipantPhaseReservationCurrentV2(_ context.Context, expected runtimeports.CheckpointParticipantPhaseReservationRefV2, phase runtimeports.CheckpointParticipantPhaseV2) (runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2, error) {
	if r.value.Ref != expected || r.value.Phase != phase {
		return runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2{}, sandboxports.ErrNotFound
	}
	return r.value, nil
}

type checkpointExecutionReaderV1 struct {
	operation runtimeports.OperationSubjectV3
	value     CheckpointDispatchExecutionCurrentV1
}

func (r *checkpointExecutionReaderV1) InspectCheckpointDispatchExecutionCurrentV1(_ context.Context, operation runtimeports.OperationSubjectV3, effectID runtimecore.EffectIntentID, attemptID string) (CheckpointDispatchExecutionCurrentV1, error) {
	if !runtimeports.SameOperationSubjectV3(operation, r.operation) || r.value.Attempt.EffectID != effectID || r.value.Attempt.AttemptID != attemptID {
		return CheckpointDispatchExecutionCurrentV1{}, sandboxports.ErrNotFound
	}
	return r.value, nil
}

type checkpointS1S2DriftSourceV1 struct {
	sandboxports.CheckpointCurrentSource
	calls atomic.Uint64
	limit uint64
}

func (s *checkpointS1S2DriftSourceV1) InspectCheckpointCurrent(ctx context.Context, query contract.CheckpointCurrentQuery) (contract.CheckpointCurrentCoordinate, error) {
	value, err := s.CheckpointCurrentSource.InspectCheckpointCurrent(ctx, query)
	if err == nil && s.calls.Add(1) > s.limit && query.Kind == contract.CheckpointCurrentAdmission {
		value.Meta.Revision++
		value.Meta.Digest = testkit.Ref("checkpoint-s1-s2-drift").Digest
	}
	return value, err
}

func TestCheckpointRestoreDispatchCurrentReaderMapsExactOwnerS1S2(t *testing.T) {
	fixture := checkpointDispatchReaderFixtureV1(t, "happy", nil)
	projection, err := fixture.reader.InspectCheckpointRestoreDispatchSandboxCurrentV1(context.Background(), fixture.operation, fixture.effectID, fixture.runtimeReservation.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ValidateCurrent(fixture.operation, fixture.effectID, fixture.runtimeReservation.Ref, runtimeports.CheckpointRestoreDispatchSandboxPrePrepareV1, testkit.FixedNow); err != nil {
		t.Fatal(err)
	}
	if projection.DispatchAttempt.ID != fixture.reservation.ExpectedRuntimeAttemptRef.ID || projection.SandboxReservation.ID != fixture.reservation.Meta.ID || projection.Participant.Revision != runtimecore.Revision(fixture.participant.Meta.Revision) || projection.RuntimeLease.FenceEpoch != runtimecore.Epoch(fixture.reservation.Runtime.FenceEpoch) {
		t.Fatalf("checkpoint exact mapping drifted: %#v", projection)
	}
}

func TestCheckpointRestoreDispatchCurrentReaderRejectsS1S2Drift(t *testing.T) {
	base := checkpointDispatchReaderFixtureV1(t, "drift", func(store *testkit.CheckpointMemoryStore) sandboxports.CheckpointCurrentSource {
		return &checkpointS1S2DriftSourceV1{CheckpointCurrentSource: store, limit: uint64(len(contract.RequiredCheckpointCurrentKinds(contract.CheckpointReadPrePrepare)))}
	})
	if _, err := base.reader.InspectCheckpointRestoreDispatchSandboxCurrentV1(context.Background(), base.operation, base.effectID, base.runtimeReservation.Ref); err == nil {
		t.Fatal("checkpoint S1/S2 current drift produced a Runtime projection")
	}
}

func TestCheckpointRestoreDispatchCurrentReaderRejectsTypedNilAndExactExpiry(t *testing.T) {
	var store *testkit.CheckpointMemoryStore
	if _, err := NewCheckpointRestoreDispatchCurrentReaderV1(store, store, nil, nil, nil, time.Now); err == nil {
		t.Fatal("typed-nil checkpoint dependencies were accepted")
	}
	fixture := checkpointDispatchReaderFixtureV1(t, "expiry", nil)
	fixture.reader.clock = func() time.Time { return time.Unix(0, fixture.runtimeReservation.ExpiresUnixNano) }
	if _, err := fixture.reader.InspectCheckpointRestoreDispatchSandboxCurrentV1(context.Background(), fixture.operation, fixture.effectID, fixture.runtimeReservation.Ref); err == nil {
		t.Fatal("now == expires was accepted as checkpoint current")
	}
}

type checkpointDispatchReaderFixture struct {
	reader             *CheckpointRestoreDispatchCurrentReaderV1
	store              *testkit.CheckpointMemoryStore
	reservation        contract.CheckpointPhaseReservation
	participant        contract.CheckpointParticipantFact
	operation          runtimeports.OperationSubjectV3
	effectID           runtimecore.EffectIntentID
	runtimeReservation runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2
}

func checkpointDispatchReaderFixtureV1(t *testing.T, suffix string, sourceFactory func(*testkit.CheckpointMemoryStore) sandboxports.CheckpointCurrentSource) checkpointDispatchReaderFixture {
	t.Helper()
	store := testkit.NewCheckpointMemoryStore()
	initial := testkit.CheckpointParticipant("runtimeadapter-" + suffix)
	if err := store.SeedCheckpointParticipant(initial); err != nil {
		t.Fatal(err)
	}
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "runtimeadapter-"+suffix, initial, nil)
	controller, err := kernel.NewCheckpointController(store, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.ReservePhase(context.Background(), &reservation); err != nil {
		t.Fatal(err)
	}
	participant, err := store.InspectCheckpointParticipantCurrent(context.Background(), initial.Meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	coordinates, _ := testkit.CheckpointCurrentFixture(reservation, participant, contract.CheckpointReadPrePrepare)
	if err := store.SeedCheckpointCurrent(coordinates...); err != nil {
		t.Fatal(err)
	}
	source := sandboxports.CheckpointCurrentSource(store)
	if sourceFactory != nil {
		source = sourceFactory(store)
	}
	ownerReader, err := kernel.NewCheckpointCurrentReader(store, source, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	operation := checkpointRuntimeOperationV1(t, reservation)
	effectID := runtimecore.EffectIntentID(reservation.EffectID)
	operationDigest, _ := operation.DigestV3()
	intentDigest := runtimecore.DigestBytes([]byte("checkpoint-intent-" + suffix))
	runtimeAttempt := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: effectID, IntentRevision: 1, IntentDigest: intentDigest, PermitID: "checkpoint-permit-" + suffix, PermitRevision: 1, PermitDigest: runtimecore.DigestBytes([]byte("checkpoint-permit-" + suffix)), AttemptID: reservation.ExpectedRuntimeAttemptRef.ID}
	operationFact := findCheckpointCoordinateV1(t, coordinates, contract.CheckpointCurrentOperation).Meta.Ref()
	attemptFact := findCheckpointCoordinateV1(t, coordinates, contract.CheckpointCurrentAttempt).Meta.Ref()
	execution, err := SealCheckpointDispatchExecutionCurrentV1(CheckpointDispatchExecutionCurrentV1{OperationFact: operationFact, AttemptFact: attemptFact, Attempt: runtimeAttempt, Current: true, CheckedUnixNano: testkit.FixedNow.UnixNano(), ExpiresUnixNano: testkit.FixedNow.Add(5 * time.Hour).UnixNano()}, testkit.FixedNow)
	if err != nil {
		t.Fatal(err)
	}
	provider := runtimeports.ProviderBindingRefV2{BindingSetID: "checkpoint-binding-" + suffix, BindingSetRevision: 1, ComponentID: "praxis/sandbox", ManifestDigest: runtimecore.DigestBytes([]byte("checkpoint-manifest-" + suffix)), ArtifactDigest: runtimecore.DigestBytes([]byte("checkpoint-artifact-" + suffix)), Capability: "praxis.sandbox/checkpoint"}
	runtimeReservation := checkpointRuntimeReservationProjectionV1(t, reservation, operation, effectID, intentDigest, provider)
	reader, err := NewCheckpointRestoreDispatchCurrentReaderV1(store, source, ownerReader, &checkpointRuntimeReservationReaderV1{value: runtimeReservation}, &checkpointExecutionReaderV1{operation: operation, value: execution}, func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	return checkpointDispatchReaderFixture{reader: reader, store: store, reservation: reservation, participant: participant, operation: operation, effectID: effectID, runtimeReservation: runtimeReservation}
}

func checkpointRuntimeOperationV1(t *testing.T, reservation contract.CheckpointPhaseReservation) runtimeports.OperationSubjectV3 {
	t.Helper()
	lease := runtimecore.SandboxLeaseRef{ID: runtimecore.SandboxLeaseID(reservation.Runtime.LeaseID), Epoch: runtimecore.Epoch(reservation.Runtime.LeaseEpoch)}
	scope := runtimecore.ExecutionScope{Identity: runtimecore.AgentIdentityRef{TenantID: runtimecore.TenantID(reservation.TenantID), ID: "checkpoint-identity", Epoch: 1}, Lineage: runtimecore.LineageRef{ID: "checkpoint-lineage", PlanDigest: runtimecore.DigestBytes([]byte("checkpoint-lineage"))}, Instance: runtimecore.InstanceRef{ID: runtimecore.AgentInstanceID(reservation.Runtime.InstanceID), Epoch: runtimecore.Epoch(reservation.Runtime.InstanceEpoch)}, SandboxLease: &lease, AuthorityEpoch: runtimecore.Epoch(reservation.Runtime.FenceEpoch)}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	return runtimeports.OperationSubjectV3{Kind: runtimeports.OperationScopeKindV3("praxis.checkpoint/participant"), ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, CustomOperationID: reservation.OperationID, SubjectRevision: 1, CurrentProjectionRef: "checkpoint-current-operation", CurrentProjectionRevision: 1, CurrentProjectionDigest: runtimecore.DigestBytes([]byte("checkpoint-current-operation"))}
}

func checkpointRuntimeReservationProjectionV1(t *testing.T, local contract.CheckpointPhaseReservation, operation runtimeports.OperationSubjectV3, effectID runtimecore.EffectIntentID, intentDigest runtimecore.Digest, owner runtimeports.ProviderBindingRefV2) runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2 {
	t.Helper()
	expires := local.Meta.ExpiresUnixNano
	attempt := runtimeports.CheckpointAttemptRefV2{TenantID: runtimecore.TenantID(local.TenantID), ID: local.Base.CheckpointAttempt.ID, Revision: runtimecore.Revision(local.Base.CheckpointAttempt.Revision), Digest: runtimeDigest(local.Base.CheckpointAttempt.Digest)}
	barrier := runtimeports.CheckpointBarrierLeaseRefV2{TenantID: attempt.TenantID, ID: local.Base.Barrier.ID, AttemptID: attempt.ID, Revision: runtimecore.Revision(local.Base.Barrier.Revision), Digest: runtimeDigest(local.Base.Barrier.Digest), ExpiresUnixNano: expires}
	cut := runtimeports.EffectCutRefV2{ID: local.Base.EffectCut.ID, Revision: runtimecore.Revision(local.Base.EffectCut.Revision), Attempt: attempt, RootDigest: runtimecore.DigestBytes([]byte("checkpoint-cut-root")), Watermark: 1, Digest: runtimeDigest(local.Base.EffectCut.Digest)}
	value := runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2{
		ContractVersion: runtimeports.CheckpointParticipantReservationContractVersionV2,
		Ref:             runtimeports.CheckpointParticipantPhaseReservationRefV2{ID: local.Meta.ID, Revision: runtimecore.Revision(local.Meta.Revision), Digest: runtimeDigest(local.Meta.Digest), ExpiresUnixNano: expires},
		Participant:     runtimeports.CheckpointParticipantRefV2{ID: local.ParticipantRef.ID, Owner: owner, Digest: runtimeDigest(local.ParticipantRef.Digest)}, OwnerBinding: owner,
		Phase: runtimeports.CheckpointPhasePrepareV2, Attempt: attempt, Barrier: barrier, EffectCut: cut, Operation: operation, OperationDigest: mustRuntimeOperationDigestV1(operation), EffectID: effectID, EffectKind: "praxis.sandbox/checkpoint", IntentDigest: intentDigest,
		Domain:          runtimeports.CheckpointParticipantDomainReservationRefV2{ID: "checkpoint-domain-" + local.Meta.ID, Revision: 1, Digest: runtimecore.DigestBytes([]byte("checkpoint-domain-" + local.Meta.ID))},
		Generation:      runtimeports.GenerationBindingAssociationRefV1{ID: local.Base.Generation.ID, Revision: runtimecore.Revision(local.Base.Generation.Revision), Digest: runtimeDigest(local.Base.Generation.Digest)},
		CheckedUnixNano: testkit.FixedNow.UnixNano(), ExpiresUnixNano: expires,
	}
	digest, err := value.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	value.ProjectionDigest = digest
	if err := value.Validate(testkit.FixedNow); err != nil {
		t.Fatal(err)
	}
	return value
}

func findCheckpointCoordinateV1(t *testing.T, values []contract.CheckpointCurrentCoordinate, kind contract.CheckpointCurrentKind) contract.CheckpointCurrentCoordinate {
	t.Helper()
	for _, value := range values {
		if value.Kind == kind {
			return value
		}
	}
	t.Fatalf("checkpoint coordinate %s is absent", kind)
	return contract.CheckpointCurrentCoordinate{}
}
