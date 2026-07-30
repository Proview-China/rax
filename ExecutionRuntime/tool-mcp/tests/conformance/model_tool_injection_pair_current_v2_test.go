package conformance_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestConformanceModelToolInjectionPairCurrentV2ImportAndHelperBoundary(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate Model Tool Injection pair current conformance test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "..", ".."))
	files := []string{
		filepath.Join(root, "contract", "model_tool_injection_pair_current_v2.go"),
		filepath.Join(root, "surface", "model_tool_injection_pair_current_v2.go"),
		filepath.Join(root, "modelinvokeradapter", "invocation_material_tool_pair_v2.go"),
	}
	for _, path := range files {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
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
			for _, forbidden := range []string{
				"/model-invoker/internal",
				"/model-invoker/provider",
				"/model-invoker/execution",
				"/model-invoker/operation",
				"/model-invoker/routegateway",
				"/harness",
				"/sandbox",
				"/tool-mcp/action",
				"/tool-mcp/mcp",
				"/runtimeadapter",
				"/storage/sqlite",
				"os/exec",
				"net/http",
			} {
				if strings.Contains(importPath, forbidden) {
					t.Fatalf("%s imports forbidden implementation path %q", path, importPath)
				}
			}
		}
	}

	surfacePath := filepath.Join(root, "surface", "model_tool_injection_pair_current_v2.go")
	surfaceSource, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatal(err)
	}
	surfaceText := string(surfaceSource)
	if !strings.Contains(surfaceText, "modelinvoker.DigestGovernedModelTurnRequestToolSetV2") {
		t.Fatal("Tool pair current reader does not call the merged Model canonical body helper")
	}
	for _, forbidden := range []string{
		"modelinvoker.RouteCall{",
		"DigestGovernedModelTurnRequestToolsV2(",
		"canonicalGovernedModelTurnRequestToolSetV2",
	} {
		if strings.Contains(surfaceText, forbidden) {
			t.Fatalf("Tool pair current reader copied Model lowering or synthesized RouteCall via %q", forbidden)
		}
	}

	adapterPath := filepath.Join(root, "modelinvokeradapter", "invocation_material_tool_pair_v2.go")
	adapterSource, err := os.ReadFile(adapterPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbiddenLiteral := range []string{
		`"praxis.tool/model-tool-injection-material"`,
		`"praxis.tool/surface-manifest-current"`,
		"InvokeProvider",
		"ExecuteTool",
	} {
		if strings.Contains(string(adapterSource), forbiddenLiteral) {
			t.Fatalf("Model adapter duplicated Tool-owned Kind literal %s", forbiddenLiteral)
		}
	}
}
