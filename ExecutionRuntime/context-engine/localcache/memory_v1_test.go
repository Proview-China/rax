package localcache

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/fragmentcache"
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
