package core_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

var forbiddenProviderSDKImports = []string{
	"github.com/openai/openai-go",
	"github.com/anthropics/anthropic-sdk-go",
	"github.com/aws/aws-sdk-go-v2",
	"google.golang.org/genai",
	"golang.org/x/oauth2",
}

func TestPublicExportedSignaturesDoNotExposeProviderSDKTypes(t *testing.T) {
	root := modelInvokerSourceRoot(t)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root {
				relative, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				first := strings.Split(filepath.ToSlash(relative), "/")[0]
				if first == "internal" || first == "tests" || first == "scripts" || strings.HasPrefix(first, ".") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		aliases := sdkImportAliases(t, path, file)
		if len(aliases) == 0 {
			return nil
		}
		for _, declaration := range file.Decls {
			switch typed := declaration.(type) {
			case *ast.FuncDecl:
				if functionIsPublic(typed) {
					assertNoSDKSelector(t, path, typed.Type, aliases)
				}
			case *ast.GenDecl:
				for _, specification := range typed.Specs {
					switch spec := specification.(type) {
					case *ast.TypeSpec:
						if ast.IsExported(spec.Name.Name) {
							assertPublicTypeHasNoSDKSelector(t, path, spec.Type, aliases)
						}
					case *ast.ValueSpec:
						if valueSpecIsExported(spec) {
							assertNoSDKSelector(t, path, spec.Type, aliases)
							for _, value := range spec.Values {
								assertNoSDKSelector(t, path, value, aliases)
							}
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProviderErrorFilesDoNotOwnSDKFailureExtraction(t *testing.T) {
	root := modelInvokerSourceRoot(t)
	paths, err := filepath.Glob(filepath.Join(root, "provider", "*", "errors.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 {
		t.Fatalf("provider errors.go count = %d, want 3", len(paths))
	}
	for _, path := range paths {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range file.Imports {
			value, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if isProviderSDKImport(value) {
				t.Errorf("%s retains provider SDK failure extraction import %q", path, value)
			}
		}
		full, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range full.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			switch function.Name.Name {
			case "normalizeError", "streamErrorPayload", "errorRawPayload", "rawErrorPayload":
				t.Errorf("%s retains duplicate provider failure extractor %s", path, function.Name.Name)
			}
		}
	}
}

func modelInvokerSourceRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
}

func sdkImportAliases(t *testing.T, path string, file *ast.File) map[string]string {
	t.Helper()
	aliases := make(map[string]string)
	for _, imported := range file.Imports {
		value, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatalf("unquote import in %s: %v", path, err)
		}
		if !isProviderSDKImport(value) {
			continue
		}
		name := filepath.Base(value)
		if imported.Name != nil {
			name = imported.Name.Name
		}
		if name == "." {
			t.Errorf("public package file %s dot-imports provider SDK %q", path, value)
			continue
		}
		aliases[name] = value
	}
	return aliases
}

func isProviderSDKImport(value string) bool {
	for _, forbidden := range forbiddenProviderSDKImports {
		if value == forbidden || strings.HasPrefix(value, forbidden+"/") {
			return true
		}
	}
	return false
}

func assertPublicTypeHasNoSDKSelector(t *testing.T, path string, expression ast.Expr, aliases map[string]string) {
	t.Helper()
	structure, ok := expression.(*ast.StructType)
	if !ok {
		assertNoSDKSelector(t, path, expression, aliases)
		return
	}
	for _, field := range structure.Fields.List {
		exported := len(field.Names) == 0
		for _, name := range field.Names {
			exported = exported || ast.IsExported(name.Name)
		}
		if exported {
			assertNoSDKSelector(t, path, field.Type, aliases)
		}
	}
}

func assertNoSDKSelector(t *testing.T, path string, node ast.Node, aliases map[string]string) {
	t.Helper()
	if node == nil {
		return
	}
	ast.Inspect(node, func(candidate ast.Node) bool {
		selector, ok := candidate.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if imported, forbidden := aliases[identifier.Name]; forbidden {
			t.Errorf("public signature in %s exposes %s.%s from %q", path, identifier.Name, selector.Sel.Name, imported)
		}
		return true
	})
}

func valueSpecIsExported(spec *ast.ValueSpec) bool {
	for _, name := range spec.Names {
		if ast.IsExported(name.Name) {
			return true
		}
	}
	return false
}

func functionIsPublic(function *ast.FuncDecl) bool {
	if function == nil || !ast.IsExported(function.Name.Name) {
		return false
	}
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return true
	}
	receiver := function.Recv.List[0].Type
	for {
		switch typed := receiver.(type) {
		case *ast.StarExpr:
			receiver = typed.X
		case *ast.IndexExpr:
			receiver = typed.X
		case *ast.IndexListExpr:
			receiver = typed.X
		case *ast.Ident:
			return ast.IsExported(typed.Name)
		default:
			return false
		}
	}
}
