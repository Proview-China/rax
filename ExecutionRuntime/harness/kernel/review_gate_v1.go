package kernel

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ReviewGateControllerV1 struct {
	actions        harnessports.CommittedPendingActionReaderV3
	authorizations harnessports.ReviewGateAuthorizationReaderV1
	clock          func() time.Time
}

func NewReviewGateControllerV1(actions harnessports.CommittedPendingActionReaderV3, authorizations harnessports.ReviewGateAuthorizationReaderV1, clock func() time.Time) (*ReviewGateControllerV1, error) {
	if reviewGateNilV1(actions) || reviewGateNilV1(authorizations) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review Gate read-only dependencies are required")
	}
	return &ReviewGateControllerV1{actions: actions, authorizations: authorizations, clock: clock}, nil
}

func (c *ReviewGateControllerV1) EvaluateReviewGateV1(ctx context.Context, request contract.ReviewGateRequestV1) (contract.ReviewGateResultV1, error) {
	baseline, err := c.freshNowV1(time.Time{})
	if err != nil {
		return contract.ReviewGateResultV1{}, err
	}
	if err := request.Validate(baseline); err != nil {
		return contract.ReviewGateResultV1{}, err
	}
	actionS1, err := c.inspectActionV1(ctx, request.Action)
	if err != nil {
		return c.closedResultV1(request, contract.ReviewGateDeferV1, err, baseline, request.RequestedNotAfterUnixNano, "")
	}
	if request.Authorization == nil {
		middle, err := c.freshNowV1(baseline)
		if err != nil {
			return contract.ReviewGateResultV1{}, err
		}
		actionS2, err := c.inspectActionV1(ctx, request.Action)
		if err != nil {
			return contract.ReviewGateResultV1{}, err
		}
		now, err := c.freshNowV1(middle)
		if err != nil {
			return contract.ReviewGateResultV1{}, err
		}
		if !reflect.DeepEqual(actionS1, actionS2) {
			return contract.ReviewGateResultV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "first Review Gate Action drifted between S1 and S2")
		}
		if err := actionS2.ValidateAgainst(request.Action, now); err != nil {
			return contract.ReviewGateResultV1{}, err
		}
		return c.closedResultV1(request, contract.ReviewGateAskV1, core.NewError(core.ErrorNotFound, core.ReasonReviewVerdictMissing, "Review Authorization has not been formed"), now, minimumReviewGateExpiryV1(request.RequestedNotAfterUnixNano, actionS2.ExpiresUnixNano), "")
	}
	exactS1, err := c.inspectExactAuthorizationV1(ctx, *request.Authorization)
	if err != nil {
		return c.closedFromAuthorizationErrorV1(request, err, baseline, actionS1.ExpiresUnixNano)
	}
	currentS1, err := c.inspectCurrentAuthorizationV1(ctx, request)
	if err != nil {
		return c.closedFromAuthorizationErrorV1(request, err, baseline, actionS1.ExpiresUnixNano)
	}
	middle, err := c.freshNowV1(baseline)
	if err != nil {
		return contract.ReviewGateResultV1{}, err
	}
	if err := validateReviewGateMaterialV1(request, actionS1, exactS1, currentS1, middle); err != nil {
		return c.closedResultV1(request, contract.ReviewGateDeferV1, err, middle, minimumReviewGateExpiryV1(request.RequestedNotAfterUnixNano, actionS1.ExpiresUnixNano, exactS1.ExpiresUnixNano, currentS1.ExpiresUnixNano), currentS1.Review.ProjectionDigest)
	}

	actionS2, err := c.inspectActionV1(ctx, request.Action)
	if err != nil {
		return c.closedResultV1(request, contract.ReviewGateDeferV1, err, middle, request.RequestedNotAfterUnixNano, "")
	}
	exactS2, err := c.inspectExactAuthorizationV1(ctx, *request.Authorization)
	if err != nil {
		return c.closedFromAuthorizationErrorV1(request, err, middle, actionS2.ExpiresUnixNano)
	}
	currentS2, err := c.inspectCurrentAuthorizationV1(ctx, request)
	if err != nil {
		return c.closedFromAuthorizationErrorV1(request, err, middle, actionS2.ExpiresUnixNano)
	}
	now, err := c.freshNowV1(middle)
	if err != nil {
		return contract.ReviewGateResultV1{}, err
	}
	if !reflect.DeepEqual(actionS1, actionS2) || !reflect.DeepEqual(exactS1, exactS2) || !reflect.DeepEqual(currentS1, currentS2) {
		return c.closedResultV1(request, contract.ReviewGateDeferV1, core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Review Gate current material drifted between S1 and S2"), now, minimumReviewGateExpiryV1(request.RequestedNotAfterUnixNano, actionS2.ExpiresUnixNano, exactS2.ExpiresUnixNano, currentS2.ExpiresUnixNano), currentS2.Review.ProjectionDigest)
	}
	if err := validateReviewGateMaterialV1(request, actionS2, exactS2, currentS2, now); err != nil {
		return c.closedResultV1(request, contract.ReviewGateDeferV1, err, now, minimumReviewGateExpiryV1(request.RequestedNotAfterUnixNano, actionS2.ExpiresUnixNano, exactS2.ExpiresUnixNano, currentS2.ExpiresUnixNano), currentS2.Review.ProjectionDigest)
	}
	expires := minimumReviewGateExpiryV1(request.RequestedNotAfterUnixNano, actionS2.ExpiresUnixNano, exactS2.ExpiresUnixNano, currentS2.ExpiresUnixNano, currentS2.Review.ExpiresUnixNano)
	return c.sealResultV1(request, contract.ReviewGateAllowV1, "", "", now, expires, currentS2.Review.ProjectionDigest)
}

func validateReviewGateMaterialV1(request contract.ReviewGateRequestV1, action contract.CommittedPendingActionCurrentV3, exact, current runtimeports.OperationReviewAuthorizationFactV5, now time.Time) error {
	if err := action.ValidateAgainst(request.Action, now); err != nil {
		return err
	}
	if err := exact.Validate(); err != nil {
		return err
	}
	if err := current.Validate(); err != nil {
		return err
	}
	if request.Authorization == nil || exact.RefV5() != *request.Authorization || current.RefV5() != *request.Authorization || !reflect.DeepEqual(exact, current) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictStale, "Review Gate exact and current Authorization differ")
	}
	if current.State != runtimeports.OperationReviewAuthorizationActiveV5 || !now.Before(time.Unix(0, current.ExpiresUnixNano)) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Gate Authorization is inactive or expired")
	}
	if current.Review.Basis != request.Basis {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review Gate basis drifted")
	}
	intentDigest, err := request.Intent.DigestV3()
	if err != nil {
		return err
	}
	if current.Intent.IntentID != request.Intent.ID || current.Intent.IntentRevision != request.Intent.Revision || current.Intent.IntentDigest != intentDigest || current.Intent.PayloadSchema != request.Intent.Payload.Schema || current.Intent.PayloadDigest != request.Intent.Payload.ContentDigest || current.Intent.PayloadRevision != request.Intent.PayloadRevision || !runtimeports.SameOperationSubjectV3(current.Intent.Operation, request.Intent.Operation) {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review Gate Authorization and Intent differ")
	}
	var target runtimeports.OperationReviewTargetRefV4
	switch current.Review.Basis {
	case runtimeports.OperationReviewBasisAcceptedQuorumV5, runtimeports.OperationReviewBasisConditionalQuorumSatisfiedV5:
		if current.Review.Quorum == nil {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictMissing, "Review Gate quorum projection is missing")
		}
		target = current.Review.Quorum.Target
	case runtimeports.OperationReviewBasisPolicyNotRequiredV5:
		if current.Review.PolicyNotRequired == nil {
			return core.NewError(core.ErrorConflict, core.ReasonReviewVerdictMissing, "Review Gate not-required projection is missing")
		}
		target = current.Review.PolicyNotRequired.Target
	default:
		return core.NewError(core.ErrorForbidden, core.ReasonReviewVerdictStale, "Review Gate Authorization basis is unsupported")
	}
	if target != request.Target {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review Gate Authorization target drifted")
	}
	return nil
}

func (c *ReviewGateControllerV1) inspectActionV1(ctx context.Context, request contract.CommittedPendingActionCurrentRequestV3) (contract.CommittedPendingActionCurrentV3, error) {
	value, err := c.actions.InspectCommittedPendingActionCurrentV3(ctx, request)
	if reviewGateRetryableReadV1(err) {
		return c.actions.InspectCommittedPendingActionCurrentV3(context.WithoutCancel(ctx), request)
	}
	return value, err
}

func (c *ReviewGateControllerV1) inspectExactAuthorizationV1(ctx context.Context, ref runtimeports.OperationReviewAuthorizationRefV5) (runtimeports.OperationReviewAuthorizationFactV5, error) {
	value, err := c.authorizations.InspectOperationReviewAuthorizationExactV5(ctx, ref)
	if reviewGateRetryableReadV1(err) {
		return c.authorizations.InspectOperationReviewAuthorizationExactV5(context.WithoutCancel(ctx), ref)
	}
	return value, err
}

func (c *ReviewGateControllerV1) inspectCurrentAuthorizationV1(ctx context.Context, request contract.ReviewGateRequestV1) (runtimeports.OperationReviewAuthorizationFactV5, error) {
	value, err := c.authorizations.InspectCurrentOperationReviewAuthorizationV5(ctx, request.Intent.Operation, request.Intent.ID, request.Authorization.ID)
	if reviewGateRetryableReadV1(err) {
		return c.authorizations.InspectCurrentOperationReviewAuthorizationV5(context.WithoutCancel(ctx), request.Intent.Operation, request.Intent.ID, request.Authorization.ID)
	}
	return value, err
}

func reviewGateRetryableReadV1(err error) bool {
	return err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate))
}

func (c *ReviewGateControllerV1) closedFromAuthorizationErrorV1(request contract.ReviewGateRequestV1, err error, checked time.Time, actionExpiry int64) (contract.ReviewGateResultV1, error) {
	decision := contract.ReviewGateDeferV1
	if core.HasCategory(err, core.ErrorNotFound) || core.HasReason(err, core.ReasonReviewVerdictMissing) || core.HasReason(err, core.ReasonReviewConditionUnsatisfied) {
		decision = contract.ReviewGateAskV1
	} else if core.HasCategory(err, core.ErrorForbidden) {
		decision = contract.ReviewGateDenyV1
	}
	return c.closedResultV1(request, decision, err, checked, minimumReviewGateExpiryV1(request.RequestedNotAfterUnixNano, actionExpiry), "")
}

func (c *ReviewGateControllerV1) closedResultV1(request contract.ReviewGateRequestV1, decision contract.ReviewGatePhaseDecisionV1, cause error, checked time.Time, expires int64, projection core.Digest) (contract.ReviewGateResultV1, error) {
	category, reason := reviewGateErrorV1(cause)
	if expires <= checked.UnixNano() {
		return contract.ReviewGateResultV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Review Gate receipt TTL crossed its hard boundary")
	}
	return c.sealResultV1(request, decision, category, reason, checked, expires, projection)
}

func (c *ReviewGateControllerV1) sealResultV1(request contract.ReviewGateRequestV1, decision contract.ReviewGatePhaseDecisionV1, category core.ErrorCategory, reason core.ReasonCode, checked time.Time, expires int64, projection core.Digest) (contract.ReviewGateResultV1, error) {
	base := request.Action.Subject.Base
	var authorization *runtimeports.OperationReviewAuthorizationRefV5
	if request.Authorization != nil {
		copy := *request.Authorization
		authorization = &copy
	}
	receipt, err := contract.SealReviewGateReceiptV1(contract.ReviewGateReceiptV1{RunID: base.Run.RunID, SessionID: base.SessionID, SessionRevision: base.SessionRevision, SessionDigest: base.SessionDigest, Turn: base.Turn, ActionRef: base.PendingActionRef, ActionRequestDigest: request.Action.Subject.ApplicationBinding.Base.PendingAction.RequestDigest, Target: request.Target, Authorization: authorization, Basis: request.Basis, ReviewProjectionDigest: projection, Decision: decision, ErrorCategory: category, Reason: reason, CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		return contract.ReviewGateResultV1{}, err
	}
	result := contract.ReviewGateResultV1{ContractVersion: contract.ReviewGateContractVersionV1, Decision: decision, Receipt: receipt}
	return result, result.Validate()
}

func reviewGateErrorV1(err error) (core.ErrorCategory, core.ReasonCode) {
	var domain *core.DomainError
	if errors.As(err, &domain) {
		return domain.Category, domain.Reason
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome
	}
	return core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome
}

func minimumReviewGateExpiryV1(values ...int64) int64 {
	var minimum int64
	for _, value := range values {
		if value > 0 && (minimum == 0 || value < minimum) {
			minimum = value
		}
	}
	return minimum
}

func (c *ReviewGateControllerV1) freshNowV1(previous time.Time) (time.Time, error) {
	now := c.clock()
	if now.IsZero() || !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Gate clock is zero or moved backwards")
	}
	return now, nil
}

func reviewGateNilV1(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
