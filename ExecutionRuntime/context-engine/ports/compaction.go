package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type ContextCompactionOwnerBackendV1 interface {
	ContextGenerationCurrentPointerReaderV1
	ReserveContextCompactionV1(context.Context, contract.ContextCompactionPendingRecordV1) (contract.ContextCompactionPreparedV1, error)
	LoadContextCompactionPendingV1(context.Context, contract.FactRef) (contract.ContextCompactionPendingRecordV1, error)
	ApplyContextCompactionCurrentCASV1(context.Context, contract.ApplyContextCompactionRequestV1) (contract.ContextCompactionResultV1, error)
	InspectContextCompactionV1(context.Context, contract.InspectContextCompactionRequestV1) (contract.ContextCompactionResultV1, error)
}
