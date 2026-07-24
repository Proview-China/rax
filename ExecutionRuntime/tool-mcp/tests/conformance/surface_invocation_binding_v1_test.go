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

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/owner/surfacebinding"
)

func TestToolSurfaceInvocationBindingV1Conformance(t *testing.T) {
	t.Run("public method set", func(t *testing.T) {
		var repository toolcontract.ToolSurfaceInvocationBindingRepositoryV1 = (*surfacebinding.InMemoryRepositoryV1)(nil)
		_ = repository
	})
	t.Run("production imports are owner bounded and zero network", func(t *testing.T) {
		_, here, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("cannot locate conformance test")
		}
		root := filepath.Clean(filepath.Join(filepath.Dir(here), "..", ".."))
		files := []string{
			filepath.Join(root, "contract", "surface_invocation_binding_v1.go"),
			filepath.Join(root, "internal", "owner", "surfacebinding", "repository_v1.go"),
		}
		forbidden := []string{
			"ExecutionRuntime/application", "ExecutionRuntime/harness", "ExecutionRuntime/model-invoker/internal",
			"ExecutionRuntime/runtime/kernel", "ExecutionRuntime/runtime/fakes", "ExecutionRuntime/runtime/internal",
			"net", "net/http", "database/sql", "os/exec", "unsafe",
		}
		for _, filename := range files {
			parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", filename, err)
			}
			for _, declaration := range parsed.Decls {
				general, ok := declaration.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, item := range general.Specs {
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
							t.Fatalf("Surface Binding production file %s imports forbidden package %s", filename, path)
						}
					}
				}
			}
		}
	})
}
