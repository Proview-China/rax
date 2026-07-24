package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func (s *Store) CreateCheckpointPhaseDomainResultV2(ctx context.Context, value contract.CheckpointPhaseDomainResultV2) (bool, error) {
	if value.ValidateShape() != nil {
		return false, ports.ErrConflict
	}
	body, err := encode(value)
	if err != nil {
		return false, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO checkpoint_phase_domain_result_history(result_id,revision,digest,reservation_id,reservation_revision,reservation_digest,body) VALUES(?,?,?,?,?,?,?)`, value.Meta.ID, value.Meta.Revision, value.Meta.Digest, value.ReservationRef.ID, value.ReservationRef.Revision, value.ReservationRef.Digest, body)
	if err != nil {
		return false, classifyWrite(err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 1 {
		return true, nil
	}
	existing, inspectErr := s.InspectCheckpointPhaseDomainResultByReservationV2(ctx, value.ReservationRef)
	if inspectErr != nil || existing.ExactRef() != value.ExactRef() {
		return false, ports.ErrConflict
	}
	return false, nil
}

func (s *Store) InspectCheckpointPhaseDomainResultV2(ctx context.Context, expected contract.SnapshotArtifactExactRefV2) (contract.CheckpointPhaseDomainResultV2, error) {
	if expected.ValidateShape("checkpoint phase DomainResult") != nil || expected.TypeURL != contract.CheckpointPhaseDomainResultTypeURLV2 {
		return contract.CheckpointPhaseDomainResultV2{}, ports.ErrConflict
	}
	return readCheckpointPhaseDomainResultV2(ctx, s.db, `result_id=? AND revision=? AND digest=?`, expected.ID, expected.Revision, expected.Digest)
}

func (s *Store) InspectCheckpointPhaseDomainResultByRefV2(ctx context.Context, expected contract.Ref) (contract.CheckpointPhaseDomainResultV2, error) {
	if expected.ValidateShape("checkpoint phase DomainResult") != nil {
		return contract.CheckpointPhaseDomainResultV2{}, ports.ErrConflict
	}
	return readCheckpointPhaseDomainResultV2(ctx, s.db, `result_id=? AND revision=? AND digest=?`, expected.ID, expected.Revision, expected.Digest)
}

func (s *Store) InspectCheckpointPhaseDomainResultByIDV2(ctx context.Context, id string) (contract.CheckpointPhaseDomainResultV2, error) {
	if id == "" {
		return contract.CheckpointPhaseDomainResultV2{}, ports.ErrConflict
	}
	return readCheckpointPhaseDomainResultV2(ctx, s.db, `result_id=?`, id)
}

func (s *Store) InspectCheckpointPhaseDomainResultByReservationV2(ctx context.Context, reservation contract.Ref) (contract.CheckpointPhaseDomainResultV2, error) {
	if reservation.ValidateShape("checkpoint DomainResult reservation") != nil {
		return contract.CheckpointPhaseDomainResultV2{}, ports.ErrConflict
	}
	return readCheckpointPhaseDomainResultV2(ctx, s.db, `reservation_id=? AND reservation_revision=? AND reservation_digest=?`, reservation.ID, reservation.Revision, reservation.Digest)
}

func readCheckpointPhaseDomainResultV2(ctx context.Context, q queryer, where string, arguments ...any) (contract.CheckpointPhaseDomainResultV2, error) {
	var body []byte
	if err := q.QueryRowContext(ctx, `SELECT body FROM checkpoint_phase_domain_result_history WHERE `+where, arguments...).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.CheckpointPhaseDomainResultV2{}, ports.ErrNotFound
		}
		return contract.CheckpointPhaseDomainResultV2{}, err
	}
	var value contract.CheckpointPhaseDomainResultV2
	if err := decode(body, &value); err != nil || value.ValidateShape() != nil {
		return contract.CheckpointPhaseDomainResultV2{}, ports.ErrConflict
	}
	return value, nil
}

func (s *Store) CommitCheckpointPhaseApplySettlementV2(ctx context.Context, expected contract.Ref, fact contract.CheckpointPhaseFact, next contract.CheckpointParticipantFact) (bool, error) {
	if expected.ValidateShape("expected checkpoint Participant") != nil || fact.ValidateShape() != nil || next.ValidateShape() != nil {
		return false, ports.ErrConflict
	}
	factBody, err := encode(fact)
	if err != nil {
		return false, err
	}
	participantBody, err := encode(next)
	if err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	current, err := readCheckpointParticipantCurrent(ctx, tx, expected.ID)
	if err != nil {
		return false, err
	}
	if !contract.SameRef(current.Meta.Ref(), expected) || next.Meta.ID != expected.ID || next.Meta.Revision != expected.Revision+1 || current.ActiveReservation.Ref == nil || !contract.SameRef(*current.ActiveReservation.Ref, fact.ReservationRef) || !contract.SameRef(fact.ParticipantRef, expected) || next.Closure == nil || !contract.SameCheckpointPhaseClosure(*next.Closure, fact.ClosureRef()) {
		return false, ports.ErrConflict
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO checkpoint_phase_fact_history(fact_id,revision,digest,reservation_id,reservation_revision,reservation_digest,body) VALUES(?,?,?,?,?,?,?)`, fact.Meta.ID, fact.Meta.Revision, fact.Meta.Digest, fact.ReservationRef.ID, fact.ReservationRef.Revision, fact.ReservationRef.Digest, factBody)
	if err != nil {
		return false, classifyWrite(err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 0 {
		existing, inspectErr := readCheckpointFactByReservationV2(ctx, tx, fact.ReservationRef)
		if inspectErr != nil || !contract.SameRef(existing.Meta.Ref(), fact.Meta.Ref()) {
			return false, ports.ErrConflict
		}
		return false, nil
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO checkpoint_phase_fact_current(fact_id,revision,digest,reservation_id,reservation_revision,reservation_digest,body) VALUES(?,?,?,?,?,?,?)`, fact.Meta.ID, fact.Meta.Revision, fact.Meta.Digest, fact.ReservationRef.ID, fact.ReservationRef.Revision, fact.ReservationRef.Digest, factBody); err != nil {
		return false, classifyWrite(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO checkpoint_participant_history(participant_id,revision,digest,body) VALUES(?,?,?,?)`, next.Meta.ID, next.Meta.Revision, next.Meta.Digest, participantBody); err != nil {
		return false, classifyWrite(err)
	}
	result, err = tx.ExecContext(ctx, `UPDATE checkpoint_participant_current SET revision=?,digest=?,body=? WHERE participant_id=? AND revision=? AND digest=?`, next.Meta.Revision, next.Meta.Digest, participantBody, expected.ID, expected.Revision, expected.Digest)
	if err != nil {
		return false, err
	}
	rows, err = result.RowsAffected()
	if err != nil || rows != 1 {
		if err != nil {
			return false, err
		}
		return false, ports.ErrConflict
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func readCheckpointFactByReservationV2(ctx context.Context, q queryer, reservation contract.Ref) (contract.CheckpointPhaseFact, error) {
	var body []byte
	err := q.QueryRowContext(ctx, `SELECT body FROM checkpoint_phase_fact_current WHERE reservation_id=? AND reservation_revision=? AND reservation_digest=?`, reservation.ID, reservation.Revision, reservation.Digest).Scan(&body)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.CheckpointPhaseFact{}, ports.ErrNotFound
		}
		return contract.CheckpointPhaseFact{}, err
	}
	var value contract.CheckpointPhaseFact
	if err := decode(body, &value); err != nil || value.ValidateShape() != nil || !contract.SameRef(value.ReservationRef, reservation) {
		return contract.CheckpointPhaseFact{}, ports.ErrConflict
	}
	return value, nil
}

var _ ports.CheckpointPhaseResultStoreV2 = (*Store)(nil)
