package localcacheconformance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/fragmentcache"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/framecache"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/localcache"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/projectioncache"
)

type FactoryV1 func(localcache.LocalCacheConfigV1) (localcache.ContextLocalCacheFacadeV1, error)

func RunV1(t *testing.T, factory FactoryV1) {
	t.Helper()
	t.Run("typed-three-cache-round-trip", func(t *testing.T) { typedRoundTripV1(t, factory) })
	t.Run("attempt-window-capacity-and-rollback", func(t *testing.T) { attemptWindowV1(t, factory) })
	t.Run("deterministic-eviction-ttl-and-telemetry", func(t *testing.T) { deterministicEvictionV1(t, factory) })
	t.Run("charge-boundary-and-expired-purge", func(t *testing.T) { chargeAndExpiryV1(t, factory) })
	t.Run("aba-cancel-no-alias-and-concurrency", func(t *testing.T) { abaCancelAliasV1(t, factory) })
}

func configV1(entries uint32) localcache.LocalCacheConfigV1 {
	capacity := localcache.LocalCacheCapacityV1{MaxEntries: entries, MaxChargeBytes: 8 * 1024 * 1024, AttemptRecoveryNanos: 10}
	return localcache.LocalCacheConfigV1{Fragment: capacity, Frame: capacity, Projection: capacity}
}

func typedRoundTripV1(t *testing.T, factory FactoryV1) {
	facade := mustFacadeV1(t, factory, configV1(8))
	fragment := fragmentEntryV1(t, 1, "typed", testkit.Now+500)
	frame := frameEntryV1(t, 1, testkit.Now+500)
	projection := projectionEntryV1(t, 1)
	if _, err := facade.PutFragmentV1(context.Background(), "fragment", fragment, testkit.Now); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.PutFrameV1(context.Background(), "frame", frame, testkit.Now); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.PutProjectionV1(context.Background(), "projection", projection, testkit.Now); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.GetFragmentV1(context.Background(), fragment.Key, testkit.Now); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.GetFrameV1(context.Background(), frame.Key, testkit.Now); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.GetProjectionV1(context.Background(), projection.Key, testkit.Now); err != nil {
		t.Fatal(err)
	}
	if got, err := facade.InspectFragmentAttemptV1(context.Background(), "fragment", testkit.Now); err != nil || got.ValueDigest != fragment.ValueDigest {
		t.Fatalf("fragment inspect: %#v %v", got, err)
	}
	if got, err := facade.InspectFrameAttemptV1(context.Background(), "frame", testkit.Now); err != nil || got.ValueDigest != frame.ValueDigest {
		t.Fatalf("frame inspect: %#v %v", got, err)
	}
	if got, err := facade.InspectProjectionAttemptV1(context.Background(), "projection", testkit.Now); err != nil || got.ValueDigest != projection.ValueDigest {
		t.Fatalf("projection inspect: %#v %v", got, err)
	}
	if err := facade.InvalidateFragmentV1(context.Background(), fragment.Key, 2); err != nil {
		t.Fatal(err)
	}
	if err := facade.InvalidateFrameV1(context.Background(), frame.Key, 2); err != nil {
		t.Fatal(err)
	}
	if err := facade.InvalidateProjectionV1(context.Background(), projection.Key, 2, contract.CacheInvalidationFrameChangedV1); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.GetFragmentV1(context.Background(), fragment.Key, testkit.Now); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("fragment stale: %v", err)
	}
	if _, err := facade.GetFrameV1(context.Background(), frame.Key, testkit.Now); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("frame stale: %v", err)
	}
	if _, err := facade.GetProjectionV1(context.Background(), projection.Key, testkit.Now); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("projection stale: %v", err)
	}
}

func attemptWindowV1(t *testing.T, factory FactoryV1) {
	facade := mustFacadeV1(t, factory, configV1(2))
	entry := fragmentEntryV1(t, 1, "attempt", testkit.Now+500)
	for _, attempt := range []string{"attempt-1", "attempt-2"} {
		if _, err := facade.PutFragmentV1(context.Background(), attempt, entry, testkit.Now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := facade.PutFragmentV1(context.Background(), "attempt-3", entry, testkit.Now); !errors.Is(err, contract.ErrLimitExceeded) {
		t.Fatalf("attempt bound: %v", err)
	}
	if _, err := facade.PutFragmentV1(context.Background(), "attempt-1", entry, testkit.Now); !errors.Is(err, contract.ErrInspectOnly) {
		t.Fatalf("repeat attempt: %v", err)
	}
	if _, err := facade.InspectFragmentAttemptV1(context.Background(), "attempt-1", testkit.Now+9); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.InspectFragmentAttemptV1(context.Background(), "attempt-1", testkit.Now+10); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("attempt expiry: %v", err)
	}
	if _, err := facade.PutFragmentV1(context.Background(), "attempt-3", entry, testkit.Now+10); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.GetFragmentV1(context.Background(), entry.Key, testkit.Now+9); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("clock rollback: %v", err)
	}
}

func deterministicEvictionV1(t *testing.T, factory FactoryV1) {
	facade := mustFacadeV1(t, factory, configV1(2))
	secondFacade := mustFacadeV1(t, factory, configV1(2))
	first := fragmentEntryV1(t, 1, "first", testkit.Now+100)
	second := fragmentEntryV1(t, 1, "second", testkit.Now+200)
	third := fragmentEntryV1(t, 1, "third", testkit.Now+300)
	if _, err := facade.PutFragmentV1(context.Background(), "first", first, testkit.Now); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.PutFragmentV1(context.Background(), "second", second, testkit.Now); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.PutFragmentV1(context.Background(), "third", third, testkit.Now+10); err != nil {
		t.Fatal(err)
	}
	for index, value := range []fragmentcache.EntryV1{first, second} {
		if _, err := secondFacade.PutFragmentV1(context.Background(), fmt.Sprintf("seed-%d", index), value, testkit.Now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := secondFacade.PutFragmentV1(context.Background(), "seed-third", third, testkit.Now+10); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.GetFragmentV1(context.Background(), first.Key, testkit.Now+10); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("first not evicted: %v", err)
	}
	if _, err := facade.GetFragmentV1(context.Background(), second.Key, testkit.Now+10); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.GetFragmentV1(context.Background(), third.Key, testkit.Now+10); err != nil {
		t.Fatal(err)
	}
	observation, err := facade.ObserveLocalCacheV1(context.Background(), testkit.Now+10)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Fragment.Evictions != 1 || observation.Fragment.Entries != 2 || observation.Fragment.Hits != 2 || observation.Fragment.Misses != 1 || observation.Fragment.ChargeBytes == 0 {
		t.Fatalf("telemetry: %#v", observation.Fragment)
	}
	observation.Fragment.Invalidations[contract.CacheInvalidationFragmentChangedV1] = 99
	again, err := facade.ObserveLocalCacheV1(context.Background(), testkit.Now+10)
	if err != nil || again.Fragment.Invalidations[contract.CacheInvalidationFragmentChangedV1] == 99 {
		t.Fatalf("telemetry aliases: %#v %v", again, err)
	}
	secondObservation, err := secondFacade.ObserveLocalCacheV1(context.Background(), testkit.Now+10)
	if err != nil {
		t.Fatal(err)
	}
	if secondObservation.Fragment.Evictions != 1 || secondObservation.Fragment.Entries != 2 || secondObservation.Fragment.ChargeBytes != observation.Fragment.ChargeBytes {
		t.Fatalf("deterministic schedule drift: %#v %#v", observation.Fragment, secondObservation.Fragment)
	}
	if _, err = secondFacade.GetFragmentV1(context.Background(), first.Key, testkit.Now+10); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("deterministic victim drift: %v", err)
	}
}

func chargeAndExpiryV1(t *testing.T, factory FactoryV1) {
	entry := fragmentEntryV1(t, 1, "charge", testkit.Now+2)
	payload, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	tooSmall := configV1(4)
	tooSmall.Fragment.MaxChargeBytes = uint64(len(payload) - 1)
	facade := mustFacadeV1(t, factory, tooSmall)
	if _, err = facade.PutFragmentV1(context.Background(), "too-large", entry, testkit.Now); !errors.Is(err, contract.ErrLimitExceeded) {
		t.Fatalf("charge bound: %v", err)
	}
	observation, err := facade.ObserveLocalCacheV1(context.Background(), testkit.Now)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Fragment.Entries != 0 || observation.Fragment.ChargeBytes != 0 {
		t.Fatalf("failed charge mutated cache: %#v", observation.Fragment)
	}
	exact := configV1(4)
	exact.Fragment.MaxChargeBytes = uint64(len(payload))
	facade = mustFacadeV1(t, factory, exact)
	if _, err = facade.PutFragmentV1(context.Background(), "exact", entry, testkit.Now); err != nil {
		t.Fatal(err)
	}
	if _, err = facade.GetFragmentV1(context.Background(), entry.Key, entry.ExpiresUnixNano); !errors.Is(err, contract.ErrExpired) {
		t.Fatalf("ttl equality: %v", err)
	}
	observation, err = facade.ObserveLocalCacheV1(context.Background(), entry.ExpiresUnixNano)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Fragment.ExpiredPurges != 1 || observation.Fragment.Entries != 0 || observation.Fragment.ChargeBytes != 0 {
		t.Fatalf("expiry telemetry: %#v", observation.Fragment)
	}
}

func abaCancelAliasV1(t *testing.T, factory FactoryV1) {
	facade := mustFacadeV1(t, factory, configV1(128))
	entry := fragmentEntryV1(t, 1, "aba", testkit.Now+500)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := facade.PutFragmentV1(ctx, "canceled", entry, testkit.Now); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := facade.InspectFragmentAttemptV1(context.Background(), "canceled", testkit.Now); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("canceled visible: %v", err)
	}
	if _, err := facade.PutFragmentV1(context.Background(), "winner", entry, testkit.Now); err != nil {
		t.Fatal(err)
	}
	changed := entry
	changed.Fragment.Position++
	changed, err := fragmentcache.SealEntryV1(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = facade.PutFragmentV1(context.Background(), "changed", changed, testkit.Now); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("same key changed value: %v", err)
	}
	got, err := facade.GetFragmentV1(context.Background(), entry.Key, testkit.Now)
	if err != nil {
		t.Fatal(err)
	}
	got.Key.PromptAssetRefs = append(got.Key.PromptAssetRefs, contract.PromptAssetRefV1{ID: "mutated", Revision: 1, Digest: testkit.D("mutated")})
	again, err := facade.GetFragmentV1(context.Background(), entry.Key, testkit.Now)
	if err != nil || len(again.Key.PromptAssetRefs) != len(entry.Key.PromptAssetRefs) {
		t.Fatalf("alias: %#v %v", again, err)
	}
	if err = facade.InvalidateFragmentV1(context.Background(), entry.Key, 2); err != nil {
		t.Fatal(err)
	}
	if _, err = facade.GetFragmentV1(context.Background(), entry.Key, testkit.Now); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("stale generation: %v", err)
	}
	next := entry
	next.Key.InvalidationGeneration = 2
	next.Key.Digest = ""
	next.Key, err = contract.SealContextFragmentCacheKeyV1(next.Key)
	if err != nil {
		t.Fatal(err)
	}
	next, err = fragmentcache.SealEntryV1(next)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = facade.PutFragmentV1(context.Background(), "next", next, testkit.Now); err != nil {
		t.Fatal(err)
	}
	if err = facade.InvalidateFragmentV1(context.Background(), entry.Key, 2); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("ABA: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for index := 0; index < 64; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, callErr := facade.PutFragmentV1(context.Background(), fmt.Sprintf("parallel-%d", index), next, testkit.Now)
			errs <- callErr
		}(index)
	}
	wg.Wait()
	close(errs)
	for callErr := range errs {
		if callErr != nil {
			t.Fatal(callErr)
		}
	}
	firstObservation, err := facade.ObserveLocalCacheV1(context.Background(), testkit.Now)
	if err != nil {
		t.Fatal(err)
	}
	secondObservation, err := facade.ObserveLocalCacheV1(context.Background(), testkit.Now)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstObservation, secondObservation) {
		t.Fatalf("observation changed decisions: %#v %#v", firstObservation, secondObservation)
	}
}

func mustFacadeV1(t *testing.T, factory FactoryV1, config localcache.LocalCacheConfigV1) localcache.ContextLocalCacheFacadeV1 {
	t.Helper()
	facade, err := factory(config)
	if err != nil {
		t.Fatal(err)
	}
	return facade
}

func fragmentEntryV1(t *testing.T, generation uint64, suffix string, expires int64) fragmentcache.EntryV1 {
	t.Helper()
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	fragment := fixture.Result.Manifest.Fragments[0]
	agent := fixture.Request.AgentInstanceRef
	agent.ID += "-" + suffix
	key, err := contract.SealContextFragmentCacheKeyV1(contract.ContextFragmentCacheKeyV1{TenantScopeDigest: fixture.Request.TenantScopeDigest, AgentInstanceRef: agent, RunID: fixture.Request.RunID, RunScopeDigest: fixture.Request.RunScopeDigest, FragmentRef: fragment.CandidateRef, Content: fragment.Content, PromptAssetRefs: []contract.PromptAssetRefV1{}, RecipeRef: fixture.Request.RecipeRef, DisclosureClass: fixture.Request.DisclosureClass, InvalidationGeneration: generation, KeyVersion: contract.FrameConsumptionKeyV1})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := fragmentcache.SealEntryV1(fragmentcache.EntryV1{Key: key, Fragment: fragment, ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func frameEntryV1(t *testing.T, generation uint64, expires int64) framecache.EntryV1 {
	t.Helper()
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	key, err := contract.SealContextFrameCacheKeyV1(contract.ContextFrameCacheKeyV1{TenantScopeDigest: fixture.Request.TenantScopeDigest, AgentInstanceRef: fixture.Request.AgentInstanceRef, RunID: fixture.Request.RunID, RunScopeDigest: fixture.Request.RunScopeDigest, FrameRef: fixture.Request.FrameRef, ManifestRef: fixture.Request.ManifestRef, GenerationRef: fixture.Request.GenerationRef, PromptAssetRefs: []contract.PromptAssetRefV1{}, RecipeRef: fixture.Request.RecipeRef, DisclosureClass: fixture.Request.DisclosureClass, InvalidationGeneration: generation, KeyVersion: contract.FrameConsumptionKeyV1})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := framecache.SealEntryV1(framecache.EntryV1{Key: key, Manifest: fixture.Result.Manifest, Frame: fixture.Result.Frame, Generation: fixture.Generation, ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func projectionEntryV1(t *testing.T, generation uint64) projectioncache.EntryV1 {
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
	ref, err := descriptor.RefV1()
	if err != nil {
		t.Fatal(err)
	}
	key, err := contract.SealContextProjectionCacheKeyV1(contract.ContextProjectionCacheKeyV1{DescriptorRef: ref, TenantScopeDigest: descriptor.TenantScopeDigest, RunScopeDigest: descriptor.RunScopeDigest, DisclosureClass: descriptor.DisclosureClass, FrameFingerprint: descriptor.CacheHint.FrameFingerprint, InvalidationGeneration: generation, KeyVersion: contract.FrameConsumptionKeyV1})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := projectioncache.SealEntryV1(projectioncache.EntryV1{Key: key, Descriptor: descriptor, ExpiresUnixNano: descriptor.ExpiresUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}
