package control

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RunStartGovernanceGatewayV3 is the only owner-authorized pending->running
// transition. It derives StartedAt from the exact provider Observation after
// the execution-start Operation has been independently settled.
type RunStartGovernanceGatewayV3 struct {
	Runs           RunSettlementFactPortV2
	Effects        OperationEffectFactPortV3
	Delegations    ports.ExecutionDelegationFactPortV2
	PlanAdmissions ports.RunSettlementPlanAdmissionPortV3
	Clock          func() time.Time
}

func (g RunStartGovernanceGatewayV3) ConfirmRunStartedV3(ctx context.Context, request ports.ConfirmRunStartedRequestV3) (ports.RunStartConfirmationEnvelopeV3, error) {
	if g.Runs == nil || g.Effects == nil || g.Delegations == nil || g.PlanAdmissions == nil || g.Clock == nil {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "run start gateway requires Run, Effect, Delegation and clock owners")
	}
	if err := request.Validate(); err != nil {
		return ports.RunStartConfirmationEnvelopeV3{}, err
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "run start gateway clock returned zero")
	}
	bundle, err := g.inspectCertifiedBundleV3(ctx, request.ExecutionScope, request.RunID)
	if err != nil {
		return ports.RunStartConfirmationEnvelopeV3{}, err
	}
	run, plan := bundle.Run, bundle.Plan
	runIdentity, _ := ports.RunIdentityDigestV2(run)
	if runIdentity != plan.RunIdentityDigest || plan.RunID != run.ID || plan.SessionRef != run.SessionRef || !ports.SameExecutionScopeV2(plan.ExecutionScope, run.Scope) {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "pending Run and immutable Plan identity drifted")
	}
	effect, err := g.Effects.InspectOperationEffectV3(ctx, request.Operation, request.Attempt.Admission.EffectID)
	if err != nil {
		return ports.RunStartConfirmationEnvelopeV3{}, err
	}
	if effect.State != OperationEffectSettledV3 || effect.Settlement == nil || effect.DispatchReceipt == nil || effect.Intent.Kind != ports.OperationEffectKindExecutionStartV3 || !ports.SameOperationSubjectV3(effect.Intent.Operation, request.Operation) {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "run start requires a settled execution-start Effect with exact dispatch receipt")
	}
	intentDigest, _ := effect.Intent.DigestV3()
	if request.Attempt.Admission.IntentDigest != intentDigest || request.Attempt.Admission.IntentRevision != effect.Intent.Revision || request.Attempt.Admission.EffectID != effect.Intent.ID {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "run start admission belongs to another Effect intent")
	}
	settlementRef, err := effect.Settlement.RefV3()
	if err != nil || !sameOperationSettlementRefV3(settlementRef, *request.Attempt.Settlement) || settlementRef.Disposition != ports.OperationSettlementAppliedV3 || settlementRef.Observation == nil || *settlementRef.Observation != *request.Attempt.Observation {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "run start settlement or observation drifted")
	}
	delegation, err := g.Delegations.InspectExecutionDelegationV2(ctx, request.Attempt.Delegation.ID)
	if err != nil {
		return ports.RunStartConfirmationEnvelopeV3{}, err
	}
	delegationRef, err := delegation.RefV2()
	if err != nil || delegationRef != request.Attempt.Delegation || delegation.State != ports.ExecutionDelegationPreparedV2 || delegation.PreparedAttemptID != request.Attempt.Prepared.ID || delegation.RuntimeSessionRef != plan.SessionRef || delegation.EndpointID != plan.Execution.EndpointID {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonExecutionInspectionInvalid, "run start delegation does not bind the Plan execution endpoint/session")
	}
	provider := plan.Execution.Binding
	if !sameRunStartProviderBindingV3(delegation.DataProvider, provider) {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "run start provider binding drifted from the immutable Plan semantics")
	}
	startedAt := time.Unix(0, request.Attempt.Observation.ObservedUnixNano)
	if startedAt.After(now) || startedAt.Before(time.Unix(0, plan.CreatedUnixNano)) {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "run start observation time is outside the governed Plan window")
	}
	operationDigest, _ := request.Operation.DigestV3()
	confirmation, err := ports.SealRunStartConfirmationFactV3(ports.RunStartConfirmationFactV3{ContractVersion: ports.RunSettlementContractVersionV2, ID: RunSettlementFactIDV2("run-start", run.ID, operationDigest), Revision: 1, RunID: run.ID, RunIdentityDigest: runIdentity, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, OperationDigest: operationDigest, Attempt: request.Attempt, RunRevision: request.ExpectedRunRevision + 1, StartedUnixNano: startedAt.UnixNano()})
	if err != nil {
		return ports.RunStartConfirmationEnvelopeV3{}, err
	}
	if run.Status == core.RunRunning || run.Status == core.RunStopping || run.Status == core.RunTerminal {
		envelope, inspectErr := g.Runs.InspectRunStartConfirmationV3(ctx, request.ExecutionScope, request.RunID)
		if inspectErr == nil && envelope.Confirmation.Digest == confirmation.Digest && envelope.Validate() == nil {
			return envelope, nil
		}
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "confirmed Run binds another immutable start attempt")
	}
	if run.Status != core.RunPending || run.Revision != request.ExpectedRunRevision {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "run start requires the exact pending Run revision")
	}
	next := run
	next.Status = core.RunRunning
	next.Revision++
	next.StartedAt = startedAt
	envelope, err := g.Runs.CommitRunStartV3(ctx, CommitRunStartRequestV3{ExpectedRunRevision: run.Revision, NextRun: next, Confirmation: confirmation})
	if err != nil {
		if !recoverableOperationWriteErrorV3(err) {
			return ports.RunStartConfirmationEnvelopeV3{}, err
		}
		envelope, err = g.Runs.InspectRunStartConfirmationV3(context.WithoutCancel(ctx), request.ExecutionScope, request.RunID)
		if err != nil || envelope.Confirmation.Digest != confirmation.Digest || !sameRunStartFactV3(envelope.Run, next) {
			return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorIndeterminate, core.ReasonRunConflict, "cannot prove atomic Run start confirmation")
		}
	}
	return envelope, envelope.Validate()
}

func sameRunStartProviderBindingV3(current ports.ProviderBindingRefV2, certified ports.EvidenceProducerBindingRefV2) bool {
	return current.BindingSetID == certified.BindingSetID &&
		current.BindingSetRevision == certified.BindingSetRevision &&
		current.ComponentID == certified.ComponentID &&
		current.ManifestDigest == certified.ManifestDigest &&
		current.ArtifactDigest == certified.ArtifactDigest &&
		current.Capability == certified.Capability
}

func (g RunStartGovernanceGatewayV3) InspectRunStartV3(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (ports.RunStartConfirmationEnvelopeV3, error) {
	if err := (ports.RunTerminationRequestV3{ExecutionScope: scope, RunID: runID}).Validate(); err != nil {
		return ports.RunStartConfirmationEnvelopeV3{}, err
	}
	if g.Runs == nil || g.PlanAdmissions == nil {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "run start gateway requires the Run fact owner")
	}
	bundle, err := g.inspectCertifiedBundleV3(ctx, scope, runID)
	if err != nil {
		return ports.RunStartConfirmationEnvelopeV3{}, err
	}
	envelope, err := g.Runs.InspectRunStartConfirmationV3(ctx, scope, runID)
	if err != nil {
		return ports.RunStartConfirmationEnvelopeV3{}, err
	}
	if envelope.Certification != bundle.Certification {
		return ports.RunStartConfirmationEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "Run start proof binds another Plan certification")
	}
	return envelope, envelope.Validate()
}

func (g RunStartGovernanceGatewayV3) inspectCertifiedBundleV3(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (RunBundleV3, error) {
	owner, ok := g.Runs.(RunBundleFactPortV3)
	if !ok {
		return RunBundleV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "certified Run bundle Fact owner is required")
	}
	bundle, err := owner.InspectRunBundleV3(ctx, scope, runID)
	if err != nil {
		return RunBundleV3{}, err
	}
	fact, err := g.PlanAdmissions.InspectCertifiedRunSettlementPlanV3(ctx, scope, runID)
	if err != nil {
		return RunBundleV3{}, err
	}
	ref, refErr := fact.RefV3()
	planRef, planErr := bundle.Plan.RefV2()
	expectedAssociation, associationErr := ports.NewRunSettlementPlanCertificationAssociationV3(bundle.Run, bundle.Plan, ref)
	if bundle.Run.Validate() != nil || bundle.Plan.Validate() != nil || fact.Validate() != nil || refErr != nil || planErr != nil || associationErr != nil || expectedAssociation != bundle.Certification || ref != bundle.Certification.Certification || fact.RunID != bundle.Run.ID || fact.RunIdentityDigest != bundle.Certification.RunIdentityDigest || fact.ExecutionScopeDigest != bundle.Certification.ExecutionScopeDigest || fact.Plan != planRef {
		return RunBundleV3{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "Run start requires the exact historical Plan certification")
	}
	return bundle, nil
}

func sameRunStartFactV3(left, right core.AgentRunRecord) bool {
	leftIdentity, leftErr := ports.RunIdentityDigestV2(left)
	rightIdentity, rightErr := ports.RunIdentityDigestV2(right)
	return leftErr == nil && rightErr == nil && leftIdentity == rightIdentity && left.Status == right.Status && left.Revision == right.Revision && left.StartedAt.Equal(right.StartedAt) && left.EndedAt.Equal(right.EndedAt) && left.Outcome == right.Outcome
}
