package kernel

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type CompletionReviewGateControllerV2 struct {
	inputs      bridgecontract.CompletionReviewInputCurrentReaderV2
	coordinator harnessports.CompletionReviewWaitingCoordinatorV2
	clock       func() time.Time
}

func NewCompletionReviewGateControllerV2(inputs bridgecontract.CompletionReviewInputCurrentReaderV2, coordinator harnessports.CompletionReviewWaitingCoordinatorV2, clock func() time.Time) (*CompletionReviewGateControllerV2, error) {
	if completionReviewGateNilV2(inputs) || completionReviewGateNilV2(coordinator) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "completion Review Gate read-only input and Application coordinator are required")
	}
	return &CompletionReviewGateControllerV2{inputs: inputs, coordinator: coordinator, clock: clock}, nil
}

func (c *CompletionReviewGateControllerV2) EvaluateCompletionReviewGateV2(ctx context.Context, request bridgecontract.CompletionReviewGateRequestV2) (bridgecontract.CompletionReviewGateResultV2, error) {
	if ctx == nil {
		return bridgecontract.CompletionReviewGateResultV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "completion Review Gate context is required")
	}
	baseline, err := c.nowAfterV2(time.Time{})
	if err != nil {
		return bridgecontract.CompletionReviewGateResultV2{}, err
	}
	if err := request.ValidateCurrent(baseline); err != nil {
		return bridgecontract.CompletionReviewGateResultV2{}, err
	}

	inputS1, err := c.inspectInputV2(ctx, request.Waiting.InputSubjectV1())
	if err != nil {
		return bridgecontract.CompletionReviewGateResultV2{}, err
	}
	afterS1, err := c.nowAfterV2(baseline)
	if err != nil {
		return bridgecontract.CompletionReviewGateResultV2{}, err
	}
	if err := inputS1.ValidateFor(request.Waiting, afterS1); err != nil {
		return bridgecontract.CompletionReviewGateResultV2{}, err
	}
	if err := ctx.Err(); err != nil {
		return bridgecontract.CompletionReviewGateResultV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "completion Review Gate caller ended before Application coordination")
	}

	// This mutation-capable boundary is called exactly once. Application owns
	// write-ahead, Inspect-only unknown recovery and its durable coordination.
	outcome, err := c.coordinator.CoordinateReviewWaitingV1(ctx, request.Waiting)
	if err != nil {
		return bridgecontract.CompletionReviewGateResultV2{}, err
	}
	afterCoordinator, err := c.nowAfterV2(afterS1)
	if err != nil {
		return bridgecontract.CompletionReviewGateResultV2{}, err
	}
	if err := outcome.ValidateFor(request.Waiting, afterCoordinator); err != nil {
		return bridgecontract.CompletionReviewGateResultV2{}, err
	}

	inputS2, err := c.inspectInputV2(ctx, request.Waiting.InputSubjectV1())
	if err != nil {
		return bridgecontract.CompletionReviewGateResultV2{}, err
	}
	now, err := c.nowAfterV2(afterCoordinator)
	if err != nil {
		return bridgecontract.CompletionReviewGateResultV2{}, err
	}
	if err := inputS2.ValidateFor(request.Waiting, now); err != nil {
		return bridgecontract.CompletionReviewGateResultV2{}, err
	}
	if !reflect.DeepEqual(inputS1, inputS2) {
		return bridgecontract.CompletionReviewGateResultV2{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "completion Review Gate input drifted across Application coordination")
	}
	if err := outcome.ValidateFor(request.Waiting, now); err != nil {
		return bridgecontract.CompletionReviewGateResultV2{}, err
	}
	return bridgecontract.NewCompletionReviewGateResultV2(request, inputS2, outcome, now)
}

func (c *CompletionReviewGateControllerV2) inspectInputV2(ctx context.Context, subject bridgecontract.ReviewWaitingInputSubjectV1) (bridgecontract.ReviewWaitingInputCurrentProjectionV1, error) {
	value, err := c.inputs.InspectReviewWaitingInputCurrentV1(ctx, subject)
	if completionReviewGateRetryableReadV2(err) {
		return c.inputs.InspectReviewWaitingInputCurrentV1(context.WithoutCancel(ctx), subject)
	}
	return value, err
}

func (c *CompletionReviewGateControllerV2) nowAfterV2(previous time.Time) (time.Time, error) {
	now := c.clock()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "completion Review Gate clock is zero or moved backwards")
	}
	return now, nil
}

func completionReviewGateRetryableReadV2(err error) bool {
	return err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate))
}

func completionReviewGateNilV2(value any) bool {
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

var _ harnessports.CompletionReviewGateV2 = (*CompletionReviewGateControllerV2)(nil)
