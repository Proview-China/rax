package kernel

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RestoreActivationGatewayV1 is the Runtime-owned final CAS. It can only
// activate a fresh reserved Instance after independently inspecting the exact
// Sandbox Stage, Runtime Settlement, Sandbox ApplySettlement and Context
// materialization facts. It never executes Restore or rewinds external state.
type RestoreActivationGatewayV1 struct {
	Restore     ports.RestoreGovernancePortV2
	Stage       ports.RestoreStageDomainResultCurrentReaderV1
	Settlements ports.RestoreStageSettlementGovernancePortV1
	Sandbox     ports.RestoreStageApplySettlementCurrentReaderV1
	Context     ports.RestoreContextMaterializationCurrentReaderV1
	Facts       ports.RestoreActivationFactPortV1
	Clock       func() time.Time
}

func (g RestoreActivationGatewayV1) ActivateRestoreV1(ctx context.Context, submission ports.RestoreActivationSubmissionV1) (ports.RestoreActivationRefV1, error) {
	if err := submission.Validate(); err != nil {
		return ports.RestoreActivationRefV1{}, err
	}
	dependencies := []struct {
		value any
		name  string
	}{{g.Restore, "Restore Governance"}, {g.Stage, "Sandbox Stage current"}, {g.Settlements, "Runtime Restore Settlement"}, {g.Sandbox, "Sandbox ApplySettlement current"}, {g.Context, "Context materialization current"}, {g.Facts, "Restore Activation facts"}}
	for _, dependency := range dependencies {
		if err := requireCheckpointDependencyV2(dependency.value, dependency.name); err != nil {
			return ports.RestoreActivationRefV1{}, err
		}
	}
	if g.Clock == nil {
		return ports.RestoreActivationRefV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Restore Activation clock is unavailable")
	}
	now := g.Clock()
	if now.IsZero() {
		return ports.RestoreActivationRefV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Restore Activation clock is zero")
	}

	attempt, err := g.Restore.InspectRestoreAttemptV2(ctx, ports.InspectRestoreAttemptRequestV2{TenantID: submission.Attempt.TenantID, AttemptID: submission.Attempt.ID})
	if err != nil {
		return ports.RestoreActivationRefV1{}, err
	}
	if attempt.State == ports.RestoreAttemptActivatedV2 && attempt.Ref.TenantID == submission.Attempt.TenantID && attempt.Ref.ID == submission.Attempt.ID && attempt.Ref.Revision == submission.Attempt.Revision+1 {
		existing, inspectErr := g.Facts.InspectRestoreActivationByAttemptV1(ctx, attempt.Ref)
		if inspectErr != nil {
			return ports.RestoreActivationRefV1{}, inspectErr
		}
		if !sameRestoreActivationSubmissionV1(existing.Submission, submission) || existing.Ref.Attempt != attempt.Ref {
			return ports.RestoreActivationRefV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "activated Restore Attempt binds another submission")
		}
		return existing.Ref, nil
	}
	if attempt.Ref != submission.Attempt || attempt.State != ports.RestoreAttemptEligibilityBoundV2 || attempt.Eligibility == nil || *attempt.Eligibility != submission.Eligibility || attempt.OperationScope.Identity != submission.Context.Identity {
		return ports.RestoreActivationRefV1{}, restoreStageConflictV1("Activation Attempt, Eligibility, or reserved Identity is no longer exact current")
	}
	eligibility, err := g.Restore.InspectCurrentRestoreEligibilityV2(ctx, ports.InspectRestoreEligibilityCurrentRequestV2{Attempt: attempt.Ref, ExpectedEligibility: submission.Eligibility})
	if err != nil {
		if recovered, ok := g.recoverActivatedRestoreV1(context.WithoutCancel(ctx), submission); ok {
			return recovered, nil
		}
		return ports.RestoreActivationRefV1{}, err
	}
	if err := eligibility.ValidateCurrent(now); err != nil || eligibility.Identity != attempt.OperationScope.Identity {
		return ports.RestoreActivationRefV1{}, restoreStageConflictV1("Activation Eligibility is not exact active current")
	}

	stage, err := g.Stage.InspectRestoreStageDomainResultCurrentV1(ctx, submission.Stage)
	if err != nil {
		return ports.RestoreActivationRefV1{}, err
	}
	if err := stage.Validate(now); err != nil || !ports.SameRestoreStageDomainResultFactRefV1(stage.Fact, submission.Stage) {
		return ports.RestoreActivationRefV1{}, restoreStageConflictV1("Activation Sandbox Stage is not exact current")
	}
	settlement, err := g.Settlements.InspectCurrentRestoreStageSettlementV1(ctx, submission.Stage.Operation, submission.Stage.EffectID)
	if err != nil {
		return ports.RestoreActivationRefV1{}, err
	}
	if settlement != submission.RuntimeSettlement || !ports.SameRestoreStageDomainResultFactRefV1(settlement.DomainResult, submission.Stage) {
		return ports.RestoreActivationRefV1{}, restoreStageConflictV1("Activation Runtime Settlement drifted")
	}
	sandbox, err := g.Sandbox.InspectRestoreStageApplySettlementCurrentV1(ctx, submission.SandboxSettlement)
	if err != nil {
		return ports.RestoreActivationRefV1{}, err
	}
	if err := sandbox.ValidateCurrent(now); err != nil || sandbox.Fact != submission.SandboxSettlement {
		return ports.RestoreActivationRefV1{}, restoreStageConflictV1("Activation Sandbox ApplySettlement is not exact current")
	}
	contextCurrent, err := g.Context.InspectRestoreContextMaterializationCurrentV1(ctx, submission.Context)
	if err != nil {
		return ports.RestoreActivationRefV1{}, err
	}
	if err := contextCurrent.ValidateCurrent(now); err != nil || !sameRestoreContextMaterializationRefV1(contextCurrent.Fact, submission.Context) || len(contextCurrent.Residuals) != 0 {
		return ports.RestoreActivationRefV1{}, restoreStageConflictV1("Activation Context materialization is stale, drifted, or residual")
	}
	if submission.Stage.Operation.ExecutionScope.Instance != attempt.OperationScope.Identity.TargetInstance || submission.Stage.Operation.ExecutionScope.SandboxLease == nil || *submission.Stage.Operation.ExecutionScope.SandboxLease != attempt.OperationScope.Identity.TargetLease || submission.Stage.Operation.ExecutionScope.AuthorityEpoch != attempt.OperationScope.Identity.TargetFenceEpoch || contextCurrent.Fact.TargetScopeDigest != submission.Stage.Operation.ExecutionScopeDigest {
		return ports.RestoreActivationRefV1{}, restoreStageConflictV1("Activation target Instance, Lease, Fence, or Context scope drifted")
	}

	next := attempt.Clone()
	next.State = ports.RestoreAttemptActivatedV2
	next.Ref.Revision++
	next.UpdatedUnixNano = now.UnixNano()
	next, err = ports.SealRestoreAttemptFactV2(next)
	if err != nil {
		return ports.RestoreActivationRefV1{}, err
	}
	fact, err := ports.SealRestoreActivationFactV1(ports.RestoreActivationFactV1{
		Ref:        ports.RestoreActivationRefV1{ID: "restore-activation:" + attempt.Ref.ID, Attempt: next.Ref},
		Submission: submission, Identity: attempt.OperationScope.Identity, ActivatedUnixNano: now.UnixNano(),
	})
	if err != nil {
		return ports.RestoreActivationRefV1{}, err
	}
	stored, err := g.Facts.CommitRestoreActivationV1(ctx, ports.RestoreActivationCommitRequestV1{ExpectedAttempt: attempt.Ref, NextAttempt: next, Activation: fact})
	if err != nil {
		var inspectErr error
		stored, inspectErr = g.Facts.InspectRestoreActivationByAttemptV1(context.WithoutCancel(ctx), next.Ref)
		if inspectErr != nil {
			return ports.RestoreActivationRefV1{}, err
		}
	}
	if stored.Ref != fact.Ref || stored.Submission.IdempotencyKey != submission.IdempotencyKey || stored.Identity != fact.Identity {
		return ports.RestoreActivationRefV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Restore Activation recovery returned another fact")
	}
	return stored.Ref, nil
}

func (g RestoreActivationGatewayV1) recoverActivatedRestoreV1(ctx context.Context, submission ports.RestoreActivationSubmissionV1) (ports.RestoreActivationRefV1, bool) {
	current, err := g.Restore.InspectRestoreAttemptV2(ctx, ports.InspectRestoreAttemptRequestV2{TenantID: submission.Attempt.TenantID, AttemptID: submission.Attempt.ID})
	if err != nil || current.State != ports.RestoreAttemptActivatedV2 || current.Ref.TenantID != submission.Attempt.TenantID || current.Ref.ID != submission.Attempt.ID || current.Ref.Revision != submission.Attempt.Revision+1 {
		return ports.RestoreActivationRefV1{}, false
	}
	fact, err := g.Facts.InspectRestoreActivationByAttemptV1(ctx, current.Ref)
	if err != nil || fact.Ref.Attempt != current.Ref || !sameRestoreActivationSubmissionV1(fact.Submission, submission) {
		return ports.RestoreActivationRefV1{}, false
	}
	return fact.Ref, true
}

func (g RestoreActivationGatewayV1) InspectRestoreActivationV1(ctx context.Context, ref ports.RestoreActivationRefV1) (ports.RestoreActivationFactV1, error) {
	if err := ref.Validate(); err != nil {
		return ports.RestoreActivationFactV1{}, err
	}
	return g.Facts.InspectRestoreActivationV1(ctx, ref)
}

func (g RestoreActivationGatewayV1) InspectRestoreActivationByAttemptV1(ctx context.Context, attempt ports.RestoreAttemptRefV2) (ports.RestoreActivationFactV1, error) {
	if err := attempt.Validate(); err != nil {
		return ports.RestoreActivationFactV1{}, err
	}
	return g.Facts.InspectRestoreActivationByAttemptV1(ctx, attempt)
}

func (g RestoreActivationGatewayV1) InspectRestoreActivationByStableAttemptV1(ctx context.Context, tenantID core.TenantID, attemptID string) (ports.RestoreActivationFactV1, error) {
	if tenantID == "" || attemptID == "" {
		return ports.RestoreActivationFactV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Restore Activation stable Attempt coordinate is incomplete")
	}
	if err := requireCheckpointDependencyV2(g.Facts, "Restore Activation facts"); err != nil {
		return ports.RestoreActivationFactV1{}, err
	}
	fact, err := g.Facts.InspectRestoreActivationByStableAttemptV1(ctx, tenantID, attemptID)
	if err != nil {
		return ports.RestoreActivationFactV1{}, err
	}
	if fact.Validate() != nil || fact.Ref.Attempt.TenantID != tenantID || fact.Ref.Attempt.ID != attemptID || fact.Submission.Attempt.TenantID != tenantID || fact.Submission.Attempt.ID != attemptID {
		return ports.RestoreActivationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "Restore Activation stable Inspect returned another Attempt")
	}
	return fact, nil
}

func sameRestoreContextMaterializationRefV1(left, right ports.RestoreContextMaterializationRefV1) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.runtime.restore-activation", ports.RestoreActivationContractVersionV1, "RestoreContextMaterializationRefV1", left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.runtime.restore-activation", ports.RestoreActivationContractVersionV1, "RestoreContextMaterializationRefV1", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func sameRestoreActivationSubmissionV1(left, right ports.RestoreActivationSubmissionV1) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.runtime.restore-activation", ports.RestoreActivationContractVersionV1, "RestoreActivationSubmissionV1", left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.runtime.restore-activation", ports.RestoreActivationContractVersionV1, "RestoreActivationSubmissionV1", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

var _ ports.RestoreActivationGovernancePortV1 = RestoreActivationGatewayV1{}
