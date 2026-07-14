package runtimeadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var adapterTestNow = time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

func TestNewDescribeAndDescriptorCapabilityMatrix(t *testing.T) {
	t.Parallel()
	manifest := testkit.Manifest(adapterTestNow, runtimeports.ConformanceFullyControlled)
	if _, err := New(Config{Manifest: manifest}); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("nil loop returned wrong error: %v", err)
	}
	loop := adapterTestLoop(t, manifest, &fakes.ScriptedModel{})
	expired := manifest
	expired.EvidenceExpiresAt = adapterTestNow
	if _, err := New(Config{Manifest: expired, Loop: loop, Clock: func() time.Time { return adapterTestNow }}); err == nil {
		t.Fatal("expired manifest accepted")
	}
	mismatch := manifest
	mismatch.ID = "different"
	if _, err := New(Config{Manifest: mismatch, Loop: loop, Clock: func() time.Time { return adapterTestNow }}); !core.HasReason(err, core.ReasonComponentMismatch) {
		t.Fatalf("loop/adapter drift returned wrong error: %v", err)
	}

	current := adapterTestNow
	adapter, err := New(Config{Manifest: manifest, Loop: loop, Clock: func() time.Time { return current }})
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := adapter.Describe(context.Background())
	if err != nil || len(descriptor.Capabilities) != 8 {
		t.Fatalf("descriptor capabilities incomplete: %+v %v", descriptor, err)
	}
	current = manifest.EvidenceExpiresAt
	if _, err := adapter.Describe(context.Background()); !core.HasReason(err, core.ReasonCapabilityExpired) {
		t.Fatalf("expired descriptor evidence accepted: %v", err)
	}

	minimal := testkit.Manifest(adapterTestNow, runtimeports.ConformanceFullyControlled)
	minimal.Bootstrap.Controls = contract.ControlCapabilities{}
	minimalLoop := adapterTestLoop(t, minimal, &fakes.ScriptedModel{})
	minimalAdapter, err := New(Config{Manifest: minimal, Loop: minimalLoop, Clock: func() time.Time { return adapterTestNow }})
	if err != nil {
		t.Fatal(err)
	}
	minimalDescriptor, _ := minimalAdapter.Describe(context.Background())
	if len(minimalDescriptor.Capabilities) != 5 {
		t.Fatalf("disabled controls leaked into descriptor: %+v", minimalDescriptor.Capabilities)
	}
}

func TestDescribeV2RequiresExplicitGovernanceIdentityAndReturnsClone(t *testing.T) {
	t.Parallel()
	manifest := testkit.Manifest(adapterTestNow, runtimeports.ConformanceFullyControlled)
	loop := adapterTestLoop(t, manifest, &fakes.ScriptedModel{})
	legacy, err := New(Config{Manifest: manifest, Loop: loop, Clock: func() time.Time { return adapterTestNow }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.DescribeV2(context.Background()); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("legacy adapter was allowed into binding v2: %v", err)
	}

	binding := testkit.BindingManifest(manifest)
	adapter, err := New(Config{Manifest: manifest, AdapterBindingManifest: &binding, Loop: loop, Clock: func() time.Time { return adapterTestNow }})
	if err != nil {
		t.Fatal(err)
	}
	described, err := adapter.DescribeV2(context.Background())
	if err != nil || described.ComponentID != binding.ComponentID {
		t.Fatalf("explicit binding identity was not described: %+v %v", described, err)
	}
	binding.Owners[0].OwnerComponentID = "attacker/owner"
	describedAgain, err := adapter.DescribeV2(context.Background())
	if err != nil || describedAgain.Owners[0].OwnerComponentID == "attacker/owner" {
		t.Fatalf("caller mutation crossed the binding handoff: %+v %v", describedAgain, err)
	}
	described.Owners[0].OwnerComponentID = "attacker/returned"
	describedAgain, _ = adapter.DescribeV2(context.Background())
	if describedAgain.Owners[0].OwnerComponentID == "attacker/returned" {
		t.Fatal("returned manifest aliases adapter state")
	}
	scope := testkit.Scope(manifest.Bootstrap.ResolvedPlanDigest)
	if _, err := adapter.Open(context.Background(), adapterOpenRequest(scope, manifest, "legacy-open")); !core.HasReason(err, core.ReasonDispatchPermitInvalid) {
		t.Fatalf("v2 binding identity silently enabled the legacy dispatch path: %v", err)
	}
}

func TestNewRejectsBindingIdentityDrift(t *testing.T) {
	t.Parallel()
	manifest := testkit.Manifest(adapterTestNow, runtimeports.ConformanceFullyControlled)
	loop := adapterTestLoop(t, manifest, &fakes.ScriptedModel{})
	for name, mutate := range map[string]func(*runtimeports.ComponentManifestV2){
		"id":       func(m *runtimeports.ComponentManifestV2) { m.ComponentID = "other/harness" },
		"version":  func(m *runtimeports.ComponentManifestV2) { m.SemanticVersion = "0.2.0" },
		"artifact": func(m *runtimeports.ComponentManifestV2) { m.ArtifactDigest = testkit.Digest("other") },
		"conformance": func(m *runtimeports.ComponentManifestV2) {
			m.Conformance = runtimeports.ConformanceRestrictedControlled
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			binding := testkit.BindingManifest(manifest)
			mutate(&binding)
			if _, err := New(Config{Manifest: manifest, AdapterBindingManifest: &binding, Loop: loop, Clock: func() time.Time { return adapterTestNow }}); err == nil {
				t.Fatal("binding identity drift was accepted")
			}
		})
	}
}

func TestPreflightValidationResidualAndExpirySelection(t *testing.T) {
	t.Parallel()
	manifest := testkit.Manifest(adapterTestNow, runtimeports.ConformanceFullyControlled)
	manifest.Bootstrap.AllowedResiduals = []string{"native_session", "host_prompt"}
	manifest.Bootstrap.EvidenceExpiresAt = adapterTestNow.Add(30 * time.Minute)
	adapter := adapterTestAdapter(t, manifest, &fakes.ScriptedModel{})
	scope := testkit.Scope(manifest.Bootstrap.ResolvedPlanDigest)
	scope.SandboxLease = nil
	request := runtimeports.ExecutionPreflightRequest{ProposedScope: scope, RequirementDigest: testkit.Digest("requirement"), ProbeBudget: runtimeports.ProbeBudget{MaxRequests: 1, MaxDuration: time.Second}}
	report, err := adapter.Preflight(context.Background(), request)
	if err != nil || !report.Accepted || !report.PossibleResidual || report.ResidualRef != "native_session,host_prompt" || report.EvidenceExpiry != manifest.Bootstrap.EvidenceExpiresAt {
		t.Fatalf("valid residual preflight failed: %+v %v", report, err)
	}
	cases := []struct {
		name   string
		mutate func(*runtimeports.ExecutionPreflightRequest)
	}{
		{"scope", func(r *runtimeports.ExecutionPreflightRequest) { r.ProposedScope.AuthorityEpoch = 0 }},
		{"requirement", func(r *runtimeports.ExecutionPreflightRequest) { r.RequirementDigest = "bad" }},
		{"plan", func(r *runtimeports.ExecutionPreflightRequest) {
			r.ProposedScope.Lineage.PlanDigest = testkit.Digest("other")
		}},
		{"requests", func(r *runtimeports.ExecutionPreflightRequest) { r.ProbeBudget.MaxRequests = 0 }},
		{"duration", func(r *runtimeports.ExecutionPreflightRequest) { r.ProbeBudget.MaxDuration = 0 }},
		{"charge", func(r *runtimeports.ExecutionPreflightRequest) { r.ProbeBudget.PossibleCharge = true }},
		{"mutation", func(r *runtimeports.ExecutionPreflightRequest) { r.ProbeBudget.PossibleMutation = true }},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			candidate := request
			test.mutate(&candidate)
			if _, err := adapter.Preflight(context.Background(), candidate); err == nil {
				t.Fatal("invalid preflight accepted")
			}
		})
	}
}

func TestOpenIdempotencyConflictAndValidation(t *testing.T) {
	t.Parallel()
	manifest := testkit.Manifest(adapterTestNow, runtimeports.ConformanceFullyControlled)
	adapter := adapterTestAdapter(t, manifest, &fakes.ScriptedModel{})
	scope := testkit.Scope(manifest.Bootstrap.ResolvedPlanDigest)
	request := adapterOpenRequest(scope, manifest, "open")
	ref, err := adapter.Open(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	again, err := adapter.Open(context.Background(), request)
	if err != nil || again != ref {
		t.Fatalf("idempotent open changed endpoint: %+v %+v %v", ref, again, err)
	}
	cases := []struct {
		name   string
		mutate func(*runtimeports.ExecutionOpenRequest)
	}{
		{"scope", func(r *runtimeports.ExecutionOpenRequest) { r.Scope.AuthorityEpoch = 0 }},
		{"plan", func(r *runtimeports.ExecutionOpenRequest) { r.Scope.Lineage.PlanDigest = testkit.Digest("other") }},
		{"requirement", func(r *runtimeports.ExecutionOpenRequest) { r.RequirementDigest = "bad" }},
		{"fence", func(r *runtimeports.ExecutionOpenRequest) { r.Fence.ExpiresAt = adapterTestNow }},
	}
	for _, test := range cases {
		candidate := request
		test.mutate(&candidate)
		if _, err := adapter.Open(context.Background(), candidate); err == nil {
			t.Fatalf("invalid open %s accepted", test.name)
		}
	}
	adapter.mu.Lock()
	closed := adapter.endpoints[ref.EndpointID]
	closed.closed = true
	adapter.endpoints[ref.EndpointID] = closed
	adapter.mu.Unlock()
	if _, err := adapter.Open(context.Background(), request); !core.HasReason(err, core.ReasonAlreadyExists) {
		t.Fatalf("closed endpoint was reopened: %v", err)
	}
}

func TestInspectIdleReadyUnknownAndEndpointFencing(t *testing.T) {
	t.Parallel()
	manifest := testkit.Manifest(adapterTestNow, runtimeports.ConformanceFullyControlled)
	adapter := adapterTestAdapter(t, manifest, &fakes.ScriptedModel{})
	scope := testkit.Scope(manifest.Bootstrap.ResolvedPlanDigest)
	ref, err := adapter.Open(context.Background(), adapterOpenRequest(scope, manifest, "open"))
	if err != nil {
		t.Fatal(err)
	}
	for kind, want := range map[string]string{InspectReady: "ready:ready", InspectState: "state:idle", InspectEvents: "events:idle"} {
		observation, err := adapter.Inspect(context.Background(), runtimeports.ExecutionInspectRequest{Scope: scope, Endpoint: ref, InspectKind: kind})
		if err != nil || observation.ObservationKind != want {
			t.Fatalf("inspect %s failed: %+v %v", kind, observation, err)
		}
	}
	if _, err := adapter.Inspect(context.Background(), runtimeports.ExecutionInspectRequest{Scope: scope, Endpoint: ref, InspectKind: "unknown"}); err == nil {
		t.Fatal("unknown inspect accepted")
	}
	badRef := ref
	badRef.ComponentID = "other"
	if _, err := adapter.Inspect(context.Background(), runtimeports.ExecutionInspectRequest{Scope: scope, Endpoint: badRef, InspectKind: InspectReady}); err == nil {
		t.Fatal("foreign component endpoint accepted")
	}
	badRef = ref
	badRef.Digest = "bad"
	if _, err := adapter.Inspect(context.Background(), runtimeports.ExecutionInspectRequest{Scope: scope, Endpoint: badRef, InspectKind: InspectReady}); err == nil {
		t.Fatal("invalid endpoint digest accepted")
	}
	missing := ref
	missing.EndpointID = "missing"
	if _, err := adapter.Inspect(context.Background(), runtimeports.ExecutionInspectRequest{Scope: scope, Endpoint: missing, InspectKind: InspectReady}); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("missing endpoint returned wrong error: %v", err)
	}
	staleScope := scope
	staleScope.AuthorityEpoch++
	if _, err := adapter.Inspect(context.Background(), runtimeports.ExecutionInspectRequest{Scope: staleScope, Endpoint: ref, InspectKind: InspectReady}); !core.HasReason(err, core.ReasonFencedInstance) {
		t.Fatalf("stale scope returned wrong error: %v", err)
	}
}

func TestControlValidationCapabilityGatesAndInputPath(t *testing.T) {
	t.Parallel()
	inputResult := harnessports.ModelTurnResult{State: harnessports.TurnInputRequired, NativeSessionRef: "session", EvidenceDigest: testkit.Digest("input")}
	output := testkit.Payload("test.output/v1", "done")
	model := &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{inputResult, {State: harnessports.TurnCompleted, Output: &output, NativeSessionRef: "session", EvidenceDigest: testkit.Digest("done")}}}
	manifest := testkit.Manifest(adapterTestNow, runtimeports.ConformanceFullyControlled)
	adapter := adapterTestAdapter(t, manifest, model)
	scope := testkit.Scope(manifest.Bootstrap.ResolvedPlanDigest)
	ref, _ := adapter.Open(context.Background(), adapterOpenRequest(scope, manifest, "open"))
	intent1, fence1 := testkit.IntentFence(adapterTestNow, scope, manifest.Bootstrap.CapabilityGrantDigest, "turn-1")
	startPayload, _ := EncodeControlPayload(CommandStartRun, StartRunCommand{RunID: "run", Input: testkit.Payload("test/v1", "first"), Intent: intent1})
	if _, err := adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{Scope: scope, Endpoint: ref, CommandKind: CommandStartRun, Payload: startPayload}); !core.HasReason(err, core.ReasonEffectFenceStale) {
		t.Fatalf("start without fence returned wrong error: %v", err)
	}
	startObservation, err := adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{Scope: scope, Endpoint: ref, CommandKind: CommandStartRun, Payload: startPayload, Fence: &fence1})
	if err != nil || startObservation.ObservationKind != "control:start_run:waiting_input" {
		t.Fatalf("start control failed: %+v %v", startObservation, err)
	}
	intent2, fence2 := testkit.IntentFence(adapterTestNow, scope, manifest.Bootstrap.CapabilityGrantDigest, "turn-2")
	inputPayload, _ := EncodeControlPayload(CommandProvideInput, ProvideInputCommand{RunID: "run", Input: testkit.Payload("test/v1", "second"), Intent: intent2})
	if _, err := adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{Scope: scope, Endpoint: ref, CommandKind: CommandProvideInput, Payload: inputPayload}); !core.HasReason(err, core.ReasonEffectFenceStale) {
		t.Fatalf("input without fence returned wrong error: %v", err)
	}
	observation, err := adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{Scope: scope, Endpoint: ref, CommandKind: CommandProvideInput, Payload: inputPayload, Fence: &fence2})
	if err != nil || observation.ObservationKind != "control:provide_input:terminal" {
		t.Fatalf("input control failed: %+v %v", observation, err)
	}
	if _, err := adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{Scope: scope, Endpoint: ref, CommandKind: "unknown", Payload: inputPayload}); err == nil {
		t.Fatal("unknown control accepted")
	}
	tampered := inputPayload
	tampered.Payload = json.RawMessage(`{}`)
	if _, err := adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{Scope: scope, Endpoint: ref, CommandKind: CommandProvideInput, Payload: tampered, Fence: &fence2}); err == nil {
		t.Fatal("tampered control accepted")
	}

	disabled := manifest
	disabled.Bootstrap.Controls = contract.ControlCapabilities{}
	disabledAdapter := adapterTestAdapter(t, disabled, &fakes.ScriptedModel{})
	disabledRef, _ := disabledAdapter.Open(context.Background(), adapterOpenRequest(scope, disabled, "disabled-open"))
	cancelPayload, _ := EncodeControlPayload(CommandCancel, CancelCommand{RunID: "run"})
	if _, err := disabledAdapter.Control(context.Background(), runtimeports.ExecutionControlRequest{Scope: scope, Endpoint: disabledRef, CommandKind: CommandCancel, Payload: cancelPayload}); !core.HasCategory(err, core.ErrorCapabilityUnavailable) {
		t.Fatalf("disabled control returned wrong error: %v", err)
	}
}

func TestCloseValidationCancelsActiveRunAndFencesEndpoint(t *testing.T) {
	t.Parallel()
	blocking := &fakes.BlockingModel{Started: make(chan struct{})}
	manifest := testkit.Manifest(adapterTestNow, runtimeports.ConformanceFullyControlled)
	adapter := adapterTestAdapter(t, manifest, blocking)
	scope := testkit.Scope(manifest.Bootstrap.ResolvedPlanDigest)
	ref, _ := adapter.Open(context.Background(), adapterOpenRequest(scope, manifest, "open"))
	if _, err := adapter.Close(context.Background(), runtimeports.ExecutionCloseRequest{Scope: scope, Endpoint: ref}); err == nil {
		t.Fatal("blank close reason accepted")
	}
	badIntent, badFence := testkit.IntentFence(adapterTestNow, scope, manifest.Bootstrap.CapabilityGrantDigest, "bad-close")
	badFence.ExpiresAt = adapterTestNow
	if _, err := adapter.Close(context.Background(), runtimeports.ExecutionCloseRequest{Scope: scope, Endpoint: ref, Reason: "close", Intent: badIntent, Fence: badFence}); !core.HasReason(err, core.ReasonEffectFenceStale) {
		t.Fatalf("stale close fence accepted: %v", err)
	}
	turnIntent, turnFence := testkit.IntentFence(adapterTestNow, scope, manifest.Bootstrap.CapabilityGrantDigest, "turn")
	payload, _ := EncodeControlPayload(CommandStartRun, StartRunCommand{RunID: "run", Input: testkit.Payload("test/v1", "x"), Intent: turnIntent})
	controlDone := make(chan error, 1)
	go func() {
		_, err := adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{Scope: scope, Endpoint: ref, CommandKind: CommandStartRun, Payload: payload, Fence: &turnFence})
		controlDone <- err
	}()
	<-blocking.Started
	closeIntent, closeFence := testkit.IntentFence(adapterTestNow, scope, manifest.Bootstrap.CapabilityGrantDigest, "close")
	observation, err := adapter.Close(context.Background(), runtimeports.ExecutionCloseRequest{Scope: scope, Endpoint: ref, Reason: "test", Intent: closeIntent, Fence: closeFence})
	if err != nil || observation.ObservationKind != "closed" {
		t.Fatalf("close failed: %+v %v", observation, err)
	}
	if err := <-controlDone; err != nil {
		t.Fatalf("cancelled control failed: %v", err)
	}
	if _, err := adapter.Inspect(context.Background(), runtimeports.ExecutionInspectRequest{Scope: scope, Endpoint: ref, InspectKind: InspectReady}); !core.HasReason(err, core.ReasonFencedInstance) {
		t.Fatalf("closed endpoint remained inspectable: %v", err)
	}
	cleanup, err := adapter.Inspect(context.Background(), runtimeports.ExecutionInspectRequest{Scope: scope, Endpoint: ref, InspectKind: InspectCleanup})
	if err != nil || cleanup.ObservationKind != "cleanup:closed" {
		t.Fatalf("lost Close reply cannot be recovered by exact cleanup inspection: %+v %v", cleanup, err)
	}
}

func TestEndpointCannotControlOrPolluteAnotherScopeRun(t *testing.T) {
	t.Parallel()
	manifest := testkit.Manifest(adapterTestNow, runtimeports.ConformanceFullyControlled)
	model := &adapterMultiBlockingModel{Started: make(chan core.AgentRunID, 2)}
	adapter := adapterTestAdapter(t, manifest, model)
	scopeA := testkit.Scope(manifest.Bootstrap.ResolvedPlanDigest)
	scopeB := scopeA
	scopeB.Identity.TenantID = "tenant-b"
	scopeB.Identity.ID = "agent-b"
	scopeB.Lineage.ID = "lineage-b"
	scopeB.Instance.ID = "instance-b"
	scopeB.SandboxLease = &core.SandboxLeaseRef{ID: "sandbox-b", Epoch: 1}
	endpointA, err := adapter.Open(context.Background(), adapterOpenRequest(scopeA, manifest, "open-a"))
	if err != nil {
		t.Fatal(err)
	}
	endpointB, err := adapter.Open(context.Background(), adapterOpenRequest(scopeB, manifest, "open-b"))
	if err != nil {
		t.Fatal(err)
	}
	type startedResult struct{ err error }
	results := make(chan startedResult, 2)
	start := func(scope core.ExecutionScope, endpoint runtimeports.ExecutionEndpointRef, runID core.AgentRunID, id string) {
		intent, fence := testkit.IntentFence(adapterTestNow, scope, manifest.Bootstrap.CapabilityGrantDigest, id)
		payload, _ := EncodeControlPayload(CommandStartRun, StartRunCommand{RunID: runID, Input: testkit.Payload("test/v1", id), Intent: intent})
		_, controlErr := adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{Scope: scope, Endpoint: endpoint, CommandKind: CommandStartRun, Payload: payload, Fence: &fence})
		results <- startedResult{err: controlErr}
	}
	go start(scopeA, endpointA, "run-a", "start-a")
	go start(scopeB, endpointB, "run-b", "start-b")
	for range 2 {
		select {
		case <-model.Started:
		case result := <-results:
			t.Fatalf("control returned before both execution surfaces started: %v", result.err)
		case <-time.After(time.Second):
			t.Fatal("both scoped Runs did not start")
		}
	}
	crossIntent, crossFence := testkit.IntentFence(adapterTestNow, scopeA, manifest.Bootstrap.CapabilityGrantDigest, "cross-cancel")
	crossPayload, _ := EncodeControlPayload(CommandCancel, CancelCommand{RunID: "run-b", Intent: crossIntent})
	if _, err := adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{Scope: scopeA, Endpoint: endpointA, CommandKind: CommandCancel, Payload: crossPayload, Fence: &crossFence}); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("endpoint A controlled endpoint B's Run: %v", err)
	}
	stateA, err := adapter.Inspect(context.Background(), runtimeports.ExecutionInspectRequest{Scope: scopeA, Endpoint: endpointA, InspectKind: InspectState})
	if err != nil || stateA.ObservationKind != "state:running" {
		t.Fatalf("cross-scope command polluted endpoint A lastRun: %+v %v", stateA, err)
	}
	for index, item := range []struct {
		scope    core.ExecutionScope
		endpoint runtimeports.ExecutionEndpointRef
		runID    core.AgentRunID
	}{{scopeA, endpointA, "run-a"}, {scopeB, endpointB, "run-b"}} {
		intent, fence := testkit.IntentFence(adapterTestNow, item.scope, manifest.Bootstrap.CapabilityGrantDigest, fmt.Sprintf("proper-cancel-%d", index))
		payload, _ := EncodeControlPayload(CommandCancel, CancelCommand{RunID: item.runID, Intent: intent})
		if _, err := adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{Scope: item.scope, Endpoint: item.endpoint, CommandKind: CommandCancel, Payload: payload, Fence: &fence}); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if result := <-results; result.err != nil {
			t.Fatal(result.err)
		}
	}
}

func TestEndpointAndRuntimeSessionPartitionSameLocalIDsAcrossTenants(t *testing.T) {
	t.Parallel()
	manifest := testkit.Manifest(adapterTestNow, runtimeports.ConformanceFullyControlled)
	adapter := adapterTestAdapter(t, manifest, &fakes.ScriptedModel{})
	left := testkit.Scope(manifest.Bootstrap.ResolvedPlanDigest)
	right := left
	right.Identity.TenantID = "tenant-b"
	leftEndpoint, err := adapter.Open(context.Background(), adapterOpenRequest(left, manifest, "open-left"))
	if err != nil {
		t.Fatal(err)
	}
	rightEndpoint, err := adapter.Open(context.Background(), adapterOpenRequest(right, manifest, "open-right"))
	if err != nil {
		t.Fatal(err)
	}
	if leftEndpoint.EndpointID == rightEndpoint.EndpointID || leftEndpoint.Digest == rightEndpoint.Digest {
		t.Fatalf("endpoint identity collapsed tenant-local IDs: left=%+v right=%+v", leftEndpoint, rightEndpoint)
	}
	leftSession, err := runtimeports.DeriveRuntimeExecutionSessionRefV2(leftEndpoint.EndpointID, "shared-run")
	if err != nil {
		t.Fatal(err)
	}
	rightSession, err := runtimeports.DeriveRuntimeExecutionSessionRefV2(rightEndpoint.EndpointID, "shared-run")
	if err != nil {
		t.Fatal(err)
	}
	if leftSession == rightSession {
		t.Fatal("Runtime-stable session identity collapsed distinct tenant endpoints")
	}
}

func TestPayloadCodecAndPrivateHelpers(t *testing.T) {
	t.Parallel()
	if _, err := EncodeControlPayload("", map[string]string{"x": "y"}); err == nil {
		t.Fatal("blank schema accepted")
	}
	if _, err := EncodeControlPayload("x", make(chan struct{})); err == nil {
		t.Fatal("unserializable value accepted")
	}
	payload, err := EncodeControlPayload("schema", map[string]string{"x": "y"})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]string
	if err := DecodeControlPayload(payload, "schema", &decoded); err != nil || decoded["x"] != "y" {
		t.Fatalf("payload roundtrip failed: %+v %v", decoded, err)
	}
	if err := DecodeControlPayload(payload, "other", &decoded); err == nil {
		t.Fatal("schema mismatch accepted")
	}
	if err := DecodeControlPayload(payload, "schema", &struct{ N int }{}); err == nil {
		t.Fatal("dropped fields did not cause digest mismatch")
	}
	stringPayload, _ := EncodeControlPayload("schema", "text")
	if err := DecodeControlPayload(stringPayload, "schema", &struct{}{}); err == nil {
		t.Fatal("json type mismatch accepted")
	}
	tampered := payload
	tampered.Payload = json.RawMessage(`{"x":"z"}`)
	if err := DecodeControlPayload(tampered, "schema", &decoded); err == nil {
		t.Fatal("tampered payload accepted")
	}

	manifest := testkit.Manifest(adapterTestNow, runtimeports.ConformanceFullyControlled)
	left := testkit.Scope(manifest.Bootstrap.ResolvedPlanDigest)
	right := left
	if !sameScope(left, right) {
		t.Fatal("equal leased scopes differ")
	}
	right.SandboxLease = nil
	if sameScope(left, right) {
		t.Fatal("lease mismatch accepted")
	}
	left.SandboxLease = nil
	if !sameScope(left, right) {
		t.Fatal("equal lease-free scopes differ")
	}
	right.Instance.Epoch++
	if sameScope(left, right) {
		t.Fatal("instance epoch mismatch accepted")
	}
	earlier := adapterTestNow
	later := adapterTestNow.Add(time.Hour)
	if minTime(earlier, later) != earlier || minTime(later, earlier) != earlier {
		t.Fatal("minTime is not order independent")
	}
}

func FuzzDecodeControlPayloadNeverPanics(f *testing.F) {
	f.Add([]byte(`{"run_id":"run"}`), CommandStartRun)
	f.Add([]byte(`{}`), "unknown")
	f.Fuzz(func(t *testing.T, raw []byte, schema string) {
		if len(raw) == 0 || len(raw) > contract.MaxOpaquePayloadBytes || !json.Valid(raw) {
			t.Skip()
		}
		digest, err := core.DigestJSON(json.RawMessage(raw))
		if err != nil {
			t.Skip()
		}
		payload := runtimeports.OpaquePayload{Schema: schema, Digest: digest, Payload: append(json.RawMessage(nil), raw...)}
		var command StartRunCommand
		_ = DecodeControlPayload(payload, CommandStartRun, &command)
	})
}

func adapterTestAdapter(t *testing.T, manifest contract.Manifest, model harnessports.ModelTurnPort) *Adapter {
	t.Helper()
	loop := adapterTestLoop(t, manifest, model)
	adapter, err := New(Config{Manifest: manifest, Loop: loop, Clock: func() time.Time { return adapterTestNow }})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func adapterTestLoop(t *testing.T, manifest contract.Manifest, model harnessports.ModelTurnPort) *kernel.Loop {
	t.Helper()
	loop, err := kernel.New(kernel.Config{
		Manifest: manifest, Context: &fakes.StaticContext{Snapshot: testkit.Context(adapterTestNow)}, Model: model,
		Events: &fakes.MemoryEvents{}, Clock: func() time.Time { return adapterTestNow }, MaxEvents: 64, MaxTurns: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	return loop
}

func adapterOpenRequest(scope core.ExecutionScope, manifest contract.Manifest, id string) runtimeports.ExecutionOpenRequest {
	intent, fence := testkit.IntentFence(adapterTestNow, scope, manifest.Bootstrap.CapabilityGrantDigest, id)
	return runtimeports.ExecutionOpenRequest{Scope: scope, RequirementDigest: testkit.Digest("requirement"), Intent: intent, Fence: fence}
}

type adapterMultiBlockingModel struct {
	Started chan core.AgentRunID
}

func (m *adapterMultiBlockingModel) Invoke(ctx context.Context, request harnessports.ModelTurnRequest) (harnessports.ModelTurnResult, error) {
	m.Started <- request.Run.RunID
	<-ctx.Done()
	return harnessports.ModelTurnResult{}, ctx.Err()
}
