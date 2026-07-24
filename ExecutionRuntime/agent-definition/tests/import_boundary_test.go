package tests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestProductionImportBoundaryV1(t *testing.T) {
	root := ".."
	const modulePrefix = "github.com/Proview-China/rax/ExecutionRuntime/agent-definition"
	const executionRuntimePrefix = "github.com/Proview-China/rax/ExecutionRuntime/"
	const runtimeCore = "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if strings.Contains(path, string(filepath.Separator)+"tests") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			value, _ := strconv.Unquote(spec.Path.Value)
			switch value {
			case "net", "net/http", "net/url", "os", "os/exec", "syscall":
				t.Errorf("%s imports forbidden side-effect surface %s", path, value)
			}
			if strings.HasPrefix(value, modulePrefix+"/") {
				relative := strings.TrimPrefix(value, modulePrefix+"/")
				for _, forbidden := range []string{"conformance", "internal", "fakes", "testkit"} {
					if relative == forbidden || strings.HasPrefix(relative, forbidden+"/") || strings.Contains(relative, "/"+forbidden+"/") {
						t.Errorf("%s imports forbidden agent-definition support package %s", path, value)
					}
				}
				continue
			}
			if !strings.HasPrefix(value, executionRuntimePrefix) {
				continue
			}
			if value != runtimeCore && !strings.HasPrefix(value, runtimeCore+"/") {
				t.Errorf("%s imports forbidden module %s", path, value)
			}
		}
		_ = ast.File{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
