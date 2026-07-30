package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

const governedModelTurnProviderBoundaryCreateTableV5 = `
CREATE TABLE governed_model_turn_v3_provider_boundary_history (
  boundary_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  fact_digest TEXT NOT NULL,
  ref_digest TEXT NOT NULL,
  turn_attempt_digest TEXT NOT NULL UNIQUE,
  runtime_boundary_digest TEXT NOT NULL UNIQUE,
  runtime_request_digest TEXT NOT NULL,
  turn_id TEXT NOT NULL,
  turn_revision INTEGER NOT NULL CHECK(turn_revision > 0),
  turn_digest TEXT NOT NULL,
  ack_id TEXT NOT NULL,
  ack_revision INTEGER NOT NULL CHECK(ack_revision > 0),
  ack_digest TEXT NOT NULL,
  dispatch_receipt_id TEXT NOT NULL,
  dispatch_receipt_revision INTEGER NOT NULL CHECK(dispatch_receipt_revision > 0),
  dispatch_receipt_digest TEXT NOT NULL,
  operation_digest TEXT NOT NULL,
  effect_id TEXT NOT NULL,
  runtime_attempt_id TEXT NOT NULL,
  dispatch_sequence INTEGER NOT NULL CHECK(dispatch_sequence > 0),
  provider_attempt_ordinal INTEGER NOT NULL CHECK(provider_attempt_ordinal > 0),
  attempt_request_digest TEXT NOT NULL,
  provider_binding_set_id TEXT NOT NULL,
  provider_binding_set_revision INTEGER NOT NULL CHECK(provider_binding_set_revision > 0),
  provider_component_id TEXT NOT NULL,
  provider_manifest_digest TEXT NOT NULL,
  provider_artifact_digest TEXT NOT NULL,
  provider_capability TEXT NOT NULL,
  turn_expires_unix_nano INTEGER NOT NULL CHECK(turn_expires_unix_nano > 0),
  checked_unix_nano INTEGER NOT NULL CHECK(checked_unix_nano > 0),
  expires_unix_nano INTEGER NOT NULL CHECK(expires_unix_nano > 0),
  projection_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(boundary_id, revision)
)`

var governedModelTurnProviderBoundaryColumnsV5 = []schemaColumnV3{
	{name: "boundary_id", kind: "TEXT", notNull: 1, pk: 1},
	{name: "revision", kind: "INTEGER", notNull: 1, pk: 2},
	{name: "fact_digest", kind: "TEXT", notNull: 1},
	{name: "ref_digest", kind: "TEXT", notNull: 1},
	{name: "turn_attempt_digest", kind: "TEXT", notNull: 1},
	{name: "runtime_boundary_digest", kind: "TEXT", notNull: 1},
	{name: "runtime_request_digest", kind: "TEXT", notNull: 1},
	{name: "turn_id", kind: "TEXT", notNull: 1},
	{name: "turn_revision", kind: "INTEGER", notNull: 1},
	{name: "turn_digest", kind: "TEXT", notNull: 1},
	{name: "ack_id", kind: "TEXT", notNull: 1},
	{name: "ack_revision", kind: "INTEGER", notNull: 1},
	{name: "ack_digest", kind: "TEXT", notNull: 1},
	{name: "dispatch_receipt_id", kind: "TEXT", notNull: 1},
	{name: "dispatch_receipt_revision", kind: "INTEGER", notNull: 1},
	{name: "dispatch_receipt_digest", kind: "TEXT", notNull: 1},
	{name: "operation_digest", kind: "TEXT", notNull: 1},
	{name: "effect_id", kind: "TEXT", notNull: 1},
	{name: "runtime_attempt_id", kind: "TEXT", notNull: 1},
	{name: "dispatch_sequence", kind: "INTEGER", notNull: 1},
	{name: "provider_attempt_ordinal", kind: "INTEGER", notNull: 1},
	{name: "attempt_request_digest", kind: "TEXT", notNull: 1},
	{name: "provider_binding_set_id", kind: "TEXT", notNull: 1},
	{name: "provider_binding_set_revision", kind: "INTEGER", notNull: 1},
	{name: "provider_component_id", kind: "TEXT", notNull: 1},
	{name: "provider_manifest_digest", kind: "TEXT", notNull: 1},
	{name: "provider_artifact_digest", kind: "TEXT", notNull: 1},
	{name: "provider_capability", kind: "TEXT", notNull: 1},
	{name: "turn_expires_unix_nano", kind: "INTEGER", notNull: 1},
	{name: "checked_unix_nano", kind: "INTEGER", notNull: 1},
	{name: "expires_unix_nano", kind: "INTEGER", notNull: 1},
	{name: "projection_digest", kind: "TEXT", notNull: 1},
	{name: "canonical_json", kind: "BLOB", notNull: 1},
}

var governedModelTurnProviderBoundaryExactIndexColumnsV5 = []string{
	"boundary_id", "revision", "fact_digest", "ref_digest",
	"turn_attempt_digest", "runtime_boundary_digest", "runtime_request_digest",
	"turn_id", "turn_revision", "turn_digest",
	"ack_id", "ack_revision", "ack_digest",
	"dispatch_receipt_id", "dispatch_receipt_revision", "dispatch_receipt_digest",
	"operation_digest", "effect_id", "runtime_attempt_id",
	"dispatch_sequence", "provider_attempt_ordinal", "attempt_request_digest",
	"provider_binding_set_id", "provider_binding_set_revision",
	"provider_component_id", "provider_manifest_digest",
	"provider_artifact_digest", "provider_capability",
	"turn_expires_unix_nano",
	"checked_unix_nano", "expires_unix_nano", "projection_digest",
}

var governedModelTurnProviderBoundaryProbeSequenceV5 atomic.Uint64

func (s *Store) verifyV5(ctx context.Context) error {
	if err := contextErrorV1(ctx, "verify_v5"); err != nil {
		return err
	}
	if s == nil || s.db == nil {
		return schemaConflictV5("sqlite repository is unavailable")
	}
	if err := verifyProviderBoundaryTableV5(ctx, s.db); err != nil {
		return err
	}
	if err := verifyProviderBoundaryColumnsV5(ctx, s.db); err != nil {
		return err
	}
	if err := verifyProviderBoundaryConstraintsV5(ctx, s.db); err != nil {
		return err
	}
	if err := verifyProviderBoundaryIndexesV5(ctx, s.db); err != nil {
		return err
	}
	var triggers int
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master
		 WHERE type='trigger'
		   AND tbl_name='governed_model_turn_v3_provider_boundary_history'`,
	).Scan(&triggers); err != nil {
		return mapDBErrorV1(ctx, "verify_v5", err, false)
	}
	if triggers != 0 {
		return schemaConflictV5("provider boundary V3 triggers are forbidden")
	}
	var current int
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master
		 WHERE type='table'
		   AND name='governed_model_turn_v3_provider_boundary_current'`,
	).Scan(&current); err != nil {
		return mapDBErrorV1(ctx, "verify_v5", err, false)
	}
	if current != 0 {
		return schemaConflictV5("provider boundary V3 current table is forbidden")
	}
	return nil
}

func verifyProviderBoundaryTableV5(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(
		ctx,
		`PRAGMA table_list('governed_model_turn_v3_provider_boundary_history')`,
	)
	if err != nil {
		return mapDBErrorV1(ctx, "verify_v5", err, false)
	}
	count := 0
	var scanErr error
	var contractErr error
	for rows.Next() {
		var schema, name, kind string
		var columns, withoutRowID, strict int
		if err := rows.Scan(
			&schema, &name, &kind, &columns, &withoutRowID, &strict,
		); err != nil {
			scanErr = err
			break
		}
		count++
		if schema != "main" ||
			name != "governed_model_turn_v3_provider_boundary_history" ||
			kind != "table" ||
			columns != len(governedModelTurnProviderBoundaryColumnsV5) ||
			withoutRowID != 0 || strict != 0 {
			contractErr = schemaConflictV5("provider boundary V3 table options drifted")
			break
		}
	}
	iterationErr := errors.Join(scanErr, rows.Err())
	closeErr := rows.Close()
	if err := errors.Join(iterationErr, closeErr); err != nil {
		return mapDBErrorV1(ctx, "verify_v5", err, false)
	}
	if contractErr != nil {
		return contractErr
	}
	if count != 1 {
		return schemaConflictV5("provider boundary V3 table cardinality drifted")
	}
	var definition string
	if err := db.QueryRowContext(
		ctx,
		`SELECT sql FROM sqlite_master
		 WHERE type='table'
		   AND name='governed_model_turn_v3_provider_boundary_history'`,
	).Scan(&definition); err != nil {
		return mapDBErrorV1(ctx, "verify_v5", err, false)
	}
	actual, err := sqliteSchemaTokensV3(definition)
	if err != nil {
		return schemaConflictV5("provider boundary V3 DDL cannot be tokenized")
	}
	expected, err := sqliteSchemaTokensV3(governedModelTurnProviderBoundaryCreateTableV5)
	if err != nil || !equalStringsV3(actual, expected) {
		return schemaConflictV5("provider boundary V3 DDL drifted")
	}
	return nil
}

func verifyProviderBoundaryColumnsV5(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(
		ctx,
		`PRAGMA table_xinfo('governed_model_turn_v3_provider_boundary_history')`,
	)
	if err != nil {
		return mapDBErrorV1(ctx, "verify_v5", err, false)
	}
	actual := make([]schemaColumnV3, 0, len(governedModelTurnProviderBoundaryColumnsV5))
	var scanErr error
	var contractErr error
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
			scanErr = err
			break
		}
		if cid != len(actual) || defaultValue.Valid {
			contractErr = schemaConflictV5("provider boundary V3 column order drifted")
			break
		}
		column.kind = strings.ToUpper(strings.TrimSpace(column.kind))
		actual = append(actual, column)
	}
	iterationErr := errors.Join(scanErr, rows.Err())
	closeErr := rows.Close()
	if err := errors.Join(iterationErr, closeErr); err != nil {
		return mapDBErrorV1(ctx, "verify_v5", err, false)
	}
	if contractErr != nil {
		return contractErr
	}
	if len(actual) != len(governedModelTurnProviderBoundaryColumnsV5) {
		return schemaConflictV5("provider boundary V3 column count drifted")
	}
	for index := range actual {
		if actual[index] != governedModelTurnProviderBoundaryColumnsV5[index] {
			return schemaConflictV5("provider boundary V3 column contract drifted")
		}
	}
	return nil
}

func verifyProviderBoundaryConstraintsV5(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBErrorV1(ctx, "verify_v5", err, false)
	}
	defer func() { _ = tx.Rollback() }()
	type probe struct {
		name               string
		revision           int64
		turnRevision       int64
		ackRevision        int64
		receiptRevision    int64
		dispatchSequence   int64
		ordinal            int64
		bindingSetRevision int64
		turnExpires        int64
		checked            int64
		expires            int64
	}
	valid := probe{
		revision: 1, turnRevision: 1, ackRevision: 1, receiptRevision: 1,
		dispatchSequence: 1, ordinal: 1, bindingSetRevision: 1,
		turnExpires: 3, checked: 1, expires: 2,
	}
	probes := []probe{
		func() probe { p := valid; p.name = "revision"; p.revision = 0; return p }(),
		func() probe { p := valid; p.name = "turn"; p.turnRevision = 0; return p }(),
		func() probe { p := valid; p.name = "ack"; p.ackRevision = 0; return p }(),
		func() probe { p := valid; p.name = "receipt"; p.receiptRevision = 0; return p }(),
		func() probe { p := valid; p.name = "sequence"; p.dispatchSequence = 0; return p }(),
		func() probe { p := valid; p.name = "ordinal"; p.ordinal = 0; return p }(),
		func() probe { p := valid; p.name = "binding"; p.bindingSetRevision = 0; return p }(),
		func() probe { p := valid; p.name = "turn-expires"; p.turnExpires = 0; return p }(),
		func() probe { p := valid; p.name = "checked"; p.checked = 0; return p }(),
		func() probe { p := valid; p.name = "expires"; p.expires = 0; return p }(),
	}
	for _, candidate := range probes {
		if err := insertProviderBoundaryProbeV5(ctx, tx, candidate); err == nil {
			return schemaConflictV5("provider boundary V3 accepted invalid " + candidate.name)
		} else if !isSQLiteConstraintErrorV3(err) {
			if isSQLiteAnyConstraintErrorV3(err) {
				return schemaConflictV5("provider boundary V3 invalid probe hit non-CHECK constraint")
			}
			return mapDBErrorV1(ctx, "verify_v5", err, false)
		}
	}
	valid.name = "valid"
	if err := insertProviderBoundaryProbeV5(ctx, tx, valid); err != nil {
		return mapDBErrorV1(ctx, "verify_v5", err, false)
	}
	return nil
}

func insertProviderBoundaryProbeV5(
	ctx context.Context,
	tx *sql.Tx,
	probe struct {
		name               string
		revision           int64
		turnRevision       int64
		ackRevision        int64
		receiptRevision    int64
		dispatchSequence   int64
		ordinal            int64
		bindingSetRevision int64
		turnExpires        int64
		checked            int64
		expires            int64
	},
) error {
	sequence := governedModelTurnProviderBoundaryProbeSequenceV5.Add(1)
	id := fmt.Sprintf("schema-v5-%s-%d-%d", probe.name, time.Now().UnixNano(), sequence)
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO governed_model_turn_v3_provider_boundary_history(
		   boundary_id,revision,fact_digest,ref_digest,
		   turn_attempt_digest,runtime_boundary_digest,runtime_request_digest,
		   turn_id,turn_revision,turn_digest,
		   ack_id,ack_revision,ack_digest,
		   dispatch_receipt_id,dispatch_receipt_revision,dispatch_receipt_digest,
		   operation_digest,effect_id,runtime_attempt_id,
		   dispatch_sequence,provider_attempt_ordinal,attempt_request_digest,
		   provider_binding_set_id,provider_binding_set_revision,
		   provider_component_id,provider_manifest_digest,
		   provider_artifact_digest,provider_capability,
		   turn_expires_unix_nano,checked_unix_nano,expires_unix_nano,
		   projection_digest,canonical_json
		 ) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, probe.revision, "fact", "ref",
		id+"-turn-attempt", id+"-runtime-boundary", "request",
		"turn", probe.turnRevision, "turn-digest",
		"ack", probe.ackRevision, "ack-digest",
		"receipt", probe.receiptRevision, "receipt-digest",
		"operation", "effect", "runtime-attempt",
		probe.dispatchSequence, probe.ordinal, "attempt-request",
		"binding-set", probe.bindingSetRevision,
		"praxis.model/provider", "manifest",
		"artifact", "praxis.model/invoke",
		probe.turnExpires, probe.checked, probe.expires, "projection", []byte(`{}`),
	)
	return err
}

func verifyProviderBoundaryIndexesV5(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(
		ctx,
		`PRAGMA index_list('governed_model_turn_v3_provider_boundary_history')`,
	)
	if err != nil {
		return mapDBErrorV1(ctx, "verify_v5", err, false)
	}
	type index struct {
		name    string
		unique  int
		origin  string
		partial int
	}
	var indexes []index
	var scanErr error
	for rows.Next() {
		var sequence int
		var value index
		if err := rows.Scan(
			&sequence,
			&value.name,
			&value.unique,
			&value.origin,
			&value.partial,
		); err != nil {
			scanErr = err
			break
		}
		indexes = append(indexes, value)
	}
	iterationErr := errors.Join(scanErr, rows.Err())
	closeErr := rows.Close()
	if err := errors.Join(iterationErr, closeErr); err != nil {
		return mapDBErrorV1(ctx, "verify_v5", err, false)
	}
	if len(indexes) != 4 {
		return schemaConflictV5("provider boundary V3 index set drifted")
	}
	exact, turnAttempt, runtimeBoundary, primary := false, false, false, false
	for _, value := range indexes {
		if value.partial != 0 {
			return schemaConflictV5("provider boundary V3 partial index is forbidden")
		}
		columns, err := indexKeyColumnsV3(ctx, db, value.name)
		if err != nil {
			return err
		}
		switch value.origin {
		case "c":
			if exact ||
				value.name != "governed_model_turn_v3_provider_boundary_history_exact" ||
				value.unique != 0 ||
				!equalStringsV3(columns, governedModelTurnProviderBoundaryExactIndexColumnsV5) {
				return schemaConflictV5("provider boundary V3 exact index drifted")
			}
			exact = true
		case "u":
			if value.unique != 1 {
				return schemaConflictV5("provider boundary V3 UNIQUE index drifted")
			}
			switch {
			case equalStringsV3(columns, []string{"turn_attempt_digest"}):
				if turnAttempt {
					return schemaConflictV5("provider boundary V3 Turn Attempt UNIQUE duplicated")
				}
				turnAttempt = true
			case equalStringsV3(columns, []string{"runtime_boundary_digest"}):
				if runtimeBoundary {
					return schemaConflictV5("provider boundary V3 Runtime boundary UNIQUE duplicated")
				}
				runtimeBoundary = true
			default:
				return schemaConflictV5("provider boundary V3 unexpected UNIQUE index")
			}
		case "pk":
			if primary || value.unique != 1 ||
				!equalStringsV3(columns, []string{"boundary_id", "revision"}) {
				return schemaConflictV5("provider boundary V3 primary key drifted")
			}
			primary = true
		default:
			return schemaConflictV5("provider boundary V3 index origin drifted")
		}
	}
	if !exact || !turnAttempt || !runtimeBoundary || !primary {
		return schemaConflictV5("provider boundary V3 index set is incomplete")
	}
	return nil
}

func schemaConflictV5(message string) error {
	return errorV1(
		modelinvoker.GovernedModelInvocationErrorConflict,
		"verify_v5",
		message,
		nil,
	)
}
