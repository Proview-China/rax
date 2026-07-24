package api

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

type apiSnapshotReaderV3Func func(context.Context, toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error)

func (f apiSnapshotReaderV3Func) InspectMCPCapabilitySnapshotV3(ctx context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error) {
	return f(ctx, exact)
}

func TestMCPDiscoveryReadV3ExactCurrent(t *testing.T) {
	now := testkit.FixedTime
	snapshot := testkit.MCPCapabilitySnapshotV3(now)
	reads := 0
	reader := apiSnapshotReaderV3Func(func(context.Context, toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV3, error) {
		reads++
		return toolcontract.CloneMCPCapabilitySnapshotV3(snapshot), nil
	})
	api, err := NewMCPDiscoveryReadV3(reader, func() time.Time { return now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	got, err := api.InspectCurrentMCPCapabilitySnapshotV3(context.Background(), snapshot.ObjectRef())
	if err != nil || reads != 2 || got.ObjectRef() != snapshot.ObjectRef() {
		t.Fatalf("got=%#v reads=%d err=%v", got, reads, err)
	}
	if _, err = api.InspectCurrentMCPCapabilitySnapshotV3(nil, snapshot.ObjectRef()); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context error=%v", err)
	}
}
