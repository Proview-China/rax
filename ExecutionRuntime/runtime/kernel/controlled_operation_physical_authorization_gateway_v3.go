package kernel

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ControlledOperationPhysicalAuthorizationGatewayV3 re-reads Runtime's V2
// governance closure and binds it to one exact domain-owned command. It never
// calls a Provider and does not persist a domain fact.
type ControlledOperationPhysicalAuthorizationGatewayV3 struct {
	provider     *ControlledOperationProviderGatewayV2
	associations ports.PreparedDomainCommandAssociationCurrentReaderV1
}

func NewControlledOperationPhysicalAuthorizationGatewayV3(
	provider *ControlledOperationProviderGatewayV2,
	associations ports.PreparedDomainCommandAssociationCurrentReaderV1,
) (*ControlledOperationPhysicalAuthorizationGatewayV3, error) {
	if provider == nil || nilPhysicalAuthorizationDependencyV3(associations) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "physical execution authorization dependencies are incomplete")
	}
	gateway := &ControlledOperationPhysicalAuthorizationGatewayV3{provider: provider, associations: associations}
	if err := gateway.validateDependencies(); err != nil {
		return nil, err
	}
	return gateway, nil
}

func (g *ControlledOperationPhysicalAuthorizationGatewayV3) AuthorizeControlledOperationPhysicalV3(ctx context.Context, request ports.ControlledOperationPhysicalAuthorizationRequestV3) (ports.ControlledOperationPhysicalExecutionAuthorizationV3, error) {
	if err := validatePhysicalAuthorizationContextV3(ctx); err != nil {
		return ports.ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}
	if g == nil {
		return ports.ControlledOperationPhysicalExecutionAuthorizationV3{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "physical execution authorization gateway is unavailable")
	}
	if err := g.validateDependencies(); err != nil {
		return ports.ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}

	nowS1 := g.provider.clock()
	if nowS1.IsZero() {
		return ports.ControlledOperationPhysicalExecutionAuthorizationV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "physical execution authorization S1 clock is zero")
	}
	closure, err := g.provider.readCurrentClosure(ctx, request.Provider, nowS1)
	if err != nil {
		return ports.ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}
	association, err := g.associations.InspectCurrentPreparedDomainCommandAssociationV1(ctx, request.Association)
	if err != nil {
		return ports.ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}
	nowS2 := g.provider.clock()
	if nowS2.IsZero() || nowS2.Before(nowS1) {
		return ports.ControlledOperationPhysicalExecutionAuthorizationV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "physical execution authorization clock regressed before S2")
	}
	if err := association.ValidateCurrent(request.Association, nowS2); err != nil {
		return ports.ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}
	if err := validatePhysicalAuthorizationAssociationV3(request, association); err != nil {
		return ports.ControlledOperationPhysicalExecutionAuthorizationV3{}, err
	}

	notAfter := minControlledProviderTimeV2(closure.notAfter, association.ExpiresUnixNano, request.Provider.CallerDeadlineUnixNano)
	if !nowS2.Before(time.Unix(0, notAfter)) {
		return ports.ControlledOperationPhysicalExecutionAuthorizationV3{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "physical execution authorization closure expired before issuance")
	}
	return ports.SealControlledOperationPhysicalExecutionAuthorizationV3(ports.ControlledOperationPhysicalExecutionAuthorizationV3{
		UnifiedNotAfterUnixNano: notAfter,
		ProviderTransport:       closure.route.ProviderTransportBinding,
		Provider:                request.Provider.ProviderBinding,
		Operation:               request.Provider.Operation,
		OperationDigest:         request.Provider.OperationDigest,
		OperationScopeDigest:    request.Provider.OperationScopeDigest,
		EffectKind:              request.Provider.EffectKind,
		Prepared:                request.Provider.Prepared,
		Attempt:                 request.Provider.Attempt,
		ExecuteEnforcement:      request.Provider.ExecuteEnforcement,
		ExecuteEvidenceHandoff:  request.Provider.ExecuteEvidenceHandoff,
		Boundary:                request.Provider.Boundary,
		Association:             request.Association,
		DomainCommand:           request.DomainCommand,
	})
}

func (g *ControlledOperationPhysicalAuthorizationGatewayV3) validateDependencies() error {
	if g == nil || g.provider == nil || nilPhysicalAuthorizationDependencyV3(g.associations) || nilPhysicalAuthorizationDependencyV3(g.provider.Routes) || nilPhysicalAuthorizationDependencyV3(g.provider.RouteInputs) || nilPhysicalAuthorizationDependencyV3(g.provider.Bindings) || nilPhysicalAuthorizationDependencyV3(g.provider.Effects) || nilPhysicalAuthorizationDependencyV3(g.provider.Prepared) || nilPhysicalAuthorizationDependencyV3(g.provider.Policies) || nilPhysicalAuthorizationDependencyV3(g.provider.Enforcement) || nilPhysicalAuthorizationDependencyV3(g.provider.Handoff) || nilPhysicalAuthorizationDependencyV3(g.provider.Evidence) || nilPhysicalAuthorizationDependencyV3(g.provider.Boundary) || g.provider.clock == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "physical execution authorization current readers are unavailable")
	}
	return nil
}

func validatePhysicalAuthorizationAssociationV3(request ports.ControlledOperationPhysicalAuthorizationRequestV3, association ports.PreparedDomainCommandAssociationCurrentProjectionV1) error {
	provider := request.Provider
	if association.Ref != request.Association || association.DomainCommand != request.DomainCommand {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "physical execution association binds another ref or domain command")
	}
	if !ports.SameOperationSubjectV3(association.Operation, provider.Operation) || association.OperationDigest != provider.OperationDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "physical execution association binds another operation")
	}
	// Association.EffectRevision is the immutable Intent revision carried by
	// Attempt/Prepared. Provider.EffectRevision is the mutable Runtime Effect
	// fact revision and therefore is deliberately not type-punned here.
	if association.EffectID != provider.EffectID || association.EffectRevision != provider.Attempt.IntentRevision || association.IntentDigest != provider.IntentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "physical execution association binds another Effect")
	}
	if association.Prepared != provider.Prepared || association.Attempt != provider.Attempt {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "physical execution association binds another Prepared or Attempt")
	}
	if association.Provider != provider.ProviderBinding {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "physical execution association binds another Provider")
	}
	if association.PayloadSchema != provider.Prepared.PayloadSchema || association.PayloadDigest != provider.Prepared.PayloadDigest || association.PayloadRevision != provider.Prepared.PayloadRevision {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "physical execution association binds another payload")
	}
	return nil
}

func validatePhysicalAuthorizationContextV3(ctx context.Context) error {
	if ctx == nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "physical execution authorization context is nil")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func nilPhysicalAuthorizationDependencyV3(value any) bool {
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

var _ ports.ControlledOperationPhysicalAuthorizationPortV3 = (*ControlledOperationPhysicalAuthorizationGatewayV3)(nil)
