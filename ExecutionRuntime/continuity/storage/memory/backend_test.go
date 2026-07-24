package memory_test

import (
	"context"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
)

func TestConcurrentProjectionIsIdempotent(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}
	service, _ := domain.NewReferenceTimeline(backend, clock, time.Minute)
	candidate := testkit.Candidate(1, 1, contract.TrustObservation)
	var failures atomic.Int32
	var created atomic.Int32
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, duplicate, err := service.Project(ctx, candidate)
			if err != nil {
				failures.Add(1)
				return
			}
			if !duplicate {
				created.Add(1)
			}
		}()
	}
	wg.Wait()
	if failures.Load() != 0 || created.Load() != 1 {
		t.Fatalf("failures=%d created=%d", failures.Load(), created.Load())
	}
	records, err := backend.ListLedgerScope(ctx, "ledger-scope-1")
	if err != nil || len(records) != 1 {
		t.Fatalf("records=%d err=%v", len(records), err)
	}
}

func TestJournalCASOnlyOneRevisionWins(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	j := contract.WriteJournal{
		JournalID: "journal-1", ObjectID: "object-1", ObjectDigest: "object-digest",
		ManifestDigest: "manifest-digest", State: contract.JournalProposed,
		Revision: 1, UpdatedUnixNano: 1,
	}
	if err := backend.CreateJournal(ctx, j); err != nil {
		t.Fatal(err)
	}
	next := j
	next.State = contract.JournalMetadataPending
	next.Revision = 2
	next.UpdatedUnixNano = 2
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if backend.CASJournal(ctx, 1, next) == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("expected one CAS winner, got %d", successes.Load())
	}
}

func TestQualityJournalLostReplyRequiresInspectBeforeProgress(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	journal := contract.WriteJournal{
		JournalID: "journal-lost-reply", ObjectID: "object-1", ObjectDigest: "object-digest",
		ManifestDigest: "manifest-digest", State: contract.JournalProposed,
		Revision: 1, UpdatedUnixNano: 1,
	}
	if err := backend.CreateJournal(ctx, journal); err != nil {
		t.Fatal(err)
	}
	next := journal
	next.State = contract.JournalMetadataPending
	next.Revision = 2
	next.UpdatedUnixNano = 2
	if err := backend.CASJournal(ctx, 1, next); err != nil {
		t.Fatal(err)
	}
	// The durable CAS succeeded but its reply is considered lost. Reissuing the
	// old CAS must fail; only Inspect reveals the exact persisted phase.
	if err := backend.CASJournal(ctx, 1, next); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("blind retry after lost reply was accepted: %v", err)
	}
	inspected, err := backend.InspectJournal(ctx, journal.JournalID)
	if err != nil || inspected.State != contract.JournalMetadataPending || inspected.Revision != 2 {
		t.Fatalf("inspect did not recover exact phase: %#v err=%v", inspected, err)
	}
}

func TestQualityConcurrentRetentionCASHasSingleWinner(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	current := contract.RetentionFact{
		ObjectID: "object-1", PolicyRef: "policy-1", Classification: "internal",
		State: contract.RetentionActive, Revision: 1, UpdatedUnixNano: 1,
	}
	if err := backend.CreateRetention(ctx, current); err != nil {
		t.Fatal(err)
	}
	next, err := contract.AdvanceRetention(current, contract.RetentionExpired, "expiry-fact-1")
	if err != nil {
		t.Fatal(err)
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if backend.CASRetention(ctx, 1, next) == nil {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("retention CAS winners=%d want=1", winners.Load())
	}
	inspected, err := backend.InspectRetention(ctx, current.ObjectID)
	if err != nil || inspected.State != contract.RetentionExpired || inspected.Revision != 2 {
		t.Fatalf("retention inspect=%#v err=%v", inspected, err)
	}
}

func TestTimelineTombstoneConcurrentConflictKeepsHistoricalEventImmutable(t *testing.T) {
	ctx := context.Background()
	backend := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 6, 30, 0, 0, time.UTC)}
	service, _ := domain.NewReferenceTimeline(backend, clock, time.Minute)
	candidate := testkit.Candidate(1, 1, contract.TrustObservation)
	before, _, err := service.Project(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}

	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := range 64 {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			suffix := strconv.Itoa(i)
			request := contract.TimelineProjectionTombstoneRequestV1{
				TombstoneID: "tombstone-" + suffix, EvidenceRecordRef: candidate.Evidence.RecordRef,
				SourceTombstoneRef: "source-tombstone-" + suffix, PolicyBasisRef: "policy-1",
				IdempotencyKey: "request-" + suffix,
			}
			if _, duplicate, err := service.CreateTombstone(ctx, request); err == nil && !duplicate {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("tombstone winners=%d want=1", winners.Load())
	}
	after, err := service.Inspect(ctx, candidate.Evidence.RecordRef)
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("historical Event changed: before=%#v after=%#v err=%v", before, after, err)
	}
	page, err := service.Query(ctx, contract.TimelineQuery{
		LedgerScopeDigest: candidate.Evidence.LedgerScopeDigest, IncludeTombstoned: true,
		AuthorityWatermark: "authority-1", PolicyWatermark: "policy-1", PageLimit: 10,
	})
	if err != nil || len(page.Records) != 1 || page.Records[0].Visibility != "tombstoned" || page.Records[0].TombstoneRef == "" {
		t.Fatalf("visibility overlay missing: %#v err=%v", page, err)
	}
}
