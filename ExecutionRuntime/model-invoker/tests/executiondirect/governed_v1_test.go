package executiondirect_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/direct"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestDirectGovernedBindingS1S2UsesOnlyGovernedBackend(t *testing.T) {
	invocation, selected := directInvocation(t, false, false)
	binding := directGovernedBindingV1(t, invocation, selected.Selection.BaseRouteID)
	reader := &directGovernedBindingReaderV1{binding: binding}
	backend := &directGovernedBackendV1{fakeBackend: &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}}
	adapter, err := direct.New(direct.Config{Identity: union.VersionedIdentity{ID: "direct-governed", Version: "v1"}, Backend: backend, RouteID: selected.Selection.BaseRouteID, Model: selected.Selection.ModelID, Invocation: upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground}, GovernedInvocationBindings: reader})
	if err != nil {
		t.Fatal(err)
	}
	report, err := adapter.Preflight(context.Background(), invocation)
	if err != nil || !report.Accepted {
		t.Fatalf("Preflight = %#v, %v", report, err)
	}
	session, err := adapter.Open(context.Background(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if reader.calls != 2 || backend.resolveCalls != 0 || backend.governedCalls != 1 || backend.invokeCalls != 0 || backend.streamCalls != 0 {
		t.Fatalf("reader/resolve/governed/legacy = %d/%d/%d/%d/%d", reader.calls, backend.resolveCalls, backend.governedCalls, backend.invokeCalls, backend.streamCalls)
	}
	if backend.command.PreparedRef != binding.PreparedRef || backend.command.AttemptRequestDigest != binding.UnifiedRequestDigest || backend.command.Call.RouteID != binding.RouteID {
		t.Fatalf("governed command drifted: %#v", backend.command)
	}
}

func TestDirectGovernedBindingDriftFailsBeforeAnyInvoke(t *testing.T) {
	invocation, selected := directInvocation(t, false, false)
	binding := directGovernedBindingV1(t, invocation, selected.Selection.BaseRouteID)
	reader := &directGovernedBindingReaderV1{binding: binding, driftSecond: true}
	backend := &directGovernedBackendV1{fakeBackend: &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}}
	adapter, err := direct.New(direct.Config{Identity: union.VersionedIdentity{ID: "direct-governed-drift", Version: "v1"}, Backend: backend, RouteID: selected.Selection.BaseRouteID, Model: selected.Selection.ModelID, Invocation: upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground}, GovernedInvocationBindings: reader})
	if err != nil {
		t.Fatal(err)
	}
	if report, err := adapter.Preflight(context.Background(), invocation); err != nil || !report.Accepted {
		t.Fatalf("Preflight = %#v, %v", report, err)
	}
	if _, err := adapter.Open(context.Background(), invocation); err == nil || backend.resolveCalls != 0 || backend.governedCalls != 0 || backend.invokeCalls != 0 {
		t.Fatalf("drift Open = %v resolve=%d governed=%d legacy=%d", err, backend.resolveCalls, backend.governedCalls, backend.invokeCalls)
	}
}

func TestDirectGovernedBindingUnavailableFailsBeforeResolve(t *testing.T) {
	invocation, selected := directInvocation(t, false, false)
	backend := &directGovernedBackendV1{fakeBackend: &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}}
	readerErr := errors.New("binding reader unavailable")
	adapter, err := direct.New(direct.Config{
		Identity: union.VersionedIdentity{ID: "direct-governed-reader-unavailable", Version: "v1"},
		Backend:  backend, RouteID: selected.Selection.BaseRouteID, Model: selected.Selection.ModelID,
		Invocation:                 upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground},
		GovernedInvocationBindings: &directGovernedBindingReaderV1{err: readerErr},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Preflight(context.Background(), invocation); !errors.Is(err, readerErr) {
		t.Fatalf("Preflight error = %v", err)
	}
	if backend.resolveCalls != 0 || backend.governedCalls != 0 || backend.invokeCalls != 0 || backend.streamCalls != 0 {
		t.Fatalf("reader failure crossed an external boundary: resolve/governed/invoke/stream=%d/%d/%d/%d",
			backend.resolveCalls, backend.governedCalls, backend.invokeCalls, backend.streamCalls)
	}
}

func TestDirectGovernedReaderRequiresGovernedBackendAndRejectsTypedNil(t *testing.T) {
	invocation, selected := directInvocation(t, false, false)
	binding := directGovernedBindingV1(t, invocation, selected.Selection.BaseRouteID)
	legacy := &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}
	if _, err := direct.New(direct.Config{Identity: union.VersionedIdentity{ID: "direct-governed-legacy", Version: "v1"}, Backend: legacy, RouteID: selected.Selection.BaseRouteID, Model: selected.Selection.ModelID, Invocation: upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground}, GovernedInvocationBindings: &directGovernedBindingReaderV1{binding: binding}}); !errors.Is(err, direct.ErrInvalidConfig) {
		t.Fatalf("legacy Backend accepted governed Reader: %v", err)
	}
	var typedNil *directGovernedBindingReaderV1
	backend := &directGovernedBackendV1{fakeBackend: legacy}
	if _, err := direct.New(direct.Config{Identity: union.VersionedIdentity{ID: "direct-governed-nil", Version: "v1"}, Backend: backend, RouteID: selected.Selection.BaseRouteID, Model: selected.Selection.ModelID, Invocation: upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground}, GovernedInvocationBindings: typedNil}); !errors.Is(err, direct.ErrInvalidConfig) {
		t.Fatalf("typed-nil governed Reader accepted: %v", err)
	}
}

func TestDirectGovernedBackendClosedErrorIsPreserved(t *testing.T) {
	invocation, selected := directInvocation(t, false, false)
	binding := directGovernedBindingV1(t, invocation, selected.Selection.BaseRouteID)
	closed := &modelinvoker.GovernedModelInvocationErrorV1{Kind: modelinvoker.GovernedModelInvocationErrorConflict, Operation: "actual_point", Message: "drift"}
	backend := &directGovernedBackendV1{fakeBackend: &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}, err: closed}
	adapter, err := direct.New(direct.Config{Identity: union.VersionedIdentity{ID: "direct-governed-errors", Version: "v1"}, Backend: backend, RouteID: selected.Selection.BaseRouteID, Model: selected.Selection.ModelID, Invocation: upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground}, GovernedInvocationBindings: &directGovernedBindingReaderV1{binding: binding}})
	if err != nil {
		t.Fatal(err)
	}
	if report, err := adapter.Preflight(context.Background(), invocation); err != nil || !report.Accepted {
		t.Fatalf("Preflight = %#v, %v", report, err)
	}
	if _, err := adapter.Open(context.Background(), invocation); !errors.Is(err, direct.ErrGovernedInvocationUnavailable) || modelinvoker.GovernedModelInvocationErrorKindOfV1(err) != modelinvoker.GovernedModelInvocationErrorConflict {
		t.Fatalf("closed governed error was downgraded: %v", err)
	}
}

func TestDirectGovernedBackendCannotReturnNonObservedSuccess(t *testing.T) {
	invocation, selected := directInvocation(t, false, false)
	binding := directGovernedBindingV1(t, invocation, selected.Selection.BaseRouteID)
	backend := &directGovernedBackendV1{fakeBackend: &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}, returnPrepared: true}
	adapter, err := direct.New(direct.Config{Identity: union.VersionedIdentity{ID: "direct-governed-non-observed", Version: "v1"}, Backend: backend, RouteID: selected.Selection.BaseRouteID, Model: selected.Selection.ModelID, Invocation: upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground}, GovernedInvocationBindings: &directGovernedBindingReaderV1{binding: binding}})
	if err != nil {
		t.Fatal(err)
	}
	if report, err := adapter.Preflight(context.Background(), invocation); err != nil || !report.Accepted {
		t.Fatalf("Preflight = %#v, %v", report, err)
	}
	if _, err := adapter.Open(context.Background(), invocation); !errors.Is(err, direct.ErrGovernedInvocationUnavailable) {
		t.Fatalf("non-observed success was accepted: %v", err)
	}
}

func TestDirectGovernedBackendSchemaMismatchFailsClosed(t *testing.T) {
	invocation, selected := directInvocation(t, false, false)
	binding := directGovernedBindingV1(t, invocation, selected.Selection.BaseRouteID)
	backend := &directGovernedBackendV1{fakeBackend: &fakeBackend{routeID: selected.Selection.BaseRouteID, model: selected.Selection.ModelID}, structuredOutput: []byte(`{"unexpected":true}`)}
	adapter, err := direct.New(direct.Config{Identity: union.VersionedIdentity{ID: "direct-governed-schema", Version: "v1"}, Backend: backend, RouteID: selected.Selection.BaseRouteID, Model: selected.Selection.ModelID, Invocation: upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground}, GovernedInvocationBindings: &directGovernedBindingReaderV1{binding: binding}})
	if err != nil {
		t.Fatal(err)
	}
	if report, err := adapter.Preflight(context.Background(), invocation); err != nil || !report.Accepted {
		t.Fatalf("Preflight = %#v, %v", report, err)
	}
	if _, err := adapter.Open(context.Background(), invocation); !errors.Is(err, direct.ErrGovernedInvocationUnavailable) || backend.governedCalls != 1 {
		t.Fatalf("schema mismatch = %v governed=%d", err, backend.governedCalls)
	}
}

func directGovernedBindingV1(t *testing.T, invocation execution.Invocation, routeID upstream.RouteID) modelinvoker.GovernedModelInvocationBindingV1 {
	t.Helper()
	requestDigest, err := invocation.Request.Digest()
	if err != nil {
		t.Fatal(err)
	}
	digest := func(label string) core.Digest { return core.DigestBytes([]byte(label)) }
	owner := core.OwnerRef{Domain: "registry", ID: "owner"}
	prepared, err := modelinvoker.SealPreparedModelInvocationFactV1(modelinvoker.PreparedModelInvocationFactV1{InvocationID: string(invocation.Request.ExecutionID), InvocationDigest: core.Digest(requestDigest), UnifiedRequestDigest: core.Digest(requestDigest), RequestToolsDigest: digest("tools"), PreparedPlanDigest: core.Digest(invocation.Plan.Digest), RouteDigest: digest("route"), ProfileDigest: digest("profile"), ActualToolSurfaceDigest: digest("surface"), ActualProviderInjectionDigest: digest("provider"), CapabilitySnapshotRef: modelinvoker.PreparedModelInvocationCapabilitySnapshotRefV1{ContractVersion: "1.0.0", ID: "capability", Revision: 1, Digest: digest("capability")}, RegistrySnapshotRef: runtimeports.RegistrySnapshotRefV1{Owner: owner, ContractVersion: "1.0.0", ID: "registry", Revision: 1, Digest: digest("registry")}, CreatedUnixNano: directTestNow.Add(-time.Minute).UnixNano(), NotAfterUnixNano: directTestNow.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	current, err := modelinvoker.SealPreparedModelInvocationCurrentV1(modelinvoker.PreparedModelInvocationCurrentProjectionV1{Prepared: prepared.Ref(), CapabilitySnapshotRef: prepared.CapabilitySnapshotRef, RegistrySnapshotRef: prepared.RegistrySnapshotRef, ActualToolSurfaceDigest: prepared.ActualToolSurfaceDigest, ActualProviderInjectionDigest: prepared.ActualProviderInjectionDigest, CheckedUnixNano: directTestNow.Add(-time.Second).UnixNano(), ExpiresUnixNano: directTestNow.Add(30 * time.Minute).UnixNano(), NotAfterUnixNano: prepared.NotAfterUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	binding := modelinvoker.GovernedModelInvocationBindingV1{ContractVersion: modelinvoker.GovernedModelInvocationBindingVersionV1, ExecutionID: string(invocation.Request.ExecutionID), UnifiedRequestDigest: core.Digest(requestDigest), PreparedPlanDigest: core.Digest(invocation.Plan.Digest), RouteID: routeID, PreparedRef: prepared.Ref(), CurrentRef: current.Ref(), DispatchSequence: 1, ProviderAttemptOrdinal: 1}
	request := modelinvoker.GovernedModelInvocationBindingRequestV1{ExecutionID: binding.ExecutionID, UnifiedRequestDigest: binding.UnifiedRequestDigest, PreparedPlanDigest: binding.PreparedPlanDigest, RouteID: routeID}
	if err := binding.ValidateAgainstV1(request); err != nil {
		t.Fatal(err)
	}
	return binding
}

type directGovernedBindingReaderV1 struct {
	mu          sync.Mutex
	binding     modelinvoker.GovernedModelInvocationBindingV1
	calls       int
	driftSecond bool
	err         error
}

func (r *directGovernedBindingReaderV1) InspectExactGovernedModelInvocationBindingV1(_ context.Context, request modelinvoker.GovernedModelInvocationBindingRequestV1) (modelinvoker.GovernedModelInvocationBindingV1, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.err != nil {
		return modelinvoker.GovernedModelInvocationBindingV1{}, r.err
	}
	result := r.binding
	if r.driftSecond && r.calls == 2 {
		result.DispatchSequence++
	}
	return result, nil
}

type directGovernedBackendV1 struct {
	*fakeBackend
	mu               sync.Mutex
	governedCalls    int
	command          modelinvoker.GovernedModelInvocationCommandV1
	err              error
	returnPrepared   bool
	structuredOutput []byte
}

func (b *directGovernedBackendV1) StartOrInspectGovernedModelInvocationV1(_ context.Context, command modelinvoker.GovernedModelInvocationCommandV1) (modelinvoker.GovernedModelInvocationResultV1, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.governedCalls++
	b.command = command
	if b.err != nil {
		return modelinvoker.GovernedModelInvocationResultV1{}, b.err
	}
	routeDigest, err := modelinvoker.DigestGovernedRouteCallV1(command.Call)
	if err != nil {
		return modelinvoker.GovernedModelInvocationResultV1{}, err
	}
	prepared, err := modelinvoker.NewPreparedGovernedModelInvocationForGatewayV1(command, routeDigest, directTestNow)
	if err != nil {
		return modelinvoker.GovernedModelInvocationResultV1{}, err
	}
	if b.returnPrepared {
		return modelinvoker.GovernedModelInvocationResultV1{Invocation: prepared}, nil
	}
	digest := func(label string) core.Digest { return core.DigestBytes([]byte(label)) }
	owner := core.OwnerRef{Domain: "host", ID: "owner"}
	ack, err := modelinvoker.SealPreparedModelInvocationCommitAckV1(modelinvoker.PreparedModelInvocationCommitAckV1{
		PreparedRef: command.PreparedRef, CurrentRef: command.CurrentRef,
		GateImplementationRef: modelinvoker.PreparedModelInvocationGateImplementationRefV1{Owner: owner, ContractVersion: "1.0.0", ID: "gate", Revision: 1, Digest: digest("gate")},
		SurfaceBindingRef:     modelinvoker.PreparedModelInvocationSurfaceBindingRefV1{Owner: owner, ContractVersion: "1.0.0", ID: "surface", Revision: 1, Digest: digest("surface")},
		CheckedUnixNano:       directTestNow.Add(-time.Second).UnixNano(), ExpiresUnixNano: directTestNow.Add(time.Minute).UnixNano(), NotAfterUnixNano: command.CurrentRef.NotAfterUnixNano,
	})
	if err != nil {
		return modelinvoker.GovernedModelInvocationResultV1{}, err
	}
	receipt, err := modelinvoker.SealPreparedModelInvocationDispatchReceiptV1(modelinvoker.PreparedModelInvocationDispatchValidationReceiptV1{
		PreparedRef: command.PreparedRef, CurrentRef: command.CurrentRef, AckRef: ack.Ref(), DispatchSequence: command.DispatchSequence,
		BoundaryKind: modelinvoker.GovernedModelProviderBoundaryKindV1, ProviderAttemptOrdinal: command.ProviderAttemptOrdinal, AttemptRequestDigest: routeDigest,
		ActualToolSurfaceDigest: digest("surface"), ActualProviderInjectionDigest: digest("provider"), CheckedUnixNano: directTestNow.UnixNano(),
	})
	if err != nil {
		return modelinvoker.GovernedModelInvocationResultV1{}, err
	}
	boundary := prepared.CloneV1()
	boundary.Revision = 2
	boundary.State = modelinvoker.GovernedModelInvocationProviderBoundaryCrossedV1
	boundary.UpdatedUnixNano = directTestNow.UnixNano()
	boundary.ExpiresUnixNano = directTestNow.Add(time.Minute).UnixNano()
	ackRef := ack.Ref()
	boundary.AckRef = &ackRef
	boundary.DispatchReceipt = &receipt
	boundary.Digest = ""
	boundary, err = modelinvoker.SealGovernedModelInvocationFactV1(boundary)
	if err != nil {
		return modelinvoker.GovernedModelInvocationResultV1{}, err
	}
	structuredOutput := b.structuredOutput
	if len(structuredOutput) == 0 {
		structuredOutput = []byte(`{}`)
	}
	observation, err := modelinvoker.SealGovernedModelInvocationObservationV1(modelinvoker.GovernedModelInvocationObservationV1{InvocationRef: boundary.RefV1(), RouteID: command.Call.RouteID, RouteSelectionDigest: digest("route-selection"), Provider: "openai", Protocol: modelinvoker.ProtocolResponses, ResponseID: "response-direct-governed", Model: command.Call.Request.Model, Status: modelinvoker.ResponseStatusCompleted, StopReason: modelinvoker.StopReasonEndTurn, StructuredOutput: structuredOutput, Usage: modelinvoker.Usage{TotalTokens: 1}, ObservedUnixNano: directTestNow.Add(time.Nanosecond).UnixNano(), ExpiresUnixNano: boundary.ExpiresUnixNano})
	if err != nil {
		return modelinvoker.GovernedModelInvocationResultV1{}, err
	}
	observed := boundary.CloneV1()
	observed.Revision = 3
	observed.State = modelinvoker.GovernedModelInvocationObservedV1
	observed.UpdatedUnixNano = observation.ObservedUnixNano
	observed.Observation = &observation
	observed.Digest = ""
	observed, err = modelinvoker.SealGovernedModelInvocationFactV1(observed)
	if err != nil {
		return modelinvoker.GovernedModelInvocationResultV1{}, err
	}
	return modelinvoker.GovernedModelInvocationResultV1{Invocation: observed, Observation: &observation}, nil
}
func (b *directGovernedBackendV1) InspectExactModelInvocationV1(context.Context, modelinvoker.GovernedModelInvocationRefV1) (modelinvoker.GovernedModelInvocationResultV1, error) {
	return modelinvoker.GovernedModelInvocationResultV1{}, errors.New("not used")
}

var _ direct.GovernedBackendV1 = (*directGovernedBackendV1)(nil)
