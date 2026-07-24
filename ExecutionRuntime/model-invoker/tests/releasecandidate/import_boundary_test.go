package releasecandidate_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestProductionImportBoundaryV1(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filename))), "releasecandidate")
	forbidden := []string{"/internal", "/fakes", "/testkit", "/foundation", "/provider/", "/routegateway"}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range file.Imports {
			value, _ := strconv.Unquote(item.Path.Value)
			for _, blocked := range forbidden {
				if strings.Contains(value, blocked) {
					t.Errorf("production candidate imports forbidden package %s", value)
				}
			}
		}
	}
}
