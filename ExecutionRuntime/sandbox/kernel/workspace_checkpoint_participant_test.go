package kernel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceCheckpointParticipantOwnerCreatesExactPreparedBundle(t *testing.T) {
	now := time.Unix(1_900_500_000, 0).UTC()
	request := workspaceCheckpointPrepareRequestV2("happy", now)
	reader := &workspaceCheckpointCurrentReaderV2{now: now}
	store := testkit.NewWorkspaceCheckpointParticipantMemoryStoreV2()
	owner, err := NewWorkspaceCheckpointParticipantOwnerV2(store, reader, func() time.Time { return now }, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := owner.PrepareWorkspaceCheckpointParticipantV2(context.Background(), &request)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.ValidateShape() != nil || bundle.Participant.State != contract.WorkspaceCheckpointParticipantPrepared || len(bundle.Coverage.ResidualRefs) != 0 {
		t.Fatalf("workspace checkpoint was not exact prepared: %+v", bundle)
	}
	inspected, err := owner.InspectWorkspaceCheckpointPreparedV2(context.Background(), &contract.InspectWorkspaceCheckpointPreparedRequestV2{TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, CheckpointAttemptID: request.CheckpointAttemptRef.ID, ParticipantID: request.ParticipantID})
	if err != nil || !contract.SameSnapshotArtifactExactRef(inspected.Participant.ExactRef(), bundle.Participant.ExactRef()) {
		t.Fatalf("workspace checkpoint Inspect drifted: %+v err=%v", inspected, err)
	}
	inspected.Coverage.Included[0] = "mutated"
	again, _ := owner.InspectWorkspaceCheckpointPreparedV2(context.Background(), &contract.InspectWorkspaceCheckpointPreparedRequestV2{TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, CheckpointAttemptID: request.CheckpointAttemptRef.ID, ParticipantID: request.ParticipantID})
	if again.Coverage.Included[0] == "mutated" {
		t.Fatal("workspace checkpoint store leaked a mutable alias")
	}
}

func TestWorkspaceCheckpointParticipantOwnerRejectsPartialAndS2DriftWithZeroWrite(t *testing.T) {
	now := time.Unix(1_900_500_100, 0).UTC()
	for _, testCase := range []struct {
		name   string
		reader *workspaceCheckpointCurrentReaderV2
	}{
		{name: "partial", reader: &workspaceCheckpointCurrentReaderV2{now: now, residual: true}},
		{name: "s2_drift", reader: &workspaceCheckpointCurrentReaderV2{now: now, driftAt: 2}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := workspaceCheckpointPrepareRequestV2(testCase.name, now)
			store := testkit.NewWorkspaceCheckpointParticipantMemoryStoreV2()
			owner, err := NewWorkspaceCheckpointParticipantOwnerV2(store, testCase.reader, func() time.Time { return now }, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := owner.PrepareWorkspaceCheckpointParticipantV2(context.Background(), &request); err == nil {
				t.Fatal("invalid workspace checkpoint preparation was accepted")
			}
			if _, err := store.InspectWorkspaceCheckpointPreparedV2(context.Background(), contract.InspectWorkspaceCheckpointPreparedRequestV2{TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, CheckpointAttemptID: request.CheckpointAttemptRef.ID, ParticipantID: request.ParticipantID}); !errors.Is(err, ports.ErrNotFound) {
				t.Fatalf("invalid preparation wrote an Owner fact: %v", err)
			}
		})
	}
}

func TestWorkspaceCheckpointParticipantOwnerLostReplyInspectsOriginalWinner(t *testing.T) {
	now := time.Unix(1_900_500_200, 0).UTC()
	request := workspaceCheckpointPrepareRequestV2("lost", now)
	base := testkit.NewWorkspaceCheckpointParticipantMemoryStoreV2()
	store := &testkit.WorkspaceCheckpointLostReplyStoreV2{Base: base}
	store.LoseNextSuccessfulReply()
	owner, err := NewWorkspaceCheckpointParticipantOwnerV2(store, &workspaceCheckpointCurrentReaderV2{now: now}, func() time.Time { return now }, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	first, err := owner.PrepareWorkspaceCheckpointParticipantV2(context.Background(), &request)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := owner.PrepareWorkspaceCheckpointParticipantV2(context.Background(), &request)
	if err != nil || !contract.SameSnapshotArtifactExactRef(first.Participant.ExactRef(), replay.Participant.ExactRef()) {
		t.Fatalf("lost reply did not recover the original winner: %+v err=%v", replay, err)
	}
}

func TestWorkspaceCheckpointParticipantOwnerConcurrentDifferentContentHasOneWinner(t *testing.T) {
	now := time.Unix(1_900_500_300, 0).UTC()
	store := testkit.NewWorkspaceCheckpointParticipantMemoryStoreV2()
	var winners atomic.Int64
	var wait sync.WaitGroup
	for index := 0; index < 64; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			request := workspaceCheckpointPrepareRequestV2("concurrent", now)
			request.StableID = request.StableID + "-" + time.Unix(int64(index+1), 0).Format("150405")
			request.SnapshotArtifactFactRef.ID = request.SnapshotArtifactFactRef.ID + "-" + string(rune('a'+index%26))
			request.SnapshotArtifactFactRef.Digest = workspaceCheckpointDigestV2(request.SnapshotArtifactFactRef.ID)
			owner, err := NewWorkspaceCheckpointParticipantOwnerV2(store, &workspaceCheckpointCurrentReaderV2{now: now}, func() time.Time { return now }, time.Minute)
			if err == nil {
				_, err = owner.PrepareWorkspaceCheckpointParticipantV2(context.Background(), &request)
			}
			if err == nil {
				winners.Add(1)
			}
		}(index)
	}
	wait.Wait()
	if winners.Load() != 1 {
		t.Fatalf("workspace checkpoint concurrent CAS winners=%d, want 1", winners.Load())
	}
}

func TestWorkspaceCheckpointParticipantOwnerSameIDsRemainTenantAndScopeIsolated(t *testing.T) {
	now := time.Unix(1_900_500_400, 0).UTC()
	store := testkit.NewWorkspaceCheckpointParticipantMemoryStoreV2()
	owner, err := NewWorkspaceCheckpointParticipantOwnerV2(store, &workspaceCheckpointCurrentReaderV2{now: now}, func() time.Time { return now }, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	firstRequest := workspaceCheckpointPrepareRequestV2("shared-id", now)
	secondRequest := firstRequest
	secondRequest.TenantID = "tenant-other"
	secondRequest.ScopeDigest = workspaceCheckpointDigestV2("scope-other")
	secondRequest.RunID = "run-other"
	first, err := owner.PrepareWorkspaceCheckpointParticipantV2(context.Background(), &firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := owner.PrepareWorkspaceCheckpointParticipantV2(context.Background(), &secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	if first.Participant.Meta.ID != second.Participant.Meta.ID || first.Participant.TenantID == second.Participant.TenantID {
		t.Fatal("fixture did not exercise same ID across distinct tenants")
	}
	firstInspect := contract.InspectWorkspaceCheckpointFactRequestV2{TenantID: first.Participant.TenantID, ScopeDigest: first.Participant.ScopeDigest, ExpectedRef: first.Participant.ExactRef()}
	if got, err := store.InspectWorkspaceCheckpointParticipantV2(context.Background(), firstInspect); err != nil || got.TenantID != first.Participant.TenantID {
		t.Fatalf("first tenant history drifted: %+v err=%v", got, err)
	}
	splice := firstInspect
	splice.TenantID = second.Participant.TenantID
	splice.ScopeDigest = second.Participant.ScopeDigest
	if _, err := store.InspectWorkspaceCheckpointParticipantV2(context.Background(), splice); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("cross-tenant same-ID exact ref was not rejected: %v", err)
	}
}

type workspaceCheckpointCurrentReaderV2 struct {
	mu       sync.Mutex
	now      time.Time
	calls    int
	driftAt  int
	residual bool
}

func (r *workspaceCheckpointCurrentReaderV2) InspectWorkspaceCheckpointPreparationCurrentV2(_ context.Context, request contract.PrepareWorkspaceCheckpointParticipantRequestV2) (contract.WorkspaceCheckpointPreparationCurrentProjectionV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	included := []string{"workspace/metadata", "workspace/content"}
	if r.calls == r.driftAt {
		included = append(included, "workspace/drift")
	}
	residuals := []contract.Ref{}
	if r.residual {
		residuals = []contract.Ref{workspaceCheckpointRefV2("capture-residual")}
	}
	return contract.SealWorkspaceCheckpointPreparationCurrentProjectionV2(contract.WorkspaceCheckpointPreparationCurrentProjectionV2{
		TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, RunID: request.RunID, CheckpointAttemptRef: request.CheckpointAttemptRef, BarrierRef: request.BarrierRef, EffectCutRef: request.EffectCutRef,
		ParticipantID: request.ParticipantID, ParticipantDigest: request.ParticipantDigest, PreparedPhaseFactRef: request.PreparedPhaseFactRef, SnapshotArtifactFactRef: request.SnapshotArtifactFactRef,
		SnapshotAggregateRef: workspaceCheckpointAggregateRefKernelV2(request.TenantID, request.StableID, request.RequestedNotAfter), CoveragePolicyRef: request.CoveragePolicyRef,
		Included: included, DeclaredExcluded: []string{"device_state", "network_session", "process_state", "secret_material"}, ResidualRefs: residuals,
		CheckedUnixNano: r.now.UnixNano(), ExpiresUnixNano: r.now.Add(time.Minute).UnixNano(),
	}, r.now)
}

func workspaceCheckpointPrepareRequestV2(suffix string, now time.Time) contract.PrepareWorkspaceCheckpointParticipantRequestV2 {
	expires := now.Add(time.Minute).UnixNano()
	return contract.PrepareWorkspaceCheckpointParticipantRequestV2{
		StableID: "workspace-checkpoint-" + suffix, TenantID: "tenant-" + suffix, ScopeDigest: workspaceCheckpointDigestV2("scope-" + suffix), RunID: "run-" + suffix,
		CheckpointAttemptRef: workspaceCheckpointRefV2("attempt-" + suffix), BarrierRef: workspaceCheckpointRefV2("barrier-" + suffix), EffectCutRef: workspaceCheckpointRefV2("cut-" + suffix),
		ParticipantID: "participant-" + suffix, ParticipantDigest: workspaceCheckpointDigestV2("participant-" + suffix), PreparedPhaseFactRef: workspaceCheckpointRefV2("prepared-" + suffix),
		SnapshotArtifactFactRef: contract.SnapshotArtifactExactRefV2{TypeURL: contract.SnapshotArtifactFactTypeURL, Version: contract.SnapshotArtifactVersion, ID: "snapshot-artifact-" + suffix, Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: contract.SnapshotArtifactFactDomain, Digest: workspaceCheckpointDigestV2("snapshot-artifact-" + suffix), ExpiresUnixNano: expires},
		CoveragePolicyRef:       workspaceCheckpointRefV2("coverage-policy-" + suffix), RequestedNotAfter: expires,
	}
}

func workspaceCheckpointRefV2(value string) contract.Ref {
	return contract.Ref{ID: value, Revision: 1, Digest: workspaceCheckpointDigestV2(value)}
}

func workspaceCheckpointDigestV2(value string) string {
	digest, err := contract.Digest("workspace-checkpoint-kernel-test", value)
	if err != nil {
		panic(err)
	}
	return digest
}

func workspaceCheckpointAggregateRefKernelV2(tenantID, suffix string, expires int64) contract.SnapshotArtifactAggregateRefV2 {
	return contract.SnapshotArtifactAggregateRefV2{
		TypeURL: contract.SnapshotArtifactAggregateRefTypeURL, Version: contract.SnapshotArtifactVersion, Owner: contract.SnapshotArtifactOwnerV2, Kind: contract.SnapshotArtifactAggregateKind,
		AggregateID: "snapshot-aggregate-" + suffix, Revision: 2, TenantID: tenantID, DataDomain: contract.WorkspaceSnapshotDataDomain,
		SchemaRef: workspaceCheckpointRefV2("snapshot-schema-" + suffix), DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: contract.SnapshotArtifactAggregateRefDomain,
		Digest: workspaceCheckpointDigestV2("snapshot-aggregate-" + suffix), ExpiresUnixNano: expires,
	}
}
