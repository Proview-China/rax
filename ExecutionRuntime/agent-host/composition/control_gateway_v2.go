package composition

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

type ControlGatewayV2 struct {
	registry    ports.ControlAdapterFactoryRegistryV2
	conformance ports.ControlAdapterConformanceReaderV2
	resources   ports.ControlAdapterResourceCurrentReaderV2
	clock       func() time.Time
}

func NewControlGatewayV2(registry ports.ControlAdapterFactoryRegistryV2, conformance ports.ControlAdapterConformanceReaderV2, resources ports.ControlAdapterResourceCurrentReaderV2, clock func() time.Time) (*ControlGatewayV2, error) {
	for name, dependency := range map[string]any{"registry": registry, "conformance reader": conformance, "resource reader": resources} {
		if contract.IsTypedNilV1(dependency) {
			return nil, contract.NewError(contract.ErrorInvalidArgument, "control_gateway_dependency_missing", name+" is required")
		}
	}
	if clock == nil {
		clock = time.Now
	}
	return &ControlGatewayV2{registry: registry, conformance: conformance, resources: resources, clock: clock}, nil
}

func (g *ControlGatewayV2) StartOrInspectControlAdapterConstructionV2(ctx context.Context, request contract.ControlAdapterConstructRequestV2) (contract.ControlAdapterInstanceV2, error) {
	return g.runV2(ctx, request, true)
}

func (g *ControlGatewayV2) InspectControlAdapterConstructionV2(ctx context.Context, request contract.ControlAdapterConstructRequestV2) (contract.ControlAdapterInstanceV2, error) {
	return g.runV2(ctx, request, false)
}

func (g *ControlGatewayV2) runV2(ctx context.Context, request contract.ControlAdapterConstructRequestV2, mayStart bool) (contract.ControlAdapterInstanceV2, error) {
	if g == nil || contract.IsTypedNilV1(g.registry) || contract.IsTypedNilV1(g.conformance) || contract.IsTypedNilV1(g.resources) {
		return contract.ControlAdapterInstanceV2{}, contract.NewError(contract.ErrorUnavailable, "control_gateway_missing", "control adapter construction gateway is unavailable")
	}
	if ctx == nil {
		return contract.ControlAdapterInstanceV2{}, contract.NewError(contract.ErrorInvalidArgument, "context_missing", "context is required")
	}
	now1, err := g.nextTimeV2(time.Time{})
	if err != nil {
		return contract.ControlAdapterInstanceV2{}, err
	}
	if err := request.ValidateCurrent(now1); err != nil {
		return contract.ControlAdapterInstanceV2{}, err
	}
	factory, err := g.registry.ResolveControlAdapterFactoryV2(request.Descriptor.Ref)
	if err != nil {
		return contract.ControlAdapterInstanceV2{}, err
	}
	if contract.IsTypedNilV1(factory) || factory.DescriptorV2().DescriptorDigest != request.Descriptor.DescriptorDigest {
		return contract.ControlAdapterInstanceV2{}, contract.NewError(contract.ErrorConflict, "control_adapter_factory_descriptor_drift", "registered executable factory descriptor drifted")
	}
	if err := g.inspectInputsV2(ctx, request, now1); err != nil {
		return contract.ControlAdapterInstanceV2{}, err
	}
	var handle ports.ControlAdapterHandleV2
	if mayStart {
		handle, err = safeStartControlV2(ctx, factory, request)
		if err != nil {
			handle, err = safeInspectControlV2(context.WithoutCancel(ctx), factory, request)
		}
	} else {
		handle, err = safeInspectControlV2(ctx, factory, request)
	}
	if err != nil {
		return contract.ControlAdapterInstanceV2{}, err
	}
	if contract.IsTypedNilV1(handle) {
		return contract.ControlAdapterInstanceV2{}, contract.NewError(contract.ErrorUnknownOutcome, "control_adapter_handle_missing", "control adapter result has no inspectable handle")
	}
	now2, err := g.nextTimeV2(now1)
	if err != nil {
		return contract.ControlAdapterInstanceV2{}, err
	}
	if err := g.inspectInputsV2(ctx, request, now2); err != nil {
		return contract.ControlAdapterInstanceV2{}, err
	}
	instance := handle.InstanceV2()
	if err := instance.ValidateCurrent(request, now2); err != nil {
		return contract.ControlAdapterInstanceV2{}, err
	}
	return instance, nil
}

func (g *ControlGatewayV2) inspectInputsV2(ctx context.Context, request contract.ControlAdapterConstructRequestV2, now time.Time) error {
	conformance, err := g.conformance.InspectControlAdapterConformanceV2(ctx, request.Descriptor.Ref)
	if err != nil {
		return err
	}
	if conformance.Digest != request.Conformance.Digest {
		return contract.NewError(contract.ErrorConflict, "control_adapter_conformance_drift", "control adapter conformance changed")
	}
	if err := conformance.ValidateCurrent(request.Descriptor.Ref, now); err != nil {
		return err
	}
	bindings, err := g.resources.InspectResourceBindingSetCurrentV1(ctx, request.Descriptor.ResourceBindingSet)
	if err != nil {
		return err
	}
	if bindings.ProjectionDigest != request.ResourceBindings.ProjectionDigest {
		return contract.NewError(contract.ErrorConflict, "control_adapter_resource_set_drift", "resource BindingSet changed")
	}
	if err := bindings.ValidateCurrent(request.Descriptor.ResourceBindingSet, now); err != nil {
		return err
	}
	for _, binding := range request.ResourceBindings.Bindings {
		current, inspectErr := g.resources.InspectResourceHandleCurrentV1(ctx, binding.Handle)
		if inspectErr != nil {
			return inspectErr
		}
		if current.CleanupContract != binding.CleanupContract || current.DeploymentAttestation != binding.DeploymentAttestation {
			return contract.NewError(contract.ErrorConflict, "control_adapter_resource_handle_drift", "resource handle owner proofs changed")
		}
		if validateErr := current.ValidateCurrent(binding.Handle, now); validateErr != nil {
			return validateErr
		}
	}
	return nil
}

func (g *ControlGatewayV2) nextTimeV2(previous time.Time) (time.Time, error) {
	now := g.clock()
	if now.IsZero() || (!previous.IsZero() && now.Before(previous)) {
		return time.Time{}, contract.NewError(contract.ErrorPrecondition, "clock_regression", "control adapter construction clock regressed")
	}
	return now, nil
}

func safeStartControlV2(ctx context.Context, factory ports.ControlAdapterFactoryV2, request contract.ControlAdapterConstructRequestV2) (result ports.ControlAdapterHandleV2, err error) {
	defer func() {
		if recover() != nil {
			result = nil
			err = contract.NewError(contract.ErrorUnknownOutcome, "control_adapter_start_panic", "control adapter construction outcome is unknown after panic")
		}
	}()
	return factory.StartOrInspectControlAdapterV2(ctx, request)
}

func safeInspectControlV2(ctx context.Context, factory ports.ControlAdapterFactoryV2, request contract.ControlAdapterConstructRequestV2) (result ports.ControlAdapterHandleV2, err error) {
	defer func() {
		if recover() != nil {
			result = nil
			err = contract.NewError(contract.ErrorUnavailable, "control_adapter_inspect_panic", "control adapter inspection panicked")
		}
	}()
	return factory.InspectControlAdapterV2(ctx, request)
}
