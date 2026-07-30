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
	if !reflect.DeepEqual(first, second) || first.RefV1().Validate() != nil || first.Lock.ManifestRef.ID != first.Lock.GenerationRef.ID+"/manifest" || first.Lock.GraphRef.ID != first.Lock.GenerationRef.ID+"/graph" || first.Lock.HandoffRef.ID != first.Lock.GenerationRef.ID+"/handoff" {
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
