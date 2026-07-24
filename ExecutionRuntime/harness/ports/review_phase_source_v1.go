package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
)

// ReviewPhaseSourceCurrentReaderV1 is read-only. It cannot create a Review
// Case, mutate a Harness Session, or grant Runtime execution authority.
type ReviewPhaseSourceCurrentReaderV1 interface {
	InspectReviewPhaseSourceCurrentV1(context.Context, contract.ReviewPhaseSourceCurrentRequestV1) (contract.ReviewPhaseSourceCurrentProjectionV1, error)
}
