package kernel

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type CheckpointGovernanceGatewayV2 struct {
	Facts        ports.CheckpointFactPortV2
	Policies     ports.CheckpointBarrierPolicyCurrentReaderV2
	Runs         ports.CheckpointRunCurrentReaderV2
	Inputs       ports.CheckpointAttemptInputsCurrentReaderV2
	Effects      ports.CheckpointEffectInventoryCurrentReaderV2
	Participants ports.CheckpointParticipantSetCurrentReaderV2
	Closures     ports.CheckpointParticipantClosureCurrentReaderV2
	Branches     ports.CheckpointParticipantBranchGuardReaderV2
	Diagnostics  ports.CheckpointAttemptDiagnosticsFinalizationOwnerPortV2
	Residuals    ports.CheckpointAttemptResidualsFinalizationOwnerPortV2
	Manifests    ports.CheckpointManifestSealReaderV2
	Clock        func() time.Time
}

func (g CheckpointGovernanceGatewayV2) CreateCheckpointAttemptV2(ctx context.Context, request ports.CreateCheckpointAttemptRequestV2) (ports.CheckpointAttemptBarrierBundleV2, error) {
	if err := request.Validate(); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Fact Owner"); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Policies, "checkpoint Barrier Policy Reader"); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Runs, "checkpoint Run current Reader"); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	now, err := g.nowV2(time.Time{})
	if err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	policy, err := g.Policies.InspectCheckpointBarrierPolicyCurrentV2(ctx, request.BarrierPolicy)
	if err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	run, err := g.Runs.InspectCheckpointRunCurrentV2(ctx, request.Scope, request.RunID)
	if err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	if err := validateCheckpointRunCurrentV2(request, run, now); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	candidate, err := control.BuildCheckpointAttemptBundleV2(request, policy, now)
	if err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Inputs, "checkpoint Attempt Inputs current Reader"); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	inputs, err := g.Inputs.InspectCheckpointAttemptInputsCurrentV2(ctx, candidate.Attempt.RefV2())
	if err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	if err := validateCheckpointAttemptInputIdentityV2(candidate.Attempt, inputs, now); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	expected, err := control.BuildCheckpointAttemptBundleBoundedV2(request, policy, minCheckpointNanosV2(inputs.ExpiresUnixNano, run.ExpiresUnixNano), now)
	if err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	fresh, err := g.nowV2(now)
	if err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	inputs2, err := g.Inputs.InspectCheckpointAttemptInputsCurrentV2(ctx, expected.Attempt.RefV2())
	if err != nil || inputs2.ProjectionDigest != inputs.ProjectionDigest {
		if err != nil {
			return ports.CheckpointAttemptBarrierBundleV2{}, err
		}
		return ports.CheckpointAttemptBarrierBundleV2{}, checkpointGatewayConflictV2("checkpoint Attempt inputs changed before create")
	}
	run2, err := g.Runs.InspectCheckpointRunCurrentV2(ctx, request.Scope, request.RunID)
	if err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	if err := validateCheckpointRunCurrentV2(request, run2, fresh); err != nil || run2.ProjectionDigest != run.ProjectionDigest {
		if err != nil {
			return ports.CheckpointAttemptBarrierBundleV2{}, err
		}
		return ports.CheckpointAttemptBarrierBundleV2{}, checkpointGatewayConflictV2("checkpoint Run changed before create")
	}
	if err := validateCheckpointAttemptInputsV2(expected.Attempt, inputs2, fresh); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	policy2, err := g.Policies.InspectCheckpointBarrierPolicyCurrentV2(ctx, request.BarrierPolicy)
	if err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	if err := policy2.Validate(fresh); err != nil || policy2.ProjectionDigest != policy.ProjectionDigest {
		if err != nil {
			return ports.CheckpointAttemptBarrierBundleV2{}, err
		}
		return ports.CheckpointAttemptBarrierBundleV2{}, checkpointGatewayConflictV2("checkpoint Barrier Policy changed before create")
	}
	created, err := g.Facts.CreateCheckpointAttemptBundleV2(ctx, expected)
	recoveredFromUnknown := false
	if err != nil {
		if !checkpointRecoverableV2(err) {
			return ports.CheckpointAttemptBarrierBundleV2{}, err
		}
		created, err = g.Facts.InspectCheckpointAttemptBundleV2(context.WithoutCancel(ctx), ports.InspectCheckpointAttemptRequestV2{TenantID: expected.Attempt.TenantID, AttemptID: expected.Attempt.ID})
		if err != nil {
			return ports.CheckpointAttemptBarrierBundleV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "checkpoint Attempt create outcome cannot be inspected")
		}
		recoveredFromUnknown = true
	}
	if err := exactCheckpointBundleV2(created, expected); err != nil {
		if recoveredFromUnknown {
			return g.recoverProgressedCheckpointCreateV2(ctx, expected, created)
		}
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	return created, nil
}

func (g CheckpointGovernanceGatewayV2) recoverProgressedCheckpointCreateV2(ctx context.Context, expected, current ports.CheckpointAttemptBarrierBundleV2) (ports.CheckpointAttemptBarrierBundleV2, error) {
	if current.Validate() != nil || current.Attempt.Revision < expected.Attempt.Revision || current.Barrier.Revision < expected.Barrier.Revision || !sameCheckpointAttemptImmutableV2(expected.Attempt, current.Attempt) || !sameCheckpointBarrierImmutableV2(expected.Barrier, current.Barrier) {
		return ports.CheckpointAttemptBarrierBundleV2{}, checkpointGatewayConflictV2("checkpoint Attempt create recovery found an ABA or immutable identity drift")
	}
	historicalAttempt, err := g.Facts.InspectCheckpointAttemptHistoricalV2(context.WithoutCancel(ctx), expected.Attempt.RefV2())
	if err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "checkpoint Attempt initial history cannot be inspected")
	}
	historicalBarrier, err := g.Facts.InspectCheckpointBarrierHistoricalV2(context.WithoutCancel(ctx), expected.Barrier.RefV2())
	if err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "checkpoint Barrier initial history cannot be inspected")
	}
	historical := ports.CheckpointAttemptBarrierBundleV2{Attempt: historicalAttempt, Barrier: historicalBarrier}
	if err := exactCheckpointBundleV2(historical, expected); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, checkpointGatewayConflictV2("checkpoint Attempt progressed recovery changed immutable initial history")
	}
	lineage, err := g.Facts.InspectCheckpointAttemptLineageV2(context.WithoutCancel(ctx), ports.InspectCheckpointAttemptLineageRequestV2{TenantID: expected.Attempt.TenantID, AttemptID: expected.Attempt.ID, FromRevision: expected.Attempt.Revision, ToRevision: current.Attempt.Revision})
	if err != nil || len(lineage.Attempts) == 0 || lineage.Attempts[0].RefV2() != expected.Attempt.RefV2() || lineage.Attempts[len(lineage.Attempts)-1].RefV2() != current.Attempt.RefV2() {
		return ports.CheckpointAttemptBarrierBundleV2{}, checkpointGatewayConflictV2("checkpoint Attempt progressed recovery lacks exact transition lineage")
	}
	for index := 1; index < len(lineage.Attempts); index++ {
		if err := control.ValidateCheckpointAttemptTransitionV2(lineage.Attempts[index-1], lineage.Attempts[index]); err != nil {
			return ports.CheckpointAttemptBarrierBundleV2{}, checkpointGatewayConflictV2("checkpoint Attempt progressed recovery contains an illegal transition")
		}
	}
	if len(lineage.Barriers) == 0 || lineage.Barriers[0].RefV2() != expected.Barrier.RefV2() || lineage.Barriers[len(lineage.Barriers)-1].RefV2() != current.Barrier.RefV2() {
		return ports.CheckpointAttemptBarrierBundleV2{}, checkpointGatewayConflictV2("checkpoint Attempt progressed recovery lacks exact Barrier lineage")
	}
	if len(lineage.Barriers) > 1 {
		if len(lineage.Barriers) != 2 || control.ValidateCheckpointBarrierTransitionV2(lineage.Barriers[0], lineage.Barriers[1]) != nil {
			return ports.CheckpointAttemptBarrierBundleV2{}, checkpointGatewayConflictV2("checkpoint Attempt progressed recovery contains an illegal Barrier transition")
		}
	}
	return historical, nil
}

func sameCheckpointAttemptImmutableV2(left, right ports.CheckpointAttemptFactV2) bool {
	return left.ContractVersion == right.ContractVersion && left.TenantID == right.TenantID && left.ID == right.ID && left.ScopeDigest == right.ScopeDigest && left.RunID == right.RunID && left.RunStableIdentityDigest == right.RunStableIdentityDigest && left.Generation == right.Generation && left.GenerationBinding == right.GenerationBinding && left.BindingSet == right.BindingSet && left.ParticipantSetCertification == right.ParticipantSetCertification && left.Workflow == right.Workflow && left.BarrierPolicy == right.BarrierPolicy && left.BarrierPolicySemanticDigest == right.BarrierPolicySemanticDigest && left.FrozenUnknownAtDeadlineMode == right.FrozenUnknownAtDeadlineMode && left.FrozenAllowNotAppliedAbort == right.FrozenAllowNotAppliedAbort && left.ReconciliationDeadlineUnixNano == right.ReconciliationDeadlineUnixNano && left.CreatedUnixNano == right.CreatedUnixNano
}

func sameCheckpointBarrierImmutableV2(left, right ports.CheckpointBarrierLeaseFactV2) bool {
	return left.ContractVersion == right.ContractVersion && left.TenantID == right.TenantID && left.ID == right.ID && left.AttemptID == right.AttemptID && left.ScopeDigest == right.ScopeDigest && left.RunID == right.RunID && left.RunStableIdentityDigest == right.RunStableIdentityDigest && left.Policy == right.Policy && left.AcquiredDispatchWatermark == right.AcquiredDispatchWatermark && left.AcquiredUnixNano == right.AcquiredUnixNano && left.ExpiresUnixNano == right.ExpiresUnixNano
}

func (g CheckpointGovernanceGatewayV2) InspectCheckpointAttemptV2(ctx context.Context, request ports.InspectCheckpointAttemptRequestV2) (ports.CheckpointAttemptBarrierBundleV2, error) {
	if err := request.Validate(); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Fact Owner"); err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	bundle, err := g.Facts.InspectCheckpointAttemptBundleV2(ctx, request)
	if err != nil {
		return ports.CheckpointAttemptBarrierBundleV2{}, err
	}
	return bundle, bundle.Validate()
}

func (g CheckpointGovernanceGatewayV2) InspectCheckpointBarrierHistoricalV2(ctx context.Context, ref ports.CheckpointBarrierLeaseRefV2) (ports.CheckpointBarrierLeaseFactV2, error) {
	if err := ref.Validate(); err != nil {
		return ports.CheckpointBarrierLeaseFactV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Fact Owner"); err != nil {
		return ports.CheckpointBarrierLeaseFactV2{}, err
	}
	fact, err := g.Facts.InspectCheckpointBarrierHistoricalV2(ctx, ref)
	if err != nil {
		return ports.CheckpointBarrierLeaseFactV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.CheckpointBarrierLeaseFactV2{}, err
	}
	if fact.RefV2() != ref {
		return ports.CheckpointBarrierLeaseFactV2{}, checkpointGatewayConflictV2("historical checkpoint Barrier ref drifted")
	}
	return fact, nil
}

func (g CheckpointGovernanceGatewayV2) InspectCheckpointAttemptHistoricalV2(ctx context.Context, ref ports.CheckpointAttemptRefV2) (ports.CheckpointAttemptFactV2, error) {
	if err := ref.Validate(); err != nil {
		return ports.CheckpointAttemptFactV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Fact Owner"); err != nil {
		return ports.CheckpointAttemptFactV2{}, err
	}
	fact, err := g.Facts.InspectCheckpointAttemptHistoricalV2(ctx, ref)
	if err != nil {
		return ports.CheckpointAttemptFactV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.CheckpointAttemptFactV2{}, err
	}
	if fact.RefV2() != ref {
		return ports.CheckpointAttemptFactV2{}, checkpointGatewayConflictV2("checkpoint Attempt historical ref drifted")
	}
	return fact, nil
}

func (g CheckpointGovernanceGatewayV2) InspectCheckpointBarrierCurrentV2(ctx context.Context, ref ports.CheckpointBarrierLeaseRefV2) (ports.CheckpointBarrierCurrentProjectionV2, error) {
	fact, err := g.InspectCheckpointBarrierHistoricalV2(ctx, ref)
	if err != nil {
		return ports.CheckpointBarrierCurrentProjectionV2{}, err
	}
	now, err := g.nowV2(time.Time{})
	if err != nil {
		return ports.CheckpointBarrierCurrentProjectionV2{}, err
	}
	projection := ports.CheckpointBarrierCurrentProjectionV2{Ref: fact.RefV2(), State: fact.State, Current: fact.State == ports.CheckpointBarrierActiveV2, CheckedUnixNano: now.UnixNano()}
	return ports.SealCheckpointBarrierCurrentProjectionV2(projection, now)
}

func (g CheckpointGovernanceGatewayV2) InspectCheckpointEvidenceAttemptCurrentV1(ctx context.Context, attempt ports.CheckpointAttemptRefV2, barrier ports.CheckpointBarrierLeaseRefV2, cut ports.EffectCutRefV2) (ports.CheckpointEvidenceAttemptCurrentProjectionV1, error) {
	if attempt.Validate() != nil || barrier.Validate() != nil || cut.Validate() != nil {
		return ports.CheckpointEvidenceAttemptCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCheckpointInconsistent, "checkpoint Evidence current request is incomplete")
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Fact Owner"); err != nil {
		return ports.CheckpointEvidenceAttemptCurrentProjectionV1{}, err
	}
	bundle, err := g.Facts.InspectCheckpointAttemptBundleV2(ctx, ports.InspectCheckpointAttemptRequestV2{TenantID: attempt.TenantID, AttemptID: attempt.ID})
	if err != nil {
		return ports.CheckpointEvidenceAttemptCurrentProjectionV1{}, err
	}
	if err := bundle.Validate(); err != nil {
		return ports.CheckpointEvidenceAttemptCurrentProjectionV1{}, err
	}
	if bundle.Attempt.RefV2() != attempt || bundle.Barrier.RefV2() != barrier || bundle.Attempt.EffectCut == nil || *bundle.Attempt.EffectCut != cut || bundle.Barrier.State != ports.CheckpointBarrierActiveV2 {
		return ports.CheckpointEvidenceAttemptCurrentProjectionV1{}, checkpointGatewayConflictV2("checkpoint Evidence Attempt, Barrier or Effect Cut is no longer exact current")
	}
	cutFact, err := g.Facts.InspectCheckpointEffectCutV2(ctx, cut)
	if err != nil {
		return ports.CheckpointEvidenceAttemptCurrentProjectionV1{}, err
	}
	if err := cutFact.Validate(); err != nil {
		return ports.CheckpointEvidenceAttemptCurrentProjectionV1{}, err
	}
	if cutFact.Ref != cut || cutFact.Barrier != barrier {
		return ports.CheckpointEvidenceAttemptCurrentProjectionV1{}, checkpointGatewayConflictV2("checkpoint Evidence Effect Cut history drifted")
	}
	now, err := g.nowV2(time.Time{})
	if err != nil {
		return ports.CheckpointEvidenceAttemptCurrentProjectionV1{}, err
	}
	projection := ports.CheckpointEvidenceAttemptCurrentProjectionV1{Attempt: attempt, Barrier: barrier, EffectCut: cut, Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: barrier.ExpiresUnixNano}
	return ports.SealCheckpointEvidenceAttemptCurrentProjectionV1(projection, now)
}

func (g CheckpointGovernanceGatewayV2) FreezeCheckpointEffectCutV2(ctx context.Context, request ports.FreezeCheckpointEffectCutRequestV2) (ports.CheckpointEffectCutBundleV2, error) {
	if err := request.Validate(); err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	if err := g.requireMutationDependenciesV2(false, false); err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Effects, "checkpoint Effect inventory current Reader"); err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	current, err := g.Facts.InspectCheckpointAttemptBundleV2(ctx, ports.InspectCheckpointAttemptRequestV2{TenantID: request.Attempt.TenantID, AttemptID: request.Attempt.ID})
	if err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	now, err := g.nowV2(time.Time{})
	if err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	policy, err := g.Policies.InspectCheckpointBarrierPolicyCurrentV2(ctx, current.Attempt.BarrierPolicy)
	if err != nil || policy.Ref != current.Attempt.BarrierPolicy {
		if err != nil {
			return ports.CheckpointEffectCutBundleV2{}, err
		}
		return ports.CheckpointEffectCutBundleV2{}, checkpointGatewayConflictV2("checkpoint Policy drifted before Effect Cut")
	}
	inventory, err := g.Effects.InspectCheckpointEffectInventoryCurrentV2(ctx, current.Attempt.RefV2(), current.Barrier.RefV2())
	if err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	fresh, err := g.nowV2(now)
	if err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	inventory2, err := g.Effects.InspectCheckpointEffectInventoryCurrentV2(ctx, current.Attempt.RefV2(), current.Barrier.RefV2())
	if err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	if inventory.ProjectionDigest != inventory2.ProjectionDigest {
		return ports.CheckpointEffectCutBundleV2{}, checkpointGatewayConflictV2("checkpoint Effect inventory changed before Cut CAS")
	}
	policy2, err := g.Policies.InspectCheckpointBarrierPolicyCurrentV2(ctx, current.Attempt.BarrierPolicy)
	if err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	if err := policy2.Validate(fresh); err != nil || policy2.ProjectionDigest != policy.ProjectionDigest {
		if err != nil {
			return ports.CheckpointEffectCutBundleV2{}, err
		}
		return ports.CheckpointEffectCutBundleV2{}, checkpointGatewayConflictV2("checkpoint Policy changed before Effect Cut CAS")
	}
	expected, err := control.BuildCheckpointEffectCutBundleV2(current, request, inventory2, fresh)
	if err != nil {
		return ports.CheckpointEffectCutBundleV2{}, err
	}
	committed, err := g.Facts.CommitCheckpointEffectCutV2(ctx, ports.CheckpointEffectCutCommitRequestV2{ExpectedAttemptRevision: request.ExpectedAttemptRevision, ExpectedBarrierRevision: request.ExpectedBarrierRevision, NextAttempt: expected.Attempt, Cut: expected.Cut})
	if err != nil {
		if !checkpointRecoverableV2(err) {
			return ports.CheckpointEffectCutBundleV2{}, err
		}
		cut, inspectErr := g.Facts.InspectCheckpointEffectCutV2(context.WithoutCancel(ctx), expected.Cut.Ref)
		if inspectErr != nil {
			return ports.CheckpointEffectCutBundleV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "checkpoint Effect Cut outcome cannot be inspected")
		}
		bundle, inspectErr := g.Facts.InspectCheckpointAttemptBundleV2(context.WithoutCancel(ctx), ports.InspectCheckpointAttemptRequestV2{TenantID: request.Attempt.TenantID, AttemptID: request.Attempt.ID})
		if inspectErr != nil || bundle.Attempt.EffectCut == nil || *bundle.Attempt.EffectCut != expected.Cut.Ref || cut.Ref != expected.Cut.Ref {
			return ports.CheckpointEffectCutBundleV2{}, checkpointGatewayConflictV2("checkpoint Effect Cut recovery found different history")
		}
		committed = ports.CheckpointEffectCutBundleV2{Attempt: bundle.Attempt, Cut: cut}
	}
	return committed, committed.Validate()
}

func (g CheckpointGovernanceGatewayV2) InspectCheckpointEffectCutV2(ctx context.Context, ref ports.EffectCutRefV2) (ports.EffectCutFactV2, error) {
	if err := ref.Validate(); err != nil {
		return ports.EffectCutFactV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Fact Owner"); err != nil {
		return ports.EffectCutFactV2{}, err
	}
	fact, err := g.Facts.InspectCheckpointEffectCutV2(ctx, ref)
	if err != nil {
		return ports.EffectCutFactV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.EffectCutFactV2{}, err
	}
	if fact.Ref != ref {
		return ports.EffectCutFactV2{}, checkpointGatewayConflictV2("checkpoint Effect Cut ref drifted")
	}
	return fact, nil
}

func (g CheckpointGovernanceGatewayV2) PrepareCheckpointFinalizationInputsV2(ctx context.Context, request ports.PrepareCheckpointFinalizationInputsRequestV2) (ports.CheckpointFinalizationInputClosureRefV2, error) {
	if err := request.Validate(); err != nil {
		return ports.CheckpointFinalizationInputClosureRefV2{}, err
	}
	if err := g.requireMutationDependenciesV2(true, false); err != nil {
		return ports.CheckpointFinalizationInputClosureRefV2{}, err
	}
	current, err := g.Facts.InspectCheckpointAttemptBundleV2(ctx, ports.InspectCheckpointAttemptRequestV2{TenantID: request.Attempt.TenantID, AttemptID: request.Attempt.ID})
	if err != nil {
		return ports.CheckpointFinalizationInputClosureRefV2{}, err
	}
	cut, err := g.Facts.InspectCheckpointEffectCutV2(ctx, request.EffectCut)
	if err != nil {
		return ports.CheckpointFinalizationInputClosureRefV2{}, err
	}
	now, err := g.nowV2(time.Time{})
	if err != nil {
		return ports.CheckpointFinalizationInputClosureRefV2{}, err
	}
	var finalizationCut ports.CheckpointFinalizationCutFactV2
	if current.Attempt.FinalizationCut == nil {
		next, expectedCut, buildErr := control.BuildCheckpointFinalizationCutV2(current, cut, request, now)
		if buildErr != nil {
			return ports.CheckpointFinalizationInputClosureRefV2{}, buildErr
		}
		finalizationCut, err = g.Facts.CommitCheckpointFinalizationCutV2(ctx, ports.CheckpointFinalizationCutCommitRequestV2{ExpectedAttemptRevision: current.Attempt.Revision, NextAttempt: next, Cut: expectedCut})
		if err != nil {
			if !checkpointRecoverableV2(err) {
				return ports.CheckpointFinalizationInputClosureRefV2{}, err
			}
			finalizationCut, err = g.Facts.InspectCheckpointFinalizationCutV2(context.WithoutCancel(ctx), expectedCut.Ref)
			if err != nil || finalizationCut.Ref != expectedCut.Ref {
				return ports.CheckpointFinalizationInputClosureRefV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "checkpoint Finalization Cut outcome cannot be inspected")
			}
		}
		current, err = g.Facts.InspectCheckpointAttemptBundleV2(context.WithoutCancel(ctx), ports.InspectCheckpointAttemptRequestV2{TenantID: request.Attempt.TenantID, AttemptID: request.Attempt.ID})
		if err != nil {
			return ports.CheckpointFinalizationInputClosureRefV2{}, err
		}
	} else {
		finalizationCut, err = g.Facts.InspectCheckpointFinalizationCutV2(ctx, *current.Attempt.FinalizationCut)
		if err != nil {
			return ports.CheckpointFinalizationInputClosureRefV2{}, err
		}
	}
	diagnostics, err := g.Diagnostics.SealCheckpointDiagnosticsForFinalizationV2(ctx, current.Attempt.RefV2(), cut.Ref, finalizationCut.Ref)
	if err != nil {
		return ports.CheckpointFinalizationInputClosureRefV2{}, err
	}
	residuals, err := g.Residuals.SealCheckpointResidualsForFinalizationV2(ctx, current.Attempt.RefV2(), cut.Ref, finalizationCut.Ref)
	if err != nil {
		return ports.CheckpointFinalizationInputClosureRefV2{}, err
	}
	diagnosticCurrent, err := g.Diagnostics.InspectCheckpointDiagnosticsFinalizationSealCurrentV2(ctx, diagnostics)
	if err != nil {
		return ports.CheckpointFinalizationInputClosureRefV2{}, err
	}
	residualCurrent, err := g.Residuals.InspectCheckpointResidualsFinalizationSealCurrentV2(ctx, residuals)
	if err != nil {
		return ports.CheckpointFinalizationInputClosureRefV2{}, err
	}
	if diagnosticCurrent.Validate() != nil || residualCurrent.Validate() != nil || !ports.SameCheckpointDiagnosticsFinalizationSealRefV2(diagnosticCurrent.Ref, diagnostics) || !ports.SameCheckpointResidualsFinalizationSealRefV2(residualCurrent.Ref, residuals) {
		return ports.CheckpointFinalizationInputClosureRefV2{}, checkpointGatewayConflictV2("checkpoint Owner Seals are not exact current projections")
	}
	if request.ExpectedDiagnostics != nil && *request.ExpectedDiagnostics != diagnostics.CompleteSet {
		return ports.CheckpointFinalizationInputClosureRefV2{}, checkpointGatewayConflictV2("expected diagnostics omit or replace Owner set")
	}
	if request.ExpectedResiduals != nil && *request.ExpectedResiduals != residuals.CompleteSet {
		return ports.CheckpointFinalizationInputClosureRefV2{}, checkpointGatewayConflictV2("expected residuals omit or replace Owner set")
	}
	fresh, err := g.nowV2(now)
	if err != nil {
		return ports.CheckpointFinalizationInputClosureRefV2{}, err
	}
	next, closure, err := control.BuildCheckpointFinalizationClosureV2(current.Attempt, current.Barrier, cut, finalizationCut, diagnostics, residuals, request.IdempotencyKey, fresh)
	if err != nil {
		return ports.CheckpointFinalizationInputClosureRefV2{}, err
	}
	committed, err := g.Facts.CommitCheckpointFinalizationInputsV2(ctx, ports.CheckpointFinalizationInputsCommitRequestV2{ExpectedAttemptRevision: current.Attempt.Revision, NextAttempt: next, Closure: closure})
	if err != nil {
		if !checkpointRecoverableV2(err) {
			return ports.CheckpointFinalizationInputClosureRefV2{}, err
		}
		committed, err = g.Facts.InspectCheckpointFinalizationInputsV2(context.WithoutCancel(ctx), closure.Ref)
		if err != nil || !ports.SameCheckpointFinalizationInputClosureRefV2(committed.Ref, closure.Ref) {
			return ports.CheckpointFinalizationInputClosureRefV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "checkpoint Finalization Closure outcome cannot be inspected")
		}
	}
	return committed.Ref, nil
}

func (g CheckpointGovernanceGatewayV2) InspectCheckpointFinalizationInputsV2(ctx context.Context, ref ports.CheckpointFinalizationInputClosureRefV2) (ports.CheckpointFinalizationInputClosureFactV2, error) {
	if err := ref.Validate(); err != nil {
		return ports.CheckpointFinalizationInputClosureFactV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Fact Owner"); err != nil {
		return ports.CheckpointFinalizationInputClosureFactV2{}, err
	}
	fact, err := g.Facts.InspectCheckpointFinalizationInputsV2(ctx, ref)
	if err != nil {
		return ports.CheckpointFinalizationInputClosureFactV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.CheckpointFinalizationInputClosureFactV2{}, err
	}
	if !ports.SameCheckpointFinalizationInputClosureRefV2(fact.Ref, ref) {
		return ports.CheckpointFinalizationInputClosureFactV2{}, checkpointGatewayConflictV2("checkpoint Finalization Closure ref drifted")
	}
	return fact, nil
}

func (g CheckpointGovernanceGatewayV2) CommitCheckpointConsistencyAndCloseBarrierV2(ctx context.Context, request ports.CommitCheckpointConsistencyRequestV2) (ports.CheckpointConsistencyCommitBundleV2, error) {
	if err := request.Validate(); err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if err := g.requireMutationDependenciesV2(false, true); err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Participants, "checkpoint Participant Set current Reader"); err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Closures, "checkpoint Participant Closure current Reader"); err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Effects, "checkpoint Effect inventory current Reader"); err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	current, err := g.Facts.InspectCheckpointAttemptBundleV2(ctx, ports.InspectCheckpointAttemptRequestV2{TenantID: request.Attempt.TenantID, AttemptID: request.Attempt.ID})
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if current.Attempt.State == ports.CheckpointAttemptConsistentV2 {
		return g.recoverCheckpointConsistencyTerminalV2(ctx, request, current)
	}
	cut, err := g.Facts.InspectCheckpointEffectCutV2(ctx, request.EffectCut)
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	now, err := g.nowV2(time.Time{})
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	inventory, err := g.Effects.InspectCheckpointEffectInventoryCurrentV2(ctx, current.Attempt.RefV2(), current.Barrier.RefV2())
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if err := validateCheckpointInventoryAgainstCutV2(inventory, cut, current.Attempt.RefV2(), current.Barrier.RefV2(), now); err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	policy, err := g.Policies.InspectCheckpointBarrierPolicyCurrentV2(ctx, current.Attempt.BarrierPolicy)
	if err != nil || policy.Ref != current.Attempt.BarrierPolicy {
		if err != nil {
			return ports.CheckpointConsistencyCommitBundleV2{}, err
		}
		return ports.CheckpointConsistencyCommitBundleV2{}, checkpointGatewayConflictV2("checkpoint Policy drifted before Consistency")
	}
	participantSet, closures, err := g.readCheckpointParticipantClosuresV2(ctx, current.Attempt.RefV2(), current.Attempt.ParticipantSetCertification, now)
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if request.ManifestSeal.ExactLookup.ScopeDigest != string(current.Attempt.ScopeDigest) {
		return ports.CheckpointConsistencyCommitBundleV2{}, checkpointGatewayConflictV2("checkpoint Manifest Seal exact scope does not match Attempt")
	}
	manifest, err := g.Manifests.InspectCheckpointManifestSealV2(ctx, ports.InspectCheckpointManifestSealRequestV2{Ref: request.ManifestSeal, ExpectedParticipantSetDigest: participantSet.Certification.Digest, ExpectedParticipantClosures: closures})
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	fresh, err := g.nowV2(now)
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	inventory2, err := g.Effects.InspectCheckpointEffectInventoryCurrentV2(ctx, current.Attempt.RefV2(), current.Barrier.RefV2())
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if inventory2.ProjectionDigest != inventory.ProjectionDigest {
		return ports.CheckpointConsistencyCommitBundleV2{}, checkpointGatewayConflictV2("checkpoint Effect inventory changed before Consistency CAS")
	}
	if err := validateCheckpointInventoryAgainstCutV2(inventory2, cut, current.Attempt.RefV2(), current.Barrier.RefV2(), fresh); err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	participantSet2, closures2, err := g.readCheckpointParticipantClosuresV2(ctx, current.Attempt.RefV2(), current.Attempt.ParticipantSetCertification, fresh)
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if participantSet.ProjectionDigest != participantSet2.ProjectionDigest || !sameCheckpointClosureRefsV2(closures, closures2) {
		return ports.CheckpointConsistencyCommitBundleV2{}, checkpointGatewayConflictV2("checkpoint Participant closure set changed before Consistency CAS")
	}
	policy2, err := g.Policies.InspectCheckpointBarrierPolicyCurrentV2(ctx, current.Attempt.BarrierPolicy)
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	manifest2, err := g.Manifests.InspectCheckpointManifestSealV2(ctx, ports.InspectCheckpointManifestSealRequestV2{Ref: request.ManifestSeal, ExpectedParticipantSetDigest: participantSet2.Certification.Digest, ExpectedParticipantClosures: closures2})
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if err := policy2.Validate(fresh); err != nil || policy2.ProjectionDigest != policy.ProjectionDigest || manifest2.Validate() != nil || manifest2.Ref != manifest.Ref || manifest2.SealDigest != manifest.SealDigest {
		if err != nil {
			return ports.CheckpointConsistencyCommitBundleV2{}, err
		}
		return ports.CheckpointConsistencyCommitBundleV2{}, checkpointGatewayConflictV2("checkpoint Policy or Manifest Seal changed before Consistency CAS")
	}
	expected, err := control.BuildCheckpointConsistencyCommitBundleV2(current.Attempt, current.Barrier, cut, request, manifest, participantSet2, closures2, fresh)
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	committed, err := g.Facts.CommitCheckpointConsistencyV2(ctx, ports.CheckpointConsistencyOwnerCommitRequestV2{ExpectedAttemptRevision: current.Attempt.Revision, ExpectedBarrierRevision: current.Barrier.Revision, Bundle: expected})
	if err != nil {
		if !checkpointRecoverableV2(err) {
			return ports.CheckpointConsistencyCommitBundleV2{}, err
		}
		consistency, inspectErr := g.Facts.InspectCheckpointConsistencyV2(context.WithoutCancel(ctx), expected.Consistency.Ref)
		bundle, bundleErr := g.Facts.InspectCheckpointAttemptBundleV2(context.WithoutCancel(ctx), ports.InspectCheckpointAttemptRequestV2{TenantID: request.Attempt.TenantID, AttemptID: request.Attempt.ID})
		if inspectErr != nil || bundleErr != nil || consistency.Ref != expected.Consistency.Ref {
			return ports.CheckpointConsistencyCommitBundleV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "checkpoint Consistency outcome cannot be inspected")
		}
		committed = ports.CheckpointConsistencyCommitBundleV2{Attempt: bundle.Attempt, Barrier: bundle.Barrier, Consistency: consistency}
	}
	if err := committed.Validate(); err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if !sameCheckpointGatewayCanonicalV2("CheckpointConsistencyCommitBundleV2", committed, expected) {
		return ports.CheckpointConsistencyCommitBundleV2{}, checkpointGatewayConflictV2("checkpoint Consistency Owner returned a non-canonical bundle")
	}
	return committed, nil
}

func (g CheckpointGovernanceGatewayV2) FinalizeCheckpointAttemptAndCloseBarrierV2(ctx context.Context, request ports.FinalizeCheckpointAttemptRequestV2) (ports.CheckpointAttemptFinalizationBundleV2, error) {
	if err := request.Validate(); err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	for _, dependency := range []struct {
		value any
		name  string
	}{{g.Facts, "checkpoint Fact Owner"}, {g.Diagnostics, "checkpoint Diagnostics Owner"}, {g.Residuals, "checkpoint Residuals Owner"}, {g.Clock, "checkpoint Clock"}} {
		if err := requireCheckpointDependencyV2(dependency.value, dependency.name); err != nil {
			return ports.CheckpointAttemptFinalizationBundleV2{}, err
		}
	}
	current, err := g.Facts.InspectCheckpointAttemptBundleV2(ctx, ports.InspectCheckpointAttemptRequestV2{TenantID: request.Attempt.TenantID, AttemptID: request.Attempt.ID})
	if err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	if terminalCheckpointStateV2(current.Attempt.State) {
		return g.recoverCheckpointFinalizationTerminalV2(ctx, request, current)
	}
	if current.Attempt.Revision != request.ExpectedAttemptRevision || current.Barrier.Revision != request.ExpectedBarrierRevision || request.Attempt != current.Attempt.RefV2() || request.Barrier != current.Barrier.RefV2() || current.Attempt.FinalizationInputs == nil || !ports.SameCheckpointFinalizationInputClosureRefV2(*current.Attempt.FinalizationInputs, request.Inputs) {
		return ports.CheckpointAttemptFinalizationBundleV2{}, checkpointGatewayConflictV2("checkpoint Finalize expected history drifted")
	}
	inputs, err := g.Facts.InspectCheckpointFinalizationInputsV2(ctx, request.Inputs)
	if err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	now, err := g.nowV2(time.Time{})
	if err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	diagnostics, err := g.Diagnostics.InspectCheckpointDiagnosticsFinalizationSealCurrentV2(ctx, inputs.Ref.DiagnosticsSeal)
	if err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	residuals, err := g.Residuals.InspectCheckpointResidualsFinalizationSealCurrentV2(ctx, inputs.Ref.ResidualsSeal)
	if err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	fresh, err := g.nowV2(now)
	if err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	diagnostics2, err := g.Diagnostics.InspectCheckpointDiagnosticsFinalizationSealCurrentV2(ctx, inputs.Ref.DiagnosticsSeal)
	if err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	residuals2, err := g.Residuals.InspectCheckpointResidualsFinalizationSealCurrentV2(ctx, inputs.Ref.ResidualsSeal)
	if err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	if !ports.SameCheckpointDiagnosticsFinalizationSealRefV2(diagnostics.Ref, diagnostics2.Ref) || !ports.SameCheckpointResidualsFinalizationSealRefV2(residuals.Ref, residuals2.Ref) || !ports.SameCheckpointDiagnosticsFinalizationSealRefV2(diagnostics.Ref, inputs.Ref.DiagnosticsSeal) || !ports.SameCheckpointResidualsFinalizationSealRefV2(residuals.Ref, inputs.Ref.ResidualsSeal) {
		return ports.CheckpointAttemptFinalizationBundleV2{}, checkpointGatewayConflictV2("checkpoint Finalization Owner Seal changed before CAS")
	}
	expected, err := control.BuildCheckpointFinalizationBundleV2(current.Attempt, current.Barrier, inputs, diagnostics2, residuals2, fresh)
	if err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	committed, err := g.Facts.CommitCheckpointFinalizationV2(ctx, ports.CheckpointFinalizationOwnerCommitRequestV2{ExpectedAttemptRevision: current.Attempt.Revision, ExpectedBarrierRevision: current.Barrier.Revision, Bundle: expected})
	if err != nil {
		if !checkpointRecoverableV2(err) {
			return ports.CheckpointAttemptFinalizationBundleV2{}, err
		}
		bundle, inspectErr := g.Facts.InspectCheckpointAttemptBundleV2(context.WithoutCancel(ctx), ports.InspectCheckpointAttemptRequestV2{TenantID: request.Attempt.TenantID, AttemptID: request.Attempt.ID})
		if inspectErr != nil || bundle.Attempt.State != expected.Attempt.State || bundle.Attempt.FinalizationInputs == nil || !ports.SameCheckpointFinalizationInputClosureRefV2(*bundle.Attempt.FinalizationInputs, request.Inputs) || bundle.Barrier.State != ports.CheckpointBarrierClosedV2 {
			return ports.CheckpointAttemptFinalizationBundleV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonCheckpointInconsistent, "checkpoint Finalize outcome cannot be inspected")
		}
		committed = ports.CheckpointAttemptFinalizationBundleV2{Attempt: bundle.Attempt, Barrier: bundle.Barrier, Inputs: inputs}
	}
	return committed, committed.Validate()
}

func (g CheckpointGovernanceGatewayV2) InspectCheckpointAttemptTerminalCurrentV2(ctx context.Context, ref ports.CheckpointAttemptRefV2) (ports.CheckpointAttemptTerminalCurrentProjectionV2, error) {
	if err := ref.Validate(); err != nil {
		return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, err
	}
	// Preflight the complete closed branch dependency set before the first Fact
	// read. This deliberately prefers fail-closed availability over a partial
	// backend read followed by a typed-nil discovery.
	for _, dependency := range []struct {
		value any
		name  string
	}{{g.Facts, "checkpoint Fact Owner"}, {g.Clock, "checkpoint Clock"}, {g.Manifests, "checkpoint Manifest Seal Reader"}, {g.Participants, "checkpoint Participant Set current Reader"}, {g.Closures, "checkpoint Participant Closure current Reader"}, {g.Branches, "checkpoint Participant branch guard Reader"}, {g.Diagnostics, "checkpoint Diagnostics Owner"}, {g.Residuals, "checkpoint Residuals Owner"}} {
		if err := requireCheckpointDependencyV2(dependency.value, dependency.name); err != nil {
			return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, err
		}
	}
	bundle, err := g.Facts.InspectCheckpointAttemptBundleV2(ctx, ports.InspectCheckpointAttemptRequestV2{TenantID: ref.TenantID, AttemptID: ref.ID})
	if err != nil {
		return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, err
	}
	if !terminalCheckpointStateV2(bundle.Attempt.State) || bundle.Attempt.RefV2() != ref || bundle.Barrier.State != ports.CheckpointBarrierClosedV2 {
		return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonCheckpointInconsistent, "checkpoint Attempt is not exact terminal current history")
	}
	now, err := g.nowV2(time.Time{})
	if err != nil {
		return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, err
	}
	projection := ports.CheckpointAttemptTerminalCurrentProjectionV2{Attempt: ref, Barrier: bundle.Barrier.RefV2(), TerminalState: bundle.Attempt.State, CheckedUnixNano: now.UnixNano()}
	if bundle.Attempt.State == ports.CheckpointAttemptConsistentV2 {
		if bundle.Attempt.Consistency == nil {
			return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, checkpointGatewayConflictV2("consistent checkpoint has no Consistency Fact")
		}
		consistency, err := g.Facts.InspectCheckpointConsistencyV2(ctx, *bundle.Attempt.Consistency)
		if err != nil {
			return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, err
		}
		participants, closures, err := g.readCheckpointParticipantClosuresV2(ctx, consistency.Ref.Attempt, bundle.Attempt.ParticipantSetCertification, now)
		if err != nil {
			return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, err
		}
		if consistency.ManifestSeal.ExactLookup.ScopeDigest != string(bundle.Attempt.ScopeDigest) {
			return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, checkpointGatewayConflictV2("checkpoint Manifest Seal exact scope does not match terminal Attempt")
		}
		manifest, err := g.Manifests.InspectCheckpointManifestSealV2(ctx, ports.InspectCheckpointManifestSealRequestV2{Ref: consistency.ManifestSeal, ExpectedParticipantSetDigest: participants.Certification.Digest, ExpectedParticipantClosures: closures})
		if err != nil {
			return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, err
		}
		if consistency.Validate() != nil || manifest.Validate() != nil || manifest.Ref != consistency.ManifestSeal || participants.Certification.Digest != consistency.ParticipantSetDigest || participants.RootDigest != consistency.ParticipantRootDigest || participants.Watermark != consistency.ParticipantWatermark || uint64(len(participants.Participants)) != consistency.ParticipantCount || !sameCheckpointClosureRefsV2(closures, consistency.ParticipantClosures) {
			return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, checkpointGatewayConflictV2("consistent checkpoint Owner closure is no longer current")
		}
		projection.Consistency = bundle.Attempt.Consistency
		return ports.SealCheckpointAttemptTerminalCurrentProjectionV2(projection)
	}
	if bundle.Attempt.FinalizationInputs == nil {
		return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, checkpointGatewayConflictV2("terminal checkpoint has no Finalization Closure")
	}
	inputs, err := g.Facts.InspectCheckpointFinalizationInputsV2(ctx, *bundle.Attempt.FinalizationInputs)
	if err != nil {
		return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, err
	}
	diagnostics, err := g.Diagnostics.InspectCheckpointDiagnosticsFinalizationSealCurrentV2(ctx, inputs.Ref.DiagnosticsSeal)
	if err != nil {
		return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, err
	}
	residuals, err := g.Residuals.InspectCheckpointResidualsFinalizationSealCurrentV2(ctx, inputs.Ref.ResidualsSeal)
	if err != nil {
		return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, err
	}
	if diagnostics.Validate() != nil || residuals.Validate() != nil || !ports.SameCheckpointDiagnosticsFinalizationSealRefV2(diagnostics.Ref, inputs.Ref.DiagnosticsSeal) || !ports.SameCheckpointResidualsFinalizationSealRefV2(residuals.Ref, inputs.Ref.ResidualsSeal) {
		return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, checkpointGatewayConflictV2("terminal checkpoint Owner Seals are no longer current")
	}
	derived, err := control.DeriveCheckpointFinalizationStateV2(bundle.Attempt, diagnostics.Ref, residuals.Ref, time.Unix(0, bundle.Attempt.UpdatedUnixNano))
	if err != nil || derived != bundle.Attempt.State {
		if err != nil {
			return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, err
		}
		return ports.CheckpointAttemptTerminalCurrentProjectionV2{}, checkpointGatewayConflictV2("terminal checkpoint state does not match typed Owner classifications")
	}
	projection.Inputs = &inputs.Ref
	projection.DiagnosticsSeal = &diagnostics.Ref
	projection.ResidualsSeal = &residuals.Ref
	return ports.SealCheckpointAttemptTerminalCurrentProjectionV2(projection)
}

func (g CheckpointGovernanceGatewayV2) InspectCheckpointConsistencyV2(ctx context.Context, ref ports.CheckpointConsistencyRefV2) (ports.CheckpointConsistencyFactV2, error) {
	if err := ref.Validate(); err != nil {
		return ports.CheckpointConsistencyFactV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Fact Owner"); err != nil {
		return ports.CheckpointConsistencyFactV2{}, err
	}
	fact, err := g.Facts.InspectCheckpointConsistencyV2(ctx, ref)
	if err != nil {
		return ports.CheckpointConsistencyFactV2{}, err
	}
	if err := fact.Validate(); err != nil {
		return ports.CheckpointConsistencyFactV2{}, err
	}
	if fact.Ref != ref {
		return ports.CheckpointConsistencyFactV2{}, checkpointGatewayConflictV2("checkpoint Consistency ref drifted")
	}
	return fact, nil
}

func (g CheckpointGovernanceGatewayV2) recoverCheckpointConsistencyTerminalV2(ctx context.Context, request ports.CommitCheckpointConsistencyRequestV2, current ports.CheckpointAttemptBarrierBundleV2) (ports.CheckpointConsistencyCommitBundleV2, error) {
	if current.Attempt.Consistency == nil || current.Barrier.State != ports.CheckpointBarrierClosedV2 || current.Attempt.Revision <= request.ExpectedAttemptRevision || current.Barrier.Revision <= request.ExpectedBarrierRevision {
		return ports.CheckpointConsistencyCommitBundleV2{}, checkpointGatewayConflictV2("checkpoint Consistency terminal history is not a legal progressed successor")
	}
	historicalAttempt, err := g.Facts.InspectCheckpointAttemptHistoricalV2(ctx, request.Attempt)
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	historicalBarrier, err := g.Facts.InspectCheckpointBarrierHistoricalV2(ctx, request.Barrier)
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if historicalAttempt.Revision != request.ExpectedAttemptRevision || historicalBarrier.Revision != request.ExpectedBarrierRevision || historicalAttempt.EffectCut == nil || *historicalAttempt.EffectCut != request.EffectCut {
		return ports.CheckpointConsistencyCommitBundleV2{}, checkpointGatewayConflictV2("checkpoint Consistency replay changed immutable preterminal history")
	}
	consistency, err := g.Facts.InspectCheckpointConsistencyV2(ctx, *current.Attempt.Consistency)
	if err != nil {
		return ports.CheckpointConsistencyCommitBundleV2{}, err
	}
	if consistency.EffectCut != request.EffectCut || consistency.ManifestSeal != request.ManifestSeal || consistency.Barrier != current.Barrier.RefV2() || consistency.Ref.Attempt != historicalAttempt.RefV2() {
		return ports.CheckpointConsistencyCommitBundleV2{}, checkpointGatewayConflictV2("checkpoint Consistency replay changed persisted closure")
	}
	result := ports.CheckpointConsistencyCommitBundleV2{Attempt: current.Attempt, Barrier: current.Barrier, Consistency: consistency}
	return result, result.Validate()
}

func (g CheckpointGovernanceGatewayV2) recoverCheckpointFinalizationTerminalV2(ctx context.Context, request ports.FinalizeCheckpointAttemptRequestV2, current ports.CheckpointAttemptBarrierBundleV2) (ports.CheckpointAttemptFinalizationBundleV2, error) {
	if current.Attempt.State == ports.CheckpointAttemptConsistentV2 || current.Attempt.FinalizationInputs == nil || !ports.SameCheckpointFinalizationInputClosureRefV2(*current.Attempt.FinalizationInputs, request.Inputs) || current.Barrier.State != ports.CheckpointBarrierClosedV2 || current.Attempt.Revision <= request.ExpectedAttemptRevision || current.Barrier.Revision <= request.ExpectedBarrierRevision {
		return ports.CheckpointAttemptFinalizationBundleV2{}, checkpointGatewayConflictV2("checkpoint Finalization terminal history is not a legal progressed successor")
	}
	historicalAttempt, err := g.Facts.InspectCheckpointAttemptHistoricalV2(ctx, request.Attempt)
	if err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	historicalBarrier, err := g.Facts.InspectCheckpointBarrierHistoricalV2(ctx, request.Barrier)
	if err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	if historicalAttempt.Revision != request.ExpectedAttemptRevision || historicalBarrier.Revision != request.ExpectedBarrierRevision || historicalAttempt.FinalizationInputs == nil || !ports.SameCheckpointFinalizationInputClosureRefV2(*historicalAttempt.FinalizationInputs, request.Inputs) {
		return ports.CheckpointAttemptFinalizationBundleV2{}, checkpointGatewayConflictV2("checkpoint Finalization replay changed immutable preterminal history")
	}
	inputs, err := g.Facts.InspectCheckpointFinalizationInputsV2(ctx, request.Inputs)
	if err != nil {
		return ports.CheckpointAttemptFinalizationBundleV2{}, err
	}
	result := ports.CheckpointAttemptFinalizationBundleV2{Attempt: current.Attempt, Barrier: current.Barrier, Inputs: inputs}
	return result, result.Validate()
}

func validateCheckpointAttemptInputsV2(attempt ports.CheckpointAttemptFactV2, projection ports.CheckpointAttemptInputsCurrentProjectionV2, now time.Time) error {
	if err := validateCheckpointAttemptInputIdentityV2(attempt, projection, now); err != nil {
		return err
	}
	if attempt.Barrier.ExpiresUnixNano > projection.ExpiresUnixNano || attempt.ReconciliationDeadlineUnixNano > projection.ExpiresUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "checkpoint Attempt TTL exceeds a current Owner input")
	}
	return nil
}

func validateCheckpointAttemptInputIdentityV2(attempt ports.CheckpointAttemptFactV2, projection ports.CheckpointAttemptInputsCurrentProjectionV2, now time.Time) error {
	if err := projection.Validate(now); err != nil {
		return err
	}
	if projection.AttemptID != attempt.ID || projection.TenantID != attempt.TenantID || projection.RunID != attempt.RunID || projection.RunStableIdentityDigest != attempt.RunStableIdentityDigest || projection.GenerationArtifact != attempt.Generation || projection.GenerationBinding != attempt.GenerationBinding || projection.BindingSet != attempt.BindingSet || projection.ParticipantSetCertification != attempt.ParticipantSetCertification || projection.Workflow != attempt.Workflow {
		return checkpointGatewayConflictV2("checkpoint Attempt current inputs do not bind the exact frozen request")
	}
	return nil
}

func (g CheckpointGovernanceGatewayV2) readCheckpointParticipantClosuresV2(ctx context.Context, attempt ports.CheckpointAttemptRefV2, certification ports.CheckpointParticipantSetCertificationRefV2, now time.Time) (ports.CheckpointParticipantSetCurrentProjectionV2, []ports.CheckpointParticipantClosureRefV2, error) {
	if err := requireCheckpointDependencyV2(g.Branches, "checkpoint Participant branch guard Reader"); err != nil {
		return ports.CheckpointParticipantSetCurrentProjectionV2{}, nil, err
	}
	participants, err := g.Participants.InspectCheckpointParticipantSetCurrentV2(ctx, attempt, certification)
	if err != nil {
		return ports.CheckpointParticipantSetCurrentProjectionV2{}, nil, err
	}
	if err := participants.Validate(now); err != nil {
		return ports.CheckpointParticipantSetCurrentProjectionV2{}, nil, err
	}
	if participants.Attempt != attempt || participants.Certification != certification {
		return ports.CheckpointParticipantSetCurrentProjectionV2{}, nil, checkpointGatewayConflictV2("checkpoint Participant set current projection belongs to another Attempt or certification")
	}
	closures := make([]ports.CheckpointParticipantClosureRefV2, 0, len(participants.Participants))
	for _, participant := range participants.Participants {
		projection, inspectErr := g.Closures.InspectCheckpointParticipantClosureCurrentV2(ctx, attempt, participant)
		if inspectErr != nil {
			return ports.CheckpointParticipantSetCurrentProjectionV2{}, nil, inspectErr
		}
		if err := projection.Validate(now); err != nil {
			return ports.CheckpointParticipantSetCurrentProjectionV2{}, nil, err
		}
		if projection.Attempt != attempt || projection.Participant != participant {
			return ports.CheckpointParticipantSetCurrentProjectionV2{}, nil, checkpointGatewayConflictV2("checkpoint Participant closure current projection belongs to another Attempt or Participant")
		}
		branch, inspectErr := g.Branches.InspectCheckpointParticipantBranchV2(ctx, projection.BranchGuard)
		if inspectErr != nil {
			return ports.CheckpointParticipantSetCurrentProjectionV2{}, nil, inspectErr
		}
		if branch.Validate() != nil || branch.Ref != projection.BranchGuard || branch.Participant != participant || branch.Terminal.Digest != projection.Closure.Terminal.Digest {
			return ports.CheckpointParticipantSetCurrentProjectionV2{}, nil, checkpointGatewayConflictV2("checkpoint Participant branch guard does not bind the current closure")
		}
		closures = append(closures, projection.Closure)
	}
	return participants, closures, nil
}

func sameCheckpointClosureRefsV2(left, right []ports.CheckpointParticipantClosureRefV2) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID != right[index].ID || left[index].Digest != right[index].Digest {
			return false
		}
	}
	return true
}

func validateCheckpointInventoryAgainstCutV2(inventory ports.CheckpointEffectInventoryCurrentProjectionV2, cut ports.EffectCutFactV2, attempt ports.CheckpointAttemptRefV2, barrier ports.CheckpointBarrierLeaseRefV2, now time.Time) error {
	if err := inventory.Validate(now); err != nil {
		return err
	}
	if inventory.Attempt != attempt || inventory.Barrier != barrier || cut.Ref.Attempt.TenantID != attempt.TenantID || cut.Ref.Attempt.ID != attempt.ID || cut.Barrier.TenantID != barrier.TenantID || cut.Barrier.AttemptID != barrier.AttemptID || inventory.RootDigest != cut.Ref.RootDigest || inventory.Watermark != cut.Ref.Watermark || uint64(len(inventory.Entries)) != cut.Ref.Count || len(inventory.Entries) != len(cut.Entries) {
		return checkpointGatewayConflictV2("checkpoint current Effect inventory no longer matches frozen Cut watermarks")
	}
	for index := range inventory.Entries {
		left, leftErr := control.CheckpointCanonicalDigestV2("EffectCutEntryV2", inventory.Entries[index])
		right, rightErr := control.CheckpointCanonicalDigestV2("EffectCutEntryV2", cut.Entries[index])
		if leftErr != nil || rightErr != nil || left != right {
			return checkpointGatewayConflictV2("checkpoint current Effect inventory entry set drifted from frozen Cut")
		}
	}
	return nil
}

func validateCheckpointRunCurrentV2(request ports.CreateCheckpointAttemptRequestV2, projection ports.CheckpointRunCurrentProjectionV2, now time.Time) error {
	if err := projection.Validate(now); err != nil {
		return err
	}
	if projection.RunID != request.RunID || projection.Revision != request.ExpectedRunRevision || projection.RunStableIdentityDigest != request.RunStableIdentityDigest || projection.ExecutionScopeDigest != request.ScopeDigest {
		return checkpointGatewayConflictV2("checkpoint Run current projection differs from the exact create request")
	}
	return nil
}

func minCheckpointNanosV2(values ...int64) int64 {
	result := int64(0)
	for _, value := range values {
		if value > 0 && (result == 0 || value < result) {
			result = value
		}
	}
	return result
}

func (g CheckpointGovernanceGatewayV2) requireMutationDependenciesV2(finalization, manifest bool) error {
	for _, dependency := range []struct {
		value any
		name  string
	}{{g.Facts, "checkpoint Fact Owner"}, {g.Policies, "checkpoint Policy Reader"}, {g.Clock, "checkpoint Clock"}} {
		if err := requireCheckpointDependencyV2(dependency.value, dependency.name); err != nil {
			return err
		}
	}
	if finalization {
		if err := requireCheckpointDependencyV2(g.Diagnostics, "checkpoint Diagnostics Owner"); err != nil {
			return err
		}
		if err := requireCheckpointDependencyV2(g.Residuals, "checkpoint Residuals Owner"); err != nil {
			return err
		}
	}
	if manifest {
		return requireCheckpointDependencyV2(g.Manifests, "checkpoint Manifest Seal Reader")
	}
	return nil
}

func (g CheckpointGovernanceGatewayV2) nowV2(previous time.Time) (time.Time, error) {
	if err := requireCheckpointDependencyV2(g.Clock, "checkpoint Clock"); err != nil {
		return time.Time{}, err
	}
	now := g.Clock()
	if now.IsZero() || (!previous.IsZero() && now.Before(previous)) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "checkpoint clock is zero or regressed")
	}
	return now, nil
}

func requireCheckpointDependencyV2(value any, name string) error {
	if value == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, name+" is required")
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, name+" is typed-nil")
		}
	}
	return nil
}

func checkpointRecoverableV2(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)
}
func checkpointGatewayConflictV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, message)
}
func terminalCheckpointStateV2(state ports.CheckpointAttemptStateV2) bool {
	return state == ports.CheckpointAttemptConsistentV2 || state == ports.CheckpointAttemptIncompleteV2 || state == ports.CheckpointAttemptAbortedV2 || state == ports.CheckpointAttemptIndeterminateV2
}

func exactCheckpointBundleV2(actual, expected ports.CheckpointAttemptBarrierBundleV2) error {
	if actual.Validate() != nil || expected.Validate() != nil {
		return checkpointGatewayConflictV2("checkpoint Attempt bundle failed validation")
	}
	a, _ := control.CheckpointCanonicalDigestV2("CheckpointAttemptBarrierBundleV2", actual)
	b, _ := control.CheckpointCanonicalDigestV2("CheckpointAttemptBarrierBundleV2", expected)
	if a != b {
		return checkpointGatewayConflictV2("checkpoint Attempt create returned different canonical bundle")
	}
	return nil
}

func sameCheckpointGatewayCanonicalV2(discriminator string, left, right any) bool {
	leftDigest, leftErr := control.CheckpointCanonicalDigestV2(discriminator, left)
	rightDigest, rightErr := control.CheckpointCanonicalDigestV2(discriminator, right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}
