package conformance

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	hostports "github.com/Proview-China/rax/ExecutionRuntime/agent-host/ports"
)

type ReviewModelInvocationAssociationFixtureV1 struct {
	Initial, Terminal contract.ReviewModelInvocationAssociationFactV1
}
type ReviewModelInvocationAssociationReportV1 struct {
	CreateOnce, S1S2Exact, DeepClone, FullRefCAS, HistoricalExact, IdempotentReplay bool
	ProductionEligible                                                              bool
}

func RunReviewModelInvocationAssociationV1(ctx context.Context, store hostports.ReviewModelInvocationAssociationPortV1, fixture ReviewModelInvocationAssociationFixtureV1) (ReviewModelInvocationAssociationReportV1, error) {
	report := ReviewModelInvocationAssociationReportV1{ProductionEligible: false}
	if contract.IsTypedNilV1(store) || fixture.Initial.ValidateHistoricalV1() != nil || fixture.Terminal.ValidateHistoricalV1() != nil || contract.ValidateReviewModelInvocationAssociationTransitionV1(fixture.Initial, fixture.Terminal) != nil {
		return report, contract.NewError(contract.ErrorInvalidArgument, "conformance_fixture_invalid", "association conformance fixture is incomplete")
	}
	first, err := store.CreateReviewModelInvocationAssociationV1(ctx, fixture.Initial)
	if err != nil || !first.Created || !reflect.DeepEqual(first.Fact, fixture.Initial) {
		return report, conformanceAssociationError(err, "create-once failed")
	}
	replay, err := store.CreateReviewModelInvocationAssociationV1(ctx, fixture.Initial)
	if err != nil || replay.Created || !reflect.DeepEqual(replay.Fact, fixture.Initial) {
		return report, conformanceAssociationError(err, "create replay drifted")
	}
	report.CreateOnce = true
	ref, err := store.ResolveCurrentReviewModelInvocationAssociationV1(ctx, fixture.Initial.Subject)
	if err != nil || ref != fixture.Initial.RefV1() {
		return report, conformanceAssociationError(err, "S1 Resolve drifted")
	}
	current, err := store.InspectCurrentReviewModelInvocationAssociationV1(ctx, fixture.Initial.Subject, ref)
	if err != nil || !reflect.DeepEqual(current, fixture.Initial) {
		return report, conformanceAssociationError(err, "S2 Inspect drifted")
	}
	report.S1S2Exact = true
	if len(current.Command.Call.Request.Input) > 0 {
		current.Command.Call.Request.Input[0].Type = "mutated"
	}
	again, err := store.InspectHistoricalReviewModelInvocationAssociationV1(ctx, fixture.Initial.RefV1())
	if err != nil || !reflect.DeepEqual(again, fixture.Initial) {
		return report, conformanceAssociationError(err, "defensive clone failed")
	}
	report.DeepClone = true
	request := hostports.ReviewModelInvocationAssociationCASRequestV1{Expected: fixture.Initial.RefV1(), Next: fixture.Terminal}
	advanced, err := store.CompareAndSwapReviewModelInvocationAssociationV1(ctx, request)
	if err != nil || !advanced.Applied || !reflect.DeepEqual(advanced.Fact, fixture.Terminal) {
		return report, conformanceAssociationError(err, "full-ref CAS failed")
	}
	report.FullRefCAS = true
	old, err := store.InspectHistoricalReviewModelInvocationAssociationV1(ctx, fixture.Initial.RefV1())
	if err != nil || !reflect.DeepEqual(old, fixture.Initial) {
		return report, conformanceAssociationError(err, "historical exact Inspect failed")
	}
	report.HistoricalExact = true
	replayedCAS, err := store.CompareAndSwapReviewModelInvocationAssociationV1(ctx, request)
	if err != nil || replayedCAS.Applied || !reflect.DeepEqual(replayedCAS.Fact, fixture.Terminal) {
		return report, conformanceAssociationError(err, "CAS replay drifted")
	}
	report.IdempotentReplay = true
	lateCreate, err := store.CreateReviewModelInvocationAssociationV1(ctx, fixture.Initial)
	if err != nil || lateCreate.Created || !reflect.DeepEqual(lateCreate.Fact, fixture.Initial) {
		return report, conformanceAssociationError(err, "historical create replay drifted after terminal CAS")
	}
	return report, nil
}
func conformanceAssociationError(err error, message string) error {
	if err != nil {
		return err
	}
	return contract.NewError(contract.ErrorConflict, "conformance_failed", message)
}
