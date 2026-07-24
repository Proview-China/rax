package assemblyadapter

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func routeDigestV2(value string) core.Digest { return core.DigestBytes([]byte(value)) }

func routeSchemaV2(name string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.route", Name: name, Version: "2.0.0", MediaType: "application/json", ContentDigest: routeDigestV2("schema:" + name)}
}

func routeObjectRefV2(id string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: routeDigestV2(id)}
}

func routeEndpointV2(role assemblycontract.ControlledOperationProviderRouteRoleV2, component string, capability runtimeports.CapabilityNameV2) assemblycontract.ControlledOperationProviderRouteEndpointV2 {
	return assemblycontract.ControlledOperationProviderRouteEndpointV2{
		Role: role, ComponentID: runtimeports.ComponentIDV2(component), ManifestDigest: routeDigestV2(component + ":manifest"), ArtifactDigest: routeDigestV2(component + ":artifact"),
		Capability: capability, ContractVersion: "2.0.0", Locality: runtimeports.LocalityHostControlPlane,
		CandidateID: component + "/candidate", CandidateDigest: routeDigestV2(component + ":candidate"), ModuleRef: component + "/module", PortSpecRef: component + "/port", ProviderRef: routeObjectRefV2(component + "/provider"),
	}
}

func routeReaderV2(role assemblycontract.ControlledOperationProviderRouteRoleV2, component string, capability runtimeports.CapabilityNameV2) assemblycontract.ControlledOperationProviderRouteReaderRefV2 {
	schemaName := strings.NewReplacer("/", "-", ".", "-").Replace(component)
	return assemblycontract.ControlledOperationProviderRouteReaderRefV2{
		Role: role, ComponentID: runtimeports.ComponentIDV2(component), ManifestDigest: routeDigestV2(component + ":manifest"), ArtifactDigest: routeDigestV2(component + ":artifact"),
		Capability: capability, PortSpecID: component + "/port", PortSpecDigest: routeDigestV2(component + ":port"), RequestSchema: routeSchemaV2(schemaName + "-request"), ProjectionSchema: routeSchemaV2(schemaName + "-projection"), ReadOnly: true, NoExecute: true,
	}
}

func routeDeclarationV2(t *testing.T) assemblycontract.ControlledOperationProviderRouteDeclarationV2 {
	t.Helper()
	appRequest, appResponse := routeSchemaV2("application-request"), routeSchemaV2("application-response")
	runtimeRequest, runtimeResponse := routeSchemaV2("runtime-request"), routeSchemaV2("runtime-response")
	value, err := assemblycontract.SealControlledOperationProviderRouteDeclarationV2(assemblycontract.ControlledOperationProviderRouteDeclarationV2{
		RouteID: "praxis.route/single-tool-action", Revision: 1, PublisherComponent: "praxis.harness/assembly",
		Matrix:                runtimeports.OperationScopeEvidenceActionMatrixV3(),
		ApplicationToolPort:   assemblycontract.ControlledOperationProviderRoutePortRefV2{Role: assemblycontract.ControlledOperationApplicationToolPortRoleV2, PortID: "praxis.application/single-tool-action", PortDigest: routeDigestV2("application-port"), OwnerCapability: "praxis.application/single-tool-action", RequestSchema: appRequest, ResponseSchema: appResponse, ContractVersion: "1.0.0"},
		ToolAdapter:           routeEndpointV2(assemblycontract.ControlledOperationToolAdapterRoleV2, "praxis.tool/adapter", runtimeports.ControlledOperationToolAdapterCapabilityV2),
		RuntimeGovernancePort: assemblycontract.ControlledOperationProviderRoutePortRefV2{Role: assemblycontract.ControlledOperationRuntimeGovernanceRoleV2, PortID: "praxis.runtime/controlled-provider", PortDigest: routeDigestV2("runtime-port"), OwnerCapability: runtimeports.ControlledOperationGatewayCapabilityV2, RequestSchema: runtimeRequest, ResponseSchema: runtimeResponse, ContractVersion: "2.0.0"},
		Gateway:               routeEndpointV2(assemblycontract.ControlledOperationRuntimeGatewayRoleV2, "praxis.runtime/gateway", runtimeports.ControlledOperationGatewayCapabilityV2),
		ProviderTransport:     routeEndpointV2(assemblycontract.ControlledOperationProviderTransportRoleV2, "praxis.tool/transport", runtimeports.ControlledOperationProviderTransportCapabilityV2),
		Provider:              routeEndpointV2(assemblycontract.ControlledOperationProviderRoleV2, "praxis.provider/tool", runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3)),
		PreparedCurrentReader: routeReaderV2(assemblycontract.ControlledOperationPreparedReaderRoleV2, "praxis.runtime/prepared-reader", runtimeports.ControlledOperationPreparedReaderCapabilityV2),
		BoundaryCurrentReader: routeReaderV2(assemblycontract.ControlledOperationBoundaryReaderRoleV2, "praxis.runtime/boundary-reader", runtimeports.ControlledOperationBoundaryReaderCapabilityV2),
		ProviderInspectReader: routeReaderV2(assemblycontract.ControlledOperationProviderInspectRoleV2, "praxis.tool/provider-inspect", runtimeports.ControlledOperationProviderInspectCapabilityV2),
		ActiveBindingPolicy:   assemblycontract.ControlledOperationProviderActiveBindingPolicyV2, BypassPolicy: assemblycontract.ControlledOperationProviderBypassPolicyV2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func routeBindingsV2(declaration assemblycontract.ControlledOperationProviderRouteDeclarationV2) [7]runtimeports.ProviderBindingRefV2 {
	targets := []struct {
		component runtimeports.ComponentIDV2
		manifest  core.Digest
		artifact  core.Digest
		cap       runtimeports.CapabilityNameV2
	}{
		{declaration.ToolAdapter.ComponentID, declaration.ToolAdapter.ManifestDigest, declaration.ToolAdapter.ArtifactDigest, runtimeports.ControlledOperationToolAdapterCapabilityV2},
		{declaration.Gateway.ComponentID, declaration.Gateway.ManifestDigest, declaration.Gateway.ArtifactDigest, runtimeports.ControlledOperationGatewayCapabilityV2},
		{declaration.ProviderTransport.ComponentID, declaration.ProviderTransport.ManifestDigest, declaration.ProviderTransport.ArtifactDigest, runtimeports.ControlledOperationProviderTransportCapabilityV2},
		{declaration.PreparedCurrentReader.ComponentID, declaration.PreparedCurrentReader.ManifestDigest, declaration.PreparedCurrentReader.ArtifactDigest, runtimeports.ControlledOperationPreparedReaderCapabilityV2},
		{declaration.BoundaryCurrentReader.ComponentID, declaration.BoundaryCurrentReader.ManifestDigest, declaration.BoundaryCurrentReader.ArtifactDigest, runtimeports.ControlledOperationBoundaryReaderCapabilityV2},
		{declaration.ProviderInspectReader.ComponentID, declaration.ProviderInspectReader.ManifestDigest, declaration.ProviderInspectReader.ArtifactDigest, runtimeports.ControlledOperationProviderInspectCapabilityV2},
		{declaration.Provider.ComponentID, declaration.Provider.ManifestDigest, declaration.Provider.ArtifactDigest, runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3)},
	}
	var result [7]runtimeports.ProviderBindingRefV2
	for index, target := range targets {
		result[index] = runtimeports.ProviderBindingRefV2{BindingSetID: "binding-set-route", BindingSetRevision: 1, ComponentID: target.component, ManifestDigest: target.manifest, ArtifactDigest: target.artifact, Capability: target.cap}
	}
	return result
}

func routeActiveIdentityV2(endpoint assemblycontract.ControlledOperationProviderRouteEndpointV2) assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2 {
	return assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{
		ProviderRef: endpoint.ProviderRef, CandidateID: endpoint.CandidateID, ModuleRef: endpoint.ModuleRef,
		ComponentID: endpoint.ComponentID, ComponentManifestDigest: endpoint.ManifestDigest, ArtifactDigest: endpoint.ArtifactDigest,
		Capability: endpoint.Capability, PortSpecRef: endpoint.PortSpecRef, PortSpecDigest: routeDigestV2(endpoint.PortSpecRef), ConflictDomain: "route:" + string(endpoint.Capability),
	}
}

func routeInventoryV2(t *testing.T, now time.Time, declaration assemblycontract.ControlledOperationProviderRouteDeclarationV2, bindings [7]runtimeports.ProviderBindingRefV2) assemblycontract.ControlledOperationProviderRouteWiringInventoryV2 {
	t.Helper()
	value, err := assemblycontract.SealControlledOperationProviderRouteWiringInventoryV2(assemblycontract.ControlledOperationProviderRouteWiringInventoryV2{
		InventoryID: "route-wiring-inventory", Revision: 1, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
		Edges: []assemblycontract.ControlledOperationProviderRouteWiringEdgeV2{
			{SourceRole: assemblycontract.ControlledOperationApplicationToolPortRoleV2, SourcePortSpecRef: declaration.ApplicationToolPort.PortID, TargetRole: assemblycontract.ControlledOperationToolAdapterRoleV2, TargetComponentID: bindings[0].ComponentID, TargetPortSpecRef: declaration.ToolAdapter.PortSpecRef, ProviderRef: declaration.ToolAdapter.ProviderRef, ModuleRef: declaration.ToolAdapter.ModuleRef, CandidateID: declaration.ToolAdapter.CandidateID, Binding: bindings[0]},
			{SourceRole: assemblycontract.ControlledOperationToolAdapterRoleV2, SourceComponentID: bindings[0].ComponentID, SourcePortSpecRef: declaration.ToolAdapter.PortSpecRef, TargetRole: assemblycontract.ControlledOperationRuntimeGovernanceRoleV2, TargetPortSpecRef: declaration.RuntimeGovernancePort.PortID},
			{SourceRole: assemblycontract.ControlledOperationRuntimeGovernanceRoleV2, SourcePortSpecRef: declaration.RuntimeGovernancePort.PortID, TargetRole: assemblycontract.ControlledOperationRuntimeGatewayRoleV2, TargetComponentID: bindings[1].ComponentID, TargetPortSpecRef: declaration.Gateway.PortSpecRef, ProviderRef: declaration.Gateway.ProviderRef, ModuleRef: declaration.Gateway.ModuleRef, CandidateID: declaration.Gateway.CandidateID, Binding: bindings[1]},
			{SourceRole: assemblycontract.ControlledOperationRuntimeGatewayRoleV2, SourceComponentID: bindings[1].ComponentID, SourcePortSpecRef: declaration.Gateway.PortSpecRef, TargetRole: assemblycontract.ControlledOperationProviderTransportRoleV2, TargetComponentID: bindings[2].ComponentID, TargetPortSpecRef: declaration.ProviderTransport.PortSpecRef, ProviderRef: declaration.ProviderTransport.ProviderRef, ModuleRef: declaration.ProviderTransport.ModuleRef, CandidateID: declaration.ProviderTransport.CandidateID, Binding: bindings[2]},
			{SourceRole: assemblycontract.ControlledOperationProviderTransportRoleV2, SourceComponentID: bindings[2].ComponentID, SourcePortSpecRef: declaration.ProviderTransport.PortSpecRef, TargetRole: assemblycontract.ControlledOperationProviderRoleV2, TargetComponentID: bindings[6].ComponentID, TargetPortSpecRef: declaration.Provider.PortSpecRef, ProviderRef: declaration.Provider.ProviderRef, ModuleRef: declaration.Provider.ModuleRef, CandidateID: declaration.Provider.CandidateID, Binding: bindings[6]},
		},
		ActiveRoutes: []assemblycontract.ControlledOperationProviderActiveRouteRecordV2{{Version: "v2", RouteID: declaration.RouteID, DeclarationRef: declaration.RefV2(), Matrix: declaration.Matrix, Active: true, TransportIdentity: routeActiveIdentityV2(declaration.ProviderTransport), ProviderIdentity: routeActiveIdentityV2(declaration.Provider), TransportBinding: bindings[2], ProviderBinding: bindings[6]}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func routeCompileResultV2(t *testing.T, declaration assemblycontract.ControlledOperationProviderRouteDeclarationV2) assemblycompiler.ControlledOperationProviderRouteCompileResultV2 {
	t.Helper()
	payload, err := json.Marshal(declaration)
	if err != nil {
		t.Fatal(err)
	}
	schema := runtimeports.SchemaRefV2{Namespace: assemblycontract.ControlledOperationProviderRouteSchemaNamespaceV2, Name: assemblycontract.ControlledOperationProviderRouteSchemaNameV2, Version: assemblycontract.ControlledOperationProviderRouteSchemaVersionV2, MediaType: assemblycontract.ControlledOperationProviderRouteSchemaMediaTypeV2, ContentDigest: routeDigestV2("route-schema")}
	extension := runtimeports.GovernanceExtensionV2{Key: assemblycontract.ControlledOperationProviderRouteExtensionKeyV2, Required: true, Payload: runtimeports.OpaquePayloadV2{Schema: schema, ContentDigest: core.DigestBytes(payload), Length: uint64(len(payload)), Inline: payload, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.route/limit", Digest: routeDigestV2("limit")}}}
	publisher := runtimeports.ComponentManifestV2{
		ContractVersion: runtimeports.BindingContractVersionV2, ComponentID: declaration.PublisherComponent, Kind: "praxis.harness/assembly", GovernanceCategory: "praxis.harness/assembly", SemanticVersion: "2.0.0", ArtifactDigest: routeDigestV2("publisher-artifact"),
		Contract: runtimeports.ContractBindingV2{Name: "praxis.harness/assembly", Version: "2.0.0", Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}}, Schemas: []runtimeports.SchemaRefV2{schema}, Locality: runtimeports.LocalityHostControlPlane,
		Dependencies: []runtimeports.ComponentDependencyV2{}, RequiredCapabilities: []runtimeports.CapabilityRequirementV2{}, ProvidedCapabilities: []runtimeports.ProvidedCapabilityV2{{Capability: "praxis.harness/assembly", TTLSeconds: 30, Schemas: []runtimeports.SchemaRefV2{schema}}}, Conformance: runtimeports.ConformanceFullyControlled, ResidualClass: runtimeports.ResidualNone,
		Owners: []runtimeports.OwnerAssignmentV2{{Role: runtimeports.OwnerEffect, OwnerComponentID: declaration.PublisherComponent}, {Role: runtimeports.OwnerSettlement, OwnerComponentID: declaration.PublisherComponent}, {Role: runtimeports.OwnerCleanup, OwnerComponentID: declaration.PublisherComponent}}, Credentials: []runtimeports.CredentialRequirementV2{}, OfflinePolicy: runtimeports.OfflineDenied, Extensions: []runtimeports.GovernanceExtensionV2{extension}, Annotations: []runtimeports.DisplayAnnotationV2{},
	}
	manifestForEndpoint := func(endpoint assemblycontract.ControlledOperationProviderRouteEndpointV2) runtimeports.ComponentManifestV2 {
		value := publisher
		value.ComponentID = endpoint.ComponentID
		value.ArtifactDigest = endpoint.ArtifactDigest
		value.Extensions = []runtimeports.GovernanceExtensionV2{}
		value.ProvidedCapabilities = []runtimeports.ProvidedCapabilityV2{{Capability: endpoint.Capability, TTLSeconds: 30, Schemas: []runtimeports.SchemaRefV2{schema}}}
		value.Owners = []runtimeports.OwnerAssignmentV2{{Role: runtimeports.OwnerEffect, OwnerComponentID: endpoint.ComponentID}, {Role: runtimeports.OwnerSettlement, OwnerComponentID: endpoint.ComponentID}, {Role: runtimeports.OwnerCleanup, OwnerComponentID: endpoint.ComponentID}}
		return value
	}
	transportManifest := manifestForEndpoint(declaration.ProviderTransport)
	providerManifest := manifestForEndpoint(declaration.Provider)
	transportManifestDigest, err := transportManifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	providerManifestDigest, err := providerManifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	ports := []assemblycontract.PortSpecV1{
		{ContractVersion: assemblycontract.ContractVersionV1, PortID: declaration.ProviderTransport.PortSpecRef, OwnerCapability: declaration.ProviderTransport.Capability, ConflictDomainRule: "route:" + string(declaration.ProviderTransport.Capability)},
		{ContractVersion: assemblycontract.ContractVersionV1, PortID: declaration.Provider.PortSpecRef, OwnerCapability: declaration.Provider.Capability, ConflictDomainRule: "route:" + string(declaration.Provider.Capability)},
	}
	modules := []assemblycontract.ModuleDescriptorV1{
		{ContractVersion: assemblycontract.ContractVersionV1, ModuleID: declaration.ProviderTransport.ModuleRef, ArtifactDigest: declaration.ProviderTransport.ArtifactDigest, ComponentManifestRef: assemblycontract.ObjectRefV1{ID: string(declaration.ProviderTransport.ComponentID), Revision: 1, Digest: transportManifestDigest}},
		{ContractVersion: assemblycontract.ContractVersionV1, ModuleID: declaration.Provider.ModuleRef, ArtifactDigest: declaration.Provider.ArtifactDigest, ComponentManifestRef: assemblycontract.ObjectRefV1{ID: string(declaration.Provider.ComponentID), Revision: 1, Digest: providerManifestDigest}},
	}
	candidates := []assemblycontract.ProviderBindingCandidateV1{
		{ContractVersion: assemblycontract.ContractVersionV1, CandidateID: declaration.ProviderTransport.CandidateID, ModuleRef: declaration.ProviderTransport.ModuleRef, PortSpecRef: declaration.ProviderTransport.PortSpecRef, ProviderRef: declaration.ProviderTransport.ProviderRef, Digest: declaration.ProviderTransport.CandidateDigest},
		{ContractVersion: assemblycontract.ContractVersionV1, CandidateID: declaration.Provider.CandidateID, ModuleRef: declaration.Provider.ModuleRef, PortSpecRef: declaration.Provider.PortSpecRef, ProviderRef: declaration.Provider.ProviderRef, Digest: declaration.Provider.CandidateDigest},
	}
	inputDigest := routeDigestV2("input")
	manifest := assemblycontract.AssemblyManifestV1{ContractVersion: assemblycontract.ContractVersionV1, InputDigest: inputDigest, CatalogDigest: routeDigestV2("assembly-catalog"), ComponentManifests: []runtimeports.ComponentManifestV2{publisher, transportManifest, providerManifest}, Modules: modules, PortSpecs: ports, ProviderBindingCandidates: candidates}
	manifest.Digest, err = assemblycontract.ManifestDigestV1(manifest)
	if err != nil {
		t.Fatal(err)
	}
	graph := assemblycontract.CompiledHarnessGraphV1{ContractVersion: assemblycontract.ContractVersionV1, InputDigest: inputDigest, CatalogDigest: manifest.CatalogDigest}
	graph.Digest, err = assemblycontract.GraphDigestV1(graph)
	if err != nil {
		t.Fatal(err)
	}
	generation := assemblycontract.AssemblyGenerationV1{ContractVersion: assemblycontract.ContractVersionV1, GenerationID: "generation-route", Revision: 1, CompilerVersion: assemblycontract.CompilerVersionV1, CreatedUnixNano: 1, State: assemblycontract.AssemblyStateSealedV1, InputDigest: inputDigest, ManifestDigest: manifest.Digest, GraphDigest: graph.Digest, DiagnosticDigest: routeDigestV2("diagnostics"), ResidualReportDigest: routeDigestV2("residuals")}
	generation.Digest, err = assemblycontract.GenerationDigestV1(generation)
	if err != nil {
		t.Fatal(err)
	}
	handoff := assemblycontract.AssemblyHandoffV1{ContractVersion: assemblycontract.ContractVersionV1, GenerationRef: assemblycontract.ObjectRefV1{ID: generation.GenerationID, Revision: generation.Revision, Digest: generation.Digest}, ManifestDigest: manifest.Digest, GraphDigest: graph.Digest, CatalogDigest: manifest.CatalogDigest, RequiredExtension: "praxis.harness/assembly-generation", ProviderCandidates: []assemblycontract.ProviderBindingCandidateV1{}}
	handoff.Digest, err = assemblycontract.HandoffDigestV1(handoff)
	if err != nil {
		t.Fatal(err)
	}
	transportPortDigest, err := assemblycontract.PortSpecDigestForControlledOperationProviderRouteV2(ports[0])
	if err != nil {
		t.Fatal(err)
	}
	providerPortDigest, err := assemblycontract.PortSpecDigestForControlledOperationProviderRouteV2(ports[1])
	if err != nil {
		t.Fatal(err)
	}
	transportIdentity := routeActiveIdentityV2(declaration.ProviderTransport)
	transportIdentity.ComponentManifestDigest = transportManifestDigest
	transportIdentity.PortSpecDigest = transportPortDigest
	providerIdentity := routeActiveIdentityV2(declaration.Provider)
	providerIdentity.ComponentManifestDigest = providerManifestDigest
	providerIdentity.PortSpecDigest = providerPortDigest
	result := assemblycompiler.ControlledOperationProviderRouteCompileResultV2{Declaration: declaration, PublisherManifest: publisher, Extension: extension, GovernanceCatalogDigest: routeDigestV2("governance-catalog"), AssemblyInputDigest: inputDigest, Manifest: manifest, Graph: graph, Generation: generation, Handoff: handoff, ProviderTransportIdentity: transportIdentity, ProviderIdentity: providerIdentity}
	result.CompileDigest, err = result.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	if err := result.ValidateV2(); err != nil {
		t.Fatal(err)
	}
	return result
}

type routeConformanceInputsStubV2 struct {
	value ControlledOperationProviderRouteConformanceRequestV2
}

func (routeConformanceInputsStubV2) controlledOperationProviderRouteConformanceInputsOwnerV2() {}

func (s routeConformanceInputsStubV2) InspectCurrentControlledOperationProviderRouteConformanceInputsV2(_ context.Context, _ ControlledOperationProviderRouteConformanceKeyV2) (ControlledOperationProviderRouteConformanceRequestV2, error) {
	return s.value, nil
}

type nilRouteConformanceInputsV2 struct{}

func (*nilRouteConformanceInputsV2) controlledOperationProviderRouteConformanceInputsOwnerV2() {}

func (*nilRouteConformanceInputsV2) InspectCurrentControlledOperationProviderRouteConformanceInputsV2(context.Context, ControlledOperationProviderRouteConformanceKeyV2) (ControlledOperationProviderRouteConformanceRequestV2, error) {
	panic("typed-nil conformance reader must never be called")
}

type nilRouteFactStoreV2 struct {
	ControlledOperationProviderRouteFactStoreV2
}
type nilRouteCurrentInputsV2 struct {
	ControlledOperationProviderRouteInputsCurrentReaderV2
}

func routeConformanceV2(t *testing.T, now time.Time) (assemblycontract.ControlledOperationProviderRouteDeclarationV2, assemblycontract.ControlledOperationProviderRouteConformanceV2) {
	t.Helper()
	declaration := routeDeclarationV2(t)
	bindings := routeBindingsV2(declaration)
	compile := routeCompileResultV2(t, declaration)
	generation := generationArtifactRefFromCompileV2(compile)
	currentness := ControlledOperationProviderRouteSourceCurrentnessV2{CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	assemblyConformance := assemblycontract.AssemblyBindingConformanceV1{
		ContractVersion: assemblycontract.ContractVersionV1,
		HandoffRef:      assemblycontract.ObjectRefV1{ID: "handoff-route", Revision: 1, Digest: compile.Handoff.Digest},
		GenerationRef:   assemblycontract.ObjectRefV1{ID: compile.Generation.GenerationID, Revision: compile.Generation.Revision, Digest: compile.Generation.Digest},
		ManifestDigest:  compile.Manifest.Digest, GraphDigest: compile.Graph.Digest,
		Binding: bindings[0], BindingSetID: "binding-set-route", BindingSetRevision: 1,
		BindingSetDigest: routeDigestV2("binding-set"), BindingSetSemanticDigest: routeDigestV2("binding-semantic"), BindingSetCurrentnessDigest: routeDigestV2("binding-current"),
		CapabilityDigest: routeDigestV2("capability"), SchemaDigests: []core.Digest{routeDigestV2("schema-set")},
		ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(), Current: true,
	}
	assemblyConformanceDigest, err := assemblycontract.BindingConformanceDigestV1(assemblyConformance)
	if err != nil {
		t.Fatal(err)
	}
	assemblyConformance.Digest = assemblyConformanceDigest
	wiring := routeInventoryV2(t, now, declaration, bindings)
	wiring.ActiveRoutes[0].TransportIdentity = compile.ProviderTransportIdentity
	wiring.ActiveRoutes[0].ProviderIdentity = compile.ProviderIdentity
	wiring.Digest = ""
	wiring, err = assemblycontract.SealControlledOperationProviderRouteWiringInventoryV2(wiring)
	if err != nil {
		t.Fatal(err)
	}
	request := ControlledOperationProviderRouteConformanceRequestV2{
		Compile:             compile,
		AssemblyInputDigest: compile.AssemblyInputDigest, ManifestDigest: compile.Manifest.Digest, GraphDigest: compile.Graph.Digest, Generation: generation,
		HandoffID: "handoff-route", HandoffRevision: 1, HandoffDigest: compile.Handoff.Digest, BindingSetID: "binding-set-route", BindingSetRevision: 1,
		BindingSetDigest: routeDigestV2("binding-set"), BindingSetSemanticDigest: routeDigestV2("binding-semantic"), BindingSetCurrentnessDigest: routeDigestV2("binding-current"),
		AssemblyConformance: assemblyConformance, AssemblyConformanceRef: assemblycontract.ObjectRefV1{ID: "assembly-conformance", Revision: 1, Digest: assemblyConformance.Digest}, ActiveRouteID: "active-route", ActiveRouteRevision: 1, ActiveRouteDigest: routeDigestV2("active-route"),
		Bindings: bindings, WiringInventory: wiring,
		GenerationCurrentness: currentness, HandoffCurrentness: currentness, BindingSetCurrentness: currentness, AssemblyConformanceCurrentness: currentness, ActiveRouteCurrentness: currentness,
		BindingCurrentness: [7]ControlledOperationProviderRouteSourceCurrentnessV2{currentness, currentness, currentness, currentness, currentness, currentness, currentness},
		CheckedUnixNano:    now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(), Revision: 1,
	}
	builder, err := NewControlledOperationProviderRouteConformanceBuilderV2(routeConformanceInputsStubV2{value: request}, (&routeClockV2{times: []time.Time{now, now}}).Now)
	if err != nil {
		t.Fatal(err)
	}
	conformance, err := builder.BuildControlledOperationProviderRouteConformanceV2(context.Background(), ControlledOperationProviderRouteConformanceKeyV2{CompileDigest: compile.CompileDigest, BindingSetID: request.BindingSetID, ActiveRouteID: request.ActiveRouteID, Revision: request.Revision})
	if err != nil {
		t.Fatal(err)
	}
	return declaration, conformance
}

func publishRouteFactsV2(t *testing.T, store *InMemoryControlledOperationProviderRouteFactStoreV2, declaration assemblycontract.ControlledOperationProviderRouteDeclarationV2, conformance assemblycontract.ControlledOperationProviderRouteConformanceV2) runtimeports.ControlledOperationProviderRouteCurrentProjectionV2 {
	t.Helper()
	if _, err := store.PublishControlledOperationProviderRouteDeclarationV2(context.Background(), declaration, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishControlledOperationProviderRouteConformanceV2(context.Background(), conformance, 0); err != nil {
		t.Fatal(err)
	}
	current, err := store.PublishControlledOperationProviderRouteCurrentV2(context.Background(), conformance, 0)
	if err != nil {
		t.Fatal(err)
	}
	return current
}

func TestControlledOperationProviderRouteLostRepliesRecoverByExactInspectV2(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC)
	declaration, conformance := routeConformanceV2(t, now)
	store := NewInMemoryControlledOperationProviderRouteFactStoreV2()
	store.LoseNextDeclarationReplyV2()
	if _, err := store.PublishControlledOperationProviderRouteDeclarationV2(context.Background(), declaration, 0); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected lost declaration reply, got %v", err)
	}
	if got, err := store.InspectControlledOperationProviderRouteDeclarationV2(context.Background(), declaration.RefV2()); err != nil || got != declaration {
		t.Fatalf("declaration recovery failed: %v", err)
	}
	store.LoseNextConformanceReplyV2()
	if _, err := store.PublishControlledOperationProviderRouteConformanceV2(context.Background(), conformance, 0); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected lost conformance reply, got %v", err)
	}
	if got, err := store.InspectControlledOperationProviderRouteConformanceV2(context.Background(), conformance.RefV2()); err != nil || got != conformance {
		t.Fatalf("conformance recovery failed: %v", err)
	}
	store.LoseNextCurrentReplyV2()
	if _, err := store.PublishControlledOperationProviderRouteCurrentV2(context.Background(), conformance, 0); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected lost current reply, got %v", err)
	}
	current, err := store.PublishControlledOperationProviderRouteCurrentV2(context.Background(), conformance, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := store.InspectControlledOperationProviderRouteCurrentV2(context.Background(), current.Ref); err != nil || got != current {
		t.Fatalf("current recovery failed: %v", err)
	}
}

func TestControlledOperationProviderRouteConcurrentPublishLinearizesOnceV2(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC)
	declaration, conformance := routeConformanceV2(t, now)
	store := NewInMemoryControlledOperationProviderRouteFactStoreV2()
	if _, err := store.PublishControlledOperationProviderRouteDeclarationV2(context.Background(), declaration, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishControlledOperationProviderRouteConformanceV2(context.Background(), conformance, 0); err != nil {
		t.Fatal(err)
	}
	var failures atomic.Int32
	refs := make(chan runtimeports.ControlledOperationProviderRouteCurrentRefV2, 64)
	var group sync.WaitGroup
	for range 64 {
		group.Add(1)
		go func() {
			defer group.Done()
			projection, err := store.PublishControlledOperationProviderRouteCurrentV2(context.Background(), conformance, 0)
			if err != nil {
				failures.Add(1)
				return
			}
			refs <- projection.Ref
		}()
	}
	group.Wait()
	close(refs)
	if failures.Load() != 0 {
		t.Fatalf("same canonical publish failed %d times", failures.Load())
	}
	var first runtimeports.ControlledOperationProviderRouteCurrentRefV2
	for ref := range refs {
		if first == (runtimeports.ControlledOperationProviderRouteCurrentRefV2{}) {
			first = ref
		} else if ref != first {
			t.Fatalf("concurrent publishes produced multiple refs")
		}
	}
	if first.Revision != 1 {
		t.Fatalf("expected one linearized revision, got %d", first.Revision)
	}
}

type routeInputsReaderStubV2 struct {
	mutate func(runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) runtimeports.ControlledOperationProviderRouteCurrentProjectionV2
}

func (s routeInputsReaderStubV2) InspectCurrentControlledOperationProviderRouteInputsV2(_ context.Context, value runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error) {
	if s.mutate != nil {
		value = s.mutate(value)
	}
	value.ProjectionDigest = ""
	return runtimeports.SealControlledOperationProviderRouteCurrentProjectionV2(value)
}

type routeClockV2 struct {
	mu    sync.Mutex
	times []time.Time
}

func (c *routeClockV2) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.times) == 0 {
		return time.Time{}
	}
	value := c.times[0]
	if len(c.times) > 1 {
		c.times = c.times[1:]
	}
	return value
}

func TestControlledOperationProviderRouteReaderRejectsClockAndBindingDriftV2(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC)
	declaration, conformance := routeConformanceV2(t, now)
	store := NewInMemoryControlledOperationProviderRouteFactStoreV2()
	current := publishRouteFactsV2(t, store, declaration, conformance)
	matrix := runtimeports.OperationScopeEvidenceActionMatrixV3()

	clock := &routeClockV2{times: []time.Time{now.Add(time.Second), now.Add(2 * time.Second)}}
	reader, err := NewControlledOperationProviderRouteCurrentReaderAdapterV2(store, routeInputsReaderStubV2{}, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.InspectCurrentControlledOperationProviderRouteV2(context.Background(), current.Ref, matrix); err != nil {
		t.Fatal(err)
	}

	for bindingIndex := 0; bindingIndex < 7; bindingIndex++ {
		bindingIndex := bindingIndex
		t.Run("binding-drift", func(t *testing.T) {
			clock := &routeClockV2{times: []time.Time{now.Add(time.Second), now.Add(2 * time.Second)}}
			stub := routeInputsReaderStubV2{mutate: func(value runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) runtimeports.ControlledOperationProviderRouteCurrentProjectionV2 {
				bindings := []*runtimeports.ProviderBindingRefV2{&value.ToolAdapterBinding, &value.GatewayBinding, &value.ProviderTransportBinding, &value.PreparedReaderBinding, &value.BoundaryReaderBinding, &value.ProviderInspectBinding, &value.ProviderBinding}
				bindings[bindingIndex].ArtifactDigest = routeDigestV2("drift")
				value.Ref.Watermark = ""
				value.Ref.Digest = ""
				return value
			}}
			reader, _ := NewControlledOperationProviderRouteCurrentReaderAdapterV2(store, stub, clock.Now)
			if _, err := reader.InspectCurrentControlledOperationProviderRouteV2(context.Background(), current.Ref, matrix); !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("binding %d drift: got %v", bindingIndex, err)
			}
		})
	}

	rollback := &routeClockV2{times: []time.Time{now.Add(2 * time.Second), now.Add(time.Second)}}
	reader, _ = NewControlledOperationProviderRouteCurrentReaderAdapterV2(store, routeInputsReaderStubV2{}, rollback.Now)
	if _, err := reader.InspectCurrentControlledOperationProviderRouteV2(context.Background(), current.Ref, matrix); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback was not rejected: %v", err)
	}

	crossing := &routeClockV2{times: []time.Time{now.Add(time.Second), time.Unix(0, current.ExpiresUnixNano)}}
	reader, _ = NewControlledOperationProviderRouteCurrentReaderAdapterV2(store, routeInputsReaderStubV2{}, crossing.Now)
	if _, err := reader.InspectCurrentControlledOperationProviderRouteV2(context.Background(), current.Ref, matrix); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("TTL crossing was not rejected: %v", err)
	}
}

func TestControlledOperationProviderRouteSameIDDriftIsConflictV2(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC)
	declaration, conformance := routeConformanceV2(t, now)
	store := NewInMemoryControlledOperationProviderRouteFactStoreV2()
	current := publishRouteFactsV2(t, store, declaration, conformance)
	drifted := current.Ref
	drifted.Revision++
	drifted.Digest = ""
	drifted, err := runtimeports.SealControlledOperationProviderRouteCurrentRefV2(drifted)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InspectControlledOperationProviderRouteCurrentV2(context.Background(), drifted); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same-ID drift was not Conflict: %v", err)
	}
}

func TestControlledOperationProviderRouteFactsAreImmutableAndRejectABAV2(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC)
	declaration, conformance := routeConformanceV2(t, now)
	store := NewInMemoryControlledOperationProviderRouteFactStoreV2()
	current := publishRouteFactsV2(t, store, declaration, conformance)

	driftedDeclaration := declaration
	driftedDeclaration.PublisherComponent = "praxis.harness/other-assembly"
	driftedDeclaration.DeclarationDigest = ""
	driftedDeclaration, err := assemblycontract.SealControlledOperationProviderRouteDeclarationV2(driftedDeclaration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishControlledOperationProviderRouteDeclarationV2(context.Background(), driftedDeclaration, declaration.Revision); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("declaration semantic update was not rejected: %v", err)
	}
	if got, err := store.PublishControlledOperationProviderRouteDeclarationV2(context.Background(), declaration, 0); err != nil || got != declaration {
		t.Fatalf("exact declaration replay failed after rejected B: %v", err)
	}

	driftedConformance := conformance
	driftedConformance.ActiveRouteDigest = routeDigestV2("semantic-B")
	driftedConformance.ConformanceDigest = ""
	driftedConformance, err = assemblycontract.SealControlledOperationProviderRouteConformanceV2(driftedConformance)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishControlledOperationProviderRouteConformanceV2(context.Background(), driftedConformance, conformance.Revision); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("conformance semantic update was not rejected: %v", err)
	}
	if _, err := store.PublishControlledOperationProviderRouteCurrentV2(context.Background(), driftedConformance, current.Ref.Revision); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("current accepted semantic B: %v", err)
	}

	next := conformance
	next.Generation.ID = "generation-route-next"
	next.Generation.Revision = 2
	next.Generation.Digest = routeDigestV2("generation-next")
	next.HandoffID = "handoff-route-next"
	next.HandoffRevision = 2
	next.HandoffDigest = routeDigestV2("handoff-next")
	next.BindingSetRevision = 2
	next.BindingSetDigest = routeDigestV2("binding-set-next")
	next.BindingSetSemanticDigest = routeDigestV2("binding-semantic-next")
	next.BindingSetCurrentnessDigest = routeDigestV2("binding-current-next")
	next.ActiveRouteRevision = 2
	next.ActiveRouteDigest = routeDigestV2("active-route-next")
	next.CheckedUnixNano = now.Add(time.Second).UnixNano()
	next.ExpiresUnixNano = now.Add(2 * time.Minute).UnixNano()
	bindings := []*runtimeports.ProviderBindingRefV2{&next.ToolAdapterBinding, &next.GatewayBinding, &next.ProviderTransportBinding, &next.PreparedReaderBinding, &next.BoundaryReaderBinding, &next.ProviderInspectBinding, &next.ProviderBinding}
	for _, binding := range bindings {
		binding.BindingSetRevision = 2
	}
	next.ConformanceID, err = assemblycontract.DeriveControlledOperationProviderRouteConformanceIDV2(next.DeclarationRef.RouteID, next.Generation, next.BindingSetID)
	if err != nil {
		t.Fatal(err)
	}
	next.Revision = 1
	next.ConformanceDigest = ""
	next, err = assemblycontract.SealControlledOperationProviderRouteConformanceV2(next)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishControlledOperationProviderRouteConformanceV2(context.Background(), next, 0); err != nil {
		t.Fatal(err)
	}
	advanced, err := store.PublishControlledOperationProviderRouteCurrentV2(context.Background(), next, current.Ref.Revision)
	if err != nil || advanced.Ref.Revision != 2 {
		t.Fatalf("monotonic current B failed: %v", err)
	}
	if _, err := store.PublishControlledOperationProviderRouteCurrentV2(context.Background(), conformance, advanced.Ref.Revision); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("current accepted A after B: %v", err)
	}
}

func TestControlledOperationProviderRouteDifferentContentConcurrencyHasOneWinnerV2(t *testing.T) {
	t.Parallel()
	base := routeDeclarationV2(t)
	store := NewInMemoryControlledOperationProviderRouteFactStoreV2()
	var successes atomic.Int32
	var conflicts atomic.Int32
	var group sync.WaitGroup
	for index := 0; index < 64; index++ {
		index := index
		group.Add(1)
		go func() {
			defer group.Done()
			candidate := base
			candidate.PublisherComponent = runtimeports.ComponentIDV2("praxis.harness/assembly-" + string(rune('a'+index%26)) + string(rune('a'+index/26)))
			candidate.DeclarationDigest = ""
			candidate, err := assemblycontract.SealControlledOperationProviderRouteDeclarationV2(candidate)
			if err != nil {
				t.Errorf("seal: %v", err)
				return
			}
			if _, err = store.PublishControlledOperationProviderRouteDeclarationV2(context.Background(), candidate, 0); err == nil {
				successes.Add(1)
			} else if core.HasCategory(err, core.ErrorConflict) {
				conflicts.Add(1)
			} else {
				t.Errorf("unexpected publish error: %v", err)
			}
		}()
	}
	group.Wait()
	if successes.Load() != 1 || conflicts.Load() != 63 {
		t.Fatalf("different-content concurrency got successes=%d conflicts=%d", successes.Load(), conflicts.Load())
	}
}

func TestControlledOperationProviderRouteOwnerArtifactsPublishStableRefsAndIndependentReadersV2(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC)
	declaration := routeDeclarationV2(t)
	compile := routeCompileResultV2(t, declaration)
	bindings := routeBindingsV2(declaration)
	wiring := routeInventoryV2(t, now, declaration, bindings)
	wiring.ActiveRoutes[0].TransportIdentity = compile.ProviderTransportIdentity
	wiring.ActiveRoutes[0].ProviderIdentity = compile.ProviderIdentity
	wiring.Digest = ""
	var err error
	wiring, err = assemblycontract.SealControlledOperationProviderRouteWiringInventoryV2(wiring)
	if err != nil {
		t.Fatal(err)
	}
	record := wiring.ActiveRoutes[0]
	activeDigest, err := core.CanonicalJSONDigest("praxis.harness.controlled-operation-provider-route", assemblycontract.ControlledOperationProviderRouteContractVersionV2, "ControlledOperationProviderActiveRouteRecordV2", record)
	if err != nil {
		t.Fatal(err)
	}
	active := ControlledOperationProviderActiveRouteCurrentV2{
		Ref:    ControlledOperationProviderActiveRouteCurrentRefV2{ActiveRouteID: record.RouteID, Revision: 1, Digest: activeDigest},
		Record: record, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	}
	key := ControlledOperationProviderRouteConformanceKeyV2{CompileDigest: compile.CompileDigest, BindingSetID: bindings[0].BindingSetID, ActiveRouteID: record.RouteID, Revision: 1}
	publication := ControlledOperationProviderRouteOwnerArtifactPublicationV2{
		Key: key, Compile: compile,
		Association: runtimeports.GenerationBindingAssociationRefV1{ID: "route-owner-association", Revision: 1, Digest: routeDigestV2("route-owner-association")},
		ActiveRoute: active, Wiring: wiring, Bindings: bindings,
	}
	store := NewInMemoryControlledOperationProviderRouteOwnerArtifactStoreV2()
	refs, err := store.PublishExactV2(context.Background(), publication, now)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := store.PublishExactV2(context.Background(), publication, now); err != nil || again != refs {
		t.Fatalf("exact owner publication is not idempotent: %v", err)
	}
	if got, err := store.InspectControlledOperationProviderRouteOwnerRefsV2(context.Background(), key); err != nil || got != refs {
		t.Fatalf("Owner refs Inspect drifted: %v", err)
	}
	if got, err := store.InspectVerifiedControlledOperationProviderRouteCompileV2(context.Background(), refs.Compile); err != nil || got.CompileDigest != compile.CompileDigest {
		t.Fatalf("verified compile Inspect drifted: %v", err)
	}
	if got, err := store.InspectControlledOperationProviderActiveRouteCurrentV2(context.Background(), refs.ActiveRoute); err != nil || got != active {
		t.Fatalf("active-route Inspect drifted: %v", err)
	}
	if got, err := store.InspectControlledOperationProviderRouteWiringInventoryV2(context.Background(), refs.Wiring); err != nil || got.Digest != wiring.Digest {
		t.Fatalf("wiring Inspect drifted: %v", err)
	}
	drifted := publication
	drifted.ActiveRoute.ExpiresUnixNano++
	if _, err := store.PublishExactV2(context.Background(), drifted, now); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("same Owner key changed content: %v", err)
	}
}

func TestControlledOperationProviderRouteConformanceBuilderRejectsSelfSignedCompileV2(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC)
	declaration, _ := routeConformanceV2(t, now)
	compile := routeCompileResultV2(t, declaration)
	compile.Extension.Payload.Inline = []byte("x")
	request := ControlledOperationProviderRouteConformanceRequestV2{Compile: compile, BindingSetID: "binding-set-route", ActiveRouteID: "active-route", Revision: 1}
	builder, err := NewControlledOperationProviderRouteConformanceBuilderV2(routeConformanceInputsStubV2{value: request}, (&routeClockV2{times: []time.Time{now, now}}).Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builder.BuildControlledOperationProviderRouteConformanceV2(context.Background(), ControlledOperationProviderRouteConformanceKeyV2{CompileDigest: compile.CompileDigest, BindingSetID: request.BindingSetID, ActiveRouteID: request.ActiveRouteID, Revision: 1}); err == nil {
		t.Fatal("self-signed Inline='x' compile result was accepted")
	}
}

func TestControlledOperationProviderRouteConstructorsRejectTypedNilDependenciesV2(t *testing.T) {
	t.Parallel()
	var conformanceInputs *nilRouteConformanceInputsV2
	if _, err := NewControlledOperationProviderRouteConformanceBuilderV2(conformanceInputs, time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil conformance inputs got %v", err)
	}
	var store *nilRouteFactStoreV2
	var currentInputs *nilRouteCurrentInputsV2
	if _, err := NewControlledOperationProviderRouteCurrentReaderAdapterV2(store, routeInputsReaderStubV2{}, time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil route store got %v", err)
	}
	if _, err := NewControlledOperationProviderRouteCurrentReaderAdapterV2(NewInMemoryControlledOperationProviderRouteFactStoreV2(), currentInputs, time.Now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("typed-nil current inputs got %v", err)
	}
}
