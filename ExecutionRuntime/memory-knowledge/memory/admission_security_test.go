package memory

import (
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestMemoryAdmissionBlocksPoisonSecretAndUnknownOutcome(t *testing.T) {
	for _, flag := range []string{"secret_plaintext", "pii_unapproved", "prompt_injection", "poisoning_suspected", "unsettled_effect", "unknown_outcome", "chain_of_thought"} {
		t.Run(flag, func(t *testing.T) {
			f := newFixture(t)
			candidate := f.candidate("candidate", CandidateCreate, "unsafe", contract.Ref{}, 1)
			candidate.RiskFlags = []string{flag}
			candidate = SealCandidate(candidate)
			if _, err := f.store.SubmitCandidate(f.access, candidate); err != nil {
				t.Fatal(err)
			}
			_, err := f.store.Admit(f.access, AdmissionRequest{ID: "admission", CandidateRef: candidate.Ref(), Decision: AdmissionCommitReady, ExpiresAt: f.now.Add(1000000000), ExpectedRevision: contract.ExpectAbsent()})
			if !errors.Is(err, contract.ErrCandidateRejected) {
				t.Fatalf("risk committed: %v", err)
			}
		})
	}
}
