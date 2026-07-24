package sandbox_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestProductionSurfaceKeepsSideEffectsInsideApprovedDataPlaneAdapter(t *testing.T) {
	t.Parallel()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	root := filepath.Dir(filepath.Dir(currentFile))
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	remainingGoMod := string(goMod)
	for _, approved := range []string{
		"github.com/Proview-China/rax/ExecutionRuntime/agent-assembler v0.0.0",
		"github.com/Proview-China/rax/ExecutionRuntime/application v0.0.0",
		"github.com/Proview-China/rax/ExecutionRuntime/harness v0.0.0",
		"github.com/Proview-China/rax/ExecutionRuntime/runtime v0.0.0",
		"github.com/Proview-China/rax/ExecutionRuntime/agent-definition v0.0.0 // indirect",
		// These are graph-only transitive requirements of the approved local
		// Agent Assembler release repository. The AST import scan below still
		// forbids Sandbox production packages from importing them directly.
		"github.com/dustin/go-humanize v1.0.1 // indirect",
		"github.com/google/uuid v1.6.0 // indirect",
		"github.com/mattn/go-isatty v0.0.20 // indirect",
		"github.com/ncruces/go-strftime v1.0.0 // indirect",
		"github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect",
		"golang.org/x/sys v0.44.0",
		"modernc.org/libc v1.73.4 // indirect",
		"modernc.org/mathutil v1.7.1 // indirect",
		"modernc.org/memory v1.11.0 // indirect",
		"modernc.org/sqlite v1.53.0",
	} {
		line := "\t" + approved
		if strings.Count(remainingGoMod, line) != 1 {
			t.Fatalf("sandbox go.mod requirement %q is missing or duplicated:\n%s", approved, goMod)
		}
		remainingGoMod = strings.Replace(remainingGoMod, line, "", 1)
	}
	if strings.Contains(remainingGoMod, "\n\t") {
		t.Fatalf("wave 1 sandbox go.mod gained external requirements:\n%s", goMod)
	}

	forbiddenStandardLibrary := map[string]bool{
		"database/sql": true,
		"net":          true,
		"net/http":     true,
		"os":           true,
		"os/exec":      true,
		"plugin":       true,
		"syscall":      true,
	}
	const modulePrefix = "github.com/Proview-China/rax/ExecutionRuntime/sandbox"
	const assemblerPrefix = "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/"
	const harnessPrefix = "github.com/Proview-China/rax/ExecutionRuntime/harness/"
	const runtimePrefix = "github.com/Proview-China/rax/ExecutionRuntime/runtime/"
	const applicationPrefix = "github.com/Proview-China/rax/ExecutionRuntime/application/"
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if relative == "tests" || relative == filepath.Join("internal", "testkit") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		dataPlaneAdapter := strings.HasPrefix(relative, "dataplaneadapter"+string(filepath.Separator))
		applicationAdapter := strings.HasPrefix(relative, "applicationadapter"+string(filepath.Separator))
		apiTransport := strings.HasPrefix(relative, "api"+string(filepath.Separator))
		apiHandler := strings.HasPrefix(relative, "apihandler"+string(filepath.Separator))
		cliTransport := strings.HasPrefix(relative, filepath.Join("cmd", "praxis-sandbox")+string(filepath.Separator))
		governedSDK := strings.HasPrefix(relative, "sdk"+string(filepath.Separator))
		releaseAdapter := strings.HasPrefix(relative, "release"+string(filepath.Separator))
		hostRoot := strings.HasPrefix(relative, "hostroot"+string(filepath.Separator))
		statePlaneAdapter := strings.HasPrefix(relative, filepath.Join("storage", "sqlite")+string(filepath.Separator))
		workspaceDriver := strings.HasPrefix(relative, "workspacefs"+string(filepath.Separator))
		if relative, _ := filepath.Rel(root, path); strings.HasPrefix(relative, "runtimeadapter"+string(filepath.Separator)) {
			payload, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, forbidden := range []string{"OperationDispatchEnforcementPhaseReceiptV4", "EnforceCurrentOperationDispatchV4", "ExecutePrepared("} {
				if strings.Contains(string(payload), forbidden) {
					t.Errorf("read-only Sandbox adapter %s contains forbidden execution/receipt symbol %q", path, forbidden)
				}
			}
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			approvedTransportImport := (apiTransport && importPath == "net/http") ||
				(cliTransport && (importPath == "net" || importPath == "net/http" || importPath == "os")) ||
				(hostRoot && (importPath == "net" || importPath == "net/http"))
			if forbiddenStandardLibrary[importPath] && !dataPlaneAdapter && !statePlaneAdapter && !(workspaceDriver && importPath == "os") && !approvedTransportImport {
				t.Errorf("production file %s imports forbidden side-effect package %q", path, importPath)
			}
			if strings.HasPrefix(importPath, runtimePrefix) {
				approved := (strings.HasPrefix(relative, "runtimeadapter"+string(filepath.Separator)) || dataPlaneAdapter || applicationAdapter || releaseAdapter || statePlaneAdapter) && (importPath == runtimePrefix+"core" || importPath == runtimePrefix+"ports")
				approved = approved || ((governedSDK || apiHandler) && importPath == runtimePrefix+"ports")
				if !approved {
					t.Errorf("production file %s imports unapproved Runtime surface %q", path, importPath)
				}
				continue
			}
			if strings.HasPrefix(importPath, applicationPrefix) {
				approved := ((applicationAdapter || statePlaneAdapter || governedSDK || apiHandler || hostRoot) && (importPath == applicationPrefix+"contract" || importPath == applicationPrefix+"ports")) || (releaseAdapter && importPath == applicationPrefix+"contract")
				if !approved {
					t.Errorf("production file %s imports unapproved Application surface %q", path, importPath)
				}
				continue
			}
			if strings.HasPrefix(importPath, assemblerPrefix) {
				if !releaseAdapter || importPath != assemblerPrefix+"contract" {
					t.Errorf("production file %s imports unapproved Agent Assembler surface %q", path, importPath)
				}
				continue
			}
			if strings.HasPrefix(importPath, harnessPrefix) {
				if !releaseAdapter || importPath != harnessPrefix+"assemblycontract" {
					t.Errorf("production file %s imports unapproved Harness surface %q", path, importPath)
				}
				continue
			}
			if strings.Contains(importPath, ".") && !strings.HasPrefix(importPath, modulePrefix) && !(statePlaneAdapter && importPath == "modernc.org/sqlite") {
				t.Errorf("production file %s imports external/cross-component package %q", path, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
