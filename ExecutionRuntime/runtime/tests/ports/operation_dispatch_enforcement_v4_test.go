package ports_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationDispatchEnforcementV4ContractVersionAndPhaseShape(t *testing.T) {
	if ports.OperationDispatchEnforcementContractVersionV4 != "4.1.0" {
		t.Fatalf("unexpected enforcement contract version %q", ports.OperationDispatchEnforcementContractVersionV4)
	}
	prepare := ports.EnforceCurrentOperationDispatchRequestV4{Phase: ports.OperationDispatchEnforcementPrepareV4}
	prepare.ExpectedJournalRevision = 1
	if err := prepare.Validate(); err == nil {
		t.Fatal("prepare accepted an existing journal watermark")
	}
	execute := ports.EnforceCurrentOperationDispatchRequestV4{Phase: ports.OperationDispatchEnforcementExecuteV4}
	if err := execute.Validate(); err == nil {
		t.Fatal("execute accepted missing prepare and prepared attempt refs")
	}
}

func TestOperationDispatchSandboxCurrentProjectionV4RejectsRootTTLAboveBoundFact(t *testing.T) {
	now := time.Unix(900_000, 0)
	scope := core.ExecutionScope{
		Identity:     core.AgentIdentityRef{TenantID: "tenant-ports-enforcement", ID: "identity-ports-enforcement", Epoch: 1},
		Lineage:      core.LineageRef{ID: "lineage-ports-enforcement", PlanDigest: core.DigestBytes([]byte("lineage"))},
		Instance:     core.InstanceRef{ID: "instance-ports-enforcement", Epoch: 1},
		SandboxLease: &core.SandboxLeaseRef{ID: "lease-ports-enforcement", Epoch: 1}, AuthorityEpoch: 1,
	}
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := ports.OperationSubjectV3{
		Kind: ports.OperationScopeRunV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, RunID: "run-ports-enforcement",
		SubjectRevision: 1, CurrentProjectionRef: "current-ports-enforcement", CurrentProjectionRevision: 1,
		CurrentProjectionDigest: core.DigestBytes([]byte("current")),
	}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	short := now.Add(time.Second).UnixNano()
	long := now.Add(2 * time.Second).UnixNano()
	ref := func(id string, expires int64) ports.OperationDispatchSandboxFactRefV4 {
		return ports.OperationDispatchSandboxFactRefV4{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(id)), ExpiresUnixNano: expires}
	}
	provider := ports.ProviderBindingRefV2{BindingSetID: "binding-ports-enforcement", BindingSetRevision: 1, ComponentID: "custom/provider", ManifestDigest: core.DigestBytes([]byte("manifest")), ArtifactDigest: core.DigestBytes([]byte("artifact")), Capability: "custom/execute"}
	projection := ports.OperationDispatchSandboxCurrentProjectionV4{
		Operation: operation, OperationDigest: operationDigest, EffectID: "effect-ports-enforcement", IntentRevision: 1,
		IntentDigest: core.DigestBytes([]byte("intent")), AttemptID: "attempt-ports-enforcement", Attempt: ref("attempt-ports-enforcement", short), Reservation: ref("reservation-ports-enforcement", long),
		SandboxLease: *scope.SandboxLease,
		RuntimeLease: ports.OperationDispatchRuntimeLeaseBindingV4{Ref: ref("runtime-lease-ports-enforcement", long), Lease: *scope.SandboxLease, Instance: scope.Instance, FenceEpoch: 1, ScopeDigest: scopeDigest, ObservedRevision: 1},
		Generation:   ports.GenerationBindingAssociationRefV1{ID: "generation-ports-enforcement", Revision: 1, Digest: core.DigestBytes([]byte("generation"))},
		Placement:    ref("placement-ports-enforcement", long), Backend: ref("backend-ports-enforcement", long), Slot: ref("slot-ports-enforcement", long),
		ProviderBinding: provider, Current: true, ProjectionRevision: 1, ExpiresUnixNano: long,
	}
	if _, err := ports.SealOperationDispatchSandboxCurrentProjectionV4(projection); err == nil {
		t.Fatal("projection root TTL exceeded its exact Attempt Fact TTL")
	}
}
