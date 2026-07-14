package application

import (
	"context"
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type RunCoordinatorConfigV3 struct {
	Facts     applicationports.RunCoordinationFactPortV3
	Journals  applicationports.WorkflowJournalFactPortV2
	Attempts  applicationports.GovernedOperationAttemptFactPortV3
	Assembler runtimeports.TrustedRunAssemblerPortV3
	Lifecycle runtimeports.RunLifecycleGovernancePortV3
	Start     runtimeports.RunStartGovernancePortV3
	Claims    runtimeports.RunClaimIngestGovernancePortV3
	Clock     func() time.Time
}

type RunCoordinatorV3 struct{ config RunCoordinatorConfigV3 }

func NewRunCoordinatorV3(config RunCoordinatorConfigV3) (*RunCoordinatorV3, error) {
	if config.Facts == nil || config.Journals == nil || config.Attempts == nil || config.Assembler == nil || config.Lifecycle == nil || config.Start == nil || config.Claims == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Application Run coordinator requires Fact, workflow, Runtime lifecycle, start and claim owners")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &RunCoordinatorV3{config: config}, nil
}

type PrepareRunCoordinationRequestV3 struct {
	CoordinationID string                                 `json:"coordination_id"`
	Plan           contract.WorkflowPlanV2                `json:"workflow_plan"`
	JournalID      string                                 `json:"journal_id"`
	StepID         string                                 `json:"step_id"`
	StartAttemptID string                                 `json:"start_attempt_id"`
	Create         runtimeports.CreatePendingRunRequestV3 `json:"create_request"`
}

type RunCoordinationRequestV3 struct {
	Plan           contract.WorkflowPlanV2 `json:"workflow_plan"`
	CoordinationID string                  `json:"coordination_id"`
}

type IngestRunClaimRequestV3 struct {
	Plan           contract.WorkflowPlanV2               `json:"workflow_plan"`
	CoordinationID string                                `json:"coordination_id"`
	Candidate      runtimeports.EvidenceEventCandidateV2 `json:"candidate"`
}

// PrepareRunV3 persists the Application create intent before invoking Runtime.
// A lost Runtime create reply is always recovered through lifecycle Inspect.
func (c *RunCoordinatorV3) PrepareRunV3(ctx context.Context, request PrepareRunCoordinationRequestV3) (contract.RunCoordinationFactV3, error) {
	if err := request.Plan.Validate(time.Time{}); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	journal, err := c.config.Journals.InspectWorkflowJournalV2(ctx, request.Plan.Target, request.JournalID)
	if err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if err := journal.ValidateFor(request.Plan); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if existing, inspectErr := c.config.Facts.InspectRunCoordinationV3(ctx, request.Plan.Target, request.CoordinationID); inspectErr == nil {
		if err := existing.Validate(); err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
		if !runCoordinationMatchesPrepareV3(existing, request.Plan, journal, request) {
			return contract.RunCoordinationFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "coordination ID already binds another workflow Run intent")
		}
		if existing.State != contract.RunCoordinationCreatePlannedV3 {
			return existing, nil
		}
		allowCreate := request.Plan.Validate(c.config.Clock()) == nil
		envelope, err := c.inspectOrCreatePendingRunV3(ctx, existing, allowCreate)
		if err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
		return c.advanceRunCoordinationV3(ctx, existing, contract.RunCoordinationPendingV3, func(next *contract.RunCoordinationFactV3) { next.Lifecycle = &envelope })
	} else if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return contract.RunCoordinationFactV3{}, inspectErr
	}
	if err := request.Plan.Validate(c.config.Clock()); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	initial, err := c.config.Attempts.InspectGovernedOperationAttemptV3(ctx, request.Plan.Target, request.StartAttemptID)
	if err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if err := validateRunStartAttemptForPlanV3(request.Plan, request.StepID, request.Create.Run.ID, initial, false); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	fact, err := contract.NewRunCoordinationFactV3(request.CoordinationID, request.Plan, journal, request.StepID, initial, request.Create, c.nowUnixNanoV3())
	if err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	current, err := c.config.Facts.CreateRunCoordinationV3(ctx, fact)
	if err != nil {
		if !recoverableApplicationWriteV3(err) {
			return contract.RunCoordinationFactV3{}, err
		}
		current, err = c.config.Facts.InspectRunCoordinationV3(context.WithoutCancel(ctx), request.Plan.Target, request.CoordinationID)
		if err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
	}
	if err := current.Validate(); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if !sameRunCoordinationIntentV3(current, fact) {
		return contract.RunCoordinationFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "coordination ID already binds another workflow Run intent")
	}
	if current.State != contract.RunCoordinationCreatePlannedV3 {
		return current, nil
	}
	envelope, err := c.inspectOrCreatePendingRunV3(ctx, current, true)
	if err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	return c.advanceRunCoordinationV3(ctx, current, contract.RunCoordinationPendingV3, func(next *contract.RunCoordinationFactV3) { next.Lifecycle = &envelope })
}

// ConfirmRunStartedV3 accepts no StartedAt or Outcome. It re-reads the exact
// settled execution-start attempt and lets the Runtime owner derive Running.
func (c *RunCoordinatorV3) ConfirmRunStartedV3(ctx context.Context, request RunCoordinationRequestV3) (contract.RunCoordinationFactV3, error) {
	current, err := c.inspectForPlanV3(ctx, request)
	if err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if current.State == contract.RunCoordinationPendingV3 {
		settled, refs, err := c.inspectSettledStartAttemptV3(ctx, request.Plan, current)
		if err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
		operation := settled.Operation
		current, err = c.advanceRunCoordinationV3(ctx, current, contract.RunCoordinationStartPlannedV3, func(next *contract.RunCoordinationFactV3) {
			next.StartAttempt = &settled
			next.StartOperation = &operation
			next.StartRuntimeAttempt = &refs
		})
		if err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
	}
	if current.State == contract.RunCoordinationRunningV3 || current.State == contract.RunCoordinationClaimPlannedV3 || current.State == contract.RunCoordinationClaimAssociatedV3 || current.State == contract.RunCoordinationStopPlannedV3 || current.State == contract.RunCoordinationStoppingV3 || current.State == contract.RunCoordinationTerminalCleanupV3 || current.State == contract.RunCoordinationTerminationClosedV3 {
		return current, nil
	}
	if current.State != contract.RunCoordinationStartPlannedV3 {
		return contract.RunCoordinationFactV3{}, invalidRunCoordinatorStateV3("Run is not ready for start confirmation")
	}
	settled, refs, err := c.inspectSettledStartAttemptV3(ctx, request.Plan, current)
	if err != nil || !sameApplicationValueV3("settled-start", current.StartAttempt, &settled) || !sameApplicationValueV3("runtime-start", current.StartRuntimeAttempt, &refs) {
		if err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
		return contract.RunCoordinationFactV3{}, core.NewError(core.ErrorConflict, core.ReasonEffectSettlementMissing, "persisted execution-start attempt drifted")
	}
	startEnvelope, err := c.config.Start.InspectRunStartV3(ctx, current.Scope, current.CreateRequest.Run.ID)
	if err != nil && !core.HasCategory(err, core.ErrorNotFound) {
		return contract.RunCoordinationFactV3{}, err
	}
	if err != nil && core.HasCategory(err, core.ErrorNotFound) {
		// InspectRunStart deliberately hides pending Runs, so the expected
		// revision comes from the exact persisted lifecycle create watermark.
		if current.Lifecycle != nil {
			startEnvelope, err = c.config.Start.ConfirmRunStartedV3(ctx, runtimeports.ConfirmRunStartedRequestV3{ExecutionScope: current.Scope, RunID: current.CreateRequest.Run.ID, ExpectedRunRevision: current.Lifecycle.Run.Revision, Operation: *current.StartOperation, Attempt: refs})
		}
		if err != nil && recoverableApplicationWriteV3(err) {
			startEnvelope, err = c.config.Start.InspectRunStartV3(context.WithoutCancel(ctx), current.Scope, current.CreateRequest.Run.ID)
		}
	}
	if err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if err := startEnvelope.Validate(); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if err := validateRunningRunV3(current, startEnvelope); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	envelope, err := c.config.Lifecycle.InspectRunLifecycleV3(context.WithoutCancel(ctx), current.Scope, startEnvelope.Run.ID)
	if err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if envelope.Phase != runtimeports.RunLifecycleRunningV3 {
		return contract.RunCoordinationFactV3{}, invalidRunCoordinatorStateV3("Runtime start did not publish a running lifecycle")
	}
	if err := validateLifecycleIdentityV3(current, envelope); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	return c.advanceRunCoordinationV3(ctx, current, contract.RunCoordinationRunningV3, func(next *contract.RunCoordinationFactV3) {
		next.Lifecycle = &envelope
		confirmation := startEnvelope.Confirmation
		next.StartConfirmation = &confirmation
	})
}

// IngestTerminalClaimV3 persists the exact candidate before invoking the
// public claim gateway. Association remains evidence and never completes Run.
func (c *RunCoordinatorV3) IngestTerminalClaimV3(ctx context.Context, request IngestRunClaimRequestV3) (contract.RunCoordinationFactV3, error) {
	current, err := c.inspectForPlanV3(ctx, RunCoordinationRequestV3{Plan: request.Plan, CoordinationID: request.CoordinationID})
	if err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if current.State == contract.RunCoordinationRunningV3 {
		candidate := request.Candidate
		current, err = c.advanceRunCoordinationV3(ctx, current, contract.RunCoordinationClaimPlannedV3, func(next *contract.RunCoordinationFactV3) { next.ClaimCandidate = &candidate })
		if err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
	}
	if current.State == contract.RunCoordinationClaimAssociatedV3 {
		if current.ClaimCandidate == nil || !sameApplicationClaimCandidateV3(*current.ClaimCandidate, request.Candidate) {
			return contract.RunCoordinationFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRunClaimConflict, "Run already binds another terminal claim")
		}
		return current, nil
	}
	if current.State != contract.RunCoordinationClaimPlannedV3 || current.ClaimCandidate == nil || !sameApplicationClaimCandidateV3(*current.ClaimCandidate, request.Candidate) {
		return contract.RunCoordinationFactV3{}, invalidRunCoordinatorStateV3("Run is not waiting for this terminal claim")
	}
	result, err := c.config.Claims.InspectRunClaimV3(ctx, current.Scope, current.CreateRequest.Run.ID)
	if err != nil && core.HasCategory(err, core.ErrorNotFound) {
		result, err = c.config.Claims.IngestRunClaimV3(ctx, runtimeports.RunClaimIngestRequestV2{ExpectedRunRevision: current.Lifecycle.Run.Revision, Candidate: *current.ClaimCandidate})
		if err != nil && recoverableApplicationWriteV3(err) {
			result, err = c.config.Claims.InspectRunClaimV3(context.WithoutCancel(ctx), current.Scope, current.CreateRequest.Run.ID)
		}
	}
	if err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if err := result.Validate(); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if result.Certification != current.CreateRequest.Certification || result.Plan != current.Lifecycle.Plan || !sameApplicationClaimCandidateV3(*current.ClaimCandidate, result.Evidence.Candidate) {
		return contract.RunCoordinationFactV3{}, core.NewError(core.ErrorConflict, core.ReasonRunClaimConflict, "Runtime claim inspection returned another certified Run, Plan or candidate")
	}
	return c.advanceRunCoordinationV3(ctx, current, contract.RunCoordinationClaimAssociatedV3, func(next *contract.RunCoordinationFactV3) { next.ClaimResult = &result })
}

// StopAndSettleRunV3 accepts no caller Outcome. Unknown participants remain in
// terminal_cleanup and require explicit termination reconciliation.
func (c *RunCoordinatorV3) StopAndSettleRunV3(ctx context.Context, request RunCoordinationRequestV3) (contract.RunCoordinationFactV3, error) {
	current, err := c.inspectForPlanV3(ctx, request)
	if err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if current.State == contract.RunCoordinationRunningV3 || current.State == contract.RunCoordinationClaimAssociatedV3 {
		current, err = c.advanceRunCoordinationV3(ctx, current, contract.RunCoordinationStopPlannedV3, nil)
		if err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
	}
	if current.State == contract.RunCoordinationStopPlannedV3 {
		envelope, err := c.config.Lifecycle.InspectRunLifecycleV3(ctx, current.Scope, current.CreateRequest.Run.ID)
		if err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
		if err := validateLifecycleIdentityV3(current, envelope); err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
		if envelope.Phase == runtimeports.RunLifecycleRunningV3 {
			envelope, err = c.config.Lifecycle.BeginStopRunV3(ctx, runtimeports.BeginStopRunRequestV3{ExecutionScope: current.Scope, RunID: current.CreateRequest.Run.ID, ExpectedRunRevision: envelope.Run.Revision})
			if err != nil && recoverableApplicationWriteV3(err) {
				envelope, err = c.config.Lifecycle.InspectRunLifecycleV3(context.WithoutCancel(ctx), current.Scope, current.CreateRequest.Run.ID)
			}
		}
		if err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
		if err := validateLifecycleIdentityV3(current, envelope); err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
		target, err := coordinationStateForLifecycleV3(envelope.Phase)
		if err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
		current, err = c.advanceRunCoordinationV3(ctx, current, target, func(next *contract.RunCoordinationFactV3) { next.Lifecycle = &envelope })
		if err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
	}
	if current.State == contract.RunCoordinationStoppingV3 {
		envelope, err := c.config.Lifecycle.InspectRunLifecycleV3(ctx, current.Scope, current.CreateRequest.Run.ID)
		if err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
		if err := validateLifecycleIdentityV3(current, envelope); err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
		if envelope.Phase == runtimeports.RunLifecycleStoppingV3 {
			envelope, err = c.config.Lifecycle.StopAndSettleRunV3(ctx, runtimeports.BeginStopRunRequestV3{ExecutionScope: current.Scope, RunID: current.CreateRequest.Run.ID, ExpectedRunRevision: envelope.Run.Revision})
			if err != nil && recoverableApplicationWriteV3(err) {
				envelope, err = c.config.Lifecycle.InspectRunLifecycleV3(context.WithoutCancel(ctx), current.Scope, current.CreateRequest.Run.ID)
			}
		}
		if err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
		if err := validateLifecycleIdentityV3(current, envelope); err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
		target, err := coordinationStateForLifecycleV3(envelope.Phase)
		if err != nil || target == contract.RunCoordinationStoppingV3 {
			if err != nil {
				return contract.RunCoordinationFactV3{}, err
			}
			return contract.RunCoordinationFactV3{}, invalidRunCoordinatorStateV3("Run settlement made no terminal progress")
		}
		current, err = c.advanceRunCoordinationV3(ctx, current, target, func(next *contract.RunCoordinationFactV3) { next.Lifecycle = &envelope })
		if err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
	}
	if current.State != contract.RunCoordinationTerminalCleanupV3 && current.State != contract.RunCoordinationTerminationClosedV3 {
		return contract.RunCoordinationFactV3{}, invalidRunCoordinatorStateV3("Run cannot be settled from its current Application watermark")
	}
	return current, nil
}

func (c *RunCoordinatorV3) ReconcileRunTerminationV3(ctx context.Context, request RunCoordinationRequestV3) (contract.RunCoordinationFactV3, error) {
	current, err := c.inspectForPlanV3(ctx, request)
	if err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if current.State == contract.RunCoordinationTerminationClosedV3 {
		return current, nil
	}
	if current.State != contract.RunCoordinationTerminalCleanupV3 {
		return contract.RunCoordinationFactV3{}, invalidRunCoordinatorStateV3("termination reconciliation requires terminal_cleanup")
	}
	req := runtimeports.RunTerminationRequestV3{ExecutionScope: current.Scope, RunID: current.CreateRequest.Run.ID}
	envelope, err := c.config.Lifecycle.InspectRunTerminationV3(ctx, req)
	if err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if err := validateLifecycleIdentityV3(current, envelope); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if envelope.Phase == runtimeports.RunLifecycleTerminalCleanupV3 {
		envelope, err = c.config.Lifecycle.ReconcileRunTerminationV3(ctx, req)
		if err != nil && recoverableApplicationWriteV3(err) {
			envelope, err = c.config.Lifecycle.InspectRunTerminationV3(context.WithoutCancel(ctx), req)
		}
	}
	if err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if err := validateLifecycleIdentityV3(current, envelope); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	target, err := coordinationStateForLifecycleV3(envelope.Phase)
	if err != nil || (target != contract.RunCoordinationTerminalCleanupV3 && target != contract.RunCoordinationTerminationClosedV3) {
		if err != nil {
			return contract.RunCoordinationFactV3{}, err
		}
		return contract.RunCoordinationFactV3{}, invalidRunCoordinatorStateV3("termination reconciliation returned a nonterminal lifecycle")
	}
	if sameApplicationValueV3("lifecycle", current.Lifecycle, &envelope) {
		return current, nil
	}
	return c.advanceRunCoordinationV3(ctx, current, target, func(next *contract.RunCoordinationFactV3) { next.Lifecycle = &envelope })
}

func (c *RunCoordinatorV3) InspectRunCoordinationV3(ctx context.Context, request RunCoordinationRequestV3) (contract.RunCoordinationFactV3, error) {
	return c.inspectForPlanV3(ctx, request)
}

func (c *RunCoordinatorV3) inspectOrCreatePendingRunV3(ctx context.Context, fact contract.RunCoordinationFactV3, allowCreate bool) (runtimeports.RunLifecycleEnvelopeV3, error) {
	envelope, err := c.config.Lifecycle.InspectRunLifecycleV3(ctx, fact.Scope, fact.CreateRequest.Run.ID)
	if err != nil && core.HasCategory(err, core.ErrorNotFound) {
		if !allowCreate {
			return runtimeports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCapabilityExpired, "expired workflow cannot dispatch a missing pending Run create")
		}
		envelope, err = c.config.Assembler.CreatePendingRunV3(ctx, fact.CreateRequest)
		if err != nil && recoverableApplicationWriteV3(err) {
			envelope, err = c.config.Lifecycle.InspectRunLifecycleV3(context.WithoutCancel(ctx), fact.Scope, fact.CreateRequest.Run.ID)
		}
	}
	if err != nil {
		return runtimeports.RunLifecycleEnvelopeV3{}, err
	}
	if envelope.Phase != runtimeports.RunLifecyclePendingPreparedV3 {
		return runtimeports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "Runtime already contains a non-pending Run for create intent")
	}
	probe := fact
	probe.State = contract.RunCoordinationPendingV3
	probe.Revision++
	probe.UpdatedUnixNano = c.nowUnixNanoV3()
	probe.Lifecycle = &envelope
	if err := probe.Validate(); err != nil {
		return runtimeports.RunLifecycleEnvelopeV3{}, err
	}
	return envelope, nil
}

func (c *RunCoordinatorV3) inspectSettledStartAttemptV3(ctx context.Context, plan contract.WorkflowPlanV2, coordination contract.RunCoordinationFactV3) (contract.GovernedOperationAttemptFactV3, runtimeports.GovernedExecutionAttemptRefsV2, error) {
	fact, err := c.config.Attempts.InspectGovernedOperationAttemptV3(ctx, coordination.Scope, coordination.StartAttemptInitial.ID)
	if err != nil {
		return contract.GovernedOperationAttemptFactV3{}, runtimeports.GovernedExecutionAttemptRefsV2{}, err
	}
	if err := validateRunStartAttemptForPlanV3(plan, coordination.StepID, coordination.CreateRequest.Run.ID, fact, true); err != nil {
		return contract.GovernedOperationAttemptFactV3{}, runtimeports.GovernedExecutionAttemptRefsV2{}, err
	}
	ref, _ := fact.RefV3()
	if !sameOperationAttemptRoutingV3(coordination.StartAttemptInitial, ref) {
		return contract.GovernedOperationAttemptFactV3{}, runtimeports.GovernedExecutionAttemptRefsV2{}, core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "execution-start attempt routing changed after Run create")
	}
	refs := governedAttemptRefsV3(fact)
	if err := refs.ValidatePrepared(); err != nil || refs.Observation == nil || refs.Settlement == nil || refs.Settlement.Disposition != runtimeports.OperationSettlementAppliedV3 {
		if err != nil {
			return contract.GovernedOperationAttemptFactV3{}, runtimeports.GovernedExecutionAttemptRefsV2{}, err
		}
		return contract.GovernedOperationAttemptFactV3{}, runtimeports.GovernedExecutionAttemptRefsV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "execution-start attempt is not independently observed and applied")
	}
	return fact, refs, nil
}

func validateRunStartAttemptForPlanV3(plan contract.WorkflowPlanV2, stepID string, runID core.AgentRunID, attempt contract.GovernedOperationAttemptFactV3, settled bool) error {
	if err := validateOperationPlanBindingV3(plan, attempt); err != nil {
		return err
	}
	if attempt.StepID != stepID || attempt.Operation.Kind != runtimeports.OperationScopeRunV3 || attempt.Operation.RunID != runID || attempt.IntentValue.Kind != runtimeports.OperationEffectKindExecutionStartV3 {
		return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "workflow step is not this Run's execution-start operation")
	}
	if !settled && (attempt.Revision != 1 || attempt.State != contract.OperationIntentRecordedV3) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectStateConflict, "Run create requires the revision-one intent_recorded start attempt")
	}
	if settled && (attempt.State != contract.OperationSettledV3 || attempt.Settlement == nil) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectSettlementMissing, "execution-start operation is not settled")
	}
	return nil
}

func (c *RunCoordinatorV3) inspectForPlanV3(ctx context.Context, request RunCoordinationRequestV3) (contract.RunCoordinationFactV3, error) {
	if err := request.Plan.Validate(time.Time{}); err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	fact, err := c.config.Facts.InspectRunCoordinationV3(ctx, request.Plan.Target, request.CoordinationID)
	if err != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if err := fact.Validate(); err != nil {
		createCertificationErr := fact.CreateRequest.Certification.Validate()
		lifecycleCertificationErr := error(nil)
		if fact.Lifecycle != nil {
			lifecycleCertificationErr = fact.Lifecycle.Certification.Validate()
		}
		return contract.RunCoordinationFactV3{}, fmt.Errorf("validate inspected Run coordination %s/%d create-cert=%v lifecycle-cert=%v: %w", fact.State, fact.Revision, createCertificationErr, lifecycleCertificationErr, err)
	}
	digest, err := request.Plan.DigestV2()
	if err != nil || fact.PlanID != request.Plan.ID || fact.PlanRevision != request.Plan.Revision || fact.PlanDigest != digest || !runtimeports.SameExecutionScopeV2(fact.Scope, request.Plan.Target) {
		return contract.RunCoordinationFactV3{}, core.NewError(core.ErrorConflict, core.ReasonPlanInvalid, "run coordination belongs to another workflow Plan")
	}
	return fact, nil
}

func (c *RunCoordinatorV3) advanceRunCoordinationV3(ctx context.Context, current contract.RunCoordinationFactV3, state contract.RunCoordinationStateV3, mutate func(*contract.RunCoordinationFactV3)) (contract.RunCoordinationFactV3, error) {
	next := current
	next.Revision++
	next.State = state
	next.UpdatedUnixNano = c.nowUnixNanoV3()
	if next.UpdatedUnixNano < current.UpdatedUnixNano {
		return contract.RunCoordinationFactV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "run coordination clock regressed")
	}
	if mutate != nil {
		mutate(&next)
	}
	committed, err := c.config.Facts.CompareAndSwapRunCoordinationV3(ctx, applicationports.RunCoordinationCASRequestV3{Scope: current.Scope, ID: current.ID, ExpectedRevision: current.Revision, Next: next})
	if err == nil {
		if validateErr := committed.Validate(); validateErr != nil {
			return contract.RunCoordinationFactV3{}, fmt.Errorf("validate successful Run coordination CAS reply for %s->%s: %w", current.State, state, validateErr)
		}
		if sameApplicationValueV3("run-coordination-cas", next, committed) || runCoordinationSuccessorV3(next, committed) {
			return committed, nil
		}
		return contract.RunCoordinationFactV3{}, runCoordinationRecoveryConflictV3(next, committed)
	}
	if !recoverableApplicationWriteV3(err) {
		return contract.RunCoordinationFactV3{}, err
	}
	committed, inspectErr := c.config.Facts.InspectRunCoordinationV3(context.WithoutCancel(ctx), current.Scope, current.ID)
	if inspectErr != nil {
		return contract.RunCoordinationFactV3{}, err
	}
	if validateErr := committed.Validate(); validateErr != nil {
		return contract.RunCoordinationFactV3{}, fmt.Errorf("validate recovered Run coordination CAS reply for %s->%s actual=%s/%d: %w", current.State, state, committed.State, committed.Revision, validateErr)
	}
	// A concurrent caller may have committed a different legal sibling of the
	// same current watermark and progressed several more stages before this
	// Inspect. Return only a strict, append-only proven successor so the outer
	// bounded state machine can continue without replaying either branch.
	if runCoordinationSuccessorV3(current, committed) || runCoordinationSuccessorV3(next, committed) {
		return committed, nil
	}
	return contract.RunCoordinationFactV3{}, runCoordinationRecoveryConflictV3(next, committed)
}

func runCoordinationRecoveryConflictV3(expected, actual contract.RunCoordinationFactV3) error {
	lifecycle := true
	if expected.Lifecycle != nil {
		lifecycle = actual.Lifecycle != nil && contract.ValidateRunLifecycleAdvanceV3(*expected.Lifecycle, *actual.Lifecycle) == nil
	}
	return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, fmt.Sprintf("run coordination CAS recovered another stage expected=%s/%d actual=%s/%d intent=%t reachable=%t sidecars=%t lifecycle=%t", expected.State, expected.Revision, actual.State, actual.Revision, sameRunCoordinationIntentV3(actual, expected), runCoordinationStateReachableV3(expected.State, actual.State), runCoordinationSidecarsExtendV3(expected, actual), lifecycle))
}

func validateRunningRunV3(coordination contract.RunCoordinationFactV3, envelope runtimeports.RunStartConfirmationEnvelopeV3) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	run := envelope.Run
	if err := run.Validate(); err != nil {
		return err
	}
	operationDigest, _ := coordination.StartOperation.DigestV3()
	if run.Status != core.RunRunning || run.ID != coordination.CreateRequest.Run.ID || run.SessionRef != coordination.CreateRequest.Run.SessionRef || !runtimeports.SameExecutionScopeV2(run.Scope, coordination.Scope) || run.CompletionClaim != nil || run.Outcome != "" || envelope.Certification != coordination.CreateRequest.Certification || coordination.StartRuntimeAttempt == nil || coordination.StartRuntimeAttempt.Observation == nil || run.StartedAt.UnixNano() != coordination.StartRuntimeAttempt.Observation.ObservedUnixNano || envelope.Confirmation.OperationDigest != operationDigest || !sameApplicationValueV3("start-confirmation-attempt", envelope.Confirmation.Attempt, *coordination.StartRuntimeAttempt) {
		return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "Runtime start inspection does not prove the exact running Run")
	}
	return nil
}

func coordinationStateForLifecycleV3(phase runtimeports.RunLifecyclePhaseV3) (contract.RunCoordinationStateV3, error) {
	switch phase {
	case runtimeports.RunLifecycleStoppingV3:
		return contract.RunCoordinationStoppingV3, nil
	case runtimeports.RunLifecycleTerminalCleanupV3:
		return contract.RunCoordinationTerminalCleanupV3, nil
	case runtimeports.RunLifecycleTerminationClosedV3:
		return contract.RunCoordinationTerminationClosedV3, nil
	default:
		return "", invalidRunCoordinatorStateV3("Runtime lifecycle cannot satisfy stop/termination progression")
	}
}

func validateLifecycleIdentityV3(coordination contract.RunCoordinationFactV3, envelope runtimeports.RunLifecycleEnvelopeV3) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	planRef, err := coordination.CreateRequest.Plan.RefV2()
	if err != nil || envelope.Run.ID != coordination.CreateRequest.Run.ID || !runtimeports.SameExecutionScopeV2(envelope.Run.Scope, coordination.Scope) || envelope.Plan.RunSettlementPlanRefV2 != planRef || envelope.Certification != coordination.CreateRequest.Certification || envelope.EffectIndex.ID != coordination.CreateRequest.EffectIndexID {
		return core.NewError(core.ErrorConflict, core.ReasonRunConflict, "Runtime lifecycle inspection returned another Run/Plan/Effect index")
	}
	return nil
}

func sameRunCoordinationIntentV3(a, b contract.RunCoordinationFactV3) bool {
	a.Revision, b.Revision = 1, 1
	a.State, b.State = contract.RunCoordinationCreatePlannedV3, contract.RunCoordinationCreatePlannedV3
	a.UpdatedUnixNano, b.UpdatedUnixNano = a.CreatedUnixNano, b.CreatedUnixNano
	a.Lifecycle, b.Lifecycle = nil, nil
	a.StartAttempt, b.StartAttempt = nil, nil
	a.StartOperation, b.StartOperation = nil, nil
	a.StartRuntimeAttempt, b.StartRuntimeAttempt = nil, nil
	a.StartConfirmation, b.StartConfirmation = nil, nil
	a.ClaimCandidate, b.ClaimCandidate = nil, nil
	a.ClaimResult, b.ClaimResult = nil, nil
	return sameApplicationValueV3("run-coordination-intent", a, b)
}

func runCoordinationMatchesPrepareV3(fact contract.RunCoordinationFactV3, plan contract.WorkflowPlanV2, journal contract.WorkflowJournalV2, request PrepareRunCoordinationRequestV3) bool {
	planDigest, pe := plan.DigestV2()
	journalDigest, je := journal.DigestV2(plan)
	return pe == nil && je == nil && fact.PlanID == plan.ID && fact.PlanRevision == plan.Revision && fact.PlanDigest == planDigest && fact.JournalID == journal.ID && fact.JournalRevision == journal.Revision && fact.JournalDigest == journalDigest && fact.StepID == request.StepID && fact.StartAttemptInitial.ID == request.StartAttemptID && sameApplicationValueV3("run-create-request", fact.CreateRequest, request.Create)
}

func runCoordinationSuccessorV3(expected, actual contract.RunCoordinationFactV3) bool {
	if actual.Revision <= expected.Revision || !sameRunCoordinationIntentV3(actual, expected) || !runCoordinationStateReachableV3(expected.State, actual.State) {
		return false
	}
	if !runCoordinationSidecarsExtendV3(expected, actual) {
		return false
	}
	if expected.Lifecycle != nil {
		if actual.Lifecycle == nil || contract.ValidateRunLifecycleAdvanceV3(*expected.Lifecycle, *actual.Lifecycle) != nil {
			return false
		}
	}
	return true
}

func runCoordinationSidecarsExtendV3(expected, actual contract.RunCoordinationFactV3) bool {
	if expected.StartAttempt != nil {
		if actual.StartAttempt == nil {
			return false
		}
		a, ae := expected.StartAttempt.DigestV3()
		b, be := actual.StartAttempt.DigestV3()
		if ae != nil || be != nil || a != b {
			return false
		}
	}
	if expected.StartOperation != nil {
		if actual.StartOperation == nil {
			return false
		}
		a, ae := expected.StartOperation.DigestV3()
		b, be := actual.StartOperation.DigestV3()
		if ae != nil || be != nil || a != b {
			return false
		}
	}
	if expected.StartRuntimeAttempt != nil && (actual.StartRuntimeAttempt == nil || !sameApplicationValueV3("start-runtime", expected.StartRuntimeAttempt, actual.StartRuntimeAttempt)) {
		return false
	}
	if expected.StartConfirmation != nil && (actual.StartConfirmation == nil || expected.StartConfirmation.Digest != actual.StartConfirmation.Digest || expected.StartConfirmation.ID != actual.StartConfirmation.ID) {
		return false
	}
	if expected.ClaimCandidate != nil && (actual.ClaimCandidate == nil || !sameApplicationClaimCandidateV3(*expected.ClaimCandidate, *actual.ClaimCandidate)) {
		return false
	}
	if expected.ClaimResult != nil {
		if actual.ClaimResult == nil || expected.ClaimResult.Association.ID != actual.ClaimResult.Association.ID || expected.ClaimResult.Association.CandidateDigest != actual.ClaimResult.Association.CandidateDigest || expected.ClaimResult.Evidence.Ref != actual.ClaimResult.Evidence.Ref || !sameApplicationClaimCandidateV3(expected.ClaimResult.Evidence.Candidate, actual.ClaimResult.Evidence.Candidate) {
			return false
		}
	}
	return true
}

func nilApplicationRunValueV3(value any) bool {
	switch v := value.(type) {
	case *contract.GovernedOperationAttemptFactV3:
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

func runCoordinationStateReachableV3(from, to contract.RunCoordinationStateV3) bool {
	if from == to {
		return true
	}
	seen := map[contract.RunCoordinationStateV3]bool{from: true}
	queue := []contract.RunCoordinationStateV3{from}
	all := []contract.RunCoordinationStateV3{contract.RunCoordinationPendingV3, contract.RunCoordinationStartPlannedV3, contract.RunCoordinationRunningV3, contract.RunCoordinationClaimPlannedV3, contract.RunCoordinationClaimAssociatedV3, contract.RunCoordinationStopPlannedV3, contract.RunCoordinationStoppingV3, contract.RunCoordinationTerminalCleanupV3, contract.RunCoordinationTerminationClosedV3}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, candidate := range all {
			if !seen[candidate] && runCoordinationDirectSuccessorV3(current, candidate) {
				if candidate == to {
					return true
				}
				seen[candidate] = true
				queue = append(queue, candidate)
			}
		}
	}
	return false
}

func runCoordinationDirectSuccessorV3(from, to contract.RunCoordinationStateV3) bool {
	switch from {
	case contract.RunCoordinationCreatePlannedV3:
		return to == contract.RunCoordinationPendingV3
	case contract.RunCoordinationPendingV3:
		return to == contract.RunCoordinationStartPlannedV3
	case contract.RunCoordinationStartPlannedV3:
		return to == contract.RunCoordinationRunningV3
	case contract.RunCoordinationRunningV3:
		return to == contract.RunCoordinationClaimPlannedV3 || to == contract.RunCoordinationStopPlannedV3
	case contract.RunCoordinationClaimPlannedV3:
		return to == contract.RunCoordinationClaimAssociatedV3
	case contract.RunCoordinationClaimAssociatedV3:
		return to == contract.RunCoordinationStopPlannedV3
	case contract.RunCoordinationStopPlannedV3:
		return to == contract.RunCoordinationStoppingV3 || to == contract.RunCoordinationTerminalCleanupV3 || to == contract.RunCoordinationTerminationClosedV3
	case contract.RunCoordinationStoppingV3:
		return to == contract.RunCoordinationTerminalCleanupV3 || to == contract.RunCoordinationTerminationClosedV3
	case contract.RunCoordinationTerminalCleanupV3:
		return to == contract.RunCoordinationTerminalCleanupV3 || to == contract.RunCoordinationTerminationClosedV3
	default:
		return false
	}
}

func sameApplicationValueV3(name string, a, b any) bool {
	ad, ae := core.CanonicalJSONDigest("praxis.application.run-coordinator", contract.RunCoordinationContractVersionV3, name, a)
	bd, be := core.CanonicalJSONDigest("praxis.application.run-coordinator", contract.RunCoordinationContractVersionV3, name, b)
	return ae == nil && be == nil && ad == bd
}
func sameApplicationClaimCandidateV3(a, b runtimeports.EvidenceEventCandidateV2) bool {
	ad, ae := a.DigestV2()
	bd, be := b.DigestV2()
	return ae == nil && be == nil && ad == bd
}
func recoverableApplicationWriteV3(err error) bool {
	return core.HasCategory(err, core.ErrorConflict) || core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)
}
func invalidRunCoordinatorStateV3(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, message)
}
func (c *RunCoordinatorV3) nowUnixNanoV3() int64 { return c.config.Clock().UnixNano() }
