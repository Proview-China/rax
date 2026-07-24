package sqlite

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestRestoreStageCoordinatesV1DurableCreateOnceConflictAndTenantIsolation(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_980_000_000, 0)
	path := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := OpenWithClock(ctx, path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	request, coordinates := restoreStageCoordinateFixtureV1(t, now)
	if err := store.PutRestoreStageCoordinatesV1(ctx, request, coordinates); err != nil {
		t.Fatal(err)
	}
	if err := store.PutRestoreStageCoordinatesV1(ctx, request, coordinates); err != nil {
		t.Fatalf("exact replay: %v", err)
	}
	drift := coordinates
	drift.SnapshotArtifact.ID = "changed-snapshot"
	drift.SnapshotArtifact.Digest = string(runtimecore.DigestBytes([]byte("changed-snapshot")))
	if err := store.PutRestoreStageCoordinatesV1(ctx, request, drift); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("changed coordinates err=%v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithClock(ctx, path, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.ReadRestoreStageCoordinatesV1(ctx, request)
	if err != nil || !reflect.DeepEqual(got, coordinates) {
		t.Fatalf("durable coordinates=%+v err=%v", got, err)
	}
	otherTenant := request
	otherTenant.TenantID = "tenant-other"
	otherTenant.Target.TenantID = otherTenant.TenantID
	if _, err := reopened.ReadRestoreStageCoordinatesV1(ctx, otherTenant); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("cross-tenant read err=%v", err)
	}
}

func TestRestoreStageCoordinatesV1ConcurrentChangedContentHasSingleWinner(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_980_000_000, 0)
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	request, base := restoreStageCoordinateFixtureV1(t, now)
	const workers = 64
	var wait sync.WaitGroup
	wait.Add(workers)
	winners := make(chan runtimeports.InspectRestoreStageGovernanceCurrentRequestV1, workers)
	errorsSeen := make(chan error, workers)
	for index := range workers {
		go func() {
			defer wait.Done()
			candidate := base
			candidate.SnapshotArtifact.ID = fmt.Sprintf("snapshot-%02d", index)
			candidate.SnapshotArtifact.Digest = string(runtimecore.DigestBytes([]byte(candidate.SnapshotArtifact.ID)))
			if putErr := store.PutRestoreStageCoordinatesV1(ctx, request, candidate); putErr == nil {
				winners <- candidate
			} else {
				errorsSeen <- putErr
			}
		}()
	}
	wait.Wait()
	close(winners)
	close(errorsSeen)
	var winner runtimeports.InspectRestoreStageGovernanceCurrentRequestV1
	count := 0
	for value := range winners {
		winner = value
		count++
	}
	if count != 1 {
		t.Fatalf("changed-content winners=%d", count)
	}
	for putErr := range errorsSeen {
		if !errors.Is(putErr, ports.ErrConflict) {
			t.Fatalf("loser error=%v", putErr)
		}
	}
	stored, err := store.ReadRestoreStageCoordinatesV1(ctx, request)
	if err != nil || !reflect.DeepEqual(stored, winner) {
		t.Fatalf("stored winner drifted: err=%v", err)
	}
}

func restoreStageCoordinateFixtureV1(t *testing.T, now time.Time) (contract.WorkspaceRestoreStageRequestV1, runtimeports.InspectRestoreStageGovernanceCurrentRequestV1) {
	t.Helper()
	governance, _, _, _, err := runtimefakes.BuildRestoreStageSettlementFixtureV1("coordinate", now)
	if err != nil {
		t.Fatal(err)
	}
	expires := governance.ExpiresUnixNano
	exact := func(kind, domain, id string, revision uint64, digest string) contract.SnapshotArtifactExactRefV2 {
		return contract.SnapshotArtifactExactRefV2{TypeURL: kind, Version: 2, ID: id, Revision: revision, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: domain, Digest: digest[7:], ExpiresUnixNano: expires}
	}
	request := contract.WorkspaceRestoreStageRequestV1{TenantID: string(governance.RestoreAttempt.TenantID), DispatchAttemptID: governance.DispatchAttempt.AttemptID, RuntimeRestoreAttempt: exact("praxis.runtime/restore-attempt/v2", "praxis.runtime/restore-attempt/body/v2", governance.RestoreAttempt.ID, uint64(governance.RestoreAttempt.Revision), string(governance.RestoreAttempt.Digest)), RestoreEligibility: exact("praxis.runtime/restore-eligibility/v2", "praxis.runtime/restore-eligibility/body/v2", governance.Eligibility.ID, uint64(governance.Eligibility.Revision), string(governance.Eligibility.Digest)), Target: contract.RuntimeLeaseBinding{TenantID: string(governance.RestoreAttempt.TenantID), InstanceID: string(governance.Identity.TargetInstance.ID), InstanceEpoch: uint64(governance.Identity.TargetInstance.Epoch), LeaseID: string(governance.Identity.TargetLease.ID), LeaseEpoch: uint64(governance.Identity.TargetLease.Epoch), FenceEpoch: uint64(governance.Identity.TargetFenceEpoch), ScopeDigest: string(governance.Operation.ExecutionScopeDigest), ObservedRevision: uint64(governance.Operation.CurrentProjectionRevision), ExpiresUnixNano: expires}, SnapshotArtifactFactRef: exact(contract.SnapshotArtifactFactTypeURL, contract.SnapshotArtifactFactDomain, governance.SnapshotArtifact.ID, uint64(governance.SnapshotArtifact.Revision), governance.SnapshotArtifact.Digest), RequestedNotAfter: expires}
	coordinates := runtimeports.InspectRestoreStageGovernanceCurrentRequestV1{RestoreAttempt: governance.RestoreAttempt, Eligibility: governance.Eligibility, Operation: governance.Operation, EffectID: governance.EffectID, Admission: governance.Admission, Authorization: governance.Authorization, PermitID: governance.PermitID, DispatchAttempt: governance.DispatchAttempt, ExecuteEnforcement: governance.ExecuteEnforcement, SnapshotArtifact: governance.SnapshotArtifact}
	return request, coordinates
}
