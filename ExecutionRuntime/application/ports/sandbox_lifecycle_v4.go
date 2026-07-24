package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/application/contract"
)

// SandboxLifecyclePortV4 is start-or-inspect. Application sees only public
// exact Runtime facts and never receives a backend handle or Provider receipt.
type SandboxLifecyclePortV4 interface {
	StartOrInspectSandboxLifecycleV4(context.Context, contract.SandboxLifecycleRequestV4) (contract.SandboxLifecycleResultV4, error)
	InspectSandboxLifecycleV4(context.Context, contract.SandboxLifecycleRequestV4) (contract.SandboxLifecycleResultV4, error)
}
