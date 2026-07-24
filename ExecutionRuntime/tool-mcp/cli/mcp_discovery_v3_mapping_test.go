package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/cli"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

type cliMCPDiscoveryFixtureV3 struct {
	snapshot toolcontract.MCPCapabilitySnapshotV3
}

func (f cliMCPDiscoveryFixtureV3) InspectCurrentMCPCapabilitySnapshotV3(context.Context, toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error) {
	return toolcontract.CloneMCPCapabilitySnapshotV3(f.snapshot), nil
}

func TestRunnerV1SnapshotV3AndMappingExactRead(t *testing.T) {
	f := testkit.MCPToolMappingFixture(testkit.FixedTime)
	store := registry.New()
	if _, err := store.SubmitCapability(f.Capability, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitTool(f.Tool, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitMCPToolMapping(f.Mapping, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	client, err := sdk.NewV1(store, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := api.NewCatalogV1(client)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := cli.NewRunnerWithMCPDiscoveryV3(catalog, client, cliMCPDiscoveryFixtureV3{snapshot: f.Snapshot})
	if err != nil {
		t.Fatal(err)
	}
	var snapshotOutput bytes.Buffer
	args := []string{"mcp", "snapshot-v3", "--id=" + f.Snapshot.ID, "--revision=1", "--digest=" + string(f.Snapshot.Digest)}
	if err := runner.RunV1(context.Background(), args, &snapshotOutput); err != nil {
		t.Fatal(err)
	}
	var snapshotResult cli.MCPCapabilitySnapshotOutputV3
	if err := json.Unmarshal(snapshotOutput.Bytes(), &snapshotResult); err != nil || snapshotResult.Snapshot.ObjectRef() != f.Snapshot.ObjectRef() || len(snapshotResult.Snapshot.ToolMaterials) != 1 {
		t.Fatalf("Snapshot V3 output=%#v err=%v", snapshotResult, err)
	}
	var mappingOutput bytes.Buffer
	args = []string{"tool", "inspect", "--kind=mcp-mapping", "--id=" + f.Mapping.Ref.ID, "--revision=1", "--digest=" + string(f.Mapping.Ref.Digest)}
	if err := runner.RunV1(context.Background(), args, &mappingOutput); err != nil {
		t.Fatal(err)
	}
	var mappingResult cli.InspectOutputV1
	if err := json.Unmarshal(mappingOutput.Bytes(), &mappingResult); err != nil || mappingResult.Kind != "mcp-mapping" || mappingResult.Record.ID != f.Mapping.Ref.ID {
		t.Fatalf("Mapping output=%#v err=%v", mappingResult, err)
	}
	var forbidden bytes.Buffer
	if err := runner.RunV1(context.Background(), []string{"mcp", "discover"}, &forbidden); !core.HasCategory(err, core.ErrorCapabilityUnavailable) || forbidden.Len() != 0 {
		t.Fatalf("write command error=%v output=%q", err, forbidden.String())
	}
}
