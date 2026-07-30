package governedmodelturnv3boundary_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestProviderBoundaryV3ImportBoundary(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate import boundary test")
	}
	modelRoot := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	files := []string{
		filepath.Join(modelRoot, "governed_model_turn_provider_boundary_v3.go"),
		filepath.Join(modelRoot, "runtimeadapter", "model_provider_boundary_current_v1.go"),
		filepath.Join(modelRoot, "storage", "sqlite", "governed_model_turn_provider_boundary_v3.go"),
		filepath.Join(modelRoot, "storage", "sqlite", "schema_verify_v5.go"),
	}
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, spec := range parsed.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("unquote %s import: %v", file, err)
			}
			if forbiddenProviderBoundaryImport(path) {
				t.Fatalf("provider boundary owner-local slice imports forbidden package %q in %s", path, file)
			}
		}
	}
}

func forbiddenProviderBoundaryImport(path string) bool {
	for _, fragment := range []string{
		"/model-invoker/provider",
		"/model-invoker/routegateway",
		"/harness",
		"/context-engine",
		"/tool-mcp",
		"/runtime/runtime",
		"/runtimeimplementation",
	} {
		if strings.Contains(path, fragment) {
			return true
		}
	}
	return false
}
