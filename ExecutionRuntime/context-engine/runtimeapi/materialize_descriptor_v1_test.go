package runtimeapi_test

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/runtimeapi"
)

func materializeFixtureV1(t *testing.T, semi string) (*testfixture.FrameConsumptionFixtureV1, *runtimeapi.ServiceV1, contract.ContextFrameConsumptionDescriptorV1) {
	t.Helper()
	fixture, err := testfixture.NewFrameConsumptionFixtureWithContentV1("stable materialize", semi, "dynamic materialize")
	if err != nil {
		t.Fatal(err)
	}
	reader := &testfixture.FrameConsumptionReaderV1{Snapshots: []kernel.FrameConsumptionCurrentSnapshotV1{fixture.Snapshot, fixture.Snapshot}}
	service, err := runtimeapi.NewServiceV1(reader, &toolReaderStubV1{}, fixture.AwareStore)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := service.ConsumeFrame(context.Background(), fixture.Request)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, service, descriptor
}

func materializeRequestV1(descriptor contract.ContextFrameConsumptionDescriptorV1) runtimeapi.MaterializeDescriptorRequestV1 {
	return runtimeapi.MaterializeDescriptorRequestV1{
		Descriptor: descriptor, CheckedUnixNano: testkit.Now + 1,
		Limits: runtimeapi.MaterializeDescriptorLimitsV1{MaxItems: 4, MaxItemBytes: runtimeapi.HardMaxMaterializedItemBytesV1, MaxTotalBytes: runtimeapi.HardMaxMaterializedTotalBytesV1},
	}
}

func TestMaterializeDescriptorCanonicalAndDeepCopy(t *testing.T) {
	_, service, descriptor := materializeFixtureV1(t, "semi materialize")
	request := materializeRequestV1(descriptor)
	first, err := service.MaterializeDescriptor(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err = first.Validate(); err != nil || len(first.Items) != 4 || first.ExpiresUnixNano != descriptor.ExpiresUnixNano {
		t.Fatalf("result=%+v error=%v", first, err)
	}
	regions := []runtimeapi.MaterializedDescriptorRegionV1{runtimeapi.MaterializedStablePrefixV1, runtimeapi.MaterializedSemiStableV1, runtimeapi.MaterializedDynamicTailV1, runtimeapi.MaterializedRenderedV1}
	for index, region := range regions {
		if first.Items[index].Region != region {
			t.Fatalf("item %d region=%s", index, first.Items[index].Region)
		}
	}
	second, err := service.MaterializeDescriptor(context.Background(), request)
	if err != nil || first.Digest != second.Digest {
		t.Fatalf("second=%+v error=%v", second, err)
	}
	original := second.Items[0].Bytes[0]
	first.Items[0].Bytes[0] ^= 0xff
	if second.Items[0].Bytes[0] != original {
		t.Fatal("results alias each other")
	}
	if err = first.Validate(); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("mutated result error=%v", err)
	}
	third, err := service.MaterializeDescriptor(context.Background(), request)
	if err != nil || third.Items[0].Bytes[0] != original || third.Digest != second.Digest {
		t.Fatalf("store or service was aliased: result=%+v error=%v", third, err)
	}
}

func TestMaterializeDescriptorWithoutSemiHasThreeItems(t *testing.T) {
	_, service, descriptor := materializeFixtureV1(t, "")
	request := materializeRequestV1(descriptor)
	request.Limits.MaxItems = 3
	got, err := service.MaterializeDescriptor(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	want := []runtimeapi.MaterializedDescriptorRegionV1{runtimeapi.MaterializedStablePrefixV1, runtimeapi.MaterializedDynamicTailV1, runtimeapi.MaterializedRenderedV1}
	if len(got.Items) != len(want) {
		t.Fatalf("items=%d", len(got.Items))
	}
	for index := range want {
		if got.Items[index].Region != want[index] {
			t.Fatalf("item %d region=%s", index, got.Items[index].Region)
		}
	}
}

type observedStoreV1 struct {
	base      testfixture.ContextAwareRefStoreV1
	calls     atomic.Int64
	fail      error
	corrupt   bool
	cancel    context.CancelFunc
	cancelOne atomic.Bool
}

func (s *observedStoreV1) GetContextV1(ctx context.Context, ref contract.ContentRef) ([]byte, error) {
	s.calls.Add(1)
	if s.fail != nil {
		return nil, s.fail
	}
	value, err := s.base.GetContextV1(ctx, ref)
	if err == nil && s.corrupt && len(value) > 0 {
		value[0] ^= 0xff
	}
	if err == nil && s.cancel != nil && s.cancelOne.CompareAndSwap(false, true) {
		s.cancel()
	}
	return value, err
}
func (s *observedStoreV1) PutContextV1(ctx context.Context, value []byte) (contract.ContentRef, error) {
	return s.base.PutContextV1(ctx, value)
}

func serviceWithStoreV1(t *testing.T, fixture *testfixture.FrameConsumptionFixtureV1, store kernel.ContextAwareReferenceStoreV1) *runtimeapi.ServiceV1 {
	t.Helper()
	reader := &testfixture.FrameConsumptionReaderV1{Snapshots: []kernel.FrameConsumptionCurrentSnapshotV1{fixture.Snapshot}}
	service, err := runtimeapi.NewServiceV1(reader, &toolReaderStubV1{}, store)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestMaterializeDescriptorLimitsPrecedeStoreReads(t *testing.T) {
	fixture, _, descriptor := materializeFixtureV1(t, "semi materialize")
	cases := []struct {
		name   string
		mutate func(*runtimeapi.MaterializeDescriptorRequestV1)
	}{
		{"items", func(r *runtimeapi.MaterializeDescriptorRequestV1) { r.Limits.MaxItems = 3 }},
		{"item_bytes", func(r *runtimeapi.MaterializeDescriptorRequestV1) {
			r.Limits.MaxItemBytes = descriptor.StablePrefix.Length - 1
		}},
		{"total_bytes", func(r *runtimeapi.MaterializeDescriptorRequestV1) {
			r.Limits.MaxTotalBytes = descriptor.StablePrefix.Length
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &observedStoreV1{base: fixture.AwareStore}
			service := serviceWithStoreV1(t, fixture, store)
			request := materializeRequestV1(descriptor)
			tc.mutate(&request)
			got, err := service.MaterializeDescriptor(context.Background(), request)
			if !errors.Is(err, contract.ErrLimitExceeded) || !reflect.DeepEqual(got, runtimeapi.MaterializeDescriptorResultV1{}) || store.calls.Load() != 0 {
				t.Fatalf("got=%+v error=%v reads=%d", got, err, store.calls.Load())
			}
		})
	}
}

func TestMaterializeDescriptorReadAndTimeFailuresAreZero(t *testing.T) {
	fixture, _, descriptor := materializeFixtureV1(t, "semi materialize")
	cases := []struct {
		name    string
		store   *observedStoreV1
		mutate  func(*runtimeapi.MaterializeDescriptorRequestV1)
		want    error
		maxRead int64
	}{
		{"missing", &observedStoreV1{base: fixture.AwareStore, fail: contract.ErrNotFound}, func(*runtimeapi.MaterializeDescriptorRequestV1) {}, contract.ErrNotFound, 1},
		{"unknown", &observedStoreV1{base: fixture.AwareStore, fail: contract.ErrUnknown}, func(*runtimeapi.MaterializeDescriptorRequestV1) {}, contract.ErrUnknown, 1},
		{"corrupt", &observedStoreV1{base: fixture.AwareStore, corrupt: true}, func(*runtimeapi.MaterializeDescriptorRequestV1) {}, contract.ErrConflict, 1},
		{"rollback", &observedStoreV1{base: fixture.AwareStore}, func(r *runtimeapi.MaterializeDescriptorRequestV1) { r.CheckedUnixNano = descriptor.CheckedUnixNano - 1 }, contract.ErrConflict, 0},
		{"expired", &observedStoreV1{base: fixture.AwareStore}, func(r *runtimeapi.MaterializeDescriptorRequestV1) { r.CheckedUnixNano = descriptor.ExpiresUnixNano }, contract.ErrExpired, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := serviceWithStoreV1(t, fixture, tc.store)
			request := materializeRequestV1(descriptor)
			tc.mutate(&request)
			got, err := service.MaterializeDescriptor(context.Background(), request)
			if !errors.Is(err, tc.want) || !reflect.DeepEqual(got, runtimeapi.MaterializeDescriptorResultV1{}) || tc.store.calls.Load() > tc.maxRead {
				t.Fatalf("got=%+v error=%v reads=%d", got, err, tc.store.calls.Load())
			}
		})
	}
}

func TestMaterializeDescriptorCancellationIsZero(t *testing.T) {
	fixture, _, descriptor := materializeFixtureV1(t, "semi materialize")
	ctx, cancel := context.WithCancel(context.Background())
	store := &observedStoreV1{base: fixture.AwareStore, cancel: cancel}
	service := serviceWithStoreV1(t, fixture, store)
	got, err := service.MaterializeDescriptor(ctx, materializeRequestV1(descriptor))
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, runtimeapi.MaterializeDescriptorResultV1{}) || store.calls.Load() != 1 {
		t.Fatalf("got=%+v error=%v reads=%d", got, err, store.calls.Load())
	}
}
