package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/applicationadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestCheckpointSnapshotCaptureBindingSQLiteCreateOnceAndConflictV2(t *testing.T) {
	ctx := context.Background()
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return testkit.FixedNow })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, err := kernel.NewSnapshotArtifactOwner(store, func() time.Time { return testkit.FixedNow }, kernel.SnapshotArtifactOwnerLimits{MaxReservationTTL: 90 * time.Minute, MaxHistoryTTL: 3 * time.Hour, MaxProjectionTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	request := testkit.SnapshotArtifactRequest("checkpoint-capture-sqlite")
	reserved, err := owner.ReserveArtifact(ctx, &request)
	if err != nil {
		t.Fatal(err)
	}
	commit := testkit.SnapshotArtifactCommitRequest(reserved.Reservation, reserved.CurrentIndex, "checkpoint-capture-sqlite", testkit.FixedNow)
	expires := commit.RequestedNotAfter
	if reserved.Reservation.ExactRef().ExpiresUnixNano < expires {
		expires = reserved.Reservation.ExactRef().ExpiresUnixNano
	}
	if commit.StorageArtifactRef.ExpiresUnixNano < expires {
		expires = commit.StorageArtifactRef.ExpiresUnixNano
	}
	providerSubject := testkit.Ref("checkpoint-capture-provider-subject").Digest
	binding, err := applicationadapter.SealCheckpointSnapshotCaptureBindingV2(applicationadapter.CheckpointSnapshotCaptureBindingV2{
		SnapshotReservation: reserved.Reservation.ExactRef(), CheckpointReservation: testkit.Ref("checkpoint-phase-reservation"),
		StorageArtifactRef: commit.StorageArtifactRef, ProviderObservationRef: commit.ProviderObservationRef,
		ProviderArtifact:             contract.CheckpointWorkspaceArtifactObservationV2{Provider: "host_workspace", ArtifactID: "praxis-checkpoint:" + providerSubject, SubjectDigest: "sha256:" + providerSubject, ContentDigest: "sha256:" + commit.StorageArtifactRef.ContentDigest, ContentLength: commit.StorageArtifactRef.Length, State: "prepared", CheckpointPhase: "checkpoint_prepare", RecordedUnixNano: testkit.FixedNow.UnixNano(), ExpiresUnixNano: expires},
		MaterializationInspectionRef: testkit.Ref("checkpoint-capture-materialization"), WorkspaceBundleDigest: testkit.Ref("checkpoint-capture-bundle").Digest,
		ProviderReceiptRef: commit.ProviderReceiptRef, EvidenceConsumptionRef: commit.FormalEvidenceRefs[0],
		OwnerInspectionRef: commit.OwnerInspectionRef, SourceAttemptRef: commit.SourceAttemptRef,
		TenantID: request.TenantID, ScopeDigest: testkit.Ref("checkpoint-scope").Digest, RunID: "checkpoint-run", CheckpointAttemptRef: commit.SourceAttemptRef,
		BarrierRef: testkit.Ref("checkpoint-barrier"), EffectCutRef: testkit.Ref("checkpoint-effect-cut"), ParticipantID: "checkpoint-participant", ParticipantDigest: testkit.Ref("checkpoint-participant").Digest,
		WorkspaceStableID: "checkpoint-capture-workspace", CoveragePolicyRef: testkit.Ref("checkpoint-coverage-policy"),
		Included: []string{"workspace/content", "workspace/metadata"}, DeclaredExcluded: []string{"device_state"}, ResidualRefs: []contract.Ref{},
		RequestedNotAfter: commit.RequestedNotAfter, ExpiresUnixNano: expires,
	}, testkit.FixedNow)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CreateCheckpointSnapshotCaptureBindingV2(ctx, binding)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateCheckpointSnapshotCaptureBindingV2(ctx, binding)
	if err != nil || !reflect.DeepEqual(second, first) {
		t.Fatalf("exact checkpoint Snapshot binding replay failed: second=%+v err=%v", second, err)
	}
	inspected, err := store.InspectCheckpointSnapshotCaptureBindingV2(ctx, binding.SnapshotReservation)
	if err != nil || !reflect.DeepEqual(inspected, binding) {
		t.Fatalf("checkpoint Snapshot binding history drifted: inspected=%+v err=%v", inspected, err)
	}
	drift := binding
	drift.ProviderReceiptRef = testkit.Ref("another-provider-receipt")
	drift, err = applicationadapter.SealCheckpointSnapshotCaptureBindingV2(drift, testkit.FixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateCheckpointSnapshotCaptureBindingV2(ctx, drift); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("different checkpoint Snapshot binding winner was not rejected: %v", err)
	}
}
