package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func TestWorkspaceReadAdmissionAttemptBindingV2MigrationRejectsPartialOrRepairableNamespace(t *testing.T) {
	tests := []struct {
		name  string
		setup func(context.Context, *sql.DB) error
	}{
		{name: "v17 partial table namespace", setup: func(ctx context.Context, db *sql.DB) error {
			_, err := db.ExecContext(ctx, `CREATE TABLE workspace_read_runtime_partial(object_id TEXT); PRAGMA user_version=17`)
			return err
		}},
		{name: "v17 partial index namespace", setup: func(ctx context.Context, db *sql.DB) error {
			_, err := db.ExecContext(ctx, `CREATE TABLE legacy_probe(object_id TEXT);
				CREATE INDEX workspace_read_runtime_partial_index ON legacy_probe(object_id);
				PRAGMA user_version=17`)
			return err
		}},
		{name: "v17 partial trigger namespace", setup: func(ctx context.Context, db *sql.DB) error {
			_, err := db.ExecContext(ctx, `CREATE TABLE legacy_probe(object_id TEXT);
				CREATE TRIGGER workspace_read_runtime_partial_trigger AFTER INSERT ON legacy_probe BEGIN SELECT 1; END;
				PRAGMA user_version=17`)
			return err
		}},
		{name: "v17 partial current namespace", setup: func(ctx context.Context, db *sql.DB) error {
			_, err := db.ExecContext(ctx, `CREATE TABLE workspace_read_runtime_current_v2(object_id TEXT);
				PRAGMA user_version=17`)
			return err
		}},
		{name: "v18 missing index no silent repair", setup: func(ctx context.Context, db *sql.DB) error {
			_, err := db.ExecContext(ctx, `DROP INDEX workspace_read_runtime_admission_identity_v2`)
			return err
		}},
		{name: "v18 missing table no silent repair", setup: func(ctx context.Context, db *sql.DB) error {
			_, err := db.ExecContext(ctx, `DROP TABLE workspace_read_runtime_attempt_admission_binding_v2`)
			return err
		}},
		{name: "v18 extra namespace index", setup: func(ctx context.Context, db *sql.DB) error {
			_, err := db.ExecContext(ctx, `CREATE INDEX workspace_read_runtime_unexpected_v2
				ON workspace_read_runtime_attempt_admission_binding_v2(binding_digest)`)
			return err
		}},
		{name: "v18 extra trigger", setup: func(ctx context.Context, db *sql.DB) error {
			_, err := db.ExecContext(ctx, `CREATE TRIGGER unexpected_runtime_v2_trigger AFTER INSERT ON workspace_read_runtime_attempt_admission_binding_v2 BEGIN SELECT 1; END`)
			return err
		}},
		{name: "v18 index collation drift", setup: func(ctx context.Context, db *sql.DB) error {
			_, err := db.ExecContext(ctx, `DROP INDEX workspace_read_runtime_admission_identity_v2;
				CREATE UNIQUE INDEX workspace_read_runtime_admission_identity_v2
				ON workspace_read_runtime_attempt_admission_binding_v2(admission_id COLLATE NOCASE,admission_revision,admission_digest)`)
			return err
		}},
		{name: "v18 extra check constraint", setup: rebuildWorkspaceReadRuntimeAttemptBindingDDLWithV2(func(statement string) string {
			return strings.Replace(
				statement,
				"binding_digest TEXT NOT NULL, body BLOB NOT NULL)",
				"binding_digest TEXT NOT NULL, body BLOB NOT NULL, CHECK(length(binding_digest)>0))",
				1,
			)
		})},
		{name: "v18 extra foreign key", setup: rebuildWorkspaceReadRuntimeAttemptBindingDDLWithV2(func(statement string) string {
			return strings.Replace(
				statement,
				"binding_digest TEXT NOT NULL, body BLOB NOT NULL)",
				"binding_digest TEXT NOT NULL, body BLOB NOT NULL, FOREIGN KEY(command_id) REFERENCES workspace_read_command_current(command_id))",
				1,
			)
		})},
		{name: "v18 non-index column collation drift", setup: rebuildWorkspaceReadRuntimeAttemptBindingDDLWithV2(func(statement string) string {
			return strings.Replace(
				statement,
				"authorization_digest TEXT NOT NULL",
				"authorization_digest TEXT COLLATE NOCASE NOT NULL",
				1,
			)
		})},
		{name: "v18 primary key NOCASE", setup: rebuildWorkspaceReadRuntimeAttemptBindingV2("NOCASE")},
		{name: "v18 primary key lowercase binary", setup: rebuildWorkspaceReadRuntimeAttemptBindingV2("binary")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			database := filepath.Join(t.TempDir(), "sandbox.db")
			if strings.HasPrefix(test.name, "v17 ") {
				raw, err := sql.Open("sqlite", dataSource(database))
				if err != nil {
					t.Fatal(err)
				}
				if err = test.setup(ctx, raw); err != nil {
					_ = raw.Close()
					t.Fatal(err)
				}
				if err = raw.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				store, err := OpenWithClock(ctx, database, time.Now)
				if err != nil {
					t.Fatal(err)
				}
				raw := store.db
				if err = test.setup(ctx, raw); err != nil {
					_ = store.Close()
					t.Fatal(err)
				}
				if err = store.Close(); err != nil {
					t.Fatal(err)
				}
			}
			schemaBefore := workspaceReadRuntimeSchemaSnapshotV2(t, ctx, database)
			if reopened, err := OpenWithClock(ctx, database, time.Now); err == nil {
				_ = reopened.Close()
				t.Fatal("incompatible V2 schema namespace was silently accepted or repaired")
			} else if !errors.Is(err, ports.ErrConflict) {
				t.Fatalf("schema incompatibility category=%v, want Conflict", err)
			}
			schemaAfter := workspaceReadRuntimeSchemaSnapshotV2(t, ctx, database)
			if !reflect.DeepEqual(schemaAfter, schemaBefore) {
				t.Fatalf("failed Open repaired V2 schema: before=%q after=%q", schemaBefore, schemaAfter)
			}
		})
	}
}

func TestWorkspaceReadAdmissionAttemptBindingV2DDLVerifierIsCommentAware(t *testing.T) {
	ctx := context.Background()
	database := filepath.Join(t.TempDir(), "sandbox.db")
	store, err := OpenWithClock(ctx, database, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	raw := store.db
	if err = rebuildWorkspaceReadRuntimeAttemptBindingDDLWithV2(func(statement string) string {
		return strings.Replace(
			statement,
			"authorization_digest TEXT NOT NULL",
			"authorization_digest /* semantics-preserving comment */ TEXT NOT NULL",
			1,
		)
	})(ctx, raw); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithClock(ctx, database, time.Now)
	if err != nil {
		t.Fatalf("semantics-preserving DDL comment rejected: %v", err)
	}
	if err = reopened.Close(); err != nil {
		t.Fatal(err)
	}

	baseTokens, err := canonicalSQLiteDDLTokensV2(`CREATE TABLE example(value TEXT /* comment */ NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	commentTokens, err := canonicalSQLiteDDLTokensV2("CREATE TABLE example(value TEXT -- comment\n NOT NULL)")
	if err != nil {
		t.Fatal(err)
	}
	if !equalSQLiteDDLTokensV2(baseTokens, commentTokens) {
		t.Fatal("equivalent SQLite comments changed canonical DDL tokens")
	}
	quotedTokens, err := canonicalSQLiteDDLTokensV2(`CREATE TABLE example(value TEXT CHECK(value!='/* not a comment */'))`)
	if err != nil {
		t.Fatal(err)
	}
	if equalSQLiteDDLTokensV2(baseTokens, quotedTokens) {
		t.Fatal("quoted comment marker was incorrectly removed as a comment")
	}
}

func rebuildWorkspaceReadRuntimeAttemptBindingV2(collation string) func(context.Context, *sql.DB) error {
	return rebuildWorkspaceReadRuntimeAttemptBindingDDLWithV2(func(statement string) string {
		return strings.Replace(
			statement,
			"runtime_attempt_digest TEXT NOT NULL PRIMARY KEY",
			"runtime_attempt_digest TEXT COLLATE "+collation+" NOT NULL PRIMARY KEY",
			1,
		)
	})
}

func rebuildWorkspaceReadRuntimeAttemptBindingDDLWithV2(
	mutateTable func(string) string,
) func(context.Context, *sql.DB) error {
	return func(ctx context.Context, db *sql.DB) error {
		if _, err := db.ExecContext(ctx, `DROP TABLE workspace_read_runtime_attempt_admission_binding_v2`); err != nil {
			return err
		}
		for _, statement := range schemaStatements {
			if !strings.Contains(statement, "workspace_read_runtime_attempt_admission_binding_v2") {
				continue
			}
			if strings.HasPrefix(statement, "CREATE TABLE") {
				statement = mutateTable(statement)
			}
			if _, err := db.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		return nil
	}
}

func workspaceReadRuntimeSchemaSnapshotV2(
	t *testing.T,
	ctx context.Context,
	database string,
) []string {
	t.Helper()
	db, err := sql.Open("sqlite", dataSource(database))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.QueryContext(
		ctx,
		`SELECT type,name,tbl_name,sql FROM sqlite_master
		  WHERE name LIKE 'workspace_read_runtime_%'
		     OR tbl_name='workspace_read_runtime_attempt_admission_binding_v2'
		  ORDER BY type,name`,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var snapshot []string
	for rows.Next() {
		var (
			kind, name, table string
			statement         sql.NullString
		)
		if err = rows.Scan(&kind, &name, &table, &statement); err != nil {
			t.Fatal(err)
		}
		value := kind + "\x00" + name + "\x00" + table + "\x00"
		if statement.Valid {
			value += "present\x00" + statement.String
		} else {
			value += "null"
		}
		snapshot = append(snapshot, value)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	return snapshot
}
