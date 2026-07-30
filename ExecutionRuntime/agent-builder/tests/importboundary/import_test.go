package importboundary_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestProductionPackagesDoNotImportHostRuntimeLoopOrConsole(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "../.."))
	forbidden := []string{
		"/ExecutionRuntime/agent-host", "/ExecutionRuntime/application", "/ExecutionRuntime/harness/kernel",
		"/ExecutionRuntime/harness/runtimeadapter", "/ExecutionRuntime/model-invoker", "/ExecutionRuntime/sandbox",
	}
	allowedNewPackageImports := map[string]map[string]struct{}{
		"loader": {
			"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract":   {},
			"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/ports":      {},
			"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract": {},
			"github.com/Proview-China/rax/ExecutionRuntime/runtime/core":             {},
		},
		"ports": {
			"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract":   {},
			"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract": {},
		},
		"repository": {
			"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/contract": {},
			"github.com/Proview-China/rax/ExecutionRuntime/agent-builder/ports":    {},
			"github.com/Proview-China/rax/ExecutionRuntime/runtime/core":           {},
		},
	}
	for _, directory := range []string{"builder", "compiler", "contract", "loader", "ports", "repository"} {
		files, err := filepath.Glob(filepath.Join(root, directory, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, spec := range file.Imports {
				value, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				for _, prefix := range forbidden {
					if strings.Contains(value, prefix) {
						t.Fatalf("%s imports forbidden boundary %s", path, value)
					}
				}
				if allowed, guarded := allowedNewPackageImports[directory]; guarded && strings.HasPrefix(value, "github.com/Proview-China/rax/") {
					if _, ok := allowed[value]; !ok {
						t.Fatalf("%s imports undeclared Praxis boundary %s", path, value)
					}
				}
			}
		}
	}
}
