package action

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

// FactStoreV2 is the Tool Owner's context-aware durable fact seam. It owns
// only Tool facts; Runtime settlement and Application result facts are read
// through their respective owners.
type FactStoreV2 interface {
	CreateCandidateFactV2(context.Context, contract.ActionCandidateV2) (RecordV2, error)
	InspectCandidateCurrentV2(context.Context, contract.ObjectRef, time.Time) (contract.ActionCandidateV2, error)
	CreateReservationFactV2(context.Context, contract.ObjectRef, contract.ApplicationAttemptRefV1, core.Digest, string, core.Digest, time.Time, time.Time) (contract.ActionReservationFactV2, error)
	InspectReservationExactV2(context.Context, string, contract.ObjectRef) (contract.ActionReservationFactV2, error)
	CreateDomainResultFactV2(context.Context, contract.ToolDomainResultFactV2) (contract.ToolDomainResultFactV2, error)
	InspectDomainResultExactV2(context.Context, string, contract.ObjectRef) (contract.ToolDomainResultFactV2, error)
	InspectDomainResultCurrentByExactV1(context.Context, contract.ObjectRef, time.Time, time.Duration) (contract.ToolDomainResultCurrentProjectionV1, error)
	ApplySettlementAndCreateResultV2(context.Context, string, contract.ObjectRef, runtimeports.OperationInspectionSettlementRefV4, contract.ToolOutcomeV2, contract.ToolDispositionV2, time.Time) (contract.ToolResultV2, error)
	InspectResultExactV2(context.Context, string, contract.ObjectRef) (contract.ToolResultV2, error)
	InspectSettledResultForApplyV2(context.Context, string, contract.ObjectRef) (contract.ToolResultV2, error)
}
