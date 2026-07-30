package repository

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	_ "modernc.org/sqlite"
)

const schemaV1 = "CREATE TABLE IF NOT EXISTS agent_package_schema_v1(version INTEGER PRIMARY KEY,digest TEXT NOT NULL,applied_unix_nano INTEGER NOT NULL);" +
	"CREATE TABLE IF NOT EXISTS agent_package_v1(package_id TEXT NOT NULL,revision INTEGER NOT NULL,digest TEXT NOT NULL,row_digest TEXT NOT NULL,payload_json BLOB NOT NULL,created_unix_nano INTEGER NOT NULL,PRIMARY KEY(package_id,revision));" +
	"CREATE UNIQUE INDEX IF NOT EXISTS agent_package_exact_v1 ON agent_package_v1(package_id,revision,digest);"

type SQLiteConfigV1 struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}

type SQLiteRepositoryV1 struct {
	db       *sql.DB
	mu       *sync.Mutex
	clock    func() time.Time
	faultMu  sync.Mutex
	loseNext bool
}

var _ ports.AgentPackageRepositoryV1 = (*SQLiteRepositoryV1)(nil)
var locksV1 sync.Map

func OpenSQLiteRepositoryV1(ctx context.Context, c SQLiteConfigV1) (*SQLiteRepositoryV1, error) {
	if ctx == nil || ctx.Err() != nil || strings.TrimSpace(c.Path) == "" {
		return nil, invalid("agent package SQLite open requires a live context and path")
	}
	if c.BusyTimeout <= 0 {
		c.BusyTimeout = 5 * time.Second
	}
	if c.BusyTimeout > time.Minute {
		return nil, invalid("agent package SQLite busy timeout exceeds one minute")
	}
	if c.MaxOpenConns <= 0 {
		c.MaxOpenConns = 4
	}
	if c.MaxOpenConns > 32 {
		return nil, invalid("agent package SQLite connection count exceeds 32")
	}
	if c.Clock == nil {
		c.Clock = time.Now
	}
	abs, err := filepath.Abs(c.Path)
	if err != nil {
		return nil, invalid("agent package SQLite path is invalid")
	}
	lock, _ := locksV1.LoadOrStore(abs, &sync.Mutex{})
	dsn := "file:" + filepath.ToSlash(abs) + fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_pragma=synchronous(FULL)&_txlock=immediate", c.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, dbErr(ctx, err, false)
	}
	db.SetMaxOpenConns(c.MaxOpenConns)
	db.SetMaxIdleConns(c.MaxOpenConns)
	r := &SQLiteRepositoryV1{db: db, mu: lock.(*sync.Mutex), clock: c.Clock}
	if err = r.migrate(ctx); err == nil {
		err = r.verify(ctx)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return r, nil
}

func (r *SQLiteRepositoryV1) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}
func (r *SQLiteRepositoryV1) LoseNextEnsureReplyV1() {
	if r == nil {
		return
	}
	r.faultMu.Lock()
	r.loseNext = true
	r.faultMu.Unlock()
}
func (r *SQLiteRepositoryV1) takeLost() bool {
	r.faultMu.Lock()
	defer r.faultMu.Unlock()
	v := r.loseNext
	r.loseNext = false
	return v
}

func (r *SQLiteRepositoryV1) migrate(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return dbErr(ctx, err, true)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, schemaV1); err != nil {
		return dbErr(ctx, err, true)
	}
	now := r.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "agent package SQLite migration clock invalid")
	}
	digest := core.DigestBytes([]byte(schemaV1))
	if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO agent_package_schema_v1(version,digest,applied_unix_nano) VALUES(1,?,?)", string(digest), now.UnixNano()); err != nil {
		return dbErr(ctx, err, true)
	}
	var stored string
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT digest FROM agent_package_schema_v1 WHERE version=1").Scan(&stored); err != nil {
		return dbErr(ctx, err, false)
	}
	if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM agent_package_schema_v1").Scan(&count); err != nil {
		return dbErr(ctx, err, false)
	}
	if stored != string(digest) || count != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "agent package SQLite schema digest drifted")
	}
	if err = tx.Commit(); err != nil {
		return indeterminate("agent package SQLite migration commit outcome unknown")
	}
	return nil
}

func (r *SQLiteRepositoryV1) verify(ctx context.Context) error {
	var wal string
	var foreignKeys, synchronous int
	if err := r.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&wal); err != nil {
		return dbErr(ctx, err, false)
	}
	if err := r.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return dbErr(ctx, err, false)
	}
	if err := r.db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		return dbErr(ctx, err, false)
	}
	if !strings.EqualFold(wal, "wal") || foreignKeys != 1 || synchronous != 2 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "agent package SQLite WAL, foreign keys or FULL sync inactive")
	}
	return nil
}

func (r *SQLiteRepositoryV1) IntegrityCheckV1(ctx context.Context) error {
	if r == nil || r.db == nil {
		return invalid("agent package SQLite repository is nil")
	}
	var value string
	if err := r.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&value); err != nil {
		return dbErr(ctx, err, false)
	}
	if value != "ok" {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "agent package SQLite integrity check failed")
	}
	return nil
}

func (r *SQLiteRepositoryV1) EnsureExactAgentPackageV1(ctx context.Context, p contract.AgentPackageV1) (contract.AgentPackageV1, error) {
	if r == nil || r.db == nil {
		return contract.AgentPackageV1{}, invalid("agent package SQLite repository is nil")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.AgentPackageV1{}, indeterminate("agent package ensure requires live context")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return contract.AgentPackageV1{}, dbErr(ctx, err, true)
	}
	defer tx.Rollback()
	stored, err := inspectTx(ctx, tx, p.PackageID, p.Revision)
	if err == nil {
		if !reflect.DeepEqual(stored, p) {
			return contract.AgentPackageV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "agent package coordinate already has different content")
		}
		if err = tx.Commit(); err != nil {
			return contract.AgentPackageV1{}, indeterminate("agent package idempotent ensure commit outcome unknown")
		}
		return clone(stored), nil
	}
	if !core.HasCategory(err, core.ErrorNotFound) {
		return contract.AgentPackageV1{}, err
	}
	if err = p.Validate(); err != nil {
		return contract.AgentPackageV1{}, err
	}
	raw, row, err := encode(p)
	if err != nil {
		return contract.AgentPackageV1{}, err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO agent_package_v1(package_id,revision,digest,row_digest,payload_json,created_unix_nano) VALUES(?,?,?,?,?,?)", p.PackageID, uint64(p.Revision), string(p.Digest), string(row), raw, p.CreatedUnixNano)
	if err != nil {
		return contract.AgentPackageV1{}, dbErr(ctx, err, true)
	}
	if err = tx.Commit(); err != nil {
		return contract.AgentPackageV1{}, indeterminate("agent package ensure commit outcome unknown")
	}
	if r.takeLost() {
		return contract.AgentPackageV1{}, indeterminate("agent package ensure reply lost after commit")
	}
	return clone(p), nil
}

func (r *SQLiteRepositoryV1) InspectExactAgentPackageV1(ctx context.Context, ref contract.AgentPackageRefV1) (contract.AgentPackageV1, error) {
	if r == nil || r.db == nil {
		return contract.AgentPackageV1{}, invalid("agent package SQLite repository is nil")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.AgentPackageV1{}, unavailable("agent package inspect requires live context")
	}
	if err := ref.Validate(); err != nil {
		return contract.AgentPackageV1{}, err
	}
	var raw []byte
	var digest, row string
	var created int64
	err := r.db.QueryRowContext(ctx, "SELECT digest,row_digest,payload_json,created_unix_nano FROM agent_package_v1 WHERE package_id=? AND revision=?", ref.PackageID, uint64(ref.Revision)).Scan(&digest, &row, &raw, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.AgentPackageV1{}, notFound()
	}
	if err != nil {
		return contract.AgentPackageV1{}, dbErr(ctx, err, false)
	}
	return decodeExact(ref, digest, row, raw, created)
}

func inspectTx(ctx context.Context, tx *sql.Tx, id string, revision core.Revision) (contract.AgentPackageV1, error) {
	var raw []byte
	var digest, row string
	var created int64
	err := tx.QueryRowContext(ctx, "SELECT digest,row_digest,payload_json,created_unix_nano FROM agent_package_v1 WHERE package_id=? AND revision=?", id, uint64(revision)).Scan(&digest, &row, &raw, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.AgentPackageV1{}, notFound()
	}
	if err != nil {
		return contract.AgentPackageV1{}, dbErr(ctx, err, false)
	}
	ref := contract.AgentPackageRefV1{PackageID: id, Revision: revision, Digest: core.Digest(digest), ContractVersion: contract.ContractVersionV1, SchemaVersion: contract.SchemaVersionV1}
	return decodeExact(ref, digest, row, raw, created)
}

func decodeExact(ref contract.AgentPackageRefV1, digest, row string, raw []byte, created int64) (contract.AgentPackageV1, error) {
	expected, err := rowDigest(ref.PackageID, ref.Revision, core.Digest(digest), raw)
	if err != nil || expected != core.Digest(row) {
		return contract.AgentPackageV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "agent package SQLite row digest drifted")
	}
	p, err := strictJSON[contract.AgentPackageV1](raw)
	if err != nil || p.PackageID != ref.PackageID || p.Revision != ref.Revision || p.Digest != core.Digest(digest) || p.CreatedUnixNano != created || p.Validate() != nil {
		return contract.AgentPackageV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "agent package SQLite payload drifted")
	}
	if p.RefV1() != ref {
		return contract.AgentPackageV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "agent package exact ref mismatch")
	}
	return clone(p), nil
}

type rowDigestInputV1 struct {
	ID            string
	Revision      core.Revision
	Digest        core.Digest
	PayloadDigest core.Digest
}

func encode(p contract.AgentPackageV1) ([]byte, core.Digest, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, "", core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "agent package SQLite encode failed")
	}
	digest, err := rowDigest(p.PackageID, p.Revision, p.Digest, raw)
	return raw, digest, err
}
func rowDigest(id string, revision core.Revision, digest core.Digest, raw []byte) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.agent-builder.sqlite", "v1", "AgentPackageSQLiteRowV1", rowDigestInputV1{id, revision, digest, core.DigestBytes(raw)})
}
func strictJSON[T any](raw []byte) (T, error) {
	var value T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	var tail any
	if err := decoder.Decode(&tail); !errors.Is(err, io.EOF) {
		return value, errors.New("trailing JSON")
	}
	return value, nil
}
func clone[T any](value T) T {
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var result T
	if json.Unmarshal(raw, &result) != nil {
		return value
	}
	return result
}
func invalid(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, message)
}
func unavailable(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, message)
}
func indeterminate(message string) error {
	return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, message)
}
func notFound() error {
	return core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "agent package exact ref not found")
}
func dbErr(ctx context.Context, err error, mutation bool) error {
	if err == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if mutation {
			return indeterminate("agent package SQLite mutation outcome unknown")
		}
		return unavailable("agent package SQLite read unavailable")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "locked") || strings.Contains(message, "busy") {
		return unavailable("agent package SQLite busy")
	}
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "agent package SQLite coordinate conflict")
	}
	if mutation {
		return indeterminate("agent package SQLite mutation outcome unknown")
	}
	return unavailable("agent package SQLite read failed")
}

func EnsureExactWithRecoveryV1(ctx context.Context, repository ports.AgentPackageRepositoryV1, reader ports.AgentPackageExactReaderV1, p contract.AgentPackageV1) (contract.AgentPackageV1, error) {
	if nilInterface(repository) || nilInterface(reader) {
		return contract.AgentPackageV1{}, invalid("agent package recovery requires repository and exact reader")
	}
	result, err := repository.EnsureExactAgentPackageV1(ctx, p)
	if err == nil {
		return result, nil
	}
	if !core.HasCategory(err, core.ErrorIndeterminate) {
		return contract.AgentPackageV1{}, err
	}
	inspected, inspectErr := reader.InspectExactAgentPackageV1(ctx, p.RefV1())
	if inspectErr != nil {
		return contract.AgentPackageV1{}, err
	}
	if !reflect.DeepEqual(inspected, p) {
		return contract.AgentPackageV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "agent package recovery observed different body")
	}
	return clone(inspected), nil
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func, reflect.Chan:
		return reflected.IsNil()
	}
	return false
}
