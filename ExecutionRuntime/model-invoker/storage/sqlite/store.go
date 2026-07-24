// Package sqlite provides the Model Invoker Owner's single-node durable
// GovernedModelInvocationV1 repository. WAL and the row layout are
// implementation details; this package makes no HA, remote durability,
// composition-root, provider transport or SLA claim.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	_ "modernc.org/sqlite"
)

const schemaVersionV1 = 1

type Config struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
}

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, config Config) (*Store, error) {
	if err := contextErrorV1(ctx, "open"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.Path) == "" {
		return nil, errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "open", "sqlite path is required", nil)
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "open", "sqlite busy timeout exceeds one minute", nil)
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 8
	}
	if config.MaxOpenConns > 32 {
		return nil, errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "open", "sqlite connection count exceeds 32", nil)
	}
	abs, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, "open", "sqlite path is invalid", err)
	}
	dsn := (&url.URL{Scheme: "file", Path: abs}).String()
	dsn += fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapDBErrorV1(ctx, "open", err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	store := &Store{db: db}
	if err := store.migrateV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.verifyV1(ctx); err != nil {
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

func (s *Store) IntegrityCheckV1(ctx context.Context) error {
	if err := contextErrorV1(ctx, "integrity_check"); err != nil {
		return err
	}
	if s == nil || s.db == nil {
		return errorV1(modelinvoker.GovernedModelInvocationErrorUnavailable, "integrity_check", "sqlite repository is unavailable", nil)
	}
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapDBErrorV1(ctx, "integrity_check", err, false)
	}
	if result != "ok" {
		return errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "integrity_check", "sqlite integrity check failed", nil)
	}
	return nil
}

func (s *Store) migrateV1(ctx context.Context) error {
	tx, err := s.beginV1(ctx, "migrate")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, schemaV1); err != nil {
		return mapDBErrorV1(ctx, "migrate", err, true)
	}
	digest := core.DigestBytes([]byte(schemaV1))
	now := time.Now()
	if now.IsZero() || now.UnixNano() <= 0 {
		return errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "migrate", "migration clock is invalid", nil)
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO model_invoker_schema(version,digest,applied_unix_nano) VALUES(?,?,?)`, schemaVersionV1, string(digest), now.UnixNano())
	if err != nil {
		return mapDBErrorV1(ctx, "migrate", err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapDBErrorV1(ctx, "migrate", err, true)
	}
	if affected == 0 {
		var stored string
		if err := tx.QueryRowContext(ctx, `SELECT digest FROM model_invoker_schema WHERE version=?`, schemaVersionV1).Scan(&stored); err != nil {
			return mapDBErrorV1(ctx, "migrate", err, false)
		}
		if stored != string(digest) {
			return errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "migrate", "sqlite schema digest drifted", nil)
		}
	}
	if err := tx.Commit(); err != nil {
		return errorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, "migrate", "sqlite migration commit outcome is unknown", err)
	}
	return nil
}

func (s *Store) verifyV1(ctx context.Context) error {
	var journal string
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		return mapDBErrorV1(ctx, "verify", err, false)
	}
	if !strings.EqualFold(journal, "wal") {
		return errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "verify", "sqlite WAL mode is not active", nil)
	}
	var foreignKeys int
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return mapDBErrorV1(ctx, "verify", err, false)
	}
	if foreignKeys != 1 {
		return errorV1(modelinvoker.GovernedModelInvocationErrorConflict, "verify", "sqlite foreign keys are not active", nil)
	}
	return nil
}

func (s *Store) beginV1(ctx context.Context, operation string) (*sql.Tx, error) {
	if err := contextErrorV1(ctx, operation); err != nil {
		return nil, err
	}
	if s == nil || s.db == nil {
		return nil, errorV1(modelinvoker.GovernedModelInvocationErrorUnavailable, operation, "sqlite repository is unavailable", nil)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, mapDBErrorV1(ctx, operation, err, true)
	}
	return tx, nil
}

func contextErrorV1(ctx context.Context, operation string) error {
	if ctx == nil {
		return errorV1(modelinvoker.GovernedModelInvocationErrorInvalid, operation, "context is nil", nil)
	}
	if err := ctx.Err(); err != nil {
		return errorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, operation, "context ended before linearization", err)
	}
	return nil
}

func mapDBErrorV1(ctx context.Context, operation string, err error, mutation bool) error {
	if err == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return errorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, operation, "sqlite outcome is indeterminate", err)
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "locked") || strings.Contains(message, "busy") {
		return errorV1(modelinvoker.GovernedModelInvocationErrorUnavailable, operation, "sqlite is busy", err)
	}
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return errorV1(modelinvoker.GovernedModelInvocationErrorConflict, operation, "sqlite uniqueness conflict", err)
	}
	if mutation {
		return errorV1(modelinvoker.GovernedModelInvocationErrorIndeterminate, operation, "sqlite mutation outcome is unknown", err)
	}
	return errorV1(modelinvoker.GovernedModelInvocationErrorUnavailable, operation, "sqlite read failed", err)
}

func errorV1(kind modelinvoker.GovernedModelInvocationErrorKindV1, operation, message string, err error) error {
	return &modelinvoker.GovernedModelInvocationErrorV1{Kind: kind, Operation: operation, Message: message, Err: err}
}
