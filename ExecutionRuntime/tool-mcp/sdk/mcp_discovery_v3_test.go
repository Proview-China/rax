package sdk

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

type snapshotReaderV3Func func(context.Context, toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error)

func (f snapshotReaderV3Func) InspectMCPCapabilitySnapshotV3(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error) {
	return f(ctx, exact)
}

func TestMCPDiscoveryV3ExactDoubleReadAndFailures(t *testing.T) {
	now := testkit.FixedTime
	snapshot := testkit.MCPCapabilitySnapshotV3(now)
	reads := 0
	reader := snapshotReaderV3Func(func(context.Context, toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error) {
		reads++
		return toolcontract.CloneMCPCapabilitySnapshotV3(snapshot), nil
	})
	client, err := NewMCPDiscoveryV3(reader, func() time.Time { return now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.InspectCurrentMCPCapabilitySnapshotV3(context.Background(), snapshot.ObjectRef())
	if err != nil || reads != 2 || got.ObjectRef() != snapshot.ObjectRef() {
		t.Fatalf("got=%#v reads=%d err=%v", got, reads, err)
	}
	got.ToolMaterials[0].Source.Name = "mutated"
	if snapshot.ToolMaterials[0].Source.Name != "echo" {
		t.Fatal("SDK returned aliased Snapshot V3")
	}
	if _, err = client.InspectCurrentMCPCapabilitySnapshotV3(nil, snapshot.ObjectRef()); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = client.InspectCurrentMCPCapabilitySnapshotV3(ctx, snapshot.ObjectRef()); err != context.Canceled {
		t.Fatalf("canceled context error=%v", err)
	}
	driftReads := 0
	driftReader := snapshotReaderV3Func(func(context.Context, toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error) {
		driftReads++
		value := toolcontract.CloneMCPCapabilitySnapshotV3(snapshot)
		if driftReads == 2 {
			value.SourceDigest = testkit.Digest("drift")
			value.Digest = ""
			value, _ = toolcontract.SealMCPCapabilitySnapshotV3(value)
		}
		return value, nil
	})
	driftClient, _ := NewMCPDiscoveryV3(driftReader, func() time.Time { return now.Add(time.Second) })
	if _, err = driftClient.InspectCurrentMCPCapabilitySnapshotV3(context.Background(), snapshot.ObjectRef()); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("drift error=%v", err)
	}
	var typedNil *snapshotReaderV3Stub
	if _, err = NewMCPDiscoveryV3(typedNil, func() time.Time { return now }); err == nil {
		t.Fatal("typed-nil Snapshot V3 reader was accepted")
	}
}

type snapshotReaderV3Stub struct{}

func (*snapshotReaderV3Stub) InspectMCPCapabilitySnapshotV3(context.Context, toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error) {
	return toolcontract.MCPCapabilitySnapshotV3{}, nil
}
