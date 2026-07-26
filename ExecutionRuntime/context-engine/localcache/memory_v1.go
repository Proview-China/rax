package localcache

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/fragmentcache"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/framecache"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/projectioncache"
)

type cacheMetaV1 struct {
	key        contract.Digest
	partition  contract.Digest
	generation uint64
}

type cacheOpsV1[K any, E any] struct {
	validateEntry func(E) error
	entryMeta     func(E) (cacheMetaV1, error)
	keyMeta       func(K) (cacheMetaV1, error)
	expires       func(E) int64
	valueDigest   func(E) contract.Digest
}

type cacheRecordV1[E any] struct {
	value          E
	expires        int64
	charge         uint64
	protectedUntil int64
}

type attemptRecordV1 struct {
	key           contract.Digest
	recoveryUntil int64
}

type cacheStoreV1[K any, E any] struct {
	mu          sync.Mutex
	config      LocalCacheCapacityV1
	ops         cacheOpsV1[K, E]
	entries     map[contract.Digest]cacheRecordV1[E]
	attempts    map[string]attemptRecordV1
	generations map[contract.Digest]uint64
	charge      uint64
	counters    LocalCacheCountersV1
}

func newCacheStoreV1[K any, E any](config LocalCacheCapacityV1, ops cacheOpsV1[K, E]) *cacheStoreV1[K, E] {
	return &cacheStoreV1[K, E]{
		config: config, ops: ops,
		entries: map[contract.Digest]cacheRecordV1[E]{}, attempts: map[string]attemptRecordV1{}, generations: map[contract.Digest]uint64{},
		counters: LocalCacheCountersV1{Invalidations: map[contract.ContextCacheInvalidationReasonV1]uint64{}},
	}
}

func (s *cacheStoreV1[K, E]) put(ctx context.Context, attemptID string, entry E, now int64) (E, error) {
	var zero E
	if err := contextErrorV1(ctx); err != nil {
		return zero, err
	}
	if strings.TrimSpace(attemptID) == "" || len(attemptID) > 256 || s.ops.validateEntry(entry) != nil {
		return zero, fmt.Errorf("%w: local cache put", contract.ErrInvalid)
	}
	meta, err := s.ops.entryMeta(entry)
	if err != nil {
		return zero, err
	}
	expires := s.ops.expires(entry)
	if now >= expires {
		return zero, fmt.Errorf("%w: local cache entry", contract.ErrExpired)
	}
	if now > math.MaxInt64-s.config.AttemptRecoveryNanos {
		return zero, fmt.Errorf("%w: local cache recovery deadline", contract.ErrLimitExceeded)
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return zero, fmt.Errorf("%w: local cache canonical entry", contract.ErrInvalid)
	}
	charge := uint64(len(payload))
	if charge == 0 || charge > s.config.MaxChargeBytes {
		return zero, fmt.Errorf("%w: local cache entry charge", contract.ErrLimitExceeded)
	}
	stored, err := cloneJSONV1(entry)
	if err != nil {
		return zero, err
	}
	recoveryUntil := now + s.config.AttemptRecoveryNanos
	if recoveryUntil > expires {
		recoveryUntil = expires
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupV1(now)
	if _, exists := s.attempts[attemptID]; exists {
		return zero, fmt.Errorf("%w: inspect local cache attempt", contract.ErrInspectOnly)
	}
	if uint32(len(s.attempts)) >= s.config.MaxEntries {
		return zero, fmt.Errorf("%w: local cache attempt capacity", contract.ErrLimitExceeded)
	}
	if current, exists := s.generations[meta.partition]; exists && current != meta.generation {
		saturatingIncrementV1(&s.counters.Conflicts)
		return zero, fmt.Errorf("%w: local cache invalidation generation", contract.ErrConflict)
	}
	if current, exists := s.entries[meta.key]; exists {
		if s.ops.valueDigest(current.value) != s.ops.valueDigest(stored) {
			saturatingIncrementV1(&s.counters.Conflicts)
			return zero, fmt.Errorf("%w: local cache same key different value", contract.ErrConflict)
		}
		if recoveryUntil > current.protectedUntil {
			current.protectedUntil = recoveryUntil
			s.entries[meta.key] = current
		}
		s.attempts[attemptID] = attemptRecordV1{key: meta.key, recoveryUntil: recoveryUntil}
		saturatingIncrementV1(&s.counters.Admissions)
		return cloneJSONV1(current.value)
	}
	victims, ok := s.victimsV1(now, charge)
	if !ok {
		return zero, fmt.Errorf("%w: local cache protected capacity", contract.ErrLimitExceeded)
	}
	for _, key := range victims {
		s.removeEntryV1(key, false)
		saturatingIncrementV1(&s.counters.Evictions)
	}
	s.entries[meta.key] = cacheRecordV1[E]{value: stored, expires: expires, charge: charge, protectedUntil: recoveryUntil}
	s.charge += charge
	s.generations[meta.partition] = meta.generation
	s.attempts[attemptID] = attemptRecordV1{key: meta.key, recoveryUntil: recoveryUntil}
	saturatingIncrementV1(&s.counters.Admissions)
	return cloneJSONV1(stored)
}

func (s *cacheStoreV1[K, E]) get(ctx context.Context, key K, now int64) (E, error) {
	var zero E
	if err := contextErrorV1(ctx); err != nil {
		return zero, err
	}
	meta, err := s.ops.keyMeta(key)
	if err != nil {
		return zero, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, existed := s.entries[meta.key]
	expiredTarget := existed && now >= record.expires
	s.cleanupV1(now)
	if expiredTarget {
		saturatingIncrementV1(&s.counters.Misses)
		return zero, fmt.Errorf("%w: local cache entry", contract.ErrExpired)
	}
	record, exists := s.entries[meta.key]
	if !exists {
		saturatingIncrementV1(&s.counters.Misses)
		return zero, fmt.Errorf("%w: local cache entry", contract.ErrNotFound)
	}
	if current, ok := s.generations[meta.partition]; !ok || current != meta.generation {
		saturatingIncrementV1(&s.counters.Conflicts)
		return zero, fmt.Errorf("%w: local cache invalidation generation", contract.ErrConflict)
	}
	if s.ops.validateEntry(record.value) != nil {
		saturatingIncrementV1(&s.counters.Conflicts)
		return zero, fmt.Errorf("%w: local cache stored entry", contract.ErrConflict)
	}
	saturatingIncrementV1(&s.counters.Hits)
	return cloneJSONV1(record.value)
}

func (s *cacheStoreV1[K, E]) inspect(ctx context.Context, attemptID string, now int64) (E, error) {
	var zero E
	if err := contextErrorV1(ctx); err != nil {
		return zero, err
	}
	if strings.TrimSpace(attemptID) == "" || len(attemptID) > 256 {
		return zero, fmt.Errorf("%w: local cache attempt", contract.ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupV1(now)
	attempt, ok := s.attempts[attemptID]
	if !ok || now >= attempt.recoveryUntil {
		return zero, fmt.Errorf("%w: local cache attempt", contract.ErrNotFound)
	}
	record, found := s.entries[attempt.key]
	if !found || now >= record.expires {
		return zero, fmt.Errorf("%w: local cache attempt value", contract.ErrNotFound)
	}
	return cloneJSONV1(record.value)
}

func (s *cacheStoreV1[K, E]) invalidate(ctx context.Context, key K, next uint64, reason contract.ContextCacheInvalidationReasonV1) error {
	if err := contextErrorV1(ctx); err != nil {
		return err
	}
	meta, err := s.ops.keyMeta(key)
	if err != nil || next != meta.generation+1 || !validInvalidationReasonV1(reason) {
		return fmt.Errorf("%w: local cache invalidate", contract.ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.generations[meta.partition]
	if !ok || current != meta.generation {
		saturatingIncrementV1(&s.counters.Conflicts)
		return fmt.Errorf("%w: local cache invalidation CAS", contract.ErrConflict)
	}
	s.generations[meta.partition] = next
	saturatingInvalidationV1(s.counters.Invalidations, reason)
	return nil
}

func (s *cacheStoreV1[K, E]) cleanupV1(now int64) {
	for id, attempt := range s.attempts {
		record, ok := s.entries[attempt.key]
		if !ok || now >= attempt.recoveryUntil || now >= record.expires {
			delete(s.attempts, id)
		}
	}
	for key, record := range s.entries {
		if now >= record.expires {
			s.removeEntryV1(key, true)
			saturatingIncrementV1(&s.counters.ExpiredPurges)
		}
	}
}

func (s *cacheStoreV1[K, E]) victimsV1(now int64, incoming uint64) ([]contract.Digest, bool) {
	needEntries := len(s.entries)+1 > int(s.config.MaxEntries)
	needBytes := s.charge > s.config.MaxChargeBytes-incoming
	if !needEntries && !needBytes {
		return nil, true
	}
	type victimV1 struct {
		key     contract.Digest
		expires int64
		charge  uint64
	}
	values := make([]victimV1, 0, len(s.entries))
	for key, record := range s.entries {
		if record.protectedUntil <= now {
			values = append(values, victimV1{key: key, expires: record.expires, charge: record.charge})
		}
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].expires != values[j].expires {
			return values[i].expires < values[j].expires
		}
		return values[i].key < values[j].key
	})
	remainingEntries, remainingCharge := len(s.entries), s.charge
	victims := make([]contract.Digest, 0, len(values))
	for _, value := range values {
		victims = append(victims, value.key)
		remainingEntries--
		remainingCharge -= value.charge
		if remainingEntries+1 <= int(s.config.MaxEntries) && remainingCharge <= s.config.MaxChargeBytes-incoming {
			return victims, true
		}
	}
	return nil, false
}

func (s *cacheStoreV1[K, E]) removeEntryV1(key contract.Digest, removeAttempts bool) {
	record, ok := s.entries[key]
	if !ok {
		return
	}
	delete(s.entries, key)
	s.charge -= record.charge
	if removeAttempts {
		for id, attempt := range s.attempts {
			if attempt.key == key {
				delete(s.attempts, id)
			}
		}
	}
}

func (s *cacheStoreV1[K, E]) snapshotV1() LocalCacheCountersV1 {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.counters
	result.Invalidations = make(map[contract.ContextCacheInvalidationReasonV1]uint64, len(s.counters.Invalidations))
	for reason, value := range s.counters.Invalidations {
		result.Invalidations[reason] = value
	}
	result.Entries = uint32(len(s.entries))
	result.ChargeBytes = s.charge
	return result
}

type MemoryFacadeV1 struct {
	clockMu      sync.Mutex
	lastNow      int64
	configDigest contract.Digest
	fragment     *cacheStoreV1[contract.ContextFragmentCacheKeyV1, fragmentcache.EntryV1]
	frame        *cacheStoreV1[contract.ContextFrameCacheKeyV1, framecache.EntryV1]
	projection   *cacheStoreV1[contract.ContextProjectionCacheKeyV1, projectioncache.EntryV1]
}

func NewMemoryFacadeV1(config LocalCacheConfigV1) (*MemoryFacadeV1, error) {
	if config.Validate() != nil {
		return nil, fmt.Errorf("%w: local cache facade config", contract.ErrInvalid)
	}
	digest, err := contract.DigestJSON(config)
	if err != nil {
		return nil, err
	}
	return &MemoryFacadeV1{
		configDigest: digest,
		fragment:     newCacheStoreV1(config.Fragment, fragmentOpsV1()),
		frame:        newCacheStoreV1(config.Frame, frameOpsV1()),
		projection:   newCacheStoreV1(config.Projection, projectionOpsV1()),
	}, nil
}

func (m *MemoryFacadeV1) acceptTimeV1(ctx context.Context, now int64) error {
	if m == nil {
		return fmt.Errorf("%w: local cache facade", contract.ErrInvalid)
	}
	if err := contextErrorV1(ctx); err != nil {
		return err
	}
	if now <= 0 {
		return fmt.Errorf("%w: local cache time", contract.ErrInvalid)
	}
	m.clockMu.Lock()
	defer m.clockMu.Unlock()
	if m.lastNow > now {
		return fmt.Errorf("%w: local cache clock rollback", contract.ErrConflict)
	}
	m.lastNow = now
	return nil
}

func (m *MemoryFacadeV1) PutFragmentV1(ctx context.Context, attempt string, entry fragmentcache.EntryV1, now int64) (fragmentcache.EntryV1, error) {
	if m == nil {
		return fragmentcache.EntryV1{}, fmt.Errorf("%w: local cache facade", contract.ErrInvalid)
	}
	if strings.TrimSpace(attempt) == "" || len(attempt) > 256 || entry.Validate() != nil {
		return fragmentcache.EntryV1{}, fmt.Errorf("%w: fragment cache put", contract.ErrInvalid)
	}
	if err := validatePutTimeV1(now, entry.ExpiresUnixNano, m, m.fragment.config.AttemptRecoveryNanos); err != nil {
		return fragmentcache.EntryV1{}, err
	}
	if err := m.acceptTimeV1(ctx, now); err != nil {
		return fragmentcache.EntryV1{}, err
	}
	return m.fragment.put(ctx, attempt, entry, now)
}
func (m *MemoryFacadeV1) GetFragmentV1(ctx context.Context, key contract.ContextFragmentCacheKeyV1, now int64) (fragmentcache.EntryV1, error) {
	if key.Validate() != nil {
		return fragmentcache.EntryV1{}, fmt.Errorf("%w: fragment cache key", contract.ErrInvalid)
	}
	if err := m.acceptTimeV1(ctx, now); err != nil {
		return fragmentcache.EntryV1{}, err
	}
	return m.fragment.get(ctx, key, now)
}
func (m *MemoryFacadeV1) InspectFragmentAttemptV1(ctx context.Context, attempt string, now int64) (fragmentcache.EntryV1, error) {
	if strings.TrimSpace(attempt) == "" || len(attempt) > 256 {
		return fragmentcache.EntryV1{}, fmt.Errorf("%w: fragment cache attempt", contract.ErrInvalid)
	}
	if err := m.acceptTimeV1(ctx, now); err != nil {
		return fragmentcache.EntryV1{}, err
	}
	return m.fragment.inspect(ctx, attempt, now)
}
func (m *MemoryFacadeV1) InvalidateFragmentV1(ctx context.Context, key contract.ContextFragmentCacheKeyV1, next uint64) error {
	if m == nil {
		return fmt.Errorf("%w: local cache facade", contract.ErrInvalid)
	}
	return m.fragment.invalidate(ctx, key, next, contract.CacheInvalidationFragmentChangedV1)
}

func (m *MemoryFacadeV1) PutFrameV1(ctx context.Context, attempt string, entry framecache.EntryV1, now int64) (framecache.EntryV1, error) {
	if m == nil {
		return framecache.EntryV1{}, fmt.Errorf("%w: local cache facade", contract.ErrInvalid)
	}
	if strings.TrimSpace(attempt) == "" || len(attempt) > 256 || entry.Validate() != nil {
		return framecache.EntryV1{}, fmt.Errorf("%w: frame cache put", contract.ErrInvalid)
	}
	if err := validatePutTimeV1(now, entry.ExpiresUnixNano, m, m.frame.config.AttemptRecoveryNanos); err != nil {
		return framecache.EntryV1{}, err
	}
	if err := m.acceptTimeV1(ctx, now); err != nil {
		return framecache.EntryV1{}, err
	}
	return m.frame.put(ctx, attempt, entry, now)
}
func (m *MemoryFacadeV1) GetFrameV1(ctx context.Context, key contract.ContextFrameCacheKeyV1, now int64) (framecache.EntryV1, error) {
	if key.Validate() != nil {
		return framecache.EntryV1{}, fmt.Errorf("%w: frame cache key", contract.ErrInvalid)
	}
	if err := m.acceptTimeV1(ctx, now); err != nil {
		return framecache.EntryV1{}, err
	}
	return m.frame.get(ctx, key, now)
}
func (m *MemoryFacadeV1) InspectFrameAttemptV1(ctx context.Context, attempt string, now int64) (framecache.EntryV1, error) {
	if strings.TrimSpace(attempt) == "" || len(attempt) > 256 {
		return framecache.EntryV1{}, fmt.Errorf("%w: frame cache attempt", contract.ErrInvalid)
	}
	if err := m.acceptTimeV1(ctx, now); err != nil {
		return framecache.EntryV1{}, err
	}
	return m.frame.inspect(ctx, attempt, now)
}
func (m *MemoryFacadeV1) InvalidateFrameV1(ctx context.Context, key contract.ContextFrameCacheKeyV1, next uint64) error {
	if m == nil {
		return fmt.Errorf("%w: local cache facade", contract.ErrInvalid)
	}
	return m.frame.invalidate(ctx, key, next, contract.CacheInvalidationFrameChangedV1)
}

func (m *MemoryFacadeV1) PutProjectionV1(ctx context.Context, attempt string, entry projectioncache.EntryV1, now int64) (projectioncache.EntryV1, error) {
	if m == nil {
		return projectioncache.EntryV1{}, fmt.Errorf("%w: local cache facade", contract.ErrInvalid)
	}
	if strings.TrimSpace(attempt) == "" || len(attempt) > 256 || entry.Validate() != nil {
		return projectioncache.EntryV1{}, fmt.Errorf("%w: projection cache put", contract.ErrInvalid)
	}
	if err := validatePutTimeV1(now, entry.ExpiresUnixNano, m, m.projection.config.AttemptRecoveryNanos); err != nil {
		return projectioncache.EntryV1{}, err
	}
	if err := m.acceptTimeV1(ctx, now); err != nil {
		return projectioncache.EntryV1{}, err
	}
	return m.projection.put(ctx, attempt, entry, now)
}
func (m *MemoryFacadeV1) GetProjectionV1(ctx context.Context, key contract.ContextProjectionCacheKeyV1, now int64) (projectioncache.EntryV1, error) {
	if key.Validate() != nil {
		return projectioncache.EntryV1{}, fmt.Errorf("%w: projection cache key", contract.ErrInvalid)
	}
	if err := m.acceptTimeV1(ctx, now); err != nil {
		return projectioncache.EntryV1{}, err
	}
	return m.projection.get(ctx, key, now)
}
func (m *MemoryFacadeV1) InspectProjectionAttemptV1(ctx context.Context, attempt string, now int64) (projectioncache.EntryV1, error) {
	if strings.TrimSpace(attempt) == "" || len(attempt) > 256 {
		return projectioncache.EntryV1{}, fmt.Errorf("%w: projection cache attempt", contract.ErrInvalid)
	}
	if err := m.acceptTimeV1(ctx, now); err != nil {
		return projectioncache.EntryV1{}, err
	}
	return m.projection.inspect(ctx, attempt, now)
}
func (m *MemoryFacadeV1) InvalidateProjectionV1(ctx context.Context, key contract.ContextProjectionCacheKeyV1, next uint64, reason contract.ContextCacheInvalidationReasonV1) error {
	if m == nil {
		return fmt.Errorf("%w: local cache facade", contract.ErrInvalid)
	}
	return m.projection.invalidate(ctx, key, next, reason)
}

func (m *MemoryFacadeV1) ObserveLocalCacheV1(ctx context.Context, now int64) (LocalCacheTelemetryObservationV1, error) {
	if err := m.acceptTimeV1(ctx, now); err != nil {
		return LocalCacheTelemetryObservationV1{}, err
	}
	return LocalCacheTelemetryObservationV1{ConfigDigest: m.configDigest, Fragment: m.fragment.snapshotV1(), Frame: m.frame.snapshotV1(), Projection: m.projection.snapshotV1(), ObservedUnixNano: now}, nil
}

func fragmentOpsV1() cacheOpsV1[contract.ContextFragmentCacheKeyV1, fragmentcache.EntryV1] {
	return cacheOpsV1[contract.ContextFragmentCacheKeyV1, fragmentcache.EntryV1]{
		validateEntry: func(value fragmentcache.EntryV1) error { return value.Validate() },
		entryMeta:     func(value fragmentcache.EntryV1) (cacheMetaV1, error) { return fragmentKeyMetaV1(value.Key) },
		keyMeta:       fragmentKeyMetaV1, expires: func(value fragmentcache.EntryV1) int64 { return value.ExpiresUnixNano },
		valueDigest: func(value fragmentcache.EntryV1) contract.Digest { return value.ValueDigest },
	}
}
func frameOpsV1() cacheOpsV1[contract.ContextFrameCacheKeyV1, framecache.EntryV1] {
	return cacheOpsV1[contract.ContextFrameCacheKeyV1, framecache.EntryV1]{
		validateEntry: func(value framecache.EntryV1) error { return value.Validate() },
		entryMeta:     func(value framecache.EntryV1) (cacheMetaV1, error) { return frameKeyMetaV1(value.Key) },
		keyMeta:       frameKeyMetaV1, expires: func(value framecache.EntryV1) int64 { return value.ExpiresUnixNano },
		valueDigest: func(value framecache.EntryV1) contract.Digest { return value.ValueDigest },
	}
}
func projectionOpsV1() cacheOpsV1[contract.ContextProjectionCacheKeyV1, projectioncache.EntryV1] {
	return cacheOpsV1[contract.ContextProjectionCacheKeyV1, projectioncache.EntryV1]{
		validateEntry: func(value projectioncache.EntryV1) error { return value.Validate() },
		entryMeta:     func(value projectioncache.EntryV1) (cacheMetaV1, error) { return projectionKeyMetaV1(value.Key) },
		keyMeta:       projectionKeyMetaV1, expires: func(value projectioncache.EntryV1) int64 { return value.ExpiresUnixNano },
		valueDigest: func(value projectioncache.EntryV1) contract.Digest { return value.ValueDigest },
	}
}

func fragmentKeyMetaV1(key contract.ContextFragmentCacheKeyV1) (cacheMetaV1, error) {
	if key.Validate() != nil {
		return cacheMetaV1{}, fmt.Errorf("%w: fragment cache key", contract.ErrInvalid)
	}
	copy := key
	copy.InvalidationGeneration = 0
	copy.Digest = ""
	partition, err := contract.DigestJSON(struct {
		Domain string
		Key    contract.ContextFragmentCacheKeyV1
	}{"praxis.context/fragment-cache-partition-v1", copy})
	return cacheMetaV1{key: key.Digest, partition: partition, generation: key.InvalidationGeneration}, err
}
func frameKeyMetaV1(key contract.ContextFrameCacheKeyV1) (cacheMetaV1, error) {
	if key.Validate() != nil {
		return cacheMetaV1{}, fmt.Errorf("%w: frame cache key", contract.ErrInvalid)
	}
	copy := key
	copy.InvalidationGeneration = 0
	copy.Digest = ""
	partition, err := contract.DigestJSON(struct {
		Domain string
		Key    contract.ContextFrameCacheKeyV1
	}{"praxis.context/frame-cache-partition-v1", copy})
	return cacheMetaV1{key: key.Digest, partition: partition, generation: key.InvalidationGeneration}, err
}
func projectionKeyMetaV1(key contract.ContextProjectionCacheKeyV1) (cacheMetaV1, error) {
	if key.Validate() != nil {
		return cacheMetaV1{}, fmt.Errorf("%w: projection cache key", contract.ErrInvalid)
	}
	copy := key
	copy.InvalidationGeneration = 0
	copy.Digest = ""
	partition, err := contract.DigestJSON(struct {
		Domain string
		Key    contract.ContextProjectionCacheKeyV1
	}{"praxis.context/projection-cache-partition-v1", copy})
	return cacheMetaV1{key: key.Digest, partition: partition, generation: key.InvalidationGeneration}, err
}

func validInvalidationReasonV1(reason contract.ContextCacheInvalidationReasonV1) bool {
	switch reason {
	case contract.CacheInvalidationFrameChangedV1, contract.CacheInvalidationManifestChangedV1, contract.CacheInvalidationGenerationChangedV1, contract.CacheInvalidationFragmentChangedV1, contract.CacheInvalidationPromptChangedV1, contract.CacheInvalidationRecipeChangedV1, contract.CacheInvalidationDisclosureChangedV1, contract.CacheInvalidationScopeChangedV1, contract.CacheInvalidationTTLExpiredV1, contract.CacheInvalidationOwnerCurrentUnknownV1:
		return true
	default:
		return false
	}
}

func validatePutTimeV1(now, expires int64, facade *MemoryFacadeV1, recoveryNanos int64) error {
	if facade == nil || now <= 0 {
		return fmt.Errorf("%w: local cache put time", contract.ErrInvalid)
	}
	if now >= expires {
		return fmt.Errorf("%w: local cache entry", contract.ErrExpired)
	}
	if now > math.MaxInt64-recoveryNanos {
		return fmt.Errorf("%w: local cache recovery deadline", contract.ErrLimitExceeded)
	}
	return nil
}

func cloneJSONV1[T any](value T) (T, error) {
	var copy T
	payload, err := json.Marshal(value)
	if err != nil {
		return copy, fmt.Errorf("%w: local cache clone", contract.ErrInvalid)
	}
	if err = json.Unmarshal(payload, &copy); err != nil {
		return copy, fmt.Errorf("%w: local cache clone", contract.ErrInvalid)
	}
	return copy, nil
}

func contextErrorV1(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	return ctx.Err()
}

var _ ContextLocalCacheFacadeV1 = (*MemoryFacadeV1)(nil)
