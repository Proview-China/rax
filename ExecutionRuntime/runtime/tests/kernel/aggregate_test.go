package kernel_test

import (
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/kernel"
)

func TestConcurrentTransitionLinearizesOnRevision(t *testing.T) {
	t.Parallel()
	aggregate := newAggregate(t)
	leaseEpoch := core.Epoch(2)
	request := kernel.TransitionRequest{
		Preconditions: core.ExecutionPreconditions{
			IdentityEpoch: 4, InstanceEpoch: 7, LeaseEpoch: &leaseEpoch,
			AuthorityEpoch: 3, Revision: 1,
		},
		NextState: core.InstanceState{
			Phase: core.PhaseRunning, Certainty: core.CertaintyConfirmed,
			Cleanup: core.CleanupPending, HasCleanupObligations: true,
		},
	}

	var successes atomic.Int32
	var conflicts atomic.Int32
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := aggregate.Transition(request)
			switch {
			case err == nil:
				successes.Add(1)
			case core.HasReason(err, core.ReasonRevisionConflict):
				conflicts.Add(1)
			default:
				t.Errorf("unexpected transition result: %v", err)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 || conflicts.Load() != 1 {
		t.Fatalf("want one success and one conflict, got success=%d conflict=%d", successes.Load(), conflicts.Load())
	}
	if snapshot := aggregate.Snapshot(); snapshot.Revision != 2 || snapshot.State.Phase != core.PhaseRunning {
		t.Fatalf("unexpected aggregate snapshot: %+v", snapshot)
	}
}

func TestLateObservationCannotMutateCurrentInstance(t *testing.T) {
	t.Parallel()
	aggregate := newAggregate(t)
	before := aggregate.Snapshot()
	lateScope := before.Scope
	lateScope.Instance = core.InstanceRef{ID: "old-instance", Epoch: 6}
	if got := aggregate.ClassifyObservation(kernel.SourceObservation{Scope: lateScope, SourceID: "harness", SourceEpoch: 1, SourceSequence: 10}); got != kernel.ObservationLate {
		t.Fatalf("want late disposition, got %s", got)
	}
	after := aggregate.Snapshot()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("observation must not mutate aggregate: before=%+v after=%+v", before, after)
	}
}

func TestCurrentObservationStillDoesNotAdvanceLifecycle(t *testing.T) {
	t.Parallel()
	aggregate := newAggregate(t)
	before := aggregate.Snapshot()
	if got := aggregate.ClassifyObservation(kernel.SourceObservation{Scope: before.Scope, SourceID: "harness", SourceEpoch: 1, SourceSequence: 11}); got != kernel.ObservationCurrent {
		t.Fatalf("want current disposition, got %s", got)
	}
	if after := aggregate.Snapshot(); after.State != before.State || after.Revision != before.Revision {
		t.Fatalf("current observation requires independent evaluation before transition")
	}
}

func TestPreCommitInstanceCannotBindSandboxLease(t *testing.T) {
	t.Parallel()
	planDigest, err := core.DigestJSON("plan")
	if err != nil {
		t.Fatal(err)
	}
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 4},
		Lineage:        core.LineageRef{ID: "lineage-1", PlanDigest: planDigest},
		Instance:       core.InstanceRef{ID: "instance-1", Epoch: 7},
		SandboxLease:   &core.SandboxLeaseRef{ID: "lease-early", Epoch: 1},
		AuthorityEpoch: 3,
	}
	state := core.InstanceState{Phase: core.PhasePreflighting, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupNotRequired}
	if _, err := kernel.NewAggregate(scope, state); !core.HasReason(err, core.ReasonInvalidState) {
		t.Fatalf("preflight lease binding must be rejected, got %v", err)
	}
}

func TestActivationCommitBindsLeaseAtProvisioningBoundary(t *testing.T) {
	t.Parallel()
	planDigest, err := core.DigestJSON("plan")
	if err != nil {
		t.Fatal(err)
	}
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 4},
		Lineage:  core.LineageRef{ID: "lineage-1", PlanDigest: planDigest},
		Instance: core.InstanceRef{ID: "instance-1", Epoch: 7}, AuthorityEpoch: 3,
	}
	state := core.InstanceState{Phase: core.PhaseActivating, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true}
	aggregate, err := kernel.NewAggregate(scope, state)
	if err != nil {
		t.Fatal(err)
	}
	next := core.InstanceState{Phase: core.PhaseProvisioning, Certainty: core.CertaintyConfirmed, Cleanup: core.CleanupPending, HasCleanupObligations: true}
	request := kernel.ActivationCommitRequest{
		Preconditions: core.ExecutionPreconditions{IdentityEpoch: 4, InstanceEpoch: 7, AuthorityEpoch: 3, Revision: 1},
		SandboxLease:  core.SandboxLeaseRef{ID: "lease-1", Epoch: 1}, NextState: next,
	}
	snapshot, err := aggregate.CommitActivation(request)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Scope.SandboxLease == nil || snapshot.Scope.SandboxLease.ID != "lease-1" || snapshot.State.Phase != core.PhaseProvisioning || snapshot.Revision != 2 {
		t.Fatalf("unexpected activation commit snapshot: %+v", snapshot)
	}
}

func newAggregate(t *testing.T) *kernel.Aggregate {
	t.Helper()
	planDigest, err := core.DigestJSON("plan")
	if err != nil {
		t.Fatal(err)
	}
	scope := core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 4},
		Lineage:        core.LineageRef{ID: "lineage-1", PlanDigest: planDigest},
		Instance:       core.InstanceRef{ID: "instance-1", Epoch: 7},
		SandboxLease:   &core.SandboxLeaseRef{ID: "lease-1", Epoch: 2},
		AuthorityEpoch: 3,
	}
	state := core.InstanceState{
		Phase: core.PhaseReady, Certainty: core.CertaintyConfirmed,
		Cleanup: core.CleanupPending, HasCleanupObligations: true,
	}
	aggregate, err := kernel.NewAggregate(scope, state)
	if err != nil {
		t.Fatal(err)
	}
	return aggregate
}
