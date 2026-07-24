// Package reviewadapter exposes Context Owner implementations of Review's
// public Reviewer Context ports. It contains no production composition root.
package reviewadapter

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/reviewcontextstore"
	reviewcontract "github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ClockV1 func() time.Time

type ReviewerContextAdapterV1 struct {
	repository reviewcontextstore.RepositoryV1
	clock      ClockV1
}

func NewReviewerContextAdapterV1(repository reviewcontextstore.RepositoryV1, clock ClockV1) (*ReviewerContextAdapterV1, error) {
	if nilLikeV1(repository) || clock == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Reviewer Context adapter dependencies are unavailable")
	}
	return &ReviewerContextAdapterV1{repository: repository, clock: clock}, nil
}

var (
	_ reviewport.ReviewerContextPublisherV1     = (*ReviewerContextAdapterV1)(nil)
	_ reviewport.ReviewerContextCurrentReaderV1 = (*ReviewerContextAdapterV1)(nil)
)

func (a *ReviewerContextAdapterV1) PublishReviewerContextV1(ctx context.Context, request reviewport.ReviewerContextPublishRequestV1) (reviewport.ReviewerContextPublishReceiptV1, error) {
	if err := a.readyV1(); err != nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, err
	}
	if ctx == nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, invalidV1("Reviewer Context publish context is required")
	}
	if err := request.Validate(); err != nil {
		return reviewport.ReviewerContextPublishReceiptV1{}, closedV1(err)
	}
	receipt, err := a.repository.CommitV1(ctx, request)
	if err == nil {
		if receipt.Ref != request.Value.Ref {
			return reviewport.ReviewerContextPublishReceiptV1{}, conflictV1("Reviewer Context publish receipt drifted")
		}
		return receipt, nil
	}
	original := closedV1(err)
	if !core.HasCategory(original, core.ErrorIndeterminate) {
		return reviewport.ReviewerContextPublishReceiptV1{}, original
	}

	// Unknown publish outcomes never call Commit again. Only the exact immutable
	// historical object can prove that this canonical value linearized.
	inspected, inspectErr := a.repository.InspectHistoricalV1(context.WithoutCancel(ctx), request.Value.Ref)
	if inspectErr != nil {
		if core.HasCategory(closedV1(inspectErr), core.ErrorConflict) {
			return reviewport.ReviewerContextPublishReceiptV1{}, closedV1(inspectErr)
		}
		return reviewport.ReviewerContextPublishReceiptV1{}, original
	}
	if !reflect.DeepEqual(inspected, request.Value) {
		return reviewport.ReviewerContextPublishReceiptV1{}, conflictV1("Reviewer Context lost-reply historical value drifted")
	}
	return reviewport.ReviewerContextPublishReceiptV1{Ref: inspected.Ref, Created: true}, nil
}

func (a *ReviewerContextAdapterV1) ResolveCurrentReviewerContextV1(ctx context.Context, request reviewport.ReviewerContextCurrentResolveRequestV1) (reviewcontract.ReviewerContextEnvelopeRefV1, error) {
	if err := a.readyV1(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, err
	}
	if ctx == nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, invalidV1("Reviewer Context resolve context is required")
	}
	if err := request.Validate(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, closedV1(err)
	}
	baseline, err := a.baselineV1()
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, err
	}
	ref, err := a.repository.ResolveV1(ctx, request.Subject)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, closedV1(err)
	}
	value, err := a.repository.InspectCurrentV1(ctx, request.Subject, ref)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, closedV1(err)
	}
	if err := a.validateFreshCurrentV1(value, ref, request.Subject, baseline); err != nil {
		return reviewcontract.ReviewerContextEnvelopeRefV1{}, err
	}
	return ref, nil
}

func (a *ReviewerContextAdapterV1) InspectCurrentReviewerContextV1(ctx context.Context, subject reviewcontract.ReviewerContextSubjectV1, expected reviewcontract.ReviewerContextEnvelopeRefV1) (reviewcontract.ReviewerContextEnvelopeV1, error) {
	if err := a.readyV1(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	if ctx == nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, invalidV1("Reviewer Context current inspect context is required")
	}
	if err := subject.Validate(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, closedV1(err)
	}
	if err := expected.Validate(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, closedV1(err)
	}
	baseline, err := a.baselineV1()
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	value, err := a.repository.InspectCurrentV1(ctx, subject, expected)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, closedV1(err)
	}
	if err := a.validateFreshCurrentV1(value, expected, subject, baseline); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	return value.Clone(), nil
}

func (a *ReviewerContextAdapterV1) InspectHistoricalReviewerContextV1(ctx context.Context, exact reviewcontract.ReviewerContextEnvelopeRefV1) (reviewcontract.ReviewerContextEnvelopeV1, error) {
	if err := a.readyV1(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, err
	}
	if ctx == nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, invalidV1("Reviewer Context historical inspect context is required")
	}
	if err := exact.Validate(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, closedV1(err)
	}
	value, err := a.repository.InspectHistoricalV1(ctx, exact)
	if err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, closedV1(err)
	}
	if err := value.Validate(); err != nil {
		return reviewcontract.ReviewerContextEnvelopeV1{}, closedV1(err)
	}
	if value.Ref != exact {
		return reviewcontract.ReviewerContextEnvelopeV1{}, conflictV1("Reviewer Context historical exact ref drifted")
	}
	return value.Clone(), nil
}

func (a *ReviewerContextAdapterV1) readyV1() error {
	if a == nil || nilLikeV1(a.repository) || a.clock == nil {
		return invalidV1("Reviewer Context adapter is unavailable")
	}
	return nil
}

func (a *ReviewerContextAdapterV1) baselineV1() (time.Time, error) {
	now := a.clock()
	if now.IsZero() {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Reviewer Context clock returned zero")
	}
	return now, nil
}

func (a *ReviewerContextAdapterV1) validateFreshCurrentV1(value reviewcontract.ReviewerContextEnvelopeV1, expected reviewcontract.ReviewerContextEnvelopeRefV1, subject reviewcontract.ReviewerContextSubjectV1, baseline time.Time) error {
	fresh := a.clock()
	if fresh.IsZero() || fresh.Before(baseline) {
		return core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Reviewer Context clock regressed during current inspection")
	}
	return value.ValidateCurrent(expected, subject, fresh)
}

func closedV1(err error) error {
	if err == nil {
		return nil
	}
	for _, category := range []core.ErrorCategory{
		core.ErrorInvalidArgument, core.ErrorNotFound, core.ErrorConflict,
		core.ErrorPreconditionFailed, core.ErrorIndeterminate, core.ErrorUnavailable,
	} {
		if core.HasCategory(err, category) {
			return err
		}
	}
	if core.HasCategory(err, core.ErrorForbidden) {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonUnknownCapability, "Reviewer Context request contains a forbidden capability")
	}
	if core.HasCategory(err, core.ErrorCapabilityUnavailable) {
		return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "Reviewer Context dependency is unavailable")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "Reviewer Context operation outcome is unknown after cancellation")
	}
	return core.NewError(core.ErrorIndeterminate, core.ReasonInspectCoverageIncomplete, "Reviewer Context repository returned an unclassified outcome")
}

func nilLikeV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func invalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, message)
}

func conflictV1(message string) error {
	return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, message)
}
