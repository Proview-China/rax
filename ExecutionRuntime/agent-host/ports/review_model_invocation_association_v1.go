package ports

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

type ReviewModelInvocationAssociationCreateReceiptV1 struct {
	Fact    contract.ReviewModelInvocationAssociationFactV1 `json:"fact"`
	Created bool                                            `json:"created"`
}
type ReviewModelInvocationAssociationCASRequestV1 struct {
	Expected contract.ReviewModelInvocationAssociationRefV1  `json:"expected"`
	Next     contract.ReviewModelInvocationAssociationFactV1 `json:"next"`
}

func (r ReviewModelInvocationAssociationCASRequestV1) Validate() error {
	if err := r.Expected.Validate(); err != nil {
		return err
	}
	if err := r.Next.ValidateHistoricalV1(); err != nil {
		return err
	}
	if r.Next.ID != r.Expected.ID || r.Next.Subject != r.Expected.Subject || r.Next.Revision != r.Expected.Revision+1 || r.Next.PreviousDigest != r.Expected.Digest {
		return contract.NewError(contract.ErrorConflict, "association_cas_drift", "association CAS coordinates drifted")
	}
	return nil
}

type ReviewModelInvocationAssociationCASReceiptV1 struct {
	Fact    contract.ReviewModelInvocationAssociationFactV1 `json:"fact"`
	Applied bool                                            `json:"applied"`
}

type ReviewModelInvocationAssociationPortV1 interface {
	CreateReviewModelInvocationAssociationV1(context.Context, contract.ReviewModelInvocationAssociationFactV1) (ReviewModelInvocationAssociationCreateReceiptV1, error)
	ResolveCurrentReviewModelInvocationAssociationV1(context.Context, contract.ReviewModelInvocationAssociationSubjectV1) (contract.ReviewModelInvocationAssociationRefV1, error)
	InspectCurrentReviewModelInvocationAssociationV1(context.Context, contract.ReviewModelInvocationAssociationSubjectV1, contract.ReviewModelInvocationAssociationRefV1) (contract.ReviewModelInvocationAssociationFactV1, error)
	InspectHistoricalReviewModelInvocationAssociationV1(context.Context, contract.ReviewModelInvocationAssociationRefV1) (contract.ReviewModelInvocationAssociationFactV1, error)
	CompareAndSwapReviewModelInvocationAssociationV1(context.Context, ReviewModelInvocationAssociationCASRequestV1) (ReviewModelInvocationAssociationCASReceiptV1, error)
}
