package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
)

// CommittedPendingActionReaderV2 is the Harness-owned exact current reader.
// It exposes no Session, Fact, Evidence, Settlement, or dispatch write.
type CommittedPendingActionReaderV2 interface {
	InspectCommittedPendingActionCurrentV2(context.Context, contract.CommittedPendingActionCurrentRequestV2) (contract.CommittedPendingActionCurrentV2, error)
}
