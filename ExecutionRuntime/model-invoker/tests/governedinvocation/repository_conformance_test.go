package governedinvocation_test

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	modelsqlite "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/storage/sqlite"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type repositoryFactoryV1 func(*testing.T) (modelinvoker.GovernedModelInvocationRepositoryV1, func())

func TestGovernedModelInvocationRepositoryConformanceV1(t *testing.T) {
	factories := map[string]repositoryFactoryV1{
		"memory": func(*testing.T) (modelinvoker.GovernedModelInvocationRepositoryV1, func()) {
			return modelinvoker.NewInMemoryGovernedModelInvocationStoreV1(), func() {}
		},
		"sqlite_wal": func(t *testing.T) (modelinvoker.GovernedModelInvocationRepositoryV1, func()) {
			store, err := modelsqlite.Open(context.Background(), modelsqlite.Config{Path: t.TempDir() + "/conformance.db"})
			if err != nil {
				t.Fatal(err)
			}
			return store, func() { _ = store.Close() }
		},
	}
	for name, factory := range factories {
		factory := factory
		t.Run(name, func(t *testing.T) { runRepositoryConformanceV1(t, factory) })
	}
}

func TestGovernedModelInvocationPublicCapabilitiesRemainNarrowV1(t *testing.T) {
	repository := reflect.TypeOf((*modelinvoker.GovernedModelInvocationRepositoryV1)(nil)).Elem()
	if repository.NumMethod() != 4 {
		t.Fatalf("repository capability widened: %v", repository)
	}
	port := reflect.TypeOf((*modelinvoker.GovernedModelInvocationPortV1)(nil)).Elem()
	if port.NumMethod() != 2 {
		t.Fatalf("execution capability widened: %v", port)
	}
	reader := reflect.TypeOf((*modelinvoker.GovernedModelInvocationBindingReaderV1)(nil)).Elem()
	if reader.NumMethod() != 1 || reader.Method(0).Name != "InspectExactGovernedModelInvocationBindingV1" {
		t.Fatalf("binding Reader capability widened: %v", reader)
	}
	observation := reflect.TypeOf(modelinvoker.GovernedModelInvocationObservationV1{})
	for _, forbidden := range []string{"RawRequest", "RawResponse", "NativeEvents", "ProviderMetadata", "State", "MappingReport"} {
		if _, ok := observation.FieldByName(forbidden); ok {
			t.Fatalf("provider-owned field %s escaped into governed Observation", forbidden)
		}
	}
}

func TestGovernedRouteCallRejectsDuplicateJSONAndToolsV1(t *testing.T) {
	call := governedConformanceCallV1()
	call.Request.Output.Schema = []byte(`{"type":"object","properties":{"a":{"type":"string","type":"number"}}}`)
	if _, err := modelinvoker.DigestGovernedRouteCallV1(call); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorInvalid {
		t.Fatalf("duplicate nested schema = %v", err)
	}
	call = governedConformanceCallV1()
	call.Request.Tools = []modelinvoker.Tool{{Name: "unsafe", Description: "not governed here"}}
	if _, err := modelinvoker.DigestGovernedRouteCallV1(call); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorInvalid {
		t.Fatalf("tool-bearing governed call = %v", err)
	}
	call = governedConformanceCallV1()
	call.Request.Output.Schema = []byte(`{"$ref":"https://example.invalid/review-schema.json"}`)
	if _, err := modelinvoker.DigestGovernedRouteCallV1(call); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorInvalid {
		t.Fatalf("external schema reference = %v", err)
	}
}

func runRepositoryConformanceV1(t *testing.T, factory repositoryFactoryV1) {
	t.Helper()
	repository, closeRepository := factory(t)
	defer closeRepository()
	prepared, boundary := governedConformanceFactsV1(t)
	created, err := repository.CreateGovernedModelInvocationV1(context.Background(), prepared)
	if err != nil || !created.Applied || created.Fact.RefV1() != prepared.RefV1() {
		t.Fatalf("create = %#v, %v", created, err)
	}
	replay, err := repository.CreateGovernedModelInvocationV1(context.Background(), prepared)
	if err != nil || replay.Applied || replay.Fact.RefV1() != prepared.RefV1() {
		t.Fatalf("create replay = %#v, %v", replay, err)
	}
	wrongExpected := prepared.RefV1()
	wrongExpected.Revision = 99
	if _, err := repository.CompareAndSwapGovernedModelInvocationV1(context.Background(), modelinvoker.GovernedModelInvocationCASV1{Expected: wrongExpected, Next: boundary}); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict {
		t.Fatalf("non-adjacent CAS = %v", err)
	}
	if current, err := repository.InspectCurrentGovernedModelInvocationV1(context.Background(), prepared.ID); err != nil || current.RefV1() != prepared.RefV1() {
		t.Fatalf("non-adjacent CAS wrote current: %#v, %v", current, err)
	}

	const workers = 64
	start := make(chan struct{})
	var wait sync.WaitGroup
	var applied atomic.Uint64
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			mutation, casErr := repository.CompareAndSwapGovernedModelInvocationV1(context.Background(), modelinvoker.GovernedModelInvocationCASV1{Expected: prepared.RefV1(), Next: boundary})
			if casErr == nil && mutation.Applied {
				applied.Add(1)
			}
		}()
	}
	close(start)
	wait.Wait()
	if applied.Load() != 1 {
		t.Fatalf("boundary winners = %d", applied.Load())
	}
	for _, fact := range []modelinvoker.GovernedModelInvocationFactV1{prepared, boundary} {
		got, inspectErr := repository.InspectExactGovernedModelInvocationV1(context.Background(), fact.RefV1())
		if inspectErr != nil || got.RefV1() != fact.RefV1() {
			t.Fatalf("exact revision %d = %#v, %v", fact.Revision, got, inspectErr)
		}
	}
	current, err := repository.InspectCurrentGovernedModelInvocationV1(context.Background(), prepared.ID)
	if err != nil || current.RefV1() != boundary.RefV1() {
		t.Fatalf("current = %#v, %v", current, err)
	}
	if durable, ok := repository.(interface{ IntegrityCheckV1(context.Context) error }); ok {
		if err := durable.IntegrityCheckV1(context.Background()); err != nil {
			t.Fatalf("durable integrity check = %v", err)
		}
	}

	changed := prepared.CloneV1()
	changed.RouteCallDigest = core.DigestBytes([]byte("changed-route"))
	changed.ID = ""
	changed.Digest = ""
	changed, err = modelinvoker.SealGovernedModelInvocationFactV1(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateGovernedModelInvocationV1(context.Background(), changed); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict {
		t.Fatalf("changed logical attempt = %v", err)
	}
	if _, err := repository.InspectExactGovernedModelInvocationV1(context.Background(), prepared.RefV1()); err != nil {
		t.Fatalf("changed attempt damaged history: %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repository.InspectExactGovernedModelInvocationV1(canceled, prepared.RefV1()); modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorIndeterminate {
		t.Fatalf("canceled exact Inspect = %v", err)
	}
}

func governedConformanceFactsV1(t *testing.T) (modelinvoker.GovernedModelInvocationFactV1, modelinvoker.GovernedModelInvocationFactV1) {
	t.Helper()
	now := time.Unix(1_900_100_000, 0)
	digest := func(label string) core.Digest { return core.DigestBytes([]byte(label)) }
	owner := func(domain, id string) core.OwnerRef { return core.OwnerRef{Domain: domain, ID: core.OwnerID(id)} }
	requestDigest := digest("conformance-request")
	preparedFact, err := modelinvoker.SealPreparedModelInvocationFactV1(modelinvoker.PreparedModelInvocationFactV1{
		InvocationID: "conformance-invocation", InvocationDigest: requestDigest, UnifiedRequestDigest: requestDigest,
		RequestToolsDigest: digest("no-tools"), PreparedPlanDigest: digest("plan"), RouteDigest: digest("route"), ProfileDigest: digest("profile"),
		ActualToolSurfaceDigest: digest("surface"), ActualProviderInjectionDigest: digest("provider-injection"),
		CapabilitySnapshotRef: modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{ContractVersion: "1.0.0", ID: "capability", Revision: 1, Digest: digest("capability")},
		RegistrySnapshotRef:   runtimeports.RegistrySnapshotRefV1{Owner: owner("registry", "owner"), ContractVersion: "1.0.0", ID: "registry", Revision: 1, Digest: digest("registry")},
		CreatedUnixNano:       now.Add(-time.Minute).UnixNano(), NotAfterUnixNano: now.Add(10 * time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(modelinvoker.PreparedModelInvocationCurrentProjectionV1{
		Prepared: preparedFact.Ref(), CapabilitySnapshotRef: preparedFact.CapabilitySnapshotRef, RegistrySnapshotRef: preparedFact.RegistrySnapshotRef,
		ActualToolSurfaceDigest: preparedFact.ActualToolSurfaceDigest, ActualProviderInjectionDigest: preparedFact.ActualProviderInjectionDigest,
		CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(8 * time.Minute).UnixNano(), NotAfterUnixNano: preparedFact.NotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	call := governedConformanceCallV1()
	routeDigest, err := modelinvoker.DigestGovernedRouteCallV1(call)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := modelinvoker.NewPreparedGovernedModelInvocationForGatewayV1(modelinvoker.GovernedModelInvocationCommandV1{PreparedRef: preparedFact.Ref(), CurrentRef: current.Ref(), AttemptRequestDigest: requestDigest, DispatchSequence: 1, ProviderAttemptOrdinal: 1, Call: call}, routeDigest, now)
	if err != nil {
		t.Fatal(err)
	}
	ack, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(modelinvoker.PreparedModelInvocationCommitAckV1{
		PreparedRef: preparedFact.Ref(), CurrentRef: current.Ref(),
		GateImplementationRef: modelinvoker.PreparedModelInvocationGateImplementationRefV1{Owner: owner("host", "gate"), ContractVersion: "1.0.0", ID: "gate", Revision: 1, Digest: digest("gate")},
		SurfaceBindingRef:     modelinvoker.PreparedModelInvocationSurfaceBindingRefV1{Owner: owner("host", "surface"), ContractVersion: "1.0.0", ID: "surface", Revision: 1, Digest: digest("surface-binding")},
		CheckedUnixNano:       now.UnixNano(), ExpiresUnixNano: now.Add(6 * time.Minute).UnixNano(), NotAfterUnixNano: preparedFact.NotAfterUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := modelinvoker.SealPreparedModelInvocationDispatchReceiptV1(modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1{
		PreparedRef: preparedFact.Ref(), CurrentRef: current.Ref(), AckRef: ack.Ref(), DispatchSequence: 1,
		BoundaryKind: modelinvoker.GovernedModelProviderBoundaryKindV1, ProviderAttemptOrdinal: 1, AttemptRequestDigest: routeDigest,
		ActualToolSurfaceDigest: preparedFact.ActualToolSurfaceDigest, ActualProviderInjectionDigest: preparedFact.ActualProviderInjectionDigest,
		CheckedUnixNano: now.Add(time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	boundary := prepared.CloneV1()
	boundary.Revision = 2
	boundary.State = modelinvoker.GovernedModelInvocationProviderBoundaryCrossedV1
	boundary.UpdatedUnixNano = now.Add(time.Second).UnixNano()
	boundary.ExpiresUnixNano = ack.ExpiresUnixNano
	ackRef := ack.Ref()
	boundary.AckRef = &ackRef
	boundary.DispatchReceipt = &receipt
	boundary.Digest = ""
	boundary, err = modelinvoker.SealGovernedModelInvocationFactV1(boundary)
	if err != nil {
		t.Fatal(err)
	}
	return prepared, boundary
}

func governedConformanceCallV1() modelinvoker.RouteCall {
	strict := true
	return modelinvoker.RouteCall{
		RouteID:    "openai.direct.payg.responses",
		Invocation: upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground},
		Request:    modelinvoker.Request{Model: "gpt-5.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "review")}, ToolChoice: modelinvoker.ToolChoice{Mode: modelinvoker.ToolChoiceNone}, Output: modelinvoker.OutputConstraint{Type: modelinvoker.OutputJSONSchema, Name: "review", Schema: []byte(`{"type":"object"}`), Strict: &strict}},
	}
}
