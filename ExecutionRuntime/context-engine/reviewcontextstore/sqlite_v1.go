// SQLiteV1 is the Context Owner single-node durable Reviewer Context
// repository. WAL and schema layout are implementation details. It makes no
// multi-node HA, backup, remote durability, composition-root or SLA claim.
package reviewcontextstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	reviewcontract "github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	_ "modernc.org/sqlite"
)

const reviewerContextSQLiteSchemaVersionV1 = 1

type SQLiteConfigV1 struct {
	Path         string
	BusyTimeout  time.Duration
	MaxOpenConns int
}

type SQLiteV1 struct {
	db *sql.DB

	faultMu       sync.Mutex
	failNextStage bool
	loseNextReply bool
}

func OpenSQLiteV1(ctx context.Context, config SQLiteConfigV1) (*SQLiteV1, error) {
	if err := sqliteContextV1(ctx, "open"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.Path) == "" {
		return nil, invalidV1("Reviewer Context sqlite path is required")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Reviewer Context sqlite busy timeout exceeds its bound")
	}
	if config.MaxOpenConns <= 0 {
		// One process-local writer domain is deliberate. SQLite file locks still
		// protect the file, but this package makes no cross-process HA claim.
		config.MaxOpenConns = 1
	}
	if config.MaxOpenConns > 8 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Reviewer Context sqlite connection count exceeds its bounded writer domain")
	}
	abs, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, invalidV1("Reviewer Context sqlite path is invalid")
	}
	dsn := (&url.URL{Scheme: "file", Path: abs}).String()
	dsn += fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(FULL)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapSQLiteErrorV1(ctx, err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	store := &SQLiteV1{db: db}
	if err := store.migrateV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.verifyPragmasV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

var _ RepositoryV1 = (*SQLiteV1)(nil)

func (s *SQLiteV1) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteV1) migrateV1(ctx context.Context) error {
	tx, err := s.beginV1(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	digest := core.DigestBytes([]byte(reviewerContextSQLiteSchemaV1))
	var schemaTable string
	err = tx.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='context_reviewer_context_schema'`).Scan(&schemaTable)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, reviewerContextSQLiteSchemaV1); err != nil {
			return mapSQLiteErrorV1(ctx, err, true)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO context_reviewer_context_schema(version,digest) VALUES(?,?)`, reviewerContextSQLiteSchemaVersionV1, string(digest)); err != nil {
			return mapSQLiteErrorV1(ctx, err, true)
		}
	} else if err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	} else {
		var versionCount, maximumVersion int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MAX(version),0) FROM context_reviewer_context_schema`).Scan(&versionCount, &maximumVersion); err != nil {
			return mapSQLiteErrorV1(ctx, err, false)
		}
		if versionCount != 1 || maximumVersion != reviewerContextSQLiteSchemaVersionV1 {
			return conflictDigestV1("Reviewer Context sqlite schema version set is unsupported")
		}
		var stored string
		if err := tx.QueryRowContext(ctx, `SELECT digest FROM context_reviewer_context_schema WHERE version=?`, reviewerContextSQLiteSchemaVersionV1).Scan(&stored); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return conflictDigestV1("Reviewer Context sqlite schema version record is missing")
			}
			return mapSQLiteErrorV1(ctx, err, false)
		}
		if stored != string(digest) {
			return conflictDigestV1("Reviewer Context sqlite schema digest drifted")
		}
		for _, object := range []struct {
			kind string
			name string
		}{
			{kind: "table", name: "context_reviewer_context_history"},
			{kind: "table", name: "context_reviewer_context_current"},
			{kind: "index", name: "context_reviewer_context_current_exact"},
		} {
			var count int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type=? AND name=?`, object.kind, object.name).Scan(&count); err != nil {
				return mapSQLiteErrorV1(ctx, err, false)
			}
			if count != 1 {
				return conflictDigestV1("Reviewer Context sqlite required schema object is missing")
			}
		}
	}
	return commitSQLiteV1(ctx, tx)
}

func (s *SQLiteV1) verifyPragmasV1(ctx context.Context) error {
	var journal string
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	}
	if !strings.EqualFold(journal, "wal") {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Reviewer Context sqlite WAL mode is not active")
	}
	var synchronous int
	if err := s.db.QueryRowContext(ctx, `PRAGMA synchronous`).Scan(&synchronous); err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	}
	if synchronous != 2 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Reviewer Context sqlite FULL synchronous mode is not active")
	}
	var foreignKeys int
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	}
	if foreignKeys != 1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Reviewer Context sqlite foreign keys are not active")
	}
	return nil
}

func (s *SQLiteV1) IntegrityCheckV1(ctx context.Context) error {
	if s == nil || s.db == nil {
		return invalidV1("Reviewer Context sqlite repository is unavailable")
	}
	if err := sqliteContextV1(ctx, "integrity check"); err != nil {
		return err
	}
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	}
	if result != "ok" {
		return conflictDigestV1("Reviewer Context sqlite integrity check failed")
	}
	return nil
}

func (s *SQLiteV1) CommitV1(ctx context.Context, request reviewport.ReviewerContextPublishRequestV1) (reviewport.ReviewerContextPublishReceiptV1, error) {
	if s == nil || s.db == nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, invalidV1("Reviewer Context sqlite repository is unavailable")
	}
	if err := sqliteContextV1(ctx, "publish"); err != nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, err
	}
	value := request.Value.Clone()
	payload, rowDigest, err := encodeReviewerContextRowV1(value)
	if err != nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, err
	}
	tx, err := s.beginV1(ctx)
	if err != nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, err
	}
	defer func() { _ = tx.Rollback() }()

	existing, loadErr := loadSQLiteHistoricalV1(ctx, tx, value.Ref)
	if loadErr == nil {
		if !reflect.DeepEqual(existing, value) || !sqliteReplayPredecessorExactV1(ctx, tx, request) {
			return reviewport.ReviewerContextPublishReceiptV1{}, conflictV1("Reviewer Context sqlite revision already binds different canonical content or predecessor")
		}
		current, highest, currentErr := loadSQLiteCurrentV1(ctx, tx, value.Ref.TenantID, value.Ref.ID)
		if currentErr != nil {
			return reviewport.ReviewerContextPublishReceiptV1{}, currentErr
		}
		maximum, maximumErr := loadSQLiteMaximumRevisionV1(ctx, tx, value.Ref.TenantID, value.Ref.ID)
		if maximumErr != nil {
			return reviewport.ReviewerContextPublishReceiptV1{}, maximumErr
		}
		if highest < value.Ref.Revision || current.Revision != highest || maximum != highest {
			return reviewport.ReviewerContextPublishReceiptV1{}, conflictV1("Reviewer Context sqlite replay found an inconsistent current/highest closure")
		}
		return reviewport.ReviewerContextPublishReceiptV1{Ref: value.Ref, Created: false}, nil
	}
	if !core.HasCategory(loadErr, core.ErrorNotFound) {
		return reviewport.ReviewerContextPublishReceiptV1{}, loadErr
	}

	current, highest, currentErr := loadSQLiteCurrentV1(ctx, tx, value.Ref.TenantID, value.Ref.ID)
	maximum, maximumErr := loadSQLiteMaximumRevisionV1(ctx, tx, value.Ref.TenantID, value.Ref.ID)
	if request.Previous == nil {
		if currentErr != nil && !core.HasCategory(currentErr, core.ErrorNotFound) {
			return reviewport.ReviewerContextPublishReceiptV1{}, currentErr
		}
		if maximumErr != nil && !core.HasCategory(maximumErr, core.ErrorNotFound) {
			return reviewport.ReviewerContextPublishReceiptV1{}, maximumErr
		}
		if currentErr == nil || maximumErr == nil {
			return reviewport.ReviewerContextPublishReceiptV1{}, conflictV1("Reviewer Context sqlite create-once identity already exists")
		}
	} else {
		if currentErr != nil || maximumErr != nil {
			return reviewport.ReviewerContextPublishReceiptV1{}, closedSQLiteRepositoryErrorV1(currentErr, maximumErr)
		}
		if current != *request.Previous || highest != request.Previous.Revision || maximum != highest || value.Ref.Revision != highest+1 {
			return reviewport.ReviewerContextPublishReceiptV1{}, conflictV1("Reviewer Context sqlite current full-ref/highest CAS failed")
		}
		previous, previousErr := loadSQLiteHistoricalV1(ctx, tx, *request.Previous)
		if previousErr != nil {
			return reviewport.ReviewerContextPublishReceiptV1{}, previousErr
		}
		if previous.Ref != *request.Previous || previous.Subject != value.Subject {
			return reviewport.ReviewerContextPublishReceiptV1{}, conflictV1("Reviewer Context sqlite predecessor or exact subject drifted")
		}
	}
	if s.consumeStageFailureV1() {
		return reviewport.ReviewerContextPublishReceiptV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Reviewer Context sqlite staged failure")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO context_reviewer_context_history(tenant_id,envelope_id,revision,envelope_digest,row_digest,payload) VALUES(?,?,?,?,?,?)`, string(value.Ref.TenantID), value.Ref.ID, uint64(value.Ref.Revision), string(value.Ref.Digest), string(rowDigest), payload); err != nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, mapSQLiteErrorV1(ctx, err, true)
	}
	if request.Previous == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO context_reviewer_context_current(tenant_id,envelope_id,revision,envelope_digest,highest_revision) VALUES(?,?,?,?,?)`, string(value.Ref.TenantID), value.Ref.ID, uint64(value.Ref.Revision), string(value.Ref.Digest), uint64(value.Ref.Revision))
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE context_reviewer_context_current SET revision=?,envelope_digest=?,highest_revision=? WHERE tenant_id=? AND envelope_id=? AND revision=? AND envelope_digest=? AND highest_revision=?`, uint64(value.Ref.Revision), string(value.Ref.Digest), uint64(value.Ref.Revision), string(value.Ref.TenantID), value.Ref.ID, uint64(request.Previous.Revision), string(request.Previous.Digest), uint64(request.Previous.Revision))
		if err == nil {
			affected, rowsErr := result.RowsAffected()
			if rowsErr != nil {
				err = rowsErr
			} else if affected != 1 {
				return reviewport.ReviewerContextPublishReceiptV1{}, conflictV1("Reviewer Context sqlite current CAS affected no exact row")
			}
		}
	}
	if err != nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, mapSQLiteErrorV1(ctx, err, true)
	}
	if err := commitSQLiteV1(ctx, tx); err != nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, err
	}
	if s.consumeLostReplyV1() {
		return reviewport.ReviewerContextPublishReceiptV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Reviewer Context sqlite publish reply was lost")
	}
	return reviewport.ReviewerContextPublishReceiptV1{Ref: value.Ref, Created: true}, nil
}

func (s *SQLiteV1) ResolveV1(ctx context.Context, subject reviewcontract.ReviewerContextSubjectV1) (reviewcontract.ReviewerContextEnvelopeRefV1, error) {
	if s == nil || s.db == nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, invalidV1("Reviewer Context sqlite repository is unavailable")
	}
	if err := sqliteContextV1(ctx, "current resolve"); err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, err
	}
	if err := subject.Validate(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, err
	}
	id, err := reviewcontract.DeriveReviewerContextEnvelopeIDV1(subject)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, err
	}
	tx, err := s.beginV1(ctx)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	ref, highest, err := loadSQLiteCurrentV1(ctx, tx, subject.TenantID, id)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, err
	}
	maximum, err := loadSQLiteMaximumRevisionV1(ctx, tx, subject.TenantID, id)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, err
	}
	if highest != ref.Revision || maximum != highest {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, conflictV1("Reviewer Context sqlite current/highest/history closure drifted")
	}
	value, err := loadSQLiteHistoricalV1(ctx, tx, ref)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, err
	}
	if value.Subject != subject {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, conflictV1("Reviewer Context sqlite current exact subject drifted")
	}
	return ref, nil
}

func (s *SQLiteV1) InspectCurrentV1(ctx context.Context, subject reviewcontract.ReviewerContextSubjectV1, expected reviewcontract.ReviewerContextEnvelopeRefV1) (reviewcontract.ReviewerContextEnvelopeV1, error) {
	if s == nil || s.db == nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, invalidV1("Reviewer Context sqlite repository is unavailable")
	}
	if err := sqliteContextV1(ctx, "current inspect"); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	if err := subject.Validate(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	id, err := reviewcontract.DeriveReviewerContextEnvelopeIDV1(subject)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	if expected.TenantID != subject.TenantID || expected.ID != id {
		return reviewcontract.ReviewerContextEnvelopeV1{}, conflictV1("Reviewer Context sqlite expected ref does not belong to exact subject")
	}
	tx, err := s.beginV1(ctx)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, highest, err := loadSQLiteCurrentV1(ctx, tx, subject.TenantID, id)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	maximum, err := loadSQLiteMaximumRevisionV1(ctx, tx, subject.TenantID, id)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	if current != expected || highest != expected.Revision || maximum != highest {
		return reviewcontract.ReviewerContextEnvelopeV1{}, conflictV1("Reviewer Context sqlite current full-ref/highest/history drifted")
	}
	value, err := loadSQLiteHistoricalV1(ctx, tx, expected)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	if value.Subject != subject {
		return reviewcontract.ReviewerContextEnvelopeV1{}, conflictV1("Reviewer Context sqlite current exact subject drifted")
	}
	return value.Clone(), nil
}

func (s *SQLiteV1) InspectHistoricalV1(ctx context.Context, exact reviewcontract.ReviewerContextEnvelopeRefV1) (reviewcontract.ReviewerContextEnvelopeV1, error) {
	if s == nil || s.db == nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, invalidV1("Reviewer Context sqlite repository is unavailable")
	}
	if err := sqliteContextV1(ctx, "historical inspect"); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	if err := exact.Validate(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	return loadSQLiteHistoricalV1(ctx, s.db, exact)
}

type sqliteQueryRowerV1 interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadSQLiteHistoricalV1(ctx context.Context, source sqliteQueryRowerV1, exact reviewcontract.ReviewerContextEnvelopeRefV1) (reviewcontract.ReviewerContextEnvelopeV1, error) {
	var envelopeDigest, rowDigest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT envelope_digest,row_digest,payload FROM context_reviewer_context_history WHERE tenant_id=? AND envelope_id=? AND revision=?`, string(exact.TenantID), exact.ID, uint64(exact.Revision)).Scan(&envelopeDigest, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return reviewcontract.ReviewerContextEnvelopeV1{}, notFoundV1("Reviewer Context sqlite historical envelope was not found")
	}
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, mapSQLiteErrorV1(ctx, err, false)
	}
	if envelopeDigest != string(exact.Digest) {
		return reviewcontract.ReviewerContextEnvelopeV1{}, conflictDigestV1("Reviewer Context sqlite historical exact digest drifted")
	}
	value, err := decodeReviewerContextRowV1(payload, rowDigest)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	if value.Ref != exact || string(value.Ref.Digest) != envelopeDigest {
		return reviewcontract.ReviewerContextEnvelopeV1{}, conflictDigestV1("Reviewer Context sqlite historical exact ref drifted")
	}
	return value.Clone(), nil
}

func loadSQLiteCurrentV1(ctx context.Context, source sqliteQueryRowerV1, tenant core.TenantID, id string) (reviewcontract.ReviewerContextEnvelopeRefV1, core.Revision, error) {
	var revision, highest uint64
	var digest string
	err := source.QueryRowContext(ctx, `SELECT revision,envelope_digest,highest_revision FROM context_reviewer_context_current WHERE tenant_id=? AND envelope_id=?`, string(tenant), id).Scan(&revision, &digest, &highest)
	if errors.Is(err, sql.ErrNoRows) {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, 0, notFoundV1("Reviewer Context sqlite current envelope was not found")
	}
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, 0, mapSQLiteErrorV1(ctx, err, false)
	}
	ref := reviewcontract.ReviewerContextEnvelopeRefV1{TenantID: tenant, ID: id, Revision: core.Revision(revision), Digest: core.Digest(digest)}
	if ref.Validate() != nil || highest == 0 {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, 0, conflictDigestV1("Reviewer Context sqlite current row is invalid")
	}
	return ref, core.Revision(highest), nil
}

func loadSQLiteMaximumRevisionV1(ctx context.Context, source sqliteQueryRowerV1, tenant core.TenantID, id string) (core.Revision, error) {
	var maximum sql.NullInt64
	if err := source.QueryRowContext(ctx, `SELECT MAX(revision) FROM context_reviewer_context_history WHERE tenant_id=? AND envelope_id=?`, string(tenant), id).Scan(&maximum); err != nil {
		return 0, mapSQLiteErrorV1(ctx, err, false)
	}
	if !maximum.Valid || maximum.Int64 <= 0 {
		return 0, notFoundV1("Reviewer Context sqlite identity has no history")
	}
	return core.Revision(maximum.Int64), nil
}

func sqliteReplayPredecessorExactV1(ctx context.Context, source sqliteQueryRowerV1, request reviewport.ReviewerContextPublishRequestV1) bool {
	if request.Value.Ref.Revision == 1 {
		return request.Previous == nil
	}
	if request.Previous == nil || request.Previous.Revision+1 != request.Value.Ref.Revision {
		return false
	}
	previous, err := loadSQLiteHistoricalV1(ctx, source, *request.Previous)
	return err == nil && previous.Ref == *request.Previous && previous.Subject == request.Value.Subject
}

func (s *SQLiteV1) beginV1(ctx context.Context) (*sql.Tx, error) {
	if err := sqliteContextV1(ctx, "transaction"); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, mapSQLiteErrorV1(ctx, err, true)
	}
	return tx, nil
}

func commitSQLiteV1(ctx context.Context, tx *sql.Tx) error {
	if err := tx.Commit(); err != nil {
		return mapSQLiteErrorV1(ctx, err, true)
	}
	return nil
}

func sqliteContextV1(ctx context.Context, operation string) error {
	if ctx == nil || ctx.Err() != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Reviewer Context sqlite "+operation+" context ended")
	}
	return nil
}

func mapSQLiteErrorV1(ctx context.Context, err error, mutation bool) error {
	if err == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Reviewer Context sqlite outcome is indeterminate")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "locked") || strings.Contains(message, "busy") {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Reviewer Context sqlite is busy")
	}
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "Reviewer Context sqlite uniqueness conflict")
	}
	if mutation {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Reviewer Context sqlite mutation outcome is unknown")
	}
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Reviewer Context sqlite read failed")
}

func closedSQLiteRepositoryErrorV1(errorsIn ...error) error {
	for _, err := range errorsIn {
		if err != nil && !core.HasCategory(err, core.ErrorNotFound) {
			return err
		}
	}
	return conflictV1("Reviewer Context sqlite expected current/history closure is missing")
}

func conflictDigestV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, message)
}

func (s *SQLiteV1) consumeStageFailureV1() bool {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	if !s.failNextStage {
		return false
	}
	s.failNextStage = false
	return true
}

func (s *SQLiteV1) consumeLostReplyV1() bool {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	if !s.loseNextReply {
		return false
	}
	s.loseNextReply = false
	return true
}
