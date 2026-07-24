package blackbox_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/conformance"
	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/repository"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/resolver"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestResolveProductionSevenComponentPlanAndCompileHarness(t *testing.T) {
	fixture := testkit.NewFixture()
	result, err := fixture.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Plan.ComponentReleases) != 7 {
		t.Fatalf("got %d releases", len(result.Plan.ComponentReleases))
	}
	if _, err := conformance.CheckResolveResultV1(result); err != nil {
		t.Fatal(err)
	}
	compiled, err := assemblycompiler.New().Compile(result.AssemblyInput)
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Generation == nil || compiled.Manifest == nil || compiled.Graph == nil || compiled.Handoff == nil {
		t.Fatalf("incomplete compile result: %#v", compiled)
	}
}

func TestNamespacedCustomComponentUsesGenericReleasePath(t *testing.T) {
	fixture := testkit.NewFixture()
	service, request := customized(t, fixture, true, true)
	result, err := service.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Plan.ComponentReleases) != 8 {
		t.Fatalf("custom component not resolved: %d", len(result.Plan.ComponentReleases))
	}
	if _, err := assemblycompiler.New().Compile(result.AssemblyInput); err != nil {
		t.Fatal(err)
	}
}

func TestOptionalCustomComponentIsDeterministicallyPruned(t *testing.T) {
	fixture := testkit.NewFixture()
	service, request := customized(t, fixture, false, false)
	result, err := service.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Plan.ComponentReleases) != 7 || len(result.BindingPlan.Requirements) != 7 {
		t.Fatalf("optional component was not pruned: releases=%d requirements=%d", len(result.Plan.ComponentReleases), len(result.BindingPlan.Requirements))
	}
}

func TestResolveIsDeterministicAndConcurrent(t *testing.T) {
	fixture := testkit.NewFixture()
	first, err := fixture.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	const workers = 64
	results := make(chan string, workers)
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			value, err := fixture.Resolver.Resolve(context.Background(), fixture.Request)
			if err != nil {
				errors <- err
				return
			}
			results <- string(value.Plan.Digest) + "\x00" + string(value.AssemblyInput.Digest)
		}()
	}
	wait.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	expected := string(first.Plan.Digest) + "\x00" + string(first.AssemblyInput.Digest)
	for result := range results {
		if result != expected {
			t.Fatalf("non-deterministic result %q != %q", result, expected)
		}
	}
}

func customized(t *testing.T, fixture testkit.Fixture, withRelease, required bool) (*resolver.Resolver, assemblercontract.ResolveRequestV1) {
	t.Helper()
	catalog := fixture.Catalog
	registration := runtimeports.GovernanceRegistrationV2{Kind: "custom/eighth", Category: "praxis/core", Capabilities: []runtimeports.CapabilityNameV2{"custom.eighth/execute"}, Schemas: []runtimeports.SchemaRefV2{}, ExtensionPolicies: []runtimeports.ExtensionPolicyV2{}, AllowedLocalities: []runtimeports.LocalityV2{runtimeports.LocalityHostControlPlane}, AllowedConformance: []runtimeports.ConformanceLevel{runtimeports.ConformanceFullyControlled}}
	catalog.Governance.Registrations = append(catalog.Governance.Registrations, registration)
	if withRelease {
		release := assemblercontract.CloneComponentReleaseV1(fixture.Releases[0])
		release.ReleaseID = "release-custom-eighth"
		release.ComponentManifest.ComponentID = "custom/eighth"
		release.ComponentManifest.Kind = "custom/eighth"
		release.ComponentManifest.ArtifactDigest = testkit.Digest("custom-artifact")
		release.ComponentManifest.Contract = runtimeports.ContractBindingV2{Name: "custom.eighth/contract", Version: "1.0.0", Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}
		release.ComponentManifest.ProvidedCapabilities = []runtimeports.ProvidedCapabilityV2{{Capability: "custom.eighth/execute", TTLSeconds: 300, Schemas: []runtimeports.SchemaRefV2{}}}
		release.ComponentManifest.Owners = customOwners()
		release.ArtifactDigest = release.ComponentManifest.ArtifactDigest
		release.SourceRef = testkit.Ref("release-source-custom")
		release.CertificationRef = testkit.Ref("certification-custom")
		release.EvidenceRefs = []assemblycontract.ObjectRefV1{testkit.Ref("evidence-custom")}
		release.ModuleDescriptors[0].ModuleID = "module/custom-eighth"
		release.ModuleDescriptors[0].ArtifactDigest = release.ArtifactDigest
		release.ModuleDescriptors[0].Owners = customOwners()
		release.ModuleDescriptors[0].PublisherRef = testkit.Ref("publisher-custom")
		release.ModuleDescriptors[0].SourceRef = testkit.Ref("module-source-custom")
		if prepareErr := testkit.PrepareProductionRelease("custom-eighth", &release); prepareErr != nil {
			t.Fatal(prepareErr)
		}
		sealedRelease, sealErr := testkit.SealProductionRelease(release)
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		release = sealedRelease
		catalog.Releases = append(catalog.Releases, release)
		catalog.Governance.Registrations[len(catalog.Governance.Registrations)-1].Schemas = append([]runtimeports.SchemaRefV2{}, release.ComponentManifest.Schemas...)
	}
	catalog.CatalogID = "catalog/customized"
	var err error
	catalog, err = assemblercontract.SealComponentReleaseCatalogV1(catalog)
	if err != nil {
		t.Fatal(err)
	}
	source := fixture.Definition.AgentDefinitionSourceV1
	source.Components = append(source.Components, definitioncontract.ComponentRequirementV1{ComponentID: "custom/eighth", Kind: "custom/eighth", SemanticVersion: definitioncontract.VersionRangeV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, ContractName: "custom.eighth/contract", ContractVersion: definitioncontract.VersionRangeV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, RequiredCapabilities: []string{"custom.eighth/execute"}, Required: required, SupportMode: definitioncontract.SupportModeProductionV1, LocalityConstraint: definitioncontract.LocalityHostControlPlaneV1, ResidualPolicy: definitioncontract.ResidualPolicyV1{Allowed: false}, DependencyIDs: []string{}})
	validation := definitioncontract.ValidationCatalogV1{}
	for _, entry := range catalog.Governance.Registrations {
		validation.Kinds = append(validation.Kinds, string(entry.Kind))
		for _, capability := range entry.Capabilities {
			validation.Capabilities = append(validation.Capabilities, string(capability))
		}
	}
	definition, err := definitioncontract.SealDefinitionV1(source, validation, testkit.Now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	facts := fixture.Facts
	facts.FactsID = "facts/customized"
	facts.DefinitionRef = definition.RefV1()
	facts, err = assemblercontract.SealResolutionFactsV1(facts)
	if err != nil {
		t.Fatal(err)
	}
	snapshots := repository.NewSnapshots()
	if err = snapshots.PutFacts(facts); err != nil {
		t.Fatal(err)
	}
	if err = snapshots.PutCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	service, err := resolver.New(snapshots, snapshots, repository.NewMemory(), func() time.Time { return testkit.Now })
	if err != nil {
		t.Fatal(err)
	}
	return service, assemblercontract.ResolveRequestV1{Definition: definition, FactsRef: facts.RefV1(), CatalogRef: catalog.RefV1()}
}

func customOwners() []runtimeports.OwnerAssignmentV2 {
	return []runtimeports.OwnerAssignmentV2{{Role: runtimeports.OwnerEffect, OwnerComponentID: "custom/eighth"}, {Role: runtimeports.OwnerSettlement, OwnerComponentID: "custom/eighth"}, {Role: runtimeports.OwnerCleanup, OwnerComponentID: "custom/eighth"}}
}
