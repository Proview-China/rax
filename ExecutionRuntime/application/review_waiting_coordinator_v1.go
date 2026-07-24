package application

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"reflect"
	"sync"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ReviewWaitingCoordinatorConfigV1 struct {
	Facts   applicationports.ReviewWaitingCoordinationFactPortV1
	Inputs  applicationports.ReviewWaitingInputCurrentReaderV1
	Review  applicationports.ReviewStartOrInspectPortV1
	Clock   func() time.Time
	ClaimID func() (string, error)
}

// ReviewWaitingCoordinatorV1 owns only Application coordination. Inline and
// Detached are immutable request fields; neither creates a second Fact owner.
type ReviewWaitingCoordinatorV1 struct {
	config ReviewWaitingCoordinatorConfigV1
	gates  reviewWaitingCoordinatorGateV1
}

func NewReviewWaitingCoordinatorV1(config ReviewWaitingCoordinatorConfigV1) (*ReviewWaitingCoordinatorV1, error) {
	for _, dependency := range []any{config.Facts, config.Inputs, config.Review} {
		if reviewWaitingNilV1(dependency) {
			return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review waiting coordinator dependency is missing")
		}
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	if config.ClaimID == nil {
		config.ClaimID = newReviewWaitingClaimIDV1
	}
	if reviewWaitingNilV1(config.Clock) || reviewWaitingNilV1(config.ClaimID) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review waiting coordinator clock or claim source is missing")
	}
	return &ReviewWaitingCoordinatorV1{config: config, gates: reviewWaitingCoordinatorGateV1{entries: make(map[string]*reviewWaitingCoordinatorGateEntryV1)}}, nil
}

func (c *ReviewWaitingCoordinatorV1) CoordinateReviewWaitingV1(ctx context.Context, request contract.ReviewWaitingRequestV1) (contract.ReviewWaitingOutcomeV1, error) {
	baseline, err := c.nowAfterV1(time.Time{})
	if err != nil {
		return contract.ReviewWaitingOutcomeV1{}, err
	}
	if err := request.ValidateCurrent(baseline); err != nil {
		return contract.ReviewWaitingOutcomeV1{}, err
	}
	release := c.gates.acquire(request.ID + "\x00" + string(request.Digest))
	defer release()

	inputS1, checkpoint, err := c.inspectInputV1(ctx, request, baseline)
	if err != nil {
		return contract.ReviewWaitingOutcomeV1{}, err
	}
	initial, err := contract.NewReviewWaitingCoordinationFactV1(request, checkpoint)
	if err != nil {
		return contract.ReviewWaitingOutcomeV1{}, err
	}
	current, err := c.createOrRecoverV1(ctx, initial)
	if err != nil {
		return contract.ReviewWaitingOutcomeV1{}, err
	}
	if err := validateReviewWaitingFactForRequestV1(current, request); err != nil {
		return contract.ReviewWaitingOutcomeV1{}, err
	}
	if current.State == contract.ReviewSupersededStateV1 {
		return contract.ReviewWaitingOutcomeV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewCandidateConflict, "Review waiting coordination was superseded; submit the new Target as a new canonical request")
	}

	inputS2, checkpoint, inputErr := c.inspectInputV1(ctx, request, checkpoint)
	if inputErr != nil || !reflect.DeepEqual(inputS1, inputS2) {
		cause := inputErr
		if cause == nil {
			cause = core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting input changed across S1/S2")
		}
		return contract.ReviewWaitingOutcomeV1{}, c.supersedeForDriftV1(ctx, current, checkpoint, cause)
	}

	if current.State == contract.ReviewCompletedStateV1 {
		return c.resumeCompletedV1(ctx, request, current, inputS2, checkpoint)
	}

	owner := false
	if current.State == contract.ReviewWaitingStateV1 {
		current, owner, err = c.claimStartV1(ctx, current, checkpoint)
		if err != nil {
			return contract.ReviewWaitingOutcomeV1{}, err
		}
	}
	if current.State == contract.ReviewCompletedStateV1 {
		return c.resumeCompletedV1(ctx, request, current, inputS2, checkpoint)
	}
	if current.State != contract.ReviewInspectStateV1 {
		return contract.ReviewWaitingOutcomeV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "Review waiting coordination cannot start or Inspect Review")
	}

	inputBefore, checkpoint, err := c.inspectInputV1(ctx, request, checkpoint)
	if err != nil || !reflect.DeepEqual(inputS2, inputBefore) {
		cause := err
		if cause == nil {
			cause = core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting input drifted before Review boundary")
		}
		return contract.ReviewWaitingOutcomeV1{}, c.supersedeForDriftV1(ctx, current, checkpoint, cause)
	}

	projection, checkpoint, err := c.startOrInspectReviewV1(ctx, request, owner, checkpoint)
	if err != nil {
		return contract.ReviewWaitingOutcomeV1{}, err
	}
	// The Review boundary has now been invoked or exact-inspected. From this
	// point, caller cancellation cannot authorize abandoning reconciliation:
	// finish only same-canonical reads and the Application-owned durable CAS.
	recoveryContext := context.WithoutCancel(ctx)
	if projection.Case.Target != request.Target {
		cause := core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review current Target drifted")
		return contract.ReviewWaitingOutcomeV1{}, c.supersedeForDriftV1(recoveryContext, current, checkpoint, cause)
	}
	inputAfter, checkpoint, err := c.inspectInputV1(recoveryContext, request, checkpoint)
	if err != nil || !reflect.DeepEqual(inputBefore, inputAfter) {
		cause := err
		if cause == nil {
			cause = core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting input drifted across Review boundary")
		}
		return contract.ReviewWaitingOutcomeV1{}, c.supersedeForDriftV1(recoveryContext, current, checkpoint, cause)
	}
	return c.persistReviewProjectionV1(recoveryContext, request, current, projection, inputAfter, checkpoint)
}

func (c *ReviewWaitingCoordinatorV1) createOrRecoverV1(ctx context.Context, initial contract.ReviewWaitingCoordinationFactV1) (contract.ReviewWaitingCoordinationFactV1, error) {
	receipt, createErr := c.config.Facts.CreateReviewWaitingCoordinationV1(ctx, initial)
	if createErr == nil {
		if err := validateReviewWaitingFactForRequestV1(receipt.Fact, initial.Request); err != nil {
			return contract.ReviewWaitingCoordinationFactV1{}, err
		}
		return receipt.Fact.Clone(), nil
	}
	inspected, inspectErr := c.config.Facts.InspectCurrentReviewWaitingCoordinationV1(context.WithoutCancel(ctx), initial.Request.ExecutionScope, initial.ID)
	if inspectErr == nil {
		if err := validateReviewWaitingFactForRequestV1(inspected, initial.Request); err != nil {
			return contract.ReviewWaitingCoordinationFactV1{}, err
		}
		return inspected.Clone(), nil
	}
	if !core.HasCategory(createErr, core.ErrorIndeterminate) || !core.HasCategory(inspectErr, core.ErrorNotFound) {
		return contract.ReviewWaitingCoordinationFactV1{}, createErr
	}
	// The Fact Owner's linearizable NotFound is authoritative and no Review
	// boundary has been crossed, so one same-canonical create retry is safe.
	receipt, retryErr := c.config.Facts.CreateReviewWaitingCoordinationV1(context.WithoutCancel(ctx), initial)
	if retryErr == nil {
		if err := validateReviewWaitingFactForRequestV1(receipt.Fact, initial.Request); err != nil {
			return contract.ReviewWaitingCoordinationFactV1{}, err
		}
		return receipt.Fact.Clone(), nil
	}
	inspected, inspectErr = c.config.Facts.InspectCurrentReviewWaitingCoordinationV1(context.WithoutCancel(ctx), initial.Request.ExecutionScope, initial.ID)
	if inspectErr == nil && validateReviewWaitingFactForRequestV1(inspected, initial.Request) == nil {
		return inspected.Clone(), nil
	}
	return contract.ReviewWaitingCoordinationFactV1{}, retryErr
}

func (c *ReviewWaitingCoordinatorV1) claimStartV1(ctx context.Context, current contract.ReviewWaitingCoordinationFactV1, now time.Time) (contract.ReviewWaitingCoordinationFactV1, bool, error) {
	claimID, err := c.config.ClaimID()
	if err != nil || claimID == "" {
		return contract.ReviewWaitingCoordinationFactV1{}, false, core.NewError(core.ErrorUnavailable, core.ReasonOwnerConflict, "Review waiting start claim source failed")
	}
	next, err := contract.ClaimReviewWaitingStartV1(current, claimID, now)
	if err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, false, err
	}
	receipt, casErr := c.config.Facts.CompareAndSwapReviewWaitingCoordinationV1(ctx, applicationports.ReviewWaitingCoordinationCASRequestV1{Scope: current.Request.ExecutionScope, Expected: current.RefV1(), Next: next})
	if casErr == nil {
		return c.validateClaimReceiptV1(current.Request, next, claimID, receipt)
	}
	inspected, inspectErr := c.config.Facts.InspectCurrentReviewWaitingCoordinationV1(context.WithoutCancel(ctx), current.Request.ExecutionScope, current.ID)
	if inspectErr != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, false, casErr
	}
	if err := validateReviewWaitingFactForRequestV1(inspected, current.Request); err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, false, err
	}
	if inspected.State == contract.ReviewInspectStateV1 || inspected.State == contract.ReviewCompletedStateV1 {
		// A different immutable claim already consumed waiting_review. Equal
		// revision with another claim is still a non-owner, not a retry grant.
		return inspected.Clone(), false, nil
	}
	if inspected.RefV1() == next.RefV1() || inspected.Revision > next.Revision {
		// Lost reply or another owner advanced the same wait: permanently Inspect.
		return inspected.Clone(), false, nil
	}
	if inspected.RefV1() != current.RefV1() || !core.HasCategory(casErr, core.ErrorIndeterminate) {
		return contract.ReviewWaitingCoordinationFactV1{}, false, casErr
	}
	// The Owner proved the predecessor is still current, so retry only the same
	// canonical CAS. Applied=false can never grant Start authority.
	receipt, retryErr := c.config.Facts.CompareAndSwapReviewWaitingCoordinationV1(context.WithoutCancel(ctx), applicationports.ReviewWaitingCoordinationCASRequestV1{Scope: current.Request.ExecutionScope, Expected: current.RefV1(), Next: next})
	if retryErr == nil {
		return c.validateClaimReceiptV1(current.Request, next, claimID, receipt)
	}
	inspected, inspectErr = c.config.Facts.InspectCurrentReviewWaitingCoordinationV1(context.WithoutCancel(ctx), current.Request.ExecutionScope, current.ID)
	if inspectErr == nil && validateReviewWaitingFactForRequestV1(inspected, current.Request) == nil && (inspected.RefV1() == next.RefV1() || inspected.Revision > next.Revision) {
		return inspected.Clone(), false, nil
	}
	return contract.ReviewWaitingCoordinationFactV1{}, false, retryErr
}

func (c *ReviewWaitingCoordinatorV1) validateClaimReceiptV1(request contract.ReviewWaitingRequestV1, next contract.ReviewWaitingCoordinationFactV1, claimID string, receipt applicationports.ReviewWaitingCoordinationCASReceiptV1) (contract.ReviewWaitingCoordinationFactV1, bool, error) {
	if err := validateReviewWaitingFactForRequestV1(receipt.Fact, request); err != nil {
		return contract.ReviewWaitingCoordinationFactV1{}, false, err
	}
	if receipt.Fact.RefV1() != next.RefV1() || receipt.Fact.StartClaimID != claimID {
		return receipt.Fact.Clone(), false, core.NewError(core.ErrorConflict, core.ReasonOwnerConflict, "Review waiting claim CAS returned another successor")
	}
	if !receipt.Applied {
		return receipt.Fact.Clone(), false, nil
	}
	return receipt.Fact.Clone(), true, nil
}

func (c *ReviewWaitingCoordinatorV1) startOrInspectReviewV1(ctx context.Context, request contract.ReviewWaitingRequestV1, owner bool, previous time.Time) (contract.ReviewWaitingCurrentProjectionV1, time.Time, error) {
	inspect := contract.ReviewWaitingInspectRequestV1{Request: request.ReviewRequest, Target: request.Target}
	var projection contract.ReviewWaitingCurrentProjectionV1
	var err error
	inspectContext := ctx
	if owner {
		projection, err = c.config.Review.StartOrInspectReviewV1(ctx, request)
		if err != nil {
			// Once StartOrInspect was invoked, every outcome including authoritative
			// NotFound is Inspect-only. Never call StartOrInspect a second time.
			var inspectErr error
			inspectContext = context.WithoutCancel(ctx)
			projection, inspectErr = c.config.Review.InspectReviewV1(inspectContext, inspect)
			if inspectErr != nil {
				return contract.ReviewWaitingCurrentProjectionV1{}, time.Time{}, err
			}
			err = nil
		}
	} else {
		// Any caller which did not receive the unique Applied=true claim is a
		// recovery reader. Its exact Inspect must not inherit cancellation from
		// the unknown mutation attempt it is recovering.
		inspectContext = context.WithoutCancel(ctx)
		projection, err = c.config.Review.InspectReviewV1(inspectContext, inspect)
	}
	if err != nil {
		return contract.ReviewWaitingCurrentProjectionV1{}, time.Time{}, err
	}
	checkpoint, err := c.nowAfterV1(previous)
	if err != nil {
		return contract.ReviewWaitingCurrentProjectionV1{}, time.Time{}, err
	}
	if err := projection.ValidateFor(request, checkpoint); err != nil {
		return projection.Clone(), checkpoint, err
	}
	second, err := c.config.Review.InspectReviewV1(inspectContext, inspect)
	if err != nil {
		return contract.ReviewWaitingCurrentProjectionV1{}, time.Time{}, err
	}
	final, err := c.nowAfterV1(checkpoint)
	if err != nil {
		return contract.ReviewWaitingCurrentProjectionV1{}, time.Time{}, err
	}
	if err := second.ValidateFor(request, final); err != nil {
		return second.Clone(), final, err
	}
	if !reflect.DeepEqual(projection, second) {
		return contract.ReviewWaitingCurrentProjectionV1{}, final, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review current projection changed across S1/S2")
	}
	return projection.Clone(), final, nil
}

func (c *ReviewWaitingCoordinatorV1) inspectInputV1(ctx context.Context, request contract.ReviewWaitingRequestV1, previous time.Time) (contract.ReviewWaitingInputCurrentProjectionV1, time.Time, error) {
	projection, err := c.config.Inputs.InspectReviewWaitingInputCurrentV1(ctx, request.InputSubjectV1())
	if err != nil {
		return contract.ReviewWaitingInputCurrentProjectionV1{}, time.Time{}, err
	}
	checkpoint, err := c.nowAfterV1(previous)
	if err != nil {
		return projection, time.Time{}, err
	}
	if err := request.ValidateCurrent(checkpoint); err != nil {
		return projection, checkpoint, err
	}
	if err := projection.ValidateFor(request, checkpoint); err != nil {
		return projection, checkpoint, err
	}
	return projection, checkpoint, nil
}

func (c *ReviewWaitingCoordinatorV1) persistReviewProjectionV1(ctx context.Context, request contract.ReviewWaitingRequestV1, current contract.ReviewWaitingCoordinationFactV1, projection contract.ReviewWaitingCurrentProjectionV1, input contract.ReviewWaitingInputCurrentProjectionV1, now time.Time) (contract.ReviewWaitingOutcomeV1, error) {
	if projection.Decision == contract.ReviewPhaseDeferV1 {
		if current.Case == nil || *current.Case != projection.Case {
			next, err := contract.RecordReviewWaitingCurrentV1(current, projection, now)
			if err != nil {
				return contract.ReviewWaitingOutcomeV1{}, err
			}
			stored, _, err := c.casWithRecoveryV1(ctx, current, next, false)
			if err != nil {
				return contract.ReviewWaitingOutcomeV1{}, err
			}
			current = stored
		}
		outcome := contract.ReviewWaitingOutcomeV1{Coordination: current.RefV1(), Review: projection.Clone()}
		if err := outcome.ValidateFor(request, now); err != nil {
			return contract.ReviewWaitingOutcomeV1{}, err
		}
		return outcome.Clone(), nil
	}
	receipt, err := contract.SealReviewPhaseReceiptV1(contract.ReviewPhaseReceiptV1{Coordination: current.RefV1()}, request, projection, input, now)
	if err != nil {
		return contract.ReviewWaitingOutcomeV1{}, err
	}
	next, err := contract.CompleteReviewWaitingV1(current, receipt, now)
	if err != nil {
		return contract.ReviewWaitingOutcomeV1{}, err
	}
	stored, _, err := c.casWithRecoveryV1(ctx, current, next, false)
	if err != nil {
		return contract.ReviewWaitingOutcomeV1{}, err
	}
	if stored.State != contract.ReviewCompletedStateV1 || stored.Receipt == nil {
		return contract.ReviewWaitingOutcomeV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review waiting completion recovered another state")
	}
	outcome := contract.ReviewWaitingOutcomeV1{Coordination: stored.RefV1(), Review: projection.Clone(), Receipt: stored.Receipt}
	if err := outcome.ValidateFor(request, now); err != nil {
		return contract.ReviewWaitingOutcomeV1{}, err
	}
	return outcome.Clone(), nil
}

func (c *ReviewWaitingCoordinatorV1) resumeCompletedV1(ctx context.Context, request contract.ReviewWaitingRequestV1, current contract.ReviewWaitingCoordinationFactV1, input contract.ReviewWaitingInputCurrentProjectionV1, previous time.Time) (contract.ReviewWaitingOutcomeV1, error) {
	if current.Receipt == nil {
		return contract.ReviewWaitingOutcomeV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictMissing, "completed Review waiting coordination lacks receipt")
	}
	projection, checkpoint, err := c.startOrInspectReviewV1(ctx, request, false, previous)
	if err != nil {
		return contract.ReviewWaitingOutcomeV1{}, err
	}
	inputS2, checkpoint, err := c.inspectInputV1(ctx, request, checkpoint)
	if err != nil || !reflect.DeepEqual(input, inputS2) {
		cause := err
		if cause == nil {
			cause = core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "completed Review input drifted")
		}
		return contract.ReviewWaitingOutcomeV1{}, c.supersedeForDriftV1(ctx, current, checkpoint, cause)
	}
	outcome := contract.ReviewWaitingOutcomeV1{Coordination: current.RefV1(), Review: projection.Clone(), Receipt: current.Receipt}
	if err := outcome.ValidateFor(request, checkpoint); err != nil {
		return contract.ReviewWaitingOutcomeV1{}, err
	}
	return outcome.Clone(), nil
}

func (c *ReviewWaitingCoordinatorV1) supersedeForDriftV1(ctx context.Context, current contract.ReviewWaitingCoordinationFactV1, previous time.Time, cause error) error {
	if current.State == contract.ReviewSupersededStateV1 {
		return cause
	}
	now, err := c.nowAfterV1(previous)
	if err != nil {
		return err
	}
	next, err := contract.SupersedeReviewWaitingV1(current, core.ReasonReviewCandidateConflict, now)
	if err != nil {
		return err
	}
	if _, _, err := c.casWithRecoveryV1(context.WithoutCancel(ctx), current, next, false); err != nil {
		return err
	}
	return cause
}

func (c *ReviewWaitingCoordinatorV1) casWithRecoveryV1(ctx context.Context, current, next contract.ReviewWaitingCoordinationFactV1, retryUncommitted bool) (contract.ReviewWaitingCoordinationFactV1, bool, error) {
	request := applicationports.ReviewWaitingCoordinationCASRequestV1{Scope: current.Request.ExecutionScope, Expected: current.RefV1(), Next: next}
	receipt, casErr := c.config.Facts.CompareAndSwapReviewWaitingCoordinationV1(ctx, request)
	if casErr == nil {
		if err := validateReviewWaitingFactForRequestV1(receipt.Fact, current.Request); err != nil {
			return contract.ReviewWaitingCoordinationFactV1{}, false, err
		}
		if receipt.Fact.RefV1() != next.RefV1() {
			return contract.ReviewWaitingCoordinationFactV1{}, false, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review waiting CAS returned another successor")
		}
		return receipt.Fact.Clone(), receipt.Applied, nil
	}
	inspected, inspectErr := c.config.Facts.InspectCurrentReviewWaitingCoordinationV1(context.WithoutCancel(ctx), current.Request.ExecutionScope, current.ID)
	if inspectErr == nil {
		if err := validateReviewWaitingFactForRequestV1(inspected, current.Request); err != nil {
			return contract.ReviewWaitingCoordinationFactV1{}, false, err
		}
		if inspected.RefV1() == next.RefV1() || inspected.Revision > next.Revision {
			return inspected.Clone(), false, nil
		}
		if retryUncommitted && inspected.RefV1() == current.RefV1() && core.HasCategory(casErr, core.ErrorIndeterminate) {
			return c.casWithRecoveryV1(context.WithoutCancel(ctx), current, next, false)
		}
	}
	return contract.ReviewWaitingCoordinationFactV1{}, false, casErr
}

func (c *ReviewWaitingCoordinatorV1) nowAfterV1(previous time.Time) (time.Time, error) {
	if c == nil || c.config.Clock == nil {
		return time.Time{}, core.NewError(core.ErrorUnavailable, core.ReasonClockRegression, "Review waiting coordinator clock is unavailable")
	}
	now := c.config.Clock()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorIndeterminate, core.ReasonClockRegression, "Review waiting coordinator clock is indeterminate or regressed")
	}
	return now, nil
}

func validateReviewWaitingFactForRequestV1(fact contract.ReviewWaitingCoordinationFactV1, request contract.ReviewWaitingRequestV1) error {
	if err := fact.Validate(); err != nil {
		return err
	}
	if fact.ID != request.ID || fact.Request.Digest != request.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Review waiting coordination belongs to another canonical request")
	}
	return nil
}

func newReviewWaitingClaimIDV1() (string, error) {
	var nonce [16]byte
	if _, err := cryptorand.Read(nonce[:]); err != nil {
		return "", err
	}
	return "review-start-claim/" + hex.EncodeToString(nonce[:]), nil
}

func reviewWaitingNilV1(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

type reviewWaitingCoordinatorGateEntryV1 struct {
	mu   sync.Mutex
	refs int
}

type reviewWaitingCoordinatorGateV1 struct {
	mu      sync.Mutex
	entries map[string]*reviewWaitingCoordinatorGateEntryV1
}

func (g *reviewWaitingCoordinatorGateV1) acquire(key string) func() {
	g.mu.Lock()
	if g.entries == nil {
		g.entries = make(map[string]*reviewWaitingCoordinatorGateEntryV1)
	}
	entry := g.entries[key]
	if entry == nil {
		entry = &reviewWaitingCoordinatorGateEntryV1{}
		g.entries[key] = entry
	}
	entry.refs++
	g.mu.Unlock()
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		g.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(g.entries, key)
		}
		g.mu.Unlock()
	}
}
