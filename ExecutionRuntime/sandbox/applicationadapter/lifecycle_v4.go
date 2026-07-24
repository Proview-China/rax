package applicationadapter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
)

type LifecycleFactReaderV4 interface {
	GetReservation(context.Context, string) (contract.DomainReservation, error)
	GetObservation(context.Context, string) (contract.Observation, error)
	GetInspection(context.Context, string) (contract.InspectionFact, error)
	GetDomainResult(context.Context, string) (contract.SandboxDomainResultFact, error)
}

type InspectProviderResultRequestV4 struct {
	Reservation contract.DomainReservation
	Observation contract.Observation
	Prepare     ProviderPhaseResultV4
	Execute     ProviderPhaseResultV4
	Inspection  GovernedInspectionPlanV4
}

// InspectionCurrentPortV4 is implemented by the independently governed
// Sandbox Inspect path. A Provider dispatch response cannot implement it.
type InspectionCurrentPortV4 interface {
	InspectProviderResultCurrentV4(context.Context, InspectProviderResultRequestV4) (GovernedInspectionResultV4, error)
}

type LifecycleSettlementPlanV4 struct {
	ID                            string
	Owner                         runtimeports.EffectOwnerRefV2
	ExpectedEffectRevision        runtimecore.Revision
	ExpectedTerminalGuardRevision runtimecore.Revision
	IdempotencyKey                string
	ConflictDomain                runtimecore.Digest
}

type LifecyclePlanV4 struct {
	ReservationID      string
	Prepare            ProviderPhasePlanV4
	Execute            ProviderPhasePlanV4
	DeclaredDelegation runtimeports.ExecutionDelegationRefV2
	ResultID           string
	Settlement         LifecycleSettlementPlanV4
	Inspection         GovernedInspectionPlanV4
}

type LifecycleResultV4 struct {
	Prepare             ProviderPhaseResultV4
	Execute             ProviderPhaseResultV4
	Observation         contract.Observation
	Inspection          contract.InspectionFact
	InspectionOperation GovernedInspectionResultV4
	DomainResult        contract.SandboxDomainResultFact
	RuntimeFact         runtimeports.OperationSettlementDomainResultFactRefV4
	Settlement          runtimeports.OperationInspectionSettlementRefV4
	Projection          contract.EnvironmentProjection
}

type LifecycleFlowV4 struct {
	controller  *kernel.Controller
	facts       LifecycleFactReaderV4
	boundary    *ProviderBoundaryV4
	inspector   InspectionCurrentPortV4
	domain      *runtimeadapter.DomainResultCurrentAdapterV4
	settlements runtimeports.OperationSettlementGovernancePortV4
	now         func() time.Time
}

func NewLifecycleFlowV4(controller *kernel.Controller, facts LifecycleFactReaderV4, boundary *ProviderBoundaryV4, inspector InspectionCurrentPortV4, domain *runtimeadapter.DomainResultCurrentAdapterV4, settlements runtimeports.OperationSettlementGovernancePortV4, now func() time.Time) (*LifecycleFlowV4, error) {
	if controller == nil || nilLike(facts) || boundary == nil || nilLike(inspector) || domain == nil || nilLike(settlements) || nilLike(now) {
		return nil, errors.New("lifecycle flow requires controller, facts, boundary, inspector, domain current, settlement, and clock")
	}
	return &LifecycleFlowV4{controller: controller, facts: facts, boundary: boundary, inspector: inspector, domain: domain, settlements: settlements, now: now}, nil
}

func (f *LifecycleFlowV4) StartOrInspectLifecycleV4(ctx context.Context, plan LifecyclePlanV4) (LifecycleResultV4, error) {
	return f.startOrInspectLifecycleV4(ctx, plan, false, lifecycleHooksV4{})
}

type lifecycleHooksV4 struct {
	beforePrepare func(context.Context) error
	beforeExecute func(context.Context) error
}

func (f *LifecycleFlowV4) startOrInspectLifecycleV4(ctx context.Context, plan LifecyclePlanV4, allowWorkspaceCommit bool, hooks lifecycleHooksV4) (LifecycleResultV4, error) {
	if f == nil || nilLike(ctx) {
		return LifecycleResultV4{}, errors.New("lifecycle flow or context is nil")
	}
	reservation, err := f.facts.GetReservation(ctx, plan.ReservationID)
	if err != nil {
		return LifecycleResultV4{}, err
	}
	if err := reservation.ValidateCurrent(f.now()); err != nil {
		return LifecycleResultV4{}, err
	}
	if err := validateLifecyclePlanV4Mode(plan, reservation, allowWorkspaceCommit); err != nil {
		return LifecycleResultV4{}, err
	}
	if hooks.beforePrepare != nil {
		if err := hooks.beforePrepare(ctx); err != nil {
			return LifecycleResultV4{}, err
		}
	}

	prepare, err := f.boundary.ExecutePhase(ctx, plan.Prepare)
	if err != nil {
		return LifecycleResultV4{}, err
	}
	prepared, err := preparedAttemptV4(plan, prepare)
	if err != nil {
		return LifecycleResultV4{}, err
	}
	executePlan := plan.Execute
	executePlan.Enforcement.Prepare = &prepare.Current.Phase
	executePlan.Enforcement.PreparedAttempt = &prepared
	executePlan.Enforcement.ExpectedJournalRevision = prepare.Current.Journal.Revision
	if hooks.beforeExecute != nil {
		if err := hooks.beforeExecute(ctx); err != nil {
			return LifecycleResultV4{}, err
		}
	}
	execute, err := f.boundary.ExecutePhase(ctx, executePlan)
	if err != nil {
		return LifecycleResultV4{}, err
	}

	observation, err := f.ensureObservation(ctx, reservation, plan, prepare, execute)
	if err != nil {
		return LifecycleResultV4{}, err
	}
	inspectionOperation, err := f.ensureInspection(ctx, reservation, observation, prepare, execute, plan.Inspection)
	if err != nil {
		return LifecycleResultV4{}, err
	}
	inspection := inspectionOperation.Fact
	domainResult, err := f.ensureDomainResult(ctx, reservation, inspection, plan.ResultID, prepare, execute)
	if err != nil {
		return LifecycleResultV4{}, err
	}
	runtimeFact, err := f.domain.BindDomainResultRuntimeV4(ctx, runtimeadapter.BindDomainResultRuntimeV4Request{
		EffectKind: runtimeports.EffectKindV2(reservation.Kind), ResultID: domainResult.Meta.ID,
		Operation: plan.Prepare.Enforcement.Operation, Attempt: plan.Prepare.Attempt,
	})
	if err != nil {
		return LifecycleResultV4{}, err
	}
	settlement, err := f.settle(ctx, plan, runtimeFact, prepare.Binding, execute.Binding)
	if err != nil {
		return LifecycleResultV4{}, err
	}
	projection, err := f.controller.ApplySettlement(ctx, domainResult.Meta.ID, contract.RuntimeOperationSettlementRef{
		OpaqueRef:   contract.Ref{ID: settlement.Settlement.ID, Revision: uint64(settlement.Settlement.Revision), Digest: string(settlement.Settlement.Digest)},
		OperationID: reservation.OperationID, AttemptID: reservation.AttemptID, DomainResultRef: domainResult.Meta.Ref(),
	})
	if err != nil {
		return LifecycleResultV4{}, err
	}
	return LifecycleResultV4{Prepare: prepare, Execute: execute, Observation: observation, Inspection: inspection, InspectionOperation: inspectionOperation, DomainResult: domainResult, RuntimeFact: runtimeFact, Settlement: settlement, Projection: projection}, nil
}

func (f *LifecycleFlowV4) ensureObservation(ctx context.Context, reservation contract.DomainReservation, plan LifecyclePlanV4, prepare, execute ProviderPhaseResultV4) (contract.Observation, error) {
	id := "observation-" + plan.ResultID
	if existing, err := f.facts.GetObservation(ctx, id); err == nil {
		return existing, validateObservationForPhases(existing, reservation, prepare, execute, f.now())
	} else if !errors.Is(err, sandboxports.ErrNotFound) {
		return contract.Observation{}, err
	}
	evidence := []contract.Ref{evidenceRef(prepare.Binding), evidenceRef(execute.Binding)}
	body := struct {
		Reservation contract.Ref
		AttemptID   string
		Receipt     contract.Ref
		Evidence    []contract.Ref
		State       string
	}{reservation.Meta.Ref(), reservation.AttemptID, providerReceiptRef(execute), evidence, execute.Response.ProviderObservation.State}
	meta, err := contract.NewMeta(id, 1, f.now(), time.Unix(0, minimumInt64(reservation.Meta.ExpiresUnixNano, execute.Response.ExpiresUnixNano)), "provider-observation-v4", body)
	if err != nil {
		return contract.Observation{}, err
	}
	value := contract.Observation{
		Meta: meta, ReservationRef: reservation.Meta.Ref(), OperationID: reservation.OperationID, AttemptID: reservation.AttemptID,
		SourceRegistrationID: plan.Execute.Evidence.Reservation.Source.RegistrationID,
		SourceEpoch:          uint64(plan.Execute.Evidence.Reservation.Source.SourceEpoch), SourceSequence: plan.Execute.Evidence.Reservation.Source.SourceSequence,
		PayloadDigest: string(execute.Binding.CandidateDigest), ReceiptRef: providerReceiptRef(execute), EvidenceRefs: evidence,
		ObservedState: execute.Response.ProviderObservation.State,
	}
	if _, err := f.controller.RecordObservation(ctx, value); err != nil {
		recovered, inspectErr := f.facts.GetObservation(context.WithoutCancel(ctx), id)
		if inspectErr != nil || validateObservationForPhases(recovered, reservation, prepare, execute, f.now()) != nil {
			return contract.Observation{}, err
		}
		return recovered, nil
	}
	return value, nil
}

func (f *LifecycleFlowV4) ensureInspection(ctx context.Context, reservation contract.DomainReservation, observation contract.Observation, prepare, execute ProviderPhaseResultV4, plan GovernedInspectionPlanV4) (GovernedInspectionResultV4, error) {
	closure, err := f.inspector.InspectProviderResultCurrentV4(ctx, InspectProviderResultRequestV4{Reservation: reservation, Observation: observation, Prepare: prepare, Execute: execute, Inspection: plan})
	if err != nil {
		return GovernedInspectionResultV4{}, err
	}
	value := closure.Fact
	if err := value.ValidateCurrent(f.now()); err != nil || !contract.SameRef(value.ReservationRef, reservation.Meta.Ref()) || !contract.SameRef(value.ObservationRef, observation.Meta.Ref()) || value.OperationID != reservation.OperationID || value.AttemptID != reservation.AttemptID {
		return GovernedInspectionResultV4{}, errors.New("independent Sandbox inspection returned another attempt")
	}
	if err := f.controller.RecordInspection(ctx, value); err != nil {
		recovered, inspectErr := f.facts.GetInspection(context.WithoutCancel(ctx), value.Meta.ID)
		if inspectErr != nil || recovered.ValidateCurrent(f.now()) != nil || !contract.SameRef(recovered.Meta.Ref(), value.Meta.Ref()) || !contract.SameRef(recovered.ReservationRef, reservation.Meta.Ref()) || !contract.SameRef(recovered.ObservationRef, observation.Meta.Ref()) {
			return GovernedInspectionResultV4{}, err
		}
		closure.Fact = recovered
		return closure, nil
	}
	return closure, nil
}

func (f *LifecycleFlowV4) ensureDomainResult(ctx context.Context, reservation contract.DomainReservation, inspection contract.InspectionFact, id string, prepare, execute ProviderPhaseResultV4) (contract.SandboxDomainResultFact, error) {
	if existing, err := f.facts.GetDomainResult(ctx, id); err == nil {
		if existing.ValidateCurrent(f.now()) != nil || existing.ReservationRef != reservation.Meta.Ref() || existing.InspectionRef != inspection.Meta.Ref() || existing.OperationID != reservation.OperationID || existing.AttemptID != reservation.AttemptID || existing.Kind != reservation.Kind {
			return contract.SandboxDomainResultFact{}, errors.New("DomainResult ID already binds another closure")
		}
		return existing, nil
	} else if !errors.Is(err, sandboxports.ErrNotFound) {
		return contract.SandboxDomainResultFact{}, err
	}
	payload, err := payloadForDisposition(reservation.Kind, inspection)
	if err != nil {
		return contract.SandboxDomainResultFact{}, err
	}
	evidence := []contract.Ref{evidenceRef(prepare.Binding), evidenceRef(execute.Binding)}
	body := struct {
		Reservation contract.Ref
		Inspection  contract.Ref
		Disposition contract.Disposition
		Payload     contract.DomainResultPayload
		Evidence    []contract.Ref
	}{reservation.Meta.Ref(), inspection.Meta.Ref(), inspection.Disposition, payload, evidence}
	meta, err := contract.NewMeta(id, 1, f.now(), time.Unix(0, minimumInt64(reservation.Meta.ExpiresUnixNano, inspection.Meta.ExpiresUnixNano, prepare.Qualification.ExpiresUnixNano, execute.Qualification.ExpiresUnixNano)), "sandbox-domain-result-v4", body)
	if err != nil {
		return contract.SandboxDomainResultFact{}, err
	}
	value := contract.SandboxDomainResultFact{Meta: meta, ReservationRef: reservation.Meta.Ref(), InspectionRef: inspection.Meta.Ref(), OperationID: reservation.OperationID, AttemptID: reservation.AttemptID, Kind: reservation.Kind, Disposition: inspection.Disposition, Lease: reservation.Lease, Payload: payload, EvidenceRefs: evidence}
	if err := f.controller.CommitDomainResult(ctx, value); err != nil {
		recovered, inspectErr := f.facts.GetDomainResult(context.WithoutCancel(ctx), id)
		if inspectErr != nil || recovered.ValidateCurrent(f.now()) != nil || !contract.SameRef(recovered.Meta.Ref(), value.Meta.Ref()) ||
			!contract.SameRef(recovered.ReservationRef, reservation.Meta.Ref()) || !contract.SameRef(recovered.InspectionRef, inspection.Meta.Ref()) ||
			recovered.OperationID != reservation.OperationID || recovered.AttemptID != reservation.AttemptID || recovered.Kind != reservation.Kind || recovered.Disposition != inspection.Disposition {
			return contract.SandboxDomainResultFact{}, err
		}
		return recovered, nil
	}
	return value, nil
}

func (f *LifecycleFlowV4) settle(ctx context.Context, plan LifecyclePlanV4, domain runtimeports.OperationSettlementDomainResultFactRefV4, prepare, execute runtimeports.OperationSettlementEvidenceBindingV4) (runtimeports.OperationInspectionSettlementRefV4, error) {
	evidence := []runtimeports.OperationSettlementEvidenceBindingV4{prepare, execute}
	scopeDigest, err := runtimeports.DigestOperationSettlementScopeSetV4(evidence)
	if err != nil {
		return runtimeports.OperationInspectionSettlementRefV4{}, err
	}
	submission, err := runtimeports.SealOperationSettlementSubmissionV4(runtimeports.OperationSettlementSubmissionV4{
		ID: plan.Settlement.ID, TenantID: domain.TenantID, Operation: domain.Operation, OperationDigest: domain.OperationDigest,
		OperationScopeDigest: scopeDigest, EffectID: domain.EffectID, ExpectedEffectRevision: plan.Settlement.ExpectedEffectRevision,
		Owner: plan.Settlement.Owner, DomainResult: domain, Evidence: evidence,
		ExpectedTerminalGuardRevision: plan.Settlement.ExpectedTerminalGuardRevision,
		IdempotencyKey:                plan.Settlement.IdempotencyKey, ConflictDomain: plan.Settlement.ConflictDomain,
		SettledUnixNano: f.now().UnixNano(),
	})
	if err != nil {
		return runtimeports.OperationInspectionSettlementRefV4{}, err
	}
	settled, err := f.settlements.SettleOperationV4(ctx, submission)
	if err != nil {
		fact, inspectErr := f.settlements.InspectOperationSettlementV4(context.WithoutCancel(ctx), runtimeports.InspectOperationSettlementRequestV4{Operation: domain.Operation, SettlementID: submission.ID})
		if inspectErr != nil || fact.Validate() != nil || fact.Submission.Digest != submission.Digest {
			return runtimeports.OperationInspectionSettlementRefV4{}, err
		}
		settled = fact.RefV4()
	} else if settled.Validate() != nil || settled.ID != submission.ID || settled.OperationDigest != submission.OperationDigest || settled.EffectID != submission.EffectID || !runtimeports.SameOperationSettlementDomainResultFactRefV4(settled.DomainResult, submission.DomainResult) {
		return runtimeports.OperationInspectionSettlementRefV4{}, errors.New("Runtime settlement returned another exact submission")
	}
	current, err := f.settlements.InspectCurrentOperationSettlementV4(ctx, runtimeports.InspectCurrentOperationSettlementRequestV4{Operation: domain.Operation, EffectID: domain.EffectID})
	if err != nil {
		return runtimeports.OperationInspectionSettlementRefV4{}, err
	}
	if current.Validate(f.now()) != nil || !runtimeports.SameOperationSettlementRefV4(current.Settlement, settled) ||
		!runtimeports.SameOperationSettlementDomainResultFactRefV4(current.DomainResult, domain) || current.EffectFactRevision != plan.Settlement.ExpectedEffectRevision || current.Owner != plan.Settlement.Owner {
		return runtimeports.OperationInspectionSettlementRefV4{}, errors.New("Runtime current settlement returned another exact closure")
	}
	return current, nil
}

func preparedAttemptV4(plan LifecyclePlanV4, prepare ProviderPhaseResultV4) (runtimeports.PreparedProviderAttemptRefV2, error) {
	legacy := prepare.Current.Dispatch.Record.Permit.LegacyPermit
	legacyDigest, err := legacy.DigestV3()
	if err != nil {
		return runtimeports.PreparedProviderAttemptRefV2{}, err
	}
	id, err := runtimeports.DerivePreparedProviderAttemptIDV2(plan.DeclaredDelegation.ID, legacy.ID, legacy.AttemptID)
	if err != nil {
		return runtimeports.PreparedProviderAttemptRefV2{}, err
	}
	if string(legacy.PayloadDigest) != prepare.Dispatch.PayloadDigest {
		return runtimeports.PreparedProviderAttemptRefV2{}, errors.New("Runtime permit payload differs from the Data Plane prepared payload")
	}
	return runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{
		ID: id, Revision: runtimecore.Revision(prepare.Response.ProviderAttempt.Revision), DeclaredDelegation: plan.DeclaredDelegation,
		OperationDigest: prepare.Current.Sandbox.OperationDigest, IntentID: legacy.IntentID, IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest,
		PermitID: legacy.ID, PermitRevision: legacy.Revision, PermitDigest: legacyDigest, AttemptID: legacy.AttemptID,
		Provider: legacy.Provider, PayloadSchema: legacy.PayloadSchema, PayloadDigest: legacy.PayloadDigest, PayloadRevision: legacy.PayloadRevision,
		PreparedUnixNano: prepare.Response.ProviderObservation.ObservedUnixNano, ExpiresUnixNano: minimumInt64(prepare.Response.ExpiresUnixNano, prepare.Current.ExpiresUnixNano),
	})
}

func payloadForDisposition(kind contract.EffectKind, inspection contract.InspectionFact) (contract.DomainResultPayload, error) {
	if inspection.Disposition != contract.DispositionConfirmedApplied {
		return contract.DomainResultPayload{}, nil
	}
	switch kind {
	case contract.EffectAllocate:
		return contract.DomainResultPayload{AllocationConfirmed: true}, nil
	case contract.EffectActivate:
		return contract.DomainResultPayload{ActivationConfirmed: true}, nil
	case contract.EffectOpen:
		return contract.DomainResultPayload{OpenConfirmed: true}, nil
	case contract.EffectCancel:
		return contract.DomainResultPayload{ExecutionQuiesced: true}, nil
	case contract.EffectClose:
		return contract.DomainResultPayload{EnvironmentClosed: true}, nil
	case contract.EffectFence:
		return contract.DomainResultPayload{FenceConfirmed: true}, nil
	case contract.EffectRelease:
		return contract.DomainResultPayload{ReleaseConfirmed: true}, nil
	case contract.EffectCleanup:
		if inspection.Cleanup == nil {
			return contract.DomainResultPayload{}, errors.New("cleanup inspection lacks the seven-dimensional report")
		}
		return contract.DomainResultPayload{Cleanup: inspection.Cleanup}, nil
	case contract.EffectWorkspaceCommit:
		if inspection.WorkspaceChangeSetRef == nil {
			return contract.DomainResultPayload{}, errors.New("workspace commit inspection lacks its exact ChangeSet ref")
		}
		ref := *inspection.WorkspaceChangeSetRef
		return contract.DomainResultPayload{WorkspaceChangeSetRef: &ref}, nil
	case contract.EffectInspect:
		return contract.DomainResultPayload{}, nil
	default:
		return contract.DomainResultPayload{}, fmt.Errorf("lifecycle flow does not support %s", kind)
	}
}

func validateLifecyclePlanV4(plan LifecyclePlanV4, reservation contract.DomainReservation) error {
	return validateLifecyclePlanV4Mode(plan, reservation, false)
}

func validateLifecyclePlanV4Mode(plan LifecyclePlanV4, reservation contract.DomainReservation, allowWorkspaceCommit bool) error {
	if plan.ReservationID == "" || plan.ReservationID != reservation.Meta.ID || plan.ResultID == "" || plan.Settlement.ID == "" || plan.Settlement.IdempotencyKey == "" || plan.Settlement.ExpectedEffectRevision == 0 || plan.Settlement.ExpectedTerminalGuardRevision == 0 || plan.Settlement.Owner.Role != runtimeports.OwnerSettlement || runtimeports.ValidateNamespacedNameV2(runtimeports.NamespacedNameV2(plan.Settlement.Owner.ComponentID)) != nil || plan.Settlement.Owner.ManifestDigest.Validate() != nil || plan.Settlement.ConflictDomain.Validate() != nil || plan.DeclaredDelegation.Validate() != nil {
		return errors.New("lifecycle plan identity or settlement is incomplete")
	}
	if plan.Prepare.Enforcement.Phase != runtimeports.OperationDispatchEnforcementPrepareV4 || plan.Execute.Enforcement.Phase != runtimeports.OperationDispatchEnforcementExecuteV4 || plan.Prepare.Enforcement.Operation.Validate() != nil || !runtimeports.SameOperationSubjectV3(plan.Prepare.Enforcement.Operation, plan.Execute.Enforcement.Operation) || string(plan.Prepare.Enforcement.EffectID) != reservation.EffectID || string(plan.Execute.Enforcement.EffectID) != reservation.EffectID || plan.Prepare.Enforcement.AttemptID != reservation.AttemptID || plan.Execute.Enforcement.AttemptID != reservation.AttemptID || plan.Prepare.EffectKind != string(reservation.Kind) || plan.Execute.EffectKind != string(reservation.Kind) || plan.Prepare.Attempt != plan.Execute.Attempt {
		return errors.New("lifecycle prepare and execute do not bind the exact Sandbox reservation")
	}
	if plan.Prepare.RequestID == plan.Execute.RequestID || plan.Prepare.Evidence.QualificationID == plan.Execute.Evidence.QualificationID || plan.Prepare.Evidence.HandoffID == plan.Execute.Evidence.HandoffID || plan.Prepare.Evidence.ConsumptionID == plan.Execute.Evidence.ConsumptionID || plan.Prepare.Evidence.Reservation.EventID == plan.Execute.Evidence.Reservation.EventID || plan.Prepare.Evidence.Reservation.Source == plan.Execute.Evidence.Reservation.Source {
		return errors.New("lifecycle prepare and execute must have independent phase identities and source positions")
	}
	if plan.Prepare.Attempt.IntentRevision != runtimecore.Revision(reservation.IntentRevision) || string(plan.Prepare.Attempt.IntentDigest) != prefixedSandboxDigest(reservation.IntentDigest) {
		return errors.New("lifecycle Runtime attempt differs from the Sandbox reservation intent")
	}
	if reservation.Kind != contract.EffectInspect {
		operationKind := plan.Prepare.Enforcement.Operation.Kind
		switch reservation.Kind {
		case contract.EffectAllocate, contract.EffectActivate, contract.EffectOpen:
			if operationKind != runtimeports.OperationScopeActivationV3 {
				return errors.New("activation lifecycle effect requires an activation_attempt Operation")
			}
		case contract.EffectCancel:
			if operationKind != runtimeports.OperationScopeRunV3 {
				return errors.New("cancel requires a run Operation")
			}
		case contract.EffectClose, contract.EffectRelease:
			if operationKind != runtimeports.OperationScopeTerminationV3 {
				return errors.New("close and release require a termination_attempt Operation")
			}
		case contract.EffectFence, contract.EffectCleanup:
			if operationKind != runtimeports.OperationScopeTerminationV3 && operationKind != runtimeports.OperationScopeAdminV3 {
				return errors.New("fence and cleanup require a termination_attempt or admin Operation")
			}
			// Each effect remains an independent Operation/Attempt. Reusing this
			// orchestration code does not merge their Runtime or Provider facts.
		case contract.EffectWorkspaceCommit:
			if !allowWorkspaceCommit {
				return errors.New("workspace commit requires its dedicated governed commit closure")
			}
			if operationKind != runtimeports.OperationScopeRunV3 {
				return errors.New("workspace commit requires a run Operation")
			}
		default:
			return errors.New("production lifecycle V4 does not support this Sandbox effect")
		}
		if plan.Inspection.ReservationID == "" || plan.Inspection.FinalInspectionID == "" || plan.Inspection.Prepare.EffectKind != string(contract.EffectInspect) || plan.Inspection.Execute.EffectKind != string(contract.EffectInspect) {
			return errors.New("lifecycle plan lacks an independently governed Inspect closure")
		}
	}
	return nil
}

func prefixedSandboxDigest(value string) string {
	if strings.HasPrefix(value, "sha256:") {
		return value
	}
	return "sha256:" + value
}

func validateObservationForPhases(value contract.Observation, reservation contract.DomainReservation, prepare, execute ProviderPhaseResultV4, now time.Time) error {
	if err := value.ValidateCurrent(now); err != nil {
		return err
	}
	if !contract.SameRef(value.ReservationRef, reservation.Meta.Ref()) || value.AttemptID != reservation.AttemptID || value.ReceiptRef != providerReceiptRef(execute) || len(value.EvidenceRefs) != 2 || value.EvidenceRefs[0] != evidenceRef(prepare.Binding) || value.EvidenceRefs[1] != evidenceRef(execute.Binding) {
		return errors.New("persisted observation binds another phase closure")
	}
	return nil
}

func providerReceiptRef(result ProviderPhaseResultV4) contract.Ref {
	return contract.Ref{ID: result.Response.ProviderAttempt.ID + "/" + string(result.Current.Phase.Phase), Revision: result.Response.ProviderAttempt.Revision, Digest: result.Response.ProviderReceipt.Digest}
}

func evidenceRef(binding runtimeports.OperationSettlementEvidenceBindingV4) contract.Ref {
	return contract.Ref{ID: binding.Consumption.ID, Revision: uint64(binding.Consumption.Revision), Digest: string(binding.Consumption.Digest)}
}

func minimumInt64(values ...int64) int64 {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}
