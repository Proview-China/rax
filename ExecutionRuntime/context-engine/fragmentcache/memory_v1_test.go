package fragmentcache_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/fragmentcache"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestFragmentCacheCrossFrameReuseTTLAndNoAlias(t *testing.T) {
	entry := fragmentEntryFixtureV1(t, 1)
	store := fragmentcache.NewMemoryV1()
	if _, err := store.PutV1(context.Background(), "attempt-1", entry, testkit.Now); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetV1(context.Background(), entry.Key, testkit.Now)
	if err != nil {
		t.Fatal(err)
	}
	got.Key.PromptAssetRefs = append(got.Key.PromptAssetRefs, contract.PromptAssetRefV1{ID: "mutated", Revision: 1, Digest: testkit.D("mutated")})
	again, err := store.GetV1(context.Background(), entry.Key, testkit.Now)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Key.PromptAssetRefs) != len(entry.Key.PromptAssetRefs) {
		t.Fatal("cache result aliases stored prompt refs")
	}
	if _, err = store.GetV1(context.Background(), entry.Key, entry.ExpiresUnixNano); !errors.Is(err, contract.ErrExpired) {
		t.Fatalf("TTL boundary error=%v", err)
	}
}

func TestFragmentCacheInvalidationABAAndInspectOnly(t *testing.T) {
	entry := fragmentEntryFixtureV1(t, 1)
	store := fragmentcache.NewMemoryV1()
	applied, err := store.PutV1(context.Background(), "attempt-1", entry, testkit.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutV1(context.Background(), "attempt-1", entry, testkit.Now); !errors.Is(err, contract.ErrInspectOnly) {
		t.Fatalf("repeat attempt error=%v", err)
	}
	inspected, err := store.InspectAttemptV1(context.Background(), "attempt-1")
	if err != nil || inspected.ValueDigest != applied.ValueDigest {
		t.Fatalf("inspect exact applied=%#v err=%v", inspected, err)
	}
	if err = store.InvalidateV1(context.Background(), entry.Key, 2); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetV1(context.Background(), entry.Key, testkit.Now); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("stale generation error=%v", err)
	}
	next := fragmentEntryFixtureV1(t, 2)
	if _, err = store.PutV1(context.Background(), "attempt-2", next, testkit.Now); err != nil {
		t.Fatal(err)
	}
	if err = store.InvalidateV1(context.Background(), entry.Key, 2); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("ABA invalidation error=%v", err)
	}
}

func TestFragmentCacheConcurrentSameKeySingleCanonicalValue(t *testing.T) {
	entry := fragmentEntryFixtureV1(t, 1)
	store := fragmentcache.NewMemoryV1()
	var failures atomic.Int64
	var wg sync.WaitGroup
	for index := 0; index < 64; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			got, err := store.PutV1(context.Background(), fmt.Sprintf("attempt-%d", index), entry, testkit.Now)
			if err != nil || got.ValueDigest != entry.ValueDigest {
				failures.Add(1)
			}
		}(index)
	}
	wg.Wait()
	if failures.Load() != 0 || store.LenV1() != 1 {
		t.Fatalf("failures=%d entries=%d", failures.Load(), store.LenV1())
	}
}

func TestFragmentCacheSameKeyDifferentValueConflicts(t *testing.T) {
	entry := fragmentEntryFixtureV1(t, 1)
	store := fragmentcache.NewMemoryV1()
	if _, err := store.PutV1(context.Background(), "attempt-1", entry, testkit.Now); err != nil {
		t.Fatal(err)
	}
	changed := entry
	changed.Fragment.Position++
	changed, err := fragmentcache.SealEntryV1(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutV1(context.Background(), "attempt-2", changed, testkit.Now); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("same key different value error=%v", err)
	}
}

func TestFragmentCacheCancellationHasNoWrite(t *testing.T) {
	entry := fragmentEntryFixtureV1(t, 1)
	store := fragmentcache.NewMemoryV1()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.PutV1(ctx, "attempt-1", entry, testkit.Now); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
	if store.LenV1() != 0 {
		t.Fatalf("canceled put wrote %d entries", store.LenV1())
	}
	if _, err := store.InspectAttemptV1(context.Background(), "attempt-1"); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("canceled attempt became visible: %v", err)
	}
}

func fragmentEntryFixtureV1(t *testing.T, generation uint64) fragmentcache.EntryV1 {
	t.Helper()
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	fragment := fixture.Result.Manifest.Fragments[0]
	key, err := contract.SealContextFragmentCacheKeyV1(contract.ContextFragmentCacheKeyV1{
		TenantScopeDigest: fixture.Request.TenantScopeDigest, AgentInstanceRef: fixture.Request.AgentInstanceRef,
		RunID: fixture.Request.RunID, RunScopeDigest: fixture.Request.RunScopeDigest,
		FragmentRef: fragment.CandidateRef, Content: fragment.Content, PromptAssetRefs: []contract.PromptAssetRefV1{},
		RecipeRef: fixture.Request.RecipeRef, DisclosureClass: fixture.Request.DisclosureClass,
		InvalidationGeneration: generation, KeyVersion: contract.FrameConsumptionKeyV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := fragmentcache.SealEntryV1(fragmentcache.EntryV1{Key: key, Fragment: fragment, ExpiresUnixNano: testkit.Now + 500})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}
