// Package sqlite is the single-node durable Agent Host State Plane. It uses
// SQLite WAL and makes no HA, remote durability, topology, or SLA claim.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	_ "modernc.org/sqlite"
)

// Version 6 adds the Host-owned Cleanup Closure/embedded Plan store. Version 5
// added the HostStart InputV3 sidecar and version 4 added the Host-owned Review
// Attempt to governed Model invocation association history/current index. The
// full idempotent DDL upgrades earlier stores in place, while every historical
// schema proof is read back and verified before the new proof is committed.
const schemaVersionV1 = 6

type Config struct {
	Path         string
	Owner        core.OwnerRef
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}

type Store struct {
	db    *sql.DB
	owner core.OwnerRef
	clock func() time.Time

	faultMu       sync.Mutex
	loseNextReply bool
}

func Open(ctx context.Context, config Config) (*Store, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "agent-host sqlite open requires a live context")
	}
	if strings.TrimSpace(config.Path) == "" {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "sqlite_path_missing", "agent-host sqlite path is required")
	}
	if err := config.Owner.Validate(); err != nil {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "system_ready_owner_invalid", "agent-host sqlite requires an exact SystemReady owner")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "busy_timeout_invalid", "agent-host sqlite busy timeout exceeds one minute")
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 8
	}
	if config.MaxOpenConns > 32 {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "connection_count_invalid", "agent-host sqlite connection count exceeds 32")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	abs, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, contract.NewError(contract.ErrorInvalidArgument, "sqlite_path_invalid", "agent-host sqlite path is invalid")
	}
	dsn := (&url.URL{Scheme: "file", Path: abs}).String()
	dsn += fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapDBError(ctx, err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	store := &Store{db: db, owner: config.Owner, clock: config.Clock}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.verifyPragmas(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	if s == nil || s.db == nil {
		return contract.NewError(contract.ErrorUnavailable, "sqlite_store_missing", "agent-host sqlite store is unavailable")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, schemaV6); err != nil {
		return mapDBError(ctx, err, true)
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return contract.NewError(contract.ErrorPrecondition, "clock_regression", "agent-host sqlite migration clock is invalid")
	}
	proofs := []struct {
		version int
		digest  core.Digest
	}{
		{3, core.DigestBytes([]byte(schemaBaseV3))},
		{4, core.DigestBytes([]byte(schemaV1))},
		{5, core.DigestBytes([]byte(schemaV5))},
		{schemaVersionV1, core.DigestBytes([]byte(schemaV6))},
	}
	for _, proof := range proofs {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_host_schema(version,digest,applied_unix_nano) VALUES(?,?,?)`, proof.version, string(proof.digest), now.UnixNano()); err != nil {
			return mapDBError(ctx, err, true)
		}
		var stored string
		if err = tx.QueryRowContext(ctx, `SELECT digest FROM agent_host_schema WHERE version=?`, proof.version).Scan(&stored); err != nil {
			return mapDBError(ctx, err, true)
		}
		if stored != string(proof.digest) {
			return contract.NewError(contract.ErrorConflict, "sqlite_schema_digest_drift", "agent-host sqlite schema digest drifted")
		}
	}
	if err = tx.Commit(); err != nil {
		return contract.NewError(contract.ErrorUnknownOutcome, "sqlite_commit_unknown", "agent-host sqlite migration commit outcome is unknown")
	}
	return nil
}

func (s *Store) verifyPragmas(ctx context.Context) error {
	var journal string
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		return mapDBError(ctx, err, false)
	}
	if !strings.EqualFold(journal, "wal") {
		return contract.NewError(contract.ErrorPrecondition, "sqlite_wal_inactive", "agent-host sqlite WAL mode is not active")
	}
	var foreignKeys int
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return mapDBError(ctx, err, false)
	}
	if foreignKeys != 1 {
		return contract.NewError(contract.ErrorPrecondition, "sqlite_foreign_keys_inactive", "agent-host sqlite foreign keys are not active")
	}
	return nil
}

func (s *Store) IntegrityCheckV1(ctx context.Context) error {
	if err := s.readReady(ctx); err != nil {
		return err
	}
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapDBError(ctx, err, false)
	}
	if result != "ok" {
		return contract.NewError(contract.ErrorConflict, "sqlite_integrity_failed", "agent-host sqlite integrity check failed")
	}
	return nil
}

func (s *Store) beginMutation(ctx context.Context) (*sql.Tx, error) {
	if err := s.writeReady(ctx); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, mapDBError(ctx, err, true)
	}
	return tx, nil
}

func (s *Store) readReady(ctx context.Context) error {
	if s == nil || s.db == nil {
		return contract.NewError(contract.ErrorUnavailable, "sqlite_store_missing", "agent-host sqlite store is unavailable")
	}
	if ctx == nil {
		return contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if ctx.Err() != nil {
		return contract.NewError(contract.ErrorUnavailable, "context_ended", "agent-host sqlite read context ended")
	}
	return nil
}

func (s *Store) writeReady(ctx context.Context) error {
	if s == nil || s.db == nil {
		return contract.NewError(contract.ErrorUnavailable, "sqlite_store_missing", "agent-host sqlite store is unavailable")
	}
	if ctx == nil {
		return contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	if ctx.Err() != nil {
		return contract.NewError(contract.ErrorUnknownOutcome, "context_ended", "agent-host sqlite mutation context ended")
	}
	return nil
}

func (s *Store) finishMutation(ctx context.Context, tx *sql.Tx) error {
	if err := tx.Commit(); err != nil {
		return mapDBError(ctx, err, true)
	}
	if s.consumeLostReply() {
		return contract.NewError(contract.ErrorUnknownOutcome, "sqlite_reply_lost", "agent-host sqlite committed but its reply was lost")
	}
	return nil
}

func mapDBError(ctx context.Context, err error, mutation bool) error {
	if err == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if mutation {
			return contract.NewError(contract.ErrorUnknownOutcome, "sqlite_outcome_unknown", "agent-host sqlite mutation outcome is unknown")
		}
		return contract.NewError(contract.ErrorUnavailable, "sqlite_read_unavailable", "agent-host sqlite read is unavailable")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "locked") || strings.Contains(message, "busy") {
		return contract.NewError(contract.ErrorUnavailable, "sqlite_busy", "agent-host sqlite is busy")
	}
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return contract.NewError(contract.ErrorConflict, "sqlite_uniqueness_conflict", "agent-host sqlite uniqueness conflict")
	}
	if mutation {
		return contract.NewError(contract.ErrorUnknownOutcome, "sqlite_outcome_unknown", "agent-host sqlite mutation outcome is unknown")
	}
	return contract.NewError(contract.ErrorUnavailable, "sqlite_read_unavailable", "agent-host sqlite read failed")
}

func (s *Store) consumeLostReply() bool {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	if !s.loseNextReply {
		return false
	}
	s.loseNextReply = false
	return true
}

var _ hostports.HostStartClaimPortV1 = (*Store)(nil)
var _ hostports.JournalFactPortV2 = (*Store)(nil)
var _ hostports.SystemReadyFactPortV2 = (*Store)(nil)
var _ hostports.SystemReadyAvailabilitySourceV2 = (*Store)(nil)
var _ hostports.CleanupAttemptFactPortV2 = (*Store)(nil)
