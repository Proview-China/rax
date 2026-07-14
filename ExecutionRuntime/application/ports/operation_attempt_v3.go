package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type GovernedOperationAttemptCASRequestV3 struct {
	Scope            core.ExecutionScope                     `json:"scope"`
	ID               string                                  `json:"id"`
	ExpectedRevision core.Revision                           `json:"expected_revision"`
	Next             contract.GovernedOperationAttemptFactV3 `json:"next"`
}

// GovernedOperationAttemptFactPortV3 persists Application recovery watermarks.
// It does not implement any Runtime Fact Owner or grant dispatch/settlement.
type GovernedOperationAttemptFactPortV3 interface {
	CreateGovernedOperationAttemptV3(context.Context, contract.GovernedOperationAttemptFactV3) (contract.GovernedOperationAttemptFactV3, error)
	InspectGovernedOperationAttemptV3(context.Context, core.ExecutionScope, string) (contract.GovernedOperationAttemptFactV3, error)
	CompareAndSwapGovernedOperationAttemptV3(context.Context, GovernedOperationAttemptCASRequestV3) (contract.GovernedOperationAttemptFactV3, error)
}
