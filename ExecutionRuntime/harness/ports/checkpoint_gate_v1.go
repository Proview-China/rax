package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// CheckpointGateStoreV1 owns the atomic Gate+Snapshot boundary. Implementations
// must preserve immutable history and a single current Gate per Harness Run.
type CheckpointGateStoreV1 interface {
	CreateCheckpointGateAndSnapshotV1(context.Context, contract.CheckpointGateFactV1, contract.HarnessCheckpointSnapshotFactV1) (contract.CheckpointGateFactV1, contract.HarnessCheckpointSnapshotFactV1, error)
	InspectCheckpointGateV1(context.Context, contract.CheckpointGateRefV1) (contract.CheckpointGateFactV1, error)
	InspectCheckpointGateCurrentV1(context.Context, contract.RunRef) (contract.CheckpointGateFactV1, error)
	InspectHarnessCheckpointSnapshotV1(context.Context, contract.HarnessCheckpointSnapshotRefV1) (contract.HarnessCheckpointSnapshotFactV1, error)
	BindCheckpointGateRuntimeV1(context.Context, contract.CheckpointGateRefV1, contract.CheckpointGateFactV1) (contract.CheckpointGateFactV1, error)
	InvalidateCheckpointGateV1(context.Context, contract.CheckpointGateRefV1, contract.CheckpointGateFactV1) (contract.CheckpointGateFactV1, error)
	ReleaseCheckpointGateV1(context.Context, contract.CheckpointGateRefV1, contract.CheckpointGateFactV1) (contract.CheckpointGateFactV1, error)
}

type CheckpointGateGovernancePortV1 interface {
	AcquireCheckpointGateV1(context.Context, contract.AcquireCheckpointGateRequestV1) (contract.CheckpointGateFactV1, contract.HarnessCheckpointSnapshotFactV1, error)
	InspectCheckpointGateV1(context.Context, contract.CheckpointGateRefV1) (contract.CheckpointGateFactV1, error)
	InspectHarnessCheckpointSnapshotV1(context.Context, contract.HarnessCheckpointSnapshotRefV1) (contract.HarnessCheckpointSnapshotFactV1, error)
	InspectCheckpointGateCurrentV1(context.Context, contract.RunRef) (contract.CheckpointGateFactV1, error)
	BindCheckpointGateRuntimeV1(context.Context, contract.BindCheckpointGateRuntimeRequestV1) (contract.CheckpointGateFactV1, error)
	InvalidateCheckpointGateV1(context.Context, contract.InvalidateCheckpointGateRequestV1) (contract.CheckpointGateFactV1, error)
	ReleaseCheckpointGateV1(context.Context, contract.ReleaseCheckpointGateRequestV1) (contract.CheckpointGateFactV1, error)
}

type CheckpointTerminalCurrentReaderV1 interface {
	InspectCheckpointAttemptTerminalCurrentV2(context.Context, ports.CheckpointAttemptRefV2) (ports.CheckpointAttemptTerminalCurrentProjectionV2, error)
}
