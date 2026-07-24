package domain_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestQualityOutOfOrderAppendQueriesByLedgerSequence(t *testing.T) {
	ctx := context.Background()
	service, _ := domain.NewReferenceTimeline(memory.New(), &testkit.Clock{Time: time.Now()}, time.Minute)
	for _, sequence := range []uint64{9, 2, 7, 1, 8, 4, 6, 3, 5} {
		if _, _, err := service.Project(ctx, testkit.Candidate(sequence, sequence, contract.TrustObservation)); err != nil {
			t.Fatalf("append sequence %d: %v", sequence, err)
		}
	}
	page, err := service.Query(ctx, qualityQuery(20))
	if err != nil || len(page.Records) != 9 {
		t.Fatalf("query records=%d err=%v", len(page.Records), err)
	}
	for i, record := range page.Records {
		want := uint64(i + 1)
		if record.LedgerSequence != want {
			t.Fatalf("record %d sequence=%d want=%d", i, record.LedgerSequence, want)
		}
	}
}

func TestQualityLostProjectionReplyReplaysExactIdentity(t *testing.T) {
	ctx := context.Background()
	service, _ := domain.NewReferenceTimeline(memory.New(), &testkit.Clock{Time: time.Now()}, time.Minute)
	candidate := testkit.Candidate(1, 1, contract.TrustObservation)
	first, duplicate, err := service.Project(ctx, candidate)
	if err != nil || duplicate {
		t.Fatalf("first append duplicate=%v err=%v", duplicate, err)
	}
	// Simulate a durable append whose response was lost: the caller can only
	// replay/inspect the exact evidence identity, never mint another sequence.
	replayed, duplicate, err := service.Project(ctx, candidate)
	if err != nil || !duplicate || replayed.EvidenceRecordRef != first.EvidenceRecordRef || replayed.LedgerSequence != first.LedgerSequence {
		t.Fatalf("exact replay changed identity: first=%#v replayed=%#v duplicate=%v err=%v", first, replayed, duplicate, err)
	}
	inspected, err := service.Inspect(ctx, candidate.Evidence.RecordRef)
	if err != nil || inspected.Candidate.Digest != candidate.Digest {
		t.Fatalf("exact inspect failed: %#v err=%v", inspected, err)
	}
}

func TestQualityWatchGapClosesOnlyAfterMissingSequenceArrives(t *testing.T) {
	ctx := context.Background()
	service, _ := domain.NewReferenceTimeline(memory.New(), &testkit.Clock{Time: time.Now()}, time.Minute)
	if _, _, err := service.Project(ctx, testkit.Candidate(1, 1, contract.TrustObservation)); err != nil {
		t.Fatal(err)
	}
	query := qualityQuery(10)
	page, err := service.Query(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	query.Cursor = page.NextCursor
	if _, _, err := service.Project(ctx, testkit.Candidate(3, 3, contract.TrustObservation)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Watch(ctx, query); !contract.HasCode(err, contract.ErrWatchGap) {
		t.Fatalf("sequence gap was hidden: %v", err)
	}
	if _, _, err := service.Project(ctx, testkit.Candidate(2, 2, contract.TrustObservation)); err != nil {
		t.Fatal(err)
	}
	page, err = service.Watch(ctx, query)
	if err != nil || len(page.Records) != 2 || page.Records[0].LedgerSequence != 2 || page.Records[1].LedgerSequence != 3 {
		t.Fatalf("repaired watch page=%#v err=%v", page, err)
	}
}

func TestQualityConcurrentUniqueAppendHasNoLossAndStableOrder(t *testing.T) {
	ctx := context.Background()
	service, _ := domain.NewReferenceTimeline(memory.New(), &testkit.Clock{Time: time.Now()}, time.Minute)
	const total = 64
	errors := make(chan error, total)
	var wg sync.WaitGroup
	for sequence := uint64(1); sequence <= total; sequence++ {
		sequence := sequence
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := service.Project(ctx, testkit.Candidate(sequence, sequence, contract.TrustObservation))
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent append: %v", err)
		}
	}
	page, err := service.Query(ctx, qualityQuery(total))
	if err != nil || len(page.Records) != total {
		t.Fatalf("records=%d err=%v", len(page.Records), err)
	}
	for i, record := range page.Records {
		if record.LedgerSequence != uint64(i+1) {
			t.Fatalf("unstable order at %d: %d", i, record.LedgerSequence)
		}
	}
}

func qualityQuery(limit int) contract.TimelineQuery {
	return contract.TimelineQuery{
		LedgerScopeDigest: "ledger-scope-1", AuthorityWatermark: "authority-1",
		PolicyWatermark: "policy-1", PageLimit: limit,
	}
}
