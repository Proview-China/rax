package ports_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestRestoreMaterializationCurrentV1SealsCanonicalExactClosure(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	projection := restoreMaterializationFixtureV1(t, now)
	projection.Snapshots[0], projection.Snapshots[1] = projection.Snapshots[1], projection.Snapshots[0]

	sealed, err := ports.SealRestoreMaterializationCurrentProjectionV1(projection, now)
	if err != nil {
		t.Fatal(err)
	}
	if !sealed.ContainsSnapshotV1(projection.Snapshots[0]) || !sealed.ContainsSnapshotV1(projection.Snapshots[1]) {
		t.Fatal("sealed materialization lost an exact Snapshot member")
	}
	clone := sealed.Clone()
	clone.Snapshots[0].ID = "mutated-clone"
	if clone.Snapshots[0] == sealed.Snapshots[0] {
		t.Fatal("materialization clone aliases its Snapshot slice")
	}
	if err := sealed.ValidateCurrent(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreMaterializationCurrentV1RejectsSpliceDriftAndExpiry(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	base := restoreMaterializationFixtureV1(t, now)
	sealed, err := ports.SealRestoreMaterializationCurrentProjectionV1(base, now)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]func(*ports.RestoreMaterializationCurrentProjectionV1){
		"eligibility tenant": func(value *ports.RestoreMaterializationCurrentProjectionV1) {
			value.Eligibility.TenantID = "tenant-other"
		},
		"plan scope": func(value *ports.RestoreMaterializationCurrentProjectionV1) {
			value.RestorePlan.ScopeDigest = string(core.DigestBytes([]byte("other-scope")))
		},
		"manifest attempt": func(value *ports.RestoreMaterializationCurrentProjectionV1) {
			value.ManifestSeal.Attempt.ID = "checkpoint-attempt-other"
		},
		"manifest scope": func(value *ports.RestoreMaterializationCurrentProjectionV1) {
			value.ManifestSeal.ExactLookup.ScopeDigest = string(core.DigestBytes([]byte("other-scope")))
		},
		"snapshot tenant": func(value *ports.RestoreMaterializationCurrentProjectionV1) {
			value.Snapshots[0].TenantID = "tenant-other"
		},
		"context scope": func(value *ports.RestoreMaterializationCurrentProjectionV1) {
			value.ContextFrames[0].ScopeDigest = string(core.DigestBytes([]byte("other-scope")))
		},
		"owner binding": func(value *ports.RestoreMaterializationCurrentProjectionV1) { value.Memory[0].Owner.BindingRevision++ },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := sealed.Clone()
			mutate(&changed)
			if err := changed.ValidateCurrent(now.Add(time.Second)); err == nil {
				t.Fatal("drifted exact closure was accepted")
			}
		})
	}
	if err := sealed.ValidateCurrent(time.Unix(0, sealed.ExpiresUnixNano)); err == nil {
		t.Fatal("expired materialization projection was accepted")
	}
}

func TestRestoreMaterializationCurrentV1OwnerIdentityHasNoDelimiterAlias(t *testing.T) {
	now := time.Unix(1_760_000_000, 0)
	projection := restoreMaterializationFixtureV1(t, now)
	left := restoreMaterializationExternalRefV1(projection.Attempt.TenantID, projection.SourceScopeDigest, "same", "snapshot")
	right := left
	left.Owner.BindingSetID = "a|b"
	left.Owner.ComponentID = "c"
	right.Owner.BindingSetID = "a"
	right.Owner.ComponentID = "b|c"
	projection.Snapshots = []ports.CheckpointExternalExactFactRefV2{left, right}

	sealed, err := ports.SealRestoreMaterializationCurrentProjectionV1(projection, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(sealed.Snapshots) != 2 || !sealed.ContainsSnapshotV1(left) || !sealed.ContainsSnapshotV1(right) {
		t.Fatal("distinct full Owner bindings collided during canonicalization")
	}
}

func restoreMaterializationFixtureV1(t *testing.T, now time.Time) ports.RestoreMaterializationCurrentProjectionV1 {
	t.Helper()
	plan, err := fakes.BuildRestorePlanCurrentFixtureV2("materialization", now)
	if err != nil {
		t.Fatal(err)
	}
	attempt := ports.RestoreAttemptRefV2{TenantID: core.TenantID(plan.RestorePlan.TenantID), ID: "restore-attempt-materialization", Revision: 2, Digest: core.DigestBytes([]byte("restore-attempt-materialization"))}
	eligibility := ports.RestoreEligibilityRefV2{TenantID: attempt.TenantID, ID: "restore-eligibility-materialization", Revision: 1, Digest: core.DigestBytes([]byte("restore-eligibility-materialization")), ExpiresUnixNano: now.Add(2 * time.Minute).UnixNano()}
	external := func(id, kind string) ports.CheckpointExternalExactFactRefV2 {
		return restoreMaterializationExternalRefV1(attempt.TenantID, plan.SourceScopeDigest, id, kind)
	}
	return ports.RestoreMaterializationCurrentProjectionV1{
		Attempt: attempt, Eligibility: eligibility, RestorePlan: plan.RestorePlan,
		Consistency: plan.CheckpointConsistency.Ref, ManifestSeal: plan.ManifestSeal,
		SourceScopeDigest: plan.SourceScopeDigest, Identity: plan.IdentityProposal,
		ContextGeneration: external("context-generation-1", "context-generation"),
		ContextFrames:     []ports.CheckpointExternalExactFactRefV2{external("context-frame-1", "context-frame")},
		Memory:            []ports.CheckpointExternalExactFactRefV2{external("memory-1", "memory")},
		Knowledge:         []ports.CheckpointExternalExactFactRefV2{external("knowledge-1", "knowledge")},
		Snapshots: []ports.CheckpointExternalExactFactRefV2{
			external("snapshot-b", "snapshot"), external("snapshot-a", "snapshot"),
		},
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	}
}

func restoreMaterializationExternalRefV1(tenant core.TenantID, scope core.Digest, id, kind string) ports.CheckpointExternalExactFactRefV2 {
	return ports.CheckpointExternalExactFactRefV2{
		ContractVersion: "praxis.test/" + kind + "/v1",
		SchemaRef:       "praxis.test/" + kind + "-fact/v1",
		Owner: ports.CheckpointManifestSealOwnerBindingV2{
			BindingSetID: "binding-set-" + kind, BindingRevision: 1,
			ComponentID: "praxis/" + kind, ManifestDigest: string(core.DigestBytes([]byte("manifest-" + kind))),
			ArtifactDigest: string(core.DigestBytes([]byte("artifact-" + kind))), Capability: kind + "-reader", FactKind: kind,
		},
		TenantID: string(tenant), ID: id, Revision: 1,
		Digest: string(core.DigestBytes([]byte(id + "-" + kind))), ScopeDigest: string(scope),
	}
}
