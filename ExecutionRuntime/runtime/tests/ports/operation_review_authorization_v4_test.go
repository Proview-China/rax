package ports_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestOperationReviewAuthorizationV4EvidenceCanonicalSet(t *testing.T) {
	t.Parallel()
	first := ports.EvidenceRecordRefV2{LedgerScopeDigest: generationPortDigestV1(t, "ledger-a"), Sequence: 1, RecordDigest: generationPortDigestV1(t, "record-a")}
	second := ports.EvidenceRecordRefV2{LedgerScopeDigest: generationPortDigestV1(t, "ledger-b"), Sequence: 2, RecordDigest: generationPortDigestV1(t, "record-b")}
	forward, err := ports.DigestOperationReviewEvidenceV4([]ports.EvidenceRecordRefV2{first, second})
	if err != nil {
		t.Fatal(err)
	}
	reverse, err := ports.DigestOperationReviewEvidenceV4([]ports.EvidenceRecordRefV2{second, first})
	if err != nil || forward != reverse {
		t.Fatalf("evidence order changed canonical digest: forward=%s reverse=%s err=%v", forward, reverse, err)
	}
	if _, err := ports.DigestOperationReviewEvidenceV4([]ports.EvidenceRecordRefV2{first, first}); !core.HasReason(err, core.ReasonEvidenceConflict) {
		t.Fatalf("duplicate evidence must fail closed: %v", err)
	}
	if _, err := ports.DigestOperationReviewEvidenceV4(nil); !core.HasReason(err, core.ReasonEvidenceUnavailable) {
		t.Fatalf("missing evidence must fail closed: %v", err)
	}
}

func TestOperationReviewAuthorizationV4RequestRejectsUnscopedOrUnboundedInput(t *testing.T) {
	t.Parallel()
	request := ports.CreateOperationReviewAuthorizationRequestV4{}
	if err := request.Validate(); !core.HasReason(err, core.ReasonReviewVerdictMissing) {
		t.Fatalf("empty public request did not fail closed: %v", err)
	}
}
