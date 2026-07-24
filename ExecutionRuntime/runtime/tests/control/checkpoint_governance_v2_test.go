package control_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCheckpointGovernanceV2BuilderDerivesBoundedTTLDeterministically(t *testing.T) {
	now := time.Unix(1_780_200_000, 0).UTC()
	request, policy := checkpointControlCreateFixtureV2(t, now)
	left, err := control.BuildCheckpointAttemptBundleV2(request, policy, now)
	if err != nil {
		t.Fatal(err)
	}
	right, err := control.BuildCheckpointAttemptBundleV2(request, policy, now)
	if err != nil {
		t.Fatal(err)
	}
	if left.Attempt.Digest != right.Attempt.Digest || left.Barrier.Digest != right.Barrier.Digest {
		t.Fatal("checkpoint create builder is non-deterministic")
	}
	wantExpiry := now.Add(time.Minute).UnixNano()
	if left.Barrier.ExpiresUnixNano != wantExpiry || left.Attempt.ReconciliationDeadlineUnixNano != now.Add(30*time.Second).UnixNano() {
		t.Fatalf("Owner TTL derivation mismatch: expiry=%d deadline=%d", left.Barrier.ExpiresUnixNano, left.Attempt.ReconciliationDeadlineUnixNano)
	}
}

func TestCheckpointGovernanceV2BuilderRejectsExpectedExpiryAndOverflow(t *testing.T) {
	now := time.Unix(1_780_300_000, 0).UTC()
	request, policy := checkpointControlCreateFixtureV2(t, now)
	request.ExpectedBarrierExpiresUnixNano = now.Add(59 * time.Second).UnixNano()
	if _, err := control.BuildCheckpointAttemptBundleV2(request, policy, now); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("caller-derived Barrier expiry must conflict: %v", err)
	}
	request.ExpectedBarrierExpiresUnixNano = 0
	policy.MaxBarrierTTLUnixNano = int64(^uint64(0) >> 1)
	policy.ProjectionDigest = ""
	digest, _ := policy.DigestV2()
	policy.ProjectionDigest = digest
	if _, err := control.BuildCheckpointAttemptBundleV2(request, policy, now); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("Barrier TTL overflow must fail before build: %v", err)
	}
}

func checkpointControlCreateFixtureV2(t *testing.T, now time.Time) (ports.CreateCheckpointAttemptRequestV2, ports.CheckpointBarrierPolicyCurrentProjectionV2) {
	t.Helper()
	d := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-control", ID: "identity-control", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-control", PlanDigest: d("lineage")}, Instance: core.InstanceRef{ID: "instance-control", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	policyRef := ports.CheckpointBarrierPolicyRefV2{ID: "policy-control", Revision: 1, Digest: d("policy"), SemanticDigest: d("semantic")}
	policy, err := ports.SealCheckpointBarrierPolicyCurrentProjectionV2(ports.CheckpointBarrierPolicyCurrentProjectionV2{Ref: policyRef, MaxBarrierTTLUnixNano: int64(time.Minute), MaxReconciliationTTLUnixNano: int64(30 * time.Second), UnknownAtDeadlineMode: ports.CheckpointUnknownAtDeadlineIndeterminateV2, AbsoluteNotAfterUnixNano: now.Add(3 * time.Minute).UnixNano(), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(2 * time.Minute).UnixNano()}, now)
	if err != nil {
		t.Fatal(err)
	}
	request := ports.CreateCheckpointAttemptRequestV2{AttemptID: "attempt-control", BarrierID: "barrier-control", IdempotencyKey: "create-control", Scope: scope, ScopeDigest: scopeDigest, RunID: "run-control", RunStableIdentityDigest: d("run"), Generation: ports.GenerationArtifactRefV1{ID: "generation-control", Revision: 1, Digest: d("generation"), InputDigest: d("input"), ManifestDigest: d("manifest"), GraphDigest: d("graph"), CatalogDigest: d("catalog")}, GenerationBinding: ports.GenerationBindingAssociationRefV1{ID: "association-control", Revision: 1, Digest: d("association")}, BindingSet: ports.RunBindingSetRefV2{ID: "binding-control", Revision: 1, Digest: d("binding"), SemanticDigest: d("binding-semantic")}, ParticipantSetCertification: ports.CheckpointParticipantSetCertificationRefV2{ID: "participants-control", Revision: 1, Digest: d("participants")}, Workflow: ports.CheckpointWorkflowRefV2{ID: "workflow-control", Revision: 1, Digest: d("workflow"), NotAfter: now.Add(time.Minute).UnixNano()}, BarrierPolicy: policyRef, ExpectedRunRevision: 1, AcquiredDispatchWatermark: 1}
	return request, policy
}
