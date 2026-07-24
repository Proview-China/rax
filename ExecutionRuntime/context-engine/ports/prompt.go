package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type ContextPromptAssetStoreV1 interface {
	PutPromptAssetV1(context.Context, contract.PromptAssetV1) (contract.PromptAssetRefV1, error)
	InspectPromptAssetV1(context.Context, contract.PromptAssetRefV1) (contract.PromptAssetV1, error)
}

type ContextPromptLifecycleStoreV1 interface {
	ContextPromptAssetStoreV1
	CreatePromptDraftV1(context.Context, contract.ContextPromptLifecycleFactV1) (contract.FactRef, error)
	CompareAndSwapPromptLifecycleV1(context.Context, contract.FactRef, contract.ContextPromptLifecycleFactV1) (contract.FactRef, error)
	InspectPromptLifecycleV1(context.Context, contract.FactRef) (contract.ContextPromptLifecycleFactV1, error)
	InspectPromptLifecycleHeadV1(context.Context, contract.PromptAssetRefV1) (contract.ContextPromptLifecycleHeadV1, error)
}
