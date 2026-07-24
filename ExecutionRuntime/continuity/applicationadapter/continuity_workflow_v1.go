package applicationadapter

import (
	"context"
	"reflect"
	"time"

	appcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	continuitycontract "github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// GovernedWorkflowAdapterV1 is a narrow component-side adapter. It does not
// construct Application bundles, call Continuity repositories, or execute a
// provider; it only submits an already-persisted Continuity exact request ref
// through the public Application gateway.
type GovernedWorkflowAdapterV1 struct {
	gateway applicationports.ContinuityWorkflowSubmissionGatewayV1
}

func NewGovernedWorkflowAdapterV1(gateway applicationports.ContinuityWorkflowSubmissionGatewayV1) (*GovernedWorkflowAdapterV1, error) {
	if nilGovernedWorkflowValueV1(gateway) {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Application Continuity workflow gateway is required")
	}
	return &GovernedWorkflowAdapterV1{gateway: gateway}, nil
}

func (a *GovernedWorkflowAdapterV1) Submit(ctx context.Context, request appcontract.ContinuityWorkflowRequestV1) (appcontract.ContinuityWorkflowInspectionV1, error) {
	if err := validateContinuityOwnedWorkflowRequestV1(request); err != nil {
		return appcontract.ContinuityWorkflowInspectionV1{}, err
	}
	result, err := a.gateway.SubmitContinuityWorkflowV1(ctx, request)
	if err != nil {
		return appcontract.ContinuityWorkflowInspectionV1{}, err
	}
	return validateCloneWorkflowInspectionV1(request, result)
}

func (a *GovernedWorkflowAdapterV1) Inspect(ctx context.Context, request appcontract.ContinuityWorkflowRequestV1) (appcontract.ContinuityWorkflowInspectionV1, error) {
	if err := validateContinuityOwnedWorkflowRequestV1(request); err != nil {
		return appcontract.ContinuityWorkflowInspectionV1{}, err
	}
	result, err := a.gateway.InspectContinuityWorkflowV1(ctx, request)
	if err != nil {
		return appcontract.ContinuityWorkflowInspectionV1{}, err
	}
	return validateCloneWorkflowInspectionV1(request, result)
}

func validateContinuityOwnedWorkflowRequestV1(request appcontract.ContinuityWorkflowRequestV1) error {
	if err := request.Validate(time.Time{}); err != nil {
		return err
	}
	if string(request.DomainRequest.Owner.ComponentID) != continuitycontract.ContinuityComponentID {
		return core.NewError(core.ErrorForbidden, core.ReasonOwnerMissing, "workflow domain request is not owned by Continuity")
	}
	return nil
}

func validateCloneWorkflowInspectionV1(request appcontract.ContinuityWorkflowRequestV1, result appcontract.ContinuityWorkflowInspectionV1) (appcontract.ContinuityWorkflowInspectionV1, error) {
	if err := result.ValidateFor(request); err != nil {
		return appcontract.ContinuityWorkflowInspectionV1{}, err
	}
	result.Steps = append([]appcontract.ContinuityWorkflowStepRefV1(nil), result.Steps...)
	if result.Journal != nil {
		journal := *result.Journal
		result.Journal = &journal
	}
	return result, nil
}

func nilGovernedWorkflowValueV1(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
