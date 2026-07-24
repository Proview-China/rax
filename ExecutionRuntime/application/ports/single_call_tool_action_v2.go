package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// SingleCallModelPendingActionIdentityCurrentReaderV2 is the Application-owned
// neutral read seam. Owner adapters must project only already-validated facts;
// this interface exposes no repository write capability.
type SingleCallModelPendingActionIdentityCurrentReaderV2 interface {
	InspectSingleCallModelPendingActionIdentityCurrentV2(context.Context, contract.SingleCallModelPendingActionIdentityCurrentRequestV2) (contract.SingleCallModelPendingActionIdentityCurrentV2, error)
}

// SingleCallToolActionInputCurrentReaderV2 returns the exact Harness aggregate
// and Runtime Authority projection used by both S1 and S2.
type SingleCallToolActionInputCurrentReaderV2 interface {
	InspectSingleCallToolActionInputCurrentV2(context.Context, contract.SingleCallToolActionRequestV2) (contract.SingleCallToolActionInputCurrentProjectionV2, error)
}

// SingleCallToolActionPortV2 is start-or-inspect. Repeating Execute with the
// same canonical request must not imply repeating a Provider side effect.
type SingleCallToolActionPortV2 interface {
	ExecuteSingleCallToolActionV2(context.Context, contract.SingleCallToolActionRequestV2) (contract.SingleCallToolActionResultV2, error)
	InspectSingleCallToolActionV2(context.Context, contract.SingleCallToolActionInspectKeyV2) (contract.SingleCallToolActionResultV2, error)
}

// SingleCallOperationSettlementCurrentReaderV2 deliberately excludes every
// Runtime settlement/Fact commit method.
type SingleCallOperationSettlementCurrentReaderV2 interface {
	InspectCurrentOperationSettlementV4(context.Context, runtimeports.InspectCurrentOperationSettlementRequestV4) (runtimeports.OperationInspectionSettlementRefV4, error)
	InspectOperationSettlementEvidenceAssociationV4(context.Context, runtimeports.OperationSubjectV3, runtimeports.OperationSettlementEvidenceAssociationRefV4) (runtimeports.OperationSettlementEvidenceAssociationV4, error)
}

type SingleCallToolActionCoordinationCASRequestV2 struct {
	ContractVersion  string                                          `json:"contract_version"`
	Scope            core.ExecutionScope                             `json:"scope"`
	ID               string                                          `json:"id"`
	ExpectedRevision core.Revision                                   `json:"expected_revision"`
	ExpectedDigest   core.Digest                                     `json:"expected_digest"`
	Next             contract.SingleCallToolActionCoordinationFactV2 `json:"next"`
	Digest           core.Digest                                     `json:"digest"`
}

func SealSingleCallToolActionCoordinationCASRequestV2(request SingleCallToolActionCoordinationCASRequestV2) (SingleCallToolActionCoordinationCASRequestV2, error) {
	request.ContractVersion = contract.SingleCallToolActionCoordinationVersionV2
	request.Digest = ""
	digest, err := request.DigestV2()
	if err != nil {
		return SingleCallToolActionCoordinationCASRequestV2{}, err
	}
	request.Digest = digest
	return request, request.Validate()
}

func (request SingleCallToolActionCoordinationCASRequestV2) DigestV2() (core.Digest, error) {
	copy := request
	copy.Digest = ""
	return core.CanonicalJSONDigest("praxis.application.single-call-coordination-v2", "2.0.0", "SingleCallToolActionCoordinationCASRequestV2", copy)
}

func (request SingleCallToolActionCoordinationCASRequestV2) Validate() error {
	if request.ContractVersion != contract.SingleCallToolActionCoordinationVersionV2 || request.Scope.Validate() != nil || request.ID == "" || request.ExpectedRevision == 0 || request.ExpectedDigest.Validate() != nil || request.Next.Validate() != nil {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "single-call V2 coordination CAS request is incomplete")
	}
	if request.ID != request.Next.ID || request.Next.Revision != request.ExpectedRevision+1 || !runtimeports.SameExecutionScopeV2(request.Scope, request.Next.Request.Action.ExecutionScope) {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "single-call V2 coordination CAS key or revision drifted")
	}
	digest, err := request.DigestV2()
	if err != nil || digest != request.Digest {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "single-call V2 coordination CAS digest drifted")
	}
	return nil
}

type SingleCallToolActionCoordinationFactPortV2 interface {
	CreateSingleCallToolActionCoordinationV2(context.Context, contract.SingleCallToolActionCoordinationFactV2) (contract.SingleCallToolActionCoordinationFactV2, error)
	InspectSingleCallToolActionCoordinationV2(context.Context, core.ExecutionScope, string) (contract.SingleCallToolActionCoordinationFactV2, error)
	CompareAndSwapSingleCallToolActionCoordinationV2(context.Context, SingleCallToolActionCoordinationCASRequestV2) (contract.SingleCallToolActionCoordinationFactV2, error)
}
