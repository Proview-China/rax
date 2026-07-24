package assemblyadapter

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
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	_ "modernc.org/sqlite"
)

const (
	sqliteControlledOperationProviderRouteSchemaVersionV2 = 1
	sqliteControlledOperationProviderRouteRowDomainV2     = "praxis.harness.controlled-operation-provider-route.sqlite-row"
)

// SQLiteControlledOperationProviderRouteStoreConfigV2 configures the
// single-node durable Route V2 Fact and Owner Artifact store. It makes no
// HA, remote durability or SLA claim.
type SQLiteControlledOperationProviderRouteStoreConfigV2 struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
	Clock        func() time.Time
}

// SQLiteControlledOperationProviderRouteStoreV2 is the Harness-owned durable
// backend for immutable Route facts and verified Owner artifacts. Runtime
// association and Provider Binding currentness remain independently owned and
// are never synthesized by this store.
type SQLiteControlledOperationProviderRouteStoreV2 struct {
	db    *sql.DB
	clock func() time.Time
	mu    *sync.Mutex

	faultMu       sync.Mutex
	loseNextReply map[controlledOperationProviderRouteReplyV2]int
}

var (
	sqliteControlledOperationProviderRouteLocksV2 sync.Map

	_ ControlledOperationProviderRouteFactStoreV2              = (*SQLiteControlledOperationProviderRouteStoreV2)(nil)
	_ ControlledOperationProviderRouteConformanceOwnerSourceV2 = (*SQLiteControlledOperationProviderRouteStoreV2)(nil)
	_ ControlledOperationProviderRouteVerifiedCompileReaderV2  = (*SQLiteControlledOperationProviderRouteStoreV2)(nil)
	_ ControlledOperationProviderActiveRouteCurrentReaderV2    = (*SQLiteControlledOperationProviderRouteStoreV2)(nil)
	_ ControlledOperationProviderRouteWiringInventoryReaderV2  = (*SQLiteControlledOperationProviderRouteStoreV2)(nil)
)

const loseOwnerArtifactReplyV2 controlledOperationProviderRouteReplyV2 = "owner_artifact"

func OpenSQLiteControlledOperationProviderRouteStoreV2(ctx context.Context, config SQLiteControlledOperationProviderRouteStoreConfigV2) (*SQLiteControlledOperationProviderRouteStoreV2, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, sqliteRouteInvalidV2("SQLite controlled Provider route open requires a live context")
	}
	if strings.TrimSpace(config.Path) == "" {
		return nil, sqliteRouteInvalidV2("SQLite controlled Provider route path is required")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, sqliteRouteInvalidV2("SQLite controlled Provider route busy timeout exceeds one minute")
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 8
	}
	if config.MaxOpenConns > 32 {
		return nil, sqliteRouteInvalidV2("SQLite controlled Provider route connection count exceeds 32")
	}
	if config.Clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "SQLite controlled Provider route clock is required")
	}
	abs, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, sqliteRouteInvalidV2("SQLite controlled Provider route path is invalid")
	}
	lock, _ := sqliteControlledOperationProviderRouteLocksV2.LoadOrStore(abs, &sync.Mutex{})
	dsn := (&url.URL{Scheme: "file", Path: abs}).String()
	dsn += fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(FULL)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	store := &SQLiteControlledOperationProviderRouteStoreV2{
		db: db, clock: config.Clock, mu: lock.(*sync.Mutex),
		loseNextReply: make(map[controlledOperationProviderRouteReplyV2]int),
	}
	if err := store.migrateV2(ctx, abs); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.verifyV2(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) IntegrityCheckV2(ctx context.Context) error {
	if err := s.readReadyV2(ctx); err != nil {
		return err
	}
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapSQLiteRouteErrorV2(ctx, err, false)
	}
	if result != "ok" {
		return sqliteRouteCorruptV2("SQLite controlled Provider route integrity check failed")
	}
	return nil
}

// LoseNext* methods are deterministic fault-injection seams. They only lose
// the reply after a successful commit; recovery must Inspect the exact ref.
func (s *SQLiteControlledOperationProviderRouteStoreV2) LoseNextDeclarationReplyV2() {
	s.loseReplyV2(loseDeclarationReplyV2)
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) LoseNextConformanceReplyV2() {
	s.loseReplyV2(loseConformanceReplyV2)
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) LoseNextCurrentReplyV2() {
	s.loseReplyV2(loseCurrentReplyV2)
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) LoseNextOwnerArtifactReplyV2() {
	s.loseReplyV2(loseOwnerArtifactReplyV2)
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) loseReplyV2(kind controlledOperationProviderRouteReplyV2) {
	if s == nil {
		return
	}
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	s.loseNextReply[kind]++
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) controlledOperationProviderRouteConformanceOwnerSourceV2() {
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) PublishControlledOperationProviderRouteDeclarationV2(ctx context.Context, value assemblycontract.ControlledOperationProviderRouteDeclarationV2, expectedPrevious core.Revision) (assemblycontract.ControlledOperationProviderRouteDeclarationV2, error) {
	if err := s.writeReadyV2(ctx); err != nil {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, err
	}
	if err := value.Validate(); err != nil {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, err
	}
	payload, rowDigest, err := encodeSQLiteRouteRowV2("ControlledOperationProviderRouteDeclarationV2", value)
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, now, err := s.beginMutationV2(ctx)
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	existing, found, err := inspectSQLiteRouteDeclarationTxV2(ctx, tx, value.RouteID)
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, err
	}
	if found {
		if existing == value {
			return existing, nil
		}
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, sqliteRouteConflictV2("controlled Provider route declaration identity changed content")
	}
	if expectedPrevious != 0 || value.Revision != 1 {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, sqliteRouteConflictV2("controlled Provider route declaration CAS conflicted")
	}
	if err := s.checkBeforeMutationV2(ctx, tx, now); err != nil {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO harness_route_declaration_history_v2(route_id,revision,digest,publisher_component,row_digest,canonical_json) VALUES(?,?,?,?,?,?)`,
		value.RouteID, value.Revision, string(value.DeclarationDigest), string(value.PublisherComponent), rowDigest, payload)
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, mapSQLiteRouteErrorV2(ctx, err, true)
	}
	if err := s.finishMutationV2(ctx, tx, loseDeclarationReplyV2); err != nil {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, err
	}
	return value, nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) InspectControlledOperationProviderRouteDeclarationV2(ctx context.Context, ref runtimeports.ControlledOperationProviderRouteDeclarationRefV2) (assemblycontract.ControlledOperationProviderRouteDeclarationV2, error) {
	if err := s.readReadyV2(ctx); err != nil {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, err
	}
	var payload []byte
	var rowDigest string
	err := s.db.QueryRowContext(ctx, `SELECT canonical_json,row_digest FROM harness_route_declaration_history_v2 WHERE route_id=? AND revision=?`, ref.RouteID, ref.Revision).Scan(&payload, &rowDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, sqliteRouteNotFoundV2("controlled Provider route declaration is absent")
	}
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	value, err := decodeSQLiteRouteRowV2[assemblycontract.ControlledOperationProviderRouteDeclarationV2](payload, rowDigest, "ControlledOperationProviderRouteDeclarationV2")
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, err
	}
	if value.Validate() != nil || value.RefV2() != ref {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, sqliteRouteConflictV2("controlled Provider route declaration exact ref drifted")
	}
	return value, nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) PublishControlledOperationProviderRouteConformanceV2(ctx context.Context, value assemblycontract.ControlledOperationProviderRouteConformanceV2, expectedPrevious core.Revision) (assemblycontract.ControlledOperationProviderRouteConformanceV2, error) {
	if err := s.writeReadyV2(ctx); err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	if err := value.Validate(time.Unix(0, value.CheckedUnixNano)); err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	payload, rowDigest, err := encodeSQLiteRouteRowV2("ControlledOperationProviderRouteConformanceV2", value)
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, now, err := s.beginMutationV2(ctx)
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	declaration, found, err := inspectSQLiteRouteDeclarationTxV2(ctx, tx, value.DeclarationRef.RouteID)
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	if !found || declaration.RefV2() != value.DeclarationRef {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, sqliteRouteConflictV2("controlled Provider route conformance lacks its exact declaration")
	}
	existing, found, err := inspectSQLiteRouteConformanceTxV2(ctx, tx, value.ConformanceID)
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	if found {
		if existing == value {
			return existing, nil
		}
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, sqliteRouteConflictV2("controlled Provider route conformance identity changed content")
	}
	if expectedPrevious != 0 || value.Revision != 1 {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, sqliteRouteConflictV2("controlled Provider route conformance CAS conflicted")
	}
	if err := s.checkBeforeMutationV2(ctx, tx, now); err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO harness_route_conformance_history_v2(conformance_id,revision,digest,declaration_route_id,declaration_revision,declaration_digest,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?,?)`,
		value.ConformanceID, value.Revision, string(value.ConformanceDigest), value.DeclarationRef.RouteID, value.DeclarationRef.Revision, string(value.DeclarationRef.DeclarationDigest), rowDigest, payload)
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, mapSQLiteRouteErrorV2(ctx, err, true)
	}
	if err := s.finishMutationV2(ctx, tx, loseConformanceReplyV2); err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	return value, nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) InspectControlledOperationProviderRouteConformanceV2(ctx context.Context, ref runtimeports.ControlledOperationProviderRouteConformanceRefV2) (assemblycontract.ControlledOperationProviderRouteConformanceV2, error) {
	if err := s.readReadyV2(ctx); err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	var payload []byte
	var rowDigest string
	err := s.db.QueryRowContext(ctx, `SELECT canonical_json,row_digest FROM harness_route_conformance_history_v2 WHERE conformance_id=? AND revision=?`, ref.ConformanceID, ref.Revision).Scan(&payload, &rowDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, sqliteRouteNotFoundV2("controlled Provider route conformance is absent")
	}
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	value, err := decodeSQLiteRouteRowV2[assemblycontract.ControlledOperationProviderRouteConformanceV2](payload, rowDigest, "ControlledOperationProviderRouteConformanceV2")
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, err
	}
	if value.Validate(time.Unix(0, value.CheckedUnixNano)) != nil || value.RefV2() != ref {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, sqliteRouteConflictV2("controlled Provider route conformance exact ref drifted")
	}
	return value, nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) PublishControlledOperationProviderRouteCurrentV2(ctx context.Context, conformance assemblycontract.ControlledOperationProviderRouteConformanceV2, expectedPrevious core.Revision) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error) {
	if err := s.writeReadyV2(ctx); err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	if err := conformance.Validate(time.Unix(0, conformance.CheckedUnixNano)); err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, now, err := s.beginMutationV2(ctx)
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := conformance.Validate(now); err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	stored, found, err := inspectSQLiteRouteConformanceTxV2(ctx, tx, conformance.ConformanceID)
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	if !found || stored != conformance {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, sqliteRouteConflictV2("controlled Provider route current lacks exact conformance")
	}
	matrixDigest, err := runtimeports.DigestOperationScopeEvidenceApplicabilityMatrixKeyV3(runtimeports.OperationScopeEvidenceActionMatrixV3())
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	currentID, err := runtimeports.DeriveControlledOperationProviderRouteCurrentIDV2(conformance.DeclarationRef.RouteID, matrixDigest)
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	current, exists, err := inspectSQLiteRouteCurrentTxV2(ctx, tx, currentID)
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	revision := core.Revision(1)
	if exists {
		revision = current.Ref.Revision
	}
	projection, err := sealSQLiteRouteCurrentProjectionV2(conformance, revision)
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	if exists {
		if projection == current {
			return current, nil
		}
		if expectedPrevious != current.Ref.Revision || conformance.CheckedUnixNano <= current.CheckedUnixNano ||
			(conformance.Generation.ID == current.Generation.ID && conformance.Generation.Revision < current.Generation.Revision) ||
			(conformance.BindingSetID == current.BindingSetID && conformance.BindingSetRevision <= current.BindingSetRevision) {
			return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, sqliteRouteConflictV2("controlled Provider route current CAS or monotonic advancement conflicted")
		}
		projection, err = sealSQLiteRouteCurrentProjectionV2(conformance, current.Ref.Revision+1)
		if err != nil {
			return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
		}
	} else if expectedPrevious != 0 {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, sqliteRouteConflictV2("controlled Provider route current CAS conflicted")
	}
	payload, rowDigest, err := encodeSQLiteRouteRowV2("ControlledOperationProviderRouteCurrentProjectionV2", projection)
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	var seen int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM harness_route_current_history_v2 WHERE current_id=? AND watermark=?`, currentID, string(projection.Ref.Watermark)).Scan(&seen); err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	if seen != 0 {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, sqliteRouteConflictV2("controlled Provider route current rejected ABA watermark reuse")
	}
	if err := s.checkBeforeMutationV2(ctx, tx, now); err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO harness_route_current_history_v2(current_id,revision,digest,watermark,declaration_route_id,declaration_revision,declaration_digest,conformance_id,conformance_revision,conformance_digest,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		currentID, projection.Ref.Revision, string(projection.Ref.Digest), string(projection.Ref.Watermark),
		projection.DeclarationRef.RouteID, projection.DeclarationRef.Revision, string(projection.DeclarationRef.DeclarationDigest),
		projection.ConformanceRef.ConformanceID, projection.ConformanceRef.Revision, string(projection.ConformanceRef.ConformanceDigest), rowDigest, payload)
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, mapSQLiteRouteErrorV2(ctx, err, true)
	}
	if exists {
		result, updateErr := tx.ExecContext(ctx, `UPDATE harness_route_current_v2 SET revision=?,digest=? WHERE current_id=? AND revision=? AND digest=?`,
			projection.Ref.Revision, string(projection.Ref.Digest), currentID, current.Ref.Revision, string(current.Ref.Digest))
		if updateErr != nil {
			return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, mapSQLiteRouteErrorV2(ctx, updateErr, true)
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, sqliteRouteConflictV2("controlled Provider route current CAS lost")
		}
	} else {
		if _, err = tx.ExecContext(ctx, `INSERT INTO harness_route_current_v2(current_id,revision,digest) VALUES(?,?,?)`, currentID, projection.Ref.Revision, string(projection.Ref.Digest)); err != nil {
			return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, mapSQLiteRouteErrorV2(ctx, err, true)
		}
	}
	if err := s.finishMutationV2(ctx, tx, loseCurrentReplyV2); err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	return projection, nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) InspectControlledOperationProviderRouteCurrentV2(ctx context.Context, ref runtimeports.ControlledOperationProviderRouteCurrentRefV2) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error) {
	if err := s.readReadyV2(ctx); err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	if err := ref.Validate(); err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	defer func() { _ = tx.Rollback() }()
	firstNow, err := s.nowV2()
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	first, found, err := inspectSQLiteRouteCurrentTxV2(ctx, tx, ref.CurrentID)
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	if !found {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, sqliteRouteNotFoundV2("controlled Provider route current is absent")
	}
	matrix := runtimeports.OperationScopeEvidenceActionMatrixV3()
	if first.Ref != ref {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, sqliteRouteConflictV2("controlled Provider route current exact ref is no longer current")
	}
	if err := first.ValidateCurrent(ref, matrix, firstNow); err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	secondNow, err := s.nowV2()
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	if secondNow.Before(firstNow) {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "SQLite controlled Provider route current clock regressed")
	}
	second, found, err := inspectSQLiteRouteCurrentTxV2(ctx, tx, ref.CurrentID)
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	if !found || second != first {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, sqliteRouteConflictV2("controlled Provider route current changed during S1/S2 Inspect")
	}
	if err := second.ValidateCurrent(ref, matrix, secondNow); err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, err
	}
	if ctx.Err() != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "SQLite controlled Provider route current Inspect context ended")
	}
	if err := tx.Commit(); err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	return second, nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) PublishExactV2(ctx context.Context, value ControlledOperationProviderRouteOwnerArtifactPublicationV2, _ time.Time) (ControlledOperationProviderRouteOwnerRefsV2, error) {
	if err := s.writeReadyV2(ctx); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, clockNow, err := s.beginMutationV2(ctx)
	if err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	defer func() { _ = tx.Rollback() }()
	refs, contentDigest, err := validateSQLiteRouteOwnerPublicationV2(value, clockNow)
	if err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	publicationJSON, publicationRowDigest, err := encodeSQLiteRouteRowV2("ControlledOperationProviderRouteOwnerArtifactPublicationV2", value)
	if err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	refsJSON, refsRowDigest, err := encodeSQLiteRouteRowV2("ControlledOperationProviderRouteOwnerRefsV2", refs)
	if err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	compileJSON, compileRowDigest, err := encodeSQLiteRouteRowV2("ControlledOperationProviderRouteCompileResultV2", value.Compile)
	if err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	activeJSON, activeRowDigest, err := encodeSQLiteRouteRowV2("ControlledOperationProviderActiveRouteCurrentV2", value.ActiveRoute)
	if err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	wiringJSON, wiringRowDigest, err := encodeSQLiteRouteRowV2("ControlledOperationProviderRouteWiringInventoryV2", value.Wiring)
	if err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	existing, found, err := inspectSQLiteRouteOwnerPublicationTxV2(ctx, tx, value.Key)
	if err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	if found {
		existingRefs, existingDigest, decodeErr := validateSQLiteRouteOwnerPublicationV2(existing, clockNow)
		if decodeErr != nil || existingRefs != refs || existingDigest != contentDigest {
			return ControlledOperationProviderRouteOwnerRefsV2{}, sqliteRouteConflictV2("controlled Provider route Owner publication key changed content")
		}
		return existingRefs, nil
	}
	activeCurrent, activeExists, err := inspectSQLiteRouteActiveCurrentTxV2(ctx, tx, refs.ActiveRoute.ActiveRouteID)
	if err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	if activeExists {
		if refs.ActiveRoute == activeCurrent.Ref {
			return ControlledOperationProviderRouteOwnerRefsV2{}, sqliteRouteConflictV2("controlled Provider active current ref was reused by a different Owner publication")
		}
		if refs.ActiveRoute.Revision != activeCurrent.Ref.Revision+1 || value.ActiveRoute.CheckedUnixNano <= activeCurrent.CheckedUnixNano {
			return ControlledOperationProviderRouteOwnerRefsV2{}, sqliteRouteConflictV2("controlled Provider active current full-ref CAS or monotonic advancement conflicted")
		}
	} else if refs.ActiveRoute.Revision != 1 {
		return ControlledOperationProviderRouteOwnerRefsV2{}, sqliteRouteConflictV2("controlled Provider active current must begin at revision one")
	}
	if err := ensureSQLiteRouteImmutableRowV2(ctx, tx, `SELECT canonical_json,row_digest FROM harness_route_verified_compile_v2 WHERE compile_digest=?`, []any{string(refs.Compile.CompileDigest)}, compileJSON, compileRowDigest, "ControlledOperationProviderRouteCompileResultV2"); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	if err := ensureSQLiteRouteImmutableRowV2(ctx, tx, `SELECT canonical_json,row_digest FROM harness_route_active_history_v2 WHERE active_route_id=? AND revision=? AND digest=?`, []any{refs.ActiveRoute.ActiveRouteID, refs.ActiveRoute.Revision, string(refs.ActiveRoute.Digest)}, activeJSON, activeRowDigest, "ControlledOperationProviderActiveRouteCurrentV2"); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	if err := ensureSQLiteRouteImmutableRowV2(ctx, tx, `SELECT canonical_json,row_digest FROM harness_route_wiring_history_v2 WHERE inventory_id=? AND revision=? AND digest=?`, []any{refs.Wiring.InventoryID, refs.Wiring.Revision, string(refs.Wiring.Digest)}, wiringJSON, wiringRowDigest, "ControlledOperationProviderRouteWiringInventoryV2"); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	if err := s.checkBeforeMutationV2(ctx, tx, clockNow); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO harness_route_verified_compile_v2(compile_digest,row_digest,canonical_json) VALUES(?,?,?)`, string(refs.Compile.CompileDigest), compileRowDigest, compileJSON); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, mapSQLiteRouteErrorV2(ctx, err, true)
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO harness_route_active_history_v2(active_route_id,revision,digest,row_digest,canonical_json) VALUES(?,?,?,?,?)`, refs.ActiveRoute.ActiveRouteID, refs.ActiveRoute.Revision, string(refs.ActiveRoute.Digest), activeRowDigest, activeJSON); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, mapSQLiteRouteErrorV2(ctx, err, true)
	}
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO harness_route_wiring_history_v2(inventory_id,revision,digest,row_digest,canonical_json) VALUES(?,?,?,?,?)`, refs.Wiring.InventoryID, refs.Wiring.Revision, string(refs.Wiring.Digest), wiringRowDigest, wiringJSON); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, mapSQLiteRouteErrorV2(ctx, err, true)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO harness_route_owner_publication_v2(compile_digest,binding_set_id,active_route_id,revision,content_digest,refs_row_digest,refs_json,publication_row_digest,publication_json) VALUES(?,?,?,?,?,?,?,?,?)`,
		string(value.Key.CompileDigest), value.Key.BindingSetID, value.Key.ActiveRouteID, value.Key.Revision, string(contentDigest), refsRowDigest, refsJSON, publicationRowDigest, publicationJSON); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, mapSQLiteRouteErrorV2(ctx, err, true)
	}
	if activeExists {
		result, updateErr := tx.ExecContext(ctx, `UPDATE harness_route_active_current_v2 SET revision=?,digest=? WHERE active_route_id=? AND revision=? AND digest=?`,
			refs.ActiveRoute.Revision, string(refs.ActiveRoute.Digest), refs.ActiveRoute.ActiveRouteID, activeCurrent.Ref.Revision, string(activeCurrent.Ref.Digest))
		if updateErr != nil {
			return ControlledOperationProviderRouteOwnerRefsV2{}, mapSQLiteRouteErrorV2(ctx, updateErr, true)
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			return ControlledOperationProviderRouteOwnerRefsV2{}, sqliteRouteConflictV2("controlled Provider active current CAS lost")
		}
	} else if _, err = tx.ExecContext(ctx, `INSERT INTO harness_route_active_current_v2(active_route_id,revision,digest) VALUES(?,?,?)`,
		refs.ActiveRoute.ActiveRouteID, refs.ActiveRoute.Revision, string(refs.ActiveRoute.Digest)); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, mapSQLiteRouteErrorV2(ctx, err, true)
	}
	if err := s.finishMutationV2(ctx, tx, loseOwnerArtifactReplyV2); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	return refs, nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) InspectControlledOperationProviderRouteOwnerRefsV2(ctx context.Context, key ControlledOperationProviderRouteConformanceKeyV2) (ControlledOperationProviderRouteOwnerRefsV2, error) {
	if err := s.readReadyV2(ctx); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	var payload []byte
	var rowDigest string
	err := s.db.QueryRowContext(ctx, `SELECT refs_json,refs_row_digest FROM harness_route_owner_publication_v2 WHERE compile_digest=? AND binding_set_id=? AND active_route_id=? AND revision=?`,
		string(key.CompileDigest), key.BindingSetID, key.ActiveRouteID, key.Revision).Scan(&payload, &rowDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return ControlledOperationProviderRouteOwnerRefsV2{}, sqliteRouteNotFoundV2("controlled Provider route Owner refs are absent")
	}
	if err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	refs, err := decodeSQLiteRouteRowV2[ControlledOperationProviderRouteOwnerRefsV2](payload, rowDigest, "ControlledOperationProviderRouteOwnerRefsV2")
	if err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	if err := refs.validateV2(); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, err
	}
	if refs.Compile.CompileDigest != key.CompileDigest || refs.Revision != key.Revision ||
		refs.ActiveRoute.ActiveRouteID != key.ActiveRouteID || refs.Bindings[0].BindingSetID != key.BindingSetID {
		return ControlledOperationProviderRouteOwnerRefsV2{}, sqliteRouteConflictV2("controlled Provider route Owner refs lookup key drifted")
	}
	return refs, nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) InspectVerifiedControlledOperationProviderRouteCompileV2(ctx context.Context, ref ControlledOperationProviderRouteVerifiedCompileRefV2) (assemblycompiler.ControlledOperationProviderRouteCompileResultV2, error) {
	if err := s.readReadyV2(ctx); err != nil {
		return assemblycompiler.ControlledOperationProviderRouteCompileResultV2{}, err
	}
	var payload []byte
	var rowDigest string
	err := s.db.QueryRowContext(ctx, `SELECT canonical_json,row_digest FROM harness_route_verified_compile_v2 WHERE compile_digest=?`, string(ref.CompileDigest)).Scan(&payload, &rowDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return assemblycompiler.ControlledOperationProviderRouteCompileResultV2{}, sqliteRouteNotFoundV2("controlled Provider route verified compile is absent")
	}
	if err != nil {
		return assemblycompiler.ControlledOperationProviderRouteCompileResultV2{}, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	value, err := decodeSQLiteRouteRowV2[assemblycompiler.ControlledOperationProviderRouteCompileResultV2](payload, rowDigest, "ControlledOperationProviderRouteCompileResultV2")
	if err != nil {
		return assemblycompiler.ControlledOperationProviderRouteCompileResultV2{}, err
	}
	if value.ValidateV2() != nil || value.CompileDigest != ref.CompileDigest {
		return assemblycompiler.ControlledOperationProviderRouteCompileResultV2{}, sqliteRouteConflictV2("controlled Provider route verified compile ref drifted")
	}
	return value, nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) InspectControlledOperationProviderActiveRouteCurrentV2(ctx context.Context, ref ControlledOperationProviderActiveRouteCurrentRefV2) (ControlledOperationProviderActiveRouteCurrentV2, error) {
	if err := s.readReadyV2(ctx); err != nil {
		return ControlledOperationProviderActiveRouteCurrentV2{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return ControlledOperationProviderActiveRouteCurrentV2{}, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	defer func() { _ = tx.Rollback() }()
	now, err := s.nowV2()
	if err != nil {
		return ControlledOperationProviderActiveRouteCurrentV2{}, err
	}
	value, found, err := inspectSQLiteRouteActiveCurrentTxV2(ctx, tx, ref.ActiveRouteID)
	if err != nil {
		return ControlledOperationProviderActiveRouteCurrentV2{}, err
	}
	if !found {
		return ControlledOperationProviderActiveRouteCurrentV2{}, sqliteRouteNotFoundV2("controlled Provider active route is absent")
	}
	if value.Ref != ref {
		return ControlledOperationProviderActiveRouteCurrentV2{}, sqliteRouteConflictV2("controlled Provider active route exact ref is no longer current")
	}
	if now.Before(time.Unix(0, value.CheckedUnixNano)) {
		return ControlledOperationProviderActiveRouteCurrentV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "controlled Provider active route current clock regressed")
	}
	if !now.Before(time.Unix(0, value.ExpiresUnixNano)) {
		return ControlledOperationProviderActiveRouteCurrentV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled Provider active route current expired")
	}
	if err := value.validateExactV2(ref, now); err != nil {
		return ControlledOperationProviderActiveRouteCurrentV2{}, err
	}
	if err := tx.Commit(); err != nil {
		return ControlledOperationProviderActiveRouteCurrentV2{}, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	return value, nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) InspectControlledOperationProviderRouteWiringInventoryV2(ctx context.Context, ref ControlledOperationProviderRouteWiringInventoryRefV2) (assemblycontract.ControlledOperationProviderRouteWiringInventoryV2, error) {
	if err := s.readReadyV2(ctx); err != nil {
		return assemblycontract.ControlledOperationProviderRouteWiringInventoryV2{}, err
	}
	var payload []byte
	var rowDigest string
	err := s.db.QueryRowContext(ctx, `SELECT canonical_json,row_digest FROM harness_route_wiring_history_v2 WHERE inventory_id=? AND revision=? AND digest=?`, ref.InventoryID, ref.Revision, string(ref.Digest)).Scan(&payload, &rowDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return assemblycontract.ControlledOperationProviderRouteWiringInventoryV2{}, sqliteRouteNotFoundV2("controlled Provider route wiring inventory is absent")
	}
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteWiringInventoryV2{}, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	value, err := decodeSQLiteRouteRowV2[assemblycontract.ControlledOperationProviderRouteWiringInventoryV2](payload, rowDigest, "ControlledOperationProviderRouteWiringInventoryV2")
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteWiringInventoryV2{}, err
	}
	if value.Validate() != nil || value.InventoryID != ref.InventoryID || value.Revision != ref.Revision || value.Digest != ref.Digest {
		return assemblycontract.ControlledOperationProviderRouteWiringInventoryV2{}, sqliteRouteConflictV2("controlled Provider route wiring inventory ref drifted")
	}
	return value, nil
}

func validateSQLiteRouteOwnerPublicationV2(value ControlledOperationProviderRouteOwnerArtifactPublicationV2, now time.Time) (ControlledOperationProviderRouteOwnerRefsV2, core.Digest, error) {
	if now.IsZero() {
		return ControlledOperationProviderRouteOwnerRefsV2{}, "", core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "controlled Provider route Owner publication clock is unavailable")
	}
	if err := value.Compile.ValidateV2(); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, "", err
	}
	if value.Key.CompileDigest != value.Compile.CompileDigest || value.Key.BindingSetID == "" || value.Key.ActiveRouteID == "" || value.Key.Revision == 0 || value.Association.Validate() != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider route Owner publication coordinates are incomplete")
	}
	if value.ActiveRoute.Record.TransportIdentity != value.Compile.ProviderTransportIdentity || value.ActiveRoute.Record.ProviderIdentity != value.Compile.ProviderIdentity || value.ActiveRoute.Ref.ActiveRouteID != value.Key.ActiveRouteID {
		return ControlledOperationProviderRouteOwnerRefsV2{}, "", sqliteRouteConflictV2("controlled Provider route Owner active identity drifted from compile")
	}
	if err := value.ActiveRoute.validateExactV2(value.ActiveRoute.Ref, now); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, "", err
	}
	if err := assemblycompiler.ValidateControlledOperationProviderWiringV2(value.Compile.Declaration, value.Compile.ProviderTransportIdentity, value.Compile.ProviderIdentity, value.Wiring, value.Bindings, value.Compile.AssemblyInputDigest, value.Compile.Graph.Digest, now.UnixNano()); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, "", err
	}
	for _, binding := range value.Bindings {
		if binding.Validate() != nil || binding.BindingSetID != value.Key.BindingSetID {
			return ControlledOperationProviderRouteOwnerRefsV2{}, "", sqliteRouteConflictV2("controlled Provider route Owner Binding set drifted")
		}
	}
	refs := ControlledOperationProviderRouteOwnerRefsV2{
		Compile:     ControlledOperationProviderRouteVerifiedCompileRefV2{CompileDigest: value.Compile.CompileDigest},
		Association: value.Association,
		ActiveRoute: value.ActiveRoute.Ref,
		Wiring:      ControlledOperationProviderRouteWiringInventoryRefV2{InventoryID: value.Wiring.InventoryID, Revision: value.Wiring.Revision, Digest: value.Wiring.Digest},
		Bindings:    value.Bindings,
		Revision:    value.Key.Revision,
	}
	if err := refs.validateV2(); err != nil {
		return ControlledOperationProviderRouteOwnerRefsV2{}, "", err
	}
	contentDigest, err := core.CanonicalJSONDigest("praxis.harness.controlled-operation-provider-route", assemblycontract.ControlledOperationProviderRouteContractVersionV2, "ControlledOperationProviderRouteOwnerArtifactPublicationV2", value)
	return refs, contentDigest, err
}

func sealSQLiteRouteCurrentProjectionV2(conformance assemblycontract.ControlledOperationProviderRouteConformanceV2, revision core.Revision) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error) {
	return runtimeports.SealControlledOperationProviderRouteCurrentProjectionV2(runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{
		Ref: runtimeports.ControlledOperationProviderRouteCurrentRefV2{Revision: revision}, DeclarationRef: conformance.DeclarationRef, ConformanceRef: conformance.RefV2(),
		Generation: conformance.Generation, HandoffID: conformance.HandoffID, HandoffRevision: conformance.HandoffRevision, HandoffDigest: conformance.HandoffDigest,
		BindingSetID: conformance.BindingSetID, BindingSetRevision: conformance.BindingSetRevision, BindingSetDigest: conformance.BindingSetDigest,
		BindingSetSemanticDigest: conformance.BindingSetSemanticDigest, BindingSetCurrentnessDigest: conformance.BindingSetCurrentnessDigest,
		ActiveRouteID: conformance.ActiveRouteID, ActiveRouteRevision: conformance.ActiveRouteRevision, ActiveRouteDigest: conformance.ActiveRouteDigest,
		ToolAdapterBinding: conformance.ToolAdapterBinding, GatewayBinding: conformance.GatewayBinding, ProviderTransportBinding: conformance.ProviderTransportBinding,
		PreparedReaderBinding: conformance.PreparedReaderBinding, BoundaryReaderBinding: conformance.BoundaryReaderBinding, ProviderInspectBinding: conformance.ProviderInspectBinding,
		ProviderBinding: conformance.ProviderBinding, CheckedUnixNano: conformance.CheckedUnixNano, ExpiresUnixNano: conformance.ExpiresUnixNano,
	})
}

func inspectSQLiteRouteDeclarationTxV2(ctx context.Context, tx *sql.Tx, routeID string) (assemblycontract.ControlledOperationProviderRouteDeclarationV2, bool, error) {
	var payload []byte
	var rowDigest string
	err := tx.QueryRowContext(ctx, `SELECT canonical_json,row_digest FROM harness_route_declaration_history_v2 WHERE route_id=? ORDER BY revision DESC LIMIT 1`, routeID).Scan(&payload, &rowDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, false, nil
	}
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}, false, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	value, err := decodeSQLiteRouteRowV2[assemblycontract.ControlledOperationProviderRouteDeclarationV2](payload, rowDigest, "ControlledOperationProviderRouteDeclarationV2")
	if err == nil {
		err = value.Validate()
	}
	return value, err == nil, err
}

func inspectSQLiteRouteConformanceTxV2(ctx context.Context, tx *sql.Tx, conformanceID string) (assemblycontract.ControlledOperationProviderRouteConformanceV2, bool, error) {
	var payload []byte
	var rowDigest string
	err := tx.QueryRowContext(ctx, `SELECT canonical_json,row_digest FROM harness_route_conformance_history_v2 WHERE conformance_id=? ORDER BY revision DESC LIMIT 1`, conformanceID).Scan(&payload, &rowDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, false, nil
	}
	if err != nil {
		return assemblycontract.ControlledOperationProviderRouteConformanceV2{}, false, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	value, err := decodeSQLiteRouteRowV2[assemblycontract.ControlledOperationProviderRouteConformanceV2](payload, rowDigest, "ControlledOperationProviderRouteConformanceV2")
	if err == nil {
		err = value.Validate(time.Unix(0, value.CheckedUnixNano))
	}
	return value, err == nil, err
}

func inspectSQLiteRouteCurrentTxV2(ctx context.Context, tx *sql.Tx, currentID string) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, bool, error) {
	var payload []byte
	var rowDigest string
	err := tx.QueryRowContext(ctx, `SELECT h.canonical_json,h.row_digest FROM harness_route_current_v2 c JOIN harness_route_current_history_v2 h ON h.current_id=c.current_id AND h.revision=c.revision AND h.digest=c.digest WHERE c.current_id=?`, currentID).Scan(&payload, &rowDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, false, nil
	}
	if err != nil {
		return runtimeports.ControlledOperationProviderRouteCurrentProjectionV2{}, false, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	value, err := decodeSQLiteRouteRowV2[runtimeports.ControlledOperationProviderRouteCurrentProjectionV2](payload, rowDigest, "ControlledOperationProviderRouteCurrentProjectionV2")
	if err == nil {
		err = value.Validate()
	}
	return value, err == nil, err
}

func inspectSQLiteRouteActiveCurrentTxV2(ctx context.Context, tx *sql.Tx, activeRouteID string) (ControlledOperationProviderActiveRouteCurrentV2, bool, error) {
	var payload []byte
	var rowDigest string
	err := tx.QueryRowContext(ctx, `SELECT h.canonical_json,h.row_digest FROM harness_route_active_current_v2 c JOIN harness_route_active_history_v2 h ON h.active_route_id=c.active_route_id AND h.revision=c.revision AND h.digest=c.digest WHERE c.active_route_id=?`, activeRouteID).Scan(&payload, &rowDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return ControlledOperationProviderActiveRouteCurrentV2{}, false, nil
	}
	if err != nil {
		return ControlledOperationProviderActiveRouteCurrentV2{}, false, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	value, err := decodeSQLiteRouteRowV2[ControlledOperationProviderActiveRouteCurrentV2](payload, rowDigest, "ControlledOperationProviderActiveRouteCurrentV2")
	if err == nil {
		recordDigest, digestErr := core.CanonicalJSONDigest("praxis.harness.controlled-operation-provider-route", assemblycontract.ControlledOperationProviderRouteContractVersionV2, "ControlledOperationProviderActiveRouteRecordV2", value.Record)
		if digestErr != nil || value.Record.RouteID != value.Ref.ActiveRouteID || value.Ref.Digest != recordDigest || value.CheckedUnixNano <= 0 || value.CheckedUnixNano >= value.ExpiresUnixNano {
			err = sqliteRouteConflictV2("controlled Provider active route current row drifted")
		}
	}
	return value, err == nil, err
}

func inspectSQLiteRouteOwnerPublicationTxV2(ctx context.Context, tx *sql.Tx, key ControlledOperationProviderRouteConformanceKeyV2) (ControlledOperationProviderRouteOwnerArtifactPublicationV2, bool, error) {
	var payload []byte
	var rowDigest string
	err := tx.QueryRowContext(ctx, `SELECT publication_json,publication_row_digest FROM harness_route_owner_publication_v2 WHERE compile_digest=? AND binding_set_id=? AND active_route_id=? AND revision=?`,
		string(key.CompileDigest), key.BindingSetID, key.ActiveRouteID, key.Revision).Scan(&payload, &rowDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return ControlledOperationProviderRouteOwnerArtifactPublicationV2{}, false, nil
	}
	if err != nil {
		return ControlledOperationProviderRouteOwnerArtifactPublicationV2{}, false, mapSQLiteRouteErrorV2(ctx, err, false)
	}
	value, err := decodeSQLiteRouteRowV2[ControlledOperationProviderRouteOwnerArtifactPublicationV2](payload, rowDigest, "ControlledOperationProviderRouteOwnerArtifactPublicationV2")
	return value, err == nil, err
}

func ensureSQLiteRouteImmutableRowV2(ctx context.Context, tx *sql.Tx, query string, args []any, expectedJSON []byte, expectedRowDigest, kind string) error {
	var payload []byte
	var rowDigest string
	err := tx.QueryRowContext(ctx, query, args...).Scan(&payload, &rowDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return mapSQLiteRouteErrorV2(ctx, err, false)
	}
	if rowDigest != expectedRowDigest || !reflect.DeepEqual(payload, expectedJSON) {
		return sqliteRouteConflictV2(kind + " exact ref changed immutable content")
	}
	return nil
}

func encodeSQLiteRouteRowV2(kind string, value any) ([]byte, string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "SQLite controlled Provider route row cannot be encoded")
	}
	digest, err := core.CanonicalJSONDigest(sqliteControlledOperationProviderRouteRowDomainV2, "2.0.0", kind, value)
	if err != nil {
		return nil, "", err
	}
	return payload, string(digest), nil
}

func decodeSQLiteRouteRowV2[T any](payload []byte, rowDigest, kind string) (T, error) {
	var zero T
	var value T
	if err := core.DecodeStrictJSON(payload, &value); err != nil {
		return zero, sqliteRouteCorruptV2("SQLite controlled Provider route row JSON drifted")
	}
	_, expected, err := encodeSQLiteRouteRowV2(kind, value)
	if err != nil {
		return zero, err
	}
	if expected != rowDigest {
		return zero, sqliteRouteCorruptV2("SQLite controlled Provider route row digest drifted")
	}
	return value, nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) migrateV2(ctx context.Context, abs string) error {
	if err := s.readReadyV2(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapSQLiteRouteErrorV2(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, sqliteControlledOperationProviderRouteSchemaV2); err != nil {
		return mapSQLiteRouteErrorV2(ctx, err, true)
	}
	now, err := s.nowV2()
	if err != nil {
		return err
	}
	schemaDigest := core.DigestBytes([]byte(sqliteControlledOperationProviderRouteSchemaV2))
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO harness_route_schema_v2(version,digest,applied_unix_nano) VALUES(?,?,?)`, sqliteControlledOperationProviderRouteSchemaVersionV2, string(schemaDigest), now.UnixNano())
	if err != nil {
		return mapSQLiteRouteErrorV2(ctx, err, true)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		var stored string
		if err = tx.QueryRowContext(ctx, `SELECT digest FROM harness_route_schema_v2 WHERE version=?`, sqliteControlledOperationProviderRouteSchemaVersionV2).Scan(&stored); err != nil {
			return mapSQLiteRouteErrorV2(ctx, err, true)
		}
		if stored != string(schemaDigest) {
			return sqliteRouteCorruptV2("SQLite controlled Provider route schema digest drifted")
		}
	}
	var count, minVersion, maxVersion int64
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*),MIN(version),MAX(version) FROM harness_route_schema_v2`).Scan(&count, &minVersion, &maxVersion); err != nil {
		return mapSQLiteRouteErrorV2(ctx, err, true)
	}
	if count != 1 || minVersion != sqliteControlledOperationProviderRouteSchemaVersionV2 || maxVersion != sqliteControlledOperationProviderRouteSchemaVersionV2 {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "SQLite controlled Provider route schema version set drifted")
	}
	identity := core.DigestBytes([]byte(abs))
	result, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO harness_route_identity_v2(singleton,database_identity_digest,clock_high_water_unix_nano) VALUES(1,?,?)`, string(identity), now.UnixNano())
	if err != nil {
		return mapSQLiteRouteErrorV2(ctx, err, true)
	}
	affected, _ = result.RowsAffected()
	var storedIdentity string
	var highWater int64
	if err = tx.QueryRowContext(ctx, `SELECT database_identity_digest,clock_high_water_unix_nano FROM harness_route_identity_v2 WHERE singleton=1`).Scan(&storedIdentity, &highWater); err != nil {
		return mapSQLiteRouteErrorV2(ctx, err, true)
	}
	if storedIdentity != string(identity) {
		return sqliteRouteConflictV2("SQLite controlled Provider route database identity drifted")
	}
	if now.UnixNano() < highWater {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "SQLite controlled Provider route clock regressed")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE harness_route_identity_v2 SET clock_high_water_unix_nano=? WHERE singleton=1`, now.UnixNano()); err != nil {
		return mapSQLiteRouteErrorV2(ctx, err, true)
	}
	if err = tx.Commit(); err != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "SQLite controlled Provider route migration outcome is unknown")
	}
	return nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) verifyV2(ctx context.Context) error {
	var journal string
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		return mapSQLiteRouteErrorV2(ctx, err, false)
	}
	if !strings.EqualFold(journal, "wal") {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "SQLite controlled Provider route WAL mode is inactive")
	}
	var foreignKeys int
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return mapSQLiteRouteErrorV2(ctx, err, false)
	}
	if foreignKeys != 1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "SQLite controlled Provider route foreign keys are inactive")
	}
	var synchronous int
	if err := s.db.QueryRowContext(ctx, `PRAGMA synchronous`).Scan(&synchronous); err != nil {
		return mapSQLiteRouteErrorV2(ctx, err, false)
	}
	if synchronous < 2 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "SQLite controlled Provider route FULL synchronous durability is inactive")
	}
	return nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) beginMutationV2(ctx context.Context) (*sql.Tx, time.Time, error) {
	if err := s.writeReadyV2(ctx); err != nil {
		return nil, time.Time{}, err
	}
	now, err := s.nowV2()
	if err != nil {
		return nil, time.Time{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, time.Time{}, mapSQLiteRouteErrorV2(ctx, err, true)
	}
	var highWater int64
	if err = tx.QueryRowContext(ctx, `SELECT clock_high_water_unix_nano FROM harness_route_identity_v2 WHERE singleton=1`).Scan(&highWater); err != nil {
		_ = tx.Rollback()
		return nil, time.Time{}, mapSQLiteRouteErrorV2(ctx, err, true)
	}
	if now.UnixNano() < highWater {
		_ = tx.Rollback()
		return nil, time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "SQLite controlled Provider route clock regressed")
	}
	return tx, now, nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) checkBeforeMutationV2(ctx context.Context, tx *sql.Tx, now time.Time) error {
	if ctx == nil || ctx.Err() != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "SQLite controlled Provider route mutation context ended")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE harness_route_identity_v2 SET clock_high_water_unix_nano=? WHERE singleton=1 AND clock_high_water_unix_nano<=?`, now.UnixNano(), now.UnixNano()); err != nil {
		return mapSQLiteRouteErrorV2(ctx, err, true)
	}
	return nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) finishMutationV2(ctx context.Context, tx *sql.Tx, kind controlledOperationProviderRouteReplyV2) error {
	if err := tx.Commit(); err != nil {
		return mapSQLiteRouteErrorV2(ctx, err, true)
	}
	s.faultMu.Lock()
	lost := s.loseNextReply[kind] > 0
	if lost {
		s.loseNextReply[kind]--
	}
	s.faultMu.Unlock()
	if lost {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "SQLite controlled Provider route committed but its reply was lost")
	}
	return nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) nowV2() (time.Time, error) {
	if s == nil || s.clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "SQLite controlled Provider route clock is unavailable")
	}
	now := s.clock()
	if now.IsZero() || now.UnixNano() <= 0 {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "SQLite controlled Provider route clock is invalid")
	}
	return now, nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) readReadyV2(ctx context.Context) error {
	if s == nil || s.db == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "SQLite controlled Provider route store is unavailable")
	}
	if ctx == nil {
		return sqliteRouteInvalidV2("SQLite controlled Provider route context is required")
	}
	if ctx.Err() != nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "SQLite controlled Provider route read context ended")
	}
	return nil
}

func (s *SQLiteControlledOperationProviderRouteStoreV2) writeReadyV2(ctx context.Context) error {
	if s == nil || s.db == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "SQLite controlled Provider route store is unavailable")
	}
	if ctx == nil {
		return sqliteRouteInvalidV2("SQLite controlled Provider route context is required")
	}
	if ctx.Err() != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "SQLite controlled Provider route mutation context ended")
	}
	return nil
}

func mapSQLiteRouteErrorV2(ctx context.Context, err error, mutation bool) error {
	if err == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if mutation {
			return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "SQLite controlled Provider route mutation outcome is unknown")
		}
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "SQLite controlled Provider route read is unavailable")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "locked") || strings.Contains(message, "busy") {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "SQLite controlled Provider route store is busy")
	}
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return sqliteRouteConflictV2("SQLite controlled Provider route uniqueness conflict")
	}
	if mutation {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "SQLite controlled Provider route mutation outcome is unknown")
	}
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "SQLite controlled Provider route read failed")
}

func sqliteRouteInvalidV2(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}

func sqliteRouteConflictV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, message)
}

func sqliteRouteNotFoundV2(message string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, message)
}

func sqliteRouteCorruptV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, message)
}

const sqliteControlledOperationProviderRouteSchemaV2 = `
CREATE TABLE IF NOT EXISTS harness_route_schema_v2(
  version INTEGER PRIMARY KEY,
  digest TEXT NOT NULL,
  applied_unix_nano INTEGER NOT NULL CHECK(applied_unix_nano > 0)
);
CREATE TABLE IF NOT EXISTS harness_route_identity_v2(
  singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
  database_identity_digest TEXT NOT NULL,
  clock_high_water_unix_nano INTEGER NOT NULL CHECK(clock_high_water_unix_nano > 0)
);
CREATE TABLE IF NOT EXISTS harness_route_declaration_history_v2(
  route_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  publisher_component TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(route_id,revision),
  UNIQUE(route_id,digest),
  UNIQUE(route_id,revision,digest)
);
CREATE TABLE IF NOT EXISTS harness_route_conformance_history_v2(
  conformance_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  declaration_route_id TEXT NOT NULL,
  declaration_revision INTEGER NOT NULL,
  declaration_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(conformance_id,revision),
  UNIQUE(conformance_id,digest),
  UNIQUE(conformance_id,revision,digest),
  FOREIGN KEY(declaration_route_id,declaration_revision,declaration_digest)
    REFERENCES harness_route_declaration_history_v2(route_id,revision,digest)
);
CREATE TABLE IF NOT EXISTS harness_route_current_history_v2(
  current_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  watermark TEXT NOT NULL,
  declaration_route_id TEXT NOT NULL,
  declaration_revision INTEGER NOT NULL,
  declaration_digest TEXT NOT NULL,
  conformance_id TEXT NOT NULL,
  conformance_revision INTEGER NOT NULL,
  conformance_digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(current_id,revision),
  UNIQUE(current_id,digest),
  UNIQUE(current_id,watermark),
  UNIQUE(current_id,revision,digest),
  FOREIGN KEY(declaration_route_id,declaration_revision,declaration_digest)
    REFERENCES harness_route_declaration_history_v2(route_id,revision,digest),
  FOREIGN KEY(conformance_id,conformance_revision,conformance_digest)
    REFERENCES harness_route_conformance_history_v2(conformance_id,revision,digest)
);
CREATE TABLE IF NOT EXISTS harness_route_current_v2(
  current_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL,
  digest TEXT NOT NULL,
  FOREIGN KEY(current_id,revision,digest)
    REFERENCES harness_route_current_history_v2(current_id,revision,digest)
);
CREATE TABLE IF NOT EXISTS harness_route_verified_compile_v2(
  compile_digest TEXT PRIMARY KEY,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS harness_route_active_history_v2(
  active_route_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(active_route_id,revision,digest)
);
CREATE TABLE IF NOT EXISTS harness_route_active_current_v2(
  active_route_id TEXT PRIMARY KEY,
  revision INTEGER NOT NULL,
  digest TEXT NOT NULL,
  FOREIGN KEY(active_route_id,revision,digest)
    REFERENCES harness_route_active_history_v2(active_route_id,revision,digest)
);
CREATE TABLE IF NOT EXISTS harness_route_wiring_history_v2(
  inventory_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  digest TEXT NOT NULL,
  row_digest TEXT NOT NULL,
  canonical_json BLOB NOT NULL,
  PRIMARY KEY(inventory_id,revision,digest)
);
CREATE TABLE IF NOT EXISTS harness_route_owner_publication_v2(
  compile_digest TEXT NOT NULL,
  binding_set_id TEXT NOT NULL,
  active_route_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  content_digest TEXT NOT NULL,
  refs_row_digest TEXT NOT NULL,
  refs_json BLOB NOT NULL,
  publication_row_digest TEXT NOT NULL,
  publication_json BLOB NOT NULL,
  PRIMARY KEY(compile_digest,binding_set_id,active_route_id,revision),
  FOREIGN KEY(compile_digest) REFERENCES harness_route_verified_compile_v2(compile_digest)
);
`
