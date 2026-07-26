package localcache

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/fragmentcache"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
)

func TestCapacityAndConfigValidationV1(t *testing.T) {
	valid := LocalCacheCapacityV1{MaxEntries: 1, MaxChargeBytes: 1, AttemptRecoveryNanos: 1}
	if valid.Validate() != nil || (LocalCacheConfigV1{Fragment: valid, Frame: valid, Projection: valid}).Validate() != nil {
		t.Fatal("valid local cache policy rejected")
	}
	invalid := []LocalCacheCapacityV1{
		{},
		{MaxEntries: HardMaxEntriesV1 + 1, MaxChargeBytes: 1, AttemptRecoveryNanos: 1},
		{MaxEntries: 1, MaxChargeBytes: HardMaxChargeBytesV1 + 1, AttemptRecoveryNanos: 1},
		{MaxEntries: 1, MaxChargeBytes: 1, AttemptRecoveryNanos: HardMaxRecoveryNanosV1 + 1},
	}
	for _, value := range invalid {
		if !errors.Is(value.Validate(), contract.ErrInvalid) {
			t.Fatalf("invalid policy accepted: %#v", value)
		}
	}
}

func TestNilFacadeAndTelemetrySaturationV1(t *testing.T) {
	var facade *MemoryFacadeV1
	if _, err := facade.PutFragmentV1(context.Background(), "attempt", fragmentcache.EntryV1{}, 1); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("nil facade put: %v", err)
	}
	if _, err := facade.ObserveLocalCacheV1(context.Background(), 1); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("nil facade observation: %v", err)
	}
	if err := facade.InvalidateFragmentV1(context.Background(), contract.ContextFragmentCacheKeyV1{}, 1); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("nil facade invalidation: %v", err)
	}
	value := uint64(math.MaxUint64)
	saturatingIncrementV1(&value)
	if value != math.MaxUint64 {
		t.Fatal("counter wrapped")
	}
	values := map[contract.ContextCacheInvalidationReasonV1]uint64{contract.CacheInvalidationFrameChangedV1: math.MaxUint64}
	saturatingInvalidationV1(values, contract.CacheInvalidationFrameChangedV1)
	if values[contract.CacheInvalidationFrameChangedV1] != math.MaxUint64 {
		t.Fatal("invalidation counter wrapped")
	}
}

func TestGenerationMetadataBoundedByLiveEntriesV1(t *testing.T) {
	capacity := LocalCacheCapacityV1{MaxEntries: 4, MaxChargeBytes: 8 * 1024 * 1024, AttemptRecoveryNanos: 1}
	facade, err := NewMemoryFacadeV1(LocalCacheConfigV1{Fragment: capacity, Frame: capacity, Projection: capacity})
	if err != nil {
		t.Fatal(err)
	}
	const base int64 = 1_000_000
	for index := 0; index < 128; index++ {
		now := base + int64(index)
		entry := uniqueFragmentEntryV1(t, fmt.Sprintf("evict-%03d", index), now+1_000)
		if _, err := facade.PutFragmentV1(context.Background(), fmt.Sprintf("attempt-%03d", index), entry, now); err != nil {
			t.Fatal(err)
		}
		if got := len(facade.fragment.generations); got > int(capacity.MaxEntries) {
			t.Fatalf("generation metadata escaped entry bound: %d > %d", got, capacity.MaxEntries)
		}
		if got := len(facade.fragment.partitions); got > int(capacity.MaxEntries) {
			t.Fatalf("partition metadata escaped entry bound: %d > %d", got, capacity.MaxEntries)
		}
	}
	if got := len(facade.fragment.entries); got != int(capacity.MaxEntries) {
		t.Fatalf("unexpected live entry count: %d", got)
	}

	expiring, err := NewMemoryFacadeV1(LocalCacheConfigV1{Fragment: capacity, Frame: capacity, Projection: capacity})
	if err != nil {
		t.Fatal(err)
	}
	var firstKey contract.ContextFragmentCacheKeyV1
	for index := 0; index < int(capacity.MaxEntries); index++ {
		entry := uniqueFragmentEntryV1(t, fmt.Sprintf("expire-%03d", index), base+10)
		if index == 0 {
			firstKey = entry.Key
		}
		if _, err := expiring.PutFragmentV1(context.Background(), fmt.Sprintf("expire-attempt-%03d", index), entry, base); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := expiring.GetFragmentV1(context.Background(), firstKey, base+10); !errors.Is(err, contract.ErrExpired) {
		t.Fatalf("expiry cleanup trigger: %v", err)
	}
	if got := len(expiring.fragment.generations); got != 0 {
		t.Fatalf("expired partition metadata retained: %d", got)
	}
	if got := len(expiring.fragment.partitions); got != 0 {
		t.Fatalf("expired partition counts retained: %d", got)
	}
}

func TestGenerationMetadataSurvivesSiblingEvictionV1(t *testing.T) {
	capacity := LocalCacheCapacityV1{MaxEntries: 2, MaxChargeBytes: 8 * 1024 * 1024, AttemptRecoveryNanos: 1}
	facade, err := NewMemoryFacadeV1(LocalCacheConfigV1{Fragment: capacity, Frame: capacity, Projection: capacity})
	if err != nil {
		t.Fatal(err)
	}
	const now int64 = 2_000_000
	first := uniqueFragmentEntryV1(t, "generation-sibling", now+100)
	if _, err := facade.PutFragmentV1(context.Background(), "generation-1", first, now); err != nil {
		t.Fatal(err)
	}
	if err := facade.InvalidateFragmentV1(context.Background(), first.Key, 2); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Key.InvalidationGeneration = 2
	second.Key.Digest = ""
	second.Key, err = contract.SealContextFragmentCacheKeyV1(second.Key)
	if err != nil {
		t.Fatal(err)
	}
	second.ExpiresUnixNano = now + 200
	second, err = fragmentcache.SealEntryV1(second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := facade.PutFragmentV1(context.Background(), "generation-2", second, now+1); err != nil {
		t.Fatal(err)
	}
	other := uniqueFragmentEntryV1(t, "generation-other", now+300)
	if _, err := facade.PutFragmentV1(context.Background(), "generation-other", other, now+2); err != nil {
		t.Fatal(err)
	}
	if _, err := facade.GetFragmentV1(context.Background(), second.Key, now+2); err != nil {
		t.Fatalf("active generation lost with sibling eviction: %v", err)
	}
	if err := facade.InvalidateFragmentV1(context.Background(), first.Key, 2); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("ABA generation accepted after sibling eviction: %v", err)
	}
}

func uniqueFragmentEntryV1(t *testing.T, suffix string, expires int64) fragmentcache.EntryV1 {
	t.Helper()
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	fragment := fixture.Result.Manifest.Fragments[0]
	agent := fixture.Request.AgentInstanceRef
	agent.ID += "-" + suffix
	key, err := contract.SealContextFragmentCacheKeyV1(contract.ContextFragmentCacheKeyV1{
		TenantScopeDigest:      fixture.Request.TenantScopeDigest,
		AgentInstanceRef:       agent,
		RunID:                  fixture.Request.RunID,
		RunScopeDigest:         fixture.Request.RunScopeDigest,
		FragmentRef:            fragment.CandidateRef,
		Content:                fragment.Content,
		PromptAssetRefs:        []contract.PromptAssetRefV1{},
		RecipeRef:              fixture.Request.RecipeRef,
		DisclosureClass:        fixture.Request.DisclosureClass,
		InvalidationGeneration: 1,
		KeyVersion:             contract.FrameConsumptionKeyV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := fragmentcache.SealEntryV1(fragmentcache.EntryV1{Key: key, Fragment: fragment, ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}
