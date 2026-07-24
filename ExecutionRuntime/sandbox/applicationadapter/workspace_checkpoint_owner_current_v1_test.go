package applicationadapter

import (
	"context"
	"errors"
	"testing"
	"time"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	sandboxkernel "github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceCheckpointOwnerCurrentAdapterPublishesOnlyExactOwnerCurrent(t *testing.T) {
	ctx := context.Background()
	work, _, _, clock := governedCheckpointFixtureV1(t, "workspace-current")
	now := clock()

	artifactStore := testkit.NewSnapshotArtifactMemoryStore()
	artifactReader := &testkit.SnapshotArtifactCommitCurrentReader{ReadFunc: func(_ int, request contract.CommitSnapshotArtifactRequestV2) (contract.SnapshotArtifactCommitCurrentProjectionV2, error) {
		return testkit.SnapshotArtifactCommitProjection(request, string(work.Gate.TenantID), contract.WorkspaceSnapshotDataDomain, now), nil
	}}
	artifactOwner, err := sandboxkernel.NewSnapshotArtifactOwnerWithCommitCurrent(artifactStore, artifactReader, clock, sandboxkernel.SnapshotArtifactOwnerLimits{MaxReservationTTL: time.Hour, MaxHistoryTTL: 2 * time.Hour, MaxProjectionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	reserve := testkit.SnapshotArtifactRequest("workspace-current")
	reserve.TenantID = string(work.Gate.TenantID)
	reserve.DataDomain = contract.WorkspaceSnapshotDataDomain
	reserve.RequestedNotAfter = work.NotAfter
	reserved, err := artifactOwner.ReserveArtifact(ctx, &reserve)
	if err != nil {
		t.Fatal(err)
	}
	reservedIndex, err := artifactStore.InspectSnapshotArtifactCurrentIndex(ctx, reserved.Reservation.SubjectRef.ArtifactAggregateID)
	if err != nil {
		t.Fatal(err)
	}
	commit := testkit.SnapshotArtifactCommitRequest(reserved.Reservation, reservedIndex, "workspace-current", now)
	commit.RequestedNotAfter = work.NotAfter
	available, err := artifactOwner.CommitArtifact(ctx, &commit)
	if err != nil {
		t.Fatal(err)
	}

	prepare := workspaceCheckpointPrepareFromWorkV1(work, available.Fact.ExactRef(), now)
	preparedReader := &workspaceCheckpointPreparationReaderAdapterTestV1{now: now, aggregate: available.CurrentIndex.HeadAggregateEnvelopeRef}
	preparedStore := testkit.NewWorkspaceCheckpointParticipantMemoryStoreV2()
	preparedOwner, err := sandboxkernel.NewWorkspaceCheckpointParticipantOwnerV2(preparedStore, preparedReader, clock, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := preparedOwner.PrepareWorkspaceCheckpointParticipantV2(ctx, &prepare)
	if err != nil {
		t.Fatal(err)
	}

	adapter, err := NewWorkspaceCheckpointOwnerCurrentAdapterV1(workspaceCheckpointOwnerCurrentConfigV1(work, preparedOwner, artifactOwner, clock))
	if err != nil {
		t.Fatal(err)
	}
	config := workspaceCheckpointOwnerCurrentConfigV1(work, preparedOwner, artifactOwner, clock)
	for name, ref := range map[string]appcontract.CheckpointExternalExactRefV1{
		"participant": workspaceCheckpointAppExactRefV1(bundle.Participant.ExactRef(), bundle.Participant, config.ParticipantOwner, config.ParticipantSchema, "workspace_checkpoint_participant_fact_v2"),
		"snapshot":    workspaceCheckpointAppExactRefV1(available.Fact.ExactRef(), bundle.Participant, config.SnapshotOwner, config.SnapshotSchema, "snapshot_artifact_fact_v2"),
		"coverage":    workspaceCheckpointAppExactRefV1(bundle.Coverage.ExactRef(), bundle.Participant, config.CoverageOwner, config.CoverageSchema, "workspace_checkpoint_coverage_fact_v2"),
	} {
		if err := ref.Validate(); err != nil {
			t.Fatalf("%s external exact ref: %v ref=%+v schema=%v owner=%v", name, err, ref, ref.Schema.Validate(), ref.Owner.Validate())
		}
	}
	candidate, err := adapter.InspectCheckpointParticipantOwnerCurrentV1(ctx, work)
	if err != nil {
		t.Fatal(err)
	}
	if err := candidate.Validate(work, now); err != nil {
		t.Fatal(err)
	}
	if candidate.ParticipantFact.ID != bundle.Participant.ExactRef().ID || candidate.Snapshot.ID != available.Fact.ExactRef().ID || candidate.Coverage.ID != bundle.Coverage.ExactRef().ID {
		t.Fatalf("Application candidate lost exact Owner refs: %+v", candidate)
	}
}

func TestWorkspaceCheckpointOwnerCurrentAdapterRejectsArtifactCurrentDrift(t *testing.T) {
	work, _, _, clock := governedCheckpointFixtureV1(t, "workspace-drift")
	prepared := &workspacePreparedOwnerStubV1{bundle: workspacePreparedBundleForWorkV1(t, work, clock())}
	fact := workspaceArtifactFactForBundleV1(t, prepared.bundle, clock())
	coverage := prepared.bundle.Coverage
	coverage.SnapshotArtifactFactRef = fact.ExactRef()
	var err error
	coverage, err = contract.SealWorkspaceCheckpointCoverageFactV2(coverage)
	if err != nil {
		t.Fatal(err)
	}
	participant := prepared.bundle.Participant
	participant.SnapshotArtifactFactRef = fact.ExactRef()
	participant.CoverageFactRef = coverage.ExactRef()
	participant, err = contract.SealWorkspaceCheckpointParticipantFactV2(participant)
	if err != nil {
		t.Fatal(err)
	}
	prepared.bundle = contract.WorkspaceCheckpointPreparedBundleV2{Coverage: coverage, Participant: participant}
	artifacts := &workspaceArtifactOwnerStubV1{fact: fact, currentErr: sandboxports.ErrStale}
	adapter, err := NewWorkspaceCheckpointOwnerCurrentAdapterV1(workspaceCheckpointOwnerCurrentConfigV1(work, prepared, artifacts, clock))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.InspectCheckpointParticipantOwnerCurrentV1(context.Background(), work); !errors.Is(err, sandboxports.ErrStale) {
		t.Fatalf("stale Snapshot Artifact current was accepted: %v", err)
	}
}

func TestWorkspaceCheckpointOwnerCurrentAdapterRejectsTypedNilOwner(t *testing.T) {
	work, _, _, clock := governedCheckpointFixtureV1(t, "workspace-typed-nil")
	var prepared *workspacePreparedOwnerStubV1
	if _, err := NewWorkspaceCheckpointOwnerCurrentAdapterV1(workspaceCheckpointOwnerCurrentConfigV1(work, prepared, &workspaceArtifactOwnerStubV1{}, clock)); err == nil {
		t.Fatal("typed-nil Workspace Checkpoint Owner was accepted")
	}
}

type workspaceCheckpointPreparationReaderAdapterTestV1 struct {
	now       time.Time
	aggregate contract.SnapshotArtifactAggregateRefV2
}

func (r *workspaceCheckpointPreparationReaderAdapterTestV1) InspectWorkspaceCheckpointPreparationCurrentV2(_ context.Context, request contract.PrepareWorkspaceCheckpointParticipantRequestV2) (contract.WorkspaceCheckpointPreparationCurrentProjectionV2, error) {
	return contract.SealWorkspaceCheckpointPreparationCurrentProjectionV2(contract.WorkspaceCheckpointPreparationCurrentProjectionV2{
		TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, RunID: request.RunID, CheckpointAttemptRef: request.CheckpointAttemptRef, BarrierRef: request.BarrierRef, EffectCutRef: request.EffectCutRef,
		ParticipantID: request.ParticipantID, ParticipantDigest: request.ParticipantDigest, PreparedPhaseFactRef: request.PreparedPhaseFactRef, SnapshotArtifactFactRef: request.SnapshotArtifactFactRef,
		SnapshotAggregateRef: r.aggregate, CoveragePolicyRef: request.CoveragePolicyRef, Included: []string{"workspace/content", "workspace/metadata"},
		DeclaredExcluded: []string{"device_state", "network_session", "process_state", "secret_material"}, ResidualRefs: []contract.Ref{}, CheckedUnixNano: r.now.UnixNano(), ExpiresUnixNano: request.RequestedNotAfter,
	}, r.now)
}

func workspaceCheckpointPrepareFromWorkV1(work appcontract.CheckpointParticipantWorkRequestV1, artifact contract.SnapshotArtifactExactRefV2, now time.Time) contract.PrepareWorkspaceCheckpointParticipantRequestV2 {
	return contract.PrepareWorkspaceCheckpointParticipantRequestV2{
		StableID: "workspace-" + work.Attempt.ID + "-" + work.Participant.ID, TenantID: string(work.Gate.TenantID), ScopeDigest: string(work.Gate.ScopeDigest), RunID: string(work.Gate.RunID),
		CheckpointAttemptRef: runtimeCheckpointRefToSandboxV1(work.Attempt.ID, uint64(work.Attempt.Revision), string(work.Attempt.Digest)),
		BarrierRef:           runtimeCheckpointRefToSandboxV1(work.Barrier.ID, uint64(work.Barrier.Revision), string(work.Barrier.Digest)), EffectCutRef: runtimeCheckpointRefToSandboxV1(work.EffectCut.ID, uint64(work.EffectCut.Revision), string(work.EffectCut.Digest)),
		ParticipantID: work.Participant.ID, ParticipantDigest: string(work.Participant.Digest), PreparedPhaseFactRef: testkit.Ref("prepared-phase-" + work.Participant.ID), SnapshotArtifactFactRef: artifact,
		CoveragePolicyRef: testkit.Ref("workspace-coverage-policy-v1"), RequestedNotAfter: work.NotAfter,
	}
}

func runtimeCheckpointRefToSandboxV1(id string, revision uint64, digest string) contract.Ref {
	return contract.Ref{ID: id, Revision: revision, Digest: digest}
}

func workspaceCheckpointOwnerCurrentConfigV1(work appcontract.CheckpointParticipantWorkRequestV1, prepared sandboxports.WorkspaceCheckpointParticipantOwnerPortV2, artifacts sandboxports.SnapshotArtifactOwnerPortV2, clock func() time.Time) WorkspaceCheckpointOwnerCurrentAdapterConfigV1 {
	return WorkspaceCheckpointOwnerCurrentAdapterConfigV1{
		Prepared: prepared, Artifacts: artifacts, ParticipantOwner: work.Participant.Owner, SnapshotOwner: work.Participant.Owner, CoverageOwner: work.Participant.Owner,
		ParticipantSchema: workspaceCheckpointSchemaV1("workspace-checkpoint-participant-fact-v2"), SnapshotSchema: workspaceCheckpointSchemaV1("snapshot-artifact-fact-v2"), CoverageSchema: workspaceCheckpointSchemaV1("workspace-checkpoint-coverage-fact-v2"), Clock: clock,
	}
}

func workspaceCheckpointSchemaV1(name string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.sandbox", Name: name, Version: "2.0.0", MediaType: "application/json", ContentDigest: checkpointDigestV1("schema-" + name)}
}

type workspacePreparedOwnerStubV1 struct {
	bundle contract.WorkspaceCheckpointPreparedBundleV2
}

func (s *workspacePreparedOwnerStubV1) PrepareWorkspaceCheckpointParticipantV2(context.Context, *contract.PrepareWorkspaceCheckpointParticipantRequestV2) (contract.WorkspaceCheckpointPreparedBundleV2, error) {
	return contract.WorkspaceCheckpointPreparedBundleV2{}, errors.New("not used")
}
func (s *workspacePreparedOwnerStubV1) InspectWorkspaceCheckpointPreparedV2(context.Context, *contract.InspectWorkspaceCheckpointPreparedRequestV2) (contract.WorkspaceCheckpointPreparedBundleV2, error) {
	return s.bundle.Clone(), nil
}

type workspaceArtifactOwnerStubV1 struct {
	fact       contract.SnapshotArtifactFactV2
	currentErr error
}

func (*workspaceArtifactOwnerStubV1) ReserveArtifact(context.Context, *contract.ReserveArtifactRequestV2) (contract.ReserveArtifactResultV2, error) {
	return contract.ReserveArtifactResultV2{}, errors.New("not used")
}
func (*workspaceArtifactOwnerStubV1) CommitArtifact(context.Context, *contract.CommitSnapshotArtifactRequestV2) (contract.CommitSnapshotArtifactResultV2, error) {
	return contract.CommitSnapshotArtifactResultV2{}, errors.New("not used")
}
func (*workspaceArtifactOwnerStubV1) InspectReservation(context.Context, *contract.InspectSnapshotArtifactReservationRequestV2) (contract.SnapshotArtifactReservationV2, error) {
	return contract.SnapshotArtifactReservationV2{}, errors.New("not used")
}
func (*workspaceArtifactOwnerStubV1) InspectReservationByStableKey(context.Context, *contract.InspectSnapshotArtifactReservationByStableKeyRequestV2) (contract.SnapshotArtifactReservationV2, error) {
	return contract.SnapshotArtifactReservationV2{}, errors.New("not used")
}
func (*workspaceArtifactOwnerStubV1) InspectAggregateHistorical(context.Context, *contract.InspectSnapshotArtifactAggregateHistoricalRequestV2) (contract.SnapshotArtifactAggregateEnvelopeV2, error) {
	return contract.SnapshotArtifactAggregateEnvelopeV2{}, errors.New("not used")
}
func (s *workspaceArtifactOwnerStubV1) InspectAggregateCurrent(context.Context, *contract.InspectSnapshotArtifactAggregateCurrentRequestV2) (contract.SnapshotArtifactAggregateCurrentProjectionV2, error) {
	return contract.SnapshotArtifactAggregateCurrentProjectionV2{}, s.currentErr
}
func (*workspaceArtifactOwnerStubV1) InspectEntryHistorical(context.Context, *contract.InspectSnapshotArtifactEntryHistoricalRequestV2) (contract.SnapshotArtifactAggregateEntryV2, error) {
	return contract.SnapshotArtifactAggregateEntryV2{}, errors.New("not used")
}
func (s *workspaceArtifactOwnerStubV1) InspectArtifactFact(context.Context, *contract.InspectSnapshotArtifactFactRequestV2) (contract.SnapshotArtifactFactV2, error) {
	return s.fact, nil
}

func workspacePreparedBundleForWorkV1(t *testing.T, work appcontract.CheckpointParticipantWorkRequestV1, now time.Time) contract.WorkspaceCheckpointPreparedBundleV2 {
	t.Helper()
	artifact := contract.SnapshotArtifactExactRefV2{TypeURL: contract.SnapshotArtifactFactTypeURL, Version: contract.SnapshotArtifactVersion, ID: "artifact-" + work.Attempt.ID, Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: contract.SnapshotArtifactFactDomain, Digest: string(checkpointDigestV1("artifact-" + work.Attempt.ID)), ExpiresUnixNano: work.NotAfter}
	aggregate := contract.SnapshotArtifactAggregateRefV2{TypeURL: contract.SnapshotArtifactAggregateRefTypeURL, Version: contract.SnapshotArtifactVersion, Owner: contract.SnapshotArtifactOwnerV2, Kind: contract.SnapshotArtifactAggregateKind, AggregateID: "aggregate-" + work.Attempt.ID, Revision: 2, TenantID: string(work.Gate.TenantID), DataDomain: contract.WorkspaceSnapshotDataDomain, SchemaRef: testkit.Ref("snapshot-schema"), DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: contract.SnapshotArtifactAggregateRefDomain, Digest: string(checkpointDigestV1("aggregate-" + work.Attempt.ID)), ExpiresUnixNano: work.NotAfter}
	reader := &workspaceCheckpointPreparationReaderAdapterTestV1{now: now, aggregate: aggregate}
	request := workspaceCheckpointPrepareFromWorkV1(work, artifact, now)
	projection, err := reader.InspectWorkspaceCheckpointPreparationCurrentV2(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := contract.SealWorkspaceCheckpointCoverageFactV2(contract.WorkspaceCheckpointCoverageFactV2{Meta: contract.Meta{ContractVersion: contract.ContractFamily, ID: request.StableID + "-coverage", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: work.NotAfter}, TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, RunID: request.RunID, CheckpointAttemptRef: request.CheckpointAttemptRef, BarrierRef: request.BarrierRef, EffectCutRef: request.EffectCutRef, ParticipantID: request.ParticipantID, ParticipantDigest: request.ParticipantDigest, PreparedPhaseFactRef: request.PreparedPhaseFactRef, SnapshotArtifactFactRef: artifact, SnapshotAggregateRef: aggregate, CoveragePolicyRef: request.CoveragePolicyRef, Included: projection.Included, DeclaredExcluded: projection.DeclaredExcluded, ResidualRefs: []contract.Ref{}, State: contract.WorkspaceCheckpointCoverageComplete, RequestedNotAfter: work.NotAfter})
	if err != nil {
		t.Fatal(err)
	}
	participant, err := contract.SealWorkspaceCheckpointParticipantFactV2(contract.WorkspaceCheckpointParticipantFactV2{Meta: contract.Meta{ContractVersion: contract.ContractFamily, ID: request.StableID + "-participant", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: work.NotAfter}, TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, RunID: request.RunID, CheckpointAttemptRef: request.CheckpointAttemptRef, BarrierRef: request.BarrierRef, EffectCutRef: request.EffectCutRef, ParticipantID: request.ParticipantID, ParticipantDigest: request.ParticipantDigest, PreparedPhaseFactRef: request.PreparedPhaseFactRef, SnapshotArtifactFactRef: artifact, SnapshotAggregateRef: aggregate, CoverageFactRef: coverage.ExactRef(), State: contract.WorkspaceCheckpointParticipantPrepared, RequestedNotAfter: work.NotAfter})
	if err != nil {
		t.Fatal(err)
	}
	return contract.WorkspaceCheckpointPreparedBundleV2{Coverage: coverage, Participant: participant}
}

func workspaceArtifactFactForBundleV1(t *testing.T, bundle contract.WorkspaceCheckpointPreparedBundleV2, now time.Time) contract.SnapshotArtifactFactV2 {
	t.Helper()
	identity, err := contract.SealSnapshotArtifactSubjectIdentityV2(contract.SnapshotArtifactSubjectIdentityV2{ArtifactAggregateID: bundle.Participant.SnapshotAggregateRef.AggregateID, TenantID: bundle.Participant.TenantID, DataDomain: contract.WorkspaceSnapshotDataDomain, ReservationID: "reservation", SourceAttemptID: "attempt"})
	if err != nil {
		t.Fatal(err)
	}
	subject, err := contract.SealSnapshotArtifactSubjectRefV2(contract.SnapshotArtifactSubjectRefV2{ArtifactAggregateID: identity.ArtifactAggregateID, Revision: 1, TenantID: identity.TenantID, DataDomain: identity.DataDomain, ReservationID: identity.ReservationID, SourceAttemptID: identity.SourceAttemptID, SchemaRef: testkit.Ref("snapshot-schema"), StableSubjectDigest: identity.StableSubjectDigest, ExpiresUnixNano: bundle.Participant.Meta.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	storage, err := contract.SealSnapshotStorageArtifactRefV2(contract.SnapshotStorageArtifactRefV2{StorageArtifactID: "storage-" + bundle.Participant.Meta.ID, Revision: 1, TenantID: bundle.Participant.TenantID, DataDomain: contract.WorkspaceSnapshotDataDomain, StorageNamespaceExactRef: contract.SnapshotArtifactExactRefV2{TypeURL: "praxis.sandbox/host-local-namespace/v1", Version: 1, ID: "namespace", Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: "praxis.sandbox/host-local-namespace/body/v1", Digest: string(checkpointDigestV1("namespace")), ExpiresUnixNano: bundle.Participant.Meta.ExpiresUnixNano}, ContentDigest: string(checkpointDigestV1("content")), SchemaRef: testkit.Ref("snapshot-schema"), Length: 1, EncryptionFactRef: testkit.Ref("encryption"), ResidencyFactRef: testkit.Ref("residency"), CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: bundle.Participant.Meta.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := contract.SealSnapshotArtifactFactV2(contract.SnapshotArtifactFactV2{Meta: contract.Meta{ContractVersion: contract.ContractFamily, ID: bundle.Participant.SnapshotArtifactFactRef.ID, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: bundle.Participant.Meta.ExpiresUnixNano}, TenantID: bundle.Participant.TenantID, DataDomain: contract.WorkspaceSnapshotDataDomain, ReservationFactRef: contract.SnapshotArtifactExactRefV2{TypeURL: contract.SnapshotArtifactReservationFactTypeURL, Version: contract.SnapshotArtifactVersion, ID: "reservation", Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: contract.SnapshotArtifactReservationFactDomain, Digest: string(checkpointDigestV1("reservation")), ExpiresUnixNano: bundle.Participant.Meta.ExpiresUnixNano}, ArtifactSubjectRef: subject, StorageArtifactRef: storage, SchemaRef: storage.SchemaRef, ContentDigest: storage.ContentDigest, Length: storage.Length, EncryptionFactRef: storage.EncryptionFactRef, ResidencyFactRef: storage.ResidencyFactRef, ProviderObservationRef: testkit.Ref("observation"), ProviderReceiptRef: testkit.Ref("receipt"), FormalEvidenceRefs: []contract.Ref{testkit.Ref("evidence")}, OwnerInspectionRef: testkit.Ref("inspection"), SourceAttemptRef: testkit.Ref("attempt"), RequestedNotAfter: bundle.Participant.Meta.ExpiresUnixNano, State: contract.SnapshotArtifactAvailable})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}
