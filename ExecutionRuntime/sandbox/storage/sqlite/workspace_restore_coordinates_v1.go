package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
)

// PutRestoreStageCoordinatesV1 durably records the exact Runtime governance
// coordinates used by the Sandbox actual execution point. The stable request
// is create-once: an exact replay is idempotent and changed content conflicts.
func (s *Store) PutRestoreStageCoordinatesV1(ctx context.Context, request contract.WorkspaceRestoreStageRequestV1, value runtimeports.InspectRestoreStageGovernanceCurrentRequestV1) error {
	if ctx == nil {
		return errors.New("workspace Restore Stage coordinate context is nil")
	}
	if err := request.ValidateShape(); err != nil {
		return fmt.Errorf("workspace Restore Stage coordinate request is invalid: %w", err)
	}
	if err := value.Validate(); err != nil {
		return fmt.Errorf("workspace Restore Stage coordinate value is invalid: %w", err)
	}
	stable, err := request.StableKeyDigest()
	if err != nil {
		return err
	}
	requestBody, err := encode(request)
	if err != nil {
		return err
	}
	coordinateBody, err := encode(value)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_restore_stage_coordinates(stable_digest,tenant_id,request_body,coordinate_body) VALUES(?,?,?,?)`, stable, request.TenantID, requestBody, coordinateBody)
	if err != nil {
		return err
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil {
		return rowsErr
	} else if rows == 1 {
		return nil
	}
	existing, err := s.readRestoreStageCoordinatesV1(ctx, stable)
	if err != nil {
		return err
	}
	if !bytes.Equal(existing.requestBody, requestBody) || !bytes.Equal(existing.coordinateBody, coordinateBody) {
		return ports.ErrConflict
	}
	return nil
}

func (s *Store) ReadRestoreStageCoordinatesV1(ctx context.Context, request contract.WorkspaceRestoreStageRequestV1) (runtimeports.InspectRestoreStageGovernanceCurrentRequestV1, error) {
	if ctx == nil || request.ValidateShape() != nil {
		return runtimeports.InspectRestoreStageGovernanceCurrentRequestV1{}, errors.New("workspace Restore Stage coordinate read is invalid")
	}
	stable, err := request.StableKeyDigest()
	if err != nil {
		return runtimeports.InspectRestoreStageGovernanceCurrentRequestV1{}, err
	}
	existing, err := s.readRestoreStageCoordinatesV1(ctx, stable)
	if err != nil {
		return runtimeports.InspectRestoreStageGovernanceCurrentRequestV1{}, err
	}
	requestBody, err := encode(request)
	if err != nil {
		return runtimeports.InspectRestoreStageGovernanceCurrentRequestV1{}, err
	}
	if !bytes.Equal(existing.requestBody, requestBody) || existing.request.TenantID != request.TenantID {
		return runtimeports.InspectRestoreStageGovernanceCurrentRequestV1{}, ports.ErrConflict
	}
	return existing.coordinates, nil
}

type restoreStageCoordinatesRowV1 struct {
	request        contract.WorkspaceRestoreStageRequestV1
	coordinates    runtimeports.InspectRestoreStageGovernanceCurrentRequestV1
	requestBody    []byte
	coordinateBody []byte
}

func (s *Store) readRestoreStageCoordinatesV1(ctx context.Context, stable string) (restoreStageCoordinatesRowV1, error) {
	var requestBody, coordinateBody []byte
	if err := s.db.QueryRowContext(ctx, `SELECT request_body,coordinate_body FROM workspace_restore_stage_coordinates WHERE stable_digest=?`, stable).Scan(&requestBody, &coordinateBody); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return restoreStageCoordinatesRowV1{}, ports.ErrNotFound
		}
		return restoreStageCoordinatesRowV1{}, err
	}
	var row restoreStageCoordinatesRowV1
	row.requestBody = append([]byte{}, requestBody...)
	row.coordinateBody = append([]byte{}, coordinateBody...)
	if err := decode(requestBody, &row.request); err != nil || row.request.ValidateShape() != nil {
		return restoreStageCoordinatesRowV1{}, fmt.Errorf("%w: stored workspace Restore Stage request drifted", ports.ErrConflict)
	}
	if err := decode(coordinateBody, &row.coordinates); err != nil || row.coordinates.Validate() != nil {
		return restoreStageCoordinatesRowV1{}, fmt.Errorf("%w: stored workspace Restore Stage coordinates drifted", ports.ErrConflict)
	}
	stableCheck, err := row.request.StableKeyDigest()
	if err != nil || stableCheck != stable {
		return restoreStageCoordinatesRowV1{}, fmt.Errorf("%w: stored workspace Restore Stage stable key drifted", ports.ErrConflict)
	}
	return row, nil
}

var _ runtimeadapter.RestoreStageCoordinateReaderV1 = (*Store)(nil)
