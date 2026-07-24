package memory

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestMemoryDecayDeterministicAndPinBypassesDecay(t *testing.T) {
	now := time.Date(2026, 7, 17, 11, 0, 0, 0, time.UTC)
	record := Record{CreatedAt: now, DecayPolicyRef: ref("decay"), DecayHalfLifeSeconds: 60}
	if got := recordRelevanceBPS(record, now.Add(59*time.Second)); got != 10000 {
		t.Fatalf("before half-life=%d", got)
	}
	if got := recordRelevanceBPS(record, now.Add(60*time.Second)); got != 5000 {
		t.Fatalf("one half-life=%d", got)
	}
	if got := recordRelevanceBPS(record, now.Add(120*time.Second)); got != 2500 {
		t.Fatalf("two half-lives=%d", got)
	}
	record.Pinned = true
	if got := recordRelevanceBPS(record, now.Add(24*time.Hour)); got != 10000 {
		t.Fatalf("pinned decayed=%d", got)
	}
}
func TestMemoryDecayPolicyShapeCanonical(t *testing.T) {
	f := newFixture(t)
	candidate := f.candidate("decay-candidate", CandidateCreate, "decay", contract.Ref{}, 1)
	candidate.DecayPolicyRef = ref("decay-policy")
	candidate = SealCandidate(candidate)
	if err := candidate.Validate(f.now); err == nil {
		t.Fatal("policy without half-life accepted")
	}
	candidate.DecayHalfLifeSeconds = 3600
	candidate = SealCandidate(candidate)
	if err := candidate.Validate(f.now); err != nil {
		t.Fatal(err)
	}
}
