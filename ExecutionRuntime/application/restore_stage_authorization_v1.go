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

type RestoreStageAuthorizationGatewayConfigV1 struct {
	Inputs    applicationports.RestoreStageAuthorizationInputCurrentReaderV1
	Admission runtimeports.OperationEffectAdmissionPortV3
	Reviews   runtimeports.OperationReviewAuthorizationGovernancePortV4
	Dispatch  runtimeports.OperationGovernancePortV4
	Clock     func() time.Time
}

// RestoreStageAuthorizationGatewayV1 is the Restore route of the existing
// host Action Gateway. It never calls Sandbox; it stops after a current begun
// V4 dispatch envelope has been independently inspected.
type RestoreStageAuthorizationGatewayV1 struct {
	config RestoreStageAuthorizationGatewayConfigV1
}

func NewRestoreStageAuthorizationGatewayV1(config RestoreStageAuthorizationGatewayConfigV1) (*RestoreStageAuthorizationGatewayV1, error) {
	if restoreExecutionNilV1(config.Inputs) || restoreExecutionNilV1(config.Admission) || restoreExecutionNilV1(config.Reviews) || restoreExecutionNilV1(config.Dispatch) || config.Clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Restore Stage authorization requires trusted input, Admission, Review, Dispatch and clock ports")
	}
	return &RestoreStageAuthorizationGatewayV1{config: config}, nil
}

func (g *RestoreStageAuthorizationGatewayV1) AuthorizeRestoreStageV1(ctx context.Context, request applicationcontract.RestoreStageActionRequestV1) (applicationcontract.RestoreStageAuthorizedDispatchV1, error) {
	if g == nil || restoreExecutionNilV1(ctx) {
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Restore Stage authorization gateway or context is nil")
	}
	now := g.config.Clock()
	if err := request.ValidateCurrent(now); err != nil {
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, err
	}
	key := restoreStageInspectKeyV1(request)
	inputS1, err := g.config.Inputs.InspectRestoreStageAuthorizationInputCurrentV1(ctx, key)
	if err != nil || inputS1.ValidateFor(request, now) != nil {
		if err != nil {
			return applicationcontract.RestoreStageAuthorizedDispatchV1{}, err
		}
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, inputS1.ValidateFor(request, now)
	}
	admission, err := g.ensureRestoreStageAdmissionV1(ctx, inputS1.Intent)
	if err != nil {
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, err
	}
	authorization, err := g.ensureRestoreStageReviewAuthorizationV1(ctx, inputS1, admission)
	if err != nil {
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, err
	}
	fresh := g.config.Clock()
	inputS2, err := g.config.Inputs.InspectRestoreStageAuthorizationInputCurrentV1(ctx, key)
	if err != nil || inputS2.ValidateFor(request, fresh) != nil || inputS2.ProjectionDigest != inputS1.ProjectionDigest {
		if err != nil {
			return applicationcontract.RestoreStageAuthorizedDispatchV1{}, err
		}
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, core.NewError(core.ErrorConflict, core.ReasonEffectAuthorizationMissing, "Restore Stage authorization input changed between S1 and S2")
	}
	current, err := g.ensureRestoreStageDispatchBegunV1(ctx, inputS2, admission, authorization)
	if err != nil {
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, err
	}
	sealedAt := g.config.Clock()
	result, err := sealRestoreStageAuthorizedDispatchV1(request.Digest, inputS2, current, sealedAt)
	if err != nil {
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, err
	}
	if err := result.ValidateFor(request, sealedAt); err != nil {
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, err
	}
	return result, nil
}

func (g *RestoreStageAuthorizationGatewayV1) InspectRestoreStageAuthorizationV1(ctx context.Context, key applicationcontract.RestoreStageActionInspectKeyV1) (applicationcontract.RestoreStageAuthorizedDispatchV1, error) {
	if g == nil || restoreExecutionNilV1(ctx) || key.Validate() != nil {
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Restore Stage authorization Inspect key is invalid")
	}
	now := g.config.Clock()
	input, err := g.config.Inputs.InspectRestoreStageAuthorizationInputCurrentV1(ctx, key)
	if err != nil || input.ValidateForInspect(key, now) != nil {
		if err != nil {
			return applicationcontract.RestoreStageAuthorizedDispatchV1{}, err
		}
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, input.ValidateForInspect(key, now)
	}
	admission, err := g.config.Admission.InspectAcceptedOperationEffectV3(ctx, input.Intent.Operation, input.Intent.ID)
	if err != nil || validateRestoreStageAdmissionV1(input.Intent, admission) != nil {
		if err != nil {
			return applicationcontract.RestoreStageAuthorizedDispatchV1{}, err
		}
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, validateRestoreStageAdmissionV1(input.Intent, admission)
	}
	authorization, err := g.config.Reviews.InspectCurrentOperationReviewAuthorizationV4(ctx, input.Intent.Operation, input.Intent.ID, input.AuthorizationID)
	if err != nil || validateRestoreStageReviewAuthorizationV1(input.Intent, authorization, now) != nil {
		if err != nil {
			return applicationcontract.RestoreStageAuthorizedDispatchV1{}, err
		}
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, validateRestoreStageReviewAuthorizationV1(input.Intent, authorization, now)
	}
	record, err := g.config.Dispatch.InspectOperationDispatchRecordV4(ctx, runtimeports.InspectOperationDispatchRecordRequestV4{Operation: input.Intent.Operation, EffectID: input.Intent.ID, PermitID: input.PermitID})
	if err != nil {
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, err
	}
	current, err := g.inspectRestoreStageDispatchCurrentV1(ctx, input, authorization.RefV4(), record)
	if err != nil || current.Record.State != runtimeports.OperationPermitBegunV4 {
		if err != nil {
			return applicationcontract.RestoreStageAuthorizedDispatchV1{}, err
		}
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonDispatchPermitInvalid, "Restore Stage dispatch has not begun")
	}
	return sealRestoreStageAuthorizedDispatchV1(key.RequestDigest, input, current, g.config.Clock())
}

func (g *RestoreStageAuthorizationGatewayV1) ensureRestoreStageAdmissionV1(ctx context.Context, intent runtimeports.OperationEffectIntentV3) (runtimeports.OperationEffectAdmissionReceiptV3, error) {
	receipt, err := g.config.Admission.InspectAcceptedOperationEffectV3(ctx, intent.Operation, intent.ID)
	if core.HasCategory(err, core.ErrorNotFound) {
		receipt, err = g.config.Admission.AdmitOperationEffectV3(ctx, intent)
		if err != nil {
			recovered, inspectErr := g.config.Admission.InspectAcceptedOperationEffectV3(context.WithoutCancel(ctx), intent.Operation, intent.ID)
			if inspectErr != nil {
				return runtimeports.OperationEffectAdmissionReceiptV3{}, errors.Join(err, inspectErr)
			}
			receipt, err = recovered, nil
		}
	}
	if err != nil {
		return runtimeports.OperationEffectAdmissionReceiptV3{}, err
	}
	return receipt, validateRestoreStageAdmissionV1(intent, receipt)
}

func validateRestoreStageAdmissionV1(intent runtimeports.OperationEffectIntentV3, receipt runtimeports.OperationEffectAdmissionReceiptV3) error {
	operationDigest, operationErr := intent.Operation.DigestV3()
	intentDigest, intentErr := intent.DigestV3()
	if operationErr != nil || intentErr != nil || receipt.Validate() != nil || receipt.OperationDigest != operationDigest || receipt.EffectID != intent.ID || receipt.IntentRevision != intent.Revision || receipt.IntentDigest != intentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Restore Stage Admission returned another Intent")
	}
	return nil
}

func (g *RestoreStageAuthorizationGatewayV1) ensureRestoreStageReviewAuthorizationV1(ctx context.Context, input applicationcontract.RestoreStageAuthorizationInputCurrentProjectionV1, admission runtimeports.OperationEffectAdmissionReceiptV3) (runtimeports.OperationReviewAuthorizationFactV4, error) {
	fact, err := g.config.Reviews.InspectCurrentOperationReviewAuthorizationV4(ctx, input.Intent.Operation, input.Intent.ID, input.AuthorizationID)
	if core.HasCategory(err, core.ErrorNotFound) {
		requested := input.PermitTTL
		remaining := time.Unix(0, input.ExpiresUnixNano).Sub(g.config.Clock())
		if remaining < requested {
			requested = remaining
		}
		fact, err = g.config.Reviews.CreateOperationReviewAuthorizationV4(ctx, runtimeports.CreateOperationReviewAuthorizationRequestV4{AuthorizationID: input.AuthorizationID, Operation: input.Intent.Operation, EffectID: input.Intent.ID, ExpectedEffectRevision: admission.FactRevision, RequestedTTL: requested})
		if err != nil {
			recovered, inspectErr := g.config.Reviews.InspectCurrentOperationReviewAuthorizationV4(context.WithoutCancel(ctx), input.Intent.Operation, input.Intent.ID, input.AuthorizationID)
			if inspectErr != nil {
				return runtimeports.OperationReviewAuthorizationFactV4{}, errors.Join(err, inspectErr)
			}
			fact, err = recovered, nil
		}
	}
	if err != nil {
		return runtimeports.OperationReviewAuthorizationFactV4{}, err
	}
	return fact, validateRestoreStageReviewAuthorizationV1(input.Intent, fact, g.config.Clock())
}

func validateRestoreStageReviewAuthorizationV1(intent runtimeports.OperationEffectIntentV3, fact runtimeports.OperationReviewAuthorizationFactV4, now time.Time) error {
	digest, err := intent.DigestV3()
	if err != nil || fact.Validate() != nil || fact.State != runtimeports.OperationReviewAuthorizationActiveV4 || now.IsZero() || !now.Before(time.Unix(0, fact.ExpiresUnixNano)) || !runtimeports.SameOperationSubjectV3(fact.Intent.Operation, intent.Operation) || fact.Intent.IntentID != intent.ID || fact.Intent.IntentRevision != intent.Revision || fact.Intent.IntentDigest != digest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Restore Stage Review Authorization returned another or stale Intent")
	}
	return nil
}

func (g *RestoreStageAuthorizationGatewayV1) ensureRestoreStageDispatchBegunV1(ctx context.Context, input applicationcontract.RestoreStageAuthorizationInputCurrentProjectionV1, admission runtimeports.OperationEffectAdmissionReceiptV3, authorization runtimeports.OperationReviewAuthorizationFactV4) (runtimeports.CurrentOperationDispatchAuthorizationV4, error) {
	inspect := runtimeports.InspectOperationDispatchRecordRequestV4{Operation: input.Intent.Operation, EffectID: input.Intent.ID, PermitID: input.PermitID}
	record, err := g.config.Dispatch.InspectOperationDispatchRecordV4(ctx, inspect)
	var current runtimeports.CurrentOperationDispatchAuthorizationV4
	if core.HasCategory(err, core.ErrorNotFound) {
		current, err = g.config.Dispatch.IssueOperationDispatchV4(ctx, runtimeports.IssueGovernedOperationDispatchRequestV4{Operation: input.Intent.Operation, EffectID: input.Intent.ID, ExpectedEffectRevision: admission.FactRevision, Admission: admission, ReviewAuthorization: authorization.RefV4(), PermitID: input.PermitID, AttemptID: input.DispatchAttemptID, PermitTTL: input.PermitTTL})
		if err != nil {
			recovered, inspectErr := g.config.Dispatch.InspectOperationDispatchRecordV4(context.WithoutCancel(ctx), inspect)
			if inspectErr != nil {
				return runtimeports.CurrentOperationDispatchAuthorizationV4{}, errors.Join(err, inspectErr)
			}
			record = recovered
			current, err = g.inspectRestoreStageDispatchCurrentV1(context.WithoutCancel(ctx), input, authorization.RefV4(), record)
		}
	} else if err == nil {
		current, err = g.inspectRestoreStageDispatchCurrentV1(ctx, input, authorization.RefV4(), record)
	}
	if err != nil {
		return runtimeports.CurrentOperationDispatchAuthorizationV4{}, err
	}
	if current.Record.State == runtimeports.OperationPermitIssuedV4 {
		issued := current
		begun, beginErr := g.config.Dispatch.BeginOperationDispatchV4(ctx, runtimeports.BeginGovernedOperationDispatchRequestV4{Operation: input.Intent.Operation, EffectID: input.Intent.ID, ExpectedEffectRevision: issued.Record.EffectFactRevision, PermitID: input.PermitID, ExpectedPermitFactRevision: issued.Record.Revision, AdmissionDigest: issued.Record.Permit.Admission.Digest, ReviewAuthorization: authorization.RefV4()})
		if beginErr != nil {
			recovered, inspectErr := g.config.Dispatch.InspectCurrentOperationDispatchV4(context.WithoutCancel(ctx), runtimeports.InspectCurrentOperationDispatchRequestV4{Inspect: inspect, AdmissionDigest: issued.Record.Permit.Admission.Digest, ReviewAuthorization: authorization.RefV4()})
			if inspectErr != nil {
				return runtimeports.CurrentOperationDispatchAuthorizationV4{}, errors.Join(beginErr, inspectErr)
			}
			current, err = recovered, nil
		} else {
			current = begun
		}
	}
	if current.Record.State != runtimeports.OperationPermitBegunV4 || current.Record.Permit.LegacyPermit.AttemptID != input.DispatchAttemptID || current.Record.Permit.LegacyPermit.IntentDigest != admission.IntentDigest {
		return runtimeports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Restore Stage dispatch did not reach the exact begun attempt")
	}
	return current, nil
}

func (g *RestoreStageAuthorizationGatewayV1) inspectRestoreStageDispatchCurrentV1(ctx context.Context, input applicationcontract.RestoreStageAuthorizationInputCurrentProjectionV1, authorization runtimeports.OperationReviewAuthorizationRefV4, record runtimeports.OperationDispatchRecordV4) (runtimeports.CurrentOperationDispatchAuthorizationV4, error) {
	if record.Validate() != nil || record.Permit.LegacyPermit.AttemptID != input.DispatchAttemptID {
		return runtimeports.CurrentOperationDispatchAuthorizationV4{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Restore Stage historical dispatch record drifted")
	}
	return g.config.Dispatch.InspectCurrentOperationDispatchV4(ctx, runtimeports.InspectCurrentOperationDispatchRequestV4{Inspect: runtimeports.InspectOperationDispatchRecordRequestV4{Operation: input.Intent.Operation, EffectID: input.Intent.ID, PermitID: input.PermitID}, AdmissionDigest: record.Permit.Admission.Digest, ReviewAuthorization: authorization})
}

func sealRestoreStageAuthorizedDispatchV1(requestDigest core.Digest, input applicationcontract.RestoreStageAuthorizationInputCurrentProjectionV1, current runtimeports.CurrentOperationDispatchAuthorizationV4, now time.Time) (applicationcontract.RestoreStageAuthorizedDispatchV1, error) {
	expires := minimumRestoreExecutionTimeV1(input.ExpiresUnixNano, current.Record.Permit.LegacyPermit.ExpiresUnixNano)
	value, err := applicationcontract.SealRestoreStageAuthorizedDispatchV1(applicationcontract.RestoreStageAuthorizedDispatchV1{RequestDigest: requestDigest, Dispatch: current, SnapshotArtifact: input.SnapshotArtifact, EvidenceSource: input.EvidenceSource, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		return applicationcontract.RestoreStageAuthorizedDispatchV1{}, err
	}
	return value, nil
}

var _ applicationports.RestoreStageAuthorizationPortV1 = (*RestoreStageAuthorizationGatewayV1)(nil)
