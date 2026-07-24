package conformance

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ReviewWaitingInputCurrentCaseV2 struct {
	Reader   applicationports.ReviewWaitingInputExactCurrentReaderV2
	Current  contract.ReviewWaitingInputCurrentRequestV2
	Expected contract.ReviewWaitingInputCurrentProjectionV2
	Missing  contract.ReviewWaitingInputCurrentRequestV2
	TypePun  contract.ReviewWaitingInputCurrentRequestV2
	Drift    contract.ReviewWaitingInputCurrentRequestV2
	Expired  contract.ReviewWaitingInputCurrentRequestV2
	Now      time.Time
}

type ReviewWaitingInputCurrentReportV2 struct {
	ExactRead          bool `json:"exact_read"`
	S1S2Stable         bool `json:"s1_s2_stable"`
	DeepClone          bool `json:"deep_clone"`
	MissingFailClosed  bool `json:"missing_fail_closed"`
	TypePunFailClosed  bool `json:"type_pun_fail_closed"`
	DriftFailClosed    bool `json:"drift_fail_closed"`
	TTLFailClosed      bool `json:"ttl_fail_closed"`
	ClosedErrorSet     bool `json:"closed_error_set"`
	ProductionEligible bool `json:"production_eligible"`
}

// CheckReviewWaitingInputExactCurrentReaderV2 is reusable against a future
// trusted host adapter. Passing this suite does not prove a production root.
func CheckReviewWaitingInputExactCurrentReaderV2(ctx context.Context, testCase ReviewWaitingInputCurrentCaseV2) (ReviewWaitingInputCurrentReportV2, error) {
	report := ReviewWaitingInputCurrentReportV2{ProductionEligible: false}
	if ctx == nil || reviewWaitingInputReaderNilV2(testCase.Reader) || testCase.Now.IsZero() {
		return report, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Review waiting input conformance dependencies are required")
	}
	if err := testCase.Current.ValidateCurrent(testCase.Now); err != nil {
		return report, err
	}
	if err := testCase.Expected.ValidateCurrentFor(testCase.Current, testCase.Now); err != nil {
		return report, err
	}

	first, err := testCase.Reader.InspectReviewWaitingInputExactCurrentV2(ctx, testCase.Current)
	if err != nil {
		return report, err
	}
	if err := first.ValidateCurrentFor(testCase.Current, testCase.Now); err != nil || !reflect.DeepEqual(first, testCase.Expected) {
		return report, reviewWaitingInputConformanceErrorV2(err, "Review waiting input exact read drifted")
	}
	report.ExactRead = true

	second, err := testCase.Reader.InspectReviewWaitingInputExactCurrentV2(ctx, testCase.Current)
	if err != nil {
		return report, err
	}
	if !reflect.DeepEqual(first, second) {
		return report, core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, "Review waiting input changed across S1/S2")
	}
	report.S1S2Stable = true

	mutated := first.Clone()
	mutated.Source.ID += "-caller-mutation"
	third, err := testCase.Reader.InspectReviewWaitingInputExactCurrentV2(ctx, testCase.Current)
	if err != nil || !reflect.DeepEqual(third, testCase.Expected) {
		return report, reviewWaitingInputConformanceErrorV2(err, "Review waiting input Reader retained caller mutation")
	}
	report.DeepClone = true

	checks := []struct {
		request  contract.ReviewWaitingInputCurrentRequestV2
		category core.ErrorCategory
		set      func()
	}{
		{testCase.Missing, core.ErrorNotFound, func() { report.MissingFailClosed = true }},
		{testCase.TypePun, core.ErrorConflict, func() { report.TypePunFailClosed = true }},
		{testCase.Drift, core.ErrorConflict, func() { report.DriftFailClosed = true }},
		{testCase.Expired, core.ErrorPreconditionFailed, func() { report.TTLFailClosed = true }},
	}
	for _, check := range checks {
		_, inspectErr := testCase.Reader.InspectReviewWaitingInputExactCurrentV2(ctx, check.request)
		if !core.HasCategory(inspectErr, check.category) || !applicationports.IsReviewWaitingInputExactCurrentClosedErrorV2(inspectErr) {
			return report, core.NewError(core.ErrorConflict, core.ReasonInvalidState, "Review waiting input Reader returned an open or mistyped error")
		}
		check.set()
	}
	report.ClosedErrorSet = true
	return report, nil
}

func reviewWaitingInputReaderNilV2(value any) bool {
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

func reviewWaitingInputConformanceErrorV2(err error, message string) error {
	if err != nil {
		return err
	}
	return core.NewError(core.ErrorConflict, core.ReasonReviewCandidateConflict, message)
}
