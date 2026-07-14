package kernel

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func (g RunSettlementGatewayV2) CreatePendingRunV3(ctx context.Context, request ports.CreatePendingRunRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	if g.Facts == nil || g.Effects == nil || g.PlanAdmissions == nil || g.Clock == nil {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Run lifecycle create requires Run, Effect index, Plan admission and clock owners")
	}
	if err := request.Validate(); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	bundleOwner, ok := g.Facts.(control.RunBundleFactPortV3)
	if !ok {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "certified Run bundle Fact owner is required")
	}
	expectedAssociation := request.Certification
	bundle, inspectErr := bundleOwner.InspectRunBundleV3(ctx, request.Run.Scope, request.Run.ID)
	if inspectErr == nil {
		if !sameRunLifecycleRunV3(bundle.Run, request.Run) || !sameRunLifecyclePlanV3(bundle.Plan, request.Plan) || bundle.Certification != expectedAssociation {
			return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "persisted certified Run bundle differs from request")
		}
		if err := g.validateHistoricalPlanCertificationV3(ctx, bundle.Certification, bundle.Run, bundle.Plan); err != nil {
			return ports.RunLifecycleEnvelopeV3{}, err
		}
	} else {
		if !core.HasCategory(inspectErr, core.ErrorNotFound) {
			return ports.RunLifecycleEnvelopeV3{}, inspectErr
		}
		if err := g.PlanAdmissions.ValidateRunSettlementPlanCertificationV3(ctx, request.Certification.Certification, request.Run, request.Plan); err != nil {
			return ports.RunLifecycleEnvelopeV3{}, err
		}
		var err error
		bundle, err = bundleOwner.CreateRunBundleV3(ctx, control.RunBundleCreateRequestV3{Run: request.Run, Plan: request.Plan, Certification: request.Certification})
		if err != nil {
			if !recoverableRunLifecycleWriteV3(err) {
				return ports.RunLifecycleEnvelopeV3{}, err
			}
			bundle, err = bundleOwner.InspectRunBundleV3(context.WithoutCancel(ctx), request.Run.Scope, request.Run.ID)
			if err != nil || !sameRunLifecycleRunV3(bundle.Run, request.Run) || !sameRunLifecyclePlanV3(bundle.Plan, request.Plan) || bundle.Certification != expectedAssociation {
				return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorIndeterminate, core.ReasonRunSettlementPlanConflict, "cannot prove certified pending Run bundle create")
			}
		} else if !sameRunLifecycleRunV3(bundle.Run, request.Run) || !sameRunLifecyclePlanV3(bundle.Plan, request.Plan) || bundle.Certification != expectedAssociation {
			return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "certified Run bundle owner returned mismatched content")
		}
	}
	index := control.RunEffectIndexFactV2{ContractVersion: ports.RunSettlementContractVersionV2, ID: request.EffectIndexID, Revision: 1, RunID: request.Run.ID, RunIdentityDigest: request.Plan.RunIdentityDigest, ExecutionScope: request.Run.Scope, ExecutionScopeDigest: request.Plan.ExecutionScopeDigest, State: control.RunEffectIndexOpen, HeadSegmentDigest: ports.EvidenceGenesisDigestV2, Watermark: 1, CreatedUnixNano: g.Clock().UnixNano()}
	storedIndex, err := g.Effects.CreateRunEffectIndexV2(ctx, index)
	if err != nil {
		if !recoverableRunLifecycleWriteV3(err) {
			return ports.RunLifecycleEnvelopeV3{}, err
		}
		partition := index.PartitionV2()
		storedIndex, err = g.Effects.InspectRunEffectIndexV2(context.WithoutCancel(ctx), partition)
		if err != nil || !sameRunLifecycleIndexV3(storedIndex, index) {
			return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorIndeterminate, core.ReasonRunEffectIndexConflict, "cannot prove empty Run Effect index create")
		}
	}
	return runLifecycleEnvelopeV3(bundle.Run, bundle.Plan, bundle.Certification, storedIndex, nil, nil, nil, nil)
}

func (g RunSettlementGatewayV2) BeginStopRunV3(ctx context.Context, request ports.BeginStopRunRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	if err := request.Validate(); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	bundle, err := g.preflightCertifiedRunMutationV3(ctx, request.ExecutionScope, request.RunID)
	if err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	if bundle.Run.Status != core.RunRunning || bundle.Run.Revision != request.ExpectedRunRevision {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "BeginStop requires the exact certified running Run")
	}
	run, err := g.BeginStopRunV2(ctx, StopAndSettleRunRequestV2{Scope: request.ExecutionScope, RunID: request.RunID, ExpectedRunRevision: request.ExpectedRunRevision})
	if err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	return g.inspectRunLifecycleV3(ctx, run)
}

func (g RunSettlementGatewayV2) StopAndSettleRunV3(ctx context.Context, request ports.BeginStopRunRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	if err := request.Validate(); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	bundle, err := g.preflightCertifiedRunMutationV3(ctx, request.ExecutionScope, request.RunID)
	if err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	if bundle.Run.Status == core.RunTerminal {
		// Terminal replays are recovered from immutable Decision/Progress below.
	} else if (bundle.Run.Status != core.RunRunning && bundle.Run.Status != core.RunStopping) || bundle.Run.Revision != request.ExpectedRunRevision {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "settlement requires the exact certified running or stopping Run")
	}
	result, err := g.StopAndSettleRunV2(ctx, StopAndSettleRunRequestV2{Scope: request.ExecutionScope, RunID: request.RunID, ExpectedRunRevision: request.ExpectedRunRevision})
	if err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	return g.inspectRunLifecycleV3(ctx, result.Run)
}

func (g RunSettlementGatewayV2) InspectRunLifecycleV3(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (ports.RunLifecycleEnvelopeV3, error) {
	if err := (ports.RunTerminationRequestV3{ExecutionScope: scope, RunID: runID}).Validate(); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	if err := g.validateRunLifecycleInspectDependenciesV3(); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	run, err := g.Facts.InspectRun(ctx, scope, runID)
	if err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	return g.inspectRunLifecycleV3(ctx, run)
}

func (g RunSettlementGatewayV2) ReconcileRunTerminationV3(ctx context.Context, request ports.RunTerminationRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	if err := request.Validate(); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	bundle, err := g.preflightCertifiedRunMutationV3(ctx, request.ExecutionScope, request.RunID)
	if err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	if bundle.Run.Status != core.RunTerminal {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonTerminationProgressConflict, "termination reconciliation requires a certified terminal Run")
	}
	if _, err := g.ReconcileTerminationProgressV2(ctx, request.ExecutionScope, request.RunID); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	if _, err := g.BuildTerminationReportV2(ctx, request.ExecutionScope, request.RunID); err != nil && !core.HasReason(err, core.ReasonTerminationReportIncomplete) {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	return g.InspectRunTerminationV3(ctx, request)
}

func (g RunSettlementGatewayV2) preflightCertifiedRunMutationV3(ctx context.Context, scope core.ExecutionScope, runID core.AgentRunID) (control.RunBundleV3, error) {
	if g.Facts == nil || g.PlanAdmissions == nil {
		return control.RunBundleV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "certified Run mutation requires bundle and Plan admission owners")
	}
	owner, ok := g.Facts.(control.RunBundleFactPortV3)
	if !ok {
		return control.RunBundleV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "certified Run bundle Fact owner is required")
	}
	bundle, err := owner.InspectRunBundleV3(ctx, scope, runID)
	if err != nil {
		return control.RunBundleV3{}, err
	}
	if err := g.validateHistoricalPlanCertificationV3(ctx, bundle.Certification, bundle.Run, bundle.Plan); err != nil {
		return control.RunBundleV3{}, err
	}
	return bundle, nil
}

func (g RunSettlementGatewayV2) InspectRunTerminationV3(ctx context.Context, request ports.RunTerminationRequestV3) (ports.RunLifecycleEnvelopeV3, error) {
	if err := request.Validate(); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	if err := g.validateRunLifecycleInspectDependenciesV3(); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	run, err := g.Facts.InspectRun(ctx, request.ExecutionScope, request.RunID)
	if err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	if run.Status != core.RunTerminal {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonTerminationProgressConflict, "termination inspection requires a terminal Run")
	}
	return g.inspectRunLifecycleV3(ctx, run)
}

func (g RunSettlementGatewayV2) validateRunLifecycleInspectDependenciesV3() error {
	if g.Facts == nil || g.Effects == nil || g.PlanAdmissions == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Run lifecycle inspection requires certified bundle, Effect index and Plan admission readers")
	}
	if _, ok := g.Facts.(control.RunBundleFactPortV3); !ok {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "certified Run bundle Fact reader is required")
	}
	return nil
}

func (g RunSettlementGatewayV2) inspectRunLifecycleV3(ctx context.Context, run core.AgentRunRecord) (ports.RunLifecycleEnvelopeV3, error) {
	bundleOwner, ok := g.Facts.(control.RunBundleFactPortV3)
	if !ok {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "certified Run bundle Fact owner is required")
	}
	bundle, err := bundleOwner.InspectRunBundleV3(ctx, run.Scope, run.ID)
	if err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	if !sameRunLifecycleRunV3(bundle.Run, run) {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonRunConflict, "Run lifecycle and certified bundle differ")
	}
	plan := bundle.Plan
	if g.PlanAdmissions == nil {
		return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Run lifecycle inspection requires Plan admission reader")
	}
	if err := g.validateHistoricalPlanCertificationV3(ctx, bundle.Certification, run, plan); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	partition := control.RunEffectPartitionV2{ExecutionScope: run.Scope, ExecutionScopeDigest: plan.ExecutionScopeDigest, RunID: run.ID, RunIdentityDigest: plan.RunIdentityDigest}
	index, err := g.Effects.InspectRunEffectIndexV2(ctx, partition)
	if err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	var closure *control.RunSettlementClosureFactV2
	if current, inspectErr := g.Facts.InspectCurrentRunSettlementClosureV2(ctx, run.Scope, run.ID); inspectErr == nil {
		closure = &current.Closure
	} else if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return ports.RunLifecycleEnvelopeV3{}, inspectErr
	}
	var decision *control.RunSettlementDecisionFactV2
	if current, inspectErr := g.Facts.InspectRunSettlementDecisionV2(ctx, run.Scope, run.ID); inspectErr == nil {
		decision = &current
	} else if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return ports.RunLifecycleEnvelopeV3{}, inspectErr
	}
	var progress *control.RunTerminationProgressFactV2
	if current, inspectErr := g.Facts.InspectRunTerminationProgressV2(ctx, run.Scope, run.ID); inspectErr == nil {
		progress = &current
	} else if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return ports.RunLifecycleEnvelopeV3{}, inspectErr
	}
	var report *control.RunTerminationReportV2
	if current, inspectErr := g.Facts.InspectRunTerminationReportV2(ctx, run.Scope, run.ID); inspectErr == nil {
		report = &current
	} else if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return ports.RunLifecycleEnvelopeV3{}, inspectErr
	}
	return runLifecycleEnvelopeV3(run, plan, bundle.Certification, index, closure, decision, progress, report)
}

func (g RunSettlementGatewayV2) validateHistoricalPlanCertificationV3(ctx context.Context, association ports.RunSettlementPlanCertificationAssociationV3, run core.AgentRunRecord, plan ports.RunSettlementPlanFactV2) error {
	if err := association.Validate(); err != nil {
		return err
	}
	fact, err := g.PlanAdmissions.InspectCertifiedRunSettlementPlanV3(ctx, run.Scope, run.ID)
	if err != nil {
		return err
	}
	if err := fact.Validate(); err != nil {
		return err
	}
	ref, err := fact.RefV3()
	expectedAssociation, associationErr := ports.NewRunSettlementPlanCertificationAssociationV3(run, plan, ref)
	if err != nil || associationErr != nil || expectedAssociation != association || ref != association.Certification || fact.RunID != run.ID || fact.RunIdentityDigest != association.RunIdentityDigest || fact.ExecutionScopeDigest != association.ExecutionScopeDigest || fact.Plan != association.Plan {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "historical Plan certification does not match the persisted Run bundle")
	}
	planRef, _ := plan.RefV2()
	if association.RunID != run.ID || association.Plan != planRef {
		return core.NewError(core.ErrorConflict, core.ReasonRunSettlementPlanConflict, "historical Plan certification association drifted")
	}
	return nil
}

func runLifecycleEnvelopeV3(run core.AgentRunRecord, plan ports.RunSettlementPlanFactV2, certification ports.RunSettlementPlanCertificationAssociationV3, index control.RunEffectIndexFactV2, closure *control.RunSettlementClosureFactV2, decision *control.RunSettlementDecisionFactV2, progress *control.RunTerminationProgressFactV2, report *control.RunTerminationReportV2) (ports.RunLifecycleEnvelopeV3, error) {
	planRef, err := plan.RefV2()
	if err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	indexDigest, err := index.DigestV2()
	if err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	envelope := ports.RunLifecycleEnvelopeV3{
		ContractVersion: ports.RunLifecycleContractVersionV3,
		Run:             run,
		Plan: ports.RunSettlementPlanLifecycleRefV3{
			RunSettlementPlanRefV2: planRef,
			RunID:                  plan.RunID,
			RunIdentityDigest:      plan.RunIdentityDigest,
			ExecutionScopeDigest:   plan.ExecutionScopeDigest,
		},
		Certification: certification,
		EffectIndex: ports.RunEffectIndexRefV3{
			ID:                   index.ID,
			Revision:             index.Revision,
			Digest:               indexDigest,
			RunID:                index.RunID,
			RunIdentityDigest:    index.RunIdentityDigest,
			ExecutionScopeDigest: index.ExecutionScopeDigest,
			Watermark:            index.Watermark,
			SegmentCount:         index.SegmentCount,
			EffectCount:          index.EffectCount,
			HeadDigest:           index.HeadSegmentDigest,
			Frozen:               index.State == control.RunEffectIndexFrozen,
		},
	}
	switch run.Status {
	case core.RunPending:
		envelope.Phase = ports.RunLifecyclePendingPreparedV3
	case core.RunRunning:
		envelope.Phase = ports.RunLifecycleRunningV3
	case core.RunStopping:
		envelope.Phase = ports.RunLifecycleStoppingV3
	case core.RunTerminal:
		envelope.Phase = ports.RunLifecycleTerminalCleanupV3
	}
	if closure != nil {
		ref, refErr := closure.RefV2()
		if refErr != nil {
			return ports.RunLifecycleEnvelopeV3{}, refErr
		}
		envelope.Closure = &ports.RunSettlementClosureRefV3{ID: ref.ID, RunID: closure.RunID, RunIdentityDigest: closure.RunIdentityDigest, ExecutionScopeDigest: closure.ExecutionScopeDigest, Attempt: ref.Attempt, Revision: ref.Revision, Digest: ref.Digest}
	}
	if decision != nil {
		if envelope.Closure == nil {
			return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonRunCompletionConflict, "Decision exists without its current Closure")
		}
		ref, refErr := decision.RefV2()
		if refErr != nil {
			return ports.RunLifecycleEnvelopeV3{}, refErr
		}
		if decision.Closure.ID != envelope.Closure.ID || decision.Closure.Attempt != envelope.Closure.Attempt || decision.Closure.Revision != envelope.Closure.Revision || decision.Closure.Digest != envelope.Closure.Digest {
			return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonRunCompletionConflict, "Decision binds a non-current Closure attempt")
		}
		envelope.Decision = &ports.RunSettlementDecisionRefV3{ID: ref.ID, RunID: decision.RunID, RunIdentityDigest: decision.RunIdentityDigest, ExecutionScopeDigest: decision.ExecutionScopeDigest, Revision: ref.Revision, Digest: ref.Digest, Outcome: decision.Outcome, Closure: *envelope.Closure}
	}
	if progress != nil {
		if envelope.Decision == nil {
			return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonTerminationProgressConflict, "Progress exists without its terminal Decision")
		}
		ref, refErr := progress.RefV2()
		if refErr != nil {
			return ports.RunLifecycleEnvelopeV3{}, refErr
		}
		var unresolved uint32
		for _, item := range progress.Items {
			if item.Disposition == ports.RunSettlementUnknown {
				unresolved++
			}
		}
		runIdentity, identityErr := ports.RunIdentityDigestV2(run)
		if identityErr != nil {
			return ports.RunLifecycleEnvelopeV3{}, identityErr
		}
		decisionRef, decisionRefErr := decision.RefV2()
		if decisionRefErr != nil || progress.Decision != decisionRef {
			return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonTerminationProgressConflict, "Progress binds a different terminal Decision")
		}
		envelope.Progress = &ports.RunTerminationProgressRefV3{ID: ref.ID, RunID: progress.RunID, RunIdentityDigest: runIdentity, ExecutionScopeDigest: progress.ExecutionScopeDigest, Revision: ref.Revision, Digest: ref.Digest, UnresolvedCount: unresolved, Decision: *envelope.Decision}
	}
	if report != nil {
		if envelope.Decision == nil || envelope.Progress == nil {
			return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonTerminationReportIncomplete, "Report exists without exact Decision and Progress")
		}
		digest, digestErr := report.DigestV2()
		if digestErr != nil {
			return ports.RunLifecycleEnvelopeV3{}, digestErr
		}
		decisionRef, decisionRefErr := decision.RefV2()
		progressRef, progressRefErr := progress.RefV2()
		if decisionRefErr != nil || progressRefErr != nil || report.Decision != decisionRef || report.Progress != progressRef {
			return ports.RunLifecycleEnvelopeV3{}, core.NewError(core.ErrorConflict, core.ReasonTerminationReportIncomplete, "Report binds stale Decision or Progress")
		}
		envelope.Report = &ports.RunTerminationReportRefV3{ID: report.ID, RunID: report.RunID, RunIdentityDigest: report.RunIdentityDigest, ExecutionScopeDigest: report.ExecutionScopeDigest, Revision: report.Revision, Digest: digest, Decision: *envelope.Decision, Progress: *envelope.Progress}
		envelope.Phase = ports.RunLifecycleTerminationClosedV3
	}
	if err := envelope.Validate(); err != nil {
		return ports.RunLifecycleEnvelopeV3{}, err
	}
	return envelope, nil
}

func recoverableRunLifecycleWriteV3(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate) || core.HasCategory(err, core.ErrorConflict)
}

func sameRunLifecycleRunV3(left, right core.AgentRunRecord) bool {
	leftDigest, leftErr := ports.RunIdentityDigestV2(left)
	rightDigest, rightErr := ports.RunIdentityDigestV2(right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest && left.Status == right.Status && left.Revision == right.Revision && left.StartedAt.Equal(right.StartedAt) && left.EndedAt.Equal(right.EndedAt) && left.Outcome == right.Outcome
}

func sameRunLifecyclePlanV3(left, right ports.RunSettlementPlanFactV2) bool {
	leftDigest, leftErr := left.DigestV2()
	rightDigest, rightErr := right.DigestV2()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func sameRunLifecycleIndexV3(left, right control.RunEffectIndexFactV2) bool {
	leftDigest, leftErr := left.DigestV2()
	rightDigest, rightErr := right.DigestV2()
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}
