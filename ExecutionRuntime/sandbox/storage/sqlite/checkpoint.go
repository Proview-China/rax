package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// CreateCheckpointParticipant is the Sandbox Owner's create-once bootstrap.
// It is intentionally not part of the Application-facing governance Port.
func (s *Store) CreateCheckpointParticipant(ctx context.Context, value contract.CheckpointParticipantFact) error {
	if err := value.ValidateShape(); err != nil {
		return err
	}
	body, err := encode(value)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO checkpoint_participant_history(participant_id,revision,digest,body) VALUES(?,?,?,?)`, value.Meta.ID, value.Meta.Revision, value.Meta.Digest, body); err != nil {
		if !errors.Is(classifyWrite(err), ports.ErrConflict) {
			return err
		}
		existing, inspectErr := readCheckpointParticipant(ctx, tx, value.Meta.Ref())
		if inspectErr != nil || !contract.SameRef(existing.Meta.Ref(), value.Meta.Ref()) {
			return ports.ErrConflict
		}
		return nil
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO checkpoint_participant_current(participant_id,revision,digest,body) VALUES(?,?,?,?)`, value.Meta.ID, value.Meta.Revision, value.Meta.Digest, body); err != nil {
		return classifyWrite(err)
	}
	return tx.Commit()
}

func (s *Store) ReserveCheckpointPhase(ctx context.Context, expected contract.Ref, reservation contract.CheckpointPhaseReservation, next contract.CheckpointParticipantFact) (bool, error) {
	if expected.ValidateShape("expected checkpoint participant") != nil || reservation.ValidateShape() != nil || next.ValidateShape() != nil {
		return false, ports.ErrConflict
	}
	phaseKey, err := contract.CheckpointPhaseKey(reservation)
	if err != nil {
		return false, err
	}
	var branchKey any
	if reservation.PreviousPresence == contract.CheckpointPresent {
		key, keyErr := contract.CheckpointBranchKey(reservation)
		if keyErr != nil {
			return false, keyErr
		}
		branchKey = key
	}
	reservationBody, err := encode(reservation)
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
	if !contract.SameRef(current.Meta.Ref(), expected) || next.Meta.ID != expected.ID || next.Meta.Revision != expected.Revision+1 || next.ActiveReservation.Ref == nil || !contract.SameRef(*next.ActiveReservation.Ref, reservation.Meta.Ref()) || next.ActivePhase != reservation.Phase {
		return false, ports.ErrConflict
	}
	result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO checkpoint_phase_reservation_history(reservation_id,revision,digest,phase_key,branch_key,body) VALUES(?,?,?,?,?,?)`, reservation.Meta.ID, reservation.Meta.Revision, reservation.Meta.Digest, phaseKey, branchKey, reservationBody)
	if err != nil {
		return false, classifyWrite(err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 0 {
		existing, inspectErr := readCheckpointReservation(ctx, tx, reservation.Meta.Ref())
		if inspectErr != nil || !contract.SameRef(existing.Meta.Ref(), reservation.Meta.Ref()) {
			return false, ports.ErrConflict
		}
		return false, nil
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

func (s *Store) InspectCheckpointPhaseReservation(ctx context.Context, expected contract.Ref) (contract.CheckpointPhaseReservation, error) {
	return readCheckpointReservation(ctx, s.db, expected)
}

func (s *Store) InspectCheckpointParticipant(ctx context.Context, expected contract.Ref) (contract.CheckpointParticipantFact, error) {
	return readCheckpointParticipant(ctx, s.db, expected)
}

func (s *Store) InspectCheckpointParticipantCurrent(ctx context.Context, id string) (contract.CheckpointParticipantFact, error) {
	return readCheckpointParticipantCurrent(ctx, s.db, id)
}

func (s *Store) InspectCheckpointPhaseFact(ctx context.Context, expected contract.Ref) (contract.CheckpointPhaseFact, error) {
	var body []byte
	err := s.db.QueryRowContext(ctx, `SELECT body FROM checkpoint_phase_fact_history WHERE fact_id=? AND revision=? AND digest=?`, expected.ID, expected.Revision, expected.Digest).Scan(&body)
	return decodeCheckpointFact(body, err, expected)
}

func (s *Store) InspectCheckpointPhaseFactCurrent(ctx context.Context, id string) (contract.CheckpointPhaseFact, error) {
	var body []byte
	err := s.db.QueryRowContext(ctx, `SELECT body FROM checkpoint_phase_fact_current WHERE fact_id=?`, id).Scan(&body)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.CheckpointPhaseFact{}, ports.ErrNotFound
		}
		return contract.CheckpointPhaseFact{}, err
	}
	var value contract.CheckpointPhaseFact
	if err := decode(body, &value); err != nil {
		return contract.CheckpointPhaseFact{}, err
	}
	return value, value.ValidateShape()
}

func (s *Store) InspectCheckpointPhaseFactByReservation(ctx context.Context, reservation contract.Ref) (contract.CheckpointPhaseFact, error) {
	var body []byte
	err := s.db.QueryRowContext(ctx, `SELECT body FROM checkpoint_phase_fact_current WHERE reservation_id=? AND reservation_revision=? AND reservation_digest=?`, reservation.ID, reservation.Revision, reservation.Digest).Scan(&body)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.CheckpointPhaseFact{}, ports.ErrNotFound
		}
		return contract.CheckpointPhaseFact{}, err
	}
	var value contract.CheckpointPhaseFact
	if err := decode(body, &value); err != nil {
		return contract.CheckpointPhaseFact{}, err
	}
	if value.ValidateShape() != nil || !contract.SameRef(value.ReservationRef, reservation) {
		return contract.CheckpointPhaseFact{}, ports.ErrConflict
	}
	return value, nil
}

func readCheckpointReservation(ctx context.Context, q queryer, expected contract.Ref) (contract.CheckpointPhaseReservation, error) {
	var body []byte
	err := q.QueryRowContext(ctx, `SELECT body FROM checkpoint_phase_reservation_history WHERE reservation_id=? AND revision=? AND digest=?`, expected.ID, expected.Revision, expected.Digest).Scan(&body)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.CheckpointPhaseReservation{}, ports.ErrNotFound
		}
		return contract.CheckpointPhaseReservation{}, err
	}
	var value contract.CheckpointPhaseReservation
	if err := decode(body, &value); err != nil {
		return contract.CheckpointPhaseReservation{}, err
	}
	if value.ValidateShape() != nil || !contract.SameRef(value.Meta.Ref(), expected) {
		return contract.CheckpointPhaseReservation{}, ports.ErrConflict
	}
	return value, nil
}

func readCheckpointParticipant(ctx context.Context, q queryer, expected contract.Ref) (contract.CheckpointParticipantFact, error) {
	var body []byte
	err := q.QueryRowContext(ctx, `SELECT body FROM checkpoint_participant_history WHERE participant_id=? AND revision=? AND digest=?`, expected.ID, expected.Revision, expected.Digest).Scan(&body)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.CheckpointParticipantFact{}, ports.ErrNotFound
		}
		return contract.CheckpointParticipantFact{}, err
	}
	var value contract.CheckpointParticipantFact
	if err := decode(body, &value); err != nil {
		return contract.CheckpointParticipantFact{}, err
	}
	if value.ValidateShape() != nil || !contract.SameRef(value.Meta.Ref(), expected) {
		return contract.CheckpointParticipantFact{}, ports.ErrConflict
	}
	return value, nil
}

func readCheckpointParticipantCurrent(ctx context.Context, q queryer, id string) (contract.CheckpointParticipantFact, error) {
	var body []byte
	err := q.QueryRowContext(ctx, `SELECT body FROM checkpoint_participant_current WHERE participant_id=?`, id).Scan(&body)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.CheckpointParticipantFact{}, ports.ErrNotFound
		}
		return contract.CheckpointParticipantFact{}, err
	}
	var value contract.CheckpointParticipantFact
	if err := decode(body, &value); err != nil {
		return contract.CheckpointParticipantFact{}, err
	}
	return value, value.ValidateShape()
}

func decodeCheckpointFact(body []byte, queryErr error, expected contract.Ref) (contract.CheckpointPhaseFact, error) {
	if queryErr != nil {
		if errors.Is(queryErr, sql.ErrNoRows) {
			return contract.CheckpointPhaseFact{}, ports.ErrNotFound
		}
		return contract.CheckpointPhaseFact{}, queryErr
	}
	var value contract.CheckpointPhaseFact
	if err := decode(body, &value); err != nil {
		return contract.CheckpointPhaseFact{}, err
	}
	if value.ValidateShape() != nil || !contract.SameRef(value.Meta.Ref(), expected) {
		return contract.CheckpointPhaseFact{}, ports.ErrConflict
	}
	return value, nil
}

var _ ports.CheckpointPhaseStore = (*Store)(nil)
