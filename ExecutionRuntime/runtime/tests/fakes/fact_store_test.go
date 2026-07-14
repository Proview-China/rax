package fakes_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
)

func TestCommandLogicalCommitAndIdempotentReplay(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 6, 40, 0, 0, time.UTC)
	store, scope := newCommandStore(t, now)
	intent := commandIntent(t, now, scope, control.CommandStart, control.DesiredRunning, 1, "cmd-start", "idem-start", "start")
	first, err := store.AcceptCommand(context.Background(), intent)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AcceptCommand(context.Background(), intent)
	if err != nil {
		t.Fatal(err)
	}
	if first.Record.Revision != 2 || first.DesiredState.Revision != 2 || first.Outbox.Revision != 2 || second.Record.Revision != first.Record.Revision {
		t.Fatalf("command, desired state and outbox must share one revision: first=%+v second=%+v", first, second)
	}
	commands, _ := store.ListCommands(context.Background(), scope)
	outbox, _ := store.ListOutbox(context.Background(), scope)
	if len(commands) != 1 || len(outbox) != 1 {
		t.Fatalf("idempotent replay must not duplicate facts: commands=%d outbox=%d", len(commands), len(outbox))
	}
	dispatched, err := store.MarkOutboxDispatched(context.Background(), scope, first.Record.Envelope.ID, first.Record.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if !dispatched.Dispatched {
		t.Fatal("outbox dispatch acknowledgement was not persisted")
	}
}

func TestIdempotencyPayloadMismatchIsConflict(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 6, 41, 0, 0, time.UTC)
	store, scope := newCommandStore(t, now)
	first := commandIntent(t, now, scope, control.CommandStart, control.DesiredRunning, 1, "cmd-1", "same-key", "payload-a")
	if _, err := store.AcceptCommand(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := commandIntent(t, now, scope, control.CommandStart, control.DesiredRunning, 1, "cmd-2", "same-key", "payload-b")
	if _, err := store.AcceptCommand(context.Background(), second); !core.HasReason(err, core.ReasonIdempotencyPayloadMismatch) {
		t.Fatalf("want idempotency payload mismatch, got %v", err)
	}
}

func TestEvidenceFailureLeavesNoPartialCommandCommit(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 6, 42, 0, 0, time.UTC)
	store, scope := newCommandStore(t, now)
	store.SetEvidenceAvailable(false)
	intent := commandIntent(t, now, scope, control.CommandStart, control.DesiredRunning, 1, "cmd-start", "idem-start", "start")
	if _, err := store.AcceptCommand(context.Background(), intent); !core.HasReason(err, core.ReasonEvidenceUnavailable) {
		t.Fatalf("want evidence unavailable, got %v", err)
	}
	desired, _ := store.ReadDesiredState(context.Background(), scope)
	commands, _ := store.ListCommands(context.Background(), scope)
	outbox, _ := store.ListOutbox(context.Background(), scope)
	if desired.Revision != 1 || desired.Desired != control.DesiredStopped || len(commands) != 0 || len(outbox) != 0 {
		t.Fatalf("failed logical commit leaked partial facts: desired=%+v commands=%d outbox=%d", desired, len(commands), len(outbox))
	}
}

func TestConcurrentCommandsLinearizeOnDesiredRevision(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 6, 43, 0, 0, time.UTC)
	store, scope := newCommandStore(t, now)
	intents := []control.CommandIntent{
		commandIntent(t, now, scope, control.CommandStart, control.DesiredRunning, 1, "cmd-a", "idem-a", "a"),
		commandIntent(t, now, scope, control.CommandStart, control.DesiredRunning, 1, "cmd-b", "idem-b", "b"),
	}
	var successes atomic.Int32
	var conflicts atomic.Int32
	var wait sync.WaitGroup
	for _, intent := range intents {
		intent := intent
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.AcceptCommand(context.Background(), intent)
			switch {
			case err == nil:
				successes.Add(1)
			case core.HasReason(err, core.ReasonRevisionConflict):
				conflicts.Add(1)
			default:
				t.Errorf("unexpected result: %v", err)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 || conflicts.Load() != 1 {
		t.Fatalf("want one accepted and one revision conflict, got accepted=%d conflict=%d", successes.Load(), conflicts.Load())
	}
}

func TestStopSupersedesStartAndBlocksRestartOfSameInstance(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 6, 44, 0, 0, time.UTC)
	store, scope := newCommandStore(t, now)
	start := commandIntent(t, now, scope, control.CommandStart, control.DesiredRunning, 1, "cmd-start", "idem-start", "start")
	if _, err := store.AcceptCommand(context.Background(), start); err != nil {
		t.Fatal(err)
	}
	stop := commandIntent(t, now, scope, control.CommandStopInstance, control.DesiredStopped, 2, "cmd-stop", "idem-stop", "stop")
	if _, err := store.AcceptCommand(context.Background(), stop); err != nil {
		t.Fatal(err)
	}
	commands, _ := store.ListCommands(context.Background(), scope)
	if len(commands) != 2 || commands[0].Status != control.CommandSuperseded || commands[1].Status != control.CommandAccepted {
		t.Fatalf("unexpected command dominance projection: %+v", commands)
	}
	restart := commandIntent(t, now, scope, control.CommandStart, control.DesiredRunning, 3, "cmd-restart", "idem-restart", "restart")
	if _, err := store.AcceptCommand(context.Background(), restart); !core.HasReason(err, core.ReasonCommandDominated) {
		t.Fatalf("stopped instance must not restart, got %v", err)
	}
}

func TestNonLifecycleCommandCannotChangeDesiredState(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 6, 45, 0, 0, time.UTC)
	store, scope := newCommandStore(t, now)
	intent := commandIntent(t, now, scope, control.CommandProvideInput, control.DesiredRunning, 1, "cmd-input", "idem-input", "input")
	if _, err := store.AcceptCommand(context.Background(), intent); !core.HasReason(err, core.ReasonInvalidState) {
		t.Fatalf("provide input cannot change desired lifecycle, got %v", err)
	}
}

func TestIdentityLeaseAllowsOnlyOneConcurrentHolder(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 6, 46, 0, 0, time.UTC)
	store := fakes.NewFactStore(func() time.Time { return now })
	requests := []control.ReserveIdentityLeaseRequest{
		leaseRequest(t, now, "attempt-a", 0),
		leaseRequest(t, now, "attempt-b", 0),
	}
	var successes atomic.Int32
	var conflicts atomic.Int32
	var wait sync.WaitGroup
	for _, request := range requests {
		request := request
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.ReserveIdentityLease(context.Background(), request)
			switch {
			case err == nil:
				successes.Add(1)
			case core.HasReason(err, core.ReasonIdentityLeaseConflict):
				conflicts.Add(1)
			default:
				t.Errorf("unexpected reserve result: %v", err)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 || conflicts.Load() != 1 {
		t.Fatalf("want one holder and one conflict, got holder=%d conflict=%d", successes.Load(), conflicts.Load())
	}
}

func TestReservedIdentityLeaseRevisionCASAndRelease(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 6, 47, 0, 0, time.UTC)
	store := fakes.NewFactStore(func() time.Time { return now })
	reserved, err := store.ReserveIdentityLease(context.Background(), leaseRequest(t, now, "attempt-a", 0))
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := store.RevokeIdentityLease(context.Background(), control.EndIdentityLeaseRequest{LeaseID: reserved.ID, ExpectedRevision: 1, Reason: "activation aborted"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RevokeIdentityLease(context.Background(), control.EndIdentityLeaseRequest{LeaseID: reserved.ID, ExpectedRevision: 1, Reason: "stale retry"}); !core.HasReason(err, core.ReasonStaleLeaseRevision) {
		t.Fatalf("stale revoke revision must conflict, got %v", err)
	}
	released, err := store.ReleaseIdentityLease(context.Background(), control.EndIdentityLeaseRequest{LeaseID: revoked.ID, ExpectedRevision: 2, Reason: "cleanup complete"})
	if err != nil {
		t.Fatal(err)
	}
	if released.State != control.IdentityLeaseReleased || released.Revision != 3 {
		t.Fatalf("unexpected released lease: %+v", released)
	}
}

func TestExpiredLeaseAdvancesEpochBeforeReplacement(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 6, 48, 0, 0, time.UTC)
	store := fakes.NewFactStore(func() time.Time { return now })
	first, err := store.ReserveIdentityLease(context.Background(), leaseRequest(t, now, "attempt-a", 0))
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	expired, err := store.InspectIdentityLease(context.Background(), "tenant-1", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if expired.State != control.IdentityLeaseExpired || expired.Revision != first.Revision+1 {
		t.Fatalf("unexpected expired lease: %+v", expired)
	}
	request := leaseRequest(t, now, "attempt-b", expired.Identity.Epoch)
	second, err := store.ReserveIdentityLease(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if second.Identity.Epoch != first.Identity.Epoch+1 || second.ID == first.ID {
		t.Fatalf("replacement must advance identity epoch: first=%+v second=%+v", first, second)
	}
}

func newCommandStore(t *testing.T, now time.Time) (*fakes.FactStore, core.ExecutionScope) {
	t.Helper()
	store := fakes.NewFactStore(func() time.Time { return now })
	scope := executionScope(t)
	_, err := store.CreateDesiredState(context.Background(), control.DesiredStateSnapshot{Scope: scope, Desired: control.DesiredStopped, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	return store, scope
}

func commandIntent(t *testing.T, now time.Time, scope core.ExecutionScope, kind control.CommandKind, desired control.DesiredExecutionState, revision core.Revision, id, idempotencyKey, payload string) control.CommandIntent {
	t.Helper()
	digest, err := core.DigestJSON(payload)
	if err != nil {
		t.Fatal(err)
	}
	leaseEpoch := scope.SandboxLease.Epoch
	return control.CommandIntent{
		Envelope: control.CommandEnvelope{
			ID: id, Kind: kind, Target: scope, Actor: "user-1", AuthorityRef: "authority-3",
			CanonicalPayloadDigest: digest,
			Preconditions: core.ExecutionPreconditions{
				IdentityEpoch: scope.Identity.Epoch, InstanceEpoch: scope.Instance.Epoch,
				LeaseEpoch: &leaseEpoch, AuthorityEpoch: scope.AuthorityEpoch, Revision: revision,
			},
			IdempotencyKey: idempotencyKey, SubmittedAt: now, ExpiresAt: now.Add(time.Hour),
		},
		Mutation: control.DesiredStateMutation{Desired: desired},
	}
}

func executionScope(t *testing.T) core.ExecutionScope {
	t.Helper()
	planDigest, err := core.DigestJSON("plan")
	if err != nil {
		t.Fatal(err)
	}
	return core.ExecutionScope{
		Identity:     core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 4},
		Lineage:      core.LineageRef{ID: "lineage-1", PlanDigest: planDigest},
		Instance:     core.InstanceRef{ID: "instance-1", Epoch: 7},
		SandboxLease: &core.SandboxLeaseRef{ID: "lease-1", Epoch: 2}, AuthorityEpoch: 3,
	}
}

func leaseRequest(t *testing.T, now time.Time, attempt string, expectedEpoch core.Epoch) control.ReserveIdentityLeaseRequest {
	t.Helper()
	planDigest, err := core.DigestJSON("plan")
	if err != nil {
		t.Fatal(err)
	}
	return control.ReserveIdentityLeaseRequest{
		TenantID: "tenant-1", IdentityID: "agent-1", ExpectedIdentityEpoch: expectedEpoch,
		Lineage:             core.LineageRef{ID: "lineage-1", PlanDigest: planDigest},
		ActivationAttemptID: attempt, AuthorityEpoch: 3, ExpiresAt: now.Add(time.Hour),
	}
}
