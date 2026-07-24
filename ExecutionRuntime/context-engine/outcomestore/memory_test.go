package outcomestore_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/outcomestore"
)

func TestMemoryPutOnceInspectAndConflictV1(t *testing.T) {
	store := outcomestore.NewMemory()
	outcome := storeOutcomeFixtureV1()
	ref, err := store.PutContextOutcomeV1(context.Background(), outcome)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := store.PutContextOutcomeV1(context.Background(), outcome)
	if err != nil || replayed != ref {
		t.Fatalf("exact replay: %#v %v", replayed, err)
	}
	inspected, err := store.InspectContextOutcomeV1(context.Background(), ref)
	if err != nil || inspected.ID != outcome.ID {
		t.Fatalf("inspect: %#v %v", inspected, err)
	}
	drift := outcome
	drift.Metrics.CostMicros++
	if _, err := store.PutContextOutcomeV1(context.Background(), drift); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("same ID drift accepted: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	other := outcome
	other.ID = "outcome-canceled"
	if _, err := store.PutContextOutcomeV1(canceled, other); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel not preserved: %v", err)
	}
}

func TestMemory64ConcurrentExactPutHasOneImmutableFactV1(t *testing.T) {
	store := outcomestore.NewMemory()
	outcome := storeOutcomeFixtureV1()
	var wg sync.WaitGroup
	refs := make(chan contract.FactRef, 64)
	errs := make(chan error, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ref, err := store.PutContextOutcomeV1(context.Background(), outcome)
			refs <- ref
			errs <- err
		}()
	}
	wg.Wait()
	close(refs)
	close(errs)
	var first contract.FactRef
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for ref := range refs {
		if first == (contract.FactRef{}) {
			first = ref
		} else if ref != first {
			t.Fatalf("non-deterministic refs: %#v %#v", first, ref)
		}
	}
	if _, err := store.InspectContextOutcomeV1(context.Background(), first); err != nil {
		t.Fatal(err)
	}
}

func storeOutcomeFixtureV1() contract.ContextOutcomeFactV1 {
	ref := func(id string) contract.FactRef { return contract.FactRef{ID: id, Revision: 1, Digest: testkit.D(id)} }
	return contract.ContextOutcomeFactV1{ContractVersion: contract.Version, ID: "outcome-store-1", Revision: 1, Execution: testkit.Execution(), FrameRef: ref("frame"), ManifestRef: ref("manifest"), RecipeRef: ref("recipe"), GenerationRef: ref("generation"), ModelAttemptObservationRef: ref("attempt"), ModelResponseObservationRef: ref("response"), ToolActionRefs: []contract.FactRef{}, UserCorrectionEvidence: []contract.EvidenceRef{}, TaskEvidenceRefs: []contract.FactRef{}, Metrics: contract.ContextOutcomeMetricsV1{InputTokens: 10, OutputTokens: 1, CacheEligiblePrefixTokens: 5, CacheReadTokens: 2, DynamicTokens: 3, LatencyNanos: 1}, EvaluationPolicyRef: ref("policy"), CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + int64(time.Minute)}
}
