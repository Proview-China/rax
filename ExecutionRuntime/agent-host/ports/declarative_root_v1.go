package ports

import (
	"context"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	definitionports "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
)

// AgentDefinitionPublicationOwnerV1 is the only Definition mutation
// capability accepted by the declarative root. Validation uses the pure
// decoder and never receives this capability.
type AgentDefinitionPublicationOwnerV1 interface {
	CreateSourceV1(context.Context, definitioncontract.AgentDefinitionSourceV1) (definitionports.CreateDefinitionResultV1, error)
}

// DefinitionSourcePublicationOwnerV1 owns the Host-local stable source alias.
// It derives revision, checked time and digest; the root cannot self-sign a
// DefinitionSourceCurrent projection.
type DefinitionSourcePublicationOwnerV1 interface {
	EnsureDefinitionSourceCurrentV1(context.Context, string, definitioncontract.AgentDefinitionRefV1, int64) (contract.DefinitionSourceCurrentV1, error)
}

// DeclarativeAssemblyOperationV1 is the explicit, configuration-only plan
// effect. It is separate from Host lifecycle Claim and has no Provider surface.
type DeclarativeAssemblyOperationV1 interface {
	StartOrInspectDeclarativeAssemblyV1(context.Context, contract.HostConfigV1, contract.DefinitionSourceCurrentV1) (assemblercontract.ResolvedAgentPlanV1, error)
	InspectDeclarativeAssemblyV1(context.Context, contract.HostConfigV1, contract.DefinitionSourceCurrentV1) (assemblercontract.ResolvedAgentPlanV1, error)
}
