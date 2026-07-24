package bridgecontract

import (
	"context"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	CompletionReviewGateContractVersionV2 = "praxis.harness.completion-review-gate/v2"
	completionReviewGateCanonicalDomainV2 = "praxis.harness.completion-review-gate"
)

// CompletionReviewInputCurrentReaderV2 and
// CompletionReviewWaitingCoordinatorV2 are narrow structural views over the
// existing Application public contracts. They add no Harness-owned DTO or
// mutation semantics.
type CompletionReviewInputCurrentReaderV2 interface {
	InspectReviewWaitingInputCurrentV1(context.Context, applicationcontract.ReviewWaitingInputSubjectV1) (applicationcontract.ReviewWaitingInputCurrentProjectionV1, error)
}

type CompletionReviewWaitingCoordinatorV2 interface {
	CoordinateReviewWaitingV1(context.Context, applicationcontract.ReviewWaitingRequestV1) (applicationcontract.ReviewWaitingOutcomeV1, error)
}

type ReviewWaitingInputSubjectV1 = applicationcontract.ReviewWaitingInputSubjectV1
type ReviewWaitingInputCurrentProjectionV1 = applicationcontract.ReviewWaitingInputCurrentProjectionV1
type ReviewPhaseDecisionV1 = applicationcontract.ReviewPhaseDecisionV1

const (
	ReviewPhaseActionV1   = applicationcontract.ReviewPhaseActionV1
	ReviewPhaseSubagentV1 = applicationcontract.ReviewPhaseSubagentV1
	ReviewPhaseRunV1      = applicationcontract.ReviewPhaseRunV1
	ReviewPhaseAllowV1    = applicationcontract.ReviewPhaseAllowV1
)

// CompletionReviewGateRequestV2 deliberately references the Application-owned
// waiting Request. It does not define a second Phase, Target, Case or Verdict.
type CompletionReviewGateRequestV2 struct {
	ContractVersion string                                     `json:"contract_version"`
	Waiting         applicationcontract.ReviewWaitingRequestV1 `json:"waiting"`
}

func (r CompletionReviewGateRequestV2) ValidateCurrent(now time.Time) error {
	if r.ContractVersion != CompletionReviewGateContractVersionV2 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "completion Review Gate contract is unsupported")
	}
	if r.Waiting.Phase.Kind != applicationcontract.ReviewPhaseSubagentV1 && r.Waiting.Phase.Kind != applicationcontract.ReviewPhaseRunV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "completion Review Gate accepts only namespaced completion phases")
	}
	return r.Waiting.ValidateCurrent(now)
}

// CompletionReviewGateResultV2 is a Harness observation over the exact
// Application objects. It grants no Completion Claim, Runtime Outcome,
// Review Verdict or Authorization authority.
type CompletionReviewGateResultV2 struct {
	ContractVersion string                                                    `json:"contract_version"`
	Input           applicationcontract.ReviewWaitingInputCurrentProjectionV1 `json:"input"`
	Outcome         applicationcontract.ReviewWaitingOutcomeV1                `json:"outcome"`
	CheckedUnixNano int64                                                     `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                                     `json:"expires_unix_nano"`
	Digest          core.Digest                                               `json:"digest"`
}

func (r CompletionReviewGateResultV2) Clone() CompletionReviewGateResultV2 {
	r.Outcome = r.Outcome.Clone()
	return r
}

func (r CompletionReviewGateResultV2) DigestV2() (core.Digest, error) {
	copy := r.Clone()
	copy.Digest = ""
	return core.CanonicalJSONDigest(completionReviewGateCanonicalDomainV2, CompletionReviewGateContractVersionV2, "CompletionReviewGateResultV2", copy)
}

func NewCompletionReviewGateResultV2(request CompletionReviewGateRequestV2, input applicationcontract.ReviewWaitingInputCurrentProjectionV1, outcome applicationcontract.ReviewWaitingOutcomeV1, checked time.Time) (CompletionReviewGateResultV2, error) {
	if err := request.ValidateCurrent(checked); err != nil {
		return CompletionReviewGateResultV2{}, err
	}
	if err := input.ValidateFor(request.Waiting, checked); err != nil {
		return CompletionReviewGateResultV2{}, err
	}
	if err := outcome.ValidateFor(request.Waiting, checked); err != nil {
		return CompletionReviewGateResultV2{}, err
	}
	expires := minimumCompletionReviewGateExpiryV2(request.Waiting.ExpiresUnixNano, input.ExpiresUnixNano, outcome.Review.ExpiresUnixNano)
	if outcome.Receipt != nil {
		expires = minimumCompletionReviewGateExpiryV2(expires, outcome.Receipt.ExpiresUnixNano)
	}
	result := CompletionReviewGateResultV2{
		ContractVersion: CompletionReviewGateContractVersionV2,
		Input:           input,
		Outcome:         outcome.Clone(),
		CheckedUnixNano: checked.UnixNano(),
		ExpiresUnixNano: expires,
	}
	digest, err := result.DigestV2()
	if err != nil {
		return CompletionReviewGateResultV2{}, err
	}
	result.Digest = digest
	if err := result.ValidateCurrentFor(request, checked); err != nil {
		return CompletionReviewGateResultV2{}, err
	}
	return result, nil
}

func (r CompletionReviewGateResultV2) ValidateCurrentFor(request CompletionReviewGateRequestV2, now time.Time) error {
	if err := request.ValidateCurrent(now); err != nil {
		return err
	}
	if now.IsZero() || r.CheckedUnixNano > 0 && now.UnixNano() < r.CheckedUnixNano {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "completion Review Gate result clock regressed")
	}
	if r.ContractVersion != CompletionReviewGateContractVersionV2 || r.CheckedUnixNano <= 0 || r.CheckedUnixNano >= r.ExpiresUnixNano || !now.Before(time.Unix(0, r.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "completion Review Gate result is incomplete or stale")
	}
	if r.CheckedUnixNano < r.Input.CheckedUnixNano || r.CheckedUnixNano < r.Outcome.Review.CheckedUnixNano || r.Outcome.Receipt != nil && r.CheckedUnixNano < r.Outcome.Receipt.CheckedUnixNano {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "completion Review Gate observation predates its current inputs")
	}
	if err := r.Input.ValidateFor(request.Waiting, now); err != nil {
		return err
	}
	if err := r.Outcome.ValidateFor(request.Waiting, now); err != nil {
		return err
	}
	if r.Outcome.Coordination.ID != request.Waiting.ID {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "completion Review Gate outcome belongs to another waiting coordination")
	}
	if r.Outcome.Receipt != nil && (r.Outcome.Receipt.Coordination.ID != request.Waiting.ID || r.Outcome.Coordination.Revision != r.Outcome.Receipt.Coordination.Revision+1) {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "completion Review Gate terminal outcome does not advance its receipt predecessor")
	}
	expectedExpiry := minimumCompletionReviewGateExpiryV2(request.Waiting.ExpiresUnixNano, r.Input.ExpiresUnixNano, r.Outcome.Review.ExpiresUnixNano)
	if r.Outcome.Receipt != nil {
		expectedExpiry = minimumCompletionReviewGateExpiryV2(expectedExpiry, r.Outcome.Receipt.ExpiresUnixNano)
	}
	if r.ExpiresUnixNano != expectedExpiry {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "completion Review Gate result TTL is not the exact minimum")
	}
	digest, err := r.DigestV2()
	if err != nil || digest != r.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "completion Review Gate result digest drifted")
	}
	return nil
}

func minimumCompletionReviewGateExpiryV2(values ...int64) int64 {
	var minimum int64
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}
