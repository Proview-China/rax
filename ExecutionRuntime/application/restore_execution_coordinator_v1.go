package application

import (
	"context"
	"errors"
	"reflect"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type RestoreExecutionCoordinatorConfigV1 struct {
	Intents         applicationports.RestoreExecutionIntentFactPortV1
	Results         applicationports.RestoreExecutionResultFactPortV1
	Restore         runtimeports.RestoreGovernancePortV2
	Materialization runtimeports.RestoreMaterializationCurrentReaderV1
	Stage           applicationports.RestoreStageActionPortV1
	Context         applicationports.RestoreContextMaterializationPortV1
	Activation      runtimeports.RestoreActivationGovernancePortV1
	Clock           func() time.Time
}

type RestoreExecutionCoordinatorV1 struct {
	config RestoreExecutionCoordinatorConfigV1
}

func NewRestoreExecutionCoordinatorV1(config RestoreExecutionCoordinatorConfigV1) (*RestoreExecutionCoordinatorV1, error) {
	if restoreExecutionNilV1(config.Intents) || restoreExecutionNilV1(config.Results) || restoreExecutionNilV1(config.Restore) || restoreExecutionNilV1(config.Materialization) || restoreExecutionNilV1(config.Stage) || restoreExecutionNilV1(config.Context) || restoreExecutionNilV1(config.Activation) || config.Clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Restore execution requires Runtime, Action Gateway, Context, Activation, and clock ports")
	}
	return &RestoreExecutionCoordinatorV1{config: config}, nil
}

func (c *RestoreExecutionCoordinatorV1) ExecuteRestoreV1(ctx context.Context, request applicationcontract.RestoreExecutionRequestV1) (applicationcontract.RestoreExecutionResultV1, error) {
	if c == nil || restoreExecutionNilV1(ctx) {
		return applicationcontract.RestoreExecutionResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Restore execution coordinator or context is nil")
	}
	now := c.config.Clock()
	if err := request.ValidateCurrent(now); err != nil {
		return applicationcontract.RestoreExecutionResultV1{}, err
	}
	intent, err := applicationcontract.SealRestoreExecutionIntentFactV1(request, now)
	if err != nil {
		return applicationcontract.RestoreExecutionResultV1{}, err
	}
	storedIntent, createIntentErr := c.config.Intents.CreateRestoreExecutionIntentV1(ctx, intent)
	if createIntentErr != nil {
		storedIntent, err = c.config.Intents.InspectRestoreExecutionIntentV1(context.WithoutCancel(ctx), intent.TenantID, intent.ID)
		if err != nil {
			return applicationcontract.RestoreExecutionResultV1{}, createIntentErr
		}
	}
	if storedIntent.Digest != intent.Digest || storedIntent.RequestDigest != request.Digest || storedIntent.ValidateCurrent(now) != nil {
		return applicationcontract.RestoreExecutionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Restore execution Intent persistence returned another request")
	}
	completed, inspectErr := c.config.Results.InspectRestoreExecutionResultV1(ctx, core.TenantID(request.RestorePlan.TenantID), request.ID)
	if inspectErr == nil {
		if completed.Request.Digest != request.Digest || completed.ValidateCurrent(now) != nil {
			return applicationcontract.RestoreExecutionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Restore execution result belongs to another request")
		}
		return completed.Result, nil
	}
	if !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return applicationcontract.RestoreExecutionResultV1{}, inspectErr
	}

	attempt, err := c.config.Restore.CreateRestoreAttemptV2(ctx, runtimeports.CreateRestoreAttemptRequestV2{AttemptID: request.RestoreAttemptID, IdempotencyKey: request.IdempotencyKey, RestorePlan: request.RestorePlan, RequestedNotAfter: request.NotAfterUnixNano})
	if err != nil {
		attempt, err = c.recoverAttemptV1(context.WithoutCancel(ctx), request, err)
		if err != nil {
			return applicationcontract.RestoreExecutionResultV1{}, err
		}
	}
	if attempt.OperationScope.RestorePlan != request.RestorePlan || attempt.IdempotencyKey != request.IdempotencyKey {
		return applicationcontract.RestoreExecutionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Restore Attempt recovery returned another request")
	}

	bundle, err := c.config.Restore.IssueRestoreEligibilityV2(ctx, runtimeports.IssueRestoreEligibilityRequestV2{EligibilityID: request.RestoreEligibilityID, Attempt: attempt.Ref, RequestedTTL: request.EligibilityTTL})
	if err != nil {
		bundle, err = c.recoverEligibilityV1(context.WithoutCancel(ctx), attempt, request, err)
		if err != nil {
			return applicationcontract.RestoreExecutionResultV1{}, err
		}
	}
	attempt = bundle.Attempt
	eligibility := bundle.Eligibility
	now = c.config.Clock()
	if err := eligibility.ValidateCurrent(now); err != nil || attempt.State != runtimeports.RestoreAttemptEligibilityBoundV2 || attempt.Eligibility == nil || *attempt.Eligibility != eligibility.Ref {
		return applicationcontract.RestoreExecutionResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRestoreIncompatible, "Restore Eligibility is not exact active current")
	}

	materialization, err := c.config.Materialization.InspectRestoreMaterializationCurrentV1(ctx, runtimeports.InspectRestoreMaterializationCurrentRequestV1{Attempt: attempt.Ref, Eligibility: eligibility.Ref})
	if err != nil {
		return applicationcontract.RestoreExecutionResultV1{}, err
	}
	now = c.config.Clock()
	if err := materialization.ValidateCurrent(now); err != nil {
		return applicationcontract.RestoreExecutionResultV1{}, err
	}
	stageRequest, err := applicationcontract.SealRestoreStageActionRequestV1(applicationcontract.RestoreStageActionRequestV1{ID: request.StageActionID, IdempotencyKey: request.StageIdempotencyKey, Attempt: attempt.Ref, Eligibility: eligibility.Ref, Materialization: materialization, NotAfterUnixNano: minimumRestoreExecutionTimeV1(request.NotAfterUnixNano, materialization.ExpiresUnixNano, eligibility.Ref.ExpiresUnixNano)})
	if err != nil {
		return applicationcontract.RestoreExecutionResultV1{}, err
	}
	stage, executeErr := c.config.Stage.ExecuteRestoreStageActionV1(ctx, stageRequest)
	if executeErr != nil {
		key := applicationcontract.RestoreStageActionInspectKeyV1{ID: stageRequest.ID, IdempotencyKey: stageRequest.IdempotencyKey, Attempt: stageRequest.Attempt, Eligibility: stageRequest.Eligibility, RequestDigest: stageRequest.Digest}
		stage, err = c.config.Stage.InspectRestoreStageActionV1(context.WithoutCancel(ctx), key)
		if err != nil {
			return applicationcontract.RestoreExecutionResultV1{}, executeErr
		}
	}
	now = c.config.Clock()
	if err := stage.ValidateFor(stageRequest, now); err != nil {
		return applicationcontract.RestoreExecutionResultV1{}, err
	}

	contextRequest, err := applicationcontract.SealRestoreContextMaterializationRequestV1(applicationcontract.RestoreContextMaterializationRequestV1{
		ID: request.ContextID, IdempotencyKey: request.ContextIdempotencyKey, Materialization: materialization,
		Stage: stage.Stage, SandboxSettlement: stage.SandboxSettlement, Requirements: request.Requirements,
		RequestedUnixNano: now.UnixNano(), NotAfterUnixNano: minimumRestoreExecutionTimeV1(request.NotAfterUnixNano, stage.ExpiresUnixNano, materialization.ExpiresUnixNano, eligibility.Ref.ExpiresUnixNano),
	})
	if err != nil {
		return applicationcontract.RestoreExecutionResultV1{}, err
	}
	contextCurrent, err := c.config.Context.MaterializeRestoreContextV1(ctx, contextRequest)
	if err != nil {
		return applicationcontract.RestoreExecutionResultV1{}, err
	}
	now = c.config.Clock()
	if err := contextCurrent.ValidateCurrent(now); err != nil || len(contextCurrent.Residuals) != 0 {
		return applicationcontract.RestoreExecutionResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonRestoreIncompatible, "Restore Context materialization is stale or residual")
	}
	activationSubmission := runtimeports.RestoreActivationSubmissionV1{Attempt: attempt.Ref, Eligibility: eligibility.Ref, Stage: stage.Stage.Fact, RuntimeSettlement: stage.RuntimeSettlement, SandboxSettlement: stage.SandboxSettlement.Fact, Context: contextCurrent.Fact, IdempotencyKey: request.ActivationIdempotencyKey}
	activation, activateErr := c.config.Activation.ActivateRestoreV1(ctx, activationSubmission)
	if activateErr != nil {
		fact, inspectErr := c.config.Activation.InspectRestoreActivationByStableAttemptV1(context.WithoutCancel(ctx), attempt.Ref.TenantID, attempt.Ref.ID)
		if inspectErr != nil {
			return applicationcontract.RestoreExecutionResultV1{}, activateErr
		}
		if fact.Validate() != nil || fact.Ref.Attempt.TenantID != attempt.Ref.TenantID || fact.Ref.Attempt.ID != attempt.Ref.ID || fact.Ref.Attempt.Revision != attempt.Ref.Revision+1 || !sameRestoreActivationSubmissionApplicationV1(fact.Submission, activationSubmission) {
			return applicationcontract.RestoreExecutionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Restore Activation inspect recovered another submission")
		}
		activation = fact.Ref
	}
	result, err := applicationcontract.SealRestoreExecutionResultV1(applicationcontract.RestoreExecutionResultV1{RequestDigest: request.Digest, Attempt: attempt.Ref, Eligibility: eligibility.Ref, Stage: stage, Context: contextCurrent, Activation: activation})
	if err != nil {
		return applicationcontract.RestoreExecutionResultV1{}, err
	}
	fact, err := applicationcontract.SealRestoreExecutionResultFactV1(applicationcontract.RestoreExecutionResultFactV1{Request: request, Result: result}, now)
	if err != nil {
		return applicationcontract.RestoreExecutionResultV1{}, err
	}
	stored, createErr := c.config.Results.CreateRestoreExecutionResultV1(ctx, fact)
	if createErr != nil {
		stored, err = c.config.Results.InspectRestoreExecutionResultV1(context.WithoutCancel(ctx), fact.TenantID, fact.ID)
		if err != nil {
			return applicationcontract.RestoreExecutionResultV1{}, errors.Join(createErr, err)
		}
	}
	if stored.Digest != fact.Digest || stored.Request.Digest != request.Digest || stored.Result.Digest != result.Digest || stored.ValidateCurrent(now) != nil {
		return applicationcontract.RestoreExecutionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Restore execution result persistence returned another fact")
	}
	return stored.Result, nil
}

func sameRestoreActivationSubmissionApplicationV1(left, right runtimeports.RestoreActivationSubmissionV1) bool {
	leftDigest, leftErr := core.CanonicalJSONDigest("praxis.application.restore-execution", applicationcontract.RestoreExecutionContractVersionV1, "RestoreActivationSubmissionV1", left)
	rightDigest, rightErr := core.CanonicalJSONDigest("praxis.application.restore-execution", applicationcontract.RestoreExecutionContractVersionV1, "RestoreActivationSubmissionV1", right)
	return leftErr == nil && rightErr == nil && leftDigest == rightDigest
}

func (c *RestoreExecutionCoordinatorV1) recoverAttemptV1(ctx context.Context, request applicationcontract.RestoreExecutionRequestV1, original error) (runtimeports.RestoreAttemptFactV2, error) {
	current, err := c.config.Restore.InspectRestoreAttemptV2(ctx, runtimeports.InspectRestoreAttemptRequestV2{TenantID: core.TenantID(request.RestorePlan.TenantID), AttemptID: request.RestoreAttemptID})
	if err != nil {
		return runtimeports.RestoreAttemptFactV2{}, original
	}
	if current.OperationScope.RestorePlan != request.RestorePlan || current.IdempotencyKey != request.IdempotencyKey {
		return runtimeports.RestoreAttemptFactV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Restore Attempt inspect recovered another request")
	}
	return current, nil
}

func (c *RestoreExecutionCoordinatorV1) recoverEligibilityV1(ctx context.Context, attempt runtimeports.RestoreAttemptFactV2, request applicationcontract.RestoreExecutionRequestV1, original error) (runtimeports.RestoreEligibilityBindBundleV2, error) {
	current, err := c.config.Restore.InspectRestoreAttemptV2(ctx, runtimeports.InspectRestoreAttemptRequestV2{TenantID: attempt.Ref.TenantID, AttemptID: attempt.Ref.ID})
	if err != nil || current.Eligibility == nil || current.Eligibility.ID != request.RestoreEligibilityID {
		return runtimeports.RestoreEligibilityBindBundleV2{}, original
	}
	eligibility, err := c.config.Restore.InspectCurrentRestoreEligibilityV2(ctx, runtimeports.InspectRestoreEligibilityCurrentRequestV2{Attempt: current.Ref, ExpectedEligibility: *current.Eligibility})
	if err != nil {
		return runtimeports.RestoreEligibilityBindBundleV2{}, original
	}
	bundle := runtimeports.RestoreEligibilityBindBundleV2{Attempt: current, Eligibility: eligibility}
	if bundle.Validate() != nil {
		return runtimeports.RestoreEligibilityBindBundleV2{}, original
	}
	return bundle, nil
}

func minimumRestoreExecutionTimeV1(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func restoreExecutionNilV1(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

var _ applicationports.RestoreExecutionPortV1 = (*RestoreExecutionCoordinatorV1)(nil)
