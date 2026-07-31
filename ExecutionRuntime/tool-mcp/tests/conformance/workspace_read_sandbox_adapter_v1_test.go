package conformance_test

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	sandboxcontract "github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/corepack"
)

type workspaceReadAdapterMethodSetV1 interface {
	StartOrInspectWorkspaceReadV1(context.Context, corepack.WorkspaceReadSandboxRequestV1, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3) (toolcontract.WorkspaceReadOutputV1, error)
	InspectWorkspaceReadRecoveryV1(context.Context, corepack.WorkspaceReadSandboxRequestV1, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3) (toolcontract.WorkspaceReadOutputV1, error)
}

var _ workspaceReadAdapterMethodSetV1 = (*corepack.WorkspaceReadSandboxAdapterV1)(nil)
var _ sandboxports.WorkspaceReadExecutionPortV2 = (*workspaceReadExecutionPortShapeV1)(nil)
var _ corepack.WorkspaceReadRuntimeAdmissionCurrentReaderV1 = (*workspaceReadExecutionPortShapeV1)(nil)
var _ sandboxports.WorkspaceReadCommandExactReaderV1 = (*workspaceReadExecutionPortShapeV1)(nil)

type workspaceReadExecutionPortShapeV1 struct{}

func (*workspaceReadExecutionPortShapeV1) ExecuteControlledOperationPhysicalV3(context.Context, runtimeports.ControlledOperationPhysicalExecutionAuthorizationV3) (runtimeports.ControlledOperationProviderAdmissionReceiptRefV2, error) {
	panic("shape only")
}
func (*workspaceReadExecutionPortShapeV1) InspectWorkspaceReadAttemptForAdmissionV1(context.Context, runtimeports.ControlledOperationProviderAdmissionReceiptRefV2) (sandboxports.WorkspaceReadAdmissionAttemptBindingV1, error) {
	panic("shape only")
}
func (*workspaceReadExecutionPortShapeV1) InspectBoundedWorkspaceReadV1(context.Context, sandboxcontract.WorkspaceReadAttemptRefV1) (sandboxcontract.WorkspaceReadExecutionProjectionV1, error) {
	panic("shape only")
}
func (*workspaceReadExecutionPortShapeV1) InspectBoundedWorkspaceReadV2(context.Context, sandboxcontract.WorkspaceReadAttemptRefV1) (sandboxports.WorkspaceReadInspectionEnvelopeV2, error) {
	panic("shape only")
}
func (*workspaceReadExecutionPortShapeV1) InspectWorkspaceReadAdmissionForRuntimeAttemptV1(context.Context, runtimeports.OperationDispatchAttemptRefV3) (corepack.WorkspaceReadRuntimeAdmissionInspectionV1, error) {
	panic("shape only")
}
func (*workspaceReadExecutionPortShapeV1) InspectWorkspaceReadCommandExactV1(context.Context, sandboxcontract.Ref) (sandboxcontract.WorkspaceReadCommandV1, error) {
	panic("shape only")
}

func TestWorkspaceReadSandboxAdapterV1ProductionImportBoundary(t *testing.T) {
	path := filepath.Join("..", "..", "corepack", "workspace_sandbox_adapter_v1.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{
		"os": true, "os/exec": true, "io/fs": true, "path/filepath": true,
		"net": true, "net/http": true, "syscall": true, "database/sql": true,
	}
	const sandboxPrefix = "github.com/Proview-China/rax/ExecutionRuntime/sandbox"
	for _, imported := range file.Imports {
		value, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if forbidden[value] {
			t.Fatalf("workspace.read Tool adapter imports physical implementation package %q", value)
		}
		if strings.HasPrefix(value, sandboxPrefix) &&
			value != sandboxPrefix+"/contract" &&
			value != sandboxPrefix+"/ports" {
			t.Fatalf("workspace.read Tool adapter imports Sandbox implementation package %q", value)
		}
	}
}

func TestWorkspaceReadSandboxAdapterV1RealRustOpenat2PreadGate(t *testing.T) {
	sandboxModule := filepath.Clean(filepath.Join("..", "..", "..", "sandbox"))
	if _, err := os.Stat(filepath.Join(sandboxModule, "go.mod")); err != nil {
		t.Fatalf("Sandbox module unavailable: %v", err)
	}
	dataplane := filepath.Join(sandboxModule, "dataplane")
	build := exec.Command("cargo", "build", "--bin", "praxis-sandbox-dataplane")
	build.Dir = dataplane
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("real Rust dataplane build failed: %v\n%s", err, output)
	}
	binary, err := filepath.Abs(filepath.Join(dataplane, "target", "debug", "praxis-sandbox-dataplane"))
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(binary); err != nil || info.IsDir() {
		t.Fatalf("real Rust dataplane binary unavailable at %q: %v", binary, err)
	}
	command := exec.Command(
		"go", "test", "-v", "../dataplaneadapter",
		"-run", "^(TestWorkspaceReadPublicExecutor(CallsConcreteAdapterAndRustOnce|ClassifiesPhysicalBoundariesThroughRustIPC)|TestWorkspaceReadCurrentV1(RejectsCallerSubstitutedAuthorizationProof|FailsClosedOnEveryOwnerAxis)|TestWorkspaceReadCurrentV2RejectsQuerySplices)$",
		"-count=1",
	)
	// This is the adjacent Sandbox actual-point gate. It proves the public
	// executor reaches the concrete Rust openat2/pread implementation; it does
	// not claim a production Tool-to-Sandbox composition root exists.
	command.Dir = dataplane
	command.Env = append(os.Environ(), "PRAXIS_SANDBOX_DATAPLANE_BIN="+binary)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("real Rust openat2/pread Sandbox gate failed: %v\n%s", err, output)
	}
	result := string(output)
	if strings.Contains(result, "--- SKIP:") ||
		!strings.Contains(result, "--- PASS: TestWorkspaceReadPublicExecutorCallsConcreteAdapterAndRustOnce") ||
		!strings.Contains(result, "--- PASS: TestWorkspaceReadPublicExecutorClassifiesPhysicalBoundariesThroughRustIPC") ||
		!strings.Contains(result, "--- PASS: TestWorkspaceReadCurrentV1RejectsCallerSubstitutedAuthorizationProof") ||
		!strings.Contains(result, "--- PASS: TestWorkspaceReadCurrentV1FailsClosedOnEveryOwnerAxis") ||
		!strings.Contains(result, "--- PASS: TestWorkspaceReadCurrentV2RejectsQuerySplices") {
		t.Fatalf("real Rust openat2/pread Sandbox gate skipped or missed its physical assertions:\n%s", output)
	}
}
