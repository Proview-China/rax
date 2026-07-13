package direct

import (
	"context"
	"fmt"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
)

type ModelStream interface {
	Next() bool
	Event() modelinvoker.StreamEvent
	Err() error
	Close() error
}

type Backend interface {
	Resolve(context.Context, modelinvoker.RouteCall) (routegateway.Resolution, error)
	Invoke(context.Context, modelinvoker.RouteCall) (routegateway.InvokeResult, error)
	OpenStream(context.Context, modelinvoker.RouteCall) (ModelStream, error)
}

type RouteGatewayBackend struct {
	Gateway *routegateway.Gateway
}

func (backend RouteGatewayBackend) Resolve(ctx context.Context, call modelinvoker.RouteCall) (routegateway.Resolution, error) {
	if backend.Gateway == nil {
		return routegateway.Resolution{}, fmt.Errorf("%w: route gateway is nil", ErrInvalidConfig)
	}
	return backend.Gateway.Resolve(ctx, call)
}

func (backend RouteGatewayBackend) Invoke(ctx context.Context, call modelinvoker.RouteCall) (routegateway.InvokeResult, error) {
	if backend.Gateway == nil {
		return routegateway.InvokeResult{}, fmt.Errorf("%w: route gateway is nil", ErrInvalidConfig)
	}
	return backend.Gateway.Invoke(ctx, call)
}

func (backend RouteGatewayBackend) OpenStream(ctx context.Context, call modelinvoker.RouteCall) (ModelStream, error) {
	if backend.Gateway == nil {
		return nil, fmt.Errorf("%w: route gateway is nil", ErrInvalidConfig)
	}
	return backend.Gateway.Stream(ctx, call)
}
