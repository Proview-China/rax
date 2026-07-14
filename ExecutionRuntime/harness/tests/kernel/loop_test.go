package kernel_test

import (
	"context"
	"errors"
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

func TestLoopCompletesWithSourceOrderedPersistedEvents(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t, &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{
		completedResult("session-1", "done"),
	}})
	intent, fence := testkit.IntentFence(fixture.now, fixture.scope, fixture.manifest.Bootstrap.CapabilityGrantDigest, "turn-1")
	snapshot, err := fixture.loop.Start(context.Background(), kernel.StartRequest{
		Run: contract.RunRef{Scope: fixture.scope, RunID: "run-1"}, Input: testkit.Payload("test.input/v1", "hello"), Intent: intent, Fence: fence,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.State.Phase != contract.RunTerminal || snapshot.State.CompletionClaim != contract.ClaimCompleted {
		t.Fatalf("run did not complete: %+v", snapshot.State)
	}
	events, err := fixture.loop.Events(contract.RunRef{Scope: fixture.scope, RunID: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []contract.EventKind{contract.EventRunStarted, contract.EventModelTurnStarted, contract.EventModelTurnObserved, contract.EventModelOutput, contract.EventRunCompleted}
	if len(events) != len(want) {
		t.Fatalf("events=%d want=%d", len(events), len(want))
	}
	for index, kind := range want {
		if events[index].Kind != kind || events[index].SourceSequence != uint64(index+1) {
			t.Fatalf("event[%d]=%+v", index, events[index])
		}
	}
}

func TestLoopStopsForActionAndOnlyResumesOnExplicitResult(t *testing.T) {
	t.Parallel()
	model := &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{
		{
			State: harnessports.TurnActionRequired, NativeSessionRef: "session-1", EvidenceDigest: testkit.Digest("turn-1"),
			Action: &contract.ActionRequest{Ref: "action-1", Capability: "tool.search", Payload: testkit.Payload("test.action/v1", map[string]string{"query": "praxis"}), ReviewRequired: true},
		},
		completedResult("session-1", "action-applied"),
	}}
	fixture := newFixture(t, model)
	intent1, fence1 := testkit.IntentFence(fixture.now, fixture.scope, fixture.manifest.Bootstrap.CapabilityGrantDigest, "turn-1")
	snapshot, err := fixture.loop.Start(context.Background(), kernel.StartRequest{
		Run: contract.RunRef{Scope: fixture.scope, RunID: "run-action"}, Input: testkit.Payload("test.input/v1", "start"), Intent: intent1, Fence: fence1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.State.Phase != contract.RunWaitingAction || snapshot.State.PendingAction == nil || !snapshot.State.PendingAction.ReviewRequired {
		t.Fatalf("action did not stop at review gateway: %+v", snapshot.State)
	}

	intent2, fence2 := testkit.IntentFence(fixture.now, fixture.scope, fixture.manifest.Bootstrap.CapabilityGrantDigest, "turn-2")
	_, err = fixture.loop.ProvideActionResult(context.Background(), kernel.ProvideActionResultRequest{
		Run: contract.RunRef{Scope: fixture.scope, RunID: "run-action"}, Result: contract.ActionResult{Ref: "wrong-action", Payload: testkit.Payload("test.action-result/v1", map[string]string{"status": "approved"})}, Intent: intent2, Fence: fence2,
	})
	if !core.HasReason(err, core.ReasonInvalidState) || model.CallCount() != 1 {
		t.Fatalf("wrong action reference must not resume the model: err=%v calls=%d", err, model.CallCount())
	}
	snapshot, err = fixture.loop.ProvideActionResult(context.Background(), kernel.ProvideActionResultRequest{
		Run: contract.RunRef{Scope: fixture.scope, RunID: "run-action"}, Result: contract.ActionResult{Ref: "action-1", Payload: testkit.Payload("test.action-result/v1", map[string]string{"status": "approved"})}, Intent: intent2, Fence: fence2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.State.CompletionClaim != contract.ClaimCompleted || model.CallCount() != 2 {
		t.Fatalf("reviewed continuation did not complete exactly once: state=%+v calls=%d", snapshot.State, model.CallCount())
	}
	events, _ := fixture.loop.Events(contract.RunRef{Scope: fixture.scope, RunID: "run-action"})
	want := []contract.EventKind{
		contract.EventRunStarted, contract.EventModelTurnStarted, contract.EventModelTurnObserved, contract.EventActionRequested,
		contract.EventActionResultReceived, contract.EventModelTurnStarted, contract.EventModelTurnObserved, contract.EventModelOutput, contract.EventRunCompleted,
	}
	if len(events) != len(want) {
		t.Fatalf("events=%v", events)
	}
	for index := range want {
		if events[index].Kind != want[index] {
			t.Fatalf("event[%d]=%s want=%s", index, events[index].Kind, want[index])
		}
	}
}

func TestEventBackpressurePreventsModelDispatch(t *testing.T) {
	t.Parallel()
	model := &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{completedResult("session-1", "must-not-run")}}
	fixture := newFixtureWithEvents(t, model, &fakes.MemoryEvents{FailAfter: 1, Err: errors.New("ledger unavailable")})
	intent, fence := testkit.IntentFence(fixture.now, fixture.scope, fixture.manifest.Bootstrap.CapabilityGrantDigest, "turn-backpressure")
	_, err := fixture.loop.Start(context.Background(), kernel.StartRequest{
		Run: contract.RunRef{Scope: fixture.scope, RunID: "run-backpressure"}, Input: testkit.Payload("test.input/v1", "hello"), Intent: intent, Fence: fence,
	})
	if err == nil || model.CallCount() != 0 {
		t.Fatalf("event failure must prevent model dispatch: err=%v calls=%d", err, model.CallCount())
	}
}

func TestEventAppendLostReplyRecoversByExactSourceInspect(t *testing.T) {
	t.Parallel()
	events := &fakes.MemoryEvents{LoseNextReply: true}
	model := &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{completedResult("native", "done")}}
	fixture := newFixtureWithEvents(t, model, events)
	intent, fence := testkit.IntentFence(fixture.now, fixture.scope, fixture.manifest.Bootstrap.CapabilityGrantDigest, "lost-event-reply")
	run := contract.RunRef{Scope: fixture.scope, RunID: "run-lost-event-reply"}
	snapshot, err := fixture.loop.Start(context.Background(), kernel.StartRequest{Run: run, Input: testkit.Payload("test.input/v1", "hello"), Intent: intent, Fence: fence})
	if err != nil || snapshot.State.CompletionClaim != contract.ClaimCompleted || model.CallCount() != 1 {
		t.Fatalf("lost append reply was not recovered exactly once: snapshot=%+v err=%v calls=%d", snapshot, err, model.CallCount())
	}
	persisted, err := fixture.loop.Events(run)
	if err != nil || len(persisted) != 5 {
		t.Fatalf("recovered event journal is incomplete: events=%d err=%v", len(persisted), err)
	}
}

func TestNativeTimeoutAfterEventCommitRecoversByExactSourceInspect(t *testing.T) {
	t.Parallel()
	events := &fakes.MemoryEvents{LoseNextReply: true, LostReplyErr: context.DeadlineExceeded}
	model := &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{completedResult("native", "done")}}
	fixture := newFixtureWithEvents(t, model, events)
	intent, fence := testkit.IntentFence(fixture.now, fixture.scope, fixture.manifest.Bootstrap.CapabilityGrantDigest, "native-timeout")
	run := contract.RunRef{Scope: fixture.scope, RunID: "run-native-timeout"}
	snapshot, err := fixture.loop.Start(context.Background(), kernel.StartRequest{Run: run, Input: testkit.Payload("test.input/v1", "hello"), Intent: intent, Fence: fence})
	if err != nil || snapshot.State.CompletionClaim != contract.ClaimCompleted || model.CallCount() != 1 {
		t.Fatalf("native lost-reply timeout was not inspected exactly once: snapshot=%+v err=%v calls=%d", snapshot, err, model.CallCount())
	}
}

func TestSequentialRunsOnOneInstanceUseIndependentEvidenceSources(t *testing.T) {
	t.Parallel()
	events := &fakes.MemoryEvents{}
	model := &fakes.ScriptedModel{Results: []harnessports.ModelTurnResult{completedResult("native-1", "one"), completedResult("native-2", "two")}}
	fixture := newFixtureWithEvents(t, model, events)
	for _, runID := range []core.AgentRunID{"run-one", "run-two"} {
		intent, fence := testkit.IntentFence(fixture.now, fixture.scope, fixture.manifest.Bootstrap.CapabilityGrantDigest, string(runID))
		snapshot, err := fixture.loop.Start(context.Background(), kernel.StartRequest{Run: contract.RunRef{Scope: fixture.scope, RunID: runID}, Input: testkit.Payload("test.input/v1", string(runID)), Intent: intent, Fence: fence})
		if err != nil || snapshot.State.Phase != contract.RunTerminal {
			t.Fatalf("sequential Run %s failed: %+v %v", runID, snapshot.State, err)
		}
	}
	recorded := events.Snapshot()
	if len(recorded) != 10 || recorded[0].SourceComponentID == recorded[5].SourceComponentID || recorded[0].SourceSequence != 1 || recorded[5].SourceSequence != 1 {
		t.Fatalf("sequential Runs reused one source sequence domain: %+v", recorded)
	}
}

func TestCancelRejectsLateModelResultAndNeverRevivesRun(t *testing.T) {
	t.Parallel()
	model := &fakes.BlockingModel{Started: make(chan struct{})}
	fixture := newFixture(t, model)
	intent, fence := testkit.IntentFence(fixture.now, fixture.scope, fixture.manifest.Bootstrap.CapabilityGrantDigest, "turn-blocked")
	result := make(chan struct {
		snapshot contract.Snapshot
		err      error
	}, 1)
	go func() {
		snapshot, err := fixture.loop.Start(context.Background(), kernel.StartRequest{
			Run: contract.RunRef{Scope: fixture.scope, RunID: "run-cancel"}, Input: testkit.Payload("test.input/v1", "hello"), Intent: intent, Fence: fence,
		})
		result <- struct {
			snapshot contract.Snapshot
			err      error
		}{snapshot, err}
	}()
	select {
	case <-model.Started:
	case <-time.After(time.Second):
		t.Fatal("model invocation did not start")
	}
	if _, err := fixture.loop.Cancel(context.Background(), kernel.CancelRequest{Run: contract.RunRef{Scope: fixture.scope, RunID: "run-cancel"}, Intent: intent, Fence: fence}); err != nil {
		t.Fatal(err)
	}
	select {
	case completed := <-result:
		if completed.err != nil {
			t.Fatal(completed.err)
		}
		if completed.snapshot.State.Phase != contract.RunTerminal || completed.snapshot.State.CompletionClaim != contract.ClaimCancelled {
			t.Fatalf("late result revived cancelled run: %+v", completed.snapshot.State)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not interrupt model invocation")
	}
}

func TestActiveRunIsolationIsPerExecutionScope(t *testing.T) {
	t.Parallel()
	model := &multiBlockingModel{started: make(chan core.AgentRunID, 2)}
	fixture := newFixture(t, model)
	scope2 := fixture.scope
	scope2.Identity.ID = "agent-2"
	scope2.Lineage.ID = "lineage-2"
	scope2.Instance.ID = "instance-2"
	scope2.SandboxLease = &core.SandboxLeaseRef{ID: "sandbox-2", Epoch: 1}

	type result struct {
		runID core.AgentRunID
		err   error
	}
	results := make(chan result, 2)
	start := func(runID core.AgentRunID, scope core.ExecutionScope) {
		intent, fence := testkit.IntentFence(fixture.now, scope, fixture.manifest.Bootstrap.CapabilityGrantDigest, string(runID))
		_, err := fixture.loop.Start(context.Background(), kernel.StartRequest{
			Run: contract.RunRef{Scope: scope, RunID: runID}, Input: testkit.Payload("test.input/v1", string(runID)), Intent: intent, Fence: fence,
		})
		results <- result{runID: runID, err: err}
	}
	go start("run-scope-1", fixture.scope)
	go start("run-scope-2", scope2)
	seen := map[core.AgentRunID]bool{}
	for len(seen) < 2 {
		select {
		case runID := <-model.started:
			seen[runID] = true
		case completed := <-results:
			t.Fatalf("run %s returned before both scopes entered model dispatch: %v", completed.runID, completed.err)
		case <-time.After(time.Second):
			t.Fatalf("different execution scopes blocked each other: started=%v", seen)
		}
	}
	if fixture.loop.ActiveRun(fixture.scope) != "run-scope-1" || fixture.loop.ActiveRun(scope2) != "run-scope-2" {
		t.Fatalf("active run index is not scope isolated")
	}
	cross := contract.RunRef{Scope: fixture.scope, RunID: "run-scope-2"}
	if _, err := fixture.loop.Inspect(cross); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("scope one inspected scope two's Run: %v", err)
	}
	if _, err := fixture.loop.Events(cross); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("scope one read scope two's events: %v", err)
	}
	crossIntent, crossFence := testkit.IntentFence(fixture.now, fixture.scope, fixture.manifest.Bootstrap.CapabilityGrantDigest, "cross-cancel")
	if _, err := fixture.loop.Cancel(context.Background(), kernel.CancelRequest{Run: cross, Intent: crossIntent, Fence: crossFence}); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("scope one controlled scope two's Run: %v", err)
	}
	intent1, fence1 := testkit.IntentFence(fixture.now, fixture.scope, fixture.manifest.Bootstrap.CapabilityGrantDigest, "cancel-scope-1")
	if _, err := fixture.loop.Cancel(context.Background(), kernel.CancelRequest{Run: contract.RunRef{Scope: fixture.scope, RunID: "run-scope-1"}, Intent: intent1, Fence: fence1}); err != nil {
		t.Fatal(err)
	}
	intent2, fence2 := testkit.IntentFence(fixture.now, scope2, fixture.manifest.Bootstrap.CapabilityGrantDigest, "cancel-scope-2")
	if _, err := fixture.loop.Cancel(context.Background(), kernel.CancelRequest{Run: contract.RunRef{Scope: scope2, RunID: "run-scope-2"}, Intent: intent2, Fence: fence2}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		completed := <-results
		if completed.err != nil {
			t.Fatalf("cancelled run %s failed: %v", completed.runID, completed.err)
		}
	}
}

func TestRunIdentityIsScopePartitionedAndCrossScopeAccessFailsClosed(t *testing.T) {
	t.Parallel()
	model := &multiBlockingModel{started: make(chan core.AgentRunID, 2)}
	fixture := newFixture(t, model)
	scope2 := fixture.scope
	scope2.Identity.TenantID = "tenant-2"
	// Every tenant-local identifier is deliberately reused. Tenant is part of
	// both the session partition and the Evidence source identity.
	run1 := contract.RunRef{Scope: fixture.scope, RunID: "shared-run"}
	run2 := contract.RunRef{Scope: scope2, RunID: "shared-run"}
	results := make(chan error, 2)
	start := func(run contract.RunRef, id string) {
		intent, fence := testkit.IntentFence(fixture.now, run.Scope, fixture.manifest.Bootstrap.CapabilityGrantDigest, id)
		_, err := fixture.loop.Start(context.Background(), kernel.StartRequest{Run: run, Input: testkit.Payload("test.input/v1", id), Intent: intent, Fence: fence})
		results <- err
	}
	go start(run1, "tenant-one")
	go start(run2, "tenant-two")
	for range 2 {
		select {
		case <-model.started:
		case err := <-results:
			t.Fatalf("same RunID in a different tenant collided: %v", err)
		case <-time.After(time.Second):
			t.Fatal("partitioned Runs did not both reach their execution surface")
		}
	}
	wrong := contract.RunRef{Scope: fixture.scope, RunID: "other-tenant-only"}
	if _, err := fixture.loop.Inspect(wrong); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("cross-scope inspect did not fail closed: %v", err)
	}
	if _, err := fixture.loop.Events(wrong); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("cross-scope events did not fail closed: %v", err)
	}
	for index, run := range []contract.RunRef{run1, run2} {
		intent, fence := testkit.IntentFence(fixture.now, run.Scope, fixture.manifest.Bootstrap.CapabilityGrantDigest, fmt.Sprintf("cancel-%d", index))
		if _, err := fixture.loop.Cancel(context.Background(), kernel.CancelRequest{Run: run, Intent: intent, Fence: fence}); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

type fixture struct {
	now      time.Time
	manifest contract.Manifest
	scope    core.ExecutionScope
	loop     *kernel.Loop
}

func newFixture(t *testing.T, model harnessports.ModelTurnPort) fixture {
	t.Helper()
	return newFixtureWithEvents(t, model, &fakes.MemoryEvents{})
}

func newFixtureWithEvents(t *testing.T, model harnessports.ModelTurnPort, events harnessports.EventCandidateJournalPort) fixture {
	t.Helper()
	now := time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)
	manifest := testkit.Manifest(now, runtimeports.ConformanceFullyControlled)
	scope := testkit.Scope(manifest.Bootstrap.ResolvedPlanDigest)
	loop, err := kernel.New(kernel.Config{
		Manifest: manifest, Context: &fakes.StaticContext{Snapshot: testkit.Context(now)}, Model: model, Events: events,
		Clock: func() time.Time { return now }, MaxEvents: 32, MaxTurns: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{now: now, manifest: manifest, scope: scope, loop: loop}
}

func completedResult(session, output string) harnessports.ModelTurnResult {
	payload := testkit.Payload("test.output/v1", output)
	return harnessports.ModelTurnResult{State: harnessports.TurnCompleted, Output: &payload, NativeSessionRef: session, EvidenceDigest: testkit.Digest("evidence-" + output)}
}

type multiBlockingModel struct {
	started chan core.AgentRunID
}

func (m *multiBlockingModel) Invoke(ctx context.Context, request harnessports.ModelTurnRequest) (harnessports.ModelTurnResult, error) {
	m.started <- request.Run.RunID
	<-ctx.Done()
	return harnessports.ModelTurnResult{}, ctx.Err()
}
