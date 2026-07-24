package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

type WorkspaceRestoreSettlementStoreV1 interface {
	CreateWorkspaceRestoreApplySettlementV1(context.Context, contract.WorkspaceRestoreApplySettlementFactV1) (contract.WorkspaceRestoreApplySettlementFactV1, error)
	InspectWorkspaceRestoreApplySettlementV1(context.Context, string, string) (contract.WorkspaceRestoreApplySettlementFactV1, error)
	InspectWorkspaceRestoreApplySettlementByStageV1(context.Context, string, contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreApplySettlementFactV1, error)
}

type WorkspaceRestoreSettlementOwnerPortV1 interface {
	ApplyWorkspaceRestoreSettlementV1(context.Context, contract.RuntimeRestoreStageSettlementRefV1) (contract.WorkspaceRestoreApplySettlementFactV1, error)
	InspectWorkspaceRestoreApplySettlementV1(context.Context, string, string) (contract.WorkspaceRestoreApplySettlementFactV1, error)
	InspectWorkspaceRestoreApplySettlementByStageV1(context.Context, string, contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreApplySettlementFactV1, error)
}
