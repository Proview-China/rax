package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

const governedModelTurnHistoryCreateTableV4 = `
CREATE TABLE governed_model_turn_v3_history (
  turn_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  attempt_digest TEXT NOT NULL UNIQUE,
  prepared_id TEXT NOT NULL,
  prepared_revision INTEGER NOT NULL CHECK(prepared_revision > 0),
  prepared_digest TEXT NOT NULL,
  current_id TEXT NOT NULL,
  current_revision INTEGER NOT NULL CHECK(current_revision > 0),
  current_digest TEXT NOT NULL,
  current_checked_unix_nano INTEGER NOT NULL CHECK(current_checked_unix_nano > 0),
  current_expires_unix_nano INTEGER NOT NULL CHECK(current_expires_unix_nano > 0),
  current_not_after_unix_nano INTEGER NOT NULL CHECK(current_not_after_unix_nano > 0),
  material_id TEXT NOT NULL,
  material_revision INTEGER NOT NULL CHECK(material_revision > 0),
  material_digest TEXT NOT NULL,
  attempt_request_digest TEXT NOT NULL,
  route_call_digest TEXT NOT NULL,
  dispatch_sequence INTEGER NOT NULL CHECK(dispatch_sequence > 0),
  provider_attempt_ordinal INTEGER NOT NULL CHECK(provider_attempt_ordinal > 0),
  expires_unix_nano INTEGER NOT NULL CHECK(expires_unix_nano > 0),
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(turn_id, revision)
)`

var governedModelTurnHistoryColumnsV4 = []schemaColumnV3{
	{name: "turn_id", kind: "TEXT", notNull: 1, pk: 1},
	{name: "revision", kind: "INTEGER", notNull: 1, pk: 2},
	{name: "fact_digest", kind: "TEXT", notNull: 1},
	{name: "attempt_digest", kind: "TEXT", notNull: 1},
	{name: "prepared_id", kind: "TEXT", notNull: 1},
	{name: "prepared_revision", kind: "INTEGER", notNull: 1},
	{name: "prepared_digest", kind: "TEXT", notNull: 1},
	{name: "current_id", kind: "TEXT", notNull: 1},
	{name: "current_revision", kind: "INTEGER", notNull: 1},
	{name: "current_digest", kind: "TEXT", notNull: 1},
	{name: "current_checked_unix_nano", kind: "INTEGER", notNull: 1},
	{name: "current_expires_unix_nano", kind: "INTEGER", notNull: 1},
	{name: "current_not_after_unix_nano", kind: "INTEGER", notNull: 1},
	{name: "material_id", kind: "TEXT", notNull: 1},
	{name: "material_revision", kind: "INTEGER", notNull: 1},
	{name: "material_digest", kind: "TEXT", notNull: 1},
	{name: "attempt_request_digest", kind: "TEXT", notNull: 1},
	{name: "route_call_digest", kind: "TEXT", notNull: 1},
	{name: "dispatch_sequence", kind: "INTEGER", notNull: 1},
	{name: "provider_attempt_ordinal", kind: "INTEGER", notNull: 1},
	{name: "expires_unix_nano", kind: "INTEGER", notNull: 1},
	{name: "canonical_json", kind: "BLOB", notNull: 1},
}

var governedModelTurnHistoryExactIndexColumnsV4 = []string{
	"turn_id",
	"revision",
	"fact_digest",
	"attempt_digest",
	"prepared_id",
	"prepared_revision",
	"prepared_digest",
	"current_id",
	"current_revision",
	"current_digest",
	"current_checked_unix_nano",
	"current_expires_unix_nano",
	"current_not_after_unix_nano",
	"material_id",
	"material_revision",
	"material_digest",
	"attempt_request_digest",
	"route_call_digest",
	"dispatch_sequence",
	"provider_attempt_ordinal",
	"expires_unix_nano",
}

var governedModelTurnConstraintProbeSequenceV4 atomic.Uint64

func (s *Store) verifyV4(ctx context.Context) error {
	if err := contextErrorV1(ctx, "verify_v4"); err != nil {
		return err
	}
	if s == nil || s.db == nil {
		return errorV1(
			modelinvoker.GovernedModelInvocationErrorUnavailable,
			"verify_v4",
			"sqlite repository is unavailable",
			nil,
		)
	}
	if err := verifyGovernedModelTurnHistoryTableV4(ctx, s.db); err != nil {
		return err
	}
	if err := verifyGovernedModelTurnHistoryColumnsV4(ctx, s.db); err != nil {
		return err
	}
	if err := verifyGovernedModelTurnHistoryConstraintsV4(ctx, s.db); err != nil {
		return err
	}
	if err := verifyGovernedModelTurnHistoryIndexesV4(ctx, s.db); err != nil {
		return err
	}
	var triggers int
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master
		 WHERE type='trigger' AND tbl_name='governed_model_turn_v3_history'`,
	).Scan(&triggers); err != nil {
		return mapDBErrorV1(ctx, "verify_v4", err, false)
	}
	if triggers != 0 {
		return schemaConflictV4("governed model turn V3 history triggers are forbidden")
	}
	var forbidden int
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master
		 WHERE type='table' AND name='governed_model_turn_v3_current'`,
	).Scan(&forbidden); err != nil {
		return mapDBErrorV1(ctx, "verify_v4", err, false)
	}
	if forbidden != 0 {
		return schemaConflictV4("forbidden governed model turn V3 current table exists")
	}
	return nil
}

func verifyGovernedModelTurnHistoryTableV4(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_list('governed_model_turn_v3_history')`)
	if err != nil {
		return mapDBErrorV1(ctx, "verify_v4", err, false)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var schema, name, kind string
		var columns, withoutRowID, strict int
		if err := rows.Scan(
			&schema,
			&name,
			&kind,
			&columns,
			&withoutRowID,
			&strict,
		); err != nil {
			return mapDBErrorV1(ctx, "verify_v4", err, false)
		}
		count++
		if schema != "main" || name != "governed_model_turn_v3_history" ||
			kind != "table" || columns != len(governedModelTurnHistoryColumnsV4) ||
			withoutRowID != 0 || strict != 0 {
			return schemaConflictV4("governed model turn V3 history table options drifted")
		}
	}
	if err := rows.Err(); err != nil {
		return mapDBErrorV1(ctx, "verify_v4", err, false)
	}
	if count != 1 {
		return schemaConflictV4("governed model turn V3 history table cardinality drifted")
	}
	var definition string
	if err := db.QueryRowContext(
		ctx,
		`SELECT sql FROM sqlite_master
		 WHERE type='table' AND name='governed_model_turn_v3_history'`,
	).Scan(&definition); err != nil {
		return mapDBErrorV1(ctx, "verify_v4", err, false)
	}
	actual, err := sqliteSchemaTokensV3(definition)
	if err != nil {
		return schemaConflictV4("governed model turn V3 history DDL cannot be tokenized")
	}
	expected, err := sqliteSchemaTokensV3(governedModelTurnHistoryCreateTableV4)
	if err != nil || !equalStringsV3(actual, expected) {
		return schemaConflictV4("governed model turn V3 history DDL drifted")
	}
	return nil
}

func verifyGovernedModelTurnHistoryColumnsV4(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_xinfo('governed_model_turn_v3_history')`)
	if err != nil {
		return mapDBErrorV1(ctx, "verify_v4", err, false)
	}
	defer rows.Close()
	actual := make([]schemaColumnV3, 0, len(governedModelTurnHistoryColumnsV4))
	for rows.Next() {
		var cid int
		var column schemaColumnV3
		var defaultValue sql.NullString
		if err := rows.Scan(
			&cid,
			&column.name,
			&column.kind,
			&column.notNull,
			&defaultValue,
			&column.pk,
			&column.hidden,
		); err != nil {
			return mapDBErrorV1(ctx, "verify_v4", err, false)
		}
		if cid != len(actual) || defaultValue.Valid {
			return schemaConflictV4("governed model turn V3 column order or default drifted")
		}
		column.kind = strings.ToUpper(strings.TrimSpace(column.kind))
		actual = append(actual, column)
	}
	if err := rows.Err(); err != nil {
		return mapDBErrorV1(ctx, "verify_v4", err, false)
	}
	if len(actual) != len(governedModelTurnHistoryColumnsV4) {
		return schemaConflictV4("governed model turn V3 column count drifted")
	}
	for index := range actual {
		if actual[index] != governedModelTurnHistoryColumnsV4[index] {
			return schemaConflictV4("governed model turn V3 column contract drifted")
		}
	}
	return nil
}

func verifyGovernedModelTurnHistoryConstraintsV4(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBErrorV1(ctx, "verify_v4", err, false)
	}
	defer func() { _ = tx.Rollback() }()
	type probeV4 struct {
		name            string
		revision        int64
		prepared        int64
		current         int64
		currentChecked  int64
		currentExpires  int64
		currentNotAfter int64
		material        int64
		dispatch        int64
		ordinal         int64
		expires         int64
	}
	valid := probeV4{
		revision: 1, prepared: 1, current: 1, currentChecked: 1,
		currentExpires: 2, currentNotAfter: 3, material: 1,
		dispatch: 1, ordinal: 1, expires: 2,
	}
	probes := []probeV4{
		func() probeV4 { p := valid; p.name = "revision"; p.revision = 0; return p }(),
		func() probeV4 { p := valid; p.name = "prepared"; p.prepared = 0; return p }(),
		func() probeV4 { p := valid; p.name = "current"; p.current = 0; return p }(),
		func() probeV4 { p := valid; p.name = "current-checked"; p.currentChecked = 0; return p }(),
		func() probeV4 { p := valid; p.name = "current-expires"; p.currentExpires = 0; return p }(),
		func() probeV4 { p := valid; p.name = "current-not-after"; p.currentNotAfter = 0; return p }(),
		func() probeV4 { p := valid; p.name = "material"; p.material = 0; return p }(),
		func() probeV4 { p := valid; p.name = "dispatch"; p.dispatch = 0; return p }(),
		func() probeV4 { p := valid; p.name = "ordinal"; p.ordinal = 0; return p }(),
		func() probeV4 { p := valid; p.name = "expires"; p.expires = 0; return p }(),
	}
	for _, probe := range probes {
		if err := insertGovernedModelTurnConstraintProbeV4(ctx, tx, probe); err == nil {
			return schemaConflictV4("governed model turn V3 accepted invalid " + probe.name)
		} else if !isSQLiteConstraintErrorV3(err) {
			if isSQLiteAnyConstraintErrorV3(err) {
				return schemaConflictV4("governed model turn V3 invalid probe hit non-CHECK constraint")
			}
			return mapDBErrorV1(ctx, "verify_v4", err, false)
		}
	}
	valid.name = "valid"
	if err := insertGovernedModelTurnConstraintProbeV4(ctx, tx, valid); err != nil {
		return mapDBErrorV1(ctx, "verify_v4", err, false)
	}
	return nil
}

func insertGovernedModelTurnConstraintProbeV4(
	ctx context.Context,
	tx *sql.Tx,
	probe struct {
		name            string
		revision        int64
		prepared        int64
		current         int64
		currentChecked  int64
		currentExpires  int64
		currentNotAfter int64
		material        int64
		dispatch        int64
		ordinal         int64
		expires         int64
	},
) error {
	sequence := governedModelTurnConstraintProbeSequenceV4.Add(1)
	id := fmt.Sprintf("schema-v4-probe-%s-%d-%d", probe.name, time.Now().UnixNano(), sequence)
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO governed_model_turn_v3_history(
		   turn_id,revision,fact_digest,attempt_digest,
		   prepared_id,prepared_revision,prepared_digest,
		   current_id,current_revision,current_digest,
		   current_checked_unix_nano,current_expires_unix_nano,current_not_after_unix_nano,
		   material_id,material_revision,material_digest,
		   attempt_request_digest,route_call_digest,
		   dispatch_sequence,provider_attempt_ordinal,
		   expires_unix_nano,canonical_json
		 ) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, probe.revision, "fact", id+"-attempt",
		"prepared", probe.prepared, "prepared-digest",
		"current", probe.current, "current-digest",
		probe.currentChecked, probe.currentExpires, probe.currentNotAfter,
		"material", probe.material, "material-digest",
		"request", "route", probe.dispatch, probe.ordinal, probe.expires, []byte(`{}`),
	)
	return err
}

func verifyGovernedModelTurnHistoryIndexesV4(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA index_list('governed_model_turn_v3_history')`)
	if err != nil {
		return mapDBErrorV1(ctx, "verify_v4", err, false)
	}
	defer rows.Close()
	type indexV4 struct {
		name    string
		unique  int
		origin  string
		partial int
	}
	var indexes []indexV4
	for rows.Next() {
		var sequence int
		var index indexV4
		if err := rows.Scan(
			&sequence,
			&index.name,
			&index.unique,
			&index.origin,
			&index.partial,
		); err != nil {
			return mapDBErrorV1(ctx, "verify_v4", err, false)
		}
		indexes = append(indexes, index)
	}
	if err := rows.Err(); err != nil {
		return mapDBErrorV1(ctx, "verify_v4", err, false)
	}
	if err := rows.Close(); err != nil {
		return mapDBErrorV1(ctx, "verify_v4", err, false)
	}
	if len(indexes) != 3 {
		return schemaConflictV4("governed model turn V3 index set drifted")
	}
	exact, attempt, primary := false, false, false
	for _, index := range indexes {
		if index.partial != 0 {
			return schemaConflictV4("governed model turn V3 partial index is forbidden")
		}
		columns, err := indexKeyColumnsV3(ctx, db, index.name)
		if err != nil {
			return err
		}
		switch index.origin {
		case "c":
			if exact || index.name != "governed_model_turn_v3_history_exact" ||
				index.unique != 0 ||
				!equalStringsV3(columns, governedModelTurnHistoryExactIndexColumnsV4) {
				return schemaConflictV4("governed model turn V3 exact index drifted")
			}
			exact = true
		case "u":
			if attempt || index.unique != 1 ||
				!equalStringsV3(columns, []string{"attempt_digest"}) {
				return schemaConflictV4("governed model turn V3 Attempt UNIQUE drifted")
			}
			attempt = true
		case "pk":
			if primary || index.unique != 1 ||
				!equalStringsV3(columns, []string{"turn_id", "revision"}) {
				return schemaConflictV4("governed model turn V3 primary key drifted")
			}
			primary = true
		default:
			return schemaConflictV4("governed model turn V3 index origin drifted")
		}
	}
	if !exact || !attempt || !primary {
		return schemaConflictV4("governed model turn V3 index set is incomplete")
	}
	return nil
}

func schemaConflictV4(message string) error {
	return errorV1(
		modelinvoker.GovernedModelInvocationErrorConflict,
		"verify_v4",
		message,
		nil,
	)
}
