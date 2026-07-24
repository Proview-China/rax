package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

type HostCleanupClosureFactPortV2 interface {
	EnsureHostCleanupClosureV2(context.Context, contract.HostCleanupClosureFactV2) (contract.HostCleanupClosureFactV2, error)
	InspectHostCleanupClosureV2(context.Context, contract.HostCleanupClosureRefV2) (contract.HostCleanupClosureFactV2, error)
	InspectHostCleanupClosureForStartV2(context.Context, string, string) (contract.HostCleanupClosureFactV2, error)
}

// HostCleanupPlanTemplateCurrentReaderV2 is a read-only input owned by the
// cleanup orchestration compiler. The Closure store never manufactures it.
type HostCleanupPlanTemplateCurrentReaderV2 interface {
	InspectHostCleanupPlanTemplateCurrentV2(context.Context, contract.ExactRefV1) (contract.HostCleanupPlanTemplateCurrentV2, error)
}

// CleanupNodeOperationV3 is deliberately separated from V2. A V3 endpoint
// receives the exact Closure and typed target; it cannot accept a caller Plan.
// Implementations must call ValidateOwnerDispatchCurrent at the actual point.
type CleanupNodeOperationV3 interface {
	StartOrInspectCleanupNodeV3(context.Context, contract.CleanupNodeDispatchEnvelopeV3) (contract.CleanupNodeResultV2, error)
	InspectCleanupNodeV3(context.Context, contract.CleanupNodeDispatchEnvelopeV3) (contract.CleanupNodeResultV2, error)
}

type CleanupNodeOperationRegistryV3 interface {
	ResolveCleanupNodeOperationV3(context.Context, contract.ExactRefV1, contract.ExactRefV1, contract.DigestV1, contract.DigestV1) (CleanupNodeOperationV3, error)
}
