package sqlite

import (
	"context"
	"database/sql"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

const reviewModelAssociationRowV1 = "ReviewModelInvocationAssociationFactV1"

func (s *Store) CreateReviewModelInvocationAssociationV1(ctx context.Context, value contract.ReviewModelInvocationAssociationFactV1) (hostports.ReviewModelInvocationAssociationCreateReceiptV1, error) {
	if err := s.writeReady(ctx); err != nil {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, err
	}
	baseline := s.clock()
	if baseline.IsZero() {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "association Create baseline clock is unavailable")
	}
	if err := value.ValidateCurrentV1(baseline); err != nil {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, err
	}
	if value.Revision != 1 || value.State != contract.ReviewModelInvocationAssociationActiveV1 {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, contract.NewError(contract.ErrorConflict, "association_create_invalid", "association create requires revision-one active")
	}
	payload, rowDigest, err := encodeRow(reviewModelAssociationRowV1, value)
	if err != nil {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, err
	}
	tx, err := s.beginMutation(ctx)
	if err != nil {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, err
	}
	defer tx.Rollback()
	actual, found, err := inspectCurrentAssociationTx(ctx, tx, value.ID)
	if err != nil {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, err
	}
	if found {
		if actual.Subject != value.Subject {
			return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, contract.NewError(contract.ErrorConflict, "association_subject_drift", "association current subject drifted")
		}
		existing, inspectErr := inspectHistoricalAssociationTx(ctx, tx, value.RefV1())
		if inspectErr != nil {
			return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, inspectErr
		}
		if reflect.DeepEqual(existing, value) {
			replayNow := s.clock()
			if replayNow.IsZero() || replayNow.Before(baseline) {
				return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "association Create replay clock regressed")
			}
			if err = value.ValidateCurrentV1(replayNow); err != nil {
				return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, err
			}
			replayCommitNow := s.clock()
			if replayCommitNow.IsZero() || replayCommitNow.Before(replayNow) {
				return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "association Create replay commit clock regressed")
			}
			if err = value.ValidateCurrentV1(replayCommitNow); err != nil {
				return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, err
			}
			if err = s.finishMutation(ctx, tx); err != nil {
				return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, err
			}
			return hostports.ReviewModelInvocationAssociationCreateReceiptV1{Fact: existing, Created: false}, nil
		}
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, contract.NewError(contract.ErrorConflict, "association_create_conflict", "association subject already binds different content")
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM agent_host_review_model_association_history_v1 WHERE id=?`, value.ID).Scan(&count); err != nil {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, mapDBError(ctx, err, false)
	}
	if count != 0 {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, contract.NewError(contract.ErrorConflict, "association_history_orphan", "association history exists without current")
	}
	actualNow := s.clock()
	if actualNow.IsZero() || actualNow.Before(baseline) {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "association Create clock regressed")
	}
	if err = value.ValidateCurrentV1(actualNow); err != nil {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_host_review_model_association_history_v1(id,revision,digest,previous_digest,checked_unix_nano,expires_unix_nano,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?,?)`, value.ID, uint64(value.Revision), string(value.Digest), "", value.CheckedUnixNano, value.ExpiresUnixNano, rowDigest, payload); err != nil {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, mapDBError(ctx, err, true)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_host_review_model_association_current_v1(id,revision,digest) VALUES(?,?,?)`, value.ID, uint64(value.Revision), string(value.Digest)); err != nil {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, mapDBError(ctx, err, true)
	}
	commitNow := s.clock()
	if commitNow.IsZero() || commitNow.Before(actualNow) {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "association Create commit clock regressed")
	}
	if err = value.ValidateCurrentV1(commitNow); err != nil {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, err
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return hostports.ReviewModelInvocationAssociationCreateReceiptV1{}, err
	}
	clone, _ := decodeAssociationRow(payload, rowDigest)
	return hostports.ReviewModelInvocationAssociationCreateReceiptV1{Fact: clone, Created: true}, nil
}

func (s *Store) ResolveCurrentReviewModelInvocationAssociationV1(ctx context.Context, subject contract.ReviewModelInvocationAssociationSubjectV1) (contract.ReviewModelInvocationAssociationRefV1, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.ReviewModelInvocationAssociationRefV1{}, err
	}
	id, err := subject.StableIDV1()
	if err != nil {
		return contract.ReviewModelInvocationAssociationRefV1{}, err
	}
	baseline := s.clock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return contract.ReviewModelInvocationAssociationRefV1{}, mapDBError(ctx, err, false)
	}
	defer tx.Rollback()
	value, found, err := inspectCurrentAssociationTx(ctx, tx, id)
	if err != nil {
		return contract.ReviewModelInvocationAssociationRefV1{}, err
	}
	if !found {
		return contract.ReviewModelInvocationAssociationRefV1{}, contract.NewError(contract.ErrorNotFound, "association_missing", "association current is absent")
	}
	if value.Subject != subject {
		return contract.ReviewModelInvocationAssociationRefV1{}, contract.NewError(contract.ErrorConflict, "association_subject_drift", "association current subject drifted")
	}
	if err = validateAssociationHighestTx(ctx, tx, id, uint64(value.Revision)); err != nil {
		return contract.ReviewModelInvocationAssociationRefV1{}, err
	}
	if err = tx.Commit(); err != nil {
		return contract.ReviewModelInvocationAssociationRefV1{}, mapDBError(ctx, err, false)
	}
	now := s.clock()
	if baseline.IsZero() || now.IsZero() || now.Before(baseline) {
		return contract.ReviewModelInvocationAssociationRefV1{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "association Resolve clock regressed")
	}
	if err = value.ValidateCurrentV1(now); err != nil {
		return contract.ReviewModelInvocationAssociationRefV1{}, err
	}
	return value.RefV1(), nil
}

func (s *Store) InspectCurrentReviewModelInvocationAssociationV1(ctx context.Context, subject contract.ReviewModelInvocationAssociationSubjectV1, expected contract.ReviewModelInvocationAssociationRefV1) (contract.ReviewModelInvocationAssociationFactV1, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, err
	}
	id, err := subject.StableIDV1()
	if err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, err
	}
	if id != expected.ID || expected.Subject != subject {
		return contract.ReviewModelInvocationAssociationFactV1{}, contract.NewError(contract.ErrorConflict, "association_subject_drift", "association expected subject drifted")
	}
	baseline := s.clock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, mapDBError(ctx, err, false)
	}
	defer tx.Rollback()
	value, found, err := inspectCurrentAssociationTx(ctx, tx, id)
	if err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, err
	}
	if !found {
		return contract.ReviewModelInvocationAssociationFactV1{}, contract.NewError(contract.ErrorNotFound, "association_missing", "association current is absent")
	}
	if value.RefV1() != expected {
		return contract.ReviewModelInvocationAssociationFactV1{}, contract.NewError(contract.ErrorConflict, "association_current_drift", "association current full Ref changed")
	}
	if err = validateAssociationHighestTx(ctx, tx, id, uint64(value.Revision)); err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, err
	}
	if err = tx.Commit(); err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, mapDBError(ctx, err, false)
	}
	now := s.clock()
	if baseline.IsZero() || now.IsZero() || now.Before(baseline) {
		return contract.ReviewModelInvocationAssociationFactV1{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "association Inspect clock regressed")
	}
	if err = value.ValidateCurrentV1(now); err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, err
	}
	return value, nil
}

func (s *Store) InspectHistoricalReviewModelInvocationAssociationV1(ctx context.Context, ref contract.ReviewModelInvocationAssociationRefV1) (contract.ReviewModelInvocationAssociationFactV1, error) {
	if err := s.readReady(ctx); err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, err
	}
	var payload []byte
	var rowDigest, previous string
	var checked, expires int64
	err := s.db.QueryRowContext(ctx, `SELECT canonical_json,row_digest,previous_digest,checked_unix_nano,expires_unix_nano FROM agent_host_review_model_association_history_v1 WHERE id=? AND revision=? AND digest=?`, ref.ID, uint64(ref.Revision), string(ref.Digest)).Scan(&payload, &rowDigest, &previous, &checked, &expires)
	if err == sql.ErrNoRows {
		return contract.ReviewModelInvocationAssociationFactV1{}, contract.NewError(contract.ErrorNotFound, "association_history_missing", "association historical Fact is absent")
	}
	if err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, mapDBError(ctx, err, false)
	}
	value, err := decodeAssociationRow(payload, rowDigest)
	if err != nil {
		return value, err
	}
	if value.RefV1() != ref || string(value.PreviousDigest) != previous || value.CheckedUnixNano != checked || value.ExpiresUnixNano != expires {
		return contract.ReviewModelInvocationAssociationFactV1{}, contract.NewError(contract.ErrorConflict, "association_history_drift", "association historical coordinates drifted")
	}
	return value, nil
}

func inspectHistoricalAssociationTx(ctx context.Context, tx *sql.Tx, ref contract.ReviewModelInvocationAssociationRefV1) (contract.ReviewModelInvocationAssociationFactV1, error) {
	var payload []byte
	var rowDigest, previous string
	var checked, expires int64
	err := tx.QueryRowContext(ctx, `SELECT canonical_json,row_digest,previous_digest,checked_unix_nano,expires_unix_nano FROM agent_host_review_model_association_history_v1 WHERE id=? AND revision=? AND digest=?`, ref.ID, uint64(ref.Revision), string(ref.Digest)).Scan(&payload, &rowDigest, &previous, &checked, &expires)
	if err == sql.ErrNoRows {
		return contract.ReviewModelInvocationAssociationFactV1{}, contract.NewError(contract.ErrorNotFound, "association_history_missing", "association historical Fact is absent")
	}
	if err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, mapDBError(ctx, err, false)
	}
	value, err := decodeAssociationRow(payload, rowDigest)
	if err != nil {
		return value, err
	}
	if value.RefV1() != ref || string(value.PreviousDigest) != previous || value.CheckedUnixNano != checked || value.ExpiresUnixNano != expires {
		return contract.ReviewModelInvocationAssociationFactV1{}, contract.NewError(contract.ErrorConflict, "association_history_drift", "association historical coordinates drifted")
	}
	return value, nil
}

func (s *Store) CompareAndSwapReviewModelInvocationAssociationV1(ctx context.Context, request hostports.ReviewModelInvocationAssociationCASRequestV1) (hostports.ReviewModelInvocationAssociationCASReceiptV1, error) {
	if err := s.writeReady(ctx); err != nil {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, err
	}
	payload, rowDigest, err := encodeRow(reviewModelAssociationRowV1, request.Next)
	if err != nil {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, err
	}
	tx, err := s.beginMutation(ctx)
	if err != nil {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, err
	}
	defer tx.Rollback()
	current, found, err := inspectCurrentAssociationTx(ctx, tx, request.Expected.ID)
	if err != nil {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, err
	}
	if !found {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, contract.NewError(contract.ErrorNotFound, "association_missing", "association current is absent")
	}
	if err = validateAssociationHighestTx(ctx, tx, current.ID, uint64(current.Revision)); err != nil {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, err
	}
	if current.RefV1() == request.Next.RefV1() && reflect.DeepEqual(current, request.Next) {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{Fact: current, Applied: false}, nil
	}
	if current.RefV1() != request.Expected {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, contract.NewError(contract.ErrorConflict, "association_cas_conflict", "association CAS predecessor changed")
	}
	if err = contract.ValidateReviewModelInvocationAssociationTransitionV1(current, request.Next); err != nil {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO agent_host_review_model_association_history_v1(id,revision,digest,previous_digest,checked_unix_nano,expires_unix_nano,row_digest,canonical_json) VALUES(?,?,?,?,?,?,?,?)`, request.Next.ID, uint64(request.Next.Revision), string(request.Next.Digest), string(request.Next.PreviousDigest), request.Next.CheckedUnixNano, request.Next.ExpiresUnixNano, rowDigest, payload); err != nil {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, mapDBError(ctx, err, true)
	}
	result, err := tx.ExecContext(ctx, `UPDATE agent_host_review_model_association_current_v1 SET revision=?,digest=? WHERE id=? AND revision=? AND digest=?`, uint64(request.Next.Revision), string(request.Next.Digest), request.Next.ID, uint64(request.Expected.Revision), string(request.Expected.Digest))
	if err != nil {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, mapDBError(ctx, err, true)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, contract.NewError(contract.ErrorConflict, "association_cas_lost", "association CAS lost")
	}
	if err = s.finishMutation(ctx, tx); err != nil {
		return hostports.ReviewModelInvocationAssociationCASReceiptV1{}, err
	}
	clone, _ := decodeAssociationRow(payload, rowDigest)
	return hostports.ReviewModelInvocationAssociationCASReceiptV1{Fact: clone, Applied: true}, nil
}

func inspectCurrentAssociationTx(ctx context.Context, tx *sql.Tx, id string) (contract.ReviewModelInvocationAssociationFactV1, bool, error) {
	var payload []byte
	var currentID, currentDigest, historyID, historyDigest, rowDigest string
	var currentRevision, historyRevision uint64
	err := tx.QueryRowContext(ctx, `SELECT id,revision,digest FROM agent_host_review_model_association_current_v1 WHERE id=?`, id).Scan(&currentID, &currentRevision, &currentDigest)
	if err == sql.ErrNoRows {
		return contract.ReviewModelInvocationAssociationFactV1{}, false, nil
	}
	if err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, false, mapDBError(ctx, err, false)
	}
	err = tx.QueryRowContext(ctx, `SELECT id,revision,digest,canonical_json,row_digest FROM agent_host_review_model_association_history_v1 WHERE id=? AND revision=? AND digest=?`, currentID, currentRevision, currentDigest).Scan(&historyID, &historyRevision, &historyDigest, &payload, &rowDigest)
	if err == sql.ErrNoRows {
		return contract.ReviewModelInvocationAssociationFactV1{}, false, contract.NewError(contract.ErrorConflict, "association_current_row_drift", "association current index points to missing history")
	}
	if err != nil {
		return contract.ReviewModelInvocationAssociationFactV1{}, false, mapDBError(ctx, err, false)
	}
	value, err := decodeAssociationRow(payload, rowDigest)
	if err != nil {
		return value, false, err
	}
	ref := value.RefV1()
	if currentID != id || historyID != currentID || historyRevision != currentRevision || historyDigest != currentDigest || ref.ID != currentID || uint64(ref.Revision) != currentRevision || string(ref.Digest) != currentDigest {
		return contract.ReviewModelInvocationAssociationFactV1{}, false, contract.NewError(contract.ErrorConflict, "association_current_row_drift", "association current/history row and payload coordinates drifted")
	}
	return value, true, nil
}
func validateAssociationHighestTx(ctx context.Context, tx *sql.Tx, id string, revision uint64) error {
	var highest uint64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(revision) FROM agent_host_review_model_association_history_v1 WHERE id=?`, id).Scan(&highest); err != nil {
		return mapDBError(ctx, err, false)
	}
	if highest != revision {
		return contract.NewError(contract.ErrorConflict, "association_current_regressed", "association current index regressed behind history")
	}
	return nil
}
func decodeAssociationRow(payload []byte, rowDigest string) (contract.ReviewModelInvocationAssociationFactV1, error) {
	value, err := decodeRow[contract.ReviewModelInvocationAssociationFactV1](payload, rowDigest, reviewModelAssociationRowV1)
	if err != nil {
		return value, err
	}
	if err = value.ValidateHistoricalV1(); err != nil {
		return value, contract.NewError(contract.ErrorConflict, "association_row_invalid", "association row validation failed")
	}
	return value, nil
}

var _ hostports.ReviewModelInvocationAssociationPortV1 = (*Store)(nil)
var _ = time.Time{}
