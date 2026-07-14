package control

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationSettlementGovernanceGatewayV3 accepts only an exact Settlement
// Owner submission. Provider Observation remains evidence; it cannot settle
// or choose Runtime Outcome.
type OperationSettlementGovernanceGatewayV3 struct {
	Effects  OperationEffectFactPortV3
	Evidence ports.EvidenceRecordReaderV2
	Clock    func() time.Time
}

func (g OperationSettlementGovernanceGatewayV3) SettleOperationEffectV3(ctx context.Context, intent ports.OperationEffectIntentV3, submission ports.OperationSettlementSubmissionV3) (ports.OperationSettlementRefV3, error) {
	if g.Effects == nil || g.Evidence == nil || g.Clock == nil {
		return ports.OperationSettlementRefV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "settlement gateway requires Effect, Evidence and clock")
	}
	if err := intent.Validate(); err != nil {
		return ports.OperationSettlementRefV3{}, err
	}
	if err := submission.Validate(); err != nil {
		return ports.OperationSettlementRefV3{}, err
	}
	now := g.Clock()
	if now.IsZero() || submission.SettledUnixNano > now.UnixNano() {
		return ports.OperationSettlementRefV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "settlement time cannot be later than the injected current time")
	}
	effect, err := g.Effects.InspectOperationEffectV3(ctx, intent.Operation, intent.ID)
	if err != nil {
		return ports.OperationSettlementRefV3{}, err
	}
	intentDigest, _ := intent.DigestV3()
	if effect.IntentDigest != intentDigest || effect.State != OperationEffectDispatchedV3 && effect.State != OperationEffectUnknownOutcomeV3 && effect.State != OperationEffectSettledV3 {
		return ports.OperationSettlementRefV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "only dispatched or independently inspected unknown Effect may settle")
	}
	if submission.SettledUnixNano < effect.CreatedUnixNano || effect.Settlement == nil && submission.SettledUnixNano < effect.UpdatedUnixNano {
		return ports.OperationSettlementRefV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "settlement time regressed behind the governed Effect")
	}
	owner, found := exactOperationOwnerV3(intent.Owners, ports.OwnerSettlement)
	if !found || owner != submission.Owner {
		return ports.OperationSettlementRefV3{}, core.NewError(core.ErrorForbidden, core.ReasonSettlementOwnerMismatch, "submission is not from the bound Settlement Owner")
	}
	permit, err := g.Effects.InspectOperationDispatchPermitV3(ctx, intent.Operation, effect.DispatchPermitID)
	if err != nil {
		return ports.OperationSettlementRefV3{}, err
	}
	permitDigest, _ := permit.Permit.DigestV3()
	operationDigest, _ := intent.Operation.DigestV3()
	attempt := ports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: intent.ID, IntentRevision: intent.Revision, IntentDigest: intentDigest, PermitID: permit.Permit.ID, PermitRevision: permit.Permit.Revision, PermitDigest: permitDigest, AttemptID: permit.Permit.AttemptID}
	if effect.DispatchReceipt != nil {
		delegation := effect.DispatchReceipt.Delegation
		attempt.Delegation = &delegation
	}
	if !sameOperationDispatchAttemptV3(submission.Attempt, attempt) {
		return ports.OperationSettlementRefV3{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "settlement binds another Effect/Permit/attempt")
	}
	wasDispatched := effect.DispatchReceipt != nil
	if wasDispatched {
		if effect.DispatchReceipt == nil || submission.Observation == nil || effect.DispatchReceipt.Observation != *submission.Observation || submission.InspectionEffect != nil {
			return ports.OperationSettlementRefV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "dispatched settlement requires its exact provider Observation")
		}
	} else if err := g.validateUnknownInspection(ctx, effect, submission); err != nil {
		return ports.OperationSettlementRefV3{}, err
	}
	for _, ref := range submission.Evidence {
		record, inspectErr := g.Evidence.InspectRecord(ctx, ref)
		if inspectErr != nil {
			return ports.OperationSettlementRefV3{}, inspectErr
		}
		if err := ValidateEvidenceLedgerRecordV2(record); err != nil {
			return ports.OperationSettlementRefV3{}, err
		}
		if !ports.SameExecutionScopeV2(record.Candidate.ExecutionScope, intent.Operation.ExecutionScope) || record.Candidate.Producer.ComponentID != owner.ComponentID || record.Candidate.Producer.ManifestDigest != owner.ManifestDigest || record.Candidate.TrustClass != ports.EvidenceTrustAttestation && record.Candidate.TrustClass != ports.EvidenceTrustAuthoritativeFact {
			return ports.OperationSettlementRefV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "settlement Evidence has wrong scope, owner or trust class")
		}
		causationID := string(intent.ID)
		if submission.Observation != nil {
			causationID = submission.Observation.ProviderOperationRef
		}
		if !evidenceHasCausationV3(record.Candidate.Causation, record.Ref.LedgerScopeDigest, causationID) {
			return ports.OperationSettlementRefV3{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "settlement Evidence does not causally bind the settled attempt")
		}
	}
	evidence := append([]ports.EvidenceRecordRefV2{}, submission.Evidence...)
	evidenceDigest, _ := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationSettlementEvidenceV3", evidence)
	disposition := SettlementConfirmedApplied
	switch submission.Disposition {
	case ports.OperationSettlementNotAppliedV3:
		disposition = SettlementConfirmedNotApplied
	case ports.OperationSettlementFailedV3:
		disposition = SettlementConfirmedFailed
	}
	settlement := OperationSettlementFactV3{ID: submission.ID, Revision: submission.Revision, Owner: submission.Owner, Attempt: submission.Attempt, Observation: submission.Observation, Disposition: disposition, Evidence: evidence, EvidenceDigest: evidenceDigest, DomainResult: submission.DomainResult, SettledUnixNano: submission.SettledUnixNano}
	if submission.InspectionEffect != nil {
		inspectionEffect := *submission.InspectionEffect
		inspectionSettlement := *submission.InspectionSettlement
		settlement.InspectionEffect = &inspectionEffect
		settlement.InspectionSettlement = &inspectionSettlement
	}
	if err := settlement.Validate(); err != nil {
		return ports.OperationSettlementRefV3{}, err
	}
	if effect.Settlement != nil {
		if sameOperationSettlementFactV3(*effect.Settlement, settlement) {
			return effect.Settlement.RefV3()
		}
		return ports.OperationSettlementRefV3{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "operation Effect already has different settlement content")
	}
	next := effect
	next.State = OperationEffectSettledV3
	next.Revision++
	next.Settlement = &settlement
	next.UpdatedUnixNano = now.UnixNano()
	stored, err := g.Effects.CompareAndSwapOperationEffectV3(ctx, intent.Operation, OperationEffectCASRequestV3{ExpectedRevision: effect.Revision, Next: next})
	if err != nil {
		if !recoverableOperationWriteErrorV3(err) {
			return ports.OperationSettlementRefV3{}, err
		}
		stored, err = g.Effects.InspectOperationEffectV3(context.WithoutCancel(ctx), intent.Operation, intent.ID)
		if err != nil || stored.Settlement == nil || !sameOperationSettlementFactV3(*stored.Settlement, settlement) {
			return ports.OperationSettlementRefV3{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "cannot prove operation settlement CAS")
		}
	}
	return stored.Settlement.RefV3()
}

func (g OperationSettlementGovernanceGatewayV3) InspectOperationSettlementV3(ctx context.Context, subject ports.OperationSubjectV3, id core.EffectIntentID) (ports.OperationSettlementRefV3, error) {
	if g.Effects == nil {
		return ports.OperationSettlementRefV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "settlement inspection requires the Effect Fact Owner")
	}
	if err := subject.Validate(); err != nil {
		return ports.OperationSettlementRefV3{}, err
	}
	if strings.TrimSpace(string(id)) == "" {
		return ports.OperationSettlementRefV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "settlement inspection requires an Effect ID")
	}
	fact, err := g.Effects.InspectOperationEffectV3(ctx, subject, id)
	if err != nil {
		return ports.OperationSettlementRefV3{}, err
	}
	if fact.Settlement == nil {
		return ports.OperationSettlementRefV3{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "operation settlement not found")
	}
	return fact.Settlement.RefV3()
}

func (g OperationSettlementGovernanceGatewayV3) validateUnknownInspection(ctx context.Context, original OperationEffectFactV3, submission ports.OperationSettlementSubmissionV3) error {
	if submission.InspectionEffect == nil || submission.InspectionSettlement == nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "unknown outcome requires an independently settled Inspect Effect")
	}
	inspection, err := g.Effects.InspectOperationEffectV3(ctx, original.Intent.Operation, submission.InspectionEffect.EffectID)
	if err != nil {
		return err
	}
	if inspection.State != OperationEffectSettledV3 || inspection.Settlement == nil || inspection.Intent.Relation.Purpose != "runtime/inspect" || inspection.Intent.Relation.OriginalOperationEffectID != original.Intent.ID || inspection.Intent.Relation.OriginalRevision != original.Intent.Revision {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectUnknownOutcome, "Inspect Effect does not settle this exact unknown operation")
	}
	inspectionRef, _ := inspection.Settlement.RefV3()
	flatInspectionRef, flatErr := inspectionRef.InspectionRefV3()
	if flatErr != nil || !sameOperationInspectionSettlementRefV3(flatInspectionRef, *submission.InspectionSettlement) {
		return core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Inspect settlement ref drifted")
	}
	return nil
}

func exactOperationOwnerV3(owners []ports.EffectOwnerRefV2, role ports.OwnerRoleV2) (ports.EffectOwnerRefV2, bool) {
	for _, owner := range owners {
		if owner.Role == role {
			return owner, true
		}
	}
	return ports.EffectOwnerRefV2{}, false
}
func evidenceHasCausationV3(refs []ports.EvidenceCausationRefV2, scope core.Digest, eventID string) bool {
	for _, ref := range refs {
		if ref.LedgerScopeDigest == scope && ref.EventID == eventID {
			return true
		}
	}
	return false
}
func sameOperationDispatchAttemptV3(left, right ports.OperationDispatchAttemptRefV3) bool {
	ld, le := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationDispatchAttemptRefV3", left)
	rd, re := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationDispatchAttemptRefV3", right)
	return le == nil && re == nil && ld == rd
}

func sameOperationSettlementFactV3(left, right OperationSettlementFactV3) bool {
	ld, le := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationSettlementFactV3", left)
	rd, re := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationSettlementFactV3", right)
	return le == nil && re == nil && ld == rd
}

func sameOperationSettlementRefV3(left, right ports.OperationSettlementRefV3) bool {
	ld, le := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationSettlementRefV3", left)
	rd, re := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationSettlementRefV3", right)
	return le == nil && re == nil && ld == rd
}

func sameOperationInspectionSettlementRefV3(left, right ports.OperationInspectionSettlementRefV3) bool {
	ld, le := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationInspectionSettlementRefV3", left)
	rd, re := core.CanonicalJSONDigest("praxis.runtime.operation-effect", ports.OperationEffectContractVersionV3, "OperationInspectionSettlementRefV3", right)
	return le == nil && re == nil && ld == rd
}
