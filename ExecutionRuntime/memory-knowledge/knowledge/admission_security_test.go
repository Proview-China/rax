package knowledge

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestKnowledgeAdmissionBlocksPoisonSecretAndUnknownOutcome(t *testing.T) {
	for i, flag := range []string{"secret_plaintext", "pii_unapproved", "prompt_injection", "poisoning_suspected", "unsettled_effect", "unknown_outcome", "chain_of_thought"} {
		t.Run(flag, func(t *testing.T) {
			f := newFixture(t, false)
			candidate, err := f.store.SubmitCandidate(f.access, CandidateInput{TenantID: f.access.TenantID, ID: "risky-" + flag, ProducerID: "producer-risk", SourceEpoch: 2, SourceSequence: uint64(i + 1), Kind: CandidateRecord, PayloadDigest: "sha256:" + flag, EvidenceRefs: []contract.Ref{ref("evidence")}, RiskFlags: []string{flag}, TTL: time.Hour, Draft: RecordDraft{ID: "record-" + flag, PackageRef: f.pkg.Ref, ContentRef: f.contentRef, SourceRefs: []contract.Ref{f.source.Ref}, EvidenceRefs: []contract.Ref{ref("record-evidence")}, Scope: "project-a", Subject: "risk", Sensitivity: "internal", License: "internal-use", TrustState: TrustUnverified, ValidFrom: *f.now, ValidTo: f.now.Add(time.Hour)}}, contract.ExpectAbsent())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.AdmitCandidate(f.access, candidate.Ref, AdmissionCommitReady, "unsafe", time.Hour, contract.ExpectAbsent()); !errors.Is(err, contract.ErrCandidateRejected) {
				t.Fatalf("risk committed: %v", err)
			}
		})
	}
}
