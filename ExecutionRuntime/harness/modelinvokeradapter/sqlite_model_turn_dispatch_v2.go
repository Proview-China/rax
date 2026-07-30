package modelinvokeradapter

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
	"unicode"

	bridgecontract "github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	_ "modernc.org/sqlite"
)

const sqliteModelTurnDispatchSchemaV2 = `
CREATE TABLE IF NOT EXISTS harness_model_turn_dispatch_schema_v2 (
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL,
  applied_unix_nano INTEGER NOT NULL CHECK(applied_unix_nano > 0)
);
CREATE TABLE IF NOT EXISTS harness_model_turn_dispatch_v2 (
  dispatch_id TEXT PRIMARY KEY,
  dispatch_ref_digest TEXT NOT NULL,
  ack_ref_digest TEXT NOT NULL,
  fact_digest TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  state TEXT NOT NULL,
  not_after_unix_nano INTEGER NOT NULL CHECK(not_after_unix_nano > 0),
  canonical_json BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS harness_model_turn_dispatch_v2_exact
  ON harness_model_turn_dispatch_v2(dispatch_id,dispatch_ref_digest,ack_ref_digest,fact_digest,revision,state,not_after_unix_nano);
`

const (
	sqliteModelTurnDispatchLedgerTableDDLV2 = `CREATE TABLE harness_model_turn_dispatch_schema_v2 (
	  version INTEGER PRIMARY KEY,
	  digest TEXT NOT NULL,
	  applied_unix_nano INTEGER NOT NULL CHECK(applied_unix_nano > 0)
	)`
	sqliteModelTurnDispatchFactTableDDLV2 = `CREATE TABLE harness_model_turn_dispatch_v2 (
	  dispatch_id TEXT PRIMARY KEY,
	  dispatch_ref_digest TEXT NOT NULL,
	  ack_ref_digest TEXT NOT NULL,
	  fact_digest TEXT NOT NULL,
	  revision INTEGER NOT NULL CHECK(revision > 0),
	  state TEXT NOT NULL,
	  not_after_unix_nano INTEGER NOT NULL CHECK(not_after_unix_nano > 0),
	  canonical_json BLOB NOT NULL
	)`
	sqliteModelTurnDispatchExactIndexDDLV2 = `CREATE INDEX harness_model_turn_dispatch_v2_exact
	  ON harness_model_turn_dispatch_v2(dispatch_id,dispatch_ref_digest,ack_ref_digest,fact_digest,revision,state,not_after_unix_nano)`
)

type sqliteModelTurnDispatchColumnV2 struct {
	name    string
	kind    string
	notNull int
	pk      int
	hidden  int
}

var sqliteModelTurnDispatchLedgerColumnsV2 = []sqliteModelTurnDispatchColumnV2{
	{name: "version", kind: "INTEGER", pk: 1},
	{name: "digest", kind: "TEXT", notNull: 1},
	{name: "applied_unix_nano", kind: "INTEGER", notNull: 1},
}

var sqliteModelTurnDispatchFactColumnsV2 = []sqliteModelTurnDispatchColumnV2{
	{name: "dispatch_id", kind: "TEXT", pk: 1},
	{name: "dispatch_ref_digest", kind: "TEXT", notNull: 1},
	{name: "ack_ref_digest", kind: "TEXT", notNull: 1},
	{name: "fact_digest", kind: "TEXT", notNull: 1},
	{name: "revision", kind: "INTEGER", notNull: 1},
	{name: "state", kind: "TEXT", notNull: 1},
	{name: "not_after_unix_nano", kind: "INTEGER", notNull: 1},
	{name: "canonical_json", kind: "BLOB", notNull: 1},
}

type SQLiteModelTurnDispatchConfigV2 struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}

// SQLiteModelTurnDispatchV2 is a durable single-node Harness sidecar. It owns
// only attempt_bound -> outcome_bound references and makes no HA, Provider,
// or production composition claim.
type SQLiteModelTurnDispatchV2 struct {
	db *sql.DB
	mu *sync.Mutex

	faultMu       sync.Mutex
	loseNextReply bool
}

var sqliteModelTurnDispatchLocksV2 sync.Map

func OpenSQLiteModelTurnDispatchV2(
	ctx context.Context,
	config SQLiteModelTurnDispatchConfigV2,
) (*SQLiteModelTurnDispatchV2, error) {
	if err := exactModelTurnContextV2(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.Path) == "" {
		return nil, exactModelTurnErrorV2(core.ErrorInvalidArgument, core.ReasonInvalidReference, "model-turn V2 SQLite path is required")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, exactModelTurnErrorV2(core.ErrorInvalidArgument, core.ReasonInvalidReference, "model-turn V2 SQLite busy timeout exceeds one minute")
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 8
	}
	if config.MaxOpenConns > 32 {
		return nil, exactModelTurnErrorV2(core.ErrorInvalidArgument, core.ReasonInvalidReference, "model-turn V2 SQLite connection count exceeds 32")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	absolute, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, exactModelTurnErrorV2(core.ErrorInvalidArgument, core.ReasonInvalidReference, "model-turn V2 SQLite path is invalid")
	}
	lock, _ := sqliteModelTurnDispatchLocksV2.LoadOrStore(absolute, &sync.Mutex{})
	dsn := (&url.URL{Scheme: "file", Path: absolute}).String()
	dsn += fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(FULL)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	store := &SQLiteModelTurnDispatchV2{db: db, mu: lock.(*sync.Mutex)}
	if err := store.initializeV2(ctx, config.Clock); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteModelTurnDispatchV2) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteModelTurnDispatchV2) LoseNextReplyForTestingV2() {
	if s == nil {
		return
	}
	s.faultMu.Lock()
	s.loseNextReply = true
	s.faultMu.Unlock()
}

func (s *SQLiteModelTurnDispatchV2) EnsureModelTurnDispatchAttemptV2(
	ctx context.Context,
	fact bridgecontract.ModelTurnDispatchFactV2,
) (bridgecontract.ModelTurnDispatchFactV2, error) {
	if err := s.preflightV2(ctx); err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	if err := fact.Validate(); err != nil ||
		fact.State != bridgecontract.ModelTurnDispatchAttemptBoundV2 {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorInvalidArgument, core.ReasonInvalidState, "model-turn V2 Ensure requires attempt_bound")
	}
	payload, err := bridgecontract.EncodeModelTurnDispatchFactV2(fact)
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, mapSQLiteModelTurnDispatchErrorV2(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	stored, exists, err := inspectSQLiteModelTurnDispatchTxV2(ctx, tx, fact.Ref.ID)
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	if exists {
		storedPayload, _ := bridgecontract.EncodeModelTurnDispatchFactV2(stored)
		if !bytes.Equal(storedPayload, payload) {
			return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "model-turn V2 ID already binds another canonical Fact")
		}
		return stored.CloneV2(), nil
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO harness_model_turn_dispatch_v2(dispatch_id,dispatch_ref_digest,ack_ref_digest,fact_digest,revision,state,not_after_unix_nano,canonical_json) VALUES(?,?,?,?,?,?,?,?)`,
		fact.Ref.ID,
		string(fact.Ref.Digest),
		string(fact.Ref.Envelope.AckRef.Digest),
		string(fact.Digest),
		fact.Revision,
		string(fact.State),
		fact.NotAfterUnixNano,
		payload,
	)
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, mapSQLiteModelTurnDispatchErrorV2(ctx, err, true)
	}
	if err = tx.Commit(); err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "model-turn V2 Ensure commit outcome is unknown")
	}
	if s.consumeLostReplyV2() {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "model-turn V2 Ensure reply was lost")
	}
	return fact.CloneV2(), nil
}

func (s *SQLiteModelTurnDispatchV2) BindModelTurnDispatchOutcomeV2(
	ctx context.Context,
	next bridgecontract.ModelTurnDispatchFactV2,
) (bridgecontract.ModelTurnDispatchFactV2, error) {
	if err := s.preflightV2(ctx); err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	if err := next.Validate(); err != nil ||
		next.State != bridgecontract.ModelTurnDispatchOutcomeBoundV2 {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorInvalidArgument, core.ReasonInvalidState, "model-turn V2 Bind requires outcome_bound")
	}
	nextPayload, err := bridgecontract.EncodeModelTurnDispatchFactV2(next)
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, mapSQLiteModelTurnDispatchErrorV2(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	current, exists, err := inspectSQLiteModelTurnDispatchTxV2(ctx, tx, next.Ref.ID)
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	if !exists {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorNotFound, core.ReasonInvalidReference, "model-turn V2 attempt Fact is absent")
	}
	currentPayload, _ := bridgecontract.EncodeModelTurnDispatchFactV2(current)
	if bytes.Equal(currentPayload, nextPayload) {
		return current.CloneV2(), nil
	}
	if current.State != bridgecontract.ModelTurnDispatchAttemptBoundV2 || next.Outcome == nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidState, "model-turn V2 sidecar transition is not append-only")
	}
	expected, err := bridgecontract.BindModelTurnDispatchOutcomeV2(current, *next.Outcome)
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	expectedPayload, _ := bridgecontract.EncodeModelTurnDispatchFactV2(expected)
	if !bytes.Equal(expectedPayload, nextPayload) {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "model-turn V2 Outcome transition differs from current Fact")
	}
	result, err := tx.ExecContext(
		ctx,
		`UPDATE harness_model_turn_dispatch_v2 SET fact_digest=?,revision=?,state=?,not_after_unix_nano=?,canonical_json=? WHERE dispatch_id=? AND fact_digest=? AND revision=?`,
		string(next.Digest),
		next.Revision,
		string(next.State),
		next.NotAfterUnixNano,
		nextPayload,
		next.Ref.ID,
		string(current.Digest),
		current.Revision,
	)
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, mapSQLiteModelTurnDispatchErrorV2(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorConflict, core.ReasonRevisionConflict, "model-turn V2 sidecar CAS lost")
	}
	if err = tx.Commit(); err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "model-turn V2 Bind commit outcome is unknown")
	}
	if s.consumeLostReplyV2() {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "model-turn V2 Bind reply was lost")
	}
	return next.CloneV2(), nil
}

func (s *SQLiteModelTurnDispatchV2) InspectExactModelTurnDispatchV2(
	ctx context.Context,
	ref bridgecontract.ModelTurnDispatchRefV2,
) (bridgecontract.ModelTurnDispatchFactV2, error) {
	if err := s.preflightV2(ctx); err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	defer func() { _ = tx.Rollback() }()
	stored, exists, err := inspectSQLiteModelTurnDispatchTxV2(ctx, tx, ref.ID)
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, err
	}
	if !exists {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorNotFound, core.ReasonInvalidReference, "exact model-turn V2 Dispatch is absent")
	}
	if stored.Ref != ref {
		return bridgecontract.ModelTurnDispatchFactV2{}, exactModelTurnErrorV2(core.ErrorConflict, core.ReasonBindingDrift, "model-turn V2 ID exists with another exact Ref")
	}
	return stored.CloneV2(), nil
}

func (s *SQLiteModelTurnDispatchV2) IntegrityCheckV2(ctx context.Context) error {
	if err := s.preflightV2(ctx); err != nil {
		return err
	}
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	if result != "ok" {
		return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidState, "model-turn V2 SQLite integrity check failed")
	}
	return nil
}

func (s *SQLiteModelTurnDispatchV2) preflightV2(ctx context.Context) error {
	if s == nil || s.db == nil || s.mu == nil {
		return exactModelTurnErrorV2(core.ErrorUnavailable, core.ReasonComponentMissing, "model-turn V2 SQLite sidecar is unavailable")
	}
	return exactModelTurnContextV2(ctx)
}

func (s *SQLiteModelTurnDispatchV2) initializeV2(ctx context.Context, clock func() time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	present, err := inspectSQLiteModelTurnDispatchSchemaObjectsV2(ctx, s.db)
	if err != nil {
		return err
	}
	switch present {
	case 0:
		if err := s.migrateV2(ctx, clock); err != nil {
			return err
		}
	case 3:
		// An already-applied schema is never repaired. verifyV2 must observe
		// the exact ledger and physical objects that existed at Open time.
	default:
		return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite schema is partial")
	}
	return s.verifyV2(ctx)
}

func inspectSQLiteModelTurnDispatchSchemaObjectsV2(ctx context.Context, db *sql.DB) (int, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT name FROM sqlite_master
		 WHERE name IN (
		   'harness_model_turn_dispatch_schema_v2',
		   'harness_model_turn_dispatch_v2',
		   'harness_model_turn_dispatch_v2_exact'
		 )
		 ORDER BY name`,
	)
	if err != nil {
		return 0, mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	seen := make(map[string]struct{}, 3)
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			_ = rows.Close()
			return 0, mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
		}
		if _, exists := seen[name]; exists {
			_ = rows.Close()
			return 0, exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite schema object is duplicated")
		}
		seen[name] = struct{}{}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return 0, mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	if err = rows.Close(); err != nil {
		return 0, mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	return len(seen), nil
}

func (s *SQLiteModelTurnDispatchV2) migrateV2(ctx context.Context, clock func() time.Time) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, sqliteModelTurnDispatchSchemaV2); err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, true)
	}
	now := clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return exactModelTurnErrorV2(core.ErrorPreconditionFailed, core.ReasonClockRegression, "model-turn V2 SQLite migration clock is invalid")
	}
	digest := core.DigestBytes([]byte(sqliteModelTurnDispatchSchemaV2))
	result, err := tx.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO harness_model_turn_dispatch_schema_v2(version,digest,applied_unix_nano) VALUES(2,?,?)`,
		string(digest),
		now.UnixNano(),
	)
	if err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, true)
	}
	if _, err = result.RowsAffected(); err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, true)
	}
	if err = verifySQLiteModelTurnDispatchLedgerRowsV2(ctx, tx, digest); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return exactModelTurnErrorV2(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "model-turn V2 SQLite migration outcome is unknown")
	}
	return nil
}

func (s *SQLiteModelTurnDispatchV2) verifyV2(ctx context.Context) error {
	if err := verifySQLiteModelTurnDispatchLedgerRowsV2(
		ctx,
		s.db,
		core.DigestBytes([]byte(sqliteModelTurnDispatchSchemaV2)),
	); err != nil {
		return err
	}
	for pragma, expected := range map[string]string{
		"journal_mode": "wal",
		"foreign_keys": "1",
		"synchronous":  "2",
	} {
		var actual string
		if err := s.db.QueryRowContext(ctx, `PRAGMA `+pragma).Scan(&actual); err != nil {
			return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
		}
		if !strings.EqualFold(actual, expected) {
			return exactModelTurnErrorV2(core.ErrorPreconditionFailed, core.ReasonInvalidState, "model-turn V2 SQLite required pragma is inactive")
		}
	}
	for _, object := range []struct {
		name     string
		kind     string
		expected string
	}{
		{"harness_model_turn_dispatch_schema_v2", "table", sqliteModelTurnDispatchLedgerTableDDLV2},
		{"harness_model_turn_dispatch_v2", "table", sqliteModelTurnDispatchFactTableDDLV2},
		{"harness_model_turn_dispatch_v2_exact", "index", sqliteModelTurnDispatchExactIndexDDLV2},
	} {
		if err := verifySQLiteModelTurnDispatchDDLV2(ctx, s.db, object.name, object.kind, object.expected); err != nil {
			return err
		}
	}
	if err := verifySQLiteModelTurnDispatchColumnsV2(
		ctx,
		s.db,
		"harness_model_turn_dispatch_schema_v2",
		sqliteModelTurnDispatchLedgerColumnsV2,
	); err != nil {
		return err
	}
	if err := verifySQLiteModelTurnDispatchColumnsV2(
		ctx,
		s.db,
		"harness_model_turn_dispatch_v2",
		sqliteModelTurnDispatchFactColumnsV2,
	); err != nil {
		return err
	}
	if err := verifySQLiteModelTurnDispatchIndexesV2(ctx, s.db); err != nil {
		return err
	}
	if err := verifySQLiteModelTurnDispatchNoTriggersV2(ctx, s.db); err != nil {
		return err
	}
	return probeSQLiteModelTurnDispatchConstraintsV2(ctx, s.db)
}

type sqliteModelTurnDispatchQueryerV2 interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func verifySQLiteModelTurnDispatchLedgerRowsV2(
	ctx context.Context,
	queryer sqliteModelTurnDispatchQueryerV2,
	expectedDigest core.Digest,
) error {
	rows, err := queryer.QueryContext(
		ctx,
		`SELECT version,digest,applied_unix_nano FROM harness_model_turn_dispatch_schema_v2 ORDER BY version`,
	)
	if err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	count := 0
	for rows.Next() {
		var version int
		var stored string
		var appliedUnixNano int64
		if err = rows.Scan(&version, &stored, &appliedUnixNano); err != nil {
			_ = rows.Close()
			return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
		}
		count++
		if version != 2 || stored != string(expectedDigest) || appliedUnixNano <= 0 {
			_ = rows.Close()
			return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidDigest, "model-turn V2 SQLite schema ledger drifted")
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	if err = rows.Close(); err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	if count != 1 {
		return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidDigest, "model-turn V2 SQLite schema version set drifted")
	}
	return nil
}

func verifySQLiteModelTurnDispatchDDLV2(
	ctx context.Context,
	db *sql.DB,
	name string,
	kind string,
	expected string,
) error {
	var actualKind, actual string
	if err := db.QueryRowContext(
		ctx,
		`SELECT type,sql FROM sqlite_master WHERE name=?`,
		name,
	).Scan(&actualKind, &actual); err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	if actualKind != kind ||
		normalizeSQLiteModelTurnDispatchDDLV2(actual) != normalizeSQLiteModelTurnDispatchDDLV2(expected) {
		return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite physical DDL drifted")
	}
	return nil
}

func normalizeSQLiteModelTurnDispatchDDLV2(ddl string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsSpace(character) {
			return -1
		}
		return unicode.ToLower(character)
	}, strings.TrimSuffix(strings.TrimSpace(ddl), ";"))
}

func verifySQLiteModelTurnDispatchColumnsV2(
	ctx context.Context,
	db *sql.DB,
	table string,
	expected []sqliteModelTurnDispatchColumnV2,
) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_xinfo('`+table+`')`)
	if err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		var cid, notNull, pk, hidden int
		var name, kind string
		var defaultValue sql.NullString
		if err = rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk, &hidden); err != nil {
			return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
		}
		if index >= len(expected) {
			return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite column set has extras")
		}
		column := expected[index]
		if cid != index || name != column.name || !strings.EqualFold(kind, column.kind) ||
			notNull != column.notNull || defaultValue.Valid || pk != column.pk || hidden != column.hidden {
			return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite column definition drifted")
		}
		index++
	}
	if err = rows.Err(); err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	if err = rows.Close(); err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	if index != len(expected) {
		return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite column set is incomplete")
	}
	return nil
}

type sqliteModelTurnDispatchIndexColumnV2 struct {
	cid  int
	name string
	key  int
}

func verifySQLiteModelTurnDispatchIndexesV2(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA index_list('harness_model_turn_dispatch_v2')`)
	if err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	defer rows.Close()
	indexes := make(map[string]struct {
		unique  int
		origin  string
		partial int
	})
	sequences := make(map[int]struct{})
	for rows.Next() {
		var sequence, unique, partial int
		var name, origin string
		if err = rows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
		}
		if _, exists := indexes[name]; exists {
			return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite index name is duplicated")
		}
		if sequence < 0 {
			return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite index sequence is invalid")
		}
		if _, exists := sequences[sequence]; exists {
			return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite index sequence is duplicated")
		}
		sequences[sequence] = struct{}{}
		indexes[name] = struct {
			unique  int
			origin  string
			partial int
		}{unique: unique, origin: origin, partial: partial}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	if err = rows.Close(); err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	if len(indexes) != 2 {
		return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite index set drifted")
	}
	for sequence := 0; sequence < len(indexes); sequence++ {
		if _, exists := sequences[sequence]; !exists {
			return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite index sequence is incomplete")
		}
	}
	exact, exactExists := indexes["harness_model_turn_dispatch_v2_exact"]
	primary, primaryExists := indexes["sqlite_autoindex_harness_model_turn_dispatch_v2_1"]
	if !exactExists || exact.unique != 0 || exact.origin != "c" || exact.partial != 0 ||
		!primaryExists || primary.unique != 1 || primary.origin != "pk" || primary.partial != 0 {
		return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite index metadata drifted")
	}
	ledgerIndexes, err := db.QueryContext(ctx, `PRAGMA index_list('harness_model_turn_dispatch_schema_v2')`)
	if err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	if ledgerIndexes.Next() {
		_ = ledgerIndexes.Close()
		return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite ledger index set drifted")
	}
	if err = ledgerIndexes.Err(); err != nil {
		_ = ledgerIndexes.Close()
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	if err = ledgerIndexes.Close(); err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	if err := verifySQLiteModelTurnDispatchIndexColumnsV2(
		ctx,
		db,
		"harness_model_turn_dispatch_v2_exact",
		[]sqliteModelTurnDispatchIndexColumnV2{
			{cid: 0, name: "dispatch_id", key: 1},
			{cid: 1, name: "dispatch_ref_digest", key: 1},
			{cid: 2, name: "ack_ref_digest", key: 1},
			{cid: 3, name: "fact_digest", key: 1},
			{cid: 4, name: "revision", key: 1},
			{cid: 5, name: "state", key: 1},
			{cid: 6, name: "not_after_unix_nano", key: 1},
			{cid: -1, key: 0},
		},
	); err != nil {
		return err
	}
	return verifySQLiteModelTurnDispatchIndexColumnsV2(
		ctx,
		db,
		"sqlite_autoindex_harness_model_turn_dispatch_v2_1",
		[]sqliteModelTurnDispatchIndexColumnV2{
			{cid: 0, name: "dispatch_id", key: 1},
			{cid: -1, key: 0},
		},
	)
}

func verifySQLiteModelTurnDispatchNoTriggersV2(ctx context.Context, db *sql.DB) error {
	var triggers int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master
		 WHERE type='trigger'
		   AND tbl_name IN ('harness_model_turn_dispatch_schema_v2','harness_model_turn_dispatch_v2')`,
	).Scan(&triggers); err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	if triggers != 0 {
		return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite trigger set drifted")
	}
	return nil
}

func verifySQLiteModelTurnDispatchIndexColumnsV2(
	ctx context.Context,
	db *sql.DB,
	indexName string,
	expected []sqliteModelTurnDispatchIndexColumnV2,
) error {
	rows, err := db.QueryContext(ctx, `PRAGMA index_xinfo('`+indexName+`')`)
	if err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	defer rows.Close()
	offset := 0
	for rows.Next() {
		var sequence, cid, descending, key int
		var name sql.NullString
		var collation string
		if err = rows.Scan(&sequence, &cid, &name, &descending, &collation, &key); err != nil {
			return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
		}
		if offset >= len(expected) {
			return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite index column set has extras")
		}
		column := expected[offset]
		nameDrifted := column.key == 1 &&
			(!name.Valid || name.String != column.name)
		if column.key == 0 {
			nameDrifted = name.Valid || column.name != "" || column.cid != -1
		}
		if sequence != offset || cid != column.cid || nameDrifted ||
			descending != 0 || collation != "BINARY" || key != column.key {
			return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite index column drifted")
		}
		offset++
	}
	if err = rows.Err(); err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	if err = rows.Close(); err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	if offset != len(expected) {
		return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite index column set is incomplete")
	}
	return nil
}

func probeSQLiteModelTurnDispatchConstraintsV2(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	const probeID = "__praxis_harness_model_turn_dispatch_schema_probe_v2__"
	if _, err = tx.ExecContext(
		ctx,
		`DELETE FROM harness_model_turn_dispatch_v2 WHERE dispatch_id IN (?,?,?)`,
		probeID,
		probeID+"/revision",
		probeID+"/not-after",
	); err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, true)
	}
	insert := `INSERT INTO harness_model_turn_dispatch_v2(dispatch_id,dispatch_ref_digest,ack_ref_digest,fact_digest,revision,state,not_after_unix_nano,canonical_json) VALUES(?,?,?,?,?,?,?,?)`
	if _, err = tx.ExecContext(ctx, insert, probeID, "ref", "ack", "fact", 1, "attempt_bound", 1, []byte(`{}`)); err != nil {
		return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite positive constraint probe failed")
	}
	if _, err = tx.ExecContext(ctx, insert, probeID, "ref", "ack", "fact", 1, "attempt_bound", 1, []byte(`{}`)); err == nil {
		return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite primary-key constraint is inactive")
	}
	if _, err = tx.ExecContext(ctx, insert, probeID+"/revision", "ref", "ack", "fact", 0, "attempt_bound", 1, []byte(`{}`)); err == nil {
		return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite revision constraint is inactive")
	}
	if _, err = tx.ExecContext(ctx, insert, probeID+"/not-after", "ref", "ack", "fact", 1, "attempt_bound", 0, []byte(`{}`)); err == nil {
		return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite not-after constraint is inactive")
	}
	if _, err = tx.ExecContext(
		ctx,
		`INSERT INTO harness_model_turn_dispatch_schema_v2(version,digest,applied_unix_nano) VALUES(9223372036854775807,'probe',0)`,
	); err == nil {
		return exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite ledger timestamp constraint is inactive")
	}
	if err = tx.Rollback(); err != nil {
		return mapSQLiteModelTurnDispatchErrorV2(ctx, err, true)
	}
	return nil
}

func (s *SQLiteModelTurnDispatchV2) consumeLostReplyV2() bool {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	if !s.loseNextReply {
		return false
	}
	s.loseNextReply = false
	return true
}

func inspectSQLiteModelTurnDispatchTxV2(
	ctx context.Context,
	tx *sql.Tx,
	id string,
) (bridgecontract.ModelTurnDispatchFactV2, bool, error) {
	var refDigest, ackDigest, factDigest, state string
	var revision uint64
	var notAfter int64
	var payload []byte
	err := tx.QueryRowContext(
		ctx,
		`SELECT dispatch_ref_digest,ack_ref_digest,fact_digest,revision,state,not_after_unix_nano,canonical_json FROM harness_model_turn_dispatch_v2 WHERE dispatch_id=?`,
		id,
	).Scan(&refDigest, &ackDigest, &factDigest, &revision, &state, &notAfter, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return bridgecontract.ModelTurnDispatchFactV2{}, false, nil
	}
	if err != nil {
		return bridgecontract.ModelTurnDispatchFactV2{}, false, mapSQLiteModelTurnDispatchErrorV2(ctx, err, false)
	}
	fact, err := bridgecontract.DecodeModelTurnDispatchFactV2(payload)
	if err != nil ||
		fact.Ref.ID != id ||
		string(fact.Ref.Digest) != refDigest ||
		string(fact.Ref.Envelope.AckRef.Digest) != ackDigest ||
		string(fact.Digest) != factDigest ||
		uint64(fact.Revision) != revision ||
		string(fact.State) != state ||
		fact.NotAfterUnixNano != notAfter {
		return bridgecontract.ModelTurnDispatchFactV2{}, false, exactModelTurnErrorV2(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "model-turn V2 SQLite row failed canonical revalidation")
	}
	return fact.CloneV2(), true, nil
}

func mapSQLiteModelTurnDispatchErrorV2(ctx context.Context, err error, mutating bool) error {
	if ctx == nil || ctx.Err() != nil {
		return exactModelTurnErrorV2(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "model-turn V2 SQLite context ended")
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "locked") || strings.Contains(lower, "busy") {
		if mutating {
			return exactModelTurnErrorV2(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "model-turn V2 SQLite mutation outcome is unknown")
		}
		return exactModelTurnErrorV2(core.ErrorUnavailable, core.ReasonInvalidState, "model-turn V2 SQLite is busy")
	}
	return exactModelTurnErrorV2(core.ErrorUnavailable, core.ReasonComponentMissing, "model-turn V2 SQLite operation failed")
}

var _ harnessports.ModelTurnDispatchRepositoryV2 = (*SQLiteModelTurnDispatchV2)(nil)
