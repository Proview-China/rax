package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func TestSQLiteHistoryDerivationReopenExactAndConcurrentSingleWinner(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "continuity.db")
	store := openStore(t, path)
	var winners, conflicts, unexpected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fact := sqliteHistoryDerivationFactV1(t, i)
			_, replay, err := store.CreateHistoryDerivationCandidateFactV1(ctx, fact)
			switch {
			case err == nil && !replay:
				winners.Add(1)
			case contract.HasCode(err, contract.ErrRevisionConflict):
				conflicts.Add(1)
			default:
				unexpected.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if winners.Load() != 1 || conflicts.Load() != 63 || unexpected.Load() != 0 {
		t.Fatalf("winners=%d conflicts=%d unexpected=%d", winners.Load(), conflicts.Load(), unexpected.Load())
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openStore(t, path)
	defer store.Close()
	fact, err := store.InspectHistoryDerivationCandidateByIDV1(ctx, ports.InspectHistoryDerivationCandidateByIDRequestV1{TenantID: testkit.Scope().TenantID, ScopeDigest: testkit.Scope().ExecutionScopeDigest, CandidateID: "derivation-race", Owner: testkit.HistoryDerivationOwnerV1()})
	if err != nil || fact.Authority != contract.HistoryDerivationAuthorityV1 {
		t.Fatalf("reopen=(%#v,%v)", fact, err)
	}
}

func TestSQLiteHistoryDerivationMigratesSevenToEight(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "continuity.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "PRAGMA user_version=7"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	store := openStore(t, path)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != 9 {
		t.Fatalf("version=%d err=%v", version, err)
	}
	var table string
	if err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='history_derivation_candidate_facts'").Scan(&table); err != nil || table == "" {
		t.Fatalf("table=%q err=%v", table, err)
	}
}

func sqliteHistoryDerivationFactV1(t *testing.T, variant int) contract.HistoryDerivationCandidateFactV1 {
	t.Helper()
	event := testkit.TimelineEvent(uint64(variant+1), uint64(variant+1), contract.TrustObservation)
	output := testkit.ContentDeltaSourceV1(testkit.Scope()).Target
	fact, err := contract.NewHistoryDerivationCandidateFactV1("derivation-race", "request-race", "request-digest", testkit.Scope(), testkit.HistoryDerivationOwnerV1(), contract.HistoryDerivationSummary, []contract.HistoryDerivationEventRefV1{contract.HistoryDerivationEventRefFromRecordV1(event)}, output, time.Date(2026, 7, 17, 19, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return fact
}
