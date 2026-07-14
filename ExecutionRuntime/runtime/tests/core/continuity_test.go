package core_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestConsistentCheckpointRequiresEveryRequiredParticipant(t *testing.T) {
	t.Parallel()
	checkpoint := checkpointFixture(t)
	checkpoint.Participants[0].State = core.CheckpointParticipantPrepared
	if err := checkpoint.Validate(); !core.HasReason(err, core.ReasonCheckpointInconsistent) {
		t.Fatalf("required prepared participant must not yield a consistent checkpoint: %v", err)
	}
	checkpoint.Consistency = core.CheckpointPartial
	if err := checkpoint.Validate(); err != nil {
		t.Fatalf("partial checkpoint should preserve diagnostic snapshots: %v", err)
	}
}

func TestCheckpointRejectsImpossibleEffectWatermarks(t *testing.T) {
	t.Parallel()
	checkpoint := checkpointFixture(t)
	checkpoint.Effects = core.EffectWatermarks{Accepted: 2, Dispatched: 3, Settled: 1}
	if err := checkpoint.Validate(); !core.HasReason(err, core.ReasonCheckpointInconsistent) {
		t.Fatalf("dispatch beyond accepted watermark must fail: %v", err)
	}
}

func TestRestoreCreatesNewInstanceAndHigherEpoch(t *testing.T) {
	t.Parallel()
	checkpoint := checkpointFixture(t)
	request := core.RestoreRequest{
		Checkpoint: checkpoint, NewInstance: core.InstanceRef{ID: "instance-2", Epoch: 8},
		CurrentPlanDigest: checkpoint.PlanDigest, CurrentProfileDigest: checkpoint.ProfileDigest,
		CurrentAuthorityEpoch: checkpoint.AuthorityEpoch,
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("compatible restore should validate: %v", err)
	}
	request.NewInstance = checkpoint.Scope.Instance
	if err := request.Validate(); !core.HasReason(err, core.ReasonRestoreIncompatible) {
		t.Fatalf("restore must not revive the checkpoint instance: %v", err)
	}
}

func checkpointFixture(t *testing.T) core.CheckpointSet {
	t.Helper()
	now := time.Now()
	plan := digest(t, "checkpoint-plan")
	profile := digest(t, "checkpoint-profile")
	contextDigest := digest(t, "checkpoint-context")
	snapshot := digest(t, "checkpoint-participant")
	scope := core.ExecutionScope{
		Identity:     core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 4},
		Lineage:      core.LineageRef{ID: "lineage-1", PlanDigest: plan},
		Instance:     core.InstanceRef{ID: "instance-1", Epoch: 7},
		SandboxLease: &core.SandboxLeaseRef{ID: "sandbox-1", Epoch: 2}, AuthorityEpoch: 3,
	}
	return core.CheckpointSet{
		ID: "checkpoint-1", Epoch: 1, BarrierID: "barrier-1", Scope: scope,
		PlanDigest: plan, ProfileDigest: profile, ContextDigest: contextDigest, AuthorityEpoch: 3,
		Effects:        core.EffectWatermarks{Accepted: 4, Dispatched: 3, Settled: 2, Remote: 1},
		EventWatermark: core.TimelinePoint{LedgerScope: "run-1", LedgerSequence: 10, SourceID: "runtime", SourceEpoch: 1, SourceSequence: 10, EventID: "event-10", ObservedAt: now, IngestedAt: now},
		Participants:   []core.CheckpointParticipantSnapshot{{ComponentID: "harness", ComponentKind: "harness", Required: true, State: core.CheckpointParticipantCommitted, SnapshotRef: "snapshot/harness/1", SnapshotDigest: snapshot}},
		Consistency:    core.CheckpointConsistent, CreatedAt: now,
	}
}
