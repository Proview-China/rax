package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// ReviewWaitingInputCurrentReaderV1 is implemented by the trusted host/Harness
// adapter. It returns only a sealed neutral projection and owns no Review Fact.
type ReviewWaitingInputCurrentReaderV1 interface {
	InspectReviewWaitingInputCurrentV1(context.Context, contract.ReviewWaitingInputSubjectV1) (contract.ReviewWaitingInputCurrentProjectionV1, error)
}

// ReviewStartOrInspectPortV1 is implemented by the Review Owner adapter. The
// first method may create/start one canonical Case; the second is read-only and
// must never create a replacement Case. Neither result is a Runtime authority.
type ReviewStartOrInspectPortV1 interface {
	StartOrInspectReviewV1(context.Context, contract.ReviewWaitingRequestV1) (contract.ReviewWaitingCurrentProjectionV1, error)
	InspectReviewV1(context.Context, contract.ReviewWaitingInspectRequestV1) (contract.ReviewWaitingCurrentProjectionV1, error)
}

type ReviewWaitingCoordinationCreateReceiptV1 struct {
	Fact    contract.ReviewWaitingCoordinationFactV1 `json:"fact"`
	Created bool                                     `json:"created"`
}

type ReviewWaitingCoordinationCASRequestV1 struct {
	Scope    core.ExecutionScope                      `json:"scope"`
	Expected contract.ReviewWaitingCoordinationRefV1  `json:"expected"`
	Next     contract.ReviewWaitingCoordinationFactV1 `json:"next"`
}

func (r ReviewWaitingCoordinationCASRequestV1) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if err := r.Expected.Validate(); err != nil {
		return err
	}
	if err := r.Next.Validate(); err != nil {
		return err
	}
	if r.Expected.ID != r.Next.ID || r.Next.Revision != r.Expected.Revision+1 || r.Next.PreviousDigest != r.Expected.Digest || !sameReviewWaitingScopeV1(r.Scope, r.Next.Request.ExecutionScope) {
		return core.NewError(core.ErrorConflict, core.ReasonRevisionConflict, "Review waiting CAS coordinates drifted")
	}
	return nil
}

type ReviewWaitingCoordinationCASReceiptV1 struct {
	Fact    contract.ReviewWaitingCoordinationFactV1 `json:"fact"`
	Applied bool                                     `json:"applied"`
}

// ReviewWaitingCoordinationFactPortV1 is Application-owned. Current Inspect is
// linearizable and ErrorNotFound is authoritative for that committed snapshot.
// Historical Inspect uses only the exact Ref and never consults current.
type ReviewWaitingCoordinationFactPortV1 interface {
	CreateReviewWaitingCoordinationV1(context.Context, contract.ReviewWaitingCoordinationFactV1) (ReviewWaitingCoordinationCreateReceiptV1, error)
	InspectCurrentReviewWaitingCoordinationV1(context.Context, core.ExecutionScope, string) (contract.ReviewWaitingCoordinationFactV1, error)
	InspectHistoricalReviewWaitingCoordinationV1(context.Context, core.ExecutionScope, contract.ReviewWaitingCoordinationRefV1) (contract.ReviewWaitingCoordinationFactV1, error)
	CompareAndSwapReviewWaitingCoordinationV1(context.Context, ReviewWaitingCoordinationCASRequestV1) (ReviewWaitingCoordinationCASReceiptV1, error)
}

func sameReviewWaitingScopeV1(left, right core.ExecutionScope) bool {
	if left.SandboxLease == nil || right.SandboxLease == nil {
		return left.SandboxLease == nil && right.SandboxLease == nil && left == right
	}
	leftLease, rightLease := *left.SandboxLease, *right.SandboxLease
	left.SandboxLease, right.SandboxLease = nil, nil
	return left == right && leftLease == rightLease
}
