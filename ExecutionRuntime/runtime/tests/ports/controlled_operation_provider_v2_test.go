package ports_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestControlledOperationProviderRouteV2CanonicalAndExactWatermark(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	projection := controlledProviderRouteProjectionV2(t, now)
	if err := projection.ValidateCurrent(projection.Ref, ports.OperationScopeEvidenceActionMatrixV3(), now); err != nil {
		t.Fatal(err)
	}
	expectedMatrix, err := core.CanonicalJSONDigest("praxis.runtime.operation-scope-evidence", ports.OperationScopeEvidenceContractVersionV3, "OperationScopeEvidenceApplicabilityMatrixKeyV3", ports.OperationScopeEvidenceActionMatrixV3())
	if err != nil || projection.Ref.MatrixDigest != expectedMatrix {
		t.Fatalf("route used a parallel matrix canonical: got=%s want=%s err=%v", projection.Ref.MatrixDigest, expectedMatrix, err)
	}
	expectedRefDigest, err := projection.Ref.DigestV2()
	if err != nil || projection.Ref.Digest != expectedRefDigest {
		t.Fatalf("route current ref digest mismatch: %v", err)
	}

	for name, mutate := range map[string]func(*ports.ControlledOperationProviderRouteCurrentProjectionV2){
		"matrix": func(p *ports.ControlledOperationProviderRouteCurrentProjectionV2) {
			p.Ref.MatrixDigest = core.DigestBytes([]byte("other-matrix"))
		},
		"watermark": func(p *ports.ControlledOperationProviderRouteCurrentProjectionV2) {
			p.Ref.Watermark = core.DigestBytes([]byte("other-watermark"))
		},
		"binding_set": func(p *ports.ControlledOperationProviderRouteCurrentProjectionV2) {
			p.ToolAdapterBinding.BindingSetRevision++
		},
		"provider_capability": func(p *ports.ControlledOperationProviderRouteCurrentProjectionV2) {
			p.ProviderBinding.Capability = "praxis.tool/other"
		},
		"role_reuse": func(p *ports.ControlledOperationProviderRouteCurrentProjectionV2) {
			p.GatewayBinding = p.ToolAdapterBinding
		},
		"checked_equals_expiry": func(p *ports.ControlledOperationProviderRouteCurrentProjectionV2) {
			p.CheckedUnixNano = p.ExpiresUnixNano
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := projection
			mutate(&changed)
			changed.ProjectionDigest = ""
			digest, digestErr := changed.DigestV2()
			if digestErr != nil {
				t.Fatal(digestErr)
			}
			changed.ProjectionDigest = digest
			if err := changed.Validate(); err == nil {
				t.Fatal("re-sealed route drift was accepted")
			}
		})
	}
}

func TestControlledOperationProviderRouteV2FrozenPublicShapeAndReaderSignature(t *testing.T) {
	type expectedReader interface {
		InspectCurrentControlledOperationProviderRouteV2(context.Context, ports.ControlledOperationProviderRouteCurrentRefV2, ports.OperationScopeEvidenceApplicabilityMatrixKeyV3) (ports.ControlledOperationProviderRouteCurrentProjectionV2, error)
	}
	var _ expectedReader = (ports.ControlledOperationProviderRouteCurrentReaderV2)(nil)

	for value, expected := range map[any][]string{
		ports.ControlledOperationProviderRouteDeclarationRefV2{}: {"route_id", "revision", "publisher_component_id", "declaration_digest"},
		ports.ControlledOperationProviderRouteConformanceRefV2{}: {"conformance_id", "revision", "declaration_ref", "conformance_digest"},
		ports.ControlledOperationProviderRouteCurrentRefV2{}:     {"current_id", "revision", "declaration_ref", "conformance_ref", "matrix_digest", "watermark", "digest"},
	} {
		typeOf := reflect.TypeOf(value)
		if typeOf.NumField() != len(expected) {
			t.Fatalf("%s field count drifted", typeOf.Name())
		}
		for index, tag := range expected {
			if got := typeOf.Field(index).Tag.Get("json"); got != tag {
				t.Fatalf("%s field %d tag=%q want=%q", typeOf.Name(), index, got, tag)
			}
		}
	}
	projectionTags := []string{
		"contract_version", "ref", "declaration_ref", "conformance_ref", "generation",
		"handoff_id", "handoff_revision", "handoff_digest", "binding_set_id", "binding_set_revision",
		"binding_set_digest", "binding_set_semantic_digest", "binding_set_currentness_digest",
		"active_route_id", "active_route_revision", "active_route_digest", "tool_adapter_binding",
		"gateway_binding", "provider_transport_binding", "prepared_reader_binding", "boundary_reader_binding",
		"provider_inspect_binding", "provider_binding", "checked_unix_nano", "expires_unix_nano", "projection_digest",
	}
	projectionType := reflect.TypeOf(ports.ControlledOperationProviderRouteCurrentProjectionV2{})
	if projectionType.NumField() != len(projectionTags) {
		t.Fatalf("route current projection field count=%d want=%d", projectionType.NumField(), len(projectionTags))
	}
	for index, tag := range projectionTags {
		if got := projectionType.Field(index).Tag.Get("json"); got != tag {
			t.Fatalf("route current projection field %d tag=%q want=%q", index, got, tag)
		}
	}
}

func TestControlledOperationProviderRouteV2PublicTypeSetIsFrozen(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate controlled Provider ports test")
	}
	source := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", "..", "ports", "controlled_operation_provider_v2.go"))
	parsed, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var routeTypes []string
	readerMethods := 0
	for _, declaration := range parsed.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, spec := range general.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			if len(typeSpec.Name.Name) < len("ControlledOperationProviderRoute") || typeSpec.Name.Name[:len("ControlledOperationProviderRoute")] != "ControlledOperationProviderRoute" {
				continue
			}
			routeTypes = append(routeTypes, typeSpec.Name.Name)
			if typeSpec.Name.Name == "ControlledOperationProviderRouteCurrentReaderV2" {
				reader, ok := typeSpec.Type.(*ast.InterfaceType)
				if !ok {
					t.Fatal("Route Current Reader is not an interface")
				}
				readerMethods = len(reader.Methods.List)
				if readerMethods != 1 || len(reader.Methods.List[0].Names) != 1 || reader.Methods.List[0].Names[0].Name != "InspectCurrentControlledOperationProviderRouteV2" {
					t.Fatal("Route Current Reader signature set drifted")
				}
			}
		}
	}
	sort.Strings(routeTypes)
	expected := []string{
		"ControlledOperationProviderRouteConformanceRefV2",
		"ControlledOperationProviderRouteCurrentProjectionV2",
		"ControlledOperationProviderRouteCurrentReaderV2",
		"ControlledOperationProviderRouteCurrentRefV2",
		"ControlledOperationProviderRouteDeclarationRefV2",
	}
	if !reflect.DeepEqual(routeTypes, expected) || readerMethods != 1 {
		t.Fatalf("public Route type set drifted: got=%v want=%v reader_methods=%d", routeTypes, expected, readerMethods)
	}
}

func TestControlledOperationProviderRouteV2SealRejectsWrongNonzeroDerivedFields(t *testing.T) {
	base := controlledProviderRouteProjectionV2(t, time.Unix(1_900_000_000, 0))
	for name, mutate := range map[string]func(*ports.ControlledOperationProviderRouteCurrentProjectionV2){
		"contract": func(p *ports.ControlledOperationProviderRouteCurrentProjectionV2) { p.ContractVersion = "9.0.0" },
		"matrix": func(p *ports.ControlledOperationProviderRouteCurrentProjectionV2) {
			p.Ref.MatrixDigest = core.DigestBytes([]byte("wrong-matrix"))
		},
		"current_id": func(p *ports.ControlledOperationProviderRouteCurrentProjectionV2) {
			p.Ref.CurrentID = "wrong-current-id"
		},
		"declaration": func(p *ports.ControlledOperationProviderRouteCurrentProjectionV2) { p.Ref.DeclarationRef.Revision++ },
		"conformance": func(p *ports.ControlledOperationProviderRouteCurrentProjectionV2) { p.Ref.ConformanceRef.Revision++ },
		"watermark": func(p *ports.ControlledOperationProviderRouteCurrentProjectionV2) {
			p.Ref.Watermark = core.DigestBytes([]byte("wrong-watermark"))
		},
		"ref_digest": func(p *ports.ControlledOperationProviderRouteCurrentProjectionV2) {
			p.Ref.Digest = core.DigestBytes([]byte("wrong-ref-digest"))
		},
		"projection_digest": func(p *ports.ControlledOperationProviderRouteCurrentProjectionV2) {
			p.ProjectionDigest = core.DigestBytes([]byte("wrong-projection-digest"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := base
			mutate(&changed)
			if sealed, err := ports.SealControlledOperationProviderRouteCurrentProjectionV2(changed); err == nil || sealed != (ports.ControlledOperationProviderRouteCurrentProjectionV2{}) {
				t.Fatalf("Seal washed a wrong nonzero derived field: err=%v sealed=%#v", err, sealed)
			}
		})
	}
}

func TestControlledOperationProviderRouteV2InvalidBindingShapeKeepsInvalidCategory(t *testing.T) {
	projection := controlledProviderRouteProjectionV2(t, time.Unix(1_900_000_000, 0))
	projection.PreparedReaderBinding.ComponentID = "not namespaced"
	projection.ProjectionDigest = ""
	digest, err := projection.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	projection.ProjectionDigest = digest
	if err := projection.Validate(); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("invalid Binding shape was misclassified: %v", err)
	}
}

func controlledProviderRouteProjectionV2(t *testing.T, now time.Time) ports.ControlledOperationProviderRouteCurrentProjectionV2 {
	t.Helper()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	declaration := ports.ControlledOperationProviderRouteDeclarationRefV2{RouteID: "route-cop2", Revision: 1, PublisherComponentID: "praxis.harness/assembly", DeclarationDigest: digest("declaration")}
	conformance := ports.ControlledOperationProviderRouteConformanceRefV2{ConformanceID: "conformance-cop2", Revision: 1, DeclarationRef: declaration, ConformanceDigest: digest("conformance")}
	binding := func(component string, capability ports.CapabilityNameV2) ports.ProviderBindingRefV2 {
		return ports.ProviderBindingRefV2{BindingSetID: "binding-set-cop2", BindingSetRevision: 4, ComponentID: ports.ComponentIDV2(component), ManifestDigest: digest(component + "-manifest"), ArtifactDigest: digest(component + "-artifact"), Capability: capability}
	}
	projection := ports.ControlledOperationProviderRouteCurrentProjectionV2{
		Ref: ports.ControlledOperationProviderRouteCurrentRefV2{Revision: 3}, DeclarationRef: declaration, ConformanceRef: conformance,
		Generation: ports.GenerationArtifactRefV1{ID: "generation-cop2", Revision: 2, Digest: digest("generation"), InputDigest: digest("input"), ManifestDigest: digest("manifest"), GraphDigest: digest("graph"), CatalogDigest: digest("catalog")},
		HandoffID:  "handoff-cop2", HandoffRevision: 2, HandoffDigest: digest("handoff"),
		BindingSetID: "binding-set-cop2", BindingSetRevision: 4, BindingSetDigest: digest("binding-set"), BindingSetSemanticDigest: digest("binding-semantics"), BindingSetCurrentnessDigest: digest("binding-current"),
		ActiveRouteID: "active-route-cop2", ActiveRouteRevision: 6, ActiveRouteDigest: digest("active-route"),
		ToolAdapterBinding:       binding("praxis.tool/adapter", ports.ControlledOperationToolAdapterCapabilityV2),
		GatewayBinding:           binding("praxis.runtime/gateway", ports.ControlledOperationGatewayCapabilityV2),
		ProviderTransportBinding: binding("praxis.tool/transport", ports.ControlledOperationProviderTransportCapabilityV2),
		PreparedReaderBinding:    binding("praxis.runtime/prepared-reader", ports.ControlledOperationPreparedReaderCapabilityV2),
		BoundaryReaderBinding:    binding("praxis.runtime/boundary-reader", ports.ControlledOperationBoundaryReaderCapabilityV2),
		ProviderInspectBinding:   binding("praxis.runtime/provider-inspect", ports.ControlledOperationProviderInspectCapabilityV2),
		ProviderBinding:          binding("praxis.tool/provider", ports.CapabilityNameV2(ports.OperationScopeEvidenceActionEffectKindV3)),
		CheckedUnixNano:          now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano(),
	}
	sealed, err := ports.SealControlledOperationProviderRouteCurrentProjectionV2(projection)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}
