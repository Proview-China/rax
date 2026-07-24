package conformance_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestConformancePackageVerificationImportBoundaryV1(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	files := []string{
		"contract/package_verification_v1.go",
		"packageverify/material_reader_v1.go",
		"packageverify/sigstore_bundle_verifier_v1.go",
		"packageverify/registry_current_v1.go",
		"packageverify/repository_v1.go",
		"packageverify/service_v1.go",
		"packageverify/admission_v1.go",
		"packageverify/admission_port_v1.go",
		"sdk/package_verify_v1.go",
		"api/package_verification_read_v1.go",
	}
	for _, relative := range files {
		path := filepath.Join(root, relative)
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
				"/runtime/kernel", "/runtime/fakes", "/runtime/foundation",
				"/application/", "/harness/", "/model-invoker/", "/context/",
				"os/exec", "database/sql", "unsafe",
			} {
				if importPath == forbidden || strings.Contains(importPath, forbidden) {
					t.Fatalf("%s imports forbidden implementation/network path %q", relative, importPath)
				}
			}
			if importPath == "net" || strings.HasPrefix(importPath, "net/") {
				t.Fatalf("%s imports forbidden network path %q", relative, importPath)
			}
		}
	}
}

func TestConformancePackageVerificationSDKDoesNotExposeFetchInstallOrEnableV1(t *testing.T) {
	client := reflect.TypeOf((*sdk.PackageVerificationV1)(nil))
	for _, forbidden := range []string{"FetchPackageV1", "InstallPackageV1", "EnablePackageV1", "LoadKeyV1", "VerifyURLV1"} {
		if _, ok := client.MethodByName(forbidden); ok {
			t.Fatalf("Package Verification SDK exposes forbidden operation %s", forbidden)
		}
	}
}

func TestConformancePackageVerificationProductionHasNoGenericHooksV1(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", "packageverify"))
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			typeSpec, ok := node.(*ast.TypeSpec)
			if !ok {
				return true
			}
			for _, forbidden := range []string{"Hook", "HookFace", "AnyProvider", "RawProvider", "NetworkFetcher"} {
				if strings.Contains(typeSpec.Name.Name, forbidden) {
					t.Fatalf("%s defines forbidden generic/provider escape type %s", entry.Name(), typeSpec.Name.Name)
				}
			}
			return true
		})
	}
}
