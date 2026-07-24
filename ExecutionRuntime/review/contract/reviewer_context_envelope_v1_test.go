package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func reviewerContextSubjectV1() contract.ReviewerContextSubjectV1 {
	return contract.ReviewerContextSubjectV1{
		TenantID:           "tenant-a",
		Case:               contract.ExactResourceRefV1{ID: "case-a", Revision: 2, Digest: testkit.Digest("case")},
		Round:              contract.ExactResourceRefV1{ID: "round-a", Revision: 1, Digest: testkit.Digest("round")},
		Assignment:         contract.ExactResourceRefV1{ID: "assignment-a", Revision: 3, Digest: testkit.Digest("assignment")},
		Target:             contract.ExactResourceRefV1{ID: "target-a", Revision: 4, Digest: testkit.Digest("target")},
		Rubric:             contract.ExactResourceRefV1{ID: "rubric-a", Revision: 5, Digest: testkit.Digest("rubric")},
		ContextFrameDigest: testkit.Digest("context-frame"),
		OutputSchema: runtimeports.SchemaRefV2{
			Namespace: "praxis.review", Name: "auto-attestation", Version: "1.0.0",
			MediaType: "application/json", ContentDigest: testkit.Digest("schema"),
		},
	}
}

func reviewerContextMaterialV1(kind contract.ReviewerContextMaterialKindV1, id, content string, expires int64) contract.ReviewerContextMaterialV1 {
	trust := contract.ReviewerContextObservationV1
	switch kind {
	case contract.ReviewerContextOriginalIntentV1, contract.ReviewerContextRequirementV1, contract.ReviewerContextAcceptanceCriterionV1, contract.ReviewerContextStableRuleV1, contract.ReviewerContextConfirmedDecisionV1:
		trust = contract.ReviewerContextInstructionV1
	}
	return contract.ReviewerContextMaterialV1{
		Kind: kind,
		Source: contract.ReviewerContextSourceRefV1{
			Owner: "praxis.context/source", ID: id, Revision: 1,
			Digest: testkit.Digest("source-" + id), ExpiresUnixNano: expires,
		},
		MediaType: "text/plain; charset=utf-8", Content: content,
		ContentDigest: core.DigestBytes([]byte(content)), Trust: trust,
	}
}

func reviewerContextEnvelopeV1(t *testing.T, now time.Time) contract.ReviewerContextEnvelopeV1 {
	t.Helper()
	expires := now.Add(10 * time.Minute).UnixNano()
	materials := []contract.ReviewerContextMaterialV1{
		reviewerContextMaterialV1(contract.ReviewerContextEvidenceV1, "evidence", "tests passed", now.Add(19*time.Minute).UnixNano()),
		reviewerContextMaterialV1(contract.ReviewerContextStableRuleV1, "rule", "never dispatch from review", now.Add(18*time.Minute).UnixNano()),
		reviewerContextMaterialV1(contract.ReviewerContextCandidateV1, "candidate", "candidate payload", now.Add(17*time.Minute).UnixNano()),
		reviewerContextMaterialV1(contract.ReviewerContextRequirementV1, "requirement", "preserve exact binding", now.Add(16*time.Minute).UnixNano()),
		reviewerContextMaterialV1(contract.ReviewerContextOriginalIntentV1, "intent", "review this candidate", now.Add(15*time.Minute).UnixNano()),
		reviewerContextMaterialV1(contract.ReviewerContextAcceptanceCriterionV1, "acceptance", "all hard checks pass", expires),
		reviewerContextMaterialV1(contract.ReviewerContextKnownRiskV1, "risk", "provider outcome may be unknown", now.Add(14*time.Minute).UnixNano()),
	}
	sealed, err := contract.SealReviewerContextEnvelopeV1(contract.ReviewerContextEnvelopeV1{
		Ref: contract.ReviewerContextEnvelopeRefV1{Revision: 1}, Subject: reviewerContextSubjectV1(),
		Materials: materials, AllowedReadCapabilities: []string{"workspace.inspect", "evidence.inspect"},
		ReadOnly: true, WorkIdentityRemoved: true, State: contract.ReviewerContextEnvelopeActiveV1,
		Current: true, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func TestReviewerContextEnvelopeExactCurrentAndDeepCloneV1(t *testing.T) {
	now := time.Unix(1_910_000_000, 0)
	value := reviewerContextEnvelopeV1(t, now)
	if err := value.ValidateCurrent(value.Ref, value.Subject, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	clone := value.Clone()
	clone.Materials[0].Content = "mutated"
	clone.AllowedReadCapabilities[0] = "workspace.write"
	if value.Materials[0].Content == "mutated" || value.AllowedReadCapabilities[0] == "workspace.write" {
		t.Fatal("Reviewer Context clone retained mutable slice aliases")
	}
	stableID, err := contract.DeriveReviewerContextEnvelopeIDV1(value.Subject)
	if err != nil || stableID != value.Ref.ID {
		t.Fatalf("stable ID drift: id=%q err=%v", stableID, err)
	}
}

func TestReviewerContextEnvelopeHardNegativesV1(t *testing.T) {
	now := time.Unix(1_910_000_100, 0)
	base := reviewerContextEnvelopeV1(t, now)
	tests := map[string]func(*contract.ReviewerContextEnvelopeV1){
		"missing-required-material": func(v *contract.ReviewerContextEnvelopeV1) {
			for i := range v.Materials {
				if v.Materials[i].Kind == contract.ReviewerContextKnownRiskV1 {
					v.Materials = append(v.Materials[:i], v.Materials[i+1:]...)
					break
				}
			}
		},
		"duplicate-intent": func(v *contract.ReviewerContextEnvelopeV1) { v.Materials = append(v.Materials, v.Materials[0]) },
		"instruction-from-candidate": func(v *contract.ReviewerContextEnvelopeV1) {
			for i := range v.Materials {
				if v.Materials[i].Kind == contract.ReviewerContextCandidateV1 {
					v.Materials[i].Trust = contract.ReviewerContextInstructionV1
				}
			}
		},
		"content-digest-drift": func(v *contract.ReviewerContextEnvelopeV1) { v.Materials[0].Content = "drift" },
		"forbidden-write": func(v *contract.ReviewerContextEnvelopeV1) {
			v.AllowedReadCapabilities = append(v.AllowedReadCapabilities, "workspace.write")
		},
		"forbidden-dispatch": func(v *contract.ReviewerContextEnvelopeV1) {
			v.AllowedReadCapabilities = append(v.AllowedReadCapabilities, "tool.dispatch")
		},
		"wrong-min-ttl": func(v *contract.ReviewerContextEnvelopeV1) { v.ExpiresUnixNano++ },
		"work-identity-present": func(v *contract.ReviewerContextEnvelopeV1) {
			v.WorkIdentityRemoved = false
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := base.Clone()
			mutate(&value)
			value.Ref.Digest, value.ProjectionDigest = "", ""
			if _, err := contract.SealReviewerContextEnvelopeV1(value); err == nil {
				t.Fatal("invalid Reviewer Context was sealed")
			}
		})
	}
}

func TestReviewerContextEnvelopeCurrentnessFailuresV1(t *testing.T) {
	now := time.Unix(1_910_000_200, 0)
	value := reviewerContextEnvelopeV1(t, now)
	drift := value.Ref
	drift.Digest = testkit.Digest("drift")
	if err := value.ValidateCurrent(drift, value.Subject, now.Add(time.Second)); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("drifted full ref accepted: %v", err)
	}
	subject := value.Subject
	subject.Target.Digest = testkit.Digest("target-drift")
	if err := value.ValidateCurrent(value.Ref, subject, now.Add(time.Second)); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("drifted subject accepted: %v", err)
	}
	if err := value.ValidateCurrent(value.Ref, value.Subject, now.Add(-time.Nanosecond)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback accepted: %v", err)
	}
	if err := value.ValidateCurrent(value.Ref, value.Subject, time.Unix(0, value.ExpiresUnixNano)); !core.HasReason(err, core.ReasonReviewVerdictStale) {
		t.Fatalf("TTL boundary accepted: %v", err)
	}
}

func TestReviewerContextEnvelopeLiteralDigestGoldenV1(t *testing.T) {
	value := reviewerContextEnvelopeV1(t, time.Unix(1_910_000_300, 0))
	const want = "sha256:936a9661be43890f0f43ed4d27ba15eac70b4f3d5101890406e2f44c37561790"
	if string(value.ProjectionDigest) != want {
		t.Fatalf("literal digest drift: got %s want %s", value.ProjectionDigest, want)
	}
}
