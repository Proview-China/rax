package conformance

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"reflect"
)

type ReviewActorAuthorityCurrentCaseV2 struct {
	Reader    ports.ReviewActorAuthorityCurrentReaderV2
	Publisher ports.ReviewActorAuthorityCurrentPublisherV2
	Initial   ports.ReviewActorAuthorityCurrentPublishRequestV2
	Next      *ports.ReviewActorAuthorityCurrentPublishRequestV2
}
type ReviewActorAuthorityCurrentReportV2 struct {
	ExactCurrent       bool `json:"exact_current"`
	AppendOnlyHistory  bool `json:"append_only_history"`
	FullRefCAS         bool `json:"full_ref_cas"`
	ActorOnly          bool `json:"actor_only"`
	ProductionEligible bool `json:"production_eligible"`
}

func RunReviewActorAuthorityCurrentV2(ctx context.Context, test ReviewActorAuthorityCurrentCaseV2) (ReviewActorAuthorityCurrentReportV2, error) {
	if reviewDecisionConformanceNilV1(test.Reader) || reviewDecisionConformanceNilV1(test.Publisher) {
		return ReviewActorAuthorityCurrentReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review actor authority V2 conformance dependencies are missing")
	}
	if _, err := test.Publisher.PublishReviewActorAuthorityCurrentV2(ctx, test.Initial); err != nil {
		return ReviewActorAuthorityCurrentReportV2{}, err
	}
	ref, err := test.Reader.ResolveCurrentReviewActorAuthorityV2(ctx, ports.ReviewActorAuthorityCurrentResolveRequestV2{Subject: test.Initial.Value.Subject})
	if err != nil {
		return ReviewActorAuthorityCurrentReportV2{}, err
	}
	current, err := test.Reader.InspectCurrentReviewActorAuthorityV2(ctx, test.Initial.Value.Subject, ref)
	if err != nil || !reflect.DeepEqual(current, test.Initial.Value) {
		return ReviewActorAuthorityCurrentReportV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review actor authority V2 exact current read drifted")
	}
	historical, err := test.Reader.InspectHistoricalReviewActorAuthorityV2(ctx, test.Initial.Value.Ref)
	if err != nil || !reflect.DeepEqual(historical, test.Initial.Value) {
		return ReviewActorAuthorityCurrentReportV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review actor authority V2 historical read drifted")
	}
	full := false
	if test.Next != nil {
		if _, err := test.Publisher.PublishReviewActorAuthorityCurrentV2(ctx, *test.Next); err != nil {
			return ReviewActorAuthorityCurrentReportV2{}, err
		}
		if _, err := test.Reader.InspectCurrentReviewActorAuthorityV2(ctx, test.Initial.Value.Subject, test.Initial.Value.Ref); !core.HasCategory(err, core.ErrorConflict) {
			return ReviewActorAuthorityCurrentReportV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review actor authority V2 stale current Ref was accepted")
		}
		again, err := test.Reader.InspectHistoricalReviewActorAuthorityV2(ctx, test.Initial.Value.Ref)
		if err != nil || !reflect.DeepEqual(again, test.Initial.Value) {
			return ReviewActorAuthorityCurrentReportV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review actor authority V2 append-only history was rewritten")
		}
		full = true
	}
	return ReviewActorAuthorityCurrentReportV2{ExactCurrent: true, AppendOnlyHistory: true, FullRefCAS: full, ActorOnly: true, ProductionEligible: false}, nil
}
