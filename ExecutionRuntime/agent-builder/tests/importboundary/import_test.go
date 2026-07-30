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
	for _, directory := range []string{"builder", "compiler", "contract"} {
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
			}
		}
	}
}
