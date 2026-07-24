package sqlite

import (
	"context"
	"fmt"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	restoreStageResultKindV1     = "restore_stage_action_result"
	restoreExecutionIntentKindV1 = "restore_execution_intent"
	restoreExecutionResultKindV1 = "restore_execution_result"
)

func (s *StoreV1) CreateRestoreExecutionIntentV1(ctx context.Context, candidate applicationcontract.RestoreExecutionIntentFactV1) (applicationcontract.RestoreExecutionIntentFactV1, error) {
	created := time.Unix(0, candidate.CreatedUnixNano)
	if err := candidate.ValidateCurrent(created); err != nil {
		return applicationcontract.RestoreExecutionIntentFactV1{}, err
	}
	key := restoreResultStoreKeyV1(candidate.TenantID, candidate.ID)
	payload, err := s.ensure(ctx, restoreExecutionIntentKindV1, key, candidate.Revision, candidate.Digest, "", candidate.CreatedUnixNano, candidate.Request.NotAfterUnixNano, candidate.Clone())
	if err != nil {
		return applicationcontract.RestoreExecutionIntentFactV1{}, err
	}
	stored, err := decodeRestoreExecutionIntentV1(payload, candidate.TenantID, candidate.ID)
	if err != nil || stored.Digest != candidate.Digest {
		if err != nil {
			return applicationcontract.RestoreExecutionIntentFactV1{}, err
		}
		return applicationcontract.RestoreExecutionIntentFactV1{}, conflict("Restore execution Intent ID binds different content")
	}
	return stored.Clone(), nil
}

func (s *StoreV1) InspectRestoreExecutionIntentV1(ctx context.Context, tenant core.TenantID, id string) (applicationcontract.RestoreExecutionIntentFactV1, error) {
	row, err := s.readCurrent(ctx, restoreExecutionIntentKindV1, restoreResultStoreKeyV1(tenant, id))
	if err != nil {
		return applicationcontract.RestoreExecutionIntentFactV1{}, err
	}
	value, err := decodeRestoreExecutionIntentV1(row.payload, tenant, id)
	if err != nil || value.Revision != row.revision || value.Digest != row.digest || value.CreatedUnixNano != row.checked || value.Request.NotAfterUnixNano != row.expires {
		if err != nil {
			return applicationcontract.RestoreExecutionIntentFactV1{}, err
		}
		return applicationcontract.RestoreExecutionIntentFactV1{}, corrupt("Restore execution Intent row coordinates drifted")
	}
	return value.Clone(), nil
}

func decodeRestoreExecutionIntentV1(payload []byte, tenant core.TenantID, id string) (applicationcontract.RestoreExecutionIntentFactV1, error) {
	value, err := strictDecodeV1[applicationcontract.RestoreExecutionIntentFactV1](payload)
	if err != nil {
		return value, err
	}
	if value.TenantID != tenant || value.ID != id {
		return applicationcontract.RestoreExecutionIntentFactV1{}, corrupt("Restore execution Intent payload drifted")
	}
	if err := value.ValidateCurrent(time.Unix(0, value.CreatedUnixNano)); err != nil {
		return applicationcontract.RestoreExecutionIntentFactV1{}, fmt.Errorf("%w: Restore execution Intent payload validation failed", err)
	}
	return value, nil
}

func restoreResultStoreKeyV1(tenant core.TenantID, id string) string {
	return fmt.Sprintf("%d:%s%d:%s", len(tenant), tenant, len(id), id)
}

func (s *StoreV1) CreateRestoreStageActionResultV1(ctx context.Context, candidate applicationcontract.RestoreStageActionResultFactV1) (applicationcontract.RestoreStageActionResultFactV1, error) {
	created := time.Unix(0, candidate.CreatedUnixNano)
	if err := candidate.ValidateFor(candidate.Request, created); err != nil {
		return applicationcontract.RestoreStageActionResultFactV1{}, err
	}
	key := restoreResultStoreKeyV1(candidate.TenantID, candidate.ID)
	payload, err := s.ensure(ctx, restoreStageResultKindV1, key, candidate.Revision, candidate.Digest, "", candidate.CreatedUnixNano, candidate.Result.ExpiresUnixNano, candidate.Clone())
	if err != nil {
		return applicationcontract.RestoreStageActionResultFactV1{}, err
	}
	stored, err := decodeRestoreStageResultV1(payload, candidate.TenantID, candidate.ID)
	if err != nil || stored.Digest != candidate.Digest {
		if err != nil {
			return applicationcontract.RestoreStageActionResultFactV1{}, err
		}
		return applicationcontract.RestoreStageActionResultFactV1{}, conflict("Restore Stage result ID binds different content")
	}
	return stored.Clone(), nil
}

func (s *StoreV1) InspectRestoreStageActionResultV1(ctx context.Context, tenant core.TenantID, id string) (applicationcontract.RestoreStageActionResultFactV1, error) {
	row, err := s.readCurrent(ctx, restoreStageResultKindV1, restoreResultStoreKeyV1(tenant, id))
	if err != nil {
		return applicationcontract.RestoreStageActionResultFactV1{}, err
	}
	value, err := decodeRestoreStageResultV1(row.payload, tenant, id)
	if err != nil || value.Revision != row.revision || value.Digest != row.digest || value.CreatedUnixNano != row.checked || value.Result.ExpiresUnixNano != row.expires {
		if err != nil {
			return applicationcontract.RestoreStageActionResultFactV1{}, err
		}
		return applicationcontract.RestoreStageActionResultFactV1{}, corrupt("Restore Stage result row coordinates drifted")
	}
	return value.Clone(), nil
}

func decodeRestoreStageResultV1(payload []byte, tenant core.TenantID, id string) (applicationcontract.RestoreStageActionResultFactV1, error) {
	value, err := strictDecodeV1[applicationcontract.RestoreStageActionResultFactV1](payload)
	if err != nil {
		return value, err
	}
	if value.TenantID != tenant || value.ID != id {
		return applicationcontract.RestoreStageActionResultFactV1{}, corrupt("Restore Stage result payload drifted")
	}
	if err := value.ValidateFor(value.Request, time.Unix(0, value.CreatedUnixNano)); err != nil {
		return applicationcontract.RestoreStageActionResultFactV1{}, fmt.Errorf("%w: Restore Stage result payload validation failed", err)
	}
	return value, nil
}

func (s *StoreV1) CreateRestoreExecutionResultV1(ctx context.Context, candidate applicationcontract.RestoreExecutionResultFactV1) (applicationcontract.RestoreExecutionResultFactV1, error) {
	created := time.Unix(0, candidate.CreatedUnixNano)
	if err := candidate.ValidateCurrent(created); err != nil {
		return applicationcontract.RestoreExecutionResultFactV1{}, err
	}
	key := restoreResultStoreKeyV1(candidate.TenantID, candidate.ID)
	payload, err := s.ensure(ctx, restoreExecutionResultKindV1, key, candidate.Revision, candidate.Digest, "", candidate.CreatedUnixNano, candidate.Request.NotAfterUnixNano, candidate.Clone())
	if err != nil {
		return applicationcontract.RestoreExecutionResultFactV1{}, err
	}
	stored, err := decodeRestoreExecutionResultV1(payload, candidate.TenantID, candidate.ID)
	if err != nil || stored.Digest != candidate.Digest {
		if err != nil {
			return applicationcontract.RestoreExecutionResultFactV1{}, err
		}
		return applicationcontract.RestoreExecutionResultFactV1{}, conflict("Restore execution result ID binds different content")
	}
	return stored.Clone(), nil
}

func (s *StoreV1) InspectRestoreExecutionResultV1(ctx context.Context, tenant core.TenantID, id string) (applicationcontract.RestoreExecutionResultFactV1, error) {
	row, err := s.readCurrent(ctx, restoreExecutionResultKindV1, restoreResultStoreKeyV1(tenant, id))
	if err != nil {
		return applicationcontract.RestoreExecutionResultFactV1{}, err
	}
	value, err := decodeRestoreExecutionResultV1(row.payload, tenant, id)
	if err != nil || value.Revision != row.revision || value.Digest != row.digest || value.CreatedUnixNano != row.checked || value.Request.NotAfterUnixNano != row.expires {
		if err != nil {
			return applicationcontract.RestoreExecutionResultFactV1{}, err
		}
		return applicationcontract.RestoreExecutionResultFactV1{}, corrupt("Restore execution result row coordinates drifted")
	}
	return value.Clone(), nil
}

func decodeRestoreExecutionResultV1(payload []byte, tenant core.TenantID, id string) (applicationcontract.RestoreExecutionResultFactV1, error) {
	value, err := strictDecodeV1[applicationcontract.RestoreExecutionResultFactV1](payload)
	if err != nil {
		return value, err
	}
	if value.TenantID != tenant || value.ID != id {
		return applicationcontract.RestoreExecutionResultFactV1{}, corrupt("Restore execution result payload drifted")
	}
	if err := value.ValidateCurrent(time.Unix(0, value.CreatedUnixNano)); err != nil {
		return applicationcontract.RestoreExecutionResultFactV1{}, fmt.Errorf("%w: Restore execution result payload validation failed", err)
	}
	return value, nil
}

func (s *StoreV1) LoseNextRestoreStageResultReplyV1() {
	s.faultMu.Lock()
	s.loseRestoreStageEnsure = true
	s.faultMu.Unlock()
}

func (s *StoreV1) LoseNextRestoreExecutionResultReplyV1() {
	s.faultMu.Lock()
	s.loseRestoreExecutionEnsure = true
	s.faultMu.Unlock()
}

var _ applicationports.RestoreStageActionResultFactPortV1 = (*StoreV1)(nil)
var _ applicationports.RestoreExecutionIntentFactPortV1 = (*StoreV1)(nil)
var _ applicationports.RestoreExecutionResultFactPortV1 = (*StoreV1)(nil)
