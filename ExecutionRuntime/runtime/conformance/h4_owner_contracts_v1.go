package conformance

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type AgentExecutionAvailabilityCaseV1 struct {
	Reader ports.AgentExecutionAvailabilityCurrentReaderV1
	Ref    ports.AgentExecutionAvailabilityRefV1
	Now    time.Time
}

type AgentExecutionAvailabilityReportV1 struct {
	ExactCurrentObserved    bool `json:"exact_current_observed"`
	ReadyEpochObserved      bool `json:"ready_epoch_observed"`
	ProductionClaimEligible bool `json:"production_claim_eligible"`
}

func CheckAgentExecutionAvailabilityV1(ctx context.Context, testCase AgentExecutionAvailabilityCaseV1) (AgentExecutionAvailabilityReportV1, error) {
	if nilInterfaceH4V1(testCase.Reader) {
		return AgentExecutionAvailabilityReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Agent execution availability Reader is required")
	}
	projection, err := testCase.Reader.InspectAgentExecutionAvailabilityCurrentV1(ctx, testCase.Ref)
	if err != nil {
		return AgentExecutionAvailabilityReportV1{}, err
	}
	if err := projection.ValidateCurrent(testCase.Ref, testCase.Now); err != nil {
		return AgentExecutionAvailabilityReportV1{}, err
	}
	return AgentExecutionAvailabilityReportV1{ExactCurrentObserved: true, ReadyEpochObserved: true, ProductionClaimEligible: false}, nil
}

type ResourceBindingCaseV1 struct {
	Reader ports.ResourceCurrentReaderV1
	Set    ports.ResourceBindingSetRefV1
	Now    time.Time
}

type ResourceBindingReportV1 struct {
	SetCurrentObserved      bool `json:"set_current_observed"`
	AllHandlesCurrent       bool `json:"all_handles_current"`
	ProductionClaimEligible bool `json:"production_claim_eligible"`
}

func CheckResourceBindingV1(ctx context.Context, testCase ResourceBindingCaseV1) (ResourceBindingReportV1, error) {
	if nilInterfaceH4V1(testCase.Reader) {
		return ResourceBindingReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Resource current Reader is required")
	}
	set, err := testCase.Reader.InspectResourceBindingSetCurrentV1(ctx, testCase.Set)
	if err != nil {
		return ResourceBindingReportV1{}, err
	}
	if err := set.ValidateCurrent(testCase.Set, testCase.Now); err != nil {
		return ResourceBindingReportV1{}, err
	}
	for _, binding := range set.Bindings {
		handle, inspectErr := testCase.Reader.InspectResourceHandleCurrentV1(ctx, binding.Handle)
		if inspectErr != nil {
			return ResourceBindingReportV1{}, inspectErr
		}
		if err := handle.ValidateCurrent(binding.Handle, testCase.Now); err != nil {
			return ResourceBindingReportV1{}, err
		}
		if handle.CleanupContract != binding.CleanupContract || handle.DeploymentAttestation != binding.DeploymentAttestation {
			return ResourceBindingReportV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "Resource BindingSet spliced handle proofs")
		}
	}
	return ResourceBindingReportV1{SetCurrentObserved: true, AllHandlesCurrent: true, ProductionClaimEligible: false}, nil
}

type BindingAdmissionCaseV1 struct {
	Gateway ports.BindingAdmissionGovernancePortV1
	Request ports.BindingAdmissionRequestV1
	Now     time.Time
}

type BindingAdmissionReportV1 struct {
	StartOrInspectObserved  bool `json:"start_or_inspect_observed"`
	ExactInspectObserved    bool `json:"exact_inspect_observed"`
	PreBindingOnly          bool `json:"pre_binding_only"`
	ProductionClaimEligible bool `json:"production_claim_eligible"`
}

func CheckBindingAdmissionV1(ctx context.Context, testCase BindingAdmissionCaseV1) (BindingAdmissionReportV1, error) {
	if nilInterfaceH4V1(testCase.Gateway) {
		return BindingAdmissionReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Binding admission governance Port is required")
	}
	if err := testCase.Request.ValidateCurrent(testCase.Now); err != nil {
		return BindingAdmissionReportV1{}, err
	}
	result, err := testCase.Gateway.StartOrInspectBindingAdmissionV1(ctx, testCase.Request)
	if err != nil {
		return BindingAdmissionReportV1{}, err
	}
	if err := result.ValidateCurrent(testCase.Request, testCase.Now); err != nil {
		return BindingAdmissionReportV1{}, err
	}
	inspected, err := testCase.Gateway.InspectBindingAdmissionV1(ctx, ports.BindingAdmissionInspectRequestV1{AttemptID: testCase.Request.AttemptID, RequestDigest: testCase.Request.RequestDigest})
	if err != nil {
		return BindingAdmissionReportV1{}, err
	}
	if inspected.ResultDigest != result.ResultDigest {
		return BindingAdmissionReportV1{}, core.NewError(core.ErrorConflict, core.ReasonBindingSetConflict, "Binding admission Inspect returned another result")
	}
	return BindingAdmissionReportV1{StartOrInspectObserved: true, ExactInspectObserved: true, PreBindingOnly: true, ProductionClaimEligible: false}, nil
}

func nilInterfaceH4V1(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}
