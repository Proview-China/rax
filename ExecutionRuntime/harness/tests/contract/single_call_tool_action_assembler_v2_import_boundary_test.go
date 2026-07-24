package contract_test

import (
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func TestSingleCallToolActionAssemblerV2ImportBoundary(t *testing.T) {
	path := "../../applicationadapter/single_call_tool_action_assembler_v2.go"
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range parsed.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"ExecutionRuntime/tool-mcp",
			"ExecutionRuntime/context-engine",
			"ExecutionRuntime/application/fakes",
			"ExecutionRuntime/model-invoker/internal",
			"ExecutionRuntime/model-invoker/execution",
			"ExecutionRuntime/runtime/foundation",
			"ExecutionRuntime/runtime/fakes",
		} {
			if strings.Contains(importPath, forbidden) {
				t.Fatalf("P3 assembler imports forbidden implementation path %q", importPath)
			}
		}
	}
}
