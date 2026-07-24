// Package hostadapter contains Review-owned adapters for Host public contracts.
// It imports no Host implementation and performs no Model or Runtime effect.
package hostadapter

import (
	"context"
	"errors"
	"reflect"
	"time"

	hostcontract "github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ClockV1 func() time.Time

type AutoReviewerAttemptCurrentReaderV1 interface {
	InspectAutoReviewerAttemptExactV1(context.Context, core.TenantID, contract.ExactResourceRefV1) (contract.AutoReviewerAttemptV1, error)
	InspectAutoReviewerAttemptCurrentV1(context.Context, core.TenantID, string) (contract.AutoReviewerAttemptV1, error)
}

type ReviewModelAssociationRequestV1 struct {
	Attempt contract.AutoReviewerAttemptV1                `json:"attempt"`
	Command modelinvoker.GovernedModelInvocationCommandV1 `json:"command"`
}

type ReviewModelAssociationAdapterV1 struct {
	attempts     AutoReviewerAttemptCurrentReaderV1
	associations hostports.ReviewModelInvocationAssociationPortV1
	clock        ClockV1
}

func NewReviewModelAssociationAdapterV1(attempts AutoReviewerAttemptCurrentReaderV1, associations hostports.ReviewModelInvocationAssociationPortV1, clock ClockV1) (*ReviewModelAssociationAdapterV1, error) {
	if nilcheck.IsNil(attempts) || nilcheck.IsNil(associations) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review Host association adapter requires Attempt reader, Host association Port and clock")
	}
	return &ReviewModelAssociationAdapterV1{attempts: attempts, associations: associations, clock: clock}, nil
}

// StartOrInspectAssociationV1 creates at most one canonical Host association.
// Unknown mutation outcomes recover only by exact historical Inspect; Create is
// never replayed. S1 Resolve and S2 exact Inspect both reread Review current.
func (a *ReviewModelAssociationAdapterV1) StartOrInspectAssociationV1(ctx context.Context, request ReviewModelAssociationRequestV1) (hostcontract.ReviewModelInvocationAssociationFactV1, error) {
	baseline := a.clock()
	if baseline.IsZero() {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, associationClockError("Review Host association baseline clock is unavailable")
	}
	if err := validateAssociationRequestV1(request, baseline); err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	recoveryCtx, recoveryCancel, recoveryReady := associationRecoveryContextV1(ctx, baseline, request.Attempt.ExpiresUnixNano, request.Command.CurrentRef.ExpiresUnixNano, request.Command.CurrentRef.NotAfterUnixNano)
	if recoveryReady {
		defer recoveryCancel()
	}
	if _, err := a.readAttemptCurrentV1(ctx, recoveryCtx, request.Attempt, baseline); err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	desired, err := desiredAssociationV1(request)
	if err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	beforeCreate, err := a.freshAfterV1(baseline)
	if err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	if _, err = a.readAttemptCurrentV1(ctx, recoveryCtx, request.Attempt, beforeCreate); err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	if err = desired.ValidateCurrentV1(beforeCreate); err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, mapHostErrorV1(err)
	}
	receipt, createErr := a.associations.CreateReviewModelInvocationAssociationV1(ctx, desired)
	if createErr != nil {
		if !recoverableHostMutationV1(createErr) {
			return hostcontract.ReviewModelInvocationAssociationFactV1{}, mapHostErrorV1(createErr)
		}
		originalUnknown := createErr
		if !recoveryReady {
			return hostcontract.ReviewModelInvocationAssociationFactV1{}, mapHostErrorV1(originalUnknown)
		}
		recovered, inspectErr := a.associations.InspectHistoricalReviewModelInvocationAssociationV1(recoveryCtx, desired.RefV1())
		if inspectErr != nil || !reflect.DeepEqual(recovered, desired) {
			return hostcontract.ReviewModelInvocationAssociationFactV1{}, mapHostErrorV1(originalUnknown)
		}
		receipt.Fact = recovered
	} else if !reflect.DeepEqual(receipt.Fact, desired) {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Host association create receipt changed canonical content")
	}
	afterCreate, err := a.freshAfterV1(beforeCreate)
	if err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	return a.verifyS1S2V1(ctx, recoveryCtx, request.Attempt, desired, afterCreate)
}

// InspectCurrentAssociationV1 is read-only and never falls back to Create.
func (a *ReviewModelAssociationAdapterV1) InspectCurrentAssociationV1(ctx context.Context, attempt contract.AutoReviewerAttemptV1, expected hostcontract.ReviewModelInvocationAssociationRefV1) (hostcontract.ReviewModelInvocationAssociationFactV1, error) {
	baseline := a.clock()
	if baseline.IsZero() {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, associationClockError("Review Host association Inspect baseline clock is unavailable")
	}
	if err := attempt.ValidateCurrent(baseline); err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	recoveryCtx, recoveryCancel, recoveryReady := associationRecoveryContextV1(ctx, baseline, attempt.ExpiresUnixNano)
	if recoveryReady {
		defer recoveryCancel()
	}
	subject := mapAttemptSubjectV1(attempt)
	if expected.Subject != subject {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Host association expected Ref belongs to another Review Attempt")
	}
	if _, err := a.readAttemptCurrentV1(ctx, recoveryCtx, attempt, baseline); err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	first, err := a.inspectHostCurrentV1(ctx, recoveryCtx, subject, expected)
	if err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	afterS1, err := a.freshAfterV1(baseline)
	if err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	s2Ctx, s2Cancel, s2Ready := associationTightenRecoveryV1(recoveryCtx, afterS1, attempt.ExpiresUnixNano, first.ExpiresUnixNano)
	if !s2Ready {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Host association S1 snapshot expired before S2")
	}
	defer s2Cancel()
	if _, err = a.readAttemptCurrentV1(ctx, s2Ctx, attempt, afterS1); err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	second, err := a.inspectHostCurrentV1(ctx, s2Ctx, subject, expected)
	if err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	actual, err := a.freshAfterV1(afterS1)
	if err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	if !reflect.DeepEqual(first, second) {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Host association drifted between S1 and S2")
	}
	if err = second.ValidateCurrentV1(actual); err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, mapHostErrorV1(err)
	}
	return second, nil
}

func (a *ReviewModelAssociationAdapterV1) verifyS1S2V1(ctx, recoveryCtx context.Context, attempt contract.AutoReviewerAttemptV1, desired hostcontract.ReviewModelInvocationAssociationFactV1, previous time.Time) (hostcontract.ReviewModelInvocationAssociationFactV1, error) {
	if _, err := a.readAttemptCurrentV1(ctx, recoveryCtx, attempt, previous); err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	subject := mapAttemptSubjectV1(attempt)
	ref, err := a.resolveHostCurrentV1(ctx, recoveryCtx, subject)
	if err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	first, err := a.inspectHostCurrentV1(ctx, recoveryCtx, subject, ref)
	if err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	afterS1, err := a.freshAfterV1(previous)
	if err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	if _, err = a.readAttemptCurrentV1(ctx, recoveryCtx, attempt, afterS1); err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	second, err := a.inspectHostCurrentV1(ctx, recoveryCtx, subject, ref)
	if err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	actual, err := a.freshAfterV1(afterS1)
	if err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, err
	}
	if ref != desired.RefV1() || !reflect.DeepEqual(first, second) || !reflect.DeepEqual(second, desired) {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Host association S1/S2 differs from the canonical Review request")
	}
	if err = second.ValidateCurrentV1(actual); err != nil {
		return hostcontract.ReviewModelInvocationAssociationFactV1{}, mapHostErrorV1(err)
	}
	return second, nil
}

func (a *ReviewModelAssociationAdapterV1) readAttemptCurrentV1(ctx, recoveryCtx context.Context, expected contract.AutoReviewerAttemptV1, now time.Time) (contract.AutoReviewerAttemptV1, error) {
	read := func(readContext context.Context) (contract.AutoReviewerAttemptV1, error) {
		historical, err := a.attempts.InspectAutoReviewerAttemptExactV1(readContext, expected.TenantID, expected.ExactRef())
		if err != nil {
			return contract.AutoReviewerAttemptV1{}, err
		}
		current, err := a.attempts.InspectAutoReviewerAttemptCurrentV1(readContext, expected.TenantID, expected.ID)
		if err != nil {
			return contract.AutoReviewerAttemptV1{}, err
		}
		if !reflect.DeepEqual(historical, expected) || current.ExactRef() != expected.ExactRef() {
			return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Auto Attempt exact/current coordinate drifted")
		}
		return current, nil
	}
	current, err := read(ctx)
	if err != nil && (core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)) {
		originalUnknown := err
		if recoveryCtx == nil {
			return contract.AutoReviewerAttemptV1{}, originalUnknown
		}
		current, err = read(recoveryCtx)
		if err != nil || recoveryCtx.Err() != nil {
			return contract.AutoReviewerAttemptV1{}, originalUnknown
		}
	}
	if err != nil {
		return contract.AutoReviewerAttemptV1{}, err
	}
	if err = current.ValidateCurrent(now); err != nil {
		return contract.AutoReviewerAttemptV1{}, err
	}
	return current, nil
}
func (a *ReviewModelAssociationAdapterV1) resolveHostCurrentV1(ctx, recoveryCtx context.Context, subject hostcontract.ReviewModelInvocationAssociationSubjectV1) (hostcontract.ReviewModelInvocationAssociationRefV1, error) {
	read := func(c context.Context) (hostcontract.ReviewModelInvocationAssociationRefV1, error) {
		return a.associations.ResolveCurrentReviewModelInvocationAssociationV1(c, subject)
	}
	ref, err := read(ctx)
	if err != nil && recoverableHostReadV1(err) {
		// Resolve has no expected Ref. A retry is a new S1 lookup, not recovery
		// of the unknown prior result.
		originalUnknown := err
		if recoveryCtx == nil {
			return ref, mapHostErrorV1(originalUnknown)
		}
		ref, err = read(recoveryCtx)
		if err != nil || recoveryCtx.Err() != nil {
			return ref, mapHostErrorV1(originalUnknown)
		}
	}
	if err != nil {
		return ref, mapHostErrorV1(err)
	}
	return ref, nil
}
func (a *ReviewModelAssociationAdapterV1) inspectHostCurrentV1(ctx, recoveryCtx context.Context, subject hostcontract.ReviewModelInvocationAssociationSubjectV1, ref hostcontract.ReviewModelInvocationAssociationRefV1) (hostcontract.ReviewModelInvocationAssociationFactV1, error) {
	read := func(c context.Context) (hostcontract.ReviewModelInvocationAssociationFactV1, error) {
		return a.associations.InspectCurrentReviewModelInvocationAssociationV1(c, subject, ref)
	}
	value, err := read(ctx)
	if err != nil && recoverableHostReadV1(err) {
		originalUnknown := err
		if recoveryCtx == nil {
			return value, mapHostErrorV1(originalUnknown)
		}
		value, err = read(recoveryCtx)
		if err != nil || recoveryCtx.Err() != nil {
			return value, mapHostErrorV1(originalUnknown)
		}
	}
	if err != nil {
		return value, mapHostErrorV1(err)
	}
	return value, nil
}

func associationRecoveryContextV1(parent context.Context, now time.Time, expiries ...int64) (context.Context, context.CancelFunc, bool) {
	if now.IsZero() {
		return nil, nil, false
	}
	limit := 5 * time.Second
	for _, expiry := range expiries {
		if expiry <= 0 {
			continue
		}
		remaining := time.Unix(0, expiry).Sub(now)
		if remaining <= 0 {
			return nil, nil, false
		}
		if remaining < limit {
			limit = remaining
		}
	}
	recovery, cancel := context.WithTimeout(context.WithoutCancel(parent), limit)
	return recovery, cancel, true
}

func associationTightenRecoveryV1(parent context.Context, now time.Time, expiries ...int64) (context.Context, context.CancelFunc, bool) {
	if parent == nil || now.IsZero() {
		return nil, nil, false
	}
	var limit time.Duration
	for _, expiry := range expiries {
		if expiry <= 0 {
			continue
		}
		remaining := time.Unix(0, expiry).Sub(now)
		if remaining <= 0 {
			return nil, nil, false
		}
		if limit == 0 || remaining < limit {
			limit = remaining
		}
	}
	if limit == 0 {
		limit = 5 * time.Second
	}
	tightened, cancel := context.WithTimeout(parent, limit)
	return tightened, cancel, true
}
func (a *ReviewModelAssociationAdapterV1) freshAfterV1(previous time.Time) (time.Time, error) {
	now := a.clock()
	if now.IsZero() || now.Before(previous) {
		return time.Time{}, associationClockError("Review Host association clock regressed")
	}
	return now, nil
}

func validateAssociationRequestV1(request ReviewModelAssociationRequestV1, now time.Time) error {
	if err := request.Attempt.ValidateCurrent(now); err != nil {
		return err
	}
	if request.Attempt.Revision != 1 || request.Attempt.State != contract.AutoReviewerAttemptPreparedV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Host association requires canonical prepared Auto Attempt revision one")
	}
	return nil
}
func desiredAssociationV1(request ReviewModelAssociationRequestV1) (hostcontract.ReviewModelInvocationAssociationFactV1, error) {
	checked := request.Attempt.UpdatedUnixNano
	if request.Command.CurrentRef.CheckedUnixNano > checked {
		checked = request.Command.CurrentRef.CheckedUnixNano
	}
	expires := minPositiveV1(request.Attempt.ExpiresUnixNano, request.Command.CurrentRef.ExpiresUnixNano, request.Command.CurrentRef.NotAfterUnixNano)
	return hostcontract.SealReviewModelInvocationAssociationV1(hostcontract.ReviewModelInvocationAssociationFactV1{Subject: mapAttemptSubjectV1(request.Attempt), Command: request.Command, CheckedUnixNano: checked, ExpiresUnixNano: expires})
}
func mapAttemptSubjectV1(attempt contract.AutoReviewerAttemptV1) hostcontract.ReviewModelInvocationAssociationSubjectV1 {
	ref := attempt.ExactRef()
	return hostcontract.ReviewModelInvocationAssociationSubjectV1{TenantID: attempt.TenantID, ReviewAttempt: hostcontract.ReviewAttemptExactCoordinateV1{ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest}}
}
func minPositiveV1(values ...int64) int64 {
	var result int64
	for _, value := range values {
		if value > 0 && (result == 0 || value < result) {
			result = value
		}
	}
	return result
}
func recoverableHostMutationV1(err error) bool {
	return hostcontract.HasCode(err, hostcontract.ErrorUnknownOutcome) || hostcontract.HasCode(err, hostcontract.ErrorUnavailable) || hostcontract.HasCode(err, hostcontract.ErrorConflict)
}
func recoverableHostReadV1(err error) bool {
	return hostcontract.HasCode(err, hostcontract.ErrorUnknownOutcome) || hostcontract.HasCode(err, hostcontract.ErrorUnavailable)
}
func mapHostErrorV1(err error) error {
	if err == nil {
		return nil
	}
	var hostErr *hostcontract.Error
	if errors.As(err, &hostErr) && hostErr.Code == hostcontract.ErrorPrecondition && hostErr.Reason == "clock_regression" {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, err.Error())
	}
	switch {
	case hostcontract.HasCode(err, hostcontract.ErrorInvalidArgument):
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, err.Error())
	case hostcontract.HasCode(err, hostcontract.ErrorConflict):
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, err.Error())
	case hostcontract.HasCode(err, hostcontract.ErrorNotFound):
		return core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, err.Error())
	case hostcontract.HasCode(err, hostcontract.ErrorUnavailable):
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, err.Error())
	case hostcontract.HasCode(err, hostcontract.ErrorPrecondition):
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, err.Error())
	case hostcontract.HasCode(err, hostcontract.ErrorUnknownOutcome):
		return core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, err.Error())
	default:
		return err
	}
}
func associationClockError(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, message)
}
