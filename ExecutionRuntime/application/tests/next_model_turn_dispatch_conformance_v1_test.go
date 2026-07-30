package application_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/application/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestNextModelTurnDispatchImportConformanceV1(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve next-turn production import scanner root")
	}
	applicationRoot := filepath.Dir(filepath.Dir(sourceFile))
	var productionImports []string
	for _, relative := range []string{
		"contract/next_model_turn_dispatch_v1.go",
		"ports/next_model_turn_dispatch_v1.go",
		"conformance/next_model_turn_dispatch_v1.go",
	} {
		file, err := parser.ParseFile(
			token.NewFileSet(),
			filepath.Join(applicationRoot, relative),
			nil,
			parser.ImportsOnly,
		)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			productionImports = append(productionImports, path)
		}
	}
	if err := conformance.CheckNextModelTurnDispatchImportsV1(productionImports); err != nil {
		t.Fatalf("production import scanner found a forbidden execution surface: %v", err)
	}
	for _, forbidden := range []string{
		"github.com/Proview-China/rax/ExecutionRuntime/model-invoker",
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports",
		"github.com/Proview-China/rax/ExecutionRuntime/harness/bridgecontract",
		"github.com/Proview-China/rax/ExecutionRuntime/harness/modelinvokeradapter",
		"github.com/Proview-China/rax/ExecutionRuntime/provider",
		"github.com/Proview-China/rax/ExecutionRuntime/console",
	} {
		if err := conformance.CheckNextModelTurnDispatchImportsV1([]string{forbidden}); !core.HasCategory(err, core.ErrorForbidden) {
			t.Fatalf("forbidden import %q was accepted: %v", forbidden, err)
		}
	}
}
