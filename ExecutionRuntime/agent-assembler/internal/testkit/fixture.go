package testkit

import (
	"context"
	"sort"
	"strings"
	"time"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/repository"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/resolver"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var Now = time.Date(2026, 7, 17, 16, 0, 0, 0, time.UTC)

func Digest(value string) core.Digest { return core.DigestBytes([]byte(value)) }
func Ref(id string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: Digest(id)}
}
func DefinitionRef(id string) definitioncontract.ObjectRefV1 {
	ref := Ref(id)
	return definitioncontract.ObjectRefV1{ID: ref.ID, Revision: ref.Revision, Digest: ref.Digest}
}

type Fixture struct {
	Definition definitioncontract.AgentDefinitionV1
	Catalog    assemblercontract.ComponentReleaseCatalogSnapshotV1
	Facts      assemblercontract.ResolutionFactsSnapshotV1
	Releases   []assemblercontract.ComponentReleaseV1
	Snapshots  *repository.Snapshots
	Plans      *repository.Memory
	Resolver   *resolver.Resolver
	Request    assemblercontract.ResolveRequestV1
}

func NewFixture() Fixture {
	coreKinds := definitioncontract.RequiredCoreKindsV1()
	releases := make([]assemblercontract.ComponentReleaseV1, 0, len(coreKinds))
	registrations := make([]runtimeports.GovernanceRegistrationV2, 0, len(coreKinds))
	components := make([]definitioncontract.ComponentRequirementV1, 0, len(coreKinds))
	for _, kind := range coreKinds {
		name := strings.TrimPrefix(kind, "praxis/")
		componentID := "components/" + name
		capability := "praxis." + name + "/execute"
		contractName := "praxis." + name + "/contract"
		locality := runtimeports.LocalityHostControlPlane
		definitionLocality := definitioncontract.LocalityHostControlPlaneV1
		if kind == "praxis/sandbox" {
			locality = runtimeports.LocalityInstanceDataPlane
			definitionLocality = definitioncontract.LocalityInstanceDataPlaneV1
		}
		manifest := runtimeports.ComponentManifestV2{ContractVersion: runtimeports.BindingContractVersionV2, ComponentID: runtimeports.ComponentIDV2(componentID), Kind: runtimeports.ComponentKindV2(kind), GovernanceCategory: "praxis/core", SemanticVersion: "1.0.0", ArtifactDigest: Digest("artifact-" + name), Contract: runtimeports.ContractBindingV2{Name: runtimeports.NamespacedNameV2(contractName), Version: "1.0.0", Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}, Schemas: []runtimeports.SchemaRefV2{}, Locality: locality, Dependencies: []runtimeports.ComponentDependencyV2{}, RequiredCapabilities: []runtimeports.CapabilityRequirementV2{}, ProvidedCapabilities: []runtimeports.ProvidedCapabilityV2{{Capability: runtimeports.CapabilityNameV2(capability), TTLSeconds: 3600, Schemas: []runtimeports.SchemaRefV2{}}}, Conformance: runtimeports.ConformanceFullyControlled, ResidualClass: runtimeports.ResidualNone, Owners: owners(componentID), Credentials: []runtimeports.CredentialRequirementV2{}, OfflinePolicy: runtimeports.OfflineDenied, Extensions: []runtimeports.GovernanceExtensionV2{}, Annotations: []runtimeports.DisplayAnnotationV2{}}
		module := assemblycontract.ModuleDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, ModuleID: "module/" + name, Namespace: "praxis." + name, SemanticVersion: "1.0.0", ArtifactDigest: manifest.ArtifactDigest, PublisherRef: Ref("publisher-" + name), SourceRef: Ref("module-source-" + name), Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, Capabilities: []runtimeports.CapabilityNameV2{}, Schemas: []runtimeports.SchemaRefV2{}, Locality: locality, ResidualClass: runtimeports.ResidualNone, Owners: owners(componentID), CredentialRequirements: []runtimeports.NamespacedNameV2{}}
		release := assemblercontract.ComponentReleaseV1{ReleaseID: "release-" + name, Revision: 1, SupportMode: assemblercontract.SupportProductionV1, ComponentManifest: manifest, ModuleDescriptors: []assemblycontract.ModuleDescriptorV1{module}, CapabilityDescriptors: []assemblycontract.CapabilityDescriptorV1{}, SlotSpecs: []assemblycontract.SlotSpecV1{}, SlotContributions: []assemblycontract.SlotContributionV1{}, PortSpecs: []assemblycontract.PortSpecV1{}, HookFaces: []assemblycontract.HookFaceSpecV1{}, PhaseContributions: []assemblycontract.PhaseContributionV1{}, Dependencies: []assemblycontract.DependencySpecV1{}, FactoryDescriptors: []assemblycontract.ModuleFactoryDescriptorV1{}, ProviderBindingCandidates: []assemblycontract.ProviderBindingCandidateV1{}, RequiredPlanArtifacts: []assemblercontract.PlanArtifactV1{}, SourceRef: Ref("release-source-" + name), ArtifactDigest: manifest.ArtifactDigest, CertificationRef: Ref("certification-" + name), EvidenceRefs: []assemblycontract.ObjectRefV1{Ref("evidence-" + name)}, CreatedUnixNano: Now.Add(-time.Minute).UnixNano(), ExpiresUnixNano: Now.Add(time.Hour).UnixNano()}
		if kind == "praxis/harness" {
			parts := buildHarnessParts(manifest)
			manifest = parts.manifest
			release.ComponentManifest = manifest
			release.ModuleDescriptors = []assemblycontract.ModuleDescriptorV1{parts.module}
			release.CapabilityDescriptors = parts.capabilities
			release.SlotSpecs = parts.slots
			release.SlotContributions = parts.contributions
			release.PortSpecs = []assemblycontract.PortSpecV1{parts.port}
			release.HookFaces = parts.hooks
			release.FactoryDescriptors = []assemblycontract.ModuleFactoryDescriptorV1{parts.factory}
			release.ProviderBindingCandidates = []assemblycontract.ProviderBindingCandidateV1{parts.provider}
			roles := []assemblercontract.PlanArtifactRoleV1{assemblercontract.ArtifactHarnessBootstrapV1, assemblercontract.ArtifactProfileV1, assemblercontract.ArtifactRuntimePolicyV1, assemblercontract.ArtifactHarnessStackV1, assemblercontract.ArtifactSemanticRouteV1, assemblercontract.ArtifactContextPlanV1, assemblercontract.ArtifactToolSurfaceV1, assemblercontract.ArtifactCapabilityGrantV1, assemblercontract.ArtifactExpectedInjectionV1}
			for _, role := range roles {
				id := string(role)
				if role == assemblercontract.ArtifactProfileV1 {
					id = "profile-selection"
				}
				release.RequiredPlanArtifacts = append(release.RequiredPlanArtifacts, assemblercontract.PlanArtifactV1{Role: role, Ref: Ref(id)})
			}
		}
		if err := PrepareProductionRelease(name, &release); err != nil {
			panic(err)
		}
		manifest = release.ComponentManifest
		release.ComponentManifest = manifest
		sealed, err := SealProductionRelease(release)
		if err != nil {
			panic(err)
		}
		releases = append(releases, sealed)
		caps := make([]runtimeports.CapabilityNameV2, 0, len(manifest.ProvidedCapabilities))
		for _, provided := range manifest.ProvidedCapabilities {
			caps = append(caps, provided.Capability)
		}
		registrations = append(registrations, runtimeports.GovernanceRegistrationV2{Kind: manifest.Kind, Category: manifest.GovernanceCategory, Capabilities: caps, Schemas: manifest.Schemas, ExtensionPolicies: []runtimeports.ExtensionPolicyV2{}, AllowedLocalities: []runtimeports.LocalityV2{locality}, AllowedConformance: []runtimeports.ConformanceLevel{runtimeports.ConformanceFullyControlled}})
		components = append(components, definitioncontract.ComponentRequirementV1{ComponentID: componentID, Kind: kind, SemanticVersion: definitioncontract.VersionRangeV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, ContractName: contractName, ContractVersion: definitioncontract.VersionRangeV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, RequiredCapabilities: []string{capability}, Required: true, SupportMode: definitioncontract.SupportModeProductionV1, LocalityConstraint: definitionLocality, ResidualPolicy: definitioncontract.ResidualPolicyV1{Allowed: false}, DependencyIDs: []string{}})
	}
	governance := runtimeports.GovernanceCatalogV2{Registrations: registrations}
	validation := definitioncontract.ValidationCatalogV1{}
	for _, registration := range registrations {
		validation.Kinds = append(validation.Kinds, string(registration.Kind))
		for _, cap := range registration.Capabilities {
			validation.Capabilities = append(validation.Capabilities, string(cap))
		}
	}
	policy := definitioncontract.PolicyRefsV1{Runtime: DefinitionRef("policy-runtime"), Authority: DefinitionRef("policy-authority"), Review: DefinitionRef("policy-review"), Budget: DefinitionRef("policy-budget"), Sandbox: DefinitionRef("policy-sandbox"), Context: DefinitionRef("policy-context"), Continuity: DefinitionRef("policy-continuity"), ToolMCP: DefinitionRef("policy-tool-mcp"), MemoryKnowledge: DefinitionRef("policy-memory-knowledge")}
	source := definitioncontract.AgentDefinitionSourceV1{ContractVersion: definitioncontract.ContractVersionV1, DefinitionID: "agent/fixture", Revision: 1, IdentityRef: DefinitionRef("identity"), ProfileSelectionRef: DefinitionRef("profile-selection"), Components: components, PolicyRefs: policy, SecretRefs: []definitioncontract.SecretRefV1{}, ProvenanceRef: DefinitionRef("provenance"), ApprovalRef: DefinitionRef("approval"), EffectiveWindow: definitioncontract.EffectiveWindowV1{NotBeforeUnixNano: Now.Add(-time.Minute).UnixNano(), NotAfterUnixNano: Now.Add(time.Hour).UnixNano()}, Extensions: []definitioncontract.ExtensionV1{}, ChangeReason: "assembler production fixture"}
	definition, err := definitioncontract.SealDefinitionV1(source, validation, Now.UnixNano())
	if err != nil {
		panic(err)
	}
	current := []assemblycontract.ObjectRefV1{Ref("identity"), Ref("profile-selection"), Ref("provenance"), Ref("approval"), Ref("policy-runtime"), Ref("policy-authority"), Ref("policy-review"), Ref("policy-budget"), Ref("policy-sandbox"), Ref("policy-context"), Ref("policy-continuity"), Ref("policy-tool-mcp"), Ref("policy-memory-knowledge")}
	facts, err := assemblercontract.SealResolutionFactsV1(assemblercontract.ResolutionFactsSnapshotV1{FactsID: "resolution-facts-fixture", Revision: 1, DefinitionRef: definition.RefV1(), IdentityRef: Ref("identity"), PolicyRefs: current[4:], SandboxRequirementRef: Ref("policy-sandbox"), CurrentFacts: current, RouteBindings: []assemblycontract.ObjectRefV1{Ref("route-binding")}, EvidenceRefs: []assemblycontract.ObjectRefV1{Ref("facts-evidence")}, OwnerRef: "praxis.agent-assembler/resolver", ScopeRef: "tenant/agent-fixture", FrozenUnixNano: Now.UnixNano(), ExpiresUnixNano: Now.Add(time.Hour).UnixNano(), MaximumPriority: 100})
	if err != nil {
		panic(err)
	}
	catalog, err := assemblercontract.SealComponentReleaseCatalogV1(assemblercontract.ComponentReleaseCatalogSnapshotV1{CatalogID: "release-catalog-fixture", Revision: 1, Releases: releases, Governance: governance, CheckedUnixNano: Now.UnixNano(), ExpiresUnixNano: Now.Add(time.Hour).UnixNano()})
	if err != nil {
		panic(err)
	}
	snapshots := repository.NewSnapshots()
	if err := snapshots.PutFacts(facts); err != nil {
		panic(err)
	}
	if err := snapshots.PutCatalog(catalog); err != nil {
		panic(err)
	}
	plans := repository.NewMemory()
	service, err := resolver.New(snapshots, snapshots, plans, func() time.Time { return Now })
	if err != nil {
		panic(err)
	}
	request := assemblercontract.ResolveRequestV1{Definition: definition, FactsRef: facts.RefV1(), CatalogRef: catalog.RefV1()}
	return Fixture{Definition: definition, Catalog: catalog, Facts: facts, Releases: releases, Snapshots: snapshots, Plans: plans, Resolver: service, Request: request}
}

func SealProductionRelease(value assemblercontract.ComponentReleaseV1) (assemblercontract.ComponentReleaseV1, error) {
	digest, err := assemblercontract.ComponentReleaseCertificationDigestV1(value)
	if err != nil {
		return assemblercontract.ComponentReleaseV1{}, err
	}
	value.CertificationRef.Digest = digest
	return assemblercontract.SealComponentReleaseV1(value)
}

func PrepareProductionRelease(name string, value *assemblercontract.ComponentReleaseV1) error {
	CompleteProductionClosure(name, value)
	manifestDigest, err := value.ComponentManifest.BindingDigestV2()
	if err != nil {
		return err
	}
	for index := range value.ModuleDescriptors {
		value.ModuleDescriptors[index].ComponentManifestRef = assemblycontract.ObjectRefV1{ID: string(value.ComponentManifest.ComponentID), Revision: value.Revision, Digest: manifestDigest}
	}
	return nil
}

func CompleteProductionClosure(name string, release *assemblercontract.ComponentReleaseV1) {
	manifest := &release.ComponentManifest
	manifestSchemas := map[string]runtimeports.SchemaRefV2{}
	for _, schema := range manifest.Schemas {
		manifestSchemas[schema.Key()] = schema
	}
	for index := range manifest.ProvidedCapabilities {
		provided := &manifest.ProvidedCapabilities[index]
		if len(provided.Schemas) == 0 {
			provided.Schemas = []runtimeports.SchemaRefV2{Schema(name + "-request"), Schema(name + "-response")}
		}
		for _, schema := range provided.Schemas {
			manifestSchemas[schema.Key()] = schema
		}
	}
	manifest.Schemas = manifest.Schemas[:0]
	for _, schema := range manifestSchemas {
		manifest.Schemas = append(manifest.Schemas, schema)
	}
	sort.Slice(manifest.Schemas, func(i, j int) bool { return manifest.Schemas[i].Key() < manifest.Schemas[j].Key() })

	existingDescriptors := map[runtimeports.CapabilityNameV2]assemblycontract.CapabilityDescriptorV1{}
	for _, descriptor := range release.CapabilityDescriptors {
		if descriptor.Provided {
			existingDescriptors[descriptor.Capability] = descriptor
		}
	}
	capabilities := make([]assemblycontract.CapabilityDescriptorV1, 0, len(manifest.ProvidedCapabilities))
	capabilityNames := make([]runtimeports.CapabilityNameV2, 0, len(manifest.ProvidedCapabilities))
	for _, provided := range manifest.ProvidedCapabilities {
		capabilityNames = append(capabilityNames, provided.Capability)
		ownerCapability := provided.Capability
		effectClass := "none-or-owner-declared"
		if existing, ok := existingDescriptors[provided.Capability]; ok {
			ownerCapability = existing.OwnerCapability
			effectClass = existing.EffectClass
		}
		capabilities = append(capabilities, assemblycontract.CapabilityDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, Capability: provided.Capability, Version: manifest.SemanticVersion, Schemas: append([]runtimeports.SchemaRefV2{}, provided.Schemas...), Provided: true, TTLSeconds: provided.TTLSeconds, EffectClass: effectClass, OwnerCapability: ownerCapability, Conformance: manifest.Conformance})
	}
	sort.Slice(capabilityNames, func(i, j int) bool { return capabilityNames[i] < capabilityNames[j] })
	release.CapabilityDescriptors = capabilities
	module := &release.ModuleDescriptors[0]
	module.Capabilities = capabilityNames
	module.Schemas = append([]runtimeports.SchemaRefV2{}, manifest.Schemas...)
	module.SemanticVersion = manifest.SemanticVersion
	module.ArtifactDigest = manifest.ArtifactDigest
	module.Locality = manifest.Locality
	module.ResidualClass = manifest.ResidualClass
	module.Owners = append([]runtimeports.OwnerAssignmentV2{}, manifest.Owners...)

	descriptorOwner := map[runtimeports.CapabilityNameV2]runtimeports.CapabilityNameV2{}
	for _, descriptor := range capabilities {
		descriptorOwner[descriptor.Capability] = descriptor.OwnerCapability
	}
	filteredFactories := release.FactoryDescriptors[:0]
	for _, factory := range release.FactoryDescriptors {
		if _, exists := descriptorOwner[factory.OutputCapability]; exists {
			filteredFactories = append(filteredFactories, factory)
		}
	}
	release.FactoryDescriptors = filteredFactories
	desiredPortOwner := map[runtimeports.CapabilityNameV2]struct{}{}
	for _, owner := range descriptorOwner {
		desiredPortOwner[owner] = struct{}{}
	}
	filteredPorts := release.PortSpecs[:0]
	for _, port := range release.PortSpecs {
		if _, exists := desiredPortOwner[port.OwnerCapability]; exists {
			filteredPorts = append(filteredPorts, port)
		}
	}
	release.PortSpecs = filteredPorts
	existingFactory := map[runtimeports.CapabilityNameV2]struct{}{}
	for _, factory := range release.FactoryDescriptors {
		existingFactory[factory.OutputCapability] = struct{}{}
	}
	existingPort := map[runtimeports.CapabilityNameV2]struct{}{}
	for _, port := range release.PortSpecs {
		existingPort[port.OwnerCapability] = struct{}{}
	}
	for _, provided := range manifest.ProvidedCapabilities {
		requestSchema := provided.Schemas[0]
		responseSchema := provided.Schemas[len(provided.Schemas)-1]
		ownerCapability := descriptorOwner[provided.Capability]
		if _, exists := existingFactory[provided.Capability]; !exists {
			release.FactoryDescriptors = append(release.FactoryDescriptors, assemblycontract.ModuleFactoryDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, FactoryID: "factory/" + name + "/" + shortCapability(provided.Capability), ModuleRef: module.ModuleID, ArtifactDigest: manifest.ArtifactDigest, ConstructionMode: assemblycontract.ConstructionTrustedInProcessGoV1, InputSchema: requestSchema, OutputCapability: provided.Capability, Lifecycle: assemblycontract.LifecycleGenerationV1, CleanupContractRef: assemblycontract.CleanupContractRefV1{Ref: Ref("cleanup-" + name + "-" + shortCapability(provided.Capability)), OwnerCapability: ownerCapability, RequestSchema: requestSchema, ResultSchema: responseSchema}, TrustRef: Ref("trust-" + name + "-" + shortCapability(provided.Capability))})
		}
		if _, exists := existingPort[ownerCapability]; !exists {
			release.PortSpecs = append(release.PortSpecs, assemblycontract.PortSpecV1{ContractVersion: assemblycontract.ContractVersionV1, PortID: "port/" + name + "/" + shortCapability(provided.Capability), OwnerCapability: ownerCapability, RequestSchema: requestSchema, ResponseSchema: responseSchema, OperationClass: "capability-adapter", Idempotency: "deterministic", FailureSemantics: "explicit-error", Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}})
		}
	}
}

func shortCapability(value runtimeports.CapabilityNameV2) string {
	parts := strings.Split(string(value), "/")
	return strings.ReplaceAll(parts[len(parts)-1], ".", "-")
}

func (f Fixture) Resolve() (assemblercontract.ResolveResultV1, error) {
	return f.Resolver.Resolve(context.Background(), f.Request)
}
func owners(id string) []runtimeports.OwnerAssignmentV2 {
	component := runtimeports.ComponentIDV2(id)
	return []runtimeports.OwnerAssignmentV2{{Role: runtimeports.OwnerEffect, OwnerComponentID: component}, {Role: runtimeports.OwnerSettlement, OwnerComponentID: component}, {Role: runtimeports.OwnerCleanup, OwnerComponentID: component}}
}

type harnessParts struct {
	manifest      runtimeports.ComponentManifestV2
	module        assemblycontract.ModuleDescriptorV1
	capabilities  []assemblycontract.CapabilityDescriptorV1
	slots         []assemblycontract.SlotSpecV1
	contributions []assemblycontract.SlotContributionV1
	port          assemblycontract.PortSpecV1
	hooks         []assemblycontract.HookFaceSpecV1
	factory       assemblycontract.ModuleFactoryDescriptorV1
	provider      assemblycontract.ProviderBindingCandidateV1
}

func buildHarnessParts(manifest runtimeports.ComponentManifestV2) harnessParts {
	slots := assemblycontract.SlotCatalogV1()
	hooks := assemblycontract.HookFaceCatalogV1()
	coreSlots := []string{"kernel.loop", "model.turn", "context.frame", "event.candidate", "runtime.gateway"}
	capabilityNames := map[string]runtimeports.CapabilityNameV2{}
	schemas := []runtimeports.SchemaRefV2{}
	capabilities := []assemblycontract.CapabilityDescriptorV1{}
	for _, id := range coreSlots {
		slot := findSlot(slots, id)
		capability := runtimeports.CapabilityNameV2("praxis.fixture/" + id)
		capabilityNames[id] = capability
		capabilities = append(capabilities, assemblycontract.CapabilityDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, Capability: capability, Version: "1.0.0", Schemas: []runtimeports.SchemaRefV2{slot.InputSchema, slot.OutputSchema}, Provided: true, TTLSeconds: 300, EffectClass: "none-or-owner-declared", OwnerCapability: slot.OwnerCapability, Conformance: runtimeports.ConformanceFullyControlled})
		manifest.ProvidedCapabilities = append(manifest.ProvidedCapabilities, runtimeports.ProvidedCapabilityV2{Capability: capability, TTLSeconds: 300, Schemas: []runtimeports.SchemaRefV2{slot.InputSchema, slot.OutputSchema}})
		schemas = append(schemas, slot.InputSchema, slot.OutputSchema)
	}
	manifest.Schemas = schemas
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		panic(err)
	}
	module := assemblycontract.ModuleDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, ModuleID: "module/harness-core", Namespace: "praxis.harness", SemanticVersion: "1.0.0", ArtifactDigest: manifest.ArtifactDigest, PublisherRef: Ref("publisher-harness"), SourceRef: Ref("source-harness"), ComponentManifestRef: assemblycontract.ObjectRefV1{ID: string(manifest.ComponentID), Revision: 1, Digest: manifestDigest}, Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, Schemas: schemas, Locality: manifest.Locality, ResidualClass: runtimeports.ResidualNone, Owners: manifest.Owners, CredentialRequirements: []runtimeports.NamespacedNameV2{}}
	for _, id := range coreSlots {
		module.Capabilities = append(module.Capabilities, capabilityNames[id])
	}
	modelSlot := findSlot(slots, "model.turn")
	port := assemblycontract.PortSpecV1{ContractVersion: assemblycontract.ContractVersionV1, PortID: "port/model-turn", OwnerCapability: modelSlot.OwnerCapability, RequestSchema: modelSlot.InputSchema, ResponseSchema: modelSlot.OutputSchema, OperationClass: "model-turn", EffectKind: "praxis.model-invoker/model-turn", ConflictDomainRule: "tenant-owner-route-operation-scope", Governance: assemblycontract.GovernanceRequirementsV1{FenceRequired: true, AuthorityRequired: true, ScopeRequired: true, BudgetRequired: true}, Idempotency: "inspect-original-attempt", CancelSupported: true, OperationScopeRef: &assemblycontract.OperationScopeRefV1{Ref: Ref("model-turn-operation-scope"), ScopeKind: assemblycontract.RuntimeOperationScopeKindV1, ScopeDigest: Digest("model-turn-operation-scope")}, InspectContractRef: &assemblycontract.InspectContractRefV1{Ref: Ref("model-turn-inspect-contract"), OwnerCapability: modelSlot.OwnerCapability, RequestSchema: Schema("inspect-request"), ObservationSchema: Schema("inspect-observation")}, DomainResultContractRef: &assemblycontract.DomainResultContractRefV1{Ref: Ref("model-turn-domain-result-contract"), OwnerCapability: modelSlot.OwnerCapability, Schema: modelSlot.OutputSchema}, RuntimeOperationSettlementRefContract: &assemblycontract.RuntimeOperationSettlementRefContractV1{Ref: Ref("runtime-operation-settlement-ref-contract"), RuntimeOwnerCapability: assemblycontract.RuntimeOperationSettlementCapabilityV1, Schema: Schema("runtime-operation-settlement-ref")}, ApplySettlementContractRef: &assemblycontract.ApplySettlementContractRefV1{Ref: Ref("model-turn-apply-settlement-contract"), OwnerCapability: modelSlot.OwnerCapability, RequestSchema: Schema("apply-settlement-request"), ResultSchema: Schema("apply-settlement-result")}, FailureSemantics: "unknown-inspect-original-attempt", Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}}
	provider := assemblycontract.ProviderBindingCandidateV1{ContractVersion: assemblycontract.ContractVersionV1, CandidateID: "provider/model", ModuleRef: module.ModuleID, SlotRef: "model.turn", PortSpecRef: port.PortID, ProviderRef: Ref("model-provider-candidate")}
	provider.Digest, _ = assemblycontract.ProviderBindingCandidateDigestV1(provider)
	contributions := []assemblycontract.SlotContributionV1{{ContributionID: "contribution/kernel-loop", ModuleRef: module.ModuleID, SlotRef: "kernel.loop", Kind: assemblycontract.SlotContributionOwnerV1, CapabilityRef: capabilityNames["kernel.loop"]}, {ContributionID: "contribution/model-turn", ModuleRef: module.ModuleID, SlotRef: "model.turn", Kind: assemblycontract.SlotContributionProviderV1, CapabilityRef: capabilityNames["model.turn"], PortSpecRef: port.PortID, ProviderCandidateRef: provider.CandidateID}, {ContributionID: "contribution/context-frame", ModuleRef: module.ModuleID, SlotRef: "context.frame", Kind: assemblycontract.SlotContributionOwnerV1, CapabilityRef: capabilityNames["context.frame"]}, {ContributionID: "contribution/event-candidate", ModuleRef: module.ModuleID, SlotRef: "event.candidate", Kind: assemblycontract.SlotContributionSourceV1, CapabilityRef: capabilityNames["event.candidate"]}, {ContributionID: "contribution/runtime-gateway", ModuleRef: module.ModuleID, SlotRef: "runtime.gateway", Kind: assemblycontract.SlotContributionReferenceV1, CapabilityRef: capabilityNames["runtime.gateway"]}}
	for index := range contributions {
		contributions[index].ContractVersion = assemblycontract.ContractVersionV1
		contributions[index].Digest, _ = assemblycontract.SlotContributionDigestV1(contributions[index])
	}
	factory := assemblycontract.ModuleFactoryDescriptorV1{ContractVersion: assemblycontract.ContractVersionV1, FactoryID: "factory/harness-core", ModuleRef: module.ModuleID, ArtifactDigest: module.ArtifactDigest, ConstructionMode: assemblycontract.ConstructionTrustedInProcessGoV1, InputSchema: modelSlot.InputSchema, OutputCapability: capabilityNames["model.turn"], Lifecycle: assemblycontract.LifecycleGenerationV1, CleanupContractRef: assemblycontract.CleanupContractRefV1{Ref: Ref("factory-cleanup-contract"), OwnerCapability: modelSlot.OwnerCapability, RequestSchema: Schema("cleanup-request"), ResultSchema: Schema("cleanup-result")}, TrustRef: Ref("factory-trust-ref")}
	return harnessParts{manifest: manifest, module: module, capabilities: capabilities, slots: slots, contributions: contributions, port: port, hooks: hooks, factory: factory, provider: provider}
}

func findSlot(values []assemblycontract.SlotSpecV1, id string) assemblycontract.SlotSpecV1 {
	for _, value := range values {
		if value.SlotID == id {
			return value
		}
	}
	panic("missing slot")
}
func Schema(name string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.assembler", Name: name, Version: "1.0.0", MediaType: "application/json", ContentDigest: Digest("schema:" + name)}
}
