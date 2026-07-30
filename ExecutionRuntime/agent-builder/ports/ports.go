package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
)

type AgentPackageExactReaderV1 interface {
	InspectExactAgentPackageV1(context.Context, contract.AgentPackageRefV1) (contract.AgentPackageV1, error)
}

type AgentPackageRepositoryV1 interface {
	AgentPackageExactReaderV1
	EnsureExactAgentPackageV1(context.Context, contract.AgentPackageV1) (contract.AgentPackageV1, error)
}

// HarnessAssemblyPublicationHistoricalReaderV2 is structurally compatible
// with Harness assemblypublication.HistoricalReaderV2 without importing its
// Owner store implementation package.
type HarnessAssemblyPublicationHistoricalReaderV2 interface {
	InspectAssemblyPublicationHistoricalV2(context.Context, assemblycontract.AssemblyPublicationRefV2) (assemblycontract.AssemblyPublicationBundleV2, error)
}
