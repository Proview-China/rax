// Package sqlite provides the single-node durable Application State Plane.
// WAL supplies local crash durability only; no HA, remote durability or SLA is claimed.
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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	_ "modernc.org/sqlite"
)

const (
	schemaVersionV1    = 1
	lifecycleKindV1    = "agent_lifecycle"
	coordinationKindV1 = "agent_activation_coordination"
	schemaV1           = `
CREATE TABLE IF NOT EXISTS application_state_schema_v1(version INTEGER PRIMARY KEY,digest TEXT NOT NULL,applied_unix_nano INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS application_fact_history_v1(fact_type TEXT NOT NULL,fact_id TEXT NOT NULL,revision INTEGER NOT NULL,digest TEXT NOT NULL,previous_digest TEXT NOT NULL,row_digest TEXT NOT NULL,payload_json BLOB NOT NULL,checked_unix_nano INTEGER NOT NULL,expires_unix_nano INTEGER NOT NULL,PRIMARY KEY(fact_type,fact_id,revision));
CREATE UNIQUE INDEX IF NOT EXISTS application_fact_exact_v1 ON application_fact_history_v1(fact_type,fact_id,revision,digest);
CREATE TABLE IF NOT EXISTS application_fact_current_v1(fact_type TEXT NOT NULL,fact_id TEXT NOT NULL,revision INTEGER NOT NULL,digest TEXT NOT NULL,row_digest TEXT NOT NULL,PRIMARY KEY(fact_type,fact_id),FOREIGN KEY(fact_type,fact_id,revision,digest) REFERENCES application_fact_history_v1(fact_type,fact_id,revision,digest));`
)

type ConfigV1 struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}

type StoreV1 struct {
	db    *sql.DB
	clock func() time.Time
	mu    *sync.Mutex

	faultMu                                            sync.Mutex
	loseLifecycleEnsure, loseLifecycleCAS              bool
	loseCoordinationEnsure, loseCoordinationCAS        bool
	loseRestoreStageEnsure, loseRestoreExecutionEnsure bool
	failCoordinationCASBeforeCommit                    core.ErrorCategory
}

var (
	_                       applicationports.AgentLifecycleFactPortV1              = (*StoreV1)(nil)
	_                       applicationports.AgentActivationCoordinationFactPortV1 = (*StoreV1)(nil)
	applicationStateLocksV1 sync.Map
)

func OpenV1(ctx context.Context, config ConfigV1) (*StoreV1, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, invalid("Application SQLite open requires a live context")
	}
	if strings.TrimSpace(config.Path) == "" {
		return nil, invalid("Application SQLite path is required")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, invalid("Application SQLite busy timeout exceeds one minute")
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 4
	}
	if config.MaxOpenConns > 32 {
		return nil, invalid("Application SQLite connection count exceeds 32")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	abs, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, invalid("Application SQLite path is invalid")
	}
	lock, _ := applicationStateLocksV1.LoadOrStore(abs, &sync.Mutex{})
	dsn := (&url.URL{Scheme: "file", Path: abs}).String() + fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_pragma=synchronous(FULL)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapDBError(ctx, err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	s := &StoreV1{db: db, clock: config.Clock, mu: lock.(*sync.Mutex)}
	if err = s.migrate(ctx); err == nil {
		err = s.verify(ctx)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *StoreV1) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *StoreV1) migrate(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, schemaV1); err != nil {
		return mapDBError(ctx, err, true)
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Application SQLite migration clock is invalid")
	}
	digest := core.DigestBytes([]byte(schemaV1))
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO application_state_schema_v1(version,digest,applied_unix_nano) VALUES(?,?,?)`, schemaVersionV1, string(digest), now.UnixNano()); err != nil {
		return mapDBError(ctx, err, true)
	}
	var stored string
	if err = tx.QueryRowContext(ctx, `SELECT digest FROM application_state_schema_v1 WHERE version=?`, schemaVersionV1).Scan(&stored); err != nil {
		return mapDBError(ctx, err, false)
	}
	if stored != string(digest) {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Application SQLite schema digest drifted")
	}
	var schemaRows int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM application_state_schema_v1`).Scan(&schemaRows); err != nil {
		return mapDBError(ctx, err, false)
	}
	if schemaRows != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Application SQLite schema version set drifted")
	}
	if err = tx.Commit(); err != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Application SQLite migration commit outcome is unknown")
	}
	return nil
}

func (s *StoreV1) verify(ctx context.Context) error {
	var journal string
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		return mapDBError(ctx, err, false)
	}
	if !strings.EqualFold(journal, "wal") {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Application SQLite WAL mode is inactive")
	}
	var fk int
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
		return mapDBError(ctx, err, false)
	}
	if fk != 1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Application SQLite foreign keys are inactive")
	}
	return nil
}

func (s *StoreV1) IntegrityCheckV1(ctx context.Context) error {
	if err := s.readReady(ctx); err != nil {
		return err
	}
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapDBError(ctx, err, false)
	}
	if result != "ok" {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "Application SQLite integrity check failed")
	}
	return nil
}

type rowIdentityV1 struct {
	FactType        string        `json:"fact_type"`
	FactID          string        `json:"fact_id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	PreviousDigest  core.Digest   `json:"previous_digest,omitempty"`
	PayloadDigest   core.Digest   `json:"payload_digest"`
	CheckedUnixNano int64         `json:"checked_unix_nano"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

func encodeRowV1(kind, id string, revision core.Revision, digest, previous core.Digest, checked, expires int64, value any) ([]byte, core.Digest, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, "", core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "Application SQLite encode failed")
	}
	rowDigest, err := core.CanonicalJSONDigest("praxis.application.state-plane", "v1", "ApplicationFactRowV1", rowIdentityV1{kind, id, revision, digest, previous, core.DigestBytes(payload), checked, expires})
	return payload, rowDigest, err
}

func strictDecodeV1[T any](payload []byte) (T, error) {
	var value T
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, corrupt("Application SQLite payload decode failed")
	}
	var tail any
	if err := decoder.Decode(&tail); !errors.Is(err, io.EOF) {
		return value, corrupt("Application SQLite payload has trailing content")
	}
	return value, nil
}

type storedRowV1 struct {
	revision                    core.Revision
	digest, previous, rowDigest core.Digest
	payload                     []byte
	checked, expires            int64
}

func (s *StoreV1) readCurrent(ctx context.Context, kind, id string) (storedRowV1, error) {
	if err := s.readReady(ctx); err != nil {
		return storedRowV1{}, err
	}
	var r storedRowV1
	var revision uint64
	var digest, previous, rowDigest, currentRowDigest string
	err := s.db.QueryRowContext(ctx, `SELECT h.revision,h.digest,h.previous_digest,h.row_digest,c.row_digest,h.payload_json,h.checked_unix_nano,h.expires_unix_nano FROM application_fact_current_v1 c JOIN application_fact_history_v1 h ON h.fact_type=c.fact_type AND h.fact_id=c.fact_id AND h.revision=c.revision AND h.digest=c.digest WHERE c.fact_type=? AND c.fact_id=?`, kind, id).Scan(&revision, &digest, &previous, &rowDigest, &currentRowDigest, &r.payload, &r.checked, &r.expires)
	if errors.Is(err, sql.ErrNoRows) {
		return r, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Application SQLite Fact is absent")
	}
	if err != nil {
		return r, mapDBError(ctx, err, false)
	}
	r.revision, r.digest, r.previous, r.rowDigest = core.Revision(revision), core.Digest(digest), core.Digest(previous), core.Digest(rowDigest)
	if core.Digest(currentRowDigest) != r.rowDigest || validateStoredRowV1(kind, id, r) != nil {
		return storedRowV1{}, corrupt("Application SQLite row digest drifted")
	}
	return r, nil
}

func validateStoredRowV1(kind, id string, r storedRowV1) error {
	expected, err := core.CanonicalJSONDigest("praxis.application.state-plane", "v1", "ApplicationFactRowV1", rowIdentityV1{kind, id, r.revision, r.digest, r.previous, core.DigestBytes(r.payload), r.checked, r.expires})
	if err != nil || expected != r.rowDigest {
		return corrupt("Application SQLite row digest drifted")
	}
	return nil
}

func (s *StoreV1) ensure(ctx context.Context, kind, id string, revision core.Revision, digest, previous core.Digest, checked, expires int64, value any) ([]byte, error) {
	if err := s.writeReady(ctx); err != nil {
		return nil, err
	}
	if revision == 0 || previous != "" {
		return nil, conflict("Application SQLite first Fact coordinates are invalid")
	}
	payload, rowDigest, err := encodeRowV1(kind, id, revision, digest, previous, checked, expires, value)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, mapDBError(ctx, err, true)
	}
	defer tx.Rollback()
	var curRev uint64
	var curDigest, curPrevious, curRowDigest, currentRowDigest string
	var committed []byte
	var curChecked, curExpires int64
	err = tx.QueryRowContext(ctx, `SELECT c.revision,c.digest,h.previous_digest,h.row_digest,c.row_digest,h.payload_json,h.checked_unix_nano,h.expires_unix_nano FROM application_fact_current_v1 c JOIN application_fact_history_v1 h ON h.fact_type=c.fact_type AND h.fact_id=c.fact_id AND h.revision=c.revision AND h.digest=c.digest WHERE c.fact_type=? AND c.fact_id=?`, kind, id).Scan(&curRev, &curDigest, &curPrevious, &curRowDigest, &currentRowDigest, &committed, &curChecked, &curExpires)
	if err == nil {
		if core.Revision(curRev) != revision || core.Digest(curDigest) != digest {
			return nil, conflict("Application SQLite Fact identity already binds different content")
		}
		r := storedRowV1{revision: core.Revision(curRev), digest: core.Digest(curDigest), previous: core.Digest(curPrevious), rowDigest: core.Digest(curRowDigest), payload: committed, checked: curChecked, expires: curExpires}
		if currentRowDigest != curRowDigest || validateStoredRowV1(kind, id, r) != nil {
			return nil, corrupt("Application SQLite existing row digest drifted")
		}
		return committed, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, mapDBError(ctx, err, true)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO application_fact_history_v1(fact_type,fact_id,revision,digest,previous_digest,row_digest,payload_json,checked_unix_nano,expires_unix_nano) VALUES(?,?,?,?,?,?,?,?,?)`, kind, id, uint64(revision), string(digest), string(previous), string(rowDigest), payload, checked, expires); err != nil {
		return nil, mapDBError(ctx, err, true)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO application_fact_current_v1(fact_type,fact_id,revision,digest,row_digest) VALUES(?,?,?,?,?)`, kind, id, uint64(revision), string(digest), string(rowDigest)); err != nil {
		return nil, mapDBError(ctx, err, true)
	}
	if err = tx.Commit(); err != nil {
		return nil, mapDBError(ctx, err, true)
	}
	if s.takeFault(kind, false) {
		return nil, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Application SQLite Ensure committed but its reply was lost")
	}
	return payload, nil
}

func (s *StoreV1) cas(ctx context.Context, kind, id string, expectedRevision core.Revision, expectedDigest core.Digest, nextRevision core.Revision, nextDigest, previous core.Digest, checked, expires int64, value any, validate func([]byte) error) ([]byte, error) {
	if err := s.writeReady(ctx); err != nil {
		return nil, err
	}
	payload, rowDigest, err := encodeRowV1(kind, id, nextRevision, nextDigest, previous, checked, expires, value)
	if err != nil {
		return nil, err
	}
	if category := s.takePreCommitCASFault(kind); category != "" {
		switch category {
		case core.ErrorConflict:
			return nil, conflict("injected Application SQLite CAS conflict before commit")
		case core.ErrorUnavailable:
			return nil, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Application SQLite CAS unavailable before commit")
		default:
			return nil, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Application SQLite CAS indeterminate before commit")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, mapDBError(ctx, err, true)
	}
	defer tx.Rollback()
	var curRev uint64
	var curDigest, curPrevious, curRowDigest, currentRowDigest string
	var current []byte
	var curChecked, curExpires int64
	err = tx.QueryRowContext(ctx, `SELECT c.revision,c.digest,h.previous_digest,h.row_digest,c.row_digest,h.payload_json,h.checked_unix_nano,h.expires_unix_nano FROM application_fact_current_v1 c JOIN application_fact_history_v1 h ON h.fact_type=c.fact_type AND h.fact_id=c.fact_id AND h.revision=c.revision AND h.digest=c.digest WHERE c.fact_type=? AND c.fact_id=?`, kind, id).Scan(&curRev, &curDigest, &curPrevious, &curRowDigest, &currentRowDigest, &current, &curChecked, &curExpires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Application SQLite Fact is absent")
	}
	if err != nil {
		return nil, mapDBError(ctx, err, true)
	}
	r := storedRowV1{revision: core.Revision(curRev), digest: core.Digest(curDigest), previous: core.Digest(curPrevious), rowDigest: core.Digest(curRowDigest), payload: current, checked: curChecked, expires: curExpires}
	if currentRowDigest != curRowDigest || validateStoredRowV1(kind, id, r) != nil {
		return nil, corrupt("Application SQLite CAS predecessor row digest drifted")
	}
	if core.Revision(curRev) != expectedRevision || core.Digest(curDigest) != expectedDigest {
		return nil, conflict("Application SQLite Fact CAS predecessor changed")
	}
	if nextRevision != expectedRevision+1 || previous != expectedDigest {
		return nil, conflict("Application SQLite Fact CAS successor coordinates drifted")
	}
	if err = validate(current); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO application_fact_history_v1(fact_type,fact_id,revision,digest,previous_digest,row_digest,payload_json,checked_unix_nano,expires_unix_nano) VALUES(?,?,?,?,?,?,?,?,?)`, kind, id, uint64(nextRevision), string(nextDigest), string(previous), string(rowDigest), payload, checked, expires); err != nil {
		return nil, mapDBError(ctx, err, true)
	}
	result, err := tx.ExecContext(ctx, `UPDATE application_fact_current_v1 SET revision=?,digest=?,row_digest=? WHERE fact_type=? AND fact_id=? AND revision=? AND digest=?`, uint64(nextRevision), string(nextDigest), string(rowDigest), kind, id, uint64(expectedRevision), string(expectedDigest))
	if err != nil {
		return nil, mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return nil, conflict("Application SQLite Fact CAS lost")
	}
	if err = tx.Commit(); err != nil {
		return nil, mapDBError(ctx, err, true)
	}
	if s.takeFault(kind, true) {
		return nil, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Application SQLite CAS committed but its reply was lost")
	}
	return payload, nil
}

func (s *StoreV1) EnsureAgentLifecycleFactV1(ctx context.Context, fact contract.AgentLifecycleFactV1) (contract.AgentLifecycleFactV1, error) {
	if err := fact.Validate(); err != nil {
		return contract.AgentLifecycleFactV1{}, err
	}
	if fact.State != contract.AgentLifecycleFactActiveV1 || fact.Revision != 1 {
		return contract.AgentLifecycleFactV1{}, conflict("Agent lifecycle Ensure requires the active revision one Fact")
	}
	if err := s.checkClock(fact.CheckedUnixNano, fact.ExpiresUnixNano); err != nil {
		return contract.AgentLifecycleFactV1{}, err
	}
	p, err := s.ensure(ctx, lifecycleKindV1, fact.LifecycleID, fact.Revision, fact.Digest, fact.PreviousDigest, fact.CheckedUnixNano, fact.ExpiresUnixNano, fact)
	if err != nil {
		return contract.AgentLifecycleFactV1{}, err
	}
	return decodeLifecycle(p, lifecycleKindV1, fact.LifecycleID)
}
func (s *StoreV1) InspectAgentLifecycleFactV1(ctx context.Context, id string) (contract.AgentLifecycleFactV1, error) {
	if strings.TrimSpace(id) == "" {
		return contract.AgentLifecycleFactV1{}, invalid("Agent lifecycle Inspect identity is required")
	}
	r, err := s.readCurrent(ctx, lifecycleKindV1, id)
	if err != nil {
		return contract.AgentLifecycleFactV1{}, err
	}
	value, err := decodeLifecycle(r.payload, lifecycleKindV1, id)
	if err != nil || value.Revision != r.revision || value.Digest != r.digest || value.PreviousDigest != r.previous || value.CheckedUnixNano != r.checked || value.ExpiresUnixNano != r.expires {
		if err != nil {
			return contract.AgentLifecycleFactV1{}, err
		}
		return contract.AgentLifecycleFactV1{}, corrupt("Application SQLite lifecycle row coordinates drifted")
	}
	return value, nil
}
func (s *StoreV1) CompareAndSwapAgentLifecycleFactV1(ctx context.Context, q applicationports.AgentLifecycleFactCASRequestV1) (contract.AgentLifecycleFactV1, error) {
	if q.LifecycleID == "" || q.ExpectedRevision == 0 || q.ExpectedDigest.Validate() != nil || q.Next.LifecycleID != q.LifecycleID {
		return contract.AgentLifecycleFactV1{}, invalid("Agent lifecycle CAS coordinates are incomplete")
	}
	if err := q.Next.Validate(); err != nil {
		return contract.AgentLifecycleFactV1{}, err
	}
	if err := s.checkClock(q.Next.CheckedUnixNano, q.Next.ExpiresUnixNano); err != nil {
		return contract.AgentLifecycleFactV1{}, err
	}
	p, err := s.cas(ctx, lifecycleKindV1, q.LifecycleID, q.ExpectedRevision, q.ExpectedDigest, q.Next.Revision, q.Next.Digest, q.Next.PreviousDigest, q.Next.CheckedUnixNano, q.Next.ExpiresUnixNano, q.Next, func(raw []byte) error {
		c, e := decodeLifecycle(raw, lifecycleKindV1, q.LifecycleID)
		if e != nil {
			return e
		}
		return contract.ValidateAgentLifecycleFactTransitionV1(c, q.Next)
	})
	if err != nil {
		return contract.AgentLifecycleFactV1{}, err
	}
	return decodeLifecycle(p, lifecycleKindV1, q.LifecycleID)
}

func (s *StoreV1) EnsureAgentActivationCoordinationV1(ctx context.Context, fact contract.AgentActivationCoordinationFactV1) (contract.AgentActivationCoordinationFactV1, error) {
	if err := fact.Validate(); err != nil {
		return contract.AgentActivationCoordinationFactV1{}, err
	}
	last := fact.Events[len(fact.Events)-1].RecordedUnixNano
	if err := s.checkClock(last, fact.Request.RequestedNotAfterUnixNano); err != nil {
		return contract.AgentActivationCoordinationFactV1{}, err
	}
	p, err := s.ensure(ctx, coordinationKindV1, fact.ActivationID, fact.Revision, fact.Digest, "", last, fact.Request.RequestedNotAfterUnixNano, fact)
	if err != nil {
		return contract.AgentActivationCoordinationFactV1{}, err
	}
	return decodeCoordination(p, coordinationKindV1, fact.ActivationID)
}
func (s *StoreV1) InspectAgentActivationCoordinationV1(ctx context.Context, id string) (contract.AgentActivationCoordinationFactV1, error) {
	if strings.TrimSpace(id) == "" {
		return contract.AgentActivationCoordinationFactV1{}, invalid("Agent activation coordination Inspect identity is required")
	}
	r, err := s.readCurrent(ctx, coordinationKindV1, id)
	if err != nil {
		return contract.AgentActivationCoordinationFactV1{}, err
	}
	value, err := decodeCoordination(r.payload, coordinationKindV1, id)
	if err != nil || value.Revision != r.revision || value.Digest != r.digest || value.Events[len(value.Events)-1].RecordedUnixNano != r.checked || value.Request.RequestedNotAfterUnixNano != r.expires {
		if err != nil {
			return contract.AgentActivationCoordinationFactV1{}, err
		}
		return contract.AgentActivationCoordinationFactV1{}, corrupt("Application SQLite coordination row coordinates drifted")
	}
	return value, nil
}
func (s *StoreV1) CompareAndSwapAgentActivationCoordinationV1(ctx context.Context, q applicationports.AgentActivationCoordinationCASRequestV1) (contract.AgentActivationCoordinationFactV1, error) {
	if q.ActivationID == "" || q.ExpectedRevision == 0 || q.ExpectedDigest.Validate() != nil || q.Next.ActivationID != q.ActivationID {
		return contract.AgentActivationCoordinationFactV1{}, invalid("Agent activation coordination CAS coordinates are incomplete")
	}
	if err := q.Next.Validate(); err != nil {
		return contract.AgentActivationCoordinationFactV1{}, err
	}
	last := q.Next.Events[len(q.Next.Events)-1].RecordedUnixNano
	if err := s.checkClock(last, q.Next.Request.RequestedNotAfterUnixNano); err != nil {
		return contract.AgentActivationCoordinationFactV1{}, err
	}
	p, err := s.cas(ctx, coordinationKindV1, q.ActivationID, q.ExpectedRevision, q.ExpectedDigest, q.Next.Revision, q.Next.Digest, q.ExpectedDigest, last, q.Next.Request.RequestedNotAfterUnixNano, q.Next, func(raw []byte) error {
		c, e := decodeCoordination(raw, coordinationKindV1, q.ActivationID)
		if e != nil {
			return e
		}
		return contract.ValidateAgentActivationCoordinationTransitionV1(c, q.Next)
	})
	if err != nil {
		return contract.AgentActivationCoordinationFactV1{}, err
	}
	return decodeCoordination(p, coordinationKindV1, q.ActivationID)
}

func decodeLifecycle(payload []byte, kind, id string) (contract.AgentLifecycleFactV1, error) {
	v, err := strictDecodeV1[contract.AgentLifecycleFactV1](payload)
	if err != nil {
		return v, err
	}
	if kind != lifecycleKindV1 || v.LifecycleID != id || v.Validate() != nil {
		return contract.AgentLifecycleFactV1{}, corrupt("Application SQLite lifecycle payload drifted")
	}
	return v, nil
}
func decodeCoordination(payload []byte, kind, id string) (contract.AgentActivationCoordinationFactV1, error) {
	v, err := strictDecodeV1[contract.AgentActivationCoordinationFactV1](payload)
	if err != nil {
		return v, err
	}
	if kind != coordinationKindV1 || v.ActivationID != id || v.Validate() != nil {
		return contract.AgentActivationCoordinationFactV1{}, corrupt("Application SQLite coordination payload drifted")
	}
	return v, nil
}

func (s *StoreV1) checkClock(checked, expires int64) error {
	now := s.clock()
	if now.IsZero() || now.UnixNano() < checked {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Application SQLite mutation clock regressed")
	}
	if expires <= checked || !now.Before(time.Unix(0, expires)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Application SQLite mutation Fact expired")
	}
	return nil
}
func (s *StoreV1) readReady(ctx context.Context) error {
	if s == nil || s.db == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Application SQLite store is unavailable")
	}
	if ctx == nil {
		return invalid("Application SQLite context is required")
	}
	if ctx.Err() != nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Application SQLite read context ended")
	}
	return nil
}
func (s *StoreV1) writeReady(ctx context.Context) error {
	if s == nil || s.db == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Application SQLite store is unavailable")
	}
	if ctx == nil {
		return invalid("Application SQLite context is required")
	}
	if ctx.Err() != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Application SQLite mutation context ended")
	}
	return nil
}
func invalid(m string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, m)
}
func conflict(m string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, m)
}
func corrupt(m string) error { return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, m) }
func mapDBError(ctx context.Context, err error, mutation bool) error {
	if err == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if mutation {
			return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Application SQLite mutation outcome is unknown")
		}
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Application SQLite read is unavailable")
	}
	m := strings.ToLower(err.Error())
	if strings.Contains(m, "locked") || strings.Contains(m, "busy") {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Application SQLite store is busy")
	}
	if strings.Contains(m, "constraint") || strings.Contains(m, "unique") {
		return conflict("Application SQLite uniqueness conflict")
	}
	if mutation {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Application SQLite mutation outcome is unknown")
	}
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Application SQLite read failed")
}

func (s *StoreV1) takeFault(kind string, cas bool) bool {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	var p *bool
	switch {
	case kind == lifecycleKindV1 && !cas:
		p = &s.loseLifecycleEnsure
	case kind == lifecycleKindV1 && cas:
		p = &s.loseLifecycleCAS
	case kind == coordinationKindV1 && !cas:
		p = &s.loseCoordinationEnsure
	case kind == coordinationKindV1 && cas:
		p = &s.loseCoordinationCAS
	case kind == restoreStageResultKindV1 && !cas:
		p = &s.loseRestoreStageEnsure
	case kind == restoreExecutionResultKindV1 && !cas:
		p = &s.loseRestoreExecutionEnsure
	default:
		return false
	}
	v := *p
	*p = false
	return v
}
func (s *StoreV1) LoseNextAgentLifecycleEnsureReplyV1() {
	s.faultMu.Lock()
	s.loseLifecycleEnsure = true
	s.faultMu.Unlock()
}
func (s *StoreV1) LoseNextAgentLifecycleCASReplyV1() {
	s.faultMu.Lock()
	s.loseLifecycleCAS = true
	s.faultMu.Unlock()
}
func (s *StoreV1) LoseNextAgentActivationCoordinationEnsureReplyV1() {
	s.faultMu.Lock()
	s.loseCoordinationEnsure = true
	s.faultMu.Unlock()
}
func (s *StoreV1) LoseNextAgentActivationCoordinationCASReplyV1() {
	s.faultMu.Lock()
	s.loseCoordinationCAS = true
	s.faultMu.Unlock()
}

// FailNextAgentActivationCoordinationCASBeforeCommitV1 is a deterministic
// no-commit fault hook for conformance tests.
func (s *StoreV1) FailNextAgentActivationCoordinationCASBeforeCommitV1(category core.ErrorCategory) {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	if category == core.ErrorConflict || category == core.ErrorUnavailable || category == core.ErrorIndeterminate {
		s.failCoordinationCASBeforeCommit = category
	}
}

func (s *StoreV1) takePreCommitCASFault(kind string) core.ErrorCategory {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	if kind != coordinationKindV1 {
		return ""
	}
	value := s.failCoordinationCASBeforeCommit
	s.failCoordinationCASBeforeCommit = ""
	return value
}
