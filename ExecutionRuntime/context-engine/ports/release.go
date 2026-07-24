package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type ContextRecipeLifecycleStoreV1 interface {
	PutRecipeV1(context.Context, contract.ContextRecipe) (contract.FactRef, error)
	InspectRecipeV1(context.Context, contract.FactRef) (contract.ContextRecipe, error)
	CreateRecipeDraftV1(context.Context, contract.ContextRecipeLifecycleFactV1) (contract.FactRef, error)
	CompareAndSwapRecipeLifecycleV1(context.Context, contract.FactRef, contract.ContextRecipeLifecycleFactV1) (contract.FactRef, error)
	InspectRecipeLifecycleV1(context.Context, contract.FactRef) (contract.ContextRecipeLifecycleFactV1, error)
	InspectRecipeLifecycleHeadV1(context.Context, contract.FactRef) (contract.ContextRecipeLifecycleHeadV1, error)
}
