// Package foundation wires the Runtime public contracts into the smallest
// executable, component-neutral lifecycle. It is a reference coordinator for
// contract and integration tests, not a production control plane.
package foundation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/admission"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type Coordinator struct {
	Registry               *ports.ComponentRegistry
	Execution              ports.ExecutionPort
	Environment            ports.EnvironmentPort
	Evidence               ports.EvidencePort
	IdentityLeases         control.IdentityLeaseFactPort
	ActivationFacts        admission.ActivationFactPort
	RunSettlements         control.RunSettlementFactPortV2
	RunEffects             control.RunEffectFactPortV2
	RunSettlement          *kernel.RunSettlementGatewayV2
	RunStart               ports.RunStartGovernancePortV3
	RunLifecycle           ports.RunLifecycleGovernancePortV3
	CheckpointParticipants []CheckpointBinding
	Clock                  func() time.Time
}

type CheckpointBinding struct {
	Port     ports.CheckpointParticipantPort
	Required bool
}

type ActivationRequest struct {
	Plan                   ports.ResolvedAgentPlan
	ProposedScope          core.ExecutionScope
	ActivationAttemptID    string
	RequirementDigest      core.Digest
	CapabilityGrantDigest  core.Digest
	ProbeBudget            ports.ProbeBudget
	IdentityLeaseExpiresAt time.Time
	FenceTTL               time.Duration
	AllocateIntent         core.EffectIntent
	ActivateIntent         core.EffectIntent
	OpenIntent             core.EffectIntent
}

type Instance struct {
	mu               sync.Mutex
	aggregate        *kernel.Aggregate
	legacyRuns       *kernel.RunRegistry
	plan             ports.ResolvedAgentPlan
	bindings         ports.BindingSet
	identityLease    control.IdentityExecutionLease
	activation       admission.ActivationAttempt
	endpoint         ports.ExecutionEndpointRef
	capabilityDigest core.Digest
	fenceTTL         time.Duration
	activeRun        *core.AgentRunRecord
}

type InstanceSnapshot struct {
	Kernel        kernel.Snapshot                `json:"kernel"`
	PlanDigest    core.Digest                    `json:"plan_digest"`
	Bindings      ports.BindingSet               `json:"bindings"`
	IdentityLease control.IdentityExecutionLease `json:"identity_lease"`
	Activation    admission.ActivationAttempt    `json:"activation"`
	Endpoint      ports.ExecutionEndpointRef     `json:"endpoint"`
	ActiveRun     *core.AgentRunRecord           `json:"active_run,omitempty"`
}

func (i *Instance) Snapshot() InstanceSnapshot {
	i.mu.Lock()
	defer i.mu.Unlock()
	var active *core.AgentRunRecord
	if i.activeRun != nil {
		copy := *i.activeRun
		active = &copy
	}
	return InstanceSnapshot{Kernel: i.aggregate.Snapshot(), PlanDigest: i.plan.Digest, Bindings: i.bindings, IdentityLease: i.identityLease, Activation: i.activation, Endpoint: i.endpoint, ActiveRun: active}
}

func (c *Coordinator) Activate(ctx context.Context, request ActivationRequest) (*Instance, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	now := c.now()
	if err := validateActivationRequest(request, now); err != nil {
		return nil, err
	}
	bindings, err := c.Registry.Resolve(ctx, request.Plan, now)
	if err != nil {
		return nil, err
	}

	aggregate, err := kernel.NewAggregate(request.ProposedScope, core.InstanceState{Phase: core.PhasePending, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupNotRequired})
	if err != nil {
		return nil, err
	}
	attempt, err := c.ActivationFacts.CreateActivationAttempt(ctx, admission.ActivationAttempt{
		ID: request.ActivationAttemptID, Scope: request.ProposedScope,
		ExpectedIdentityEpoch: request.ProposedScope.Identity.Epoch - 1,
		RequirementDigest:     request.RequirementDigest, Stage: admission.StageProposed,
		Recovery:           admission.RecoveryNormal,
		Budget:             admission.ActivationOperation{State: admission.OperationNotStarted},
		SandboxReservation: admission.ActivationOperation{State: admission.OperationNotStarted},
		SandboxActivation:  admission.ActivationOperation{State: admission.OperationNotStarted},
		Revision:           1, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return nil, err
	}
	if _, err = transition(aggregate, core.InstanceState{Phase: core.PhaseAdmitted, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupNotRequired}, core.TransitionContext{}); err != nil {
		return nil, err
	}
	if _, err = transition(aggregate, core.InstanceState{Phase: core.PhasePreflighting, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupNotRequired}, core.TransitionContext{}); err != nil {
		return nil, err
	}
	attempt, err = c.advanceActivation(ctx, attempt, admission.StagePreflighting, nil)
	if err != nil {
		return nil, err
	}
	preflight, err := c.Execution.Preflight(ctx, ports.ExecutionPreflightRequest{ProposedScope: request.ProposedScope, RequirementDigest: request.RequirementDigest, ProbeBudget: request.ProbeBudget})
	if err != nil {
		return nil, err
	}
	if !preflight.Accepted || preflight.Descriptor.ID == "" || preflight.RequirementDigest != request.RequirementDigest || !now.Before(preflight.EvidenceExpiry) {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonReadyEvidenceIncomplete, "execution preflight was rejected or returned stale evidence")
	}
	attempt, err = c.advanceActivation(ctx, attempt, admission.StagePreflightPassed, nil)
	if err != nil {
		return nil, err
	}
	activationSnapshot, err := buildActivationSnapshot(request, preflight)
	if err != nil {
		return nil, err
	}
	attempt, err = c.advanceActivation(ctx, attempt, admission.StageSnapshotFrozen, func(next *admission.ActivationAttempt) {
		next.Snapshot = &activationSnapshot
	})
	if err != nil {
		return nil, err
	}
	if _, err = transition(aggregate, core.InstanceState{Phase: core.PhaseActivating, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupNotRequired}, core.TransitionContext{}); err != nil {
		return nil, err
	}

	identityLease, err := c.IdentityLeases.ReserveIdentityLease(ctx, control.ReserveIdentityLeaseRequest{
		TenantID: request.ProposedScope.Identity.TenantID, IdentityID: request.ProposedScope.Identity.ID,
		ExpectedIdentityEpoch: request.ProposedScope.Identity.Epoch - 1, Lineage: request.ProposedScope.Lineage,
		ActivationAttemptID: request.ActivationAttemptID, AuthorityEpoch: request.ProposedScope.AuthorityEpoch,
		ExpiresAt: request.IdentityLeaseExpiresAt,
	})
	if err != nil {
		return nil, err
	}
	if identityLease.Identity != request.ProposedScope.Identity {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonStaleIdentityEpoch, "reserved identity lease does not match the proposed scope")
	}
	attempt, err = c.advanceActivation(ctx, attempt, admission.StageIdentityLeaseReserved, func(next *admission.ActivationAttempt) {
		next.IdentityLeaseID = identityLease.ID
		next.IdentityLeaseState = identityLease.State
		next.IdentityLeaseRevision = identityLease.Revision
	})
	if err != nil {
		return nil, err
	}
	attempt, err = c.advanceActivation(ctx, attempt, admission.StageBudgetResolved, func(next *admission.ActivationAttempt) {
		next.Budget = admission.ActivationOperation{State: admission.OperationNotRequired}
	})
	if err != nil {
		return nil, err
	}
	if _, err = transition(aggregate, core.InstanceState{Phase: core.PhaseActivating, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true}, core.TransitionContext{}); err != nil {
		return nil, err
	}

	allocateIntent, err := c.persistIntent(ctx, request.ProposedScope, request.AllocateIntent, "sandbox_allocate")
	if err != nil {
		return nil, err
	}
	attempt, err = c.advanceActivation(ctx, attempt, admission.StageBudgetResolved, func(next *admission.ActivationAttempt) {
		next.SandboxReservation = admission.ActivationOperation{State: admission.OperationIntentRecorded, IntentID: allocateIntent.ID}
	})
	if err != nil {
		return nil, err
	}
	allocateFence := makeFence(request.ProposedScope, core.FenceBoundaryActivation, request.CapabilityGrantDigest, allocateIntent, now.Add(request.FenceTTL))
	allocation, err := c.Environment.Allocate(ctx, ports.SandboxAllocateRequest{
		ProposedInstance: request.ProposedScope.Instance, RequirementDigest: request.RequirementDigest,
		FenceEpoch: request.ProposedScope.Instance.Epoch, Intent: allocateIntent, Fence: allocateFence,
	})
	if err != nil {
		_ = c.quarantineActivation(ctx, attempt, func(next *admission.ActivationAttempt) {
			next.SandboxReservation = admission.ActivationOperation{State: admission.OperationUnknownOutcome, IntentID: allocateIntent.ID}
		}, "sandbox allocation outcome is unknown")
		return nil, err
	}
	if allocation.State != "reserved_quarantined" {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "sandbox must remain reserved and quarantined before activation commit")
	}
	allocationEvidence, err := core.DigestJSON(allocation)
	if err != nil {
		return nil, err
	}
	attempt, err = c.advanceActivation(ctx, attempt, admission.StageSandboxReserved, func(next *admission.ActivationAttempt) {
		next.SandboxReservation = admission.ActivationOperation{
			State: admission.OperationConfirmedApplied, IntentID: allocateIntent.ID,
			Reference: string(allocation.Lease.ID), EvidenceDigest: allocationEvidence,
		}
	})
	if err != nil {
		return nil, err
	}

	commitResult, err := c.ActivationFacts.CommitActivation(ctx, admission.ActivationCommitRequest{
		AttemptID: attempt.ID, ExpectedAttemptRevision: attempt.Revision,
		IdentityLeaseID: identityLease.ID, ExpectedIdentityLeaseRevision: identityLease.Revision,
		SandboxLease: allocation.Lease, AuthorityEpoch: request.ProposedScope.AuthorityEpoch,
	})
	if err != nil {
		return nil, err
	}
	attempt = commitResult.Attempt
	identityLease = commitResult.IdentityLease
	commitSnapshot, err := aggregate.CommitActivation(kernel.ActivationCommitRequest{
		Preconditions: preconditions(aggregate.Snapshot()), SandboxLease: allocation.Lease,
		NextState: core.InstanceState{Phase: core.PhaseProvisioning, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true},
	})
	if err != nil {
		return nil, err
	}
	activateIntent, err := c.persistIntent(ctx, commitSnapshot.Scope, request.ActivateIntent, "sandbox_activate")
	if err != nil {
		return nil, err
	}
	attempt, err = c.advanceActivation(ctx, attempt, admission.StageCommitted, func(next *admission.ActivationAttempt) {
		next.SandboxActivation = admission.ActivationOperation{State: admission.OperationIntentRecorded, IntentID: activateIntent.ID}
	})
	if err != nil {
		return nil, err
	}
	activateFence := makeFence(commitSnapshot.Scope, core.FenceBoundaryInstance, request.CapabilityGrantDigest, activateIntent, c.now().Add(request.FenceTTL))
	activationObservation, err := c.Environment.Activate(ctx, ports.SandboxActivateRequest{Scope: commitSnapshot.Scope, Intent: activateIntent, Fence: activateFence})
	if err != nil {
		_ = c.quarantineActivation(ctx, attempt, func(next *admission.ActivationAttempt) {
			next.SandboxActivation = admission.ActivationOperation{State: admission.OperationUnknownOutcome, IntentID: activateIntent.ID}
		}, "sandbox activation outcome is unknown")
		return nil, err
	}
	activationEvidence, err := core.DigestJSON(activationObservation)
	if err != nil {
		return nil, err
	}
	attempt, err = c.advanceActivation(ctx, attempt, admission.StageSandboxActive, func(next *admission.ActivationAttempt) {
		next.SandboxActivation = admission.ActivationOperation{
			State: admission.OperationConfirmedApplied, IntentID: activateIntent.ID,
			Reference: string(activationObservation.Lease.ID), EvidenceDigest: activationEvidence,
		}
	})
	if err != nil {
		return nil, err
	}
	if _, err = transition(aggregate, core.InstanceState{Phase: core.PhaseBinding, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true}, core.TransitionContext{}); err != nil {
		return nil, err
	}

	openIntent, err := c.persistIntent(ctx, commitSnapshot.Scope, request.OpenIntent, "execution_open")
	if err != nil {
		return nil, err
	}
	openFence := makeFence(commitSnapshot.Scope, core.FenceBoundaryInstance, request.CapabilityGrantDigest, openIntent, c.now().Add(request.FenceTTL))
	endpoint, err := c.Execution.Open(ctx, ports.ExecutionOpenRequest{Scope: commitSnapshot.Scope, RequirementDigest: request.RequirementDigest, Intent: openIntent, Fence: openFence})
	if err != nil {
		return nil, err
	}
	if _, err = transition(aggregate, core.InstanceState{Phase: core.PhaseStarting, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true}, core.TransitionContext{}); err != nil {
		return nil, err
	}

	environmentObservation, err := c.Environment.Inspect(ctx, allocation.Lease)
	if err != nil {
		return nil, err
	}
	executionObservation, err := c.Execution.Inspect(ctx, ports.ExecutionInspectRequest{Scope: commitSnapshot.Scope, Endpoint: endpoint, InspectKind: "ready"})
	if err != nil {
		return nil, err
	}
	if environmentObservation.State != "active" || executionObservation.SourceComponentID != endpoint.ComponentID || executionObservation.ObservationKind != "ready:ready" {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonReadyEvidenceIncomplete, "independent environment and execution evidence do not establish ready")
	}
	if _, err = transition(aggregate, core.InstanceState{Phase: core.PhaseReady, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true}, core.TransitionContext{}); err != nil {
		return nil, err
	}
	return &Instance{aggregate: aggregate, legacyRuns: kernel.NewRunRegistry(), plan: request.Plan, bindings: bindings, identityLease: identityLease, activation: attempt, endpoint: endpoint, capabilityDigest: request.CapabilityGrantDigest, fenceTTL: request.FenceTTL}, nil
}

// StartRun is restricted legacy compatibility. It uses an in-process registry
// and must not be imported by Application or production composition.
func (c *Coordinator) StartRun(_ context.Context, instance *Instance, runID core.AgentRunID, sessionRef string) (core.AgentRunRecord, error) {
	if instance == nil {
		return core.AgentRunRecord{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "instance is required")
	}
	instance.mu.Lock()
	defer instance.mu.Unlock()
	snapshot := instance.aggregate.Snapshot()
	if snapshot.State.Phase != core.PhaseReady || instance.activeRun != nil {
		return core.AgentRunRecord{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "instance is not ready for a new run")
	}
	record, err := instance.legacyRuns.Start(snapshot.Scope, runID, sessionRef, c.now())
	if err != nil {
		return core.AgentRunRecord{}, err
	}
	if _, err = transition(instance.aggregate, core.InstanceState{Phase: core.PhaseRunning, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true}, core.TransitionContext{}); err != nil {
		_, _ = instance.legacyRuns.Finish(snapshot.Scope, runID, core.OutcomeFailed, c.now())
		return core.AgentRunRecord{}, err
	}
	instance.activeRun = &record
	return record, nil
}

type StopRequest struct {
	Outcome       core.ExecutionOutcome
	Reason        string
	CloseIntent   core.EffectIntent
	ReleaseIntent core.EffectIntent
}

// Stop is restricted legacy compatibility. StopRequest.Outcome is caller data
// and therefore this path must never be used by Application or production V2.
func (c *Coordinator) Stop(ctx context.Context, instance *Instance, request StopRequest) (core.TerminationReport, error) {
	if instance == nil {
		return core.TerminationReport{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "instance is required")
	}
	instance.mu.Lock()
	defer instance.mu.Unlock()
	snapshot := instance.aggregate.Snapshot()
	if snapshot.State.Phase != core.PhaseReady && snapshot.State.Phase != core.PhaseRunning {
		return core.TerminationReport{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "only ready or running instances can use the normal stop closure")
	}
	if instance.activeRun != nil {
		finished, err := instance.legacyRuns.Finish(snapshot.Scope, instance.activeRun.ID, request.Outcome, c.now())
		if err != nil {
			return core.TerminationReport{}, err
		}
		instance.activeRun = &finished
	}
	if _, err := transition(instance.aggregate, core.InstanceState{Phase: core.PhaseStopping, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true}, core.TransitionContext{}); err != nil {
		return core.TerminationReport{}, err
	}
	snapshot = instance.aggregate.Snapshot()
	closeIntent, err := c.persistIntent(ctx, snapshot.Scope, request.CloseIntent, "execution_close")
	if err != nil {
		return core.TerminationReport{}, err
	}
	closeFence := makeFence(snapshot.Scope, core.FenceBoundaryInstance, instance.capabilityDigest, closeIntent, c.now().Add(instance.fenceTTL))
	closeObservation, err := c.Execution.Close(ctx, ports.ExecutionCloseRequest{Scope: snapshot.Scope, Endpoint: instance.endpoint, Reason: request.Reason, Intent: closeIntent, Fence: closeFence})
	if err != nil || closeObservation.ObservationKind != "closed" {
		return core.TerminationReport{}, core.NewError(core.ErrorIndeterminate, core.ReasonCleanupEvidenceIncomplete, "execution close lacks authoritative observation")
	}
	releaseIntent, err := c.persistIntent(ctx, snapshot.Scope, request.ReleaseIntent, "sandbox_release")
	if err != nil {
		return core.TerminationReport{}, err
	}
	fenceObservation, err := c.Environment.Fence(ctx, ports.SandboxFenceRequest{Lease: *snapshot.Scope.SandboxLease, Reason: request.Reason})
	if err != nil || fenceObservation.State != "fenced" {
		return core.TerminationReport{}, core.NewError(core.ErrorIndeterminate, core.ReasonCleanupEvidenceIncomplete, "sandbox fence lacks complete evidence")
	}
	releaseFence := makeFence(snapshot.Scope, core.FenceBoundaryInstance, instance.capabilityDigest, releaseIntent, c.now().Add(instance.fenceTTL))
	releaseObservation, err := c.Environment.Release(ctx, ports.SandboxReleaseRequest{Lease: *snapshot.Scope.SandboxLease, Intent: releaseIntent, Fence: releaseFence})
	if err != nil || releaseObservation.State != "released" {
		return core.TerminationReport{}, core.NewError(core.ErrorIndeterminate, core.ReasonCleanupEvidenceIncomplete, "sandbox release lacks complete evidence")
	}
	identityLease, err := c.IdentityLeases.RevokeIdentityLease(ctx, control.EndIdentityLeaseRequest{LeaseID: instance.identityLease.ID, ExpectedRevision: instance.identityLease.Revision, Reason: request.Reason})
	if err != nil {
		return core.TerminationReport{}, err
	}
	identityLease, err = c.IdentityLeases.ReleaseIdentityLease(ctx, control.EndIdentityLeaseRequest{LeaseID: identityLease.ID, ExpectedRevision: identityLease.Revision, Reason: request.Reason})
	if err != nil {
		return core.TerminationReport{}, err
	}
	instance.identityLease = identityLease
	terminal, err := transition(instance.aggregate, core.InstanceState{Phase: core.PhaseTerminal, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupComplete, HasCleanupObligations: true, CleanupEvidenceComplete: true}, core.TransitionContext{})
	if err != nil {
		return core.TerminationReport{}, err
	}
	report := core.TerminationReport{Scope: terminal.Scope, State: terminal.State, ExecutionOutcome: request.Outcome, EffectSettlement: "settled", RemoteContinuationsStatus: "none", ProviderRetentionStatus: "none", CompletedAt: c.now()}
	if err := report.Validate(); err != nil {
		return core.TerminationReport{}, err
	}
	return report, nil
}

type CheckpointRequest struct {
	ID             string
	Epoch          core.Epoch
	BarrierID      string
	Effects        core.EffectWatermarks
	EventWatermark core.TimelinePoint
}

func (c *Coordinator) Checkpoint(ctx context.Context, instance *Instance, request CheckpointRequest) (core.CheckpointSet, error) {
	if instance == nil {
		return core.CheckpointSet{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "instance is required")
	}
	if len(c.CheckpointParticipants) == 0 {
		return core.CheckpointSet{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonCheckpointUnsupported, "no checkpoint participant is bound")
	}
	instance.mu.Lock()
	defer instance.mu.Unlock()
	snapshot := instance.aggregate.Snapshot()
	if snapshot.State.Phase != core.PhaseReady && snapshot.State.Phase != core.PhaseRunning {
		return core.CheckpointSet{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "checkpoint requires a ready or running instance")
	}
	prepared := make([]CheckpointBinding, 0, len(c.CheckpointParticipants))
	reports := make([]core.CheckpointParticipantSnapshot, 0, len(c.CheckpointParticipants))
	for _, binding := range c.CheckpointParticipants {
		report, err := binding.Port.PrepareCheckpoint(ctx, ports.CheckpointPrepareRequest{BarrierID: request.BarrierID, Epoch: request.Epoch, Scope: snapshot.Scope, Effects: request.Effects})
		if err != nil {
			for index, prior := range prepared {
				aborted, abortErr := prior.Port.AbortCheckpoint(ctx, ports.CheckpointAbortRequest{BarrierID: request.BarrierID, Epoch: request.Epoch, Reason: "participant_prepare_failed"})
				if abortErr == nil {
					reports[index] = checkpointSnapshot(aborted, prior.Required)
				}
			}
			partial := c.buildCheckpoint(instance, request, snapshot.Scope, reports, core.CheckpointPartial)
			return partial, err
		}
		prepared = append(prepared, binding)
		reports = append(reports, checkpointSnapshot(report, binding.Required))
	}
	for index, binding := range prepared {
		report, err := binding.Port.CommitCheckpoint(ctx, ports.CheckpointCommitRequest{BarrierID: request.BarrierID, Epoch: request.Epoch})
		if err != nil {
			partial := c.buildCheckpoint(instance, request, snapshot.Scope, reports, core.CheckpointIndeterminate)
			return partial, err
		}
		reports[index] = checkpointSnapshot(report, binding.Required)
	}
	checkpoint := c.buildCheckpoint(instance, request, snapshot.Scope, reports, core.CheckpointConsistent)
	if err := checkpoint.Validate(); err != nil {
		return core.CheckpointSet{}, err
	}
	return checkpoint, nil
}

type RestoreResult struct {
	ProposedScope core.ExecutionScope                 `json:"proposed_scope"`
	Reports       []ports.CheckpointParticipantReport `json:"participant_reports"`
}

func (c *Coordinator) Restore(ctx context.Context, request core.RestoreRequest) (RestoreResult, error) {
	if err := request.Validate(); err != nil {
		return RestoreResult{}, err
	}
	newScope := request.Checkpoint.Scope
	newScope.Identity.Epoch++
	newScope.Instance = request.NewInstance
	newScope.SandboxLease = nil
	newScope.AuthorityEpoch = request.CurrentAuthorityEpoch
	reports := make([]ports.CheckpointParticipantReport, 0, len(c.CheckpointParticipants))
	snapshots := make(map[string]core.CheckpointParticipantSnapshot, len(request.Checkpoint.Participants))
	for _, snapshot := range request.Checkpoint.Participants {
		snapshots[snapshot.ComponentID] = snapshot
	}
	for _, binding := range c.CheckpointParticipants {
		descriptor, err := binding.Port.Describe(ctx)
		if err != nil {
			return RestoreResult{}, err
		}
		snapshot, exists := snapshots[descriptor.ID]
		if !exists {
			if binding.Required {
				return RestoreResult{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRestoreIncompatible, "required restore participant snapshot is missing")
			}
			continue
		}
		report, err := binding.Port.RestoreCheckpoint(ctx, ports.CheckpointRestoreRequest{CheckpointID: request.Checkpoint.ID, SnapshotRef: snapshot.SnapshotRef, SnapshotDigest: snapshot.SnapshotDigest, NewScope: newScope})
		if err != nil {
			return RestoreResult{}, err
		}
		reports = append(reports, report)
	}
	return RestoreResult{ProposedScope: newScope, Reports: reports}, nil
}

// RecoverActivation returns the single safe next action from the durable
// journal. The caller must execute that action through the owning Port and
// persist its outcome before planning again.
func (c *Coordinator) RecoverActivation(ctx context.Context, attemptID string) (admission.ActivationAttempt, admission.RecoveryDecision, error) {
	if c.ActivationFacts == nil {
		return admission.ActivationAttempt{}, admission.RecoveryDecision{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "activation fact port is required")
	}
	attempt, err := c.ActivationFacts.InspectActivationAttempt(ctx, attemptID)
	if err != nil {
		return admission.ActivationAttempt{}, admission.RecoveryDecision{}, err
	}
	decision, err := admission.PlanRecovery(attempt, c.now())
	return attempt, decision, err
}

func (c *Coordinator) buildCheckpoint(instance *Instance, request CheckpointRequest, scope core.ExecutionScope, reports []core.CheckpointParticipantSnapshot, consistency core.CheckpointConsistency) core.CheckpointSet {
	return core.CheckpointSet{ID: request.ID, Epoch: request.Epoch, BarrierID: request.BarrierID, Scope: scope, PlanDigest: instance.plan.Digest, ProfileDigest: instance.plan.ProfileDigest, ContextDigest: instance.plan.ContextDigest, AuthorityEpoch: scope.AuthorityEpoch, Effects: request.Effects, EventWatermark: request.EventWatermark, Participants: reports, Consistency: consistency, CreatedAt: c.now()}
}

func checkpointSnapshot(report ports.CheckpointParticipantReport, required bool) core.CheckpointParticipantSnapshot {
	return core.CheckpointParticipantSnapshot{ComponentID: report.ComponentID, ComponentKind: string(report.ComponentKind), Required: required, State: report.State, SnapshotRef: report.SnapshotRef, SnapshotDigest: report.SnapshotDigest}
}

func (c *Coordinator) persistIntent(ctx context.Context, scope core.ExecutionScope, intent core.EffectIntent, kind string) (core.EffectIntent, error) {
	intent.PersistedAt = c.now()
	if err := intent.Validate(); err != nil {
		return core.EffectIntent{}, err
	}
	ref, err := c.Evidence.AppendIntent(ctx, ports.EvidenceIntentRecord{Scope: scope, Kind: kind, PayloadDigest: intent.CanonicalPayloadDigest, CausationID: string(intent.ID)})
	if err != nil {
		return core.EffectIntent{}, err
	}
	record, err := c.Evidence.Read(ctx, ref)
	if err != nil {
		return core.EffectIntent{}, err
	}
	intent.PersistedAt = record.RecordedAt
	return intent, intent.Validate()
}

func (c *Coordinator) validate() error {
	if c.Registry == nil || c.Execution == nil || c.Environment == nil || c.Evidence == nil || c.IdentityLeases == nil || c.ActivationFacts == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "registry, execution, environment, evidence, identity lease and activation fact ports are required")
	}
	return nil
}

func (c *Coordinator) now() time.Time {
	if c.Clock == nil {
		return time.Now()
	}
	return c.Clock()
}

func validateActivationRequest(request ActivationRequest, now time.Time) error {
	if err := request.Plan.Validate(); err != nil {
		return err
	}
	if err := request.ProposedScope.Validate(); err != nil {
		return err
	}
	if request.ProposedScope.SandboxLease != nil || request.Plan.Digest != request.ProposedScope.Lineage.PlanDigest || strings.TrimSpace(request.ActivationAttemptID) == "" {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "activation requires a lease-free proposed scope bound to the resolved plan")
	}
	if err := request.RequirementDigest.Validate(); err != nil {
		return err
	}
	if err := request.CapabilityGrantDigest.Validate(); err != nil {
		return err
	}
	if request.ProbeBudget.MaxRequests == 0 || request.ProbeBudget.MaxDuration <= 0 || request.ProbeBudget.PossibleCharge || request.ProbeBudget.PossibleMutation {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonPlanInvalid, "minimal foundation only admits bounded read-only preflight")
	}
	if request.FenceTTL <= 0 || !request.IdentityLeaseExpiresAt.After(now) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "activation requires future lease expiry and positive fence ttl")
	}
	for _, intent := range []core.EffectIntent{request.AllocateIntent, request.ActivateIntent, request.OpenIntent} {
		candidate := intent
		candidate.PersistedAt = now
		if err := candidate.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func buildActivationSnapshot(request ActivationRequest, preflight ports.ExecutionPreflightReport) (admission.ActivationSnapshot, error) {
	expiresAt := preflight.EvidenceExpiry
	if request.IdentityLeaseExpiresAt.Before(expiresAt) {
		expiresAt = request.IdentityLeaseExpiresAt
	}
	snapshot := admission.ActivationSnapshot{
		AuthorityEpoch:           request.ProposedScope.AuthorityEpoch,
		EntitlementDigest:        request.Plan.AuthorityDigest,
		RouteDigest:              request.Plan.Digest,
		CapabilityEvidenceDigest: preflight.EvidenceDigest,
		PolicyDigest:             request.Plan.AuthorityDigest,
		BudgetPolicyDigest:       request.Plan.AuthorityDigest,
		SandboxRequirementDigest: request.RequirementDigest,
		EvidenceExpiresAt:        expiresAt,
	}
	digest, err := core.DigestJSON(struct {
		Scope     core.ExecutionScope
		Plan      core.Digest
		Preflight core.Digest
		Sandbox   core.Digest
		ExpiresAt time.Time
	}{request.ProposedScope, request.Plan.Digest, preflight.EvidenceDigest, request.RequirementDigest, expiresAt})
	if err != nil {
		return admission.ActivationSnapshot{}, err
	}
	snapshot.Digest = digest
	return snapshot, snapshot.Validate()
}

func (c *Coordinator) advanceActivation(ctx context.Context, current admission.ActivationAttempt, stage admission.ActivationStage, mutate func(*admission.ActivationAttempt)) (admission.ActivationAttempt, error) {
	next := current
	next.Stage = stage
	next.Revision++
	next.UpdatedAt = c.monotonicTimestamp(current.UpdatedAt)
	if mutate != nil {
		mutate(&next)
	}
	return c.ActivationFacts.CompareAndSwapActivation(ctx, next, admission.TransitionContext{})
}

func (c *Coordinator) quarantineActivation(ctx context.Context, current admission.ActivationAttempt, mutate func(*admission.ActivationAttempt), reason string) error {
	next := current
	next.Recovery = admission.RecoveryQuarantined
	next.FailureReason = reason
	next.Revision++
	next.UpdatedAt = c.monotonicTimestamp(current.UpdatedAt)
	mutate(&next)
	_, err := c.ActivationFacts.CompareAndSwapActivation(ctx, next, admission.TransitionContext{})
	return err
}

func (c *Coordinator) monotonicTimestamp(previous time.Time) time.Time {
	now := c.now()
	if now.Before(previous) {
		return previous
	}
	return now
}

func transition(aggregate *kernel.Aggregate, next core.InstanceState, context core.TransitionContext) (kernel.Snapshot, error) {
	snapshot := aggregate.Snapshot()
	return aggregate.Transition(kernel.TransitionRequest{Preconditions: preconditions(snapshot), NextState: next, Context: context})
}

func preconditions(snapshot kernel.Snapshot) core.ExecutionPreconditions {
	conditions := core.ExecutionPreconditions{IdentityEpoch: snapshot.Scope.Identity.Epoch, InstanceEpoch: snapshot.Scope.Instance.Epoch, AuthorityEpoch: snapshot.Scope.AuthorityEpoch, Revision: snapshot.Revision}
	if snapshot.Scope.SandboxLease != nil {
		epoch := snapshot.Scope.SandboxLease.Epoch
		conditions.LeaseEpoch = &epoch
	}
	return conditions
}

func makeFence(scope core.ExecutionScope, boundary core.FenceBoundaryScope, capabilityDigest core.Digest, intent core.EffectIntent, expiresAt time.Time) core.ExecutionFence {
	return core.ExecutionFence{BoundaryScope: boundary, Scope: scope, CapabilityGrantDigest: capabilityDigest, EffectIntentID: intent.ID, EffectIntentRevision: intent.Revision, CanonicalPayloadDigest: intent.CanonicalPayloadDigest, ExpiresAt: expiresAt}
}

func (i InstanceSnapshot) String() string {
	return fmt.Sprintf("%s/%s phase=%s revision=%d", i.Kernel.Scope.Identity.ID, i.Kernel.Scope.Instance.ID, i.Kernel.State.Phase, i.Kernel.Revision)
}
