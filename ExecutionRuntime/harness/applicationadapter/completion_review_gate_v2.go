package applicationadapter

import (
	"context"
	"reflect"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// CompletionReviewWaitingAdapterV2 narrows the Application coordinator to the
// one method needed by Harness. It never retries: unknown mutation outcomes are
// recovered by the Application Owner against its own persisted state.
type CompletionReviewWaitingAdapterV2 struct {
	coordinator harnessports.CompletionReviewWaitingCoordinatorV2
}

func NewCompletionReviewWaitingAdapterV2(coordinator harnessports.CompletionReviewWaitingCoordinatorV2) (*CompletionReviewWaitingAdapterV2, error) {
	if completionReviewAdapterNilV2(coordinator) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Application Review waiting coordinator is required")
	}
	return &CompletionReviewWaitingAdapterV2{coordinator: coordinator}, nil
}

func (a *CompletionReviewWaitingAdapterV2) CoordinateReviewWaitingV1(ctx context.Context, request applicationcontract.ReviewWaitingRequestV1) (applicationcontract.ReviewWaitingOutcomeV1, error) {
	return a.coordinator.CoordinateReviewWaitingV1(ctx, request)
}

func completionReviewAdapterNilV2(value any) bool {
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

var _ harnessports.CompletionReviewWaitingCoordinatorV2 = (*CompletionReviewWaitingAdapterV2)(nil)
