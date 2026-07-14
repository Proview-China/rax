package contract_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHarnessApplicationImportsAreConfinedToPublicBridgeLayers(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate Harness tests")
	}
	harnessRoot := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	allowed := map[string]bool{"applicationadapter": true, "bridgecontract": true}
	err := filepath.WalkDir(harnessRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "tests" || entry.Name() == "internal" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(harnessRoot, path)
		top := strings.Split(filepath.ToSlash(rel), "/")[0]
		for _, imported := range file.Imports {
			value := strings.Trim(imported.Path.Value, "\"")
			if !strings.Contains(value, "/ExecutionRuntime/application") {
				continue
			}
			if !allowed[top] {
				t.Errorf("Harness package %s leaks Application semantics through %s", top, value)
				continue
			}
			if !strings.HasSuffix(value, "/application/contract") && !strings.HasSuffix(value, "/application/ports") {
				t.Errorf("bridge layer %s imports Application implementation package %s", top, value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
