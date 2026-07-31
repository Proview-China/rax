package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	_ "modernc.org/sqlite"
)

func TestSQLiteV2WeakPrecreatedSchemasFailClosed(t *testing.T) {
	for _, test := range []struct {
		name   string
		schema string
		open   func(context.Context, string) error
	}{
		{
			name:   "action_fact",
			schema: `CREATE TABLE tool_action_candidate_v2(fact_id TEXT PRIMARY KEY)`,
			open: func(ctx context.Context, path string) error {
				store, err := OpenActionFactStoreV2(ctx, sqliteActionConfigV2(path, testkit.FixedTime), sqliteActionReadersV2())
				if store != nil {
					_ = store.Close()
				}
				return err
			},
		},
		{
			name: "action_nullable_exact_columns",
			schema: `CREATE TABLE tool_action_candidate_v2(
fact_id TEXT PRIMARY KEY,revision INTEGER NOT NULL,digest TEXT,
action_id TEXT NOT NULL UNIQUE,body_json BLOB NOT NULL,row_digest TEXT NOT NULL,
UNIQUE(fact_id,revision,digest)) STRICT`,
			open: func(ctx context.Context, path string) error {
				store, err := OpenActionFactStoreV2(ctx, sqliteActionConfigV2(path, testkit.FixedTime), sqliteActionReadersV2())
				if store != nil {
					_ = store.Close()
				}
				return err
			},
		},
		{
			name:   "claim_execution",
			schema: `CREATE TABLE tool_owner_single_call_claim_v2(claim_id TEXT PRIMARY KEY)`,
			open: func(ctx context.Context, path string) error {
				store, err := OpenOwnerClaimExecutionStoreV2(ctx, ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return testkit.FixedTime }})
				if store != nil {
					_ = store.Close()
				}
				return err
			},
		},
		{
			name: "claim_partial_unique",
			schema: `CREATE TABLE tool_owner_single_call_claim_v2(
claim_id TEXT PRIMARY KEY,claim_revision INTEGER NOT NULL,claim_digest TEXT NOT NULL,
request_id TEXT NOT NULL,request_revision INTEGER NOT NULL,request_digest TEXT NOT NULL,
action_coordinate_digest TEXT NOT NULL,execution_scope_digest TEXT NOT NULL,
binding_id TEXT NOT NULL,binding_revision INTEGER NOT NULL,binding_digest TEXT NOT NULL,
input_digest TEXT NOT NULL,claim_json BLOB NOT NULL,input_json BLOB NOT NULL,row_digest TEXT NOT NULL,
UNIQUE(claim_id,claim_revision,claim_digest)) STRICT;
CREATE UNIQUE INDEX attacker_partial_request ON tool_owner_single_call_claim_v2(
request_id,request_digest,action_coordinate_digest,execution_scope_digest) WHERE request_revision=1`,
			open: func(ctx context.Context, path string) error {
				store, err := OpenOwnerClaimExecutionStoreV2(ctx, ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return testkit.FixedTime }})
				if store != nil {
					_ = store.Close()
				}
				return err
			},
		},
		{
			name: "execution_head_fake_fk_comment",
			schema: `CREATE TABLE tool_owner_execution_head_v2(
request_key_digest TEXT PRIMARY KEY,request_id TEXT NOT NULL,request_digest TEXT NOT NULL,
action_coordinate_digest TEXT NOT NULL,execution_scope_digest TEXT NOT NULL,
binding_id TEXT NOT NULL,binding_revision INTEGER NOT NULL,binding_digest TEXT NOT NULL,
input_digest TEXT NOT NULL,state_id TEXT NOT NULL UNIQUE,state_revision INTEGER NOT NULL,state_digest TEXT NOT NULL
-- FOREIGN KEY(state_id,state_revision,state_digest) REFERENCES tool_owner_execution_history_v2(state_id,state_revision,state_digest)
) STRICT`,
			open: func(ctx context.Context, path string) error {
				store, err := OpenOwnerClaimExecutionStoreV2(ctx, ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return testkit.FixedTime }})
				if store != nil {
					_ = store.Close()
				}
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "weak.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(test.schema); err != nil {
				t.Fatal(err)
			}
			_ = db.Close()
			if err = test.open(context.Background(), path); err == nil || !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("weak schema open error=%v, want Conflict", err)
			}
		})
	}
}

func TestSQLiteV2LedgerNeverRepairsMissingOwnedTables(t *testing.T) {
	tests := []struct {
		name   string
		tables []string
		open   func(context.Context, string) error
	}{
		{
			name: "action_fact",
			tables: []string{
				"tool_action_candidate_v2", "tool_action_reservation_v2", "tool_domain_result_v2",
				"tool_apply_settlement_v2", "tool_result_v2", "tool_action_head_v2",
			},
			open: func(ctx context.Context, path string) error {
				store, err := OpenActionFactStoreV2(ctx, sqliteActionConfigV2(path, testkit.FixedTime), sqliteActionReadersV2())
				if store != nil {
					_ = store.Close()
				}
				return err
			},
		},
		{
			name: "claim_execution",
			tables: []string{
				"tool_owner_single_call_claim_v2", "tool_owner_execution_head_v2", "tool_owner_execution_history_v2",
				"tool_owner_entry_lease_head_v2", "tool_owner_entry_lease_history_v2",
			},
			open: func(ctx context.Context, path string) error {
				store, err := OpenOwnerClaimExecutionStoreV2(ctx, ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return testkit.FixedTime }})
				if store != nil {
					_ = store.Close()
				}
				return err
			},
		},
	}
	for _, test := range tests {
		for _, table := range test.tables {
			t.Run(test.name+"/"+table, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "missing-owned.db")
				if err := test.open(context.Background(), path); err != nil {
					t.Fatal(err)
				}
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
					t.Fatal(err)
				}
				if _, err = db.Exec(`DROP TABLE ` + table); err != nil {
					t.Fatal(err)
				}
				if err = db.Close(); err != nil {
					t.Fatal(err)
				}
				if err = test.open(context.Background(), path); err == nil || !core.HasCategory(err, core.ErrorConflict) {
					t.Fatalf("missing %s reopen error=%v, want Conflict", table, err)
				}
				db, err = sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				var count int
				if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
					t.Fatal(err)
				}
				_ = db.Close()
				if count != 0 {
					t.Fatalf("missing table %s was silently repaired", table)
				}
			})
		}
	}
}

func TestSQLiteV2NoLedgerPartialOwnerSchemaIsNeverCompleted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial-owner.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE tool_owner_entry_lease_head_v2(execution_attempt_id TEXT PRIMARY KEY) STRICT`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenOwnerClaimExecutionStoreV2(context.Background(), ConfigV1{
		Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return testkit.FixedTime },
	})
	if store != nil {
		_ = store.Close()
	}
	if err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("partial owner schema open error=%v, want Conflict", err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"tool_owner_claim_execution_schema_v2", "tool_owner_single_call_claim_v2", "tool_owner_execution_history_v2"} {
		var count int
		if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("partial owner schema silently created %s", table)
		}
	}
}

func TestBaseToolOwnerV1LedgerNeverRepairsMissingTable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "base-v1-missing.db")
	config := ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return testkit.FixedTime }}
	store, err := OpenV1(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`DROP TABLE model_tool_injection_material_v1`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if store, err = OpenV1(ctx, config); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		if store != nil {
			_ = store.Close()
		}
		t.Fatalf("missing base V1 table reopen error=%v, want Conflict", err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='model_tool_injection_material_v1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("base V1 migration silently repaired the missing material table")
	}
}

func TestBaseToolOwnerV1PhysicalSchemaDriftFailsClosed(t *testing.T) {
	validDigest := core.DigestBytes([]byte(schemaV1))
	tests := []struct {
		name   string
		schema string
		digest core.Digest
	}{
		{name: "extra_column", schema: strings.Replace(schemaV1, "applied_unix_nano INTEGER NOT NULL\n", "applied_unix_nano INTEGER NOT NULL,\n    attacker TEXT\n", 1), digest: validDigest},
		{name: "collation", schema: strings.Replace(schemaV1, "digest TEXT NOT NULL", "digest TEXT COLLATE NOCASE NOT NULL", 1), digest: validDigest},
		{name: "extra_index", schema: schemaV1 + `CREATE INDEX attacker_material_revision ON model_tool_injection_material_v1(revision);`, digest: validDigest},
		{name: "extra_trigger", schema: schemaV1 + `CREATE TRIGGER attacker_material_insert AFTER INSERT ON model_tool_injection_material_v1 BEGIN SELECT 1; END;`, digest: validDigest},
		{name: "forged_ledger", schema: schemaV1, digest: testkit.Digest("forged-v1-ledger")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "base-v1-drift.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(test.schema); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`INSERT INTO tool_owner_schema_v1(version,digest,applied_unix_nano) VALUES(?,?,?)`, schemaVersionV1, string(test.digest), testkit.FixedTime.UnixNano()); err != nil {
				t.Fatal(err)
			}
			_ = db.Close()
			store, err := OpenV1(context.Background(), ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return testkit.FixedTime }})
			if store != nil {
				_ = store.Close()
			}
			if err == nil || !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("physical drift error=%v, want Conflict", err)
			}
		})
	}
}

func TestBaseToolOwnerV1NoLedgerPartialNamespaceFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "base-v1-partial.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE model_tool_injection_material_v1(material_id TEXT PRIMARY KEY) STRICT`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	store, err := OpenV1(context.Background(), ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return testkit.FixedTime }})
	if store != nil {
		_ = store.Close()
	}
	if err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("partial base V1 namespace error=%v, want Conflict", err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, name := range []string{"tool_owner_schema_v1", "tool_surface_invocation_binding_v1"} {
		var count int
		if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name=?`, name).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("partial base V1 namespace silently created %s", name)
		}
	}
}

func TestToolDurableConstructorsExcludeApplicationResultStoreV2(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "owner-only.db")
	config := ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return testkit.FixedTime }}
	for range 8 {
		owner, err := OpenOwnerClaimExecutionStoreV2(ctx, config)
		if err != nil {
			t.Fatal(err)
		}
		if err = owner.Close(); err != nil {
			t.Fatal(err)
		}
		facts, err := OpenActionFactStoreV2(ctx, config, sqliteActionReadersV2())
		if err != nil {
			t.Fatal(err)
		}
		if err = facts.Close(); err != nil {
			t.Fatal(err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='single_call_application_result_v2'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("Tool durable constructors initialized the cross-Owner Application result table")
	}
	for _, value := range []any{(*OwnerClaimExecutionStoreV2)(nil), (*ActionFactStoreV2)(nil)} {
		typ := reflect.TypeOf(value)
		if _, present := typ.MethodByName("CreateSingleCallApplicationResultV2"); present {
			t.Fatalf("%v unexpectedly exposes the Application result writer", typ)
		}
	}
}

func TestSQLiteV2CorrectLedgerDoesNotAuthorizeMissingUniqueIndexes(t *testing.T) {
	tests := []struct {
		name          string
		schema        string
		ledgerTable   string
		ledgerVersion int64
		ledgerDigest  core.Digest
		open          func(context.Context, string) error
	}{
		{
			name: "action_fact",
			schema: strings.Replace(actionFactSchemaV2,
				"    action_id TEXT NOT NULL UNIQUE,\n", "    action_id TEXT NOT NULL,\n", 1),
			ledgerTable: "tool_action_fact_schema_v2", ledgerVersion: actionFactSchemaVersionV2, ledgerDigest: actionFactSchemaDigestV2,
			open: func(ctx context.Context, path string) error {
				store, err := OpenActionFactStoreV2(ctx, sqliteActionConfigV2(path, testkit.FixedTime), sqliteActionReadersV2())
				if store != nil {
					_ = store.Close()
				}
				return err
			},
		},
		{
			name: "claim_execution",
			schema: strings.Replace(ownerClaimExecutionSchemaV2,
				"    UNIQUE(claim_id,claim_revision,claim_digest),\n    UNIQUE(request_id,request_digest,action_coordinate_digest,execution_scope_digest)\n",
				"    UNIQUE(claim_id,claim_revision,claim_digest)\n", 1),
			ledgerTable: "tool_owner_claim_execution_schema_v2", ledgerVersion: ownerClaimExecutionSchemaVersionV2, ledgerDigest: ownerClaimExecutionSchemaDigestV2,
			open: func(ctx context.Context, path string) error {
				store, err := OpenOwnerClaimExecutionStoreV2(ctx, ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return testkit.FixedTime }})
				if store != nil {
					_ = store.Close()
				}
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "missing-unique.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(test.schema); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`INSERT INTO `+test.ledgerTable+`(version,digest,applied_unix_nano) VALUES(?,?,?)`,
				test.ledgerVersion, string(test.ledgerDigest), testkit.FixedTime.UnixNano()); err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			if err = test.open(context.Background(), path); err == nil || !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("correct-ledger weak-index open error=%v, want Conflict", err)
			}
		})
	}
}

func TestSQLiteV2StageCheckCommentSpoofFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "weak-stage.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	validCheck := "CHECK(stage IN ('candidate','reserved','domain_result','settled'))"
	weakCheck := "CHECK(1 /* " + validCheck + " */)"
	weakSchema := strings.Replace(actionFactSchemaV2, validCheck, weakCheck, 1)
	if weakSchema == actionFactSchemaV2 {
		t.Fatal("test fixture did not replace the stage CHECK")
	}
	if _, err = db.Exec(weakSchema); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO tool_action_fact_schema_v2(version,digest,applied_unix_nano) VALUES(?,?,?)`,
		actionFactSchemaVersionV2, string(actionFactSchemaDigestV2), testkit.FixedTime.UnixNano()); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO tool_action_head_v2(
action_id,head_revision,head_digest,stage,candidate_id,candidate_revision,candidate_digest,updated_unix_nano,row_digest
) VALUES(?,?,?,?,?,?,?,?,?)`, "attacker-action", 1, "attacker-head", "invalid-stage", "candidate", 1, "candidate-digest", testkit.FixedTime.UnixNano(), "attacker-row"); err != nil {
		t.Fatalf("weak CHECK unexpectedly rejected invalid stage: %v", err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenActionFactStoreV2(context.Background(), sqliteActionConfigV2(path, testkit.FixedTime), sqliteActionReadersV2())
	if store != nil {
		_ = store.Close()
	}
	if err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("comment-spoofed CHECK open error=%v, want Conflict", err)
	}
}

func TestSQLiteV2PhysicalSchemaExtrasAndORCheckFailClosed(t *testing.T) {
	validCheck := "CHECK(stage IN ('candidate','reserved','domain_result','settled'))"
	for _, test := range []struct {
		name   string
		schema string
	}{
		{name: "or_check", schema: strings.Replace(actionFactSchemaV2, validCheck, "CHECK(1 OR stage IN ('candidate','reserved','domain_result','settled'))", 1)},
		{name: "extra_index", schema: actionFactSchemaV2 + `CREATE INDEX attacker_stage_index ON tool_action_head_v2(stage);`},
		{name: "extra_trigger", schema: actionFactSchemaV2 + `CREATE TRIGGER attacker_stage_trigger AFTER INSERT ON tool_action_head_v2 BEGIN SELECT 1; END;`},
		{name: "generated_hidden", schema: strings.Replace(actionFactSchemaV2, "updated_unix_nano INTEGER NOT NULL,\n    row_digest TEXT NOT NULL\n) STRICT;", "updated_unix_nano INTEGER NOT NULL,\n    row_digest TEXT NOT NULL,\n    attacker TEXT GENERATED ALWAYS AS (stage) VIRTUAL\n) STRICT;", 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "physical-spoof.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(test.schema); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`INSERT INTO tool_action_fact_schema_v2(version,digest,applied_unix_nano) VALUES(?,?,?)`,
				actionFactSchemaVersionV2, string(actionFactSchemaDigestV2), testkit.FixedTime.UnixNano()); err != nil {
				t.Fatal(err)
			}
			if test.name == "or_check" {
				if _, err = db.Exec(`INSERT INTO tool_action_head_v2(action_id,head_revision,head_digest,stage,candidate_id,candidate_revision,candidate_digest,updated_unix_nano,row_digest) VALUES(?,?,?,?,?,?,?,?,?)`,
					"or-action", 1, "head", "invalid-stage", "candidate", 1, "candidate-digest", 1, "row"); err != nil {
					t.Fatalf("OR CHECK unexpectedly rejected invalid stage: %v", err)
				}
			}
			_ = db.Close()
			store, openErr := OpenActionFactStoreV2(context.Background(), sqliteActionConfigV2(path, testkit.FixedTime), sqliteActionReadersV2())
			if store != nil {
				_ = store.Close()
			}
			if openErr == nil || !core.HasCategory(openErr, core.ErrorConflict) {
				t.Fatalf("physical spoof open error=%v, want Conflict", openErr)
			}
		})
	}
}

func TestSQLiteV2IndexXInfoShapeFailsClosed(t *testing.T) {
	validName := sql.NullString{String: "fact_id", Valid: true}
	validBinary := sql.NullString{String: "BINARY", Valid: true}
	tests := []struct {
		name       string
		cid        int
		column     sql.NullString
		descending int
		collation  sql.NullString
		key        int
	}{
		{name: "key_2", cid: 0, column: validName, collation: validBinary, key: 2},
		{name: "key_minus_1", cid: 0, column: validName, collation: validBinary, key: -1},
		{name: "key_NOCASE", cid: 0, column: validName, collation: sql.NullString{String: "NOCASE", Valid: true}, key: 1},
		{name: "aux_NOCASE", cid: -1, collation: sql.NullString{String: "NOCASE", Valid: true}, key: 0},
		{name: "key_lowercase_binary", cid: 0, column: validName, collation: sql.NullString{String: "binary", Valid: true}, key: 1},
		{name: "aux_lowercase_binary", cid: -1, collation: sql.NullString{String: "binary", Valid: true}, key: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := verifyIndexXInfoRowV2("tool_test_v2", test.cid, test.column, test.descending, test.collation, test.key); err == nil || !core.HasCategory(err, core.ErrorConflict) || !core.HasReason(err, core.ReasonUnknownSchema) {
				t.Fatalf("index_xinfo shape error=%v, want Conflict/UnknownSchema", err)
			}
		})
	}
	if column, key, err := verifyIndexXInfoRowV2("tool_test_v2", 0, validName, 0, validBinary, 1); err != nil || !key || column != "fact_id" {
		t.Fatalf("valid key column=%q key=%v err=%v", column, key, err)
	}
	if column, key, err := verifyIndexXInfoRowV2("tool_test_v2", -1, sql.NullString{}, 0, validBinary, 0); err != nil || key || column != "" {
		t.Fatalf("valid aux column=%q key=%v err=%v", column, key, err)
	}
}

func TestSQLiteV2PragmaSequenceFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name     string
		actual   int
		expected int
		wantErr  bool
	}{
		{name: "exact_zero", actual: 0, expected: 0},
		{name: "exact_aux", actual: 3, expected: 3},
		{name: "gap", actual: 2, expected: 1, wantErr: true},
		{name: "duplicate", actual: 1, expected: 2, wantErr: true},
		{name: "negative", actual: -1, expected: 0, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := verifySchemaSequenceV2("test index_xinfo", test.actual, test.expected)
			if test.wantErr {
				if err == nil || !core.HasCategory(err, core.ErrorConflict) || !core.HasReason(err, core.ReasonUnknownSchema) {
					t.Fatalf("sequence error=%v, want Conflict/UnknownSchema", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOwnerExecutionHeadResolvesExactImmutableHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "owner.db")
	raw, err := OpenOwnerClaimExecutionStoreV2(ctx, ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return testkit.FixedTime }})
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	row := validOwnerExecutionRowV2(t)
	if _, created, err := raw.CreateExecutionRowV2(ctx, row); err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	recovered, err := raw.InspectExecutionRowV2(ctx, row.RequestKeyDigest)
	if err != nil || string(recovered.StateJSON) != string(row.StateJSON) {
		t.Fatalf("recovered=%#v err=%v", recovered, err)
	}
	retargeted := advanceOwnerExecutionRowInspectOnlyV2(t, row)
	var retargetedState ownerExecutionStateRowV2
	if err = core.DecodeStrictJSON(retargeted.StateJSON, &retargetedState); err != nil {
		t.Fatal(err)
	}
	retargetedState.BindingRef.ID = "binding-attacker-v2"
	retargetedState.Digest = ""
	retargetedState.Digest, _ = core.CanonicalJSONDigest("praxis.tool-mcp.single-call-execution-state", "2.0.0", "ToolOwnerSingleCallExecutionStateV2", retargetedState)
	retargeted.StateJSON, _ = json.Marshal(retargetedState)
	retargeted.BindingID, retargeted.StateDigest = retargetedState.BindingRef.ID, string(retargetedState.Digest)
	retargetedDigest, _ := core.CanonicalJSONDigest("praxis.tool-mcp.sqlite-row", "v1", "ToolOwnerSingleCallExecutionStateV2", retargetedState)
	retargeted.RowDigest = string(retargetedDigest)
	if err = raw.AdvanceExecutionRowV2(ctx, row.StateID, row.StateRevision, row.StateDigest, retargeted); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("raw identity splice error=%v, want Conflict", err)
	}
	if _, err = raw.store.db.Exec(`UPDATE tool_owner_execution_history_v2 SET state_json=? WHERE state_id=? AND state_revision=?`, []byte(`{"state":"spliced"}`), row.StateID, row.StateRevision); err != nil {
		t.Fatal(err)
	}
	if _, err = raw.InspectExecutionRowV2(ctx, row.RequestKeyDigest); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("spliced raw history error=%v, want Conflict", err)
	}
}

func TestOwnerClaimExecutionCommitLostRepliesRecoverExactRows(t *testing.T) {
	ctx := context.Background()
	raw, err := OpenOwnerClaimExecutionStoreV2(ctx, ConfigV1{Path: filepath.Join(t.TempDir(), "lost.db"), Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return testkit.FixedTime }})
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	claim := OwnerClaimRowV2{ClaimID: "claim-v2", ClaimRevision: 1, ClaimDigest: string(testkit.Digest("claim")), Request: OwnerRequestKeyRowV2{RequestKeyDigest: string(testkit.Digest("key")), RequestID: "request-v2", RequestRevision: 1, RequestDigest: string(testkit.Digest("request")), ActionCoordinateDigest: string(testkit.Digest("action")), ExecutionScopeDigest: string(testkit.Digest("scope"))}, BindingID: "binding-v2", BindingRevision: 1, BindingDigest: string(testkit.Digest("binding")), InputDigest: string(testkit.Digest("input")), ClaimJSON: []byte(`{"claim":"opaque"}`), InputJSON: []byte(`{"input":"opaque"}`), RowDigest: string(testkit.Digest("row"))}
	if _, _, err = raw.CreateClaimRowV2(ctx, claim); err == nil || !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("non-canonical raw claim poisoned the unique winner: %v", err)
	}
	start := validOwnerExecutionRowV2(t)
	raw.fault = func(point string) error {
		if point == "execution_start_after_commit" {
			return errors.New("lost")
		}
		return nil
	}
	if _, _, err = raw.CreateExecutionRowV2(ctx, start); err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("marker start lost reply error=%v", err)
	}
	if _, err = raw.InspectExecutionRowV2(ctx, start.RequestKeyDigest); err != nil {
		t.Fatal(err)
	}
	next := advanceOwnerExecutionRowInspectOnlyV2(t, start)
	raw.fault = func(point string) error {
		if point == "execution_advance_after_commit" {
			return errors.New("lost")
		}
		return nil
	}
	if err = raw.AdvanceExecutionRowV2(ctx, start.StateID, start.StateRevision, start.StateDigest, next); err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("marker advance lost reply error=%v", err)
	}
	recovered, err := raw.InspectExecutionRowV2(ctx, start.RequestKeyDigest)
	if err != nil || recovered.StateRevision != next.StateRevision || recovered.StateDigest != next.StateDigest {
		t.Fatalf("recovered marker=%#v err=%v", recovered, err)
	}
}

func TestOwnerEntryLeaseCommitLostReplyAndRestartRecoverExactWinner(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "lease-lost.db")
	config := ConfigV1{
		Path:  path,
		Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"},
		Clock: func() time.Time { return testkit.FixedTime },
	}
	raw, err := OpenOwnerClaimExecutionStoreV2(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	lease := validOwnerEntryLeaseRowV2(t, "holder-v2", 1, testkit.FixedTime, testkit.FixedTime.Add(time.Minute))
	raw.fault = func(point string) error {
		if point == "entry_lease_after_commit" {
			return errors.New("lost")
		}
		return nil
	}
	if err = raw.CompareAndSwapEntryLeaseRowV2(ctx, nil, lease); err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lease lost reply error=%v, want Indeterminate", err)
	}
	recovered, err := raw.InspectEntryLeaseRowV2(ctx, lease.ExecutionAttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.LeaseID != lease.LeaseID || recovered.LeaseRevision != lease.LeaseRevision ||
		recovered.LeaseDigest != lease.LeaseDigest || string(recovered.LeaseJSON) != string(lease.LeaseJSON) {
		t.Fatalf("lost-reply winner drifted: recovered=%#v lease=%#v", recovered, lease)
	}
	if err = raw.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err = OpenOwnerClaimExecutionStoreV2(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	restarted, err := raw.InspectEntryLeaseRowV2(ctx, lease.ExecutionAttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.LeaseID != lease.LeaseID || restarted.LeaseRevision != lease.LeaseRevision ||
		restarted.LeaseDigest != lease.LeaseDigest || string(restarted.LeaseJSON) != string(lease.LeaseJSON) {
		t.Fatalf("restart winner drifted: restarted=%#v lease=%#v", restarted, lease)
	}
}

func TestOwnerExecutionAdvanceLostReplyInspectsPresealedHistoryNotCurrent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "execution-exact-recovery.db")
	config := ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return testkit.FixedTime }}
	raw, err := OpenOwnerClaimExecutionStoreV2(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	r1 := validOwnerExecutionRowV2(t)
	if _, created, createErr := raw.CreateExecutionRowV2(ctx, r1); createErr != nil || !created {
		t.Fatalf("r1 created=%v err=%v", created, createErr)
	}
	r2 := advanceOwnerExecutionRowInspectOnlyV2(t, r1)
	r3 := advanceOwnerExecutionRowSettledV2(t, r2)
	raw.fault = func(point string) error {
		if point != "execution_advance_after_commit" {
			return nil
		}
		other, openErr := OpenOwnerClaimExecutionStoreV2(ctx, config)
		if openErr != nil {
			return openErr
		}
		defer other.Close()
		if advanceErr := other.AdvanceExecutionRowV2(ctx, r2.StateID, r2.StateRevision, r2.StateDigest, r3); advanceErr != nil {
			return advanceErr
		}
		return errors.New("lost after r2 commit")
	}
	if err = raw.AdvanceExecutionRowV2(ctx, r1.StateID, r1.StateRevision, r1.StateDigest, r2); err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("r2 lost reply error=%v, want Indeterminate", err)
	}
	exactR2, err := raw.InspectExecutionHistoryExactRowV2(ctx, r2)
	if err != nil || !reflect.DeepEqual(exactR2, r2) {
		t.Fatalf("exact r2=%#v err=%v", exactR2, err)
	}
	current, err := raw.InspectExecutionRowByStateIDV2(ctx, r1.StateID)
	if err != nil || !reflect.DeepEqual(current, r3) {
		t.Fatalf("current=%#v err=%v, want r3", current, err)
	}
}

func TestOwnerExecutionStartLostReplyInspectsPresealedHistoryNotCurrent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "execution-start-exact-recovery.db")
	config := ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return testkit.FixedTime }}
	raw, err := OpenOwnerClaimExecutionStoreV2(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	r1 := validOwnerExecutionRowV2(t)
	r2 := advanceOwnerExecutionRowInspectOnlyV2(t, r1)
	raw.fault = func(point string) error {
		if point != "execution_start_after_commit" {
			return nil
		}
		other, openErr := OpenOwnerClaimExecutionStoreV2(ctx, config)
		if openErr != nil {
			return openErr
		}
		defer other.Close()
		if advanceErr := other.AdvanceExecutionRowV2(ctx, r1.StateID, r1.StateRevision, r1.StateDigest, r2); advanceErr != nil {
			return advanceErr
		}
		return errors.New("lost after r1 commit")
	}
	if _, _, err = raw.CreateExecutionRowV2(ctx, r1); err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("r1 lost reply error=%v, want Indeterminate", err)
	}
	exactR1, err := raw.InspectExecutionHistoryExactRowV2(ctx, r1)
	if err != nil || !reflect.DeepEqual(exactR1, r1) {
		t.Fatalf("exact r1=%#v err=%v", exactR1, err)
	}
	current, err := raw.InspectExecutionRowV2(ctx, r1.RequestKeyDigest)
	if err != nil || !reflect.DeepEqual(current, r2) {
		t.Fatalf("current=%#v err=%v, want r2", current, err)
	}
	replayed, created, err := raw.CreateExecutionRowV2(ctx, r1)
	if err != nil || created || !reflect.DeepEqual(replayed, r1) {
		t.Fatalf("replayed start=%#v created=%v err=%v, want exact r1", replayed, created, err)
	}
}

func TestOwnerEntryLeaseLostReplyInspectsPresealedHistoryNotCurrent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "lease-exact-recovery.db")
	config := ConfigV1{Path: path, Owner: core.OwnerRef{Domain: "tool-mcp", ID: "owner-v2"}, Clock: func() time.Time { return testkit.FixedTime }}
	raw, err := OpenOwnerClaimExecutionStoreV2(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	r1 := validOwnerEntryLeaseRowV2(t, "holder-a-v2", 1, testkit.FixedTime, testkit.FixedTime.Add(10*time.Second))
	if err = raw.CompareAndSwapEntryLeaseRowV2(ctx, nil, r1); err != nil {
		t.Fatal(err)
	}
	r2 := advanceOwnerEntryLeaseRowV2(t, r1, "holder-a-v2", "handoff_inspect", testkit.FixedTime.Add(time.Second), testkit.FixedTime.Add(10*time.Second))
	r3 := advanceOwnerEntryLeaseRowV2(t, r2, "holder-b-v2", "inspect", testkit.FixedTime.Add(2*time.Second), testkit.FixedTime.Add(12*time.Second))
	raw.fault = func(point string) error {
		if point != "entry_lease_after_commit" {
			return nil
		}
		other, openErr := OpenOwnerClaimExecutionStoreV2(ctx, config)
		if openErr != nil {
			return openErr
		}
		defer other.Close()
		if advanceErr := other.CompareAndSwapEntryLeaseRowV2(ctx, &r2, r3); advanceErr != nil {
			return advanceErr
		}
		return errors.New("lost after r2 commit")
	}
	if err = raw.CompareAndSwapEntryLeaseRowV2(ctx, &r1, r2); err == nil || !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("r2 lost reply error=%v, want Indeterminate", err)
	}
	exactR2, err := raw.InspectEntryLeaseHistoryExactRowV2(ctx, r2)
	if err != nil || !reflect.DeepEqual(exactR2, r2) {
		t.Fatalf("exact r2=%#v err=%v", exactR2, err)
	}
	current, err := raw.InspectEntryLeaseRowV2(ctx, r1.ExecutionAttemptID)
	if err != nil || !reflect.DeepEqual(current, r3) {
		t.Fatalf("current=%#v err=%v, want r3", current, err)
	}
}

func validOwnerExecutionRowV2(t *testing.T) OwnerExecutionRowV2 {
	t.Helper()
	key := applicationcontract.SingleCallToolActionInspectKeyV2{ContractVersion: applicationcontract.SingleCallToolActionContractVersionV2, RequestID: "request-v2", RequestRevision: 1, RequestDigest: testkit.Digest("request"), ActionCoordinateDigest: testkit.Digest("action"), ScopeDigest: testkit.Digest("scope")}
	var err error
	key.Digest, err = key.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	claim := toolcontract.ObjectRef{ID: "claim-v2", Revision: 1, Digest: testkit.Digest("claim")}
	inputDigest := testkit.Digest("input")
	stateID, _ := toolcontract.StableID("tool-owner-execution-state-v2", claim.ID, string(inputDigest))
	attemptID, _ := toolcontract.StableID("tool-owner-execution-attempt-v2", claim.ID, string(inputDigest))
	state := ownerExecutionStateRowV2{ContractVersion: "praxis.tool-mcp.single-call-execution-state/v2", ID: stateID, Revision: 1, ClaimRef: claim, RequestKey: key, RequestDigest: key.RequestDigest, ActionDigest: key.ActionCoordinateDigest, ExecutionScopeDigest: key.ScopeDigest, BindingRef: toolcontract.SingleCallToolActionBindingCurrentRefV2{ID: "binding-v2", Revision: 1, Digest: testkit.Digest("binding")}, ExecutionInputDigest: inputDigest, ExecutionAttemptID: attemptID, State: "start_committed", CreatedUnixNano: testkit.FixedTime.UnixNano(), UpdatedUnixNano: testkit.FixedTime.UnixNano(), ExpiresUnixNano: testkit.FixedTime.Add(time.Minute).UnixNano()}
	state.Digest, err = core.CanonicalJSONDigest("praxis.tool-mcp.single-call-execution-state", "2.0.0", "ToolOwnerSingleCallExecutionStateV2", state)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	rowDigest, err := core.CanonicalJSONDigest("praxis.tool-mcp.sqlite-row", "v1", "ToolOwnerSingleCallExecutionStateV2", state)
	if err != nil {
		t.Fatal(err)
	}
	return OwnerExecutionRowV2{RequestKeyDigest: string(key.Digest), RequestID: key.RequestID, RequestDigest: string(key.RequestDigest), ActionCoordinateDigest: string(key.ActionCoordinateDigest), ExecutionScopeDigest: string(key.ScopeDigest), BindingID: state.BindingRef.ID, BindingRevision: 1, BindingDigest: string(state.BindingRef.Digest), InputDigest: string(inputDigest), StateID: state.ID, StateRevision: 1, StateDigest: string(state.Digest), StateJSON: body, RowDigest: string(rowDigest)}
}

func validOwnerEntryLeaseRowV2(
	t *testing.T,
	holder string,
	revision core.Revision,
	acquired time.Time,
	expires time.Time,
) OwnerEntryLeaseRowV2 {
	t.Helper()
	execution := validOwnerExecutionRowV2(t)
	var state ownerExecutionStateRowV2
	if err := core.DecodeStrictJSON(execution.StateJSON, &state); err != nil {
		t.Fatal(err)
	}
	leaseID, err := toolcontract.StableID("tool-owner-entry-lease-v2", state.ExecutionAttemptID)
	if err != nil {
		t.Fatal(err)
	}
	lease := ownerEntryLeaseBodyRowV2{
		ContractVersion:      "praxis.tool-mcp.single-call-entry-lease/v2",
		ID:                   leaseID,
		Revision:             revision,
		RequestKey:           state.RequestKey,
		RequestDigest:        state.RequestDigest,
		ExecutionInputDigest: state.ExecutionInputDigest,
		ExecutionAttemptID:   state.ExecutionAttemptID,
		HolderIncarnationID:  holder,
		Phase:                "start_or_inspect",
		AcquiredUnixNano:     acquired.UnixNano(),
		ExpiresUnixNano:      expires.UnixNano(),
	}
	lease.Digest, err = core.CanonicalJSONDigest(
		"praxis.tool-mcp.single-call-entry-lease",
		"2.0.0",
		"ToolOwnerSingleCallEntryLeaseV2",
		lease,
	)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(lease)
	if err != nil {
		t.Fatal(err)
	}
	rowDigest, err := core.CanonicalJSONDigest(
		"praxis.tool-mcp.sqlite-row",
		"v1",
		"ToolOwnerSingleCallEntryLeaseV2",
		lease,
	)
	if err != nil {
		t.Fatal(err)
	}
	return OwnerEntryLeaseRowV2{
		LeaseID:             lease.ID,
		LeaseRevision:       int64(lease.Revision),
		LeaseDigest:         string(lease.Digest),
		ExecutionAttemptID:  lease.ExecutionAttemptID,
		RequestKeyDigest:    string(lease.RequestKey.Digest),
		RequestDigest:       string(lease.RequestDigest),
		InputDigest:         string(lease.ExecutionInputDigest),
		HolderIncarnationID: lease.HolderIncarnationID,
		Phase:               lease.Phase,
		AcquiredUnixNano:    lease.AcquiredUnixNano,
		ExpiresUnixNano:     lease.ExpiresUnixNano,
		LeaseJSON:           body,
		RowDigest:           string(rowDigest),
	}
}

func advanceOwnerExecutionRowInspectOnlyV2(t *testing.T, current OwnerExecutionRowV2) OwnerExecutionRowV2 {
	t.Helper()
	var state ownerExecutionStateRowV2
	if err := core.DecodeStrictJSON(current.StateJSON, &state); err != nil {
		t.Fatal(err)
	}
	state.Revision = 2
	state.State = "inspect_only"
	state.UpdatedUnixNano++
	state.Unknown = &ownerExecutionUnknownRowV2{Class: "entry_outcome_unknown", ErrorDigest: testkit.Digest("unknown"), MarkedUnixNano: state.UpdatedUnixNano}
	state.Digest = ""
	var err error
	state.Digest, err = core.CanonicalJSONDigest("praxis.tool-mcp.single-call-execution-state", "2.0.0", "ToolOwnerSingleCallExecutionStateV2", state)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(state)
	rowDigest, _ := core.CanonicalJSONDigest("praxis.tool-mcp.sqlite-row", "v1", "ToolOwnerSingleCallExecutionStateV2", state)
	next := current
	next.StateRevision, next.StateDigest, next.StateJSON, next.RowDigest = 2, string(state.Digest), body, string(rowDigest)
	return next
}

func advanceOwnerExecutionRowSettledV2(t *testing.T, current OwnerExecutionRowV2) OwnerExecutionRowV2 {
	t.Helper()
	var state ownerExecutionStateRowV2
	if err := core.DecodeStrictJSON(current.StateJSON, &state); err != nil {
		t.Fatal(err)
	}
	state.Revision++
	state.State = "settled"
	state.UpdatedUnixNano++
	state.Unknown = nil
	result := toolcontract.ObjectRef{ID: "tool-result-v2", Revision: 1, Digest: testkit.Digest("tool-result-v2")}
	state.Result = &result
	state.Digest = ""
	var err error
	state.Digest, err = core.CanonicalJSONDigest("praxis.tool-mcp.single-call-execution-state", "2.0.0", "ToolOwnerSingleCallExecutionStateV2", state)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	rowDigest, err := core.CanonicalJSONDigest("praxis.tool-mcp.sqlite-row", "v1", "ToolOwnerSingleCallExecutionStateV2", state)
	if err != nil {
		t.Fatal(err)
	}
	next := current
	next.StateRevision, next.StateDigest, next.StateJSON, next.RowDigest = int64(state.Revision), string(state.Digest), body, string(rowDigest)
	return next
}

func advanceOwnerEntryLeaseRowV2(t *testing.T, current OwnerEntryLeaseRowV2, holder, phase string, acquired, expires time.Time) OwnerEntryLeaseRowV2 {
	t.Helper()
	var lease ownerEntryLeaseBodyRowV2
	if err := core.DecodeStrictJSON(current.LeaseJSON, &lease); err != nil {
		t.Fatal(err)
	}
	lease.Revision++
	lease.HolderIncarnationID = holder
	lease.Phase = phase
	lease.AcquiredUnixNano = acquired.UnixNano()
	lease.ExpiresUnixNano = expires.UnixNano()
	lease.Digest = ""
	var err error
	lease.Digest, err = core.CanonicalJSONDigest("praxis.tool-mcp.single-call-entry-lease", "2.0.0", "ToolOwnerSingleCallEntryLeaseV2", lease)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(lease)
	if err != nil {
		t.Fatal(err)
	}
	rowDigest, err := core.CanonicalJSONDigest("praxis.tool-mcp.sqlite-row", "v1", "ToolOwnerSingleCallEntryLeaseV2", lease)
	if err != nil {
		t.Fatal(err)
	}
	return OwnerEntryLeaseRowV2{
		LeaseID: lease.ID, LeaseRevision: int64(lease.Revision), LeaseDigest: string(lease.Digest),
		ExecutionAttemptID: lease.ExecutionAttemptID, RequestKeyDigest: string(lease.RequestKey.Digest),
		RequestDigest: string(lease.RequestDigest), InputDigest: string(lease.ExecutionInputDigest),
		HolderIncarnationID: lease.HolderIncarnationID, Phase: lease.Phase,
		AcquiredUnixNano: lease.AcquiredUnixNano, ExpiresUnixNano: lease.ExpiresUnixNano,
		LeaseJSON: body, RowDigest: string(rowDigest),
	}
}
