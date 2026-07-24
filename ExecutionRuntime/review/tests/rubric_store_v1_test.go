package review_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	reviewsqlite "github.com/Proview-China/rax/ExecutionRuntime/review/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func nextRubricV1(t testing.TB, current contract.RubricDefinitionV1, name string, updated time.Time, state contract.RubricStateV1) contract.RubricDefinitionV1 {
	t.Helper()
	next := current
	next.Revision++
	next.Name = name
	next.State = state
	next.UpdatedUnixNano = updated.UnixNano()
	next.Digest = ""
	sealed, err := contract.SealRubricDefinitionV1(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func revokeRubricV1(t testing.TB, current contract.RubricDefinitionV1, updated time.Time) contract.RubricDefinitionV1 {
	t.Helper()
	next := current
	next.Revision++
	next.State = contract.RubricRevokedV1
	next.UpdatedUnixNano = updated.UnixNano()
	next.Digest = ""
	sealed, err := contract.SealRubricDefinitionV1(next)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func rubricFixtureV1(t testing.TB, now time.Time) conformance.RubricStoreFixtureV1 {
	t.Helper()
	created := testkit.Rubric(now, "tenant-a")
	next := nextRubricV1(t, created, "Action safety revised", now.Add(time.Second), contract.RubricActiveV1)
	revoked := revokeRubricV1(t, next, now.Add(2*time.Second))
	return conformance.RubricStoreFixtureV1{
		Now:       now.Add(3 * time.Second),
		Create:    reviewport.PublishRubricMutationV1{Next: created},
		Supersede: reviewport.PublishRubricMutationV1{Expected: refPtrV1(created.ExactRef()), Next: next},
		Revoke:    reviewport.RevokeRubricMutationV1{Expected: next.ExactRef(), Next: revoked},
	}
}

func refPtrV1(value contract.ExactResourceRefV1) *contract.ExactResourceRefV1 { return &value }

func TestRubricStoreConformanceMemoryAndSQLiteV1(t *testing.T) {
	now := time.Unix(1_901_100_000, 0)
	t.Run("memory", func(t *testing.T) {
		if err := conformance.CheckRubricStoreV1(context.Background(), memory.NewStore(), rubricFixtureV1(t, now)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("sqlite", func(t *testing.T) {
		store, err := reviewsqlite.Open(context.Background(), reviewsqlite.Config{Path: filepath.Join(t.TempDir(), "rubric.sqlite"), Clock: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		if err := conformance.CheckRubricStoreV1(context.Background(), store, rubricFixtureV1(t, now)); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRubricHistoricalExactDeepCloneAndExpiryV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_901_100_100, 0)
	store := memory.NewStore()
	first := testkit.PublishRubric(ctx, store, now, "tenant-a")
	copyValue, err := store.InspectRubricExactV1(ctx, first.TenantID, first.ExactRef())
	if err != nil {
		t.Fatal(err)
	}
	copyValue.Criteria[0].Title = "mutated alias"
	again, err := store.InspectRubricExactV1(ctx, first.TenantID, first.ExactRef())
	if err != nil || again.Criteria[0].Title == "mutated alias" {
		t.Fatalf("Rubric exact Inspect leaked a mutable alias: %v", err)
	}
	next := nextRubricV1(t, first, "revised", now.Add(time.Second), contract.RubricActiveV1)
	if _, err := store.PublishRubricV1(ctx, reviewport.PublishRubricMutationV1{Expected: refPtrV1(first.ExactRef()), Next: next}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectRubricExactV1(ctx, first.TenantID, first.ExactRef()); err != nil {
		t.Fatalf("historical exact Inspect borrowed current index: %v", err)
	}
	if _, err := store.InspectRubricCurrentV1(ctx, first.TenantID, next.ExactRef(), time.Unix(0, next.ExpiresUnixNano)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("Rubric expiry did not fail closed: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.InspectRubricExactV1(cancelled, first.TenantID, first.ExactRef()); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("cancelled exact Inspect did not preserve a closed error: %v", err)
	}
}

func TestRubricSQLiteRestartAndIntegrityV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_901_100_200, 0)
	path := filepath.Join(t.TempDir(), "rubric-restart.sqlite")
	store, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	first := testkit.PublishRubric(ctx, store, now, "tenant-a")
	next := nextRubricV1(t, first, "restarted", now.Add(time.Second), contract.RubricActiveV1)
	if _, err := store.PublishRubricV1(ctx, reviewport.PublishRubricMutationV1{Expected: refPtrV1(first.ExactRef()), Next: next}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: path, Clock: func() time.Time { return now.Add(2 * time.Second) }})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if err := restarted.IntegrityCheckV1(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.InspectRubricExactV1(ctx, first.TenantID, first.ExactRef()); err != nil {
		t.Fatal(err)
	}
	if current, err := restarted.InspectRubricCurrentV1(ctx, next.TenantID, next.ExactRef(), now.Add(2*time.Second)); err != nil || current.Digest != next.Digest {
		t.Fatalf("Rubric current did not survive restart: %+v %v", current, err)
	}
}

func TestRubricConcurrentCASHasOneCanonicalWinnerV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_901_100_300, 0)
	store := memory.NewStore()
	first := testkit.PublishRubric(ctx, store, now, "tenant-a")
	candidates := make([]contract.RubricDefinitionV1, 64)
	for i := range candidates {
		candidates[i] = nextRubricV1(t, first, fmt.Sprintf("candidate-%02d", i), now.Add(time.Second), contract.RubricActiveV1)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := store.PublishRubricV1(ctx, reviewport.PublishRubricMutationV1{Expected: refPtrV1(first.ExactRef()), Next: candidates[i]}); err == nil {
				successes.Add(1)
			} else if !core.HasCategory(err, core.ErrorConflict) {
				t.Errorf("unexpected CAS error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("Rubric CAS winners=%d want=1", successes.Load())
	}
}

func TestRubricSQLiteConcurrentCASHasOneCanonicalWinnerV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_901_100_350, 0)
	store, err := reviewsqlite.Open(ctx, reviewsqlite.Config{Path: filepath.Join(t.TempDir(), "rubric-race.sqlite"), BusyTimeout: 10 * time.Second, MaxOpenConns: 8, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := testkit.PublishRubric(ctx, store, now, "tenant-a")
	candidates := make([]contract.RubricDefinitionV1, 64)
	for i := range candidates {
		candidates[i] = nextRubricV1(t, first, fmt.Sprintf("sqlite-candidate-%02d", i), now.Add(time.Second), contract.RubricActiveV1)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := range candidates {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := store.PublishRubricV1(ctx, reviewport.PublishRubricMutationV1{Expected: refPtrV1(first.ExactRef()), Next: candidates[i]})
			if err == nil {
				successes.Add(1)
				return
			}
			if !core.HasCategory(err, core.ErrorConflict) {
				t.Errorf("unexpected SQLite CAS error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("SQLite Rubric CAS winners=%d want=1", successes.Load())
	}
}

func TestRubricRejectsCreateDriftRevisionGapAndSnapshotABAV1(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_901_100_375, 0)
	store := memory.NewStore()
	first := testkit.PublishRubric(ctx, store, now, "tenant-a")
	drift := first
	drift.Name = "same ID and revision, changed payload"
	drift.Digest = ""
	drift, _ = contract.SealRubricDefinitionV1(drift)
	if _, err := store.PublishRubricV1(ctx, reviewport.PublishRubricMutationV1{Next: drift}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same Rubric revision with changed content was accepted: %v", err)
	}
	gap := first
	gap.Revision = 3
	gap.UpdatedUnixNano = now.Add(time.Second).UnixNano()
	gap.Digest = ""
	gap, _ = contract.SealRubricDefinitionV1(gap)
	if _, err := store.PublishRubricV1(ctx, reviewport.PublishRubricMutationV1{Expected: refPtrV1(first.ExactRef()), Next: gap}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Rubric revision gap was accepted: %v", err)
	}
	snapshot, err := store.ExportSnapshotV1(first.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Rubrics.HighestRevision[first.ID] = first.Revision + 1
	snapshot.Digest = ""
	if _, err := memory.SealSnapshotV1(snapshot); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Rubric snapshot ABA/highest drift was sealed: %v", err)
	}
	if historical, err := store.InspectRubricExactV1(ctx, first.TenantID, first.ExactRef()); err != nil || historical.Digest != first.Digest {
		t.Fatalf("failed Rubric mutations damaged exact history: %+v %v", historical, err)
	}
}

type lostReplyRubricStoreV1 struct {
	*memory.Store
	publishCalls atomic.Int32
	revokeCalls  atomic.Int32
}

func (s *lostReplyRubricStoreV1) PublishRubricV1(ctx context.Context, m reviewport.PublishRubricMutationV1) (contract.RubricDefinitionV1, error) {
	s.publishCalls.Add(1)
	value, err := s.Store.PublishRubricV1(ctx, m)
	if err != nil {
		return value, err
	}
	return contract.RubricDefinitionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Rubric publish reply loss")
}

func (s *lostReplyRubricStoreV1) RevokeRubricV1(ctx context.Context, m reviewport.RevokeRubricMutationV1) (contract.RubricDefinitionV1, error) {
	s.revokeCalls.Add(1)
	value, err := s.Store.RevokeRubricV1(ctx, m)
	if err != nil {
		return value, err
	}
	return contract.RubricDefinitionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Rubric revoke reply loss")
}

func TestRubricServiceLostReplyUsesExactInspectOnlyV1(t *testing.T) {
	now := time.Unix(1_901_100_400, 0)
	clock := testkit.NewClock(now.Add(3 * time.Second))
	store := &lostReplyRubricStoreV1{Store: memory.NewStore()}
	owner, err := service.New(store, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	first := testkit.Rubric(now, "tenant-a")
	published, err := owner.PublishRubricV1(context.Background(), reviewport.PublishRubricMutationV1{Next: first})
	if err != nil || published.Digest != first.Digest || store.publishCalls.Load() != 1 {
		t.Fatalf("Rubric publish recovery re-mutated or drifted: calls=%d err=%v", store.publishCalls.Load(), err)
	}
	revoked := revokeRubricV1(t, first, now.Add(time.Second))
	got, err := owner.RevokeRubricV1(context.Background(), reviewport.RevokeRubricMutationV1{Expected: first.ExactRef(), Next: revoked})
	if err != nil || got.Digest != revoked.Digest || store.revokeCalls.Load() != 1 {
		t.Fatalf("Rubric revoke recovery re-mutated or drifted: calls=%d err=%v", store.revokeCalls.Load(), err)
	}
}
