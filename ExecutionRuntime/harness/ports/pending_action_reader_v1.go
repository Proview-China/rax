package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
)

// CommittedPendingActionReaderV1 is the Application-facing, read-only Harness
// seam for one exact waiting_action Session revision. It exposes no Fact Store
// or mutation capability.
type CommittedPendingActionReaderV1 interface {
	InspectCommittedPendingActionCurrentV1(context.Context, contract.InspectCommittedPendingActionCurrentRequestV1) (contract.CommittedPendingActionCurrentV1, error)
}
