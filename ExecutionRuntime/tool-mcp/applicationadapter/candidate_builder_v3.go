package applicationadapter

import (
	"context"
	"strconv"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type SingleCallToolActionCandidateBuildRequestV3 struct {
	ApplicationRequest applicationcontract.SingleCallToolActionRequestV2                `json:"application_request"`
	ApplicationInput   applicationcontract.SingleCallToolActionInputCurrentProjectionV2 `json:"application_input"`
	ModelProjection    modelinvoker.ToolCallCandidateObservationProjectionV1            `json:"model_projection"`
	SurfaceBinding     toolcontract.ToolSurfaceInvocationBindingV1                      `json:"surface_binding"`
	SurfaceBindingAck  toolcontract.ToolSurfaceInvocationBindingAckV1                   `json:"surface_binding_ack"`
	InputContract      toolcontract.ToolInputContractCurrentProjectionV1                `json:"input_contract"`
	Route              runtimeports.ControlledOperationProviderRouteCurrentProjectionV2 `json:"route"`
	ProviderCurrent    runtimeports.ProviderBindingCurrentProjectionV2                  `json:"provider_current"`
}

type CandidateBuilderV3 struct {
	registry toolcontract.ToolRegistryObjectCurrentReaderV1
	clock    ClockV1
}

func NewCandidateBuilderV3(registry toolcontract.ToolRegistryObjectCurrentReaderV1, clock ClockV1) (*CandidateBuilderV3, error) {
	if isNilFlowDependencyV1(registry) || isNilFlowDependencyV1(clock) {
		return nil, bindingInvalidV1("CandidateV3 Builder dependencies are required")
	}
	return &CandidateBuilderV3{registry: registry, clock: clock}, nil
}

func (b *CandidateBuilderV3) BuildSingleCallToolActionCandidateV3(ctx context.Context, request SingleCallToolActionCandidateBuildRequestV3) (toolcontract.ActionCandidateV3, error) {
	if ctx == nil {
		return toolcontract.ActionCandidateV3{}, bindingInvalidV1("CandidateV3 Builder context is required")
	}
	if err := ctx.Err(); err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	if b == nil || isNilFlowDependencyV1(b.registry) || isNilFlowDependencyV1(b.clock) {
		return toolcontract.ActionCandidateV3{}, bindingUnavailableV1("CandidateV3 Builder is unavailable")
	}
	now := b.clock.Now()
	if now.IsZero() {
		return toolcontract.ActionCandidateV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "CandidateV3 Builder clock is unavailable")
	}
	if err := request.ApplicationRequest.ValidateCurrent(now); err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	if err := request.ApplicationInput.ValidateFor(request.ApplicationRequest, now); err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	if err := request.ModelProjection.Validate(); err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	if err := request.SurfaceBinding.ValidateCurrent(now); err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	if err := request.SurfaceBindingAck.ValidateAgainst(request.SurfaceBinding, now); err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	if err := request.InputContract.ValidateCurrent(now); err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	if err := request.Route.Validate(); err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	if err := request.ProviderCurrent.ValidateCurrent(request.Route.ProviderBinding, now); err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	if len(request.ModelProjection.Observation.Calls) != 1 {
		return toolcontract.ActionCandidateV3{}, bindingConflictV1("CandidateV3 requires exactly one Model Tool Call")
	}
	subject := request.ApplicationRequest.Action.PendingSubject
	if err := validateModelForBindingV1(request.ModelProjection, request.ModelProjection.Ref, request.ApplicationInput, subject); err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	if request.SurfaceBinding.Subject.Invocation.InvocationID != request.ModelProjection.Ref.InvocationID || request.SurfaceBinding.Subject.Invocation.InvocationDigest != request.ModelProjection.Ref.InvocationDigest || request.SurfaceBinding.Subject.SurfaceCurrent.Ref != request.InputContract.SurfaceCurrent.Ref || request.Route.ProviderBinding != request.InputContract.BindingSubject.ProviderBinding || effectOwnerFromRouteV1(request.Route) != request.InputContract.BindingSubject.ExpectedOwner || request.ProviderCurrent.Ref != request.Route.ProviderBinding {
		return toolcontract.ActionCandidateV3{}, bindingConflictV1("CandidateV3 cross-owner exact closure drifted")
	}
	tool, current, err := b.registry.InspectExactToolDescriptorCurrentV1(ctx, request.InputContract.BindingSubject.Tool, request.InputContract.ToolCurrent.Ref)
	if err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	now2 := b.clock.Now()
	if now2.IsZero() || now2.Before(now) {
		return toolcontract.ActionCandidateV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "CandidateV3 Builder clock regressed")
	}
	if err := current.ValidateCurrent(now2); err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	if tool.ArtifactDigest != request.Route.ProviderBinding.ArtifactDigest || tool.ConflictDomain == "" || tool.Idempotency == "" {
		return toolcontract.ActionCandidateV3{}, bindingConflictV1("CandidateV3 Tool Descriptor or Provider binding drifted")
	}
	source, err := toolcontract.SealModelSourceCandidateHistoricalRefV1(request.ModelProjection)
	if err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	call := request.ModelProjection.Observation.Calls[0]
	payload := runtimeports.OpaquePayloadV2{
		Schema: request.InputContract.BindingSubject.InputSchema, ContentDigest: core.DigestBytes(call.CanonicalArguments),
		Length: uint64(len(call.CanonicalArguments)), Inline: append([]byte(nil), call.CanonicalArguments...), LimitPolicy: request.InputContract.BindingSubject.LimitPolicy,
	}
	expires := minimumPositiveUnixNanoV1(
		request.ApplicationRequest.ExpiresUnixNano, request.ApplicationInput.ExpiresUnixNano, request.InputContract.ExpiresUnixNano,
		request.SurfaceBinding.NotAfterUnixNano, request.Route.ExpiresUnixNano, request.ProviderCurrent.ExpiresUnixNano, current.ExpiresUnixNano,
	)
	if expires <= now2.UnixNano() {
		return toolcontract.ActionCandidateV3{}, bindingExpiredV1("CandidateV3 source window crossed before Seal")
	}
	pending := toolcontract.PendingActionExactRefV2{ID: subject.PendingActionRef, Revision: 1, RequestDigest: subject.PendingActionDigest}
	candidate, err := toolcontract.SealActionCandidateV3(toolcontract.ActionCandidateV3{
		TenantID: subject.Run.ExecutionScope.Identity.TenantID, RunID: string(subject.Run.RunID), SessionID: subject.SessionID, TurnID: strconv.FormatUint(uint64(subject.Turn), 10),
		PendingAction: pending, SourceCandidate: source, Surface: request.InputContract.BindingSubject.Surface,
		Capability: request.InputContract.BindingSubject.Capability, Tool: request.InputContract.BindingSubject.Tool, InputSchema: request.InputContract.BindingSubject.InputSchema,
		Payload: payload, PayloadRevision: 1, LimitPolicy: request.InputContract.BindingSubject.LimitPolicy, InputContractCurrentRef: request.InputContract.Ref,
		SurfaceCurrent: request.InputContract.SurfaceCurrent.Ref, CapabilityCurrent: request.InputContract.CapabilityCurrent.Ref, ToolCurrent: request.InputContract.ToolCurrent.Ref, InputSchemaCurrent: request.InputContract.InputSchemaCurrent,
		OperationScopeDigest: request.ApplicationRequest.Action.ExecutionScopeDigest, EffectKind: runtimeports.OperationScopeEvidenceActionEffectKindV3,
		ExpectedOwner: effectOwnerFromRouteV1(request.Route), ConflictDomain: tool.ConflictDomain, IdempotencyKey: request.ApplicationRequest.ID,
		CreatedUnixNano: now2.UnixNano(), RequestedExpiresUnixNano: expires,
	})
	if err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	if err := candidate.ValidateAgainstInputContract(request.InputContract); err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	if err := candidate.ValidateAgainstModelProjection(request.ModelProjection); err != nil {
		return toolcontract.ActionCandidateV3{}, err
	}
	return candidate, nil
}
