package conformance

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ReviewWaitingCoordinationFixtureV1 struct {
	Initial contract.ReviewWaitingCoordinationFactV1
	Claimed contract.ReviewWaitingCoordinationFactV1
}

type ReviewWaitingCoordinationReportV1 struct {
	CreateOnce         bool
	FullRefCAS         bool
	HistoricalExact    bool
	IdempotentNonOwner bool
	ProductionEligible bool
}

func RunReviewWaitingCoordinationV1(ctx context.Context, store applicationports.ReviewWaitingCoordinationFactPortV1, fixture ReviewWaitingCoordinationFixtureV1) (ReviewWaitingCoordinationReportV1, error) {
	report := ReviewWaitingCoordinationReportV1{ProductionEligible: false}
	if store == nil || fixture.Initial.Validate() != nil || fixture.Claimed.Validate() != nil || contract.ValidateReviewWaitingCoordinationTransitionV1(fixture.Initial, fixture.Claimed) != nil {
		return report, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Review waiting conformance fixture is incomplete")
	}
	first, err := store.CreateReviewWaitingCoordinationV1(ctx, fixture.Initial)
	if err != nil || !reflect.DeepEqual(first.Fact, fixture.Initial) {
		return report, errOrReviewWaitingConformanceV1(err, "Review waiting create-once failed")
	}
	replayed, err := store.CreateReviewWaitingCoordinationV1(ctx, fixture.Initial)
	if err != nil || replayed.Created || !reflect.DeepEqual(replayed.Fact, fixture.Initial) {
		return report, errOrReviewWaitingConformanceV1(err, "Review waiting canonical create replay drifted")
	}
	report.CreateOnce = true
	request := applicationports.ReviewWaitingCoordinationCASRequestV1{Scope: fixture.Initial.Request.ExecutionScope, Expected: fixture.Initial.RefV1(), Next: fixture.Claimed}
	advanced, err := store.CompareAndSwapReviewWaitingCoordinationV1(ctx, request)
	if err != nil || !advanced.Applied || !reflect.DeepEqual(advanced.Fact, fixture.Claimed) {
		return report, errOrReviewWaitingConformanceV1(err, "Review waiting full-ref CAS failed")
	}
	report.FullRefCAS = true
	old, err := store.InspectHistoricalReviewWaitingCoordinationV1(ctx, fixture.Initial.Request.ExecutionScope, fixture.Initial.RefV1())
	if err != nil || !reflect.DeepEqual(old, fixture.Initial) {
		return report, errOrReviewWaitingConformanceV1(err, "Review waiting historical exact Inspect failed")
	}
	report.HistoricalExact = true
	replayedCAS, err := store.CompareAndSwapReviewWaitingCoordinationV1(ctx, request)
	if err != nil || replayedCAS.Applied || !reflect.DeepEqual(replayedCAS.Fact, fixture.Claimed) {
		return report, errOrReviewWaitingConformanceV1(err, "Review waiting idempotent non-owner receipt drifted")
	}
	report.IdempotentNonOwner = true
	return report, nil
}

func errOrReviewWaitingConformanceV1(err error, message string) error {
	if err != nil {
		return err
	}
	return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, message)
}
