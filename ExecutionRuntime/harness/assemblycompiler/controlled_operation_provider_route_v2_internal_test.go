package assemblycompiler

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type compilerRouteLegacyCurrentReaderV2 struct {
	mu    sync.Mutex
	facts []ControlledOperationProviderLegacyActiveRouteFactV2
	calls int
}

func (r *compilerRouteLegacyCurrentReaderV2) InspectControlledOperationProviderLegacyRouteCurrentV2(_ context.Context, _ assemblycontract.ObjectRefV1) (ControlledOperationProviderLegacyActiveRouteFactV2, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.facts) == 0 {
		return ControlledOperationProviderLegacyActiveRouteFactV2{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "test legacy current Reader has no Fact")
	}
	index := r.calls
	if index >= len(r.facts) {
		index = len(r.facts) - 1
	}
	r.calls++
	return r.facts[index], nil
}

func compilerRouteDigestV2(value string) core.Digest { return core.DigestBytes([]byte(value)) }

func compilerRouteRefV2(value string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: value, Revision: 1, Digest: compilerRouteDigestV2(value)}
}

func compilerRouteSchemaV2(value string) runtimeports.SchemaRefV2 {
	return runtimeports.SchemaRefV2{Namespace: "praxis.route", Name: value, Version: "2.0.0", MediaType: "application/json", ContentDigest: compilerRouteDigestV2("schema:" + value)}
}

func compilerRoutePortV2(role assemblycontract.ControlledOperationProviderRouteRoleV2, id string, capability runtimeports.CapabilityNameV2) assemblycontract.ControlledOperationProviderRoutePortRefV2 {
	schemaName := strings.NewReplacer("/", "-", ".", "-").Replace(id)
	return assemblycontract.ControlledOperationProviderRoutePortRefV2{Role: role, PortID: id, PortDigest: compilerRouteDigestV2(id), OwnerCapability: capability, RequestSchema: compilerRouteSchemaV2(schemaName + "-request"), ResponseSchema: compilerRouteSchemaV2(schemaName + "-response"), ContractVersion: "2.0.0"}
}

func compilerRouteEndpointV2(role assemblycontract.ControlledOperationProviderRouteRoleV2, suffix string, capability runtimeports.CapabilityNameV2) assemblycontract.ControlledOperationProviderRouteEndpointV2 {
	component := "praxis.route/" + suffix
	return assemblycontract.ControlledOperationProviderRouteEndpointV2{Role: role, ComponentID: runtimeports.ComponentIDV2(component), ManifestDigest: compilerRouteDigestV2(component + ":manifest"), ArtifactDigest: compilerRouteDigestV2(component + ":artifact"), Capability: capability, ContractVersion: "2.0.0", Locality: runtimeports.LocalityHostControlPlane, CandidateID: component + "/candidate", CandidateDigest: compilerRouteDigestV2(component + ":candidate"), ModuleRef: component + "/module", PortSpecRef: component + "/port", ProviderRef: compilerRouteRefV2(component + "/provider")}
}

func compilerRouteReaderV2(role assemblycontract.ControlledOperationProviderRouteRoleV2, suffix string, capability runtimeports.CapabilityNameV2) assemblycontract.ControlledOperationProviderRouteReaderRefV2 {
	component := "praxis.route/" + suffix
	return assemblycontract.ControlledOperationProviderRouteReaderRefV2{Role: role, ComponentID: runtimeports.ComponentIDV2(component), ManifestDigest: compilerRouteDigestV2(component + ":manifest"), ArtifactDigest: compilerRouteDigestV2(component + ":artifact"), Capability: capability, PortSpecID: component + "/port", PortSpecDigest: compilerRouteDigestV2(component + ":port"), RequestSchema: compilerRouteSchemaV2(suffix + "-request"), ProjectionSchema: compilerRouteSchemaV2(suffix + "-projection"), ReadOnly: true, NoExecute: true}
}

func compilerRouteDeclarationV2(t *testing.T) assemblycontract.ControlledOperationProviderRouteDeclarationV2 {
	t.Helper()
	declaration, err := assemblycontract.SealControlledOperationProviderRouteDeclarationV2(assemblycontract.ControlledOperationProviderRouteDeclarationV2{
		RouteID: "praxis.route/single-tool", Revision: 1, PublisherComponent: "praxis.harness/assembly", Matrix: runtimeports.OperationScopeEvidenceActionMatrixV3(),
		ApplicationToolPort:   compilerRoutePortV2(assemblycontract.ControlledOperationApplicationToolPortRoleV2, "praxis.route/application-port", "praxis.application/single-call-tool-action"),
		ToolAdapter:           compilerRouteEndpointV2(assemblycontract.ControlledOperationToolAdapterRoleV2, "tool-adapter", runtimeports.ControlledOperationToolAdapterCapabilityV2),
		RuntimeGovernancePort: compilerRoutePortV2(assemblycontract.ControlledOperationRuntimeGovernanceRoleV2, "praxis.route/runtime-port", runtimeports.ControlledOperationGatewayCapabilityV2),
		Gateway:               compilerRouteEndpointV2(assemblycontract.ControlledOperationRuntimeGatewayRoleV2, "gateway", runtimeports.ControlledOperationGatewayCapabilityV2),
		ProviderTransport:     compilerRouteEndpointV2(assemblycontract.ControlledOperationProviderTransportRoleV2, "transport", runtimeports.ControlledOperationProviderTransportCapabilityV2),
		Provider:              compilerRouteEndpointV2(assemblycontract.ControlledOperationProviderRoleV2, "provider", runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3)),
		PreparedCurrentReader: compilerRouteReaderV2(assemblycontract.ControlledOperationPreparedReaderRoleV2, "prepared-reader", runtimeports.ControlledOperationPreparedReaderCapabilityV2),
		BoundaryCurrentReader: compilerRouteReaderV2(assemblycontract.ControlledOperationBoundaryReaderRoleV2, "boundary-reader", runtimeports.ControlledOperationBoundaryReaderCapabilityV2),
		ProviderInspectReader: compilerRouteReaderV2(assemblycontract.ControlledOperationProviderInspectRoleV2, "inspect-reader", runtimeports.ControlledOperationProviderInspectCapabilityV2),
		ActiveBindingPolicy:   assemblycontract.ControlledOperationProviderActiveBindingPolicyV2, BypassPolicy: assemblycontract.ControlledOperationProviderBypassPolicyV2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return declaration
}

func compilerRouteBindingsV2(declaration assemblycontract.ControlledOperationProviderRouteDeclarationV2) [7]runtimeports.ProviderBindingRefV2 {
	values := []struct {
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
	for index, value := range values {
		result[index] = runtimeports.ProviderBindingRefV2{BindingSetID: "binding-route", BindingSetRevision: 1, ComponentID: value.component, ManifestDigest: value.manifest, ArtifactDigest: value.artifact, Capability: value.cap}
	}
	return result
}

func compilerRouteActiveIdentityV2(endpoint assemblycontract.ControlledOperationProviderRouteEndpointV2) assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2 {
	return assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{
		ProviderRef: endpoint.ProviderRef, CandidateID: endpoint.CandidateID, ModuleRef: endpoint.ModuleRef,
		ComponentID: endpoint.ComponentID, ComponentManifestDigest: endpoint.ManifestDigest, ArtifactDigest: endpoint.ArtifactDigest,
		Capability: endpoint.Capability, PortSpecRef: endpoint.PortSpecRef, PortSpecDigest: compilerRouteDigestV2(endpoint.PortSpecRef), ConflictDomain: "route:" + string(endpoint.Capability),
	}
}

func compilerRouteInventoryV2(t *testing.T, declaration assemblycontract.ControlledOperationProviderRouteDeclarationV2, bindings [7]runtimeports.ProviderBindingRefV2) assemblycontract.ControlledOperationProviderRouteWiringInventoryV2 {
	t.Helper()
	now := time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC)
	inventory, err := assemblycontract.SealControlledOperationProviderRouteWiringInventoryV2(assemblycontract.ControlledOperationProviderRouteWiringInventoryV2{
		InventoryID: "route-inventory", Revision: 1, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
		Edges: []assemblycontract.ControlledOperationProviderRouteWiringEdgeV2{
			{SourceRole: assemblycontract.ControlledOperationApplicationToolPortRoleV2, SourcePortSpecRef: declaration.ApplicationToolPort.PortID, TargetRole: assemblycontract.ControlledOperationToolAdapterRoleV2, TargetComponentID: bindings[0].ComponentID, TargetPortSpecRef: declaration.ToolAdapter.PortSpecRef, ProviderRef: declaration.ToolAdapter.ProviderRef, ModuleRef: declaration.ToolAdapter.ModuleRef, CandidateID: declaration.ToolAdapter.CandidateID, Binding: bindings[0]},
			{SourceRole: assemblycontract.ControlledOperationToolAdapterRoleV2, SourceComponentID: bindings[0].ComponentID, SourcePortSpecRef: declaration.ToolAdapter.PortSpecRef, TargetRole: assemblycontract.ControlledOperationRuntimeGovernanceRoleV2, TargetPortSpecRef: declaration.RuntimeGovernancePort.PortID},
			{SourceRole: assemblycontract.ControlledOperationRuntimeGovernanceRoleV2, SourcePortSpecRef: declaration.RuntimeGovernancePort.PortID, TargetRole: assemblycontract.ControlledOperationRuntimeGatewayRoleV2, TargetComponentID: bindings[1].ComponentID, TargetPortSpecRef: declaration.Gateway.PortSpecRef, ProviderRef: declaration.Gateway.ProviderRef, ModuleRef: declaration.Gateway.ModuleRef, CandidateID: declaration.Gateway.CandidateID, Binding: bindings[1]},
			{SourceRole: assemblycontract.ControlledOperationRuntimeGatewayRoleV2, SourceComponentID: bindings[1].ComponentID, SourcePortSpecRef: declaration.Gateway.PortSpecRef, TargetRole: assemblycontract.ControlledOperationProviderTransportRoleV2, TargetComponentID: bindings[2].ComponentID, TargetPortSpecRef: declaration.ProviderTransport.PortSpecRef, ProviderRef: declaration.ProviderTransport.ProviderRef, ModuleRef: declaration.ProviderTransport.ModuleRef, CandidateID: declaration.ProviderTransport.CandidateID, Binding: bindings[2]},
			{SourceRole: assemblycontract.ControlledOperationProviderTransportRoleV2, SourceComponentID: bindings[2].ComponentID, SourcePortSpecRef: declaration.ProviderTransport.PortSpecRef, TargetRole: assemblycontract.ControlledOperationProviderRoleV2, TargetComponentID: bindings[6].ComponentID, TargetPortSpecRef: declaration.Provider.PortSpecRef, ProviderRef: declaration.Provider.ProviderRef, ModuleRef: declaration.Provider.ModuleRef, CandidateID: declaration.Provider.CandidateID, Binding: bindings[6]},
		}, ActiveRoutes: []assemblycontract.ControlledOperationProviderActiveRouteRecordV2{{Version: "v2", RouteID: declaration.RouteID, DeclarationRef: declaration.RefV2(), Matrix: declaration.Matrix, Active: true, TransportIdentity: compilerRouteActiveIdentityV2(declaration.ProviderTransport), ProviderIdentity: compilerRouteActiveIdentityV2(declaration.Provider), TransportBinding: bindings[2], ProviderBinding: bindings[6]}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return inventory
}

func routeGovernanceCatalogV2(t *testing.T, input assemblycontract.AssemblyInputV1) runtimeports.GovernanceCatalogV2 {
	t.Helper()
	registrations := make([]runtimeports.GovernanceRegistrationV2, 0, len(input.ComponentManifests))
	for _, manifest := range input.ComponentManifests {
		capabilities := make([]runtimeports.CapabilityNameV2, 0, len(manifest.ProvidedCapabilities))
		for _, capability := range manifest.ProvidedCapabilities {
			capabilities = append(capabilities, capability.Capability)
		}
		extensions := make([]runtimeports.ExtensionPolicyV2, 0, len(manifest.Extensions))
		for _, extension := range manifest.Extensions {
			extensions = append(extensions, runtimeports.ExtensionPolicyV2{Key: extension.Key})
		}
		registrations = append(registrations, runtimeports.GovernanceRegistrationV2{Kind: manifest.Kind, Category: manifest.GovernanceCategory, Capabilities: capabilities, Schemas: append([]runtimeports.SchemaRefV2(nil), manifest.Schemas...), ExtensionPolicies: extensions, AllowedLocalities: []runtimeports.LocalityV2{manifest.Locality}, AllowedConformance: []runtimeports.ConformanceLevel{manifest.Conformance}})
	}
	catalog := runtimeports.GovernanceCatalogV2{Registrations: registrations}
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestValidateControlledOperationProviderWiringRejectsRawAliasAndV1V2V2(t *testing.T) {
	t.Parallel()
	declaration := compilerRouteDeclarationV2(t)
	bindings := compilerRouteBindingsV2(declaration)
	inventory := compilerRouteInventoryV2(t, declaration, bindings)
	now := time.Unix(0, inventory.CheckedUnixNano).Add(time.Second)
	if err := ValidateControlledOperationProviderWiringV2(declaration, compilerRouteActiveIdentityV2(declaration.ProviderTransport), compilerRouteActiveIdentityV2(declaration.Provider), inventory, bindings, compilerRouteDigestV2("input"), compilerRouteDigestV2("graph"), now.UnixNano()); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*assemblycontract.ControlledOperationProviderRouteWiringInventoryV2)
	}{
		{"raw-alias-edge", func(value *assemblycontract.ControlledOperationProviderRouteWiringInventoryV2) {
			value.Edges = append(value.Edges, value.Edges[0])
		}},
		{"v1-v2-dual-active", func(value *assemblycontract.ControlledOperationProviderRouteWiringInventoryV2) {
			route := value.ActiveRoutes[0]
			route.Version, route.RouteID = "v1", "legacy-route"
			value.ActiveRoutes = append(value.ActiveRoutes, route)
		}},
		{"same-matrix-different-provider", func(value *assemblycontract.ControlledOperationProviderRouteWiringInventoryV2) {
			route := value.ActiveRoutes[0]
			route.RouteID = "other-route"
			route.ProviderBinding.ArtifactDigest = compilerRouteDigestV2("other-provider")
			value.ActiveRoutes = append(value.ActiveRoutes, route)
		}},
		{"transport-port-digest-drift", func(value *assemblycontract.ControlledOperationProviderRouteWiringInventoryV2) {
			value.ActiveRoutes[0].TransportIdentity.PortSpecDigest = compilerRouteDigestV2("drifted-port")
		}},
		{"provider-conflict-domain-drift", func(value *assemblycontract.ControlledOperationProviderRouteWiringInventoryV2) {
			value.ActiveRoutes[0].ProviderIdentity.ConflictDomain = "drifted-domain"
		}},
		{"transport-binding-drift", func(value *assemblycontract.ControlledOperationProviderRouteWiringInventoryV2) {
			value.ActiveRoutes[0].TransportBinding.ArtifactDigest = compilerRouteDigestV2("drifted-binding")
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			changed := inventory
			changed.Edges = append([]assemblycontract.ControlledOperationProviderRouteWiringEdgeV2(nil), inventory.Edges...)
			changed.ActiveRoutes = append([]assemblycontract.ControlledOperationProviderActiveRouteRecordV2(nil), inventory.ActiveRoutes...)
			testCase.mutate(&changed)
			changed.Digest = ""
			changed, err := assemblycontract.SealControlledOperationProviderRouteWiringInventoryV2(changed)
			if err != nil {
				if !core.HasCategory(err, core.ErrorConflict) {
					t.Fatalf("seal: %v", err)
				}
				return
			}
			err = ValidateControlledOperationProviderWiringV2(declaration, compilerRouteActiveIdentityV2(declaration.ProviderTransport), compilerRouteActiveIdentityV2(declaration.Provider), changed, bindings, compilerRouteDigestV2("input"), compilerRouteDigestV2("graph"), now.UnixNano())
			if !core.HasCategory(err, core.ErrorConflict) {
				t.Fatalf("got %v", err)
			}
			if testCase.name == "v1-v2-dual-active" || testCase.name == "same-matrix-different-provider" {
				var conflict *assemblycontract.ControlledOperationProviderRouteConflictErrorV2
				if !errors.As(err, &conflict) || conflict.Conflict.Phase != assemblycontract.ControlledOperationProviderRoutePostbindingPhaseV2 || conflict.Conflict.AssemblyInputDigest.Validate() != nil || conflict.Conflict.GraphDigest.Validate() != nil || conflict.Conflict.WiringInventoryDigest != changed.Digest || conflict.Conflict.Validate() != nil {
					t.Fatalf("active-route conflict is not structured: %v", err)
				}
				for name, mutate := range map[string]func(*assemblycontract.ControlledOperationProviderRouteConflictV2){
					"missing-assembly-input": func(value *assemblycontract.ControlledOperationProviderRouteConflictV2) {
						value.AssemblyInputDigest = ""
					},
					"missing-graph": func(value *assemblycontract.ControlledOperationProviderRouteConflictV2) { value.GraphDigest = "" },
					"missing-wiring": func(value *assemblycontract.ControlledOperationProviderRouteConflictV2) {
						value.WiringInventoryDigest = ""
					},
					"wrong-prebinding-phase": func(value *assemblycontract.ControlledOperationProviderRouteConflictV2) {
						value.Phase = assemblycontract.ControlledOperationProviderRoutePrebindingPhaseV2
					},
				} {
					tampered := conflict.Conflict
					tampered.ConflictDigest = ""
					mutate(&tampered)
					if _, sealErr := assemblycontract.SealControlledOperationProviderRouteConflictV2(tampered); sealErr == nil {
						t.Fatalf("postbinding Conflict accepted invalid %s provenance", name)
					}
				}
			}
		})
	}
}

func TestCompileControlledOperationProviderRouteResolvesLegacyRouteThroughOwnerCurrentReaderV2(t *testing.T) {
	t.Parallel()
	input := routeCompileInputV2(t)
	declaration := compilerRouteDeclarationFromInputV2(t, input)
	transportIdentity, providerIdentity, err := expectedControlledOperationProviderRouteIdentitiesV2(input.ComponentManifests, input.PortSpecs, input.Modules, input.ProviderBindingCandidates, declaration)
	if err != nil {
		t.Fatal(err)
	}
	bindings := compilerRouteBindingsV2(declaration)
	legacyRecord := assemblycontract.ControlledOperationProviderActiveRouteRecordV2{
		Version: "v1", RouteID: "legacy-route", DeclarationRef: declaration.RefV2(), Matrix: declaration.Matrix, Active: true,
		TransportIdentity: transportIdentity, ProviderIdentity: providerIdentity, TransportBinding: bindings[2], ProviderBinding: bindings[6],
	}
	bindingRef, err := ControlledOperationProviderLegacyRouteBindingRefV2(legacyRecord)
	if err != nil {
		t.Fatal(err)
	}
	input.RouteBindings = []assemblycontract.ObjectRefV1{bindingRef}
	input, err = assemblycontract.SealAssemblyInputV1(input)
	if err != nil {
		t.Fatal(err)
	}
	legacyWiring := compilerRouteInventoryV2(t, declaration, bindings)
	legacyWiring.ActiveRoutes = append(legacyWiring.ActiveRoutes, legacyRecord)
	legacyWiring.Digest = ""
	legacyWiring, err = assemblycontract.SealControlledOperationProviderRouteWiringInventoryV2(legacyWiring)
	if err != nil {
		t.Fatal(err)
	}
	fact, err := SealControlledOperationProviderLegacyActiveRouteFactV2(ControlledOperationProviderLegacyActiveRouteFactV2{
		RouteBindingRef: bindingRef,
		State:           ControlledOperationProviderLegacyRouteActiveV2, Record: legacyRecord, WiringInventory: legacyWiring,
		CheckedUnixNano: legacyWiring.CheckedUnixNano, ExpiresUnixNano: legacyWiring.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	currentClock := func() time.Time { return time.Unix(0, fact.CheckedUnixNano).Add(time.Second) }
	_, err = CompileControlledOperationProviderRouteWithLegacyCurrentReaderV2(context.Background(), input, routeGovernanceCatalogV2(t, input), &compilerRouteLegacyCurrentReaderV2{facts: []ControlledOperationProviderLegacyActiveRouteFactV2{fact}}, currentClock)
	var conflict *assemblycontract.ControlledOperationProviderRouteConflictErrorV2
	if !errors.As(err, &conflict) || conflict.Conflict.ConflictCode != assemblycontract.ControlledOperationProviderRouteActiveVersionConflictV2 || conflict.Conflict.Phase != assemblycontract.ControlledOperationProviderRoutePrebindingPhaseV2 || conflict.Conflict.AssemblyInputDigest != input.Digest || conflict.Conflict.GraphDigest != "" || conflict.Conflict.WiringInventoryDigest != "" || conflict.Conflict.Validate() != nil || conflict.Conflict.Left.TransportBinding != nil || conflict.Conflict.Right.TransportBinding == nil {
		t.Fatalf("legacy active route did not produce an exact structured version conflict: %v", err)
	}
	tamperedConflict := conflict.Conflict
	tamperedConflict.ConflictDigest = ""
	tamperedConflict.GraphDigest = compilerRouteDigestV2("forbidden-prebinding-graph")
	if _, sealErr := assemblycontract.SealControlledOperationProviderRouteConflictV2(tamperedConflict); sealErr == nil {
		t.Fatal("prebinding V1 conflict accepted postbinding Graph provenance")
	}
	if _, err := CompileControlledOperationProviderRouteV2(input, routeGovernanceCatalogV2(t, input)); !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("legacy RouteBinding without owner current Reader did not fail closed: %v", err)
	}
	if _, err := CompileControlledOperationProviderRouteWithLegacyCurrentReaderV2(context.Background(), input, routeGovernanceCatalogV2(t, input), &compilerRouteLegacyCurrentReaderV2{facts: []ControlledOperationProviderLegacyActiveRouteFactV2{fact}}, func() time.Time { return time.Unix(0, fact.ExpiresUnixNano) }); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expired legacy current proof was accepted: %v", err)
	}
	if _, err := CompileControlledOperationProviderRouteWithLegacyCurrentReaderV2(context.Background(), input, routeGovernanceCatalogV2(t, input), &compilerRouteLegacyCurrentReaderV2{facts: []ControlledOperationProviderLegacyActiveRouteFactV2{fact}}, func() time.Time { return time.Unix(0, fact.CheckedUnixNano).Add(-time.Nanosecond) }); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("legacy current proof accepted a regressed clock: %v", err)
	}
	rollbackTimes := []time.Time{
		time.Unix(0, fact.CheckedUnixNano).Add(50 * time.Second),
		time.Unix(0, fact.CheckedUnixNano).Add(10 * time.Second),
	}
	rollbackIndex := 0
	rollbackClock := func() time.Time {
		value := rollbackTimes[rollbackIndex]
		rollbackIndex++
		return value
	}
	if _, err := CompileControlledOperationProviderRouteWithLegacyCurrentReaderV2(context.Background(), input, routeGovernanceCatalogV2(t, input), &compilerRouteLegacyCurrentReaderV2{facts: []ControlledOperationProviderLegacyActiveRouteFactV2{fact}}, rollbackClock); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("legacy current proof accepted an in-window S2 clock rollback: %v", err)
	}
	driftedProof := fact
	driftedProof.Digest = ""
	driftedProof.WiringInventory.Digest = compilerRouteDigestV2("drifted-wiring-proof")
	if _, err := SealControlledOperationProviderLegacyActiveRouteFactV2(driftedProof); err == nil {
		t.Fatal("legacy route accepted a drifted sealed-wiring proof")
	}
	unknownState := fact
	unknownState.Digest = ""
	unknownState.State = ControlledOperationProviderLegacyRouteStateV2("paused")
	if _, err := SealControlledOperationProviderLegacyActiveRouteFactV2(unknownState); err == nil {
		t.Fatal("legacy route accepted a state outside active|inactive|revoked")
	}
	stateFlagDrift := fact
	stateFlagDrift.Digest = ""
	stateFlagDrift.State = ControlledOperationProviderLegacyRouteInactiveV2
	if _, err := SealControlledOperationProviderLegacyActiveRouteFactV2(stateFlagDrift); err == nil {
		t.Fatal("legacy route accepted inactive state with an active record")
	}
	missingProof := fact
	missingProof.Digest = ""
	missingProof.WiringInventory = assemblycontract.ControlledOperationProviderRouteWiringInventoryV2{}
	if _, err := SealControlledOperationProviderLegacyActiveRouteFactV2(missingProof); err == nil {
		t.Fatal("legacy route accepted a missing sealed-wiring proof")
	}
	unrelatedBinding := fact
	unrelatedBinding.Digest = ""
	unrelatedBinding.RouteBindingRef = compilerRouteRefV2("unrelated-binding")
	if _, err := SealControlledOperationProviderLegacyActiveRouteFactV2(unrelatedBinding); err == nil {
		t.Fatal("legacy route accepted a RouteBindingRef unrelated to its exact record")
	}
	driftedInventory := legacyWiring
	driftedInventory.InventoryID = "route-inventory-s2"
	driftedInventory.Digest = ""
	driftedInventory, err = assemblycontract.SealControlledOperationProviderRouteWiringInventoryV2(driftedInventory)
	if err != nil {
		t.Fatal(err)
	}
	driftedS2, err := SealControlledOperationProviderLegacyActiveRouteFactV2(ControlledOperationProviderLegacyActiveRouteFactV2{
		RouteBindingRef: bindingRef, State: ControlledOperationProviderLegacyRouteActiveV2, Record: legacyRecord, WiringInventory: driftedInventory,
		CheckedUnixNano: driftedInventory.CheckedUnixNano, ExpiresUnixNano: driftedInventory.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileControlledOperationProviderRouteWithLegacyCurrentReaderV2(context.Background(), input, routeGovernanceCatalogV2(t, input), &compilerRouteLegacyCurrentReaderV2{facts: []ControlledOperationProviderLegacyActiveRouteFactV2{fact, driftedS2}}, currentClock); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("legacy current S1/S2 drift was accepted: %v", err)
	}
	for _, state := range []ControlledOperationProviderLegacyRouteStateV2{ControlledOperationProviderLegacyRouteInactiveV2, ControlledOperationProviderLegacyRouteRevokedV2} {
		inactiveRecord := legacyRecord
		inactiveRecord.Active = false
		inactiveWiring := compilerRouteInventoryV2(t, declaration, bindings)
		inactiveWiring.ActiveRoutes = append(inactiveWiring.ActiveRoutes, inactiveRecord)
		inactiveWiring.Digest = ""
		inactiveWiring, err = assemblycontract.SealControlledOperationProviderRouteWiringInventoryV2(inactiveWiring)
		if err != nil {
			t.Fatal(err)
		}
		inactiveFact, sealErr := SealControlledOperationProviderLegacyActiveRouteFactV2(ControlledOperationProviderLegacyActiveRouteFactV2{RouteBindingRef: bindingRef, State: state, Record: inactiveRecord, WiringInventory: inactiveWiring, CheckedUnixNano: inactiveWiring.CheckedUnixNano, ExpiresUnixNano: inactiveWiring.ExpiresUnixNano})
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		if _, compileErr := CompileControlledOperationProviderRouteWithLegacyCurrentReaderV2(context.Background(), input, routeGovernanceCatalogV2(t, input), &compilerRouteLegacyCurrentReaderV2{facts: []ControlledOperationProviderLegacyActiveRouteFactV2{inactiveFact}}, currentClock); compileErr != nil {
			t.Fatalf("legacy %s route with exact absence proof was not released: %v", state, compileErr)
		}
		invalidProof := inactiveFact
		invalidProof.Digest = ""
		invalidProof.WiringInventory = legacyWiring
		invalidProof.CheckedUnixNano = legacyWiring.CheckedUnixNano
		invalidProof.ExpiresUnixNano = legacyWiring.ExpiresUnixNano
		if _, sealErr := SealControlledOperationProviderLegacyActiveRouteFactV2(invalidProof); sealErr == nil {
			t.Fatalf("legacy %s route was released while sealed wiring still contained an active route", state)
		}
		missingTarget := inactiveFact
		missingTarget.Digest = ""
		missingTarget.WiringInventory = compilerRouteInventoryV2(t, declaration, bindings)
		missingTarget.CheckedUnixNano = missingTarget.WiringInventory.CheckedUnixNano
		missingTarget.ExpiresUnixNano = missingTarget.WiringInventory.ExpiresUnixNano
		if _, sealErr := SealControlledOperationProviderLegacyActiveRouteFactV2(missingTarget); sealErr == nil {
			t.Fatalf("legacy %s absence proof omitted its target binding", state)
		}
		aliasActive := legacyRecord
		aliasActive.RouteID = "legacy-route-alias-active"
		aliasProof := inactiveFact
		aliasProof.Digest = ""
		aliasProof.WiringInventory.ActiveRoutes = append(aliasProof.WiringInventory.ActiveRoutes, aliasActive)
		aliasProof.WiringInventory.Digest = ""
		aliasProof.WiringInventory, sealErr = assemblycontract.SealControlledOperationProviderRouteWiringInventoryV2(aliasProof.WiringInventory)
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		if _, sealErr := SealControlledOperationProviderLegacyActiveRouteFactV2(aliasProof); sealErr == nil {
			t.Fatalf("legacy %s route hid another active V1 route with the same matrix/alias identity", state)
		}
		matrixActive := legacyRecord
		matrixActive.RouteID = "legacy-route-matrix-active"
		matrixActive.TransportIdentity.CandidateID = "praxis.route/other-transport-candidate"
		matrixActive.ProviderIdentity.CandidateID = "praxis.route/other-provider-candidate"
		matrixActive.TransportBinding.BindingSetID = "binding-route-other"
		matrixActive.ProviderBinding.BindingSetID = "binding-route-other"
		matrixProof := inactiveFact
		matrixProof.Digest = ""
		matrixProof.WiringInventory.ActiveRoutes = append(matrixProof.WiringInventory.ActiveRoutes, matrixActive)
		matrixProof.WiringInventory.Digest = ""
		matrixProof.WiringInventory, sealErr = assemblycontract.SealControlledOperationProviderRouteWiringInventoryV2(matrixProof.WiringInventory)
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		if _, sealErr := SealControlledOperationProviderLegacyActiveRouteFactV2(matrixProof); sealErr == nil {
			t.Fatalf("legacy %s route hid another active V1 route in the same closed matrix", state)
		}
	}
}

func TestMergeControlledOperationProviderRouteDeclarationsIsDeterministicAndStructuredV2(t *testing.T) {
	t.Parallel()
	declaration := compilerRouteDeclarationV2(t)
	got, err := MergeControlledOperationProviderRouteDeclarationsV2([]assemblycontract.ControlledOperationProviderRouteDeclarationV2{declaration, declaration})
	if err != nil || got != declaration {
		t.Fatalf("exact duplicate merge got %+v, %v", got, err)
	}
	drifted := declaration
	drifted.PublisherComponent = "praxis.harness/other-assembly"
	drifted.DeclarationDigest = ""
	drifted, err = assemblycontract.SealControlledOperationProviderRouteDeclarationV2(drifted)
	if err != nil {
		t.Fatal(err)
	}
	_, err = MergeControlledOperationProviderRouteDeclarationsV2([]assemblycontract.ControlledOperationProviderRouteDeclarationV2{drifted, declaration})
	var conflict *assemblycontract.ControlledOperationProviderRouteConflictErrorV2
	if !errors.As(err, &conflict) || conflict.Conflict.ConflictCode != assemblycontract.ControlledOperationProviderRouteDeclarationConflictV2 || conflict.Conflict.Phase != assemblycontract.ControlledOperationProviderRouteDeclarationPhaseV2 || conflict.Conflict.AssemblyInputDigest != "" || conflict.Conflict.GraphDigest != "" || conflict.Conflict.WiringInventoryDigest != "" || conflict.Conflict.Validate() != nil {
		t.Fatalf("different declaration merge is not a sealed structured conflict: %v", err)
	}
}

func TestScanControlledOperationProviderBypassesRejectsNormalizedAliasesV2(t *testing.T) {
	t.Parallel()
	base := routeCompileInputV2(t)
	declaration := compilerRouteDeclarationFromInputV2(t, base)
	manifests, ports, modules, candidates := compilerRouteIndexesV2(base)

	t.Run("effect-free-exact-port", func(t *testing.T) {
		input := base
		protected := candidates[declaration.ProviderTransport.CandidateID]
		alias := protected
		alias.CandidateID = "praxis.route/effect-free-exact-port-alias"
		alias.ProviderRef = compilerRouteRefV2("praxis.route/effect-free-provider-alias")
		alias.Digest = ""
		var err error
		alias.Digest, err = assemblycontract.ProviderBindingCandidateDigestV1(alias)
		if err != nil {
			t.Fatal(err)
		}
		input.ProviderBindingCandidates = append(append([]assemblycontract.ProviderBindingCandidateV1{}, base.ProviderBindingCandidates...), alias)
		assertProviderAliasConflictV2(t, scanControlledOperationProviderBypassesV2(input, declaration, manifests, ports, modules, candidates), assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasCandidateSurfaceV2, Ref: alias.CandidateID, ModuleRef: alias.ModuleRef, PortSpecRef: alias.PortSpecRef})
	})

	t.Run("phase-module-alias", func(t *testing.T) {
		input := base
		aliasModule := modules[declaration.ProviderTransport.ModuleRef]
		aliasModule.ModuleID = "praxis.route/provider-module-alias"
		localModules := cloneRouteModulesV2(modules)
		localModules[aliasModule.ModuleID] = aliasModule
		input.PhaseContributions = append(append([]assemblycontract.PhaseContributionV1{}, base.PhaseContributions...), assemblycontract.PhaseContributionV1{ContributionID: "praxis.route/phase-module-alias", HookFaceRef: "praxis.route/phase-module-hook", ModuleRef: aliasModule.ModuleID, HandlerDescriptorRef: compilerRouteRefV2("praxis.route/unrelated-handler"), Capability: assemblycontract.PhaseGateV1})
		assertProviderAliasConflictV2(t, scanControlledOperationProviderBypassesV2(input, declaration, manifests, ports, localModules, candidates), assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasPhaseSurfaceV2, Ref: "praxis.route/phase-module-alias", ModuleRef: aliasModule.ModuleID, PortSpecRef: "praxis.route/phase-module-hook", Capability: string(assemblycontract.PhaseGateV1)})
	})

	t.Run("phase-handler-alias", func(t *testing.T) {
		input := base
		handler := declaration.Provider.ProviderRef
		handler.ID = "praxis.route/provider-handler-alias"
		input.PhaseContributions = append(append([]assemblycontract.PhaseContributionV1{}, base.PhaseContributions...), assemblycontract.PhaseContributionV1{ContributionID: "praxis.route/phase-handler-alias", HookFaceRef: "praxis.route/phase-handler-hook", ModuleRef: "praxis.route/unrelated-module", HandlerDescriptorRef: handler, Capability: assemblycontract.PhaseGateV1})
		assertProviderAliasConflictV2(t, scanControlledOperationProviderBypassesV2(input, declaration, manifests, ports, modules, candidates), assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasPhaseSurfaceV2, Ref: "praxis.route/phase-handler-alias", ModuleRef: "praxis.route/unrelated-module", PortSpecRef: "praxis.route/phase-handler-hook", Capability: string(assemblycontract.PhaseGateV1)})
	})
}

func assertProviderAliasConflictV2(t *testing.T, err error, expectedSurface assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2) {
	t.Helper()
	var conflict *assemblycontract.ControlledOperationProviderRouteConflictErrorV2
	expectedSurface, expectedErr := assemblycontract.SealControlledOperationProviderRouteAliasSurfaceV2(expectedSurface)
	if expectedErr != nil {
		t.Fatal(expectedErr)
	}
	if !errors.As(err, &conflict) || conflict.Conflict.ConflictCode != assemblycontract.ControlledOperationProviderRouteAliasConflictV2 || conflict.Conflict.Phase != assemblycontract.ControlledOperationProviderRoutePrebindingPhaseV2 || conflict.Conflict.AssemblyInputDigest.Validate() != nil || conflict.Conflict.GraphDigest != "" || conflict.Conflict.WiringInventoryDigest != "" || conflict.Conflict.AliasSurface == nil || *conflict.Conflict.AliasSurface != expectedSurface || conflict.Conflict.AliasSurface.Validate() != nil || conflict.Conflict.Validate() != nil {
		if conflict != nil {
			t.Fatalf("Provider alias is not a sealed structured conflict: %v phase=%s surface=%+v", err, conflict.Conflict.Phase, conflict.Conflict.AliasSurface)
		}
		t.Fatalf("Provider alias is not a sealed structured conflict: %v", err)
	}
	if expectedSurface.Kind != assemblycontract.ControlledOperationProviderRouteAliasCandidateSurfaceV2 && (conflict.Conflict.Right.ProviderTransport != (assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{}) || conflict.Conflict.Right.Provider != (assemblycontract.ControlledOperationProviderRouteNormalizedIdentityV2{})) {
		t.Fatal("non-candidate alias fell back to protected normalized identities")
	}
	tampered := conflict.Conflict
	tampered.ConflictDigest = ""
	tampered.GraphDigest = compilerRouteDigestV2("forbidden-prebinding-graph")
	if _, sealErr := assemblycontract.SealControlledOperationProviderRouteConflictV2(tampered); sealErr == nil {
		t.Fatal("prebinding alias accepted postbinding graph provenance")
	}
	tampered = conflict.Conflict
	tampered.ConflictDigest = ""
	tampered.AliasSurface.CanonicalDigest = compilerRouteDigestV2("drifted-alias-surface")
	if _, sealErr := assemblycontract.SealControlledOperationProviderRouteConflictV2(tampered); sealErr == nil {
		t.Fatal("alias conflict accepted a drifted surface digest")
	}
	tampered = conflict.Conflict
	tampered.ConflictDigest = ""
	tampered.AliasSurface.CanonicalDigest = ""
	switch tampered.AliasSurface.Kind {
	case assemblycontract.ControlledOperationProviderRouteAliasCandidateSurfaceV2:
		tampered.AliasSurface.Capability = "forbidden-candidate-capability"
	case assemblycontract.ControlledOperationProviderRouteAliasPortSurfaceV2:
		tampered.AliasSurface.ModuleRef = "forbidden-port-module"
	case assemblycontract.ControlledOperationProviderRouteAliasSlotSurfaceV2:
		tampered.AliasSurface.PortSpecRef = ""
	case assemblycontract.ControlledOperationProviderRouteAliasFactorySurfaceV2:
		tampered.AliasSurface.PortSpecRef = "forbidden-factory-port"
	case assemblycontract.ControlledOperationProviderRouteAliasDependencySurfaceV2:
		tampered.AliasSurface.ModuleRef = "forbidden-dependency-module"
	case assemblycontract.ControlledOperationProviderRouteAliasPhaseSurfaceV2:
		tampered.AliasSurface.Capability = ""
	}
	if _, sealErr := assemblycontract.SealControlledOperationProviderRouteConflictV2(tampered); sealErr == nil {
		t.Fatalf("closed %s AliasSurface accepted a wrong coordinate shape", tampered.AliasSurface.Kind)
	}
}

func compilerRouteDeclarationFromInputV2(t *testing.T, input assemblycontract.AssemblyInputV1) assemblycontract.ControlledOperationProviderRouteDeclarationV2 {
	t.Helper()
	for _, manifest := range input.ComponentManifests {
		for _, extension := range manifest.Extensions {
			if extension.Key == assemblycontract.ControlledOperationProviderRouteExtensionKeyV2 && extension.Payload.Inline != nil {
				declaration, err := assemblycontract.DecodeControlledOperationProviderRouteDeclarationV2(extension.Payload.Inline)
				if err != nil {
					t.Fatal(err)
				}
				return declaration
			}
		}
	}
	t.Fatal("controlled Provider route declaration is absent")
	return assemblycontract.ControlledOperationProviderRouteDeclarationV2{}
}

func compilerRouteIndexesV2(input assemblycontract.AssemblyInputV1) (map[string]runtimeports.ComponentManifestV2, map[string]assemblycontract.PortSpecV1, map[string]assemblycontract.ModuleDescriptorV1, map[string]assemblycontract.ProviderBindingCandidateV1) {
	manifests := map[string]runtimeports.ComponentManifestV2{}
	ports := map[string]assemblycontract.PortSpecV1{}
	modules := map[string]assemblycontract.ModuleDescriptorV1{}
	candidates := map[string]assemblycontract.ProviderBindingCandidateV1{}
	for _, value := range input.ComponentManifests {
		manifests[string(value.ComponentID)] = value
	}
	for _, value := range input.PortSpecs {
		ports[value.PortID] = value
	}
	for _, value := range input.Modules {
		modules[value.ModuleID] = value
	}
	for _, value := range input.ProviderBindingCandidates {
		candidates[value.CandidateID] = value
	}
	return manifests, ports, modules, candidates
}

func cloneRouteModulesV2(values map[string]assemblycontract.ModuleDescriptorV1) map[string]assemblycontract.ModuleDescriptorV1 {
	result := make(map[string]assemblycontract.ModuleDescriptorV1, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func TestScanControlledOperationProviderBypassesRejectsEveryPublicSurfaceV2(t *testing.T) {
	t.Parallel()
	base := routeCompileInputV2(t)
	declaration := compilerRouteDeclarationFromInputV2(t, base)
	manifests, ports, modules, candidates := compilerRouteIndexesV2(base)
	transportCandidate := candidates[declaration.ProviderTransport.CandidateID]

	tests := []struct {
		name            string
		expectedSurface assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2
		mutate          func(*assemblycontract.AssemblyInputV1)
	}{
		{"alias-candidate", assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasCandidateSurfaceV2, Ref: "praxis.route/alias", ModuleRef: transportCandidate.ModuleRef, PortSpecRef: transportCandidate.PortSpecRef}, func(value *assemblycontract.AssemblyInputV1) {
			alias := transportCandidate
			alias.CandidateID = "praxis.route/alias"
			value.ProviderBindingCandidates = append(value.ProviderBindingCandidates, alias)
		}},
		{"port", assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasPortSurfaceV2, Ref: "praxis.route/alias-port", PortSpecRef: "praxis.route/alias-port", Capability: string(runtimeports.ControlledOperationProviderTransportCapabilityV2)}, func(_ *assemblycontract.AssemblyInputV1) {
			ports["praxis.route/alias-port"] = assemblycontract.PortSpecV1{PortID: "praxis.route/alias-port", OwnerCapability: runtimeports.ControlledOperationProviderTransportCapabilityV2, ConflictDomainRule: "transport-domain"}
		}},
		{"alias-module-component-port", assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasCandidateSurfaceV2, Ref: "praxis.route/alias-structured", ModuleRef: "praxis.route/alias-module", PortSpecRef: "praxis.route/alias-port"}, func(value *assemblycontract.AssemblyInputV1) {
			aliasModule := assemblycontract.ModuleDescriptorV1{ModuleID: "praxis.route/alias-module", ArtifactDigest: declaration.ProviderTransport.ArtifactDigest, ComponentManifestRef: assemblycontract.ObjectRefV1{ID: string(declaration.ProviderTransport.ComponentID), Revision: 1, Digest: declaration.ProviderTransport.ManifestDigest}}
			modules[aliasModule.ModuleID] = aliasModule
			ports["praxis.route/alias-port"] = assemblycontract.PortSpecV1{PortID: "praxis.route/alias-port", OwnerCapability: declaration.ProviderTransport.Capability, ConflictDomainRule: "transport-domain"}
			value.ProviderBindingCandidates = append(value.ProviderBindingCandidates, assemblycontract.ProviderBindingCandidateV1{CandidateID: "praxis.route/alias-structured", ModuleRef: aliasModule.ModuleID, PortSpecRef: "praxis.route/alias-port", ProviderRef: compilerRouteRefV2("praxis.route/other-provider")})
		}},
		{"slot", assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasSlotSurfaceV2, Ref: "praxis.route/alias-slot", ModuleRef: declaration.ProviderTransport.ModuleRef, PortSpecRef: declaration.ProviderTransport.PortSpecRef, Capability: string(declaration.ProviderTransport.Capability)}, func(value *assemblycontract.AssemblyInputV1) {
			value.SlotContributions = []assemblycontract.SlotContributionV1{{ContributionID: "praxis.route/alias-slot", ModuleRef: declaration.ProviderTransport.ModuleRef, SlotRef: "praxis.route/alias-slot-ref", PortSpecRef: declaration.ProviderTransport.PortSpecRef, CapabilityRef: declaration.ProviderTransport.Capability, ProviderCandidateRef: declaration.ProviderTransport.CandidateID}}
		}},
		{"factory", assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasFactorySurfaceV2, Ref: "praxis.route/alias-factory", ModuleRef: declaration.Provider.ModuleRef, Capability: string(declaration.Provider.Capability)}, func(value *assemblycontract.AssemblyInputV1) {
			value.Factories = []assemblycontract.ModuleFactoryDescriptorV1{{FactoryID: "praxis.route/alias-factory", ModuleRef: declaration.Provider.ModuleRef, OutputCapability: declaration.Provider.Capability}}
		}},
		{"dependency", assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasDependencySurfaceV2, Ref: "praxis.route/alias-dependency-from", PortSpecRef: "praxis.route/alias-dependency-to", Capability: string(runtimeports.ControlledOperationProviderTransportCapabilityV2)}, func(value *assemblycontract.AssemblyInputV1) {
			value.Dependencies = []assemblycontract.DependencySpecV1{{FromRef: "praxis.route/alias-dependency-from", ToRef: "praxis.route/alias-dependency-to", Capability: runtimeports.ControlledOperationProviderTransportCapabilityV2}}
		}},
		{"phase", assemblycontract.ControlledOperationProviderRouteAliasSurfaceV2{Kind: assemblycontract.ControlledOperationProviderRouteAliasPhaseSurfaceV2, Ref: "praxis.route/alias-phase", ModuleRef: declaration.Provider.ModuleRef, PortSpecRef: "praxis.route/alias-hook", Capability: string(assemblycontract.PhaseGateV1)}, func(value *assemblycontract.AssemblyInputV1) {
			value.PhaseContributions = []assemblycontract.PhaseContributionV1{{ContributionID: "praxis.route/alias-phase", ModuleRef: declaration.Provider.ModuleRef, HookFaceRef: "praxis.route/alias-hook", HandlerDescriptorRef: declaration.Provider.ProviderRef, Capability: assemblycontract.PhaseGateV1}}
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			input := base
			input.ProviderBindingCandidates = append([]assemblycontract.ProviderBindingCandidateV1(nil), base.ProviderBindingCandidates...)
			testCase.mutate(&input)
			err := scanControlledOperationProviderBypassesV2(input, declaration, manifests, ports, modules, candidates)
			delete(ports, "praxis.route/alias-port")
			delete(modules, "praxis.route/alias-module")
			assertProviderAliasConflictV2(t, err, testCase.expectedSurface)
		})
	}
}

func TestCompileControlledOperationProviderRouteRequiresRegisteredPublisherFactsV2(t *testing.T) {
	t.Parallel()
	input := routeCompileInputV2(t)
	catalog := routeGovernanceCatalogV2(t, input)
	publisherIndex := -1
	for manifestIndex, manifest := range input.ComponentManifests {
		for _, extension := range manifest.Extensions {
			if extension.Key == assemblycontract.ControlledOperationProviderRouteExtensionKeyV2 {
				publisherIndex = manifestIndex
			}
		}
	}
	if publisherIndex < 0 {
		t.Fatal("route publisher manifest is absent")
	}
	for index := range catalog.Registrations {
		if catalog.Registrations[index].Kind == input.ComponentManifests[publisherIndex].Kind {
			catalog.Registrations[index].ExtensionPolicies = nil
		}
	}
	if _, err := CompileControlledOperationProviderRouteV2(input, catalog); err == nil {
		t.Fatal("unregistered required route extension was accepted")
	}

	drifted := input
	drifted.ComponentManifests = append([]runtimeports.ComponentManifestV2(nil), input.ComponentManifests...)
	drifted.Modules = append([]assemblycontract.ModuleDescriptorV1(nil), input.Modules...)
	drifted.ComponentManifests[publisherIndex].SemanticVersion = "1.9.0"
	digest, err := drifted.ComponentManifests[publisherIndex].BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	for index := range drifted.Modules {
		if drifted.Modules[index].ComponentManifestRef.ID == string(drifted.ComponentManifests[publisherIndex].ComponentID) {
			drifted.Modules[index].ComponentManifestRef.Digest = digest
			drifted.Modules[index].SemanticVersion = "1.9.0"
		}
	}
	drifted, err = assemblycontract.SealAssemblyInputV1(drifted)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileControlledOperationProviderRouteV2(drifted, routeGovernanceCatalogV2(t, drifted)); err == nil {
		t.Fatal("publisher outside registered 2.x version was accepted")
	}
}

func TestControlledOperationProviderExtensionChangesManifestDigestWithoutAssemblyInputFieldV2(t *testing.T) {
	t.Parallel()
	base := runtimeports.ComponentManifestV2{
		ContractVersion: runtimeports.BindingContractVersionV2, ComponentID: "praxis.harness/assembly", Kind: "praxis.harness/assembly", GovernanceCategory: "praxis.harness/assembly", SemanticVersion: "2.0.0", ArtifactDigest: compilerRouteDigestV2("artifact"),
		Contract: runtimeports.ContractBindingV2{Name: "praxis.harness/assembly", Version: "2.0.0", Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}}, Schemas: []runtimeports.SchemaRefV2{}, Locality: runtimeports.LocalityHostControlPlane,
		Dependencies: []runtimeports.ComponentDependencyV2{}, RequiredCapabilities: []runtimeports.CapabilityRequirementV2{}, ProvidedCapabilities: []runtimeports.ProvidedCapabilityV2{{Capability: "praxis.harness/assembly", TTLSeconds: 30, Schemas: []runtimeports.SchemaRefV2{}}}, Conformance: runtimeports.ConformanceFullyControlled, ResidualClass: runtimeports.ResidualNone,
		Owners: []runtimeports.OwnerAssignmentV2{{Role: runtimeports.OwnerEffect, OwnerComponentID: "praxis.harness/assembly"}, {Role: runtimeports.OwnerSettlement, OwnerComponentID: "praxis.harness/assembly"}, {Role: runtimeports.OwnerCleanup, OwnerComponentID: "praxis.harness/assembly"}}, Credentials: []runtimeports.CredentialRequirementV2{}, OfflinePolicy: runtimeports.OfflineDenied, Extensions: []runtimeports.GovernanceExtensionV2{}, Annotations: []runtimeports.DisplayAnnotationV2{},
	}
	digestWithout, err := base.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"route":"v2"}`)
	schema := compilerRouteSchemaV2("extension")
	base.Schemas = append(base.Schemas, schema)
	base.Extensions = []runtimeports.GovernanceExtensionV2{{Key: assemblycontract.ControlledOperationProviderRouteExtensionKeyV2, Required: true, Payload: runtimeports.OpaquePayloadV2{Schema: schema, ContentDigest: core.DigestBytes(payload), Length: uint64(len(payload)), Inline: payload, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.route/limit", Digest: compilerRouteDigestV2("limit")}}}}
	digestWith, err := base.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	if digestWith == digestWithout {
		t.Fatal("required extension did not enter ComponentManifest digest")
	}
}

func routeCompileInputV2(t *testing.T) assemblycontract.AssemblyInputV1 {
	t.Helper()
	input := assemblytestkit.ValidInput()
	input.RouteBindings = nil

	componentID := runtimeports.ComponentIDV2("praxis.route/components")
	artifact := compilerRouteDigestV2("route-components-artifact")
	portDefinitions := []struct {
		id  string
		cap runtimeports.CapabilityNameV2
	}{
		{"praxis.route/application-port", "praxis.application/single-call-tool-action"},
		{"praxis.route/tool-adapter-port", runtimeports.ControlledOperationToolAdapterCapabilityV2},
		{"praxis.route/runtime-port", runtimeports.ControlledOperationGatewayCapabilityV2},
		{"praxis.route/gateway-port", runtimeports.ControlledOperationGatewayCapabilityV2},
		{"praxis.route/transport-port", runtimeports.ControlledOperationProviderTransportCapabilityV2},
		{"praxis.route/provider-port", runtimeports.CapabilityNameV2(runtimeports.OperationScopeEvidenceActionEffectKindV3)},
		{"praxis.route/prepared-reader-port", runtimeports.ControlledOperationPreparedReaderCapabilityV2},
		{"praxis.route/boundary-reader-port", runtimeports.ControlledOperationBoundaryReaderCapabilityV2},
		{"praxis.route/inspect-reader-port", runtimeports.ControlledOperationProviderInspectCapabilityV2},
	}
	ports := make([]assemblycontract.PortSpecV1, 0, len(portDefinitions))
	schemas := []runtimeports.SchemaRefV2{}
	capSet := map[runtimeports.CapabilityNameV2]struct{}{}
	for _, definition := range portDefinitions {
		name := strings.ReplaceAll(strings.TrimPrefix(definition.id, "praxis.route/"), "/", "-")
		request, response := compilerRouteSchemaV2(name+"-request"), compilerRouteSchemaV2(name+"-response")
		ports = append(ports, assemblycontract.PortSpecV1{PortID: definition.id, OwnerCapability: definition.cap, RequestSchema: request, ResponseSchema: response, OperationClass: "controlled-route", Idempotency: "inspect-original", FailureSemantics: "fail-closed", Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "1.0.0", MaximumExclusive: "3.0.0"}})
		schemas = append(schemas, request, response)
		capSet[definition.cap] = struct{}{}
	}
	sort.Slice(schemas, func(i, j int) bool { return schemas[i].Key() < schemas[j].Key() })
	provided := make([]runtimeports.ProvidedCapabilityV2, 0, len(capSet))
	moduleCaps := make([]runtimeports.CapabilityNameV2, 0, len(capSet))
	descriptors := make([]assemblycontract.CapabilityDescriptorV1, 0, len(capSet))
	for capability := range capSet {
		provided = append(provided, runtimeports.ProvidedCapabilityV2{Capability: capability, TTLSeconds: 30, Schemas: append([]runtimeports.SchemaRefV2(nil), schemas...)})
		moduleCaps = append(moduleCaps, capability)
		descriptors = append(descriptors, assemblycontract.CapabilityDescriptorV1{Capability: capability, Version: "2.0.0", Schemas: append([]runtimeports.SchemaRefV2(nil), schemas...), Provided: true, TTLSeconds: 30, EffectClass: "owner-controlled", OwnerCapability: capability, Conformance: runtimeports.ConformanceFullyControlled})
	}
	sort.Slice(provided, func(i, j int) bool { return provided[i].Capability < provided[j].Capability })
	sort.Slice(moduleCaps, func(i, j int) bool { return moduleCaps[i] < moduleCaps[j] })
	componentManifest := runtimeports.ComponentManifestV2{
		ContractVersion: runtimeports.BindingContractVersionV2, ComponentID: componentID, Kind: "praxis.route/component", GovernanceCategory: "praxis.route/controlled-provider", SemanticVersion: "2.0.0", ArtifactDigest: artifact,
		Contract: runtimeports.ContractBindingV2{Name: "praxis.route/component", Version: "2.0.0", Compatible: runtimeports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}}, Schemas: schemas, Locality: runtimeports.LocalityHostControlPlane,
		Dependencies: []runtimeports.ComponentDependencyV2{}, RequiredCapabilities: []runtimeports.CapabilityRequirementV2{}, ProvidedCapabilities: provided, Conformance: runtimeports.ConformanceFullyControlled, ResidualClass: runtimeports.ResidualNone,
		Owners: []runtimeports.OwnerAssignmentV2{{Role: runtimeports.OwnerEffect, OwnerComponentID: componentID}, {Role: runtimeports.OwnerSettlement, OwnerComponentID: componentID}, {Role: runtimeports.OwnerCleanup, OwnerComponentID: componentID}}, Credentials: []runtimeports.CredentialRequirementV2{}, OfflinePolicy: runtimeports.OfflineDenied, Extensions: []runtimeports.GovernanceExtensionV2{}, Annotations: []runtimeports.DisplayAnnotationV2{},
	}
	manifestDigest, err := componentManifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	moduleID := "praxis.route/components-module"
	module := assemblycontract.ModuleDescriptorV1{ModuleID: moduleID, Namespace: "praxis.route", SemanticVersion: "2.0.0", ArtifactDigest: artifact, PublisherRef: compilerRouteRefV2("route-publisher"), SourceRef: compilerRouteRefV2("route-source"), ComponentManifestRef: assemblycontract.ObjectRefV1{ID: string(componentID), Revision: 1, Digest: manifestDigest}, Compatibility: assemblycontract.CompatibilityV1{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}, Capabilities: moduleCaps, Schemas: schemas, Locality: runtimeports.LocalityHostControlPlane, ResidualClass: runtimeports.ResidualNone, Owners: componentManifest.Owners}

	portByID := map[string]assemblycontract.PortSpecV1{}
	for _, port := range ports {
		port.ContractVersion = assemblycontract.ContractVersionV1
		portByID[port.PortID] = port
	}
	portRef := func(role assemblycontract.ControlledOperationProviderRouteRoleV2, id string) assemblycontract.ControlledOperationProviderRoutePortRefV2 {
		port := portByID[id]
		digest, err := assemblycontract.PortSpecDigestForControlledOperationProviderRouteV2(port)
		if err != nil {
			t.Fatal(err)
		}
		return assemblycontract.ControlledOperationProviderRoutePortRefV2{Role: role, PortID: id, PortDigest: digest, OwnerCapability: port.OwnerCapability, RequestSchema: port.RequestSchema, ResponseSchema: port.ResponseSchema, ContractVersion: "2.0.0"}
	}
	candidateFor := func(role assemblycontract.ControlledOperationProviderRouteRoleV2, id string) (assemblycontract.ControlledOperationProviderRouteEndpointV2, assemblycontract.ProviderBindingCandidateV1) {
		port := portByID[id]
		candidate := assemblycontract.ProviderBindingCandidateV1{ContractVersion: assemblycontract.ContractVersionV1, CandidateID: id + "-candidate", ModuleRef: moduleID, SlotRef: "runtime.gateway", PortSpecRef: id, ProviderRef: compilerRouteRefV2(id + "-provider")}
		candidate.Digest, err = assemblycontract.ProviderBindingCandidateDigestV1(candidate)
		if err != nil {
			t.Fatal(err)
		}
		return assemblycontract.ControlledOperationProviderRouteEndpointV2{Role: role, ComponentID: componentID, ManifestDigest: manifestDigest, ArtifactDigest: artifact, Capability: port.OwnerCapability, ContractVersion: "2.0.0", Locality: runtimeports.LocalityHostControlPlane, CandidateID: candidate.CandidateID, CandidateDigest: candidate.Digest, ModuleRef: moduleID, PortSpecRef: id, ProviderRef: candidate.ProviderRef}, candidate
	}
	toolAdapter, toolCandidate := candidateFor(assemblycontract.ControlledOperationToolAdapterRoleV2, "praxis.route/tool-adapter-port")
	gateway, gatewayCandidate := candidateFor(assemblycontract.ControlledOperationRuntimeGatewayRoleV2, "praxis.route/gateway-port")
	transport, transportCandidate := candidateFor(assemblycontract.ControlledOperationProviderTransportRoleV2, "praxis.route/transport-port")
	provider, providerCandidate := candidateFor(assemblycontract.ControlledOperationProviderRoleV2, "praxis.route/provider-port")
	readerFor := func(role assemblycontract.ControlledOperationProviderRouteRoleV2, id string) assemblycontract.ControlledOperationProviderRouteReaderRefV2 {
		port := portByID[id]
		digest, err := assemblycontract.PortSpecDigestForControlledOperationProviderRouteV2(port)
		if err != nil {
			t.Fatal(err)
		}
		return assemblycontract.ControlledOperationProviderRouteReaderRefV2{Role: role, ComponentID: componentID, ManifestDigest: manifestDigest, ArtifactDigest: artifact, Capability: port.OwnerCapability, PortSpecID: id, PortSpecDigest: digest, RequestSchema: port.RequestSchema, ProjectionSchema: port.ResponseSchema, ReadOnly: true, NoExecute: true}
	}
	declaration, err := assemblycontract.SealControlledOperationProviderRouteDeclarationV2(assemblycontract.ControlledOperationProviderRouteDeclarationV2{
		RouteID: "praxis.route/compiled-single-tool", Revision: 1, PublisherComponent: input.ComponentManifests[0].ComponentID, Matrix: runtimeports.OperationScopeEvidenceActionMatrixV3(),
		ApplicationToolPort: portRef(assemblycontract.ControlledOperationApplicationToolPortRoleV2, "praxis.route/application-port"), ToolAdapter: toolAdapter,
		RuntimeGovernancePort: portRef(assemblycontract.ControlledOperationRuntimeGovernanceRoleV2, "praxis.route/runtime-port"), Gateway: gateway, ProviderTransport: transport, Provider: provider,
		PreparedCurrentReader: readerFor(assemblycontract.ControlledOperationPreparedReaderRoleV2, "praxis.route/prepared-reader-port"), BoundaryCurrentReader: readerFor(assemblycontract.ControlledOperationBoundaryReaderRoleV2, "praxis.route/boundary-reader-port"), ProviderInspectReader: readerFor(assemblycontract.ControlledOperationProviderInspectRoleV2, "praxis.route/inspect-reader-port"),
		ActiveBindingPolicy: assemblycontract.ControlledOperationProviderActiveBindingPolicyV2, BypassPolicy: assemblycontract.ControlledOperationProviderBypassPolicyV2,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(declaration)
	if err != nil {
		t.Fatal(err)
	}
	extensionSchema := runtimeports.SchemaRefV2{Namespace: assemblycontract.ControlledOperationProviderRouteSchemaNamespaceV2, Name: assemblycontract.ControlledOperationProviderRouteSchemaNameV2, Version: assemblycontract.ControlledOperationProviderRouteSchemaVersionV2, MediaType: assemblycontract.ControlledOperationProviderRouteSchemaMediaTypeV2, ContentDigest: compilerRouteDigestV2("route-extension-schema")}
	publisher := input.ComponentManifests[0]
	publisher.SemanticVersion = "2.0.0"
	publisher.Locality = runtimeports.LocalityHostControlPlane
	publisher.Schemas = append(publisher.Schemas, extensionSchema)
	sort.Slice(publisher.Schemas, func(i, j int) bool { return publisher.Schemas[i].Key() < publisher.Schemas[j].Key() })
	publisher.Extensions = append(publisher.Extensions, runtimeports.GovernanceExtensionV2{Key: assemblycontract.ControlledOperationProviderRouteExtensionKeyV2, Required: true, Payload: runtimeports.OpaquePayloadV2{Schema: extensionSchema, ContentDigest: core.DigestBytes(payload), Length: uint64(len(payload)), Inline: payload, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.route/limit", Digest: compilerRouteDigestV2("route-limit")}}})
	publisherDigest, err := publisher.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	input.ComponentManifests[0] = publisher
	input.Modules[0].ComponentManifestRef.Digest = publisherDigest
	input.Modules[0].SemanticVersion = "2.0.0"
	input.Modules[0].Locality = runtimeports.LocalityHostControlPlane
	input.ComponentManifests = append(input.ComponentManifests, componentManifest)
	input.Modules = append(input.Modules, module)
	input.Capabilities = append(input.Capabilities, descriptors...)
	input.PortSpecs = append(input.PortSpecs, ports...)
	input.ProviderBindingCandidates = append(input.ProviderBindingCandidates, toolCandidate, gatewayCandidate, transportCandidate, providerCandidate)
	sealed, err := assemblycontract.SealAssemblyInputV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func TestCompileControlledOperationProviderRouteFromRequiredManifestExtensionV2(t *testing.T) {
	t.Parallel()
	input := routeCompileInputV2(t)
	before := input.Digest
	result, err := CompileControlledOperationProviderRouteV2(input, routeGovernanceCatalogV2(t, input))
	if err != nil {
		t.Fatal(err)
	}
	if result.Declaration.RouteID != "praxis.route/compiled-single-tool" || result.Extension.Key != assemblycontract.ControlledOperationProviderRouteExtensionKeyV2 {
		t.Fatalf("unexpected compile result: %+v", result)
	}
	if input.Digest != before {
		t.Fatal("route compiler mutated AssemblyInputV1")
	}
	optional := input
	for manifestIndex := range optional.ComponentManifests {
		for extensionIndex := range optional.ComponentManifests[manifestIndex].Extensions {
			if optional.ComponentManifests[manifestIndex].Extensions[extensionIndex].Key == assemblycontract.ControlledOperationProviderRouteExtensionKeyV2 {
				optional.ComponentManifests[manifestIndex].Extensions[extensionIndex].Required = false
			}
		}
	}
	for manifestIndex := range optional.ComponentManifests {
		manifestDigest, digestErr := optional.ComponentManifests[manifestIndex].BindingDigestV2()
		if digestErr != nil {
			t.Fatal(digestErr)
		}
		for moduleIndex := range optional.Modules {
			if optional.Modules[moduleIndex].ComponentManifestRef.ID == string(optional.ComponentManifests[manifestIndex].ComponentID) {
				optional.Modules[moduleIndex].ComponentManifestRef.Digest = manifestDigest
			}
		}
	}
	optional, err = assemblycontract.SealAssemblyInputV1(optional)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompileControlledOperationProviderRouteV2(optional, routeGovernanceCatalogV2(t, optional)); err == nil {
		t.Fatal("optional route extension was accepted")
	}
}
