package foundation

import (
	"context"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// StartRunRequestV2 carries only create-time facts. Dispatch is deliberately
// absent: P0.6 must cross the governed Permit/Begin/Enforcement bridge.
type StartRunRequestV2 struct {
	Bundle      control.RunBundleCreateRequestV2 `json:"bundle"`
	EffectIndex control.RunEffectIndexFactV2     `json:"effect_index"`
}

type StartRunResultV2 struct {
	Bundle      control.RunBundleV2          `json:"bundle"`
	EffectIndex control.RunEffectIndexFactV2 `json:"effect_index"`
}

// ConfirmRunStartedV3 advances the local Instance only after the persistent
// Runtime Run owner has verified an exact applied execution-start settlement.
// A crash after the Run CAS but before the Instance transition is recovered by
// replaying this method; no provider action is re-dispatched.
func (c *Coordinator) ConfirmRunStartedV3(ctx context.Context, instance *Instance, request ports.ConfirmRunStartedRequestV3) (core.AgentRunRecord, error) {
	if c.RunStart == nil {
		return core.AgentRunRecord{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "run start governance gateway is required")
	}
	if instance == nil {
		return core.AgentRunRecord{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "instance is required")
	}
	if err := request.Validate(); err != nil {
		return core.AgentRunRecord{}, err
	}
	instance.mu.Lock()
	defer instance.mu.Unlock()
	snapshot := instance.aggregate.Snapshot()
	if snapshot.State.Phase != core.PhaseReady && snapshot.State.Phase != core.PhaseRunning || !ports.SameExecutionScopeV2(snapshot.Scope, request.ExecutionScope) {
		return core.AgentRunRecord{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "run start confirmation requires the matching ready or recovering running instance")
	}
	confirmation, err := c.RunStart.ConfirmRunStartedV3(ctx, request)
	if err != nil {
		return core.AgentRunRecord{}, err
	}
	run := confirmation.Run
	if snapshot.State.Phase == core.PhaseReady {
		if _, err := transition(instance.aggregate, core.InstanceState{Phase: core.PhaseRunning, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true}, core.TransitionContext{}); err != nil {
			return core.AgentRunRecord{}, err
		}
	} else if instance.activeRun != nil && !sameFoundationRunV2(*instance.activeRun, run) {
		return core.AgentRunRecord{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "running instance is bound to another persisted Run")
	}
	copy := run
	instance.activeRun = &copy
	return run, nil
}

// StartRunV2 prepares persistent Run+Plan and the empty Effect index before any
// execution dispatch. It never calls Execution.Control and does not claim that
// RunBundle and Effect index are cross-owner atomic. P0.6's persistent
// Operation Step Journal is the required recoverable handoff boundary.
func (c *Coordinator) StartRunV2(ctx context.Context, instance *Instance, request StartRunRequestV2) (StartRunResultV2, error) {
	if err := c.validateRunV2(); err != nil {
		return StartRunResultV2{}, err
	}
	if instance == nil {
		return StartRunResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "instance is required")
	}
	if err := request.Bundle.Validate(); err != nil {
		return StartRunResultV2{}, err
	}
	if err := request.EffectIndex.Validate(); err != nil {
		return StartRunResultV2{}, err
	}
	instance.mu.Lock()
	defer instance.mu.Unlock()
	snapshot := instance.aggregate.Snapshot()
	if snapshot.State.Phase != core.PhaseReady || !ports.SameExecutionScopeV2(snapshot.Scope, request.Bundle.Run.Scope) || request.EffectIndex.RunID != request.Bundle.Run.ID || request.EffectIndex.RunIdentityDigest != request.Bundle.Plan.RunIdentityDigest || request.EffectIndex.ExecutionScopeDigest != request.Bundle.Plan.ExecutionScopeDigest || request.EffectIndex.State != control.RunEffectIndexOpen || request.EffectIndex.SegmentCount != 0 || request.EffectIndex.EffectCount != 0 {
		return StartRunResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementPlanConflict, "V2 Start requires a ready instance and matching empty Run facts")
	}
	bundle, err := c.RunSettlements.CreateRunBundleV2(ctx, request.Bundle)
	if err != nil {
		if core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate) {
			run, runErr := c.RunSettlements.InspectRun(ctx, request.Bundle.Run.Scope, request.Bundle.Run.ID)
			plan, planErr := c.RunSettlements.InspectRunSettlementPlanV2(ctx, request.Bundle.Run.Scope, request.Bundle.Run.ID)
			if runErr == nil && planErr == nil && sameFoundationRunV2(run, request.Bundle.Run) && sameFoundationPlanV2(plan, request.Bundle.Plan) {
				bundle = control.RunBundleV2{Run: run, Plan: plan}
				err = nil
			} else if runErr == nil || planErr == nil {
				err = core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "lost Run bundle reply inspected conflicting persisted facts")
			}
		}
		if err != nil {
			return StartRunResultV2{}, err
		}
	}
	index, err := c.RunEffects.CreateRunEffectIndexV2(ctx, request.EffectIndex)
	if err != nil {
		if core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate) {
			if inspected, inspectErr := c.RunEffects.InspectRunEffectIndexV2(ctx, request.EffectIndex.PartitionV2()); inspectErr == nil && sameFoundationEffectIndexV2(inspected, request.EffectIndex) {
				index, err = inspected, nil
			} else if inspectErr == nil {
				err = core.NewError(core.ErrorConflict, core.ReasonRunEffectIndexConflict, "lost Effect index reply inspected conflicting persisted facts")
			}
		}
		if err != nil {
			return StartRunResultV2{}, err
		}
	}
	return StartRunResultV2{Bundle: bundle, EffectIndex: index}, nil
}

func sameFoundationRunV2(left, right core.AgentRunRecord) bool {
	leftIdentity, leftErr := ports.RunIdentityDigestV2(left)
	rightIdentity, rightErr := ports.RunIdentityDigestV2(right)
	return leftErr == nil && rightErr == nil && leftIdentity == rightIdentity && left.Status == right.Status && left.Revision == right.Revision && left.StartedAt.Equal(right.StartedAt) && left.EndedAt.Equal(right.EndedAt) && left.Outcome == right.Outcome
}

func sameFoundationPlanV2(left, right ports.RunSettlementPlanFactV2) bool {
	leftDigest, leftErr := left.DigestV2()
	rightDigest, rightErr := right.DigestV2()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func sameFoundationEffectIndexV2(left, right control.RunEffectIndexFactV2) bool {
	leftDigest, leftErr := left.DigestV2()
	rightDigest, rightErr := right.DigestV2()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

// StopAndSettleRunV3 is the public, restart-safe Foundation composition. It
// never performs Close/Fence/Release directly: each cleanup action is a
// separate governed termination Operation V3 owned by the Application. This
// method only advances/inspects Runtime lifecycle facts and therefore cannot
// blindly replay an external side effect after an unknown result.
func (c *Coordinator) StopAndSettleRunV3(ctx context.Context, instance *Instance, request ports.BeginStopRunRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	if c.RunLifecycle == nil {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "public Run lifecycle governance is required")
	}
	if instance == nil {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "instance is required")
	}
	if err := request.Validate(); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	instance.mu.Lock()
	defer instance.mu.Unlock()
	snapshot := instance.aggregate.Snapshot()
	if !ports.SameExecutionScopeV2(snapshot.Scope, request.ExecutionScope) {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "Run lifecycle request belongs to another instance scope")
	}
	switch snapshot.State.Phase {
	case core.PhaseRunning:
	case core.PhaseStopping, core.PhaseTerminal:
		// A prior process may have committed the terminal Run or completed the
		// report before losing its reply. Recovery inspects public watermarks.
	default:
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Run settlement requires running, stopping or recovering terminal instance")
	}
	envelope, err := c.RunLifecycle.StopAndSettleRunV3(ctx, request)
	if err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	if err := envelope.Validate(); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	if snapshot.State.Phase == core.PhaseRunning {
		if _, err := transition(instance.aggregate, core.InstanceState{Phase: core.PhaseStopping, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true}, core.TransitionContext{}); err != nil {
			return ports.RunLifecycleEnvelopeV3{}, err
		}
	}
	copy := envelope.Run
	instance.activeRun = &copy
	if envelope.Phase == ports.RunLifecycleTerminationClosedV3 {
		if err := completeFoundationTerminationV3(instance); err != nil {
			return ports.RunLifecycleEnvelopeV3{}, err
		}
	}
	return envelope, nil
}

// ReconcileRunTerminationV3 only re-inspects participant/cleanup facts. An
// unresolved cleanup keeps the instance stopping and remains deliverable as
// Progress; it never causes Foundation to re-execute Close/Fence/Release.
func (c *Coordinator) ReconcileRunTerminationV3(ctx context.Context, instance *Instance, request ports.RunTerminationRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	if c.RunLifecycle == nil {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "public Run lifecycle governance is required")
	}
	if instance == nil {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "instance is required")
	}
	if err := request.Validate(); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	instance.mu.Lock()
	defer instance.mu.Unlock()
	snapshot := instance.aggregate.Snapshot()
	if !ports.SameExecutionScopeV2(snapshot.Scope, request.ExecutionScope) || snapshot.State.Phase != core.PhaseStopping && snapshot.State.Phase != core.PhaseTerminal {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "termination reconciliation requires the matching stopping or terminal instance")
	}
	envelope, err := c.RunLifecycle.ReconcileRunTerminationV3(ctx, request)
	if err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	if err := envelope.Validate(); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	copy := envelope.Run
	instance.activeRun = &copy
	if envelope.Phase == ports.RunLifecycleTerminationClosedV3 && snapshot.State.Phase != core.PhaseTerminal {
		if err := completeFoundationTerminationV3(instance); err != nil {
			return ports.RunLifecycleEnvelopeV3{}, err
		}
	}
	return envelope, nil
}

func (c *Coordinator) InspectRunTerminationV3(ctx context.Context, request ports.RunTerminationRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	if c.RunLifecycle == nil {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "public Run lifecycle governance is required")
	}
	if err := request.Validate(); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	envelope, err := c.RunLifecycle.InspectRunTerminationV3(ctx, request)
	if err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	return envelope, envelope.Validate()
}

func completeFoundationTerminationV3(instance *Instance) error {
	if instance.aggregate.Snapshot().State.Phase == core.PhaseTerminal {
		return nil
	}
	_, err := transition(instance.aggregate, core.InstanceState{Phase: core.PhaseTerminal, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupComplete, HasCleanupObligations: true, CleanupEvidenceComplete: true}, core.TransitionContext{})
	if err == nil {
		instance.activeRun = nil
	}
	return err
}

type StopRunRequestV2 struct {
	RunID               core.AgentRunID   `json:"run_id"`
	ExpectedRunRevision core.Revision     `json:"expected_run_revision"`
	Reason              string            `json:"reason"`
	CloseIntent         core.EffectIntent `json:"close_intent"`
	ReleaseIntent       core.EffectIntent `json:"release_intent"`
}

type StopRunResultV2 struct {
	Settlement  kernel.StopAndSettleRunResultV2 `json:"settlement"`
	Termination control.RunTerminationReportV2  `json:"termination"`
}

// StopRunV2 is restricted legacy compatibility. It accepts no caller outcome,
// but its direct Close/Fence/Release calls cannot recover an unknown external
// result. Production/Application composition must use StopAndSettleRunV3 plus
// separate governed termination Operation V3 attempts.
func (c *Coordinator) StopRunV2(ctx context.Context, instance *Instance, request StopRunRequestV2) (StopRunResultV2, error) {
	if err := c.validateRunV2(); err != nil {
		return StopRunResultV2{}, err
	}
	if instance == nil || strings.TrimSpace(request.Reason) == "" {
		return StopRunResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "instance and stop reason are required")
	}
	instance.mu.Lock()
	defer instance.mu.Unlock()
	snapshot := instance.aggregate.Snapshot()
	if snapshot.State.Phase != core.PhaseRunning {
		return StopRunResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "V2 Stop requires a running instance")
	}
	settled, err := c.RunSettlement.StopAndSettleRunV2(ctx, kernel.StopAndSettleRunRequestV2{Scope: snapshot.Scope, RunID: request.RunID, ExpectedRunRevision: request.ExpectedRunRevision})
	if err != nil {
		return StopRunResultV2{}, err
	}
	if _, err := transition(instance.aggregate, core.InstanceState{Phase: core.PhaseStopping, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true}, core.TransitionContext{}); err != nil {
		return StopRunResultV2{}, err
	}
	if err := c.closeInstanceAfterSettlementV2(ctx, instance, request); err != nil {
		return StopRunResultV2{}, err
	}
	if _, err := c.RunSettlement.ReconcileTerminationProgressV2(ctx, snapshot.Scope, request.RunID); err != nil {
		return StopRunResultV2{}, err
	}
	report, err := c.RunSettlement.BuildTerminationReportV2(ctx, snapshot.Scope, request.RunID)
	if err != nil {
		return StopRunResultV2{}, err
	}
	instance.activeRun = nil
	return StopRunResultV2{Settlement: settled, Termination: report}, nil
}

func (c *Coordinator) closeInstanceAfterSettlementV2(ctx context.Context, instance *Instance, request StopRunRequestV2) error {
	snapshot := instance.aggregate.Snapshot()
	closeIntent, err := c.persistIntent(ctx, snapshot.Scope, request.CloseIntent, "execution_close")
	if err != nil {
		return err
	}
	closeFence := makeFence(snapshot.Scope, core.FenceBoundaryInstance, instance.capabilityDigest, closeIntent, c.now().Add(instance.fenceTTL))
	closeObservation, err := c.Execution.Close(ctx, ports.ExecutionCloseRequest{Scope: snapshot.Scope, Endpoint: instance.endpoint, Reason: request.Reason, Intent: closeIntent, Fence: closeFence})
	if err != nil || closeObservation.ObservationKind != "closed" {
		return core.NewError(core.ErrorIndeterminate, core.ReasonCleanupEvidenceIncomplete, "execution close requires independent Effect reconciliation")
	}
	releaseIntent, err := c.persistIntent(ctx, snapshot.Scope, request.ReleaseIntent, "sandbox_release")
	if err != nil {
		return err
	}
	if fenced, err := c.Environment.Fence(ctx, ports.SandboxFenceRequest{Lease: *snapshot.Scope.SandboxLease, Reason: request.Reason}); err != nil || fenced.State != "fenced" {
		return core.NewError(core.ErrorIndeterminate, core.ReasonCleanupEvidenceIncomplete, "sandbox fence requires independent Effect reconciliation")
	}
	releaseFence := makeFence(snapshot.Scope, core.FenceBoundaryInstance, instance.capabilityDigest, releaseIntent, c.now().Add(instance.fenceTTL))
	if released, err := c.Environment.Release(ctx, ports.SandboxReleaseRequest{Lease: *snapshot.Scope.SandboxLease, Intent: releaseIntent, Fence: releaseFence}); err != nil || released.State != "released" {
		return core.NewError(core.ErrorIndeterminate, core.ReasonCleanupEvidenceIncomplete, "sandbox release requires independent Effect reconciliation")
	}
	lease, err := c.IdentityLeases.RevokeIdentityLease(ctx, control.EndIdentityLeaseRequest{LeaseID: instance.identityLease.ID, ExpectedRevision: instance.identityLease.Revision, Reason: request.Reason})
	if err != nil {
		return err
	}
	lease, err = c.IdentityLeases.ReleaseIdentityLease(ctx, control.EndIdentityLeaseRequest{LeaseID: lease.ID, ExpectedRevision: lease.Revision, Reason: request.Reason})
	if err != nil {
		return err
	}
	instance.identityLease = lease
	_, err = transition(instance.aggregate, core.InstanceState{Phase: core.PhaseTerminal, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupComplete, HasCleanupObligations: true, CleanupEvidenceComplete: true}, core.TransitionContext{})
	return err
}

func (c *Coordinator) validateRunV2() error {
	if c.RunSettlements == nil || c.RunEffects == nil || c.RunSettlement == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "persistent Run settlement, Run effect index and settlement Gateway are required")
	}
	return nil
}
