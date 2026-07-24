package application_test

import (
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestApplicationProductionImportBoundary is an executable architecture rule.
// Future component modules may enter Application through public Ports, but
// Application production packages must never couple to Runtime owners,
// kernels, fakes, Foundation, or Harness internals.
func TestApplicationProductionImportBoundary(t *testing.T) {
	t.Parallel()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate Application module root")
	}
	moduleRoot := filepath.Dir(filepath.Dir(filename))
	command := exec.Command("go", "list", "-json", "./...")
	command.Dir = moduleRoot
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list Application production packages: %v", err)
	}

	type packageDescription struct {
		ImportPath string
		Imports    []string
	}
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	const runtimePrefix = "github.com/Proview-China/rax/ExecutionRuntime/runtime/"
	forbiddenComponentPrefixes := []string{
		"github.com/Proview-China/rax/ExecutionRuntime/harness",
		"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp",
		"github.com/Proview-China/rax/ExecutionRuntime/context-engine",
		"github.com/Proview-China/rax/ExecutionRuntime/model-invoker",
	}
	allowedComponentContracts := map[string]bool{
		"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract": true,
	}
	allowedRuntime := map[string]bool{
		runtimePrefix + "core":  true,
		runtimePrefix + "ports": true,
	}
	for {
		var description packageDescription
		if err := decoder.Decode(&description); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("decode go list output: %v", err)
		}
		// Tests may use Runtime public test owners. The production boundary is
		// enforced for every other package, including contract/conformance/fakes.
		if strings.HasSuffix(description.ImportPath, "/tests") {
			continue
		}
		for _, imported := range description.Imports {
			for _, prefix := range forbiddenComponentPrefixes {
				if strings.HasPrefix(imported, prefix) && !allowedComponentContracts[imported] {
					t.Errorf("production package %s imports component implementation package %s", description.ImportPath, imported)
				}
			}
			if strings.HasPrefix(imported, runtimePrefix) && !allowedRuntime[imported] {
				t.Errorf("production package %s imports non-public Runtime package %s", description.ImportPath, imported)
			}
		}
	}
}
