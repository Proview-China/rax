package ports

import (
	"context"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type InspectSingleCallToolActionRequestV1 struct {
	RequestID     string      `json:"request_id"`
	RequestDigest core.Digest `json:"request_digest"`
	ScopeDigest   core.Digest `json:"scope_digest"`
}

func (r InspectSingleCallToolActionRequestV1) Validate() error {
	if strings.TrimSpace(r.RequestID) == "" || len(r.RequestID) > contract.MaxSingleCallCoordinateIDBytesV1 || r.RequestDigest.Validate() != nil || r.ScopeDigest.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call Tool Inspect key is incomplete")
	}
	return nil
}

type SingleCallToolActionPortV1 interface {
	ExecuteSingleCallToolActionV1(context.Context, contract.SingleCallToolActionRequestV1) (contract.SingleCallToolActionResultV1, error)
	InspectSingleCallToolActionV1(context.Context, InspectSingleCallToolActionRequestV1) (contract.SingleCallToolActionResultV1, error)
}

type SingleCallToolActionInputCurrentReaderV1 interface {
	InspectSingleCallToolActionInputCurrentV1(context.Context, contract.SingleCallToolActionRequestV1) (contract.SingleCallToolActionInputCurrentProjectionV1, error)
}

// SingleCallOperationSettlementCurrentReaderV1 is deliberately narrower than
// Runtime's governance and Fact ports. It cannot settle or commit anything.
type SingleCallOperationSettlementCurrentReaderV1 interface {
	InspectCurrentOperationSettlementV4(context.Context, runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error)
	InspectOperationSettlementEvidenceAssociationV4(context.Context, runtimeports.OperationSubjectV3, runtimeports.OperationSettlementEvidenceAssociationRefV4) (runtimeports.OperationSettlementEvidenceAssociationV4, error)
}

type SingleCallToolActionCoordinationCASRequestV1 struct {
	Scope            core.ExecutionScope                             `json:"scope"`
	ID               string                                          `json:"id"`
	ExpectedRevision core.Revision                                   `json:"expected_revision"`
	Next             contract.SingleCallToolActionCoordinationFactV1 `json:"next"`
}

type SingleCallToolActionCoordinationFactPortV1 interface {
	CreateSingleCallToolActionCoordinationV1(context.Context, contract.SingleCallToolActionCoordinationFactV1) (contract.SingleCallToolActionCoordinationFactV1, error)
	InspectSingleCallToolActionCoordinationV1(context.Context, core.ExecutionScope, string) (contract.SingleCallToolActionCoordinationFactV1, error)
	CompareAndSwapSingleCallToolActionCoordinationV1(context.Context, SingleCallToolActionCoordinationCASRequestV1) (contract.SingleCallToolActionCoordinationFactV1, error)
}
