package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
)

// ContextOwnerSourceReaderV1 is the Application-owned neutral read seam. A
// Memory or Knowledge adapter validates its own opaque request and returns only
// exact refs plus a bounded body. It cannot create Context facts.
type ContextOwnerSourceReaderV1 interface {
	InspectContextOwnerSourceCurrentV1(context.Context, contract.ContextOwnerSourceRequestV1) (contract.ContextOwnerSourceEnvelopeV1, error)
	ReadContextOwnerContentExactV1(context.Context, contract.ContextOwnerContentRequestV1) (contract.ContextOwnerContentObservationV1, []byte, error)
}

// ContextTurnRefreshPortV1 is the only public three-stage seam. Its
// implementation is Context-owned; Application only coordinates exact refs.
type ContextTurnRefreshPortV1 interface {
	PrepareContextTurnRefreshV1(context.Context, contract.ContextTurnRefreshPrepareRequestV1) (contract.ContextTurnRefreshPreparedV1, error)
	ApplyContextTurnRefreshV1(context.Context, contract.ContextTurnRefreshApplyRequestV1) (contract.ContextTurnRefreshResultV1, error)
	InspectContextTurnRefreshV1(context.Context, contract.ContextTurnRefreshInspectRequestV1) (contract.ContextTurnRefreshResultV1, error)
}
