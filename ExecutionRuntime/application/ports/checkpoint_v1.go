package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// CheckpointGatePortV1 is the Harness-owned G7 Gate seam. Application can
// sequence it but cannot read or write the Harness Session Fact Store.
type CheckpointGatePortV1 interface {
	AcquireCheckpointGateV1(context.Context, contract.AcquireCheckpointGateRequestV1) (contract.CheckpointGateCommitV1, error)
	BindCheckpointGateRuntimeV1(context.Context, contract.BindCheckpointGateRuntimeRequestV1) (contract.CheckpointGateCommitV1, error)
	InspectCheckpointGateV1(context.Context, contract.CheckpointExternalExactRefV1) (contract.CheckpointGateCommitV1, error)
	InvalidateCheckpointGateV1(context.Context, contract.CheckpointExternalExactRefV1) (contract.CheckpointGateCommitV1, error)
	ReleaseCheckpointGateV1(context.Context, contract.CheckpointGateCommitV1, runtimeports.CheckpointAttemptRefV2) (contract.CheckpointGateCommitV1, error)
}

// CheckpointParticipantDriverV1 is implemented in each Participant Owner.
// Complete must run that Owner's Reservation/governance/Inspect/CAS chain;
// Application never accepts a caller-supplied closure as authoritative.
type CheckpointParticipantDriverV1 interface {
	CompleteCheckpointParticipantV1(context.Context, contract.CheckpointParticipantWorkRequestV1) (contract.CheckpointParticipantCommitV1, error)
	InspectCheckpointParticipantV1(context.Context, runtimeports.CheckpointAttemptRefV2, runtimeports.CheckpointParticipantRefV2) (contract.CheckpointParticipantCommitV1, error)
}

// CheckpointParticipantOwnerCurrentReaderV1 is implemented by each semantic
// Participant Owner. It returns only that Owner's sealed snapshot facts.
type CheckpointParticipantOwnerCurrentReaderV1 interface {
	InspectCheckpointParticipantOwnerCurrentV1(context.Context, contract.CheckpointParticipantWorkRequestV1) (contract.CheckpointParticipantOwnerCandidateV1, error)
}

// CheckpointParticipantPhaseCommitPortV1 is the public governed bridge. Its
// implementation must complete Runtime Evidence/Settlement and Owner
// ApplySettlement before returning a terminal Runtime closure.
type CheckpointParticipantPhaseCommitPortV1 interface {
	CommitCheckpointParticipantPhaseV1(context.Context, contract.CheckpointParticipantWorkRequestV1, contract.CheckpointParticipantOwnerCandidateV1) (contract.CheckpointParticipantCommitV1, error)
	InspectCheckpointParticipantPhaseV1(context.Context, runtimeports.CheckpointAttemptRefV2, runtimeports.CheckpointParticipantRefV2) (contract.CheckpointParticipantCommitV1, error)
}

// CheckpointManifestInputCurrentReaderV1 aggregates only exact current refs
// from their semantic Owners. It cannot create a Manifest or Participant Fact.
type CheckpointManifestInputCurrentReaderV1 interface {
	InspectCheckpointManifestInputCurrentV1(context.Context, runtimeports.CheckpointAttemptRefV2, runtimeports.CheckpointBarrierLeaseRefV2, runtimeports.EffectCutRefV2) (contract.CheckpointManifestInputCurrentProjectionV1, error)
}

// CheckpointManifestPortV1 is Continuity-owned. It creates/inspects only the
// Manifest and immutable Seal; it cannot commit Runtime Consistency.
type CheckpointManifestPortV1 interface {
	CreateCheckpointManifestSealV1(context.Context, contract.CreateCheckpointManifestSealRequestV1) (runtimeports.CheckpointManifestSealRefV2, error)
	InspectCheckpointManifestSealV1(context.Context, contract.InspectCheckpointManifestSealRequestV1) (runtimeports.CheckpointManifestSealRefV2, error)
}
