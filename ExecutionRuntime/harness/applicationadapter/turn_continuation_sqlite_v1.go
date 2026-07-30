package applicationadapter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	_ "modernc.org/sqlite"
)

const sqliteTurnContinuationSchemaV1 = `
CREATE TABLE IF NOT EXISTS harness_turn_continuation_schema_v1 (
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL,
  applied_unix_nano INTEGER NOT NULL CHECK(applied_unix_nano > 0)
);
CREATE TABLE IF NOT EXISTS harness_turn_continuation_current_v1 (
  attempt_id TEXT PRIMARY KEY,
  attempt_revision INTEGER NOT NULL CHECK(attempt_revision = 1),
  attempt_digest TEXT NOT NULL,
  start_digest TEXT NOT NULL,
  state TEXT NOT NULL CHECK(state IN ('continuation_pending','context_current')),
  state_revision INTEGER NOT NULL CHECK(state_revision IN (1,2)),
  state_digest TEXT NOT NULL,
  active_context_id TEXT NOT NULL,
  active_context_revision INTEGER NOT NULL CHECK(active_context_revision > 0),
  active_context_digest TEXT NOT NULL,
  commit_digest TEXT,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  updated_unix_nano INTEGER NOT NULL CHECK(updated_unix_nano > 0),
  CHECK (
    (state = 'continuation_pending' AND state_revision = 1 AND commit_digest IS NULL) OR
    (state = 'context_current' AND state_revision = 2 AND commit_digest IS NOT NULL)
  )
);
`

type SQLiteTurnContinuationStoreConfigV1 struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}

// SQLiteTurnContinuationStoreV1 is the Harness-owned single-node durable
// implementation of TurnContinuationPortV1. WAL+FULL synchronous mode makes
// no HA, remote durability, coordinator, Model invocation, or production SLA
// claim.
type SQLiteTurnContinuationStoreV1 struct {
	db    *sql.DB
	clock func() time.Time
	mu    *sync.Mutex

	faultMu             sync.Mutex
	loseNextBeginReply  bool
	loseNextCommitReply bool
}

var _ applicationports.TurnContinuationPortV1 = (*SQLiteTurnContinuationStoreV1)(nil)

var sqliteTurnContinuationLocksV1 sync.Map

func OpenSQLiteTurnContinuationStoreV1(ctx context.Context, config SQLiteTurnContinuationStoreConfigV1) (*SQLiteTurnContinuationStoreV1, error) {
	if err := turnContinuationContextErrorV1(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.Path) == "" {
		return nil, turnContinuationInvalidV1("TurnContinuation SQLite path is required")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, turnContinuationInvalidV1("TurnContinuation SQLite busy timeout exceeds one minute")
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 8
	}
	if config.MaxOpenConns > 32 {
		return nil, turnContinuationInvalidV1("TurnContinuation SQLite connection count exceeds 32")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	abs, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, turnContinuationInvalidV1("TurnContinuation SQLite path is invalid")
	}
	lock, _ := sqliteTurnContinuationLocksV1.LoadOrStore(abs, &sync.Mutex{})
	dsn := (&url.URL{Scheme: "file", Path: abs}).String()
	dsn += fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(FULL)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapSQLiteTurnContinuationErrorV1(ctx, err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	store := &SQLiteTurnContinuationStoreV1{db: db, clock: config.Clock, mu: lock.(*sync.Mutex)}
	if err := store.migrateV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.verifyPragmasV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteTurnContinuationStoreV1) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteTurnContinuationStoreV1) migrateV1(ctx context.Context) error {
	if err := s.readReadyV1(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapSQLiteTurnContinuationErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, sqliteTurnContinuationSchemaV1); err != nil {
		return mapSQLiteTurnContinuationErrorV1(ctx, err, true)
	}
	now, err := s.nowV1()
	if err != nil {
		return err
	}
	schemaDigest := core.DigestBytes([]byte(sqliteTurnContinuationSchemaV1))
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO harness_turn_continuation_schema_v1(version,digest,applied_unix_nano) VALUES(1,?,?)`, string(schemaDigest), now.UnixNano())
	if err != nil {
		return mapSQLiteTurnContinuationErrorV1(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapSQLiteTurnContinuationErrorV1(ctx, err, true)
	}
	if affected == 0 {
		var stored string
		if err = tx.QueryRowContext(ctx, `SELECT digest FROM harness_turn_continuation_schema_v1 WHERE version=1`).Scan(&stored); err != nil {
			return mapSQLiteTurnContinuationErrorV1(ctx, err, false)
		}
		if stored != string(schemaDigest) {
			return turnContinuationConflictV1(core.ReasonInvalidDigest, "TurnContinuation SQLite schema digest drifted")
		}
	}
	var schemaVersions int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM harness_turn_continuation_schema_v1`).Scan(&schemaVersions); err != nil {
		return mapSQLiteTurnContinuationErrorV1(ctx, err, false)
	}
	if schemaVersions != 1 {
		return turnContinuationConflictV1(core.ReasonInvalidDigest, "TurnContinuation SQLite schema version set drifted")
	}
	if err = tx.Commit(); err != nil {
		return turnContinuationIndeterminateV1("TurnContinuation SQLite migration commit outcome is unknown")
	}
	return nil
}

func (s *SQLiteTurnContinuationStoreV1) verifyPragmasV1(ctx context.Context) error {
	for pragma, expected := range map[string]string{"journal_mode": "wal", "foreign_keys": "1", "synchronous": "2"} {
		var actual string
		if err := s.db.QueryRowContext(ctx, `PRAGMA `+pragma).Scan(&actual); err != nil {
			return mapSQLiteTurnContinuationErrorV1(ctx, err, false)
		}
		if !strings.EqualFold(actual, expected) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "TurnContinuation SQLite required pragma is inactive")
		}
	}
	return nil
}

func (s *SQLiteTurnContinuationStoreV1) IntegrityCheckV1(ctx context.Context) error {
	if err := s.readReadyV1(ctx); err != nil {
		return err
	}
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapSQLiteTurnContinuationErrorV1(ctx, err, false)
	}
	if result != "ok" {
		return turnContinuationConflictV1(core.ReasonInvalidState, "TurnContinuation SQLite integrity check failed")
	}
	return nil
}

// LoseNextBeginReplyForTestingV1 simulates a reply lost after durable commit.
func (s *SQLiteTurnContinuationStoreV1) LoseNextBeginReplyForTestingV1() {
	if s == nil {
		return
	}
	s.faultMu.Lock()
	s.loseNextBeginReply = true
	s.faultMu.Unlock()
}

// LoseNextCommitReplyForTestingV1 simulates a reply lost after durable commit.
func (s *SQLiteTurnContinuationStoreV1) LoseNextCommitReplyForTestingV1() {
	if s == nil {
		return
	}
	s.faultMu.Lock()
	s.loseNextCommitReply = true
	s.faultMu.Unlock()
}

func (s *SQLiteTurnContinuationStoreV1) BeginTurnContinuationV1(ctx context.Context, start applicationcontract.TurnContinuationStartRequestV1) (applicationcontract.TurnContinuationCurrentV1, error) {
	if err := s.writeReadyV1(ctx); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	now, err := s.nowV1()
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	if err = start.ValidateCurrent(now); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err = turnContinuationContextErrorV1(ctx); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, mapSQLiteTurnContinuationErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	stored, exists, err := inspectSQLiteTurnContinuationTxV1(ctx, tx, start.AttemptID)
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	if exists {
		if stored.Start.AttemptRefV1() != start.AttemptRefV1() || stored.Start.Digest != start.Digest {
			return applicationcontract.TurnContinuationCurrentV1{}, turnContinuationConflictV1(core.ReasonIdempotencyPayloadMismatch, "TurnContinuation Attempt already binds another Start digest")
		}
		now, err = s.nowV1()
		if err != nil {
			return applicationcontract.TurnContinuationCurrentV1{}, err
		}
		if err = start.ValidateCurrent(now); err != nil {
			return applicationcontract.TurnContinuationCurrentV1{}, err
		}
		if err = stored.ValidateCurrent(now); err != nil {
			return applicationcontract.TurnContinuationCurrentV1{}, err
		}
		return stored.Clone(), nil
	}
	now, err = s.nowV1()
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	if err = start.ValidateCurrent(now); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	// The writer lock may have waited past the exclusive not-after bound. Only
	// this final in-transaction sample authorizes creation.
	pending, err := applicationcontract.SealTurnContinuationPendingV1(start, now.UnixNano(), start.RequestedNotAfterUnixNano)
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	payload, rowDigest, err := encodeSQLiteTurnContinuationRowV1(pending)
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO harness_turn_continuation_current_v1(attempt_id,attempt_revision,attempt_digest,start_digest,state,state_revision,state_digest,active_context_id,active_context_revision,active_context_digest,commit_digest,row_digest,canonical_json,updated_unix_nano) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		start.AttemptID, start.Revision, string(start.Digest), string(start.Digest), string(pending.State), pending.Revision, string(pending.Digest), pending.ActiveContext.ID, pending.ActiveContext.Revision, string(pending.ActiveContext.Digest), nil, rowDigest, payload, pending.CheckedUnixNano); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, mapSQLiteTurnContinuationErrorV1(ctx, err, true)
	}
	if err = turnContinuationContextErrorV1(ctx); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, turnContinuationIndeterminateV1("TurnContinuation Begin outcome is unknown after mutation began")
	}
	if err = tx.Commit(); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, turnContinuationIndeterminateV1("TurnContinuation Begin commit outcome is unknown")
	}
	if s.consumeLostBeginReplyV1() {
		return applicationcontract.TurnContinuationCurrentV1{}, turnContinuationIndeterminateV1("TurnContinuation Begin reply was lost")
	}
	return pending.Clone(), nil
}

func (s *SQLiteTurnContinuationStoreV1) CommitTurnContinuationV1(ctx context.Context, request applicationcontract.TurnContinuationCommitRequestV1) (applicationcontract.TurnContinuationCurrentV1, error) {
	if err := s.writeReadyV1(ctx); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	now, err := s.nowV1()
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err = turnContinuationContextErrorV1(ctx); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, mapSQLiteTurnContinuationErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	attemptRef := request.Pending.Start.AttemptRefV1()
	stored, exists, err := inspectSQLiteTurnContinuationTxV1(ctx, tx, attemptRef.ID)
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	if !exists {
		return applicationcontract.TurnContinuationCurrentV1{}, turnContinuationAbsentV1("exact TurnContinuation Attempt is absent")
	}
	if stored.Start.AttemptRefV1() != attemptRef {
		return applicationcontract.TurnContinuationCurrentV1{}, turnContinuationConflictV1(core.ReasonRevisionConflict, "TurnContinuation Attempt exists with another exact Start ref")
	}
	// The writer lock may have waited past the Commit deadline. Only this final
	// in-transaction sample authorizes an idempotent return or mutation.
	now, err = s.nowV1()
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	if stored.State == applicationcontract.TurnContinuationContextCurrentV1 {
		if stored.CommitRequestDigest != request.Digest {
			return applicationcontract.TurnContinuationCurrentV1{}, turnContinuationConflictV1(core.ReasonIdempotencyPayloadMismatch, "TurnContinuation Attempt already committed another request")
		}
		if err = stored.ValidateCurrent(now); err != nil {
			return applicationcontract.TurnContinuationCurrentV1{}, err
		}
		return stored.Clone(), nil
	}
	if stored.State != applicationcontract.TurnContinuationPendingV1 || stored.Revision != request.Pending.Revision || stored.Digest != request.Pending.Digest || stored.Start.Digest != request.Pending.Start.Digest {
		return applicationcontract.TurnContinuationCurrentV1{}, turnContinuationConflictV1(core.ReasonRevisionConflict, "TurnContinuation Commit does not bind the exact pending revision")
	}
	if stored.ActiveContext != request.ExpectedActiveContext {
		return applicationcontract.TurnContinuationCurrentV1{}, turnContinuationConflictV1(core.ReasonEvidenceConflict, "TurnContinuation ActiveContext CAS is stale")
	}
	if err = stored.ValidateCurrent(now); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	next, err := applicationcontract.SealTurnContinuationContextCurrentV1(request, now.UnixNano(), request.RequestedNotAfterUnixNano)
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	payload, rowDigest, err := encodeSQLiteTurnContinuationRowV1(next)
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE harness_turn_continuation_current_v1 SET state=?,state_revision=?,state_digest=?,active_context_id=?,active_context_revision=?,active_context_digest=?,commit_digest=?,row_digest=?,canonical_json=?,updated_unix_nano=? WHERE attempt_id=? AND start_digest=? AND state=? AND state_revision=? AND state_digest=? AND active_context_id=? AND active_context_revision=? AND active_context_digest=?`,
		string(next.State), next.Revision, string(next.Digest), next.ActiveContext.ID, next.ActiveContext.Revision, string(next.ActiveContext.Digest), string(request.Digest), rowDigest, payload, next.CheckedUnixNano,
		attemptRef.ID, string(request.Pending.Start.Digest), string(applicationcontract.TurnContinuationPendingV1), request.Pending.Revision, string(request.Pending.Digest), request.ExpectedActiveContext.ID, request.ExpectedActiveContext.Revision, string(request.ExpectedActiveContext.Digest))
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, mapSQLiteTurnContinuationErrorV1(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, mapSQLiteTurnContinuationErrorV1(ctx, err, true)
	}
	if affected != 1 {
		return applicationcontract.TurnContinuationCurrentV1{}, turnContinuationConflictV1(core.ReasonRevisionConflict, "TurnContinuation pending or ActiveContext CAS lost")
	}
	if err = turnContinuationContextErrorV1(ctx); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, turnContinuationIndeterminateV1("TurnContinuation Commit outcome is unknown after mutation began")
	}
	if err = tx.Commit(); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, turnContinuationIndeterminateV1("TurnContinuation Commit outcome is unknown")
	}
	if s.consumeLostCommitReplyV1() {
		return applicationcontract.TurnContinuationCurrentV1{}, turnContinuationIndeterminateV1("TurnContinuation Commit reply was lost")
	}
	return next.Clone(), nil
}

func (s *SQLiteTurnContinuationStoreV1) InspectTurnContinuationV1(ctx context.Context, request applicationcontract.TurnContinuationInspectRequestV1) (applicationcontract.TurnContinuationCurrentV1, error) {
	if err := s.readReadyV1(ctx); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	if err := request.Validate(); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	stored, exists, err := inspectSQLiteTurnContinuationDBV1(ctx, s.db, request.AttemptRef.ID)
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	if !exists {
		return applicationcontract.TurnContinuationCurrentV1{}, turnContinuationAbsentV1("exact TurnContinuation Attempt is absent")
	}
	if stored.Start.AttemptRefV1() != request.AttemptRef {
		return applicationcontract.TurnContinuationCurrentV1{}, turnContinuationConflictV1(core.ReasonRevisionConflict, "TurnContinuation Inspect requires the original exact Attempt ref")
	}
	now, err := s.nowV1()
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	if err = stored.ValidateCurrent(now); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	if err = turnContinuationContextErrorV1(ctx); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, err
	}
	return stored.Clone(), nil
}

func (s *SQLiteTurnContinuationStoreV1) nowV1() (time.Time, error) {
	if s == nil || s.clock == nil {
		return time.Time{}, turnContinuationUnavailableV1("TurnContinuation SQLite clock is unavailable")
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "TurnContinuation SQLite clock is invalid")
	}
	return now, nil
}

func (s *SQLiteTurnContinuationStoreV1) readReadyV1(ctx context.Context) error {
	if s == nil || s.db == nil {
		return turnContinuationUnavailableV1("TurnContinuation SQLite store is unavailable")
	}
	return turnContinuationContextErrorV1(ctx)
}

func (s *SQLiteTurnContinuationStoreV1) writeReadyV1(ctx context.Context) error {
	if err := s.readReadyV1(ctx); err != nil {
		return err
	}
	if s.mu == nil {
		return turnContinuationUnavailableV1("TurnContinuation SQLite writer lock is unavailable")
	}
	return nil
}

func (s *SQLiteTurnContinuationStoreV1) consumeLostBeginReplyV1() bool {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	if !s.loseNextBeginReply {
		return false
	}
	s.loseNextBeginReply = false
	return true
}

func (s *SQLiteTurnContinuationStoreV1) consumeLostCommitReplyV1() bool {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	if !s.loseNextCommitReply {
		return false
	}
	s.loseNextCommitReply = false
	return true
}

func encodeSQLiteTurnContinuationRowV1(current applicationcontract.TurnContinuationCurrentV1) ([]byte, string, error) {
	payload, err := json.Marshal(current.Clone())
	if err != nil || len(payload) == 0 || len(payload) > core.MaxCanonicalDocumentBytes {
		return nil, "", core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "TurnContinuation SQLite row exceeds canonical bounds")
	}
	digest, err := core.CanonicalJSONDigest("praxis.harness.turn-continuation.sqlite", "v1", "TurnContinuationCurrentV1", current.Clone())
	if err != nil {
		return nil, "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "TurnContinuation SQLite row cannot be sealed")
	}
	return payload, string(digest), nil
}

func decodeSQLiteTurnContinuationRowV1(payload []byte, storedDigest string) (applicationcontract.TurnContinuationCurrentV1, error) {
	var current applicationcontract.TurnContinuationCurrentV1
	if len(payload) == 0 || storedDigest == "" || core.DecodeStrictJSON(payload, &current) != nil {
		return current, turnContinuationConflictV1(core.ReasonInvalidCanonicalForm, "TurnContinuation SQLite row is not strict canonical JSON")
	}
	digest, err := core.CanonicalJSONDigest("praxis.harness.turn-continuation.sqlite", "v1", "TurnContinuationCurrentV1", current.Clone())
	if err != nil || string(digest) != storedDigest {
		return current, turnContinuationConflictV1(core.ReasonInvalidDigest, "TurnContinuation SQLite row digest drifted")
	}
	if current.CheckedUnixNano <= 0 || current.ValidateCurrent(time.Unix(0, current.CheckedUnixNano)) != nil {
		return current, turnContinuationConflictV1(core.ReasonInvalidState, "TurnContinuation SQLite canonical current is invalid")
	}
	return current.Clone(), nil
}

type sqliteTurnContinuationColumnsV1 struct {
	attemptID             string
	attemptRevision       int64
	attemptDigest         string
	startDigest           string
	state                 string
	stateRevision         int64
	stateDigest           string
	activeContextID       string
	activeContextRevision int64
	activeContextDigest   string
	commitDigest          sql.NullString
	rowDigest             string
	payload               []byte
	updatedUnixNano       int64
}

func (c *sqliteTurnContinuationColumnsV1) scan(row interface{ Scan(...any) error }) error {
	return row.Scan(&c.attemptRevision, &c.attemptDigest, &c.startDigest, &c.state, &c.stateRevision, &c.stateDigest, &c.activeContextID, &c.activeContextRevision, &c.activeContextDigest, &c.commitDigest, &c.rowDigest, &c.payload, &c.updatedUnixNano)
}

func inspectSQLiteTurnContinuationDBV1(ctx context.Context, db *sql.DB, attemptID string) (applicationcontract.TurnContinuationCurrentV1, bool, error) {
	columns := sqliteTurnContinuationColumnsV1{attemptID: attemptID}
	err := columns.scan(db.QueryRowContext(ctx, `SELECT attempt_revision,attempt_digest,start_digest,state,state_revision,state_digest,active_context_id,active_context_revision,active_context_digest,commit_digest,row_digest,canonical_json,updated_unix_nano FROM harness_turn_continuation_current_v1 WHERE attempt_id=?`, attemptID))
	return decodeSQLiteTurnContinuationInspectionV1(ctx, columns, err)
}

func inspectSQLiteTurnContinuationTxV1(ctx context.Context, tx *sql.Tx, attemptID string) (applicationcontract.TurnContinuationCurrentV1, bool, error) {
	columns := sqliteTurnContinuationColumnsV1{attemptID: attemptID}
	err := columns.scan(tx.QueryRowContext(ctx, `SELECT attempt_revision,attempt_digest,start_digest,state,state_revision,state_digest,active_context_id,active_context_revision,active_context_digest,commit_digest,row_digest,canonical_json,updated_unix_nano FROM harness_turn_continuation_current_v1 WHERE attempt_id=?`, attemptID))
	return decodeSQLiteTurnContinuationInspectionV1(ctx, columns, err)
}

func decodeSQLiteTurnContinuationInspectionV1(ctx context.Context, columns sqliteTurnContinuationColumnsV1, err error) (applicationcontract.TurnContinuationCurrentV1, bool, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return applicationcontract.TurnContinuationCurrentV1{}, false, nil
	}
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, false, mapSQLiteTurnContinuationErrorV1(ctx, err, false)
	}
	current, err := decodeSQLiteTurnContinuationRowV1(columns.payload, columns.rowDigest)
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, false, err
	}
	if err = validateSQLiteTurnContinuationColumnsV1(columns, current); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, false, err
	}
	return current.Clone(), true, nil
}

func validateSQLiteTurnContinuationColumnsV1(columns sqliteTurnContinuationColumnsV1, current applicationcontract.TurnContinuationCurrentV1) error {
	attempt := current.Start.AttemptRefV1()
	if columns.attemptID != attempt.ID || columns.attemptRevision != int64(attempt.Revision) || columns.attemptDigest != string(attempt.Digest) || columns.startDigest != string(current.Start.Digest) || columns.state != string(current.State) || columns.stateRevision != int64(current.Revision) || columns.stateDigest != string(current.Digest) || columns.activeContextID != current.ActiveContext.ID || columns.activeContextRevision != int64(current.ActiveContext.Revision) || columns.activeContextDigest != string(current.ActiveContext.Digest) || columns.updatedUnixNano != current.CheckedUnixNano {
		return turnContinuationConflictV1(core.ReasonEvidenceConflict, "TurnContinuation SQLite indexed columns drifted from canonical current")
	}
	if current.CommitRequestDigest == "" {
		if columns.commitDigest.Valid {
			return turnContinuationConflictV1(core.ReasonEvidenceConflict, "TurnContinuation SQLite pending row carries a Commit digest")
		}
	} else if !columns.commitDigest.Valid || columns.commitDigest.String != string(current.CommitRequestDigest) {
		return turnContinuationConflictV1(core.ReasonEvidenceConflict, "TurnContinuation SQLite Commit digest drifted from canonical current")
	}
	return nil
}

func turnContinuationContextErrorV1(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return turnContinuationUnavailableV1("TurnContinuation SQLite context is canceled")
	}
	return nil
}

func mapSQLiteTurnContinuationErrorV1(ctx context.Context, err error, write bool) error {
	if err == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil {
		return turnContinuationUnavailableV1("TurnContinuation SQLite context is canceled")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "busy") || strings.Contains(message, "locked") {
		return turnContinuationUnavailableV1("TurnContinuation SQLite is busy")
	}
	if write {
		return turnContinuationIndeterminateV1("TurnContinuation SQLite mutation outcome is unknown")
	}
	return turnContinuationUnavailableV1("TurnContinuation SQLite read failed")
}

func turnContinuationInvalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}

func turnContinuationUnavailableV1(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, message)
}

func turnContinuationAbsentV1(message string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, message)
}

func turnContinuationConflictV1(reason core.ReasonCode, message string) error {
	return core.NewError(core.ErrorConflict, reason, message)
}

func turnContinuationIndeterminateV1(message string) error {
	return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, message)
}
