package contract

import (
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const (
	ContractVersionV1            = "praxis.agent.package/v1"
	SchemaVersionV1              = "v1"
	LockObjectKindV1             = "AgentPackageLockManifestV1"
	PackageObjectKindV1          = "AgentPackageV1"
	MaxLockedComponentReleasesV1 = 256
)

type AgentPackageRefV1 struct {
	PackageID       string        `json:"package_id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ContractVersion string        `json:"contract_version"`
	SchemaVersion   string        `json:"schema_version"`
}

type AgentPackageLockManifestV1 struct {
	ContractVersion        string                                         `json:"contract_version"`
	SchemaVersion          string                                         `json:"schema_version"`
	ObjectKind             string                                         `json:"object_kind"`
	DefinitionRef          definitioncontract.AgentDefinitionRefV1        `json:"definition_ref"`
	ResolvedPlanRef        assemblercontract.ResolvedAgentPlanRefV1       `json:"resolved_plan_ref"`
	ResolutionFactsRef     assemblercontract.ResolutionFactsRefV1         `json:"resolution_facts_ref"`
	CatalogRef             assemblercontract.ComponentReleaseCatalogRefV1 `json:"catalog_ref"`
	ComponentReleaseRefs   []assemblercontract.ComponentReleaseRefV1      `json:"component_release_refs"`
	BindingPlanDigest      core.Digest                                    `json:"binding_plan_digest"`
	AssemblyInputDigest    core.Digest                                    `json:"assembly_input_digest"`
	FrozenUnixNano         int64                                          `json:"frozen_unix_nano"`
	HarnessCompilerVersion string                                         `json:"harness_compiler_version"`
	PublicationRef         assemblycontract.AssemblyPublicationRefV2      `json:"publication_ref"`
	GenerationRef          assemblycontract.ObjectRefV1                   `json:"generation_ref"`
	ManifestRef            assemblycontract.ObjectRefV1                   `json:"manifest_ref"`
	GraphRef               assemblycontract.ObjectRefV1                   `json:"graph_ref"`
	HandoffRef             assemblycontract.ObjectRefV1                   `json:"handoff_ref"`
	Digest                 core.Digest                                    `json:"digest"`
}

type AgentPackageV1 struct {
	ContractVersion string                     `json:"contract_version"`
	SchemaVersion   string                     `json:"schema_version"`
	ObjectKind      string                     `json:"object_kind"`
	PackageID       string                     `json:"package_id"`
	Revision        core.Revision              `json:"revision"`
	CreatedUnixNano int64                      `json:"created_unix_nano"`
	Lock            AgentPackageLockManifestV1 `json:"lock"`
	Digest          core.Digest                `json:"digest"`
}

func (p AgentPackageV1) RefV1() AgentPackageRefV1 {
	return AgentPackageRefV1{PackageID: p.PackageID, Revision: p.Revision, Digest: p.Digest, ContractVersion: p.ContractVersion, SchemaVersion: p.SchemaVersion}
}
