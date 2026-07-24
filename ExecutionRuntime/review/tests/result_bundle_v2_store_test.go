package review

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestResultBundleV2StoreCreateOnceAndExactInspect(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	store, err := memory.NewStoreWithClockV1(func() time.Time { return now.Add(time.Second) })
	if err != nil {
		t.Fatal(err)
	}
	target := testkit.Target(now)
	testkit.PublishRubric(context.Background(), store, now, target.TenantID)
	request := testkit.Request(now, target, "case-result-v2")
	value := minimalResultBundleV2(t)
	value.TenantID = target.TenantID
	value.Request = contract.ExactResourceRefV1{ID: request.ID, Revision: request.Revision, Digest: request.Digest}
	value.Target = contract.ExactResourceRefV1{ID: target.ID, Revision: target.Revision, Digest: target.Digest}
	value.Digest = ""
	value, err = contract.SealReviewResultBundleV2(value)
	if err != nil {
		t.Fatal(err)
	}
	caseFact, err := contract.SealReviewCaseV1(contract.ReviewCaseV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: request.CaseID, Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		TargetID:       target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest, Rubric: &request.Rubric,
		State: contract.CaseRequestedV1, ExpiresUnixNano: request.ExpiresUnixNano,
	})
	if err != nil {
		t.Fatal(err)
	}
	gotCase, err := store.CreateTargetCaseV1(context.Background(), reviewport.CreateTargetCaseMutationV1{
		Request: &request, ResultBundleV2: &value, Target: target, Case: caseFact, RubricCheckedUnixNano: now.Add(time.Second).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotCase.Digest != caseFact.Digest {
		t.Fatal("atomic V2 admission drifted")
	}
	ref := reviewport.ExactV1(value.ID, value.Revision, value.Digest)
	if _, err := store.InspectResultBundleExactV2(context.Background(), value.TenantID, ref); err != nil {
		t.Fatal(err)
	}
	changed := value
	changed.Limitations = []string{"changed"}
	changed.Digest = ""
	changed, err = contract.SealReviewResultBundleV2(changed)
	if err != nil {
		t.Fatal(err)
	}
	changedMutation := reviewport.CreateTargetCaseMutationV1{
		Request: &request, ResultBundleV2: &changed, Target: target, Case: caseFact, RubricCheckedUnixNano: now.Add(time.Second).UnixNano(),
	}
	if _, err := store.CreateTargetCaseV1(context.Background(), changedMutation); err == nil {
		t.Fatal("same atomic admission with changed V2 bundle must conflict")
	}
	after, err := store.InspectResultBundleExactV2(context.Background(), value.TenantID, ref)
	if err != nil || after.Digest != value.Digest {
		t.Fatal("conflicting create must not overwrite exact history")
	}
	snapshot, err := store.ExportSnapshotV1(value.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := memory.NewStoreFromSnapshotV1(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := restored.InspectResultBundleExactV2(context.Background(), value.TenantID, ref)
	if err != nil || persisted.Digest != value.Digest {
		t.Fatal("V2 bundle must survive the durable snapshot boundary")
	}
}

func minimalResultBundleV2(t *testing.T) contract.ReviewResultBundleV2 {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	d0, d1 := core.DigestBytes([]byte("0")), core.DigestBytes([]byte("1"))
	schema := runtimeports.SchemaRefV2{Namespace: "praxis.review", Name: "anchor", Version: "1.0.0", MediaType: "application/json", ContentDigest: d0}
	body := []byte(`{"line":1}`)
	locator, err := runtimeports.SealReviewArtifactLocatorV2(runtimeports.ReviewArtifactLocatorV2{Kind: "praxis.anchor/line", Schema: schema, Payload: runtimeports.OpaquePayloadV2{Schema: schema, ContentDigest: core.DigestBytes(body), Length: uint64(len(body)), Inline: body, LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.limit/review-anchor", Digest: d1}}})
	if err != nil {
		t.Fatal(err)
	}
	owner := runtimeports.ReviewGroundingOwnerRefV2{Binding: runtimeports.ReviewComponentBindingRefV2{BindingSetID: "set", BindingSetRevision: 1, ComponentID: "praxis.artifact/owner", ManifestDigest: d0, ArtifactDigest: d1, Capability: "praxis.artifact/current"}, SourceContract: "praxis.artifact/current-v2"}
	artifact := runtimeports.ReviewArtifactExactSourceRefV2{Kind: "praxis.artifact/code", Owner: owner, TenantID: "tenant", ID: "artifact", Revision: 1, Digest: d0, ScopeDigest: d1}
	evidence := runtimeports.ReviewEvidenceRefV2{Ref: "evidence", Classification: "praxis.evidence/test", Digest: d0}
	evidenceDigest, _ := contract.ComputeReviewEvidenceDigestV1([]runtimeports.ReviewEvidenceRefV2{evidence})
	intent := contract.ReviewerContextSourceRefV1{Owner: "praxis.context/owner", ID: "intent", Revision: 1, Digest: d0, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	criterion := contract.ReviewerContextSourceRefV1{Owner: "praxis.context/owner", ID: "criterion", Revision: 1, Digest: d1, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	envOwner := owner
	envOwner.Binding.ComponentID, envOwner.Binding.Capability, envOwner.SourceContract = "praxis.environment/owner", "praxis.environment/current", "praxis.environment/current-v2"
	scopeOwner := owner
	scopeOwner.Binding.ComponentID, scopeOwner.Binding.Capability, scopeOwner.SourceContract = "praxis.validation/owner", "praxis.validation/current", "praxis.validation/current-v2"
	value := contract.ReviewResultBundleV2{
		FactIdentityV1: contract.FactIdentityV1{TenantID: "tenant", ID: "bundle", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		Request:        contract.ExactResourceRefV1{ID: "request", Revision: 1, Digest: d0}, Target: contract.ExactResourceRefV1{ID: "target", Revision: 1, Digest: d1},
		OriginalIntent: intent, AcceptanceCriteria: []contract.ReviewerContextSourceRefV1{criterion},
		Artifacts:       []contract.ReviewResultArtifactBindingV2{{Source: artifact, Anchors: []runtimeports.ReviewArtifactLocatorV2{locator}}},
		Claims:          []contract.ReviewResultClaimV2{{ID: "claim", Statement: "complete", Artifact: artifact, Anchor: locator, Evidence: []runtimeports.ReviewEvidenceRefV2{evidence}}},
		Environment:     runtimeports.ReviewEnvironmentExactRefV2{Kind: "praxis.environment/sandbox", Owner: envOwner, TenantID: "tenant", ID: "env", Revision: 1, Digest: d0, ScopeDigest: d1},
		ReviewerContext: contract.ReviewerContextEnvelopeRefV1{TenantID: "tenant", ID: "context", Revision: 1, Digest: d0}, ReviewerContextSources: []contract.ReviewerContextSourceRefV1{criterion, intent},
		ValidationScope: runtimeports.ReviewValidationScopeExactRefV2{Source: runtimeports.ReviewValidationScopeSourceIdentityV2{Kind: "praxis.validation/test", TenantID: "tenant", ID: "scope"}, Owner: scopeOwner, Revision: 1, Digest: d1, ScopeDigest: d0},
		Limitations:     []string{}, Uncovered: []string{}, EvidenceSetDigest: evidenceDigest, ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	}
	value, err = contract.SealReviewResultBundleV2(value)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
