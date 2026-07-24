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

func TestSQLiteContentIntegrityAuditReopenExactAndNoAlias(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "continuity.db")
	store := openStore(t, path)
	now := time.Date(2026, 7, 17, 17, 0, 0, 0, time.UTC)
	fact := sqliteContentIntegrityFactV1(t, "content-audit-1", "request-1", "metadata_absent", now)
	if _, replay, err := store.CreateContentIntegrityAuditFactV1(ctx, fact); err != nil || replay {
		t.Fatalf("create = (%v,%v)", replay, err)
	}
	if _, replay, err := store.CreateContentIntegrityAuditFactV1(ctx, fact); err != nil || !replay {
		t.Fatalf("lost reply replay = (%v,%v)", replay, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openStore(t, path)
	defer store.Close()
	inspected, err := store.InspectContentIntegrityAuditV1(ctx, ports.InspectContentIntegrityAuditRequestV1{Ref: fact.Ref()})
	if err != nil || inspected.Ref() != fact.Ref() {
		t.Fatalf("reopen exact inspect = (%#v,%v)", inspected, err)
	}
	inspected.Findings[0].DetailCode = "mutated"
	again, err := store.InspectContentIntegrityAuditV1(ctx, ports.InspectContentIntegrityAuditRequestV1{Ref: fact.Ref()})
	if err != nil || again.Findings[0].DetailCode == "mutated" {
		t.Fatal("SQLite Content Integrity Audit aliases decoded caller memory")
	}
}

func TestSQLiteContentIntegrityAuditConcurrentDifferentContentSingleWinner(t *testing.T) {
	ctx := context.Background()
	store := openStore(t, filepath.Join(t.TempDir(), "continuity.db"))
	defer store.Close()
	now := time.Date(2026, 7, 17, 17, 0, 0, 0, time.UTC)
	var winners atomic.Int32
	var conflicts atomic.Int32
	var unexpected atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fact := sqliteContentIntegrityFactV1(t, "content-audit-race", "request-race", "finding-"+decimal(i), now)
			_, replay, err := store.CreateContentIntegrityAuditFactV1(ctx, fact)
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
		t.Fatalf("SQLite create-once closure winners=%d conflicts=%d unexpected=%d", winners.Load(), conflicts.Load(), unexpected.Load())
	}
}

func TestSQLiteContentIntegrityAuditMigratesSchemaFiveToCurrent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "continuity.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA user_version=5"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
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
		t.Fatalf("schema version = %d, err=%v", version, err)
	}
	var table string
	if err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='content_integrity_audit_facts'").Scan(&table); err != nil || table == "" {
		t.Fatalf("audit table migration = %q, err=%v", table, err)
	}
}

func sqliteContentIntegrityFactV1(t *testing.T, auditID, requestID, detail string, now time.Time) contract.ContentIntegrityAuditFactV1 {
	t.Helper()
	request := testkit.ContentIntegrityAuditRequestV1()
	request.AuditID = auditID
	request.IdempotencyKey = requestID
	requestDigest, err := request.CanonicalDigest()
	if err != nil {
		t.Fatal(err)
	}
	fact, err := contract.NewContentIntegrityAuditFactV1(auditID, requestID, requestDigest, request.Scope, testkit.ContentIntegrityAuditOwnerV1(), request.Subjects, []contract.ContentIntegrityFindingV1{{
		Subject: request.Subjects[0], Classification: contract.ContentIntegrityMetadataAbsent, DetailCode: detail,
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	return fact
}
