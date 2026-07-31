package preparedproviderinjectionv1_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

func TestPreparedProviderInjectionShapeV1PublicContractIsClosed(t *testing.T) {
	assertFieldsV1(t, reflect.TypeOf(modelinvoker.PreparedProviderInjectionShapeV1{}), []string{
		"ContractVersion", "Provider", "Protocol", "OrderedTools", "ToolChoice", "ParallelToolCalls", "ProviderOptions",
	})
	assertFieldsV1(t, reflect.TypeOf(modelinvoker.PreparedProviderToolV1{}), []string{
		"Name", "Description", "Parameters", "Strict",
	})
	assertFieldsV1(t, reflect.TypeOf(modelinvoker.PreparedProviderToolChoiceV1{}), []string{
		"Mode", "Name",
	})
	assertFieldsV1(t, reflect.TypeOf(modelinvoker.PreparedProviderOptionsV1{}), []string{
		"Provider", "Present", "CanonicalJSON",
	})
}

func TestPreparedProviderInjectionShapeV1ProductionImportBoundary(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate import boundary test")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(current), "..", "..", "prepared_provider_injection_shape_v1.go"))
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range parsed.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"/model-invoker/provider",
			"/model-invoker/routegateway",
			"/model-invoker/internal",
			"/harness",
			"/context-engine",
			"/tool-mcp",
			"/application",
			"/runtime/runtime",
			"/runtimeimplementation",
		} {
			if strings.Contains(importPath, forbidden) {
				t.Fatalf("A1 imports forbidden production boundary %q", importPath)
			}
		}
	}
}

func assertFieldsV1(t *testing.T, typ reflect.Type, want []string) {
	t.Helper()
	if typ.NumField() != len(want) {
		t.Fatalf("%s has %d fields, want %d", typ, typ.NumField(), len(want))
	}
	for index, name := range want {
		if typ.Field(index).Name != name {
			t.Fatalf("%s field %d = %q, want %q", typ, index, typ.Field(index).Name, name)
		}
	}
}
