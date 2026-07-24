// Package sqlite provides the single-node durable Runtime Binding State Plane.
// WAL and the row layout are implementation details. This package makes no HA,
// remote durability, topology or SLA claim.
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

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	_ "modernc.org/sqlite"
)

const (
	schemaVersionV1 = 1
	schemaVersionV2 = 2
	schemaVersionV3 = 3
	schemaVersionV4 = 4
	schemaVersionV5 = 5
	schemaVersionV6 = 6
)

type Config struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}

type Store struct {
	db    *sql.DB
	clock func() time.Time

	faultMu       sync.Mutex
	failNextStage bool
	loseNextReply bool
}

func Open(ctx context.Context, config Config) (*Store, error) {
	if err := contextError(ctx, "open"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.Path) == "" {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "runtime Binding sqlite path is required")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "runtime Binding sqlite busy timeout exceeds its bound")
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 8
	}
	if config.MaxOpenConns > 32 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "runtime Binding sqlite connection count exceeds its bound")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	abs, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "runtime Binding sqlite path is invalid")
	}
	dsn := (&url.URL{Scheme: "file", Path: abs}).String()
	dsn += fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapDBError(ctx, err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	store := &Store{db: db, clock: config.Clock}
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
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, schemaV1); err != nil {
		return mapDBError(ctx, err, true)
	}
	digest := core.DigestBytes([]byte(schemaV1))
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "runtime Binding sqlite migration clock is invalid")
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO runtime_binding_schema(version,digest,applied_unix_nano) VALUES(?,?,?)`, schemaVersionV1, string(digest), now.UnixNano())
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected == 0 {
		var stored string
		if err := tx.QueryRowContext(ctx, `SELECT digest FROM runtime_binding_schema WHERE version=?`, schemaVersionV1).Scan(&stored); err != nil {
			return mapDBError(ctx, err, true)
		}
		if stored != string(digest) {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "runtime Binding sqlite schema digest drifted")
		}
	}
	if _, err := tx.ExecContext(ctx, schemaV2); err != nil {
		return mapDBError(ctx, err, true)
	}
	digestV2 := core.DigestBytes([]byte(schemaV2))
	result, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO runtime_binding_schema(version,digest,applied_unix_nano) VALUES(?,?,?)`, schemaVersionV2, string(digestV2), now.UnixNano())
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err = result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected == 0 {
		var stored string
		if err := tx.QueryRowContext(ctx, `SELECT digest FROM runtime_binding_schema WHERE version=?`, schemaVersionV2).Scan(&stored); err != nil {
			return mapDBError(ctx, err, true)
		}
		if stored != string(digestV2) {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "runtime Binding sqlite schema v2 digest drifted")
		}
	}
	if _, err := tx.ExecContext(ctx, schemaV3); err != nil {
		return mapDBError(ctx, err, true)
	}
	digestV3 := core.DigestBytes([]byte(schemaV3))
	result, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO runtime_binding_schema(version,digest,applied_unix_nano) VALUES(?,?,?)`, schemaVersionV3, string(digestV3), now.UnixNano())
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err = result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected == 0 {
		var stored string
		if err := tx.QueryRowContext(ctx, `SELECT digest FROM runtime_binding_schema WHERE version=?`, schemaVersionV3).Scan(&stored); err != nil {
			return mapDBError(ctx, err, true)
		}
		if stored != string(digestV3) {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "runtime Binding sqlite schema v3 digest drifted")
		}
	}
	if _, err := tx.ExecContext(ctx, schemaV4); err != nil {
		return mapDBError(ctx, err, true)
	}
	digestV4 := core.DigestBytes([]byte(schemaV4))
	result, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO runtime_binding_schema(version,digest,applied_unix_nano) VALUES(?,?,?)`, schemaVersionV4, string(digestV4), now.UnixNano())
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err = result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected == 0 {
		var stored string
		if err := tx.QueryRowContext(ctx, `SELECT digest FROM runtime_binding_schema WHERE version=?`, schemaVersionV4).Scan(&stored); err != nil {
			return mapDBError(ctx, err, true)
		}
		if stored != string(digestV4) {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "runtime Binding sqlite schema v4 digest drifted")
		}
	}
	if _, err := tx.ExecContext(ctx, schemaV5); err != nil {
		return mapDBError(ctx, err, true)
	}
	digestV5 := core.DigestBytes([]byte(schemaV5))
	result, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO runtime_binding_schema(version,digest,applied_unix_nano) VALUES(?,?,?)`, schemaVersionV5, string(digestV5), now.UnixNano())
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err = result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected == 0 {
		var stored string
		if err := tx.QueryRowContext(ctx, `SELECT digest FROM runtime_binding_schema WHERE version=?`, schemaVersionV5).Scan(&stored); err != nil {
			return mapDBError(ctx, err, true)
		}
		if stored != string(digestV5) {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "runtime Binding sqlite schema v5 digest drifted")
		}
	}
	if _, err := tx.ExecContext(ctx, schemaV6); err != nil {
		return mapDBError(ctx, err, true)
	}
	digestV6 := core.DigestBytes([]byte(schemaV6))
	result, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO runtime_binding_schema(version,digest,applied_unix_nano) VALUES(?,?,?)`, schemaVersionV6, string(digestV6), now.UnixNano())
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err = result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected == 0 {
		var stored string
		if err := tx.QueryRowContext(ctx, `SELECT digest FROM runtime_binding_schema WHERE version=?`, schemaVersionV6).Scan(&stored); err != nil {
			return mapDBError(ctx, err, true)
		}
		if stored != string(digestV6) {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "runtime Binding sqlite schema v6 digest drifted")
		}
	}
	if err := tx.Commit(); err != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "runtime Binding sqlite migration commit outcome is unknown")
	}
	return nil
}

func (s *Store) verifyPragmas(ctx context.Context) error {
	var journal string
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		return mapDBError(ctx, err, false)
	}
	if !strings.EqualFold(journal, "wal") {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "runtime Binding sqlite WAL mode is not active")
	}
	var foreignKeys int
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return mapDBError(ctx, err, false)
	}
	if foreignKeys != 1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "runtime Binding sqlite foreign keys are not active")
	}
	return nil
}

func (s *Store) IntegrityCheckV1(ctx context.Context) error {
	if err := contextError(ctx, "integrity check"); err != nil {
		return err
	}
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapDBError(ctx, err, false)
	}
	if result != "ok" {
		return core.NewError(core.ErrorInternal, core.ReasonInvalidDigest, "runtime Binding sqlite integrity check failed")
	}
	return nil
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) begin(ctx context.Context) (*sql.Tx, error) {
	if err := contextError(ctx, "transaction"); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, mapDBError(ctx, err, true)
	}
	return tx, nil
}

func commit(ctx context.Context, tx *sql.Tx) error {
	if err := tx.Commit(); err != nil {
		return mapDBError(ctx, err, true)
	}
	return nil
}

func contextError(ctx context.Context, operation string) error {
	if ctx == nil || ctx.Err() != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "runtime Binding sqlite "+operation+" context ended")
	}
	return nil
}

func mapDBError(ctx context.Context, err error, mutation bool) error {
	if err == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "runtime Binding sqlite outcome is indeterminate")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "locked") || strings.Contains(message, "busy") {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "runtime Binding sqlite is busy")
	}
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "runtime Binding sqlite uniqueness conflict")
	}
	if mutation {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "runtime Binding sqlite mutation outcome is unknown")
	}
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "runtime Binding sqlite read failed")
}

func (s *Store) consumeStageFailure() bool {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	if !s.failNextStage {
		return false
	}
	s.failNextStage = false
	return true
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

func cloneStrict[T any](value T) (T, error) {
	var zero T
	payload, err := marshalStrict(value)
	if err != nil {
		return zero, err
	}
	if err := core.DecodeStrictJSON(payload, &zero); err != nil {
		return zero, err
	}
	return zero, nil
}

var _ control.BindingFactPortV2 = (*Store)(nil)
var _ control.BindingRenewalPortV2 = (*Store)(nil)
var _ ports.ReviewBindingAuthoritativeCurrentReaderV1 = (*Store)(nil)
var _ ports.ReviewBindingConsumerAssociationCurrentReaderV1 = (*Store)(nil)
var _ ports.ReviewBindingProjectionPublisherV1 = (*Store)(nil)
var _ control.ReviewBindingAssociationProjectionPublisherV1 = (*Store)(nil)
var _ ports.OperationReviewAuthorizationFactPortV4 = (*Store)(nil)
var _ ports.OperationReviewAuthorizationFactPortV5 = (*Store)(nil)
var _ ports.HumanQuorumPolicyCurrentReaderV2 = (*Store)(nil)
var _ ports.HumanQuorumPolicyCurrentPublisherV2 = (*Store)(nil)
