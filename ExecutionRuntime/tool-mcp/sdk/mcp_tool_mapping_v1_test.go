package sdk

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/registry"
)

type sdkMappingSnapshotReaderV1 struct {
	value toolcontract.MCPCapabilitySnapshotV3
}

func (r *sdkMappingSnapshotReaderV1) InspectMCPCapabilitySnapshotV3(context.Context, toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error) {
	return toolcontract.CloneMCPCapabilitySnapshotV3(r.value), nil
}

type sdkMappingMaterialReaderV1 struct {
	value toolcontract.MCPToolDiscoveryMaterialV1
}

func (r *sdkMappingMaterialReaderV1) InspectExactMCPToolDiscoveryMaterialV1(context.Context, toolcontract.MCPToolDiscoveryMaterialRefV1) (toolcontract.MCPToolDiscoveryMaterialV1, error) {
	return r.value.Clone(), nil
}

func TestMCPToolMappingSDKRegisterInspectAndAdmit(t *testing.T) {
	f := testkit.MCPToolMappingFixture(testkit.FixedTime)
	store := registry.New()
	client, err := NewV1(store, func() time.Time { return testkit.FixedTime })
	if err != nil {
		t.Fatal(err)
	}
	capabilityRecord, err := client.RegisterCapabilityV1(context.Background(), f.Capability)
	if err != nil {
		t.Fatal(err)
	}
	toolRecord, err := client.RegisterToolV1(context.Background(), f.Tool)
	if err != nil {
		t.Fatal(err)
	}
	mappingRecord, err := client.RegisterMCPToolMappingV1(context.Background(), f.Mapping)
	if err != nil {
		t.Fatal(err)
	}
	got, gotRecord, err := client.InspectMCPToolMappingV1(context.Background(), f.Mapping.Ref)
	if err != nil || got != f.Mapping || gotRecord != mappingRecord {
		t.Fatalf("mapping=%#v record=%#v err=%v", got, gotRecord, err)
	}
	service, _ := registry.NewMCPToolMappingAdmissionServiceV1(store, &sdkMappingSnapshotReaderV1{value: f.Snapshot}, &sdkMappingMaterialReaderV1{value: f.Material}, func() time.Time { return testkit.FixedTime.Add(time.Second) })
	mappingSDK, err := NewMCPToolMappingV1(service)
	if err != nil {
		t.Fatal(err)
	}
	result, err := mappingSDK.AdmitMCPToolMappingV1(context.Background(), toolcontract.MCPToolMappingAdmissionRequestV1{
		ContractVersion: toolcontract.MCPToolMappingContractVersionV1, Mapping: f.Mapping.Ref,
		ExpectedMappingRegistryRevision:    mappingRecord.RegistryRevision,
		ExpectedCapabilityRegistryRevision: capabilityRecord.RegistryRevision,
		ExpectedToolRegistryRevision:       toolRecord.RegistryRevision,
		RequestedExpiresUnixNano:           testkit.FixedTime.Add(10 * time.Second).UnixNano(),
	})
	if err != nil || result.Tool.State != registry.StateAdmitted {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if _, err = mappingSDK.AdmitMCPToolMappingV1(nil, toolcontract.MCPToolMappingAdmissionRequestV1{}); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
	var typedNil *registry.MCPToolMappingAdmissionServiceV1
	if _, err = NewMCPToolMappingV1(typedNil); err == nil {
		t.Fatal("typed-nil Mapping Admission Port was accepted")
	}
}
