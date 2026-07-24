package ports_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestGenerationBindingAssociationV1CanonicalAndNotBindingAuthority(t *testing.T) {
	t.Parallel()
	now := time.Unix(80_000, 0)
	candidate := generationBindingCandidatePortV1(t, now)
	if err := candidate.Validate(); err != nil {
		t.Fatal(err)
	}

	reordered := candidate
	reordered.Generation.ComponentManifests = append([]ports.GenerationComponentManifestRefV1{}, candidate.Generation.ComponentManifests...)
	// A second component proves canonical ordering and semantic tamper checks.
	reordered.Generation.ComponentManifests = append(reordered.Generation.ComponentManifests, ports.GenerationComponentManifestRefV1{
		ComponentID: "vendor/second", ManifestDigest: generationPortDigestV1(t, "manifest-second"), ArtifactDigest: generationPortDigestV1(t, "artifact-second"),
	})
	sealedGeneration, err := ports.SealGenerationCurrentProjectionV1(reordered.Generation)
	if err != nil {
		t.Fatal(err)
	}
	reordered.Generation = sealedGeneration
	reordered.Binding.ComponentManifestSetDigest = ports.GenerationComponentManifestSetDigestV1(sealedGeneration.ComponentManifests)
	reordered.Binding, err = ports.SealGenerationBindingSetCurrentProjectionV1(reordered.Binding)
	if err != nil {
		t.Fatal(err)
	}
	reordered, err = ports.SealGenerationBindingAssociationCandidateV1(reordered)
	if err != nil {
		t.Fatal(err)
	}
	unsorted := reordered
	unsorted.Generation.ComponentManifests[0], unsorted.Generation.ComponentManifests[1] = unsorted.Generation.ComponentManifests[1], unsorted.Generation.ComponentManifests[0]
	if err := unsorted.Validate(); err == nil {
		t.Fatalf("unsorted component set must fail closed: %v", err)
	}

	tampered := candidate
	tampered.Generation.Generation.GraphDigest = generationPortDigestV1(t, "other-graph")
	if err := tampered.Validate(); err == nil {
		t.Fatalf("generation graph tamper retained old candidate identity: %v", err)
	}
}

func TestGenerationBindingAssociationV1TransitionIsMonotonic(t *testing.T) {
	t.Parallel()
	now := time.Unix(81_000, 0)
	candidate := generationBindingCandidatePortV1(t, now)
	fact, err := ports.SealGenerationBindingAssociationFactV1(ports.GenerationBindingAssociationFactV1{
		ID: candidate.AssociationID, Revision: 1, State: ports.GenerationBindingAssociationActiveV1,
		Candidate: candidate, CandidateDigest: candidate.Digest,
		CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := ports.NextGenerationBindingAssociationStateV1(fact, ports.GenerationBindingAssociationRevokedV1, core.ReasonBindingDrift, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := ports.ValidateGenerationBindingAssociationTransitionV1(fact, revoked, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	changed := revoked
	changed.Candidate.Generation.Generation.InputDigest = generationPortDigestV1(t, "changed")
	changed, _ = ports.SealGenerationBindingAssociationFactV1(changed)
	if err := ports.ValidateGenerationBindingAssociationTransitionV1(fact, changed, now.Add(time.Second)); err == nil {
		t.Fatal("CAS must not replace immutable generation input")
	}
	if _, err := ports.NextGenerationBindingAssociationStateV1(fact, ports.GenerationBindingAssociationExpiredV1, core.ReasonBindingExpired, now.Add(-time.Second)); err == nil {
		t.Fatal("expiry before the exact TTL boundary must fail")
	}
}

func generationBindingCandidatePortV1(t *testing.T, now time.Time) ports.GenerationBindingAssociationCandidateV1 {
	t.Helper()
	component := ports.GenerationComponentManifestRefV1{ComponentID: "vendor/component", ManifestDigest: generationPortDigestV1(t, "manifest"), ArtifactDigest: generationPortDigestV1(t, "artifact")}
	generation, err := ports.SealGenerationCurrentProjectionV1(ports.GenerationCurrentProjectionV1{
		Generation:         ports.GenerationArtifactRefV1{ID: "generation-1", Revision: 1, Digest: generationPortDigestV1(t, "generation"), InputDigest: generationPortDigestV1(t, "input"), ManifestDigest: generationPortDigestV1(t, "assembly-manifest"), GraphDigest: generationPortDigestV1(t, "graph"), CatalogDigest: generationPortDigestV1(t, "catalog")},
		ComponentManifests: []ports.GenerationComponentManifestRefV1{component},
		Extension: ports.GenerationGovernanceExtensionRefV1{Kind: "praxis.harness/assembly-generation", Contract: ports.SchemaRefV2{
			Namespace: "praxis.harness", Name: "assembly-generation", Version: "1.0.0", MediaType: "application/json", ContentDigest: generationPortDigestV1(t, "extension-schema"),
		}, Digest: generationPortDigestV1(t, "extension")},
		State: ports.GenerationCurrentSealedV1, Current: true, Watermark: 1, ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := ports.SealGenerationBindingSetCurrentProjectionV1(ports.GenerationBindingSetCurrentProjectionV1{
		BindingSetID: "binding-set-1", BindingSetRevision: 1,
		BindingSetDigest: generationPortDigestV1(t, "binding-set"), BindingSetSemanticDigest: generationPortDigestV1(t, "binding-semantic"),
		PlanDigest: generationPortDigestV1(t, "plan"), GovernanceDigest: generationPortDigestV1(t, "governance"),
		ComponentManifestSetDigest: ports.GenerationComponentManifestSetDigestV1(generation.ComponentManifests), CurrentnessDigest: generationPortDigestV1(t, "currentness"),
		IssuedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "identity-1", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-1", PlanDigest: generationPortDigestV1(t, "lineage-plan")}, Instance: core.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "lease-1", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := ports.OperationSubjectV3{Kind: ports.OperationScopeActivationV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, ActivationAttemptID: "activation-1", SubjectRevision: 1, CurrentProjectionRef: "activation-projection-1", CurrentProjectionDigest: generationPortDigestV1(t, "activation-current"), CurrentProjectionRevision: 1}
	operationDigest, _ := operation.DigestV3()
	activation, err := ports.SealGenerationActivationCurrentProjectionV1(ports.GenerationActivationCurrentProjectionV1{Operation: operation, OperationDigest: operationDigest, Active: true, Watermark: 1, CurrentnessDigest: generationPortDigestV1(t, "activation-watermark"), ExpiresUnixNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := ports.SealGenerationBindingAssociationCandidateV1(ports.GenerationBindingAssociationCandidateV1{AssociationID: "association-1", Generation: generation, Binding: binding, Activation: activation, RequestedExpiresUnixNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return candidate
}

func generationPortDigestV1(t *testing.T, value string) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
