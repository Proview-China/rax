package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

// WorkspaceRestoreBundleCurrentReaderV1 resolves the canonical bundle only
// from the SnapshotArtifact Owner and encrypted Content Store exact refs.
type WorkspaceRestoreBundleCurrentReaderV1 interface {
	InspectWorkspaceRestoreBundleCurrentV1(context.Context, contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreBundleCurrentProjectionV1, error)
	InspectWorkspaceRestoreBundleExactV1(context.Context, contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreBundleCurrentProjectionV1, error)
}

// WorkspaceRestoreGovernanceCurrentReaderV1 aggregates typed Runtime,
// Application, Review and Enforcement current facts. It is read-only.
type WorkspaceRestoreGovernanceCurrentReaderV1 interface {
	InspectWorkspaceRestoreGovernanceCurrentV1(context.Context, contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreGovernanceCurrentProjectionV1, error)
}

// WorkspaceRestoreProviderV1 owns materialization only. Its response remains a
// receipt-like opaque root ref until Sandbox independently inspects and commits
// a Stage Fact.
type WorkspaceRestoreProviderV1 interface {
	StageWorkspaceRestoreV1(context.Context, *contract.WorkspaceRestoreProviderRequestV1) (contract.WorkspaceRestoreProviderResultV1, error)
	InspectWorkspaceRestoreV1(context.Context, *contract.WorkspaceRestoreProviderRequestV1) (contract.WorkspaceRestoreProviderResultV1, error)
}

// WorkspaceRestoreStoreV1 is the Sandbox Owner's private atomic persistence
// boundary. Application callers cannot write Attempts or Facts directly.
type WorkspaceRestoreStoreV1 interface {
	CreateWorkspaceRestoreAttemptV1(context.Context, contract.WorkspaceRestoreAttemptV1) (bool, error)
	CASWorkspaceRestoreAttemptV1(context.Context, contract.SnapshotArtifactExactRefV2, contract.WorkspaceRestoreAttemptV1) (bool, error)
	CommitWorkspaceRestoreStageV1(context.Context, contract.SnapshotArtifactExactRefV2, contract.WorkspaceRestoreAttemptV1, contract.WorkspaceRestoreStageFactV1) (bool, error)
	InspectWorkspaceRestoreAttemptByStableKeyV1(context.Context, string) (contract.WorkspaceRestoreAttemptV1, error)
	InspectWorkspaceRestoreAttemptV1(context.Context, contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreAttemptV1, error)
	InspectWorkspaceRestoreStageFactV1(context.Context, contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreStageFactV1, error)
}

type WorkspaceRestoreOwnerPortV1 interface {
	PrepareWorkspaceV1(context.Context, *contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreAttemptV1, error)
	StageWorkspaceV1(context.Context, *contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreStageFactV1, error)
	ReconcileWorkspaceV1(context.Context, *contract.WorkspaceRestoreStageRequestV1) (contract.WorkspaceRestoreStageFactV1, error)
	InspectWorkspaceRestoreAttemptV1(context.Context, *contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreAttemptV1, error)
	InspectWorkspaceRestoreStageFactV1(context.Context, *contract.SnapshotArtifactExactRefV2) (contract.WorkspaceRestoreStageFactV1, error)
}
