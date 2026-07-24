package contract_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIdentityModelImportBoundaryUsesOnlyPublicRootPackage(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate contract tests")
	}
	root := filepath.Join(filepath.Dir(current), "..", "..")
	files := []string{
		filepath.Join(root, "contract", "model_tool_call_pending_action_identity_v1.go"),
		filepath.Join(root, "contract", "settled_turn_domain_result_v3.go"),
		filepath.Join(root, "contract", "governed_v3.go"),
		filepath.Join(root, "contract", "pending_action_reader_v2.go"),
		filepath.Join(root, "ports", "settled_turn_domain_result_v3.go"),
		filepath.Join(root, "ports", "governed_v3.go"),
		filepath.Join(root, "ports", "pending_action_reader_v2.go"),
	}
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			path := strings.Trim(imported.Path.Value, "\"")
			if strings.Contains(path, "/ExecutionRuntime/model-invoker/") || strings.Contains(path, "/ExecutionRuntime/application") || strings.Contains(path, "/ExecutionRuntime/tool-mcp") || strings.Contains(path, "/ExecutionRuntime/context-engine") || strings.Contains(path, "/ExecutionRuntime/runtime/foundation") || strings.Contains(path, "/ExecutionRuntime/runtime/fakes") {
				t.Fatalf("%s imports forbidden implementation or owner package %s", file, path)
			}
		}
	}
}
