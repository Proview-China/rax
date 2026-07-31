package routegateway_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestGovernedModelTurnV3ActualBoundaryImportClosure(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate V3 actual-boundary import test")
	}
	modelRoot := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	for _, path := range []string{
		filepath.Join(modelRoot, "governed_model_turn_actual_boundary_v3.go"),
		filepath.Join(modelRoot, "routegateway", "governed_v3_actual_boundary.go"),
	} {
		parsed, err := parser.ParseFile(
			token.NewFileSet(),
			path,
			nil,
			parser.ImportsOnly,
		)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{
				"/harness",
				"/application",
				"/context-engine",
				"/tool-mcp",
				"/runtime/kernel",
				"/runtime/control",
				"/runtime/fakes",
			} {
				if strings.Contains(importPath, forbidden) {
					t.Fatalf("%s imports forbidden boundary %q", path, importPath)
				}
			}
		}
	}
}
