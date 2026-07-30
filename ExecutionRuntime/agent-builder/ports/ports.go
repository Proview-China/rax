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

type AssemblyGenerationArtifactV1 struct {
	Ref   assemblycontract.ObjectRefV1
	Value assemblycontract.AssemblyGenerationV1
}
type AssemblyManifestArtifactV1 struct {
	Ref   assemblycontract.ObjectRefV1
	Value assemblycontract.AssemblyManifestV1
}
type CompiledHarnessGraphArtifactV1 struct {
	Ref   assemblycontract.ObjectRefV1
	Value assemblycontract.CompiledHarnessGraphV1
}
type AssemblyHandoffArtifactV1 struct {
	Ref   assemblycontract.ObjectRefV1
	Value assemblycontract.AssemblyHandoffV1
}

type HarnessArtifactExactReaderV1 interface {
	InspectExactGenerationV1(context.Context, assemblycontract.ObjectRefV1) (AssemblyGenerationArtifactV1, error)
	InspectExactManifestV1(context.Context, assemblycontract.ObjectRefV1) (AssemblyManifestArtifactV1, error)
	InspectExactGraphV1(context.Context, assemblycontract.ObjectRefV1) (CompiledHarnessGraphArtifactV1, error)
	InspectExactHandoffV1(context.Context, assemblycontract.ObjectRefV1) (AssemblyHandoffArtifactV1, error)
}
