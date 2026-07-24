package conformance_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/cli"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestConformanceMCPToolDiscoveryMaterialReadV1ExactOnly(t *testing.T) {
	var _ toolcontract.MCPToolDiscoveryMaterialExactReaderV1 = (*sdk.MCPToolDiscoveryMaterialV1)(nil)
	var _ toolcontract.MCPToolDiscoveryMaterialExactReaderV1 = (*api.MCPToolDiscoveryMaterialReadV1)(nil)
	var _ cli.MCPToolDiscoveryMaterialReadPortV1 = (*sdk.MCPToolDiscoveryMaterialV1)(nil)
	var _ cli.MCPToolDiscoveryMaterialReadPortV1 = (*api.MCPToolDiscoveryMaterialReadV1)(nil)
	var _ toolcontract.MCPDiscoveryPageToolMaterialSetExactReaderV1 = (*sdk.MCPDiscoveryPageToolMaterialSetV1)(nil)
	var _ toolcontract.MCPDiscoveryPageToolMaterialSetExactReaderV1 = (*api.MCPDiscoveryPageToolMaterialSetReadV1)(nil)
	var _ cli.MCPDiscoveryPageToolMaterialSetReadPortV1 = (*sdk.MCPDiscoveryPageToolMaterialSetV1)(nil)
	var _ cli.MCPDiscoveryPageToolMaterialSetReadPortV1 = (*api.MCPDiscoveryPageToolMaterialSetReadV1)(nil)
	var _ toolcontract.MCPResourceDiscoveryMaterialExactReaderV1 = (*sdk.MCPResourceDiscoveryMaterialV1)(nil)
	var _ toolcontract.MCPResourceDiscoveryMaterialExactReaderV1 = (*api.MCPResourceDiscoveryMaterialReadV1)(nil)
	var _ cli.MCPResourceDiscoveryMaterialReadPortV1 = (*sdk.MCPResourceDiscoveryMaterialV1)(nil)
	var _ cli.MCPResourceDiscoveryMaterialReadPortV1 = (*api.MCPResourceDiscoveryMaterialReadV1)(nil)
	var _ toolcontract.MCPPromptDiscoveryMaterialExactReaderV1 = (*sdk.MCPPromptDiscoveryMaterialV1)(nil)
	var _ toolcontract.MCPPromptDiscoveryMaterialExactReaderV1 = (*api.MCPPromptDiscoveryMaterialReadV1)(nil)
	var _ cli.MCPPromptDiscoveryMaterialReadPortV1 = (*sdk.MCPPromptDiscoveryMaterialV1)(nil)
	var _ cli.MCPPromptDiscoveryMaterialReadPortV1 = (*api.MCPPromptDiscoveryMaterialReadV1)(nil)
	var _ toolcontract.MCPDiscoveryPageResourceMaterialSetExactReaderV1 = (*sdk.MCPDiscoveryPageResourceMaterialSetV1)(nil)
	var _ toolcontract.MCPDiscoveryPageResourceMaterialSetExactReaderV1 = (*api.MCPDiscoveryPageResourceMaterialSetReadV1)(nil)
	var _ cli.MCPDiscoveryPageResourceMaterialSetReadPortV1 = (*sdk.MCPDiscoveryPageResourceMaterialSetV1)(nil)
	var _ cli.MCPDiscoveryPageResourceMaterialSetReadPortV1 = (*api.MCPDiscoveryPageResourceMaterialSetReadV1)(nil)
	var _ toolcontract.MCPDiscoveryPagePromptMaterialSetExactReaderV1 = (*sdk.MCPDiscoveryPagePromptMaterialSetV1)(nil)
	var _ toolcontract.MCPDiscoveryPagePromptMaterialSetExactReaderV1 = (*api.MCPDiscoveryPagePromptMaterialSetReadV1)(nil)
	var _ cli.MCPDiscoveryPagePromptMaterialSetReadPortV1 = (*sdk.MCPDiscoveryPagePromptMaterialSetV1)(nil)
	var _ cli.MCPDiscoveryPagePromptMaterialSetReadPortV1 = (*api.MCPDiscoveryPagePromptMaterialSetReadV1)(nil)
	for _, value := range []any{(*sdk.MCPToolDiscoveryMaterialV1)(nil), (*api.MCPToolDiscoveryMaterialReadV1)(nil), (*sdk.MCPDiscoveryPageToolMaterialSetV1)(nil), (*api.MCPDiscoveryPageToolMaterialSetReadV1)(nil), (*sdk.MCPResourceDiscoveryMaterialV1)(nil), (*api.MCPResourceDiscoveryMaterialReadV1)(nil), (*sdk.MCPPromptDiscoveryMaterialV1)(nil), (*api.MCPPromptDiscoveryMaterialReadV1)(nil), (*sdk.MCPDiscoveryPageResourceMaterialSetV1)(nil), (*api.MCPDiscoveryPageResourceMaterialSetReadV1)(nil), (*sdk.MCPDiscoveryPagePromptMaterialSetV1)(nil), (*api.MCPDiscoveryPagePromptMaterialSetReadV1)(nil)} {
		typeOf := reflect.TypeOf(value)
		for _, forbidden := range []string{"Discover", "Map", "Admit", "Enable", "Call", "Invoke", "Register", "Transition"} {
			if _, exists := typeOf.MethodByName(forbidden); exists {
				t.Fatalf("exact material reader exposes forbidden method %s", forbidden)
			}
		}
	}
}

func TestConformanceMCPToolDiscoveryMaterialReadV1ImportBoundary(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate MCP Tool Discovery Material conformance test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	for _, name := range []string{
		filepath.Join(root, "contract", "mcp_tool_discovery_material_v1.go"),
		filepath.Join(root, "sdk", "mcp_tool_discovery_material_v1.go"),
		filepath.Join(root, "api", "mcp_tool_discovery_material_read_v1.go"),
		filepath.Join(root, "contract", "mcp_discovery_page_tool_material_set_v1.go"),
		filepath.Join(root, "sdk", "mcp_discovery_page_tool_material_set_v1.go"),
		filepath.Join(root, "api", "mcp_discovery_page_tool_material_set_read_v1.go"),
		filepath.Join(root, "contract", "mcp_resource_prompt_discovery_material_v1.go"),
		filepath.Join(root, "sdk", "mcp_resource_prompt_discovery_material_v1.go"),
		filepath.Join(root, "api", "mcp_resource_prompt_discovery_material_read_v1.go"),
		filepath.Join(root, "contract", "mcp_discovery_page_resource_prompt_material_set_v1.go"),
		filepath.Join(root, "sdk", "mcp_discovery_page_resource_prompt_material_set_v1.go"),
		filepath.Join(root, "api", "mcp_discovery_page_resource_prompt_material_set_read_v1.go"),
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			path := strings.Trim(spec.Path.Value, `"`)
			for _, forbidden := range []string{"/runtime/kernel", "/runtime/fakes", "/application/", "/harness/", "/model-invoker/", "/context/", "github.com/modelcontextprotocol/go-sdk", "net/http", "os/exec", "database/sql", "unsafe"} {
				if strings.Contains(path, forbidden) {
					t.Fatalf("%s imports forbidden implementation/provider path %q", name, path)
				}
			}
		}
	}
}
