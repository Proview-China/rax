package fakes

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const operationSettlementTerminalVersionV5 = "v5"

func (s *OperationEffectStoreV3) LoseNextCheckpointSettlementV5Reply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextSettlementV5Reply = true
}

// FailNextCheckpointSettlementV5CommitAfterStage is a fixture-only logical
// transaction failpoint. The staged four-object closure is never published.
func (s *OperationEffectStoreV3) FailNextCheckpointSettlementV5CommitAfterStage(stage int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if stage < 1 || stage > 6 {
		stage = 1
	}
	s.failNextSettlementV5Stage = stage
}

func (s *OperationEffectStoreV3) CheckpointSettlementV5CommitCount() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settlementV5CommitCount
}

func (s *OperationEffectStoreV3) CommitCheckpointPhaseSettlementV5(ctx context.Context, bundle ports.OperationCheckpointRestoreSettlementCommitBundleV5) (ports.OperationCheckpointRestoreSettlementCommitBundleV5, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	if err := bundle.Validate(); err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	expected, err := control.BuildOperationCheckpointRestoreSettlementBundleV5(bundle.Submission)
	if err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	expectedDigest, _ := control.CheckpointCanonicalDigestV2("OperationCheckpointRestoreSettlementCommitBundleV5", expected)
	actualDigest, _ := control.CheckpointCanonicalDigestV2("OperationCheckpointRestoreSettlementCommitBundleV5", bundle)
	if expectedDigest != actualDigest {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "V5 settlement bundle is not Owner-derived")
	}
	submission := bundle.Submission
	operationKey, err := operationKeyV3(submission.Operation)
	if err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	guardKey := operationSettlementGuardKeyV4(submission.Operation.ExecutionScope.Identity.TenantID, submission.EffectID)
	idKey := operationSettlementIDKeyV4(submission.Operation.ExecutionScope.Identity.TenantID, submission.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.settlementsV5ByID[idKey]; ok {
		existingDigest, _ := control.CheckpointCanonicalDigestV2("OperationCheckpointRestoreSettlementCommitBundleV5", existing)
		if existingDigest == expectedDigest {
			return cloneOSE(existing), nil
		}
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "V5 settlement ID binds different content")
	}
	if _, occupied := s.settlementTerminalGuards[guardKey]; occupied {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "operation Effect terminal guard is occupied")
	}
	effect, ok := s.effects[operationKey][submission.EffectID]
	if !ok {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, core.NewError(core.ErrorNotFound, core.ReasonEffectIntentMissing, "operation Effect not found for V5 settlement")
	}
	if err := effect.Validate(); err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	if effect.Revision != submission.ExpectedEffectRevision || (effect.State != control.OperationEffectDispatchedV3 && effect.State != control.OperationEffectUnknownOutcomeV3) || effect.Settlement != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "operation Effect is not at the exact V5 terminal watermark")
	}
	if !ports.SameOperationSubjectV3(effect.Intent.Operation, submission.Operation) || effect.Intent.ID != submission.EffectID {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, core.NewError(core.ErrorForbidden, core.ReasonSettlementOwnerMismatch, "V5 settlement belongs to another operation Effect")
	}
	ownerMatched := false
	for _, owner := range effect.Intent.Owners {
		if owner.Role == ports.OwnerSettlement && owner.ComponentID == submission.Owner.ComponentID && owner.ManifestDigest == submission.Owner.ManifestDigest {
			ownerMatched = true
			break
		}
	}
	if !ownerMatched {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, core.NewError(core.ErrorForbidden, core.ReasonSettlementOwnerMismatch, "V5 settlement Owner does not own this Effect")
	}
	staged := ports.OperationCheckpointRestoreSettlementCommitBundleV5{}
	staged.Submission = cloneOSE(expected.Submission)
	staged.Settlement = cloneOSE(expected.Settlement)
	if err := s.failCheckpointSettlementV5StageLocked(1); err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	staged.Association = cloneOSE(expected.Association)
	if err := s.failCheckpointSettlementV5StageLocked(2); err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	staged.Guard = cloneOSE(expected.Guard)
	if err := s.failCheckpointSettlementV5StageLocked(3); err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	staged.Projection = cloneOSE(expected.Projection)
	if err := s.failCheckpointSettlementV5StageLocked(4); err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	staged.EffectTerminal = cloneOSE(expected.EffectTerminal)
	if err := s.failCheckpointSettlementV5StageLocked(5); err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	if err := staged.Validate(); err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	if err := s.failCheckpointSettlementV5StageLocked(6); err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	s.settlementsV5ByID[idKey] = cloneOSE(staged)
	s.settlementsV5ByEffect[guardKey] = cloneOSE(staged)
	s.settlementTerminalGuards[guardKey] = operationSettlementTerminalGuardOwner{Version: operationSettlementTerminalVersionV5, SettlementID: submission.ID, OperationDigest: submission.OperationDigest}
	s.terminalEffectsV5[guardKey] = cloneOSE(staged.EffectTerminal)
	s.settlementV5CommitCount++
	if s.loseNextSettlementV5Reply {
		s.loseNextSettlementV5Reply = false
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected V5 settlement reply loss")
	}
	return cloneOSE(staged), nil
}

func (s *OperationEffectStoreV3) failCheckpointSettlementV5StageLocked(stage int) error {
	if s.failNextSettlementV5Stage != stage {
		return nil
	}
	s.failNextSettlementV5Stage = 0
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected V5 settlement staged failure")
}

func (s *OperationEffectStoreV3) InspectCheckpointPhaseSettlementHistoricalV5(ctx context.Context, request ports.InspectOperationCheckpointRestoreSettlementRequestV5) (ports.OperationCheckpointRestoreSettlementCommitBundleV5, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	if err := request.Operation.Validate(); err != nil {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, err
	}
	if request.SettlementID == "" {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "V5 settlement ID is required")
	}
	key := operationSettlementIDKeyV4(request.Operation.ExecutionScope.Identity.TenantID, request.SettlementID)
	s.mu.Lock()
	defer s.mu.Unlock()
	bundle, ok := s.settlementsV5ByID[key]
	if !ok {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "V5 settlement not found")
	}
	if !ports.SameOperationSubjectV3(bundle.Submission.Operation, request.Operation) {
		return ports.OperationCheckpointRestoreSettlementCommitBundleV5{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "V5 settlement operation drifted")
	}
	return cloneOSE(bundle), nil
}

func (s *OperationEffectStoreV3) InspectCheckpointPhaseSettlementCurrentV5(ctx context.Context, request ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5) (ports.OperationCheckpointRestoreSettlementInspectionV5, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationCheckpointRestoreSettlementInspectionV5{}, err
	}
	if err := request.Operation.Validate(); err != nil {
		return ports.OperationCheckpointRestoreSettlementInspectionV5{}, err
	}
	if request.EffectID == "" {
		return ports.OperationCheckpointRestoreSettlementInspectionV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "V5 Effect ID is required")
	}
	key := operationSettlementGuardKeyV4(request.Operation.ExecutionScope.Identity.TenantID, request.EffectID)
	s.mu.Lock()
	defer s.mu.Unlock()
	bundle, ok := s.settlementsV5ByEffect[key]
	if !ok {
		return ports.OperationCheckpointRestoreSettlementInspectionV5{}, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "current V5 settlement not found")
	}
	if !ports.SameOperationSubjectV3(bundle.Submission.Operation, request.Operation) {
		return ports.OperationCheckpointRestoreSettlementInspectionV5{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "current V5 settlement operation drifted")
	}
	return ports.OperationCheckpointRestoreSettlementInspectionV5{Bundle: cloneOSE(bundle), Current: true, CheckedUnixNano: s.clock().UnixNano()}, nil
}

func (s *OperationEffectStoreV3) InspectCheckpointPhaseSettlementAssociationV5(ctx context.Context, operation ports.OperationSubjectV3, ref ports.OperationCheckpointRestoreSettlementAssociationRefV5) (ports.OperationCheckpointRestoreSettlementAssociationV5, error) {
	bundle, err := s.InspectCheckpointPhaseSettlementHistoricalV5(ctx, ports.InspectOperationCheckpointRestoreSettlementRequestV5{Operation: operation, SettlementID: ref.Settlement.ID})
	if err != nil {
		return ports.OperationCheckpointRestoreSettlementAssociationV5{}, err
	}
	if bundle.Association.Ref != ref {
		return ports.OperationCheckpointRestoreSettlementAssociationV5{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "V5 association ref drifted")
	}
	return bundle.Association, nil
}

func (s *OperationEffectStoreV3) InspectCheckpointPhaseTerminalGuardV5(ctx context.Context, operation ports.OperationSubjectV3, ref ports.OperationCheckpointRestoreTerminalGuardRefV5) (ports.OperationCheckpointRestoreTerminalGuardV5, error) {
	bundle, err := s.InspectCheckpointPhaseSettlementHistoricalV5(ctx, ports.InspectOperationCheckpointRestoreSettlementRequestV5{Operation: operation, SettlementID: ref.Settlement.ID})
	if err != nil {
		return ports.OperationCheckpointRestoreTerminalGuardV5{}, err
	}
	if bundle.Guard.Ref != ref {
		return ports.OperationCheckpointRestoreTerminalGuardV5{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "V5 terminal guard ref drifted")
	}
	return bundle.Guard, nil
}

func (s *OperationEffectStoreV3) InspectCheckpointPhaseTerminalProjectionV5(ctx context.Context, operation ports.OperationSubjectV3, ref ports.OperationCheckpointRestoreTerminalProjectionRefV5) (ports.OperationCheckpointRestoreTerminalProjectionV5, error) {
	bundle, err := s.InspectCheckpointPhaseSettlementHistoricalV5(ctx, ports.InspectOperationCheckpointRestoreSettlementRequestV5{Operation: operation, SettlementID: ref.Settlement.ID})
	if err != nil {
		return ports.OperationCheckpointRestoreTerminalProjectionV5{}, err
	}
	if bundle.Projection.Ref != ref {
		return ports.OperationCheckpointRestoreTerminalProjectionV5{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "V5 terminal projection ref drifted")
	}
	return bundle.Projection, nil
}
