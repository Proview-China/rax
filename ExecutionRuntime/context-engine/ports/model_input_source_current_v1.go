package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type ContextModelInputSourceCurrentReaderV1 interface {
	InspectContextModelInputSourceCurrentV1(
		context.Context,
		contract.ContextModelInputSourceCurrentRequestV1,
	) (contract.ContextModelInputSourceCurrentProjectionV1, error)
}
