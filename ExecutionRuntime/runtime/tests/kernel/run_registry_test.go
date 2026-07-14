package kernel_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
)

func TestRunRegistryEnforcesOneActiveRunPerInstance(t *testing.T) {
	t.Parallel()
	registry := kernel.NewRunRegistry()
	scope := newAggregate(t).Snapshot().Scope
	now := time.Now()
	first, err := registry.Start(scope, "run-1", "session-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Start(scope, "run-2", "session-2", now); !core.HasReason(err, core.ReasonRunConflict) {
		t.Fatalf("second active run must conflict: %v", err)
	}
	finished, err := registry.Finish(scope, first.ID, core.OutcomeCompleted, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != core.RunTerminal || finished.Outcome != core.OutcomeCompleted {
		t.Fatalf("unexpected terminal run: %+v", finished)
	}
	if _, err := registry.Start(scope, "run-2", "session-2", now.Add(2*time.Second)); err != nil {
		t.Fatalf("new run should be admitted after prior terminal result: %v", err)
	}
}
