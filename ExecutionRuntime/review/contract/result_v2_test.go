package contract

import (
	"reflect"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestReviewResultBundleV2CanonicalAndDeepClone(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	value := resultBundleFixtureV2(t, now)
	sealed, err := SealReviewResultBundleV2(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := sealed.Validate(); err != nil {
		t.Fatal(err)
	}
	cloned := sealed.Clone()
	cloned.Artifacts[0].Anchors[0].Payload.Inline[0] ^= 0xff
	if reflect.DeepEqual(cloned, sealed) || cloned.Artifacts[0].Anchors[0].Payload.ContentDigest != sealed.Artifacts[0].Anchors[0].Payload.ContentDigest {
		t.Fatal("bundle clone must isolate mutable locator payload bytes")
	}
}

func TestReviewResultBundleV2RejectsUnreachableAnchor(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	value := resultBundleFixtureV2(t, now)
	extra := value.Artifacts[0].Anchors[0]
	extra.Kind = "praxis.anchor/other"
	extra.LocatorDigest = ""
	extra, err := runtimeports.SealReviewArtifactLocatorV2(extra)
	if err != nil {
		t.Fatal(err)
	}
	value.Artifacts[0].Anchors = append(value.Artifacts[0].Anchors, extra)
	if _, err := SealReviewResultBundleV2(value); err == nil {
		t.Fatal("unreachable artifact anchor must fail closed")
	}
}

func resultBundleFixtureV2(t *testing.T, now time.Time) ReviewResultBundleV2 {
	t.Helper()
	d0 := core.DigestBytes([]byte("0"))
	d1 := core.DigestBytes([]byte("1"))
	schema := runtimeports.SchemaRefV2{Namespace: "praxis.review", Name: "anchor", Version: "1.0.0", MediaType: "application/json", ContentDigest: d0}
	payload := []byte(`{"line":7}`)
	locator, err := runtimeports.SealReviewArtifactLocatorV2(runtimeports.ReviewArtifactLocatorV2{
		Kind:   "praxis.anchor/line",
		Schema: schema,
		Payload: runtimeports.OpaquePayloadV2{
			Schema: schema, ContentDigest: core.DigestBytes(payload), Length: uint64(len(payload)), Inline: payload,
			LimitPolicy: runtimeports.OpaqueLimitPolicyRefV2{Policy: "praxis.limit/review-anchor", Digest: d1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := runtimeports.ReviewGroundingOwnerRefV2{
		Binding: runtimeports.ReviewComponentBindingRefV2{
			BindingSetID: "set-a", BindingSetRevision: 1, ComponentID: "praxis.artifact/owner",
			ManifestDigest: d0, ArtifactDigest: d1, Capability: "praxis.artifact/current",
		},
		SourceContract: "praxis.artifact/current-v2",
	}
	artifact := runtimeports.ReviewArtifactExactSourceRefV2{Kind: "praxis.artifact/code", Owner: owner, TenantID: "tenant-a", ID: "artifact-a", Revision: 1, Digest: d0, ScopeDigest: d1}
	environmentOwner := owner
	environmentOwner.Binding.ComponentID = "praxis.environment/owner"
	environmentOwner.Binding.Capability = "praxis.environment/current"
	environmentOwner.SourceContract = "praxis.environment/current-v2"
	scopeOwner := owner
	scopeOwner.Binding.ComponentID = "praxis.validation/owner"
	scopeOwner.Binding.Capability = "praxis.validation/current"
	scopeOwner.SourceContract = "praxis.validation/current-v2"
	evidence := runtimeports.ReviewEvidenceRefV2{Ref: "evidence-a", Classification: "praxis.evidence/test", Digest: d0}
	evidenceDigest, err := ComputeReviewEvidenceDigestV1([]runtimeports.ReviewEvidenceRefV2{evidence})
	if err != nil {
		t.Fatal(err)
	}
	intent := ReviewerContextSourceRefV1{Owner: "praxis.context/owner", ID: "intent-a", Revision: 1, Digest: d0, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	criterion := ReviewerContextSourceRefV1{Owner: "praxis.context/owner", ID: "criterion-a", Revision: 1, Digest: d1, ExpiresUnixNano: now.Add(time.Hour).UnixNano()}
	return ReviewResultBundleV2{
		FactIdentityV1:         FactIdentityV1{TenantID: "tenant-a", ID: "bundle-v2-a", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		Request:                ExactResourceRefV1{ID: "request-a", Revision: 1, Digest: d0},
		Target:                 ExactResourceRefV1{ID: "target-a", Revision: 1, Digest: d1},
		OriginalIntent:         intent,
		AcceptanceCriteria:     []ReviewerContextSourceRefV1{criterion},
		Artifacts:              []ReviewResultArtifactBindingV2{{Source: artifact, Anchors: []runtimeports.ReviewArtifactLocatorV2{locator}}},
		Claims:                 []ReviewResultClaimV2{{ID: "claim-a", Statement: "tests pass", Artifact: artifact, Anchor: locator, Evidence: []runtimeports.ReviewEvidenceRefV2{evidence}}},
		Environment:            runtimeports.ReviewEnvironmentExactRefV2{Kind: "praxis.environment/sandbox", Owner: environmentOwner, TenantID: "tenant-a", ID: "environment-a", Revision: 1, Digest: d0, ScopeDigest: d1},
		ReviewerContext:        ReviewerContextEnvelopeRefV1{TenantID: "tenant-a", ID: "context-a", Revision: 1, Digest: d0},
		ReviewerContextSources: []ReviewerContextSourceRefV1{criterion, intent},
		ValidationScope:        runtimeports.ReviewValidationScopeExactRefV2{Source: runtimeports.ReviewValidationScopeSourceIdentityV2{Kind: "praxis.validation/test", TenantID: "tenant-a", ID: "scope-a"}, Owner: scopeOwner, Revision: 1, Digest: d1, ScopeDigest: d0},
		Limitations:            []string{}, Uncovered: []string{}, EvidenceSetDigest: evidenceDigest, ExpiresUnixNano: now.Add(time.Hour).UnixNano(),
	}
}
