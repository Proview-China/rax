package blackbox_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/builder"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/compiler"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycompiler"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func resolved(t *testing.T) assemblercontract.ResolveResultV1 {
	t.Helper()
	result, err := testkit.NewFixture().Resolve()
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func validationCatalog(f testkit.Fixture) definitioncontract.ValidationCatalogV1 {
	result := definitioncontract.ValidationCatalogV1{}
	for _, registration := range f.Catalog.Governance.Registrations {
		result.Kinds = append(result.Kinds, string(registration.Kind))
		for _, capability := range registration.Capabilities {
			result.Capabilities = append(result.Capabilities, string(capability))
		}
		for _, extension := range registration.ExtensionPolicies {
			result.RegisteredExtensionKeys = append(result.RegisteredExtensionKeys, string(extension.Key))
		}
	}
	sort.Strings(result.Kinds)
	sort.Strings(result.Capabilities)
	sort.Strings(result.RegisteredExtensionKeys)
	return result
}

func TestDeclarativeAndCodeBuildersProduceExistingDefinitionSource(t *testing.T) {
	fixture := testkit.NewFixture()
	source := fixture.Definition.AgentDefinitionSourceV1
	payload, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	catalog := validationCatalog(fixture)
	fromJSON, err := builder.DecodeDefinitionSourceV1(builder.DefinitionFormatJSONV1, payload, catalog)
	if err != nil {
		t.Fatal(err)
	}
	fromYAML, err := builder.DecodeDefinitionSourceV1(builder.DefinitionFormatYAMLV1, payload, catalog)
	if err != nil {
		t.Fatal(err)
	}
	fromCode, err := builder.NewDefinitionSourceBuilderV1(source).Build(catalog)
	if err != nil {
		t.Fatal(err)
	}
	want, err := definitioncontract.SourceDigestV1(source, catalog)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]definitioncontract.AgentDefinitionSourceV1{"json": fromJSON, "yaml": fromYAML, "code": fromCode} {
		got, digestErr := definitioncontract.SourceDigestV1(value, catalog)
		if digestErr != nil || got != want {
			t.Fatalf("%s builder diverged from existing definition source: %v %s %s", name, digestErr, got, want)
		}
	}
	fromCode.Components[0].ComponentID = "mutated"
	again, err := builder.NewDefinitionSourceBuilderV1(source).Build(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if again.Components[0].ComponentID == "mutated" {
		t.Fatal("code builder returned aliased source")
	}
}

func TestCodeBuilderAddMethodsOwnTheirInputs(t *testing.T) {
	fixture := testkit.NewFixture()
	source, catalog := fixture.Definition.AgentDefinitionSourceV1, validationCatalog(fixture)

	componentBase := definitioncontract.CloneSourceV1(source)
	last := len(componentBase.Components) - 1
	component := componentBase.Components[last]
	componentBase.Components = append([]definitioncontract.ComponentRequirementV1{}, componentBase.Components[:last]...)
	componentBuilder := builder.NewDefinitionSourceBuilderV1(componentBase).AddComponent(component)
	component.RequiredCapabilities[0] = "example.invalid/mutated"
	builtComponent, err := componentBuilder.Build(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if builtComponent.Components[len(builtComponent.Components)-1].RequiredCapabilities[0] == component.RequiredCapabilities[0] {
		t.Fatal("AddComponent retained caller slice alias")
	}

	secret := definitioncontract.SecretRefV1{SecretID: "secrets/example", Class: "secret/token", RequestedScopeDigest: core.DigestBytes([]byte("scope"))}
	secretBuilder := builder.NewDefinitionSourceBuilderV1(source).AddSecretRef(secret)
	secret.SecretID = "secrets/mutated"
	builtSecret, err := secretBuilder.Build(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if builtSecret.SecretRefs[0].SecretID != "secrets/example" {
		t.Fatal("AddSecretRef retained caller state")
	}

	payload := json.RawMessage(`{"mode":"safe"}`)
	extension := definitioncontract.ExtensionV1{Key: "example/optional", Schema: definitioncontract.SchemaRefV1{Namespace: "example", Name: "extension", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("schema"))}, ContentDigest: core.DigestBytes(payload), Payload: payload}
	extensionBuilder := builder.NewDefinitionSourceBuilderV1(source).AddExtension(extension)
	extension.Payload[9] = 'x'
	builtExtension, err := extensionBuilder.Build(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if string(builtExtension.Extensions[0].Payload) != `{"mode":"safe"}` {
		t.Fatal("AddExtension retained caller payload alias")
	}
}

func TestResolvedDefinitionCompilesToDeterministicPackage(t *testing.T) {
	result := resolved(t)
	first, err := compiler.NewV1().Compile(result)
	if err != nil {
		t.Fatal(err)
	}
	second, err := compiler.NewV1().Compile(result)
	if err != nil {
		t.Fatal(err)
	}
	publicationID, err := assemblycontract.DeriveAssemblyPublicationIDV2(first.Lock.AssemblyInputDigest, first.Lock.GenerationRef.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.RefV1().Validate() != nil || first.Lock.ManifestRef.ID != publicationID+"/manifest" || first.Lock.GraphRef.ID != publicationID+"/graph" || first.Lock.HandoffRef.ID != publicationID+"/handoff" {
		t.Fatalf("package closure is not deterministic and exact: %#v", first)
	}

	reordered := assemblercontract.CloneResolveResultV1(result)
	for left, right := 0, len(reordered.Plan.ComponentReleases)-1; left < right; left, right = left+1, right-1 {
		reordered.Plan.ComponentReleases[left], reordered.Plan.ComponentReleases[right] = reordered.Plan.ComponentReleases[right], reordered.Plan.ComponentReleases[left]
	}
	third, err := compiler.NewV1().Compile(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, third) {
		t.Fatal("semantic release reorder changed package")
	}
}

func TestPackageLocksHarnessPublicationArtifactCoordinates(t *testing.T) {
	result := resolved(t)
	pkg, err := compiler.NewV1().Compile(result)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := assemblycompiler.New().Compile(result.AssemblyInput)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := assemblycontract.NewAssemblyPublicationBundleV2("agent-package-blackbox", compiled)
	if err != nil {
		t.Fatal(err)
	}
	locked := assemblycontract.AssemblyPublicationArtifactRefsV2{
		Generation: pkg.Lock.GenerationRef,
		Manifest:   pkg.Lock.ManifestRef,
		Graph:      pkg.Lock.GraphRef,
		Handoff:    pkg.Lock.HandoffRef,
	}
	if locked != publication.Publication.Artifacts {
		t.Fatalf("AgentPackage and Harness Publication exact refs drifted: package=%+v publication=%+v", locked, publication.Publication.Artifacts)
	}
}

func TestCompilerRejectsOmitAddAndClosureSplice(t *testing.T) {
	base := resolved(t)
	tests := map[string]func(*assemblercontract.ResolveResultV1){
		"omit release": func(v *assemblercontract.ResolveResultV1) { v.Plan.ComponentReleases = v.Plan.ComponentReleases[1:] },
		"add release": func(v *assemblercontract.ResolveResultV1) {
			v.Plan.ComponentReleases = append(v.Plan.ComponentReleases, v.Plan.ComponentReleases[0])
		},
		"binding splice": func(v *assemblercontract.ResolveResultV1) { v.BindingPlan.ID += "-other" },
		"facts splice": func(v *assemblercontract.ResolveResultV1) {
			v.Plan.ResolutionFactsRef.Digest = core.DigestBytes([]byte("other-facts"))
		},
		"catalog splice": func(v *assemblercontract.ResolveResultV1) {
			v.Plan.CatalogRef.Digest = core.DigestBytes([]byte("other-catalog"))
		},
		"assembly splice": func(v *assemblercontract.ResolveResultV1) {
			v.AssemblyInput.Plan.ContextPlan = v.AssemblyInput.Plan.Profile
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := assemblercontract.CloneResolveResultV1(base)
			mutate(&value)
			if _, err := compiler.NewV1().Compile(value); err == nil {
				t.Fatal("spliced closure accepted")
			}
		})
	}
}
