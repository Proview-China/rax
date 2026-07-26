package projectioncache

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type EntryV1 struct {
	Key             contract.ContextProjectionCacheKeyV1         `json:"key"`
	Descriptor      contract.ContextFrameConsumptionDescriptorV1 `json:"descriptor"`
	ExpiresUnixNano int64                                        `json:"expires_unix_nano"`
	ValueDigest     contract.Digest                              `json:"value_digest"`
}

func (e EntryV1) digestValue() (contract.Digest, error) {
	copy := cloneEntryV1(e)
	copy.ValueDigest = ""
	return contract.DigestJSON(struct {
		Domain string  `json:"domain"`
		Value  EntryV1 `json:"value"`
	}{"praxis.context/projection-cache-entry-v1", copy})
}
func (e EntryV1) Validate() error {
	if e.Key.Validate() != nil || e.Descriptor.Validate() != nil || e.ExpiresUnixNano <= 0 || e.ValueDigest.Validate() != nil {
		return fmt.Errorf("%w: projection cache entry", contract.ErrInvalid)
	}
	ref, err := e.Descriptor.RefV1()
	if err != nil {
		return err
	}
	if ref != e.Key.DescriptorRef || e.Descriptor.TenantScopeDigest != e.Key.TenantScopeDigest || e.Descriptor.RunScopeDigest != e.Key.RunScopeDigest || e.Descriptor.DisclosureClass != e.Key.DisclosureClass || e.Descriptor.CacheHint.FrameFingerprint != e.Key.FrameFingerprint || e.ExpiresUnixNano > e.Descriptor.ExpiresUnixNano {
		return fmt.Errorf("%w: projection cache exact closure", contract.ErrConflict)
	}
	want, err := e.digestValue()
	if err != nil || want != e.ValueDigest {
		return fmt.Errorf("%w: projection cache entry digest", contract.ErrConflict)
	}
	return nil
}
func SealEntryV1(e EntryV1) (EntryV1, error) {
	e = cloneEntryV1(e)
	e.ValueDigest = ""
	d, err := e.digestValue()
	if err != nil {
		return EntryV1{}, err
	}
	e.ValueDigest = d
	return e, e.Validate()
}

type TelemetryV1 struct {
	Hits, Misses, Conflicts uint64
	Invalidations           map[contract.ContextCacheInvalidationReasonV1]uint64
}
type MemoryV1 struct {
	mu          sync.RWMutex
	entries     map[contract.Digest]EntryV1
	attempts    map[string]contract.Digest
	generations map[contract.Digest]uint64
	telemetry   TelemetryV1
}

func NewMemoryV1() *MemoryV1 {
	return &MemoryV1{entries: map[contract.Digest]EntryV1{}, attempts: map[string]contract.Digest{}, generations: map[contract.Digest]uint64{}, telemetry: TelemetryV1{Invalidations: map[contract.ContextCacheInvalidationReasonV1]uint64{}}}
}
func (m *MemoryV1) PutV1(ctx context.Context, attemptID string, entry EntryV1, now int64) (EntryV1, error) {
	if err := contextErrorV1(ctx); err != nil {
		return EntryV1{}, err
	}
	if strings.TrimSpace(attemptID) == "" || entry.Validate() != nil || now <= 0 {
		return EntryV1{}, fmt.Errorf("%w: projection cache put", contract.ErrInvalid)
	}
	if now >= entry.ExpiresUnixNano {
		return EntryV1{}, contract.ErrExpired
	}
	partition, err := partitionDigestV1(entry.Key)
	if err != nil {
		return EntryV1{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.attempts[attemptID]; ok {
		return EntryV1{}, fmt.Errorf("%w: inspect projection cache attempt", contract.ErrInspectOnly)
	}
	current, ok := m.generations[partition]
	if !ok {
		m.generations[partition] = entry.Key.InvalidationGeneration
		current = entry.Key.InvalidationGeneration
	}
	if current != entry.Key.InvalidationGeneration {
		m.telemetry.Conflicts++
		return EntryV1{}, contract.ErrConflict
	}
	if old, ok := m.entries[entry.Key.Digest]; ok {
		if old.ValueDigest != entry.ValueDigest {
			m.telemetry.Conflicts++
			return EntryV1{}, contract.ErrConflict
		}
		m.attempts[attemptID] = entry.Key.Digest
		return cloneEntryV1(old), nil
	}
	m.entries[entry.Key.Digest] = cloneEntryV1(entry)
	m.attempts[attemptID] = entry.Key.Digest
	return cloneEntryV1(entry), nil
}
func (m *MemoryV1) GetV1(ctx context.Context, key contract.ContextProjectionCacheKeyV1, now int64) (EntryV1, error) {
	if err := contextErrorV1(ctx); err != nil {
		return EntryV1{}, err
	}
	if key.Validate() != nil || now <= 0 {
		return EntryV1{}, contract.ErrInvalid
	}
	partition, err := partitionDigestV1(key)
	if err != nil {
		return EntryV1{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.entries[key.Digest]
	if !ok {
		m.telemetry.Misses++
		return EntryV1{}, contract.ErrNotFound
	}
	if m.generations[partition] != key.InvalidationGeneration {
		m.telemetry.Conflicts++
		return EntryV1{}, contract.ErrConflict
	}
	if now >= entry.ExpiresUnixNano {
		m.telemetry.Misses++
		return EntryV1{}, contract.ErrExpired
	}
	if entry.Validate() != nil {
		m.telemetry.Conflicts++
		return EntryV1{}, contract.ErrConflict
	}
	m.telemetry.Hits++
	return cloneEntryV1(entry), nil
}
func (m *MemoryV1) InspectAttemptV1(ctx context.Context, attemptID string) (EntryV1, error) {
	if err := contextErrorV1(ctx); err != nil {
		return EntryV1{}, err
	}
	m.mu.RLock()
	key, ok := m.attempts[attemptID]
	entry, found := m.entries[key]
	m.mu.RUnlock()
	if !ok || !found {
		return EntryV1{}, contract.ErrNotFound
	}
	return cloneEntryV1(entry), nil
}
func (m *MemoryV1) InvalidateV1(ctx context.Context, key contract.ContextProjectionCacheKeyV1, next uint64, reason contract.ContextCacheInvalidationReasonV1) error {
	if err := contextErrorV1(ctx); err != nil {
		return err
	}
	if key.Validate() != nil || next != key.InvalidationGeneration+1 || !validReasonV1(reason) {
		return contract.ErrInvalid
	}
	partition, err := partitionDigestV1(key)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.generations[partition] != key.InvalidationGeneration {
		m.telemetry.Conflicts++
		return contract.ErrConflict
	}
	m.generations[partition] = next
	m.telemetry.Invalidations[reason]++
	return nil
}

func validReasonV1(reason contract.ContextCacheInvalidationReasonV1) bool {
	switch reason {
	case contract.CacheInvalidationFrameChangedV1, contract.CacheInvalidationManifestChangedV1, contract.CacheInvalidationGenerationChangedV1, contract.CacheInvalidationFragmentChangedV1, contract.CacheInvalidationPromptChangedV1, contract.CacheInvalidationRecipeChangedV1, contract.CacheInvalidationDisclosureChangedV1, contract.CacheInvalidationScopeChangedV1, contract.CacheInvalidationTTLExpiredV1, contract.CacheInvalidationOwnerCurrentUnknownV1:
		return true
	default:
		return false
	}
}
func (m *MemoryV1) TelemetryV1() TelemetryV1 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v := m.telemetry
	v.Invalidations = map[contract.ContextCacheInvalidationReasonV1]uint64{}
	for k, n := range m.telemetry.Invalidations {
		v.Invalidations[k] = n
	}
	return v
}
func partitionDigestV1(k contract.ContextProjectionCacheKeyV1) (contract.Digest, error) {
	k.InvalidationGeneration = 0
	k.Digest = ""
	return contract.DigestJSON(struct {
		Domain string
		Key    contract.ContextProjectionCacheKeyV1
	}{"praxis.context/projection-cache-partition-v1", k})
}
func cloneEntryV1(e EntryV1) EntryV1 {
	e.Descriptor.FragmentRefs = append([]contract.FactRef{}, e.Descriptor.FragmentRefs...)
	e.Descriptor.PromptAssetRefs = append([]contract.PromptAssetRefV1{}, e.Descriptor.PromptAssetRefs...)
	e.Descriptor.CacheHint.InvalidationReasons = append([]contract.ContextCacheInvalidationReasonV1{}, e.Descriptor.CacheHint.InvalidationReasons...)
	if e.Descriptor.SemiStable != nil {
		x := *e.Descriptor.SemiStable
		e.Descriptor.SemiStable = &x
	}
	if e.Descriptor.CacheHint.SemiStablePrefixFingerprint != nil {
		x := *e.Descriptor.CacheHint.SemiStablePrefixFingerprint
		e.Descriptor.CacheHint.SemiStablePrefixFingerprint = &x
	}
	return e
}
func contextErrorV1(ctx context.Context) error {
	if ctx == nil {
		return contract.ErrInvalid
	}
	return ctx.Err()
}
