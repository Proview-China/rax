package kernel

import (
	"sync"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestProviderUsageIsNotCacheHit(t *testing.T) {
	index := NewCacheIndex()
	usage := contract.ProviderCacheUsageObservation{ObservationID: "usage-1", ReadTokens: 500, WriteTokens: 100, ObservedUnixNano: testkit.Now}
	if err := index.ObserveUsage(usage); err != nil {
		t.Fatal(err)
	}
	if index.Len() != 0 {
		t.Fatal("provider usage created authoritative cache state")
	}
	_, err := index.Inspect("entry-1", testkit.D("partition"), testkit.D("key"), testkit.D("authority"), 1, testkit.Now)
	if err == nil {
		t.Fatal("usage observation was treated as a cache hit")
	}
}

func TestCacheInspectAndCASConflict(t *testing.T) {
	index := NewCacheIndex()
	entry := cacheEntry()
	if err := index.CAS(0, entry); err != nil {
		t.Fatal(err)
	}
	fact, err := index.Inspect(entry.ID, entry.PartitionDigest, entry.KeyDigest, entry.AuthorityDigest, entry.InvalidationGeneration, testkit.Now)
	if err != nil || fact.EntryRef.ID != entry.ID {
		t.Fatalf("inspect=%#v err=%v", fact, err)
	}
	invalidated := entry
	invalidated.Revision = 2
	invalidated.State = contract.CacheEntryInvalidated
	invalidated.InvalidationGeneration = 2
	var successes int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if index.CAS(1, invalidated) == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if successes != 1 {
		t.Fatalf("CAS successes=%d want 1", successes)
	}
	if _, err := index.Inspect(entry.ID, entry.PartitionDigest, entry.KeyDigest, entry.AuthorityDigest, 2, testkit.Now); err == nil {
		t.Fatal("invalidated entry remained a hit")
	}
}

func TestCacheCurrentnessBoundaries(t *testing.T) {
	index := NewCacheIndex()
	entry := cacheEntry()
	if err := index.CAS(0, entry); err != nil {
		t.Fatal(err)
	}
	if _, err := index.Inspect(entry.ID, entry.PartitionDigest, entry.KeyDigest, entry.AuthorityDigest, entry.InvalidationGeneration, entry.CreatedUnixNano-1); err == nil {
		t.Fatal("entry was current before creation")
	}
	if _, err := index.Inspect(entry.ID, entry.PartitionDigest, entry.KeyDigest, entry.AuthorityDigest, entry.InvalidationGeneration, entry.CreatedUnixNano); err != nil {
		t.Fatalf("entry not current at creation boundary: %v", err)
	}
	if _, err := index.Inspect(entry.ID, entry.PartitionDigest, entry.KeyDigest, entry.AuthorityDigest, entry.InvalidationGeneration, entry.ExpiresUnixNano-1); err != nil {
		t.Fatalf("entry not current before expiry: %v", err)
	}
	if _, err := index.Inspect(entry.ID, entry.PartitionDigest, entry.KeyDigest, entry.AuthorityDigest, entry.InvalidationGeneration, entry.ExpiresUnixNano); err == nil {
		t.Fatal("entry current at expiry boundary")
	}
	if _, err := index.Inspect(entry.ID, entry.PartitionDigest, testkit.D("wrong-key"), entry.AuthorityDigest, entry.InvalidationGeneration, testkit.Now); err == nil {
		t.Fatal("wrong cache key was accepted")
	}
}

func cacheEntry() contract.CacheEntry {
	return contract.CacheEntry{
		ContractVersion: contract.Version, ID: "entry-1", Revision: 1,
		PartitionDigest: testkit.D("partition"), KeyDigest: testkit.D("key"), PrefixDigest: testkit.D("prefix"), AuthorityDigest: testkit.D("authority"),
		State: contract.CacheEntryCurrent, InvalidationGeneration: 1, CreatedUnixNano: testkit.Now - 100, ExpiresUnixNano: testkit.Now + 100,
	}
}
