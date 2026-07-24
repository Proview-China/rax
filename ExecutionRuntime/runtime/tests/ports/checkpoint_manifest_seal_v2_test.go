package ports_test

import (
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCheckpointExternalSHA256DigestV2CanonicalNormalization(t *testing.T) {
	raw := strings.Repeat("a", 64)
	want := core.Digest("sha256:" + raw)
	for _, input := range []string{raw, "sha256:" + raw} {
		got, err := ports.NormalizeCheckpointExternalSHA256DigestV2(input)
		if err != nil || got != want {
			t.Fatalf("canonical external digest normalization drifted: input=%q got=%q err=%v", input, got, err)
		}
	}
	for _, input := range []string{strings.ToUpper(raw), raw[:63], "sha512:" + raw} {
		if _, err := ports.NormalizeCheckpointExternalSHA256DigestV2(input); err == nil {
			t.Fatalf("non-canonical external digest was accepted: %q", input)
		}
	}
}

func TestCheckpointManifestSealOwnerBindingV2IsStructural(t *testing.T) {
	left := checkpointSealOwnerBindingV2()
	right := left
	left.BindingSetID = "a|b"
	left.ComponentID = "c"
	right.BindingSetID = "a"
	right.ComponentID = "b|c"
	if left.Validate() != nil || right.Validate() != nil {
		t.Fatal("delimiter-bearing bounded owner fields should remain structurally representable")
	}
	if left == right {
		t.Fatal("distinct full Owner bindings aliased")
	}
	fields := []func(*ports.CheckpointManifestSealOwnerBindingV2){
		func(v *ports.CheckpointManifestSealOwnerBindingV2) { v.BindingSetID += "-drift" },
		func(v *ports.CheckpointManifestSealOwnerBindingV2) { v.BindingRevision++ },
		func(v *ports.CheckpointManifestSealOwnerBindingV2) { v.ComponentID += "-drift" },
		func(v *ports.CheckpointManifestSealOwnerBindingV2) { v.ManifestDigest += "-drift" },
		func(v *ports.CheckpointManifestSealOwnerBindingV2) { v.ArtifactDigest += "-drift" },
		func(v *ports.CheckpointManifestSealOwnerBindingV2) { v.Capability += "-drift" },
		func(v *ports.CheckpointManifestSealOwnerBindingV2) { v.FactKind += "-drift" },
	}
	base := checkpointSealOwnerBindingV2()
	for index, mutate := range fields {
		changed := base
		mutate(&changed)
		if changed == base {
			t.Fatalf("Owner field %d did not participate in structural identity", index)
		}
	}
}

func TestCheckpointManifestSealExactLookupRejectsOwnerRouteDrift(t *testing.T) {
	lookup := ports.CheckpointExternalExactFactRefV2{
		ContractVersion: ports.CheckpointManifestSealOwnerContractV2,
		SchemaRef:       ports.CheckpointManifestSealExactSchemaV2,
		Owner:           checkpointSealOwnerBindingV2(),
		TenantID:        "tenant-1", ID: "seal-1", Revision: 1,
		Digest: strings.Repeat("b", 64), ScopeDigest: "sha256:" + strings.Repeat("c", 64),
	}
	if err := lookup.Validate(); err != nil {
		t.Fatal(err)
	}
	// The generic external coordinate remains neutral. The specialized Seal ref
	// is the boundary that rejects a wrong Continuity route.
	normalized, err := ports.NormalizeCheckpointExternalSHA256DigestV2(lookup.Digest)
	if err != nil {
		t.Fatal(err)
	}
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	attempt := ports.CheckpointAttemptRefV2{TenantID: "tenant-1", ID: "attempt-1", Revision: 1, Digest: digest("attempt")}
	barrier := ports.CheckpointBarrierLeaseRefV2{TenantID: "tenant-1", ID: "barrier-1", AttemptID: attempt.ID, Revision: 1, Digest: digest("barrier"), ExpiresUnixNano: 1}
	cut := ports.EffectCutRefV2{ID: "cut-1", Revision: 1, Attempt: attempt, RootDigest: digest("root"), Watermark: 1, Digest: digest("cut")}
	ref := ports.CheckpointManifestSealRefV2{ExactLookup: lookup, ID: lookup.ID, Revision: 1, Digest: normalized, ManifestID: "manifest-1", ManifestRevision: 1, ManifestDigest: digest("manifest"), Attempt: attempt, Barrier: barrier, EffectCut: cut, FrozenRefSetDigest: digest("frozen")}
	if err := ref.Validate(); err != nil {
		t.Fatal(err)
	}
	ref.ExactLookup.Owner.ComponentID = "praxis/other"
	if err := ref.Validate(); err == nil {
		t.Fatal("Manifest Seal accepted a non-Continuity exact Owner route")
	}
}

func checkpointSealOwnerBindingV2() ports.CheckpointManifestSealOwnerBindingV2 {
	return ports.CheckpointManifestSealOwnerBindingV2{
		BindingSetID: "binding-set-continuity", BindingRevision: 1,
		ComponentID:    ports.CheckpointManifestSealOwnerComponentV2,
		ManifestDigest: "manifest-digest", ArtifactDigest: "artifact-digest",
		Capability: ports.CheckpointManifestSealOwnerCapabilityV2,
		FactKind:   ports.CheckpointManifestSealOwnerFactKindV2,
	}
}
