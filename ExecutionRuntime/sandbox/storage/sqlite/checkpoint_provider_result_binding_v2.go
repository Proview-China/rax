package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/applicationadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

func (s *Store) CreateCheckpointProviderResultBindingV2(ctx context.Context, value applicationadapter.CheckpointProviderResultBindingV2) (applicationadapter.CheckpointProviderResultBindingV2, error) {
	if value.Validate() != nil {
		return applicationadapter.CheckpointProviderResultBindingV2{}, ports.ErrConflict
	}
	body, err := encode(value)
	if err != nil {
		return applicationadapter.CheckpointProviderResultBindingV2{}, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO checkpoint_provider_result_bindings(reservation_id,reservation_revision,reservation_digest,body) VALUES(?,?,?,?)`, value.Reservation.ID, value.Reservation.Revision, value.Reservation.Digest, body)
	if err != nil {
		return applicationadapter.CheckpointProviderResultBindingV2{}, classifyWrite(err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return applicationadapter.CheckpointProviderResultBindingV2{}, err
	}
	if rows == 1 {
		return cloneCheckpointProviderResultBindingV2(value)
	}
	existing, err := s.InspectCheckpointProviderResultBindingV2(ctx, value.Reservation)
	if err != nil {
		return applicationadapter.CheckpointProviderResultBindingV2{}, err
	}
	if !reflect.DeepEqual(existing, value) {
		return applicationadapter.CheckpointProviderResultBindingV2{}, ports.ErrConflict
	}
	return existing, nil
}

func (s *Store) InspectCheckpointProviderResultBindingV2(ctx context.Context, reservation contract.Ref) (applicationadapter.CheckpointProviderResultBindingV2, error) {
	if reservation.ValidateShape("checkpoint Provider result reservation") != nil {
		return applicationadapter.CheckpointProviderResultBindingV2{}, ports.ErrConflict
	}
	var body []byte
	err := s.db.QueryRowContext(ctx, `SELECT body FROM checkpoint_provider_result_bindings WHERE reservation_id=? AND reservation_revision=? AND reservation_digest=?`, reservation.ID, reservation.Revision, reservation.Digest).Scan(&body)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return applicationadapter.CheckpointProviderResultBindingV2{}, ports.ErrNotFound
		}
		return applicationadapter.CheckpointProviderResultBindingV2{}, err
	}
	var value applicationadapter.CheckpointProviderResultBindingV2
	if decode(body, &value) != nil || value.Validate() != nil || !contract.SameRef(value.Reservation, reservation) {
		return applicationadapter.CheckpointProviderResultBindingV2{}, ports.ErrConflict
	}
	return cloneCheckpointProviderResultBindingV2(value)
}

func cloneCheckpointProviderResultBindingV2(value applicationadapter.CheckpointProviderResultBindingV2) (applicationadapter.CheckpointProviderResultBindingV2, error) {
	body, err := encode(value)
	if err != nil {
		return applicationadapter.CheckpointProviderResultBindingV2{}, err
	}
	var cloned applicationadapter.CheckpointProviderResultBindingV2
	if err := decode(body, &cloned); err != nil {
		return applicationadapter.CheckpointProviderResultBindingV2{}, err
	}
	return cloned, nil
}

var _ applicationadapter.CheckpointProviderResultBindingStoreV2 = (*Store)(nil)
