package mapper

import (
	"sort"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func BindingPlanV2(planID string, definition definitioncontract.AgentDefinitionV1, releases []assemblercontract.ComponentReleaseV1, governanceDigest core.Digest) (runtimeports.BindingPlanV2, error) {
	byComponent := make(map[string]assemblercontract.ComponentReleaseV1, len(releases))
	for _, release := range releases {
		byComponent[string(release.ComponentManifest.ComponentID)] = release
	}
	requirements := make([]runtimeports.BindingRequirementV2, 0, len(definition.Components))
	for _, requirement := range definition.Components {
		release, ok := byComponent[requirement.ComponentID]
		if !ok && !requirement.Required {
			continue
		}
		if !ok {
			return runtimeports.BindingPlanV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonComponentMissing, "binding mapper is missing a resolved component release")
		}
		capabilities := make([]runtimeports.CapabilityNameV2, len(requirement.RequiredCapabilities))
		for index, capability := range requirement.RequiredCapabilities {
			capabilities[index] = runtimeports.CapabilityNameV2(capability)
		}
		requirements = append(requirements, runtimeports.BindingRequirementV2{
			ComponentID: runtimeports.ComponentIDV2(requirement.ComponentID), Kind: runtimeports.ComponentKindV2(requirement.Kind),
			SemanticVersion: runtimeports.VersionRangeV2{MinimumInclusive: requirement.SemanticVersion.MinimumInclusive, MaximumExclusive: requirement.SemanticVersion.MaximumExclusive},
			ContractName:    runtimeports.NamespacedNameV2(requirement.ContractName),
			Contract:        runtimeports.VersionRangeV2{MinimumInclusive: requirement.ContractVersion.MinimumInclusive, MaximumExclusive: requirement.ContractVersion.MaximumExclusive},
			ArtifactDigest:  release.ArtifactDigest, RequiredCapabilities: capabilities, Required: requirement.Required, AllowResidual: !requirement.Required || requirement.ResidualPolicy.Allowed,
		})
	}
	sort.Slice(requirements, func(i, j int) bool { return requirements[i].ComponentID < requirements[j].ComponentID })
	return runtimeports.SealBindingPlanV2(runtimeports.BindingPlanV2{ID: planID + "-binding", GovernanceDigest: governanceDigest, Requirements: requirements})
}

func AssemblyInputV1(plan assemblercontract.ResolvedAgentPlanV1, facts assemblercontract.ResolutionFactsSnapshotV1, releases []assemblercontract.ComponentReleaseV1) (assemblycontract.AssemblyInputV1, error) {
	input := assemblycontract.AssemblyInputV1{
		InputID: plan.PlanID + "-assembly", Revision: plan.Revision, OwnerRef: facts.OwnerRef, ScopeRef: facts.ScopeRef,
		CreatedUnixNano: facts.FrozenUnixNano, Plan: plan.AssemblyPlanRefs,
		CurrentFacts: append([]assemblycontract.ObjectRefV1{}, facts.CurrentFacts...), RouteBindings: append([]assemblycontract.ObjectRefV1{}, facts.RouteBindings...),
		Policy: assemblycontract.AssemblyPolicyV1{MaximumPriority: facts.MaximumPriority}, EvidenceRefs: append([]assemblycontract.ObjectRefV1{}, facts.EvidenceRefs...),
	}
	for _, release := range releases {
		input.ComponentManifests = append(input.ComponentManifests, release.ComponentManifest)
		input.Modules = append(input.Modules, release.ModuleDescriptors...)
		input.Capabilities = append(input.Capabilities, release.CapabilityDescriptors...)
		input.Slots = append(input.Slots, release.SlotSpecs...)
		input.SlotContributions = append(input.SlotContributions, release.SlotContributions...)
		input.PortSpecs = append(input.PortSpecs, release.PortSpecs...)
		input.HookFaces = append(input.HookFaces, release.HookFaces...)
		input.PhaseContributions = append(input.PhaseContributions, release.PhaseContributions...)
		input.Dependencies = append(input.Dependencies, release.Dependencies...)
		input.Factories = append(input.Factories, release.FactoryDescriptors...)
		input.ProviderBindingCandidates = append(input.ProviderBindingCandidates, release.ProviderBindingCandidates...)
		input.CurrentFacts = append(input.CurrentFacts, release.SourceRef, release.CertificationRef)
		input.EvidenceRefs = append(input.EvidenceRefs, release.EvidenceRefs...)
	}
	var err error
	input.CurrentFacts, err = uniqueRefs(input.CurrentFacts)
	if err != nil {
		return assemblycontract.AssemblyInputV1{}, err
	}
	input.RouteBindings, err = uniqueRefs(input.RouteBindings)
	if err != nil {
		return assemblycontract.AssemblyInputV1{}, err
	}
	input.EvidenceRefs, err = uniqueRefs(input.EvidenceRefs)
	if err != nil {
		return assemblycontract.AssemblyInputV1{}, err
	}
	return assemblycontract.SealAssemblyInputV1(input)
}

func uniqueRefs(values []assemblycontract.ObjectRefV1) ([]assemblycontract.ObjectRefV1, error) {
	byID := map[string]assemblycontract.ObjectRefV1{}
	for _, ref := range values {
		if existing, ok := byID[ref.ID]; ok && existing != ref {
			return nil, core.NewError(core.ErrorConflict, core.ReasonBindingDrift, "assembly exact ref id maps to different content")
		}
		byID[ref.ID] = ref
	}
	result := make([]assemblycontract.ObjectRefV1, 0, len(byID))
	for _, ref := range byID {
		result = append(result, ref)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}
