package kernel_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type completionInputReaderV2 struct {
	mu                     sync.Mutex
	values                 []applicationcontract.ReviewWaitingInputCurrentProjectionV1
	err                    error
	failOnce               error
	calls                  int
	detachedRecoveryCalled bool
}

func (r *completionInputReaderV2) InspectReviewWaitingInputCurrentV1(ctx context.Context, subject applicationcontract.ReviewWaitingInputSubjectV1) (applicationcontract.ReviewWaitingInputCurrentProjectionV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.failOnce != nil {
		err := r.failOnce
		r.failOnce = nil
		return applicationcontract.ReviewWaitingInputCurrentProjectionV1{}, err
	}
	if r.err != nil {
		return applicationcontract.ReviewWaitingInputCurrentProjectionV1{}, r.err
	}
	if len(r.values) == 0 {
		return applicationcontract.ReviewWaitingInputCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "completion input absent")
	}
	index := r.calls - 1
	if index >= len(r.values) {
		index = len(r.values) - 1
	}
	value := r.values[index]
	if value.Subject != subject {
		return applicationcontract.ReviewWaitingInputCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "completion input subject drifted")
	}
	if ctx.Err() == nil && r.calls > 1 {
		r.detachedRecoveryCalled = true
	}
	return value, nil
}

type completionCoordinatorV2 struct {
	mu      sync.Mutex
	outcome applicationcontract.ReviewWaitingOutcomeV1
	err     error
	calls   int
}

func (c *completionCoordinatorV2) CoordinateReviewWaitingV1(context.Context, applicationcontract.ReviewWaitingRequestV1) (applicationcontract.ReviewWaitingOutcomeV1, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return c.outcome.Clone(), c.err
}

func (c *completionCoordinatorV2) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func TestCompletionReviewGateV2CoversBothNamespacedCompletionPhases(t *testing.T) {
	for _, phase := range []struct {
		name string
		kind applicationcontract.ReviewPhasePointCoordinateV1
	}{
		{"subagent", applicationcontract.ReviewPhasePointCoordinateV1{Kind: applicationcontract.ReviewPhaseSubagentV1}},
		{"run", applicationcontract.ReviewPhasePointCoordinateV1{Kind: applicationcontract.ReviewPhaseRunV1}},
	} {
		t.Run(phase.name, func(t *testing.T) {
			fixture := testkit.CompletionReviewGateV2(t, phase.kind.Kind, phase.name)
			reader := &completionInputReaderV2{values: []applicationcontract.ReviewWaitingInputCurrentProjectionV1{fixture.Input}}
			coordinator := &completionCoordinatorV2{outcome: fixture.Outcome}
			controller, err := kernel.NewCompletionReviewGateControllerV2(reader, coordinator, func() time.Time { return fixture.Now.Add(5 * time.Second) })
			if err != nil {
				t.Fatal(err)
			}
			result, err := controller.EvaluateCompletionReviewGateV2(context.Background(), fixture.Request)
			if err != nil || result.Outcome.Review.Decision != applicationcontract.ReviewPhaseAllowV1 || result.Input != fixture.Input || result.Outcome.Receipt == nil || result.Outcome.Receipt.Digest != fixture.Outcome.Receipt.Digest {
				t.Fatalf("phase=%s result=%#v err=%v", phase.kind.Kind, result, err)
			}
		})
	}
}

func TestCompletionReviewGateV2PreservesApplicationDecisionClosedSet(t *testing.T) {
	for _, decision := range []applicationcontract.ReviewPhaseDecisionV1{applicationcontract.ReviewPhaseAllowV1, applicationcontract.ReviewPhaseDenyV1, applicationcontract.ReviewPhaseAskV1, applicationcontract.ReviewPhaseDeferV1} {
		t.Run(string(decision), func(t *testing.T) {
			fixture := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseRunV1, "decision-"+string(decision))
			current := fixture.Outcome.Review.Clone()
			current.Decision = decision
			if decision == applicationcontract.ReviewPhaseDeferV1 {
				current.Verdict = nil
			}
			current, err := applicationcontract.SealReviewWaitingCurrentProjectionV1(current)
			if err != nil {
				t.Fatal(err)
			}
			outcome := applicationcontract.ReviewWaitingOutcomeV1{Coordination: fixture.Outcome.Coordination, Review: current}
			if decision != applicationcontract.ReviewPhaseDeferV1 {
				receipt, err := applicationcontract.SealReviewPhaseReceiptV1(applicationcontract.ReviewPhaseReceiptV1{Coordination: fixture.Outcome.Receipt.Coordination}, fixture.Request.Waiting, current, fixture.Input, fixture.Now.Add(4*time.Second))
				if err != nil {
					t.Fatal(err)
				}
				outcome.Receipt = &receipt
			}
			reader := &completionInputReaderV2{values: []applicationcontract.ReviewWaitingInputCurrentProjectionV1{fixture.Input}}
			controller, _ := kernel.NewCompletionReviewGateControllerV2(reader, &completionCoordinatorV2{outcome: outcome}, func() time.Time { return fixture.Now.Add(5 * time.Second) })
			result, err := controller.EvaluateCompletionReviewGateV2(context.Background(), fixture.Request)
			if err != nil || result.Outcome.Review.Decision != decision || (decision == applicationcontract.ReviewPhaseDeferV1) != (result.Outcome.Receipt == nil) {
				t.Fatalf("decision=%s result=%#v err=%v", decision, result, err)
			}
		})
	}
}

func TestCompletionReviewGateV2S1S2DriftTTLAndRollbackFailClosed(t *testing.T) {
	t.Run("input-drift", func(t *testing.T) {
		fixture := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseRunV1, "drift")
		drift := fixture.Input
		drift.ExpiresUnixNano--
		drift, _ = applicationcontract.SealReviewWaitingInputCurrentProjectionV1(drift)
		reader := &completionInputReaderV2{values: []applicationcontract.ReviewWaitingInputCurrentProjectionV1{fixture.Input, drift}}
		controller, _ := kernel.NewCompletionReviewGateControllerV2(reader, &completionCoordinatorV2{outcome: fixture.Outcome}, func() time.Time { return fixture.Now.Add(5 * time.Second) })
		if result, err := controller.EvaluateCompletionReviewGateV2(context.Background(), fixture.Request); err == nil || result != (bridgecontract.CompletionReviewGateResultV2{}) {
			t.Fatalf("drift accepted: result=%#v err=%v", result, err)
		}
	})
	t.Run("ttl-crossing", func(t *testing.T) {
		fixture := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseRunV1, "ttl")
		times := []time.Time{fixture.Now.Add(5 * time.Second), fixture.Now.Add(6 * time.Second), fixture.Now.Add(7 * time.Second), time.Unix(0, fixture.Input.ExpiresUnixNano)}
		var index int
		clock := func() time.Time {
			value := times[index]
			if index < len(times)-1 {
				index++
			}
			return value
		}
		controller, _ := kernel.NewCompletionReviewGateControllerV2(&completionInputReaderV2{values: []applicationcontract.ReviewWaitingInputCurrentProjectionV1{fixture.Input}}, &completionCoordinatorV2{outcome: fixture.Outcome}, clock)
		if result, err := controller.EvaluateCompletionReviewGateV2(context.Background(), fixture.Request); err == nil || result != (bridgecontract.CompletionReviewGateResultV2{}) {
			t.Fatalf("TTL crossing accepted: result=%#v err=%v", result, err)
		}
	})
	t.Run("clock-rollback", func(t *testing.T) {
		fixture := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseRunV1, "rollback")
		times := []time.Time{fixture.Now.Add(5 * time.Second), fixture.Now.Add(6 * time.Second), fixture.Now.Add(4 * time.Second)}
		var index int
		clock := func() time.Time {
			value := times[index]
			if index < len(times)-1 {
				index++
			}
			return value
		}
		coordinator := &completionCoordinatorV2{outcome: fixture.Outcome}
		controller, _ := kernel.NewCompletionReviewGateControllerV2(&completionInputReaderV2{values: []applicationcontract.ReviewWaitingInputCurrentProjectionV1{fixture.Input}}, coordinator, clock)
		if _, err := controller.EvaluateCompletionReviewGateV2(context.Background(), fixture.Request); !core.HasReason(err, core.ReasonClockRegression) {
			t.Fatalf("rollback error=%v", err)
		}
	})
}

func TestCompletionReviewGateV2ReadLostReplyDetachedButCoordinatorNeverRetried(t *testing.T) {
	fixture := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseSubagentV1, "lost")
	reader := &completionInputReaderV2{values: []applicationcontract.ReviewWaitingInputCurrentProjectionV1{fixture.Input}, failOnce: core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "read reply lost")}
	coordinator := &completionCoordinatorV2{outcome: fixture.Outcome}
	controller, _ := kernel.NewCompletionReviewGateControllerV2(reader, coordinator, func() time.Time { return fixture.Now.Add(5 * time.Second) })
	result, err := controller.EvaluateCompletionReviewGateV2(context.Background(), fixture.Request)
	if err != nil || result.Outcome.Review.Decision != applicationcontract.ReviewPhaseAllowV1 || !reader.detachedRecoveryCalled || coordinator.callCount() != 1 {
		t.Fatalf("result=%#v err=%v detached=%v coordinator calls=%d", result, err, reader.detachedRecoveryCalled, coordinator.callCount())
	}

	unknown := &completionCoordinatorV2{err: core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "Application coordination reply lost")}
	controller, _ = kernel.NewCompletionReviewGateControllerV2(&completionInputReaderV2{values: []applicationcontract.ReviewWaitingInputCurrentProjectionV1{fixture.Input}}, unknown, func() time.Time { return fixture.Now.Add(5 * time.Second) })
	if _, err := controller.EvaluateCompletionReviewGateV2(context.Background(), fixture.Request); !core.HasCategory(err, core.ErrorIndeterminate) || unknown.callCount() != 1 {
		t.Fatalf("unknown mutation was retried or downgraded: calls=%d err=%v", unknown.callCount(), err)
	}

	canceledCoordinator := &completionCoordinatorV2{outcome: fixture.Outcome}
	controller, _ = kernel.NewCompletionReviewGateControllerV2(&completionInputReaderV2{values: []applicationcontract.ReviewWaitingInputCurrentProjectionV1{fixture.Input}, failOnce: context.Canceled}, canceledCoordinator, func() time.Time { return fixture.Now.Add(5 * time.Second) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := controller.EvaluateCompletionReviewGateV2(ctx, fixture.Request); !core.HasCategory(err, core.ErrorIndeterminate) || canceledCoordinator.callCount() != 0 {
		t.Fatalf("canceled caller crossed mutation boundary: calls=%d err=%v", canceledCoordinator.callCount(), err)
	}
}

func TestCompletionReviewGateV2TerminalCoordinationMustAdvanceReceiptPredecessor(t *testing.T) {
	fixture := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseRunV1, "coordination-drift")
	bad := fixture.Outcome.Clone()
	bad.Coordination.Revision = bad.Receipt.Coordination.Revision
	controller, _ := kernel.NewCompletionReviewGateControllerV2(&completionInputReaderV2{values: []applicationcontract.ReviewWaitingInputCurrentProjectionV1{fixture.Input}}, &completionCoordinatorV2{outcome: bad}, func() time.Time { return fixture.Now.Add(5 * time.Second) })
	if result, err := controller.EvaluateCompletionReviewGateV2(context.Background(), fixture.Request); err == nil || result != (bridgecontract.CompletionReviewGateResultV2{}) {
		t.Fatalf("non-advancing terminal coordination accepted: result=%#v err=%v", result, err)
	}
}

func TestCompletionReviewGateV2Concurrent64AndReusableConformance(t *testing.T) {
	subagent := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseSubagentV1, "conformance-subagent")
	run := testkit.CompletionReviewGateV2(t, applicationcontract.ReviewPhaseRunV1, "conformance-run")
	reader := &completionInputMultiplexV2{values: map[applicationcontract.ReviewWaitingInputSubjectV1]applicationcontract.ReviewWaitingInputCurrentProjectionV1{subagent.Input.Subject: subagent.Input, run.Input.Subject: run.Input}}
	coordinator := &completionCoordinatorMultiplexV2{values: map[string]applicationcontract.ReviewWaitingOutcomeV1{subagent.Request.Waiting.ID: subagent.Outcome, run.Request.Waiting.ID: run.Outcome}}
	controller, err := kernel.NewCompletionReviewGateControllerV2(reader, coordinator, func() time.Time { return run.Now.Add(5 * time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	report, err := conformance.CheckCompletionReviewGateV2(context.Background(), conformance.CompletionReviewGateCaseV2{Gate: controller, Subagent: subagent.Request, Run: run.Request, Now: run.Now.Add(5 * time.Second), Concurrent: 64})
	if err != nil || !report.SubagentCompletionExact || !report.RunCompletionExact || !report.ConcurrentReadExact || !report.TargetDriftFailClosed || !report.PhaseDriftFailClosed || report.CompletionClaimWriter || report.RuntimeOutcomeWriter || report.VerdictWriter || report.AuthorizationWriter || report.ProductionRootProven {
		t.Fatalf("report=%#v err=%v", report, err)
	}
	payload, _ := json.Marshal(report)
	for _, forbidden := range []string{"completion_claim_writer\":true", "runtime_outcome_writer\":true", "verdict_writer\":true", "authorization_writer\":true"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("authority leaked: %s", payload)
		}
	}
}

type completionInputMultiplexV2 struct {
	values map[applicationcontract.ReviewWaitingInputSubjectV1]applicationcontract.ReviewWaitingInputCurrentProjectionV1
}

func (r *completionInputMultiplexV2) InspectReviewWaitingInputCurrentV1(_ context.Context, subject applicationcontract.ReviewWaitingInputSubjectV1) (applicationcontract.ReviewWaitingInputCurrentProjectionV1, error) {
	value, ok := r.values[subject]
	if !ok {
		return applicationcontract.ReviewWaitingInputCurrentProjectionV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "completion input absent")
	}
	return value, nil
}

type completionCoordinatorMultiplexV2 struct {
	calls  atomic.Int64
	values map[string]applicationcontract.ReviewWaitingOutcomeV1
}

func (c *completionCoordinatorMultiplexV2) CoordinateReviewWaitingV1(_ context.Context, request applicationcontract.ReviewWaitingRequestV1) (applicationcontract.ReviewWaitingOutcomeV1, error) {
	c.calls.Add(1)
	value, ok := c.values[request.ID]
	if !ok {
		return applicationcontract.ReviewWaitingOutcomeV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "completion coordination absent")
	}
	return value.Clone(), nil
}

func TestCompletionReviewGateV2ConstructorsRejectTypedNil(t *testing.T) {
	var input *completionInputReaderV2
	var coordinator *completionCoordinatorV2
	if _, err := kernel.NewCompletionReviewGateControllerV2(input, &completionCoordinatorV2{}, time.Now); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil input accepted: %v", err)
	}
	if _, err := kernel.NewCompletionReviewGateControllerV2(&completionInputReaderV2{}, coordinator, time.Now); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil coordinator accepted: %v", err)
	}
}
