package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/domain"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/memory"
	continuitysqlite "github.com/Proview-China/rax/ExecutionRuntime/continuity/storage/sqlite"
	_ "modernc.org/sqlite"
)

func TestStoreCrossStoreJournalRecoversAfterMetadataReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "continuity.db")
	store := openStore(t, path)
	content := memory.New()
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)}
	fired := false
	manager, err := domain.NewContentManager(store, content, clock, 4, func(state contract.JournalState, _ contract.WriteJournal) error {
		if state == contract.JournalContentStaged && !fired {
			fired = true
			return errors.New("lost metadata reply")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("durable-content-journal")
	request := domain.PutObjectRequest{
		JournalID: "journal-1", ObjectID: "object-1", SchemaVersion: "content/v1",
		Classification: "sensitive", OwnerID: "continuity", ScopeDigest: "scope-1",
		RetentionPolicyRef: "retention-1", Compression: "identity",
		EncryptionRef: "envelope-1", Data: data,
	}
	manifest, journal, err := manager.Put(ctx, request)
	if !contract.HasCode(err, contract.ErrCrossStoreIndeterminate) || journal.State != contract.JournalContentStaged {
		t.Fatalf("fault cut was not durable: journal=%#v err=%v", journal, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openStore(t, path)
	defer store.Close()
	manager, _ = domain.NewContentManager(store, content, clock, 4, nil)
	recovered, err := manager.Recover(ctx, journal.JournalID, data)
	if err != nil || recovered.State != contract.JournalClosed {
		t.Fatalf("recovery after reopen failed: journal=%#v err=%v", recovered, err)
	}
	got, reopenedManifest, err := manager.Read(ctx, manifest.ObjectID)
	if err != nil || string(got) != string(data) || reopenedManifest.Digest != manifest.Digest {
		t.Fatalf("durable metadata/content relation failed: got=%q manifest=%#v err=%v", got, reopenedManifest, err)
	}
	got[0] ^= 0xff
	again, _, err := manager.Read(ctx, manifest.ObjectID)
	if err != nil || string(again) != string(data) {
		t.Fatalf("read leaked mutable bytes: got=%q err=%v", again, err)
	}
}

func TestStoreTimelineHistoryTombstoneAndSourceConflictSurviveReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "continuity.db")
	store := openStore(t, path)
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)}
	timeline, _ := domain.NewReferenceTimeline(store, clock, time.Minute)
	candidate := testkit.Candidate(1, 1, contract.TrustObservation)
	record, duplicate, err := timeline.Project(ctx, candidate)
	if err != nil || duplicate {
		t.Fatalf("project: duplicate=%v err=%v", duplicate, err)
	}
	if _, duplicate, err = timeline.Project(ctx, candidate); err != nil || !duplicate {
		t.Fatalf("lost projection reply did not inspect exact event: duplicate=%v err=%v", duplicate, err)
	}
	conflict := testkit.Candidate(2, 1, contract.TrustObservation)
	if _, _, err := timeline.Project(ctx, conflict); !contract.HasCode(err, contract.ErrEvidenceConflict) {
		t.Fatalf("same source changed content was accepted: %v", err)
	}
	fact, duplicate, err := timeline.CreateTombstone(ctx, contract.TimelineProjectionTombstoneRequestV1{
		TombstoneID: "tombstone-1", EvidenceRecordRef: record.EvidenceRecordRef,
		SourceTombstoneRef: "retention-fact-1", PolicyBasisRef: "policy-1",
		IdempotencyKey: "tombstone-request-1",
	})
	if err != nil || duplicate {
		t.Fatalf("tombstone: fact=%#v duplicate=%v err=%v", fact, duplicate, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openStore(t, path)
	defer store.Close()
	historical, err := store.InspectByEvidence(ctx, record.EvidenceRecordRef)
	if err != nil || historical.Visibility != "visible" || historical.TombstoneRef != "" {
		t.Fatalf("historical event was mutated: %#v err=%v", historical, err)
	}
	view, err := store.ListLedgerScope(ctx, record.LedgerScopeDigest)
	if err != nil || len(view) != 1 || view[0].Visibility != "tombstoned" || view[0].TombstoneRef != fact.TombstoneID {
		t.Fatalf("visibility overlay was not durable: %#v err=%v", view, err)
	}
	inspected, err := store.InspectTombstone(ctx, fact.TombstoneID)
	if err != nil || inspected.Digest != fact.Digest {
		t.Fatalf("tombstone fact was not durable: %#v err=%v", inspected, err)
	}
}

func TestStoreTimelineTypedObjectQuerySurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "continuity.db")
	store := openStore(t, path)
	clock := &testkit.Clock{Time: time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)}
	timeline, _ := domain.NewReferenceTimeline(store, clock, time.Minute)
	candidate := testkit.Candidate(1, 1, contract.TrustObservation)
	candidate.ObjectRefs = []string{"action-1", "artifact-1", "checkpoint-1", "effect-1", "review-case-1", "step-1", "turn-1"}
	candidate.Digest, _ = candidate.CanonicalDigest()
	if _, _, err := timeline.Project(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openStore(t, path)
	defer store.Close()
	timeline, _ = domain.NewReferenceTimeline(store, clock, time.Minute)
	page, err := timeline.Query(ctx, contract.TimelineQuery{
		LedgerScopeDigest: "ledger-scope-1", TurnRef: "turn-1", StepRef: "step-1",
		ActionRef: "action-1", ArtifactRef: "artifact-1", EffectRef: "effect-1",
		ReviewCaseRef: "review-case-1", CheckpointRef: "checkpoint-1",
		AuthorityWatermark: "authority-1", PolicyWatermark: "policy-1", PageLimit: 10,
	})
	if err != nil || len(page.Records) != 1 || page.Records[0].EvidenceRecordRef != candidate.Evidence.RecordRef {
		t.Fatalf("typed query did not survive reopen: page=%+v err=%v", page, err)
	}
}

func TestStoreRetentionCASIsSingleWinnerAndNoABA(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, filepath.Join(t.TempDir(), "continuity.db"))
	defer store.Close()
	created := contract.RetentionFact{
		ObjectID: "object-1", PolicyRef: "policy-1", Classification: "internal",
		State: contract.RetentionActive, Revision: 1, UpdatedUnixNano: 1,
	}
	if err := store.CreateRetention(ctx, created); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRetention(ctx, created); err != nil {
		t.Fatalf("lost create reply must be exact-idempotent: %v", err)
	}
	var winners atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			next := created
			next.State = contract.RetentionExpired
			next.PreviousState = contract.RetentionActive
			next.TransitionEvidenceRef = "expiry-" + decimal(i)
			next.Revision = 2
			next.UpdatedUnixNano = int64(i + 2)
			err := store.CASRetention(ctx, 1, next)
			if err == nil {
				winners.Add(1)
			} else if !contract.HasCode(err, contract.ErrRevisionConflict) {
				unexpected.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if winners.Load() != 1 || unexpected.Load() != 0 {
		t.Fatalf("CAS closure failed: winners=%d unexpected=%d", winners.Load(), unexpected.Load())
	}
	current, err := store.InspectRetention(ctx, created.ObjectID)
	if err != nil || current.Revision != 2 {
		t.Fatalf("current retention: %#v err=%v", current, err)
	}
	if err := store.CASRetention(ctx, 1, current); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("stale revision enabled ABA: %v", err)
	}
}

func TestStoreRejectsUnsupportedSchemaVersion(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "continuity.db")
	store := openStore(t, path)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// The production opener must fail closed instead of guessing migrations.
	db := openRaw(t, path)
	if _, err := db.ExecContext(ctx, "PRAGMA user_version=999"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := continuitysqlite.Open(ctx, path); !contract.HasCode(err, contract.ErrUnsupported) {
		t.Fatalf("unsupported schema was accepted: %v", err)
	}
}

func openStore(t *testing.T, path string) *continuitysqlite.Store {
	t.Helper()
	store, err := continuitysqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func decimal(value int) string {
	if value == 0 {
		return "0"
	}
	var buffer [20]byte
	i := len(buffer)
	for value > 0 {
		i--
		buffer[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buffer[i:])
}
