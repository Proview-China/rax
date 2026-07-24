package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	operationReviewAuthorizationVersionV4 = 4
	operationReviewAuthorizationVersionV5 = 5
)

type operationReviewAuthorizationCurrentRow struct {
	tenant   core.TenantID
	revision core.Revision
	digest   core.Digest
	highest  core.Revision
}

func loadOperationReviewAuthorizationCurrentRow(ctx context.Context, source queryRower, version int, id string) (operationReviewAuthorizationCurrentRow, error) {
	var tenant, digest string
	var revision, highest, historyHighest uint64
	err := source.QueryRowContext(ctx, `SELECT c.tenant_id,c.revision,c.fact_digest,c.highest_revision,
  (SELECT COALESCE(MAX(h.revision),0) FROM runtime_operation_review_authorization_history h
   WHERE h.contract_version=c.contract_version AND h.authorization_id=c.authorization_id)
FROM runtime_operation_review_authorization_current c
WHERE c.contract_version=? AND c.authorization_id=?`, version, id).Scan(&tenant, &revision, &digest, &highest, &historyHighest)
	if errors.Is(err, sql.ErrNoRows) {
		return operationReviewAuthorizationCurrentRow{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "Operation Review Authorization current index is absent")
	}
	if err != nil {
		return operationReviewAuthorizationCurrentRow{}, mapDBError(ctx, err, false)
	}
	if revision == 0 || highest == 0 || historyHighest == 0 || revision != highest || highest != historyHighest || digest == "" || tenant == "" {
		return operationReviewAuthorizationCurrentRow{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Operation Review Authorization current/highest index drifted")
	}
	return operationReviewAuthorizationCurrentRow{tenant: core.TenantID(tenant), revision: core.Revision(revision), digest: core.Digest(digest), highest: core.Revision(highest)}, nil
}

func loadOperationReviewAuthorizationV4History(ctx context.Context, source queryRower, id string, revision core.Revision) (ports.OperationReviewAuthorizationFactV4, error) {
	var tenant, factDigest, operationDigest, effectID, state, rowDigest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT tenant_id,fact_digest,operation_digest,effect_id,state,row_digest,canonical_json FROM runtime_operation_review_authorization_history WHERE contract_version=? AND authorization_id=? AND revision=?`, operationReviewAuthorizationVersionV4, id, revision).Scan(&tenant, &factDigest, &operationDigest, &effectID, &state, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "Operation Review Authorization V4 history is absent")
	}
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, mapDBError(ctx, err, false)
	}
	fact, err := decodeRow[ports.OperationReviewAuthorizationFactV4](payload, rowDigest, "OperationReviewAuthorizationFactV4")
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	gotOperation, operationErr := fact.Intent.Operation.DigestV3()
	if operationErr != nil || fact.Validate() != nil || fact.ID != id || fact.Revision != revision || fact.Digest != core.Digest(factDigest) || string(fact.State) != state || fact.Intent.Operation.ExecutionScope.Identity.TenantID != core.TenantID(tenant) || gotOperation != core.Digest(operationDigest) || fact.Intent.IntentID != core.EffectIntentID(effectID) {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Operation Review Authorization V4 history drifted")
	}
	return cloneStrict(fact)
}

func loadOperationReviewAuthorizationV5History(ctx context.Context, source queryRower, id string, revision core.Revision) (ports.OperationReviewAuthorizationFactV5, error) {
	var tenant, factDigest, operationDigest, effectID, state, rowDigest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT tenant_id,fact_digest,operation_digest,effect_id,state,row_digest,canonical_json FROM runtime_operation_review_authorization_history WHERE contract_version=? AND authorization_id=? AND revision=?`, operationReviewAuthorizationVersionV5, id, revision).Scan(&tenant, &factDigest, &operationDigest, &effectID, &state, &rowDigest, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "Operation Review Authorization V5 history is absent")
	}
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, mapDBError(ctx, err, false)
	}
	fact, err := decodeRow[ports.OperationReviewAuthorizationFactV5](payload, rowDigest, "OperationReviewAuthorizationFactV5")
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	gotOperation, operationErr := fact.Intent.Operation.DigestV3()
	if operationErr != nil || fact.Validate() != nil || fact.ID != id || fact.Revision != revision || fact.Digest != core.Digest(factDigest) || string(fact.State) != state || fact.Intent.Operation.ExecutionScope.Identity.TenantID != core.TenantID(tenant) || gotOperation != core.Digest(operationDigest) || fact.Intent.IntentID != core.EffectIntentID(effectID) {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Operation Review Authorization V5 history drifted")
	}
	return cloneStrict(fact)
}

func insertOperationReviewAuthorizationHistory(ctx context.Context, tx *sql.Tx, version int, tenant core.TenantID, id string, revision core.Revision, factDigest, operationDigest core.Digest, effectID core.EffectIntentID, state string, discriminator string, value any) error {
	payload, err := marshalStrict(value)
	if err != nil {
		return err
	}
	rowDigest, err := digestRow(discriminator, value)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_operation_review_authorization_history(contract_version,tenant_id,authorization_id,revision,fact_digest,operation_digest,effect_id,state,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?,?,?,?)`, version, string(tenant), id, revision, string(factDigest), string(operationDigest), string(effectID), state, string(rowDigest), payload)
	return mapDBError(ctx, err, true)
}

func insertOperationReviewAuthorizationCurrent(ctx context.Context, tx *sql.Tx, version int, tenant core.TenantID, id string, revision core.Revision, digest core.Digest) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO runtime_operation_review_authorization_current(contract_version,tenant_id,authorization_id,revision,fact_digest,highest_revision) VALUES(?,?,?,?,?,?)`, version, string(tenant), id, revision, string(digest), revision)
	return mapDBError(ctx, err, true)
}

func advanceOperationReviewAuthorizationCurrent(ctx context.Context, tx *sql.Tx, version int, expected operationReviewAuthorizationCurrentRow, id string, nextRevision core.Revision, nextDigest core.Digest) error {
	result, err := tx.ExecContext(ctx, `UPDATE runtime_operation_review_authorization_current SET revision=?,fact_digest=?,highest_revision=? WHERE contract_version=? AND authorization_id=? AND tenant_id=? AND revision=? AND fact_digest=? AND highest_revision=?`, nextRevision, string(nextDigest), nextRevision, version, id, string(expected.tenant), expected.revision, string(expected.digest), expected.highest)
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Operation Review Authorization CAS lost its current/highest precondition")
	}
	return nil
}

func insertOperationReviewAuthorizationGuard(ctx context.Context, tx *sql.Tx, version int, tenant core.TenantID, operationDigest core.Digest, effectID core.EffectIntentID, id string, revision core.Revision, digest core.Digest) error {
	var occupiedVersion int
	var occupiedID string
	err := tx.QueryRowContext(ctx, `SELECT contract_version,authorization_id FROM runtime_operation_review_authorization_active_guard WHERE tenant_id=? AND operation_digest=? AND effect_id=?`, string(tenant), string(operationDigest), string(effectID)).Scan(&occupiedVersion, &occupiedID)
	if err == nil {
		return core.NewError(core.ErrorConflict, core.ReasonEffectConflictDomainOccupied, "another V4/V5 Review Authorization is current for this Operation Effect")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return mapDBError(ctx, err, false)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_operation_review_authorization_active_guard(tenant_id,operation_digest,effect_id,contract_version,authorization_id,authorization_revision,authorization_digest) VALUES(?,?,?,?,?,?,?)`, string(tenant), string(operationDigest), string(effectID), version, id, revision, string(digest))
	return mapDBError(ctx, err, true)
}

func deleteOperationReviewAuthorizationGuard(ctx context.Context, tx *sql.Tx, version int, tenant core.TenantID, operationDigest core.Digest, effectID core.EffectIntentID, id string) error {
	result, err := tx.ExecContext(ctx, `DELETE FROM runtime_operation_review_authorization_active_guard WHERE tenant_id=? AND operation_digest=? AND effect_id=? AND contract_version=? AND authorization_id=?`, string(tenant), string(operationDigest), string(effectID), version, id)
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Operation Review Authorization active guard drifted")
	}
	return nil
}

func (s *Store) CreateOperationReviewAuthorizationV4(ctx context.Context, fact ports.OperationReviewAuthorizationFactV4) (ports.OperationReviewAuthorizationFactV4, error) {
	if err := fact.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if fact.Revision != 1 || fact.State != ports.OperationReviewAuthorizationActiveV4 {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "new Review Authorization V4 must be active revision one")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := loadOperationReviewAuthorizationCurrentRow(ctx, tx, operationReviewAuthorizationVersionV4, fact.ID)
	if err == nil {
		stored, loadErr := loadOperationReviewAuthorizationV4History(ctx, tx, fact.ID, current.revision)
		if loadErr != nil {
			return ports.OperationReviewAuthorizationFactV4{}, loadErr
		}
		if stored.Digest != fact.Digest {
			return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review Authorization V4 ID contains different content")
		}
		return stored, nil
	}
	if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	now := s.clock()
	if now.IsZero() || fact.CreatedUnixNano > now.UnixNano() || fact.UpdatedUnixNano != fact.CreatedUnixNano || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "new Review Authorization V4 time is inconsistent")
	}
	operationDigest, err := fact.Intent.Operation.DigestV3()
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	staged, err := cloneStrict(fact)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if s.consumeStageFailure() {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review Authorization V4 staged failure")
	}
	tenant := fact.Intent.Operation.ExecutionScope.Identity.TenantID
	if err := insertOperationReviewAuthorizationHistory(ctx, tx, operationReviewAuthorizationVersionV4, tenant, fact.ID, fact.Revision, fact.Digest, operationDigest, fact.Intent.IntentID, string(fact.State), "OperationReviewAuthorizationFactV4", staged); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if err := insertOperationReviewAuthorizationCurrent(ctx, tx, operationReviewAuthorizationVersionV4, tenant, fact.ID, fact.Revision, fact.Digest); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if err := insertOperationReviewAuthorizationGuard(ctx, tx, operationReviewAuthorizationVersionV4, tenant, operationDigest, fact.Intent.IntentID, fact.ID, fact.Revision, fact.Digest); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if s.consumeLostReply() {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Review Authorization V4 Create reply loss")
	}
	return cloneStrict(staged)
}

func (s *Store) InspectOperationReviewAuthorizationV4(ctx context.Context, id string) (ports.OperationReviewAuthorizationFactV4, error) {
	if err := contextError(ctx, "Review Authorization V4 Inspect"); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	current, err := loadOperationReviewAuthorizationCurrentRow(ctx, s.db, operationReviewAuthorizationVersionV4, id)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if current.highest != current.revision {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Authorization V4 current/highest index drifted")
	}
	fact, err := loadOperationReviewAuthorizationV4History(ctx, s.db, id, current.revision)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if fact.Digest != current.digest || fact.Intent.Operation.ExecutionScope.Identity.TenantID != current.tenant {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review Authorization V4 current index drifted")
	}
	return fact, nil
}

func (s *Store) CompareAndSwapOperationReviewAuthorizationV4(ctx context.Context, request ports.OperationReviewAuthorizationCASRequestV4) (ports.OperationReviewAuthorizationFactV4, error) {
	if request.ExpectedRevision == 0 {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "Review Authorization V4 CAS requires expected revision")
	}
	if err := request.Next.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	defer func() { _ = tx.Rollback() }()
	currentRow, err := loadOperationReviewAuthorizationCurrentRow(ctx, tx, operationReviewAuthorizationVersionV4, request.Next.ID)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	current, err := loadOperationReviewAuthorizationV4History(ctx, tx, request.Next.ID, currentRow.revision)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if current.Digest == request.Next.Digest {
		return current, nil
	}
	if currentRow.highest != current.Revision || current.Revision != request.ExpectedRevision {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Authorization V4 CAS current/highest revision changed")
	}
	now := s.clock()
	if err := ports.ValidateOperationReviewAuthorizationTransitionV4(current, request.Next, now); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	operationDigest, err := current.Intent.Operation.DigestV3()
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	staged, err := cloneStrict(request.Next)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if s.consumeStageFailure() {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review Authorization V4 CAS staged failure")
	}
	if err := insertOperationReviewAuthorizationHistory(ctx, tx, operationReviewAuthorizationVersionV4, currentRow.tenant, staged.ID, staged.Revision, staged.Digest, operationDigest, staged.Intent.IntentID, string(staged.State), "OperationReviewAuthorizationFactV4", staged); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if err := advanceOperationReviewAuthorizationCurrent(ctx, tx, operationReviewAuthorizationVersionV4, currentRow, staged.ID, staged.Revision, staged.Digest); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if err := deleteOperationReviewAuthorizationGuard(ctx, tx, operationReviewAuthorizationVersionV4, currentRow.tenant, operationDigest, staged.Intent.IntentID, staged.ID); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.OperationReviewAuthorizationFactV4{}, err
	}
	if s.consumeLostReply() {
		return ports.OperationReviewAuthorizationFactV4{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Review Authorization V4 CAS reply loss")
	}
	return cloneStrict(staged)
}

func (s *Store) CreateOperationReviewAuthorizationV5(ctx context.Context, fact ports.OperationReviewAuthorizationFactV5) (ports.OperationReviewAuthorizationFactV5, error) {
	if err := fact.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if fact.Revision != 1 || fact.State != ports.OperationReviewAuthorizationActiveV5 {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidTransition, "new Review Authorization V5 must be active revision one")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := loadOperationReviewAuthorizationCurrentRow(ctx, tx, operationReviewAuthorizationVersionV5, fact.ID)
	if err == nil {
		stored, loadErr := loadOperationReviewAuthorizationV5History(ctx, tx, fact.ID, current.revision)
		if loadErr != nil {
			return ports.OperationReviewAuthorizationFactV5{}, loadErr
		}
		if stored.Digest != fact.Digest {
			return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review Authorization V5 ID contains different content")
		}
		return stored, nil
	}
	if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	now := s.clock()
	if now.IsZero() || fact.CreatedUnixNano > now.UnixNano() || fact.UpdatedUnixNano != fact.CreatedUnixNano || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "new Review Authorization V5 time is inconsistent")
	}
	operationDigest, err := fact.Intent.Operation.DigestV3()
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	staged, err := cloneStrict(fact)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if s.consumeStageFailure() {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review Authorization V5 staged failure")
	}
	tenant := fact.Intent.Operation.ExecutionScope.Identity.TenantID
	if err := insertOperationReviewAuthorizationHistory(ctx, tx, operationReviewAuthorizationVersionV5, tenant, fact.ID, fact.Revision, fact.Digest, operationDigest, fact.Intent.IntentID, string(fact.State), "OperationReviewAuthorizationFactV5", staged); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if err := insertOperationReviewAuthorizationCurrent(ctx, tx, operationReviewAuthorizationVersionV5, tenant, fact.ID, fact.Revision, fact.Digest); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if err := insertOperationReviewAuthorizationGuard(ctx, tx, operationReviewAuthorizationVersionV5, tenant, operationDigest, fact.Intent.IntentID, fact.ID, fact.Revision, fact.Digest); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if s.consumeLostReply() {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Review Authorization V5 Create reply loss")
	}
	return cloneStrict(staged)
}

func (s *Store) InspectOperationReviewAuthorizationV5(ctx context.Context, id string) (ports.OperationReviewAuthorizationFactV5, error) {
	if err := contextError(ctx, "Review Authorization V5 Inspect"); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	current, err := loadOperationReviewAuthorizationCurrentRow(ctx, s.db, operationReviewAuthorizationVersionV5, id)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if current.highest != current.revision {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Authorization V5 current/highest index drifted")
	}
	fact, err := loadOperationReviewAuthorizationV5History(ctx, s.db, id, current.revision)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if fact.Digest != current.digest || fact.Intent.Operation.ExecutionScope.Identity.TenantID != current.tenant {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review Authorization V5 current index drifted")
	}
	return fact, nil
}

func (s *Store) InspectOperationReviewAuthorizationExactV5(ctx context.Context, ref ports.OperationReviewAuthorizationRefV5) (ports.OperationReviewAuthorizationFactV5, error) {
	if err := contextError(ctx, "Review Authorization V5 exact Inspect"); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	fact, err := loadOperationReviewAuthorizationV5History(ctx, s.db, ref.ID, ref.Revision)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if fact.Digest != ref.Digest {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Review Authorization V5 exact ref digest drifted")
	}
	return fact, nil
}

func (s *Store) CompareAndSwapOperationReviewAuthorizationV5(ctx context.Context, request ports.OperationReviewAuthorizationCASRequestV5) (ports.OperationReviewAuthorizationFactV5, error) {
	if request.ExpectedRevision == 0 {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "Review Authorization V5 CAS requires expected revision")
	}
	if err := request.Next.Validate(); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	defer func() { _ = tx.Rollback() }()
	currentRow, err := loadOperationReviewAuthorizationCurrentRow(ctx, tx, operationReviewAuthorizationVersionV5, request.Next.ID)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	current, err := loadOperationReviewAuthorizationV5History(ctx, tx, request.Next.ID, currentRow.revision)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if current.Digest == request.Next.Digest {
		return current, nil
	}
	if currentRow.highest != current.Revision || current.Revision != request.ExpectedRevision {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Authorization V5 CAS current/highest revision changed")
	}
	now := s.clock()
	if err := ports.ValidateOperationReviewAuthorizationTransitionV5(current, request.Next, now); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	operationDigest, err := current.Intent.Operation.DigestV3()
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	staged, err := cloneStrict(request.Next)
	if err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if s.consumeStageFailure() {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Review Authorization V5 CAS staged failure")
	}
	if err := insertOperationReviewAuthorizationHistory(ctx, tx, operationReviewAuthorizationVersionV5, currentRow.tenant, staged.ID, staged.Revision, staged.Digest, operationDigest, staged.Intent.IntentID, string(staged.State), "OperationReviewAuthorizationFactV5", staged); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if err := advanceOperationReviewAuthorizationCurrent(ctx, tx, operationReviewAuthorizationVersionV5, currentRow, staged.ID, staged.Revision, staged.Digest); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if err := deleteOperationReviewAuthorizationGuard(ctx, tx, operationReviewAuthorizationVersionV5, currentRow.tenant, operationDigest, staged.Intent.IntentID, staged.ID); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.OperationReviewAuthorizationFactV5{}, err
	}
	if s.consumeLostReply() {
		return ports.OperationReviewAuthorizationFactV5{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Review Authorization V5 CAS reply loss")
	}
	return cloneStrict(staged)
}
