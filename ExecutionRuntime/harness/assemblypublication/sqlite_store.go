package assemblypublication

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	_ "modernc.org/sqlite"
)

type SQLiteStoreConfigV2 struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}

// SQLiteStoreV2 is the single-node durable OwnerStoreV2 backend. WAL provides
// crash durability; this type makes no HA, remote durability or SLA claim.
type SQLiteStoreV2 struct {
	db    *sql.DB
	clock func() time.Time
	mu    *sync.Mutex

	faultMu       sync.Mutex
	loseNextReply bool
}

var _ OwnerStoreV2 = (*SQLiteStoreV2)(nil)

var sqlitePublicationLocks sync.Map

func OpenSQLiteStoreV2(ctx context.Context, config SQLiteStoreConfigV2) (*SQLiteStoreV2, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, invalidStore("SQLite publication open requires a live context")
	}
	if strings.TrimSpace(config.Path) == "" {
		return nil, invalidStore("SQLite publication path is required")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, invalidStore("SQLite publication busy timeout exceeds one minute")
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 8
	}
	if config.MaxOpenConns > 32 {
		return nil, invalidStore("SQLite publication connection count exceeds 32")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	abs, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, invalidStore("SQLite publication path is invalid")
	}
	lock, _ := sqlitePublicationLocks.LoadOrStore(abs, &sync.Mutex{})
	dsn := (&url.URL{Scheme: "file", Path: abs}).String()
	dsn += fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapPublicationDBError(ctx, err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	store := &SQLiteStoreV2{db: db, clock: config.Clock, mu: lock.(*sync.Mutex)}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.verifyPragmas(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStoreV2) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStoreV2) migrate(ctx context.Context) error {
	if err := s.readReady(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapPublicationDBError(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, sqlitePublicationSchemaV2); err != nil {
		return mapPublicationDBError(ctx, err, true)
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "SQLite publication migration clock is invalid")
	}
	digest := core.DigestBytes([]byte(sqlitePublicationSchemaV2))
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO harness_publication_schema_v2(version,digest,applied_unix_nano) VALUES(?,?,?)`, sqlitePublicationSchemaVersionV2, string(digest), now.UnixNano())
	if err != nil {
		return mapPublicationDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapPublicationDBError(ctx, err, true)
	}
	if affected == 0 {
		var stored string
		if err = tx.QueryRowContext(ctx, `SELECT digest FROM harness_publication_schema_v2 WHERE version=?`, sqlitePublicationSchemaVersionV2).Scan(&stored); err != nil {
			return mapPublicationDBError(ctx, err, true)
		}
		if stored != string(digest) {
			return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "SQLite publication schema digest drifted")
		}
	}
	if err = tx.Commit(); err != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "SQLite publication migration commit outcome is unknown")
	}
	return nil
}

func (s *SQLiteStoreV2) verifyPragmas(ctx context.Context) error {
	var journal string
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		return mapPublicationDBError(ctx, err, false)
	}
	if !strings.EqualFold(journal, "wal") {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "SQLite publication WAL mode is inactive")
	}
	var foreignKeys int
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return mapPublicationDBError(ctx, err, false)
	}
	if foreignKeys != 1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "SQLite publication foreign keys are inactive")
	}
	return nil
}

func (s *SQLiteStoreV2) IntegrityCheckV2(ctx context.Context) error {
	if err := s.readReady(ctx); err != nil {
		return err
	}
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapPublicationDBError(ctx, err, false)
	}
	if result != "ok" {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidState, "SQLite publication integrity check failed")
	}
	return nil
}

func (s *SQLiteStoreV2) StageGenerationV2(ctx context.Context, id string, value assemblycontract.AssemblyGenerationV1) error {
	expected, err := assemblycontract.DeriveAssemblyPublicationIDV2(value.InputDigest, value.GenerationID)
	if err != nil || id != expected {
		return conflictStore("Generation does not belong to the requested PublicationID")
	}
	return s.stage(ctx, id, "generation", value.Digest, value)
}

func (s *SQLiteStoreV2) StageManifestV2(ctx context.Context, id string, value assemblycontract.AssemblyManifestV1) error {
	return s.stage(ctx, id, "manifest", value.Digest, value)
}

func (s *SQLiteStoreV2) StageGraphV2(ctx context.Context, id string, value assemblycontract.CompiledHarnessGraphV1) error {
	return s.stage(ctx, id, "graph", value.Digest, value)
}

func (s *SQLiteStoreV2) StageHandoffV2(ctx context.Context, id string, value assemblycontract.AssemblyHandoffV1) error {
	return s.stage(ctx, id, "handoff", value.Digest, value)
}

func (s *SQLiteStoreV2) stage(ctx context.Context, id, column string, digest core.Digest, value any) error {
	if err := s.writeReady(ctx); err != nil {
		return err
	}
	if id == "" || digest.Validate() != nil {
		return invalidStore("SQLite publication staging identity and digest are required")
	}
	payload, rowDigest, err := encodePublicationRowV2("Staged"+strings.Title(column)+"V2", value)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapPublicationDBError(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	var committedPayload []byte
	var committedRowDigest string
	err = tx.QueryRowContext(ctx, `SELECT bundle_json,bundle_row_digest FROM harness_publication_committed_v2 WHERE publication_id=?`, id).Scan(&committedPayload, &committedRowDigest)
	if err == nil {
		bundle, decodeErr := decodePublicationRowV2[assemblycontract.AssemblyPublicationBundleV2](committedPayload, committedRowDigest, "CommittedBundleV2")
		if decodeErr != nil || bundle.Validate() != nil {
			if decodeErr != nil {
				return decodeErr
			}
			return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "committed publication bundle is invalid")
		}
		var existing core.Digest
		switch column {
		case "generation":
			existing = bundle.Generation.Digest
		case "manifest":
			existing = bundle.Manifest.Digest
		case "graph":
			existing = bundle.Graph.Digest
		case "handoff":
			existing = bundle.Handoff.Digest
		}
		if existing != digest {
			return conflictStore("committed publication object content drifted")
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return mapPublicationDBError(ctx, err, false)
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO harness_publication_staged_v2(publication_id) VALUES(?)`, id); err != nil {
		return mapPublicationDBError(ctx, err, true)
	}
	var existingDigest, existingRowDigest sql.NullString
	query := `SELECT ` + column + `_digest,` + column + `_row_digest FROM harness_publication_staged_v2 WHERE publication_id=?`
	if err = tx.QueryRowContext(ctx, query, id).Scan(&existingDigest, &existingRowDigest); err != nil {
		return mapPublicationDBError(ctx, err, false)
	}
	if existingDigest.Valid {
		if !existingRowDigest.Valid || existingDigest.String != string(digest) || existingRowDigest.String != rowDigest {
			return conflictStore("staged publication object content drifted")
		}
		return nil
	}
	update := `UPDATE harness_publication_staged_v2 SET ` + column + `_digest=?,` + column + `_row_digest=?,` + column + `_json=? WHERE publication_id=? AND ` + column + `_digest IS NULL`
	if _, err = tx.ExecContext(ctx, update, string(digest), rowDigest, payload, id); err != nil {
		return mapPublicationDBError(ctx, err, true)
	}
	return s.finishMutation(ctx, tx)
}

func (s *SQLiteStoreV2) InspectStagedPublicationV2(ctx context.Context, id string) (StagedPublicationInspectionV2, error) {
	if err := s.readReady(ctx); err != nil || id == "" {
		if err != nil {
			return StagedPublicationInspectionV2{}, err
		}
		return StagedPublicationInspectionV2{}, invalidStore("staged publication Inspect requires identity")
	}
	var values, rowDigests [4]sql.NullString
	var payloads [4][]byte
	err := s.db.QueryRowContext(ctx, `SELECT generation_digest,generation_row_digest,generation_json,manifest_digest,manifest_row_digest,manifest_json,graph_digest,graph_row_digest,graph_json,handoff_digest,handoff_row_digest,handoff_json FROM harness_publication_staged_v2 WHERE publication_id=?`, id).Scan(
		&values[0], &rowDigests[0], &payloads[0], &values[1], &rowDigests[1], &payloads[1],
		&values[2], &rowDigests[2], &payloads[2], &values[3], &rowDigests[3], &payloads[3],
	)
	if errors.Is(err, sql.ErrNoRows) {
		bundle, inspectErr := s.inspectBundleByID(ctx, id)
		if inspectErr != nil {
			return StagedPublicationInspectionV2{}, inspectErr
		}
		return inspectionFromBundle(bundle), nil
	}
	if err != nil {
		return StagedPublicationInspectionV2{}, mapPublicationDBError(ctx, err, false)
	}
	result := StagedPublicationInspectionV2{PublicationID: id}
	if values[0].Valid {
		value, decodeErr := decodePublicationRowV2[assemblycontract.AssemblyGenerationV1](payloads[0], rowDigests[0].String, "StagedGenerationV2")
		if decodeErr != nil || value.Digest != core.Digest(values[0].String) {
			if decodeErr != nil {
				return StagedPublicationInspectionV2{}, decodeErr
			}
			return StagedPublicationInspectionV2{}, conflictStore("staged Generation row drifted")
		}
		result.GenerationDigest = core.Digest(values[0].String)
	}
	if values[1].Valid {
		value, decodeErr := decodePublicationRowV2[assemblycontract.AssemblyManifestV1](payloads[1], rowDigests[1].String, "StagedManifestV2")
		if decodeErr != nil || value.Digest != core.Digest(values[1].String) {
			if decodeErr != nil {
				return StagedPublicationInspectionV2{}, decodeErr
			}
			return StagedPublicationInspectionV2{}, conflictStore("staged Manifest row drifted")
		}
		result.ManifestDigest = core.Digest(values[1].String)
	}
	if values[2].Valid {
		value, decodeErr := decodePublicationRowV2[assemblycontract.CompiledHarnessGraphV1](payloads[2], rowDigests[2].String, "StagedGraphV2")
		if decodeErr != nil || value.Digest != core.Digest(values[2].String) {
			if decodeErr != nil {
				return StagedPublicationInspectionV2{}, decodeErr
			}
			return StagedPublicationInspectionV2{}, conflictStore("staged Graph row drifted")
		}
		result.GraphDigest = core.Digest(values[2].String)
	}
	if values[3].Valid {
		value, decodeErr := decodePublicationRowV2[assemblycontract.AssemblyHandoffV1](payloads[3], rowDigests[3].String, "StagedHandoffV2")
		if decodeErr != nil || value.Digest != core.Digest(values[3].String) {
			if decodeErr != nil {
				return StagedPublicationInspectionV2{}, decodeErr
			}
			return StagedPublicationInspectionV2{}, conflictStore("staged Handoff row drifted")
		}
		result.HandoffDigest = core.Digest(values[3].String)
	}
	return result, nil
}

func (s *SQLiteStoreV2) CommitPublicationCurrentV2(ctx context.Context, request CommitPublicationCurrentRequestV2) (assemblycontract.AssemblyPublicationCurrentV2, error) {
	if err := s.writeReady(ctx); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, err
	}
	if err := validateCommitPublicationRequestV2(request); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, err
	}
	bundleJSON, bundleRowDigest, err := encodePublicationRowV2("CommittedBundleV2", request.Bundle)
	if err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, err
	}
	currentJSON, currentRowDigest, err := encodePublicationRowV2("CommittedCurrentV2", request.Current)
	if err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, mapPublicationDBError(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	var predecessorRevision int64
	var predecessorDigest string
	err = tx.QueryRowContext(ctx, `SELECT revision,digest FROM harness_publication_current_v2 WHERE scope_ref=?`, request.Current.ScopeRef).Scan(&predecessorRevision, &predecessorDigest)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return assemblycontract.AssemblyPublicationCurrentV2{}, mapPublicationDBError(ctx, err, false)
	}
	if request.Expected.Exists != exists || (exists && (core.Revision(predecessorRevision) != request.Expected.Revision || core.Digest(predecessorDigest) != request.Expected.Digest)) {
		return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("publication current predecessor changed before CAS")
	}
	var existingDigest string
	err = tx.QueryRowContext(ctx, `SELECT digest FROM harness_publication_committed_v2 WHERE publication_id=?`, request.Bundle.Publication.PublicationID).Scan(&existingDigest)
	if err == nil {
		if existingDigest != string(request.Bundle.Publication.Digest) {
			return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("PublicationID already carries different immutable content")
		}
		return assemblycontract.AssemblyPublicationCurrentV2{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "create-once publication was already committed")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return assemblycontract.AssemblyPublicationCurrentV2{}, mapPublicationDBError(ctx, err, false)
	}
	var stagedDigests, stagedRowDigests [4]sql.NullString
	var stagedPayloads [4][]byte
	err = tx.QueryRowContext(ctx, `SELECT generation_digest,generation_row_digest,generation_json,manifest_digest,manifest_row_digest,manifest_json,graph_digest,graph_row_digest,graph_json,handoff_digest,handoff_row_digest,handoff_json FROM harness_publication_staged_v2 WHERE publication_id=?`, request.Bundle.Publication.PublicationID).Scan(
		&stagedDigests[0], &stagedRowDigests[0], &stagedPayloads[0],
		&stagedDigests[1], &stagedRowDigests[1], &stagedPayloads[1],
		&stagedDigests[2], &stagedRowDigests[2], &stagedPayloads[2],
		&stagedDigests[3], &stagedRowDigests[3], &stagedPayloads[3],
	)
	if errors.Is(err, sql.ErrNoRows) {
		return assemblycontract.AssemblyPublicationCurrentV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReadyEvidenceIncomplete, "publication commit cannot expose a partial staged set")
	}
	if err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, mapPublicationDBError(ctx, err, false)
	}
	want := []core.Digest{request.Bundle.Generation.Digest, request.Bundle.Manifest.Digest, request.Bundle.Graph.Digest, request.Bundle.Handoff.Digest}
	for index := range stagedDigests {
		if !stagedDigests[index].Valid || !stagedRowDigests[index].Valid || stagedDigests[index].String != string(want[index]) {
			return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("staged publication content drifted before commit")
		}
	}
	if value, decodeErr := decodePublicationRowV2[assemblycontract.AssemblyGenerationV1](stagedPayloads[0], stagedRowDigests[0].String, "StagedGenerationV2"); decodeErr != nil || value.Digest != request.Bundle.Generation.Digest {
		if decodeErr != nil {
			return assemblycontract.AssemblyPublicationCurrentV2{}, decodeErr
		}
		return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("staged Generation payload drifted before commit")
	}
	if value, decodeErr := decodePublicationRowV2[assemblycontract.AssemblyManifestV1](stagedPayloads[1], stagedRowDigests[1].String, "StagedManifestV2"); decodeErr != nil || value.Digest != request.Bundle.Manifest.Digest {
		if decodeErr != nil {
			return assemblycontract.AssemblyPublicationCurrentV2{}, decodeErr
		}
		return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("staged Manifest payload drifted before commit")
	}
	if value, decodeErr := decodePublicationRowV2[assemblycontract.CompiledHarnessGraphV1](stagedPayloads[2], stagedRowDigests[2].String, "StagedGraphV2"); decodeErr != nil || value.Digest != request.Bundle.Graph.Digest {
		if decodeErr != nil {
			return assemblycontract.AssemblyPublicationCurrentV2{}, decodeErr
		}
		return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("staged Graph payload drifted before commit")
	}
	if value, decodeErr := decodePublicationRowV2[assemblycontract.AssemblyHandoffV1](stagedPayloads[3], stagedRowDigests[3].String, "StagedHandoffV2"); decodeErr != nil || value.Digest != request.Bundle.Handoff.Digest {
		if decodeErr != nil {
			return assemblycontract.AssemblyPublicationCurrentV2{}, decodeErr
		}
		return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("staged Handoff payload drifted before commit")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO harness_publication_committed_v2(publication_id,revision,digest,scope_ref,bundle_row_digest,bundle_json,current_row_digest,current_json) VALUES(?,?,?,?,?,?,?,?)`, request.Bundle.Publication.PublicationID, request.Bundle.Publication.Revision, string(request.Bundle.Publication.Digest), request.Bundle.Publication.ScopeRef, bundleRowDigest, bundleJSON, currentRowDigest, currentJSON); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, mapPublicationDBError(ctx, err, true)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO harness_publication_current_history_v2(scope_ref,revision,digest,publication_id,row_digest,canonical_json) VALUES(?,?,?,?,?,?)`, request.Current.ScopeRef, request.Current.Revision, string(request.Current.Digest), request.Bundle.Publication.PublicationID, currentRowDigest, currentJSON); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, mapPublicationDBError(ctx, err, true)
	}
	if exists {
		result, updateErr := tx.ExecContext(ctx, `UPDATE harness_publication_current_v2 SET revision=?,digest=?,publication_id=? WHERE scope_ref=? AND revision=? AND digest=?`, request.Current.Revision, string(request.Current.Digest), request.Bundle.Publication.PublicationID, request.Current.ScopeRef, request.Expected.Revision, string(request.Expected.Digest))
		if updateErr != nil {
			return assemblycontract.AssemblyPublicationCurrentV2{}, mapPublicationDBError(ctx, updateErr, true)
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("publication current CAS lost")
		}
	} else if _, err = tx.ExecContext(ctx, `INSERT INTO harness_publication_current_v2(scope_ref,revision,digest,publication_id) VALUES(?,?,?,?)`, request.Current.ScopeRef, request.Current.Revision, string(request.Current.Digest), request.Bundle.Publication.PublicationID); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, mapPublicationDBError(ctx, err, true)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM harness_publication_staged_v2 WHERE publication_id=?`, request.Bundle.Publication.PublicationID); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, mapPublicationDBError(ctx, err, true)
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, err
	}
	return clone(request.Current), nil
}

func validateCommitPublicationRequestV2(request CommitPublicationCurrentRequestV2) error {
	if err := request.Expected.Validate(); err != nil {
		return err
	}
	if err := request.Bundle.Validate(); err != nil {
		return err
	}
	if err := request.Current.ValidateAt(time.Unix(0, request.Current.CheckedUnixNano)); err != nil {
		return err
	}
	ref := assemblycontract.AssemblyPublicationRefV2{PublicationID: request.Bundle.Publication.PublicationID, Revision: request.Bundle.Publication.Revision, Digest: request.Bundle.Publication.Digest}
	if request.Current.ScopeRef != request.Bundle.Publication.ScopeRef || request.Current.Publication != ref || request.Current.InputDigest != request.Bundle.Publication.InputDigest || request.Current.Artifacts != request.Bundle.Publication.Artifacts {
		return conflictStore("publication current does not bind the staged bundle")
	}
	expectedRevision := core.Revision(1)
	if request.Expected.Exists {
		expectedRevision = request.Expected.Revision + 1
	}
	if expectedRevision == 0 || request.Current.Revision != expectedRevision {
		return conflictStore("publication current successor revision is invalid")
	}
	return nil
}

func (s *SQLiteStoreV2) InspectHistoricalPublicationV2(ctx context.Context, ref assemblycontract.AssemblyPublicationRefV2) (assemblycontract.AssemblyPublicationBundleV2, error) {
	if err := s.readReady(ctx); err != nil {
		return assemblycontract.AssemblyPublicationBundleV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return assemblycontract.AssemblyPublicationBundleV2{}, invalidStore("historical publication Inspect requires an exact ref")
	}
	bundle, err := s.inspectBundleByID(ctx, ref.PublicationID)
	if err != nil {
		return assemblycontract.AssemblyPublicationBundleV2{}, err
	}
	if bundle.Publication.Revision != ref.Revision || bundle.Publication.Digest != ref.Digest {
		return assemblycontract.AssemblyPublicationBundleV2{}, conflictStore("historical publication exact ref drifted")
	}
	return bundle, nil
}

func (s *SQLiteStoreV2) inspectBundleByID(ctx context.Context, id string) (assemblycontract.AssemblyPublicationBundleV2, error) {
	var payload []byte
	var rowDigest string
	err := s.db.QueryRowContext(ctx, `SELECT bundle_json,bundle_row_digest FROM harness_publication_committed_v2 WHERE publication_id=?`, id).Scan(&payload, &rowDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return assemblycontract.AssemblyPublicationBundleV2{}, notFoundStore("historical publication is not committed")
	}
	if err != nil {
		return assemblycontract.AssemblyPublicationBundleV2{}, mapPublicationDBError(ctx, err, false)
	}
	bundle, err := decodePublicationRowV2[assemblycontract.AssemblyPublicationBundleV2](payload, rowDigest, "CommittedBundleV2")
	if err != nil {
		return assemblycontract.AssemblyPublicationBundleV2{}, err
	}
	if err := bundle.Validate(); err != nil {
		return assemblycontract.AssemblyPublicationBundleV2{}, err
	}
	return bundle, nil
}

func (s *SQLiteStoreV2) InspectCommittedPublicationCurrentV2(ctx context.Context, ref assemblycontract.AssemblyPublicationRefV2) (assemblycontract.AssemblyPublicationCurrentV2, error) {
	if err := s.readReady(ctx); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, invalidStore("committed publication current Inspect requires an exact ref")
	}
	var publicationDigest, rowDigest string
	var publicationRevision int64
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT revision,digest,current_json,current_row_digest FROM harness_publication_committed_v2 WHERE publication_id=?`, ref.PublicationID).Scan(&publicationRevision, &publicationDigest, &payload, &rowDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return assemblycontract.AssemblyPublicationCurrentV2{}, notFoundStore("publication commit current is unavailable")
	}
	if err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, mapPublicationDBError(ctx, err, false)
	}
	if core.Revision(publicationRevision) != ref.Revision || publicationDigest != string(ref.Digest) {
		return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("publication commit current ref drifted")
	}
	current, err := decodePublicationRowV2[assemblycontract.AssemblyPublicationCurrentV2](payload, rowDigest, "CommittedCurrentV2")
	if err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, err
	}
	if current.Publication != ref || current.ValidateAt(time.Unix(0, current.CheckedUnixNano)) != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("publication commit current is invalid")
	}
	return current, nil
}

func (s *SQLiteStoreV2) InspectCurrentPublicationV2(ctx context.Context, scopeRef string) (assemblycontract.AssemblyPublicationCurrentV2, error) {
	if err := s.readReady(ctx); err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, err
	}
	if scopeRef == "" {
		return assemblycontract.AssemblyPublicationCurrentV2{}, invalidStore("publication current Inspect requires scope")
	}
	var payload []byte
	var rowDigest, pointerDigest, pointerPublicationID string
	var pointerRevision int64
	err := s.db.QueryRowContext(ctx, `SELECT c.revision,c.digest,c.publication_id,h.canonical_json,h.row_digest FROM harness_publication_current_v2 c JOIN harness_publication_current_history_v2 h ON h.scope_ref=c.scope_ref AND h.revision=c.revision WHERE c.scope_ref=?`, scopeRef).Scan(&pointerRevision, &pointerDigest, &pointerPublicationID, &payload, &rowDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return assemblycontract.AssemblyPublicationCurrentV2{}, notFoundStore("publication current is unavailable")
	}
	if err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, mapPublicationDBError(ctx, err, false)
	}
	current, err := decodePublicationRowV2[assemblycontract.AssemblyPublicationCurrentV2](payload, rowDigest, "CommittedCurrentV2")
	if err != nil {
		return assemblycontract.AssemblyPublicationCurrentV2{}, err
	}
	if current.ValidateAt(time.Unix(0, current.CheckedUnixNano)) != nil || current.ScopeRef != scopeRef || current.Revision != core.Revision(pointerRevision) || current.Digest != core.Digest(pointerDigest) || current.Publication.PublicationID != pointerPublicationID {
		return assemblycontract.AssemblyPublicationCurrentV2{}, conflictStore("publication current is invalid")
	}
	return current, nil
}

func (s *SQLiteStoreV2) readReady(ctx context.Context) error {
	if s == nil || s.db == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "SQLite publication store is unavailable")
	}
	if ctx == nil {
		return invalidStore("SQLite publication context is required")
	}
	if ctx.Err() != nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "SQLite publication read context ended")
	}
	return nil
}

func (s *SQLiteStoreV2) writeReady(ctx context.Context) error {
	if s == nil || s.db == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "SQLite publication store is unavailable")
	}
	if ctx == nil {
		return invalidStore("SQLite publication context is required")
	}
	if ctx.Err() != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "SQLite publication mutation context ended")
	}
	return nil
}

func (s *SQLiteStoreV2) finishMutation(ctx context.Context, tx *sql.Tx) error {
	if err := tx.Commit(); err != nil {
		return mapPublicationDBError(ctx, err, true)
	}
	s.faultMu.Lock()
	lose := s.loseNextReply
	s.loseNextReply = false
	s.faultMu.Unlock()
	if lose {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "SQLite publication committed but its reply was lost")
	}
	return nil
}

func mapPublicationDBError(ctx context.Context, err error, mutation bool) error {
	if err == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if mutation {
			return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "SQLite publication mutation outcome is unknown")
		}
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "SQLite publication read is unavailable")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "locked") || strings.Contains(message, "busy") {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "SQLite publication store is busy")
	}
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return conflictStore("SQLite publication uniqueness conflict")
	}
	if mutation {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "SQLite publication mutation outcome is unknown")
	}
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "SQLite publication read failed")
}
