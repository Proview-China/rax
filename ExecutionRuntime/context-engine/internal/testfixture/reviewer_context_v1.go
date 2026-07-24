package testfixture

import (
	"sync"
	"time"

	reviewcontract "github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type SequenceClockV1 struct {
	mu     sync.Mutex
	values []time.Time
	last   time.Time
}

func NewSequenceClockV1(values ...time.Time) *SequenceClockV1 {
	return &SequenceClockV1{values: append([]time.Time(nil), values...)}
}

func (c *SequenceClockV1) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.values) > 0 {
		c.last = c.values[0]
		c.values = c.values[1:]
	}
	return c.last
}

func ReviewerContextSubjectV1(suffix string) reviewcontract.ReviewerContextSubjectV1 {
	return reviewcontract.ReviewerContextSubjectV1{
		TenantID:           core.TenantID("tenant-" + suffix),
		Case:               exactReviewerContextRefV1("case-"+suffix, "case-"+suffix),
		Round:              exactReviewerContextRefV1("round-"+suffix, "round-"+suffix),
		Assignment:         exactReviewerContextRefV1("assignment-"+suffix, "assignment-"+suffix),
		Target:             exactReviewerContextRefV1("target-"+suffix, "target-"+suffix),
		Rubric:             exactReviewerContextRefV1("rubric-"+suffix, "rubric-"+suffix),
		ContextFrameDigest: core.DigestBytes([]byte("frame-" + suffix)),
		OutputSchema: runtimeports.SchemaRefV2{
			Namespace: "praxis.review", Name: "attestation", Version: "1.0.0",
			MediaType: "application/json", ContentDigest: core.DigestBytes([]byte("schema-" + suffix)),
		},
	}
}

func ReviewerContextEnvelopeV1(subject reviewcontract.ReviewerContextSubjectV1, revision core.Revision, checked, expires time.Time, variant string) (reviewcontract.ReviewerContextEnvelopeV1, error) {
	kinds := []reviewcontract.ReviewerContextMaterialKindV1{
		reviewcontract.ReviewerContextOriginalIntentV1,
		reviewcontract.ReviewerContextRequirementV1,
		reviewcontract.ReviewerContextAcceptanceCriterionV1,
		reviewcontract.ReviewerContextStableRuleV1,
		reviewcontract.ReviewerContextCandidateV1,
		reviewcontract.ReviewerContextEvidenceV1,
		reviewcontract.ReviewerContextKnownRiskV1,
	}
	materials := make([]reviewcontract.ReviewerContextMaterialV1, 0, len(kinds))
	for _, kind := range kinds {
		content := string(kind) + "-" + variant
		trust := reviewcontract.ReviewerContextObservationV1
		switch kind {
		case reviewcontract.ReviewerContextOriginalIntentV1,
			reviewcontract.ReviewerContextRequirementV1,
			reviewcontract.ReviewerContextAcceptanceCriterionV1,
			reviewcontract.ReviewerContextStableRuleV1:
			trust = reviewcontract.ReviewerContextInstructionV1
		}
		materials = append(materials, reviewcontract.ReviewerContextMaterialV1{
			Kind: kind,
			Source: reviewcontract.ReviewerContextSourceRefV1{
				Owner: "praxis.context/source", ID: "source-" + string(kind) + "-" + variant,
				Revision: 1, Digest: core.DigestBytes([]byte("source-" + string(kind) + "-" + variant)),
				ExpiresUnixNano: expires.UnixNano(),
			},
			MediaType: "text/plain", Content: content,
			ContentDigest: core.DigestBytes([]byte(content)), Trust: trust,
		})
	}
	return reviewcontract.SealReviewerContextEnvelopeV1(reviewcontract.ReviewerContextEnvelopeV1{
		Ref: reviewcontract.ReviewerContextEnvelopeRefV1{Revision: revision}, Subject: subject,
		Materials: materials, AllowedReadCapabilities: []string{"workspace.inspect"},
		ReadOnly: true, WorkIdentityRemoved: true,
		State: reviewcontract.ReviewerContextEnvelopeActiveV1, Current: true,
		CheckedUnixNano: checked.UnixNano(), ExpiresUnixNano: expires.UnixNano(),
	})
}

func exactReviewerContextRefV1(id, digest string) reviewcontract.ExactResourceRefV1 {
	return reviewcontract.ExactResourceRefV1{ID: id, Revision: 1, Digest: core.DigestBytes([]byte(digest))}
}
