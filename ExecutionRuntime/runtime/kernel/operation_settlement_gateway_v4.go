package kernel

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationSettlementGatewayV4 is the only Application-facing path from two
// already-consumed OperationScope Evidence chains to Runtime terminal truth.
// It never calls a Provider and never mutates Evidence or DomainResult facts.
type OperationSettlementGatewayV4 struct {
	Facts       ports.OperationSettlementFactPortV4
	Effects     control.OperationEffectDispatchFactPortV4
	Evidence    ports.OperationSettlementEvidenceReaderV4
	Enforcement ports.OperationSettlementEnforcementReaderV4
	Domain      ports.OperationSettlementDomainResultCurrentReaderV4
	Clock       func() time.Time
}

func (g OperationSettlementGatewayV4) SettleOperationV4(ctx context.Context, submission ports.OperationSettlementSubmissionV4) (ports.OperationSettlementRefV4, error) {
	if err := g.validate(); err != nil {
		return ports.OperationSettlementRefV4{}, err
	}
	if err := submission.Validate(); err != nil {
		return ports.OperationSettlementRefV4{}, err
	}
	now := g.Clock()
	if now.IsZero() || submission.SettledUnixNano > now.UnixNano() || submission.SettledUnixNano < submission.DomainResult.AuthoritativeTime {
		return ports.OperationSettlementRefV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "V4 settlement time is future-dated or precedes DomainResult truth")
	}

	historical, err := g.Facts.InspectOperationSettlementV4(ctx, submission.Operation, submission.ID)
	if err == nil {
		if sameOperationSettlementSubmissionV4(historical.Settlement.Submission, submission) {
			return historical.Settlement.RefV4(), nil
		}
		return ports.OperationSettlementRefV4{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "V4 settlement ID already binds different content")
	}
	if !core.HasCategory(err, core.ErrorNotFound) {
		return ports.OperationSettlementRefV4{}, err
	}

	if err := g.validateCurrentInputs(ctx, submission, now); err != nil {
		return ports.OperationSettlementRefV4{}, err
	}
	bundle, err := control.BuildOperationSettlementCommitBundleV4(submission)
	if err != nil {
		return ports.OperationSettlementRefV4{}, err
	}
	// This second read is the last cross-Owner currentness boundary. The Fact
	// Owner then re-reads its own Effect/guard under the shared terminal lock.
	freshNow := g.Clock()
	if freshNow.IsZero() || freshNow.Before(now) || freshNow.UnixNano() < submission.SettledUnixNano {
		return ports.OperationSettlementRefV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "V4 settlement final currentness clock regressed")
	}
	if err := g.validateCurrentInputs(ctx, submission, freshNow); err != nil {
		return ports.OperationSettlementRefV4{}, err
	}
	stored, err := g.Facts.CommitOperationSettlementV4(ctx, ports.OperationSettlementCommitRequestV4{ExpectedEffectRevision: submission.ExpectedEffectRevision, Bundle: bundle})
	if err != nil {
		if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) && !core.HasCategory(err, core.ErrorConflict) {
			return ports.OperationSettlementRefV4{}, err
		}
		recovered, inspectErr := g.Facts.InspectOperationSettlementV4(context.WithoutCancel(ctx), submission.Operation, submission.ID)
		if inspectErr != nil || !sameOperationSettlementCommitBundleV4(recovered, bundle) {
			return ports.OperationSettlementRefV4{}, err
		}
		stored = recovered
	}
	if !sameOperationSettlementCommitBundleV4(stored, bundle) {
		return ports.OperationSettlementRefV4{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 settlement Owner returned another terminal bundle")
	}
	return stored.Settlement.RefV4(), nil
}

func (g OperationSettlementGatewayV4) InspectOperationSettlementV4(ctx context.Context, request ports.InspectOperationSettlementRequestV4) (ports.OperationSettlementFactV4, error) {
	if g.Facts == nil {
		return ports.OperationSettlementFactV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V4 settlement Fact Owner is required")
	}
	if err := request.Validate(); err != nil {
		return ports.OperationSettlementFactV4{}, err
	}
	bundle, err := g.Facts.InspectOperationSettlementV4(ctx, request.Operation, request.SettlementID)
	if err != nil {
		return ports.OperationSettlementFactV4{}, err
	}
	if err := bundle.Validate(); err != nil {
		return ports.OperationSettlementFactV4{}, err
	}
	if bundle.Settlement.Submission.ID != request.SettlementID || !ports.SameOperationSubjectV3(bundle.Settlement.Submission.Operation, request.Operation) {
		return ports.OperationSettlementFactV4{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "historical V4 settlement belongs to another operation")
	}
	return bundle.Settlement, nil
}

func (g OperationSettlementGatewayV4) InspectOperationSettlementClosureV4(ctx context.Context, request ports.InspectOperationSettlementRequestV4) (ports.OperationSettlementCommitBundleV4, error) {
	if g.Facts == nil {
		return ports.OperationSettlementCommitBundleV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V4 settlement Fact reader is required")
	}
	if err := request.Validate(); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	bundle, err := g.Facts.InspectOperationSettlementV4(ctx, request.Operation, request.SettlementID)
	if err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	if err := bundle.Validate(); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	association, err := g.InspectOperationSettlementEvidenceAssociationV4(ctx, request.Operation, bundle.Association.RefV4())
	if err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	guard, err := g.InspectOperationSettlementTerminalGuardV4(ctx, request.Operation, bundle.Guard.RefV4())
	if err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	projection, err := g.InspectOperationSettlementTerminalProjectionV4(ctx, request.Operation, bundle.Projection.RefV4())
	if err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	exact := ports.OperationSettlementCommitBundleV4{Settlement: bundle.Settlement, Association: association, Guard: guard, Projection: projection}
	if err := exact.Validate(); err != nil {
		return ports.OperationSettlementCommitBundleV4{}, err
	}
	return exact, nil
}

func (g OperationSettlementGatewayV4) InspectCurrentOperationSettlementV4(ctx context.Context, request ports.InspectCurrentOperationSettlementRequestV4) (ports.OperationInspectionSettlementRefV4, error) {
	if err := g.validate(); err != nil {
		return ports.OperationInspectionSettlementRefV4{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.OperationInspectionSettlementRefV4{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.OperationInspectionSettlementRefV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "V4 settlement Inspect clock returned zero")
	}
	bundle, err := g.Facts.InspectOperationSettlementByEffectV4(ctx, request.Operation, request.EffectID)
	if err != nil {
		return ports.OperationInspectionSettlementRefV4{}, err
	}
	if err := bundle.Validate(); err != nil {
		return ports.OperationInspectionSettlementRefV4{}, err
	}
	association, err := g.Facts.InspectOperationSettlementEvidenceAssociationV4(ctx, request.Operation, bundle.Association.RefV4())
	if err != nil || association.Digest != bundle.Association.Digest {
		return ports.OperationInspectionSettlementRefV4{}, ownerClosureErrorV4(err, "V4 settlement association is unavailable or drifted")
	}
	guard, err := g.Facts.InspectOperationSettlementTerminalGuardV4(ctx, request.Operation, bundle.Guard.RefV4())
	if err != nil || guard.Digest != bundle.Guard.Digest {
		return ports.OperationInspectionSettlementRefV4{}, ownerClosureErrorV4(err, "V4 terminal guard is unavailable or drifted")
	}
	projection, err := g.Facts.InspectOperationSettlementTerminalProjectionV4(ctx, request.Operation, bundle.Projection.RefV4())
	if err != nil || projection.Digest != bundle.Projection.Digest {
		return ports.OperationInspectionSettlementRefV4{}, ownerClosureErrorV4(err, "V4 terminal projection is unavailable or drifted")
	}
	submission := bundle.Settlement.Submission
	effect, err := g.Effects.InspectOperationEffectV3(ctx, request.Operation, request.EffectID)
	if err != nil {
		return ports.OperationInspectionSettlementRefV4{}, err
	}
	if err := validateOperationSettlementEffectV4(effect, submission); err != nil {
		return ports.OperationInspectionSettlementRefV4{}, err
	}
	domain, err := g.Domain.InspectOperationSettlementDomainResultCurrentV4(ctx, effect.Intent.Kind, submission.DomainResult)
	if err != nil {
		return ports.OperationInspectionSettlementRefV4{}, err
	}
	if err := domain.Validate(now); err != nil || domain.EffectKind != effect.Intent.Kind || !ports.SameOperationSettlementDomainResultFactRefV4(domain.Fact, submission.DomainResult) {
		return ports.OperationInspectionSettlementRefV4{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonSettlementOwnerMismatch, "V4 DomainResult is no longer current")
	}
	return ports.SealOperationInspectionSettlementRefV4(ports.OperationInspectionSettlementRefV4{
		Settlement: bundle.Settlement.RefV4(), Association: association.RefV4(), Guard: guard.RefV4(), Projection: projection.RefV4(),
		DomainResult: submission.DomainResult, EffectFactRevision: effect.Revision, Owner: submission.Owner,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: domain.ExpiresUnixNano,
	}, now)
}

func (g OperationSettlementGatewayV4) InspectOperationSettlementEvidenceAssociationV4(ctx context.Context, operation ports.OperationSubjectV3, ref ports.OperationSettlementEvidenceAssociationRefV4) (ports.OperationSettlementEvidenceAssociationV4, error) {
	if g.Facts == nil {
		return ports.OperationSettlementEvidenceAssociationV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V4 settlement Fact reader is required")
	}
	if err := validateOperationSettlementReadRefV4(operation, ref.OperationDigest, ref.EffectID); err != nil {
		return ports.OperationSettlementEvidenceAssociationV4{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.OperationSettlementEvidenceAssociationV4{}, err
	}
	value, err := g.Facts.InspectOperationSettlementEvidenceAssociationV4(ctx, operation, ref)
	if err != nil {
		return ports.OperationSettlementEvidenceAssociationV4{}, err
	}
	if err := value.Validate(); err != nil || !ports.SameOperationSettlementEvidenceAssociationRefV4(value.RefV4(), ref) {
		return ports.OperationSettlementEvidenceAssociationV4{}, ownerClosureErrorV4(err, "V4 settlement association reader returned another fact")
	}
	return value, nil
}

func (g OperationSettlementGatewayV4) InspectOperationSettlementTerminalGuardV4(ctx context.Context, operation ports.OperationSubjectV3, ref ports.OperationSettlementTerminalGuardRefV4) (ports.OperationSettlementTerminalGuardV4, error) {
	if g.Facts == nil {
		return ports.OperationSettlementTerminalGuardV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V4 settlement Fact reader is required")
	}
	if err := validateOperationSettlementReadRefV4(operation, ref.OperationDigest, ref.EffectID); err != nil {
		return ports.OperationSettlementTerminalGuardV4{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.OperationSettlementTerminalGuardV4{}, err
	}
	value, err := g.Facts.InspectOperationSettlementTerminalGuardV4(ctx, operation, ref)
	if err != nil {
		return ports.OperationSettlementTerminalGuardV4{}, err
	}
	if err := value.Validate(); err != nil || !ports.SameOperationSettlementTerminalGuardRefV4(value.RefV4(), ref) {
		return ports.OperationSettlementTerminalGuardV4{}, ownerClosureErrorV4(err, "V4 settlement guard reader returned another fact")
	}
	return value, nil
}

func (g OperationSettlementGatewayV4) InspectOperationSettlementTerminalProjectionV4(ctx context.Context, operation ports.OperationSubjectV3, ref ports.OperationSettlementTerminalProjectionRefV4) (ports.OperationSettlementTerminalProjectionV4, error) {
	if g.Facts == nil {
		return ports.OperationSettlementTerminalProjectionV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V4 settlement Fact reader is required")
	}
	if err := validateOperationSettlementReadRefV4(operation, ref.OperationDigest, ref.EffectID); err != nil {
		return ports.OperationSettlementTerminalProjectionV4{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.OperationSettlementTerminalProjectionV4{}, err
	}
	value, err := g.Facts.InspectOperationSettlementTerminalProjectionV4(ctx, operation, ref)
	if err != nil {
		return ports.OperationSettlementTerminalProjectionV4{}, err
	}
	if err := value.Validate(); err != nil || !ports.SameOperationSettlementTerminalProjectionRefV4(value.RefV4(), ref) {
		return ports.OperationSettlementTerminalProjectionV4{}, ownerClosureErrorV4(err, "V4 settlement projection reader returned another fact")
	}
	return value, nil
}

func validateOperationSettlementReadRefV4(operation ports.OperationSubjectV3, operationDigest core.Digest, effectID core.EffectIntentID) error {
	if err := operation.Validate(); err != nil {
		return err
	}
	digest, err := operation.DigestV3()
	if err != nil || digest != operationDigest || effectID == "" {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "V4 settlement read ref belongs to another operation Effect")
	}
	return nil
}

func (g OperationSettlementGatewayV4) validateCurrentInputs(ctx context.Context, submission ports.OperationSettlementSubmissionV4, now time.Time) error {
	effect, err := g.Effects.InspectOperationEffectV3(ctx, submission.Operation, submission.EffectID)
	if err != nil {
		return err
	}
	if err := validateOperationSettlementEffectV4(effect, submission); err != nil {
		return err
	}
	permitID := submission.Evidence[0].Attempt.PermitID
	permit, err := g.Effects.InspectOperationDispatchPermitV4(ctx, submission.Operation, permitID)
	if err != nil {
		return err
	}
	if err := permit.Validate(); err != nil {
		return err
	}
	if permit.State == ports.OperationPermitIssuedV4 || permit.Permit.LegacyPermit.ID != permitID || permit.Permit.LegacyPermit.IntentID != submission.EffectID || !ports.SameOperationSubjectV3(permit.Permit.LegacyPermit.Operation, submission.Operation) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "V4 settlement requires a historical begun dispatch")
	}
	legacyDigest, err := permit.Permit.LegacyPermit.DigestV3()
	if err != nil {
		return err
	}
	for _, binding := range submission.Evidence {
		if binding.Attempt.PermitID != permitID || binding.Attempt.PermitRevision != permit.Permit.LegacyPermit.Revision || binding.Attempt.PermitDigest != legacyDigest || binding.EnforcementPhase.PermitDigest != permit.PermitDigest || binding.EnforcementPhase.PermitFactRevision > permit.Revision {
			return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V4 settlement Evidence binds another historical Permit")
		}
		if err := g.validateEvidenceBinding(ctx, submission, effect.Intent.Kind, binding); err != nil {
			return err
		}
	}
	if submission.Evidence[0].Consumption == submission.Evidence[1].Consumption || submission.Evidence[0].Handoff == submission.Evidence[1].Handoff || !sameCanonicalSettlementV4("OperationDispatchAttemptRefV3", submission.Evidence[0].Attempt, submission.Evidence[1].Attempt) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "prepare and execute require independent Evidence consumption for one exact attempt")
	}
	domain, err := g.Domain.InspectOperationSettlementDomainResultCurrentV4(ctx, effect.Intent.Kind, submission.DomainResult)
	if err != nil {
		return err
	}
	if err := domain.Validate(now); err != nil || domain.EffectKind != effect.Intent.Kind || !ports.SameOperationSettlementDomainResultFactRefV4(domain.Fact, submission.DomainResult) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonSettlementOwnerMismatch, "V4 DomainResult current Reader returned another fact")
	}
	return nil
}

func (g OperationSettlementGatewayV4) validateEvidenceBinding(ctx context.Context, submission ports.OperationSettlementSubmissionV4, effectKind ports.EffectKindV2, binding ports.OperationSettlementEvidenceBindingV4) error {
	consumption, err := g.Evidence.InspectOperationScopeEvidenceConsumptionV3(ctx, binding.Consumption.ID)
	if err != nil {
		return err
	}
	if err := consumption.Validate(); err != nil || consumption.RefV3() != binding.Consumption || consumption.LateObservation || consumption.Qualification != binding.IssuedQualification || consumption.Handoff != binding.Handoff || consumption.Record != binding.Record || consumption.CandidateDigest != binding.CandidateDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 settlement consumption drifted")
	}
	final, err := g.Evidence.InspectOperationScopeEvidenceQualificationV3(ctx, binding.FinalQualification.ID)
	if err != nil {
		return err
	}
	if err := final.Validate(); err != nil || final.RefV3() != binding.FinalQualification || final.State != ports.OperationScopeEvidenceConsumedCurrentV3 || final.Consumption == nil || *final.Consumption != binding.Consumption {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "V4 settlement final qualification is not consumed_current")
	}
	issued := final
	issued.Revision = binding.IssuedQualification.Revision
	issued.State = ports.OperationScopeEvidenceIssuedV3
	issued.UpdatedUnixNano = issued.CreatedUnixNano
	issued.Consumption = nil
	issued.InvalidationReason = ""
	issued.Digest = ""
	issued, err = ports.SealOperationScopeEvidenceQualificationFactV3(issued)
	if err != nil || issued.RefV3() != binding.IssuedQualification {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 settlement issued qualification history drifted")
	}
	handoff, err := g.Evidence.InspectOperationScopeEvidenceProviderHandoffV3(ctx, binding.Handoff.ID)
	if err != nil {
		return err
	}
	if err := handoff.Validate(); err != nil || handoff.RefV3() != binding.Handoff || handoff.Qualification != binding.IssuedQualification || !sameCanonicalSettlementV4("OperationDispatchEnforcementPhaseRefV4", handoff.Phase, binding.EnforcementPhase) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 settlement Provider handoff drifted")
	}
	record, err := g.Evidence.InspectOperationScopeEvidenceRecordV3(ctx, binding.Record)
	if err != nil {
		return err
	}
	if err := record.Validate(); err != nil || record.Ref != binding.Record || record.CandidateDigest != binding.CandidateDigest || record.LateObservation || record.Candidate.Qualification != binding.IssuedQualification || record.Candidate.Source != final.Reservation.Source || record.Candidate.EventID != final.Reservation.EventID {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 settlement Evidence record or Candidate drifted")
	}
	scopeDigest, err := ports.DigestOperationSettlementEvidenceScopeV4(final.Scope)
	if err != nil || scopeDigest != binding.OperationScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceScopeConflict, "V4 settlement full OperationScope drifted")
	}
	if final.Scope.Operation.Kind != ports.OperationScopeActivationV3 || !ports.SameOperationSubjectV3(final.Scope.Operation, submission.Operation) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 settlement Evidence belongs to another operation subject")
	}
	if final.Scope.EffectID != submission.EffectID || final.Scope.EffectRevision != binding.Attempt.IntentRevision || final.Scope.EffectDigest != binding.Attempt.IntentDigest || final.Scope.EffectKind != effectKind {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 settlement Evidence belongs to another operation Effect")
	}
	if final.Scope.AttemptID != binding.Attempt.AttemptID || final.Scope.Phase != binding.Phase {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "V4 settlement Evidence belongs to another operation attempt")
	}
	if !operationSettlementApplicabilityForbiddenV4(final.Scope.Applicability) || !operationSettlementEffectKindAllowedV4(final.Scope.EffectKind) {
		return core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "V4 settlement is outside the activation first-slice matrix")
	}
	if final.Runtime.PermitID != binding.Attempt.PermitID || final.Runtime.PermitFactRevision != binding.EnforcementPhase.PermitFactRevision || final.Runtime.PermitDigest != binding.EnforcementPhase.PermitDigest || !sameCanonicalSettlementV4("OperationDispatchEnforcementPhaseRefV4", final.Runtime.Phase, binding.EnforcementPhase) {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V4 settlement runtime Evidence projection drifted")
	}
	journal, err := g.Enforcement.InspectOperationDispatchEnforcementV4(ctx, submission.Operation, submission.EffectID, binding.Attempt.PermitID)
	if err != nil {
		return err
	}
	if err := journal.Validate(); err != nil {
		return err
	}
	phaseRef, err := journal.PhaseRefV4(binding.Phase)
	if err != nil || !sameCanonicalSettlementV4("OperationDispatchEnforcementPhaseRefV4", phaseRef, binding.EnforcementPhase) {
		return core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V4 settlement Enforcement phase drifted")
	}
	return nil
}

func validateOperationSettlementEffectV4(effect control.OperationEffectFactV3, submission ports.OperationSettlementSubmissionV4) error {
	if err := effect.Validate(); err != nil {
		return err
	}
	if effect.Revision != submission.ExpectedEffectRevision || (effect.State != control.OperationEffectDispatchedV3 && effect.State != control.OperationEffectUnknownOutcomeV3) || effect.Settlement != nil || effect.Intent.ID != submission.EffectID || effect.Intent.Revision != submission.DomainResult.EffectRevision || !ports.SameOperationSubjectV3(effect.Intent.Operation, submission.Operation) || effect.Intent.Operation.ExecutionScope.Identity.TenantID != submission.TenantID {
		return core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "V4 settlement requires the exact non-terminal operation Effect")
	}
	for _, owner := range effect.Intent.Owners {
		if owner == submission.Owner && owner.Role == ports.OwnerSettlement {
			return nil
		}
	}
	return core.NewError(core.ErrorForbidden, core.ReasonSettlementOwnerMismatch, "V4 settlement Owner is not bound to the operation Effect")
}

func operationSettlementEffectKindAllowedV4(kind ports.EffectKindV2) bool {
	return kind == "praxis.sandbox/allocate" || kind == "praxis.sandbox/activate" || kind == "praxis.sandbox/open" || kind == "praxis.sandbox/inspect"
}

func operationSettlementApplicabilityForbiddenV4(values []ports.OperationScopeEvidenceApplicabilityV3) bool {
	values = ports.NormalizeOperationScopeEvidenceApplicabilityV3(values)
	expected := []ports.OperationScopeEvidenceApplicabilityDimensionV3{
		ports.OperationScopeEvidenceActionV3,
		ports.OperationScopeEvidenceContextV3,
		ports.OperationScopeEvidenceRunV3,
		ports.OperationScopeEvidenceSessionV3,
		ports.OperationScopeEvidenceTurnV3,
	}
	if len(values) != len(expected) {
		return false
	}
	for index := range expected {
		if values[index].Dimension != expected[index] || values[index].Mode != ports.OperationScopeEvidenceForbiddenV3 {
			return false
		}
	}
	return true
}

func sameOperationSettlementSubmissionV4(left, right ports.OperationSettlementSubmissionV4) bool {
	return left.Digest != "" && left.Digest == right.Digest
}

func sameOperationSettlementCommitBundleV4(left, right ports.OperationSettlementCommitBundleV4) bool {
	leftDigest, leftErr := control.OperationSettlementCommitBundleDigestV4(left)
	rightDigest, rightErr := control.OperationSettlementCommitBundleDigestV4(right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func sameCanonicalSettlementV4(typeName string, left, right any) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.runtime.operation-settlement", ports.OperationSettlementContractVersionV4, typeName, left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.runtime.operation-settlement", ports.OperationSettlementContractVersionV4, typeName, right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func ownerClosureErrorV4(err error, message string) error {
	if err != nil {
		return err
	}
	return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, message)
}

func (g OperationSettlementGatewayV4) validate() error {
	if g.Facts == nil || g.Effects == nil || g.Evidence == nil || g.Enforcement == nil || g.Domain == nil || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "V4 settlement requires Effect Owner, Evidence, Enforcement, DomainResult and clock readers")
	}
	return nil
}
