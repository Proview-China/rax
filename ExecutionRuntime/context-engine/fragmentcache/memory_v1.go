package fragmentcache

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type EntryV1 struct {
	Key             contract.ContextFragmentCacheKeyV1 `json:"key"`
	Fragment        contract.ContextFragment           `json:"fragment"`
	ExpiresUnixNano int64                              `json:"expires_unix_nano"`
	ValueDigest     contract.Digest                    `json:"value_digest"`
}

func (e EntryV1) digestValue() (contract.Digest, error) {
	copy := e
	copy.ValueDigest = ""
	return contract.DigestJSON(struct {
		Domain string  `json:"domain"`
		Value  EntryV1 `json:"value"`
	}{"praxis.context/fragment-cache-entry-v1", copy})
}

func (e EntryV1) Validate() error {
	if e.Key.Validate() != nil || e.Fragment.Validate() != nil || e.Fragment.CandidateRef != e.Key.FragmentRef || e.Fragment.Content != e.Key.Content || e.ExpiresUnixNano <= 0 || e.ValueDigest.Validate() != nil {
		return fmt.Errorf("%w: fragment cache entry", contract.ErrInvalid)
	}
	want, err := e.digestValue()
	if err != nil || want != e.ValueDigest {
		return fmt.Errorf("%w: fragment cache entry digest", contract.ErrConflict)
	}
	return nil
}

func SealEntryV1(e EntryV1) (EntryV1, error) {
	e.Key.PromptAssetRefs = append([]contract.PromptAssetRefV1{}, e.Key.PromptAssetRefs...)
	e.ValueDigest = ""
	digest, err := e.digestValue()
	if err != nil {
		return EntryV1{}, err
	}
	e.ValueDigest = digest
	return e, e.Validate()
}

type MemoryV1 struct {
	mu          sync.RWMutex
	entries     map[contract.Digest]EntryV1
	attempts    map[string]contract.Digest
	generations map[contract.Digest]uint64
}

func NewMemoryV1() *MemoryV1 {
	return &MemoryV1{
		entries:     make(map[contract.Digest]EntryV1),
		attempts:    make(map[string]contract.Digest),
		generations: make(map[contract.Digest]uint64),
	}
}

func (m *MemoryV1) PutV1(ctx context.Context, attemptID string, entry EntryV1, now int64) (EntryV1, error) {
	if err := contextErrorV1(ctx); err != nil {
		return EntryV1{}, err
	}
	if strings.TrimSpace(attemptID) == "" || len(attemptID) > 256 || entry.Validate() != nil || now <= 0 {
		return EntryV1{}, fmt.Errorf("%w: fragment cache put", contract.ErrInvalid)
	}
	if now >= entry.ExpiresUnixNano {
		return EntryV1{}, fmt.Errorf("%w: fragment cache entry", contract.ErrExpired)
	}
	partition, err := fragmentPartitionDigestV1(entry.Key)
	if err != nil {
		return EntryV1{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.attempts[attemptID]; exists {
		return EntryV1{}, fmt.Errorf("%w: inspect fragment cache attempt %s", contract.ErrInspectOnly, attemptID)
	}
	current, exists := m.generations[partition]
	if !exists {
		m.generations[partition] = entry.Key.InvalidationGeneration
		current = entry.Key.InvalidationGeneration
	}
	if current != entry.Key.InvalidationGeneration {
		return EntryV1{}, fmt.Errorf("%w: fragment cache invalidation generation", contract.ErrConflict)
	}
	if existing, ok := m.entries[entry.Key.Digest]; ok {
		if existing.ValueDigest != entry.ValueDigest {
			return EntryV1{}, fmt.Errorf("%w: fragment cache same key different value", contract.ErrConflict)
		}
		m.attempts[attemptID] = entry.Key.Digest
		return cloneEntryV1(existing), nil
	}
	stored := cloneEntryV1(entry)
	m.entries[entry.Key.Digest] = stored
	m.attempts[attemptID] = entry.Key.Digest
	return cloneEntryV1(stored), nil
}

func (m *MemoryV1) GetV1(ctx context.Context, key contract.ContextFragmentCacheKeyV1, now int64) (EntryV1, error) {
	if err := contextErrorV1(ctx); err != nil {
		return EntryV1{}, err
	}
	if key.Validate() != nil || now <= 0 {
		return EntryV1{}, fmt.Errorf("%w: fragment cache get", contract.ErrInvalid)
	}
	partition, err := fragmentPartitionDigestV1(key)
	if err != nil {
		return EntryV1{}, err
	}
	m.mu.RLock()
	entry, ok := m.entries[key.Digest]
	current := m.generations[partition]
	m.mu.RUnlock()
	if !ok {
		return EntryV1{}, fmt.Errorf("%w: fragment cache entry", contract.ErrNotFound)
	}
	if current != key.InvalidationGeneration {
		return EntryV1{}, fmt.Errorf("%w: fragment cache invalidation generation", contract.ErrConflict)
	}
	if now >= entry.ExpiresUnixNano {
		return EntryV1{}, fmt.Errorf("%w: fragment cache entry", contract.ErrExpired)
	}
	if entry.Validate() != nil {
		return EntryV1{}, fmt.Errorf("%w: fragment cache stored entry", contract.ErrConflict)
	}
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
		return EntryV1{}, fmt.Errorf("%w: fragment cache attempt", contract.ErrNotFound)
	}
	return cloneEntryV1(entry), nil
}

func (m *MemoryV1) InvalidateV1(ctx context.Context, key contract.ContextFragmentCacheKeyV1, nextGeneration uint64) error {
	if err := contextErrorV1(ctx); err != nil {
		return err
	}
	if key.Validate() != nil || nextGeneration != key.InvalidationGeneration+1 {
		return fmt.Errorf("%w: fragment cache invalidate", contract.ErrInvalid)
	}
	partition, err := fragmentPartitionDigestV1(key)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.generations[partition]
	if !ok || current != key.InvalidationGeneration {
		return fmt.Errorf("%w: fragment cache invalidation CAS", contract.ErrConflict)
	}
	m.generations[partition] = nextGeneration
	return nil
}

func (m *MemoryV1) LenV1() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

func fragmentPartitionDigestV1(key contract.ContextFragmentCacheKeyV1) (contract.Digest, error) {
	copy := key
	copy.InvalidationGeneration = 0
	copy.Digest = ""
	return contract.DigestJSON(struct {
		Domain string                             `json:"domain"`
		Key    contract.ContextFragmentCacheKeyV1 `json:"key"`
	}{"praxis.context/fragment-cache-partition-v1", copy})
}

func cloneEntryV1(entry EntryV1) EntryV1 {
	copy := entry
	copy.Key.PromptAssetRefs = append([]contract.PromptAssetRefV1{}, entry.Key.PromptAssetRefs...)
	return copy
}

func contextErrorV1(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", contract.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
