package modelinvokeradapter_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/modelinvokeradapter"
	contextports "github.com/Proview-China/rax/ExecutionRuntime/context-engine/ports"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

var adapterNowV1 = time.Unix(1_810_000_000, 0)

func TestContextOwnerBindingAdapterMapsAuthoritativeProjectionLosslesslyV1(t *testing.T) {
	fixture := newAdapterFixtureV1(t)
	reader := &scriptedLineageReaderV1{projections: []contract.ContextModelInputLineageCurrentProjectionV1{fixture.lineage}}
	adapter := newAdapterV1(t, fixture.owner, reader, fixedAdapterClockV1(fixture.now.Add(time.Second)))

	projection, err := adapter.InspectCurrentInvocationContextOwnerBindingV1(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if reader.callCountV1() != 2 {
		t.Fatalf("authoritative lineage read count=%d want=2", reader.callCountV1())
	}
	for _, observed := range reader.requestsV1() {
		if observed.Source != fixture.contextRequest.Source ||
			observed.Source.Kind != contract.ContextInvocationSourceModelInputMaterialV1 ||
			observed != fixture.contextRequest {
			t.Fatalf("Model request was not mapped to the exact Context material source: %+v", observed)
		}
	}
	if projection.ContextOwner.ComponentID != fixture.owner.ComponentID ||
		projection.ContextOwner.BindingDigest != core.Digest(fixture.owner.BindingDigest) {
		t.Fatalf("authoritative Context owner was not preserved: %+v", projection.ContextOwner)
	}
	if projection.Material.Kind != string(fixture.lineage.Material.Kind) ||
		projection.Material.ID != fixture.lineage.Material.ID ||
		uint64(projection.Material.Revision) != fixture.lineage.Material.Revision ||
		projection.Material.Digest != core.Digest(fixture.lineage.Material.Digest) ||
		projection.Frame.Kind != string(fixture.lineage.Frame.Kind) ||
		projection.Frame.ID != fixture.lineage.Frame.ID ||
		uint64(projection.Frame.Revision) != fixture.lineage.Frame.Revision ||
		projection.Frame.Digest != core.Digest(fixture.lineage.Frame.Digest) {
		t.Fatalf("Context exact sources were not mapped losslessly: %+v", projection)
	}
	if projection.ContextLineageDigest != core.Digest(fixture.lineage.Digest) {
		t.Fatalf("Context lineage digest=%s want=%s", projection.ContextLineageDigest, fixture.lineage.Digest)
	}
	if projection.Material.Owner != projection.NeutralOwner ||
		projection.Frame.Owner != projection.NeutralOwner ||
		projection.CheckedUnixNano != fixture.now.Add(time.Second).UnixNano() ||
		projection.ExpiresUnixNano != fixture.lineage.ExpiresUnixNano {
		t.Fatalf("neutral owner or fresh seal drifted: %+v", projection)
	}
	if err := projection.ValidateAgainstV1(fixture.request, fixture.now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestContextOwnerBindingAdapterRejectsCallerArbitraryKindBeforeOwnerReadV1(t *testing.T) {
	fixture := newAdapterFixtureV1(t)
	request := fixture.request
	request.MaterialLookup.Kind = "caller/arbitrary-context-kind"
	request.Digest = ""
	var err error
	request, err = modelinvoker.SealInvocationContextOwnerBindingRequestV1(request)
	if err != nil {
		t.Fatal(err)
	}
	reader := &scriptedLineageReaderV1{projections: []contract.ContextModelInputLineageCurrentProjectionV1{fixture.lineage}}
	adapter := newAdapterV1(t, fixture.owner, reader, fixedAdapterClockV1(fixture.now))

	projection, inspectErr := adapter.InspectCurrentInvocationContextOwnerBindingV1(context.Background(), request)
	if !errors.Is(inspectErr, contract.ErrConflict) ||
		projection != (modelinvoker.InvocationContextOwnerBindingProjectionV1{}) ||
		reader.callCountV1() != 0 {
		t.Fatalf("arbitrary Kind reached Context owner: projection=%+v calls=%d err=%v", projection, reader.callCountV1(), inspectErr)
	}
}

func TestContextOwnerBindingAdapterRejectsEveryAuthoritativeCoordinateDriftV1(t *testing.T) {
	fixture := newAdapterFixtureV1(t)
	mutations := map[string]func(*contract.ContextModelInputLineageCurrentProjectionV1){
		"material owner component": func(v *contract.ContextModelInputLineageCurrentProjectionV1) {
			v.Material.Owner.ComponentID += "-drift"
		},
		"material owner binding": func(v *contract.ContextModelInputLineageCurrentProjectionV1) {
			v.Material.Owner.BindingDigest = contextDigestV1("binding-drift")
		},
		"material kind": func(v *contract.ContextModelInputLineageCurrentProjectionV1) {
			v.Material.Kind = contract.ContextInvocationSourceFrameV1
		},
		"material id":       func(v *contract.ContextModelInputLineageCurrentProjectionV1) { v.Material.ID += "-drift" },
		"material revision": func(v *contract.ContextModelInputLineageCurrentProjectionV1) { v.Material.Revision++ },
		"material digest": func(v *contract.ContextModelInputLineageCurrentProjectionV1) {
			v.Material.Digest = contextDigestV1("material-drift")
		},
		"frame owner component": func(v *contract.ContextModelInputLineageCurrentProjectionV1) {
			v.Frame.Owner.ComponentID += "-drift"
		},
		"frame owner binding": func(v *contract.ContextModelInputLineageCurrentProjectionV1) {
			v.Frame.Owner.BindingDigest = contextDigestV1("frame-binding-drift")
		},
		"frame kind": func(v *contract.ContextModelInputLineageCurrentProjectionV1) {
			v.Frame.Kind = contract.ContextInvocationSourceModelInputMaterialV1
		},
		"frame id":       func(v *contract.ContextModelInputLineageCurrentProjectionV1) { v.Frame.ID += "-drift" },
		"frame revision": func(v *contract.ContextModelInputLineageCurrentProjectionV1) { v.Frame.Revision++ },
		"frame digest": func(v *contract.ContextModelInputLineageCurrentProjectionV1) {
			v.Frame.Digest = contextDigestV1("frame-drift")
		},
		"material frame swap": func(v *contract.ContextModelInputLineageCurrentProjectionV1) {
			v.Material, v.Frame = v.Frame, v.Material
		},
		"lineage digest": func(v *contract.ContextModelInputLineageCurrentProjectionV1) {
			v.Digest = contextDigestV1("lineage-drift")
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := fixture.lineage
			mutate(&changed)
			reader := &scriptedLineageReaderV1{projections: []contract.ContextModelInputLineageCurrentProjectionV1{changed}}
			adapter := newAdapterV1(t, fixture.owner, reader, fixedAdapterClockV1(fixture.now.Add(time.Second)))
			projection, err := adapter.InspectCurrentInvocationContextOwnerBindingV1(context.Background(), fixture.request)
			if err == nil || projection != (modelinvoker.InvocationContextOwnerBindingProjectionV1{}) {
				t.Fatalf("authoritative %s drift accepted: projection=%+v err=%v", name, projection, err)
			}
		})
	}
}

func TestContextOwnerBindingAdapterRejectsS1S2FullProjectionDriftV1(t *testing.T) {
	fixture := newAdapterFixtureV1(t)
	tests := map[string]func(contract.ContextModelInputLineageCurrentProjectionV1) contract.ContextModelInputLineageCurrentProjectionV1{
		"frame coordinate": func(v contract.ContextModelInputLineageCurrentProjectionV1) contract.ContextModelInputLineageCurrentProjectionV1 {
			v.Frame.ID += "-next"
			v.Frame.Revision++
			v.Frame.Digest = contextDigestV1("frame-next")
			return resealContextLineageV1(t, v, fixture.now)
		},
		"checked": func(v contract.ContextModelInputLineageCurrentProjectionV1) contract.ContextModelInputLineageCurrentProjectionV1 {
			v.CheckedUnixNano++
			return resealContextLineageV1(t, v, fixture.now.Add(time.Nanosecond))
		},
		"expiry": func(v contract.ContextModelInputLineageCurrentProjectionV1) contract.ContextModelInputLineageCurrentProjectionV1 {
			v.ExpiresUnixNano--
			return resealContextLineageV1(t, v, fixture.now)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			s2 := mutate(fixture.lineage)
			reader := &scriptedLineageReaderV1{projections: []contract.ContextModelInputLineageCurrentProjectionV1{fixture.lineage, s2}}
			adapter := newAdapterV1(t, fixture.owner, reader, fixedAdapterClockV1(fixture.now.Add(time.Second)))
			projection, err := adapter.InspectCurrentInvocationContextOwnerBindingV1(context.Background(), fixture.request)
			if !errors.Is(err, contract.ErrConflict) ||
				projection != (modelinvoker.InvocationContextOwnerBindingProjectionV1{}) ||
				reader.callCountV1() != 2 {
				t.Fatalf("%s S1/S2 drift: projection=%+v calls=%d err=%v", name, projection, reader.callCountV1(), err)
			}
		})
	}
}

func TestContextOwnerBindingAdapterFailClosedOwnerOutcomesAndTypedNilV1(t *testing.T) {
	fixture := newAdapterFixtureV1(t)
	for name, injected := range map[string]error{
		"history_only":  contract.ErrConflict,
		"current_false": contract.ErrConflict,
		"unavailable":   contract.ErrUnavailable,
		"unknown":       contract.ErrUnknown,
	} {
		t.Run(name, func(t *testing.T) {
			reader := &scriptedLineageReaderV1{errs: []error{injected}}
			adapter := newAdapterV1(t, fixture.owner, reader, fixedAdapterClockV1(fixture.now))
			projection, err := adapter.InspectCurrentInvocationContextOwnerBindingV1(context.Background(), fixture.request)
			if !errors.Is(err, injected) || projection != (modelinvoker.InvocationContextOwnerBindingProjectionV1{}) {
				t.Fatalf("%s was not zero-projection: projection=%+v err=%v", name, projection, err)
			}
		})
	}

	var typedNil *scriptedLineageReaderV1
	if adapter, err := modelinvokeradapter.NewInvocationContextOwnerBindingAdapterV1(fixture.owner, typedNil, fixedAdapterClockV1(fixture.now)); err == nil || adapter != nil {
		t.Fatalf("typed nil lineage reader accepted: adapter=%v err=%v", adapter, err)
	}
	if adapter, err := modelinvokeradapter.NewInvocationContextOwnerBindingAdapterV1(fixture.owner, nil, fixedAdapterClockV1(fixture.now)); err == nil || adapter != nil {
		t.Fatalf("nil lineage reader accepted: adapter=%v err=%v", adapter, err)
	}
	var nilAdapter *modelinvokeradapter.InvocationContextOwnerBindingAdapterV1
	if projection, err := nilAdapter.InspectCurrentInvocationContextOwnerBindingV1(context.Background(), fixture.request); !errors.Is(err, contract.ErrUnavailable) || projection != (modelinvoker.InvocationContextOwnerBindingProjectionV1{}) {
		t.Fatalf("nil adapter was not unavailable/zero: projection=%+v err=%v", projection, err)
	}
}

func TestContextOwnerBindingAdapterCancellationReturnsZeroProjectionV1(t *testing.T) {
	fixture := newAdapterFixtureV1(t)
	t.Run("before S1", func(t *testing.T) {
		reader := &scriptedLineageReaderV1{projections: []contract.ContextModelInputLineageCurrentProjectionV1{fixture.lineage}}
		adapter := newAdapterV1(t, fixture.owner, reader, fixedAdapterClockV1(fixture.now))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		projection, err := adapter.InspectCurrentInvocationContextOwnerBindingV1(ctx, fixture.request)
		if !errors.Is(err, context.Canceled) ||
			projection != (modelinvoker.InvocationContextOwnerBindingProjectionV1{}) ||
			reader.callCountV1() != 0 {
			t.Fatalf("pre-cancel: projection=%+v calls=%d err=%v", projection, reader.callCountV1(), err)
		}
	})
	t.Run("between S1 and S2", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		reader := &scriptedLineageReaderV1{
			projections: []contract.ContextModelInputLineageCurrentProjectionV1{fixture.lineage},
			after:       func(call int) { cancel() },
		}
		adapter := newAdapterV1(t, fixture.owner, reader, fixedAdapterClockV1(fixture.now.Add(time.Second)))
		projection, err := adapter.InspectCurrentInvocationContextOwnerBindingV1(ctx, fixture.request)
		if !errors.Is(err, context.Canceled) ||
			projection != (modelinvoker.InvocationContextOwnerBindingProjectionV1{}) ||
			reader.callCountV1() != 1 {
			t.Fatalf("mid-cancel: projection=%+v calls=%d err=%v", projection, reader.callCountV1(), err)
		}
	})
}

func TestContextOwnerBindingAdapterClockAndTTLFailClosedV1(t *testing.T) {
	fixture := newAdapterFixtureV1(t)
	tests := []struct {
		name   string
		values []time.Time
		want   error
	}{
		{"initial rollback", []time.Time{fixture.now.Add(-2 * time.Second)}, contract.ErrConflict},
		{"mid rollback", []time.Time{fixture.now, fixture.now.Add(-time.Nanosecond)}, contract.ErrConflict},
		{"seal rollback", []time.Time{fixture.now, fixture.now.Add(time.Second), fixture.now}, contract.ErrConflict},
		{"now equals expiry", []time.Time{fixture.now, fixture.now.Add(time.Second), time.Unix(0, fixture.lineage.ExpiresUnixNano)}, contract.ErrExpired},
		{"TTL crossing", []time.Time{fixture.now, fixture.now.Add(time.Second), time.Unix(0, fixture.lineage.ExpiresUnixNano).Add(time.Nanosecond)}, contract.ErrExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &scriptedLineageReaderV1{projections: []contract.ContextModelInputLineageCurrentProjectionV1{fixture.lineage}}
			adapter := newAdapterV1(t, fixture.owner, reader, scriptedAdapterClockV1(tt.values))
			projection, err := adapter.InspectCurrentInvocationContextOwnerBindingV1(context.Background(), fixture.request)
			if !errors.Is(err, tt.want) || projection != (modelinvoker.InvocationContextOwnerBindingProjectionV1{}) {
				t.Fatalf("%s: projection=%+v err=%v", tt.name, projection, err)
			}
		})
	}
}

func TestContextOwnerBindingAdapterPreservesOwnerWhenBindingDigestEqualsMaterialDigestV1(t *testing.T) {
	fixture := newAdapterFixtureV1(t)
	owner := fixture.owner
	owner.BindingDigest = fixture.lineage.Material.Digest
	material, err := contract.ContextModelInputMaterialExactSourceV1(
		owner,
		contract.ContextModelInputMaterialRefV1{
			ID: fixture.lineage.Material.ID, Revision: fixture.lineage.Material.Revision, Digest: fixture.lineage.Material.Digest,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := contract.ContextFrameExactSourceV1(
		owner,
		contract.FactRef{ID: fixture.lineage.Frame.ID, Revision: fixture.lineage.Frame.Revision, Digest: fixture.lineage.Frame.Digest},
	)
	if err != nil {
		t.Fatal(err)
	}
	lineage := resealContextLineageV1(t, contract.ContextModelInputLineageCurrentProjectionV1{
		Material: material, Frame: frame, CheckedUnixNano: fixture.lineage.CheckedUnixNano, ExpiresUnixNano: fixture.lineage.ExpiresUnixNano,
	}, fixture.now)
	request := modelRequestV1(t, material, fixture.now)
	reader := &scriptedLineageReaderV1{projections: []contract.ContextModelInputLineageCurrentProjectionV1{lineage}}
	adapter := newAdapterV1(t, owner, reader, fixedAdapterClockV1(fixture.now.Add(time.Second)))
	projection, err := adapter.InspectCurrentInvocationContextOwnerBindingV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if projection.ContextOwner.ComponentID != owner.ComponentID ||
		projection.ContextOwner.BindingDigest != core.Digest(owner.BindingDigest) ||
		projection.ContextOwner.BindingDigest != projection.Material.Digest {
		t.Fatalf("equal Binding/Material digest was not preserved losslessly: %+v", projection)
	}
	if projection.Material.Kind != string(contract.ContextInvocationSourceModelInputMaterialV1) ||
		projection.Frame.Kind != string(contract.ContextInvocationSourceFrameV1) ||
		projection.Material.ID != material.ID ||
		projection.Material.Revision != core.Revision(material.Revision) ||
		projection.Material.Digest != core.Digest(material.Digest) ||
		projection.Frame.ID != frame.ID ||
		projection.Frame.Revision != core.Revision(frame.Revision) ||
		projection.Frame.Digest != core.Digest(frame.Digest) ||
		projection.Material.Kind == projection.Frame.Kind ||
		projection.Material.Digest == projection.Frame.Digest {
		t.Fatalf("Material/Frame roles were not preserved by Kind and exact coordinate: %+v", projection)
	}
	if err := projection.ValidateAgainstV1(request, fixture.now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestContextOwnerBindingAdapter64ConcurrentStableWindowV1(t *testing.T) {
	fixture := newAdapterFixtureV1(t)
	reader := &scriptedLineageReaderV1{projections: []contract.ContextModelInputLineageCurrentProjectionV1{fixture.lineage}}
	adapter := newAdapterV1(t, fixture.owner, reader, fixedAdapterClockV1(fixture.now.Add(time.Second)))
	var expected modelinvoker.InvocationContextOwnerBindingProjectionV1
	var expectedSet atomic.Bool
	var failures atomic.Int64
	var mutex sync.Mutex
	var wait sync.WaitGroup
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			projection, err := adapter.InspectCurrentInvocationContextOwnerBindingV1(context.Background(), fixture.request)
			if err != nil {
				failures.Add(1)
				return
			}
			mutex.Lock()
			defer mutex.Unlock()
			if !expectedSet.Load() {
				expected = projection
				expectedSet.Store(true)
			} else if projection != expected {
				failures.Add(1)
			}
		}()
	}
	wait.Wait()
	if failures.Load() != 0 || !expectedSet.Load() || reader.callCountV1() != 128 {
		t.Fatalf("stable window failures=%d expected=%t reads=%d", failures.Load(), expectedSet.Load(), reader.callCountV1())
	}
}

func TestContextOwnerBindingAdapter64ConcurrentFlipWindowFailsClosedV1(t *testing.T) {
	fixture := newAdapterFixtureV1(t)
	next := fixture.lineage
	next.Frame.ID += "-next"
	next.Frame.Revision++
	next.Frame.Digest = contextDigestV1("frame-next-concurrent")
	next = resealContextLineageV1(t, next, fixture.now)
	reader := newFlipBarrierReaderV1(fixture.lineage, next, 64)
	adapter := newAdapterV1(t, fixture.owner, reader, fixedAdapterClockV1(fixture.now.Add(time.Second)))

	var accepted atomic.Int64
	var failures atomic.Int64
	var wait sync.WaitGroup
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			projection, err := adapter.InspectCurrentInvocationContextOwnerBindingV1(context.Background(), fixture.request)
			if err == nil || projection != (modelinvoker.InvocationContextOwnerBindingProjectionV1{}) {
				accepted.Add(1)
				return
			}
			if !errors.Is(err, contract.ErrConflict) {
				failures.Add(1)
			}
		}()
	}
	wait.Wait()
	if accepted.Load() != 0 || failures.Load() != 0 || reader.calls.Load() != 128 {
		t.Fatalf("flip window accepted=%d wrong_errors=%d reads=%d", accepted.Load(), failures.Load(), reader.calls.Load())
	}
}

type adapterFixtureV1 struct {
	now            time.Time
	owner          contract.OwnerRef
	contextRequest contract.ContextModelInputLineageCurrentRequestV1
	request        modelinvoker.InvocationContextOwnerBindingRequestV1
	lineage        contract.ContextModelInputLineageCurrentProjectionV1
}

func newAdapterFixtureV1(t *testing.T) adapterFixtureV1 {
	t.Helper()
	now := adapterNowV1
	owner := contract.OwnerRef{
		ComponentID:   "components/context",
		BindingDigest: contextDigestV1("context-owner-binding"),
	}
	material, err := contract.ContextModelInputMaterialExactSourceV1(
		owner,
		contract.ContextModelInputMaterialRefV1{
			ID: "context-model-input", Revision: 7, Digest: contextDigestV1("context-material"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := contract.ContextFrameExactSourceV1(
		owner,
		contract.FactRef{
			ID: "context-frame", Revision: 11, Digest: contextDigestV1("context-frame"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	request := modelRequestV1(t, material, now)
	contextRequest, err := contract.SealContextModelInputLineageCurrentRequestV1(
		contract.ContextModelInputLineageCurrentRequestV1{
			Source: material, CheckedUnixNano: request.CheckedUnixNano, NotAfterUnixNano: request.NotAfterUnixNano,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	lineage := resealContextLineageV1(t, contract.ContextModelInputLineageCurrentProjectionV1{
		Material: material, Frame: frame, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(10 * time.Second).UnixNano(),
	}, now)
	if lineage.Digest == owner.BindingDigest || lineage.Digest == material.Digest || lineage.Digest == frame.Digest {
		t.Fatal("fixture digest roles unexpectedly collided")
	}
	return adapterFixtureV1{now: now, owner: owner, contextRequest: contextRequest, request: request, lineage: lineage}
}

func modelRequestV1(
	t *testing.T,
	material contract.ContextInvocationExactSourceRefV1,
	now time.Time,
) modelinvoker.InvocationContextOwnerBindingRequestV1 {
	t.Helper()
	request, err := modelinvoker.SealInvocationContextOwnerBindingRequestV1(
		modelinvoker.InvocationContextOwnerBindingRequestV1{
			MaterialLookup: modelinvoker.ContextMaterialLookupV1{
				Kind: string(material.Kind), ID: material.ID, Revision: core.Revision(material.Revision), Digest: core.Digest(material.Digest),
			},
			CheckedUnixNano: now.Add(-time.Second).UnixNano(), NotAfterUnixNano: now.Add(20 * time.Second).UnixNano(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func resealContextLineageV1(
	t *testing.T,
	projection contract.ContextModelInputLineageCurrentProjectionV1,
	now time.Time,
) contract.ContextModelInputLineageCurrentProjectionV1 {
	t.Helper()
	projection.Digest = ""
	sealed, err := contract.SealContextModelInputLineageCurrentProjectionV1(projection, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func newAdapterV1(
	t *testing.T,
	owner contract.OwnerRef,
	reader contextports.ContextModelInputLineageCurrentReaderV1,
	clock func() time.Time,
) *modelinvokeradapter.InvocationContextOwnerBindingAdapterV1 {
	t.Helper()
	adapter, err := modelinvokeradapter.NewInvocationContextOwnerBindingAdapterV1(owner, reader, clock)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func contextDigestV1(value string) contract.Digest {
	return contract.DigestBytes([]byte(value))
}

func fixedAdapterClockV1(value time.Time) func() time.Time {
	return func() time.Time { return value }
}

func scriptedAdapterClockV1(values []time.Time) func() time.Time {
	var mutex sync.Mutex
	index := 0
	return func() time.Time {
		mutex.Lock()
		defer mutex.Unlock()
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}

type scriptedLineageReaderV1 struct {
	mutex       sync.Mutex
	calls       int
	projections []contract.ContextModelInputLineageCurrentProjectionV1
	errs        []error
	requests    []contract.ContextModelInputLineageCurrentRequestV1
	after       func(int)
}

func (r *scriptedLineageReaderV1) InspectContextModelInputLineageCurrentV1(
	_ context.Context,
	request contract.ContextModelInputLineageCurrentRequestV1,
) (contract.ContextModelInputLineageCurrentProjectionV1, error) {
	r.mutex.Lock()
	r.calls++
	call := r.calls
	r.requests = append(r.requests, request)
	index := call - 1
	var projection contract.ContextModelInputLineageCurrentProjectionV1
	if len(r.projections) > 0 {
		if index >= len(r.projections) {
			index = len(r.projections) - 1
		}
		projection = r.projections[index]
	}
	var err error
	if len(r.errs) > 0 {
		errIndex := call - 1
		if errIndex >= len(r.errs) {
			errIndex = len(r.errs) - 1
		}
		err = r.errs[errIndex]
	}
	after := r.after
	r.mutex.Unlock()
	if after != nil {
		after(call)
	}
	return projection, err
}

func (r *scriptedLineageReaderV1) callCountV1() int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.calls
}

func (r *scriptedLineageReaderV1) requestsV1() []contract.ContextModelInputLineageCurrentRequestV1 {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return append([]contract.ContextModelInputLineageCurrentRequestV1(nil), r.requests...)
}

type flipBarrierReaderV1 struct {
	first       contract.ContextModelInputLineageCurrentProjectionV1
	second      contract.ContextModelInputLineageCurrentProjectionV1
	firstTarget int64
	calls       atomic.Int64
	firstReady  chan struct{}
	closeOnce   sync.Once
}

func newFlipBarrierReaderV1(
	first contract.ContextModelInputLineageCurrentProjectionV1,
	second contract.ContextModelInputLineageCurrentProjectionV1,
	firstTarget int64,
) *flipBarrierReaderV1 {
	return &flipBarrierReaderV1{
		first: first, second: second, firstTarget: firstTarget, firstReady: make(chan struct{}),
	}
}

func (r *flipBarrierReaderV1) InspectContextModelInputLineageCurrentV1(
	context.Context,
	contract.ContextModelInputLineageCurrentRequestV1,
) (contract.ContextModelInputLineageCurrentProjectionV1, error) {
	call := r.calls.Add(1)
	if call <= r.firstTarget {
		if call == r.firstTarget {
			r.closeOnce.Do(func() { close(r.firstReady) })
		}
		<-r.firstReady
		return r.first, nil
	}
	return r.second, nil
}
