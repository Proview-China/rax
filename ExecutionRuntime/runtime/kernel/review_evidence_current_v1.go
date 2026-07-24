package kernel

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ReviewEvidenceApplicabilityGatewayV1 is the Runtime Evidence Owner's public
// read boundary plus its owner-only publisher. It creates no Review fact and
// grants no authority: every current read is grounded in the public
// EvidenceSubject current Reader and an atomic applicability fact snapshot.
type ReviewEvidenceApplicabilityGatewayV1 struct {
	Facts    control.ReviewEvidenceApplicabilityFactPortV1
	Evidence ports.EvidenceSubjectCurrentReaderV1
	Clock    func() time.Time
}

func (g ReviewEvidenceApplicabilityGatewayV1) ResolveReviewEvidenceApplicabilityCurrentV1(ctx context.Context, request ports.ResolveReviewEvidenceApplicabilityCurrentRequestV1) (ports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	if err := g.validateReviewEvidenceDependenciesV1(); err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	subjectDigest, err := ports.DigestReviewEvidenceApplicabilitySubjectV1(request.Subject)
	if err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	return g.inspectReviewEvidenceCurrentV1(ctx, subjectDigest, nil, &request.Subject)
}

func (g ReviewEvidenceApplicabilityGatewayV1) InspectCurrentReviewEvidenceApplicabilityV1(ctx context.Context, expected ports.ReviewEvidenceApplicabilityRefV1) (ports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	if err := g.validateReviewEvidenceDependenciesV1(); err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	return g.inspectReviewEvidenceCurrentV1(ctx, expected.SubjectDigest, &expected, nil)
}

func (g ReviewEvidenceApplicabilityGatewayV1) InspectHistoricalReviewEvidenceApplicabilityV1(ctx context.Context, expected ports.ReviewEvidenceApplicabilityRefV1) (ports.ReviewEvidenceApplicabilityProjectionV1, error) {
	if err := g.validateReviewEvidenceFactsV1(); err != nil {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, err
	}
	if err := expected.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, err
	}
	projection, err := g.Facts.InspectReviewEvidenceApplicabilityProjectionFactV1(ctx, expected)
	if err != nil {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, err
	}
	if projection.Ref != expected || projection.Validate() != nil {
		return ports.ReviewEvidenceApplicabilityProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability historical projection drifted")
	}
	return ports.CloneReviewEvidenceApplicabilityProjectionV1(projection), nil
}

func (g ReviewEvidenceApplicabilityGatewayV1) inspectReviewEvidenceCurrentV1(ctx context.Context, subjectDigest core.Digest, expected *ports.ReviewEvidenceApplicabilityRefV1, subject *ports.ReviewEvidenceApplicabilitySubjectV1) (ports.ReviewEvidenceApplicabilityCurrentSnapshotV1, error) {
	baseline, err := g.reviewEvidenceClockV1(time.Time{})
	if err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	s1, err := g.Facts.InspectReviewEvidenceApplicabilityCurrentFactV1(ctx, subjectDigest)
	if err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	if err := s1.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	if s1.Projection.SubjectDigest != subjectDigest || expected != nil && s1.Projection.Ref != *expected || subject != nil && !reflect.DeepEqual(s1.Projection.Subject, *subject) {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability S1 belongs to another exact request")
	}
	if err := g.inspectReviewEvidenceSubjectCurrentV1(ctx, s1.Projection); err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	s2, err := g.Facts.InspectReviewEvidenceApplicabilityCurrentFactV1(ctx, subjectDigest)
	if err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	if !reflect.DeepEqual(s1, s2) {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceConflict, "Review evidence applicability S1 and S2 snapshots differ")
	}
	if err := g.validateReviewEvidenceSubjectCurrentV1(ctx, s1.Projection); err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	now, err := g.reviewEvidenceClockV1(baseline)
	if err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	if err := s2.ValidateCurrent(s1.Projection.Ref, now); err != nil {
		return ports.ReviewEvidenceApplicabilityCurrentSnapshotV1{}, err
	}
	return ports.CloneReviewEvidenceApplicabilityCurrentSnapshotV1(s2), nil
}

func (g ReviewEvidenceApplicabilityGatewayV1) inspectReviewEvidenceSubjectCurrentV1(ctx context.Context, applicability ports.ReviewEvidenceApplicabilityProjectionV1) error {
	expected := applicability.EvidenceSubjectSnapshot
	p := expected.Projection
	lookup := ports.EvidenceSubjectCurrentLookupRequestV1{
		ContractVersion:              ports.EvidenceSubjectCurrentContractVersionV1,
		Subject:                      applicability.EvidenceSubject,
		ExpectedConsumer:             p.ReaderBinding.Binding,
		ExpectedExecutionScopeDigest: p.ExecutionScopeDigest,
		ExpectedSourcePolicy:         p.SourcePolicy,
	}
	if err := lookup.Validate(); err != nil {
		return err
	}
	s1, err := g.Evidence.InspectEvidenceSubjectCurrentV1(ctx, lookup)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(s1, expected) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability EvidenceSubject S1 drifted")
	}
	return nil
}

func (g ReviewEvidenceApplicabilityGatewayV1) validateReviewEvidenceSubjectCurrentV1(ctx context.Context, applicability ports.ReviewEvidenceApplicabilityProjectionV1) error {
	expected := applicability.EvidenceSubjectSnapshot
	p := expected.Projection
	validation := ports.EvidenceSubjectCurrentValidationRequestV1{
		ContractVersion:              ports.EvidenceSubjectCurrentContractVersionV1,
		Subject:                      applicability.EvidenceSubject,
		ExpectedProjection:           p.Ref,
		ExpectedCurrentIndex:         expected.CurrentIndex,
		ExpectedRegistration:         p.Registration,
		ExpectedReaderBinding:        p.ReaderBinding,
		ExpectedReaderCapability:     p.ReaderCapability,
		ExpectedConsumer:             p.ReaderBinding.Binding,
		ExpectedExecutionScopeDigest: p.ExecutionScopeDigest,
		ExpectedSourcePolicy:         p.SourcePolicy,
	}
	if err := validation.Validate(); err != nil {
		return err
	}
	s2, err := g.Evidence.ValidateEvidenceSubjectCurrentV1(ctx, validation)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(s2, expected) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability EvidenceSubject S2 drifted")
	}
	return nil
}

func (g ReviewEvidenceApplicabilityGatewayV1) PublishReviewEvidenceApplicabilityV1(ctx context.Context, request ports.PublishReviewEvidenceApplicabilityRequestV1) (ports.ReviewEvidenceApplicabilityPublishReceiptV1, error) {
	if err := g.validateReviewEvidenceDependenciesV1(); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if err := request.Validate(); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	baseline, err := g.reviewEvidenceClockV1(time.Time{})
	if err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if err := g.inspectReviewEvidenceSubjectCurrentV1(ctx, request.Projection); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if err := g.validateReviewEvidenceSubjectCurrentV1(ctx, request.Projection); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	now, err := g.reviewEvidenceClockV1(baseline)
	if err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if err := request.Projection.ValidateCurrent(request.Projection.Ref, now); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	receipt, err := ports.SealReviewEvidenceApplicabilityPublishReceiptV1(ports.ReviewEvidenceApplicabilityPublishReceiptV1{
		RequestDigest: request.RequestDigest, Projection: request.Projection.Ref, CurrentIndex: request.NextCurrentIndex, CommittedUnixNano: now.UnixNano(),
	})
	if err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	committed, err := g.Facts.PublishReviewEvidenceApplicabilityFactV1(ctx, request, receipt)
	if err != nil {
		if !core.HasCategory(err, core.ErrorIndeterminate) {
			return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
		}
		// Lost replies recover only by exact receipt inspection and deliberately
		// ignore cancellation of the original mutation context.
		committed, err = g.Facts.InspectReviewEvidenceApplicabilityPublishFactV1(context.WithoutCancel(ctx), string(request.RequestDigest))
		if err != nil {
			return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
		}
	}
	if err := validateReviewEvidenceReceiptV1(committed, request); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	return ports.CloneReviewEvidenceApplicabilityPublishReceiptV1(committed), nil
}

func (g ReviewEvidenceApplicabilityGatewayV1) InspectReviewEvidenceApplicabilityPublishV1(ctx context.Context, publishID string) (ports.ReviewEvidenceApplicabilityPublishReceiptV1, error) {
	if err := g.validateReviewEvidenceFactsV1(); err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if strings.TrimSpace(publishID) == "" {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review evidence applicability PublishID is required")
	}
	receipt, err := g.Facts.InspectReviewEvidenceApplicabilityPublishFactV1(ctx, publishID)
	if err != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, err
	}
	if receipt.PublishID != publishID || receipt.Validate() != nil {
		return ports.ReviewEvidenceApplicabilityPublishReceiptV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability inspected receipt drifted")
	}
	return ports.CloneReviewEvidenceApplicabilityPublishReceiptV1(receipt), nil
}

func validateReviewEvidenceReceiptV1(receipt ports.ReviewEvidenceApplicabilityPublishReceiptV1, request ports.PublishReviewEvidenceApplicabilityRequestV1) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	if receipt.PublishID != string(request.RequestDigest) || receipt.RequestDigest != request.RequestDigest {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability receipt request identity is not canonical")
	}
	if receipt.Projection != request.Projection.Ref {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability receipt Projection is not canonical")
	}
	if !reflect.DeepEqual(receipt.CurrentIndex, request.NextCurrentIndex) {
		return core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Review evidence applicability receipt current index is not canonical")
	}
	return nil
}

func (g ReviewEvidenceApplicabilityGatewayV1) reviewEvidenceClockV1(baseline time.Time) (time.Time, error) {
	now := g.Clock()
	if now.IsZero() || !baseline.IsZero() && now.Before(baseline) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "Review evidence applicability clock regressed")
	}
	return now, nil
}

func (g ReviewEvidenceApplicabilityGatewayV1) validateReviewEvidenceDependenciesV1() error {
	if err := g.validateReviewEvidenceFactsV1(); err != nil {
		return err
	}
	if nilOrTypedNilReviewEvidenceV1(g.Evidence) || g.Clock == nil {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Review evidence applicability Gateway dependency is unavailable")
	}
	return nil
}

func (g ReviewEvidenceApplicabilityGatewayV1) validateReviewEvidenceFactsV1() error {
	if nilOrTypedNilReviewEvidenceV1(g.Facts) {
		return core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Review evidence applicability Fact Owner is unavailable")
	}
	return nil
}

func nilOrTypedNilReviewEvidenceV1(value any) bool {
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

var _ ports.ReviewEvidenceApplicabilityCurrentReaderV1 = ReviewEvidenceApplicabilityGatewayV1{}
var _ ports.ReviewEvidenceApplicabilityOwnerPublisherV1 = ReviewEvidenceApplicabilityGatewayV1{}
