package kernel

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RestoreGovernanceGatewayV2 owns only create-once Restore Attempt reservation
// and short-TTL Eligibility binding. It deliberately has no Provider, Stage,
// Activate, Review Authorization, Permit, or external rollback capability.
type RestoreGovernanceGatewayV2 struct {
	Facts  ports.RestoreGovernanceFactPortV2
	Plans  ports.RestorePlanCurrentReaderV2
	Inputs ports.RestoreEligibilityInputsCurrentReaderV2
	Clock  func() time.Time
}

func (g RestoreGovernanceGatewayV2) CreateRestoreAttemptV2(ctx context.Context, request ports.CreateRestoreAttemptRequestV2) (ports.RestoreAttemptFactV2, error) {
	if err := request.Validate(); err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "Restore Fact Owner"); err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Plans, "Restore Plan current Reader"); err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	now, err := g.nowRestoreV2(time.Time{})
	if err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	plan, err := g.Plans.InspectRestorePlanCurrentV2(ctx, request.RestorePlan)
	if err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	expected, err := control.BuildRestoreAttemptReservationV2(request, plan, now)
	if err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	fresh, err := g.nowRestoreV2(now)
	if err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	plan2, err := g.Plans.InspectRestorePlanCurrentV2(ctx, request.RestorePlan)
	if err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	if err := plan2.Validate(fresh); err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	if plan2.ProjectionDigest != plan.ProjectionDigest {
		return ports.RestoreAttemptFactV2{}, restoreGatewayConflictV2("Restore Plan changed between S1 and S2")
	}
	expected2, err := control.BuildRestoreAttemptReservationV2(request, plan2, now)
	if err != nil || expected2.Ref != expected.Ref {
		if err != nil {
			return ports.RestoreAttemptFactV2{}, err
		}
		return ports.RestoreAttemptFactV2{}, restoreGatewayConflictV2("Restore Attempt candidate drifted between S1 and S2")
	}
	created, err := g.Facts.CreateRestoreAttemptV2(ctx, expected)
	if err != nil {
		if !checkpointRecoverableV2(err) {
			return ports.RestoreAttemptFactV2{}, err
		}
		return g.recoverRestoreAttemptCreateV2(ctx, expected)
	}
	if !sameRestoreAttemptExactV2(created, expected) {
		return ports.RestoreAttemptFactV2{}, restoreGatewayConflictV2("Restore Attempt create returned different canonical fact")
	}
	return created.Clone(), nil
}

func (g RestoreGovernanceGatewayV2) recoverRestoreAttemptCreateV2(ctx context.Context, expected ports.RestoreAttemptFactV2) (ports.RestoreAttemptFactV2, error) {
	current, err := g.Facts.InspectRestoreAttemptCurrentV2(context.WithoutCancel(ctx), ports.InspectRestoreAttemptRequestV2{TenantID: expected.Ref.TenantID, AttemptID: expected.Ref.ID})
	if err != nil {
		return ports.RestoreAttemptFactV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonRestoreIncompatible, "Restore Attempt create outcome cannot be inspected")
	}
	if sameRestoreAttemptExactV2(current, expected) {
		return current.Clone(), nil
	}
	if current.Ref.TenantID != expected.Ref.TenantID || current.Ref.ID != expected.Ref.ID || current.Ref.Revision < expected.Ref.Revision || current.OperationScope != expected.OperationScope || current.IdempotencyKey != expected.IdempotencyKey || current.RequestedNotAfter != expected.RequestedNotAfter || current.CreatedUnixNano != expected.CreatedUnixNano {
		return ports.RestoreAttemptFactV2{}, restoreGatewayConflictV2("Restore Attempt create recovery found identity drift or ABA")
	}
	historical, err := g.Facts.InspectRestoreAttemptHistoricalV2(context.WithoutCancel(ctx), expected.Ref)
	if err != nil || !sameRestoreAttemptExactV2(historical, expected) {
		return ports.RestoreAttemptFactV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonRestoreIncompatible, "Restore Attempt initial history cannot be inspected exactly")
	}
	return historical.Clone(), nil
}

func (g RestoreGovernanceGatewayV2) InspectRestoreAttemptV2(ctx context.Context, request ports.InspectRestoreAttemptRequestV2) (ports.RestoreAttemptFactV2, error) {
	if err := request.Validate(); err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "Restore Fact Owner"); err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	fact, err := g.Facts.InspectRestoreAttemptCurrentV2(ctx, request)
	if err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	return fact.Clone(), fact.Validate()
}

func (g RestoreGovernanceGatewayV2) InspectRestoreAttemptHistoricalV2(ctx context.Context, ref ports.RestoreAttemptRefV2) (ports.RestoreAttemptFactV2, error) {
	if err := ref.Validate(); err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "Restore Fact Owner"); err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	fact, err := g.Facts.InspectRestoreAttemptHistoricalV2(ctx, ref)
	if err != nil {
		return ports.RestoreAttemptFactV2{}, err
	}
	if fact.Validate() != nil || fact.Ref != ref {
		return ports.RestoreAttemptFactV2{}, restoreGatewayConflictV2("Restore Attempt historical ref drifted")
	}
	return fact.Clone(), nil
}

func (g RestoreGovernanceGatewayV2) IssueRestoreEligibilityV2(ctx context.Context, request ports.IssueRestoreEligibilityRequestV2) (ports.RestoreEligibilityBindBundleV2, error) {
	if err := request.Validate(); err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "Restore Fact Owner"); err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Plans, "Restore Plan current Reader"); err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Inputs, "Restore Eligibility inputs current Reader"); err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	now, err := g.nowRestoreV2(time.Time{})
	if err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	current, err := g.Facts.InspectRestoreAttemptCurrentV2(ctx, ports.InspectRestoreAttemptRequestV2{TenantID: request.Attempt.TenantID, AttemptID: request.Attempt.ID})
	if err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	if current.Ref != request.Attempt {
		return ports.RestoreEligibilityBindBundleV2{}, restoreGatewayConflictV2("Restore Attempt is no longer exact reserved current")
	}
	plan, err := g.Plans.InspectRestorePlanCurrentV2(ctx, current.OperationScope.RestorePlan)
	if err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	inputs, err := g.Inputs.InspectRestoreEligibilityInputsCurrentV2(ctx, current.Clone())
	if err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	expected, err := control.BuildRestoreEligibilityBindBundleV2(current, request, plan, inputs, now)
	if err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	fresh, err := g.nowRestoreV2(now)
	if err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	plan2, err := g.Plans.InspectRestorePlanCurrentV2(ctx, current.OperationScope.RestorePlan)
	if err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	inputs2, err := g.Inputs.InspectRestoreEligibilityInputsCurrentV2(ctx, current.Clone())
	if err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	if err := plan2.Validate(fresh); err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	if err := inputs2.Validate(fresh); err != nil {
		return ports.RestoreEligibilityBindBundleV2{}, err
	}
	if plan2.ProjectionDigest != plan.ProjectionDigest || inputs2.ProjectionDigest != inputs.ProjectionDigest {
		return ports.RestoreEligibilityBindBundleV2{}, restoreGatewayConflictV2("Restore Eligibility inputs changed between S1 and S2")
	}
	expected2, err := control.BuildRestoreEligibilityBindBundleV2(current, request, plan2, inputs2, now)
	if err != nil || !sameRestoreEligibilityBundleExactV2(expected2, expected) {
		if err != nil {
			return ports.RestoreEligibilityBindBundleV2{}, err
		}
		return ports.RestoreEligibilityBindBundleV2{}, restoreGatewayConflictV2("Restore Eligibility candidate drifted between S1 and S2")
	}
	created, err := g.Facts.BindRestoreEligibilityV2(ctx, ports.RestoreEligibilityBindCommitRequestV2{ExpectedAttempt: current.Ref, Bundle: expected})
	if err != nil {
		if !checkpointRecoverableV2(err) {
			return ports.RestoreEligibilityBindBundleV2{}, err
		}
		return g.recoverRestoreEligibilityBindV2(ctx, expected)
	}
	if !sameRestoreEligibilityBundleExactV2(created, expected) {
		return ports.RestoreEligibilityBindBundleV2{}, restoreGatewayConflictV2("Restore Eligibility bind returned different canonical facts")
	}
	return cloneRestoreEligibilityBundleV2(created), nil
}

func (g RestoreGovernanceGatewayV2) recoverRestoreEligibilityBindV2(ctx context.Context, expected ports.RestoreEligibilityBindBundleV2) (ports.RestoreEligibilityBindBundleV2, error) {
	currentAttempt, err := g.Facts.InspectRestoreAttemptCurrentV2(context.WithoutCancel(ctx), ports.InspectRestoreAttemptRequestV2{TenantID: expected.Attempt.Ref.TenantID, AttemptID: expected.Attempt.Ref.ID})
	if err != nil || currentAttempt.Ref.Revision < expected.Attempt.Ref.Revision || currentAttempt.OperationScope != expected.Attempt.OperationScope || currentAttempt.IdempotencyKey != expected.Attempt.IdempotencyKey {
		return ports.RestoreEligibilityBindBundleV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonRestoreIncompatible, "Restore Eligibility bind outcome cannot be inspected")
	}
	historicalAttempt, err := g.Facts.InspectRestoreAttemptHistoricalV2(context.WithoutCancel(ctx), expected.Attempt.Ref)
	if err != nil || !sameRestoreAttemptExactV2(historicalAttempt, expected.Attempt) {
		return ports.RestoreEligibilityBindBundleV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonRestoreIncompatible, "Restore Eligibility bind Attempt history cannot be inspected exactly")
	}
	historicalEligibility, err := g.Facts.InspectRestoreEligibilityHistoricalV2(context.WithoutCancel(ctx), expected.Eligibility.Ref)
	if err != nil || !sameRestoreEligibilityExactV2(historicalEligibility, expected.Eligibility) {
		return ports.RestoreEligibilityBindBundleV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonRestoreIncompatible, "Restore Eligibility bind history cannot be inspected exactly")
	}
	return cloneRestoreEligibilityBundleV2(expected), nil
}

func (g RestoreGovernanceGatewayV2) InspectRestoreEligibilityV2(ctx context.Context, ref ports.RestoreEligibilityRefV2) (ports.RestoreEligibilityFactV2, error) {
	if err := ref.Validate(); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "Restore Fact Owner"); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	fact, err := g.Facts.InspectRestoreEligibilityHistoricalV2(ctx, ref)
	if err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	if fact.Validate() != nil || fact.Ref != ref {
		return ports.RestoreEligibilityFactV2{}, restoreGatewayConflictV2("Restore Eligibility historical ref drifted")
	}
	return fact.Clone(), nil
}

func (g RestoreGovernanceGatewayV2) InspectCurrentRestoreEligibilityV2(ctx context.Context, request ports.InspectRestoreEligibilityCurrentRequestV2) (ports.RestoreEligibilityFactV2, error) {
	if err := request.Validate(); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "Restore Fact Owner"); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Plans, "Restore Plan current Reader"); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	if err := requireCheckpointDependencyV2(g.Inputs, "Restore Eligibility inputs current Reader"); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	now, err := g.nowRestoreV2(time.Time{})
	if err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	attempt, err := g.Facts.InspectRestoreAttemptCurrentV2(ctx, ports.InspectRestoreAttemptRequestV2{TenantID: request.Attempt.TenantID, AttemptID: request.Attempt.ID})
	if err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	if attempt.Ref != request.Attempt || attempt.State != ports.RestoreAttemptEligibilityBoundV2 || attempt.Eligibility == nil || *attempt.Eligibility != request.ExpectedEligibility {
		return ports.RestoreEligibilityFactV2{}, restoreGatewayConflictV2("Restore Eligibility is not bound to exact current Attempt")
	}
	fact, err := g.Facts.InspectRestoreEligibilityCurrentV2(ctx, request)
	if err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	if fact.Ref != request.ExpectedEligibility || fact.ValidateCurrent(now) != nil {
		return ports.RestoreEligibilityFactV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Restore Eligibility is not exact active current")
	}
	reserved, err := g.Facts.InspectRestoreAttemptHistoricalV2(ctx, fact.Attempt)
	if err != nil || reserved.Ref != fact.Attempt || reserved.State != ports.RestoreAttemptReservedV2 {
		if err != nil {
			return ports.RestoreEligibilityFactV2{}, err
		}
		return ports.RestoreEligibilityFactV2{}, restoreGatewayConflictV2("Restore Eligibility reserved Attempt history drifted")
	}
	plan, err := g.Plans.InspectRestorePlanCurrentV2(ctx, fact.RestorePlan)
	if err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	inputs, err := g.Inputs.InspectRestoreEligibilityInputsCurrentV2(ctx, reserved)
	if err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	fresh, err := g.nowRestoreV2(now)
	if err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	if err := plan.Validate(fresh); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	if err := inputs.Validate(fresh); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	if plan.RestorePlan != fact.RestorePlan || plan.CheckpointConsistency.Ref != fact.CheckpointConsistency || plan.IdentityProposal != fact.Identity || inputs.Attempt != fact.Attempt || inputs.OperationScopeDigest != fact.OperationScopeDigest || inputs.ProjectionDigest != fact.InputsProjectionDigest {
		return ports.RestoreEligibilityFactV2{}, restoreGatewayConflictV2("Restore Eligibility prerequisite current projections drifted")
	}
	finalAttempt, err := g.Facts.InspectRestoreAttemptCurrentV2(ctx, ports.InspectRestoreAttemptRequestV2{TenantID: request.Attempt.TenantID, AttemptID: request.Attempt.ID})
	if err != nil || finalAttempt.Ref != request.Attempt || finalAttempt.Eligibility == nil || *finalAttempt.Eligibility != fact.Ref {
		if err != nil {
			return ports.RestoreEligibilityFactV2{}, err
		}
		return ports.RestoreEligibilityFactV2{}, restoreGatewayConflictV2("Restore Eligibility Attempt changed during current inspection")
	}
	finalFact, err := g.Facts.InspectRestoreEligibilityCurrentV2(ctx, request)
	if err != nil || finalFact.Ref != fact.Ref || finalFact.ValidateCurrent(fresh) != nil {
		if err != nil {
			return ports.RestoreEligibilityFactV2{}, err
		}
		return ports.RestoreEligibilityFactV2{}, restoreGatewayConflictV2("Restore Eligibility changed during current inspection")
	}
	return finalFact.Clone(), nil
}

func (g RestoreGovernanceGatewayV2) CompareAndSwapRestoreEligibilityV2(ctx context.Context, request ports.RestoreEligibilityCASRequestV2) (ports.RestoreEligibilityFactV2, error) {
	if request.Expected.Validate() != nil || request.Next.Validate() != nil {
		return ports.RestoreEligibilityFactV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonRestoreIncompatible, "Restore Eligibility CAS request is incomplete")
	}
	if err := requireCheckpointDependencyV2(g.Facts, "Restore Fact Owner"); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	now, err := g.nowRestoreV2(time.Time{})
	if err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	current, err := g.Facts.InspectRestoreEligibilityHistoricalV2(ctx, request.Expected)
	if err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	if err := control.ValidateRestoreEligibilityTransitionV2(current, request.Next, now); err != nil {
		return ports.RestoreEligibilityFactV2{}, err
	}
	updated, err := g.Facts.CompareAndSwapRestoreEligibilityV2(ctx, request)
	if err != nil {
		if !checkpointRecoverableV2(err) {
			return ports.RestoreEligibilityFactV2{}, err
		}
		updated, err = g.Facts.InspectRestoreEligibilityHistoricalV2(context.WithoutCancel(ctx), request.Next.Ref)
		if err != nil {
			return ports.RestoreEligibilityFactV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonRestoreIncompatible, "Restore Eligibility CAS outcome cannot be inspected")
		}
	}
	if !sameRestoreEligibilityExactV2(updated, request.Next) {
		return ports.RestoreEligibilityFactV2{}, restoreGatewayConflictV2("Restore Eligibility CAS returned different canonical fact")
	}
	return updated.Clone(), nil
}

func (g RestoreGovernanceGatewayV2) nowRestoreV2(previous time.Time) (time.Time, error) {
	if err := requireCheckpointDependencyV2(g.Clock, "Restore Clock"); err != nil {
		return time.Time{}, err
	}
	now := g.Clock()
	if now.IsZero() || (!previous.IsZero() && now.Before(previous)) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Restore clock is zero or regressed")
	}
	return now, nil
}

func sameRestoreAttemptExactV2(left, right ports.RestoreAttemptFactV2) bool {
	return left.Validate() == nil && right.Validate() == nil && left.Ref == right.Ref
}

func sameRestoreEligibilityExactV2(left, right ports.RestoreEligibilityFactV2) bool {
	return left.Validate() == nil && right.Validate() == nil && left.Ref == right.Ref
}

func sameRestoreEligibilityBundleExactV2(left, right ports.RestoreEligibilityBindBundleV2) bool {
	return left.Validate() == nil && right.Validate() == nil && left.Attempt.Ref == right.Attempt.Ref && left.Eligibility.Ref == right.Eligibility.Ref
}

func cloneRestoreEligibilityBundleV2(value ports.RestoreEligibilityBindBundleV2) ports.RestoreEligibilityBindBundleV2 {
	return ports.RestoreEligibilityBindBundleV2{Attempt: value.Attempt.Clone(), Eligibility: value.Eligibility.Clone()}
}

func restoreGatewayConflictV2(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, message)
}

var _ ports.RestoreGovernancePortV2 = RestoreGovernanceGatewayV2{}
