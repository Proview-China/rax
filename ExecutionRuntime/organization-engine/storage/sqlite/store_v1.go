package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/organization-engine/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	_ "modernc.org/sqlite"
)

const schemaV1 = `
CREATE TABLE IF NOT EXISTS organization_schema(version INTEGER PRIMARY KEY,digest TEXT NOT NULL,applied_unix_nano INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS organization_fact_history(kind TEXT NOT NULL,tenant_id TEXT NOT NULL,id TEXT NOT NULL,revision INTEGER NOT NULL,digest TEXT NOT NULL,canonical_json BLOB NOT NULL,created_unix_nano INTEGER NOT NULL,updated_unix_nano INTEGER NOT NULL,expires_unix_nano INTEGER NOT NULL,PRIMARY KEY(kind,tenant_id,id,revision));
CREATE UNIQUE INDEX IF NOT EXISTS organization_fact_exact ON organization_fact_history(kind,tenant_id,id,revision,digest);
CREATE TABLE IF NOT EXISTS organization_fact_current(kind TEXT NOT NULL,tenant_id TEXT NOT NULL,id TEXT NOT NULL,revision INTEGER NOT NULL,digest TEXT NOT NULL,PRIMARY KEY(kind,tenant_id,id),FOREIGN KEY(kind,tenant_id,id,revision,digest) REFERENCES organization_fact_history(kind,tenant_id,id,revision,digest));
CREATE TABLE IF NOT EXISTS organization_review_projection(tenant_id TEXT NOT NULL,id TEXT NOT NULL,digest TEXT NOT NULL,canonical_json BLOB NOT NULL,checked_unix_nano INTEGER NOT NULL,expires_unix_nano INTEGER NOT NULL,PRIMARY KEY(tenant_id,id));`

type Config struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}
type Store struct {
	db    *sql.DB
	clock func() time.Time
}

var _ ports.StoreV1 = (*Store)(nil)

func Open(ctx context.Context, c Config) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, ports.IndeterminateV1("organization sqlite open context ended")
	}
	if strings.TrimSpace(c.Path) == "" {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "organization sqlite path is required")
	}
	if c.BusyTimeout <= 0 {
		c.BusyTimeout = 5 * time.Second
	}
	if c.BusyTimeout > time.Minute {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "organization sqlite busy timeout exceeds bound")
	}
	if c.MaxOpenConns <= 0 {
		c.MaxOpenConns = 4
	}
	if c.MaxOpenConns > 32 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "organization sqlite connection count exceeds bound")
	}
	if c.Clock == nil {
		c.Clock = time.Now
	}
	abs, err := filepath.Abs(c.Path)
	if err != nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "organization sqlite path is invalid")
	}
	dsn := (&url.URL{Scheme: "file", Path: abs}).String() + fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_txlock=immediate", c.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapErr(ctx, err, false)
	}
	db.SetMaxOpenConns(c.MaxOpenConns)
	db.SetMaxIdleConns(c.MaxOpenConns)
	s := &Store{db, c.Clock}
	if err = s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err = s.verify(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
func (s *Store) IntegrityCheckV1(ctx context.Context) error {
	var v string
	if err := s.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&v); err != nil {
		return mapErr(ctx, err, false)
	}
	if v != "ok" {
		return core.NewError(core.ErrorInternal, core.ReasonInvalidDigest, "organization sqlite integrity check failed")
	}
	return nil
}
func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapErr(ctx, err, true)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, schemaV1); err != nil {
		return mapErr(ctx, err, true)
	}
	d := core.DigestBytes([]byte(schemaV1))
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "organization migration clock invalid")
	}
	res, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO organization_schema(version,digest,applied_unix_nano) VALUES(1,?,?)", string(d), now.UnixNano())
	if err != nil {
		return mapErr(ctx, err, true)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		var got string
		if err = tx.QueryRowContext(ctx, "SELECT digest FROM organization_schema WHERE version=1").Scan(&got); err != nil {
			return mapErr(ctx, err, true)
		}
		if got != string(d) {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "organization schema digest drifted")
		}
	}
	if err = tx.Commit(); err != nil {
		return ports.IndeterminateV1("organization migration commit outcome unknown")
	}
	return nil
}

func (s *Store) verify(ctx context.Context) error {
	var wal string
	if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&wal); err != nil {
		return mapErr(ctx, err, false)
	}
	if !strings.EqualFold(wal, "wal") {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "organization WAL inactive")
	}
	var fk int
	if err := s.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		return mapErr(ctx, err, false)
	}
	if fk != 1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "organization foreign keys inactive")
	}
	return nil
}

type ref struct {
	revision core.Revision
	digest   core.Digest
}

func (s *Store) publish(ctx context.Context, kind string, tenant core.TenantID, id string, revision core.Revision, digest core.Digest, expected *ref, value any, created, updated, expires int64) error {
	if err := ctx.Err(); err != nil {
		return ports.IndeterminateV1("organization sqlite mutation context ended")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "organization sqlite encode failed")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapErr(ctx, err, true)
	}
	defer tx.Rollback()
	var curRev uint64
	var curDigest string
	curErr := tx.QueryRowContext(ctx, "SELECT revision,digest FROM organization_fact_current WHERE kind=? AND tenant_id=? AND id=?", kind, string(tenant), id).Scan(&curRev, &curDigest)
	exists := curErr == nil
	if curErr != nil && !errors.Is(curErr, sql.ErrNoRows) {
		return mapErr(ctx, curErr, true)
	}
	var old []byte
	var oldDigest string
	rowErr := tx.QueryRowContext(ctx, "SELECT canonical_json,digest FROM organization_fact_history WHERE kind=? AND tenant_id=? AND id=? AND revision=?", kind, string(tenant), id, uint64(revision)).Scan(&old, &oldDigest)
	if rowErr == nil {
		if string(old) == string(payload) && oldDigest == string(digest) && exists && curRev == uint64(revision) && curDigest == string(digest) {
			return nil
		}
		return ports.ConflictV1("same revision carries different content or is historical")
	}
	if !errors.Is(rowErr, sql.ErrNoRows) {
		return mapErr(ctx, rowErr, true)
	}
	if expected == nil {
		if exists || revision != 1 {
			return ports.ConflictV1("first publish requires empty current and revision one")
		}
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM organization_fact_history WHERE kind=? AND tenant_id=? AND id=?", kind, string(tenant), id).Scan(&count); err != nil {
			return mapErr(ctx, err, true)
		}
		if count != 0 {
			return ports.ConflictV1("first publish cannot reuse history")
		}
	} else {
		if !exists || curRev != uint64(expected.revision) || curDigest != string(expected.digest) {
			return ports.ConflictV1("current full ref CAS failed")
		}
		if revision != expected.revision+1 {
			return ports.ConflictV1("revision must increase by exactly one")
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO organization_fact_history(kind,tenant_id,id,revision,digest,canonical_json,created_unix_nano,updated_unix_nano,expires_unix_nano) VALUES(?,?,?,?,?,?,?,?,?)", kind, string(tenant), id, uint64(revision), string(digest), payload, created, updated, expires); err != nil {
		return mapErr(ctx, err, true)
	}
	if expected == nil {
		_, err = tx.ExecContext(ctx, "INSERT INTO organization_fact_current(kind,tenant_id,id,revision,digest) VALUES(?,?,?,?,?)", kind, string(tenant), id, uint64(revision), string(digest))
	} else {
		var res sql.Result
		res, err = tx.ExecContext(ctx, "UPDATE organization_fact_current SET revision=?,digest=? WHERE kind=? AND tenant_id=? AND id=? AND revision=? AND digest=?", uint64(revision), string(digest), kind, string(tenant), id, uint64(expected.revision), string(expected.digest))
		if err == nil {
			var n int64
			n, err = res.RowsAffected()
			if err == nil && n != 1 {
				return ports.ConflictV1("current CAS lost")
			}
		}
	}
	if err != nil {
		return mapErr(ctx, err, true)
	}
	if err = tx.Commit(); err != nil {
		return ports.IndeterminateV1("organization fact commit outcome unknown")
	}
	return nil
}

func projectionCurrentTx(ctx context.Context, tx *sql.Tx, v contract.ReviewEligibilityCurrentProjectionV1) bool {
	check := func(kind string, tenant core.TenantID, id string, revision core.Revision, digest core.Digest) bool {
		var n int
		return tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM organization_fact_current WHERE kind=? AND tenant_id=? AND id=? AND revision=? AND digest=?", kind, string(tenant), id, uint64(revision), string(digest)).Scan(&n) == nil && n == 1
	}
	if !check("identity", v.Identity.TenantID, v.Identity.ID, v.Identity.Revision, v.Identity.Digest) || !check("identity", v.ResponsibilityIdentity.TenantID, v.ResponsibilityIdentity.ID, v.ResponsibilityIdentity.Revision, v.ResponsibilityIdentity.Digest) || !check("responsibility", v.Responsibility.TenantID, v.Responsibility.ID, v.Responsibility.Revision, v.Responsibility.Digest) {
		return false
	}
	for _, x := range v.Roles {
		if !check("role", x.TenantID, x.ID, x.Revision, x.Digest) {
			return false
		}
	}
	if v.Delegation != nil {
		return v.DelegatorIdentity != nil && check("delegation", v.Delegation.TenantID, v.Delegation.ID, v.Delegation.Revision, v.Delegation.Digest) && check("identity", v.DelegatorIdentity.TenantID, v.DelegatorIdentity.ID, v.DelegatorIdentity.Revision, v.DelegatorIdentity.Digest)
	}
	return true
}

func ptr(v any) *ref {
	switch x := v.(type) {
	case *contract.IdentityRefV1:
		if x == nil {
			return nil
		}
		return &ref{x.Revision, x.Digest}
	case *contract.RoleGrantRefV1:
		if x == nil {
			return nil
		}
		return &ref{x.Revision, x.Digest}
	case *contract.DelegationRefV1:
		if x == nil {
			return nil
		}
		return &ref{x.Revision, x.Digest}
	case *contract.ResponsibilityRefV1:
		if x == nil {
			return nil
		}
		return &ref{x.Revision, x.Digest}
	}
	return nil
}
func (s *Store) PublishIdentityV1(c context.Context, e *contract.IdentityRefV1, v contract.IdentityFactV1) error {
	if err := v.Validate(); err != nil {
		return err
	}
	return s.publish(c, "identity", v.TenantID, v.ID, v.Revision, v.Digest, ptr(e), v, v.CreatedUnixNano, v.UpdatedUnixNano, v.ExpiresUnixNano)
}
func (s *Store) PublishRoleGrantV1(c context.Context, e *contract.RoleGrantRefV1, v contract.RoleGrantFactV1) error {
	if err := v.Validate(); err != nil {
		return err
	}
	return s.publish(c, "role", v.TenantID, v.ID, v.Revision, v.Digest, ptr(e), v, v.CreatedUnixNano, v.UpdatedUnixNano, v.ExpiresUnixNano)
}
func (s *Store) PublishDelegationV1(c context.Context, e *contract.DelegationRefV1, v contract.DelegationFactV1) error {
	if err := v.Validate(); err != nil {
		return err
	}
	return s.publish(c, "delegation", v.TenantID, v.ID, v.Revision, v.Digest, ptr(e), v, v.CreatedUnixNano, v.UpdatedUnixNano, v.ExpiresUnixNano)
}
func (s *Store) PublishResponsibilityV1(c context.Context, e *contract.ResponsibilityRefV1, v contract.ResponsibilityFactV1) error {
	if err := v.Validate(); err != nil {
		return err
	}
	return s.publish(c, "responsibility", v.TenantID, v.ID, v.Revision, v.Digest, ptr(e), v, v.CreatedUnixNano, v.UpdatedUnixNano, v.ExpiresUnixNano)
}

type rower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadExact(ctx context.Context, q rower, kind string, tenant core.TenantID, id string, revision core.Revision, digest core.Digest, out any) error {
	var payload []byte
	var got string
	if err := q.QueryRowContext(ctx, "SELECT canonical_json,digest FROM organization_fact_history WHERE kind=? AND tenant_id=? AND id=? AND revision=?", kind, string(tenant), id, uint64(revision)).Scan(&payload, &got); errors.Is(err, sql.ErrNoRows) {
		return ports.NotFoundV1("organization exact fact not found")
	} else if err != nil {
		return mapErr(ctx, err, false)
	}
	if got != string(digest) {
		return ports.ConflictV1("organization exact digest drifted")
	}
	if err := core.DecodeStrictJSON(payload, out); err != nil {
		return core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "organization stored fact invalid")
	}
	return nil
}
func loadCurrent(ctx context.Context, q rower, kind string, tenant core.TenantID, id string, out any) error {
	var payload []byte
	var digest string
	if err := q.QueryRowContext(ctx, "SELECT h.canonical_json,c.digest FROM organization_fact_current c JOIN organization_fact_history h ON h.kind=c.kind AND h.tenant_id=c.tenant_id AND h.id=c.id AND h.revision=c.revision AND h.digest=c.digest WHERE c.kind=? AND c.tenant_id=? AND c.id=?", kind, string(tenant), id).Scan(&payload, &digest); errors.Is(err, sql.ErrNoRows) {
		return ports.NotFoundV1("organization current fact not found")
	} else if err != nil {
		return mapErr(ctx, err, false)
	}
	if err := core.DecodeStrictJSON(payload, out); err != nil {
		return core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "organization current fact invalid")
	}
	return nil
}
func (s *Store) InspectIdentityV1(c context.Context, r contract.IdentityRefV1) (v contract.IdentityFactV1, e error) {
	if e = r.Validate(); e != nil {
		return
	}
	e = loadExact(c, s.db, "identity", r.TenantID, r.ID, r.Revision, r.Digest, &v)
	if e == nil {
		e = v.Validate()
	}
	return
}
func (s *Store) InspectRoleGrantV1(c context.Context, r contract.RoleGrantRefV1) (v contract.RoleGrantFactV1, e error) {
	if e = r.Validate(); e != nil {
		return
	}
	e = loadExact(c, s.db, "role", r.TenantID, r.ID, r.Revision, r.Digest, &v)
	if e == nil {
		e = v.Validate()
	}
	return
}
func (s *Store) InspectDelegationV1(c context.Context, r contract.DelegationRefV1) (v contract.DelegationFactV1, e error) {
	if e = r.Validate(); e != nil {
		return
	}
	e = loadExact(c, s.db, "delegation", r.TenantID, r.ID, r.Revision, r.Digest, &v)
	if e == nil {
		e = v.Validate()
	}
	return
}
func (s *Store) InspectResponsibilityV1(c context.Context, r contract.ResponsibilityRefV1) (v contract.ResponsibilityFactV1, e error) {
	if e = r.Validate(); e != nil {
		return
	}
	e = loadExact(c, s.db, "responsibility", r.TenantID, r.ID, r.Revision, r.Digest, &v)
	if e == nil {
		e = v.Validate()
	}
	return
}

func (s *Store) ReadReviewEligibilityClosureV1(ctx context.Context, source contract.ReviewEligibilitySourceV1) (ports.ReviewEligibilityClosureV1, error) {
	identityID, roleIDs, delegationID, responsibilityID, err := ports.StableIDsForSourceV1(source)
	if err != nil {
		return ports.ReviewEligibilityClosureV1{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelSerializable})
	if err != nil {
		return ports.ReviewEligibilityClosureV1{}, mapErr(ctx, err, false)
	}
	defer tx.Rollback()
	var out ports.ReviewEligibilityClosureV1
	if err = loadCurrent(ctx, tx, "identity", source.TenantID, identityID, &out.Identity); err != nil {
		return out, err
	}
	out.Roles = make([]contract.RoleGrantFactV1, len(roleIDs))
	for i, id := range roleIDs {
		if err = loadCurrent(ctx, tx, "role", source.TenantID, id, &out.Roles[i]); err != nil {
			return ports.ReviewEligibilityClosureV1{}, err
		}
	}
	if source.RequireDelegation {
		var d contract.DelegationFactV1
		if err = loadCurrent(ctx, tx, "delegation", source.TenantID, delegationID, &d); err != nil {
			return ports.ReviewEligibilityClosureV1{}, err
		}
		out.Delegation = &d
		var di contract.IdentityFactV1
		if err = loadCurrent(ctx, tx, "identity", source.TenantID, d.Delegator.ID, &di); err != nil {
			return ports.ReviewEligibilityClosureV1{}, err
		}
		out.DelegatorIdentity = &di
	}
	if err = loadCurrent(ctx, tx, "responsibility", source.TenantID, responsibilityID, &out.Responsibility); err != nil {
		return ports.ReviewEligibilityClosureV1{}, err
	}
	if err = loadCurrent(ctx, tx, "identity", source.TenantID, out.Responsibility.Identity.ID, &out.ResponsibilityIdentity); err != nil {
		return ports.ReviewEligibilityClosureV1{}, err
	}
	if err = tx.Commit(); err != nil {
		return ports.ReviewEligibilityClosureV1{}, ports.IndeterminateV1("organization closure read outcome unknown")
	}
	return out.Clone(), nil
}

func (s *Store) CreateOrInspectReviewEligibilityProjectionV1(ctx context.Context, v contract.ReviewEligibilityCurrentProjectionV1) (contract.ReviewEligibilityCurrentProjectionV1, error) {
	if err := v.Validate(); err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	payload, err := json.Marshal(v)
	if err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "organization projection encode failed")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, mapErr(ctx, err, true)
	}
	defer tx.Rollback()
	if !projectionCurrentTx(ctx, tx, v) {
		return contract.ReviewEligibilityCurrentProjectionV1{}, ports.ConflictV1("projection closure is not current at publish")
	}
	var old []byte
	var digest string
	err = tx.QueryRowContext(ctx, "SELECT canonical_json,digest FROM organization_review_projection WHERE tenant_id=? AND id=?", string(v.Ref.TenantID), v.Ref.ID).Scan(&old, &digest)
	if err == nil {
		var existing contract.ReviewEligibilityCurrentProjectionV1
		if err = core.DecodeStrictJSON(old, &existing); err != nil {
			return contract.ReviewEligibilityCurrentProjectionV1{}, core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "organization projection row invalid")
		}
		if existing.Ref.ID != v.Ref.ID || !sameClosure(existing, v) {
			return contract.ReviewEligibilityCurrentProjectionV1{}, ports.ConflictV1("projection id carries different closure")
		}
		return existing.Clone(), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return contract.ReviewEligibilityCurrentProjectionV1{}, mapErr(ctx, err, true)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO organization_review_projection(tenant_id,id,digest,canonical_json,checked_unix_nano,expires_unix_nano) VALUES(?,?,?,?,?,?)", string(v.Ref.TenantID), v.Ref.ID, string(v.ProjectionDigest), payload, v.CheckedUnixNano, v.ExpiresUnixNano); err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, mapErr(ctx, err, true)
	}
	if err = tx.Commit(); err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, ports.IndeterminateV1("organization projection commit outcome unknown")
	}
	return v.Clone(), nil
}
func sameClosure(a, b contract.ReviewEligibilityCurrentProjectionV1) bool {
	if a.Ref.Identity != b.Ref.Identity || a.Ref.Responsibility != b.Ref.Responsibility || len(a.Ref.Roles) != len(b.Ref.Roles) || (a.Ref.Delegation == nil) != (b.Ref.Delegation == nil) {
		return false
	}
	for i := range a.Ref.Roles {
		if a.Ref.Roles[i] != b.Ref.Roles[i] {
			return false
		}
	}
	return a.Ref.Delegation == nil || *a.Ref.Delegation == *b.Ref.Delegation
}
func (s *Store) InspectReviewEligibilityProjectionV1(ctx context.Context, r contract.ReviewEligibilityProjectionRefV1) (contract.ReviewEligibilityCurrentProjectionV1, error) {
	if err := r.Validate(); err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, err
	}
	var payload []byte
	var digest string
	if err := s.db.QueryRowContext(ctx, "SELECT canonical_json,digest FROM organization_review_projection WHERE tenant_id=? AND id=?", string(r.TenantID), r.ID).Scan(&payload, &digest); errors.Is(err, sql.ErrNoRows) {
		return contract.ReviewEligibilityCurrentProjectionV1{}, ports.NotFoundV1("organization projection not found")
	} else if err != nil {
		return contract.ReviewEligibilityCurrentProjectionV1{}, mapErr(ctx, err, false)
	}
	if digest != string(r.Digest) {
		return contract.ReviewEligibilityCurrentProjectionV1{}, ports.ConflictV1("organization projection exact digest drifted")
	}
	var out contract.ReviewEligibilityCurrentProjectionV1
	if err := core.DecodeStrictJSON(payload, &out); err != nil {
		return out, core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "organization projection row invalid")
	}
	if err := out.Validate(); err != nil {
		return out, err
	}
	return out.Clone(), nil
}

func mapErr(ctx context.Context, err error, mutation bool) error {
	if ctx != nil && ctx.Err() != nil {
		return ports.IndeterminateV1("organization sqlite context ended")
	}
	if mutation {
		return ports.IndeterminateV1("organization sqlite mutation outcome unknown")
	}
	return core.NewError(core.ErrorUnavailable, core.ReasonOwnerMissing, "organization sqlite unavailable")
}
