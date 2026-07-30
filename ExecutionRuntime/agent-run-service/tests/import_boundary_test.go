package tests_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const modulePathV1 = "github.com/Proview-China/rax/ExecutionRuntime/agent-run-service"

func TestProductionImportBoundaryV1(t *testing.T) {
	root := moduleRootV1(t)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range parsed.Imports {
			value, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			first := strings.Split(value, "/")[0]
			isExternal := strings.Contains(first, ".")
			relative, relativeErr := filepath.Rel(root, path)
			if relativeErr != nil {
				return relativeErr
			}
			isSQLiteAdapter := strings.HasPrefix(filepath.ToSlash(relative), "storage/sqlite/")
			isAllowedSQLiteDriver := isSQLiteAdapter && value == "modernc.org/sqlite"
			isGoldenGenerator := strings.HasPrefix(filepath.ToSlash(relative), "conformance/goldenv1/cmd/") && value == modulePathV1+"/conformance/goldenv1"
			if isExternal && value != modulePathV1+"/contract" && value != modulePathV1+"/ports" && value != modulePathV1+"/transport/jsonv1" && !isAllowedSQLiteDriver && !isGoldenGenerator {
				t.Errorf("horizontal production file %s imports non-service package %s", path, value)
			}
			for _, forbidden := range []string{"/internal", "/storage", "/fakes", "/sqlite", "/console", "/frontend"} {
				if strings.Contains(value, forbidden) && !isAllowedSQLiteDriver {
					t.Errorf("horizontal production file %s imports forbidden implementation package %s", path, value)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPublicGoSurfaceHasNoConsoleOrPageContractsV1(t *testing.T) {
	root := moduleRootV1(t)
	forbidden := []string{"AgentVM", "PraxisCommands", "Canvas", "Sidebar", "PageVM", "ConsoleCommand", "TypeScriptDTO"}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, token := range forbidden {
			if strings.Contains(string(content), token) {
				t.Errorf("public Go surface %s contains forbidden Console/page token %s", path, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAgentRunServiceV1MethodSetIsNarrow(t *testing.T) {
	root := moduleRootV1(t)
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "ports", "agent_run_service_v1.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"NegotiateV1": true, "InspectAgentRunV1": true, "InspectOriginalV1": true,
		"WatchAgentRunV1": true, "CancelAgentRunV1": true, "StopAgentHostV1": true,
	}
	found := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || typeSpec.Name.Name != "AgentRunServiceV1" {
			return true
		}
		iface, ok := typeSpec.Type.(*ast.InterfaceType)
		if !ok {
			t.Fatal("AgentRunServiceV1 is not an interface")
		}
		for _, method := range iface.Methods.List {
			for _, name := range method.Names {
				found[name.Name] = true
				if !want[name.Name] {
					t.Errorf("AgentRunServiceV1 exposes unplanned method %s", name.Name)
				}
			}
		}
		return false
	})
	if len(found) != len(want) {
		t.Fatalf("AgentRunServiceV1 methods=%v want=%v", found, want)
	}
}

func moduleRootV1(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate module root")
	}
	return filepath.Dir(filepath.Dir(filename))
}
