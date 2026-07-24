package sqlite

import (
	"context"
	"database/sql"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func loadBindingAdmissionAttemptV1(ctx context.Context, source queryRower, attemptID string) (control.BindingAdmissionAttemptFactV1, error) {
	var revision uint64
	var digest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT revision,digest,canonical_json FROM runtime_binding_admission_attempts WHERE attempt_id=?`, attemptID).Scan(&revision, &digest, &payload)
	if err == sql.ErrNoRows {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Binding admission Attempt does not exist")
	}
	if err != nil {
		return control.BindingAdmissionAttemptFactV1{}, mapDBError(ctx, err, false)
	}
	fact, err := decodeRow[control.BindingAdmissionAttemptFactV1](payload, digest, "BindingAdmissionAttemptFactV1")
	if err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if fact.AttemptID != attemptID || fact.Revision != core.Revision(revision) || string(fact.Digest) == "" {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Binding admission Attempt row coordinates drifted")
	}
	if err := fact.Validate(); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	return fact, nil
}

func insertBindingAdmissionAttemptV1(ctx context.Context, tx *sql.Tx, fact control.BindingAdmissionAttemptFactV1) error {
	payload, err := marshalStrict(fact)
	if err != nil {
		return err
	}
	rowDigest, err := digestRow("BindingAdmissionAttemptFactV1", fact)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_binding_admission_attempts(attempt_id,revision,digest,canonical_json) VALUES(?,?,?,?)`, fact.AttemptID, fact.Revision, string(rowDigest), payload)
	return mapDBError(ctx, err, true)
}

func updateBindingAdmissionAttemptV1(ctx context.Context, tx *sql.Tx, expectedRevision core.Revision, expectedDigest core.Digest, next control.BindingAdmissionAttemptFactV1) error {
	payload, err := marshalStrict(next)
	if err != nil {
		return err
	}
	rowDigest, err := digestRow("BindingAdmissionAttemptFactV1", next)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE runtime_binding_admission_attempts SET revision=?,digest=?,canonical_json=? WHERE attempt_id=? AND revision=? AND json_extract(canonical_json,'$.digest')=?`, next.Revision, string(rowDigest), payload, next.AttemptID, expectedRevision, string(expectedDigest))
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Binding admission Attempt CAS lost its exact precondition")
	}
	return nil
}

// CreateBindingAdmissionAttemptV1 creates the durable execution token. An
// existing Attempt always returns Conflict, even when its content is equal;
// recovery must Inspect and never infer execution ownership from replay.
func (s *Store) CreateBindingAdmissionAttemptV1(ctx context.Context, fact control.BindingAdmissionAttemptFactV1) (control.BindingAdmissionAttemptFactV1, error) {
	if err := contextError(ctx, "Create Binding admission Attempt"); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if s == nil || s.db == nil {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "runtime Binding sqlite is unavailable")
	}
	staged, err := cloneStrict(fact)
	if err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if err := staged.Validate(); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if staged.Revision != 1 || staged.State != control.BindingAdmissionIntentRecordedV1 {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Binding admission Attempt must begin at intent_recorded revision one")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := loadBindingAdmissionAttemptV1(ctx, tx, staged.AttemptID); err == nil {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "Binding admission Attempt already exists; Inspect is required")
	} else if !core.HasCategory(err, core.ErrorNotFound) {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if s.consumeStageFailure() {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Binding admission Attempt staged failure")
	}
	if err := insertBindingAdmissionAttemptV1(ctx, tx, staged); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if s.consumeLostReply() {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Binding admission Attempt Create reply loss")
	}
	return cloneStrict(staged)
}

func (s *Store) InspectBindingAdmissionAttemptV1(ctx context.Context, attemptID string) (control.BindingAdmissionAttemptFactV1, error) {
	if err := contextError(ctx, "Inspect Binding admission Attempt"); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if s == nil || s.db == nil || attemptID == "" {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Binding admission Attempt lookup is invalid")
	}
	fact, err := loadBindingAdmissionAttemptV1(ctx, s.db, attemptID)
	if err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	return cloneStrict(fact)
}

func (s *Store) CompareAndSwapBindingAdmissionAttemptV1(ctx context.Context, request control.BindingAdmissionAttemptCASRequestV1) (control.BindingAdmissionAttemptFactV1, error) {
	if err := contextError(ctx, "CAS Binding admission Attempt"); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if s == nil || s.db == nil {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "runtime Binding sqlite is unavailable")
	}
	if request.ExpectedRevision == 0 || request.ExpectedDigest.Validate() != nil || request.Next.Revision != request.ExpectedRevision+1 {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "Binding admission Attempt CAS precondition is incomplete")
	}
	next, err := cloneStrict(request.Next)
	if err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if err := next.Validate(); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := loadBindingAdmissionAttemptV1(ctx, tx, next.AttemptID)
	if err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if current.Revision != request.ExpectedRevision || current.Digest != request.ExpectedDigest {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Binding admission Attempt CAS expected revision or digest drifted")
	}
	if err := control.ValidateBindingAdmissionAttemptSuccessorV1(current, next); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if s.consumeStageFailure() {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Binding admission Attempt CAS staged failure")
	}
	if err := updateBindingAdmissionAttemptV1(ctx, tx, request.ExpectedRevision, request.ExpectedDigest, next); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return control.BindingAdmissionAttemptFactV1{}, err
	}
	if s.consumeLostReply() {
		return control.BindingAdmissionAttemptFactV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Binding admission Attempt CAS reply loss")
	}
	return cloneStrict(next)
}

var _ control.BindingAdmissionAttemptFactPortV1 = (*Store)(nil)
