package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
)

// NextModelTurnDispatchPortV1 binds or inspects only the Application-neutral
// derived dispatch coordinate. It has no Model, Runtime guard, Provider,
// Harness-private dispatch, or Console method.
type NextModelTurnDispatchPortV1 interface {
	StartOrInspectNextModelTurnV1(
		context.Context,
		contract.NextModelTurnDispatchRequestV1,
	) (contract.NextModelTurnDispatchCurrentV1, error)
	InspectNextModelTurnV1(
		context.Context,
		contract.NextModelTurnDispatchInspectRequestV1,
	) (contract.NextModelTurnDispatchCurrentV1, error)
}
