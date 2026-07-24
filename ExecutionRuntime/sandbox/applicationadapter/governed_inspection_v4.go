package applicationadapter

import (
	"context"
	"errors"
	"strings"
	"time"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/runtimeadapter"
)

type GovernedInspectionPlanV4 struct {
	ReservationID      string
	Prepare            ProviderPhasePlanV4
	Execute            ProviderPhasePlanV4
	DeclaredDelegation runtimeports.ExecutionDelegationRefV2
	ResultID           string
	Settlement         LifecycleSettlementPlanV4
	FinalInspectionID  string
}

type GovernedInspectionResultV4 struct {
	Prepare       ProviderPhaseResultV4
	Execute       ProviderPhaseResultV4
	Observation   contract.Observation
	ExecutionFact contract.InspectionFact
	DomainResult  contract.SandboxDomainResultFact
	RuntimeFact   runtimeports.OperationSettlementDomainResultFactRefV4
	Settlement    runtimeports.OperationInspectionSettlementRefV4
	Projection    contract.EnvironmentProjection
	Fact          contract.InspectionFact
}

// GovernedInspectionV4 runs an independent praxis.sandbox/inspect Effect. The
// inspect Operation has its own Attempt, Permit, prepare/execute Enforcement,
// Evidence, DomainResult, Settlement and Sandbox ApplySettlement. Its payload
// binds the exact original Provider attempt; it never retries that attempt.
type GovernedInspectionV4 struct {
	controller  *kernel.Controller
	facts       LifecycleFactReaderV4
	boundary    *ProviderBoundaryV4
	domain      *runtimeadapter.DomainResultCurrentAdapterV4
	settlements runtimeports.OperationSettlementGovernancePortV4
	now         func() time.Time
}

func NewGovernedInspectionV4(controller *kernel.Controller, facts LifecycleFactReaderV4, boundary *ProviderBoundaryV4, domain *runtimeadapter.DomainResultCurrentAdapterV4, settlements runtimeports.OperationSettlementGovernancePortV4, now func() time.Time) (*GovernedInspectionV4, error) {
	if controller == nil || nilLike(facts) || boundary == nil || domain == nil || nilLike(settlements) || nilLike(now) {
		return nil, errors.New("governed inspection requires controller, facts, boundary, domain current, settlement, and clock")
	}
	return &GovernedInspectionV4{controller: controller, facts: facts, boundary: boundary, domain: domain, settlements: settlements, now: now}, nil
}

var _ InspectionCurrentPortV4 = (*GovernedInspectionV4)(nil)

func (g *GovernedInspectionV4) InspectProviderResultCurrentV4(ctx context.Context, request InspectProviderResultRequestV4) (GovernedInspectionResultV4, error) {
	if g == nil || nilLike(ctx) {
		return GovernedInspectionResultV4{}, errors.New("governed inspection or context is nil")
	}
	if err := request.Reservation.ValidateCurrent(g.now()); err != nil {
		return GovernedInspectionResultV4{}, err
	}
	if err := request.Observation.ValidateCurrent(g.now()); err != nil {
		return GovernedInspectionResultV4{}, err
	}
	inspectionReservation, err := g.facts.GetReservation(ctx, request.Inspection.ReservationID)
	if err != nil {
		return GovernedInspectionResultV4{}, err
	}
	if err := inspectionReservation.ValidateCurrent(g.now()); err != nil {
		return GovernedInspectionResultV4{}, err
	}
	if err := validateGovernedInspectionPlanV4(request, inspectionReservation); err != nil {
		return GovernedInspectionResultV4{}, err
	}

	prepare, err := g.boundary.ExecutePhase(ctx, request.Inspection.Prepare)
	if err != nil {
		return GovernedInspectionResultV4{}, err
	}
	prepared, err := preparedAttemptV4(LifecyclePlanV4{DeclaredDelegation: request.Inspection.DeclaredDelegation}, prepare)
	if err != nil {
		return GovernedInspectionResultV4{}, err
	}
	executePlan := request.Inspection.Execute
	executePlan.Enforcement.Prepare = &prepare.Current.Phase
	executePlan.Enforcement.PreparedAttempt = &prepared
	executePlan.Enforcement.ExpectedJournalRevision = prepare.Current.Journal.Revision
	execute, err := g.boundary.ExecutePhase(ctx, executePlan)
	if err != nil {
		return GovernedInspectionResultV4{}, err
	}

	flow := &LifecycleFlowV4{controller: g.controller, facts: g.facts, domain: g.domain, settlements: g.settlements, now: g.now}
	inspectPlan := LifecyclePlanV4{ReservationID: inspectionReservation.Meta.ID, Prepare: request.Inspection.Prepare, Execute: request.Inspection.Execute, DeclaredDelegation: request.Inspection.DeclaredDelegation, ResultID: request.Inspection.ResultID, Settlement: request.Inspection.Settlement}
	observation, err := flow.ensureObservation(ctx, inspectionReservation, inspectPlan, prepare, execute)
	if err != nil {
		return GovernedInspectionResultV4{}, err
	}
	executionFact, err := g.ensureInspectionExecutionFact(ctx, inspectionReservation, observation, prepare, execute, request.Inspection.ResultID)
	if err != nil {
		return GovernedInspectionResultV4{}, err
	}
	domainResult, err := flow.ensureDomainResult(ctx, inspectionReservation, executionFact, request.Inspection.ResultID, prepare, execute)
	if err != nil {
		return GovernedInspectionResultV4{}, err
	}
	runtimeFact, err := g.domain.BindDomainResultRuntimeV4(ctx, runtimeadapter.BindDomainResultRuntimeV4Request{
		EffectKind: runtimeports.EffectKindV2(contract.EffectInspect), ResultID: domainResult.Meta.ID,
		Operation: request.Inspection.Prepare.Enforcement.Operation, Attempt: request.Inspection.Prepare.Attempt,
	})
	if err != nil {
		return GovernedInspectionResultV4{}, err
	}
	settlement, err := flow.settle(ctx, inspectPlan, runtimeFact, prepare.Binding, execute.Binding)
	if err != nil {
		return GovernedInspectionResultV4{}, err
	}
	projection, err := g.controller.ApplySettlement(ctx, domainResult.Meta.ID, contract.RuntimeOperationSettlementRef{
		OpaqueRef:   contract.Ref{ID: settlement.Settlement.ID, Revision: uint64(settlement.Settlement.Revision), Digest: string(settlement.Settlement.Digest)},
		OperationID: inspectionReservation.OperationID, AttemptID: inspectionReservation.AttemptID, DomainResultRef: domainResult.Meta.Ref(),
	})
	if err != nil {
		return GovernedInspectionResultV4{}, err
	}
	fact, err := g.finalInspectionFact(request, executionFact, settlement, prepare, execute)
	if err != nil {
		return GovernedInspectionResultV4{}, err
	}
	return GovernedInspectionResultV4{Prepare: prepare, Execute: execute, Observation: observation, ExecutionFact: executionFact, DomainResult: domainResult, RuntimeFact: runtimeFact, Settlement: settlement, Projection: projection, Fact: fact}, nil
}

func (g *GovernedInspectionV4) ensureInspectionExecutionFact(ctx context.Context, reservation contract.DomainReservation, observation contract.Observation, prepare, execute ProviderPhaseResultV4, resultID string) (contract.InspectionFact, error) {
	id := "inspection-execution-" + resultID
	if existing, err := g.facts.GetInspection(ctx, id); err == nil {
		if existing.ValidateCurrent(g.now()) != nil || !sameInspectionExecutionFactV4(existing, reservation, observation, prepare, execute) {
			return contract.InspectionFact{}, errors.New("inspection execution Fact drifted")
		}
		return existing, nil
	} else if !errors.Is(err, sandboxports.ErrNotFound) {
		return contract.InspectionFact{}, err
	}
	evidence := []contract.Ref{evidenceRef(prepare.Binding), evidenceRef(execute.Binding)}
	body := struct {
		Reservation contract.Ref
		Observation contract.Ref
		Evidence    []contract.Ref
	}{reservation.Meta.Ref(), observation.Meta.Ref(), evidence}
	meta, err := contract.NewMeta(id, 1, g.now(), time.Unix(0, minimumInt64(reservation.Meta.ExpiresUnixNano, observation.Meta.ExpiresUnixNano, prepare.Qualification.ExpiresUnixNano, execute.Qualification.ExpiresUnixNano)), "governed-inspection-execution-v4", body)
	if err != nil {
		return contract.InspectionFact{}, err
	}
	value := contract.InspectionFact{Meta: meta, ReservationRef: reservation.Meta.Ref(), ObservationRef: observation.Meta.Ref(), OperationID: reservation.OperationID, AttemptID: reservation.AttemptID, Disposition: contract.DispositionConfirmedApplied, Coverage: []string{"attempt", "lease", "provider", "scope"}, EvidenceRefs: evidence}
	if err := g.controller.RecordInspection(ctx, value); err != nil {
		recovered, inspectErr := g.facts.GetInspection(context.WithoutCancel(ctx), id)
		if inspectErr != nil || recovered.ValidateCurrent(g.now()) != nil || !contract.SameRef(recovered.Meta.Ref(), value.Meta.Ref()) || !sameInspectionExecutionFactV4(recovered, reservation, observation, prepare, execute) {
			return contract.InspectionFact{}, err
		}
		return recovered, nil
	}
	return value, nil
}

func (g *GovernedInspectionV4) finalInspectionFact(request InspectProviderResultRequestV4, executionFact contract.InspectionFact, settlement runtimeports.OperationInspectionSettlementRefV4, prepare, execute ProviderPhaseResultV4) (contract.InspectionFact, error) {
	if execute.Response.ProviderObservation == nil {
		return contract.InspectionFact{}, errors.New("inspection execution omitted Provider observation")
	}
	state := execute.Response.ProviderObservation.State
	disposition := dispositionForProviderState(request.Reservation.Kind, execute.Dispatch.Payload.ProviderKind, state)
	var workspaceChangeSetRef *contract.Ref
	if request.Reservation.Kind == contract.EffectWorkspaceCommit {
		commit := execute.Response.ProviderObservation.WorkspaceCommit
		if commit == nil {
			return contract.InspectionFact{}, errors.New("workspace commit inspection lacks its exact ChangeSet observation")
		}
		ref := contract.Ref{ID: commit.ChangeSet.ID, Revision: commit.ChangeSet.Revision, Digest: strings.TrimPrefix(commit.ChangeSet.Digest, "sha256:")}
		if err := ref.ValidateShape("workspace change set ref"); err != nil {
			return contract.InspectionFact{}, err
		}
		workspaceChangeSetRef = &ref
		switch commit.State {
		case "committed":
			disposition = contract.DispositionConfirmedApplied
		case "not_applied":
			disposition = contract.DispositionConfirmedNotApplied
		case "indeterminate":
			disposition = contract.DispositionUnknown
		default:
			return contract.InspectionFact{}, errors.New("workspace commit inspection state is not terminal")
		}
	}
	settlementRef := contract.Ref{ID: settlement.Settlement.ID, Revision: uint64(settlement.Settlement.Revision), Digest: string(settlement.Settlement.Digest)}
	evidence := []contract.Ref{
		evidenceRef(prepare.Binding),
		evidenceRef(execute.Binding),
		settlementRef,
	}
	cleanup := cleanupReportForProviderState(request.Reservation.Kind, state, evidence)
	body := struct {
		Reservation           contract.Ref
		Observation           contract.Ref
		Inspection            contract.Ref
		Settlement            contract.Ref
		Disposition           contract.Disposition
		ObservedState         string
		Evidence              []contract.Ref
		Cleanup               *contract.CleanupReport
		WorkspaceChangeSetRef *contract.Ref
	}{request.Reservation.Meta.Ref(), request.Observation.Meta.Ref(), executionFact.Meta.Ref(), settlementRef, disposition, state, evidence, cleanup, workspaceChangeSetRef}
	meta, err := contract.NewMeta(request.Inspection.FinalInspectionID, 1, g.now(), time.Unix(0, minimumInt64(request.Reservation.Meta.ExpiresUnixNano, request.Observation.Meta.ExpiresUnixNano, executionFact.Meta.ExpiresUnixNano, settlement.ExpiresUnixNano, execute.Response.ExpiresUnixNano)), "governed-provider-inspection-v4", body)
	if err != nil {
		return contract.InspectionFact{}, err
	}
	coverage := []string{"attempt", "lease", "provider", "scope"}
	if cleanup != nil {
		coverage = []string{"attempt", "background_tasks", "file_mounts", "lease", "network", "processes", "provider", "provider_retention", "remote_continuation", "scope", "secrets"}
	}
	value := contract.InspectionFact{
		Meta: meta, ReservationRef: request.Reservation.Meta.Ref(), ObservationRef: request.Observation.Meta.Ref(),
		OperationID: request.Reservation.OperationID, AttemptID: request.Reservation.AttemptID,
		Disposition: disposition, Coverage: coverage, EvidenceRefs: evidence, Cleanup: cleanup,
		WorkspaceChangeSetRef: workspaceChangeSetRef,
	}
	if err := value.ValidateCurrent(g.now()); err != nil {
		return contract.InspectionFact{}, err
	}
	return value, nil
}

func validateGovernedInspectionPlanV4(request InspectProviderResultRequestV4, reservation contract.DomainReservation) error {
	plan := request.Inspection
	if reservation.Kind != contract.EffectInspect || strings.TrimSpace(plan.FinalInspectionID) == "" {
		return errors.New("governed inspection requires an inspect reservation and final Fact identity")
	}
	lifecycle := LifecyclePlanV4{ReservationID: plan.ReservationID, Prepare: plan.Prepare, Execute: plan.Execute, DeclaredDelegation: plan.DeclaredDelegation, ResultID: plan.ResultID, Settlement: plan.Settlement}
	if err := validateLifecyclePlanV4(lifecycle, reservation); err != nil {
		return err
	}
	if !runtimeports.SameOperationSubjectV3(plan.Prepare.Enforcement.Operation, request.Prepare.Current.Sandbox.Operation) ||
		!runtimeports.SameOperationSubjectV3(request.Prepare.Current.Sandbox.Operation, request.Execute.Current.Sandbox.Operation) ||
		request.Prepare.Current.Sandbox.ProviderBinding != request.Execute.Current.Sandbox.ProviderBinding ||
		plan.Prepare.Enforcement.Verifier != request.Execute.Current.Sandbox.ProviderBinding || plan.Execute.Enforcement.Verifier != request.Execute.Current.Sandbox.ProviderBinding ||
		plan.Prepare.Payload.ProviderKind != request.Execute.Dispatch.Payload.ProviderKind || plan.Execute.Payload.ProviderKind != request.Execute.Dispatch.Payload.ProviderKind ||
		reservation.OperationID != request.Reservation.OperationID || reservation.EffectID == request.Reservation.EffectID || reservation.AttemptID == request.Reservation.AttemptID {
		return errors.New("inspection Effect does not inherit the original Operation or has reused original identities")
	}
	prepareTarget, err := plan.Prepare.Payload.InspectionTarget()
	if err != nil || prepareTarget == nil {
		return errors.New("inspection prepare lacks the exact original target")
	}
	executeTarget, err := plan.Execute.Payload.InspectionTarget()
	if err != nil || executeTarget == nil || *prepareTarget != *executeTarget {
		return errors.New("inspection phases bind different original targets")
	}
	providerAttempt := request.Execute.Response.ProviderAttempt
	if providerAttempt == nil || prepareTarget.OriginalEffectKind != string(request.Reservation.Kind) || prepareTarget.OriginalAttemptID != request.Reservation.AttemptID || prepareTarget.ProviderAttempt != *providerAttempt || prepareTarget.OriginalRequestDigest != request.Execute.Dispatch.Digest || prepareTarget.OriginalPayloadDigest != request.Execute.Dispatch.PayloadDigest {
		return errors.New("inspection target does not equal the original Provider attempt")
	}
	return nil
}

func sameInspectionExecutionFactV4(value contract.InspectionFact, reservation contract.DomainReservation, observation contract.Observation, prepare, execute ProviderPhaseResultV4) bool {
	return contract.SameRef(value.ReservationRef, reservation.Meta.Ref()) && contract.SameRef(value.ObservationRef, observation.Meta.Ref()) &&
		value.OperationID == reservation.OperationID && value.AttemptID == reservation.AttemptID && value.Disposition == contract.DispositionConfirmedApplied &&
		len(value.EvidenceRefs) == 2 && value.EvidenceRefs[0] == evidenceRef(prepare.Binding) && value.EvidenceRefs[1] == evidenceRef(execute.Binding)
}

func dispositionForProviderState(kind contract.EffectKind, providerKind, state string) contract.Disposition {
	switch kind {
	case contract.EffectAllocate:
		if state == "container_prepared" || state == "allocated" {
			return contract.DispositionConfirmedApplied
		}
	case contract.EffectActivate, contract.EffectOpen:
		if providerKind == "wasmtime_component" && (state == "executing" || strings.HasPrefix(state, "exited:")) {
			return contract.DispositionConfirmedApplied
		}
		for _, prefix := range []string{"task:running:pid:", "task:stopped:pid:", "task:paused:pid:", "task:pausing:pid:"} {
			if strings.HasPrefix(state, prefix) {
				return contract.DispositionConfirmedApplied
			}
		}
	case contract.EffectCancel, contract.EffectClose, contract.EffectFence:
		if state == "fenced" || state == "not_found" || state == "container_prepared" || strings.HasPrefix(state, "exited:") || strings.HasPrefix(state, "task:stopped:pid:") {
			return contract.DispositionConfirmedApplied
		}
	case contract.EffectRelease:
		if state == "not_found" || state == "released" {
			return contract.DispositionConfirmedApplied
		}
	case contract.EffectCleanup:
		if state == "cleanup_absent" {
			return contract.DispositionConfirmedApplied
		}
	case contract.EffectWorkspaceCommit:
		if strings.HasPrefix(state, "workspace_committed:sha256:") {
			return contract.DispositionConfirmedApplied
		}
		if state == "workspace_commit_not_applied" {
			return contract.DispositionConfirmedNotApplied
		}
	}
	return contract.DispositionUnknown
}

func cleanupReportForProviderState(kind contract.EffectKind, state string, evidence []contract.Ref) *contract.CleanupReport {
	if kind != contract.EffectCleanup || state != "cleanup_absent" {
		return nil
	}
	return &contract.CleanupReport{
		Processes: contract.CleanupConfirmedClean, FileMounts: contract.CleanupConfirmedClean,
		Network: contract.CleanupConfirmedClean, Secrets: contract.CleanupConfirmedClean,
		BackgroundTasks: contract.CleanupConfirmedClean, RemoteContinuation: contract.CleanupConfirmedClean,
		ProviderRetention: contract.CleanupConfirmedClean, EvidenceRefs: append([]contract.Ref(nil), evidence...),
	}
}
