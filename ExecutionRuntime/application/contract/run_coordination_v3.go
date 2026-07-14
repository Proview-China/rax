package contract

import (
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const RunCoordinationContractVersionV3 = "3.0.0"

type RunCoordinationStateV3 string

const (
	RunCoordinationCreatePlannedV3     RunCoordinationStateV3 = "create_planned"
	RunCoordinationPendingV3           RunCoordinationStateV3 = "pending"
	RunCoordinationStartPlannedV3      RunCoordinationStateV3 = "start_planned"
	RunCoordinationRunningV3           RunCoordinationStateV3 = "running"
	RunCoordinationClaimPlannedV3      RunCoordinationStateV3 = "claim_planned"
	RunCoordinationClaimAssociatedV3   RunCoordinationStateV3 = "claim_associated"
	RunCoordinationStopPlannedV3       RunCoordinationStateV3 = "stop_planned"
	RunCoordinationStoppingV3          RunCoordinationStateV3 = "stopping"
	RunCoordinationTerminalCleanupV3   RunCoordinationStateV3 = "terminal_cleanup"
	RunCoordinationTerminationClosedV3 RunCoordinationStateV3 = "termination_closed"
)

// RunCoordinationFactV3 is the Application-owned restart watermark. It binds
// workflow intent to Runtime-owned facts, but never becomes a Run, Outcome,
// Claim, settlement or termination authority itself.
type RunCoordinationFactV3 struct {
	ContractVersion string                 `json:"contract_version"`
	ID              string                 `json:"id"`
	Revision        core.Revision          `json:"revision"`
	State           RunCoordinationStateV3 `json:"state"`

	Scope           core.ExecutionScope           `json:"scope"`
	ScopeDigest     core.Digest                   `json:"scope_digest"`
	PlanID          string                        `json:"plan_id"`
	PlanRevision    core.Revision                 `json:"plan_revision"`
	PlanDigest      core.Digest                   `json:"plan_digest"`
	JournalID       string                        `json:"journal_id"`
	JournalRevision core.Revision                 `json:"journal_revision"`
	JournalDigest   core.Digest                   `json:"journal_digest"`
	StepID          string                        `json:"step_id"`
	StepKind        runtimeports.NamespacedNameV2 `json:"step_kind"`

	StartAttemptInitial         GovernedOperationAttemptRefV3          `json:"start_attempt_initial"`
	StartAttemptPlanID          string                                 `json:"start_attempt_plan_id"`
	StartAttemptPlanRevision    core.Revision                          `json:"start_attempt_plan_revision"`
	StartAttemptPlanDigest      core.Digest                            `json:"start_attempt_plan_digest"`
	StartAttemptJournalRevision core.Revision                          `json:"start_attempt_journal_revision"`
	StartAttemptJournalDigest   core.Digest                            `json:"start_attempt_journal_digest"`
	CreateRequest               runtimeports.CreatePendingRunRequestV3 `json:"create_request"`
	Lifecycle                   *runtimeports.RunLifecycleEnvelopeV3   `json:"lifecycle,omitempty"`

	StartAttempt        *GovernedOperationAttemptFactV3              `json:"start_attempt,omitempty"`
	StartOperation      *runtimeports.OperationSubjectV3             `json:"start_operation,omitempty"`
	StartRuntimeAttempt *runtimeports.GovernedExecutionAttemptRefsV2 `json:"start_runtime_attempt,omitempty"`
	StartConfirmation   *runtimeports.RunStartConfirmationFactV3     `json:"start_confirmation,omitempty"`

	ClaimCandidate *runtimeports.EvidenceEventCandidateV2 `json:"claim_candidate,omitempty"`
	ClaimResult    *runtimeports.RunClaimIngestResultV3   `json:"claim_result,omitempty"`

	CreatedUnixNano int64 `json:"created_unix_nano"`
	UpdatedUnixNano int64 `json:"updated_unix_nano"`
}

func NewRunCoordinationFactV3(id string, plan WorkflowPlanV2, journal WorkflowJournalV2, stepID string, initial GovernedOperationAttemptFactV3, create runtimeports.CreatePendingRunRequestV3, nowUnixNano int64) (RunCoordinationFactV3, error) {
	if err := journal.ValidateFor(plan); err != nil {
		return RunCoordinationFactV3{}, err
	}
	if err := create.Validate(); err != nil {
		return RunCoordinationFactV3{}, err
	}
	if err := initial.Validate(); err != nil {
		return RunCoordinationFactV3{}, err
	}
	planDigest, err := plan.DigestV2()
	if err != nil {
		return RunCoordinationFactV3{}, err
	}
	journalDigest, err := journal.DigestV2(plan)
	if err != nil {
		return RunCoordinationFactV3{}, err
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(plan.Target)
	if err != nil {
		return RunCoordinationFactV3{}, err
	}
	initialRef, err := initial.RefV3()
	if err != nil {
		return RunCoordinationFactV3{}, err
	}
	provider := runtimeports.EvidenceProducerBindingRefV2(initial.PlannedProvider)
	if create.Plan.Execution.EndpointID != initial.DelegationPlan.EndpointID || create.Plan.Execution.SessionRef != initial.DelegationPlan.RuntimeSessionRef || create.Plan.Execution.Binding != provider {
		return RunCoordinationFactV3{}, core.NewError(core.ErrorConflict, core.ReasonExecutionInspectionInvalid, "Run create Plan differs from the execution-start delegation/provider")
	}
	f := RunCoordinationFactV3{
		ContractVersion: RunCoordinationContractVersionV3,
		ID:              id, Revision: 1, State: RunCoordinationCreatePlannedV3,
		Scope: plan.Target, ScopeDigest: scopeDigest,
		PlanID: plan.ID, PlanRevision: plan.Revision, PlanDigest: planDigest,
		JournalID: journal.ID, JournalRevision: journal.Revision, JournalDigest: journalDigest,
		StepID: stepID, StepKind: initial.StepKind,
		StartAttemptInitial: initialRef, CreateRequest: create,
		StartAttemptPlanID: initial.PlanID, StartAttemptPlanRevision: initial.PlanRevision, StartAttemptPlanDigest: initial.PlanDigest, StartAttemptJournalRevision: initial.JournalRevision, StartAttemptJournalDigest: initial.JournalDigest,
		CreatedUnixNano: nowUnixNano, UpdatedUnixNano: nowUnixNano,
	}
	return f, f.Validate()
}

func (f RunCoordinationFactV3) Validate() error {
	if f.ContractVersion != RunCoordinationContractVersionV3 || strings.TrimSpace(f.ID) == "" || len(f.ID) > 512 || f.Revision == 0 || strings.TrimSpace(f.PlanID) == "" || f.PlanRevision == 0 || strings.TrimSpace(f.JournalID) == "" || f.JournalRevision == 0 || strings.TrimSpace(f.StepID) == "" || f.CreatedUnixNano <= 0 || f.UpdatedUnixNano < f.CreatedUnixNano {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "run coordination identity is incomplete")
	}
	if err := f.Scope.Validate(); err != nil {
		return err
	}
	scopeDigest, err := runtimeports.ExecutionScopeDigestV2(f.Scope)
	if err != nil || scopeDigest != f.ScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEffectFenceStale, "run coordination scope drifted")
	}
	for _, digest := range []core.Digest{f.PlanDigest, f.JournalDigest} {
		if err := digest.Validate(); err != nil {
			return err
		}
	}
	if err := runtimeports.ValidateNamespacedNameV2(f.StepKind); err != nil {
		return err
	}
	if err := f.StartAttemptInitial.Validate(); err != nil {
		return err
	}
	if f.StartAttemptInitial.Revision != 1 || f.StartAttemptInitial.State != OperationIntentRecordedV3 || f.StartAttemptInitial.JournalID != f.JournalID || f.StartAttemptInitial.StepID != f.StepID || f.StartAttemptInitial.StepKind != f.StepKind || f.StartAttemptInitial.ScopeDigest != f.ScopeDigest {
		return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "run coordination workflow and initial operation attempt differ")
	}
	if f.StartAttemptPlanID != f.PlanID || f.StartAttemptPlanRevision != f.PlanRevision || f.StartAttemptPlanDigest != f.PlanDigest || f.StartAttemptJournalRevision != f.JournalRevision || f.StartAttemptJournalDigest != f.JournalDigest {
		return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "initial execution-start Attempt Plan/Journal anchors drifted")
	}
	if err := f.CreateRequest.Validate(); err != nil {
		return err
	}
	if !runtimeports.SameExecutionScopeV2(f.CreateRequest.Run.Scope, f.Scope) || !runtimeports.SameExecutionScopeV2(f.CreateRequest.Plan.ExecutionScope, f.Scope) {
		return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "run create request belongs to another workflow scope")
	}
	if f.StartOperation != nil {
		if err := f.StartOperation.Validate(); err != nil {
			return err
		}
		if f.StartOperation.Kind != runtimeports.OperationScopeRunV3 || f.StartOperation.RunID != f.CreateRequest.Run.ID || !runtimeports.SameExecutionScopeV2(f.StartOperation.ExecutionScope, f.Scope) {
			return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "execution-start operation belongs to another Run")
		}
	}
	if f.StartAttempt != nil {
		if err := f.StartAttempt.Validate(); err != nil {
			return err
		}
		settledRef, err := f.StartAttempt.RefV3()
		if err != nil || !sameRunCoordinationAttemptRoutingV3(f.StartAttemptInitial, settledRef) || f.StartAttempt.State != OperationSettledV3 || f.StartAttempt.Settlement == nil {
			return core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "run start requires the exact settled workflow attempt")
		}
		if f.StartAttempt.PlanID != f.PlanID || f.StartAttempt.PlanRevision != f.PlanRevision || f.StartAttempt.PlanDigest != f.PlanDigest || f.StartAttempt.JournalID != f.JournalID || f.StartAttempt.JournalRevision != f.JournalRevision || f.StartAttempt.JournalDigest != f.JournalDigest {
			return core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "settled execution-start attempt belongs to another Plan or Journal revision")
		}
	}
	if f.StartRuntimeAttempt != nil {
		if err := f.StartRuntimeAttempt.ValidatePrepared(); err != nil {
			return err
		}
		if f.StartRuntimeAttempt.Observation == nil || f.StartRuntimeAttempt.Settlement == nil || f.StartRuntimeAttempt.Settlement.Disposition != runtimeports.OperationSettlementAppliedV3 {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "run start Runtime attempt lacks applied settlement")
		}
	}
	if f.StartAttempt != nil && f.StartOperation != nil && f.StartRuntimeAttempt != nil {
		operationDigest, err := f.StartOperation.DigestV3()
		if err != nil || operationDigest != f.StartAttempt.Intent.OperationDigest || operationDigest != f.StartRuntimeAttempt.Admission.OperationDigest || f.StartAttempt.IntentValue.Kind != runtimeports.OperationEffectKindExecutionStartV3 || f.StartAttempt.IntentValue.ID != f.StartRuntimeAttempt.Admission.EffectID || f.StartAttempt.IntentValue.Revision != f.StartRuntimeAttempt.Admission.IntentRevision {
			return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "execution-start operation, Application attempt and Runtime attempt differ")
		}
		intentDigest, err := f.StartAttempt.IntentValue.DigestV3()
		if err != nil || intentDigest != f.StartRuntimeAttempt.Admission.IntentDigest || !sameRunCoordinationValueV3("runtime-attempt", runCoordinationRuntimeRefsV3(*f.StartAttempt), *f.StartRuntimeAttempt) {
			return core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "execution-start Runtime refs drifted from settled Application attempt")
		}
	}
	if f.StartConfirmation != nil {
		if err := f.StartConfirmation.Validate(); err != nil {
			return err
		}
		if f.StartOperation == nil || f.StartRuntimeAttempt == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunConflict, "Run start confirmation lacks its persisted operation/attempt")
		}
		operationDigest, err := f.StartOperation.DigestV3()
		if err != nil || f.StartConfirmation.RunID != f.CreateRequest.Run.ID || !runtimeports.SameExecutionScopeV2(f.StartConfirmation.ExecutionScope, f.Scope) || f.StartConfirmation.OperationDigest != operationDigest || !sameRunCoordinationValueV3("start-confirmation-attempt", f.StartConfirmation.Attempt, *f.StartRuntimeAttempt) {
			return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "Run start confirmation does not bind the exact settled start attempt")
		}
	}
	if f.StartAttempt != nil {
		provider := runtimeports.EvidenceProducerBindingRefV2(f.StartAttempt.PlannedProvider)
		if f.CreateRequest.Plan.Execution.EndpointID != f.StartAttempt.DelegationPlan.EndpointID || f.CreateRequest.Plan.Execution.SessionRef != f.StartAttempt.DelegationPlan.RuntimeSessionRef || f.CreateRequest.Plan.Execution.Binding != provider {
			return core.NewError(core.ErrorConflict, core.ReasonExecutionInspectionInvalid, "Run settlement Plan execution subject differs from the governed start delegation/provider")
		}
	}
	if f.Lifecycle != nil {
		if err := f.Lifecycle.Validate(); err != nil {
			return err
		}
		if f.Lifecycle.Run.ID != f.CreateRequest.Run.ID || !runtimeports.SameExecutionScopeV2(f.Lifecycle.Run.Scope, f.Scope) {
			return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "run lifecycle belongs to another Run")
		}
		planRef, err := f.CreateRequest.Plan.RefV2()
		if err != nil || f.Lifecycle.Plan.RunSettlementPlanRefV2 != planRef || f.Lifecycle.Certification != f.CreateRequest.Certification || f.Lifecycle.EffectIndex.ID != f.CreateRequest.EffectIndexID {
			return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "run lifecycle Plan or Effect index differs from create intent")
		}
		if f.Lifecycle.Phase != runtimeports.RunLifecyclePendingPreparedV3 && (f.StartRuntimeAttempt == nil || f.StartRuntimeAttempt.Observation == nil || f.Lifecycle.Run.StartedAt.UnixNano() != f.StartRuntimeAttempt.Observation.ObservedUnixNano) {
			return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "Run StartedAt does not bind the settled execution-start observation")
		}
	}
	if f.ClaimCandidate != nil {
		if err := f.ClaimCandidate.Validate(); err != nil {
			return err
		}
		ledger := f.ClaimCandidate.LedgerScope
		if ledger.Partition != runtimeports.EvidencePartitionRun || ledger.RunID != f.CreateRequest.Run.ID || ledger.TenantID != f.Scope.Identity.TenantID || ledger.IdentityID != f.Scope.Identity.ID || ledger.LineageID != f.Scope.Lineage.ID || ledger.InstanceID != f.Scope.Instance.ID || !runtimeports.SameExecutionScopeV2(f.ClaimCandidate.ExecutionScope, f.Scope) {
			return core.NewError(core.ErrorConflict, core.ReasonRunClaimConflict, "claim candidate belongs to another Run")
		}
	}
	if f.ClaimResult != nil {
		if err := f.ClaimResult.Validate(); err != nil {
			return err
		}
		claimIdentity, identityErr := runtimeports.RunIdentityDigestV2(f.ClaimResult.Run)
		if f.ClaimCandidate == nil || f.Lifecycle == nil || f.ClaimResult.Certification != f.CreateRequest.Certification || f.ClaimResult.Plan != f.Lifecycle.Plan || f.ClaimResult.Run.ID != f.CreateRequest.Run.ID || f.ClaimResult.Run.SessionRef != f.CreateRequest.Run.SessionRef || !runtimeports.SameExecutionScopeV2(f.ClaimResult.Run.Scope, f.Scope) || identityErr != nil || claimIdentity != f.CreateRequest.Plan.RunIdentityDigest || !sameRunClaimCandidateV3(*f.ClaimCandidate, f.ClaimResult.Evidence.Candidate) {
			return core.NewError(core.ErrorConflict, core.ReasonRunClaimConflict, "claim result does not bind the planned candidate")
		}
	}
	return f.validateStateV3()
}

func (f RunCoordinationFactV3) validateStateV3() error {
	hasLifecycle := f.Lifecycle != nil
	hasStart := f.StartAttempt != nil || f.StartOperation != nil || f.StartRuntimeAttempt != nil
	hasConfirmation := f.StartConfirmation != nil
	hasCandidate, hasClaim := f.ClaimCandidate != nil, f.ClaimResult != nil
	requireStart := func() bool { return f.StartAttempt != nil && f.StartOperation != nil && f.StartRuntimeAttempt != nil }
	switch f.State {
	case RunCoordinationCreatePlannedV3:
		if hasLifecycle || hasStart || hasConfirmation || hasCandidate || hasClaim {
			return invalidRunCoordinationStateV3("create plan carries later facts")
		}
	case RunCoordinationPendingV3:
		if !hasLifecycle || f.Lifecycle.Phase != runtimeports.RunLifecyclePendingPreparedV3 || hasStart || hasConfirmation || hasCandidate || hasClaim {
			return invalidRunCoordinationStateV3("pending watermark is incomplete")
		}
	case RunCoordinationStartPlannedV3:
		if !hasLifecycle || f.Lifecycle.Phase != runtimeports.RunLifecyclePendingPreparedV3 || !requireStart() || hasConfirmation || hasCandidate || hasClaim {
			return invalidRunCoordinationStateV3("start plan is incomplete")
		}
	case RunCoordinationRunningV3:
		if !hasLifecycle || f.Lifecycle.Phase != runtimeports.RunLifecycleRunningV3 || !requireStart() || !hasConfirmation || hasCandidate || hasClaim {
			return invalidRunCoordinationStateV3("running watermark is incomplete")
		}
	case RunCoordinationClaimPlannedV3:
		if !hasLifecycle || f.Lifecycle.Phase != runtimeports.RunLifecycleRunningV3 || !requireStart() || !hasConfirmation || !hasCandidate || hasClaim {
			return invalidRunCoordinationStateV3("claim plan is incomplete")
		}
	case RunCoordinationClaimAssociatedV3:
		if !hasLifecycle || f.Lifecycle.Phase != runtimeports.RunLifecycleRunningV3 || !requireStart() || !hasConfirmation || !hasCandidate || !hasClaim {
			return invalidRunCoordinationStateV3("claim association is incomplete")
		}
	case RunCoordinationStopPlannedV3:
		if !hasLifecycle || f.Lifecycle.Phase != runtimeports.RunLifecycleRunningV3 || !requireStart() || !hasConfirmation || hasCandidate != hasClaim {
			return invalidRunCoordinationStateV3("stop plan is incomplete")
		}
	case RunCoordinationStoppingV3:
		if !hasLifecycle || f.Lifecycle.Phase != runtimeports.RunLifecycleStoppingV3 || !requireStart() || !hasConfirmation || hasCandidate != hasClaim {
			return invalidRunCoordinationStateV3("stopping watermark is incomplete")
		}
	case RunCoordinationTerminalCleanupV3:
		if !hasLifecycle || f.Lifecycle.Phase != runtimeports.RunLifecycleTerminalCleanupV3 || !requireStart() || !hasConfirmation || hasCandidate != hasClaim {
			return invalidRunCoordinationStateV3("terminal cleanup watermark is incomplete")
		}
	case RunCoordinationTerminationClosedV3:
		if !hasLifecycle || f.Lifecycle.Phase != runtimeports.RunLifecycleTerminationClosedV3 || !requireStart() || !hasConfirmation || hasCandidate != hasClaim {
			return invalidRunCoordinationStateV3("termination close watermark is incomplete")
		}
	default:
		return invalidRunCoordinationStateV3("unknown run coordination state")
	}
	return nil
}

func (f RunCoordinationFactV3) DigestV3() (core.Digest, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	return core.CanonicalJSONDigest("praxis.application.run-coordination", RunCoordinationContractVersionV3, "RunCoordinationFactV3", f)
}

func ValidateRunCoordinationTransitionV3(current, next RunCoordinationFactV3) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if next.Revision != current.Revision+1 || next.UpdatedUnixNano < current.UpdatedUnixNano || !allowedRunCoordinationTransitionV3(current.State, next.State) {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "run coordination CAS must advance one authorized stage")
	}
	a, b := current, next
	a.Revision, b.Revision = 0, 0
	a.State, b.State = "", ""
	a.UpdatedUnixNano, b.UpdatedUnixNano = 0, 0
	a.Lifecycle, b.Lifecycle = nil, nil
	a.StartAttempt, b.StartAttempt = nil, nil
	a.StartOperation, b.StartOperation = nil, nil
	a.StartRuntimeAttempt, b.StartRuntimeAttempt = nil, nil
	a.StartConfirmation, b.StartConfirmation = nil, nil
	a.ClaimCandidate, b.ClaimCandidate = nil, nil
	a.ClaimResult, b.ClaimResult = nil, nil
	ad, _ := core.CanonicalJSONDigest("praxis.application.run-coordination", RunCoordinationContractVersionV3, "immutable", a)
	bd, _ := core.CanonicalJSONDigest("praxis.application.run-coordination", RunCoordinationContractVersionV3, "immutable", b)
	if ad != bd {
		return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "run coordination immutable workflow or Run identity changed")
	}
	for name, pair := range map[string][2]any{
		"settled-start-attempt": {current.StartAttempt, next.StartAttempt},
		"start-operation":       {current.StartOperation, next.StartOperation},
		"start-runtime-attempt": {current.StartRuntimeAttempt, next.StartRuntimeAttempt},
		"start-confirmation":    {current.StartConfirmation, next.StartConfirmation},
		"claim-candidate":       {current.ClaimCandidate, next.ClaimCandidate},
		"claim-result":          {current.ClaimResult, next.ClaimResult},
	} {
		if !runCoordinationAppendOnlyV3(name, pair[0], pair[1]) {
			return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "run coordination "+name+" changed after persistence")
		}
	}
	if current.Lifecycle != nil {
		if next.Lifecycle == nil || !validRunLifecycleAdvanceV3(*current.Lifecycle, *next.Lifecycle) {
			return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "run lifecycle regressed or changed Plan/Effect index identity")
		}
	}
	return nil
}

func allowedRunCoordinationTransitionV3(current, next RunCoordinationStateV3) bool {
	switch current {
	case RunCoordinationCreatePlannedV3:
		return next == RunCoordinationPendingV3
	case RunCoordinationPendingV3:
		return next == RunCoordinationStartPlannedV3
	case RunCoordinationStartPlannedV3:
		return next == RunCoordinationRunningV3
	case RunCoordinationRunningV3:
		return next == RunCoordinationClaimPlannedV3 || next == RunCoordinationStopPlannedV3
	case RunCoordinationClaimPlannedV3:
		return next == RunCoordinationClaimAssociatedV3
	case RunCoordinationClaimAssociatedV3:
		return next == RunCoordinationStopPlannedV3
	case RunCoordinationStopPlannedV3:
		return next == RunCoordinationStoppingV3 || next == RunCoordinationTerminalCleanupV3 || next == RunCoordinationTerminationClosedV3
	case RunCoordinationStoppingV3:
		return next == RunCoordinationTerminalCleanupV3 || next == RunCoordinationTerminationClosedV3
	case RunCoordinationTerminalCleanupV3:
		return next == RunCoordinationTerminalCleanupV3 || next == RunCoordinationTerminationClosedV3
	default:
		return false
	}
}

func sameRunCoordinationAttemptRoutingV3(a, b GovernedOperationAttemptRefV3) bool {
	return a.ID == b.ID && a.ScopeDigest == b.ScopeDigest && a.JournalID == b.JournalID && a.StepID == b.StepID && a.StepKind == b.StepKind && a.Descriptor == b.Descriptor && a.PlannedProvider == b.PlannedProvider && a.DomainAdapter == b.DomainAdapter && a.PlanAuthority == b.PlanAuthority && a.RoutingDigest == b.RoutingDigest && a.WorkflowAttempt == b.WorkflowAttempt && a.OperationDigest == b.OperationDigest && a.EffectID == b.EffectID
}

func sameRunClaimCandidateV3(a, b runtimeports.EvidenceEventCandidateV2) bool {
	ad, ae := a.DigestV2()
	bd, be := b.DigestV2()
	return ae == nil && be == nil && ad == bd
}

func runCoordinationRuntimeRefsV3(fact GovernedOperationAttemptFactV3) runtimeports.GovernedExecutionAttemptRefsV2 {
	a := fact.BegunAuthorization.Attempt
	refs := runtimeports.GovernedExecutionAttemptRefsV2{Admission: *fact.Admission, PermitID: a.PermitID, PermitRevision: a.PermitRevision, PermitDigest: a.PermitDigest, AttemptID: a.AttemptID, Delegation: *fact.PreparedDelegation, Prepared: *fact.Prepared, Enforcement: *fact.Enforcement}
	if fact.Observation != nil {
		value := *fact.Observation
		refs.Observation = &value
	}
	if fact.Settlement != nil {
		value := *fact.Settlement
		value.Evidence = append([]runtimeports.EvidenceRecordRefV2(nil), fact.Settlement.Evidence...)
		refs.Settlement = &value
	}
	return refs
}

func sameRunCoordinationValueV3(name string, a, b any) bool {
	ad, ae := core.CanonicalJSONDigest("praxis.application.run-coordination", RunCoordinationContractVersionV3, name, a)
	bd, be := core.CanonicalJSONDigest("praxis.application.run-coordination", RunCoordinationContractVersionV3, name, b)
	return ae == nil && be == nil && ad == bd
}

func runCoordinationAppendOnlyV3(name string, current, next any) bool {
	if isNilRunCoordinationV3(current) {
		return true
	}
	if isNilRunCoordinationV3(next) {
		return false
	}
	return sameRunCoordinationValueV3(name, current, next)
}

func isNilRunCoordinationV3(value any) bool {
	switch v := value.(type) {
	case *GovernedOperationAttemptFactV3:
		return v == nil
	case *runtimeports.OperationSubjectV3:
		return v == nil
	case *runtimeports.GovernedExecutionAttemptRefsV2:
		return v == nil
	case *runtimeports.RunStartConfirmationFactV3:
		return v == nil
	case *runtimeports.EvidenceEventCandidateV2:
		return v == nil
	case *runtimeports.RunClaimIngestResultV3:
		return v == nil
	default:
		return value == nil
	}
}

func validRunLifecycleAdvanceV3(current, next runtimeports.RunLifecycleEnvelopeV3) bool {
	if current.Plan != next.Plan || current.Certification != next.Certification || current.Run.ID != next.Run.ID || current.Run.SessionRef != next.Run.SessionRef || !runtimeports.SameExecutionScopeV2(current.Run.Scope, next.Run.Scope) || next.Run.Revision < current.Run.Revision || current.EffectIndex.ID != next.EffectIndex.ID || current.EffectIndex.RunID != next.EffectIndex.RunID || current.EffectIndex.RunIdentityDigest != next.EffectIndex.RunIdentityDigest || current.EffectIndex.ExecutionScopeDigest != next.EffectIndex.ExecutionScopeDigest || next.EffectIndex.Revision < current.EffectIndex.Revision || next.EffectIndex.Watermark < current.EffectIndex.Watermark || next.EffectIndex.SegmentCount < current.EffectIndex.SegmentCount || next.EffectIndex.EffectCount < current.EffectIndex.EffectCount || current.EffectIndex.Frozen && !next.EffectIndex.Frozen {
		return false
	}
	if !current.Run.StartedAt.IsZero() && !current.Run.StartedAt.Equal(next.Run.StartedAt) || current.Run.CompletionClaim != nil && (next.Run.CompletionClaim == nil || *current.Run.CompletionClaim != *next.Run.CompletionClaim) || !current.Run.EndedAt.IsZero() && !current.Run.EndedAt.Equal(next.Run.EndedAt) || current.Run.Outcome != "" && current.Run.Outcome != next.Run.Outcome {
		return false
	}
	if current.Run.Revision == next.Run.Revision && !sameRunCoordinationValueV3("run-record", current.Run, next.Run) || current.EffectIndex.Revision == next.EffectIndex.Revision && current.EffectIndex != next.EffectIndex || current.EffectIndex.Frozen && current.EffectIndex != next.EffectIndex {
		return false
	}
	if !runLifecyclePhaseReachableV3(current.Phase, next.Phase) || !appendOnlyLifecycleRefV3("closure", current.Closure, next.Closure) || !appendOnlyLifecycleRefV3("decision", current.Decision, next.Decision) || !appendOnlyLifecycleRefV3("report", current.Report, next.Report) {
		return false
	}
	if current.Progress != nil {
		if next.Progress == nil || current.Progress.ID != next.Progress.ID || current.Progress.RunID != next.Progress.RunID || current.Progress.RunIdentityDigest != next.Progress.RunIdentityDigest || current.Progress.ExecutionScopeDigest != next.Progress.ExecutionScopeDigest || current.Progress.Decision != next.Progress.Decision || next.Progress.Revision < current.Progress.Revision || next.Progress.UnresolvedCount > current.Progress.UnresolvedCount || current.Progress.Revision == next.Progress.Revision && *current.Progress != *next.Progress {
			return false
		}
	}
	return true
}

// ValidateRunLifecycleAdvanceV3 verifies the causal Runtime projection used
// when recovering a lost Application CAS reply.
func ValidateRunLifecycleAdvanceV3(current, next runtimeports.RunLifecycleEnvelopeV3) error {
	if !validRunLifecycleAdvanceV3(current, next) {
		return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "Run lifecycle projection is not a causal successor")
	}
	return nil
}

func appendOnlyLifecycleRefV3(name string, current, next any) bool {
	if lifecycleRefNilV3(current) {
		return true
	}
	if lifecycleRefNilV3(next) {
		return false
	}
	return sameRunCoordinationValueV3("lifecycle-"+name, current, next)
}

func lifecycleRefNilV3(value any) bool {
	switch v := value.(type) {
	case *runtimeports.RunSettlementClosureRefV3:
		return v == nil
	case *runtimeports.RunSettlementDecisionRefV3:
		return v == nil
	case *runtimeports.RunTerminationReportRefV3:
		return v == nil
	default:
		return value == nil
	}
}

func runLifecyclePhaseReachableV3(from, to runtimeports.RunLifecyclePhaseV3) bool {
	order := map[runtimeports.RunLifecyclePhaseV3]int{runtimeports.RunLifecyclePendingPreparedV3: 0, runtimeports.RunLifecycleRunningV3: 1, runtimeports.RunLifecycleStoppingV3: 2, runtimeports.RunLifecycleTerminalCleanupV3: 3, runtimeports.RunLifecycleTerminationClosedV3: 4}
	a, aok := order[from]
	b, bok := order[to]
	return aok && bok && b >= a
}

func invalidRunCoordinationStateV3(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, message)
}
