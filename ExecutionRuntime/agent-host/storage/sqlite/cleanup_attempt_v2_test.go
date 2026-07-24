package sqlite

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

func TestCleanupAttemptV2RequiresClaimPersistsAndRecoversLostReplies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cleanup-attempt.db")
	store := openTestStore(t, path)
	claim := claimFixture(t, "host-cleanup", "start-cleanup", "config-cleanup")
	initial := cleanupAttemptFixtureV2(t, claim.HostID, claim.StartID, "cleanup/plan/node")
	if _, err := store.CreateCleanupAttemptV2(context.Background(), initial); err == nil {
		t.Fatal("cleanup attempt without permanent HostStart claim succeeded")
	}
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	store.loseNextReplyForTest()
	if _, err := store.CreateCleanupAttemptV2(context.Background(), initial); err == nil {
		t.Fatal("lost create reply was reported as success")
	}
	created, err := store.InspectCleanupAttemptV2(context.Background(), initial.AttemptID)
	if err != nil || created.Digest != initial.Digest {
		t.Fatalf("lost create reply did not recover by exact Inspect: %+v %v", created, err)
	}
	next := cleanupAttemptUnknownV2(t, created)
	store.loseNextReplyForTest()
	if _, err = store.CompareAndSwapCleanupAttemptV2(context.Background(), cleanupAttemptRefV2(created), next); err == nil {
		t.Fatal("lost CAS reply was reported as success")
	}
	got, err := store.InspectCleanupAttemptV2(context.Background(), initial.AttemptID)
	if err != nil || got.Digest != next.Digest {
		t.Fatalf("lost CAS reply did not recover by exact Inspect: %+v %v", got, err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTestStore(t, path)
	defer reopened.Close()
	got, err = reopened.InspectCleanupAttemptV2(context.Background(), initial.AttemptID)
	if err != nil || got.Digest != next.Digest {
		t.Fatalf("restart lost cleanup attempt: %+v %v", got, err)
	}
}

func TestCleanupAttemptV2CreateAndCASAreLinearizedAcross64Callers(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "cleanup-concurrent.db"))
	defer store.Close()
	claim := claimFixture(t, "host-concurrent-cleanup", "start-concurrent-cleanup", "config-concurrent-cleanup")
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	initial := cleanupAttemptFixtureV2(t, claim.HostID, claim.StartID, "cleanup/plan/concurrent-node")
	var creates atomic.Int64
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := store.CreateCleanupAttemptV2(context.Background(), initial)
			if err == nil && got.Digest == initial.Digest {
				creates.Add(1)
			}
		}()
	}
	wg.Wait()
	// Exact create is idempotent for every caller; the following CAS proves
	// only one caller can own the successor linearization point.
	if creates.Load() != 64 {
		t.Fatalf("exact idempotent create successes = %d, want 64", creates.Load())
	}
	next := cleanupAttemptUnknownV2(t, initial)
	var casWins atomic.Int64
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.CompareAndSwapCleanupAttemptV2(context.Background(), cleanupAttemptRefV2(initial), next); err == nil {
				casWins.Add(1)
			}
		}()
	}
	wg.Wait()
	if casWins.Load() != 1 {
		t.Fatalf("cleanup CAS wins = %d, want 1", casWins.Load())
	}
	got, err := store.InspectCleanupAttemptV2(context.Background(), initial.AttemptID)
	if err != nil || got.Digest != next.Digest {
		t.Fatalf("cleanup CAS final state = %+v %v", got, err)
	}
}

func TestCleanupAttemptV2ChangedCreateAndStaleCASConflict(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "cleanup-conflict.db"))
	defer store.Close()
	claim := claimFixture(t, "host-cleanup-conflict", "start-cleanup-conflict", "config-cleanup-conflict")
	if _, err := store.ClaimOrInspectHostStartV1(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	initial := cleanupAttemptFixtureV2(t, claim.HostID, claim.StartID, "cleanup/plan/conflict-node")
	if _, err := store.CreateCleanupAttemptV2(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	changed := initial
	changed.RequestDigest = digestV1(t, "changed-cleanup-request")
	changed.Digest = ""
	changed, err := contract.SealCleanupAttemptV2(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateCleanupAttemptV2(context.Background(), changed); err == nil {
		t.Fatal("changed-content create succeeded")
	}
	next := cleanupAttemptUnknownV2(t, initial)
	wrong := cleanupAttemptRefV2(initial)
	wrong.Digest = digestV1(t, "wrong-expected")
	if _, err = store.CompareAndSwapCleanupAttemptV2(context.Background(), wrong, next); err == nil {
		t.Fatal("stale cleanup CAS succeeded")
	}
}

func cleanupAttemptFixtureV2(t *testing.T, hostID, startID, attemptID string) contract.CleanupAttemptV2 {
	t.Helper()
	value, err := contract.SealCleanupAttemptV2(contract.CleanupAttemptV2{
		ContractVersion:     contract.CleanupContractVersionV2,
		AttemptID:           attemptID,
		Revision:            1,
		HostID:              hostID,
		StartID:             startID,
		PlanRef:             exactRef(t, "praxis.agent-host/cleanup-plan-v2", "cleanup-plan"),
		NodeID:              "cleanup-node",
		RequestDigest:       digestV1(t, "cleanup-request"),
		PredecessorRevision: 2,
		State:               contract.CleanupIntentRecordedV2,
		CreatedUnixNano:     testNow.UnixNano(),
		UpdatedUnixNano:     testNow.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func cleanupAttemptUnknownV2(t *testing.T, current contract.CleanupAttemptV2) contract.CleanupAttemptV2 {
	t.Helper()
	next := current
	next.Revision++
	next.State = contract.CleanupOutcomeUnknownV2
	next.UpdatedUnixNano++
	next.Digest = ""
	next, err := contract.SealCleanupAttemptV2(next)
	if err != nil {
		t.Fatal(err)
	}
	return next
}
