package fakes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

var fakesTestNow = time.Date(2026, 7, 14, 11, 30, 0, 0, time.UTC)

func TestStaticContextValidationErrorAndClone(t *testing.T) {
	t.Parallel()
	manifest := testkit.Manifest(fakesTestNow, runtimeports.ConformanceFullyControlled)
	run := contract.RunRef{Scope: testkit.Scope(manifest.Bootstrap.ResolvedPlanDigest), RunID: "run"}
	request := ports.ContextRequest{Run: run, ContextPlanDigest: manifest.Bootstrap.ContextPlanDigest, Input: testkit.Payload("test/v1", "input")}
	fake := &StaticContext{Snapshot: testkit.Context(fakesTestNow)}
	result, err := fake.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result.Payload.Payload[0] = 'x'
	if fake.Snapshot.Payload.Payload[0] == 'x' {
		t.Fatal("StaticContext leaked mutable payload")
	}
	bad := request
	bad.Run.RunID = ""
	if _, err := fake.Prepare(context.Background(), bad); err == nil {
		t.Fatal("invalid request accepted")
	}
	injected := errors.New("context failure")
	fake.Err = injected
	if _, err := fake.Prepare(context.Background(), request); !errors.Is(err, injected) {
		t.Fatalf("injected error not returned: %v", err)
	}
}

func TestMemoryEventsValidationFailureThresholdAndClone(t *testing.T) {
	t.Parallel()
	event := fakeTestEvent()
	fake := &MemoryEvents{}
	if err := fake.AppendCandidate(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	snapshot := fake.Snapshot()
	snapshot[0].Payload.Payload[0] = 'x'
	if fake.Snapshot()[0].Payload.Payload[0] == 'x' {
		t.Fatal("MemoryEvents leaked mutable payload")
	}
	bad := event
	bad.SourceSequence = 0
	if err := fake.AppendCandidate(context.Background(), bad); err == nil {
		t.Fatal("invalid event accepted")
	}
	injected := errors.New("event store unavailable")
	fake = &MemoryEvents{Err: injected}
	if err := fake.AppendCandidate(context.Background(), event); !errors.Is(err, injected) {
		t.Fatalf("immediate failure not injected: %v", err)
	}
	fake = &MemoryEvents{Err: injected, FailAfter: 1}
	if err := fake.AppendCandidate(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	event.SourceSequence++
	if err := fake.AppendCandidate(context.Background(), event); !errors.Is(err, injected) {
		t.Fatalf("threshold failure not injected: %v", err)
	}
}

func TestMemoryEventsExactInspectIdempotencyAndConflict(t *testing.T) {
	t.Parallel()
	event := fakeTestEvent()
	fake := &MemoryEvents{LoseNextReply: true, LostReplyErr: context.DeadlineExceeded}
	if err := fake.AppendCandidate(context.Background(), event); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lost append reply was not injected: %v", err)
	}
	inspected, err := fake.InspectCandidate(context.Background(), event.SourceComponentID, event.SourceEpoch, event.SourceSequence)
	if err != nil || !sameEvent(inspected, event) {
		t.Fatalf("committed event was not recoverable by exact source key: %+v %v", inspected, err)
	}
	inspected.Payload.Payload[0] = 'x'
	again, err := fake.InspectCandidate(context.Background(), event.SourceComponentID, event.SourceEpoch, event.SourceSequence)
	if err != nil || again.Payload.Payload[0] == 'x' {
		t.Fatalf("InspectCandidate leaked mutable payload storage: %+v %v", again, err)
	}
	if err := fake.AppendCandidate(context.Background(), event); err != nil || len(fake.Snapshot()) != 1 {
		t.Fatalf("exact replay was not idempotent: err=%v count=%d", err, len(fake.Snapshot()))
	}
	conflict := event
	conflict.Payload = testkit.Payload("test.event/v1", "different")
	if err := fake.AppendCandidate(context.Background(), conflict); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("same source sequence with different content was accepted: %v", err)
	}
	if _, err := fake.InspectCandidate(context.Background(), event.SourceComponentID, event.SourceEpoch, event.SourceSequence+1); !core.HasReason(err, core.ReasonEvidenceUnavailable) {
		t.Fatalf("missing exact source key returned wrong error: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fake.InspectCandidate(cancelled, event.SourceComponentID, event.SourceEpoch, event.SourceSequence); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled inspection ignored context: %v", err)
	}
}

func TestScriptedAndBlockingModelBehavior(t *testing.T) {
	t.Parallel()
	request := fakeModelRequest("turn")
	output := testkit.Payload("test.output/v1", "done")
	want := ports.ModelTurnResult{State: ports.TurnCompleted, Output: &output, NativeSessionRef: "session", EvidenceDigest: testkit.Digest("evidence")}
	fake := &ScriptedModel{Results: []ports.ModelTurnResult{want}}
	got, err := fake.Invoke(context.Background(), request)
	if err != nil || got.NativeSessionRef != want.NativeSessionRef || fake.CallCount() != 1 {
		t.Fatalf("scripted result mismatch: got=%+v err=%v calls=%d", got, err, fake.CallCount())
	}
	if _, err := fake.Invoke(context.Background(), request); !core.HasReason(err, core.ReasonEvidenceUnavailable) {
		t.Fatalf("empty script returned wrong error: %v", err)
	}
	injected := errors.New("model failure")
	fake = &ScriptedModel{Err: injected}
	if _, err := fake.Invoke(context.Background(), request); !errors.Is(err, injected) || fake.CallCount() != 1 {
		t.Fatalf("model failure not injected: %v", err)
	}
	bad := request
	bad.Intent.PersistedAt = time.Time{}
	if _, err := fake.Invoke(context.Background(), bad); err == nil || fake.CallCount() != 1 {
		t.Fatalf("invalid request was recorded: err=%v calls=%d", err, fake.CallCount())
	}

	blocking := &BlockingModel{Started: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := blocking.Invoke(ctx, request); done <- err }()
	<-blocking.Started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("blocking model did not propagate cancel: %v", err)
	}
}

func fakeModelRequest(id string) ports.ModelTurnRequest {
	manifest := testkit.Manifest(fakesTestNow, runtimeports.ConformanceFullyControlled)
	scope := testkit.Scope(manifest.Bootstrap.ResolvedPlanDigest)
	intent, fence := testkit.IntentFence(fakesTestNow, scope, manifest.Bootstrap.CapabilityGrantDigest, id)
	return ports.ModelTurnRequest{
		Run: contract.RunRef{Scope: scope, RunID: "run"}, Input: testkit.Payload("test/v1", "input"),
		Context: testkit.Context(fakesTestNow), Intent: intent, Fence: fence,
	}
}

func fakeTestEvent() contract.Event {
	return contract.Event{
		SourceComponentID: "harness", SourceEpoch: 1, SourceSequence: 1, RunID: "run",
		Kind: contract.EventRunStarted, Payload: testkit.Payload("test.event/v1", "event"), ObservedAt: fakesTestNow,
	}
}
