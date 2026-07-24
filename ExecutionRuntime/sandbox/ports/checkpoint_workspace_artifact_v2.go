package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

// CheckpointWorkspaceArtifactReaderV2 independently reads the exact local
// Provider checkpoint artifact and returns canonical workspace content. It
// never dispatches a Provider and owns no Runtime or Snapshot Artifact Fact.
type CheckpointWorkspaceArtifactReaderV2 interface {
	InspectCheckpointWorkspaceArtifactV2(context.Context, *contract.InspectCheckpointWorkspaceArtifactRequestV2) (contract.CheckpointWorkspaceArtifactInspectionV2, error)
}
