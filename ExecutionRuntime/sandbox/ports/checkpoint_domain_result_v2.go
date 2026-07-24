package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

// CheckpointPhaseResultStoreV2 is private to the Sandbox Owner. DomainResult
// history is append-only; ApplySettlement atomically publishes the final phase
// fact and advances Participant current.
type CheckpointPhaseResultStoreV2 interface {
	CreateCheckpointPhaseDomainResultV2(context.Context, contract.CheckpointPhaseDomainResultV2) (bool, error)
	InspectCheckpointPhaseDomainResultV2(context.Context, contract.SnapshotArtifactExactRefV2) (contract.CheckpointPhaseDomainResultV2, error)
	InspectCheckpointPhaseDomainResultByIDV2(context.Context, string) (contract.CheckpointPhaseDomainResultV2, error)
	InspectCheckpointPhaseDomainResultByRefV2(context.Context, contract.Ref) (contract.CheckpointPhaseDomainResultV2, error)
	InspectCheckpointPhaseDomainResultByReservationV2(context.Context, contract.Ref) (contract.CheckpointPhaseDomainResultV2, error)
	CommitCheckpointPhaseApplySettlementV2(context.Context, contract.Ref, contract.CheckpointPhaseFact, contract.CheckpointParticipantFact) (bool, error)
}

type CheckpointPhaseResultCurrentReaderV2 interface {
	InspectCheckpointPhaseResultCurrentV2(context.Context, contract.Ref) (contract.CheckpointPhaseResultCurrentProjectionV2, error)
}

type CheckpointPhaseSettlementCurrentReaderV2 interface {
	InspectCheckpointPhaseSettlementCurrentV2(context.Context, contract.SnapshotArtifactExactRefV2, contract.Ref) (contract.CheckpointPhaseSettlementCurrentProjectionV2, error)
}

type CheckpointPhaseResultOwnerPortV2 interface {
	RecordCheckpointPhaseDomainResultV2(context.Context, *contract.RecordCheckpointPhaseDomainResultRequestV2) (contract.CheckpointPhaseDomainResultV2, error)
	InspectCheckpointPhaseDomainResultV2(context.Context, *contract.SnapshotArtifactExactRefV2) (contract.CheckpointPhaseDomainResultV2, error)
	ApplyCheckpointPhaseSettlementV2(context.Context, *contract.CheckpointPhaseApplySettlementV2) (contract.CheckpointPhaseFact, error)
}
