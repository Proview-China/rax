package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// ControlAdapterFactoryRegistryV2 is the sealed process-local registry. It
// resolves only an exact public factory descriptor; it never discovers or
// opens a resource.
type ControlAdapterFactoryRegistryV2 interface {
	ResolveControlAdapterFactoryV2(contract.ControlAdapterFactoryRefV2) (ControlAdapterFactoryV2, error)
}

// ControlAdapterConstructionGatewayV2 is the only Host-facing construction
// seam. Callers cannot bypass conformance/resource S1/S2 by invoking a factory.
type ControlAdapterConstructionGatewayV2 interface {
	StartOrInspectControlAdapterConstructionV2(context.Context, contract.ControlAdapterConstructRequestV2) (contract.ControlAdapterInstanceV2, error)
	InspectControlAdapterConstructionV2(context.Context, contract.ControlAdapterConstructRequestV2) (contract.ControlAdapterInstanceV2, error)
}

// Compile-time documentation of the Runtime current dependency used by the
// gateway; the semantic owner remains Runtime/resource owners.
type ControlAdapterResourceCurrentReaderV2 interface {
	runtimeports.ResourceCurrentReaderV1
}
