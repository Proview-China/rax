package integration_test

import (
	"context"
	"testing"
	"time"

	application "github.com/Proview-China/rax/ExecutionRuntime/application"
	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationfakes "github.com/Proview-China/rax/ExecutionRuntime/application/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/applicationadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
)

type completionReviewOwnerV2 struct {
	current applicationcontract.ReviewWaitingCurrentProjectionV1
	starts  int
	reads   int
}

func (o *completionReviewOwnerV2) StartOrInspectReviewV1(context.Context, applicationcontract.ReviewWaitingRequestV1) (applicationcontract.ReviewWaitingCurrentProjectionV1, error) {
	o.starts++
	return o.current.Clone(), nil
}

func (o *completionReviewOwnerV2) InspectReviewV1(context.Context, applicationcontract.ReviewWaitingInspectRequestV1) (applicationcontract.ReviewWaitingCurrentProjectionV1, error) {
	o.reads++
	return o.current.Clone(), nil
}

func TestIntegrationCompletionReviewGateV2UsesApplicationDurableWaitingBoundary(t *testing.T) {
	for _, phase := range []struct {
		name string
		kind applicationcontract.ReviewPhasePointCoordinateV1
	}{
		{name: "subagent", kind: applicationcontract.ReviewPhasePointCoordinateV1{Kind: applicationcontract.ReviewPhaseSubagentV1}},
		{name: "run", kind: applicationcontract.ReviewPhasePointCoordinateV1{Kind: applicationcontract.ReviewPhaseRunV1}},
	} {
		t.Run(phase.name, func(t *testing.T) {
			fixture := testkit.CompletionReviewGateV2(t, phase.kind.Kind, "integration-"+phase.name)
			now := fixture.Now.Add(5 * time.Second)
			inputs := &testkit.CompletionReviewInputReaderV2{Values: map[applicationcontract.ReviewWaitingInputSubjectV1]applicationcontract.ReviewWaitingInputCurrentProjectionV1{fixture.Input.Subject: fixture.Input}}
			owner := &completionReviewOwnerV2{current: fixture.Outcome.Review}
			facts := applicationfakes.NewReviewWaitingStoreV1(func() time.Time { return now })
			coordinator, err := application.NewReviewWaitingCoordinatorV1(application.ReviewWaitingCoordinatorConfigV1{Facts: facts, Inputs: inputs, Review: owner, Clock: func() time.Time { return now }, ClaimID: func() (string, error) { return "completion-review-claim-" + phase.name, nil }})
			if err != nil {
				t.Fatal(err)
			}
			adapter, err := applicationadapter.NewCompletionReviewWaitingAdapterV2(coordinator)
			if err != nil {
				t.Fatal(err)
			}
			gate, err := kernel.NewCompletionReviewGateControllerV2(inputs, adapter, func() time.Time { return now })
			if err != nil {
				t.Fatal(err)
			}
			result, err := gate.EvaluateCompletionReviewGateV2(context.Background(), fixture.Request)
			if err != nil || result.Outcome.Review.Decision != applicationcontract.ReviewPhaseAllowV1 || result.Outcome.Receipt == nil || owner.starts != 1 || owner.reads != 1 {
				t.Fatalf("result=%#v starts=%d reads=%d err=%v", result, owner.starts, owner.reads, err)
			}
			stored, err := facts.InspectCurrentReviewWaitingCoordinationV1(context.Background(), fixture.Request.Waiting.ExecutionScope, fixture.Request.Waiting.ID)
			if err != nil || stored.State != applicationcontract.ReviewCompletedStateV1 || stored.Receipt == nil || stored.Receipt.Digest != result.Outcome.Receipt.Digest {
				t.Fatalf("coordination=%#v err=%v", stored, err)
			}
		})
	}
}

func TestIntegrationCompletionReviewGateV2ClaimLostReplyRemainsInspectOnly(t *testing.T) {
	fixture := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseRunV1, "integration-lost-claim")
	now := fixture.Now.Add(5 * time.Second)
	inputs := &testkit.CompletionReviewInputReaderV2{Values: map[applicationcontract.ReviewWaitingInputSubjectV1]applicationcontract.ReviewWaitingInputCurrentProjectionV1{fixture.Input.Subject: fixture.Input}}
	owner := &completionReviewOwnerV2{current: fixture.Outcome.Review}
	facts := applicationfakes.NewReviewWaitingStoreV1(func() time.Time { return now })
	facts.LoseNextCASReplyV1()
	coordinator, err := application.NewReviewWaitingCoordinatorV1(application.ReviewWaitingCoordinatorConfigV1{Facts: facts, Inputs: inputs, Review: owner, Clock: func() time.Time { return now }, ClaimID: func() (string, error) { return "completion-review-lost-claim", nil }})
	if err != nil {
		t.Fatal(err)
	}
	adapter, _ := applicationadapter.NewCompletionReviewWaitingAdapterV2(coordinator)
	gate, _ := kernel.NewCompletionReviewGateControllerV2(inputs, adapter, func() time.Time { return now })
	result, err := gate.EvaluateCompletionReviewGateV2(context.Background(), fixture.Request)
	if err != nil || result.Outcome.Review.Decision != applicationcontract.ReviewPhaseAllowV1 || owner.starts != 0 || owner.reads != 2 {
		t.Fatalf("lost-claim result=%#v starts=%d reads=%d err=%v", result, owner.starts, owner.reads, err)
	}
}
