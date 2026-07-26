package failure_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func TestFrameConsumptionFailurePreservesUnknownAndReturnsNoDescriptor(t *testing.T) {
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	reader := &testfixture.FrameConsumptionReaderV1{Errors: []error{contract.ErrUnknown}}
	descriptor, err := kernel.BuildFrameConsumptionDescriptorV1(context.Background(), reader, fixture.AwareStore, fixture.Request)
	if !errors.Is(err, contract.ErrUnknown) || !reflect.DeepEqual(descriptor, contract.ContextFrameConsumptionDescriptorV1{}) {
		t.Fatalf("descriptor=%#v err=%v", descriptor, err)
	}
}

func TestFrameConsumptionContentDriftFailsClosed(t *testing.T) {
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	drift := fixture.Snapshot
	drift.Frame.DynamicTail.Digest = contract.DigestBytes([]byte("drift"))
	reader := &testfixture.FrameConsumptionReaderV1{Snapshots: []kernel.FrameConsumptionCurrentSnapshotV1{drift}}
	descriptor, err := kernel.BuildFrameConsumptionDescriptorV1(context.Background(), reader, fixture.AwareStore, fixture.Request)
	if err == nil || !reflect.DeepEqual(descriptor, contract.ContextFrameConsumptionDescriptorV1{}) {
		t.Fatalf("content drift descriptor=%#v err=%v", descriptor, err)
	}
}
