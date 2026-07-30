package control_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestModelDispatchControlCurrentV1RejectsCancelAndDrift(t *testing.T) {
	now := time.Unix(1_300_000, 0)
	lease := &core.SandboxLeaseRef{ID: "lease-control", Epoch: 1}
	scope := core.ExecutionScope{
		Identity: core.AgentIdentityRef{TenantID: "tenant-control", ID: "agent-control", Epoch: 1},
		Lineage:  core.LineageRef{ID: "lineage-control", PlanDigest: controlDigestV1("plan")},
		Instance: core.InstanceRef{ID: "instance-control", Epoch: 1}, SandboxLease: lease, AuthorityEpoch: 1,
	}
	scopeDigest, _ := ports.ExecutionScopeDigestV2(scope)
	operation := ports.OperationSubjectV3{
		Kind: ports.OperationScopeRunV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, RunID: "run-control",
		SubjectRevision: 1, CurrentProjectionRef: "run-current-control", CurrentProjectionRevision: 1, CurrentProjectionDigest: controlDigestV1("run-current"),
	}
	operationDigest, _ := operation.DigestV3()
	run := core.AgentRunRecord{ID: operation.RunID, Scope: scope, Status: core.RunRunning, Revision: 4, StartedAt: now.Add(-time.Minute)}
	projection, err := control.SealModelDispatchControlCurrentProjectionV1(control.ModelDispatchControlCurrentProjectionV1{
		OperationDigest: operationDigest, EffectID: "effect-control", RunID: run.ID, ExecutionScopeDigest: scopeDigest,
		RunRevision: run.Revision, DesiredStateRevision: 3, LastCommandID: "command-control",
		State: control.ModelDispatchControlDispatchableV1, WatermarkDigest: controlDigestV1("watermark"),
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ValidateCurrent(operation, "effect-control", run, now); err != nil {
		t.Fatal(err)
	}
	cancelled := projection
	cancelled.State = control.ModelDispatchControlCancelRequestedV1
	cancelled, err = control.SealModelDispatchControlCurrentProjectionV1(cancelled)
	if err != nil {
		t.Fatal(err)
	}
	if err := cancelled.ValidateCurrent(operation, "effect-control", run, now); err == nil {
		t.Fatal("cancel-requested current projection remained dispatchable")
	}
	if err := projection.ValidateCurrent(operation, "effect-control", run, now.Add(2*time.Second)); err == nil {
		t.Fatal("expired dispatch control remained current")
	}
}

func controlDigestV1(value string) core.Digest {
	digest, _ := core.DigestJSON(value)
	return digest
}
