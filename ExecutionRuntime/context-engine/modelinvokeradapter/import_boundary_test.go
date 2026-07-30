package modelinvokeradapter_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	contextContractImportV2 = "github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	contextPortsImportV2    = "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
	modelPublicImportV2     = "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	runtimeCoreImportV2     = "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var contextModelInputPairProductionImportsV2 = map[string]map[string]struct{}{
	"context_owner_binding_v1.go": importSetV2(
		"context",
		"fmt",
		"reflect",
		"time",
		contextContractImportV2,
		contextPortsImportV2,
		modelPublicImportV2,
		runtimeCoreImportV2,
	),
	"invocation_material_context_pair_v2.go": importSetV2(
		"context",
		"encoding/json",
		"fmt",
		"reflect",
		"strings",
		"time",
		contextContractImportV2,
		contextPortsImportV2,
		modelPublicImportV2,
		runtimeCoreImportV2,
	),
	filepath.Join("..", "contract", "model_input_source_current_v1.go"): importSetV2(
		"fmt",
	),
	filepath.Join("..", "ports", "model_input_source_current_v1.go"): importSetV2(
		"context",
		contextContractImportV2,
	),
	filepath.Join("..", "kernel", "model_input_source_current_v1.go"): importSetV2(
		"context",
		"fmt",
		"reflect",
		"time",
		contextContractImportV2,
		contextPortsImportV2,
	),
}

var contextModelInputPairDeniedImportRootsV2 = []string{
	"net",
	"net/http",
	"os",
	"os/exec",
	"database/sql",
	"syscall",
	"plugin",
	"unsafe",

	// External Provider SDKs remain below the Model Owner boundary.
	"github.com/openai",
	"github.com/anthropics",
	"github.com/anthropic-ai",
	"github.com/aws/aws-sdk-go",
	"github.com/aws/aws-sdk-go-v2",
	"github.com/Azure/azure-sdk-for-go",
	"github.com/azure/azure-sdk-for-go",
	"github.com/google/generative-ai-go",
	"cloud.google.com/go",
	"google.golang.org/genai",

	// Model execution implementations are not public contracts.
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider",
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway",
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal",
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/runtimeadapter",

	// Context lowering is not a Harness, Tool, Application, or Host writer.
	"github.com/Proview-China/rax/ExecutionRuntime/harness",
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp",
	"github.com/Proview-China/rax/ExecutionRuntime/application",
	"github.com/Proview-China/rax/ExecutionRuntime/host",
}

func TestContextModelInputPairProductionImportBoundaryV2(t *testing.T) {
	for path, allowed := range contextModelInputPairProductionImportsV2 {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if (filepath.Base(path) == "context_owner_binding_v1.go" ||
			filepath.Base(path) == "invocation_material_context_pair_v2.go") &&
			(strings.Contains(string(source), `"praxis.context/model-input-material"`) ||
				strings.Contains(string(source), `"praxis.context/frame"`)) {
			t.Fatalf("%s duplicated a Context-owned Kind literal", path)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, source, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range parsed.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if deniedContextModelInputPairImportV2(importPath) {
				t.Fatalf("%s imports explicitly denied dependency %q", path, importPath)
			}
			if _, ok := allowed[importPath]; !ok {
				t.Fatalf("%s imports dependency outside its strict allowlist: %q", path, importPath)
			}
		}
	}
}

func TestContextModelInputPairProductionImportDenylistV2(t *testing.T) {
	for _, importPath := range []string{
		"net",
		"net/http",
		"net/url",
		"os",
		"os/exec",
		"os/signal",
		"database/sql",
		"database/sql/driver",
		"syscall",
		"plugin",
		"unsafe",
		"github.com/openai/openai-go",
		"github.com/anthropics/anthropic-sdk-go",
		"github.com/aws/aws-sdk-go-v2/service/bedrockruntime",
		"github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai",
		"cloud.google.com/go/vertexai/genai",
		"google.golang.org/genai",
		"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/provider/openai",
		"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway",
		"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/compatprovider",
		"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/runtimeadapter",
		"github.com/Proview-China/rax/ExecutionRuntime/harness",
		"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp",
	} {
		if !deniedContextModelInputPairImportV2(importPath) {
			t.Fatalf("denylist does not reject %q", importPath)
		}
	}
	for _, importPath := range []string{
		"context",
		"encoding/json",
		"fmt",
		contextContractImportV2,
		contextPortsImportV2,
		modelPublicImportV2,
		runtimeCoreImportV2,
	} {
		if deniedContextModelInputPairImportV2(importPath) {
			t.Fatalf("denylist rejects allowed public dependency %q", importPath)
		}
	}
}

func deniedContextModelInputPairImportV2(importPath string) bool {
	for _, root := range contextModelInputPairDeniedImportRootsV2 {
		if importPath == root || strings.HasPrefix(importPath, root+"/") {
			return true
		}
	}
	return false
}

func importSetV2(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}
