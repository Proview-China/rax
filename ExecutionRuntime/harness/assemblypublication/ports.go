// Package assemblypublication owns publication of complete Harness Assembly
// artifact sets. It does not perform Runtime Binding or construct an Agent.
package assemblypublication

import (
	"context"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type CompilerV1 interface {
	Compile(assemblycontract.AssemblyInputV1) (assemblycontract.CompileResultV1, error)
}

// OwnerStoreV2 exposes the Harness-owned staging and single visibility
// barrier. Implementations must keep staged objects private from HistoricalV2.
type OwnerStoreV2 interface {
	StageGenerationV2(context.Context, string, assemblycontract.AssemblyGenerationV1) error
	StageManifestV2(context.Context, string, assemblycontract.AssemblyManifestV1) error
	StageGraphV2(context.Context, string, assemblycontract.CompiledHarnessGraphV1) error
	StageHandoffV2(context.Context, string, assemblycontract.AssemblyHandoffV1) error
	InspectStagedPublicationV2(context.Context, string) (StagedPublicationInspectionV2, error)
	CommitPublicationCurrentV2(context.Context, CommitPublicationCurrentRequestV2) (assemblycontract.AssemblyPublicationCurrentV2, error)
	InspectHistoricalPublicationV2(context.Context, assemblycontract.AssemblyPublicationRefV2) (assemblycontract.AssemblyPublicationBundleV2, error)
	InspectCommittedPublicationCurrentV2(context.Context, assemblycontract.AssemblyPublicationRefV2) (assemblycontract.AssemblyPublicationCurrentV2, error)
	InspectCurrentPublicationV2(context.Context, string) (assemblycontract.AssemblyPublicationCurrentV2, error)
}

type StagedPublicationInspectionV2 struct {
	PublicationID    string      `json:"publication_id"`
	GenerationDigest core.Digest `json:"generation_digest,omitempty"`
	ManifestDigest   core.Digest `json:"manifest_digest,omitempty"`
	GraphDigest      core.Digest `json:"graph_digest,omitempty"`
	HandoffDigest    core.Digest `json:"handoff_digest,omitempty"`
}

type CommitPublicationCurrentRequestV2 struct {
	Expected assemblycontract.AssemblyPublicationCurrentExpectationV2 `json:"expected"`
	Bundle   assemblycontract.AssemblyPublicationBundleV2             `json:"bundle"`
	Current  assemblycontract.AssemblyPublicationCurrentV2            `json:"current"`
}

type HistoricalReaderV2 interface {
	InspectAssemblyPublicationHistoricalV2(context.Context, assemblycontract.AssemblyPublicationRefV2) (assemblycontract.AssemblyPublicationBundleV2, error)
}

type CurrentReaderV2 interface {
	InspectAssemblyPublicationCurrentV2(context.Context, string) (assemblycontract.AssemblyPublicationCurrentV2, error)
}

type CompileAndPublisherV2 interface {
	CompileAndPublishAssemblyV2(context.Context, assemblycontract.CompileAndPublishAssemblyRequestV2) (assemblycontract.CompileAndPublishAssemblyResultV2, error)
	EnsureAssemblyPublicationV2(context.Context, assemblycontract.CompileAndPublishAssemblyRequestV2) (assemblycontract.CompileAndPublishAssemblyResultV2, error)
}
