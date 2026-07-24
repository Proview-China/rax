package api_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolapi "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestCatalogInspectRegistryObjectV1ClosedTypedUnion(t *testing.T) {
	fixture := populatedCatalogV1(t)
	tests := []struct {
		kind  string
		exact toolcontract.ObjectRef
		check func(toolapi.RegistryObjectProjectionV1) bool
	}{
		{"capability", objectRefCatalogInspectV1(testkit.Capability()), func(v toolapi.RegistryObjectProjectionV1) bool {
			return v.Capability != nil && v.Tool == nil && v.Package == nil && v.ToolAlias == nil
		}},
		{"tool", objectRefCatalogInspectV1(testkit.Tool()), func(v toolapi.RegistryObjectProjectionV1) bool {
			return v.Capability == nil && v.Tool != nil && v.Package == nil && v.ToolAlias == nil
		}},
		{"package", objectRefCatalogInspectV1(testkit.Package()), func(v toolapi.RegistryObjectProjectionV1) bool {
			return v.Capability == nil && v.Tool == nil && v.Package != nil && v.ToolAlias == nil
		}},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			projection, err := fixture.catalog.InspectRegistryObjectV1(context.Background(), toolapi.InspectRegistryObjectRequestV1{Kind: test.kind, Exact: test.exact})
			if err != nil || projection.Validate() != nil || projection.Record.ID != test.exact.ID || !test.check(projection) {
				t.Fatalf("typed exact Inspect failed: projection=%#v err=%v", projection, err)
			}
		})
	}
}

func TestCatalogInspectRegistryObjectV1ToolAliasAndDeepCopy(t *testing.T) {
	store := registry.New()
	capability := testkit.Capability()
	capabilityRecord, err := store.SubmitCapability(capability, testkit.FixedTime)
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
	tool := testkit.Tool()
	toolRecord, err := store.SubmitTool(tool, testkit.FixedTime)
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
	alias := testkit.ToolAliasV1(1, objectRefCatalogInspectV1(tool), testkit.FixedTime)
	if _, err = store.SubmitToolAlias(alias, nil, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	client, err := sdk.NewV1(store, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := toolapi.NewCatalogV1(client)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := catalog.InspectRegistryObjectV1(context.Background(), toolapi.InspectRegistryObjectRequestV1{Kind: "tool-alias", Exact: toolcontract.ObjectRef{ID: alias.Ref.ID, Revision: alias.Ref.Revision, Digest: alias.Ref.Digest}})
	if err != nil || projection.ToolAlias == nil || projection.ToolAlias.Ref != alias.Ref {
		t.Fatalf("Tool Alias exact Inspect failed: projection=%#v err=%v", projection, err)
	}

	toolProjection, err := catalog.InspectRegistryObjectV1(context.Background(), toolapi.InspectRegistryObjectRequestV1{Kind: "tool", Exact: objectRefCatalogInspectV1(tool)})
	if err != nil {
		t.Fatal(err)
	}
	toolProjection.Tool.EffectKinds[0] = "praxis.test/tampered"
	again, err := catalog.InspectRegistryObjectV1(context.Background(), toolapi.InspectRegistryObjectRequestV1{Kind: "tool", Exact: objectRefCatalogInspectV1(tool)})
	if err != nil || again.Tool.EffectKinds[0] == "praxis.test/tampered" {
		t.Fatalf("Catalog Inspect leaked mutable Tool slices: projection=%#v err=%v", again, err)
	}
}

func TestCatalogInspectRegistryObjectV1FailClosed(t *testing.T) {
	fixture := populatedCatalogV1(t)
	capability := testkit.Capability()
	exact := objectRefCatalogInspectV1(capability)
	if _, err := fixture.catalog.InspectRegistryObjectV1(nil, toolapi.InspectRegistryObjectRequestV1{Kind: "capability", Exact: exact}); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.catalog.InspectRegistryObjectV1(ctx, toolapi.InspectRegistryObjectRequestV1{Kind: "capability", Exact: exact}); err != context.Canceled {
		t.Fatalf("canceled context error=%v", err)
	}
	wrongDigest := exact
	wrongDigest.Digest = testkit.Digest("wrong")
	if _, err := fixture.catalog.InspectRegistryObjectV1(context.Background(), toolapi.InspectRegistryObjectRequestV1{Kind: "capability", Exact: wrongDigest}); err == nil {
		t.Fatal("wrong exact digest was accepted")
	}
	if _, err := fixture.catalog.InspectRegistryObjectV1(context.Background(), toolapi.InspectRegistryObjectRequestV1{Kind: "tool", Exact: exact}); err == nil {
		t.Fatal("cross-kind type pun was accepted")
	}
	projection, err := fixture.catalog.InspectRegistryObjectV1(context.Background(), toolapi.InspectRegistryObjectRequestV1{Kind: "capability", Exact: exact})
	if err != nil {
		t.Fatal(err)
	}
	projection.Record.ObjectDigest = testkit.Digest("tampered")
	if projection.Validate() == nil {
		t.Fatal("tampered Registry projection was accepted")
	}
}

func TestCatalogInspectRegistryObjectV1RejectsS1S2Drift(t *testing.T) {
	store := registry.New()
	capability := testkit.Capability()
	record, err := store.SubmitCapability(capability, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	client, err := sdk.NewV1(store, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	drifting := &driftingCatalogInspectV1{SDKV1: client, store: store, expected: record.RegistryRevision}
	catalog, err := toolapi.NewCatalogV1(drifting)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = catalog.InspectRegistryObjectV1(context.Background(), toolapi.InspectRegistryObjectRequestV1{Kind: "capability", Exact: objectRefCatalogInspectV1(capability)}); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("S1/S2 Registry drift error=%v", err)
	}
}

func TestCatalogInspectRegistryObjectV1ConcurrentDeterministic(t *testing.T) {
	fixture := populatedCatalogV1(t)
	request := toolapi.InspectRegistryObjectRequestV1{Kind: "tool", Exact: objectRefCatalogInspectV1(testkit.Tool())}
	const workers = 64
	var wg sync.WaitGroup
	results := make(chan toolapi.RegistryObjectProjectionV1, workers)
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			projection, err := fixture.catalog.InspectRegistryObjectV1(context.Background(), request)
			results <- projection
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var digest core.Digest
	for result := range results {
		if digest == "" {
			digest = result.ProjectionDigest
		} else if result.ProjectionDigest != digest {
			t.Fatal("concurrent exact Registry projections diverged")
		}
	}
}

type driftingCatalogInspectV1 struct {
	*sdk.SDKV1
	store    *registry.Registry
	expected core.Revision
	once     sync.Once
}

func (d *driftingCatalogInspectV1) InspectCapabilityV1(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.CapabilityDescriptor, registry.Record, error) {
	value, record, err := d.SDKV1.InspectCapabilityV1(ctx, exact)
	if err != nil {
		return value, record, err
	}
	d.once.Do(func() {
		_, _ = d.store.Transition("capability", exact.ID, d.expected, registry.StateAdmitted, testkit.FixedTime)
	})
	return value, record, nil
}

func objectRefCatalogInspectV1(value any) toolcontract.ObjectRef {
	switch object := value.(type) {
	case toolcontract.CapabilityDescriptor:
		return toolcontract.ObjectRef{ID: string(object.ID), Revision: object.Revision, Digest: object.Digest}
	case toolcontract.ToolDescriptor:
		return toolcontract.ObjectRef{ID: string(object.ID), Revision: object.Revision, Digest: object.Digest}
	case toolcontract.ToolPackageManifest:
		return toolcontract.ObjectRef{ID: string(object.ID), Revision: object.Revision, Digest: object.Digest}
	default:
		panic("unsupported catalog test object")
	}
}
