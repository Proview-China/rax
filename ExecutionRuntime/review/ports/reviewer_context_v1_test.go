package ports

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func reviewerContextProjectionForPortV1(t *testing.T, revision core.Revision) contract.ReviewerContextEnvelopeV1 {
	t.Helper()
	now := time.Unix(1_911_000_000, 0)
	expires := now.Add(time.Hour).UnixNano()
	subject := contract.ReviewerContextSubjectV1{
		TenantID:           "tenant-a",
		Case:               contract.ExactResourceRefV1{ID: "case-a", Revision: 1, Digest: core.DigestBytes([]byte("case"))},
		Round:              contract.ExactResourceRefV1{ID: "round-a", Revision: 1, Digest: core.DigestBytes([]byte("round"))},
		Assignment:         contract.ExactResourceRefV1{ID: "assignment-a", Revision: 1, Digest: core.DigestBytes([]byte("assignment"))},
		Target:             contract.ExactResourceRefV1{ID: "target-a", Revision: 1, Digest: core.DigestBytes([]byte("target"))},
		Rubric:             contract.ExactResourceRefV1{ID: "rubric-a", Revision: 1, Digest: core.DigestBytes([]byte("rubric"))},
		ContextFrameDigest: core.DigestBytes([]byte("frame")),
		OutputSchema:       runtimeports.SchemaRefV2{Namespace: "praxis.review", Name: "attestation", Version: "1.0.0", MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("schema"))},
	}
	kinds := []contract.ReviewerContextMaterialKindV1{
		contract.ReviewerContextOriginalIntentV1, contract.ReviewerContextRequirementV1,
		contract.ReviewerContextAcceptanceCriterionV1, contract.ReviewerContextStableRuleV1,
		contract.ReviewerContextCandidateV1, contract.ReviewerContextEvidenceV1, contract.ReviewerContextKnownRiskV1,
	}
	materials := make([]contract.ReviewerContextMaterialV1, 0, len(kinds))
	for _, kind := range kinds {
		content := string(kind)
		trust := contract.ReviewerContextObservationV1
		if kind == contract.ReviewerContextOriginalIntentV1 || kind == contract.ReviewerContextRequirementV1 || kind == contract.ReviewerContextAcceptanceCriterionV1 || kind == contract.ReviewerContextStableRuleV1 {
			trust = contract.ReviewerContextInstructionV1
		}
		materials = append(materials, contract.ReviewerContextMaterialV1{
			Kind:      kind,
			Source:    contract.ReviewerContextSourceRefV1{Owner: "praxis.context/source", ID: "source-" + content, Revision: 1, Digest: core.DigestBytes([]byte("source-" + content)), ExpiresUnixNano: expires},
			MediaType: "text/plain", Content: content, ContentDigest: core.DigestBytes([]byte(content)), Trust: trust,
		})
	}
	value, err := contract.SealReviewerContextEnvelopeV1(contract.ReviewerContextEnvelopeV1{
		Ref: contract.ReviewerContextEnvelopeRefV1{Revision: revision}, Subject: subject,
		Materials: materials, AllowedReadCapabilities: []string{"workspace.inspect"},
		ReadOnly: true, WorkIdentityRemoved: true, State: contract.ReviewerContextEnvelopeActiveV1,
		Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestReviewerContextPublishRequestRequiresCreateOnceOrFullRefCASV1(t *testing.T) {
	initial := reviewerContextProjectionForPortV1(t, 1)
	if err := (ReviewerContextPublishRequestV1{Value: initial}).Validate(); err != nil {
		t.Fatalf("valid create-once request rejected: %v", err)
	}
	next := reviewerContextProjectionForPortV1(t, 2)
	if err := (ReviewerContextPublishRequestV1{Previous: &initial.Ref, Value: next}).Validate(); err != nil {
		t.Fatalf("valid full-ref CAS rejected: %v", err)
	}
	gap := reviewerContextProjectionForPortV1(t, 3)
	if err := (ReviewerContextPublishRequestV1{Previous: &initial.Ref, Value: gap}).Validate(); !core.HasReason(err, core.ReasonRevisionConflict) {
		t.Fatal("revision gap accepted")
	}
	drift := next
	drift.Ref.ID = "reviewer-context-drift"
	if err := (ReviewerContextPublishRequestV1{Previous: &initial.Ref, Value: drift}).Validate(); err == nil {
		t.Fatal("stable identity drift accepted")
	}
}
