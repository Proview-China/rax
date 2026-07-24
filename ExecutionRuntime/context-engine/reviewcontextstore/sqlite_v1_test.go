package reviewcontextstore

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	reviewcontract "github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func openSQLiteTestV1(t testing.TB, path string) *SQLiteV1 {
	t.Helper()
	store, err := OpenSQLiteV1(context.Background(), SQLiteConfigV1{Path: path, BusyTimeout: time.Second, MaxOpenConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func sqliteFixtureV1(t testing.TB, suffix string) (reviewcontract.ReviewerContextEnvelopeV1, reviewcontract.ReviewerContextEnvelopeV1) {
	t.Helper()
	now := time.Unix(1_933_000_000, 0)
	subject := testfixture.ReviewerContextSubjectV1(suffix)
	first, err := testfixture.ReviewerContextEnvelopeV1(subject, 1, now, now.Add(time.Hour), "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := testfixture.ReviewerContextEnvelopeV1(subject, 2, now, now.Add(time.Hour), "second")
	if err != nil {
		t.Fatal(err)
	}
	return first, second
}

func (s *SQLiteV1) failNextStageForTestV1() {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	s.failNextStage = true
}

func (s *SQLiteV1) loseNextReplyForTestV1() {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	s.loseNextReply = true
}

func TestReviewerContextSQLiteRestartHistoryAndDeepCloneV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviewer-context.db")
	first, second := sqliteFixtureV1(t, "restart")
	store := openSQLiteTestV1(t, path)
	if _, err := store.CommitV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: first}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Previous: &first.Ref, Value: second}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := openSQLiteTestV1(t, path)
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.IntegrityCheckV1(context.Background()); err != nil {
		t.Fatal(err)
	}
	ref, err := reopened.ResolveV1(context.Background(), second.Subject)
	if err != nil || ref != second.Ref {
		t.Fatalf("restart current: %#v %v", ref, err)
	}
	historical, err := reopened.InspectHistoricalV1(context.Background(), first.Ref)
	if err != nil || historical.Ref != first.Ref {
		t.Fatalf("restart history: %#v %v", historical.Ref, err)
	}
	historical.Materials[0].Content = "alias"
	again, err := reopened.InspectHistoricalV1(context.Background(), first.Ref)
	if err != nil || again.Materials[0].Content == "alias" {
		t.Fatal("restart historical inspect leaked mutable state")
	}
}

func TestReviewerContextSQLiteStageFailureAndLostReplyV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviewer-context.db")
	first, _ := sqliteFixtureV1(t, "fault")
	store := openSQLiteTestV1(t, path)
	t.Cleanup(func() { _ = store.Close() })
	store.failNextStageForTestV1()
	if receipt, err := store.CommitV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: first}); !core.HasCategory(err, core.ErrorUnavailable) || receipt.Ref.ID != "" {
		t.Fatalf("staged fault did not return zero/unavailable: %#v %v", receipt, err)
	}
	if _, err := store.InspectHistoricalV1(context.Background(), first.Ref); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged fault leaked history: %v", err)
	}
	if _, err := store.ResolveV1(context.Background(), first.Subject); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged fault leaked current: %v", err)
	}

	store.loseNextReplyForTestV1()
	if receipt, err := store.CommitV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: first}); !core.HasCategory(err, core.ErrorIndeterminate) || receipt.Ref.ID != "" {
		t.Fatalf("lost reply was not unknown/zero: %#v %v", receipt, err)
	}
	if exact, err := store.InspectHistoricalV1(context.Background(), first.Ref); err != nil || exact.Ref != first.Ref {
		t.Fatalf("lost reply exact historical recovery unavailable: %#v %v", exact.Ref, err)
	}
	if replay, err := store.CommitV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: first}); err != nil || replay.Created {
		t.Fatalf("canonical replay was not create-once: %#v %v", replay, err)
	}
}

func TestReviewerContextSQLiteBadCurrentAndABAHistoryFailClosedV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviewer-context.db")
	first, second := sqliteFixtureV1(t, "aba")
	store := openSQLiteTestV1(t, path)
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.CommitV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: first}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Previous: &first.Ref, Value: second}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE context_reviewer_context_current SET revision=?,envelope_digest=?,highest_revision=? WHERE tenant_id=? AND envelope_id=?`, uint64(first.Ref.Revision), string(first.Ref.Digest), uint64(second.Ref.Revision), string(first.Ref.TenantID), first.Ref.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveV1(context.Background(), first.Subject); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("ABA current was accepted: %v", err)
	}
	if _, err := store.InspectCurrentV1(context.Background(), first.Subject, first.Ref); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("ABA exact current was accepted: %v", err)
	}
	for _, ref := range []reviewcontract.ReviewerContextEnvelopeRefV1{first.Ref, second.Ref} {
		if _, err := store.InspectHistoricalV1(context.Background(), ref); err != nil {
			t.Fatalf("bad current contaminated exact history %d: %v", ref.Revision, err)
		}
	}
}

func TestReviewerContextSQLiteCorruptHistoricalRowsFailClosedV1(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
		arg   any
	}{
		{name: "row-digest", query: `UPDATE context_reviewer_context_history SET row_digest=?`, arg: string(core.DigestBytes([]byte("wrong")))},
		{name: "payload", query: `UPDATE context_reviewer_context_history SET payload=?`, arg: []byte(`{"contract_version":"duplicate","contract_version":"duplicate"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "reviewer-context.db")
			first, _ := sqliteFixtureV1(t, "corrupt-"+tc.name)
			store := openSQLiteTestV1(t, path)
			defer func() { _ = store.Close() }()
			if _, err := store.CommitV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: first}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.db.Exec(tc.query, tc.arg); err != nil {
				t.Fatal(err)
			}
			if _, err := store.InspectHistoricalV1(context.Background(), first.Ref); !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("corrupt row was accepted: %v", err)
			}
		})
	}
}

func TestReviewerContextSQLiteSchemaDigestDriftBlocksRestartV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviewer-context.db")
	store := openSQLiteTestV1(t, path)
	if _, err := store.db.Exec(`UPDATE context_reviewer_context_schema SET digest=? WHERE version=?`, string(core.DigestBytes([]byte("drift"))), reviewerContextSQLiteSchemaVersionV1); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := OpenSQLiteV1(context.Background(), SQLiteConfigV1{Path: path}); !core.HasCategory(err, core.ErrorConflict) || reopened != nil {
		if reopened != nil {
			_ = reopened.Close()
		}
		t.Fatalf("schema digest drift did not block restart: %#v %v", reopened, err)
	}
}

func TestReviewerContextSQLiteMissingDurableTablesBlockRestartV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviewer-context.db")
	store := openSQLiteTestV1(t, path)
	if _, err := store.db.Exec(`DROP TABLE context_reviewer_context_current`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE context_reviewer_context_history`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := OpenSQLiteV1(context.Background(), SQLiteConfigV1{Path: path}); !core.HasCategory(err, core.ErrorConflict) || reopened != nil {
		if reopened != nil {
			_ = reopened.Close()
		}
		t.Fatalf("missing durable tables were silently recreated: %#v %v", reopened, err)
	}
}

func TestReviewerContextSQLiteConcurrentCreateAndCASV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviewer-context.db")
	first, second := sqliteFixtureV1(t, "concurrent-sqlite")
	store := openSQLiteTestV1(t, path)
	t.Cleanup(func() { _ = store.Close() })
	run := func(request reviewport.ReviewerContextPublishRequestV1) (int64, int64) {
		var created, failed atomic.Int64
		var wg sync.WaitGroup
		for range 64 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				receipt, err := store.CommitV1(context.Background(), request)
				if err != nil {
					failed.Add(1)
					return
				}
				if receipt.Created {
					created.Add(1)
				}
			}()
		}
		wg.Wait()
		return created.Load(), failed.Load()
	}
	if created, failed := run(reviewport.ReviewerContextPublishRequestV1{Value: first}); created != 1 || failed != 0 {
		t.Fatalf("sqlite concurrent create: created=%d failed=%d", created, failed)
	}
	if created, failed := run(reviewport.ReviewerContextPublishRequestV1{Previous: &first.Ref, Value: second}); created != 1 || failed != 0 {
		t.Fatalf("sqlite concurrent CAS: created=%d failed=%d", created, failed)
	}
	if ref, err := store.ResolveV1(context.Background(), second.Subject); err != nil || ref != second.Ref {
		t.Fatalf("sqlite concurrent closure: %#v %v", ref, err)
	}
}

func TestReviewerContextSQLiteSameRevisionPayloadConflictPreservesWinnerV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviewer-context.db")
	first, second := sqliteFixtureV1(t, "payload-conflict")
	now := time.Unix(1_933_000_000, 0)
	drift, err := testfixture.ReviewerContextEnvelopeV1(first.Subject, 2, now, now.Add(time.Hour), "different-second")
	if err != nil {
		t.Fatal(err)
	}
	store := openSQLiteTestV1(t, path)
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.CommitV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Value: first}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Previous: &first.Ref, Value: second}); err != nil {
		t.Fatal(err)
	}
	if receipt, err := store.CommitV1(context.Background(), reviewport.ReviewerContextPublishRequestV1{Previous: &first.Ref, Value: drift}); !core.HasCategory(err, core.ErrorConflict) || receipt.Ref.ID != "" {
		t.Fatalf("same revision payload drift was accepted: %#v %v", receipt, err)
	}
	if winner, err := store.InspectHistoricalV1(context.Background(), second.Ref); err != nil || winner.Ref != second.Ref {
		t.Fatalf("payload conflict overwrote winner: %#v %v", winner.Ref, err)
	}
	if ref, err := store.ResolveV1(context.Background(), second.Subject); err != nil || ref != second.Ref {
		t.Fatalf("payload conflict changed current: %#v %v", ref, err)
	}
}

func TestReviewerContextSQLiteCanceledContextIsIndeterminateV1(t *testing.T) {
	store := openSQLiteTestV1(t, filepath.Join(t.TempDir(), "reviewer-context.db"))
	t.Cleanup(func() { _ = store.Close() })
	first, _ := sqliteFixtureV1(t, "canceled")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.CommitV1(ctx, reviewport.ReviewerContextPublishRequestV1{Value: first}); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("canceled mutation was not indeterminate: %v", err)
	}
	if _, err := store.ResolveV1(ctx, first.Subject); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("canceled resolve was not indeterminate: %v", err)
	}
	if _, err := store.InspectCurrentV1(ctx, first.Subject, first.Ref); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("canceled current inspect was not indeterminate: %v", err)
	}
}
