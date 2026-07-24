package registry_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

type mappingSnapshotReaderV1 struct {
	value toolcontract.MCPCapabilitySnapshotV3
	err   error
}

func (r *mappingSnapshotReaderV1) InspectMCPCapabilitySnapshotV3(context.Context, toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error) {
	return toolcontract.CloneMCPCapabilitySnapshotV3(r.value), r.err
}

type mappingMaterialReaderV1 struct {
	value toolcontract.MCPToolDiscoveryMaterialV1
	err   error
}

func (r *mappingMaterialReaderV1) InspectExactMCPToolDiscoveryMaterialV1(context.Context, toolcontract.MCPToolDiscoveryMaterialRefV1) (toolcontract.MCPToolDiscoveryMaterialV1, error) {
	return r.value.Clone(), r.err
}

func TestMCPToolMappingAdmissionAtomicAndLostReply(t *testing.T) {
	f, store, request := setupMCPToolMappingAdmissionV1(t)
	if _, err := store.Transition("tool", string(f.Tool.ID), request.ExpectedToolRegistryRevision, registry.StateAdmitted, testkit.FixedTime); !core.HasReason(err, core.ReasonInvalidTransition) {
		t.Fatalf("generic MCP Tool Admission error=%v", err)
	}
	if _, err := store.Transition("mcp-tool-mapping", f.Mapping.Ref.ID, request.ExpectedMappingRegistryRevision, registry.StateAdmitted, testkit.FixedTime); !core.HasReason(err, core.ReasonInvalidTransition) {
		t.Fatalf("generic Mapping Admission error=%v", err)
	}
	service, err := registry.NewMCPToolMappingAdmissionServiceV1(store, &mappingSnapshotReaderV1{value: f.Snapshot}, &mappingMaterialReaderV1{value: f.Material}, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	winner, err := service.AdmitMCPToolMappingV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if winner.Mapping.State != registry.StateAdmitted || winner.Capability.State != registry.StateAdmitted || winner.Tool.State != registry.StateAdmitted || winner.Mapping.RegistryRevision != winner.Capability.RegistryRevision || winner.Mapping.RegistryRevision != winner.Tool.RegistryRevision {
		t.Fatalf("Admission was not one atomic Registry revision: %#v", winner)
	}
	retry, err := service.AdmitMCPToolMappingV1(context.Background(), request)
	if err != nil || retry != winner {
		t.Fatalf("lost-reply retry=%#v err=%v", retry, err)
	}
}

func TestMCPToolMappingAdmissionConcurrentSingleWinner(t *testing.T) {
	f, store, request := setupMCPToolMappingAdmissionV1(t)
	service, _ := registry.NewMCPToolMappingAdmissionServiceV1(store, &mappingSnapshotReaderV1{value: f.Snapshot}, &mappingMaterialReaderV1{value: f.Material}, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	const workers = 64
	var group sync.WaitGroup
	results := make(chan registry.MCPToolMappingAdmissionResultV1, workers)
	errs := make(chan error, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := service.AdmitMCPToolMappingV1(context.Background(), request)
			results <- result
			errs <- err
		}()
	}
	group.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var revision core.Revision
	for result := range results {
		if revision == 0 {
			revision = result.Mapping.RegistryRevision
		} else if result.Mapping.RegistryRevision != revision || result.Capability.RegistryRevision != revision || result.Tool.RegistryRevision != revision {
			t.Fatalf("concurrent Admission returned multiple revisions: %#v", result)
		}
	}
}

func TestMCPToolMappingAdmissionFailsClosed(t *testing.T) {
	t.Run("expired_snapshot", func(t *testing.T) {
		f, store, request := setupMCPToolMappingAdmissionV1(t)
		service, _ := registry.NewMCPToolMappingAdmissionServiceV1(store, &mappingSnapshotReaderV1{value: f.Snapshot}, &mappingMaterialReaderV1{value: f.Material}, func() time.Time { return time.Unix(0, f.Snapshot.ExpiresUnixNano) })
		if _, err := service.AdmitMCPToolMappingV1(context.Background(), request); err == nil {
			t.Fatal("expired Snapshot admitted an MCP Tool Mapping")
		}
	})
	t.Run("material_unavailable", func(t *testing.T) {
		f, store, request := setupMCPToolMappingAdmissionV1(t)
		unavailable := core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "material unavailable")
		service, _ := registry.NewMCPToolMappingAdmissionServiceV1(store, &mappingSnapshotReaderV1{value: f.Snapshot}, &mappingMaterialReaderV1{err: unavailable}, func() time.Time { return testkit.FixedTime.Add(time.Second) })
		if _, err := service.AdmitMCPToolMappingV1(context.Background(), request); !core.HasCategory(err, core.ErrorUnavailable) {
			t.Fatalf("unavailable material error=%v", err)
		}
	})
	t.Run("nil_and_canceled_context", func(t *testing.T) {
		f, store, request := setupMCPToolMappingAdmissionV1(t)
		service, _ := registry.NewMCPToolMappingAdmissionServiceV1(store, &mappingSnapshotReaderV1{value: f.Snapshot}, &mappingMaterialReaderV1{value: f.Material}, func() time.Time { return testkit.FixedTime.Add(time.Second) })
		if _, err := service.AdmitMCPToolMappingV1(nil, request); !core.HasCategory(err, core.ErrorInvalidArgument) {
			t.Fatalf("nil context error=%v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := service.AdmitMCPToolMappingV1(ctx, request); err != context.Canceled {
			t.Fatalf("canceled context error=%v", err)
		}
	})
	t.Run("typed_nil_dependency", func(t *testing.T) {
		var snapshots *mappingSnapshotReaderV1
		if _, err := registry.NewMCPToolMappingAdmissionServiceV1(registry.New(), snapshots, &mappingMaterialReaderV1{}, time.Now); err == nil {
			t.Fatal("typed-nil Snapshot reader was accepted")
		}
	})
}

func TestMCPToolMappingRegistrySameIDChangedPolicyConflicts(t *testing.T) {
	f, store, _ := setupMCPToolMappingAdmissionV1(t)
	drift := f.Mapping
	drift.MappingPolicyDigest = testkit.Digest("changed-policy")
	drift.Ref = toolcontract.MCPToolMappingManifestRefV1{}
	var err error
	drift, err = toolcontract.SealMCPToolMappingManifestV1(drift)
	if err != nil {
		t.Fatal(err)
	}
	if drift.Ref.ID != f.Mapping.Ref.ID || drift.Ref.Digest == f.Mapping.Ref.Digest {
		t.Fatal("policy drift fixture did not preserve stable ID with a new digest")
	}
	if _, err = store.SubmitMCPToolMapping(drift, testkit.FixedTime.Add(time.Second)); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same ID changed policy error=%v", err)
	}
}

func setupMCPToolMappingAdmissionV1(t *testing.T) (testkit.MCPToolMappingFixtureV1, *registry.Registry, toolcontract.MCPToolMappingAdmissionRequestV1) {
	t.Helper()
	f := testkit.MCPToolMappingFixture(testkit.FixedTime)
	store := registry.New()
	capabilityRecord, err := store.SubmitCapability(f.Capability, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	toolRecord, err := store.SubmitTool(f.Tool, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	mappingRecord, err := store.SubmitMCPToolMapping(f.Mapping, testkit.FixedTime)
	if err != nil {
		t.Fatal(err)
	}
	request := toolcontract.MCPToolMappingAdmissionRequestV1{
		ContractVersion: toolcontract.MCPToolMappingContractVersionV1, Mapping: f.Mapping.Ref,
		ExpectedMappingRegistryRevision:    mappingRecord.RegistryRevision,
		ExpectedCapabilityRegistryRevision: capabilityRecord.RegistryRevision,
		ExpectedToolRegistryRevision:       toolRecord.RegistryRevision,
		RequestedExpiresUnixNano:           testkit.FixedTime.Add(10 * time.Second).UnixNano(),
	}
	return f, store, request
}
