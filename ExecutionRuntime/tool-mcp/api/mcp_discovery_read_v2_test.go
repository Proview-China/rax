package api_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/api"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
)

func TestMCPDiscoveryReadAPIV2ExactCurrentAndClone(t *testing.T) {
	now := testkit.FixedTime
	source := &mcpDiscoverySnapshotSourceV2{value: testkit.MCPCapabilitySnapshotV2(now)}
	read, err := api.NewMCPDiscoveryReadV2(source, func() time.Time { return now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	value, err := read.InspectCurrentMCPCapabilitySnapshotV2(context.Background(), source.value.ObjectRef())
	if err != nil || value.Digest != source.value.Digest {
		t.Fatalf("snapshot=%#v err=%v", value, err)
	}
	value.Tools[0].Name = "mutated"
	again, _ := read.InspectCurrentMCPCapabilitySnapshotV2(context.Background(), source.value.ObjectRef())
	if again.Tools[0].Name != "echo" {
		t.Fatal("API returned aliased Snapshot")
	}
}

func TestMCPDiscoveryReadAPIV2FailsClosedAndConcurrent(t *testing.T) {
	now := testkit.FixedTime
	source := &mcpDiscoverySnapshotSourceV2{value: testkit.MCPCapabilitySnapshotV2(now)}
	var typedNil *mcpDiscoverySnapshotSourceV2
	if _, err := api.NewMCPDiscoveryReadV2(typedNil, func() time.Time { return now }); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed nil=%v", err)
	}
	read, _ := api.NewMCPDiscoveryReadV2(source, func() time.Time { return now.Add(time.Second) })
	if _, err := read.InspectCurrentMCPCapabilitySnapshotV2(nil, source.value.ObjectRef()); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("nil context=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := read.InspectCurrentMCPCapabilitySnapshotV2(ctx, source.value.ObjectRef()); err != context.Canceled {
		t.Fatalf("canceled context=%v", err)
	}
	source.drift = true
	if _, err := read.InspectCurrentMCPCapabilitySnapshotV2(context.Background(), source.value.ObjectRef()); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("S1/S2 drift=%v", err)
	}
	source.drift = false
	const workers = 64
	var group sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := read.InspectCurrentMCPCapabilitySnapshotV2(context.Background(), source.value.ObjectRef())
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

type mcpDiscoverySnapshotSourceV2 struct {
	mu    sync.Mutex
	value toolcontract.MCPCapabilitySnapshotV2
	drift bool
	calls int
}

func (s *mcpDiscoverySnapshotSourceV2) InspectMCPCapabilitySnapshotV2(_ context.Context, exact toolcontract.ObjectRef) (toolcontract.MCPCapabilitySnapshotV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if exact != s.value.ObjectRef() {
		return toolcontract.MCPCapabilitySnapshotV2{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "Snapshot not found")
	}
	s.calls++
	value := toolcontract.CloneMCPCapabilitySnapshotV2(s.value)
	if s.drift && s.calls%2 == 0 {
		value.Tools[0].Name = "drift"
		value.Digest, value.ValidationDigest = "", ""
		value, _ = toolcontract.SealMCPCapabilitySnapshotV2(value)
	}
	return value, nil
}
