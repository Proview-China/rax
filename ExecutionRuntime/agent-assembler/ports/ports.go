package ports

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
)

type AgentAssemblerPortV1 interface {
	Resolve(context.Context, contract.ResolveRequestV1) (contract.ResolveResultV1, error)
}

type ResolutionFactsReaderV1 interface {
	InspectExactResolutionFactsV1(context.Context, contract.ResolutionFactsRefV1) (contract.ResolutionFactsSnapshotV1, error)
}

type ComponentReleaseCatalogReaderV1 interface {
	InspectExactComponentReleaseCatalogV1(context.Context, contract.ComponentReleaseCatalogRefV1) (contract.ComponentReleaseCatalogSnapshotV1, error)
}

// ResolvedAgentPlanExactReaderV1 is the read-only capability consumed by
// compilers and composition adapters. It intentionally excludes Ensure/CAS.
type ResolvedAgentPlanExactReaderV1 interface {
	InspectExactResolvedAgentPlanV1(context.Context, contract.ResolvedAgentPlanRefV1) (contract.ResolvedAgentPlanV1, error)
}

type ResolvedAgentPlanRepositoryV1 interface {
	ResolvedAgentPlanExactReaderV1
	EnsureExactResolvedAgentPlanV1(context.Context, contract.ResolvedAgentPlanV1) (contract.ResolvedAgentPlanV1, error)
	InspectCurrentResolvedAgentPlanV1(context.Context, string) (contract.CurrentResolvedPlanV1, error)
	CompareAndSwapCurrentResolvedAgentPlanV1(context.Context, *contract.CurrentResolvedPlanRefV1, contract.CurrentResolvedPlanV1) (contract.CurrentResolvedPlanV1, error)
}

// ComponentReleasePublisherV1 is implemented by component owners. It is a
// public conformance boundary and must never be injected into the resolver.
type ComponentReleasePublisherV1 interface {
	EnsureExactComponentReleaseV1(context.Context, contract.ComponentReleaseV1) (contract.ComponentReleaseV1, error)
}

type ComponentReleaseReaderV1 interface {
	InspectExactComponentReleaseV1(context.Context, contract.ComponentReleaseRefV1) (contract.ComponentReleaseV1, error)
}
