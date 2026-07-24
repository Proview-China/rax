package applicationadapter_test

import (
	"context"
	"testing"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/applicationadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type completionAdapterCoordinatorV2 struct {
	result applicationcontract.ReviewWaitingOutcomeV1
	err    error
	calls  int
}

func (c *completionAdapterCoordinatorV2) CoordinateReviewWaitingV1(context.Context, applicationcontract.ReviewWaitingRequestV1) (applicationcontract.ReviewWaitingOutcomeV1, error) {
	c.calls++
	return c.result.Clone(), c.err
}

func TestCompletionReviewWaitingAdapterV2ForwardsExactlyOnce(t *testing.T) {
	coordinator := &completionAdapterCoordinatorV2{err: core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "reply lost")}
	adapter, err := applicationadapter.NewCompletionReviewWaitingAdapterV2(coordinator)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.CoordinateReviewWaitingV1(context.Background(), applicationcontract.ReviewWaitingRequestV1{}); !core.HasCategory(err, core.ErrorIndeterminate) || coordinator.calls != 1 {
		t.Fatalf("mutation was retried or error changed: calls=%d err=%v", coordinator.calls, err)
	}
	var typedNil *completionAdapterCoordinatorV2
	if _, err := applicationadapter.NewCompletionReviewWaitingAdapterV2(typedNil); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil accepted: %v", err)
	}
}
