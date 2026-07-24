package conformance

import (
	"context"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	defaultReviewGateParallelismV1 = 16
	maxReviewGateParallelismV1     = 128
)

type ReviewGateCaseV1 struct {
	Gate        harnessports.ReviewGateV1
	Request     contract.ReviewGateRequestV1
	Parallelism int
}

// ReviewGateReportV1 certifies only the Harness read-only flow behavior. It
// cannot certify a Verdict, Authorization, Runtime Permit, or production root.
type ReviewGateReportV1 struct {
	ConcurrentExact         bool `json:"concurrent_exact"`
	ExactRefDriftFailClosed bool `json:"exact_ref_drift_fail_closed"`
	ActionDriftFailClosed   bool `json:"action_drift_fail_closed"`
	TargetDriftFailClosed   bool `json:"target_drift_fail_closed"`
	ReceiptObservationOnly  bool `json:"receipt_observation_only"`
	VerdictAuthority        bool `json:"verdict_authority"`
	AuthorizationAuthority  bool `json:"authorization_authority"`
	DispatchAuthority       bool `json:"dispatch_authority"`
	ProductionRootProven    bool `json:"production_root_proven"`
}

func CheckReviewGateV1(ctx context.Context, testCase ReviewGateCaseV1) (ReviewGateReportV1, error) {
	if testCase.Gate == nil {
		return ReviewGateReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review Gate is required")
	}
	parallelism := testCase.Parallelism
	if parallelism == 0 {
		parallelism = defaultReviewGateParallelismV1
	}
	if parallelism < 2 || parallelism > maxReviewGateParallelismV1 {
		return ReviewGateReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Review Gate parallelism must be between 2 and 128")
	}
	results := make([]contract.ReviewGateResultV1, parallelism)
	errs := make([]error, parallelism)
	var ready sync.WaitGroup
	var start sync.WaitGroup
	var workers sync.WaitGroup
	ready.Add(parallelism)
	start.Add(1)
	workers.Add(parallelism)
	for index := range results {
		go func(index int) {
			defer workers.Done()
			ready.Done()
			start.Wait()
			results[index], errs[index] = testCase.Gate.EvaluateReviewGateV1(ctx, testCase.Request)
		}(index)
	}
	ready.Wait()
	start.Done()
	workers.Wait()
	for index, result := range results {
		if errs[index] != nil {
			return ReviewGateReportV1{}, errs[index]
		}
		if err := result.Validate(); err != nil {
			return ReviewGateReportV1{}, err
		}
		if result.Decision != contract.ReviewGateAllowV1 || result.Receipt.Authorization == nil || testCase.Request.Authorization == nil || *result.Receipt.Authorization != *testCase.Request.Authorization || result.Receipt.Target != testCase.Request.Target {
			return ReviewGateReportV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "concurrent Review Gate did not preserve one exact allow material")
		}
	}
	drift := testCase.Request
	if drift.Authorization == nil {
		return ReviewGateReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictMissing, "Review Gate conformance requires one allow-capable exact Authorization")
	}
	driftRef := *drift.Authorization
	drift.Authorization = &driftRef
	drift.Authorization.Digest = core.DigestBytes([]byte("praxis.harness/review-gate-conformance/drift"))
	drifted, driftErr := testCase.Gate.EvaluateReviewGateV1(ctx, drift)
	if driftErr == nil && drifted.Decision == contract.ReviewGateAllowV1 {
		return ReviewGateReportV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review Gate allowed a changed exact Authorization ref")
	}
	actionDrift := testCase.Request
	actionDrift.Action.Subject.Base.SessionRevision++
	if result, err := testCase.Gate.EvaluateReviewGateV1(ctx, actionDrift); err == nil && result.Decision == contract.ReviewGateAllowV1 {
		return ReviewGateReportV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review Gate allowed a changed frozen Action")
	}
	targetDrift := testCase.Request
	targetDrift.Target.Digest = core.DigestBytes([]byte("praxis.harness/review-gate-conformance/target-drift"))
	if result, err := testCase.Gate.EvaluateReviewGateV1(ctx, targetDrift); err == nil && result.Decision == contract.ReviewGateAllowV1 {
		return ReviewGateReportV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review Gate allowed a changed exact target")
	}
	return ReviewGateReportV1{ConcurrentExact: true, ExactRefDriftFailClosed: true, ActionDriftFailClosed: true, TargetDriftFailClosed: true, ReceiptObservationOnly: true}, nil
}
