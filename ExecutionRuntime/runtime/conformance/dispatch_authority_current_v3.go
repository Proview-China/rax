package conformance

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"reflect"
)

type DispatchAuthorityCurrentCaseV3 struct {
	Reader    ports.DispatchAuthorityCurrentReaderV3
	Publisher ports.DispatchAuthorityCurrentPublisherV3
	Initial   ports.DispatchAuthorityFactPublishRequestV3
	Next      *ports.DispatchAuthorityFactPublishRequestV3
}
type DispatchAuthorityCurrentReportV3 struct {
	ExactCurrent       bool `json:"exact_current"`
	AppendOnlyHistory  bool `json:"append_only_history"`
	FullRefCAS         bool `json:"full_ref_cas"`
	ProductionEligible bool `json:"production_eligible"`
}

func RunDispatchAuthorityCurrentV3(ctx context.Context, test DispatchAuthorityCurrentCaseV3) (DispatchAuthorityCurrentReportV3, error) {
	if reviewDecisionConformanceNilV1(test.Reader) || reviewDecisionConformanceNilV1(test.Publisher) {
		return DispatchAuthorityCurrentReportV3{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "dispatch authority V3 conformance dependencies are missing")
	}
	if _, err := test.Publisher.PublishDispatchAuthorityFactV3(ctx, test.Initial); err != nil {
		return DispatchAuthorityCurrentReportV3{}, err
	}
	current, err := test.Reader.InspectCurrentDispatchAuthorityV3(ctx, test.Initial.Value.Ref)
	if err != nil || !reflect.DeepEqual(current, test.Initial.Value) {
		return DispatchAuthorityCurrentReportV3{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "dispatch authority V3 exact current read drifted")
	}
	historical, err := test.Reader.InspectHistoricalDispatchAuthorityV3(ctx, test.Initial.Value.Ref)
	if err != nil || !reflect.DeepEqual(historical, test.Initial.Value) {
		return DispatchAuthorityCurrentReportV3{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "dispatch authority V3 historical read drifted")
	}
	full := false
	if test.Next != nil {
		if _, err := test.Publisher.PublishDispatchAuthorityFactV3(ctx, *test.Next); err != nil {
			return DispatchAuthorityCurrentReportV3{}, err
		}
		if _, err := test.Reader.InspectCurrentDispatchAuthorityV3(ctx, test.Initial.Value.Ref); !core.HasCategory(err, core.ErrorConflict) {
			return DispatchAuthorityCurrentReportV3{}, core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "dispatch authority V3 stale current Ref was accepted")
		}
		again, err := test.Reader.InspectHistoricalDispatchAuthorityV3(ctx, test.Initial.Value.Ref)
		if err != nil || !reflect.DeepEqual(again, test.Initial.Value) {
			return DispatchAuthorityCurrentReportV3{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "dispatch authority V3 append-only history was rewritten")
		}
		full = true
	}
	return DispatchAuthorityCurrentReportV3{ExactCurrent: true, AppendOnlyHistory: true, FullRefCAS: full, ProductionEligible: false}, nil
}
