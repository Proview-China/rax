package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type ContextFrameExactCurrentReaderV1 interface {
	InspectContextFrameExactCurrentV1(context.Context, contract.FactRef, int64) (contract.ContextFrameExactCurrentProjectionV1, error)
}

type ContextModelInputLineageCurrentReaderV1 interface {
	InspectContextModelInputLineageCurrentV1(context.Context, contract.ContextModelInputLineageCurrentRequestV1) (contract.ContextModelInputLineageCurrentProjectionV1, error)
}
