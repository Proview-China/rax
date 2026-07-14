package fakes_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/admission"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
)

func TestActivationCommitLinearizesAttemptAndIdentityLease(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 20, 0, 0, time.UTC)
	store, attempt, lease := preparedActivationStore(t, now)
	request := commitRequest(attempt, lease)

	var successes atomic.Int32
	var conflicts atomic.Int32
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.CommitActivation(context.Background(), request)
			switch {
			case err == nil:
				successes.Add(1)
			case core.HasReason(err, core.ReasonRevisionConflict):
				conflicts.Add(1)
			default:
				t.Errorf("unexpected commit result: %v", err)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 || conflicts.Load() != 1 {
		t.Fatalf("want one logical commit and one CAS conflict, got success=%d conflict=%d", successes.Load(), conflicts.Load())
	}

	committed, err := store.InspectActivationAttempt(context.Background(), attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.InspectIdentityLease(context.Background(), attempt.Scope.Identity.TenantID, attempt.Scope.Identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Stage != admission.StageCommitted || committed.Scope.SandboxLease == nil || active.State != control.IdentityLeaseActive || committed.IdentityLeaseRevision != active.Revision {
		t.Fatalf("logical commit split facts: attempt=%+v lease=%+v", committed, active)
	}

	decision, err := admission.PlanRecovery(committed, now)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != admission.ActionActivateSandbox {
		t.Fatalf("lost commit response must resume after commit, got %+v", decision)
	}
	if _, err := store.ReleaseIdentityLease(context.Background(), control.EndIdentityLeaseRequest{LeaseID: active.ID, ExpectedRevision: active.Revision, Reason: "unsafe direct release"}); !core.HasReason(err, core.ReasonIdentityLeaseStateInvalid) {
		t.Fatalf("active identity authority must require revoke before release: %v", err)
	}
}

func TestEvidenceFailureLeavesActivationCommitWithNoPartialWrite(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 21, 0, 0, time.UTC)
	store, attempt, lease := preparedActivationStore(t, now)
	store.SetEvidenceAvailable(false)
	if _, err := store.CommitActivation(context.Background(), commitRequest(attempt, lease)); !core.HasReason(err, core.ReasonEvidenceUnavailable) {
		t.Fatalf("commit without durable journal must fail closed: %v", err)
	}
	store.SetEvidenceAvailable(true)
	after, err := store.InspectActivationAttempt(context.Background(), attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	leaseAfter, err := store.InspectIdentityLease(context.Background(), attempt.Scope.Identity.TenantID, attempt.Scope.Identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Stage != admission.StageSandboxReserved || leaseAfter.State != control.IdentityLeaseReserved || after.Revision != attempt.Revision || leaseAfter.Revision != lease.Revision {
		t.Fatalf("failed commit partially advanced facts: attempt=%+v lease=%+v", after, leaseAfter)
	}
}

func TestExpiredSnapshotCannotCommitAndDoesNotActivateLease(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 14, 7, 22, 0, 0, time.UTC)
	current := now
	store, attempt, lease := preparedActivationStoreWithClock(t, &current)
	current = now.Add(2 * time.Hour)
	if _, err := store.CommitActivation(context.Background(), commitRequest(attempt, lease)); !core.HasReason(err, core.ReasonActivationFactDrift) {
		t.Fatalf("expired snapshot must reject activation commit: %v", err)
	}
	current = now
	leaseAfter, err := store.InspectIdentityLease(context.Background(), attempt.Scope.Identity.TenantID, attempt.Scope.Identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if leaseAfter.State != control.IdentityLeaseReserved {
		t.Fatalf("snapshot drift activated identity authority: %+v", leaseAfter)
	}
}

func preparedActivationStore(t *testing.T, now time.Time) (*fakes.FactStore, admission.ActivationAttempt, control.IdentityExecutionLease) {
	t.Helper()
	current := now
	return preparedActivationStoreWithClock(t, &current)
}

func preparedActivationStoreWithClock(t *testing.T, now *time.Time) (*fakes.FactStore, admission.ActivationAttempt, control.IdentityExecutionLease) {
	t.Helper()
	store := fakes.NewFactStore(func() time.Time { return *now })
	plan := digest(t, "plan")
	requirement := digest(t, "sandbox-requirement")
	attempt := admission.ActivationAttempt{
		ID: "attempt-activation-1",
		Scope: core.ExecutionScope{
			Identity:       core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-activation", Epoch: 1},
			Lineage:        core.LineageRef{ID: "lineage-activation", PlanDigest: plan},
			Instance:       core.InstanceRef{ID: "instance-activation", Epoch: 9},
			AuthorityEpoch: 3,
		},
		ExpectedIdentityEpoch: 0,
		RequirementDigest:     requirement,
		Stage:                 admission.StageProposed,
		Recovery:              admission.RecoveryNormal,
		Budget:                admission.ActivationOperation{State: admission.OperationNotStarted},
		SandboxReservation:    admission.ActivationOperation{State: admission.OperationNotStarted},
		SandboxActivation:     admission.ActivationOperation{State: admission.OperationNotStarted},
		Revision:              1,
		CreatedAt:             now.Add(-time.Minute),
		UpdatedAt:             now.Add(-time.Minute),
	}
	var err error
	attempt, err = store.CreateActivationAttempt(context.Background(), attempt)
	if err != nil {
		t.Fatal(err)
	}
	attempt = advance(t, store, attempt, admission.StagePreflighting, func(*admission.ActivationAttempt) {})
	attempt = advance(t, store, attempt, admission.StagePreflightPassed, func(*admission.ActivationAttempt) {})
	attempt = advance(t, store, attempt, admission.StageSnapshotFrozen, func(next *admission.ActivationAttempt) {
		next.Snapshot = &admission.ActivationSnapshot{
			Digest: digest(t, "snapshot"), AuthorityEpoch: 3,
			EntitlementDigest: digest(t, "entitlement"), RouteDigest: digest(t, "route"),
			CapabilityEvidenceDigest: digest(t, "capability"), PolicyDigest: digest(t, "policy"),
			BudgetPolicyDigest: digest(t, "budget-policy"), SandboxRequirementDigest: requirement,
			EvidenceExpiresAt: now.Add(time.Hour),
		}
	})
	lease, err := store.ReserveIdentityLease(context.Background(), control.ReserveIdentityLeaseRequest{
		TenantID: attempt.Scope.Identity.TenantID, IdentityID: attempt.Scope.Identity.ID,
		ExpectedIdentityEpoch: attempt.ExpectedIdentityEpoch, Lineage: attempt.Scope.Lineage,
		ActivationAttemptID: attempt.ID, AuthorityEpoch: attempt.Scope.AuthorityEpoch, ExpiresAt: now.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt = advance(t, store, attempt, admission.StageIdentityLeaseReserved, func(next *admission.ActivationAttempt) {
		next.IdentityLeaseID = lease.ID
		next.IdentityLeaseState = lease.State
		next.IdentityLeaseRevision = lease.Revision
	})
	attempt = advance(t, store, attempt, admission.StageBudgetResolved, func(next *admission.ActivationAttempt) {
		next.Budget = admission.ActivationOperation{State: admission.OperationNotRequired}
	})
	attempt = advance(t, store, attempt, admission.StageSandboxReserved, func(next *admission.ActivationAttempt) {
		next.SandboxReservation = admission.ActivationOperation{
			State: admission.OperationConfirmedApplied, IntentID: "effect-sandbox-reserve",
			Reference: "sandbox-activation-1", EvidenceDigest: digest(t, "sandbox-reserved"),
		}
	})
	return store, attempt, lease
}

func advance(t *testing.T, store *fakes.FactStore, current admission.ActivationAttempt, stage admission.ActivationStage, mutate func(*admission.ActivationAttempt)) admission.ActivationAttempt {
	t.Helper()
	next := current
	next.Stage = stage
	next.Revision++
	next.UpdatedAt = next.UpdatedAt.Add(time.Second)
	mutate(&next)
	updated, err := store.CompareAndSwapActivation(context.Background(), next, admission.TransitionContext{})
	if err != nil {
		t.Fatalf("advance to %s: %v", stage, err)
	}
	return updated
}

func commitRequest(attempt admission.ActivationAttempt, lease control.IdentityExecutionLease) admission.ActivationCommitRequest {
	return admission.ActivationCommitRequest{
		AttemptID: attempt.ID, ExpectedAttemptRevision: attempt.Revision,
		IdentityLeaseID: lease.ID, ExpectedIdentityLeaseRevision: lease.Revision,
		SandboxLease: core.SandboxLeaseRef{ID: "sandbox-activation-1", Epoch: 1}, AuthorityEpoch: attempt.Scope.AuthorityEpoch,
	}
}

func digest(t *testing.T, value string) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
