package kernel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	harnessports "github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var kernelTestNow = time.Date(2026, 7, 14, 11, 0, 0, 0, time.UTC)

func TestNewRejectsIncompleteOrInvalidConfiguration(t *testing.T) {
	t.Parallel()
	manifest := testkit.Manifest(kernelTestNow, runtimeports.ConformanceFullyControlled)
	contextPort := &fakes.StaticContext{Snapshot: testkit.Context(kernelTestNow)}
	model := &fakes.ScriptedModel{}
	events := &fakes.MemoryEvents{}
	cases := []Config{
		{Manifest: manifest, Model: model, Events: events, MaxEvents: 1, MaxTurns: 1},
		{Manifest: manifest, Context: contextPort, Events: events, MaxEvents: 1, MaxTurns: 1},
		{Manifest: manifest, Context: contextPort, Model: model, MaxEvents: 1, MaxTurns: 1},
		{Manifest: manifest, Context: contextPort, Model: model, Events: events, MaxTurns: 1},
		{Manifest: manifest, Context: contextPort, Model: model, Events: events, MaxEvents: 1},
	}
	for index, config := range cases {
		if _, err := New(config); err == nil {
			t.Fatalf("invalid config %d accepted", index)
		}
	}
	expired := manifest
	expired.EvidenceExpiresAt = kernelTestNow
	if _, err := New(Config{Manifest: expired, Context: contextPort, Model: model, Events: events, Clock: func() time.Time { return kernelTestNow }, MaxEvents: 1, MaxTurns: 1}); err == nil {
		t.Fatal("expired manifest accepted")
	}
}

func TestStartFailClosedValidationAndContextFailures(t *testing.T) {
	t.Parallel()
	model := &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{internalCompleted("done")}}
	loop := newInternalLoop(t, model, &fakes.MemoryEvents{}, 16, 4, nil)
	scope := internalScope(loop)
	intent, fence := testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "start")
	request := StartRequest{Run: contract.RunRef{Scope: scope, RunID: "run"}, Input: testkit.Payload("test.input/v1", "input"), Intent: intent, Fence: fence}

	tampered := request
	tampered.Input.Payload = []byte(`"changed"`)
	if _, err := loop.Start(context.Background(), tampered); err == nil || model.CallCount() != 0 {
		t.Fatalf("tampered input reached model: err=%v calls=%d", err, model.CallCount())
	}
	stale := request
	stale.Fence.ExpiresAt = kernelTestNow
	if _, err := loop.Start(context.Background(), stale); !core.HasReason(err, core.ReasonEffectFenceStale) {
		t.Fatalf("stale fence accepted: %v", err)
	}

	contextErr := errors.New("context unavailable")
	failing := newInternalLoop(t, model, &fakes.MemoryEvents{}, 16, 4, &fakes.StaticContext{Err: contextErr})
	if _, err := failing.Start(context.Background(), request); !errors.Is(err, contextErr) {
		t.Fatalf("context failure not surfaced: %v", err)
	}
	invalidContext := testkit.Context(kernelTestNow)
	invalidContext.Ref = ""
	failing = newInternalLoop(t, model, &fakes.MemoryEvents{}, 16, 4, &fakes.StaticContext{Snapshot: invalidContext})
	if _, err := failing.Start(context.Background(), request); err == nil {
		t.Fatal("invalid context snapshot accepted")
	}
}

func TestProvideInputContinuationAndInvalidState(t *testing.T) {
	t.Parallel()
	model := &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{
		{State: harnessports.TurnInputRequired, NativeSessionRef: "session", EvidenceDigest: testkit.Digest("input-required")},
		internalCompleted("done"),
	}}
	loop := newInternalLoop(t, model, &fakes.MemoryEvents{}, 32, 4, nil)
	scope := internalScope(loop)
	intent1, fence1 := testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "turn-1")
	snapshot, err := loop.Start(context.Background(), StartRequest{
		Run: contract.RunRef{Scope: scope, RunID: "run-input"}, Input: testkit.Payload("test.input/v1", "first"), Intent: intent1, Fence: fence1,
	})
	if err != nil || snapshot.State.Phase != contract.RunWaitingInput {
		t.Fatalf("run did not wait for input: snapshot=%+v err=%v", snapshot, err)
	}
	intent2, fence2 := testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "turn-2")
	input := testkit.Payload("test.input/v1", "second")
	run := contract.RunRef{Scope: scope, RunID: "run-input"}
	snapshot, err = loop.ProvideInput(context.Background(), ProvideInputRequest{Run: run, Input: input, Intent: intent2, Fence: fence2})
	if err != nil || snapshot.State.CompletionClaim != contract.ClaimCompleted || model.CallCount() != 2 {
		t.Fatalf("input continuation failed: snapshot=%+v err=%v calls=%d", snapshot, err, model.CallCount())
	}
	if model.Calls[1].Input.Digest != input.Digest {
		t.Fatal("second model turn did not receive provided input")
	}
	if _, err := loop.ProvideInput(context.Background(), ProvideInputRequest{Run: run, Input: input, Intent: intent2, Fence: fence2}); !core.HasReason(err, core.ReasonInvalidState) {
		t.Fatalf("terminal run accepted input: %v", err)
	}
}

func TestProvideActionResultRequiresExactPendingActionAndCurrentFence(t *testing.T) {
	t.Parallel()
	action := contract.ActionRequest{
		Ref:        "action-1",
		Capability: "example/tool",
		Payload:    testkit.Payload("test.action/v1", "request"),
	}
	model := &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{
		{State: harnessports.TurnActionRequired, NativeSessionRef: "provider-1", EvidenceDigest: testkit.Digest("action-required"), Action: &action},
		internalCompleted("done"),
	}}
	loop := newInternalLoop(t, model, &fakes.MemoryEvents{}, 32, 4, nil)
	scope := internalScope(loop)
	run := contract.RunRef{Scope: scope, RunID: "run-action"}
	startIntent, startFence := testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "action-start")
	started, err := loop.Start(context.Background(), StartRequest{
		Run: run, Input: testkit.Payload("test.input/v1", "start"), Intent: startIntent, Fence: startFence,
	})
	if err != nil || started.State.Phase != contract.RunWaitingAction || started.State.PendingAction == nil {
		t.Fatalf("run did not stop at the action gateway: snapshot=%+v err=%v", started, err)
	}
	if active := loop.ActiveRun(scope); active != run.RunID {
		t.Fatalf("active run projection drifted: got %q want %q", active, run.RunID)
	}

	result := contract.ActionResult{Ref: action.Ref, Payload: testkit.Payload("test.action-result/v1", "result")}
	continueIntent, continueFence := testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "action-continue")
	wrong := result
	wrong.Ref = "another-action"
	if _, err := loop.ProvideActionResult(context.Background(), ProvideActionResultRequest{Run: run, Result: wrong, Intent: continueIntent, Fence: continueFence}); !core.HasReason(err, core.ReasonInvalidState) {
		t.Fatalf("mismatched action result was accepted: %v", err)
	}
	staleFence := continueFence
	staleFence.ExpiresAt = kernelTestNow
	if _, err := loop.ProvideActionResult(context.Background(), ProvideActionResultRequest{Run: run, Result: result, Intent: continueIntent, Fence: staleFence}); !core.HasReason(err, core.ReasonEffectFenceStale) {
		t.Fatalf("stale action continuation fence was accepted: %v", err)
	}

	finished, err := loop.ProvideActionResult(context.Background(), ProvideActionResultRequest{Run: run, Result: result, Intent: continueIntent, Fence: continueFence})
	if err != nil || finished.State.CompletionClaim != contract.ClaimCompleted || model.CallCount() != 2 {
		t.Fatalf("exact action continuation failed: snapshot=%+v err=%v calls=%d", finished, err, model.CallCount())
	}
	if model.Calls[1].ActionResult == nil || model.Calls[1].ActionResult.Ref != action.Ref || model.Calls[1].ActionResult.Payload.Digest != result.Payload.Digest {
		t.Fatalf("model did not receive the exact action result: %+v", model.Calls[1].ActionResult)
	}
	if active := loop.ActiveRun(scope); active != "" {
		t.Fatalf("terminal run remained active: %q", active)
	}
	if _, err := loop.ProvideActionResult(context.Background(), ProvideActionResultRequest{Run: run, Result: result, Intent: continueIntent, Fence: continueFence}); !core.HasReason(err, core.ReasonInvalidState) {
		t.Fatalf("terminal run accepted a duplicate action result: %v", err)
	}
}

func TestSameScopeDuplicateAndRunIDReuseAreRejected(t *testing.T) {
	t.Parallel()
	blocking := &fakes.BlockingModel{Started: make(chan struct{})}
	loop := newInternalLoop(t, blocking, &fakes.MemoryEvents{}, 32, 4, nil)
	scope := internalScope(loop)
	intent, fence := testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "blocked")
	done := make(chan error, 1)
	go func() {
		_, err := loop.Start(context.Background(), StartRequest{Run: contract.RunRef{Scope: scope, RunID: "run-1"}, Input: testkit.Payload("test/v1", "x"), Intent: intent, Fence: fence})
		done <- err
	}()
	<-blocking.Started
	intent2, fence2 := testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "second")
	_, err := loop.Start(context.Background(), StartRequest{Run: contract.RunRef{Scope: scope, RunID: "run-2"}, Input: testkit.Payload("test/v1", "x"), Intent: intent2, Fence: fence2})
	if !core.HasReason(err, core.ReasonInvalidState) {
		t.Fatalf("same scope accepted second active run: %v", err)
	}
	if _, err := loop.Cancel(context.Background(), CancelRequest{Run: contract.RunRef{Scope: scope, RunID: "run-1"}, Intent: intent, Fence: fence}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_, err = loop.Start(context.Background(), StartRequest{Run: contract.RunRef{Scope: scope, RunID: "run-1"}, Input: testkit.Payload("test/v1", "x"), Intent: intent2, Fence: fence2})
	if !core.HasReason(err, core.ReasonInvalidReference) {
		t.Fatalf("completed run id was reused: %v", err)
	}
}

func TestModelUnknownInvalidResultAndTurnLimitAreFailClosed(t *testing.T) {
	t.Parallel()
	scopeManifest := testkit.Manifest(kernelTestNow, runtimeports.ConformanceFullyControlled)
	scope := testkit.Scope(scopeManifest.Bootstrap.ResolvedPlanDigest)
	modelErr := errors.New("model unavailable")
	model := &fakes.ScriptedModel{Err: modelErr}
	loop := newInternalLoop(t, model, &fakes.MemoryEvents{}, 16, 4, nil)
	intent, fence := testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "failure")
	snapshot, err := loop.Start(context.Background(), StartRequest{Run: contract.RunRef{Scope: scope, RunID: "run-failure"}, Input: testkit.Payload("test/v1", "x"), Intent: intent, Fence: fence})
	if !errors.Is(err, modelErr) || snapshot.State.Phase != contract.RunReconciling || snapshot.State.CompletionClaim != "" {
		t.Fatalf("unknown model reply was falsely terminalized: snapshot=%+v err=%v", snapshot, err)
	}

	invalidModel := &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{{State: harnessports.TurnCompleted, NativeSessionRef: "session", EvidenceDigest: testkit.Digest("invalid")}}}
	loop = newInternalLoop(t, invalidModel, &fakes.MemoryEvents{}, 16, 4, nil)
	intent, fence = testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "invalid")
	if _, err := loop.Start(context.Background(), StartRequest{Run: contract.RunRef{Scope: scope, RunID: "run-invalid"}, Input: testkit.Payload("test/v1", "x"), Intent: intent, Fence: fence}); !core.HasReason(err, core.ReasonInvalidState) {
		t.Fatalf("invalid model result not rejected: %v", err)
	}
	invalidSnapshot, inspectErr := loop.Inspect(contract.RunRef{Scope: scope, RunID: "run-invalid"})
	if inspectErr != nil || invalidSnapshot.State.CompletionClaim != contract.ClaimFailed {
		t.Fatalf("invalid result did not leave a failed claim: %+v %v", invalidSnapshot, inspectErr)
	}

	limitedModel := &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{{State: harnessports.TurnInputRequired, NativeSessionRef: "session", EvidenceDigest: testkit.Digest("input")}}}
	loop = newInternalLoop(t, limitedModel, &fakes.MemoryEvents{}, 16, 1, nil)
	intent, fence = testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "limit-1")
	_, err = loop.Start(context.Background(), StartRequest{Run: contract.RunRef{Scope: scope, RunID: "run-limit"}, Input: testkit.Payload("test/v1", "x"), Intent: intent, Fence: fence})
	if err != nil {
		t.Fatal(err)
	}
	intent, fence = testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "limit-2")
	snapshot, err = loop.ProvideInput(context.Background(), ProvideInputRequest{Run: contract.RunRef{Scope: scope, RunID: "run-limit"}, Input: testkit.Payload("test/v1", "y"), Intent: intent, Fence: fence})
	if err != nil || snapshot.State.CompletionClaim != contract.ClaimFailed || limitedModel.CallCount() != 1 {
		t.Fatalf("turn limit dispatched an extra model turn: snapshot=%+v err=%v calls=%d", snapshot, err, limitedModel.CallCount())
	}
}

func TestEventLimitsAndTerminalAppendFailureNeverClaimSuccess(t *testing.T) {
	t.Parallel()
	model := &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{internalCompleted("done")}}
	loop := newInternalLoop(t, model, &fakes.MemoryEvents{}, 2, 4, nil)
	scope := internalScope(loop)
	intent, fence := testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "event-limit")
	_, err := loop.Start(context.Background(), StartRequest{Run: contract.RunRef{Scope: scope, RunID: "run-limit"}, Input: testkit.Payload("test/v1", "x"), Intent: intent, Fence: fence})
	if !core.HasReason(err, core.ReasonPlanInvalid) {
		t.Fatalf("event limit was not enforced: %v", err)
	}
	snapshot, err := loop.Inspect(contract.RunRef{Scope: scope, RunID: "run-limit"})
	if err != nil || snapshot.State.Phase == contract.RunTerminal || snapshot.State.CompletionClaim != "" {
		t.Fatalf("event limit falsely claimed terminal: %+v %v", snapshot, err)
	}

	eventErr := errors.New("terminal event unavailable")
	events := &fakes.MemoryEvents{FailAfter: 4, Err: eventErr}
	model = &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{internalCompleted("done")}}
	loop = newInternalLoop(t, model, events, 16, 4, nil)
	scope = internalScope(loop)
	intent, fence = testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "terminal-event")
	_, err = loop.Start(context.Background(), StartRequest{Run: contract.RunRef{Scope: scope, RunID: "run-terminal-event"}, Input: testkit.Payload("test/v1", "x"), Intent: intent, Fence: fence})
	if !errors.Is(err, eventErr) {
		t.Fatalf("terminal event failure not surfaced: %v", err)
	}
	snapshot, _ = loop.Inspect(contract.RunRef{Scope: scope, RunID: "run-terminal-event"})
	if snapshot.State.Phase == contract.RunTerminal || snapshot.State.CompletionClaim != "" {
		t.Fatalf("unpersisted terminal event produced a claim: %+v", snapshot.State)
	}
}

func TestPostProviderEvidenceFailureRequiresReconciliationAndNeverRedispatches(t *testing.T) {
	t.Parallel()
	output := testkit.Payload("test.output/v1", "done")
	model := &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{{State: harnessports.TurnCompleted, Output: &output, NativeSessionRef: "native", EvidenceDigest: testkit.Digest("provider")}}}
	events := &fakes.MemoryEvents{FailAfter: 2, Err: core.NewError(core.ErrorUnavailable, core.ReasonEvidenceUnavailable, "journal unavailable")}
	loop := newInternalLoop(t, model, events, 16, 4, nil)
	scope := internalScope(loop)
	run := contract.RunRef{Scope: scope, RunID: "run-post-provider-journal"}
	intent, fence := testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "turn")
	snapshot, err := loop.Start(context.Background(), StartRequest{Run: run, Input: testkit.Payload("test.input/v1", "start"), Intent: intent, Fence: fence})
	if !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("journal failure was hidden: %v", err)
	}
	if snapshot.State.Phase != contract.RunReconciling || snapshot.State.CompletionClaim != "" || model.CallCount() != 1 {
		t.Fatalf("post-provider failure was not held for reconciliation: %+v calls=%d", snapshot.State, model.CallCount())
	}
	if _, err := loop.Start(context.Background(), StartRequest{Run: run, Input: testkit.Payload("test.input/v1", "retry"), Intent: intent, Fence: fence}); err == nil || model.CallCount() != 1 {
		t.Fatalf("same effect was redispatched after uncertain evidence persistence: %v calls=%d", err, model.CallCount())
	}
}

func TestHarnessSessionIsStableAndProviderNativeSessionRemainsObservation(t *testing.T) {
	t.Parallel()
	first := harnessports.ModelTurnResult{State: harnessports.TurnInputRequired, NativeSessionRef: "provider-native-one", EvidenceDigest: testkit.Digest("first")}
	output := testkit.Payload("test.output/v1", "done")
	second := harnessports.ModelTurnResult{State: harnessports.TurnCompleted, Output: &output, NativeSessionRef: "provider-native-two", EvidenceDigest: testkit.Digest("second")}
	loop := newInternalLoop(t, &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{first, second}}, &fakes.MemoryEvents{}, 32, 4, nil)
	scope := internalScope(loop)
	run := contract.RunRef{Scope: scope, RunID: "stable-session"}
	intent, fence := testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "stable-session-start")
	started, err := loop.Start(context.Background(), StartRequest{Run: run, Input: testkit.Payload("test.input/v1", "start"), Intent: intent, Fence: fence})
	if err != nil {
		t.Fatal(err)
	}
	if started.State.SessionRef == first.NativeSessionRef || started.State.SessionRef == "" {
		t.Fatalf("Harness session was replaced by provider native session: %q", started.State.SessionRef)
	}
	stable := started.State.SessionRef
	intent, fence = testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "stable-session-input")
	finished, err := loop.ProvideInput(context.Background(), ProvideInputRequest{Run: run, Input: testkit.Payload("test.input/v1", "continue"), Intent: intent, Fence: fence})
	if err != nil {
		t.Fatal(err)
	}
	if finished.State.SessionRef != stable || finished.State.SessionRef == second.NativeSessionRef {
		t.Fatalf("Harness session drifted across provider turns: before=%q after=%q", stable, finished.State.SessionRef)
	}
	events, err := loop.Events(run)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(first.NativeSessionRef)) || !bytes.Contains(encoded, []byte(second.NativeSessionRef)) {
		t.Fatalf("provider native sessions were not preserved as observations: %s", encoded)
	}
}

func TestHarnessSessionRefPartitionsSameRunIDByFullScope(t *testing.T) {
	t.Parallel()
	manifest := testkit.Manifest(kernelTestNow, runtimeports.ConformanceFullyControlled)
	left := contract.RunRef{Scope: testkit.Scope(manifest.Bootstrap.ResolvedPlanDigest), RunID: "shared-run"}
	right := left
	right.Scope.Identity.TenantID = "tenant-b"
	if err := right.Validate(); err != nil {
		t.Fatal(err)
	}
	if harnessSessionRef(left) == harnessSessionRef(right) {
		t.Fatal("Harness session identity collapsed distinct tenant scopes")
	}
}

func TestCancelInspectEventsAndInternalSerializationFailures(t *testing.T) {
	t.Parallel()
	model := &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{
		{State: harnessports.TurnActionRequired, NativeSessionRef: "session", EvidenceDigest: testkit.Digest("action"), Action: &contract.ActionRequest{Ref: "a", Capability: "tool", Payload: testkit.Payload("test/v1", "a")}},
	}}
	loop := newInternalLoop(t, model, &fakes.MemoryEvents{}, 16, 4, nil)
	scope := internalScope(loop)
	intent, fence := testkit.IntentFence(kernelTestNow, scope, loop.config.Manifest.Bootstrap.CapabilityGrantDigest, "cancel")
	_, err := loop.Start(context.Background(), StartRequest{Run: contract.RunRef{Scope: scope, RunID: "run-cancel"}, Input: testkit.Payload("test/v1", "x"), Intent: intent, Fence: fence})
	if err != nil {
		t.Fatal(err)
	}
	run := contract.RunRef{Scope: scope, RunID: "run-cancel"}
	first, err := loop.Cancel(context.Background(), CancelRequest{Run: run, Intent: intent, Fence: fence})
	if err != nil || first.State.CompletionClaim != contract.ClaimCancelled {
		t.Fatalf("waiting run did not cancel: %+v %v", first, err)
	}
	second, err := loop.Cancel(context.Background(), CancelRequest{Run: run, Intent: intent, Fence: fence})
	if err != nil || second.State.Revision != first.State.Revision || second.State.SourceSequence != first.State.SourceSequence {
		t.Fatalf("terminal cancel was not idempotent: first=%+v second=%+v err=%v", first.State, second.State, err)
	}
	missing := contract.RunRef{Scope: scope, RunID: "missing"}
	if _, err := loop.Cancel(context.Background(), CancelRequest{Run: missing, Intent: intent, Fence: fence}); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("missing cancel returned wrong error: %v", err)
	}
	if _, err := loop.Inspect(missing); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("missing inspect returned wrong error: %v", err)
	}
	if _, err := loop.Events(missing); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("missing events returned wrong error: %v", err)
	}
	events, err := loop.Events(run)
	if err != nil || len(events) == 0 {
		t.Fatalf("run events unavailable: %v", err)
	}
	events[0].Payload.Payload[0] = 'x'
	again, _ := loop.Events(run)
	if again[0].Payload.Payload[0] == 'x' {
		t.Fatal("Events leaked mutable payload storage")
	}

	s := &session{state: contract.RunState{Ref: contract.RunRef{Scope: scope, RunID: "manual"}}}
	loop.mu.Lock()
	err = loop.appendLocked(context.Background(), s, contract.EventRunStarted, make(chan struct{}))
	loop.mu.Unlock()
	if !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("unserializable event payload returned wrong error: %v", err)
	}
}

func newInternalLoop(t *testing.T, model harnessports.ModelTurnPort, events harnessports.EventCandidateJournalPort, maxEvents uint64, maxTurns uint32, contextOverride harnessports.ContextPort) *Loop {
	t.Helper()
	manifest := testkit.Manifest(kernelTestNow, runtimeports.ConformanceFullyControlled)
	contextPort := contextOverride
	if contextPort == nil {
		contextPort = &fakes.StaticContext{Snapshot: testkit.Context(kernelTestNow)}
	}
	loop, err := New(Config{Manifest: manifest, Context: contextPort, Model: model, Events: events, Clock: func() time.Time { return kernelTestNow }, MaxEvents: maxEvents, MaxTurns: maxTurns})
	if err != nil {
		t.Fatal(err)
	}
	return loop
}

func internalScope(loop *Loop) core.ExecutionScope {
	return testkit.Scope(loop.config.Manifest.Bootstrap.ResolvedPlanDigest)
}

func internalCompleted(output string) harnessports.ModelTurnResult {
	payload := testkit.Payload("test.output/v1", output)
	return harnessports.ModelTurnResult{State: harnessports.TurnCompleted, Output: &payload, NativeSessionRef: "session", EvidenceDigest: testkit.Digest("evidence-" + output)}
}
