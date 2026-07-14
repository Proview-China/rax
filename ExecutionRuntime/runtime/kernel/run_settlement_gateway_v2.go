package kernel

import (
	"context"
	"sort"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RunSettlementGatewayV2 is the sole Runtime authority that derives a Run
// outcome. Requests intentionally contain no outcome, success flag, or free-form
// disposition; the Gateway derives them from current authoritative facts.
type RunSettlementGatewayV2 struct {
	Facts          control.RunSettlementFactPortV2
	Effects        control.RunEffectFactPortV2
	Claims         ports.RunClaimAssociationPortV2
	Evidence       ports.EvidenceRecordReaderV2
	Execution      ports.RunExecutionSettlementInspectorV2
	Participants   ports.RunSettlementParticipantPortV2
	Policies       ports.RunSettlementPolicyReaderV2
	Bindings       control.BindingFactPortV2
	Authority      ports.AuthorityFactReaderV2
	PlanAdmissions ports.RunSettlementPlanAdmissionPortV3
	Clock          func() time.Time
}

type StopAndSettleRunRequestV2 struct {
	Scope               core.ExecutionScope `json:"scope"`
	RunID               core.AgentRunID     `json:"run_id"`
	ExpectedRunRevision core.Revision       `json:"expected_run_revision"`
}

type StopAndSettleRunResultV2 struct {
	Run      core.AgentRunRecord                  `json:"run"`
	Closure  control.RunSettlementClosureFactV2   `json:"closure"`
	Decision control.RunSettlementDecisionFactV2  `json:"decision"`
	Progress control.RunTerminationProgressFactV2 `json:"progress"`
}

// ReconcileTerminationProgressV2 advances only post-terminal barriers from
// independently re-inspected participant facts. It never rewrites the immutable
// Run outcome or Decision.
func (g RunSettlementGatewayV2) ReconcileTerminationProgressV2(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (control.RunTerminationProgressFactV2, error) {
	if err := g.validate(); err != nil {
		return control.RunTerminationProgressFactV2{}, err
	}
	run, err := g.Facts.InspectRun(ctx, scope, runID)
	if err != nil {
		return control.RunTerminationProgressFactV2{}, err
	}
	if run.Status != core.RunTerminal {
		return control.RunTerminationProgressFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonTerminationProgressConflict, "termination Progress exists only after terminal Run commit")
	}
	plan, err := g.Facts.InspectRunSettlementPlanV2(ctx, scope, runID)
	if err != nil {
		return control.RunTerminationProgressFactV2{}, err
	}
	currentClosure, err := g.Facts.InspectCurrentRunSettlementClosureV2(ctx, scope, runID)
	if err != nil {
		return control.RunTerminationProgressFactV2{}, err
	}
	progress, err := g.Facts.InspectRunTerminationProgressV2(ctx, scope, runID)
	if err != nil {
		return control.RunTerminationProgressFactV2{}, err
	}
	next := progress
	next.Items = append([]control.RunSettlementResolutionV2{}, progress.Items...)
	changed := false
	for index, item := range next.Items {
		if item.Disposition != ports.RunSettlementUnknown {
			continue
		}
		requirement, found := findRequirementV2(plan, item.RequirementID)
		link, linked := closureParticipantLinkV2(currentClosure.Closure, item.RequirementID)
		if !found || !linked {
			return control.RunTerminationProgressFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonTerminationProgressConflict, "termination requirement is absent from Plan or Closure")
		}
		request := ports.RunSettlementParticipantInspectRequestV2{RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, Plan: currentClosure.Closure.Plan, RequirementID: requirement.ID, RequirementDigest: link.RequirementDigest, SubjectDigest: requirement.SubjectDigest, Owner: requirement.Owner}
		participant, err := g.Participants.InspectRunSettlementParticipant(ctx, request)
		if err != nil {
			return control.RunTerminationProgressFactV2{}, err
		}
		ref, err := participant.RefV2()
		if err != nil || g.validateParticipant(ctx, plan, requirement, participant) != nil {
			return control.RunTerminationProgressFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementParticipantStale, "termination participant is stale")
		}
		if participant.Disposition == ports.RunSettlementUnknown {
			// Re-reading the same unresolved fact is not progress. In particular,
			// do not create an unbounded CAS/revision stream during 24x7 polling.
			continue
		}
		// A newer exact participant is allowed to close a previously unknown item;
		// the immutable Closure ref remains its provenance watermark.
		next.Items[index].Disposition = participant.Disposition
		next.Items[index].Participant = &ref
		next.Items[index].EvidenceDigest = ref.Digest
		changed = true
	}
	if !changed {
		return progress, nil
	}
	next.Revision++
	next.UpdatedUnixNano = g.Clock().UnixNano()
	updated, err := g.Facts.CompareAndSwapRunTerminationProgressV2(ctx, control.RunTerminationProgressCASRequestV2{ExpectedRevision: progress.Revision, Next: next})
	if err == nil {
		return updated, nil
	}
	if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) {
		return control.RunTerminationProgressFactV2{}, err
	}
	inspected, inspectErr := g.Facts.InspectRunTerminationProgressV2(ctx, scope, runID)
	if inspectErr == nil && sameProgressDigestV2(inspected, next) {
		return inspected, nil
	}
	return control.RunTerminationProgressFactV2{}, err
}

func (g RunSettlementGatewayV2) BuildTerminationReportV2(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (control.RunTerminationReportV2, error) {
	if err := g.validate(); err != nil {
		return control.RunTerminationReportV2{}, err
	}
	run, err := g.Facts.InspectRun(ctx, scope, runID)
	if err != nil {
		return control.RunTerminationReportV2{}, err
	}
	decision, err := g.Facts.InspectRunSettlementDecisionV2(ctx, scope, runID)
	if err != nil {
		return control.RunTerminationReportV2{}, err
	}
	progress, err := g.Facts.InspectRunTerminationProgressV2(ctx, scope, runID)
	if err != nil {
		return control.RunTerminationReportV2{}, err
	}
	report, err := control.BuildRunTerminationReportV2(run, decision, progress)
	if err != nil {
		return control.RunTerminationReportV2{}, err
	}
	created, err := g.Facts.CreateRunTerminationReportV2(ctx, report)
	if err == nil {
		return created, nil
	}
	if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) {
		return control.RunTerminationReportV2{}, err
	}
	inspected, inspectErr := g.Facts.InspectRunTerminationReportV2(ctx, scope, runID)
	left, leftErr := inspected.DigestV2()
	right, rightErr := report.DigestV2()
	if inspectErr == nil && leftErr == nil && rightErr == nil && left == right {
		return inspected, nil
	}
	return control.RunTerminationReportV2{}, err
}

// BeginStopRunV2 establishes the durable freeze boundary. A lost CAS reply is
// recovered only by Inspect; the Gateway never submits a second blind mutation.
func (g RunSettlementGatewayV2) BeginStopRunV2(ctx context.Context, request StopAndSettleRunRequestV2) (core.AgentRunRecord, error) {
	if err := g.validate(); err != nil {
		return core.AgentRunRecord{}, err
	}
	current, err := g.Facts.InspectRun(ctx, request.Scope, request.RunID)
	if err != nil {
		return core.AgentRunRecord{}, err
	}
	if current.Status == core.RunStopping {
		return current, nil
	}
	if current.Status != core.RunRunning || current.Revision != request.ExpectedRunRevision {
		return core.AgentRunRecord{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "BeginStop requires the exact running Run revision")
	}
	next := current
	next.Status = core.RunStopping
	next.Revision++
	stopped, err := g.Facts.CompareAndSwapRun(ctx, control.RunFactCASRequest{ExpectedRevision: current.Revision, Next: next})
	if err == nil {
		return stopped, nil
	}
	if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) {
		return core.AgentRunRecord{}, err
	}
	inspected, inspectErr := g.Facts.InspectRun(ctx, request.Scope, request.RunID)
	if inspectErr != nil {
		return core.AgentRunRecord{}, err
	}
	if inspected.Status == core.RunStopping && inspected.Revision == next.Revision {
		return inspected, nil
	}
	return core.AgentRunRecord{}, err
}

// StopAndSettleRunV2 freezes, independently inspects, creates an immutable
// Closure, derives the Decision, and atomically commits Decision+terminal Run.
func (g RunSettlementGatewayV2) StopAndSettleRunV2(ctx context.Context, request StopAndSettleRunRequestV2) (StopAndSettleRunResultV2, error) {
	if err := g.validate(); err != nil {
		return StopAndSettleRunResultV2{}, err
	}
	current, inspectErr := g.Facts.InspectRun(ctx, request.Scope, request.RunID)
	if inspectErr == nil && current.Status == core.RunTerminal {
		decision, decisionErr := g.Facts.InspectRunSettlementDecisionV2(ctx, request.Scope, request.RunID)
		progress, progressErr := g.Facts.InspectRunTerminationProgressV2(ctx, request.Scope, request.RunID)
		attempt, attemptErr := g.Facts.InspectCurrentRunSettlementClosureV2(ctx, request.Scope, request.RunID)
		runIdentity, identityErr := ports.RunIdentityDigestV2(current)
		decisionRef, refErr := decision.RefV2()
		if decisionErr != nil || progressErr != nil || attemptErr != nil || identityErr != nil || refErr != nil || decision.ExpectedRunRevision != request.ExpectedRunRevision+1 || decision.RunIdentityDigest != runIdentity || decision.ExecutionScopeDigest != attempt.Closure.ExecutionScopeDigest || decision.Closure != attempt.Pointer.Current || decision.ClosurePointerRevision != attempt.Pointer.Revision || current.Outcome != decision.Outcome || progress.Decision != decisionRef {
			return StopAndSettleRunResultV2{}, core.NewError(core.ErrorInternal, core.ReasonRunCompletionAtomicityBroken, "terminal settlement replay cannot inspect one exact committed result")
		}
		return StopAndSettleRunResultV2{Run: current, Closure: attempt.Closure, Decision: decision, Progress: progress}, nil
	}
	stopping, err := g.BeginStopRunV2(ctx, request)
	if err != nil {
		return StopAndSettleRunResultV2{}, err
	}
	closure, plan, err := g.buildOrInspectClosure(ctx, stopping)
	if err != nil {
		return StopAndSettleRunResultV2{}, err
	}
	decision, progress, err := g.deriveDecision(ctx, stopping, plan, closure)
	if err != nil {
		return StopAndSettleRunResultV2{}, err
	}
	committed, err := g.Facts.CommitRunCompletionV2(ctx, control.CommitRunCompletionRequestV2{ExecutionScope: stopping.Scope, ExpectedRunRevision: stopping.Revision, Decision: decision, InitialProgress: progress})
	if err == nil {
		return StopAndSettleRunResultV2{Run: committed.Run, Closure: closure, Decision: committed.Decision, Progress: committed.Progress}, nil
	}
	if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) {
		return StopAndSettleRunResultV2{}, err
	}
	recovered, recoveryErr := g.inspectCommitted(ctx, stopping, decision)
	if recoveryErr != nil {
		return StopAndSettleRunResultV2{}, recoveryErr
	}
	return StopAndSettleRunResultV2{Run: recovered.Run, Closure: closure, Decision: recovered.Decision, Progress: recovered.Progress}, nil
}

func (g RunSettlementGatewayV2) buildOrInspectClosure(ctx context.Context, run core.AgentRunRecord) (control.RunSettlementClosureFactV2, ports.RunSettlementPlanFactV2, error) {
	plan, err := g.Facts.InspectRunSettlementPlanV2(ctx, run.Scope, run.ID)
	if err != nil {
		return control.RunSettlementClosureFactV2{}, ports.RunSettlementPlanFactV2{}, err
	}
	if err := g.validatePlanCurrent(ctx, run, plan); err != nil {
		return control.RunSettlementClosureFactV2{}, ports.RunSettlementPlanFactV2{}, err
	}
	currentAttempt, currentErr := g.Facts.InspectCurrentRunSettlementClosureV2(ctx, run.Scope, run.ID)
	if currentErr == nil && g.revalidateClosure(ctx, run, plan, currentAttempt.Closure) == nil {
		return currentAttempt.Closure, plan, nil
	}
	if currentErr != nil && !core.HasCategory(currentErr, core.ErrorNotFound) {
		return control.RunSettlementClosureFactV2{}, ports.RunSettlementPlanFactV2{}, currentErr
	}
	partition := control.RunEffectPartitionV2{ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest}
	index, err := g.Effects.InspectRunEffectIndexV2(ctx, partition)
	if err != nil {
		return control.RunSettlementClosureFactV2{}, ports.RunSettlementPlanFactV2{}, err
	}
	if index.State == control.RunEffectIndexOpen {
		index, err = g.Effects.FreezeRunEffectSetV2(ctx, control.FreezeRunEffectSetRequestV2{Partition: partition, ExpectedIndexRevision: index.Revision, ExpectedRunRevision: run.Revision})
		if err != nil {
			if core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate) {
				inspected, inspectErr := g.Effects.InspectRunEffectIndexV2(ctx, partition)
				if inspectErr == nil && inspected.State == control.RunEffectIndexFrozen {
					index, err = inspected, nil
				}
			}
			if err != nil {
				return control.RunSettlementClosureFactV2{}, ports.RunSettlementPlanFactV2{}, err
			}
		}
	}
	if index.State != control.RunEffectIndexFrozen || index.RunIdentityDigest != plan.RunIdentityDigest || index.ExecutionScopeDigest != plan.ExecutionScopeDigest {
		return control.RunSettlementClosureFactV2{}, ports.RunSettlementPlanFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectSetFrozen, "frozen Effect set does not bind the exact Run")
	}
	claim, err := g.inspectClaim(ctx, run, plan)
	if err != nil {
		return control.RunSettlementClosureFactV2{}, ports.RunSettlementPlanFactV2{}, err
	}
	execution, err := g.inspectExecution(ctx, run, plan)
	if err != nil {
		return control.RunSettlementClosureFactV2{}, ports.RunSettlementPlanFactV2{}, err
	}
	participants, err := g.inspectParticipants(ctx, run, plan)
	if err != nil {
		return control.RunSettlementClosureFactV2{}, ports.RunSettlementPlanFactV2{}, err
	}
	indexDigest, _ := index.DigestV2()
	planRef, _ := plan.RefV2()
	attempt := uint64(1)
	previousDigest := ports.EvidenceGenesisDigestV2
	if currentErr == nil {
		attempt = currentAttempt.Closure.Attempt + 1
		previousDigest, _ = currentAttempt.Closure.DigestV2()
	}
	identityDigest, err := core.CanonicalJSONDigest("praxis.runtime.run-settlement", ports.RunSettlementContractVersionV2, "RunSettlementClosureIdentityV2", struct {
		RunID        core.AgentRunID                             `json:"run_id"`
		Attempt      uint64                                      `json:"attempt"`
		Previous     core.Digest                                 `json:"previous_digest"`
		RunRevision  core.Revision                               `json:"run_revision"`
		Plan         ports.RunSettlementPlanRefV2                `json:"plan"`
		Effect       core.Digest                                 `json:"effect_set_digest"`
		Execution    ports.RunExecutionInspectionRefV2           `json:"execution"`
		Participants []control.RunSettlementClosureParticipantV2 `json:"participants"`
	}{run.ID, attempt, previousDigest, run.Revision, planRef, indexDigest, mustInspectionRefV2(execution), participants})
	if err != nil {
		return control.RunSettlementClosureFactV2{}, ports.RunSettlementPlanFactV2{}, err
	}
	closure := control.RunSettlementClosureFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: control.RunSettlementFactIDV2("closure", run.ID, identityDigest), Revision: 1, RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest, RunRevision: run.Revision, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, Attempt: attempt, PreviousClosureDigest: previousDigest, Plan: planRef, Claim: claim, Execution: execution, EffectSet: control.RunEffectSetRefV2{IndexID: index.ID, Revision: index.Revision, Digest: indexDigest, Watermark: index.Watermark, SegmentCount: index.SegmentCount, EffectCount: index.EffectCount, HeadSegmentDigest: index.HeadSegmentDigest}, Participants: participants, CreatedUnixNano: g.Clock().UnixNano()}
	created, err := g.Facts.CreateRunSettlementClosureAttemptV2(ctx, closure)
	if err == nil {
		return created.Closure, plan, nil
	}
	if !core.HasCategory(err, core.ErrorUnavailable) && !core.HasCategory(err, core.ErrorIndeterminate) {
		return control.RunSettlementClosureFactV2{}, ports.RunSettlementPlanFactV2{}, err
	}
	inspected, inspectErr := g.Facts.InspectRunSettlementClosureAttemptV2(ctx, run.Scope, run.ID, attempt)
	if inspectErr != nil || !sameClosureDigestV2(inspected, closure) {
		return control.RunSettlementClosureFactV2{}, ports.RunSettlementPlanFactV2{}, err
	}
	return inspected, plan, nil
}

func (g RunSettlementGatewayV2) deriveDecision(ctx context.Context, run core.AgentRunRecord, plan ports.RunSettlementPlanFactV2, closure control.RunSettlementClosureFactV2) (control.RunSettlementDecisionFactV2, control.RunTerminationProgressFactV2, error) {
	if err := g.validatePlanCurrent(ctx, run, plan); err != nil {
		return control.RunSettlementDecisionFactV2{}, control.RunTerminationProgressFactV2{}, err
	}
	if err := g.revalidateClosure(ctx, run, plan, closure); err != nil {
		return control.RunSettlementDecisionFactV2{}, control.RunTerminationProgressFactV2{}, err
	}
	completion := make([]control.RunSettlementResolutionV2, 0, len(plan.Requirements))
	termination := make([]control.RunSettlementResolutionV2, 0, len(plan.Requirements))
	truth := closure.Execution.Truth
	needsReconciliation := false
	for _, requirement := range plan.Requirements {
		policy, err := g.inspectPolicy(ctx, plan, requirement)
		if err != nil {
			return control.RunSettlementDecisionFactV2{}, control.RunTerminationProgressFactV2{}, err
		}
		resolution, err := g.resolveRequirement(ctx, plan, closure, requirement, policy)
		if err != nil {
			return control.RunSettlementDecisionFactV2{}, control.RunTerminationProgressFactV2{}, err
		}
		if requirement.Phase == ports.RunSettlementPhaseCompletion {
			switch resolution.Disposition {
			case ports.RunSettlementUnknown:
				isExecutionTruth := requirement.Kind == ports.RunRequirementExecutionTruth && (truth == ports.RunExecutionUnknown || truth == ports.RunExecutionConfirmedLost)
				if isExecutionTruth {
					if policy.UnknownMode != ports.RunUnknownTerminalizeIndeterminate {
						return control.RunSettlementDecisionFactV2{}, control.RunTerminationProgressFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementBlocked, "unknown execution truth remains blocked by its exact policy")
					}
				} else if policy.UnknownMode == ports.RunUnknownTerminalizeReconciliation {
					needsReconciliation = true
				} else {
					return control.RunSettlementDecisionFactV2{}, control.RunTerminationProgressFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementBlocked, "unknown required dimension remains blocked by policy")
				}
			case ports.RunSettlementConfirmedFailed:
				if policy.FailureMode != ports.RunClosedFailureReconcile {
					return control.RunSettlementDecisionFactV2{}, control.RunTerminationProgressFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementBlocked, "confirmed failed dimension remains blocked by failure policy")
				}
				needsReconciliation = true
			case ports.RunSettlementConfirmedNotApplied:
				if policy.NotAppliedMode != ports.RunClosedFailureReconcile {
					return control.RunSettlementDecisionFactV2{}, control.RunTerminationProgressFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementBlocked, "confirmed not-applied dimension remains blocked by not-applied policy")
				}
				needsReconciliation = true
			}
			completion = append(completion, resolution)
		} else {
			termination = append(termination, resolution)
		}
	}
	outcome, err := deriveRunOutcomeV2(truth, needsReconciliation, plan, g, ctx)
	if err != nil {
		return control.RunSettlementDecisionFactV2{}, control.RunTerminationProgressFactV2{}, err
	}
	control.SortRunSettlementResolutionsV2(completion)
	control.SortRunSettlementResolutionsV2(termination)
	planRef, _ := plan.RefV2()
	closureRef, _ := closure.RefV2()
	executionRef, _ := closure.Execution.RefV2()
	decisionIdentity, err := core.CanonicalJSONDigest("praxis.runtime.run-settlement", ports.RunSettlementContractVersionV2, "RunSettlementDecisionIdentityV2", struct {
		Closure control.RunSettlementClosureRefV2 `json:"closure"`
		Outcome core.ExecutionOutcome             `json:"outcome"`
	}{closureRef, outcome})
	if err != nil {
		return control.RunSettlementDecisionFactV2{}, control.RunTerminationProgressFactV2{}, err
	}
	now := g.Clock()
	currentAttempt, err := g.Facts.InspectCurrentRunSettlementClosureV2(ctx, run.Scope, run.ID)
	currentRef, currentRefErr := currentAttempt.Closure.RefV2()
	if err != nil || currentRefErr != nil || currentRef != closureRef {
		return control.RunSettlementDecisionFactV2{}, control.RunTerminationProgressFactV2{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementClosureConflict, "Decision requires the current Closure attempt")
	}
	decision := control.RunSettlementDecisionFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: control.RunSettlementFactIDV2("decision", run.ID, decisionIdentity), Revision: 1, RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest, ExpectedRunRevision: run.Revision, ExecutionScopeDigest: plan.ExecutionScopeDigest, Plan: planRef, Closure: closureRef, ClosurePointerRevision: currentAttempt.Pointer.Revision, Claim: closure.Claim, Execution: executionRef, Resolutions: completion, Outcome: outcome, CreatedUnixNano: now.UnixNano()}
	decisionRef, err := decision.RefV2()
	if err != nil {
		return control.RunSettlementDecisionFactV2{}, control.RunTerminationProgressFactV2{}, err
	}
	progress := control.RunTerminationProgressFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: control.RunSettlementFactIDV2("termination-progress", run.ID, decisionRef.Digest), Revision: 1, RunID: run.ID, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, Decision: decisionRef, Items: termination, UpdatedUnixNano: now.UnixNano()}
	if err := progress.Validate(); err != nil {
		return control.RunSettlementDecisionFactV2{}, control.RunTerminationProgressFactV2{}, err
	}
	return decision, progress, nil
}

func (g RunSettlementGatewayV2) resolveRequirement(ctx context.Context, plan ports.RunSettlementPlanFactV2, closure control.RunSettlementClosureFactV2, requirement ports.RunSettlementRequirementV2, policy ports.RunSettlementPolicyFactV2) (control.RunSettlementResolutionV2, error) {
	requirementDigest, _ := requirement.DigestV2()
	base := control.RunSettlementResolutionV2{RequirementID: requirement.ID, Kind: requirement.Kind, Phase: requirement.Phase, Policy: requirement.Policy, EvidenceDigest: requirementDigest}
	switch requirement.Kind {
	case ports.RunRequirementExecutionTruth:
		base.Disposition = ports.RunSettlementConfirmedSatisfied
		if closure.Execution.Truth == ports.RunExecutionUnknown {
			base.Disposition = ports.RunSettlementUnknown
		}
		if closure.Execution.Truth == ports.RunExecutionConfirmedLost && !policy.AllowConfirmedLost {
			base.Disposition = ports.RunSettlementUnknown
		}
		base.EvidenceDigest = closure.Execution.PayloadDigest
		return base, nil
	case ports.RunRequirementEffects:
		partition := control.RunEffectPartitionV2{ExecutionScope: plan.ExecutionScope, ExecutionScopeDigest: plan.ExecutionScopeDigest, RunID: plan.RunID, RunIdentityDigest: plan.RunIdentityDigest}
		index, err := g.Effects.InspectRunEffectIndexV2(ctx, partition)
		if err != nil {
			return control.RunSettlementResolutionV2{}, err
		}
		if index.State != control.RunEffectIndexFrozen || !sameRunEffectSetRefV2(index, closure.EffectSet) {
			return control.RunSettlementResolutionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectSetFrozen, "Effect set changed after Closure")
		}
		base.Disposition = ports.RunSettlementConfirmedSatisfied
		refs, err := inspectFrozenRunEffectRefsV2(ctx, g.Effects, partition, index)
		if err != nil {
			return control.RunSettlementResolutionV2{}, err
		}
		for _, ref := range refs {
			effect, err := g.Effects.InspectRunEffectV2(ctx, partition, ref.EffectID)
			if err != nil || effect.IntentDigest != ref.IntentDigest || effect.Intent.Revision != ref.IntentRevision {
				return control.RunSettlementResolutionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectIndexConflict, "indexed Effect fact is missing or drifted")
			}
			base.Disposition = mergeRunSettlementDispositionV2(base.Disposition, runEffectSettlementDispositionV2(effect))
		}
		indexDigest, _ := index.DigestV2()
		base.EvidenceDigest = indexDigest
		return base, nil
	default:
		link, found := closureParticipantLinkV2(closure, requirement.ID)
		if !found {
			return control.RunSettlementResolutionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementParticipantMissing, "Closure lacks a required participant")
		}
		request := ports.RunSettlementParticipantInspectRequestV2{RunID: plan.RunID, RunIdentityDigest: plan.RunIdentityDigest, ExecutionScope: plan.ExecutionScope, ExecutionScopeDigest: plan.ExecutionScopeDigest, Plan: closure.Plan, RequirementID: requirement.ID, RequirementDigest: link.RequirementDigest, SubjectDigest: requirement.SubjectDigest, Owner: requirement.Owner}
		participant, err := g.Participants.InspectRunSettlementParticipant(ctx, request)
		if err != nil {
			return control.RunSettlementResolutionV2{}, err
		}
		participantRef, err := participant.RefV2()
		if err != nil || participantRef != link.Participant || g.validateParticipant(ctx, plan, requirement, participant) != nil {
			return control.RunSettlementResolutionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementParticipantStale, "required participant drifted after Closure")
		}
		base.Disposition = participant.Disposition
		base.Participant = &participantRef
		base.EvidenceDigest = participantRef.Digest
		return base, nil
	}
}

func runEffectSettlementDispositionV2(effect control.EffectFactV2) ports.RunSettlementDispositionV2 {
	switch effect.State {
	case control.EffectRejected:
		return ports.RunSettlementConfirmedNotApplied
	case control.EffectCompensated:
		return ports.RunSettlementConfirmedSatisfied
	case control.EffectSettled:
		if effect.Settlement == nil {
			return ports.RunSettlementUnknown
		}
		switch effect.Settlement.Disposition {
		case control.SettlementConfirmedApplied:
			return ports.RunSettlementConfirmedSatisfied
		case control.SettlementConfirmedNotApplied:
			return ports.RunSettlementConfirmedNotApplied
		case control.SettlementConfirmedFailed:
			return ports.RunSettlementConfirmedFailed
		default:
			return ports.RunSettlementUnknown
		}
	default:
		return ports.RunSettlementUnknown
	}
}

func mergeRunSettlementDispositionV2(current, next ports.RunSettlementDispositionV2) ports.RunSettlementDispositionV2 {
	priority := map[ports.RunSettlementDispositionV2]int{
		ports.RunSettlementConfirmedSatisfied:  0,
		ports.RunSettlementConfirmedNotApplied: 1,
		ports.RunSettlementConfirmedFailed:     2,
		ports.RunSettlementUnknown:             3,
	}
	if priority[next] > priority[current] {
		return next
	}
	return current
}

func (g RunSettlementGatewayV2) inspectClaim(ctx context.Context, run core.AgentRunRecord, plan ports.RunSettlementPlanFactV2) (*ports.RunClaimAssociationFactV2, error) {
	claim, err := g.Claims.InspectRunClaimAssociation(ctx, plan.ExecutionScopeDigest, run.ID)
	if err != nil {
		if !core.HasCategory(err, core.ErrorNotFound) {
			return nil, err
		}
		if plan.Claim.Mode == ports.RunClaimRequiredV2 || plan.Claim.OmissionPolicy == nil {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "required Claim association is missing")
		}
		policy, policyErr := g.inspectClaimOmissionPolicy(ctx, plan)
		if policyErr != nil || !policy.AllowMissingClaim {
			return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "missing Claim is not explicitly allowed")
		}
		return nil, nil
	}
	if err := claim.Validate(); err != nil {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "Claim association is structurally invalid")
	}
	if claim.RunID != run.ID || claim.RunIdentityDigest != plan.RunIdentityDigest || claim.ExecutionScopeDigest != plan.ExecutionScopeDigest || !ports.SameExecutionScopeV2(claim.ExecutionScope, run.Scope) || claim.LineagePlanDigest != run.Scope.Lineage.PlanDigest {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "Claim association does not bind the exact Run")
	}
	record, err := g.Evidence.InspectRecord(ctx, claim.Evidence)
	if err != nil || validateClaimAssociationRecordV2(claim, record) != nil {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "Claim association Evidence is missing or drifted")
	}
	return &claim, nil
}

func (g RunSettlementGatewayV2) inspectClaimOmissionPolicy(ctx context.Context, plan ports.RunSettlementPlanFactV2) (ports.RunSettlementPolicyFactV2, error) {
	if plan.Claim.OmissionPolicy == nil {
		return ports.RunSettlementPolicyFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunClaimUnverified, "Claim omission policy is absent")
	}
	policy, err := g.Policies.InspectRunSettlementPolicy(ctx, plan.Claim.OmissionPolicy.Ref)
	if err != nil {
		return ports.RunSettlementPolicyFactV2{}, err
	}
	if err := policy.ValidateCurrent(*plan.Claim.OmissionPolicy, plan, ports.RunRequirementClaimAssociation, g.Clock()); err != nil {
		return ports.RunSettlementPolicyFactV2{}, err
	}
	if policy.PolicyOwner == plan.Execution.Binding && !policy.AllowSelfPolicy {
		return ports.RunSettlementPolicyFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "Execution owner cannot self-authorize Claim omission without explicit governed policy")
	}
	set, err := g.Bindings.InspectBindingSet(ctx, policy.PolicyOwner.BindingSetID)
	if err != nil {
		return ports.RunSettlementPolicyFactV2{}, err
	}
	executionRequirement, found := findRequirementKindV2(plan, ports.RunRequirementExecutionTruth)
	if !found {
		return ports.RunSettlementPolicyFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "Claim omission requires the execution governance requirement")
	}
	executionRequirement.ID = ports.RunRequirementClaimAssociation
	executionRequirement.Owner = policy.PolicyOwner
	if err := g.validateRequirementBinding(ctx, set, executionRequirement, g.Clock()); err != nil {
		return ports.RunSettlementPolicyFactV2{}, err
	}
	authority, err := g.Authority.InspectDispatchAuthority(ctx, policy.PolicyAuthority.Ref)
	if err != nil {
		return ports.RunSettlementPolicyFactV2{}, err
	}
	if err := authority.ValidateCurrent(policy.PolicyAuthority, policy.ExecutionScope, policy.ActionScopeDigest, g.Clock()); err != nil {
		return ports.RunSettlementPolicyFactV2{}, err
	}
	return policy, nil
}

func inspectFrozenRunEffectRefsV2(ctx context.Context, reader control.RunEffectFactPortV2, partition control.RunEffectPartitionV2, root control.RunEffectIndexFactV2) ([]control.RunEffectRefV2, error) {
	refs := make([]control.RunEffectRefV2, 0, root.EffectCount)
	previousDigest := ports.EvidenceGenesisDigestV2
	after := uint64(0)
	for {
		page, err := reader.ListRunEffectSegmentsV2(ctx, partition, after, 128)
		if err != nil {
			return nil, err
		}
		if len(page.Segments) == 0 && page.NextNumber != 0 {
			return nil, core.NewError(core.ErrorInternal, core.ReasonRunEffectIndexConflict, "Run effect segment paging made no progress")
		}
		for _, segment := range page.Segments {
			if segment.PreviousDigest != previousDigest {
				return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectIndexConflict, "Run effect segment chain is discontinuous")
			}
			digest, err := segment.DigestV2()
			if err != nil {
				return nil, err
			}
			previousDigest = digest
			refs = append(refs, segment.Effects...)
			after = segment.Number
		}
		if page.NextNumber == 0 {
			break
		}
	}
	if uint64(len(refs)) != root.EffectCount || previousDigest != root.HeadSegmentDigest {
		return nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectIndexConflict, "Run effect root count or chain head drifted")
	}
	return refs, nil
}

func (g RunSettlementGatewayV2) inspectExecution(ctx context.Context, run core.AgentRunRecord, plan ports.RunSettlementPlanFactV2) (ports.ExecutionSettlementInspectionV2, error) {
	inspection, err := g.Execution.InspectRunExecutionV2(ctx, ports.RunExecutionInspectionRequestV2{RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest, ExpectedRunRevision: run.Revision, ExecutionScope: run.Scope, Subject: plan.Execution})
	if err != nil {
		return ports.ExecutionSettlementInspectionV2{}, err
	}
	if err := inspection.Validate(); err != nil {
		return ports.ExecutionSettlementInspectionV2{}, err
	}
	now := g.Clock()
	if inspection.RunID != run.ID || inspection.RunIdentityDigest != plan.RunIdentityDigest || inspection.RunRevision != run.Revision || !ports.SameExecutionScopeV2(inspection.ExecutionScope, run.Scope) || inspection.Subject != plan.Execution || !now.Before(time.Unix(0, inspection.ExpiresUnixNano)) {
		return ports.ExecutionSettlementInspectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonExecutionInspectionInvalid, "Execution inspection is stale or mismatched")
	}
	record, err := g.Evidence.InspectRecord(ctx, inspection.Evidence)
	requirement, found := findRequirementKindV2(plan, ports.RunRequirementExecutionTruth)
	correlation, correlationErr := ports.RunSettlementEvidenceCorrelationIDV2(plan.ID, plan.RunID, requirement.ID, requirement.SubjectDigest)
	causation, causationErr := ports.RunExecutionInspectionEvidenceCausationIDV2(inspection)
	ledgerScopeDigest, ledgerScopeErr := record.Candidate.LedgerScope.DigestV2()
	if err != nil || !found || correlationErr != nil || causationErr != nil || ledgerScopeErr != nil || control.ValidateEvidenceLedgerRecordV2(record) != nil || record.Ref != inspection.Evidence || record.Candidate.LedgerScope.Partition != ports.EvidencePartitionRun || record.Candidate.LedgerScope.RunID != plan.RunID || !ports.SameExecutionScopeV2(record.Candidate.ExecutionScope, plan.ExecutionScope) || record.Candidate.Producer != plan.Execution.Binding || record.Candidate.TrustClass != requirement.EvidenceTrust || record.Candidate.EventKind != requirement.EvidenceKind || record.Candidate.CorrelationID != correlation || len(record.Candidate.Causation) != 1 || record.Candidate.Causation[0].LedgerScopeDigest != ledgerScopeDigest || record.Candidate.Causation[0].EventID != causation || record.Candidate.Payload.Schema != requirement.Schema || record.Candidate.Payload.ContentDigest != inspection.PayloadDigest || record.Candidate.Payload.Revision != inspection.Revision || record.Candidate.SourceEpoch != inspection.SourceEpoch || record.Candidate.SourceSequence != inspection.SourceSequence {
		return ports.ExecutionSettlementInspectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonExecutionInspectionInvalid, "Execution inspection Evidence is missing or mismatched")
	}
	return inspection, nil
}

func (g RunSettlementGatewayV2) inspectParticipants(ctx context.Context, run core.AgentRunRecord, plan ports.RunSettlementPlanFactV2) ([]control.RunSettlementClosureParticipantV2, error) {
	participants := make([]control.RunSettlementClosureParticipantV2, 0, len(plan.Requirements))
	planRef, _ := plan.RefV2()
	for _, requirement := range plan.Requirements {
		if requirement.Kind == ports.RunRequirementExecutionTruth || requirement.Kind == ports.RunRequirementEffects {
			continue
		}
		requirementDigest, _ := requirement.DigestV2()
		request := ports.RunSettlementParticipantInspectRequestV2{RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, Plan: planRef, RequirementID: requirement.ID, RequirementDigest: requirementDigest, SubjectDigest: requirement.SubjectDigest, Owner: requirement.Owner}
		fact, err := g.Participants.InspectRunSettlementParticipant(ctx, request)
		if err != nil {
			return nil, err
		}
		if err := g.validateParticipant(ctx, plan, requirement, fact); err != nil {
			return nil, err
		}
		policy, err := g.inspectPolicy(ctx, plan, requirement)
		if err != nil {
			return nil, err
		}
		ref, _ := fact.RefV2()
		participants = append(participants, control.RunSettlementClosureParticipantV2{RequirementID: requirement.ID, RequirementDigest: requirementDigest, Participant: ref, ParticipantFact: fact, PolicyFact: policy})
	}
	sort.Slice(participants, func(i, j int) bool { return participants[i].RequirementID < participants[j].RequirementID })
	return participants, nil
}

func (g RunSettlementGatewayV2) validateParticipant(ctx context.Context, plan ports.RunSettlementPlanFactV2, requirement ports.RunSettlementRequirementV2, fact ports.RunSettlementParticipantFactV2) error {
	requirementDigest, _ := requirement.DigestV2()
	planRef, _ := plan.RefV2()
	now := g.Clock()
	if fact.RunID != plan.RunID || fact.RunIdentityDigest != plan.RunIdentityDigest || fact.ExecutionScopeDigest != plan.ExecutionScopeDigest || fact.Plan != planRef || fact.RequirementID != requirement.ID || fact.RequirementDigest != requirementDigest || fact.SubjectDigest != requirement.SubjectDigest || fact.Owner != requirement.Owner || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementParticipantStale, "participant does not bind the exact current requirement")
	}
	policy, err := g.inspectPolicy(ctx, plan, requirement)
	if err != nil {
		return err
	}
	if fact.Disposition == ports.RunSettlementOperationNotRequired {
		if fact.Policy == nil || *fact.Policy != requirement.Policy || !policy.AllowOperationNotRequired {
			return core.NewError(core.ErrorForbidden, core.ReasonRunSettlementRequirementInvalid, "operation_not_required is not explicitly authorized")
		}
	}
	for _, ref := range fact.Evidence {
		record, err := g.Evidence.InspectRecord(ctx, ref)
		if err != nil || record.Ref != ref || validateParticipantEvidenceV2(plan, requirement, fact, record) != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "participant Evidence is missing or drifted")
		}
	}
	return nil
}

func validateParticipantEvidenceV2(plan ports.RunSettlementPlanFactV2, requirement ports.RunSettlementRequirementV2, fact ports.RunSettlementParticipantFactV2, record ports.EvidenceLedgerRecordV2) error {
	if err := control.ValidateEvidenceLedgerRecordV2(record); err != nil {
		return err
	}
	candidate := record.Candidate
	correlation, err := ports.RunSettlementEvidenceCorrelationIDV2(plan.ID, plan.RunID, requirement.ID, requirement.SubjectDigest)
	if err != nil {
		return err
	}
	ledgerScopeDigest, scopeErr := candidate.LedgerScope.DigestV2()
	causationEvent, causationErr := ports.RunSettlementEvidenceCausationEventIDV2(plan.ID, plan.RunID, requirement.ID, fact.ID, fact.Revision)
	if scopeErr != nil || causationErr != nil || len(candidate.Causation) != 1 || candidate.Causation[0].LedgerScopeDigest != ledgerScopeDigest || candidate.Causation[0].EventID != causationEvent || candidate.LedgerScope.Partition != ports.EvidencePartitionRun || candidate.LedgerScope.RunID != plan.RunID || !ports.SameExecutionScopeV2(candidate.ExecutionScope, plan.ExecutionScope) || candidate.Producer != fact.Owner || candidate.TrustClass != requirement.EvidenceTrust || candidate.EventKind != requirement.EvidenceKind || candidate.CorrelationID != correlation || candidate.Payload.Schema != requirement.Schema || candidate.Payload.Revision != fact.Revision {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "participant Evidence governance coordinates differ")
	}
	expectedPayload := requirement.SubjectDigest
	if fact.Payload != nil {
		expectedPayload = fact.Payload.ContentDigest
		if candidate.Payload.Schema != fact.Payload.Schema {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "participant Evidence payload schema differs")
		}
	}
	if candidate.Payload.ContentDigest != expectedPayload {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "participant Evidence payload does not bind its subject")
	}
	if requirement.EvidenceTrust == ports.EvidenceTrustAuthoritativeFact {
		if candidate.OwnerFact == nil || candidate.OwnerFact.Owner != fact.Owner || candidate.OwnerFact.FactID != fact.ID || candidate.OwnerFact.Revision != fact.Revision || candidate.OwnerFact.PayloadDigest != expectedPayload {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "authoritative participant Evidence lacks its exact Owner Fact")
		}
	} else if candidate.OwnerFact != nil {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceConflict, "attestation Evidence cannot impersonate an authoritative Owner Fact")
	}
	return nil
}

func (g RunSettlementGatewayV2) validatePlanCurrent(ctx context.Context, run core.AgentRunRecord, plan ports.RunSettlementPlanFactV2) error {
	if run.Status != core.RunStopping || run.ID != plan.RunID || !ports.SameExecutionScopeV2(run.Scope, plan.ExecutionScope) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementPlanConflict, "settlement Plan requires exact stopping Run")
	}
	runIdentity, err := ports.RunIdentityDigestV2(run)
	if err != nil || runIdentity != plan.RunIdentityDigest {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementPlanConflict, "Run identity drifted from create-time Plan")
	}
	set, err := g.Bindings.InspectBindingSet(ctx, plan.BindingSet.ID)
	if err != nil {
		return err
	}
	setDigest, err := control.BindingSetDigestV2(set)
	setSemantic, semanticErr := control.BindingSetSemanticDigestV2(set)
	if err != nil || semanticErr != nil || set.ID != plan.BindingSet.ID || set.Revision < plan.BindingSet.Revision || setSemantic != plan.BindingSet.SemanticDigest || set.Revision == plan.BindingSet.Revision && setDigest != plan.BindingSet.Digest || set.State != control.BindingSetActive || !g.Clock().Before(time.Unix(0, set.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "Run settlement BindingSet is stale or drifted")
	}
	for _, requirement := range plan.Requirements {
		if err := g.validateRequirementBinding(ctx, set, requirement, g.Clock()); err != nil {
			return err
		}
	}
	return nil
}

func (g RunSettlementGatewayV2) validateRequirementBinding(ctx context.Context, set control.BindingSetFactV2, requirement ports.RunSettlementRequirementV2, now time.Time) error {
	for _, member := range set.Members {
		if member.ComponentID != requirement.Owner.ComponentID || member.ManifestDigest != requirement.Owner.ManifestDigest || member.ArtifactDigest != requirement.Owner.ArtifactDigest || set.ID != requirement.Owner.BindingSetID || set.Revision < requirement.Owner.BindingSetRevision {
			continue
		}
		granted := false
		for _, grant := range member.Grants {
			if grant.Capability == requirement.Owner.Capability && now.Before(time.Unix(0, grant.ExpiresUnixNano)) {
				granted = true
				break
			}
		}
		if !granted {
			break
		}
		binding, err := g.Bindings.InspectBinding(ctx, member.BindingID)
		if err != nil || binding.Revision != member.BindingRevision || binding.State != control.BindingBound || binding.ManifestDigest != member.ManifestDigest || !now.Before(time.Unix(0, binding.ExpiresUnixNano)) {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, "requirement owner Binding is stale")
		}
		for _, schema := range binding.Manifest.Schemas {
			if schema == requirement.Schema {
				return nil
			}
		}
		return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownCapability, "required settlement schema is not certified by its owner")
	}
	return core.NewError(core.ErrorCapabilityUnavailable, core.ReasonUnknownCapability, "required settlement owner capability is not granted")
}

func (g RunSettlementGatewayV2) inspectPolicy(ctx context.Context, plan ports.RunSettlementPlanFactV2, requirement ports.RunSettlementRequirementV2) (ports.RunSettlementPolicyFactV2, error) {
	policy, err := g.Policies.InspectRunSettlementPolicy(ctx, requirement.Policy.Ref)
	if err != nil {
		return ports.RunSettlementPolicyFactV2{}, err
	}
	if err := policy.ValidateCurrent(requirement.Policy, plan, requirement.ID, g.Clock()); err != nil {
		return ports.RunSettlementPolicyFactV2{}, err
	}
	if policy.PolicyOwner == requirement.Owner && !policy.AllowSelfPolicy {
		return ports.RunSettlementPolicyFactV2{}, core.NewError(core.ErrorForbidden, core.ReasonEffectAuthorizationMissing, "settlement participant cannot self-authorize terminalization without explicit governed policy")
	}
	set, err := g.Bindings.InspectBindingSet(ctx, policy.PolicyOwner.BindingSetID)
	if err != nil {
		return ports.RunSettlementPolicyFactV2{}, err
	}
	ownerRequirement := requirement
	ownerRequirement.Owner = policy.PolicyOwner
	if err := g.validateRequirementBinding(ctx, set, ownerRequirement, g.Clock()); err != nil {
		return ports.RunSettlementPolicyFactV2{}, err
	}
	authority, err := g.Authority.InspectDispatchAuthority(ctx, policy.PolicyAuthority.Ref)
	if err != nil {
		return ports.RunSettlementPolicyFactV2{}, err
	}
	if err := authority.ValidateCurrent(policy.PolicyAuthority, policy.ExecutionScope, policy.ActionScopeDigest, g.Clock()); err != nil {
		return ports.RunSettlementPolicyFactV2{}, err
	}
	return policy, nil
}

func (g RunSettlementGatewayV2) revalidateClosure(ctx context.Context, run core.AgentRunRecord, plan ports.RunSettlementPlanFactV2, closure control.RunSettlementClosureFactV2) error {
	partition := control.RunEffectPartitionV2{ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest}
	index, effectErr := g.Effects.InspectRunEffectIndexV2(ctx, partition)
	if effectErr != nil || index.State != control.RunEffectIndexFrozen || !sameRunEffectSetRefV2(index, closure.EffectSet) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunEffectSetFrozen, "frozen Effect set watermark drifted after Closure")
	}
	if _, effectErr = inspectFrozenRunEffectRefsV2(ctx, g.Effects, partition, index); effectErr != nil {
		return effectErr
	}
	currentExecution, err := g.inspectExecution(ctx, run, plan)
	if err != nil || !sameInspectionDigestV2(currentExecution, closure.Execution) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonExecutionInspectionInvalid, "Execution inspection drifted after Closure")
	}
	for _, link := range closure.Participants {
		requirement, found := findRequirementV2(plan, link.RequirementID)
		if !found {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementRequirementInvalid, "Closure references an unknown requirement")
		}
		request := ports.RunSettlementParticipantInspectRequestV2{RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest, ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, Plan: closure.Plan, RequirementID: requirement.ID, RequirementDigest: link.RequirementDigest, SubjectDigest: requirement.SubjectDigest, Owner: requirement.Owner}
		fact, err := g.Participants.InspectRunSettlementParticipant(ctx, request)
		if err != nil {
			return err
		}
		ref, _ := fact.RefV2()
		if ref != link.Participant || g.validateParticipant(ctx, plan, requirement, fact) != nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementParticipantStale, "participant drifted after Closure")
		}
	}
	return nil
}

func sameRunEffectSetRefV2(index control.RunEffectIndexFactV2, expected control.RunEffectSetRefV2) bool {
	digest, err := index.DigestV2()
	return err == nil && index.ID == expected.IndexID && index.Revision == expected.Revision && digest == expected.Digest && index.Watermark == expected.Watermark && index.SegmentCount == expected.SegmentCount && index.EffectCount == expected.EffectCount && index.HeadSegmentDigest == expected.HeadSegmentDigest
}

func (g RunSettlementGatewayV2) inspectCommitted(ctx context.Context, stopping core.AgentRunRecord, expected control.RunSettlementDecisionFactV2) (control.CommitRunCompletionResultV2, error) {
	decision, decisionErr := g.Facts.InspectRunSettlementDecisionV2(ctx, stopping.Scope, stopping.ID)
	run, runErr := g.Facts.InspectRun(ctx, stopping.Scope, stopping.ID)
	progress, progressErr := g.Facts.InspectRunTerminationProgressV2(ctx, stopping.Scope, stopping.ID)
	if decisionErr != nil || runErr != nil || progressErr != nil {
		return control.CommitRunCompletionResultV2{}, core.NewError(core.ErrorInternal, core.ReasonRunCompletionAtomicityBroken, "completion reply was lost and atomic facts cannot all be inspected")
	}
	if !sameDecisionDigestV2(decision, expected) || run.Status != core.RunTerminal || run.Outcome != decision.Outcome {
		return control.CommitRunCompletionResultV2{}, core.NewError(core.ErrorInternal, core.ReasonRunCompletionAtomicityBroken, "inspected Decision and terminal Run do not match")
	}
	decisionRef, _ := decision.RefV2()
	if progress.Decision != decisionRef {
		return control.CommitRunCompletionResultV2{}, core.NewError(core.ErrorInternal, core.ReasonRunCompletionAtomicityBroken, "inspected Progress does not bind the Decision")
	}
	return control.CommitRunCompletionResultV2{Run: run, Decision: decision, Progress: progress}, nil
}

func (g RunSettlementGatewayV2) validate() error {
	if g.Facts == nil || g.Effects == nil || g.Claims == nil || g.Evidence == nil || g.Execution == nil || g.Participants == nil || g.Policies == nil || g.Bindings == nil || g.Authority == nil || g.Clock == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Run settlement Gateway requires fact, effect, claim, evidence, execution, participant, policy, binding and clock ports")
	}
	if g.Clock().IsZero() {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Run settlement Gateway clock is zero")
	}
	return nil
}

func closureParticipantLinkV2(closure control.RunSettlementClosureFactV2, id ports.NamespacedNameV2) (control.RunSettlementClosureParticipantV2, bool) {
	for _, link := range closure.Participants {
		if link.RequirementID == id {
			return link, true
		}
	}
	return control.RunSettlementClosureParticipantV2{}, false
}

func findRequirementV2(plan ports.RunSettlementPlanFactV2, id ports.NamespacedNameV2) (ports.RunSettlementRequirementV2, bool) {
	for _, requirement := range plan.Requirements {
		if requirement.ID == id {
			return requirement, true
		}
	}
	return ports.RunSettlementRequirementV2{}, false
}

func findRequirementKindV2(plan ports.RunSettlementPlanFactV2, kind ports.NamespacedNameV2) (ports.RunSettlementRequirementV2, bool) {
	for _, requirement := range plan.Requirements {
		if requirement.Kind == kind {
			return requirement, true
		}
	}
	return ports.RunSettlementRequirementV2{}, false
}

func deriveRunOutcomeV2(truth ports.RunExecutionTruthV2, reconcile bool, plan ports.RunSettlementPlanFactV2, g RunSettlementGatewayV2, ctx context.Context) (core.ExecutionOutcome, error) {
	if reconcile {
		return core.OutcomeNeedsReconciliation, nil
	}
	switch truth {
	case ports.RunExecutionTerminalCompleted:
		return core.OutcomeCompleted, nil
	case ports.RunExecutionTerminalCancelled:
		return core.OutcomeCancelled, nil
	case ports.RunExecutionTerminalFailed:
		return core.OutcomeFailed, nil
	case ports.RunExecutionConfirmedLost:
		requirement, found := findRequirementKindV2(plan, ports.RunRequirementExecutionTruth)
		if !found {
			break
		}
		policy, err := g.inspectPolicy(ctx, plan, requirement)
		if err == nil && policy.AllowConfirmedLost && policy.UnknownMode == ports.RunUnknownTerminalizeIndeterminate {
			return core.OutcomeLost, nil
		}
	case ports.RunExecutionUnknown:
		requirement, found := findRequirementKindV2(plan, ports.RunRequirementExecutionTruth)
		if !found {
			break
		}
		policy, err := g.inspectPolicy(ctx, plan, requirement)
		if err == nil && policy.UnknownMode == ports.RunUnknownTerminalizeIndeterminate {
			return core.OutcomeIndeterminate, nil
		}
	}
	return "", core.NewError(core.ErrorPreconditionFailed, core.ReasonRunSettlementBlocked, "execution truth cannot derive an authorized terminal outcome")
}

func mustInspectionRefV2(f ports.ExecutionSettlementInspectionV2) ports.RunExecutionInspectionRefV2 {
	ref, _ := f.RefV2()
	return ref
}

func sameClosureDigestV2(left, right control.RunSettlementClosureFactV2) bool {
	ld, le := left.DigestV2()
	rd, re := right.DigestV2()
	return le == nil && re == nil && ld == rd
}

func sameDecisionDigestV2(left, right control.RunSettlementDecisionFactV2) bool {
	ld, le := left.DigestV2()
	rd, re := right.DigestV2()
	return le == nil && re == nil && ld == rd
}

func sameInspectionDigestV2(left, right ports.ExecutionSettlementInspectionV2) bool {
	ld, le := left.DigestV2()
	rd, re := right.DigestV2()
	return le == nil && re == nil && ld == rd
}

func sameProgressDigestV2(left, right control.RunTerminationProgressFactV2) bool {
	ld, le := left.DigestV2()
	rd, re := right.DigestV2()
	return le == nil && re == nil && ld == rd
}
