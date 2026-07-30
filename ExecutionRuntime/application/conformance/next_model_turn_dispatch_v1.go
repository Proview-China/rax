package conformance

import (
	"context"
	"reflect"
	"strings"
	"sync"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	applicationports "github.com/Proview-China/rax/ExecutionRuntime/application/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var nextModelTurnDispatchAllowedImportsV1 = [...]string{
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core",
	"github.com/Proview-China/rax/ExecutionRuntime/application/contract",
	"github.com/Proview-China/rax/ExecutionRuntime/application/ports",
}

func NextModelTurnDispatchAllowedImportsV1() []string {
	result := make([]string, len(nextModelTurnDispatchAllowedImportsV1))
	copy(result, nextModelTurnDispatchAllowedImportsV1[:])
	return result
}

// CheckNextModelTurnDispatchImportsV1 prevents an Application implementation
// from reaching Model, Runtime guard, Provider, Harness-private, or Console
// packages. It is build hygiene, not production certification.
func CheckNextModelTurnDispatchImportsV1(imports []string) error {
	allowed := NextModelTurnDispatchAllowedImportsV1()
	for _, candidate := range imports {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "next-turn dispatcher import is empty")
		}
		if !strings.HasPrefix(candidate, "github.com/Proview-China/rax/ExecutionRuntime/") {
			continue
		}
		permitted := false
		for _, prefix := range allowed {
			if candidate == prefix || strings.HasPrefix(candidate, prefix+"/") {
				permitted = true
				break
			}
		}
		if !permitted {
			return core.NewError(core.ErrorForbidden, core.ReasonComponentMismatch, "next-turn dispatcher imports a forbidden Owner package")
		}
	}
	return nil
}

type NextModelTurnDispatchCaseV1 struct {
	NewPort     func() applicationports.NextModelTurnDispatchPortV1
	Request     contract.NextModelTurnDispatchRequestV1
	Conflicting contract.NextModelTurnDispatchRequestV1
}

type NextModelTurnDispatchReportV1 struct {
	ExactInspect           bool `json:"exact_inspect"`
	IdempotentReplay       bool `json:"idempotent_replay"`
	ConcurrentSingleWinner bool `json:"concurrent_single_winner"`
	ChangedPayloadRejected bool `json:"changed_payload_rejected"`
	AttemptBoundOnly       bool `json:"attempt_bound_only"`
	ProductionEligible     bool `json:"production_eligible"`
	OutcomeBindingEligible bool `json:"outcome_binding_eligible"`
}

func CheckNextModelTurnDispatchPortV1(
	ctx context.Context,
	tc NextModelTurnDispatchCaseV1,
) (NextModelTurnDispatchReportV1, error) {
	if ctx == nil || ctx.Err() != nil {
		return NextModelTurnDispatchReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "next-turn dispatcher conformance context is unavailable")
	}
	if tc.NewPort == nil || tc.Request.Validate() != nil || tc.Conflicting.Validate() != nil {
		return NextModelTurnDispatchReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "next-turn dispatcher conformance fixture is incomplete")
	}
	if tc.Request.EligibilityProjection.DerivedDispatch.ID != tc.Conflicting.EligibilityProjection.DerivedDispatch.ID ||
		tc.Request.RequestDigest == tc.Conflicting.RequestDigest {
		return NextModelTurnDispatchReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidState, "next-turn dispatcher conflict fixture must reuse one derived ID with another payload")
	}
	port := tc.NewPort()
	if nextModelTurnConformanceNilV1(port) {
		return NextModelTurnDispatchReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "next-turn dispatcher factory returned nil")
	}
	first, err := port.StartOrInspectNextModelTurnV1(ctx, tc.Request)
	if err != nil {
		return NextModelTurnDispatchReportV1{}, err
	}
	replayed, err := port.StartOrInspectNextModelTurnV1(ctx, tc.Request)
	if err != nil || replayed != first {
		return NextModelTurnDispatchReportV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "next-turn dispatcher replay was not exact")
	}
	inspect, err := contract.NewNextModelTurnDispatchInspectRequestV1(tc.Request)
	if err != nil {
		return NextModelTurnDispatchReportV1{}, err
	}
	inspected, err := port.InspectNextModelTurnV1(ctx, inspect)
	if err != nil || inspected != first {
		return NextModelTurnDispatchReportV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "next-turn dispatcher Inspect was not exact")
	}
	if _, err = port.StartOrInspectNextModelTurnV1(ctx, tc.Conflicting); !core.HasCategory(err, core.ErrorConflict) {
		return NextModelTurnDispatchReportV1{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "next-turn dispatcher accepted another payload for one derived ID")
	}
	if err = checkNextModelTurnConcurrentV1(ctx, port, tc.Request); err != nil {
		return NextModelTurnDispatchReportV1{}, err
	}
	return NextModelTurnDispatchReportV1{
		ExactInspect:           true,
		IdempotentReplay:       true,
		ConcurrentSingleWinner: true,
		ChangedPayloadRejected: true,
		AttemptBoundOnly:       first.State == contract.NextModelTurnDispatchAttemptBoundV1,
		ProductionEligible:     false,
		OutcomeBindingEligible: false,
	}, nil
}

func checkNextModelTurnConcurrentV1(
	ctx context.Context,
	port applicationports.NextModelTurnDispatchPortV1,
	request contract.NextModelTurnDispatchRequestV1,
) error {
	const workers = 64
	values := make(chan contract.NextModelTurnDispatchCurrentV1, workers)
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			value, err := port.StartOrInspectNextModelTurnV1(ctx, request)
			if err != nil {
				errs <- err
				return
			}
			values <- value
		}()
	}
	group.Wait()
	close(values)
	close(errs)
	for err := range errs {
		return err
	}
	for value := range values {
		if value.RequestDigest != request.RequestDigest ||
			value.State != contract.NextModelTurnDispatchAttemptBoundV1 {
			return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "next-turn dispatcher concurrency did not linearize to one attempt-bound payload")
		}
	}
	return nil
}

func nextModelTurnConformanceNilV1(value any) bool {
	if value == nil {
		return true
	}
	ref := reflect.ValueOf(value)
	switch ref.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return ref.IsNil()
	default:
		return false
	}
}
