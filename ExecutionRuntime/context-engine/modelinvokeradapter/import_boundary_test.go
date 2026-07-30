package modelinvokeradapter_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestContextOwnerBindingAdapterImportBoundaryV1(t *testing.T) {
	path := filepath.Join("context_owner_binding_v1.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), `"praxis.context/model-input-material"`) {
		t.Fatal("adapter duplicated Context-owned material Kind as a literal")
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), path, source, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract": true,
		"github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports":    true,
		"github.com/Proview-China/rax/ExecutionRuntime/model-invoker":           true,
		"github.com/Proview-China/rax/ExecutionRuntime/runtime/core":            true,
	}
	for _, spec := range parsed.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(importPath, "github.com/Proview-China/rax/") && !allowed[importPath] {
			t.Fatalf("forbidden Praxis implementation import %q", importPath)
		}
	}
}
