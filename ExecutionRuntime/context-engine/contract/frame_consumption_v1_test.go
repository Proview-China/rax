package contract_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestFrameConsumptionRequestAndCacheKeysSealExactClosure(t *testing.T) {
	fixture, err := testfixture.NewFrameConsumptionFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	request := fixture.Request
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	changed := request
	changed.DisclosureClass = contract.DisclosureRestrictedV1
	changed, err = contract.SealContextFrameConsumptionRequestV1(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changed.RequestDigest == request.RequestDigest {
		t.Fatal("disclosure change did not change request digest")
	}
	fragment := fixture.Result.Manifest.Fragments[0]
	key, err := contract.SealContextFragmentCacheKeyV1(contract.ContextFragmentCacheKeyV1{
		TenantScopeDigest: fixture.Request.TenantScopeDigest,
		AgentInstanceRef:  fixture.Request.AgentInstanceRef, RunID: fixture.Request.RunID, RunScopeDigest: fixture.Request.RunScopeDigest,
		FragmentRef: fragment.CandidateRef, Content: fragment.Content, PromptAssetRefs: []contract.PromptAssetRefV1{},
		RecipeRef: fixture.Request.RecipeRef, DisclosureClass: fixture.Request.DisclosureClass,
		InvalidationGeneration: 1, KeyVersion: contract.FrameConsumptionKeyV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	changedKey := key
	changedKey.Content = fixture.Result.Manifest.Fragments[1].Content
	changedKey, err = contract.SealContextFragmentCacheKeyV1(changedKey)
	if err != nil {
		t.Fatal(err)
	}
	if changedKey.Digest == key.Digest {
		t.Fatal("content change did not change fragment cache key")
	}
}

func TestCompressionEvidenceRejectsRemovedRequiredAnchor(t *testing.T) {
	required := contract.FactRef{ID: "required-anchor", Revision: 1, Digest: testkit.D("required")}
	_, err := contract.SealCompressionEvidenceV1(contract.CompressionEvidenceV1{
		ID: "compression-evidence", SourceFrameRef: required, SourceGenerationRef: required,
		CandidateOutputRef: contract.ContentRef{Ref: "candidate", Digest: testkit.D("candidate"), Length: 1},
		RetainedAnchorRefs: []contract.FactRef{}, DeltaRefs: []contract.FactRef{}, RequiredAnchorRefs: []contract.FactRef{required},
		EvaluationRef: required, EvaluatorRef: contract.ContextEvaluatorRefV1{ID: "evaluator", Revision: 1, Digest: testkit.D("evaluator")},
		EvaluatorVersion: "v1", SourceTokens: 100, CandidateTokens: 50, CoveragePPM: 1_000_000,
		Limitations: []string{}, InvariantGateDigest: testkit.D("gate"),
		CheckedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 100,
	})
	if !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("removed required anchor error=%v", err)
	}
}

func TestFrameConsumptionErrorClassificationPreservesIndeterminate(t *testing.T) {
	cases := []struct {
		err  error
		want contract.FrameConsumptionErrorClassV1
	}{
		{contract.ErrInspectOnly, contract.FrameConsumptionInspectOnlyV1},
		{contract.ErrUnauthorized, contract.FrameConsumptionUnauthorizedV1},
		{contract.ErrNotFound, contract.FrameConsumptionNotFoundV1},
		{contract.ErrExpired, contract.FrameConsumptionExpiredV1},
		{contract.ErrConflict, contract.FrameConsumptionConflictV1},
		{contract.ErrUnavailable, contract.FrameConsumptionUnavailableV1},
		{contract.ErrUnknown, contract.FrameConsumptionIndeterminateV1},
		{context.Canceled, contract.FrameConsumptionIndeterminateV1},
		{context.DeadlineExceeded, contract.FrameConsumptionIndeterminateV1},
		{contract.ErrLimitExceeded, contract.FrameConsumptionLimitV1},
		{contract.ErrUnsupported, contract.FrameConsumptionUnsupportedV1},
		{errors.New("untyped"), contract.FrameConsumptionInvalidV1},
	}
	for _, test := range cases {
		if got := contract.ClassifyFrameConsumptionErrorV1(test.err); got != test.want {
			t.Fatalf("classification %v=%s want=%s", test.err, got, test.want)
		}
	}
}
