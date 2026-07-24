package sdk_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/sdk"
)

func TestMCPDiscoverySDKV2ExactCurrent(t *testing.T) {
	now := testkit.FixedTime
	value := testkit.MCPCapabilitySnapshotV2(now)
	repository := mcpDiscoverySDKReaderV2{value: value}
	client, err := sdk.NewMCPDiscoveryV2(repository, func() time.Time { return now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.InspectCurrentMCPCapabilitySnapshotV2(context.Background(), value.ObjectRef())
	if err != nil || got.Digest != value.Digest || got.Tools[0].Name != "echo" {
		t.Fatalf("snapshot=%#v err=%v", got, err)
	}
}

func TestMCPDiscoverySDKV2FailsClosed(t *testing.T) {
	now := testkit.FixedTime
	value := testkit.MCPCapabilitySnapshotV2(now)
	var typedNil *mcpDiscoverySDKReaderV2
	if _, err := sdk.NewMCPDiscoveryV2(typedNil, func() time.Time { return now }); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil=%v", err)
	}
	client, _ := sdk.NewMCPDiscoveryV2(mcpDiscoverySDKReaderV2{value: value}, func() time.Time { return now.Add(time.Second) })
	if _, err := client.InspectCurrentMCPCapabilitySnapshotV2(nil, value.ObjectRef()); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context=%v", err)
	}
	client, _ = sdk.NewMCPDiscoveryV2(mcpDiscoverySDKReaderV2{value: value}, func() time.Time { return time.Unix(0, value.ExpiresUnixNano) })
	if _, err := client.InspectCurrentMCPCapabilitySnapshotV2(context.Background(), value.ObjectRef()); err == nil {
		t.Fatal("expired Snapshot was accepted")
	}
}

type mcpDiscoverySDKReaderV2 struct {
	value toolcontract.MCPCapabilitySnapshotV2
}

func (r mcpDiscoverySDKReaderV2) InspectMCPCapabilitySnapshotV2(_ context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV2, error) {
	if exact != r.value.ObjectRef() {
		return toolcontract.MCPCapabilitySnapshotV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Snapshot not found")
	}
	return toolcontract.CloneMCPCapabilitySnapshotV2(r.value), nil
}
