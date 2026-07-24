package sdk_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func registerSecondAliasToolV1(t *testing.T, fixture sdkFixtureV1) toolcontract.ToolDescriptor {
	t.Helper()
	tool := fixture.tool
	tool.ID = "tool/example-alias-second"
	tool.ArtifactDigest = testkit.Digest("alias-second-artifact")
	tool.Digest = ""
	var err error
	tool, err = toolcontract.SealTool(tool)
	if err != nil {
		t.Fatal(err)
	}
	record, err := fixture.client.RegisterToolV1(context.Background(), tool)
	if err != nil {
		t.Fatal(err)
	}
	record, err = fixture.registry.Transition("tool", string(tool.ID), record.RegistryRevision, registry.StateAdmitted, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.registry.Transition("tool", string(tool.ID), record.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	return tool
}

func TestToolAliasSDKV1RegisterResolveAndFreezeSurface(t *testing.T) {
	fixture := activeSDKFixtureV1(t)
	first := testkit.ToolAliasV1(1, toolcontract.ObjectRef{ID: string(fixture.tool.ID), Revision: fixture.tool.Revision, Digest: fixture.tool.Digest}, testkit.FixedTime)
	firstRecord, err := fixture.client.RegisterToolAliasV1(context.Background(), first, nil)
	if err != nil || firstRecord.State != registry.StateActive {
		t.Fatalf("Tool Alias register=%+v err=%v", firstRecord, err)
	}
	snapshot, err := fixture.client.InspectRegistrySnapshotV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshotRef := sdk.RegistrySnapshotRefV1{Revision: snapshot.Revision, Digest: snapshot.Digest}
	resolution, err := fixture.client.ResolveToolAliasForAssemblyV1(context.Background(), testkit.Owner(), testkit.ToolAliasNameV1(), snapshotRef)
	if err != nil || resolution.Validate() != nil || resolution.Alias.Ref != first.Ref || resolution.Tool.Digest != fixture.tool.Digest {
		t.Fatalf("Tool Alias resolution=%+v err=%v", resolution, err)
	}
	request := compileRequestV1(t, fixture)
	request.RegistrySnapshot = snapshotRef
	request.Selections[0].Tool = toolcontract.ObjectRef{ID: string(resolution.Tool.ID), Revision: resolution.Tool.Revision, Digest: resolution.Tool.Digest}
	oldSurface, err := fixture.client.CompileToolSurfaceV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	secondTool := registerSecondAliasToolV1(t, fixture)
	second := testkit.ToolAliasV1(2, toolcontract.ObjectRef{ID: string(secondTool.ID), Revision: secondTool.Revision, Digest: secondTool.Digest}, testkit.FixedTime)
	expected := first.Ref
	if _, err = fixture.client.RegisterToolAliasV1(context.Background(), second, &expected); err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.client.ResolveToolAliasForAssemblyV1(context.Background(), testkit.Owner(), testkit.ToolAliasNameV1(), snapshotRef); err == nil {
		t.Fatal("Tool Alias resolved through a stale pre-repoint Registry Snapshot")
	}
	if oldSurface.Entries[0].Tool.ID != string(fixture.tool.ID) || oldSurface.Entries[0].Tool.Digest != fixture.tool.Digest {
		t.Fatal("Alias repoint mutated an already compiled Tool Surface")
	}
	historical, _, err := fixture.client.InspectToolAliasV1(context.Background(), first.Ref)
	if err != nil || historical.Ref != first.Ref {
		t.Fatalf("Tool Alias historical Inspect=%+v err=%v", historical, err)
	}
}

func TestToolAliasSDKV1RejectsInactiveTargetAndCanceledContext(t *testing.T) {
	store := registry.New()
	client, err := sdk.NewV1(store, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	capability := testkit.Capability()
	if _, err = client.RegisterCapabilityV1(context.Background(), capability); err != nil {
		t.Fatal(err)
	}
	tool := testkit.Tool()
	if _, err = client.RegisterToolV1(context.Background(), tool); err != nil {
		t.Fatal(err)
	}
	alias := testkit.ToolAliasV1(1, toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}, testkit.FixedTime)
	if _, err = client.RegisterToolAliasV1(context.Background(), alias, nil); err == nil {
		t.Fatal("Tool Alias targeted a submitted Tool")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = client.RegisterToolAliasV1(ctx, alias, nil); err != context.Canceled {
		t.Fatalf("canceled Tool Alias register error=%v", err)
	}
}

func TestToolAliasSDKV1ConcurrentAssemblyReads(t *testing.T) {
	fixture := activeSDKFixtureV1(t)
	alias := testkit.ToolAliasV1(1, toolcontract.ObjectRef{ID: string(fixture.tool.ID), Revision: fixture.tool.Revision, Digest: fixture.tool.Digest}, testkit.FixedTime)
	if _, err := fixture.client.RegisterToolAliasV1(context.Background(), alias, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, err := fixture.client.InspectRegistrySnapshotV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	exact := sdk.RegistrySnapshotRefV1{Revision: snapshot.Revision, Digest: snapshot.Digest}
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := fixture.client.ResolveToolAliasForAssemblyV1(context.Background(), testkit.Owner(), testkit.ToolAliasNameV1(), exact)
			if err == nil && (value.Alias.Ref != alias.Ref || value.Tool.Digest != fixture.tool.Digest) {
				err = value.Validate()
				if err == nil {
					err = fmt.Errorf("Tool Alias concurrent resolution drifted")
				}
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}
