// Package sqlite provides the single-node durable Sandbox Owner State Plane.
// It persists Sandbox-owned facts and exact bindings only. Runtime, Review,
// Retention, Legal Hold, Continuity, and Provider facts remain with their
// semantic owners and are injected through public readers.
package sqlite

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	_ "modernc.org/sqlite"
)

const schemaVersion = 19

const (
	workspaceReadCommandBodySealTableDDLV1 = `CREATE TABLE IF NOT EXISTS workspace_read_command_body_seal (
		command_id TEXT NOT NULL PRIMARY KEY, revision INTEGER NOT NULL,
		digest TEXT NOT NULL, canonical_body_digest TEXT NOT NULL)`
	workspaceReadRuntimeAttemptBindingTableDDLV2 = `CREATE TABLE IF NOT EXISTS workspace_read_runtime_attempt_admission_binding_v2 (
		runtime_attempt_digest TEXT NOT NULL PRIMARY KEY,
		operation_digest TEXT NOT NULL, effect_id TEXT NOT NULL,
		intent_revision INTEGER NOT NULL, intent_digest TEXT NOT NULL,
		permit_id TEXT NOT NULL, permit_revision INTEGER NOT NULL, permit_digest TEXT NOT NULL,
		runtime_attempt_id TEXT NOT NULL,
		delegation_present INTEGER NOT NULL, delegation_id TEXT NOT NULL,
		delegation_revision INTEGER NOT NULL, delegation_digest TEXT NOT NULL,
		authorization_digest TEXT NOT NULL,
		association_id TEXT NOT NULL, association_revision INTEGER NOT NULL, association_digest TEXT NOT NULL,
		domain_command_id TEXT NOT NULL, domain_command_revision INTEGER NOT NULL, domain_command_digest TEXT NOT NULL,
		command_id TEXT NOT NULL, command_revision INTEGER NOT NULL, command_digest TEXT NOT NULL,
		admission_id TEXT NOT NULL, admission_revision INTEGER NOT NULL, admission_digest TEXT NOT NULL,
		workspace_attempt_id TEXT NOT NULL, workspace_attempt_revision INTEGER NOT NULL,
		workspace_attempt_digest TEXT NOT NULL, binding_digest TEXT NOT NULL, body BLOB NOT NULL)`
	workspaceReadRuntimeAttemptIdentityIndexDDLV2 = `CREATE UNIQUE INDEX IF NOT EXISTS workspace_read_runtime_attempt_identity_v2
		ON workspace_read_runtime_attempt_admission_binding_v2(
			operation_digest,effect_id,intent_revision,intent_digest,
			permit_id,permit_revision,permit_digest,runtime_attempt_id,
			delegation_present,delegation_id,delegation_revision,delegation_digest)`
	workspaceReadRuntimeAdmissionIdentityIndexDDLV2 = `CREATE UNIQUE INDEX IF NOT EXISTS workspace_read_runtime_admission_identity_v2
		ON workspace_read_runtime_attempt_admission_binding_v2(
			admission_id,admission_revision,admission_digest)`
	workspaceReadRuntimeWorkspaceAttemptIdentityIndexDDLV2 = `CREATE UNIQUE INDEX IF NOT EXISTS workspace_read_runtime_workspace_attempt_identity_v2
		ON workspace_read_runtime_attempt_admission_binding_v2(
			workspace_attempt_id,workspace_attempt_revision,workspace_attempt_digest)`
	workspaceReadCommandPublicationTableDDLV19 = `CREATE TABLE workspace_read_command_publication_v2 (
		publication_id TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		command_id TEXT NOT NULL, command_revision INTEGER NOT NULL, command_digest TEXT NOT NULL,
		source_id TEXT NOT NULL, source_revision INTEGER NOT NULL, source_digest TEXT NOT NULL,
		runtime_attempt_digest TEXT NOT NULL,
		semantic_digest TEXT NOT NULL, created_unix_nano INTEGER NOT NULL,
		expires_unix_nano INTEGER NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(publication_id,revision,digest),
		UNIQUE(publication_id,revision),
		UNIQUE(command_id,command_revision,command_digest),
		UNIQUE(source_id,source_revision,source_digest),
		UNIQUE(runtime_attempt_digest))`
	workspaceReadCommandOwnerCurrentHistoryTableDDLV19 = `CREATE TABLE workspace_read_command_owner_current_history_v2 (
		current_id TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		command_id TEXT NOT NULL, command_revision INTEGER NOT NULL, command_digest TEXT NOT NULL,
		publication_id TEXT NOT NULL, publication_revision INTEGER NOT NULL, publication_digest TEXT NOT NULL,
		checked_unix_nano INTEGER NOT NULL, expires_unix_nano INTEGER NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(current_id,revision,digest),
		UNIQUE(current_id,revision))`
	workspaceReadCommandOwnerCurrentPointerTableDDLV19 = `CREATE TABLE workspace_read_command_owner_current_pointer_v2 (
		command_id TEXT NOT NULL PRIMARY KEY, command_revision INTEGER NOT NULL, command_digest TEXT NOT NULL,
		current_id TEXT NOT NULL, current_revision INTEGER NOT NULL, current_digest TEXT NOT NULL,
		publication_id TEXT NOT NULL, publication_revision INTEGER NOT NULL, publication_digest TEXT NOT NULL)`
	workspaceReadCommandPublicationLedgerTableDDLV19 = `CREATE TABLE workspace_read_command_publication_schema_v19 (
		singleton INTEGER NOT NULL PRIMARY KEY CHECK(singleton=1),
		contract_version TEXT NOT NULL, namespace_digest TEXT NOT NULL)`
)

var workspaceReadCommandPublicationSchemaStatementsV19 = []string{
	workspaceReadCommandPublicationTableDDLV19,
	workspaceReadCommandOwnerCurrentHistoryTableDDLV19,
	workspaceReadCommandOwnerCurrentPointerTableDDLV19,
	workspaceReadCommandPublicationLedgerTableDDLV19,
}

type Store struct {
	db                            *sql.DB
	clock                         func() time.Time
	workspaceReadOwnerIncarnation string
}

func Open(ctx context.Context, path string) (*Store, error) {
	return OpenWithClock(ctx, path, time.Now)
}

func OpenWithClock(ctx context.Context, path string, clock func() time.Time) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("Sandbox SQLite path is required")
	}
	if clock == nil {
		return nil, errors.New("Sandbox SQLite clock is required")
	}
	db, err := sql.Open("sqlite", dataSource(path))
	if err != nil {
		return nil, fmt.Errorf("open Sandbox SQLite State Plane: %w", err)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(16)
	incarnation, err := newWorkspaceReadOwnerIncarnationV1()
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &Store{db: db, clock: clock, workspaceReadOwnerIncarnation: incarnation}
	if err := store.initialize(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func dataSource(path string) string {
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

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Stats() sql.DBStats { return s.db.Stats() }

func (s *Store) initialize(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Sandbox SQLite schema transaction: %w", err)
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("inspect Sandbox SQLite schema in migration transaction: %w", err)
	}
	if version < 0 || version > schemaVersion {
		return fmt.Errorf("Sandbox SQLite schema version %d is unsupported", version)
	}
	if version == schemaVersion {
		if err := verifyWorkspaceReadCommandBodySealSchemaV1(ctx, tx); err != nil {
			return fmt.Errorf("%w: v19 workspace read Command body-seal schema drifted: %v", ports.ErrConflict, err)
		}
		if err := verifyWorkspaceReadRuntimeAttemptBindingSchemaV2(ctx, tx); err != nil {
			return fmt.Errorf("%w: v19 workspace read Runtime-attempt schema drifted: %v", ports.ErrConflict, err)
		}
		if err := verifyWorkspaceReadCommandPublicationSchemaV19(ctx, tx); err != nil {
			return fmt.Errorf("%w: v19 workspace read Command publication schema drifted: %v", ports.ErrConflict, err)
		}
		return tx.Commit()
	}
	if err := preflightWorkspaceReadRuntimeAttemptBindingMigrationV2(ctx, tx, version); err != nil {
		return err
	}
	if err := preflightWorkspaceReadCommandPublicationMigrationV19(ctx, tx); err != nil {
		return err
	}
	for _, statement := range schemaStatements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create Sandbox SQLite schema: %w", err)
		}
	}
	if err := verifyWorkspaceReadCommandBodySealSchemaV1(ctx, tx); err != nil {
		return err
	}
	if err := verifyWorkspaceReadRuntimeAttemptBindingSchemaV2(ctx, tx); err != nil {
		return err
	}
	for _, statement := range workspaceReadCommandPublicationSchemaStatementsV19 {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create workspace read Command publication v19 schema: %w", err)
		}
	}
	if err := verifyWorkspaceReadCommandPublicationObjectsV19(ctx, tx); err != nil {
		return err
	}
	if err := probeWorkspaceReadCommandPublicationSchemaV19(ctx, tx); err != nil {
		return err
	}
	namespaceDigest := workspaceReadCommandPublicationSchemaDigestV19()
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO workspace_read_command_publication_schema_v19(
			singleton,contract_version,namespace_digest
		) VALUES(1,?,?)`,
		"praxis.sandbox/workspace-read-command-publication/v2",
		namespaceDigest,
	); err != nil {
		return fmt.Errorf("write workspace read Command publication v19 schema ledger: %w", err)
	}
	if err := verifyWorkspaceReadCommandPublicationSchemaV19(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", schemaVersion)); err != nil {
		return fmt.Errorf("set Sandbox SQLite schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Sandbox SQLite schema: %w", err)
	}
	return nil
}

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS sandbox_facts (
		kind TEXT NOT NULL, id TEXT NOT NULL, revision INTEGER NOT NULL,
		digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(kind,id,revision,digest))`,
	`CREATE UNIQUE INDEX IF NOT EXISTS sandbox_fact_identity
		ON sandbox_facts(kind,id)`,
	`CREATE TABLE IF NOT EXISTS reservation_attempts (
		operation_id TEXT NOT NULL, effect_id TEXT NOT NULL, attempt_id TEXT NOT NULL,
		reservation_id TEXT NOT NULL UNIQUE,
		PRIMARY KEY(operation_id,effect_id,attempt_id))`,
	`CREATE TABLE IF NOT EXISTS observation_source_current (
		source_id TEXT PRIMARY KEY, source_epoch INTEGER NOT NULL,
		source_sequence INTEGER NOT NULL, payload_digest TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS domain_result_by_reservation (
		reservation_id TEXT PRIMARY KEY, result_id TEXT NOT NULL UNIQUE)`,
	`CREATE TABLE IF NOT EXISTS environment_projection_history (
		lease_id TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		body BLOB NOT NULL, PRIMARY KEY(lease_id,revision))`,
	`CREATE TABLE IF NOT EXISTS environment_projection_current (
		lease_id TEXT PRIMARY KEY, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS settlement_bindings (
		opaque_id TEXT PRIMARY KEY, opaque_revision INTEGER NOT NULL,
		opaque_digest TEXT NOT NULL, result_id TEXT NOT NULL,
		result_revision INTEGER NOT NULL, result_digest TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS domain_result_runtime_bindings (
		binding_id TEXT PRIMARY KEY, digest TEXT NOT NULL, body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS lifecycle_plans (
		plan_id TEXT PRIMARY KEY, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		expires_unix_nano INTEGER NOT NULL, body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS lifecycle_results (
		result_id TEXT PRIMARY KEY, request_digest TEXT NOT NULL, body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS snapshot_reservation_by_stable (
		stable_digest TEXT PRIMARY KEY, reservation_key TEXT NOT NULL,
		reservation_id TEXT NOT NULL,
		reservation_revision INTEGER NOT NULL, reservation_digest TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS snapshot_current_index (
		aggregate_id TEXT PRIMARY KEY, ref_id TEXT NOT NULL, revision INTEGER NOT NULL,
		digest TEXT NOT NULL, owner_clock_watermark INTEGER NOT NULL, body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS snapshot_owner_clock (
		singleton INTEGER PRIMARY KEY CHECK(singleton=1), watermark INTEGER NOT NULL)`,
	`INSERT OR IGNORE INTO snapshot_owner_clock(singleton,watermark) VALUES(1,0)`,
	`CREATE TABLE IF NOT EXISTS workspace_restore_attempt_history (
		stable_digest TEXT NOT NULL, attempt_id TEXT NOT NULL, revision INTEGER NOT NULL,
		digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(stable_digest,revision), UNIQUE(attempt_id,revision,digest))`,
	`CREATE TABLE IF NOT EXISTS workspace_restore_attempt_current (
		stable_digest TEXT PRIMARY KEY, attempt_id TEXT NOT NULL, revision INTEGER NOT NULL,
		digest TEXT NOT NULL, body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS workspace_restore_stage_facts (
		fact_id TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(fact_id,revision,digest), UNIQUE(fact_id))`,
	`CREATE TABLE IF NOT EXISTS sandbox_api_operation_history (
		cursor INTEGER PRIMARY KEY AUTOINCREMENT,
		operation_id TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL, body BLOB NOT NULL,
		UNIQUE(operation_id,revision,digest), UNIQUE(operation_id,revision))`,
	`CREATE TABLE IF NOT EXISTS sandbox_api_operation_current (
		operation_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, idempotency_key TEXT NOT NULL,
		revision INTEGER NOT NULL, digest TEXT NOT NULL, body BLOB NOT NULL,
		UNIQUE(tenant_id,idempotency_key))`,
	`CREATE TABLE IF NOT EXISTS checkpoint_participant_history (
		participant_id TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		body BLOB NOT NULL, PRIMARY KEY(participant_id,revision,digest),
		UNIQUE(participant_id,revision))`,
	`CREATE TABLE IF NOT EXISTS checkpoint_participant_current (
		participant_id TEXT PRIMARY KEY, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS checkpoint_phase_reservation_history (
		reservation_id TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		phase_key TEXT NOT NULL UNIQUE, branch_key TEXT UNIQUE, body BLOB NOT NULL,
		PRIMARY KEY(reservation_id,revision,digest), UNIQUE(reservation_id))`,
	`CREATE TABLE IF NOT EXISTS checkpoint_phase_fact_history (
		fact_id TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		reservation_id TEXT NOT NULL, reservation_revision INTEGER NOT NULL,
		reservation_digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(fact_id,revision,digest), UNIQUE(fact_id,revision))`,
	`CREATE TABLE IF NOT EXISTS checkpoint_phase_fact_current (
		fact_id TEXT PRIMARY KEY, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		reservation_id TEXT NOT NULL, reservation_revision INTEGER NOT NULL,
		reservation_digest TEXT NOT NULL, body BLOB NOT NULL,
		UNIQUE(reservation_id,reservation_revision,reservation_digest))`,
	`CREATE TABLE IF NOT EXISTS checkpoint_phase_domain_result_history (
		result_id TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		reservation_id TEXT NOT NULL, reservation_revision INTEGER NOT NULL,
		reservation_digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(result_id,revision,digest), UNIQUE(result_id,revision),
		UNIQUE(reservation_id,reservation_revision,reservation_digest))`,
	`CREATE TABLE IF NOT EXISTS checkpoint_provider_result_bindings (
		reservation_id TEXT NOT NULL, reservation_revision INTEGER NOT NULL,
		reservation_digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(reservation_id,reservation_revision,reservation_digest),
		UNIQUE(reservation_id))`,
	`CREATE TABLE IF NOT EXISTS checkpoint_phase_execution_plans (
		tenant_id TEXT NOT NULL, attempt_id TEXT NOT NULL, participant_id TEXT NOT NULL,
		phase TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		expires_unix_nano INTEGER NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(tenant_id,attempt_id,participant_id,phase))`,
	`CREATE TABLE IF NOT EXISTS checkpoint_snapshot_capture_bindings (
		snapshot_reservation_id TEXT NOT NULL, snapshot_reservation_revision INTEGER NOT NULL,
		snapshot_reservation_digest TEXT NOT NULL, checkpoint_reservation_id TEXT NOT NULL UNIQUE,
		expires_unix_nano INTEGER NOT NULL, digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(snapshot_reservation_id,snapshot_reservation_revision,snapshot_reservation_digest))`,
	`CREATE TABLE IF NOT EXISTS workspace_checkpoint_prepared (
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, checkpoint_attempt_id TEXT NOT NULL,
		participant_id TEXT NOT NULL, participant_fact_id TEXT NOT NULL, participant_revision INTEGER NOT NULL,
		participant_digest TEXT NOT NULL, coverage_fact_id TEXT NOT NULL, coverage_revision INTEGER NOT NULL,
		coverage_digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(tenant_id,scope_digest,checkpoint_attempt_id,participant_id),
		UNIQUE(tenant_id,scope_digest,participant_fact_id),
		UNIQUE(tenant_id,scope_digest,coverage_fact_id))`,
	`CREATE TABLE IF NOT EXISTS workspace_restore_prepared_runtime_bindings (
		tenant_id TEXT NOT NULL, attempt_id TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(tenant_id,attempt_id))`,
	`CREATE TABLE IF NOT EXISTS workspace_restore_stage_runtime_bindings (
		tenant_id TEXT NOT NULL, fact_id TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(tenant_id,fact_id))`,
	`CREATE TABLE IF NOT EXISTS workspace_restore_apply_settlement_facts (
		tenant_id TEXT NOT NULL, fact_id TEXT NOT NULL, stage_id TEXT NOT NULL,
		stage_revision INTEGER NOT NULL, stage_digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(tenant_id,fact_id),
		UNIQUE(tenant_id,stage_id,stage_revision,stage_digest))`,
	`CREATE TABLE IF NOT EXISTS workspace_restore_stage_coordinates (
		stable_digest TEXT PRIMARY KEY, tenant_id TEXT NOT NULL,
		request_body BLOB NOT NULL, coordinate_body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS workspace_view_history (
		view_id TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(view_id,revision,digest), UNIQUE(view_id,revision))`,
	`CREATE TABLE IF NOT EXISTS workspace_view_current (
		view_id TEXT PRIMARY KEY, revision INTEGER NOT NULL, digest TEXT NOT NULL, body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS workspace_change_set_history (
		change_set_id TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(change_set_id,revision,digest), UNIQUE(change_set_id,revision))`,
	`CREATE TABLE IF NOT EXISTS workspace_change_set_current (
		change_set_id TEXT PRIMARY KEY, revision INTEGER NOT NULL, digest TEXT NOT NULL, body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS workspace_read_command_current (
		command_id TEXT PRIMARY KEY, revision INTEGER NOT NULL, digest TEXT NOT NULL, body BLOB NOT NULL)`,
	workspaceReadCommandBodySealTableDDLV1,
	`CREATE TABLE IF NOT EXISTS workspace_read_reservation (
		stable_digest TEXT PRIMARY KEY, reservation_id TEXT NOT NULL UNIQUE, body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS workspace_read_attempt_current (
		stable_digest TEXT PRIMARY KEY, attempt_id TEXT NOT NULL UNIQUE, revision INTEGER NOT NULL, digest TEXT NOT NULL, body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS workspace_read_attempt_origin (
		attempt_id TEXT PRIMARY KEY, stable_digest TEXT NOT NULL UNIQUE, revision INTEGER NOT NULL, digest TEXT NOT NULL, body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS workspace_read_admission_attempt_binding (
		admission_id TEXT NOT NULL, admission_revision INTEGER NOT NULL, admission_digest TEXT NOT NULL,
		attempt_id TEXT NOT NULL, attempt_revision INTEGER NOT NULL, attempt_digest TEXT NOT NULL,
		body BLOB NOT NULL,
		PRIMARY KEY(admission_id,admission_revision,admission_digest),
		UNIQUE(attempt_id,attempt_revision,attempt_digest))`,
	workspaceReadRuntimeAttemptBindingTableDDLV2,
	workspaceReadRuntimeAttemptIdentityIndexDDLV2,
	workspaceReadRuntimeAdmissionIdentityIndexDDLV2,
	workspaceReadRuntimeWorkspaceAttemptIdentityIndexDDLV2,
	`CREATE TABLE IF NOT EXISTS workspace_read_attempt_owner_incarnation (
		attempt_id TEXT PRIMARY KEY, owner_incarnation_id TEXT NOT NULL, reserved_unix_nano INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS workspace_read_recovery_evidence (
		attempt_id TEXT PRIMARY KEY, previous_owner_incarnation_id TEXT NOT NULL,
		current_owner_incarnation_id TEXT NOT NULL, recovered_unix_nano INTEGER NOT NULL,
		evidence_digest TEXT NOT NULL UNIQUE)`,
	`CREATE TABLE IF NOT EXISTS workspace_read_observation (
		observation_id TEXT PRIMARY KEY, stable_digest TEXT NOT NULL UNIQUE, body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS workspace_rewind_composition_facts (
		fact_id TEXT PRIMARY KEY, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, request_id TEXT NOT NULL,
		idempotency_key TEXT NOT NULL, request_digest TEXT NOT NULL,
		planned_change_set_id TEXT NOT NULL, planned_change_set_revision INTEGER NOT NULL,
		planned_change_set_digest TEXT NOT NULL, body BLOB NOT NULL,
		UNIQUE(tenant_id,scope_digest,request_id),
		UNIQUE(tenant_id,scope_digest,idempotency_key),
		UNIQUE(planned_change_set_id,planned_change_set_revision,planned_change_set_digest))`,
}

func verifyWorkspaceReadCommandBodySealSchemaV1(ctx context.Context, tx *sql.Tx) error {
	if ctx == nil || tx == nil {
		return errors.New("verify workspace read Command body seal schema requires context and transaction")
	}
	namespaceRows, err := tx.QueryContext(
		ctx,
		`SELECT type,name,tbl_name FROM sqlite_master
		  WHERE name='workspace_read_command_body_seal'
		     OR name LIKE 'workspace_read_command_body_seal_%'
		     OR tbl_name='workspace_read_command_body_seal'
		  ORDER BY type,name`,
	)
	if err != nil {
		return fmt.Errorf("inspect workspace read Command body seal namespace: %w", err)
	}
	expectedNamespace := map[string]struct{}{
		"table:workspace_read_command_body_seal:workspace_read_command_body_seal":                    {},
		"index:sqlite_autoindex_workspace_read_command_body_seal_1:workspace_read_command_body_seal": {},
	}
	foundNamespace := make(map[string]struct{}, len(expectedNamespace))
	for namespaceRows.Next() {
		var kind, name, table string
		if err = namespaceRows.Scan(&kind, &name, &table); err != nil {
			_ = namespaceRows.Close()
			return fmt.Errorf("decode workspace read Command body seal namespace: %w", err)
		}
		object := kind + ":" + name + ":" + table
		if _, ok := expectedNamespace[object]; !ok {
			_ = namespaceRows.Close()
			return fmt.Errorf("workspace read Command body seal namespace contains unexpected object %q", object)
		}
		foundNamespace[object] = struct{}{}
	}
	if err = namespaceRows.Err(); err != nil {
		_ = namespaceRows.Close()
		return fmt.Errorf("inspect workspace read Command body seal namespace rows: %w", err)
	}
	if err = namespaceRows.Close(); err != nil {
		return fmt.Errorf("close workspace read Command body seal namespace rows: %w", err)
	}
	if len(foundNamespace) != len(expectedNamespace) {
		return errors.New("workspace read Command body seal namespace is incomplete")
	}
	var actualDDL sql.NullString
	if err = tx.QueryRowContext(
		ctx,
		`SELECT sql FROM sqlite_master
		  WHERE type='table' AND name='workspace_read_command_body_seal'`,
	).Scan(&actualDDL); err != nil {
		return fmt.Errorf("inspect workspace read Command body seal DDL: %w", err)
	}
	if !actualDDL.Valid {
		return errors.New("workspace read Command body seal DDL is missing")
	}
	actualTokens, err := canonicalSQLiteDDLTokensV2(actualDDL.String)
	if err != nil {
		return err
	}
	expectedTokens, err := canonicalSQLiteDDLTokensV2(workspaceReadCommandBodySealTableDDLV1)
	if err != nil {
		return err
	}
	if !equalSQLiteDDLTokensV2(actualTokens, expectedTokens) {
		return errors.New("workspace read Command body seal DDL semantics drifted")
	}
	rows, err := tx.QueryContext(ctx, `PRAGMA table_xinfo(workspace_read_command_body_seal)`)
	if err != nil {
		return fmt.Errorf("inspect workspace read Command body seal schema: %w", err)
	}
	defer rows.Close()
	expected := []struct {
		name       string
		kind       string
		notNull    int
		primaryKey int
	}{
		{name: "command_id", kind: "TEXT", notNull: 1, primaryKey: 1},
		{name: "revision", kind: "INTEGER", notNull: 1},
		{name: "digest", kind: "TEXT", notNull: 1},
		{name: "canonical_body_digest", kind: "TEXT", notNull: 1},
	}
	index := 0
	for rows.Next() {
		var (
			columnID    int
			name        string
			kind        string
			notNull     int
			defaultBody sql.NullString
			primaryKey  int
			hidden      int
		)
		if err := rows.Scan(
			&columnID,
			&name,
			&kind,
			&notNull,
			&defaultBody,
			&primaryKey,
			&hidden,
		); err != nil {
			return fmt.Errorf("decode workspace read Command body seal schema: %w", err)
		}
		if index >= len(expected) || columnID != index ||
			name != expected[index].name ||
			kind != expected[index].kind ||
			notNull != expected[index].notNull ||
			primaryKey != expected[index].primaryKey ||
			hidden != 0 ||
			defaultBody.Valid {
			return errors.New("workspace read Command body seal schema is incompatible")
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect workspace read Command body seal schema rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close workspace read Command body seal schema rows: %w", err)
	}
	if index != len(expected) {
		return errors.New("workspace read Command body seal schema is incomplete")
	}
	indexRows, err := tx.QueryContext(ctx, `PRAGMA index_list(workspace_read_command_body_seal)`)
	if err != nil {
		return fmt.Errorf("inspect workspace read Command body seal indexes: %w", err)
	}
	indexCount := 0
	for indexRows.Next() {
		var sequence, unique, partial int
		var name, origin string
		if err = indexRows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			_ = indexRows.Close()
			return fmt.Errorf("decode workspace read Command body seal index: %w", err)
		}
		if indexCount != 0 || sequence != 0 ||
			name != "sqlite_autoindex_workspace_read_command_body_seal_1" ||
			unique != 1 || origin != "pk" || partial != 0 {
			_ = indexRows.Close()
			return errors.New("workspace read Command body seal index list drifted")
		}
		indexCount++
	}
	if err = indexRows.Err(); err != nil {
		_ = indexRows.Close()
		return fmt.Errorf("inspect workspace read Command body seal index rows: %w", err)
	}
	if err = indexRows.Close(); err != nil {
		return fmt.Errorf("close workspace read Command body seal index rows: %w", err)
	}
	if indexCount != 1 {
		return errors.New("workspace read Command body seal index list is incomplete")
	}
	indexInfoRows, err := tx.QueryContext(
		ctx,
		`PRAGMA index_xinfo(sqlite_autoindex_workspace_read_command_body_seal_1)`,
	)
	if err != nil {
		return fmt.Errorf("inspect workspace read Command body seal index columns: %w", err)
	}
	indexPosition := 0
	for indexInfoRows.Next() {
		var sequence, columnID, descending, key int
		var column, collation sql.NullString
		if err = indexInfoRows.Scan(
			&sequence, &columnID, &column, &descending, &collation, &key,
		); err != nil {
			_ = indexInfoRows.Close()
			return fmt.Errorf("decode workspace read Command body seal index column: %w", err)
		}
		if sequence != indexPosition || descending != 0 ||
			!collation.Valid || collation.String != "BINARY" {
			_ = indexInfoRows.Close()
			return errors.New("workspace read Command body seal index metadata drifted")
		}
		switch indexPosition {
		case 0:
			if key != 1 || columnID != 0 || !column.Valid || column.String != "command_id" {
				_ = indexInfoRows.Close()
				return errors.New("workspace read Command body seal index key drifted")
			}
		case 1:
			if key != 0 || columnID != -1 || column.Valid {
				_ = indexInfoRows.Close()
				return errors.New("workspace read Command body seal index auxiliary column drifted")
			}
		default:
			_ = indexInfoRows.Close()
			return errors.New("workspace read Command body seal index contains extra columns")
		}
		indexPosition++
	}
	if err = indexInfoRows.Err(); err != nil {
		_ = indexInfoRows.Close()
		return fmt.Errorf("inspect workspace read Command body seal index column rows: %w", err)
	}
	if err = indexInfoRows.Close(); err != nil {
		return fmt.Errorf("close workspace read Command body seal index column rows: %w", err)
	}
	if indexPosition != 2 {
		return errors.New("workspace read Command body seal index columns are incomplete")
	}
	return nil
}

func preflightWorkspaceReadRuntimeAttemptBindingMigrationV2(ctx context.Context, tx *sql.Tx, version int) error {
	if version >= 18 {
		if err := verifyWorkspaceReadRuntimeAttemptBindingSchemaV2(ctx, tx); err != nil {
			return fmt.Errorf("%w: v18 workspace read Runtime-attempt schema drifted: %v", ports.ErrConflict, err)
		}
		return nil
	}
	rows, err := tx.QueryContext(
		ctx,
		`SELECT type,name FROM sqlite_master
		  WHERE name LIKE 'workspace_read_runtime_%'
		  ORDER BY type,name`,
	)
	if err != nil {
		return fmt.Errorf("inspect workspace read Runtime-attempt migration namespace: %w", err)
	}
	defer rows.Close()
	var objects []string
	for rows.Next() {
		var kind, name string
		if err := rows.Scan(&kind, &name); err != nil {
			return fmt.Errorf("decode workspace read Runtime-attempt migration namespace: %w", err)
		}
		objects = append(objects, kind+":"+name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect workspace read Runtime-attempt migration namespace rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close workspace read Runtime-attempt migration namespace rows: %w", err)
	}
	if len(objects) != 0 {
		return fmt.Errorf("%w: workspace read Runtime-attempt migration namespace is partial without a v18 ledger: %v", ports.ErrConflict, objects)
	}
	return nil
}

func preflightWorkspaceReadCommandPublicationMigrationV19(ctx context.Context, tx *sql.Tx) error {
	objects, err := workspaceReadCommandPublicationNamespaceV19(ctx, tx)
	if err != nil {
		return err
	}
	if len(objects) != 0 {
		return fmt.Errorf(
			"%w: workspace read Command publication v19 namespace is partial without its ledger: %v",
			ports.ErrConflict,
			objects,
		)
	}
	return nil
}

func workspaceReadCommandPublicationNamespaceV19(
	ctx context.Context,
	tx *sql.Tx,
) ([]string, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT type,name,tbl_name FROM sqlite_master
		  WHERE name LIKE 'workspace_read_command_publication_%'
		     OR name LIKE 'workspace_read_command_owner_current_%'
		     OR tbl_name LIKE 'workspace_read_command_publication_%'
		     OR tbl_name LIKE 'workspace_read_command_owner_current_%'
		  ORDER BY type,name`,
	)
	if err != nil {
		return nil, fmt.Errorf("inspect workspace read Command publication namespace: %w", err)
	}
	defer rows.Close()
	var objects []string
	for rows.Next() {
		var kind, name, table string
		if err := rows.Scan(&kind, &name, &table); err != nil {
			return nil, fmt.Errorf("decode workspace read Command publication namespace: %w", err)
		}
		objects = append(objects, kind+":"+name+":"+table)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inspect workspace read Command publication namespace rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close workspace read Command publication namespace rows: %w", err)
	}
	return objects, nil
}

type workspaceReadSchemaColumnV19 struct {
	name       string
	kind       string
	notNull    int
	primaryKey int
}

func verifyWorkspaceReadCommandPublicationSchemaV19(ctx context.Context, tx *sql.Tx) error {
	if err := verifyWorkspaceReadCommandPublicationObjectsV19(ctx, tx); err != nil {
		return err
	}
	var contractVersion, namespaceDigest string
	if err := tx.QueryRowContext(
		ctx,
		`SELECT contract_version,namespace_digest
		   FROM workspace_read_command_publication_schema_v19
		  WHERE singleton=1`,
	).Scan(&contractVersion, &namespaceDigest); err != nil {
		return fmt.Errorf("inspect workspace read Command publication schema ledger: %w", err)
	}
	if contractVersion != "praxis.sandbox/workspace-read-command-publication/v2" ||
		namespaceDigest != workspaceReadCommandPublicationSchemaDigestV19() {
		return errors.New("workspace read Command publication schema ledger drifted")
	}
	var rows int
	if err := tx.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM workspace_read_command_publication_schema_v19`,
	).Scan(&rows); err != nil {
		return fmt.Errorf("count workspace read Command publication schema ledger: %w", err)
	}
	if rows != 1 {
		return errors.New("workspace read Command publication schema ledger is not singleton")
	}
	return nil
}

func verifyWorkspaceReadCommandPublicationObjectsV19(ctx context.Context, tx *sql.Tx) error {
	expectedNamespace := map[string]struct{}{
		"table:workspace_read_command_publication_schema_v19:workspace_read_command_publication_schema_v19":                        {},
		"table:workspace_read_command_publication_v2:workspace_read_command_publication_v2":                                        {},
		"table:workspace_read_command_owner_current_history_v2:workspace_read_command_owner_current_history_v2":                    {},
		"table:workspace_read_command_owner_current_pointer_v2:workspace_read_command_owner_current_pointer_v2":                    {},
		"index:sqlite_autoindex_workspace_read_command_publication_v2_1:workspace_read_command_publication_v2":                     {},
		"index:sqlite_autoindex_workspace_read_command_publication_v2_2:workspace_read_command_publication_v2":                     {},
		"index:sqlite_autoindex_workspace_read_command_publication_v2_3:workspace_read_command_publication_v2":                     {},
		"index:sqlite_autoindex_workspace_read_command_publication_v2_4:workspace_read_command_publication_v2":                     {},
		"index:sqlite_autoindex_workspace_read_command_publication_v2_5:workspace_read_command_publication_v2":                     {},
		"index:sqlite_autoindex_workspace_read_command_owner_current_history_v2_1:workspace_read_command_owner_current_history_v2": {},
		"index:sqlite_autoindex_workspace_read_command_owner_current_history_v2_2:workspace_read_command_owner_current_history_v2": {},
		"index:sqlite_autoindex_workspace_read_command_owner_current_pointer_v2_1:workspace_read_command_owner_current_pointer_v2": {},
	}
	objects, err := workspaceReadCommandPublicationNamespaceV19(ctx, tx)
	if err != nil {
		return err
	}
	found := make(map[string]struct{}, len(objects))
	for _, object := range objects {
		if _, ok := expectedNamespace[object]; !ok {
			return fmt.Errorf("workspace read Command publication namespace contains unexpected object %q", object)
		}
		found[object] = struct{}{}
	}
	if len(found) != len(expectedNamespace) {
		return errors.New("workspace read Command publication namespace is incomplete")
	}

	if err := verifyWorkspaceReadSchemaColumnsV19(ctx, tx, "workspace_read_command_publication_v2", []workspaceReadSchemaColumnV19{
		{name: "publication_id", kind: "TEXT", notNull: 1, primaryKey: 1},
		{name: "revision", kind: "INTEGER", notNull: 1, primaryKey: 2},
		{name: "digest", kind: "TEXT", notNull: 1, primaryKey: 3},
		{name: "command_id", kind: "TEXT", notNull: 1},
		{name: "command_revision", kind: "INTEGER", notNull: 1},
		{name: "command_digest", kind: "TEXT", notNull: 1},
		{name: "source_id", kind: "TEXT", notNull: 1},
		{name: "source_revision", kind: "INTEGER", notNull: 1},
		{name: "source_digest", kind: "TEXT", notNull: 1},
		{name: "runtime_attempt_digest", kind: "TEXT", notNull: 1},
		{name: "semantic_digest", kind: "TEXT", notNull: 1},
		{name: "created_unix_nano", kind: "INTEGER", notNull: 1},
		{name: "expires_unix_nano", kind: "INTEGER", notNull: 1},
		{name: "body", kind: "BLOB", notNull: 1},
	}); err != nil {
		return err
	}
	if err := verifyWorkspaceReadSchemaColumnsV19(ctx, tx, "workspace_read_command_owner_current_history_v2", []workspaceReadSchemaColumnV19{
		{name: "current_id", kind: "TEXT", notNull: 1, primaryKey: 1},
		{name: "revision", kind: "INTEGER", notNull: 1, primaryKey: 2},
		{name: "digest", kind: "TEXT", notNull: 1, primaryKey: 3},
		{name: "command_id", kind: "TEXT", notNull: 1},
		{name: "command_revision", kind: "INTEGER", notNull: 1},
		{name: "command_digest", kind: "TEXT", notNull: 1},
		{name: "publication_id", kind: "TEXT", notNull: 1},
		{name: "publication_revision", kind: "INTEGER", notNull: 1},
		{name: "publication_digest", kind: "TEXT", notNull: 1},
		{name: "checked_unix_nano", kind: "INTEGER", notNull: 1},
		{name: "expires_unix_nano", kind: "INTEGER", notNull: 1},
		{name: "body", kind: "BLOB", notNull: 1},
	}); err != nil {
		return err
	}
	if err := verifyWorkspaceReadSchemaColumnsV19(ctx, tx, "workspace_read_command_owner_current_pointer_v2", []workspaceReadSchemaColumnV19{
		{name: "command_id", kind: "TEXT", notNull: 1, primaryKey: 1},
		{name: "command_revision", kind: "INTEGER", notNull: 1},
		{name: "command_digest", kind: "TEXT", notNull: 1},
		{name: "current_id", kind: "TEXT", notNull: 1},
		{name: "current_revision", kind: "INTEGER", notNull: 1},
		{name: "current_digest", kind: "TEXT", notNull: 1},
		{name: "publication_id", kind: "TEXT", notNull: 1},
		{name: "publication_revision", kind: "INTEGER", notNull: 1},
		{name: "publication_digest", kind: "TEXT", notNull: 1},
	}); err != nil {
		return err
	}
	if err := verifyWorkspaceReadSchemaColumnsV19(ctx, tx, "workspace_read_command_publication_schema_v19", []workspaceReadSchemaColumnV19{
		{name: "singleton", kind: "INTEGER", notNull: 1, primaryKey: 1},
		{name: "contract_version", kind: "TEXT", notNull: 1},
		{name: "namespace_digest", kind: "TEXT", notNull: 1},
	}); err != nil {
		return err
	}
	if err := verifyWorkspaceReadCommandPublicationIndexesV19(ctx, tx); err != nil {
		return err
	}
	return verifyWorkspaceReadCommandPublicationDDLV19(ctx, tx)
}

func verifyWorkspaceReadSchemaColumnsV19(
	ctx context.Context,
	tx *sql.Tx,
	table string,
	expected []workspaceReadSchemaColumnV19,
) error {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_xinfo(%s)", table))
	if err != nil {
		return fmt.Errorf("inspect %s columns: %w", table, err)
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		var (
			columnID    int
			name        string
			kind        string
			notNull     int
			defaultBody sql.NullString
			primaryKey  int
			hidden      int
		)
		if err := rows.Scan(
			&columnID, &name, &kind, &notNull, &defaultBody, &primaryKey, &hidden,
		); err != nil {
			return fmt.Errorf("decode %s columns: %w", table, err)
		}
		if index >= len(expected) ||
			columnID != index ||
			name != expected[index].name ||
			kind != expected[index].kind ||
			notNull != expected[index].notNull ||
			primaryKey != expected[index].primaryKey ||
			hidden != 0 ||
			defaultBody.Valid {
			return fmt.Errorf("%s columns drifted", table)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect %s column rows: %w", table, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close %s column rows: %w", table, err)
	}
	if index != len(expected) {
		return fmt.Errorf("%s columns are incomplete", table)
	}
	return nil
}

func verifyWorkspaceReadCommandPublicationDDLV19(ctx context.Context, tx *sql.Tx) error {
	expected := map[string]string{
		"workspace_read_command_publication_v2":           workspaceReadCommandPublicationTableDDLV19,
		"workspace_read_command_owner_current_history_v2": workspaceReadCommandOwnerCurrentHistoryTableDDLV19,
		"workspace_read_command_owner_current_pointer_v2": workspaceReadCommandOwnerCurrentPointerTableDDLV19,
		"workspace_read_command_publication_schema_v19":   workspaceReadCommandPublicationLedgerTableDDLV19,
	}
	for name, expectedDDL := range expected {
		var actual sql.NullString
		if err := tx.QueryRowContext(
			ctx,
			`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`,
			name,
		).Scan(&actual); err != nil {
			return fmt.Errorf("inspect %s DDL: %w", name, err)
		}
		if !actual.Valid {
			return fmt.Errorf("%s DDL is missing", name)
		}
		actualTokens, err := canonicalSQLiteDDLTokensV2(actual.String)
		if err != nil {
			return err
		}
		expectedTokens, err := canonicalSQLiteDDLTokensV2(expectedDDL)
		if err != nil {
			return err
		}
		if !equalSQLiteDDLTokensV2(actualTokens, expectedTokens) {
			return fmt.Errorf("%s DDL semantics drifted", name)
		}
	}
	return nil
}

type workspaceReadIndexExpectationV19 struct {
	sequence int
	origin   string
	columns  []string
	cids     []int
}

func verifyWorkspaceReadCommandPublicationIndexesV19(ctx context.Context, tx *sql.Tx) error {
	tables := map[string]map[string]workspaceReadIndexExpectationV19{
		"workspace_read_command_publication_v2": {
			"sqlite_autoindex_workspace_read_command_publication_v2_5": {
				sequence: 0, origin: "u",
				columns: []string{"runtime_attempt_digest"}, cids: []int{9},
			},
			"sqlite_autoindex_workspace_read_command_publication_v2_4": {
				sequence: 1, origin: "u",
				columns: []string{"source_id", "source_revision", "source_digest"},
				cids:    []int{6, 7, 8},
			},
			"sqlite_autoindex_workspace_read_command_publication_v2_3": {
				sequence: 2, origin: "u",
				columns: []string{"command_id", "command_revision", "command_digest"},
				cids:    []int{3, 4, 5},
			},
			"sqlite_autoindex_workspace_read_command_publication_v2_2": {
				sequence: 3, origin: "u",
				columns: []string{"publication_id", "revision"}, cids: []int{0, 1},
			},
			"sqlite_autoindex_workspace_read_command_publication_v2_1": {
				sequence: 4, origin: "pk",
				columns: []string{"publication_id", "revision", "digest"},
				cids:    []int{0, 1, 2},
			},
		},
		"workspace_read_command_owner_current_history_v2": {
			"sqlite_autoindex_workspace_read_command_owner_current_history_v2_2": {
				sequence: 0, origin: "u",
				columns: []string{"current_id", "revision"}, cids: []int{0, 1},
			},
			"sqlite_autoindex_workspace_read_command_owner_current_history_v2_1": {
				sequence: 1, origin: "pk",
				columns: []string{"current_id", "revision", "digest"},
				cids:    []int{0, 1, 2},
			},
		},
		"workspace_read_command_owner_current_pointer_v2": {
			"sqlite_autoindex_workspace_read_command_owner_current_pointer_v2_1": {
				sequence: 0, origin: "pk",
				columns: []string{"command_id"}, cids: []int{0},
			},
		},
		"workspace_read_command_publication_schema_v19": {},
	}
	for table, expected := range tables {
		if err := verifyWorkspaceReadIndexSetV19(ctx, tx, table, expected); err != nil {
			return err
		}
	}
	return nil
}

func verifyWorkspaceReadIndexSetV19(
	ctx context.Context,
	tx *sql.Tx,
	table string,
	expected map[string]workspaceReadIndexExpectationV19,
) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA index_list(`+table+`)`)
	if err != nil {
		return fmt.Errorf("inspect %s indexes: %w", table, err)
	}
	defer rows.Close()
	found := make(map[string]bool, len(expected))
	position := 0
	for rows.Next() {
		var sequence, unique, partial int
		var name, origin string
		if err = rows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			_ = rows.Close()
			return fmt.Errorf("decode %s index: %w", table, err)
		}
		expectation, ok := expected[name]
		if !ok ||
			sequence != position ||
			sequence != expectation.sequence ||
			unique != 1 ||
			origin != expectation.origin ||
			partial != 0 ||
			found[name] {
			_ = rows.Close()
			return fmt.Errorf("%s index list drifted", table)
		}
		found[name] = true
		position++
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("inspect %s index rows: %w", table, err)
	}
	if err = rows.Close(); err != nil {
		return fmt.Errorf("close %s index rows: %w", table, err)
	}
	if position != len(expected) {
		return fmt.Errorf("%s index list is incomplete", table)
	}
	for name, expectation := range expected {
		if !found[name] {
			return fmt.Errorf("%s expected index %s is missing", table, name)
		}
		indexRows, queryErr := tx.QueryContext(ctx, `PRAGMA index_xinfo(`+name+`)`)
		if queryErr != nil {
			return fmt.Errorf("inspect %s index columns: %w", name, queryErr)
		}
		indexPosition := 0
		for indexRows.Next() {
			var sequence, columnID, descending, key int
			var column, collation sql.NullString
			if err = indexRows.Scan(
				&sequence,
				&columnID,
				&column,
				&descending,
				&collation,
				&key,
			); err != nil {
				_ = indexRows.Close()
				return fmt.Errorf("decode %s index column: %w", name, err)
			}
			if sequence != indexPosition ||
				descending != 0 ||
				!collation.Valid ||
				collation.String != "BINARY" {
				_ = indexRows.Close()
				return fmt.Errorf("%s index column metadata drifted", name)
			}
			if indexPosition < len(expectation.columns) {
				if key != 1 ||
					columnID != expectation.cids[indexPosition] ||
					!column.Valid ||
					column.String != expectation.columns[indexPosition] {
					_ = indexRows.Close()
					return fmt.Errorf("%s key columns drifted", name)
				}
			} else if indexPosition == len(expectation.columns) {
				if key != 0 || columnID != -1 || column.Valid {
					_ = indexRows.Close()
					return fmt.Errorf("%s auxiliary column drifted", name)
				}
			} else {
				_ = indexRows.Close()
				return fmt.Errorf("%s contains extra columns", name)
			}
			indexPosition++
		}
		if err = indexRows.Err(); err != nil {
			_ = indexRows.Close()
			return fmt.Errorf("inspect %s index column rows: %w", name, err)
		}
		if err = indexRows.Close(); err != nil {
			return fmt.Errorf("close %s index column rows: %w", name, err)
		}
		if indexPosition != len(expectation.columns)+1 {
			return fmt.Errorf("%s index columns are incomplete", name)
		}
	}
	return nil
}

func workspaceReadCommandPublicationSchemaDigestV19() string {
	sum := sha256.Sum256([]byte(strings.Join(workspaceReadCommandPublicationSchemaStatementsV19, "\n")))
	return hex.EncodeToString(sum[:])
}

func probeWorkspaceReadCommandPublicationSchemaV19(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "SAVEPOINT workspace_read_command_publication_v19_probe"); err != nil {
		return fmt.Errorf("begin workspace read Command publication v19 probe: %w", err)
	}
	rollback := func() {
		_, _ = tx.ExecContext(ctx, "ROLLBACK TO workspace_read_command_publication_v19_probe")
		_, _ = tx.ExecContext(ctx, "RELEASE workspace_read_command_publication_v19_probe")
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO workspace_read_command_publication_v2(
				publication_id,revision,digest,
				command_id,command_revision,command_digest,
				source_id,source_revision,source_digest,
				runtime_attempt_digest,semantic_digest,
				created_unix_nano,expires_unix_nano,body
			) VALUES('probe-publication',1,'probe-publication-digest',
				'probe-command',1,'probe-command-digest',
				'probe-source',1,'probe-source-digest',
				'probe-runtime-attempt-digest','probe-semantic-digest',1,2,x'00')`,
	); err != nil {
		rollback()
		return fmt.Errorf("write workspace read Command publication v19 probe: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO workspace_read_command_owner_current_history_v2(
			current_id,revision,digest,
			command_id,command_revision,command_digest,
			publication_id,publication_revision,publication_digest,
			checked_unix_nano,expires_unix_nano,body
		) VALUES('probe-current',1,'probe-current-digest',
			'probe-command',1,'probe-command-digest',
			'probe-publication',1,'probe-publication-digest',1,2,x'00')`,
	); err != nil {
		rollback()
		return fmt.Errorf("write workspace read Command Owner current v19 probe: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO workspace_read_command_owner_current_pointer_v2(
			command_id,command_revision,command_digest,
			current_id,current_revision,current_digest,
			publication_id,publication_revision,publication_digest
		) VALUES('probe-command',1,'probe-command-digest',
			'probe-current',1,'probe-current-digest',
			'probe-publication',1,'probe-publication-digest')`,
	); err != nil {
		rollback()
		return fmt.Errorf("write workspace read Command Owner pointer v19 probe: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "ROLLBACK TO workspace_read_command_publication_v19_probe"); err != nil {
		rollback()
		return fmt.Errorf("rollback workspace read Command publication v19 probe: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "RELEASE workspace_read_command_publication_v19_probe"); err != nil {
		return fmt.Errorf("release workspace read Command publication v19 probe: %w", err)
	}
	return nil
}

func verifyWorkspaceReadRuntimeAttemptBindingSchemaV2(ctx context.Context, tx *sql.Tx) error {
	if ctx == nil || tx == nil {
		return errors.New("verify workspace read Runtime-attempt binding schema requires context and transaction")
	}
	rows, err := tx.QueryContext(ctx, `PRAGMA table_xinfo(workspace_read_runtime_attempt_admission_binding_v2)`)
	if err != nil {
		return fmt.Errorf("inspect workspace read Runtime-attempt binding schema: %w", err)
	}
	defer rows.Close()
	expected := []struct {
		name       string
		kind       string
		primaryKey int
	}{
		{name: "runtime_attempt_digest", kind: "TEXT", primaryKey: 1},
		{name: "operation_digest", kind: "TEXT"},
		{name: "effect_id", kind: "TEXT"},
		{name: "intent_revision", kind: "INTEGER"},
		{name: "intent_digest", kind: "TEXT"},
		{name: "permit_id", kind: "TEXT"},
		{name: "permit_revision", kind: "INTEGER"},
		{name: "permit_digest", kind: "TEXT"},
		{name: "runtime_attempt_id", kind: "TEXT"},
		{name: "delegation_present", kind: "INTEGER"},
		{name: "delegation_id", kind: "TEXT"},
		{name: "delegation_revision", kind: "INTEGER"},
		{name: "delegation_digest", kind: "TEXT"},
		{name: "authorization_digest", kind: "TEXT"},
		{name: "association_id", kind: "TEXT"},
		{name: "association_revision", kind: "INTEGER"},
		{name: "association_digest", kind: "TEXT"},
		{name: "domain_command_id", kind: "TEXT"},
		{name: "domain_command_revision", kind: "INTEGER"},
		{name: "domain_command_digest", kind: "TEXT"},
		{name: "command_id", kind: "TEXT"},
		{name: "command_revision", kind: "INTEGER"},
		{name: "command_digest", kind: "TEXT"},
		{name: "admission_id", kind: "TEXT"},
		{name: "admission_revision", kind: "INTEGER"},
		{name: "admission_digest", kind: "TEXT"},
		{name: "workspace_attempt_id", kind: "TEXT"},
		{name: "workspace_attempt_revision", kind: "INTEGER"},
		{name: "workspace_attempt_digest", kind: "TEXT"},
		{name: "binding_digest", kind: "TEXT"},
		{name: "body", kind: "BLOB"},
	}
	index := 0
	for rows.Next() {
		var (
			columnID    int
			name        string
			kind        string
			notNull     int
			defaultBody sql.NullString
			primaryKey  int
			hidden      int
		)
		if err := rows.Scan(&columnID, &name, &kind, &notNull, &defaultBody, &primaryKey, &hidden); err != nil {
			return fmt.Errorf("decode workspace read Runtime-attempt binding schema: %w", err)
		}
		if index >= len(expected) ||
			columnID != index ||
			name != expected[index].name ||
			kind != expected[index].kind ||
			notNull != 1 ||
			primaryKey != expected[index].primaryKey ||
			hidden != 0 ||
			defaultBody.Valid {
			return errors.New("workspace read Runtime-attempt binding schema is incompatible")
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect workspace read Runtime-attempt binding schema rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close workspace read Runtime-attempt binding schema rows: %w", err)
	}
	if index != len(expected) {
		return errors.New("workspace read Runtime-attempt binding schema is incomplete")
	}
	if err := verifyWorkspaceReadRuntimeAttemptBindingNamespaceV2(ctx, tx); err != nil {
		return err
	}
	if err := verifyWorkspaceReadRuntimeAttemptBindingDDLV2(ctx, tx); err != nil {
		return err
	}
	return verifyWorkspaceReadRuntimeAttemptBindingIndexesV2(ctx, tx)
}

func verifyWorkspaceReadRuntimeAttemptBindingDDLV2(ctx context.Context, tx *sql.Tx) error {
	type ddlExpectation struct {
		kind      string
		name      string
		statement string
	}
	expected := []ddlExpectation{
		{
			kind:      "table",
			name:      "workspace_read_runtime_attempt_admission_binding_v2",
			statement: workspaceReadRuntimeAttemptBindingTableDDLV2,
		},
		{
			kind:      "index",
			name:      "workspace_read_runtime_attempt_identity_v2",
			statement: workspaceReadRuntimeAttemptIdentityIndexDDLV2,
		},
		{
			kind:      "index",
			name:      "workspace_read_runtime_admission_identity_v2",
			statement: workspaceReadRuntimeAdmissionIdentityIndexDDLV2,
		},
		{
			kind:      "index",
			name:      "workspace_read_runtime_workspace_attempt_identity_v2",
			statement: workspaceReadRuntimeWorkspaceAttemptIdentityIndexDDLV2,
		},
		{
			kind: "index",
			name: "sqlite_autoindex_workspace_read_runtime_attempt_admission_binding_v2_1",
		},
	}
	for _, expectation := range expected {
		var (
			kind      string
			name      string
			statement sql.NullString
		)
		if err := tx.QueryRowContext(
			ctx,
			`SELECT type,name,sql FROM sqlite_master WHERE type=? AND name=?`,
			expectation.kind,
			expectation.name,
		).Scan(&kind, &name, &statement); err != nil {
			return fmt.Errorf("inspect workspace read Runtime-attempt DDL %s: %w", expectation.name, err)
		}
		if kind != expectation.kind || name != expectation.name {
			return errors.New("workspace read Runtime-attempt DDL identity drifted")
		}
		if expectation.statement == "" {
			if statement.Valid {
				return errors.New("workspace read Runtime-attempt implicit index unexpectedly has explicit DDL")
			}
			continue
		}
		if !statement.Valid {
			return errors.New("workspace read Runtime-attempt DDL is missing")
		}
		actualTokens, err := canonicalSQLiteDDLTokensV2(statement.String)
		if err != nil {
			return fmt.Errorf("parse stored workspace read Runtime-attempt DDL %s: %w", expectation.name, err)
		}
		expectedTokens, err := canonicalSQLiteDDLTokensV2(expectation.statement)
		if err != nil {
			return fmt.Errorf("parse expected workspace read Runtime-attempt DDL %s: %w", expectation.name, err)
		}
		if !equalSQLiteDDLTokensV2(actualTokens, expectedTokens) {
			return errors.New("workspace read Runtime-attempt DDL semantics drifted")
		}
	}
	return nil
}

func canonicalSQLiteDDLTokensV2(statement string) ([]string, error) {
	tokens := make([]string, 0, len(statement)/4)
	for index := 0; index < len(statement); {
		switch {
		case isSQLiteDDLWhitespaceV2(statement[index]):
			index++
		case index+1 < len(statement) && statement[index:index+2] == "--":
			index += 2
			for index < len(statement) && statement[index] != '\n' && statement[index] != '\r' {
				index++
			}
		case index+1 < len(statement) && statement[index:index+2] == "/*":
			end := strings.Index(statement[index+2:], "*/")
			if end < 0 {
				return nil, errors.New("SQLite DDL contains an unterminated block comment")
			}
			index += end + 4
		case statement[index] == '\'' || statement[index] == '"' || statement[index] == '`':
			start := index
			quote := statement[index]
			index++
			closed := false
			for index < len(statement) {
				if statement[index] != quote {
					index++
					continue
				}
				if index+1 < len(statement) && statement[index+1] == quote {
					index += 2
					continue
				}
				index++
				closed = true
				break
			}
			if !closed {
				return nil, errors.New("SQLite DDL contains an unterminated quoted token")
			}
			tokens = append(tokens, "quoted:"+statement[start:index])
		case statement[index] == '[':
			start := index
			index++
			for index < len(statement) && statement[index] != ']' {
				index++
			}
			if index == len(statement) {
				return nil, errors.New("SQLite DDL contains an unterminated bracketed identifier")
			}
			index++
			tokens = append(tokens, "quoted:"+statement[start:index])
		case isSQLiteDDLWordV2(statement[index]):
			start := index
			for index < len(statement) && isSQLiteDDLWordV2(statement[index]) {
				index++
			}
			tokens = append(tokens, "word:"+strings.ToUpper(statement[start:index]))
		default:
			tokens = append(tokens, "punct:"+statement[index:index+1])
			index++
		}
	}
	if len(tokens) >= 5 &&
		tokens[0] == "word:CREATE" &&
		(tokens[1] == "word:TABLE" ||
			(len(tokens) >= 6 && tokens[1] == "word:UNIQUE" && tokens[2] == "word:INDEX")) {
		position := 2
		if tokens[1] == "word:UNIQUE" {
			position = 3
		}
		if len(tokens) >= position+3 &&
			tokens[position] == "word:IF" &&
			tokens[position+1] == "word:NOT" &&
			tokens[position+2] == "word:EXISTS" {
			tokens = append(tokens[:position], tokens[position+3:]...)
		}
	}
	return tokens, nil
}

func isSQLiteDDLWhitespaceV2(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n', '\f':
		return true
	default:
		return false
	}
}

func isSQLiteDDLWordV2(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' ||
		value == '_' ||
		value == '$'
}

func equalSQLiteDDLTokensV2(left, right []string) bool {
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

func verifyWorkspaceReadRuntimeAttemptBindingNamespaceV2(ctx context.Context, tx *sql.Tx) error {
	const table = "workspace_read_runtime_attempt_admission_binding_v2"
	expected := map[string]string{
		"table:" + table:                                             table,
		"index:sqlite_autoindex_" + table + "_1":                     table,
		"index:workspace_read_runtime_attempt_identity_v2":           table,
		"index:workspace_read_runtime_admission_identity_v2":         table,
		"index:workspace_read_runtime_workspace_attempt_identity_v2": table,
	}
	rows, err := tx.QueryContext(
		ctx,
		`SELECT type,name,tbl_name FROM sqlite_master
		  WHERE name LIKE 'workspace_read_runtime_%' OR tbl_name=?
		  ORDER BY type,name`,
		table,
	)
	if err != nil {
		return fmt.Errorf("inspect workspace read Runtime-attempt namespace: %w", err)
	}
	defer rows.Close()
	found := make(map[string]string, len(expected))
	for rows.Next() {
		var kind, name, tableName string
		if err := rows.Scan(&kind, &name, &tableName); err != nil {
			return fmt.Errorf("decode workspace read Runtime-attempt namespace: %w", err)
		}
		key := kind + ":" + name
		if expected[key] != tableName {
			return errors.New("workspace read Runtime-attempt namespace contains an unexpected object")
		}
		found[key] = tableName
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect workspace read Runtime-attempt namespace rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close workspace read Runtime-attempt namespace rows: %w", err)
	}
	if len(found) != len(expected) {
		return errors.New("workspace read Runtime-attempt namespace is incomplete")
	}
	return nil
}

func verifyWorkspaceReadRuntimeAttemptBindingIndexesV2(ctx context.Context, tx *sql.Tx) error {
	type indexExpectation struct {
		sequence int
		origin   string
		columns  []string
		cids     []int
	}
	expected := map[string]indexExpectation{
		"workspace_read_runtime_attempt_identity_v2": {
			sequence: 2,
			origin:   "c",
			columns: []string{
				"operation_digest", "effect_id", "intent_revision", "intent_digest",
				"permit_id", "permit_revision", "permit_digest", "runtime_attempt_id",
				"delegation_present", "delegation_id", "delegation_revision", "delegation_digest",
			},
			cids: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		},
		"workspace_read_runtime_admission_identity_v2": {
			sequence: 1,
			origin:   "c",
			columns:  []string{"admission_id", "admission_revision", "admission_digest"},
			cids:     []int{23, 24, 25},
		},
		"workspace_read_runtime_workspace_attempt_identity_v2": {
			sequence: 0,
			origin:   "c",
			columns:  []string{"workspace_attempt_id", "workspace_attempt_revision", "workspace_attempt_digest"},
			cids:     []int{26, 27, 28},
		},
		"sqlite_autoindex_workspace_read_runtime_attempt_admission_binding_v2_1": {
			sequence: 3,
			origin:   "pk",
			columns:  []string{"runtime_attempt_digest"},
			cids:     []int{0},
		},
	}
	rows, err := tx.QueryContext(ctx, `PRAGMA index_list(workspace_read_runtime_attempt_admission_binding_v2)`)
	if err != nil {
		return fmt.Errorf("inspect workspace read Runtime-attempt binding indexes: %w", err)
	}
	found := make(map[string]bool, len(expected))
	position := 0
	for rows.Next() {
		var sequence, unique, partial int
		var name, origin string
		if err := rows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			_ = rows.Close()
			return fmt.Errorf("decode workspace read Runtime-attempt binding index: %w", err)
		}
		expectation, ok := expected[name]
		if !ok ||
			sequence != position ||
			sequence != expectation.sequence ||
			unique != 1 ||
			origin != expectation.origin ||
			partial != 0 ||
			found[name] {
			_ = rows.Close()
			return errors.New("workspace read Runtime-attempt binding index list is incompatible")
		}
		found[name] = true
		position++
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("inspect workspace read Runtime-attempt binding index rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close workspace read Runtime-attempt binding index rows: %w", err)
	}
	if position != len(expected) {
		return errors.New("workspace read Runtime-attempt binding index list is incomplete")
	}
	for name, expectation := range expected {
		if !found[name] {
			return errors.New("workspace read Runtime-attempt binding unique index is missing")
		}
		indexRows, err := tx.QueryContext(ctx, `PRAGMA index_xinfo(`+name+`)`)
		if err != nil {
			return fmt.Errorf("inspect workspace read Runtime-attempt binding index columns: %w", err)
		}
		position := 0
		for indexRows.Next() {
			var sequence, columnID, descending, key int
			var column, collation sql.NullString
			if err := indexRows.Scan(&sequence, &columnID, &column, &descending, &collation, &key); err != nil {
				_ = indexRows.Close()
				return fmt.Errorf("decode workspace read Runtime-attempt binding index column: %w", err)
			}
			if sequence != position || descending != 0 || !collation.Valid || collation.String != "BINARY" {
				_ = indexRows.Close()
				return errors.New("workspace read Runtime-attempt binding index columns drifted")
			}
			if position < len(expectation.columns) {
				if key != 1 ||
					columnID != expectation.cids[position] ||
					!column.Valid ||
					column.String != expectation.columns[position] {
					_ = indexRows.Close()
					return errors.New("workspace read Runtime-attempt binding key columns drifted")
				}
			} else if position == len(expectation.columns) {
				if key != 0 || columnID != -1 || column.Valid {
					_ = indexRows.Close()
					return errors.New("workspace read Runtime-attempt binding auxiliary index column drifted")
				}
			} else {
				_ = indexRows.Close()
				return errors.New("workspace read Runtime-attempt binding index has extra columns")
			}
			position++
		}
		if err := indexRows.Err(); err != nil {
			_ = indexRows.Close()
			return fmt.Errorf("inspect workspace read Runtime-attempt binding index column rows: %w", err)
		}
		if err := indexRows.Close(); err != nil {
			return fmt.Errorf("close workspace read Runtime-attempt binding index column rows: %w", err)
		}
		if position != len(expectation.columns)+1 {
			return errors.New("workspace read Runtime-attempt binding index columns are incomplete")
		}
	}
	return nil
}

func encode(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode Sandbox State Plane object: %w", err)
	}
	return body, nil
}

func decode(body []byte, target any) error {
	if len(body) == 0 {
		return errors.New("stored Sandbox State Plane body is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode stored Sandbox State Plane body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("stored Sandbox State Plane body contains trailing data")
	}
	return nil
}

func insertFact(ctx context.Context, tx *sql.Tx, kind, id string, revision uint64, digest string, value any) error {
	body, err := encode(value)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sandbox_facts(kind,id,revision,digest,body) VALUES(?,?,?,?,?)`, kind, id, revision, digest, body)
	return classifyWrite(err)
}

func readFact[T any](ctx context.Context, db queryer, kind, id string) (T, error) {
	var zero T
	var body []byte
	if err := db.QueryRowContext(ctx, `SELECT body FROM sandbox_facts WHERE kind=? AND id=?`, kind, id).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return zero, ports.ErrNotFound
		}
		return zero, err
	}
	if err := decode(body, &zero); err != nil {
		return zero, err
	}
	return zero, nil
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func classifyWrite(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return ports.ErrConflict
	}
	return err
}

func newWorkspaceReadOwnerIncarnationV1() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("create workspace read Owner incarnation: %w", err)
	}
	return "workspace-read-owner-" + hex.EncodeToString(bytes[:]), nil
}
