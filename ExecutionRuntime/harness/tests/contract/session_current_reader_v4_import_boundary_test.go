package contract_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSessionCurrentReaderV4PortImportBoundary(t *testing.T) {
	t.Parallel()

	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate Session Current Reader V4 import test")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(current), "..", "..", "ports", "governed_v4.go"))
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imported := range file.Imports {
		value := strings.Trim(imported.Path.Value, "\"")
		if value == "context" || value == "github.com/Proview-China/rax/ExecutionRuntime/harness/contract" {
			continue
		}
		t.Fatalf("Session V4 public port imports forbidden package %s", value)
	}
}
