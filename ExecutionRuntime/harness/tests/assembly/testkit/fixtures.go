package testkit

import (
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	basetestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var Now = time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

func Digest(value any) core.Digest { return basetestkit.Digest(value) }

func Ref(id string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: Digest(id)}
}

func Schema(name string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.harness.assembly", Name: name, Version: "1.0.0", MediaType: "application/json", ContentDigest: Digest("schema:" + name)}
}

func ValidInput() assemblycontract.AssemblyInputV1 {
	base := basetestkit.Manifest(Now, runtimeports.ConformanceFullyControlled)
	manifest := basetestkit.BindingManifest(base)
	manifest.ResidualClass = runtimeports.ResidualNone
	slots := assemblycontract.SlotCatalogV1()
	hooks := assemblycontract.HookFaceCatalogV1()
	coreSlots := []string{"kernel.loop", "model.turn", "context.frame", "event.candidate", "runtime.gateway"}
	capabilityNames := map[string]runtimeports.CapabilityNameV2{}
	schemas := []runtimeports.SchemaRefV2{}
	capabilities := []assemblycontract.CapabilityDescriptorV1{}
	for _, id := range coreSlots {
		var slot assemblycontract.SlotSpecV1
		for _, candidate := range slots {
			if candidate.SlotID == id {
				slot = candidate
				break
			}
		}
		capability := runtimeports.CapabilityNameV2("praxis.fixture/" + id)
		capabilityNames[id] = capability
		capabilities = append(capabilities, assemblycontract.CapabilityDescriptorV1{Capability: capability, Version: "1.0.0", Schemas: []runtimeports.SchemaRefV2{slot.InputSchema, slot.OutputSchema}, Provided: true, TTLSeconds: 300, EffectClass: "none-or-owner-declared", OwnerCapability: slot.OwnerCapability, Conformance: runtimeports.ConformanceFullyControlled})
		manifest.ProvidedCapabilities = append(manifest.ProvidedCapabilities, runtimeports.ProvidedCapabilityV2{Capability: capability, TTLSeconds: 300, Schemas: []runtimeports.SchemaRefV2{slot.InputSchema, slot.OutputSchema}})
		schemas = append(schemas, slot.InputSchema, slot.OutputSchema)
	}
	manifest.Schemas = append([]runtimeports.SchemaRefV2(nil), schemas...)
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		panic(err)
	}
	owners := append([]runtimeports.OwnerAssignmentV2(nil), manifest.Owners...)
	module := assemblycontract.ModuleDescriptorV1{ModuleID: "praxis.fixture/core-module", Namespace: "praxis.fixture", SemanticVersion: "1.0.0", ArtifactDigest: manifest.ArtifactDigest, PublisherRef: Ref("publisher-fixture"), SourceRef: Ref("source-fixture"), ComponentManifestRef: assemblycontract.ObjectRefV1{ID: string(manifest.ComponentID), Revision: 1, Digest: manifestDigest}, Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, Schemas: schemas, Locality: runtimeports.LocalityHostControlPlane, ResidualClass: runtimeports.ResidualNone, Owners: owners}
	for _, id := range coreSlots {
		module.Capabilities = append(module.Capabilities, capabilityNames[id])
	}

	modelSlot := findSlot(slots, "model.turn")
	modelPort := assemblycontract.PortSpecV1{
		PortID: "praxis.fixture/model-turn-port", OwnerCapability: modelSlot.OwnerCapability, RequestSchema: modelSlot.InputSchema, ResponseSchema: modelSlot.OutputSchema,
		OperationClass: "model-turn", EffectKind: "praxis.model-invoker/model-turn", ConflictDomainRule: "tenant-owner-route-operation-scope",
		Governance:  assemblycontract.GovernanceRequirementsV1{FenceRequired: true, AuthorityRequired: true, ScopeRequired: true, BudgetRequired: true},
		Idempotency: "inspect-original-attempt", CancelSupported: true,
		OperationScopeRef:                     &assemblycontract.OperationScopeRefV1{Ref: Ref("model-turn-operation-scope"), ScopeKind: assemblycontract.RuntimeOperationScopeKindV1, ScopeDigest: Digest("model-turn-operation-scope")},
		InspectContractRef:                    &assemblycontract.InspectContractRefV1{Ref: Ref("model-turn-inspect-contract"), OwnerCapability: modelSlot.OwnerCapability, RequestSchema: Schema("inspect-request"), ObservationSchema: Schema("inspect-observation")},
		DomainResultContractRef:               &assemblycontract.DomainResultContractRefV1{Ref: Ref("model-turn-domain-result-contract"), OwnerCapability: modelSlot.OwnerCapability, Schema: modelSlot.OutputSchema},
		RuntimeOperationSettlementRefContract: &assemblycontract.RuntimeOperationSettlementRefContractV1{Ref: Ref("runtime-operation-settlement-ref-contract"), RuntimeOwnerCapability: assemblycontract.RuntimeOperationSettlementCapabilityV1, Schema: Schema("runtime-operation-settlement-ref")},
		ApplySettlementContractRef:            &assemblycontract.ApplySettlementContractRefV1{Ref: Ref("model-turn-apply-settlement-contract"), OwnerCapability: modelSlot.OwnerCapability, RequestSchema: Schema("apply-settlement-request"), ResultSchema: Schema("apply-settlement-result")},
		FailureSemantics:                      "unknown-inspect-original-attempt", Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"},
	}
	provider := assemblycontract.ProviderBindingCandidateV1{CandidateID: "praxis.fixture/model-provider", ModuleRef: module.ModuleID, SlotRef: "model.turn", PortSpecRef: modelPort.PortID, ProviderRef: Ref("model-provider-candidate")}
	contributions := []assemblycontract.SlotContributionV1{
		{ContributionID: "praxis.fixture/kernel-loop", ModuleRef: module.ModuleID, SlotRef: "kernel.loop", Kind: assemblycontract.SlotContributionOwnerV1, CapabilityRef: capabilityNames["kernel.loop"]},
		{ContributionID: "praxis.fixture/model-turn", ModuleRef: module.ModuleID, SlotRef: "model.turn", Kind: assemblycontract.SlotContributionProviderV1, CapabilityRef: capabilityNames["model.turn"], PortSpecRef: modelPort.PortID, ProviderCandidateRef: provider.CandidateID},
		{ContributionID: "praxis.fixture/context-frame", ModuleRef: module.ModuleID, SlotRef: "context.frame", Kind: assemblycontract.SlotContributionOwnerV1, CapabilityRef: capabilityNames["context.frame"]},
		{ContributionID: "praxis.fixture/event-candidate", ModuleRef: module.ModuleID, SlotRef: "event.candidate", Kind: assemblycontract.SlotContributionSourceV1, CapabilityRef: capabilityNames["event.candidate"]},
		{ContributionID: "praxis.fixture/runtime-gateway", ModuleRef: module.ModuleID, SlotRef: "runtime.gateway", Kind: assemblycontract.SlotContributionReferenceV1, CapabilityRef: capabilityNames["runtime.gateway"]},
	}
	factory := assemblycontract.ModuleFactoryDescriptorV1{FactoryID: "praxis.fixture/core-factory", ModuleRef: module.ModuleID, ArtifactDigest: module.ArtifactDigest, ConstructionMode: assemblycontract.ConstructionTrustedInProcessGoV1, InputSchema: modelSlot.InputSchema, OutputCapability: capabilityNames["model.turn"], Lifecycle: assemblycontract.LifecycleGenerationV1, CleanupContractRef: assemblycontract.CleanupContractRefV1{Ref: Ref("factory-cleanup-contract"), OwnerCapability: modelSlot.OwnerCapability, RequestSchema: Schema("cleanup-request"), ResultSchema: Schema("cleanup-result")}, TrustRef: Ref("factory-trust-ref")}
	input := assemblycontract.AssemblyInputV1{InputID: "assembly-input-fixture", Revision: 1, OwnerRef: "praxis.harness/assembly", ScopeRef: "tenant-fixture/agent-fixture", CreatedUnixNano: Now.UnixNano(), Plan: assemblycontract.AssemblyPlanRefsV1{ResolvedAgentPlan: Ref("resolved-agent-plan"), HarnessBootstrapPlan: Ref("harness-bootstrap-plan"), Profile: Ref("profile"), RuntimePolicy: Ref("runtime-policy"), HarnessStack: Ref("harness-stack"), SemanticRoute: Ref("semantic-route"), ContextPlan: Ref("context-plan"), ToolSurface: Ref("tool-surface"), CapabilityGrant: Ref("capability-grant"), ExpectedInjectionManifest: Ref("expected-injection-manifest")}, CurrentFacts: []assemblycontract.ObjectRefV1{Ref("runtime-policy-current")}, RouteBindings: []assemblycontract.ObjectRefV1{Ref("route-binding")}, ComponentManifests: []runtimeports.ComponentManifestV2{manifest}, Modules: []assemblycontract.ModuleDescriptorV1{module}, Capabilities: capabilities, Slots: slots, SlotContributions: contributions, PortSpecs: []assemblycontract.PortSpecV1{modelPort}, HookFaces: hooks, Factories: []assemblycontract.ModuleFactoryDescriptorV1{factory}, ProviderBindingCandidates: []assemblycontract.ProviderBindingCandidateV1{provider}, Policy: assemblycontract.AssemblyPolicyV1{MaximumPriority: 100}, EvidenceRefs: []assemblycontract.ObjectRefV1{Ref("upstream-evidence")}}
	sealed, err := assemblycontract.SealAssemblyInputV1(input)
	if err != nil {
		panic(err)
	}
	return sealed
}

func findSlot(values []assemblycontract.SlotSpecV1, id string) assemblycontract.SlotSpecV1 {
	for _, value := range values {
		if value.SlotID == id {
			return value
		}
	}
	panic("missing slot")
}

func RuntimeBindingRef() runtimeports.ProviderBindingRefV2 {
	return runtimeports.ProviderBindingRefV2{BindingSetID: "binding-set-fixture", BindingSetRevision: 2, ComponentID: "praxis/harness-test", ManifestDigest: Digest("binding-manifest"), ArtifactDigest: Digest("binding-artifact"), Capability: "praxis/harness-execution"}
}
