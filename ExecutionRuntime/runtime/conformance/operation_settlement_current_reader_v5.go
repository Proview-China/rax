package conformance

import (
	"context"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationSettlementCurrentReaderCaseV5 exercises only the capability-
// narrowed current read surface. Production composition must inject the
// Runtime Kernel Gateway behind Reader; a raw Fact Port is not certified here.
type OperationSettlementCurrentReaderCaseV5 struct {
	Provider ports.OperationSettlementCurrentReaderProviderV5
	Request  ports.InspectCurrentOperationCheckpointRestoreSettlementRequestV5
}

type OperationSettlementCurrentReaderReportV5 struct {
	CurrentInspectObserved  bool `json:"current_inspect_observed"`
	SettleAuthorityUsed     bool `json:"settle_authority_used"`
	ProductionClaimEligible bool `json:"production_claim_eligible"`
}

func CheckOperationSettlementCurrentReaderV5(ctx context.Context, testCase OperationSettlementCurrentReaderCaseV5) (OperationSettlementCurrentReaderReportV5, error) {
	if operationSettlementCurrentReaderProviderNilV5(testCase.Provider) {
		return OperationSettlementCurrentReaderReportV5{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Operation Settlement V5 current Reader provider is required")
	}
	if err := testCase.Request.Validate(); err != nil {
		return OperationSettlementCurrentReaderReportV5{}, err
	}
	inspection, err := testCase.Provider.InspectCheckpointPhaseSettlementCurrentV5(ctx, testCase.Request)
	if err != nil {
		return OperationSettlementCurrentReaderReportV5{}, err
	}
	if err := inspection.Validate(); err != nil {
		return OperationSettlementCurrentReaderReportV5{}, err
	}
	if !ports.SameOperationSubjectV3(inspection.Bundle.Submission.Operation, testCase.Request.Operation) || inspection.Bundle.Submission.EffectID != testCase.Request.EffectID || inspection.Bundle.Settlement.EffectID != testCase.Request.EffectID {
		return OperationSettlementCurrentReaderReportV5{}, core.NewError(core.ErrorConflict, core.ReasonCheckpointInconsistent, "Operation Settlement V5 current Reader returned another closure")
	}
	return OperationSettlementCurrentReaderReportV5{
		CurrentInspectObserved:  true,
		SettleAuthorityUsed:     false,
		ProductionClaimEligible: false,
	}, nil
}

func operationSettlementCurrentReaderProviderNilV5(provider ports.OperationSettlementCurrentReaderProviderV5) bool {
	if provider == nil {
		return true
	}
	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
