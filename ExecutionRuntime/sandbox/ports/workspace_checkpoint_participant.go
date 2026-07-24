package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

// WorkspaceCheckpointPreparationCurrentReaderV2 is a read-only aggregation
// of the real Workspace, prepared phase, Snapshot Artifact, and coverage
// policy Owners. It does not create Participant facts or call a Provider.
type WorkspaceCheckpointPreparationCurrentReaderV2 interface {
	InspectWorkspaceCheckpointPreparationCurrentV2(context.Context, contract.PrepareWorkspaceCheckpointParticipantRequestV2) (contract.WorkspaceCheckpointPreparationCurrentProjectionV2, error)
}

// WorkspaceCheckpointParticipantStoreV2 is the Sandbox Owner's atomic
// create-once boundary. No raw CAS method is exposed to Application callers.
type WorkspaceCheckpointParticipantStoreV2 interface {
	CommitWorkspaceCheckpointPreparedV2(context.Context, contract.WorkspaceCheckpointPreparedBundleV2) (created bool, err error)
	InspectWorkspaceCheckpointPreparedV2(context.Context, contract.InspectWorkspaceCheckpointPreparedRequestV2) (contract.WorkspaceCheckpointPreparedBundleV2, error)
	InspectWorkspaceCheckpointParticipantV2(context.Context, contract.InspectWorkspaceCheckpointFactRequestV2) (contract.WorkspaceCheckpointParticipantFactV2, error)
	InspectWorkspaceCheckpointCoverageV2(context.Context, contract.InspectWorkspaceCheckpointFactRequestV2) (contract.WorkspaceCheckpointCoverageFactV2, error)
}

type WorkspaceCheckpointParticipantOwnerPortV2 interface {
	PrepareWorkspaceCheckpointParticipantV2(context.Context, *contract.PrepareWorkspaceCheckpointParticipantRequestV2) (contract.WorkspaceCheckpointPreparedBundleV2, error)
	InspectWorkspaceCheckpointPreparedV2(context.Context, *contract.InspectWorkspaceCheckpointPreparedRequestV2) (contract.WorkspaceCheckpointPreparedBundleV2, error)
}
