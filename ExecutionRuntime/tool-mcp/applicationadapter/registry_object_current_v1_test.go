package applicationadapter

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

func TestRegistryObjectCurrentReaderV1ExactAndFailClosed(t *testing.T) {
	reader, clock, capability, tool := registryCurrentFixtureV1(t)
	ctx := context.Background()
	capObject := toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
	toolObject := toolcontract.ObjectRef{ID: string(tool.ID), Revision: tool.Revision, Digest: tool.Digest}

	capValue, capCurrent, err := reader.ResolveExactToolCapabilityCurrentV1(ctx, capObject)
	if err != nil || capValue.Digest != capability.Digest || capCurrent.Object != capObject || capCurrent.RegistryOwner != capability.Owner {
		t.Fatalf("resolve capability current: %v", err)
	}
	toolValue, toolCurrent, err := reader.ResolveExactToolDescriptorCurrentV1(ctx, toolObject)
	if err != nil || toolValue.Digest != tool.Digest || toolCurrent.Object != toolObject || toolCurrent.RegistryOwner != tool.Owner {
		t.Fatalf("resolve tool current: %v", err)
	}
	if _, _, err = reader.InspectExactToolCapabilityCurrentV1(ctx, capObject, toolCurrent.Ref); err == nil {
		t.Fatal("Tool current Ref was type-punned as Capability current")
	}
	drift := capObject
	drift.Digest = testkit.Digest("drift")
	if _, _, err = reader.ResolveExactToolCapabilityCurrentV1(ctx, drift); err == nil {
		t.Fatal("same ID with another digest was accepted")
	}
	clock.Set(testkit.FixedTime.Add(-2 * time.Second))
	if _, _, err = reader.InspectExactToolCapabilityCurrentV1(ctx, capObject, capCurrent.Ref); err == nil || !core.HasCategory(err, core.ErrorPreconditionFailed) {
		t.Fatalf("clock rollback was not rejected: %v", err)
	}
}

func TestRegistryObjectCurrentReaderV1ConcurrentExact(t *testing.T) {
	reader, _, capability, _ := registryCurrentFixtureV1(t)
	object := toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, projection, err := reader.ResolveExactToolCapabilityCurrentV1(context.Background(), object)
			if err == nil {
				err = projection.ValidateCurrent(testkit.FixedTime)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestRegistryObjectCurrentReaderV1NilBoundaries(t *testing.T) {
	var nilClock *testkit.ManualClock
	if _, err := NewRegistryObjectCurrentReaderV1(registry.New(), nilClock); err == nil {
		t.Fatal("typed-nil clock was accepted")
	}
	reader, _, capability, _ := registryCurrentFixtureV1(t)
	object := toolcontract.ObjectRef{ID: string(capability.ID), Revision: capability.Revision, Digest: capability.Digest}
	if _, _, err := reader.ResolveExactToolCapabilityCurrentV1(nil, object); err == nil {
		t.Fatal("nil context was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := reader.ResolveExactToolCapabilityCurrentV1(ctx, object); err != context.Canceled {
		t.Fatalf("canceled context was not preserved: %v", err)
	}
}

func registryCurrentFixtureV1(t *testing.T) (*RegistryObjectCurrentReaderV1, *testkit.ManualClock, toolcontract.CapabilityDescriptor, toolcontract.ToolDescriptor) {
	t.Helper()
	r := registry.New()
	capability, tool := testkit.Capability(), testkit.Tool()
	capRecord, err := r.SubmitCapability(capability, testkit.FixedTime.Add(-3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	capRecord, err = r.Transition("capability", string(capability.ID), capRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime.Add(-2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Transition("capability", string(capability.ID), capRecord.RegistryRevision, registry.StateActive, testkit.FixedTime.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	toolRecord, err := r.SubmitTool(tool, testkit.FixedTime.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	toolRecord, err = r.Transition("tool", string(tool.ID), toolRecord.RegistryRevision, registry.StateAdmitted, testkit.FixedTime.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Transition("tool", string(tool.ID), toolRecord.RegistryRevision, registry.StateActive, testkit.FixedTime); err != nil {
		t.Fatal(err)
	}
	clock := testkit.NewManualClock(testkit.FixedTime)
	reader, err := NewRegistryObjectCurrentReaderV1(r, clock)
	if err != nil {
		t.Fatal(err)
	}
	return reader, clock, capability, tool
}
