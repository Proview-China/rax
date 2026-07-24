package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
)

func (s *Store) CreateWorkspaceRestorePreparedRuntimeBindingV1(ctx context.Context, value runtimeadapter.WorkspaceRestorePreparedRuntimeBindingV1) (runtimeadapter.WorkspaceRestorePreparedRuntimeBindingV1, error) {
	if err := value.Validate(); err != nil {
		return runtimeadapter.WorkspaceRestorePreparedRuntimeBindingV1{}, err
	}
	body, err := encode(value)
	if err != nil {
		return runtimeadapter.WorkspaceRestorePreparedRuntimeBindingV1{}, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_restore_prepared_runtime_bindings(tenant_id,attempt_id,body) VALUES(?,?,?)`, value.TenantID, value.Attempt.ID, body)
	if err != nil {
		return runtimeadapter.WorkspaceRestorePreparedRuntimeBindingV1{}, err
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil {
		return runtimeadapter.WorkspaceRestorePreparedRuntimeBindingV1{}, rowsErr
	} else if rows == 1 {
		return value, nil
	}
	existing, err := s.InspectWorkspaceRestorePreparedRuntimeBindingV1(ctx, value.TenantID, value.Attempt.ID)
	if err != nil {
		return runtimeadapter.WorkspaceRestorePreparedRuntimeBindingV1{}, err
	}
	if existing != value {
		return runtimeadapter.WorkspaceRestorePreparedRuntimeBindingV1{}, ports.ErrConflict
	}
	return existing, nil
}

func (s *Store) InspectWorkspaceRestorePreparedRuntimeBindingV1(ctx context.Context, tenantID, attemptID string) (runtimeadapter.WorkspaceRestorePreparedRuntimeBindingV1, error) {
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM workspace_restore_prepared_runtime_bindings WHERE tenant_id=? AND attempt_id=?`, tenantID, attemptID).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return runtimeadapter.WorkspaceRestorePreparedRuntimeBindingV1{}, ports.ErrNotFound
		}
		return runtimeadapter.WorkspaceRestorePreparedRuntimeBindingV1{}, err
	}
	var value runtimeadapter.WorkspaceRestorePreparedRuntimeBindingV1
	if err := decode(body, &value); err != nil || value.Validate() != nil || value.TenantID != tenantID || value.Attempt.ID != attemptID {
		return runtimeadapter.WorkspaceRestorePreparedRuntimeBindingV1{}, fmt.Errorf("%w: stored workspace restore prepared binding drifted", ports.ErrConflict)
	}
	return value, nil
}

func (s *Store) CreateWorkspaceRestoreStageRuntimeBindingV1(ctx context.Context, value runtimeadapter.WorkspaceRestoreStageRuntimeBindingV1) (runtimeadapter.WorkspaceRestoreStageRuntimeBindingV1, error) {
	if err := value.Validate(); err != nil {
		return runtimeadapter.WorkspaceRestoreStageRuntimeBindingV1{}, err
	}
	body, err := encode(value)
	if err != nil {
		return runtimeadapter.WorkspaceRestoreStageRuntimeBindingV1{}, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_restore_stage_runtime_bindings(tenant_id,fact_id,body) VALUES(?,?,?)`, value.TenantID, value.FactRef.ID, body)
	if err != nil {
		return runtimeadapter.WorkspaceRestoreStageRuntimeBindingV1{}, err
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil {
		return runtimeadapter.WorkspaceRestoreStageRuntimeBindingV1{}, rowsErr
	} else if rows == 1 {
		return value, nil
	}
	existing, err := s.InspectWorkspaceRestoreStageRuntimeBindingV1(ctx, value.TenantID, value.FactRef.ID)
	if err != nil {
		return runtimeadapter.WorkspaceRestoreStageRuntimeBindingV1{}, err
	}
	if existing != value {
		return runtimeadapter.WorkspaceRestoreStageRuntimeBindingV1{}, ports.ErrConflict
	}
	return existing, nil
}

func (s *Store) InspectWorkspaceRestoreStageRuntimeBindingV1(ctx context.Context, tenantID, factID string) (runtimeadapter.WorkspaceRestoreStageRuntimeBindingV1, error) {
	var body []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM workspace_restore_stage_runtime_bindings WHERE tenant_id=? AND fact_id=?`, tenantID, factID).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return runtimeadapter.WorkspaceRestoreStageRuntimeBindingV1{}, ports.ErrNotFound
		}
		return runtimeadapter.WorkspaceRestoreStageRuntimeBindingV1{}, err
	}
	var value runtimeadapter.WorkspaceRestoreStageRuntimeBindingV1
	if err := decode(body, &value); err != nil || value.Validate() != nil || value.TenantID != tenantID || value.FactRef.ID != factID {
		return runtimeadapter.WorkspaceRestoreStageRuntimeBindingV1{}, fmt.Errorf("%w: stored workspace restore Stage binding drifted", ports.ErrConflict)
	}
	return value, nil
}

func (s *Store) CreateWorkspaceRestoreApplySettlementV1(ctx context.Context, fact contract.WorkspaceRestoreApplySettlementFactV1) (contract.WorkspaceRestoreApplySettlementFactV1, error) {
	if err := fact.ValidateShape(); err != nil {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, err
	}
	body, err := encode(fact)
	if err != nil {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, err
	}
	result, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO workspace_restore_apply_settlement_facts(tenant_id,fact_id,stage_id,stage_revision,stage_digest,body) VALUES(?,?,?,?,?,?)`, fact.TenantID, fact.Meta.ID, fact.StageFactRef.ID, fact.StageFactRef.Revision, fact.StageFactRef.Digest, body)
	if err != nil {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, err
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, rowsErr
	} else if rows == 1 {
		return fact, nil
	}
	existing, err := s.InspectWorkspaceRestoreApplySettlementV1(ctx, fact.TenantID, fact.Meta.ID)
	if err != nil {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, err
	}
	if existing != fact {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, ports.ErrConflict
	}
	return existing, nil
}

func (s *Store) InspectWorkspaceRestoreApplySettlementV1(ctx context.Context, tenantID, id string) (contract.WorkspaceRestoreApplySettlementFactV1, error) {
	return readWorkspaceRestoreApplySettlementV1(ctx, s.db, `tenant_id=? AND fact_id=?`, tenantID, id)
}

func (s *Store) InspectWorkspaceRestoreApplySettlementByStageV1(ctx context.Context, tenantID string, stage contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreApplySettlementFactV1, error) {
	return readWorkspaceRestoreApplySettlementV1(ctx, s.db, `tenant_id=? AND stage_id=? AND stage_revision=? AND stage_digest=?`, tenantID, stage.ID, stage.Revision, stage.Digest)
}

func readWorkspaceRestoreApplySettlementV1(ctx context.Context, q queryer, where string, arguments ...any) (contract.WorkspaceRestoreApplySettlementFactV1, error) {
	var body []byte
	if err := q.QueryRowContext(ctx, `SELECT body FROM workspace_restore_apply_settlement_facts WHERE `+where, arguments...).Scan(&body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.WorkspaceRestoreApplySettlementFactV1{}, ports.ErrNotFound
		}
		return contract.WorkspaceRestoreApplySettlementFactV1{}, err
	}
	var value contract.WorkspaceRestoreApplySettlementFactV1
	if err := decode(body, &value); err != nil || value.ValidateShape() != nil {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, fmt.Errorf("%w: stored workspace restore ApplySettlement drifted", ports.ErrConflict)
	}
	return value, nil
}

var _ runtimeadapter.WorkspaceRestorePreparedRuntimeBindingStoreV1 = (*Store)(nil)
var _ runtimeadapter.WorkspaceRestoreStageRuntimeBindingStoreV1 = (*Store)(nil)
var _ ports.WorkspaceRestoreSettlementStoreV1 = (*Store)(nil)
