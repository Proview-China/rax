package assemblycompiler

import (
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type indexes struct {
	manifests          map[string]runtimeports.ComponentManifestV2
	modules            map[string]assemblycontract.ModuleDescriptorV1
	capabilities       map[runtimeports.CapabilityNameV2]assemblycontract.CapabilityDescriptorV1
	slots              map[string]assemblycontract.SlotSpecV1
	slotContributions  map[string]assemblycontract.SlotContributionV1
	ports              map[string]assemblycontract.PortSpecV1
	hookfaces          map[string]assemblycontract.HookFaceSpecV1
	phaseContributions map[string]assemblycontract.PhaseContributionV1
	factories          map[string]assemblycontract.ModuleFactoryDescriptorV1
	candidates         map[string]assemblycontract.ProviderBindingCandidateV1
}

func uniqueInsert[T any](target map[string]T, key string, value T) error {
	if _, exists := target[key]; exists {
		return core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "assembly collection contains a duplicate id")
	}
	target[key] = value
	return nil
}

func buildIndex(input assemblycontract.AssemblyInputV1) (indexes, error) {
	idx := indexes{manifests: map[string]runtimeports.ComponentManifestV2{}, modules: map[string]assemblycontract.ModuleDescriptorV1{}, capabilities: map[runtimeports.CapabilityNameV2]assemblycontract.CapabilityDescriptorV1{}, slots: map[string]assemblycontract.SlotSpecV1{}, slotContributions: map[string]assemblycontract.SlotContributionV1{}, ports: map[string]assemblycontract.PortSpecV1{}, hookfaces: map[string]assemblycontract.HookFaceSpecV1{}, phaseContributions: map[string]assemblycontract.PhaseContributionV1{}, factories: map[string]assemblycontract.ModuleFactoryDescriptorV1{}, candidates: map[string]assemblycontract.ProviderBindingCandidateV1{}}
	for _, value := range input.ComponentManifests {
		if err := uniqueInsert(idx.manifests, string(value.ComponentID), value); err != nil {
			return indexes{}, err
		}
	}
	for _, value := range input.Modules {
		if err := uniqueInsert(idx.modules, value.ModuleID, value); err != nil {
			return indexes{}, err
		}
	}
	for _, value := range input.Capabilities {
		if _, ok := idx.capabilities[value.Capability]; ok {
			return indexes{}, core.NewError(core.ErrorConflict, core.ReasonDuplicateCanonicalKey, "capability descriptor is duplicated")
		}
		idx.capabilities[value.Capability] = value
	}
	for _, value := range input.Slots {
		if err := uniqueInsert(idx.slots, value.SlotID, value); err != nil {
			return indexes{}, err
		}
	}
	for _, value := range input.SlotContributions {
		if err := uniqueInsert(idx.slotContributions, value.ContributionID, value); err != nil {
			return indexes{}, err
		}
	}
	for _, value := range input.PortSpecs {
		if err := uniqueInsert(idx.ports, value.PortID, value); err != nil {
			return indexes{}, err
		}
	}
	for _, value := range input.HookFaces {
		if err := uniqueInsert(idx.hookfaces, value.HookFaceID, value); err != nil {
			return indexes{}, err
		}
	}
	for _, value := range input.PhaseContributions {
		if err := uniqueInsert(idx.phaseContributions, value.ContributionID, value); err != nil {
			return indexes{}, err
		}
	}
	for _, value := range input.Factories {
		if err := uniqueInsert(idx.factories, value.FactoryID, value); err != nil {
			return indexes{}, err
		}
	}
	for _, value := range input.ProviderBindingCandidates {
		if err := uniqueInsert(idx.candidates, value.CandidateID, value); err != nil {
			return indexes{}, err
		}
	}
	return idx, nil
}
