package conformance_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/surface"
)

func TestConformanceModelToolMaterializationV1MethodSets(t *testing.T) {
	var _ toolcontract.ToolDefinitionMaterialRepositoryV1 = (*surface.InMemoryToolDefinitionMaterialRepositoryV1)(nil)
	var _ toolcontract.ToolDefinitionMaterialReaderV1 = (*surface.InMemoryToolDefinitionMaterialRepositoryV1)(nil)
}

func TestConformanceMCPStatusV1MethodSet(t *testing.T) {
	var _ sdk.MCPLifecycleReaderV1 = (*mcp.Manager)(nil)
}

func TestConformanceModelToolMaterializationV1ImportBoundary(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate Model Tool materialization conformance test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	files := []string{
		filepath.Join(root, "contract", "tool_definition_material_v1.go"),
		filepath.Join(root, "surface", "tool_definition_material_repository_v1.go"),
		filepath.Join(root, "surface", "model_tool_compiler_v1.go"),
		filepath.Join(root, "sdk", "model_tools_v1.go"),
		filepath.Join(root, "sdk", "mcp_status_v1.go"),
		filepath.Join(root, "cli", "runner_v1.go"),
	}
	for _, name := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			path := strings.Trim(spec.Path.Value, `"`)
			for _, forbidden := range []string{"/model-invoker/internal", "/model-invoker/provider", "/harness/", "/application/", "/runtime/kernel", "/runtime/fakes", "github.com/openai/", "github.com/anthropics/", "google.golang.org/genai", "os/exec", "net/http"} {
				if strings.Contains(path, forbidden) {
					t.Fatalf("%s imports forbidden implementation or vendor path %q", name, path)
				}
			}
		}
	}
}
