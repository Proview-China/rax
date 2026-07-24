package runtimeadapter_test

import (
	"context"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/runtimeadapter"
)

type v2Clock struct {
	mu  sync.RWMutex
	now time.Time
}

func (c *v2Clock) Now() time.Time  { c.mu.RLock(); defer c.mu.RUnlock(); return c.now }
func (c *v2Clock) Set(v time.Time) { c.mu.Lock(); c.now = v; c.mu.Unlock() }

type v2RouteReader struct {
	route  runtimeports.ControlledOperationProviderRouteCurrentProjectionV2
	mutate func()
	calls  atomic.Int32
}

func (r *v2RouteReader) InspectCurrentControlledOperationProviderRouteV2(_ context.Context, _ runtimeports.ControlledOperationProviderRouteCurrentRefV2, _ runtimeports.OperationScopeEvidenceApplicabilityMatrixKeyV3) (runtimeports.ControlledOperationProviderRouteCurrentProjectionV2, error) {
	r.calls.Add(1)
	if r.mutate != nil {
		r.mutate()
	}
	return r.route, nil
}

type v2Gateway struct {
	mu                 sync.Mutex
	results            map[string]runtimeports.ControlledOperationProviderResultV2
	requests           map[string]core.Digest
	status             runtimeports.ControlledOperationProviderResultStatusV2
	observation        runtimeports.ProviderAttemptObservationRefV2
	lostEnter          atomic.Bool
	raw                atomic.Bool
	enterCalls         atomic.Int32
	entryCalls         atomic.Int32
	admissions         atomic.Int32
	inspect            atomic.Int32
	inspectFail        atomic.Bool
	requireLiveContext atomic.Bool
}

func (g *v2Gateway) EnterControlledOperationProviderV2(_ context.Context, request runtimeports.ControlledOperationProviderRequestV2) (runtimeports.ControlledOperationProviderResultV2, error) {
	g.enterCalls.Add(1)
	if g.raw.Load() {
		return runtimeports.ControlledOperationProviderResultV2{}, nil
	}
	key, err := runtimeports.DeriveControlledOperationProviderEntryKeyV2(request)
	if err != nil {
		return runtimeports.ControlledOperationProviderResultV2{}, err
	}
	g.mu.Lock()
	result, ok := g.results[key.EntryID]
	if g.requests == nil {
		g.requests = make(map[string]core.Digest)
	}
	if ok && g.requests[key.EntryID] != request.RequestDigest {
		g.mu.Unlock()
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorConflict, core.ReasonIdempotencyPayloadMismatch, "same Entry key changed request")
	}
	if !ok {
		g.entryCalls.Add(1)
		if g.status == runtimeports.ControlledOperationProviderObservedV2 {
			g.admissions.Add(1)
			observation := g.observation
			result = testkit.ControlledProviderResultV2(request, g.status, &observation, testkit.FixedTime)
		} else {
			result = testkit.ControlledProviderResultV2(request, g.status, nil, testkit.FixedTime)
		}
		g.results[key.EntryID] = result
		g.requests[key.EntryID] = request.RequestDigest
	}
	g.mu.Unlock()
	if g.lostEnter.CompareAndSwap(true, false) {
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "lost Gateway reply")
	}
	return result, nil
}

func TestControlledProviderV2SameStableKeyChangedRequestConflicts(t *testing.T) {
	fixture := testkit.ControlledProviderV2(testkit.FixedTime)
	gateway := &v2Gateway{results: map[string]runtimeports.ControlledOperationProviderResultV2{}, status: runtimeports.ControlledOperationProviderObservedV2, observation: fixture.Observation}
	adapter, _ := newV2Adapter(t, fixture, &v2Clock{now: testkit.FixedTime}, gateway)
	if _, err := adapter.EnterControlledProviderV2(context.Background(), fixture.Request); err != nil {
		t.Fatal(err)
	}
	drifted := fixture.Request
	drifted.CallerDeadlineUnixNano--
	var err error
	drifted, err = runtimeports.SealControlledOperationProviderRequestV2(drifted)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.EnterControlledProviderV2(context.Background(), drifted); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("same stable key drift error=%v", err)
	}
	if gateway.entryCalls.Load() != 1 || gateway.admissions.Load() != 1 {
		t.Fatalf("entries=%d admissions=%d", gateway.entryCalls.Load(), gateway.admissions.Load())
	}
}

func (g *v2Gateway) InspectControlledOperationProviderV2(ctx context.Context, request runtimeports.ControlledOperationProviderInspectRequestV2) (runtimeports.ControlledOperationProviderResultV2, error) {
	g.inspect.Add(1)
	if g.requireLiveContext.Load() && ctx.Err() != nil {
		return runtimeports.ControlledOperationProviderResultV2{}, ctx.Err()
	}
	if g.inspectFail.Load() {
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "exact Inspect unavailable")
	}
	if err := request.Validate(); err != nil {
		return runtimeports.ControlledOperationProviderResultV2{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	result, ok := g.results[request.Key.EntryID]
	if !ok {
		return runtimeports.ControlledOperationProviderResultV2{}, core.NewError(core.ErrorNotFound, core.ReasonEvidenceUnavailable, "Entry not found")
	}
	return result, nil
}

func TestControlledProviderV2TypedNilAndNilContextFailClosed(t *testing.T) {
	fixture := testkit.ControlledProviderV2(testkit.FixedTime)
	clock := &v2Clock{now: testkit.FixedTime}
	gateway := &v2Gateway{results: map[string]runtimeports.ControlledOperationProviderResultV2{}, status: runtimeports.ControlledOperationProviderObservedV2, observation: fixture.Observation}
	var nilRoutes *v2RouteReader
	if _, err := runtimeadapter.NewControlledProviderV2(nilRoutes, gateway, clock, time.Second); err == nil {
		t.Fatal("typed-nil route reader passed constructor")
	}
	var nilGateway *v2Gateway
	if _, err := runtimeadapter.NewControlledProviderV2(&v2RouteReader{route: fixture.Route}, nilGateway, clock, time.Second); err == nil {
		t.Fatal("typed-nil Gateway passed constructor")
	}
	var nilClock *v2Clock
	if _, err := runtimeadapter.NewControlledProviderV2(&v2RouteReader{route: fixture.Route}, gateway, nilClock, time.Second); err == nil {
		t.Fatal("typed-nil clock passed constructor")
	}
	adapter, routes := newV2Adapter(t, fixture, clock, gateway)
	if _, err := adapter.EnterControlledProviderV2(nil, fixture.Request); err == nil {
		t.Fatal("nil context entered")
	}
	if _, err := adapter.InspectControlledProviderV2(nil, fixture.Request); err == nil {
		t.Fatal("nil context inspected")
	}
	if routes.calls.Load() != 0 || gateway.enterCalls.Load() != 0 || gateway.inspect.Load() != 0 {
		t.Fatalf("nil context touched dependencies: routes=%d enter=%d inspect=%d", routes.calls.Load(), gateway.enterCalls.Load(), gateway.inspect.Load())
	}
}

func TestControlledProviderV2LostReplyInspectFailurePreservesEnterAndNeverRedispatches(t *testing.T) {
	fixture := testkit.ControlledProviderV2(testkit.FixedTime)
	gateway := &v2Gateway{results: map[string]runtimeports.ControlledOperationProviderResultV2{}, status: runtimeports.ControlledOperationProviderObservedV2, observation: fixture.Observation}
	gateway.lostEnter.Store(true)
	gateway.inspectFail.Store(true)
	adapter, _ := newV2Adapter(t, fixture, &v2Clock{now: testkit.FixedTime}, gateway)
	_, err := adapter.EnterControlledProviderV2(context.Background(), fixture.Request)
	if !core.HasCategory(err, core.ErrorUnavailable) || gateway.enterCalls.Load() != 1 || gateway.inspect.Load() != 1 || gateway.entryCalls.Load() != 1 || gateway.admissions.Load() != 1 {
		t.Fatalf("err=%v enter=%d inspect=%d entry=%d admission=%d", err, gateway.enterCalls.Load(), gateway.inspect.Load(), gateway.entryCalls.Load(), gateway.admissions.Load())
	}
}

func TestControlledProviderV2UnknownRecoveryDetachesCallerCancellationWithoutRedispatch(t *testing.T) {
	fixture := testkit.ControlledProviderV2(testkit.FixedTime)
	gateway := &v2Gateway{results: map[string]runtimeports.ControlledOperationProviderResultV2{}, status: runtimeports.ControlledOperationProviderUnknownV2}
	gateway.requireLiveContext.Store(true)
	adapter, _ := newV2Adapter(t, fixture, &v2Clock{now: testkit.FixedTime}, gateway)
	ctx, cancel := context.WithCancel(context.Background())
	// Enter ignores cancellation in this public-port fake, then the Tool adapter
	// must detach only the exact Inspect recovery.
	cancel()
	result, err := adapter.EnterControlledProviderV2(ctx, fixture.Request)
	if err != nil || result.Status != runtimeports.ControlledOperationProviderUnknownV2 || gateway.enterCalls.Load() != 1 || gateway.inspect.Load() != 1 {
		t.Fatalf("result=%s err=%v enter=%d inspect=%d", result.Status, err, gateway.enterCalls.Load(), gateway.inspect.Load())
	}
}

func TestRuntimeAdapterImportBoundary(t *testing.T) {
	packages, err := parser.ParseDir(token.NewFileSet(), ".", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range packages {
		for name, file := range pkg.Files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			for _, spec := range file.Imports {
				path, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(path, "/runtime/") && path != "github.com/Proview-China/rax/ExecutionRuntime/runtime/core" && path != "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports" {
					t.Fatalf("forbidden Runtime implementation import %s", path)
				}
			}
		}
	}
}

func newV2Adapter(t *testing.T, fixture testkit.ControlledProviderFixtureV2, clock *v2Clock, gateway *v2Gateway) (*runtimeadapter.ControlledProviderV2, *v2RouteReader) {
	t.Helper()
	routes := &v2RouteReader{route: fixture.Route}
	adapter, err := runtimeadapter.NewControlledProviderV2(routes, gateway, clock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return adapter, routes
}

func TestControlledProviderV2PositiveAndLostReplyInspect(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(map[bool]string{false: "positive", true: "lost-reply"}[lost], func(t *testing.T) {
			fixture := testkit.ControlledProviderV2(testkit.FixedTime)
			gateway := &v2Gateway{results: map[string]runtimeports.ControlledOperationProviderResultV2{}, status: runtimeports.ControlledOperationProviderObservedV2, observation: fixture.Observation}
			gateway.lostEnter.Store(lost)
			adapter, _ := newV2Adapter(t, fixture, &v2Clock{now: testkit.FixedTime}, gateway)
			result, err := adapter.EnterControlledProviderV2(context.Background(), fixture.Request)
			if err != nil || result.Status != runtimeports.ControlledOperationProviderObservedV2 {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if gateway.entryCalls.Load() != 1 || gateway.admissions.Load() != 1 {
				t.Fatalf("entries=%d admissions=%d", gateway.entryCalls.Load(), gateway.admissions.Load())
			}
			if lost && gateway.inspect.Load() != 1 {
				t.Fatalf("lost reply inspect=%d", gateway.inspect.Load())
			}
		})
	}
}

func TestControlledProviderV2RouteSevenBindingDriftAndAliasFailClosed(t *testing.T) {
	fixture := testkit.ControlledProviderV2(testkit.FixedTime)
	fields := []func(*runtimeports.ControlledOperationProviderRouteCurrentProjectionV2){
		func(v *runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) {
			v.ToolAdapterBinding.ComponentID = "praxis.tool/drift"
		},
		func(v *runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) {
			v.GatewayBinding.ComponentID = "praxis.runtime/drift"
		},
		func(v *runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) {
			v.ProviderTransportBinding.ComponentID = "praxis.tool/drift"
		},
		func(v *runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) {
			v.PreparedReaderBinding.ComponentID = "praxis.runtime/drift"
		},
		func(v *runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) {
			v.BoundaryReaderBinding.ComponentID = "praxis.runtime/drift"
		},
		func(v *runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) {
			v.ProviderInspectBinding.ComponentID = "praxis.runtime/drift"
		},
		func(v *runtimeports.ControlledOperationProviderRouteCurrentProjectionV2) {
			v.ProviderBinding.ComponentID = "praxis.tool/drift"
		},
	}
	for index, drift := range fields {
		t.Run(string(rune('a'+index)), func(t *testing.T) {
			changed := fixture
			drift(&changed.Route)
			gateway := &v2Gateway{results: map[string]runtimeports.ControlledOperationProviderResultV2{}, status: runtimeports.ControlledOperationProviderObservedV2, observation: fixture.Observation}
			adapter, _ := newV2Adapter(t, changed, &v2Clock{now: testkit.FixedTime}, gateway)
			if _, err := adapter.EnterControlledProviderV2(context.Background(), fixture.Request); err == nil {
				t.Fatal("binding drift reached Gateway")
			}
			if gateway.enterCalls.Load() != 0 {
				t.Fatalf("Gateway calls=%d", gateway.enterCalls.Load())
			}
		})
	}
	t.Run("transport-provider-alias", func(t *testing.T) {
		changed := fixture
		changed.Route.ProviderTransportBinding.ComponentID = changed.Route.ProviderBinding.ComponentID
		changed.Route.ProviderTransportBinding.ManifestDigest = testkit.Digest("alias-transport-manifest")
		changed.Route.ProviderTransportBinding.ArtifactDigest = testkit.Digest("alias-transport-artifact")
		changed.Route, _ = runtimeports.SealControlledOperationProviderRouteCurrentProjectionV2(changed.Route)
		// Preserve the request exact ref so this is also a same-ID changed-content attack.
		changed.Route.Ref = fixture.Route.Ref
		gateway := &v2Gateway{results: map[string]runtimeports.ControlledOperationProviderResultV2{}, status: runtimeports.ControlledOperationProviderObservedV2, observation: fixture.Observation}
		adapter, _ := newV2Adapter(t, changed, &v2Clock{now: testkit.FixedTime}, gateway)
		if _, err := adapter.EnterControlledProviderV2(context.Background(), fixture.Request); err == nil || gateway.enterCalls.Load() != 0 {
			t.Fatalf("alias err=%v calls=%d", err, gateway.enterCalls.Load())
		}
	})
}

func TestControlledProviderV2TTLClockRawUnknownAndConcurrency(t *testing.T) {
	t.Run("route-ttl-crossing", func(t *testing.T) {
		fixture := testkit.ControlledProviderV2(testkit.FixedTime)
		clock := &v2Clock{now: testkit.FixedTime}
		gateway := &v2Gateway{results: map[string]runtimeports.ControlledOperationProviderResultV2{}, status: runtimeports.ControlledOperationProviderObservedV2, observation: fixture.Observation}
		adapter, routes := newV2Adapter(t, fixture, clock, gateway)
		routes.mutate = func() { clock.Set(testkit.FixedTime.Add(7 * time.Second)) }
		if _, err := adapter.EnterControlledProviderV2(context.Background(), fixture.Request); err == nil || gateway.enterCalls.Load() != 0 {
			t.Fatalf("TTL err=%v calls=%d", err, gateway.enterCalls.Load())
		}
	})
	t.Run("clock-rollback", func(t *testing.T) {
		fixture := testkit.ControlledProviderV2(testkit.FixedTime)
		clock := &v2Clock{now: testkit.FixedTime}
		gateway := &v2Gateway{results: map[string]runtimeports.ControlledOperationProviderResultV2{}, status: runtimeports.ControlledOperationProviderObservedV2, observation: fixture.Observation}
		adapter, routes := newV2Adapter(t, fixture, clock, gateway)
		routes.mutate = func() { clock.Set(testkit.FixedTime.Add(-time.Second)) }
		if _, err := adapter.EnterControlledProviderV2(context.Background(), fixture.Request); !core.HasReason(err, core.ReasonClockRegression) || gateway.enterCalls.Load() != 0 {
			t.Fatalf("rollback err=%v calls=%d", err, gateway.enterCalls.Load())
		}
	})
	t.Run("raw-result-is-not-observation", func(t *testing.T) {
		fixture := testkit.ControlledProviderV2(testkit.FixedTime)
		gateway := &v2Gateway{results: map[string]runtimeports.ControlledOperationProviderResultV2{}}
		gateway.raw.Store(true)
		adapter, _ := newV2Adapter(t, fixture, &v2Clock{now: testkit.FixedTime}, gateway)
		if _, err := adapter.EnterControlledProviderV2(context.Background(), fixture.Request); err == nil {
			t.Fatal("raw zero result was upgraded")
		}
	})
	t.Run("unknown-is-inspect-only", func(t *testing.T) {
		fixture := testkit.ControlledProviderV2(testkit.FixedTime)
		gateway := &v2Gateway{results: map[string]runtimeports.ControlledOperationProviderResultV2{}, status: runtimeports.ControlledOperationProviderUnknownV2}
		adapter, _ := newV2Adapter(t, fixture, &v2Clock{now: testkit.FixedTime}, gateway)
		result, err := adapter.EnterControlledProviderV2(context.Background(), fixture.Request)
		if err != nil || result.Status != runtimeports.ControlledOperationProviderUnknownV2 || gateway.enterCalls.Load() != 1 || gateway.inspect.Load() != 1 || gateway.admissions.Load() != 0 {
			t.Fatalf("result=%s err=%v enter=%d inspect=%d admission=%d", result.Status, err, gateway.enterCalls.Load(), gateway.inspect.Load(), gateway.admissions.Load())
		}
	})
	t.Run("64-same-key", func(t *testing.T) {
		fixture := testkit.ControlledProviderV2(testkit.FixedTime)
		gateway := &v2Gateway{results: map[string]runtimeports.ControlledOperationProviderResultV2{}, status: runtimeports.ControlledOperationProviderObservedV2, observation: fixture.Observation}
		adapter, _ := newV2Adapter(t, fixture, &v2Clock{now: testkit.FixedTime}, gateway)
		var wg sync.WaitGroup
		errs := make(chan error, 64)
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := adapter.EnterControlledProviderV2(context.Background(), fixture.Request)
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
		if gateway.entryCalls.Load() != 1 || gateway.admissions.Load() != 1 {
			t.Fatalf("entries=%d admissions=%d", gateway.entryCalls.Load(), gateway.admissions.Load())
		}
	})
}
