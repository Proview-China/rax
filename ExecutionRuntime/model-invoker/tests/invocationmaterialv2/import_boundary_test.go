package invocationmaterialv2_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestInvocationMaterialV2ImportBoundary(t *testing.T) {
	moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	targets := []string{
		"invocation_material_lineage_v2.go",
		"invocation_material_v2.go",
		filepath.Join("storage", "sqlite", "invocation_material_v2.go"),
		filepath.Join("storage", "sqlite", "schema_verify_v3.go"),
	}
	allowedPraxisImports := map[string]struct{}{
		"github.com/Proview-China/rax/ExecutionRuntime/model-invoker":          {},
		"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream": {},
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/core":           {},
	}
	for _, target := range targets {
		t.Run(filepath.ToSlash(target), func(t *testing.T) {
			path := filepath.Join(moduleRoot, target)
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, source, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, spec := range parsed.Imports {
				importPath, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.HasPrefix(importPath, "github.com/Proview-China/rax/") {
					continue
				}
				if _, ok := allowedPraxisImports[importPath]; !ok {
					t.Fatalf("forbidden Praxis implementation import %q", importPath)
				}
			}
		})
	}
}
