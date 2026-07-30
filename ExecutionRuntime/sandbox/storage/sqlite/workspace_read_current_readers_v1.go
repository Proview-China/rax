package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func (s *Store) InspectWorkspaceReadReservationExactV1(ctx context.Context, exact contract.Ref) (contract.WorkspaceReadReservationV1, error) {
	if err := exact.ValidateShape("workspace read reservation"); err != nil {
		return contract.WorkspaceReadReservationV1{}, err
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM workspace_read_reservation WHERE reservation_id=?`, exact.ID).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceReadReservationV1{}, ports.ErrNotFound
		}
		return contract.WorkspaceReadReservationV1{}, err
	}
	var reservation contract.WorkspaceReadReservationV1
	if err := decode(body, &reservation); err != nil {
		return contract.WorkspaceReadReservationV1{}, err
	}
	if err := reservation.ValidateShape(); err != nil {
		return contract.WorkspaceReadReservationV1{}, err
	}
	if reservation.Meta.Ref() != exact {
		return contract.WorkspaceReadReservationV1{}, ports.ErrConflict
	}
	return reservation, nil
}

func (s *Store) InspectWorkspaceReadAttemptCurrentV1(ctx context.Context, exact contract.WorkspaceReadAttemptRefV1) (contract.WorkspaceReadAttemptV1, error) {
	if err := exact.Validate(); err != nil {
		return contract.WorkspaceReadAttemptV1{}, err
	}
	var originRevision uint64
	var originDigest string
	if err := s.db.QueryRowContext(ctx, `SELECT revision,digest FROM workspace_read_attempt_origin WHERE attempt_id=?`, exact.ID).Scan(&originRevision, &originDigest); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceReadAttemptV1{}, ports.ErrNotFound
		}
		return contract.WorkspaceReadAttemptV1{}, err
	}
	if originRevision != exact.Revision || originDigest != exact.Digest {
		return contract.WorkspaceReadAttemptV1{}, ports.ErrConflict
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM workspace_read_attempt_current WHERE attempt_id=?`, exact.ID).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceReadAttemptV1{}, ports.ErrNotFound
		}
		return contract.WorkspaceReadAttemptV1{}, err
	}
	var attempt contract.WorkspaceReadAttemptV1
	if err := decode(body, &attempt); err != nil {
		return contract.WorkspaceReadAttemptV1{}, err
	}
	if err := attempt.ValidateShape(); err != nil {
		return contract.WorkspaceReadAttemptV1{}, err
	}
	if attempt.Meta.ID != exact.ID {
		return contract.WorkspaceReadAttemptV1{}, ports.ErrConflict
	}
	return attempt, nil
}

var _ ports.WorkspaceReadReservationExactReaderV1 = (*Store)(nil)
var _ ports.WorkspaceReadAttemptCurrentReaderV1 = (*Store)(nil)
