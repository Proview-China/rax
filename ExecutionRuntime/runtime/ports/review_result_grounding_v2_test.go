package ports

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type artifactReaderStubV2 struct{}

func (*artifactReaderStubV2) ResolveCurrentReviewArtifactV2(context.Context, ReviewArtifactCurrentSubjectV2) (ReviewArtifactCurrentProjectionRefV2, error) {
	return ReviewArtifactCurrentProjectionRefV2{}, nil
}
func (*artifactReaderStubV2) InspectCurrentReviewArtifactV2(context.Context, ReviewArtifactCurrentSubjectV2, ReviewArtifactCurrentProjectionRefV2) (ReviewArtifactCurrentProjectionV2, error) {
	return ReviewArtifactCurrentProjectionV2{}, nil
}
func (*artifactReaderStubV2) InspectHistoricalReviewArtifactV2(context.Context, ReviewArtifactCurrentProjectionRefV2) (ReviewArtifactCurrentProjectionV2, error) {
	return ReviewArtifactCurrentProjectionV2{}, nil
}

func TestReviewGroundingProjectionIdentityGoldenV2(t *testing.T) {
	d0 := core.Digest("sha256:0000000000000000000000000000000000000000000000000000000000000000")
	d1 := core.Digest("sha256:1111111111111111111111111111111111111111111111111111111111111111")
	artifact := ReviewArtifactCurrentProjectionIdentityInputV2{Expected: ReviewArtifactExactSourceRefV2{
		Kind:     "praxis.artifact/code",
		Owner:    ReviewGroundingOwnerRefV2{Binding: ReviewComponentBindingRefV2{BindingSetID: "set-a", BindingSetRevision: 7, ComponentID: "praxis.artifact/code-owner", ManifestDigest: d0, ArtifactDigest: d1, Capability: "praxis.artifact/current"}, SourceContract: "praxis.artifact/current-v2"},
		TenantID: "tenant-a", ID: "artifact-a", Revision: 9, Digest: d0, ScopeDigest: d1,
	}}
	literal, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	wantLiteral := `{"expected":{"kind":"praxis.artifact/code","owner":{"binding":{"binding_set_id":"set-a","binding_set_revision":7,"component_id":"praxis.artifact/code-owner","manifest_digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000","artifact_digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","capability":"praxis.artifact/current"},"source_contract":"praxis.artifact/current-v2"},"tenant_id":"tenant-a","id":"artifact-a","revision":9,"digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000","scope_digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111"}}`
	if string(literal) != wantLiteral {
		t.Fatalf("canonical identity JSON drifted:\n%s", literal)
	}
	id, err := DeriveReviewArtifactCurrentProjectionIDV2(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if id != "sha256:01cf0b40f11c76489cdeb368ec8909efbf603ae333916b95a9a1ed56ac0ba9c3" {
		t.Fatalf("artifact projection ID drifted: %s", id)
	}
	environment := ReviewEnvironmentCurrentProjectionIdentityInputV2{Expected: ReviewEnvironmentExactRefV2{
		Kind:     "praxis.environment/sandbox",
		Owner:    ReviewGroundingOwnerRefV2{Binding: ReviewComponentBindingRefV2{BindingSetID: "set-a", BindingSetRevision: 7, ComponentID: "praxis.sandbox/environment-owner", ManifestDigest: d0, ArtifactDigest: d1, Capability: "praxis.environment/current"}, SourceContract: "praxis.environment/current-v2"},
		TenantID: "tenant-a", ID: "environment-a", Revision: 3, Digest: d0, ScopeDigest: d1,
	}}
	environmentID, err := DeriveReviewEnvironmentCurrentProjectionIDV2(environment)
	if err != nil {
		t.Fatal(err)
	}
	if environmentID != "sha256:66bffe251c5759f9a64bec3af11ba44fadab24aa7f9bc01e02d0f914c9cbd369" {
		t.Fatalf("environment projection ID drifted: %s", environmentID)
	}
	scope := ReviewValidationScopeCurrentProjectionIdentityInputV2{Source: ReviewValidationScopeSourceIdentityV2{Kind: "praxis.validation/test-coverage", TenantID: "tenant-a", ID: "validation-scope-a"}}
	scopeID, err := DeriveReviewValidationScopeCurrentProjectionIDV2(scope)
	if err != nil {
		t.Fatal(err)
	}
	if scopeID != "sha256:f133bc4b9da38df6a69d194d4166ff0b43911ba630c4005dcdc421638fb303ca" {
		t.Fatalf("validation scope projection ID drifted: %s", scopeID)
	}
}

func TestReviewArtifactLocatorV2DeepCanonicalDigest(t *testing.T) {
	schema := SchemaRefV2{Namespace: "praxis.review", Name: "anchor", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("schema"))}
	body := []byte(`{"line":1}`)
	value, err := SealReviewArtifactLocatorV2(ReviewArtifactLocatorV2{Kind: "praxis.anchor/line", Schema: schema, Payload: OpaquePayloadV2{Schema: schema, ContentDigest: core.DigestBytes(body), Length: uint64(len(body)), Inline: body, LimitPolicy: OpaqueLimitPolicyRefV2{Policy: "praxis.limit/review-anchor", Digest: core.DigestBytes([]byte("policy"))}}})
	if err != nil {
		t.Fatal(err)
	}
	changed := value
	changed.Payload.Inline = append([]byte(nil), changed.Payload.Inline...)
	changed.Payload.Inline[0] ^= 0xff
	if changed.Validate() == nil {
		t.Fatal("mutated locator payload must fail exact digest validation")
	}
}

func TestReviewGroundingRouterV2IsClosedAndRejectsTypedNil(t *testing.T) {
	d0, d1 := core.DigestBytes([]byte("0")), core.DigestBytes([]byte("1"))
	owner := ReviewGroundingOwnerRefV2{Binding: ReviewComponentBindingRefV2{BindingSetID: "set", BindingSetRevision: 1, ComponentID: "praxis.artifact/owner", ManifestDigest: d0, ArtifactDigest: d1, Capability: "praxis.artifact/current"}, SourceContract: "praxis.artifact/current-v2"}
	declaration := ReviewGroundingRouteDeclarationV2{Family: ReviewGroundingArtifactRouteV2, Kind: "praxis.artifact/code", Owner: owner, Required: true}
	catalog, err := SealReviewGroundingRequiredRouteCatalogV2(ReviewGroundingRequiredRouteCatalogV2{Artifact: []ReviewGroundingRouteDeclarationV2{declaration}, Environment: []ReviewGroundingRouteDeclarationV2{}, ValidationScope: []ReviewGroundingRouteDeclarationV2{}})
	if err != nil {
		t.Fatal(err)
	}
	route, err := DeriveReviewGroundingRouteRefV2(declaration)
	if err != nil {
		t.Fatal(err)
	}
	readerBinding, err := SealReviewGroundingReaderBindingRefV2(ReviewGroundingReaderBindingRefV2{ID: "artifact-reader", Revision: 1, Route: route, AdapterArtifactDigest: d0})
	if err != nil {
		t.Fatal(err)
	}
	var typedNil *artifactReaderStubV2
	if _, err := NewReviewGroundingReaderResolverV2(catalog, []ReviewArtifactRouteBindingV2{{Declaration: declaration, ReaderBinding: readerBinding, Reader: typedNil}}, nil, nil); err == nil {
		t.Fatal("typed-nil grounding Reader must fail at construction")
	}
	resolver, err := NewReviewGroundingReaderResolverV2(catalog, []ReviewArtifactRouteBindingV2{{Declaration: declaration, ReaderBinding: readerBinding, Reader: &artifactReaderStubV2{}}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolver.ResolveReviewArtifactReaderV2(context.Background(), ReviewGroundingRouteRequestV2{Family: ReviewGroundingArtifactRouteV2, Kind: declaration.Kind, Owner: owner})
	if err != nil || resolved.Proof.Declaration != declaration {
		t.Fatalf("exact declared route failed: resolved=%+v err=%v", resolved, err)
	}
	if _, err := resolver.ResolveReviewArtifactReaderV2(context.Background(), ReviewGroundingRouteRequestV2{Family: ReviewGroundingArtifactRouteV2, Kind: "praxis.artifact/unknown", Owner: owner}); err == nil {
		t.Fatal("unknown grounding kind must fail closed")
	}
	for name, mutate := range map[string]func(*ReviewGroundingOwnerRefV2){
		"binding-set":      func(v *ReviewGroundingOwnerRefV2) { v.Binding.BindingSetID = "other-set" },
		"binding-revision": func(v *ReviewGroundingOwnerRefV2) { v.Binding.BindingSetRevision++ },
		"manifest": func(v *ReviewGroundingOwnerRefV2) {
			v.Binding.ManifestDigest = core.DigestBytes([]byte("other-manifest"))
		},
		"artifact": func(v *ReviewGroundingOwnerRefV2) {
			v.Binding.ArtifactDigest = core.DigestBytes([]byte("other-artifact"))
		},
		"source-contract": func(v *ReviewGroundingOwnerRefV2) { v.SourceContract = "praxis.artifact/current-v3" },
	} {
		t.Run("full-owner-"+name, func(t *testing.T) {
			drift := owner
			mutate(&drift)
			if _, err := resolver.ResolveReviewArtifactReaderV2(context.Background(), ReviewGroundingRouteRequestV2{Family: ReviewGroundingArtifactRouteV2, Kind: declaration.Kind, Owner: drift}); err == nil {
				t.Fatal("full Owner binding drift must not resolve through a weaker route key")
			}
		})
	}
	extra := declaration
	extra.Kind = "praxis.artifact/extra"
	if _, err := NewReviewGroundingReaderResolverV2(catalog, []ReviewArtifactRouteBindingV2{{Declaration: extra, ReaderBinding: readerBinding, Reader: &artifactReaderStubV2{}}}, nil, nil); err == nil {
		t.Fatal("binding absent from sealed catalog must fail construction")
	}
}
