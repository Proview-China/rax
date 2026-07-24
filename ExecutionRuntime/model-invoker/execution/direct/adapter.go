package direct

import (
	"context"
	"fmt"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

func (adapter *Adapter) Preflight(ctx context.Context, invocation execution.Invocation) (execution.PreflightReport, error) {
	if adapter == nil {
		return execution.PreflightReport{}, ErrInvalidConfig
	}
	if err := invocation.Validate(); err != nil {
		return execution.PreflightReport{}, err
	}
	if invocation.Plan.ExecutionKind != union.ExecutionKindModel || invocation.Plan.Route.ID != string(adapter.config.RouteID) {
		return execution.PreflightReport{Accepted: false, RejectionCode: "direct_route_mismatch"}, nil
	}
	request, err := mapRequest(invocation, adapter.config)
	if err != nil {
		return execution.PreflightReport{Accepted: false, RejectionCode: "direct_mapping_rejected"}, nil
	}
	if len(request.Tools) != 0 && projectionRepositoryUnavailableV1(adapter.config.ToolCallObservationRepository) {
		return execution.PreflightReport{Accepted: false, RejectionCode: "direct_tool_call_observation_projection_unavailable"}, nil
	}
	if binding, governed, err := adapter.resolveGovernedBindingV1(ctx, invocation); err != nil {
		return execution.PreflightReport{}, fmt.Errorf("resolve governed Model invocation binding: %w", err)
	} else if governed {
		if request.Stream {
			return execution.PreflightReport{Accepted: false, RejectionCode: "direct_governed_stream_unsupported"}, nil
		}
		if err := adapter.cacheGovernedBindingV1(invocation.Request.ExecutionID, binding); err != nil {
			return execution.PreflightReport{}, err
		}
	} else {
		// Legacy Direct compatibility resolves eagerly. Governed Direct must
		// remain a pure preflight: RouteGateway performs Resolve/secret/pool/
		// provider preparation only after the Prepared Commit Gate succeeds.
		resolution, err := adapter.config.Backend.Resolve(ctx, adapter.routeCall(request))
		if err != nil {
			return execution.PreflightReport{}, fmt.Errorf("resolve direct Route: %w", err)
		}
		if resolution.Route.RouteID != adapter.config.RouteID || resolution.Route.Model != adapter.config.Model {
			return execution.PreflightReport{Accepted: false, RejectionCode: "direct_resolution_drift"}, nil
		}
	}
	actual, err := invocation.Plan.ExpectedManifest.Clone()
	if err != nil {
		return execution.PreflightReport{}, err
	}
	actual.ID = invocation.Plan.ExpectedManifest.ID + ".actual"
	actual.Digest = ""
	digest, err := actual.ComputeDigest()
	if err != nil {
		return execution.PreflightReport{}, err
	}
	actual.Digest = digest
	return execution.PreflightReport{Accepted: true, ActualManifest: actual}, nil
}

func (adapter *Adapter) Open(ctx context.Context, invocation execution.Invocation) (execution.Session, error) {
	if adapter == nil {
		return nil, ErrInvalidConfig
	}
	if err := invocation.Validate(); err != nil {
		return nil, err
	}
	request, err := mapRequest(invocation, adapter.config)
	if err != nil {
		return nil, err
	}
	if len(request.Tools) != 0 && projectionRepositoryUnavailableV1(adapter.config.ToolCallObservationRepository) {
		return nil, ErrToolCallObservationProjectionUnavailable
	}
	binding, governed, err := adapter.takeGovernedBindingV1(ctx, invocation)
	if err != nil {
		return nil, err
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	session := newSession(
		sessionCtx, cancel, adapter.config.Backend, adapter.routeCall(request), request, invocation.Request, invocation.Plan,
		adapter.config.ToolCallObservationRepository,
	)
	if governed {
		backend, ok := adapter.config.Backend.(GovernedBackendV1)
		if !ok {
			cancel()
			return nil, ErrGovernedInvocationUnavailable
		}
		result, invokeErr := backend.StartOrInspectGovernedModelInvocationV1(sessionCtx, modelinvoker.GovernedModelInvocationCommandV1{PreparedRef: binding.PreparedRef, CurrentRef: binding.CurrentRef, AttemptRequestDigest: binding.UnifiedRequestDigest, DispatchSequence: binding.DispatchSequence, ProviderAttemptOrdinal: binding.ProviderAttemptOrdinal, Call: session.call})
		if invokeErr != nil {
			cancel()
			return nil, fmt.Errorf("%w: %w", ErrGovernedInvocationUnavailable, invokeErr)
		}
		if err := result.Validate(); err != nil {
			cancel()
			return nil, fmt.Errorf("%w: %w", ErrGovernedInvocationUnavailable, err)
		}
		if result.Invocation.State != modelinvoker.GovernedModelInvocationObservedV1 || result.Observation == nil {
			cancel()
			return nil, ErrGovernedInvocationUnavailable
		}
		routeCallDigest, digestErr := modelinvoker.DigestGovernedRouteCallV1(session.call)
		if digestErr != nil || result.Invocation.PreparedRef != binding.PreparedRef || result.Invocation.CurrentRef != binding.CurrentRef || result.Invocation.AttemptRequestDigest != binding.UnifiedRequestDigest || result.Invocation.RouteCallDigest != routeCallDigest || result.Invocation.DispatchSequence != binding.DispatchSequence || result.Invocation.ProviderAttemptOrdinal != binding.ProviderAttemptOrdinal || result.Observation.RouteID != binding.RouteID || result.Observation.Model != adapter.config.Model {
			cancel()
			return nil, ErrGovernedInvocationUnavailable
		}
		if err := modelinvoker.ValidateGovernedStructuredOutputV1(session.call.Request.Output.Schema, result.Observation.StructuredOutput); err != nil {
			cancel()
			return nil, fmt.Errorf("%w: %w", ErrGovernedInvocationUnavailable, err)
		}
		response, responseErr := result.Observation.ResponseV1()
		if responseErr != nil {
			cancel()
			return nil, responseErr
		}
		session.acceptResponse(response)
		return session, nil
	}
	if request.Stream {
		stream, err := adapter.config.Backend.OpenStream(sessionCtx, session.call)
		if err != nil {
			cancel()
			return nil, err
		}
		session.stream = stream
		return session, nil
	}
	result, err := adapter.config.Backend.Invoke(sessionCtx, session.call)
	if err != nil {
		cancel()
		return nil, err
	}
	session.acceptResponse(result.Response)
	return session, nil
}

func (adapter *Adapter) routeCall(request modelinvoker.Request) modelinvoker.RouteCall {
	return modelinvoker.RouteCall{RouteID: adapter.config.RouteID, Invocation: adapter.config.Invocation, Request: request}
}

var _ execution.Adapter = (*Adapter)(nil)
