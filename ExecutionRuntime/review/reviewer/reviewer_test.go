package reviewer_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/reviewer"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestReviewerContextReadOnlyAndCurrent(t *testing.T) {
	now := time.Unix(1_750_000_000, 0)
	f, err := reviewer.SealContextFrameV1(reviewer.ContextFrameV1{ContractVersion: contract.ContractVersionV1, TenantID: "tenant-a", CaseID: "case-a", RoundID: "round-a", TargetDigest: testkit.Digest("target"), OriginalIntentDigest: testkit.Digest("intent"), StableRulesDigest: testkit.Digest("rules"), ConfirmedDecisionsDigest: testkit.Digest("decisions"), EvidenceSetDigest: testkit.Digest("evidence"), RubricDigest: testkit.Digest("rubric"), OutputSchemaDigest: testkit.Digest("schema"), AllowedReadCapabilities: []string{"review.read/evidence", "review.read/target"}, ReadOnly: true, CreatedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.ValidateCurrent(now.Add(time.Second), f.TargetDigest); err != nil {
		t.Fatal(err)
	}
	f.AllowedReadCapabilities = []string{"review.write/verdict"}
	f.Digest = ""
	if _, err := reviewer.SealContextFrameV1(f); !core.HasCategory(err, core.ErrorForbidden) {
		t.Fatalf("expected forbidden capability, got %v", err)
	}
}

func TestTerminationDecisions(t *testing.T) {
	p := reviewer.TerminationPolicyV1{MaxRounds: 2, MaxTokens: 100, MaxDuration: time.Minute, MaxCostMicros: 1000, MaxRepeatedFinding: 2, MaxRepeatedRejection: 2}
	cases := []struct {
		p    reviewer.TerminationProgressV1
		want reviewer.TerminationDecisionV1
	}{{reviewer.TerminationProgressV1{UnknownOutcome: true}, reviewer.TerminationInspectOriginalV1}, {reviewer.TerminationProgressV1{StructuredAttestationProduced: true}, reviewer.TerminationCompleteV1}, {reviewer.TerminationProgressV1{Rounds: 2}, reviewer.TerminationEscalateHumanV1}, {reviewer.TerminationProgressV1{ParseFailed: true}, reviewer.TerminationFailClosedV1}, {reviewer.TerminationProgressV1{}, reviewer.TerminationContinueV1}}
	for _, tc := range cases {
		got, err := reviewer.EvaluateTerminationV1(p, tc.p)
		if err != nil {
			t.Fatal(err)
		}
		if got.Decision != tc.want {
			t.Fatalf("got %s want %s", got.Decision, tc.want)
		}
	}
}

func TestTerminationCeilingsCannotBeMaskedByStructuredOutput(t *testing.T) {
	policy := reviewer.TerminationPolicyV1{MaxRounds: 3, MaxTokens: 100, MaxDuration: time.Minute, MaxCostMicros: 1000, MaxRepeatedFinding: 2, MaxRepeatedRejection: 2}
	for name, progress := range map[string]reviewer.TerminationProgressV1{
		"tokens":             {Tokens: 100, StructuredAttestationProduced: true},
		"duration":           {Elapsed: time.Minute, StructuredAttestationProduced: true},
		"cost":               {CostMicros: 1000, StructuredAttestationProduced: true},
		"repeated finding":   {RepeatedFinding: 2, StructuredAttestationProduced: true},
		"repeated rejection": {RepeatedRejection: 2, StructuredAttestationProduced: true},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := reviewer.EvaluateTerminationV1(policy, progress)
			if err != nil || got.Decision != reviewer.TerminationEscalateHumanV1 {
				t.Fatalf("hard ceiling was masked by structured output: got=%+v err=%v", got, err)
			}
		})
	}

	t.Run("last round terminal output completes", func(t *testing.T) {
		got, err := reviewer.EvaluateTerminationV1(policy, reviewer.TerminationProgressV1{Rounds: 3, StructuredAttestationProduced: true, OutputResolution: contract.ResolutionAcceptV1})
		if err != nil || got.Decision != reviewer.TerminationCompleteV1 {
			t.Fatalf("terminal output at the exact round ceiling did not complete: got=%+v err=%v", got, err)
		}
	})
	t.Run("last round nonterminal output escalates", func(t *testing.T) {
		got, err := reviewer.EvaluateTerminationV1(policy, reviewer.TerminationProgressV1{Rounds: 3, StructuredAttestationProduced: true, OutputResolution: contract.ResolutionRequestChangesV1})
		if err != nil || got.Decision != reviewer.TerminationEscalateHumanV1 {
			t.Fatalf("nonterminal output at the exact round ceiling did not escalate: got=%+v err=%v", got, err)
		}
	})
}

func TestProductionTerminationBaselineAndObservationDegrade(t *testing.T) {
	p, err := reviewer.ProductionBaselineTerminationPolicyV1(2_000_000, true)
	if err != nil {
		t.Fatal(err)
	}
	if p.MaxRounds != 3 || p.MaxTokens != 64_000 || p.MaxDuration != 10*time.Minute || p.MaxRepeatedFinding != 2 || p.MaxRepeatedRejection != 2 {
		t.Fatalf("production Auto Review limits drifted: %+v", p)
	}
	degraded, err := reviewer.EvaluateTerminationV1(p, reviewer.TerminationProgressV1{Unavailable: true, ObservationOnly: true})
	if err != nil || degraded.Decision != reviewer.TerminationDeferObservationV1 {
		t.Fatalf("explicit observation-only degradation failed: %+v err=%v", degraded, err)
	}
	closed, err := reviewer.EvaluateTerminationV1(p, reviewer.TerminationProgressV1{Unavailable: true})
	if err != nil || closed.Decision != reviewer.TerminationFailClosedV1 {
		t.Fatalf("effect-capable failure did not fail closed: %+v err=%v", closed, err)
	}
	if _, err := reviewer.ProductionBaselineTerminationPolicyV1(0, false); !core.HasCategory(err, core.ErrorInvalidArgument) {
		t.Fatalf("missing tenant cost policy was guessed: %v", err)
	}
}
