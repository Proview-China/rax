package applicationadapter

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	_ "modernc.org/sqlite"
)

const sqliteNextModelTurnDispatchLedgerTableV1 = `CREATE TABLE harness_next_model_turn_dispatch_schema_v1 (
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL,
  applied_unix_nano INTEGER NOT NULL CHECK(applied_unix_nano > 0)
)`

const sqliteNextModelTurnDispatchHistoryTableV1 = `CREATE TABLE harness_next_model_turn_dispatch_history_v1 (
  dispatch_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision = 1),
  fact_digest TEXT NOT NULL,
  request_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  created_unix_nano INTEGER NOT NULL CHECK(created_unix_nano > 0),
  PRIMARY KEY(dispatch_id, revision)
)`

const sqliteNextModelTurnDispatchCurrentTableV1 = `CREATE TABLE harness_next_model_turn_dispatch_current_v1 (
  dispatch_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision = 1),
  fact_digest TEXT NOT NULL,
  request_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  updated_unix_nano INTEGER NOT NULL CHECK(updated_unix_nano > 0),
  FOREIGN KEY(dispatch_id, revision)
    REFERENCES harness_next_model_turn_dispatch_history_v1(dispatch_id, revision)
)`

const sqliteNextModelTurnDispatchCurrentIndexV1 = `CREATE UNIQUE INDEX harness_next_model_turn_dispatch_current_exact_v1
  ON harness_next_model_turn_dispatch_current_v1(
    dispatch_id, revision, fact_digest, request_digest
  )`

const sqliteNextModelTurnDispatchSchemaV1 = sqliteNextModelTurnDispatchLedgerTableV1 + ";\n" +
	sqliteNextModelTurnDispatchHistoryTableV1 + ";\n" +
	sqliteNextModelTurnDispatchCurrentTableV1 + ";\n" +
	sqliteNextModelTurnDispatchCurrentIndexV1 + ";\n"

type sqliteNextModelTurnDispatchObjectV1 struct {
	kind      string
	name      string
	tableName string
	sql       string
}

var sqliteNextModelTurnDispatchExpectedObjectsV1 = map[string]sqliteNextModelTurnDispatchObjectV1{
	"harness_next_model_turn_dispatch_schema_v1": {
		kind:      "table",
		name:      "harness_next_model_turn_dispatch_schema_v1",
		tableName: "harness_next_model_turn_dispatch_schema_v1",
		sql:       sqliteNextModelTurnDispatchLedgerTableV1,
	},
	"harness_next_model_turn_dispatch_history_v1": {
		kind:      "table",
		name:      "harness_next_model_turn_dispatch_history_v1",
		tableName: "harness_next_model_turn_dispatch_history_v1",
		sql:       sqliteNextModelTurnDispatchHistoryTableV1,
	},
	"harness_next_model_turn_dispatch_current_v1": {
		kind:      "table",
		name:      "harness_next_model_turn_dispatch_current_v1",
		tableName: "harness_next_model_turn_dispatch_current_v1",
		sql:       sqliteNextModelTurnDispatchCurrentTableV1,
	},
	"harness_next_model_turn_dispatch_current_exact_v1": {
		kind:      "index",
		name:      "harness_next_model_turn_dispatch_current_exact_v1",
		tableName: "harness_next_model_turn_dispatch_current_v1",
		sql:       sqliteNextModelTurnDispatchCurrentIndexV1,
	},
}

type SQLiteNextModelTurnDispatchConfigV1 struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}

// SQLiteNextModelTurnDispatchV1 is the only durable authority introduced by
// this slice. It is a single-node append-only history plus current exact index;
// it does not own a Harness dispatch or a Model attempt/outcome.
type SQLiteNextModelTurnDispatchV1 struct {
	db    *sql.DB
	clock func() time.Time
	mu    *nextModelTurnDispatchMutexV1

	faultMu       sync.Mutex
	loseNextReply bool

	afterMutationForTesting func()
}

var sqliteNextModelTurnDispatchLocksV1 sync.Map

type nextModelTurnDispatchMutexV1 struct {
	token chan struct{}
}

func newNextModelTurnDispatchMutexV1() *nextModelTurnDispatchMutexV1 {
	mutex := &nextModelTurnDispatchMutexV1{token: make(chan struct{}, 1)}
	mutex.token <- struct{}{}
	return mutex
}

func (m *nextModelTurnDispatchMutexV1) Lock() {
	<-m.token
}

func (m *nextModelTurnDispatchMutexV1) LockContext(ctx context.Context) error {
	select {
	case <-m.token:
		return nil
	case <-ctx.Done():
		return nextModelTurnDispatchContextV1(ctx)
	}
}

func (m *nextModelTurnDispatchMutexV1) Unlock() {
	m.token <- struct{}{}
}

func OpenSQLiteNextModelTurnDispatchV1(
	ctx context.Context,
	config SQLiteNextModelTurnDispatchConfigV1,
) (*SQLiteNextModelTurnDispatchV1, error) {
	if err := nextModelTurnDispatchContextV1(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.Path) == "" {
		return nil, nextModelTurnDispatchErrorV1(core.ErrorInvalidArgument, core.ReasonInvalidReference, "next-turn SQLite path is required")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, nextModelTurnDispatchErrorV1(core.ErrorInvalidArgument, core.ReasonInvalidReference, "next-turn SQLite busy timeout exceeds one minute")
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 8
	}
	if config.MaxOpenConns > 32 {
		return nil, nextModelTurnDispatchErrorV1(core.ErrorInvalidArgument, core.ReasonInvalidReference, "next-turn SQLite connection count exceeds 32")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	absolute, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, nextModelTurnDispatchErrorV1(core.ErrorInvalidArgument, core.ReasonInvalidReference, "next-turn SQLite path is invalid")
	}
	lock, _ := sqliteNextModelTurnDispatchLocksV1.LoadOrStore(absolute, newNextModelTurnDispatchMutexV1())
	dsn := (&url.URL{Scheme: "file", Path: absolute}).String()
	dsn += fmt.Sprintf(
		"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(FULL)&_pragma=busy_timeout(%d)&_txlock=immediate",
		config.BusyTimeout.Milliseconds(),
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	store := &SQLiteNextModelTurnDispatchV1{
		db:    db,
		clock: config.Clock,
		mu:    lock.(*nextModelTurnDispatchMutexV1),
	}
	if err = store.initializeV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteNextModelTurnDispatchV1) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteNextModelTurnDispatchV1) LoseNextReplyForTestingV1() {
	if s == nil {
		return
	}
	s.faultMu.Lock()
	s.loseNextReply = true
	s.faultMu.Unlock()
}

func (s *SQLiteNextModelTurnDispatchV1) ensureNextModelTurnDispatchBindingV1(
	ctx context.Context,
	request applicationcontract.NextModelTurnDispatchRequestV1,
	current applicationcontract.NextModelTurnDispatchCurrentV1,
) (applicationcontract.NextModelTurnDispatchCurrentV1, error) {
	if err := s.preflightV1(ctx); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if err := validateNextModelTurnDispatchAttemptBoundV1(current); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if err := current.ValidateFor(request); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	payload, err := applicationcontract.EncodeNextModelTurnDispatchCurrentV1(current)
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	writeCtx, cancel := context.WithDeadline(ctx, time.Unix(0, current.NotAfterUnixNano))
	defer cancel()
	if err = s.mu.LockContext(writeCtx); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	defer s.mu.Unlock()
	now, err := freshNextModelTurnDispatchTimeV1(
		s.clock,
		time.Unix(0, current.CheckedUnixNano),
	)
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if err = current.ValidateCurrentFor(request, now); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	tx, err := s.db.BeginTx(writeCtx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, mapSQLiteNextModelTurnDispatchErrorV1(writeCtx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	stored, exists, err := inspectSQLiteNextModelTurnDispatchTxV1(
		writeCtx,
		tx,
		current.DerivedDispatch.ID,
	)
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if exists {
		storedPayload, _ := applicationcontract.EncodeNextModelTurnDispatchCurrentV1(stored)
		if !bytes.Equal(storedPayload, payload) ||
			stored.ValidateCurrentFor(request, now) != nil {
			return applicationcontract.NextModelTurnDispatchCurrentV1{}, nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "next-turn derived ID already binds another canonical payload")
		}
		return stored, nil
	}
	mutationNow, err := freshNextModelTurnDispatchTimeV1(s.clock, now)
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if err = current.ValidateCurrentFor(request, mutationNow); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	_, err = tx.ExecContext(
		writeCtx,
		`INSERT INTO harness_next_model_turn_dispatch_history_v1(dispatch_id,revision,fact_digest,request_digest,canonical_json,created_unix_nano) VALUES(?,?,?,?,?,?)`,
		current.DerivedDispatch.ID,
		current.Revision,
		string(current.Digest),
		string(current.RequestDigest),
		payload,
		current.CheckedUnixNano,
	)
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, mapSQLiteNextModelTurnDispatchErrorV1(writeCtx, err, true)
	}
	_, err = tx.ExecContext(
		writeCtx,
		`INSERT INTO harness_next_model_turn_dispatch_current_v1(dispatch_id,revision,fact_digest,request_digest,canonical_json,updated_unix_nano) VALUES(?,?,?,?,?,?)`,
		current.DerivedDispatch.ID,
		current.Revision,
		string(current.Digest),
		string(current.RequestDigest),
		payload,
		current.CheckedUnixNano,
	)
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, mapSQLiteNextModelTurnDispatchErrorV1(writeCtx, err, true)
	}
	if s.afterMutationForTesting != nil {
		s.afterMutationForTesting()
	}
	if err = nextModelTurnDispatchContextV1(writeCtx); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, nextModelTurnDispatchErrorV1(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "next-turn SQLite context canceled after mutation began")
	}
	commitNow, err := freshNextModelTurnDispatchTimeV1(s.clock, mutationNow)
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if err = current.ValidateCurrentFor(request, commitNow); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if err = tx.Commit(); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, nextModelTurnDispatchErrorV1(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "next-turn SQLite commit outcome is unknown")
	}
	if s.consumeLostReplyV1() {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, nextModelTurnDispatchErrorV1(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "next-turn SQLite reply was lost")
	}
	return current, nil
}

func (s *SQLiteNextModelTurnDispatchV1) inspectNextModelTurnDispatchBindingV1(
	ctx context.Context,
	request applicationcontract.NextModelTurnDispatchInspectRequestV1,
) (applicationcontract.NextModelTurnDispatchCurrentV1, error) {
	if err := s.preflightV1(ctx); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if err := request.Validate(); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if err := s.mu.LockContext(ctx); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	defer s.mu.Unlock()
	current, exists, err := inspectSQLiteNextModelTurnDispatchDBV1(
		ctx,
		s.db,
		request.DerivedDispatch.ID,
	)
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if !exists {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, nextModelTurnDispatchErrorV1(core.ErrorNotFound, core.ReasonInvalidReference, "next-turn durable binding is absent")
	}
	if current.DerivedDispatch != request.DerivedDispatch ||
		current.RequestDigest != request.RequestDigest ||
		validateNextModelTurnDispatchAttemptBoundV1(current) != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonBindingDrift, "next-turn derived ID exists with another exact payload")
	}
	return current, nil
}

func (s *SQLiteNextModelTurnDispatchV1) IntegrityCheckV1(ctx context.Context) error {
	if err := s.preflightV1(ctx); err != nil {
		return err
	}
	if err := s.mu.LockContext(ctx); err != nil {
		return err
	}
	defer s.mu.Unlock()
	if err := s.verifyPragmasV1(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
	}
	defer func() { _ = tx.Rollback() }()
	if err = verifySQLiteNextModelTurnDispatchStateV1(ctx, tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
	}
	return nil
}

func (s *SQLiteNextModelTurnDispatchV1) preflightV1(ctx context.Context) error {
	if s == nil || s.db == nil || s.mu == nil {
		return nextModelTurnDispatchErrorV1(core.ErrorUnavailable, core.ReasonComponentMissing, "next-turn SQLite repository is unavailable")
	}
	return nextModelTurnDispatchContextV1(ctx)
}

func (s *SQLiteNextModelTurnDispatchV1) initializeV1(ctx context.Context) error {
	if err := s.mu.LockContext(ctx); err != nil {
		return err
	}
	defer s.mu.Unlock()
	if err := nextModelTurnDispatchContextV1(ctx); err != nil {
		return err
	}
	if err := s.verifyPragmasV1(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	objects, err := inspectSQLiteNextModelTurnDispatchObjectsV1(ctx, tx)
	if err != nil {
		return err
	}
	switch {
	case len(objects) == 0:
		if err = s.migrateFreshV1(ctx, tx); err != nil {
			return err
		}
	case !hasExactSQLiteNextModelTurnDispatchObjectClosureV1(objects):
		return nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidState, "next-turn SQLite physical object closure is partial or unexpected")
	}
	if err = verifySQLiteNextModelTurnDispatchStateV1(ctx, tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return nextModelTurnDispatchErrorV1(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "next-turn SQLite initialization outcome is unknown")
	}
	return nil
}

func (s *SQLiteNextModelTurnDispatchV1) migrateFreshV1(ctx context.Context, tx *sql.Tx) error {
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return nextModelTurnDispatchErrorV1(core.ErrorPreconditionFailed, core.ReasonClockRegression, "next-turn SQLite migration clock is invalid")
	}
	for _, ddl := range []string{
		sqliteNextModelTurnDispatchLedgerTableV1,
		sqliteNextModelTurnDispatchHistoryTableV1,
		sqliteNextModelTurnDispatchCurrentTableV1,
		sqliteNextModelTurnDispatchCurrentIndexV1,
	} {
		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, true)
		}
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO harness_next_model_turn_dispatch_schema_v1(version,digest,applied_unix_nano) VALUES(1,?,?)`,
		string(core.DigestBytes([]byte(sqliteNextModelTurnDispatchSchemaV1))),
		now.UnixNano(),
	); err != nil {
		return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, true)
	}
	return nil
}

func (s *SQLiteNextModelTurnDispatchV1) verifyPragmasV1(ctx context.Context) error {
	for pragma, expected := range map[string]string{
		"journal_mode": "wal",
		"foreign_keys": "1",
		"synchronous":  "2",
	} {
		var actual string
		if err := s.db.QueryRowContext(ctx, `PRAGMA `+pragma).Scan(&actual); err != nil {
			return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
		}
		if !strings.EqualFold(actual, expected) {
			return nextModelTurnDispatchErrorV1(core.ErrorPreconditionFailed, core.ReasonInvalidState, "next-turn SQLite required pragma is inactive")
		}
	}
	return nil
}

func inspectSQLiteNextModelTurnDispatchObjectsV1(
	ctx context.Context,
	tx *sql.Tx,
) (map[string]sqliteNextModelTurnDispatchObjectV1, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT type,name,tbl_name,sql
		   FROM sqlite_master
		  WHERE sql IS NOT NULL
		    AND (
		      name LIKE 'harness_next_model_turn_dispatch_%'
		      OR tbl_name IN (
		        'harness_next_model_turn_dispatch_schema_v1',
		        'harness_next_model_turn_dispatch_history_v1',
		        'harness_next_model_turn_dispatch_current_v1'
		      )
		    )
		  ORDER BY type,name`,
	)
	if err != nil {
		return nil, mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
	}
	defer func() { _ = rows.Close() }()
	objects := make(map[string]sqliteNextModelTurnDispatchObjectV1)
	for rows.Next() {
		var object sqliteNextModelTurnDispatchObjectV1
		if err = rows.Scan(&object.kind, &object.name, &object.tableName, &object.sql); err != nil {
			return nil, mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
		}
		if _, exists := objects[object.name]; exists {
			return nil, nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidState, "next-turn SQLite physical object names are ambiguous")
		}
		objects[object.name] = object
	}
	if err = rows.Err(); err != nil {
		return nil, mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
	}
	return objects, nil
}

func hasExactSQLiteNextModelTurnDispatchObjectClosureV1(
	objects map[string]sqliteNextModelTurnDispatchObjectV1,
) bool {
	if len(objects) != len(sqliteNextModelTurnDispatchExpectedObjectsV1) {
		return false
	}
	for name, expected := range sqliteNextModelTurnDispatchExpectedObjectsV1 {
		actual, exists := objects[name]
		if !exists || actual.kind != expected.kind || actual.tableName != expected.tableName {
			return false
		}
	}
	return true
}

func verifySQLiteNextModelTurnDispatchStateV1(ctx context.Context, tx *sql.Tx) error {
	objects, err := inspectSQLiteNextModelTurnDispatchObjectsV1(ctx, tx)
	if err != nil {
		return err
	}
	if !hasExactSQLiteNextModelTurnDispatchObjectClosureV1(objects) {
		return nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidState, "next-turn SQLite physical object closure drifted")
	}
	for name, expected := range sqliteNextModelTurnDispatchExpectedObjectsV1 {
		if normalizeSQLiteNextModelTurnDispatchSQLV1(objects[name].sql) !=
			normalizeSQLiteNextModelTurnDispatchSQLV1(expected.sql) {
			return nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidState, "next-turn SQLite exact object schema drifted")
		}
	}
	rows, err := tx.QueryContext(
		ctx,
		`SELECT version,digest,applied_unix_nano FROM harness_next_model_turn_dispatch_schema_v1 ORDER BY version`,
	)
	if err != nil {
		return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
	}
	defer func() { _ = rows.Close() }()
	count := 0
	for rows.Next() {
		var version, appliedUnixNano int64
		var digest string
		if err = rows.Scan(&version, &digest, &appliedUnixNano); err != nil {
			return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
		}
		count++
		if version != 1 ||
			digest != string(core.DigestBytes([]byte(sqliteNextModelTurnDispatchSchemaV1))) ||
			appliedUnixNano <= 0 {
			return nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidDigest, "next-turn SQLite schema ledger drifted")
		}
	}
	if err = rows.Err(); err != nil {
		return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
	}
	if err = rows.Close(); err != nil {
		return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
	}
	if count != 1 {
		return nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidDigest, "next-turn SQLite schema ledger is incomplete")
	}
	var result string
	if err = tx.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
	}
	if result != "ok" {
		return nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidState, "next-turn SQLite integrity check failed")
	}
	var orphanHistory, orphanCurrent int
	if err = tx.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM harness_next_model_turn_dispatch_history_v1 h LEFT JOIN harness_next_model_turn_dispatch_current_v1 c ON c.dispatch_id=h.dispatch_id AND c.revision=h.revision WHERE c.dispatch_id IS NULL`,
	).Scan(&orphanHistory); err != nil {
		return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
	}
	if err = tx.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM harness_next_model_turn_dispatch_current_v1 c LEFT JOIN harness_next_model_turn_dispatch_history_v1 h ON h.dispatch_id=c.dispatch_id AND h.revision=c.revision WHERE h.dispatch_id IS NULL`,
	).Scan(&orphanCurrent); err != nil {
		return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
	}
	if orphanHistory != 0 || orphanCurrent != 0 {
		return nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidState, "next-turn SQLite ledger/current relation drifted")
	}
	idRows, err := tx.QueryContext(
		ctx,
		`SELECT dispatch_id FROM harness_next_model_turn_dispatch_current_v1 ORDER BY dispatch_id`,
	)
	if err != nil {
		return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
	}
	var dispatchIDs []string
	for idRows.Next() {
		var dispatchID string
		if err = idRows.Scan(&dispatchID); err != nil {
			_ = idRows.Close()
			return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
		}
		dispatchIDs = append(dispatchIDs, dispatchID)
	}
	if err = idRows.Close(); err != nil {
		return mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
	}
	for _, dispatchID := range dispatchIDs {
		if _, exists, inspectErr := inspectSQLiteNextModelTurnDispatchTxV1(ctx, tx, dispatchID); inspectErr != nil {
			return inspectErr
		} else if !exists {
			return nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidState, "next-turn SQLite exact binding disappeared during verification")
		}
	}
	return nil
}

func normalizeSQLiteNextModelTurnDispatchSQLV1(value string) string {
	return strings.Join(strings.Fields(strings.TrimSuffix(strings.TrimSpace(value), ";")), " ")
}

func inspectSQLiteNextModelTurnDispatchDBV1(
	ctx context.Context,
	db *sql.DB,
	dispatchID string,
) (applicationcontract.NextModelTurnDispatchCurrentV1, bool, error) {
	return inspectSQLiteNextModelTurnDispatchV1(ctx, db, dispatchID)
}

func inspectSQLiteNextModelTurnDispatchTxV1(
	ctx context.Context,
	tx *sql.Tx,
	dispatchID string,
) (applicationcontract.NextModelTurnDispatchCurrentV1, bool, error) {
	return inspectSQLiteNextModelTurnDispatchV1(ctx, tx, dispatchID)
}

type nextModelTurnDispatchRowScannerV1 interface {
	Scan(...any) error
}

type nextModelTurnDispatchQueryRowerV1 interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func inspectSQLiteNextModelTurnDispatchV1(
	ctx context.Context,
	queryer nextModelTurnDispatchQueryRowerV1,
	dispatchID string,
) (applicationcontract.NextModelTurnDispatchCurrentV1, bool, error) {
	current, exists, err := scanSQLiteNextModelTurnDispatchV1(
		queryer.QueryRowContext(
			ctx,
			`SELECT
			   c.revision,c.fact_digest,c.request_digest,c.canonical_json,c.updated_unix_nano,
			   h.revision,h.fact_digest,h.request_digest,h.canonical_json,h.created_unix_nano
			 FROM harness_next_model_turn_dispatch_current_v1 c
			 JOIN harness_next_model_turn_dispatch_history_v1 h
			   ON h.dispatch_id=c.dispatch_id AND h.revision=c.revision
			WHERE c.dispatch_id=?`,
			dispatchID,
		),
		dispatchID,
	)
	if err != nil || exists {
		return current, exists, err
	}
	var currentRows, historyRows int
	if err = queryer.QueryRowContext(
		ctx,
		`SELECT
		   (SELECT COUNT(*) FROM harness_next_model_turn_dispatch_current_v1 WHERE dispatch_id=?),
		   (SELECT COUNT(*) FROM harness_next_model_turn_dispatch_history_v1 WHERE dispatch_id=?)`,
		dispatchID,
		dispatchID,
	).Scan(&currentRows, &historyRows); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, false, mapSQLiteNextModelTurnDispatchErrorV1(ctx, err, false)
	}
	if currentRows != 0 || historyRows != 0 {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, false, nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidState, "next-turn SQLite history/current pair is incomplete")
	}
	return applicationcontract.NextModelTurnDispatchCurrentV1{}, false, nil
}

func scanSQLiteNextModelTurnDispatchV1(
	row nextModelTurnDispatchRowScannerV1,
	dispatchID string,
) (applicationcontract.NextModelTurnDispatchCurrentV1, bool, error) {
	var currentRevision, historyRevision uint64
	var currentFactDigest, currentRequestDigest string
	var historyFactDigest, historyRequestDigest string
	var currentPayload, historyPayload []byte
	var updatedUnixNano, createdUnixNano int64
	err := row.Scan(
		&currentRevision,
		&currentFactDigest,
		&currentRequestDigest,
		&currentPayload,
		&updatedUnixNano,
		&historyRevision,
		&historyFactDigest,
		&historyRequestDigest,
		&historyPayload,
		&createdUnixNano,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, false, nil
	}
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, false, mapSQLiteNextModelTurnDispatchErrorV1(context.Background(), err, false)
	}
	if currentRevision != historyRevision ||
		currentFactDigest != historyFactDigest ||
		currentRequestDigest != historyRequestDigest ||
		updatedUnixNano != createdUnixNano ||
		!bytes.Equal(currentPayload, historyPayload) {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, false, nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidState, "next-turn SQLite history/current facts diverged")
	}
	current, err := applicationcontract.DecodeNextModelTurnDispatchCurrentV1(currentPayload)
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, false, err
	}
	if current.DerivedDispatch.ID != dispatchID ||
		uint64(current.Revision) != currentRevision ||
		string(current.Digest) != currentFactDigest ||
		string(current.RequestDigest) != currentRequestDigest ||
		current.CheckedUnixNano != updatedUnixNano ||
		validateNextModelTurnDispatchAttemptBoundV1(current) != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, false, nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidDigest, "next-turn SQLite row metadata drifted from canonical payload")
	}
	return current, true, nil
}

func validateNextModelTurnDispatchAttemptBoundV1(
	current applicationcontract.NextModelTurnDispatchCurrentV1,
) error {
	if current.ContractVersion != applicationcontract.NextModelTurnDispatchContractVersionV1 ||
		current.DerivedDispatch.Validate() != nil ||
		current.Revision != 1 ||
		current.State != applicationcontract.NextModelTurnDispatchAttemptBoundV1 ||
		current.RequestDigest.Validate() != nil ||
		current.CheckedUnixNano <= 0 ||
		current.NotAfterUnixNano <= current.CheckedUnixNano ||
		current.Digest.Validate() != nil {
		return nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidState, "next-turn SQLite fact is not an exact attempt-bound binding")
	}
	digest, err := current.DigestV1()
	if err != nil || digest != current.Digest {
		return nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonInvalidDigest, "next-turn SQLite attempt-bound digest drifted")
	}
	return nil
}

func (s *SQLiteNextModelTurnDispatchV1) consumeLostReplyV1() bool {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	if !s.loseNextReply {
		return false
	}
	s.loseNextReply = false
	return true
}

func mapSQLiteNextModelTurnDispatchErrorV1(
	ctx context.Context,
	err error,
	write bool,
) error {
	if err == nil {
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		if write {
			return nextModelTurnDispatchErrorV1(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "next-turn SQLite write outcome is unknown")
		}
		return nextModelTurnDispatchErrorV1(core.ErrorUnavailable, core.ReasonInvalidState, "next-turn SQLite context is canceled")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonRevisionConflict, "next-turn SQLite CAS conflict")
	}
	if strings.Contains(message, "busy") || strings.Contains(message, "locked") {
		if write {
			return nextModelTurnDispatchErrorV1(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "next-turn SQLite write was interrupted by contention")
		}
		return nextModelTurnDispatchErrorV1(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "next-turn SQLite is busy")
	}
	return nextModelTurnDispatchErrorV1(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "next-turn SQLite operation failed")
}

var _ nextModelTurnDispatchBindingRepositoryV1 = (*SQLiteNextModelTurnDispatchV1)(nil)
