package conformance_test

import (
	"context"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
)

func TestReusableCompletionReviewGateConformanceV2(t *testing.T) {
	subagent := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseSubagentV1, "suite-subagent")
	run := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseRunV1, "suite-run")
	inputs := &testkit.CompletionReviewInputReaderV2{Values: map[applicationcontract.ReviewWaitingInputSubjectV1]applicationcontract.ReviewWaitingInputCurrentProjectionV1{subagent.Input.Subject: subagent.Input, run.Input.Subject: run.Input}}
	coordinator := &testkit.CompletionReviewCoordinatorV2{Values: map[string]applicationcontract.ReviewWaitingOutcomeV1{subagent.Request.Waiting.ID: subagent.Outcome, run.Request.Waiting.ID: run.Outcome}}
	gate, err := kernel.NewCompletionReviewGateControllerV2(inputs, coordinator, func() time.Time { return run.Now.Add(5 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	report, err := conformance.CheckCompletionReviewGateV2(context.Background(), conformance.CompletionReviewGateCaseV2{Gate: gate, Subagent: subagent.Request, Run: run.Request, Now: run.Now.Add(5 * time.Second), Concurrent: 64})
	if err != nil || !report.SubagentCompletionExact || !report.RunCompletionExact || !report.ConcurrentReadExact || report.CompletionClaimWriter || report.RuntimeOutcomeWriter || report.VerdictWriter || report.AuthorizationWriter || report.ProductionRootProven {
		t.Fatalf("report=%#v err=%v", report, err)
	}
}
