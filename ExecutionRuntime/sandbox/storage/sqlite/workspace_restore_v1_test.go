package sqlite

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceRestoreStoreV1HistoryCASAndNoABA(t *testing.T) {
	ctx := context.Background()
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	prepared := workspaceRestorePreparedFixtureSQLiteV1(t, "tenant-1")
	created, err := store.CreateWorkspaceRestoreAttemptV1(ctx, prepared)
	if err != nil || !created {
		t.Fatalf("create=%v err=%v", created, err)
	}
	if created, err := store.CreateWorkspaceRestoreAttemptV1(ctx, prepared); err != nil || created {
		t.Fatalf("exact replay=%v err=%v", created, err)
	}

	drift := prepared.Clone()
	drift.Meta.Revision++
	drift.Meta.UpdatedUnixNano++
	drift.BundleDigest = strings.Repeat("9", contract.DigestSizeHex)
	drift, _ = contract.SealWorkspaceRestoreAttemptV1(drift)
	if _, err := store.CASWorkspaceRestoreAttemptV1(ctx, prepared.ExactRef(), drift); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("direct repository immutable drift was accepted: %v", err)
	}

	invocation := workspaceRestoreInvocationFixtureSQLiteV1(t, ctx, store, prepared)
	next, fact := workspaceRestoreFinalFixtureSQLiteV1(t, invocation, "winner")
	created, err = store.CommitWorkspaceRestoreStageV1(ctx, invocation.ExactRef(), next, fact)
	if err != nil || !created {
		t.Fatalf("commit=%v err=%v", created, err)
	}
	historical, err := store.InspectWorkspaceRestoreAttemptV1(ctx, prepared.ExactRef())
	if err != nil || historical.ExactRef() != prepared.ExactRef() {
		t.Fatalf("historical=%#v err=%v", historical, err)
	}
	current, err := store.InspectWorkspaceRestoreAttemptByStableKeyV1(ctx, prepared.StableKeyDigest)
	if err != nil || current.ExactRef() != next.ExactRef() {
		t.Fatalf("current=%#v err=%v", current, err)
	}
	if _, err := store.CASWorkspaceRestoreAttemptV1(ctx, prepared.ExactRef(), drift); !errors.Is(err, ports.ErrConflict) {
		t.Fatalf("ABA from historical revision was accepted: %v", err)
	}
}

func TestWorkspaceRestoreStoreV1ConcurrentDifferentFinalSingleWinner(t *testing.T) {
	ctx := context.Background()
	store, err := OpenWithClock(ctx, filepath.Join(t.TempDir(), "sandbox.db"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	prepared := workspaceRestorePreparedFixtureSQLiteV1(t, "tenant-1")
	if _, err := store.CreateWorkspaceRestoreAttemptV1(ctx, prepared); err != nil {
		t.Fatal(err)
	}
	invocation := workspaceRestoreInvocationFixtureSQLiteV1(t, ctx, store, prepared)
	var winners atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			next, fact := workspaceRestoreFinalFixtureSQLiteV1(t, invocation, fmt.Sprintf("candidate-%02d", index))
			created, err := store.CommitWorkspaceRestoreStageV1(ctx, invocation.ExactRef(), next, fact)
			if err == nil && created {
				winners.Add(1)
				return
			}
			if err != nil && !errors.Is(err, ports.ErrConflict) {
				t.Errorf("candidate %d: %v", index, err)
			}
		}(i)
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("winners=%d want=1", winners.Load())
	}
}

func workspaceRestorePreparedFixtureSQLiteV1(t *testing.T, tenant string) contract.WorkspaceRestoreAttemptV1 {
	t.Helper()
	now := time.Unix(1_950_000_000, 0)
	ref := func(typeURL, domain, id string) contract.SnapshotArtifactExactRefV2 {
		digest, _ := contract.Digest("workspace-restore-sqlite-test-ref-v1", struct{ Tenant, ID string }{tenant, id})
		return contract.SnapshotArtifactExactRefV2{TypeURL: typeURL, Version: 1, ID: id, Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: domain, Digest: digest, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	}
	request := contract.WorkspaceRestoreStageRequestV1{
		TenantID: tenant, DispatchAttemptID: "dispatch-attempt", RuntimeRestoreAttempt: ref("praxis.runtime/restore-attempt/v2", "runtime-attempt", "runtime-attempt"), RestoreEligibility: ref("praxis.runtime/restore-eligibility/v2", "eligibility", "eligibility"),
		Target:                  contract.RuntimeLeaseBinding{TenantID: tenant, InstanceID: "new-instance", InstanceEpoch: 2, LeaseID: "new-lease", LeaseEpoch: 2, FenceEpoch: 2, ScopeDigest: strings.Repeat("a", contract.DigestSizeHex), ObservedRevision: 1, ExpiresUnixNano: now.Add(time.Hour).UnixNano()},
		SnapshotArtifactFactRef: ref(contract.SnapshotArtifactFactTypeURL, contract.SnapshotArtifactFactDomain, "artifact"), RequestedNotAfter: now.Add(time.Hour).UnixNano(),
	}
	stable, _ := request.StableKeyDigest()
	value, err := contract.SealWorkspaceRestoreAttemptV1(contract.WorkspaceRestoreAttemptV1{Meta: contract.Meta{ContractVersion: contract.ContractFamily, ID: request.DispatchAttemptID, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}, StableKeyDigest: stable, Request: request, BundleProjectionDigest: strings.Repeat("b", contract.DigestSizeHex), BundleDigest: strings.Repeat("c", contract.DigestSizeHex), State: contract.WorkspaceRestoreAttemptPreparedV1})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func workspaceRestoreInvocationFixtureSQLiteV1(t *testing.T, ctx context.Context, store *Store, prepared contract.WorkspaceRestoreAttemptV1) contract.WorkspaceRestoreAttemptV1 {
	t.Helper()
	now := time.Unix(1_950_000_000, int64(time.Second))
	ref := func(typeURL, domain, id string) contract.SnapshotArtifactExactRefV2 {
		digest, _ := contract.Digest("workspace-restore-sqlite-governance-ref-v1", id)
		return contract.SnapshotArtifactExactRefV2{TypeURL: typeURL, Version: 1, ID: id, Revision: 1, DigestAlgorithm: contract.SnapshotArtifactDigestSHA256, DigestDomain: domain, Digest: digest, ExpiresUnixNano: prepared.Meta.ExpiresUnixNano}
	}
	governance, err := contract.SealWorkspaceRestoreGovernanceCurrentProjectionV1(contract.WorkspaceRestoreGovernanceCurrentProjectionV1{
		TenantID: prepared.Request.TenantID, RuntimeRestoreAttempt: prepared.Request.RuntimeRestoreAttempt, RestoreEligibility: prepared.Request.RestoreEligibility, Target: prepared.Request.Target,
		ActionAdmissionRef: ref("praxis.runtime/action-admission/v1", "admission", "admission"), ReviewAuthorizationRef: ref("praxis.runtime/review-authorization/v5", "review", "review"), DispatchPermitRef: ref("praxis.runtime/dispatch-permit/v1", "permit", "permit"), BeginRef: ref("praxis.runtime/operation-begin/v3", "begin", "begin"), EnforcementRef: ref("praxis.runtime/enforcement/v1", "enforcement", prepared.Request.DispatchAttemptID),
		CheckedUnixNano: prepared.Meta.CreatedUnixNano, ExpiresUnixNano: prepared.Meta.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	governed := prepared.Clone()
	governed.Meta.Revision++
	governed.Meta.UpdatedUnixNano = now.UnixNano()
	governed.State = contract.WorkspaceRestoreAttemptGovernedV1
	governed.GovernanceProjectionDigest = governance.ProjectionDigest
	governed.Governance = &governance
	governed, err = contract.SealWorkspaceRestoreAttemptV1(governed)
	if err != nil {
		t.Fatal(err)
	}
	if created, err := store.CASWorkspaceRestoreAttemptV1(ctx, prepared.ExactRef(), governed); err != nil || !created {
		t.Fatalf("governed create=%v err=%v", created, err)
	}
	invocation := governed.Clone()
	invocation.Meta.Revision++
	invocation.Meta.UpdatedUnixNano++
	invocation.State = contract.WorkspaceRestoreAttemptInvocationV1
	invocation, err = contract.SealWorkspaceRestoreAttemptV1(invocation)
	if err != nil {
		t.Fatal(err)
	}
	if created, err := store.CASWorkspaceRestoreAttemptV1(ctx, governed.ExactRef(), invocation); err != nil || !created {
		t.Fatalf("invocation create=%v err=%v", created, err)
	}
	return invocation
}

func workspaceRestoreFinalFixtureSQLiteV1(t *testing.T, prepared contract.WorkspaceRestoreAttemptV1, suffix string) (contract.WorkspaceRestoreAttemptV1, contract.WorkspaceRestoreStageFactV1) {
	t.Helper()
	now := time.Unix(1_950_000_002, 0)
	root, err := contract.SealWorkspaceRootRefV1(contract.WorkspaceRootRefV1{ID: "workspace-root-" + suffix, TenantID: prepared.Request.TenantID, RestoreAttemptID: prepared.Request.RuntimeRestoreAttempt.ID, RuntimeRestoreAttempt: prepared.Request.RuntimeRestoreAttempt, StageAttemptRef: prepared.ExactRef(), Target: prepared.Request.Target, BundleDigest: prepared.BundleDigest})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := contract.SealWorkspaceRestoreStageFactV1(contract.WorkspaceRestoreStageFactV1{Meta: contract.Meta{ContractVersion: contract.ContractFamily, ID: prepared.Meta.ID + "-fact-" + suffix, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(24 * time.Hour).UnixNano()}, TenantID: prepared.Request.TenantID, AttemptRef: prepared.ExactRef(), RuntimeRestoreAttempt: prepared.Request.RuntimeRestoreAttempt, RestoreEligibility: prepared.Request.RestoreEligibility, Target: prepared.Request.Target, SnapshotArtifactFactRef: prepared.Request.SnapshotArtifactFactRef, BundleDigest: prepared.BundleDigest, RootRef: root, Governance: *prepared.Governance, State: contract.WorkspaceRestoreStageCompleteV1})
	if err != nil {
		t.Fatal(err)
	}
	next := prepared.Clone()
	next.Meta.Revision++
	next.Meta.UpdatedUnixNano = now.UnixNano()
	next.State = contract.WorkspaceRestoreAttemptStagedV1
	next.RootRef = &root
	providerRef := prepared.ExactRef()
	next.ProviderStageAttemptRef = &providerRef
	factRef := fact.ExactRef()
	next.StageFactRef = &factRef
	next, err = contract.SealWorkspaceRestoreAttemptV1(next)
	if err != nil {
		t.Fatal(err)
	}
	return next, fact
}
