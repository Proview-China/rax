package blackbox_test

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestFrameConsumptionPublicSliceProducesExactImmutableDescriptor(t *testing.T) {
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	reader := &testfixture.FrameConsumptionReaderV1{Snapshots: []kernel.FrameConsumptionCurrentSnapshotV1{fixture.Snapshot, fixture.Snapshot}}
	descriptor, err := kernel.BuildFrameConsumptionDescriptorV1(context.Background(), reader, fixture.AwareStore, fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	if err = descriptor.Validate(); err != nil {
		t.Fatal(err)
	}
	if descriptor.FrameRef != fixture.Request.FrameRef || descriptor.ManifestRef != fixture.Request.ManifestRef || descriptor.GenerationRef != fixture.Request.GenerationRef {
		t.Fatal("descriptor did not preserve exact Context references")
	}
	if descriptor.StablePrefix != fixture.Result.Frame.StablePrefix || descriptor.DynamicTail != fixture.Result.Frame.DynamicTail || descriptor.Rendered != fixture.Result.Frame.Rendered {
		t.Fatal("descriptor did not preserve exact materialization references")
	}
}
