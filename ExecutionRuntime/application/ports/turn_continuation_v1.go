package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
)

// TurnContinuationPortV1 is implemented by the Harness owner. Begin must
// atomically publish continuation_pending before any Context refresh. Commit
// must atomically CAS Pending.ActiveContext to the derived target ContextFrame
// and publish context_current in one Harness-local transaction. These calls do
// not form a distributed transaction with Tool or Context owners.
//
// If either mutating call has an unknown or lost reply, callers may only call
// InspectTurnContinuationV1 with the original AttemptRef. They must not create
// or redispatch another Attempt. A next Model Turn is forbidden until the
// inspected current passes ModelTurnAllowedV1.
type TurnContinuationPortV1 interface {
	BeginTurnContinuationV1(context.Context, contract.TurnContinuationStartRequestV1) (contract.TurnContinuationCurrentV1, error)
	CommitTurnContinuationV1(context.Context, contract.TurnContinuationCommitRequestV1) (contract.TurnContinuationCurrentV1, error)
	InspectTurnContinuationV1(context.Context, contract.TurnContinuationInspectRequestV1) (contract.TurnContinuationCurrentV1, error)
}
