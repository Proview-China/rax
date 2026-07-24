package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/cli"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/mcp"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

type cliFixtureV1 struct {
	runner    *cli.RunnerV1
	catalog   *api.CatalogV1
	inspector *sdk.SDKV1
	registry  *registry.Registry
	toolID    string
	digest    core.Digest
}

func newCLIFixtureV1(t *testing.T) cliFixtureV1 {
	t.Helper()
	store := registry.New()
	if _, err := store.SubmitCapability(testkit.Capability(), testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	tool := testkit.Tool()
	if _, err := store.SubmitTool(tool, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SubmitPackage(testkit.Package(), testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	client, err := sdk.NewV1(store, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := api.NewCatalogV1(client)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := cli.NewRunnerV1(catalog, client)
	if err != nil {
		t.Fatal(err)
	}
	return cliFixtureV1{runner: runner, catalog: catalog, inspector: client, registry: store, toolID: string(tool.ID), digest: tool.Digest}
}

func TestRunnerV1ToolAliasListAndExactInspect(t *testing.T) {
	fixture := newCLIFixtureV1(t)
	capability := testkit.Capability()
	_, capRecord, ok := fixture.registry.ResolveCapability(string(capability.ID))
	if !ok {
		t.Fatal("Capability fixture is absent")
	}
	capRecord, err := fixture.registry.Transition("capability", string(capability.ID), capRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.registry.Transition("capability", string(capability.ID), capRecord.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	tool := testkit.Tool()
	_, toolRecord, ok := fixture.registry.ResolveTool(string(tool.ID))
	if !ok {
		t.Fatal("Tool fixture is absent")
	}
	toolRecord, err = fixture.registry.Transition("tool", string(tool.ID), toolRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.registry.Transition("tool", string(tool.ID), toolRecord.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	alias := testkit.ToolAliasV1(1, contract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}, testkit.FixedTime)
	if _, err = fixture.inspector.RegisterToolAliasV1(context.Background(), alias, nil); err != nil {
		t.Fatal(err)
	}
	var listed bytes.Buffer
	if err := fixture.runner.RunV1(context.Background(), []string{"tool", "list", "--kind=tool-alias", "--page-size=10"}, &listed); err != nil {
		t.Fatal(err)
	}
	var page cli.ListOutputV1
	if err := json.Unmarshal(listed.Bytes(), &page); err != nil || len(page.Records) != 1 || page.Records[0].ID != alias.Ref.ID {
		t.Fatalf("Tool Alias list=%+v err=%v", page, err)
	}
	var inspected bytes.Buffer
	args := []string{"tool", "inspect", "--kind=alias", "--id=" + alias.Ref.ID, "--revision=1", "--digest=" + string(alias.Ref.Digest)}
	if err := fixture.runner.RunV1(context.Background(), args, &inspected); err != nil {
		t.Fatal(err)
	}
	var result cli.InspectOutputV1
	if err := json.Unmarshal(inspected.Bytes(), &result); err != nil || result.Kind != "alias" || result.Record.ID != alias.Ref.ID {
		t.Fatalf("Tool Alias inspect=%+v err=%v", result, err)
	}
}

func TestRunnerV1MCPStatusIsExactAndReadOnly(t *testing.T) {
	fixture := newCLIFixtureV1(t)
	manager := mcp.NewManager()
	connection := testkit.MCPConnection()
	if _, err := manager.Register(connection, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	status, err := sdk.NewMCPStatusV1(manager, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	runner, err := cli.NewRunnerWithMCPV1(fixture.catalog, fixture.inspector, status)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"mcp", "status", "--id=" + connection.ID, "--revision=1", "--digest=" + string(connection.Digest)}
	var output bytes.Buffer
	if err := runner.RunV1(context.Background(), args, &output); err != nil {
		t.Fatal(err)
	}
	var result cli.MCPStatusOutputV1
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.ContractVersion != cli.ContractVersionV1 || result.Record.Connection.Digest != connection.Digest || result.Record.State != mcp.ConnectionRegistered {
		t.Fatalf("mcp status output is invalid: %v %+v", err, result)
	}
	output.Reset()
	if err := runner.RunV1(context.Background(), []string{"mcp", "discover"}, &output); err == nil || output.Len() != 0 {
		t.Fatalf("mcp discover bypassed governed Effect boundary: %v %q", err, output.String())
	}
}

func TestRunnerV1ToolListAndInspect(t *testing.T) {
	fixture := newCLIFixtureV1(t)
	var listed bytes.Buffer
	if err := fixture.runner.RunV1(context.Background(), []string{"tool", "list", "--page-size=1"}, &listed); err != nil {
		t.Fatal(err)
	}
	var list cli.ListOutputV1
	if err := json.Unmarshal(listed.Bytes(), &list); err != nil || list.ContractVersion != cli.ContractVersionV1 || len(list.Records) != 3 {
		t.Fatalf("tool list output is invalid: %v %+v", err, list)
	}

	var inspected bytes.Buffer
	args := []string{"tool", "inspect", "--kind=tool", "--id=" + fixture.toolID, "--revision=1", "--digest=" + string(fixture.digest)}
	if err := fixture.runner.RunV1(context.Background(), args, &inspected); err != nil {
		t.Fatal(err)
	}
	var result struct {
		ContractVersion string          `json:"contract_version"`
		Kind            string          `json:"kind"`
		Record          registry.Record `json:"record"`
		Object          map[string]any  `json:"object"`
	}
	if err := json.Unmarshal(inspected.Bytes(), &result); err != nil || result.ContractVersion != cli.ContractVersionV1 || result.Kind != "tool" || result.Record.ID != fixture.toolID || result.Object["id"] != fixture.toolID {
		t.Fatalf("tool inspect output is invalid: %v %+v", err, result)
	}
}

func TestRunnerV1UnsupportedAndInvalidCommandsAreZeroOutput(t *testing.T) {
	fixture := newCLIFixtureV1(t)
	for _, args := range [][]string{
		{"tool", "call"},
		{"mcp", "discover"},
		{"tool", "list", "--page-size=0"},
		{"tool", "inspect", "--kind=tool", "--id=x", "--revision=1", "--digest=bad"},
	} {
		var output bytes.Buffer
		if err := fixture.runner.RunV1(context.Background(), args, &output); err == nil {
			t.Fatalf("unsupported or invalid command succeeded: %v", args)
		}
		if output.Len() != 0 {
			t.Fatalf("rejected command wrote output: %v %q", args, output.String())
		}
	}
}

func TestRunnerV1RejectsTypedNilNilAndCanceledContext(t *testing.T) {
	var typedNilCatalog *api.CatalogV1
	var typedNilSDK *sdk.SDKV1
	if _, err := cli.NewRunnerV1(typedNilCatalog, typedNilSDK); err == nil {
		t.Fatal("typed-nil CLI dependencies were accepted")
	}
	fixture := newCLIFixtureV1(t)
	var output bytes.Buffer
	if err := fixture.runner.RunV1(nil, []string{"tool", "list"}, &output); err == nil {
		t.Fatal("nil context was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := fixture.runner.RunV1(ctx, []string{"tool", "list"}, &output); err != context.Canceled {
		t.Fatalf("canceled context was not preserved: %v", err)
	}
	var typedNilOutput *bytes.Buffer
	if err := fixture.runner.RunV1(context.Background(), []string{"tool", "list"}, typedNilOutput); err == nil {
		t.Fatal("typed-nil output Writer was accepted")
	}
}

type driftingCatalogV1 struct {
	mu    sync.Mutex
	calls int
}

func (c *driftingCatalogV1) ListRegistryV1(context.Context, api.ListRegistryRequestV1) (api.ListRegistryResultV1, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	snapshot := sdk.RegistrySnapshotRefV1{Revision: core.Revision(c.calls), Digest: testkit.Digest("snapshot-" + strconv.Itoa(c.calls))}
	record := api.RegistryRecordV1{Kind: "tool", ID: "tool/example", ObjectRevision: 1, ObjectDigest: testkit.Digest("tool"), State: registry.StateActive, RegistryRevision: 1, UpdatedUnixNano: testkit.FixedTime.UnixNano()}
	result := api.ListRegistryResultV1{ContractVersion: api.CatalogContractVersionV1, Snapshot: snapshot, Records: []api.RegistryRecordV1{record}}
	if c.calls == 1 {
		next := api.RegistryPageCursorV1{ContractVersion: api.CatalogContractVersionV1, Snapshot: snapshot, AfterKind: record.Kind, AfterID: record.ID}
		next.Digest, _ = next.ComputeDigest()
		result.Next = &next
	}
	return result, nil
}

func TestRunnerV1RejectsSnapshotDriftBeforeWritingOutput(t *testing.T) {
	fixture := newCLIFixtureV1(t)
	drifting := &driftingCatalogV1{}
	runner, err := cli.NewRunnerV1(drifting, fixture.runnerInspectorV1(t))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := runner.RunV1(context.Background(), []string{"tool", "list", "--page-size=1"}, &output); err == nil {
		t.Fatal("CLI accepted Registry Snapshot drift")
	}
	if output.Len() != 0 {
		t.Fatal("CLI wrote partial output before detecting Snapshot drift")
	}
}

func (f cliFixtureV1) runnerInspectorV1(t *testing.T) *sdk.SDKV1 {
	t.Helper()
	store := registry.New()
	client, err := sdk.NewV1(store, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestRunnerV1ConcurrentReadCommandsAreDeterministic(t *testing.T) {
	fixture := newCLIFixtureV1(t)
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	outputs := make(chan string, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var output bytes.Buffer
			err := fixture.runner.RunV1(context.Background(), []string{"tool", "list", "--page-size=2"}, &output)
			errs <- err
			outputs <- output.String()
		}()
	}
	wg.Wait()
	close(errs)
	close(outputs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first string
	for output := range outputs {
		if first == "" {
			first = output
		} else if first != output {
			t.Fatal("concurrent CLI list output diverged")
		}
	}
}
