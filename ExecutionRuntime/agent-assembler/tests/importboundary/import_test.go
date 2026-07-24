package importboundary_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestAssemblerProductionPackagesKeepOwnerImportBoundary(t *testing.T) {
	root := "../.."
	forbidden := []string{"/runtime/kernel", "/runtime/foundation", "/runtime/fakes", "/runtime/internal", "/harness/kernel", "/harness/fakes", "/harness/internal", "/context-engine", "/memory-knowledge", "/tool-mcp", "/sandbox", "/continuity", "/review"}
	packages := []string{"contract", "ports", "repository", "mapper", "resolver", "conformance"}
	for _, name := range packages {
		entries, err := os.ReadDir(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(root, name, entry.Name())
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range file.Imports {
				value, err := strconv.Unquote(item.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				for _, prefix := range forbidden {
					if strings.Contains(value, prefix) {
						t.Fatalf("%s imports forbidden owner implementation %s", path, value)
					}
				}
			}
			ast.Inspect(file, func(ast.Node) bool { return true })
		}
	}
}
