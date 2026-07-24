package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestCLIMCPCapabilitySnapshotV2ExactRead(t *testing.T) {
	snapshot := testkit.MCPCapabilitySnapshotV2(testkit.FixedTime)
	runner := newMCPDiscoveryCLIRunnerV2(t, cliMCPDiscoveryFixtureV2{snapshot: snapshot})
	var output bytes.Buffer
	err := runner.RunV1(context.Background(), []string{"mcp", "snapshot", "--id", snapshot.ID, "--revision", strconv.FormatUint(uint64(snapshot.Revision), 10), "--digest", string(snapshot.Digest)}, &output)
	if err != nil {
		t.Fatal(err)
	}
	var value cli.MCPCapabilitySnapshotOutputV2
	if err = json.Unmarshal(output.Bytes(), &value); err != nil || value.Snapshot.ObjectRef() != snapshot.ObjectRef() || value.Snapshot.Tools[0].Name != "echo" {
		t.Fatalf("output=%s err=%v", output.String(), err)
	}
}

func TestCLIMCPCapabilitySnapshotV2FailsClosed(t *testing.T) {
	snapshot := testkit.MCPCapabilitySnapshotV2(testkit.FixedTime)
	var typedNil *cliMCPDiscoveryFixtureV2
	fixture := newCLIFixtureV1(t)
	manager := mcp.NewManager()
	legacy := testkit.MCPConnection()
	_, _ = manager.Register(legacy, testkit.FixedTime)
	status, _ := sdk.NewMCPStatusV1(manager, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	if _, err := cli.NewRunnerWithMCPDiscoveryV2(fixture.catalog, fixture.inspector, status, newCLIConnectReadV1(t), typedNil); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil=%v", err)
	}
	runner := newMCPDiscoveryCLIRunnerV2(t, cliMCPDiscoveryFixtureV2{snapshot: snapshot})
	if err := runner.RunV1(context.Background(), []string{"mcp", "snapshot", "--id", snapshot.ID, "--revision", "1", "--digest", string(testkit.Digest("wrong"))}, &bytes.Buffer{}); err == nil {
		t.Fatal("wrong exact Snapshot digest was accepted")
	}
}

func newMCPDiscoveryCLIRunnerV2(t *testing.T, discovery cli.MCPDiscoveryReadPortV2) *cli.RunnerV1 {
	t.Helper()
	fixture := newCLIFixtureV1(t)
	manager := mcp.NewManager()
	legacy := testkit.MCPConnection()
	if _, err := manager.Register(legacy, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	status, err := sdk.NewMCPStatusV1(manager, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	runner, err := cli.NewRunnerWithMCPDiscoveryV2(fixture.catalog, fixture.inspector, status, newCLIConnectReadV1(t), discovery)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

type cliMCPDiscoveryFixtureV2 struct {
	snapshot toolcontract.MCPCapabilitySnapshotV2
}

func (f cliMCPDiscoveryFixtureV2) InspectCurrentMCPCapabilitySnapshotV2(_ context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV2, error) {
	if exact != f.snapshot.ObjectRef() {
		return toolcontract.MCPCapabilitySnapshotV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Snapshot not found")
	}
	return toolcontract.CloneMCPCapabilitySnapshotV2(f.snapshot), nil
}
