package conformance

import (
	"context"
	"reflect"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ModelDispatchControlCurrentReaderFixtureV1 struct {
	Operation                    ports.OperationSubjectV3
	EffectID                     core.EffectIntentID
	CheckedUnixNano              int64
	ExpectedState                control.ModelDispatchControlStateV1
	ExpectedRunRevision          core.Revision
	ExpectedDesiredStateRevision core.Revision
	ExpectedLastCommandID        string
}

type ModelDispatchControlCurrentReaderReportV1 struct {
	CurrentProjectionExact bool `json:"current_projection_exact"`
	RepeatedReadExact      bool `json:"repeated_read_exact"`
	ReadOnlyCapability     bool `json:"read_only_capability"`
	ProductionEligible     bool `json:"production_eligible"`
}

// CheckModelDispatchControlCurrentReaderV1 exercises only the owner-local
// read-only contract. It cannot establish production eligibility while no
// production Run/Command persistence composition is bound.
func CheckModelDispatchControlCurrentReaderV1(
	ctx context.Context,
	reader control.ModelDispatchControlCurrentReaderV1,
	fixture ModelDispatchControlCurrentReaderFixtureV1,
) (ModelDispatchControlCurrentReaderReportV1, error) {
	if conformanceDependencyNilV1(reader) {
		return ModelDispatchControlCurrentReaderReportV1{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Model dispatch-control current reader is missing")
	}
	if err := fixture.Operation.Validate(); err != nil {
		return ModelDispatchControlCurrentReaderReportV1{}, err
	}
	if fixture.EffectID == "" || fixture.CheckedUnixNano <= 0 || fixture.ExpectedRunRevision == 0 || fixture.ExpectedDesiredStateRevision == 0 {
		return ModelDispatchControlCurrentReaderReportV1{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "Model dispatch-control conformance fixture is incomplete")
	}
	first, err := reader.InspectModelDispatchControlCurrentV1(ctx, fixture.Operation, fixture.EffectID)
	if err != nil {
		return ModelDispatchControlCurrentReaderReportV1{}, err
	}
	if err := first.Validate(); err != nil {
		return ModelDispatchControlCurrentReaderReportV1{}, err
	}
	second, err := reader.InspectModelDispatchControlCurrentV1(ctx, fixture.Operation, fixture.EffectID)
	if err != nil {
		return ModelDispatchControlCurrentReaderReportV1{}, err
	}
	if err := second.Validate(); err != nil {
		return ModelDispatchControlCurrentReaderReportV1{}, err
	}
	checkedAt := time.Unix(0, fixture.CheckedUnixNano)
	exact := first.OperationDigest != "" &&
		first.EffectID == fixture.EffectID &&
		first.RunID == fixture.Operation.RunID &&
		first.RunRevision == fixture.ExpectedRunRevision &&
		first.DesiredStateRevision == fixture.ExpectedDesiredStateRevision &&
		first.LastCommandID == fixture.ExpectedLastCommandID &&
		first.State == fixture.ExpectedState &&
		first.CheckedUnixNano <= fixture.CheckedUnixNano &&
		checkedAt.Before(time.Unix(0, first.ExpiresUnixNano))
	readerType := reflect.TypeOf((*control.ModelDispatchControlCurrentReaderV1)(nil)).Elem()
	return ModelDispatchControlCurrentReaderReportV1{
		CurrentProjectionExact: exact,
		RepeatedReadExact: first.OperationDigest == second.OperationDigest &&
			first.EffectID == second.EffectID &&
			first.RunID == second.RunID &&
			first.ExecutionScopeDigest == second.ExecutionScopeDigest &&
			first.RunRevision == second.RunRevision &&
			first.DesiredStateRevision == second.DesiredStateRevision &&
			first.LastCommandID == second.LastCommandID &&
			first.State == second.State &&
			first.WatermarkDigest == second.WatermarkDigest,
		ReadOnlyCapability: readerType.NumMethod() == 1 && readerType.Method(0).Name == "InspectModelDispatchControlCurrentV1",
		ProductionEligible: false,
	}, nil
}
