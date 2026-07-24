package consolidation

import (
	"errors"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
	"testing"
	"time"
)

func r(id string) contract.Ref { return contract.Ref{ID: id, Revision: 1, Digest: "sha256:" + id} }
func TestConsolidationSettledInputsCanonicalAndProposalBoundary(t *testing.T) {
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	input, err := SealInputV1(InputV1{Ref: contract.Ref{ID: "input", Revision: 1}, TenantID: "tenant", ScopeRef: r("scope"), PolicyRef: r("policy"), TimelineStartRef: r("timeline-start"), TimelineEndRef: r("timeline-end"), Facts: []InputFactV1{{Kind: InputOutcome, FactRef: r("outcome"), SettlementRef: r("settlement"), EvidenceRefs: []contract.Ref{r("evidence")}}}, RuleRef: r("rule"), InputDigest: "sha256:input", CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := SealBatchV1(BatchV1{Ref: contract.Ref{ID: "batch", Revision: 1}, InputRef: input.Ref, JobAttemptRef: r("job"), Proposals: []ProposalV1{{ID: "proposal", Subject: "retry policy", Scope: "identity_private", ContentRef: contract.ContentRef{ID: "content", Digest: "sha256:content", Length: 10, MediaType: "text/plain"}, SourceRefs: []contract.Ref{r("outcome")}, EvidenceRefs: []contract.Ref{r("evidence")}, Sensitivity: "internal", FutureUse: "future transient failures", Verifiable: true, Decision: DecisionSubmitCandidate, Reason: "repeated verified pattern"}}, CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil || batch.Owner != contract.OwnerMemory {
		t.Fatalf("batch=%+v err=%v", batch, err)
	}
	tampered := batch
	tampered.Proposals[0].Reason = "tampered"
	if err := tampered.Validate(now); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("tamper accepted: %v", err)
	}
}
func TestConsolidationRejectsUnsettledDuplicateAndUnverifiableAuto(t *testing.T) {
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	base := InputV1{Ref: contract.Ref{ID: "input", Revision: 1}, TenantID: "tenant", ScopeRef: r("scope"), PolicyRef: r("policy"), TimelineStartRef: r("start"), TimelineEndRef: r("end"), RuleRef: r("rule"), InputDigest: "sha256:input", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	base.Facts = []InputFactV1{{Kind: InputOutcome, FactRef: r("unknown-outcome")}}
	if _, err := SealInputV1(base); err == nil {
		t.Fatal("unsettled outcome accepted")
	}
	base.Facts = []InputFactV1{{Kind: InputOutcome, FactRef: r("outcome"), SettlementRef: r("settlement")}, {Kind: InputOutcome, FactRef: r("outcome"), SettlementRef: r("settlement")}}
	if _, err := SealInputV1(base); !errors.Is(err, contract.ErrEvidenceConflict) {
		t.Fatalf("duplicate accepted: %v", err)
	}
	_, err := SealBatchV1(BatchV1{Ref: contract.Ref{ID: "batch", Revision: 1}, InputRef: r("input"), JobAttemptRef: r("job"), Proposals: []ProposalV1{{ID: "p", Subject: "x", Scope: "user", ContentRef: contract.ContentRef{ID: "c", Digest: "sha256:c", Length: 1, MediaType: "text/plain"}, SourceRefs: []contract.Ref{r("source")}, Sensitivity: "internal", FutureUse: "later", Decision: DecisionSubmitCandidate, Reason: "model guess"}}, CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	if !errors.Is(err, contract.ErrCandidateRejected) {
		t.Fatalf("unverifiable auto accepted: %v", err)
	}
}
