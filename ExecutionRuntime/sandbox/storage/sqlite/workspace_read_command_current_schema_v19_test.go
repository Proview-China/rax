package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceReadCommandCurrentSchemaV19RejectsDriftWithoutRepair(t *testing.T) {
	tests := []struct {
		name       string
		statements []string
	}{
		{name: "missing table", statements: []string{
			`DROP TABLE workspace_read_command_current`,
		}},
		{name: "extra column", statements: []string{
			`DROP TABLE workspace_read_command_current`,
			`CREATE TABLE workspace_read_command_current (
				command_id TEXT PRIMARY KEY, revision INTEGER NOT NULL,
				digest TEXT NOT NULL, body BLOB NOT NULL, extra TEXT)`,
		}},
		{name: "primary key collation", statements: []string{
			`DROP TABLE workspace_read_command_current`,
			`CREATE TABLE workspace_read_command_current (
				command_id TEXT COLLATE NOCASE PRIMARY KEY, revision INTEGER NOT NULL,
				digest TEXT NOT NULL, body BLOB NOT NULL)`,
		}},
		{name: "primary key axis", statements: []string{
			`DROP TABLE workspace_read_command_current`,
			`CREATE TABLE workspace_read_command_current (
				command_id TEXT NOT NULL, revision INTEGER PRIMARY KEY,
				digest TEXT NOT NULL, body BLOB NOT NULL)`,
		}},
		{name: "extra index", statements: []string{
			`CREATE INDEX workspace_read_command_current_unexpected ON workspace_read_command_current(digest)`,
		}},
		{name: "extra trigger", statements: []string{
			`CREATE TRIGGER workspace_read_command_current_unexpected_trigger
			 AFTER INSERT ON workspace_read_command_current BEGIN SELECT 1; END`,
		}},
		{name: "extra view", statements: []string{
			`CREATE VIEW workspace_read_command_current_unexpected_view AS
			 SELECT command_id FROM workspace_read_command_current`,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "sandbox.db")
			store, err := OpenWithClock(ctx, path, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			raw, err := sql.Open("sqlite", dataSource(path))
			if err != nil {
				t.Fatal(err)
			}
			raw.SetMaxOpenConns(1)
			if _, err = raw.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
				_ = raw.Close()
				t.Fatal(err)
			}
			for _, statement := range test.statements {
				if _, err = raw.ExecContext(ctx, statement); err != nil {
					_ = raw.Close()
					t.Fatal(err)
				}
			}
			if err = raw.Close(); err != nil {
				t.Fatal(err)
			}
			before := workspaceReadCommandCurrentSchemaSnapshotV19(t, ctx, path)
			if reopened, openErr := OpenWithClock(ctx, path, time.Now); openErr == nil {
				_ = reopened.Close()
				t.Fatal("drifted workspace read Command current schema was accepted")
			} else if !errors.Is(openErr, ports.ErrConflict) {
				t.Fatalf("schema drift category=%v, want Conflict", openErr)
			}
			after := workspaceReadCommandCurrentSchemaSnapshotV19(t, ctx, path)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("Open repaired drifted schema\nbefore=%v\nafter=%v", before, after)
			}
		})
	}
}

func TestWorkspaceReadCommandCurrentSchemaV19MigrationRejectsCoreDriftWithoutRepair(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := OpenWithClock(ctx, path, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.ExecContext(ctx, `
		DROP TABLE workspace_read_command_owner_current_pointer_v2;
		DROP TABLE workspace_read_command_owner_current_history_v2;
		DROP TABLE workspace_read_command_publication_v2;
		DROP TABLE workspace_read_command_publication_schema_v19;
		DROP TABLE workspace_read_command_current;
		CREATE TABLE workspace_read_command_current (
			command_id TEXT COLLATE NOCASE PRIMARY KEY,
			revision INTEGER NOT NULL,digest TEXT NOT NULL,body BLOB NOT NULL);
		PRAGMA user_version=18`); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	before := workspaceReadCommandCurrentSchemaSnapshotV19(t, ctx, path)
	if reopened, openErr := OpenWithClock(ctx, path, time.Now); openErr == nil {
		_ = reopened.Close()
		t.Fatal("migration accepted drifted workspace read Command current schema")
	} else if !errors.Is(openErr, ports.ErrConflict) {
		t.Fatalf("migration schema drift category=%v, want Conflict", openErr)
	}
	after := workspaceReadCommandCurrentSchemaSnapshotV19(t, ctx, path)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("migration repaired drifted schema\nbefore=%v\nafter=%v", before, after)
	}
}

func TestWorkspaceReadCommandCurrentSchemaV19MigratesRealV13Namespace(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := OpenWithClock(ctx, path, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	// Schema v13 predates every bounded workspace.read table. Starting from a
	// fully verified database and removing precisely the v15-v19 namespaces
	// retains the real legacy tables while avoiding a hand-written partial
	// fixture that could accidentally bless an invalid migration.
	if _, err = store.db.ExecContext(ctx, `
		DROP TABLE workspace_read_observation;
		DROP TABLE workspace_read_recovery_evidence;
		DROP TABLE workspace_read_attempt_owner_incarnation;
		DROP TABLE workspace_read_attempt_current;
		DROP TABLE workspace_read_attempt_origin;
		DROP TABLE workspace_read_admission_attempt_binding;
		DROP TABLE workspace_read_reservation;
		DROP TABLE workspace_read_runtime_attempt_admission_binding_v2;
		DROP TABLE workspace_read_command_owner_current_pointer_v2;
		DROP TABLE workspace_read_command_owner_current_history_v2;
		DROP TABLE workspace_read_command_publication_v2;
		DROP TABLE workspace_read_command_publication_schema_v19;
		DROP TABLE workspace_read_command_body_seal;
		DROP TABLE workspace_read_command_current;
		PRAGMA user_version=13`); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenWithClock(ctx, path, time.Now)
	if err != nil {
		t.Fatalf("valid v13 namespace did not migrate: %v", err)
	}
	defer reopened.Close()
	var version int
	if err = reopened.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Fatalf("migrated schema version=%d want=%d", version, schemaVersion)
	}
	if err = verifyWorkspaceReadCommandCurrentSchemaV19(ctx, mustWorkspaceReadReadOnlyTxV19(t, ctx, reopened.db)); err != nil {
		t.Fatalf("migrated workspace read Command current schema: %v", err)
	}
}

func mustWorkspaceReadReadOnlyTxV19(t *testing.T, ctx context.Context, db *sql.DB) *sql.Tx {
	t.Helper()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	return tx
}

func workspaceReadCommandCurrentSchemaSnapshotV19(
	t *testing.T,
	ctx context.Context,
	path string,
) []string {
	t.Helper()
	db, err := sql.Open("sqlite", dataSource(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.QueryContext(
		ctx,
		`SELECT type,name,tbl_name,coalesce(sql,'') FROM sqlite_master
		  WHERE name='workspace_read_command_current'
		     OR name LIKE 'workspace_read_command_current_%'
		     OR tbl_name='workspace_read_command_current'
		  ORDER BY type,name`,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var snapshot []string
	for rows.Next() {
		var kind, name, table, ddl string
		if err = rows.Scan(&kind, &name, &table, &ddl); err != nil {
			t.Fatal(err)
		}
		snapshot = append(snapshot, kind+":"+name+":"+table+":"+ddl)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return snapshot
}
