package application

import (
	"context"
	"errors"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type RestoreStageActionGatewayConfigV1 struct {
	Results       applicationports.RestoreStageActionResultFactPortV1
	Authorization applicationports.RestoreStageAuthorizationPortV1
	Participant   applicationports.RestoreStageParticipantPortV1
	Enforcement   runtimeports.RestoreStageEnforcementGovernancePortV1
	Governance    runtimeports.RestoreStageGovernanceCurrentPortV1
	Evidence      applicationports.RestoreStageEvidencePublisherV1
	Settlements   runtimeports.RestoreStageSettlementGovernancePortV1
	Clock         func() time.Time
}

type RestoreStageActionGatewayV1 struct {
	config RestoreStageActionGatewayConfigV1
}

func NewRestoreStageActionGatewayV1(config RestoreStageActionGatewayConfigV1) (*RestoreStageActionGatewayV1, error) {
	if restoreExecutionNilV1(config.Results) || restoreExecutionNilV1(config.Authorization) || restoreExecutionNilV1(config.Participant) || restoreExecutionNilV1(config.Enforcement) || restoreExecutionNilV1(config.Governance) || restoreExecutionNilV1(config.Evidence) || restoreExecutionNilV1(config.Settlements) || config.Clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Restore Stage Action Gateway requires result, authorization, participant, enforcement, evidence and settlement Owners")
	}
	return &RestoreStageActionGatewayV1{config: config}, nil
}

func (g *RestoreStageActionGatewayV1) ExecuteRestoreStageActionV1(ctx context.Context, request applicationcontract.RestoreStageActionRequestV1) (applicationcontract.RestoreStageActionResultV1, error) {
	if g == nil || restoreExecutionNilV1(ctx) {
		return applicationcontract.RestoreStageActionResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Restore Stage Action Gateway or context is nil")
	}
	now := g.config.Clock()
	if err := request.ValidateCurrent(now); err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	if stored, err := g.config.Results.InspectRestoreStageActionResultV1(ctx, request.Attempt.TenantID, request.ID); err == nil {
		if stored.ValidateFor(request, now) != nil {
			return applicationcontract.RestoreStageActionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Restore Stage result belongs to another request")
		}
		return stored.Result, nil
	} else if !core.HasCategory(err, core.ErrorNotFound) {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}

	authorized, authorizeErr := g.config.Authorization.AuthorizeRestoreStageV1(ctx, request)
	if authorizeErr != nil {
		authorized, _ = g.config.Authorization.InspectRestoreStageAuthorizationV1(context.WithoutCancel(ctx), restoreStageInspectKeyV1(request))
		if authorized.Digest == "" {
			return applicationcontract.RestoreStageActionResultV1{}, authorizeErr
		}
	}
	now = g.config.Clock()
	if err := authorized.ValidateFor(request, now); err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	prepared, err := g.config.Participant.PrepareRestoreStageV1(ctx, request, authorized)
	if err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	if err := prepared.ValidateCurrent(g.config.Clock()); err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}

	record := authorized.Dispatch.Record
	legacy := record.Permit.LegacyPermit
	operationDigest, err := legacy.Operation.DigestV3()
	if err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	dispatchAttempt := runtimeports.OperationDispatchAttemptRefV3{OperationDigest: operationDigest, EffectID: legacy.IntentID, IntentRevision: legacy.IntentRevision, IntentDigest: legacy.IntentDigest, PermitID: legacy.ID, PermitRevision: record.Revision, PermitDigest: record.PermitDigest, AttemptID: legacy.AttemptID}
	base := runtimeports.EnforceRestoreStageDispatchRequestV1{
		Operation: legacy.Operation, EffectID: legacy.IntentID, PermitID: legacy.ID, ExpectedPermitFactRevision: record.Revision, PermitDigest: record.PermitDigest,
		AdmissionDigest: record.Permit.Admission.Digest, ReviewAuthorization: authorized.Dispatch.ReviewAuthorization, DispatchAttempt: dispatchAttempt,
		SandboxAttempt: prepared.SandboxAttempt, SandboxProjectionDigest: prepared.ProjectionDigest, RestoreAttempt: request.Attempt, Eligibility: request.Eligibility,
		Identity: request.Materialization.Identity, SnapshotArtifact: authorized.SnapshotArtifact, Verifier: legacy.EnforcementPoint,
	}
	prepareRequest := base
	prepareRequest.Phase = runtimeports.OperationDispatchEnforcementPrepareV4
	prepare, enforceErr := g.config.Enforcement.EnforceRestoreStageDispatchV1(ctx, prepareRequest)
	if enforceErr != nil {
		var inspectErr error
		prepare, inspectErr = g.config.Enforcement.InspectRestoreStageDispatchEnforcementByRequestV1(context.WithoutCancel(ctx), prepareRequest)
		if inspectErr != nil {
			return applicationcontract.RestoreStageActionResultV1{}, errors.Join(enforceErr, inspectErr)
		}
	}
	executeRequest := base
	executeRequest.Phase = runtimeports.OperationDispatchEnforcementExecuteV4
	executeRequest.ExpectedJournalRevision = 1
	executeRequest.Prepare = &prepare
	executeRequest.Prepared = &prepared.Prepared
	execute, enforceErr := g.config.Enforcement.EnforceRestoreStageDispatchV1(ctx, executeRequest)
	if enforceErr != nil {
		var inspectErr error
		execute, inspectErr = g.config.Enforcement.InspectRestoreStageDispatchEnforcementByRequestV1(context.WithoutCancel(ctx), executeRequest)
		if inspectErr != nil {
			return applicationcontract.RestoreStageActionResultV1{}, errors.Join(enforceErr, inspectErr)
		}
	}
	coordinates := runtimeports.InspectRestoreStageGovernanceCurrentRequestV1{RestoreAttempt: request.Attempt, Eligibility: request.Eligibility, Operation: legacy.Operation, EffectID: legacy.IntentID, Admission: record.Permit.Admission.Admission, Authorization: authorized.Dispatch.ReviewAuthorization, PermitID: legacy.ID, DispatchAttempt: dispatchAttempt, ExecuteEnforcement: execute, SnapshotArtifact: authorized.SnapshotArtifact}
	governance, err := g.config.Governance.InspectRestoreStageGovernanceCurrentV1(ctx, coordinates)
	if err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	now = g.config.Clock()
	if err := governance.Validate(now); err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	stage, err := g.config.Participant.ExecuteRestoreStageV1(ctx, request, authorized, execute)
	if err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	now = g.config.Clock()
	if err := stage.Validate(now); err != nil || stage.Fact.Attempt != dispatchAttempt {
		return applicationcontract.RestoreStageActionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage Participant returned another DomainResult")
	}
	evidenceRequest := applicationcontract.RestoreStageEvidenceRequestV1{RequestDigest: request.Digest, Governance: governance, DomainResult: stage.Fact, SourceRegistration: authorized.EvidenceSource}
	if err := evidenceRequest.Validate(now); err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	evidence, err := g.config.Evidence.PublishRestoreStageEvidenceV1(ctx, evidenceRequest)
	if err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	settlementID, err := restoreStageStableIDV1("restore-stage-settlement", request.Digest)
	if err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	settlementKey, err := restoreStageStableIDV1("restore-stage-settlement-key", request.Digest)
	if err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	submission, err := runtimeports.SealRestoreStageSettlementSubmissionV1(runtimeports.RestoreStageSettlementSubmissionV1{ID: settlementID, Operation: governance.Operation, OperationDigest: stage.Fact.OperationDigest, EffectID: stage.Fact.EffectID, EffectRevision: stage.Fact.EffectRevision, RestoreAttempt: request.Attempt, Eligibility: request.Eligibility, Governance: governance, DomainResult: stage.Fact, Evidence: evidence, IdempotencyKey: settlementKey, SettledUnixNano: now.UnixNano()})
	if err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	settlement, err := g.config.Settlements.SettleRestoreStageV1(ctx, submission)
	if err != nil {
		recovered, inspectErr := g.config.Settlements.InspectCurrentRestoreStageSettlementV1(context.WithoutCancel(ctx), governance.Operation, governance.EffectID)
		if inspectErr != nil {
			return applicationcontract.RestoreStageActionResultV1{}, err
		}
		if recovered.DomainResult.ID != stage.Fact.ID {
			return applicationcontract.RestoreStageActionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonSettlementOwnerMismatch, "Restore Stage settlement recovery returned another DomainResult")
		}
		settlement = recovered
	}
	apply, err := g.config.Participant.ApplyRestoreStageSettlementV1(ctx, settlement, stage)
	if err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	now = g.config.Clock()
	expires := minimumRestoreExecutionTimeV1(request.NotAfterUnixNano, authorized.ExpiresUnixNano, governance.ExpiresUnixNano, stage.ExpiresUnixNano, apply.ExpiresUnixNano)
	result, err := applicationcontract.SealRestoreStageActionResultV1(applicationcontract.RestoreStageActionResultV1{RequestDigest: request.Digest, Stage: stage, RuntimeSettlement: settlement, SandboxSettlement: apply, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil || result.ValidateFor(request, now) != nil {
		if err != nil {
			return applicationcontract.RestoreStageActionResultV1{}, err
		}
		return applicationcontract.RestoreStageActionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonRestoreIncompatible, "Restore Stage result closure is invalid")
	}
	fact, err := applicationcontract.SealRestoreStageActionResultFactV1(applicationcontract.RestoreStageActionResultFactV1{Result: result}, request, now)
	if err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	stored, createErr := g.config.Results.CreateRestoreStageActionResultV1(ctx, fact)
	if createErr != nil {
		stored, err = g.config.Results.InspectRestoreStageActionResultV1(context.WithoutCancel(ctx), fact.TenantID, fact.ID)
		if err != nil {
			return applicationcontract.RestoreStageActionResultV1{}, errors.Join(createErr, err)
		}
	}
	if stored.Digest != fact.Digest || stored.ValidateFor(request, now) != nil {
		return applicationcontract.RestoreStageActionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Restore Stage result persistence returned another fact")
	}
	return stored.Result, nil
}

func (g *RestoreStageActionGatewayV1) InspectRestoreStageActionV1(ctx context.Context, key applicationcontract.RestoreStageActionInspectKeyV1) (applicationcontract.RestoreStageActionResultV1, error) {
	if g == nil || key.Validate() != nil {
		return applicationcontract.RestoreStageActionResultV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Restore Stage Inspect key is invalid")
	}
	fact, err := g.config.Results.InspectRestoreStageActionResultV1(ctx, key.Attempt.TenantID, key.ID)
	if err != nil {
		return applicationcontract.RestoreStageActionResultV1{}, err
	}
	if fact.RequestDigest != key.RequestDigest || fact.Result.RequestDigest != key.RequestDigest || fact.Result.Stage.Fact.RestoreAttempt != key.Attempt || fact.Result.Stage.Fact.Eligibility != key.Eligibility {
		return applicationcontract.RestoreStageActionResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Restore Stage Inspect returned another request")
	}
	return fact.Result, nil
}

func restoreStageInspectKeyV1(request applicationcontract.RestoreStageActionRequestV1) applicationcontract.RestoreStageActionInspectKeyV1 {
	return applicationcontract.RestoreStageActionInspectKeyV1{ID: request.ID, IdempotencyKey: request.IdempotencyKey, Attempt: request.Attempt, Eligibility: request.Eligibility, RequestDigest: request.Digest}
}

func restoreStageStableIDV1(prefix string, digest core.Digest) (string, error) {
	value := prefix + ":" + string(digest)
	if len(value) > 512 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Restore Stage derived identity is too long")
	}
	return value, nil
}

var _ applicationports.RestoreStageActionPortV1 = (*RestoreStageActionGatewayV1)(nil)
