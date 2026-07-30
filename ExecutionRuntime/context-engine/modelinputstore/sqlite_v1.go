// Package modelinputstore provides Context Owner-local durable storage for
// provider-neutral model input materials. It makes no HA, backup, Provider, or
// production composition claim.
package modelinputstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	_ "modernc.org/sqlite"
)

const (
	sqliteSchemaVersionV1 = 1
	sqliteSchemaV1        = `
CREATE TABLE IF NOT EXISTS context_model_input_material_schema (
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS context_model_input_material_history (
  material_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  material_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  payload BLOB NOT NULL,
  PRIMARY KEY(material_id, revision)
);
CREATE TABLE IF NOT EXISTS context_model_input_material_current (
  material_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL CHECK(revision > 0),
  material_digest TEXT NOT NULL,
  highest_revision INTEGER NOT NULL CHECK(highest_revision > 0),
  FOREIGN KEY(material_id, revision)
    REFERENCES context_model_input_material_history(material_id, revision)
);
CREATE INDEX IF NOT EXISTS context_model_input_material_current_exact
  ON context_model_input_material_current(material_id, revision, material_digest, highest_revision);
`
)

type SQLiteConfigV1 struct {
	Path        string
	BusyTimeout time.Duration
}

type PublishReceiptV1 struct {
	Ref     contract.ContextModelInputMaterialRefV1
	Created bool
}

type SQLiteV1 struct {
	db *sql.DB
}

var (
	_ contract.ContextModelInputMaterialExactReaderV1   = (*SQLiteV1)(nil)
	_ contract.ContextModelInputMaterialCurrentReaderV1 = (*SQLiteV1)(nil)
)

func OpenSQLiteV1(ctx context.Context, config SQLiteConfigV1) (*SQLiteV1, error) {
	if err := checkContextV1(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.Path) == "" {
		return nil, fmt.Errorf("%w: model input sqlite path", contract.ErrInvalid)
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, fmt.Errorf("%w: model input sqlite busy timeout", contract.ErrLimitExceeded)
	}
	abs, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, fmt.Errorf("%w: model input sqlite path", contract.ErrInvalid)
	}
	dsn := (&url.URL{Scheme: "file", Path: abs}).String()
	dsn += fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(FULL)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapSQLiteErrorV1(ctx, err, false)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &SQLiteV1{db: db}
	if err = store.migrateV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err = store.verifyPragmasV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteV1) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteV1) IntegrityCheckV1(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("%w: model input sqlite repository", contract.ErrUnavailable)
	}
	if err := checkContextV1(ctx); err != nil {
		return err
	}
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	}
	if result != "ok" {
		return fmt.Errorf("%w: model input sqlite integrity", contract.ErrConflict)
	}
	return nil
}

func (s *SQLiteV1) CommitV1(ctx context.Context, material contract.ContextModelInputMaterialV1, previous *contract.ContextModelInputMaterialRefV1, nowUnixNano int64) (PublishReceiptV1, error) {
	if s == nil || s.db == nil {
		return PublishReceiptV1{}, fmt.Errorf("%w: model input sqlite repository", contract.ErrUnavailable)
	}
	if err := checkContextV1(ctx); err != nil {
		return PublishReceiptV1{}, err
	}
	if material.Validate() != nil || nowUnixNano <= 0 {
		return PublishReceiptV1{}, fmt.Errorf("%w: model input sqlite publish", contract.ErrInvalid)
	}
	if nowUnixNano < material.CheckedUnixNano {
		return PublishReceiptV1{}, fmt.Errorf("%w: model input sqlite clock rollback", contract.ErrConflict)
	}
	if nowUnixNano >= material.ExpiresUnixNano {
		return PublishReceiptV1{}, fmt.Errorf("%w: model input sqlite publish lifetime", contract.ErrExpired)
	}
	if previous != nil && previous.Validate() != nil {
		return PublishReceiptV1{}, fmt.Errorf("%w: model input sqlite predecessor", contract.ErrInvalid)
	}
	payload, rowDigest, err := encodeRowV1(material)
	if err != nil {
		return PublishReceiptV1{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return PublishReceiptV1{}, mapSQLiteErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()

	existing, loadErr := loadExactV1(ctx, tx, material.Ref)
	if loadErr == nil {
		if !reflect.DeepEqual(existing, material) || !replayPredecessorExactV1(ctx, tx, material.Ref, previous) {
			return PublishReceiptV1{}, fmt.Errorf("%w: model input sqlite revision replay drift", contract.ErrConflict)
		}
		current, highest, currentErr := loadCurrentRefV1(ctx, tx, material.Ref.ID)
		maximum, maximumErr := loadMaximumRevisionV1(ctx, tx, material.Ref.ID)
		if currentErr != nil || maximumErr != nil || current.Revision != highest || highest != maximum || highest < material.Ref.Revision {
			return PublishReceiptV1{}, fmt.Errorf("%w: model input sqlite replay closure drift", contract.ErrConflict)
		}
		return PublishReceiptV1{Ref: material.Ref, Created: false}, nil
	}
	if !errors.Is(loadErr, contract.ErrNotFound) {
		return PublishReceiptV1{}, loadErr
	}

	current, highest, currentErr := loadCurrentRefV1(ctx, tx, material.Ref.ID)
	maximum, maximumErr := loadMaximumRevisionV1(ctx, tx, material.Ref.ID)
	if previous == nil {
		if material.Ref.Revision != 1 || currentErr == nil || maximumErr == nil || !errors.Is(currentErr, contract.ErrNotFound) || !errors.Is(maximumErr, contract.ErrNotFound) {
			return PublishReceiptV1{}, fmt.Errorf("%w: model input sqlite create-once identity", contract.ErrConflict)
		}
	} else {
		if currentErr != nil || maximumErr != nil || current != *previous || highest != previous.Revision || maximum != highest || material.Ref.ID != previous.ID || material.Ref.Revision != previous.Revision+1 {
			return PublishReceiptV1{}, fmt.Errorf("%w: model input sqlite current exact CAS", contract.ErrConflict)
		}
		if _, err = loadExactV1(ctx, tx, *previous); err != nil {
			return PublishReceiptV1{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO context_model_input_material_history(material_id,revision,material_digest,row_digest,payload) VALUES(?,?,?,?,?)`, material.Ref.ID, material.Ref.Revision, string(material.Ref.Digest), string(rowDigest), payload); err != nil {
		return PublishReceiptV1{}, mapSQLiteErrorV1(ctx, err, true)
	}
	if previous == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO context_model_input_material_current(material_id,revision,material_digest,highest_revision) VALUES(?,?,?,?)`, material.Ref.ID, material.Ref.Revision, string(material.Ref.Digest), material.Ref.Revision)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE context_model_input_material_current SET revision=?,material_digest=?,highest_revision=? WHERE material_id=? AND revision=? AND material_digest=? AND highest_revision=?`, material.Ref.Revision, string(material.Ref.Digest), material.Ref.Revision, material.Ref.ID, previous.Revision, string(previous.Digest), previous.Revision)
		if err == nil {
			var affected int64
			affected, err = result.RowsAffected()
			if err == nil && affected != 1 {
				return PublishReceiptV1{}, fmt.Errorf("%w: model input sqlite current CAS affected no row", contract.ErrConflict)
			}
		}
	}
	if err != nil {
		return PublishReceiptV1{}, mapSQLiteErrorV1(ctx, err, true)
	}
	if err = tx.Commit(); err != nil {
		return PublishReceiptV1{}, mapSQLiteErrorV1(ctx, err, true)
	}
	return PublishReceiptV1{Ref: material.Ref, Created: true}, nil
}

func (s *SQLiteV1) ReadContextModelInputMaterialExactV1(ctx context.Context, exact contract.ContextModelInputMaterialRefV1, nowUnixNano int64) (contract.ContextModelInputMaterialV1, error) {
	if s == nil || s.db == nil {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input sqlite repository", contract.ErrUnavailable)
	}
	if err := checkContextV1(ctx); err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	if exact.Validate() != nil || nowUnixNano <= 0 {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input sqlite exact read", contract.ErrInvalid)
	}
	material, err := loadExactV1(ctx, s.db, exact)
	if err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	return currentAtV1(material, nowUnixNano)
}

func (s *SQLiteV1) ReadContextModelInputMaterialCurrentV1(ctx context.Context, id string, nowUnixNano int64) (contract.ContextModelInputMaterialV1, error) {
	if s == nil || s.db == nil {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input sqlite repository", contract.ErrUnavailable)
	}
	if err := checkContextV1(ctx); err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	identity := contract.ContextModelInputMaterialRefV1{ID: id, Revision: 1, Digest: contract.DigestBytes(nil)}
	if identity.Validate() != nil || nowUnixNano <= 0 {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input sqlite current read", contract.ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return contract.ContextModelInputMaterialV1{}, mapSQLiteErrorV1(ctx, err, false)
	}
	defer func() { _ = tx.Rollback() }()
	ref, highest, err := loadCurrentRefV1(ctx, tx, id)
	if err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	maximum, err := loadMaximumRevisionV1(ctx, tx, id)
	if err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	if ref.Revision != highest || highest != maximum {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input sqlite current/history drift", contract.ErrConflict)
	}
	material, err := loadExactV1(ctx, tx, ref)
	if err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	return currentAtV1(material, nowUnixNano)
}

func (s *SQLiteV1) migrateV1(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapSQLiteErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	schemaDigest := contract.DigestBytes([]byte(sqliteSchemaV1))
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='context_model_input_material_schema'`).Scan(&count); err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	}
	if count == 0 {
		if _, err = tx.ExecContext(ctx, sqliteSchemaV1); err != nil {
			return mapSQLiteErrorV1(ctx, err, true)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO context_model_input_material_schema(version,digest) VALUES(?,?)`, sqliteSchemaVersionV1, string(schemaDigest)); err != nil {
			return mapSQLiteErrorV1(ctx, err, true)
		}
	} else {
		var versionCount, maximumVersion int
		var stored string
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MAX(version),0) FROM context_model_input_material_schema`).Scan(&versionCount, &maximumVersion); err != nil || versionCount != 1 || maximumVersion != sqliteSchemaVersionV1 {
			return fmt.Errorf("%w: model input sqlite schema version drift", contract.ErrConflict)
		}
		if err = tx.QueryRowContext(ctx, `SELECT digest FROM context_model_input_material_schema WHERE version=?`, sqliteSchemaVersionV1).Scan(&stored); err != nil || stored != string(schemaDigest) {
			return fmt.Errorf("%w: model input sqlite schema drift", contract.ErrConflict)
		}
		for _, object := range []struct{ kind, name string }{
			{"table", "context_model_input_material_history"},
			{"table", "context_model_input_material_current"},
			{"index", "context_model_input_material_current_exact"},
		} {
			if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type=? AND name=?`, object.kind, object.name).Scan(&count); err != nil || count != 1 {
				return fmt.Errorf("%w: model input sqlite schema object drift", contract.ErrConflict)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return mapSQLiteErrorV1(ctx, err, true)
	}
	return nil
}

func (s *SQLiteV1) verifyPragmasV1(ctx context.Context) error {
	var journal string
	var synchronous, foreignKeys int
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA synchronous`).Scan(&synchronous); err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	}
	if !strings.EqualFold(journal, "wal") || synchronous != 2 || foreignKeys != 1 {
		return fmt.Errorf("%w: model input sqlite durability pragmas", contract.ErrConflict)
	}
	return nil
}

type queryRowerV1 interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadExactV1(ctx context.Context, source queryRowerV1, exact contract.ContextModelInputMaterialRefV1) (contract.ContextModelInputMaterialV1, error) {
	var materialDigest, rowDigest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT material_digest,row_digest,payload FROM context_model_input_material_history WHERE material_id=? AND revision=?`, exact.ID, exact.Revision).Scan(&materialDigest, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input material exact row", contract.ErrNotFound)
	}
	if err != nil {
		return contract.ContextModelInputMaterialV1{}, mapSQLiteErrorV1(ctx, err, false)
	}
	if materialDigest != string(exact.Digest) {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input material exact digest drift", contract.ErrConflict)
	}
	material, err := decodeRowV1(payload, rowDigest)
	if err != nil {
		return contract.ContextModelInputMaterialV1{}, err
	}
	if material.Ref != exact || material.Digest != exact.Digest {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input material exact ref drift", contract.ErrConflict)
	}
	return material.Clone(), nil
}

func loadCurrentRefV1(ctx context.Context, source queryRowerV1, id string) (contract.ContextModelInputMaterialRefV1, uint64, error) {
	var revision, highest uint64
	var digest string
	err := source.QueryRowContext(ctx, `SELECT revision,material_digest,highest_revision FROM context_model_input_material_current WHERE material_id=?`, id).Scan(&revision, &digest, &highest)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.ContextModelInputMaterialRefV1{}, 0, fmt.Errorf("%w: model input material current row", contract.ErrNotFound)
	}
	if err != nil {
		return contract.ContextModelInputMaterialRefV1{}, 0, mapSQLiteErrorV1(ctx, err, false)
	}
	ref := contract.ContextModelInputMaterialRefV1{ID: id, Revision: revision, Digest: contract.Digest(digest)}
	if ref.Validate() != nil || highest == 0 {
		return contract.ContextModelInputMaterialRefV1{}, 0, fmt.Errorf("%w: model input material current row drift", contract.ErrConflict)
	}
	return ref, highest, nil
}

func loadMaximumRevisionV1(ctx context.Context, source queryRowerV1, id string) (uint64, error) {
	var maximum sql.NullInt64
	if err := source.QueryRowContext(ctx, `SELECT MAX(revision) FROM context_model_input_material_history WHERE material_id=?`, id).Scan(&maximum); err != nil {
		return 0, mapSQLiteErrorV1(ctx, err, false)
	}
	if !maximum.Valid || maximum.Int64 <= 0 {
		return 0, fmt.Errorf("%w: model input material history", contract.ErrNotFound)
	}
	return uint64(maximum.Int64), nil
}

func replayPredecessorExactV1(ctx context.Context, source queryRowerV1, ref contract.ContextModelInputMaterialRefV1, previous *contract.ContextModelInputMaterialRefV1) bool {
	if ref.Revision == 1 {
		return previous == nil
	}
	if previous == nil || previous.ID != ref.ID || previous.Revision+1 != ref.Revision {
		return false
	}
	_, err := loadExactV1(ctx, source, *previous)
	return err == nil
}

func encodeRowV1(material contract.ContextModelInputMaterialV1) ([]byte, contract.Digest, error) {
	payload, err := jsonMarshalV1(material.Clone())
	if err != nil {
		return nil, "", err
	}
	rowDigest, err := contract.DigestJSON(struct {
		Domain   string
		Material contract.ContextModelInputMaterialV1
	}{"praxis.context/model-input-sqlite-row-v1", material.Clone()})
	if err != nil {
		return nil, "", err
	}
	return payload, rowDigest, nil
}

func decodeRowV1(payload []byte, storedRowDigest string) (contract.ContextModelInputMaterialV1, error) {
	material, err := contract.DecodeStrict[contract.ContextModelInputMaterialV1](payload)
	if err != nil {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input sqlite strict row", contract.ErrConflict)
	}
	_, rowDigest, err := encodeRowV1(material)
	if err != nil || string(rowDigest) != storedRowDigest || material.Validate() != nil {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input sqlite row digest drift", contract.ErrConflict)
	}
	return material.Clone(), nil
}

func jsonMarshalV1(value any) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) == 0 {
		return nil, fmt.Errorf("%w: model input sqlite row", contract.ErrInvalid)
	}
	return payload, nil
}

func currentAtV1(material contract.ContextModelInputMaterialV1, nowUnixNano int64) (contract.ContextModelInputMaterialV1, error) {
	if nowUnixNano < material.CheckedUnixNano {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input material clock rollback", contract.ErrConflict)
	}
	if nowUnixNano >= material.ExpiresUnixNano {
		return contract.ContextModelInputMaterialV1{}, fmt.Errorf("%w: model input material lifetime", contract.ErrExpired)
	}
	return material.Clone(), nil
}

func checkContextV1(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return fmt.Errorf("%w: model input sqlite context ended", contract.ErrUnknown)
	}
	return nil
}

func mapSQLiteErrorV1(ctx context.Context, err error, mutation bool) error {
	if ctx == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: model input sqlite outcome", contract.ErrUnknown)
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "locked") || strings.Contains(message, "busy") {
		return fmt.Errorf("%w: model input sqlite busy", contract.ErrUnavailable)
	}
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return fmt.Errorf("%w: model input sqlite constraint", contract.ErrConflict)
	}
	if mutation {
		return fmt.Errorf("%w: model input sqlite mutation outcome", contract.ErrUnknown)
	}
	return fmt.Errorf("%w: model input sqlite read", contract.ErrUnavailable)
}
