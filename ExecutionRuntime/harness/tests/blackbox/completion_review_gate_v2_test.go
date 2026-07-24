package blackbox_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
)

func TestBlackboxCompletionReviewGateV2StopsAtTypedPhaseDecision(t *testing.T) {
	for _, phase := range []struct {
		name string
		kind applicationcontract.ReviewPhasePointCoordinateV1
	}{
		{name: "subagent", kind: applicationcontract.ReviewPhasePointCoordinateV1{Kind: applicationcontract.ReviewPhaseSubagentV1}},
		{name: "run", kind: applicationcontract.ReviewPhasePointCoordinateV1{Kind: applicationcontract.ReviewPhaseRunV1}},
	} {
		t.Run(phase.name, func(t *testing.T) {
			fixture := testkit.CompletionReviewGateV2(t, phase.kind.Kind, "blackbox-"+phase.name)
			inputs := &testkit.CompletionReviewInputReaderV2{Values: map[applicationcontract.ReviewWaitingInputSubjectV1]applicationcontract.ReviewWaitingInputCurrentProjectionV1{fixture.Input.Subject: fixture.Input}}
			coordinator := &testkit.CompletionReviewCoordinatorV2{Values: map[string]applicationcontract.ReviewWaitingOutcomeV1{fixture.Request.Waiting.ID: fixture.Outcome}}
			gate, err := kernel.NewCompletionReviewGateControllerV2(inputs, coordinator, func() time.Time { return fixture.Now.Add(5 * time.Second) })
			if err != nil {
				t.Fatal(err)
			}
			result, err := gate.EvaluateCompletionReviewGateV2(context.Background(), fixture.Request)
			if err != nil || result.Outcome.Review.Decision != applicationcontract.ReviewPhaseAllowV1 || coordinator.Calls.Load() != 1 {
				t.Fatalf("result=%#v calls=%d err=%v", result, coordinator.Calls.Load(), err)
			}
			payload, _ := json.Marshal(result)
			for _, forbidden := range []string{"completion_claim", "runtime_outcome", "authorization_fact", "dispatch", "commit"} {
				if strings.Contains(string(payload), forbidden) {
					t.Fatalf("Gate observation leaked forbidden authority %q: %s", forbidden, payload)
				}
			}
		})
	}
}
