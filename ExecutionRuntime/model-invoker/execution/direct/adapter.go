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
	resolution, err := adapter.config.Backend.Resolve(ctx, adapter.routeCall(request))
	if err != nil {
		return execution.PreflightReport{}, fmt.Errorf("resolve direct Route: %w", err)
	}
	if resolution.Route.RouteID != adapter.config.RouteID || resolution.Route.Model != adapter.config.Model {
		return execution.PreflightReport{Accepted: false, RejectionCode: "direct_resolution_drift"}, nil
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
	sessionCtx, cancel := context.WithCancel(ctx)
	session := newSession(sessionCtx, cancel, adapter.config.Backend, adapter.routeCall(request), request, invocation.Request, invocation.Plan)
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
