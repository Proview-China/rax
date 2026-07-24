package owneradapter

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	hostcontract "github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestConstructionGraphMapsComponentDependencyToAllTargetFactories(t *testing.T) {
	manifest, graph, generation := graphFixtureV1(t)
	result, err := constructionGraphV1(manifest, graph, generation)
	if err != nil {
		t.Fatal(err)
	}
	node, ok := result.NodeV1("fixture/source-factory")
	if !ok || !reflect.DeepEqual(node.Dependencies, []string{"fixture/target-factory-a", "fixture/target-factory-b"}) {
		t.Fatalf("source node=%+v", node)
	}
}

func TestConstructionGraphRejectsRequiredUnknownAndOneSidedFactoryDependency(t *testing.T) {
	manifest, graph, generation := graphFixtureV1(t)
	manifest.Dependencies = []assemblycontract.DependencySpecV1{dependencyV1("fixture/unknown-a", "fixture/unknown-b", true)}
	if _, err := constructionGraphV1(manifest, graph, generation); !hostcontract.HasCode(err, hostcontract.ErrorPrecondition) {
		t.Fatalf("required unknown error=%v", err)
	}
	manifest.Dependencies = []assemblycontract.DependencySpecV1{dependencyV1("fixture/source-factory", "fixture/unknown", false)}
	if _, err := constructionGraphV1(manifest, graph, generation); !hostcontract.HasCode(err, hostcontract.ErrorPrecondition) {
		t.Fatalf("one-sided error=%v", err)
	}
}

func TestConstructionGraphRejectsCycleAndFactoryAlias(t *testing.T) {
	manifest, graph, generation := graphFixtureV1(t)
	manifest.Dependencies = []assemblycontract.DependencySpecV1{dependencyV1("fixture/target-factory-a", "fixture/source-factory", true)}
	if _, err := constructionGraphV1(manifest, graph, generation); !hostcontract.HasCode(err, hostcontract.ErrorPrecondition) {
		t.Fatalf("cycle error=%v", err)
	}
	manifest, graph, generation = graphFixtureV1(t)
	manifest.Factories[2].OutputCapability = manifest.Factories[1].OutputCapability
	if _, err := constructionGraphV1(manifest, graph, generation); !hostcontract.HasCode(err, hostcontract.ErrorConflict) {
		t.Fatalf("alias error=%v", err)
	}
}

func TestConstructionGraphDeterministicAt64Concurrency(t *testing.T) {
	manifest, graph, generation := graphFixtureV1(t)
	want, err := constructionGraphV1(manifest, graph, generation)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := constructionGraphV1(manifest, graph, generation)
			if err != nil {
				errs <- err
				return
			}
			if !reflect.DeepEqual(got, want) {
				errs <- fmt.Errorf("nondeterministic graph")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func graphFixtureV1(t *testing.T) (assemblycontract.AssemblyManifestV1, assemblycontract.CompiledHarnessGraphV1, assemblycontract.AssemblyGenerationV1) {
	t.Helper()
	target := componentV1("fixture/target", nil)
	source := componentV1("fixture/source", []runtimeports.ComponentDependencyV2{{ComponentID: target.ComponentID}})
	modules := []assemblycontract.ModuleDescriptorV1{moduleV1(t, source, "fixture/source-module"), moduleV1(t, target, "fixture/target-module-a"), moduleV1(t, target, "fixture/target-module-b")}
	factories := []assemblycontract.ModuleFactoryDescriptorV1{factoryV1(modules[0], "fixture/source-factory", "fixture/source-capability"), factoryV1(modules[1], "fixture/target-factory-a", "fixture/target-capability-a"), factoryV1(modules[2], "fixture/target-factory-b", "fixture/target-capability-b")}
	return assemblycontract.AssemblyManifestV1{ComponentManifests: []runtimeports.ComponentManifestV2{source, target}, Modules: modules, Factories: factories}, assemblycontract.CompiledHarnessGraphV1{Digest: digestCoreV1("graph")}, assemblycontract.AssemblyGenerationV1{GenerationID: "fixture-generation", Revision: 1}
}

func componentV1(id string, dependencies []runtimeports.ComponentDependencyV2) runtimeports.ComponentManifestV2 {
	return runtimeports.ComponentManifestV2{ContractVersion: runtimeports.BindingContractVersionV2, ComponentID: runtimeports.ComponentIDV2(id), Kind: "fixture/component", GovernanceCategory: "fixture/execution", SemanticVersion: "1.0.0", ArtifactDigest: digestCoreV1("artifact:" + id), Contract: runtimeports.ContractBindingV2{Name: "fixture/contract", Version: "1.0.0", Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}, Locality: runtimeports.LocalityHostControlPlane, Dependencies: dependencies, ProvidedCapabilities: []runtimeports.ProvidedCapabilityV2{{Capability: runtimeports.CapabilityNameV2(id + "-execute"), TTLSeconds: 300}}, Conformance: runtimeports.ConformanceFullyControlled, ResidualClass: runtimeports.ResidualNone, Owners: []runtimeports.OwnerAssignmentV2{{Role: runtimeports.OwnerEffect, OwnerComponentID: runtimeports.ComponentIDV2(id)}, {Role: runtimeports.OwnerSettlement, OwnerComponentID: runtimeports.ComponentIDV2(id)}, {Role: runtimeports.OwnerCleanup, OwnerComponentID: runtimeports.ComponentIDV2(id)}}, OfflinePolicy: runtimeports.OfflineDenied}
}

func moduleV1(t *testing.T, component runtimeports.ComponentManifestV2, id string) assemblycontract.ModuleDescriptorV1 {
	t.Helper()
	digest, err := component.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	return assemblycontract.ModuleDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, ModuleID: id, Namespace: "fixture", SemanticVersion: "1.0.0", ArtifactDigest: component.ArtifactDigest, PublisherRef: objectRefV1("publisher:" + id), SourceRef: objectRefV1("source:" + id), ComponentManifestRef: assemblycontract.ObjectRefV1{ID: string(component.ComponentID), Revision: 1, Digest: digest}, Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, Locality: component.Locality, ResidualClass: component.ResidualClass, Owners: component.Owners}
}

func factoryV1(module assemblycontract.ModuleDescriptorV1, id string, capability runtimeports.CapabilityNameV2) assemblycontract.ModuleFactoryDescriptorV1 {
	return assemblycontract.ModuleFactoryDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, FactoryID: id, ModuleRef: module.ModuleID, ArtifactDigest: module.ArtifactDigest, ConstructionMode: assemblycontract.ConstructionTrustedInProcessGoV1, InputSchema: schemaV1("factory-input"), OutputCapability: capability, Lifecycle: assemblycontract.LifecycleGenerationV1, CleanupContractRef: assemblycontract.CleanupContractRefV1{Ref: objectRefV1("cleanup:" + id), OwnerCapability: capability, RequestSchema: schemaV1("cleanup-request"), ResultSchema: schemaV1("cleanup-result")}, TrustRef: objectRefV1("trust:" + id)}
}

func dependencyV1(from, to string, required bool) assemblycontract.DependencySpecV1 {
	return assemblycontract.DependencySpecV1{ContractVersion: assemblycontract.ContractVersionV1, FromRef: from, ToRef: to, Relation: "depends-on", Required: required, VersionRange: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, FailureMode: "fail-closed"}
}
func objectRefV1(id string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: digestCoreV1(id)}
}
func schemaV1(name string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "fixture", Name: name, Version: "1.0.0", MediaType: "application/json", ContentDigest: digestCoreV1("schema:" + name)}
}
func digestCoreV1(value string) core.Digest {
	digest, _ := core.CanonicalJSONDigest("fixture", "v1", "value", value)
	return digest
}
