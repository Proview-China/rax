package owneradapter

import (
	"sort"

	hostcontract "github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func constructionGraphV1(manifest assemblycontract.AssemblyManifestV1, graph assemblycontract.CompiledHarnessGraphV1, generation assemblycontract.AssemblyGenerationV1) (hostcontract.ConstructionGraphV1, error) {
	components := make(map[runtimeports.ComponentIDV2]runtimeports.ComponentManifestV2, len(manifest.ComponentManifests))
	for _, component := range manifest.ComponentManifests {
		if err := component.Validate(); err != nil {
			return hostcontract.ConstructionGraphV1{}, ownerErrorV1(err, "component_manifest_invalid")
		}
		if _, ok := components[component.ComponentID]; ok {
			return hostcontract.ConstructionGraphV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "component_alias", "assembly manifest contains duplicate component identity")
		}
		components[component.ComponentID] = component
	}
	modules := make(map[string]assemblycontract.ModuleDescriptorV1, len(manifest.Modules))
	for _, module := range manifest.Modules {
		if err := module.Validate(); err != nil {
			return hostcontract.ConstructionGraphV1{}, ownerErrorV1(err, "module_invalid")
		}
		component, ok := components[runtimeports.ComponentIDV2(module.ComponentManifestRef.ID)]
		if !ok {
			return hostcontract.ConstructionGraphV1{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "module_component_missing", "factory module component manifest is absent")
		}
		digest, err := component.BindingDigestV2()
		if err != nil {
			return hostcontract.ConstructionGraphV1{}, ownerErrorV1(err, "component_manifest_digest_failed")
		}
		if module.ComponentManifestRef.Digest != digest || module.ArtifactDigest != component.ArtifactDigest {
			return hostcontract.ConstructionGraphV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "module_component_splice", "module does not bind its exact component manifest")
		}
		if _, ok := modules[module.ModuleID]; ok {
			return hostcontract.ConstructionGraphV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "module_alias", "assembly manifest contains duplicate module identity")
		}
		modules[module.ModuleID] = module
	}
	nodes := make(map[string]*hostcontract.ComponentNodeV1, len(manifest.Factories))
	componentFactories := make(map[runtimeports.ComponentIDV2][]string)
	for _, factory := range manifest.Factories {
		if err := factory.Validate(); err != nil {
			return hostcontract.ConstructionGraphV1{}, ownerErrorV1(err, "factory_invalid")
		}
		module, ok := modules[factory.ModuleRef]
		if !ok {
			return hostcontract.ConstructionGraphV1{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "factory_module_missing", "factory module is absent")
		}
		componentID := runtimeports.ComponentIDV2(module.ComponentManifestRef.ID)
		if factory.ArtifactDigest != module.ArtifactDigest {
			return hostcontract.ConstructionGraphV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "factory_artifact_splice", "factory and module artifact digests differ")
		}
		if _, ok := nodes[factory.FactoryID]; ok {
			return hostcontract.ConstructionGraphV1{}, hostcontract.NewError(hostcontract.ErrorConflict, "factory_alias", "assembly manifest contains duplicate factory identity")
		}
		node := &hostcontract.ComponentNodeV1{NodeID: factory.FactoryID, Factory: hostcontract.FactoryKeyV1{ComponentID: string(componentID), ArtifactDigest: hostcontract.DigestV1(factory.ArtifactDigest), Contract: factory.ContractVersion, Capability: string(factory.OutputCapability)}}
		nodes[factory.FactoryID] = node
		componentFactories[componentID] = append(componentFactories[componentID], factory.FactoryID)
	}
	for componentID := range components {
		if len(componentFactories[componentID]) == 0 {
			return hostcontract.ConstructionGraphV1{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "component_factory_missing", "selected component has no construction factory")
		}
	}
	for sourceID, component := range components {
		for _, dependency := range component.Dependencies {
			targets := componentFactories[dependency.ComponentID]
			if len(targets) == 0 && dependency.Optional {
				continue
			}
			if len(targets) == 0 {
				return hostcontract.ConstructionGraphV1{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "component_dependency_missing", "required component dependency has no construction factory")
			}
			for _, source := range componentFactories[sourceID] {
				for _, target := range targets {
					addDependencyV1(nodes[source], target)
				}
			}
		}
	}
	for _, dependency := range manifest.Dependencies {
		from, fromFactory := nodes[dependency.FromRef]
		_, toFactory := nodes[dependency.ToRef]
		switch {
		case fromFactory && toFactory:
			addDependencyV1(from, dependency.ToRef)
		case fromFactory != toFactory:
			return hostcontract.ConstructionGraphV1{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "factory_dependency_splice", "explicit dependency has only one known factory endpoint")
		case dependency.Required:
			return hostcontract.ConstructionGraphV1{}, hostcontract.NewError(hostcontract.ErrorPrecondition, "required_factory_dependency_unknown", "required explicit dependency has no known factory endpoint")
		}
	}
	result := hostcontract.ConstructionGraphV1{GraphRef: artifactRefV1(GraphKindV1, generation.GenerationID+"/graph", uint64(generation.Revision), graph.Digest)}
	for _, node := range nodes {
		sort.Strings(node.Dependencies)
		result.Nodes = append(result.Nodes, *node)
	}
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].NodeID < result.Nodes[j].NodeID })
	if err := result.Validate(); err != nil {
		return hostcontract.ConstructionGraphV1{}, err
	}
	return result, nil
}

func addDependencyV1(node *hostcontract.ComponentNodeV1, dependency string) {
	for _, existing := range node.Dependencies {
		if existing == dependency {
			return
		}
	}
	node.Dependencies = append(node.Dependencies, dependency)
}
