package fakes

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func operationSettlementGuardKeyV4(tenantID core.TenantID, effectID core.EffectIntentID) string {
	return string(tenantID) + "\x00" + string(effectID)
}

func operationSettlementIDKeyV4(tenantID core.TenantID, settlementID string) string {
	return string(tenantID) + "\x00" + settlementID
}

const (
	operationSettlementTerminalVersionV3 = "v3"
	operationSettlementTerminalVersionV4 = "v4"
)

type operationSettlementTerminalGuardOwner struct {
	Version         string
	SettlementID    string
	OperationDigest core.Digest
}

func (s *OperationEffectStoreV3) LoseNextOperationSettlementV4Reply() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loseNextSettlementV4Reply = true
}

func (s *OperationEffectStoreV3) FailNextOperationSettlementV4Commit() {
	s.FailNextOperationSettlementV4CommitAfterStage(1)
}

// FailNextOperationSettlementV4CommitAfterStage injects a fixture-only failure
// after staging one of the four terminal objects, or after staging their
// indexes (stage 5). Staged copies are never published on failure.
func (s *OperationEffectStoreV3) FailNextOperationSettlementV4CommitAfterStage(stage int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if stage < 1 || stage > 5 {
		stage = 1
	}
	s.failNextSettlementV4Stage = stage
}

func (s *OperationEffectStoreV3) OperationSettlementV4CommitCount() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settlementV4CommitCount
}

// ReplaceOperationSettlementCurrentIndexForTestV4 is a fixture-only corruption
// hook for proving that historical object reads never borrow the mutable
// current-by-Effect index. It is absent from every public port.
func (s *OperationEffectStoreV3) ReplaceOperationSettlementCurrentIndexForTestV4(tenantID core.TenantID, effectID core.EffectIntentID, bundle ports.OperationSettlementCommitBundleV4) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settlementsV4ByEffect[operationSettlementGuardKeyV4(tenantID, effectID)] = cloneOSE(bundle)
	return nil
}

// InstallOperationEffectFactForTestV3 is a fixture-only helper for exercising
// same-owner partition behavior. It is deliberately absent from every public
// Fact/Governance port and makes no production admission claim.
func (s *OperationEffectStoreV3) InstallOperationEffectFactForTestV3(fact control.OperationEffectFactV3) error {
	if err := fact.Validate(); err != nil {
		return err
	}
	key, err := operationKeyV3(fact.Intent.Operation)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.effects[key] == nil {
		s.effects[key] = map[core.EffectIntentID]control.OperationEffectFactV3{}
	}
	if _, exists := s.effects[key][fact.Intent.ID]; exists {
		return core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "fixture operation Effect already exists")
	}
	s.effects[key][fact.Intent.ID] = cloneOperationEffectFactV3(fact)
	return nil
}

func (s *OperationEffectStoreV3) CommitOperationSettlementV4(ctx context.Context, request ports.OperationSettlementCommitRequestV4) (ports.OperationSettlementCommitBundleV4, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	expected, err := control.BuildOperationSettlementCommitBundleV4(request.Bundle.Settlement.Submission)
	if err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	expectedDigest, err := control.OperationSettlementCommitBundleDigestV4(expected)
	if err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	requestDigest, err := control.OperationSettlementCommitBundleDigestV4(request.Bundle)
	if err != nil || requestDigest != expectedDigest {
		return ports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "V4 settlement owner received a non-derived terminal bundle")
	}
	submission := expected.Settlement.Submission
	operationKey, err := operationKeyV3(submission.Operation)
	if err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	idKey := operationSettlementIDKeyV4(submission.TenantID, submission.ID)
	guardKey := operationSettlementGuardKeyV4(submission.TenantID, submission.EffectID)

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.settlementsV4ByID[idKey]; ok {
		existingDigest, digestErr := control.OperationSettlementCommitBundleDigestV4(existing)
		if digestErr == nil && existingDigest == expectedDigest {
			return cloneOSE(existing), nil
		}
		return ports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "V4 settlement ID binds different canonical content")
	}
	if s.v3SettlementIDExistsLocked(submission.TenantID, submission.ID) {
		return ports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "settlement ID already belongs to V3")
	}
	if existing, ok := s.settlementTerminalGuards[guardKey]; ok {
		if existing.SettlementID == submission.ID {
			return ports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "V4 terminal guard content conflicts")
		}
		return ports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "operation Effect terminal guard is occupied")
	}
	effect, ok := s.effects[operationKey][submission.EffectID]
	if !ok {
		return ports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorNotFound, core.ReasonEffectIntentMissing, "operation Effect not found for V4 settlement")
	}
	if err := effect.Validate(); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	if effect.Revision != request.ExpectedEffectRevision || effect.Revision != submission.ExpectedEffectRevision || (effect.State != control.OperationEffectDispatchedV3 && effect.State != control.OperationEffectUnknownOutcomeV3) || effect.Settlement != nil {
		return ports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "operation Effect is not at the exact V4 settlement watermark")
	}
	if !ports.SameOperationSubjectV3(effect.Intent.Operation, submission.Operation) || effect.Intent.ID != submission.EffectID || effect.Intent.Revision != submission.DomainResult.EffectRevision || effect.Intent.Operation.ExecutionScope.Identity.TenantID != submission.TenantID {
		return ports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "V4 settlement belongs to another operation Effect")
	}
	ownerMatched := false
	for _, owner := range effect.Intent.Owners {
		if owner == submission.Owner && owner.Role == ports.OwnerSettlement {
			ownerMatched = true
			break
		}
	}
	if !ownerMatched {
		return ports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorForbidden, core.ReasonSettlementOwnerMismatch, "V4 settlement Owner does not own this Effect")
	}
	if submission.ExpectedTerminalGuardRevision != 0 {
		return ports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "new V4 settlement requires an absent terminal guard")
	}
	// The reference owner uses copy-on-write staging so fixture fault injection
	// can prove that none of the four objects or their indexes become visible
	// before the single publish boundary.
	staged := ports.OperationSettlementCommitBundleV4{}
	staged.Settlement = cloneOSE(expected.Settlement)
	if err := s.failOperationSettlementV4StageLocked(1); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	staged.Association = cloneOSE(expected.Association)
	if err := s.failOperationSettlementV4StageLocked(2); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	staged.Guard = cloneOSE(expected.Guard)
	if err := s.failOperationSettlementV4StageLocked(3); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	staged.Projection = cloneOSE(expected.Projection)
	if err := s.failOperationSettlementV4StageLocked(4); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	if err := staged.Validate(); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	operationIndex := make(map[string]ports.OperationSettlementCommitBundleV4, len(s.settlementsV4[operationKey])+1)
	for key, value := range s.settlementsV4[operationKey] {
		operationIndex[key] = value
	}
	byEffect := make(map[string]ports.OperationSettlementCommitBundleV4, len(s.settlementsV4ByEffect)+1)
	for key, value := range s.settlementsV4ByEffect {
		byEffect[key] = value
	}
	byID := make(map[string]ports.OperationSettlementCommitBundleV4, len(s.settlementsV4ByID)+1)
	for key, value := range s.settlementsV4ByID {
		byID[key] = value
	}
	terminalGuards := make(map[string]operationSettlementTerminalGuardOwner, len(s.settlementTerminalGuards)+1)
	for key, value := range s.settlementTerminalGuards {
		terminalGuards[key] = value
	}
	committed := cloneOSE(staged)
	operationIndex[submission.ID] = committed
	byEffect[guardKey] = committed
	byID[idKey] = committed
	terminalGuards[guardKey] = operationSettlementTerminalGuardOwner{
		Version:         operationSettlementTerminalVersionV4,
		SettlementID:    submission.ID,
		OperationDigest: submission.OperationDigest,
	}
	if err := s.failOperationSettlementV4StageLocked(5); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	s.settlementsV4[operationKey] = operationIndex
	s.settlementsV4ByEffect = byEffect
	s.settlementsV4ByID = byID
	s.settlementTerminalGuards = terminalGuards
	s.settlementV4CommitCount++
	if s.loseNextSettlementV4Reply {
		s.loseNextSettlementV4Reply = false
		return ports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected V4 settlement commit reply loss")
	}
	return cloneOSE(committed), nil
}

func (s *OperationEffectStoreV3) failOperationSettlementV4StageLocked(stage int) error {
	if s.failNextSettlementV4Stage != stage {
		return nil
	}
	s.failNextSettlementV4Stage = 0
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "injected V4 settlement staged publish failure")
}

func (s *OperationEffectStoreV3) InspectOperationSettlementV4(ctx context.Context, operation ports.OperationSubjectV3, settlementID string) (ports.OperationSettlementCommitBundleV4, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	if err := operation.Validate(); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	if settlementID == "" {
		return ports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "V4 settlement ID is required")
	}
	key, err := operationKeyV3(operation)
	if err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bundle, ok := s.settlementsV4[key][settlementID]
	if !ok {
		return ports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "V4 settlement not found")
	}
	if err := bundle.Validate(); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	return cloneOSE(bundle), nil
}

func (s *OperationEffectStoreV3) InspectOperationSettlementByEffectV4(ctx context.Context, operation ports.OperationSubjectV3, effectID core.EffectIntentID) (ports.OperationSettlementCommitBundleV4, error) {
	if err := contextError(ctx); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	if err := operation.Validate(); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	tenantID := operation.ExecutionScope.Identity.TenantID
	s.mu.Lock()
	defer s.mu.Unlock()
	bundle, ok := s.settlementsV4ByEffect[operationSettlementGuardKeyV4(tenantID, effectID)]
	if !ok || !ports.SameOperationSubjectV3(bundle.Settlement.Submission.Operation, operation) {
		return ports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorNotFound, core.ReasonEffectSettlementMissing, "V4 settlement not found for exact operation Effect")
	}
	if err := bundle.Validate(); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	return cloneOSE(bundle), nil
}

func (s *OperationEffectStoreV3) InspectOperationSettlementEvidenceAssociationV4(ctx context.Context, operation ports.OperationSubjectV3, ref ports.OperationSettlementEvidenceAssociationRefV4) (ports.OperationSettlementEvidenceAssociationV4, error) {
	bundle, err := s.InspectOperationSettlementV4(ctx, operation, ref.Settlement.ID)
	if err != nil {
		return ports.OperationSettlementEvidenceAssociationV4{}, err
	}
	if err := ref.Validate(); err != nil || !ports.SameOperationSettlementEvidenceAssociationRefV4(bundle.Association.RefV4(), ref) {
		return ports.OperationSettlementEvidenceAssociationV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 settlement association ref drifted")
	}
	return cloneOSE(bundle.Association), nil
}

func (s *OperationEffectStoreV3) InspectOperationSettlementTerminalGuardV4(ctx context.Context, operation ports.OperationSubjectV3, ref ports.OperationSettlementTerminalGuardRefV4) (ports.OperationSettlementTerminalGuardV4, error) {
	bundle, err := s.InspectOperationSettlementV4(ctx, operation, ref.Settlement.ID)
	if err != nil {
		return ports.OperationSettlementTerminalGuardV4{}, err
	}
	if err := ref.Validate(); err != nil || !ports.SameOperationSettlementTerminalGuardRefV4(bundle.Guard.RefV4(), ref) {
		return ports.OperationSettlementTerminalGuardV4{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "V4 terminal guard ref drifted")
	}
	return cloneOSE(bundle.Guard), nil
}

func (s *OperationEffectStoreV3) InspectOperationSettlementTerminalProjectionV4(ctx context.Context, operation ports.OperationSubjectV3, ref ports.OperationSettlementTerminalProjectionRefV4) (ports.OperationSettlementTerminalProjectionV4, error) {
	bundle, err := s.InspectOperationSettlementV4(ctx, operation, ref.Settlement.ID)
	if err != nil {
		return ports.OperationSettlementTerminalProjectionV4{}, err
	}
	if err := ref.Validate(); err != nil || !ports.SameOperationSettlementTerminalProjectionRefV4(bundle.Projection.RefV4(), ref) {
		return ports.OperationSettlementTerminalProjectionV4{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "V4 terminal projection ref drifted")
	}
	return cloneOSE(bundle.Projection), nil
}

func (s *OperationEffectStoreV3) v3SettlementIDExistsLocked(tenantID core.TenantID, settlementID string) bool {
	for _, partition := range s.effects {
		for _, effect := range partition {
			if effect.Intent.Operation.ExecutionScope.Identity.TenantID == tenantID && effect.Settlement != nil && effect.Settlement.ID == settlementID {
				return true
			}
		}
	}
	return false
}
