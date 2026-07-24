package conformance_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/cli"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestConformanceMCPCallReadV1UsesExactOwnerFacts(t *testing.T) {
	var _ cli.MCPCallReadPortV1 = (*api.MCPCallReadV1)(nil)
	var _ cli.MCPCallReadPortV1 = (*sdk.MCPCallV1)(nil)
}

func TestConformanceMCPCallReadV1ImportBoundary(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate MCP Call read conformance test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	for _, name := range []string{
		filepath.Join(root, "api", "mcp_call_read_v1.go"),
		filepath.Join(root, "sdk", "mcp_call_v1.go"),
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			path := strings.Trim(spec.Path.Value, `"`)
			for _, forbidden := range []string{"/runtime/kernel", "/runtime/fakes", "/application/", "/harness/", "/model-invoker/", "/context/", "/tool-mcp/mcp"} {
				if strings.Contains(path, forbidden) {
					t.Fatalf("%s imports forbidden implementation path %q", name, path)
				}
			}
		}
	}
}
