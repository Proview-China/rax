package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
)

// SnapshotArtifactStoreV2 is the SnapshotArtifact Owner's durable, atomic
// persistence boundary. It is intentionally separate from the public
// Reserve/Inspect port: callers cannot mint Facts or move aggregate current.
// Implementations must preserve append-only history and update the current
// pointer in the same transaction as the new immutable objects.
type SnapshotArtifactStoreV2 interface {
	CreateReservedSnapshotArtifact(context.Context, contract.SnapshotArtifactReservedBundleV2) (bool, error)
	CommitAvailableSnapshotArtifact(context.Context, contract.SnapshotArtifactAvailableBundleV2) (bool, error)
	InspectSnapshotArtifactReservation(context.Context, contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactReservationV2, error)
	InspectSnapshotArtifactReservationByStableKey(context.Context, contract.SnapshotArtifactStableSourceKeyV2) (contract.SnapshotArtifactReservationV2, error)
	InspectSnapshotArtifactReservationFact(context.Context, contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactReservationFactV2, error)
	InspectSnapshotArtifactEntry(context.Context, contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactAggregateEntryV2, error)
	InspectSnapshotArtifactEnvelope(context.Context, contract.SnapshotArtifactAggregateRefV2) (contract.SnapshotArtifactAggregateEnvelopeV2, error)
	InspectSnapshotArtifactCurrentIndex(context.Context, string) (contract.SnapshotArtifactAggregateCurrentIndexV2, error)
	InspectSnapshotArtifactFact(context.Context, contract.SnapshotArtifactExactRefV2) (contract.SnapshotArtifactFactV2, error)
}

// SnapshotArtifactOwnerPortV2 is the complete public surface of the first
// owner-local slice. It deliberately exposes no raw CAS, Apply, Evidence,
// Settlement, Provider, Retention, deletion, or backend operation.
type SnapshotArtifactOwnerPortV2 interface {
	ReserveArtifact(context.Context, *contract.ReserveArtifactRequestV2) (contract.ReserveArtifactResultV2, error)
	CommitArtifact(context.Context, *contract.CommitSnapshotArtifactRequestV2) (contract.CommitSnapshotArtifactResultV2, error)
	InspectReservation(context.Context, *contract.InspectSnapshotArtifactReservationRequestV2) (contract.SnapshotArtifactReservationV2, error)
	InspectReservationByStableKey(context.Context, *contract.InspectSnapshotArtifactReservationByStableKeyRequestV2) (contract.SnapshotArtifactReservationV2, error)
	InspectAggregateHistorical(context.Context, *contract.InspectSnapshotArtifactAggregateHistoricalRequestV2) (contract.SnapshotArtifactAggregateEnvelopeV2, error)
	InspectAggregateCurrent(context.Context, *contract.InspectSnapshotArtifactAggregateCurrentRequestV2) (contract.SnapshotArtifactAggregateCurrentProjectionV2, error)
	InspectEntryHistorical(context.Context, *contract.InspectSnapshotArtifactEntryHistoricalRequestV2) (contract.SnapshotArtifactAggregateEntryV2, error)
	InspectArtifactFact(context.Context, *contract.InspectSnapshotArtifactFactRequestV2) (contract.SnapshotArtifactFactV2, error)
}

// SnapshotArtifactCommitCurrentReaderV2 aggregates exact current coordinates
// from their semantic Owners. It never writes an Artifact Fact.
type SnapshotArtifactCommitCurrentReaderV2 interface {
	InspectSnapshotArtifactCommitCurrentV2(context.Context, contract.CommitSnapshotArtifactRequestV2) (contract.SnapshotArtifactCommitCurrentProjectionV2, error)
}

// SnapshotContentStoreV2 persists encrypted snapshot bytes and exposes only
// opaque exact storage refs. It owns no Checkpoint, Runtime, or Artifact Fact.
type SnapshotContentStoreV2 interface {
	PutSnapshotContentV2(context.Context, *contract.PutSnapshotContentRequestV2) (contract.PutSnapshotContentResultV2, error)
	InspectSnapshotContentV2(context.Context, *contract.InspectSnapshotContentRequestV2) (contract.InspectSnapshotContentResultV2, error)
}
