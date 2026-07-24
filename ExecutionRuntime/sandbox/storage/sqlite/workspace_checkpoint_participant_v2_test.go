package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceCheckpointParticipantSQLiteCreateOnceExactInspectAndConflictV2(t *testing.T) {
	ctx := context.Background()
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	reader := &workspaceCheckpointSQLiteCurrentV2{now: testkit.FixedNow}
	owner, err := kernel.NewWorkspaceCheckpointParticipantOwnerV2(store, reader, func() time.Time { return testkit.FixedNow }, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request := workspaceCheckpointSQLiteRequestV2("durable")
	first, err := owner.PrepareWorkspaceCheckpointParticipantV2(ctx, &request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := owner.PrepareWorkspaceCheckpointParticipantV2(ctx, &request)
	if err != nil || !contract.SameSnapshotArtifactExactRef(first.Participant.ExactRef(), second.Participant.ExactRef()) || !contract.SameSnapshotArtifactExactRef(first.Coverage.ExactRef(), second.Coverage.ExactRef()) {
		t.Fatalf("workspace checkpoint exact replay failed: %+v err=%v", second, err)
	}
	participant, err := store.InspectWorkspaceCheckpointParticipantV2(ctx, contract.InspectWorkspaceCheckpointFactRequestV2{TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, ExpectedRef: first.Participant.ExactRef()})
	if err != nil || !contract.SameSnapshotArtifactExactRef(participant.ExactRef(), first.Participant.ExactRef()) {
		t.Fatalf("participant historical Inspect failed: %+v err=%v", participant, err)
	}
	coverage, err := store.InspectWorkspaceCheckpointCoverageV2(ctx, contract.InspectWorkspaceCheckpointFactRequestV2{TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, ExpectedRef: first.Coverage.ExactRef()})
	if err != nil || !contract.SameSnapshotArtifactExactRef(coverage.ExactRef(), first.Coverage.ExactRef()) {
		t.Fatalf("coverage historical Inspect failed: %+v err=%v", coverage, err)
	}
	drift := request
	drift.CoveragePolicyRef = testkit.Ref("another-coverage-policy")
	if _, err := owner.PrepareWorkspaceCheckpointParticipantV2(ctx, &drift); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("workspace checkpoint content drift did not conflict: %v", err)
	}
}

type workspaceCheckpointSQLiteCurrentV2 struct{ now time.Time }

func (r *workspaceCheckpointSQLiteCurrentV2) InspectWorkspaceCheckpointPreparationCurrentV2(_ context.Context, request contract.PrepareWorkspaceCheckpointParticipantRequestV2) (contract.WorkspaceCheckpointPreparationCurrentProjectionV2, error) {
	aggregate := contract.SnapshotArtifactAggregateRefV2{TypeURL: contract.SnapshotArtifactAggregateRefTypeURL, Version: contract.SnapshotArtifactVersion, Owner: contract.SnapshotArtifactOwnerV2, Kind: contract.SnapshotArtifactAggregateKind, AggregateID: "aggregate-" + request.StableID, Revision: 2, TenantID: request.TenantID, DataDomain: contract.WorkspaceSnapshotDataDomain, SchemaRef: testkit.Ref("workspace-schema"), DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: contract.SnapshotArtifactAggregateRefDomain, Digest: testkit.Ref("aggregate-" + request.StableID).Digest, ExpiresUnixNano: request.RequestedNotAfter}
	return contract.SealWorkspaceCheckpointPreparationCurrentProjectionV2(contract.WorkspaceCheckpointPreparationCurrentProjectionV2{
		TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, RunID: request.RunID, CheckpointAttemptRef: request.CheckpointAttemptRef, BarrierRef: request.BarrierRef, EffectCutRef: request.EffectCutRef,
		ParticipantID: request.ParticipantID, ParticipantDigest: request.ParticipantDigest, PreparedPhaseFactRef: request.PreparedPhaseFactRef, SnapshotArtifactFactRef: request.SnapshotArtifactFactRef,
		SnapshotAggregateRef: aggregate, CoveragePolicyRef: request.CoveragePolicyRef, Included: []string{"workspace/content", "workspace/metadata"}, DeclaredExcluded: []string{"device_state"}, ResidualRefs: []contract.Ref{},
		CheckedUnixNano: r.now.UnixNano(), ExpiresUnixNano: request.RequestedNotAfter,
	}, r.now)
}

func workspaceCheckpointSQLiteRequestV2(suffix string) contract.PrepareWorkspaceCheckpointParticipantRequestV2 {
	expires := testkit.FixedNow.Add(time.Hour).UnixNano()
	ref := func(id string) contract.Ref { return testkit.Ref(id + "-" + suffix) }
	return contract.PrepareWorkspaceCheckpointParticipantRequestV2{
		StableID: "workspace-checkpoint-" + suffix, TenantID: "tenant-" + suffix, ScopeDigest: testkit.Ref("scope-" + suffix).Digest, RunID: "run-" + suffix,
		CheckpointAttemptRef: ref("attempt"), BarrierRef: ref("barrier"), EffectCutRef: ref("effect-cut"), ParticipantID: "participant-" + suffix, ParticipantDigest: testkit.Ref("participant-" + suffix).Digest,
		PreparedPhaseFactRef: ref("prepared-phase"), SnapshotArtifactFactRef: contract.SnapshotArtifactExactRefV2{TypeURL: contract.SnapshotArtifactFactTypeURL, Version: contract.SnapshotArtifactVersion, ID: "artifact-" + suffix, Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: contract.SnapshotArtifactFactDomain, Digest: testkit.Ref("artifact-" + suffix).Digest, ExpiresUnixNano: expires},
		CoveragePolicyRef: ref("coverage-policy"), RequestedNotAfter: expires,
	}
}
