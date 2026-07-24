package runtimeadapter

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ControlledProviderV2 is the Tool-owned, production-neutral adapter for the
// Runtime controlled Provider V2 gateway. It never owns a Provider transport,
// raw Provider endpoint, or production composition root.
type ControlledProviderV2 struct {
	routes          runtimeports.ControlledOperationProviderRouteCurrentReaderV2
	gateway         runtimeports.ControlledOperationProviderPortV2
	clock           ClockV1
	recoveryTimeout time.Duration
}

func NewControlledProviderV2(
	routes runtimeports.ControlledOperationProviderRouteCurrentReaderV2,
	gateway runtimeports.ControlledOperationProviderPortV2,
	clock ClockV1,
	recoveryTimeout time.Duration,
) (*ControlledProviderV2, error) {
	if isNilDependencyV2(routes) || isNilDependencyV2(gateway) || isNilDependencyV2(clock) || recoveryTimeout <= 0 || recoveryTimeout > 30*time.Second {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "controlled Provider V2 adapter dependencies are incomplete")
	}
	return &ControlledProviderV2{routes: routes, gateway: gateway, clock: clock, recoveryTimeout: recoveryTimeout}, nil
}

func (a *ControlledProviderV2) EnterControlledProviderV2(ctx context.Context, request runtimeports.ControlledOperationProviderRequestV2) (runtimeports.ControlledOperationProviderResultV2, error) {
	if isNilDependencyV2(ctx) {
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider V2 context is nil")
	}
	if err := request.Validate(); err != nil {
		return runtimeports.ControlledOperationProviderResultV2{}, err
	}
	if a == nil || isNilDependencyV2(a.routes) || isNilDependencyV2(a.gateway) || isNilDependencyV2(a.clock) {
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "controlled Provider V2 adapter is unavailable")
	}
	if request.EffectKind != runtimeports.OperationScopeEvidenceActionEffectKindV3 || request.Operation.Kind != runtimeports.OperationScopeRunV3 {
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorForbidden, core.ReasonUnknownGovernanceCategory, "controlled Provider V2 adapter only accepts the N=1 Tool Action matrix")
	}

	before := a.clock.Now()
	if before.IsZero() {
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "controlled Provider V2 route read clock is zero")
	}
	matrix := runtimeports.OperationScopeEvidenceActionMatrixV3()
	route, err := a.routes.InspectCurrentControlledOperationProviderRouteV2(ctx, request.RouteCurrentRef, matrix)
	if err != nil {
		return runtimeports.ControlledOperationProviderResultV2{}, err
	}
	after := a.clock.Now()
	if after.IsZero() || after.Before(before) {
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "controlled Provider V2 route read clock regressed")
	}
	if err = route.ValidateCurrent(request.RouteCurrentRef, matrix, after); err != nil {
		return runtimeports.ControlledOperationProviderResultV2{}, err
	}
	if route.DeclarationRef != request.RouteDeclarationRef || route.ConformanceRef != request.RouteConformanceRef || route.ToolAdapterBinding != request.ToolAdapterBinding || route.ProviderBinding != request.ProviderBinding {
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider V2 route binds another request")
	}
	// Transport and actual Provider are separate route roles. A shared component
	// or artifact is an alias injection even when the nominal capabilities differ.
	if route.ProviderTransportBinding.ComponentID == route.ProviderBinding.ComponentID || route.ProviderTransportBinding.ArtifactDigest == route.ProviderBinding.ArtifactDigest {
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "controlled Provider V2 transport aliases the actual Provider")
	}
	if !after.Before(time.Unix(0, request.CallerDeadlineUnixNano)) {
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "controlled Provider V2 caller deadline expired before Gateway entry")
	}

	result, enterErr := a.gateway.EnterControlledOperationProviderV2(ctx, request)
	if enterErr != nil {
		if !core.HasCategory(enterErr, core.ErrorUnavailable) && !core.HasCategory(enterErr, core.ErrorIndeterminate) {
			return runtimeports.ControlledOperationProviderResultV2{}, enterErr
		}
		inspected, inspectErr := a.inspectAfterLostReply(ctx, request)
		if inspectErr != nil {
			return runtimeports.ControlledOperationProviderResultV2{}, enterErr
		}
		return inspected, nil
	}
	if err = validateControlledProviderResultV2(request, result); err != nil {
		return runtimeports.ControlledOperationProviderResultV2{}, err
	}
	if result.Status == runtimeports.ControlledOperationProviderEnteredV2 || result.Status == runtimeports.ControlledOperationProviderUnknownV2 {
		return a.inspectAfterLostReply(ctx, request)
	}
	return result, nil
}

// InspectControlledProviderV2 only inspects the exact original Entry. It does
// not reread the route and cannot create or redispatch an Entry.
func (a *ControlledProviderV2) InspectControlledProviderV2(ctx context.Context, request runtimeports.ControlledOperationProviderRequestV2) (runtimeports.ControlledOperationProviderResultV2, error) {
	if isNilDependencyV2(ctx) {
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "controlled Provider V2 context is nil")
	}
	if err := request.Validate(); err != nil {
		return runtimeports.ControlledOperationProviderResultV2{}, err
	}
	if a == nil || isNilDependencyV2(a.gateway) {
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "controlled Provider V2 Gateway is unavailable")
	}
	key, err := runtimeports.DeriveControlledOperationProviderEntryKeyV2(request)
	if err != nil {
		return runtimeports.ControlledOperationProviderResultV2{}, err
	}
	result, err := a.gateway.InspectControlledOperationProviderV2(ctx, runtimeports.ControlledOperationProviderInspectRequestV2{Operation: request.Operation, Key: key})
	if err != nil {
		return runtimeports.ControlledOperationProviderResultV2{}, err
	}
	if err = validateControlledProviderResultV2(request, result); err != nil {
		return runtimeports.ControlledOperationProviderResultV2{}, err
	}
	return result, nil
}

func isNilDependencyV2(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func (a *ControlledProviderV2) inspectAfterLostReply(ctx context.Context, request runtimeports.ControlledOperationProviderRequestV2) (runtimeports.ControlledOperationProviderResultV2, error) {
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.recoveryTimeout)
	defer cancel()
	return a.InspectControlledProviderV2(recoveryCtx, request)
}

func validateControlledProviderResultV2(request runtimeports.ControlledOperationProviderRequestV2, result runtimeports.ControlledOperationProviderResultV2) error {
	if err := result.Validate(); err != nil {
		return err
	}
	key, err := runtimeports.DeriveControlledOperationProviderEntryKeyV2(request)
	if err != nil {
		return err
	}
	if result.EntryRef.EntryID != key.EntryID || result.EntryRef.StableKeyDigest != key.StableKeyDigest || result.Prepared != request.Prepared || result.Attempt != request.Attempt {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "controlled Provider V2 result binds another Entry or Attempt")
	}
	if result.AdmissionReceipt != nil && result.AdmissionReceipt.StableKeyDigest != key.StableKeyDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "controlled Provider V2 admission receipt binds another Entry")
	}
	if result.Observation != nil && (result.Observation.PreparedAttemptID != request.Prepared.ID || result.Observation.Delegation != request.PreparedSemantics.Delegation) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "controlled Provider V2 observation binds another Prepared Attempt")
	}
	return nil
}
