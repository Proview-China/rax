package control

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ReviewBindingCurrentRepositoryV1 is the Binding Owner's atomic repository
// seam. A production implementation must provide one snapshot/transaction for
// the Binding closure and projection indexes; the reference implementation in
// runtime/fakes is conformance-only.
type ReviewBindingCurrentRepositoryV1 interface {
	ports.ReviewBindingAuthoritativeCurrentReaderV1
	ports.ReviewBindingConsumerAssociationCurrentReaderV1
	ports.ReviewBindingProjectionPublisherV1
	ReviewBindingAssociationProjectionPublisherV1
}

// CompareAndSwapReviewBindingAssociationProjectionRequestV1 is an Owner-local
// compound mutation. It is deliberately outside runtime/ports: consumers see
// only the public projection Publisher, while the Binding Owner can advance an
// association and every affected projection sidecar in one transaction.
type CompareAndSwapReviewBindingAssociationProjectionRequestV1 struct {
	ExpectedAssociation ports.ReviewBindingConsumerAssociationRefV1               `json:"expected_association"`
	NextAssociation     ports.ReviewBindingConsumerAssociationCurrentProjectionV1 `json:"next_association"`
	Projection          ports.CompareAndSwapReviewBindingProjectionRequestV1      `json:"projection"`
}

func (r CompareAndSwapReviewBindingAssociationProjectionRequestV1) Validate() error {
	if r.ExpectedAssociation.Validate() != nil || r.NextAssociation.Validate() != nil || r.Projection.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review Binding association/projection CAS request is incomplete")
	}
	if r.NextAssociation.Ref.ID != r.ExpectedAssociation.ID || r.NextAssociation.Ref.Revision != r.ExpectedAssociation.Revision+1 || r.NextAssociation.Source != r.Projection.Input.Source || r.Projection.Input.Association != r.NextAssociation.Ref || !r.NextAssociation.Current {
		return core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding association/projection CAS coordinates drifted")
	}
	return nil
}

// ReviewBindingAssociationProjectionPublisherV1 is held only inside the
// Runtime Binding Owner. It is not exposed to Review or host consumers.
type ReviewBindingAssociationProjectionPublisherV1 interface {
	CompareAndSwapReviewBindingAssociationProjectionV1(context.Context, CompareAndSwapReviewBindingAssociationProjectionRequestV1) (ports.ReviewBindingProjectionPublishReceiptV1, error)
}

// ReviewBindingCurrentOwnerV1 validates the public read/publish boundary. It
// owns no Provider, Review verdict, dispatch authority or production root.
type ReviewBindingCurrentOwnerV1 struct {
	repository ReviewBindingCurrentRepositoryV1
	clock      func() time.Time
}

func NewReviewBindingCurrentOwnerV1(repository ReviewBindingCurrentRepositoryV1, clock func() time.Time) (*ReviewBindingCurrentOwnerV1, error) {
	if isNilReviewBindingDependencyV1(repository) || clock == nil {
		return nil, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "Review Binding repository and clock are required")
	}
	return &ReviewBindingCurrentOwnerV1{repository: repository, clock: clock}, nil
}

func (o *ReviewBindingCurrentOwnerV1) ResolveCurrentReviewBindingV1(ctx context.Context, request ports.ResolveReviewBindingCurrentRequestV1) (ports.ReviewBindingProjectionRefV1, error) {
	if o == nil || isNilReviewBindingDependencyV1(o.repository) || o.clock == nil {
		return ports.ReviewBindingProjectionRefV1{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "Review Binding Owner is unavailable")
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingProjectionRefV1{}, err
	}
	baseline, err := o.baselineV1()
	if err != nil {
		return ports.ReviewBindingProjectionRefV1{}, err
	}
	ref, err := o.repository.ResolveCurrentReviewBindingV1(ctx, request)
	if err != nil {
		return ports.ReviewBindingProjectionRefV1{}, err
	}
	if err := ref.Validate(); err != nil {
		return ports.ReviewBindingProjectionRefV1{}, core.NewError(core.ErrorInternal, core.ReasonInvalidCanonicalForm, "Review Binding repository returned an invalid current Ref")
	}
	expectedID, deriveErr := ports.DeriveReviewBindingProjectionIDV1(ports.ReviewBindingProjectionIdentityInputV1{Source: request.Source, Subject: request.Subject})
	if deriveErr != nil || ref.ID != expectedID {
		return ports.ReviewBindingProjectionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding repository returned another current identity")
	}
	projection, inspectErr := o.repository.InspectCurrentReviewBindingV1(ctx, ports.InspectCurrentReviewBindingRequestV1{ExpectedRef: ref, ExpectedSource: request.Source, ExpectedSubject: request.Subject})
	if inspectErr != nil {
		return ports.ReviewBindingProjectionRefV1{}, inspectErr
	}
	now, err := o.freshV1(baseline)
	if err != nil {
		return ports.ReviewBindingProjectionRefV1{}, err
	}
	if validateErr := projection.ValidateCurrent(ref, request.Source, request.Subject, now); validateErr != nil {
		return ports.ReviewBindingProjectionRefV1{}, validateErr
	}
	return ref, nil
}

func (o *ReviewBindingCurrentOwnerV1) InspectReviewBindingProjectionV1(ctx context.Context, request ports.InspectReviewBindingProjectionRequestV1) (ports.ReviewBindingCurrentProjectionV1, error) {
	if o == nil || isNilReviewBindingDependencyV1(o.repository) || o.clock == nil {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "Review Binding Owner is unavailable")
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	baseline, err := o.baselineV1()
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	projection, err := o.repository.InspectReviewBindingProjectionV1(ctx, request)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	if _, err := o.freshV1(baseline); err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	if err := projection.Validate(); err != nil || projection.Ref != request.Ref || projection.Source != request.ExpectedSource || projection.Subject != request.ExpectedSubject {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding historical projection drifted")
	}
	return projection.CloneV1(), nil
}

func (o *ReviewBindingCurrentOwnerV1) InspectCurrentReviewBindingV1(ctx context.Context, request ports.InspectCurrentReviewBindingRequestV1) (ports.ReviewBindingCurrentProjectionV1, error) {
	if o == nil || isNilReviewBindingDependencyV1(o.repository) || o.clock == nil {
		return ports.ReviewBindingCurrentProjectionV1{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "Review Binding Owner is unavailable")
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	baseline, err := o.baselineV1()
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	projection, err := o.repository.InspectCurrentReviewBindingV1(ctx, request)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	now, err := o.freshV1(baseline)
	if err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	if err := projection.ValidateCurrent(request.ExpectedRef, request.ExpectedSource, request.ExpectedSubject, now); err != nil {
		return ports.ReviewBindingCurrentProjectionV1{}, err
	}
	return projection.CloneV1(), nil
}

func (o *ReviewBindingCurrentOwnerV1) InspectCurrentReviewBindingConsumerAssociationV1(ctx context.Context, expected ports.ReviewBindingConsumerAssociationRefV1) (ports.ReviewBindingConsumerAssociationCurrentProjectionV1, error) {
	if o == nil || isNilReviewBindingDependencyV1(o.repository) || o.clock == nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "Review Binding Owner is unavailable")
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	baseline, err := o.baselineV1()
	if err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	projection, err := o.repository.InspectCurrentReviewBindingConsumerAssociationV1(ctx, expected)
	if err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	now, err := o.freshV1(baseline)
	if err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	if err := projection.ValidateCurrent(expected, projection.Consumer, projection.Source, now); err != nil {
		return ports.ReviewBindingConsumerAssociationCurrentProjectionV1{}, err
	}
	return projection, nil
}

func (o *ReviewBindingCurrentOwnerV1) CreateReviewBindingProjectionV1(ctx context.Context, request ports.CreateReviewBindingProjectionRequestV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	if o == nil || isNilReviewBindingDependencyV1(o.repository) {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "Review Binding Owner is unavailable")
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	// Unknown outcomes are intentionally returned unchanged. The caller may
	// only Inspect the exact pre-derived PublishRef; this method never retries.
	receipt, err := o.repository.CreateReviewBindingProjectionV1(ctx, request)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	return validateReviewBindingPublishReceiptV1(receipt, request.PublishRef)
}

func (o *ReviewBindingCurrentOwnerV1) CompareAndSwapReviewBindingProjectionV1(ctx context.Context, request ports.CompareAndSwapReviewBindingProjectionRequestV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	if o == nil || isNilReviewBindingDependencyV1(o.repository) {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "Review Binding Owner is unavailable")
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	receipt, err := o.repository.CompareAndSwapReviewBindingProjectionV1(ctx, request)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	return validateReviewBindingPublishReceiptV1(receipt, request.PublishRef)
}

func (o *ReviewBindingCurrentOwnerV1) CompareAndSwapReviewBindingAssociationProjectionV1(ctx context.Context, request CompareAndSwapReviewBindingAssociationProjectionRequestV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	if o == nil || isNilReviewBindingDependencyV1(o.repository) {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "Review Binding Owner is unavailable")
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	receipt, err := o.repository.CompareAndSwapReviewBindingAssociationProjectionV1(ctx, request)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	return validateReviewBindingPublishReceiptV1(receipt, request.Projection.PublishRef)
}

func (o *ReviewBindingCurrentOwnerV1) InspectReviewBindingProjectionPublishV1(ctx context.Context, expected ports.ReviewBindingProjectionPublishRefV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	if o == nil || isNilReviewBindingDependencyV1(o.repository) || o.clock == nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorCapabilityUnavailable, core.ReasonComponentMissing, "Review Binding Owner is unavailable")
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	baseline, err := o.baselineV1()
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	receipt, err := o.repository.InspectReviewBindingProjectionPublishV1(ctx, expected)
	if err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	if _, err := o.freshV1(baseline); err != nil {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, err
	}
	return validateReviewBindingPublishReceiptV1(receipt, expected)
}

func (o *ReviewBindingCurrentOwnerV1) baselineV1() (time.Time, error) {
	now := o.clock()
	if now.IsZero() {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Binding clock is unavailable")
	}
	return now, nil
}

func (o *ReviewBindingCurrentOwnerV1) freshV1(baseline time.Time) (time.Time, error) {
	now := o.clock()
	if now.IsZero() || now.Before(baseline) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review Binding clock regressed during Owner inspection")
	}
	return now, nil
}

func validateReviewBindingPublishReceiptV1(receipt ports.ReviewBindingProjectionPublishReceiptV1, expected ports.ReviewBindingProjectionPublishRefV1) (ports.ReviewBindingProjectionPublishReceiptV1, error) {
	if err := receipt.Validate(); err != nil || receipt.PublishRef != expected {
		return ports.ReviewBindingProjectionPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review Binding Publish receipt drifted")
	}
	return receipt, nil
}

func isNilReviewBindingDependencyV1(value any) bool {
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

var _ ports.ReviewBindingAuthoritativeCurrentReaderV1 = (*ReviewBindingCurrentOwnerV1)(nil)
var _ ports.ReviewBindingConsumerAssociationCurrentReaderV1 = (*ReviewBindingCurrentOwnerV1)(nil)
var _ ports.ReviewBindingProjectionPublisherV1 = (*ReviewBindingCurrentOwnerV1)(nil)
var _ ReviewBindingAssociationProjectionPublisherV1 = (*ReviewBindingCurrentOwnerV1)(nil)
