package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestResultBundleGroundsEveryClaimV1(t *testing.T) {
	now := time.Unix(1_900_500_000, 0)
	artifact := contract.ExactResourceRefV1{ID: "artifact-a", Revision: 2, Digest: testkit.Digest("artifact")}
	evidence := []runtimeports.ReviewEvidenceRefV2{testkit.Evidence("result")}
	evidenceDigest, _ := contract.ComputeReviewEvidenceDigestV1(evidence)
	value, err := contract.SealReviewResultBundleV1(contract.ReviewResultBundleV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: "tenant-a", ID: "bundle-a", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, OriginalTaskDigest: testkit.Digest("task"), AcceptanceDigest: testkit.Digest("acceptance"), Artifacts: []contract.ExactResourceRefV1{artifact}, Claims: []contract.ResultClaimV1{{ID: "claim-a", Statement: "artifact passes exact acceptance", Artifact: artifact, Anchor: "section-a", Evidence: evidence}}, EnvironmentDigest: testkit.Digest("environment"), ValidationScopeDigest: testkit.Digest("scope"), EvidenceSetDigest: evidenceDigest, ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	value.Claims[0].Artifact.Digest = testkit.Digest("drift")
	value.Digest = ""
	if _, err := contract.SealReviewResultBundleV1(value); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("claim-to-artifact drift accepted: %v", err)
	}
}

func TestBehaviorFeedbackRemainsCandidateV1(t *testing.T) {
	now := time.Unix(1_900_500_100, 0)
	evidence := []runtimeports.ReviewEvidenceRefV2{testkit.Evidence("behavior")}
	digest, _ := contract.ComputeReviewEvidenceDigestV1(evidence)
	value, err := contract.SealBehaviorFeedbackCandidateV1(contract.BehaviorFeedbackCandidateV1{FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: "tenant-a", ID: "feedback-a", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()}, Case: contract.ExactResourceRefV1{ID: "case-a", Revision: 2, Digest: testkit.Digest("case")}, Target: contract.ExactResourceRefV1{ID: "target-a", Revision: 1, Digest: testkit.Digest("target")}, Verdict: contract.ExactResourceRefV1{ID: "verdict-a", Revision: 1, Digest: testkit.Digest("verdict")}, Policy: runtimeports.ReviewPolicyBindingRefV2{Ref: "policy-a", Revision: 1, Digest: testkit.Digest("policy")}, ReviewerID: "reviewer-a", ReviewerBinding: runtimeports.ReviewComponentBindingRefV2{BindingSetID: "binding-a", BindingSetRevision: 1, ComponentID: "praxis.review/reviewer", ManifestDigest: testkit.Digest("manifest"), ArtifactDigest: testkit.Digest("reviewer-artifact"), Capability: "praxis.review/attest"}, BehaviorClass: "praxis.review/repeated-unsafe-action", SignalDigest: testkit.Digest("signal"), Evidence: evidence, EvidenceDigest: digest, ExpiresUnixNano: now.Add(time.Hour).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
}
