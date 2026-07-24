package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCleanupNodeTargetV3ClosedTaggedUnion(t *testing.T) {
	control := exactCleanupRefV3("fixture/control", "control")
	handle := runtimeports.ResourceHandleRefV1{Owner: core.OwnerRef{Domain: "fixture.resources", ID: "owner"}, ID: "resource", Revision: 1, Digest: core.DigestBytes([]byte("resource")), Kind: "fixture/resource", ScopeDigest: core.DigestBytes([]byte("scope")), ExpiresUnixNano: time.Now().Add(time.Hour).UnixNano()}
	constructed, err := contract.SealCleanupNodeTargetV3(contract.CleanupNodeTargetV3{Role: contract.CleanupTargetConstructedV3, Class: contract.CleanupTargetControlV3, ControlInstance: &control, ResourceHandles: []runtimeports.ResourceHandleRefV1{handle}})
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(contract.CleanupNodeTargetV3) contract.CleanupNodeTargetV3{
		"constructed mixed with attempt": func(v contract.CleanupNodeTargetV3) contract.CleanupNodeTargetV3 {
			ref := exactCleanupRefV3("fixture/attempt", "attempt")
			v.AttemptRef = &ref
			v.Digest = ""
			return v
		},
		"control missing resources": func(v contract.CleanupNodeTargetV3) contract.CleanupNodeTargetV3 {
			v.ResourceHandles = nil
			v.Digest = ""
			return v
		},
		"control type-punned as harness": func(v contract.CleanupNodeTargetV3) contract.CleanupNodeTargetV3 {
			v.Class = contract.CleanupTargetHarnessCloseV3
			v.Digest = ""
			return v
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := contract.SealCleanupNodeTargetV3(mutate(constructed)); err == nil {
				t.Fatal("invalid target sealed")
			}
		})
	}
	attempt := exactCleanupRefV3("fixture/attempt", "attempt")
	intent := exactCleanupRefV3("fixture/intent", "intent")
	operation := exactCleanupRefV3("fixture/operation", "operation")
	inspect, err := contract.SealCleanupNodeTargetV3(contract.CleanupNodeTargetV3{Role: contract.CleanupTargetAttemptInspectV3, Class: contract.CleanupTargetSandboxFenceV3, AttemptRef: &attempt, IntentRef: &intent, OperationRef: &operation})
	if err != nil {
		t.Fatal(err)
	}
	if inspect.Role != contract.CleanupTargetAttemptInspectV3 {
		t.Fatal("attempt target role drifted")
	}
	journal := exactCleanupRefV3("praxis.agent-host/journal", "journal")
	if _, err = contract.SealCleanupNodeTargetV3(contract.CleanupNodeTargetV3{Role: contract.CleanupTargetNotConstructedV3, Class: contract.CleanupTargetRuntimeAggregateV3, JournalProof: &journal}); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupDispatchV3RejectsAttemptInspectAndRequiresFreshCurrents(t *testing.T) {
	now := time.Now()
	expires := now.Add(time.Minute).UnixNano()
	digest := func(v string) contract.DigestV1 { return contract.DigestV1(core.DigestBytes([]byte(v))) }
	plan := exactCleanupRefV3("praxis.agent-host/cleanup-plan-v2", "plan")
	closure := contract.HostCleanupClosureRefV2{ClosureID: "closure/id", Revision: 1, HostID: "host", StartID: "start", PlanRef: plan, CoverageDigest: digest("coverage"), Digest: digest("closure")}
	node := exactCleanupRefV3("praxis.agent-host/cleanup-node-v2", "node")
	current := func(id string) runtimeports.OwnerCurrentRefV1 {
		return runtimeports.OwnerCurrentRefV1{Owner: core.OwnerRef{Domain: "fixture.governance", ID: "owner"}, ContractVersion: "1.0.0", ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id)), ExpiresUnixNano: expires}
	}
	attempt := exactCleanupRefV3("fixture/attempt", "attempt")
	intent := exactCleanupRefV3("fixture/intent", "intent")
	operation := exactCleanupRefV3("fixture/operation", "operation")
	target, err := contract.SealCleanupNodeTargetV3(contract.CleanupNodeTargetV3{Role: contract.CleanupTargetAttemptInspectV3, Class: contract.CleanupTargetControlV3, AttemptRef: &attempt, IntentRef: &intent, OperationRef: &operation})
	if err != nil {
		t.Fatal(err)
	}
	inspectEnvelope, err := contract.SealCleanupNodeDispatchEnvelopeV3(contract.CleanupNodeDispatchEnvelopeV3{ClosureRef: closure, PlanRef: plan, NodeRef: node, Target: target, AuthorizationCurrent: current("authorization"), FenceCurrent: current("fence"), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	if err = inspectEnvelope.ValidateOwnerDispatchCurrent(now); err == nil {
		t.Fatal("attempt-inspect target gained dispatch authority")
	}
	journal := exactCleanupRefV3("praxis.agent-host/journal", "journal")
	target, err = contract.SealCleanupNodeTargetV3(contract.CleanupNodeTargetV3{Role: contract.CleanupTargetNotConstructedV3, Class: contract.CleanupTargetControlV3, JournalProof: &journal})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := contract.SealCleanupNodeDispatchEnvelopeV3(contract.CleanupNodeDispatchEnvelopeV3{ClosureRef: closure, PlanRef: plan, NodeRef: node, Target: target, AuthorizationCurrent: current("authorization"), FenceCurrent: current("fence"), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires})
	if err != nil {
		t.Fatal(err)
	}
	if err = envelope.ValidateCurrent(time.Unix(0, expires)); err == nil {
		t.Fatal("expired dispatch remained current")
	}
}

func exactCleanupRefV3(kind, id string) contract.ExactRefV1 {
	return contract.ExactRefV1{Kind: kind, ID: id, Revision: 1, Digest: contract.DigestV1(core.DigestBytes([]byte(kind + "\x00" + id)))}
}
