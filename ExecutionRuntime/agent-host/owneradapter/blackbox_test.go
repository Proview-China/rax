package owneradapter

import (
	"context"
	"strings"
	"testing"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/repository"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/resolver"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	definitionports "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports"
	definitionstore "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/store"
	hostcontract "github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// This fixture is test-only evidence built exclusively through public structs,
// Seal APIs, repositories and resolver. It is not a production release source.
func TestDefinitionResolveCompilePublicOwnerChain(t *testing.T) {
	now := time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)
	definition, validation, releases, governance := publicOwnerFixtureV1(t, now)
	definitionRepo := definitionstore.NewMemoryRepositoryV1(validation)
	created, err := definitionRepo.CreateDefinitionV1(context.Background(), definitionports.CreateDefinitionRequestV1{Definition: definition})
	if err != nil {
		t.Fatal(err)
	}
	refs := definitionPolicyRefsV1()
	currentFacts := []assemblycontract.ObjectRefV1{assemblyRefV1("identity"), assemblyRefV1("profile-selection"), assemblyRefV1("provenance"), assemblyRefV1("approval"), assemblyRefV1("policy-runtime"), assemblyRefV1("policy-authority"), assemblyRefV1("policy-review"), assemblyRefV1("policy-budget"), assemblyRefV1("policy-sandbox"), assemblyRefV1("policy-context"), assemblyRefV1("policy-continuity"), assemblyRefV1("policy-tool-mcp"), assemblyRefV1("policy-memory-knowledge")}
	facts, err := assemblercontract.SealResolutionFactsV1(assemblercontract.ResolutionFactsSnapshotV1{FactsID: "facts-exact", Revision: 1, DefinitionRef: definition.RefV1(), IdentityRef: assemblyRefV1("identity"), PolicyRefs: currentFacts[4:], SandboxRequirementRef: assemblyRefV1("policy-sandbox"), CurrentFacts: currentFacts, RouteBindings: []assemblycontract.ObjectRefV1{assemblyRefV1("route-binding")}, EvidenceRefs: []assemblycontract.ObjectRefV1{assemblyRefV1("facts-evidence")}, OwnerRef: "praxis.agent-assembler/resolver", ScopeRef: "tenant/agent-fixture", FrozenUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano(), MaximumPriority: 100})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := assemblercontract.SealComponentReleaseCatalogV1(assemblercontract.ComponentReleaseCatalogSnapshotV1{CatalogID: "catalog-exact", Revision: 1, Releases: releases, Governance: governance, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	_ = refs
	snapshots := repository.NewSnapshots()
	if err := snapshots.PutFacts(facts); err != nil {
		t.Fatal(err)
	}
	if err := snapshots.PutCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	plans := repository.NewMemory()
	ownerResolver, err := resolver.New(snapshots, snapshots, plans, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	sourceCurrent, err := hostcontract.SealDefinitionSourceCurrentV1(hostcontract.DefinitionSourceCurrentV1{ContractVersion: hostcontract.ContractVersionV1, ObjectKind: hostcontract.DefinitionSourceCurrentKindV1, SourceStableID: hostConfigV1().DefinitionSourceRef, DefinitionExactRef: definitionRefV1(definition.RefV1()), Revision: 1, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	inputsCurrent, err := hostcontract.SealResolutionInputsCurrentV1(hostcontract.ResolutionInputsCurrentV1{ContractVersion: hostcontract.ContractVersionV1, ObjectKind: hostcontract.ResolutionInputsCurrentKindV1, CatalogStableID: hostConfigV1().CatalogRef, ResolutionFactsStableID: hostConfigV1().ResolutionFactsRef, CatalogExactRef: artifactRefV1(CatalogKindV1, catalog.CatalogID, uint64(catalog.Revision), catalog.Digest), ResolutionFactsExactRef: artifactRefV1(FactsKindV1, facts.FactsID, uint64(facts.Revision), facts.Digest), Revision: 1, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	sourceReader := &sourceReaderStubV1{values: []hostcontract.DefinitionSourceCurrentV1{sourceCurrent, sourceCurrent, sourceCurrent, sourceCurrent}}
	inputReader := &inputsReaderStubV1{values: []hostcontract.ResolutionInputsCurrentV1{inputsCurrent, inputsCurrent, inputsCurrent, inputsCurrent}}
	clock := func() time.Time { return now.Add(time.Second) }
	decoded, err := NewDefinitionAdapterV1(definitionRepo, sourceReader, validation, clock).DecodeDefinitionV1(context.Background(), hostConfigV1())
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := NewAssemblerAdapterV1(definitionRepo, sourceReader, inputReader, ownerResolver, validation, clock).ResolveAgentV1(context.Background(), hostConfigV1(), decoded)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := NewCompilerAdapterV1(plans, snapshots, snapshots, inputReader, clock).CompileHarnessV1(context.Background(), hostConfigV1(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	if err := compiled.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(compiled.Graph.Nodes) < len(definitioncontract.RequiredCoreKindsV1()) {
		t.Fatalf("nodes=%d", len(compiled.Graph.Nodes))
	}
	artifacts, err := NewCompilerAdapterV1(plans, snapshots, snapshots, inputReader, clock).CompileHarnessArtifactsV2(context.Background(), hostConfigV1(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	if err := artifacts.ValidateAt(clock()); err != nil {
		t.Fatal(err)
	}
	if artifacts.Compiled.GenerationRef != compiled.GenerationRef || artifacts.Harness.Generation == nil || artifacts.Harness.Manifest == nil || artifacts.Harness.Graph == nil || artifacts.Harness.Handoff == nil {
		t.Fatal("H3 additive output did not preserve the same complete Harness compile")
	}
	if created.Definition.RefV1() != definition.RefV1() {
		t.Fatal("definition repository exact identity drift")
	}
}

func publicOwnerFixtureV1(t *testing.T, now time.Time) (definitioncontract.AgentDefinitionV1, definitioncontract.ValidationCatalogV1, []assemblercontract.ComponentReleaseV1, runtimeports.GovernanceCatalogV2) {
	t.Helper()
	kinds := definitioncontract.RequiredCoreKindsV1()
	releases := make([]assemblercontract.ComponentReleaseV1, 0, len(kinds))
	registrations := make([]runtimeports.GovernanceRegistrationV2, 0, len(kinds))
	requirements := make([]definitioncontract.ComponentRequirementV1, 0, len(kinds))
	validation := definitioncontract.ValidationCatalogV1{}
	for _, kind := range kinds {
		name := strings.TrimPrefix(kind, "praxis/")
		componentID := "components/" + name
		capability := runtimeports.CapabilityNameV2("praxis." + name + "/execute")
		locality := runtimeports.LocalityHostControlPlane
		definitionLocality := definitioncontract.LocalityHostControlPlaneV1
		if kind == "praxis/sandbox" {
			locality = runtimeports.LocalityInstanceDataPlane
			definitionLocality = definitioncontract.LocalityInstanceDataPlaneV1
		}
		schema := schemaV1(name + "-request")
		manifest := runtimeports.ComponentManifestV2{ContractVersion: runtimeports.BindingContractVersionV2, ComponentID: runtimeports.ComponentIDV2(componentID), Kind: runtimeports.ComponentKindV2(kind), GovernanceCategory: "praxis/core", SemanticVersion: "1.0.0", ArtifactDigest: digestCoreV1("artifact-" + name), Contract: runtimeports.ContractBindingV2{Name: runtimeports.NamespacedNameV2("praxis." + name + "/contract"), Version: "1.0.0", Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}, Schemas: []runtimeports.SchemaRefV2{schema}, Locality: locality, ProvidedCapabilities: []runtimeports.ProvidedCapabilityV2{{Capability: capability, TTLSeconds: 3600, Schemas: []runtimeports.SchemaRefV2{schema}}}, Conformance: runtimeports.ConformanceFullyControlled, ResidualClass: runtimeports.ResidualNone, Owners: graphOwnersV1(componentID), OfflinePolicy: runtimeports.OfflineDenied}
		module := assemblycontract.ModuleDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, ModuleID: "module/" + name, Namespace: "praxis." + name, SemanticVersion: "1.0.0", ArtifactDigest: manifest.ArtifactDigest, PublisherRef: assemblyRefV1("publisher-" + name), SourceRef: assemblyRefV1("module-source-" + name), Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, Capabilities: []runtimeports.CapabilityNameV2{capability}, Schemas: []runtimeports.SchemaRefV2{schema}, Locality: locality, ResidualClass: runtimeports.ResidualNone, Owners: graphOwnersV1(componentID)}
		release := assemblercontract.ComponentReleaseV1{ContractVersion: assemblercontract.ReleaseContractVersionV1, ReleaseID: "release-" + name, Revision: 1, SupportMode: assemblercontract.SupportProductionV1, ComponentManifest: manifest, ModuleDescriptors: []assemblycontract.ModuleDescriptorV1{module}, CapabilityDescriptors: []assemblycontract.CapabilityDescriptorV1{{ContractVersion: assemblycontract.ContractVersionV1, Capability: capability, Version: "1.0.0", Schemas: []runtimeports.SchemaRefV2{schema}, Provided: true, TTLSeconds: 3600, EffectClass: "none-or-owner-declared", OwnerCapability: capability, Conformance: runtimeports.ConformanceFullyControlled}}, PortSpecs: []assemblycontract.PortSpecV1{{ContractVersion: assemblycontract.ContractVersionV1, PortID: "port/" + name, OwnerCapability: capability, RequestSchema: schema, ResponseSchema: schema, OperationClass: "capability-adapter", Idempotency: "deterministic", FailureSemantics: "explicit-error", Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}}, FactoryDescriptors: []assemblycontract.ModuleFactoryDescriptorV1{simpleFactoryV1(module, "factory/"+name, capability, schema)}, SourceRef: assemblyRefV1("release-source-" + name), ArtifactDigest: manifest.ArtifactDigest, CertificationRef: assemblyRefV1("certification-" + name), EvidenceRefs: []assemblycontract.ObjectRefV1{assemblyRefV1("evidence-" + name)}, CreatedUnixNano: now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
		if kind == "praxis/harness" {
			addHarnessDeclarationsV1(&release, &manifest)
			release.ComponentManifest = manifest
			release.ModuleDescriptors[0].Schemas = append([]runtimeports.SchemaRefV2(nil), manifest.Schemas...)
			release.ModuleDescriptors[0].Capabilities = manifestCapabilitiesV1(manifest)
			for _, role := range []assemblercontract.PlanArtifactRoleV1{assemblercontract.ArtifactHarnessBootstrapV1, assemblercontract.ArtifactProfileV1, assemblercontract.ArtifactRuntimePolicyV1, assemblercontract.ArtifactHarnessStackV1, assemblercontract.ArtifactSemanticRouteV1, assemblercontract.ArtifactContextPlanV1, assemblercontract.ArtifactToolSurfaceV1, assemblercontract.ArtifactCapabilityGrantV1, assemblercontract.ArtifactExpectedInjectionV1} {
				id := string(role)
				if role == assemblercontract.ArtifactProfileV1 {
					id = "profile-selection"
				}
				release.RequiredPlanArtifacts = append(release.RequiredPlanArtifacts, assemblercontract.PlanArtifactV1{Role: role, Ref: assemblyRefV1(id)})
			}
		}
		manifestDigest, err := release.ComponentManifest.BindingDigestV2()
		if err != nil {
			t.Fatal(err)
		}
		release.ModuleDescriptors[0].ComponentManifestRef = assemblycontract.ObjectRefV1{ID: componentID, Revision: 1, Digest: manifestDigest}
		certification, err := assemblercontract.ComponentReleaseCertificationDigestV1(release)
		if err != nil {
			t.Fatal(err)
		}
		release.CertificationRef.Digest = certification
		release, err = assemblercontract.SealComponentReleaseV1(release)
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
		caps := manifestCapabilitiesV1(release.ComponentManifest)
		registrations = append(registrations, runtimeports.GovernanceRegistrationV2{Kind: release.ComponentManifest.Kind, Category: release.ComponentManifest.GovernanceCategory, Capabilities: caps, Schemas: release.ComponentManifest.Schemas, AllowedLocalities: []runtimeports.LocalityV2{locality}, AllowedConformance: []runtimeports.ConformanceLevel{runtimeports.ConformanceFullyControlled}})
		validation.Kinds = append(validation.Kinds, kind)
		for _, cap := range caps {
			validation.Capabilities = append(validation.Capabilities, string(cap))
		}
		requirements = append(requirements, definitioncontract.ComponentRequirementV1{ComponentID: componentID, Kind: kind, SemanticVersion: definitioncontract.VersionRangeV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, ContractName: "praxis." + name + "/contract", ContractVersion: definitioncontract.VersionRangeV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, RequiredCapabilities: []string{string(capability)}, Required: true, SupportMode: definitioncontract.SupportModeProductionV1, LocalityConstraint: definitionLocality, ResidualPolicy: definitioncontract.ResidualPolicyV1{Allowed: false}})
	}
	source := definitioncontract.AgentDefinitionSourceV1{ContractVersion: definitioncontract.ContractVersionV1, DefinitionID: "agent/fixture", Revision: 1, IdentityRef: definitionRefObjectV1("identity"), ProfileSelectionRef: definitionRefObjectV1("profile-selection"), Components: requirements, PolicyRefs: definitionPolicyRefsV1(), ProvenanceRef: definitionRefObjectV1("provenance"), ApprovalRef: definitionRefObjectV1("approval"), EffectiveWindow: definitioncontract.EffectiveWindowV1{NotBeforeUnixNano: now.Add(-time.Minute).UnixNano(), NotAfterUnixNano: now.Add(time.Hour).UnixNano()}, ChangeReason: "public Host blackbox fixture"}
	definition, err := definitioncontract.SealDefinitionV1(source, validation, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	return definition, validation, releases, runtimeports.GovernanceCatalogV2{Registrations: registrations}
}

func addHarnessDeclarationsV1(release *assemblercontract.ComponentReleaseV1, manifest *runtimeports.ComponentManifestV2) {
	slots := assemblycontract.SlotCatalogV1()
	hooks := assemblycontract.HookFaceCatalogV1()
	module := &release.ModuleDescriptors[0]
	release.SlotSpecs = slots
	release.HookFaces = hooks
	for _, id := range []string{"kernel.loop", "model.turn", "context.frame", "event.candidate", "runtime.gateway"} {
		slot := findAssemblySlotV1(slots, id)
		cap := runtimeports.CapabilityNameV2("praxis.fixture/" + id)
		manifest.Schemas = append(manifest.Schemas, slot.InputSchema, slot.OutputSchema)
		manifest.ProvidedCapabilities = append(manifest.ProvidedCapabilities, runtimeports.ProvidedCapabilityV2{Capability: cap, TTLSeconds: 300, Schemas: []runtimeports.SchemaRefV2{slot.InputSchema, slot.OutputSchema}})
		release.CapabilityDescriptors = append(release.CapabilityDescriptors, assemblycontract.CapabilityDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, Capability: cap, Version: "1.0.0", Schemas: []runtimeports.SchemaRefV2{slot.InputSchema, slot.OutputSchema}, Provided: true, TTLSeconds: 300, EffectClass: "none-or-owner-declared", OwnerCapability: slot.OwnerCapability, Conformance: runtimeports.ConformanceFullyControlled})
		release.PortSpecs = append(release.PortSpecs, assemblycontract.PortSpecV1{ContractVersion: assemblycontract.ContractVersionV1, PortID: "port/harness-" + strings.ReplaceAll(id, ".", "-"), OwnerCapability: slot.OwnerCapability, RequestSchema: slot.InputSchema, ResponseSchema: slot.OutputSchema, OperationClass: "capability-adapter", Idempotency: "deterministic", FailureSemantics: "explicit-error", Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}})
		kind := assemblycontract.SlotContributionOwnerV1
		switch id {
		case "model.turn", "runtime.gateway":
			kind = assemblycontract.SlotContributionReferenceV1
		case "event.candidate":
			kind = assemblycontract.SlotContributionSourceV1
		}
		contribution := assemblycontract.SlotContributionV1{ContractVersion: assemblycontract.ContractVersionV1, ContributionID: "contribution/" + strings.ReplaceAll(id, ".", "-"), ModuleRef: module.ModuleID, SlotRef: id, Kind: kind, CapabilityRef: cap}
		contribution.Digest, _ = assemblycontract.SlotContributionDigestV1(contribution)
		release.SlotContributions = append(release.SlotContributions, contribution)
		factory := simpleFactoryV1(*module, "factory/harness-"+strings.ReplaceAll(id, ".", "-"), cap, slot.InputSchema)
		factory.CleanupContractRef.OwnerCapability = slot.OwnerCapability
		release.FactoryDescriptors = append(release.FactoryDescriptors, factory)
	}
}

func simpleFactoryV1(module assemblycontract.ModuleDescriptorV1, id string, cap runtimeports.CapabilityNameV2, schema runtimeports.SchemaRefV2) assemblycontract.ModuleFactoryDescriptorV1 {
	return assemblycontract.ModuleFactoryDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, FactoryID: id, ModuleRef: module.ModuleID, ArtifactDigest: module.ArtifactDigest, ConstructionMode: assemblycontract.ConstructionTrustedInProcessGoV1, InputSchema: schema, OutputCapability: cap, Lifecycle: assemblycontract.LifecycleGenerationV1, CleanupContractRef: assemblycontract.CleanupContractRefV1{Ref: assemblyRefV1("cleanup-" + id), OwnerCapability: cap, RequestSchema: schema, ResultSchema: schema}, TrustRef: assemblyRefV1("trust-" + id)}
}
func manifestCapabilitiesV1(m runtimeports.ComponentManifestV2) []runtimeports.CapabilityNameV2 {
	result := make([]runtimeports.CapabilityNameV2, 0, len(m.ProvidedCapabilities))
	for _, value := range m.ProvidedCapabilities {
		result = append(result, value.Capability)
	}
	return result
}
func graphOwnersV1(id string) []runtimeports.OwnerAssignmentV2 {
	component := runtimeports.ComponentIDV2(id)
	return []runtimeports.OwnerAssignmentV2{{Role: runtimeports.OwnerEffect, OwnerComponentID: component}, {Role: runtimeports.OwnerSettlement, OwnerComponentID: component}, {Role: runtimeports.OwnerCleanup, OwnerComponentID: component}}
}
func findAssemblySlotV1(values []assemblycontract.SlotSpecV1, id string) assemblycontract.SlotSpecV1 {
	for _, value := range values {
		if value.SlotID == id {
			return value
		}
	}
	panic("missing slot")
}
func assemblyRefV1(id string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: digestCoreV1(id)}
}
func definitionRefObjectV1(id string) definitioncontract.ObjectRefV1 {
	ref := assemblyRefV1(id)
	return definitioncontract.ObjectRefV1{ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest}
}
func definitionPolicyRefsV1() definitioncontract.PolicyRefsV1 {
	return definitioncontract.PolicyRefsV1{Runtime: definitionRefObjectV1("policy-runtime"), Authority: definitionRefObjectV1("policy-authority"), Review: definitionRefObjectV1("policy-review"), Budget: definitionRefObjectV1("policy-budget"), Sandbox: definitionRefObjectV1("policy-sandbox"), Context: definitionRefObjectV1("policy-context"), Continuity: definitionRefObjectV1("policy-continuity"), ToolMCP: definitionRefObjectV1("policy-tool-mcp"), MemoryKnowledge: definitionRefObjectV1("policy-memory-knowledge")}
}
