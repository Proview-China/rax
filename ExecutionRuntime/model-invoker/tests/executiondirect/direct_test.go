package executiondirect_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/effect"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/direct"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

var directTestNow = time.Date(2026, 7, 13, 2, 0, 0, 0, time.UTC)

func TestDirectAdapterPreflightMapsRequestWithoutProviderContact(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}
	adapter := newDirectAdapter(t, backend, selected)
	report, err := adapter.Preflight(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Accepted || report.ActualManifest.Digest == "" || backend.resolveCalls != 1 || backend.invokeCalls != 0 || backend.streamCalls != 0 {
		t.Fatalf("preflight = %#v, backend = %#v", report, backend)
	}
	if backend.lastCall.Request.Provider != "" || backend.lastCall.Request.Protocol != modelinvoker.ProtocolAuto || backend.lastCall.Request.Endpoint != "" {
		t.Fatalf("Route-owned selectors leaked into request: %#v", backend.lastCall.Request)
	}
	if backend.lastCall.Request.Output.Type != modelinvoker.OutputJSONSchema || len(backend.lastCall.Request.Tools) != 1 || backend.lastCall.Request.ToolChoice.Mode != modelinvoker.ToolChoiceAuto {
		t.Fatalf("mapped request = %#v", backend.lastCall.Request)
	}
}

func TestDirectToolRouteRequiresSingleProjectionRepositoryAndCannotPairStores(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}
	adapter, err := direct.New(direct.Config{
		Identity: union.VersionedIdentity{ID: "direct-missing-projection-ports", Version: "v1"}, Backend: backend,
		RouteID: selected.Selection.BaseRouteID, Model: selected.Selection.ModelID,
		Invocation: upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := adapter.Preflight(context.Background(), invocation)
	if err != nil || report.Accepted || report.RejectionCode != "direct_tool_call_observation_projection_unavailable" || backend.resolveCalls != 0 {
		t.Fatalf("Preflight() = %#v/%v, resolve=%d", report, err, backend.resolveCalls)
	}
	if _, err = adapter.Open(context.Background(), invocation); !errors.Is(err, direct.ErrToolCallObservationProjectionUnavailable) {
		t.Fatalf("Open() error = %v", err)
	}
	configType := reflect.TypeOf(direct.Config{})
	publisherType := reflect.TypeOf((*modelinvoker.ToolCallCandidateObservationProjectionPublisherV1)(nil)).Elem()
	readerType := reflect.TypeOf((*modelinvoker.ToolCallCandidateObservationProjectionReaderV1)(nil)).Elem()
	repositoryType := reflect.TypeOf((*modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1)(nil)).Elem()
	repositoryFields := 0
	for i := 0; i < configType.NumField(); i++ {
		field := configType.Field(i)
		if field.Type == publisherType || field.Type == readerType {
			t.Fatalf("Direct Config exposes separately pairable projection capability %s", field.Name)
		}
		if field.Type == repositoryType {
			repositoryFields++
		}
	}
	if repositoryFields != 1 {
		t.Fatalf("Direct Config repository fields = %d, want 1", repositoryFields)
	}
	storeA := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	storeB := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	splitType := reflect.TypeOf(splitProjectionPorts{publisher: storeA, reader: storeB})
	if splitType.Implements(repositoryType) {
		t.Fatal("split Store A/B Publisher+Reader unexpectedly satisfies atomic Repository")
	}
	var typedNilPointer *modelinvoker.InMemoryToolCallCandidateObservationProjectionStoreV1
	var typedNilRepository modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1 = typedNilPointer
	if _, err = direct.New(direct.Config{
		Identity: union.VersionedIdentity{ID: "direct-typed-nil-repository", Version: "v1"}, Backend: backend,
		RouteID: selected.Selection.BaseRouteID, Model: selected.Selection.ModelID,
		Invocation:                    upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground},
		ToolCallObservationRepository: typedNilRepository,
	}); !errors.Is(err, direct.ErrInvalidConfig) || backend.resolveCalls != 0 || backend.invokeCalls != 0 || backend.streamCalls != 0 {
		t.Fatalf("typed-nil New() = %v, backend=%#v", err, backend)
	}
}

func TestDirectTypedNilRepositoryFailsClosedInPreflightAndOpenBeforeBackend(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "sync", true: "stream"}[stream], func(t *testing.T) {
			invocation, selected := directInvocation(t, stream, true)
			backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}
			adapter := newDirectAdapter(t, backend, selected)
			var typedNilPointer *modelinvoker.InMemoryToolCallCandidateObservationProjectionStoreV1
			var typedNilRepository modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1 = typedNilPointer
			injectProjectionRepositoryFault(t, adapter, typedNilRepository)

			report, err := adapter.Preflight(context.Background(), invocation)
			if err != nil || report.Accepted || report.RejectionCode != "direct_tool_call_observation_projection_unavailable" {
				t.Fatalf("typed-nil Preflight() = %#v/%v", report, err)
			}
			if _, err = adapter.Open(context.Background(), invocation); !errors.Is(err, direct.ErrToolCallObservationProjectionUnavailable) {
				t.Fatalf("typed-nil Open() = %v", err)
			}
			if backend.resolveCalls != 0 || backend.invokeCalls != 0 || backend.streamCalls != 0 {
				t.Fatalf("typed-nil reached Backend: %#v", backend)
			}
		})
	}
}

// injectProjectionRepositoryFault mutates only the private copied Config in an
// already-constructed Adapter. It is an external-package fault injection used
// to prove Preflight/Open defense in depth without adding a test-only public
// production hook.
func injectProjectionRepositoryFault(t *testing.T, adapter *direct.Adapter, repository modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1) {
	t.Helper()
	adapterValue := reflect.ValueOf(adapter).Elem()
	repositoryField := adapterValue.FieldByName("config").FieldByName("ToolCallObservationRepository")
	if !repositoryField.IsValid() || !repositoryField.CanAddr() {
		t.Fatal("Direct Adapter repository field is not fault-injectable")
	}
	writable := reflect.NewAt(repositoryField.Type(), unsafe.Pointer(repositoryField.UnsafeAddr())).Elem()
	writable.Set(reflect.ValueOf(repository))
}

func TestDirectToolPolicyFiltersDeclaredToolsAndRejectsUnknownIDs(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	invocation.Request.Tools = append(invocation.Request.Tools, union.ToolDefinition{
		ID: "write-config", Name: "write_config", Kind: "function", ExecutionOwner: union.ExecutionOwnerPraxis,
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	})
	invocation.Request.ToolPolicy.AllowedToolIDs = []string{"read-config"}
	invocation.Plan.Digest = ""
	delete(invocation.Plan.Metadata, "request_digest")
	invocation, err := execution.NewInvocation(invocation.Request, invocation.Plan)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}
	report, err := newDirectAdapter(t, backend, selected).Preflight(context.Background(), invocation)
	if err != nil || !report.Accepted {
		t.Fatalf("Preflight() = %#v, %v", report, err)
	}
	if got := backend.lastCall.Request.Tools; len(got) != 1 || got[0].Name != "read_config" {
		t.Fatalf("filtered tools = %#v", got)
	}

	invocation.Request.ToolPolicy.AllowedToolIDs = []string{"missing-tool"}
	invocation.Plan.Digest = ""
	delete(invocation.Plan.Metadata, "request_digest")
	if _, err = execution.NewInvocation(invocation.Request, invocation.Plan); !errors.Is(err, execution.ErrInvalidInvocation) {
		t.Fatalf("unknown allowed ToolID invocation error=%v", err)
	}
}

func TestDirectUndeclaredProviderToolCallFailsClosed(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-unknown-tool", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted,
		StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{
			ID: "call-unknown", Name: "undeclared_tool", Arguments: json.RawMessage(`{}`),
		}}},
	}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	seenViolation := false
	for {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			if !errors.Is(receiveErr, direct.ErrProtocolTerminal) || !seenViolation {
				t.Fatalf("Receive() error=%v violation=%v", receiveErr, seenViolation)
			}
			break
		}
		seenViolation = seenViolation || event.Diagnostic != nil && event.Diagnostic.Code == "unattributed_tool_call"
	}
}

func TestDirectDuplicateProviderToolCallIDsFailBeforePendingMap(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	first := modelinvoker.FunctionCall{ID: "call-duplicate", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	second := modelinvoker.FunctionCall{ID: "call-duplicate", Name: "read_config", Arguments: json.RawMessage(`{"path":"b"}`)}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-duplicate", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &first}, {Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &second}},
	}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	seenViolation, seenToolCall := false, false
	for {
		event, receiveErr := session.Receive(context.Background())
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		seenViolation = seenViolation || event.Diagnostic != nil && event.Diagnostic.Code == "invalid_tool_call"
		seenToolCall = seenToolCall || event.Model != nil && event.Model.Kind == "model_tool_call"
	}
	if !seenViolation || seenToolCall {
		t.Fatalf("duplicate pre-map fail-closed violation/tool-call = %v/%v", seenViolation, seenToolCall)
	}
}

func TestDirectSyncMultiToolOneInvalidPublishesNoToolCallOrPendingItem(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	valid := modelinvoker.FunctionCall{ID: "call-valid", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	invalid := modelinvoker.FunctionCall{ID: "call-invalid", Name: "read_config", Arguments: json.RawMessage(`["not-an-object"]`)}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-atomic-sync", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &valid}, {Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &invalid}},
	}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	assertAtomicToolCallRejection(t, session)
}

func TestDirectSyncPublishesOneExactObservationBeforeCompatibilityCall(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	call := modelinvoker.FunctionCall{ID: "call-sync-observation", Name: "read_config", Arguments: json.RawMessage(`{"z":2,"path":"a"}`)}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-sync-observation", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}},
	}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	projection, compatibility := receiveObservationAndCompatibility(t, session, 1)
	if projection.Ref.InvocationID != string(invocation.Request.ExecutionID) || projection.Ref.InvocationDigest != projection.Observation.InvocationDigest || projection.Ref.ObservationDigest != projection.Observation.Digest {
		t.Fatalf("sync projection lineage = %#v", projection.Ref)
	}
	if len(projection.Observation.Calls) != 1 || projection.Observation.Calls[0].Ordinal != 0 || projection.Observation.Calls[0].CallID != call.ID || projection.Observation.Calls[0].Name != call.Name || string(projection.Observation.Calls[0].CanonicalArguments) != `{"path":"a","z":2}` {
		t.Fatalf("sync projection calls = %#v", projection.Observation.Calls)
	}
	assertCompatibilityBinding(t, compatibility[0], projection.Ref, 0)
}

func TestDirectSyncNMoreThanOnePublishesWholeObservationBeforeNonAuthoritativeCalls(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	first := modelinvoker.FunctionCall{ID: "call-sync-b", Name: "read_config", Arguments: json.RawMessage(`{"path":"b"}`)}
	second := modelinvoker.FunctionCall{ID: "call-sync-a", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-sync-batch", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &first}, {Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &second}},
	}}}
	session, err := newDirectAdapterWithProjectionRepository(t, backend, selected, store).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	projection, compatibility := receiveObservationAndCompatibility(t, session, 2)
	if len(projection.Observation.Calls) != 2 || projection.Observation.Calls[0].CallID != first.ID || projection.Observation.Calls[0].Ordinal != 0 || projection.Observation.Calls[1].CallID != second.ID || projection.Observation.Calls[1].Ordinal != 1 {
		t.Fatalf("sync N>1 observation was split or reordered: %#v", projection.Observation.Calls)
	}
	for index := range compatibility {
		assertCompatibilityBinding(t, compatibility[index], projection.Ref, uint32(index))
	}
	stored, err := store.InspectExactProjectionV1(context.Background(), projection.Ref)
	if err != nil || !reflect.DeepEqual(stored, projection) || store.StatsV1().EnsureCalls != 1 || store.StatsV1().Records != 1 {
		t.Fatalf("sync N>1 store = %#v/%v/%#v", stored, err, store.StatsV1())
	}
}

func TestDirectSyncEnsureLostReplyRetriesSameAtomicEnsureOnce(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	repository := &faultProjectionRepository{delegate: store, loseReplyAfterEnsure: true}
	call := modelinvoker.FunctionCall{ID: "call-lost-reply", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-lost-reply", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}},
	}}}
	session, err := newDirectAdapterWithProjectionRepository(t, backend, selected, repository).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	receiveObservationAndCompatibility(t, session, 1)
	if repository.callCount() != 2 || store.StatsV1().EnsureCalls != 2 || store.StatsV1().PublishCalls != 0 || store.StatsV1().Records != 1 {
		t.Fatalf("transport/store stats = %d/%#v", repository.callCount(), store.StatsV1())
	}
}

func TestDirectSyncIndeterminateEnsureBeforeLinearizationRetriesSameProjectionOnce(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	repository := &faultProjectionRepository{delegate: store, indeterminateBeforeEnsure: true}
	call := modelinvoker.FunctionCall{ID: "call-authoritative-retry", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-authoritative-retry", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}},
	}}}
	session, err := newDirectAdapterWithProjectionRepository(t, backend, selected, repository).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	projection, _ := receiveObservationAndCompatibility(t, session, 1)
	if repository.callCount() != 2 || store.StatsV1().EnsureCalls != 1 || store.StatsV1().PublishCalls != 0 || store.StatsV1().Records != 1 {
		t.Fatalf("transport/store stats = %d/%#v", repository.callCount(), store.StatsV1())
	}
	if repository.firstRef != projection.Ref || repository.secondRef != projection.Ref || !reflect.DeepEqual(repository.firstProjection, repository.secondProjection) {
		t.Fatalf("recovery changed projection/ref: %#v/%#v", repository.firstRef, repository.secondRef)
	}
}

func TestDirectSyncNonIndeterminateEnsureFailureNeverRetriesOrEmits(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	repository := &faultProjectionRepository{delegate: store, failBeforeEnsureKind: modelinvoker.ToolCallCandidateObservationProjectionErrorUnknownAbsent}
	call := modelinvoker.FunctionCall{ID: "call-unknown-not-found", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-unknown-not-found", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}},
	}}}
	session, err := newDirectAdapterWithProjectionRepository(t, backend, selected, repository).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	assertAtomicToolCallRejection(t, session)
	if repository.callCount() != 1 || store.StatsV1().EnsureCalls != 0 || store.StatsV1().PublishCalls != 0 || store.StatsV1().Records != 0 {
		t.Fatalf("non-indeterminate Ensure triggered retry: %d/%#v", repository.callCount(), store.StatsV1())
	}
}

func TestDirectSyncUnavailableAtomicEnsureFailsClosedWithoutRetry(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	repository := &faultProjectionRepository{delegate: store, failBeforeEnsureKind: modelinvoker.ToolCallCandidateObservationProjectionErrorUnavailable}
	call := modelinvoker.FunctionCall{ID: "call-eventual-reader", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-eventual-reader", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}},
	}}}
	session, err := newDirectAdapterWithProjectionRepository(t, backend, selected, repository).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	assertAtomicToolCallRejection(t, session)
	if repository.callCount() != 1 || store.StatsV1().EnsureCalls != 0 || store.StatsV1().Records != 0 {
		t.Fatalf("unavailable Ensure was retried or mutated state: %d/%#v", repository.callCount(), store.StatsV1())
	}
}

func TestDirectSyncValidButNonExactEnsureProjectionFailsClosed(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	repository := &faultProjectionRepository{delegate: store, returnDifferentProjection: true}
	call := modelinvoker.FunctionCall{ID: "call-sync-non-exact", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-sync-non-exact", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}},
	}}}
	session, err := newDirectAdapterWithProjectionRepository(t, backend, selected, repository).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	assertAtomicToolCallRejection(t, session)
	if repository.callCount() != 1 || store.StatsV1().EnsureCalls != 0 || store.StatsV1().Records != 0 {
		t.Fatalf("non-exact Ensure result mutated state or retried: %d/%#v", repository.callCount(), store.StatsV1())
	}
}

func TestDirectSyncRepeatedLostEnsureReplyStopsAfterOneCanonicalRetry(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	repository := &faultProjectionRepository{delegate: store, indeterminateBeforeEnsure: true, loseReplyAfterEveryEnsure: true}
	call := modelinvoker.FunctionCall{ID: "call-repeated-lost-reply", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-repeated-lost-reply", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}},
	}}}
	session, err := newDirectAdapterWithProjectionRepository(t, backend, selected, repository).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	assertAtomicToolCallRejection(t, session)
	if repository.callCount() != 2 || store.StatsV1().EnsureCalls != 1 || store.StatsV1().PublishCalls != 0 || store.StatsV1().Records != 1 {
		t.Fatalf("repeated lost reply was not bounded: %d/%#v", repository.callCount(), store.StatsV1())
	}
}

func TestDirectSyncRepeatedPreLinearizationIndeterminateStopsAfterOneRetry(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	repository := &faultProjectionRepository{delegate: store, indeterminateBeforeEveryEnsure: true}
	call := modelinvoker.FunctionCall{ID: "call-bounded-absence", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-bounded-absence", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}},
	}}}
	session, err := newDirectAdapterWithProjectionRepository(t, backend, selected, repository).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	assertAtomicToolCallRejection(t, session)
	if repository.callCount() != 2 || store.StatsV1().EnsureCalls != 0 || store.StatsV1().PublishCalls != 0 || store.StatsV1().Records != 0 {
		t.Fatalf("indeterminate Ensure retried beyond budget: %d/%#v", repository.callCount(), store.StatsV1())
	}
}

func TestDirectNonStreamToolLoopPairsCallResultThenTerminates(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}
	backend.invokeResponses = []modelinvoker.Response{
		{
			ID: "response-1", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
			Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-1", Name: "read_config", Arguments: json.RawMessage(`{"path":"config.go"}`)}}},
		},
		{
			ID: "response-2", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn,
			Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemText, Text: "done"}},
		},
	}
	adapter := newDirectAdapter(t, backend, selected)
	session, err := adapter.Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	seenToolCall := false
	runningAttempts := 0
	for !seenToolCall {
		event, err := session.Receive(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		seenToolCall = event.Model != nil && event.Model.Kind == "model_tool_call"
		if event.Mechanism != nil && event.Mechanism.Attempt != nil && event.Mechanism.Attempt.Status == union.AttemptStatusRunning {
			runningAttempts++
		}
	}
	resultPayload := json.RawMessage(`{"call_id":"call-1","name":"read_config","output":"strict","is_error":false,"executed":true,"result_origin":"external","side_effect_state":"none"}`)
	if err := session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandProvideToolResult, ActionID: "call-1", Payload: resultPayload}); err != nil {
		t.Fatal(err)
	}
	seenFinal, seenTerminal, seenPendingItem, seenCompletedItem, seenToolResult, completedAttempts := false, false, false, false, false, 0
	for !seenTerminal {
		event, err := session.Receive(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if event.Model != nil && event.Model.Kind == "content_completed" && len(event.Model.Content) == 1 && event.Model.Content[0].Text == "done" {
			seenFinal = true
		}
		if event.Mechanism != nil && event.Mechanism.Attempt != nil && event.Mechanism.Attempt.Status == union.AttemptStatusCompleted {
			completedAttempts++
		}
		if event.Item != nil && event.Item.Item.ActionID == "call-1" {
			seenPendingItem = seenPendingItem || event.Item.Item.Status == union.ItemStatusPending
			seenCompletedItem = seenCompletedItem || event.Item.Item.Status == union.ItemStatusCompleted
		}
		if event.Model != nil && event.Model.Kind == "tool_result_provided" {
			seenToolResult = event.Model.Executed != nil && *event.Model.Executed && event.Model.ResultOrigin == union.EventOriginExternal
			if strings.Contains(string(event.Model.Payload), "strict") {
				t.Fatalf("tool output leaked into audit payload: %s", event.Model.Payload)
			}
		}
		seenTerminal = event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate
	}
	if !seenFinal || !seenPendingItem || !seenCompletedItem || !seenToolResult || backend.invokeCalls != 2 || runningAttempts == 0 || runningAttempts != completedAttempts {
		t.Fatalf("final=%v items=%v/%v result=%v invokeCalls=%d attempts=%d/%d", seenFinal, seenPendingItem, seenCompletedItem, seenToolResult, backend.invokeCalls, runningAttempts, completedAttempts)
	}
	second := backend.calls[1].Request.Input
	if len(second) < 3 || second[len(second)-2].Type != modelinvoker.InputTypeFunctionCall || second[len(second)-1].Type != modelinvoker.InputTypeFunctionResult {
		t.Fatalf("continuation did not preserve call/result pairing: %#v", second)
	}
	if _, err := session.Receive(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("terminal Receive() error = %v", err)
	}
}

func TestDirectToolResultRequiresExplicitExecutionProvenance(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-1", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-provenance", Name: "read_config", Arguments: json.RawMessage(`{}`)}}},
	}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Model != nil && event.Model.Kind == "model_tool_call" {
			break
		}
	}
	err = session.Command(context.Background(), union.ExecutionCommand{
		Kind: union.CommandProvideToolResult, ActionID: "call-provenance",
		Payload: json.RawMessage(`{"call_id":"call-provenance","output":"unproven"}`),
	})
	if !errors.Is(err, direct.ErrUnsupportedCommand) {
		t.Fatalf("Command() error = %v", err)
	}
	if backend.invokeCalls != 1 {
		t.Fatalf("unproven result reached backend; invoke calls = %d", backend.invokeCalls)
	}
}

func TestDirectToolLoopUsesProviderContinuationState(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	state := &modelinvoker.State{Kind: modelinvoker.StateServerContinuation, Provider: "openai", Protocol: modelinvoker.ProtocolResponses, ID: "response-state-1"}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{
		{
			ID: "response-1", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall, State: state,
			Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-state", Name: "read_config", Arguments: json.RawMessage(`{"path":"config.go"}`)}}},
		},
		{ID: "response-2", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn},
	}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Model != nil && event.Model.Kind == "model_tool_call" {
			break
		}
	}
	payload := json.RawMessage(`{"call_id":"call-state","name":"read_config","output":"ok","executed":true,"result_origin":"external","side_effect_state":"none"}`)
	if err := session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandProvideToolResult, ActionID: "call-state", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if backend.invokeCalls != 2 {
		t.Fatalf("invoke calls = %d", backend.invokeCalls)
	}
	next := backend.calls[1].Request
	if next.State == nil || next.State.ID != state.ID || len(next.Input) != 1 || next.Input[0].Type != modelinvoker.InputTypeFunctionResult {
		t.Fatalf("state continuation = %#v", next)
	}
}

func TestDirectRejectsToolResultNameMutationWithoutConsumingPendingCall(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{
		{
			ID: "response-name", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
			Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-name", Name: "read_config", Arguments: json.RawMessage(`{"path":"config.go"}`)}}},
		},
		{ID: "response-final", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn},
	}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Model != nil && event.Model.Kind == "model_tool_call" {
			break
		}
	}
	mutated := json.RawMessage(`{"call_id":"call-name","name":"write_config","output":"bad","executed":true,"result_origin":"external","side_effect_state":"none"}`)
	if err := session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandProvideToolResult, ActionID: "call-name", Payload: mutated}); !errors.Is(err, direct.ErrUnsupportedCommand) {
		t.Fatalf("mutated name error = %v", err)
	}
	if backend.invokeCalls != 1 {
		t.Fatalf("mutated result reached backend: %d calls", backend.invokeCalls)
	}
	valid := json.RawMessage(`{"call_id":"call-name","name":"read_config","output":"ok","executed":true,"result_origin":"external","side_effect_state":"none"}`)
	if err := session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandProvideToolResult, ActionID: "call-name", Payload: valid}); err != nil {
		t.Fatalf("valid result after rejection: %v", err)
	}
	if backend.invokeCalls != 2 {
		t.Fatalf("valid result did not continue: %d calls", backend.invokeCalls)
	}
}

func TestDirectConcurrentDuplicateToolResultContinuesExactlyOnce(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{
		{
			ID: "response-concurrent", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
			Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-concurrent", Name: "read_config", Arguments: json.RawMessage(`{}`)}}},
		},
		{ID: "response-final", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn},
	}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Model != nil && event.Model.Kind == "model_tool_call" {
			break
		}
	}
	payload := json.RawMessage(`{"call_id":"call-concurrent","name":"read_config","output":"ok","executed":true,"result_origin":"external","side_effect_state":"none"}`)
	start := make(chan struct{})
	errorsByCall := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			errorsByCall <- session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandProvideToolResult, ActionID: "call-concurrent", Payload: payload})
		}()
	}
	close(start)
	successes, rejections := 0, 0
	for range 2 {
		err := <-errorsByCall
		switch {
		case err == nil:
			successes++
		case errors.Is(err, direct.ErrUnsupportedCommand), errors.Is(err, execution.ErrSessionClosed):
			rejections++
		default:
			t.Fatalf("concurrent result error = %v", err)
		}
	}
	if successes != 1 || rejections != 1 || backend.invokeCalls != 2 {
		t.Fatalf("success/rejection/invoke = %d/%d/%d", successes, rejections, backend.invokeCalls)
	}
}

func TestDirectParallelToolResultsContinueOnlyAfterAllCalls(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{
		{
			ID: "response-parallel", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
			Output: []modelinvoker.OutputItem{
				{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-b", Name: "read_config", Arguments: json.RawMessage(`{"path":"b"}`)}},
				{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-a", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}},
			},
		},
		{ID: "response-final", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn},
	}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	seen := map[union.ActionID]bool{}
	for len(seen) != 2 {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Model != nil && event.Model.Kind == "model_tool_call" {
			seen[event.Model.ActionID] = true
		}
	}
	for index, callID := range []string{"call-b", "call-a"} {
		payload, _ := json.Marshal(map[string]any{"call_id": callID, "name": "read_config", "output": callID, "executed": true, "result_origin": "external", "side_effect_state": "none"})
		if err := session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandProvideToolResult, ActionID: union.ActionID(callID), Payload: payload}); err != nil {
			t.Fatal(err)
		}
		if got := backend.invokeCalls; got != index+1 {
			t.Fatalf("invoke calls after result %d = %d", index, got)
		}
	}
	input := backend.calls[1].Request.Input
	if len(input) < 5 {
		t.Fatalf("parallel continuation input = %#v", input)
	}
	if input[len(input)-4].FunctionCall.ID != "call-a" || input[len(input)-2].FunctionCall.ID != "call-b" {
		t.Fatalf("parallel call/result order is not deterministic: %#v", input)
	}
}

func TestDirectCancelReportsAcknowledgedQuiescedAndCancelledAttempt(t *testing.T) {
	invocation, selected := directInvocation(t, true, false)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, streams: []*fakeStream{{}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandCancelExecution}); err != nil {
		t.Fatal(err)
	}
	var ack, quiesced, cancelledAttempt, terminal bool
	var lastSource uint64
	for {
		event, receiveErr := session.Receive(context.Background())
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Header.SourceSequence <= lastSource {
			t.Fatalf("source sequence regressed: %d <= %d", event.Header.SourceSequence, lastSource)
		}
		lastSource = event.Header.SourceSequence
		if event.Control != nil {
			ack = ack || event.Control.Kind == execution.ControlCancelAcknowledged
			quiesced = quiesced || event.Control.Kind == execution.ControlCancellationQuiesced
		}
		cancelledAttempt = cancelledAttempt || event.Mechanism != nil && event.Mechanism.Attempt != nil && event.Mechanism.Attempt.Status == union.AttemptStatusCancelled
		terminal = terminal || event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate
	}
	if !ack || !quiesced || !cancelledAttempt || !terminal {
		t.Fatalf("cancel evidence ack=%v quiesced=%v attempt=%v terminal=%v", ack, quiesced, cancelledAttempt, terminal)
	}
}

func TestDirectCleanStreamEOFBecomesProtocolViolationAndIndeterminate(t *testing.T) {
	invocation, selected := directInvocation(t, true, false)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, streams: []*fakeStream{{}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var violation, terminal bool
	for {
		event, receiveErr := session.Receive(context.Background())
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		violation = violation || event.Diagnostic != nil && event.Diagnostic.Kind == "protocol_violation"
		if event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate {
			var candidate execution.RouteTerminalCandidate
			if json.Unmarshal(event.Diagnostic.Payload, &candidate) != nil || candidate.Status != union.ExecutionStatusIndeterminate {
				t.Fatalf("terminal candidate = %#v", event.Diagnostic)
			}
			terminal = true
		}
	}
	if !violation || !terminal {
		t.Fatalf("violation=%v terminal=%v", violation, terminal)
	}
}

func TestDirectStreamPreservesDeltasAndEmitsCandidateOnlyAtCompletion(t *testing.T) {
	invocation, selected := directInvocation(t, true, false)
	response := modelinvoker.Response{ID: "response-stream", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn}
	backend := &fakeBackend{
		routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID,
		streams: []*fakeStream{{events: []modelinvoker.StreamEvent{
			{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1},
			{Type: modelinvoker.StreamEventTextDelta, Sequence: 2, TextDelta: "hello"},
			{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 3, Response: &response},
		}}},
	}
	adapter := newDirectAdapter(t, backend, selected)
	session, err := adapter.Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var kinds []string
	for {
		event, err := session.Receive(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if event.Model != nil {
			kinds = append(kinds, event.Model.Kind)
		}
		if event.Diagnostic != nil {
			kinds = append(kinds, event.Diagnostic.Kind)
		}
	}
	want := []string{"model_step_started", "content_delta", "model_step_completed", execution.EventKindRouteTerminalCandidate}
	if !equalStrings(kinds, want) {
		t.Fatalf("event kinds = %#v, want %#v", kinds, want)
	}
}

func TestDirectStreamMaterializesToolCallFoundOnlyInFinalResponse(t *testing.T) {
	invocation, selected := directInvocation(t, true, true)
	toolResponse := modelinvoker.Response{
		ID: "response-stream-tool", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "call-final-only", Name: "read_config", Arguments: json.RawMessage(`{"path":"config.go"}`)}}},
	}
	finalResponse := modelinvoker.Response{ID: "response-stream-final", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn}
	backend := &fakeBackend{
		routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID,
		streams: []*fakeStream{
			{events: []modelinvoker.StreamEvent{{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1}, {Type: modelinvoker.StreamEventResponseCompleted, Sequence: 2, Response: &toolResponse}}},
			{events: []modelinvoker.StreamEvent{{Type: modelinvoker.StreamEventResponseStarted, Sequence: 3}, {Type: modelinvoker.StreamEventResponseCompleted, Sequence: 4, Response: &finalResponse}}},
		},
	}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	seenCall, seenPending := false, false
	for !seenCall || !seenPending {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		seenCall = seenCall || event.Model != nil && event.Model.Kind == "model_tool_call" && event.Model.ActionID == "call-final-only"
		seenPending = seenPending || event.Item != nil && event.Item.Item.ActionID == "call-final-only" && event.Item.Item.Status == union.ItemStatusPending
	}
	payload := json.RawMessage(`{"call_id":"call-final-only","name":"read_config","output":"ok","executed":true,"result_origin":"external","side_effect_state":"none"}`)
	if err := session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandProvideToolResult, ActionID: "call-final-only", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	seenTerminal := false
	for !seenTerminal {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		seenTerminal = event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate
	}
	if backend.streamCalls != 2 || len(backend.calls) != 2 {
		t.Fatalf("stream continuation calls = %d/%d", backend.streamCalls, len(backend.calls))
	}
	continuation := backend.calls[1].Request.Input
	if len(continuation) < 3 || continuation[len(continuation)-2].FunctionCall == nil || continuation[len(continuation)-2].FunctionCall.ID != "call-final-only" || continuation[len(continuation)-1].FunctionResult == nil || continuation[len(continuation)-1].FunctionResult.CallID != "call-final-only" {
		t.Fatalf("final-only call was not paired in continuation: %#v", continuation)
	}
}

func TestDirectStreamCompletedCallThenConflictingTerminalPublishesNothing(t *testing.T) {
	invocation, selected := directInvocation(t, true, true)
	completed := modelinvoker.FunctionCall{ID: "call-conflict", Name: "read_config", Arguments: json.RawMessage(`{"path":"before"}`)}
	terminalCall := modelinvoker.FunctionCall{ID: "call-conflict", Name: "read_config", Arguments: json.RawMessage(`{"path":"after"}`)}
	terminal := modelinvoker.Response{
		ID: "response-conflict", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &terminalCall}},
	}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, streams: []*fakeStream{{events: []modelinvoker.StreamEvent{
		{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1, ResponseID: terminal.ID},
		{Type: modelinvoker.StreamEventFunctionCallStarted, Sequence: 2, ResponseID: terminal.ID, FunctionCall: &modelinvoker.FunctionCall{ID: completed.ID, Name: completed.Name}},
		{Type: modelinvoker.StreamEventFunctionCallCompleted, Sequence: 3, ResponseID: terminal.ID, FunctionCall: &completed},
		{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 4, ResponseID: terminal.ID, Response: &terminal},
	}}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	assertAtomicToolCallRejection(t, session)
}

func TestDirectStreamProvisionalCompletePublishesOnceAfterMatchingTerminal(t *testing.T) {
	invocation, selected := directInvocation(t, true, true)
	call := modelinvoker.FunctionCall{ID: "call-atomic-success", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	terminal := modelinvoker.Response{
		ID: "response-atomic-success", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}},
	}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, streams: []*fakeStream{{events: []modelinvoker.StreamEvent{
		{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1, ResponseID: terminal.ID},
		{Type: modelinvoker.StreamEventFunctionCallStarted, Sequence: 2, ResponseID: terminal.ID, FunctionCall: &modelinvoker.FunctionCall{ID: call.ID, Name: call.Name}},
		{Type: modelinvoker.StreamEventFunctionCallCompleted, Sequence: 3, ResponseID: terminal.ID, FunctionCall: &call},
		{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 4, ResponseID: terminal.ID, Response: &terminal},
	}}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	toolCalls, pendingItems, observations := 0, 0, 0
	for pendingItems == 0 {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Model != nil {
			switch event.Model.Kind {
			case "tool_input_started", "tool_input_delta":
				t.Fatalf("provisional event escaped before canonical publication: %#v", event.Model)
			case modelinvoker.ToolCallCandidateObservationModelEventKindV1:
				observations++
				projection, decodeErr := modelinvoker.DecodeToolCallCandidateObservationProjectionV1(event.Model.Payload)
				if decodeErr != nil || projection.Ref.Source.SourceSequence != event.Header.SourceSequence || len(projection.Observation.Calls) != 1 {
					t.Fatalf("stream observation projection = %#v, %v", projection, decodeErr)
				}
			case "model_tool_call":
				toolCalls++
			}
		}
		if event.Item != nil && event.Item.Item.Status == union.ItemStatusPending {
			pendingItems++
		}
	}
	if observations != 1 || toolCalls != 1 || pendingItems != 1 {
		t.Fatalf("atomic observation/tool call/pending publication = %d/%d/%d", observations, toolCalls, pendingItems)
	}
}

func TestDirectStreamIndeterminateEnsureUsesSameAtomicRecoveryBarrier(t *testing.T) {
	invocation, selected := directInvocation(t, true, true)
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	repository := &faultProjectionRepository{delegate: store, indeterminateBeforeEnsure: true}
	call := modelinvoker.FunctionCall{ID: "call-stream-recovery", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	terminal := modelinvoker.Response{
		ID: "response-stream-recovery", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}},
	}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, streams: []*fakeStream{{events: []modelinvoker.StreamEvent{
		{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1, ResponseID: terminal.ID},
		{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 2, ResponseID: terminal.ID, Response: &terminal},
	}}}}
	session, err := newDirectAdapterWithProjectionRepository(t, backend, selected, repository).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	projection, _ := receiveObservationAndCompatibility(t, session, 1)
	if repository.callCount() != 2 || repository.firstRef != projection.Ref || repository.secondRef != projection.Ref || store.StatsV1().Records != 1 {
		t.Fatalf("stream recovery changed canonical identity: calls=%d refs=%#v/%#v stats=%#v", repository.callCount(), repository.firstRef, repository.secondRef, store.StatsV1())
	}
}

func TestDirectStreamNMoreThanOneValidButNonExactEnsureProjectionFailsClosed(t *testing.T) {
	invocation, selected := directInvocation(t, true, true)
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	repository := &faultProjectionRepository{delegate: store, returnDifferentProjection: true}
	first := modelinvoker.FunctionCall{ID: "call-stream-non-exact-a", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	second := modelinvoker.FunctionCall{ID: "call-stream-non-exact-b", Name: "read_config", Arguments: json.RawMessage(`{"path":"b"}`)}
	terminal := modelinvoker.Response{
		ID: "response-stream-non-exact", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &first}, {Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &second}},
	}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, streams: []*fakeStream{{events: []modelinvoker.StreamEvent{
		{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1, ResponseID: terminal.ID},
		{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 2, ResponseID: terminal.ID, Response: &terminal},
	}}}}
	session, err := newDirectAdapterWithProjectionRepository(t, backend, selected, repository).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	assertAtomicToolCallRejection(t, session)
	if repository.callCount() != 1 || store.StatsV1().EnsureCalls != 0 || store.StatsV1().Records != 0 {
		t.Fatalf("stream N>1 non-exact Ensure result mutated state or retried: %d/%#v", repository.callCount(), store.StatsV1())
	}
}

func TestDirectStreamNMoreThanOnePublishesWholeObservationBeforeNonAuthoritativeCalls(t *testing.T) {
	invocation, selected := directInvocation(t, true, true)
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	first := modelinvoker.FunctionCall{ID: "call-b", Name: "read_config", Arguments: json.RawMessage(`{"path":"b"}`)}
	second := modelinvoker.FunctionCall{ID: "call-a", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	terminal := modelinvoker.Response{
		ID: "response-stream-batch", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &first}, {Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &second}},
	}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, streams: []*fakeStream{{events: []modelinvoker.StreamEvent{
		{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1, ResponseID: terminal.ID},
		{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 2, ResponseID: terminal.ID, Response: &terminal},
	}}}}
	session, err := newDirectAdapterWithProjectionRepository(t, backend, selected, store).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	projection, compatibility := receiveObservationAndCompatibility(t, session, 2)
	if len(projection.Observation.Calls) != 2 || projection.Observation.Calls[0].CallID != first.ID || projection.Observation.Calls[0].Ordinal != 0 || projection.Observation.Calls[1].CallID != second.ID || projection.Observation.Calls[1].Ordinal != 1 {
		t.Fatalf("N>1 observation was split or reordered: %#v", projection.Observation.Calls)
	}
	for index := range compatibility {
		assertCompatibilityBinding(t, compatibility[index], projection.Ref, uint32(index))
	}
	stored, err := store.InspectExactProjectionV1(context.Background(), projection.Ref)
	if err != nil || !reflect.DeepEqual(stored, projection) || store.StatsV1().EnsureCalls != 1 || store.StatsV1().Records != 1 {
		t.Fatalf("stream N>1 store = %#v/%v/%#v", stored, err, store.StatsV1())
	}
}

func TestDirectStreamTerminalResponseIDMismatchPublishesNothing(t *testing.T) {
	invocation, selected := directInvocation(t, true, true)
	call := modelinvoker.FunctionCall{ID: "call-response-mismatch", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	terminal := modelinvoker.Response{
		ID: "response-other", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}},
	}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, streams: []*fakeStream{{events: []modelinvoker.StreamEvent{
		{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1, ResponseID: "response-bound"},
		{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 2, ResponseID: "response-bound", Response: &terminal},
	}}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	assertAtomicToolCallRejection(t, session)
}

func TestDirectStreamTerminalEmptyResponseIDInheritsBoundSource(t *testing.T) {
	invocation, selected := directInvocation(t, true, true)
	call := modelinvoker.FunctionCall{ID: "call-response-inherited", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	terminal := modelinvoker.Response{
		Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}},
	}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, streams: []*fakeStream{{events: []modelinvoker.StreamEvent{
		{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1, ResponseID: "response-bound"},
		{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 2, Response: &terminal},
	}}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	projection, compatibility := receiveObservationAndCompatibility(t, session, 1)
	if projection.Ref.Source.ResponseID != "response-bound" {
		t.Fatalf("projection source response ID = %q", projection.Ref.Source.ResponseID)
	}
	assertCompatibilityBinding(t, compatibility[0], projection.Ref, 0)
}

func TestDirectStreamTerminalEmptyResponseIDWithoutBindingPublishesNothing(t *testing.T) {
	invocation, selected := directInvocation(t, true, true)
	call := modelinvoker.FunctionCall{ID: "call-response-missing", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	terminal := modelinvoker.Response{
		Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &call}},
	}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, streams: []*fakeStream{{events: []modelinvoker.StreamEvent{
		{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 1, Response: &terminal},
	}}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	assertAtomicToolCallRejection(t, session)
}

func TestDirectStreamMultiToolOneInvalidPublishesNothing(t *testing.T) {
	invocation, selected := directInvocation(t, true, true)
	valid := modelinvoker.FunctionCall{ID: "call-valid", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	invalid := modelinvoker.FunctionCall{ID: "call-invalid", Name: "read_config", Arguments: json.RawMessage(`{"path":"a","path":"b"}`)}
	terminal := modelinvoker.Response{
		ID: "response-multi-invalid", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &valid}, {Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &invalid}},
	}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, streams: []*fakeStream{{events: []modelinvoker.StreamEvent{
		{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1, ResponseID: terminal.ID},
		{Type: modelinvoker.StreamEventResponseCompleted, Sequence: 2, ResponseID: terminal.ID, Response: &terminal},
	}}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	assertAtomicToolCallRejection(t, session)
}

func TestDirectStreamDuplicateCompletePublishesNothing(t *testing.T) {
	invocation, selected := directInvocation(t, true, true)
	call := modelinvoker.FunctionCall{ID: "call-duplicate-complete", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, streams: []*fakeStream{{events: []modelinvoker.StreamEvent{
		{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1, ResponseID: "response-duplicate-complete"},
		{Type: modelinvoker.StreamEventFunctionCallStarted, Sequence: 2, ResponseID: "response-duplicate-complete", FunctionCall: &modelinvoker.FunctionCall{ID: call.ID, Name: call.Name}},
		{Type: modelinvoker.StreamEventFunctionCallCompleted, Sequence: 3, ResponseID: "response-duplicate-complete", FunctionCall: &call},
		{Type: modelinvoker.StreamEventFunctionCallCompleted, Sequence: 4, ResponseID: "response-duplicate-complete", FunctionCall: &call},
	}}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	assertAtomicToolCallRejection(t, session)
}

func TestDirectStreamLostTerminalAfterCompletePublishesNothing(t *testing.T) {
	invocation, selected := directInvocation(t, true, true)
	call := modelinvoker.FunctionCall{ID: "call-lost-terminal", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, streams: []*fakeStream{{events: []modelinvoker.StreamEvent{
		{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1, ResponseID: "response-lost-terminal"},
		{Type: modelinvoker.StreamEventFunctionCallStarted, Sequence: 2, ResponseID: "response-lost-terminal", FunctionCall: &modelinvoker.FunctionCall{ID: call.ID, Name: call.Name}},
		{Type: modelinvoker.StreamEventFunctionCallCompleted, Sequence: 3, ResponseID: "response-lost-terminal", FunctionCall: &call},
	}}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	assertAtomicToolCallRejection(t, session)
}

func TestDirectStreamCancelAfterProvisionalCompletePublishesNothing(t *testing.T) {
	invocation, selected := directInvocation(t, true, true)
	call := modelinvoker.FunctionCall{ID: "call-cancelled", Name: "read_config", Arguments: json.RawMessage(`{"path":"a"}`)}
	stream := newGatedFakeStream([]modelinvoker.StreamEvent{
		{Type: modelinvoker.StreamEventResponseStarted, Sequence: 1, ResponseID: "response-cancelled"},
		{Type: modelinvoker.StreamEventFunctionCallStarted, Sequence: 2, ResponseID: "response-cancelled", FunctionCall: &modelinvoker.FunctionCall{ID: call.ID, Name: call.Name}},
		{Type: modelinvoker.StreamEventFunctionCallCompleted, Sequence: 3, ResponseID: "response-cancelled", FunctionCall: &call},
	})
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, customStreams: []direct.ModelStream{stream}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	var observed []union.UnifiedExecutionEvent
	for {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		observed = append(observed, event)
		if event.Model != nil && event.Model.Kind == "model_step_started" {
			break
		}
	}
	received := make(chan struct {
		event union.UnifiedExecutionEvent
		err   error
	}, 1)
	go func() {
		event, receiveErr := session.Receive(context.Background())
		received <- struct {
			event union.UnifiedExecutionEvent
			err   error
		}{event: event, err: receiveErr}
	}()
	select {
	case <-stream.blocked:
	case <-time.After(time.Second):
		t.Fatal("stream did not buffer the completed call while awaiting terminal")
	}
	if err := session.Command(context.Background(), union.ExecutionCommand{Kind: union.CommandCancelExecution}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-received:
		if result.err != nil {
			t.Fatal(result.err)
		}
		observed = append(observed, result.event)
	case <-time.After(time.Second):
		t.Fatal("cancel did not unblock Receive")
	}
	for {
		event, receiveErr := session.Receive(context.Background())
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		observed = append(observed, event)
	}
	for _, event := range observed {
		if event.Model != nil {
			switch event.Model.Kind {
			case "tool_input_started", "tool_input_delta", "model_tool_call", modelinvoker.ToolCallCandidateObservationModelEventKindV1:
				t.Fatalf("cancel exposed provisional tool event: %#v", event.Model)
			}
		}
		if event.Item != nil && event.Item.Item.Status == union.ItemStatusPending {
			t.Fatalf("cancel exposed pending item: %#v", event.Item)
		}
	}
}

func assertAtomicToolCallRejection(t *testing.T, session execution.Session) {
	t.Helper()
	seenViolation := false
	for {
		event, receiveErr := session.Receive(context.Background())
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Model != nil {
			switch event.Model.Kind {
			case "tool_input_started", "tool_input_delta", "model_tool_call", modelinvoker.ToolCallCandidateObservationModelEventKindV1:
				t.Fatalf("provisional tool event escaped atomic finalization: %#v", event.Model)
			}
		}
		if event.Item != nil && event.Item.Item.Status == union.ItemStatusPending {
			t.Fatalf("pending item escaped atomic finalization: %#v", event.Item)
		}
		seenViolation = seenViolation || event.Diagnostic != nil && event.Diagnostic.Kind == "protocol_violation"
	}
	if !seenViolation {
		t.Fatal("atomic rejection emitted no protocol violation")
	}
}

type compatibilityToolCallProjection struct {
	CallID               string                                         `json:"call_id"`
	Name                 string                                         `json:"name"`
	Arguments            json.RawMessage                                `json:"arguments"`
	Authority            string                                         `json:"authority"`
	GatewayAuthoritative bool                                           `json:"gateway_authoritative"`
	ObservationRef       modelinvoker.ToolCallCandidateObservationRefV1 `json:"observation_ref"`
	Ordinal              uint32                                         `json:"ordinal"`
}

func receiveObservationAndCompatibility(t *testing.T, session execution.Session, expectedCalls int) (modelinvoker.ToolCallCandidateObservationProjectionV1, []compatibilityToolCallProjection) {
	t.Helper()
	var projection modelinvoker.ToolCallCandidateObservationProjectionV1
	compatibility := make([]compatibilityToolCallProjection, 0, expectedCalls)
	seenObservation := false
	for len(compatibility) < expectedCalls {
		event, receiveErr := session.Receive(context.Background())
		if receiveErr != nil {
			t.Fatal(receiveErr)
		}
		if event.Model == nil {
			continue
		}
		switch event.Model.Kind {
		case modelinvoker.ToolCallCandidateObservationModelEventKindV1:
			if seenObservation {
				t.Fatal("tool call observation projection was published more than once")
			}
			var decodeErr error
			projection, decodeErr = modelinvoker.DecodeToolCallCandidateObservationProjectionV1(event.Model.Payload)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if projection.Ref.Source.SourceSequence != event.Header.SourceSequence {
				t.Fatalf("projection/event source coordinate differs: %d/%d", projection.Ref.Source.SourceSequence, event.Header.SourceSequence)
			}
			seenObservation = true
		case "model_tool_call":
			if !seenObservation {
				t.Fatal("partial tool call escaped before the whole observation")
			}
			var item compatibilityToolCallProjection
			if err := json.Unmarshal(event.Model.Payload, &item); err != nil {
				t.Fatal(err)
			}
			compatibility = append(compatibility, item)
		}
	}
	if !seenObservation {
		t.Fatal("no authoritative tool call observation projection was published")
	}
	return projection, compatibility
}

func assertCompatibilityBinding(t *testing.T, compatibility compatibilityToolCallProjection, ref modelinvoker.ToolCallCandidateObservationRefV1, ordinal uint32) {
	t.Helper()
	if compatibility.Authority != modelinvoker.ToolCallCompatibilityAuthorityV1 || compatibility.GatewayAuthoritative || compatibility.Ordinal != ordinal || compatibility.ObservationRef != ref {
		t.Fatalf("compatibility event has authority or lineage: %#v", compatibility)
	}
}

func TestDirectInvalidProviderToolCallTerminatesWithoutWaitingForResult(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID, invokeResponses: []modelinvoker.Response{{
		ID: "response-invalid-tool", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{ID: "", Name: "read_config", Arguments: json.RawMessage(`{}`)}}},
	}}}
	session, err := newDirectAdapter(t, backend, selected).Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	seenViolation, seenTerminal := false, false
	for {
		event, receiveErr := session.Receive(ctx)
		if errors.Is(receiveErr, io.EOF) {
			break
		}
		if receiveErr != nil {
			t.Fatalf("Receive() = %v", receiveErr)
		}
		seenViolation = seenViolation || event.Diagnostic != nil && event.Diagnostic.Code == "invalid_tool_call"
		seenTerminal = seenTerminal || event.Diagnostic != nil && event.Diagnostic.Kind == execution.EventKindRouteTerminalCandidate
	}
	if !seenViolation || !seenTerminal {
		t.Fatalf("invalid tool evidence violation=%v terminal=%v", seenViolation, seenTerminal)
	}
}

func TestDirectMapperRejectsHarnessOwnedToolBeforeBackend(t *testing.T) {
	invocation, selected := directInvocation(t, false, true)
	invocation.Request.Tools[0].ExecutionOwner = union.ExecutionOwnerHarness
	invocation.Plan.Digest = ""
	delete(invocation.Plan.Metadata, "request_digest")
	invocation, err := execution.NewInvocation(invocation.Request, invocation.Plan)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}
	adapter := newDirectAdapter(t, backend, selected)
	report, err := adapter.Preflight(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	if report.Accepted || report.RejectionCode != "direct_mapping_rejected" || backend.resolveCalls != 0 {
		t.Fatalf("report = %#v backend = %#v", report, backend)
	}
}

func TestDirectAdapterRunsThroughUnifiedRuntimeWithVerifiedStructuredEffect(t *testing.T) {
	invocation, selected := directInvocation(t, false, false)
	backend := &fakeBackend{
		routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID,
		invokeResponses: []modelinvoker.Response{{
			ID: "response-runtime", Model: selected.Selection.ModelID, Status: modelinvoker.ResponseStatusCompleted,
			StopReason: modelinvoker.StopReasonEndTurn,
			Output:     []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemText, Text: `{}`}},
			Usage:      modelinvoker.Usage{InputTokens: 10, OutputTokens: 4, TotalTokens: 14},
		}},
	}
	adapter := newDirectAdapter(t, backend, selected)
	registry := execution.NewRegistry()
	if err := registry.Register(context.Background(), adapter); err != nil {
		t.Fatal(err)
	}
	mechanism := invocation.Plan.Mechanisms[0]
	for _, candidate := range invocation.Plan.Mechanisms {
		if candidate.IntentID == "structured" && candidate.PreferredRank < mechanism.PreferredRank {
			mechanism = candidate
		}
	}
	attemptID := union.MechanismAttemptID("direct:" + string(mechanism.ID) + ":attempt:1")
	validated, err := effect.ValidateStructuredOutputWithMechanism(
		"effect-runtime", "verify-runtime", "structured", attemptID,
		[]byte(`{}`), invocation.Request.OutputContract.JSONSchema,
		effect.StructuredMechanism{Kind: union.StructuredStrictJSONSchema, Origin: mechanism.Origin, Fidelity: mechanism.SemanticFidelity, Transport: "responses"},
		0, directTestNow.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := execution.NewRuntime(execution.RuntimeConfig{
		Registry: registry,
		Reconciler: directReconcilerFunc(func(_ context.Context, input execution.ReconcileInput) (execution.ReconcileReport, error) {
			if _, exists := input.State.Attempts[attemptID]; !exists {
				t.Fatalf("attempt %q was not observed", attemptID)
			}
			return execution.ReconcileReport{Effects: []union.EffectRecord{validated.Effect}, SideEffectState: union.SideEffectObserved, Quiesced: true}, nil
		}),
		Verifier: directVerifierFunc(func(context.Context, execution.VerifyInput) (execution.VerificationReport, error) {
			return execution.VerificationReport{Verifications: []union.VerificationRecord{validated.Verification}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Execute(context.Background(), adapterIdentity(adapter), invocation)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != union.ExecutionStatusSucceeded || result.VerificationStatus != union.VerificationVerified || len(result.Effects) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(result.FinalContent) != 1 || result.FinalContent[0].Text != `{}` || len(result.UsageMetrics) != 3 {
		t.Fatalf("content/usage = %#v / %#v", result.FinalContent, result.UsageMetrics)
	}
}

type directReconcilerFunc func(context.Context, execution.ReconcileInput) (execution.ReconcileReport, error)

func (function directReconcilerFunc) Reconcile(ctx context.Context, input execution.ReconcileInput) (execution.ReconcileReport, error) {
	return function(ctx, input)
}

type directVerifierFunc func(context.Context, execution.VerifyInput) (execution.VerificationReport, error)

func (function directVerifierFunc) Verify(ctx context.Context, input execution.VerifyInput) (execution.VerificationReport, error) {
	return function(ctx, input)
}

func adapterIdentity(adapter *direct.Adapter) string {
	descriptor, _ := adapter.Describe(context.Background())
	return descriptor.Identity.ID
}

func directInvocation(t *testing.T, stream, withTool bool) (execution.Invocation, profile.SemanticRouteProfile) {
	t.Helper()
	profiles, err := profile.RepresentativeProfiles(directTestNow)
	if err != nil {
		t.Fatal(err)
	}
	var selected profile.SemanticRouteProfile
	for _, candidate := range profiles {
		if candidate.ID == profile.ProfileOpenAIDirect {
			selected = candidate
			break
		}
	}
	registry, err := profile.NewRegistry(directTestNow, profiles...)
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := profile.NewCompiler(registry, directTestNow)
	if err != nil {
		t.Fatal(err)
	}
	request := union.UnifiedExecutionRequest{
		SemanticVersion: union.SemanticVersionV1, ExecutionID: "exec.direct", ExecutionKind: union.ExecutionKindModel,
		ProfileSelector:   union.ProfileSelector{Exact: &union.VersionedIdentity{ID: string(profile.ProfileOpenAIDirect), Version: "v1candidate"}},
		Input:             []union.InputItem{{ID: "message-1", Kind: "message", Role: "user", Content: []union.ContentPart{{Kind: "text", Text: "return a result"}}}},
		Instructions:      []union.Instruction{{ID: "instruction-1", Authority: "developer", Scope: "execution", ConflictPolicy: "higher_authority_wins", Content: []union.ContentPart{{Kind: "text", Text: "Use the allowed tools only."}}}},
		ToolPolicy:        union.ToolPolicy{DefaultApproval: "on_side_effect", Parallelism: 1, MaxActions: 2},
		OutputContract:    union.OutputContract{AcceptedContentKinds: []string{"json"}, CompletionMode: "final", JSONSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)},
		SessionIntent:     union.SessionIntent{Mode: "new"},
		ExecutionPolicy:   union.ExecutionPolicy{Stream: stream, Sandbox: "workspace_write", CWDReference: "/workspace", NetworkPolicy: "denied", UserPresence: "present", Foreground: "required", InteractionMode: "interactive", MaxConcurrency: 1},
		Budget:            union.Budget{MaxWallTime: time.Minute, MaxToolActions: 2},
		DegradationPolicy: union.DegradationPolicy{Default: union.DegradationDefaultReject},
		IntentGraph:       union.IntentGraph{Nodes: []union.IntentNode{{ID: "structured", Kind: union.IntentProduceStructured, Target: "summary", Required: true, AcceptedFidelity: []union.SemanticFidelity{union.SemanticFidelityExact}}}},
	}
	if withTool {
		request.Tools = []union.ToolDefinition{{
			ID: "read-config", Name: "read_config", Kind: "function", ExecutionOwner: union.ExecutionOwnerPraxis,
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
		}}
	}
	compiled, err := compiler.Compile(profile.CompileInput{
		Request: request, PaperOnly: true,
		ActualManifest: profile.InjectionManifest{SchemaVersion: "v1candidate", ProbeStatus: profile.ManifestProbeNotRun},
	})
	if err != nil {
		t.Fatal(err)
	}
	return execution.Invocation{Request: request, Plan: compiled.Plan}, selected
}

func newDirectAdapter(t *testing.T, backend direct.Backend, selected profile.SemanticRouteProfile) *direct.Adapter {
	t.Helper()
	store := modelinvoker.NewInMemoryToolCallCandidateObservationProjectionStoreV1()
	return newDirectAdapterWithProjectionRepository(t, backend, selected, store)
}

func newDirectAdapterWithProjectionRepository(
	t *testing.T,
	backend direct.Backend,
	selected profile.SemanticRouteProfile,
	repository modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1,
) *direct.Adapter {
	t.Helper()
	adapter, err := direct.New(direct.Config{
		Identity: union.VersionedIdentity{ID: "direct-test", Version: "v1"}, Backend: backend,
		RouteID: selected.Selection.BaseRouteID, Model: selected.Selection.ModelID,
		Invocation:                    upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground},
		ToolCallObservationRepository: repository,
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

type faultProjectionRepository struct {
	mu                             sync.Mutex
	delegate                       modelinvoker.ToolCallCandidateObservationProjectionRepositoryV1
	loseReplyAfterEnsure           bool
	loseReplyAfterEveryEnsure      bool
	indeterminateBeforeEnsure      bool
	indeterminateBeforeEveryEnsure bool
	failBeforeEnsureKind           modelinvoker.ToolCallCandidateObservationProjectionErrorKindV1
	returnDifferentProjection      bool
	calls                          int
	firstRef                       modelinvoker.ToolCallCandidateObservationRefV1
	secondRef                      modelinvoker.ToolCallCandidateObservationRefV1
	firstProjection                modelinvoker.ToolCallCandidateObservationProjectionV1
	secondProjection               modelinvoker.ToolCallCandidateObservationProjectionV1
}

func (repository *faultProjectionRepository) EnsureSealedProjectionV1(ctx context.Context, projection modelinvoker.ToolCallCandidateObservationProjectionV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	repository.mu.Lock()
	repository.calls++
	call := repository.calls
	if call == 1 {
		repository.firstRef = projection.Ref
		repository.firstProjection = projection.Clone()
	} else if call == 2 {
		repository.secondRef = projection.Ref
		repository.secondProjection = projection.Clone()
	}
	loseAfterEnsure := call == 1 && repository.loseReplyAfterEnsure || repository.loseReplyAfterEveryEnsure
	loseBeforeEnsure := call == 1 && repository.indeterminateBeforeEnsure || repository.indeterminateBeforeEveryEnsure
	failKind := repository.failBeforeEnsureKind
	returnDifferent := repository.returnDifferentProjection
	repository.mu.Unlock()
	if failKind != "" {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, &modelinvoker.ToolCallCandidateObservationProjectionErrorV1{
			Kind: failKind, Operation: "ensure", Message: "atomic Ensure failed before its linearization point",
		}
	}
	if loseBeforeEnsure {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, indeterminateProjectionError("Ensure outcome was lost before the repository linearization point")
	}
	if returnDifferent {
		return differentValidProjection(projection)
	}
	ensured, err := repository.delegate.EnsureSealedProjectionV1(ctx, projection)
	if err != nil {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, err
	}
	if loseAfterEnsure {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, indeterminateProjectionError("Ensure reply was lost after the repository linearization point")
	}
	return ensured, nil
}

func differentValidProjection(sealed modelinvoker.ToolCallCandidateObservationProjectionV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	responseID := sealed.Ref.Source.ResponseID + ".different"
	observation, err := modelinvoker.FinalizeToolCallCandidateObservationV1(sealed.Observation.InvocationDigest, modelinvoker.Response{
		ID: responseID, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonToolCall,
		Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemFunctionCall, FunctionCall: &modelinvoker.FunctionCall{
			ID: "different-call", Name: "read_config", Arguments: json.RawMessage(`{"path":"different"}`),
		}}},
	})
	if err != nil {
		return modelinvoker.ToolCallCandidateObservationProjectionV1{}, err
	}
	return modelinvoker.NewToolCallCandidateObservationProjectionV1(
		sealed.Ref.InvocationID, sealed.Ref.Source.SourceSequence+1, responseID, observation,
	)
}

func (repository *faultProjectionRepository) callCount() int {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	return repository.calls
}

func indeterminateProjectionError(message string) error {
	return &modelinvoker.ToolCallCandidateObservationProjectionErrorV1{
		Kind: modelinvoker.ToolCallCandidateObservationProjectionErrorIndeterminate, Operation: "ensure", Message: message,
	}
}

// splitProjectionPorts deliberately delegates write and read to different
// stores. It proves that the legacy pair cannot satisfy Direct's atomic
// Repository capability because it has no Ensure method.
type splitProjectionPorts struct {
	publisher modelinvoker.ToolCallCandidateObservationProjectionPublisherV1
	reader    modelinvoker.ToolCallCandidateObservationProjectionReaderV1
}

func (ports splitProjectionPorts) PublishSealedProjectionV1(ctx context.Context, projection modelinvoker.ToolCallCandidateObservationProjectionV1) (modelinvoker.ToolCallCandidateObservationRefV1, error) {
	return ports.publisher.PublishSealedProjectionV1(ctx, projection)
}

func (ports splitProjectionPorts) InspectExactProjectionV1(ctx context.Context, ref modelinvoker.ToolCallCandidateObservationRefV1) (modelinvoker.ToolCallCandidateObservationProjectionV1, error) {
	return ports.reader.InspectExactProjectionV1(ctx, ref)
}

type fakeBackend struct {
	mu              sync.Mutex
	routeID         upstream.RouteID
	model           string
	resolveCalls    int
	invokeCalls     int
	streamCalls     int
	lastCall        modelinvoker.RouteCall
	calls           []modelinvoker.RouteCall
	invokeResponses []modelinvoker.Response
	streams         []*fakeStream
	customStreams   []direct.ModelStream
}

func (backend *fakeBackend) Resolve(_ context.Context, call modelinvoker.RouteCall) (routegateway.Resolution, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.resolveCalls++
	backend.lastCall = call
	return routegateway.Resolution{Route: modelinvoker.RouteSelection{RouteID: backend.routeID, Model: backend.model}}, nil
}

func (backend *fakeBackend) Invoke(_ context.Context, call modelinvoker.RouteCall) (routegateway.InvokeResult, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.invokeCalls++
	backend.lastCall = call
	backend.calls = append(backend.calls, call)
	if len(backend.invokeResponses) == 0 {
		return routegateway.InvokeResult{}, errors.New("unexpected Invoke")
	}
	response := backend.invokeResponses[0]
	backend.invokeResponses = backend.invokeResponses[1:]
	return routegateway.InvokeResult{Response: response}, nil
}

func (backend *fakeBackend) OpenStream(_ context.Context, call modelinvoker.RouteCall) (direct.ModelStream, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.streamCalls++
	backend.lastCall = call
	backend.calls = append(backend.calls, call)
	if len(backend.customStreams) != 0 {
		stream := backend.customStreams[0]
		backend.customStreams = backend.customStreams[1:]
		return stream, nil
	}
	if len(backend.streams) == 0 {
		return nil, errors.New("unexpected OpenStream")
	}
	stream := backend.streams[0]
	backend.streams = backend.streams[1:]
	return stream, nil
}

type fakeStream struct {
	events []modelinvoker.StreamEvent
	index  int
	closed bool
}

func (stream *fakeStream) Next() bool {
	if stream.closed || stream.index >= len(stream.events) {
		return false
	}
	stream.index++
	return true
}
func (stream *fakeStream) Event() modelinvoker.StreamEvent { return stream.events[stream.index-1] }
func (stream *fakeStream) Err() error                      { return nil }
func (stream *fakeStream) Close() error {
	stream.closed = true
	return nil
}

type gatedFakeStream struct {
	mu          sync.Mutex
	events      []modelinvoker.StreamEvent
	index       int
	current     modelinvoker.StreamEvent
	closed      bool
	blocked     chan struct{}
	release     chan struct{}
	blockedOnce sync.Once
	releaseOnce sync.Once
}

func newGatedFakeStream(events []modelinvoker.StreamEvent) *gatedFakeStream {
	return &gatedFakeStream{events: events, blocked: make(chan struct{}), release: make(chan struct{})}
}

func (stream *gatedFakeStream) Next() bool {
	stream.mu.Lock()
	if stream.closed {
		stream.mu.Unlock()
		return false
	}
	if stream.index < len(stream.events) {
		stream.current = stream.events[stream.index]
		stream.index++
		stream.mu.Unlock()
		return true
	}
	stream.blockedOnce.Do(func() { close(stream.blocked) })
	release := stream.release
	stream.mu.Unlock()
	<-release
	return false
}

func (stream *gatedFakeStream) Event() modelinvoker.StreamEvent {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	return stream.current
}

func (stream *gatedFakeStream) Err() error { return nil }

func (stream *gatedFakeStream) Close() error {
	stream.mu.Lock()
	stream.closed = true
	stream.mu.Unlock()
	stream.releaseOnce.Do(func() { close(stream.release) })
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
