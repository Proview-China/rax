package release

import (
	"context"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

// ProductionReadinessReaderV1 is implemented by the host composition layer.
// Sandbox never accepts a caller-supplied readiness projection directly.
type ProductionReadinessReaderV1 interface {
	InspectSandboxProductionReadinessV1(context.Context, string, core.Revision) (SandboxProductionReadinessProjectionV1, error)
}

// ComponentReleaseCatalogPortV1 is the Assembler-owned exact catalog seam.
// Ensure and Inspect are both required so a lost Ensure reply is recovered by
// exact inspection instead of publishing a new release identity.
type ComponentReleaseCatalogPortV1 interface {
	EnsureExactComponentReleaseV1(context.Context, assemblercontract.ComponentReleaseV1) (assemblercontract.ComponentReleaseV1, error)
	InspectExactComponentReleaseV1(context.Context, assemblercontract.ComponentReleaseRefV1) (assemblercontract.ComponentReleaseV1, error)
}
