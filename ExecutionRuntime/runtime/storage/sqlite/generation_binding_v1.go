package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func loadGenerationBindingAssociationHistoryV1(ctx context.Context, source queryRower, id string, revision core.Revision) (ports.GenerationBindingAssociationFactV1, error) {
	var factDigest, rowDigest string
	var payload []byte
	err := source.QueryRowContext(ctx, `SELECT fact_digest,row_digest,canonical_json FROM runtime_generation_binding_association_history WHERE association_id=? AND revision=?`, id, revision).Scan(&factDigest, &rowDigest, &payload)
	if err == sql.ErrNoRows {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Generation Binding association history is absent")
	}
	if err != nil {
		return ports.GenerationBindingAssociationFactV1{}, mapDBError(ctx, err, false)
	}
	fact, err := decodeRow[ports.GenerationBindingAssociationFactV1](payload, rowDigest, "GenerationBindingAssociationFactV1")
	if err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if fact.ID != id || fact.Revision != revision || string(fact.Digest) != factDigest {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Generation Binding association history coordinates drifted")
	}
	if err := fact.Validate(); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	return fact, nil
}

func loadGenerationBindingAssociationCurrentV1(ctx context.Context, source queryRower, id string) (ports.GenerationBindingAssociationFactV1, core.Revision, error) {
	var revision, highest uint64
	var digest string
	err := source.QueryRowContext(ctx, `SELECT revision,fact_digest,highest_revision FROM runtime_generation_binding_association_current WHERE association_id=?`, id).Scan(&revision, &digest, &highest)
	if err == sql.ErrNoRows {
		return ports.GenerationBindingAssociationFactV1{}, 0, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Generation Binding association is absent")
	}
	if err != nil {
		return ports.GenerationBindingAssociationFactV1{}, 0, mapDBError(ctx, err, false)
	}
	if highest < revision {
		return ports.GenerationBindingAssociationFactV1{}, 0, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Generation Binding association highest revision regressed")
	}
	fact, err := loadGenerationBindingAssociationHistoryV1(ctx, source, id, core.Revision(revision))
	if err != nil {
		return ports.GenerationBindingAssociationFactV1{}, 0, err
	}
	if string(fact.Digest) != digest {
		return ports.GenerationBindingAssociationFactV1{}, 0, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Generation Binding association current index drifted")
	}
	return fact, core.Revision(highest), nil
}

func insertGenerationBindingAssociationHistoryV1(ctx context.Context, tx *sql.Tx, fact ports.GenerationBindingAssociationFactV1) error {
	payload, err := marshalStrict(fact)
	if err != nil {
		return err
	}
	rowDigest, err := digestRow("GenerationBindingAssociationFactV1", fact)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_generation_binding_association_history(association_id,revision,fact_digest,row_digest,canonical_json) VALUES(?,?,?,?,?)`, fact.ID, fact.Revision, string(fact.Digest), string(rowDigest), payload)
	return mapDBError(ctx, err, true)
}

func insertGenerationBindingAssociationCurrentV1(ctx context.Context, tx *sql.Tx, fact ports.GenerationBindingAssociationFactV1) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO runtime_generation_binding_association_current(association_id,revision,fact_digest,highest_revision) VALUES(?,?,?,?)`, fact.ID, fact.Revision, string(fact.Digest), fact.Revision)
	return mapDBError(ctx, err, true)
}

func advanceGenerationBindingAssociationCurrentV1(ctx context.Context, tx *sql.Tx, expected core.Revision, next ports.GenerationBindingAssociationFactV1) error {
	result, err := tx.ExecContext(ctx, `UPDATE runtime_generation_binding_association_current SET revision=?,fact_digest=?,highest_revision=? WHERE association_id=? AND revision=? AND highest_revision=?`, next.Revision, string(next.Digest), next.Revision, next.ID, expected, expected)
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapDBError(ctx, err, true)
	}
	if affected != 1 {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Generation Binding association CAS lost current/highest precondition")
	}
	return nil
}

func (s *Store) CreateGenerationBindingAssociationV1(ctx context.Context, fact ports.GenerationBindingAssociationFactV1) (ports.GenerationBindingAssociationFactV1, error) {
	if err := contextError(ctx, "Create Generation Binding association"); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if s == nil || s.db == nil {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "runtime Binding sqlite is unavailable")
	}
	staged, err := cloneStrict(fact)
	if err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if err := staged.Validate(); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	now := s.clock()
	if staged.Revision != 1 || staged.State != ports.GenerationBindingAssociationActiveV1 || now.IsZero() || staged.CreatedUnixNano > now.UnixNano() || staged.UpdatedUnixNano != staged.CreatedUnixNano || !now.Before(time.Unix(0, staged.ExpiresUnixNano)) {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Generation Binding association create time is invalid or expired")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, _, err := loadGenerationBindingAssociationCurrentV1(ctx, tx, staged.ID); err == nil {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonAlreadyExists, "Generation Binding association already exists; Inspect is required")
	} else if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if s.consumeStageFailure() {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Generation Binding association staged failure")
	}
	if err := insertGenerationBindingAssociationHistoryV1(ctx, tx, staged); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if err := insertGenerationBindingAssociationCurrentV1(ctx, tx, staged); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if s.consumeLostReply() {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Generation Binding association Create reply loss")
	}
	return cloneStrict(staged)
}

func (s *Store) InspectGenerationBindingAssociationV1(ctx context.Context, id string) (ports.GenerationBindingAssociationFactV1, error) {
	if err := contextError(ctx, "Inspect Generation Binding association"); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if s == nil || s.db == nil || id == "" {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Generation Binding association lookup is invalid")
	}
	fact, _, err := loadGenerationBindingAssociationCurrentV1(ctx, s.db, id)
	if err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	return cloneStrict(fact)
}

func (s *Store) CompareAndSwapGenerationBindingAssociationV1(ctx context.Context, request ports.GenerationBindingAssociationCASRequestV1) (ports.GenerationBindingAssociationFactV1, error) {
	if err := contextError(ctx, "CAS Generation Binding association"); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if s == nil || s.db == nil || request.ExpectedRevision == 0 || request.Next.Revision != request.ExpectedRevision+1 {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "Generation Binding association CAS precondition is incomplete")
	}
	next, err := cloneStrict(request.Next)
	if err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if err := next.Validate(); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	now := s.clock()
	if now.IsZero() {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Generation Binding association CAS clock is invalid")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, highest, err := loadGenerationBindingAssociationCurrentV1(ctx, tx, next.ID)
	if err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if current.Revision != request.ExpectedRevision || highest != current.Revision {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Generation Binding association CAS expected/current/highest revision drifted")
	}
	if err := ports.ValidateGenerationBindingAssociationTransitionV1(current, next, now); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if s.consumeStageFailure() {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected Generation Binding association CAS staged failure")
	}
	if err := insertGenerationBindingAssociationHistoryV1(ctx, tx, next); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if err := advanceGenerationBindingAssociationCurrentV1(ctx, tx, request.ExpectedRevision, next); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if err := commit(ctx, tx); err != nil {
		return ports.GenerationBindingAssociationFactV1{}, err
	}
	if s.consumeLostReply() {
		return ports.GenerationBindingAssociationFactV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected Generation Binding association CAS reply loss")
	}
	return cloneStrict(next)
}

var _ ports.GenerationBindingAssociationFactPortV1 = (*Store)(nil)
