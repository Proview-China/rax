package cli_test

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/cli"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestCLIMCPCallV1ExactInspect(t *testing.T) {
	fixture := testkit.MCPExecutionV1(testkit.FixedTime)
	call := cliMCPCallFixtureV1{command: fixture.Command, receipt: testkit.MCPProtocolReceiptV1(fixture, fixture.Now.Add(time.Second))}
	runner := newMCPCallCLIRunnerV1(t, call)
	for _, tc := range []struct {
		kind   string
		id     string
		rev    uint64
		digest string
	}{
		{"call-command", call.command.Ref.ID, uint64(call.command.Ref.Revision), string(call.command.Ref.Digest)},
		{"call-receipt", call.receipt.Ref.ID, uint64(call.receipt.Ref.Revision), string(call.receipt.Ref.Digest)},
	} {
		var output bytes.Buffer
		err := runner.RunV1(context.Background(), []string{"mcp", "inspect", "--kind", tc.kind, "--id", tc.id, "--revision", strconv.FormatUint(tc.rev, 10), "--digest", tc.digest}, &output)
		if err != nil || !bytes.Contains(output.Bytes(), []byte(tc.id)) || !bytes.Contains(output.Bytes(), []byte(tc.kind)) {
			t.Fatalf("kind=%s output=%s err=%v", tc.kind, output.String(), err)
		}
		if bytes.Contains(output.Bytes(), []byte(`"inline"`)) || bytes.Contains(output.Bytes(), []byte(`"canonical_response"`)) || bytes.Contains(output.Bytes(), []byte(`"text":"ok"`)) {
			t.Fatalf("kind=%s exposed raw MCP call content: %s", tc.kind, output.String())
		}
	}
}

func TestCLIMCPCallV1FailsClosed(t *testing.T) {
	fixture := testkit.MCPExecutionV1(testkit.FixedTime)
	call := cliMCPCallFixtureV1{command: fixture.Command, receipt: testkit.MCPProtocolReceiptV1(fixture, fixture.Now.Add(time.Second))}
	var typedNil *cliMCPCallFixtureV1
	base := newCLIFixtureV1(t)
	manager := mcp.NewManager()
	legacy := testkit.MCPConnection()
	_, _ = manager.Register(legacy, testkit.FixedTime)
	status, _ := sdk.NewMCPStatusV1(manager, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	if _, err := cli.NewRunnerWithMCPCallV1(base.catalog, base.inspector, status, newCLIConnectReadV1(t), cliMCPDiscoveryFixtureV2{snapshot: testkit.MCPCapabilitySnapshotV2(testkit.FixedTime)}, typedNil); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil=%v", err)
	}
	runner := newMCPCallCLIRunnerV1(t, call)
	var output bytes.Buffer
	err := runner.RunV1(context.Background(), []string{"mcp", "inspect", "--kind", "call-command", "--id", call.command.Ref.ID, "--revision", "1", "--digest", string(testkit.Digest("wrong"))}, &output)
	if err == nil || output.Len() != 0 {
		t.Fatalf("wrong digest output=%s err=%v", output.String(), err)
	}
}

func newMCPCallCLIRunnerV1(t *testing.T, call cli.MCPCallReadPortV1) *cli.RunnerV1 {
	t.Helper()
	base := newCLIFixtureV1(t)
	manager := mcp.NewManager()
	legacy := testkit.MCPConnection()
	if _, err := manager.Register(legacy, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	status, err := sdk.NewMCPStatusV1(manager, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	runner, err := cli.NewRunnerWithMCPCallV1(base.catalog, base.inspector, status, newCLIConnectReadV1(t), cliMCPDiscoveryFixtureV2{snapshot: testkit.MCPCapabilitySnapshotV2(testkit.FixedTime)}, call)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

type cliMCPCallFixtureV1 struct {
	command toolcontract.MCPExecutionCommandFactV1
	receipt toolcontract.MCPProtocolReceiptV1
}

func (f cliMCPCallFixtureV1) InspectMCPExecutionCommandV1(_ context.Context, exact toolcontract.MCPExecutionCommandRefV1) (toolcontract.MCPExecutionCommandFactV1, error) {
	if exact != f.command.Ref {
		return toolcontract.MCPExecutionCommandFactV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "command not found")
	}
	return toolcontract.CloneMCPExecutionCommandFactV1(f.command), nil
}

func (f cliMCPCallFixtureV1) InspectMCPProtocolReceiptV1(_ context.Context, exact toolcontract.MCPProtocolReceiptRefV1) (toolcontract.MCPProtocolReceiptV1, error) {
	if exact != f.receipt.Ref {
		return toolcontract.MCPProtocolReceiptV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "receipt not found")
	}
	return toolcontract.CloneMCPProtocolReceiptV1(f.receipt), nil
}
