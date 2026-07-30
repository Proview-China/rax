package selection

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type ConfigV1 struct {
	Closures   ports.VerifiedAgentPackageClosureReaderV1
	Selections ports.AgentPackageSelectionCurrentRepositoryV1
	Clock      func() time.Time
}

// ServiceV1 publishes only a nominal package selection. It does not construct
// components, call Host/Runtime/Factory, or grant production eligibility.
type ServiceV1 struct {
	closures   ports.VerifiedAgentPackageClosureReaderV1
	selections ports.AgentPackageSelectionCurrentRepositoryV1
	clock      func() time.Time
}

func NewServiceV1(config ConfigV1) (*ServiceV1, error) {
	if nilInterfaceV1(config.Closures) || nilInterfaceV1(config.Selections) {
		return nil, invalidV1("package selection service requires closure loader and selection Owner store")
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}
	return &ServiceV1{closures: config.Closures, selections: config.Selections, clock: config.Clock}, nil
}

func (service *ServiceV1) SelectPackageV1(ctx context.Context, request contract.AgentPackageSelectionRequestV1) (contract.AgentPackageSelectionCurrentV1, error) {
	if service == nil || nilInterfaceV1(service.closures) || nilInterfaceV1(service.selections) || service.clock == nil {
		return contract.AgentPackageSelectionCurrentV1{}, invalidV1("package selection service is unavailable")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.AgentPackageSelectionCurrentV1{}, unavailableV1("package selection requires live context")
	}
	now := service.clock()
	if err := request.ValidateCurrent(now); err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, err
	}

	// The service always performs a fresh exact reread. Callers cannot submit a
	// Publication ref or a closure digest for the selected current.
	closure, err := service.closures.LoadVerifiedAgentPackageClosureV1(ctx, request.PackageRef)
	if err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, err
	}
	if err = closure.Validate(); err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, err
	}
	if closure.Package.RefV1() != request.PackageRef {
		return contract.AgentPackageSelectionCurrentV1{}, driftV1("verified closure returned another Package ref")
	}

	revision := core.Revision(1)
	if !request.ExpectedCurrent.IsZero() {
		revision = request.ExpectedCurrent.Revision + 1
		if revision == 0 {
			return contract.AgentPackageSelectionCurrentV1{}, invalidV1("package selection revision overflowed")
		}
	}
	current, err := contract.SealAgentPackageSelectionCurrentV1(contract.AgentPackageSelectionCurrentV1{
		Ref: contract.AgentPackageSelectionCurrentRefV1{
			SelectionID:     request.SelectionID,
			Revision:        revision,
			ExpiresUnixNano: request.RequestedNotAfterUnixNano,
		},
		PackageRef:      closure.Package.RefV1(),
		PublicationRef:  closure.PublicationRefV2(),
		ClosureDigest:   closure.ClosureDigest,
		CheckedUnixNano: now.UnixNano(),
		ExpiresUnixNano: request.RequestedNotAfterUnixNano,
	})
	if err != nil {
		return contract.AgentPackageSelectionCurrentV1{}, err
	}

	result, err := service.selections.CompareAndSwapAgentPackageSelectionCurrentV1(ctx, request.ExpectedCurrent, current)
	if err == nil {
		if !reflect.DeepEqual(result, current) {
			return contract.AgentPackageSelectionCurrentV1{}, driftV1("package selection Owner returned another current")
		}
		return cloneV1(result), nil
	}
	if !core.HasCategory(err, core.ErrorIndeterminate) {
		return contract.AgentPackageSelectionCurrentV1{}, err
	}

	// Unknown write outcome is permanently Inspect-only: no retry, new
	// revision, alternate Package or alternate selection ID is attempted.
	inspected, inspectErr := service.selections.InspectAgentPackageSelectionExactV1(
		context.WithoutCancel(ctx),
		current.RefV1(),
	)
	if inspectErr != nil {
		return contract.AgentPackageSelectionCurrentV1{}, errors.Join(err, inspectErr)
	}
	if !reflect.DeepEqual(inspected, current) {
		return contract.AgentPackageSelectionCurrentV1{}, driftV1("package selection recovery observed another current body")
	}
	return cloneV1(inspected), nil
}

func (service *ServiceV1) InspectCurrentV1(ctx context.Context, selectionID string) (contract.AgentPackageSelectionCurrentV1, error) {
	if service == nil || nilInterfaceV1(service.selections) || service.clock == nil {
		return contract.AgentPackageSelectionCurrentV1{}, invalidV1("package selection service is unavailable")
	}
	if ctx == nil || ctx.Err() != nil {
		return contract.AgentPackageSelectionCurrentV1{}, unavailableV1("package selection Inspect requires live context")
	}
	return service.selections.InspectAgentPackageSelectionCurrentV1(ctx, selectionID)
}

func cloneV1[T any](value T) T {
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var result T
	if json.Unmarshal(raw, &result) != nil {
		return value
	}
	return result
}

func nilInterfaceV1(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func, reflect.Chan:
		return reflected.IsNil()
	}
	return false
}

func invalidV1(message string) error {
	return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, message)
}
func unavailableV1(message string) error {
	return core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, message)
}
func driftV1(message string) error {
	return core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingDrift, message)
}
