// Package sqlite provides the default durable metadata backend for Continuity.
// It stores Continuity-owned metadata and projections only; content blobs,
// Runtime facts, Restore execution, and external effects remain outside it.
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
	"sort"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	_ "modernc.org/sqlite"
)

const schemaVersion = 9

type Store struct {
	db    *sql.DB
	clock func() time.Time
}

// Open opens or creates a durable SQLite metadata store. Every pooled
// connection receives the same safety settings; writes use BEGIN IMMEDIATE so
// compare-and-swap decisions are serialized by SQLite rather than by process
// memory.
func Open(ctx context.Context, path string) (*Store, error) {
	return OpenWithClock(ctx, path, time.Now)
}

func OpenWithClock(ctx context.Context, path string, clock func() time.Time) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, contract.NewError(contract.ErrInvalidArgument, "sqlite_path", "path is required")
	}
	if clock == nil {
		return nil, contract.NewError(contract.ErrInvalidArgument, "clock", "clock is required")
	}
	db, err := sql.Open("sqlite", dataSource(path))
	if err != nil {
		return nil, unavailable("open", err)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(16)
	store := &Store{db: db, clock: clock}
	if err := store.initialize(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func dataSource(path string) string {
	u := &url.URL{Scheme: "file", Path: path}
	q := u.Query()
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(FULL)")
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Stats() sql.DBStats { return s.db.Stats() }

func (s *Store) initialize(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return unavailable("inspect schema version", err)
	}
	if version < 0 || version > schemaVersion {
		return contract.NewError(contract.ErrUnsupported, "sqlite_schema_version", fmt.Sprintf("database version %d is not supported", version))
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable("begin schema transaction", err)
	}
	defer tx.Rollback()
	for _, statement := range schemaStatements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return unavailable("create schema", err)
		}
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", schemaVersion)); err != nil {
		return unavailable("set schema version", err)
	}
	if err := tx.Commit(); err != nil {
		return unavailable("commit schema", err)
	}
	return nil
}

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS journal_history (
		journal_id TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		body BLOB NOT NULL, PRIMARY KEY(journal_id, revision))`,
	`CREATE TABLE IF NOT EXISTS journal_current (
		journal_id TEXT PRIMARY KEY, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS objects (
		object_id TEXT PRIMARY KEY, manifest_digest TEXT NOT NULL,
		content_digest TEXT NOT NULL, body BLOB NOT NULL,
		committed INTEGER NOT NULL, visible INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS retention_history (
		object_id TEXT NOT NULL, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		body BLOB NOT NULL, PRIMARY KEY(object_id, revision))`,
	`CREATE TABLE IF NOT EXISTS retention_current (
		object_id TEXT PRIMARY KEY, revision INTEGER NOT NULL, digest TEXT NOT NULL,
		body BLOB NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS timeline_events (
		evidence_ref TEXT PRIMARY KEY, ledger_scope TEXT NOT NULL,
		ledger_sequence INTEGER NOT NULL, registration_id TEXT NOT NULL,
		source_epoch INTEGER NOT NULL, source_sequence INTEGER NOT NULL,
		evidence_digest TEXT NOT NULL, projection_digest TEXT NOT NULL,
		body BLOB NOT NULL,
		UNIQUE(ledger_scope, ledger_sequence),
		UNIQUE(ledger_scope, registration_id, source_epoch, source_sequence))`,
	`CREATE INDEX IF NOT EXISTS timeline_scope_sequence
		ON timeline_events(ledger_scope, ledger_sequence)`,
	`CREATE TABLE IF NOT EXISTS timeline_tombstones (
		tombstone_id TEXT PRIMARY KEY, evidence_ref TEXT NOT NULL UNIQUE,
		scope_digest TEXT NOT NULL, digest TEXT NOT NULL, body BLOB NOT NULL,
		FOREIGN KEY(evidence_ref) REFERENCES timeline_events(evidence_ref))`,
	`CREATE TABLE IF NOT EXISTS timeline_attempt_history (
		scope_digest TEXT NOT NULL, attempt_id TEXT NOT NULL,
		revision INTEGER NOT NULL, ref_digest TEXT NOT NULL,
		request_digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(scope_digest,attempt_id,revision))`,
	`CREATE TABLE IF NOT EXISTS timeline_attempt_current (
		scope_digest TEXT NOT NULL, attempt_id TEXT NOT NULL,
		revision INTEGER NOT NULL,
		PRIMARY KEY(scope_digest,attempt_id))`,
	`CREATE TABLE IF NOT EXISTS timeline_attempt_idempotency (
		scope_digest TEXT NOT NULL, idempotency_key TEXT NOT NULL,
		attempt_id TEXT NOT NULL,
		PRIMARY KEY(scope_digest,idempotency_key))`,
	`CREATE TABLE IF NOT EXISTS timeline_projection_current (
		ledger_scope TEXT NOT NULL, evidence_ref TEXT NOT NULL,
		digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(ledger_scope,evidence_ref),
		FOREIGN KEY(evidence_ref) REFERENCES timeline_events(evidence_ref))`,
	`CREATE TABLE IF NOT EXISTS timeline_policy_history (
		scope_digest TEXT NOT NULL, policy_id TEXT NOT NULL,
		revision INTEGER NOT NULL, ref_digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(scope_digest,policy_id,revision))`,
	`CREATE TABLE IF NOT EXISTS timeline_policy_current (
		scope_digest TEXT NOT NULL, policy_id TEXT NOT NULL,
		revision INTEGER NOT NULL,
		PRIMARY KEY(scope_digest,policy_id))`,
	`CREATE TABLE IF NOT EXISTS checkpoint_manifest_history (
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, manifest_id TEXT NOT NULL,
		revision INTEGER NOT NULL, ref_digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(tenant_id,scope_digest,manifest_id,revision))`,
	`CREATE TABLE IF NOT EXISTS checkpoint_manifest_current (
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, manifest_id TEXT NOT NULL,
		revision INTEGER NOT NULL,
		PRIMARY KEY(tenant_id,scope_digest,manifest_id))`,
	`CREATE TABLE IF NOT EXISTS checkpoint_manifest_idempotency (
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, idempotency_key TEXT NOT NULL,
		manifest_id TEXT NOT NULL,
		PRIMARY KEY(tenant_id,scope_digest,idempotency_key))`,
	`CREATE TABLE IF NOT EXISTS checkpoint_manifest_seals (
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, seal_id TEXT NOT NULL,
		idempotency_key TEXT NOT NULL, manifest_identity_digest TEXT NOT NULL,
		ref_digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(tenant_id,scope_digest,seal_id),
		UNIQUE(tenant_id,scope_digest,idempotency_key),
		UNIQUE(manifest_identity_digest))`,
	`CREATE TABLE IF NOT EXISTS restore_plan_history (
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, plan_id TEXT NOT NULL,
		revision INTEGER NOT NULL, ref_digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(tenant_id,scope_digest,plan_id,revision))`,
	`CREATE TABLE IF NOT EXISTS restore_plan_current (
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, plan_id TEXT NOT NULL,
		revision INTEGER NOT NULL,
		PRIMARY KEY(tenant_id,scope_digest,plan_id))`,
	`CREATE TABLE IF NOT EXISTS restore_plan_idempotency (
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, idempotency_key TEXT NOT NULL,
		plan_id TEXT NOT NULL,
		PRIMARY KEY(tenant_id,scope_digest,idempotency_key))`,
	`CREATE TABLE IF NOT EXISTS rewind_plan_history (
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, plan_id TEXT NOT NULL,
		revision INTEGER NOT NULL, ref_digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(tenant_id,scope_digest,plan_id,revision))`,
	`CREATE TABLE IF NOT EXISTS rewind_plan_current (
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, plan_id TEXT NOT NULL,
		revision INTEGER NOT NULL,
		PRIMARY KEY(tenant_id,scope_digest,plan_id))`,
	`CREATE TABLE IF NOT EXISTS rewind_plan_idempotency (
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, idempotency_key TEXT NOT NULL,
		plan_id TEXT NOT NULL,
		PRIMARY KEY(tenant_id,scope_digest,idempotency_key))`,
	`CREATE TABLE IF NOT EXISTS artifact_relation_facts (
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, relation_id TEXT NOT NULL,
		idempotency_key TEXT NOT NULL, artifact_identity_digest TEXT NOT NULL,
		related_identity_digest TEXT NOT NULL, ref_digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(tenant_id,scope_digest,relation_id),
		UNIQUE(tenant_id,scope_digest,idempotency_key))`,
	`CREATE INDEX IF NOT EXISTS artifact_relation_by_artifact
		ON artifact_relation_facts(artifact_identity_digest,tenant_id,scope_digest,relation_id)`,
	`CREATE INDEX IF NOT EXISTS artifact_relation_by_related
		ON artifact_relation_facts(related_identity_digest,tenant_id,scope_digest,relation_id)`,
	`CREATE TABLE IF NOT EXISTS content_integrity_audit_facts (
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, audit_id TEXT NOT NULL,
		idempotency_key TEXT NOT NULL, request_digest TEXT NOT NULL,
		ref_digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(tenant_id,scope_digest,audit_id),
		UNIQUE(tenant_id,scope_digest,idempotency_key))`,
	`CREATE TABLE IF NOT EXISTS content_delta_facts (
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, delta_id TEXT NOT NULL,
		idempotency_key TEXT NOT NULL, request_digest TEXT NOT NULL,
		base_object_id TEXT NOT NULL, target_object_id TEXT NOT NULL,
		ref_digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(tenant_id,scope_digest,delta_id),
		UNIQUE(tenant_id,scope_digest,idempotency_key))`,
	`CREATE TABLE IF NOT EXISTS history_derivation_candidate_facts (
		tenant_id TEXT NOT NULL, scope_digest TEXT NOT NULL, candidate_id TEXT NOT NULL,
		idempotency_key TEXT NOT NULL, request_digest TEXT NOT NULL,
		kind TEXT NOT NULL, source_set_digest TEXT NOT NULL, output_object_id TEXT NOT NULL,
		ref_digest TEXT NOT NULL, body BLOB NOT NULL,
		PRIMARY KEY(tenant_id,scope_digest,candidate_id),
		UNIQUE(tenant_id,scope_digest,idempotency_key))`,
}

func encode(value any) ([]byte, string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, "", contract.NewError(contract.ErrInvalidArgument, "canonical_body", err.Error())
	}
	return body, contract.DigestBytes(body), nil
}

func decode(body []byte, target any) error {
	if len(body) == 0 {
		return contract.NewError(contract.ErrContentDigestMismatch, "stored_body", "stored canonical body is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	decoder = json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return contract.NewError(contract.ErrContentDigestMismatch, "stored_body", "stored canonical body cannot be decoded")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return contract.NewError(contract.ErrContentDigestMismatch, "stored_body", "stored canonical body contains trailing data")
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return contract.NewError(contract.ErrContentDigestMismatch, "stored_body", "stored canonical body is invalid JSON")
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return contract.NewError(contract.ErrContentDigestMismatch, "stored_body", "stored canonical body has an invalid object key")
			}
			key, ok := keyToken.(string)
			if !ok {
				return contract.NewError(contract.ErrContentDigestMismatch, "stored_body", "stored canonical body has a non-string object key")
			}
			if _, duplicate := seen[key]; duplicate {
				return contract.NewError(contract.ErrContentDigestMismatch, "stored_body", "stored canonical body contains a duplicate key")
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return contract.NewError(contract.ErrContentDigestMismatch, "stored_body", "stored canonical body has an unterminated object")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return contract.NewError(contract.ErrContentDigestMismatch, "stored_body", "stored canonical body has an unterminated array")
		}
	default:
		return contract.NewError(contract.ErrContentDigestMismatch, "stored_body", "stored canonical body has an invalid delimiter")
	}
	return nil
}

func (s *Store) validateCall(ctx context.Context) error {
	if s == nil || s.db == nil {
		return contract.NewError(contract.ErrInvalidArgument, "sqlite_store", "SQLite Store is unavailable")
	}
	if ctx == nil {
		return contract.NewError(contract.ErrInvalidArgument, "context", "context is required")
	}
	return nil
}

func unavailable(operation string, err error) error {
	return contract.NewError(contract.ErrUnavailable, "sqlite", operation+": "+err.Error())
}

func notFound(field, message string) error {
	return contract.NewError(contract.ErrNotFound, field, message)
}

func (s *Store) CreateJournal(ctx context.Context, journal contract.WriteJournal) error {
	if err := journal.Validate(); err != nil {
		return err
	}
	body, digest, err := encode(journal)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable("begin journal create", err)
	}
	defer tx.Rollback()
	var currentDigest string
	err = tx.QueryRowContext(ctx, "SELECT digest FROM journal_current WHERE journal_id=?", journal.JournalID).Scan(&currentDigest)
	switch {
	case err == nil && currentDigest == digest:
		return tx.Commit()
	case err == nil:
		return contract.NewError(contract.ErrRevisionConflict, "journal_id", "create-once journal changed content")
	case !errors.Is(err, sql.ErrNoRows):
		return unavailable("inspect journal create", err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO journal_history(journal_id,revision,digest,body) VALUES(?,?,?,?)", journal.JournalID, journal.Revision, digest, body); err != nil {
		return unavailable("insert journal history", err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO journal_current(journal_id,revision,digest,body) VALUES(?,?,?,?)", journal.JournalID, journal.Revision, digest, body); err != nil {
		return unavailable("insert journal current", err)
	}
	if err = tx.Commit(); err != nil {
		return unavailable("commit journal create", err)
	}
	return nil
}

func (s *Store) CASJournal(ctx context.Context, expected uint64, next contract.WriteJournal) error {
	if err := next.Validate(); err != nil {
		return err
	}
	body, digest, err := encode(next)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable("begin journal CAS", err)
	}
	defer tx.Rollback()
	current, err := inspectJournalTx(ctx, tx, next.JournalID)
	if err != nil {
		return err
	}
	if current.Revision != expected || next.Revision != expected+1 {
		return contract.NewError(contract.ErrRevisionConflict, "journal_revision", "CAS mismatch")
	}
	if current.ObjectID != next.ObjectID || current.ManifestDigest != next.ManifestDigest || current.ObjectDigest != next.ObjectDigest {
		return contract.NewError(contract.ErrRevisionConflict, "journal_identity", "immutable identity changed")
	}
	if err := contract.AdvanceJournal(current.State, next.State); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO journal_history(journal_id,revision,digest,body) VALUES(?,?,?,?)", next.JournalID, next.Revision, digest, body); err != nil {
		return unavailable("insert journal history", err)
	}
	result, err := tx.ExecContext(ctx, "UPDATE journal_current SET revision=?,digest=?,body=? WHERE journal_id=? AND revision=?", next.Revision, digest, body, next.JournalID, expected)
	if err != nil {
		return unavailable("update journal current", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return contract.NewError(contract.ErrRevisionConflict, "journal_revision", "CAS lost")
	}
	if err = tx.Commit(); err != nil {
		return unavailable("commit journal CAS", err)
	}
	return nil
}

func inspectJournalTx(ctx context.Context, tx *sql.Tx, id string) (contract.WriteJournal, error) {
	var body []byte
	if err := tx.QueryRowContext(ctx, "SELECT body FROM journal_current WHERE journal_id=?", id).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WriteJournal{}, notFound("journal_id", "journal not found")
		}
		return contract.WriteJournal{}, unavailable("inspect journal", err)
	}
	var journal contract.WriteJournal
	if err := decode(body, &journal); err != nil {
		return contract.WriteJournal{}, err
	}
	if err := journal.Validate(); err != nil {
		return contract.WriteJournal{}, contract.NewError(contract.ErrContentDigestMismatch, "journal", "stored journal failed validation")
	}
	return journal, nil
}

func (s *Store) InspectJournal(ctx context.Context, id string) (contract.WriteJournal, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.WriteJournal{}, err
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, "SELECT body FROM journal_current WHERE journal_id=?", id).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WriteJournal{}, notFound("journal_id", "journal not found")
		}
		return contract.WriteJournal{}, unavailable("inspect journal", err)
	}
	var journal contract.WriteJournal
	if err := decode(body, &journal); err != nil {
		return contract.WriteJournal{}, err
	}
	if err := journal.Validate(); err != nil {
		return contract.WriteJournal{}, contract.NewError(contract.ErrContentDigestMismatch, "journal", "stored journal failed validation")
	}
	return journal, nil
}

func (s *Store) StageManifest(ctx context.Context, manifest contract.ObjectManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	body, _, err := encode(manifest)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable("begin manifest stage", err)
	}
	defer tx.Rollback()
	var digest string
	err = tx.QueryRowContext(ctx, "SELECT manifest_digest FROM objects WHERE object_id=?", manifest.ObjectID).Scan(&digest)
	switch {
	case err == nil && digest == manifest.Digest:
		return tx.Commit()
	case err == nil:
		return contract.NewError(contract.ErrRevisionConflict, "object_id", "manifest changed")
	case !errors.Is(err, sql.ErrNoRows):
		return unavailable("inspect manifest stage", err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO objects(object_id,manifest_digest,content_digest,body,committed,visible) VALUES(?,?,?,?,0,0)", manifest.ObjectID, manifest.Digest, manifest.ContentDigest, body); err != nil {
		return unavailable("insert manifest", err)
	}
	if err = tx.Commit(); err != nil {
		return unavailable("commit manifest stage", err)
	}
	return nil
}

func (s *Store) CommitObjectReference(ctx context.Context, objectID, digest string) error {
	result, err := s.db.ExecContext(ctx, "UPDATE objects SET committed=1 WHERE object_id=? AND content_digest=?", objectID, digest)
	if err != nil {
		return unavailable("commit object reference", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		var existing string
		if err := s.db.QueryRowContext(ctx, "SELECT content_digest FROM objects WHERE object_id=?", objectID).Scan(&existing); errors.Is(err, sql.ErrNoRows) {
			return notFound("object_id", "manifest not staged")
		}
		return contract.NewError(contract.ErrContentDigestMismatch, "content_digest", "reference digest mismatch")
	}
	return nil
}

func (s *Store) SetObjectVisible(ctx context.Context, objectID string, visible bool) error {
	value := 0
	if visible {
		value = 1
	}
	result, err := s.db.ExecContext(ctx, "UPDATE objects SET visible=? WHERE object_id=? AND committed=1", value, objectID)
	if err != nil {
		return unavailable("set object visibility", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return contract.NewError(contract.ErrCrossStoreIndeterminate, "object_id", "reference is not committed")
	}
	return nil
}

func (s *Store) InspectObject(ctx context.Context, objectID string) (contract.ObjectManifest, bool, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.ObjectManifest{}, false, err
	}
	var body []byte
	var visible int
	if err := s.db.QueryRowContext(ctx, "SELECT body,visible FROM objects WHERE object_id=?", objectID).Scan(&body, &visible); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.ObjectManifest{}, false, notFound("object_id", "object not found")
		}
		return contract.ObjectManifest{}, false, unavailable("inspect object", err)
	}
	var manifest contract.ObjectManifest
	if err := decode(body, &manifest); err != nil {
		return contract.ObjectManifest{}, false, err
	}
	if err := manifest.Validate(); err != nil {
		return contract.ObjectManifest{}, false, contract.NewError(contract.ErrContentDigestMismatch, "manifest", "stored manifest failed validation")
	}
	return manifest, visible == 1, nil
}

func (s *Store) CreateRetention(ctx context.Context, fact contract.RetentionFact) error {
	if err := fact.Validate(); err != nil {
		return err
	}
	body, digest, err := encode(fact)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable("begin retention create", err)
	}
	defer tx.Rollback()
	var currentDigest string
	err = tx.QueryRowContext(ctx, "SELECT digest FROM retention_current WHERE object_id=?", fact.ObjectID).Scan(&currentDigest)
	switch {
	case err == nil && currentDigest == digest:
		return tx.Commit()
	case err == nil:
		return contract.NewError(contract.ErrRevisionConflict, "object_id", "create-once retention changed content")
	case !errors.Is(err, sql.ErrNoRows):
		return unavailable("inspect retention create", err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO retention_history(object_id,revision,digest,body) VALUES(?,?,?,?)", fact.ObjectID, fact.Revision, digest, body); err != nil {
		return unavailable("insert retention history", err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO retention_current(object_id,revision,digest,body) VALUES(?,?,?,?)", fact.ObjectID, fact.Revision, digest, body); err != nil {
		return unavailable("insert retention current", err)
	}
	if err = tx.Commit(); err != nil {
		return unavailable("commit retention create", err)
	}
	return nil
}

func (s *Store) CASRetention(ctx context.Context, expected uint64, next contract.RetentionFact) error {
	if err := next.Validate(); err != nil {
		return err
	}
	body, digest, err := encode(next)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return unavailable("begin retention CAS", err)
	}
	defer tx.Rollback()
	current, err := inspectRetentionTx(ctx, tx, next.ObjectID)
	if err != nil {
		return err
	}
	if current.Revision != expected || next.Revision != expected+1 {
		return contract.NewError(contract.ErrRevisionConflict, "retention_revision", "CAS mismatch")
	}
	if current.PolicyRef != next.PolicyRef || current.Classification != next.Classification {
		return contract.NewError(contract.ErrRevisionConflict, "retention_identity", "immutable retention identity changed")
	}
	expectedNext, err := contract.AdvanceRetention(current, next.State, next.TransitionEvidenceRef)
	if err != nil {
		return err
	}
	if expectedNext.State != next.State || expectedNext.PreviousState != next.PreviousState ||
		expectedNext.TombstoneRef != next.TombstoneRef || expectedNext.HoldRef != next.HoldRef ||
		expectedNext.TransitionEvidenceRef != next.TransitionEvidenceRef || next.UpdatedUnixNano < current.UpdatedUnixNano {
		return contract.NewError(contract.ErrRevisionConflict, "retention_transition", "next fact does not match the owner transition")
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO retention_history(object_id,revision,digest,body) VALUES(?,?,?,?)", next.ObjectID, next.Revision, digest, body); err != nil {
		return unavailable("insert retention history", err)
	}
	result, err := tx.ExecContext(ctx, "UPDATE retention_current SET revision=?,digest=?,body=? WHERE object_id=? AND revision=?", next.Revision, digest, body, next.ObjectID, expected)
	if err != nil {
		return unavailable("update retention current", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return contract.NewError(contract.ErrRevisionConflict, "retention_revision", "CAS lost")
	}
	if err = tx.Commit(); err != nil {
		return unavailable("commit retention CAS", err)
	}
	return nil
}

func inspectRetentionTx(ctx context.Context, tx *sql.Tx, objectID string) (contract.RetentionFact, error) {
	var body []byte
	if err := tx.QueryRowContext(ctx, "SELECT body FROM retention_current WHERE object_id=?", objectID).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.RetentionFact{}, notFound("object_id", "retention fact not found")
		}
		return contract.RetentionFact{}, unavailable("inspect retention", err)
	}
	var fact contract.RetentionFact
	if err := decode(body, &fact); err != nil {
		return contract.RetentionFact{}, err
	}
	if err := fact.Validate(); err != nil {
		return contract.RetentionFact{}, contract.NewError(contract.ErrContentDigestMismatch, "retention", "stored retention fact failed validation")
	}
	return fact, nil
}

func (s *Store) InspectRetention(ctx context.Context, objectID string) (contract.RetentionFact, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.RetentionFact{}, err
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, "SELECT body FROM retention_current WHERE object_id=?", objectID).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.RetentionFact{}, notFound("object_id", "retention fact not found")
		}
		return contract.RetentionFact{}, unavailable("inspect retention", err)
	}
	var fact contract.RetentionFact
	if err := decode(body, &fact); err != nil {
		return contract.RetentionFact{}, err
	}
	if err := fact.Validate(); err != nil {
		return contract.RetentionFact{}, contract.NewError(contract.ErrContentDigestMismatch, "retention", "stored retention fact failed validation")
	}
	return fact, nil
}

func (s *Store) PutProjection(ctx context.Context, record contract.TimelineEventRecord) (contract.TimelineEventRecord, bool, error) {
	if err := record.Validate(); err != nil {
		return contract.TimelineEventRecord{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.TimelineEventRecord{}, false, unavailable("begin projection put", err)
	}
	defer tx.Rollback()
	result, duplicate, err := putProjectionTx(ctx, tx, record)
	if err != nil {
		return contract.TimelineEventRecord{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return contract.TimelineEventRecord{}, false, unavailable("commit projection", err)
	}
	return result, duplicate, nil
}

func putProjectionTx(ctx context.Context, tx *sql.Tx, record contract.TimelineEventRecord) (contract.TimelineEventRecord, bool, error) {
	body, _, err := encode(record)
	if err != nil {
		return contract.TimelineEventRecord{}, false, err
	}
	if existing, found, err := inspectProjectionTx(ctx, tx, "evidence_ref=?", record.EvidenceRecordRef); err != nil {
		return contract.TimelineEventRecord{}, false, err
	} else if found {
		return compareProjection(existing, record)
	}
	key := record.Candidate.Evidence.SourceKey
	if existing, found, err := inspectProjectionTx(ctx, tx,
		"ledger_scope=? AND registration_id=? AND source_epoch=? AND source_sequence=?",
		record.LedgerScopeDigest, key.RegistrationID, key.SourceEpoch, key.SourceSequence); err != nil {
		return contract.TimelineEventRecord{}, false, err
	} else if found {
		return compareProjection(existing, record)
	}
	if existing, found, err := inspectProjectionTx(ctx, tx, "ledger_scope=? AND ledger_sequence=?", record.LedgerScopeDigest, record.LedgerSequence); err != nil {
		return contract.TimelineEventRecord{}, false, err
	} else if found {
		return compareProjection(existing, record)
	}
	existing, err := listLedgerScopeTx(ctx, tx, record.LedgerScopeDigest, false)
	if err != nil {
		return contract.TimelineEventRecord{}, false, err
	}
	if projectionCycle(existing, record) {
		return contract.TimelineEventRecord{}, false, contract.NewError(contract.ErrProjectionConflict, "parent_refs", "cycle detected")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO timeline_events(
		evidence_ref,ledger_scope,ledger_sequence,registration_id,source_epoch,source_sequence,
		evidence_digest,projection_digest,body) VALUES(?,?,?,?,?,?,?,?,?)`,
		record.EvidenceRecordRef, record.LedgerScopeDigest, record.LedgerSequence,
		key.RegistrationID, key.SourceEpoch, key.SourceSequence,
		record.EvidenceRecordDigest, record.Candidate.Digest, body)
	if err != nil {
		return contract.TimelineEventRecord{}, false, unavailable("insert projection", err)
	}
	return record.Clone(), false, nil
}

func compareProjection(existing, incoming contract.TimelineEventRecord) (contract.TimelineEventRecord, bool, error) {
	if existing.EvidenceRecordDigest != incoming.EvidenceRecordDigest {
		return contract.TimelineEventRecord{}, false, contract.NewError(contract.ErrEvidenceConflict, "source_key", "same source coordinate changed evidence")
	}
	if existing.Candidate.Digest != incoming.Candidate.Digest {
		return contract.TimelineEventRecord{}, false, contract.NewError(contract.ErrProjectionConflict, "projection", "same evidence changed projection semantics")
	}
	return existing.Clone(), true, nil
}

func inspectProjectionTx(ctx context.Context, tx *sql.Tx, predicate string, args ...any) (contract.TimelineEventRecord, bool, error) {
	var body []byte
	err := tx.QueryRowContext(ctx, "SELECT body FROM timeline_events WHERE "+predicate, args...).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.TimelineEventRecord{}, false, nil
	}
	if err != nil {
		return contract.TimelineEventRecord{}, false, unavailable("inspect projection", err)
	}
	var record contract.TimelineEventRecord
	if err := decode(body, &record); err != nil {
		return contract.TimelineEventRecord{}, false, err
	}
	if err := record.Validate(); err != nil {
		return contract.TimelineEventRecord{}, false, contract.NewError(contract.ErrContentDigestMismatch, "timeline_event", "stored event failed validation")
	}
	return record, true, nil
}

func (s *Store) InspectByEvidence(ctx context.Context, evidenceRef string) (contract.TimelineEventRecord, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.TimelineEventRecord{}, err
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, "SELECT body FROM timeline_events WHERE evidence_ref=?", evidenceRef).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.TimelineEventRecord{}, notFound("evidence_ref", "projection not found")
		}
		return contract.TimelineEventRecord{}, unavailable("inspect projection", err)
	}
	var record contract.TimelineEventRecord
	if err := decode(body, &record); err != nil {
		return contract.TimelineEventRecord{}, err
	}
	if err := record.Validate(); err != nil {
		return contract.TimelineEventRecord{}, contract.NewError(contract.ErrContentDigestMismatch, "timeline_event", "stored event failed validation")
	}
	return record.Clone(), nil
}

func (s *Store) ListLedgerScope(ctx context.Context, scope string) ([]contract.TimelineEventRecord, error) {
	if err := s.validateCall(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT e.body,t.tombstone_id
		FROM timeline_events e LEFT JOIN timeline_tombstones t ON t.evidence_ref=e.evidence_ref
		WHERE e.ledger_scope=? ORDER BY e.ledger_sequence`, scope)
	if err != nil {
		return nil, unavailable("list ledger scope", err)
	}
	defer rows.Close()
	return scanTimelineRows(rows, true)
}

func listLedgerScopeTx(ctx context.Context, tx *sql.Tx, scope string, overlay bool) ([]contract.TimelineEventRecord, error) {
	rows, err := tx.QueryContext(ctx, "SELECT body,NULL FROM timeline_events WHERE ledger_scope=? ORDER BY ledger_sequence", scope)
	if err != nil {
		return nil, unavailable("list ledger scope", err)
	}
	defer rows.Close()
	return scanTimelineRows(rows, overlay)
}

type rowScanner interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func scanTimelineRows(rows rowScanner, overlay bool) ([]contract.TimelineEventRecord, error) {
	result := make([]contract.TimelineEventRecord, 0)
	for rows.Next() {
		var body []byte
		var tombstone sql.NullString
		if err := rows.Scan(&body, &tombstone); err != nil {
			return nil, unavailable("scan timeline event", err)
		}
		var record contract.TimelineEventRecord
		if err := decode(body, &record); err != nil {
			return nil, err
		}
		if err := record.Validate(); err != nil {
			return nil, contract.NewError(contract.ErrContentDigestMismatch, "timeline_event", "stored event failed validation")
		}
		if overlay && tombstone.Valid {
			record.Visibility = "tombstoned"
			record.TombstoneRef = tombstone.String
		}
		result = append(result, record.Clone())
	}
	if err := rows.Err(); err != nil {
		return nil, unavailable("iterate timeline events", err)
	}
	return result, nil
}

func (s *Store) CreateTombstoneOverlay(ctx context.Context, fact contract.TimelineProjectionTombstoneFactV1) (contract.TimelineProjectionTombstoneFactV1, bool, error) {
	if err := fact.Validate(); err != nil {
		return contract.TimelineProjectionTombstoneFactV1{}, false, err
	}
	body, _, err := encode(fact)
	if err != nil {
		return contract.TimelineProjectionTombstoneFactV1{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.TimelineProjectionTombstoneFactV1{}, false, unavailable("begin tombstone create", err)
	}
	defer tx.Rollback()
	var existingBody []byte
	err = tx.QueryRowContext(ctx, "SELECT body FROM timeline_tombstones WHERE tombstone_id=?", fact.TombstoneID).Scan(&existingBody)
	if err == nil {
		var existing contract.TimelineProjectionTombstoneFactV1
		if err := decode(existingBody, &existing); err != nil {
			return contract.TimelineProjectionTombstoneFactV1{}, false, err
		}
		if existing.Digest != fact.Digest {
			return contract.TimelineProjectionTombstoneFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "tombstone_id", "create-once tombstone changed content")
		}
		return existing, true, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return contract.TimelineProjectionTombstoneFactV1{}, false, unavailable("inspect tombstone create", err)
	}
	record, found, err := inspectProjectionTx(ctx, tx, "evidence_ref=?", fact.EvidenceRecordRef)
	if err != nil {
		return contract.TimelineProjectionTombstoneFactV1{}, false, err
	}
	if !found {
		return contract.TimelineProjectionTombstoneFactV1{}, false, notFound("evidence_ref", "projection not found")
	}
	if record.Candidate.Scope.ExecutionScopeDigest != fact.ScopeDigest {
		return contract.TimelineProjectionTombstoneFactV1{}, false, contract.NewError(contract.ErrProjectionConflict, "scope_digest", "tombstone belongs to another execution scope")
	}
	var other string
	if err := tx.QueryRowContext(ctx, "SELECT tombstone_id FROM timeline_tombstones WHERE evidence_ref=?", fact.EvidenceRecordRef).Scan(&other); err == nil {
		return contract.TimelineProjectionTombstoneFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "tombstone_ref", "projection already has another immutable tombstone")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return contract.TimelineProjectionTombstoneFactV1{}, false, unavailable("inspect tombstone overlay", err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO timeline_tombstones(tombstone_id,evidence_ref,scope_digest,digest,body) VALUES(?,?,?,?,?)", fact.TombstoneID, fact.EvidenceRecordRef, fact.ScopeDigest, fact.Digest, body); err != nil {
		return contract.TimelineProjectionTombstoneFactV1{}, false, unavailable("insert tombstone", err)
	}
	if err = tx.Commit(); err != nil {
		return contract.TimelineProjectionTombstoneFactV1{}, false, unavailable("commit tombstone", err)
	}
	return fact, false, nil
}

func (s *Store) InspectTombstone(ctx context.Context, tombstoneID string) (contract.TimelineProjectionTombstoneFactV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.TimelineProjectionTombstoneFactV1{}, err
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, "SELECT body FROM timeline_tombstones WHERE tombstone_id=?", tombstoneID).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.TimelineProjectionTombstoneFactV1{}, notFound("tombstone_id", "tombstone not found")
		}
		return contract.TimelineProjectionTombstoneFactV1{}, unavailable("inspect tombstone", err)
	}
	var fact contract.TimelineProjectionTombstoneFactV1
	if err := decode(body, &fact); err != nil {
		return contract.TimelineProjectionTombstoneFactV1{}, err
	}
	if err := fact.Validate(); err != nil {
		return contract.TimelineProjectionTombstoneFactV1{}, contract.NewError(contract.ErrContentDigestMismatch, "tombstone", "stored tombstone failed validation")
	}
	return fact, nil
}

func projectionCycle(existing []contract.TimelineEventRecord, incoming contract.TimelineEventRecord) bool {
	graph := make(map[string][]string, len(existing)+1)
	for _, record := range existing {
		graph[record.Candidate.CandidateID] = append([]string{}, record.Candidate.ParentRefs...)
	}
	graph[incoming.Candidate.CandidateID] = append([]string{}, incoming.Candidate.ParentRefs...)
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, parent := range graph[id] {
			if _, exists := graph[parent]; exists && visit(parent) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	ids := make([]string, 0, len(graph))
	for id := range graph {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if visit(id) {
			return true
		}
	}
	return false
}

var _ ProductionSPI = (*Store)(nil)
