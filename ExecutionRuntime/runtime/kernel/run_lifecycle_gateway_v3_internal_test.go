package kernel

import (
	"context"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type runLifecycleFactReadSpyV3 struct {
	control.RunSettlementFactPortV2
	reads int
}

func (s *runLifecycleFactReadSpyV3) InspectRun(context.Context, core.ExecutionScope, core.AgentRunID) (core.AgentRunRecord, error) {
	s.reads++
	return core.AgentRunRecord{}, core.NewError(core.ErrorNotFound, core.ReasonRunConflict, "unexpected Run read")
}

type planAdmissionNoopV3 struct {
	ports.RunSettlementPlanAdmissionPortV3
}

func validRunLifecycleInspectScopeV3() core.ExecutionScope {
	return core.ExecutionScope{
		Identity:       core.AgentIdentityRef{TenantID: "tenant-lifecycle-input", ID: "identity-lifecycle-input", Epoch: 1},
		Lineage:        core.LineageRef{ID: "lineage-lifecycle-input", PlanDigest: core.DigestBytes([]byte("lineage-lifecycle-input"))},
		Instance:       core.InstanceRef{ID: "instance-lifecycle-input", Epoch: 1},
		AuthorityEpoch: 1,
	}
}

func TestRunLifecycleV3InspectDependenciesFailBeforeBackend(t *testing.T) {
	facts := &runLifecycleFactReadSpyV3{}
	gateway := RunSettlementGatewayV2{Facts: facts, PlanAdmissions: &planAdmissionNoopV3{}}
	scope := validRunLifecycleInspectScopeV3()
	if _, err := gateway.InspectRunLifecycleV3(context.Background(), scope, "run-lifecycle-input"); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("unexpected lifecycle dependency error: %v", err)
	}
	if _, err := gateway.InspectRunTerminationV3(context.Background(), ports.RunTerminationRequestV3{ExecutionScope: scope, RunID: "run-lifecycle-input"}); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("unexpected termination dependency error: %v", err)
	}
	if facts.reads != 0 {
		t.Fatalf("missing lifecycle dependency caused backend reads: %d", facts.reads)
	}
}
