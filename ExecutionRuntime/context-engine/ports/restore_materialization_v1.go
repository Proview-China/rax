package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

type RestoreContextRequirementCurrentReaderV1 interface {
	InspectRestoreContextRequirementsCurrentV1(context.Context, contract.RestoreContextMaterializationRequestV1) (contract.RestoreContextRequirementsCurrentV1, error)
}

type RestoreContextMaterializationStoreV1 interface {
	CommitRestoreContextMaterializationV1(context.Context, contract.RestoreContextMaterializationRequestV1, []contract.RestoredContextFrameV1, contract.RestoredContextGenerationV1, contract.RestoreContextMaterializationFactV1) (contract.RestoreContextMaterializationFactV1, error)
	InspectRestoreContextMaterializationV1(context.Context, contract.FactRef) (contract.RestoreContextMaterializationFactV1, error)
	InspectRestoreContextMaterializationByTargetV1(context.Context, contract.RestoreContextTargetBindingV1) (contract.RestoreContextMaterializationFactV1, error)
	InspectRestoredContextGenerationV1(context.Context, contract.FactRef, contract.Digest) (contract.RestoredContextGenerationV1, error)
	InspectRestoredContextFrameV1(context.Context, contract.FactRef, contract.Digest) (contract.RestoredContextFrameV1, error)
}

type RestoreContextMaterializationOwnerPortV1 interface {
	MaterializeRestoreContextV1(context.Context, contract.RestoreContextMaterializationRequestV1) (contract.RestoreContextMaterializationFactV1, error)
	InspectRestoreContextMaterializationV1(context.Context, contract.FactRef) (contract.RestoreContextMaterializationFactV1, error)
	InspectRestoreContextMaterializationByTargetV1(context.Context, contract.RestoreContextTargetBindingV1) (contract.RestoreContextMaterializationFactV1, error)
}
