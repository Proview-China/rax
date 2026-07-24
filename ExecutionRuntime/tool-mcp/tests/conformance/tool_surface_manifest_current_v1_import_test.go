package conformance_test

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

func assertC2ImportBoundary(t *testing.T) {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate C2 conformance test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "..", ".."))
	files := []string{
		filepath.Join(root, "contract", "surface_manifest_current_v1.go"),
		filepath.Join(root, "surface", "manifest_current_repository_v1.go"),
	}
	forbidden := []string{
		"ExecutionRuntime/application",
		"ExecutionRuntime/model-invoker",
		"ExecutionRuntime/harness",
		"ExecutionRuntime/runtime/kernel",
		"ExecutionRuntime/runtime/fakes",
		"ExecutionRuntime/runtime/internal",
		"net",
		"net/http",
		"database/sql",
		"os/exec",
		"unsafe",
	}
	for _, filename := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", filename, err)
		}
		for _, spec := range parsed.Decls {
			declaration, ok := spec.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, item := range declaration.Specs {
				importSpec, ok := item.(*ast.ImportSpec)
				if !ok {
					continue
				}
				path, err := strconv.Unquote(importSpec.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				for _, blocked := range forbidden {
					if path == blocked || strings.Contains(path, "/"+blocked) || strings.HasPrefix(path, blocked+"/") {
						t.Fatalf("C2 production file %s imports forbidden package %s", filename, path)
					}
				}
			}
		}
	}
}
