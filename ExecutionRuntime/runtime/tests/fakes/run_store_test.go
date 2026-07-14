package fakes_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRunFactStoreAtomicallyAllowsOneActiveRunPerInstance(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)
	store := fakes.NewFactStore(func() time.Time { return now })
	scope := runScope(t)

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, runID := range []core.AgentRunID{"run-a", "run-b"} {
		wg.Add(1)
		go func(id core.AgentRunID) {
			defer wg.Done()
			_, err := store.CreateRun(context.Background(), runningRecord(scope, id, now))
			results <- err
		}(runID)
	}
	wg.Wait()
	close(results)

	var successes, conflicts int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case core.HasReason(err, core.ReasonRunConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent create result: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("expected one active fact and one conflict, got success=%d conflict=%d", successes, conflicts)
	}
	active, err := store.InspectActiveRun(context.Background(), scope)
	if err != nil || active.Status != core.RunRunning {
		t.Fatalf("active run fact was not durable: record=%+v err=%v", active, err)
	}
}

func TestRunFactStoreRecoversCommittedTerminalAfterReplyLoss(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 1, 0, 0, time.UTC)
	store := fakes.NewFactStore(func() time.Time { return now })
	scope := runScope(t)
	current, err := store.CreateRun(context.Background(), runningRecord(scope, "run-restart", now))
	if err != nil {
		t.Fatal(err)
	}

	next := current
	next.Status = core.RunTerminal
	next.Revision++
	next.EndedAt = now.Add(time.Second)
	next.Outcome = core.OutcomeCompleted
	store.LoseNextRunWriteReply()
	if _, err := store.CompareAndSwapRun(context.Background(), control.RunFactCASRequest{ExpectedRevision: current.Revision, Next: next}); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected injected lost reply, got %v", err)
	}

	// A fresh coordinator can recover solely by inspecting the persisted fact.
	recovered, err := store.InspectRun(context.Background(), scope, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != core.RunTerminal || recovered.Outcome != core.OutcomeCompleted || recovered.Revision != 2 {
		t.Fatalf("committed terminal fact was lost across restart: %+v", recovered)
	}
	if _, err := store.InspectActiveRun(context.Background(), scope); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("terminal run remained in active index: %v", err)
	}
}

func TestRunFactStoreRejectsStaleOrIllegalCASWithoutMutation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 2, 0, 0, time.UTC)
	store := fakes.NewFactStore(func() time.Time { return now })
	scope := runScope(t)
	current, err := store.CreateRun(context.Background(), runningRecord(scope, "run-cas", now))
	if err != nil {
		t.Fatal(err)
	}

	illegal := current
	illegal.Revision++
	illegal.Status = core.RunPending
	illegal.StartedAt = time.Time{}
	if _, err := store.CompareAndSwapRun(context.Background(), control.RunFactCASRequest{ExpectedRevision: current.Revision, Next: illegal}); !core.HasReason(err, core.ReasonInvalidTransition) {
		t.Fatalf("illegal reverse transition was accepted: %v", err)
	}
	terminal := current
	terminal.Revision++
	terminal.Status = core.RunTerminal
	terminal.EndedAt = now.Add(time.Second)
	terminal.Outcome = core.OutcomeCompleted
	if _, err := store.CompareAndSwapRun(context.Background(), control.RunFactCASRequest{ExpectedRevision: current.Revision + 1, Next: terminal}); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatalf("stale CAS was accepted: %v", err)
	}
	unchanged, err := store.InspectRun(context.Background(), scope, current.ID)
	if err != nil || !reflect.DeepEqual(unchanged, current) {
		t.Fatalf("failed CAS partially mutated run fact: record=%+v err=%v", unchanged, err)
	}
}

func runningRecord(scope core.ExecutionScope, runID core.AgentRunID, now time.Time) core.AgentRunRecord {
	session, _ := ports.DeriveRuntimeExecutionSessionRefV2("endpoint-run", runID)
	return core.AgentRunRecord{ID: runID, Scope: scope, Status: core.RunRunning, Revision: 1, SessionRef: session, StartedAt: now}
}

func runScope(t *testing.T) core.ExecutionScope {
	t.Helper()
	plan, err := core.DigestJSON("run-fact-plan")
	if err != nil {
		t.Fatal(err)
	}
	return core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-run", ID: "agent-run", Epoch: 2},
		Lineage:        core.LineageRef{ID: "lineage-run", PlanDigest: plan},
		Instance:       core.InstanceRef{ID: "instance-run", Epoch: 3},
		SandboxLease:   &core.SandboxLeaseRef{ID: "sandbox-run", Epoch: 1},
		AuthorityEpoch: 4,
	}
}
