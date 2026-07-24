package knowledge

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestKnowledgeCommitCanonicalFailureLeavesZeroAuthoritativeDelta(t *testing.T) {
	f := newFixture(t, false)

	// Corrupt only the stored Begun Attempt with a time encoding/json cannot
	// encode. Record, Inspection, and DomainResult preparation still succeeds;
	// the final Attempt digest is the last fallible step before linearization.
	tenant := f.store.tenants[f.access.TenantID]
	attempt := tenant.attempts[f.attempt.Ref.ID]
	attempt.BegunAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	tenant.attempts[f.attempt.Ref.ID] = attempt
	before := captureCommitState(f)

	if _, err := f.store.CommitAttempt(f.access, f.attempt.Ref.ID); !errors.Is(err, contract.ErrInvalidArgument) {
		t.Fatalf("canonical failure = %v, want invalid argument", err)
	}
	assertZeroCommitDelta(t, f, before)
}

func TestKnowledgeCommitInjectedFailureConcurrentLeavesZeroAuthoritativeDelta(t *testing.T) {
	f := newFixture(t, false)
	before := captureCommitState(f)
	injected := errors.New("injected before linearized commit")
	f.store.beforeLinearizedCommit = func() error { return injected }

	const callers = 32
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.store.CommitAttempt(f.access, f.attempt.Ref.ID)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, injected) {
			t.Fatalf("concurrent injected failure = %v", err)
		}
	}
	assertZeroCommitDelta(t, f, before)

	// Once the pre-commit failure clears, the original attempt remains usable
	// and exactly one caller may publish the fully prepared batch.
	f.store.beforeLinearizedCommit = nil
	if _, err := f.store.CommitAttempt(f.access, f.attempt.Ref.ID); err != nil {
		t.Fatalf("original attempt did not recover: %v", err)
	}
	tenant := f.store.tenants[f.access.TenantID]
	if len(tenant.records[f.candidate.Draft.ID]) != 1 || len(tenant.results) != 1 || len(tenant.inspections) != 1 || tenant.attempts[f.attempt.Ref.ID].State != AttemptApplied {
		t.Fatalf("recovery was not one complete commit: records=%d results=%d inspections=%d attempt=%s",
			len(tenant.records[f.candidate.Draft.ID]), len(tenant.results), len(tenant.inspections), tenant.attempts[f.attempt.Ref.ID].State)
	}
}

type commitState struct {
	records       int
	attempt       CommitAttempt
	attempts      int
	inspections   int
	results       int
	tombstones    int
	snapshots     int
	currentExists bool
	current       SnapshotPointer
}

func captureCommitState(f *fixture) commitState {
	f.store.mu.RLock()
	defer f.store.mu.RUnlock()
	tenant := f.store.tenants[f.access.TenantID]
	state := commitState{
		records: len(tenant.records[f.candidate.Draft.ID]), attempt: tenant.attempts[f.attempt.Ref.ID],
		attempts: len(tenant.attempts), inspections: len(tenant.inspections), results: len(tenant.results),
		tombstones: len(tenant.tombstones), snapshots: len(tenant.snapshots), currentExists: tenant.current != nil,
	}
	if tenant.current != nil {
		state.current = *tenant.current
	}
	return state
}

func assertZeroCommitDelta(t *testing.T, f *fixture, before commitState) {
	t.Helper()
	f.store.mu.RLock()
	defer f.store.mu.RUnlock()
	tenant := f.store.tenants[f.access.TenantID]
	afterAttempt := tenant.attempts[f.attempt.Ref.ID]
	if len(tenant.records[f.candidate.Draft.ID]) != before.records || len(tenant.attempts) != before.attempts || afterAttempt != before.attempt || len(tenant.inspections) != before.inspections || len(tenant.results) != before.results || len(tenant.tombstones) != before.tombstones || len(tenant.snapshots) != before.snapshots {
		t.Fatalf("failed commit left authoritative delta: records=%d attempts=%d attempt=%+v inspections=%d results=%d tombstones=%d snapshots=%d",
			len(tenant.records[f.candidate.Draft.ID]), len(tenant.attempts), afterAttempt,
			len(tenant.inspections), len(tenant.results), len(tenant.tombstones), len(tenant.snapshots))
	}
	if (tenant.current != nil) != before.currentExists {
		t.Fatalf("failed commit changed publication currentness: before=%t after=%t", before.currentExists, tenant.current != nil)
	}
	if tenant.current != nil && *tenant.current != before.current {
		t.Fatalf("failed commit changed publication watermark: before=%+v after=%+v", before.current, *tenant.current)
	}
}
