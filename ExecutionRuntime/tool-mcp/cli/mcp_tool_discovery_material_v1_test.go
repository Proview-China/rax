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

func TestCLIMCPToolDiscoveryMaterialV1ExactInspect(t *testing.T) {
	material := testkit.MCPToolDiscoveryMaterialV1()
	fixture := cliMCPToolMaterialFixtureV1{material: material, set: testkit.MCPDiscoveryPageToolMaterialSetV1()}
	runner := newMCPToolMaterialCLIRunnerV1(t, fixture)
	var output bytes.Buffer
	err := runner.RunV1(context.Background(), []string{"mcp", "inspect", "--kind", "tool-material", "--id", material.Ref.ID, "--revision", strconv.FormatUint(uint64(material.Ref.Revision), 10), "--digest", string(material.Ref.Digest)}, &output)
	if err != nil {
		t.Fatal(err)
	}
	var value cli.MCPConnectInspectOutputV1
	if err = json.Unmarshal(output.Bytes(), &value); err != nil || value.Kind != "tool-material" || !bytes.Contains(output.Bytes(), []byte(`"name":"echo"`)) || !bytes.Contains(output.Bytes(), []byte(`"inputSchema"`)) {
		t.Fatalf("tool material output=%s err=%v", output.String(), err)
	}
}

func TestCLIMCPDiscoveryPageToolMaterialSetV1ExactInspect(t *testing.T) {
	fixture := cliMCPToolMaterialFixtureV1{material: testkit.MCPToolDiscoveryMaterialV1(), set: testkit.MCPDiscoveryPageToolMaterialSetV1()}
	runner := newMCPToolMaterialCLIRunnerV1(t, fixture)
	var output bytes.Buffer
	err := runner.RunV1(context.Background(), []string{"mcp", "inspect", "--kind", "tool-material-set", "--id", fixture.set.Receipt.ID, "--revision", strconv.FormatUint(uint64(fixture.set.Receipt.Revision), 10), "--digest", string(fixture.set.Receipt.Digest)}, &output)
	if err != nil || !bytes.Contains(output.Bytes(), []byte(fixture.material.Ref.ID)) || !bytes.Contains(output.Bytes(), []byte(`"kind":"tool-material-set"`)) {
		t.Fatalf("tool material set output=%s err=%v", output.String(), err)
	}
}

func TestCLIMCPResourcePromptDiscoveryMaterialV1ExactInspect(t *testing.T) {
	fixture := cliMCPToolMaterialFixtureV1{
		material: testkit.MCPToolDiscoveryMaterialV1(), set: testkit.MCPDiscoveryPageToolMaterialSetV1(),
		resource: testkit.MCPResourceDiscoveryMaterialV1(), resourceSet: testkit.MCPDiscoveryPageResourceMaterialSetV1(),
		prompt: testkit.MCPPromptDiscoveryMaterialV1(), promptSet: testkit.MCPDiscoveryPagePromptMaterialSetV1(),
	}
	runner := newMCPToolMaterialCLIRunnerV1(t, fixture)
	for _, item := range []struct {
		kind string
		id   string
		rev  core.Revision
		dig  core.Digest
		want string
	}{
		{"resource-material", fixture.resource.Ref.ID, fixture.resource.Ref.Revision, fixture.resource.Ref.Digest, `"uri":"file:///workspace/readme.md"`},
		{"prompt-material", fixture.prompt.Ref.ID, fixture.prompt.Ref.Revision, fixture.prompt.Ref.Digest, `"name":"review"`},
		{"resource-material-set", fixture.resourceSet.Receipt.ID, fixture.resourceSet.Receipt.Revision, fixture.resourceSet.Receipt.Digest, `"uri":"file:///workspace/readme.md"`},
		{"prompt-material-set", fixture.promptSet.Receipt.ID, fixture.promptSet.Receipt.Revision, fixture.promptSet.Receipt.Digest, `"name":"review"`},
	} {
		var output bytes.Buffer
		err := runner.RunV1(context.Background(), []string{"mcp", "inspect", "--kind", item.kind, "--id", item.id, "--revision", strconv.FormatUint(uint64(item.rev), 10), "--digest", string(item.dig)}, &output)
		if err != nil || !bytes.Contains(output.Bytes(), []byte(item.want)) || !bytes.Contains(output.Bytes(), []byte(`"kind":"`+item.kind+`"`)) {
			t.Fatalf("%s output=%s err=%v", item.kind, output.String(), err)
		}
	}
}

func TestCLIMCPToolDiscoveryMaterialV1FailsClosed(t *testing.T) {
	material := testkit.MCPToolDiscoveryMaterialV1()
	var typedNil *cliMCPToolMaterialFixtureV1
	base := newCLIFixtureV1(t)
	manager := mcp.NewManager()
	legacy := testkit.MCPConnection()
	_, _ = manager.Register(legacy, testkit.FixedTime)
	status, _ := sdk.NewMCPStatusV1(manager, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	if _, err := cli.NewRunnerWithMCPToolDiscoveryMaterialV1(base.catalog, base.inspector, status, newCLIConnectReadV1(t), cliMCPDiscoveryFixtureV2{snapshot: testkit.MCPCapabilitySnapshotV2(testkit.FixedTime)}, typedNil); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil constructor error=%v", err)
	}
	fixture := cliMCPToolMaterialFixtureV1{material: material, set: testkit.MCPDiscoveryPageToolMaterialSetV1()}
	runner := newMCPToolMaterialCLIRunnerV1(t, fixture)
	var output bytes.Buffer
	err := runner.RunV1(context.Background(), []string{"mcp", "inspect", "--kind", "tool-material", "--id", material.Ref.ID, "--revision", "1", "--digest", string(testkit.Digest("wrong-tool-material"))}, &output)
	if err == nil || output.Len() != 0 {
		t.Fatalf("wrong exact Ref output=%s err=%v", output.String(), err)
	}
}

func newMCPToolMaterialCLIRunnerV1(t *testing.T, material cliMCPToolMaterialFixtureV1) *cli.RunnerV1 {
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
	runner, err := cli.NewRunnerWithMCPDiscoveryMaterialReadV1(base.catalog, base.inspector, status, newCLIConnectReadV1(t), cliMCPDiscoveryFixtureV2{snapshot: testkit.MCPCapabilitySnapshotV2(testkit.FixedTime)}, material, material, material, material, material, material)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

type cliMCPToolMaterialFixtureV1 struct {
	material    toolcontract.MCPToolDiscoveryMaterialV1
	set         toolcontract.MCPDiscoveryPageToolMaterialSetV1
	resource    toolcontract.MCPResourceDiscoveryMaterialV1
	prompt      toolcontract.MCPPromptDiscoveryMaterialV1
	resourceSet toolcontract.MCPDiscoveryPageResourceMaterialSetV1
	promptSet   toolcontract.MCPDiscoveryPagePromptMaterialSetV1
}

func (f cliMCPToolMaterialFixtureV1) InspectExactMCPResourceDiscoveryMaterialV1(_ context.Context, exact toolcontract.MCPResourceDiscoveryMaterialRefV1) (toolcontract.MCPResourceDiscoveryMaterialV1, error) {
	if exact != f.resource.Ref {
		return toolcontract.MCPResourceDiscoveryMaterialV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Resource Discovery Material not found")
	}
	return f.resource.Clone(), nil
}

func (f cliMCPToolMaterialFixtureV1) InspectExactMCPPromptDiscoveryMaterialV1(_ context.Context, exact toolcontract.MCPPromptDiscoveryMaterialRefV1) (toolcontract.MCPPromptDiscoveryMaterialV1, error) {
	if exact != f.prompt.Ref {
		return toolcontract.MCPPromptDiscoveryMaterialV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Prompt Discovery Material not found")
	}
	return f.prompt.Clone(), nil
}

func (f cliMCPToolMaterialFixtureV1) InspectMCPDiscoveryPageResourceMaterialSetV1(_ context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageResourceMaterialSetV1, error) {
	if exact != f.resourceSet.Receipt {
		return toolcontract.MCPDiscoveryPageResourceMaterialSetV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Discovery Page Resource Material Set not found")
	}
	return toolcontract.CloneMCPDiscoveryPageResourceMaterialSetV1(f.resourceSet), nil
}

func (f cliMCPToolMaterialFixtureV1) InspectMCPDiscoveryPagePromptMaterialSetV1(_ context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPagePromptMaterialSetV1, error) {
	if exact != f.promptSet.Receipt {
		return toolcontract.MCPDiscoveryPagePromptMaterialSetV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Discovery Page Prompt Material Set not found")
	}
	return toolcontract.CloneMCPDiscoveryPagePromptMaterialSetV1(f.promptSet), nil
}

func (f cliMCPToolMaterialFixtureV1) InspectMCPDiscoveryPageToolMaterialSetV1(_ context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPDiscoveryPageToolMaterialSetV1, error) {
	if exact != f.set.Receipt {
		return toolcontract.MCPDiscoveryPageToolMaterialSetV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Discovery Page Tool Material Set not found")
	}
	return toolcontract.CloneMCPDiscoveryPageToolMaterialSetV1(f.set), nil
}

func (f cliMCPToolMaterialFixtureV1) InspectExactMCPToolDiscoveryMaterialV1(_ context.Context, exact toolcontract.MCPToolDiscoveryMaterialRefV1) (toolcontract.MCPToolDiscoveryMaterialV1, error) {
	if exact != f.material.Ref {
		return toolcontract.MCPToolDiscoveryMaterialV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "MCP Tool Discovery Material not found")
	}
	return f.material.Clone(), nil
}
