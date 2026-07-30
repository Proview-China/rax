package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
)

// TurnContinuationCurrentReaderV1 is the read-only subset of the Harness-owned
// TurnContinuation port. Application eligibility inspection cannot Begin or
// Commit a continuation.
type TurnContinuationCurrentReaderV1 interface {
	InspectTurnContinuationV1(context.Context, contract.TurnContinuationInspectRequestV1) (contract.TurnContinuationCurrentV1, error)
}

// NextModelTurnEligibilityPortV1 only inspects an advisory snapshot of the exact
// Continuation current and derives a future dispatch coordinate. A real dispatch
// must fresh-read Harness and Runtime currentness. Runtime actual-point
// inspection is forbidden here: it belongs in the same call stack as the
// physical Provider invocation after the future Model boundary CAS winner and
// Model S3.
type NextModelTurnEligibilityPortV1 interface {
	InspectNextModelTurnEligibilityV1(context.Context, contract.NextModelTurnEligibilityRequestV1) (contract.NextModelTurnEligibilityProjectionV1, error)
}
