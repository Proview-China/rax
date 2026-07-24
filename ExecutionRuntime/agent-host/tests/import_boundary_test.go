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

func TestProductionImportBoundary(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate import boundary test")
	}
	root := filepath.Dir(filepath.Dir(filename))
	forbidden := []string{"/runtime/foundation", "/fakes", "/internal", "/testkit", "openai-go", "anthropic-sdk", "aws-sdk", "google.golang.org/genai"}
	ownerPublic := []string{
		"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/",
		"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/",
		"github.com/Proview-China/rax/ExecutionRuntime/harness/",
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/",
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
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
			for _, blocked := range forbidden {
				if strings.Contains(value, blocked) {
					t.Errorf("production file %s imports forbidden package %s", path, value)
				}
			}
			allowedOwnerPublic := false
			if strings.HasPrefix(path, filepath.Join(root, "owneradapter")+string(filepath.Separator)) {
				for _, prefix := range ownerPublic {
					allowedOwnerPublic = allowedOwnerPublic || strings.HasPrefix(value, prefix)
				}
			}
			relative, _ := filepath.Rel(root, path)
			allowedAdditiveContractPublic := map[string]map[string]bool{
				filepath.Join("composition", "root_v1.go"): {
					"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract":  true,
					"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract": true,
					"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/decoder":  true,
					"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports":    true,
					"github.com/Proview-China/rax/ExecutionRuntime/runtime/core":              true,
				},
				filepath.Join("ports", "declarative_root_v1.go"): {
					"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract":  true,
					"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract": true,
					"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/ports":    true,
				},
				filepath.Join("bootstrap", "decoder_v1.go"): {
					"github.com/Proview-China/rax/ExecutionRuntime/runtime/core": true,
					"gopkg.in/yaml.v3": true,
				},
				filepath.Join("contract", "bootstrap_v1.go"): {
					"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports": true,
				},
				filepath.Join("contract", "cleanup_closure_v2.go"): {
					"github.com/Proview-China/rax/ExecutionRuntime/runtime/core":  true,
					"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports": true,
				},
				filepath.Join("contract", "cleanup_dispatch_v3.go"): {
					"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports": true,
				},
				filepath.Join("contract", "host_v3.go"): {
					"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports": true,
				},
				filepath.Join("contract", "review_model_invocation_association_v1.go"): {
					"github.com/Proview-China/rax/ExecutionRuntime/model-invoker": true,
					"github.com/Proview-China/rax/ExecutionRuntime/runtime/core":  true,
				},
				filepath.Join("contract", "assembly.go"): {
					"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract": true,
				},
				filepath.Join("contract", "assembly_publication_v2.go"): {
					"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract": true,
					"github.com/Proview-China/rax/ExecutionRuntime/runtime/core":             true,
					"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports":            true,
				},
				filepath.Join("contract", "host_v2.go"): {
					"github.com/Proview-China/rax/ExecutionRuntime/application/contract": true,
					"github.com/Proview-China/rax/ExecutionRuntime/runtime/core":         true,
					"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports":        true,
				},
				filepath.Join("ports", "host_v2.go"): {
					"github.com/Proview-China/rax/ExecutionRuntime/application/contract": true,
					"github.com/Proview-China/rax/ExecutionRuntime/application/ports":    true,
					"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports":        true,
				},
				filepath.Join("lifecycle", "host_v2.go"): {
					"github.com/Proview-China/rax/ExecutionRuntime/application/contract": true,
					"github.com/Proview-China/rax/ExecutionRuntime/runtime/core":         true,
					"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports":        true,
				},
			}[relative][value]
			inSQLiteStore := strings.HasPrefix(relative, filepath.Join("storage", "sqlite")+string(filepath.Separator))
			allowedSQLiteDependency := inSQLiteStore && (value == "github.com/Proview-China/rax/ExecutionRuntime/runtime/core" ||
				value == "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports" ||
				value == "modernc.org/sqlite")
			allowedRuntimeNeutralFile := map[string]bool{
				filepath.Join("contract", "control_factory_v2.go"):      true,
				filepath.Join("contract", "system_ready_v2.go"):         true,
				filepath.Join("contract", "system_ready_gateway_v2.go"): true,
				filepath.Join("ports", "ports.go"):                      true,
				filepath.Join("ports", "control_adapter_gateway_v2.go"): true,
				filepath.Join("ports", "system_ready_gateway_v2.go"):    true,
				filepath.Join("journal", "system_ready_store.go"):       true,
				filepath.Join("journal", "system_ready_gateway_v2.go"):  true,
			}[relative]
			allowedRuntimeNeutral := allowedRuntimeNeutralFile &&
				(value == "github.com/Proview-China/rax/ExecutionRuntime/runtime/core" || value == "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports")
			if strings.Contains(strings.Split(value, "/")[0], ".") && !strings.HasPrefix(value, "github.com/Proview-China/rax/ExecutionRuntime/agent-host/") && !allowedOwnerPublic && !allowedAdditiveContractPublic && !allowedRuntimeNeutral && !allowedSQLiteDependency {
				t.Errorf("H1-H4 production file %s imports non-host or non-owner-public package %s", path, value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHostAPIHasNoPrivilegedBypassMethod(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(filename))
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, "ports", "ports.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"Validate": true, "Assemble": true, "Start": true, "Inspect": true, "Stop": true}
	found := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || typeSpec.Name.Name != "HostV1" {
			return true
		}
		iface, ok := typeSpec.Type.(*ast.InterfaceType)
		if !ok {
			t.Fatal("HostV1 is not an interface")
		}
		for _, method := range iface.Methods.List {
			for _, name := range method.Names {
				found[name.Name] = true
				if !allowed[name.Name] {
					t.Errorf("HostV1 exposes bypass method %s", name.Name)
				}
			}
		}
		return false
	})
	if len(found) != len(allowed) {
		t.Fatalf("HostV1 methods=%v want=%v", found, allowed)
	}
}
