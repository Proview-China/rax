package protocol_test

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const modelInvokerImport = "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"

func TestProtocolProductionCodeHasNoProviderDependencyOrHardcodedProviderIdentity(t *testing.T) {
	forbiddenImport := modelInvokerImport + "/provider/"
	forbiddenIdentities := map[string]struct{}{"openai": {}, "anthropic": {}, "gemini": {}}
	fset := token.NewFileSet()
	for _, path := range allProtocolProductionFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imported := range file.Imports {
			value, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", path, err)
			}
			if strings.HasPrefix(value, forbiddenImport) {
				t.Errorf("%s imports forbidden provider package %q", filepath.Base(path), value)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err == nil {
				if _, forbidden := forbiddenIdentities[strings.ToLower(value)]; forbidden {
					t.Errorf("%s hardcodes provider identity %q", filepath.Base(path), value)
				}
			}
			return true
		})
	}
}

func TestProtocolRootHasNoProviderSDKDependency(t *testing.T) {
	forbiddenImports := []string{
		"github.com/openai/openai-go",
		"github.com/anthropics/anthropic-sdk-go",
		"google.golang.org/genai",
	}
	fset := token.NewFileSet()
	for _, path := range protocolRootProductionFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imported := range file.Imports {
			value, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", path, err)
			}
			for _, forbidden := range forbiddenImports {
				if value == forbidden || strings.HasPrefix(value, forbidden+"/") {
					t.Errorf("protocol root file %s imports forbidden SDK package %q", filepath.Base(path), value)
				}
			}
		}
	}
}

func TestBindingAndFailureASTShapesAreExactAndSDKNeutral(t *testing.T) {
	expected := map[string]map[string]string{
		"Binding": {
			"Provider":         modelInvokerImport + ".ProviderID",
			"Protocol":         modelInvokerImport + ".Protocol",
			"Endpoint":         "string",
			"RequestIDHeaders": "[]string",
		},
		"Failure": {
			"Source":     "FailureSource",
			"Context":    "FailureContext",
			"HTTPStatus": "int",
			"Type":       "string",
			"Code":       "string",
			"Message":    "string",
			"RequestID":  "string",
			"RetryAfter": "time.Duration",
			"Signals":    "[]Signal",
			"Raw":        modelInvokerImport + ".RawPayload",
		},
	}
	found := make(map[string]bool, len(expected))
	fset := token.NewFileSet()
	for _, path := range protocolRootProductionFiles(t) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		imports := importAliases(t, file)
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, specification := range generic.Specs {
				typeSpec := specification.(*ast.TypeSpec)
				want, guarded := expected[typeSpec.Name.Name]
				if !guarded {
					continue
				}
				if found[typeSpec.Name.Name] {
					t.Fatalf("duplicate guarded type %s", typeSpec.Name.Name)
				}
				found[typeSpec.Name.Name] = true
				structure, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					t.Fatalf("%s must remain a struct", typeSpec.Name.Name)
				}
				actual := make(map[string]string)
				for _, field := range structure.Fields.List {
					if len(field.Names) == 0 {
						t.Errorf("%s contains forbidden embedded field %s", typeSpec.Name.Name, renderNode(t, fset, field.Type))
						continue
					}
					fieldType := canonicalASTType(t, fset, field.Type, imports)
					for _, name := range field.Names {
						actual[name.Name] = fieldType
					}
				}
				if diff := fieldShapeDiff(want, actual); diff != "" {
					t.Errorf("%s field shape changed:\n%s", typeSpec.Name.Name, diff)
				}
			}
		}
	}
	for name := range expected {
		if !found[name] {
			t.Errorf("guarded type %s not found", name)
		}
	}
}

func protocolRootProductionFiles(t *testing.T) []string {
	t.Helper()
	directory := protocolSourceDirectory(t)
	entries, err := fs.ReadDir(os.DirFS(directory), ".")
	if err != nil {
		t.Fatalf("read internal/protocol: %v", err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		files = append(files, filepath.Join(directory, entry.Name()))
	}
	if len(files) == 0 {
		t.Fatal("no internal/protocol root production files found")
	}
	sort.Strings(files)
	return files
}

func allProtocolProductionFiles(t *testing.T) []string {
	t.Helper()
	directory := protocolSourceDirectory(t)
	var files []string
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/protocol: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no internal/protocol production files found")
	}
	sort.Strings(files)
	return files
}

func protocolSourceDirectory(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	return filepath.Join(root, "internal", "protocol")
}

func importAliases(t *testing.T, file *ast.File) map[string]string {
	t.Helper()
	aliases := make(map[string]string, len(file.Imports))
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(path)
		if imported.Name != nil {
			name = imported.Name.Name
		}
		aliases[name] = path
	}
	return aliases
}

func canonicalASTType(t *testing.T, fset *token.FileSet, expression ast.Expr, imports map[string]string) string {
	t.Helper()
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		identifier, ok := value.X.(*ast.Ident)
		if !ok {
			return renderNode(t, fset, expression)
		}
		prefix := identifier.Name
		if imported, exists := imports[prefix]; exists {
			prefix = imported
		}
		return prefix + "." + value.Sel.Name
	case *ast.ArrayType:
		if value.Len != nil {
			return renderNode(t, fset, expression)
		}
		return "[]" + canonicalASTType(t, fset, value.Elt, imports)
	case *ast.StarExpr:
		return "*" + canonicalASTType(t, fset, value.X, imports)
	case *ast.MapType:
		return "map[" + canonicalASTType(t, fset, value.Key, imports) + "]" + canonicalASTType(t, fset, value.Value, imports)
	default:
		return renderNode(t, fset, expression)
	}
}

func renderNode(t *testing.T, fset *token.FileSet, node any) string {
	t.Helper()
	var buffer bytes.Buffer
	if err := format.Node(&buffer, fset, node); err != nil {
		t.Fatalf("render AST node: %v", err)
	}
	return buffer.String()
}

func fieldShapeDiff(want, got map[string]string) string {
	keys := make(map[string]struct{}, len(want)+len(got))
	for key := range want {
		keys[key] = struct{}{}
	}
	for key := range got {
		keys[key] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	var lines []string
	for _, key := range ordered {
		if want[key] != got[key] {
			lines = append(lines, fmt.Sprintf("  %s: got %q, want %q", key, got[key], want[key]))
		}
	}
	return strings.Join(lines, "\n")
}
