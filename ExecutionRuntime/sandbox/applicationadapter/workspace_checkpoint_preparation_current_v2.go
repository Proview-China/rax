package applicationadapter

import (
	"context"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

// WorkspaceCheckpointPreparationCurrentReaderV2 rebuilds the workspace
// Participant candidate from Sandbox Owner current facts. Capture plans and
// Provider replies are never accepted directly: the Reader follows the
// Artifact Fact to its Reservation, re-reads the immutable capture binding,
// the applied prepare phase, and the aggregate current pointer.
type WorkspaceCheckpointPreparationCurrentReaderV2 struct {
	composition *CheckpointProductionCompositionV2
	bindings    CheckpointSnapshotCaptureBindingStoreV2
	artifacts   ports.SnapshotArtifactStoreV2
	clock       func() time.Time
}

func NewWorkspaceCheckpointPreparationCurrentReaderV2(composition *CheckpointProductionCompositionV2, bindings CheckpointSnapshotCaptureBindingStoreV2, artifacts ports.SnapshotArtifactStoreV2, clock func() time.Time) (*WorkspaceCheckpointPreparationCurrentReaderV2, error) {
	if composition == nil || composition.Store == nil || nilLike(bindings) || nilLike(artifacts) || clock == nil {
		return nil, fmt.Errorf("workspace checkpoint preparation current Reader dependencies are required")
	}
	return &WorkspaceCheckpointPreparationCurrentReaderV2{composition: composition, bindings: bindings, artifacts: artifacts, clock: clock}, nil
}

func (r *WorkspaceCheckpointPreparationCurrentReaderV2) InspectWorkspaceCheckpointPreparationCurrentV2(ctx context.Context, request contract.PrepareWorkspaceCheckpointParticipantRequestV2) (contract.WorkspaceCheckpointPreparationCurrentProjectionV2, error) {
	now := r.clock()
	if err := request.ValidateCurrent(now); err != nil {
		return contract.WorkspaceCheckpointPreparationCurrentProjectionV2{}, err
	}
	fact, err := r.artifacts.InspectSnapshotArtifactFact(ctx, request.SnapshotArtifactFactRef)
	if err != nil || fact.ValidateCurrent(now) != nil || fact.DataDomain != contract.WorkspaceSnapshotDataDomain || !contract.SameSnapshotArtifactExactRef(fact.ExactRef(), request.SnapshotArtifactFactRef) {
		return contract.WorkspaceCheckpointPreparationCurrentProjectionV2{}, fmt.Errorf("%w: workspace Snapshot Artifact Fact is not exact current", ports.ErrStale)
	}
	reservationFact, err := r.artifacts.InspectSnapshotArtifactReservationFact(ctx, fact.ReservationFactRef)
	if err != nil || reservationFact.ValidateShape() != nil || reservationFact.Meta.ValidateCurrent(now) != nil || reservationFact.TenantID != request.TenantID {
		return contract.WorkspaceCheckpointPreparationCurrentProjectionV2{}, fmt.Errorf("%w: workspace Snapshot Reservation Fact is unavailable", ports.ErrStale)
	}
	binding, err := r.bindings.InspectCheckpointSnapshotCaptureBindingV2(ctx, reservationFact.ReservationRef)
	if err != nil || binding.ValidateCurrent(now) != nil {
		return contract.WorkspaceCheckpointPreparationCurrentProjectionV2{}, fmt.Errorf("%w: workspace checkpoint capture binding is unavailable", ports.ErrStale)
	}
	if binding.TenantID != request.TenantID || binding.ScopeDigest != request.ScopeDigest || binding.RunID != request.RunID || !contract.SameRef(binding.CheckpointAttemptRef, request.CheckpointAttemptRef) || !contract.SameRef(binding.BarrierRef, request.BarrierRef) || !contract.SameRef(binding.EffectCutRef, request.EffectCutRef) || binding.ParticipantID != request.ParticipantID || binding.ParticipantDigest != request.ParticipantDigest || binding.WorkspaceStableID != request.StableID || !contract.SameRef(binding.CoveragePolicyRef, request.CoveragePolicyRef) || !contract.SameRef(binding.SourceAttemptRef, request.CheckpointAttemptRef) || !contract.SameSnapshotArtifactExactRef(binding.StorageArtifactRef.ExactRef(), fact.StorageArtifactRef.ExactRef()) || !contract.SameRef(binding.ProviderObservationRef, fact.ProviderObservationRef) || !contract.SameRef(binding.ProviderReceiptRef, fact.ProviderReceiptRef) || len(fact.FormalEvidenceRefs) != 1 || !contract.SameRef(binding.EvidenceConsumptionRef, fact.FormalEvidenceRefs[0]) {
		return contract.WorkspaceCheckpointPreparationCurrentProjectionV2{}, fmt.Errorf("%w: workspace checkpoint capture binding crossed Owner facts", ports.ErrConflict)
	}
	phase, err := r.composition.Store.InspectCheckpointPhaseFactByReservation(ctx, binding.CheckpointReservation)
	if err != nil || phase.ValidateCurrent(now) != nil || phase.Phase != contract.CheckpointPhasePrepare || phase.State != contract.CheckpointPhasePrepared || !contract.SameRef(phase.Meta.Ref(), request.PreparedPhaseFactRef) || !contract.SameRef(phase.CheckpointAttemptRef, request.CheckpointAttemptRef) || phase.ParticipantRef.ID != request.ParticipantID {
		return contract.WorkspaceCheckpointPreparationCurrentProjectionV2{}, fmt.Errorf("%w: workspace checkpoint prepare phase is not exact current", ports.ErrConflict)
	}
	index, err := r.artifacts.InspectSnapshotArtifactCurrentIndex(ctx, fact.ArtifactSubjectRef.ArtifactAggregateID)
	if err != nil || index.ValidateCurrent(now) != nil || index.AggregateState != contract.SnapshotArtifactAggregateAvailable || index.ArtifactFactRef.Ref == nil || !contract.SameSnapshotArtifactExactRef(*index.ArtifactFactRef.Ref, fact.ExactRef()) {
		return contract.WorkspaceCheckpointPreparationCurrentProjectionV2{}, fmt.Errorf("%w: workspace Snapshot aggregate is not exact available current", ports.ErrStale)
	}
	fresh := r.clock()
	expires := minimumInt64(request.RequestedNotAfter, binding.ExpiresUnixNano, phase.Meta.ExpiresUnixNano, fact.Meta.ExpiresUnixNano, index.CurrentIndexRef.ExpiresUnixNano, index.HeadAggregateEnvelopeRef.ExpiresUnixNano)
	projection, err := contract.SealWorkspaceCheckpointPreparationCurrentProjectionV2(contract.WorkspaceCheckpointPreparationCurrentProjectionV2{
		TenantID: request.TenantID, ScopeDigest: request.ScopeDigest, RunID: request.RunID,
		CheckpointAttemptRef: request.CheckpointAttemptRef, BarrierRef: request.BarrierRef, EffectCutRef: request.EffectCutRef,
		ParticipantID: request.ParticipantID, ParticipantDigest: request.ParticipantDigest, PreparedPhaseFactRef: request.PreparedPhaseFactRef,
		SnapshotArtifactFactRef: fact.ExactRef(), SnapshotAggregateRef: index.HeadAggregateEnvelopeRef, CoveragePolicyRef: binding.CoveragePolicyRef,
		Included: append([]string(nil), binding.Included...), DeclaredExcluded: append([]string(nil), binding.DeclaredExcluded...), ResidualRefs: append([]contract.Ref(nil), binding.ResidualRefs...),
		CheckedUnixNano: fresh.UnixNano(), ExpiresUnixNano: expires,
	}, fresh)
	if err != nil || !projection.MatchesRequest(request) {
		return contract.WorkspaceCheckpointPreparationCurrentProjectionV2{}, fmt.Errorf("%w: workspace checkpoint preparation projection is invalid", ports.ErrConflict)
	}
	return projection, nil
}

var _ ports.WorkspaceCheckpointPreparationCurrentReaderV2 = (*WorkspaceCheckpointPreparationCurrentReaderV2)(nil)
