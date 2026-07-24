package applicationadapter

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceCheckpointPreparationCurrentReaderReReadsExactOwnersV2(t *testing.T) {
	ctx := context.Background()
	now := testkit.FixedNow
	participant := testkit.CheckpointParticipant("workspace-current")
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "workspace-current", participant, nil)
	checkpointStore := &workspaceCheckpointCurrentStoreV2{CheckpointMemoryStore: testkit.NewCheckpointMemoryStore()}
	if err := checkpointStore.SeedCheckpointParticipant(participant); err != nil {
		t.Fatal(err)
	}
	controller, err := kernel.NewCheckpointController(checkpointStore, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	_, err = controller.ReservePhase(ctx, &reservation)
	if err != nil {
		t.Fatal(err)
	}
	reservedParticipant, err := checkpointStore.InspectCheckpointParticipantCurrent(ctx, participant.Meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	phase, next := testkit.CheckpointAppliedPhase(reservation, reservedParticipant, contract.CheckpointPhasePrepared, "workspace-current", now.Add(time.Hour))
	if err := checkpointStore.AppendAppliedCheckpointPhase(nil, reservedParticipant.Meta.Ref(), phase, next, now.Add(time.Hour).UnixNano()); err != nil {
		t.Fatal(err)
	}

	baseArtifacts := testkit.NewSnapshotArtifactMemoryStore()
	countingArtifacts := &workspaceCheckpointCountingSnapshotStoreV2{SnapshotArtifactMemoryStore: baseArtifacts}
	reserveOwner, err := kernel.NewSnapshotArtifactOwner(countingArtifacts, func() time.Time { return now }, kernel.SnapshotArtifactOwnerLimits{MaxReservationTTL: time.Hour, MaxHistoryTTL: 2 * time.Hour, MaxProjectionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	artifactRequest := testkit.SnapshotArtifactRequest("workspace-current")
	artifactRequest.TenantID = participant.TenantID
	artifactRequest.DataDomain = contract.WorkspaceSnapshotDataDomain
	artifactRequest.SourceOperationID = reservation.OperationID
	artifactRequest.SourceEffectID = reservation.EffectID
	artifactRequest.SourceAttemptRef = reservation.Base.CheckpointAttempt
	reservedArtifact, err := reserveOwner.ReserveArtifact(ctx, &artifactRequest)
	if err != nil {
		t.Fatal(err)
	}
	commit := testkit.SnapshotArtifactCommitRequest(reservedArtifact.Reservation, reservedArtifact.CurrentIndex, "workspace-current", now)
	commit.SourceAttemptRef = reservation.Base.CheckpointAttempt
	commitCurrent := &testkit.SnapshotArtifactCommitCurrentReader{ReadFunc: func(_ int, request contract.CommitSnapshotArtifactRequestV2) (contract.SnapshotArtifactCommitCurrentProjectionV2, error) {
		return testkit.SnapshotArtifactCommitProjection(request, participant.TenantID, contract.WorkspaceSnapshotDataDomain, now), nil
	}}
	artifactOwner, err := kernel.NewSnapshotArtifactOwnerWithCommitCurrent(countingArtifacts, commitCurrent, func() time.Time { return now }, kernel.SnapshotArtifactOwnerLimits{MaxReservationTTL: time.Hour, MaxHistoryTTL: 2 * time.Hour, MaxProjectionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	committed, err := artifactOwner.CommitArtifact(ctx, &commit)
	if err != nil {
		t.Fatal(err)
	}
	providerSubject := testkit.Ref("workspace-current-provider-subject").Digest
	binding, err := SealCheckpointSnapshotCaptureBindingV2(CheckpointSnapshotCaptureBindingV2{
		SnapshotReservation: reservedArtifact.Reservation.ExactRef(), CheckpointReservation: reservation.Meta.Ref(), StorageArtifactRef: commit.StorageArtifactRef,
		ProviderArtifact:             contract.CheckpointWorkspaceArtifactObservationV2{Provider: "host_workspace", ArtifactID: "praxis-checkpoint:" + providerSubject, SubjectDigest: "sha256:" + providerSubject, ContentDigest: "sha256:" + commit.StorageArtifactRef.ContentDigest, ContentLength: commit.StorageArtifactRef.Length, State: "prepared", CheckpointPhase: "checkpoint_prepare", RecordedUnixNano: now.UnixNano(), ExpiresUnixNano: committed.Fact.Meta.ExpiresUnixNano},
		MaterializationInspectionRef: testkit.Ref("workspace-current-materialization"), WorkspaceBundleDigest: testkit.Ref("workspace-current-bundle").Digest,
		ProviderObservationRef: commit.ProviderObservationRef, ProviderReceiptRef: commit.ProviderReceiptRef, EvidenceConsumptionRef: commit.FormalEvidenceRefs[0], OwnerInspectionRef: commit.OwnerInspectionRef,
		SourceAttemptRef: reservation.Base.CheckpointAttempt, TenantID: participant.TenantID, ScopeDigest: testkit.Ref("workspace-current-scope").Digest, RunID: "workspace-current-run",
		CheckpointAttemptRef: reservation.Base.CheckpointAttempt, BarrierRef: reservation.Base.Barrier, EffectCutRef: reservation.Base.EffectCut, ParticipantID: participant.Meta.ID, ParticipantDigest: participant.Meta.Digest,
		WorkspaceStableID: "workspace-current-stable", CoveragePolicyRef: testkit.Ref("workspace-current-coverage-policy"),
		Included: []string{"workspace/content", "workspace/metadata"}, DeclaredExcluded: []string{"device_state", "network_session", "process_state", "secret_material"}, ResidualRefs: []contract.Ref{},
		RequestedNotAfter: committed.Fact.Meta.ExpiresUnixNano, ExpiresUnixNano: committed.Fact.Meta.ExpiresUnixNano,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	countingArtifacts.mu.Lock()
	countingArtifacts.currentReads = 0
	countingArtifacts.mu.Unlock()
	bindings := &workspaceCheckpointCaptureBindingStoreV2{value: binding}
	composition := &CheckpointProductionCompositionV2{Store: checkpointStore}
	reader, err := NewWorkspaceCheckpointPreparationCurrentReaderV2(composition, bindings, countingArtifacts, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	request := contract.PrepareWorkspaceCheckpointParticipantRequestV2{
		StableID: binding.WorkspaceStableID, TenantID: participant.TenantID, ScopeDigest: testkit.Ref("workspace-current-scope").Digest, RunID: "workspace-current-run",
		CheckpointAttemptRef: reservation.Base.CheckpointAttempt, BarrierRef: reservation.Base.Barrier, EffectCutRef: reservation.Base.EffectCut,
		ParticipantID: participant.Meta.ID, ParticipantDigest: participant.Meta.Digest, PreparedPhaseFactRef: phase.Meta.Ref(), SnapshotArtifactFactRef: committed.Fact.ExactRef(),
		CoveragePolicyRef: binding.CoveragePolicyRef, RequestedNotAfter: committed.Fact.Meta.ExpiresUnixNano,
	}
	first, err := reader.InspectWorkspaceCheckpointPreparationCurrentV2(ctx, request)
	if err != nil || first.ValidateCurrent(now) != nil || first.SnapshotAggregateRef != committed.CurrentIndex.HeadAggregateEnvelopeRef {
		t.Fatalf("workspace checkpoint current = %+v err=%v", first, err)
	}
	second, err := reader.InspectWorkspaceCheckpointPreparationCurrentV2(ctx, request)
	if err != nil || second.ProjectionDigest != first.ProjectionDigest || countingArtifacts.currentReads != 2 || bindings.reads != 2 {
		t.Fatalf("reader cached a prior projection: second=%+v err=%v current_reads=%d binding_reads=%d", second, err, countingArtifacts.currentReads, bindings.reads)
	}
	drift := request
	drift.CoveragePolicyRef = testkit.Ref("workspace-current-drift")
	if _, err := reader.InspectWorkspaceCheckpointPreparationCurrentV2(ctx, drift); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("coverage binding drift did not fail closed: %v", err)
	}
	now = time.Unix(0, binding.ExpiresUnixNano)
	if _, err := reader.InspectWorkspaceCheckpointPreparationCurrentV2(ctx, request); err == nil {
		t.Fatal("now == expires remained current")
	}
}

type workspaceCheckpointCurrentStoreV2 struct{ *testkit.CheckpointMemoryStore }

func (s *workspaceCheckpointCurrentStoreV2) CreateCheckpointParticipant(_ context.Context, value contract.CheckpointParticipantFact) error {
	return s.SeedCheckpointParticipant(value)
}
func (*workspaceCheckpointCurrentStoreV2) CreateCheckpointPhaseDomainResultV2(context.Context, contract.CheckpointPhaseDomainResultV2) (bool, error) {
	return false, ports.ErrUnsupported
}
func (*workspaceCheckpointCurrentStoreV2) InspectCheckpointPhaseDomainResultV2(context.Context, contract.SnapshotArtifactExactRefV2) (contract.CheckpointPhaseDomainResultV2, error) {
	return contract.CheckpointPhaseDomainResultV2{}, ports.ErrUnsupported
}
func (*workspaceCheckpointCurrentStoreV2) InspectCheckpointPhaseDomainResultByIDV2(context.Context, string) (contract.CheckpointPhaseDomainResultV2, error) {
	return contract.CheckpointPhaseDomainResultV2{}, ports.ErrUnsupported
}
func (*workspaceCheckpointCurrentStoreV2) InspectCheckpointPhaseDomainResultByRefV2(context.Context, contract.Ref) (contract.CheckpointPhaseDomainResultV2, error) {
	return contract.CheckpointPhaseDomainResultV2{}, ports.ErrUnsupported
}
func (*workspaceCheckpointCurrentStoreV2) InspectCheckpointPhaseDomainResultByReservationV2(context.Context, contract.Ref) (contract.CheckpointPhaseDomainResultV2, error) {
	return contract.CheckpointPhaseDomainResultV2{}, ports.ErrUnsupported
}
func (*workspaceCheckpointCurrentStoreV2) CommitCheckpointPhaseApplySettlementV2(context.Context, contract.Ref, contract.CheckpointPhaseFact, contract.CheckpointParticipantFact) (bool, error) {
	return false, ports.ErrUnsupported
}
func (*workspaceCheckpointCurrentStoreV2) CreateCheckpointProviderResultBindingV2(context.Context, CheckpointProviderResultBindingV2) (CheckpointProviderResultBindingV2, error) {
	return CheckpointProviderResultBindingV2{}, ports.ErrUnsupported
}
func (*workspaceCheckpointCurrentStoreV2) InspectCheckpointProviderResultBindingV2(context.Context, contract.Ref) (CheckpointProviderResultBindingV2, error) {
	return CheckpointProviderResultBindingV2{}, ports.ErrUnsupported
}

type workspaceCheckpointCountingSnapshotStoreV2 struct {
	*testkit.SnapshotArtifactMemoryStore
	mu           sync.Mutex
	currentReads int
}

func (s *workspaceCheckpointCountingSnapshotStoreV2) InspectSnapshotArtifactCurrentIndex(ctx context.Context, aggregateID string) (contract.SnapshotArtifactAggregateCurrentIndexV2, error) {
	s.mu.Lock()
	s.currentReads++
	s.mu.Unlock()
	return s.SnapshotArtifactMemoryStore.InspectSnapshotArtifactCurrentIndex(ctx, aggregateID)
}

type workspaceCheckpointCaptureBindingStoreV2 struct {
	mu    sync.Mutex
	value CheckpointSnapshotCaptureBindingV2
	reads int
}

func (s *workspaceCheckpointCaptureBindingStoreV2) CreateCheckpointSnapshotCaptureBindingV2(_ context.Context, value CheckpointSnapshotCaptureBindingV2) (CheckpointSnapshotCaptureBindingV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.value.Digest != "" && s.value.Digest != value.Digest {
		return CheckpointSnapshotCaptureBindingV2{}, ports.ErrConflict
	}
	s.value = value.Clone()
	return s.value.Clone(), nil
}
func (s *workspaceCheckpointCaptureBindingStoreV2) InspectCheckpointSnapshotCaptureBindingV2(_ context.Context, expected contract.SnapshotArtifactExactRefV2) (CheckpointSnapshotCaptureBindingV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	if !contract.SameSnapshotArtifactExactRef(s.value.SnapshotReservation, expected) {
		return CheckpointSnapshotCaptureBindingV2{}, ports.ErrNotFound
	}
	return s.value.Clone(), nil
}
