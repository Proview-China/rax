package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// HumanMultiSignExactReaderV2 is the minimum Review-owned read surface needed
// by the Human Multi-Sign external-current aggregator. StoreV2
// satisfies it, but the aggregator is deliberately denied every mutation.
type HumanMultiSignExactReaderV2 interface {
	InspectTargetExactV1(context.Context, core.TenantID, ExactFactRefV1) (contract.TargetSnapshotV1, error)
	ListHumanPanelAssignmentsV2(context.Context, contract.HumanPanelExactRefV2) ([]contract.HumanPanelAssignmentV2, error)
}

// HumanOrganizationCurrentRequestResolverV2 resolves immutable Organization
// lookup coordinates for an exact Review Panel/Assignment set. The returned
// values are only coordinates for HumanOrganizationCurrentReaderV2; they are
// not current facts and grant no Identity, Authority or Delegation.
//
// A lost/unknown reply is recovered only by repeating this exact read with the
// same sealed Panel and Assignment set. Implementations must return deep
// clones and must not expose registration or mutation through this interface.
type HumanOrganizationCurrentRequestResolverV2 interface {
	InspectHumanOrganizationCurrentRequestsV2(context.Context, contract.HumanReviewPanelV2, []contract.HumanPanelAssignmentV2) ([]HumanOrganizationCurrentRequestV2, error)
}
