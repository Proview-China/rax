package contract_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/fakes"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/internal/testkit"
)

func TestCustomComponentGovernedFactConformanceNeverSelfGrantsAuthority(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	store := fakes.NewGovernedStoreV2()
	store.Clock = func() time.Time { return now }
	session, candidate := testkit.GovernedFactsV2(now)
	report, err := conformance.CheckGovernedFactsV2(context.Background(), conformance.GovernedFactCaseV2{Sessions: store, Candidates: store, Session: session, Candidate: candidate})
	if err != nil {
		t.Fatal(err)
	}
	if !report.SessionCreateIdempotent || !report.CandidateCreateIdempotent || !report.SessionCASLinearized || !report.ExactInspectVerified || !report.CertificationCandidate {
		t.Fatalf("incomplete conformance report: %#v", report)
	}
	if report.ProductionClaimEligible || report.DispatchEligible || report.CompletionEligible {
		t.Fatalf("fixture self-granted production authority: %#v", report)
	}
}
