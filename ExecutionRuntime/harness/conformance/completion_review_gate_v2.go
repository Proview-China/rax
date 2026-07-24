package conformance

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type CompletionReviewGateCaseV2 struct {
	Gate       harnessports.CompletionReviewGateV2
	Subagent   bridgecontract.CompletionReviewGateRequestV2
	Run        bridgecontract.CompletionReviewGateRequestV2
	Now        time.Time
	Concurrent int
}

type CompletionReviewGateReportV2 struct {
	SubagentCompletionExact bool `json:"subagent_completion_exact"`
	RunCompletionExact      bool `json:"run_completion_exact"`
	ConcurrentReadExact     bool `json:"concurrent_read_exact"`
	TargetDriftFailClosed   bool `json:"target_drift_fail_closed"`
	PhaseDriftFailClosed    bool `json:"phase_drift_fail_closed"`
	CompletionClaimWriter   bool `json:"completion_claim_writer"`
	RuntimeOutcomeWriter    bool `json:"runtime_outcome_writer"`
	VerdictWriter           bool `json:"verdict_writer"`
	AuthorizationWriter     bool `json:"authorization_writer"`
	ProductionRootProven    bool `json:"production_root_proven"`
}

func CheckCompletionReviewGateV2(ctx context.Context, testCase CompletionReviewGateCaseV2) (CompletionReviewGateReportV2, error) {
	report := CompletionReviewGateReportV2{}
	if testCase.Gate == nil || testCase.Now.IsZero() {
		return report, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "completion Review Gate conformance dependencies are required")
	}
	requests := []struct {
		request bridgecontract.CompletionReviewGateRequestV2
		phase   bridgecontract.ReviewPhaseDecisionV1
	}{
		{testCase.Subagent, bridgecontract.ReviewPhaseAllowV1},
		{testCase.Run, bridgecontract.ReviewPhaseAllowV1},
	}
	for index, item := range requests {
		result, err := testCase.Gate.EvaluateCompletionReviewGateV2(ctx, item.request)
		if err != nil {
			return report, err
		}
		if err := result.ValidateCurrentFor(item.request, testCase.Now); err != nil || result.Outcome.Review.Decision != item.phase {
			if err != nil {
				return report, err
			}
			return report, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "completion Review Gate decision drifted")
		}
		if index == 0 {
			report.SubagentCompletionExact = true
		} else {
			report.RunCompletionExact = true
		}
	}

	parallelism := testCase.Concurrent
	if parallelism == 0 {
		parallelism = 16
	}
	if parallelism < 2 || parallelism > 128 {
		return report, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "completion Review Gate parallelism must be between 2 and 128")
	}
	results := make([]bridgecontract.CompletionReviewGateResultV2, parallelism)
	errs := make([]error, parallelism)
	var ready, workers sync.WaitGroup
	var start sync.WaitGroup
	ready.Add(parallelism)
	workers.Add(parallelism)
	start.Add(1)
	for index := range results {
		go func(index int) {
			defer workers.Done()
			ready.Done()
			start.Wait()
			results[index], errs[index] = testCase.Gate.EvaluateCompletionReviewGateV2(ctx, testCase.Run)
		}(index)
	}
	ready.Wait()
	start.Done()
	workers.Wait()
	for index := range results {
		if errs[index] != nil || !reflect.DeepEqual(results[0], results[index]) {
			if errs[index] != nil {
				return report, errs[index]
			}
			return report, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "concurrent completion Review Gate results differ")
		}
	}
	report.ConcurrentReadExact = true

	targetDrift := testCase.Run
	targetDrift.Waiting.Target.Digest = core.DigestBytes([]byte("completion-conformance-target-drift"))
	if result, err := testCase.Gate.EvaluateCompletionReviewGateV2(ctx, targetDrift); err == nil && result.Outcome.Review.Decision == bridgecontract.ReviewPhaseAllowV1 {
		return report, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "completion Review Gate allowed Target drift")
	}
	report.TargetDriftFailClosed = true
	phaseDrift := testCase.Run
	phaseDrift.Waiting.Phase.Kind = bridgecontract.ReviewPhaseActionV1
	if result, err := testCase.Gate.EvaluateCompletionReviewGateV2(ctx, phaseDrift); err == nil && result.Outcome.Review.Decision == bridgecontract.ReviewPhaseAllowV1 {
		return report, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "completion Review Gate allowed a non-completion phase")
	}
	report.PhaseDriftFailClosed = true
	return report, nil
}
