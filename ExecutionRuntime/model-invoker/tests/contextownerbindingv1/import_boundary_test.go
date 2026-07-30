package contextownerbindingv1_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestContextOwnerBindingImportBoundaryV1(t *testing.T) {
	moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(moduleRoot, "context_owner_binding_v1.go")
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
		if strings.HasPrefix(importPath, "github.com/Proview-China/rax/") &&
			importPath != "github.com/Proview-China/rax/ExecutionRuntime/runtime/core" {
			t.Fatalf("forbidden Praxis implementation import %q", importPath)
		}
	}
}
