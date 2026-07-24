package api_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

type catalogFixtureV1 struct {
	catalog          *api.CatalogV1
	registry         *registry.Registry
	capabilityRecord registry.Record
}

func populatedCatalogV1(t *testing.T) catalogFixtureV1 {
	t.Helper()
	store := registry.New()
	capabilityRecord, err := store.SubmitCapability(testkit.Capability(), testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SubmitTool(testkit.Tool(), testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SubmitPackage(testkit.Package(), testkit.FixedTime); err != nil {
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
	return catalogFixtureV1{catalog: catalog, registry: store, capabilityRecord: capabilityRecord}
}

func TestCatalogV1PaginatesOneExactRegistrySnapshot(t *testing.T) {
	fixture := populatedCatalogV1(t)
	request := api.ListRegistryRequestV1{PageSize: 1}
	var records []api.RegistryRecordV1
	var snapshot sdk.RegistrySnapshotRefV1
	for {
		page, err := fixture.catalog.ListRegistryV1(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Digest == "" {
			snapshot = page.Snapshot
		} else if page.Snapshot != snapshot {
			t.Fatal("pagination changed Registry Snapshot")
		}
		records = append(records, page.Records...)
		if page.Next == nil {
			break
		}
		if err := page.Next.Validate(); err != nil {
			t.Fatal(err)
		}
		request.Cursor = page.Next
	}
	if len(records) != 3 || records[0].Kind != "capability" || records[1].Kind != "package" || records[2].Kind != "tool" {
		t.Fatalf("Catalog pagination is not stable: %+v", records)
	}
}

func TestCatalogV1FiltersAndSupportsEmptyRegistry(t *testing.T) {
	fixture := populatedCatalogV1(t)
	page, err := fixture.catalog.ListRegistryV1(context.Background(), api.ListRegistryRequestV1{PageSize: 10, KindFilter: "tool"})
	if err != nil || len(page.Records) != 1 || page.Records[0].Kind != "tool" || page.Next != nil {
		t.Fatalf("Catalog filter failed: %v %+v", err, page)
	}

	emptyStore := registry.New()
	client, _ := sdk.NewV1(emptyStore, func() time.Time { return testkit.FixedTime })
	emptyCatalog, _ := api.NewCatalogV1(client)
	empty, err := emptyCatalog.ListRegistryV1(context.Background(), api.ListRegistryRequestV1{PageSize: 10})
	if err != nil || len(empty.Records) != 0 || empty.Snapshot.Revision != 0 || empty.Next != nil {
		t.Fatalf("empty Registry page failed: %v %+v", err, empty)
	}
}

func TestCatalogV1RejectsCursorAndSnapshotDrift(t *testing.T) {
	fixture := populatedCatalogV1(t)
	first, err := fixture.catalog.ListRegistryV1(context.Background(), api.ListRegistryRequestV1{PageSize: 1})
	if err != nil || first.Next == nil {
		t.Fatalf("first page failed: %v %+v", err, first)
	}
	tampered := *first.Next
	tampered.AfterID = "changed"
	if _, err := fixture.catalog.ListRegistryV1(context.Background(), api.ListRegistryRequestV1{PageSize: 1, Cursor: &tampered}); err == nil {
		t.Fatal("tampered page cursor was accepted")
	}

	if _, err = fixture.registry.Transition("capability", fixture.capabilityRecord.ID, fixture.capabilityRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.catalog.ListRegistryV1(context.Background(), api.ListRegistryRequestV1{PageSize: 1, Cursor: first.Next}); err == nil {
		t.Fatal("stale Registry cursor was accepted after mutation")
	}
}

func TestCatalogV1RejectsTypedNilNilContextAndCanceledContext(t *testing.T) {
	var typedNil *sdk.SDKV1
	if _, err := api.NewCatalogV1(typedNil); err == nil {
		t.Fatal("typed-nil Catalog SDK Port was accepted")
	}
	fixture := populatedCatalogV1(t)
	request := api.ListRegistryRequestV1{PageSize: 1}
	if _, err := fixture.catalog.ListRegistryV1(nil, request); err == nil {
		t.Fatal("nil context was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.catalog.ListRegistryV1(ctx, request); err != context.Canceled {
		t.Fatalf("canceled context was not preserved: %v", err)
	}
}

func TestCatalogV1ConcurrentReadIsDeterministic(t *testing.T) {
	fixture := populatedCatalogV1(t)
	const workers = 64
	var wg sync.WaitGroup
	results := make(chan api.ListRegistryResultV1, workers)
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := fixture.catalog.ListRegistryV1(context.Background(), api.ListRegistryRequestV1{PageSize: 10})
			results <- result
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
	var snapshot sdk.RegistrySnapshotRefV1
	for result := range results {
		if snapshot.Digest == "" {
			snapshot = result.Snapshot
		} else if snapshot != result.Snapshot || len(result.Records) != 3 {
			t.Fatal("concurrent Catalog reads diverged")
		}
	}
}
