package projectioncache_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/projectioncache"
)

func entryFixtureV1(t *testing.T, generation uint64) projectioncache.EntryV1 {
	t.Helper()
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	reader := &testfixture.FrameConsumptionReaderV1{Snapshots: []kernel.FrameConsumptionCurrentSnapshotV1{fixture.Snapshot, fixture.Snapshot}}
	descriptor, err := kernel.BuildFrameConsumptionDescriptorV1(context.Background(), reader, fixture.AwareStore, fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	descriptorRef, err := descriptor.RefV1()
	if err != nil {
		t.Fatal(err)
	}
	key, err := contract.SealContextProjectionCacheKeyV1(contract.ContextProjectionCacheKeyV1{DescriptorRef: descriptorRef, TenantScopeDigest: descriptor.TenantScopeDigest, RunScopeDigest: descriptor.RunScopeDigest, DisclosureClass: descriptor.DisclosureClass, FrameFingerprint: descriptor.CacheHint.FrameFingerprint, InvalidationGeneration: generation, KeyVersion: contract.FrameConsumptionKeyV1})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := projectioncache.SealEntryV1(projectioncache.EntryV1{Key: key, Descriptor: descriptor, ExpiresUnixNano: descriptor.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func TestProjectionCache64ConcurrentCreateOnceAndNoAlias(t *testing.T) {
	cache, entry := projectioncache.NewMemoryV1(), entryFixtureV1(t, 1)
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cache.PutV1(context.Background(), fmt.Sprintf("attempt-%d", i), entry, testkit.Now)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := cache.GetV1(context.Background(), entry.Key, testkit.Now)
	if err != nil {
		t.Fatal(err)
	}
	got.Descriptor.FragmentRefs[0].ID = "mutated"
	again, err := cache.GetV1(context.Background(), entry.Key, testkit.Now)
	if err != nil {
		t.Fatal(err)
	}
	if again.Descriptor.FragmentRefs[0].ID == "mutated" {
		t.Fatal("projection cache leaked alias")
	}
}

func TestProjectionCacheInvalidationABAInspectAndTelemetry(t *testing.T) {
	cache, entry := projectioncache.NewMemoryV1(), entryFixtureV1(t, 1)
	if _, err := cache.PutV1(context.Background(), "attempt-1", entry, testkit.Now); err != nil {
		t.Fatal(err)
	}
	if err := cache.InvalidateV1(context.Background(), entry.Key, 2, contract.CacheInvalidationFrameChangedV1); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.GetV1(context.Background(), entry.Key, testkit.Now); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("stale key survived: %v", err)
	}
	if _, err := cache.PutV1(context.Background(), "attempt-2", entry, testkit.Now); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("ABA put accepted: %v", err)
	}
	original, err := cache.InspectAttemptV1(context.Background(), "attempt-1")
	if err != nil || original.ValueDigest != entry.ValueDigest {
		t.Fatalf("lost-reply inspect: %#v %v", original, err)
	}
	if err := cache.InvalidateV1(context.Background(), entry.Key, 2, contract.CacheInvalidationFrameChangedV1); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("ABA invalidate accepted: %v", err)
	}
	telemetry := cache.TelemetryV1()
	if telemetry.Conflicts != 3 || telemetry.Invalidations[contract.CacheInvalidationFrameChangedV1] != 1 {
		t.Fatalf("telemetry %#v", telemetry)
	}
}

func TestProjectionCacheCancellationAndTTL(t *testing.T) {
	cache, entry := projectioncache.NewMemoryV1(), entryFixtureV1(t, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cache.PutV1(ctx, "attempt-1", entry, testkit.Now); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel not preserved: %v", err)
	}
	if _, err := cache.PutV1(context.Background(), "attempt-2", entry, entry.ExpiresUnixNano); !errors.Is(err, contract.ErrExpired) {
		t.Fatalf("expired put accepted: %v", err)
	}
}
