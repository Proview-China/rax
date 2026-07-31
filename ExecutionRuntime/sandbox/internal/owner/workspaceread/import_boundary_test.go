package workspaceread_test

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

func TestWorkspaceReadAuthorizedReservationV2InternalImportBoundary(t *testing.T) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate import-boundary test")
	}
	executionRuntime := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../.."))
	internalImport := "github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/owner/workspaceread"
	err := filepath.WalkDir(executionRuntime, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != executionRuntime && strings.HasPrefix(path, filepath.Join(executionRuntime, "sandbox")) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, spec := range file.Imports {
			value, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				return unquoteErr
			}
			if value == internalImport {
				t.Errorf("external Owner imports Sandbox internal writer capability: %s", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Public ports remain read-only: the writer name cannot be exported there.
	portsDir := filepath.Join(executionRuntime, "sandbox", "ports")
	err = filepath.WalkDir(portsDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || filepath.Ext(path) != ".go" {
			return walkErr
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if ident, ok := node.(*ast.Ident); ok && ident.Name == "ReserveWorkspaceReadAuthorizedV2" {
				t.Errorf("public ports export owner-private writer in %s", path)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
