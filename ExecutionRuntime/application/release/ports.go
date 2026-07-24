package release

import (
	"context"
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type LocalReadinessReaderV1 interface {
	InspectApplicationLocalReadinessV1(context.Context, string, core.Revision) (LocalReadinessProjectionV1, error)
}
type ProductionReadinessReaderV1 interface {
	InspectApplicationProductionReadinessV1(context.Context, string, core.Revision) (ProductionReadinessProjectionV1, error)
}
type CatalogPortV1 interface {
	EnsureExactComponentReleaseV1(context.Context, assemblercontract.ComponentReleaseV1) (assemblercontract.ComponentReleaseV1, error)
	InspectExactComponentReleaseV1(context.Context, assemblercontract.ComponentReleaseRefV1) (assemblercontract.ComponentReleaseV1, error)
}
