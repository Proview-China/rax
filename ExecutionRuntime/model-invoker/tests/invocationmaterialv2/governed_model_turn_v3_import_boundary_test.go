package invocationmaterialv2_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestGovernedModelTurnV3OwnerLocalImportBoundary(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	files := []string{
		filepath.Join(root, "governed_model_turn_v3.go"),
		filepath.Join(root, "storage", "sqlite", "governed_model_turn_v3.go"),
		filepath.Join(root, "storage", "sqlite", "schema_verify_v4.go"),
	}
	for _, path := range files {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Decls {
			declaration, ok := spec.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, item := range declaration.Specs {
				importSpec, ok := item.(*ast.ImportSpec)
				if !ok {
					continue
				}
				importPath, err := strconv.Unquote(importSpec.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				for _, forbidden := range []string{
					"/provider/",
					"/routegateway",
					"/execution/harness",
					"/context-engine/",
					"/tool-mcp/",
				} {
					if strings.Contains(importPath, forbidden) {
						t.Fatalf("%s imports forbidden boundary %q", path, importPath)
					}
				}
			}
		}
	}
}
