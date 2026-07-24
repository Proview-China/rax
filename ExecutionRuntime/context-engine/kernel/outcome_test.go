package kernel_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/outcomestore"
)

func TestContextOutcomeCoordinatorExactChainV1(t *testing.T) {
	coordinator, err := kernel.NewContextOutcomeCoordinatorV1(outcomestore.NewMemory())
	if err != nil {
		t.Fatal(err)
	}
	baseline, candidate, policy := outcomeKernelRefV1("recipe-base"), outcomeKernelRefV1("recipe-candidate"), outcomeKernelRefV1("policy")
	outcome := outcomeKernelFixtureV1(candidate, policy)
	outcomeRef, err := coordinator.RecordOutcome(context.Background(), outcome)
	if err != nil {
		t.Fatal(err)
	}
	evaluation := contract.ContextEvaluationFactV1{ContractVersion: contract.Version, ID: "evaluation-kernel", Revision: 1, OutcomeRefs: []contract.FactRef{outcomeRef}, BaselineRecipeRef: baseline, CandidateRecipeRef: candidate, PolicyRef: policy, QualityScorePPM: 800_000, EconomicScorePPM: 600_000, RiskScorePPM: 100_000, Disposition: contract.ContextEvaluationBetterV1, Evidence: []contract.EvidenceRef{testkit.Evidence("evaluation-kernel")}, CreatedUnixNano: testkit.Now + 1, ExpiresUnixNano: outcome.ExpiresUnixNano}
	evaluationRef, err := coordinator.RecordEvaluation(context.Background(), evaluation)
	if err != nil {
		t.Fatal(err)
	}
	feedback := contract.ContextFeedbackCandidateFactV1{ContractVersion: contract.Version, ID: "feedback-kernel", Revision: 1, BaseRecipeRef: baseline, OutcomeRefs: []contract.FactRef{outcomeRef}, EvaluationRef: evaluationRef, ChangeDigest: testkit.D("change"), RiskScorePPM: evaluation.RiskScorePPM, Evidence: []contract.EvidenceRef{testkit.Evidence("feedback-kernel")}, State: contract.ContextFeedbackEvaluatedV1, CreatedUnixNano: evaluation.CreatedUnixNano + 1, ExpiresUnixNano: evaluation.ExpiresUnixNano}
	feedbackRef, err := coordinator.RecordFeedbackCandidate(context.Background(), feedback)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.InspectFeedbackCandidate(context.Background(), feedbackRef); err != nil {
		t.Fatal(err)
	}
}

func TestContextOutcomeCoordinatorRejectsPolicyRecipeAndTTLDriftV1(t *testing.T) {
	coordinator, _ := kernel.NewContextOutcomeCoordinatorV1(outcomestore.NewMemory())
	baseline, candidate, policy := outcomeKernelRefV1("recipe-base"), outcomeKernelRefV1("recipe-candidate"), outcomeKernelRefV1("policy")
	outcome := outcomeKernelFixtureV1(candidate, policy)
	outcomeRef, _ := coordinator.RecordOutcome(context.Background(), outcome)
	base := contract.ContextEvaluationFactV1{ContractVersion: contract.Version, ID: "evaluation-drift", Revision: 1, OutcomeRefs: []contract.FactRef{outcomeRef}, BaselineRecipeRef: baseline, CandidateRecipeRef: candidate, PolicyRef: policy, Disposition: contract.ContextEvaluationInconclusiveV1, Evidence: []contract.EvidenceRef{testkit.Evidence("evaluation")}, CreatedUnixNano: testkit.Now + 1, ExpiresUnixNano: outcome.ExpiresUnixNano}
	for _, mutate := range []func(*contract.ContextEvaluationFactV1){
		func(v *contract.ContextEvaluationFactV1) { v.PolicyRef = outcomeKernelRefV1("other-policy") },
		func(v *contract.ContextEvaluationFactV1) { v.CandidateRecipeRef = outcomeKernelRefV1("other-recipe") },
		func(v *contract.ContextEvaluationFactV1) { v.ExpiresUnixNano = outcome.ExpiresUnixNano + 1 },
	} {
		value := base
		mutate(&value)
		if _, err := coordinator.RecordEvaluation(context.Background(), value); !errors.Is(err, contract.ErrConflict) {
			t.Fatalf("drift accepted: %v", err)
		}
	}
}

func outcomeKernelFixtureV1(recipe, policy contract.FactRef) contract.ContextOutcomeFactV1 {
	return contract.ContextOutcomeFactV1{ContractVersion: contract.Version, ID: "outcome-kernel", Revision: 1, Execution: testkit.Execution(), FrameRef: outcomeKernelRefV1("frame"), ManifestRef: outcomeKernelRefV1("manifest"), RecipeRef: recipe, GenerationRef: outcomeKernelRefV1("generation"), ModelAttemptObservationRef: outcomeKernelRefV1("attempt"), ModelResponseObservationRef: outcomeKernelRefV1("response"), ToolActionRefs: []contract.FactRef{}, UserCorrectionEvidence: []contract.EvidenceRef{}, TaskEvidenceRefs: []contract.FactRef{}, Metrics: contract.ContextOutcomeMetricsV1{InputTokens: 100, OutputTokens: 10, CacheEligiblePrefixTokens: 60, CacheReadTokens: 40, DynamicTokens: 20, LatencyNanos: 1}, EvaluationPolicyRef: policy, CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + int64(time.Minute)}
}

func outcomeKernelRefV1(id string) contract.FactRef {
	return contract.FactRef{ID: id, Revision: 1, Digest: testkit.D(id)}
}
