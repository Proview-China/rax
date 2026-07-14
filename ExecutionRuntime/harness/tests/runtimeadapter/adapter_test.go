package runtimeadapter_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	harnessfakes "github.com/Proview-China/rax/ExecutionRuntime/harness/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/kernel"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/runtimeadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimefakes "github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/foundation"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestHarnessBindingV2BlackBoxSupportsMultipleAndFutureCustomAdapters(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	registry, err := runtimeports.NewComponentRegistryV2(testkit.HarnessCatalog())
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"official", "custom-eighth"} {
		manifest := testkit.Manifest(now, runtimeports.ConformanceFullyControlled)
		manifest.ID = "vendor/harness-" + suffix
		manifest.ArtifactDigest = testkit.Digest("artifact-" + suffix)
		loop, err := kernel.New(kernel.Config{
			Manifest: manifest, Context: &harnessfakes.StaticContext{Snapshot: testkit.Context(now)},
			Model: &harnessfakes.ScriptedModel{}, Events: &harnessfakes.MemoryEvents{}, Clock: func() time.Time { return now }, MaxEvents: 16, MaxTurns: 4,
		})
		if err != nil {
			t.Fatal(err)
		}
		binding := testkit.BindingManifest(manifest)
		adapter, err := runtimeadapter.New(runtimeadapter.Config{Manifest: manifest, AdapterBindingManifest: &binding, Loop: loop, Clock: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		registered, err := registry.Register(context.Background(), adapter)
		if err != nil || registered.Manifest.ComponentID != binding.ComponentID {
			t.Fatalf("same-kind adapter %s was not independently registered: %+v %v", suffix, registered, err)
		}
		report, err := conformance.CheckBindingAdapterV2(context.Background(), conformance.BindingAdapterCaseV2{SubjectClass: conformance.SubjectExternalAdapter, Adapter: adapter, Catalog: testkit.HarnessCatalog(), Clock: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		if !report.CertificationCandidate || report.BindingEligible || report.ProductionClaimEligible || report.DispatchEligible {
			t.Fatalf("discovery self-authorized custom adapter %s: %+v", suffix, report)
		}
	}
}

func TestRuntimeFoundationActivatesHarnessAndCompletesRunThroughExecutionPort(t *testing.T) {
	t.Parallel()
	model := &harnessfakes.ScriptedModel{Results: []harnessports.ModelTurnResult{completedResult("native-1", "done")}}
	fixture := newFoundationFixture(t, model, "direct")
	instance, err := fixture.coordinator.Activate(context.Background(), fixture.activation)
	if err != nil {
		t.Fatal(err)
	}
	activated := instance.Snapshot()
	if activated.Kernel.State.Phase != core.PhaseReady || activated.Endpoint.ComponentID != fixture.manifest.ID {
		t.Fatalf("runtime did not bind the harness endpoint: %+v", activated)
	}
	if _, err := fixture.coordinator.StartRun(context.Background(), instance, "run-direct", "runtime-session-direct"); err != nil {
		t.Fatal(err)
	}

	scope := instance.Snapshot().Kernel.Scope
	intent, fence := fixture.persistTurnIntent(t, scope, "direct-turn")
	payload, err := runtimeadapter.EncodeControlPayload(runtimeadapter.CommandStartRun, runtimeadapter.StartRunCommand{
		RunID: "run-direct", Input: testkit.Payload("test.input/v1", "hello"), Intent: intent,
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := fixture.adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{
		Scope: scope, Endpoint: activated.Endpoint, CommandKind: runtimeadapter.CommandStartRun, Payload: payload, Fence: &fence,
	})
	if err != nil {
		t.Fatal(err)
	}
	if observation.ObservationKind != "control:start_run:terminal" {
		t.Fatalf("unexpected control observation: %+v", observation)
	}
	stateObservation, err := fixture.adapter.Inspect(context.Background(), runtimeports.ExecutionInspectRequest{
		Scope: scope, Endpoint: activated.Endpoint, InspectKind: runtimeadapter.InspectState,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stateObservation.ObservationKind != "state:terminal" {
		t.Fatalf("terminal run was not inspectable after completion: %+v", stateObservation)
	}

	report, err := fixture.coordinator.Stop(context.Background(), instance, foundation.StopRequest{
		Outcome: core.OutcomeCompleted, Reason: "integration complete",
		CloseIntent: fixture.baseIntent("close"), ReleaseIntent: fixture.baseIntent("release"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.State.Phase != core.PhaseTerminal || report.State.Cleanup != core.CleanupComplete {
		t.Fatalf("runtime did not close the full lifecycle: %+v", report)
	}
}

func TestRuntimePortStopsAtActionBoundaryUntilExternalResultArrives(t *testing.T) {
	t.Parallel()
	model := &harnessfakes.ScriptedModel{Results: []harnessports.ModelTurnResult{
		{
			State: harnessports.TurnActionRequired, NativeSessionRef: "native-action", EvidenceDigest: testkit.Digest("action-evidence"),
			Action: &contract.ActionRequest{Ref: "action-1", Capability: "tool.search", Payload: testkit.Payload("test.action/v1", map[string]string{"query": "praxis"}), ReviewRequired: true},
		},
		completedResult("native-action", "approved"),
	}}
	fixture := newFoundationFixture(t, model, "action")
	instance, err := fixture.coordinator.Activate(context.Background(), fixture.activation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.coordinator.StartRun(context.Background(), instance, "run-action", "runtime-session-action"); err != nil {
		t.Fatal(err)
	}
	snapshot := instance.Snapshot()
	scope := snapshot.Kernel.Scope
	intent1, fence1 := fixture.persistTurnIntent(t, scope, "action-turn-1")
	startPayload, _ := runtimeadapter.EncodeControlPayload(runtimeadapter.CommandStartRun, runtimeadapter.StartRunCommand{
		RunID: "run-action", Input: testkit.Payload("test.input/v1", "search"), Intent: intent1,
	})
	first, err := fixture.adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{
		Scope: scope, Endpoint: snapshot.Endpoint, CommandKind: runtimeadapter.CommandStartRun, Payload: startPayload, Fence: &fence1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ObservationKind != "control:start_run:waiting_action" || model.CallCount() != 1 {
		t.Fatalf("pending action escaped the external boundary: observation=%+v calls=%d", first, model.CallCount())
	}

	intent2, fence2 := fixture.persistTurnIntent(t, scope, "action-turn-2")
	resultPayload, _ := runtimeadapter.EncodeControlPayload(runtimeadapter.CommandProvideActionResult, runtimeadapter.ProvideActionResultCommand{
		RunID: "run-action", Result: contract.ActionResult{Ref: "action-1", Payload: testkit.Payload("test.action-result/v1", map[string]string{"review": "approved"})}, Intent: intent2,
	})
	second, err := fixture.adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{
		Scope: scope, Endpoint: snapshot.Endpoint, CommandKind: runtimeadapter.CommandProvideActionResult, Payload: resultPayload, Fence: &fence2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ObservationKind != "control:provide_action_result:terminal" || model.CallCount() != 2 {
		t.Fatalf("external result did not resume exactly once: observation=%+v calls=%d", second, model.CallCount())
	}
	eventsObservation, err := fixture.adapter.Inspect(context.Background(), runtimeports.ExecutionInspectRequest{
		Scope: scope, Endpoint: snapshot.Endpoint, InspectKind: runtimeadapter.InspectEvents,
	})
	if err != nil {
		t.Fatal(err)
	}
	var events []contract.Event
	if err := json.Unmarshal(eventsObservation.Payload.Payload, &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 9 || events[3].Kind != contract.EventActionRequested || events[4].Kind != contract.EventActionResultReceived {
		t.Fatalf("action boundary event chain is incomplete: %+v", events)
	}
}

func TestRestrictedHarnessCompletesInputContinuationThroughRuntimeBoundary(t *testing.T) {
	t.Parallel()
	output := testkit.Payload("test.output/v1", "continued")
	model := &harnessfakes.ScriptedModel{Results: []harnessports.ModelTurnResult{
		{State: harnessports.TurnInputRequired, NativeSessionRef: "native-input", EvidenceDigest: testkit.Digest("input-required")},
		{State: harnessports.TurnCompleted, Output: &output, NativeSessionRef: "native-input", EvidenceDigest: testkit.Digest("input-completed")},
	}}
	fixture := newFoundationFixtureWithConformance(t, model, "input", runtimeports.ConformanceRestrictedControlled)
	instance, err := fixture.coordinator.Activate(context.Background(), fixture.activation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.coordinator.StartRun(context.Background(), instance, "run-input", "runtime-session-input"); err != nil {
		t.Fatal(err)
	}
	snapshot := instance.Snapshot()
	scope := snapshot.Kernel.Scope
	intent1, fence1 := fixture.persistTurnIntent(t, scope, "input-turn-1")
	startPayload, _ := runtimeadapter.EncodeControlPayload(runtimeadapter.CommandStartRun, runtimeadapter.StartRunCommand{
		RunID: "run-input", Input: testkit.Payload("test.input/v1", "first"), Intent: intent1,
	})
	first, err := fixture.adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{
		Scope: scope, Endpoint: snapshot.Endpoint, CommandKind: runtimeadapter.CommandStartRun, Payload: startPayload, Fence: &fence1,
	})
	if err != nil || first.ObservationKind != "control:start_run:waiting_input" {
		t.Fatalf("restricted harness did not wait for input: %+v %v", first, err)
	}
	intent2, fence2 := fixture.persistTurnIntent(t, scope, "input-turn-2")
	inputPayload, _ := runtimeadapter.EncodeControlPayload(runtimeadapter.CommandProvideInput, runtimeadapter.ProvideInputCommand{
		RunID: "run-input", Input: testkit.Payload("test.input/v1", "second"), Intent: intent2,
	})
	second, err := fixture.adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{
		Scope: scope, Endpoint: snapshot.Endpoint, CommandKind: runtimeadapter.CommandProvideInput, Payload: inputPayload, Fence: &fence2,
	})
	if err != nil || second.ObservationKind != "control:provide_input:terminal" || model.CallCount() != 2 {
		t.Fatalf("restricted input continuation failed: %+v err=%v calls=%d", second, err, model.CallCount())
	}
}

func TestRuntimeControlCancelInterruptsBlockingHarnessBlackBox(t *testing.T) {
	t.Parallel()
	model := &harnessfakes.BlockingModel{Started: make(chan struct{})}
	fixture := newFoundationFixture(t, model, "cancel")
	instance, err := fixture.coordinator.Activate(context.Background(), fixture.activation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.coordinator.StartRun(context.Background(), instance, "run-cancel", "runtime-session-cancel"); err != nil {
		t.Fatal(err)
	}
	snapshot := instance.Snapshot()
	scope := snapshot.Kernel.Scope
	intent, fence := fixture.persistTurnIntent(t, scope, "cancel-turn")
	startPayload, _ := runtimeadapter.EncodeControlPayload(runtimeadapter.CommandStartRun, runtimeadapter.StartRunCommand{
		RunID: "run-cancel", Input: testkit.Payload("test.input/v1", "block"), Intent: intent,
	})
	type controlResult struct {
		observation runtimeports.ExecutionObservation
		err         error
	}
	started := make(chan controlResult, 1)
	go func() {
		observation, err := fixture.adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{
			Scope: scope, Endpoint: snapshot.Endpoint, CommandKind: runtimeadapter.CommandStartRun, Payload: startPayload, Fence: &fence,
		})
		started <- controlResult{observation: observation, err: err}
	}()
	select {
	case <-model.Started:
	case <-time.After(time.Second):
		t.Fatal("blocking model did not start")
	}
	cancelIntent, cancelFence := fixture.persistTurnIntent(t, scope, "cancel-control")
	cancelPayload, _ := runtimeadapter.EncodeControlPayload(runtimeadapter.CommandCancel, runtimeadapter.CancelCommand{RunID: "run-cancel", Intent: cancelIntent})
	cancelled, err := fixture.adapter.Control(context.Background(), runtimeports.ExecutionControlRequest{
		Scope: scope, Endpoint: snapshot.Endpoint, CommandKind: runtimeadapter.CommandCancel, Payload: cancelPayload, Fence: &cancelFence,
	})
	if err != nil || cancelled.ObservationKind != "control:cancel:cancelling" {
		t.Fatalf("cancel command failed: %+v %v", cancelled, err)
	}
	select {
	case result := <-started:
		if result.err != nil || result.observation.ObservationKind != "control:start_run:terminal" {
			t.Fatalf("late blocking result escaped cancellation: %+v %v", result.observation, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not interrupt blocking model")
	}
	state, err := fixture.adapter.Inspect(context.Background(), runtimeports.ExecutionInspectRequest{Scope: scope, Endpoint: snapshot.Endpoint, InspectKind: runtimeadapter.InspectState})
	if err != nil || state.ObservationKind != "state:terminal" {
		t.Fatalf("cancelled terminal state is not inspectable: %+v %v", state, err)
	}
}

func TestAdapterRejectsScopeFromDifferentResolvedPlan(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	manifest := testkit.Manifest(now, runtimeports.ConformanceFullyControlled)
	loop, err := kernel.New(kernel.Config{
		Manifest: manifest, Context: &harnessfakes.StaticContext{Snapshot: testkit.Context(now)},
		Model:  &harnessfakes.ScriptedModel{Results: []harnessports.ModelTurnResult{completedResult("native", "done")}},
		Events: &harnessfakes.MemoryEvents{}, Clock: func() time.Time { return now }, MaxEvents: 16, MaxTurns: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := runtimeadapter.New(runtimeadapter.Config{Manifest: manifest, Loop: loop, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := adapter.Describe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range descriptor.Capabilities {
		if capability.Name == "checkpoint" {
			t.Fatal("minimal harness must not advertise unsupported checkpoint capability")
		}
	}
	scope := testkit.Scope(testkit.Digest("another-plan"))
	scope.SandboxLease = nil
	_, err = adapter.Preflight(context.Background(), runtimeports.ExecutionPreflightRequest{
		ProposedScope: scope, RequirementDigest: testkit.Digest("requirement"), ProbeBudget: runtimeports.ProbeBudget{MaxRequests: 1, MaxDuration: time.Second},
	})
	if !core.HasReason(err, core.ReasonComponentMismatch) {
		t.Fatalf("plan drift must fail closed: %v", err)
	}
}

type foundationFixture struct {
	now         time.Time
	manifest    contract.Manifest
	adapter     *runtimeadapter.Adapter
	coordinator *foundation.Coordinator
	activation  foundation.ActivationRequest
	evidence    *runtimefakes.FakeEvidence
}

func newFoundationFixture(t *testing.T, model harnessports.ModelTurnPort, suffix string) foundationFixture {
	t.Helper()
	return newFoundationFixtureWithConformance(t, model, suffix, runtimeports.ConformanceFullyControlled)
}

func newFoundationFixtureWithConformance(t *testing.T, model harnessports.ModelTurnPort, suffix string, conformance runtimeports.ConformanceLevel) foundationFixture {
	t.Helper()
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	manifest := testkit.Manifest(now, conformance)
	manifest.ID += "-" + suffix
	manifest.ArtifactDigest = testkit.Digest("harness-artifact-" + suffix)
	loop, err := kernel.New(kernel.Config{
		Manifest: manifest, Context: &harnessfakes.StaticContext{Snapshot: testkit.Context(now)}, Model: model,
		Events: &harnessfakes.MemoryEvents{}, Clock: clock, MaxEvents: 64, MaxTurns: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := runtimeadapter.New(runtimeadapter.Config{Manifest: manifest, Loop: loop, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := runtimefakes.NewFakeEnvironment("sandbox-"+suffix, clock)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := runtimefakes.NewFakeEvidence("evidence-"+suffix, clock)
	if err != nil {
		t.Fatal(err)
	}
	registry := runtimeports.NewComponentRegistry()
	adapters := []runtimeports.Describer{adapter, environment, evidence}
	plan := runtimeports.ResolvedAgentPlan{
		ID: "plan-" + suffix, Digest: manifest.Bootstrap.ResolvedPlanDigest,
		ProfileDigest: manifest.Bootstrap.ProfileDigest, ContextDigest: manifest.Bootstrap.ContextPlanDigest,
		AuthorityDigest: manifest.Bootstrap.CapabilityGrantDigest,
	}
	for _, component := range adapters {
		if err := registry.Register(context.Background(), component); err != nil {
			t.Fatal(err)
		}
		descriptor, err := component.Describe(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		plan.Requirements = append(plan.Requirements, runtimeports.ComponentRequirement{
			ID: descriptor.ID, Kind: descriptor.Kind, Version: descriptor.Version, ArtifactDigest: descriptor.ArtifactDigest, Required: true,
		})
	}
	scope := testkit.Scope(plan.Digest)
	scope.Identity.ID = core.AgentIdentityID("agent-" + suffix)
	scope.Lineage.ID = core.InstanceLineageID("lineage-" + suffix)
	scope.Instance.ID = core.AgentInstanceID("instance-" + suffix)
	scope.SandboxLease = nil
	store := runtimefakes.NewFactStore(clock)
	coordinator := &foundation.Coordinator{
		Registry: registry, Execution: adapter, Environment: environment, Evidence: evidence,
		IdentityLeases: store, ActivationFacts: store, Clock: clock,
	}
	fixture := foundationFixture{now: now, manifest: manifest, adapter: adapter, coordinator: coordinator, evidence: evidence}
	fixture.activation = foundation.ActivationRequest{
		Plan: plan, ProposedScope: scope, ActivationAttemptID: "activation-" + suffix,
		RequirementDigest: testkit.Digest("sandbox-requirement-" + suffix), CapabilityGrantDigest: manifest.Bootstrap.CapabilityGrantDigest,
		ProbeBudget: runtimeports.ProbeBudget{MaxRequests: 1, MaxDuration: time.Second}, IdentityLeaseExpiresAt: now.Add(time.Hour), FenceTTL: time.Minute,
		AllocateIntent: fixture.baseIntent("allocate"), ActivateIntent: fixture.baseIntent("activate"), OpenIntent: fixture.baseIntent("open"),
	}
	return fixture
}

func (f foundationFixture) baseIntent(id string) core.EffectIntent {
	return core.EffectIntent{
		ID: core.EffectIntentID(fmt.Sprintf("intent-%s-%s", f.manifest.ID, id)), Revision: 1, Kind: core.EffectKindResourceLifecycle,
		RiskClass: "integration", CanonicalPayloadDigest: testkit.Digest(f.manifest.ID + "-" + id), Target: id,
		ConflictEffectDomain: "harness/integration", Ownership: core.EffectOwnership{
			IntentOwner: core.OwnerRef{Domain: "runtime", ID: "foundation"}, SettlementOwner: core.OwnerRef{Domain: "runtime", ID: "foundation"},
		},
		AuthorizationRef: "authorization-1", IdempotencyClass: core.IdempotencyQueryable,
	}
}

func (f foundationFixture) persistTurnIntent(t *testing.T, scope core.ExecutionScope, id string) (core.EffectIntent, core.ExecutionFence) {
	t.Helper()
	intent, fence := testkit.IntentFence(f.now, scope, f.manifest.Bootstrap.CapabilityGrantDigest, id)
	ref, err := f.evidence.AppendIntent(context.Background(), runtimeports.EvidenceIntentRecord{
		Scope: scope, Kind: "harness_model_turn", PayloadDigest: intent.CanonicalPayloadDigest, CausationID: string(intent.ID),
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := f.evidence.Read(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	intent.PersistedAt = record.RecordedAt
	return intent, fence
}

func completedResult(session, output string) harnessports.ModelTurnResult {
	payload := testkit.Payload("test.output/v1", output)
	return harnessports.ModelTurnResult{State: harnessports.TurnCompleted, Output: &payload, NativeSessionRef: session, EvidenceDigest: testkit.Digest("evidence-" + output)}
}
