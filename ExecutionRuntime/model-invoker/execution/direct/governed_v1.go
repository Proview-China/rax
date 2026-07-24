package direct

import (
	"context"
	"fmt"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func (adapter *Adapter) resolveGovernedBindingV1(ctx context.Context, invocation execution.Invocation) (modelinvoker.GovernedModelInvocationBindingV1, bool, error) {
	if adapter == nil || adapter.config.GovernedInvocationBindings == nil {
		return modelinvoker.GovernedModelInvocationBindingV1{}, false, nil
	}
	request, err := governedBindingRequestV1(invocation, adapter.config.RouteID)
	if err != nil {
		return modelinvoker.GovernedModelInvocationBindingV1{}, true, err
	}
	binding, err := adapter.config.GovernedInvocationBindings.InspectExactGovernedModelInvocationBindingV1(ctx, request)
	if err != nil {
		return modelinvoker.GovernedModelInvocationBindingV1{}, true, err
	}
	if err := binding.ValidateAgainstV1(request); err != nil {
		return modelinvoker.GovernedModelInvocationBindingV1{}, true, err
	}
	return binding, true, nil
}

func governedBindingRequestV1(invocation execution.Invocation, routeID upstream.RouteID) (modelinvoker.GovernedModelInvocationBindingRequestV1, error) {
	requestDigest, err := invocation.Request.Digest()
	if err != nil {
		return modelinvoker.GovernedModelInvocationBindingRequestV1{}, err
	}
	request := modelinvoker.GovernedModelInvocationBindingRequestV1{ExecutionID: string(invocation.Request.ExecutionID), UnifiedRequestDigest: core.Digest(requestDigest), PreparedPlanDigest: core.Digest(invocation.Plan.Digest), RouteID: routeID}
	if err := request.Validate(); err != nil {
		return modelinvoker.GovernedModelInvocationBindingRequestV1{}, err
	}
	return request, nil
}

func (adapter *Adapter) cacheGovernedBindingV1(executionID union.ExecutionID, binding modelinvoker.GovernedModelInvocationBindingV1) error {
	adapter.governedMu.Lock()
	defer adapter.governedMu.Unlock()
	if existing, ok := adapter.governedPrepared[executionID]; ok && existing != binding {
		return fmt.Errorf("%w: governed binding changed during preflight", ErrInvalidConfig)
	}
	adapter.governedPrepared[executionID] = binding
	return nil
}

func (adapter *Adapter) takeGovernedBindingV1(ctx context.Context, invocation execution.Invocation) (modelinvoker.GovernedModelInvocationBindingV1, bool, error) {
	resolved, governed, err := adapter.resolveGovernedBindingV1(ctx, invocation)
	if err != nil || !governed {
		return resolved, governed, err
	}
	adapter.governedMu.Lock()
	prepared, ok := adapter.governedPrepared[invocation.Request.ExecutionID]
	if ok {
		delete(adapter.governedPrepared, invocation.Request.ExecutionID)
	}
	adapter.governedMu.Unlock()
	if !ok || prepared != resolved {
		return modelinvoker.GovernedModelInvocationBindingV1{}, true, fmt.Errorf("%w: governed binding S1/S2 drifted or preflight binding is absent", ErrGovernedInvocationUnavailable)
	}
	return resolved, true, nil
}

func (adapter *Adapter) ClosePrepared(executionID union.ExecutionID) error {
	if adapter == nil {
		return ErrInvalidConfig
	}
	adapter.governedMu.Lock()
	delete(adapter.governedPrepared, executionID)
	adapter.governedMu.Unlock()
	return nil
}

var _ execution.PreflightCleaner = (*Adapter)(nil)
