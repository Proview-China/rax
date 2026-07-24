package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
)

// CreateBypassDecisionMutationV1 is one Review-owner transaction. Trace is
// optional for test fixtures only; production callers must provide it.
type CreateBypassDecisionMutationV1 struct {
	Decision contract.BypassDecisionV1 `json:"decision"`
	Trace    contract.TraceFactV1      `json:"trace,omitempty"`
}

type BypassDecisionCASMutationV1 struct {
	Expected contract.BypassDecisionExactRefV1 `json:"expected"`
	Next     contract.BypassDecisionV1         `json:"next"`
	Trace    contract.TraceFactV1              `json:"trace,omitempty"`
}

// BypassStoreV1 is a Review-owned append-only history/current-index boundary.
// Historical Inspect depends only on the full exact Ref; current Inspect also
// verifies the Case index. Create/CAS and optional Trace are all-or-nothing.
type BypassStoreV1 interface {
	CreateBypassDecisionV1(context.Context, CreateBypassDecisionMutationV1) (contract.BypassDecisionV1, error)
	InspectBypassDecisionExactV1(context.Context, contract.BypassDecisionExactRefV1) (contract.BypassDecisionV1, error)
	InspectCurrentBypassDecisionByCaseV1(context.Context, contract.BypassCaseExactRefV1) (contract.BypassDecisionV1, error)
	CompareAndSwapBypassDecisionV1(context.Context, BypassDecisionCASMutationV1) (contract.BypassDecisionV1, error)
}
