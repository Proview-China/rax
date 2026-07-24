package assemblyadapter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	_ "modernc.org/sqlite"
)

const (
	sqliteModelPreDispatchSchemaVersionV1 = 1
	sqliteModelPreDispatchRowDomainV1     = "praxis.harness.model-predispatch.sqlite"
)

const sqliteModelPreDispatchSchemaV1 = `
CREATE TABLE IF NOT EXISTS harness_model_predispatch_schema_v1 (
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL,
  applied_unix_nano INTEGER NOT NULL CHECK(applied_unix_nano > 0)
);
CREATE TABLE IF NOT EXISTS harness_model_predispatch_assembly_history_v1 (
  id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  ref_digest TEXT NOT NULL,
  watermark_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(id, revision),
  UNIQUE(id, watermark_digest)
);
CREATE TABLE IF NOT EXISTS harness_model_predispatch_assembly_current_v1 (
  id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  ref_digest TEXT NOT NULL,
  FOREIGN KEY(id, revision) REFERENCES harness_model_predispatch_assembly_history_v1(id, revision)
);
CREATE TABLE IF NOT EXISTS harness_model_predispatch_verified_history_v1 (
  id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  ref_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(id, revision)
);
CREATE TABLE IF NOT EXISTS harness_model_predispatch_verified_current_v1 (
  id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  ref_digest TEXT NOT NULL,
  FOREIGN KEY(id, revision) REFERENCES harness_model_predispatch_verified_history_v1(id, revision)
);
`

type SQLiteModelPreDispatchStoreConfigV1 struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}

type sqliteModelPreDispatchDBV1 struct {
	db    *sql.DB
	clock func() time.Time
	mu    *sync.Mutex
}

var sqliteModelPreDispatchLocksV1 sync.Map

func openSQLiteModelPreDispatchDBV1(ctx context.Context, config SQLiteModelPreDispatchStoreConfigV1) (*sqliteModelPreDispatchDBV1, error) {
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.Path) == "" {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model pre-dispatch SQLite path is required")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model pre-dispatch SQLite busy timeout exceeds one minute")
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 8
	}
	if config.MaxOpenConns > 32 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model pre-dispatch SQLite connection count exceeds 32")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	abs, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model pre-dispatch SQLite path is invalid")
	}
	lock, _ := sqliteModelPreDispatchLocksV1.LoadOrStore(abs, &sync.Mutex{})
	dsn := (&url.URL{Scheme: "file", Path: abs}).String()
	dsn += fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(FULL)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	value := &sqliteModelPreDispatchDBV1{db: db, clock: config.Clock, mu: lock.(*sync.Mutex)}
	if err := value.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := value.verify(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return value, nil
}

func (s *sqliteModelPreDispatchDBV1) migrate(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapSQLiteModelPreDispatchErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, sqliteModelPreDispatchSchemaV1); err != nil {
		return mapSQLiteModelPreDispatchErrorV1(ctx, err, true)
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model pre-dispatch SQLite migration clock is invalid")
	}
	digest := core.DigestBytes([]byte(sqliteModelPreDispatchSchemaV1))
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO harness_model_predispatch_schema_v1(version,digest,applied_unix_nano) VALUES(?,?,?)`, sqliteModelPreDispatchSchemaVersionV1, string(digest), now.UnixNano())
	if err != nil {
		return mapSQLiteModelPreDispatchErrorV1(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapSQLiteModelPreDispatchErrorV1(ctx, err, true)
	}
	if affected == 0 {
		var stored string
		if err = tx.QueryRowContext(ctx, `SELECT digest FROM harness_model_predispatch_schema_v1 WHERE version=?`, sqliteModelPreDispatchSchemaVersionV1).Scan(&stored); err != nil {
			return mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
		}
		if stored != string(digest) {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Model pre-dispatch SQLite schema digest drifted")
		}
	}
	if err = tx.Commit(); err != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Model pre-dispatch SQLite migration commit outcome is unknown")
	}
	return nil
}

func (s *sqliteModelPreDispatchDBV1) verify(ctx context.Context) error {
	for pragma, expected := range map[string]string{"journal_mode": "wal", "foreign_keys": "1", "synchronous": "2"} {
		var actual string
		if err := s.db.QueryRowContext(ctx, `PRAGMA `+pragma).Scan(&actual); err != nil {
			return mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
		}
		if !strings.EqualFold(actual, expected) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Model pre-dispatch SQLite required pragma is inactive")
		}
	}
	return nil
}

func (s *sqliteModelPreDispatchDBV1) integrity(ctx context.Context) error {
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return err
	}
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
	}
	if result != "ok" {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "Model pre-dispatch SQLite integrity check failed")
	}
	return nil
}

func encodeSQLiteModelPreDispatchRowV1(discriminator string, value any) ([]byte, string, error) {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) == 0 || len(payload) > core.MaxCanonicalDocumentBytes {
		return nil, "", core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Model pre-dispatch SQLite row exceeds canonical bounds")
	}
	digest, err := core.CanonicalJSONDigest(sqliteModelPreDispatchRowDomainV1, "v1", discriminator, value)
	if err != nil {
		return nil, "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Model pre-dispatch SQLite row cannot be sealed")
	}
	return payload, string(digest), nil
}

func decodeSQLiteModelPreDispatchRowV1[T any](payload []byte, storedDigest, discriminator string) (T, error) {
	var value T
	if len(payload) == 0 || storedDigest == "" || core.DecodeStrictJSON(payload, &value) != nil {
		return value, core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "Model pre-dispatch SQLite row is not strict canonical JSON")
	}
	digest, err := core.CanonicalJSONDigest(sqliteModelPreDispatchRowDomainV1, "v1", discriminator, value)
	if err != nil || string(digest) != storedDigest {
		return value, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Model pre-dispatch SQLite row digest drifted")
	}
	return value, nil
}

type SQLiteModelPreDispatchAssemblyCurrentStoreV1 struct {
	state *sqliteModelPreDispatchDBV1

	faultMu       sync.Mutex
	loseNextReply bool
}

func OpenSQLiteModelPreDispatchAssemblyCurrentStoreV1(ctx context.Context, config SQLiteModelPreDispatchStoreConfigV1) (*SQLiteModelPreDispatchAssemblyCurrentStoreV1, error) {
	state, err := openSQLiteModelPreDispatchDBV1(ctx, config)
	if err != nil {
		return nil, err
	}
	return &SQLiteModelPreDispatchAssemblyCurrentStoreV1{state: state}, nil
}

func (s *SQLiteModelPreDispatchAssemblyCurrentStoreV1) Close() error {
	if s == nil || s.state == nil || s.state.db == nil {
		return nil
	}
	return s.state.db.Close()
}

func (s *SQLiteModelPreDispatchAssemblyCurrentStoreV1) IntegrityCheckV1(ctx context.Context) error {
	if s == nil || s.state == nil || s.state.db == nil {
		return componentMissingModelPreDispatchAssemblyV1("Harness Assembly SQLite Store is unavailable")
	}
	return s.state.integrity(ctx)
}

func (s *SQLiteModelPreDispatchAssemblyCurrentStoreV1) LoseNextReplyForTestingV1() {
	if s == nil {
		return
	}
	s.faultMu.Lock()
	s.loseNextReply = true
	s.faultMu.Unlock()
}

func (s *SQLiteModelPreDispatchAssemblyCurrentStoreV1) CompareAndSwapModelPreDispatchAssemblyCurrentV1(ctx context.Context, expected runtimeports.ModelPreDispatchAssemblyCurrentRefV1, next runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error) {
	if s == nil || s.state == nil || s.state.db == nil || s.state.clock == nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, componentMissingModelPreDispatchAssemblyV1("Harness Assembly SQLite Store is unavailable")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if err := next.Validate(); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if expected != (runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}) {
		if err := expected.Validate(); err != nil {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
		}
	}
	payload, rowDigest, err := encodeSQLiteModelPreDispatchRowV1("ModelPreDispatchAssemblyCurrentProjectionV1", next)
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	now := s.state.clock()
	if now.IsZero() {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Harness Assembly SQLite Store clock is unavailable")
	}
	if err := next.ValidateCurrent(next.Ref, now); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	tx, err := s.state.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, mapSQLiteModelPreDispatchErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()

	stored, exists, err := inspectSQLiteAssemblyHistoryTxV1(ctx, tx, next.Ref.ID, next.Ref.Revision)
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if exists {
		if stored != next {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Harness Assembly SQLite revision already stores different content")
		}
		current, currentExists, err := inspectSQLiteAssemblyCurrentRefTxV1(ctx, tx, next.Ref.ID)
		if err != nil {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
		}
		if !currentExists || current != next.Ref {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "historical Harness Assembly SQLite revision is no longer current")
		}
		replayNow := s.state.clock()
		if replayNow.IsZero() || replayNow.Before(now) {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Harness Assembly SQLite replay clock regressed")
		}
		if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
		}
		if err := stored.ValidateCurrent(stored.Ref, replayNow); err != nil {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
		}
		return stored, nil
	}
	current, currentExists, err := inspectSQLiteAssemblyCurrentRefTxV1(ctx, tx, next.Ref.ID)
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if !currentExists {
		if expected != (runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}) || next.Ref.Revision != 1 {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Harness Assembly SQLite initial current CAS conflicted")
		}
	} else if current != expected || next.Ref.Revision != current.Revision+1 {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Harness Assembly SQLite full current Ref CAS conflicted")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	mutationNow := s.state.clock()
	if mutationNow.IsZero() || mutationNow.Before(now) {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Harness Assembly SQLite Store clock regressed before mutation")
	}
	if err := next.ValidateCurrent(next.Ref, mutationNow); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO harness_model_predispatch_assembly_history_v1(id,revision,ref_digest,watermark_digest,row_digest,canonical_json) VALUES(?,?,?,?,?,?)`,
		next.Ref.ID, next.Ref.Revision, string(next.Ref.Digest), string(next.Ref.WatermarkDigest), rowDigest, payload); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "watermark") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Harness Assembly SQLite rejected ABA watermark reuse")
		}
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, mapSQLiteModelPreDispatchErrorV1(ctx, err, true)
	}
	if currentExists {
		result, err := tx.ExecContext(ctx, `UPDATE harness_model_predispatch_assembly_current_v1 SET revision=?,ref_digest=? WHERE id=? AND revision=? AND ref_digest=?`,
			next.Ref.Revision, string(next.Ref.Digest), next.Ref.ID, expected.Revision, string(expected.Digest))
		if err != nil {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, mapSQLiteModelPreDispatchErrorV1(ctx, err, true)
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Harness Assembly SQLite current CAS outcome is unknown after history mutation")
		}
	} else if _, err = tx.ExecContext(ctx, `INSERT INTO harness_model_predispatch_assembly_current_v1(id,revision,ref_digest) VALUES(?,?,?)`, next.Ref.ID, next.Ref.Revision, string(next.Ref.Digest)); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, mapSQLiteModelPreDispatchErrorV1(ctx, err, true)
	}
	commitNow := s.state.clock()
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Harness Assembly SQLite CAS outcome is unknown after mutation clock")
	}
	if commitNow.IsZero() || commitNow.Before(mutationNow) {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Harness Assembly SQLite CAS clock regressed after mutation")
	}
	if err := next.ValidateCurrent(next.Ref, commitNow); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Harness Assembly SQLite CAS current expired after mutation")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Harness Assembly SQLite CAS outcome is unknown after mutation began")
	}
	if err = tx.Commit(); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Harness Assembly SQLite CAS commit outcome is unknown")
	}
	if s.consumeLostReplyV1() {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Harness Assembly SQLite CAS reply was lost")
	}
	return next, nil
}

func (s *SQLiteModelPreDispatchAssemblyCurrentStoreV1) InspectHistoricalModelPreDispatchAssemblyV1(ctx context.Context, ref runtimeports.ModelPreDispatchAssemblyCurrentRefV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error) {
	if s == nil || s.state == nil || s.state.db == nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, componentMissingModelPreDispatchAssemblyV1("Harness Assembly SQLite historical Store is unavailable")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	stored, exists, err := inspectSQLiteAssemblyHistoryDBV1(ctx, s.state.db, ref.ID, ref.Revision)
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if !exists {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Harness Assembly SQLite historical revision is absent")
	}
	if stored.Ref != ref {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Harness Assembly SQLite historical exact Ref drifted")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	return stored, nil
}

func (s *SQLiteModelPreDispatchAssemblyCurrentStoreV1) InspectCurrentModelPreDispatchAssemblyV1(ctx context.Context, ref runtimeports.ModelPreDispatchAssemblyCurrentRefV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error) {
	if s == nil || s.state == nil || s.state.db == nil || s.state.clock == nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, componentMissingModelPreDispatchAssemblyV1("Harness Assembly SQLite current Reader is unavailable")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	first, err := s.inspectAssemblyCurrentSnapshotV1(ctx, ref)
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	nowS1 := s.state.clock()
	if nowS1.IsZero() {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Harness Assembly SQLite current Reader clock is unavailable")
	}
	if err := first.ValidateCurrent(ref, nowS1); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	second, err := s.inspectAssemblyCurrentSnapshotV1(ctx, ref)
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if first != second {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Harness Assembly SQLite current changed during exact read")
	}
	nowS2 := s.state.clock()
	if nowS2.IsZero() || nowS2.Before(nowS1) {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Harness Assembly SQLite current Reader clock regressed after S2")
	}
	if err := second.ValidateCurrent(ref, nowS2); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	return first, nil
}

func (s *SQLiteModelPreDispatchAssemblyCurrentStoreV1) inspectAssemblyCurrentSnapshotV1(ctx context.Context, ref runtimeports.ModelPreDispatchAssemblyCurrentRefV1) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, error) {
	tx, err := s.state.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelSerializable})
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
	}
	defer func() { _ = tx.Rollback() }()
	current, exists, err := inspectSQLiteAssemblyCurrentRefTxV1(ctx, tx, ref.ID)
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if !exists {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Harness Assembly SQLite current is absent")
	}
	if current != ref {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Harness Assembly SQLite exact historical Ref is not current")
	}
	stored, exists, err := inspectSQLiteAssemblyHistoryTxV1(ctx, tx, ref.ID, ref.Revision)
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, err
	}
	if !exists || stored.Ref != ref {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Harness Assembly SQLite current index and history drifted")
	}
	if err := tx.Commit(); err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
	}
	return stored, nil
}

func (s *SQLiteModelPreDispatchAssemblyCurrentStoreV1) consumeLostReplyV1() bool {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	if !s.loseNextReply {
		return false
	}
	s.loseNextReply = false
	return true
}

func inspectSQLiteAssemblyCurrentRefDBV1(ctx context.Context, db *sql.DB, id string) (runtimeports.ModelPreDispatchAssemblyCurrentRefV1, bool, error) {
	var revision int64
	var digest string
	err := db.QueryRowContext(ctx, `SELECT revision,ref_digest FROM harness_model_predispatch_assembly_current_v1 WHERE id=?`, id).Scan(&revision, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}, false, nil
	}
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}, false, mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
	}
	stored, exists, err := inspectSQLiteAssemblyHistoryDBV1(ctx, db, id, core.Revision(revision))
	if err != nil || !exists {
		if err != nil {
			return runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}, false, err
		}
		return runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}, false, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Harness Assembly SQLite current target is absent")
	}
	if string(stored.Ref.Digest) != digest {
		return runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}, false, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Harness Assembly SQLite current digest drifted")
	}
	return stored.Ref, true, nil
}

func inspectSQLiteAssemblyCurrentRefTxV1(ctx context.Context, tx *sql.Tx, id string) (runtimeports.ModelPreDispatchAssemblyCurrentRefV1, bool, error) {
	var revision int64
	var digest string
	err := tx.QueryRowContext(ctx, `SELECT revision,ref_digest FROM harness_model_predispatch_assembly_current_v1 WHERE id=?`, id).Scan(&revision, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}, false, nil
	}
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}, false, mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
	}
	stored, exists, err := inspectSQLiteAssemblyHistoryTxV1(ctx, tx, id, core.Revision(revision))
	if err != nil || !exists {
		if err != nil {
			return runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}, false, err
		}
		return runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}, false, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Harness Assembly SQLite current target is absent")
	}
	if string(stored.Ref.Digest) != digest {
		return runtimeports.ModelPreDispatchAssemblyCurrentRefV1{}, false, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Harness Assembly SQLite current digest drifted")
	}
	return stored.Ref, true, nil
}

func inspectSQLiteAssemblyHistoryDBV1(ctx context.Context, db *sql.DB, id string, revision core.Revision) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, bool, error) {
	var payload []byte
	var refDigest, watermarkDigest, rowDigest string
	err := db.QueryRowContext(ctx, `SELECT ref_digest,watermark_digest,row_digest,canonical_json FROM harness_model_predispatch_assembly_history_v1 WHERE id=? AND revision=?`, id, revision).Scan(&refDigest, &watermarkDigest, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, false, nil
	}
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, false, mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
	}
	value, err := decodeSQLiteModelPreDispatchRowV1[runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1](payload, rowDigest, "ModelPreDispatchAssemblyCurrentProjectionV1")
	if err != nil || value.Validate() != nil {
		if err != nil {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, false, err
		}
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, false, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Harness Assembly SQLite stored projection is invalid")
	}
	if value.Ref.ID != id || value.Ref.Revision != revision || string(value.Ref.Digest) != refDigest || string(value.Ref.WatermarkDigest) != watermarkDigest {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, false, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Harness Assembly SQLite history indexes drifted from canonical payload")
	}
	return value, true, nil
}

func inspectSQLiteAssemblyHistoryTxV1(ctx context.Context, tx *sql.Tx, id string, revision core.Revision) (runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1, bool, error) {
	var payload []byte
	var refDigest, watermarkDigest, rowDigest string
	err := tx.QueryRowContext(ctx, `SELECT ref_digest,watermark_digest,row_digest,canonical_json FROM harness_model_predispatch_assembly_history_v1 WHERE id=? AND revision=?`, id, revision).Scan(&refDigest, &watermarkDigest, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, false, nil
	}
	if err != nil {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, false, mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
	}
	value, err := decodeSQLiteModelPreDispatchRowV1[runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1](payload, rowDigest, "ModelPreDispatchAssemblyCurrentProjectionV1")
	if err != nil || value.Validate() != nil {
		if err != nil {
			return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, false, err
		}
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, false, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Harness Assembly SQLite stored projection is invalid")
	}
	if value.Ref.ID != id || value.Ref.Revision != revision || string(value.Ref.Digest) != refDigest || string(value.Ref.WatermarkDigest) != watermarkDigest {
		return runtimeports.ModelPreDispatchAssemblyCurrentProjectionV1{}, false, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Harness Assembly SQLite history indexes drifted from canonical payload")
	}
	return value, true, nil
}

type SQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1 struct {
	state *sqliteModelPreDispatchDBV1

	faultMu       sync.Mutex
	loseNextReply bool
}

func OpenSQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1(ctx context.Context, config SQLiteModelPreDispatchStoreConfigV1) (*SQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1, error) {
	state, err := openSQLiteModelPreDispatchDBV1(ctx, config)
	if err != nil {
		return nil, err
	}
	return &SQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1{state: state}, nil
}

func (*SQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) modelPreDispatchVerifiedAssemblyOwnerCurrentReaderV1() {
}

func (s *SQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) Close() error {
	if s == nil || s.state == nil || s.state.db == nil {
		return nil
	}
	return s.state.db.Close()
}

func (s *SQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) IntegrityCheckV1(ctx context.Context) error {
	if s == nil || s.state == nil || s.state.db == nil {
		return componentMissingModelPreDispatchAssemblyV1("verified Assembly SQLite Store is unavailable")
	}
	return s.state.integrity(ctx)
}

func (s *SQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) LoseNextReplyForTestingV1() {
	if s == nil {
		return
	}
	s.faultMu.Lock()
	s.loseNextReply = true
	s.faultMu.Unlock()
}

func (s *SQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) EnsureModelPreDispatchVerifiedAssemblyOwnerCurrentV1(ctx context.Context, next ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error) {
	return s.writeV1(ctx, ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1{}, next, true)
}

func (s *SQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) CompareAndSwapModelPreDispatchVerifiedAssemblyOwnerCurrentV1(ctx context.Context, expected ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1, next ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error) {
	return s.writeV1(ctx, expected, next, false)
}

func (s *SQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) writeV1(ctx context.Context, expected ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1, next ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, ensure bool) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error) {
	if s == nil || s.state == nil || s.state.db == nil || s.state.clock == nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, componentMissingModelPreDispatchAssemblyV1("verified Assembly SQLite Store is unavailable")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if !ensure {
		if err := expected.Validate(); err != nil {
			return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
		}
	}
	next, err := cloneModelPreDispatchVerifiedAssemblyV1(next)
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	payload, rowDigest, err := encodeSQLiteModelPreDispatchRowV1("ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1", next)
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	now := s.state.clock()
	if now.IsZero() {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "verified Assembly SQLite Store clock is unavailable")
	}
	if err := next.ValidateCurrent(next.Ref, now); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	tx, err := s.state.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, mapSQLiteModelPreDispatchErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	stored, exists, err := inspectSQLiteVerifiedHistoryTxV1(ctx, tx, next.Ref.ID, next.Ref.Revision)
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if exists {
		if !reflect.DeepEqual(stored, next) {
			return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "verified Assembly SQLite revision already stores different content")
		}
		current, currentExists, err := inspectSQLiteVerifiedCurrentRefTxV1(ctx, tx, next.Ref.ID)
		if err != nil {
			return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
		}
		if !currentExists || current != next.Ref {
			return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "verified Assembly SQLite historical revision is no longer current")
		}
		replayNow := s.state.clock()
		if replayNow.IsZero() || replayNow.Before(now) {
			return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "verified Assembly SQLite replay clock regressed")
		}
		if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
			return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
		}
		if err := stored.ValidateCurrent(stored.Ref, replayNow); err != nil {
			return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
		}
		return cloneModelPreDispatchVerifiedAssemblyV1(stored)
	}
	current, currentExists, err := inspectSQLiteVerifiedCurrentRefTxV1(ctx, tx, next.Ref.ID)
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if ensure {
		if currentExists || next.Ref.Revision != 1 {
			return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "verified Assembly SQLite initial Ensure conflicted")
		}
	} else if !currentExists || current != expected || next.Ref.ID != expected.ID || next.Ref.Revision != expected.Revision+1 {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "verified Assembly SQLite full Ref CAS conflicted")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	mutationNow := s.state.clock()
	if mutationNow.IsZero() || mutationNow.Before(now) {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "verified Assembly SQLite Store clock regressed before mutation")
	}
	if err := next.ValidateCurrent(next.Ref, mutationNow); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO harness_model_predispatch_verified_history_v1(id,revision,ref_digest,row_digest,canonical_json) VALUES(?,?,?,?,?)`,
		next.Ref.ID, next.Ref.Revision, string(next.Ref.Digest), rowDigest, payload); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, mapSQLiteModelPreDispatchErrorV1(ctx, err, true)
	}
	if currentExists {
		result, err := tx.ExecContext(ctx, `UPDATE harness_model_predispatch_verified_current_v1 SET revision=?,ref_digest=? WHERE id=? AND revision=? AND ref_digest=?`,
			next.Ref.Revision, string(next.Ref.Digest), next.Ref.ID, expected.Revision, string(expected.Digest))
		if err != nil {
			return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, mapSQLiteModelPreDispatchErrorV1(ctx, err, true)
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "verified Assembly SQLite current CAS outcome is unknown after history mutation")
		}
	} else if _, err = tx.ExecContext(ctx, `INSERT INTO harness_model_predispatch_verified_current_v1(id,revision,ref_digest) VALUES(?,?,?)`, next.Ref.ID, next.Ref.Revision, string(next.Ref.Digest)); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, mapSQLiteModelPreDispatchErrorV1(ctx, err, true)
	}
	commitNow := s.state.clock()
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "verified Assembly SQLite CAS outcome is unknown after mutation clock")
	}
	if commitNow.IsZero() || commitNow.Before(mutationNow) {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "verified Assembly SQLite CAS clock regressed after mutation")
	}
	if err := next.ValidateCurrent(next.Ref, commitNow); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "verified Assembly SQLite CAS current expired after mutation")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "verified Assembly SQLite CAS outcome is unknown after mutation began")
	}
	if err = tx.Commit(); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "verified Assembly SQLite CAS commit outcome is unknown")
	}
	if s.consumeLostReplyV1() {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "verified Assembly SQLite CAS reply was lost")
	}
	return cloneModelPreDispatchVerifiedAssemblyV1(next)
}

func (s *SQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) InspectHistoricalModelPreDispatchVerifiedAssemblyOwnerV1(ctx context.Context, ref ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error) {
	if s == nil || s.state == nil || s.state.db == nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, componentMissingModelPreDispatchAssemblyV1("verified Assembly SQLite historical Store is unavailable")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	stored, exists, err := inspectSQLiteVerifiedHistoryDBV1(ctx, s.state.db, ref.ID, ref.Revision)
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if !exists || stored.Ref != ref {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "verified Assembly SQLite historical revision is absent")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	return cloneModelPreDispatchVerifiedAssemblyV1(stored)
}

func (s *SQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) InspectCurrentModelPreDispatchVerifiedAssemblyOwnerV1(ctx context.Context, ref ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error) {
	if s == nil || s.state == nil || s.state.db == nil || s.state.clock == nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, componentMissingModelPreDispatchAssemblyV1("verified Assembly SQLite current Reader is unavailable")
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	first, err := s.inspectVerifiedCurrentSnapshotV1(ctx, ref)
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	nowS1 := s.state.clock()
	if nowS1.IsZero() {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "verified Assembly SQLite current Reader clock is unavailable")
	}
	if err := first.ValidateCurrent(ref, nowS1); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	second, err := s.inspectVerifiedCurrentSnapshotV1(ctx, ref)
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if !reflect.DeepEqual(first, second) {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "verified Assembly SQLite current changed during exact read")
	}
	nowS2 := s.state.clock()
	if nowS2.IsZero() || nowS2.Before(nowS1) {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "verified Assembly SQLite current Reader clock regressed after S2")
	}
	if err := second.ValidateCurrent(ref, nowS2); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if err := modelPreDispatchAssemblyContextErrorV1(ctx); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	return cloneModelPreDispatchVerifiedAssemblyV1(first)
}

func (s *SQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) inspectVerifiedCurrentSnapshotV1(ctx context.Context, ref ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, error) {
	tx, err := s.state.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelSerializable})
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
	}
	defer func() { _ = tx.Rollback() }()
	current, exists, err := inspectSQLiteVerifiedCurrentRefTxV1(ctx, tx, ref.ID)
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if !exists || current != ref {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "verified Assembly SQLite exact Ref is not current")
	}
	stored, exists, err := inspectSQLiteVerifiedHistoryTxV1(ctx, tx, ref.ID, ref.Revision)
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, err
	}
	if !exists || stored.Ref != ref {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "verified Assembly SQLite current index and history drifted")
	}
	if err := tx.Commit(); err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
	}
	return stored, nil
}

func (s *SQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1) consumeLostReplyV1() bool {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	if !s.loseNextReply {
		return false
	}
	s.loseNextReply = false
	return true
}

func inspectSQLiteVerifiedCurrentRefDBV1(ctx context.Context, db *sql.DB, id string) (ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1, bool, error) {
	var revision int64
	var digest string
	err := db.QueryRowContext(ctx, `SELECT revision,ref_digest FROM harness_model_predispatch_verified_current_v1 WHERE id=?`, id).Scan(&revision, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1{}, false, nil
	}
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1{}, false, mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
	}
	return ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1{ID: id, Revision: core.Revision(revision), Digest: core.Digest(digest)}, true, nil
}

func inspectSQLiteVerifiedCurrentRefTxV1(ctx context.Context, tx *sql.Tx, id string) (ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1, bool, error) {
	var revision int64
	var digest string
	err := tx.QueryRowContext(ctx, `SELECT revision,ref_digest FROM harness_model_predispatch_verified_current_v1 WHERE id=?`, id).Scan(&revision, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1{}, false, nil
	}
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1{}, false, mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
	}
	return ModelPreDispatchVerifiedAssemblyOwnerCurrentRefV1{ID: id, Revision: core.Revision(revision), Digest: core.Digest(digest)}, true, nil
}

func inspectSQLiteVerifiedHistoryDBV1(ctx context.Context, db *sql.DB, id string, revision core.Revision) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, bool, error) {
	var payload []byte
	var refDigest, rowDigest string
	err := db.QueryRowContext(ctx, `SELECT ref_digest,row_digest,canonical_json FROM harness_model_predispatch_verified_history_v1 WHERE id=? AND revision=?`, id, revision).Scan(&refDigest, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, false, nil
	}
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, false, mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
	}
	value, err := decodeSQLiteModelPreDispatchRowV1[ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1](payload, rowDigest, "ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1")
	if err != nil || value.Validate() != nil {
		if err != nil {
			return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, false, err
		}
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, false, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "verified Assembly SQLite stored projection is invalid")
	}
	if value.Ref.ID != id || value.Ref.Revision != revision || string(value.Ref.Digest) != refDigest {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, false, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "verified Assembly SQLite history indexes drifted from canonical payload")
	}
	return value, true, nil
}

func inspectSQLiteVerifiedHistoryTxV1(ctx context.Context, tx *sql.Tx, id string, revision core.Revision) (ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1, bool, error) {
	var payload []byte
	var refDigest, rowDigest string
	err := tx.QueryRowContext(ctx, `SELECT ref_digest,row_digest,canonical_json FROM harness_model_predispatch_verified_history_v1 WHERE id=? AND revision=?`, id, revision).Scan(&refDigest, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, false, nil
	}
	if err != nil {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, false, mapSQLiteModelPreDispatchErrorV1(ctx, err, false)
	}
	value, err := decodeSQLiteModelPreDispatchRowV1[ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1](payload, rowDigest, "ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1")
	if err != nil || value.Validate() != nil {
		if err != nil {
			return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, false, err
		}
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, false, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "verified Assembly SQLite stored projection is invalid")
	}
	if value.Ref.ID != id || value.Ref.Revision != revision || string(value.Ref.Digest) != refDigest {
		return ModelPreDispatchVerifiedAssemblyOwnerCurrentProjectionV1{}, false, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "verified Assembly SQLite history indexes drifted from canonical payload")
	}
	return value, true, nil
}

func mapSQLiteModelPreDispatchErrorV1(ctx context.Context, err error, write bool) error {
	if err == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "Model pre-dispatch SQLite context is canceled")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "busy") || strings.Contains(message, "locked") {
		return core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "Model pre-dispatch SQLite is busy")
	}
	if write {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Model pre-dispatch SQLite mutation outcome is unknown")
	}
	return core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "Model pre-dispatch SQLite read failed")
}

var _ ModelPreDispatchAssemblyCurrentStoreV1 = (*SQLiteModelPreDispatchAssemblyCurrentStoreV1)(nil)
var _ ModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1 = (*SQLiteModelPreDispatchVerifiedAssemblyOwnerCurrentStoreV1)(nil)
