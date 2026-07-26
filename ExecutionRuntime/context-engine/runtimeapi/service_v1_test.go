package runtimeapi_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/runtimeapi"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type toolReaderStubV1 struct {
	result toolcontract.ToolResultV2
	err    error
}

func (r *toolReaderStubV1) InspectSettledToolResultCurrentV2(context.Context, toolcontract.ObjectRef) (toolcontract.ToolResultV2, error) {
	if r.err != nil {
		return toolcontract.ToolResultV2{}, r.err
	}
	return r.result, nil
}

type storeStubV1 struct{}

func (*storeStubV1) GetContextV1(context.Context, contract.ContentRef) ([]byte, error) {
	return nil, contract.ErrNotFound
}
func (*storeStubV1) PutContextV1(context.Context, []byte) (contract.ContentRef, error) {
	return contract.ContentRef{}, contract.ErrUnavailable
}

func TestNewServiceV1RejectsNilAndTypedNilDependencies(t *testing.T) {
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	frameReader := &testfixture.FrameConsumptionReaderV1{Snapshots: []kernel.FrameConsumptionCurrentSnapshotV1{fixture.Snapshot}}
	toolReader := &toolReaderStubV1{}
	store := &storeStubV1{}

	var nilFrameReader *testfixture.FrameConsumptionReaderV1
	var nilToolReader *toolReaderStubV1
	var nilStore *storeStubV1
	cases := []struct {
		name  string
		frame kernel.FrameConsumptionCurrentReaderV1
		tool  kernel.SettledToolResultCurrentReaderV2
		store kernel.ContextAwareReferenceStoreV1
	}{
		{"nil_frame", nil, toolReader, store},
		{"typed_nil_frame", nilFrameReader, toolReader, store},
		{"nil_tool", frameReader, nil, store},
		{"typed_nil_tool", frameReader, nilToolReader, store},
		{"nil_store", frameReader, toolReader, nil},
		{"typed_nil_store", frameReader, toolReader, nilStore},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, gotErr := runtimeapi.NewServiceV1(tc.frame, tc.tool, tc.store)
			if service != nil || !errors.Is(gotErr, contract.ErrInvalid) {
				t.Fatalf("service=%v error=%v", service, gotErr)
			}
		})
	}
}

func TestServiceV1ConsumeFrameDelegatesExactKernelBehavior(t *testing.T) {
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	reader := &testfixture.FrameConsumptionReaderV1{Snapshots: []kernel.FrameConsumptionCurrentSnapshotV1{fixture.Snapshot, fixture.Snapshot}}
	service, err := runtimeapi.NewServiceV1(reader, &toolReaderStubV1{}, fixture.AwareStore)
	if err != nil {
		t.Fatal(err)
	}
	var api runtimeapi.ContextRuntimeAPIV1 = service
	descriptor, err := api.ConsumeFrame(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.FrameRef != fixture.Request.FrameRef || reader.Calls != 2 {
		t.Fatalf("descriptor=%+v calls=%d", descriptor, reader.Calls)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	descriptor, err = api.ConsumeFrame(ctx, fixture.Request)
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(descriptor, contract.ContextFrameConsumptionDescriptorV1{}) {
		t.Fatalf("descriptor=%+v error=%v", descriptor, err)
	}
}

func TestNilServiceV1FailsClosed(t *testing.T) {
	var service *runtimeapi.ServiceV1
	if got, err := service.ConsumeFrame(context.Background(), contract.ContextFrameConsumptionRequestV1{}); !errors.Is(err, contract.ErrInvalid) || !reflect.DeepEqual(got, contract.ContextFrameConsumptionDescriptorV1{}) {
		t.Fatalf("consume got=%+v error=%v", got, err)
	}
	if got, err := service.AppendSettledToolResult(context.Background(), runtimeapi.AppendSettledToolResultRequestV2{}); !errors.Is(err, contract.ErrInvalid) || !reflect.DeepEqual(got, runtimeapi.AppendSettledToolResultResultV2{}) {
		t.Fatalf("append got=%+v error=%v", got, err)
	}
}
