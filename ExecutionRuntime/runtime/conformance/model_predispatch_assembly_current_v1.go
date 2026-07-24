package conformance

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type RegistrySnapshotExactReaderCaseV1 struct {
	Reader   ports.RegistrySnapshotExactReaderV1
	Expected ports.RegistrySnapshotRefV1
}

type RegistrySnapshotExactReaderReportV1 struct {
	ExactCurrentObserved    bool `json:"exact_current_observed"`
	MutationAuthorityUsed   bool `json:"mutation_authority_used"`
	ProductionClaimEligible bool `json:"production_claim_eligible"`
}

func CheckRegistrySnapshotExactReaderV1(ctx context.Context, testCase RegistrySnapshotExactReaderCaseV1) (RegistrySnapshotExactReaderReportV1, error) {
	if modelPreDispatchReaderNilV1(testCase.Reader) {
		return RegistrySnapshotExactReaderReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Registry snapshot exact Reader is required")
	}
	if err := testCase.Expected.Validate(); err != nil {
		return RegistrySnapshotExactReaderReportV1{}, err
	}
	actual, err := testCase.Reader.InspectExactRegistrySnapshotV1(ctx, testCase.Expected)
	if err != nil {
		return RegistrySnapshotExactReaderReportV1{}, err
	}
	if err := actual.Validate(); err != nil {
		return RegistrySnapshotExactReaderReportV1{}, err
	}
	if actual != testCase.Expected {
		return RegistrySnapshotExactReaderReportV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Registry snapshot exact Reader returned another Ref")
	}
	return RegistrySnapshotExactReaderReportV1{ExactCurrentObserved: true, MutationAuthorityUsed: false, ProductionClaimEligible: false}, nil
}

type ModelPreDispatchAssemblyCurrentReaderCaseV1 struct {
	Reader   ports.ModelPreDispatchAssemblyCurrentReaderV1
	Expected ports.ModelPreDispatchAssemblyCurrentRefV1
	Now      time.Time
}

type ModelPreDispatchAssemblyCurrentReaderReportV1 struct {
	ExactCurrentObserved    bool `json:"exact_current_observed"`
	PublishAuthorityUsed    bool `json:"publish_authority_used"`
	ProductionClaimEligible bool `json:"production_claim_eligible"`
}

func CheckModelPreDispatchAssemblyCurrentReaderV1(ctx context.Context, testCase ModelPreDispatchAssemblyCurrentReaderCaseV1) (ModelPreDispatchAssemblyCurrentReaderReportV1, error) {
	if modelPreDispatchReaderNilV1(testCase.Reader) {
		return ModelPreDispatchAssemblyCurrentReaderReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Model pre-dispatch Assembly current Reader is required")
	}
	if err := testCase.Expected.Validate(); err != nil {
		return ModelPreDispatchAssemblyCurrentReaderReportV1{}, err
	}
	projection, err := testCase.Reader.InspectCurrentModelPreDispatchAssemblyV1(ctx, testCase.Expected)
	if err != nil {
		return ModelPreDispatchAssemblyCurrentReaderReportV1{}, err
	}
	if err := projection.ValidateCurrent(testCase.Expected, testCase.Now); err != nil {
		return ModelPreDispatchAssemblyCurrentReaderReportV1{}, err
	}
	return ModelPreDispatchAssemblyCurrentReaderReportV1{ExactCurrentObserved: true, PublishAuthorityUsed: false, ProductionClaimEligible: false}, nil
}

func modelPreDispatchReaderNilV1(reader any) bool {
	if reader == nil {
		return true
	}
	value := reflect.ValueOf(reader)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
