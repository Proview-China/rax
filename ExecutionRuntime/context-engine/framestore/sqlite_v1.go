package framestore

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

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
	_ "modernc.org/sqlite"
)

const MaxFrameExactCurrentTTLV1 = 30 * time.Second

type SQLiteConfigV1 struct {
	Path        string
	Owner       contract.OwnerRef
	BusyTimeout time.Duration
	Clock       func() time.Time
	MaxTTL      time.Duration
}

type SQLiteV1 struct {
	db     *sql.DB
	owner  contract.OwnerRef
	clock  func() time.Time
	maxTTL time.Duration

	faultMu         sync.Mutex
	failNextStage   bool
	loseNextReply   bool
	beforeS2        func()
	beforePointerS2 func()
}

type ledgerRecordV1 struct {
	OperationID     string
	Owner           contract.OwnerRef
	Scope           contract.Digest
	RunID           string
	SessionRef      contract.FactRef
	Expected        *contract.ContextGenerationCurrentPointerV1
	Next            contract.ContextGenerationCurrentPointerV1
	FrameRef        contract.FactRef
	StateRowDigest  contract.Digest
	CreatedUnixNano int64
	Receipt         CommitReceiptV1
}

func (r ledgerRecordV1) digestValue() (contract.Digest, error) {
	return contract.DigestJSON(struct {
		Domain string
		Record ledgerRecordV1
	}{"praxis.context/frame-store-ledger-row-v1", r})
}

func (r ledgerRecordV1) Validate() error {
	if !validIDV1(r.OperationID) || r.Owner.Validate() != nil || r.Scope.Validate() != nil ||
		!validIDV1(r.RunID) || r.SessionRef.Validate() != nil || r.Next.Validate() != nil ||
		r.FrameRef.Validate() != nil || r.StateRowDigest.Validate() != nil ||
		r.CreatedUnixNano <= 0 || r.Receipt.Validate() != nil ||
		r.Next.ExecutionScopeDigest != r.Scope || r.Next.RunID != r.RunID ||
		r.Next.SessionRef != r.SessionRef || r.Receipt.OperationID != r.OperationID ||
		r.Receipt.FrameRef != r.FrameRef || r.Receipt.Pointer != r.Next {
		return fmt.Errorf("%w: frame store ledger record", contract.ErrConflict)
	}
	if r.Expected != nil {
		if r.Expected.Validate() != nil || !sameLineageV1(*r.Expected, r.Next) ||
			r.Expected.ID != r.Next.ID || r.Expected.Revision+1 != r.Next.Revision {
			return fmt.Errorf("%w: frame store ledger predecessor", contract.ErrConflict)
		}
	}
	return nil
}

var (
	_ contextports.ContextFrameExactCurrentReaderV1        = (*SQLiteV1)(nil)
	_ contextports.ContextParentFrameSourceBindingReaderV1 = (*SQLiteV1)(nil)
	_ contextports.ContextFrameMetadataReaderV1            = (*SQLiteV1)(nil)
	_ contextports.ContextManifestMetadataReaderV1         = (*SQLiteV1)(nil)
	_ contextports.ContextGenerationMetadataReaderV1       = (*SQLiteV1)(nil)
	_ contextports.ContextGenerationCurrentPointerReaderV1 = (*SQLiteV1)(nil)
)

func OpenSQLiteV1(ctx context.Context, config SQLiteConfigV1) (*SQLiteV1, error) {
	if err := checkContextSQLiteV1(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.Path) == "" || config.Owner.Validate() != nil {
		return nil, fmt.Errorf("%w: frame store sqlite configuration", contract.ErrInvalid)
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, fmt.Errorf("%w: frame store sqlite busy timeout", contract.ErrLimitExceeded)
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.MaxTTL <= 0 {
		config.MaxTTL = MaxFrameExactCurrentTTLV1
	}
	if config.MaxTTL > MaxFrameExactCurrentTTLV1 {
		return nil, fmt.Errorf("%w: frame store sqlite observation TTL", contract.ErrLimitExceeded)
	}
	absolute, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, fmt.Errorf("%w: frame store sqlite path", contract.ErrInvalid)
	}
	dsn := (&url.URL{Scheme: "file", Path: absolute}).String()
	dsn += fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(FULL)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapSQLiteErrorV1(ctx, err, false)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &SQLiteV1{db: db, owner: config.Owner, clock: config.Clock, maxTTL: config.MaxTTL}
	if err := store.migrateV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.verifyPragmasV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := verifyPhysicalSchemaV1(ctx, db); err != nil {
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

// ContextOwnerRefV1 exposes only the reader's immutable Owner binding. It is
// used by the Context lineage constructor to reject cross-Owner reader splice.
func (s *SQLiteV1) ContextOwnerRefV1() contract.OwnerRef {
	if s == nil {
		return contract.OwnerRef{}
	}
	return s.owner
}

func (s *SQLiteV1) IntegrityCheckV1(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("%w: frame store sqlite repository", contract.ErrUnavailable)
	}
	if err := checkContextSQLiteV1(ctx); err != nil {
		return err
	}
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	}
	if result != "ok" {
		return fmt.Errorf("%w: frame store sqlite integrity", contract.ErrConflict)
	}
	return verifyPhysicalSchemaV1(ctx, s.db)
}

func (s *SQLiteV1) CommitCurrentV1(
	ctx context.Context,
	operationID string,
	input CurrentStateV1,
	expected *contract.ContextGenerationCurrentPointerV1,
	nowUnixNano int64,
) (CommitReceiptV1, error) {
	if s == nil || s.db == nil {
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store sqlite repository", contract.ErrUnavailable)
	}
	if err := checkContextSQLiteV1(ctx); err != nil {
		return CommitReceiptV1{}, err
	}
	if !validIDV1(operationID) {
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store operation ID", contract.ErrInvalid)
	}
	checked := s.clock()
	state, err := normalizeStateV1(input)
	if err != nil {
		return CommitReceiptV1{}, err
	}
	if expected != nil && expected.Validate() != nil {
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store expected current", contract.ErrInvalid)
	}
	frameRef, manifestRef, generationRef, err := stateRefsV1(state)
	if err != nil {
		return CommitReceiptV1{}, err
	}
	payload, rowDigest, err := encodeStateRowV1(state)
	if err != nil {
		return CommitReceiptV1{}, err
	}
	receipt, err := sealCommitReceiptV1(CommitReceiptV1{
		OperationID: operationID,
		FrameRef:    frameRef,
		Pointer:     state.Pointer,
		Created:     true,
	})
	if err != nil {
		return CommitReceiptV1{}, err
	}
	receiptPayload, _, err := encodeReceiptRowV1(receipt)
	if err != nil {
		return CommitReceiptV1{}, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return CommitReceiptV1{}, mapSQLiteErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()

	existing, ledgerErr := loadLedgerV1(ctx, tx, s.owner, operationID)
	if ledgerErr == nil {
		if existing.StateRowDigest != rowDigest ||
			existing.Owner != s.owner ||
			existing.Scope != state.Pointer.ExecutionScopeDigest ||
			existing.RunID != state.Pointer.RunID ||
			existing.SessionRef != state.Pointer.SessionRef ||
			existing.FrameRef != frameRef ||
			existing.Next != state.Pointer ||
			!sameOptionalPointerV1(existing.Expected, expected) {
			return CommitReceiptV1{}, fmt.Errorf("%w: frame store operation replay drift", contract.ErrConflict)
		}
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store operation already committed; inspect original operation", contract.ErrInspectOnly)
	}
	if !errors.Is(ledgerErr, contract.ErrNotFound) {
		return CommitReceiptV1{}, ledgerErr
	}
	if checked.IsZero() {
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store commit clock", contract.ErrInvalid)
	}
	if nowUnixNano <= 0 {
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store observed commit time", contract.ErrInvalid)
	}
	if checked.UnixNano() < nowUnixNano {
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store commit clock rollback", contract.ErrConflict)
	}
	if err := validateStateAtV1(ctx, state, s.owner, checked.UnixNano()); err != nil {
		return CommitReceiptV1{}, err
	}

	current, currentErr := loadCurrentPointerForStateV1(ctx, tx, s.owner, state)
	maximum, maximumErr := loadMaximumPointerRevisionV1(ctx, tx, s.owner, state.Pointer)
	if expected == nil {
		if state.Pointer.Revision != 1 || currentErr == nil || maximumErr == nil ||
			!errors.Is(currentErr, contract.ErrNotFound) ||
			!errors.Is(maximumErr, contract.ErrNotFound) {
			return CommitReceiptV1{}, fmt.Errorf("%w: frame store initial current", contract.ErrConflict)
		}
	} else {
		currentState, loadErr := loadCurrentStateByRequestV1(ctx, tx, s.owner, contract.ContextGenerationCurrentPointerRequestV1{
			ExecutionScopeDigest: expected.ExecutionScopeDigest,
			RunID:                expected.RunID,
			SessionRef:           expected.SessionRef,
			Turn:                 expected.Turn,
		})
		if loadErr != nil {
			return CommitReceiptV1{}, fmt.Errorf("%w: frame store current CAS", contract.ErrConflict)
		}
		if err := validateStateAtV1(ctx, currentState, s.owner, checked.UnixNano()); err != nil {
			return CommitReceiptV1{}, err
		}
		current, currentErr = loadCurrentPointerForStateV1(ctx, tx, s.owner, currentState)
		maximum, maximumErr = loadMaximumPointerRevisionV1(ctx, tx, s.owner, currentState.Pointer)
		if currentErr != nil || maximumErr != nil || current != *expected ||
			maximum != expected.Revision || !sameLineageV1(*expected, state.Pointer) ||
			state.Pointer.ID != expected.ID || state.Pointer.Revision != expected.Revision+1 ||
			state.Pointer.Turn < expected.Turn || state.Pointer.Turn > expected.Turn+1 {
			return CommitReceiptV1{}, fmt.Errorf("%w: frame store current CAS", contract.ErrConflict)
		}
	}

	if _, err = tx.ExecContext(ctx, `
INSERT INTO context_frame_store_history(
 owner_component_id,owner_binding_digest,execution_scope_digest,run_id,
 session_id,session_revision,session_digest,turn,
 source_kind,source_id,source_revision,source_digest,
 frame_id,frame_revision,frame_digest,
 manifest_id,manifest_revision,manifest_digest,
 generation_id,generation_revision,generation_digest,
 pointer_id,pointer_revision,pointer_digest,expires_unix_nano,row_digest,payload
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		s.owner.ComponentID, string(s.owner.BindingDigest), string(state.Frame.Execution.ScopeDigest), state.Frame.Execution.RunID,
		state.Binding.Subject.SessionRef.ID, state.Binding.Subject.SessionRef.Revision, string(state.Binding.Subject.SessionRef.Digest), state.Frame.Execution.Turn,
		state.Binding.Source.Kind, state.Binding.Source.ID, state.Binding.Source.Revision, string(state.Binding.Source.Digest),
		frameRef.ID, frameRef.Revision, string(frameRef.Digest),
		manifestRef.ID, manifestRef.Revision, string(manifestRef.Digest),
		generationRef.ID, generationRef.Revision, string(generationRef.Digest),
		state.Pointer.ID, state.Pointer.Revision, string(state.Pointer.Digest), stateExpiryV1(state), string(rowDigest), payload,
	); err != nil {
		return CommitReceiptV1{}, mapSQLiteErrorV1(ctx, err, true)
	}
	if s.consumeFailNextStageV1() {
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store staged fault", contract.ErrUnavailable)
	}
	if expected == nil {
		_, err = tx.ExecContext(ctx, `
INSERT INTO context_frame_store_current(
 owner_component_id,owner_binding_digest,execution_scope_digest,run_id,
 session_id,session_revision,session_digest,turn,
 frame_id,frame_revision,frame_digest,pointer_id,pointer_revision,pointer_digest,highest_pointer_revision
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			s.owner.ComponentID, string(s.owner.BindingDigest), string(state.Pointer.ExecutionScopeDigest), state.Pointer.RunID,
			state.Pointer.SessionRef.ID, state.Pointer.SessionRef.Revision, string(state.Pointer.SessionRef.Digest), state.Pointer.Turn,
			frameRef.ID, frameRef.Revision, string(frameRef.Digest),
			state.Pointer.ID, state.Pointer.Revision, string(state.Pointer.Digest), state.Pointer.Revision,
		)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `
UPDATE context_frame_store_current
SET turn=?,frame_id=?,frame_revision=?,frame_digest=?,pointer_id=?,pointer_revision=?,pointer_digest=?,highest_pointer_revision=?
WHERE owner_component_id=? AND owner_binding_digest=? AND execution_scope_digest=? AND run_id=?
  AND session_id=? AND session_revision=? AND session_digest=?
  AND turn=? AND pointer_id=? AND pointer_revision=? AND pointer_digest=? AND highest_pointer_revision=?`,
			state.Pointer.Turn, frameRef.ID, frameRef.Revision, string(frameRef.Digest),
			state.Pointer.ID, state.Pointer.Revision, string(state.Pointer.Digest), state.Pointer.Revision,
			s.owner.ComponentID, string(s.owner.BindingDigest), string(expected.ExecutionScopeDigest), expected.RunID,
			expected.SessionRef.ID, expected.SessionRef.Revision, string(expected.SessionRef.Digest),
			expected.Turn, expected.ID, expected.Revision, string(expected.Digest), expected.Revision,
		)
		if err == nil {
			affected, rowsErr := result.RowsAffected()
			if rowsErr != nil {
				err = rowsErr
			} else if affected != 1 {
				return CommitReceiptV1{}, fmt.Errorf("%w: frame store current CAS affected no row", contract.ErrConflict)
			}
		}
	}
	if err != nil {
		return CommitReceiptV1{}, mapSQLiteErrorV1(ctx, err, true)
	}
	var expectedID, expectedDigest any
	var expectedRevision any
	if expected != nil {
		expectedID, expectedRevision, expectedDigest = expected.ID, expected.Revision, string(expected.Digest)
	}
	ledgerRecord := ledgerRecordV1{
		OperationID:     operationID,
		Owner:           s.owner,
		Scope:           state.Pointer.ExecutionScopeDigest,
		RunID:           state.Pointer.RunID,
		SessionRef:      state.Pointer.SessionRef,
		Expected:        expected,
		Next:            state.Pointer,
		FrameRef:        frameRef,
		StateRowDigest:  rowDigest,
		CreatedUnixNano: checked.UnixNano(),
		Receipt:         receipt,
	}
	if err := ledgerRecord.Validate(); err != nil {
		return CommitReceiptV1{}, err
	}
	ledgerRowDigest, err := ledgerRecord.digestValue()
	if err != nil {
		return CommitReceiptV1{}, err
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO context_frame_store_ledger(
 operation_id,owner_component_id,owner_binding_digest,execution_scope_digest,run_id,
 session_id,session_revision,session_digest,
 expected_pointer_id,expected_pointer_revision,expected_pointer_digest,
 next_pointer_id,next_pointer_revision,next_pointer_digest,
 frame_id,frame_revision,frame_digest,state_row_digest,created_unix_nano,row_digest,payload
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		operationID, s.owner.ComponentID, string(s.owner.BindingDigest), string(state.Pointer.ExecutionScopeDigest), state.Pointer.RunID,
		state.Pointer.SessionRef.ID, state.Pointer.SessionRef.Revision, string(state.Pointer.SessionRef.Digest),
		expectedID, expectedRevision, expectedDigest,
		state.Pointer.ID, state.Pointer.Revision, string(state.Pointer.Digest),
		frameRef.ID, frameRef.Revision, string(frameRef.Digest), string(rowDigest), checked.UnixNano(), string(ledgerRowDigest), receiptPayload,
	); err != nil {
		return CommitReceiptV1{}, mapSQLiteErrorV1(ctx, err, true)
	}
	completed := s.clock()
	if completed.IsZero() || completed.Before(checked) {
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store commit clock regression", contract.ErrConflict)
	}
	if completed.UnixNano() >= stateExpiryV1(state) {
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store commit TTL crossing", contract.ErrExpired)
	}
	if err := ctx.Err(); err != nil {
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store mutation outcome", contract.ErrUnknown)
	}
	if err = tx.Commit(); err != nil {
		return CommitReceiptV1{}, mapSQLiteErrorV1(ctx, err, true)
	}
	if s.consumeLoseNextReplyV1() {
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store commit reply was lost; inspect original operation", contract.ErrUnknown)
	}
	return cloneV1(receipt)
}

func (s *SQLiteV1) InspectCommitV1(ctx context.Context, operationID string) (CommitReceiptV1, error) {
	if s == nil || s.db == nil {
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store sqlite repository", contract.ErrUnavailable)
	}
	if err := checkContextSQLiteV1(ctx); err != nil {
		return CommitReceiptV1{}, err
	}
	if !validIDV1(operationID) {
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store operation ID", contract.ErrInvalid)
	}
	record, err := loadLedgerV1(ctx, s.db, s.owner, operationID)
	if err != nil {
		return CommitReceiptV1{}, err
	}
	return cloneV1(record.Receipt)
}

func (s *SQLiteV1) InspectContextFrameExactCurrentV1(ctx context.Context, exact contract.FactRef, observedUnixNano int64) (contract.ContextFrameExactCurrentProjectionV1, error) {
	if s == nil || s.db == nil {
		return contract.ContextFrameExactCurrentProjectionV1{}, fmt.Errorf("%w: frame store sqlite repository", contract.ErrUnavailable)
	}
	if err := checkContextSQLiteV1(ctx); err != nil {
		return contract.ContextFrameExactCurrentProjectionV1{}, err
	}
	if exact.Validate() != nil || observedUnixNano <= 0 {
		return contract.ContextFrameExactCurrentProjectionV1{}, fmt.Errorf("%w: frame exact-current request", contract.ErrInvalid)
	}
	checked := s.clock()
	if checked.IsZero() {
		return contract.ContextFrameExactCurrentProjectionV1{}, fmt.Errorf("%w: frame exact-current clock", contract.ErrInvalid)
	}
	if checked.UnixNano() < observedUnixNano {
		return contract.ContextFrameExactCurrentProjectionV1{}, fmt.Errorf("%w: frame exact-current clock rollback", contract.ErrConflict)
	}
	s1, err := s.loadExactCurrentStateV1(ctx, exact, checked.UnixNano())
	if err != nil {
		return contract.ContextFrameExactCurrentProjectionV1{}, err
	}
	expires := stateExpiryV1(s1)
	if capExpiry := checked.Add(s.maxTTL).UnixNano(); capExpiry < expires {
		expires = capExpiry
	}
	if checked.UnixNano() >= expires {
		return contract.ContextFrameExactCurrentProjectionV1{}, fmt.Errorf("%w: frame exact-current S1 TTL", contract.ErrExpired)
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextFrameExactCurrentProjectionV1{}, err
	}
	s.runBeforeExactCurrentS2ForTestV1()
	s2, err := s.loadExactCurrentStateV1(ctx, exact, checked.UnixNano())
	if err != nil {
		return contract.ContextFrameExactCurrentProjectionV1{}, err
	}
	s1Digest, err := contract.DigestJSON(s1)
	if err != nil {
		return contract.ContextFrameExactCurrentProjectionV1{}, err
	}
	s2Digest, err := contract.DigestJSON(s2)
	if err != nil {
		return contract.ContextFrameExactCurrentProjectionV1{}, err
	}
	if s1Digest != s2Digest || stateExpiryV1(s2) != stateExpiryV1(s1) {
		return contract.ContextFrameExactCurrentProjectionV1{}, fmt.Errorf("%w: frame exact-current S1/S2 drift", contract.ErrConflict)
	}
	completed := s.clock()
	if completed.IsZero() || completed.Before(checked) {
		return contract.ContextFrameExactCurrentProjectionV1{}, fmt.Errorf("%w: frame exact-current clock regression", contract.ErrConflict)
	}
	if completed.UnixNano() >= expires {
		return contract.ContextFrameExactCurrentProjectionV1{}, fmt.Errorf("%w: frame exact-current TTL crossing", contract.ErrExpired)
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextFrameExactCurrentProjectionV1{}, err
	}
	return contract.SealContextFrameExactCurrentProjectionV1(contract.ContextFrameExactCurrentProjectionV1{
		FrameRef:        exact,
		Current:         true,
		CheckedUnixNano: checked.UnixNano(),
		ExpiresUnixNano: expires,
	}, completed.UnixNano())
}

func (s *SQLiteV1) ResolveExactSourceBinding(ctx context.Context, source contract.ContextParentFrameApplicabilitySourceCoordinateV1) (contract.ContextParentFrameSourceBindingV1, error) {
	if err := s.checkReadV1(ctx); err != nil {
		return contract.ContextParentFrameSourceBindingV1{}, err
	}
	if source.Validate() != nil {
		return contract.ContextParentFrameSourceBindingV1{}, fmt.Errorf("%w: frame source binding", contract.ErrInvalid)
	}
	state, err := loadStateBySourceV1(ctx, s.db, s.owner, source)
	if err != nil {
		return contract.ContextParentFrameSourceBindingV1{}, err
	}
	return cloneV1(state.Binding)
}

func (s *SQLiteV1) FrameByExactRef(ctx context.Context, ref contract.FactRef, scope contract.Digest) (contract.ContextFrame, error) {
	state, err := s.loadExactMetadataStateV1(ctx, "frame", ref, scope)
	if err != nil {
		return contract.ContextFrame{}, err
	}
	return cloneV1(state.Frame)
}

func (s *SQLiteV1) ManifestByExactRef(ctx context.Context, ref contract.FactRef, scope contract.Digest) (contract.ContextManifest, error) {
	state, err := s.loadExactMetadataStateV1(ctx, "manifest", ref, scope)
	if err != nil {
		return contract.ContextManifest{}, err
	}
	return cloneV1(state.Manifest)
}

func (s *SQLiteV1) GenerationByExactRef(ctx context.Context, ref contract.FactRef, scope contract.Digest) (contract.ContextGeneration, error) {
	state, err := s.loadExactMetadataStateV1(ctx, "generation", ref, scope)
	if err != nil {
		return contract.ContextGeneration{}, err
	}
	return cloneV1(state.Generation)
}

func (s *SQLiteV1) InspectCurrentGenerationPointer(ctx context.Context, request contract.ContextGenerationCurrentPointerRequestV1) (contract.ContextGenerationCurrentPointerV1, error) {
	if err := s.checkReadV1(ctx); err != nil {
		return contract.ContextGenerationCurrentPointerV1{}, err
	}
	if request.Validate() != nil {
		return contract.ContextGenerationCurrentPointerV1{}, fmt.Errorf("%w: generation current pointer request", contract.ErrInvalid)
	}
	checked := s.clock()
	if checked.IsZero() {
		return contract.ContextGenerationCurrentPointerV1{}, fmt.Errorf("%w: generation current pointer clock", contract.ErrInvalid)
	}
	s1, err := s.loadGenerationCurrentSnapshotV1(ctx, request, checked.UnixNano())
	if err != nil {
		return contract.ContextGenerationCurrentPointerV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextGenerationCurrentPointerV1{}, err
	}
	s.runBeforeGenerationCurrentS2ForTestV1()
	s2, err := s.loadGenerationCurrentSnapshotV1(ctx, request, checked.UnixNano())
	if err != nil {
		return contract.ContextGenerationCurrentPointerV1{}, err
	}
	s1Digest, err := contract.DigestJSON(s1)
	if err != nil {
		return contract.ContextGenerationCurrentPointerV1{}, err
	}
	s2Digest, err := contract.DigestJSON(s2)
	if err != nil {
		return contract.ContextGenerationCurrentPointerV1{}, err
	}
	if s1Digest != s2Digest {
		return contract.ContextGenerationCurrentPointerV1{}, fmt.Errorf("%w: generation current pointer S1/S2 drift", contract.ErrConflict)
	}
	completed := s.clock()
	if completed.IsZero() || completed.Before(checked) {
		return contract.ContextGenerationCurrentPointerV1{}, fmt.Errorf("%w: generation current pointer clock regression", contract.ErrConflict)
	}
	if completed.UnixNano() >= stateExpiryV1(s2) {
		return contract.ContextGenerationCurrentPointerV1{}, fmt.Errorf("%w: generation current pointer TTL crossing", contract.ErrExpired)
	}
	if err := ctx.Err(); err != nil {
		return contract.ContextGenerationCurrentPointerV1{}, err
	}
	return cloneV1(s2.Pointer)
}

func (s *SQLiteV1) loadGenerationCurrentSnapshotV1(
	ctx context.Context,
	request contract.ContextGenerationCurrentPointerRequestV1,
	nowUnixNano int64,
) (CurrentStateV1, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return CurrentStateV1{}, mapSQLiteErrorV1(ctx, err, false)
	}
	defer func() { _ = tx.Rollback() }()
	state, err := loadCurrentStateByRequestV1(ctx, tx, s.owner, request)
	if err != nil {
		return CurrentStateV1{}, err
	}
	if err := validateStateAtV1(ctx, state, s.owner, nowUnixNano); err != nil {
		return CurrentStateV1{}, err
	}
	current, err := loadCurrentPointerForStateV1(ctx, tx, s.owner, state)
	if err != nil {
		return CurrentStateV1{}, err
	}
	maximum, err := loadMaximumPointerRevisionV1(ctx, tx, s.owner, state.Pointer)
	if err != nil {
		return CurrentStateV1{}, err
	}
	if current != state.Pointer || maximum != state.Pointer.Revision {
		return CurrentStateV1{}, fmt.Errorf("%w: generation current pointer is not authoritative current", contract.ErrConflict)
	}
	if err := ctx.Err(); err != nil {
		return CurrentStateV1{}, err
	}
	return state, nil
}

func (s *SQLiteV1) loadExactMetadataStateV1(ctx context.Context, kind string, ref contract.FactRef, scope contract.Digest) (CurrentStateV1, error) {
	if err := s.checkReadV1(ctx); err != nil {
		return CurrentStateV1{}, err
	}
	if ref.Validate() != nil || scope.Validate() != nil {
		return CurrentStateV1{}, fmt.Errorf("%w: frame store exact metadata request", contract.ErrInvalid)
	}
	return loadStateByMetadataRefV1(ctx, s.db, s.owner, kind, ref, scope)
}

func (s *SQLiteV1) loadExactCurrentStateV1(ctx context.Context, exact contract.FactRef, nowUnixNano int64) (CurrentStateV1, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return CurrentStateV1{}, mapSQLiteErrorV1(ctx, err, false)
	}
	defer func() { _ = tx.Rollback() }()
	state, err := loadStateByFrameWithoutScopeV1(ctx, tx, s.owner, exact)
	if err != nil {
		return CurrentStateV1{}, err
	}
	if err := validateStateAtV1(ctx, state, s.owner, nowUnixNano); err != nil {
		return CurrentStateV1{}, err
	}
	current, err := loadCurrentPointerForStateV1(ctx, tx, s.owner, state)
	if err != nil {
		return CurrentStateV1{}, err
	}
	maximum, err := loadMaximumPointerRevisionV1(ctx, tx, s.owner, state.Pointer)
	if err != nil {
		return CurrentStateV1{}, err
	}
	if current != state.Pointer || maximum != state.Pointer.Revision {
		return CurrentStateV1{}, fmt.Errorf("%w: frame is historical, not authoritative current", contract.ErrConflict)
	}
	return state, nil
}

func (s *SQLiteV1) checkReadV1(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("%w: frame store sqlite repository", contract.ErrUnavailable)
	}
	return checkContextSQLiteV1(ctx)
}

func (s *SQLiteV1) migrateV1(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapSQLiteErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	digest := contract.DigestBytes([]byte(sqliteSchemaV1))
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='context_frame_store_schema'`).Scan(&count); err != nil {
		return mapSQLiteErrorV1(ctx, err, false)
	}
	if count == 0 {
		if _, err = tx.ExecContext(ctx, sqliteSchemaV1); err != nil {
			return mapSQLiteErrorV1(ctx, err, true)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO context_frame_store_schema(version,digest) VALUES(?,?)`, sqliteSchemaVersionV1, string(digest)); err != nil {
			return mapSQLiteErrorV1(ctx, err, true)
		}
	} else {
		var versionCount, maximumVersion int
		var stored string
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MAX(version),0) FROM context_frame_store_schema`).Scan(&versionCount, &maximumVersion); err != nil ||
			versionCount != 1 || maximumVersion != sqliteSchemaVersionV1 {
			return fmt.Errorf("%w: frame store schema version set", contract.ErrConflict)
		}
		if err = tx.QueryRowContext(ctx, `SELECT digest FROM context_frame_store_schema WHERE version=?`, sqliteSchemaVersionV1).Scan(&stored); err != nil || stored != string(digest) {
			return fmt.Errorf("%w: frame store schema digest", contract.ErrConflict)
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
		return fmt.Errorf("%w: frame store sqlite durability pragmas", contract.ErrConflict)
	}
	return nil
}

type queryRowerV1 interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadStateByFrameWithoutScopeV1(ctx context.Context, source queryRowerV1, owner contract.OwnerRef, exact contract.FactRef) (CurrentStateV1, error) {
	var count int
	if err := source.QueryRowContext(ctx, `
SELECT COUNT(*) FROM context_frame_store_history
WHERE owner_component_id=? AND owner_binding_digest=? AND frame_id=? AND frame_revision=?`,
		owner.ComponentID, string(owner.BindingDigest), exact.ID, exact.Revision).Scan(&count); err != nil {
		return CurrentStateV1{}, mapSQLiteErrorV1(ctx, err, false)
	}
	if count == 0 {
		var other int
		if err := source.QueryRowContext(ctx, `SELECT COUNT(*) FROM context_frame_store_history WHERE frame_id=? AND frame_revision=?`, exact.ID, exact.Revision).Scan(&other); err != nil {
			return CurrentStateV1{}, mapSQLiteErrorV1(ctx, err, false)
		}
		if other > 0 {
			return CurrentStateV1{}, fmt.Errorf("%w: frame Owner binding drift", contract.ErrConflict)
		}
		return CurrentStateV1{}, fmt.Errorf("%w: frame exact history", contract.ErrNotFound)
	}
	if count != 1 {
		return CurrentStateV1{}, fmt.Errorf("%w: frame exact scope is ambiguous", contract.ErrConflict)
	}
	var storedDigest, rowDigest string
	var payload []byte
	if err := source.QueryRowContext(ctx, `
SELECT frame_digest,row_digest,payload FROM context_frame_store_history
WHERE owner_component_id=? AND owner_binding_digest=? AND frame_id=? AND frame_revision=?`,
		owner.ComponentID, string(owner.BindingDigest), exact.ID, exact.Revision).Scan(&storedDigest, &rowDigest, &payload); err != nil {
		return CurrentStateV1{}, mapSQLiteErrorV1(ctx, err, false)
	}
	if storedDigest != string(exact.Digest) {
		return CurrentStateV1{}, fmt.Errorf("%w: frame exact digest drift", contract.ErrConflict)
	}
	state, err := decodeStateRowV1(ctx, payload, rowDigest, owner)
	if err != nil {
		return CurrentStateV1{}, err
	}
	frameRef, _, _, _ := stateRefsV1(state)
	if frameRef != exact {
		return CurrentStateV1{}, fmt.Errorf("%w: frame exact payload drift", contract.ErrConflict)
	}
	return state, nil
}

func loadStateByMetadataRefV1(ctx context.Context, source queryRowerV1, owner contract.OwnerRef, kind string, exact contract.FactRef, scope contract.Digest) (CurrentStateV1, error) {
	var idColumn, revisionColumn, digestColumn string
	switch kind {
	case "frame":
		idColumn, revisionColumn, digestColumn = "frame_id", "frame_revision", "frame_digest"
	case "manifest":
		idColumn, revisionColumn, digestColumn = "manifest_id", "manifest_revision", "manifest_digest"
	case "generation":
		idColumn, revisionColumn, digestColumn = "generation_id", "generation_revision", "generation_digest"
	default:
		return CurrentStateV1{}, fmt.Errorf("%w: frame store metadata kind", contract.ErrInvalid)
	}
	query := fmt.Sprintf(`
SELECT %s,row_digest,payload FROM context_frame_store_history
WHERE owner_component_id=? AND owner_binding_digest=? AND execution_scope_digest=? AND %s=? AND %s=?`,
		digestColumn, idColumn, revisionColumn)
	var storedDigest, rowDigest string
	var payload []byte
	err := source.QueryRowContext(ctx, query, owner.ComponentID, string(owner.BindingDigest), string(scope), exact.ID, exact.Revision).Scan(&storedDigest, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return CurrentStateV1{}, fmt.Errorf("%w: frame store exact metadata", contract.ErrNotFound)
	}
	if err != nil {
		return CurrentStateV1{}, mapSQLiteErrorV1(ctx, err, false)
	}
	if storedDigest != string(exact.Digest) {
		return CurrentStateV1{}, fmt.Errorf("%w: frame store exact metadata digest", contract.ErrConflict)
	}
	state, err := decodeStateRowV1(ctx, payload, rowDigest, owner)
	if err != nil {
		return CurrentStateV1{}, err
	}
	frameRef, manifestRef, generationRef, refErr := stateRefsV1(state)
	if refErr != nil {
		return CurrentStateV1{}, refErr
	}
	var actual contract.FactRef
	switch kind {
	case "frame":
		actual = frameRef
	case "manifest":
		actual = manifestRef
	case "generation":
		actual = generationRef
	}
	if actual != exact || state.Frame.Execution.ScopeDigest != scope ||
		state.Manifest.Execution.ScopeDigest != scope ||
		state.Pointer.ExecutionScopeDigest != scope {
		return CurrentStateV1{}, fmt.Errorf("%w: frame store exact metadata payload", contract.ErrConflict)
	}
	return state, nil
}

func loadStateBySourceV1(ctx context.Context, source queryRowerV1, owner contract.OwnerRef, exact contract.ContextParentFrameApplicabilitySourceCoordinateV1) (CurrentStateV1, error) {
	var rowDigest string
	var payload []byte
	err := source.QueryRowContext(ctx, `
SELECT row_digest,payload FROM context_frame_store_history
WHERE owner_component_id=? AND owner_binding_digest=? AND source_kind=? AND source_id=? AND source_revision=? AND source_digest=?`,
		owner.ComponentID, string(owner.BindingDigest), exact.Kind, exact.ID, exact.Revision, string(exact.Digest)).Scan(&rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		var sameID int
		if countErr := source.QueryRowContext(ctx, `
SELECT COUNT(*) FROM context_frame_store_history
WHERE owner_component_id=? AND owner_binding_digest=? AND source_id=?`,
			owner.ComponentID, string(owner.BindingDigest), exact.ID).Scan(&sameID); countErr != nil {
			return CurrentStateV1{}, mapSQLiteErrorV1(ctx, countErr, false)
		}
		if sameID > 0 {
			return CurrentStateV1{}, fmt.Errorf("%w: frame source coordinate drift", contract.ErrConflict)
		}
		return CurrentStateV1{}, fmt.Errorf("%w: frame source binding", contract.ErrNotFound)
	}
	if err != nil {
		return CurrentStateV1{}, mapSQLiteErrorV1(ctx, err, false)
	}
	state, err := decodeStateRowV1(ctx, payload, rowDigest, owner)
	if err != nil || state.Binding.Source != exact {
		return CurrentStateV1{}, fmt.Errorf("%w: frame source binding payload", contract.ErrConflict)
	}
	return state, nil
}

func loadCurrentStateByRequestV1(ctx context.Context, source queryRowerV1, owner contract.OwnerRef, request contract.ContextGenerationCurrentPointerRequestV1) (CurrentStateV1, error) {
	var frameID, frameDigest string
	var frameRevision uint64
	err := source.QueryRowContext(ctx, `
SELECT frame_id,frame_revision,frame_digest FROM context_frame_store_current
WHERE owner_component_id=? AND owner_binding_digest=? AND execution_scope_digest=? AND run_id=?
  AND session_id=? AND session_revision=? AND session_digest=?`,
		owner.ComponentID, string(owner.BindingDigest), string(request.ExecutionScopeDigest), request.RunID,
		request.SessionRef.ID, request.SessionRef.Revision, string(request.SessionRef.Digest)).Scan(&frameID, &frameRevision, &frameDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return CurrentStateV1{}, fmt.Errorf("%w: generation current pointer", contract.ErrNotFound)
	}
	if err != nil {
		return CurrentStateV1{}, mapSQLiteErrorV1(ctx, err, false)
	}
	state, err := loadStateByMetadataRefV1(ctx, source, owner, "frame", contract.FactRef{ID: frameID, Revision: frameRevision, Digest: contract.Digest(frameDigest)}, request.ExecutionScopeDigest)
	if err != nil {
		return CurrentStateV1{}, err
	}
	frameRef, _, _, refErr := stateRefsV1(state)
	if refErr != nil || frameRef.ID != frameID || frameRef.Revision != frameRevision || string(frameRef.Digest) != frameDigest ||
		state.Pointer.ExecutionScopeDigest != request.ExecutionScopeDigest ||
		state.Pointer.RunID != request.RunID || state.Pointer.SessionRef != request.SessionRef ||
		state.Pointer.Turn != request.Turn {
		return CurrentStateV1{}, fmt.Errorf("%w: generation current pointer turn drift", contract.ErrConflict)
	}
	return state, nil
}

func loadCurrentPointerForStateV1(ctx context.Context, source queryRowerV1, owner contract.OwnerRef, state CurrentStateV1) (contract.ContextGenerationCurrentPointerV1, error) {
	var turn, pointerRevision, highest uint64
	var pointerID, pointerDigest, frameID, frameDigest string
	var frameRevision uint64
	err := source.QueryRowContext(ctx, `
SELECT turn,pointer_id,pointer_revision,pointer_digest,frame_id,frame_revision,frame_digest,highest_pointer_revision
FROM context_frame_store_current
WHERE owner_component_id=? AND owner_binding_digest=? AND execution_scope_digest=? AND run_id=?
  AND session_id=? AND session_revision=? AND session_digest=?`,
		owner.ComponentID, string(owner.BindingDigest), string(state.Pointer.ExecutionScopeDigest), state.Pointer.RunID,
		state.Pointer.SessionRef.ID, state.Pointer.SessionRef.Revision, string(state.Pointer.SessionRef.Digest),
	).Scan(&turn, &pointerID, &pointerRevision, &pointerDigest, &frameID, &frameRevision, &frameDigest, &highest)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.ContextGenerationCurrentPointerV1{}, fmt.Errorf("%w: frame store current", contract.ErrNotFound)
	}
	if err != nil {
		return contract.ContextGenerationCurrentPointerV1{}, mapSQLiteErrorV1(ctx, err, false)
	}
	frameRef, _, _, _ := stateRefsV1(state)
	if turn != uint64(state.Pointer.Turn) || pointerID != state.Pointer.ID ||
		pointerRevision != state.Pointer.Revision || pointerDigest != string(state.Pointer.Digest) ||
		frameID != frameRef.ID || frameRevision != frameRef.Revision || frameDigest != string(frameRef.Digest) ||
		highest != pointerRevision {
		return contract.ContextGenerationCurrentPointerV1{}, fmt.Errorf("%w: frame store current row drift", contract.ErrConflict)
	}
	return state.Pointer, nil
}

func loadMaximumPointerRevisionV1(ctx context.Context, source queryRowerV1, owner contract.OwnerRef, pointer contract.ContextGenerationCurrentPointerV1) (uint64, error) {
	var maximum sql.NullInt64
	if err := source.QueryRowContext(ctx, `
SELECT MAX(pointer_revision) FROM context_frame_store_history
WHERE owner_component_id=? AND owner_binding_digest=? AND execution_scope_digest=? AND run_id=?
  AND session_id=? AND session_revision=? AND session_digest=?`,
		owner.ComponentID, string(owner.BindingDigest), string(pointer.ExecutionScopeDigest), pointer.RunID,
		pointer.SessionRef.ID, pointer.SessionRef.Revision, string(pointer.SessionRef.Digest),
	).Scan(&maximum); err != nil {
		return 0, mapSQLiteErrorV1(ctx, err, false)
	}
	if !maximum.Valid || maximum.Int64 <= 0 {
		return 0, fmt.Errorf("%w: frame store pointer history", contract.ErrNotFound)
	}
	return uint64(maximum.Int64), nil
}

func loadLedgerV1(ctx context.Context, source queryRowerV1, owner contract.OwnerRef, operationID string) (ledgerRecordV1, error) {
	var storedOperation, ownerComponent, ownerBinding, scope, runID string
	var sessionID, sessionDigest string
	var sessionRevision uint64
	var expectedID, expectedDigest sql.NullString
	var expectedRevision sql.NullInt64
	var nextID, nextDigest, frameID, frameDigest, stateRowDigest, rowDigest string
	var nextRevision, frameRevision uint64
	var createdUnixNano int64
	var payload []byte
	err := source.QueryRowContext(ctx, `
SELECT operation_id,owner_component_id,owner_binding_digest,execution_scope_digest,run_id,
       session_id,session_revision,session_digest,
       expected_pointer_id,expected_pointer_revision,expected_pointer_digest,
       next_pointer_id,next_pointer_revision,next_pointer_digest,
       frame_id,frame_revision,frame_digest,state_row_digest,created_unix_nano,row_digest,payload
FROM context_frame_store_ledger WHERE operation_id=?`, operationID).Scan(
		&storedOperation, &ownerComponent, &ownerBinding, &scope, &runID,
		&sessionID, &sessionRevision, &sessionDigest,
		&expectedID, &expectedRevision, &expectedDigest,
		&nextID, &nextRevision, &nextDigest,
		&frameID, &frameRevision, &frameDigest, &stateRowDigest, &createdUnixNano, &rowDigest, &payload,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ledgerRecordV1{}, fmt.Errorf("%w: frame store ledger operation", contract.ErrNotFound)
	}
	if err != nil {
		return ledgerRecordV1{}, mapSQLiteErrorV1(ctx, err, false)
	}
	storedOwner := contract.OwnerRef{ComponentID: ownerComponent, BindingDigest: contract.Digest(ownerBinding)}
	storedScope := contract.Digest(scope)
	sessionRef := contract.FactRef{ID: sessionID, Revision: sessionRevision, Digest: contract.Digest(sessionDigest)}
	frameRef := contract.FactRef{ID: frameID, Revision: frameRevision, Digest: contract.Digest(frameDigest)}
	if storedOperation != operationID || storedOwner != owner || owner.Validate() != nil ||
		storedScope.Validate() != nil || !validIDV1(runID) || sessionRef.Validate() != nil ||
		frameRef.Validate() != nil || contract.Digest(stateRowDigest).Validate() != nil ||
		createdUnixNano <= 0 {
		return ledgerRecordV1{}, fmt.Errorf("%w: frame store ledger coordinate drift", contract.ErrConflict)
	}
	nextState, nextStoredRowDigest, err := loadStateByFullCoordinateV1(
		ctx, source, owner, storedScope, runID, sessionRef, frameRef,
		contract.FactRef{ID: nextID, Revision: nextRevision, Digest: contract.Digest(nextDigest)},
	)
	if err != nil {
		return ledgerRecordV1{}, fmt.Errorf("%w: frame store ledger next state", contract.ErrConflict)
	}
	if string(nextStoredRowDigest) != stateRowDigest {
		return ledgerRecordV1{}, fmt.Errorf("%w: frame store ledger state row digest", contract.ErrConflict)
	}
	// row_digest seals the semantic ledger record. Canonical byte equality
	// separately prevents alternate JSON encodings of the receipt payload.
	receipt, err := contract.DecodeStrict[CommitReceiptV1](payload)
	if err != nil || receipt.Validate() != nil {
		return ledgerRecordV1{}, fmt.Errorf("%w: frame store ledger receipt payload", contract.ErrConflict)
	}
	canonicalReceiptPayload, marshalErr := json.Marshal(receipt)
	if marshalErr != nil || !reflect.DeepEqual(canonicalReceiptPayload, payload) {
		return ledgerRecordV1{}, fmt.Errorf("%w: frame store noncanonical ledger receipt payload", contract.ErrConflict)
	}
	var expected *contract.ContextGenerationCurrentPointerV1
	if expectedID.Valid || expectedRevision.Valid || expectedDigest.Valid {
		if !expectedID.Valid || !expectedRevision.Valid || !expectedDigest.Valid || expectedRevision.Int64 <= 0 {
			return ledgerRecordV1{}, fmt.Errorf("%w: frame store ledger expected pointer", contract.ErrConflict)
		}
		state, _, loadErr := loadStateByPointerCoordinateV1(
			ctx, source, owner, storedScope, runID, sessionRef,
			contract.FactRef{ID: expectedID.String, Revision: uint64(expectedRevision.Int64), Digest: contract.Digest(expectedDigest.String)},
		)
		if loadErr != nil {
			return ledgerRecordV1{}, fmt.Errorf("%w: frame store ledger predecessor", contract.ErrConflict)
		}
		pointer := state.Pointer
		expected = &pointer
	}
	record := ledgerRecordV1{
		OperationID:     storedOperation,
		Owner:           storedOwner,
		Scope:           storedScope,
		RunID:           runID,
		SessionRef:      sessionRef,
		Expected:        expected,
		Next:            nextState.Pointer,
		FrameRef:        frameRef,
		StateRowDigest:  contract.Digest(stateRowDigest),
		CreatedUnixNano: createdUnixNano,
		Receipt:         receipt,
	}
	if err := record.Validate(); err != nil ||
		record.Next.ID != nextID || record.Next.Revision != nextRevision || string(record.Next.Digest) != nextDigest {
		return ledgerRecordV1{}, fmt.Errorf("%w: frame store ledger denormalized columns", contract.ErrConflict)
	}
	digest, err := record.digestValue()
	if err != nil || string(digest) != rowDigest {
		return ledgerRecordV1{}, fmt.Errorf("%w: frame store ledger row digest", contract.ErrConflict)
	}
	return record, nil
}

func loadStateByFullCoordinateV1(
	ctx context.Context,
	source queryRowerV1,
	owner contract.OwnerRef,
	scope contract.Digest,
	runID string,
	sessionRef contract.FactRef,
	frameRef contract.FactRef,
	pointerRef contract.FactRef,
) (CurrentStateV1, contract.Digest, error) {
	var storedRowDigest string
	var payload []byte
	err := source.QueryRowContext(ctx, `
SELECT row_digest,payload FROM context_frame_store_history
WHERE owner_component_id=? AND owner_binding_digest=? AND execution_scope_digest=? AND run_id=?
  AND session_id=? AND session_revision=? AND session_digest=?
  AND frame_id=? AND frame_revision=? AND frame_digest=?
  AND pointer_id=? AND pointer_revision=? AND pointer_digest=?`,
		owner.ComponentID, string(owner.BindingDigest), string(scope), runID,
		sessionRef.ID, sessionRef.Revision, string(sessionRef.Digest),
		frameRef.ID, frameRef.Revision, string(frameRef.Digest),
		pointerRef.ID, pointerRef.Revision, string(pointerRef.Digest),
	).Scan(&storedRowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return CurrentStateV1{}, "", fmt.Errorf("%w: frame store exact full coordinate", contract.ErrNotFound)
	}
	if err != nil {
		return CurrentStateV1{}, "", mapSQLiteErrorV1(ctx, err, false)
	}
	state, err := decodeStateRowV1(ctx, payload, storedRowDigest, owner)
	if err != nil {
		return CurrentStateV1{}, "", err
	}
	actualFrame, _, _, refErr := stateRefsV1(state)
	if refErr != nil || actualFrame != frameRef ||
		state.Pointer.ID != pointerRef.ID || state.Pointer.Revision != pointerRef.Revision || state.Pointer.Digest != pointerRef.Digest ||
		state.Pointer.ExecutionScopeDigest != scope || state.Pointer.RunID != runID || state.Pointer.SessionRef != sessionRef {
		return CurrentStateV1{}, "", fmt.Errorf("%w: frame store full coordinate payload", contract.ErrConflict)
	}
	return state, contract.Digest(storedRowDigest), nil
}

func loadStateByPointerCoordinateV1(
	ctx context.Context,
	source queryRowerV1,
	owner contract.OwnerRef,
	scope contract.Digest,
	runID string,
	sessionRef contract.FactRef,
	pointerRef contract.FactRef,
) (CurrentStateV1, contract.Digest, error) {
	var storedRowDigest string
	var payload []byte
	err := source.QueryRowContext(ctx, `
SELECT row_digest,payload FROM context_frame_store_history
WHERE owner_component_id=? AND owner_binding_digest=? AND execution_scope_digest=? AND run_id=?
  AND session_id=? AND session_revision=? AND session_digest=?
  AND pointer_id=? AND pointer_revision=? AND pointer_digest=?`,
		owner.ComponentID, string(owner.BindingDigest), string(scope), runID,
		sessionRef.ID, sessionRef.Revision, string(sessionRef.Digest),
		pointerRef.ID, pointerRef.Revision, string(pointerRef.Digest),
	).Scan(&storedRowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return CurrentStateV1{}, "", fmt.Errorf("%w: frame store exact pointer coordinate", contract.ErrNotFound)
	}
	if err != nil {
		return CurrentStateV1{}, "", mapSQLiteErrorV1(ctx, err, false)
	}
	state, err := decodeStateRowV1(ctx, payload, storedRowDigest, owner)
	if err != nil {
		return CurrentStateV1{}, "", err
	}
	if state.Pointer.ID != pointerRef.ID || state.Pointer.Revision != pointerRef.Revision || state.Pointer.Digest != pointerRef.Digest ||
		state.Pointer.ExecutionScopeDigest != scope || state.Pointer.RunID != runID || state.Pointer.SessionRef != sessionRef {
		return CurrentStateV1{}, "", fmt.Errorf("%w: frame store pointer coordinate payload", contract.ErrConflict)
	}
	return state, contract.Digest(storedRowDigest), nil
}

func encodeStateRowV1(state CurrentStateV1) ([]byte, contract.Digest, error) {
	payload, err := json.Marshal(state)
	if err != nil || len(payload) == 0 {
		return nil, "", fmt.Errorf("%w: frame store state row", contract.ErrInvalid)
	}
	digest, err := contract.DigestJSON(struct {
		Domain string
		State  CurrentStateV1
	}{"praxis.context/frame-store-state-row-v1", state})
	return payload, digest, err
}

func decodeStateRowV1(ctx context.Context, payload []byte, storedDigest string, owner contract.OwnerRef) (CurrentStateV1, error) {
	if err := checkContextSQLiteV1(ctx); err != nil {
		return CurrentStateV1{}, err
	}
	state, err := contract.DecodeStrict[CurrentStateV1](payload)
	if err != nil {
		return CurrentStateV1{}, fmt.Errorf("%w: frame store strict state row", contract.ErrConflict)
	}
	if err := checkContextSQLiteV1(ctx); err != nil {
		return CurrentStateV1{}, err
	}
	state, err = normalizeStateV1(state)
	if err != nil {
		return CurrentStateV1{}, err
	}
	if err := checkContextSQLiteV1(ctx); err != nil {
		return CurrentStateV1{}, err
	}
	encoded, digest, err := encodeStateRowV1(state)
	if err != nil || string(digest) != storedDigest || !reflect.DeepEqual(encoded, payload) {
		return CurrentStateV1{}, fmt.Errorf("%w: frame store state row digest", contract.ErrConflict)
	}
	if err := validateStateAtV1(ctx, state, owner, stateCreatedFloorV1(state)); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return CurrentStateV1{}, contextErr
		}
		return CurrentStateV1{}, fmt.Errorf("%w: frame store state row closure", contract.ErrConflict)
	}
	if err := checkContextSQLiteV1(ctx); err != nil {
		return CurrentStateV1{}, err
	}
	return state, nil
}

func encodeReceiptRowV1(receipt CommitReceiptV1) ([]byte, contract.Digest, error) {
	payload, err := json.Marshal(receipt)
	if err != nil || len(payload) == 0 {
		return nil, "", fmt.Errorf("%w: frame store receipt row", contract.ErrInvalid)
	}
	digest, err := contract.DigestJSON(struct {
		Domain  string
		Receipt CommitReceiptV1
	}{"praxis.context/frame-store-receipt-row-v1", receipt})
	return payload, digest, err
}

func decodeReceiptRowV1(payload []byte, storedDigest string) (CommitReceiptV1, error) {
	receipt, err := contract.DecodeStrict[CommitReceiptV1](payload)
	if err != nil || receipt.Validate() != nil {
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store strict receipt row", contract.ErrConflict)
	}
	encoded, digest, err := encodeReceiptRowV1(receipt)
	if err != nil || string(digest) != storedDigest || !reflect.DeepEqual(encoded, payload) {
		return CommitReceiptV1{}, fmt.Errorf("%w: frame store receipt row digest", contract.ErrConflict)
	}
	return receipt, nil
}

func sameLineageV1(left, right contract.ContextGenerationCurrentPointerV1) bool {
	return left.ExecutionScopeDigest == right.ExecutionScopeDigest &&
		left.RunID == right.RunID && left.SessionRef == right.SessionRef
}

func sameOptionalPointerV1(left, right *contract.ContextGenerationCurrentPointerV1) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func checkContextSQLiteV1(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil frame store context", contract.ErrInvalid)
	}
	return ctx.Err()
}

func mapSQLiteErrorV1(ctx context.Context, err error, mutation bool) error {
	if mutation && (ctx == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return fmt.Errorf("%w: frame store mutation outcome", contract.ErrUnknown)
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "locked") || strings.Contains(message, "busy") {
		return fmt.Errorf("%w: frame store sqlite busy", contract.ErrUnavailable)
	}
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") ||
		strings.Contains(message, "append-only") {
		return fmt.Errorf("%w: frame store sqlite constraint", contract.ErrConflict)
	}
	if mutation {
		return fmt.Errorf("%w: frame store mutation outcome", contract.ErrUnknown)
	}
	return fmt.Errorf("%w: frame store sqlite read", contract.ErrUnavailable)
}

func (s *SQLiteV1) consumeFailNextStageV1() bool {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	value := s.failNextStage
	s.failNextStage = false
	return value
}

func (s *SQLiteV1) consumeLoseNextReplyV1() bool {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	value := s.loseNextReply
	s.loseNextReply = false
	return value
}

func (s *SQLiteV1) failNextStageForTestV1() {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	s.failNextStage = true
}

func (s *SQLiteV1) loseNextReplyForTestV1() {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	s.loseNextReply = true
}

func (s *SQLiteV1) runBeforeExactCurrentS2ForTestV1() {
	s.faultMu.Lock()
	hook := s.beforeS2
	s.beforeS2 = nil
	s.faultMu.Unlock()
	if hook != nil {
		hook()
	}
}

func (s *SQLiteV1) runBeforeGenerationCurrentS2ForTestV1() {
	s.faultMu.Lock()
	hook := s.beforePointerS2
	s.beforePointerS2 = nil
	s.faultMu.Unlock()
	if hook != nil {
		hook()
	}
}

// FailNextStageForTestingV1 injects a rollback before the current pointer is
// published. It exists only for black-box fault tests.
func (s *SQLiteV1) FailNextStageForTestingV1() {
	if s != nil {
		s.failNextStageForTestV1()
	}
}

// LoseNextReplyForTestingV1 commits durably and then returns ErrUnknown. It
// exists only to prove inspect-only recovery in black-box tests.
func (s *SQLiteV1) LoseNextReplyForTestingV1() {
	if s != nil {
		s.loseNextReplyForTestV1()
	}
}

// BeforeExactCurrentS2ForTestingV1 installs a one-shot barrier between S1 and
// S2. It exists only to prove current-pointer drift is detected.
func (s *SQLiteV1) BeforeExactCurrentS2ForTestingV1(hook func()) {
	if s == nil {
		return
	}
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	s.beforeS2 = hook
}

// BeforeGenerationCurrentS2ForTestingV1 installs a one-shot barrier between
// two complete current-pointer snapshots. It exists only for cross-connection
// drift tests.
func (s *SQLiteV1) BeforeGenerationCurrentS2ForTestingV1(hook func()) {
	if s == nil {
		return
	}
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	s.beforePointerS2 = hook
}
