package corepack

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"
)

func TestCorePackAssemblyProductionImportsStayEffectFreeV1(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{"os": true, "os/exec": true, "io/fs": true, "net": true, "net/http": true,
		"github.com/Proview-China/rax/ExecutionRuntime/sandbox": true,
		"github.com/Proview-China/rax/ExecutionRuntime/harness": true}
	for _, name := range files {
		if filepath.Ext(name) != ".go" || len(name) >= 8 && name[len(name)-8:] == "_test.go" {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		for _, imported := range file.Imports {
			path, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil {
				t.Fatal(unquoteErr)
			}
			if forbidden[path] {
				t.Fatalf("production file %s imports forbidden package %s", name, path)
			}
		}
	}
	_ = ast.File{}
}
