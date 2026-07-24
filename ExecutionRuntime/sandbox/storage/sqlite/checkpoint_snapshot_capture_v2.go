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

func (s *Store) CreateCheckpointSnapshotCaptureBindingV2(ctx context.Context, value applicationadapter.CheckpointSnapshotCaptureBindingV2) (applicationadapter.CheckpointSnapshotCaptureBindingV2, error) {
	if err := value.ValidateCurrent(s.clock()); err != nil {
		return applicationadapter.CheckpointSnapshotCaptureBindingV2{}, err
	}
	body, err := encode(value)
	if err != nil {
		return applicationadapter.CheckpointSnapshotCaptureBindingV2{}, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO checkpoint_snapshot_capture_bindings(snapshot_reservation_id,snapshot_reservation_revision,snapshot_reservation_digest,checkpoint_reservation_id,expires_unix_nano,digest,body) VALUES(?,?,?,?,?,?,?)`, value.SnapshotReservation.ID, value.SnapshotReservation.Revision, value.SnapshotReservation.Digest, value.CheckpointReservation.ID, value.ExpiresUnixNano, value.Digest, body)
	if err != nil {
		return applicationadapter.CheckpointSnapshotCaptureBindingV2{}, classifyWrite(err)
	}
	if rows, err := result.RowsAffected(); err != nil {
		return applicationadapter.CheckpointSnapshotCaptureBindingV2{}, err
	} else if rows == 1 {
		return value, nil
	}
	existing, err := s.InspectCheckpointSnapshotCaptureBindingV2(ctx, value.SnapshotReservation)
	if err != nil {
		return applicationadapter.CheckpointSnapshotCaptureBindingV2{}, err
	}
	if !reflect.DeepEqual(existing, value) {
		return applicationadapter.CheckpointSnapshotCaptureBindingV2{}, ports.ErrConflict
	}
	return existing, nil
}

func (s *Store) InspectCheckpointSnapshotCaptureBindingV2(ctx context.Context, expected contract.SnapshotArtifactExactRefV2) (applicationadapter.CheckpointSnapshotCaptureBindingV2, error) {
	if expected.ValidateShape("checkpoint snapshot capture binding") != nil {
		return applicationadapter.CheckpointSnapshotCaptureBindingV2{}, ports.ErrConflict
	}
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM checkpoint_snapshot_capture_bindings WHERE snapshot_reservation_id=? AND snapshot_reservation_revision=? AND snapshot_reservation_digest=?`, expected.ID, expected.Revision, expected.Digest).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return applicationadapter.CheckpointSnapshotCaptureBindingV2{}, ports.ErrNotFound
		}
		return applicationadapter.CheckpointSnapshotCaptureBindingV2{}, err
	}
	var value applicationadapter.CheckpointSnapshotCaptureBindingV2
	if err := decode(body, &value); err != nil {
		return applicationadapter.CheckpointSnapshotCaptureBindingV2{}, err
	}
	if err := value.ValidateCurrent(s.clock()); err != nil || !contract.SameSnapshotArtifactExactRef(value.SnapshotReservation, expected) {
		if err != nil {
			return applicationadapter.CheckpointSnapshotCaptureBindingV2{}, err
		}
		return applicationadapter.CheckpointSnapshotCaptureBindingV2{}, ports.ErrConflict
	}
	return value, nil
}

var _ applicationadapter.CheckpointSnapshotCaptureBindingStoreV2 = (*Store)(nil)
