package routegateway_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
	modelsqlite "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestGovernedModelInvocationActualPointReplayAndExactHistory(t *testing.T) {
	fixture := newGovernedFixtureV1(t, nil)
	defer fixture.gateway.Close()
	result, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if err != nil || result.Observation == nil || result.Invocation.State != modelinvoker.GovernedModelInvocationObservedV1 {
		t.Fatalf("governed invoke = %#v, %v", result, err)
	}
	if fixture.state.invoke.Load() != 1 || fixture.gate.commit.Load() != 1 || fixture.gate.inspect.Load() != 2 {
		t.Fatalf("provider/commit/inspect = %d/%d/%d", fixture.state.invoke.Load(), fixture.gate.commit.Load(), fixture.gate.inspect.Load())
	}
	if string(result.Observation.StructuredOutput) != `{"decision":"pass"}` || result.Observation.ResponseID != "response-governed-v1" {
		t.Fatalf("provider-neutral Observation = %#v", result.Observation)
	}
	// Raw/native/provider metadata have no field in the sealed Observation.
	response, err := result.Observation.ResponseV1()
	if err != nil || !response.RawResponse.Empty() || len(response.NativeEvents) != 0 || len(response.ProviderMetadata) != 0 {
		t.Fatalf("sanitized response = %#v, %v", response, err)
	}
	replayed, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if err != nil || replayed.Invocation.RefV1() != result.Invocation.RefV1() || fixture.state.invoke.Load() != 1 {
		t.Fatalf("replay = %#v, %v provider=%d", replayed, err, fixture.state.invoke.Load())
	}
	for revision := core.Revision(1); revision <= 3; revision++ {
		ref := result.Invocation.RefV1()
		ref.Revision = revision
		fact, inspectErr := fixture.repository.InspectCurrentGovernedModelInvocationV1(context.Background(), ref.ID)
		if inspectErr != nil {
			t.Fatal(inspectErr)
		}
		if revision == 1 {
			ref.Digest = fixture.preparedDigest
		}
		if revision == 2 {
			ref.Digest = result.Observation.InvocationRef.Digest
		}
		if revision == 3 {
			ref.Digest = fact.Digest
		}
		if _, inspectErr = fixture.gateway.InspectExactModelInvocationV1(context.Background(), ref); inspectErr != nil {
			t.Fatalf("exact revision %d: %v", revision, inspectErr)
		}
	}
}

func TestGovernedModelInvocationGateFailurePrecedesRouteAndProviderPreparation(t *testing.T) {
	fixture := newGovernedFixtureV1(t, nil)
	defer fixture.gateway.Close()
	fixture.gate.commitErr = core.NewError(core.ErrorUnavailable, core.ReasonComponentMissing, "Commit Gate unavailable")

	if _, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command); err == nil {
		t.Fatal("gate failure was accepted")
	}
	state := fixture.state.snapshot()
	for _, name := range []string{"binding", "secret", "factory", "capabilities", "invoke", "stream"} {
		if state[name] != 0 {
			t.Fatalf("%s calls = %d before a current ACK", name, state[name])
		}
	}
}

func TestGovernedModelInvocationDelayedCanonicalReplayDoesNotChangePreparedFact(t *testing.T) {
	var clock atomic.Int64
	clock.Store(gatewayNow.UnixNano())
	fixture := newGovernedFixtureV1(t, nil, routegateway.WithClock(func() time.Time { return time.Unix(0, clock.Load()) }))
	defer fixture.gateway.Close()
	first, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if err != nil || first.Observation == nil {
		t.Fatalf("first = %#v, %v", first, err)
	}
	clock.Store(gatewayNow.Add(time.Second).UnixNano())
	replayed, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if err != nil || replayed.Invocation.RefV1() != first.Invocation.RefV1() || fixture.state.invoke.Load() != 1 {
		t.Fatalf("delayed replay = %#v, %v provider=%d", replayed, err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelInvocationConcurrentCallersOnlyOneProvider(t *testing.T) {
	fixture := newGovernedFixtureV1(t, nil)
	defer fixture.gateway.Close()
	const workers = 64
	start := make(chan struct{})
	var wait sync.WaitGroup
	var successes atomic.Uint64
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
			if err == nil && result.Observation != nil {
				successes.Add(1)
			}
		}()
	}
	close(start)
	wait.Wait()
	if fixture.state.invoke.Load() != 1 || successes.Load() == 0 {
		t.Fatalf("provider=%d observed callers=%d", fixture.state.invoke.Load(), successes.Load())
	}
}

func TestGovernedModelInvocationBoundaryLostReplyNeverCallsProvider(t *testing.T) {
	base := modelinvoker.NewInMemoryGovernedModelInvocationStoreV1()
	repository := &lostBoundaryReplyRepositoryV1{inner: base}
	fixture := newGovernedFixtureV1(t, repository)
	defer fixture.gateway.Close()
	result, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate || result.Invocation.State != modelinvoker.GovernedModelInvocationProviderBoundaryCrossedV1 || fixture.state.invoke.Load() != 0 {
		t.Fatalf("lost boundary reply = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
	if _, err = fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate || fixture.state.invoke.Load() != 0 {
		t.Fatalf("lost-reply replay called provider: %v provider=%d", err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelInvocationFreshClockExpiryFailsBeforeProvider(t *testing.T) {
	var clock atomic.Int64
	clock.Store(gatewayNow.UnixNano())
	fixture := newGovernedFixtureV1(t, nil, routegateway.WithClock(func() time.Time { return time.Unix(0, clock.Load()) }))
	fixture.gate.afterInspect = func() { clock.Store(fixture.ack.ExpiresUnixNano) }
	defer fixture.gateway.Close()
	if _, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command); err == nil || fixture.state.invoke.Load() != 0 {
		t.Fatalf("expired actual point = %v provider=%d", err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelInvocationClockRollbackFailsBeforeProvider(t *testing.T) {
	var clock atomic.Int64
	clock.Store(gatewayNow.UnixNano())
	fixture := newGovernedFixtureV1(t, nil, routegateway.WithClock(func() time.Time { return time.Unix(0, clock.Load()) }))
	fixture.gate.afterInspect = func() { clock.Store(gatewayNow.Add(-time.Nanosecond).UnixNano()) }
	defer fixture.gateway.Close()
	if _, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict || fixture.state.invoke.Load() != 0 {
		t.Fatalf("rollback actual point = %v provider=%d", err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelInvocationPhysicalAckDriftAfterBoundaryCallsNoProvider(t *testing.T) {
	fixture := newGovernedFixtureV1(t, nil)
	fixture.gate.afterInspect = func() {
		if fixture.gate.inspect.Load() == 2 {
			fixture.gate.ack.Digest = core.DigestBytes([]byte("drifted-ack"))
		}
	}
	defer fixture.gateway.Close()
	result, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if err == nil || result.Invocation.State != modelinvoker.GovernedModelInvocationRejectedNoEffectV1 || fixture.state.invoke.Load() != 0 {
		t.Fatalf("physical ACK drift = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
	replayed, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict || replayed.Invocation.RefV1() != result.Invocation.RefV1() || fixture.state.invoke.Load() != 0 {
		t.Fatalf("physical ACK drift replay = %#v, %v provider=%d", replayed, err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelInvocationTTLExpiryDuringPhysicalRereadCallsNoProvider(t *testing.T) {
	var clock atomic.Int64
	clock.Store(gatewayNow.UnixNano())
	fixture := newGovernedFixtureV1(t, nil, routegateway.WithClock(func() time.Time { return time.Unix(0, clock.Load()) }))
	fixture.gate.afterInspect = func() {
		if fixture.gate.inspect.Load() == 2 {
			clock.Store(fixture.ack.ExpiresUnixNano)
		}
	}
	defer fixture.gateway.Close()
	result, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict || result.Invocation.State != modelinvoker.GovernedModelInvocationRejectedNoEffectV1 || fixture.state.invoke.Load() != 0 {
		t.Fatalf("physical reread TTL crossing = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelInvocationClockRollbackAfterProviderIsUnknown(t *testing.T) {
	var clock atomic.Int64
	clock.Store(gatewayNow.UnixNano())
	fixture := newGovernedFixtureV1(t, nil, routegateway.WithClock(func() time.Time { return time.Unix(0, clock.Load()) }))
	fixture.state.beforeInvokeReturn = func() { clock.Store(gatewayNow.Add(-time.Nanosecond).UnixNano()) }
	defer fixture.gateway.Close()
	result, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if err == nil || result.Invocation.State != modelinvoker.GovernedModelInvocationUnknownV1 || result.Observation != nil || fixture.state.invoke.Load() != 1 {
		t.Fatalf("post-provider rollback = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
	replayed, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate || replayed.Invocation.RefV1() != result.Invocation.RefV1() || fixture.state.invoke.Load() != 1 {
		t.Fatalf("post-provider rollback replay = %#v, %v provider=%d", replayed, err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelInvocationChangedRouteSameAttemptConflicts(t *testing.T) {
	fixture := newGovernedFixtureV1(t, nil)
	defer fixture.gateway.Close()
	if _, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command); err != nil {
		t.Fatal(err)
	}
	changed := fixture.command
	changed.Call.Request.Input = []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "changed review candidate")}
	if _, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), changed); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict || fixture.state.invoke.Load() != 1 {
		t.Fatalf("changed same attempt = %v provider=%d", err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelInvocationInvalidProviderObservationBecomesInspectOnlyUnknown(t *testing.T) {
	fixture := newGovernedFixtureV1(t, nil)
	fixture.state.missingModel.Store(true)
	defer fixture.gateway.Close()
	result, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if err == nil || result.Invocation.State != modelinvoker.GovernedModelInvocationUnknownV1 || fixture.state.invoke.Load() != 1 {
		t.Fatalf("invalid Observation = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
	result, err = fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate || result.Invocation.State != modelinvoker.GovernedModelInvocationUnknownV1 || fixture.state.invoke.Load() != 1 {
		t.Fatalf("unknown replay = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelInvocationSchemaMismatchBecomesInspectOnlyUnknown(t *testing.T) {
	fixture := newGovernedFixtureV1(t, nil)
	fixture.command.Call.Request.Output.Schema = []byte(`{"type":"object","properties":{"decision":{"type":"integer"}},"required":["decision"],"additionalProperties":false}`)
	defer fixture.gateway.Close()
	result, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if err == nil || result.Invocation.State != modelinvoker.GovernedModelInvocationUnknownV1 || result.Observation != nil || fixture.state.invoke.Load() != 1 {
		t.Fatalf("schema mismatch = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
	replayed, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate || replayed.Invocation.RefV1() != result.Invocation.RefV1() || fixture.state.invoke.Load() != 1 {
		t.Fatalf("schema mismatch replay = %#v, %v provider=%d", replayed, err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelInvocationObservedCASLostReplyRecoversExactWithoutRedispatch(t *testing.T) {
	repository := &lostTerminalReplyRepositoryV1{inner: modelinvoker.NewInMemoryGovernedModelInvocationStoreV1()}
	fixture := newGovernedFixtureV1(t, repository)
	defer fixture.gateway.Close()
	result, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if err != nil || result.Invocation.State != modelinvoker.GovernedModelInvocationObservedV1 || result.Observation == nil || fixture.state.invoke.Load() != 1 {
		t.Fatalf("lost terminal recovery = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
	replayed, err := fixture.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), fixture.command)
	if err != nil || replayed.Invocation.RefV1() != result.Invocation.RefV1() || fixture.state.invoke.Load() != 1 {
		t.Fatalf("lost terminal replay = %#v, %v provider=%d", replayed, err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelInvocationSQLiteRestartUnknownIsInspectOnly(t *testing.T) {
	path := t.TempDir() + "/governed.db"
	store, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	first := newGovernedFixtureV1(t, store)
	first.state.missingModel.Store(true)
	result, err := first.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), first.command)
	if err == nil || result.Invocation.State != modelinvoker.GovernedModelInvocationUnknownV1 || first.state.invoke.Load() != 1 {
		t.Fatalf("first unknown = %#v, %v provider=%d", result, err, first.state.invoke.Load())
	}
	if err := first.gateway.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	second := newGovernedFixtureV1(t, store, routegateway.WithClock(func() time.Time { return gatewayNow.Add(time.Second) }))
	defer second.gateway.Close()
	result, err = second.gateway.StartOrInspectGovernedModelInvocationV1(context.Background(), second.command)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate || result.Invocation.State != modelinvoker.GovernedModelInvocationUnknownV1 || second.state.invoke.Load() != 0 {
		t.Fatalf("restart replay = %#v, %v provider=%d", result, err, second.state.invoke.Load())
	}
	exact, err := second.gateway.InspectExactModelInvocationV1(context.Background(), result.Invocation.RefV1())
	if err != nil || exact.Invocation.RefV1() != result.Invocation.RefV1() {
		t.Fatalf("restart exact Inspect = %#v, %v", exact, err)
	}
}

type governedFixtureV1 struct {
	gateway        *routegateway.Gateway
	command        modelinvoker.GovernedModelInvocationCommandV1
	repository     modelinvoker.GovernedModelInvocationRepositoryV1
	state          *callState
	gate           *governedGateV1
	ack            modelinvoker.PreparedModelInvocationCommitAckV1
	preparedDigest core.Digest
}

func newGovernedFixtureV1(t *testing.T, repository modelinvoker.GovernedModelInvocationRepositoryV1, options ...routegateway.Option) governedFixtureV1 {
	t.Helper()
	call := governedCallV1()
	requestDigest := core.DigestBytes([]byte("unified-request-governed-v1"))
	prepared, current, ack := governedPreparedFixtureV1(t, requestDigest)
	preparedStore := modelinvoker.NewInMemoryPreparedModelInvocationStoreV1()
	if _, err := preparedStore.EnsurePreparedModelInvocationV1(context.Background(), prepared); err != nil {
		t.Fatal(err)
	}
	if _, err := preparedStore.EnsurePreparedModelInvocationCurrentV1(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	if repository == nil {
		repository = modelinvoker.NewInMemoryGovernedModelInvocationStoreV1()
	}
	gate := &governedGateV1{ack: ack}
	state := &callState{}
	governedOption := routegateway.WithGovernedModelInvocationsV1(routegateway.GovernedModelInvocationDependenciesV1{PreparedHistory: preparedStore, PreparedCurrent: preparedStore, CommitGate: gate, Invocations: repository})
	options = append(options, governedOption)
	gateway := fakeGateway(t, defaultCatalog(t), countingBinding{state: state}, countingSecret{state: state, version: "v1"}, state, options...)
	routeDigest, err := modelinvoker.DigestGovernedRouteCallV1(call)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := modelinvoker.NewPreparedGovernedModelInvocationForGatewayV1(modelinvoker.GovernedModelInvocationCommandV1{PreparedRef: prepared.Ref(), CurrentRef: current.Ref(), AttemptRequestDigest: requestDigest, DispatchSequence: 1, ProviderAttemptOrdinal: 1, Call: call}, routeDigest, gatewayNow)
	if err != nil {
		t.Fatal(err)
	}
	return governedFixtureV1{gateway: gateway, command: modelinvoker.GovernedModelInvocationCommandV1{PreparedRef: prepared.Ref(), CurrentRef: current.Ref(), AttemptRequestDigest: requestDigest, DispatchSequence: 1, ProviderAttemptOrdinal: 1, Call: call}, repository: repository, state: state, gate: gate, ack: ack, preparedDigest: initial.Digest}
}

func governedCallV1() modelinvoker.RouteCall {
	strict := true
	return modelinvoker.RouteCall{RouteID: "openai.direct.payg.responses", Invocation: generalInvocation(), Request: modelinvoker.Request{Model: "gpt-5.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "review exact candidate")}, ToolChoice: modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceNone}, Output: modelinvoker.OutputConstraint{Type: modelinvoker.OutputJSONSchema, Name: "review_result", Schema: []byte(`{"type":"object","properties":{"decision":{"type":"string"}},"required":["decision"],"additionalProperties":false}`), Strict: &strict}}}
}

func governedPreparedFixtureV1(t *testing.T, requestDigest core.Digest) (modelinvoker.PreparedModelInvocationFactV1, modelinvoker.PreparedModelInvocationCurrentProjectionV1, modelinvoker.PreparedModelInvocationCommitAckV1) {
	t.Helper()
	owner := func(domain, id string) core.OwnerRef { return core.OwnerRef{Domain: domain, ID: core.OwnerID(id)} }
	digest := func(label string) core.Digest { return core.DigestBytes([]byte(label)) }
	prepared, err := modelinvoker.SealPreparedModelInvocationFactV1(modelinvoker.PreparedModelInvocationFactV1{InvocationID: "execution-governed-v1", InvocationDigest: requestDigest, UnifiedRequestDigest: requestDigest, RequestToolsDigest: digest("no-tools"), PreparedPlanDigest: digest("plan"), RouteDigest: digest("route"), ProfileDigest: digest("profile"), ActualToolSurfaceDigest: digest("empty-tool-surface"), ActualProviderInjectionDigest: digest("provider-injection"), CapabilitySnapshotRef: modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{ContractVersion: "1.0.0", ID: "capability", Revision: 1, Digest: digest("capability")}, RegistrySnapshotRef: runtimeports.RegistrySnapshotRefV1{Owner: owner("registry", "owner"), ContractVersion: "1.0.0", ID: "registry", Revision: 1, Digest: digest("registry")}, CreatedUnixNano: gatewayNow.Add(-2 * time.Minute).UnixNano(), NotAfterUnixNano: gatewayNow.Add(10 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(modelinvoker.PreparedModelInvocationCurrentProjectionV1{Prepared: prepared.Ref(), CapabilitySnapshotRef: prepared.CapabilitySnapshotRef, RegistrySnapshotRef: prepared.RegistrySnapshotRef, ActualToolSurfaceDigest: prepared.ActualToolSurfaceDigest, ActualProviderInjectionDigest: prepared.ActualProviderInjectionDigest, CheckedUnixNano: gatewayNow.Add(-90 * time.Second).UnixNano(), ExpiresUnixNano: gatewayNow.Add(8 * time.Minute).UnixNano(), NotAfterUnixNano: prepared.NotAfterUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	ack, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(modelinvoker.PreparedModelInvocationCommitAckV1{PreparedRef: prepared.Ref(), CurrentRef: current.Ref(), GateImplementationRef: modelinvoker.PreparedModelInvocationGateImplementationRefV1{Owner: owner("host", "gate"), ContractVersion: "1.0.0", ID: "gate", Revision: 1, Digest: digest("gate")}, SurfaceBindingRef: modelinvoker.PreparedModelInvocationSurfaceBindingRefV1{Owner: owner("host", "surface"), ContractVersion: "1.0.0", ID: "surface", Revision: 1, Digest: digest("surface")}, CheckedUnixNano: gatewayNow.Add(-time.Minute).UnixNano(), ExpiresUnixNano: gatewayNow.Add(7 * time.Minute).UnixNano(), NotAfterUnixNano: prepared.NotAfterUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	return prepared, current, ack
}

type governedGateV1 struct {
	ack             modelinvoker.PreparedModelInvocationCommitAckV1
	commit, inspect atomic.Uint64
	afterInspect    func()
	commitErr       error
}

func (g *governedGateV1) Commit(_ context.Context, prepared modelinvoker.PreparedModelInvocationRefV1, current modelinvoker.PreparedModelInvocationCurrentRefV1) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	g.commit.Add(1)
	if g.commitErr != nil {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, g.commitErr
	}
	if g.ack.PreparedRef != prepared || g.ack.CurrentRef != current {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, errors.New("gate lineage drift")
	}
	return g.ack, nil
}
func (g *governedGateV1) InspectExactAck(_ context.Context, ref modelinvoker.PreparedModelInvocationCommitAckRefV1) (modelinvoker.PreparedModelInvocationCommitAckV1, error) {
	g.inspect.Add(1)
	if g.afterInspect != nil {
		g.afterInspect()
	}
	if g.ack.Ref() != ref {
		return modelinvoker.PreparedModelInvocationCommitAckV1{}, errors.New("ack absent")
	}
	return g.ack, nil
}

type lostBoundaryReplyRepositoryV1 struct {
	inner modelinvoker.GovernedModelInvocationRepositoryV1
	once  atomic.Bool
}

type lostTerminalReplyRepositoryV1 struct {
	inner modelinvoker.GovernedModelInvocationRepositoryV1
	once  atomic.Bool
}

func (r *lostBoundaryReplyRepositoryV1) CreateGovernedModelInvocationV1(ctx context.Context, fact modelinvoker.GovernedModelInvocationFactV1) (modelinvoker.GovernedModelInvocationMutationV1, error) {
	return r.inner.CreateGovernedModelInvocationV1(ctx, fact)
}
func (r *lostBoundaryReplyRepositoryV1) CompareAndSwapGovernedModelInvocationV1(ctx context.Context, request modelinvoker.GovernedModelInvocationCASV1) (modelinvoker.GovernedModelInvocationMutationV1, error) {
	mutation, err := r.inner.CompareAndSwapGovernedModelInvocationV1(ctx, request)
	if err == nil && request.Next.State == modelinvoker.GovernedModelInvocationProviderBoundaryCrossedV1 && r.once.CompareAndSwap(false, true) {
		return modelinvoker.GovernedModelInvocationMutationV1{}, &modelinvoker.GovernedModelInvocationErrorV1{Kind: modelinvoker.GovernedModelInvocationErrorIndeterminate, Operation: "cas", Message: "lost boundary reply"}
	}
	return mutation, err
}
func (r *lostBoundaryReplyRepositoryV1) InspectExactGovernedModelInvocationV1(ctx context.Context, ref modelinvoker.GovernedModelInvocationRefV1) (modelinvoker.GovernedModelInvocationFactV1, error) {
	return r.inner.InspectExactGovernedModelInvocationV1(ctx, ref)
}
func (r *lostBoundaryReplyRepositoryV1) InspectCurrentGovernedModelInvocationV1(ctx context.Context, id string) (modelinvoker.GovernedModelInvocationFactV1, error) {
	return r.inner.InspectCurrentGovernedModelInvocationV1(ctx, id)
}

func (r *lostTerminalReplyRepositoryV1) CreateGovernedModelInvocationV1(ctx context.Context, fact modelinvoker.GovernedModelInvocationFactV1) (modelinvoker.GovernedModelInvocationMutationV1, error) {
	return r.inner.CreateGovernedModelInvocationV1(ctx, fact)
}
func (r *lostTerminalReplyRepositoryV1) CompareAndSwapGovernedModelInvocationV1(ctx context.Context, request modelinvoker.GovernedModelInvocationCASV1) (modelinvoker.GovernedModelInvocationMutationV1, error) {
	mutation, err := r.inner.CompareAndSwapGovernedModelInvocationV1(ctx, request)
	if err == nil && request.Next.State == modelinvoker.GovernedModelInvocationObservedV1 && r.once.CompareAndSwap(false, true) {
		return modelinvoker.GovernedModelInvocationMutationV1{}, &modelinvoker.GovernedModelInvocationErrorV1{Kind: modelinvoker.GovernedModelInvocationErrorIndeterminate, Operation: "cas", Message: "lost terminal reply"}
	}
	return mutation, err
}
func (r *lostTerminalReplyRepositoryV1) InspectExactGovernedModelInvocationV1(ctx context.Context, ref modelinvoker.GovernedModelInvocationRefV1) (modelinvoker.GovernedModelInvocationFactV1, error) {
	return r.inner.InspectExactGovernedModelInvocationV1(ctx, ref)
}
func (r *lostTerminalReplyRepositoryV1) InspectCurrentGovernedModelInvocationV1(ctx context.Context, id string) (modelinvoker.GovernedModelInvocationFactV1, error) {
	return r.inner.InspectCurrentGovernedModelInvocationV1(ctx, id)
}

func ExampleGovernedModelInvocationPortV1() {
	fmt.Println("provider-neutral governed StartOrInspect + exact Inspect")
}
