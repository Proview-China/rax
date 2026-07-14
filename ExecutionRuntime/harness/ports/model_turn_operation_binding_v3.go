package ports

import (
	"context"
	"strings"

	bridgecontract "github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type ModelTurnOperationBindingCASRequestV3 struct {
	Scope            core.ExecutionScope                            `json:"scope"`
	StepKind         runtimeports.NamespacedNameV2                  `json:"step_kind"`
	ID               string                                         `json:"binding_id"`
	ExpectedRevision core.Revision                                  `json:"expected_revision"`
	Next             bridgecontract.ModelTurnOperationBindingFactV3 `json:"next"`
}

func (r ModelTurnOperationBindingCASRequestV3) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if runtimeports.ValidateNamespacedNameV2(r.StepKind) != nil || strings.TrimSpace(r.ID) == "" || r.ExpectedRevision == 0 || r.Next.Revision != r.ExpectedRevision+1 {
		return core.NewError(core.ErrorInvalidArgument, core.ReasonRevisionConflict, "model-turn binding CAS key and revisions are incomplete")
	}
	if err := r.Next.Validate(); err != nil {
		return err
	}
	if r.Next.ID != r.ID || r.Next.StepKind != r.StepKind || !samePortScopeV3(r.Next.Scope, r.Scope) {
		return core.NewError(core.ErrorConflict, core.ReasonInvalidReference, "model-turn binding CAS key differs from next fact")
	}
	return nil
}

// ModelTurnOperationBindingFactPortV3 is the Harness-owned persistent mapping
// from Application attempt ID to Run/Session/Candidate. Implementations must
// enforce the centralized transition validator on every CAS.
type ModelTurnOperationBindingFactPortV3 interface {
	CreateModelTurnOperationBindingV3(context.Context, bridgecontract.ModelTurnOperationBindingFactV3) (bridgecontract.ModelTurnOperationBindingFactV3, error)
	InspectModelTurnOperationBindingV3(context.Context, core.ExecutionScope, runtimeports.NamespacedNameV2, string) (bridgecontract.ModelTurnOperationBindingFactV3, error)
	CompareAndSwapModelTurnOperationBindingV3(context.Context, ModelTurnOperationBindingCASRequestV3) (bridgecontract.ModelTurnOperationBindingFactV3, error)
}

func samePortScopeV3(left, right core.ExecutionScope) bool {
	ld, le := runtimeports.ExecutionScopeDigestV2(left)
	rd, re := runtimeports.ExecutionScopeDigestV2(right)
	return le == nil && re == nil && ld == rd
}
