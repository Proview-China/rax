package kernel

import (
	"fmt"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type CacheIndex struct {
	mu      sync.RWMutex
	entries map[string]contract.CacheEntry
}

func NewCacheIndex() *CacheIndex {
	return &CacheIndex{entries: make(map[string]contract.CacheEntry)}
}

func (c *CacheIndex) CAS(expectedRevision uint64, entry contract.CacheEntry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	current, exists := c.entries[entry.ID]
	if !exists {
		if expectedRevision != 0 || entry.Revision != 1 {
			return fmt.Errorf("%w: cache entry create revision", contract.ErrConflict)
		}
		c.entries[entry.ID] = entry
		return nil
	}
	if expectedRevision == 0 && entry == current {
		return nil
	}
	if expectedRevision != current.Revision || entry.Revision != current.Revision+1 {
		return fmt.Errorf("%w: cache entry revision", contract.ErrConflict)
	}
	if entry.PartitionDigest != current.PartitionDigest || entry.KeyDigest != current.KeyDigest || entry.PrefixDigest != current.PrefixDigest || entry.AuthorityDigest != current.AuthorityDigest || entry.CreatedUnixNano != current.CreatedUnixNano || entry.InvalidationGeneration < current.InvalidationGeneration {
		return fmt.Errorf("%w: immutable cache entry fields", contract.ErrConflict)
	}
	if current.State != contract.CacheEntryCurrent || entry.State == contract.CacheEntryCurrent {
		return fmt.Errorf("%w: cache entry state transition", contract.ErrConflict)
	}
	c.entries[entry.ID] = entry
	return nil
}

func (c *CacheIndex) Inspect(id string, partitionDigest, keyDigest, authorityDigest contract.Digest, invalidationGeneration uint64, now int64) (contract.CacheAccessFact, error) {
	if id == "" || partitionDigest.Validate() != nil || keyDigest.Validate() != nil || authorityDigest.Validate() != nil || invalidationGeneration == 0 || now <= 0 {
		return contract.CacheAccessFact{}, fmt.Errorf("%w: cache inspect request", contract.ErrInvalid)
	}
	c.mu.RLock()
	entry, ok := c.entries[id]
	c.mu.RUnlock()
	if !ok {
		return contract.CacheAccessFact{}, fmt.Errorf("%w: cache entry", contract.ErrUnknown)
	}
	if now < entry.CreatedUnixNano {
		return contract.CacheAccessFact{}, fmt.Errorf("%w: cache entry not yet current", contract.ErrConflict)
	}
	if entry.State != contract.CacheEntryCurrent || now >= entry.ExpiresUnixNano {
		return contract.CacheAccessFact{}, fmt.Errorf("%w: cache entry not current", contract.ErrExpired)
	}
	if entry.PartitionDigest != partitionDigest || entry.KeyDigest != keyDigest || entry.AuthorityDigest != authorityDigest || entry.InvalidationGeneration != invalidationGeneration {
		return contract.CacheAccessFact{}, fmt.Errorf("%w: cache inspect binding", contract.ErrConflict)
	}
	digest, err := contract.DigestJSON(entry)
	if err != nil {
		return contract.CacheAccessFact{}, err
	}
	return contract.CacheAccessFact{EntryRef: contract.FactRef{ID: entry.ID, Revision: entry.Revision, Digest: digest}, PartitionDigest: entry.PartitionDigest, AuthorityDigest: entry.AuthorityDigest, InspectedUnixNano: now}, nil
}

func (c *CacheIndex) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// ObserveUsage validates telemetry only. Usage never creates or mutates an
// entry and therefore can never establish a cache hit.
func (c *CacheIndex) ObserveUsage(observation contract.ProviderCacheUsageObservation) error {
	return observation.Validate()
}
