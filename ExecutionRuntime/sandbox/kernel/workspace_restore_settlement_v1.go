package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

type WorkspaceRestoreSettlementStageReaderV1 interface {
	InspectWorkspaceRestoreStageFactV1(context.Context, contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreStageFactV1, error)
}

type WorkspaceRestoreSettlementOwnerV1 struct {
	stages WorkspaceRestoreSettlementStageReaderV1
	store  ports.WorkspaceRestoreSettlementStoreV1
	clock  func() time.Time
}

func NewWorkspaceRestoreSettlementOwnerV1(stages WorkspaceRestoreSettlementStageReaderV1, store ports.WorkspaceRestoreSettlementStoreV1, clock func() time.Time) (*WorkspaceRestoreSettlementOwnerV1, error) {
	if nilInterface(stages) || nilInterface(store) || nilInterface(clock) {
		return nil, errors.New("workspace restore Stage Reader, Settlement store, and clock are required")
	}
	return &WorkspaceRestoreSettlementOwnerV1{stages: stages, store: store, clock: clock}, nil
}

func (o *WorkspaceRestoreSettlementOwnerV1) ApplyWorkspaceRestoreSettlementV1(ctx context.Context, settlement contract.RuntimeRestoreStageSettlementRefV1) (contract.WorkspaceRestoreApplySettlementFactV1, error) {
	if o == nil || nilInterface(ctx) || settlement.ValidateShape() != nil {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, errors.New("workspace restore ApplySettlement request is invalid")
	}
	stage, err := o.stages.InspectWorkspaceRestoreStageFactV1(ctx, settlement.DomainResult)
	if err != nil {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, err
	}
	if stage.ValidateShape() != nil || stage.ExactRef() != settlement.DomainResult {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, fmt.Errorf("%w: opaque Runtime Settlement binds another Stage Fact", ports.ErrConflict)
	}
	now := o.clock()
	if now.IsZero() || now.UnixNano() >= stage.Meta.ExpiresUnixNano {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, fmt.Errorf("%w: workspace restore Stage Fact is stale", ports.ErrStale)
	}
	fact, err := contract.SealWorkspaceRestoreApplySettlementFactV1(contract.WorkspaceRestoreApplySettlementFactV1{Meta: contract.Meta{ID: settlement.ID + "-apply", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: stage.Meta.ExpiresUnixNano}, TenantID: stage.TenantID, StageFactRef: stage.ExactRef(), RuntimeSettlement: settlement})
	if err != nil {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, err
	}
	committed, createErr := o.store.CreateWorkspaceRestoreApplySettlementV1(ctx, fact)
	if createErr != nil {
		committed, err = o.store.InspectWorkspaceRestoreApplySettlementV1(context.WithoutCancel(ctx), stage.TenantID, fact.Meta.ID)
		if err != nil {
			return contract.WorkspaceRestoreApplySettlementFactV1{}, createErr
		}
	}
	if committed != fact {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, fmt.Errorf("%w: workspace restore ApplySettlement create-once winner differs", ports.ErrConflict)
	}
	return committed, nil
}

func (o *WorkspaceRestoreSettlementOwnerV1) InspectWorkspaceRestoreApplySettlementV1(ctx context.Context, tenantID, id string) (contract.WorkspaceRestoreApplySettlementFactV1, error) {
	if o == nil || nilInterface(ctx) {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, errors.New("workspace restore Settlement Owner or context is nil")
	}
	fact, err := o.store.InspectWorkspaceRestoreApplySettlementV1(ctx, tenantID, id)
	if err != nil {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, err
	}
	return fact, fact.ValidateShape()
}

func (o *WorkspaceRestoreSettlementOwnerV1) InspectWorkspaceRestoreApplySettlementByStageV1(ctx context.Context, tenantID string, stage contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreApplySettlementFactV1, error) {
	if o == nil || nilInterface(ctx) {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, errors.New("workspace restore Settlement Owner or context is nil")
	}
	fact, err := o.store.InspectWorkspaceRestoreApplySettlementByStageV1(ctx, tenantID, stage)
	if err != nil {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, err
	}
	if fact.ValidateShape() != nil || fact.TenantID != tenantID || fact.StageFactRef != stage {
		return contract.WorkspaceRestoreApplySettlementFactV1{}, fmt.Errorf("%w: workspace restore ApplySettlement current index drifted", ports.ErrConflict)
	}
	return fact, nil
}

var _ ports.WorkspaceRestoreSettlementOwnerPortV1 = (*WorkspaceRestoreSettlementOwnerV1)(nil)
