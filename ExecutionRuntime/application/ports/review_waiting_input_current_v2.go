package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// ReviewWaitingInputExactCurrentReaderV2 is read-only and accepts only the
// full exact request. Closed errors are InvalidArgument (malformed request),
// NotFound (exact source absent), Conflict (type-pun/drift/ABA), PreconditionFailed
// (clock/TTL), CapabilityUnavailable (unsupported closed kind), Unavailable,
// and Indeterminate. A lost read reply may only repeat this same exact request.
type ReviewWaitingInputExactCurrentReaderV2 interface {
	InspectReviewWaitingInputExactCurrentV2(context.Context, contract.ReviewWaitingInputCurrentRequestV2) (contract.ReviewWaitingInputCurrentProjectionV2, error)
}

func IsReviewWaitingInputExactCurrentClosedErrorV2(err error) bool {
	for _, category := range []core.ErrorCategory{
		core.ErrorInvalidArgument,
		core.ErrorNotFound,
		core.ErrorConflict,
		core.ErrorPreconditionFailed,
		core.ErrorCapabilityUnavailable,
		core.ErrorUnavailable,
		core.ErrorIndeterminate,
	} {
		if core.HasCategory(err, category) {
			return true
		}
	}
	return false
}
