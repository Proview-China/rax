package kernel

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ModelProviderActualPointGuardGatewayV1 struct {
	ModelBoundary ports.ModelProviderBoundaryCurrentReaderV1
	Runs          ModelRunLifecycleCurrentReaderV1
	Control       control.ModelDispatchControlCurrentReaderV1
	Effects       ModelOperationEffectCurrentReaderV1
	Dispatch      ModelOperationDispatchCurrentReaderV1
	Clock         func() time.Time
}

type ModelRunLifecycleCurrentReaderV1 interface {
	InspectRunLifecycleV3(context.Context, core.ExecutionScope, core.AgentRunID) (ports.RunLifecycleEnvelopeV3, error)
}

type ModelOperationEffectCurrentReaderV1 interface {
	InspectOperationEffectV3(context.Context, ports.OperationSubjectV3, core.EffectIntentID) (control.OperationEffectFactV3, error)
}

type ModelOperationDispatchCurrentReaderV1 interface {
	InspectCurrentOperationDispatchV4(context.Context, ports.InspectCurrentOperationDispatchRequestV4) (ports.CurrentOperationDispatchAuthorizationV4, error)
}

type modelProviderActualPointClosureV1 struct {
	ModelBoundary ports.ModelProviderBoundaryCurrentProjectionV1
	Run           ports.RunLifecycleEnvelopeV3
	Control       control.ModelDispatchControlCurrentProjectionV1
	Effect        control.OperationEffectFactV3
	Dispatch      ports.CurrentOperationDispatchAuthorizationV4
}

type modelProviderActualPointWatermarkV1 struct {
	ModelRef               ports.ModelProviderBoundaryCurrentRefV1
	ModelState             string
	ModelProvider          ports.ProviderBindingRefV2
	ModelExpiresUnixNano   int64
	Run                    ports.RunLifecycleEnvelopeV3
	ControlOperationDigest core.Digest
	ControlEffectID        core.EffectIntentID
	ControlRunID           core.AgentRunID
	ControlScopeDigest     core.Digest
	ControlRunRevision     core.Revision
	DesiredStateRevision   core.Revision
	LastCommandID          string
	ControlState           control.ModelDispatchControlStateV1
	ControlWatermark       core.Digest
	ControlExpiresUnixNano int64
	Effect                 control.OperationEffectFactV3
	DispatchRecord         ports.OperationDispatchRecordV4
	ReviewAuthorization    ports.OperationReviewAuthorizationRefV4
	ReviewProjection       core.Digest
	ReviewCurrentness      core.Digest
	GovernanceSnapshot     core.Digest
}

func (g ModelProviderActualPointGuardGatewayV1) InspectCurrentModelProviderActualPointV1(ctx context.Context, request ports.InspectCurrentModelProviderActualPointRequestV1) (ports.ModelProviderActualPointCurrentProjectionV1, error) {
	if err := request.Validate(); err != nil {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, err
	}
	if ctx == nil {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model actual-point context is nil")
	}
	if err := ctx.Err(); err != nil {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, err
	}
	if dependencyNilV1(g.ModelBoundary) || dependencyNilV1(g.Runs) || dependencyNilV1(g.Control) || dependencyNilV1(g.Effects) || dependencyNilV1(g.Dispatch) || g.Clock == nil {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Model actual-point current readers are incomplete")
	}
	firstNow := g.Clock()
	if firstNow.IsZero() {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model actual-point clock is zero")
	}
	first, err := g.readCurrentClosureV1(ctx, request, firstNow)
	if err != nil {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, err
	}
	firstDigest, err := digestModelProviderActualPointClosureV1(first)
	if err != nil {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, err
	}

	secondNow := g.Clock()
	if secondNow.IsZero() || secondNow.Before(firstNow) {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model actual-point clock regressed")
	}
	second, err := g.readCurrentClosureV1(ctx, request, secondNow)
	if err != nil {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, err
	}
	if err := ctx.Err(); err != nil {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, err
	}
	secondDigest, err := digestModelProviderActualPointClosureV1(second)
	if err != nil {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, err
	}
	if firstDigest != secondDigest {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model actual-point current facts drifted between S1 and S2")
	}

	sealNow := g.Clock()
	if sealNow.IsZero() || sealNow.Before(secondNow) {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Model actual-point seal clock regressed")
	}
	if err := ctx.Err(); err != nil {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, err
	}
	record := second.Dispatch.Record
	notAfter := ports.ModelProviderActualPointNotAfterV1(
		request.RequestedNotAfterUnixNano,
		second.ModelBoundary.ExpiresUnixNano,
		second.Control.ExpiresUnixNano,
		second.Effect.Intent.ExpiresUnixNano,
		record.Permit.LegacyPermit.ExpiresUnixNano,
		record.Permit.Admission.ExpiresUnixNano,
		record.Fence.ExpiresAt.UnixNano(),
	)
	if notAfter <= sealNow.UnixNano() {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "Model actual-point TTL crossed before seal")
	}
	operationDigest, _ := request.Operation.DigestV3()
	requestDigest, _ := request.DigestV1()
	projection, err := ports.SealModelProviderActualPointCurrentProjectionV1(ports.ModelProviderActualPointCurrentProjectionV1{
		RequestDigest:        requestDigest,
		OperationDigest:      operationDigest,
		EffectID:             request.EffectID,
		EffectFactRevision:   second.Effect.Revision,
		PermitID:             request.PermitID,
		PermitFactRevision:   record.Revision,
		PermitDigest:         record.PermitDigest,
		AdmissionDigest:      record.Permit.Admission.Digest,
		ReviewAuthorization:  record.Permit.Admission.Authorization,
		Attempt:              request.Attempt,
		FenceDigest:          request.FenceDigest,
		RuntimeControlDigest: second.Control.ProjectionDigest,
		ModelBoundary:        request.ModelBoundary,
		Provider:             second.Effect.Intent.Provider,
		Verifier:             request.Verifier,
		CheckedUnixNano:      sealNow.UnixNano(),
		NotAfterUnixNano:     notAfter,
	})
	if err != nil {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, err
	}
	if err := projection.ValidateAgainst(request, sealNow); err != nil {
		return ports.ModelProviderActualPointCurrentProjectionV1{}, err
	}
	return projection, nil
}

func (g ModelProviderActualPointGuardGatewayV1) readCurrentClosureV1(ctx context.Context, request ports.InspectCurrentModelProviderActualPointRequestV1, now time.Time) (modelProviderActualPointClosureV1, error) {
	modelBoundary, err := g.ModelBoundary.InspectCurrentModelProviderBoundaryV1(ctx, request.ModelBoundary)
	if err != nil {
		return modelProviderActualPointClosureV1{}, err
	}
	if err := modelBoundary.Validate(); err != nil {
		return modelProviderActualPointClosureV1{}, err
	}
	if !ports.SameModelProviderBoundaryCurrentRefV1(modelBoundary.Ref, request.ModelBoundary) || modelBoundary.CheckedUnixNano > now.UnixNano() || !now.Before(time.Unix(0, modelBoundary.ExpiresUnixNano)) {
		return modelProviderActualPointClosureV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Model boundary is not the exact current ref")
	}

	run, err := g.Runs.InspectRunLifecycleV3(ctx, request.Operation.ExecutionScope, request.Operation.RunID)
	if err != nil {
		return modelProviderActualPointClosureV1{}, err
	}
	if err := run.Validate(); err != nil {
		return modelProviderActualPointClosureV1{}, err
	}
	if run.Phase != ports.RunLifecycleRunningV3 || run.Run.Status != core.RunRunning || run.Run.ID != request.Operation.RunID || !ports.SameExecutionScopeV2(run.Run.Scope, request.Operation.ExecutionScope) {
		return modelProviderActualPointClosureV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Model actual-point Run is not exact and running")
	}

	controlCurrent, err := g.Control.InspectModelDispatchControlCurrentV1(ctx, request.Operation, request.EffectID)
	if err != nil {
		return modelProviderActualPointClosureV1{}, err
	}
	if err := controlCurrent.ValidateCurrent(request.Operation, request.EffectID, run.Run, now); err != nil {
		return modelProviderActualPointClosureV1{}, err
	}

	effect, err := g.Effects.InspectOperationEffectV3(ctx, request.Operation, request.EffectID)
	if err != nil {
		return modelProviderActualPointClosureV1{}, err
	}
	if err := effect.Validate(); err != nil {
		return modelProviderActualPointClosureV1{}, err
	}
	operationDigest, _ := request.Operation.DigestV3()
	if effect.State != control.OperationEffectDispatchIntentV3 || effect.Revision != request.ExpectedEffectRevision || effect.Intent.ID != request.EffectID || effect.IntentDigest != request.Attempt.IntentDigest || !ports.SameOperationSubjectV3(effect.Intent.Operation, request.Operation) || effect.Intent.Kind != ports.ModelTurnEffectKindV1 || effect.Intent.Provider.Capability != ports.ModelInvokeCapabilityV1 || effect.Intent.Provider != modelBoundary.Provider || effect.DispatchPermitID != request.PermitID || effect.DispatchPermitDigest != request.PermitDigest || effect.UpdatedUnixNano > now.UnixNano() || !now.Before(time.Unix(0, effect.Intent.ExpiresUnixNano)) {
		return modelProviderActualPointClosureV1{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "Model Effect is not one current model-turn dispatch intent")
	}

	dispatch, err := g.Dispatch.InspectCurrentOperationDispatchV4(ctx, ports.InspectCurrentOperationDispatchRequestV4{
		Inspect: ports.InspectOperationDispatchRecordRequestV4{
			Operation: request.Operation,
			EffectID:  request.EffectID,
			PermitID:  request.PermitID,
		},
		AdmissionDigest:     request.AdmissionDigest,
		ReviewAuthorization: request.ReviewAuthorization,
	})
	if err != nil {
		return modelProviderActualPointClosureV1{}, err
	}
	if err := dispatch.Validate(); err != nil {
		return modelProviderActualPointClosureV1{}, err
	}
	if dispatch.CheckedUnixNano > now.UnixNano() {
		return modelProviderActualPointClosureV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "V4 dispatch current check is from the future")
	}
	record := dispatch.Record
	legacy := record.Permit.LegacyPermit
	legacyOperationDigest, legacyOperationErr := legacy.Operation.DigestV3()
	legacyDigest, legacyDigestErr := legacy.DigestV3()
	if record.State != ports.OperationPermitBegunV4 || record.Revision != request.ExpectedPermitFactRevision || record.PermitDigest != request.PermitDigest || record.Permit.Admission.Digest != request.AdmissionDigest || record.Permit.Admission.Authorization != request.ReviewAuthorization || record.EffectFactRevision != effect.Revision || legacy.ID != request.PermitID || legacyOperationErr != nil || legacyOperationDigest != operationDigest || legacy.IntentID != request.EffectID || legacy.IntentRevision != request.Attempt.IntentRevision || legacy.IntentDigest != request.Attempt.IntentDigest || legacy.Revision != request.Attempt.PermitRevision || legacyDigestErr != nil || legacyDigest != request.Attempt.PermitDigest || legacy.AttemptID != request.Attempt.AttemptID || legacy.Provider != effect.Intent.Provider || legacy.EnforcementPoint != request.Verifier || legacy.FenceDigest != request.FenceDigest {
		return modelProviderActualPointClosureV1{}, core.NewError(core.ErrorConflict, core.ReasonDispatchPermitInvalid, "V4 begun Permit does not match the exact Model attempt")
	}
	if record.Fence.BoundaryScope != core.FenceBoundaryInstance || record.Fence.Scope.SandboxLease == nil || record.Fence.EffectIntentID != request.EffectID || record.Fence.EffectIntentRevision != effect.Intent.Revision {
		return modelProviderActualPointClosureV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEffectFenceStale, "Model dispatch requires an exact instance Lease Fence")
	}
	if err := core.CheckFence(record.Fence, core.CurrentFenceFacts{Scope: run.Run.Scope, CapabilityGrantDigest: record.Fence.CapabilityGrantDigest}, now); err != nil {
		return modelProviderActualPointClosureV1{}, err
	}
	return modelProviderActualPointClosureV1{ModelBoundary: modelBoundary, Run: run, Control: controlCurrent, Effect: effect, Dispatch: dispatch}, nil
}

func digestModelProviderActualPointClosureV1(value modelProviderActualPointClosureV1) (core.Digest, error) {
	watermark := modelProviderActualPointWatermarkV1{
		ModelRef: value.ModelBoundary.Ref, ModelState: value.ModelBoundary.State, ModelProvider: value.ModelBoundary.Provider,
		ModelExpiresUnixNano: value.ModelBoundary.ExpiresUnixNano, Run: value.Run,
		ControlOperationDigest: value.Control.OperationDigest, ControlEffectID: value.Control.EffectID, ControlRunID: value.Control.RunID,
		ControlScopeDigest: value.Control.ExecutionScopeDigest, ControlRunRevision: value.Control.RunRevision,
		DesiredStateRevision: value.Control.DesiredStateRevision, LastCommandID: value.Control.LastCommandID,
		ControlState: value.Control.State, ControlWatermark: value.Control.WatermarkDigest, ControlExpiresUnixNano: value.Control.ExpiresUnixNano,
		Effect: value.Effect, DispatchRecord: value.Dispatch.Record, ReviewAuthorization: value.Dispatch.ReviewAuthorization,
		ReviewProjection: value.Dispatch.ReviewProjectionDigest, ReviewCurrentness: value.Dispatch.ReviewCurrentnessDigest,
		GovernanceSnapshot: value.Dispatch.GovernanceSnapshotDigest,
	}
	return core.CanonicalJSONDigest("praxis.runtime.model-provider-actual-point", ports.ModelProviderActualPointGuardContractVersionV1, "ModelProviderActualPointWatermarkV1", watermark)
}

func dependencyNilV1(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}
