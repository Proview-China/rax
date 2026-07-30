package applicationadapter

import (
	"context"
	"errors"
	"reflect"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type nextModelTurnDispatchBindingRepositoryV1 interface {
	ensureNextModelTurnDispatchBindingV1(
		context.Context,
		applicationcontract.NextModelTurnDispatchRequestV1,
		applicationcontract.NextModelTurnDispatchCurrentV1,
	) (applicationcontract.NextModelTurnDispatchCurrentV1, error)
	inspectNextModelTurnDispatchBindingV1(
		context.Context,
		applicationcontract.NextModelTurnDispatchInspectRequestV1,
	) (applicationcontract.NextModelTurnDispatchCurrentV1, error)
}

type NextModelTurnDispatchBindingConfigV1 struct {
	Continuations applicationports.TurnContinuationCurrentReaderV1
	Bindings      nextModelTurnDispatchBindingRepositoryV1
	Clock         func() time.Time
}

// NextModelTurnDispatchBindingV1 fresh-reads the Harness-owned continuation
// on both sides of candidate construction and persists only the stable
// Application-derived attempt_bound binding. It has no Model, Runtime guard,
// Provider, Harness-private dispatch, or Console dependency.
type NextModelTurnDispatchBindingV1 struct {
	continuations applicationports.TurnContinuationCurrentReaderV1
	bindings      nextModelTurnDispatchBindingRepositoryV1
	clock         func() time.Time
}

func NewNextModelTurnDispatchBindingV1(
	config NextModelTurnDispatchBindingConfigV1,
) (*NextModelTurnDispatchBindingV1, error) {
	if nextModelTurnDispatchNilV1(config.Continuations) ||
		nextModelTurnDispatchNilV1(config.Bindings) ||
		config.Clock == nil {
		return nil, nextModelTurnDispatchErrorV1(core.ErrorInvalidArgument, core.ReasonComponentMissing, "next-turn dispatch binding requires Continuation, repository, and clock")
	}
	return &NextModelTurnDispatchBindingV1{
		continuations: config.Continuations,
		bindings:      config.Bindings,
		clock:         config.Clock,
	}, nil
}

func (b *NextModelTurnDispatchBindingV1) StartOrInspectNextModelTurnV1(
	ctx context.Context,
	request applicationcontract.NextModelTurnDispatchRequestV1,
) (applicationcontract.NextModelTurnDispatchCurrentV1, error) {
	if err := b.preflightV1(ctx); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	now, err := freshNextModelTurnDispatchTimeV1(b.clock, time.Time{})
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	inspect, err := applicationcontract.NewNextModelTurnDispatchInspectRequestV1(request)
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	stored, err := b.bindings.inspectNextModelTurnDispatchBindingV1(ctx, inspect)
	if err == nil {
		if err = stored.ValidateCurrentFor(request, now); err != nil {
			return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
		}
		return stored, nil
	}
	if !core.HasCategory(err, core.ErrorNotFound) {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}

	s1, s1Now, err := b.inspectContinuationV1(ctx, request, now)
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if err = nextModelTurnDispatchContextV1(ctx); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	s2, s2Now, err := b.inspectContinuationV1(ctx, request, s1Now)
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonBindingDrift, "next-turn Continuation drifted between S1 and S2")
	}
	if err = nextModelTurnDispatchContextV1(ctx); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	finalNow, err := freshNextModelTurnDispatchTimeV1(b.clock, s2Now)
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if err = request.ValidateCurrent(finalNow); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	notAfter := applicationcontract.NextModelTurnDispatchNotAfterV1(
		request.RequestedNotAfterUnixNano,
		request.EligibilityRequest.RequestedNotAfterUnixNano,
		request.EligibilityRequest.Session.ExpiresUnixNano,
		request.EligibilityRequest.RuntimeActualPoint.ModelBoundary.ExpiresUnixNano,
		request.EligibilityProjection.NotAfterUnixNano,
		s1.ExpiresUnixNano,
		s2.ExpiresUnixNano,
	)
	if !finalNow.Before(time.Unix(0, notAfter)) {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, nextModelTurnDispatchErrorV1(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "next-turn exact closure crossed its common TTL")
	}
	checked := nextModelTurnDispatchCheckedUpperBoundV1(request, s1, s2)
	current, err := applicationcontract.SealNextModelTurnDispatchAttemptBoundV1(
		request,
		checked,
		notAfter,
	)
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if err = current.ValidateCurrentFor(request, finalNow); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if err = nextModelTurnDispatchContextV1(ctx); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	writeCtx, writeCancel := context.WithDeadline(ctx, time.Unix(0, notAfter))
	persisted, err := b.bindings.ensureNextModelTurnDispatchBindingV1(writeCtx, request, current)
	writeCancel()
	if err != nil {
		if !core.HasCategory(err, core.ErrorIndeterminate) &&
			!core.HasCategory(err, core.ErrorUnavailable) &&
			!core.HasCategory(err, core.ErrorConflict) {
			return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
		}
		recovery, cancel := nextModelTurnDispatchRecoveryContextV1(ctx, notAfter)
		recovered, inspectErr := b.bindings.inspectNextModelTurnDispatchBindingV1(recovery, inspect)
		cancel()
		if inspectErr != nil {
			return applicationcontract.NextModelTurnDispatchCurrentV1{}, errors.Join(err, inspectErr)
		}
		persisted = recovered
	}
	if persisted != current {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "next-turn repository returned another durable binding")
	}
	return persisted, nil
}

func (b *NextModelTurnDispatchBindingV1) InspectNextModelTurnV1(
	ctx context.Context,
	request applicationcontract.NextModelTurnDispatchInspectRequestV1,
) (applicationcontract.NextModelTurnDispatchCurrentV1, error) {
	if err := b.preflightV1(ctx); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if err := request.Validate(); err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	current, err := b.bindings.inspectNextModelTurnDispatchBindingV1(ctx, request)
	if err != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, err
	}
	if current.DerivedDispatch != request.DerivedDispatch ||
		current.RequestDigest != request.RequestDigest ||
		validateNextModelTurnDispatchAttemptBoundV1(current) != nil {
		return applicationcontract.NextModelTurnDispatchCurrentV1{}, nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonBindingDrift, "next-turn Inspect returned another exact binding")
	}
	return current, nil
}

func (b *NextModelTurnDispatchBindingV1) inspectContinuationV1(
	ctx context.Context,
	request applicationcontract.NextModelTurnDispatchRequestV1,
	previous time.Time,
) (applicationcontract.TurnContinuationCurrentV1, time.Time, error) {
	if err := nextModelTurnDispatchContextV1(ctx); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, time.Time{}, err
	}
	now, err := freshNextModelTurnDispatchTimeV1(b.clock, previous)
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, time.Time{}, err
	}
	if err = request.ValidateCurrent(now); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, time.Time{}, err
	}
	inspect, err := applicationcontract.SealTurnContinuationInspectRequestV1(
		applicationcontract.TurnContinuationInspectRequestV1{
			AttemptRef: request.EligibilityRequest.ContinuationAttempt,
		},
	)
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, time.Time{}, err
	}
	current, err := b.continuations.InspectTurnContinuationV1(ctx, inspect)
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, time.Time{}, err
	}
	fresh, err := freshNextModelTurnDispatchTimeV1(b.clock, now)
	if err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, time.Time{}, err
	}
	if err = validateNextModelTurnDispatchContinuationV1(current, request, fresh); err != nil {
		return applicationcontract.TurnContinuationCurrentV1{}, time.Time{}, err
	}
	return current, fresh, nil
}

func validateNextModelTurnDispatchContinuationV1(
	current applicationcontract.TurnContinuationCurrentV1,
	request applicationcontract.NextModelTurnDispatchRequestV1,
	now time.Time,
) error {
	eligibility := request.EligibilityRequest
	if err := current.ModelTurnAllowedV1(now); err != nil {
		return err
	}
	if current.Start.AttemptRefV1() != eligibility.ContinuationAttempt ||
		current.Digest != eligibility.ContinuationCurrentDigest ||
		current.ActiveContext != eligibility.ActiveContext ||
		current.Start.ExecutionScopeDigest != eligibility.ContinuationAttempt.ExecutionScopeDigest ||
		current.Start.RunID != eligibility.Run.RunID ||
		current.Start.Source.Session != eligibility.Session ||
		current.Start.TargetTurn != eligibility.TargetTurn ||
		request.EligibilityProjection.ContinuationCurrentDigest != current.Digest ||
		request.EligibilityProjection.ActiveContextDigest != current.ActiveContext.Digest {
		return nextModelTurnDispatchErrorV1(core.ErrorConflict, core.ReasonEvidenceConflict, "next-turn Continuation differs from the exact request")
	}
	return nil
}

func nextModelTurnDispatchCheckedUpperBoundV1(
	request applicationcontract.NextModelTurnDispatchRequestV1,
	currents ...applicationcontract.TurnContinuationCurrentV1,
) int64 {
	values := []int64{
		request.EligibilityRequest.Session.CheckedUnixNano,
		request.EligibilityProjection.CheckedUnixNano,
	}
	for _, current := range currents {
		values = append(values, current.CheckedUnixNano)
	}
	var maximum int64
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func (b *NextModelTurnDispatchBindingV1) preflightV1(ctx context.Context) error {
	if b == nil || nextModelTurnDispatchNilV1(b.continuations) ||
		nextModelTurnDispatchNilV1(b.bindings) || b.clock == nil {
		return nextModelTurnDispatchErrorV1(core.ErrorUnavailable, core.ReasonComponentMissing, "next-turn dispatch binding is unavailable")
	}
	return nextModelTurnDispatchContextV1(ctx)
}

func freshNextModelTurnDispatchTimeV1(clock func() time.Time, previous time.Time) (time.Time, error) {
	now := clock()
	if now.IsZero() || now.UnixNano() <= 0 ||
		(!previous.IsZero() && now.Before(previous)) {
		return time.Time{}, nextModelTurnDispatchErrorV1(core.ErrorPreconditionFailed, core.ReasonClockRegression, "next-turn dispatch clock regressed")
	}
	return now, nil
}

func nextModelTurnDispatchRecoveryContextV1(
	ctx context.Context,
	notAfterUnixNano int64,
) (context.Context, context.CancelFunc) {
	return context.WithDeadline(
		context.WithoutCancel(ctx),
		time.Unix(0, notAfterUnixNano),
	)
}

func nextModelTurnDispatchContextV1(ctx context.Context) error {
	if ctx == nil {
		return nextModelTurnDispatchErrorV1(core.ErrorInvalidArgument, core.ReasonInvalidReference, "next-turn dispatch context is required")
	}
	if ctx.Err() != nil {
		return nextModelTurnDispatchErrorV1(core.ErrorUnavailable, core.ReasonInvalidState, "next-turn dispatch context is canceled")
	}
	return nil
}

func nextModelTurnDispatchNilV1(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}

func nextModelTurnDispatchErrorV1(
	category core.ErrorCategory,
	reason core.ReasonCode,
	message string,
) error {
	return core.NewError(category, reason, message)
}

var _ applicationports.NextModelTurnDispatchPortV1 = (*NextModelTurnDispatchBindingV1)(nil)
