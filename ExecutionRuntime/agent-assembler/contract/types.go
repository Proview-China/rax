package contract

import (
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const (
	ContractVersionV1        = "praxis.agent.assembler/v1"
	PlanContractVersionV1    = "praxis.agent.assembler.plan/v1"
	ReleaseContractVersionV1 = "praxis.agent.assembler.release/v1"
	CatalogContractVersionV1 = "praxis.agent.assembler.catalog/v1"
	FactsContractVersionV1   = "praxis.agent.assembler.resolution-facts/v1"
	MaxEntriesV1             = 512
)

type SupportModeV1 string

const (
	SupportDisabledV1      SupportModeV1 = "disabled"
	SupportReferenceOnlyV1 SupportModeV1 = "reference_only"
	SupportStandaloneV1    SupportModeV1 = "standalone"
	SupportProductionV1    SupportModeV1 = "production"
)

type PlanArtifactRoleV1 string

const (
	ArtifactHarnessBootstrapV1  PlanArtifactRoleV1 = "harness_bootstrap_plan"
	ArtifactProfileV1           PlanArtifactRoleV1 = "profile"
	ArtifactRuntimePolicyV1     PlanArtifactRoleV1 = "runtime_policy"
	ArtifactHarnessStackV1      PlanArtifactRoleV1 = "harness_stack"
	ArtifactSemanticRouteV1     PlanArtifactRoleV1 = "semantic_route"
	ArtifactContextPlanV1       PlanArtifactRoleV1 = "context_plan"
	ArtifactToolSurfaceV1       PlanArtifactRoleV1 = "tool_surface"
	ArtifactCapabilityGrantV1   PlanArtifactRoleV1 = "capability_grant"
	ArtifactExpectedInjectionV1 PlanArtifactRoleV1 = "expected_injection_manifest"
)

type PlanArtifactV1 struct {
	Role PlanArtifactRoleV1           `json:"role"`
	Ref  assemblycontract.ObjectRefV1 `json:"ref"`
}

type ComponentReleaseRefV1 struct {
	ReleaseID   string                     `json:"release_id"`
	Revision    core.Revision              `json:"revision"`
	Digest      core.Digest                `json:"digest"`
	ComponentID runtimeports.ComponentIDV2 `json:"component_id"`
}

type ComponentReleaseV1 struct {
	ContractVersion           string                                        `json:"contract_version"`
	ReleaseID                 string                                        `json:"release_id"`
	Revision                  core.Revision                                 `json:"revision"`
	SupportMode               SupportModeV1                                 `json:"support_mode"`
	ComponentManifest         runtimeports.ComponentManifestV2              `json:"component_manifest"`
	ModuleDescriptors         []assemblycontract.ModuleDescriptorV1         `json:"module_descriptors"`
	CapabilityDescriptors     []assemblycontract.CapabilityDescriptorV1     `json:"capability_descriptors"`
	SlotSpecs                 []assemblycontract.SlotSpecV1                 `json:"slot_specs"`
	SlotContributions         []assemblycontract.SlotContributionV1         `json:"slot_contributions"`
	PortSpecs                 []assemblycontract.PortSpecV1                 `json:"port_specs"`
	HookFaces                 []assemblycontract.HookFaceSpecV1             `json:"hookfaces"`
	PhaseContributions        []assemblycontract.PhaseContributionV1        `json:"phase_contributions"`
	Dependencies              []assemblycontract.DependencySpecV1           `json:"dependencies"`
	FactoryDescriptors        []assemblycontract.ModuleFactoryDescriptorV1  `json:"factory_descriptors"`
	ProviderBindingCandidates []assemblycontract.ProviderBindingCandidateV1 `json:"provider_binding_candidates"`
	RequiredPlanArtifacts     []PlanArtifactV1                              `json:"required_plan_artifacts"`
	SourceRef                 assemblycontract.ObjectRefV1                  `json:"source_ref"`
	ArtifactDigest            core.Digest                                   `json:"artifact_digest"`
	CertificationRef          assemblycontract.ObjectRefV1                  `json:"certification_ref"`
	EvidenceRefs              []assemblycontract.ObjectRefV1                `json:"evidence_refs"`
	CreatedUnixNano           int64                                         `json:"created_unix_nano"`
	ExpiresUnixNano           int64                                         `json:"expires_unix_nano"`
	ReleaseDigest             core.Digest                                   `json:"release_digest"`
}

type ComponentReleaseCatalogRefV1 struct {
	CatalogID string        `json:"catalog_id"`
	Revision  core.Revision `json:"revision"`
	Digest    core.Digest   `json:"digest"`
}

type ComponentReleaseCatalogSnapshotV1 struct {
	ContractVersion  string                           `json:"contract_version"`
	CatalogID        string                           `json:"catalog_id"`
	Revision         core.Revision                    `json:"revision"`
	Releases         []ComponentReleaseV1             `json:"releases"`
	Governance       runtimeports.GovernanceCatalogV2 `json:"governance"`
	GovernanceDigest core.Digest                      `json:"governance_digest"`
	CheckedUnixNano  int64                            `json:"checked_unix_nano"`
	ExpiresUnixNano  int64                            `json:"expires_unix_nano"`
	Digest           core.Digest                      `json:"digest"`
}

type ResolutionFactsRefV1 struct {
	FactsID  string        `json:"facts_id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

type ResolutionFactsSnapshotV1 struct {
	ContractVersion       string                                  `json:"contract_version"`
	FactsID               string                                  `json:"facts_id"`
	Revision              core.Revision                           `json:"revision"`
	DefinitionRef         definitioncontract.AgentDefinitionRefV1 `json:"definition_ref"`
	IdentityRef           assemblycontract.ObjectRefV1            `json:"identity_ref"`
	PolicyRefs            []assemblycontract.ObjectRefV1          `json:"policy_refs"`
	SandboxRequirementRef assemblycontract.ObjectRefV1            `json:"sandbox_requirement_ref"`
	CurrentFacts          []assemblycontract.ObjectRefV1          `json:"current_facts"`
	RouteBindings         []assemblycontract.ObjectRefV1          `json:"route_bindings"`
	EvidenceRefs          []assemblycontract.ObjectRefV1          `json:"evidence_refs"`
	OwnerRef              string                                  `json:"owner_ref"`
	ScopeRef              string                                  `json:"scope_ref"`
	FrozenUnixNano        int64                                   `json:"frozen_unix_nano"`
	ExpiresUnixNano       int64                                   `json:"expires_unix_nano"`
	MaximumPriority       int32                                   `json:"maximum_priority"`
	Digest                core.Digest                             `json:"digest"`
}

type ResolvedComponentV1 struct {
	RequirementID string                           `json:"requirement_id"`
	ReleaseRef    ComponentReleaseRefV1            `json:"release_ref"`
	Manifest      runtimeports.ComponentManifestV2 `json:"manifest"`
}

type ResolvedAgentPlanRefV1 struct {
	PlanID   string        `json:"plan_id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

type ResolvedAgentPlanV1 struct {
	ContractVersion       string                                  `json:"contract_version"`
	PlanID                string                                  `json:"plan_id"`
	Revision              core.Revision                           `json:"revision"`
	Digest                core.Digest                             `json:"digest"`
	DefinitionRef         definitioncontract.AgentDefinitionRefV1 `json:"definition_ref"`
	IdentityRef           assemblycontract.ObjectRefV1            `json:"identity_ref"`
	ProfileRef            assemblycontract.ObjectRefV1            `json:"profile_ref"`
	PolicyRefs            []assemblycontract.ObjectRefV1          `json:"policy_refs"`
	SandboxRequirementRef assemblycontract.ObjectRefV1            `json:"sandbox_requirement_ref"`
	ComponentReleases     []ResolvedComponentV1                   `json:"component_releases"`
	BindingPlan           runtimeports.BindingPlanV2              `json:"binding_plan"`
	AssemblyPlanRefs      assemblycontract.AssemblyPlanRefsV1     `json:"assembly_plan_refs"`
	HarnessBootstrapRef   assemblycontract.ObjectRefV1            `json:"harness_bootstrap_ref"`
	ResolutionFactsRef    ResolutionFactsRefV1                    `json:"resolution_facts_ref"`
	CatalogRef            ComponentReleaseCatalogRefV1            `json:"catalog_ref"`
	Residuals             []assemblycontract.ResidualReportV1     `json:"residuals"`
	EvidenceRefs          []assemblycontract.ObjectRefV1          `json:"evidence_refs"`
	CreatedUnixNano       int64                                   `json:"created_unix_nano"`
	ValidUntilUnixNano    int64                                   `json:"valid_until_unix_nano"`
}

type ResolveRequestV1 struct {
	Definition definitioncontract.AgentDefinitionV1 `json:"definition"`
	FactsRef   ResolutionFactsRefV1                 `json:"facts_ref"`
	CatalogRef ComponentReleaseCatalogRefV1         `json:"catalog_ref"`
}

type ResolveResultV1 struct {
	Plan          ResolvedAgentPlanV1              `json:"plan"`
	BindingPlan   runtimeports.BindingPlanV2       `json:"binding_plan"`
	AssemblyInput assemblycontract.AssemblyInputV1 `json:"assembly_input"`
}

type CurrentResolvedPlanV1 struct {
	DefinitionID    string                    `json:"definition_id"`
	Revision        core.Revision             `json:"revision"`
	PlanRef         ResolvedAgentPlanRefV1    `json:"plan_ref"`
	PreviousRef     *CurrentResolvedPlanRefV1 `json:"previous_ref,omitempty"`
	UpdatedUnixNano int64                     `json:"updated_unix_nano"`
	CheckedUnixNano int64                     `json:"checked_unix_nano"`
	ExpiresUnixNano int64                     `json:"expires_unix_nano"`
	Digest          core.Digest               `json:"digest"`
}

type CurrentResolvedPlanRefV1 struct {
	DefinitionID string        `json:"definition_id"`
	Revision     core.Revision `json:"revision"`
	Digest       core.Digest   `json:"digest"`
}
