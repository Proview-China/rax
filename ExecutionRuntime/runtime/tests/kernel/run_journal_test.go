package kernel_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
)

func TestRunJournalRecoversActiveRunAfterStartReplyLoss(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 10, 0, 0, time.UTC)
	store := fakes.NewFactStore(func() time.Time { return now })
	journal, err := kernel.NewRunJournal(store)
	if err != nil {
		t.Fatal(err)
	}
	scope := newAggregate(t).Snapshot().Scope
	store.LoseNextRunWriteReply()
	if _, err := journal.Start(context.Background(), scope, "run-lost-start", "session-lost-start", now); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("expected lost start reply, got %v", err)
	}

	// New process, same fact owner: recover instead of dispatching another run.
	restarted, err := kernel.NewRunJournal(store)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := restarted.RecoverActive(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.ID != "run-lost-start" || recovered.Status != core.RunRunning || recovered.Revision != 1 {
		t.Fatalf("restart did not recover the active run fact: %+v", recovered)
	}
}

func TestRunJournalTerminalSettlementIsIdempotentAndRejectsConflictingClaim(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 11, 0, 0, time.UTC)
	store := fakes.NewFactStore(func() time.Time { return now })
	journal, err := kernel.NewRunJournal(store)
	if err != nil {
		t.Fatal(err)
	}
	scope := newAggregate(t).Snapshot().Scope
	started, err := journal.Start(context.Background(), scope, "run-terminal", "session-terminal", now)
	if err != nil {
		t.Fatal(err)
	}
	finished, err := journal.Finish(context.Background(), scope, started.ID, core.OutcomeCompleted, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := journal.Finish(context.Background(), scope, started.ID, core.OutcomeCompleted, now.Add(2*time.Second))
	if err != nil || replayed.Revision != finished.Revision || !replayed.EndedAt.Equal(finished.EndedAt) {
		t.Fatalf("same terminal settlement was not idempotent: record=%+v err=%v", replayed, err)
	}
	if _, err := journal.Finish(context.Background(), scope, started.ID, core.OutcomeFailed, now.Add(3*time.Second)); !core.HasReason(err, core.ReasonRunConflict) {
		t.Fatalf("conflicting late terminal claim overwrote Runtime outcome: %v", err)
	}
}

func TestRunJournalConcurrentTerminalClaimsLinearizeExactlyOnce(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 8, 12, 0, 0, time.UTC)
	store := fakes.NewFactStore(func() time.Time { return now })
	journal, err := kernel.NewRunJournal(store)
	if err != nil {
		t.Fatal(err)
	}
	scope := newAggregate(t).Snapshot().Scope
	if _, err := journal.Start(context.Background(), scope, "run-race", "session-race", now); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, outcome := range []core.ExecutionOutcome{core.OutcomeCompleted, core.OutcomeFailed} {
		wg.Add(1)
		go func(value core.ExecutionOutcome) {
			defer wg.Done()
			_, err := journal.Finish(context.Background(), scope, "run-race", value, now.Add(time.Second))
			results <- err
		}(outcome)
	}
	wg.Wait()
	close(results)

	var successes, conflicts int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case core.HasReason(err, core.ReasonRevisionConflict), core.HasReason(err, core.ReasonRunConflict):
			conflicts++
		default:
			t.Fatalf("unexpected terminal race result: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("terminal claims did not linearize once: successes=%d conflicts=%d", successes, conflicts)
	}
	persisted, err := journal.Inspect(context.Background(), scope, "run-race")
	if err != nil || persisted.Status != core.RunTerminal || persisted.Revision != 2 {
		t.Fatalf("terminal fact did not remain stable: record=%+v err=%v", persisted, err)
	}
}
