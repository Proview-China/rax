package assemblysdk

import (
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type Builder struct {
	input assemblycontract.AssemblyInputV1
}

func NewBuilder(input assemblycontract.AssemblyInputV1) *Builder {
	return &Builder{input: input}
}

func (b *Builder) UsePublicCatalogV1() *Builder {
	if b == nil {
		return b
	}
	b.input.Slots = assemblycontract.SlotCatalogV1()
	b.input.HookFaces = assemblycontract.HookFaceCatalogV1()
	return b
}

func (b *Builder) AddComponentManifest(value runtimeports.ComponentManifestV2) *Builder {
	if b != nil {
		b.input.ComponentManifests = append(b.input.ComponentManifests, value)
	}
	return b
}
func (b *Builder) AddModule(value assemblycontract.ModuleDescriptorV1) *Builder {
	if b != nil {
		b.input.Modules = append(b.input.Modules, value)
	}
	return b
}
func (b *Builder) AddCapability(value assemblycontract.CapabilityDescriptorV1) *Builder {
	if b != nil {
		b.input.Capabilities = append(b.input.Capabilities, value)
	}
	return b
}
func (b *Builder) AddSlotContribution(value assemblycontract.SlotContributionV1) *Builder {
	if b != nil {
		b.input.SlotContributions = append(b.input.SlotContributions, value)
	}
	return b
}
func (b *Builder) AddPortSpec(value assemblycontract.PortSpecV1) *Builder {
	if b != nil {
		b.input.PortSpecs = append(b.input.PortSpecs, value)
	}
	return b
}
func (b *Builder) AddPhaseContribution(value assemblycontract.PhaseContributionV1) *Builder {
	if b != nil {
		b.input.PhaseContributions = append(b.input.PhaseContributions, value)
	}
	return b
}
func (b *Builder) AddDependency(value assemblycontract.DependencySpecV1) *Builder {
	if b != nil {
		b.input.Dependencies = append(b.input.Dependencies, value)
	}
	return b
}
func (b *Builder) AddFactory(value assemblycontract.ModuleFactoryDescriptorV1) *Builder {
	if b != nil {
		b.input.Factories = append(b.input.Factories, value)
	}
	return b
}
func (b *Builder) AddProviderCandidate(value assemblycontract.ProviderBindingCandidateV1) *Builder {
	if b != nil {
		b.input.ProviderBindingCandidates = append(b.input.ProviderBindingCandidates, value)
	}
	return b
}

func (b *Builder) Build() (assemblycontract.AssemblyInputV1, error) {
	if b == nil {
		return assemblycontract.AssemblyInputV1{}, errNilBuilder()
	}
	return assemblycontract.SealAssemblyInputV1(clone(b.input))
}
