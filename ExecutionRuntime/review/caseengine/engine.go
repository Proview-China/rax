// Package caseengine owns Review Case, Round, Assignment, Finding and
// Attestation transitions. It has no external delivery or execution ability.
package caseengine

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type Clock func() time.Time

type Engine struct {
	store           reviewport.StoreV1
	clock           Clock
	recoveryTimeout time.Duration
}

const lostReplyRecoveryTimeoutV1 = 5 * time.Second

func New(store reviewport.StoreV1, clock Clock) (*Engine, error) {
	if nilcheck.IsNil(store) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "case engine requires store and clock")
	}
	return &Engine{store: store, clock: clock, recoveryTimeout: lostReplyRecoveryTimeoutV1}, nil
}

type CreateCaseCommandV1 struct {
	CaseID          string
	Request         *contract.ReviewRequestV1
	ResultBundle    *contract.ReviewResultBundleV1
	Target          contract.TargetSnapshotV1
	ExpiresUnixNano int64
	Trace           contract.TraceFactV1
}

func (e *Engine) CreateCaseV1(ctx context.Context, command CreateCaseCommandV1) (contract.ReviewCaseV1, error) {
	if err := command.Target.Validate(); err != nil {
		return contract.ReviewCaseV1{}, err
	}
	now := e.clock()
	if now.IsZero() || now.UnixNano() < command.Target.CreatedUnixNano {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "case creation clock regressed")
	}
	if command.ExpiresUnixNano > command.Target.ExpiresUnixNano || command.ExpiresUnixNano <= now.UnixNano() {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonReviewVerdictStale, "case expiry must be current and no later than target expiry")
	}
	var rubric *contract.ExactResourceRefV1
	if command.Request != nil {
		exact := command.Request.Rubric
		rubric = &exact
		if _, err := e.store.InspectRubricCurrentV1(ctx, command.Request.TenantID, exact, now); err != nil {
			return contract.ReviewCaseV1{}, err
		}
	}
	value, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: command.Target.TenantID, ID: command.CaseID, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, TargetID: command.Target.ID, TargetRevision: command.Target.Revision, TargetDigest: command.Target.Digest, Rubric: rubric, State: contract.CaseRequestedV1, ExpiresUnixNano: command.ExpiresUnixNano})
	if err != nil {
		return contract.ReviewCaseV1{}, err
	}
	fresh := e.clock()
	if fresh.IsZero() || fresh.Before(now) {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "case creation Rubric clock regressed")
	}
	if command.Request != nil {
		if _, err := e.store.InspectRubricCurrentV1(ctx, command.Request.TenantID, *rubric, fresh); err != nil {
			return contract.ReviewCaseV1{}, err
		}
	}
	mutation := reviewport.CreateTargetCaseMutationV1{Request: command.Request, ResultBundle: command.ResultBundle, Target: command.Target, Case: value, Trace: command.Trace, RubricCheckedUnixNano: fresh.UnixNano()}
	if err := reviewport.ValidateRequestedTraceV2(mutation); err != nil {
		return contract.ReviewCaseV1{}, err
	}
	created, err := e.store.CreateTargetCaseV1(ctx, mutation)
	if err != nil {
		if !unknownOutcome(err) {
			return contract.ReviewCaseV1{}, err
		}
		originalErr := err
		expiries := []int64{command.Target.ExpiresUnixNano, value.ExpiresUnixNano}
		if command.Request != nil {
			expiries = append(expiries, command.Request.ExpiresUnixNano)
		}
		if command.ResultBundle != nil {
			expiries = append(expiries, command.ResultBundle.ExpiresUnixNano)
		}
		recoveryCtx, cancel := e.lostReplyRecoveryContextV1(ctx, fresh, expiries...)
		defer cancel()
		target, inspectErr := e.store.InspectTargetExactV1(recoveryCtx, command.Target.TenantID, reviewport.ExactV1(command.Target.ID, command.Target.Revision, command.Target.Digest))
		if inspectErr != nil {
			return contract.ReviewCaseV1{}, originalErr
		}
		created, inspectErr = e.store.InspectCaseExactV1(recoveryCtx, value.TenantID, reviewport.ExactV1(value.ID, value.Revision, value.Digest))
		if inspectErr != nil {
			return contract.ReviewCaseV1{}, originalErr
		}
		trace, inspectErr := e.store.InspectTraceExactV1(recoveryCtx, command.Trace.TenantID, reviewport.ExactV1(command.Trace.ID, command.Trace.Revision, command.Trace.Digest))
		if inspectErr != nil {
			return contract.ReviewCaseV1{}, originalErr
		}
		if !reflect.DeepEqual(target, command.Target) || !reflect.DeepEqual(created, value) || !reflect.DeepEqual(trace, command.Trace) {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "lost-reply Case recovery changed canonical content")
		}
		if command.Request != nil {
			request, inspectErr := e.store.InspectRequestExactV1(recoveryCtx, command.Request.TenantID, reviewport.ExactV1(command.Request.ID, command.Request.Revision, command.Request.Digest))
			if inspectErr != nil {
				return contract.ReviewCaseV1{}, originalErr
			}
			if !reflect.DeepEqual(request, *command.Request) {
				return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "lost-reply Case recovery changed canonical Request")
			}
			if command.ResultBundle != nil {
				bundle, inspectErr := e.store.InspectResultBundleExactV1(recoveryCtx, command.ResultBundle.TenantID, reviewport.ExactV1(command.ResultBundle.ID, command.ResultBundle.Revision, command.ResultBundle.Digest))
				if inspectErr != nil {
					return contract.ReviewCaseV1{}, originalErr
				}
				if !reflect.DeepEqual(bundle, *command.ResultBundle) {
					return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "lost-reply Case recovery changed canonical Result Bundle")
				}
			}
		}
	}
	return created, nil
}

func (e *Engine) InspectCaseV1(ctx context.Context, tenant core.TenantID, id string) (contract.ReviewCaseV1, error) {
	return e.store.InspectCaseV1(ctx, tenant, id)
}

type TransitionCommandV1 struct {
	TenantID core.TenantID
	CaseID   string
	Expected reviewport.ExpectedFactV1
	Next     contract.CaseStateV1
	Reason   core.ReasonCode
}

func (e *Engine) TransitionV1(ctx context.Context, command TransitionCommandV1) (contract.ReviewCaseV1, error) {
	return contract.ReviewCaseV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "eventless Case transition V1 is unsupported; use TransitionWithTraceV2")
}

type TransitionWithTraceCommandV2 struct {
	TransitionCommandV1
	Trace contract.TraceFactV1
}

func (e *Engine) TransitionWithTraceV2(ctx context.Context, command TransitionWithTraceCommandV2) (contract.ReviewCaseV1, error) {
	current, err := e.store.InspectCaseV1(ctx, command.TenantID, command.CaseID)
	if err != nil {
		return contract.ReviewCaseV1{}, err
	}
	if current.Revision != command.Expected.Revision || current.Digest != command.Expected.Digest {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "case transition expected fact is stale")
	}
	if !contract.CanTransitionCaseV1(current.State, command.Next) {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidTransition, "case transition is not allowed")
	}
	now := e.clock()
	if now.IsZero() || now.UnixNano() <= current.UpdatedUnixNano {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "case transition time must advance")
	}
	next := current
	next.Revision++
	next.State = command.Next
	next.UpdatedUnixNano = now.UnixNano()
	next.Digest = ""
	if command.Next == contract.CaseExpiredV1 || command.Next == contract.CaseRevokedV1 || command.Next == contract.CaseSupersededV1 || command.Next == contract.CaseCancelledV1 || command.Next == contract.CaseIndeterminateV1 {
		if command.Reason == "" {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "terminal/indeterminate transition requires reason")
		}
		next.InvalidationReason = command.Reason
	}
	sealed, err := contract.SealReviewCaseV1(next)
	if err != nil {
		return contract.ReviewCaseV1{}, err
	}
	mutation := reviewport.TransitionCaseWithTraceMutationV2{Expected: command.Expected, Next: sealed, Trace: command.Trace}
	if err := reviewport.ValidateTransitionCaseTraceV2(mutation); err != nil {
		return contract.ReviewCaseV1{}, err
	}
	store, ok := e.store.(reviewport.CaseTransitionStoreV2)
	if !ok {
		return contract.ReviewCaseV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "compound Case transition Store capability is unavailable")
	}
	updated, err := store.TransitionCaseWithTraceV2(ctx, mutation)
	if err != nil && unknownOutcome(err) {
		originalErr := err
		recoveryCtx, cancel := e.lostReplyRecoveryContextV1(ctx, now, sealed.ExpiresUnixNano)
		defer cancel()
		updated, inspectErr := e.store.InspectCaseExactV1(recoveryCtx, sealed.TenantID, reviewport.ExactV1(sealed.ID, sealed.Revision, sealed.Digest))
		if inspectErr != nil {
			return contract.ReviewCaseV1{}, originalErr
		}
		trace, inspectErr := store.InspectTraceExactV1(recoveryCtx, command.Trace.TenantID, reviewport.ExactV1(command.Trace.ID, command.Trace.Revision, command.Trace.Digest))
		if inspectErr != nil {
			return contract.ReviewCaseV1{}, originalErr
		}
		if !reflect.DeepEqual(updated, sealed) || !reflect.DeepEqual(trace, command.Trace) {
			return contract.ReviewCaseV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "lost-reply Case transition recovery changed canonical content")
		}
		return updated, nil
	}
	return updated, err
}

func (e *Engine) StartRoundV1(ctx context.Context, mutation reviewport.StartRoundMutationV1) (contract.ReviewCaseV1, contract.ReviewRoundV1, contract.ReviewerAssignmentV1, error) {
	if mutation.Round.Rubric == nil || mutation.Round.Rubric.Validate() != nil {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "new Review Round requires an exact Rubric ref")
	}
	baseline := e.clock()
	if baseline.IsZero() {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Round Rubric baseline clock is unavailable")
	}
	first, err := e.store.InspectRubricCurrentV1(ctx, mutation.Round.TenantID, *mutation.Round.Rubric, baseline)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, err
	}
	fresh := e.clock()
	if fresh.IsZero() || fresh.Before(baseline) {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Round Rubric clock regressed")
	}
	second, err := e.store.InspectRubricCurrentV1(ctx, mutation.Round.TenantID, *mutation.Round.Rubric, fresh)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, err
	}
	if first.ExactRef() != second.ExactRef() || first.Digest != second.Digest {
		return contract.ReviewCaseV1{}, contract.ReviewRoundV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review Round Rubric drifted between S1 and S2")
	}
	mutation.RubricCheckedUnixNano = fresh.UnixNano()
	return e.store.StartRoundV1(ctx, mutation)
}
func (e *Engine) ClaimAssignmentV1(ctx context.Context, mutation reviewport.ClaimAssignmentMutationV1) (contract.ReviewCaseV1, contract.ReviewerAssignmentV1, error) {
	if len(mutation.Traces) != 1 || mutation.Traces[0].Event != contract.TraceStartedV1 {
		return contract.ReviewCaseV1{}, contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "case engine Claim requires exactly one ReviewStarted Trace")
	}
	return e.store.ClaimAssignmentV1(ctx, mutation)
}
func (e *Engine) CreateFindingWithTraceV2(ctx context.Context, mutation reviewport.CreateFindingWithTraceMutationV2) (contract.FindingV1, error) {
	store, ok := e.store.(reviewport.TraceEventStoreV2)
	if !ok {
		return contract.FindingV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "case engine Finding event Store capability is unavailable")
	}
	return store.CreateFindingWithTraceV2(ctx, mutation)
}
func (e *Engine) InspectFindingV1(ctx context.Context, tenant core.TenantID, id string) (contract.FindingV1, error) {
	return e.store.InspectFindingV1(ctx, tenant, id)
}

func AttestationNextStateV1(resolution contract.ResolutionV1) (contract.CaseStateV1, error) {
	return contract.AttestationNextCaseStateV1(resolution)
}

func (e *Engine) RecordAttestationV1(ctx context.Context, expected reviewport.ExpectedFactV1, attestation contract.AttestationV1, trace contract.TraceFactV1) (contract.ReviewCaseV1, contract.AttestationV1, error) {
	return e.RecordAttestationWithTraceV2(ctx, expected, attestation, trace, nil)
}

func (e *Engine) RecordAttestationWithTraceV2(ctx context.Context, expected reviewport.ExpectedFactV1, attestation contract.AttestationV1, trace contract.TraceFactV1, additional []contract.TraceFactV1) (contract.ReviewCaseV1, contract.AttestationV1, error) {
	next, err := AttestationNextStateV1(attestation.Resolution)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
	}
	if next == contract.CaseWaitingHumanV1 {
		if len(additional) != 1 || additional[0].Event != contract.TraceEscalatedV1 {
			return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "escalating Attestation requires exactly one Escalated Trace")
		}
	} else if len(additional) != 0 {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonInvalidState, "non-escalating Attestation cannot publish an Escalated Trace")
	}
	current, err := e.store.InspectCaseExactV1(ctx, attestation.TenantID, reviewport.ExactV1(attestation.CaseID, expected.Revision, expected.Digest))
	if err != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
	}
	nextCase := current
	nextCase.Revision++
	nextCase.State = next
	nextCase.UpdatedUnixNano = attestation.ObservedUnixNano
	nextCase.Digest = ""
	nextCase, err = contract.SealReviewCaseV1(nextCase)
	if err != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, err
	}
	caseFact, recorded, err := e.store.RecordAttestationV1(ctx, reviewport.RecordAttestationMutationV1{Expected: expected, Attestation: attestation, NextState: next, Trace: trace, AdditionalTraces: append([]contract.TraceFactV1(nil), additional...)})
	if err == nil || !unknownOutcome(err) {
		return caseFact, recorded, err
	}
	originalErr := err
	recoveryCtx, cancel := e.lostReplyRecoveryContextV1(ctx, time.Unix(0, attestation.ObservedUnixNano), nextCase.ExpiresUnixNano, attestation.ExpiresUnixNano)
	defer cancel()
	recorded, inspectErr := e.store.InspectAttestationByIdempotencyV1(recoveryCtx, attestation.TenantID, attestation.IdempotencyKey)
	if inspectErr != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, originalErr
	}
	if recorded.ID != attestation.ID || recorded.Revision != attestation.Revision || recorded.Digest != attestation.Digest {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "lost-reply attestation recovery changed canonical payload")
	}
	caseFact, inspectErr = e.store.InspectCaseExactV1(recoveryCtx, nextCase.TenantID, reviewport.ExactV1(nextCase.ID, nextCase.Revision, nextCase.Digest))
	if inspectErr != nil {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, originalErr
	}
	if !reflect.DeepEqual(caseFact, nextCase) || !reflect.DeepEqual(recorded, attestation) {
		return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "lost-reply attestation recovery changed canonical content")
	}
	for _, event := range append([]contract.TraceFactV1{trace}, additional...) {
		inspected, inspectErr := e.store.InspectTraceExactV1(recoveryCtx, event.TenantID, reviewport.ExactV1(event.ID, event.Revision, event.Digest))
		if inspectErr != nil {
			return contract.ReviewCaseV1{}, contract.AttestationV1{}, originalErr
		}
		if !reflect.DeepEqual(inspected, event) {
			return contract.ReviewCaseV1{}, contract.AttestationV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "lost-reply attestation recovery changed canonical Trace")
		}
	}
	return caseFact, recorded, nil
}

func unknownOutcome(err error) bool {
	return core.HasCategory(err, core.ErrorUnavailable) || core.HasCategory(err, core.ErrorIndeterminate)
}

func (e *Engine) lostReplyRecoveryContextV1(ctx context.Context, now time.Time, expiries ...int64) (context.Context, context.CancelFunc) {
	timeout := e.recoveryTimeout
	if timeout <= 0 || timeout > lostReplyRecoveryTimeoutV1 {
		timeout = lostReplyRecoveryTimeoutV1
	}
	for _, expiry := range expiries {
		if expiry <= 0 {
			continue
		}
		if remaining := time.Unix(0, expiry).Sub(now); remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		timeout = time.Nanosecond
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}
