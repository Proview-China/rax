package assemblyintegration_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestControlledOperationProviderRouteV2ImportAndTypeOwnershipBoundary(t *testing.T) {
	t.Parallel()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate Harness assembly integration tests")
	}
	harnessRoot := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	files := []string{
		filepath.Join(harnessRoot, "assemblycontract", "controlled_operation_provider_route_v2.go"),
		filepath.Join(harnessRoot, "assemblycontract", "controlled_operation_provider_route_conformance_v2.go"),
		filepath.Join(harnessRoot, "assemblycompiler", "controlled_operation_provider_route_v2.go"),
		filepath.Join(harnessRoot, "assemblyadapter", "controlled_operation_provider_route_v2.go"),
	}
	forbiddenImports := []string{
		"/ExecutionRuntime/application",
		"/ExecutionRuntime/tool-mcp",
		"/ExecutionRuntime/context-engine",
		"/ExecutionRuntime/model-invoker",
		"/ExecutionRuntime/runtime/control",
		"/ExecutionRuntime/runtime/kernel",
		"/ExecutionRuntime/runtime/fakes",
		"/ExecutionRuntime/runtime/foundation",
	}
	runtimeOwnedTypes := map[string]struct{}{
		"ControlledOperationProviderRouteDeclarationRefV2":    {},
		"ControlledOperationProviderRouteConformanceRefV2":    {},
		"ControlledOperationProviderRouteCurrentRefV2":        {},
		"ControlledOperationProviderRouteCurrentProjectionV2": {},
		"ControlledOperationProviderRouteCurrentReaderV2":     {},
		"OperationScopeEvidenceApplicabilityMatrixKeyV3":      {},
	}
	for _, path := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			value := strings.Trim(imported.Path.Value, "\"")
			for _, forbidden := range forbiddenImports {
				if strings.Contains(value, forbidden) {
					t.Fatalf("%s imports forbidden Owner/implementation package %s", path, value)
				}
			}
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		full, err := parser.ParseFile(token.NewFileSet(), path, payload, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(full, func(node ast.Node) bool {
			declaration, ok := node.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if _, copied := runtimeOwnedTypes[declaration.Name.Name]; copied {
				t.Errorf("Harness copied Runtime-owned neutral type %s in %s", declaration.Name.Name, path)
			}
			return true
		})
	}
}
