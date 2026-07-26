package corepack

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

func TestCorePackProductionHasNoEffectImplementationImports(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{
		"os": true, "os/exec": true, "io/fs": true, "path/filepath": true,
		"net": true, "net/http": true,
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, source, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			name, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if forbidden[name] || strings.Contains(name, "/sandbox/internal") {
				t.Fatalf("%s directly imports forbidden effect package %q", file, name)
			}
		}
		ast.Inspect(parsed, func(ast.Node) bool { return true })
	}
}

func TestCorePackDoesNotExposeSecondShellSemantic(t *testing.T) {
	catalog := testCatalogV1(t)
	for _, definition := range catalog.Definitions {
		if definition.ModelName == "shell.run" {
			t.Fatal("shell.run must not be a second descriptor")
		}
	}
}
