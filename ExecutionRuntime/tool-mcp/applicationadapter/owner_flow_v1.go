package applicationadapter

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/action"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// SingleCallToolOwnerPlanV1 contains exact public facts already committed by
// their owners. It does not grant dispatch authority and is never a Provider
// request by itself.
type SingleCallToolOwnerPlanV1 struct {
	Candidate                  toolcontract.ActionCandidateV2
	ApplicationAttempt         toolcontract.ApplicationAttemptRefV1
	IntentDigest               core.Digest
	DomainSubjectDigest        core.Digest
	ReservationExpiresUnixNano int64
	Operation                  runtimeports.OperationSubjectV3
	Attempt                    runtimeports.OperationDispatchAttemptRefV3
	ExecuteEnforcement         runtimeports.OperationDispatchEnforcementPhaseRefV4
	ExecuteHandoff             runtimeports.OperationScopeEvidenceProviderHandoffRefV3
	ControlledProviderV2       *SingleCallToolControlledProviderPlanV2
	Settlement                 SingleCallToolSettlementPlanV1
}

// SingleCallToolControlledProviderPlanV2 carries only Runtime public exact
// refs. It contains no raw Provider, transport handle, or dispatch authority.
type SingleCallToolControlledProviderPlanV2 struct {
	RouteCurrentRef        runtimeports.ControlledOperationProviderRouteCurrentRefV2
	ToolAdapterBinding     runtimeports.ProviderBindingRefV2
	PreparedSemantics      runtimeports.ControlledOperationPreparedSemanticSnapshotV2
	EvidencePolicy         runtimeports.OperationScopeEvidencePolicyRefV3
	ApplicabilityPolicy    runtimeports.OperationScopeEvidenceApplicabilityPolicyRefV3
	EffectRevision         core.Revision
	CallerDeadlineUnixNano int64
}

type SingleCallToolSettlementPlanV1 struct {
	ID                            string
	DomainOwner                   runtimeports.ProviderBindingRefV2
	DomainKind                    runtimeports.NamespacedNameV2
	Evidence                      []runtimeports.OperationSettlementEvidenceBindingV4
	ExpectedEffectRevision        core.Revision
	ExpectedTerminalGuardRevision core.Revision
	IdempotencyKey                string
	ConflictDomain                core.Digest
}

type SingleCallToolOwnerPlanReaderV1 interface {
	InspectSingleCallToolOwnerPlanV1(context.Context, ToolOwnerSingleCallExecutionV1) (SingleCallToolOwnerPlanV1, error)
}

// SingleCallToolProviderInspectionV1 is the Tool Owner's independent exact
// inspection projection after the controlled Provider seam. A Provider return
// value alone never creates a Tool DomainResult.
type SingleCallToolProviderInspectionV1 struct {
	Operation        runtimeports.OperationSubjectV3
	Attempt          runtimeports.OperationDispatchAttemptRefV3
	Prepared         runtimeports.PreparedProviderAttemptRefV2
	Observation      runtimeports.ProviderAttemptObservationRefV2
	Schema           runtimeports.SchemaRefV2
	PayloadDigest    core.Digest
	PayloadRevision  core.Revision
	Residuals        []toolcontract.Residual
	Outcome          toolcontract.ToolOutcomeV2
	Disposition      toolcontract.ToolDispositionV2
	ObservedUnixNano int64
}

type SingleCallToolProviderObservationReaderV1 interface {
	InspectSingleCallToolProviderObservationV1(context.Context, runtimeports.OperationDispatchAttemptRefV3) (SingleCallToolProviderInspectionV1, error)
}

// ToolControlledProviderV2 is the narrow Tool-side seam over Runtime's public
// V2 Gateway. Enter may create the one canonical Entry; Inspect never does.
type ToolControlledProviderV2 interface {
	EnterControlledProviderV2(context.Context, runtimeports.ControlledOperationProviderRequestV2) (runtimeports.ControlledOperationProviderResultV2, error)
	InspectControlledProviderV2(context.Context, runtimeports.ControlledOperationProviderRequestV2) (runtimeports.ControlledOperationProviderResultV2, error)
}

type ToolOwnerSingleCallFlowConfigV1 struct {
	Facts        *action.StoreV2
	Coordination *action.CoordinationStoreV1
	Plans        SingleCallToolOwnerPlanReaderV1
	Observations SingleCallToolProviderObservationReaderV1
	// Deprecated V1 actual-point inputs are retained for source compatibility
	// only. The flow never invokes them: V1 cannot bind actual Provider,
	// Prepared current proof and a unified NotAfter at the physical effect
	// entry. Runtime V2 is required before Provider execution can be enabled.
	Boundary        runtimeports.OperationProviderBoundaryCurrentReaderV1
	Enforcement     runtimeports.OperationProviderExecuteEnforcementCurrentReaderV1
	Handoff         runtimeports.OperationProviderEvidenceHandoffCurrentReaderV1
	Provider        runtimeports.ControlledOperationProviderPortV1
	ControlledV2    ToolControlledProviderV2
	Settlements     runtimeports.OperationSettlementGovernancePortV4
	Clock           ClockV1
	RecoveryTimeout time.Duration
}

// ToolOwnerSingleCallFlowImplV1 is production-neutral orchestration. All
// external capabilities are injected public ports; this package provides no
// production Provider, backend, composition root or transport guarantee.
type ToolOwnerSingleCallFlowImplV1 struct {
	facts           *action.StoreV2
	coordination    *action.CoordinationStoreV1
	plans           SingleCallToolOwnerPlanReaderV1
	observations    SingleCallToolProviderObservationReaderV1
	controlledV2    ToolControlledProviderV2
	settlements     runtimeports.OperationSettlementGovernancePortV4
	clock           ClockV1
	recoveryTimeout time.Duration
}

func NewToolOwnerSingleCallFlowV1(config ToolOwnerSingleCallFlowConfigV1) (*ToolOwnerSingleCallFlowImplV1, error) {
	if config.Facts == nil || config.Coordination == nil || isNilFlowDependencyV1(config.Plans) || isNilFlowDependencyV1(config.Observations) || isNilFlowDependencyV1(config.Settlements) || isNilFlowDependencyV1(config.Clock) || config.RecoveryTimeout <= 0 || config.RecoveryTimeout > 30*time.Second {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool Owner flow dependencies are incomplete")
	}
	controlledV2 := config.ControlledV2
	if isNilToolControlledProviderV2(controlledV2) {
		controlledV2 = nil
	}
	return &ToolOwnerSingleCallFlowImplV1{config.Facts, config.Coordination, config.Plans, config.Observations, controlledV2, config.Settlements, config.Clock, config.RecoveryTimeout}, nil
}

func (f *ToolOwnerSingleCallFlowImplV1) StartOrInspectToolOwnerSingleCallV1(ctx context.Context, input ToolOwnerSingleCallExecutionV1) (toolcontract.ToolResultV2, error) {
	if err := input.Validate(); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	origin, err := f.coordination.InspectWatermarkV1(input.Watermark.WatermarkID, input.Watermark.WatermarkRevision, input.Watermark.WatermarkDigest)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if origin.ApplicationRequestID != input.RequestID || origin.ApplicationRequestRevision != input.RequestRevision || origin.ApplicationRequestDigest != input.RequestDigest || origin.OperationScopeDigest != input.ScopeDigest {
		return toolcontract.ToolResultV2{}, flowConflict("execution coordinate drifted from canonical watermark")
	}
	command, err := f.coordination.InspectCanonicalCommandV1(origin.ID, origin.CanonicalCommandDigest)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	plan, err := f.plans.InspectSingleCallToolOwnerPlanV1(ctx, input)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if err = validateOwnerPlanV1(plan, origin, command, input); err != nil {
		return toolcontract.ToolResultV2{}, err
	}

	previous := time.Time{}
	for transitions := 0; transitions < 16; transitions++ {
		now, err := f.nowAfter(previous)
		if err != nil {
			return toolcontract.ToolResultV2{}, err
		}
		previous = now
		watermark, err := f.coordination.InspectCanonicalCurrentV1(origin.ID, origin.CanonicalCommandDigest, now)
		if err != nil {
			return toolcontract.ToolResultV2{}, err
		}
		exact := sourceV1(watermark)
		switch watermark.Stage {
		case toolcontract.CoordinationRequestRecordedV1:
			record, err := f.facts.PutCandidateV2(plan.Candidate)
			if err != nil {
				return toolcontract.ToolResultV2{}, err
			}
			_, err = f.coordination.BindCandidateV1(exact, objectRefCandidateV1(record.Candidate), now)
			if err != nil && !isResumeErrorV1(err) {
				return toolcontract.ToolResultV2{}, err
			}
		case toolcontract.CoordinationCandidateRecordedV1:
			reservation, err := f.facts.ReserveV2(ctx, *watermark.ActionCandidate, plan.ApplicationAttempt, plan.IntentDigest, plan.Candidate.SessionID, plan.DomainSubjectDigest, now, time.Unix(0, plan.ReservationExpiresUnixNano))
			if err != nil {
				return toolcontract.ToolResultV2{}, err
			}
			_, err = f.coordination.BindReservationV1(exact, objectRefReservationV1(reservation), now)
			if err != nil && !isResumeErrorV1(err) {
				return toolcontract.ToolResultV2{}, err
			}
		case toolcontract.CoordinationReservationRecordedV1:
			_, err := f.coordination.BindRuntimeAttemptV1(exact, plan.Operation, plan.Attempt, now)
			if err != nil && !isResumeErrorV1(err) {
				return toolcontract.ToolResultV2{}, err
			}
		case toolcontract.CoordinationRuntimeAttemptBoundV1:
			if plan.ControlledProviderV2 == nil || isNilToolControlledProviderV2(f.controlledV2) {
				return toolcontract.ToolResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Runtime ControlledOperationProviderPortV2 is unavailable; V1 Provider execution remains unsupported")
			}
			boundarySource, err := f.coordination.CrossProviderBoundaryV1(ctx, exact, plan.ExecuteEnforcement, plan.ExecuteHandoff, now)
			if err != nil && !isResumeErrorV1(err) {
				return toolcontract.ToolResultV2{}, err
			}
			if err != nil {
				continue
			}
			request, err := controlledProviderRequestV2(plan, boundarySource)
			if err != nil {
				return toolcontract.ToolResultV2{}, err
			}
			result, err := f.controlledV2.EnterControlledProviderV2(ctx, request)
			if err != nil {
				return toolcontract.ToolResultV2{}, err
			}
			if result.Status != runtimeports.ControlledOperationProviderObservedV2 {
				return toolcontract.ToolResultV2{}, controlledProviderPendingErrorV2(result)
			}
			if err = f.recordControlledObservationV2(ctx, plan, boundarySource, result, now); err != nil {
				return toolcontract.ToolResultV2{}, err
			}
		case toolcontract.CoordinationProviderBoundaryV1:
			if plan.ControlledProviderV2 != nil && f.controlledV2 != nil {
				request, requestErr := controlledProviderRequestV2(plan, exact)
				if requestErr != nil {
					return toolcontract.ToolResultV2{}, requestErr
				}
				result, inspectErr := f.controlledV2.InspectControlledProviderV2(ctx, request)
				if inspectErr != nil {
					return toolcontract.ToolResultV2{}, inspectErr
				}
				if result.Status != runtimeports.ControlledOperationProviderObservedV2 {
					return toolcontract.ToolResultV2{}, controlledProviderPendingErrorV2(result)
				}
				if err = f.recordControlledObservationV2(ctx, plan, exact, result, now); err != nil {
					return toolcontract.ToolResultV2{}, err
				}
				continue
			}
			inspection, err := f.observations.InspectSingleCallToolProviderObservationV1(ctx, plan.Attempt)
			if err != nil {
				return toolcontract.ToolResultV2{}, err
			}
			if err = validateProviderInspectionV1(inspection, plan); err != nil {
				return toolcontract.ToolResultV2{}, err
			}
			afterInspect, clockErr := f.nowAfter(now)
			if clockErr != nil {
				return toolcontract.ToolResultV2{}, clockErr
			}
			_, err = f.coordination.RecordProviderObservationV1(exact, inspection.Observation, afterInspect)
			if err != nil && !isResumeErrorV1(err) {
				return toolcontract.ToolResultV2{}, err
			}
		case toolcontract.CoordinationProviderObservedV1:
			domainResult, err := f.createDomainResultV1(ctx, plan, watermark, now)
			if err != nil {
				return toolcontract.ToolResultV2{}, err
			}
			_, err = f.coordination.BindDomainResultV1(exact, objectRefDomainV1(domainResult), now)
			if err != nil && !isResumeErrorV1(err) {
				return toolcontract.ToolResultV2{}, err
			}
		case toolcontract.CoordinationDomainResultV1:
			result, err := f.settleAndApplyV1(ctx, plan, watermark, now)
			if err != nil {
				return toolcontract.ToolResultV2{}, err
			}
			_, err = f.coordination.BindApplyV1(exact, result.Apply, now)
			if err != nil && !isResumeErrorV1(err) {
				return toolcontract.ToolResultV2{}, err
			}
		case toolcontract.CoordinationSettlementAppliedV1:
			result, err := f.facts.InspectSettledResultForApplyV2(plan.Candidate.ID, *watermark.Apply)
			if err != nil {
				return toolcontract.ToolResultV2{}, err
			}
			_, err = f.coordination.BindResultV1(exact, objectRefResultV1(result), now)
			if err != nil && !isResumeErrorV1(err) {
				return toolcontract.ToolResultV2{}, err
			}
		case toolcontract.CoordinationResultSettledV1:
			return f.facts.InspectResultV2(plan.Candidate.ID, *watermark.Result)
		default:
			return toolcontract.ToolResultV2{}, flowConflict("unsupported Tool coordination stage")
		}
	}
	return toolcontract.ToolResultV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonInvalidTransition, "Tool Owner flow did not converge")
}

func isNilToolControlledProviderV2(value ToolControlledProviderV2) bool {
	return isNilFlowDependencyV1(value)
}

func isNilFlowDependencyV1(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func (f *ToolOwnerSingleCallFlowImplV1) recordControlledObservationV2(ctx context.Context, plan SingleCallToolOwnerPlanV1, exact toolcontract.ToolProviderBoundarySourceRefV1, result runtimeports.ControlledOperationProviderResultV2, previous time.Time) error {
	if result.Observation == nil {
		return flowConflict("Runtime controlled Provider observed result lacks an Observation")
	}
	inspection, err := f.observations.InspectSingleCallToolProviderObservationV1(ctx, plan.Attempt)
	if err != nil {
		return err
	}
	if err = validateProviderInspectionV1(inspection, plan); err != nil {
		return err
	}
	if inspection.Observation != *result.Observation {
		return flowConflict("Tool Owner inspection drifted from the Runtime controlled Provider Observation")
	}
	now, err := f.nowAfter(previous)
	if err != nil {
		return err
	}
	_, err = f.coordination.RecordProviderObservationV1(exact, inspection.Observation, now)
	if err != nil && !isResumeErrorV1(err) {
		return err
	}
	return nil
}

func controlledProviderRequestV2(plan SingleCallToolOwnerPlanV1, boundary toolcontract.ToolProviderBoundarySourceRefV1) (runtimeports.ControlledOperationProviderRequestV2, error) {
	if plan.ControlledProviderV2 == nil {
		return runtimeports.ControlledOperationProviderRequestV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "controlled Provider V2 plan is unavailable")
	}
	runtimeBoundary, err := boundary.RuntimeRefV1()
	if err != nil {
		return runtimeports.ControlledOperationProviderRequestV2{}, err
	}
	v2 := plan.ControlledProviderV2
	return runtimeports.SealControlledOperationProviderRequestV2(runtimeports.ControlledOperationProviderRequestV2{
		RouteDeclarationRef:    v2.RouteCurrentRef.DeclarationRef,
		RouteConformanceRef:    v2.RouteCurrentRef.ConformanceRef,
		RouteCurrentRef:        v2.RouteCurrentRef,
		ToolAdapterBinding:     v2.ToolAdapterBinding,
		Operation:              plan.Operation,
		OperationDigest:        plan.Attempt.OperationDigest,
		OperationScopeDigest:   plan.Operation.ExecutionScopeDigest,
		EffectID:               plan.Attempt.EffectID,
		EffectRevision:         v2.EffectRevision,
		EffectKind:             plan.Candidate.EffectKind,
		IntentDigest:           plan.IntentDigest,
		Attempt:                plan.Attempt,
		ProviderBinding:        plan.Settlement.DomainOwner,
		Prepared:               v2.PreparedSemantics.Prepared,
		PreparedSemantics:      v2.PreparedSemantics,
		ExecuteEnforcement:     plan.ExecuteEnforcement,
		ExecuteEvidenceHandoff: plan.ExecuteHandoff,
		Boundary:               runtimeBoundary,
		EvidencePolicy:         v2.EvidencePolicy,
		ApplicabilityPolicy:    v2.ApplicabilityPolicy,
		CallerDeadlineUnixNano: v2.CallerDeadlineUnixNano,
	})
}

func controlledProviderPendingErrorV2(result runtimeports.ControlledOperationProviderResultV2) error {
	switch result.Status {
	case runtimeports.ControlledOperationProviderRejectedNoEffectV2:
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "controlled Provider rejected the original Entry with verifiable no-effect")
	case runtimeports.ControlledOperationProviderEnteredV2:
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "controlled Provider Entry requires exact inspection")
	case runtimeports.ControlledOperationProviderUnknownV2:
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "controlled Provider outcome is unknown; only exact inspection may continue")
	default:
		return flowConflict("controlled Provider returned an unsupported state")
	}
}

func (f *ToolOwnerSingleCallFlowImplV1) createDomainResultV1(ctx context.Context, plan SingleCallToolOwnerPlanV1, watermark toolcontract.SingleCallToolActionCoordinationWatermarkV1, now time.Time) (toolcontract.ToolDomainResultFactV2, error) {
	inspection, err := f.observations.InspectSingleCallToolProviderObservationV1(ctx, plan.Attempt)
	if err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	if err = validateProviderInspectionV1(inspection, plan); err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	causality, err := toolcontract.SealRuntimeAttemptCausalityV1(toolcontract.RuntimeAttemptCausalityV1{Reservation: *watermark.Reservation, ApplicationAttempt: plan.ApplicationAttempt, Operation: plan.Operation, OperationDigest: plan.Attempt.OperationDigest, Attempt: plan.Attempt, EffectID: plan.Attempt.EffectID, EffectRevision: plan.Attempt.IntentRevision, IntentDigest: plan.IntentDigest})
	if err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	id, err := toolcontract.StableID("tool-domain-result-v2", plan.Candidate.ID, watermark.Reservation.ID, string(inspection.Observation.Digest))
	if err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	fact, err := toolcontract.SealToolDomainResultFactV2(toolcontract.ToolDomainResultFactV2{ID: id, TenantID: plan.Candidate.TenantID, OperationScopeDigest: plan.Candidate.OperationScopeDigest, Action: *watermark.ActionCandidate, Reservation: *watermark.Reservation, ApplicationAttempt: plan.ApplicationAttempt, Causality: causality, PreparedAttempt: inspection.Prepared, Observation: inspection.Observation, PrepareEnforcement: plan.Settlement.Evidence[0].EnforcementPhase, ExecuteEnforcement: plan.Settlement.Evidence[1].EnforcementPhase, PrepareConsumption: plan.Settlement.Evidence[0].Consumption, ExecuteConsumption: plan.Settlement.Evidence[1].Consumption, Schema: inspection.Schema, PayloadDigest: inspection.PayloadDigest, PayloadRevision: inspection.PayloadRevision, Residuals: inspection.Residuals, Owner: plan.Candidate.ExpectedOwner, Outcome: inspection.Outcome, Disposition: inspection.Disposition, CreatedUnixNano: maxInt64V1(inspection.ObservedUnixNano, now.UnixNano())})
	if err != nil {
		return toolcontract.ToolDomainResultFactV2{}, err
	}
	return f.facts.PutDomainResultV2(ctx, fact)
}

func (f *ToolOwnerSingleCallFlowImplV1) settleAndApplyV1(ctx context.Context, plan SingleCallToolOwnerPlanV1, watermark toolcontract.SingleCallToolActionCoordinationWatermarkV1, now time.Time) (toolcontract.ToolResultV2, error) {
	domain, err := f.facts.InspectDomainResultV2(plan.Candidate.ID, *watermark.DomainResult)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	domainRef := runtimeports.OperationSettlementDomainResultFactRefV4{Owner: plan.Settlement.DomainOwner, Kind: plan.Settlement.DomainKind, ID: domain.ID, Revision: domain.Revision, Digest: domain.Digest, TenantID: domain.TenantID, EffectID: plan.Attempt.EffectID, EffectRevision: plan.Attempt.IntentRevision, Operation: plan.Operation, OperationDigest: plan.Attempt.OperationDigest, Attempt: plan.Attempt, Schema: domain.Schema, PayloadDigest: domain.PayloadDigest, PayloadRevision: domain.PayloadRevision, AuthoritativeTime: domain.CreatedUnixNano}
	if err = domainRef.Validate(); err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	key := runtimeports.InspectCurrentOperationSettlementRequestV4{Operation: plan.Operation, EffectID: plan.Attempt.EffectID}
	inspection, inspectErr := f.settlements.InspectCurrentOperationSettlementV4(ctx, key)
	if inspectErr != nil {
		if !core.HasCategory(inspectErr, core.ErrorNotFound) {
			return toolcontract.ToolResultV2{}, inspectErr
		}
		scopeDigest, err := runtimeports.DigestOperationSettlementScopeSetV4(plan.Settlement.Evidence)
		if err != nil {
			return toolcontract.ToolResultV2{}, err
		}
		submission, err := runtimeports.SealOperationSettlementSubmissionV4(runtimeports.OperationSettlementSubmissionV4{ID: plan.Settlement.ID, TenantID: domain.TenantID, Operation: plan.Operation, OperationDigest: plan.Attempt.OperationDigest, OperationScopeDigest: scopeDigest, EffectID: plan.Attempt.EffectID, ExpectedEffectRevision: plan.Settlement.ExpectedEffectRevision, Owner: domain.Owner, DomainResult: domainRef, Evidence: plan.Settlement.Evidence, ExpectedTerminalGuardRevision: plan.Settlement.ExpectedTerminalGuardRevision, IdempotencyKey: plan.Settlement.IdempotencyKey, ConflictDomain: plan.Settlement.ConflictDomain, SettledUnixNano: now.UnixNano()})
		if err != nil {
			return toolcontract.ToolResultV2{}, err
		}
		if _, err = f.settlements.SettleOperationV4(ctx, submission); err != nil && !core.HasCategory(err, core.ErrorConflict) {
			// A lost settlement reply is recovered only by one bounded exact
			// Inspect. WithoutCancel prevents transport cancellation from
			// erasing an already-committed Runtime fact; it grants no retry.
			inspection, inspectErr = inspectSettlementAfterLostReplyV1(ctx, f.settlements, key, f.recoveryTimeout)
			if inspectErr != nil {
				return toolcontract.ToolResultV2{}, inspectErr
			}
		}
		if inspection.Digest == "" {
			inspection, err = f.settlements.InspectCurrentOperationSettlementV4(ctx, key)
			if err != nil {
				return toolcontract.ToolResultV2{}, err
			}
		}
	}
	fresh, err := f.nowAfter(now)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if err = inspection.Validate(fresh); err != nil || !runtimeports.SameOperationSettlementDomainResultFactRefV4(inspection.DomainResult, domainRef) {
		if err != nil {
			return toolcontract.ToolResultV2{}, err
		}
		return toolcontract.ToolResultV2{}, flowConflict("Runtime settlement closes another Tool DomainResult")
	}
	association, err := f.settlements.InspectOperationSettlementEvidenceAssociationV4(ctx, plan.Operation, inspection.Association)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	applyNow, err := f.nowAfter(fresh)
	if err != nil {
		return toolcontract.ToolResultV2{}, err
	}
	if err = inspection.Validate(applyNow); err != nil || association.Validate() != nil || !runtimeports.SameOperationSettlementEvidenceAssociationRefV4(association.RefV4(), inspection.Association) {
		return toolcontract.ToolResultV2{}, flowConflict("Runtime V4 Association closure is stale or drifted")
	}
	return f.facts.ApplySettlementV2(plan.Candidate.ID, objectRefDomainV1(domain), inspection, domain.Outcome, domain.Disposition, applyNow)
}

type settlementCurrentInspectorV1 interface {
	InspectCurrentOperationSettlementV4(context.Context, runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error)
}

func inspectSettlementAfterLostReplyV1(ctx context.Context, inspector settlementCurrentInspectorV1, exact runtimeports.InspectCurrentOperationSettlementRequestV4, timeout time.Duration) (runtimeports.OperationInspectionSettlementRefV4, error) {
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	return inspector.InspectCurrentOperationSettlementV4(recoveryCtx, exact)
}

func validateOwnerPlanV1(plan SingleCallToolOwnerPlanV1, watermark toolcontract.SingleCallToolActionCoordinationWatermarkV1, command toolcontract.SingleCallCanonicalCommandV1, input ToolOwnerSingleCallExecutionV1) error {
	if plan.Candidate.Validate() != nil || plan.ApplicationAttempt.Validate() != nil || plan.IntentDigest.Validate() != nil || plan.DomainSubjectDigest.Validate() != nil || plan.Operation.Validate() != nil || plan.Attempt.Validate() != nil || plan.ExecuteEnforcement.Validate() != nil || plan.ExecuteHandoff.Validate() != nil || plan.Settlement.DomainOwner.Validate() != nil || runtimeports.ValidateNamespacedNameV2(plan.Settlement.DomainKind) != nil || toolcontract.ValidateStableID(plan.Settlement.ID) != nil || toolcontract.ValidateStableID(plan.Settlement.IdempotencyKey) != nil || plan.Settlement.ExpectedEffectRevision == 0 || plan.Settlement.ConflictDomain.Validate() != nil || len(plan.Settlement.Evidence) != 2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Owner plan is incomplete")
	}
	for _, evidence := range plan.Settlement.Evidence {
		if evidence.Validate() != nil || !reflect.DeepEqual(evidence.Attempt, plan.Attempt) {
			return flowConflict("Tool Owner settlement Evidence belongs to another Runtime Attempt")
		}
	}
	if _, err := runtimeports.DigestOperationSettlementScopeSetV4(plan.Settlement.Evidence); err != nil {
		return err
	}
	operationDigest, err := plan.Operation.DigestV3()
	if err != nil {
		return err
	}
	commandDigest, err := command.DigestV1()
	if err != nil {
		return err
	}
	if commandDigest != watermark.CanonicalCommandDigest || command.ActionCoordinateDigest != input.ActionCoordinateDigest || objectRefCandidateV1(plan.Candidate) != command.ActionCandidate || plan.Candidate.ID != command.ActionID || plan.Candidate.TenantID != command.TenantID || plan.Candidate.RunID != command.RunID || plan.Candidate.SessionID != command.SessionID || plan.Candidate.TurnID != command.TurnID || plan.Candidate.PendingAction != command.PendingAction || plan.Candidate.SourceCandidate != command.SourceCandidate || plan.Candidate.Capability != command.Capability || plan.Candidate.Tool != command.Tool || plan.Candidate.InputSchema != command.InputSchema || plan.Candidate.Payload.Schema != command.InputSchema || plan.Candidate.Payload.ContentDigest != command.PayloadDigest || plan.Candidate.OperationScopeDigest != command.OperationScopeDigest || plan.Candidate.EffectKind != command.EffectKind {
		return flowConflict("Tool Candidate drifted from the exact canonical command proof")
	}
	if plan.Candidate.ExpectedOwner != watermark.Owner || plan.Settlement.DomainOwner != command.Provider || plan.Candidate.ExpectedOwner.ComponentID != plan.Settlement.DomainOwner.ComponentID || plan.Candidate.ExpectedOwner.ManifestDigest != plan.Settlement.DomainOwner.ManifestDigest || string(plan.Settlement.DomainOwner.Capability) != string(command.EffectKind) || string(plan.Settlement.DomainOwner.Capability) != string(plan.Candidate.EffectKind) {
		return flowConflict("Tool Candidate, coordination Owner and Provider binding drifted")
	}
	if plan.Candidate.ID == "" || plan.Candidate.TenantID != watermark.TenantID || plan.Candidate.OperationScopeDigest != input.ScopeDigest || plan.Operation.ExecutionScope.Identity.TenantID != plan.Candidate.TenantID || plan.Operation.ExecutionScopeDigest != plan.Candidate.OperationScopeDigest || string(plan.Operation.RunID) != plan.Candidate.RunID || operationDigest != plan.Attempt.OperationDigest || plan.Candidate.PendingAction.RequestDigest == "" || plan.Attempt.IntentDigest != plan.IntentDigest || plan.Attempt.OperationDigest != plan.ExecuteEnforcement.OperationDigest || plan.Attempt.AttemptID != plan.ExecuteEnforcement.AttemptID || plan.ExecuteEnforcement.Phase != runtimeports.OperationDispatchEnforcementExecuteV4 || plan.Settlement.Evidence[0].Phase != runtimeports.OperationDispatchEnforcementPrepareV4 || plan.Settlement.Evidence[1].Phase != runtimeports.OperationDispatchEnforcementExecuteV4 || !reflect.DeepEqual(plan.Settlement.Evidence[1].EnforcementPhase, plan.ExecuteEnforcement) || !reflect.DeepEqual(plan.Settlement.Evidence[1].Handoff, plan.ExecuteHandoff) {
		return flowConflict("Tool Owner plan exact bindings drifted")
	}
	if plan.ReservationExpiresUnixNano <= watermark.UpdatedUnixNano || plan.ReservationExpiresUnixNano > plan.Candidate.CurrentExpiresUnixNano() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonCapabilityExpired, "reservation lifetime exceeds Candidate currentness")
	}
	if plan.ControlledProviderV2 != nil {
		v2 := plan.ControlledProviderV2
		if v2.RouteCurrentRef.Validate() != nil || v2.ToolAdapterBinding.Validate() != nil || v2.PreparedSemantics.Validate() != nil || v2.EvidencePolicy.Validate() != nil || v2.ApplicabilityPolicy.Validate() != nil || v2.EffectRevision == 0 || v2.CallerDeadlineUnixNano <= watermark.UpdatedUnixNano {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider V2 plan is incomplete")
		}
		prepared := v2.PreparedSemantics
		if v2.ToolAdapterBinding.Capability != runtimeports.ControlledOperationToolAdapterCapabilityV2 || prepared.Attempt != plan.Attempt || plan.Attempt.Delegation == nil || prepared.Delegation != *plan.Attempt.Delegation || prepared.OperationDigest != plan.Attempt.OperationDigest || prepared.EffectID != plan.Attempt.EffectID || prepared.IntentRevision != plan.Attempt.IntentRevision || prepared.IntentDigest != plan.IntentDigest || prepared.ProviderBinding != plan.Settlement.DomainOwner || prepared.PayloadSchema != plan.Candidate.InputSchema || prepared.PayloadDigest != plan.Candidate.Payload.ContentDigest || prepared.PayloadRevision != plan.Candidate.PayloadRevision || v2.EffectRevision != plan.Settlement.ExpectedEffectRevision || v2.CallerDeadlineUnixNano > plan.ReservationExpiresUnixNano {
			return flowConflict("controlled Provider V2 Prepared or Tool Adapter binding drifted from the Tool plan")
		}
	}
	return nil
}

func validateProviderInspectionV1(inspection SingleCallToolProviderInspectionV1, plan SingleCallToolOwnerPlanV1) error {
	if inspection.Operation.Validate() != nil || inspection.Attempt.Validate() != nil || inspection.Prepared.Validate() != nil || inspection.Observation.Validate() != nil || inspection.Schema.Validate() != nil || inspection.PayloadDigest.Validate() != nil || inspection.PayloadRevision == 0 || inspection.ObservedUnixNano <= 0 || toolcontract.ValidateToolOutcomeDispositionV2(inspection.Outcome, inspection.Disposition) != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonEvidenceUnavailable, "Tool Provider inspection is incomplete")
	}
	operationDigest, err := inspection.Operation.DigestV3()
	if err != nil {
		return err
	}
	if !runtimeports.SameOperationSubjectV3(inspection.Operation, plan.Operation) || !reflect.DeepEqual(inspection.Attempt, plan.Attempt) || operationDigest != plan.Attempt.OperationDigest || inspection.Prepared.OperationDigest != operationDigest || inspection.Prepared.IntentID != plan.Attempt.EffectID || inspection.Prepared.IntentRevision != plan.Attempt.IntentRevision || inspection.Prepared.IntentDigest != plan.Attempt.IntentDigest || inspection.Prepared.PermitID != plan.Attempt.PermitID || inspection.Prepared.PermitRevision != plan.Attempt.PermitRevision || inspection.Prepared.PermitDigest != plan.Attempt.PermitDigest || inspection.Prepared.AttemptID != plan.Attempt.AttemptID || inspection.Prepared.Provider != plan.Settlement.DomainOwner || inspection.Prepared.PayloadSchema != plan.Candidate.InputSchema || inspection.Prepared.PayloadDigest != plan.Candidate.Payload.ContentDigest || inspection.Prepared.PayloadRevision != plan.Candidate.PayloadRevision {
		return flowConflict("Provider inspection belongs to another Operation, Effect, Attempt or Provider binding")
	}
	if plan.Attempt.Delegation == nil || inspection.Observation.Delegation != *plan.Attempt.Delegation || inspection.Prepared.DeclaredDelegation.ID != inspection.Observation.Delegation.ID || inspection.Prepared.DeclaredDelegation.Revision >= inspection.Observation.Delegation.Revision || inspection.Observation.PreparedAttemptID != inspection.Prepared.ID {
		return flowConflict("Provider inspection Delegation or Prepared Attempt drifted")
	}
	if inspection.Observation.ObservedUnixNano != inspection.ObservedUnixNano || inspection.Observation.PayloadDigest != inspection.PayloadDigest || inspection.Observation.PayloadRevision != inspection.PayloadRevision || inspection.Observation.State != runtimeports.ProviderAttemptObservedV2 {
		return flowConflict("Provider Observation exact projection drifted")
	}
	return nil
}

func (f *ToolOwnerSingleCallFlowImplV1) nowAfter(previous time.Time) (time.Time, error) {
	now := f.clock.Now()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "Tool Owner flow clock regressed")
	}
	return now, nil
}

func sourceV1(w toolcontract.SingleCallToolActionCoordinationWatermarkV1) toolcontract.ToolProviderBoundarySourceRefV1 {
	return toolcontract.ToolProviderBoundarySourceRefV1{WatermarkID: w.ID, WatermarkRevision: w.Revision, WatermarkDigest: w.Digest}
}
func objectRefCandidateV1(v toolcontract.ActionCandidateV2) toolcontract.ObjectRef {
	return toolcontract.ObjectRef{ID: v.ID, Revision: v.Revision, Digest: v.Digest}
}
func objectRefReservationV1(v toolcontract.ActionReservationFactV2) toolcontract.ObjectRef {
	return toolcontract.ObjectRef{ID: v.ID, Revision: v.Revision, Digest: v.Digest}
}
func objectRefDomainV1(v toolcontract.ToolDomainResultFactV2) toolcontract.ObjectRef {
	return toolcontract.ObjectRef{ID: v.ID, Revision: v.Revision, Digest: v.Digest}
}
func objectRefResultV1(v toolcontract.ToolResultV2) toolcontract.ObjectRef {
	return toolcontract.ObjectRef{ID: v.ID, Revision: v.Revision, Digest: v.Digest}
}
func maxInt64V1(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
func flowConflict(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, message)
}
func isResumeErrorV1(err error) bool {
	return core.HasCategory(err, core.ErrorConflict) || core.HasReason(err, core.ReasonInvalidTransition)
}

var _ ToolOwnerSingleCallFlowV1 = (*ToolOwnerSingleCallFlowImplV1)(nil)
