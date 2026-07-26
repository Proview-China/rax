package framecache_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/framecache"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestFrameCacheExactTTLAndNoAlias(t *testing.T) {
	entry := frameEntryFixtureV1(t, 1)
	store := framecache.NewMemoryV1()
	if _, err := store.PutV1(context.Background(), "attempt-1", entry, testkit.Now); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetV1(context.Background(), entry.Key, testkit.Now)
	if err != nil {
		t.Fatal(err)
	}
	got.Manifest.Fragments[0].Position = 99
	got.Generation.RetainedAnchors = append(got.Generation.RetainedAnchors, contract.FactRef{ID: "mutated", Revision: 1, Digest: testkit.D("mutated")})
	again, err := store.GetV1(context.Background(), entry.Key, testkit.Now)
	if err != nil {
		t.Fatal(err)
	}
	if again.Manifest.Fragments[0].Position == 99 || len(again.Generation.RetainedAnchors) != 0 {
		t.Fatal("frame cache result aliases stored closure")
	}
	if _, err = store.GetV1(context.Background(), entry.Key, entry.ExpiresUnixNano); !errors.Is(err, contract.ErrExpired) {
		t.Fatalf("TTL boundary error=%v", err)
	}
}

func TestFrameCacheInvalidationABAAndLostReplyInspect(t *testing.T) {
	entry := frameEntryFixtureV1(t, 1)
	store := framecache.NewMemoryV1()
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
	next := frameEntryFixtureV1(t, 2)
	if _, err = store.PutV1(context.Background(), "attempt-2", next, testkit.Now); err != nil {
		t.Fatal(err)
	}
	if err = store.InvalidateV1(context.Background(), entry.Key, 2); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("ABA invalidation error=%v", err)
	}
}

func TestFrameCacheConcurrentSameKeySingleCanonicalValue(t *testing.T) {
	entry := frameEntryFixtureV1(t, 1)
	store := framecache.NewMemoryV1()
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

func TestFrameCacheSameKeyDifferentValueConflicts(t *testing.T) {
	entry := frameEntryFixtureV1(t, 1)
	store := framecache.NewMemoryV1()
	if _, err := store.PutV1(context.Background(), "attempt-1", entry, testkit.Now); err != nil {
		t.Fatal(err)
	}
	changed := entry
	changed.ExpiresUnixNano++
	changed, err := framecache.SealEntryV1(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutV1(context.Background(), "attempt-2", changed, testkit.Now); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("same key different value error=%v", err)
	}
}

func TestFrameCacheCancellationHasNoWrite(t *testing.T) {
	entry := frameEntryFixtureV1(t, 1)
	store := framecache.NewMemoryV1()
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

func frameEntryFixtureV1(t *testing.T, generation uint64) framecache.EntryV1 {
	t.Helper()
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	key, err := contract.SealContextFrameCacheKeyV1(contract.ContextFrameCacheKeyV1{
		TenantScopeDigest: fixture.Request.TenantScopeDigest, AgentInstanceRef: fixture.Request.AgentInstanceRef,
		RunID: fixture.Request.RunID, RunScopeDigest: fixture.Request.RunScopeDigest,
		FrameRef: fixture.Request.FrameRef, ManifestRef: fixture.Request.ManifestRef, GenerationRef: fixture.Request.GenerationRef,
		PromptAssetRefs: []contract.PromptAssetRefV1{}, RecipeRef: fixture.Request.RecipeRef,
		DisclosureClass: fixture.Request.DisclosureClass, InvalidationGeneration: generation, KeyVersion: contract.FrameConsumptionKeyV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := framecache.SealEntryV1(framecache.EntryV1{
		Key: key, Manifest: fixture.Result.Manifest, Frame: fixture.Result.Frame, Generation: fixture.Generation,
		ExpiresUnixNano: testkit.Now + 500,
	})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}
