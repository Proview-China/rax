package fault_test

import (
	"context"
	"sync"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type driftingCompletionInputV2 struct {
	mu     sync.Mutex
	values []applicationcontract.ReviewWaitingInputCurrentProjectionV1
	calls  int
}

func (r *driftingCompletionInputV2) InspectReviewWaitingInputCurrentV1(_ context.Context, _ applicationcontract.ReviewWaitingInputSubjectV1) (applicationcontract.ReviewWaitingInputCurrentProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	index := r.calls
	if index >= len(r.values) {
		index = len(r.values) - 1
	}
	r.calls++
	return r.values[index], nil
}

func TestFaultCompletionReviewGateV2DriftUnknownAndExpiredNeverAllow(t *testing.T) {
	fixture := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseRunV1, "fault")
	drift := fixture.Input
	drift.ExpiresUnixNano--
	drift, _ = applicationcontract.SealReviewWaitingInputCurrentProjectionV1(drift)
	coordinator := &testkit.CompletionReviewCoordinatorV2{Values: map[string]applicationcontract.ReviewWaitingOutcomeV1{fixture.Request.Waiting.ID: fixture.Outcome}}
	gate, _ := kernel.NewCompletionReviewGateControllerV2(&driftingCompletionInputV2{values: []applicationcontract.ReviewWaitingInputCurrentProjectionV1{fixture.Input, drift}}, coordinator, func() time.Time { return fixture.Now.Add(5 * time.Second) })
	if result, err := gate.EvaluateCompletionReviewGateV2(context.Background(), fixture.Request); err == nil || result != (bridgecontract.CompletionReviewGateResultV2{}) {
		t.Fatalf("S1/S2 drift allowed: result=%#v err=%v", result, err)
	}

	unknown := &testkit.CompletionReviewCoordinatorV2{Err: core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Application reply lost")}
	gate, _ = kernel.NewCompletionReviewGateControllerV2(&testkit.CompletionReviewInputReaderV2{Values: map[applicationcontract.ReviewWaitingInputSubjectV1]applicationcontract.ReviewWaitingInputCurrentProjectionV1{fixture.Input.Subject: fixture.Input}}, unknown, func() time.Time { return fixture.Now.Add(5 * time.Second) })
	if result, err := gate.EvaluateCompletionReviewGateV2(context.Background(), fixture.Request); !core.HasCategory(err, core.ErrorIndeterminate) || result != (bridgecontract.CompletionReviewGateResultV2{}) || unknown.Calls.Load() != 1 {
		t.Fatalf("unknown mutation was retried or allowed: result=%#v calls=%d err=%v", result, unknown.Calls.Load(), err)
	}

	expired := fixture
	gate, _ = kernel.NewCompletionReviewGateControllerV2(&testkit.CompletionReviewInputReaderV2{Values: map[applicationcontract.ReviewWaitingInputSubjectV1]applicationcontract.ReviewWaitingInputCurrentProjectionV1{expired.Input.Subject: expired.Input}}, coordinator, func() time.Time { return time.Unix(0, expired.Input.ExpiresUnixNano) })
	if result, err := gate.EvaluateCompletionReviewGateV2(context.Background(), expired.Request); err == nil || result != (bridgecontract.CompletionReviewGateResultV2{}) {
		t.Fatalf("expired input allowed: result=%#v err=%v", result, err)
	}
}
