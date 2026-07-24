package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
)

type CommittedPendingActionReaderV3 interface {
	InspectCommittedPendingActionCurrentV3(context.Context, contract.CommittedPendingActionCurrentRequestV3) (contract.CommittedPendingActionCurrentV3, error)
}
