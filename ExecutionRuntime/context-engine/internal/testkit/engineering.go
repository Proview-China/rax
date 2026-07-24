package testkit

import (
	"fmt"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func EngineeringRefV1(id string) contract.FactRef {
	return contract.FactRef{ID: id, Revision: 1, Digest: D(id)}
}

func EngineeringEvaluatorRefV1() contract.ContextEvaluatorRefV1 {
	return contract.ContextEvaluatorRefV1{ID: "evaluator-local-v1", Revision: 1, Digest: D("evaluator-local-v1")}
}

func EngineeringOutcomesV1() (contract.FactRef, contract.FactRef, contract.FactRef, []contract.ContextOutcomeFactV1) {
	baseline := EngineeringRefV1("recipe-baseline")
	candidate := EngineeringRefV1("recipe-candidate")
	policy := EngineeringRefV1("evaluation-policy")
	return baseline, candidate, policy, []contract.ContextOutcomeFactV1{
		engineeringOutcomeV1("outcome-baseline", baseline, policy, 0),
		engineeringOutcomeV1("outcome-candidate", candidate, policy, 1),
	}
}

func EngineeringObservationV1(input contract.ContextEvaluationInputV1) contract.ContextEvaluationObservationV1 {
	value, err := contract.SealContextEvaluationObservationV1(contract.ContextEvaluationObservationV1{
		EvaluatorRef: input.EvaluatorRef, InputDigest: input.InputDigest,
		OutcomeRefs: input.OutcomeRefs, PolicyRef: input.PolicyRef,
		QualityScorePPM: 800_000, EconomicScorePPM: 700_000, RiskScorePPM: 100_000,
		Disposition: contract.ContextEvaluationBetterV1, Evidence: []contract.EvidenceRef{Evidence("evaluator-observation")},
		ObservedUnixNano: Now + int64(time.Second), ExpiresUnixNano: input.ExpiresUnixNano,
	})
	if err != nil {
		panic(err)
	}
	return value
}

func engineeringOutcomeV1(id string, recipe, policy contract.FactRef, ordinal int) contract.ContextOutcomeFactV1 {
	return contract.ContextOutcomeFactV1{
		ContractVersion: contract.Version, ID: id, Revision: 1, Execution: Execution(),
		FrameRef: EngineeringRefV1(fmt.Sprintf("frame-%d", ordinal)), ManifestRef: EngineeringRefV1(fmt.Sprintf("manifest-%d", ordinal)),
		RecipeRef: recipe, GenerationRef: EngineeringRefV1(fmt.Sprintf("generation-%d", ordinal)),
		ModelAttemptObservationRef:  EngineeringRefV1(fmt.Sprintf("model-attempt-%d", ordinal)),
		ModelResponseObservationRef: EngineeringRefV1(fmt.Sprintf("model-response-%d", ordinal)),
		ToolActionRefs:              []contract.FactRef{}, UserCorrectionEvidence: []contract.EvidenceRef{}, TaskEvidenceRefs: []contract.FactRef{},
		Metrics:             contract.ContextOutcomeMetricsV1{InputTokens: 100, OutputTokens: 10, CacheEligiblePrefixTokens: 60, CacheReadTokens: 40, DynamicTokens: 20, LatencyNanos: 1},
		EvaluationPolicyRef: policy, CreatedUnixNano: Now - int64(time.Minute), ExpiresUnixNano: Now + int64(time.Minute),
	}
}
