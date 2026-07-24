// Package repository contains Agent Assembler owner repositories.
package repository

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

	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	_ "modernc.org/sqlite"
)

const assemblerSQLiteSchemaV1 = `
CREATE TABLE IF NOT EXISTS assembler_schema_v1(version INTEGER PRIMARY KEY,digest TEXT NOT NULL,applied_unix_nano INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS resolved_plan_history_v1(plan_id TEXT NOT NULL,revision INTEGER NOT NULL,digest TEXT NOT NULL,definition_id TEXT NOT NULL,row_digest TEXT NOT NULL,payload_json BLOB NOT NULL,created_unix_nano INTEGER NOT NULL,valid_until_unix_nano INTEGER NOT NULL,PRIMARY KEY(plan_id,revision));
CREATE UNIQUE INDEX IF NOT EXISTS resolved_plan_exact_v1 ON resolved_plan_history_v1(plan_id,revision,digest);
CREATE TABLE IF NOT EXISTS resolved_plan_current_history_v1(definition_id TEXT NOT NULL,revision INTEGER NOT NULL,digest TEXT NOT NULL,plan_id TEXT NOT NULL,plan_revision INTEGER NOT NULL,plan_digest TEXT NOT NULL,row_digest TEXT NOT NULL,payload_json BLOB NOT NULL,updated_unix_nano INTEGER NOT NULL,checked_unix_nano INTEGER NOT NULL,expires_unix_nano INTEGER NOT NULL,PRIMARY KEY(definition_id,revision),FOREIGN KEY(plan_id,plan_revision,plan_digest) REFERENCES resolved_plan_history_v1(plan_id,revision,digest));
CREATE UNIQUE INDEX IF NOT EXISTS resolved_plan_current_exact_v1 ON resolved_plan_current_history_v1(definition_id,revision,digest);
CREATE TABLE IF NOT EXISTS resolved_plan_current_v1(definition_id TEXT PRIMARY KEY,revision INTEGER NOT NULL,digest TEXT NOT NULL,row_digest TEXT NOT NULL,FOREIGN KEY(definition_id,revision,digest) REFERENCES resolved_plan_current_history_v1(definition_id,revision,digest));`

type SQLiteConfigV1 struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}
type SQLiteV1 struct {
	db                  *sql.DB
	clock               func() time.Time
	mu                  *sync.Mutex
	faultMu             sync.Mutex
	loseEnsure, loseCAS bool
}

var _ ports.ResolvedAgentPlanRepositoryV1 = (*SQLiteV1)(nil)
var assemblerSQLiteLocksV1 sync.Map

func OpenSQLiteV1(ctx context.Context, c SQLiteConfigV1) (*SQLiteV1, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, assemblerInvalid("assembler SQLite open requires a live context")
	}
	if strings.TrimSpace(c.Path) == "" {
		return nil, assemblerInvalid("assembler SQLite path is required")
	}
	if c.BusyTimeout <= 0 {
		c.BusyTimeout = 5 * time.Second
	}
	if c.BusyTimeout > time.Minute {
		return nil, assemblerInvalid("assembler SQLite busy timeout exceeds one minute")
	}
	if c.MaxOpenConns <= 0 {
		c.MaxOpenConns = 4
	}
	if c.MaxOpenConns > 32 {
		return nil, assemblerInvalid("assembler SQLite connection count exceeds 32")
	}
	if c.Clock == nil {
		c.Clock = time.Now
	}
	abs, err := filepath.Abs(c.Path)
	if err != nil {
		return nil, assemblerInvalid("assembler SQLite path invalid")
	}
	lock, _ := assemblerSQLiteLocksV1.LoadOrStore(abs, &sync.Mutex{})
	dsn := (&url.URL{Scheme: "file", Path: abs}).String() + fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_pragma=synchronous(FULL)&_txlock=immediate", c.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, assemblerDBErr(ctx, err, false)
	}
	db.SetMaxOpenConns(c.MaxOpenConns)
	db.SetMaxIdleConns(c.MaxOpenConns)
	s := &SQLiteV1{db: db, clock: c.Clock, mu: lock.(*sync.Mutex)}
	if err = s.migrate(ctx); err == nil {
		err = s.verify(ctx)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}
func (s *SQLiteV1) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
func (s *SQLiteV1) LoseNextEnsureReplyV1() {
	if s == nil {
		return
	}
	s.faultMu.Lock()
	s.loseEnsure = true
	s.faultMu.Unlock()
}
func (s *SQLiteV1) LoseNextCASReplyV1() {
	if s == nil {
		return
	}
	s.faultMu.Lock()
	s.loseCAS = true
	s.faultMu.Unlock()
}
func (s *SQLiteV1) takeFault(cas bool) bool {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	if cas {
		v := s.loseCAS
		s.loseCAS = false
		return v
	}
	v := s.loseEnsure
	s.loseEnsure = false
	return v
}
func (s *SQLiteV1) migrate(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return assemblerDBErr(ctx, err, true)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, assemblerSQLiteSchemaV1); err != nil {
		return assemblerDBErr(ctx, err, true)
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "assembler SQLite migration clock invalid")
	}
	digest := core.DigestBytes([]byte(assemblerSQLiteSchemaV1))
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO assembler_schema_v1(version,digest,applied_unix_nano) VALUES(1,?,?)`, string(digest), now.UnixNano()); err != nil {
		return assemblerDBErr(ctx, err, true)
	}
	var stored string
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT digest FROM assembler_schema_v1 WHERE version=1`).Scan(&stored); err != nil {
		return assemblerDBErr(ctx, err, false)
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM assembler_schema_v1`).Scan(&count); err != nil {
		return assemblerDBErr(ctx, err, false)
	}
	if stored != string(digest) || count != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "assembler SQLite schema digest drifted")
	}
	if err = tx.Commit(); err != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "assembler SQLite migration commit unknown")
	}
	return nil
}
func (s *SQLiteV1) verify(ctx context.Context) error {
	var wal string
	var fk int
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&wal); err != nil {
		return assemblerDBErr(ctx, err, false)
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
		return assemblerDBErr(ctx, err, false)
	}
	if !strings.EqualFold(wal, "wal") || fk != 1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "assembler SQLite WAL or foreign keys inactive")
	}
	return nil
}
func (s *SQLiteV1) IntegrityCheckV1(ctx context.Context) error {
	if s == nil || s.db == nil {
		return assemblerInvalid("assembler SQLite repository is nil")
	}
	var v string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&v); err != nil {
		return assemblerDBErr(ctx, err, false)
	}
	if v != "ok" {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "assembler SQLite integrity check failed")
	}
	return nil
}

func assemblerStrictV1[T any](raw []byte) (T, error) {
	var v T
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&v); err != nil {
		return v, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "assembler SQLite strict decode failed")
	}
	var tail any
	if err := d.Decode(&tail); !errors.Is(err, io.EOF) {
		return v, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "assembler SQLite trailing JSON rejected")
	}
	return v, nil
}
func assemblerRowDigestV1(kind, id string, revision core.Revision, digest core.Digest, raw []byte) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.agent-assembler.sqlite", "v1", "AssemblerSQLiteRowV1", struct {
		Kind          string        `json:"kind"`
		ID            string        `json:"id"`
		Revision      core.Revision `json:"revision"`
		Digest        core.Digest   `json:"digest"`
		PayloadDigest core.Digest   `json:"payload_digest"`
	}{kind, id, revision, digest, core.DigestBytes(raw)})
}
func assemblerEncodeV1(v any, kind, id string, revision core.Revision, digest core.Digest) ([]byte, core.Digest, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, "", core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "assembler SQLite encode failed")
	}
	rd, err := assemblerRowDigestV1(kind, id, revision, digest, raw)
	return raw, rd, err
}

func (s *SQLiteV1) EnsureExactResolvedAgentPlanV1(ctx context.Context, v contract.ResolvedAgentPlanV1) (contract.ResolvedAgentPlanV1, error) {
	if s == nil || s.db == nil {
		return contract.ResolvedAgentPlanV1{}, assemblerInvalid("assembler SQLite repository is nil")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.ResolvedAgentPlanV1{}, assemblerUnavailable("assembler SQLite Ensure context ended")
	}
	if err := v.Validate(); err != nil {
		return contract.ResolvedAgentPlanV1{}, err
	}
	raw, rd, err := assemblerEncodeV1(v, "plan", v.PlanID, v.Revision, v.Digest)
	if err != nil {
		return contract.ResolvedAgentPlanV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return contract.ResolvedAgentPlanV1{}, assemblerDBErr(ctx, err, true)
	}
	defer tx.Rollback()
	existing, readErr := s.inspectPlanRevisionTx(ctx, tx, v.PlanID, v.Revision)
	if readErr == nil {
		if existing.Digest != v.Digest {
			return contract.ResolvedAgentPlanV1{}, assemblerConflict("resolved plan identity occupied")
		}
		return existing, nil
	}
	if !core.HasCategory(readErr, core.ErrorNotFound) {
		return contract.ResolvedAgentPlanV1{}, readErr
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO resolved_plan_history_v1(plan_id,revision,digest,definition_id,row_digest,payload_json,created_unix_nano,valid_until_unix_nano) VALUES(?,?,?,?,?,?,?,?)`, v.PlanID, uint64(v.Revision), string(v.Digest), v.DefinitionRef.DefinitionID, string(rd), raw, v.CreatedUnixNano, v.ValidUntilUnixNano); err != nil {
		return contract.ResolvedAgentPlanV1{}, assemblerDBErr(ctx, err, true)
	}
	if err = tx.Commit(); err != nil {
		return contract.ResolvedAgentPlanV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "assembler SQLite plan commit unknown")
	}
	if s.takeFault(false) {
		return contract.ResolvedAgentPlanV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "assembler SQLite plan committed but reply lost")
	}
	return contract.CloneResolvedAgentPlanV1(v), nil
}
func (s *SQLiteV1) inspectPlanRevisionTx(ctx context.Context, tx *sql.Tx, id string, revision core.Revision) (contract.ResolvedAgentPlanV1, error) {
	var digest, definitionID, rowDigest string
	var raw []byte
	var created, expires int64
	err := tx.QueryRowContext(ctx, `SELECT digest,definition_id,row_digest,payload_json,created_unix_nano,valid_until_unix_nano FROM resolved_plan_history_v1 WHERE plan_id=? AND revision=?`, id, uint64(revision)).Scan(&digest, &definitionID, &rowDigest, &raw, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.ResolvedAgentPlanV1{}, core.NewError(core.ErrorNotFound, core.ReasonPlanInvalid, "resolved plan absent")
	}
	if err != nil {
		return contract.ResolvedAgentPlanV1{}, assemblerDBErr(ctx, err, false)
	}
	expected, e := assemblerRowDigestV1("plan", id, revision, core.Digest(digest), raw)
	v, e2 := assemblerStrictV1[contract.ResolvedAgentPlanV1](raw)
	if e != nil || e2 != nil || expected != core.Digest(rowDigest) || v.PlanID != id || v.Revision != revision || v.Digest != core.Digest(digest) || v.DefinitionRef.DefinitionID != definitionID || v.CreatedUnixNano != created || v.ValidUntilUnixNano != expires || v.Validate() != nil {
		return contract.ResolvedAgentPlanV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "assembler SQLite plan row drifted")
	}
	return contract.CloneResolvedAgentPlanV1(v), nil
}
func (s *SQLiteV1) InspectExactResolvedAgentPlanV1(ctx context.Context, ref contract.ResolvedAgentPlanRefV1) (contract.ResolvedAgentPlanV1, error) {
	if s == nil || s.db == nil {
		return contract.ResolvedAgentPlanV1{}, assemblerInvalid("assembler SQLite repository is nil")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.ResolvedAgentPlanV1{}, assemblerUnavailable("assembler SQLite plan Inspect context ended")
	}
	if err := ref.Validate(); err != nil {
		return contract.ResolvedAgentPlanV1{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return contract.ResolvedAgentPlanV1{}, assemblerDBErr(ctx, err, false)
	}
	defer tx.Rollback()
	v, err := s.inspectPlanRevisionTx(ctx, tx, ref.PlanID, ref.Revision)
	if err != nil {
		return v, err
	}
	if v.Digest != ref.Digest {
		return contract.ResolvedAgentPlanV1{}, assemblerConflict("resolved plan exact ref drifted")
	}
	return v, nil
}

func (s *SQLiteV1) inspectCurrentTx(ctx context.Context, tx *sql.Tx, definitionID string) (contract.CurrentResolvedPlanV1, error) {
	var rev uint64
	var digest, rowDigest, currentRowDigest string
	var raw []byte
	var planID, planDigest string
	var planRev uint64
	var updated, checked, expires int64
	err := tx.QueryRowContext(ctx, `SELECT h.revision,h.digest,h.plan_id,h.plan_revision,h.plan_digest,h.row_digest,c.row_digest,h.payload_json,h.updated_unix_nano,h.checked_unix_nano,h.expires_unix_nano FROM resolved_plan_current_v1 c JOIN resolved_plan_current_history_v1 h ON h.definition_id=c.definition_id AND h.revision=c.revision AND h.digest=c.digest WHERE c.definition_id=?`, definitionID).Scan(&rev, &digest, &planID, &planRev, &planDigest, &rowDigest, &currentRowDigest, &raw, &updated, &checked, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.CurrentResolvedPlanV1{}, core.NewError(core.ErrorNotFound, core.ReasonPlanInvalid, "resolved plan current absent")
	}
	if err != nil {
		return contract.CurrentResolvedPlanV1{}, assemblerDBErr(ctx, err, false)
	}
	expected, e := assemblerRowDigestV1("current", definitionID, core.Revision(rev), core.Digest(digest), raw)
	v, e2 := assemblerStrictV1[contract.CurrentResolvedPlanV1](raw)
	if e != nil || e2 != nil || expected != core.Digest(rowDigest) || rowDigest != currentRowDigest || v.DefinitionID != definitionID || v.Revision != core.Revision(rev) || v.Digest != core.Digest(digest) || v.PlanRef.PlanID != planID || v.PlanRef.Revision != core.Revision(planRev) || v.PlanRef.Digest != core.Digest(planDigest) || v.UpdatedUnixNano != updated || v.CheckedUnixNano != checked || v.ExpiresUnixNano != expires || v.Validate() != nil {
		return contract.CurrentResolvedPlanV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "assembler SQLite current row drifted")
	}
	plan, e3 := s.inspectPlanRevisionTx(ctx, tx, v.PlanRef.PlanID, v.PlanRef.Revision)
	if e3 != nil || plan.RefV1() != v.PlanRef || plan.DefinitionRef.DefinitionID != definitionID {
		return contract.CurrentResolvedPlanV1{}, core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "assembler SQLite current target drifted")
	}
	return contract.CloneCurrentResolvedPlanV1(v), nil
}
func (s *SQLiteV1) InspectCurrentResolvedAgentPlanV1(ctx context.Context, definitionID string) (contract.CurrentResolvedPlanV1, error) {
	if s == nil || s.db == nil {
		return contract.CurrentResolvedPlanV1{}, assemblerInvalid("assembler SQLite repository is nil")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.CurrentResolvedPlanV1{}, assemblerUnavailable("assembler SQLite current Inspect context ended")
	}
	if strings.TrimSpace(definitionID) == "" {
		return contract.CurrentResolvedPlanV1{}, assemblerInvalid("assembler SQLite definition identity required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return contract.CurrentResolvedPlanV1{}, assemblerDBErr(ctx, err, false)
	}
	defer tx.Rollback()
	return s.inspectCurrentTx(ctx, tx, definitionID)
}

func (s *SQLiteV1) CompareAndSwapCurrentResolvedAgentPlanV1(ctx context.Context, expected *contract.CurrentResolvedPlanRefV1, next contract.CurrentResolvedPlanV1) (contract.CurrentResolvedPlanV1, error) {
	if s == nil || s.db == nil {
		return contract.CurrentResolvedPlanV1{}, assemblerInvalid("assembler SQLite repository is nil")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.CurrentResolvedPlanV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "assembler SQLite CAS context ended")
	}
	if err := next.Validate(); err != nil {
		return contract.CurrentResolvedPlanV1{}, err
	}
	if expected != nil {
		if err := expected.Validate(); err != nil {
			return contract.CurrentResolvedPlanV1{}, err
		}
	}
	raw, rd, err := assemblerEncodeV1(next, "current", next.DefinitionID, next.Revision, next.Digest)
	if err != nil {
		return contract.CurrentResolvedPlanV1{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return contract.CurrentResolvedPlanV1{}, assemblerDBErr(ctx, err, true)
	}
	defer tx.Rollback()
	current, readErr := s.inspectCurrentTx(ctx, tx, next.DefinitionID)
	exists := readErr == nil
	if readErr != nil && !core.HasCategory(readErr, core.ErrorNotFound) {
		return contract.CurrentResolvedPlanV1{}, readErr
	}
	if expected == nil {
		if exists {
			return contract.CurrentResolvedPlanV1{}, assemblerConflict("resolved plan current already exists")
		}
		if next.Revision != 1 || next.PreviousRef != nil {
			return contract.CurrentResolvedPlanV1{}, assemblerConflict("initial current projection coordinates drifted")
		}
	} else {
		if !exists || current.RefV1() != *expected {
			return contract.CurrentResolvedPlanV1{}, assemblerConflict("resolved plan current CAS predecessor changed")
		}
		if next.Revision != current.Revision+1 || next.PreviousRef == nil || *next.PreviousRef != *expected || next.UpdatedUnixNano < current.UpdatedUnixNano {
			return contract.CurrentResolvedPlanV1{}, assemblerConflict("resolved plan current successor drifted")
		}
	}
	plan, e := s.inspectPlanRevisionTx(ctx, tx, next.PlanRef.PlanID, next.PlanRef.Revision)
	if e != nil || plan.RefV1() != next.PlanRef || plan.DefinitionRef.DefinitionID != next.DefinitionID {
		return contract.CurrentResolvedPlanV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "resolved plan current target absent or drifted")
	}
	var seen int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM resolved_plan_current_history_v1 WHERE definition_id=? AND plan_id=? AND plan_revision=? AND plan_digest=?`, next.DefinitionID, next.PlanRef.PlanID, uint64(next.PlanRef.Revision), string(next.PlanRef.Digest)).Scan(&seen); err != nil {
		return contract.CurrentResolvedPlanV1{}, assemblerDBErr(ctx, err, false)
	}
	if seen != 0 {
		return contract.CurrentResolvedPlanV1{}, assemblerConflict("resolved plan current cannot revisit historical plan")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO resolved_plan_current_history_v1(definition_id,revision,digest,plan_id,plan_revision,plan_digest,row_digest,payload_json,updated_unix_nano,checked_unix_nano,expires_unix_nano) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, next.DefinitionID, uint64(next.Revision), string(next.Digest), next.PlanRef.PlanID, uint64(next.PlanRef.Revision), string(next.PlanRef.Digest), string(rd), raw, next.UpdatedUnixNano, next.CheckedUnixNano, next.ExpiresUnixNano); err != nil {
		return contract.CurrentResolvedPlanV1{}, assemblerDBErr(ctx, err, true)
	}
	if expected == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO resolved_plan_current_v1(definition_id,revision,digest,row_digest) VALUES(?,?,?,?)`, next.DefinitionID, uint64(next.Revision), string(next.Digest), string(rd))
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE resolved_plan_current_v1 SET revision=?,digest=?,row_digest=? WHERE definition_id=? AND revision=? AND digest=?`, uint64(next.Revision), string(next.Digest), string(rd), next.DefinitionID, uint64(expected.Revision), string(expected.Digest))
		if err == nil {
			var affected int64
			affected, err = result.RowsAffected()
			if err == nil && affected != 1 {
				return contract.CurrentResolvedPlanV1{}, assemblerConflict("resolved plan current CAS lost")
			}
		}
	}
	if err != nil {
		return contract.CurrentResolvedPlanV1{}, assemblerDBErr(ctx, err, true)
	}
	if err = tx.Commit(); err != nil {
		return contract.CurrentResolvedPlanV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "assembler SQLite CAS commit unknown")
	}
	if s.takeFault(true) {
		return contract.CurrentResolvedPlanV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "assembler SQLite CAS committed but reply lost")
	}
	return contract.CloneCurrentResolvedPlanV1(next), nil
}

func assemblerInvalid(m string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, m)
}
func assemblerConflict(m string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, m)
}
func assemblerUnavailable(m string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, m)
}
func assemblerDBErr(ctx context.Context, err error, mutation bool) error {
	if err == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if mutation {
			return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "assembler SQLite mutation outcome unknown")
		}
		return assemblerUnavailable("assembler SQLite read unavailable")
	}
	m := strings.ToLower(err.Error())
	if strings.Contains(m, "locked") || strings.Contains(m, "busy") {
		return assemblerUnavailable("assembler SQLite busy")
	}
	if strings.Contains(m, "constraint") || strings.Contains(m, "unique") {
		return assemblerConflict("assembler SQLite constraint conflict")
	}
	if mutation {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "assembler SQLite mutation outcome unknown")
	}
	return assemblerUnavailable("assembler SQLite read failed")
}
