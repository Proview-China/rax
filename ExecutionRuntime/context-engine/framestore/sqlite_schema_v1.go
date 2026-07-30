package framestore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	sqliteSchemaVersionV1 = 1
	sqliteSchemaV1        = `
CREATE TABLE IF NOT EXISTS context_frame_store_schema (
  version INTEGER NOT NULL PRIMARY KEY CHECK(version > 0),
  digest TEXT NOT NULL
) STRICT;
CREATE TABLE IF NOT EXISTS context_frame_store_history (
  owner_component_id TEXT NOT NULL,
  owner_binding_digest TEXT NOT NULL,
  execution_scope_digest TEXT NOT NULL,
  run_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  session_revision INTEGER NOT NULL CHECK(session_revision > 0),
  session_digest TEXT NOT NULL,
  turn INTEGER NOT NULL CHECK(turn > 0),
  source_kind TEXT NOT NULL,
  source_id TEXT NOT NULL,
  source_revision INTEGER NOT NULL CHECK(source_revision > 0),
  source_digest TEXT NOT NULL,
  frame_id TEXT NOT NULL,
  frame_revision INTEGER NOT NULL CHECK(frame_revision > 0),
  frame_digest TEXT NOT NULL,
  manifest_id TEXT NOT NULL,
  manifest_revision INTEGER NOT NULL CHECK(manifest_revision > 0),
  manifest_digest TEXT NOT NULL,
  generation_id TEXT NOT NULL,
  generation_revision INTEGER NOT NULL CHECK(generation_revision > 0),
  generation_digest TEXT NOT NULL,
  pointer_id TEXT NOT NULL,
  pointer_revision INTEGER NOT NULL CHECK(pointer_revision > 0),
  pointer_digest TEXT NOT NULL,
  expires_unix_nano INTEGER NOT NULL CHECK(expires_unix_nano > 0),
  row_digest TEXT NOT NULL,
  payload BLOB NOT NULL,
  PRIMARY KEY(owner_component_id,owner_binding_digest,execution_scope_digest,frame_id,frame_revision)
) STRICT;
CREATE UNIQUE INDEX IF NOT EXISTS context_frame_store_history_manifest_exact
  ON context_frame_store_history(owner_component_id,owner_binding_digest,execution_scope_digest,manifest_id,manifest_revision);
CREATE UNIQUE INDEX IF NOT EXISTS context_frame_store_history_frame_public_exact
  ON context_frame_store_history(owner_component_id,owner_binding_digest,frame_id,frame_revision);
CREATE UNIQUE INDEX IF NOT EXISTS context_frame_store_history_generation_exact
  ON context_frame_store_history(owner_component_id,owner_binding_digest,execution_scope_digest,generation_id,generation_revision);
CREATE UNIQUE INDEX IF NOT EXISTS context_frame_store_history_pointer_exact
  ON context_frame_store_history(owner_component_id,owner_binding_digest,execution_scope_digest,pointer_id,pointer_revision);
CREATE UNIQUE INDEX IF NOT EXISTS context_frame_store_history_source_exact
  ON context_frame_store_history(owner_component_id,owner_binding_digest,source_kind,source_id,source_revision,source_digest);
CREATE TABLE IF NOT EXISTS context_frame_store_current (
  owner_component_id TEXT NOT NULL,
  owner_binding_digest TEXT NOT NULL,
  execution_scope_digest TEXT NOT NULL,
  run_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  session_revision INTEGER NOT NULL CHECK(session_revision > 0),
  session_digest TEXT NOT NULL,
  turn INTEGER NOT NULL CHECK(turn > 0),
  frame_id TEXT NOT NULL,
  frame_revision INTEGER NOT NULL CHECK(frame_revision > 0),
  frame_digest TEXT NOT NULL,
  pointer_id TEXT NOT NULL,
  pointer_revision INTEGER NOT NULL CHECK(pointer_revision > 0),
  pointer_digest TEXT NOT NULL,
  highest_pointer_revision INTEGER NOT NULL CHECK(highest_pointer_revision > 0),
  PRIMARY KEY(owner_component_id,owner_binding_digest,execution_scope_digest,run_id,session_id,session_revision,session_digest),
  FOREIGN KEY(owner_component_id,owner_binding_digest,execution_scope_digest,frame_id,frame_revision)
    REFERENCES context_frame_store_history(owner_component_id,owner_binding_digest,execution_scope_digest,frame_id,frame_revision)
) STRICT;
CREATE INDEX IF NOT EXISTS context_frame_store_current_exact
  ON context_frame_store_current(owner_component_id,owner_binding_digest,execution_scope_digest,run_id,session_id,session_revision,session_digest,turn,pointer_id,pointer_revision,pointer_digest,frame_id,frame_revision,frame_digest,highest_pointer_revision);
CREATE TABLE IF NOT EXISTS context_frame_store_ledger (
  operation_id TEXT NOT NULL PRIMARY KEY,
  owner_component_id TEXT NOT NULL,
  owner_binding_digest TEXT NOT NULL,
  execution_scope_digest TEXT NOT NULL,
  run_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  session_revision INTEGER NOT NULL CHECK(session_revision > 0),
  session_digest TEXT NOT NULL,
  expected_pointer_id TEXT,
  expected_pointer_revision INTEGER,
  expected_pointer_digest TEXT,
  next_pointer_id TEXT NOT NULL,
  next_pointer_revision INTEGER NOT NULL CHECK(next_pointer_revision > 0),
  next_pointer_digest TEXT NOT NULL,
  frame_id TEXT NOT NULL,
  frame_revision INTEGER NOT NULL CHECK(frame_revision > 0),
  frame_digest TEXT NOT NULL,
  state_row_digest TEXT NOT NULL,
  created_unix_nano INTEGER NOT NULL CHECK(created_unix_nano > 0),
  row_digest TEXT NOT NULL,
  payload BLOB NOT NULL,
  CHECK(
    (expected_pointer_id IS NULL AND expected_pointer_revision IS NULL AND expected_pointer_digest IS NULL)
    OR
    (expected_pointer_id IS NOT NULL AND expected_pointer_revision > 0 AND expected_pointer_digest IS NOT NULL)
  ),
  FOREIGN KEY(owner_component_id,owner_binding_digest,execution_scope_digest,frame_id,frame_revision)
    REFERENCES context_frame_store_history(owner_component_id,owner_binding_digest,execution_scope_digest,frame_id,frame_revision)
) STRICT;
CREATE UNIQUE INDEX IF NOT EXISTS context_frame_store_ledger_next_pointer
  ON context_frame_store_ledger(owner_component_id,owner_binding_digest,execution_scope_digest,next_pointer_id,next_pointer_revision);
CREATE TRIGGER IF NOT EXISTS context_frame_store_history_no_update
BEFORE UPDATE ON context_frame_store_history BEGIN SELECT RAISE(ABORT,'context frame history is append-only'); END;
CREATE TRIGGER IF NOT EXISTS context_frame_store_history_no_delete
BEFORE DELETE ON context_frame_store_history BEGIN SELECT RAISE(ABORT,'context frame history is append-only'); END;
CREATE TRIGGER IF NOT EXISTS context_frame_store_ledger_no_update
BEFORE UPDATE ON context_frame_store_ledger BEGIN SELECT RAISE(ABORT,'context frame ledger is append-only'); END;
CREATE TRIGGER IF NOT EXISTS context_frame_store_ledger_no_delete
BEFORE DELETE ON context_frame_store_ledger BEGIN SELECT RAISE(ABORT,'context frame ledger is append-only'); END;
`
)

type physicalColumnV1 struct {
	name    string
	kind    string
	notNull bool
	pk      int
}

var physicalTablesV1 = map[string][]physicalColumnV1{
	"context_frame_store_schema": {
		{"version", "INTEGER", true, 1}, {"digest", "TEXT", true, 0},
	},
	"context_frame_store_history": {
		{"owner_component_id", "TEXT", true, 1}, {"owner_binding_digest", "TEXT", true, 2},
		{"execution_scope_digest", "TEXT", true, 3}, {"run_id", "TEXT", true, 0},
		{"session_id", "TEXT", true, 0}, {"session_revision", "INTEGER", true, 0},
		{"session_digest", "TEXT", true, 0}, {"turn", "INTEGER", true, 0},
		{"source_kind", "TEXT", true, 0}, {"source_id", "TEXT", true, 0},
		{"source_revision", "INTEGER", true, 0}, {"source_digest", "TEXT", true, 0},
		{"frame_id", "TEXT", true, 4}, {"frame_revision", "INTEGER", true, 5},
		{"frame_digest", "TEXT", true, 0}, {"manifest_id", "TEXT", true, 0},
		{"manifest_revision", "INTEGER", true, 0}, {"manifest_digest", "TEXT", true, 0},
		{"generation_id", "TEXT", true, 0}, {"generation_revision", "INTEGER", true, 0},
		{"generation_digest", "TEXT", true, 0}, {"pointer_id", "TEXT", true, 0},
		{"pointer_revision", "INTEGER", true, 0}, {"pointer_digest", "TEXT", true, 0},
		{"expires_unix_nano", "INTEGER", true, 0}, {"row_digest", "TEXT", true, 0},
		{"payload", "BLOB", true, 0},
	},
	"context_frame_store_current": {
		{"owner_component_id", "TEXT", true, 1}, {"owner_binding_digest", "TEXT", true, 2},
		{"execution_scope_digest", "TEXT", true, 3}, {"run_id", "TEXT", true, 4},
		{"session_id", "TEXT", true, 5}, {"session_revision", "INTEGER", true, 6},
		{"session_digest", "TEXT", true, 7}, {"turn", "INTEGER", true, 0},
		{"frame_id", "TEXT", true, 0}, {"frame_revision", "INTEGER", true, 0},
		{"frame_digest", "TEXT", true, 0}, {"pointer_id", "TEXT", true, 0},
		{"pointer_revision", "INTEGER", true, 0}, {"pointer_digest", "TEXT", true, 0},
		{"highest_pointer_revision", "INTEGER", true, 0},
	},
	"context_frame_store_ledger": {
		{"operation_id", "TEXT", true, 1}, {"owner_component_id", "TEXT", true, 0},
		{"owner_binding_digest", "TEXT", true, 0}, {"execution_scope_digest", "TEXT", true, 0},
		{"run_id", "TEXT", true, 0}, {"session_id", "TEXT", true, 0},
		{"session_revision", "INTEGER", true, 0}, {"session_digest", "TEXT", true, 0},
		{"expected_pointer_id", "TEXT", false, 0}, {"expected_pointer_revision", "INTEGER", false, 0},
		{"expected_pointer_digest", "TEXT", false, 0}, {"next_pointer_id", "TEXT", true, 0},
		{"next_pointer_revision", "INTEGER", true, 0}, {"next_pointer_digest", "TEXT", true, 0},
		{"frame_id", "TEXT", true, 0}, {"frame_revision", "INTEGER", true, 0},
		{"frame_digest", "TEXT", true, 0}, {"state_row_digest", "TEXT", true, 0}, {"created_unix_nano", "INTEGER", true, 0},
		{"row_digest", "TEXT", true, 0}, {"payload", "BLOB", true, 0},
	},
}

type physicalIndexV1 struct {
	table   string
	unique  bool
	columns []string
}

var physicalIndexesV1 = map[string]physicalIndexV1{
	"context_frame_store_history_manifest_exact":     {"context_frame_store_history", true, []string{"owner_component_id", "owner_binding_digest", "execution_scope_digest", "manifest_id", "manifest_revision"}},
	"context_frame_store_history_frame_public_exact": {"context_frame_store_history", true, []string{"owner_component_id", "owner_binding_digest", "frame_id", "frame_revision"}},
	"context_frame_store_history_generation_exact":   {"context_frame_store_history", true, []string{"owner_component_id", "owner_binding_digest", "execution_scope_digest", "generation_id", "generation_revision"}},
	"context_frame_store_history_pointer_exact":      {"context_frame_store_history", true, []string{"owner_component_id", "owner_binding_digest", "execution_scope_digest", "pointer_id", "pointer_revision"}},
	"context_frame_store_history_source_exact":       {"context_frame_store_history", true, []string{"owner_component_id", "owner_binding_digest", "source_kind", "source_id", "source_revision", "source_digest"}},
	"context_frame_store_current_exact":              {"context_frame_store_current", false, []string{"owner_component_id", "owner_binding_digest", "execution_scope_digest", "run_id", "session_id", "session_revision", "session_digest", "turn", "pointer_id", "pointer_revision", "pointer_digest", "frame_id", "frame_revision", "frame_digest", "highest_pointer_revision"}},
	"context_frame_store_ledger_next_pointer":        {"context_frame_store_ledger", true, []string{"owner_component_id", "owner_binding_digest", "execution_scope_digest", "next_pointer_id", "next_pointer_revision"}},
}

func verifyPhysicalSchemaV1(ctx context.Context, db *sql.DB) error {
	if err := verifyObjectClosureV1(ctx, db); err != nil {
		return err
	}
	for table, expected := range physicalTablesV1 {
		var strict int
		if err := db.QueryRowContext(ctx, `SELECT strict FROM pragma_table_list WHERE schema='main' AND type='table' AND name=?`, table).Scan(&strict); err != nil || strict != 1 {
			return fmt.Errorf("%w: frame store table %s is missing or not STRICT", contract.ErrConflict, table)
		}
		rows, err := db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_xinfo("%s")`, table))
		if err != nil {
			return mapSQLiteErrorV1(ctx, err, false)
		}
		var actual []physicalColumnV1
		var iterationErr error
		for rows.Next() {
			var cid, hidden int
			var name, kind string
			var notNull, pk int
			var defaultValue sql.NullString
			if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk, &hidden); err != nil {
				iterationErr = mapSQLiteErrorV1(ctx, err, false)
				break
			}
			if cid != len(actual) || defaultValue.Valid || hidden != 0 {
				iterationErr = fmt.Errorf("%w: frame store table %s hidden/default/cid drifted", contract.ErrConflict, table)
				break
			}
			actual = append(actual, physicalColumnV1{name, strings.ToUpper(kind), notNull == 1, pk})
		}
		if err := finishRowsV1(ctx, rows, iterationErr); err != nil {
			return err
		}
		if !equalPhysicalColumnsV1(actual, expected) {
			return fmt.Errorf("%w: frame store table %s physical columns drifted", contract.ErrConflict, table)
		}
	}
	for index, expected := range physicalIndexesV1 {
		var tableName string
		if err := db.QueryRowContext(ctx, `SELECT tbl_name FROM sqlite_master WHERE type='index' AND name=?`, index).Scan(&tableName); err != nil {
			return fmt.Errorf("%w: frame store index %s is missing", contract.ErrConflict, index)
		}
		if tableName != expected.table {
			return fmt.Errorf("%w: frame store index %s table drifted", contract.ErrConflict, index)
		}
		rows, err := db.QueryContext(ctx, fmt.Sprintf(`PRAGMA index_list("%s")`, tableName))
		if err != nil {
			return mapSQLiteErrorV1(ctx, err, false)
		}
		found := false
		var iterationErr error
		for rows.Next() {
			var unique int
			var sequence, partial int
			var name, origin string
			if err := rows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
				iterationErr = mapSQLiteErrorV1(ctx, err, false)
				break
			}
			if name == index {
				found = true
				if partial != 0 || origin != "c" || (unique == 1) != expected.unique {
					iterationErr = fmt.Errorf("%w: frame store index %s flags drifted", contract.ErrConflict, index)
					break
				}
			}
		}
		if err := finishRowsV1(ctx, rows, iterationErr); err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: frame store index %s physical definition is missing", contract.ErrConflict, index)
		}
		columnRows, err := db.QueryContext(ctx, fmt.Sprintf(`PRAGMA index_xinfo("%s")`, index))
		if err != nil {
			return mapSQLiteErrorV1(ctx, err, false)
		}
		var columns []string
		auxiliary := 0
		rowIndex := 0
		iterationErr = nil
		for columnRows.Next() {
			var sequence, cid, descending, key int
			var name sql.NullString
			var collation string
			if err := columnRows.Scan(&sequence, &cid, &name, &descending, &collation, &key); err != nil {
				iterationErr = mapSQLiteErrorV1(ctx, err, false)
				break
			}
			if sequence != rowIndex {
				iterationErr = fmt.Errorf("%w: frame store index %s sequence drifted", contract.ErrConflict, index)
				break
			}
			if key != 0 && key != 1 {
				iterationErr = fmt.Errorf("%w: frame store index %s key flag drifted", contract.ErrConflict, index)
				break
			}
			if descending != 0 || collation != "BINARY" {
				iterationErr = fmt.Errorf("%w: frame store index %s collation/order drifted", contract.ErrConflict, index)
				break
			}
			if key == 1 {
				if !name.Valid || cid < 0 || len(columns) >= len(expected.columns) ||
					name.String != expected.columns[len(columns)] {
					iterationErr = fmt.Errorf("%w: frame store index %s key shape drifted", contract.ErrConflict, index)
					break
				}
				columns = append(columns, name.String)
			} else {
				auxiliary++
				if name.Valid || cid != -1 || auxiliary != 1 || len(columns) != len(expected.columns) {
					iterationErr = fmt.Errorf("%w: frame store index %s auxiliary shape drifted", contract.ErrConflict, index)
					break
				}
			}
			rowIndex++
		}
		if err := finishRowsV1(ctx, columnRows, iterationErr); err != nil {
			return err
		}
		if !equalStringsV1(columns, expected.columns) || auxiliary != 1 {
			return fmt.Errorf("%w: frame store index %s columns drifted", contract.ErrConflict, index)
		}
	}
	expectedFK := []struct {
		id, sequence              int
		from, to                  string
		onUpdate, onDelete, match string
	}{
		{0, 0, "owner_component_id", "owner_component_id", "NO ACTION", "NO ACTION", "NONE"},
		{0, 1, "owner_binding_digest", "owner_binding_digest", "NO ACTION", "NO ACTION", "NONE"},
		{0, 2, "execution_scope_digest", "execution_scope_digest", "NO ACTION", "NO ACTION", "NONE"},
		{0, 3, "frame_id", "frame_id", "NO ACTION", "NO ACTION", "NONE"},
		{0, 4, "frame_revision", "frame_revision", "NO ACTION", "NO ACTION", "NONE"},
	}
	for _, table := range []string{"context_frame_store_current", "context_frame_store_ledger"} {
		rows, err := db.QueryContext(ctx, fmt.Sprintf(`PRAGMA foreign_key_list("%s")`, table))
		if err != nil {
			return mapSQLiteErrorV1(ctx, err, false)
		}
		var actual []struct {
			id, sequence              int
			from, to                  string
			onUpdate, onDelete, match string
		}
		var iterationErr error
		for rows.Next() {
			var id, sequence int
			var target, from, to, onUpdate, onDelete, match string
			if err := rows.Scan(&id, &sequence, &target, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				iterationErr = mapSQLiteErrorV1(ctx, err, false)
				break
			}
			if target != "context_frame_store_history" {
				iterationErr = fmt.Errorf("%w: frame store foreign key target drifted", contract.ErrConflict)
				break
			}
			actual = append(actual, struct {
				id, sequence              int
				from, to                  string
				onUpdate, onDelete, match string
			}{id, sequence, from, to, onUpdate, onDelete, match})
		}
		if err := finishRowsV1(ctx, rows, iterationErr); err != nil {
			return err
		}
		if len(actual) != len(expectedFK) {
			return fmt.Errorf("%w: frame store foreign key cardinality drifted", contract.ErrConflict)
		}
		for index := range expectedFK {
			want, got := expectedFK[index], actual[index]
			if want.id != got.id || want.sequence != got.sequence || want.from != got.from || want.to != got.to ||
				want.onUpdate != got.onUpdate || want.onDelete != got.onDelete || want.match != got.match {
				return fmt.Errorf("%w: frame store foreign key closure drifted", contract.ErrConflict)
			}
		}
	}
	if err := verifyCheckConstraintsPhysicallyV1(ctx, db); err != nil {
		return err
	}
	if err := verifyPositiveChainV1(ctx, db); err != nil {
		return err
	}
	return verifyAppendOnlyBehaviorV1(ctx, db)
}

func verifyObjectClosureV1(ctx context.Context, db *sql.DB) error {
	expectedDDL, err := expectedDDLCanonicalV1()
	if err != nil {
		return err
	}
	expected := map[string]struct {
		kind  string
		table string
	}{
		"context_frame_store_schema":                     {"table", "context_frame_store_schema"},
		"context_frame_store_history":                    {"table", "context_frame_store_history"},
		"context_frame_store_current":                    {"table", "context_frame_store_current"},
		"context_frame_store_ledger":                     {"table", "context_frame_store_ledger"},
		"context_frame_store_history_manifest_exact":     {"index", "context_frame_store_history"},
		"context_frame_store_history_frame_public_exact": {"index", "context_frame_store_history"},
		"context_frame_store_history_generation_exact":   {"index", "context_frame_store_history"},
		"context_frame_store_history_pointer_exact":      {"index", "context_frame_store_history"},
		"context_frame_store_history_source_exact":       {"index", "context_frame_store_history"},
		"context_frame_store_current_exact":              {"index", "context_frame_store_current"},
		"context_frame_store_ledger_next_pointer":        {"index", "context_frame_store_ledger"},
		"sqlite_autoindex_context_frame_store_history_1": {"index", "context_frame_store_history"},
		"sqlite_autoindex_context_frame_store_current_1": {"index", "context_frame_store_current"},
		"sqlite_autoindex_context_frame_store_ledger_1":  {"index", "context_frame_store_ledger"},
		"context_frame_store_history_no_update":          {"trigger", "context_frame_store_history"},
		"context_frame_store_history_no_delete":          {"trigger", "context_frame_store_history"},
		"context_frame_store_ledger_no_update":           {"trigger", "context_frame_store_ledger"},
		"context_frame_store_ledger_no_delete":           {"trigger", "context_frame_store_ledger"},
	}
	rows, err := db.QueryContext(ctx, `
SELECT type,name,tbl_name,sql FROM sqlite_master
WHERE type IN ('table','index','trigger')
  AND (
    name LIKE 'context_frame_store_%'
    OR name LIKE 'sqlite_autoindex_context_frame_store_%'
    OR tbl_name LIKE 'context_frame_store_%'
  )
ORDER BY type,name`)
	if err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	}
	seen := make(map[string]struct{}, len(expected))
	var iterationErr error
	for rows.Next() {
		var kind, name, table string
		var body sql.NullString
		if err := rows.Scan(&kind, &name, &table, &body); err != nil {
			iterationErr = mapSQLiteErrorV1(ctx, err, false)
			break
		}
		want, ok := expected[name]
		if !ok || want.kind != kind || want.table != table {
			iterationErr = fmt.Errorf("%w: frame store sqlite object %s drifted", contract.ErrConflict, name)
			break
		}
		if _, duplicate := seen[name]; duplicate {
			iterationErr = fmt.Errorf("%w: duplicate frame store sqlite object %s", contract.ErrConflict, name)
			break
		}
		expectedBody, hasDDL := expectedDDL[name]
		if strings.HasPrefix(name, "sqlite_autoindex_") {
			if body.Valid || hasDDL {
				iterationErr = fmt.Errorf("%w: frame store automatic index %s SQL drifted", contract.ErrConflict, name)
				break
			}
		} else {
			if !body.Valid || !hasDDL {
				iterationErr = fmt.Errorf("%w: frame store sqlite object %s DDL is missing", contract.ErrConflict, name)
				break
			}
			actualBody, canonicalErr := canonicalSQLV1(body.String)
			if canonicalErr != nil || actualBody != expectedBody {
				iterationErr = fmt.Errorf("%w: frame store sqlite object %s DDL drifted", contract.ErrConflict, name)
				break
			}
		}
		seen[name] = struct{}{}
	}
	if err := finishRowsV1(ctx, rows, iterationErr); err != nil {
		return err
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("%w: frame store sqlite object closure cardinality", contract.ErrConflict)
	}
	return nil
}

func finishRowsV1(ctx context.Context, rows *sql.Rows, iterationErr error) error {
	if rows == nil {
		return errors.Join(iterationErr, fmt.Errorf("%w: nil frame store sqlite rows", contract.ErrUnavailable))
	}
	if err := rows.Err(); err != nil {
		iterationErr = errors.Join(iterationErr, mapSQLiteErrorV1(ctx, err, false))
	}
	if err := rows.Close(); err != nil {
		iterationErr = errors.Join(iterationErr, mapSQLiteErrorV1(ctx, err, false))
	}
	return iterationErr
}

func expectedDDLCanonicalV1() (map[string]string, error) {
	tokens, err := tokenizeSQLV1(sqliteSchemaV1)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	starts := make([]int, 0)
	for index, token := range tokens {
		if token == "create" {
			starts = append(starts, index)
		}
	}
	if len(starts) == 0 || starts[0] != 0 {
		return nil, fmt.Errorf("%w: frame store expected DDL start", contract.ErrConflict)
	}
	starts = append(starts, len(tokens))
	for index := 0; index+1 < len(starts); index++ {
		statement := append([]string(nil), tokens[starts[index]:starts[index+1]]...)
		for len(statement) > 0 && statement[len(statement)-1] == ";" {
			statement = statement[:len(statement)-1]
		}
		statement = removeIfNotExistsV1(statement)
		if len(statement) > 0 {
			name, ok := createdObjectNameV1(statement)
			if !ok {
				return nil, fmt.Errorf("%w: frame store expected DDL statement", contract.ErrConflict)
			}
			if _, duplicate := result[name]; duplicate {
				return nil, fmt.Errorf("%w: duplicate frame store expected DDL %s", contract.ErrConflict, name)
			}
			result[name] = strings.Join(statement, "\x1f")
		}
	}
	return result, nil
}

func canonicalSQLV1(value string) (string, error) {
	tokens, err := tokenizeSQLV1(value)
	if err != nil {
		return "", err
	}
	for len(tokens) > 0 && tokens[len(tokens)-1] == ";" {
		tokens = tokens[:len(tokens)-1]
	}
	return strings.Join(removeIfNotExistsV1(tokens), "\x1f"), nil
}

func removeIfNotExistsV1(tokens []string) []string {
	result := make([]string, 0, len(tokens))
	for index := 0; index < len(tokens); index++ {
		if index+2 < len(tokens) && tokens[index] == "if" && tokens[index+1] == "not" && tokens[index+2] == "exists" {
			index += 2
			continue
		}
		result = append(result, tokens[index])
	}
	return result
}

func createdObjectNameV1(tokens []string) (string, bool) {
	if len(tokens) < 3 || tokens[0] != "create" {
		return "", false
	}
	if tokens[1] == "unique" {
		if len(tokens) < 4 || tokens[2] != "index" {
			return "", false
		}
		return tokens[3], true
	}
	switch tokens[1] {
	case "table", "index", "trigger":
		return tokens[2], true
	default:
		return "", false
	}
}

func tokenizeSQLV1(value string) ([]string, error) {
	tokens := make([]string, 0, len(value)/4)
	for index := 0; index < len(value); {
		switch {
		case isSQLSpaceV1(value[index]):
			index++
		case index+1 < len(value) && value[index] == '-' && value[index+1] == '-':
			index += 2
			for index < len(value) && value[index] != '\n' {
				index++
			}
		case index+1 < len(value) && value[index] == '/' && value[index+1] == '*':
			end := strings.Index(value[index+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("%w: unterminated frame store SQL comment", contract.ErrConflict)
			}
			index += end + 4
		case value[index] == '\'' || value[index] == '"' || value[index] == '`':
			start, quote := index, value[index]
			index++
			closed := false
			for index < len(value) {
				if value[index] == quote {
					if index+1 < len(value) && value[index+1] == quote {
						index += 2
						continue
					}
					index++
					closed = true
					break
				}
				index++
			}
			if !closed {
				return nil, fmt.Errorf("%w: unterminated frame store SQL quote", contract.ErrConflict)
			}
			tokens = append(tokens, value[start:index])
		case value[index] == '[':
			start := index
			end := strings.IndexByte(value[index+1:], ']')
			if end < 0 {
				return nil, fmt.Errorf("%w: unterminated frame store SQL identifier", contract.ErrConflict)
			}
			index += end + 2
			tokens = append(tokens, value[start:index])
		case isSQLPunctuationV1(value[index]):
			if index+1 < len(value) && isSQLPairV1(value[index], value[index+1]) {
				tokens = append(tokens, value[index:index+2])
				index += 2
			} else {
				tokens = append(tokens, value[index:index+1])
				index++
			}
		default:
			start := index
			for index < len(value) && !isSQLSpaceV1(value[index]) &&
				!isSQLPunctuationV1(value[index]) && value[index] != '\'' &&
				value[index] != '"' && value[index] != '`' && value[index] != '[' {
				if index+1 < len(value) &&
					((value[index] == '-' && value[index+1] == '-') ||
						(value[index] == '/' && value[index+1] == '*')) {
					break
				}
				index++
			}
			if start == index {
				return nil, fmt.Errorf("%w: frame store SQL token", contract.ErrConflict)
			}
			tokens = append(tokens, strings.ToLower(value[start:index]))
		}
	}
	return tokens, nil
}

func isSQLSpaceV1(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n' || value == '\f'
}

func isSQLPunctuationV1(value byte) bool {
	return strings.ContainsRune("(),;=<>!+-*/|.", rune(value))
}

func isSQLPairV1(left, right byte) bool {
	return (left == '>' && right == '=') || (left == '<' && (right == '=' || right == '>')) ||
		(left == '!' && right == '=') || (left == '|' && right == '|')
}

func verifyCheckConstraintsPhysicallyV1(ctx context.Context, db *sql.DB) error {
	nonce, negative, err := physicalProbeNonceV1()
	if err != nil {
		return err
	}
	if err := expectConstraintCodeV1(
		execProbeV1(ctx, db, `INSERT INTO context_frame_store_schema(version,digest) VALUES(?,?)`, negative, nonce),
		sqlite3.SQLITE_CONSTRAINT_CHECK,
		"schema.version",
	); err != nil {
		return err
	}
	historyColumns := []struct {
		name  string
		index int
	}{
		{"session_revision", 5}, {"turn", 7}, {"source_revision", 10},
		{"frame_revision", 13}, {"manifest_revision", 16}, {"generation_revision", 19},
		{"pointer_revision", 22}, {"expires_unix_nano", 24},
	}
	for _, probe := range historyColumns {
		args := historyProbeArgsV1(nonce + "-" + probe.name)
		args[probe.index] = int64(0)
		if err := expectConstraintCodeV1(
			execProbeV1(ctx, db, historyProbeInsertV1, args...),
			sqlite3.SQLITE_CONSTRAINT_CHECK,
			"history."+probe.name,
		); err != nil {
			return err
		}
	}
	currentColumns := []struct {
		name  string
		index int
	}{
		{"session_revision", 5}, {"turn", 7}, {"frame_revision", 9},
		{"pointer_revision", 12}, {"highest_pointer_revision", 14},
	}
	for _, probe := range currentColumns {
		suffix := nonce + "-current-" + probe.name
		args := currentProbeArgsV1(suffix)
		args[probe.index] = int64(0)
		if err := expectProbeWithHistoryCodeV1(ctx, db, suffix, currentProbeInsertV1, args, sqlite3.SQLITE_CONSTRAINT_CHECK, "current."+probe.name); err != nil {
			return err
		}
	}
	ledgerColumns := []struct {
		name  string
		index int
	}{
		{"session_revision", 6}, {"next_pointer_revision", 12},
		{"frame_revision", 15}, {"created_unix_nano", 18},
	}
	for _, probe := range ledgerColumns {
		suffix := nonce + "-ledger-" + probe.name
		args := ledgerProbeArgsV1(suffix)
		args[probe.index] = int64(0)
		if err := expectProbeWithHistoryCodeV1(ctx, db, suffix, ledgerProbeInsertV1, args, sqlite3.SQLITE_CONSTRAINT_CHECK, "ledger."+probe.name); err != nil {
			return err
		}
	}
	partial := ledgerProbeArgsV1(nonce + "-ledger-expected-partial")
	partial[8], partial[9], partial[10] = nonce, nil, nil
	return expectProbeWithHistoryCodeV1(ctx, db, nonce+"-ledger-expected-partial", ledgerProbeInsertV1, partial, sqlite3.SQLITE_CONSTRAINT_CHECK, "ledger.expected_pointer_closure")
}

func verifyPositiveChainV1(ctx context.Context, db *sql.DB) error {
	nonce, _, err := physicalProbeNonceV1()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, historyProbeInsertV1, historyProbeArgsV1(nonce)...); err != nil {
		return fmt.Errorf("%w: frame store positive history probe", contract.ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, currentProbeInsertV1, currentProbeArgsV1(nonce)...); err != nil {
		return fmt.Errorf("%w: frame store positive current probe", contract.ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, ledgerProbeInsertV1, ledgerProbeArgsV1(nonce)...); err != nil {
		return fmt.Errorf("%w: frame store positive ledger probe", contract.ErrConflict)
	}
	for _, table := range []string{
		"context_frame_store_history",
		"context_frame_store_current",
		"context_frame_store_ledger",
	} {
		var count int
		if err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM "%s" WHERE owner_component_id=?`, table), nonce).Scan(&count); err != nil {
			return mapSQLiteErrorV1(ctx, err, false)
		}
		if count != 1 {
			return fmt.Errorf("%w: frame store positive %s probe visibility", contract.ErrConflict, table)
		}
	}
	return nil
}

func verifyAppendOnlyBehaviorV1(ctx context.Context, db *sql.DB) error {
	nonce, _, err := physicalProbeNonceV1()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, historyProbeInsertV1, historyProbeArgsV1(nonce)...); err != nil {
		return fmt.Errorf("%w: frame store append-only history probe setup", contract.ErrConflict)
	}
	if _, err := tx.ExecContext(ctx, ledgerProbeInsertV1, ledgerProbeArgsV1(nonce)...); err != nil {
		return fmt.Errorf("%w: frame store append-only ledger probe setup", contract.ErrConflict)
	}
	for _, probe := range []struct {
		name  string
		query string
		args  []any
	}{
		{"history_update", `UPDATE context_frame_store_history SET row_digest=? WHERE frame_id=?`, []any{"changed", nonce}},
		{"history_delete", `DELETE FROM context_frame_store_history WHERE frame_id=?`, []any{nonce}},
		{"ledger_update", `UPDATE context_frame_store_ledger SET row_digest=? WHERE operation_id=?`, []any{"changed", nonce}},
		{"ledger_delete", `DELETE FROM context_frame_store_ledger WHERE operation_id=?`, []any{nonce}},
	} {
		_, execErr := tx.ExecContext(ctx, probe.query, probe.args...)
		if err := expectConstraintCodeV1(execErr, sqlite3.SQLITE_CONSTRAINT_TRIGGER, probe.name); err != nil {
			return err
		}
	}
	return nil
}

const historyProbeInsertV1 = `
INSERT INTO context_frame_store_history(
 owner_component_id,owner_binding_digest,execution_scope_digest,run_id,
 session_id,session_revision,session_digest,turn,
 source_kind,source_id,source_revision,source_digest,
 frame_id,frame_revision,frame_digest,
 manifest_id,manifest_revision,manifest_digest,
 generation_id,generation_revision,generation_digest,
 pointer_id,pointer_revision,pointer_digest,expires_unix_nano,row_digest,payload
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

const currentProbeInsertV1 = `
INSERT INTO context_frame_store_current(
 owner_component_id,owner_binding_digest,execution_scope_digest,run_id,
 session_id,session_revision,session_digest,turn,
 frame_id,frame_revision,frame_digest,pointer_id,pointer_revision,pointer_digest,highest_pointer_revision
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

const ledgerProbeInsertV1 = `
INSERT INTO context_frame_store_ledger(
 operation_id,owner_component_id,owner_binding_digest,execution_scope_digest,run_id,
 session_id,session_revision,session_digest,
 expected_pointer_id,expected_pointer_revision,expected_pointer_digest,
 next_pointer_id,next_pointer_revision,next_pointer_digest,
 frame_id,frame_revision,frame_digest,state_row_digest,created_unix_nano,row_digest,payload
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

func historyProbeArgsV1(nonce string) []any {
	return []any{
		nonce, nonce, nonce, nonce,
		nonce, int64(1), nonce, int64(1),
		nonce, nonce, int64(1), nonce,
		nonce, int64(1), nonce,
		nonce, int64(1), nonce,
		nonce, int64(1), nonce,
		nonce, int64(1), nonce, int64(1), nonce, []byte(`{}`),
	}
}

func currentProbeArgsV1(nonce string) []any {
	return []any{
		nonce, nonce, nonce, nonce,
		nonce, int64(1), nonce, int64(1),
		nonce, int64(1), nonce,
		nonce, int64(1), nonce, int64(1),
	}
}

func ledgerProbeArgsV1(nonce string) []any {
	return []any{
		nonce, nonce, nonce, nonce, nonce,
		nonce, int64(1), nonce,
		nil, nil, nil,
		nonce, int64(1), nonce,
		nonce, int64(1), nonce, nonce, int64(1), nonce, []byte(`{}`),
	}
}

func expectProbeWithHistoryCodeV1(ctx context.Context, db *sql.DB, nonce, query string, args []any, code int, name string) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, historyProbeInsertV1, historyProbeArgsV1(nonce)...); err != nil {
		return fmt.Errorf("%w: frame store %s probe history setup", contract.ErrConflict, name)
	}
	_, err = tx.ExecContext(ctx, query, args...)
	return expectConstraintCodeV1(err, code, name)
}

func execProbeV1(ctx context.Context, db *sql.DB, query string, args ...any) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, query, args...)
	return err
}

func expectConstraintCodeV1(err error, code int, name string) error {
	if err == nil {
		return fmt.Errorf("%w: frame store %s constraint is not physically enforced", contract.ErrConflict, name)
	}
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) || sqliteErr.Code() != code {
		return fmt.Errorf("%w: frame store %s failed with code %v, want %d", contract.ErrConflict, name, sqliteErrorCodeV1(err), code)
	}
	return nil
}

func sqliteErrorCodeV1(err error) int {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code()
	}
	return -1
}

func physicalProbeNonceV1() (string, int64, error) {
	var randomBytes [16]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return "", 0, fmt.Errorf("%w: frame store schema probe randomness", contract.ErrUnavailable)
	}
	nonce := fmt.Sprintf("physical-%x", randomBytes[:])
	negative := -int64(binary.BigEndian.Uint64(randomBytes[:8])&0x3fffffffffffffff) - 2
	return nonce, negative, nil
}

func equalPhysicalColumnsV1(left, right []physicalColumnV1) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalStringsV1(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
