package runtimeadapter_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPendingActionApplicabilityAdapterImportBoundary(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate runtimeadapter tests")
	}
	adapterFile := filepath.Join(filepath.Dir(current), "..", "..", "runtimeadapter", "pending_action_applicability_current_v3.go")
	file, err := parser.ParseFile(token.NewFileSet(), adapterFile, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"/ExecutionRuntime/application",
		"/ExecutionRuntime/tool-mcp",
		"/ExecutionRuntime/context-engine",
		"/ExecutionRuntime/model-invoker",
		"/ExecutionRuntime/runtime/foundation",
		"/ExecutionRuntime/runtime/fakes",
		"/ExecutionRuntime/harness/kernel",
		"/ExecutionRuntime/harness/internal",
	}
	for _, imported := range file.Imports {
		path := strings.Trim(imported.Path.Value, "\"")
		for _, prefix := range forbidden {
			if strings.Contains(path, prefix) {
				t.Fatalf("applicability Adapter imports forbidden implementation package %s", path)
			}
		}
	}
	if _, err := os.Stat(adapterFile); err != nil {
		t.Fatal(err)
	}
}
