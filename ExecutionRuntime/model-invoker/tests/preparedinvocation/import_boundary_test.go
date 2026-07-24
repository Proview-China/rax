package preparedinvocation_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreparedInvocationProductionImportAndNeutralNominalBoundary(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	files, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.AllErrors)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			value := strings.Trim(imported.Path.Value, `"`)
			for _, forbidden := range []string{"/harness", "/tool", "/application", "/runtime/internal", "/internal/", "/vendor/"} {
				if strings.Contains(value, forbidden) {
					t.Fatalf("%s imports forbidden boundary %s", path, value)
				}
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			declaration, ok := node.(*ast.TypeSpec)
			if ok && strings.Contains(declaration.Name.Name, "RegistrySnapshotRef") {
				t.Fatalf("%s defines forbidden Registry nominal %s", path, declaration.Name.Name)
			}
			if ok && declaration.Assign.IsValid() {
				if selector, isSelector := declaration.Type.(*ast.SelectorExpr); isSelector && selector.Sel.Name == "RegistrySnapshotRefV1" {
					t.Fatalf("%s defines forbidden Registry alias %s", path, declaration.Name.Name)
				}
			}
			return true
		})
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(contents), "type RegistrySnapshotRefV1 =") {
			t.Fatalf("%s defines a forbidden Runtime alias", path)
		}
	}
}
