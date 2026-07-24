package sdk_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

type sdkFixtureV1 struct {
	client     *sdk.SDKV1
	registry   *registry.Registry
	capability contract.CapabilityDescriptor
	tool       contract.ToolDescriptor
}

func activeSDKFixtureV1(t *testing.T) sdkFixtureV1 {
	t.Helper()
	store := registry.New()
	client, err := sdk.NewV1(store, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	capability, tool := testkit.Capability(), testkit.Tool()
	capabilityRecord, err := client.RegisterCapabilityV1(context.Background(), capability)
	if err != nil {
		t.Fatal(err)
	}
	capabilityRecord, err = store.Transition("capability", string(capability.ID), capabilityRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Transition("capability", string(capability.ID), capabilityRecord.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	toolRecord, err := client.RegisterToolV1(context.Background(), tool)
	if err != nil {
		t.Fatal(err)
	}
	toolRecord, err = store.Transition("tool", string(tool.ID), toolRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Transition("tool", string(tool.ID), toolRecord.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	return sdkFixtureV1{client: client, registry: store, capability: capability, tool: tool}
}

func compileRequestV1(t *testing.T, fixture sdkFixtureV1) sdk.CompileSurfaceRequestV1 {
	t.Helper()
	snapshot, err := fixture.client.InspectRegistrySnapshotV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return sdk.CompileSurfaceRequestV1{
		Owner: testkit.Owner(), ResolvedPlanDigest: testkit.Digest("plan"), ProfileDigest: testkit.Digest("profile"),
		CapabilityGrantDigest: testkit.Digest("grant"), RegistrySnapshot: sdk.RegistrySnapshotRefV1{Revision: snapshot.Revision, Digest: snapshot.Digest},
		Dialect: "tool/test-dialect", Revision: 1, RequestedExpiresUnixNano: testkit.FixedTime.Add(time.Minute).UnixNano(),
		Selections: []sdk.SurfaceSelectionV1{{
			Capability: contract.ObjectRef{ID: string(fixture.capability.ID), Revision: fixture.capability.Revision, Digest: fixture.capability.Digest},
			Tool:       contract.ObjectRef{ID: string(fixture.tool.ID), Revision: fixture.tool.Revision, Digest: fixture.tool.Digest},
			ModelName:  "example", DescriptionDigest: testkit.Digest("description"), Visible: true, Allowed: true,
		}},
	}
}

func TestSDKV1RegistersInspectsAndCompilesExactSurface(t *testing.T) {
	fixture := activeSDKFixtureV1(t)
	snapshot, err := fixture.client.InspectRegistrySnapshotV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshotRef := sdk.RegistrySnapshotRefV1{Revision: snapshot.Revision, Digest: snapshot.Digest}
	if resolved, record, err := fixture.client.ResolveCapabilityForAssemblyV1(context.Background(), fixture.capability.ID, snapshotRef); err != nil || resolved.Digest != fixture.capability.Digest || record.State != registry.StateActive {
		t.Fatalf("Capability assembly resolution failed: %v %+v %+v", err, resolved, record)
	}
	if resolved, record, err := fixture.client.ResolveToolForAssemblyV1(context.Background(), fixture.tool.ID, snapshotRef); err != nil || resolved.Digest != fixture.tool.Digest || record.State != registry.StateActive {
		t.Fatalf("Tool assembly resolution failed: %v %+v %+v", err, resolved, record)
	}
	capabilityRef := contract.ObjectRef{ID: string(fixture.capability.ID), Revision: fixture.capability.Revision, Digest: fixture.capability.Digest}
	if _, record, err := fixture.client.InspectCapabilityV1(context.Background(), capabilityRef); err != nil || record.State != registry.StateActive {
		t.Fatalf("exact Capability inspect failed: %v %+v", err, record)
	}
	toolRef := contract.ObjectRef{ID: string(fixture.tool.ID), Revision: fixture.tool.Revision, Digest: fixture.tool.Digest}
	if _, record, err := fixture.client.InspectToolV1(context.Background(), toolRef); err != nil || record.State != registry.StateActive {
		t.Fatalf("exact Tool inspect failed: %v %+v", err, record)
	}
	surface, err := fixture.client.CompileToolSurfaceV1(context.Background(), compileRequestV1(t, fixture))
	if err != nil {
		t.Fatal(err)
	}
	if err := surface.Validate(); err != nil || len(surface.Entries) != 1 || surface.Entries[0].Tool != toolRef || surface.Entries[0].Capability != capabilityRef {
		t.Fatalf("compiled Surface lost exact Registry objects: %v %+v", err, surface)
	}
}

func TestSDKV1PackageRegisterAndExactInspect(t *testing.T) {
	fixture := activeSDKFixtureV1(t)
	manifest := testkit.Package()
	record, err := fixture.client.RegisterPackageV1(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	exact := contract.ObjectRef{ID: string(manifest.ID), Revision: manifest.Revision, Digest: manifest.Digest}
	got, inspected, err := fixture.client.InspectPackageV1(context.Background(), exact)
	if err != nil || got.Digest != manifest.Digest || inspected != record {
		t.Fatalf("exact Package inspect failed: %v %+v %+v", err, got, inspected)
	}
	got.Descriptors[0].Digest = testkit.Digest("mutated")
	again, _, err := fixture.client.InspectPackageV1(context.Background(), exact)
	if err != nil || again.Descriptors[0].Digest != manifest.Descriptors[0].Digest {
		t.Fatal("Package inspect did not deep-clone Registry state")
	}
}

func TestSDKV1FailsClosedOnExactAndCurrentDrift(t *testing.T) {
	fixture := activeSDKFixtureV1(t)
	wrong := contract.ObjectRef{ID: string(fixture.tool.ID), Revision: fixture.tool.Revision, Digest: testkit.Digest("wrong-tool")}
	if _, _, err := fixture.client.InspectToolV1(context.Background(), wrong); err == nil {
		t.Fatal("exact Tool digest drift was accepted")
	}

	request := compileRequestV1(t, fixture)
	if _, err := fixture.client.RegisterPackageV1(context.Background(), testkit.Package()); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.client.CompileToolSurfaceV1(context.Background(), request); err == nil {
		t.Fatal("stale Registry Snapshot compiled a Surface")
	}
}

func TestSDKV1RejectsNilCanceledAndFutureFacts(t *testing.T) {
	var typedNil *registry.Registry
	if _, err := sdk.NewV1(typedNil, func() time.Time { return testkit.FixedTime }); err == nil {
		t.Fatal("typed-nil Registry Port was accepted")
	}
	store := registry.New()
	client, err := sdk.NewV1(store, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.RegisterCapabilityV1(nil, testkit.Capability()); err == nil {
		t.Fatal("nil context was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.RegisterCapabilityV1(ctx, testkit.Capability()); err != context.Canceled {
		t.Fatalf("canceled context was not preserved: %v", err)
	}
	capability := testkit.Capability()
	capability.CreatedUnixNano = testkit.FixedTime.Add(time.Second).UnixNano()
	capability.Digest = ""
	capability, err = contract.SealCapability(capability)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.RegisterCapabilityV1(context.Background(), capability); err == nil {
		t.Fatal("future Capability was registered")
	}
	snapshot, err := client.InspectRegistrySnapshotV1(context.Background())
	if err != nil || snapshot.Revision != 0 || len(snapshot.Records) != 0 {
		t.Fatalf("rejected SDK requests changed Registry state: %v %+v", err, snapshot)
	}
}

func TestSDKV1ConcurrentSameRegistrationIsIdempotent(t *testing.T) {
	store := registry.New()
	client, err := sdk.NewV1(store, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	records := make(chan registry.Record, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			record, err := client.RegisterCapabilityV1(context.Background(), testkit.Capability())
			records <- record
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	close(records)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var revision uint64
	for record := range records {
		if revision == 0 {
			revision = uint64(record.RegistryRevision)
		} else if revision != uint64(record.RegistryRevision) {
			t.Fatal("same registration produced multiple Registry revisions")
		}
	}
	snapshot, err := client.InspectRegistrySnapshotV1(context.Background())
	if err != nil || snapshot.Revision != 1 || len(snapshot.Records) != 1 {
		t.Fatalf("concurrent create-once did not produce one Registry fact: %v %+v", err, snapshot)
	}
}

func TestSDKV1CompileRejectsTTLAndClockCrossing(t *testing.T) {
	fixture := activeSDKFixtureV1(t)
	for _, times := range [][]time.Time{
		{testkit.FixedTime, testkit.FixedTime.Add(time.Minute)},
		{testkit.FixedTime, testkit.FixedTime.Add(-time.Nanosecond)},
	} {
		var mu sync.Mutex
		index := 0
		clock := func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			if index >= len(times) {
				return times[len(times)-1]
			}
			value := times[index]
			index++
			return value
		}
		client, err := sdk.NewV1(fixture.registry, clock)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.CompileToolSurfaceV1(context.Background(), compileRequestV1(t, fixture)); err == nil {
			t.Fatal("Surface compilation crossed TTL or clock monotonicity")
		}
	}
}
