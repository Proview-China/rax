package control_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestStopSupersedesPendingResume(t *testing.T) {
	t.Parallel()
	resume := control.CommandEnvelope{Kind: control.CommandResume, Target: target(t)}
	stop := control.CommandEnvelope{Kind: control.CommandStopInstance, Target: target(t)}
	if !control.Supersedes(stop, resume) {
		t.Fatal("stop must supersede an unexecuted resume for the same instance")
	}
	if control.Supersedes(resume, stop) {
		t.Fatal("resume must never supersede stop")
	}
}

func TestDenyBindsExactEffectRevision(t *testing.T) {
	t.Parallel()
	approve := control.CommandEnvelope{
		Kind: control.CommandApproveEffect, Target: target(t),
		EffectIntentID: "effect-1", EffectIntentRevision: 2,
	}
	deny := control.CommandEnvelope{
		Kind: control.CommandDenyEffect, Target: target(t),
		EffectIntentID: "effect-1", EffectIntentRevision: 3,
	}
	if control.Supersedes(deny, approve) {
		t.Fatal("deny for a different intent revision cannot rewrite an earlier approval")
	}
	deny.EffectIntentRevision = 2
	if !control.Supersedes(deny, approve) {
		t.Fatal("deny must dominate an undispatched approval for the exact intent revision")
	}
}

func TestSafetyCommandDoesNotCrossInstanceEpoch(t *testing.T) {
	t.Parallel()
	start := control.CommandEnvelope{Kind: control.CommandStart, Target: target(t)}
	fence := control.CommandEnvelope{Kind: control.CommandFence, Target: target(t)}
	fence.Target.Instance.Epoch++
	if control.Supersedes(fence, start) {
		t.Fatal("command dominance is scoped to one exact instance")
	}
}

func target(t *testing.T) core.ExecutionScope {
	t.Helper()
	digest, err := core.DigestJSON("plan")
	if err != nil {
		t.Fatal(err)
	}
	return core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-1", ID: "agent-1", Epoch: 4},
		Lineage:        core.LineageRef{ID: "lineage-1", PlanDigest: digest},
		Instance:       core.InstanceRef{ID: "instance-1", Epoch: 7},
		SandboxLease:   &core.SandboxLeaseRef{ID: "lease-1", Epoch: 2},
		AuthorityEpoch: 3,
	}
}
