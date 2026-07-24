package modelinvokeradapter

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

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	_ "modernc.org/sqlite"
)

const sqlitePreparedAckSchemaV1 = `
CREATE TABLE IF NOT EXISTS harness_prepared_ack_schema_v1 (
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL,
  applied_unix_nano INTEGER NOT NULL CHECK(applied_unix_nano > 0)
);
CREATE TABLE IF NOT EXISTS harness_prepared_ack_v1 (
  ack_id TEXT PRIMARY KEY,
  ack_digest TEXT NOT NULL,
  prepared_current_key TEXT NOT NULL UNIQUE,
  prepared_ref_key TEXT NOT NULL UNIQUE,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL
);
`

type SQLitePreparedModelInvocationAckRepositoryConfigV1 struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}

// SQLitePreparedModelInvocationAckRepositoryV1 is the durable single-node
// ACK repository. It owns one transactionally consistent three-index
// create-once domain and makes no HA or remote durability claim.
type SQLitePreparedModelInvocationAckRepositoryV1 struct {
	db *sql.DB
	mu *sync.Mutex

	faultMu       sync.Mutex
	loseNextReply bool
}

var sqlitePreparedAckLocksV1 sync.Map

func OpenSQLitePreparedModelInvocationAckRepositoryV1(ctx context.Context, config SQLitePreparedModelInvocationAckRepositoryConfigV1) (*SQLitePreparedModelInvocationAckRepositoryV1, error) {
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.Path) == "" {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model ACK SQLite path is required")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model ACK SQLite busy timeout exceeds one minute")
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 8
	}
	if config.MaxOpenConns > 32 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model ACK SQLite connection count exceeds 32")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	abs, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model ACK SQLite path is invalid")
	}
	lock, _ := sqlitePreparedAckLocksV1.LoadOrStore(abs, &sync.Mutex{})
	dsn := (&url.URL{Scheme: "file", Path: abs}).String()
	dsn += fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(FULL)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapSQLitePreparedAckErrorV1(ctx, err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	repository := &SQLitePreparedModelInvocationAckRepositoryV1{db: db, mu: lock.(*sync.Mutex)}
	if err := repository.migrateV1(ctx, config.Clock); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := repository.verifyV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return repository, nil
}

func (r *SQLitePreparedModelInvocationAckRepositoryV1) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *SQLitePreparedModelInvocationAckRepositoryV1) migrateV1(ctx context.Context, clock func() time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapSQLitePreparedAckErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, sqlitePreparedAckSchemaV1); err != nil {
		return mapSQLitePreparedAckErrorV1(ctx, err, true)
	}
	now := clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model ACK SQLite migration clock is invalid")
	}
	digest := core.DigestBytes([]byte(sqlitePreparedAckSchemaV1))
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO harness_prepared_ack_schema_v1(version,digest,applied_unix_nano) VALUES(1,?,?)`, string(digest), now.UnixNano())
	if err != nil {
		return mapSQLitePreparedAckErrorV1(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapSQLitePreparedAckErrorV1(ctx, err, true)
	}
	if affected == 0 {
		var stored string
		if err = tx.QueryRowContext(ctx, `SELECT digest FROM harness_prepared_ack_schema_v1 WHERE version=1`).Scan(&stored); err != nil {
			return mapSQLitePreparedAckErrorV1(ctx, err, false)
		}
		if stored != string(digest) {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Model ACK SQLite schema digest drifted")
		}
	}
	if err = tx.Commit(); err != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Model ACK SQLite migration commit outcome is unknown")
	}
	return nil
}

func (r *SQLitePreparedModelInvocationAckRepositoryV1) verifyV1(ctx context.Context) error {
	for pragma, expected := range map[string]string{"journal_mode": "wal", "foreign_keys": "1", "synchronous": "2"} {
		var actual string
		if err := r.db.QueryRowContext(ctx, `PRAGMA `+pragma).Scan(&actual); err != nil {
			return mapSQLitePreparedAckErrorV1(ctx, err, false)
		}
		if !strings.EqualFold(actual, expected) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Model ACK SQLite required pragma is inactive")
		}
	}
	return nil
}

func (r *SQLitePreparedModelInvocationAckRepositoryV1) IntegrityCheckV1(ctx context.Context) error {
	if r == nil || r.db == nil {
		return preparedAckRepositoryUnavailableV1("Model ACK SQLite Repository is unavailable")
	}
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return err
	}
	var result string
	if err := r.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapSQLitePreparedAckErrorV1(ctx, err, false)
	}
	if result != "ok" {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "Model ACK SQLite integrity check failed")
	}
	return nil
}

func (r *SQLitePreparedModelInvocationAckRepositoryV1) LoseNextReplyForTestingV1() {
	if r == nil {
		return
	}
	r.faultMu.Lock()
	r.loseNextReply = true
	r.faultMu.Unlock()
}

func (r *SQLitePreparedModelInvocationAckRepositoryV1) EnsureAck(ctx context.Context, ack modelinvoker.PreparedModelInvocationCommitAckV1) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	if r == nil || r.db == nil || r.mu == nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryUnavailableV1("Model ACK SQLite Repository is unavailable")
	}
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	if err := ack.Validate(); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	stableKey, preparedKey, err := preparedAckRepositoryKeysV1(ack.PreparedRef, ack.CurrentRef)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	ack = ack.Clone()
	payload, rowDigest, err := encodeSQLitePreparedAckRowV1(ack)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, mapSQLitePreparedAckErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	stored, exists, err := inspectSQLitePreparedAckByIDTxV1(ctx, tx, ack.ID)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	if exists {
		if stored != ack {
			return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryConflictV1("Model ACK SQLite ID already binds different canonical content")
		}
		var storedStable, storedPrepared string
		if err = tx.QueryRowContext(ctx, `SELECT prepared_current_key,prepared_ref_key FROM harness_prepared_ack_v1 WHERE ack_id=?`, ack.ID).Scan(&storedStable, &storedPrepared); err != nil {
			return modelinvoker.PreparedModelInvocationCommitAckV1{}, mapSQLitePreparedAckErrorV1(ctx, err, false)
		}
		if storedStable != string(stableKey) || storedPrepared != string(preparedKey) {
			return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryConflictV1("Model ACK SQLite indexes drifted")
		}
		return stored.Clone(), nil
	}
	var existingID string
	err = tx.QueryRowContext(ctx, `SELECT ack_id FROM harness_prepared_ack_v1 WHERE prepared_current_key=?`, string(stableKey)).Scan(&existingID)
	if err == nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryConflictV1("Prepared+Current SQLite recovery key already binds another Model ACK")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, mapSQLitePreparedAckErrorV1(ctx, err, false)
	}
	err = tx.QueryRowContext(ctx, `SELECT ack_id FROM harness_prepared_ack_v1 WHERE prepared_ref_key=?`, string(preparedKey)).Scan(&existingID)
	if err == nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryConflictV1("Prepared SQLite epoch already binds another Current/ACK")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, mapSQLitePreparedAckErrorV1(ctx, err, false)
	}
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO harness_prepared_ack_v1(ack_id,ack_digest,prepared_current_key,prepared_ref_key,row_digest,canonical_json) VALUES(?,?,?,?,?,?)`,
		ack.ID, string(ack.Digest), string(stableKey), string(preparedKey), rowDigest, payload); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, mapSQLitePreparedAckErrorV1(ctx, err, true)
	}
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Model ACK SQLite Ensure outcome is unknown after mutation began")
	}
	if err = tx.Commit(); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Model ACK SQLite Ensure commit outcome is unknown")
	}
	if r.consumeLostReplyV1() {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Model ACK SQLite Ensure reply was lost")
	}
	return ack.Clone(), nil
}

func (r *SQLitePreparedModelInvocationAckRepositoryV1) InspectExactAck(ctx context.Context, ref modelinvoker.PreparedModelInvocationCommitAckRefV1) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	if r == nil || r.db == nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryUnavailableV1("Model ACK SQLite Repository is unavailable")
	}
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	stored, exists, err := inspectSQLitePreparedAckByIDDBV1(ctx, r.db, ref.ID)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	if !exists {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryAbsentV1("exact Model ACK SQLite row is absent")
	}
	if stored.Ref() != ref {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryConflictV1("Model ACK SQLite ID exists with another exact Ref")
	}
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	return stored.Clone(), nil
}

func (r *SQLitePreparedModelInvocationAckRepositoryV1) inspectByPreparedCurrent(ctx context.Context, prepared modelinvoker.PreparedModelInvocationRefV1, current modelinvoker.PreparedModelInvocationCurrentRefV1) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	if r == nil || r.db == nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryUnavailableV1("Model ACK SQLite Repository is unavailable")
	}
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	stableKey, preparedKey, err := preparedAckRepositoryKeysV1(prepared, current)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelSerializable})
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, mapSQLitePreparedAckErrorV1(ctx, err, false)
	}
	defer func() { _ = tx.Rollback() }()
	var ackID, ackDigest, storedStable, storedPrepared, rowDigest string
	var payload []byte
	err = tx.QueryRowContext(ctx, `SELECT ack_id,ack_digest,prepared_current_key,prepared_ref_key,row_digest,canonical_json FROM harness_prepared_ack_v1 WHERE prepared_current_key=?`, string(stableKey)).Scan(&ackID, &ackDigest, &storedStable, &storedPrepared, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		var winner string
		winnerErr := tx.QueryRowContext(ctx, `SELECT prepared_current_key FROM harness_prepared_ack_v1 WHERE prepared_ref_key=?`, string(preparedKey)).Scan(&winner)
		if winnerErr == nil && winner != string(stableKey) {
			return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryConflictV1("Prepared SQLite epoch already binds another Current/ACK")
		}
		if winnerErr != nil && !errors.Is(winnerErr, sql.ErrNoRows) {
			return modelinvoker.PreparedModelInvocationCommitAckV1{}, mapSQLitePreparedAckErrorV1(ctx, winnerErr, false)
		}
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryAbsentV1("Prepared+Current Model ACK SQLite row is authoritatively absent")
	}
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, mapSQLitePreparedAckErrorV1(ctx, err, false)
	}
	if storedStable != string(stableKey) || storedPrepared != string(preparedKey) {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryConflictV1("Model ACK SQLite indexes drifted")
	}
	stored, err := decodeSQLitePreparedAckRowV1(payload, rowDigest)
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	if err := validateSQLitePreparedAckColumnsV1(ackID, ackDigest, storedStable, storedPrepared, stored); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	if stored.PreparedRef != prepared || stored.CurrentRef != current {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, preparedAckRepositoryConflictV1("Model ACK SQLite indexes drifted")
	}
	if err := tx.Commit(); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, mapSQLitePreparedAckErrorV1(ctx, err, false)
	}
	if err := preparedAckRepositoryContextErrorV1(ctx); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, err
	}
	return stored.Clone(), nil
}

func (r *SQLitePreparedModelInvocationAckRepositoryV1) consumeLostReplyV1() bool {
	r.faultMu.Lock()
	defer r.faultMu.Unlock()
	if !r.loseNextReply {
		return false
	}
	r.loseNextReply = false
	return true
}

func encodeSQLitePreparedAckRowV1(ack modelinvoker.PreparedModelInvocationCommitAckV1) ([]byte, string, error) {
	payload, err := json.Marshal(ack)
	if err != nil || len(payload) == 0 || len(payload) > core.MaxCanonicalDocumentBytes {
		return nil, "", core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Model ACK SQLite row exceeds canonical bounds")
	}
	digest, err := core.CanonicalJSONDigest("praxis.harness.prepared-model-invocation-ack.sqlite", "v1", "PreparedModelInvocationCommitAckV1", ack)
	if err != nil {
		return nil, "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Model ACK SQLite row cannot be sealed")
	}
	return payload, string(digest), nil
}

func decodeSQLitePreparedAckRowV1(payload []byte, storedDigest string) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	var ack modelinvoker.PreparedModelInvocationCommitAckV1
	if len(payload) == 0 || storedDigest == "" || core.DecodeStrictJSON(payload, &ack) != nil {
		return ack, core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "Model ACK SQLite row is not strict canonical JSON")
	}
	digest, err := core.CanonicalJSONDigest("praxis.harness.prepared-model-invocation-ack.sqlite", "v1", "PreparedModelInvocationCommitAckV1", ack)
	if err != nil || string(digest) != storedDigest || ack.Validate() != nil {
		return ack, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Model ACK SQLite row digest or ACK fact drifted")
	}
	return ack, nil
}

func inspectSQLitePreparedAckByIDDBV1(ctx context.Context, db *sql.DB, id string) (modelinvoker.PreparedModelInvocationCommitAckV1, bool, error) {
	var payload []byte
	var ackDigest, stableKey, preparedKey, rowDigest string
	err := db.QueryRowContext(ctx, `SELECT ack_digest,prepared_current_key,prepared_ref_key,row_digest,canonical_json FROM harness_prepared_ack_v1 WHERE ack_id=?`, id).Scan(&ackDigest, &stableKey, &preparedKey, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, false, nil
	}
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, false, mapSQLitePreparedAckErrorV1(ctx, err, false)
	}
	value, err := decodeSQLitePreparedAckRowV1(payload, rowDigest)
	if err != nil {
		return value, false, err
	}
	if err := validateSQLitePreparedAckColumnsV1(id, ackDigest, stableKey, preparedKey, value); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, false, err
	}
	return value, true, nil
}

func inspectSQLitePreparedAckByIDTxV1(ctx context.Context, tx *sql.Tx, id string) (modelinvoker.PreparedModelInvocationCommitAckV1, bool, error) {
	var payload []byte
	var ackDigest, stableKey, preparedKey, rowDigest string
	err := tx.QueryRowContext(ctx, `SELECT ack_digest,prepared_current_key,prepared_ref_key,row_digest,canonical_json FROM harness_prepared_ack_v1 WHERE ack_id=?`, id).Scan(&ackDigest, &stableKey, &preparedKey, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, false, nil
	}
	if err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, false, mapSQLitePreparedAckErrorV1(ctx, err, false)
	}
	value, err := decodeSQLitePreparedAckRowV1(payload, rowDigest)
	if err != nil {
		return value, false, err
	}
	if err := validateSQLitePreparedAckColumnsV1(id, ackDigest, stableKey, preparedKey, value); err != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, false, err
	}
	return value, true, nil
}

func validateSQLitePreparedAckColumnsV1(id, ackDigest, stableKey, preparedKey string, ack modelinvoker.PreparedModelInvocationCommitAckV1) error {
	expectedStable, expectedPrepared, err := preparedAckRepositoryKeysV1(ack.PreparedRef, ack.CurrentRef)
	if err != nil {
		return err
	}
	if ack.ID != id || string(ack.Digest) != ackDigest || string(expectedStable) != stableKey || string(expectedPrepared) != preparedKey {
		return preparedAckRepositoryConflictV1("Model ACK SQLite indexed columns drifted from canonical payload")
	}
	return nil
}

func mapSQLitePreparedAckErrorV1(ctx context.Context, err error, write bool) error {
	if err == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "Model ACK SQLite context is canceled")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "busy") || strings.Contains(message, "locked") {
		return core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "Model ACK SQLite is busy")
	}
	if write {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Model ACK SQLite mutation outcome is unknown")
	}
	return core.NewError(core.ErrorUnavailable, core.ReasonInvalidState, "Model ACK SQLite read failed")
}

var _ PreparedModelInvocationAckRepositoryV1 = (*SQLitePreparedModelInvocationAckRepositoryV1)(nil)
