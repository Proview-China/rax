package routegateway_test

import (
	"context"
	"reflect"
	"strings"
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

func TestGovernedModelTurnV2ToolCallIsAtomicAndRestartReadable(t *testing.T) {
	path := t.TempDir() + "/turn-v2.db"
	fixture := newGovernedFixtureV2(t, path, nil)
	result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || result.State != modelinvoker.GovernedModelTurnObservedV2 || result.Observation == nil || result.Observation.ToolCallProjection == nil {
		t.Fatalf("governed model turn = %#v, %v", result, err)
	}
	if fixture.state.invoke.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", fixture.state.invoke.Load())
	}
	projectionRef := result.Observation.ToolCallProjection.Ref
	if projectionRef.InvocationID != fixture.prepared.InvocationID || projectionRef.InvocationDigest != fixture.prepared.InvocationDigest {
		t.Fatalf("projection lineage = %#v, prepared = %#v", projectionRef, fixture.prepared.Ref())
	}
	if err := fixture.gateway.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exact, err := store.InspectExactGovernedModelTurnV2(context.Background(), result.RefV2())
	if err != nil || exact.RefV2() != result.RefV2() {
		t.Fatalf("restart exact turn = %#v, %v", exact, err)
	}
	projection, err := store.InspectExactGovernedModelTurnToolCallProjectionV2(context.Background(), projectionRef)
	if err != nil || projection.Ref != projectionRef {
		t.Fatalf("restart exact projection = %#v, %v", projection, err)
	}
}

func TestGovernedModelTurnV2ConcurrentCallersOnlyOneProvider(t *testing.T) {
	fixture := newGovernedFixtureV2(t, t.TempDir()+"/turn-v2.db", nil)
	defer fixture.close(t)
	const workers = 64
	start := make(chan struct{})
	var wait sync.WaitGroup
	var observed atomic.Uint64
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
			if err == nil && result.State == modelinvoker.GovernedModelTurnObservedV2 {
				observed.Add(1)
			}
		}()
	}
	close(start)
	wait.Wait()
	if fixture.state.invoke.Load() != 1 || observed.Load() == 0 {
		t.Fatalf("provider calls/observed callers = %d/%d, want 1/>0", fixture.state.invoke.Load(), observed.Load())
	}
	current, err := fixture.store.InspectCurrentGovernedModelTurnV2(context.Background(), fixture.turnID)
	if err != nil || current.State != modelinvoker.GovernedModelTurnObservedV2 {
		t.Fatalf("current = %#v, %v", current, err)
	}
}

func TestGovernedModelTurnV2RequestRouteDigestSpliceCallsNoProvider(t *testing.T) {
	fixture := newGovernedFixtureV2(t, t.TempDir()+"/turn-v2.db", nil)
	defer fixture.close(t)
	spliced := fixture.command
	spliced.AttemptRequestDigest, spliced.RouteCallDigest = spliced.RouteCallDigest, spliced.AttemptRequestDigest
	if _, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), spliced); err == nil {
		t.Fatal("request/route digest splice was accepted")
	}
	if fixture.state.invoke.Load() != 0 {
		t.Fatalf("provider calls after digest splice = %d, want 0", fixture.state.invoke.Load())
	}
}

func TestGovernedModelTurnV2OrdinaryCASCannotBypassAtomicToolProjection(t *testing.T) {
	path := t.TempDir() + "/turn-v2.db"
	base, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	repository := &ordinaryObservedCASRepositoryV2{inner: base}
	fixture := newGovernedFixtureV2WithStore(t, base, repository, nil)
	defer fixture.close(t)
	result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err == nil || result.State != modelinvoker.GovernedModelTurnProviderBoundaryCrossedV2 || fixture.state.invoke.Load() != 1 {
		t.Fatalf("ordinary observed CAS bypass = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
	current, inspectErr := base.InspectCurrentGovernedModelTurnV2(context.Background(), fixture.turnID)
	if inspectErr != nil || current.State != modelinvoker.GovernedModelTurnProviderBoundaryCrossedV2 {
		t.Fatalf("current after rejected bypass = %#v, %v", current, inspectErr)
	}
}

func TestGovernedModelTurnV2BoundaryLostReplyIsInspectOnly(t *testing.T) {
	path := t.TempDir() + "/turn-v2.db"
	base, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	repository := &lostBoundaryReplyRepositoryV2{inner: base}
	fixture := newGovernedFixtureV2WithStore(t, base, repository, nil)
	defer fixture.close(t)
	result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate || result.State != modelinvoker.GovernedModelTurnProviderBoundaryCrossedV2 || fixture.state.invoke.Load() != 0 {
		t.Fatalf("lost boundary reply = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
	replayed, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate || replayed.RefV2() != result.RefV2() || fixture.state.invoke.Load() != 0 {
		t.Fatalf("lost boundary replay = %#v, %v provider=%d", replayed, err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelTurnV2ObservedLostReplyRecoversExact(t *testing.T) {
	path := t.TempDir() + "/turn-v2.db"
	base, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	repository := &lostObservedReplyRepositoryV2{inner: base}
	fixture := newGovernedFixtureV2WithStore(t, base, repository, nil)
	defer fixture.close(t)
	result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || result.State != modelinvoker.GovernedModelTurnObservedV2 || fixture.state.invoke.Load() != 1 {
		t.Fatalf("lost observed reply = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
	replayed, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || replayed.RefV2() != result.RefV2() || fixture.state.invoke.Load() != 1 {
		t.Fatalf("observed replay = %#v, %v provider=%d", replayed, err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelTurnV2ProviderInvalidOutputsNeverPublishCandidate(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*callState)
	}{
		{"unknown tool", func(state *callState) { state.governedV2UnknownTool.Store(true) }},
		{"multiple calls", func(state *callState) { state.governedV2MultipleCalls.Store(true) }},
		{"text and call", func(state *callState) { state.governedV2TextAndCall.Store(true) }},
		{"invalid arguments", func(state *callState) { state.governedV2InvalidArguments.Store(true) }},
		{"model drift", func(state *callState) { state.wrongModel.Store(true) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGovernedFixtureV2(t, t.TempDir()+"/turn-v2.db", nil)
			defer fixture.close(t)
			test.mutate(fixture.state)
			result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
			if modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate || result.State != modelinvoker.GovernedModelTurnUnknownV2 || result.Observation != nil || fixture.state.invoke.Load() != 1 {
				t.Fatalf("invalid provider output = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
			}
			current, inspectErr := fixture.store.InspectCurrentGovernedModelTurnV2(context.Background(), fixture.turnID)
			if inspectErr != nil || current.State != modelinvoker.GovernedModelTurnUnknownV2 {
				t.Fatalf("inspect unknown = %#v, %v", current, inspectErr)
			}
		})
	}
}

func TestGovernedModelTurnV2ExpiryAndClockRegressionCallNoProvider(t *testing.T) {
	var clock atomic.Int64
	clock.Store(gatewayNow.UnixNano())
	fixture := newGovernedFixtureV2(t, t.TempDir()+"/turn-v2.db", []routegateway.Option{
		routegateway.WithClock(func() time.Time { return time.Unix(0, clock.Load()) }),
	})
	defer fixture.close(t)
	fixture.gate.afterInspect = func() {
		if fixture.gate.inspect.Load() == 1 {
			clock.Store(fixture.ack.ExpiresUnixNano)
		}
	}
	result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err == nil || result.State != modelinvoker.GovernedModelTurnRejectedNoEffectV2 || fixture.state.invoke.Load() != 0 {
		t.Fatalf("expired S2 = %#v, %v provider=%d", result, err, fixture.state.invoke.Load())
	}
}

func TestGovernedModelTurnV2ShorterAckNarrowsExpiry(t *testing.T) {
	fixture := newGovernedFixtureV2(t, t.TempDir()+"/turn-v2.db", nil)
	defer fixture.close(t)
	result, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || result.State != modelinvoker.GovernedModelTurnObservedV2 {
		t.Fatalf("shorter ACK turn = %#v, %v", result, err)
	}
	if result.ExpiresUnixNano != fixture.ack.ExpiresUnixNano {
		t.Fatalf("turn expiry = %d, want ACK expiry %d", result.ExpiresUnixNano, fixture.ack.ExpiresUnixNano)
	}
}

func TestGovernedModelTurnV2SealersAreAtomicAndIdempotent(t *testing.T) {
	fixture := newGovernedFixtureV2(t, t.TempDir()+"/turn-v2.db", nil)
	defer fixture.close(t)
	outcome, err := fixture.gateway.StartOrInspectGovernedModelTurnV2(context.Background(), fixture.command)
	if err != nil || outcome.Observation == nil {
		t.Fatalf("observed turn = %#v, %v", outcome, err)
	}

	resealedOutcome, err := modelinvoker.SealGovernedModelTurnOutcomeV2(outcome)
	if err != nil || !reflect.DeepEqual(resealedOutcome, outcome) {
		t.Fatalf("re-sealed outcome = %#v, %v", resealedOutcome, err)
	}
	invalidOutcome := outcome
	invalidOutcome.Digest = ""
	invalidOutcome.Revision = 0
	if sealed, sealErr := modelinvoker.SealGovernedModelTurnOutcomeV2(invalidOutcome); sealErr == nil || sealed != (modelinvoker.GovernedModelTurnOutcomeV2{}) {
		t.Fatalf("invalid outcome sealer leaked partial value = %#v, %v", sealed, sealErr)
	}

	observation := *outcome.Observation
	resealedObservation, err := modelinvoker.SealGovernedModelTurnObservationV2(observation)
	if err != nil || !reflect.DeepEqual(resealedObservation, observation) {
		t.Fatalf("re-sealed observation = %#v, %v", resealedObservation, err)
	}
	invalidObservation := observation
	invalidObservation.Digest = ""
	invalidObservation.Provider = ""
	if sealed, sealErr := modelinvoker.SealGovernedModelTurnObservationV2(invalidObservation); sealErr == nil || sealed != (modelinvoker.GovernedModelTurnObservationV2{}) {
		t.Fatalf("invalid observation sealer leaked partial value = %#v, %v", sealed, sealErr)
	}

	material, err := fixture.store.InspectExactInvocationMaterialV1(context.Background(), fixture.command.MaterialRef)
	if err != nil {
		t.Fatal(err)
	}
	resealedMaterial, err := modelinvoker.SealInvocationMaterialV1(material)
	if err != nil || !reflect.DeepEqual(resealedMaterial, material) {
		t.Fatalf("re-sealed material = %#v, %v", resealedMaterial, err)
	}
	invalidMaterial := material
	invalidMaterial.Digest = ""
	invalidMaterial.ExpiresUnixNano = invalidMaterial.CreatedUnixNano
	if sealed, sealErr := modelinvoker.SealInvocationMaterialV1(invalidMaterial); sealErr == nil || !reflect.DeepEqual(sealed, modelinvoker.InvocationMaterialV1{}) {
		t.Fatalf("invalid material sealer leaked partial value = %#v, %v", sealed, sealErr)
	}

	authorization := material.Authorization
	resealedAuthorization, err := modelinvoker.SealInvocationMaterialAuthorizationV1(authorization)
	if err != nil || !reflect.DeepEqual(resealedAuthorization, authorization) {
		t.Fatalf("re-sealed authorization = %#v, %v", resealedAuthorization, err)
	}
	invalidAuthorization := authorization
	invalidAuthorization.Digest = ""
	invalidAuthorization.ExpiresUnixNano = invalidAuthorization.AuthorizedUnixNano
	if sealed, sealErr := modelinvoker.SealInvocationMaterialAuthorizationV1(invalidAuthorization); sealErr == nil || sealed != (modelinvoker.InvocationMaterialAuthorizationV1{}) {
		t.Fatalf("invalid authorization sealer leaked partial value = %#v, %v", sealed, sealErr)
	}
}

type governedFixtureV2 struct {
	gateway  *routegateway.Gateway
	store    *modelsqlite.Store
	state    *callState
	gate     *governedGateV1
	command  modelinvoker.GovernedModelTurnCommandV2
	prepared modelinvoker.PreparedModelInvocationFactV1
	ack      modelinvoker.PreparedModelInvocationCommitAckV1
	turnID   string
}

func newGovernedFixtureV2(t *testing.T, path string, options []routegateway.Option) governedFixtureV2 {
	t.Helper()
	store, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	return newGovernedFixtureV2WithStore(t, store, store, options)
}

func newGovernedFixtureV2WithStore(t *testing.T, store *modelsqlite.Store, turns modelinvoker.GovernedModelTurnRepositoryV2, options []routegateway.Option) governedFixtureV2 {
	t.Helper()
	call := governedCallV2()
	tempState := &callState{}
	temp := fakeGateway(t, defaultCatalog(t), countingBinding{state: tempState}, countingSecret{state: tempState, version: "v1"}, tempState)
	resolution, err := temp.Resolve(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if err := temp.Close(); err != nil {
		t.Fatal(err)
	}
	routeDigest, err := modelinvoker.DigestGovernedRouteSelectionV1(resolution.Route)
	if err != nil {
		t.Fatal(err)
	}
	requestDigest := digestV2(t, "unified request", call)
	toolsDigest := digestV2(t, "request tools", call.Request.Tools)
	prepared, err := modelinvoker.SealPreparedModelInvocationFactV1(modelinvoker.PreparedModelInvocationFactV1{
		InvocationID: "execution-governed-v2", InvocationDigest: requestDigest, UnifiedRequestDigest: requestDigest,
		RequestToolsDigest: toolsDigest, PreparedPlanDigest: digestV2(t, "prepared plan", call.Request.Input),
		RouteDigest: routeDigest, ProfileDigest: digestV2(t, "profile", call.Request.Model),
		ActualToolSurfaceDigest:       digestV2(t, "tool surface", call.Request.Tools),
		ActualProviderInjectionDigest: digestV2(t, "provider injection", call.RouteID),
		CapabilitySnapshotRef:         modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{ContractVersion: "1.0.0", ID: "capability-v2", Revision: 1, Digest: digestV2(t, "capability", call.RouteID)},
		RegistrySnapshotRef:           runtimeports.RegistrySnapshotRefV1{Owner: ownerV2("registry", "model-v2"), ContractVersion: "1.0.0", ID: "registry-v2", Revision: 1, Digest: digestV2(t, "registry", call.RouteID)},
		CreatedUnixNano:               gatewayNow.Add(-2 * time.Minute).UnixNano(), NotAfterUnixNano: gatewayNow.Add(10 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(modelinvoker.PreparedModelInvocationCurrentProjectionV1{
		Prepared: prepared.Ref(), CapabilitySnapshotRef: prepared.CapabilitySnapshotRef, RegistrySnapshotRef: prepared.RegistrySnapshotRef,
		ActualToolSurfaceDigest: prepared.ActualToolSurfaceDigest, ActualProviderInjectionDigest: prepared.ActualProviderInjectionDigest,
		CheckedUnixNano: gatewayNow.Add(-time.Minute).UnixNano(), ExpiresUnixNano: gatewayNow.Add(8 * time.Minute).UnixNano(), NotAfterUnixNano: prepared.NotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	ack, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(modelinvoker.PreparedModelInvocationCommitAckV1{
		PreparedRef: prepared.Ref(), CurrentRef: current.Ref(),
		GateImplementationRef: modelinvoker.PreparedModelInvocationGateImplementationRefV1{Owner: ownerV2("host", "gate-v2"), ContractVersion: "1.0.0", ID: "gate-v2", Revision: 1, Digest: digestV2(t, "gate", prepared.Ref())},
		SurfaceBindingRef:     modelinvoker.PreparedModelInvocationSurfaceBindingRefV1{Owner: ownerV2("tool", "surface-v2"), ContractVersion: "1.0.0", ID: "surface-v2", Revision: 1, Digest: prepared.ActualToolSurfaceDigest},
		CheckedUnixNano:       gatewayNow.Add(-30 * time.Second).UnixNano(), ExpiresUnixNano: gatewayNow.Add(6 * time.Minute).UnixNano(), NotAfterUnixNano: prepared.NotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	routeCallDigest, err := modelinvoker.DigestGovernedModelTurnRouteCallV2(call)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := modelinvoker.SealInvocationMaterialAuthorizationV1(modelinvoker.InvocationMaterialAuthorizationV1{
		PreparedRef: prepared.Ref(), CurrentRef: current.Ref(), RouteCallDigest: routeCallDigest,
		ContextFrameRef:      exactSourceV2("context", "frame-v2", digestV2(t, "context", call.Request.Input)),
		ToolSurfaceRef:       exactSourceV2("tool", "surface-v2", prepared.ActualToolSurfaceDigest),
		ProviderInjectionRef: exactSourceV2("model", "provider-v2", prepared.ActualProviderInjectionDigest),
		RouteRef:             exactSourceV2("model", "route-v2", prepared.RouteDigest),
		ProfileRef:           exactSourceV2("model", "profile-v2", prepared.ProfileDigest),
		AuthorizedUnixNano:   gatewayNow.UnixNano(), ExpiresUnixNano: gatewayNow.Add(7 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := authorization.ValidateAgainstV1(prepared, current, routeCallDigest, gatewayNow); err != nil {
		t.Fatalf("authorization exact validation: %v\nprepared=%#v\ncurrent=%#v\nauthorization=%#v", err, prepared, current, authorization)
	}
	authorizer := fixedInvocationMaterialAuthorizerV2{authorization: authorization}
	material, err := modelinvoker.NewInvocationMaterialV1(context.Background(), authorizer, prepared, current, call, gatewayNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.EnsurePreparedModelInvocationV1(context.Background(), prepared); err != nil {
		t.Fatal(err)
	}
	if _, err = store.EnsurePreparedModelInvocationCurrentV1(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	if _, err = store.EnsureInvocationMaterialV1(context.Background(), material); err != nil {
		t.Fatal(err)
	}
	state := &callState{}
	state.governedV2.Store(true)
	gate := &governedGateV1{ack: ack}
	dependencies := routegateway.GovernedModelTurnDependenciesV2{
		PreparedHistory: store, PreparedCurrent: store, CommitGate: gate, Materials: store, Turns: turns,
	}
	allOptions := append([]routegateway.Option{}, options...)
	allOptions = append(allOptions, routegateway.WithGovernedModelTurnsV2(dependencies))
	gateway := fakeGateway(t, defaultCatalog(t), countingBinding{state: state}, countingSecret{state: state, version: "v1"}, state, allOptions...)
	command := modelinvoker.GovernedModelTurnCommandV2{
		PreparedRef: prepared.Ref(), CurrentRef: current.Ref(), MaterialRef: material.RefV1(),
		AttemptRequestDigest: prepared.UnifiedRequestDigest, RouteCallDigest: material.RouteCallDigest,
		DispatchSequence: 1, ProviderAttemptOrdinal: 1,
	}
	initial, err := modelinvoker.NewPreparedGovernedModelTurnV2(command, gatewayNow)
	if err != nil {
		t.Fatal(err)
	}
	return governedFixtureV2{gateway: gateway, store: store, state: state, gate: gate, command: command, prepared: prepared, ack: ack, turnID: initial.ID}
}

func (fixture governedFixtureV2) close(t *testing.T) {
	t.Helper()
	if err := fixture.gateway.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.Close(); err != nil {
		t.Fatal(err)
	}
}

func governedCallV2() modelinvoker.RouteCall {
	strict := true
	parallel := false
	return modelinvoker.RouteCall{
		RouteID: "openai.direct.payg.responses", Invocation: generalInvocation(),
		Request: modelinvoker.Request{
			Model: "gpt-5.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "read README")},
			Tools: []modelinvoker.Tool{{
				Name: "workspace.read", Description: "read one file",
				Parameters: []byte(`{"type":"object","required":["path"],"properties":{"path":{"type":"string"}},"additionalProperties":false}`),
				Strict:     &strict,
			}},
			ToolChoice:        modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceRequired},
			ParallelToolCalls: &parallel,
			Budget:            modelinvoker.Budget{MaxOutputTokens: 256, Timeout: time.Minute},
		},
	}
}

type fixedInvocationMaterialAuthorizerV2 struct {
	authorization modelinvoker.InvocationMaterialAuthorizationV1
}

func (authorizer fixedInvocationMaterialAuthorizerV2) AuthorizeInvocationMaterialV1(context.Context, modelinvoker.PreparedModelInvocationFactV1, modelinvoker.PreparedModelInvocationCurrentProjectionV1, modelinvoker.RouteCall, time.Time) (modelinvoker.InvocationMaterialAuthorizationV1, error) {
	return authorizer.authorization, nil
}

func exactSourceV2(domain, id string, digest core.Digest) modelinvoker.InvocationMaterialExactSourceRefV1 {
	return modelinvoker.InvocationMaterialExactSourceRefV1{Owner: ownerV2(domain, id+"-owner"), Kind: domain + "-material", ID: id, Revision: 1, Digest: digest}
}

func ownerV2(domain, id string) core.OwnerRef {
	return core.OwnerRef{Domain: domain, ID: core.OwnerID(id)}
}

func digestV2(t *testing.T, label string, value any) core.Digest {
	t.Helper()
	label = strings.ReplaceAll(label, " ", "-")
	digest, err := core.CanonicalJSONDigest("praxis.model-invoker.tests", "v2", label, value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

type lostBoundaryReplyRepositoryV2 struct {
	inner modelinvoker.GovernedModelTurnRepositoryV2
	once  atomic.Bool
}

type ordinaryObservedCASRepositoryV2 struct {
	inner modelinvoker.GovernedModelTurnRepositoryV2
}

func (repository *ordinaryObservedCASRepositoryV2) CreateGovernedModelTurnV2(ctx context.Context, outcome modelinvoker.GovernedModelTurnOutcomeV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CreateGovernedModelTurnV2(ctx, outcome)
}
func (repository *ordinaryObservedCASRepositoryV2) CompareAndSwapGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CompareAndSwapGovernedModelTurnV2(ctx, request)
}
func (repository *ordinaryObservedCASRepositoryV2) CompareAndSwapObservedGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CompareAndSwapGovernedModelTurnV2(ctx, request)
}
func (repository *ordinaryObservedCASRepositoryV2) InspectExactGovernedModelTurnV2(ctx context.Context, ref modelinvoker.GovernedModelTurnRefV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectExactGovernedModelTurnV2(ctx, ref)
}
func (repository *ordinaryObservedCASRepositoryV2) InspectCurrentGovernedModelTurnV2(ctx context.Context, id string) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectCurrentGovernedModelTurnV2(ctx, id)
}
func (repository *ordinaryObservedCASRepositoryV2) InspectExactGovernedModelTurnToolCallProjectionV2(ctx context.Context, ref modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	return repository.inner.InspectExactGovernedModelTurnToolCallProjectionV2(ctx, ref)
}

func (repository *lostBoundaryReplyRepositoryV2) CreateGovernedModelTurnV2(ctx context.Context, outcome modelinvoker.GovernedModelTurnOutcomeV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CreateGovernedModelTurnV2(ctx, outcome)
}
func (repository *lostBoundaryReplyRepositoryV2) CompareAndSwapGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	mutation, err := repository.inner.CompareAndSwapGovernedModelTurnV2(ctx, request)
	if err == nil && request.Next.State == modelinvoker.GovernedModelTurnProviderBoundaryCrossedV2 && repository.once.CompareAndSwap(false, true) {
		return modelinvoker.GovernedModelTurnMutationV2{}, &modelinvoker.GovernedModelInvocationErrorV1{Kind: modelinvoker.GovernedModelInvocationErrorIndeterminate, Operation: "cas_boundary", Message: "lost boundary reply"}
	}
	return mutation, err
}
func (repository *lostBoundaryReplyRepositoryV2) CompareAndSwapObservedGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CompareAndSwapObservedGovernedModelTurnV2(ctx, request)
}
func (repository *lostBoundaryReplyRepositoryV2) InspectExactGovernedModelTurnV2(ctx context.Context, ref modelinvoker.GovernedModelTurnRefV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectExactGovernedModelTurnV2(ctx, ref)
}
func (repository *lostBoundaryReplyRepositoryV2) InspectCurrentGovernedModelTurnV2(ctx context.Context, id string) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectCurrentGovernedModelTurnV2(ctx, id)
}
func (repository *lostBoundaryReplyRepositoryV2) InspectExactGovernedModelTurnToolCallProjectionV2(ctx context.Context, ref modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	return repository.inner.InspectExactGovernedModelTurnToolCallProjectionV2(ctx, ref)
}

type lostObservedReplyRepositoryV2 struct {
	inner modelinvoker.GovernedModelTurnRepositoryV2
	once  atomic.Bool
}

func (repository *lostObservedReplyRepositoryV2) CreateGovernedModelTurnV2(ctx context.Context, outcome modelinvoker.GovernedModelTurnOutcomeV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CreateGovernedModelTurnV2(ctx, outcome)
}
func (repository *lostObservedReplyRepositoryV2) CompareAndSwapGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	return repository.inner.CompareAndSwapGovernedModelTurnV2(ctx, request)
}
func (repository *lostObservedReplyRepositoryV2) CompareAndSwapObservedGovernedModelTurnV2(ctx context.Context, request modelinvoker.GovernedModelTurnCASV2) (modelinvoker.GovernedModelTurnMutationV2, error) {
	mutation, err := repository.inner.CompareAndSwapObservedGovernedModelTurnV2(ctx, request)
	if err == nil && repository.once.CompareAndSwap(false, true) {
		return modelinvoker.GovernedModelTurnMutationV2{}, &modelinvoker.GovernedModelInvocationErrorV1{Kind: modelinvoker.GovernedModelInvocationErrorIndeterminate, Operation: "cas_observed", Message: "lost observed reply"}
	}
	return mutation, err
}
func (repository *lostObservedReplyRepositoryV2) InspectExactGovernedModelTurnV2(ctx context.Context, ref modelinvoker.GovernedModelTurnRefV2) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectExactGovernedModelTurnV2(ctx, ref)
}
func (repository *lostObservedReplyRepositoryV2) InspectCurrentGovernedModelTurnV2(ctx context.Context, id string) (modelinvoker.GovernedModelTurnOutcomeV2, error) {
	return repository.inner.InspectCurrentGovernedModelTurnV2(ctx, id)
}
func (repository *lostObservedReplyRepositoryV2) InspectExactGovernedModelTurnToolCallProjectionV2(ctx context.Context, ref modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	return repository.inner.InspectExactGovernedModelTurnToolCallProjectionV2(ctx, ref)
}

var _ modelinvoker.InvocationMaterialAuthorizerV1 = fixedInvocationMaterialAuthorizerV2{}
var _ modelinvoker.GovernedModelTurnRepositoryV2 = (*lostBoundaryReplyRepositoryV2)(nil)
var _ modelinvoker.GovernedModelTurnRepositoryV2 = (*lostObservedReplyRepositoryV2)(nil)
var _ modelinvoker.GovernedModelTurnRepositoryV2 = (*ordinaryObservedCASRepositoryV2)(nil)
