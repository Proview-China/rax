package ports_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestCustomReviewerConformanceCannotSelfGrantProductionDispatchOrCommit(t *testing.T) {
	t.Parallel()
	now := time.Unix(46_000, 0)
	intent, _, _, _, _ := effectPortFixtureV2(t, now)
	subject, err := intent.ReviewSubjectDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	binding := ports.ReviewComponentBindingRefV2{BindingSetID: intent.Provider.BindingSetID, BindingSetRevision: intent.Provider.BindingSetRevision, ComponentID: "custom/reviewer", ManifestDigest: portEffectDigestV2(t, "reviewer-manifest"), ArtifactDigest: portEffectDigestV2(t, "reviewer-artifact"), Capability: "custom/review-attest"}
	candidate := ports.ReviewCandidateV2{ContractVersion: ports.ReviewContractVersionV2, ID: intent.Review.Ref, Revision: intent.Review.Revision, IntentID: intent.ID, IntentRevision: intent.Revision, SubjectDigest: subject, CandidateKind: "custom/effect-review", RiskClass: intent.RiskClass, PayloadSchema: intent.Payload.Schema, PayloadDigest: intent.Payload.ContentDigest, PayloadRevision: intent.PayloadRevision, Scope: intent.Scope, RunID: intent.RunID, ActionScopeDigest: intent.ActionScopeDigest, SubjectProvider: intent.Provider, ReviewOwnerBinding: binding, ReviewerBinding: binding, CurrentScope: intent.CurrentScope, ActorAuthority: intent.Authority, ReviewerAuthority: intent.Authority, Policy: ports.ReviewPolicyBindingRefV2{Ref: "review-policy", Digest: intent.Review.PolicyDigest, Revision: 1}, Evidence: []ports.ReviewEvidenceRefV2{}, InvocationMode: ports.ReviewInvocationHuman, RequestedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}
	candidateDigest, err := candidate.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	attestation := ports.ReviewAttestationObservationV2{CaseID: candidate.ID, CandidateDigest: candidateDigest, ReviewerBinding: binding, ReviewerAuthority: candidate.ReviewerAuthority, ProposedState: ports.ReviewVerdictAccepted, Basis: ports.ReviewBasisHuman, Evidence: []ports.ReviewEvidenceRefV2{{Ref: "custom-attestation", Classification: "custom/reviewer-attestation", Digest: portEffectDigestV2(t, "attestation")}}, Conditions: []ports.ReviewConditionV2{}, ObservedUnixNano: now.UnixNano()}
	report, err := conformance.CheckReviewAdapterV2(conformance.ReviewAdapterCaseV2{Candidate: candidate, Attestation: attestation})
	if err != nil {
		t.Fatal(err)
	}
	if !report.AttestationValid || !report.CertificationCandidate || report.BindingEligible || report.ProductionClaimEligible || report.DispatchEligible || report.DomainCommitEligible {
		t.Fatalf("custom reviewer observation must remain non-authoritative: %+v", report)
	}
	attestation.CandidateDigest = portEffectDigestV2(t, "other-candidate")
	if _, err := conformance.CheckReviewAdapterV2(conformance.ReviewAdapterCaseV2{Candidate: candidate, Attestation: attestation}); err == nil {
		t.Fatal("custom reviewer cannot attest a different candidate revision/digest")
	}
}
