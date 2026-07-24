package conformance_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	toolapi "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	toolcli "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/cli"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestConformanceSDKV1HasNoRegistryAdmissionOrProviderBackdoor(t *testing.T) {
	client, err := sdk.NewV1(registry.New(), func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"TransitionRegistryObjectV1", "Call", "Connect", "Invoke", "EnterProvider"} {
		if _, ok := reflect.TypeOf(client).MethodByName(forbidden); ok {
			t.Fatalf("SDK exposes forbidden admission/provider method %s", forbidden)
		}
	}

	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate SDK conformance test")
	}
	filename := filepath.Clean(filepath.Join(filepath.Dir(here), "..", "..", "sdk", "sdk_v1.go"))
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	forbiddenImports := []string{
		"ExecutionRuntime/application", "ExecutionRuntime/model-invoker", "ExecutionRuntime/harness",
		"ExecutionRuntime/runtime/kernel", "ExecutionRuntime/runtime/fakes", "ExecutionRuntime/runtime/internal",
		"github.com/modelcontextprotocol/go-sdk", "net", "net/http", "os/exec", "database/sql", "unsafe",
	}
	for _, declaration := range parsed.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, item := range general.Specs {
			importSpec, ok := item.(*ast.ImportSpec)
			if !ok {
				continue
			}
			path, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			for _, blocked := range forbiddenImports {
				if path == blocked || strings.Contains(path, "/"+blocked) || strings.HasPrefix(path, blocked+"/") {
					t.Fatalf("SDK imports forbidden execution/provider package %s", path)
				}
			}
		}
	}
}

func TestConformanceCLIV1HasNoProviderOrProcessBackdoor(t *testing.T) {
	client, err := sdk.NewV1(registry.New(), func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := toolapi.NewCatalogV1(client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = toolcli.NewRunnerV1(catalog, client); err != nil {
		t.Fatal(err)
	}
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate CLI conformance test")
	}
	filename := filepath.Clean(filepath.Join(filepath.Dir(here), "..", "..", "cli", "runner_v1.go"))
	assertNoForbiddenImportsV1(t, filename, []string{
		"ExecutionRuntime/application", "ExecutionRuntime/model-invoker", "ExecutionRuntime/harness",
		"ExecutionRuntime/runtime/kernel", "ExecutionRuntime/runtime/fakes", "ExecutionRuntime/runtime/internal",
		"github.com/modelcontextprotocol/go-sdk", "net", "net/http", "os", "os/exec", "database/sql", "unsafe",
	})
}

func TestConformanceCatalogAPIV1IsTransportNeutralAndReadOnly(t *testing.T) {
	client, err := sdk.NewV1(registry.New(), func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := toolapi.NewCatalogV1(client)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"Call", "Connect", "Invoke", "Register", "Transition", "Listen", "Serve"} {
		if _, ok := reflect.TypeOf(catalog).MethodByName(forbidden); ok {
			t.Fatalf("Catalog API exposes forbidden mutation/transport method %s", forbidden)
		}
	}

	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate Catalog API conformance test")
	}
	for _, name := range []string{"catalog_v1.go", "catalog_inspect_v1.go"} {
		filename := filepath.Clean(filepath.Join(filepath.Dir(here), "..", "..", "api", name))
		assertNoForbiddenImportsV1(t, filename, []string{
			"ExecutionRuntime/application", "ExecutionRuntime/model-invoker", "ExecutionRuntime/harness",
			"ExecutionRuntime/runtime/kernel", "ExecutionRuntime/runtime/fakes", "ExecutionRuntime/runtime/internal",
			"github.com/modelcontextprotocol/go-sdk", "net", "net/http", "os/exec", "database/sql", "unsafe",
		})
	}
}

func assertNoForbiddenImportsV1(t *testing.T, filename string, forbiddenImports []string) {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range parsed.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, item := range general.Specs {
			importSpec, ok := item.(*ast.ImportSpec)
			if !ok {
				continue
			}
			path, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			for _, blocked := range forbiddenImports {
				if path == blocked || strings.Contains(path, "/"+blocked) || strings.HasPrefix(path, blocked+"/") {
					t.Fatalf("%s imports forbidden package %s", filename, path)
				}
			}
		}
	}
}
