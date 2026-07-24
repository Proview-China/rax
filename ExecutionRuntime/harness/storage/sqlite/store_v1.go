// Package sqlite provides the Harness-owned single-node durable Session V4
// and Event Candidate State Plane. It does not construct a Harness, settle a
// Runtime outcome, or claim HA/SLA/production-root conformance.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	_ "modernc.org/sqlite"
)

type ConfigV1 struct {
	Path         string
	StoreID      string
	BusyTimeout  time.Duration
	MaxOpenConns int
	ProofTTL     time.Duration
	Clock        func() time.Time
}

type StoreV1 struct {
	db       *sql.DB
	storeID  string
	clock    func() time.Time
	proofTTL time.Duration
	mu       *sync.Mutex

	faultMu       sync.Mutex
	loseNextReply bool
}

var (
	_                 harnessports.SessionFactPortV4         = (*StoreV1)(nil)
	_                 harnessports.EventCandidateJournalPort = (*StoreV1)(nil)
	_                 DurableSessionEventCurrentReaderV1     = (*StoreV1)(nil)
	statePlaneLocksV1 sync.Map
)

func OpenV1(ctx context.Context, config ConfigV1) (*StoreV1, error) {
	if ctx == nil || ctx.Err() != nil || !validIDV1(config.StoreID) || strings.TrimSpace(config.Path) == "" {
		return nil, invalidV1("Harness SQLite path, StoreID and live context are required")
	}
	if config.BusyTimeout <= 0 {
		config.BusyTimeout = 5 * time.Second
	}
	if config.BusyTimeout > time.Minute {
		return nil, invalidV1("Harness SQLite busy timeout exceeds one minute")
	}
	if config.MaxOpenConns <= 0 {
		config.MaxOpenConns = 8
	}
	if config.MaxOpenConns > 32 {
		return nil, invalidV1("Harness SQLite connection count exceeds 32")
	}
	if config.ProofTTL == 0 {
		config.ProofTTL = 5 * time.Minute
	}
	if config.ProofTTL < time.Second || config.ProofTTL > 24*time.Hour || config.ProofTTL%time.Second != 0 {
		return nil, invalidV1("Harness SQLite proof TTL is outside its exact bounded window")
	}
	if config.Clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "Harness SQLite clock is required")
	}
	abs, err := filepath.Abs(config.Path)
	if err != nil {
		return nil, invalidV1("Harness SQLite path is invalid")
	}
	lock, _ := statePlaneLocksV1.LoadOrStore(abs, &sync.Mutex{})
	dsn := (&url.URL{Scheme: "file", Path: abs}).String()
	dsn += fmt.Sprintf("?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(FULL)&_pragma=busy_timeout(%d)&_txlock=immediate", config.BusyTimeout.Milliseconds())
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, mapDBErrorV1(ctx, err, false)
	}
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxOpenConns)
	store := &StoreV1{db: db, storeID: config.StoreID, clock: config.Clock, proofTTL: config.ProofTTL, mu: lock.(*sync.Mutex)}
	if err := store.migrateV1(ctx, abs); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.verifyV1(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *StoreV1) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *StoreV1) IntegrityCheckV1(ctx context.Context) error {
	if err := s.readReadyV1(ctx); err != nil {
		return err
	}
	var value string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&value); err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	if value != "ok" {
		return corruptV1("Harness SQLite integrity check failed")
	}
	return nil
}

func (s *StoreV1) migrateV1(ctx context.Context, abs string) error {
	if err := s.readReadyV1(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, schemaV1); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	now, err := s.nowV1()
	if err != nil {
		return err
	}
	schemaDigest := core.DigestBytes([]byte(schemaV1))
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO harness_state_schema_v1(version,digest,applied_unix_nano) VALUES(?,?,?)`, schemaVersionV1, string(schemaDigest), now.UnixNano())
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		var stored string
		if err = tx.QueryRowContext(ctx, `SELECT digest FROM harness_state_schema_v1 WHERE version=?`, schemaVersionV1).Scan(&stored); err != nil {
			return mapDBErrorV1(ctx, err, true)
		}
		if stored != string(schemaDigest) {
			return corruptV1("Harness SQLite schema digest drifted")
		}
	}
	var schemaCount, schemaMin, schemaMax int64
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*),MIN(version),MAX(version) FROM harness_state_schema_v1`).Scan(&schemaCount, &schemaMin, &schemaMax); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	if schemaCount != 1 || schemaMin != schemaVersionV1 || schemaMax != schemaVersionV1 {
		return core.NewError(core.ErrorConflict, core.ReasonUnknownSchema, "Harness SQLite schema version set drifted")
	}
	identity, err := core.CanonicalJSONDigest(rowDomainV1, "1.0.0", "DatabaseIdentityV1", struct {
		StoreID string `json:"store_id"`
		Path    string `json:"path"`
	}{s.storeID, abs})
	if err != nil {
		return err
	}
	result, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO harness_state_identity_v1(singleton,store_id,database_identity_digest,clock_high_water_unix_nano) VALUES(1,?,?,?)`, s.storeID, string(identity), now.UnixNano())
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	affected, _ = result.RowsAffected()
	var storedID, storedIdentity string
	var highWater int64
	if err = tx.QueryRowContext(ctx, `SELECT store_id,database_identity_digest,clock_high_water_unix_nano FROM harness_state_identity_v1 WHERE singleton=1`).Scan(&storedID, &storedIdentity, &highWater); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	if storedID != s.storeID || storedIdentity != string(identity) {
		return conflictV1("Harness SQLite StoreID or database identity drifted")
	}
	if now.UnixNano() < highWater {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Harness SQLite clock regressed")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE harness_state_identity_v1 SET clock_high_water_unix_nano=? WHERE singleton=1`, now.UnixNano()); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	var currentRevision int64
	var currentDigest string
	err = tx.QueryRowContext(ctx, `SELECT revision,digest FROM harness_session_event_proof_current_v1 WHERE store_id=?`, s.storeID).Scan(&currentRevision, &currentDigest)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err = s.advanceProofTxV1(ctx, tx, now, identity, schemaDigest); err != nil {
			return err
		}
	} else if err != nil {
		return mapDBErrorV1(ctx, err, true)
	} else {
		current, proofErr := s.readProofTxV1(ctx, tx, DurableSessionEventCurrentRefV1{StoreID: s.storeID, Revision: core.Revision(currentRevision), Digest: core.Digest(currentDigest)}, true)
		if proofErr != nil {
			return proofErr
		}
		if now.UnixNano() >= current.ExpiresUnixNano {
			if _, err = s.advanceProofTxV1(ctx, tx, now, identity, schemaDigest); err != nil {
				return err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Harness SQLite migration outcome is unknown")
	}
	return nil
}

func (s *StoreV1) verifyV1(ctx context.Context) error {
	var journal string
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	if !strings.EqualFold(journal, "wal") {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Harness SQLite WAL mode is inactive")
	}
	var foreignKeys int
	if err := s.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	if foreignKeys != 1 {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Harness SQLite foreign keys are inactive")
	}
	return nil
}

func (s *StoreV1) CreateSessionV4(ctx context.Context, session contract.GovernedSessionV4) (contract.GovernedSessionV4, error) {
	if err := s.writeReadyV1(ctx); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	if err := session.Validate(); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	if session.Revision != 1 || session.Phase != contract.SessionCreatingV2 || session.ApplicationBinding != nil {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "new durable V4 Session must be creating revision one")
	}
	now, err := s.nowV1()
	if err != nil || session.UpdatedUnixNano > now.UnixNano() {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "durable V4 Session time is ahead of the store clock")
	}
	scope, err := runtimeports.ExecutionScopeDigestV2(session.Run.Scope)
	if err != nil {
		return contract.GovernedSessionV4{}, err
	}
	payload, rowDigest, err := encodeRowV1("GovernedSessionV4", session)
	if err != nil {
		return contract.GovernedSessionV4{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return contract.GovernedSessionV4{}, mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	if err = s.checkClockTxV1(ctx, tx, now); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	current, found, err := inspectSessionTxV1(ctx, tx, string(scope), string(session.Run.RunID), session.ID)
	if err != nil {
		return contract.GovernedSessionV4{}, err
	}
	if found {
		if reflect.DeepEqual(current, session) {
			return current.Clone(), nil
		}
		return contract.GovernedSessionV4{}, conflictV1("durable V4 Session identity already binds different content")
	}
	var activeRun, activeSession string
	err = tx.QueryRowContext(ctx, `SELECT run_id,session_id FROM harness_active_scope_v4 WHERE scope_digest=?`, string(scope)).Scan(&activeRun, &activeSession)
	if err == nil {
		return contract.GovernedSessionV4{}, conflictV1("Execution Scope already has an active durable V4 Session")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return contract.GovernedSessionV4{}, mapDBErrorV1(ctx, err, true)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO harness_session_history_v4(scope_digest,run_id,session_id,revision,digest,updated_unix_nano,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?,?)`, string(scope), session.Run.RunID, session.ID, session.Revision, session.Digest, session.UpdatedUnixNano, rowDigest, payload); err != nil {
		return contract.GovernedSessionV4{}, mapDBErrorV1(ctx, err, true)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO harness_session_current_v4(scope_digest,run_id,session_id,highest_revision,digest) VALUES(?,?,?,?,?)`, string(scope), session.Run.RunID, session.ID, session.Revision, session.Digest); err != nil {
		return contract.GovernedSessionV4{}, mapDBErrorV1(ctx, err, true)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO harness_active_scope_v4(scope_digest,run_id,session_id) VALUES(?,?,?)`, string(scope), session.Run.RunID, session.ID); err != nil {
		return contract.GovernedSessionV4{}, mapDBErrorV1(ctx, err, true)
	}
	if err = s.finishMutationTxV1(ctx, tx, now); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	return session.Clone(), nil
}

func (s *StoreV1) InspectSessionV4(ctx context.Context, run contract.RunRef, id string) (contract.GovernedSessionV4, error) {
	if err := s.readReadyV1(ctx); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	if run.Validate() != nil || !validIDV1(id) {
		return contract.GovernedSessionV4{}, invalidV1("durable V4 Session Inspect coordinates are invalid")
	}
	scope, err := runtimeports.ExecutionScopeDigestV2(run.Scope)
	if err != nil {
		return contract.GovernedSessionV4{}, err
	}
	current, found, err := inspectSessionDBV1(ctx, s.db, string(scope), string(run.RunID), id)
	if err != nil {
		return contract.GovernedSessionV4{}, err
	}
	if !found {
		return contract.GovernedSessionV4{}, notFoundV1("durable V4 Session is absent")
	}
	return current.Clone(), nil
}

func (s *StoreV1) CompareAndSwapSessionV4(ctx context.Context, request contract.SessionCASRequestV4) (contract.GovernedSessionV4, error) {
	if err := s.writeReadyV1(ctx); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	if err := request.Validate(); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	now, err := s.nowV1()
	if err != nil || request.Next.UpdatedUnixNano > now.UnixNano() {
		return contract.GovernedSessionV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "durable V4 successor time is ahead of the store clock")
	}
	scope, err := runtimeports.ExecutionScopeDigestV2(request.Run.Scope)
	if err != nil {
		return contract.GovernedSessionV4{}, err
	}
	payload, rowDigest, err := encodeRowV1("GovernedSessionV4", request.Next)
	if err != nil {
		return contract.GovernedSessionV4{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return contract.GovernedSessionV4{}, mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	if err = s.checkClockTxV1(ctx, tx, now); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	current, found, err := inspectSessionTxV1(ctx, tx, string(scope), string(request.Run.RunID), request.SessionID)
	if err != nil {
		return contract.GovernedSessionV4{}, err
	}
	if !found {
		return contract.GovernedSessionV4{}, notFoundV1("durable V4 Session is absent")
	}
	if reflect.DeepEqual(current, request.Next) {
		return current.Clone(), nil
	}
	if current.Revision != request.ExpectedRevision || current.Digest != request.ExpectedDigest {
		return contract.GovernedSessionV4{}, conflictV1("durable V4 Session predecessor changed")
	}
	if err = contract.ValidateSessionTransitionV4(current, request.Next); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO harness_session_history_v4(scope_digest,run_id,session_id,revision,digest,updated_unix_nano,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?,?)`, string(scope), request.Run.RunID, request.SessionID, request.Next.Revision, request.Next.Digest, request.Next.UpdatedUnixNano, rowDigest, payload); err != nil {
		return contract.GovernedSessionV4{}, mapDBErrorV1(ctx, err, true)
	}
	result, err := tx.ExecContext(ctx, `UPDATE harness_session_current_v4 SET highest_revision=?,digest=? WHERE scope_digest=? AND run_id=? AND session_id=? AND highest_revision=? AND digest=?`, request.Next.Revision, request.Next.Digest, string(scope), request.Run.RunID, request.SessionID, request.ExpectedRevision, request.ExpectedDigest)
	if err != nil {
		return contract.GovernedSessionV4{}, mapDBErrorV1(ctx, err, true)
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return contract.GovernedSessionV4{}, conflictV1("durable V4 Session CAS lost")
	}
	if request.Next.Phase == contract.SessionTerminalV2 {
		if _, err = tx.ExecContext(ctx, `DELETE FROM harness_active_scope_v4 WHERE scope_digest=? AND run_id=? AND session_id=?`, string(scope), request.Run.RunID, request.SessionID); err != nil {
			return contract.GovernedSessionV4{}, mapDBErrorV1(ctx, err, true)
		}
	}
	if err = s.finishMutationTxV1(ctx, tx, now); err != nil {
		return contract.GovernedSessionV4{}, err
	}
	return request.Next.Clone(), nil
}

func (s *StoreV1) AppendCandidate(ctx context.Context, event contract.Event) error {
	if err := s.writeReadyV1(ctx); err != nil {
		return err
	}
	if err := event.Validate(); err != nil {
		return err
	}
	if uint64(event.SourceEpoch) > math.MaxInt64 || event.SourceSequence > math.MaxInt64 {
		return invalidV1("event source coordinate exceeds SQLite integer range")
	}
	now, err := s.nowV1()
	if err != nil || event.ObservedAt.UTC().UnixNano() > now.UnixNano() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "event observation is ahead of the store clock")
	}
	payload, rowDigest, err := encodeRowV1("EventCandidateV1", event)
	if err != nil {
		return err
	}
	eventDigest, err := core.CanonicalJSONDigest(rowDomainV1, "1.0.0", "EventCandidateIdentityV1", event)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	if err = s.checkClockTxV1(ctx, tx, now); err != nil {
		return err
	}
	existing, found, err := inspectEventTxV1(ctx, tx, event.SourceComponentID, event.SourceEpoch, event.SourceSequence)
	if err != nil {
		return err
	}
	if found {
		if sameEventV1(existing, event) {
			return nil
		}
		return eventConflictV1("event source coordinate already binds different content")
	}
	var highest uint64
	var highestDigest string
	var lastObserved int64
	err = tx.QueryRowContext(ctx, `SELECT highest_sequence,highest_event_digest,last_observed_unix_nano FROM harness_event_source_head_v1 WHERE source_component_id=? AND source_epoch=?`, event.SourceComponentID, event.SourceEpoch).Scan(&highest, &highestDigest, &lastObserved)
	if errors.Is(err, sql.ErrNoRows) {
		if event.SourceSequence != 1 {
			return eventConflictV1("new event source epoch must begin at sequence one")
		}
	} else if err != nil {
		return mapDBErrorV1(ctx, err, true)
	} else {
		previous, previousFound, previousErr := inspectEventTxV1(ctx, tx, event.SourceComponentID, event.SourceEpoch, highest)
		previousDigest, digestErr := core.CanonicalJSONDigest(rowDomainV1, "1.0.0", "EventCandidateIdentityV1", previous)
		if previousErr != nil || !previousFound || digestErr != nil || string(previousDigest) != highestDigest {
			return corruptV1("event source head drifted from its exact highest event")
		}
		if highest == math.MaxInt64 || event.SourceSequence != highest+1 || event.ObservedAt.UTC().UnixNano() < lastObserved {
			return eventConflictV1("event source sequence or observation clock is not monotonic")
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO harness_event_candidate_v1(source_component_id,source_epoch,source_sequence,run_id,observed_unix_nano,event_digest,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?,?)`, event.SourceComponentID, event.SourceEpoch, event.SourceSequence, event.RunID, event.ObservedAt.UTC().UnixNano(), eventDigest, rowDigest, payload); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	if highest == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO harness_event_source_head_v1(source_component_id,source_epoch,highest_sequence,highest_event_digest,last_observed_unix_nano) VALUES(?,?,?,?,?)`, event.SourceComponentID, event.SourceEpoch, event.SourceSequence, eventDigest, event.ObservedAt.UTC().UnixNano())
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE harness_event_source_head_v1 SET highest_sequence=?,highest_event_digest=?,last_observed_unix_nano=? WHERE source_component_id=? AND source_epoch=? AND highest_sequence=? AND highest_event_digest=?`, event.SourceSequence, eventDigest, event.ObservedAt.UTC().UnixNano(), event.SourceComponentID, event.SourceEpoch, highest, highestDigest)
		if err == nil {
			affected, rowsErr := result.RowsAffected()
			if rowsErr != nil || affected != 1 {
				return eventConflictV1("event source head CAS lost")
			}
		}
	}
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	return s.finishMutationTxV1(ctx, tx, now)
}

func (s *StoreV1) InspectCandidate(ctx context.Context, sourceID string, epoch core.Epoch, sequence uint64) (contract.Event, error) {
	if err := s.readReadyV1(ctx); err != nil {
		return contract.Event{}, err
	}
	if !validIDV1(sourceID) || epoch == 0 || uint64(epoch) > math.MaxInt64 || sequence == 0 || sequence > math.MaxInt64 {
		return contract.Event{}, invalidV1("event exact Inspect coordinate is invalid")
	}
	event, found, err := inspectEventDBV1(ctx, s.db, sourceID, epoch, sequence)
	if err != nil {
		return contract.Event{}, err
	}
	if !found {
		return contract.Event{}, notFoundV1("event candidate is absent")
	}
	return cloneEventV1(event), nil
}

func inspectSessionDBV1(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, scope, run, session string) (contract.GovernedSessionV4, bool, error) {
	return inspectSessionQueryV1(ctx, q, scope, run, session)
}

func inspectSessionTxV1(ctx context.Context, tx *sql.Tx, scope, run, session string) (contract.GovernedSessionV4, bool, error) {
	return inspectSessionQueryV1(ctx, tx, scope, run, session)
}

func inspectSessionQueryV1(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, scope, run, session string) (contract.GovernedSessionV4, bool, error) {
	var revision int64
	var pointerDigest, rowDigest string
	var payload []byte
	err := q.QueryRowContext(ctx, `SELECT c.highest_revision,c.digest,h.row_digest,h.canonical_json FROM harness_session_current_v4 c JOIN harness_session_history_v4 h ON h.scope_digest=c.scope_digest AND h.run_id=c.run_id AND h.session_id=c.session_id AND h.revision=c.highest_revision WHERE c.scope_digest=? AND c.run_id=? AND c.session_id=?`, scope, run, session).Scan(&revision, &pointerDigest, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.GovernedSessionV4{}, false, nil
	}
	if err != nil {
		return contract.GovernedSessionV4{}, false, mapDBErrorV1(ctx, err, false)
	}
	value, err := decodeRowV1[contract.GovernedSessionV4](payload, rowDigest, "GovernedSessionV4")
	if err != nil || value.Validate() != nil || value.Revision != core.Revision(revision) || value.Digest != core.Digest(pointerDigest) || string(value.Run.RunID) != run || value.ID != session {
		return contract.GovernedSessionV4{}, false, corruptV1("durable V4 Session current row drifted")
	}
	scopeDigest, scopeErr := runtimeports.ExecutionScopeDigestV2(value.Run.Scope)
	if scopeErr != nil || string(scopeDigest) != scope {
		return contract.GovernedSessionV4{}, false, corruptV1("durable V4 Session scope drifted")
	}
	return value.Clone(), true, nil
}

func inspectEventDBV1(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, source string, epoch core.Epoch, sequence uint64) (contract.Event, bool, error) {
	return inspectEventQueryV1(ctx, q, source, epoch, sequence)
}

func inspectEventTxV1(ctx context.Context, tx *sql.Tx, source string, epoch core.Epoch, sequence uint64) (contract.Event, bool, error) {
	return inspectEventQueryV1(ctx, tx, source, epoch, sequence)
}

func inspectEventQueryV1(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, source string, epoch core.Epoch, sequence uint64) (contract.Event, bool, error) {
	var storedEventDigest, rowDigest string
	var payload []byte
	err := q.QueryRowContext(ctx, `SELECT event_digest,row_digest,canonical_json FROM harness_event_candidate_v1 WHERE source_component_id=? AND source_epoch=? AND source_sequence=?`, source, epoch, sequence).Scan(&storedEventDigest, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.Event{}, false, nil
	}
	if err != nil {
		return contract.Event{}, false, mapDBErrorV1(ctx, err, false)
	}
	value, err := decodeRowV1[contract.Event](payload, rowDigest, "EventCandidateV1")
	if err != nil || value.Validate() != nil || value.SourceComponentID != source || value.SourceEpoch != epoch || value.SourceSequence != sequence {
		return contract.Event{}, false, corruptV1("event candidate row drifted")
	}
	digest, digestErr := core.CanonicalJSONDigest(rowDomainV1, "1.0.0", "EventCandidateIdentityV1", value)
	if digestErr != nil || string(digest) != storedEventDigest {
		return contract.Event{}, false, corruptV1("event candidate identity digest drifted")
	}
	return cloneEventV1(value), true, nil
}

func cloneEventV1(value contract.Event) contract.Event {
	value.Payload = contract.CloneOpaque(value.Payload)
	return value
}

func sameEventV1(left, right contract.Event) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest(rowDomainV1, "1.0.0", "EventCandidateIdentityV1", left)
	rightDigest, rightErr := core.CanonicalJSONDigest(rowDomainV1, "1.0.0", "EventCandidateIdentityV1", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func (s *StoreV1) finishMutationTxV1(ctx context.Context, tx *sql.Tx, now time.Time) error {
	if _, err := s.observeClockTxV1(ctx, tx, now); err != nil {
		return err
	}
	var identity, schemaDigest string
	if err := tx.QueryRowContext(ctx, `SELECT database_identity_digest FROM harness_state_identity_v1 WHERE singleton=1`).Scan(&identity); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	if err := tx.QueryRowContext(ctx, `SELECT digest FROM harness_state_schema_v1 WHERE version=?`, schemaVersionV1).Scan(&schemaDigest); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	if _, err := s.advanceProofTxV1(ctx, tx, now, core.Digest(identity), core.Digest(schemaDigest)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	s.faultMu.Lock()
	lose := s.loseNextReply
	s.loseNextReply = false
	s.faultMu.Unlock()
	if lose {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Harness SQLite mutation committed but its reply was lost")
	}
	return nil
}

func (s *StoreV1) observeClockTxV1(ctx context.Context, tx *sql.Tx, now time.Time) (int64, error) {
	var highWater int64
	if err := tx.QueryRowContext(ctx, `SELECT clock_high_water_unix_nano FROM harness_state_identity_v1 WHERE singleton=1`).Scan(&highWater); err != nil {
		return 0, mapDBErrorV1(ctx, err, true)
	}
	if now.IsZero() || now.UnixNano() <= 0 || now.UnixNano() < highWater {
		return 0, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Harness SQLite clock regressed")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE harness_state_identity_v1 SET clock_high_water_unix_nano=? WHERE singleton=1`, now.UnixNano()); err != nil {
		return 0, mapDBErrorV1(ctx, err, true)
	}
	return highWater, nil
}

func (s *StoreV1) checkClockTxV1(ctx context.Context, tx *sql.Tx, now time.Time) error {
	var highWater int64
	if err := tx.QueryRowContext(ctx, `SELECT clock_high_water_unix_nano FROM harness_state_identity_v1 WHERE singleton=1`).Scan(&highWater); err != nil {
		return mapDBErrorV1(ctx, err, false)
	}
	if now.IsZero() || now.UnixNano() <= 0 || now.UnixNano() < highWater {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Harness SQLite clock regressed")
	}
	return nil
}

func (s *StoreV1) nowV1() (time.Time, error) {
	if s == nil || s.clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Harness SQLite clock is unavailable")
	}
	now := s.clock().UTC()
	if now.IsZero() || now.UnixNano() <= 0 {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Harness SQLite clock is invalid")
	}
	return now, nil
}

func (s *StoreV1) readReadyV1(ctx context.Context) error {
	if s == nil || s.db == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Harness SQLite store is unavailable")
	}
	if ctx == nil || ctx.Err() != nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Harness SQLite read context is unavailable")
	}
	return nil
}

func (s *StoreV1) writeReadyV1(ctx context.Context) error {
	if s == nil || s.db == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Harness SQLite store is unavailable")
	}
	if ctx == nil || ctx.Err() != nil {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Harness SQLite mutation context is unavailable")
	}
	return nil
}

func validIDV1(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= contract.MaxReferenceBytes
}

func invalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}
func conflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, message)
}
func eventConflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, message)
}
func corruptV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, message)
}
func notFoundV1(message string) error {
	return core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, message)
}

func mapDBErrorV1(ctx context.Context, err error, mutation bool) error {
	if err == nil {
		return nil
	}
	if ctx == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if mutation {
			return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Harness SQLite mutation outcome is unknown")
		}
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Harness SQLite read is unavailable")
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "locked") || strings.Contains(message, "busy") {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Harness SQLite store is busy")
	}
	if strings.Contains(message, "constraint") || strings.Contains(message, "unique") {
		return conflictV1("Harness SQLite uniqueness conflict")
	}
	if mutation {
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Harness SQLite mutation outcome is unknown")
	}
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Harness SQLite read failed")
}
