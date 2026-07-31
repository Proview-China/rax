package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolaction "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/action"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type ActionFactStoreV2 struct {
	store   *StoreV1
	readers toolaction.CausalReadersV1
	fault   func(string) error
	clockMu sync.Mutex
	lastNow time.Time
}

func OpenActionFactStoreV2(ctx context.Context, config ConfigV1, readers toolaction.CausalReadersV1) (*ActionFactStoreV2, error) {
	store, err := OpenV1(ctx, config)
	if err != nil {
		return nil, err
	}
	facts, err := NewActionFactStoreV2(ctx, store, readers)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return facts, nil
}

func NewActionFactStoreV2(ctx context.Context, store *StoreV1, readers toolaction.CausalReadersV1) (*ActionFactStoreV2, error) {
	if store == nil || store.db == nil {
		return nil, invalidV1("Tool Action Fact SQLite store is required")
	}
	facts := &ActionFactStoreV2{store: store, readers: readers}
	if err := facts.migrateV2(ctx); err != nil {
		return nil, err
	}
	return facts, nil
}

func (s *ActionFactStoreV2) Close() error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Close()
}

func (s *ActionFactStoreV2) migrateV2(ctx context.Context) error {
	if err := s.readyV2(ctx, true); err != nil {
		return err
	}
	toolOwnerSchemaMigrationMuV2.Lock()
	defer toolOwnerSchemaMigrationMuV2.Unlock()
	tx, err := s.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	disposition, err := classifyOwnedSchemaV2(ctx, tx, "tool_action_fact_schema_v2", actionFactOwnedObjectsV2)
	if err != nil {
		return err
	}
	if disposition == ownedSchemaCreateV2 {
		if _, err = tx.ExecContext(ctx, actionFactSchemaV2); err != nil {
			return mapDBErrorV1(ctx, err, true)
		}
		now := s.store.clock()
		if now.IsZero() {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Action Fact SQLite clock is unavailable")
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO tool_action_fact_schema_v2(version,digest,applied_unix_nano) VALUES(?,?,?)`, actionFactSchemaVersionV2, string(actionFactSchemaDigestV2), now.UnixNano()); err != nil {
			return mapDBErrorV1(ctx, err, true)
		}
	}
	if err = verifyActionFactSchemaV2(ctx, tx); err != nil {
		return err
	}
	if err = verifyOwnedSchemaLedgerV2(ctx, tx, "tool_action_fact_schema_v2", actionFactSchemaVersionV2, actionFactSchemaDigestV2); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return indeterminateV1("Tool Action Fact SQLite schema commit outcome is unknown")
	}
	return nil
}

func (s *ActionFactStoreV2) readyV2(ctx context.Context, write bool) error {
	if s == nil || s.store == nil || s.store.db == nil {
		return unavailableV1("Tool Action Fact SQLite store is unavailable")
	}
	if write {
		return s.store.writeReadyV1(ctx)
	}
	return s.store.readReadyV1(ctx)
}

type actionHeadV2 struct {
	ActionID        string                  `json:"action_id"`
	Revision        core.Revision           `json:"revision"`
	Stage           string                  `json:"stage"`
	Candidate       toolcontract.ObjectRef  `json:"candidate"`
	Reservation     *toolcontract.ObjectRef `json:"reservation,omitempty"`
	DomainResult    *toolcontract.ObjectRef `json:"domain_result,omitempty"`
	Apply           *toolcontract.ObjectRef `json:"apply,omitempty"`
	Result          *toolcontract.ObjectRef `json:"result,omitempty"`
	UpdatedUnixNano int64                   `json:"updated_unix_nano"`
	Digest          core.Digest             `json:"digest"`
}

func sealActionHeadV2(head actionHeadV2) (actionHeadV2, error) {
	head.Digest = ""
	digest, err := core.CanonicalJSONDigest("praxis.tool-mcp.action-head", "2.0.0", "ActionHeadV2", head)
	if err != nil {
		return actionHeadV2{}, err
	}
	head.Digest = digest
	return head, nil
}

func (s *ActionFactStoreV2) CreateCandidateFactV2(ctx context.Context, candidate toolcontract.ActionCandidateV2) (toolaction.RecordV2, error) {
	if err := s.readyV2(ctx, true); err != nil {
		return toolaction.RecordV2{}, err
	}
	if err := candidate.Validate(); err != nil {
		return toolaction.RecordV2{}, err
	}
	body, rowDigest, err := encodeActionRowV2("ActionCandidateV2", candidate)
	if err != nil {
		return toolaction.RecordV2{}, err
	}
	tx, err := s.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return toolaction.RecordV2{}, mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	existing, found, err := readCandidateTxV2(ctx, tx, candidate.ID)
	if err != nil {
		return toolaction.RecordV2{}, err
	}
	if found {
		if !reflect.DeepEqual(existing, candidate) {
			return toolaction.RecordV2{}, conflictV1("Tool Action Candidate create-once content drifted")
		}
		head, headFound, readErr := readActionHeadTxV2(ctx, tx, candidate.ID)
		if readErr != nil {
			return toolaction.RecordV2{}, readErr
		}
		if !headFound || head.Candidate != objectRefActionV2(candidate.ID, candidate.Revision, candidate.Digest) {
			return toolaction.RecordV2{}, conflictV1("Tool Action Candidate head drifted")
		}
		record, readErr := readRecordTxV2(ctx, tx, head)
		return record, readErr
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO tool_action_candidate_v2(fact_id,revision,digest,action_id,body_json,row_digest) VALUES(?,?,?,?,?,?)`,
		candidate.ID, int64(candidate.Revision), string(candidate.Digest), candidate.ID, body, string(rowDigest)); err != nil {
		return toolaction.RecordV2{}, mapDBErrorV1(ctx, err, true)
	}
	head, err := sealActionHeadV2(actionHeadV2{ActionID: candidate.ID, Revision: 1, Stage: "candidate", Candidate: objectRefActionV2(candidate.ID, candidate.Revision, candidate.Digest), UpdatedUnixNano: candidate.CreatedUnixNano})
	if err != nil {
		return toolaction.RecordV2{}, err
	}
	if err = insertActionHeadTxV2(ctx, tx, head); err != nil {
		return toolaction.RecordV2{}, err
	}
	if err = tx.Commit(); err != nil {
		return toolaction.RecordV2{}, indeterminateV1("Tool Action Candidate commit outcome is unknown")
	}
	if s.fault != nil {
		if err = s.fault("candidate_after_commit"); err != nil {
			recoveryCtx, cancel := boundedSQLiteRecoveryContextV2(ctx)
			_, _, _ = readCandidateQueryV2(recoveryCtx, s.store.db, candidate.ID)
			cancel()
			return toolaction.RecordV2{}, indeterminateV1("Tool Action Candidate reply outcome is unknown; exact Inspect completed")
		}
	}
	if err = factStorePostCommitContextV2(ctx); err != nil {
		return toolaction.RecordV2{}, err
	}
	return toolaction.RecordV2{Candidate: cloneActionValueV2(candidate), Revision: 1}, nil
}

func (s *ActionFactStoreV2) InspectCandidateCurrentV2(ctx context.Context, exact toolcontract.ObjectRef, now time.Time) (toolcontract.ActionCandidateV2, error) {
	if err := s.readyV2(ctx, false); err != nil {
		return toolcontract.ActionCandidateV2{}, err
	}
	if exact.Validate() != nil || now.IsZero() {
		return toolcontract.ActionCandidateV2{}, invalidV1("exact Tool Action Candidate ref and time are required")
	}
	candidate, found, err := readCandidateQueryV2(ctx, s.store.db, exact.ID)
	if err != nil {
		return toolcontract.ActionCandidateV2{}, err
	}
	if !found {
		return toolcontract.ActionCandidateV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Tool Action Candidate not found")
	}
	if objectRefActionV2(candidate.ID, candidate.Revision, candidate.Digest) != exact {
		return toolcontract.ActionCandidateV2{}, conflictV1("Tool Action Candidate exact ref drifted")
	}
	head, found, err := readActionHeadQueryV2(ctx, s.store.db, exact.ID)
	if err != nil || !found || head.Candidate != exact {
		return toolcontract.ActionCandidateV2{}, conflictV1("Tool Action Candidate head exact ref drifted")
	}
	if err = s.inspectCandidateOwnerCurrentClosureV2(ctx, candidate, now); err != nil {
		return toolcontract.ActionCandidateV2{}, err
	}
	return cloneActionValueV2(candidate), nil
}

func (s *ActionFactStoreV2) inspectCandidateOwnerCurrentClosureV2(ctx context.Context, candidate toolcontract.ActionCandidateV2, now time.Time) error {
	if now.IsZero() || now.UnixNano() < candidate.CreatedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Action Candidate owner clock predates the immutable fact")
	}
	if now.UnixNano() >= candidate.CurrentExpiresUnixNano() {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Tool Action Candidate is not current")
	}
	if s.readers.OwnerCurrent == nil {
		return unavailableV1("Tool Action Candidate owner current reader is unavailable")
	}
	for _, expected := range []toolcontract.OwnerCurrentRefV1{candidate.PendingActionCurrent, candidate.SurfaceCurrent, candidate.CapabilityCurrent, candidate.ToolCurrent, candidate.InputSchemaCurrent, candidate.SourceCandidateCurrent} {
		actual, readErr := s.readers.OwnerCurrent.InspectOwnerCurrentV1(ctx, expected)
		if readErr != nil {
			return readErr
		}
		if !reflect.DeepEqual(actual, expected) || actual.Validate(now) != nil {
			return conflictV1("Tool Action Candidate owner current projection drifted")
		}
	}
	return contextErrorV1(ctx)
}

func (s *ActionFactStoreV2) CreateReservationFactV2(ctx context.Context, action toolcontract.ObjectRef, appAttempt toolcontract.ApplicationAttemptRefV1, intentDigest core.Digest, sessionRef string, subjectDigest core.Digest, now, expires time.Time) (toolcontract.ActionReservationFactV2, error) {
	ownerNowBefore, err := s.freshClockV2()
	if err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	if now.IsZero() || now.After(ownerNowBefore) {
		return toolcontract.ActionReservationFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Action Reservation caller clock is ahead of the Owner clock")
	}
	candidate, err := s.InspectCandidateCurrentV2(ctx, action, ownerNowBefore)
	if err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	if appAttempt.Validate() != nil || intentDigest.Validate() != nil || subjectDigest.Validate() != nil || !expires.After(now) || expires.UnixNano() > candidate.CurrentExpiresUnixNano() || sessionRef != candidate.SessionID {
		return toolcontract.ActionReservationFactV2{}, invalidV1("Tool Action Reservation exact bindings are invalid")
	}
	if !ownerNowBefore.Before(expires) {
		return toolcontract.ActionReservationFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Tool Action Reservation is expired at Owner admission")
	}
	id, err := toolcontract.StableID("reservation-v2", candidate.ID, appAttempt.ID, string(appAttempt.Digest), string(intentDigest))
	if err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	fact, err := toolcontract.SealActionReservationFactV2(toolcontract.ActionReservationFactV2{ID: id, TenantID: candidate.TenantID, Action: action, ApplicationAttempt: appAttempt, IntentDigest: intentDigest, SessionRef: sessionRef, DomainSubjectDigest: subjectDigest, ReservedUnixNano: now.UnixNano(), ExpiresUnixNano: expires.UnixNano()})
	if err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	body, rowDigest, err := encodeActionRowV2("ActionReservationFactV2", fact)
	if err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	lockClockStart, err := s.freshClockV2()
	if err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	if now.After(lockClockStart) || !lockClockStart.Before(expires) {
		return toolcontract.ActionReservationFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Tool Action Reservation is not current at Owner lock admission")
	}
	tx, err := s.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return toolcontract.ActionReservationFactV2{}, mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	head, found, err := readActionHeadTxV2(ctx, tx, action.ID)
	if err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	if !found || head.Candidate != action {
		return toolcontract.ActionReservationFactV2{}, conflictV1("Tool Action Reservation candidate head drifted")
	}
	if head.Reservation != nil {
		existing, readErr := readReservationTxV2(ctx, tx, action.ID)
		if readErr != nil {
			return toolcontract.ActionReservationFactV2{}, readErr
		}
		if reflect.DeepEqual(existing, fact) {
			return existing, nil
		}
		return toolcontract.ActionReservationFactV2{}, conflictV1("Tool Action Reservation create-once content drifted")
	}
	if head.Stage != "candidate" {
		return toolcontract.ActionReservationFactV2{}, conflictV1("Tool Action Reservation head stage drifted")
	}
	candidateCurrent, candidateFound, err := readCandidateTxV2(ctx, tx, action.ID)
	if err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	lockClockEnd, err := s.freshClockV2()
	if err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	if !candidateFound || !reflect.DeepEqual(candidateCurrent, candidate) || candidateCurrent.Validate() != nil ||
		lockClockEnd.UnixNano() < candidateCurrent.CreatedUnixNano || lockClockEnd.UnixNano() >= candidateCurrent.CurrentExpiresUnixNano() ||
		!lockClockEnd.Before(expires) {
		return toolcontract.ActionReservationFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Tool Action Reservation current expired or drifted while waiting for the write lock")
	}
	if err = s.inspectCandidateOwnerCurrentClosureV2(ctx, candidateCurrent, lockClockEnd); err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	if err = contextErrorV1(ctx); err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO tool_action_reservation_v2(fact_id,revision,digest,action_id,body_json,row_digest) VALUES(?,?,?,?,?,?)`,
		fact.ID, int64(fact.Revision), string(fact.Digest), action.ID, body, string(rowDigest)); err != nil {
		return toolcontract.ActionReservationFactV2{}, mapDBErrorV1(ctx, err, true)
	}
	next := head
	next.Revision++
	next.Stage = "reserved"
	ref := objectRefActionV2(fact.ID, fact.Revision, fact.Digest)
	next.Reservation = &ref
	next.UpdatedUnixNano = lockClockEnd.UnixNano()
	next, err = sealActionHeadV2(next)
	if err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	if err = casActionHeadTxV2(ctx, tx, head, next); err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	commitNow, err := s.freshClockV2()
	if err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	if !commitNow.Before(expires) || commitNow.UnixNano() >= candidateCurrent.CurrentExpiresUnixNano() {
		return toolcontract.ActionReservationFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Tool Action Reservation expired before commit")
	}
	if err = s.inspectCandidateOwnerCurrentClosureV2(ctx, candidateCurrent, commitNow); err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	commitSealNow, err := s.freshClockV2()
	if err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	if !commitSealNow.Before(expires) || commitSealNow.UnixNano() >= candidateCurrent.CurrentExpiresUnixNano() {
		return toolcontract.ActionReservationFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Tool Action Reservation expired at commit boundary")
	}
	if err = tx.Commit(); err != nil {
		return toolcontract.ActionReservationFactV2{}, indeterminateV1("Tool Action Reservation commit outcome is unknown")
	}
	if s.fault != nil {
		if err = s.fault("reservation_after_commit"); err != nil {
			recoveryCtx, cancel := boundedSQLiteRecoveryContextV2(ctx)
			_, _ = s.InspectReservationExactV2(recoveryCtx, action.ID, ref)
			cancel()
			return toolcontract.ActionReservationFactV2{}, indeterminateV1("Tool Action Reservation reply outcome is unknown; exact Inspect completed")
		}
	}
	if err = factStorePostCommitContextV2(ctx); err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	return fact, nil
}

func (s *ActionFactStoreV2) InspectReservationExactV2(ctx context.Context, actionID string, exact toolcontract.ObjectRef) (toolcontract.ActionReservationFactV2, error) {
	if err := s.readyV2(ctx, false); err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	if exact.Validate() != nil {
		return toolcontract.ActionReservationFactV2{}, invalidV1("exact Tool Action Reservation ref is required")
	}
	fact, err := readReservationQueryV2(ctx, s.store.db, actionID)
	if err != nil {
		return toolcontract.ActionReservationFactV2{}, err
	}
	if objectRefActionV2(fact.ID, fact.Revision, fact.Digest) != exact {
		return toolcontract.ActionReservationFactV2{}, conflictV1("Tool Action Reservation exact ref drifted")
	}
	head, found, err := readActionHeadQueryV2(ctx, s.store.db, actionID)
	if err != nil || !found || head.Reservation == nil || *head.Reservation != exact {
		return toolcontract.ActionReservationFactV2{}, conflictV1("Tool Action Reservation head exact ref drifted")
	}
	return fact, nil
}

func (s *ActionFactStoreV2) CreateDomainResultFactV2(ctx context.Context, fact toolcontract.ToolDomainResultFactV2) (toolcontract.ToolDomainResultFactV2, error) {
	if err := s.readyV2(ctx, true); err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	if err := s.rereadDomainCausalityV2(ctx, fact); err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	body, rowDigest, err := encodeActionRowV2("ToolDomainResultFactV2", fact)
	if err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	tx, err := s.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return toolcontract.ToolDomainResultFactV2{}, mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	head, found, err := readActionHeadTxV2(ctx, tx, fact.Action.ID)
	if err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	if !found || head.Candidate != fact.Action || head.Reservation == nil || *head.Reservation != fact.Reservation {
		return toolcontract.ToolDomainResultFactV2{}, conflictV1("Tool DomainResult head or predecessor drifted")
	}
	candidate, found, err := readCandidateTxV2(ctx, tx, fact.Action.ID)
	if err != nil || !found {
		return toolcontract.ToolDomainResultFactV2{}, conflictV1("Tool DomainResult Candidate predecessor is absent")
	}
	reservation, err := readReservationTxV2(ctx, tx, fact.Action.ID)
	if err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	if reservation.ApplicationAttempt != fact.ApplicationAttempt || candidate.TenantID != fact.TenantID || candidate.OperationScopeDigest != fact.OperationScopeDigest || candidate.ExpectedOwner != fact.Owner {
		return toolcontract.ToolDomainResultFactV2{}, conflictV1("Tool DomainResult causal chain drifted")
	}
	if head.DomainResult != nil {
		existing, readErr := readDomainQueryV2(ctx, tx, fact.Action.ID)
		if readErr != nil {
			return toolcontract.ToolDomainResultFactV2{}, readErr
		}
		if reflect.DeepEqual(existing, fact) {
			return existing, nil
		}
		return toolcontract.ToolDomainResultFactV2{}, conflictV1("Tool DomainResult create-once content drifted")
	}
	if head.Stage != "reserved" {
		return toolcontract.ToolDomainResultFactV2{}, conflictV1("Tool DomainResult head stage drifted")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO tool_domain_result_v2(fact_id,revision,digest,action_id,body_json,row_digest) VALUES(?,?,?,?,?,?)`,
		fact.ID, int64(fact.Revision), string(fact.Digest), fact.Action.ID, body, string(rowDigest)); err != nil {
		return toolcontract.ToolDomainResultFactV2{}, mapDBErrorV1(ctx, err, true)
	}
	next := head
	next.Revision++
	next.Stage = "domain_result"
	ref := objectRefActionV2(fact.ID, fact.Revision, fact.Digest)
	next.DomainResult = &ref
	next.UpdatedUnixNano = fact.CreatedUnixNano
	next, err = sealActionHeadV2(next)
	if err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	if err = casActionHeadTxV2(ctx, tx, head, next); err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	if err = tx.Commit(); err != nil {
		return toolcontract.ToolDomainResultFactV2{}, indeterminateV1("Tool DomainResult commit outcome is unknown")
	}
	if s.fault != nil {
		if err = s.fault("domain_result_after_commit"); err != nil {
			recoveryCtx, cancel := boundedSQLiteRecoveryContextV2(ctx)
			_, _ = s.InspectDomainResultExactV2(recoveryCtx, fact.Action.ID, ref)
			cancel()
			return toolcontract.ToolDomainResultFactV2{}, indeterminateV1("Tool DomainResult reply outcome is unknown; exact Inspect completed")
		}
	}
	if err = factStorePostCommitContextV2(ctx); err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	return cloneActionValueV2(fact), nil
}

func (s *ActionFactStoreV2) InspectDomainResultExactV2(ctx context.Context, actionID string, exact toolcontract.ObjectRef) (toolcontract.ToolDomainResultFactV2, error) {
	if err := s.readyV2(ctx, false); err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	if exact.Validate() != nil {
		return toolcontract.ToolDomainResultFactV2{}, invalidV1("exact Tool DomainResult ref is required")
	}
	fact, err := readDomainQueryV2(ctx, s.store.db, actionID)
	if err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	if objectRefActionV2(fact.ID, fact.Revision, fact.Digest) != exact {
		return toolcontract.ToolDomainResultFactV2{}, conflictV1("Tool DomainResult exact ref drifted")
	}
	head, found, err := readActionHeadQueryV2(ctx, s.store.db, actionID)
	if err != nil || !found || head.DomainResult == nil || *head.DomainResult != exact {
		return toolcontract.ToolDomainResultFactV2{}, conflictV1("Tool DomainResult head exact ref drifted")
	}
	return fact, nil
}

func (s *ActionFactStoreV2) InspectDomainResultCurrentByExactV1(ctx context.Context, exact toolcontract.ObjectRef, now time.Time, ttl time.Duration) (toolcontract.ToolDomainResultCurrentProjectionV1, error) {
	if exact.Validate() != nil || now.IsZero() || ttl <= 0 || ttl > toolcontract.MaxDomainResultCurrentTTLV1 {
		return toolcontract.ToolDomainResultCurrentProjectionV1{}, invalidV1("Tool DomainResult exact current request is invalid")
	}
	// The FactStoreV2 contract makes caller now authoritative for currentness.
	// The injected store clock is still sampled to detect a broken persistence
	// clock, but it must never silently replace the caller coordinate.
	if _, err := s.freshClockV2(); err != nil {
		return toolcontract.ToolDomainResultCurrentProjectionV1{}, err
	}
	fresh := now
	var actionID string
	if err := s.store.db.QueryRowContext(ctx, `SELECT action_id FROM tool_domain_result_v2 WHERE fact_id=? AND revision=? AND digest=?`, exact.ID, int64(exact.Revision), string(exact.Digest)).Scan(&actionID); err != nil {
		return toolcontract.ToolDomainResultCurrentProjectionV1{}, mapDBErrorV1(ctx, err, false)
	}
	fact, err := s.InspectDomainResultExactV2(ctx, actionID, exact)
	if err != nil {
		return toolcontract.ToolDomainResultCurrentProjectionV1{}, err
	}
	if fresh.UnixNano() < fact.CreatedUnixNano {
		return toolcontract.ToolDomainResultCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool DomainResult current caller clock predates the immutable fact")
	}
	if err = s.rereadDomainCausalityV2(ctx, fact); err != nil {
		return toolcontract.ToolDomainResultCurrentProjectionV1{}, err
	}
	expires := fresh.Add(ttl).UnixNano()
	for _, natural := range []int64{fact.PreparedAttempt.ExpiresUnixNano, fact.PrepareEnforcement.ExpiresUnixNano, fact.ExecuteEnforcement.ExpiresUnixNano} {
		if natural > 0 && natural < expires {
			expires = natural
		}
	}
	if fresh.UnixNano() >= expires {
		return toolcontract.ToolDomainResultCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Tool DomainResult current natural evidence bound expired")
	}
	projection := toolcontract.ToolDomainResultCurrentProjectionV1{ContractVersion: toolcontract.ResultContractVersionV2, Fact: exact, CausalityDigest: fact.Causality.Digest, Observation: fact.Observation, PrepareEnforcement: fact.PrepareEnforcement, ExecuteEnforcement: fact.ExecuteEnforcement, PrepareConsumption: fact.PrepareConsumption, ExecuteConsumption: fact.ExecuteConsumption, Owner: fact.Owner, CheckedUnixNano: fresh.UnixNano(), ExpiresUnixNano: expires}
	projection.Digest, err = projection.ComputeDigest()
	if err != nil {
		return toolcontract.ToolDomainResultCurrentProjectionV1{}, err
	}
	return projection, projection.Validate(fresh)
}

func (s *ActionFactStoreV2) freshClockV2() (time.Time, error) {
	if s == nil || s.store == nil || s.store.clock == nil {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Action Fact SQLite clock is unavailable")
	}
	s.clockMu.Lock()
	defer s.clockMu.Unlock()
	now := s.store.clock()
	if now.IsZero() || (!s.lastNow.IsZero() && now.Before(s.lastNow)) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Action Fact SQLite clock regressed")
	}
	s.lastNow = now
	return now, nil
}

func (s *ActionFactStoreV2) ApplySettlementAndCreateResultV2(ctx context.Context, actionID string, domainRef toolcontract.ObjectRef, inspection runtimeports.OperationInspectionSettlementRefV4, outcome toolcontract.ToolOutcomeV2, disposition toolcontract.ToolDispositionV2, now time.Time) (toolcontract.ToolResultV2, error) {
	if err := s.readyV2(ctx, true); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if now.IsZero() || inspection.Validate(now) != nil || toolcontract.ValidateToolOutcomeDispositionV2(outcome, disposition) != nil {
		return toolcontract.ToolResultV2{}, invalidV1("fresh Runtime inspection and legal Tool outcome are required")
	}
	ownerNowBefore, err := s.freshClockV2()
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if now.After(ownerNowBefore) {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Apply caller clock is ahead of the Owner clock")
	}
	if inspection.Validate(ownerNowBefore) != nil {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Runtime settlement inspection is not current at Owner admission")
	}
	domainBefore, err := s.InspectDomainResultExactV2(ctx, actionID, domainRef)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	lockClockStart, err := s.freshClockV2()
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if now.UnixNano() < inspection.CheckedUnixNano || inspection.Validate(lockClockStart) != nil || s.readers.Settlement == nil {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "fresh Runtime settlement reader is unavailable or inspection is not current")
	}
	actual, err := s.readers.Settlement.InspectCurrentOperationSettlementV4(ctx, runtimeports.InspectCurrentOperationSettlementRequestV4{Operation: domainBefore.Causality.Operation, EffectID: domainBefore.Causality.EffectID})
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if actual.Validate(lockClockStart) != nil || !reflect.DeepEqual(actual, inspection) {
		return toolcontract.ToolResultV2{}, conflictV1("Runtime current settlement inspection drifted before Tool Apply")
	}
	association, err := s.readers.Settlement.InspectOperationSettlementEvidenceAssociationV4(ctx, domainBefore.Causality.Operation, actual.Association)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if association.Validate() != nil || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(association.RefV4(), actual.Association) {
		return toolcontract.ToolResultV2{}, conflictV1("Runtime settlement association drifted before Tool Apply")
	}
	tx, err := s.store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return toolcontract.ToolResultV2{}, mapDBErrorV1(ctx, err, true)
	}
	defer func() { _ = tx.Rollback() }()
	head, found, err := readActionHeadTxV2(ctx, tx, actionID)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if !found || head.DomainResult == nil || *head.DomainResult != domainRef {
		return toolcontract.ToolResultV2{}, conflictV1("Tool settlement DomainResult head drifted")
	}
	domain, err := readDomainQueryV2(ctx, tx, actionID)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	actualAtCommit, err := s.readers.Settlement.InspectCurrentOperationSettlementV4(ctx, runtimeports.InspectCurrentOperationSettlementRequestV4{Operation: domain.Causality.Operation, EffectID: domain.Causality.EffectID})
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	associationAtCommit, err := s.readers.Settlement.InspectOperationSettlementEvidenceAssociationV4(ctx, domain.Causality.Operation, actualAtCommit.Association)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	lockClockEnd, err := s.freshClockV2()
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if actualAtCommit.Validate(lockClockEnd) != nil || !reflect.DeepEqual(actualAtCommit, inspection) ||
		associationAtCommit.Validate() != nil ||
		!runtimeports.SameOperationSettlementEvidenceAssociationRefV4(associationAtCommit.RefV4(), actualAtCommit.Association) ||
		!reflect.DeepEqual(associationAtCommit, association) {
		return toolcontract.ToolResultV2{}, conflictV1("Runtime settlement current or association drifted while waiting for the Tool write lock")
	}
	if err = contextErrorV1(ctx); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	inspection = actualAtCommit
	rdr := inspection.DomainResult
	if rdr.ID != domain.ID || rdr.Revision != domain.Revision || rdr.Digest != domain.Digest || rdr.TenantID != domain.TenantID || rdr.OperationDigest != domain.Causality.OperationDigest || !reflect.DeepEqual(rdr.Attempt, domain.Causality.Attempt) || rdr.Schema != domain.Schema || rdr.PayloadDigest != domain.PayloadDigest || rdr.PayloadRevision != domain.PayloadRevision || inspection.Owner != domain.Owner {
		return toolcontract.ToolResultV2{}, conflictV1("Runtime inspection does not close the exact Tool DomainResult")
	}
	if head.Stage == "settled" {
		result, readErr := readResultQueryV2(ctx, tx, actionID)
		if readErr != nil {
			return toolcontract.ToolResultV2{}, readErr
		}
		apply, applyErr := readApplyQueryV2(ctx, tx, actionID)
		if applyErr != nil || head.Apply == nil || objectRefActionV2(apply.ID, apply.Revision, apply.Digest) != *head.Apply || result.Apply != *head.Apply {
			return toolcontract.ToolResultV2{}, conflictV1("Tool settled ApplySettlement row drifted")
		}
		if result.Inspection.Digest == inspection.Digest && result.Outcome == outcome && result.Disposition == disposition {
			return result, nil
		}
		return toolcontract.ToolResultV2{}, conflictV1("Tool settlement already binds different content")
	}
	if head.Stage != "domain_result" {
		return toolcontract.ToolResultV2{}, conflictV1("Tool settlement head stage drifted")
	}
	applyID, err := toolcontract.StableID("tool-apply-v2", actionID, domain.ID, string(inspection.Digest))
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	apply, err := toolcontract.SealToolApplySettlementFactV2(toolcontract.ToolApplySettlementFactV2{ID: applyID, TenantID: domain.TenantID, OperationScopeDigest: domain.OperationScopeDigest, Action: domain.Action, Reservation: domain.Reservation, DomainResult: domainRef, Inspection: inspection, Outcome: outcome, Disposition: disposition, Owner: domain.Owner, AppliedUnixNano: now.UnixNano()})
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	resultID, err := toolcontract.StableID("tool-result-v2", actionID, domain.ID, apply.ID, string(apply.Digest))
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	result, err := toolcontract.SealToolResultV2(toolcontract.ToolResultV2{ID: resultID, Action: domain.Action, Reservation: domain.Reservation, DomainResult: domainRef, Apply: objectRefActionV2(apply.ID, apply.Revision, apply.Digest), Inspection: inspection, Outcome: outcome, Disposition: disposition, Schema: domain.Schema, PayloadDigest: domain.PayloadDigest, PayloadRevision: domain.PayloadRevision, Residuals: append([]toolcontract.Residual(nil), domain.Residuals...), FinalizedUnixNano: now.UnixNano()})
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	applyBody, applyRow, err := encodeActionRowV2("ToolApplySettlementFactV2", apply)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	resultBody, resultRow, err := encodeActionRowV2("ToolResultV2", result)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO tool_apply_settlement_v2(fact_id,revision,digest,action_id,body_json,row_digest) VALUES(?,?,?,?,?,?)`,
		apply.ID, int64(apply.Revision), string(apply.Digest), actionID, applyBody, string(applyRow)); err != nil {
		return toolcontract.ToolResultV2{}, mapDBErrorV1(ctx, err, true)
	}
	if s.fault != nil {
		if err = s.fault("after_apply_insert"); err != nil {
			return toolcontract.ToolResultV2{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO tool_result_v2(fact_id,revision,digest,action_id,apply_id,body_json,row_digest) VALUES(?,?,?,?,?,?,?)`,
		result.ID, int64(result.Revision), string(result.Digest), actionID, apply.ID, resultBody, string(resultRow)); err != nil {
		return toolcontract.ToolResultV2{}, mapDBErrorV1(ctx, err, true)
	}
	if s.fault != nil {
		if err = s.fault("after_result_insert"); err != nil {
			return toolcontract.ToolResultV2{}, err
		}
	}
	next := head
	next.Revision++
	next.Stage = "settled"
	applyRef := objectRefActionV2(apply.ID, apply.Revision, apply.Digest)
	resultRef := objectRefActionV2(result.ID, result.Revision, result.Digest)
	next.Apply, next.Result = &applyRef, &resultRef
	next.UpdatedUnixNano = lockClockEnd.UnixNano()
	next, err = sealActionHeadV2(next)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if err = casActionHeadTxV2(ctx, tx, head, next); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if s.fault != nil {
		if err = s.fault("after_head_cas"); err != nil {
			return toolcontract.ToolResultV2{}, err
		}
	}
	commitNow, err := s.freshClockV2()
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	actualBeforeCommit, err := s.readers.Settlement.InspectCurrentOperationSettlementV4(ctx, runtimeports.InspectCurrentOperationSettlementRequestV4{Operation: domain.Causality.Operation, EffectID: domain.Causality.EffectID})
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	associationBeforeCommit, err := s.readers.Settlement.InspectOperationSettlementEvidenceAssociationV4(ctx, domain.Causality.Operation, actualBeforeCommit.Association)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if actualBeforeCommit.Validate(commitNow) != nil || !reflect.DeepEqual(actualBeforeCommit, inspection) ||
		associationBeforeCommit.Validate() != nil ||
		!runtimeports.SameOperationSettlementEvidenceAssociationRefV4(associationBeforeCommit.RefV4(), actualBeforeCommit.Association) ||
		!reflect.DeepEqual(associationBeforeCommit, association) {
		return toolcontract.ToolResultV2{}, conflictV1("Runtime settlement current or association drifted before Tool commit")
	}
	commitSealNow, err := s.freshClockV2()
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if actualBeforeCommit.Validate(commitSealNow) != nil {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "Runtime settlement inspection expired at Tool commit boundary")
	}
	if err = contextErrorV1(ctx); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if err = tx.Commit(); err != nil {
		return toolcontract.ToolResultV2{}, indeterminateV1("Tool ApplySettlement and Result commit outcome is unknown")
	}
	if s.fault != nil {
		if err = s.fault("after_commit"); err != nil {
			recoveryCtx, cancel := boundedSQLiteRecoveryContextV2(ctx)
			_, _ = s.InspectSettledResultForApplyV2(recoveryCtx, actionID, applyRef)
			cancel()
			return toolcontract.ToolResultV2{}, indeterminateV1("Tool ApplySettlement and Result reply outcome is unknown; exact Inspect completed")
		}
	}
	if err = factStorePostCommitContextV2(ctx); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	return cloneActionValueV2(result), nil
}

func factStorePostCommitContextV2(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil {
		return indeterminateV1("Tool Action fact committed while caller outcome became unknown; only exact Inspect may continue")
	}
	return nil
}

func (s *ActionFactStoreV2) InspectResultExactV2(ctx context.Context, actionID string, exact toolcontract.ObjectRef) (toolcontract.ToolResultV2, error) {
	if err := s.readyV2(ctx, false); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if exact.Validate() != nil {
		return toolcontract.ToolResultV2{}, invalidV1("exact ToolResult ref is required")
	}
	result, err := readResultQueryV2(ctx, s.store.db, actionID)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if objectRefActionV2(result.ID, result.Revision, result.Digest) != exact {
		return toolcontract.ToolResultV2{}, conflictV1("ToolResult exact ref drifted")
	}
	head, found, err := readActionHeadQueryV2(ctx, s.store.db, actionID)
	if err != nil || !found || head.Stage != "settled" || head.Result == nil || *head.Result != exact || head.Apply == nil || result.Apply != *head.Apply {
		return toolcontract.ToolResultV2{}, conflictV1("ToolResult head exact refs drifted")
	}
	apply, err := readApplyQueryV2(ctx, s.store.db, actionID)
	if err != nil || objectRefActionV2(apply.ID, apply.Revision, apply.Digest) != *head.Apply {
		return toolcontract.ToolResultV2{}, conflictV1("ToolResult ApplySettlement row drifted")
	}
	return result, nil
}

func (s *ActionFactStoreV2) InspectSettledResultForApplyV2(ctx context.Context, actionID string, apply toolcontract.ObjectRef) (toolcontract.ToolResultV2, error) {
	if err := s.readyV2(ctx, false); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if apply.Validate() != nil {
		return toolcontract.ToolResultV2{}, invalidV1("exact ApplySettlement ref is required")
	}
	head, found, err := readActionHeadQueryV2(ctx, s.store.db, actionID)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if !found || head.Stage != "settled" || head.Apply == nil || *head.Apply != apply || head.Result == nil {
		return toolcontract.ToolResultV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "settled ToolResult not found")
	}
	result, err := s.InspectResultExactV2(ctx, actionID, *head.Result)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if result.Apply != apply {
		return toolcontract.ToolResultV2{}, conflictV1("ApplySettlement exact ref drifted from ToolResult")
	}
	applyFact, err := readApplyQueryV2(ctx, s.store.db, actionID)
	if err != nil || objectRefActionV2(applyFact.ID, applyFact.Revision, applyFact.Digest) != apply {
		return toolcontract.ToolResultV2{}, conflictV1("ApplySettlement exact row drifted")
	}
	return result, nil
}

func (s *ActionFactStoreV2) rereadDomainCausalityV2(ctx context.Context, fact toolcontract.ToolDomainResultFactV2) error {
	if s.readers.Observation == nil || s.readers.Enforcement == nil || s.readers.Consumption == nil {
		return unavailableV1("Tool DomainResult causal readers are unavailable")
	}
	observation, err := s.readers.Observation.InspectProviderObservationExactV1(ctx, fact.Observation)
	if err != nil || !reflect.DeepEqual(observation, fact.Observation) {
		if err != nil {
			return err
		}
		return conflictV1("Provider Observation exact ref drifted")
	}
	for _, expected := range []runtimeports.OperationDispatchEnforcementPhaseRefV4{fact.PrepareEnforcement, fact.ExecuteEnforcement} {
		actual, readErr := s.readers.Enforcement.InspectEnforcementPhaseExactV1(ctx, expected)
		if readErr != nil || !reflect.DeepEqual(actual, expected) {
			if readErr != nil {
				return readErr
			}
			return conflictV1("Enforcement exact ref drifted")
		}
	}
	for _, expected := range []runtimeports.OperationScopeEvidenceConsumptionRefV3{fact.PrepareConsumption, fact.ExecuteConsumption} {
		actual, readErr := s.readers.Consumption.InspectEvidenceConsumptionExactV1(ctx, expected)
		if readErr != nil || !reflect.DeepEqual(actual, expected) {
			if readErr != nil {
				return readErr
			}
			return conflictV1("Evidence Consumption exact ref drifted")
		}
	}
	return contextErrorV1(ctx)
}

type actionQueryerV2 interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func encodeActionRowV2(discriminator string, value any) ([]byte, core.Digest, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, "", invalidV1("Tool Action fact JSON encode failed")
	}
	digest, err := rowDigestV1(discriminator, value)
	return body, digest, err
}

func decodeActionRowV2[T any](ctx context.Context, row scanRowV1, discriminator string, validate func(T) error, exact func(T) (string, core.Revision, core.Digest, string)) (T, error) {
	var zero T
	var id, digest, actionID, storedRow string
	var revision int64
	var body []byte
	if err := row.Scan(&id, &revision, &digest, &actionID, &body, &storedRow); err != nil {
		return zero, mapDBErrorV1(ctx, err, false)
	}
	var value T
	if core.DecodeStrictJSON(body, &value) != nil || !canonicalJSONBytesV1(body, value) || validate(value) != nil {
		return zero, conflictV1("stored Tool Action fact JSON is invalid")
	}
	valueID, valueRevision, valueDigest, valueAction := exact(value)
	rowDigest, err := rowDigestV1(discriminator, value)
	if err != nil || id != valueID || revision != int64(valueRevision) || digest != string(valueDigest) || actionID != valueAction || storedRow != string(rowDigest) {
		return zero, conflictV1("stored Tool Action fact row digest or exact refs drifted")
	}
	return value, nil
}

func readCandidateQueryV2(ctx context.Context, q actionQueryerV2, actionID string) (toolcontract.ActionCandidateV2, bool, error) {
	row := q.QueryRowContext(ctx, `SELECT fact_id,revision,digest,action_id,body_json,row_digest FROM tool_action_candidate_v2 WHERE action_id=?`, actionID)
	value, err := decodeActionRowV2(ctx, row, "ActionCandidateV2", func(v toolcontract.ActionCandidateV2) error { return v.Validate() }, func(v toolcontract.ActionCandidateV2) (string, core.Revision, core.Digest, string) {
		return v.ID, v.Revision, v.Digest, v.ID
	})
	if errors.Is(rawSQLErrorV2(row), sql.ErrNoRows) {
		return toolcontract.ActionCandidateV2{}, false, nil
	}
	if err != nil {
		if core.HasCategory(err, core.ErrorNotFound) {
			return toolcontract.ActionCandidateV2{}, false, nil
		}
		return toolcontract.ActionCandidateV2{}, false, err
	}
	return value, true, nil
}

// rawSQLErrorV2 exists only to keep generic row decoding centralized; sql.Row
// cannot be scanned twice, so absence is recognized through mapped NotFound.
func rawSQLErrorV2(*sql.Row) error { return nil }

func readCandidateTxV2(ctx context.Context, tx *sql.Tx, actionID string) (toolcontract.ActionCandidateV2, bool, error) {
	return readCandidateQueryV2(ctx, tx, actionID)
}

func readReservationQueryV2(ctx context.Context, q actionQueryerV2, actionID string) (toolcontract.ActionReservationFactV2, error) {
	return decodeActionRowV2(ctx, q.QueryRowContext(ctx, `SELECT fact_id,revision,digest,action_id,body_json,row_digest FROM tool_action_reservation_v2 WHERE action_id=?`, actionID), "ActionReservationFactV2", func(v toolcontract.ActionReservationFactV2) error { return v.Validate() }, func(v toolcontract.ActionReservationFactV2) (string, core.Revision, core.Digest, string) {
		return v.ID, v.Revision, v.Digest, v.Action.ID
	})
}

func readReservationTxV2(ctx context.Context, tx *sql.Tx, actionID string) (toolcontract.ActionReservationFactV2, error) {
	return readReservationQueryV2(ctx, tx, actionID)
}

func readDomainQueryV2(ctx context.Context, q actionQueryerV2, actionID string) (toolcontract.ToolDomainResultFactV2, error) {
	return decodeActionRowV2(ctx, q.QueryRowContext(ctx, `SELECT fact_id,revision,digest,action_id,body_json,row_digest FROM tool_domain_result_v2 WHERE action_id=?`, actionID), "ToolDomainResultFactV2", func(v toolcontract.ToolDomainResultFactV2) error { return v.Validate() }, func(v toolcontract.ToolDomainResultFactV2) (string, core.Revision, core.Digest, string) {
		return v.ID, v.Revision, v.Digest, v.Action.ID
	})
}

func readResultQueryV2(ctx context.Context, q actionQueryerV2, actionID string) (toolcontract.ToolResultV2, error) {
	return decodeActionRowV2(ctx, q.QueryRowContext(ctx, `SELECT fact_id,revision,digest,action_id,body_json,row_digest FROM tool_result_v2 WHERE action_id=?`, actionID), "ToolResultV2", func(v toolcontract.ToolResultV2) error { return v.Validate() }, func(v toolcontract.ToolResultV2) (string, core.Revision, core.Digest, string) {
		return v.ID, v.Revision, v.Digest, v.Action.ID
	})
}

func readApplyQueryV2(ctx context.Context, q actionQueryerV2, actionID string) (toolcontract.ToolApplySettlementFactV2, error) {
	return decodeActionRowV2(ctx, q.QueryRowContext(ctx, `SELECT fact_id,revision,digest,action_id,body_json,row_digest FROM tool_apply_settlement_v2 WHERE action_id=?`, actionID), "ToolApplySettlementFactV2", func(v toolcontract.ToolApplySettlementFactV2) error {
		return v.Validate()
	}, func(v toolcontract.ToolApplySettlementFactV2) (string, core.Revision, core.Digest, string) {
		return v.ID, v.Revision, v.Digest, v.Action.ID
	})
}

func insertActionHeadTxV2(ctx context.Context, tx *sql.Tx, head actionHeadV2) error {
	args, rowDigest, err := actionHeadColumnsV2(head)
	if err != nil {
		return err
	}
	args = append(args, string(rowDigest))
	_, err = tx.ExecContext(ctx, `INSERT INTO tool_action_head_v2(action_id,head_revision,head_digest,stage,candidate_id,candidate_revision,candidate_digest,reservation_id,reservation_revision,reservation_digest,domain_result_id,domain_result_revision,domain_result_digest,apply_id,apply_revision,apply_digest,result_id,result_revision,result_digest,updated_unix_nano,row_digest) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, args...)
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	return nil
}

func casActionHeadTxV2(ctx context.Context, tx *sql.Tx, old, next actionHeadV2) error {
	args, rowDigest, err := actionHeadColumnsV2(next)
	if err != nil {
		return err
	}
	args = append(args, string(rowDigest), old.ActionID, int64(old.Revision), string(old.Digest))
	result, err := tx.ExecContext(ctx, `UPDATE tool_action_head_v2 SET action_id=?,head_revision=?,head_digest=?,stage=?,candidate_id=?,candidate_revision=?,candidate_digest=?,reservation_id=?,reservation_revision=?,reservation_digest=?,domain_result_id=?,domain_result_revision=?,domain_result_digest=?,apply_id=?,apply_revision=?,apply_digest=?,result_id=?,result_revision=?,result_digest=?,updated_unix_nano=?,row_digest=? WHERE action_id=? AND head_revision=? AND head_digest=?`, args...)
	if err != nil {
		return mapDBErrorV1(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return conflictV1("Tool Action head CAS lost")
	}
	return nil
}

func actionHeadColumnsV2(head actionHeadV2) ([]any, core.Digest, error) {
	sealed, err := sealActionHeadV2(head)
	if err != nil || sealed.Digest != head.Digest {
		return nil, "", conflictV1("Tool Action head digest drifted")
	}
	rowDigest, err := rowDigestV1("ActionHeadV2", head)
	if err != nil {
		return nil, "", err
	}
	refColumns := func(ref *toolcontract.ObjectRef) []any {
		if ref == nil {
			return []any{nil, nil, nil}
		}
		return []any{ref.ID, int64(ref.Revision), string(ref.Digest)}
	}
	args := []any{head.ActionID, int64(head.Revision), string(head.Digest), head.Stage, head.Candidate.ID, int64(head.Candidate.Revision), string(head.Candidate.Digest)}
	args = append(args, refColumns(head.Reservation)...)
	args = append(args, refColumns(head.DomainResult)...)
	args = append(args, refColumns(head.Apply)...)
	args = append(args, refColumns(head.Result)...)
	args = append(args, head.UpdatedUnixNano)
	return args, rowDigest, nil
}

func readActionHeadQueryV2(ctx context.Context, q actionQueryerV2, actionID string) (actionHeadV2, bool, error) {
	row := q.QueryRowContext(ctx, `SELECT action_id,head_revision,head_digest,stage,candidate_id,candidate_revision,candidate_digest,reservation_id,reservation_revision,reservation_digest,domain_result_id,domain_result_revision,domain_result_digest,apply_id,apply_revision,apply_digest,result_id,result_revision,result_digest,updated_unix_nano,row_digest FROM tool_action_head_v2 WHERE action_id=?`, actionID)
	var head actionHeadV2
	var revision, candidateRevision, updated int64
	var digest, candidateDigest, storedRow string
	var reservationID, reservationDigest, domainID, domainDigest, applyID, applyDigest, resultID, resultDigest sql.NullString
	var reservationRevision, domainRevision, applyRevision, resultRevision sql.NullInt64
	if err := row.Scan(&head.ActionID, &revision, &digest, &head.Stage, &head.Candidate.ID, &candidateRevision, &candidateDigest, &reservationID, &reservationRevision, &reservationDigest, &domainID, &domainRevision, &domainDigest, &applyID, &applyRevision, &applyDigest, &resultID, &resultRevision, &resultDigest, &updated, &storedRow); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return actionHeadV2{}, false, nil
		}
		return actionHeadV2{}, false, mapDBErrorV1(ctx, err, false)
	}
	head.Revision, head.Digest, head.Candidate.Revision, head.Candidate.Digest, head.UpdatedUnixNano = core.Revision(revision), core.Digest(digest), core.Revision(candidateRevision), core.Digest(candidateDigest), updated
	build := func(id, digest sql.NullString, revision sql.NullInt64) *toolcontract.ObjectRef {
		if !id.Valid && !digest.Valid && !revision.Valid {
			return nil
		}
		return &toolcontract.ObjectRef{ID: id.String, Revision: core.Revision(revision.Int64), Digest: core.Digest(digest.String)}
	}
	head.Reservation = build(reservationID, reservationDigest, reservationRevision)
	head.DomainResult = build(domainID, domainDigest, domainRevision)
	head.Apply = build(applyID, applyDigest, applyRevision)
	head.Result = build(resultID, resultDigest, resultRevision)
	sealed, err := sealActionHeadV2(head)
	rowDigest, rowErr := rowDigestV1("ActionHeadV2", head)
	if err != nil || rowErr != nil || sealed.Digest != head.Digest || string(rowDigest) != storedRow || !validActionHeadShapeV2(head) {
		return actionHeadV2{}, false, conflictV1("Tool Action head row is corrupt")
	}
	if err = validateActionHeadClosureV2(ctx, q, head); err != nil {
		return actionHeadV2{}, false, err
	}
	return head, true, nil
}

func readActionHeadTxV2(ctx context.Context, tx *sql.Tx, actionID string) (actionHeadV2, bool, error) {
	return readActionHeadQueryV2(ctx, tx, actionID)
}

func validActionHeadShapeV2(head actionHeadV2) bool {
	if head.ActionID == "" || head.Candidate.Validate() != nil || head.Candidate.ID != head.ActionID || head.Revision == 0 || head.UpdatedUnixNano <= 0 {
		return false
	}
	switch head.Stage {
	case "candidate":
		return head.Revision == 1 && head.Reservation == nil && head.DomainResult == nil && head.Apply == nil && head.Result == nil
	case "reserved":
		return head.Revision == 2 && head.Reservation != nil && head.Reservation.Validate() == nil && head.DomainResult == nil && head.Apply == nil && head.Result == nil
	case "domain_result":
		return head.Revision == 3 && head.Reservation != nil && head.DomainResult != nil && head.Reservation.Validate() == nil && head.DomainResult.Validate() == nil && head.Apply == nil && head.Result == nil
	case "settled":
		return head.Revision == 4 && head.Reservation != nil && head.DomainResult != nil && head.Apply != nil && head.Result != nil && head.Reservation.Validate() == nil && head.DomainResult.Validate() == nil && head.Apply.Validate() == nil && head.Result.Validate() == nil
	default:
		return false
	}
}

func validateActionHeadClosureV2(ctx context.Context, q actionQueryerV2, head actionHeadV2) error {
	candidate, found, err := readCandidateQueryV2(ctx, q, head.ActionID)
	if err != nil || !found || objectRefActionV2(candidate.ID, candidate.Revision, candidate.Digest) != head.Candidate {
		return conflictV1("Tool Action head Candidate immutable predecessor drifted")
	}

	var reservation toolcontract.ActionReservationFactV2
	if head.Reservation != nil {
		reservation, err = readReservationQueryV2(ctx, q, head.ActionID)
		if err != nil ||
			objectRefActionV2(reservation.ID, reservation.Revision, reservation.Digest) != *head.Reservation ||
			reservation.Action != head.Candidate ||
			reservation.TenantID != candidate.TenantID ||
			reservation.SessionRef != candidate.SessionID {
			return conflictV1("Tool Action head Reservation immutable predecessor drifted")
		}
	}

	var domain toolcontract.ToolDomainResultFactV2
	if head.DomainResult != nil {
		domain, err = readDomainQueryV2(ctx, q, head.ActionID)
		if err != nil || head.Reservation == nil ||
			objectRefActionV2(domain.ID, domain.Revision, domain.Digest) != *head.DomainResult ||
			domain.Action != head.Candidate ||
			domain.Reservation != *head.Reservation ||
			domain.ApplicationAttempt != reservation.ApplicationAttempt ||
			domain.TenantID != candidate.TenantID ||
			domain.OperationScopeDigest != candidate.OperationScopeDigest ||
			domain.Owner != candidate.ExpectedOwner {
			return conflictV1("Tool Action head DomainResult immutable predecessor drifted")
		}
	}

	var apply toolcontract.ToolApplySettlementFactV2
	if head.Apply != nil {
		apply, err = readApplyQueryV2(ctx, q, head.ActionID)
		if err != nil || head.Reservation == nil || head.DomainResult == nil ||
			objectRefActionV2(apply.ID, apply.Revision, apply.Digest) != *head.Apply ||
			apply.Action != head.Candidate ||
			apply.Reservation != *head.Reservation ||
			apply.DomainResult != *head.DomainResult ||
			apply.TenantID != domain.TenantID ||
			apply.OperationScopeDigest != domain.OperationScopeDigest ||
			apply.Owner != domain.Owner {
			return conflictV1("Tool Action head ApplySettlement immutable predecessor drifted")
		}
	}

	if head.Result != nil {
		result, readErr := readResultQueryV2(ctx, q, head.ActionID)
		if readErr != nil || head.Reservation == nil || head.DomainResult == nil || head.Apply == nil ||
			objectRefActionV2(result.ID, result.Revision, result.Digest) != *head.Result ||
			result.Action != head.Candidate ||
			result.Reservation != *head.Reservation ||
			result.DomainResult != *head.DomainResult ||
			result.Apply != *head.Apply ||
			!reflect.DeepEqual(result.Inspection, apply.Inspection) ||
			result.Outcome != apply.Outcome ||
			result.Disposition != apply.Disposition ||
			result.Schema != domain.Schema ||
			result.PayloadDigest != domain.PayloadDigest ||
			result.PayloadRevision != domain.PayloadRevision ||
			!reflect.DeepEqual(result.Residuals, domain.Residuals) {
			return conflictV1("Tool Action head Result immutable predecessor drifted")
		}
	}
	return nil
}

func readRecordTxV2(ctx context.Context, tx *sql.Tx, head actionHeadV2) (toolaction.RecordV2, error) {
	candidate, found, err := readCandidateTxV2(ctx, tx, head.ActionID)
	if err != nil || !found {
		return toolaction.RecordV2{}, conflictV1("Tool Action record Candidate is absent")
	}
	record := toolaction.RecordV2{Candidate: candidate, Revision: head.Revision}
	if head.Reservation != nil {
		value, readErr := readReservationTxV2(ctx, tx, head.ActionID)
		if readErr != nil || objectRefActionV2(value.ID, value.Revision, value.Digest) != *head.Reservation {
			return toolaction.RecordV2{}, conflictV1("Tool Action record Reservation drifted")
		}
		record.Reservation = &value
	}
	if head.DomainResult != nil {
		value, readErr := readDomainQueryV2(ctx, tx, head.ActionID)
		if readErr != nil || objectRefActionV2(value.ID, value.Revision, value.Digest) != *head.DomainResult {
			return toolaction.RecordV2{}, conflictV1("Tool Action record DomainResult drifted")
		}
		record.DomainResult = &value
	}
	if head.Result != nil {
		apply, readErr := readApplyQueryV2(ctx, tx, head.ActionID)
		if readErr != nil || head.Apply == nil || objectRefActionV2(apply.ID, apply.Revision, apply.Digest) != *head.Apply {
			return toolaction.RecordV2{}, conflictV1("Tool Action record ApplySettlement drifted")
		}
		record.Apply = &apply
		value, readErr := readResultQueryV2(ctx, tx, head.ActionID)
		if readErr != nil || objectRefActionV2(value.ID, value.Revision, value.Digest) != *head.Result {
			return toolaction.RecordV2{}, conflictV1("Tool Action record Result drifted")
		}
		record.Result = &value
	}
	return record, nil
}

func objectRefActionV2(id string, revision core.Revision, digest core.Digest) toolcontract.ObjectRef {
	return toolcontract.ObjectRef{ID: id, Revision: revision, Digest: digest}
}

func cloneActionValueV2[T any](value T) T {
	body, _ := json.Marshal(value)
	var out T
	_ = core.DecodeStrictJSON(body, &out)
	return out
}

var _ toolaction.FactStoreV2 = (*ActionFactStoreV2)(nil)
