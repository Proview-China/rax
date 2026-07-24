package conformance_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	toolmcp "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/runtimeadapter"
)

func TestConformanceOfficialSDKPhysicalExecutorImplementsRuntimePublicV3(t *testing.T) {
	var _ runtimeports.ControlledOperationPhysicalExecutionPortV3 = (*toolmcp.OfficialSDKPhysicalExecutorV1)(nil)
	var _ toolcontract.MCPExecutionCommandStoreV1 = (*toolmcp.InMemoryMCPExecutionCommandRepositoryV1)(nil)
	var _ toolcontract.MCPExecutionCommandCurrentReaderV1 = (*toolmcp.InMemoryMCPExecutionCommandRepositoryV1)(nil)
	var _ toolmcp.OfficialSDKCallSessionCurrentReaderV1 = (*toolmcp.InMemoryOfficialSDKCallSessionRepositoryV1)(nil)
	var _ toolmcp.MCPListChangedObservationSinkV1 = (*toolmcp.InMemoryMCPListChangedJournalV1)(nil)
	var _ toolmcp.MCPListChangedObservationReaderV1 = (*toolmcp.InMemoryMCPListChangedJournalV1)(nil)
	var _ toolcontract.MCPProtocolReceiptExactReaderV1 = (*toolmcp.InMemoryMCPPhysicalExecutionStoreV1)(nil)
	var _ runtimeports.OperationProviderReceiptReaderV1 = (*runtimeadapter.MCPProtocolReceiptObservationReaderV1)(nil)
}

func TestConformanceOfficialSDKCallImportBoundary(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate conformance test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	files := []string{
		filepath.Join(root, "contract", "mcp_execution_v1.go"),
		filepath.Join(root, "contract", "mcp_protocol_receipt_v1.go"),
		filepath.Join(root, "mcp", "execution_command_store_v1.go"),
		filepath.Join(root, "mcp", "official_sdk_call_v1.go"),
		filepath.Join(root, "contract", "mcp_list_changed_v1.go"),
		filepath.Join(root, "mcp", "list_changed_v1.go"),
		filepath.Join(root, "runtimeadapter", "mcp_protocol_receipt_observation_v1.go"),
	}
	for _, name := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			path := strings.Trim(spec.Path.Value, `"`)
			for _, forbidden := range []string{"/runtime/kernel", "/runtime/fakes", "/application/", "/harness/", "/model-invoker/internal", "/context/", "os/exec", "net/http"} {
				if strings.Contains(path, forbidden) {
					t.Fatalf("%s imports forbidden implementation path %q", name, path)
				}
			}
		}
		for _, declaration := range parsed.Decls {
			if general, ok := declaration.(*ast.GenDecl); ok && general.Tok == token.IMPORT && len(general.Specs) == 0 {
				t.Fatalf("%s contains an empty import declaration", name)
			}
		}
	}
}
