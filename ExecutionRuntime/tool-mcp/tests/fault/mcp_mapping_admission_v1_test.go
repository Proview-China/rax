package fault_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

type faultMappingSnapshotReaderV1 struct {
	value toolcontract.MCPCapabilitySnapshotV3
	calls atomic.Uint64
}

func (r *faultMappingSnapshotReaderV1) InspectMCPCapabilitySnapshotV3(context.Context, toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error) {
	if r.calls.Add(1) > 1 {
		return toolcontract.MCPCapabilitySnapshotV3{}, core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "second Snapshot read unavailable")
	}
	return toolcontract.CloneMCPCapabilitySnapshotV3(r.value), nil
}

type faultMappingMaterialReaderV1 struct {
	value toolcontract.MCPToolDiscoveryMaterialV1
}

func (r *faultMappingMaterialReaderV1) InspectExactMCPToolDiscoveryMaterialV1(context.Context, toolcontract.MCPToolDiscoveryMaterialRefV1) (toolcontract.MCPToolDiscoveryMaterialV1, error) {
	return r.value.Clone(), nil
}

func TestFaultMCPMappingSecondReadUnavailableLeavesRegistryUnchanged(t *testing.T) {
	f := testkit.MCPToolMappingFixture(testkit.FixedTime)
	store := registry.New()
	capabilityRecord, _ := store.SubmitCapability(f.Capability, testkit.FixedTime)
	toolRecord, _ := store.SubmitTool(f.Tool, testkit.FixedTime)
	mappingRecord, _ := store.SubmitMCPToolMapping(f.Mapping, testkit.FixedTime)
	service, _ := registry.NewMCPToolMappingAdmissionServiceV1(store, &faultMappingSnapshotReaderV1{value: f.Snapshot}, &faultMappingMaterialReaderV1{value: f.Material}, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	request := toolcontract.MCPToolMappingAdmissionRequestV1{
		ContractVersion: toolcontract.MCPToolMappingContractVersionV1, Mapping: f.Mapping.Ref,
		ExpectedMappingRegistryRevision:    mappingRecord.RegistryRevision,
		ExpectedCapabilityRegistryRevision: capabilityRecord.RegistryRevision,
		ExpectedToolRegistryRevision:       toolRecord.RegistryRevision,
		RequestedExpiresUnixNano:           testkit.FixedTime.Add(10 * time.Second).UnixNano(),
	}
	if _, err := service.AdmitMCPToolMappingV1(context.Background(), request); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("second-read error=%v", err)
	}
	_, capabilityAfter, _ := store.ResolveCapability(string(f.Capability.ID))
	_, toolAfter, _ := store.ResolveTool(string(f.Tool.ID))
	_, mappingAfter, _ := store.InspectMCPToolMapping(f.Mapping.Ref)
	if capabilityAfter != capabilityRecord || toolAfter != toolRecord || mappingAfter != mappingRecord {
		t.Fatalf("Registry changed after unavailable S2: cap=%#v tool=%#v mapping=%#v", capabilityAfter, toolAfter, mappingAfter)
	}
}
