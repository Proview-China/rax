package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func evidenceDigestV1(seed string) core.Digest { return core.DigestBytes([]byte(seed)) }

func evidenceAttachmentV1(t *testing.T) contract.EvidenceAttachmentV1 {
	t.Helper()
	now := time.Unix(1_900_300_000, 0)
	value, err := contract.SealEvidenceAttachmentV1(contract.EvidenceAttachmentV1{
		FactIdentityV1: contract.FactIdentityV1{TenantID: "tenant-a", ID: "attachment-a", Revision: 1, CreatedUnixNano: now.UnixNano(), UpdatedUnixNano: now.UnixNano()},
		IdempotencyKey: "attachment-key-a",
		Case:           contract.ExactResourceRefV1{ID: "case-a", Revision: 3, Digest: evidenceDigestV1("case")},
		Target:         contract.ExactResourceRefV1{ID: "target-a", Revision: 2, Digest: evidenceDigestV1("target")},
		SubmitterID:    "reviewer-a",
		Evidence: []runtimeports.ReviewEvidenceRefV2{
			{Ref: "evidence-b", Classification: "praxis.evidence/test", Digest: evidenceDigestV1("evidence-b")},
			{Ref: "evidence-a", Classification: "praxis.evidence/test", Digest: evidenceDigestV1("evidence-a")},
		},
		ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestEvidenceAttachmentV1SealsCanonicalEvidenceSet(t *testing.T) {
	value := evidenceAttachmentV1(t)
	if value.Evidence[0].Ref != "evidence-a" || value.EvidenceDigest.Validate() != nil || value.Validate() != nil {
		t.Fatalf("attachment was not canonically sealed: %+v", value)
	}
	mutated := value
	mutated.Evidence[0].Digest = evidenceDigestV1("drift")
	if err := mutated.Validate(); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("evidence drift must fail closed: %v", err)
	}
}

func TestEvidenceAttachmentV1RejectsDuplicateEvidence(t *testing.T) {
	value := evidenceAttachmentV1(t)
	value.Digest = ""
	value.Evidence = append(value.Evidence, value.Evidence[1])
	if _, err := contract.SealEvidenceAttachmentV1(value); !core.HasReason(err, core.ReasonDuplicateCanonicalKey) {
		t.Fatalf("duplicate evidence must fail closed: %v", err)
	}
}

func TestEvidenceAttachmentV1RejectsSameRefWithDifferentDigestOrClassV1(t *testing.T) {
	for _, mutate := range []func(*runtimeports.ReviewEvidenceRefV2){
		func(value *runtimeports.ReviewEvidenceRefV2) { value.Digest = evidenceDigestV1("other-digest") },
		func(value *runtimeports.ReviewEvidenceRefV2) { value.Classification = "praxis.evidence/other" },
	} {
		value := evidenceAttachmentV1(t)
		value.Digest = ""
		duplicate := value.Evidence[0]
		mutate(&duplicate)
		value.Evidence = append(value.Evidence, duplicate)
		if _, err := contract.SealEvidenceAttachmentV1(value); !core.HasReason(err, core.ReasonDuplicateCanonicalKey) {
			t.Fatalf("same Evidence Ref with changed metadata must fail closed: %v", err)
		}
	}
}
