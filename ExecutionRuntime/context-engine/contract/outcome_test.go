package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestContextOutcomeEvaluationFeedbackExactContractsV1(t *testing.T) {
	outcome := outcomeFixtureV1()
	if err := outcome.Validate(); err != nil {
		t.Fatal(err)
	}
	outcomeDigest, _ := outcome.DigestValue()
	outcomeRef := contract.FactRef{ID: outcome.ID, Revision: 1, Digest: outcomeDigest}
	evaluation := contract.ContextEvaluationFactV1{ContractVersion: contract.Version, ID: "evaluation-1", Revision: 1, OutcomeRefs: []contract.FactRef{outcomeRef}, BaselineRecipeRef: refOutcomeV1("recipe-base"), CandidateRecipeRef: refOutcomeV1("recipe-candidate"), PolicyRef: outcome.EvaluationPolicyRef, QualityScorePPM: 700_000, EconomicScorePPM: 600_000, RiskScorePPM: 100_000, Disposition: contract.ContextEvaluationBetterV1, Evidence: []contract.EvidenceRef{testkit.Evidence("evaluation")}, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + int64(time.Minute)}
	if err := evaluation.Validate(); err != nil {
		t.Fatal(err)
	}
	evaluationDigest, _ := evaluation.DigestValue()
	feedback := contract.ContextFeedbackCandidateFactV1{ContractVersion: contract.Version, ID: "feedback-1", Revision: 1, BaseRecipeRef: evaluation.BaselineRecipeRef, OutcomeRefs: evaluation.OutcomeRefs, EvaluationRef: contract.FactRef{ID: evaluation.ID, Revision: 1, Digest: evaluationDigest}, ChangeDigest: testkit.D("candidate-change"), RiskScorePPM: evaluation.RiskScorePPM, Evidence: []contract.EvidenceRef{testkit.Evidence("feedback")}, State: contract.ContextFeedbackEvaluatedV1, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + int64(time.Minute)}
	if err := feedback.Validate(); err != nil {
		t.Fatal(err)
	}
	if outcome.Metrics.CacheReadTokens == 0 {
		t.Fatal("fixture must prove provider cache usage can exist without any CacheEntry/hit field")
	}
}

func TestContextOutcomeContractsRejectDriftAndNonCanonicalSetsV1(t *testing.T) {
	outcome := outcomeFixtureV1()
	bad := outcome
	bad.ToolActionRefs = []contract.FactRef{refOutcomeV1("z"), refOutcomeV1("a")}
	if err := bad.Validate(); err == nil {
		t.Fatal("unsorted tool refs accepted")
	}
	bad = outcome
	bad.Metrics.CacheReadTokens = bad.Metrics.CacheEligiblePrefixTokens + 1
	if err := bad.Validate(); err == nil {
		t.Fatal("impossible cache metrics accepted")
	}
	outcomeDigest, _ := outcome.DigestValue()
	evaluation := contract.ContextEvaluationFactV1{ContractVersion: contract.Version, ID: "evaluation-1", Revision: 1, OutcomeRefs: []contract.FactRef{{ID: outcome.ID, Revision: 1, Digest: outcomeDigest}}, BaselineRecipeRef: refOutcomeV1("recipe-base"), CandidateRecipeRef: refOutcomeV1("recipe-candidate"), PolicyRef: outcome.EvaluationPolicyRef, QualityScorePPM: contract.ScorePPMMaxV1 + 1, Disposition: contract.ContextEvaluationBetterV1, Evidence: []contract.EvidenceRef{testkit.Evidence("evaluation")}, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 1}
	if err := evaluation.Validate(); err == nil {
		t.Fatal("out-of-range score accepted")
	}
}

func outcomeFixtureV1() contract.ContextOutcomeFactV1 {
	return contract.ContextOutcomeFactV1{ContractVersion: contract.Version, ID: "outcome-1", Revision: 1, Execution: testkit.Execution(), FrameRef: refOutcomeV1("frame-1"), ManifestRef: refOutcomeV1("manifest-1"), RecipeRef: refOutcomeV1("recipe-1"), GenerationRef: refOutcomeV1("generation-1"), ModelAttemptObservationRef: refOutcomeV1("model-attempt-1"), ModelResponseObservationRef: refOutcomeV1("model-response-1"), ToolActionRefs: []contract.FactRef{refOutcomeV1("tool-action-1")}, UserCorrectionEvidence: []contract.EvidenceRef{}, TaskEvidenceRefs: []contract.FactRef{}, Metrics: contract.ContextOutcomeMetricsV1{InputTokens: 1000, OutputTokens: 100, CacheEligiblePrefixTokens: 700, CacheReadTokens: 600, CacheWriteTokens: 0, DynamicTokens: 200, RetryCount: 1, LatencyNanos: uint64(time.Second), CostMicros: 900, CompactionLossTokens: 10}, EvaluationPolicyRef: refOutcomeV1("evaluation-policy-1"), CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + int64(time.Minute)}
}

func refOutcomeV1(id string) contract.FactRef {
	return contract.FactRef{ID: id, Revision: 1, Digest: testkit.D(id)}
}
