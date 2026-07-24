package contract

import (
	"encoding/json"
	"sort"

	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const digestDomainV1 = "praxis.agent.assembler"

func clone[T any](value T) T {
	payload, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var result T
	if err := json.Unmarshal(payload, &result); err != nil {
		return value
	}
	return result
}

func ComponentReleaseDigestV1(value ComponentReleaseV1) (core.Digest, error) {
	value = normalizeRelease(value)
	value.ReleaseDigest = ""
	return core.CanonicalJSONDigest(digestDomainV1, ReleaseContractVersionV1, "ComponentReleaseV1", value)
}

// ComponentReleaseCertificationDigestV1 is the exact payload certified by a
// production CertificationRef. The reference itself and the outer release
// seal are excluded to avoid a digest cycle; every semantic release field,
// including identity, revision, manifest, artifact, contract, conformance,
// construction closure, evidence, and validity window, remains covered.
func ComponentReleaseCertificationDigestV1(value ComponentReleaseV1) (core.Digest, error) {
	value = normalizeRelease(clone(value))
	value.ContractVersion = ReleaseContractVersionV1
	value.CertificationRef = assemblycontract.ObjectRefV1{}
	value.ReleaseDigest = ""
	return core.CanonicalJSONDigest(digestDomainV1, ReleaseContractVersionV1, "ComponentReleaseCertificationV1", value)
}

func SealComponentReleaseV1(value ComponentReleaseV1) (ComponentReleaseV1, error) {
	value = normalizeRelease(clone(value))
	value.ContractVersion = ReleaseContractVersionV1
	value.ReleaseDigest = runtimeports.EvidenceGenesisDigestV2
	digest, err := ComponentReleaseDigestV1(value)
	if err != nil {
		return ComponentReleaseV1{}, err
	}
	value.ReleaseDigest = digest
	return value, value.Validate()
}

func normalizeRelease(value ComponentReleaseV1) ComponentReleaseV1 {
	sort.Slice(value.ModuleDescriptors, func(i, j int) bool { return value.ModuleDescriptors[i].ModuleID < value.ModuleDescriptors[j].ModuleID })
	sort.Slice(value.CapabilityDescriptors, func(i, j int) bool {
		return value.CapabilityDescriptors[i].Capability < value.CapabilityDescriptors[j].Capability
	})
	sort.Slice(value.SlotSpecs, func(i, j int) bool { return value.SlotSpecs[i].SlotID < value.SlotSpecs[j].SlotID })
	sort.Slice(value.SlotContributions, func(i, j int) bool {
		return value.SlotContributions[i].ContributionID < value.SlotContributions[j].ContributionID
	})
	sort.Slice(value.PortSpecs, func(i, j int) bool { return value.PortSpecs[i].PortID < value.PortSpecs[j].PortID })
	sort.Slice(value.HookFaces, func(i, j int) bool { return value.HookFaces[i].HookFaceID < value.HookFaces[j].HookFaceID })
	sort.Slice(value.PhaseContributions, func(i, j int) bool {
		return value.PhaseContributions[i].ContributionID < value.PhaseContributions[j].ContributionID
	})
	sort.Slice(value.Dependencies, func(i, j int) bool {
		if value.Dependencies[i].FromRef != value.Dependencies[j].FromRef {
			return value.Dependencies[i].FromRef < value.Dependencies[j].FromRef
		}
		return value.Dependencies[i].ToRef < value.Dependencies[j].ToRef
	})
	sort.Slice(value.FactoryDescriptors, func(i, j int) bool {
		return value.FactoryDescriptors[i].FactoryID < value.FactoryDescriptors[j].FactoryID
	})
	sort.Slice(value.ProviderBindingCandidates, func(i, j int) bool {
		return value.ProviderBindingCandidates[i].CandidateID < value.ProviderBindingCandidates[j].CandidateID
	})
	sort.Slice(value.RequiredPlanArtifacts, func(i, j int) bool { return value.RequiredPlanArtifacts[i].Role < value.RequiredPlanArtifacts[j].Role })
	value.EvidenceRefs = normalizeRefs(value.EvidenceRefs)
	normalizeNilRelease(&value)
	return value
}

func normalizeNilRelease(value *ComponentReleaseV1) {
	if value.ModuleDescriptors == nil {
		value.ModuleDescriptors = []assemblycontract.ModuleDescriptorV1{}
	}
	if value.CapabilityDescriptors == nil {
		value.CapabilityDescriptors = []assemblycontract.CapabilityDescriptorV1{}
	}
	if value.SlotSpecs == nil {
		value.SlotSpecs = []assemblycontract.SlotSpecV1{}
	}
	if value.SlotContributions == nil {
		value.SlotContributions = []assemblycontract.SlotContributionV1{}
	}
	if value.PortSpecs == nil {
		value.PortSpecs = []assemblycontract.PortSpecV1{}
	}
	if value.HookFaces == nil {
		value.HookFaces = []assemblycontract.HookFaceSpecV1{}
	}
	if value.PhaseContributions == nil {
		value.PhaseContributions = []assemblycontract.PhaseContributionV1{}
	}
	if value.Dependencies == nil {
		value.Dependencies = []assemblycontract.DependencySpecV1{}
	}
	if value.FactoryDescriptors == nil {
		value.FactoryDescriptors = []assemblycontract.ModuleFactoryDescriptorV1{}
	}
	if value.ProviderBindingCandidates == nil {
		value.ProviderBindingCandidates = []assemblycontract.ProviderBindingCandidateV1{}
	}
	if value.RequiredPlanArtifacts == nil {
		value.RequiredPlanArtifacts = []PlanArtifactV1{}
	}
	if value.EvidenceRefs == nil {
		value.EvidenceRefs = []assemblycontract.ObjectRefV1{}
	}
}

func ComponentReleaseCatalogDigestV1(value ComponentReleaseCatalogSnapshotV1) (core.Digest, error) {
	value = normalizeCatalog(clone(value))
	value.Digest = ""
	return core.CanonicalJSONDigest(digestDomainV1, CatalogContractVersionV1, "ComponentReleaseCatalogSnapshotV1", value)
}

func normalizeCatalog(value ComponentReleaseCatalogSnapshotV1) ComponentReleaseCatalogSnapshotV1 {
	for index := range value.Releases {
		value.Releases[index] = normalizeRelease(value.Releases[index])
	}
	sort.Slice(value.Releases, func(i, j int) bool {
		if value.Releases[i].ComponentManifest.ComponentID != value.Releases[j].ComponentManifest.ComponentID {
			return value.Releases[i].ComponentManifest.ComponentID < value.Releases[j].ComponentManifest.ComponentID
		}
		if value.Releases[i].ComponentManifest.SemanticVersion != value.Releases[j].ComponentManifest.SemanticVersion {
			return value.Releases[i].ComponentManifest.SemanticVersion < value.Releases[j].ComponentManifest.SemanticVersion
		}
		if value.Releases[i].ReleaseID != value.Releases[j].ReleaseID {
			return value.Releases[i].ReleaseID < value.Releases[j].ReleaseID
		}
		return value.Releases[i].Revision < value.Releases[j].Revision
	})
	if value.Releases == nil {
		value.Releases = []ComponentReleaseV1{}
	}
	return value
}

func SealComponentReleaseCatalogV1(value ComponentReleaseCatalogSnapshotV1) (ComponentReleaseCatalogSnapshotV1, error) {
	value = normalizeCatalog(clone(value))
	value.ContractVersion = CatalogContractVersionV1
	governanceDigest, err := value.Governance.DigestV2()
	if err != nil {
		return ComponentReleaseCatalogSnapshotV1{}, err
	}
	value.GovernanceDigest = governanceDigest
	value.Digest = runtimeports.EvidenceGenesisDigestV2
	digest, err := ComponentReleaseCatalogDigestV1(value)
	if err != nil {
		return ComponentReleaseCatalogSnapshotV1{}, err
	}
	value.Digest = digest
	return value, value.Validate()
}

func ResolutionFactsDigestV1(value ResolutionFactsSnapshotV1) (core.Digest, error) {
	value = clone(value)
	value.Digest = ""
	value.PolicyRefs = normalizeRefs(value.PolicyRefs)
	value.CurrentFacts = normalizeRefs(value.CurrentFacts)
	value.RouteBindings = normalizeRefs(value.RouteBindings)
	value.EvidenceRefs = normalizeRefs(value.EvidenceRefs)
	return core.CanonicalJSONDigest(digestDomainV1, FactsContractVersionV1, "ResolutionFactsSnapshotV1", value)
}

func SealResolutionFactsV1(value ResolutionFactsSnapshotV1) (ResolutionFactsSnapshotV1, error) {
	value = clone(value)
	value.PolicyRefs = normalizeRefs(value.PolicyRefs)
	value.CurrentFacts = normalizeRefs(value.CurrentFacts)
	value.RouteBindings = normalizeRefs(value.RouteBindings)
	value.EvidenceRefs = normalizeRefs(value.EvidenceRefs)
	value.ContractVersion = FactsContractVersionV1
	value.Digest = runtimeports.EvidenceGenesisDigestV2
	digest, err := ResolutionFactsDigestV1(value)
	if err != nil {
		return ResolutionFactsSnapshotV1{}, err
	}
	value.Digest = digest
	return value, value.Validate()
}

func DerivePlanIDV1(definition definitioncontract.AgentDefinitionRefV1, facts ResolutionFactsRefV1, catalog ComponentReleaseCatalogRefV1) (string, error) {
	digest, err := core.CanonicalJSONDigest(digestDomainV1, PlanContractVersionV1, "ResolvedAgentPlanIdentityV1", struct {
		Definition definitioncontract.AgentDefinitionRefV1 `json:"definition"`
		Facts      ResolutionFactsRefV1                    `json:"facts"`
		Catalog    ComponentReleaseCatalogRefV1            `json:"catalog"`
	}{Definition: definition, Facts: facts, Catalog: catalog})
	if err != nil {
		return "", err
	}
	return "resolved-agent-plan-" + string(digest), nil
}

func ResolvedAgentPlanDigestV1(value ResolvedAgentPlanV1) (core.Digest, error) {
	value = clone(value)
	value.Digest = ""
	value.AssemblyPlanRefs.ResolvedAgentPlan.Digest = ""
	value.PolicyRefs = normalizeRefs(value.PolicyRefs)
	value.EvidenceRefs = normalizeRefs(value.EvidenceRefs)
	sort.Slice(value.ComponentReleases, func(i, j int) bool {
		return value.ComponentReleases[i].Manifest.ComponentID < value.ComponentReleases[j].Manifest.ComponentID
	})
	if value.Residuals == nil {
		value.Residuals = []assemblycontract.ResidualReportV1{}
	}
	if value.ComponentReleases == nil {
		value.ComponentReleases = []ResolvedComponentV1{}
	}
	return core.CanonicalJSONDigest(digestDomainV1, PlanContractVersionV1, "ResolvedAgentPlanV1", value)
}

func SealResolvedAgentPlanV1(value ResolvedAgentPlanV1) (ResolvedAgentPlanV1, error) {
	value = clone(value)
	value.PolicyRefs = normalizeRefs(value.PolicyRefs)
	value.EvidenceRefs = normalizeRefs(value.EvidenceRefs)
	sort.Slice(value.ComponentReleases, func(i, j int) bool {
		return value.ComponentReleases[i].Manifest.ComponentID < value.ComponentReleases[j].Manifest.ComponentID
	})
	if value.Residuals == nil {
		value.Residuals = []assemblycontract.ResidualReportV1{}
	}
	if value.ComponentReleases == nil {
		value.ComponentReleases = []ResolvedComponentV1{}
	}
	value.ContractVersion = PlanContractVersionV1
	value.Digest = runtimeports.EvidenceGenesisDigestV2
	value.AssemblyPlanRefs.ResolvedAgentPlan = assemblycontract.ObjectRefV1{ID: value.PlanID, Revision: value.Revision, Digest: runtimeports.EvidenceGenesisDigestV2}
	digest, err := ResolvedAgentPlanDigestV1(value)
	if err != nil {
		return ResolvedAgentPlanV1{}, err
	}
	value.Digest = digest
	value.AssemblyPlanRefs.ResolvedAgentPlan.Digest = digest
	return value, value.Validate()
}

func CurrentResolvedPlanDigestV1(value CurrentResolvedPlanV1) (core.Digest, error) {
	value.Digest = ""
	return core.CanonicalJSONDigest(digestDomainV1, PlanContractVersionV1, "CurrentResolvedPlanV1", value)
}

func SealCurrentResolvedPlanV1(value CurrentResolvedPlanV1) (CurrentResolvedPlanV1, error) {
	value.Digest = runtimeports.EvidenceGenesisDigestV2
	digest, err := CurrentResolvedPlanDigestV1(value)
	if err != nil {
		return CurrentResolvedPlanV1{}, err
	}
	value.Digest = digest
	return value, value.Validate()
}

func CloneComponentReleaseV1(value ComponentReleaseV1) ComponentReleaseV1 { return clone(value) }
func CloneCatalogV1(value ComponentReleaseCatalogSnapshotV1) ComponentReleaseCatalogSnapshotV1 {
	return clone(value)
}
func CloneResolutionFactsV1(value ResolutionFactsSnapshotV1) ResolutionFactsSnapshotV1 {
	return clone(value)
}
func CloneResolvedAgentPlanV1(value ResolvedAgentPlanV1) ResolvedAgentPlanV1 { return clone(value) }
func CloneCurrentResolvedPlanV1(value CurrentResolvedPlanV1) CurrentResolvedPlanV1 {
	return clone(value)
}
func CloneResolveResultV1(value ResolveResultV1) ResolveResultV1 { return clone(value) }
