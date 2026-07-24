package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestGenerationBindingSQLiteLostRepliesRecoverOnlyByInspect(t *testing.T) {
	now := time.Unix(2_410_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now.Add(time.Second) })
	fact := generationBindingSQLiteFactV1(t, now)
	store.loseNextReplyForTest()
	if _, err := store.CreateGenerationBindingAssociationV1(context.Background(), fact); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost Create reply was not indeterminate: %v", err)
	}
	recovered, err := store.InspectGenerationBindingAssociationV1(context.Background(), fact.ID)
	if err != nil || recovered.Digest != fact.Digest {
		t.Fatalf("Create recovery did not require exact Inspect: %+v err=%v", recovered, err)
	}
	if _, err := store.CreateGenerationBindingAssociationV1(context.Background(), fact); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("Create replay returned another normal response: %v", err)
	}
	revoked, err := ports.NextGenerationBindingAssociationStateV1(fact, ports.GenerationBindingAssociationRevokedV1, core.ReasonBindingDrift, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	store.loseNextReplyForTest()
	if _, err := store.CompareAndSwapGenerationBindingAssociationV1(context.Background(), ports.GenerationBindingAssociationCASRequestV1{ExpectedRevision: fact.Revision, Next: revoked}); !core.HasCategory(err, core.ErrorIndeterminate) {
		t.Fatalf("lost CAS reply was not indeterminate: %v", err)
	}
	recovered, err = store.InspectGenerationBindingAssociationV1(context.Background(), fact.ID)
	if err != nil || recovered.Digest != revoked.Digest {
		t.Fatalf("CAS recovery did not require exact Inspect: %+v err=%v", recovered, err)
	}
	if _, err := store.CompareAndSwapGenerationBindingAssociationV1(context.Background(), ports.GenerationBindingAssociationCASRequestV1{ExpectedRevision: fact.Revision, Next: revoked}); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("stale CAS replay was not Conflict: %v", err)
	}
}

func TestGenerationBindingSQLiteStagedFailureClockAndClone(t *testing.T) {
	now := time.Unix(2_420_000_000, 0)
	store := openTestStore(t, testDBPath(t), func() time.Time { return now })
	fact := generationBindingSQLiteFactV1(t, now)
	store.failNextStageForTest()
	if _, err := store.CreateGenerationBindingAssociationV1(context.Background(), fact); !core.HasCategory(err, core.ErrorUnavailable) {
		t.Fatalf("staged failure was not unavailable: %v", err)
	}
	if _, err := store.InspectGenerationBindingAssociationV1(context.Background(), fact.ID); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("staged failure leaked a Fact: %v", err)
	}
	future := fact
	future.CreatedUnixNano = now.Add(time.Second).UnixNano()
	future.UpdatedUnixNano = future.CreatedUnixNano
	future.Digest = ""
	future, err := ports.SealGenerationBindingAssociationFactV1(future)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateGenerationBindingAssociationV1(context.Background(), future); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("future Fact crossed the clock gate: %v", err)
	}
	created, err := store.CreateGenerationBindingAssociationV1(context.Background(), fact)
	if err != nil {
		t.Fatal(err)
	}
	created.Candidate.Generation.ComponentManifests[0].ComponentID = "vendor/other"
	again, err := store.InspectGenerationBindingAssociationV1(context.Background(), fact.ID)
	if err != nil || again.Candidate.Generation.ComponentManifests[0].ComponentID != "vendor/component" {
		t.Fatalf("returned Fact aliased persisted JSON: %+v err=%v", again, err)
	}
}

func generationBindingSQLiteFactV1(t *testing.T, now time.Time) ports.GenerationBindingAssociationFactV1 {
	t.Helper()
	digest := func(value string) core.Digest { return core.DigestBytes([]byte(value)) }
	component := ports.GenerationComponentManifestRefV1{ComponentID: "vendor/component", ManifestDigest: digest("manifest"), ArtifactDigest: digest("artifact")}
	generation, err := ports.SealGenerationCurrentProjectionV1(ports.GenerationCurrentProjectionV1{Generation: ports.GenerationArtifactRefV1{ID: "generation-1", Revision: 1, Digest: digest("generation"), InputDigest: digest("input"), ManifestDigest: digest("assembly"), GraphDigest: digest("graph"), CatalogDigest: digest("catalog")}, ComponentManifests: []ports.GenerationComponentManifestRefV1{component}, Extension: ports.GenerationGovernanceExtensionRefV1{Kind: "praxis.harness/assembly-generation", Contract: ports.SchemaRefV2{Namespace: "praxis.harness", Name: "assembly-generation", Version: "1.0.0", MediaType: "application/json", ContentDigest: digest("schema")}, Digest: digest("extension")}, State: ports.GenerationCurrentSealedV1, Current: true, Watermark: 1, ExpiresUnixNano: now.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := ports.SealGenerationBindingSetCurrentProjectionV1(ports.GenerationBindingSetCurrentProjectionV1{BindingSetID: "binding-set-1", BindingSetRevision: 1, BindingSetDigest: digest("set"), BindingSetSemanticDigest: digest("semantic"), PlanDigest: digest("plan"), GovernanceDigest: digest("governance"), ComponentManifestSetDigest: ports.GenerationComponentManifestSetDigestV1(generation.ComponentManifests), CurrentnessDigest: digest("current"), IssuedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	scope := core.ExecutionScope{Identity: core.AgentIdentityRef{TenantID: "tenant-1", ID: "identity-1", Epoch: 1}, Lineage: core.LineageRef{ID: "lineage-1", PlanDigest: digest("lineage")}, Instance: core.InstanceRef{ID: "instance-1", Epoch: 1}, SandboxLease: &core.SandboxLeaseRef{ID: "lease-1", Epoch: 1}, AuthorityEpoch: 1}
	scopeDigest, err := ports.ExecutionScopeDigestV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	operation := ports.OperationSubjectV3{Kind: ports.OperationScopeActivationV3, ExecutionScope: scope, ExecutionScopeDigest: scopeDigest, ActivationAttemptID: "activation-1", SubjectRevision: 1, CurrentProjectionRef: "activation-current", CurrentProjectionDigest: digest("activation-current"), CurrentProjectionRevision: 1}
	operationDigest, err := operation.DigestV3()
	if err != nil {
		t.Fatal(err)
	}
	activation, err := ports.SealGenerationActivationCurrentProjectionV1(ports.GenerationActivationCurrentProjectionV1{Operation: operation, OperationDigest: operationDigest, Active: true, Watermark: 1, CurrentnessDigest: digest("activation-watermark"), ExpiresUnixNano: now.Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := ports.SealGenerationBindingAssociationCandidateV1(ports.GenerationBindingAssociationCandidateV1{AssociationID: "association-1", Generation: generation, Binding: binding, Activation: activation, RequestedExpiresUnixNano: now.Add(4 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	fact, err := ports.SealGenerationBindingAssociationFactV1(ports.GenerationBindingAssociationFactV1{ID: candidate.AssociationID, Revision: 1, State: ports.GenerationBindingAssociationActiveV1, Candidate: candidate, CandidateDigest: candidate.Digest, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(4 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	return fact
}
