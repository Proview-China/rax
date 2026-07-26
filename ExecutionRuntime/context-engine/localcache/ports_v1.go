package localcache

import (
	"context"
	"fmt"
	"math"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/fragmentcache"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/framecache"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/projectioncache"
)

const (
	HardMaxEntriesV1       = uint32(100_000)
	HardMaxChargeBytesV1   = uint64(512 * 1024 * 1024)
	HardMaxRecoveryNanosV1 = int64(5 * 60 * 1_000_000_000)
)

type LocalCacheCapacityV1 struct {
	MaxEntries           uint32 `json:"max_entries"`
	MaxChargeBytes       uint64 `json:"max_charge_bytes"`
	AttemptRecoveryNanos int64  `json:"attempt_recovery_nanos"`
}

func (c LocalCacheCapacityV1) Validate() error {
	if c.MaxEntries == 0 || c.MaxEntries > HardMaxEntriesV1 || c.MaxChargeBytes == 0 || c.MaxChargeBytes > HardMaxChargeBytesV1 || c.AttemptRecoveryNanos <= 0 || c.AttemptRecoveryNanos > HardMaxRecoveryNanosV1 {
		return fmt.Errorf("%w: local cache capacity", contract.ErrInvalid)
	}
	return nil
}

type LocalCacheConfigV1 struct {
	Fragment   LocalCacheCapacityV1 `json:"fragment"`
	Frame      LocalCacheCapacityV1 `json:"frame"`
	Projection LocalCacheCapacityV1 `json:"projection"`
}

func (c LocalCacheConfigV1) Validate() error {
	if c.Fragment.Validate() != nil || c.Frame.Validate() != nil || c.Projection.Validate() != nil {
		return fmt.Errorf("%w: local cache config", contract.ErrInvalid)
	}
	return nil
}

type FragmentCachePortV1 interface {
	PutFragmentV1(context.Context, string, fragmentcache.EntryV1, int64) (fragmentcache.EntryV1, error)
	GetFragmentV1(context.Context, contract.ContextFragmentCacheKeyV1, int64) (fragmentcache.EntryV1, error)
	InspectFragmentAttemptV1(context.Context, string, int64) (fragmentcache.EntryV1, error)
	InvalidateFragmentV1(context.Context, contract.ContextFragmentCacheKeyV1, uint64) error
}

type FrameCachePortV1 interface {
	PutFrameV1(context.Context, string, framecache.EntryV1, int64) (framecache.EntryV1, error)
	GetFrameV1(context.Context, contract.ContextFrameCacheKeyV1, int64) (framecache.EntryV1, error)
	InspectFrameAttemptV1(context.Context, string, int64) (framecache.EntryV1, error)
	InvalidateFrameV1(context.Context, contract.ContextFrameCacheKeyV1, uint64) error
}

type ProjectionCachePortV1 interface {
	PutProjectionV1(context.Context, string, projectioncache.EntryV1, int64) (projectioncache.EntryV1, error)
	GetProjectionV1(context.Context, contract.ContextProjectionCacheKeyV1, int64) (projectioncache.EntryV1, error)
	InspectProjectionAttemptV1(context.Context, string, int64) (projectioncache.EntryV1, error)
	InvalidateProjectionV1(context.Context, contract.ContextProjectionCacheKeyV1, uint64, contract.ContextCacheInvalidationReasonV1) error
}

type ContextLocalCacheFacadeV1 interface {
	FragmentCachePortV1
	FrameCachePortV1
	ProjectionCachePortV1
	ObserveLocalCacheV1(context.Context, int64) (LocalCacheTelemetryObservationV1, error)
}

type LocalCacheCountersV1 struct {
	Hits          uint64                                               `json:"hits"`
	Misses        uint64                                               `json:"misses"`
	Conflicts     uint64                                               `json:"conflicts"`
	Admissions    uint64                                               `json:"admissions"`
	Evictions     uint64                                               `json:"evictions"`
	ExpiredPurges uint64                                               `json:"expired_purges"`
	Invalidations map[contract.ContextCacheInvalidationReasonV1]uint64 `json:"invalidations"`
	Entries       uint32                                               `json:"entries"`
	ChargeBytes   uint64                                               `json:"charge_bytes"`
}

type LocalCacheTelemetryObservationV1 struct {
	ConfigDigest     contract.Digest      `json:"config_digest"`
	Fragment         LocalCacheCountersV1 `json:"fragment"`
	Frame            LocalCacheCountersV1 `json:"frame"`
	Projection       LocalCacheCountersV1 `json:"projection"`
	ObservedUnixNano int64                `json:"observed_unix_nano"`
}

func saturatingIncrementV1(value *uint64) {
	if *value != math.MaxUint64 {
		*value++
	}
}

func saturatingInvalidationV1(values map[contract.ContextCacheInvalidationReasonV1]uint64, reason contract.ContextCacheInvalidationReasonV1) {
	if values[reason] != math.MaxUint64 {
		values[reason]++
	}
}
