package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/ports"
)

func (s *Store) CreateTimelineProjectionAttemptV1(ctx context.Context, candidate contract.TimelineProjectionAttemptFactV1) (contract.TimelineProjectionAttemptFactV1, bool, error) {
	if err := candidate.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, false, err
	}
	if candidate.Ref.Revision != 1 || candidate.State != contract.TimelineAttemptProposedV1 {
		return contract.TimelineProjectionAttemptFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "attempt_create", "create requires proposed revision one")
	}
	body, _, err := encode(candidate)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, false, unavailable("begin attempt create", err)
	}
	defer tx.Rollback()
	var bound string
	err = tx.QueryRowContext(ctx, "SELECT attempt_id FROM timeline_attempt_idempotency WHERE scope_digest=? AND idempotency_key=?", candidate.Ref.ScopeDigest, candidate.Request.IdempotencyKey).Scan(&bound)
	if err == nil && bound != candidate.Ref.AttemptID {
		return contract.TimelineProjectionAttemptFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "idempotency_key", "idempotency key belongs to another attempt")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return contract.TimelineProjectionAttemptFactV1{}, false, unavailable("inspect attempt idempotency", err)
	}
	existing, found, err := inspectCurrentAttemptTx(ctx, tx, candidate.Ref.ScopeDigest, candidate.Ref.AttemptID)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, false, err
	}
	if found {
		if existing.Request.Digest == candidate.Request.Digest {
			return existing, true, tx.Commit()
		}
		return contract.TimelineProjectionAttemptFactV1{}, false, contract.NewError(contract.ErrRevisionConflict, "attempt_id", "create-once attempt changed request")
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO timeline_attempt_history(scope_digest,attempt_id,revision,ref_digest,request_digest,body) VALUES(?,?,?,?,?,?)", candidate.Ref.ScopeDigest, candidate.Ref.AttemptID, 1, candidate.Ref.Digest, candidate.Request.Digest, body); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, false, unavailable("insert attempt history", err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO timeline_attempt_current(scope_digest,attempt_id,revision) VALUES(?,?,1)", candidate.Ref.ScopeDigest, candidate.Ref.AttemptID); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, false, unavailable("insert attempt current", err)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO timeline_attempt_idempotency(scope_digest,idempotency_key,attempt_id) VALUES(?,?,?)", candidate.Ref.ScopeDigest, candidate.Request.IdempotencyKey, candidate.Ref.AttemptID); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, false, unavailable("insert attempt idempotency", err)
	}
	if err = tx.Commit(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, false, unavailable("commit attempt create", err)
	}
	return candidate.Clone(), false, nil
}

func (s *Store) InspectTimelineProjectionAttemptV1(ctx context.Context, ref contract.TimelineProjectionAttemptRefV1) (contract.TimelineProjectionAttemptFactV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, "SELECT body FROM timeline_attempt_history WHERE scope_digest=? AND attempt_id=? AND revision=?", ref.ScopeDigest, ref.AttemptID, ref.Revision).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.TimelineProjectionAttemptFactV1{}, notFound("attempt_ref", "attempt revision not found")
		}
		return contract.TimelineProjectionAttemptFactV1{}, unavailable("inspect attempt", err)
	}
	fact, err := decodeAttempt(body)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	if fact.Ref != ref {
		return contract.TimelineProjectionAttemptFactV1{}, contract.NewError(contract.ErrRevisionConflict, "attempt_ref", "attempt digest drifted")
	}
	return fact, nil
}

func (s *Store) InspectCurrentTimelineProjectionAttemptV1(ctx context.Context, scopeDigest, attemptID string) (contract.TimelineProjectionAttemptFactV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	if err := contract.ValidateToken("scope_digest", scopeDigest); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	if err := contract.ValidateToken("attempt_id", attemptID); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	var body []byte
	err := s.db.QueryRowContext(ctx, `SELECT h.body FROM timeline_attempt_current c
		JOIN timeline_attempt_history h ON h.scope_digest=c.scope_digest AND h.attempt_id=c.attempt_id AND h.revision=c.revision
		WHERE c.scope_digest=? AND c.attempt_id=?`, scopeDigest, attemptID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.TimelineProjectionAttemptFactV1{}, notFound("attempt_id", "attempt not found")
	}
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, unavailable("inspect current attempt", err)
	}
	return decodeAttempt(body)
}

func inspectCurrentAttemptTx(ctx context.Context, tx *sql.Tx, scopeDigest, attemptID string) (contract.TimelineProjectionAttemptFactV1, bool, error) {
	var body []byte
	err := tx.QueryRowContext(ctx, `SELECT h.body FROM timeline_attempt_current c
		JOIN timeline_attempt_history h ON h.scope_digest=c.scope_digest AND h.attempt_id=c.attempt_id AND h.revision=c.revision
		WHERE c.scope_digest=? AND c.attempt_id=?`, scopeDigest, attemptID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return contract.TimelineProjectionAttemptFactV1{}, false, nil
	}
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, false, unavailable("inspect current attempt", err)
	}
	fact, err := decodeAttempt(body)
	return fact, true, err
}

func decodeAttempt(body []byte) (contract.TimelineProjectionAttemptFactV1, error) {
	var fact contract.TimelineProjectionAttemptFactV1
	if err := decode(body, &fact); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	if err := fact.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.NewError(contract.ErrContentDigestMismatch, "timeline_attempt", "stored attempt failed validation")
	}
	return fact.Clone(), nil
}

func (s *Store) CompareAndSwapTimelineProjectionAttemptV1(ctx context.Context, expected contract.TimelineProjectionAttemptRefV1, next contract.TimelineProjectionAttemptFactV1) (contract.TimelineProjectionAttemptFactV1, error) {
	if err := expected.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	if err := next.Validate(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	if next.State == contract.TimelineAttemptVisibleV1 {
		return contract.TimelineProjectionAttemptFactV1{}, contract.NewError(contract.ErrUnsupported, "attempt_state", "visible transition requires atomic publish")
	}
	body, _, err := encode(next)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, unavailable("begin attempt CAS", err)
	}
	defer tx.Rollback()
	current, found, err := inspectCurrentAttemptTx(ctx, tx, expected.ScopeDigest, expected.AttemptID)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	if !found {
		return contract.TimelineProjectionAttemptFactV1{}, notFound("attempt_id", "attempt not found")
	}
	if current.Ref != expected {
		return contract.TimelineProjectionAttemptFactV1{}, contract.NewError(contract.ErrRevisionConflict, "attempt_ref", "CAS expected ref is stale")
	}
	if err := validateAttemptSuccessor(current, next); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	if err := insertAttemptRevisionTx(ctx, tx, next, body); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, err
	}
	if err = tx.Commit(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, unavailable("commit attempt CAS", err)
	}
	return next.Clone(), nil
}

func insertAttemptRevisionTx(ctx context.Context, tx *sql.Tx, next contract.TimelineProjectionAttemptFactV1, body []byte) error {
	if _, err := tx.ExecContext(ctx, "INSERT INTO timeline_attempt_history(scope_digest,attempt_id,revision,ref_digest,request_digest,body) VALUES(?,?,?,?,?,?)", next.Ref.ScopeDigest, next.Ref.AttemptID, next.Ref.Revision, next.Ref.Digest, next.Request.Digest, body); err != nil {
		return unavailable("insert attempt revision", err)
	}
	result, err := tx.ExecContext(ctx, "UPDATE timeline_attempt_current SET revision=? WHERE scope_digest=? AND attempt_id=? AND revision=?", next.Ref.Revision, next.Ref.ScopeDigest, next.Ref.AttemptID, next.Ref.Revision-1)
	if err != nil {
		return unavailable("update attempt current", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return contract.NewError(contract.ErrRevisionConflict, "attempt_ref", "attempt current CAS lost")
	}
	return nil
}

func validateAttemptSuccessor(current, next contract.TimelineProjectionAttemptFactV1) error {
	if next.Ref.AttemptID != current.Ref.AttemptID || next.Ref.ScopeDigest != current.Ref.ScopeDigest || next.Ref.Revision != current.Ref.Revision+1 || next.Request.Digest != current.Request.Digest {
		return contract.NewError(contract.ErrRevisionConflict, "attempt_ref", "attempt identity, request or revision drifted")
	}
	return contract.AdvanceTimelineProjectionAttemptV1(current.State, next.State)
}

func (s *Store) PublishTimelineProjectionV1(ctx context.Context, request ports.PublishTimelineProjectionV1Request) (contract.TimelineProjectionAttemptFactV1, contract.TimelineProjectionCurrentV1, error) {
	if err := validatePublishRequest(request); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, unavailable("begin projection publish", err)
	}
	defer tx.Rollback()
	currentAttempt, found, err := inspectCurrentAttemptTx(ctx, tx, request.ExpectedAttempt.ScopeDigest, request.ExpectedAttempt.AttemptID)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	if !found {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, notFound("attempt_id", "attempt not found")
	}
	if currentAttempt.Ref != request.ExpectedAttempt {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrRevisionConflict, "attempt_ref", "CAS expected ref is stale")
	}
	if err := validateAttemptSuccessor(currentAttempt, request.VisibleAttempt); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	if _, _, err := putProjectionTx(ctx, tx, request.Event); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	currentBody, currentDigest, err := encode(request.Current)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	var existingBody []byte
	err = tx.QueryRowContext(ctx, "SELECT body FROM timeline_projection_current WHERE ledger_scope=? AND evidence_ref=?", request.Current.Event.LedgerScopeDigest, request.Current.Event.EvidenceRecordRef).Scan(&existingBody)
	if err == nil {
		var existing contract.TimelineProjectionCurrentV1
		if err := decode(existingBody, &existing); err != nil {
			return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
		}
		if !reflect.DeepEqual(existing, request.Current) {
			return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrRevisionConflict, "projection_current", "event already has another current projection")
		}
	} else if errors.Is(err, sql.ErrNoRows) {
		if _, err = tx.ExecContext(ctx, "INSERT INTO timeline_projection_current(ledger_scope,evidence_ref,digest,body) VALUES(?,?,?,?)", request.Current.Event.LedgerScopeDigest, request.Current.Event.EvidenceRecordRef, currentDigest, currentBody); err != nil {
			return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, unavailable("insert projection current", err)
		}
	} else {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, unavailable("inspect projection current", err)
	}
	attemptBody, _, err := encode(request.VisibleAttempt)
	if err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	if err := insertAttemptRevisionTx(ctx, tx, request.VisibleAttempt, attemptBody); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, err
	}
	if err = tx.Commit(); err != nil {
		return contract.TimelineProjectionAttemptFactV1{}, contract.TimelineProjectionCurrentV1{}, unavailable("commit projection publish", err)
	}
	return request.VisibleAttempt.Clone(), request.Current, nil
}

func validatePublishRequest(request ports.PublishTimelineProjectionV1Request) error {
	if err := request.ExpectedAttempt.Validate(); err != nil {
		return err
	}
	if err := request.VisibleAttempt.Validate(); err != nil {
		return err
	}
	if err := request.Event.Validate(); err != nil {
		return err
	}
	if err := request.Current.Validate(); err != nil {
		return err
	}
	if request.VisibleAttempt.State != contract.TimelineAttemptVisibleV1 || request.VisibleAttempt.Event == nil {
		return contract.NewError(contract.ErrRevisionConflict, "attempt_state", "atomic publish requires visible attempt")
	}
	eventRef := eventRef(request.Event)
	if *request.VisibleAttempt.Event != eventRef || request.Current.Event != eventRef || request.Current.Attempt != request.VisibleAttempt.Ref {
		return contract.NewError(contract.ErrProjectionConflict, "atomic_publish", "event, attempt and current bindings differ")
	}
	if request.Current.EvidenceProjectionRef != request.VisibleAttempt.EvidenceProjectionRef ||
		request.Current.EvidenceProjectionDigest != request.VisibleAttempt.EvidenceProjectionDigest ||
		request.Current.EvidenceCurrentIndexRef != request.VisibleAttempt.EvidenceCurrentIndexRef ||
		request.Current.EvidenceCurrentIndexDigest != request.VisibleAttempt.EvidenceCurrentIndexDigest ||
		request.Current.OwnerProjectionDigest != request.VisibleAttempt.OwnerProjectionDigest ||
		request.Current.PolicyProjectionDigest != request.VisibleAttempt.PolicyProjectionDigest ||
		request.Current.CheckedUnixNano != request.VisibleAttempt.CheckedUnixNano ||
		request.Current.NotAfterUnixNano != request.VisibleAttempt.NotAfterUnixNano {
		return contract.NewError(contract.ErrProjectionConflict, "atomic_publish", "current projection differs from admitted attempt")
	}
	return nil
}

func eventRef(event contract.TimelineEventRecord) contract.TimelineEventRefV1 {
	return contract.TimelineEventRefV1{
		EventID: event.Candidate.CandidateID, EvidenceRecordRef: event.EvidenceRecordRef,
		LedgerScopeDigest: event.LedgerScopeDigest, LedgerSequence: event.LedgerSequence,
		Digest: event.Candidate.Digest,
	}
}

func (s *Store) InspectTimelineProjectionCurrentV1(ctx context.Context, event contract.TimelineEventRefV1) (contract.TimelineProjectionCurrentV1, error) {
	if err := s.validateCall(ctx); err != nil {
		return contract.TimelineProjectionCurrentV1{}, err
	}
	if err := event.Validate(); err != nil {
		return contract.TimelineProjectionCurrentV1{}, err
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, "SELECT body FROM timeline_projection_current WHERE ledger_scope=? AND evidence_ref=?", event.LedgerScopeDigest, event.EvidenceRecordRef).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.TimelineProjectionCurrentV1{}, notFound("event_ref", "current projection not found")
		}
		return contract.TimelineProjectionCurrentV1{}, unavailable("inspect projection current", err)
	}
	var current contract.TimelineProjectionCurrentV1
	if err := decode(body, &current); err != nil {
		return contract.TimelineProjectionCurrentV1{}, err
	}
	if err := current.Validate(); err != nil {
		return contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrContentDigestMismatch, "projection_current", "stored current projection failed validation")
	}
	if current.Event != event {
		return contract.TimelineProjectionCurrentV1{}, contract.NewError(contract.ErrRevisionConflict, "event_ref", "current projection belongs to another event revision")
	}
	return current, nil
}

var _ ports.TimelineGovernanceRepositoryV1 = (*Store)(nil)
