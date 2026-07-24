package ports

import (
	"context"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// RestoreContextMaterializationPortV1 is implemented by the Context Owner.
// Application coordinates exact refs but cannot create or mutate Context facts.
type RestoreContextMaterializationPortV1 interface {
	MaterializeRestoreContextV1(context.Context, applicationcontract.RestoreContextMaterializationRequestV1) (runtimeports.RestoreContextMaterializationCurrentProjectionV1, error)
	InspectRestoreContextMaterializationV1(context.Context, runtimeports.RestoreContextMaterializationRefV1) (runtimeports.RestoreContextMaterializationCurrentProjectionV1, error)
}
