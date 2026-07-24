package conformance_test

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

func TestConformanceToolMCPDefinesNoGenericHookfaceOrPrivateOwnerImport(t *testing.T) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate Tool/MCP conformance test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "..", ".."))
	blockedTypes := map[string]struct{}{
		"Hook": {}, "HookFace": {}, "Hookface": {}, "GenericHook": {},
		"ContextHook": {}, "TurnHook": {}, "PhaseHook": {}, "Slot": {}, "Phase": {},
	}
	blockedMethods := map[string]struct{}{
		"BeforeTurn": {}, "AfterTurn": {}, "MutateContext": {}, "WriteFact": {}, "OpenNetwork": {},
	}
	blockedImports := []string{
		"ExecutionRuntime/harness/internal", "ExecutionRuntime/harness/kernel", "ExecutionRuntime/harness/fakes",
		"ExecutionRuntime/harness/ports", "ExecutionRuntime/context", "ExecutionRuntime/model-invoker/internal",
		"ExecutionRuntime/runtime/kernel", "ExecutionRuntime/runtime/fakes", "ExecutionRuntime/runtime/internal",
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly|parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, spec := range parsed.Imports {
			value, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			for _, blocked := range blockedImports {
				if strings.Contains(value, blocked) {
					t.Fatalf("%s imports private cross-owner package %q", path, value)
				}
			}
		}
		parsed, err = parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, declaration := range parsed.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, item := range general.Specs {
				typeSpec, ok := item.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if _, blocked := blockedTypes[typeSpec.Name.Name]; blocked {
					t.Fatalf("%s defines forbidden generic hook/slot/phase type %s", path, typeSpec.Name.Name)
				}
				interfaceType, ok := typeSpec.Type.(*ast.InterfaceType)
				if !ok || interfaceType.Methods == nil {
					continue
				}
				for _, method := range interfaceType.Methods.List {
					for _, name := range method.Names {
						if _, blocked := blockedMethods[name.Name]; blocked {
							t.Fatalf("%s interface %s defines forbidden generic hook method %s", path, typeSpec.Name.Name, name.Name)
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
