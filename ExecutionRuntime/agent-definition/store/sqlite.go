// Package store contains Definition-owner repositories.
package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	_ "modernc.org/sqlite"
)

const definitionSQLiteSchemaV1 = `
CREATE TABLE IF NOT EXISTS definition_schema_v1(version INTEGER PRIMARY KEY,digest TEXT NOT NULL,applied_unix_nano INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS definition_history_v1(definition_id TEXT NOT NULL,revision INTEGER NOT NULL,digest TEXT NOT NULL,row_digest TEXT NOT NULL,payload_json BLOB NOT NULL,created_unix_nano INTEGER NOT NULL,PRIMARY KEY(definition_id,revision));
CREATE UNIQUE INDEX IF NOT EXISTS definition_history_exact_v1 ON definition_history_v1(definition_id,revision,digest);
CREATE TABLE IF NOT EXISTS definition_current_v1(definition_id TEXT PRIMARY KEY,definition_revision INTEGER NOT NULL,definition_digest TEXT NOT NULL,current_revision INTEGER NOT NULL,state TEXT NOT NULL,updated_unix_nano INTEGER NOT NULL,highest_checked_unix_nano INTEGER NOT NULL,reason TEXT NOT NULL,row_digest TEXT NOT NULL,payload_json BLOB NOT NULL,FOREIGN KEY(definition_id,definition_revision,definition_digest) REFERENCES definition_history_v1(definition_id,revision,digest));`

type SQLiteConfigV1 struct {
	Path         string
	Catalog      contract.ValidationCatalogV1
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}
type SQLiteRepositoryV1 struct {
	db         *sql.DB
	catalog    contract.ValidationCatalogV1
	clock      func() time.Time
	mu         *sync.Mutex
	faultMu    sync.Mutex
	loseCreate bool
}

var _ ports.DefinitionRepositoryV1 = (*SQLiteRepositoryV1)(nil)
var definitionSQLiteLocksV1 sync.Map

func OpenSQLiteRepositoryV1(ctx context.Context, c SQLiteConfigV1) (*SQLiteRepositoryV1, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, invalid("definition SQLite open requires a live context")
	}
	if strings.TrimSpace(c.Path) == "" {
		return nil, invalid("definition SQLite path is required")
	}
	if c.BusyTimeout <= 0 {
		c.BusyTimeout = 5 * time.Second
	}
	if c.BusyTimeout > time.Minute {
		return nil, invalid("definition SQLite busy timeout exceeds one minute")
	}
	if c.MaxOpenConns <= 0 {
		c.MaxOpenConns = 4
	}
	if c.MaxOpenConns > 32 {
		return nil, invalid("definition SQLite connection count exceeds 32")
	}
	if c.Clock == nil {
		c.Clock = time.Now
	}
	abs, err := filepath.Abs(c.Path)
	if err != nil {
		return nil, invalid("definition SQLite path is invalid")
	}
	lock, _ := definitionSQLiteLocksV1.LoadOrStore(abs, &sync.Mutex{})
	dsn := "file:" + filepath.ToSlash(abs) + fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_pragma=synchronous(FULL)&_txlock=immediate", c.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, definitionDBErr(ctx, err, false)
	}
	db.SetMaxOpenConns(c.MaxOpenConns)
	db.SetMaxIdleConns(c.MaxOpenConns)
	s := &SQLiteRepositoryV1{db: db, catalog: contract.CloneValidationCatalogV1(c.Catalog), clock: c.Clock, mu: lock.(*sync.Mutex)}
	if err = s.migrate(ctx); err == nil {
		err = s.verify(ctx)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}
func (s *SQLiteRepositoryV1) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
func (s *SQLiteRepositoryV1) LoseNextCreateReply() {
	if s == nil {
		return
	}
	s.faultMu.Lock()
	s.loseCreate = true
	s.faultMu.Unlock()
}
func (s *SQLiteRepositoryV1) takeLoseCreate() bool {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	v := s.loseCreate
	s.loseCreate = false
	return v
}

func (s *SQLiteRepositoryV1) migrate(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return definitionDBErr(ctx, err, true)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, definitionSQLiteSchemaV1); err != nil {
		return definitionDBErr(ctx, err, true)
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "definition SQLite migration clock is invalid")
	}
	digest := core.DigestBytes([]byte(definitionSQLiteSchemaV1))
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO definition_schema_v1(version,digest,applied_unix_nano) VALUES(1,?,?)`, string(digest), now.UnixNano()); err != nil {
		return definitionDBErr(ctx, err, true)
	}
	var stored string
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT digest FROM definition_schema_v1 WHERE version=1`).Scan(&stored); err != nil {
		return definitionDBErr(ctx, err, false)
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM definition_schema_v1`).Scan(&count); err != nil {
		return definitionDBErr(ctx, err, false)
	}
	if stored != string(digest) || count != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "definition SQLite schema digest drifted")
	}
	if err = tx.Commit(); err != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "definition SQLite migration commit outcome is unknown")
	}
	return nil
}
func (s *SQLiteRepositoryV1) verify(ctx context.Context) error {
	var wal string
	var fk int
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&wal); err != nil {
		return definitionDBErr(ctx, err, false)
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
		return definitionDBErr(ctx, err, false)
	}
	if !strings.EqualFold(wal, "wal") || fk != 1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "definition SQLite WAL or foreign keys inactive")
	}
	return nil
}
func (s *SQLiteRepositoryV1) IntegrityCheckV1(ctx context.Context) error {
	if s == nil || s.db == nil {
		return invalid("definition SQLite repository is nil")
	}
	var v string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&v); err != nil {
		return definitionDBErr(ctx, err, false)
	}
	if v != "ok" {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "definition SQLite integrity check failed")
	}
	return nil
}

type definitionSQLiteCurrentV1 struct {
	DefinitionID           string                            `json:"definition_id"`
	DefinitionRevision     core.Revision                     `json:"definition_revision"`
	DefinitionDigest       core.Digest                       `json:"definition_digest"`
	CurrentRevision        core.Revision                     `json:"current_revision"`
	State                  contract.DefinitionCurrentStateV1 `json:"state"`
	UpdatedUnixNano        int64                             `json:"updated_unix_nano"`
	HighestCheckedUnixNano int64                             `json:"highest_checked_unix_nano"`
	Reason                 string                            `json:"reason"`
}

func definitionStrictJSONV1[T any](raw []byte) (T, error) {
	var v T
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return v, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "definition SQLite strict decode failed")
	}
	var tail any
	if err := d.Decode(&tail); !errors.Is(err, io.EOF) {
		return v, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "definition SQLite trailing JSON rejected")
	}
	return v, nil
}
func definitionRowDigestV1(kind, id string, revision core.Revision, digest core.Digest, payload []byte) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.agent-definition.sqlite", "v1", "DefinitionSQLiteRowV1", struct {
		Kind          string        `json:"kind"`
		ID            string        `json:"id"`
		Revision      core.Revision `json:"revision"`
		Digest        core.Digest   `json:"digest"`
		PayloadDigest core.Digest   `json:"payload_digest"`
	}{kind, id, revision, digest, core.DigestBytes(payload)})
}
func encodeDefinitionV1(v any, kind, id string, rev core.Revision, digest core.Digest) ([]byte, core.Digest, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, "", core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "definition SQLite encode failed")
	}
	rd, err := definitionRowDigestV1(kind, id, rev, digest, raw)
	return raw, rd, err
}

func (s *SQLiteRepositoryV1) loadTx(ctx context.Context, tx *sql.Tx, id string) (*MemoryRepositoryV1, error) {
	m := NewMemoryRepositoryV1(s.catalog)
	rows, err := tx.QueryContext(ctx, `SELECT revision,digest,row_digest,payload_json,created_unix_nano FROM definition_history_v1 WHERE definition_id=? ORDER BY revision`, id)
	if err != nil {
		return nil, definitionDBErr(ctx, err, false)
	}
	defer rows.Close()
	for rows.Next() {
		var rev uint64
		var digest, rowDigest string
		var raw []byte
		var created int64
		if err = rows.Scan(&rev, &digest, &rowDigest, &raw, &created); err != nil {
			return nil, definitionDBErr(ctx, err, false)
		}
		expected, e := definitionRowDigestV1("history", id, core.Revision(rev), core.Digest(digest), raw)
		if e != nil || expected != core.Digest(rowDigest) {
			return nil, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "definition SQLite history row digest drifted")
		}
		v, e := definitionStrictJSONV1[contract.AgentDefinitionV1](raw)
		if e != nil || v.DefinitionID != id || v.Revision != core.Revision(rev) || v.Digest != core.Digest(digest) || v.CreatedUnixNano != created || v.Validate(s.catalog) != nil {
			return nil, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "definition SQLite history payload drifted")
		}
		if m.history[id] == nil {
			m.history[id] = map[core.Revision]contract.AgentDefinitionV1{}
		}
		m.history[id][v.Revision] = v
	}
	if err = rows.Err(); err != nil {
		return nil, definitionDBErr(ctx, err, false)
	}
	var defRev, curRev uint64
	var defDigest, state string
	var updated, highest int64
	var reason, rowDigest string
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT definition_revision,definition_digest,current_revision,state,updated_unix_nano,highest_checked_unix_nano,reason,row_digest,payload_json FROM definition_current_v1 WHERE definition_id=?`, id).Scan(&defRev, &defDigest, &curRev, &state, &updated, &highest, &reason, &rowDigest, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return m, nil
	}
	if err != nil {
		return nil, definitionDBErr(ctx, err, false)
	}
	payload, e := definitionStrictJSONV1[definitionSQLiteCurrentV1](raw)
	expected, e2 := definitionRowDigestV1("current", id, core.Revision(curRev), core.Digest(defDigest), raw)
	if e != nil || e2 != nil || expected != core.Digest(rowDigest) || payload.DefinitionID != id || payload.DefinitionRevision != core.Revision(defRev) || payload.DefinitionDigest != core.Digest(defDigest) || payload.CurrentRevision != core.Revision(curRev) || string(payload.State) != state || payload.UpdatedUnixNano != updated || payload.HighestCheckedUnixNano != highest || payload.Reason != reason {
		return nil, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "definition SQLite current row drifted")
	}
	if d, ok := m.history[id][payload.DefinitionRevision]; !ok || d.Digest != payload.DefinitionDigest {
		return nil, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "definition SQLite current does not bind history")
	}
	m.current[id] = currentFactV1{ref: contract.AgentDefinitionRefV1{DefinitionID: id, Revision: payload.DefinitionRevision, Digest: payload.DefinitionDigest}, state: payload.State, revision: payload.CurrentRevision, updatedUnixNano: payload.UpdatedUnixNano, highestCheckedUnixNano: payload.HighestCheckedUnixNano, reason: payload.Reason}
	return m, nil
}

func (s *SQLiteRepositoryV1) saveCurrentTx(ctx context.Context, tx *sql.Tx, id string, f currentFactV1) error {
	v := definitionSQLiteCurrentV1{id, f.ref.Revision, f.ref.Digest, f.revision, f.state, f.updatedUnixNano, f.highestCheckedUnixNano, f.reason}
	raw, rd, err := encodeDefinitionV1(v, "current", id, f.revision, f.ref.Digest)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO definition_current_v1(definition_id,definition_revision,definition_digest,current_revision,state,updated_unix_nano,highest_checked_unix_nano,reason,row_digest,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(definition_id) DO UPDATE SET definition_revision=excluded.definition_revision,definition_digest=excluded.definition_digest,current_revision=excluded.current_revision,state=excluded.state,updated_unix_nano=excluded.updated_unix_nano,highest_checked_unix_nano=excluded.highest_checked_unix_nano,reason=excluded.reason,row_digest=excluded.row_digest,payload_json=excluded.payload_json`, id, uint64(f.ref.Revision), string(f.ref.Digest), uint64(f.revision), string(f.state), f.updatedUnixNano, f.highestCheckedUnixNano, f.reason, string(rd), raw)
	return definitionDBErr(ctx, err, true)
}

func (s *SQLiteRepositoryV1) CreateDefinitionV1(ctx context.Context, q ports.CreateDefinitionRequestV1) (ports.CreateDefinitionResultV1, error) {
	if s == nil || s.db == nil {
		return ports.CreateDefinitionResultV1{}, invalid("definition SQLite repository is nil")
	}
	if ctx == nil || ctx.Err() != nil {
		return ports.CreateDefinitionResultV1{}, unavailable("definition SQLite create context ended")
	}
	if err := q.Definition.Validate(s.catalog); err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ports.CreateDefinitionResultV1{}, definitionDBErr(ctx, err, true)
	}
	defer tx.Rollback()
	m, err := s.loadTx(ctx, tx, q.Definition.DefinitionID)
	if err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	_, existed := m.history[q.Definition.DefinitionID][q.Definition.Revision]
	result, err := m.CreateDefinitionV1(ctx, q)
	if err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	if !existed {
		raw, rd, e := encodeDefinitionV1(q.Definition, "history", q.Definition.DefinitionID, q.Definition.Revision, q.Definition.Digest)
		if e != nil {
			return ports.CreateDefinitionResultV1{}, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO definition_history_v1(definition_id,revision,digest,row_digest,payload_json,created_unix_nano) VALUES(?,?,?,?,?,?)`, q.Definition.DefinitionID, uint64(q.Definition.Revision), string(q.Definition.Digest), string(rd), raw, q.Definition.CreatedUnixNano); e != nil {
			return ports.CreateDefinitionResultV1{}, definitionDBErr(ctx, e, true)
		}
	}
	if err = s.saveCurrentTx(ctx, tx, q.Definition.DefinitionID, m.current[q.Definition.DefinitionID]); err != nil {
		return ports.CreateDefinitionResultV1{}, err
	}
	if err = tx.Commit(); err != nil {
		return ports.CreateDefinitionResultV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "definition SQLite create commit outcome unknown")
	}
	if s.takeLoseCreate() {
		return ports.CreateDefinitionResultV1{}, unavailable("definition SQLite create reply lost after commit")
	}
	return result, nil
}

func (s *SQLiteRepositoryV1) InspectDefinitionRevisionV1(ctx context.Context, id string, rev core.Revision) (contract.AgentDefinitionV1, error) {
	if s == nil || s.db == nil {
		return contract.AgentDefinitionV1{}, invalid("definition SQLite repository is nil")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.AgentDefinitionV1{}, unavailable("definition SQLite inspect context ended")
	}
	if strings.TrimSpace(id) == "" || rev == 0 {
		return contract.AgentDefinitionV1{}, invalid("definition revision request is incomplete")
	}
	var digest, rowDigest string
	var raw []byte
	var created int64
	err := s.db.QueryRowContext(ctx, `SELECT digest,row_digest,payload_json,created_unix_nano FROM definition_history_v1 WHERE definition_id=? AND revision=?`, id, uint64(rev)).Scan(&digest, &rowDigest, &raw, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.AgentDefinitionV1{}, notFound("definition revision was not found")
	}
	if err != nil {
		return contract.AgentDefinitionV1{}, definitionDBErr(ctx, err, false)
	}
	expected, e := definitionRowDigestV1("history", id, rev, core.Digest(digest), raw)
	v, e2 := definitionStrictJSONV1[contract.AgentDefinitionV1](raw)
	if e != nil || e2 != nil || expected != core.Digest(rowDigest) || v.DefinitionID != id || v.Revision != rev || v.Digest != core.Digest(digest) || v.CreatedUnixNano != created || v.Validate(s.catalog) != nil {
		return contract.AgentDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "definition SQLite history row drifted")
	}
	return contract.CloneDefinitionV1(v), nil
}
func (s *SQLiteRepositoryV1) InspectExactDefinitionV1(ctx context.Context, ref contract.AgentDefinitionRefV1) (contract.AgentDefinitionV1, error) {
	if s == nil || s.db == nil {
		return contract.AgentDefinitionV1{}, invalid("definition SQLite repository is nil")
	}
	if err := ref.Validate(); err != nil {
		return contract.AgentDefinitionV1{}, err
	}
	v, err := s.InspectDefinitionRevisionV1(ctx, ref.DefinitionID, ref.Revision)
	if err != nil {
		return v, err
	}
	if v.Digest != ref.Digest {
		return contract.AgentDefinitionV1{}, conflict("definition exact ref digest drifted")
	}
	return v, nil
}
func (s *SQLiteRepositoryV1) InspectCurrentDefinitionV1(ctx context.Context, id string, checked int64) (contract.DefinitionCurrentV1, error) {
	if s == nil || s.db == nil {
		return contract.DefinitionCurrentV1{}, invalid("definition SQLite repository is nil")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.DefinitionCurrentV1{}, unavailable("definition SQLite inspect context ended")
	}
	if strings.TrimSpace(id) == "" || checked <= 0 {
		return contract.DefinitionCurrentV1{}, invalid("current definition request is incomplete")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return contract.DefinitionCurrentV1{}, definitionDBErr(ctx, err, true)
	}
	defer tx.Rollback()
	m, err := s.loadTx(ctx, tx, id)
	if err != nil {
		return contract.DefinitionCurrentV1{}, err
	}
	v, err := m.InspectCurrentDefinitionV1(ctx, id, checked)
	if err != nil {
		return contract.DefinitionCurrentV1{}, err
	}
	if err = s.saveCurrentTx(ctx, tx, id, m.current[id]); err != nil {
		return contract.DefinitionCurrentV1{}, err
	}
	if err = tx.Commit(); err != nil {
		return contract.DefinitionCurrentV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "definition SQLite checked watermark commit unknown")
	}
	return v, nil
}
func (s *SQLiteRepositoryV1) RevokeDefinitionV1(ctx context.Context, q ports.RevokeDefinitionRequestV1) (contract.DefinitionCurrentV1, error) {
	if s == nil || s.db == nil {
		return contract.DefinitionCurrentV1{}, invalid("definition SQLite repository is nil")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.DefinitionCurrentV1{}, unavailable("definition SQLite revoke context ended")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return contract.DefinitionCurrentV1{}, definitionDBErr(ctx, err, true)
	}
	defer tx.Rollback()
	m, err := s.loadTx(ctx, tx, q.DefinitionID)
	if err != nil {
		return contract.DefinitionCurrentV1{}, err
	}
	v, err := m.RevokeDefinitionV1(ctx, q)
	if err != nil {
		return contract.DefinitionCurrentV1{}, err
	}
	if err = s.saveCurrentTx(ctx, tx, q.DefinitionID, m.current[q.DefinitionID]); err != nil {
		return contract.DefinitionCurrentV1{}, err
	}
	if err = tx.Commit(); err != nil {
		return contract.DefinitionCurrentV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "definition SQLite revoke commit outcome unknown")
	}
	return v, nil
}

func definitionDBErr(ctx context.Context, err error, mutation bool) error {
	if err == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if mutation {
			return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "definition SQLite mutation outcome unknown")
		}
		return unavailable("definition SQLite read unavailable")
	}
	m := strings.ToLower(err.Error())
	if strings.Contains(m, "locked") || strings.Contains(m, "busy") {
		return unavailable("definition SQLite busy")
	}
	if strings.Contains(m, "constraint") || strings.Contains(m, "unique") {
		return conflict("definition SQLite constraint conflict")
	}
	if mutation {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "definition SQLite mutation outcome unknown")
	}
	return unavailable("definition SQLite read failed")
}
