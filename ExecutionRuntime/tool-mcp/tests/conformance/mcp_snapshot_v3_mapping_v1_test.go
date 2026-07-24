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
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

func TestConformanceMCPSnapshotV3MappingExactInterfaces(t *testing.T) {
	var _ toolcontract.MCPCapabilitySnapshotExactReaderV3 = (*mcp.InMemoryMCPCapabilitySnapshotRepositoryV3)(nil)
	var _ toolcontract.MCPToolMappingManifestExactReaderV1 = (*registry.Registry)(nil)
	var _ cli.MCPDiscoveryReadPortV3 = (*api.MCPDiscoveryReadV3)(nil)
}

func TestConformanceMCPSnapshotV3MappingImportBoundary(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	files := []string{
		filepath.Join(root, "contract", "mcp_discovery_v3.go"),
		filepath.Join(root, "contract", "mcp_tool_mapping_v1.go"),
		filepath.Join(root, "registry", "mcp_tool_mapping_v1.go"),
	}
	for _, path := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			value := strings.Trim(spec.Path.Value, "\"")
			if strings.Contains(value, "/application/") || strings.Contains(value, "/harness/") || strings.Contains(value, "/model-invoker/internal") || strings.Contains(value, "/runtime/kernel") || strings.Contains(value, "/runtime/fakes") || strings.Contains(value, "net/http") {
				t.Fatalf("forbidden import %q in %s", value, path)
			}
		}
	}
}
