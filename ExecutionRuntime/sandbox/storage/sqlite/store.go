// Package sqlite provides the single-node durable Sandbox Owner State Plane.
// It persists Sandbox-owned facts and exact bindings only. Runtime, Review,
// Retention, Legal Hold, Continuity, and Provider facts remain with their
// semantic owners and are injected through public readers.
package sqlite

import (
	"bytes"
	"context"
	"database/sql"
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

const schemaVersion = 13

type Store struct {
	db    *sql.DB
	clock func() time.Time
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
	store := &Store{db: db, clock: clock}
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
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("inspect Sandbox SQLite schema: %w", err)
	}
	if version < 0 || version > schemaVersion {
		return fmt.Errorf("Sandbox SQLite schema version %d is unsupported", version)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Sandbox SQLite schema transaction: %w", err)
	}
	defer tx.Rollback()
	for _, statement := range schemaStatements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create Sandbox SQLite schema: %w", err)
		}
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
