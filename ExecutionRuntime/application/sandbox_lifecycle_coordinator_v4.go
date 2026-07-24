package application

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type SandboxLifecycleCoordinatorV4 struct {
	lifecycle applicationports.SandboxLifecyclePortV4
	clock     func() time.Time
}

func NewSandboxLifecycleCoordinatorV4(lifecycle applicationports.SandboxLifecyclePortV4, clock func() time.Time) (*SandboxLifecycleCoordinatorV4, error) {
	if nilApplicationPortV4(lifecycle) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Sandbox lifecycle V4 port is required")
	}
	if clock == nil {
		clock = time.Now
	}
	return &SandboxLifecycleCoordinatorV4{lifecycle: lifecycle, clock: clock}, nil
}

func (c *SandboxLifecycleCoordinatorV4) CoordinateSandboxLifecycleV4(ctx context.Context, request contract.SandboxLifecycleRequestV4) (contract.SandboxLifecycleResultV4, error) {
	if c == nil || nilApplicationPortV4(ctx) {
		return contract.SandboxLifecycleResultV4{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Sandbox lifecycle coordinator or context is nil")
	}
	if err := request.ValidateCurrent(c.clock()); err != nil {
		return contract.SandboxLifecycleResultV4{}, err
	}
	result, err := c.lifecycle.StartOrInspectSandboxLifecycleV4(ctx, request)
	if err != nil {
		recovered, inspectErr := c.lifecycle.InspectSandboxLifecycleV4(context.WithoutCancel(ctx), request)
		if inspectErr != nil {
			return contract.SandboxLifecycleResultV4{}, err
		}
		result = recovered
	}
	if err := result.ValidateCurrent(c.clock()); err != nil || result.ID != request.ID || result.RequestDigest != request.Digest || result.Plan != request.Plan {
		return contract.SandboxLifecycleResultV4{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Sandbox lifecycle returned another request result")
	}
	return result, nil
}

func nilApplicationPortV4(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
