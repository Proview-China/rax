// Package autoreviewer owns the Review-side coordination of one governed
// Auto Reviewer attempt. It never invokes a model provider directly and never
// creates Runtime authorization, dispatch, permit, settlement, or outcome
// facts.
package autoreviewer

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/nilcheck"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/reviewer"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type Clock func() time.Time

const defaultRecoveryTimeoutV1 = 5 * time.Second

// storeV1 is the narrow Review-owned persistence boundary required by the
// coordinator. Exact DomainResult Inspect is included so a lost mutation reply
// can be recovered without replaying the mutation.
type storeV1 interface {
	reviewport.AutoReviewerStoreV1
	reviewport.RubricCurrentReaderV1
	InspectDomainResultExactV1(context.Context, core.TenantID, reviewport.ExactFactRefV1) (contract.ReviewerInvocationResultFactV1, error)
}

type Owner struct {
	store           storeV1
	invocation      reviewport.AutoReviewerInvocationPortV1
	contextReader   reviewport.ReviewerContextCurrentReaderV1
	schemaReader    reviewport.AutoReviewerOutputSchemaReaderV1
	clock           Clock
	recoveryTimeout time.Duration
}

// NewProduction requires the Context Owner exact-current Reader. Unlike New,
// it never treats the caller-supplied legacy ContextFrameV1 as current truth.
func NewProduction(store storeV1, invocation reviewport.AutoReviewerInvocationPortV1, contextReader reviewport.ReviewerContextCurrentReaderV1, schemaReader reviewport.AutoReviewerOutputSchemaReaderV1, clock Clock) (*Owner, error) {
	if nilcheck.IsNil(store) || nilcheck.IsNil(invocation) || nilcheck.IsNil(contextReader) || nilcheck.IsNil(schemaReader) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "production Auto Reviewer Owner requires Store, invocation Port, Context current Reader, output Schema Reader and clock")
	}
	return &Owner{store: store, invocation: invocation, contextReader: contextReader, schemaReader: schemaReader, clock: clock, recoveryTimeout: defaultRecoveryTimeoutV1}, nil
}

func New(store storeV1, invocation reviewport.AutoReviewerInvocationPortV1, clock Clock) (*Owner, error) {
	if nilcheck.IsNil(store) || nilcheck.IsNil(invocation) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Auto Reviewer Owner requires Store, invocation Port and clock")
	}
	schemaReader, err := reviewer.NewBuiltinOutputSchemaReaderV1()
	if err != nil {
		return nil, err
	}
	return &Owner{store: store, invocation: invocation, schemaReader: schemaReader, clock: clock, recoveryTimeout: defaultRecoveryTimeoutV1}, nil
}

// RunCommandV1 contains only Review-owned, already sealed inputs. ResultID is
// the stable create-once identity for the Review DomainResult; retries must use
// the same value.
type RunCommandV1 struct {
	Attempt  contract.AutoReviewerAttemptV1
	Context  reviewer.ContextFrameV1
	ResultID string
}

type RunResultV1 struct {
	Attempt      contract.AutoReviewerAttemptV1
	Observation  *contract.AutoReviewerInvocationObservationV1
	DomainResult *contract.ReviewerInvocationResultFactV1
}

// RunV1 is start-or-inspect at the Review Owner boundary. A prepared attempt
// may call StartOrInspect exactly with its canonical command. Once the Review
// state reaches waiting_inspect, every continuation calls exact Inspect only.
func (o *Owner) RunV1(ctx context.Context, command RunCommandV1) (RunResultV1, error) {
	baseline := o.clock()
	if baseline.IsZero() {
		return RunResultV1{}, clockError("Auto Reviewer baseline clock is unavailable")
	}
	if err := o.validateCommandV1(command, baseline); err != nil {
		return RunResultV1{}, err
	}

	current, err := o.ensureAttemptV1(ctx, command.Attempt, baseline)
	if err != nil {
		return RunResultV1{}, err
	}
	current, now, err := o.validateOrRefreshCurrentV1(ctx, command, current, baseline)
	if err != nil {
		return RunResultV1{}, err
	}

	switch current.State {
	case contract.AutoReviewerAttemptObservedV1:
		if current.DomainResult == nil || current.DomainResult.ID != command.ResultID {
			return RunResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Auto Reviewer replay changed the stable DomainResult identity")
		}
		return o.inspectObservedV1(ctx, current)
	case contract.AutoReviewerAttemptFailedClosedV1, contract.AutoReviewerAttemptEscalatedV1:
		return RunResultV1{Attempt: current}, nil
	case contract.AutoReviewerAttemptPreparedV1, contract.AutoReviewerAttemptWaitingInspectV1:
	default:
		return RunResultV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "Auto Reviewer current Attempt state is not executable")
	}

	// This is the Review-side actual point. It repeats all available current
	// reads with a fresh clock immediately before crossing the injected Port.
	current, actual, err := o.validateOrRefreshCurrentV1(ctx, command, current, now)
	if err != nil {
		return RunResultV1{}, err
	}
	// Another same-canonical caller may have advanced the Attempt between S1
	// and the actual point. Honor that current state read-only before deciding
	// whether the invocation boundary may still be crossed.
	switch current.State {
	case contract.AutoReviewerAttemptObservedV1:
		if current.DomainResult == nil || current.DomainResult.ID != command.ResultID {
			return RunResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Auto Reviewer replay changed the stable DomainResult identity")
		}
		return o.inspectObservedV1(ctx, current)
	case contract.AutoReviewerAttemptFailedClosedV1, contract.AutoReviewerAttemptEscalatedV1:
		return RunResultV1{Attempt: current}, nil
	case contract.AutoReviewerAttemptPreparedV1, contract.AutoReviewerAttemptWaitingInspectV1:
	default:
		return RunResultV1{}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "Auto Reviewer current Attempt state is not executable")
	}

	startGranted := false
	if current.State == contract.AutoReviewerAttemptPreparedV1 {
		current, startGranted, err = o.claimInvocationStartV1(ctx, command, current, actual)
		if err != nil {
			return RunResultV1{Attempt: current}, err
		}
		// Re-read all Review-owned and injected current inputs after the claim and
		// immediately before the external actual point. A drift leaves the durable
		// waiting_inspect fence in place and grants no invocation.
		current, actual, err = o.validateOrRefreshCurrentV1(ctx, command, current, actual)
		if err != nil {
			return RunResultV1{Attempt: current}, err
		}
		switch current.State {
		case contract.AutoReviewerAttemptObservedV1:
			if current.DomainResult == nil || current.DomainResult.ID != command.ResultID {
				return RunResultV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "concurrent Auto Reviewer closure changed the stable DomainResult identity")
			}
			recovery, cancel, recoveryErr := o.boundedRecoveryContextV1(ctx, actual, current.UpdatedUnixNano, current.ExpiresUnixNano)
			if recoveryErr != nil {
				return RunResultV1{Attempt: current}, recoveryErr
			}
			defer cancel()
			return o.inspectObservedV1(recovery, current)
		case contract.AutoReviewerAttemptFailedClosedV1, contract.AutoReviewerAttemptEscalatedV1:
			return RunResultV1{Attempt: current}, nil
		case contract.AutoReviewerAttemptWaitingInspectV1:
		default:
			return RunResultV1{Attempt: current}, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "Auto Reviewer start claim did not establish an inspect-only fence")
		}
	}

	origin, originErr := invocationAttemptRefV1(current)
	if originErr != nil {
		return RunResultV1{Attempt: current}, originErr
	}
	var invocationResult reviewport.AutoReviewerInvocationResultV1
	if startGranted {
		// The canonical external command remains the sealed prepared revision one.
		// The waiting_inspect successor is only the durable Review-side fence.
		invocationResult, err = o.invocation.StartOrInspectAutoReviewerInvocationV1(ctx, reviewport.AutoReviewerInvocationCommandV1{Attempt: command.Attempt})
	} else {
		invocationResult, err = o.invocation.InspectAutoReviewerInvocationV1(ctx, origin)
	}
	if err != nil {
		return o.handleInvocationErrorV1(ctx, command, current, startGranted, actual, err)
	}
	return o.recordObservationV1(ctx, command, current, invocationResult, actual)
}

func (o *Owner) validateCommandV1(command RunCommandV1, now time.Time) error {
	if strings.TrimSpace(command.ResultID) == "" || len(command.ResultID) > 512 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Auto Reviewer DomainResult identity is invalid")
	}
	if err := command.Attempt.ValidateCurrent(now); err != nil {
		return err
	}
	if command.Attempt.Revision != 1 || command.Attempt.State != contract.AutoReviewerAttemptPreparedV1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "Auto Reviewer command requires the canonical prepared revision one")
	}
	if o.contextReader != nil {
		if command.Attempt.ReviewerContext == nil {
			return core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "production Auto Reviewer Attempt lacks an exact Reviewer Context ref")
		}
		return nil
	}
	if err := command.Context.ValidateCurrent(now, command.Attempt.Target.Digest); err != nil {
		return err
	}
	if command.Context.TenantID != command.Attempt.TenantID || command.Context.CaseID != command.Attempt.Case.ID || command.Context.RoundID != command.Attempt.Round.ID || command.Context.Digest != command.Attempt.ContextFrameDigest || command.Context.RubricDigest != command.Attempt.Rubric.Digest || command.Context.OutputSchemaDigest != command.Attempt.ResultSchema.ContentDigest {
		return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Auto Reviewer Context does not bind the exact Attempt")
	}
	return nil
}

func (o *Owner) ensureAttemptV1(ctx context.Context, expected contract.AutoReviewerAttemptV1, previous time.Time) (contract.AutoReviewerAttemptV1, error) {
	current, err := o.store.InspectAutoReviewerAttemptByIdempotencyV1(ctx, expected.TenantID, expected.IdempotencyKey)
	if err == nil {
		return o.verifyExistingAttemptV1(ctx, expected, current)
	}
	if !core.HasCategory(err, core.ErrorNotFound) {
		// Unknown/Unavailable reads cannot grant a fresh Begin.
		return contract.AutoReviewerAttemptV1{}, err
	}

	created, beginErr := o.store.BeginAutoReviewerAttemptV1(ctx, reviewport.BeginAutoReviewerAttemptMutationV1{Attempt: expected})
	if beginErr == nil {
		return o.verifyExistingAttemptV1(ctx, expected, created)
	}
	if !unknownV1(beginErr) && !core.HasCategory(beginErr, core.ErrorConflict) {
		return contract.AutoReviewerAttemptV1{}, beginErr
	}

	// Begin may have committed before its reply was lost, or a same-canonical
	// caller may have won. Recovery is read-only and first proves the exact
	// prepared history; it never calls Begin again.
	recovery, cancel, recoveryErr := o.boundedRecoveryContextV1(ctx, previous, expected.UpdatedUnixNano, expected.ExpiresUnixNano)
	if recoveryErr != nil {
		return contract.AutoReviewerAttemptV1{}, beginErr
	}
	defer cancel()
	historical, inspectErr := o.store.InspectAutoReviewerAttemptExactV1(recovery, expected.TenantID, expected.ExactRef())
	if inspectErr != nil || historical.Digest != expected.Digest {
		return contract.AutoReviewerAttemptV1{}, beginErr
	}
	current, inspectErr = o.store.InspectAutoReviewerAttemptCurrentV1(recovery, expected.TenantID, expected.ID)
	if inspectErr != nil {
		return contract.AutoReviewerAttemptV1{}, beginErr
	}
	return o.verifyExistingAttemptV1(recovery, expected, current)
}

func (o *Owner) verifyExistingAttemptV1(ctx context.Context, expected, current contract.AutoReviewerAttemptV1) (contract.AutoReviewerAttemptV1, error) {
	if err := current.Validate(); err != nil {
		return contract.AutoReviewerAttemptV1{}, err
	}
	historical, err := o.store.InspectAutoReviewerAttemptExactV1(ctx, expected.TenantID, expected.ExactRef())
	if err != nil {
		return contract.AutoReviewerAttemptV1{}, err
	}
	if historical.Digest != expected.Digest || !sameSubjectV1(expected, current) {
		return contract.AutoReviewerAttemptV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "Auto Reviewer canonical Attempt subject drifted")
	}
	return current, nil
}

func (o *Owner) validateCurrentV1(ctx context.Context, command RunCommandV1, current contract.AutoReviewerAttemptV1, now time.Time) (contract.RubricDefinitionV1, error) {
	stored, err := o.store.InspectAutoReviewerAttemptCurrentV1(ctx, current.TenantID, current.ID)
	if err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	if stored.ExactRef() != current.ExactRef() || !sameSubjectV1(command.Attempt, stored) {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Auto Reviewer current Attempt drifted")
	}
	if err := stored.ValidateCurrent(now); err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	rubric, err := o.store.InspectRubricCurrentV1(ctx, stored.TenantID, stored.Rubric, now)
	if err != nil {
		return contract.RubricDefinitionV1{}, err
	}
	if rubric.ExactRef() != stored.Rubric || stored.RoundOrdinal > rubric.Termination.MaxRounds || stored.ExpiresUnixNano > rubric.ExpiresUnixNano {
		return contract.RubricDefinitionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBudgetBindingStale, "Auto Reviewer Attempt exceeds its exact Rubric, Context or round boundary")
	}
	allowedCapabilities := command.Context.AllowedReadCapabilities
	if o.contextReader != nil {
		if stored.ReviewerContext == nil {
			return contract.RubricDefinitionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "production Auto Reviewer Attempt lacks an exact Reviewer Context ref")
		}
		subject := stored.ReviewerContextSubjectV1()
		envelope, inspectErr := o.contextReader.InspectCurrentReviewerContextV1(ctx, subject, *stored.ReviewerContext)
		if inspectErr != nil {
			return contract.RubricDefinitionV1{}, inspectErr
		}
		if inspectErr = envelope.ValidateCurrent(*stored.ReviewerContext, subject, now); inspectErr != nil {
			return contract.RubricDefinitionV1{}, inspectErr
		}
		if envelope.ExpiresUnixNano < stored.ExpiresUnixNano {
			return contract.RubricDefinitionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBudgetBindingStale, "Auto Reviewer Attempt exceeds its exact Reviewer Context boundary")
		}
		allowedCapabilities = envelope.AllowedReadCapabilities
	} else {
		if stored.ExpiresUnixNano > command.Context.ExpiresUnixNano {
			return contract.RubricDefinitionV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBudgetBindingStale, "Auto Reviewer Attempt exceeds its legacy Context boundary")
		}
		if err := command.Context.ValidateCurrent(now, stored.Target.Digest); err != nil {
			return contract.RubricDefinitionV1{}, err
		}
	}
	allowed := make(map[string]struct{}, len(rubric.AllowedReadOnlyCapabilities))
	for _, capability := range rubric.AllowedReadOnlyCapabilities {
		allowed[string(capability)] = struct{}{}
	}
	for _, capability := range allowedCapabilities {
		if _, ok := allowed[capability]; !ok {
			return contract.RubricDefinitionV1{}, core.NewError(core.ErrorForbidden, core.ReasonUnknownCapability, "Auto Reviewer Context exceeds the exact Rubric read capability set")
		}
	}
	return rubric, nil
}

// validateOrRefreshCurrentV1 preserves create-once concurrency without turning
// a stale local snapshot into authority. It performs a small bounded sequence
// of read-only refreshes only when the Review-owned Attempt exact current ref
// actually advanced (prepared -> waiting -> observed/terminal). A conflict
// raised by an external current Reader while the Attempt ref is unchanged is
// returned unchanged; no mutation or external invocation is replayed.
func (o *Owner) validateOrRefreshCurrentV1(ctx context.Context, command RunCommandV1, current contract.AutoReviewerAttemptV1, previous time.Time) (contract.AutoReviewerAttemptV1, time.Time, error) {
	now := previous
	for refresh := 0; refresh < 4; refresh++ {
		var err error
		now, err = o.freshAfterV1(now, current.UpdatedUnixNano)
		if err != nil {
			return contract.AutoReviewerAttemptV1{}, time.Time{}, err
		}
		if _, err = o.validateCurrentV1(ctx, command, current, now); err == nil {
			return current, now, nil
		} else if !core.HasReason(err, core.ReasonRevisionConflict) {
			return contract.AutoReviewerAttemptV1{}, time.Time{}, err
		} else {
			latest, inspectErr := o.store.InspectAutoReviewerAttemptCurrentV1(ctx, current.TenantID, current.ID)
			if inspectErr != nil || latest.ExactRef() == current.ExactRef() {
				// An unchanged Review ref proves that the revision conflict came
				// from another Owner current source. Never hide it with a retry.
				return contract.AutoReviewerAttemptV1{}, time.Time{}, err
			}
			latest, inspectErr = o.verifyExistingAttemptV1(ctx, command.Attempt, latest)
			if inspectErr != nil {
				return contract.AutoReviewerAttemptV1{}, time.Time{}, err
			}
			current = latest
		}
	}
	return contract.AutoReviewerAttemptV1{}, time.Time{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Auto Reviewer current Attempt did not stabilize")
}

func (o *Owner) handleInvocationErrorV1(ctx context.Context, command RunCommandV1, current contract.AutoReviewerAttemptV1, startGranted bool, previous time.Time, invocationErr error) (RunResultV1, error) {
	if !startGranted {
		// A replay or a caller that did not win the persistent start claim may
		// only Inspect. Any Inspect error preserves waiting_inspect for a later
		// exact Inspect; it can never grant another external Start.
		return RunResultV1{Attempt: current}, invocationErr
	}
	if !unknownV1(invocationErr) {
		terminal, termErr := o.terminateV1(ctx, current, contract.AutoReviewerAttemptFailedClosedV1, "reviewer_invocation_failed_closed")
		if termErr != nil {
			return RunResultV1{Attempt: current}, invocationErr
		}
		return RunResultV1{Attempt: terminal}, invocationErr
	}

	// The start claim was persisted before the external boundary. A lost reply
	// therefore needs only an exact Inspect outside the canceled caller context;
	// no further Review mutation is required to retain the permanent fence.
	recovery, cancel, recoveryErr := o.boundedRecoveryContextV1(ctx, previous, current.UpdatedUnixNano, current.ExpiresUnixNano)
	if recoveryErr != nil {
		return RunResultV1{Attempt: current}, invocationErr
	}
	defer cancel()
	origin, originErr := invocationAttemptRefV1(current)
	if originErr != nil {
		return RunResultV1{Attempt: current}, invocationErr
	}
	invocationResult, inspectErr := o.invocation.InspectAutoReviewerInvocationV1(recovery, origin)
	if inspectErr != nil {
		return RunResultV1{Attempt: current}, invocationErr
	}
	observationRecovery, observationCancel, recoveryErr := o.boundedRecoveryContextV1(ctx, previous, current.UpdatedUnixNano, current.ExpiresUnixNano, invocationResult.ExpiresUnixNano)
	if recoveryErr != nil {
		return RunResultV1{Attempt: current}, invocationErr
	}
	defer observationCancel()
	now := o.clock()
	if now.IsZero() || now.Before(previous) {
		return RunResultV1{Attempt: current}, invocationErr
	}
	result, recordErr := o.recordObservationV1(observationRecovery, command, current, invocationResult, now)
	if recordErr != nil {
		return RunResultV1{Attempt: result.Attempt}, invocationErr
	}
	return result, nil
}

// claimInvocationStartV1 atomically establishes the permanent inspect-only
// fence before the external invocation boundary. Only the caller receiving
// Applied=true may call StartOrInspect. Unknown or conflicting mutation replies
// are recovered by exact/current Inspect and never grant caller execution.
func (o *Owner) claimInvocationStartV1(ctx context.Context, command RunCommandV1, current contract.AutoReviewerAttemptV1, previous time.Time) (contract.AutoReviewerAttemptV1, bool, error) {
	now, err := o.freshAfterV1(previous, current.UpdatedUnixNano)
	if err != nil {
		return current, false, err
	}
	next := current
	next.Revision++
	next.UpdatedUnixNano = now.UnixNano()
	next.State = contract.AutoReviewerAttemptWaitingInspectV1
	origin := current.ExactRef()
	next.InvocationAttempt = &origin
	next.TerminationReason = ""
	next.Digest = ""
	sealed, err := contract.SealAutoReviewerAttemptV1(next)
	if err != nil {
		return current, false, err
	}
	receipt, err := o.store.MarkAutoReviewerWaitingInspectV1(ctx, reviewport.MarkAutoReviewerWaitingInspectMutationV1{Expected: current.ExactRef(), Next: sealed})
	if err == nil {
		if receipt.Attempt.ExactRef() != sealed.ExactRef() || !sameSubjectV1(current, receipt.Attempt) {
			return current, false, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Auto Reviewer start-claim receipt drifted")
		}
		return receipt.Attempt, receipt.Applied, nil
	}
	if !unknownV1(err) && !core.HasCategory(err, core.ErrorConflict) {
		return current, false, err
	}

	recovery, cancel, recoveryErr := o.boundedRecoveryContextV1(ctx, now, sealed.UpdatedUnixNano, sealed.ExpiresUnixNano)
	if recoveryErr != nil {
		return current, false, err
	}
	defer cancel()
	historical, inspectErr := o.store.InspectAutoReviewerAttemptExactV1(recovery, sealed.TenantID, sealed.ExactRef())
	if inspectErr == nil && historical.Digest == sealed.Digest && sameSubjectV1(current, historical) {
		return historical, false, nil
	}
	latest, inspectErr := o.store.InspectAutoReviewerAttemptCurrentV1(recovery, current.TenantID, current.ID)
	if inspectErr != nil {
		return current, false, err
	}
	latest, inspectErr = o.verifyExistingAttemptV1(recovery, command.Attempt, latest)
	if inspectErr != nil {
		return current, false, err
	}
	switch latest.State {
	case contract.AutoReviewerAttemptWaitingInspectV1, contract.AutoReviewerAttemptObservedV1, contract.AutoReviewerAttemptFailedClosedV1, contract.AutoReviewerAttemptEscalatedV1:
		return latest, false, nil
	default:
		// A pre-commit Unknown leaves the canonical prepared Attempt untouched.
		// No external call occurred, and a later Run may retry only this claim.
		return current, false, err
	}
}

func (o *Owner) recordObservationV1(ctx context.Context, command RunCommandV1, current contract.AutoReviewerAttemptV1, invocationResult reviewport.AutoReviewerInvocationResultV1, previous time.Time) (RunResultV1, error) {
	invocationResult = invocationResult.Clone()
	now, err := o.freshAfterV1(previous, current.UpdatedUnixNano)
	if err != nil {
		return RunResultV1{Attempt: current}, err
	}
	rubric, err := o.validateCurrentV1(ctx, command, current, now)
	if err != nil {
		if core.HasReason(err, core.ReasonRevisionConflict) {
			recovery, cancel, recoveryErr := o.boundedRecoveryContextV1(ctx, now, current.UpdatedUnixNano, current.ExpiresUnixNano, invocationResult.ExpiresUnixNano)
			if recoveryErr != nil {
				return RunResultV1{Attempt: current}, err
			}
			defer cancel()
			return o.recoverConcurrentObservedV1(recovery, command, current, invocationResult, now, err)
		}
		return RunResultV1{Attempt: current}, err
	}
	observation, err := o.sealInvocationObservationV1(ctx, current, rubric, invocationResult, now)
	if err != nil {
		return RunResultV1{Attempt: current}, err
	}
	origin, err := invocationAttemptRefV1(current)
	if err != nil {
		return RunResultV1{Attempt: current}, err
	}
	if observation.Tokens > rubric.Termination.MaxTokens || observation.CostMicros > current.MaxCostMicros || observation.ObservedUnixNano-current.CreatedUnixNano > rubric.Termination.MaxDurationNanos {
		terminal, termErr := o.terminateV1(ctx, current, contract.AutoReviewerAttemptEscalatedV1, "review_budget_or_loop_bound_reached")
		if termErr != nil {
			return RunResultV1{Attempt: current}, termErr
		}
		return RunResultV1{Attempt: terminal}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBudgetBindingStale, "Auto Reviewer observation exceeded the exact budget")
	}

	// The DomainResult uses immutable Provider observation time. The Attempt
	// successor uses the fresh Owner clock after S2.
	transitionTime := now.UnixNano()
	if transitionTime <= current.UpdatedUnixNano {
		return RunResultV1{Attempt: current}, clockError("Auto Reviewer result transition clock did not advance")
	}
	result, err := contract.SealReviewerInvocationResultFactV1(contract.ReviewerInvocationResultFactV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: current.TenantID, ID: command.ResultID, Revision: 1, CreatedUnixNano: observation.ObservedUnixNano, UpdatedUnixNano: observation.ObservedUnixNano},
		CaseID:         current.Case.ID, CaseRevision: current.Case.Revision,
		RoundID: current.Round.ID, RoundRevision: current.Round.Revision, RoundDigest: current.Round.Digest,
		AssignmentID: current.Assignment.ID, AssignmentRevision: current.Assignment.Revision, AssignmentDigest: current.Assignment.Digest,
		TargetID: current.Target.ID, TargetRevision: current.Target.Revision, TargetDigest: current.Target.Digest,
		AttemptID: observation.RuntimeAttempt.AttemptID, ResultSchema: current.ResultSchema,
		ResultDigest: observation.Output.Digest, ObservationRefs: []string{observation.ID},
	})
	if err != nil {
		return RunResultV1{Attempt: current}, err
	}
	next := current
	next.Revision++
	next.UpdatedUnixNano = transitionTime
	next.State = contract.AutoReviewerAttemptObservedV1
	if next.InvocationAttempt == nil {
		originCopy := origin
		next.InvocationAttempt = &originCopy
	}
	observationRef, resultRef := observation.Ref(), result.ExactRef()
	next.Observation, next.DomainResult = &observationRef, &resultRef
	next.Digest = ""
	next, err = contract.SealAutoReviewerAttemptV1(next)
	if err != nil {
		return RunResultV1{Attempt: current}, err
	}

	stored, storedResult, err := o.store.RecordAutoReviewerObservationV1(ctx, reviewport.RecordAutoReviewerObservationMutationV1{Expected: current.ExactRef(), Next: next, Observation: observation, DomainResult: result})
	if err == nil {
		return resultV1(stored, observation, storedResult), nil
	}
	if !unknownV1(err) && !core.HasCategory(err, core.ErrorConflict) {
		return RunResultV1{Attempt: current}, err
	}
	recovery, cancel, recoveryErr := o.boundedRecoveryContextV1(ctx, now, next.UpdatedUnixNano, next.ExpiresUnixNano, observation.ExpiresUnixNano)
	if recoveryErr != nil {
		return RunResultV1{Attempt: current}, err
	}
	defer cancel()
	return o.inspectRecordedV1(recovery, next, observation, result, err)
}

func (o *Owner) sealInvocationObservationV1(ctx context.Context, current contract.AutoReviewerAttemptV1, rubric contract.RubricDefinitionV1, invocationResult reviewport.AutoReviewerInvocationResultV1, now time.Time) (contract.AutoReviewerInvocationObservationV1, error) {
	origin, err := invocationAttemptRefV1(current)
	if err != nil {
		return contract.AutoReviewerInvocationObservationV1{}, err
	}
	if invocationResult.Attempt != origin || invocationResult.OperationDigest != current.OperationDigest || invocationResult.ResultSchema != current.ResultSchema || len(invocationResult.RawOutput) == 0 || len(invocationResult.RawOutput) > core.MaxCanonicalDocumentBytes || invocationResult.Tokens == 0 || invocationResult.CostMicros == 0 || invocationResult.ObservedUnixNano <= 0 || invocationResult.ExpiresUnixNano <= invocationResult.ObservedUnixNano || invocationResult.RuntimeAttempt.Validate() != nil || invocationResult.ProviderObservation.Validate() != nil {
		return contract.AutoReviewerInvocationObservationV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Auto Reviewer Observation does not bind the exact current Attempt")
	}
	if now.IsZero() || now.UnixNano() < invocationResult.ObservedUnixNano {
		return contract.AutoReviewerInvocationObservationV1{}, clockError("Auto Reviewer Observation is ahead of the Owner clock")
	}
	if err := contract.ValidateNow(now, invocationResult.ObservedUnixNano, invocationResult.ExpiresUnixNano); err != nil {
		return contract.AutoReviewerInvocationObservationV1{}, err
	}
	schema, err := o.schemaReader.InspectAutoReviewerOutputSchemaV1(ctx, current.ResultSchema)
	if err != nil {
		return contract.AutoReviewerInvocationObservationV1{}, err
	}
	if err = schema.ValidateForRubricV1(rubric, current.ResultSchema); err != nil {
		return contract.AutoReviewerInvocationObservationV1{}, err
	}
	output, err := schema.ValidateDraftV1(invocationResult.RawOutput)
	if err != nil {
		return contract.AutoReviewerInvocationObservationV1{}, err
	}
	if err = rubric.ValidateAutoReviewerOutputV1(output); err != nil {
		return contract.AutoReviewerInvocationObservationV1{}, err
	}
	return contract.SealAutoReviewerInvocationObservationV1(contract.AutoReviewerInvocationObservationV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: current.TenantID, ID: invocationResult.ObservationID, Revision: 1, CreatedUnixNano: invocationResult.ObservedUnixNano, UpdatedUnixNano: invocationResult.ObservedUnixNano},
		AttemptID:      origin.ID, AttemptRevision: origin.Revision, AttemptDigest: origin.Digest,
		OperationDigest: current.OperationDigest, RuntimeAttempt: invocationResult.RuntimeAttempt,
		ProviderObservation: invocationResult.ProviderObservation, Output: output, ResultSchema: current.ResultSchema,
		Tokens: invocationResult.Tokens, CostMicros: invocationResult.CostMicros,
		ObservedUnixNano: invocationResult.ObservedUnixNano, ExpiresUnixNano: invocationResult.ExpiresUnixNano,
	})
}

func (o *Owner) recoverConcurrentObservedV1(ctx context.Context, command RunCommandV1, previous contract.AutoReviewerAttemptV1, invocationResult reviewport.AutoReviewerInvocationResultV1, now time.Time, original error) (RunResultV1, error) {
	latest, err := o.store.InspectAutoReviewerAttemptCurrentV1(ctx, previous.TenantID, previous.ID)
	if err != nil || latest.State != contract.AutoReviewerAttemptObservedV1 || latest.Observation == nil || latest.DomainResult == nil || latest.DomainResult.ID != command.ResultID || !sameSubjectV1(command.Attempt, latest) {
		return RunResultV1{Attempt: previous}, original
	}
	rubric, err := o.validateCurrentV1(ctx, command, latest, now)
	if err != nil {
		return RunResultV1{Attempt: previous}, original
	}
	expected, err := o.sealInvocationObservationV1(ctx, latest, rubric, invocationResult, now)
	if err != nil || *latest.Observation != expected.Ref() {
		return RunResultV1{Attempt: previous}, original
	}
	return o.inspectObservedV1(ctx, latest)
}

func (o *Owner) inspectRecordedV1(ctx context.Context, next contract.AutoReviewerAttemptV1, observation contract.AutoReviewerInvocationObservationV1, result contract.ReviewerInvocationResultFactV1, original error) (RunResultV1, error) {
	stored, err := o.store.InspectAutoReviewerAttemptExactV1(ctx, next.TenantID, next.ExactRef())
	if err != nil || stored.Digest != next.Digest {
		// A concurrent same-canonical caller may have published the identical
		// Observation and DomainResult with another monotonic Attempt timestamp.
		// Recover that already-stored closure read-only; never replay Record.
		stored, err = o.store.InspectAutoReviewerAttemptCurrentV1(ctx, next.TenantID, next.ID)
		if err != nil || stored.State != contract.AutoReviewerAttemptObservedV1 || !sameSubjectV1(next, stored) || stored.InvocationAttempt == nil || next.InvocationAttempt == nil || *stored.InvocationAttempt != *next.InvocationAttempt || stored.Observation == nil || *stored.Observation != observation.Ref() || stored.DomainResult == nil || stored.DomainResult.ID != result.ID || stored.DomainResult.Revision != result.Revision || stored.DomainResult.Digest != result.Digest {
			return RunResultV1{}, original
		}
	}
	storedObservation, err := o.store.InspectAutoReviewerObservationExactV1(ctx, next.TenantID, observation.Ref())
	if err != nil || storedObservation.Digest != observation.Digest {
		return RunResultV1{}, original
	}
	storedResult, err := o.store.InspectDomainResultExactV1(ctx, next.TenantID, reviewport.ExactV1(result.ID, result.Revision, result.Digest))
	if err != nil || storedResult.Digest != result.Digest {
		return RunResultV1{}, original
	}
	return resultV1(stored, storedObservation, storedResult), nil
}

func (o *Owner) inspectObservedV1(ctx context.Context, current contract.AutoReviewerAttemptV1) (RunResultV1, error) {
	if current.Observation == nil || current.DomainResult == nil {
		return RunResultV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Auto Reviewer observed Attempt closure is incomplete")
	}
	observation, err := o.store.InspectAutoReviewerObservationExactV1(ctx, current.TenantID, *current.Observation)
	if err != nil {
		return RunResultV1{}, err
	}
	result, err := o.store.InspectDomainResultExactV1(ctx, current.TenantID, reviewport.ExactV1(current.DomainResult.ID, current.DomainResult.Revision, current.DomainResult.Digest))
	if err != nil {
		return RunResultV1{}, err
	}
	return resultV1(current, observation, result), nil
}

func (o *Owner) moveV1(ctx context.Context, current contract.AutoReviewerAttemptV1, state contract.AutoReviewerAttemptStateV1, reason string) (contract.AutoReviewerAttemptV1, error) {
	now := o.clock()
	if now.IsZero() || now.UnixNano() <= current.UpdatedUnixNano {
		return contract.AutoReviewerAttemptV1{}, clockError("Auto Reviewer transition clock did not advance")
	}
	next := current
	next.Revision++
	next.UpdatedUnixNano = now.UnixNano()
	next.State = state
	if next.InvocationAttempt == nil {
		origin := current.ExactRef()
		next.InvocationAttempt = &origin
	}
	next.TerminationReason = reason
	next.Digest = ""
	sealed, err := contract.SealAutoReviewerAttemptV1(next)
	if err != nil {
		return contract.AutoReviewerAttemptV1{}, err
	}
	written, err := o.store.TerminateAutoReviewerAttemptV1(ctx, reviewport.TerminateAutoReviewerAttemptMutationV1{Expected: current.ExactRef(), Next: sealed})
	if err == nil {
		return written, nil
	}
	if !unknownV1(err) && !core.HasCategory(err, core.ErrorConflict) {
		return contract.AutoReviewerAttemptV1{}, err
	}
	recovery, cancel, recoveryErr := o.boundedRecoveryContextV1(ctx, now, sealed.UpdatedUnixNano, sealed.ExpiresUnixNano)
	if recoveryErr != nil {
		return contract.AutoReviewerAttemptV1{}, err
	}
	defer cancel()
	recovered, inspectErr := o.store.InspectAutoReviewerAttemptExactV1(recovery, sealed.TenantID, sealed.ExactRef())
	if inspectErr == nil && recovered.Digest == sealed.Digest {
		return recovered, nil
	}
	// A same-subject concurrent caller may have published the only valid
	// successor with another monotonic timestamp. Accept that current state,
	// but never perform another mutation.
	recovered, inspectErr = o.store.InspectAutoReviewerAttemptCurrentV1(recovery, current.TenantID, current.ID)
	if inspectErr == nil && recovered.State == state && sameSubjectV1(current, recovered) {
		return recovered, nil
	}
	return contract.AutoReviewerAttemptV1{}, err
}

func (o *Owner) terminateV1(ctx context.Context, current contract.AutoReviewerAttemptV1, state contract.AutoReviewerAttemptStateV1, reason string) (contract.AutoReviewerAttemptV1, error) {
	return o.moveV1(ctx, current, state, reason)
}

func (o *Owner) freshAfterV1(previous time.Time, updatedUnixNano int64) (time.Time, error) {
	now := o.clock()
	if now.IsZero() || now.Before(previous) || now.UnixNano() < updatedUnixNano {
		return time.Time{}, clockError("Auto Reviewer clock regressed")
	}
	return now, nil
}

// boundedRecoveryContextV1 detaches only cancellation, never time or scope.
// Every lost-reply/concurrent recovery is capped at five seconds and shortened
// by the earliest exact Attempt/Observation expiry. It also rejects Owner clock
// rollback before any recovery Inspect is attempted.
func (o *Owner) boundedRecoveryContextV1(parent context.Context, previous time.Time, updatedUnixNano int64, expiries ...int64) (context.Context, context.CancelFunc, error) {
	now, err := o.freshAfterV1(previous, updatedUnixNano)
	if err != nil {
		return nil, nil, err
	}
	limit := o.recoveryTimeout
	if limit <= 0 || limit > defaultRecoveryTimeoutV1 {
		limit = defaultRecoveryTimeoutV1
	}
	for _, expiresUnixNano := range expiries {
		if expiresUnixNano <= 0 {
			continue
		}
		if now.UnixNano() >= expiresUnixNano {
			return nil, nil, core.NewError(core.ErrorPreconditionFailed, core.ReasonReviewVerdictStale, "Auto Reviewer recovery subject expired")
		}
		remaining := time.Duration(expiresUnixNano - now.UnixNano())
		if remaining < limit {
			limit = remaining
		}
	}
	recovery, cancel := context.WithTimeout(context.WithoutCancel(parent), limit)
	return recovery, cancel, nil
}

func resultV1(attempt contract.AutoReviewerAttemptV1, observation contract.AutoReviewerInvocationObservationV1, result contract.ReviewerInvocationResultFactV1) RunResultV1 {
	return RunResultV1{Attempt: attempt, Observation: &observation, DomainResult: &result}
}

func sameSubjectV1(left, right contract.AutoReviewerAttemptV1) bool {
	return left.TenantID == right.TenantID && left.ID == right.ID && left.IdempotencyKey == right.IdempotencyKey && left.Case == right.Case && left.Round == right.Round && left.Assignment == right.Assignment && left.Target == right.Target && left.Rubric == right.Rubric && left.ContextFrameDigest == right.ContextFrameDigest && sameReviewerContextRefV1(left.ReviewerContext, right.ReviewerContext) && left.ReviewerID == right.ReviewerID && left.ReviewerAuthority == right.ReviewerAuthority && left.ReviewerBinding == right.ReviewerBinding && left.RouteID == right.RouteID && left.Operation == right.Operation && left.OperationDigest == right.OperationDigest && left.InvocationEffect == right.InvocationEffect && left.ResultSchema == right.ResultSchema && left.RoundOrdinal == right.RoundOrdinal && left.MaxCostMicros == right.MaxCostMicros && left.CreatedUnixNano == right.CreatedUnixNano && left.ExpiresUnixNano == right.ExpiresUnixNano
}

func sameReviewerContextRefV1(left, right *contract.ReviewerContextEnvelopeRefV1) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func invocationAttemptRefV1(attempt contract.AutoReviewerAttemptV1) (contract.ExactResourceRefV1, error) {
	if attempt.InvocationAttempt != nil {
		if err := attempt.InvocationAttempt.Validate(); err != nil {
			return contract.ExactResourceRefV1{}, err
		}
		if attempt.InvocationAttempt.ID != attempt.ID {
			return contract.ExactResourceRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Auto Reviewer original invocation Attempt identity drifted")
		}
		return *attempt.InvocationAttempt, nil
	}
	if attempt.State != contract.AutoReviewerAttemptPreparedV1 {
		return contract.ExactResourceRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Auto Reviewer current state lacks the original invocation Attempt")
	}
	return attempt.ExactRef(), nil
}

func unknownV1(err error) bool {
	return core.HasCategory(err, core.ErrorIndeterminate) || core.HasCategory(err, core.ErrorUnavailable) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func clockError(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, message)
}
