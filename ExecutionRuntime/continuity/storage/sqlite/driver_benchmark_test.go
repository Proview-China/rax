//go:build cgo && continuity_sqlite_bench

package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	_ "modernc.org/sqlite"
)

var errCASConflict = errors.New("cas conflict")

type sqliteCandidate struct {
	name   string
	driver string
	dsn    func(string) string
}

var sqliteCandidates = []sqliteCandidate{
	{name: "modernc", driver: "sqlite", dsn: moderncDSN},
	{name: "mattn-cgo", driver: "sqlite3", dsn: mattnDSN},
}

func moderncDSN(path string) string {
	u := &url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(FULL)")
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	return u.String()
}

func mattnDSN(path string) string {
	u := &url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Set("_busy_timeout", "5000")
	q.Set("_foreign_keys", "on")
	q.Set("_journal_mode", "WAL")
	q.Set("_synchronous", "FULL")
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	return u.String()
}

func TestSQLiteDriverCandidatesDurabilityCASAndRollback(t *testing.T) {
	for _, candidate := range sqliteCandidates {
		candidate := candidate
		t.Run(candidate.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "continuity.db")
			db := openSQLiteCandidate(t, candidate, path, 8)
			ctx := context.Background()
			if err := createFact(ctx, db, "scope-a", "fact-a", "digest-1", []byte("body-1")); err != nil {
				t.Fatal(err)
			}
			// Same create after a lost reply must be inspectable and must not
			// overwrite content.
			if revision, digest, body, err := inspectFact(ctx, db, "scope-a", "fact-a"); err != nil || revision != 1 || digest != "digest-1" || string(body) != "body-1" {
				t.Fatalf("lost-reply Inspect failed: revision=%d digest=%s body=%q err=%v", revision, digest, body, err)
			}
			if err := createFact(ctx, db, "scope-a", "fact-a", "digest-changed", []byte("changed")); err == nil {
				t.Fatal("same identity changed content must conflict")
			}
			if err := casFact(ctx, db, "scope-a", "fact-a", 1, "digest-2", []byte("body-2"), false); err != nil {
				t.Fatal(err)
			}
			if err := casFact(ctx, db, "scope-a", "fact-a", 2, "digest-3", []byte("body-3"), true); err == nil {
				t.Fatal("injected pre-commit failure must roll back")
			}
			if revision, digest, _, err := inspectFact(ctx, db, "scope-a", "fact-a"); err != nil || revision != 2 || digest != "digest-2" {
				t.Fatalf("rollback changed current: revision=%d digest=%s err=%v", revision, digest, err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db = openSQLiteCandidate(t, candidate, path, 8)
			defer db.Close()
			if revision, digest, _, err := inspectFact(ctx, db, "scope-a", "fact-a"); err != nil || revision != 2 || digest != "digest-2" {
				t.Fatalf("reopen durability failed: revision=%d digest=%s err=%v", revision, digest, err)
			}
		})
	}
}

func TestSQLiteDriverCandidatesConcurrentCASOneWinner(t *testing.T) {
	for _, candidate := range sqliteCandidates {
		candidate := candidate
		t.Run(candidate.name, func(t *testing.T) {
			db := openSQLiteCandidate(t, candidate, filepath.Join(t.TempDir(), "continuity.db"), 16)
			defer db.Close()
			ctx := context.Background()
			if err := createFact(ctx, db, "scope-a", "fact-race", "digest-1", []byte("body")); err != nil {
				t.Fatal(err)
			}
			var winners atomic.Int32
			var unexpected atomic.Int32
			var wg sync.WaitGroup
			for i := 0; i < 64; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					err := casFact(ctx, db, "scope-a", "fact-race", 1, fmt.Sprintf("digest-%d", i+2), []byte{byte(i)}, false)
					if err == nil {
						winners.Add(1)
					} else if !errors.Is(err, errCASConflict) {
						unexpected.Add(1)
					}
				}(i)
			}
			wg.Wait()
			if winners.Load() != 1 || unexpected.Load() != 0 {
				t.Fatalf("CAS closure failed: winners=%d unexpected=%d", winners.Load(), unexpected.Load())
			}
		})
	}
}

func BenchmarkSQLiteDriverCandidates(b *testing.B) {
	for _, candidate := range sqliteCandidates {
		candidate := candidate
		b.Run(candidate.name+"/create-full-sync", func(b *testing.B) {
			db := openSQLiteCandidate(b, candidate, filepath.Join(b.TempDir(), "continuity.db"), 1)
			defer db.Close()
			ctx := context.Background()
			body := make([]byte, 512)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := createFact(ctx, db, "scope", fmt.Sprintf("fact-%09d", i), fmt.Sprintf("digest-%09d", i), body); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(candidate.name+"/inspect", func(b *testing.B) {
			db := openSQLiteCandidate(b, candidate, filepath.Join(b.TempDir(), "continuity.db"), 1)
			defer db.Close()
			ctx := context.Background()
			if err := createFact(ctx, db, "scope", "fact", "digest", make([]byte, 512)); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, _, _, err := inspectFact(ctx, db, "scope", "fact"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

type testingTB interface {
	Helper()
	TempDir() string
	Fatal(args ...any)
}

func openSQLiteCandidate(tb testingTB, candidate sqliteCandidate, path string, maxOpen int) *sql.DB {
	tb.Helper()
	db, err := sql.Open(candidate.driver, candidate.dsn(path))
	if err != nil {
		tb.Fatal(err)
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxOpen)
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS facts (
            scope_digest TEXT NOT NULL,
            fact_id TEXT NOT NULL,
            revision INTEGER NOT NULL,
            digest TEXT NOT NULL,
            body BLOB NOT NULL,
            PRIMARY KEY(scope_digest, fact_id)
        )`,
		`CREATE TABLE IF NOT EXISTS fact_history (
            scope_digest TEXT NOT NULL,
            fact_id TEXT NOT NULL,
            revision INTEGER NOT NULL,
            digest TEXT NOT NULL,
            body BLOB NOT NULL,
            PRIMARY KEY(scope_digest, fact_id, revision)
        )`,
	} {
		if _, err = db.Exec(statement); err != nil {
			db.Close()
			tb.Fatal(err)
		}
	}
	return db
}

func createFact(ctx context.Context, db *sql.DB, scope, id, digest string, body []byte) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO fact_history(scope_digest,fact_id,revision,digest,body) VALUES(?,?,?,?,?)", scope, id, 1, digest, body); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO facts(scope_digest,fact_id,revision,digest,body) VALUES(?,?,?,?,?)", scope, id, 1, digest, body); err != nil {
		return err
	}
	return tx.Commit()
}

func casFact(ctx context.Context, db *sql.DB, scope, id string, expected uint64, digest string, body []byte, failBeforeCommit bool) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE facts SET revision=?,digest=?,body=? WHERE scope_digest=? AND fact_id=? AND revision=?", expected+1, digest, body, scope, id, expected)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errCASConflict
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO fact_history(scope_digest,fact_id,revision,digest,body) VALUES(?,?,?,?,?)", scope, id, expected+1, digest, body); err != nil {
		return err
	}
	if failBeforeCommit {
		return errors.New("injected pre-commit failure")
	}
	return tx.Commit()
}

func inspectFact(ctx context.Context, db *sql.DB, scope, id string) (uint64, string, []byte, error) {
	var revision uint64
	var digest string
	var body []byte
	err := db.QueryRowContext(ctx, "SELECT revision,digest,body FROM facts WHERE scope_digest=? AND fact_id=?", scope, id).Scan(&revision, &digest, &body)
	return revision, digest, body, err
}
