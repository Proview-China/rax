package sdk

import (
	"context"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// SingleCallToolActionClientV2 is a narrow SDK facade over the Application
// Owner's governed start-or-inspect port. It accepts only an already-sealed
// Application request; it cannot assemble PendingAction, call a Provider, or
// retry an unknown physical effect.
type SingleCallToolActionClientV2 struct {
	port  applicationports.SingleCallToolActionPortV2
	clock ClockV1
}

func NewSingleCallToolActionClientV2(port applicationports.SingleCallToolActionPortV2, clock ClockV1) (*SingleCallToolActionClientV2, error) {
	if nilLikeV1(port) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Tool Action SDK dependencies are required")
	}
	return &SingleCallToolActionClientV2{port: port, clock: clock}, nil
}

func (c *SingleCallToolActionClientV2) ExecuteSingleCallToolActionV2(ctx context.Context, request applicationcontract.SingleCallToolActionRequestV2) (applicationcontract.SingleCallToolActionResultV2, error) {
	first, err := c.readyV2(ctx)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	if err := request.ValidateCurrent(first); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	result, err := c.port.ExecuteSingleCallToolActionV2(ctx, request)
	if err != nil {
		// The Application port owns start-or-inspect recovery. The SDK never
		// converts Unavailable/Unknown into NotFound and never retries Execute.
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	return c.validateResultV2(ctx, request, result, first)
}

func (c *SingleCallToolActionClientV2) InspectSingleCallToolActionV2(ctx context.Context, request applicationcontract.SingleCallToolActionRequestV2) (applicationcontract.SingleCallToolActionResultV2, error) {
	first, err := c.readyV2(ctx)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	if err := request.ValidateCurrent(first); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	key, err := applicationcontract.SealSingleCallToolActionInspectKeyV2(request)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	result, err := c.port.InspectSingleCallToolActionV2(ctx, key)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	return c.validateResultV2(ctx, request, result, first)
}

func (c *SingleCallToolActionClientV2) validateResultV2(ctx context.Context, request applicationcontract.SingleCallToolActionRequestV2, result applicationcontract.SingleCallToolActionResultV2, previous time.Time) (applicationcontract.SingleCallToolActionResultV2, error) {
	now, err := c.readyV2(ctx)
	if err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	if now.Before(previous) {
		return applicationcontract.SingleCallToolActionResultV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Action SDK clock regressed")
	}
	if err := result.ValidateCurrentFor(request, now); err != nil {
		return applicationcontract.SingleCallToolActionResultV2{}, err
	}
	return applicationcontract.CloneSingleCallToolActionResultV2(result), nil
}

func (c *SingleCallToolActionClientV2) readyV2(ctx context.Context) (time.Time, error) {
	if c == nil || nilLikeV1(c.port) || c.clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Tool Action SDK is unavailable")
	}
	if ctx == nil {
		return time.Time{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Tool Action SDK context is required")
	}
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	now := c.clock()
	if now.IsZero() {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Tool Action SDK clock is unavailable")
	}
	return now.UTC(), nil
}
