package ports

import (
	"context"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type ContextTurnRefreshStoreV1 interface {
	ReserveContextTurnRefreshV1(context.Context, contract.ContextTurnRefreshPendingRecordV1) (contract.ContextTurnRefreshPreparedV1, error)
	LoadContextTurnRefreshPendingRecordV1(context.Context, contract.FactRef) (contract.ContextTurnRefreshPendingRecordV1, error)
	ApplyContextTurnRefreshCurrentCASV1(context.Context, contract.ContextTurnRefreshCommitV1) (contract.ContextTurnRefreshResultV1, error)
	InspectContextTurnRefreshV1(context.Context, contract.InspectContextTurnRefreshRequestV1) (contract.ContextTurnRefreshResultV1, error)
}

// ContextTurnRefreshOwnerBackendV1 is one Context Owner authority and lock
// domain. Implementations must serve S2 exact-current reads and Apply CAS from
// the same state; split reader/store injection is intentionally impossible.
type ContextTurnRefreshOwnerBackendV1 interface {
	ContextTurnRefreshStoreV1
	ContextParentFrameSourceBindingReaderV1
	ContextFrameMetadataReaderV1
	ContextManifestMetadataReaderV1
	ContextGenerationMetadataReaderV1
	ContextGenerationCurrentPointerReaderV1
}

type ContextTurnTransitionProofStoreV1 interface {
	ReserveContextTurnTransitionRequestV1(context.Context, contract.ContextTurnTransitionRequestV1) error
	InspectContextTurnTransitionRequestByApplicationAttemptV1(context.Context, contract.FactRef) (contract.ContextTurnTransitionRequestV1, error)
	EnsureContextTurnTransitionProofV1(context.Context, contract.ContextTurnTransitionProofCurrentV1) (contract.ContextTurnTransitionProofCurrentV1, error)
	InspectContextTurnTransitionProofV1(context.Context, contract.FactRef) (contract.ContextTurnTransitionProofCurrentV1, error)
}
