package conformance

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ReviewDecisionPolicyCurrentCaseV2 struct {
	Reader    ports.ReviewDecisionPolicyCurrentReaderV2
	Publisher ports.ReviewDecisionPolicyCurrentPublisherV2
	Initial   ports.ReviewDecisionPolicyCurrentPublishRequestV2
	Next      *ports.ReviewDecisionPolicyCurrentPublishRequestV2
}
type ReviewDecisionPolicyCurrentReportV2 struct {
	ExactCurrent       bool `json:"exact_current"`
	AppendOnlyHistory  bool `json:"append_only_history"`
	FullRefCAS         bool `json:"full_ref_cas"`
	ProductionEligible bool `json:"production_eligible"`
}

// RunReviewDecisionPolicyCurrentV2 checks the public reference contract only.
// Passing does not certify production composition, durability, HA or an SLA.
func RunReviewDecisionPolicyCurrentV2(ctx context.Context, test ReviewDecisionPolicyCurrentCaseV2) (ReviewDecisionPolicyCurrentReportV2, error) {
	if reviewDecisionConformanceNilV1(test.Reader) || reviewDecisionConformanceNilV1(test.Publisher) {
		return ReviewDecisionPolicyCurrentReportV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review decision policy V2 conformance dependencies are missing")
	}
	if _, err := test.Publisher.PublishReviewDecisionPolicyCurrentV2(ctx, test.Initial); err != nil {
		return ReviewDecisionPolicyCurrentReportV2{}, err
	}
	ref, err := test.Reader.ResolveCurrentReviewDecisionPolicyV2(ctx, ports.ReviewDecisionPolicyCurrentResolveRequestV2{Subject: test.Initial.Value.Subject})
	if err != nil {
		return ReviewDecisionPolicyCurrentReportV2{}, err
	}
	p, err := test.Reader.InspectCurrentReviewDecisionPolicyV2(ctx, test.Initial.Value.Subject, ref)
	if err != nil {
		return ReviewDecisionPolicyCurrentReportV2{}, err
	}
	if !reflect.DeepEqual(p, test.Initial.Value) {
		return ReviewDecisionPolicyCurrentReportV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review decision policy V2 exact current read drifted")
	}
	historical, err := test.Reader.InspectHistoricalReviewDecisionPolicyV2(ctx, test.Initial.Value.Ref)
	if err != nil || !reflect.DeepEqual(historical, test.Initial.Value) {
		return ReviewDecisionPolicyCurrentReportV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review decision policy V2 historical read drifted")
	}
	fullRef := false
	if test.Next != nil {
		if _, err := test.Publisher.PublishReviewDecisionPolicyCurrentV2(ctx, *test.Next); err != nil {
			return ReviewDecisionPolicyCurrentReportV2{}, err
		}
		if _, err := test.Reader.InspectCurrentReviewDecisionPolicyV2(ctx, test.Initial.Value.Subject, test.Initial.Value.Ref); !core.HasCategory(err, core.ErrorConflict) {
			return ReviewDecisionPolicyCurrentReportV2{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review decision policy V2 stale full Ref was accepted")
		}
		fullRef = true
		again, err := test.Reader.InspectHistoricalReviewDecisionPolicyV2(ctx, test.Initial.Value.Ref)
		if err != nil || !reflect.DeepEqual(again, test.Initial.Value) {
			return ReviewDecisionPolicyCurrentReportV2{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Review decision policy V2 append-only history was rewritten")
		}
	}
	return ReviewDecisionPolicyCurrentReportV2{ExactCurrent: true, AppendOnlyHistory: true, FullRefCAS: fullRef, ProductionEligible: false}, nil
}
